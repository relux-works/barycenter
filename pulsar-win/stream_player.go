package main

import (
	"context"
	"errors"
	"io"
	"math"
	"reflect"
	"sync"
	"sync/atomic"
	"time"

	protocol "relux.works/duet/pulsar-win/wire"
)

const (
	windowsStreamSampleRateHz   = 48_000
	windowsStreamChannels       = 2
	windowsStreamPCMRingBytes   = 1 << 20
	windowsStreamPCMRingFloats  = windowsStreamPCMRingBytes / 4
	windowsStreamProgressPeriod = time.Second
	windowsStreamWorkerBackoff  = 200 * time.Microsecond
)

type WindowsStreamChunkReader interface {
	Manifest() WindowsStreamManifest
	ChunkForTime(int64) int
	ReadChunk(context.Context, int) ([]byte, error)
}

type windowsStreamChunkVerifier interface {
	VerifyWhole() error
}

type windowsStreamChunkRevoker interface {
	Revoke() error
}

type windowsStreamChunkCloser interface {
	Close()
}

type WindowsStreamPCMWriter interface {
	WritePCM(context.Context, []float32) error
}

type WindowsStreamDecodeRequest struct {
	Manifest           WindowsStreamManifest
	StartPositionMS    int64
	PlaybackGeneration int64
	SeekGeneration     int64
	Chunks             WindowsStreamChunkReader
	PCM                WindowsStreamPCMWriter
}

// WindowsStreamCandidateDecoder is deliberately an injected test seam. The
// accepted ADR registers no production implementation.
type WindowsStreamCandidateDecoder interface {
	Decode(context.Context, WindowsStreamDecodeRequest) error
}

type WindowsStreamPlayerState string

const (
	WindowsStreamIdle        WindowsStreamPlayerState = "idle"
	WindowsStreamLoading     WindowsStreamPlayerState = "loading"
	WindowsStreamReady       WindowsStreamPlayerState = "ready"
	WindowsStreamPlaying     WindowsStreamPlayerState = "playing"
	WindowsStreamPaused      WindowsStreamPlayerState = "paused"
	WindowsStreamRebuffering WindowsStreamPlayerState = "rebuffering"
	WindowsStreamTerminal    WindowsStreamPlayerState = "terminal"
)

type WindowsStreamPlayerSnapshot struct {
	State                              WindowsStreamPlayerState
	StreamID                           string
	PlaybackGeneration, SeekGeneration int64
	AudiblePositionMS, BufferedMS      int64
	RingBytes, RingCeilingBytes        int64
	Volume                             int
	Cache                              WindowsStreamCacheStats
}

type windowsStreamGenerationToken struct {
	Playback, Seek, Epoch int64
}

type windowsStreamInternalKind uint8

const (
	windowsStreamInternalReady windowsStreamInternalKind = iota
	windowsStreamInternalArm
	windowsStreamInternalStarted
	windowsStreamInternalRebuffer
	windowsStreamInternalDrained
	windowsStreamInternalFailed
)

type windowsStreamInternalEvent struct {
	kind  windowsStreamInternalKind
	token windowsStreamGenerationToken
	err   error
}

type windowsStreamCacheReader struct {
	cache    *WindowsStreamChunkCache
	manifest WindowsStreamManifest
}

func (reader *windowsStreamCacheReader) Manifest() WindowsStreamManifest { return reader.manifest }
func (reader *windowsStreamCacheReader) ChunkForTime(positionMS int64) int {
	return reader.manifest.ChunkForTime(positionMS)
}
func (reader *windowsStreamCacheReader) ReadChunk(ctx context.Context, index int) ([]byte, error) {
	data, err := reader.cache.Get(ctx, reader.manifest, index)
	if err != nil {
		return nil, err
	}
	pins := []int{index}
	if index+1 < len(reader.manifest.Chunks) {
		pins = append(pins, index+1)
	}
	if err := reader.cache.SetPinned(reader.manifest, pins); err != nil {
		return nil, err
	}
	return data, nil
}

type windowsStreamPCMWriter struct {
	player *WindowsStreamCandidatePlayer
	token  windowsStreamGenerationToken
}

