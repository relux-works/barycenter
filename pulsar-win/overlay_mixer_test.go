package main

import (
	"bytes"
	"math"
	"os"
	"path/filepath"
	"testing"
	"time"

	protocol "relux.works/duet/pulsar-win/wire"
)

func overlayPlan(start time.Time, frames int) MediaClipPlayPlan {
	return MediaClipPlayPlan{
		LocalStartMS:         start.UnixMilli(),
		LocalStartDeadlineMS: start.Add(100 * time.Millisecond).UnixMilli(),
		Control: MixerControlParameters{
			TransmissionID: "tr_overlay", Generation: 1, Delivery: "overlay",
			DuckDB: -12, AttackMS: 250, ReleaseMS: 600,
			LimiterCeilingDB: -1, ReportStarted: true, ReportEnded: true,
		},
		Payload: protocol.PlayMediaAtPayload{
			TransmissionID: "tr_overlay", Generation: 1, Delivery: "overlay",
			TCoordMS: start.UnixMilli(), StartDeadlineCoordMS: start.Add(100 * time.Millisecond).UnixMilli(),
		},
	}
}

func fillConstantRing(t *testing.T, ring *Ring, frames int, value float32) {
	t.Helper()
	samples := make([]float32, frames*channels)
	for index := range samples {
		samples[index] = value
	}
	if written := ring.Write(samples); written != len(samples) {
		t.Fatalf("ring write=%d want=%d", written, len(samples))
	}
}

func TestWindowsOverlayContinuouslyConsumesMainAndLimitsFinalMix(t *testing.T) {
	const (
		clipSeconds = 10
		chunkFrames = 441 // 10 ms
	)
	ring := NewRing(sampleRate * channels * 12)
	fillConstantRing(t, ring, sampleRate*11, 0.5)
	engine, gain := newTestEngine(ring)
	t.Cleanup(engine.Close)
	t.Cleanup(gain.Close)

	base := time.Unix(2_000, 0)
	now := base
	engine.now = func() time.Time { return now }
	overlay := make([]float32, sampleRate*channels*clipSeconds)
	for index := range overlay {
		overlay[index] = 0.8
	}
	started := make(chan int64, 1)
	ended := make(chan int64, 1)
	state, err := engine.ArmOverlay(
		overlay, overlayPlan(base.Add(250*time.Millisecond), len(overlay)/channels),
		func(at int64) { started <- at }, func(at int64) { ended <- at })
	if err != nil || state == nil {
		t.Fatalf("arm overlay state=%v err=%v", state, err)
	}

	dst := make([]float32, chunkFrames*channels)
	callbacks := 0
	for callbacks < 1_090 { // clip plus the full 600 ms release tail
		if got := engine.Render(dst); got != len(dst) {
			t.Fatalf("callback %d consumed %d main floats, want %d", callbacks, got, len(dst))
		}
		if callbacks == 30 { // overlay is active and main is already ducked
			ceiling := dbAmplitude(-1)
			if math.Abs(float64(dst[0]-ceiling)) > 0.0001 {
				t.Fatalf("post-mix limiter output=%v want ceiling=%v", dst[0], ceiling)
			}
		}
		callbacks++
		now = now.Add(10 * time.Millisecond)
	}

	select {
	case at := <-started:
		if at < base.Add(250*time.Millisecond).UnixMilli() || at > base.Add(260*time.Millisecond).UnixMilli() {
			t.Fatalf("started timestamp=%d", at)
		}
	case <-time.After(time.Second):
		t.Fatal("overlay started callback missing")
	}
	select {
	case <-ended:
	case <-time.After(time.Second):
		t.Fatal("overlay ended callback missing")
	}
	if engine.OverlayActive() {
		t.Fatal("overlay/duck release remained active after its tail")
	}
	stats := engine.Stats()
	if stats.OverlayFrames != sampleRate*clipSeconds || stats.LimiterHits == 0 || stats.Starved != 0 {
		t.Fatalf("overlay stats=%+v", stats)
	}
	wantConsumed := callbacks * chunkFrames * channels
	if remaining := ring.Fill(); remaining != sampleRate*11*channels-wantConsumed {
		t.Fatalf("main ring remaining=%d want=%d", remaining, sampleRate*11*channels-wantConsumed)
	}
}

func TestWindowsOverlayWithoutMainKeepsNormalClipGain(t *testing.T) {
	ring := NewRing(4096)
	engine, gain := newTestEngine(ring)
	t.Cleanup(engine.Close)
	t.Cleanup(gain.Close)
	now := time.Unix(3_000, 0)
	engine.now = func() time.Time { return now }
	clip := make([]float32, 64*channels)
	for index := range clip {
		clip[index] = 0.5
	}
	state, err := engine.ArmOverlay(clip, overlayPlan(now, 64), func(int64) {}, func(int64) {})
	if err != nil {
		t.Fatal(err)
	}
	dst := make([]float32, 32*channels)
	if got := engine.Render(dst); got != 0 {
		t.Fatalf("main floats=%d want=0", got)
	}
	for index, sample := range dst {
		if sample != 0.5 {
			t.Fatalf("overlay-only dst[%d]=%v want=0.5", index, sample)
		}
	}
	if math.Abs(float64(state.duckCurrent-dbAmplitude(-12))) > 0.0001 {
		t.Fatalf("late pre-duck catch-up=%v want=%v", state.duckCurrent, dbAmplitude(-12))
	}
}

