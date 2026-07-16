// Engine: the portable render mix — the pulsar-win equivalent of the macOS
// AVAudioEngine graph (AudioEngine.swift), pulled per callback by the WASAPI
// render loop (audio_windows.go):
//
//	music ring -> music fade -> overlay duck \
//	media overlay (additive) + click cues     +-> limiter -> master gain -> dst
//	media interrupt (replacement at T, exact resume handshake)
//	legacy voice (REPLACES music, separate compatibility path)
//
// Semantics ported from the mac node:
//   - Voice REPLACES music: while a voice insert is sounding the music ring
//     is not consumed (the coordinator paused the daemon anyway; the ring
//     tail stays for the resume) — spec 6.3 play_voice.
//   - offset_test clicks are ADDED on top (the AVAudioPlayerNode branch).
//   - Media overlay continuously consumes music, begins ducking 250 ms before
//     clip start, and applies a post-mix ceiling before local master gain.
//   - Master amplitude ((v/100)^2, glided) scales everything, like the
//     mainMixer output volume did.
//   - Dropout telemetry: fed vs starved callbacks, starved streak gated on
//     expectingMusic (UNRESOLVED R4: idle silence is not an underrun).
package main

import (
	"math"
	"sync"
	"sync/atomic"
	"time"
)

// clickDurationMS/clickFreqHz: offset_test bursts are 30 ms of 1 kHz sine —
// sharp, easy to compare by ear across nodes.
const (
	clickDurationMS = 30
	clickFreqHz     = 1000
	clickAmplitude  = 0.9
	liveInputRate   = 48000
	liveFrameInput  = 960
	liveFrameOutput = sampleRate * livePTTFrameDurationMS / 1000
	livePCMFrames   = liveInputRate * 320 / 1000
	liveDuckDB      = -12.0
	liveLimiterDB   = -1.0
)

// livePTTFrameDurationMS stays local to the render package so the WASAPI
// boundary does not import protocol code. The wire mirror pins the same 20 ms.
const livePTTFrameDurationMS = 20

type voiceState struct {
	samples []float32 // interleaved stereo 44.1k
	cursor  int
	startAt time.Time // zero = start on the next render pull
	onDone  func()    // dispatched off-render after the last sample rendered
}

type clickSchedule struct {
	starts []time.Time
}

type overlayCancelRequest struct {
	fadeMS int64
	done   func()
}

type overlayState struct {
	samples   []float32
	cursor    int
	startAt   time.Time
	preDuckAt time.Time
	control   MixerControlParameters
	onStarted func(int64)
	onEnded   func(int64)
	cancel    atomic.Pointer[overlayCancelRequest]

	started      bool
	ended        bool
	cancelling   bool
	cancelActive *overlayCancelRequest
	duckStarted  bool
	duckCurrent  float32
	duckStart    float32
	duckTarget   float32
	duckTotal    int
	duckDone     int
	overlayGain  float32
	overlayStart float32
	overlayTotal int
	overlayDone  int
}

type interruptCancelRequest struct {
	fadeMS int64
	ready  func()
}

type interruptState struct {
	samples   []float32
	cursor    int
	startAt   time.Time
	preFadeAt time.Time
	control   MixerControlParameters
	onStarted func(int64)
	onEnded   func(int64)
	cancel    atomic.Pointer[interruptCancelRequest]

	started         bool
	ended           bool
	cancelling      bool
	resumeRequested bool
	cancelActive    *interruptCancelRequest
	mainFadeStarted bool
	mainGain        float32
	mainStart       float32
	mainTarget      float32
	mainTotal       int
	mainDone        int
	clipGain        float32
	clipStart       float32
	clipTotal       int
	clipDone        int
}

type renderCompletionKind uint8

const (
	renderVoiceDone renderCompletionKind = iota
	renderOverlayStarted
	renderOverlayEnded
	renderOverlayCancelled
	renderInterruptStarted
	renderInterruptEnded
	renderInterruptCancelReady
	renderLiveStarted
)