func (writer windowsStreamPCMWriter) WritePCM(ctx context.Context, samples []float32) error {
	if len(samples) == 0 || len(samples)%windowsStreamChannels != 0 {
		return windowsStreamFailure("decoder", "invalid_pcm")
	}
	for len(samples) > 0 {
		if ctx.Err() != nil || writer.player.epoch.Load() != writer.token.Epoch {
			return context.Canceled
		}
		written := writer.player.ring.Write(samples)
		if writer.player.epoch.Load() != writer.token.Epoch {
			writer.player.ring.Clear()
			return context.Canceled
		}
		samples = samples[written:]
		if writer.player.readyWanted.Load() &&
			writer.player.ring.FillMS(windowsStreamSampleRateHz, windowsStreamChannels) >= protocol.StreamMinimumBufferedMS &&
			writer.player.readyWanted.CompareAndSwap(true, false) {
			if err := writer.player.postWorkerEvent(ctx, windowsStreamInternalEvent{
				kind: windowsStreamInternalReady, token: writer.token,
			}); err != nil {
				return err
			}
		}
		if written == 0 {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-writer.player.done:
				return context.Canceled
			case <-time.After(windowsStreamWorkerBackoff):
			}
		}
	}
	return nil
}

// WindowsStreamCandidatePlayer implements the frozen lifecycle without being
// composed into main. ReadPCM is the future render seam: it takes no mutex,
// allocates nothing and never calls cache, network or decoder code.
type WindowsStreamCandidatePlayer struct {
	cache   *WindowsStreamChunkCache
	decoder WindowsStreamCandidateDecoder
	clock   deadlineClock
	send    func(string, any)
	ring    *Ring
	chunks  WindowsStreamChunkReader

	mu            sync.Mutex
	guard         protocol.StreamGenerationGuard
	state         WindowsStreamPlayerState
	load          *protocol.StreamLoadPayload
	manifest      WindowsStreamManifest
	decoderCancel context.CancelFunc
	readyTimer    *time.Timer
	startTimer    *time.Timer
	startExpiry   *time.Timer
	decoderMu     sync.Mutex

	events chan windowsStreamInternalEvent
	done   chan struct{}
	close  sync.Once

	epoch              atomic.Int64
	playbackGeneration atomic.Int64
	seekGeneration     atomic.Int64
	audibleAnchorMS    atomic.Int64
	renderedFrames     atomic.Int64
	armed              atomic.Bool
	readyWanted        atomic.Bool
	startedPosted      atomic.Bool
	rebufferPosted     atomic.Bool
	endedPosted        atomic.Bool
	decoderEOF         atomic.Bool
	volume             atomic.Int64
	underruns          atomic.Int64
}

func NewWindowsStreamCandidatePlayer(
	cache *WindowsStreamChunkCache,
	decoder WindowsStreamCandidateDecoder,
	clock deadlineClock,
	send func(string, any),
) (*WindowsStreamCandidatePlayer, error) {
	return newWindowsStreamCandidatePlayer(cache, decoder, clock, nil, send)
}

// newWindowsStreamCandidatePlayer accepts an already-authenticated chunk
// reader without changing the clear-stream production-dark constructor. The
// injected reader owns any protected-media lease and is never exposed to the
// decoder as transport or cache access.
func newWindowsStreamCandidatePlayer(
	cache *WindowsStreamChunkCache,
	decoder WindowsStreamCandidateDecoder,
	clock deadlineClock,
	chunks WindowsStreamChunkReader,
	send func(string, any),
) (*WindowsStreamCandidatePlayer, error) {
	if cache == nil || decoder == nil || clock == nil || send == nil {
		return nil, windowsStreamFailure("player", "invalid_configuration")
	}
	player := &WindowsStreamCandidatePlayer{
		cache: cache, decoder: decoder, clock: clock, send: send,
		chunks: chunks,
		ring:   NewRing(windowsStreamPCMRingFloats), state: WindowsStreamIdle,
		events: make(chan windowsStreamInternalEvent, 32), done: make(chan struct{}),
	}
	player.volume.Store(100)
	go player.dispatch()
	return player, nil
}

