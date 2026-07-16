package main

import (
	"context"
	"errors"
	"math"
	"sync"
	"time"

	protocol "relux.works/duet/pulsar-win/wire"
)

const (
	windowsLiveCaptureFrameSamples = 960
	windowsLiveCaptureQueueFrames  = 8
	windowsLiveCaptureReadFrames   = 2048
	windowsLiveHoldWatchdogMS      = int64(1500)
)

var (
	ErrWindowsLiveCaptureInvalidStart = errors.New("windows_live_capture_invalid_start")
	ErrWindowsLiveCaptureUnavailable  = errors.New("windows_live_capture_unavailable")
	ErrWindowsLiveCaptureEncoder      = errors.New("windows_live_capture_encoder_failure")
)

type WindowsLiveOpusEncoder interface {
	Encode(samples []float32, output []byte) (int, error)
	Reset()
}

type WindowsLiveHoldSource string

const (
	WindowsLiveHoldButton   WindowsLiveHoldSource = "button"
	WindowsLiveHoldMenu     WindowsLiveHoldSource = "menu"
	WindowsLiveHoldShortcut WindowsLiveHoldSource = "shortcut"
)

type WindowsLiveCapturePhase string

const (
	WindowsLiveCaptureIdle       WindowsLiveCapturePhase = "idle"
	WindowsLiveCaptureAwaiting   WindowsLiveCapturePhase = "awaiting_start"
	WindowsLiveCapturePermission WindowsLiveCapturePhase = "requesting_permission"
	WindowsLiveCaptureActive     WindowsLiveCapturePhase = "capturing"
	WindowsLiveCaptureStopping   WindowsLiveCapturePhase = "stopping"
)

type WindowsLiveCaptureStopReason string

const (
	WindowsLiveCaptureReleased       WindowsLiveCaptureStopReason = "release"
	WindowsLiveCaptureLocalStop      WindowsLiveCaptureStopReason = "local_stop"
	WindowsLiveCaptureLostRelease    WindowsLiveCaptureStopReason = "lost_release"
	WindowsLiveCaptureLock           WindowsLiveCaptureStopReason = "lock"
	WindowsLiveCaptureSleep          WindowsLiveCaptureStopReason = "sleep"
	WindowsLiveCapturePermissionLost WindowsLiveCaptureStopReason = "permission_revoked"
	WindowsLiveCaptureDeviceLost     WindowsLiveCaptureStopReason = "device_lost"
	WindowsLiveCaptureQuit           WindowsLiveCaptureStopReason = "quit"
	WindowsLiveCaptureDisconnected   WindowsLiveCaptureStopReason = "disconnect"
	WindowsLiveCaptureBackpressure   WindowsLiveCaptureStopReason = "backpressure"
	WindowsLiveCaptureEncodeFailure  WindowsLiveCaptureStopReason = "encoder_failure"
	WindowsLiveCaptureMaximum        WindowsLiveCaptureStopReason = "maximum_duration"
	WindowsLiveCaptureCoordinator    WindowsLiveCaptureStopReason = "coordinator_cancelled"
)

type WindowsLiveCaptureEventKind string

const (
	WindowsLiveCapturePhaseEvent    WindowsLiveCaptureEventKind = "phase"
	WindowsLiveCaptureRequestEvent  WindowsLiveCaptureEventKind = "request_start"
	WindowsLiveCaptureMeterEvent    WindowsLiveCaptureEventKind = "meter"
	WindowsLiveCaptureStartCueEvent WindowsLiveCaptureEventKind = "start_cue"
	WindowsLiveCaptureStopCueEvent  WindowsLiveCaptureEventKind = "stop_cue"
	WindowsLiveCaptureFallbackEvent WindowsLiveCaptureEventKind = "fallback_to_clip"
	WindowsLiveCaptureTerminalEvent WindowsLiveCaptureEventKind = "terminal"
)

type WindowsLiveCaptureEvent struct {
	Kind       WindowsLiveCaptureEventKind
	Phase      WindowsLiveCapturePhase
	Source     WindowsLiveHoldSource
	Generation uint64
	Meter      float32
	Reason     WindowsLiveCaptureStopReason
}