type renderCompletion struct {
	kind            renderCompletionKind
	voice           *voiceState
	overlay         *overlayState
	cancel          *overlayCancelRequest
	interrupt       *interruptState
	interruptCancel *interruptCancelRequest
	liveStarted     func()
	localMS         int64
}

type liveStopRequest struct{ discard bool }

// Control publishes only atomics after construction. Gain, duck and cursor
// fields are owned by the single WASAPI render consumer.
type windowsLiveRenderState struct {
	generation int64
	ring       *Ring
	active     atomic.Bool
	stop       atomic.Pointer[liveStopRequest]

	activated  bool
	stopping   bool
	liveGain   float32
	liveStart  float32
	liveTarget float32
	liveTotal  int
	liveDone   int
	duckGain   float32
	duckStart  float32
	duckTarget float32
	duckTotal  int
	duckDone   int
	lastLeft   float32
	lastRight  float32
	onStarted  func()
	started    bool
}

// RenderStats is a snapshot of the dropout telemetry counters.
type RenderStats struct {
	Fed             int64 // callbacks fully served from the music ring
	Starved         int64 // callbacks that zero-filled a shortfall
	StarvedStreak   int64 // consecutive starved callbacks while expecting music
	OverlayFrames   int64 // clip frames mixed into the output
	InterruptFrames int64 // replacement clip frames rendered
	LimiterHits     int64 // samples constrained by the post-mix ceiling
	LiveFrames      int64 // live frames mixed before the common limiter
	LiveUnderruns   int64 // callbacks that exhausted the bounded live PCM ring
}

type Engine struct {
	music *Ring
	gain  *Gain
	now   func() time.Time // injectable for click/voice scheduling tests

	voice             atomic.Pointer[voiceState]
	overlay           atomic.Pointer[overlayState]
	interrupt         atomic.Pointer[interruptState]
	mediaBusy         atomic.Bool
	clicks            atomic.Pointer[clickSchedule]
	clickBurst        []float32 // precomputed mono burst
	expectingMusic    atomic.Bool
	fed               atomic.Int64
	starved           atomic.Int64
	starvedStreak     atomic.Int64
	overlayFrames     atomic.Int64
	interruptFrames   atomic.Int64
	limiterHits       atomic.Int64
	liveFrames        atomic.Int64
	liveUnderruns     atomic.Int64
	liveEpoch         atomic.Int64
	live              atomic.Pointer[windowsLiveRenderState]
	liveWriteScratch  [liveFrameOutput * channels]float32
	liveRenderScratch [2048]float32
	completions       chan renderCompletion
	done              chan struct{}
	closeOnce         sync.Once
}

func NewEngine(music *Ring, gain *Gain) *Engine {
	burst := make([]float32, sampleRate*clickDurationMS/1000)
	for i := range burst {
		burst[i] = clickAmplitude * float32(math.Sin(2*math.Pi*clickFreqHz*float64(i)/sampleRate))
	}
	e := &Engine{
		music: music, gain: gain, now: time.Now, clickBurst: burst,
		completions: make(chan renderCompletion, 16),
		done:        make(chan struct{}),
	}
	go e.dispatchDoneCallbacks()
	return e
}

func (e *Engine) dispatchDoneCallbacks() {
	for {
		select {
		case completion := <-e.completions:
			switch completion.kind {
			case renderVoiceDone:
				if completion.voice.onDone != nil {
					completion.voice.onDone()
				}
			case renderOverlayStarted:
				completion.overlay.onStarted(completion.localMS)
			case renderOverlayEnded:
				completion.overlay.onEnded(completion.localMS)
			case renderOverlayCancelled:
				completion.cancel.done()
			case renderInterruptStarted:
				completion.interrupt.onStarted(completion.localMS)
			case renderInterruptEnded:
				completion.interrupt.onEnded(completion.localMS)
			case renderInterruptCancelReady:
				completion.interruptCancel.ready()
			case renderLiveStarted:
				completion.liveStarted()
			}
		case <-e.done:
			return
		}
	}
}

func (e *Engine) postCompletion(completion renderCompletion) {
	select {
	case e.completions <- completion:
	default:
		// The queue is fixed and intentionally non-blocking. A full queue means
		// the control consumer is stalled; audio timing always wins.
	}
}