func (player *WindowsStreamCandidatePlayer) Close() {
	player.close.Do(func() {
		player.mu.Lock()
		if player.load != nil {
			_ = player.cache.SetPinned(player.manifest, nil)
		}
		player.cancelRuntimeLocked()
		player.mu.Unlock()
		close(player.done)
		// Cancellation is only a request. Join the serialized decoder/cache
		// worker before returning so callers may safely remove its cache root or
		// release package resources immediately after Close.
		player.decoderMu.Lock()
		player.decoderMu.Unlock()
		if closer, ok := player.chunks.(windowsStreamChunkCloser); ok {
			closer.Close()
		}
	})
}

func (player *WindowsStreamCandidatePlayer) Load(
	payload protocol.StreamLoadPayload,
	manifest WindowsStreamManifest,
) error {
	if err := validateWindowsStreamManifest(payload, manifest); err != nil {
		return err
	}
	if player.chunks != nil && !reflect.DeepEqual(player.chunks.Manifest(), manifest) {
		return windowsStreamFailure("manifest", "invalid_manifest")
	}
	player.mu.Lock()
	decision := player.guard.AcceptLoad(
		payload.PlaybackGeneration, payload.SeekGeneration, payload.CommandSequence,
	)
	if decision == protocol.StreamGenerationDuplicate || decision == protocol.StreamGenerationStale {
		player.mu.Unlock()
		return nil
	}
	if decision != protocol.StreamGenerationApply {
		player.mu.Unlock()
		return windowsStreamFailure("generation", "invalid_command")
	}
	if player.load != nil {
		_ = player.cache.SetPinned(player.manifest, nil)
	}
	player.cancelRuntimeLocked()
	copyPayload := payload
	player.load = &copyPayload
	player.manifest = manifest
	player.state = WindowsStreamLoading
	token := player.resetGenerationLocked(payload.PlaybackGeneration, 0, payload.StartPositionMS)
	player.armReadyDeadlineLocked(token, payload.ReadyDeadlineCoordMS)
	player.startDecoderLocked(token, payload.StartPositionMS)
	player.mu.Unlock()
	return nil
}

func (player *WindowsStreamCandidatePlayer) ResumeAt(payload protocol.StreamResumeAtPayload) error {
	if payload.StreamID == "" || payload.PlaybackGeneration <= 0 || payload.SeekGeneration < 0 ||
		payload.CommandSequence <= 0 || payload.TCoordMS <= 0 ||
		payload.StartDeadlineCoordMS < payload.TCoordMS {
		return windowsStreamFailure("scheduler", "invalid_command")
	}
	startLocal, startOK := player.clock.LocalDeadline(payload.TCoordMS, 0)
	deadlineLocal, deadlineOK := player.clock.LocalDeadline(payload.StartDeadlineCoordMS, 0)
	if !startOK || !deadlineOK {
		return windowsStreamFailure("scheduler", "clock_unsynchronized")
	}
	now := nowMS()
	if startLocal > deadlineLocal || now > deadlineLocal {
		return windowsStreamFailure("scheduler", "start_timeout")
	}
	player.mu.Lock()
	if player.load == nil || payload.StreamID != player.load.StreamID {
		player.mu.Unlock()
		return nil
	}
	decision := player.guard.AcceptCommand(
		payload.PlaybackGeneration, payload.SeekGeneration, payload.CommandSequence, "resume",
	)
	if decision == protocol.StreamGenerationDuplicate || decision == protocol.StreamGenerationStale {
		player.mu.Unlock()
		return nil
	}
	if decision != protocol.StreamGenerationApply {
		player.mu.Unlock()
		return windowsStreamFailure("generation", "invalid_command")
	}
	if player.startTimer != nil {
		player.startTimer.Stop()
	}
	if player.startExpiry != nil {
		player.startExpiry.Stop()
	}
	token := player.tokenLocked()
	delay := time.Duration(max(startLocal-now, 0)) * time.Millisecond
	player.startTimer = time.AfterFunc(delay, func() {
		player.postInternal(windowsStreamInternalEvent{kind: windowsStreamInternalArm, token: token})
	})
	player.startExpiry = time.AfterFunc(time.Duration(max(deadlineLocal-now, 0))*time.Millisecond, func() {
		if player.epoch.Load() == token.Epoch && !player.startedPosted.Load() {
			player.postInternal(windowsStreamInternalEvent{
				kind: windowsStreamInternalFailed, token: token,
				err: windowsStreamFailure("scheduler", "start_timeout"),
			})
		}
	})
	player.mu.Unlock()
	return nil
}