type WindowsLiveCaptureSnapshot struct {
	Phase           WindowsLiveCapturePhase
	LocalGeneration uint64
	SessionID       string
	Sequence        uint32
	QueuedFrames    int
	StreamActive    bool
	EncodedFrames   uint64
	EncodedBytes    uint64
}

type windowsLiveCaptureRuntime struct {
	start             protocol.LivePTTStartPayload
	sessionID         [16]byte
	localGeneration   uint64
	ctx               context.Context
	cancel            context.CancelFunc
	stream            WindowsMicrophoneStream
	stopReason        WindowsLiveCaptureStopReason
	frames            chan protocol.LivePTTBinaryFrame
	retryTransport    chan struct{}
	stopTransport     chan struct{}
	done              chan struct{}
	doneOnce          sync.Once
	transportOnce     sync.Once
	transportMu       sync.Mutex
	terminalReason    WindowsLiveCaptureStopReason
	terminalSequence  uint32
	transportDeadline time.Time
}

func (r *windowsLiveCaptureRuntime) stopTransportNow() {
	r.transportOnce.Do(func() { close(r.stopTransport) })
}

func (r *windowsLiveCaptureRuntime) finishWorker() {
	r.doneOnce.Do(func() { close(r.done) })
}

func (r *windowsLiveCaptureRuntime) setTerminal(reason WindowsLiveCaptureStopReason, sequence uint32) {
	r.transportMu.Lock()
	r.terminalReason = reason
	r.terminalSequence = sequence
	r.transportDeadline = time.Now().Add(time.Duration(protocol.LivePTTDrainTimeoutMS) * time.Millisecond)
	r.transportMu.Unlock()
}

func (r *windowsLiveCaptureRuntime) terminal() (WindowsLiveCaptureStopReason, uint32, time.Time) {
	r.transportMu.Lock()
	defer r.transportMu.Unlock()
	return r.terminalReason, r.terminalSequence, r.transportDeadline
}

// WindowsLiveCaptureSender binds microphone ownership to a current local hold.
// The WASAPI reader only copies into fixed worker buffers; encode and transport
// are downstream. The injected transport must be a non-blocking try operation.
// No production constructor is exposed until a reviewed signed libopus path can
// provide every frozen encoder control.
type WindowsLiveCaptureSender struct {
	mu               sync.Mutex
	backend          WindowsMicrophoneBackend
	encoder          WindowsLiveOpusEncoder
	coordinatorNowMS func() int64
	monotonicUS      func() uint64
	trySendFrame     func(protocol.LivePTTBinaryFrame) bool
	sendControl      func(string, any)
	onEvent          func(WindowsLiveCaptureEvent)
	automaticWatch   bool

	phase            WindowsLiveCapturePhase
	localGeneration  uint64
	lastHeartbeatMS  int64
	selectedDeviceID string
	active           *windowsLiveCaptureRuntime
	sequence         uint32
	queuedFrames     int
	streamActive     bool
	encodedFrames    uint64
	encodedBytes     uint64
}

func NewWindowsLiveCaptureSender(
	backend WindowsMicrophoneBackend,
	encoder WindowsLiveOpusEncoder,
	automaticWatch bool,
	coordinatorNowMS func() int64,
	monotonicUS func() uint64,
	trySendFrame func(protocol.LivePTTBinaryFrame) bool,
	sendControl func(string, any),
	onEvent func(WindowsLiveCaptureEvent),
) *WindowsLiveCaptureSender {
	if backend == nil || encoder == nil || coordinatorNowMS == nil ||
		monotonicUS == nil || trySendFrame == nil || sendControl == nil {
		return nil
	}
	return &WindowsLiveCaptureSender{
		backend: backend, encoder: encoder, automaticWatch: automaticWatch,
		coordinatorNowMS: coordinatorNowMS, monotonicUS: monotonicUS,
		trySendFrame: trySendFrame, sendControl: sendControl, onEvent: onEvent,
		phase: WindowsLiveCaptureIdle,
	}
}

