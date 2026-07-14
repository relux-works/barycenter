package main

import (
	"math"
	"testing"
	"time"

	protocol "relux.works/duet/pulsar-win/wire"
)

func windowsInterruptPlan(start time.Time) MediaClipPlayPlan {
	return MediaClipPlayPlan{
		LocalStartMS:         start.UnixMilli(),
		LocalStartDeadlineMS: start.Add(100 * time.Millisecond).UnixMilli(),
		Control: MixerControlParameters{
			TransmissionID: "tr_interrupt", Generation: 1, Delivery: "interrupt",
			FadeOutMS: 250, FadeInMS: 120, LimiterCeilingDB: -1,
			Interrupt: true, ReportStarted: true, ReportEnded: true,
		},
		Payload: protocol.PlayMediaAtPayload{
			TransmissionID: "tr_interrupt", Generation: 1, Delivery: "interrupt",
			TCoordMS:             start.UnixMilli(),
			StartDeadlineCoordMS: start.Add(100 * time.Millisecond).UnixMilli(),
		},
	}
}

func TestWindowsInterruptRenderFreezesMainAtTAndReplacesIt(t *testing.T) {
	const chunkFrames = 441
	ring := NewRing(sampleRate * channels * 2)
	fillConstantRing(t, ring, sampleRate, 0.5)
	engine, gain := newTestEngine(ring)
	t.Cleanup(engine.Close)
	t.Cleanup(gain.Close)

	base := time.Unix(5_000, 0)
	now := base
	engine.now = func() time.Time { return now }
	clip := make([]float32, sampleRate/10*channels)
	for index := range clip {
		clip[index] = 1.2
	}
	started := make(chan int64, 1)
	ended := make(chan int64, 1)
	state, err := engine.ArmInterrupt(
		clip, windowsInterruptPlan(base.Add(250*time.Millisecond)),
		func(localMS int64) { started <- localMS },
		func(localMS int64) { ended <- localMS })
	if err != nil {
		t.Fatal(err)
	}

	dst := make([]float32, chunkFrames*channels)
	for callback := 0; callback < 25; callback++ {
		if got := engine.Render(dst); got != len(dst) {
			t.Fatalf("pre-T callback %d consumed=%d want=%d", callback, got, len(dst))
		}
		now = now.Add(10 * time.Millisecond)
	}
	remainingAtT := ring.Fill()
	if remainingAtT != sampleRate*channels-25*len(dst) {
		t.Fatalf("ring at T=%d", remainingAtT)
	}

	for callback := 0; callback < 10; callback++ {
		if got := engine.Render(dst); got != 0 {
			t.Fatalf("interrupt callback %d consumed main=%d", callback, got)
		}
		if callback == 0 {
			ceiling := dbAmplitude(-1)
			if math.Abs(float64(dst[0]-ceiling)) > 0.0001 {
				t.Fatalf("interrupt limiter output=%v want=%v", dst[0], ceiling)
			}
		}
		now = now.Add(10 * time.Millisecond)
	}
	if ring.Fill() != remainingAtT {
		t.Fatalf("main advanced during replacement: before=%d after=%d", remainingAtT, ring.Fill())
	}
	select {
	case at := <-started:
		if at != base.Add(250*time.Millisecond).UnixMilli() {
			t.Fatalf("started=%d", at)
		}
	case <-time.After(time.Second):
		t.Fatal("interrupt start callback missing")
	}
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Fatal("interrupt end callback missing")
	}
	if stats := engine.Stats(); stats.InterruptFrames != sampleRate/10 || stats.LimiterHits == 0 {
		t.Fatalf("stats=%+v", stats)
	}
	if !engine.InterruptActive() || !engine.ReleaseInterrupt(state) || engine.InterruptActive() {
		t.Fatal("interrupt release handshake failed")
	}
}

func setupPlayerInterrupt(t *testing.T, clipFrames int) (*Player, *fakeDaemon, *Engine, *WindowsOverlayMediaClipMixer, *PreparedMediaClip, MediaClipPlayPlan, chan int64, chan int64) {
	t.Helper()
	daemon := newFakeDaemon()
	player, _, ring := newTestPlayer(t, daemon, fixedClock{ok: true})
	fillConstantRing(t, ring, sampleRate/20, 0.25) // 50 ms buffered ahead
	player.mu.Lock()
	player.playback = PlaybackPlaying
	player.elementID = "el_interrupt"
	player.uri = "spotify:track:interrupt"
	player.anchorPosMS = 10_000
	player.anchorAt = time.Now()
	player.extrapolate = false
	player.mu.Unlock()
	engine := player.engine
	now := time.Now()
	engine.now = func() time.Time { return now }
	mixer := NewWindowsOverlayMediaClipMixer(engine)
	mixer.BindInterruptController(player)
	clip := &PreparedMediaClip{
		LocalPath: "/tmp/interrupt.wav", DecodedDurationMS: int64(clipFrames * 1000 / sampleRate),
		Decoder: &windowsOverlayPrepared{
			samples: make([]float32, clipFrames*channels), delivery: "interrupt",
		},
	}
	started := make(chan int64, 1)
	ended := make(chan int64, 1)
	return player, daemon, engine, mixer, clip, windowsInterruptPlan(now), started, ended
}

