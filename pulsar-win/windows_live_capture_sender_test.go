package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	protocol "relux.works/duet/pulsar-win/wire"
)

type windowsLiveSenderStream struct {
	format     WindowsCaptureFormat
	reads      chan []float32
	stopped    chan WindowsCaptureStopReason
	stopOnce   sync.Once
	mu         sync.Mutex
	closeCount int
}

func newWindowsLiveSenderStream() *windowsLiveSenderStream {
	return &windowsLiveSenderStream{
		format: WindowsCaptureFormat{SampleRate: 48_000, Channels: 1},
		reads:  make(chan []float32, 256), stopped: make(chan WindowsCaptureStopReason, 1),
	}
}

func (s *windowsLiveSenderStream) Format() WindowsCaptureFormat { return s.format }
func (s *windowsLiveSenderStream) Read(ctx context.Context, output []float32) (uint32, error) {
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	case reason := <-s.stopped:
		return 0, &WindowsCaptureTerminalError{Reason: reason}
	case samples := <-s.reads:
		copy(output, samples)
		return uint32(len(samples)) / s.format.Channels, nil
	}
}
func (s *windowsLiveSenderStream) Stop(reason WindowsCaptureStopReason) error {
	s.stopOnce.Do(func() { s.stopped <- reason })
	return nil
}
func (s *windowsLiveSenderStream) Close() error {
	s.mu.Lock()
	s.closeCount++
	s.mu.Unlock()
	return nil
}
func (s *windowsLiveSenderStream) emit(samples []float32) {
	s.reads <- append([]float32(nil), samples...)
}
func (s *windowsLiveSenderStream) fail(reason WindowsCaptureStopReason) {
	s.stopOnce.Do(func() { s.stopped <- reason })
}

type windowsLiveSenderBackend struct {
	mu              sync.Mutex
	permission      WindowsCapturePermission
	format          WindowsCaptureFormat
	streams         []*windowsLiveSenderStream
	opens           int
	qualityRequests []WindowsCaptureQualityRequest
}

func newWindowsLiveSenderBackend() *windowsLiveSenderBackend {
	return &windowsLiveSenderBackend{
		permission: WindowsCapturePermissionAllowed,
		format: WindowsCaptureFormat{
			SampleRate: 48_000, Channels: 1,
			CommunicationsCategoryActive: true, NativeEffectsVerified: true,
		},
	}
}
func (b *windowsLiveSenderBackend) Permission(ctx context.Context, _ bool) (WindowsCapturePermission, error) {
	select {
	case <-ctx.Done():
		return WindowsCapturePermissionUnavailable, ctx.Err()
	default:
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.permission, nil
}
func (b *windowsLiveSenderBackend) ResolveInput(ctx context.Context, requested string) (string, error) {
	select {
	case <-ctx.Done():
		return "", ctx.Err()
	default:
	}
	if requested != "" {
		return requested, nil
	}
	return "mic", nil
}
func (b *windowsLiveSenderBackend) Open(ctx context.Context, _ string) (WindowsMicrophoneStream, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	default:
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	stream := newWindowsLiveSenderStream()
	stream.format = b.format
	b.streams = append(b.streams, stream)
	b.opens++
	return stream, nil
}
func (b *windowsLiveSenderBackend) OpenQuality(
	ctx context.Context,
	deviceID string,
	request WindowsCaptureQualityRequest,
) (WindowsMicrophoneStream, error) {
	b.mu.Lock()
	b.qualityRequests = append(b.qualityRequests, request)
	b.mu.Unlock()
	return b.Open(ctx, deviceID)
}
func (b *windowsLiveSenderBackend) latest() *windowsLiveSenderStream {
	b.mu.Lock()
	defer b.mu.Unlock()
	if len(b.streams) == 0 {
		return nil
	}
	return b.streams[len(b.streams)-1]
}

type windowsLiveSenderEncoder struct {
	mu    sync.Mutex
	calls int
	fail  bool
}

func (e *windowsLiveSenderEncoder) Encode(samples []float32, output []byte) (int, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.fail || len(samples) != windowsLiveCaptureFrameSamples || len(output) < 2 {
		return 0, ErrWindowsLiveCaptureEncoder
	}
	e.calls++
	output[0] = byte(e.calls)
	output[1] = 0x5a
	return 2, nil
}
func (e *windowsLiveSenderEncoder) Reset() {}

type windowsLiveSenderBox struct {
	mu       sync.Mutex
	frames   []protocol.LivePTTBinaryFrame
	controls []windowsLiveSenderControl
	events   []WindowsLiveCaptureEvent
	accept   bool
}

type windowsLiveSenderControl struct {
	kind    string
	payload any
}

func (b *windowsLiveSenderBox) tryFrame(frame protocol.LivePTTBinaryFrame) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.accept {
		return false
	}
	frame.Payload = append([]byte(nil), frame.Payload...)
	b.frames = append(b.frames, frame)
	return true
}
func (b *windowsLiveSenderBox) control(kind string, payload any) {
	b.mu.Lock()
	b.controls = append(b.controls, windowsLiveSenderControl{kind: kind, payload: payload})
	b.mu.Unlock()
}
func (b *windowsLiveSenderBox) event(event WindowsLiveCaptureEvent) {
	b.mu.Lock()
	b.events = append(b.events, event)
	b.mu.Unlock()
}
func (b *windowsLiveSenderBox) counts() (int, int) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.frames), len(b.controls)
}

