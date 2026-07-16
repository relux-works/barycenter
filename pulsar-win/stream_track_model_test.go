package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func streamTrackFixture(now time.Time, phase StreamTrackDraftPhase) StreamTrackSnapshot {
	localID := strings.Repeat("a", 32)
	mediaID := "med_opaque_server_value"
	manifest := "opaque manifest"
	confirmed := phase == StreamTrackDraftReady
	if !confirmed {
		manifest = ""
	}
	return StreamTrackSnapshot{
		State: TargetsInboxReady,
		Draft: &StreamTrackDraft{
			LocalID: localID, LocalByteCount: 500000000, RetainedLocalBytes: true,
			Title: "Long track", ClientMIME: "audio/mpeg", DurationMS: 7000000,
			HasDuration: true, Phase: phase,
			PhaseLabel: TargetsInboxLocalizedLabel{Key: "stream_track.draft." + string(phase), EN: string(phase), RU: string(phase)},
			MediaID:    mediaID, VariantManifest: manifest,
			ServerMetadataConfirmed: confirmed, UploadOffset: 250000000, ProcessingPercent: 100,
		},
		Playback: StreamTrackPlayback{
			Phase: StreamTrackPlaybackPlaying, StreamID: "str_opaque_server_value",
			PhaseLabel: TargetsInboxLocalizedLabel{Key: "stream_track.playback.playing", EN: "Playing", RU: "Воспроизводится"},
			DurationMS: 10000, AudiblePositionMS: 1000,
			PlaybackGeneration: 7, SeekGeneration: 3,
		},
		Targets: []TargetsInboxTargetChoice{{
			Reference: "trf_" + strings.Repeat("A", 43), Kind: "pulsar",
			ExpiresAt: now.Add(time.Hour), Capabilities: []string{"stream_track", "stream_track"},
		}},
		SelectedReferences: []string{"trf_" + strings.Repeat("A", 43)},
		SelectedAudience:   StreamTrackExplicit, SelectedInsertion: StreamTrackQueue,
		ActiveAirAvailable: true, ContentPolicyState: "current",
		Actions: streamTrackAllActions(),
	}
}

func streamTrackAllActions() []TargetsInboxActionCapability {
	result := make([]TargetsInboxActionCapability, 0, 10)
	for _, action := range []string{"accept_policy", "upload", "retry", "delete", "queue", "replace", "pause", "seek", "resume", "report"} {
		result = append(result, TargetsInboxActionCapability{
			Action: action,
			Label:  TargetsInboxLocalizedLabel{Key: "stream_track.action." + action, EN: action, RU: action},
		})
	}
	return result
}

func TestStreamTrackDraftSurvivesOfflineMissingProjection(t *testing.T) {
	now := time.Unix(1800000000, 0)
	model := NewStreamTrackModel()
	fixture := streamTrackFixture(now, StreamTrackDraftUploading)
	model.Replace(fixture, now)
	outage := streamTrackFixture(now, StreamTrackDraftReady)
	outage.State = TargetsInboxOffline
	outage.Draft = nil
	model.Replace(outage, now)
	got := model.Snapshot()
	if got.Draft == nil || got.Draft.LocalID != strings.Repeat("a", 32) || !got.Draft.RetainedLocalBytes {
		t.Fatalf("retained draft lost: %+v", got.Draft)
	}
	if len(got.Actions) != 0 {
		t.Fatalf("offline capabilities retained: %+v", got.Actions)
	}
	if _, ok := model.BuildCommand(StreamTrackCommand{Kind: StreamTrackUpload, LocalID: got.Draft.LocalID}); ok {
		t.Fatal("offline projection authorized upload")
	}
}

