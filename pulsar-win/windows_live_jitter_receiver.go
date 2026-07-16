package main

import (
	"bytes"
	"errors"
	"sync"
	"time"

	protocol "relux.works/duet/pulsar-win/wire"
)

var (
	ErrWindowsLiveInvalidPacket  = errors.New("invalid live Opus packet")
	ErrWindowsLiveFECUnavailable = errors.New("live Opus FEC unavailable")
)

type WindowsLiveOpusDecoder interface {
	Decode(packet []byte, fec bool, output []float32) (int, error)
	Reset()
}

type WindowsLiveAudioRoute interface {
	PrepareLivePCM() int64
	ActivateLivePCM(generation int64, audibleStarted func()) bool
	WriteLivePCM(generation int64, samples []float32) int
	StopLivePCM(generation int64, discard bool) bool
	LivePCMCapacityFrames() int
	LivePCMBufferedFrames() int
	LivePCMUnderrunCallbacks() int64
}

type WindowsLiveJitterPhase string

const (
	WindowsLiveIdle            WindowsLiveJitterPhase = "idle"
	WindowsLiveBuffering       WindowsLiveJitterPhase = "buffering"
	WindowsLivePlaying         WindowsLiveJitterPhase = "playing"
	WindowsLiveDraining        WindowsLiveJitterPhase = "draining"
	windowsLivePacketWindow                           = int(protocol.LivePTTMaxGapFrames) + 1
	windowsLiveMaxConcealments                        = 8
)

type WindowsLiveJitterSnapshot struct {
	Phase             WindowsLiveJitterPhase
	SessionID         string
	Generation        int64
	ExpectedSequence  uint32
	HighestSequence   uint32
	EncodedFrames     int
	EncodedBytes      int
	PCMFrames         int
	PCMCapacityFrames int
	ReceivedFrames    int
	DecodedFrames     int
	DuplicateFrames   int
	LateFrames        int
	FECFrames         int
	PLCFrames         int
	FailedFrames      int
	UnderrunCallbacks int64
}

type windowsLiveJitterSession struct {
	start                   protocol.LivePTTStartPayload
	sessionID               [16]byte
	routeGeneration         int64
	phase                   WindowsLiveJitterPhase
	packets                 map[uint32]protocol.LivePTTBinaryFrame
	expectedSequence        uint32
	highestSequence         uint32
	captureBaseUS           uint64
	lastCommandSequence     int64
	endSequence             uint32
	hasEnd                  bool
	drainDeadlineMS         int64
	eventSequence           int64
	receivedFrames          int
	decodedFrames           int
	duplicateFrames         int
	lateFrames              int
	fecFrames               int
	plcFrames               int
	failedFrames            int
	consecutiveConcealments int
	audibleStarted          bool
}

// WindowsLiveJitterReceiver owns packet validation, reorder, decode and PCM
// production on a mutex-serialized worker boundary. The WASAPI callback sees
// only Engine's fixed ring and envelopes. No production constructor is exposed
// until the reviewed libopus binary path supplies this decoder interface.
type WindowsLiveJitterReceiver struct {
	mu                sync.Mutex
	route             WindowsLiveAudioRoute
	decoder           WindowsLiveOpusDecoder
	coordinatorNowMS  func() int64
	send              func(string, any)
	automaticTick     bool
	session           *windowsLiveJitterSession
	highestGeneration int64
	decodeScratch     [liveFrameInput]float32
	lastPCM           [liveFrameInput]float32
	hasLastPCM        bool
	timerStop         chan struct{}
	timerGeneration   int64
}

func NewWindowsLiveJitterReceiver(
	route WindowsLiveAudioRoute,
	decoder WindowsLiveOpusDecoder,
	automaticTick bool,
	coordinatorNowMS func() int64,
	send func(string, any),
) *WindowsLiveJitterReceiver {
	if route == nil || decoder == nil || coordinatorNowMS == nil || send == nil {
		return nil
	}
	return &WindowsLiveJitterReceiver{
		route: route, decoder: decoder, automaticTick: automaticTick,
		coordinatorNowMS: coordinatorNowMS, send: send,
	}
}

