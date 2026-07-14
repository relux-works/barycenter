package main

import (
	"math"
	"runtime"
	"sync/atomic"
	"testing"
	"time"
	"unsafe"

	protocol "relux.works/duet/pulsar-win/wire"
)

func TestWindowsRuns100SequentialOverlaysWithoutGrowthOrDeadlock(t *testing.T) {
	const overlays = 100
	framesPerRun := sampleRate*600/1000 + 2 // one clip frame plus full release
	ring := NewRing(framesPerRun * channels)
	engine, gain := newTestEngine(ring)
	t.Cleanup(engine.Close)
	t.Cleanup(gain.Close)
	now := time.Unix(6_000, 0)
	engine.now = func() time.Time { return now }
	dst := make([]float32, framesPerRun*channels)
	main := make([]float32, len(dst))
	clip := []float32{0.2, 0.2}
	var ended atomic.Int64

	runtime.GC()
	var before runtime.MemStats
	runtime.ReadMemStats(&before)
	for iteration := 0; iteration < overlays; iteration++ {
		if written := ring.Write(main); written != len(main) {
			t.Fatalf("iteration %d main write=%d want=%d", iteration, written, len(main))
		}
		if _, err := engine.ArmOverlay(
			clip,
			overlayPlan(now, 1),
			func(int64) {},
			func(int64) { ended.Add(1) },
		); err != nil {
			t.Fatalf("iteration %d arm: %v", iteration, err)
		}
		if got := engine.Render(dst); got != len(dst) {
			t.Fatalf("iteration %d consumed=%d want=%d", iteration, got, len(dst))
		}
		if engine.OverlayActive() || ring.Fill() != 0 {
			t.Fatalf("iteration %d leaked active graph or ring tail", iteration)
		}
		now = now.Add(time.Duration(framesPerRun) * time.Second / sampleRate)
	}
	deadline := time.Now().Add(time.Second)
	for ended.Load() != overlays && time.Now().Before(deadline) {
		runtime.Gosched()
	}
	if ended.Load() != overlays {
		t.Fatalf("terminal callbacks=%d want=%d", ended.Load(), overlays)
	}
	runtime.GC()
	var after runtime.MemStats
	runtime.ReadMemStats(&after)
	if growth := int64(after.HeapAlloc) - int64(before.HeapAlloc); growth > 4<<20 {
		t.Fatalf("100-overlay retained heap growth=%d bytes", growth)
	}
}

func TestWindowsMaximumP1ClipHasOneBoundedDecodedBuffer(t *testing.T) {
	maxFrames := sampleRate * int(maximumP1ClipDurationMS) / 1000
	maxFloats := maxFrames * channels
	maxBytes := int64(maxFloats) * int64(unsafe.Sizeof(float32(0)))
	if maxBytes != 63_504_000 || maxBytes >= 64<<20 {
		t.Fatalf("maximum decoded PCM bound=%d", maxBytes)
	}

	clip := make([]float32, maxFloats)
	ring := NewRing(2)
	engine, gain := newTestEngine(ring)
	t.Cleanup(engine.Close)
	t.Cleanup(gain.Close)
	state, err := engine.ArmInterrupt(
		clip,
		windowsInterruptPlan(time.Unix(7_000, 0)),
		func(int64) {},
		func(int64) {},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(state.samples) != maxFloats || &state.samples[0] != &clip[0] {
		t.Fatal("render state copied or expanded the maximum prepared PCM buffer")
	}
	if !engine.ReleaseInterrupt(state) {
		t.Fatal("maximum prepared buffer owner did not release")
	}
}

func TestWindowsSenderDeleteDuringOverlayRestoresMainProgram(t *testing.T) {
	ring := NewRing(sampleRate * channels * 2)
	fillConstantRing(t, ring, sampleRate*2, 0.4)
	engine, gain := newTestEngine(ring)
	t.Cleanup(engine.Close)
	t.Cleanup(gain.Close)
	now := time.Unix(8_000, 0)
	engine.now = func() time.Time { return now }
	mixer := NewWindowsOverlayMediaClipMixer(engine)
	prepared := &windowsOverlayPrepared{
		samples:  make([]float32, sampleRate*channels),
		delivery: "overlay",
	}
	for index := range prepared.samples {
		prepared.samples[index] = 0.2
	}
	clip := &PreparedMediaClip{Decoder: prepared}
	if err := mixer.Arm(clip, overlayPlan(now, sampleRate), func(int64) {}, func(int64) {}); err != nil {
		t.Fatal(err)
	}
	dst := make([]float32, 441*channels)
	for index := 0; index < 30; index++ {
		engine.Render(dst)
		now = now.Add(10 * time.Millisecond)
	}
	acknowledged := make(chan bool, 1)
	mixer.Cancel(clip, protocol.CancelMediaPayload{
		TransmissionID: "tr_overlay", Generation: 1,
		Reason: "media_deleted", Action: "fade_stop", FadeMS: 120,
	}, func(resumed bool, err error) {
		if err != nil {
			t.Errorf("sender-delete cancel: %v", err)
		}
		acknowledged <- resumed
	})
	for index := 0; index < 72; index++ {
		if got := engine.Render(dst); got != len(dst) {
			t.Fatalf("recovery callback %d consumed=%d want=%d", index, got, len(dst))
		}
		now = now.Add(10 * time.Millisecond)
	}
	select {
	case resumed := <-acknowledged:
		if resumed {
			t.Fatal("overlay sender-delete reported an interrupt resume")
		}
	case <-time.After(time.Second):
		t.Fatal("sender-delete acknowledgement missing")
	}
	if engine.OverlayActive() {
		t.Fatal("sender-delete leaked overlay state")
	}
	if got := dst[len(dst)-1]; math.Abs(float64(got-0.4)) > 0.0001 {
		t.Fatalf("main program did not recover: sample=%v want=0.4", got)
	}
}
