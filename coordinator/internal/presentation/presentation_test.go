package presentation

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"relux.works/duet/coordinator/internal/store"
)

func TestGoldenEnglishAndRussianCatalog(t *testing.T) {
	labels := append(StaticLabels(),
		SenderLabel("Alice"),
		MemberLabel(""),
		OriginLabel("Home"),
		TargetLabel(TargetMetadata{OrbitTitle: "Home", Slot: "b", MultipleSlots: true}),
		TargetLabel(TargetMetadata{OrbitTitle: "Home", MultipleSlots: true}),
		AudienceLabel(store.TransmissionAudienceCurrentAir, "Peer"),
	)
	sort.Slice(labels, func(i, j int) bool { return labels[i].Key < labels[j].Key })
	raw, err := json.MarshalIndent(labels, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(raw)
	const goldenSHA256 = "3daae4adc1f1ecf3974038b78178b31c4221c6b801d68b8ae2728724ddd4eee6"
	if got := hex.EncodeToString(digest[:]); got != goldenSHA256 {
		t.Fatalf("RU/EN presentation golden digest=%s want=%s", got, goldenSHA256)
	}
}

func TestEveryFrozenTransmissionStatusAndReasonHasLocalizedCopy(t *testing.T) {
	statuses := []string{
		"processing", "ready", "error",
		string(store.TransmissionStatusAccepted), string(store.TransmissionStatusPreparing),
		string(store.TransmissionStatusScheduled), string(store.TransmissionStatusPlaying),
		string(store.TransmissionStatusCancelling), string(store.TransmissionStatusPlayed),
		string(store.TransmissionStatusPartial), string(store.TransmissionStatusFailed),
		string(store.TransmissionStatusCancelled), string(store.TransmissionStatusExpired),
		string(store.TransmissionTargetReady), string(store.TransmissionTargetMissedOffline),
		string(store.TransmissionTargetMissedDND), string(store.TransmissionTargetMissedNotReady),
		string(store.TransmissionTargetBlocked),
	}
	for _, status := range statuses {
		localized := ReceiptLabel(status, "")
		if localized.Key == "receipt.unknown" || localized.EN == "" || localized.RU == "" {
			t.Errorf("status %q has no exact localized label: %+v", status, localized)
		}
	}

	reasons := []store.TransmissionReason{
		store.TransmissionReasonCompleted, store.TransmissionReasonPartialDelivery,
		store.TransmissionReasonNoEligibleTargets, store.TransmissionReasonNoReadyTargets,
		store.TransmissionReasonAllTargetsFailed, store.TransmissionReasonOfflineAtAcceptance,
		store.TransmissionReasonOfflineBeforePrepare, store.TransmissionReasonOfflineBeforeStart,
		store.TransmissionReasonLocalDND, store.TransmissionReasonOrbitDND,
		store.TransmissionReasonPrepareDeadline, store.TransmissionReasonActorBlocked,
		store.TransmissionReasonOrbitBlocked, store.TransmissionReasonMediaDownloadFailed,
		store.TransmissionReasonMediaAuthFailed, store.TransmissionReasonMediaExpired,
		store.TransmissionReasonHashMismatch, store.TransmissionReasonDecodeFailed,
		store.TransmissionReasonDurationMismatch, store.TransmissionReasonClockUnsynchronized,
		store.TransmissionReasonStalePlay, store.TransmissionReasonDeviceUnavailable,
		store.TransmissionReasonAudioGraphFailed, store.TransmissionReasonConnectionLost,
		store.TransmissionReasonCapabilityLost, store.TransmissionReasonInterruptCapabilityLost,
		store.TransmissionReasonCancelUnacknowledged, store.TransmissionReasonInternalError,
		store.TransmissionReasonSenderCancelled, store.TransmissionReasonMediaDeleted,
		store.TransmissionReasonModerationDisabled, store.TransmissionReasonApproachLeft,
		store.TransmissionReasonApproachApart, store.TransmissionReasonTargetRevoked,
		store.TransmissionReasonDNDEnabled, store.TransmissionReasonSenderBlocked,
		store.TransmissionReasonCoordinatorRestarted, store.TransmissionReasonDeliveryExpired,
	}
	if len(reasons) != len(reasonText) {
		t.Fatalf("frozen reason inventory=%d catalog=%d", len(reasons), len(reasonText))
	}
	for _, reason := range reasons {
		localized := ReceiptLabel("failed", reason)
		if localized.Key != "receipt.reason."+string(reason) || localized.EN == "" || localized.RU == "" {
			t.Errorf("reason %q has no exact localized label: %+v", reason, localized)
		}
	}
}

func TestPrivacySafeMetadataFallbacksNeverRenderRawIdentifiers(t *testing.T) {
	labels := []Label{
		SenderLabel("987654321"),
		MemberLabel("tg_987654321"),
		OriginLabel("orbit:42"),
		TargetLabel(TargetMetadata{OrbitTitle: "42", Slot: "a", MultipleSlots: true}),
		AudienceLabel(store.TransmissionAudienceCurrentAir, "42:a"),
	}
	rendered, err := json.Marshal(labels)
	if err != nil {
		t.Fatal(err)
	}
	text := string(rendered)
	for _, forbidden := range []string{"987654321", "tg_", "orbit:42", "42:a"} {
		if strings.Contains(text, forbidden) {
			t.Errorf("raw identifier %q leaked in %s", forbidden, text)
		}
	}
	if labels[0].EN != "Unknown sender" || labels[0].RU != "Неизвестный отправитель" ||
		labels[2].EN != "Unknown Barycenter" || labels[3].EN != "Unknown recipient" ||
		labels[4].EN != "Current Air" {
		t.Fatalf("unstable privacy fallback: %+v", labels)
	}
}

func TestSharedDeliveryAudienceAndTargetSemantics(t *testing.T) {
	direct := TargetLabel(TargetMetadata{OrbitTitle: "Andromeda"})
	if direct.EN != "«Andromeda»" || direct.RU != "«Andromeda»" {
		t.Fatalf("direct target=%+v", direct)
	}
	target := TargetLabel(TargetMetadata{OrbitTitle: "Andromeda", Slot: "b", MultipleSlots: true})
	if target.EN != "«Andromeda», Pulsar B" || target.RU != "«Andromeda», Пульсар B" {
		t.Fatalf("target=%+v", target)
	}
	audience := AudienceLabel(store.TransmissionAudienceCurrentAir, "Orion")
	if audience.EN != "Current Air with «Orion»" || audience.RU != "Текущий эфир с «Orion»" {
		t.Fatalf("audience=%+v", audience)
	}
	missingSlot := TargetLabel(TargetMetadata{OrbitTitle: "Andromeda", MultipleSlots: true})
	if missingSlot.EN != "«Andromeda», unknown Pulsar" ||
		missingSlot.RU != "«Andromeda», неизвестный Пульсар" {
		t.Fatalf("missing slot target=%+v", missingSlot)
	}
	decision := PresentDelivery(
		store.TransmissionDeliveryOverlay,
		store.TransmissionDeliveryAfterCurrent,
		DowngradeMissingOverlayCapability,
	)
	if !decision.Changed || decision.Notice == nil ||
		decision.Requested.Key != "delivery.overlay" ||
		decision.Effective.Key != "delivery.after_current" ||
		decision.Notice.Key != "downgrade.missing_overlay_capability" {
		t.Fatalf("delivery decision=%+v", decision)
	}
	confirmation := ConfirmationLabel("interrupt_required")
	if confirmation.Key != "confirmation.interrupt_required" || confirmation.EN == confirmation.RU {
		t.Fatalf("confirmation=%+v", confirmation)
	}
}

func TestCallbackPolicyAndDowngradeCopyIsExactAcrossBothLocales(t *testing.T) {
	callbacks := []struct {
		code, key, en, ru string
	}{
		{"applied", "callback.applied", "Done", "Готово"},
		{"already_applied", "callback.already_applied", "Already applied", "Уже применено"},
		{"requires_confirmation", "callback.requires_confirmation", "Confirmation required", "Нужно подтверждение"},
		{"too_late", "callback.too_late", "Too late to change", "Уже поздно менять"},
		{"expired", "callback.expired", "This button has expired", "Кнопка устарела"},
		{"forbidden", "callback.forbidden", "Insufficient permission", "Недостаточно прав"},
		{"unsupported", "callback.unsupported", "This action is not available yet", "Действие пока недоступно"},
		{"failed", "callback.failed", "Could not complete the action", "Не удалось выполнить"},
	}
	for _, tc := range callbacks {
		got := CallbackResultLabel(tc.code)
		if got.Key != tc.key || got.EN != tc.en || got.RU != tc.ru {
			t.Errorf("callback %q=%+v", tc.code, got)
		}
	}
	if got := CallbackResultLabel("future_internal_value"); got.Key != "callback.failed" {
		t.Fatalf("unknown callback did not fail closed: %+v", got)
	}

	policy := []struct {
		reason store.TransmissionReason
		en, ru string
	}{
		{store.TransmissionReasonLocalDND, "Local Do Not Disturb", "Локальный режим «Не беспокоить»"},
		{store.TransmissionReasonOrbitDND, "Barycenter Do Not Disturb", "Режим «Не беспокоить» Барицентра"},
		{store.TransmissionReasonActorBlocked, "Sender is blocked", "Отправитель заблокирован"},
		{store.TransmissionReasonOrbitBlocked, "Sender's Barycenter is blocked", "Барицентр отправителя заблокирован"},
	}
	for _, tc := range policy {
		got := ReceiptLabel("failed", tc.reason)
		if got.EN != tc.en || got.RU != tc.ru {
			t.Errorf("policy reason %q=%+v", tc.reason, got)
		}
	}

	downgrade := PresentDelivery(store.TransmissionDeliveryOverlay,
		store.TransmissionDeliveryAfterCurrent, DowngradeMissingOverlayCapability)
	if downgrade.Notice == nil ||
		downgrade.Notice.EN != "Overlay is unavailable for all recipients; queued after current." ||
		downgrade.Notice.RU != "Режим поверх эфира недоступен для всех получателей; поставлено после текущего." {
		t.Fatalf("exact downgrade copy=%+v", downgrade)
	}
}

func TestHistoryModerationActionsAndOutcomesShareExactLocalizedCopy(t *testing.T) {
	actions := []struct {
		action string
		reason store.ModerationReason
		key    string
	}{
		{"replay", "", "history.action.replay"},
		{"delete", "", "history.action.delete"},
		{"block_actor", "", "history.action.block_actor"},
		{"report", store.ModerationReasonSpam, "history.action.report.spam"},
		{"report", store.ModerationReasonHarassment, "history.action.report.harassment"},
		{"report", store.ModerationReasonIllegal, "history.action.report.illegal"},
		{"report", store.ModerationReasonSexualContent, "history.action.report.sexual_content"},
		{"report", store.ModerationReasonViolence, "history.action.report.violence"},
		{"report", store.ModerationReasonOther, "history.action.report.other"},
	}
	for _, tc := range actions {
		got := HistoryActionLabel(tc.action, tc.reason)
		if got.Key != tc.key || got.EN == "" || got.RU == "" || got.EN == got.RU {
			t.Errorf("history action %s/%s=%+v", tc.action, tc.reason, got)
		}
	}
	for _, outcome := range []string{
		"media_deleted", "report_received", "report_already_received",
		"sender_blocked", "sender_already_blocked", "replay_accepted",
		"replay_already_accepted", "history_action_unavailable",
	} {
		got := HistoryActionOutcomeLabel(outcome)
		if got.Key != "history.outcome."+outcome || got.EN == "" || got.RU == "" || got.EN == got.RU {
			t.Errorf("history outcome %s=%+v", outcome, got)
		}
	}
	if got := HistoryActionLabel("report", "future_reason"); got.Key != "callback.unsupported" {
		t.Fatalf("unknown history reason did not fail closed: %+v", got)
	}
	if got := HistoryActionOutcomeLabel("future_outcome"); got.Key != "history.outcome.failed" {
		t.Fatalf("unknown history outcome did not fail closed: %+v", got)
	}
}

func TestCatalogHasNoTransportSpecificOrDuplicateKeys(t *testing.T) {
	seen := map[string]bool{}
	for _, localized := range StaticLabels() {
		if seen[localized.Key] {
			t.Errorf("duplicate catalog key %q", localized.Key)
		}
		seen[localized.Key] = true
		rendered := strings.ToLower(localized.EN + " " + localized.RU)
		for _, forbidden := range []string{"telegram", "database", "node id", "actor id", "orbit id"} {
			if strings.Contains(rendered, forbidden) {
				t.Errorf("transport/internal wording %q leaked in %+v", forbidden, localized)
			}
		}
	}
}

func TestPresentationHandoffStaysLinkedAndComplete(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..", "..")
	path := filepath.Join(repositoryRoot, "docs", "analysis", "p1-shared-delivery-presentation-model.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	handoff := string(raw)
	for _, required := range []string{
		"coordinator/internal/presentation", "PresentDelivery",
		"ReceiptLabel(status, reason)", "Unknown sender", "Неизвестный отправитель",
		"42:a", "SHA-256 golden", "requested and effective",
	} {
		if !strings.Contains(handoff, required) {
			t.Errorf("presentation handoff lost %q", required)
		}
	}
	protocolRaw, err := os.ReadFile(filepath.Join(repositoryRoot, "docs", "protocol.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(protocolRaw), "p1-shared-delivery-presentation-model.md") {
		t.Fatal("protocol entry point lost shared presentation handoff")
	}
}

func TestTelegramParityRolloutHandoffStaysComplete(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..", "..")
	path := filepath.Join(repositoryRoot, "docs", "analysis",
		"p1-telegram-history-presence-rollout-handoff.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	handoff := string(raw)
	for _, required := range []string{
		"p1-history-presence-telegram-v1",
		"EPIC-260714-th54l3",
		"GET /v1/history",
		"GET /v1/presence",
		"track_not_available_phase1",
		"media_group_not_supported_phase1",
		"tg1_",
		"15 minutes",
		"24 hours",
		"synthetic `wait`",
		"mandatory_target_missing_overlay_capability",
		"callback.failed",
		"Phase 1 history is not an inbox",
		"global runtime flag",
		"Drain-first rollback",
		"STORY-260712-2e36uz",
		"p1-telegram-history-presence-parity-regressions.md",
	} {
		if !strings.Contains(handoff, required) {
			t.Errorf("Telegram parity rollout handoff lost %q", required)
		}
	}
	protocolRaw, err := os.ReadFile(filepath.Join(repositoryRoot, "docs", "protocol.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(protocolRaw),
		"p1-telegram-history-presence-rollout-handoff.md") {
		t.Fatal("protocol entry point lost Telegram parity rollout handoff")
	}
}