// Close stops the pre-created completion dispatcher. The render callback
// never creates goroutines or waits for completion delivery.
func (e *Engine) Close() {
	e.closeOnce.Do(func() { close(e.done) })
}

// SetExpectingMusic gates the starved-streak counter: silence while stopped
// or paused is idle, not an underrun (UNRESOLVED R4).
func (e *Engine) SetExpectingMusic(v bool) {
	e.expectingMusic.Store(v)
	if !v {
		e.starvedStreak.Store(0)
	}
}

// PlayVoice replaces music with the decoded voice samples starting at startAt
// (zero time = immediately). onDone fires once after the last sample rendered
// — that IS the audible end, so voice_ended needs no extra drain wait.
// A newer voice replaces a pending one (its onDone is dropped, mirroring the
// mac player node which just schedules over).
func (e *Engine) PlayVoice(samples []float32, startAt time.Time, onDone func()) {
	e.voice.Store(&voiceState{samples: samples, startAt: startAt, onDone: onDone})
}

// StopVoice drops any active or pending voice insert without firing onDone.
func (e *Engine) StopVoice() {
	e.voice.Store(nil)
}

// VoiceActive reports whether a voice insert is pending or sounding.
func (e *Engine) VoiceActive() bool {
	return e.voice.Load() != nil
}

// PlayClicks schedules count clicks, the first at firstAt, then every
// intervalMS (offset_test, spec 7.3).
func (e *Engine) PlayClicks(count int, firstAt time.Time, intervalMS int64) {
	if count <= 0 {
		return
	}
	previous := e.clicks.Load()
	previousCount := 0
	if previous != nil {
		previousCount = len(previous.starts)
	}
	starts := make([]time.Time, previousCount+count)
	if previous != nil {
		copy(starts, previous.starts)
	}
	for k := 0; k < count; k++ {
		starts[previousCount+k] = firstAt.Add(time.Duration(intervalMS*int64(k)) * time.Millisecond)
	}
	e.clicks.Store(&clickSchedule{starts: starts})
}

// ScheduledClickCount exposes the immutable control snapshot for tests and
// diagnostics without reaching into render-owned state.
func (e *Engine) ScheduledClickCount() int {
	schedule := e.clicks.Load()
	if schedule == nil {
		return 0
	}
	return len(schedule.starts)
}

// Stats returns the telemetry counters snapshot.
func (e *Engine) Stats() RenderStats {
	return RenderStats{
		Fed: e.fed.Load(), Starved: e.starved.Load(), StarvedStreak: e.starvedStreak.Load(),
		OverlayFrames: e.overlayFrames.Load(), InterruptFrames: e.interruptFrames.Load(),
		LimiterHits: e.limiterHits.Load(), LiveFrames: e.liveFrames.Load(),
		LiveUnderruns: e.liveUnderruns.Load(),
	}
}

// PrepareLivePCM creates a generation-isolated 320 ms ring. A replacement
// raises the epoch before publication so a callback holding the old pointer
// cannot consume or mix samples from the new generation.
func (e *Engine) PrepareLivePCM() int64 {
	generation := e.liveEpoch.Add(1)
	state := &windowsLiveRenderState{
		generation: generation,
		ring:       NewRing(livePCMFrames * sampleRate * channels / liveInputRate),
		duckGain:   1, duckStart: 1, duckTarget: dbAmplitude(liveDuckDB),
		duckTotal: sampleRate * 60 / 1000,
	}
	if previous := e.live.Swap(state); previous != nil {
		previous.ring.Clear()
	}
	return generation
}

func (e *Engine) ActivateLivePCM(generation int64, audibleStarted func()) bool {
	state := e.live.Load()
	if state == nil || state.generation != generation || state.stop.Load() != nil || audibleStarted == nil {
		return false
	}
	state.onStarted = audibleStarted
	state.active.Store(true)
	return true
}