func (s *WindowsLiveCaptureSender) Snapshot() WindowsLiveCaptureSnapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	snapshot := WindowsLiveCaptureSnapshot{
		Phase: s.phase, LocalGeneration: s.localGeneration,
		Sequence: s.sequence, QueuedFrames: s.queuedFrames,
		StreamActive: s.streamActive, EncodedFrames: s.encodedFrames,
		EncodedBytes: s.encodedBytes,
	}
	if s.active != nil {
		snapshot.SessionID = s.active.start.SessionID
	}
	return snapshot
}

func (s *WindowsLiveCaptureSender) LocalHoldBegan(source WindowsLiveHoldSource, holdAvailable bool, deviceID string) (uint64, bool) {
	s.mu.Lock()
	if s.phase != WindowsLiveCaptureIdle || s.active != nil {
		s.mu.Unlock()
		return 0, false
	}
	if !holdAvailable {
		s.mu.Unlock()
		s.emit(WindowsLiveCaptureEvent{Kind: WindowsLiveCaptureFallbackEvent})
		return 0, false
	}
	s.localGeneration++
	generation := s.localGeneration
	s.selectedDeviceID = deviceID
	s.lastHeartbeatMS = s.coordinatorNowMS()
	s.phase = WindowsLiveCaptureAwaiting
	s.mu.Unlock()
	s.emit(WindowsLiveCaptureEvent{Kind: WindowsLiveCapturePhaseEvent, Phase: WindowsLiveCaptureAwaiting})
	s.emit(WindowsLiveCaptureEvent{Kind: WindowsLiveCaptureRequestEvent, Source: source, Generation: generation})
	return generation, true
}

func (s *WindowsLiveCaptureSender) LocalHoldHeartbeat(generation uint64) {
	s.mu.Lock()
	if generation == s.localGeneration && s.phase != WindowsLiveCaptureIdle {
		s.lastHeartbeatMS = s.coordinatorNowMS()
	}
	s.mu.Unlock()
}

func (s *WindowsLiveCaptureSender) AcceptStart(ctx context.Context, payload protocol.LivePTTStartPayload, generation uint64, authorized bool) error {
	sessionID, parseErr := protocol.ParseLivePTTSessionID(payload.SessionID)
	s.mu.Lock()
	if parseErr != nil || protocol.ValidateLivePTTStartPayload(payload) != nil || !authorized ||
		generation != s.localGeneration || s.phase != WindowsLiveCaptureAwaiting || s.active != nil {
		s.mu.Unlock()
		return ErrWindowsLiveCaptureInvalidStart
	}
	startCtx, cancel := context.WithCancel(ctx)
	runtime := &windowsLiveCaptureRuntime{
		start: payload, sessionID: sessionID, localGeneration: generation,
		ctx: startCtx, cancel: cancel, frames: make(chan protocol.LivePTTBinaryFrame, windowsLiveCaptureQueueFrames),
		retryTransport: make(chan struct{}, 1), stopTransport: make(chan struct{}), done: make(chan struct{}),
	}
	s.active = runtime
	s.phase = WindowsLiveCapturePermission
	deviceID := s.selectedDeviceID
	s.mu.Unlock()
	s.emit(WindowsLiveCaptureEvent{Kind: WindowsLiveCapturePhaseEvent, Phase: WindowsLiveCapturePermission})

	permission, err := s.backend.Permission(startCtx, true)
	if err != nil || permission != WindowsCapturePermissionAllowed {
		s.finishStartFailure(runtime, WindowsLiveCapturePermissionLost)
		return ErrWindowsLiveCaptureUnavailable
	}
	resolved, err := s.backend.ResolveInput(startCtx, deviceID)
	if err != nil || resolved == "" {
		s.finishStartFailure(runtime, WindowsLiveCaptureDeviceLost)
		return ErrWindowsLiveCaptureUnavailable
	}
	stream, err := s.backend.Open(startCtx, resolved)
	if err != nil || stream == nil {
		s.finishStartFailure(runtime, WindowsLiveCaptureDeviceLost)
		return ErrWindowsLiveCaptureUnavailable
	}
	format := stream.Format()
	if format.SampleRate < 8_000 || format.SampleRate > 192_000 || format.Channels == 0 || format.Channels > 32 {
		_ = stream.Stop(WindowsCaptureBackendFailure)
		_ = stream.Close()
		s.finishStartFailure(runtime, WindowsLiveCaptureDeviceLost)
		return ErrWindowsLiveCaptureUnavailable
	}

	s.mu.Lock()
	if s.active != runtime || s.phase != WindowsLiveCapturePermission ||
		runtime.stopReason != "" || generation != s.localGeneration {
		s.mu.Unlock()
		_ = stream.Stop(WindowsCaptureCancel)
		_ = stream.Close()
		s.finishStartFailure(runtime, s.runtimeReason(runtime, WindowsLiveCaptureLocalStop))
		return ErrWindowsLiveCaptureInvalidStart
	}
	runtime.stream = stream
	s.encoder.Reset()
	s.sequence = 0
	s.queuedFrames = 0
	s.streamActive = true
	s.phase = WindowsLiveCaptureActive
	s.mu.Unlock()
	s.emit(WindowsLiveCaptureEvent{Kind: WindowsLiveCapturePhaseEvent, Phase: WindowsLiveCaptureActive})
	s.emit(WindowsLiveCaptureEvent{Kind: WindowsLiveCaptureStartCueEvent})
	go s.transportWorker(runtime)
	go s.captureWorker(runtime, format)
	if s.automaticWatch {
		go s.watchdogWorker(runtime)
	}
	return nil
}