func TestStreamTrackReadinessIsServerOwned(t *testing.T) {
	now := time.Unix(1800000000, 0)
	fixture := streamTrackFixture(now, StreamTrackDraftReady)
	fixture.Draft.ServerMetadataConfirmed = false
	fixture.Draft.VariantManifest = ""
	fixture.Draft.UploadOffset = fixture.Draft.LocalByteCount + 1
	fixture.Draft.ProcessingPercent = 150
	model := NewStreamTrackModel()
	model.Replace(fixture, now)
	got := model.Snapshot().Draft
	if got == nil || got.Phase != StreamTrackDraftProcessing || got.UploadOffset != got.LocalByteCount || got.ProcessingPercent != 100 {
		t.Fatalf("client metadata manufactured readiness: %+v", got)
	}
	if _, ok := model.BuildCommand(StreamTrackCommand{
		Kind: StreamTrackQueueCommand, MediaID: fixture.Draft.MediaID,
		Audience: StreamTrackExplicit, Targets: fixture.SelectedReferences,
	}); ok {
		t.Fatal("unconfirmed draft queued")
	}
}

func TestStreamTrackCommandsRequireFreshCapabilitiesAndExactGeneration(t *testing.T) {
	now := time.Unix(1800000000, 0)
	fixture := streamTrackFixture(now, StreamTrackDraftReady)
	model := NewStreamTrackModel()
	model.Replace(fixture, now)
	valid := []StreamTrackCommand{
		{Kind: StreamTrackQueueCommand, MediaID: fixture.Draft.MediaID, Audience: StreamTrackExplicit, Targets: fixture.SelectedReferences},
		{Kind: StreamTrackPause, StreamID: fixture.Playback.StreamID, PlaybackGeneration: 7},
		{Kind: StreamTrackSeek, StreamID: fixture.Playback.StreamID, PositionMS: 5000, PlaybackGeneration: 7, SeekGeneration: 3},
		{Kind: StreamTrackDelete, LocalID: fixture.Draft.LocalID, Confirmed: true},
		{Kind: StreamTrackReport, MediaID: fixture.Draft.MediaID, Details: ""},
	}
	for _, request := range valid {
		if _, ok := model.BuildCommand(request); !ok {
			t.Errorf("valid command rejected: %s", request.Kind)
		}
	}
	if !model.SelectAudience(StreamTrackCurrentAir) || !model.SelectInsertion(StreamTrackReplace) {
		t.Fatal("current Air replace selection rejected")
	}
	if _, ok := model.BuildCommand(StreamTrackCommand{
		Kind: StreamTrackReplaceCommand, MediaID: fixture.Draft.MediaID, Audience: StreamTrackCurrentAir,
	}); !ok {
		t.Fatal("selected current Air replace rejected")
	}
	if !model.SelectTargets(fixture.SelectedReferences) || !model.SelectAudience(StreamTrackExplicit) || !model.SelectInsertion(StreamTrackQueue) {
		t.Fatal("explicit queue selection could not be restored")
	}
	invalid := []StreamTrackCommand{
		{Kind: StreamTrackQueueCommand, MediaID: fixture.Draft.MediaID, Audience: StreamTrackExplicit},
		{Kind: StreamTrackPause, StreamID: fixture.Playback.StreamID, PlaybackGeneration: 6},
		{Kind: StreamTrackSeek, StreamID: fixture.Playback.StreamID, PositionMS: 5000, PlaybackGeneration: 7, SeekGeneration: 2},
		{Kind: StreamTrackDelete, LocalID: fixture.Draft.LocalID, Confirmed: false},
	}
	for _, request := range invalid {
		if _, ok := model.BuildCommand(request); ok {
			t.Errorf("invalid command accepted: %s", request.Kind)
		}
	}
}