// WriteLivePCM resamples one 48 kHz mono/20 ms decoder frame into the fixed
// 44.1 kHz stereo render format. It is called by the receiver worker only.
func (e *Engine) WriteLivePCM(generation int64, samples []float32) int {
	state := e.live.Load()
	if state == nil || state.generation != generation || e.liveEpoch.Load() != generation ||
		state.stop.Load() != nil || len(samples) != liveFrameInput {
		return 0
	}
	for frame := 0; frame < liveFrameOutput; frame++ {
		position := float64(frame) * liveInputRate / sampleRate
		lower := int(position)
		fraction := float32(position - float64(lower))
		value := samples[lower]
		if lower+1 < len(samples) {
			value += (samples[lower+1] - value) * fraction
		}
		e.liveWriteScratch[frame*channels] = value
		e.liveWriteScratch[frame*channels+1] = value
	}
	output := e.liveWriteScratch[:]
	if state.ring.Capacity()-state.ring.Fill() < len(output) || state.ring.Write(output) != len(output) {
		return 0
	}
	return len(samples)
}

func (e *Engine) StopLivePCM(generation int64, discard bool) bool {
	state := e.live.Load()
	if state == nil || state.generation != generation {
		return false
	}
	request := &liveStopRequest{discard: discard}
	if !state.stop.CompareAndSwap(nil, request) {
		return false
	}
	if discard {
		state.ring.Clear()
	}
	return true
}

func (e *Engine) LivePCMCapacityFrames() int { return livePCMFrames }

func (e *Engine) LivePCMBufferedFrames() int {
	state := e.live.Load()
	if state == nil {
		return 0
	}
	return state.ring.Fill() / channels * liveInputRate / sampleRate
}

func (e *Engine) LivePCMUnderrunCallbacks() int64 { return e.liveUnderruns.Load() }
func (e *Engine) LiveRenderActive() bool          { return e.live.Load() != nil }

// ArmOverlay publishes a fully decoded immutable clip to the render consumer.
// Only the render callback mutates cursors and ramps after this CAS succeeds.
func (e *Engine) ArmOverlay(samples []float32, plan MediaClipPlayPlan, started, ended func(int64)) (*overlayState, error) {
	if len(samples) < channels || len(samples)%channels != 0 || plan.Control.Delivery != "overlay" ||
		plan.Control.Interrupt || started == nil || ended == nil {
		return nil, mediaClipFailure("audio_graph_failed")
	}
	startAt := time.UnixMilli(plan.LocalStartMS)
	state := &overlayState{
		samples: samples, startAt: startAt, preDuckAt: startAt.Add(-250 * time.Millisecond),
		control: plan.Control, onStarted: started, onEnded: ended,
		duckCurrent: 1, duckStart: 1, duckTarget: 1, overlayGain: 1, overlayStart: 1,
	}
	if !e.mediaBusy.CompareAndSwap(false, true) {
		return nil, mediaClipFailure("audio_graph_failed")
	}
	e.overlay.Store(state)
	return state, nil
}

// CancelOverlay posts a fixed cancellation request. The render callback owns
// the fade/release and acknowledges only after ducking is fully restored.
func (e *Engine) CancelOverlay(state *overlayState, fadeMS int64, done func()) bool {
	if state == nil || done == nil || e.overlay.Load() != state {
		return false
	}
	request := &overlayCancelRequest{fadeMS: max(fadeMS, 0), done: done}
	return state.cancel.CompareAndSwap(nil, request)
}

func (e *Engine) OverlayActive() bool { return e.overlay.Load() != nil }

// ArmInterrupt publishes a replacement clip. The render consumer owns the
// T-minus-fade envelope and stops consuming the main ring at the exact clip
// boundary. Control callbacks suspend and restore the provider off-render.
func (e *Engine) ArmInterrupt(
	samples []float32,
	plan MediaClipPlayPlan,
	started, ended func(int64),
) (*interruptState, error) {
	if len(samples) < channels || len(samples)%channels != 0 ||
		plan.Control.Delivery != "interrupt" || !plan.Control.Interrupt ||
		started == nil || ended == nil {
		return nil, mediaClipFailure("interrupt_capability_lost")
	}
	if !e.mediaBusy.CompareAndSwap(false, true) {
		return nil, mediaClipFailure("audio_graph_failed")
	}
	startAt := time.UnixMilli(plan.LocalStartMS)
	state := &interruptState{
		samples: samples, startAt: startAt,
		preFadeAt: startAt.Add(-time.Duration(plan.Control.FadeOutMS) * time.Millisecond),
		control:   plan.Control, onStarted: started, onEnded: ended,
		mainGain: 1, mainStart: 1, mainTarget: 1, clipGain: 1, clipStart: 1,
	}
	e.interrupt.Store(state)
	return state, nil
}

