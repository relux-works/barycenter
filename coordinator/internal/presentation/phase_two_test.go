package presentation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"relux.works/duet/coordinator/internal/store"
)

func TestPhaseTwoStatesAndCommandsAreLocalizedAndFailClosed(t *testing.T) {
	for _, state := range []PhaseTwoSurfaceState{
		PhaseTwoLoading, PhaseTwoReady, PhaseTwoStale, PhaseTwoOffline, PhaseTwoCoordinatorError,
	} {
		got := PhaseTwoSurfaceStateLabel(state)
		if got.Key != "surface."+string(state) || got.EN == "" || got.RU == "" {
			t.Errorf("state %q=%+v", state, got)
		}
	}
	if got := PhaseTwoSurfaceStateLabel("future"); got.Key != "surface.coordinator_error" {
		t.Fatalf("unknown state=%+v", got)
	}
	actions := PresentActions([]string{"replay", "dismiss", "future_action", "replay", "bad action"})
	if len(actions) != 3 || actions[0].Action != "replay" || actions[1].Action != "dismiss" ||
		actions[2].Action != "future_action" || actions[2].Label.Key != "action.unsupported" {
		t.Fatalf("actions=%+v", actions)
	}
	if !CommandAllowed(PhaseTwoReady, actions, "replay") ||
		CommandAllowed(PhaseTwoStale, actions, "replay") ||
		CommandAllowed(PhaseTwoReady, actions, "delete") ||
		CommandAllowed(PhaseTwoReady, actions, "future_action") {
		t.Fatalf("command capability gate did not fail closed: %+v", actions)
	}
}

func TestPhaseTwoTargetChoiceUsesOnlyOpaqueCurrentCapabilities(t *testing.T) {
	referenceA := "trf_" + strings.Repeat("A", 43)
	referenceB := "trf_" + strings.Repeat("B", 43)
	if !OpaqueSelectionAllowed([]string{referenceA, referenceB}, []string{referenceB}) {
		t.Fatal("current opaque reference rejected")
	}
	for name, requested := range map[string][]string{
		"empty": {}, "duplicate": {referenceA, referenceA}, "stale": {"trf_" + strings.Repeat("C", 43)},
		"raw identity": {"orbit_42"},
	} {
		if OpaqueSelectionAllowed([]string{referenceA, referenceB}, requested) {
			t.Errorf("%s selection accepted", name)
		}
	}
	choice := PresentTargetChoice(
		TargetMetadata{OrbitTitle: "Home", Slot: "b", MultipleSlots: true},
		"mixed", []string{"overlay_mix_v1", "media_clip_v1", "media_clip_v1", "bad value"},
	)
	if choice.Label.EN != "«Home», Pulsar B" || choice.Label.RU != "«Home», Пульсар B" ||
		choice.CapabilityState.Key != "capability_state.mixed" ||
		strings.Join(choice.Capabilities, ",") != "media_clip_v1,overlay_mix_v1" {
		t.Fatalf("target choice=%+v", choice)
	}
}

func TestPhaseTwoInboxHistoryAndReceiptsReuseCanonicalSemantics(t *testing.T) {
	inbox := PresentInboxItem(
		"Alice", "Home", store.TransmissionDeliveryInterrupt,
		store.TransmissionDeliveryAfterCurrent, "available", "missed_offline",
		store.TransmissionReasonOfflineAtAcceptance,
		[]string{"replay", "dismiss", "report", "block_actor"},
	)
	if inbox.Sender.EN != "Alice" || inbox.Source.EN != "From «Home»" ||
		inbox.RequestedDelivery.Key != "delivery.interrupt" ||
		inbox.EffectiveDelivery.Key != "delivery.after_current" ||
		inbox.Availability.Key != "inbox.availability.available" ||
		inbox.Receipt.Key != "receipt.reason.offline_at_acceptance" || len(inbox.Actions) != 4 {
		t.Fatalf("inbox=%+v", inbox)
	}
	history := PresentHistoryItem(
		store.HistorySentAndReceived, "Alice", "Home", "overlay", "after_current", "partial",
		store.TransmissionReasonPartialDelivery, []string{"delete", "replay"},
	)
	if history.Direction.Key != "history.direction.sent_and_received" ||
		history.RequestedDelivery == nil || history.RequestedDelivery.Key != "delivery.overlay" ||
		history.EffectiveDelivery == nil || history.EffectiveDelivery.Key != "delivery.after_current" ||
		history.Status.Key != "receipt.reason.partial_delivery" {
		t.Fatalf("history=%+v", history)
	}
	receipt := PresentHistoryReceipt("blocked", store.TransmissionReasonReported)
	if receipt.Status.Key != "receipt.status.blocked" || strings.Contains(strings.ToLower(receipt.Status.EN), "report") {
		t.Fatalf("reporter-private reason must not gain presentation copy: %+v", receipt)
	}
	raw, err := json.Marshal([]any{inbox, history, receipt})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"actor_id", "orbit_id", "telegram", "media_id", "transmission_id"} {
		if strings.Contains(strings.ToLower(string(raw)), forbidden) {
			t.Errorf("private field %q leaked in %s", forbidden, raw)
		}
	}
}

