// Candidate-neutral macOS streamed-track lifecycle and render seam.
//
// No concrete decoder implements MacStreamCandidateDecoder in production and
// NodeApp does not compose this player. The seam exists so bounded cache,
// scheduling, generation fencing and realtime behavior can be proved with a
// deterministic injected decoder while the codec/player ADR remains no-go.

import Foundation

public let macStreamSampleRateHz = 48_000
public let macStreamChannels = 2
public let macStreamPCMRingBytes = 1 << 20

public protocol MacStreamChunkReading: AnyObject, Sendable {
    var manifest: MacStreamManifest { get }
    func chunkIndex(forTimeMs positionMs: Int64) -> Int
    func readChunk(index: Int) async throws -> Data
}

public protocol MacStreamPCMWriting: AnyObject, Sendable {
    func writePCM(_ samples: [Float]) async throws
}

public struct MacStreamDecodeRequest: Sendable {
    public let manifest: MacStreamManifest
    public let startPositionMs: Int64
    public let playbackGeneration: Int64
    public let seekGeneration: Int64
    public let chunks: MacStreamChunkReading
    public let pcm: MacStreamPCMWriting
}

/// Deliberately injected candidate seam. There is no production conformance.
public protocol MacStreamCandidateDecoder: AnyObject, Sendable {
    func decode(_ request: MacStreamDecodeRequest) async throws
}

public protocol MacStreamDeadlineClock: AnyObject, Sendable {
    func localDeadline(coordinatorMs: Int64) -> Int64?
    func coordinatorNowMs() -> Int64
    func localNowMs() -> Int64
}

public final class MacStreamSystemClock: MacStreamDeadlineClock, @unchecked Sendable {
    private let offsetMs: Int64

    public init(offsetMs: Int64) { self.offsetMs = offsetMs }

    public func localDeadline(coordinatorMs: Int64) -> Int64? { coordinatorMs + offsetMs }
    public func coordinatorNowMs() -> Int64 { localNowMs() - offsetMs }
    public func localNowMs() -> Int64 {
        Int64((Date().timeIntervalSince1970 * 1_000).rounded())
    }
}

public enum MacStreamPlayerState: String, Sendable {
    case idle, loading, ready, playing, paused, rebuffering, terminal
}

public struct MacStreamPlayerSnapshot: Sendable {
    public var state: MacStreamPlayerState
    public var streamId: String
    public var playbackGeneration: Int64
    public var seekGeneration: Int64
    public var audiblePositionMs: Int64
    public var bufferedMs: Int64
    public var ringBytes: Int
    public var ringCeilingBytes: Int
    public var volume: Int
}

private struct MacStreamGenerationToken: Equatable, Sendable {
    var playback: Int64
    var seek: Int64
    var epoch: Int64
}

private final class MacStreamCacheReader: MacStreamChunkReading, @unchecked Sendable {
    let cache: MacStreamChunkCache
    public let manifest: MacStreamManifest

    init(cache: MacStreamChunkCache, manifest: MacStreamManifest) {
        self.cache = cache
        self.manifest = manifest
    }

    func chunkIndex(forTimeMs positionMs: Int64) -> Int {
        manifest.chunkIndex(forTimeMs: positionMs)
    }

    func readChunk(index: Int) async throws -> Data {
        let data = try await cache.chunk(manifest, index: index)
        var pins = [index]
        if index + 1 < manifest.chunks.count { pins.append(index + 1) }
        try await cache.setPinned(manifest, indexes: pins)
        return data
    }
}

private final class MacStreamPCMWriter: MacStreamPCMWriting, @unchecked Sendable {
    weak var player: MacStreamCandidatePlayer?
    let token: MacStreamGenerationToken

    init(player: MacStreamCandidatePlayer, token: MacStreamGenerationToken) {
        self.player = player
        self.token = token
    }

    func writePCM(_ samples: [Float]) async throws {
        guard !samples.isEmpty, samples.count % macStreamChannels == 0 else {
            throw MacStreamFailure.frozen(stage: "decoder", code: "invalid_pcm")
        }
        var offset = 0
        while offset < samples.count {
            try Task.checkCancellation()
            guard let player, player.currentEpoch == token.epoch else {
                throw CancellationError()
            }
            let written = samples.withUnsafeBufferPointer { buffer in
                player.writeCandidatePCM(buffer.baseAddress! + offset, count: samples.count - offset)
            }
            guard player.currentEpoch == token.epoch else {
                player.discardBufferedPCM()
                throw CancellationError()
            }
            offset += written
            player.publishReadyIfNeeded(token)
            if written == 0 {
                try await Task.sleep(nanoseconds: 200_000)
            }
        }
    }
}

