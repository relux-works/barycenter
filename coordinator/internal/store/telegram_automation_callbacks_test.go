package store

import (
	"strings"
	"sync"
	"testing"
	"time"
)

func telegramAutomationFixture(t *testing.T) (*Store, OnboardingCredentials, int64, SavedCue, AutomationSchedule) {
	t.Helper()
	st, owner := newMediaIngestTestStore(t)
	now := time.Now().UnixMilli()
	feature, err := st.ReplaceAuthorizedAutomationFeatureState(
		automationControlTestAuth(owner, "telegram-feature", "enabled", now),
		AutomationFeatureControlParams{
			SoundboardEnabled: true, AutomationEnabled: true, Timezone: "UTC",
			QuietHours: []AutomationQuietWindow{}, ExpectedRevision: 0,
		})
	if err != nil || feature.State.Revision != 1 {
		t.Fatalf("feature=%+v err=%v", feature, err)
	}
	created, err := st.CreateAuthorizedSavedCue(
		automationControlTestAuth(owner, "telegram-cue", "builtin", now+1),
		CreateSavedCueControlParams{Title: "Door bell",
			BuiltinAssetID: BuiltinRecordingCueAssetID,
			BuiltinSHA256:  BuiltinRecordingCueSHA256})
	if err != nil {
		t.Fatal(err)
	}
	scheduled, err := st.CreateAuthorizedAutomationSchedule(
		automationControlTestAuth(owner, "telegram-schedule", "morning", now+2),
		AutomationScheduleControlParams{
			CueID: created.Cue.ID, DisplayName: "Morning", Timezone: "UTC",
			WeekdaysMask: 127, LocalMinute: 480, AudienceKind: "own_barycenter",
			AdditionalQuietHours: []AutomationQuietWindow{}, PolicyRevision: 1,
		})
	if err != nil {
		t.Fatal(err)
	}
	link, err := st.IssueTelegramLink(owner.ActorID, owner.ControlToken, "companion")
	if err != nil {
		t.Fatal(err)
	}
	const telegramUserID = int64(808001)
	if _, err := st.ConsumeTelegramLink(telegramUserID, "Telegram owner", "private", link.Code); err != nil {
		t.Fatal(err)
	}
	if err := st.TransferPrimary(owner.OrbitID, telegramUserID); err != nil {
		t.Fatal(err)
	}
	return st, owner, telegramUserID, created.Cue, scheduled.Control.Schedule
}