// CancelInterrupt fades an already sounding replacement clip. Before T it
// simply restores any partial main fade. The ready callback runs off-render
// only when the clip is silent and the provider may be resumed safely.
func (e *Engine) CancelInterrupt(state *interruptState, fadeMS int64, ready func()) bool {
	if state == nil || ready == nil || e.interrupt.Load() != state {
		return false
	}
	request := &interruptCancelRequest{fadeMS: max(fadeMS, 0), ready: ready}
	return state.cancel.CompareAndSwap(nil, request)
}

func (e *Engine) ReleaseInterrupt(state *interruptState) bool {
	if state == nil || !e.interrupt.CompareAndSwap(state, nil) {
		return false
	}
	e.mediaBusy.Store(false)
	return true
}

func (e *Engine) InterruptActive() bool { return e.interrupt.Load() != nil }

func dbAmplitude(db float64) float32 {
	return float32(math.Pow(10, db/20))
}

func rampValue(current, start, target float32, total, done *int) float32 {
	if *total <= 0 || *done >= *total {
		*total = 0
		return target
	}
	t := float64(*done) / float64(*total)
	s := 0.5 * (1 - math.Cos(math.Pi*t))
	current = start + (target-start)*float32(s)
	*done = *done + 1
	return current
}

func (e *Engine) beginDuckRamp(state *overlayState, target float32, durationMS int64) {
	state.duckStart = state.duckCurrent
	state.duckTarget = target
	state.duckDone = 0
	state.duckTotal = sampleRate * int(durationMS) / 1000
	if durationMS > 0 && state.duckTotal < 1 {
		state.duckTotal = 1
	}
}

func beginInterruptRamp(current float32, target float32, durationMS int64) (float32, float32, int, int) {
	if current == target {
		return current, target, 0, 0
	}
	total := sampleRate * int(durationMS) / 1000
	if durationMS > 0 && total < 1 {
		total = 1
	}
	return current, target, total, 0
}

