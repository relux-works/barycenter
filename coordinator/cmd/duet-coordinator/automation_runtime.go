package main

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	automationcontract "relux.works/duet/coordinator/internal/automation"
	"relux.works/duet/coordinator/internal/media"
	"relux.works/duet/coordinator/internal/store"
)

func (api *onboardingAPI) prepareAutomationBuiltinCue(input automationTriggerInput) error {
	principal, err := api.store.ResolveAutomationPrincipalSecret(input.Secret, input.Now)
	if err != nil {
		return err
	}
	cues, err := api.store.ListSavedCues(principal.OwnerOrbitID)
	if err != nil {
		return err
	}
	builtin := false
	for _, cue := range cues {
		if cue.ID == input.CueID {
			builtin = cue.SourceKind == store.SavedCueSourceBuiltin
			break
		}
	}
	if !builtin {
		return nil
	}
	payload, err := automationcontract.BuiltinRecordingCueWAV()
	if err != nil {
		return err
	}
	storageKey := store.AutomationBuiltinStorageKey(principal.OwnerOrbitID)
	path, ok := media.CanonicalPath(filepath.Join(api.config.MediaDir, "canonical"), storageKey)
	if !ok {
		return fmt.Errorf("invalid automation builtin storage key")
	}
	if raw, readErr := os.ReadFile(path); readErr == nil {
		digest := sha256.Sum256(raw)
		if len(raw) == len(payload) && fmt.Sprintf("%x", digest) == automationcontract.BuiltinCueSHA256 {
			_, err = api.store.EnsureAutomationBuiltinMedia(input.Secret, input.CueID, input.Now)
			return err
		}
	} else if !os.IsNotExist(readErr) {
		return readErr
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(filepath.Dir(path), ".automation-builtin-*")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return err
	}
	if _, err := temporary.Write(payload); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	_, err = api.store.EnsureAutomationBuiltinMedia(input.Secret, input.CueID, input.Now)
	return err
}

func (api *onboardingAPI) reconcileAutomationCancellations(now int64) error {
	results, err := api.store.CancelInvalidAutomationRuntime(now, 1000)
	if err != nil {
		return err
	}
	for _, result := range results {
		api.transmissionCancelled(result)
	}
	return nil
}

func (api *onboardingAPI) runAutomationRuntime(stop <-chan struct{}) {
	if _, err := api.store.ReconcileAutomationRuntimeClaims(time.Now().UTC().UnixMilli()); err != nil {
		api.log.Error("automation startup claim reconciliation failed", "err", err)
	}
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case now := <-ticker.C:
			if _, err := api.store.ReconcileAutomationRuntimeClaims(now.UTC().UnixMilli()); err != nil {
				api.log.Error("automation claim reconciliation failed", "err", err)
			}
			results, err := api.store.RunDueAutomationRuntime(
				api.transmissionAvailability(), now.UTC().UnixMilli(), 1000)
			if err != nil {
				api.log.Error("automation schedule evaluation failed", "err", err)
			} else {
				for _, result := range results {
					api.transmissionAccepted(result.Transmission.Transmission.ID)
				}
			}
			if err := api.reconcileAutomationCancellations(now.UTC().UnixMilli()); err != nil {
				api.log.Error("automation cancellation reconciliation failed", "err", err)
			}
		}
	}
}

// TriggerAutomation is the only scoped-automation adapter. It contributes no
// capture, playback, or target-policy implementation of its own: the store
// atomically seals the accepted execution into the ordinary transmission
// scheduler, and this adapter only wakes that scheduler after commit.
func (api *onboardingAPI) TriggerAutomation(input automationTriggerInput) (automationTriggerOutput, error) {
	if err := api.prepareAutomationBuiltinCue(input); err != nil &&
		!errors.Is(err, store.ErrAutomationCueNotReady) {
		return automationTriggerOutput{}, err
	}
	references := append([]string(nil), input.TargetReferences...)
	sort.Strings(references)
	canonical, err := json.Marshal(struct {
		CueID      string   `json:"cue_id"`
		Audience   string   `json:"audience"`
		References []string `json:"references"`
		Delivery   string   `json:"delivery"`
	}{
		CueID: input.CueID, Audience: string(input.AudienceKind),
		References: references, Delivery: string(input.Delivery),
	})
	if err != nil {
		return automationTriggerOutput{}, err
	}
	result, err := api.store.TriggerAutomationRuntime(store.AutomationRuntimeTriggerParams{
		Secret: input.Secret, IdempotencyKey: input.IdempotencyKey,
		RequestDigest: hashAutomationHTTP(string(canonical)), CueID: input.CueID,
		AudienceKind: input.AudienceKind, TargetReferences: references,
		Availability: api.transmissionAvailability(), AttemptedAt: input.Now,
	})
	if err != nil {
		return automationTriggerOutput{}, err
	}
	if !result.Replayed {
		api.transmissionAccepted(result.Transmission.Transmission.ID)
	}
	return automationTriggerOutput{Execution: result.Execution, Replayed: result.Replayed}, nil
}