func (s *WindowsLiveCaptureSender) LocalHoldEnded(generation uint64) {
	s.mu.Lock()
	valid := generation == s.localGeneration && s.phase != WindowsLiveCaptureIdle
	s.mu.Unlock()
	if valid {
		s.requestStop(WindowsLiveCaptureReleased)
	}
}

func (s *WindowsLiveCaptureSender) LocalStop()         { s.requestStop(WindowsLiveCaptureLocalStop) }
func (s *WindowsLiveCaptureSender) HandleSessionLock() { s.requestStop(WindowsLiveCaptureLock) }
func (s *WindowsLiveCaptureSender) HandleSuspend()     { s.requestStop(WindowsLiveCaptureSleep) }
func (s *WindowsLiveCaptureSender) HandlePermissionRevoke() {
	s.requestStop(WindowsLiveCapturePermissionLost)
}
func (s *WindowsLiveCaptureSender) HandleDeviceLoss() { s.requestStop(WindowsLiveCaptureDeviceLost) }
func (s *WindowsLiveCaptureSender) HandleDisconnect() { s.requestStop(WindowsLiveCaptureDisconnected) }
func (s *WindowsLiveCaptureSender) CoordinatorCancelled() {
	s.requestStop(WindowsLiveCaptureCoordinator)
}
func (s *WindowsLiveCaptureSender) Shutdown() { s.requestStop(WindowsLiveCaptureQuit) }

func (s *WindowsLiveCaptureSender) RetryOutbound() {
	s.mu.Lock()
	active := s.active
	s.mu.Unlock()
	if active != nil {
		select {
		case active.retryTransport <- struct{}{}:
		default:
		}
	}
}

func (s *WindowsLiveCaptureSender) RunWatchdogCheck() {
	s.mu.Lock()
	active := s.active
	if active == nil || s.phase == WindowsLiveCaptureIdle {
		s.mu.Unlock()
		return
	}
	now := s.coordinatorNowMS()
	reason := WindowsLiveCaptureStopReason("")
	if now-s.lastHeartbeatMS > windowsLiveHoldWatchdogMS {
		reason = WindowsLiveCaptureLostRelease
	} else if now > active.start.StartedAtCoordMS+active.start.MaxDurationMS {
		reason = WindowsLiveCaptureMaximum
	}
	s.mu.Unlock()
	if reason != "" {
		s.requestStop(reason)
	}
}

