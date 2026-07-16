import CryptoKit
import Foundation
import Testing
@testable import NodeCore

private func streamHash(_ data: Data) -> String {
    SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined()
}

private func streamManifest(_ payloads: [Data], identity: String = "svm1.mac-test") -> MacStreamManifest {
    var offset: Int64 = 0
    let chunks = payloads.enumerated().map { index, data -> MacStreamChunk in
        defer { offset += Int64(data.count) }
        return MacStreamChunk(
            index: index, start: offset, end: offset + Int64(data.count) - 1,
            sha256: streamHash(data))
    }
    let whole = payloads.reduce(into: Data()) { $0.append($1) }
    return MacStreamManifest(
        identity: identity, variantUrl: "/v1/media/media_mac/variants/candidate",
        etag: "\"sha256-\(streamHash(whole))\"", sha256: streamHash(whole),
        sizeBytes: Int64(whole.count), durationMs: 120_000, chunks: chunks,
        seekMap: [MacStreamSeekPoint(timeMs: 0, offset: 0)])
}

private final class StreamRangeFixture: MacStreamRangeFetching, @unchecked Sendable {
    struct Call: Equatable { var path: String; var etag: String; var start: Int64; var end: Int64 }
    private let lock = NSLock()
    private var bodies: [ClosedRange<Int64>: Data]
    private(set) var callCount = 0
    private var storedCalls: [Call] = []
    var failNextNetwork = false

    init(_ payloads: [Data]) {
        var offset: Int64 = 0
        var bodies: [ClosedRange<Int64>: Data] = [:]
        for payload in payloads {
            bodies[offset...(offset + Int64(payload.count) - 1)] = payload
            offset += Int64(payload.count)
        }
        self.bodies = bodies
    }

    func fetchRange(
        path: String, etag: String, start: Int64, end: Int64
    ) async throws -> (data: Data, etag: String) {
        let (shouldFail, body) = lock.withLock {
            callCount += 1
            storedCalls.append(.init(path: path, etag: etag, start: start, end: end))
            let shouldFail = failNextNetwork
            failNextNetwork = false
            return (shouldFail, bodies[start...end])
        }
        if shouldFail {
            throw MacStreamFailure.frozen(stage: "fetch", code: "network_failed")
        }
        guard let body else {
            throw MacStreamFailure.frozen(stage: "fetch", code: "invalid_range")
        }
        return (body, etag)
    }

    var calls: [Call] { lock.withLock { storedCalls } }
}

private final class StreamClockFixture: MacStreamDeadlineClock, @unchecked Sendable {
    var now: Int64 = 1_000
    var synchronized = true
    func localDeadline(coordinatorMs: Int64) -> Int64? { synchronized ? coordinatorMs : nil }
    func coordinatorNowMs() -> Int64 { now }
    func localNowMs() -> Int64 { now }
}

private final class StreamEventFixture: @unchecked Sendable {
    private let lock = NSLock()
    private var events: [Message] = []
    func append(_ event: Message) { lock.withLock { events.append(event) } }
    var hasReady: Bool { lock.withLock { events.contains { if case .streamReady = $0 { true } else { false } } } }
    var hasStarted: Bool { lock.withLock { events.contains { if case .streamStarted = $0 { true } else { false } } } }
    var hasProgress: Bool {
        lock.withLock { events.contains { if case .streamProgress = $0 { true } else { false } } }
    }
    var hasEnded: Bool { lock.withLock { events.contains { if case .streamEnded = $0 { true } else { false } } } }
    var hasRebuffer: Bool {
        lock.withLock { events.contains { if case .streamRebuffer = $0 { true } else { false } } }
    }
    var readyCount: Int {
        lock.withLock { events.reduce(0) { count, event in
            if case .streamReady = event { count + 1 } else { count }
        } }
    }
    var readySeekGenerations: [Int64] {
        lock.withLock { events.compactMap { if case .streamReady(let value) = $0 { value.seekGeneration } else { nil } } }
    }
}

