package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func targetsInboxFixture(now time.Time) TargetsInboxSnapshot {
	refA := "trf_" + strings.Repeat("A", 43)
	refB := "trf_" + strings.Repeat("B", 43)
	ib := "ib_01J00000000000000000000000"
	hi := "hi_01J00000000000000000000000"
	label := TargetsInboxLocalizedLabel{Key: "action.replay", EN: "Replay", RU: "Повторить"}
	return TargetsInboxSnapshot{
		State: TargetsInboxReady, ActiveAirTitle: "Family", IncludeOrigin: true,
		AvailableAudiences: []TargetsInboxAudienceChoice{
			{Kind: PhaseOneThisPulsar}, {Kind: PhaseOneOwnBarycenter},
			{Kind: PhaseOneCurrentAir}, {Kind: TargetsInboxExplicitAudience},
		},
		SelectedAudience:    PhaseOneOwnBarycenter,
		TargetedTrackPolicy: "clip", ContentPolicyState: "current",
		Targets: []TargetsInboxTargetChoice{
			{Reference: refA, Kind: "barycenter", ExpiresAt: now.Add(time.Hour), Capabilities: []string{"overlay_mix_v1", "media_clip_v1", "media_clip_v1"}},
			{Reference: refB, Kind: "pulsar", ExpiresAt: now.Add(-time.Second)},
		},
		SelectedReferences: []string{refA, refB},
		Inbox:              []TargetsInboxInboxItem{{ID: ib, HistoryItemID: hi, Title: "Voice", ExpiresAt: now.Add(time.Hour), Actions: []TargetsInboxActionCapability{{Action: "replay", Label: label}, {Action: "dismiss", Label: label}, {Action: "report", Label: label}, {Action: "block_actor", Label: label}}}},
		InboxNextCursor:    "ic_" + strings.Repeat("a", 64),
		History:            []TargetsInboxHistoryItem{{ID: hi, Title: "Voice", Actions: []TargetsInboxActionCapability{{Action: "delete", Label: label}, {Action: "report", Label: label}, {Action: "block_actor", Label: label}}, ReceiptPage: TargetsInboxReceiptPage{NextCursor: "rc_" + strings.Repeat("b", 64)}}},
		HistoryNextCursor:  "hc_" + strings.Repeat("c", 64),
	}
}

func TestTargetsInboxModelPrunesExpiredCapabilitiesAndDoesNotAutoplay(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	model := NewTargetsInboxModel()
	model.Replace(targetsInboxFixture(now), now)
	snapshot := model.Snapshot()
	if len(snapshot.Targets) != 1 || len(snapshot.SelectedReferences) != 1 ||
		strings.Join(snapshot.Targets[0].Capabilities, ",") != "media_clip_v1,overlay_mix_v1" {
		t.Fatalf("normalized snapshot=%+v", snapshot)
	}
	for _, kind := range []TargetsInboxCommandKind{
		TargetsInboxRefresh, TargetsInboxSelectTargets, TargetsInboxSetIncludeOrigin,
		TargetsInboxSetAudience,
		TargetsInboxLoadMoreInbox, TargetsInboxLoadMoreHistory, TargetsInboxLoadMoreReceipts,
		TargetsInboxReplayInbox, TargetsInboxDismissInbox, TargetsInboxDeleteHistory,
		TargetsInboxReportInbox, TargetsInboxReportHistory, TargetsInboxMuteSender,
	} {
		if strings.Contains(string(kind), "autoplay") {
			t.Fatalf("late autoplay command exists: %s", kind)
		}
	}
}