func (r *WindowsLiveJitterReceiver) Start(payload protocol.LivePTTStartPayload, authorized bool) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := r.coordinatorNowMS()
	reject := ""
	if protocol.ValidateLivePTTStartPayload(payload) != nil {
		reject = "unsupported"
	} else if !authorized {
		reject = "unauthorized"
	} else if r.session != nil {
		reject = "busy"
	} else if payload.Generation <= r.highestGeneration || now > payload.AcceptDeadlineCoordMS {
		reject = "expired"
	} else if r.route.LivePCMCapacityFrames() < liveFrameInput*4 {
		reject = "unsupported"
	}
	if reject != "" {
		r.send(protocol.TypeLivePTTReject, protocol.LivePTTRejectPayload{
			SessionID: payload.SessionID, Generation: payload.Generation,
			EventSequence: 1, Code: reject, RejectedAtCoordMS: max(int64(1), now),
		})
		return false
	}
	sessionID, err := protocol.ParseLivePTTSessionID(payload.SessionID)
	if err != nil {
		return false
	}
	r.highestGeneration = payload.Generation
	r.decoder.Reset()
	r.hasLastPCM = false
	r.session = &windowsLiveJitterSession{
		start: payload, sessionID: sessionID,
		routeGeneration: r.route.PrepareLivePCM(), phase: WindowsLiveBuffering,
		packets:          make(map[uint32]protocol.LivePTTBinaryFrame, windowsLivePacketWindow),
		expectedSequence: 1, eventSequence: 1,
	}
	r.send(protocol.TypeLivePTTAccept, protocol.LivePTTAcceptPayload{
		SessionID: payload.SessionID, Generation: payload.Generation,
		EventSequence: 1, AcceptedAtCoordMS: max(int64(1), now),
		LiveEdgeSequence: 1, BufferFrames: 3,
	})
	r.armTimerLocked()
	return true
}

func (r *WindowsLiveJitterReceiver) Receive(frame protocol.LivePTTBinaryFrame) protocol.LivePTTFrameDecision {
	r.mu.Lock()
	defer r.mu.Unlock()
	active := r.session
	if active == nil || frame.SessionID != active.sessionID {
		return protocol.LivePTTFrameStale
	}
	if _, err := protocol.EncodeLivePTTBinaryFrame(frame); err != nil {
		return protocol.LivePTTFrameInvalid
	}
	if frame.Sequence < active.expectedSequence {
		active.lateFrames++
		return protocol.LivePTTFrameStale
	}
	if existing, ok := active.packets[frame.Sequence]; ok {
		if windowsLiveFramesEqual(existing, frame) {
			active.duplicateFrames++
			return protocol.LivePTTFrameDuplicate
		}
		active.lateFrames++
		return protocol.LivePTTFrameStale
	}
	if frame.Sequence > uint32(protocol.LivePTTMaxDurationMS/protocol.LivePTTFrameMS) ||
		int(frame.Sequence-active.expectedSequence) >= windowsLivePacketWindow ||
		len(active.packets) >= windowsLivePacketWindow {
		return protocol.LivePTTFrameInvalid
	}
	if frame.Sequence == 1 {
		if active.captureBaseUS != 0 {
			return protocol.LivePTTFrameInvalid
		}
		active.captureBaseUS = frame.CaptureMonotonicUS
	}
	if active.captureBaseUS == 0 || frame.CaptureMonotonicUS !=
		active.captureBaseUS+uint64(frame.Sequence-1)*uint64(protocol.LivePTTFrameMS*1000) {
		return protocol.LivePTTFrameInvalid
	}
	frame.Payload = append([]byte(nil), frame.Payload...)
	active.packets[frame.Sequence] = frame
	active.highestSequence = max(active.highestSequence, frame.Sequence)
	active.receivedFrames++
	if active.phase == WindowsLiveBuffering && active.highestSequence >= 3 {
		r.prebufferLocked()
	}
	return protocol.LivePTTFrameApply
}

func (r *WindowsLiveJitterReceiver) End(payload protocol.LivePTTEndPayload) {
	r.mu.Lock()
	defer r.mu.Unlock()
	active := r.session
	if active == nil || payload.SessionID != active.start.SessionID ||
		payload.Generation != active.start.Generation ||
		payload.CommandSequence <= active.lastCommandSequence ||
		payload.LastSequence < active.expectedSequence-1 ||
		protocol.ValidateLivePTTEndPayload(payload) != nil {
		return
	}
	active.lastCommandSequence = payload.CommandSequence
	active.endSequence = payload.LastSequence
	active.hasEnd = true
	active.drainDeadlineMS = payload.DrainDeadlineCoordMS
	active.phase = WindowsLiveDraining
	r.tickLocked(r.coordinatorNowMS())
}

