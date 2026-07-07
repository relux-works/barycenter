// Gain: raised-cosine music ramps and the glided master volume.
package main

import (
	"math"
	"testing"
	"time"
)

// rampGains pushes DC 1.0 through ApplyMusicRamp and returns one gain value
// per frame (channel 0 of each frame IS the gain after multiplication).
func rampGains(g *Gain, frames int) []float32 {
	out := make([]float32, 0, frames)
	buf := make([]float32, 512*channels)
	for len(out) < frames {
		for i := range buf {
			buf[i] = 1
		}
		g.ApplyMusicRamp(buf)
		for f := 0; f < 512 && len(out) < frames; f++ {
			out = append(out, buf[f*channels])
		}
	}
	return out
}

func TestMusicRampRaisedCosineShape(t *testing.T) {
	g := NewGain()
	g.SetMusicGain(0, 0) // snap down, then ramp up 0 -> 1
	const fadeMS = 100
	g.SetMusicGain(1, fadeMS)
	total := sampleRate * fadeMS / 1000

	gains := rampGains(g, total+256)

	if gains[0] != 0 {
		t.Fatalf("ramp must start at the old gain, first frame %v", gains[0])
	}
	for i := 1; i < total; i++ {
		if gains[i] < gains[i-1]-1e-6 {
			t.Fatalf("ramp not monotonic at frame %d: %v -> %v", i, gains[i-1], gains[i])
		}
	}
	if got := gains[total/2]; math.Abs(float64(got)-0.5) > 0.01 {
		t.Fatalf("cosine midpoint %v, want ~0.5", got)
	}
	// Quarter point of a raised cosine is 0.5*(1-cos(pi/4)) ~= 0.146 — the
	// S-curve signature (a linear ramp would read 0.25 here).
	if got := gains[total/4]; math.Abs(float64(got)-0.1464) > 0.01 {
		t.Fatalf("quarter point %v, want ~0.146 (raised cosine, not linear)", got)
	}
	if got := gains[total+128]; got != 1 {
		t.Fatalf("gain after the ramp %v, want exactly the target 1", got)
	}
}

func TestMusicRampDownAndSnap(t *testing.T) {
	g := NewGain()
	g.SetMusicGain(0, 50) // 1 -> 0 over 50 ms
	total := sampleRate * 50 / 1000
	gains := rampGains(g, total+64)
	if gains[0] != 1 {
		t.Fatalf("down ramp must start at 1, got %v", gains[0])
	}
	for i := 1; i < total; i++ {
		if gains[i] > gains[i-1]+1e-6 {
			t.Fatalf("down ramp not monotonic at frame %d", i)
		}
	}
	if gains[total+32] != 0 {
		t.Fatalf("down ramp must land on 0, got %v", gains[total+32])
	}

	// fadeMs <= 0 snaps instantly, no ramp.
	g.SetMusicGain(0.7, 0)
	if got := g.MusicGain(); got != 0.7 {
		t.Fatalf("snap gain %v, want 0.7", got)
	}
	one := rampGains(g, 4)
	for _, v := range one {
		if v != 0.7 {
			t.Fatalf("snapped gain must hold 0.7, got %v", v)
		}
	}
}

func TestVolumeGlideConvergesToSquaredAmplitude(t *testing.T) {
	g := NewGain()
	t.Cleanup(g.Close)
	g.glideInterval = time.Millisecond // shrink the 16 ms tick for the test

	if got := g.Amplitude(); math.Abs(float64(got)-0.64) > 1e-6 {
		t.Fatalf("default amplitude %v, want 0.64 (volume 80)", got)
	}

	g.SetVolume(50) // amplitude glides to 0.25
	waitAmplitude(t, g, 0.25)

	// Retarget mid-flight: the running glide must pick the new target up.
	g.SetVolume(100)
	g.SetVolume(0)
	waitAmplitude(t, g, 0)

	// Out-of-range values clamp.
	g.SetVolume(150)
	waitAmplitude(t, g, 1)
}

func waitAmplitude(t *testing.T, g *Gain, want float64) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if math.Abs(float64(g.Amplitude())-want) < 0.002 {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("amplitude %v never converged to %v", g.Amplitude(), want)
}
