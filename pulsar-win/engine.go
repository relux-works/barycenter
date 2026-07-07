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
	onDone  func()    // fired (in a fresh goroutine) after the last sample rendered
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

	mu             sync.Mutex
	voice          *voiceState
	clicks         []time.Time // scheduled click start times
	clickBurst     []float32   // precomputed mono burst
	expectingMusic bool
	fed            int64
	starved        int64
	starvedStreak  int64
}

func NewEngine(music *Ring, gain *Gain) *Engine {
	burst := make([]float32, sampleRate*clickDurationMS/1000)
	for i := range burst {
		burst[i] = clickAmplitude * float32(math.Sin(2*math.Pi*clickFreqHz*float64(i)/sampleRate))
	}
	return &Engine{music: music, gain: gain, now: time.Now, clickBurst: burst}
}

// SetExpectingMusic gates the starved-streak counter: silence while stopped
// or paused is idle, not an underrun (UNRESOLVED R4).
func (e *Engine) SetExpectingMusic(v bool) {
	e.mu.Lock()
	e.expectingMusic = v
	if !v {
		e.starvedStreak = 0
	}
	e.mu.Unlock()
}

// PlayVoice replaces music with the decoded voice samples starting at startAt
// (zero time = immediately). onDone fires once after the last sample rendered
// — that IS the audible end, so voice_ended needs no extra drain wait.
// A newer voice replaces a pending one (its onDone is dropped, mirroring the
// mac player node which just schedules over).
func (e *Engine) PlayVoice(samples []float32, startAt time.Time, onDone func()) {
	e.mu.Lock()
	e.voice = &voiceState{samples: samples, startAt: startAt, onDone: onDone}
	e.mu.Unlock()
}

// StopVoice drops any active or pending voice insert without firing onDone.
func (e *Engine) StopVoice() {
	e.mu.Lock()
	e.voice = nil
	e.mu.Unlock()
}

// VoiceActive reports whether a voice insert is pending or sounding.
func (e *Engine) VoiceActive() bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.voice != nil
}

// PlayClicks schedules count clicks, the first at firstAt, then every
// intervalMS (offset_test, spec 7.3).
func (e *Engine) PlayClicks(count int, firstAt time.Time, intervalMS int64) {
	e.mu.Lock()
	for k := 0; k < count; k++ {
		e.clicks = append(e.clicks, firstAt.Add(time.Duration(intervalMS*int64(k))*time.Millisecond))
	}
	e.mu.Unlock()
}

// Stats returns the telemetry counters snapshot.
func (e *Engine) Stats() RenderStats {
	e.mu.Lock()
	defer e.mu.Unlock()
	return RenderStats{Fed: e.fed, Starved: e.starved, StarvedStreak: e.starvedStreak}
}

// Render fills dst (interleaved stereo f32) and returns how many floats were
// pulled from the MUSIC ring (0 while a voice insert plays — voice must not
// trip the started detector or the underrun counters).
//
// Single consumer: only the render loop calls this (SPSC contract of Ring).
func (e *Engine) Render(dst []float32) int {
	e.mu.Lock()
	now := e.now()

	var doneCb func()
	musicFloats := 0

	if v := e.voice; v != nil && !now.Before(v.startAt) {
		// Voice REPLACES music: the ring is left untouched.
		n := copy(dst, v.samples[v.cursor:])
		v.cursor += n
		for i := n; i < len(dst); i++ {
			dst[i] = 0
		}
		if v.cursor >= len(v.samples) {
			doneCb = v.onDone
			e.voice = nil
		}
	} else {
		got := e.music.Read(dst)
		for i := got; i < len(dst); i++ {
			dst[i] = 0
		}
		e.gain.ApplyMusicRamp(dst)
		musicFloats = got
		if got < len(dst) {
			e.starved++
			if e.expectingMusic {
				e.starvedStreak++
			}
		} else {
			e.fed++
			e.starvedStreak = 0
		}
	}

	// Click overlay (additive, like the mac insert player node).
	if len(e.clicks) > 0 {
		frames := len(dst) / channels
		remaining := e.clicks[:0]
		for _, start := range e.clicks {
			elapsed := now.Sub(start)
			if elapsed < 0 {
				remaining = append(remaining, start) // still in the future
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
				remaining = append(remaining, start) // tail still to sound
			}
		}
		e.clicks = remaining
	}
	e.mu.Unlock()

	// Master amplitude last: scales music, voice and clicks alike.
	amp := e.gain.Amplitude()
	for i := range dst {
		dst[i] *= amp
	}

	if doneCb != nil {
		go doneCb() // voice_ended must not run under the engine lock
	}
	return musicFloats
}