type windowsLiveSenderFixture struct {
	backend *windowsLiveSenderBackend
	encoder *windowsLiveSenderEncoder
	box     *windowsLiveSenderBox
	clockMu sync.Mutex
	clock   int64
	sender  *WindowsLiveCaptureSender
}

type fixedWindowsCaptureQualityRoute string

func (r fixedWindowsCaptureQualityRoute) ResolvedCaptureQualityMode() string { return string(r) }

func newWindowsLiveSenderFixture() *windowsLiveSenderFixture {
	f := &windowsLiveSenderFixture{
		backend: newWindowsLiveSenderBackend(), encoder: &windowsLiveSenderEncoder{},
		box: &windowsLiveSenderBox{accept: true}, clock: 1_001,
	}
	f.sender = NewWindowsLiveCaptureSender(
		f.backend, f.encoder, false, f.now, func() uint64 { return 2_000_000 },
		f.box.tryFrame, f.box.control, f.box.event,
	)
	f.sender.SetCaptureQualityRouteResolver(fixedWindowsCaptureQualityRoute("headphone"))
	return f
}

func TestWindowsLiveCaptureQualityIsRequiredAndForwarded(t *testing.T) {
	f := newWindowsLiveSenderFixture()
	generation, ok := f.sender.LocalHoldBegan(WindowsLiveHoldButton, true, "")
	if !ok {
		t.Fatal("hold did not start")
	}
	if err := f.sender.AcceptStart(context.Background(), windowsLiveSenderStart(1, 1000), generation, true); err != nil {
		t.Fatal(err)
	}
	f.backend.mu.Lock()
	requests := append([]WindowsCaptureQualityRequest(nil), f.backend.qualityRequests...)
	f.backend.mu.Unlock()
	if len(requests) != 1 || requests[0] != windowsLiveCaptureQualityRequest() || requests[0].DegradedConsent {
		t.Fatalf("live quality request = %+v", requests)
	}
	f.box.mu.Lock()
	events := append([]WindowsLiveCaptureEvent(nil), f.box.events...)
	f.box.mu.Unlock()
	foundAccepted := false
	for _, event := range events {
		if event.Kind == WindowsLiveCaptureQualityEvent && event.Quality != nil &&
			event.Quality.Lifecycle == protocol.CaptureLifecycleCapturing &&
			event.Quality.Quality == protocol.CaptureQualityAccepted {
			foundAccepted = true
		}
	}
	if !foundAccepted {
		t.Fatal("accepted quality state was not forwarded")
	}
	f.sender.LocalHoldEnded(generation)
	waitWindowsLiveSender(t, func() bool { return f.sender.Snapshot().Phase == WindowsLiveCaptureIdle })
}