private actor MacStreamDecoderGate {
    func run(decoder: MacStreamCandidateDecoder, request: MacStreamDecodeRequest) async throws {
        try await decoder.decode(request)
    }
}

/// Bounded candidate player. `readPCM` is the future render boundary: it only
/// touches the SPSC ring, fixed atomics and caller-owned memory.
public final class MacStreamCandidatePlayer: @unchecked Sendable {
    private let cache: MacStreamChunkCache
    private let decoder: MacStreamCandidateDecoder
    private let decoderGate = MacStreamDecoderGate()
    private let clock: MacStreamDeadlineClock
    private let send: @Sendable (Message) -> Void
    private let injectedChunks: MacStreamChunkReading?
    private var protectedLifetimeOwner: AnyObject?
    private let ring = RingBuffer(capacityFloats: macStreamPCMRingBytes / MemoryLayout<Float>.size)
    private let queue = DispatchQueue(label: "live.barycenter.mac-stream-player")

    private var generationGuard = StreamGenerationGuard()
    private var state: MacStreamPlayerState = .idle
    private var loadPayload: StreamLoadPayload?
    private var manifest: MacStreamManifest?
    private var decoderTask: Task<Void, Never>?
    private var readyTimer: DispatchSourceTimer?
    private var startTimer: DispatchSourceTimer?
    private var startExpiryTimer: DispatchSourceTimer?
    private var signalTimer: DispatchSourceTimer?

    private let epoch = RenderAtomicInt64()
    private let playbackGeneration = RenderAtomicInt64()
    private let seekGeneration = RenderAtomicInt64()
    private let audibleAnchorMs = RenderAtomicInt64()
    private let renderedFrames = RenderAtomicInt64()
    private let armed = RenderAtomicInt64()
    private let readyWanted = RenderAtomicInt64()
    private let startedPosted = RenderAtomicInt64()
    private let rebufferPosted = RenderAtomicInt64()
    private let endedPosted = RenderAtomicInt64()
    private let decoderEOF = RenderAtomicInt64()
    private let volume = RenderAtomicInt64(100)
    private let pendingStarted = RenderAtomicInt64()
    private let pendingRebuffer = RenderAtomicInt64()
    private let pendingDrained = RenderAtomicInt64()

    public init(
        cache: MacStreamChunkCache, decoder: MacStreamCandidateDecoder,
        clock: MacStreamDeadlineClock, protectedChunks: MacStreamChunkReading? = nil,
        send: @escaping @Sendable (Message) -> Void
    ) {
        self.cache = cache
        self.decoder = decoder
        self.clock = clock
        self.injectedChunks = protectedChunks
        self.protectedLifetimeOwner = nil
        self.send = send
        let timer = DispatchSource.makeTimerSource(queue: queue)
        timer.schedule(deadline: .now(), repeating: .milliseconds(2))
        timer.setEventHandler { [weak self] in self?.drainRenderSignals() }
        timer.resume()
        signalTimer = timer
    }

    deinit {
        decoderTask?.cancel()
        readyTimer?.cancel()
        startTimer?.cancel()
        startExpiryTimer?.cancel()
        signalTimer?.cancel()
    }

    func retainProtectedLifetime(_ owner: AnyObject) {
        protectedLifetimeOwner = owner
    }

    public func load(_ payload: StreamLoadPayload, manifest: MacStreamManifest) throws {
        try manifest.validate(load: payload)
        if let injectedChunks, injectedChunks.manifest != manifest {
            throw MacStreamFailure.frozen(stage: "manifest", code: "invalid_manifest")
        }
        try queue.sync {
            let decision = generationGuard.acceptLoad(
                playback: payload.playbackGeneration, seek: payload.seekGeneration,
                command: payload.commandSequence)
            if decision == .duplicate || decision == .stale { return }
            guard decision == .apply else {
                throw MacStreamFailure.frozen(stage: "generation", code: "invalid_command")
            }
            cancelRuntime()
            loadPayload = payload
            self.manifest = manifest
            state = .loading
            let token = resetGeneration(
                playback: payload.playbackGeneration, seek: 0,
                positionMs: payload.startPositionMs)
            armReadyDeadline(token, coordinatorMs: payload.readyDeadlineCoordMs)
            startDecoder(token, positionMs: payload.startPositionMs)
        }
    }