func (r *WindowsLiveJitterReceiver) Cancel(payload protocol.LivePTTCancelPayload) {
	r.mu.Lock()
	defer r.mu.Unlock()
	active := r.session
	if active == nil || payload.SessionID != active.start.SessionID ||
		payload.Generation != active.start.Generation ||
		payload.CommandSequence <= active.lastCommandSequence ||
		protocol.ValidateLivePTTCancelPayload(payload) != nil {
		return
	}
	r.terminateLocked("cancelled", true)
}

func (r *WindowsLiveJitterReceiver) Revoke() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.session != nil {
		r.terminateLocked("cancelled", true)
	}
}

func (r *WindowsLiveJitterReceiver) Close() { r.Revoke() }

func (r *WindowsLiveJitterReceiver) Tick() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tickLocked(r.coordinatorNowMS())
}

func (r *WindowsLiveJitterReceiver) Snapshot() WindowsLiveJitterSnapshot {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := WindowsLiveJitterSnapshot{
		Phase: WindowsLiveIdle, PCMFrames: r.route.LivePCMBufferedFrames(),
		PCMCapacityFrames: r.route.LivePCMCapacityFrames(),
		UnderrunCallbacks: r.route.LivePCMUnderrunCallbacks(),
	}
	active := r.session
	if active == nil {
		return result
	}
	result.Phase, result.SessionID, result.Generation = active.phase, active.start.SessionID, active.start.Generation
	result.ExpectedSequence, result.HighestSequence = active.expectedSequence, active.highestSequence
	result.EncodedFrames = len(active.packets)
	for _, frame := range active.packets {
		result.EncodedBytes += len(frame.Payload)
	}
	result.ReceivedFrames, result.DecodedFrames = active.receivedFrames, active.decodedFrames
	result.DuplicateFrames, result.LateFrames = active.duplicateFrames, active.lateFrames
	result.FECFrames, result.PLCFrames = active.fecFrames, active.plcFrames
	result.FailedFrames = active.failedFrames
	return result
}

func (r *WindowsLiveJitterReceiver) prebufferLocked() {
	active := r.session
	if active == nil || active.phase != WindowsLiveBuffering {
		return
	}
	for range 3 {
		if !r.decodeExpectedLocked(false) {
			return
		}
	}
	active = r.session
	if active == nil {
		return
	}
	routeGeneration := active.routeGeneration
	if !r.route.ActivateLivePCM(routeGeneration, func() {
		r.markAudibleStarted(routeGeneration)
	}) {
		r.failLocked("render", "activation_failed")
		return
	}
	active.phase = WindowsLivePlaying
}

func (r *WindowsLiveJitterReceiver) markAudibleStarted(routeGeneration int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	active := r.session
	if active == nil || active.routeGeneration != routeGeneration || active.audibleStarted {
		return
	}
	active.audibleStarted = true
	active.eventSequence++
	r.send(protocol.TypeLivePTTReceipt, protocol.LivePTTReceiptPayload{
		SessionID: active.start.SessionID, Generation: active.start.Generation,
		EventSequence: active.eventSequence, State: "audible_started",
		LastSequence:      active.expectedSequence - 1,
		ObservedAtCoordMS: max(int64(1), r.coordinatorNowMS()),
	})
}

func (r *WindowsLiveJitterReceiver) tickLocked(now int64) {
	active := r.session
	if active == nil {
		return
	}
	if now > active.start.StartedAtCoordMS+active.start.MaxDurationMS {
		r.failLocked("render", "max_duration")
		return
	}
	if active.phase == WindowsLivePlaying || active.phase == WindowsLiveDraining {
		if !active.hasEnd || active.expectedSequence <= active.endSequence {
			r.decodeExpectedLocked(true)
		}
	}
	active = r.session
	if active == nil || active.phase != WindowsLiveDraining {
		return
	}
	if now >= active.drainDeadlineMS {
		r.terminateLocked("ended", true)
	} else if active.expectedSequence > active.endSequence && r.route.LivePCMBufferedFrames() == 0 {
		r.terminateLocked("ended", false)
	}
}