func TestWindowsLiveCaptureUnverifiedEffectsFailClosed(t *testing.T) {
	f := newWindowsLiveSenderFixture()
	f.backend.format.NativeEffectsVerified = false
	generation, ok := f.sender.LocalHoldBegan(WindowsLiveHoldButton, true, "")
	if !ok {
		t.Fatal("hold did not start")
	}
	err := f.sender.AcceptStart(context.Background(), windowsLiveSenderStart(1, 1000), generation, true)
	if !errors.Is(err, ErrWindowsLiveCaptureQualityUnsupported) {
		t.Fatalf("quality error = %v", err)
	}
	if f.sender.Snapshot().Phase != WindowsLiveCaptureIdle || f.backend.latest().closeCount != 1 {
		t.Fatal("quality rejection retained capture")
	}
	f.box.mu.Lock()
	controls := append([]windowsLiveSenderControl(nil), f.box.controls...)
	f.box.mu.Unlock()
	if len(controls) != 1 || controls[0].kind != protocol.TypeLivePTTFailed {
		t.Fatalf("quality rejection control = %+v", controls)
	}
	payload, ok := controls[0].payload.(protocol.LivePTTFailedPayload)
	if !ok || payload.Code != "capture_quality_unsupported" {
		t.Fatalf("quality rejection payload = %+v", controls[0].payload)
	}
}

func TestWindowsLiveCaptureExplicitDegradedConsentCanProceed(t *testing.T) {
	f := newWindowsLiveSenderFixture()
	f.backend.format.NativeEffectsVerified = false
	f.sender.SetCaptureQualityRequest(WindowsCaptureQualityRequest{
		Mode: WindowsCaptureQualityAuto, ProcessingRequested: true, DegradedConsent: true,
	})
	generation, ok := f.sender.LocalHoldBegan(WindowsLiveHoldButton, true, "")
	if !ok {
		t.Fatal("hold did not start")
	}
	if err := f.sender.AcceptStart(context.Background(), windowsLiveSenderStart(1, 1000), generation, true); err != nil {
		t.Fatal(err)
	}
	f.box.mu.Lock()
	events := append([]WindowsLiveCaptureEvent(nil), f.box.events...)
	f.box.mu.Unlock()
	foundDegraded := false
	for _, event := range events {
		if event.Kind == WindowsLiveCaptureQualityEvent && event.Quality != nil &&
			event.Quality.Lifecycle == protocol.CaptureLifecycleCapturing &&
			event.Quality.Quality == protocol.CaptureQualityDegraded &&
			event.Quality.Reason == "aec_unavailable" {
			foundDegraded = true
		}
	}
	if !foundDegraded {
		t.Fatal("explicit degraded consent did not expose degraded capture state")
	}
	f.sender.LocalHoldEnded(generation)
	waitWindowsLiveSender(t, func() bool { return f.sender.Snapshot().Phase == WindowsLiveCaptureIdle })
}

func (f *windowsLiveSenderFixture) now() int64 {
	f.clockMu.Lock()
	defer f.clockMu.Unlock()
	return f.clock
}
func (f *windowsLiveSenderFixture) setNow(value int64) {
	f.clockMu.Lock()
	f.clock = value
	f.clockMu.Unlock()
}