    public func resume(at payload: StreamResumeAtPayload) throws {
        guard !payload.streamId.isEmpty, payload.playbackGeneration > 0,
              payload.seekGeneration >= 0, payload.commandSequence > 0,
              payload.tCoordMs > 0, payload.startDeadlineCoordMs >= payload.tCoordMs else {
            throw MacStreamFailure.frozen(stage: "scheduler", code: "invalid_command")
        }
        guard let localStart = clock.localDeadline(coordinatorMs: payload.tCoordMs),
              let localDeadline = clock.localDeadline(
                coordinatorMs: payload.startDeadlineCoordMs) else {
            throw MacStreamFailure.frozen(stage: "scheduler", code: "clock_unsynchronized")
        }
        guard localStart <= localDeadline, clock.localNowMs() <= localDeadline else {
            throw MacStreamFailure.frozen(stage: "scheduler", code: "start_timeout")
        }
        try queue.sync {
            guard loadPayload?.streamId == payload.streamId else { return }
            let decision = generationGuard.acceptCommand(
                playback: payload.playbackGeneration, seek: payload.seekGeneration,
                command: payload.commandSequence, kind: "resume")
            if decision == .duplicate || decision == .stale { return }
            guard decision == .apply else {
                throw MacStreamFailure.frozen(stage: "generation", code: "invalid_command")
            }
            startTimer?.cancel()
            startExpiryTimer?.cancel()
            let token = currentToken()
            startTimer = makeTimer(deadlineMs: localStart) { [weak self] in
                guard let self, self.currentEpoch == token.epoch else { return }
                self.armed.store(1)
            }
            startExpiryTimer = makeTimer(deadlineMs: localDeadline) { [weak self] in
                guard let self, self.currentEpoch == token.epoch,
                      self.startedPosted.load() == 0 else { return }
                self.fail(token, error: MacStreamFailure.frozen(
                    stage: "scheduler", code: "start_timeout"))
            }
        }
    }

    public func pause(_ payload: StreamPausePayload) throws {
        guard !payload.streamId.isEmpty, payload.fadeMs >= 0, payload.fadeMs <= 1_000 else {
            throw MacStreamFailure.frozen(stage: "player", code: "invalid_command")
        }
        try queue.sync {
            guard loadPayload?.streamId == payload.streamId else { return }
            let decision = generationGuard.acceptCommand(
                playback: payload.playbackGeneration, seek: payload.seekGeneration,
                command: payload.commandSequence, kind: "pause")
            if decision == .duplicate || decision == .stale { return }
            guard decision == .apply else {
                throw MacStreamFailure.frozen(stage: "generation", code: "invalid_command")
            }
            startTimer?.cancel()
            armed.store(0)
            startedPosted.store(0)
            state = .paused
        }
    }

    public func seek(_ payload: StreamSeekPayload) throws {
        guard !payload.streamId.isEmpty, payload.playbackGeneration > 0,
              payload.seekGeneration > 0, payload.commandSequence > 0,
              payload.positionMs >= 0,
              payload.minimumBufferedMs == ProtocolConstants.streamMinimumBufferedMs,
              payload.readyDeadlineCoordMs > 0 else {
            throw MacStreamFailure.frozen(stage: "player", code: "invalid_command")
        }
        try queue.sync {
            guard loadPayload?.streamId == payload.streamId,
                  let manifest, payload.positionMs <= manifest.durationMs else { return }
            let decision = generationGuard.acceptSeek(
                playback: payload.playbackGeneration, seek: payload.seekGeneration,
                command: payload.commandSequence)
            if decision == .duplicate || decision == .stale { return }
            guard decision == .apply else {
                throw MacStreamFailure.frozen(stage: "generation", code: "invalid_command")
            }
            cancelRuntime()
            state = .loading
            let token = resetGeneration(
                playback: payload.playbackGeneration, seek: payload.seekGeneration,
                positionMs: payload.positionMs)
            armReadyDeadline(token, coordinatorMs: payload.readyDeadlineCoordMs)
            startDecoder(token, positionMs: payload.positionMs)
        }
    }

