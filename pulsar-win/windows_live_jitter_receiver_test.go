package main

import (
	"math"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	protocol "relux.works/duet/pulsar-win/wire"
)

type fakeWindowsLiveDecoder struct {
	fec        bool
	mu         sync.Mutex
	decodes    int
	fecDecodes int
}

func (d *fakeWindowsLiveDecoder) Decode(packet []byte, fec bool, output []float32) (int, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if len(packet) == 0 || len(output) < liveFrameInput || packet[0] == 0xff {
		return 0, ErrWindowsLiveInvalidPacket
	}
	if fec && !d.fec {
		return 0, ErrWindowsLiveFECUnavailable
	}
	d.decodes++
	if fec {
		d.fecDecodes++
	}
	value := float32(packet[0]) / 255
	for index := range output[:liveFrameInput] {
		output[index] = value
	}
	return liveFrameInput, nil
}

func (d *fakeWindowsLiveDecoder) Reset() {}

type windowsLiveEvent struct {
	kind    string
	payload any
}

type windowsLiveFixture struct {
	clock       int64
	music       *Ring
	engine      *Engine
	decoder     *fakeWindowsLiveDecoder
	receiver    *WindowsLiveJitterReceiver
	eventMu     sync.Mutex
	events      []windowsLiveEvent
	eventSignal chan struct{}
}

func newWindowsLiveFixture(t *testing.T, fec bool) *windowsLiveFixture {
	t.Helper()
	f := &windowsLiveFixture{
		clock: 1_001, music: NewRing(sampleRate * channels * 4),
		decoder:     &fakeWindowsLiveDecoder{fec: fec},
		eventSignal: make(chan struct{}, 1),
	}
	f.engine, _ = newTestEngine(f.music)
	t.Cleanup(f.engine.Close)
	f.receiver = NewWindowsLiveJitterReceiver(
		f.engine, f.decoder, false, func() int64 { return f.clock },
		func(kind string, payload any) { f.recordEvent(windowsLiveEvent{kind, payload}) },
	)
	if f.receiver == nil {
		t.Fatal("receiver constructor failed")
	}
	t.Cleanup(func() { validateWindowsLiveEvents(t, f.eventSnapshot()) })
	return f
}

func windowsLiveStart(generation int64) protocol.LivePTTStartPayload {
	return protocol.LivePTTStartPayload{
		SessionID: "00112233445566778899aabbccddeeff", Generation: generation,
		SenderActorID: 1, SenderOrbitID: 2, SenderNodeID: "mac",
		TargetSnapshot: "lts1.sender", TargetSHA256: strings.Repeat("a", 64),
		TargetCount: 1, PlaybackDomain: "personal", PlaybackDomainID: 1,
		CodecProfile: protocol.LivePTTCodecProfile, FrameMS: protocol.LivePTTFrameMS,
		MaxPayloadBytes:  protocol.LivePTTMaxPayloadBytes,
		JitterBufferMS:   protocol.LivePTTJitterBufferMS,
		StartedAtCoordMS: 1_000, AcceptDeadlineCoordMS: 2_000,
		MaxDurationMS:      protocol.LivePTTMaxDurationMS,
		MixedVersionPolicy: protocol.LivePTTMixedVersionRequireAll,
		LateJoinPolicy:     protocol.LivePTTLateJoinPolicy,
		CaptureAuthority:   protocol.LivePTTCaptureAuthority,
	}
}

func windowsLiveFrame(t *testing.T, sequence uint32, value byte) protocol.LivePTTBinaryFrame {
	t.Helper()
	session, err := protocol.ParseLivePTTSessionID("00112233445566778899aabbccddeeff")
	if err != nil {
		t.Fatal(err)
	}
	flags := byte(protocol.LivePTTFlagFEC)
	if sequence == 1 {
		flags |= protocol.LivePTTFlagStart
	}
	return protocol.LivePTTBinaryFrame{
		Flags: flags, SessionID: session, Sequence: sequence,
		CaptureMonotonicUS: 2_000_000 + uint64(sequence-1)*20_000,
		Payload:            []byte{value, 0x5a},
	}
}