func TestWindowsInterruptResumesOnceFromAudibleAnchorWithFadeIn(t *testing.T) {
	player, daemon, engine, mixer, clip, plan, started, ended :=
		setupPlayerInterrupt(t, 441)
	if err := mixer.Arm(clip, plan, func(v int64) { started <- v }, func(v int64) { ended <- v }); err != nil {
		t.Fatal(err)
	}
	engine.Render(make([]float32, 441*channels))
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("started callback missing")
	}
	expectCall(t, daemon, "pause")
	if got := expectCall(t, daemon, "seek "); got != "seek 9950" {
		t.Fatalf("resume anchor call=%q", got)
	}
	expectCall(t, daemon, "resume")
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Fatal("ended callback missing")
	}
	if engine.InterruptActive() {
		t.Fatal("natural end left interrupt owner armed")
	}
	player.mu.Lock()
	playback, anchor := player.playback, player.anchorPosMS
	player.mu.Unlock()
	if playback != PlaybackPlaying || anchor != 9950 {
		t.Fatalf("player playback=%s anchor=%d", playback, anchor)
	}
	target, frames, ok := player.engine.gain.PendingMusicGain()
	if !ok || target != 1 || frames != sampleRate*120/1000 {
		t.Fatalf("fade-in target=%v frames=%d ok=%v", target, frames, ok)
	}
	neverCall(t, daemon, "resume", 30*time.Millisecond)
}

func TestWindowsInterruptCancelFadesThenResumesAndAcknowledgesOnce(t *testing.T) {
	_, daemon, engine, mixer, clip, plan, started, ended := setupPlayerInterrupt(t, sampleRate)
	if err := mixer.Arm(clip, plan, func(v int64) { started <- v }, func(v int64) { ended <- v }); err != nil {
		t.Fatal(err)
	}
	dst := make([]float32, 441*channels)
	engine.Render(dst)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("started callback missing")
	}
	expectCall(t, daemon, "pause")
	result := make(chan bool, 1)
	mixer.Cancel(clip, protocol.CancelMediaPayload{
		TransmissionID: "tr_interrupt", Generation: 1, Reason: "media_deleted",
		Action: "fade_stop", ResumeMain: true, FadeMS: 120,
	}, func(resumed bool, err error) {
		if err != nil {
			t.Errorf("cancel error: %v", err)
		}
		result <- resumed
	})
	for callback := 0; callback < 13; callback++ {
		engine.Render(dst)
	}
	select {
	case resumed := <-result:
		if !resumed {
			t.Fatal("active interrupt cancellation did not resume main")
		}
	case <-time.After(time.Second):
		t.Fatal("cancel acknowledgement missing")
	}
	if got := expectCall(t, daemon, "seek "); got != "seek 9950" {
		t.Fatalf("cancel resume anchor=%q", got)
	}
	expectCall(t, daemon, "resume")
	select {
	case <-ended:
		t.Fatal("cancelled interrupt emitted natural end")
	default:
	}
	if engine.InterruptActive() {
		t.Fatal("cancel left interrupt owner active")
	}
}

func TestWindowsInterruptStopInvalidatesOldResumeToken(t *testing.T) {
	player, daemon, engine, mixer, clip, plan, started, _ := setupPlayerInterrupt(t, sampleRate)
	if err := mixer.Arm(clip, plan, func(v int64) { started <- v }, func(int64) {}); err != nil {
		t.Fatal(err)
	}
	dst := make([]float32, 441*channels)
	engine.Render(dst)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("started callback missing")
	}
	expectCall(t, daemon, "pause")
	player.stopAll()
	expectCall(t, daemon, "stop")
	result := make(chan bool, 1)
	mixer.Cancel(clip, protocol.CancelMediaPayload{
		TransmissionID: "tr_interrupt", Generation: 1, Action: "fade_stop",
		ResumeMain: true, FadeMS: 0,
	}, func(resumed bool, _ error) { result <- resumed })
	engine.Render(dst)
	select {
	case resumed := <-result:
		if resumed {
			t.Fatal("stale stop token resumed main")
		}
	case <-time.After(time.Second):
		t.Fatal("cancel acknowledgement missing")
	}
	neverCall(t, daemon, "seek ", 30*time.Millisecond)
	neverCall(t, daemon, "resume", 30*time.Millisecond)
	if engine.InterruptActive() {
		t.Fatal("stop left interrupt owner active")
	}
}