func TestStreamTrackGenerationFencingAndOptimisticSeek(t *testing.T) {
	now := time.Unix(1800000000, 0)
	fixture := streamTrackFixture(now, StreamTrackDraftReady)
	fixture.Playback.AudiblePositionMS = 5000
	model := NewStreamTrackModel()
	model.Replace(fixture, now)

	stale := fixture
	stale.Playback.PlaybackGeneration = 6
	stale.Playback.AudiblePositionMS = 9000
	model.Replace(stale, now)
	if got := model.Snapshot().Playback; got.PlaybackGeneration != 7 || got.AudiblePositionMS != 5000 {
		t.Fatalf("stale playback replaced current: %+v", got)
	}

	backward := fixture
	backward.Playback.AudiblePositionMS = 1000
	model.Replace(backward, now)
	if got := model.Snapshot().Playback.AudiblePositionMS; got != 5000 {
		t.Fatalf("same-generation progress moved backward: %d", got)
	}

	seeked := fixture
	seeked.Playback.SeekGeneration = 4
	seeked.Playback.AudiblePositionMS = 1000
	model.Replace(seeked, now)
	request := StreamTrackCommand{
		Kind: StreamTrackSeek, StreamID: seeked.Playback.StreamID, PositionMS: 2000,
		PlaybackGeneration: 7, SeekGeneration: 4,
	}
	if !model.ApplyOptimistic(request) {
		t.Fatal("valid optimistic seek rejected")
	}
	got := model.Snapshot().Playback
	if got.Phase != StreamTrackPlaybackSeeking || got.SeekGeneration != 5 || got.AudiblePositionMS != 2000 {
		t.Fatalf("optimistic seek=%+v", got)
	}
}

func TestStreamTrackDeleteWaitsForServerAndOpaqueValuesAreRedacted(t *testing.T) {
	now := time.Unix(1800000000, 0)
	fixture := streamTrackFixture(now, StreamTrackDraftReady)
	model := NewStreamTrackModel()
	model.Replace(fixture, now)
	request := StreamTrackCommand{Kind: StreamTrackDelete, LocalID: fixture.Draft.LocalID, Confirmed: true}
	if !model.ApplyOptimistic(request) || model.Snapshot().Draft == nil {
		t.Fatal("delete removed draft before server confirmation")
	}
	confirmed := fixture
	confirmed.Draft = nil
	confirmed.ConfirmedDeletedLocalID = fixture.Draft.LocalID
	model.Replace(confirmed, now)
	if model.Snapshot().Draft != nil {
		t.Fatal("exact server delete confirmation did not remove draft")
	}
	for _, rendered := range []string{request.String(), request.GoString(), fixture.Draft.String(), fixture.Playback.String(), fixture.String()} {
		for _, secret := range []string{fixture.Draft.LocalID, fixture.Draft.MediaID, fixture.Playback.StreamID, fixture.Targets[0].Reference} {
			if strings.Contains(rendered, secret) {
				t.Fatalf("opaque value leaked in %q", rendered)
			}
		}
	}
}

func TestStreamTrackPortableContractMatchesGoEnums(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "protocol", "pulsar-stream-track-ui-model-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		ContractID     string   `json:"contract_id"`
		DraftPhases    []string `json:"draft_phases"`
		PlaybackPhases []string `json:"playback_phases"`
		Audiences      []string `json:"audiences"`
		Insertions     []string `json:"insertions"`
		Actions        []string `json:"actions"`
		FailureCodes   []string `json:"failure_codes"`
	}
	if err := json.Unmarshal(raw, &contract); err != nil {
		t.Fatal(err)
	}
	wantDraft := []string{"retained", "uploading", "uploaded", "processing", "ready", "failed"}
	wantPlayback := []string{"idle", "queued", "loading", "ready", "playing", "paused", "seeking", "rebuffering", "ended", "failed"}
	wantAudience := []string{"current_air", "explicit"}
	wantInsertion := []string{"queue", "replace"}
	wantActions := []string{"accept_policy", "upload", "retry", "delete", "queue", "replace", "pause", "seek", "resume", "report"}
	wantFailures := []string{"offline", "quota_exceeded", "unsupported_targets", "policy_required", "processing_failed", "variant_unavailable", "stale_generation", "service_unavailable"}
	if contract.ContractID != "pulsar.stream-track-ui-model.v1" ||
		!reflect.DeepEqual(contract.DraftPhases, wantDraft) || !reflect.DeepEqual(contract.PlaybackPhases, wantPlayback) ||
		!reflect.DeepEqual(contract.Audiences, wantAudience) || !reflect.DeepEqual(contract.Insertions, wantInsertion) ||
		!reflect.DeepEqual(contract.Actions, wantActions) || !reflect.DeepEqual(contract.FailureCodes, wantFailures) {
		t.Fatalf("portable contract diverged: %+v", contract)
	}
}