    public func cancel(_ payload: StreamCancelPayload) throws {
        guard !payload.streamId.isEmpty, !payload.reason.isEmpty else {
            throw MacStreamFailure.frozen(stage: "player", code: "invalid_command")
        }
        try queue.sync {
            guard loadPayload?.streamId == payload.streamId else { return }
            let decision = generationGuard.acceptCommand(
                playback: payload.playbackGeneration, seek: payload.seekGeneration,
                command: payload.commandSequence, kind: "cancel")
            if decision == .duplicate || decision == .stale { return }
            guard decision == .apply else {
                throw MacStreamFailure.frozen(stage: "generation", code: "invalid_command")
            }
            cancelRuntime()
            _ = epoch.add(1)
            ring.clear()
            let sequence = generationGuard.eventSequence + 1
            guard generationGuard.acceptEvent(
                playback: payload.playbackGeneration, seek: payload.seekGeneration,
                event: sequence, kind: .cancelled) == .apply else {
                throw MacStreamFailure.frozen(stage: "generation", code: "invalid_event")
            }
            state = .terminal
            let event = StreamCancelledPayload(
                streamId: payload.streamId, playbackGeneration: payload.playbackGeneration,
                seekGeneration: payload.seekGeneration, eventSequence: sequence,
                audiblePositionMs: audiblePositionMs(), reason: payload.reason)
            send(.streamCancelled(event))
            if let manifest { Task { try? await cache.setPinned(manifest, indexes: []) } }
        }
    }

    /// Delete/disable creates a durable no-refill tombstone and purges PCM.
    public func revoke() async throws {
        let revokedManifest: MacStreamManifest? = queue.sync {
            guard let manifest else { return nil }
            cancelRuntime()
            _ = epoch.add(1)
            ring.clear()
            state = .terminal
            return manifest
        }
        if let revokedManifest { try await cache.tombstone(revokedManifest) }
    }

    public func setLocalVolume(_ value: Int) {
        volume.store(Int64(min(max(value, 0), 100)))
    }

    public func snapshot() -> MacStreamPlayerSnapshot {
        queue.sync {
            MacStreamPlayerSnapshot(
                state: state, streamId: loadPayload?.streamId ?? "",
                playbackGeneration: generationGuard.playbackGeneration,
                seekGeneration: generationGuard.seekGeneration,
                audiblePositionMs: audiblePositionMs(), bufferedMs: bufferedMs,
                ringBytes: ring.fill * MemoryLayout<Float>.size,
                ringCeilingBytes: macStreamPCMRingBytes, volume: Int(volume.load()))
        }
    }

    /// Control-side one-second progress hook. A future production composition
    /// may call it from its scheduler; it never runs from the render callback.
    public func publishProgress() {
        queue.async {
            guard let loadPayload = self.loadPayload,
                  self.generationGuard.phase == "started" else { return }
            let sequence = self.generationGuard.eventSequence + 1
            guard self.generationGuard.acceptEvent(
                playback: self.generationGuard.playbackGeneration,
                seek: self.generationGuard.seekGeneration,
                event: sequence, kind: .progress) == .apply else { return }
            self.send(.streamProgress(StreamProgressPayload(
                streamId: loadPayload.streamId,
                playbackGeneration: self.generationGuard.playbackGeneration,
                seekGeneration: self.generationGuard.seekGeneration,
                eventSequence: sequence, audiblePositionMs: self.audiblePositionMs(),
                bufferedDurationMs: self.bufferedMs)))
        }
    }

    /// Render-safe: no queue, lock, allocation, filesystem, network or decode.
    @inline(__always)
    public func readPCM(into output: UnsafeMutablePointer<Float>, count: Int) -> Int {
        guard count > 0, count % macStreamChannels == 0 else { return 0 }
        guard armed.load() != 0 else {
            // The render consumer owns tail advancement. Apply a pending
            // generation cut even while output is disarmed so the replacement
            // decoder can reuse bounded ring capacity without a control-thread
            // tail write racing this callback.
            _ = ring.read(into: output, count: 0)
            output.initialize(repeating: 0, count: count)
            return 0
        }
        let observedEpoch = epoch.load()
        let read = ring.read(into: output, count: count)
        guard epoch.load() == observedEpoch else {
            output.initialize(repeating: 0, count: count)
            return 0
        }
        if read < count { (output + read).initialize(repeating: 0, count: count - read) }
        let localVolume = volume.load()
        let gain = Float(localVolume * localVolume) / 10_000
        for index in 0..<read { output[index] = min(max(output[index] * gain, -1), 1) }
        if read > 0 {
            _ = renderedFrames.add(Int64(read / macStreamChannels))
            var expected: Int64 = 0
            if startedPosted.compareExchange(expected: &expected, desired: 1) {
                pendingStarted.store(observedEpoch)
            }
        }
        if read < count {
            if decoderEOF.load() != 0 && ring.fill == 0 {
                var expected: Int64 = 0
                if endedPosted.compareExchange(expected: &expected, desired: 1) {
                    pendingDrained.store(observedEpoch)
                }
            } else if decoderEOF.load() == 0 && startedPosted.load() != 0 {
                var expected: Int64 = 0
                if rebufferPosted.compareExchange(expected: &expected, desired: 1) {
                    armed.store(0)
                    pendingRebuffer.store(observedEpoch)
                }
            }
        }
        return read
    }