func windowsLiveSenderStart(generation int64, now int64) protocol.LivePTTStartPayload {
	return protocol.LivePTTStartPayload{
		SessionID: "00112233445566778899aabbccddeeff", Generation: generation,
		SenderActorID: 1, SenderOrbitID: 2, SenderNodeID: "win",
		TargetSnapshot: "lts1.sender", TargetSHA256: strings.Repeat("a", 64), TargetCount: 1,
		PlaybackDomain: "personal", PlaybackDomainID: 1,
		CodecProfile: protocol.LivePTTCodecProfile, FrameMS: protocol.LivePTTFrameMS,
		MaxPayloadBytes: protocol.LivePTTMaxPayloadBytes, JitterBufferMS: protocol.LivePTTJitterBufferMS,
		StartedAtCoordMS: now, AcceptDeadlineCoordMS: now + protocol.LivePTTAcceptTimeoutMS,
		MaxDurationMS: protocol.LivePTTMaxDurationMS, MixedVersionPolicy: protocol.LivePTTMixedVersionRequireAll,
		LateJoinPolicy: protocol.LivePTTLateJoinPolicy, CaptureAuthority: protocol.LivePTTCaptureAuthority,
	}
}

func waitWindowsLiveSender(t *testing.T, condition func() bool) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if condition() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("timed out waiting for live sender state")
}

func TestWindowsLiveCaptureRequiresCurrentLocalHoldAndFallsBackBeforeMicrophone(t *testing.T) {
	f := newWindowsLiveSenderFixture()
	if generation, ok := f.sender.LocalHoldBegan(WindowsLiveHoldShortcut, false, ""); ok || generation != 0 {
		t.Fatal("unsupported hold must fall back without a generation")
	}
	if err := f.sender.AcceptStart(context.Background(), windowsLiveSenderStart(1, 1000), 1, true); !errors.Is(err, ErrWindowsLiveCaptureInvalidStart) {
		t.Fatalf("unsolicited start error = %v", err)
	}
	if f.backend.opens != 0 {
		t.Fatal("remote start opened the microphone")
	}
	generation, ok := f.sender.LocalHoldBegan(WindowsLiveHoldButton, true, "mic")
	if !ok || generation == 0 {
		t.Fatal("local hold was not accepted")
	}
	if _, second := f.sender.LocalHoldBegan(WindowsLiveHoldButton, true, ""); second {
		t.Fatal("concurrent hold was accepted")
	}
	if err := f.sender.AcceptStart(context.Background(), windowsLiveSenderStart(1, 1000), generation+1, true); !errors.Is(err, ErrWindowsLiveCaptureInvalidStart) {
		t.Fatalf("stale generation error = %v", err)
	}
	if err := f.sender.AcceptStart(context.Background(), windowsLiveSenderStart(1, 1000), generation, false); !errors.Is(err, ErrWindowsLiveCaptureInvalidStart) {
		t.Fatalf("unauthorized start error = %v", err)
	}
	if err := f.sender.AcceptStart(context.Background(), windowsLiveSenderStart(1, 1000), generation, true); err != nil {
		t.Fatal(err)
	}
	f.sender.LocalHoldEnded(generation)
	waitWindowsLiveSender(t, func() bool { return f.sender.Snapshot().Phase == WindowsLiveCaptureIdle })
}

