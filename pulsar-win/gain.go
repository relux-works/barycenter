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
	"sync/atomic"
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
	// Music fade commands are built on the control path and atomically handed
	// to the single render consumer. The mutable ramp below is render-owned.
	musicCommand atomic.Pointer[musicGainCommand]
	currentBits  atomic.Uint32
	current      float32
	start        float32
	target       float32
	rampTotal    int // frames; 0 = no ramp active (snap to target)
	rampDone     int

	// Master amplitude glide. The control goroutine owns target/lifecycle
	// under mu; render only performs an atomic load of ampBits.
	mu            sync.Mutex
	ampBits       atomic.Uint64
	ampTarget     float64
	glideRunning  bool
	closed        bool
	glideInterval time.Duration // variable so tests can shrink it
}

type musicGainCommand struct {
	start      float32
	target     float32
	rampFrames int
}

// NewGain starts at music gain 1 and the default volume 80 -> amplitude 0.64,
// the same resting state as the macOS engine.
func NewGain() *Gain {
	g := &Gain{
		current:       1,
		target:        1,
		ampTarget:     0.64,
		glideInterval: volumeGlideInterval,
	}
	g.currentBits.Store(math.Float32bits(1))
	g.ampBits.Store(math.Float64bits(0.64)) // (80/100)^2
	return g
}

// SetMusicGain fades the music branch to target over fadeMS along a raised
// cosine. fadeMS <= 0 snaps instantly (mirror of AudioEngine.setMusicGain).
func (g *Gain) SetMusicGain(target float32, fadeMS int) {
	start := math.Float32frombits(g.currentBits.Load())
	frames := 0
	if fadeMS > 0 {
		frames = sampleRate * fadeMS / 1000
		if frames < 1 {
			frames = 1
		}
	} else {
		start = target
		g.currentBits.Store(math.Float32bits(target))
	}
	g.musicCommand.Store(&musicGainCommand{start: start, target: target, rampFrames: frames})
}

// MusicGain is the instantaneous music-branch gain (test hook).
func (g *Gain) MusicGain() float32 {
	return math.Float32frombits(g.currentBits.Load())
}

// ApplyMusicRamp multiplies interleaved stereo samples by the music gain,
// advancing the raised-cosine ramp one step per frame — the exact loop the
// macOS render callback ran (0.5*(1-cos(pi*t)) easing from start to target).
func (g *Gain) ApplyMusicRamp(dst []float32) {
	if command := g.musicCommand.Swap(nil); command != nil {
		g.start = command.start
		g.current = command.start
		g.target = command.target
		g.rampDone = 0
		g.rampTotal = command.rampFrames
		if command.rampFrames == 0 {
			g.current = command.target
		}
	}
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
	g.currentBits.Store(math.Float32bits(g.current))
}

// PendingMusicGain reports the latest immutable control command. It is used
// by diagnostics/tests; render consumes the same snapshot atomically.
func (g *Gain) PendingMusicGain() (target float32, rampFrames int, ok bool) {
	command := g.musicCommand.Load()
	if command == nil {
		return 0, 0, false
	}
	return command.target, command.rampFrames, true
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
		amp := math.Float64frombits(g.ampBits.Load())
		next := amp + (g.ampTarget-amp)*volumeGlideFactor
		if math.Abs(next-g.ampTarget) < volumeGlideEpsilon {
			g.ampBits.Store(math.Float64bits(g.ampTarget))
			g.glideRunning = false
			g.mu.Unlock()
			return
		}
		g.ampBits.Store(math.Float64bits(next))
		g.mu.Unlock()
	}
}

// Amplitude is the current master amplitude (applied to music, voice and
// clicks alike — the macOS mainMixer position in the graph).
func (g *Gain) Amplitude() float32 {
	return float32(math.Float64frombits(g.ampBits.Load()))
}

// setAmplitudeForTest pins the render-visible amplitude without starting a
// glide. It deliberately remains test-only by convention.
func (g *Gain) setAmplitudeForTest(amplitude float64) {
	g.mu.Lock()
	g.ampTarget = amplitude
	g.ampBits.Store(math.Float64bits(amplitude))
	g.mu.Unlock()
}

// Close stops the glide goroutine (test hygiene / shutdown).
func (g *Gain) Close() {
	g.mu.Lock()
	g.closed = true
	g.mu.Unlock()
}
