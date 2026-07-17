package main

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/bot"
	"relux.works/duet/coordinator/internal/hub"
	"relux.works/duet/coordinator/internal/protocol"
	"relux.works/duet/coordinator/internal/store"
)

func telegramAutomationButtonContaining(t *testing.T,
	prompts []capturedTelegramHistoryPrompt, parts ...string) (string, int64) {
	t.Helper()
	for index := len(prompts) - 1; index >= 0; index-- {
		for _, row := range prompts[index].keyboard {
			for _, button := range row {
				matched := true
				for _, part := range parts {
					matched = matched && strings.Contains(button.Text, part)
				}
				if matched {
					return button.Data, prompts[index].messageID
				}
			}
		}
	}
	t.Fatalf("Telegram automation button containing %v not found", parts)
	return "", 0
}

func telegramAutomationControlAuth(ctx store.ActorContext, identity store.Identity,
	key, request string, now int64) store.AutomationControlAuth {
	return store.AutomationControlAuth{
		ExpectedActorID: ctx.ActorID, Identity: identity,
		IdempotencyKeyHash: hashAutomationHTTP("key:" + key),
		RequestHash:        hashAutomationHTTP("request:" + request), Now: now,
	}
}

func TestTelegramSoundboardAutomationParityUsesCanonicalServices(t *testing.T) {
	harness := newOnboardingHarness(t)
	created := createViaAPI(t, harness)
	orbitID := int64(created["orbit_id"].(float64))
	originalActorID := int64(created["actor_id"].(float64))
	control := created["control_token"].(string)
	link, err := harness.store.IssueTelegramLink(originalActorID, control, "companion")
	if err != nil {
		t.Fatal(err)
	}
	const telegramUserID = int64(8_800_888)
	if _, err := harness.store.ConsumeTelegramLink(telegramUserID,
		"Telegram automation owner", "private", link.Code); err != nil {
		t.Fatal(err)
	}
	if err := harness.store.TransferPrimary(orbitID, telegramUserID); err != nil {
		t.Fatal(err)
	}
	ctx, err := harness.store.ResolveTelegramActorContext(telegramUserID)
	if err != nil || ctx.Role != "primary" {
		t.Fatalf("Telegram context=%+v err=%v", ctx, err)
	}
	identity := store.Identity{Kind: store.IdentityTelegram, TelegramUserID: telegramUserID}

	// Policy acceptance is actor-scoped; Telegram reuses the same canonical
	// acceptance service before it can arm automation.
	policyReplies := &replies{}
	loopForPolicy := newLoop(slog.Default(), harness.api.config, &fakeSender{},
		harness.store, nil, nil)
	loopForPolicy.warmup()
	loopForPolicy.handleBot(telegramBotEvent(t, telegramUserID, "private",
		"/accept_content_policy en", policyReplies))
	if len(policyReplies.texts) == 0 {
		t.Fatal("Telegram content policy acceptance produced no outcome")
	}

	now := time.Now().UnixMilli()
	feature, err := harness.store.ReplaceAuthorizedAutomationFeatureState(
		telegramAutomationControlAuth(ctx, identity, "feature", "on", now),
		store.AutomationFeatureControlParams{
			SoundboardEnabled: true, AutomationEnabled: true, Timezone: "UTC",
			QuietHours: []store.AutomationQuietWindow{}, ExpectedRevision: 0,
		})
	if err != nil {
		t.Fatal(err)
	}
	cue, err := harness.store.CreateAuthorizedSavedCue(
		telegramAutomationControlAuth(ctx, identity, "cue", "door-bell", now+1),
		store.CreateSavedCueControlParams{Title: "Door bell",
			BuiltinAssetID: store.BuiltinRecordingCueAssetID,
			BuiltinSHA256:  store.BuiltinRecordingCueSHA256})
	if err != nil {
		t.Fatal(err)
	}
	schedule, err := harness.store.CreateAuthorizedAutomationSchedule(
		telegramAutomationControlAuth(ctx, identity, "schedule", "morning", now+2),
		store.AutomationScheduleControlParams{
			CueID: cue.Cue.ID, DisplayName: "Morning bell", Timezone: "UTC",
			WeekdaysMask: 127, LocalMinute: 480, AudienceKind: "own_barycenter",
			AdditionalQuietHours: []store.AutomationQuietWindow{},
			PolicyRevision:       feature.State.Revision,
		})
	if err != nil {
		t.Fatal(err)
	}

	capabilities, err := protocol.ParseCapabilitySet([]string{
		protocol.CapabilityMediaClip, protocol.CapabilityOverlayMix,
	})
	if err != nil {
		t.Fatal(err)
	}
	slot := protocol.NodeID(created["slot"].(string))
	fake := &fakeSender{snapshots: map[hub.NodeKey]hub.NodeSnapshot{
		{Orbit: orbitID, Slot: slot}: {
			Connected: true, LastSeenAt: now, Capabilities: capabilities,
			CredentialTokenHash: transmissionDigest(created["node_token"].(string)),
		},
	}}
	l := newLoop(slog.Default(), harness.api.config, fake, harness.store, nil, nil)
	l.warmup()
	var prompts []capturedTelegramHistoryPrompt
	nextMessageID := int64(1200)
	l.telegramInlinePrompt = func(chatID int64, text string, builder bot.InlineKeyboardBuilder) {
		nextMessageID++
		keyboard, buildErr := builder(nextMessageID)
		if buildErr != nil {
			t.Fatalf("build Telegram automation keyboard: %v", buildErr)
		}
		prompts = append(prompts, capturedTelegramHistoryPrompt{
			chatID: chatID, messageID: nextMessageID, text: text, keyboard: keyboard,
		})
	}

	groupReplies := &replies{}
	l.handleBot(telegramBotEvent(t, telegramUserID, "group", "/soundboard", groupReplies))
	if !strings.Contains(groupReplies.last(t), "личном чате") {
		t.Fatalf("group soundboard reply=%q", groupReplies.last(t))
	}
	l.handleBot(telegramBotEvent(t, telegramUserID, "private", "/soundboard", &replies{}))
	if len(prompts) != 1 || !strings.Contains(prompts[0].text, "Soundboard") {
		t.Fatalf("soundboard prompts=%+v", prompts)
	}
	selectToken, selectMessage := telegramAutomationButtonContaining(t, prompts, "Door bell")
	if !strings.HasPrefix(selectToken, "tg1_") || strings.Contains(selectToken, cue.Cue.ID) {
		t.Fatalf("cue callback is not opaque: %q", selectToken)
	}
	var selectAnswer bot.CallbackAnswerCode
	selectCleared := false
	l.handleBot(telegramAirCallbackEvent(t, telegramUserID, selectMessage,
		"select-cue", selectToken, &replies{}, &selectAnswer, &selectCleared))
	if selectAnswer != bot.CallbackApplied || !selectCleared || len(prompts) != 2 {
		t.Fatalf("select answer=%s cleared=%v prompts=%d", selectAnswer, selectCleared, len(prompts))
	}
	triggerToken, triggerMessage := telegramAutomationButtonContaining(t, prompts,
		"Overlay", "own Barycenter")

	// Forwarding to another message fails without consuming or disclosing the
	// route; the authorized click can still execute afterwards.
	var forwardedAnswer bot.CallbackAnswerCode
	forwardedReplies := &replies{}
	l.handleBot(telegramAirCallbackEvent(t, telegramUserID, triggerMessage+99,
		"forwarded-trigger", triggerToken, forwardedReplies, &forwardedAnswer, new(bool)))
	if forwardedAnswer != bot.CallbackForbidden || len(forwardedReplies.texts) != 0 {
		t.Fatalf("forwarded answer=%s replies=%v", forwardedAnswer, forwardedReplies.texts)
	}
	var triggerAnswer bot.CallbackAnswerCode
	triggerCleared := false
	triggerReplies := &replies{}
	l.handleBot(telegramAirCallbackEvent(t, telegramUserID, triggerMessage,
		"trigger-cue", triggerToken, triggerReplies, &triggerAnswer, &triggerCleared))
	if triggerAnswer != bot.CallbackApplied || !triggerCleared ||
		!strings.Contains(triggerReplies.last(t), "Звук принят") {
		t.Fatalf("trigger answer=%s cleared=%v replies=%v", triggerAnswer,
			triggerCleared, triggerReplies.texts)
	}
	work, err := harness.store.ListTransmissionSchedulerWork(10)
	if err != nil || len(work) != 1 || work[0].Transmission.SourceActorID != ctx.ActorID {
		t.Fatalf("scheduler work=%+v err=%v", work, err)
	}
	var repeatAnswer bot.CallbackAnswerCode
	l.handleBot(telegramAirCallbackEvent(t, telegramUserID, triggerMessage,
		"trigger-repeat", triggerToken, &replies{}, &repeatAnswer, new(bool)))
	if repeatAnswer != bot.CallbackAlreadyApplied {
		t.Fatalf("repeat answer=%s", repeatAnswer)
	}

	prompts = nil
	l.handleBot(telegramBotEvent(t, telegramUserID, "private", "/automation", &replies{}))
	if len(prompts) != 1 || !strings.Contains(prompts[0].text, "Morning bell") {
		t.Fatalf("automation prompts=%+v", prompts)
	}
	enableToken, enableMessage := telegramAutomationButtonContaining(t, prompts,
		"Enable", "Morning bell")
	var enableAnswer bot.CallbackAnswerCode
	l.handleBot(telegramAirCallbackEvent(t, telegramUserID, enableMessage,
		"enable-schedule", enableToken, &replies{}, &enableAnswer, new(bool)))
	if enableAnswer != bot.CallbackApplied {
		t.Fatalf("enable answer=%s", enableAnswer)
	}
	schedules, err := harness.store.AuthorizedAutomationSchedulesForIdentity(ctx.ActorID, identity)
	if err != nil || len(schedules) != 1 || !schedules[0].Schedule.Enabled ||
		schedules[0].Schedule.ID != schedule.Control.Schedule.ID {
		t.Fatalf("schedules=%+v err=%v", schedules, err)
	}

	prompts = nil
	l.handleBot(telegramBotEvent(t, telegramUserID, "private", "/automation", &replies{}))
	emergencyToken, emergencyMessage := telegramAutomationButtonContaining(t, prompts,
		"Emergency stop")
	var emergencyAnswer bot.CallbackAnswerCode
	l.handleBot(telegramAirCallbackEvent(t, telegramUserID, emergencyMessage,
		"emergency-stop", emergencyToken, &replies{}, &emergencyAnswer, new(bool)))
	if emergencyAnswer != bot.CallbackApplied {
		t.Fatalf("emergency answer=%s", emergencyAnswer)
	}
	afterFeature, err := harness.store.AuthorizedAutomationFeatureStateForIdentity(ctx.ActorID, identity)
	afterSchedules, scheduleErr := harness.store.AuthorizedAutomationSchedulesForIdentity(ctx.ActorID, identity)
	if err != nil || scheduleErr != nil || !afterFeature.EmergencyDisabled ||
		len(afterSchedules) != 1 || afterSchedules[0].Schedule.Enabled {
		t.Fatalf("feature=%+v schedules=%+v err=%v schedule_err=%v",
			afterFeature, afterSchedules, err, scheduleErr)
	}

	// Removing Telegram transport changes no soundboard/automation state.
	revision, scheduleRevision := afterFeature.Revision, afterSchedules[0].Schedule.Revision
	l.telegramInlinePrompt = nil
	l.handleBot(telegramBotEvent(t, telegramUserID, "private", "/automation", &replies{}))
	unchangedFeature, _ := harness.store.AuthorizedAutomationFeatureStateForIdentity(ctx.ActorID, identity)
	unchangedSchedules, _ := harness.store.AuthorizedAutomationSchedulesForIdentity(ctx.ActorID, identity)
	if unchangedFeature.Revision != revision || unchangedSchedules[0].Schedule.Revision != scheduleRevision {
		t.Fatal("Telegram downtime mutated desktop-owned automation state")
	}
}

func TestTelegramScheduleNextRunUsesCanonicalDSTOrdering(t *testing.T) {
	schedule := store.AutomationSchedule{
		Enabled: true, Timezone: "America/New_York", WeekdaysMask: 127,
		LocalMinute: 90, // 01:30 occurs twice on the 2026 fall-back day.
	}
	now := time.Date(2026, 11, 1, 4, 59, 0, 0, time.UTC)
	next, ok := telegramScheduleNextRun(schedule, now)
	if !ok || !next.Equal(time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)) {
		t.Fatalf("next=%s ok=%v", next, ok)
	}
}
