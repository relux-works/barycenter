package session

import (
	"errors"
	"reflect"
	"strings"
	"testing"

	"relux.works/duet/coordinator/internal/protocol"
)

func streamFlowItem(id, policy string) StreamProgramItem {
	digest := strings.Repeat("a", 64)
	mediaID := "m_" + id
	return StreamProgramItem{
		StreamID: "sq_" + id, MediaID: mediaID,
		VariantManifest: "svm1." + id,
		VariantURL:      "/v1/media/" + mediaID + "/variants/sv_" + id,
		VariantETag:     `"sha256-` + digest + `"`, VariantSHA256: digest,
		VariantSizeBytes: 8 << 20, DurationMS: 120000,
		MixedVersionPolicy: policy,
	}
}

func streamFlowTargets() []StreamFlowTarget {
	return []StreamFlowTarget{
		{Node: protocol.NodeA, RTTMS: 20, SupportsStream: true},
		{Node: protocol.NodeB, RTTMS: 40, SupportsStream: true},
	}
}

func streamEffects[T any](effects []StreamFlowEffect) []T {
	var out []T
	for _, effect := range effects {
		if typed, ok := effect.(T); ok {
			out = append(out, typed)
		}
	}
	return out
}

func TestStreamMainProgramGenerationSafeLifecycleRebufferJoinAndDrain(t *testing.T) {
	flow := NewStreamMainProgram(MainProgramSource{Kind: "spotify", Ref: "spotify:track:parked"})
	item := streamFlowItem("lifecycle", protocol.StreamMixedVersionRequireAll)
	effects, err := flow.Start(item, 1, 0, 1000, streamFlowTargets())
	if err != nil || flow.State != StreamFlowLoading || len(streamEffects[EffStreamLoad](effects)) != 2 {
		t.Fatalf("start state=%s effects=%#v err=%v", flow.State, effects, err)
	}
	for index, node := range []protocol.NodeID{protocol.NodeA, protocol.NodeB} {
		effects, err = flow.OnReady(2000, node, protocol.StreamReadyPayload{
			StreamID: item.StreamID, PlaybackGeneration: 1, EventSequence: 1,
			AudiblePositionMS: 0, BufferedDurationMS: protocol.StreamMinimumBufferedMS,
		})
		if err != nil {
			t.Fatal(err)
		}
		if index == 0 && len(streamEffects[EffStreamResumeAt](effects)) != 0 {
			t.Fatal("first ready crossed the all-target barrier")
		}
	}
	resumes := streamEffects[EffStreamResumeAt](effects)
	if flow.State != StreamFlowArmed || len(resumes) != 2 ||
		resumes[0].Payload.TCoordMS != 2580 || resumes[1].Payload.TCoordMS != 2580 {
		t.Fatalf("ready barrier state=%s resumes=%+v", flow.State, resumes)
	}
	for index, node := range []protocol.NodeID{protocol.NodeA, protocol.NodeB} {
		effects, err = flow.OnStarted(node, protocol.StreamStartedPayload{
			StreamID: item.StreamID, PlaybackGeneration: 1, EventSequence: 2,
			AudiblePositionMS: 0, TFirstSampleCoordMS: 2580 + int64(index*4),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if flow.State != StreamFlowPlaying {
		t.Fatalf("started state=%s", flow.State)
	}
	for _, progress := range []struct {
		node     protocol.NodeID
		position int64
	}{{protocol.NodeA, 5000}, {protocol.NodeB, 4500}} {
		if _, err := flow.OnProgress(progress.node, protocol.StreamProgressPayload{
			StreamID: item.StreamID, PlaybackGeneration: 1, EventSequence: 3,
			AudiblePositionMS: progress.position, BufferedDurationMS: 2500,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if flow.AudiblePositionMS != 4500 {
		t.Fatalf("persisted decoded-ahead position=%d", flow.AudiblePositionMS)
	}
	seek, err := flow.SeekTo(9000, 3000)
	if err != nil || flow.SeekGeneration != 1 || flow.State != StreamFlowLoading ||
		len(streamEffects[EffStreamSeek](seek)) != 2 {
		t.Fatalf("seek state=%s generation=%d effects=%#v err=%v", flow.State, flow.SeekGeneration, seek, err)
	}
	stale, err := flow.OnProgress(protocol.NodeA, protocol.StreamProgressPayload{
		StreamID: item.StreamID, PlaybackGeneration: 1, SeekGeneration: 0,
		EventSequence: 4, AudiblePositionMS: 9100, BufferedDurationMS: 2000,
	})
	if err != nil || stale != nil || flow.AudiblePositionMS != 9000 {
		t.Fatalf("stale seek output effects=%#v position=%d err=%v", stale, flow.AudiblePositionMS, err)
	}
	for _, node := range []protocol.NodeID{protocol.NodeA, protocol.NodeB} {
		effects, err = flow.OnReady(3500, node, protocol.StreamReadyPayload{
			StreamID: item.StreamID, PlaybackGeneration: 1, SeekGeneration: 1,
			EventSequence: 1, AudiblePositionMS: 9000,
			BufferedDurationMS: protocol.StreamMinimumBufferedMS,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if flow.State != StreamFlowArmed || len(streamEffects[EffStreamResumeAt](effects)) != 2 {
		t.Fatalf("post-seek ready state=%s effects=%#v", flow.State, effects)
	}
	for _, node := range []protocol.NodeID{protocol.NodeA, protocol.NodeB} {
		if _, err := flow.OnStarted(node, protocol.StreamStartedPayload{
			StreamID: item.StreamID, PlaybackGeneration: 1, SeekGeneration: 1,
			EventSequence: 2, AudiblePositionMS: 9000, TFirstSampleCoordMS: 4080,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := flow.OnProgress(protocol.NodeB, protocol.StreamProgressPayload{
		StreamID: item.StreamID, PlaybackGeneration: 1, SeekGeneration: 1,
		EventSequence: 3, AudiblePositionMS: 9400, BufferedDurationMS: 2200,
	}); err != nil {
		t.Fatal(err)
	}
	rebuffer, err := flow.OnRebuffer(protocol.NodeA, protocol.StreamRebufferPayload{
		StreamID: item.StreamID, PlaybackGeneration: 1, SeekGeneration: 1,
		EventSequence: 3, AudiblePositionMS: 9500, BufferedDurationMS: 0,
	})
	if err != nil || flow.State != StreamFlowRebuffering || len(streamEffects[EffStreamPause](rebuffer)) != 2 {
		t.Fatalf("rebuffer state=%s effects=%#v err=%v", flow.State, rebuffer, err)
	}
	ready, err := flow.OnReady(5000, protocol.NodeA, protocol.StreamReadyPayload{
		StreamID: item.StreamID, PlaybackGeneration: 1, SeekGeneration: 1,
		EventSequence: 4, AudiblePositionMS: 9500,
		BufferedDurationMS: protocol.StreamMinimumBufferedMS,
	})
	if err != nil || flow.State != StreamFlowArmed || len(streamEffects[EffStreamResumeAt](ready)) != 2 {
		t.Fatalf("rebuffer ready state=%s effects=%#v err=%v", flow.State, ready, err)
	}
	if _, err := flow.OnStarted(protocol.NodeA, protocol.StreamStartedPayload{
		StreamID: item.StreamID, PlaybackGeneration: 1, SeekGeneration: 1,
		EventSequence: 5, AudiblePositionMS: 9500, TFirstSampleCoordMS: 5580,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := flow.OnStarted(protocol.NodeB, protocol.StreamStartedPayload{
		StreamID: item.StreamID, PlaybackGeneration: 1, SeekGeneration: 1,
		EventSequence: 4, AudiblePositionMS: 9500, TFirstSampleCoordMS: 5582,
	}); err != nil {
		t.Fatal(err)
	}
	for _, progress := range []struct {
		node     protocol.NodeID
		sequence int64
		position int64
	}{{protocol.NodeA, 6, 10000}, {protocol.NodeB, 5, 9900}} {
		if _, err := flow.OnProgress(progress.node, protocol.StreamProgressPayload{
			StreamID: item.StreamID, PlaybackGeneration: 1, SeekGeneration: 1,
			EventSequence: progress.sequence, AudiblePositionMS: progress.position,
			BufferedDurationMS: 2300,
		}); err != nil {
			t.Fatal(err)
		}
	}
	join, err := flow.JoinLivingAir(6000, StreamFlowTarget{
		Node: "c", RTTMS: 30, SupportsStream: true,
	})
	loads := streamEffects[EffStreamLoad](join)
	if err != nil || len(loads) != 1 || loads[0].Payload.StartPositionMS != 9900 {
		t.Fatalf("join effects=%#v err=%v", join, err)
	}
	joinReady, err := flow.OnReady(6100, "c", protocol.StreamReadyPayload{
		StreamID: item.StreamID, PlaybackGeneration: 1, EventSequence: 1,
		AudiblePositionMS: 9900, BufferedDurationMS: 2100,
	})
	if err != nil || len(streamEffects[EffStreamResumeAt](joinReady)) != 1 || flow.State != StreamFlowPlaying {
		t.Fatalf("join ready state=%s effects=%#v err=%v", flow.State, joinReady, err)
	}
	if _, err := flow.OnStarted("c", protocol.StreamStartedPayload{
		StreamID: item.StreamID, PlaybackGeneration: 1, EventSequence: 2,
		AudiblePositionMS: 9900, TFirstSampleCoordMS: 6660,
	}); err != nil {
		t.Fatal(err)
	}
	leave, err := flow.LeaveLivingAir("c")
	if err != nil || len(streamEffects[EffStreamCancel](leave)) != 1 || flow.State != StreamFlowPlaying {
		t.Fatalf("leave state=%s effects=%#v err=%v", flow.State, leave, err)
	}
	firstEnd, err := flow.OnEnded(protocol.NodeA, protocol.StreamEndedPayload{
		StreamID: item.StreamID, PlaybackGeneration: 1, SeekGeneration: 1,
		EventSequence: 7, AudiblePositionMS: item.DurationMS,
		TLastSampleCoordMS: 120000, Reason: "eof_drained",
	})
	if err != nil || len(streamEffects[EffStreamCompleted](firstEnd)) != 0 {
		t.Fatalf("first drained effects=%#v err=%v", firstEnd, err)
	}
	completed, err := flow.OnEnded(protocol.NodeB, protocol.StreamEndedPayload{
		StreamID: item.StreamID, PlaybackGeneration: 1, SeekGeneration: 1,
		EventSequence: 6, AudiblePositionMS: item.DurationMS,
		TLastSampleCoordMS: 120004, Reason: "eof_drained",
	})
	if err != nil || flow.State != StreamFlowIdle || len(streamEffects[EffStreamCompleted](completed)) != 1 ||
		len(streamEffects[EffRestoreMainProgram](completed)) != 1 {
		t.Fatalf("drained completion state=%s effects=%#v err=%v", flow.State, completed, err)
	}
}

func TestStreamMainProgramMixedVersionReplacePauseSeekAndRestart(t *testing.T) {
	base := MainProgramSource{Kind: "legacy_session", Ref: "orbit:42"}
	flow := NewStreamMainProgram(base)
	targets := streamFlowTargets()
	targets[1].SupportsStream = false
	requireAll := streamFlowItem("require_all", protocol.StreamMixedVersionRequireAll)
	if effects, err := flow.Start(requireAll, 1, 0, 1000, targets); !errors.Is(err, ErrStreamFlowConflict) || effects != nil || flow.State != StreamFlowIdle {
		t.Fatalf("require-all effects=%#v state=%s err=%v", effects, flow.State, err)
	}
	partial := streamFlowItem("partial", protocol.StreamMixedVersionSupportedOnlyWithReceipts)
	effects, err := flow.Start(partial, 1, 0, 1000, targets)
	if err != nil || len(streamEffects[EffStreamLoad](effects)) != 1 ||
		len(streamEffects[EffStreamReceipt](effects)) != 1 {
		t.Fatalf("partial effects=%#v err=%v", effects, err)
	}
	replacement := streamFlowItem("replacement", protocol.StreamMixedVersionRequireAll)
	bad := replacement
	bad.VariantETag = `"sha256-bad"`
	if _, err := flow.Replace(bad, 2, 0, 1500, streamFlowTargets()); !errors.Is(err, ErrStreamFlowInvalid) || flow.Current.StreamID != partial.StreamID {
		t.Fatalf("invalid replacement changed current=%+v err=%v", flow.Current, err)
	}
	effects, err = flow.Replace(replacement, 2, 0, 1500, streamFlowTargets())
	if err != nil || flow.PlaybackGeneration != 2 || len(streamEffects[EffStreamCancel](effects)) != 1 ||
		len(streamEffects[EffStreamCompleted](effects)) != 1 || len(streamEffects[EffStreamLoad](effects)) != 2 ||
		len(streamEffects[EffRestoreMainProgram](effects)) != 0 {
		t.Fatalf("replace generation=%d effects=%#v err=%v", flow.PlaybackGeneration, effects, err)
	}
	for _, node := range []protocol.NodeID{protocol.NodeA, protocol.NodeB} {
		if _, err := flow.OnReady(2000, node, protocol.StreamReadyPayload{
			StreamID: replacement.StreamID, PlaybackGeneration: 2, EventSequence: 1,
			BufferedDurationMS: 2000,
		}); err != nil {
			t.Fatal(err)
		}
	}
	for _, node := range []protocol.NodeID{protocol.NodeA, protocol.NodeB} {
		if _, err := flow.OnStarted(node, protocol.StreamStartedPayload{
			StreamID: replacement.StreamID, PlaybackGeneration: 2, EventSequence: 2,
			TFirstSampleCoordMS: 2580,
		}); err != nil {
			t.Fatal(err)
		}
	}
	pause, err := flow.Pause(100)
	if err != nil || flow.State != StreamFlowPaused || len(streamEffects[EffStreamPause](pause)) != 2 {
		t.Fatalf("pause state=%s effects=%#v err=%v", flow.State, pause, err)
	}
	seek, err := flow.SeekTo(30000, 3000)
	if err != nil || !flow.PausedAfterReady || len(streamEffects[EffStreamSeek](seek)) != 2 {
		t.Fatalf("paused seek state=%s effects=%#v err=%v", flow.State, seek, err)
	}
	for _, node := range []protocol.NodeID{protocol.NodeA, protocol.NodeB} {
		effects, err = flow.OnReady(3200, node, protocol.StreamReadyPayload{
			StreamID: replacement.StreamID, PlaybackGeneration: 2, SeekGeneration: 1,
			EventSequence: 1, AudiblePositionMS: 30000, BufferedDurationMS: 2000,
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	if flow.State != StreamFlowPaused || len(streamEffects[EffStreamResumeAt](effects)) != 0 {
		t.Fatalf("paused ready auto-resumed state=%s effects=%#v", flow.State, effects)
	}
	resume, err := flow.Resume(3400)
	if err != nil || flow.State != StreamFlowArmed || len(streamEffects[EffStreamResumeAt](resume)) != 2 {
		t.Fatalf("resume state=%s effects=%#v err=%v", flow.State, resume, err)
	}
	for _, node := range []protocol.NodeID{protocol.NodeA, protocol.NodeB} {
		if _, err := flow.OnStarted(node, protocol.StreamStartedPayload{
			StreamID: replacement.StreamID, PlaybackGeneration: 2, SeekGeneration: 1,
			EventSequence: 2, AudiblePositionMS: 30000, TFirstSampleCoordMS: 3980,
		}); err != nil {
			t.Fatal(err)
		}
	}
	snapshot := flow.Snapshot()
	restored := NewStreamMainProgram(MainProgramSource{})
	if err := restored.RestorePaused(snapshot); err != nil || restored.State != StreamFlowPaused {
		t.Fatalf("restore state=%s err=%v", restored.State, err)
	}
	restart, err := restored.Restart(3, 5000, streamFlowTargets())
	loads := streamEffects[EffStreamLoad](restart)
	if err != nil || restored.State != StreamFlowLoading || len(loads) != 2 ||
		loads[0].Payload.StartPositionMS != 30000 || loads[0].Payload.PlaybackGeneration != 3 {
		t.Fatalf("restart state=%s effects=%#v err=%v", restored.State, restart, err)
	}
}

func TestStreamMainProgramReadyTimeoutAndRuntimeFailurePolicy(t *testing.T) {
	flow := NewStreamMainProgram(MainProgramSource{Kind: "spotify", Ref: "spotify:track:base"})
	item := streamFlowItem("timeout", protocol.StreamMixedVersionSupportedOnlyWithReceipts)
	if _, err := flow.Start(item, 1, 0, 1000, streamFlowTargets()); err != nil {
		t.Fatal(err)
	}
	if _, err := flow.OnReady(1500, protocol.NodeA, protocol.StreamReadyPayload{
		StreamID: item.StreamID, PlaybackGeneration: 1, EventSequence: 1,
		BufferedDurationMS: protocol.StreamMinimumBufferedMS,
	}); err != nil {
		t.Fatal(err)
	}
	effects, err := flow.ReadyTimeout(6001)
	if err != nil || flow.State != StreamFlowArmed || len(streamEffects[EffStreamCancel](effects)) != 1 ||
		len(streamEffects[EffStreamReceipt](effects)) != 1 || len(streamEffects[EffStreamResumeAt](effects)) != 1 {
		t.Fatalf("supported-only timeout state=%s effects=%#v err=%v", flow.State, effects, err)
	}
	if _, err := flow.OnStarted(protocol.NodeA, protocol.StreamStartedPayload{
		StreamID: item.StreamID, PlaybackGeneration: 1, EventSequence: 2,
		TFirstSampleCoordMS: 6541,
	}); err != nil {
		t.Fatal(err)
	}
	failed, err := flow.OnFailed(7000, protocol.NodeA, protocol.StreamFailedPayload{
		StreamID: item.StreamID, PlaybackGeneration: 1, EventSequence: 3,
		Stage: "decoder", Code: "decoder_failed",
	})
	if err != nil || flow.State != StreamFlowIdle || len(streamEffects[EffStreamReceipt](failed)) != 1 ||
		len(streamEffects[EffStreamCompleted](failed)) != 1 || len(streamEffects[EffRestoreMainProgram](failed)) != 1 {
		t.Fatalf("last-target failure state=%s effects=%#v err=%v", flow.State, failed, err)
	}
	startTimeoutFlow := NewStreamMainProgram(MainProgramSource{})
	startTimeoutItem := streamFlowItem("start_timeout", protocol.StreamMixedVersionSupportedOnlyWithReceipts)
	if _, err := startTimeoutFlow.Start(startTimeoutItem, 1, 0, 1000, streamFlowTargets()); err != nil {
		t.Fatal(err)
	}
	for _, node := range []protocol.NodeID{protocol.NodeA, protocol.NodeB} {
		if _, err := startTimeoutFlow.OnReady(1500, node, protocol.StreamReadyPayload{
			StreamID: startTimeoutItem.StreamID, PlaybackGeneration: 1, EventSequence: 1,
			BufferedDurationMS: protocol.StreamMinimumBufferedMS,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := startTimeoutFlow.OnStarted(protocol.NodeA, protocol.StreamStartedPayload{
		StreamID: startTimeoutItem.StreamID, PlaybackGeneration: 1, EventSequence: 2,
		TFirstSampleCoordMS: 2040,
	}); err != nil {
		t.Fatal(err)
	}
	startTimedOut, err := startTimeoutFlow.StartTimeout(7041)
	if err != nil || startTimeoutFlow.State != StreamFlowPlaying ||
		len(streamEffects[EffStreamCancel](startTimedOut)) != 1 ||
		len(streamEffects[EffStreamReceipt](startTimedOut)) != 1 {
		t.Fatalf("start timeout state=%s effects=%#v err=%v", startTimeoutFlow.State, startTimedOut, err)
	}

	requireAll := NewStreamMainProgram(MainProgramSource{})
	strictItem := streamFlowItem("strict_timeout", protocol.StreamMixedVersionRequireAll)
	if _, err := requireAll.Start(strictItem, 1, 0, 1000, streamFlowTargets()); err != nil {
		t.Fatal(err)
	}
	strictEffects, err := requireAll.ReadyTimeout(6001)
	if err != nil || requireAll.State != StreamFlowIdle ||
		len(streamEffects[EffStreamCancel](strictEffects)) != 2 ||
		len(streamEffects[EffStreamCompleted](strictEffects)) != 1 {
		t.Fatalf("require-all timeout state=%s effects=%#v err=%v", requireAll.State, strictEffects, err)
	}
}

func TestStreamMainProgramTerminalEdgesDoNotStrandFlow(t *testing.T) {
	t.Run("empty air cannot resume", func(t *testing.T) {
		flow := NewStreamMainProgram(MainProgramSource{})
		item := streamFlowItem("empty_air", protocol.StreamMixedVersionRequireAll)
		target := StreamFlowTarget{Node: protocol.NodeA, RTTMS: 20, SupportsStream: true}
		if _, err := flow.Start(item, 1, 0, 1000, []StreamFlowTarget{target}); err != nil {
			t.Fatal(err)
		}
		if _, err := flow.LeaveLivingAir(protocol.NodeA); err != nil {
			t.Fatal(err)
		}
		if effects, err := flow.Resume(2000); !errors.Is(err, ErrStreamFlowConflict) || effects != nil || flow.State != StreamFlowPaused {
			t.Fatalf("empty resume state=%s effects=%#v err=%v", flow.State, effects, err)
		}
	})

	t.Run("drained participant cancels not-yet-audible join", func(t *testing.T) {
		flow := NewStreamMainProgram(MainProgramSource{})
		item := streamFlowItem("join_at_eof", protocol.StreamMixedVersionRequireAll)
		target := StreamFlowTarget{Node: protocol.NodeA, RTTMS: 20, SupportsStream: true}
		if _, err := flow.Start(item, 1, 0, 1000, []StreamFlowTarget{target}); err != nil {
			t.Fatal(err)
		}
		if _, err := flow.OnReady(1500, protocol.NodeA, protocol.StreamReadyPayload{
			StreamID: item.StreamID, PlaybackGeneration: 1, EventSequence: 1,
			BufferedDurationMS: protocol.StreamMinimumBufferedMS,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := flow.OnStarted(protocol.NodeA, protocol.StreamStartedPayload{
			StreamID: item.StreamID, PlaybackGeneration: 1, EventSequence: 2,
			TFirstSampleCoordMS: 2040,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := flow.JoinLivingAir(3000, StreamFlowTarget{Node: "c", RTTMS: 30, SupportsStream: true}); err != nil {
			t.Fatal(err)
		}
		if _, err := flow.OnReady(3200, "c", protocol.StreamReadyPayload{
			StreamID: item.StreamID, PlaybackGeneration: 1, EventSequence: 1,
			BufferedDurationMS: protocol.StreamMinimumBufferedMS,
		}); err != nil {
			t.Fatal(err)
		}
		effects, err := flow.OnEnded(protocol.NodeA, protocol.StreamEndedPayload{
			StreamID: item.StreamID, PlaybackGeneration: 1, EventSequence: 3,
			AudiblePositionMS: item.DurationMS, TLastSampleCoordMS: 120000,
			Reason: "eof_drained",
		})
		cancels := streamEffects[EffStreamCancel](effects)
		if err != nil || flow.State != StreamFlowIdle || len(cancels) != 1 ||
			cancels[0].To != "c" || cancels[0].Payload.Reason != "item_drained_before_join" ||
			len(streamEffects[EffStreamCompleted](effects)) != 1 {
			t.Fatalf("join drain state=%s effects=%#v err=%v", flow.State, effects, err)
		}
	})

	t.Run("failure after peer drain completes", func(t *testing.T) {
		flow := NewStreamMainProgram(MainProgramSource{})
		item := streamFlowItem("failure_after_drain", protocol.StreamMixedVersionSupportedOnlyWithReceipts)
		if _, err := flow.Start(item, 1, 0, 1000, streamFlowTargets()); err != nil {
			t.Fatal(err)
		}
		for _, node := range []protocol.NodeID{protocol.NodeA, protocol.NodeB} {
			if _, err := flow.OnReady(1500, node, protocol.StreamReadyPayload{
				StreamID: item.StreamID, PlaybackGeneration: 1, EventSequence: 1,
				BufferedDurationMS: protocol.StreamMinimumBufferedMS,
			}); err != nil {
				t.Fatal(err)
			}
		}
		for _, node := range []protocol.NodeID{protocol.NodeA, protocol.NodeB} {
			if _, err := flow.OnStarted(node, protocol.StreamStartedPayload{
				StreamID: item.StreamID, PlaybackGeneration: 1, EventSequence: 2,
				TFirstSampleCoordMS: 2040,
			}); err != nil {
				t.Fatal(err)
			}
		}
		if _, err := flow.OnEnded(protocol.NodeA, protocol.StreamEndedPayload{
			StreamID: item.StreamID, PlaybackGeneration: 1, EventSequence: 3,
			AudiblePositionMS: item.DurationMS, TLastSampleCoordMS: 120000,
			Reason: "eof_drained",
		}); err != nil {
			t.Fatal(err)
		}
		effects, err := flow.OnFailed(121000, protocol.NodeB, protocol.StreamFailedPayload{
			StreamID: item.StreamID, PlaybackGeneration: 1, EventSequence: 3,
			Stage: "output", Code: "device_lost",
		})
		if err != nil || flow.State != StreamFlowIdle || len(streamEffects[EffStreamReceipt](effects)) != 1 ||
			len(streamEffects[EffStreamCompleted](effects)) != 1 {
			t.Fatalf("failure after drain state=%s effects=%#v err=%v", flow.State, effects, err)
		}
	})
}

func TestStreamMainProgramLeavesLegacySpotifyAndClipSessionUntouched(t *testing.T) {
	legacy := New()
	legacy.SetPeers([]string{"a", "b"})
	legacy.SeedOnline(map[protocol.NodeID]bool{protocol.NodeA: true, protocol.NodeB: true})
	legacy.EnqueueTrack(Element{
		ID: "legacy-spotify", Kind: KindTrack, URI: "spotify:track:legacy",
		DurationMS: 180000,
	})
	legacy.EnqueueVoice(Element{
		ID: "legacy-overlay", Kind: KindVoice, MediaID: "m_legacy_clip",
		DurationMS: 4000, Target: "both", CreatedAt: 1,
	})
	before := legacy.Snapshot(80)

	flow := NewStreamMainProgram(MainProgramSource{
		Kind: "legacy_session", Ref: "orbit:legacy",
	})
	item := streamFlowItem("compatibility", protocol.StreamMixedVersionRequireAll)
	if _, err := flow.Start(item, 1, 0, 1000, streamFlowTargets()); err != nil {
		t.Fatal(err)
	}
	if _, err := flow.Cancel("compatibility_test"); err != nil {
		t.Fatal(err)
	}
	after := legacy.Snapshot(80)
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("stream flow mutated legacy session\nbefore=%+v\nafter=%+v", before, after)
	}
}