func TestWindowsLiveCaptureFramesAreBoundedOrderedAndTerminal(t *testing.T) {
	f := newWindowsLiveSenderFixture()
	generation, _ := f.sender.LocalHoldBegan(WindowsLiveHoldMenu, true, "")
	if err := f.sender.AcceptStart(context.Background(), windowsLiveSenderStart(9, 1000), generation, true); err != nil {
		t.Fatal(err)
	}
	for range 3 {
		f.backend.latest().emit(make([]float32, 960))
	}
	waitWindowsLiveSender(t, func() bool { return f.sender.Snapshot().Sequence == 3 })
	f.sender.LocalHoldEnded(generation)
	waitWindowsLiveSender(t, func() bool {
		frames, controls := f.box.counts()
		return f.sender.Snapshot().Phase == WindowsLiveCaptureIdle && frames == 3 && controls == 1
	})
	f.box.mu.Lock()
	frames := append([]protocol.LivePTTBinaryFrame(nil), f.box.frames...)
	control := f.box.controls[0]
	f.box.mu.Unlock()
	for index, frame := range frames {
		if frame.Sequence != uint32(index+1) || frame.CaptureMonotonicUS != 2_000_000+uint64(index)*20_000 {
			t.Fatalf("frame %d = %+v", index, frame)
		}
		if len(frame.Payload) == 0 || len(frame.Payload) > protocol.LivePTTMaxPayloadBytes {
			t.Fatalf("payload bytes = %d", len(frame.Payload))
		}
		if _, err := protocol.EncodeLivePTTBinaryFrame(frame); err != nil {
			t.Fatalf("invalid frame: %v", err)
		}
	}
	if frames[0].Flags&protocol.LivePTTFlagStart == 0 || frames[2].Flags&protocol.LivePTTFlagEnd == 0 {
		t.Fatal("start/end flags are not frozen on first/last frame")
	}
	end, ok := control.payload.(protocol.LivePTTEndPayload)
	if control.kind != protocol.TypeLivePTTEnd || !ok || end.LastSequence != 3 || end.Reason != "release" || protocol.ValidateLivePTTEndPayload(end) != nil {
		t.Fatalf("terminal = %#v", control)
	}
	stream := f.backend.latest()
	stream.mu.Lock()
	closed := stream.closeCount
	stream.mu.Unlock()
	if closed != 1 || f.sender.Snapshot().StreamActive {
		t.Fatalf("stream close=%d active=%v", closed, f.sender.Snapshot().StreamActive)
	}
}

func TestWindowsLiveCaptureBackpressureAndEncoderFailureFailClosed(t *testing.T) {
	backpressure := newWindowsLiveSenderFixture()
	backpressure.box.accept = false
	generation, _ := backpressure.sender.LocalHoldBegan(WindowsLiveHoldButton, true, "")
	if err := backpressure.sender.AcceptStart(context.Background(), windowsLiveSenderStart(1, 1000), generation, true); err != nil {
		t.Fatal(err)
	}
	for range 16 {
		backpressure.backend.latest().emit(make([]float32, 960))
	}
	waitWindowsLiveSender(t, func() bool { return backpressure.sender.Snapshot().Phase == WindowsLiveCaptureIdle })
	waitWindowsLiveSender(t, func() bool { _, controls := backpressure.box.counts(); return controls == 1 })
	backpressure.box.mu.Lock()
	cancel, ok := backpressure.box.controls[0].payload.(protocol.LivePTTCancelPayload)
	backpressure.box.mu.Unlock()
	if !ok || cancel.Reason != "backpressure" || protocol.ValidateLivePTTCancelPayload(cancel) != nil {
		t.Fatalf("backpressure terminal = %#v", cancel)
	}
	if backpressure.sender.Snapshot().QueuedFrames != 0 || backpressure.sender.Snapshot().StreamActive {
		t.Fatal("backpressure retained stream or queue")
	}

	encoding := newWindowsLiveSenderFixture()
	encoding.encoder.fail = true
	generation, _ = encoding.sender.LocalHoldBegan(WindowsLiveHoldButton, true, "")
	if err := encoding.sender.AcceptStart(context.Background(), windowsLiveSenderStart(2, 1000), generation, true); err != nil {
		t.Fatal(err)
	}
	encoding.backend.latest().emit(make([]float32, 960))
	waitWindowsLiveSender(t, func() bool { return encoding.sender.Snapshot().Phase == WindowsLiveCaptureIdle })
	waitWindowsLiveSender(t, func() bool { _, controls := encoding.box.counts(); return controls == 1 })
	encoding.box.mu.Lock()
	failed, ok := encoding.box.controls[0].payload.(protocol.LivePTTFailedPayload)
	encoding.box.mu.Unlock()
	if !ok || failed.Code != "encoder_failure" || protocol.ValidateLivePTTFailedPayload(failed) != nil {
		t.Fatalf("encoder terminal = %#v", failed)
	}
}