func (s *WindowsLiveCaptureSender) requestStop(reason WindowsLiveCaptureStopReason) {
	s.mu.Lock()
	if s.phase == WindowsLiveCaptureIdle {
		s.mu.Unlock()
		return
	}
	active := s.active
	if active == nil {
		s.localGeneration++
		s.phase = WindowsLiveCaptureIdle
		s.selectedDeviceID = ""
		s.mu.Unlock()
		s.emit(WindowsLiveCaptureEvent{Kind: WindowsLiveCaptureTerminalEvent, Reason: reason})
		s.emit(WindowsLiveCaptureEvent{Kind: WindowsLiveCapturePhaseEvent, Phase: WindowsLiveCaptureIdle})
		return
	}
	if active.stopReason == "" {
		active.stopReason = reason
		s.localGeneration++
	}
	s.phase = WindowsLiveCaptureStopping
	stream := active.stream
	active.cancel()
	s.mu.Unlock()
	s.emit(WindowsLiveCaptureEvent{Kind: WindowsLiveCapturePhaseEvent, Phase: WindowsLiveCaptureStopping})
	if stream != nil {
		_ = stream.Stop(windowsLiveStreamStopReason(reason))
	}
}

func (s *WindowsLiveCaptureSender) captureWorker(runtime *windowsLiveCaptureRuntime, format WindowsCaptureFormat) {
	input := make([]float32, windowsLiveCaptureReadFrames*int(format.Channels))
	// 8 kHz is the lowest accepted input rate: one full read expands to 12,288
	// samples, plus at most one incomplete 960-sample frame from the prior read.
	pcm := make([]float32, 0, windowsLiveCaptureReadFrames*7)
	packet := make([]byte, protocol.LivePTTMaxPayloadBytes)
	resampler := windowsLiveFloatResampler{inputRate: uint64(format.SampleRate)}
	baseUS := max(uint64(1), s.monotonicUS())
	var pending *protocol.LivePTTBinaryFrame
	reason := WindowsLiveCaptureDeviceLost

	for {
		frames, readErr := runtime.stream.Read(runtime.ctx, input)
		if frames > 0 {
			count := int(frames * format.Channels)
			if count > len(input) {
				reason = WindowsLiveCaptureDeviceLost
				break
			}
			var bounded bool
			pcm, bounded = resampler.append(pcm, input[:count], int(format.Channels))
			if !bounded {
				reason = WindowsLiveCaptureBackpressure
				break
			}
			for len(pcm) >= windowsLiveCaptureFrameSamples {
				if requested := s.runtimeReason(runtime, ""); requested != "" {
					reason = requested
					break
				}
				encoded, encodeErr := s.encoder.Encode(pcm[:windowsLiveCaptureFrameSamples], packet)
				if encodeErr != nil || encoded <= 0 || encoded > protocol.LivePTTMaxPayloadBytes {
					reason = WindowsLiveCaptureEncodeFailure
					break
				}
				sequence := s.nextSequence(runtime, encoded)
				if sequence == 0 {
					reason = WindowsLiveCaptureMaximum
					break
				}
				flags := byte(protocol.LivePTTFlagFEC)
				if sequence == 1 {
					flags |= protocol.LivePTTFlagStart
				}
				frame := protocol.LivePTTBinaryFrame{
					Flags: flags, SessionID: runtime.sessionID, Sequence: sequence,
					CaptureMonotonicUS: baseUS + uint64(sequence-1)*uint64(protocol.LivePTTFrameMS*1000),
					Payload:            append([]byte(nil), packet[:encoded]...),
				}
				if _, frameErr := protocol.EncodeLivePTTBinaryFrame(frame); frameErr != nil {
					reason = WindowsLiveCaptureEncodeFailure
					break
				}
				if pending != nil && !s.enqueueFrame(runtime, *pending) {
					reason = WindowsLiveCaptureBackpressure
					break
				}
				pending = &frame
				s.emit(WindowsLiveCaptureEvent{Kind: WindowsLiveCaptureMeterEvent, Meter: windowsLiveRMS(pcm[:windowsLiveCaptureFrameSamples])})
				copy(pcm, pcm[windowsLiveCaptureFrameSamples:])
				pcm = pcm[:len(pcm)-windowsLiveCaptureFrameSamples]
			}
			if reason == WindowsLiveCaptureBackpressure || reason == WindowsLiveCaptureEncodeFailure || reason == WindowsLiveCaptureMaximum {
				break
			}
		}
		if readErr != nil {
			reason = s.reasonFromRead(runtime, readErr)
			break
		}
	}
	if requested := s.runtimeReason(runtime, ""); requested != "" {
		reason = requested
	}
	s.finishCapture(runtime, reason, pending)
}

