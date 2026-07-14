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
	const goldenSHA256 = "b8b4e9654440f50751ae45d871b6a056f0cce02bf601c2ed2767def44c4c5938"
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
