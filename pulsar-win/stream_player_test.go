package main

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	protocol "relux.works/duet/pulsar-win/wire"
)

type scriptedWindowsStreamDecoder struct {
	mu         sync.Mutex
	requests   []WindowsStreamDecodeRequest
	frames     int
	value      float32
	gate       <-chan struct{}
	beforeGate int
	failure    error
}

type waitingWindowsStreamDecoder struct{}

func (waitingWindowsStreamDecoder) Decode(ctx context.Context, _ WindowsStreamDecodeRequest) error {
	<-ctx.Done()
	return ctx.Err()
}

func (decoder *scriptedWindowsStreamDecoder) Decode(ctx context.Context, request WindowsStreamDecodeRequest) error {
	decoder.mu.Lock()
	decoder.requests = append(decoder.requests, request)
	frames, value, gate, beforeGate, failure := decoder.frames, decoder.value, decoder.gate, decoder.beforeGate, decoder.failure
	decoder.mu.Unlock()
	start := request.Chunks.ChunkForTime(request.StartPositionMS)
	// Candidate fake consumes every verified chunk so the cache can prove the
	// whole-object digest before publishing EOF.
	for index := 0; index < len(request.Manifest.Chunks); index++ {
		if index < start {
			// A seek may reuse earlier verified chunks; reading them here is test
			// evidence for the whole-object fence, not decoder-owned I/O.
		}
		if _, err := request.Chunks.ReadChunk(ctx, index); err != nil {
			return err
		}
	}
	if failure != nil {
		return failure
	}
	writeFrames := func(count int) error {
		if count <= 0 {
			return nil
		}
		pcm := make([]float32, count*windowsStreamChannels)
		for index := range pcm {
			pcm[index] = value
		}
		return request.PCM.WritePCM(ctx, pcm)
	}
	if gate != nil {
		if beforeGate <= 0 || beforeGate > frames {
			beforeGate = frames
		}
		if err := writeFrames(beforeGate); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-gate:
		}
		if err := writeFrames(frames - beforeGate); err != nil {
			return err
		}
	} else if err := writeFrames(frames); err != nil {
		return err
	}
	return io.EOF
}

func (decoder *scriptedWindowsStreamDecoder) snapshotRequests() []WindowsStreamDecodeRequest {
	decoder.mu.Lock()
	defer decoder.mu.Unlock()
	return append([]WindowsStreamDecodeRequest(nil), decoder.requests...)
}

type windowsStreamSent struct {
	typeName string
	payload  any
}

func protocolStreamLoad(
	manifest WindowsStreamManifest,
	playbackGeneration, positionMS, readyDeadlineMS int64,
) protocol.StreamLoadPayload {
	parts := strings.Split(manifest.VariantURL, "/")
	return protocol.StreamLoadPayload{
		StreamID: "stream_" + manifest.Identity, PlaybackGeneration: playbackGeneration,
		CommandSequence: 1, MediaID: parts[3], VariantManifest: manifest.Identity,
		VariantURL: manifest.VariantURL, VariantETag: manifest.ETag,
		VariantSHA256: manifest.SHA256, VariantSizeBytes: manifest.SizeBytes,
		StartPositionMS: positionMS, MinimumBufferedMS: protocol.StreamMinimumBufferedMS,
		ReadyDeadlineCoordMS: readyDeadlineMS,
		MixedVersionPolicy:   protocol.StreamMixedVersionRequireAll,
	}
}

