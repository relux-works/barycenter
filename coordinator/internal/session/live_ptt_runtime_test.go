package session

import (
	"testing"

	"relux.works/duet/coordinator/internal/protocol"
)

func livePTTStartRequest(generation int64) LivePTTStart {
	payload := protocol.LivePTTStartPayload{
		SessionID: "00112233445566778899aabbccddeeff", Generation: generation,
		SenderActorID: 11, SenderOrbitID: 1, SenderNodeID: "a", TargetSnapshot: "lts1.opaque",
		TargetSHA256: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", TargetCount: 2,
		PlaybackDomain: "air", PlaybackDomainID: 44, CodecProfile: protocol.LivePTTCodecProfile,
		FrameMS: 20, MaxPayloadBytes: 400, JitterBufferMS: 60, StartedAtCoordMS: 1000,
		AcceptDeadlineCoordMS: 2500, MaxDurationMS: 300000,
		MixedVersionPolicy: protocol.LivePTTMixedVersionReceipts,
		LateJoinPolicy:     protocol.LivePTTLateJoinPolicy, CaptureAuthority: protocol.LivePTTCaptureAuthority,
	}
	return LivePTTStart{Sender: LivePTTNode{OrbitID: 1, Slot: "a"}, SenderActorID: 11,
		DomainKind: "air", DomainID: 44, Payload: payload, NowMS: 1000,
		Targets: []LivePTTTarget{{Node: LivePTTNode{OrbitID: 2, Slot: "a"}, ActorID: 21}, {Node: LivePTTNode{OrbitID: 3, Slot: "b"}, ActorID: 31}}}
}

