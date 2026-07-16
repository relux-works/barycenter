package main

import (
	"context"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	protocol "relux.works/duet/pulsar-win/wire"
)

type windowsLiveNodeSender struct {
	mu         sync.Mutex
	handler    func(WindowsLiveCaptureEvent)
	phase      WindowsLiveCapturePhase
	generation uint64
	accepted   []uint64
	stops      []WindowsLiveCaptureStopReason
}

func (s *windowsLiveNodeSender) SetEventHandler(handler func(WindowsLiveCaptureEvent)) {
	s.mu.Lock()
	s.handler = handler
	s.mu.Unlock()
}

func (s *windowsLiveNodeSender) Snapshot() WindowsLiveCaptureSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return WindowsLiveCaptureSnapshot{Phase: s.phase, LocalGeneration: s.generation}
}

func (s *windowsLiveNodeSender) LocalHoldBegan(source WindowsLiveHoldSource, available bool, _ string) (uint64, bool) {
	s.mu.Lock()
	handler := s.handler
	if !available {
		s.mu.Unlock()
		if handler != nil {
			handler(WindowsLiveCaptureEvent{Kind: WindowsLiveCaptureFallbackEvent})
		}
		return 0, false
	}
	if s.phase != WindowsLiveCaptureIdle {
		s.mu.Unlock()
		return 0, false
	}
	s.generation++
	generation := s.generation
	s.phase = WindowsLiveCaptureAwaiting
	s.mu.Unlock()
	if handler != nil {
		handler(WindowsLiveCaptureEvent{Kind: WindowsLiveCapturePhaseEvent, Phase: WindowsLiveCaptureAwaiting})
		handler(WindowsLiveCaptureEvent{Kind: WindowsLiveCaptureRequestEvent, Source: source, Generation: generation})
	}
	return generation, true
}

func (s *windowsLiveNodeSender) LocalHoldHeartbeat(uint64) {}

func (s *windowsLiveNodeSender) AcceptStart(_ context.Context, _ protocol.LivePTTStartPayload, generation uint64, authorized bool) error {
	s.mu.Lock()
	if !authorized || generation != s.generation || s.phase != WindowsLiveCaptureAwaiting {
		s.mu.Unlock()
		return ErrWindowsLiveCaptureInvalidStart
	}
	s.accepted = append(s.accepted, generation)
	s.phase = WindowsLiveCaptureActive
	handler := s.handler
	s.mu.Unlock()
	if handler != nil {
		handler(WindowsLiveCaptureEvent{Kind: WindowsLiveCapturePhaseEvent, Phase: WindowsLiveCaptureActive})
	}
	return nil
}

func (s *windowsLiveNodeSender) LocalHoldEnded(generation uint64) {
	s.mu.Lock()
	valid := generation == s.generation
	s.mu.Unlock()
	if valid {
		s.terminate(WindowsLiveCaptureReleased)
	}
}
func (s *windowsLiveNodeSender) LocalStop()         { s.terminate(WindowsLiveCaptureLocalStop) }
func (s *windowsLiveNodeSender) HandleSessionLock() { s.terminate(WindowsLiveCaptureLock) }
func (s *windowsLiveNodeSender) HandleSuspend()     { s.terminate(WindowsLiveCaptureSleep) }
func (s *windowsLiveNodeSender) HandlePermissionRevoke() {
	s.terminate(WindowsLiveCapturePermissionLost)
}
func (s *windowsLiveNodeSender) HandleDeviceLoss()     { s.terminate(WindowsLiveCaptureDeviceLost) }
func (s *windowsLiveNodeSender) HandleDisconnect()     { s.terminate(WindowsLiveCaptureDisconnected) }
func (s *windowsLiveNodeSender) CoordinatorCancelled() { s.terminate(WindowsLiveCaptureCoordinator) }
func (s *windowsLiveNodeSender) Shutdown()             { s.terminate(WindowsLiveCaptureQuit) }

func (s *windowsLiveNodeSender) terminate(reason WindowsLiveCaptureStopReason) {
	s.mu.Lock()
	if s.phase == WindowsLiveCaptureIdle {
		s.mu.Unlock()
		return
	}
	s.phase = WindowsLiveCaptureIdle
	s.stops = append(s.stops, reason)
	handler := s.handler
	s.mu.Unlock()
	if handler != nil {
		handler(WindowsLiveCaptureEvent{Kind: WindowsLiveCaptureTerminalEvent, Reason: reason})
		handler(WindowsLiveCaptureEvent{Kind: WindowsLiveCapturePhaseEvent, Phase: WindowsLiveCaptureIdle})
	}
}

func (s *windowsLiveNodeSender) acceptedSnapshot() []uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]uint64(nil), s.accepted...)
}