func (s *WindowsLiveCaptureSender) nextSequence(runtime *windowsLiveCaptureRuntime, encoded int) uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != runtime || s.phase == WindowsLiveCaptureIdle ||
		s.sequence >= uint32(protocol.LivePTTMaxDurationMS/protocol.LivePTTFrameMS) {
		return 0
	}
	s.sequence++
	s.encodedFrames++
	s.encodedBytes += uint64(encoded)
	return s.sequence
}

func (s *WindowsLiveCaptureSender) enqueueFrame(runtime *windowsLiveCaptureRuntime, frame protocol.LivePTTBinaryFrame) bool {
	select {
	case runtime.frames <- frame:
		s.mu.Lock()
		if s.active == runtime {
			s.queuedFrames = len(runtime.frames)
		}
		s.mu.Unlock()
		return true
	default:
		return false
	}
}

func (s *WindowsLiveCaptureSender) transportWorker(runtime *windowsLiveCaptureRuntime) {
	var pending *protocol.LivePTTBinaryFrame
	for {
		if pending == nil {
			select {
			case <-runtime.stopTransport:
				return
			case frame, ok := <-runtime.frames:
				if !ok {
					reason := s.sendTransportTerminal(runtime, false)
					s.completeCapture(runtime, reason)
					return
				}
				pending = &frame
			}
		}
		if s.trySendFrame(*pending) {
			pending = nil
			s.mu.Lock()
			if s.active == runtime {
				s.queuedFrames = len(runtime.frames)
			}
			s.mu.Unlock()
			continue
		}
		_, _, deadline := runtime.terminal()
		if !deadline.IsZero() && time.Now().After(deadline) {
			reason := s.sendTransportTerminal(runtime, true)
			s.completeCapture(runtime, reason)
			return
		}
		select {
		case <-runtime.stopTransport:
			return
		case <-runtime.retryTransport:
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func (s *WindowsLiveCaptureSender) finishCapture(runtime *windowsLiveCaptureRuntime, reason WindowsLiveCaptureStopReason, pending *protocol.LivePTTBinaryFrame) {
	_ = runtime.stream.Stop(windowsLiveStreamStopReason(reason))
	_ = runtime.stream.Close()
	runtime.cancel()

	sequence := s.sequenceFor(runtime)
	discard := windowsLiveDiscard(reason)
	if !discard && pending != nil {
		pending.Flags |= protocol.LivePTTFlagEnd
		if !s.enqueueFrame(runtime, *pending) {
			reason = WindowsLiveCaptureBackpressure
			discard = true
		}
	}
	if discard {
		runtime.stopTransportNow()
	} else {
		runtime.setTerminal(reason, sequence)
		close(runtime.frames)
	}

	emitStopping := false
	s.mu.Lock()
	if s.active == runtime {
		emitStopping = s.phase != WindowsLiveCaptureStopping
		s.phase = WindowsLiveCaptureStopping
		s.streamActive = false
	}
	s.encoder.Reset()
	s.mu.Unlock()
	if emitStopping {
		s.emit(WindowsLiveCaptureEvent{Kind: WindowsLiveCapturePhaseEvent, Phase: WindowsLiveCaptureStopping})
	}
	runtime.finishWorker()
	if discard {
		s.sendImmediateTerminal(runtime, reason, sequence)
		s.completeCapture(runtime, reason)
	}
}

func (s *WindowsLiveCaptureSender) finishStartFailure(runtime *windowsLiveCaptureRuntime, fallback WindowsLiveCaptureStopReason) {
	reason := s.runtimeReason(runtime, fallback)
	runtime.cancel()
	runtime.stopTransportNow()
	s.mu.Lock()
	if s.active == runtime {
		s.active = nil
		s.phase = WindowsLiveCaptureIdle
		s.selectedDeviceID = ""
		s.streamActive = false
		s.queuedFrames = 0
	}
	s.mu.Unlock()
	s.sendImmediateTerminal(runtime, reason, 0)
	runtime.finishWorker()
	s.emit(WindowsLiveCaptureEvent{Kind: WindowsLiveCaptureTerminalEvent, Reason: reason})
	s.emit(WindowsLiveCaptureEvent{Kind: WindowsLiveCapturePhaseEvent, Phase: WindowsLiveCaptureIdle})
}

func (s *WindowsLiveCaptureSender) sendTransportTerminal(runtime *windowsLiveCaptureRuntime, timedOut bool) WindowsLiveCaptureStopReason {
	reason, sequence, _ := runtime.terminal()
	if timedOut {
		reason = WindowsLiveCaptureBackpressure
	}
	s.sendImmediateTerminal(runtime, reason, sequence)
	return reason
}

func (s *WindowsLiveCaptureSender) completeCapture(runtime *windowsLiveCaptureRuntime, reason WindowsLiveCaptureStopReason) {
	s.mu.Lock()
	if s.active != runtime {
		s.mu.Unlock()
		return
	}
	s.active = nil
	s.phase = WindowsLiveCaptureIdle
	s.selectedDeviceID = ""
	s.streamActive = false
	s.queuedFrames = 0
	s.sequence = 0
	s.mu.Unlock()
	s.emit(WindowsLiveCaptureEvent{Kind: WindowsLiveCaptureStopCueEvent})
	s.emit(WindowsLiveCaptureEvent{Kind: WindowsLiveCaptureTerminalEvent, Reason: reason})
	s.emit(WindowsLiveCaptureEvent{Kind: WindowsLiveCapturePhaseEvent, Phase: WindowsLiveCaptureIdle})
}

func (s *WindowsLiveCaptureSender) sendImmediateTerminal(runtime *windowsLiveCaptureRuntime, reason WindowsLiveCaptureStopReason, sequence uint32) {
	if reason == WindowsLiveCaptureDisconnected || reason == WindowsLiveCaptureQuit || reason == WindowsLiveCaptureCoordinator {
		return
	}
	now := max(int64(1), s.coordinatorNowMS())
	if sequence > 0 && windowsLiveEndReason(reason) != "" {
		payload := protocol.LivePTTEndPayload{
			SessionID: runtime.start.SessionID, Generation: runtime.start.Generation,
			CommandSequence: 1, LastSequence: sequence, EndedAtCoordMS: now,
			DrainDeadlineCoordMS: now + protocol.LivePTTDrainTimeoutMS,
			Reason:               windowsLiveEndReason(reason),
		}
		if protocol.ValidateLivePTTEndPayload(payload) == nil {
			s.sendControl(protocol.TypeLivePTTEnd, payload)
		}
		return
	}
	if cancelReason := windowsLiveCancelReason(reason); cancelReason != "" {
		payload := protocol.LivePTTCancelPayload{
			SessionID: runtime.start.SessionID, Generation: runtime.start.Generation,
			CommandSequence: 1, CancelledAtCoordMS: now,
			Reason: cancelReason, DiscardBuffered: true,
		}
		if protocol.ValidateLivePTTCancelPayload(payload) == nil {
			s.sendControl(protocol.TypeLivePTTCancel, payload)
		}
		return
	}
	payload := protocol.LivePTTFailedPayload{
		SessionID: runtime.start.SessionID, Generation: runtime.start.Generation,
		EventSequence: 1, Stage: "capture", Code: string(reason), FailedAtCoordMS: now,
	}
	if protocol.ValidateLivePTTFailedPayload(payload) == nil {
		s.sendControl(protocol.TypeLivePTTFailed, payload)
	}
}

func (s *WindowsLiveCaptureSender) sequenceFor(runtime *windowsLiveCaptureRuntime) uint32 {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.active != runtime {
		return 0
	}
	return s.sequence
}

func (s *WindowsLiveCaptureSender) runtimeReason(runtime *windowsLiveCaptureRuntime, fallback WindowsLiveCaptureStopReason) WindowsLiveCaptureStopReason {
	s.mu.Lock()
	defer s.mu.Unlock()
	if runtime.stopReason != "" {
		return runtime.stopReason
	}
	return fallback
}

func (s *WindowsLiveCaptureSender) reasonFromRead(runtime *windowsLiveCaptureRuntime, err error) WindowsLiveCaptureStopReason {
	if reason := s.runtimeReason(runtime, ""); reason != "" {
		return reason
	}
	var terminal *WindowsCaptureTerminalError
	if errors.As(err, &terminal) {
		switch terminal.Reason {
		case WindowsCaptureSessionLock:
			return WindowsLiveCaptureLock
		case WindowsCaptureSuspend:
			return WindowsLiveCaptureSleep
		case WindowsCapturePermissionRevoke:
			return WindowsLiveCapturePermissionLost
		case WindowsCaptureDeviceLost:
			return WindowsLiveCaptureDeviceLost
		case WindowsCaptureQuit:
			return WindowsLiveCaptureQuit
		case WindowsCaptureOverflow:
			return WindowsLiveCaptureBackpressure
		}
	}
	return WindowsLiveCaptureDeviceLost
}

func (s *WindowsLiveCaptureSender) watchdogWorker(runtime *windowsLiveCaptureRuntime) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-runtime.done:
			return
		case <-ticker.C:
			s.RunWatchdogCheck()
		}
	}
}