func TestTelegramAutomationCallbacksAreOpaqueActorBoundExpiringAndAtomic(t *testing.T) {
	st, _, telegramUserID, cue, schedule := telegramAutomationFixture(t)
	now := time.Now().UnixMilli()
	trigger := TelegramAutomationBinding{
		Action: TelegramAutomationTrigger, CueID: cue.ID,
		Audience: TransmissionAudienceOwnBarycenter,
		Delivery: TransmissionDeliveryOverlay, IncludeOrigin: true,
	}
	token, err := st.MintTelegramAutomationCallback(MintTelegramAutomationCallbackParams{
		TelegramUserID: telegramUserID, ChatID: telegramUserID, MessageID: 77,
		Binding: trigger, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !telegramCallbackPattern.MatchString(token) || strings.Contains(token, cue.ID) ||
		strings.Contains(token, schedule.ID) {
		t.Fatalf("callback leaked domain binding: %q", token)
	}
	forged, err := st.ClaimTelegramAutomationCallback(ClaimTelegramAutomationCallbackParams{
		TelegramUserID: telegramUserID, ChatID: telegramUserID, MessageID: 77,
		QueryID: "forged", Token: "tg1_" + strings.Repeat("A", 32), Now: now + 1,
	})
	if err != nil || forged.Found {
		t.Fatalf("forged=%+v err=%v", forged, err)
	}
	forwarded, err := st.ClaimTelegramAutomationCallback(ClaimTelegramAutomationCallbackParams{
		TelegramUserID: telegramUserID, ChatID: telegramUserID + 1, MessageID: 77,
		QueryID: "forwarded", Token: token, Now: now + 2,
	})
	if err != nil || !forwarded.Found || forwarded.Outcome != TelegramCallbackForbidden ||
		forwarded.Binding != nil {
		t.Fatalf("forwarded=%+v err=%v", forwarded, err)
	}
	claimed, err := st.ClaimTelegramAutomationCallback(ClaimTelegramAutomationCallbackParams{
		TelegramUserID: telegramUserID, ChatID: telegramUserID, MessageID: 77,
		QueryID: "claim", Token: token, Now: now + 3,
	})
	if err != nil || claimed.Binding == nil || claimed.Outcome != TelegramCallbackApplied ||
		claimed.Binding.CueRevision != cue.Revision ||
		claimed.Binding.CueSourceGeneration != cue.SourceGeneration {
		t.Fatalf("claimed=%+v err=%v", claimed, err)
	}
	if err := st.FinalizeTelegramAutomationCallback(token, "claim",
		TelegramCallbackUnsupported, now+4); err != nil {
		t.Fatal(err)
	}
	replayed, err := st.ClaimTelegramAutomationCallback(ClaimTelegramAutomationCallbackParams{
		TelegramUserID: telegramUserID, ChatID: telegramUserID, MessageID: 77,
		QueryID: "claim", Token: token, Now: now + 5,
	})
	if err != nil || !replayed.Replay || replayed.Binding != nil ||
		replayed.Outcome != TelegramCallbackUnsupported {
		t.Fatalf("replayed=%+v err=%v", replayed, err)
	}
	repeated, err := st.ClaimTelegramAutomationCallback(ClaimTelegramAutomationCallbackParams{
		TelegramUserID: telegramUserID, ChatID: telegramUserID, MessageID: 77,
		QueryID: "repeat", Token: token, Now: now + 6,
	})
	if err != nil || repeated.Binding != nil || repeated.Outcome != TelegramCallbackUnsupported {
		t.Fatalf("repeated=%+v err=%v", repeated, err)
	}

	atomicToken, err := st.MintTelegramAutomationCallback(MintTelegramAutomationCallbackParams{
		TelegramUserID: telegramUserID, ChatID: telegramUserID, MessageID: 79,
		Binding: trigger, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan TelegramAutomationCallbackResult, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for index := 0; index < 2; index++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			<-start
			result, claimErr := st.ClaimTelegramAutomationCallback(
				ClaimTelegramAutomationCallbackParams{
					TelegramUserID: telegramUserID, ChatID: telegramUserID, MessageID: 79,
					QueryID: string(rune('x' + index)), Token: atomicToken,
					Now: now + 10 + int64(index),
				})
			results <- result
			errs <- claimErr
		}(index)
	}
	close(start)
	wg.Wait()
	close(results)
	close(errs)
	for claimErr := range errs {
		if claimErr != nil {
			t.Fatal(claimErr)
		}
	}
	claimedCount, repeatedCount := 0, 0
	for result := range results {
		if result.Binding != nil && result.Outcome == TelegramCallbackApplied {
			claimedCount++
		}
		if result.Binding == nil && result.Outcome == TelegramCallbackAlreadyApplied {
			repeatedCount++
		}
	}
	if claimedCount != 1 || repeatedCount != 1 {
		t.Fatalf("atomic claims=%d repeated=%d", claimedCount, repeatedCount)
	}

	expiring, err := st.MintTelegramAutomationCallback(MintTelegramAutomationCallbackParams{
		TelegramUserID: telegramUserID, ChatID: telegramUserID, MessageID: 78,
		Binding: TelegramAutomationBinding{Action: TelegramAutomationCueSelect, CueID: cue.ID},
		Now:     now,
	})
	if err != nil {
		t.Fatal(err)
	}
	expired, err := st.ClaimTelegramAutomationCallback(ClaimTelegramAutomationCallbackParams{
		TelegramUserID: telegramUserID, ChatID: telegramUserID, MessageID: 78,
		QueryID: "expired", Token: expiring, Now: now + telegramCallbackTTL.Milliseconds(),
	})
	if err != nil || expired.Outcome != TelegramCallbackExpired || expired.Binding != nil {
		t.Fatalf("expired=%+v err=%v", expired, err)
	}
}

func TestTelegramAutomationCallbackSnapshotsScheduleFeatureAndRole(t *testing.T) {
	st, _, telegramUserID, _, schedule := telegramAutomationFixture(t)
	now := time.Now().UnixMilli()
	scheduleToken, err := st.MintTelegramAutomationCallback(MintTelegramAutomationCallbackParams{
		TelegramUserID: telegramUserID, ChatID: telegramUserID, MessageID: 88,
		Binding: TelegramAutomationBinding{
			Action: TelegramAutomationScheduleEnable, ScheduleID: schedule.ID,
		}, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	emergencyToken, err := st.MintTelegramAutomationCallback(MintTelegramAutomationCallbackParams{
		TelegramUserID: telegramUserID, ChatID: telegramUserID, MessageID: 89,
		Binding: TelegramAutomationBinding{Action: TelegramAutomationEmergencyDisable}, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := st.ResolveTelegramActorContext(telegramUserID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`UPDATE memberships SET role = 'companion' WHERE actor_id = ?`,
		ctx.ActorID); err != nil {
		t.Fatal(err)
	}
	for index, item := range []struct {
		token   string
		message int64
	}{{scheduleToken, 88}, {emergencyToken, 89}} {
		result, err := st.ClaimTelegramAutomationCallback(ClaimTelegramAutomationCallbackParams{
			TelegramUserID: telegramUserID, ChatID: telegramUserID, MessageID: item.message,
			QueryID: string(rune('a' + index)), Token: item.token, Now: now + int64(index) + 1,
		})
		if err != nil || !result.Found || result.Binding != nil ||
			result.Outcome != TelegramCallbackForbidden {
			t.Fatalf("role-changed result=%+v err=%v", result, err)
		}
	}
}