func (e *Engine) mixInterrupt(dst []float32, state *interruptState, now time.Time) int {
	if request := state.cancel.Load(); request != nil && !state.cancelling {
		state.cancelling = true
		state.cancelActive = request
		if state.started {
			state.clipStart, _, state.clipTotal, state.clipDone =
				beginInterruptRamp(state.clipGain, 0, request.fadeMS)
		} else {
			state.mainStart, state.mainTarget, state.mainTotal, state.mainDone =
				beginInterruptRamp(state.mainGain, 1, state.control.FadeInMS)
		}
	}

	frames := len(dst) / channels
	musicFrames := 0
	if !state.started {
		if state.cancelling {
			musicFrames = frames
		} else if now.Before(state.startAt) {
			untilStart := state.startAt.Sub(now)
			musicFrames = int((untilStart*time.Duration(sampleRate) + time.Second - 1) / time.Second)
			musicFrames = min(max(musicFrames, 0), frames)
		}
	}

	musicFloats := 0
	if musicFrames > 0 {
		musicFloats = e.renderMusic(dst[:musicFrames*channels])
	}
	for index := musicFrames * channels; index < len(dst); index++ {
		dst[index] = 0
	}
	for frame := 0; frame < musicFrames; frame++ {
		frameAt := now.Add(time.Duration(frame) * time.Second / sampleRate)
		if !state.mainFadeStarted && !state.cancelling && !frameAt.Before(state.preFadeAt) {
			state.mainFadeStarted = true
			state.mainStart, state.mainTarget, state.mainTotal, state.mainDone =
				beginInterruptRamp(state.mainGain, 0, state.control.FadeOutMS)
			elapsedFrames := int(frameAt.Sub(state.preFadeAt) * sampleRate / time.Second)
			state.mainDone = min(max(elapsedFrames, 0), state.mainTotal)
		}
		state.mainGain = rampValue(
			state.mainGain, state.mainStart, state.mainTarget,
			&state.mainTotal, &state.mainDone)
		for channel := 0; channel < channels; channel++ {
			dst[frame*channels+channel] *= state.mainGain
		}
	}

	if state.cancelling && !state.started {
		if state.mainTotal == 0 && state.mainGain == 1 && !state.resumeRequested {
			state.resumeRequested = true
			if e.overlay.Load() == nil && e.interrupt.CompareAndSwap(state, nil) {
				e.mediaBusy.Store(false)
				e.postCompletion(renderCompletion{
					kind: renderInterruptCancelReady, interrupt: state,
					interruptCancel: state.cancelActive,
				})
			}
		}
		return musicFloats
	}

	for frame := musicFrames; frame < frames && !state.ended; frame++ {
		frameAt := now.Add(time.Duration(frame) * time.Second / sampleRate)
		if !state.started {
			state.started = true
			state.mainGain = 0
			e.postCompletion(renderCompletion{
				kind: renderInterruptStarted, interrupt: state, localMS: frameAt.UnixMilli(),
			})
		}
		if state.cancelling {
			state.clipGain = rampValue(
				state.clipGain, state.clipStart, 0,
				&state.clipTotal, &state.clipDone)
		}
		if state.cursor < len(state.samples) {
			for channel := 0; channel < channels; channel++ {
				dst[frame*channels+channel] = state.samples[state.cursor+channel] * state.clipGain
			}
			state.cursor += channels
			e.interruptFrames.Add(1)
		}
		if state.cursor >= len(state.samples) {
			state.ended = true
			if !state.cancelling {
				state.resumeRequested = true
				e.postCompletion(renderCompletion{
					kind: renderInterruptEnded, interrupt: state,
					localMS: frameAt.Add(time.Second / sampleRate).UnixMilli(),
				})
			}
		}
	}
	if state.cancelling && state.started && !state.resumeRequested &&
		((state.clipTotal == 0 && state.clipGain == 0) || state.ended) {
		state.resumeRequested = true
		e.postCompletion(renderCompletion{
			kind: renderInterruptCancelReady, interrupt: state,
			interruptCancel: state.cancelActive,
		})
	}
	return musicFloats
}

func (e *Engine) mixOverlay(dst []float32, state *overlayState, now time.Time) {
	if request := state.cancel.Load(); request != nil && !state.cancelling {
		state.cancelling = true
		state.cancelActive = request
		state.overlayStart = state.overlayGain
		state.overlayDone = 0
		state.overlayTotal = sampleRate * int(request.fadeMS) / 1000
		if request.fadeMS > 0 && state.overlayTotal < 1 {
			state.overlayTotal = 1
		}
		e.beginDuckRamp(state, 1, state.control.ReleaseMS)
	}

	frames := len(dst) / channels
	for frame := 0; frame < frames; frame++ {
		frameAt := now.Add(time.Duration(frame) * time.Second / sampleRate)
		if !state.duckStarted && !state.cancelling && !frameAt.Before(state.preDuckAt) {
			state.duckStarted = true
			e.beginDuckRamp(state, dbAmplitude(state.control.DuckDB), state.control.AttackMS)
			// Arm normally precedes pre-duck. If scheduling arrives later but is
			// still within the start deadline, catch the envelope up to absolute
			// time instead of restarting a 250 ms attack at the first callback.
			elapsed := frameAt.Sub(state.preDuckAt)
			elapsedFrames := int(elapsed * sampleRate / time.Second)
			if elapsedFrames > state.duckTotal {
				elapsedFrames = state.duckTotal
			}
			state.duckDone = max(elapsedFrames, 0)
		}

		state.duckCurrent = rampValue(
			state.duckCurrent, state.duckStart, state.duckTarget,
			&state.duckTotal, &state.duckDone)
		if state.cancelling {
			state.overlayGain = rampValue(
				state.overlayGain, state.overlayStart, 0,
				&state.overlayTotal, &state.overlayDone)
		}
		for channel := 0; channel < channels; channel++ {
			dst[frame*channels+channel] *= state.duckCurrent
		}

		if !state.started && !state.cancelling && !frameAt.Before(state.startAt) {
			state.started = true
			e.postCompletion(renderCompletion{
				kind: renderOverlayStarted, overlay: state, localMS: frameAt.UnixMilli(),
			})
		}
		if state.started && !state.ended && state.cursor < len(state.samples) {
			for channel := 0; channel < channels; channel++ {
				dst[frame*channels+channel] += state.samples[state.cursor+channel] * state.overlayGain
			}
			state.cursor += channels
			e.overlayFrames.Add(1)
			if state.cursor >= len(state.samples) && !state.cancelling {
				state.ended = true
				e.beginDuckRamp(state, 1, state.control.ReleaseMS)
				e.postCompletion(renderCompletion{
					kind: renderOverlayEnded, overlay: state,
					localMS: frameAt.Add(time.Second / sampleRate).UnixMilli(),
				})
			}
		}
	}

	duckReleased := state.duckTotal == 0 && state.duckCurrent == 1
	if state.cancelling && state.overlayTotal == 0 && state.overlayGain == 0 && duckReleased {
		if e.overlay.CompareAndSwap(state, nil) {
			e.mediaBusy.Store(false)
			e.postCompletion(renderCompletion{
				kind: renderOverlayCancelled, overlay: state, cancel: state.cancelActive,
			})
		}
	} else if state.ended && duckReleased {
		if e.overlay.CompareAndSwap(state, nil) {
			e.mediaBusy.Store(false)
		}
	}
}

