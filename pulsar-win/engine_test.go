// Engine render mix: music + fade gain, voice replacement, click overlay,
// dropout counters.
package main

import (
	"testing"
	"time"
)

// newTestEngine returns an engine whose master amplitude is pinned to 1 so
// sample values assert cleanly (the default is 0.64 = volume 80).
func newTestEngine(ring *Ring) (*Engine, *Gain) {
	gain := NewGain()
	gain.mu.Lock()
	gain.amp = 1
	gain.ampTarget = 1
	gain.mu.Unlock()
	return NewEngine(ring, gain), gain
}

func TestRenderPullsMusicAndCounts(t *testing.T) {
	ring := NewRing(4096)
	e, _ := newTestEngine(ring)

	src := make([]float32, 512)
	for i := range src {
		src[i] = 0.5
	}
	ring.Write(src)

	dst := make([]float32, 512)
	if got := e.Render(dst); got != 512 {
		t.Fatalf("rendered %d floats, want 512", got)
	}
	for i, v := range dst {
		if v != 0.5 {
			t.Fatalf("dst[%d] = %v, want 0.5", i, v)
		}
	}
	if s := e.Stats(); s.Fed != 1 || s.Starved != 0 {
		t.Fatalf("stats after a fed callback: %+v", s)
	}

	// Empty ring: zero fill + starved counter; streak only when expecting.
	if got := e.Render(dst); got != 0 {
		t.Fatalf("rendered %d floats from an empty ring", got)
	}
	for i, v := range dst {
		if v != 0 {
			t.Fatalf("underrun must render silence, dst[%d]=%v", i, v)
		}
	}
	if s := e.Stats(); s.Starved != 1 || s.StarvedStreak != 0 {
		t.Fatalf("idle starvation must not streak: %+v", s)
	}

	e.SetExpectingMusic(true)
	e.Render(dst)
	e.Render(dst)
	if s := e.Stats(); s.StarvedStreak != 2 {
		t.Fatalf("streak %d, want 2 while expecting music", s.StarvedStreak)
	}
	e.SetExpectingMusic(false)
	if s := e.Stats(); s.StarvedStreak != 0 {
		t.Fatalf("streak must reset when not expecting music: %+v", s)
	}
}

func TestVoiceReplacesMusicThenEnds(t *testing.T) {
	ring := NewRing(4096)
	e, _ := newTestEngine(ring)

	music := make([]float32, 1024)
	for i := range music {
		music[i] = 0.25
	}
	ring.Write(music)

	voice := make([]float32, 600)
	for i := range voice {
		voice[i] = 0.75
	}
	done := make(chan struct{})
	e.PlayVoice(voice, time.Time{}, func() { close(done) })
	if !e.VoiceActive() {
		t.Fatal("voice must be active after PlayVoice")
	}

	dst := make([]float32, 512)
	if got := e.Render(dst); got != 0 {
		t.Fatalf("music floats %d during voice, want 0 (voice replaces music)", got)
	}
	if dst[0] != 0.75 || dst[511] != 0.75 {
		t.Fatalf("voice samples not rendered: %v %v", dst[0], dst[511])
	}
	if ring.Fill() != 1024 {
		t.Fatalf("music ring consumed during voice: fill %d, want 1024", ring.Fill())
	}
	select {
	case <-done:
		t.Fatal("voice_ended before the last sample rendered")
	default:
	}

	// Second pull: 88 voice floats remain, the rest silence, then done.
	e.Render(dst)
	if dst[0] != 0.75 || dst[87] != 0.75 || dst[88] != 0 {
		t.Fatalf("voice tail wrong: %v %v %v", dst[0], dst[87], dst[88])
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("voice onDone never fired")
	}
	if e.VoiceActive() {
		t.Fatal("voice still active after the last sample")
	}

	// Music flows again.
	if got := e.Render(dst); got != 512 {
		t.Fatalf("music floats %d after voice, want 512", got)
	}
	if dst[0] != 0.25 {
		t.Fatalf("music sample %v after voice, want 0.25", dst[0])
	}
}

func TestVoiceWaitsForScheduledStart(t *testing.T) {
	ring := NewRing(1024)
	e, _ := newTestEngine(ring)
	now := time.Unix(1000, 0)
	e.now = func() time.Time { return now }

	voice := []float32{0.5, 0.5, 0.5, 0.5}
	e.PlayVoice(voice, now.Add(50*time.Millisecond), func() {})

	dst := make([]float32, 8)
	e.Render(dst)
	if dst[0] != 0 {
		t.Fatal("voice must stay silent before its scheduled start")
	}

	now = now.Add(50 * time.Millisecond)
	e.Render(dst)
	if dst[0] != 0.5 || dst[3] != 0.5 || dst[4] != 0 {
		t.Fatalf("scheduled voice wrong: %v", dst[:6])
	}
}

func TestClicksOverlayAtScheduledTimes(t *testing.T) {
	ring := NewRing(1024)
	e, _ := newTestEngine(ring)
	base := time.Unix(2000, 0)
	now := base
	e.now = func() time.Time { return now }

	e.PlayClicks(2, base.Add(10*time.Millisecond), 100)

	frames := 256
	dst := make([]float32, frames*channels)

	// Before the first click: silence, clicks retained.
	e.Render(dst)
	if !allZero(dst) {
		t.Fatal("click sounded before its scheduled time")
	}

	// At the first click: burst begins (sample 0 of a sine is 0 — check the
	// second frame, and stereo duplication).
	now = base.Add(10 * time.Millisecond)
	e.Render(dst)
	want := e.clickBurst[1]
	if dst[2] != want || dst[3] != want {
		t.Fatalf("click frame 1 = %v/%v, want %v on both channels", dst[2], dst[3], want)
	}

	// Between clicks (burst is 30 ms): silence again.
	now = base.Add(60 * time.Millisecond)
	e.Render(dst)
	if !allZero(dst) {
		t.Fatal("expected silence between clicks")
	}

	// Second click at +110 ms.
	now = base.Add(110 * time.Millisecond)
	e.Render(dst)
	if dst[2] != want {
		t.Fatalf("second click missing: %v, want %v", dst[2], want)
	}
}

func TestMasterAmplitudeScalesEverything(t *testing.T) {
	ring := NewRing(1024)
	gain := NewGain() // default amplitude 0.64 (volume 80)
	e := NewEngine(ring, gain)

	ring.Write([]float32{1, 1, 1, 1})
	dst := make([]float32, 4)
	e.Render(dst)
	if dst[0] != 0.64 {
		t.Fatalf("music sample %v, want 0.64 (master amplitude)", dst[0])
	}

	e.PlayVoice([]float32{1, 1}, time.Time{}, func() {})
	dst2 := make([]float32, 2)
	e.Render(dst2)
	if dst2[0] != 0.64 {
		t.Fatalf("voice sample %v, want 0.64 (master amplitude applies)", dst2[0])
	}
}

func allZero(p []float32) bool {
	for _, v := range p {
		if v != 0 {
			return false
		}
	}
	return true
}
