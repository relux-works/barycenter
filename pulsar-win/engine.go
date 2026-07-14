// Engine: the portable render mix — the pulsar-win equivalent of the macOS
// AVAudioEngine graph (AudioEngine.swift), pulled per callback by the WASAPI
// render loop (audio_windows.go):
//
//	music ring -> music fade gain \
//	voice samples (REPLACE music)  +-> click overlay -> master amplitude -> dst
//
// Semantics ported from the mac node:
//   - Voice REPLACES music: while a voice insert is sounding the music ring
//     is not consumed (the coordinator paused the daemon anyway; the ring
//     tail stays for the resume) — spec 6.3 play_voice.
//   - offset_test clicks are ADDED on top (the AVAudioPlayerNode branch).
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
)

type voiceState struct {
	samples []float32 // interleaved stereo 44.1k
	cursor  int
	startAt time.Time // zero = start on the next render pull
	onDone  func()    // dispatched off-render after the last sample rendered
}

type clickSchedule struct {
	starts []time.Time
}

// RenderStats is a snapshot of the dropout telemetry counters.
type RenderStats struct {
	Fed           int64 // callbacks fully served from the music ring
	Starved       int64 // callbacks that zero-filled a shortfall
	StarvedStreak int64 // consecutive starved callbacks while expecting music
}

type Engine struct {
	music *Ring
	gain  *Gain
	now   func() time.Time // injectable for click/voice scheduling tests

	voice          atomic.Pointer[voiceState]
	clicks         atomic.Pointer[clickSchedule]
	clickBurst     []float32 // precomputed mono burst
	expectingMusic atomic.Bool
	fed            atomic.Int64
	starved        atomic.Int64
	starvedStreak  atomic.Int64
	doneCallbacks  chan func()
	done           chan struct{}
	closeOnce      sync.Once
}

func NewEngine(music *Ring, gain *Gain) *Engine {
	burst := make([]float32, sampleRate*clickDurationMS/1000)
	for i := range burst {
		burst[i] = clickAmplitude * float32(math.Sin(2*math.Pi*clickFreqHz*float64(i)/sampleRate))
	}
	e := &Engine{
		music: music, gain: gain, now: time.Now, clickBurst: burst,
		doneCallbacks: make(chan func(), 8),
		done:          make(chan struct{}),
	}
	go e.dispatchDoneCallbacks()
	return e
}

func (e *Engine) dispatchDoneCallbacks() {
	for {
		select {
		case callback := <-e.doneCallbacks:
			callback()
		case <-e.done:
			return
		}
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
	}
}

// Render fills dst (interleaved stereo f32) and returns how many floats were
// pulled from the MUSIC ring (0 while a voice insert plays — voice must not
// trip the started detector or the underrun counters).
//
// Single consumer: only the render loop calls this (SPSC contract of Ring).
func (e *Engine) Render(dst []float32) int {
	now := e.now()

	var doneCb func()
	musicFloats := 0

	if v := e.voice.Load(); v != nil && !now.Before(v.startAt) {
		// Voice REPLACES music: the ring is left untouched.
		n := copy(dst, v.samples[v.cursor:])
		v.cursor += n
		for i := n; i < len(dst); i++ {
			dst[i] = 0
		}
		if v.cursor >= len(v.samples) {
			if e.voice.CompareAndSwap(v, nil) {
				doneCb = v.onDone
			}
		}
	} else {
		got := e.music.Read(dst)
		for i := got; i < len(dst); i++ {
			dst[i] = 0
		}
		e.gain.ApplyMusicRamp(dst)
		musicFloats = got
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
	}

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

	// Master amplitude last: scales music, voice and clicks alike.
	amp := e.gain.Amplitude()
	for i := range dst {
		dst[i] *= amp
	}

	if doneCb != nil {
		select {
		case e.doneCallbacks <- doneCb:
		default:
			// The queue is preallocated and intentionally non-blocking. There
			// can only be one active voice, so a full queue means the callback
			// consumer is stalled; preserve audio timing instead of waiting.
		}
	}
	return musicFloats
}