    fileprivate var currentEpoch: Int64 { epoch.load() }
    fileprivate func writeCandidatePCM(_ samples: UnsafePointer<Float>, count: Int) -> Int {
        ring.write(samples, count: count)
    }
    fileprivate func discardBufferedPCM() { ring.clear() }

    fileprivate func publishReadyIfNeeded(_ token: MacStreamGenerationToken) {
        guard readyWanted.load() != 0, bufferedMs >= ProtocolConstants.streamMinimumBufferedMs else { return }
        var expected: Int64 = 1
        guard readyWanted.compareExchange(expected: &expected, desired: 0) else { return }
        queue.async { [weak self] in self?.publishReady(token) }
    }

    private var bufferedMs: Int64 {
        Int64(ring.fill / macStreamChannels) * 1_000 / Int64(macStreamSampleRateHz)
    }

    private func audiblePositionMs() -> Int64 {
        audibleAnchorMs.load() + renderedFrames.load() * 1_000 / Int64(macStreamSampleRateHz)
    }

    private func resetGeneration(
        playback: Int64, seek: Int64, positionMs: Int64
    ) -> MacStreamGenerationToken {
        let newEpoch = epoch.add(1)
        playbackGeneration.store(playback)
        seekGeneration.store(seek)
        audibleAnchorMs.store(positionMs)
        renderedFrames.store(0)
        armed.store(0)
        readyWanted.store(1)
        startedPosted.store(0)
        rebufferPosted.store(0)
        endedPosted.store(0)
        decoderEOF.store(0)
        pendingStarted.store(0)
        pendingRebuffer.store(0)
        pendingDrained.store(0)
        ring.clear()
        return MacStreamGenerationToken(playback: playback, seek: seek, epoch: newEpoch)
    }

    private func currentToken() -> MacStreamGenerationToken {
        .init(playback: playbackGeneration.load(), seek: seekGeneration.load(), epoch: epoch.load())
    }

    private func startDecoder(_ token: MacStreamGenerationToken, positionMs: Int64) {
        guard let manifest else { return }
        let reader = injectedChunks ?? MacStreamCacheReader(cache: cache, manifest: manifest)
        let writer = MacStreamPCMWriter(player: self, token: token)
        let request = MacStreamDecodeRequest(
            manifest: manifest, startPositionMs: positionMs,
            playbackGeneration: token.playback, seekGeneration: token.seek,
            chunks: reader, pcm: writer)
        decoderTask = Task { [weak self, decoder, decoderGate] in
            do {
                try await decoderGate.run(decoder: decoder, request: request)
                guard let self, self.currentEpoch == token.epoch else { return }
                self.decoderEOF.store(1)
            } catch is CancellationError {
                return
            } catch {
                self?.queue.async { [weak self] in self?.fail(token, error: error) }
            }
        }
    }

    private func publishReady(_ token: MacStreamGenerationToken) {
        guard token == currentToken(), let loadPayload, state != .terminal else { return }
        let sequence = generationGuard.eventSequence + 1
        guard generationGuard.acceptReady(
            playback: token.playback, seek: token.seek, event: sequence,
            buffered: bufferedMs, minimum: ProtocolConstants.streamMinimumBufferedMs) == .apply else { return }
        readyTimer?.cancel()
        state = generationGuard.phase == "paused_ready" ? .paused : .ready
        rebufferPosted.store(0)
        send(.streamReady(StreamReadyPayload(
            streamId: loadPayload.streamId, playbackGeneration: token.playback,
            seekGeneration: token.seek, eventSequence: sequence,
            audiblePositionMs: audiblePositionMs(), bufferedDurationMs: bufferedMs)))
    }

    private func drainRenderSignals() {
        let token = currentToken()
        if pendingStarted.load() == token.epoch {
            pendingStarted.store(0)
            publishStarted(token)
        }
        if pendingRebuffer.load() == token.epoch {
            pendingRebuffer.store(0)
            publishRebuffer(token)
        }
        if pendingDrained.load() == token.epoch {
            pendingDrained.store(0)
            publishDrained(token)
        }
    }