// mixLive is part of the WASAPI render boundary. It reads only a fixed SPSC
// ring and preallocated scratch, owns every mutable envelope field, and never
// waits, allocates, decodes, logs or performs transport/filesystem work.
func (e *Engine) mixLive(dst []float32, state *windowsLiveRenderState) bool {
	if state == nil || state.generation != e.liveEpoch.Load() {
		return false
	}
	if request := state.stop.Load(); request != nil && !state.stopping {
		state.stopping = true
		state.liveStart, state.liveTarget, state.liveTotal, state.liveDone =
			beginInterruptRamp(state.liveGain, 0, 5)
		state.duckStart, state.duckTarget, state.duckTotal, state.duckDone =
			beginInterruptRamp(state.duckGain, 1, 160)
	}
	if state.active.Load() && !state.activated && !state.stopping {
		state.activated = true
		state.liveStart, state.liveTarget, state.liveTotal, state.liveDone =
			beginInterruptRamp(0, 1, 5)
	}

	underrun := false
	for offset := 0; offset < len(dst); {
		count := min(len(e.liveRenderScratch), len(dst)-offset)
		count -= count % channels
		if count == 0 {
			break
		}
		scratch := e.liveRenderScratch[:count]
		got := 0
		if state.activated {
			got = state.ring.Read(scratch)
			if got < count && !state.stopping {
				underrun = true
			}
		}
		if got > 0 && !state.started {
			state.started = true
			e.postCompletion(renderCompletion{kind: renderLiveStarted, liveStarted: state.onStarted})
		}
		for index := got; index < count; index++ {
			scratch[index] = 0
		}
		for frame := 0; frame < count/channels; frame++ {
			state.duckGain = rampValue(
				state.duckGain, state.duckStart, state.duckTarget,
				&state.duckTotal, &state.duckDone)
			state.liveGain = rampValue(
				state.liveGain, state.liveStart, state.liveTarget,
				&state.liveTotal, &state.liveDone)
			base := offset + frame*channels
			dst[base] *= state.duckGain
			dst[base+1] *= state.duckGain
			left, right := scratch[frame*channels], scratch[frame*channels+1]
			if state.stopping && frame*channels >= got {
				left, right = state.lastLeft, state.lastRight
			}
			dst[base] += left * state.liveGain
			dst[base+1] += right * state.liveGain
			if frame*channels < got {
				state.lastLeft, state.lastRight = left, right
				e.liveFrames.Add(1)
			}
		}
		offset += count
	}
	if underrun {
		e.liveUnderruns.Add(1)
	}
	if state.stopping && state.liveTotal == 0 && state.liveGain == 0 &&
		state.duckTotal == 0 && state.duckGain == 1 {
		e.live.CompareAndSwap(state, nil)
	}
	return true
}