func (player *WindowsStreamCandidatePlayer) Pause(payload protocol.StreamPausePayload) error {
	if payload.StreamID == "" || payload.FadeMS < 0 || payload.FadeMS > 1000 {
		return windowsStreamFailure("player", "invalid_command")
	}
	player.mu.Lock()
	defer player.mu.Unlock()
	if player.load == nil || payload.StreamID != player.load.StreamID {
		return nil
	}
	decision := player.guard.AcceptCommand(
		payload.PlaybackGeneration, payload.SeekGeneration, payload.CommandSequence, "pause",
	)
	if decision == protocol.StreamGenerationDuplicate || decision == protocol.StreamGenerationStale {
		return nil
	}
	if decision != protocol.StreamGenerationApply {
		return windowsStreamFailure("generation", "invalid_command")
	}
	if player.startTimer != nil {
		player.startTimer.Stop()
	}
	player.armed.Store(false)
	player.startedPosted.Store(false)
	player.state = WindowsStreamPaused
	return nil
}

func (player *WindowsStreamCandidatePlayer) Seek(payload protocol.StreamSeekPayload) error {
	if payload.StreamID == "" || payload.PlaybackGeneration <= 0 || payload.SeekGeneration <= 0 ||
		payload.CommandSequence <= 0 || payload.PositionMS < 0 ||
		payload.MinimumBufferedMS != protocol.StreamMinimumBufferedMS || payload.ReadyDeadlineCoordMS <= 0 {
		return windowsStreamFailure("player", "invalid_command")
	}
	player.mu.Lock()
	if player.load == nil || payload.StreamID != player.load.StreamID {
		player.mu.Unlock()
		return nil
	}
	if payload.PositionMS > player.manifest.DurationMS {
		player.mu.Unlock()
		return windowsStreamFailure("player", "invalid_command")
	}
	decision := player.guard.AcceptSeek(
		payload.PlaybackGeneration, payload.SeekGeneration, payload.CommandSequence,
	)
	if decision == protocol.StreamGenerationDuplicate || decision == protocol.StreamGenerationStale {
		player.mu.Unlock()
		return nil
	}
	if decision != protocol.StreamGenerationApply {
		player.mu.Unlock()
		return windowsStreamFailure("generation", "invalid_command")
	}
	player.cancelRuntimeLocked()
	player.state = WindowsStreamLoading
	token := player.resetGenerationLocked(payload.PlaybackGeneration, payload.SeekGeneration, payload.PositionMS)
	player.armReadyDeadlineLocked(token, payload.ReadyDeadlineCoordMS)
	player.startDecoderLocked(token, payload.PositionMS)
	player.mu.Unlock()
	return nil
}

func (player *WindowsStreamCandidatePlayer) Cancel(payload protocol.StreamCancelPayload) error {
	if payload.StreamID == "" || payload.Reason == "" {
		return windowsStreamFailure("player", "invalid_command")
	}
	player.mu.Lock()
	if player.load == nil || payload.StreamID != player.load.StreamID {
		player.mu.Unlock()
		return nil
	}
	decision := player.guard.AcceptCommand(
		payload.PlaybackGeneration, payload.SeekGeneration, payload.CommandSequence, "cancel",
	)
	if decision == protocol.StreamGenerationDuplicate || decision == protocol.StreamGenerationStale {
		player.mu.Unlock()
		return nil
	}
	if decision != protocol.StreamGenerationApply {
		player.mu.Unlock()
		return windowsStreamFailure("generation", "invalid_command")
	}
	player.cancelRuntimeLocked()
	player.epoch.Add(1)
	player.ring.Clear()
	position := player.audiblePosition()
	sequence := player.guard.EventSequence + 1
	if player.guard.AcceptEvent(
		payload.PlaybackGeneration, payload.SeekGeneration, sequence, protocol.StreamEventCancelled,
	) != protocol.StreamGenerationApply {
		player.mu.Unlock()
		return windowsStreamFailure("generation", "invalid_event")
	}
	streamID := player.load.StreamID
	manifest := player.manifest
	player.state = WindowsStreamTerminal
	player.mu.Unlock()
	_ = player.cache.SetPinned(manifest, nil)
	player.send(protocol.TypeStreamCancelled, &protocol.StreamCancelledPayload{
		StreamID: streamID, PlaybackGeneration: payload.PlaybackGeneration,
		SeekGeneration: payload.SeekGeneration, EventSequence: sequence,
		AudiblePositionMS: position, Reason: payload.Reason,
	})
	return nil
}