    private func publishStarted(_ token: MacStreamGenerationToken) {
        guard token == currentToken(), let loadPayload, state != .terminal else { return }
        let sequence = generationGuard.eventSequence + 1
        guard generationGuard.acceptEvent(
            playback: token.playback, seek: token.seek, event: sequence, kind: .started) == .apply else { return }
        state = .playing
        startExpiryTimer?.cancel()
        send(.streamStarted(StreamStartedPayload(
            streamId: loadPayload.streamId, playbackGeneration: token.playback,
            seekGeneration: token.seek, eventSequence: sequence,
            audiblePositionMs: audiblePositionMs(),
            tFirstSampleCoordMs: clock.coordinatorNowMs())))
    }

    private func publishRebuffer(_ token: MacStreamGenerationToken) {
        guard token == currentToken(), let loadPayload, state != .terminal else { return }
        let sequence = generationGuard.eventSequence + 1
        guard generationGuard.acceptEvent(
            playback: token.playback, seek: token.seek, event: sequence, kind: .rebuffer) == .apply else { return }
        state = .rebuffering
        startedPosted.store(0)
        readyWanted.store(1)
        send(.streamRebuffer(StreamRebufferPayload(
            streamId: loadPayload.streamId, playbackGeneration: token.playback,
            seekGeneration: token.seek, eventSequence: sequence,
            audiblePositionMs: audiblePositionMs(), bufferedDurationMs: bufferedMs)))
        publishReadyIfNeeded(token)
    }

    private func publishDrained(_ token: MacStreamGenerationToken) {
        guard token == currentToken(), let loadPayload, let manifest, state != .terminal else { return }
        let sequence = generationGuard.eventSequence + 1
        guard generationGuard.acceptEvent(
            playback: token.playback, seek: token.seek, event: sequence, kind: .ended) == .apply else { return }
        armed.store(0)
        state = .terminal
        send(.streamEnded(StreamEndedPayload(
            streamId: loadPayload.streamId, playbackGeneration: token.playback,
            seekGeneration: token.seek, eventSequence: sequence,
            audiblePositionMs: min(audiblePositionMs(), manifest.durationMs),
            tLastSampleCoordMs: clock.coordinatorNowMs(), reason: "eof_drained")))
        Task { try? await cache.setPinned(manifest, indexes: []) }
    }

    private func fail(_ token: MacStreamGenerationToken, error: Error) {
        guard token == currentToken(), let loadPayload, let manifest, state != .terminal else { return }
        let failure = MacStreamFailure.sanitized(error)
        let sequence = generationGuard.eventSequence + 1
        guard generationGuard.acceptEvent(
            playback: token.playback, seek: token.seek, event: sequence, kind: .failed) == .apply else { return }
        cancelRuntime()
        armed.store(0)
        state = .terminal
        send(.streamFailed(StreamFailedPayload(
            streamId: loadPayload.streamId, playbackGeneration: token.playback,
            seekGeneration: token.seek, eventSequence: sequence,
            stage: failure.stage, code: failure.code)))
        Task { try? await cache.setPinned(manifest, indexes: []) }
    }

    private func armReadyDeadline(_ token: MacStreamGenerationToken, coordinatorMs: Int64) {
        guard let deadline = clock.localDeadline(coordinatorMs: coordinatorMs) else {
            fail(token, error: MacStreamFailure.frozen(
                stage: "scheduler", code: "clock_unsynchronized"))
            return
        }
        readyTimer = makeTimer(deadlineMs: deadline) { [weak self] in
            guard let self, self.currentEpoch == token.epoch, self.readyWanted.load() != 0 else { return }
            self.readyWanted.store(0)
            self.fail(token, error: MacStreamFailure.frozen(
                stage: "buffer", code: "ready_timeout"))
        }
    }

    private func makeTimer(deadlineMs: Int64, handler: @escaping @Sendable () -> Void) -> DispatchSourceTimer {
        let timer = DispatchSource.makeTimerSource(queue: queue)
        let delay = max(0, deadlineMs - clock.localNowMs())
        timer.schedule(deadline: .now() + .milliseconds(Int(delay)))
        timer.setEventHandler(handler: handler)
        timer.resume()
        return timer
    }

    private func cancelRuntime() {
        decoderTask?.cancel()
        decoderTask = nil
        readyTimer?.cancel()
        readyTimer = nil
        startTimer?.cancel()
        startTimer = nil
        startExpiryTimer?.cancel()
        startExpiryTimer = nil
    }
}