func TestTargetsInboxCommandsRequireLatestServerCapabilities(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	model := NewTargetsInboxModel()
	fixture := targetsInboxFixture(now)
	model.Replace(fixture, now)
	ref := fixture.Targets[0].Reference
	ib := fixture.Inbox[0].ID
	hi := fixture.History[0].ID
	valid := []TargetsInboxCommand{
		{Kind: TargetsInboxRefresh},
		{Kind: TargetsInboxSetAudience, Audience: PhaseOneCurrentAir},
		{Kind: TargetsInboxSetAudience, Audience: TargetsInboxExplicitAudience},
		{Kind: TargetsInboxSelectTargets, References: []string{ref}},
		{Kind: TargetsInboxSetIncludeOrigin, Enabled: false},
		{Kind: TargetsInboxLoadMoreInbox, Cursor: fixture.InboxNextCursor},
		{Kind: TargetsInboxLoadMoreHistory, Cursor: fixture.HistoryNextCursor},
		{Kind: TargetsInboxLoadMoreReceipts, ObjectID: hi, Cursor: fixture.History[0].ReceiptPage.NextCursor},
		{Kind: TargetsInboxReplayInbox, ObjectID: ib, Delivery: PhaseOneOverlay},
		{Kind: TargetsInboxDismissInbox, ObjectID: ib},
		{Kind: TargetsInboxDeleteHistory, ObjectID: hi},
		{Kind: TargetsInboxReportHistory, ObjectID: hi, Reason: PhaseOneReportSpam},
		{Kind: TargetsInboxReportInbox, ObjectID: ib, Reason: PhaseOneReportSpam},
		{Kind: TargetsInboxMuteSender, ObjectID: hi},
		{Kind: TargetsInboxMuteSender, ObjectID: ib},
	}
	for _, request := range valid {
		if _, ok := model.BuildCommand(request); !ok {
			t.Errorf("valid command rejected: %s", request.Kind)
		}
	}
	invalid := []TargetsInboxCommand{
		{Kind: TargetsInboxSelectTargets, References: []string{"trf_" + strings.Repeat("Z", 43)}},
		{Kind: TargetsInboxReplayInbox, ObjectID: hi, Delivery: PhaseOneOverlay},
		{Kind: TargetsInboxDismissInbox, ObjectID: "ib_01J00000000000000000000001"},
		{Kind: TargetsInboxLoadMoreInbox, Cursor: "ic_" + strings.Repeat("f", 64)},
	}
	for _, request := range invalid {
		if _, ok := model.BuildCommand(request); ok {
			t.Errorf("manufactured command accepted: %s", request.Kind)
		}
	}
	fixture.State = TargetsInboxStale
	model.Replace(fixture, now)
	if _, ok := model.BuildCommand(TargetsInboxCommand{Kind: TargetsInboxDeleteHistory, ObjectID: hi}); ok {
		t.Fatal("stale model authorized mutation")
	}
	if _, ok := model.BuildCommand(TargetsInboxCommand{Kind: TargetsInboxRefresh}); !ok {
		t.Fatal("stale model blocked refresh")
	}
}

func TestTargetsInboxOpaqueValuesAreRedacted(t *testing.T) {
	secret := "trf_" + strings.Repeat("S", 43)
	choice := TargetsInboxTargetChoice{Reference: secret}
	command := TargetsInboxCommand{Kind: TargetsInboxSelectTargets, References: []string{secret}, Cursor: "ic_" + strings.Repeat("a", 64)}
	for _, rendered := range []string{choice.String(), choice.GoString(), command.String(), command.GoString()} {
		if strings.Contains(rendered, secret) || strings.Contains(rendered, "ic_") {
			t.Fatalf("opaque capability leaked: %s", rendered)
		}
	}
}

func TestTargetsInboxContractMatchesPortableEnums(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "protocol", "pulsar-targets-inbox-presentation-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		ContractID    string         `json:"contract_id"`
		SurfaceStates []string       `json:"surface_states"`
		Commands      map[string]any `json:"commands"`
		Playback      struct {
			LateAutoplay bool `json:"late_inbox_autoplay_command_exists"`
		} `json:"playback"`
	}
	if err := json.Unmarshal(raw, &contract); err != nil {
		t.Fatal(err)
	}
	wantStates := []string{"loading", "ready", "stale", "offline", "coordinator_error"}
	if strings.Join(contract.SurfaceStates, ",") != strings.Join(wantStates, ",") || contract.ContractID != "pulsar.targets-inbox-presentation.v1" {
		t.Fatalf("contract states=%v id=%s", contract.SurfaceStates, contract.ContractID)
	}
	for _, command := range []string{"refresh", "set_audience", "select_targets", "set_include_origin", "load_more_inbox", "load_more_history", "load_more_receipts", "replay_inbox", "dismiss_inbox", "delete_history", "report_inbox", "report_history", "mute_sender"} {
		if _, ok := contract.Commands[command]; !ok {
			t.Errorf("contract missing command %s", command)
		}
	}
	if contract.Playback.LateAutoplay {
		t.Fatal("contract permits late autoplay")
	}
}