func (r *WindowsLiveJitterReceiver) decodeExpectedLocked(allowUnconfirmedGap bool) bool {
	active := r.session
	if active == nil {
		return false
	}
	sequence := active.expectedSequence
	concealed := false
	if frame, ok := active.packets[sequence]; ok {
		delete(active.packets, sequence)
		count, err := r.decoder.Decode(frame.Payload, false, r.decodeScratch[:])
		if err != nil || count != liveFrameInput {
			active.failedFrames++
			r.failLocked("decode", "decode_failed")
			return false
		}
		active.consecutiveConcealments = 0
	} else if next, ok := active.packets[sequence+1]; ok {
		count, err := r.decoder.Decode(next.Payload, true, r.decodeScratch[:])
		if err == nil && count == liveFrameInput {
			active.fecFrames++
			concealed = true
		} else if errors.Is(err, ErrWindowsLiveFECUnavailable) {
			r.concealLocked(active.consecutiveConcealments)
			active.plcFrames++
			concealed = true
		} else {
			active.failedFrames++
			r.failLocked("decode", "decode_failed")
			return false
		}
	} else {
		if active.highestSequence < sequence && !active.hasEnd && !allowUnconfirmedGap {
			return false
		}
		r.concealLocked(active.consecutiveConcealments)
		active.plcFrames++
		concealed = true
	}
	if concealed {
		active.consecutiveConcealments++
	}
	if active.consecutiveConcealments > windowsLiveMaxConcealments {
		active.failedFrames++
		r.failLocked("jitter", "concealment_exhausted")
		return false
	}
	if r.route.WriteLivePCM(active.routeGeneration, r.decodeScratch[:]) != liveFrameInput {
		active.failedFrames++
		r.failLocked("render", "buffer_full")
		return false
	}
	copy(r.lastPCM[:], r.decodeScratch[:])
	r.hasLastPCM = true
	active.decodedFrames++
	active.expectedSequence++
	return true
}

func (r *WindowsLiveJitterReceiver) concealLocked(consecutive int) {
	if !r.hasLastPCM {
		clear(r.decodeScratch[:])
		return
	}
	attenuation := float32(1)
	for range consecutive + 1 {
		attenuation *= 0.86
	}
	if attenuation < 0.2 {
		attenuation = 0.2
	}
	for index, value := range r.lastPCM {
		edge := min(float32(index)/96, float32(len(r.lastPCM)-index)/96, float32(1))
		r.decodeScratch[index] = value * attenuation * edge
	}
}

func (r *WindowsLiveJitterReceiver) failLocked(stage, code string) {
	active := r.session
	if active == nil {
		return
	}
	active.eventSequence++
	r.send(protocol.TypeLivePTTFailed, protocol.LivePTTFailedPayload{
		SessionID: active.start.SessionID, Generation: active.start.Generation,
		EventSequence: active.eventSequence, Stage: stage, Code: code,
		FailedAtCoordMS: max(int64(1), r.coordinatorNowMS()),
	})
	r.terminateLocked("failed", true)
}

func (r *WindowsLiveJitterReceiver) terminateLocked(state string, discard bool) {
	active := r.session
	if active == nil {
		return
	}
	active.eventSequence++
	r.route.StopLivePCM(active.routeGeneration, discard)
	r.send(protocol.TypeLivePTTReceipt, protocol.LivePTTReceiptPayload{
		SessionID: active.start.SessionID, Generation: active.start.Generation,
		EventSequence: active.eventSequence, State: state,
		LastSequence:      max(uint32(0), active.expectedSequence-1),
		ObservedAtCoordMS: max(int64(1), r.coordinatorNowMS()),
	})
	r.session = nil
	r.stopTimerLocked()
	r.decoder.Reset()
	r.hasLastPCM = false
}

func (r *WindowsLiveJitterReceiver) armTimerLocked() {
	if !r.automaticTick {
		return
	}
	r.stopTimerLocked()
	stop := make(chan struct{})
	r.timerStop = stop
	generation := r.timerGeneration
	go func() {
		ticker := time.NewTicker(protocol.LivePTTFrameMS * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				r.tickFromTimer(generation)
			case <-stop:
				return
			}
		}
	}()
}

func (r *WindowsLiveJitterReceiver) stopTimerLocked() {
	r.timerGeneration++
	if r.timerStop != nil {
		close(r.timerStop)
		r.timerStop = nil
	}
}

func (r *WindowsLiveJitterReceiver) tickFromTimer(generation int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if generation == r.timerGeneration {
		r.tickLocked(r.coordinatorNowMS())
	}
}

func windowsLiveFramesEqual(left, right protocol.LivePTTBinaryFrame) bool {
	return left.Flags == right.Flags && left.SessionID == right.SessionID &&
		left.Sequence == right.Sequence && left.CaptureMonotonicUS == right.CaptureMonotonicUS &&
		bytes.Equal(left.Payload, right.Payload)
}