func livePTTFrame(t *testing.T, sequence uint32, timestamp uint64, flags byte) (protocol.LivePTTBinaryFrame, []byte) {
	t.Helper()
	id, err := protocol.ParseLivePTTSessionID("00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	frame := protocol.LivePTTBinaryFrame{Flags: flags, SessionID: id, Sequence: sequence, CaptureMonotonicUS: timestamp, Payload: []byte{0xf8, 0xff, byte(sequence)}}
	raw, err := protocol.EncodeLivePTTBinaryFrame(frame)
	if err != nil {
		t.Fatal(err)
	}
	return frame, raw
}

func TestLivePTTRuntimeWinnerFanoutBackpressureAndNoAudioRetention(t *testing.T) {
	runtime := NewLivePTTRuntime()
	request := livePTTStartRequest(7)
	effects, err := runtime.Start(request)
	if err != nil || len(effects) != 3 {
		t.Fatalf("start effects=%d err=%v", len(effects), err)
	}
	if _, err := runtime.Start(request); err != ErrLivePTTBusy {
		t.Fatalf("concurrent start=%v", err)
	}
	targetA := request.Targets[0].Node
	accept := protocol.LivePTTAcceptPayload{SessionID: request.Payload.SessionID, Generation: 7, EventSequence: 1, AcceptedAtCoordMS: 1100, LiveEdgeSequence: 1, BufferFrames: 3}
	effects, err = runtime.Accept(targetA, accept)
	if err != nil || len(effects) != 2 || effects[1].Kind != LivePTTDuckStart {
		t.Fatalf("accept=%+v err=%v", effects, err)
	}
	frame, raw := livePTTFrame(t, 1, 1000000, protocol.LivePTTFlagStart|protocol.LivePTTFlagFEC)
	effects, err = runtime.RelayFrame(request.Sender, frame, raw, 1100)
	if err != nil || len(effects) != 1 || effects[0].To != targetA {
		t.Fatalf("relay=%+v err=%v", effects, err)
	}
	if duplicate, err := runtime.RelayFrame(request.Sender, frame, raw, 1100); err != nil || len(duplicate) != 0 {
		t.Fatalf("duplicate effects=%+v err=%v", duplicate, err)
	}
	effects = runtime.TargetUnavailable(targetA, request.Payload.SessionID, "backpressure", 1200)
	if len(effects) != 1 || runtime.Metrics().ActiveSessions != 1 {
		t.Fatalf("slow target affected the remaining target: %+v", effects)
	}
	targetB := request.Targets[1].Node
	if _, err := runtime.Accept(targetB, accept); err != nil {
		t.Fatal(err)
	}
	frame2, raw2 := livePTTFrame(t, 2, 1020000, protocol.LivePTTFlagFEC)
	if deliveries, err := runtime.RelayFrame(request.Sender, frame2, raw2, 1120); err != nil || len(deliveries) != 1 || deliveries[0].To != targetB {
		t.Fatalf("healthy peer relay=%+v err=%v", deliveries, err)
	}
	if terminal := runtime.TargetUnavailable(targetB, request.Payload.SessionID, "backpressure", 1300); len(terminal) < 2 {
		t.Fatalf("last target did not terminate: %+v", terminal)
	}
	metrics := runtime.Metrics()
	if metrics.ActiveSessions != 0 || metrics.RetainedAudioBytes != 0 || metrics.PersistedAudioBytes != 0 || metrics.FramesRelayedTotal != 2 || metrics.DuplicateFramesTotal != 1 || metrics.TargetBackpressureTotal != 2 {
		t.Fatalf("metrics=%+v", metrics)
	}
}

func TestLivePTTRuntimeRejectsUnauthorizedStaleAndPreAcceptMedia(t *testing.T) {
	runtime := NewLivePTTRuntime()
	request := livePTTStartRequest(7)
	if _, err := runtime.Start(request); err != nil {
		t.Fatal(err)
	}
	frame, raw := livePTTFrame(t, 1, 1000000, protocol.LivePTTFlagStart|protocol.LivePTTFlagFEC)
	if _, err := runtime.RelayFrame(request.Sender, frame, raw, 1050); err != ErrLivePTTNotReady {
		t.Fatalf("pre-accept=%v", err)
	}
	foreign := LivePTTNode{OrbitID: 99, Slot: "a"}
	accept := protocol.LivePTTAcceptPayload{SessionID: request.Payload.SessionID, Generation: 7, EventSequence: 1, AcceptedAtCoordMS: 1100, LiveEdgeSequence: 1, BufferFrames: 3}
	if _, err := runtime.Accept(foreign, accept); err != ErrLivePTTUnauthorized {
		t.Fatalf("foreign accept=%v", err)
	}
	if _, err := runtime.Accept(request.Targets[0].Node, accept); err != nil {
		t.Fatal(err)
	}
	if effects, err := runtime.RelayFrame(request.Sender, frame, raw, 1100); err != nil || len(effects) != 1 {
		t.Fatalf("pre-accept attempt consumed frame guard: effects=%+v err=%v", effects, err)
	}
	if effects := runtime.Disconnect(request.Sender, 1200); len(effects) == 0 {
		t.Fatal("sender disconnect did not cancel")
	}
	request.Payload.Generation = 7
	request.NowMS = 1300
	request.Payload.StartedAtCoordMS = 1300
	request.Payload.AcceptDeadlineCoordMS = 2800
	if _, err := runtime.Start(request); err != ErrLivePTTStale {
		t.Fatalf("generation replay=%v", err)
	}
}

func TestLivePTTRuntimeWatchdogAndRestartNeverResume(t *testing.T) {
	runtime := NewLivePTTRuntime()
	request := livePTTStartRequest(7)
	if _, err := runtime.Start(request); err != nil {
		t.Fatal(err)
	}
	if effects := runtime.Sweep(2500); len(effects) == 0 {
		t.Fatal("accept watchdog did not cancel")
	}
	request.Payload.SessionID = "ffeeddccbbaa99887766554433221100"
	request.Payload.Generation = 8
	request.NowMS = 3000
	request.Payload.StartedAtCoordMS = 3000
	request.Payload.AcceptDeadlineCoordMS = 4500
	if _, err := runtime.Start(request); err != nil {
		t.Fatal(err)
	}
	runtime.ResetForRestart()
	if runtime.Metrics().ActiveSessions != 0 {
		t.Fatal("restart retained live session")
	}
	frame, raw := livePTTFrame(t, 1, 1000000, protocol.LivePTTFlagStart|protocol.LivePTTFlagFEC)
	if _, err := runtime.RelayFrame(request.Sender, frame, raw, 5000); err != ErrLivePTTStale {
		t.Fatalf("old frame after restart=%v", err)
	}
}

func TestLivePTTRuntimeSenderDeliveryFailureCancelsFrozenTargets(t *testing.T) {
	runtime := NewLivePTTRuntime()
	request := livePTTStartRequest(7)
	if _, err := runtime.Start(request); err != nil {
		t.Fatal(err)
	}
	effects := runtime.DeliveryUnavailable(request.Sender, request.Payload.SessionID, 1100)
	if len(effects) != len(request.Targets) || runtime.Metrics().ActiveSessions != 0 {
		t.Fatalf("sender delivery failure effects=%+v metrics=%+v", effects, runtime.Metrics())
	}
	for _, effect := range effects {
		if effect.Type != protocol.TypeLivePTTCancel || effect.To == request.Sender {
			t.Fatalf("sender failure did not cancel target: %+v", effect)
		}
	}
}

func TestLivePTTRuntimeBoundsRelayRateWithoutConsumingFrame(t *testing.T) {
	runtime := NewLivePTTRuntime()
	request := livePTTStartRequest(7)
	if _, err := runtime.Start(request); err != nil {
		t.Fatal(err)
	}
	accept := protocol.LivePTTAcceptPayload{SessionID: request.Payload.SessionID, Generation: 7,
		EventSequence: 1, AcceptedAtCoordMS: 1050, LiveEdgeSequence: 1, BufferFrames: 3}
	if _, err := runtime.Accept(request.Targets[0].Node, accept); err != nil {
		t.Fatal(err)
	}
	for sequence := uint32(1); sequence <= livePTTRateBurstFrames; sequence++ {
		flags := byte(protocol.LivePTTFlagFEC)
		if sequence == 1 {
			flags |= protocol.LivePTTFlagStart
		}
		frame, raw := livePTTFrame(t, sequence, 1000000+uint64(sequence-1)*20000, flags)
		if _, err := runtime.RelayFrame(request.Sender, frame, raw, 1100); err != nil {
			t.Fatalf("burst frame %d: %v", sequence, err)
		}
	}
	frame, raw := livePTTFrame(t, livePTTRateBurstFrames+1,
		1000000+uint64(livePTTRateBurstFrames)*20000, protocol.LivePTTFlagFEC)
	if _, err := runtime.RelayFrame(request.Sender, frame, raw, 1100); err == nil {
		t.Fatal("unbounded burst frame was relayed")
	}
	if effects, err := runtime.RelayFrame(request.Sender, frame, raw, 1120); err != nil || len(effects) != 1 {
		t.Fatalf("rate rejection consumed frame guard: effects=%+v err=%v", effects, err)
	}
}