func TestWindowsLiveCaptureDoesNotOverlapTerminalDrainWithNextHold(t *testing.T) {
	f := newWindowsLiveSenderFixture()
	f.box.accept = false
	generation, _ := f.sender.LocalHoldBegan(WindowsLiveHoldButton, true, "")
	if err := f.sender.AcceptStart(context.Background(), windowsLiveSenderStart(1, 1000), generation, true); err != nil {
		t.Fatal(err)
	}
	f.backend.latest().emit(make([]float32, 960))
	waitWindowsLiveSender(t, func() bool { return f.sender.Snapshot().Sequence == 1 })
	f.sender.LocalHoldEnded(generation)
	waitWindowsLiveSender(t, func() bool { return f.sender.Snapshot().Phase == WindowsLiveCaptureStopping })
	if next, ok := f.sender.LocalHoldBegan(WindowsLiveHoldButton, true, ""); ok || next != 0 {
		t.Fatal("new hold overlapped the prior terminal drain")
	}
	waitWindowsLiveSender(t, func() bool { return f.sender.Snapshot().Phase == WindowsLiveCaptureIdle })
	f.box.mu.Lock()
	defer f.box.mu.Unlock()
	if len(f.box.controls) != 1 {
		t.Fatalf("terminal controls = %d", len(f.box.controls))
	}
	cancel, ok := f.box.controls[0].payload.(protocol.LivePTTCancelPayload)
	if !ok || cancel.Reason != "backpressure" {
		t.Fatalf("drain timeout terminal = %#v", f.box.controls[0])
	}
}

func TestWindowsLiveCaptureLowestRateResamplingAndNativeFailureStayBounded(t *testing.T) {
	f := newWindowsLiveSenderFixture()
	f.backend.format = WindowsCaptureFormat{
		SampleRate: 8_000, Channels: 1,
		CommunicationsCategoryActive: true, NativeEffectsVerified: true,
	}
	generation, _ := f.sender.LocalHoldBegan(WindowsLiveHoldButton, true, "")
	if err := f.sender.AcceptStart(context.Background(), windowsLiveSenderStart(1, 1000), generation, true); err != nil {
		t.Fatal(err)
	}
	for sequence := 1; sequence <= 12; sequence++ {
		f.backend.latest().emit(make([]float32, 160))
		want := uint32(sequence)
		waitWindowsLiveSender(t, func() bool { return f.sender.Snapshot().Sequence == want })
	}
	f.sender.LocalHoldEnded(generation)
	waitWindowsLiveSender(t, func() bool {
		frames, controls := f.box.counts()
		return f.sender.Snapshot().Phase == WindowsLiveCaptureIdle && frames == 12 && controls == 1
	})
	if f.sender.Snapshot().EncodedFrames != 12 {
		t.Fatalf("encoded frames = %d", f.sender.Snapshot().EncodedFrames)
	}

	failed := newWindowsLiveSenderFixture()
	generation, _ = failed.sender.LocalHoldBegan(WindowsLiveHoldButton, true, "")
	if err := failed.sender.AcceptStart(context.Background(), windowsLiveSenderStart(2, 1000), generation, true); err != nil {
		t.Fatal(err)
	}
	failed.backend.latest().fail(WindowsCaptureDeviceLost)
	waitWindowsLiveSender(t, func() bool { return failed.sender.Snapshot().Phase == WindowsLiveCaptureIdle })
	waitWindowsLiveSender(t, func() bool { _, controls := failed.box.counts(); return controls == 1 })
	failed.box.mu.Lock()
	payload, ok := failed.box.controls[0].payload.(protocol.LivePTTFailedPayload)
	failed.box.mu.Unlock()
	if !ok || payload.Code != "device_lost" || protocol.ValidateLivePTTFailedPayload(payload) != nil {
		t.Fatalf("native failure terminal = %#v", payload)
	}
}

