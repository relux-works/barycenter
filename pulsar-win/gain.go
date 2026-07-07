// Gain: the two volume mechanisms of the macOS AudioEngine, portable.
//
//  1. Music fade — a raised-cosine (S-curve) per-frame ramp applied to the
//     music branch only (pause fade_ms / resume 120 ms / stop 250 ms). Zero
//     slope at both ends, so fades into silence land softly instead of
//     perceptually "cutting off" (AudioEngine.setMusicGain).
//  2. Master volume — 0..100 mapped to amplitude (v/100)^2, glided towards
//     the target by a 16 ms ticker moving 0.18 of the remaining distance per
//     tick (AudioEngine.setVolume). Spotify Connect sends the phone slider
//     as discrete jumps; the glide keeps them from stepping audibly.
package main

import (
	"math"
	"sync"
	"time"
)

const (
	// volumeGlideInterval / volumeGlideFactor mirror the macOS volume ramp
	// timer: every 16 ms move 18% of the remaining distance to the target.
	volumeGlideInterval = 16 * time.Millisecond
	volumeGlideFactor   = 0.18
	// volumeGlideEpsilon: once within this distance, snap to the target and
	// stop the ticker (same 0.001 cutoff as the macOS engine).
	volumeGlideEpsilon = 0.001
)

type Gain struct {
	mu sync.Mutex

	// Music fade ramp state (consumed per frame by ApplyMusicRamp).
	current   float32
	start     float32
	target    float32
	rampTotal int // frames; 0 = no ramp active (snap to target)
	rampDone  int

	// Master amplitude glide.
	amp           float64
	ampTarget     float64
	glideRunning  bool
	closed        bool
	glideInterval time.Duration // variable so tests can shrink it
}

// NewGain starts at music gain 1 and the default volume 80 -> amplitude 0.64,
// the same resting state as the macOS engine.
func NewGain() *Gain {
	return &Gain{
		current:       1,
		target:        1,
		amp:           0.64, // (80/100)^2
		ampTarget:     0.64,
		glideInterval: volumeGlideInterval,
	}
}

// SetMusicGain fades the music branch to target over fadeMS along a raised
// cosine. fadeMS <= 0 snaps instantly (mirror of AudioEngine.setMusicGain).
func (g *Gain) SetMusicGain(target float32, fadeMS int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if fadeMS <= 0 {
		g.current = target
		g.target = target
		g.rampTotal = 0
		return
	}
	g.start = g.current
	g.target = target
	g.rampDone = 0
	g.rampTotal = sampleRate * fadeMS / 1000
	if g.rampTotal < 1 {
		g.rampTotal = 1
	}
}

// MusicGain is the instantaneous music-branch gain (test hook).
func (g *Gain) MusicGain() float32 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return g.current
}

// ApplyMusicRamp multiplies interleaved stereo samples by the music gain,
// advancing the raised-cosine ramp one step per frame — the exact loop the
// macOS render callback ran (0.5*(1-cos(pi*t)) easing from start to target).
func (g *Gain) ApplyMusicRamp(dst []float32) {
	g.mu.Lock()
	defer g.mu.Unlock()
	frames := len(dst) / channels
	for f := 0; f < frames; f++ {
		if g.rampTotal > 0 && g.rampDone < g.rampTotal {
			t := float64(g.rampDone) / float64(g.rampTotal)
			s := 0.5 * (1 - math.Cos(math.Pi*t))
			g.current = g.start + (g.target-g.start)*float32(s)
			g.rampDone++
		} else {
			g.current = g.target
			g.rampTotal = 0
		}
		for ch := 0; ch < channels; ch++ {
			dst[f*channels+ch] *= g.current
		}
	}
}

// SetVolume sets the master volume 0..100; the amplitude glides to (v/100)^2
// on a 16 ms ticker instead of stepping (mirror of AudioEngine.setVolume).
func (g *Gain) SetVolume(v int) {
	if v < 0 {
		v = 0
	}
	if v > 100 {
		v = 100
	}
	lin := float64(v) / 100
	g.mu.Lock()
	g.ampTarget = lin * lin
	if g.glideRunning || g.closed {
		g.mu.Unlock()
		return // the running glide goroutine picks the new target up
	}
	g.glideRunning = true
	interval := g.glideInterval
	g.mu.Unlock()
	go g.glideLoop(interval)
}

func (g *Gain) glideLoop(interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for range ticker.C {
		g.mu.Lock()
		if g.closed {
			g.glideRunning = false
			g.mu.Unlock()
			return
		}
		next := g.amp + (g.ampTarget-g.amp)*volumeGlideFactor
		if math.Abs(next-g.ampTarget) < volumeGlideEpsilon {
			g.amp = g.ampTarget
			g.glideRunning = false
			g.mu.Unlock()
			return
		}
		g.amp = next
		g.mu.Unlock()
	}
}

// Amplitude is the current master amplitude (applied to music, voice and
// clicks alike — the macOS mainMixer position in the graph).
func (g *Gain) Amplitude() float32 {
	g.mu.Lock()
	defer g.mu.Unlock()
	return float32(g.amp)
}

// Close stops the glide goroutine (test hygiene / shutdown).
func (g *Gain) Close() {
	g.mu.Lock()
	g.closed = true
	g.mu.Unlock()
}