func TestPhaseTwoPolicyTrackAndAvailabilityCopyIsComplete(t *testing.T) {
	for _, value := range []string{"current", "required", "stale"} {
		if got := ContentPolicyStateLabel(value); got.Key != "content_policy."+value || got.EN == got.RU {
			t.Errorf("policy %q=%+v", value, got)
		}
	}
	for _, value := range []string{"clip", "queue", "replace", "unsupported"} {
		if got := TargetedTrackPolicyLabel(value); got.Key != "targeted_track."+value || got.EN == got.RU {
			t.Errorf("track %q=%+v", value, got)
		}
	}
	for _, value := range []string{"available", "dismissed", "replayed", "unavailable", "expired"} {
		if got := InboxAvailabilityLabel(value); got.Key != "inbox.availability."+value || got.EN == got.RU {
			t.Errorf("availability %q=%+v", value, got)
		}
	}
}

func TestPhaseTwoPresentationContractAndHandoffStayLinked(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	contractRaw, err := os.ReadFile(filepath.Join(root, "protocol", "pulsar-targets-inbox-presentation-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		ContractID    string          `json:"contract_id"`
		SurfaceStates []string        `json:"surface_states"`
		Commands      map[string]any  `json:"commands"`
		Playback      map[string]bool `json:"playback"`
	}
	if err := json.Unmarshal(contractRaw, &contract); err != nil {
		t.Fatal(err)
	}
	if contract.ContractID != "pulsar.targets-inbox-presentation.v1" ||
		strings.Join(contract.SurfaceStates, ",") != "loading,ready,stale,offline,coordinator_error" ||
		contract.Playback["late_inbox_autoplay_command_exists"] {
		t.Fatalf("contract=%+v", contract)
	}
	for _, command := range []string{
		"refresh", "select_targets", "set_include_origin", "load_more_inbox",
		"load_more_history", "load_more_receipts", "replay_inbox", "dismiss_inbox",
		"delete_history", "report_history", "mute_sender",
	} {
		if _, ok := contract.Commands[command]; !ok {
			t.Errorf("contract missing %s", command)
		}
	}
	handoffRaw, err := os.ReadFile(filepath.Join(root, "docs", "analysis", "p2-pulsar-targets-inbox-presentation-model.md"))
	if err != nil {
		t.Fatal(err)
	}
	handoff := string(handoffRaw)
	for _, required := range []string{
		"pulsar.targets-inbox-presentation.v1", "PulsarTargetsInboxModel.swift",
		"targets_inbox_model.go", "loading", "stale", "offline", "coordinator_error",
		"422 unsupported_targets", "410 cursor_expired", "no autoplay command",
		"TASK-260712-2nto40", "TASK-260712-cuplon",
	} {
		if !strings.Contains(handoff, required) {
			t.Errorf("handoff missing %q", required)
		}
	}
	protocolRaw, err := os.ReadFile(filepath.Join(root, "docs", "protocol.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(protocolRaw), "p2-pulsar-targets-inbox-presentation-model.md") {
		t.Fatal("protocol entry point lost presentation handoff")
	}
}