private final class StreamDecoderFixture: MacStreamCandidateDecoder, @unchecked Sendable {
    func decode(_ request: MacStreamDecodeRequest) async throws {
        _ = try await request.chunks.readChunk(
            index: request.chunks.chunkIndex(forTimeMs: request.startPositionMs))
        let value = Float(request.seekGeneration + 1) * 0.25
        let pcm = [Float](repeating: value, count: 202_000)
        try await request.pcm.writePCM(pcm)
    }
}

private final class RebufferDecoderFixture: MacStreamCandidateDecoder, @unchecked Sendable {
    private let lock = NSLock()
    private var continuation: CheckedContinuation<Void, Never>?

    func decode(_ request: MacStreamDecodeRequest) async throws {
        _ = try await request.chunks.readChunk(index: 0)
        try await request.pcm.writePCM([Float](repeating: 0.25, count: 192_480))
        await withCheckedContinuation { continuation in
            lock.withLock { self.continuation = continuation }
        }
        try await request.pcm.writePCM([Float](repeating: 0.5, count: 192_480))
    }

    func releaseRefill() {
        let continuation = lock.withLock { () -> CheckedContinuation<Void, Never>? in
            let value = self.continuation
            self.continuation = nil
            return value
        }
        continuation?.resume()
    }
}

private final class MacStreamURLProtocol: URLProtocol, @unchecked Sendable {
    typealias Handler = @Sendable (URLRequest) -> (HTTPURLResponse, Data)
    private static let lock = NSLock()
    private static var handler: Handler?
    private static var recorded: URLRequest?

    static func install(_ handler: @escaping Handler) {
        lock.withLock {
            self.handler = handler
            recorded = nil
        }
    }

    static var request: URLRequest? { lock.withLock { recorded } }

    override class func canInit(with request: URLRequest) -> Bool { true }
    override class func canonicalRequest(for request: URLRequest) -> URLRequest { request }

    override func startLoading() {
        let handler = Self.lock.withLock { () -> Handler? in
            Self.recorded = request
            return Self.handler
        }
        guard let handler else { return }
        let (response, data) = handler(request)
        client?.urlProtocol(self, didReceive: response, cacheStoragePolicy: .notAllowed)
        if !data.isEmpty { client?.urlProtocol(self, didLoad: data) }
        client?.urlProtocolDidFinishLoading(self)
    }

    override func stopLoading() {}
}

private func streamLoad(_ manifest: MacStreamManifest) -> StreamLoadPayload {
    StreamLoadPayload(
        streamId: "stream_mac", playbackGeneration: 7, seekGeneration: 0,
        commandSequence: 1, mediaId: "media_mac", variantManifest: manifest.identity,
        variantUrl: manifest.variantUrl, variantEtag: manifest.etag,
        variantSha256: manifest.sha256, variantSizeBytes: manifest.sizeBytes,
        startPositionMs: 0, minimumBufferedMs: ProtocolConstants.streamMinimumBufferedMs,
        readyDeadlineCoordMs: 10_000,
        mixedVersionPolicy: ProtocolConstants.streamMixedVersionRequireAll)
}

private func eventually(
    timeoutMs: Int = 2_000, _ predicate: @escaping @Sendable () -> Bool
) async -> Bool {
    for _ in 0..<(timeoutMs / 5) {
        if predicate() { return true }
        try? await Task.sleep(nanoseconds: 5_000_000)
    }
    return predicate()
}

@Suite(.serialized) struct MacStreamTrackPlayerTests {
    private func temporaryRoot() throws -> URL {
        let root = FileManager.default.temporaryDirectory
            .appendingPathComponent("mac-stream-tests-\(UUID().uuidString)", isDirectory: true)
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
        return root
    }