func (player *WindowsStreamCandidatePlayer) Revoke() error {
	player.mu.Lock()
	if player.load == nil {
		player.mu.Unlock()
		return nil
	}
	manifest := player.manifest
	player.cancelRuntimeLocked()
	player.epoch.Add(1)
	player.ring.Clear()
	player.state = WindowsStreamTerminal
	player.mu.Unlock()
	if revoker, ok := player.chunks.(windowsStreamChunkRevoker); ok {
		return revoker.Revoke()
	}
	return player.cache.Tombstone(manifest)
}

// ReadPCM is render-safe: Ring.Read plus atomics and arithmetic only. The
// caller supplies the buffer, and the method never waits or takes a mutex.
func (player *WindowsStreamCandidatePlayer) ReadPCM(dst []float32) int {
	if len(dst) == 0 || len(dst)%windowsStreamChannels != 0 {
		clear(dst)
		return 0
	}
	if !player.armed.Load() {
		// Apply Ring.Clear on the sole render consumer before a replacement
		// decoder reuses the reclaimed capacity. No samples are consumed.
		player.ring.Read(nil)
		clear(dst)
		return 0
	}
	epoch := player.epoch.Load()
	n := player.ring.Read(dst)
	if player.epoch.Load() != epoch {
		clear(dst)
		return 0
	}
	if n < len(dst) {
		clear(dst[n:])
	}
	volume := player.volume.Load()
	gain := float32(float64(volume*volume) / 10_000)
	for index := 0; index < n; index++ {
		value := dst[index] * gain
		if value > 1 {
			value = 1
		} else if value < -1 {
			value = -1
		}
		dst[index] = value
	}
	if n > 0 {
		player.renderedFrames.Add(int64(n / windowsStreamChannels))
		if player.startedPosted.CompareAndSwap(false, true) {
			player.postRenderEvent(windowsStreamInternalStarted, epoch)
		}
	}
	if n < len(dst) {
		if player.decoderEOF.Load() && player.ring.Fill() == 0 {
			if player.endedPosted.CompareAndSwap(false, true) {
				player.postRenderEvent(windowsStreamInternalDrained, epoch)
			}
		} else if !player.decoderEOF.Load() && player.startedPosted.Load() {
			player.underruns.Add(1)
			if player.rebufferPosted.CompareAndSwap(false, true) {
				player.armed.Store(false)
				player.postRenderEvent(windowsStreamInternalRebuffer, epoch)
			}
		}
	}
	return n
}

func (player *WindowsStreamCandidatePlayer) SetLocalVolume(volume int) {
	player.volume.Store(int64(min(max(volume, 0), 100)))
}

func (player *WindowsStreamCandidatePlayer) Progress() {
	player.mu.Lock()
	if player.load == nil || player.guard.Phase != "started" {
		player.mu.Unlock()
		return
	}
	position := player.audiblePosition()
	buffered := player.ring.FillMS(windowsStreamSampleRateHz, windowsStreamChannels)
	sequence := player.guard.EventSequence + 1
	if player.guard.AcceptEvent(
		player.guard.PlaybackGeneration, player.guard.SeekGeneration, sequence, protocol.StreamEventProgress,
	) != protocol.StreamGenerationApply {
		player.mu.Unlock()
		return
	}
	payload := protocol.StreamProgressPayload{
		StreamID: player.load.StreamID, PlaybackGeneration: player.guard.PlaybackGeneration,
		SeekGeneration: player.guard.SeekGeneration, EventSequence: sequence,
		AudiblePositionMS: position, BufferedDurationMS: buffered,
	}
	player.mu.Unlock()
	player.send(protocol.TypeStreamProgress, &payload)
}