func TestWindowsLiveCaptureOneHundredCyclesAndStaleRelease(t *testing.T) {
	f := newWindowsLiveSenderFixture()
	var old uint64
	for cycle := 1; cycle <= 100; cycle++ {
		generation, ok := f.sender.LocalHoldBegan(WindowsLiveHoldButton, true, "")
		if !ok {
			t.Fatalf("cycle %d hold rejected", cycle)
		}
		if old != 0 {
			f.sender.LocalHoldEnded(old)
		}
		if err := f.sender.AcceptStart(context.Background(), windowsLiveSenderStart(int64(cycle), 1000), generation, true); err != nil {
			t.Fatalf("cycle %d: %v", cycle, err)
		}
		f.backend.latest().emit(make([]float32, 1_920))
		waitWindowsLiveSender(t, func() bool { return f.sender.Snapshot().Sequence == 2 })
		f.sender.LocalHoldEnded(generation)
		waitWindowsLiveSender(t, func() bool { return f.sender.Snapshot().Phase == WindowsLiveCaptureIdle })
		old = generation
	}
	f.backend.mu.Lock()
	opens := f.backend.opens
	streams := append([]*windowsLiveSenderStream(nil), f.backend.streams...)
	f.backend.mu.Unlock()
	if opens != 100 {
		t.Fatalf("opens = %d", opens)
	}
	for index, stream := range streams {
		stream.mu.Lock()
		closed := stream.closeCount
		stream.mu.Unlock()
		if closed != 1 {
			t.Fatalf("stream %d close count = %d", index, closed)
		}
	}
}