type windowsLiveNodeReceiver struct {
	mu          sync.Mutex
	phase       WindowsLiveJitterPhase
	start       protocol.LivePTTStartPayload
	frames      []protocol.LivePTTBinaryFrame
	revocations int
}

func (r *windowsLiveNodeReceiver) Start(payload protocol.LivePTTStartPayload, authorized bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !authorized || r.phase != WindowsLiveIdle {
		return false
	}
	r.start, r.phase = payload, WindowsLiveBuffering
	return true
}

func (r *windowsLiveNodeReceiver) Receive(frame protocol.LivePTTBinaryFrame) protocol.LivePTTFrameDecision {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.phase == WindowsLiveIdle {
		return protocol.LivePTTFrameStale
	}
	session, _ := protocol.ParseLivePTTSessionID(r.start.SessionID)
	if frame.SessionID != session {
		return protocol.LivePTTFrameStale
	}
	r.frames = append(r.frames, frame)
	if len(r.frames) >= 3 {
		r.phase = WindowsLivePlaying
	}
	return protocol.LivePTTFrameApply
}

func (r *windowsLiveNodeReceiver) End(payload protocol.LivePTTEndPayload) {
	r.mu.Lock()
	if payload.SessionID == r.start.SessionID && payload.Generation == r.start.Generation {
		r.phase, r.start = WindowsLiveIdle, protocol.LivePTTStartPayload{}
	}
	r.mu.Unlock()
}

func (r *windowsLiveNodeReceiver) Cancel(payload protocol.LivePTTCancelPayload) {
	r.mu.Lock()
	if payload.SessionID == r.start.SessionID && payload.Generation == r.start.Generation {
		r.phase, r.start = WindowsLiveIdle, protocol.LivePTTStartPayload{}
	}
	r.mu.Unlock()
}

func (r *windowsLiveNodeReceiver) Revoke() {
	r.mu.Lock()
	r.revocations++
	r.phase, r.start = WindowsLiveIdle, protocol.LivePTTStartPayload{}
	r.mu.Unlock()
}

func (r *windowsLiveNodeReceiver) Snapshot() WindowsLiveJitterSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	return WindowsLiveJitterSnapshot{Phase: r.phase, SessionID: r.start.SessionID, Generation: r.start.Generation, ReceivedFrames: len(r.frames)}
}

func (r *windowsLiveNodeReceiver) revocationCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.revocations
}

type windowsLiveNodeControl struct {
	kind    string
	payload any
}

type windowsLiveNodeBox struct {
	mu       sync.Mutex
	controls []windowsLiveNodeControl
}

func (b *windowsLiveNodeBox) send(kind string, payload any) {
	b.mu.Lock()
	b.controls = append(b.controls, windowsLiveNodeControl{kind: kind, payload: payload})
	b.mu.Unlock()
}

func (b *windowsLiveNodeBox) snapshot() []windowsLiveNodeControl {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]windowsLiveNodeControl(nil), b.controls...)
}

func windowsLiveNodeStart(generation int64) protocol.LivePTTStartPayload {
	return protocol.LivePTTStartPayload{
		SessionID: "00112233445566778899aabbccddeeff", Generation: generation,
		SenderActorID: 11, SenderOrbitID: 12, SenderNodeID: "win-a",
		TargetSnapshot: "lts1.fixture", TargetSHA256: strings.Repeat("a", 64), TargetCount: 2,
		PlaybackDomain: "personal", PlaybackDomainID: 11,
		CodecProfile: protocol.LivePTTCodecProfile, FrameMS: protocol.LivePTTFrameMS,
		MaxPayloadBytes: protocol.LivePTTMaxPayloadBytes, JitterBufferMS: protocol.LivePTTJitterBufferMS,
		StartedAtCoordMS: 1_000, AcceptDeadlineCoordMS: 2_500,
		MaxDurationMS: protocol.LivePTTMaxDurationMS, MixedVersionPolicy: protocol.LivePTTMixedVersionRequireAll,
		LateJoinPolicy: protocol.LivePTTLateJoinPolicy, CaptureAuthority: protocol.LivePTTCaptureAuthority,
	}
}

func windowsLiveNodeFrame(sequence uint32) protocol.LivePTTBinaryFrame {
	session, _ := protocol.ParseLivePTTSessionID(windowsLiveNodeStart(7).SessionID)
	return protocol.LivePTTBinaryFrame{
		Flags: protocol.LivePTTFlagFEC, SessionID: session, Sequence: sequence,
		CaptureMonotonicUS: 2_000_000 + uint64(sequence-1)*20_000,
		Payload:            []byte{0xf8, 0xff, byte(sequence)},
	}
}