func (player *WindowsStreamCandidatePlayer) Snapshot() WindowsStreamPlayerSnapshot {
	player.mu.Lock()
	defer player.mu.Unlock()
	snapshot := WindowsStreamPlayerSnapshot{
		State: player.state, PlaybackGeneration: player.guard.PlaybackGeneration,
		SeekGeneration: player.guard.SeekGeneration, AudiblePositionMS: player.audiblePosition(),
		BufferedMS: player.ring.FillMS(windowsStreamSampleRateHz, windowsStreamChannels),
		RingBytes:  int64(player.ring.Capacity() * 4), RingCeilingBytes: windowsStreamPCMRingBytes,
		Volume: int(player.volume.Load()), Cache: player.cache.Stats(),
	}
	if player.load != nil {
		snapshot.StreamID = player.load.StreamID
	}
	return snapshot
}

func (player *WindowsStreamCandidatePlayer) dispatch() {
	for {
		select {
		case event := <-player.events:
			player.handleInternal(event)
		case <-player.done:
			return
		}
	}
}

func (player *WindowsStreamCandidatePlayer) handleInternal(event windowsStreamInternalEvent) {
	player.mu.Lock()
	if player.load == nil || event.token != player.tokenLocked() || player.state == WindowsStreamTerminal {
		player.mu.Unlock()
		return
	}
	switch event.kind {
	case windowsStreamInternalReady:
		buffered := player.ring.FillMS(windowsStreamSampleRateHz, windowsStreamChannels)
		sequence := player.guard.EventSequence + 1
		if player.guard.AcceptReady(
			event.token.Playback, event.token.Seek, sequence, buffered, protocol.StreamMinimumBufferedMS,
		) != protocol.StreamGenerationApply {
			player.mu.Unlock()
			return
		}
		if player.readyTimer != nil {
			player.readyTimer.Stop()
		}
		if player.guard.Phase == "paused_ready" {
			player.state = WindowsStreamPaused
		} else {
			player.state = WindowsStreamReady
		}
		player.rebufferPosted.Store(false)
		payload := protocol.StreamReadyPayload{
			StreamID: player.load.StreamID, PlaybackGeneration: event.token.Playback,
			SeekGeneration: event.token.Seek, EventSequence: sequence,
			AudiblePositionMS: player.audiblePosition(), BufferedDurationMS: buffered,
		}
		player.mu.Unlock()
		player.send(protocol.TypeStreamReady, &payload)
	case windowsStreamInternalArm:
		if player.guard.Phase != "ready" {
			player.mu.Unlock()
			return
		}
		player.state = WindowsStreamReady
		player.armed.Store(true)
		player.mu.Unlock()
	case windowsStreamInternalStarted:
		sequence := player.guard.EventSequence + 1
		if player.guard.AcceptEvent(
			event.token.Playback, event.token.Seek, sequence, protocol.StreamEventStarted,
		) != protocol.StreamGenerationApply {
			player.mu.Unlock()
			return
		}
		player.state = WindowsStreamPlaying
		if player.startExpiry != nil {
			player.startExpiry.Stop()
		}
		payload := protocol.StreamStartedPayload{
			StreamID: player.load.StreamID, PlaybackGeneration: event.token.Playback,
			SeekGeneration: event.token.Seek, EventSequence: sequence,
			AudiblePositionMS: player.audiblePosition(), TFirstSampleCoordMS: player.coordinatorNowMS(),
		}
		player.mu.Unlock()
		player.send(protocol.TypeStreamStarted, &payload)
	case windowsStreamInternalRebuffer:
		sequence := player.guard.EventSequence + 1
		if player.guard.AcceptEvent(
			event.token.Playback, event.token.Seek, sequence, protocol.StreamEventRebuffer,
		) != protocol.StreamGenerationApply {
			player.mu.Unlock()
			return
		}
		player.state = WindowsStreamRebuffering
		player.startedPosted.Store(false)
		player.readyWanted.Store(true)
		refilled := player.ring.FillMS(windowsStreamSampleRateHz, windowsStreamChannels) >= protocol.StreamMinimumBufferedMS &&
			player.readyWanted.CompareAndSwap(true, false)
		payload := protocol.StreamRebufferPayload{
			StreamID: player.load.StreamID, PlaybackGeneration: event.token.Playback,
			SeekGeneration: event.token.Seek, EventSequence: sequence,
			AudiblePositionMS:  player.audiblePosition(),
			BufferedDurationMS: player.ring.FillMS(windowsStreamSampleRateHz, windowsStreamChannels),
		}
		player.mu.Unlock()
		player.send(protocol.TypeStreamRebuffer, &payload)
		if refilled {
			player.postInternal(windowsStreamInternalEvent{kind: windowsStreamInternalReady, token: event.token})
		}
	case windowsStreamInternalDrained:
		sequence := player.guard.EventSequence + 1
		if player.guard.AcceptEvent(
			event.token.Playback, event.token.Seek, sequence, protocol.StreamEventEnded,
		) != protocol.StreamGenerationApply {
			player.mu.Unlock()
			return
		}
		player.armed.Store(false)
		player.state = WindowsStreamTerminal
		manifest := player.manifest
		payload := protocol.StreamEndedPayload{
			StreamID: player.load.StreamID, PlaybackGeneration: event.token.Playback,
			SeekGeneration: event.token.Seek, EventSequence: sequence,
			AudiblePositionMS:  min(player.audiblePosition(), player.manifest.DurationMS),
			TLastSampleCoordMS: player.coordinatorNowMS(), Reason: "eof_drained",
		}
		player.mu.Unlock()
		_ = player.cache.SetPinned(manifest, nil)
		player.send(protocol.TypeStreamEnded, &payload)
	case windowsStreamInternalFailed:
		player.failLocked(event.token, event.err)
	default:
		player.mu.Unlock()
	}
}