func TestWindowsOverlayCancellationFadesAndReleasesDuckBeforeAck(t *testing.T) {
	ring := NewRing(sampleRate * channels * 3)
	fillConstantRing(t, ring, sampleRate*2, 0.4)
	engine, gain := newTestEngine(ring)
	t.Cleanup(engine.Close)
	t.Cleanup(gain.Close)
	now := time.Unix(4_000, 0)
	engine.now = func() time.Time { return now }
	clip := make([]float32, sampleRate*channels*2)
	for index := range clip {
		clip[index] = 0.2
	}
	state, err := engine.ArmOverlay(clip, overlayPlan(now, len(clip)/channels), func(int64) {}, func(int64) {})
	if err != nil {
		t.Fatal(err)
	}
	dst := make([]float32, 441*channels)
	for index := 0; index < 30; index++ {
		engine.Render(dst)
		now = now.Add(10 * time.Millisecond)
	}
	lastBeforeCancel := dst[len(dst)-1]
	cancelled := make(chan struct{}, 1)
	if !engine.CancelOverlay(state, 120, func() { cancelled <- struct{}{} }) {
		t.Fatal("cancel request was not accepted")
	}
	for index := 0; index < 61; index++ {
		if got := engine.Render(dst); got != len(dst) {
			t.Fatalf("cancel callback %d consumed %d main floats, want %d", index, got, len(dst))
		}
		if index == 0 && math.Abs(float64(dst[0]-lastBeforeCancel)) > 0.01 {
			t.Fatalf("cancel introduced a gain step: before=%v after=%v", lastBeforeCancel, dst[0])
		}
		now = now.Add(10 * time.Millisecond)
		if index < 59 {
			select {
			case <-cancelled:
				t.Fatalf("cancel acknowledged before duck release at callback %d", index)
			default:
			}
		}
	}
	select {
	case <-cancelled:
	case <-time.After(time.Second):
		t.Fatal("cancel acknowledgement missing")
	}
	if engine.OverlayActive() || state.duckCurrent != 1 || state.overlayGain != 0 {
		t.Fatalf("cancel tail active=%v duck=%v overlay=%v", engine.OverlayActive(), state.duckCurrent, state.overlayGain)
	}
}

func TestWindowsOverlayRenderAllocationsStayZero(t *testing.T) {
	const runs = 100
	ring := NewRing((runs + 2) * 256 * channels)
	fillConstantRing(t, ring, (runs+1)*256, 0.1)
	engine, gain := newTestEngine(ring)
	t.Cleanup(engine.Close)
	t.Cleanup(gain.Close)
	now := time.Unix(4_500, 0)
	engine.now = func() time.Time { return now }
	clip := make([]float32, (runs+2)*256*channels)
	state, err := engine.ArmOverlay(clip, overlayPlan(now, len(clip)/channels), func(int64) {}, func(int64) {})
	if err != nil {
		t.Fatal(err)
	}
	// Avoid measuring the one-time control notification; render still executes
	// the active clip, duck, limiter and continuous main-ring paths.
	state.started = true
	dst := make([]float32, 256*channels)
	if allocations := testing.AllocsPerRun(runs, func() {
		engine.Render(dst)
	}); allocations != 0 {
		t.Fatalf("overlay render allocations=%v want=0", allocations)
	}
}

func TestWindowsOverlayKeepsContinuityAcrossRepeatedMainHandoffs(t *testing.T) {
	const handoffs = 20
	ring := NewRing(512 * channels)
	engine, gain := newTestEngine(ring)
	t.Cleanup(engine.Close)
	t.Cleanup(gain.Close)
	now := time.Unix(4_750, 0)
	engine.now = func() time.Time { return now }
	clip := make([]float32, handoffs*128*channels)
	if _, err := engine.ArmOverlay(clip, overlayPlan(now, len(clip)/channels), func(int64) {}, func(int64) {}); err != nil {
		t.Fatal(err)
	}
	mainChunk := make([]float32, 128*channels)
	for index := range mainChunk {
		mainChunk[index] = 0.25
	}
	dst := make([]float32, len(mainChunk))
	for handoff := 0; handoff < handoffs; handoff++ {
		if err := pumpF32LE(bytes.NewReader(f32leBytes(mainChunk)), ring, make(chan struct{})); err != nil {
			t.Fatalf("handoff %d: %v", handoff, err)
		}
		if got := engine.Render(dst); got != len(dst) {
			t.Fatalf("handoff %d consumed=%d want=%d", handoff, got, len(dst))
		}
		now = now.Add(time.Duration(128) * time.Second / sampleRate)
	}
	if ring.Fill() != 0 {
		t.Fatalf("repeated handoff left %d main floats", ring.Fill())
	}
}