func makeWindowsLiveNode(enabled bool, decision WindowsLivePTTIncomingDecision) (*WindowsLivePTTNode, *windowsLiveNodeSender, *windowsLiveNodeReceiver, *windowsLiveNodeBox) {
	sender := &windowsLiveNodeSender{phase: WindowsLiveCaptureIdle}
	receiver := &windowsLiveNodeReceiver{phase: WindowsLiveIdle}
	box := &windowsLiveNodeBox{}
	node := NewWindowsLivePTTNode(
		sender, receiver, func() bool { return enabled },
		func(uint64, WindowsLiveHoldSource) (protocol.LivePTTStartPayload, bool) {
			return windowsLiveNodeStart(7), true
		},
		func(protocol.LivePTTStartPayload) WindowsLivePTTIncomingDecision { return decision },
		func() int64 { return 1_100 }, box.send,
	)
	return node, sender, receiver, box
}

func TestWindowsLivePTTNodeDisabledFallsBackAndRejectsIncoming(t *testing.T) {
	node, sender, receiver, box := makeWindowsLiveNode(false, WindowsLivePTTIncomingDecision{Allow: true})
	if _, ok := node.HoldBegan(WindowsLiveHoldShortcut, true, ""); ok {
		t.Fatal("disabled live hold did not fall back")
	}
	if status := node.Snapshot(); !status.FallbackToClip || status.Phase != WindowsLivePTTPhaseFallback {
		t.Fatalf("fallback status %+v", status)
	}
	start := windowsLiveNodeStart(7)
	node.Handle(&start)
	controls := box.snapshot()
	if len(controls) != 1 || controls[0].kind != protocol.TypeLivePTTReject {
		t.Fatalf("unsupported controls %+v", controls)
	}
	reject := controls[0].payload.(protocol.LivePTTRejectPayload)
	if reject.Code != "unsupported" || protocol.ValidateLivePTTRejectPayload(reject) != nil {
		t.Fatalf("invalid unsupported rejection %+v", reject)
	}
	if sender.Snapshot().Phase != WindowsLiveCaptureIdle || receiver.Snapshot().Phase != WindowsLiveIdle {
		t.Fatal("disabled node opened sender or receiver")
	}
}

func TestWindowsLivePTTNodeOutgoingStartsAfterMatchingAcceptAndRejectsConcurrentReceive(t *testing.T) {
	node, sender, receiver, box := makeWindowsLiveNode(true, WindowsLivePTTIncomingDecision{Allow: true})
	local, ok := node.HoldBegan(WindowsLiveHoldButton, true, "mic")
	if !ok || local == 0 {
		t.Fatal("hold was not accepted")
	}
	controls := box.snapshot()
	if len(controls) != 1 || controls[0].kind != protocol.TypeLivePTTStart || !reflect.DeepEqual(controls[0].payload, windowsLiveNodeStart(7)) {
		t.Fatalf("start controls %+v", controls)
	}
	stale := protocol.LivePTTAcceptPayload{SessionID: windowsLiveNodeStart(7).SessionID, Generation: 6, EventSequence: 1, AcceptedAtCoordMS: 1_101, LiveEdgeSequence: 1, BufferFrames: 3}
	node.Handle(&stale)
	if len(sender.acceptedSnapshot()) != 0 {
		t.Fatal("stale accept opened capture")
	}
	accept := stale
	accept.Generation = 7
	node.Handle(&accept)
	waitFor(t, 2*time.Second, func() bool { return sender.Snapshot().Phase == WindowsLiveCaptureActive }, "matching accept did not start capture")
	if got := sender.acceptedSnapshot(); !reflect.DeepEqual(got, []uint64{local}) {
		t.Fatalf("accepted generations %v", got)
	}
	concurrent := windowsLiveNodeStart(8)
	node.Handle(&concurrent)
	controls = box.snapshot()
	if len(controls) != 2 || controls[1].payload.(protocol.LivePTTRejectPayload).Code != "busy" {
		t.Fatalf("busy controls %+v", controls)
	}
	if receiver.Snapshot().Phase != WindowsLiveIdle {
		t.Fatal("concurrent receiver started")
	}
	node.HoldEnded(local)
	if node.Snapshot().Direction != WindowsLivePTTIdle {
		t.Fatal("release did not clear node status")
	}
}