func TestWindowsLiveReceiverValidatesAuthorizationGenerationAndReorder(t *testing.T) {
	f := newWindowsLiveFixture(t, true)
	if f.receiver.Start(windowsLiveStart(1), false) {
		t.Fatal("unauthorized start accepted")
	}
	if !f.receiver.Start(windowsLiveStart(2), true) {
		t.Fatal("valid start rejected")
	}
	if f.receiver.Start(windowsLiveStart(3), true) {
		t.Fatal("second session accepted")
	}
	if got := f.receiver.Receive(windowsLiveFrame(t, 2, 50)); got != protocol.LivePTTFrameInvalid {
		t.Fatalf("frame before sequence one: %s", got)
	}

	first := windowsLiveFrame(t, 1, 40)
	malformed := first
	malformed.Flags = protocol.LivePTTFlagStart
	if got := f.receiver.Receive(malformed); got != protocol.LivePTTFrameInvalid {
		t.Fatalf("missing FEC flag: %s", got)
	}
	if got := f.receiver.Receive(first); got != protocol.LivePTTFrameApply {
		t.Fatalf("first: %s", got)
	}
	if got := f.receiver.Receive(first); got != protocol.LivePTTFrameDuplicate {
		t.Fatalf("duplicate: %s", got)
	}
	conflict := first
	conflict.Payload = []byte{41}
	if got := f.receiver.Receive(conflict); got != protocol.LivePTTFrameStale {
		t.Fatalf("conflict: %s", got)
	}
	if got := f.receiver.Receive(windowsLiveFrame(t, 10, 60)); got != protocol.LivePTTFrameInvalid {
		t.Fatalf("out-of-window frame: %s", got)
	}
	if got := f.receiver.Receive(windowsLiveFrame(t, 3, 60)); got != protocol.LivePTTFrameApply {
		t.Fatalf("reordered third: %s", got)
	}
	snapshot := f.receiver.Snapshot()
	if snapshot.Phase != WindowsLivePlaying || snapshot.ExpectedSequence != 4 ||
		snapshot.FECFrames != 1 || snapshot.EncodedFrames != 0 || snapshot.PCMFrames != 2_880 {
		t.Fatalf("unexpected prebuffer snapshot: %+v", snapshot)
	}
	f.engine.Render(make([]float32, liveFrameOutput*channels))
	f.waitForEvents(t, 4)
	if got := f.receiver.Receive(windowsLiveFrame(t, 2, 50)); got != protocol.LivePTTFrameStale {
		t.Fatalf("late frame: %s", got)
	}
	events := f.eventSnapshot()
	if len(events) < 4 || events[0].kind != protocol.TypeLivePTTReject ||
		events[1].kind != protocol.TypeLivePTTAccept || events[len(events)-1].kind != protocol.TypeLivePTTReceipt {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestWindowsLiveTwoPercentLossStaysBoundedAndMixerRecovers(t *testing.T) {
	f := newWindowsLiveFixture(t, true)
	fillConstantRing(t, f.music, sampleRate*3, 0.8)
	if !f.receiver.Start(windowsLiveStart(1), true) {
		t.Fatal("start rejected")
	}
	for sequence := uint32(1); sequence <= 3; sequence++ {
		f.receiver.Receive(windowsLiveFrame(t, sequence, byte(80+sequence)))
	}
	dst := make([]float32, liveFrameOutput*channels)
	maximum := float32(0)
	missing := map[uint32]bool{25: true, 75: true}
	for sequence := uint32(4); sequence <= 100; sequence++ {
		if !missing[sequence] {
			f.receiver.Receive(windowsLiveFrame(t, sequence, byte(80+sequence%40)))
		}
		if sequence >= 6 {
			f.receiver.Tick()
			f.engine.Render(dst)
			for _, sample := range dst {
				if float32(math.Abs(float64(sample))) > maximum {
					maximum = float32(math.Abs(float64(sample)))
				}
			}
		}
	}
	for f.receiver.Snapshot().ExpectedSequence <= 100 {
		f.receiver.Tick()
		f.engine.Render(dst)
	}
	snapshot := f.receiver.Snapshot()
	if snapshot.DecodedFrames != 100 || snapshot.FECFrames != 2 || snapshot.PLCFrames != 0 ||
		snapshot.EncodedFrames > windowsLivePacketWindow || snapshot.EncodedBytes > windowsLivePacketWindow*protocol.LivePTTMaxPayloadBytes {
		t.Fatalf("loss snapshot: %+v", snapshot)
	}
	if maximum > dbAmplitude(liveLimiterDB)+0.0001 {
		t.Fatalf("post-mix ceiling exceeded: %f", maximum)
	}

	f.receiver.End(protocol.LivePTTEndPayload{
		SessionID: windowsLiveStart(1).SessionID, Generation: 1, CommandSequence: 1,
		LastSequence: 100, EndedAtCoordMS: 2_000,
		DrainDeadlineCoordMS: 2_000 + protocol.LivePTTDrainTimeoutMS, Reason: "release",
	})
	for range 4 {
		f.engine.Render(dst)
	}
	f.receiver.Tick()
	for range 10 {
		f.engine.Render(dst)
	}
	if f.receiver.Snapshot().Phase != WindowsLiveIdle || f.engine.LiveRenderActive() {
		t.Fatalf("terminal state retained: snapshot=%+v active=%v", f.receiver.Snapshot(), f.engine.LiveRenderActive())
	}
	for _, sample := range dst {
		if math.Abs(float64(sample-0.8)) > 0.002 {
			t.Fatalf("music did not recover: %f", sample)
		}
	}
}

func TestWindowsLivePLCFallbackAndOverflowFailClosed(t *testing.T) {
	plc := newWindowsLiveFixture(t, false)
	plc.receiver.Start(windowsLiveStart(1), true)
	plc.receiver.Receive(windowsLiveFrame(t, 1, 80))
	plc.receiver.Receive(windowsLiveFrame(t, 3, 90))
	if snapshot := plc.receiver.Snapshot(); snapshot.PLCFrames != 1 || snapshot.Phase != WindowsLivePlaying {
		t.Fatalf("PLC fallback missing: %+v", snapshot)
	}

	overflow := newWindowsLiveFixture(t, true)
	overflow.receiver.Start(windowsLiveStart(1), true)
	for sequence := uint32(1); sequence <= 24 && overflow.receiver.Snapshot().Phase != WindowsLiveIdle; sequence++ {
		overflow.receiver.Receive(windowsLiveFrame(t, sequence, 100))
		if sequence > 3 {
			overflow.receiver.Tick()
		}
	}
	if overflow.receiver.Snapshot().Phase != WindowsLiveIdle || !hasWindowsLiveEvent(overflow.eventSnapshot(), protocol.TypeLivePTTFailed) {
		t.Fatalf("overflow did not fail closed: snapshot=%+v events=%+v", overflow.receiver.Snapshot(), overflow.eventSnapshot())
	}
}

func TestWindowsLiveCancelRejectsOldGenerationAndReleasesDuck(t *testing.T) {
	f := newWindowsLiveFixture(t, true)
	fillConstantRing(t, f.music, sampleRate, 0.4)
	f.receiver.Start(windowsLiveStart(1), true)
	for sequence := uint32(1); sequence <= 3; sequence++ {
		f.receiver.Receive(windowsLiveFrame(t, sequence, 90))
	}
	f.receiver.Cancel(protocol.LivePTTCancelPayload{
		SessionID: windowsLiveStart(1).SessionID, Generation: 1, CommandSequence: 1,
		CancelledAtCoordMS: 1_100, Reason: "user_cancel", DiscardBuffered: true,
	})
	dst := make([]float32, liveFrameOutput*channels)
	for range 10 {
		f.engine.Render(dst)
	}
	if f.engine.LiveRenderActive() {
		t.Fatal("cancel stranded live render state")
	}
	if f.receiver.Start(windowsLiveStart(1), true) {
		t.Fatal("old generation restarted")
	}
	if !f.receiver.Start(windowsLiveStart(2), true) {
		t.Fatal("new generation rejected")
	}
	if got := f.receiver.Receive(windowsLiveFrame(t, 1, 90)); got != protocol.LivePTTFrameApply {
		t.Fatalf("new generation frame rejected: %s", got)
	}
}

func TestWindowsLiveStallAndPolicyRevokeCannotStrandRoute(t *testing.T) {
	stall := newWindowsLiveFixture(t, false)
	stall.receiver.Start(windowsLiveStart(1), true)
	for sequence := uint32(1); sequence <= 3; sequence++ {
		stall.receiver.Receive(windowsLiveFrame(t, sequence, 90))
	}
	dst := make([]float32, liveFrameOutput*channels)
	for range windowsLiveMaxConcealments + 1 {
		stall.receiver.Tick()
		stall.engine.Render(dst)
	}
	if stall.receiver.Snapshot().Phase != WindowsLiveIdle ||
		!hasWindowsLiveEvent(stall.eventSnapshot(), protocol.TypeLivePTTFailed) {
		t.Fatalf("stalled session did not fail closed: %+v", stall.receiver.Snapshot())
	}

	expired := newWindowsLiveFixture(t, true)
	expired.receiver.Start(windowsLiveStart(1), true)
	for sequence := uint32(1); sequence <= 3; sequence++ {
		expired.receiver.Receive(windowsLiveFrame(t, sequence, 90))
	}
	expired.clock = 301_001
	expired.receiver.Tick()
	if expired.receiver.Snapshot().Phase != WindowsLiveIdle ||
		!hasWindowsLiveEvent(expired.eventSnapshot(), protocol.TypeLivePTTFailed) {
		t.Fatalf("maximum duration did not terminate: %+v", expired.receiver.Snapshot())
	}

	revoked := newWindowsLiveFixture(t, true)
	revoked.receiver.Start(windowsLiveStart(1), true)
	for sequence := uint32(1); sequence <= 3; sequence++ {
		revoked.receiver.Receive(windowsLiveFrame(t, sequence, 90))
	}
	revoked.receiver.Revoke()
	for range 10 {
		revoked.engine.Render(dst)
	}
	if revoked.receiver.Snapshot().Phase != WindowsLiveIdle || revoked.engine.LiveRenderActive() {
		t.Fatalf("policy/coordinator revoke stranded state: snapshot=%+v active=%v",
			revoked.receiver.Snapshot(), revoked.engine.LiveRenderActive())
	}
}

func TestWindowsLiveReceiverSourceHasNoPersistenceOrRenderTransport(t *testing.T) {
	source, err := os.ReadFile("windows_live_jitter_receiver.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{
		"os.", "net.", "http.", "websocket", "File", "WriteFile", "media_items", "transmissions",
	} {
		if strings.Contains(string(source), token) {
			t.Fatalf("receiver contains persistence/transport token %q", token)
		}
	}
}

func (f *windowsLiveFixture) recordEvent(event windowsLiveEvent) {
	f.eventMu.Lock()
	f.events = append(f.events, event)
	f.eventMu.Unlock()
	select {
	case f.eventSignal <- struct{}{}:
	default:
	}
}

func (f *windowsLiveFixture) eventSnapshot() []windowsLiveEvent {
	f.eventMu.Lock()
	defer f.eventMu.Unlock()
	return append([]windowsLiveEvent(nil), f.events...)
}

func (f *windowsLiveFixture) waitForEvents(t *testing.T, count int) {
	t.Helper()
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	for len(f.eventSnapshot()) < count {
		select {
		case <-f.eventSignal:
		case <-timer.C:
			t.Fatalf("timed out waiting for %d events: %+v", count, f.eventSnapshot())
		}
	}
}

func hasWindowsLiveEvent(events []windowsLiveEvent, kind string) bool {
	for _, event := range events {
		if event.kind == kind {
			return true
		}
	}
	return false
}

func validateWindowsLiveEvents(t *testing.T, events []windowsLiveEvent) {
	t.Helper()
	for _, event := range events {
		var err error
		switch event.kind {
		case protocol.TypeLivePTTAccept:
			value, ok := event.payload.(protocol.LivePTTAcceptPayload)
			if !ok {
				t.Fatalf("accept payload type %T", event.payload)
			}
			err = protocol.ValidateLivePTTAcceptPayload(value)
		case protocol.TypeLivePTTReject:
			value, ok := event.payload.(protocol.LivePTTRejectPayload)
			if !ok {
				t.Fatalf("reject payload type %T", event.payload)
			}
			err = protocol.ValidateLivePTTRejectPayload(value)
		case protocol.TypeLivePTTFailed:
			value, ok := event.payload.(protocol.LivePTTFailedPayload)
			if !ok {
				t.Fatalf("failed payload type %T", event.payload)
			}
			err = protocol.ValidateLivePTTFailedPayload(value)
		case protocol.TypeLivePTTReceipt:
			value, ok := event.payload.(protocol.LivePTTReceiptPayload)
			if !ok {
				t.Fatalf("receipt payload type %T", event.payload)
			}
			err = protocol.ValidateLivePTTReceiptPayload(value)
		default:
			t.Fatalf("unexpected live event type %q", event.kind)
		}
		if err != nil {
			t.Fatalf("invalid %s payload: %v", event.kind, err)
		}
	}
}