func (s *WindowsLiveCaptureSender) emit(event WindowsLiveCaptureEvent) {
	if s.onEvent != nil {
		s.onEvent(event)
	}
}

type windowsLiveFloatResampler struct {
	inputRate uint64
	phase     uint64
}

func (r *windowsLiveFloatResampler) append(dst []float32, input []float32, channels int) ([]float32, bool) {
	if r.inputRate == 0 || channels <= 0 {
		return dst, false
	}
	for offset := 0; offset+channels <= len(input); offset += channels {
		var mono float64
		for channel := 0; channel < channels; channel++ {
			mono += float64(input[offset+channel])
		}
		mono = math.Max(-1, math.Min(1, mono/float64(channels)))
		r.phase += WindowsCaptureSampleRate
		for r.phase >= r.inputRate {
			r.phase -= r.inputRate
			if len(dst) == cap(dst) {
				return dst, false
			}
			dst = append(dst, float32(mono))
		}
	}
	return dst, true
}

func windowsLiveRMS(samples []float32) float32 {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, sample := range samples {
		sum += float64(sample * sample)
	}
	return float32(math.Sqrt(sum / float64(len(samples))))
}

func windowsLiveDiscard(reason WindowsLiveCaptureStopReason) bool {
	switch reason {
	case WindowsLiveCaptureReleased, WindowsLiveCaptureLostRelease, WindowsLiveCaptureLock,
		WindowsLiveCaptureSleep, WindowsLiveCapturePermissionLost, WindowsLiveCaptureDeviceLost:
		return false
	default:
		return true
	}
}