func (player *WindowsStreamCandidatePlayer) failLocked(token windowsStreamGenerationToken, err error) {
	stage, code := windowsStreamFailureCode(err)
	if errors.Is(err, context.Canceled) {
		player.mu.Unlock()
		return
	}
	sequence := player.guard.EventSequence + 1
	if player.guard.AcceptEvent(
		token.Playback, token.Seek, sequence, protocol.StreamEventFailed,
	) != protocol.StreamGenerationApply {
		player.mu.Unlock()
		return
	}
	player.cancelRuntimeLocked()
	player.armed.Store(false)
	player.state = WindowsStreamTerminal
	manifest := player.manifest
	payload := protocol.StreamFailedPayload{
		StreamID: player.load.StreamID, PlaybackGeneration: token.Playback,
		SeekGeneration: token.Seek, EventSequence: sequence, Stage: stage, Code: code,
	}
	player.mu.Unlock()
	_ = player.cache.SetPinned(manifest, nil)
	player.send(protocol.TypeStreamFailed, &payload)
}

func (player *WindowsStreamCandidatePlayer) startDecoderLocked(token windowsStreamGenerationToken, positionMS int64) {
	ctx, cancel := context.WithCancel(context.Background())
	player.decoderCancel = cancel
	chunks := player.chunks
	if chunks == nil {
		chunks = &windowsStreamCacheReader{cache: player.cache, manifest: player.manifest}
	}
	request := WindowsStreamDecodeRequest{
		Manifest: player.manifest, StartPositionMS: positionMS,
		PlaybackGeneration: token.Playback, SeekGeneration: token.Seek,
		Chunks: chunks,
		PCM:    windowsStreamPCMWriter{player: player, token: token},
	}
	go func() {
		player.decoderMu.Lock()
		defer player.decoderMu.Unlock()
		if ctx.Err() != nil || player.epoch.Load() != token.Epoch {
			return
		}
		err := player.decoder.Decode(ctx, request)
		if err != nil && !errors.Is(err, io.EOF) {
			player.postInternal(windowsStreamInternalEvent{kind: windowsStreamInternalFailed, token: token, err: err})
			return
		}
		if ctx.Err() != nil || player.epoch.Load() != token.Epoch {
			return
		}
		var verifyErr error
		if verifier, ok := chunks.(windowsStreamChunkVerifier); ok {
			verifyErr = verifier.VerifyWhole()
		} else {
			verifyErr = player.cache.VerifyWhole(request.Manifest)
		}
		if verifyErr != nil {
			player.postInternal(windowsStreamInternalEvent{kind: windowsStreamInternalFailed, token: token, err: verifyErr})
			return
		}
		player.decoderEOF.Store(true)
	}()
}