    @Test func boundedCacheRetriesHitsEvictsAndPersistsRevocation() async throws {
        let bodies = [Data(repeating: 1, count: 8), Data(repeating: 2, count: 8), Data(repeating: 3, count: 8)]
        let manifest = streamManifest(bodies)
        let fetcher = StreamRangeFixture(bodies)
        fetcher.failNextNetwork = true
        let root = try temporaryRoot()
        defer { try? FileManager.default.removeItem(at: root) }
        let limits = MacStreamCacheLimits(
            globalBytes: 16, perVariantBytes: 16, pinnedBytes: 8,
            chunkBytes: 8, networkBytes: 8)
        let secret = Data(repeating: 7, count: 32)
        let cache = try MacStreamChunkCache(
            root: root, installationSecret: secret, fetcher: fetcher, limits: limits)

        #expect(try await cache.chunk(manifest, index: 0) == bodies[0])
        #expect(fetcher.callCount == 2, "one bounded retry follows a network reset")
        #expect(try await cache.chunk(manifest, index: 0) == bodies[0])
        #expect(fetcher.callCount == 2, "verified cache hit performs no network access")
        #expect(fetcher.calls.last == .init(
            path: manifest.variantUrl, etag: manifest.etag, start: 0, end: 7))

        try await cache.setPinned(manifest, indexes: [0])
        #expect(try await cache.chunk(manifest, index: 1) == bodies[1])
        #expect(try await cache.chunk(manifest, index: 2) == bodies[2])
        let stats = await cache.stats()
        #expect(stats.bytes <= limits.globalBytes)
        #expect(stats.pinnedBytes <= limits.pinnedBytes)
        #expect(stats.evictions >= 1)

        let indexText = try String(contentsOf: root.appendingPathComponent("stream-v1/index-v1.json"))
        #expect(!indexText.contains(manifest.identity))
        #expect(!indexText.contains(manifest.variantUrl))
        try await cache.tombstone(manifest)

        let restarted = try MacStreamChunkCache(
            root: root, installationSecret: secret, fetcher: fetcher, limits: limits)
        await #expect(throws: MacStreamFailure.frozen(stage: "fetch", code: "revoked")) {
            try await restarted.chunk(manifest, index: 0)
        }
    }

    @Test func httpFetcherUsesBearerExactRangeAndStopsDeclaredOverflow() async throws {
        let hash = String(repeating: "a", count: 64)
        let etag = "\"sha256-\(hash)\""
        let configuration = URLSessionConfiguration.ephemeral
        configuration.protocolClasses = [MacStreamURLProtocol.self]
        MacStreamURLProtocol.install { request in
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 206, httpVersion: "HTTP/1.1",
                headerFields: [
                    "ETag": etag, "Content-Range": "bytes 4-7/12",
                    "Content-Length": "4"
                ])!
            return (response, Data([4, 5, 6, 7]))
        }
        let fetcher = try MacStreamHTTPRangeFetcher(
            coordinatorURL: URL(string: "wss://coord.example/socket")!,
            nodeToken: "node-secret", sessionConfiguration: configuration)
        let result = try await fetcher.fetchRange(
            path: "/v1/media/media_mac/variants/candidate", etag: etag, start: 4, end: 7)
        #expect(result.data == Data([4, 5, 6, 7]))
        let request = try #require(MacStreamURLProtocol.request)
        #expect(request.url?.absoluteString ==
            "https://coord.example/v1/media/media_mac/variants/candidate")
        #expect(request.value(forHTTPHeaderField: "Authorization") == "Bearer node-secret")
        #expect(request.value(forHTTPHeaderField: "Range") == "bytes=4-7")
        #expect(request.value(forHTTPHeaderField: "If-Range") == etag)
        #expect(!request.url!.absoluteString.contains("node-secret"))

        MacStreamURLProtocol.install { request in
            let response = HTTPURLResponse(
                url: request.url!, statusCode: 206, httpVersion: "HTTP/1.1",
                headerFields: [
                    "ETag": etag, "Content-Range": "bytes 0-0/1",
                    "Content-Length": "\((1 << 20) + 1)"
                ])!
            return (response, Data())
        }
        await #expect(throws: MacStreamFailure.frozen(
            stage: "fetch", code: "range_too_large")) {
            try await fetcher.fetchRange(
                path: "/v1/media/media_mac/variants/candidate",
                etag: etag, start: 0, end: 0)
        }
    }

    @Test func lifecycleUsesThresholdScheduleRenderDrainAndFixedCeilings() async throws {
        let bodies = [Data(repeating: 9, count: 32)]
        let manifest = streamManifest(bodies)
        let fetcher = StreamRangeFixture(bodies)
        let root = try temporaryRoot()
        defer { try? FileManager.default.removeItem(at: root) }
        let cache = try MacStreamChunkCache(
            root: root, installationSecret: Data(repeating: 4, count: 32), fetcher: fetcher)
        let clock = StreamClockFixture()
        let sink = StreamEventFixture()
        let player = MacStreamCandidatePlayer(
            cache: cache, decoder: StreamDecoderFixture(), clock: clock,
            send: { sink.append($0) })

        try player.load(streamLoad(manifest), manifest: manifest)
        #expect(await eventually { sink.hasReady })
        let ready = player.snapshot()
        #expect(ready.state == .ready)
        #expect(ready.bufferedMs >= ProtocolConstants.streamMinimumBufferedMs)
        #expect(ready.ringCeilingBytes == macStreamPCMRingBytes)
        #expect(ready.ringBytes <= macStreamPCMRingBytes)

        try player.resume(at: StreamResumeAtPayload(
            streamId: "stream_mac", playbackGeneration: 7, seekGeneration: 0,
            commandSequence: 2, tCoordMs: 1_000, startDeadlineCoordMs: 2_000))
        var output = [Float](repeating: 0, count: 480)
        for _ in 0..<50 where !sink.hasStarted {
            _ = output.withUnsafeMutableBufferPointer {
                player.readPCM(into: $0.baseAddress!, count: $0.count)
            }
            try? await Task.sleep(nanoseconds: 2_000_000)
        }
        #expect(await eventually { sink.hasStarted })
        player.publishProgress()
        #expect(await eventually { sink.hasProgress })
        for _ in 0..<520 {
            _ = output.withUnsafeMutableBufferPointer {
                player.readPCM(into: $0.baseAddress!, count: $0.count)
            }
            if sink.hasEnded { break }
            try? await Task.sleep(nanoseconds: 200_000)
        }
        #expect(await eventually { sink.hasEnded }, "ended is emitted only after decoder EOF and ring drain")
        #expect(player.snapshot().state == .terminal)
    }

    @Test func pauseSeekResumeCutsOldGenerationWithoutFullReload() async throws {
        let bodies = [Data(repeating: 6, count: 32)]
        let manifest = streamManifest(bodies)
        let fetcher = StreamRangeFixture(bodies)
        let root = try temporaryRoot()
        defer { try? FileManager.default.removeItem(at: root) }
        let cache = try MacStreamChunkCache(
            root: root, installationSecret: Data(repeating: 3, count: 32), fetcher: fetcher)
        let clock = StreamClockFixture()
        let sink = StreamEventFixture()
        let player = MacStreamCandidatePlayer(
            cache: cache, decoder: StreamDecoderFixture(), clock: clock,
            send: { sink.append($0) })
        try player.load(streamLoad(manifest), manifest: manifest)
        #expect(await eventually { sink.hasReady })
        try player.resume(at: StreamResumeAtPayload(
            streamId: "stream_mac", playbackGeneration: 7, seekGeneration: 0,
            commandSequence: 2, tCoordMs: 1_000, startDeadlineCoordMs: 2_000))
        var first = [Float](repeating: 0, count: 480)
        for _ in 0..<50 where !sink.hasStarted {
            _ = first.withUnsafeMutableBufferPointer {
                player.readPCM(into: $0.baseAddress!, count: $0.count)
            }
            try? await Task.sleep(nanoseconds: 2_000_000)
        }
        #expect(await eventually { sink.hasStarted })
        try player.pause(StreamPausePayload(
            streamId: "stream_mac", playbackGeneration: 7, seekGeneration: 0,
            commandSequence: 3, fadeMs: 100))
        try player.seek(StreamSeekPayload(
            streamId: "stream_mac", playbackGeneration: 7, seekGeneration: 1,
            commandSequence: 4, positionMs: 30_000,
            minimumBufferedMs: ProtocolConstants.streamMinimumBufferedMs,
            readyDeadlineCoordMs: 10_000))
        var cut = [Float](repeating: 1, count: 480)
        _ = cut.withUnsafeMutableBufferPointer {
            player.readPCM(into: $0.baseAddress!, count: $0.count)
        }
        #expect(cut.allSatisfy { $0 == 0 })
        #expect(await eventually { sink.readySeekGenerations.contains(1) })
        try player.resume(at: StreamResumeAtPayload(
            streamId: "stream_mac", playbackGeneration: 7, seekGeneration: 1,
            commandSequence: 5, tCoordMs: 1_000, startDeadlineCoordMs: 2_000))
        var afterSeek = [Float](repeating: 0, count: 480)
        for _ in 0..<50 where afterSeek.allSatisfy({ $0 == 0 }) {
            _ = afterSeek.withUnsafeMutableBufferPointer {
                player.readPCM(into: $0.baseAddress!, count: $0.count)
            }
            try? await Task.sleep(nanoseconds: 2_000_000)
        }
        #expect(afterSeek.allSatisfy { $0 == 0.5 }, "old generation PCM cannot cross the seek cut")
        #expect(player.snapshot().audiblePositionMs >= 30_000)
        #expect(fetcher.callCount == 1, "seek reuses verified bounded chunks instead of reloading the object")
    }

    @Test func underrunDisarmsAndRequiresRefillReadyPlusFreshResume() async throws {
        let bodies = [Data(repeating: 8, count: 32)]
        let manifest = streamManifest(bodies)
        let fetcher = StreamRangeFixture(bodies)
        let root = try temporaryRoot()
        defer { try? FileManager.default.removeItem(at: root) }
        let cache = try MacStreamChunkCache(
            root: root, installationSecret: Data(repeating: 2, count: 32), fetcher: fetcher)
        let clock = StreamClockFixture()
        let sink = StreamEventFixture()
        let decoder = RebufferDecoderFixture()
        let player = MacStreamCandidatePlayer(
            cache: cache, decoder: decoder, clock: clock, send: { sink.append($0) })
        try player.load(streamLoad(manifest), manifest: manifest)
        #expect(await eventually { sink.hasReady })
        try player.resume(at: StreamResumeAtPayload(
            streamId: "stream_mac", playbackGeneration: 7, seekGeneration: 0,
            commandSequence: 2, tCoordMs: 1_000, startDeadlineCoordMs: 2_000))
        var output = [Float](repeating: 0, count: 480)
        for _ in 0..<450 where !sink.hasRebuffer {
            _ = output.withUnsafeMutableBufferPointer {
                player.readPCM(into: $0.baseAddress!, count: $0.count)
            }
            try? await Task.sleep(nanoseconds: 200_000)
        }
        #expect(await eventually { sink.hasRebuffer })
        #expect(player.snapshot().state == .rebuffering)

        decoder.releaseRefill()
        #expect(await eventually { sink.readyCount == 2 })
        #expect(player.snapshot().state == .ready)
        try player.resume(at: StreamResumeAtPayload(
            streamId: "stream_mac", playbackGeneration: 7, seekGeneration: 0,
            commandSequence: 3, tCoordMs: 1_000, startDeadlineCoordMs: 2_000))
        output = [Float](repeating: 0, count: 480)
        for _ in 0..<50 where output.allSatisfy({ $0 == 0 }) {
            _ = output.withUnsafeMutableBufferPointer {
                player.readPCM(into: $0.baseAddress!, count: $0.count)
            }
            try? await Task.sleep(nanoseconds: 2_000_000)
        }
        #expect(output.allSatisfy { $0 == 0.5 })
    }

    @Test func productionCompositionStaysFailClosed() throws {
        let sourceRoot = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent()
            .deletingLastPathComponent().appendingPathComponent("Sources")
        let main = try String(contentsOf: sourceRoot.appendingPathComponent("NodeApp/main.swift"))
        let player = try String(contentsOf: sourceRoot.appendingPathComponent("NodeCore/PlayerCore.swift"))
        #expect(!main.contains("MacStreamCandidatePlayer"))
        #expect(!player.contains("MacStreamCandidatePlayer"))
        #expect(player.contains("rejecting unadvertised stream_track_v1 command"))
        #expect(MacStreamCacheLimits.maximumGlobalBytes == 512 << 20)
        #expect(MacStreamCacheLimits.maximumPerVariantBytes == 64 << 20)
        #expect(MacStreamCacheLimits.maximumPinnedBytes == 128 << 20)
        #expect(MacStreamCacheLimits.maximumChunkBytes == 1 << 20)
        #expect(macStreamPCMRingBytes == 1 << 20,
                "PCM memory is fixed and independent of a two-hour track duration")
    }
}