func TestWindowsLivePTTNodeIncomingPolicyBinaryAndTerminalAreGenerationBound(t *testing.T) {
	denied, _, _, deniedBox := makeWindowsLiveNode(true, WindowsLivePTTIncomingDecision{Code: "dnd"})
	start := windowsLiveNodeStart(7)
	denied.Handle(&start)
	if got := deniedBox.snapshot()[0].payload.(protocol.LivePTTRejectPayload).Code; got != "dnd" {
		t.Fatalf("policy rejection %q", got)
	}

	node, _, receiver, _ := makeWindowsLiveNode(true, WindowsLivePTTIncomingDecision{Allow: true})
	node.Handle(&start)
	if receiver.Snapshot().Phase != WindowsLiveBuffering {
		t.Fatal("incoming start not buffered")
	}
	if _, ok := node.HoldBegan(WindowsLiveHoldButton, true, ""); ok {
		t.Fatal("send started during receive")
	}
	if status := node.Snapshot(); status.Direction != WindowsLivePTTReceiving ||
		status.Phase != WindowsLivePTTPhaseBuffering || status.LastError != "busy" {
		t.Fatalf("busy hold hid the active receiver: %+v", status)
	}
	for sequence := uint32(1); sequence <= 3; sequence++ {
		node.HandleFrame(windowsLiveNodeFrame(sequence))
	}
	if receiver.Snapshot().Phase != WindowsLivePlaying || node.Snapshot().Phase != WindowsLivePTTPhasePlaying {
		t.Fatalf("playing receiver=%+v node=%+v", receiver.Snapshot(), node.Snapshot())
	}
	end := protocol.LivePTTEndPayload{SessionID: start.SessionID, Generation: 7, CommandSequence: 1, LastSequence: 3, EndedAtCoordMS: 1_100, DrainDeadlineCoordMS: 1_700, Reason: "release"}
	node.Handle(&end)
	if receiver.Snapshot().Phase != WindowsLiveIdle || node.Snapshot().Direction != WindowsLivePTTIdle {
		t.Fatal("end did not clear receiving direction")
	}
}

func TestWindowsLivePTTNodeLifecycleAlwaysCleansBothDirections(t *testing.T) {
	actions := []func(*WindowsLivePTTNode){
		(*WindowsLivePTTNode).HandleSessionLock,
		(*WindowsLivePTTNode).HandleSuspend,
		(*WindowsLivePTTNode).HandlePermissionRevoke,
		(*WindowsLivePTTNode).HandleDeviceLoss,
		(*WindowsLivePTTNode).HandleDisconnect,
		(*WindowsLivePTTNode).RollbackFeature,
		(*WindowsLivePTTNode).Shutdown,
	}
	for index, action := range actions {
		node, sender, receiver, _ := makeWindowsLiveNode(true, WindowsLivePTTIncomingDecision{Allow: true})
		start := windowsLiveNodeStart(int64(index + 1))
		node.Handle(&start)
		if receiver.Snapshot().Phase != WindowsLiveBuffering {
			t.Fatalf("case %d did not start receiver", index)
		}
		action(node)
		if receiver.Snapshot().Phase != WindowsLiveIdle || sender.Snapshot().Phase != WindowsLiveCaptureIdle || node.Snapshot().Direction != WindowsLivePTTIdle {
			t.Fatalf("case %d leaked sender/receiver/node", index)
		}
		if receiver.revocationCount() == 0 {
			t.Fatalf("case %d did not revoke receiver", index)
		}
	}
}

func TestWindowsLivePTTNodeConcurrentLocalAndIncomingClaimsNeverOpenBothDirections(t *testing.T) {
	for iteration := 0; iteration < 100; iteration++ {
		node, sender, receiver, _ := makeWindowsLiveNode(true, WindowsLivePTTIncomingDecision{Allow: true})
		start := windowsLiveNodeStart(int64(iteration + 1))
		gate := make(chan struct{})
		var workers sync.WaitGroup
		workers.Add(2)
		go func() {
			defer workers.Done()
			<-gate
			_, _ = node.HoldBegan(WindowsLiveHoldButton, true, "mic")
		}()
		go func() {
			defer workers.Done()
			<-gate
			node.Handle(&start)
		}()
		close(gate)
		workers.Wait()
		sending := sender.Snapshot().Phase != WindowsLiveCaptureIdle
		receiving := receiver.Snapshot().Phase != WindowsLiveIdle
		if sending && receiving {
			t.Fatalf("iteration %d opened both directions", iteration)
		}
		node.Shutdown()
	}
}

func TestWindowsLivePTTProductionCompositionAndHoldStayFailClosed(t *testing.T) {
	mainSource, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(mainSource), "CapabilityLivePTT") || strings.Contains(string(mainSource), "NewWindowsLivePTTNode(") {
		t.Fatal("shipping main advertises or constructs production-dark live PTT")
	}
	uiSource, err := os.ReadFile("ui_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(uiSource), "SetWindowsHookEx") {
		t.Fatal("shipping shell added an unreviewed global keyboard hook")
	}
}
