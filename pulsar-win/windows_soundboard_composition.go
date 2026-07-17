package main

import (
	"context"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

type WindowsSoundboardSnapshot struct {
	Cues          []SoundboardCue
	OrderRevision int64
	Selected      int
	Route         PhaseOneRoute
	Delivery      PhaseOneDelivery
	IncludeOrigin bool
	History       []PhaseOneHistoryItem
	Busy          bool
	Outcome       string
	Failure       string
}

type soundboardHistoryService interface {
	History(context.Context, int, string) (PhaseOneHistoryPage, error)
}

type soundboardMediaService interface {
	Upload(context.Context, string, string, string, bool) (PhaseOneUploadConfirmation, error)
	DeleteMedia(context.Context, string) error
}

type WindowsSoundboardComposition struct {
	mu              sync.RWMutex
	service         SoundboardAppService
	history         soundboardHistoryService
	media           soundboardMediaService
	mediaStore      *CaptureMediaStore
	store           WindowsSoundboardPreferenceStore
	prefs           WindowsSoundboardPreferences
	ctx             context.Context
	cancel          context.CancelFunc
	state           WindowsSoundboardSnapshot
	wake            chan struct{}
	pendingCue      string
	pendingKey      string
	pendingFallback *PhaseOneFallbackConfirmation
}

func NewWindowsSoundboardComposition(service SoundboardAppService, history soundboardHistoryService,
	store WindowsSoundboardPreferenceStore) (*WindowsSoundboardComposition, error) {
	if service == nil || history == nil || store.Path == "" {
		return nil, ErrPhaseOnePersistence
	}
	ctx, cancel := context.WithCancel(context.Background())
	prefs := store.Load()
	result := &WindowsSoundboardComposition{service: service, history: history, store: store, prefs: prefs,
		ctx: ctx, cancel: cancel, wake: make(chan struct{}, 1), state: WindowsSoundboardSnapshot{
			Route: prefs.Route, Delivery: prefs.Delivery, IncludeOrigin: prefs.IncludeOrigin, Failure: "refresh_pending"}}
	go result.run()
	return result, nil
}

func newProductionWindowsSoundboardComposition(dir string, workflow *WindowsCaptureWorkflowController) (*WindowsSoundboardComposition, error) {
	repository, err := newDefaultCredentialRepository(dir)
	if err != nil {
		return nil, err
	}
	bundle, err := repository.LoadBundle()
	if err != nil || bundle == nil {
		return nil, ErrPhaseOnePersistence
	}
	client, err := NewPhaseOneAppClient(*bundle, nil)
	if err != nil {
		return nil, err
	}
	result, err := NewWindowsSoundboardComposition(client, client,
		WindowsSoundboardPreferenceStore{Path: filepath.Join(dir, "soundboard", "preferences.v1.json")})
	if err == nil && workflow != nil {
		result.media, result.mediaStore = client, workflow.CaptureMediaStore()
	}
	return result, err
}

func (c *WindowsSoundboardComposition) Snapshot() WindowsSoundboardSnapshot {
	if c == nil {
		return WindowsSoundboardSnapshot{Route: PhaseOneOwnBarycenter, Delivery: PhaseOneOverlay, Failure: "soundboard_unavailable"}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := c.state
	result.Cues = append([]SoundboardCue(nil), c.state.Cues...)
	result.History = append([]PhaseOneHistoryItem(nil), c.state.History...)
	return result
}

func (c *WindowsSoundboardComposition) Preferences() WindowsSoundboardPreferences {
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := c.prefs
	result.Shortcuts = append([]WindowsSoundboardShortcutBinding(nil), c.prefs.Shortcuts...)
	return result
}

func (c *WindowsSoundboardComposition) ApplyShellSnapshot(shell *ShellSnapshot, shortcuts []WindowsSoundboardShortcutState) {
	if shell == nil {
		return
	}
	state := c.Snapshot()
	statusByCue := map[string]WindowsSoundboardShortcutState{}
	for _, shortcut := range shortcuts {
		statusByCue[shortcut.CueID] = shortcut
	}
	prefs := c.Preferences()
	configured := map[string]WindowsRecordingShortcut{}
	for _, binding := range prefs.Shortcuts {
		configured[binding.CueID] = binding.Shortcut
	}
	shell.SelectedSoundboardCue, shell.SoundboardRoute = state.Selected, state.Route
	shell.SoundboardDelivery, shell.SoundboardIncludeOrigin = state.Delivery, state.IncludeOrigin
	shell.SoundboardBusy, shell.SoundboardOutcome, shell.SoundboardFailure = state.Busy, state.Outcome, state.Failure
	shell.SoundboardHistoryCount = len(state.History)
	for _, cue := range state.Cues {
		item := ShellSoundboardCue{Title: cue.Title, SourceKind: cue.SourceKind, ShortcutStatus: WindowsShortcutInactive}
		if shortcut, ok := configured[cue.ID]; ok {
			item.ShortcutLabel = shortcut.Label()
		}
		if actual, ok := statusByCue[cue.ID]; ok {
			item.ShortcutStatus = actual.Status
			item.ShortcutLabel = actual.Shortcut.Label()
		}
		shell.SoundboardCues = append(shell.SoundboardCues, item)
	}
}

func (c *WindowsSoundboardComposition) SelectNextCue() {
	c.mu.Lock()
	if len(c.state.Cues) > 0 {
		c.state.Selected = (c.state.Selected + 1) % len(c.state.Cues)
		c.prefs.SelectedCueID = c.state.Cues[c.state.Selected].ID
		_ = c.store.Save(c.prefs)
	}
	c.mu.Unlock()
}

func (c *WindowsSoundboardComposition) SelectNextRoute() {
	routes := []PhaseOneRoute{PhaseOneThisPulsar, PhaseOneOwnBarycenter, PhaseOneCurrentAir}
	c.mu.Lock()
	for index, route := range routes {
		if c.state.Route == route {
			c.state.Route = routes[(index+1)%len(routes)]
			break
		}
	}
	c.prefs.Route = c.state.Route
	_ = c.store.Save(c.prefs)
	c.mu.Unlock()
}

func (c *WindowsSoundboardComposition) SelectNextDelivery() {
	deliveries := []PhaseOneDelivery{PhaseOneOverlay, PhaseOneInterrupt, PhaseOneAfterCurrent}
	c.mu.Lock()
	for index, delivery := range deliveries {
		if c.state.Delivery == delivery {
			c.state.Delivery = deliveries[(index+1)%len(deliveries)]
			break
		}
	}
	c.prefs.Delivery = c.state.Delivery
	_ = c.store.Save(c.prefs)
	c.mu.Unlock()
}

func (c *WindowsSoundboardComposition) ToggleIncludeOrigin() {
	c.mu.Lock()
	c.state.IncludeOrigin = !c.state.IncludeOrigin
	c.prefs.IncludeOrigin = c.state.IncludeOrigin
	_ = c.store.Save(c.prefs)
	c.mu.Unlock()
}

func (c *WindowsSoundboardComposition) CycleSelectedShortcut() {
	c.mu.Lock()
	if c.state.Selected < 0 || c.state.Selected >= len(c.state.Cues) {
		c.mu.Unlock()
		return
	}
	cueID, index := c.state.Cues[c.state.Selected].ID, c.state.Selected
	primary := WindowsRecordingShortcut{VirtualKey: uint32(0x70 + index%16), Modifiers: WindowsShortcutModControl | WindowsShortcutModAlt}
	secondary := WindowsRecordingShortcut{VirtualKey: primary.VirtualKey, Modifiers: WindowsShortcutModControl | WindowsShortcutModShift}
	candidate := c.prefs
	candidate.Shortcuts = append([]WindowsSoundboardShortcutBinding(nil), c.prefs.Shortcuts...)
	position := -1
	for i, binding := range candidate.Shortcuts {
		if binding.CueID == cueID {
			position = i
			break
		}
	}
	if position < 0 {
		candidate.Shortcuts = append(candidate.Shortcuts, WindowsSoundboardShortcutBinding{CueID: cueID, Shortcut: primary})
	} else if candidate.Shortcuts[position].Shortcut == primary {
		candidate.Shortcuts[position].Shortcut = secondary
	} else {
		candidate.Shortcuts = append(candidate.Shortcuts[:position], candidate.Shortcuts[position+1:]...)
	}
	if !candidate.valid() {
		c.state.Failure = "shortcut_conflict"
	} else if err := c.store.Save(candidate); err != nil {
		c.state.Failure = "shortcut_unavailable"
	} else {
		c.prefs = candidate
		c.state.Outcome = "shortcut_updated"
		c.state.Failure = ""
	}
	c.mu.Unlock()
}

func (c *WindowsSoundboardComposition) TriggerSelected()        { c.triggerCue("") }
func (c *WindowsSoundboardComposition) TriggerCue(cueID string) { c.triggerCue(cueID) }

func (c *WindowsSoundboardComposition) triggerCue(cueID string) {
	c.mu.Lock()
	if c.state.Busy {
		c.mu.Unlock()
		return
	}
	selected := c.state.Selected
	if cueID == "" && selected >= 0 && selected < len(c.state.Cues) {
		cueID = c.state.Cues[selected].ID
	}
	if !validPhaseOnePublicID(cueID, "cq_") {
		c.state.Failure = "cue_unavailable"
		c.mu.Unlock()
		return
	}
	intent := SoundboardTriggerIntent{Route: c.state.Route, Delivery: c.state.Delivery, IncludeOrigin: c.state.IncludeOrigin}
	key := "windows-soundboard-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 36)
	if c.pendingCue == cueID && c.pendingKey != "" && c.pendingFallback != nil {
		key, intent.Fallback = c.pendingKey, c.pendingFallback
	}
	c.state.Busy, c.state.Failure, c.state.Outcome = true, "", ""
	c.mu.Unlock()
	go func() {
		receipt, err := c.service.TriggerSoundboardCue(c.ctx, cueID, intent, key)
		c.mu.Lock()
		c.state.Busy = false
		if err != nil {
			var rejected *PhaseOneClientError
			if errors.As(err, &rejected) && rejected.Code == "requires_confirmation" {
				for _, alternative := range rejected.Alternatives {
					if alternative.Available && alternative.Delivery == PhaseOneAfterCurrent {
						c.pendingCue, c.pendingKey = cueID, key
						c.pendingFallback = &PhaseOneFallbackConfirmation{Token: rejected.ConfirmationToken, Delivery: PhaseOneAfterCurrent}
						c.state.Outcome = "confirmation_required"
						break
					}
				}
			}
			if c.state.Outcome != "confirmation_required" {
				c.state.Failure = phaseOneFailureCode(err)
			}
		} else if receipt.Reused {
			c.pendingCue, c.pendingKey, c.pendingFallback = "", "", nil
			c.state.Outcome = "already_accepted"
		} else {
			c.pendingCue, c.pendingKey, c.pendingFallback = "", "", nil
			c.state.Outcome = "accepted"
		}
		c.mu.Unlock()
		c.signal()
	}()
}

func (c *WindowsSoundboardComposition) RenameSelected(title string) {
	c.mutateSelected(func(cue SoundboardCue, key string) error {
		_, err := c.service.RenameSoundboardCue(c.ctx, cue.ID, title, cue.Revision, key)
		return err
	})
}

func (c *WindowsSoundboardComposition) DeleteSelected() {
	c.mutateSelected(func(cue SoundboardCue, key string) error {
		_, err := c.service.DeleteSoundboardCue(c.ctx, cue.ID, cue.Revision, key)
		return err
	})
}

func (c *WindowsSoundboardComposition) MoveSelected(delta int) {
	c.mu.RLock()
	state := c.state
	c.mu.RUnlock()
	if len(state.Cues) < 2 || state.Selected < 0 || state.Selected >= len(state.Cues) || delta == 0 {
		return
	}
	target := state.Selected + delta
	if target < 0 || target >= len(state.Cues) {
		return
	}
	ids := make([]string, len(state.Cues))
	for index, cue := range state.Cues {
		ids[index] = cue.ID
	}
	ids[state.Selected], ids[target] = ids[target], ids[state.Selected]
	c.begin(func(key string) error {
		_, err := c.service.ReorderSoundboardCues(c.ctx, ids, state.OrderRevision, key)
		return err
	})
}

func (c *WindowsSoundboardComposition) CreateMediaCue(title, mediaID string) {
	c.begin(func(key string) error {
		_, err := c.service.CreateSoundboardMediaCue(c.ctx, title, mediaID, key)
		return err
	})
}

func (c *WindowsSoundboardComposition) AcceptBrokeredCue(file WindowsBrokeredAudioFile) {
	if c == nil || c.media == nil || c.mediaStore == nil {
		if file.Release != nil {
			file.Release()
		}
		return
	}
	if !c.markBusy() {
		if file.Release != nil {
			file.Release()
		}
		return
	}
	go func() {
		if file.Release != nil {
			defer file.Release()
		}
		stream, err := file.Open()
		if err != nil || stream == nil {
			c.finish("file_unreadable", "")
			return
		}
		handle, err := c.mediaStore.ImportUserDraft(stream)
		closeErr := stream.Close()
		if err != nil || closeErr != nil {
			c.finish("file_ineligible", "")
			return
		}
		title := strings.TrimSpace(strings.TrimSuffix(brokeredDisplayName(file.DisplayName), filepath.Ext(file.DisplayName)))
		if !validPhaseOneDisplayText(title, 128, false) {
			title = "Soundboard cue"
		}
		keySuffix := strconv.FormatInt(time.Now().UTC().UnixNano(), 36)
		upload, uploadErr := c.media.Upload(c.ctx, handle.Path, title, "windows-soundboard-upload-"+keySuffix, true)
		if uploadErr != nil {
			_ = c.mediaStore.ExplicitlyDelete(handle)
			c.finish(phaseOneFailureCode(uploadErr), "")
			return
		}
		_ = c.mediaStore.ConfirmUploadAndDelete(handle)
		_, createErr := c.service.CreateSoundboardMediaCue(c.ctx, title, upload.MediaID, "windows-soundboard-create-"+keySuffix)
		if createErr != nil {
			_ = c.media.DeleteMedia(c.ctx, upload.MediaID)
			c.finish(phaseOneFailureCode(createErr), "")
			return
		}
		c.finish("", "cue_created")
	}()
}

func (c *WindowsSoundboardComposition) mutateSelected(operation func(SoundboardCue, string) error) {
	c.mu.RLock()
	state := c.state
	c.mu.RUnlock()
	if state.Selected < 0 || state.Selected >= len(state.Cues) {
		return
	}
	c.begin(func(key string) error { return operation(state.Cues[state.Selected], key) })
}

func (c *WindowsSoundboardComposition) begin(operation func(string) error) {
	if !c.markBusy() {
		return
	}
	go func() {
		err := operation("windows-soundboard-mutation-" + strconv.FormatInt(time.Now().UTC().UnixNano(), 36))
		if err != nil {
			c.finish(phaseOneFailureCode(err), "")
		} else {
			c.finish("", "updated")
		}
	}()
}

func (c *WindowsSoundboardComposition) markBusy() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.state.Busy {
		return false
	}
	c.state.Busy, c.state.Failure, c.state.Outcome = true, "", ""
	return true
}

func (c *WindowsSoundboardComposition) finish(failure, outcome string) {
	c.mu.Lock()
	c.state.Busy, c.state.Failure, c.state.Outcome = false, failure, outcome
	c.mu.Unlock()
	c.signal()
}

func (c *WindowsSoundboardComposition) run() {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		c.refresh()
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
		case <-c.wake:
		}
	}
}

func (c *WindowsSoundboardComposition) refresh() {
	ctx, cancel := context.WithTimeout(c.ctx, 4*time.Second)
	defer cancel()
	list, cueErr := c.service.SoundboardCues(ctx)
	history, historyErr := c.history.History(ctx, 30, "")
	c.mu.Lock()
	defer c.mu.Unlock()
	if cueErr == nil {
		c.state.Cues = list.Cues
		c.state.OrderRevision = list.OrderRevision
		c.state.Selected = 0
		for index, cue := range list.Cues {
			if cue.ID == c.prefs.SelectedCueID {
				c.state.Selected = index
			}
		}
	}
	if historyErr == nil {
		c.state.History = nil
		for _, item := range history.Items {
			if item.Automation != nil {
				c.state.History = append(c.state.History, item)
			}
		}
	}
	if cueErr != nil || historyErr != nil {
		c.state.Failure = "coordinator_unavailable"
	} else if !c.state.Busy {
		c.state.Failure = ""
	}
}

func (c *WindowsSoundboardComposition) signal() {
	select {
	case c.wake <- struct{}{}:
	default:
	}
}
func (c *WindowsSoundboardComposition) Close() {
	if c != nil {
		c.cancel()
	}
}