func newWindowsStreamCandidateTestPlayer(
	t *testing.T,
	manifest WindowsStreamManifest,
	decoder *scriptedWindowsStreamDecoder,
	fetcher *fakeWindowsStreamRangeFetcher,
	clock deadlineClock,
) (*WindowsStreamCandidatePlayer, chan windowsStreamSent, *WindowsStreamChunkCache) {
	t.Helper()
	cache, err := NewWindowsStreamChunkCache(t.TempDir(), []byte("0123456789abcdef"), fetcher)
	if err != nil {
		t.Fatal(err)
	}
	sent := make(chan windowsStreamSent, 64)
	player, err := NewWindowsStreamCandidatePlayer(cache, decoder, clock, func(kind string, payload any) {
		sent <- windowsStreamSent{typeName: kind, payload: payload}
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(player.Close)
	return player, sent, cache
}

func expectWindowsStreamSent(t *testing.T, sent <-chan windowsStreamSent, want string) windowsStreamSent {
	t.Helper()
	select {
	case message := <-sent:
		if message.typeName != want {
			t.Fatalf("message type=%s payload=%+v want=%s", message.typeName, message.payload, want)
		}
		return message
	case <-time.After(5 * time.Second):
		t.Fatalf("timed out waiting for %s", want)
		return windowsStreamSent{}
	}
}

func readWindowsStreamUntilMessage(
	t *testing.T,
	player *WindowsStreamCandidatePlayer,
	sent <-chan windowsStreamSent,
	want string,
) windowsStreamSent {
	t.Helper()
	buffer := make([]float32, 4800*windowsStreamChannels)
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		player.ReadPCM(buffer)
		select {
		case message := <-sent:
			if message.typeName == want {
				return message
			}
			t.Fatalf("message type=%s payload=%+v want=%s", message.typeName, message.payload, want)
		default:
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("timed out rendering until %s", want)
	return windowsStreamSent{}
}

func TestWindowsStreamCandidateLifecyclePauseSeekResumeAndDrainedEnd(t *testing.T) {
	parts := [][]byte{[]byte("chunk-a"), []byte("chunk-b")}
	manifest := windowsStreamManifest("lifecycle", parts...)
	fetcher := newFakeWindowsStreamRangeFetcher(manifest.ETag)
	populateStreamFetcher(fetcher, manifest, parts...)
	decoder := &scriptedWindowsStreamDecoder{
		frames: windowsStreamSampleRateHz * 21 / 10, value: 2,
	}
	player, sent, _ := newWindowsStreamCandidateTestPlayer(t, manifest, decoder, fetcher, fixedClock{ok: true})
	load := protocolStreamLoad(manifest, 1, 0, nowMS()+3000)
	if err := player.Load(load, manifest); err != nil {
		t.Fatal(err)
	}
	ready := expectWindowsStreamSent(t, sent, protocol.TypeStreamReady).payload.(*protocol.StreamReadyPayload)
	if ready.BufferedDurationMS < protocol.StreamMinimumBufferedMS || ready.AudiblePositionMS != 0 {
		t.Fatalf("ready=%+v", ready)
	}
	if snapshot := player.Snapshot(); snapshot.RingBytes != windowsStreamPCMRingBytes ||
		snapshot.RingCeilingBytes != windowsStreamPCMRingBytes || snapshot.State != WindowsStreamReady {
		t.Fatalf("ready snapshot=%+v", snapshot)
	}
	resume := protocol.StreamResumeAtPayload{
		StreamID: load.StreamID, PlaybackGeneration: 1, CommandSequence: 2,
		TCoordMS: nowMS() + 20, StartDeadlineCoordMS: nowMS() + 2000,
	}
	if err := player.ResumeAt(resume); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	player.SetLocalVolume(140)
	first := make([]float32, 480*windowsStreamChannels)
	if n := player.ReadPCM(first); n != len(first) || first[0] != 1 {
		t.Fatalf("first render n=%d sample=%v", n, first[0])
	}
	started := expectWindowsStreamSent(t, sent, protocol.TypeStreamStarted).payload.(*protocol.StreamStartedPayload)
	if started.AudiblePositionMS < 10 || started.PlaybackGeneration != 1 {
		t.Fatalf("started=%+v", started)
	}
	player.Progress()
	progress := expectWindowsStreamSent(t, sent, protocol.TypeStreamProgress).payload.(*protocol.StreamProgressPayload)
	if progress.AudiblePositionMS < 10 || progress.BufferedDurationMS <= 0 {
		t.Fatalf("progress=%+v", progress)
	}
	if err := player.Pause(protocol.StreamPausePayload{
		StreamID: load.StreamID, PlaybackGeneration: 1, CommandSequence: 3, FadeMS: 50,
	}); err != nil {
		t.Fatal(err)
	}
	before := player.Snapshot().BufferedMS
	if n := player.ReadPCM(first); n != 0 || player.Snapshot().BufferedMS != before {
		t.Fatalf("paused render consumed n=%d before=%d after=%d", n, before, player.Snapshot().BufferedMS)
	}
	if err := player.Seek(protocol.StreamSeekPayload{
		StreamID: load.StreamID, PlaybackGeneration: 1, SeekGeneration: 1,
		CommandSequence: 4, PositionMS: 1000,
		MinimumBufferedMS: protocol.StreamMinimumBufferedMS, ReadyDeadlineCoordMS: nowMS() + 3000,
	}); err != nil {
		t.Fatal(err)
	}
	player.ReadPCM(first) // render consumer applies the posted generation cut
	seekReady := expectWindowsStreamSent(t, sent, protocol.TypeStreamReady).payload.(*protocol.StreamReadyPayload)
	if seekReady.SeekGeneration != 1 || seekReady.AudiblePositionMS != 1000 {
		t.Fatalf("seek ready=%+v", seekReady)
	}
	if err := player.ResumeAt(protocol.StreamResumeAtPayload{
		StreamID: load.StreamID, PlaybackGeneration: 1, SeekGeneration: 1,
		CommandSequence: 5, TCoordMS: nowMS() + 20, StartDeadlineCoordMS: nowMS() + 2000,
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(30 * time.Millisecond)
	player.ReadPCM(first)
	seekStarted := expectWindowsStreamSent(t, sent, protocol.TypeStreamStarted).payload.(*protocol.StreamStartedPayload)
	if seekStarted.SeekGeneration != 1 || seekStarted.AudiblePositionMS < 1010 {
		t.Fatalf("seek started=%+v", seekStarted)
	}
	ended := readWindowsStreamUntilMessage(t, player, sent, protocol.TypeStreamEnded).payload.(*protocol.StreamEndedPayload)
	if ended.Reason != "eof_drained" || ended.SeekGeneration != 1 ||
		player.Snapshot().State != WindowsStreamTerminal || player.Snapshot().BufferedMS != 0 {
		t.Fatalf("ended=%+v snapshot=%+v", ended, player.Snapshot())
	}
	requests := decoder.snapshotRequests()
	if len(requests) != 2 || requests[1].StartPositionMS != 1000 || requests[1].SeekGeneration != 1 {
		t.Fatalf("decode requests=%+v", requests)
	}
}

func TestWindowsStreamCandidateRebufferRequiresFreshReadyAndResume(t *testing.T) {
	parts := [][]byte{[]byte("rebuffer-a"), []byte("rebuffer-b")}
	manifest := windowsStreamManifest("rebuffer", parts...)
	fetcher := newFakeWindowsStreamRangeFetcher(manifest.ETag)
	populateStreamFetcher(fetcher, manifest, parts...)
	release := make(chan struct{})
	decoder := &scriptedWindowsStreamDecoder{
		frames: windowsStreamSampleRateHz * 4, beforeGate: windowsStreamSampleRateHz * 2,
		value: 0.25, gate: release,
	}
	player, sent, _ := newWindowsStreamCandidateTestPlayer(t, manifest, decoder, fetcher, fixedClock{ok: true})
	load := protocolStreamLoad(manifest, 1, 0, nowMS()+3000)
	if err := player.Load(load, manifest); err != nil {
		t.Fatal(err)
	}
	expectWindowsStreamSent(t, sent, protocol.TypeStreamReady)
	if err := player.ResumeAt(protocol.StreamResumeAtPayload{
		StreamID: load.StreamID, PlaybackGeneration: 1, CommandSequence: 2,
		TCoordMS: nowMS() + 10, StartDeadlineCoordMS: nowMS() + 2000,
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	buffer := make([]float32, 24_000*windowsStreamChannels)
	player.ReadPCM(buffer)
	expectWindowsStreamSent(t, sent, protocol.TypeStreamStarted)
	rebuffer := readWindowsStreamUntilMessage(t, player, sent, protocol.TypeStreamRebuffer).payload.(*protocol.StreamRebufferPayload)
	if rebuffer.BufferedDurationMS != 0 || player.Snapshot().State != WindowsStreamRebuffering {
		t.Fatalf("rebuffer=%+v snapshot=%+v", rebuffer, player.Snapshot())
	}
	if n := player.ReadPCM(buffer); n != 0 {
		t.Fatalf("rebuffered player consumed %d floats before new resume", n)
	}
	close(release)
	expectWindowsStreamSent(t, sent, protocol.TypeStreamReady)
	if err := player.ResumeAt(protocol.StreamResumeAtPayload{
		StreamID: load.StreamID, PlaybackGeneration: 1, CommandSequence: 3,
		TCoordMS: nowMS() + 10, StartDeadlineCoordMS: nowMS() + 2000,
	}); err != nil {
		t.Fatal(err)
	}
	time.Sleep(20 * time.Millisecond)
	player.ReadPCM(buffer[:960])
	expectWindowsStreamSent(t, sent, protocol.TypeStreamStarted)
}

func TestWindowsStreamCandidateReplacementFencesOldWorkerAndCancelIsExact(t *testing.T) {
	parts := [][]byte{[]byte("replace-a"), []byte("replace-b")}
	manifest := windowsStreamManifest("replace", parts...)
	fetcher := newFakeWindowsStreamRangeFetcher(manifest.ETag)
	populateStreamFetcher(fetcher, manifest, parts...)
	blocked := make(chan struct{})
	decoder := &scriptedWindowsStreamDecoder{
		frames: windowsStreamSampleRateHz * 3, beforeGate: windowsStreamSampleRateHz,
		value: 0.5, gate: blocked,
	}
	player, sent, _ := newWindowsStreamCandidateTestPlayer(t, manifest, decoder, fetcher, fixedClock{ok: true})
	first := protocolStreamLoad(manifest, 1, 0, nowMS()+3000)
	if err := player.Load(first, manifest); err != nil {
		t.Fatal(err)
	}
	second := protocolStreamLoad(manifest, 2, 500, nowMS()+3000)
	if err := player.Load(second, manifest); err != nil {
		t.Fatal(err)
	}
	player.ReadPCM(make([]float32, 480*windowsStreamChannels))
	// The new worker is serialized behind cancellation of the old one; release
	// cannot resurrect generation one because its context and epoch are stale.
	close(blocked)
	ready := expectWindowsStreamSent(t, sent, protocol.TypeStreamReady).payload.(*protocol.StreamReadyPayload)
	if ready.PlaybackGeneration != 2 || ready.AudiblePositionMS != 500 {
		t.Fatalf("replacement ready=%+v", ready)
	}
	if err := player.Cancel(protocol.StreamCancelPayload{
		StreamID: second.StreamID, PlaybackGeneration: 2, CommandSequence: 2, Reason: "replaced",
	}); err != nil {
		t.Fatal(err)
	}
	cancelled := expectWindowsStreamSent(t, sent, protocol.TypeStreamCancelled).payload.(*protocol.StreamCancelledPayload)
	if cancelled.PlaybackGeneration != 2 || cancelled.Reason != "replaced" || player.Snapshot().BufferedMS != 0 {
		t.Fatalf("cancelled=%+v snapshot=%+v", cancelled, player.Snapshot())
	}
	if err := player.Load(first, manifest); err != nil {
		t.Fatal(err)
	}
	select {
	case message := <-sent:
		t.Fatalf("stale generation emitted %+v", message)
	case <-time.After(30 * time.Millisecond):
	}
}

func TestWindowsStreamCandidateRevocationPurgesAndReportsSanitizedFailure(t *testing.T) {
	parts := [][]byte{[]byte("revoked-a"), []byte("revoked-b")}
	manifest := windowsStreamManifest("revoked", parts...)
	fetcher := newFakeWindowsStreamRangeFetcher(manifest.ETag)
	fetcher.err = windowsStreamFailure("fetch", "revoked")
	decoder := &scriptedWindowsStreamDecoder{frames: windowsStreamSampleRateHz * 3, value: 0.5}
	player, sent, cache := newWindowsStreamCandidateTestPlayer(t, manifest, decoder, fetcher, fixedClock{ok: true})
	load := protocolStreamLoad(manifest, 1, 0, nowMS()+3000)
	if err := player.Load(load, manifest); err != nil {
		t.Fatal(err)
	}
	failed := expectWindowsStreamSent(t, sent, protocol.TypeStreamFailed).payload.(*protocol.StreamFailedPayload)
	if failed.Stage != "fetch" || failed.Code != "revoked" || strings.Contains(failed.Code, manifest.VariantURL) {
		t.Fatalf("failed=%+v", failed)
	}
	if cache.Stats().Bytes != 0 {
		t.Fatalf("revoked cache=%+v", cache.Stats())
	}
	if _, err := cache.Get(context.Background(), manifest, 0); err == nil {
		t.Fatal("revoked cache permitted refill")
	}
}

func TestWindowsStreamCandidateExplicitRevokeCancelsAndDeniesRefill(t *testing.T) {
	parts := [][]byte{[]byte("explicit-revoke-a"), []byte("explicit-revoke-b")}
	manifest := windowsStreamManifest("explicit_revoke", parts...)
	fetcher := newFakeWindowsStreamRangeFetcher(manifest.ETag)
	populateStreamFetcher(fetcher, manifest, parts...)
	decoder := &scriptedWindowsStreamDecoder{
		frames: windowsStreamSampleRateHz * 21 / 10, value: 0.5,
	}
	player, sent, cache := newWindowsStreamCandidateTestPlayer(t, manifest, decoder, fetcher, fixedClock{ok: true})
	load := protocolStreamLoad(manifest, 1, 0, nowMS()+3000)
	if err := player.Load(load, manifest); err != nil {
		t.Fatal(err)
	}
	expectWindowsStreamSent(t, sent, protocol.TypeStreamReady)
	if cache.Stats().Bytes == 0 {
		t.Fatal("candidate cache was not populated before revoke")
	}
	if err := player.Revoke(); err != nil {
		t.Fatal(err)
	}
	if snapshot := player.Snapshot(); snapshot.State != WindowsStreamTerminal ||
		snapshot.BufferedMS != 0 || snapshot.Cache.Bytes != 0 || snapshot.Cache.PinnedBytes != 0 {
		t.Fatalf("revoked snapshot=%+v", snapshot)
	}
	if n := player.ReadPCM(make([]float32, 480*windowsStreamChannels)); n != 0 {
		t.Fatalf("revoked generation rendered %d floats", n)
	}
	before := fetcher.callCount(manifest.Chunks[0].Start, manifest.Chunks[0].End)
	if _, err := cache.Get(context.Background(), manifest, 0); err == nil {
		t.Fatal("explicit revoke allowed cache refill")
	} else if _, code := windowsStreamFailureCode(err); code != "revoked" {
		t.Fatalf("explicit revoke error=%v", err)
	}
	if fetcher.callCount(manifest.Chunks[0].Start, manifest.Chunks[0].End) != before {
		t.Fatal("explicit revoke reached network")
	}
	select {
	case message := <-sent:
		t.Fatalf("explicit revoke emitted an invented wire event: %+v", message)
	case <-time.After(20 * time.Millisecond):
	}
}

func TestWindowsStreamCandidateBoundsAreDurationIndependentAndProductionRemainsNoGo(t *testing.T) {
	parts := [][]byte{[]byte("long-a"), []byte("long-b")}
	manifest := windowsStreamManifest("two_hour", parts...)
	manifest.DurationMS = 2 * 60 * 60 * 1000
	fetcher := newFakeWindowsStreamRangeFetcher(manifest.ETag)
	populateStreamFetcher(fetcher, manifest, parts...)
	decoder := &scriptedWindowsStreamDecoder{frames: windowsStreamSampleRateHz * 21 / 10, value: 0.2}
	player, sent, _ := newWindowsStreamCandidateTestPlayer(t, manifest, decoder, fetcher, fixedClock{ok: true})
	load := protocolStreamLoad(manifest, 1, 0, nowMS()+3000)
	if err := player.Load(load, manifest); err != nil {
		t.Fatal(err)
	}
	expectWindowsStreamSent(t, sent, protocol.TypeStreamReady)
	snapshot := player.Snapshot()
	if snapshot.RingBytes != 1<<20 || snapshot.Cache.Bytes > windowsStreamCachePerVariantBytes ||
		snapshot.Cache.PinnedBytes > windowsStreamCachePinnedBytes {
		t.Fatalf("duration-dependent bounds=%+v", snapshot)
	}
	mainSource, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(mainSource), "CapabilityStreamTrack") {
		t.Fatal("production composition advertised candidate stream player")
	}
	playerSource, err := os.ReadFile("player.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(playerSource), "rejecting unadvertised stream_track_v1 command") ||
		strings.Contains(string(playerSource), "NewWindowsStreamCandidatePlayer") {
		t.Fatal("production Player no-go boundary changed")
	}
}

func TestWindowsStreamCandidateClockFailureDoesNotConsumeGenerationCommand(t *testing.T) {
	parts := [][]byte{[]byte("clock-a"), []byte("clock-b")}
	manifest := windowsStreamManifest("clock", parts...)
	fetcher := newFakeWindowsStreamRangeFetcher(manifest.ETag)
	populateStreamFetcher(fetcher, manifest, parts...)
	decoder := &scriptedWindowsStreamDecoder{frames: windowsStreamSampleRateHz * 21 / 10, value: 0.2}
	player, sent, _ := newWindowsStreamCandidateTestPlayer(t, manifest, decoder, fetcher, fixedClock{ok: false})
	load := protocolStreamLoad(manifest, 1, 0, nowMS()+3000)
	if err := player.Load(load, manifest); err != nil {
		t.Fatal(err)
	}
	failed := expectWindowsStreamSent(t, sent, protocol.TypeStreamFailed).payload.(*protocol.StreamFailedPayload)
	if failed.Stage != "scheduler" || failed.Code != "clock_unsynchronized" {
		t.Fatalf("failed=%+v", failed)
	}
	if err := player.ResumeAt(protocol.StreamResumeAtPayload{
		StreamID: load.StreamID, PlaybackGeneration: 1, CommandSequence: 2,
		TCoordMS: nowMS() + 10, StartDeadlineCoordMS: nowMS() + 1000,
	}); err == nil {
		t.Fatal("terminal unsynchronized generation resumed")
	}
}

func TestWindowsStreamCandidateDecoderFailureUsesBoundedCode(t *testing.T) {
	parts := [][]byte{[]byte("decode-a"), []byte("decode-b")}
	manifest := windowsStreamManifest("decode_failure", parts...)
	fetcher := newFakeWindowsStreamRangeFetcher(manifest.ETag)
	populateStreamFetcher(fetcher, manifest, parts...)
	decoder := &scriptedWindowsStreamDecoder{failure: windowsStreamFailure("decoder", "candidate_failed")}
	player, sent, _ := newWindowsStreamCandidateTestPlayer(t, manifest, decoder, fetcher, fixedClock{ok: true})
	if err := player.Load(protocolStreamLoad(manifest, 1, 0, nowMS()+3000), manifest); err != nil {
		t.Fatal(err)
	}
	failed := expectWindowsStreamSent(t, sent, protocol.TypeStreamFailed).payload.(*protocol.StreamFailedPayload)
	if failed.Stage != "decoder" || failed.Code != "candidate_failed" {
		t.Fatalf("failed=%+v", failed)
	}
}

func TestWindowsStreamCandidateReadyAndFirstSampleDeadlinesFailClosed(t *testing.T) {
	t.Run("ready deadline", func(t *testing.T) {
		parts := [][]byte{[]byte("timeout-a"), []byte("timeout-b")}
		manifest := windowsStreamManifest("ready_timeout", parts...)
		fetcher := newFakeWindowsStreamRangeFetcher(manifest.ETag)
		populateStreamFetcher(fetcher, manifest, parts...)
		cache, err := NewWindowsStreamChunkCache(t.TempDir(), []byte("0123456789abcdef"), fetcher)
		if err != nil {
			t.Fatal(err)
		}
		sent := make(chan windowsStreamSent, 8)
		player, err := NewWindowsStreamCandidatePlayer(cache, waitingWindowsStreamDecoder{}, fixedClock{ok: true}, func(kind string, payload any) {
			sent <- windowsStreamSent{typeName: kind, payload: payload}
		})
		if err != nil {
			t.Fatal(err)
		}
		defer player.Close()
		if err := player.Load(protocolStreamLoad(manifest, 1, 0, nowMS()+40), manifest); err != nil {
			t.Fatal(err)
		}
		failed := expectWindowsStreamSent(t, sent, protocol.TypeStreamFailed).payload.(*protocol.StreamFailedPayload)
		if failed.Stage != "buffer" || failed.Code != "ready_timeout" {
			t.Fatalf("ready timeout=%+v", failed)
		}
	})

	t.Run("first audible sample deadline", func(t *testing.T) {
		parts := [][]byte{[]byte("start-a"), []byte("start-b")}
		manifest := windowsStreamManifest("start_timeout", parts...)
		fetcher := newFakeWindowsStreamRangeFetcher(manifest.ETag)
		populateStreamFetcher(fetcher, manifest, parts...)
		decoder := &scriptedWindowsStreamDecoder{frames: windowsStreamSampleRateHz * 21 / 10, value: 0.2}
		player, sent, _ := newWindowsStreamCandidateTestPlayer(t, manifest, decoder, fetcher, fixedClock{ok: true})
		load := protocolStreamLoad(manifest, 1, 0, nowMS()+1000)
		if err := player.Load(load, manifest); err != nil {
			t.Fatal(err)
		}
		expectWindowsStreamSent(t, sent, protocol.TypeStreamReady)
		now := nowMS()
		if err := player.ResumeAt(protocol.StreamResumeAtPayload{
			StreamID: load.StreamID, PlaybackGeneration: 1, CommandSequence: 2,
			TCoordMS: now + 10, StartDeadlineCoordMS: now + 50,
		}); err != nil {
			t.Fatal(err)
		}
		failed := expectWindowsStreamSent(t, sent, protocol.TypeStreamFailed).payload.(*protocol.StreamFailedPayload)
		if failed.Stage != "scheduler" || failed.Code != "start_timeout" {
			t.Fatalf("start timeout=%+v", failed)
		}
	})
}

func TestWindowsStreamCandidateSanitizesInjectedFailureTokens(t *testing.T) {
	parts := [][]byte{[]byte("sanitize-a"), []byte("sanitize-b")}
	manifest := windowsStreamManifest("sanitize", parts...)
	fetcher := newFakeWindowsStreamRangeFetcher(manifest.ETag)
	populateStreamFetcher(fetcher, manifest, parts...)
	decoder := &scriptedWindowsStreamDecoder{failure: &WindowsStreamFailure{
		Stage: "decoder/C:/private", Code: "token=https://secret.example",
	}}
	player, sent, _ := newWindowsStreamCandidateTestPlayer(t, manifest, decoder, fetcher, fixedClock{ok: true})
	if err := player.Load(protocolStreamLoad(manifest, 1, 0, nowMS()+1000), manifest); err != nil {
		t.Fatal(err)
	}
	failed := expectWindowsStreamSent(t, sent, protocol.TypeStreamFailed).payload.(*protocol.StreamFailedPayload)
	if failed.Stage != "internal" || failed.Code != "internal_error" {
		t.Fatalf("unsanitized failure=%+v", failed)
	}
}