func TestWindowsMixerAdvertisesExactImplementedDeliveriesAndPreparesPCM(t *testing.T) {
	ring := NewRing(4096)
	engine, gain := newTestEngine(ring)
	t.Cleanup(engine.Close)
	t.Cleanup(gain.Close)
	mixer := NewWindowsOverlayMediaClipMixer(engine)
	if capabilities := mixer.DeliveryCapabilities(); len(capabilities) != 2 ||
		capabilities[0] != protocol.CapabilityOverlayMix || capabilities[1] != protocol.CapabilityInterruptResume {
		t.Fatalf("capabilities=%v", capabilities)
	}
	path := filepath.Join(t.TempDir(), "overlay.wav")
	if err := os.WriteFile(path, makeWAV16(2, sampleRate, make([]int16, sampleRate/10*channels)), 0o600); err != nil {
		t.Fatal(err)
	}
	clip, err := mixer.Prepare(path, "overlay")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := clip.Decoder.(*windowsOverlayPrepared); !ok {
		t.Fatalf("decoder=%T", clip.Decoder)
	}
	interruptClip, err := mixer.Prepare(path, "interrupt")
	if err != nil {
		t.Fatal(err)
	}
	plan := windowsInterruptPlan(time.Now())
	if err := mixer.Arm(interruptClip, plan, func(int64) {}, func(int64) {}); mediaClipFailureCode(err, "") != "interrupt_capability_lost" {
		t.Fatalf("interrupt arm error=%v", err)
	}
	if engine.InterruptActive() || engine.OverlayActive() {
		t.Fatal("unsupported interrupt must not arm any fallback")
	}
}

func TestWindowsOverlayMixerRunsMediaClientLifecycle(t *testing.T) {
	path := filepath.Join(t.TempDir(), "client-overlay.wav")
	frames := sampleRate / 10
	pcm := make([]int16, frames*channels)
	for index := range pcm {
		pcm[index] = 3_000
	}
	if err := os.WriteFile(path, makeWAV16(2, sampleRate, pcm), 0o600); err != nil {
		t.Fatal(err)
	}
	ring := NewRing(sampleRate * channels)
	fillConstantRing(t, ring, sampleRate/2, 0.2)
	engine, gain := newTestEngine(ring)
	t.Cleanup(engine.Close)
	t.Cleanup(gain.Close)
	mixer := NewWindowsOverlayMediaClipMixer(engine)
	fetcher := &stubMediaClipFetcher{path: path}
	recorder := &mediaEventRecorder{}
	now := time.Now()
	localNowMS := now.UnixMilli()
	engineNow := now
	engine.now = func() time.Time { return engineNow }
	client := NewMediaClipClient(fetcher, mixer, testLogger(), func() int64 { return localNowMS })
	t.Cleanup(client.Stop)
	client.Bind(fixedClock{ok: true}, recorder.send, 0)
	client.Synchronize()

	prepare := testPrepareMedia(1)
	prepare.DurationMS = 100
	prepare.SizeBytes = int64(len(makeWAV16(2, sampleRate, pcm)))
	prepare.PrepareDeadlineCoordMS = localNowMS + 1_000
	prepare.MediaExpiresAtCoordMS = localNowMS + 5_000
	client.Prepare(&prepare)
	events := waitForMediaEvents(t, recorder, 1)
	if events[0].typ != protocol.TypeMediaReady {
		t.Fatalf("prepare events=%+v", events)
	}

	play := testOverlayPlay(1)
	play.TCoordMS = localNowMS + 200
	play.StartDeadlineCoordMS = play.TCoordMS + 100
	client.Play(&play)
	client.Synchronize()
	engineNow = time.UnixMilli(play.TCoordMS)
	dst := make([]float32, 441*channels)
	for index := 0; index < 12; index++ {
		engine.Render(dst)
		engineNow = engineNow.Add(10 * time.Millisecond)
		localNowMS = engineNow.UnixMilli()
	}
	events = waitForMediaEvents(t, recorder, 3)
	if events[1].typ != protocol.TypeMediaStarted || events[2].typ != protocol.TypeMediaEnded ||
		events[1].generation != 1 || events[2].code != "completed" {
		t.Fatalf("lifecycle events=%+v", events)
	}
	if capabilities := client.AdvertisedCapabilities(); len(capabilities) != 3 ||
		capabilities[0] != protocol.CapabilityInterruptResume ||
		capabilities[1] != protocol.CapabilityMediaClip ||
		capabilities[2] != protocol.CapabilityOverlayMix {
		t.Fatalf("client capabilities=%v", capabilities)
	}
}