func (e *Engine) applyOverlayLimiter(dst []float32, ceilingDB float64) {
	ceiling := dbAmplitude(ceilingDB)
	for index, sample := range dst {
		if sample > ceiling {
			dst[index] = ceiling
			e.limiterHits.Add(1)
		} else if sample < -ceiling {
			dst[index] = -ceiling
			e.limiterHits.Add(1)
		}
	}
}

func (e *Engine) renderMusic(dst []float32) int {
	got := e.music.Read(dst)
	for index := got; index < len(dst); index++ {
		dst[index] = 0
	}
	e.gain.ApplyMusicRamp(dst)
	if got < len(dst) {
		e.starved.Add(1)
		if e.expectingMusic.Load() {
			e.starvedStreak.Add(1)
		} else {
			e.starvedStreak.Store(0)
		}
	} else {
		e.fed.Add(1)
		e.starvedStreak.Store(0)
	}
	return got
}

// Render fills dst (interleaved stereo f32) and returns how many floats were
// pulled from the MUSIC ring (0 while a voice insert plays — voice must not
// trip the started detector or the underrun counters).
//
// Single consumer: only the render loop calls this (SPSC contract of Ring).
func (e *Engine) Render(dst []float32) int {
	now := e.now()

	musicFloats := 0
	activeOverlay := e.overlay.Load()
	activeInterrupt := e.interrupt.Load()
	if activeInterrupt != nil {
		musicFloats = e.mixInterrupt(dst, activeInterrupt, now)
	} else if activeOverlay != nil {
		// Overlay never pauses the program timeline: consume the main ring on
		// every callback, including silence and pre-duck/release tails.
		musicFloats = e.renderMusic(dst)
		e.mixOverlay(dst, activeOverlay, now)
	} else if v := e.voice.Load(); v != nil && !now.Before(v.startAt) {
		// Voice REPLACES music: the ring is left untouched.
		n := copy(dst, v.samples[v.cursor:])
		v.cursor += n
		for i := n; i < len(dst); i++ {
			dst[i] = 0
		}
		if v.cursor >= len(v.samples) {
			if e.voice.CompareAndSwap(v, nil) {
				e.postCompletion(renderCompletion{kind: renderVoiceDone, voice: v})
			}
		}
	} else {
		musicFloats = e.renderMusic(dst)
	}

	activeLive := e.live.Load()
	liveMixed := e.mixLive(dst, activeLive)

	// Click overlay (additive, like the mac insert player node).
	if schedule := e.clicks.Load(); schedule != nil {
		frames := len(dst) / channels
		retained := false
		for _, start := range schedule.starts {
			elapsed := now.Sub(start)
			if elapsed < 0 {
				retained = true // still in the future
				continue
			}
			off := int(elapsed.Seconds() * sampleRate)
			if off >= len(e.clickBurst) {
				continue // already fully sounded, drop it
			}
			for f := 0; f < frames && off+f < len(e.clickBurst); f++ {
				s := e.clickBurst[off+f]
				for ch := 0; ch < channels; ch++ {
					dst[f*channels+ch] += s
				}
			}
			if off+frames < len(e.clickBurst) {
				retained = true // tail still to sound
			}
		}
		if !retained {
			e.clicks.CompareAndSwap(schedule, nil)
		}
	}
	if liveMixed {
		e.applyOverlayLimiter(dst, liveLimiterDB)
	} else if activeInterrupt != nil {
		e.applyOverlayLimiter(dst, activeInterrupt.control.LimiterCeilingDB)
	} else if activeOverlay != nil {
		e.applyOverlayLimiter(dst, activeOverlay.control.LimiterCeilingDB)
	}

	// Master amplitude last: scales music, voice and clicks alike.
	amp := e.gain.Amplitude()
	for i := range dst {
		dst[i] *= amp
	}

	return musicFloats
}