func windowsLiveEndReason(reason WindowsLiveCaptureStopReason) string {
	switch reason {
	case WindowsLiveCaptureReleased:
		return "release"
	case WindowsLiveCaptureLostRelease:
		return "lost_release"
	case WindowsLiveCaptureLock:
		return "lock"
	case WindowsLiveCaptureSleep:
		return "sleep"
	case WindowsLiveCapturePermissionLost:
		return "permission_revoked"
	case WindowsLiveCaptureDeviceLost:
		return "device_lost"
	}
	return ""
}

func windowsLiveCancelReason(reason WindowsLiveCaptureStopReason) string {
	switch reason {
	case WindowsLiveCaptureReleased:
		return "user_cancel"
	case WindowsLiveCaptureBackpressure:
		return "backpressure"
	case WindowsLiveCaptureLostRelease:
		return "lost_release"
	case WindowsLiveCaptureMaximum:
		return "timeout"
	case WindowsLiveCaptureLocalStop:
		return "user_cancel"
	}
	return ""
}

func windowsLiveStreamStopReason(reason WindowsLiveCaptureStopReason) WindowsCaptureStopReason {
	switch reason {
	case WindowsLiveCaptureLock:
		return WindowsCaptureSessionLock
	case WindowsLiveCaptureSleep:
		return WindowsCaptureSuspend
	case WindowsLiveCapturePermissionLost:
		return WindowsCapturePermissionRevoke
	case WindowsLiveCaptureDeviceLost:
		return WindowsCaptureDeviceLost
	case WindowsLiveCaptureQuit:
		return WindowsCaptureQuit
	case WindowsLiveCaptureBackpressure:
		return WindowsCaptureOverflow
	default:
		return WindowsCaptureCancel
	}
}