func (player *WindowsStreamCandidatePlayer) resetGenerationLocked(
	playbackGeneration, seekGeneration, positionMS int64,
) windowsStreamGenerationToken {
	epoch := player.epoch.Add(1)
	player.playbackGeneration.Store(playbackGeneration)
	player.seekGeneration.Store(seekGeneration)
	player.audibleAnchorMS.Store(positionMS)
	player.renderedFrames.Store(0)
	player.armed.Store(false)
	player.readyWanted.Store(true)
	player.startedPosted.Store(false)
	player.rebufferPosted.Store(false)
	player.endedPosted.Store(false)
	player.decoderEOF.Store(false)
	player.ring.Clear()
	return windowsStreamGenerationToken{Playback: playbackGeneration, Seek: seekGeneration, Epoch: epoch}
}

func (player *WindowsStreamCandidatePlayer) armReadyDeadlineLocked(token windowsStreamGenerationToken, deadlineCoordMS int64) {
	local, ok := player.clock.LocalDeadline(deadlineCoordMS, 0)
	if !ok {
		player.postInternal(windowsStreamInternalEvent{
			kind: windowsStreamInternalFailed, token: token,
			err: windowsStreamFailure("scheduler", "clock_unsynchronized"),
		})
		return
	}
	player.readyTimer = time.AfterFunc(time.Duration(max(local-nowMS(), 0))*time.Millisecond, func() {
		if player.epoch.Load() == token.Epoch && player.readyWanted.CompareAndSwap(true, false) {
			player.postInternal(windowsStreamInternalEvent{
				kind: windowsStreamInternalFailed, token: token,
				err: windowsStreamFailure("buffer", "ready_timeout"),
			})
		}
	})
}

func (player *WindowsStreamCandidatePlayer) cancelRuntimeLocked() {
	if player.decoderCancel != nil {
		player.decoderCancel()
		player.decoderCancel = nil
	}
	if player.readyTimer != nil {
		player.readyTimer.Stop()
		player.readyTimer = nil
	}
	if player.startTimer != nil {
		player.startTimer.Stop()
		player.startTimer = nil
	}
	if player.startExpiry != nil {
		player.startExpiry.Stop()
		player.startExpiry = nil
	}
	player.armed.Store(false)
}

func (player *WindowsStreamCandidatePlayer) tokenLocked() windowsStreamGenerationToken {
	return windowsStreamGenerationToken{
		Playback: player.guard.PlaybackGeneration, Seek: player.guard.SeekGeneration,
		Epoch: player.epoch.Load(),
	}
}

func (player *WindowsStreamCandidatePlayer) audiblePosition() int64 {
	frames := player.renderedFrames.Load()
	return player.audibleAnchorMS.Load() + frames*1000/windowsStreamSampleRateHz
}

func (player *WindowsStreamCandidatePlayer) coordinatorNowMS() int64 {
	value := nowMS()
	if offset, ok := player.clock.OffsetMS(); ok {
		value -= int64(math.Round(offset))
	}
	return value
}

func (player *WindowsStreamCandidatePlayer) postWorkerEvent(ctx context.Context, event windowsStreamInternalEvent) error {
	select {
	case player.events <- event:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-player.done:
		return context.Canceled
	}
}

func (player *WindowsStreamCandidatePlayer) postInternal(event windowsStreamInternalEvent) {
	select {
	case player.events <- event:
	case <-player.done:
	}
}

func (player *WindowsStreamCandidatePlayer) postRenderEvent(kind windowsStreamInternalKind, epoch int64) {
	event := windowsStreamInternalEvent{kind: kind, token: windowsStreamGenerationToken{
		Playback: player.playbackGeneration.Load(), Seek: player.seekGeneration.Load(), Epoch: epoch,
	}}
	select {
	case player.events <- event:
	default:
		// Fixed non-blocking control mailbox: realtime output wins. The next
		// pull retries rebuffer/drain through their CAS flags after control reset.
		switch kind {
		case windowsStreamInternalStarted:
			player.startedPosted.Store(false)
		case windowsStreamInternalRebuffer:
			player.rebufferPosted.Store(false)
		case windowsStreamInternalDrained:
			player.endedPosted.Store(false)
		}
	}
}