func TestWindowsLiveCaptureWatchdogLifecycleAndPermissionCleanup(t *testing.T) {
	tests := []struct {
		name   string
		stop   func(*WindowsLiveCaptureSender)
		reason WindowsLiveCaptureStopReason
	}{
		{"lock", (*WindowsLiveCaptureSender).HandleSessionLock, WindowsLiveCaptureLock},
		{"suspend", (*WindowsLiveCaptureSender).HandleSuspend, WindowsLiveCaptureSleep},
		{"permission", (*WindowsLiveCaptureSender).HandlePermissionRevoke, WindowsLiveCapturePermissionLost},
		{"device", (*WindowsLiveCaptureSender).HandleDeviceLoss, WindowsLiveCaptureDeviceLost},
		{"disconnect", (*WindowsLiveCaptureSender).HandleDisconnect, WindowsLiveCaptureDisconnected},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newWindowsLiveSenderFixture()
			generation, _ := f.sender.LocalHoldBegan(WindowsLiveHoldButton, true, "")
			if err := f.sender.AcceptStart(context.Background(), windowsLiveSenderStart(1, 1000), generation, true); err != nil {
				t.Fatal(err)
			}
			f.backend.latest().emit(make([]float32, 960))
			waitWindowsLiveSender(t, func() bool { return f.sender.Snapshot().Sequence == 1 })
			test.stop(f.sender)
			waitWindowsLiveSender(t, func() bool { return f.sender.Snapshot().Phase == WindowsLiveCaptureIdle })
			if f.sender.Snapshot().StreamActive {
				t.Fatal("stream remains active")
			}
			f.box.mu.Lock()
			events := append([]WindowsLiveCaptureEvent(nil), f.box.events...)
			f.box.mu.Unlock()
			found := false
			for _, event := range events {
				found = found || event.Kind == WindowsLiveCaptureTerminalEvent && event.Reason == test.reason
			}
			if !found {
				t.Fatalf("terminal event %s missing: %#v", test.reason, events)
			}
		})
	}

	lost := newWindowsLiveSenderFixture()
	generation, _ := lost.sender.LocalHoldBegan(WindowsLiveHoldButton, true, "")
	if err := lost.sender.AcceptStart(context.Background(), windowsLiveSenderStart(1, 1000), generation, true); err != nil {
		t.Fatal(err)
	}
	lost.setNow(2_502)
	lost.sender.RunWatchdogCheck()
	waitWindowsLiveSender(t, func() bool { return lost.sender.Snapshot().Phase == WindowsLiveCaptureIdle })

	denied := newWindowsLiveSenderFixture()
	denied.backend.permission = WindowsCapturePermissionDenied
	generation, _ = denied.sender.LocalHoldBegan(WindowsLiveHoldButton, true, "")
	if err := denied.sender.AcceptStart(context.Background(), windowsLiveSenderStart(1, 1000), generation, true); !errors.Is(err, ErrWindowsLiveCaptureUnavailable) {
		t.Fatalf("permission error = %v", err)
	}
	if denied.backend.opens != 0 || denied.sender.Snapshot().Phase != WindowsLiveCaptureIdle {
		t.Fatal("denied permission opened or retained capture")
	}

	localStop := newWindowsLiveSenderFixture()
	generation, _ = localStop.sender.LocalHoldBegan(WindowsLiveHoldButton, true, "")
	if err := localStop.sender.AcceptStart(context.Background(), windowsLiveSenderStart(1, 1000), generation, true); err != nil {
		t.Fatal(err)
	}
	localStop.sender.LocalStop()
	waitWindowsLiveSender(t, func() bool { return localStop.sender.Snapshot().Phase == WindowsLiveCaptureIdle })
	waitWindowsLiveSender(t, func() bool { _, controls := localStop.box.counts(); return controls == 1 })
	localStop.box.mu.Lock()
	cancel, ok := localStop.box.controls[0].payload.(protocol.LivePTTCancelPayload)
	localStop.box.mu.Unlock()
	if !ok || cancel.Reason != "user_cancel" || protocol.ValidateLivePTTCancelPayload(cancel) != nil {
		t.Fatalf("local stop terminal = %#v", cancel)
	}

	maximum := newWindowsLiveSenderFixture()
	generation, _ = maximum.sender.LocalHoldBegan(WindowsLiveHoldButton, true, "")
	if err := maximum.sender.AcceptStart(context.Background(), windowsLiveSenderStart(1, 1000), generation, true); err != nil {
		t.Fatal(err)
	}
	maximum.setNow(301_001)
	maximum.sender.LocalHoldHeartbeat(generation)
	maximum.sender.RunWatchdogCheck()
	waitWindowsLiveSender(t, func() bool { return maximum.sender.Snapshot().Phase == WindowsLiveCaptureIdle })
	waitWindowsLiveSender(t, func() bool { _, controls := maximum.box.counts(); return controls == 1 })
	maximum.box.mu.Lock()
	timeout, ok := maximum.box.controls[0].payload.(protocol.LivePTTCancelPayload)
	maximum.box.mu.Unlock()
	if !ok || timeout.Reason != "timeout" || protocol.ValidateLivePTTCancelPayload(timeout) != nil {
		t.Fatalf("maximum duration terminal = %#v", timeout)
	}
}

func TestWindowsLiveCaptureWorkerHasNoDiskLoggingOrTransportWork(t *testing.T) {
	source, err := os.ReadFile("windows_live_capture_sender.go")
	if err != nil {
		t.Fatal(err)
	}
	text := string(source)
	for _, forbidden := range []string{"os.Open", "os.Write", "log.", "slog.", "CaptureMediaStore"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("live sender contains forbidden persistence/logging token %q", forbidden)
		}
	}
	start := strings.Index(text, "func (s *WindowsLiveCaptureSender) captureWorker")
	end := strings.Index(text[start:], "\nfunc (s *WindowsLiveCaptureSender) nextSequence")
	if start < 0 || end < 0 {
		t.Fatal("capture worker source markers missing")
	}
	worker := text[start : start+end]
	for _, forbidden := range []string{"trySendFrame", "sendControl", "os.", "log."} {
		if strings.Contains(worker, forbidden) {
			t.Fatalf("capture worker contains forbidden realtime-adjacent work %q", forbidden)
		}
	}
}
