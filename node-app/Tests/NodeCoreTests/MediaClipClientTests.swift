import Foundation
import Testing
@testable import NodeCore

private final class StubMediaClipFetcher: MediaClipFetching {
    let resultURL = URL(fileURLWithPath: "/tmp/media-clip-test.wav")
    var delayNanoseconds: UInt64 = 0
    var failure: MediaClipFailure?
    private let lock = NSLock()
    private var _fetchCount = 0
    private var _removed: [URL] = []

    func fetch(_ request: MediaClipFetchRequest) async throws -> URL {
        lock.withLock { _fetchCount += 1 }
        if delayNanoseconds > 0 {
            try await Task.sleep(nanoseconds: delayNanoseconds)
        }
        if let failure { throw failure }
        return resultURL
    }

    func remove(_ localURL: URL) {
        lock.withLock { _removed.append(localURL) }
    }

    var fetchCount: Int {
        lock.withLock { _fetchCount }
    }

    var removed: [URL] {
        lock.withLock { _removed }
    }
}

private final class StubMediaClipHTTPTransport: MediaClipHTTPTransport {
    var data = Data("hello".utf8)
    var statusCode = 200
    private let lock = NSLock()
    private var _requests: [URLRequest] = []

    func download(_ request: URLRequest) async throws -> (URL, URLResponse) {
        let (body, status) = lock.withLock {
            _requests.append(request)
            return (data, statusCode)
        }
        let temporary = FileManager.default.temporaryDirectory
            .appendingPathComponent("media-http-\(UUID().uuidString)")
        try body.write(to: temporary)
        let response = HTTPURLResponse(
            url: request.url!,
            statusCode: status,
            httpVersion: "HTTP/1.1",
            headerFields: ["Content-Length": "\(body.count)"])!
        return (temporary, response)
    }

    var requests: [URLRequest] {
        lock.withLock { _requests }
    }
}

private final class StubMediaClipMixer: MediaClipMixer {
    var deliveryCapabilities: [String]
    var decodedDurationMs: Int64 = 4_200
    var prepareFailure: MediaClipFailure?
    var cancelResult: Result<Bool, MediaClipFailure> = .success(false)
    private let lock = NSLock()
    private var started: ((Int64) -> Void)?
    private var ended: ((Int64) -> Void)?
    private var failed: ((MediaClipFailure) -> Void)?
    private var _armCount = 0
    private var _cancelCount = 0
    private var _disposeCount = 0
    private var _lastControl: MixerControlParameters?

    init(deliveryCapabilities: [String] = []) {
        self.deliveryCapabilities = deliveryCapabilities
    }

    func prepare(localURL: URL, delivery: String) throws -> PreparedMediaClip {
        if let prepareFailure { throw prepareFailure }
        return PreparedMediaClip(
            localURL: localURL,
            decodedDurationMs: decodedDurationMs,
            decoderHandle: NSObject())
    }

    func arm(
        _ clip: PreparedMediaClip,
        plan: MediaClipPlayPlan,
        onStarted: @escaping (Int64) -> Void,
        onEnded: @escaping (Int64) -> Void,
        onFailed: @escaping (MediaClipFailure) -> Void
    ) throws {
        lock.withLock {
            _armCount += 1
            _lastControl = plan.control
            started = onStarted
            ended = onEnded
            failed = onFailed
        }
    }

    func cancel(
        _ clip: PreparedMediaClip,
        command: CancelMediaPayload,
        completion: @escaping (Result<Bool, MediaClipFailure>) -> Void
    ) {
        lock.withLock { _cancelCount += 1 }
        completion(cancelResult)
    }

    func dispose(_ clip: PreparedMediaClip) {
        lock.withLock { _disposeCount += 1 }
    }

    func fireStarted(_ localMs: Int64) {
        let callback = lock.withLock { started }
        callback?(localMs)
    }

    func fireEnded(_ localMs: Int64) {
        let callback = lock.withLock { ended }
        callback?(localMs)
    }

    func fireFailed(_ failure: MediaClipFailure) {
        let callback = lock.withLock { failed }
        callback?(failure)
    }

    var armCount: Int {
        lock.withLock { _armCount }
    }

    var cancelCount: Int {
        lock.withLock { _cancelCount }
    }

    var disposeCount: Int {
        lock.withLock { _disposeCount }
    }

    var lastControl: MixerControlParameters? {
        lock.withLock { _lastControl }
    }
}

private struct RecordedMediaEvent: Equatable {
    enum Kind: Equatable { case ready, started, ended, failed, cancelled }
    let kind: Kind
    let generation: Int64
    let code: String?
    let timestamp: Int64?
}

private final class MediaEventRecorder {
    private let lock = NSLock()
    private var values: [RecordedMediaEvent] = []

    func record(_ message: Message) {
        let event: RecordedMediaEvent?
        switch message {
        case .mediaReady(let p):
            event = .init(kind: .ready, generation: p.generation, code: nil, timestamp: nil)
        case .mediaStarted(let p):
            event = .init(kind: .started, generation: p.generation,
                          code: nil, timestamp: p.tFirstSampleCoordMs)
        case .mediaEnded(let p):
            event = .init(kind: .ended, generation: p.generation,
                          code: p.reason, timestamp: p.tLastSampleCoordMs)
        case .mediaFailed(let p):
            event = .init(kind: .failed, generation: p.generation,
                          code: p.code, timestamp: nil)
        case .mediaCancelled(let p):
            event = .init(kind: .cancelled, generation: p.generation,
                          code: p.reason, timestamp: nil)
        default:
            event = nil
        }
        guard let event else { return }
        lock.withLock { values.append(event) }
    }

    var events: [RecordedMediaEvent] {
        lock.withLock { values }
    }
}

private func preparedClock() -> ClockSync {
    var clock = ClockSync()
    clock.addSample(t1: 1_000, t2: 1_000, t3: 1_000, t4: 1_000)
    return clock
}

private func preparePayload(
    generation: Int64 = 1,
    durationMs: Int64 = 4_200,
    deadline: Int64 = 20_000,
    expiry: Int64 = 30_000,
    delivery: String = "overlay"
) -> PrepareMediaPayload {
    PrepareMediaPayload(
        transmissionId: "tr_test",
        generation: generation,
        mediaId: "m_test",
        kind: "voice_clip",
        delivery: delivery,
        fileUrl: "https://coord.example/v1/media/m_test",
        sha256: String(repeating: "a", count: 64),
        sizeBytes: 100,
        durationMs: durationMs,
        mediaExpiresAtCoordMs: expiry,
        prepareDeadlineCoordMs: deadline)
}

private func overlayPlayPayload(generation: Int64 = 1) -> PlayMediaAtPayload {
    PlayMediaAtPayload(
        transmissionId: "tr_test",
        generation: generation,
        tCoordMs: 11_000,
        startDeadlineCoordMs: 11_100,
        delivery: "overlay",
        duckDb: -12,
        attackMs: 250,
        releaseMs: 600,
        fadeOutMs: nil,
        fadeInMs: nil)
}

private func interruptPlayPayload(generation: Int64 = 1) -> PlayMediaAtPayload {
    PlayMediaAtPayload(
        transmissionId: "tr_test",
        generation: generation,
        tCoordMs: 11_000,
        startDeadlineCoordMs: 11_100,
        delivery: "interrupt",
        duckDb: nil,
        attackMs: nil,
        releaseMs: nil,
        fadeOutMs: 250,
        fadeInMs: 120)
}

private func eventually(
    timeoutIterations: Int = 200,
    _ predicate: @escaping () -> Bool
) async -> Bool {
    for _ in 0..<timeoutIterations {
        if predicate() { return true }
        try? await Task.sleep(nanoseconds: 5_000_000)
    }
    return predicate()
}

private func makeClient(
    fetcher: StubMediaClipFetcher,
    mixer: StubMediaClipMixer,
    recorder: MediaEventRecorder,
    now: @escaping () -> Int64 = { 10_000 },
    clock: ClockSync? = preparedClock()
) -> MediaClipClient {
    let client = MediaClipClient(
        fetcher: fetcher,
        mixer: mixer,
        log: Logger(level: .error, path: nil),
        nowLocalMs: now)
    client.bind(
        send: recorder.record,
        clock: { clock },
        outputLatencyOffsetMs: 0)
    client.synchronize()
    return client
}

@Suite struct MediaClipClientTests {
    @Test func readyWaitsForAuthenticatedFetchAndDecoder() async {
        let fetcher = StubMediaClipFetcher()
        fetcher.delayNanoseconds = 80_000_000
        let mixer = StubMediaClipMixer()
        let recorder = MediaEventRecorder()
        let client = makeClient(fetcher: fetcher, mixer: mixer, recorder: recorder)
        defer { client.stop() }

        client.prepare(preparePayload())
        client.synchronize()
        #expect(recorder.events.isEmpty)
        #expect(await eventually { recorder.events.count == 1 })
        #expect(recorder.events == [
            .init(kind: .ready, generation: 1, code: nil, timestamp: nil)
        ])
    }

    @Test func frozenPrepareFailuresAreTypedAndTerminalOnce() async {
        let fetcher = StubMediaClipFetcher()
        fetcher.failure = .frozenCode("hash_mismatch")
        let mixer = StubMediaClipMixer()
        let recorder = MediaEventRecorder()
        let client = makeClient(fetcher: fetcher, mixer: mixer, recorder: recorder)
        defer { client.stop() }

        let payload = preparePayload()
        client.prepare(payload)
        #expect(await eventually { recorder.events.count == 1 })
        client.prepare(payload)
        client.synchronize()
        #expect(recorder.events == [
            .init(kind: .failed, generation: 1, code: "hash_mismatch", timestamp: nil)
        ])
    }

    @Test func exactDecodedDurationIsRequired() async {
        let fetcher = StubMediaClipFetcher()
        let mixer = StubMediaClipMixer()
        mixer.decodedDurationMs = 4_201
        let recorder = MediaEventRecorder()
        let client = makeClient(fetcher: fetcher, mixer: mixer, recorder: recorder)
        defer { client.stop() }

        client.prepare(preparePayload())
        #expect(await eventually { recorder.events.count == 1 })
        #expect(recorder.events.first?.code == "duration_mismatch")
        #expect(fetcher.removed == [fetcher.resultURL])
        #expect(mixer.disposeCount == 1)
    }

    @Test func decoderFailureUsesFrozenCodeAndRemovesDownload() async {
        let fetcher = StubMediaClipFetcher()
        let mixer = StubMediaClipMixer()
        mixer.prepareFailure = .frozenCode("decode_failed")
        let recorder = MediaEventRecorder()
        let client = makeClient(fetcher: fetcher, mixer: mixer, recorder: recorder)
        defer { client.stop() }

        client.prepare(preparePayload())
        #expect(await eventually { recorder.events.count == 1 })
        #expect(recorder.events.first?.code == "decode_failed")
        #expect(fetcher.removed == [fetcher.resultURL])
    }

    @Test func scheduledCallbacksEmitStartedAndEndedExactlyOnce() async {
        let fetcher = StubMediaClipFetcher()
        let mixer = StubMediaClipMixer(deliveryCapabilities: [overlayMixCapability])
        let recorder = MediaEventRecorder()
        let client = makeClient(fetcher: fetcher, mixer: mixer, recorder: recorder)
        defer { client.stop() }

        client.prepare(preparePayload())
        #expect(await eventually { recorder.events.count == 1 })
        client.play(overlayPlayPayload())
        client.synchronize()
        #expect(mixer.armCount == 1)
        #expect(mixer.lastControl == MixerControlParameters(overlayPlayPayload()))

        mixer.fireStarted(11_010)
        mixer.fireStarted(11_011)
        #expect(await eventually { recorder.events.count == 2 })
        mixer.fireEnded(15_210)
        mixer.fireEnded(15_211)
        #expect(await eventually { recorder.events.count == 3 })

        #expect(recorder.events.map(\.kind) == [.ready, .started, .ended])
        #expect(recorder.events[1].timestamp == 11_010)
        #expect(recorder.events[2].timestamp == 15_210)
        #expect(fetcher.removed == [fetcher.resultURL])
    }

    @Test func interruptPlaybackFailureIsTerminalInsteadOfFalseEnded() async {
        let fetcher = StubMediaClipFetcher()
        let mixer = StubMediaClipMixer(deliveryCapabilities: [interruptResumeCapability])
        let recorder = MediaEventRecorder()
        let client = makeClient(fetcher: fetcher, mixer: mixer, recorder: recorder)
        defer { client.stop() }

        client.prepare(preparePayload(delivery: "interrupt"))
        #expect(await eventually { recorder.events.count == 1 })
        client.play(interruptPlayPayload())
        client.synchronize()
        mixer.fireStarted(11_000)
        #expect(await eventually { recorder.events.count == 2 })
        mixer.fireFailed(.frozenCode("audio_graph_failed"))
        #expect(await eventually { recorder.events.count == 3 })
        mixer.fireEnded(15_000)
        client.synchronize()

        #expect(recorder.events.map(\.kind) == [.ready, .started, .failed])
        #expect(recorder.events.last?.code == "audio_graph_failed")
        #expect(mixer.disposeCount == 1)
    }

    @Test func reconnectResetCancelsInterruptAndRejectsLateCallbacks() async {
        let fetcher = StubMediaClipFetcher()
        let mixer = StubMediaClipMixer(deliveryCapabilities: [interruptResumeCapability])
        let recorder = MediaEventRecorder()
        let client = makeClient(fetcher: fetcher, mixer: mixer, recorder: recorder)
        defer { client.stop() }

        client.prepare(preparePayload(delivery: "interrupt"))
        #expect(await eventually { recorder.events.count == 1 })
        client.play(interruptPlayPayload())
        client.synchronize()
        mixer.fireStarted(11_000)
        #expect(await eventually { recorder.events.count == 2 })

        client.reset()
        client.synchronize()
        #expect(mixer.cancelCount == 1)
        #expect(mixer.disposeCount == 1)
        mixer.fireEnded(15_000)
        mixer.fireFailed(.frozenCode("audio_graph_failed"))
        client.synchronize()
        #expect(recorder.events.map(\.kind) == [.ready, .started])
    }

    @Test func playingGenerationCannotBeReplacedAndHigherCancelStopsFirst() async {
        let fetcher = StubMediaClipFetcher()
        let mixer = StubMediaClipMixer(deliveryCapabilities: [overlayMixCapability])
        let recorder = MediaEventRecorder()
        let client = makeClient(fetcher: fetcher, mixer: mixer, recorder: recorder)
        defer { client.stop() }

        client.prepare(preparePayload(generation: 1))
        #expect(await eventually { recorder.events.count == 1 })
        client.play(overlayPlayPayload(generation: 1))
        client.synchronize()
        mixer.fireStarted(11_000)
        #expect(await eventually { recorder.events.count == 2 })

        client.prepare(preparePayload(generation: 2))
        client.synchronize()
        #expect(fetcher.fetchCount == 1)
        #expect(mixer.cancelCount == 0)
        #expect(mixer.disposeCount == 0)

        client.cancel(CancelMediaPayload(
            transmissionId: "tr_test", generation: 2,
            reason: "media_deleted", action: "fade_stop",
            resumeMain: false, fadeMs: 120))
        #expect(await eventually { recorder.events.count == 3 })
        #expect(recorder.events.last == .init(
            kind: .cancelled, generation: 2, code: "media_deleted", timestamp: nil))
        #expect(mixer.cancelCount == 1)
        #expect(mixer.disposeCount == 1)

        mixer.fireEnded(15_000)
        client.synchronize()
        #expect(recorder.events.count == 3)
    }

    @Test func staleScheduleNeverArms() async {
        let fetcher = StubMediaClipFetcher()
        let mixer = StubMediaClipMixer(deliveryCapabilities: [overlayMixCapability])
        let recorder = MediaEventRecorder()
        let client = makeClient(
            fetcher: fetcher,
            mixer: mixer,
            recorder: recorder,
            now: { 12_000 })
        defer { client.stop() }

        client.prepare(preparePayload(deadline: 20_000, expiry: 30_000))
        #expect(await eventually { recorder.events.count == 1 })
        client.play(overlayPlayPayload())
        #expect(await eventually { recorder.events.count == 2 })
        #expect(mixer.armCount == 0)
        #expect(recorder.events.last?.code == "stale_play")
    }

    @Test func armedClipIsDisarmedWhenFirstSampleMissesDeadline() async {
        let fetcher = StubMediaClipFetcher()
        let mixer = StubMediaClipMixer(deliveryCapabilities: [overlayMixCapability])
        let recorder = MediaEventRecorder()
        let client = makeClient(fetcher: fetcher, mixer: mixer, recorder: recorder)
        defer { client.stop() }

        client.prepare(preparePayload())
        #expect(await eventually { recorder.events.count == 1 })
        client.play(PlayMediaAtPayload(
            transmissionId: "tr_test", generation: 1,
            tCoordMs: 9_930, startDeadlineCoordMs: 10_030,
            delivery: "overlay", duckDb: -12,
            attackMs: 250, releaseMs: 600,
            fadeOutMs: nil, fadeInMs: nil))
        #expect(await eventually { recorder.events.count == 2 })

        #expect(mixer.armCount == 1)
        #expect(mixer.cancelCount == 1)
        #expect(recorder.events.last?.code == "stale_play")
        #expect(fetcher.removed == [fetcher.resultURL])
    }

    @Test func unsynchronizedClockFailsBeforeArm() async {
        let fetcher = StubMediaClipFetcher()
        let mixer = StubMediaClipMixer(deliveryCapabilities: [overlayMixCapability])
        let recorder = MediaEventRecorder()
        let client = makeClient(
            fetcher: fetcher, mixer: mixer, recorder: recorder, clock: nil)
        defer { client.stop() }

        client.prepare(preparePayload())
        #expect(await eventually { recorder.events.count == 1 })
        client.play(overlayPlayPayload())
        #expect(await eventually { recorder.events.count == 2 })
        #expect(mixer.armCount == 0)
        #expect(recorder.events.last?.code == "clock_unsynchronized")
    }

    @Test func unadvertisedDeliveryCapabilityFailsBeforeArm() async {
        let fetcher = StubMediaClipFetcher()
        let mixer = StubMediaClipMixer()
        let recorder = MediaEventRecorder()
        let client = makeClient(fetcher: fetcher, mixer: mixer, recorder: recorder)
        defer { client.stop() }

        client.prepare(preparePayload())
        #expect(await eventually { recorder.events.count == 1 })
        client.play(overlayPlayPayload())
        #expect(await eventually { recorder.events.count == 2 })
        #expect(mixer.armCount == 0)
        #expect(recorder.events.last?.code == "capability_lost")
    }

    @Test func cancelTombstoneBlocksLatePrepareAndAcknowledgesOnce() async {
        let fetcher = StubMediaClipFetcher()
        let mixer = StubMediaClipMixer(deliveryCapabilities: [overlayMixCapability])
        let recorder = MediaEventRecorder()
        let client = makeClient(fetcher: fetcher, mixer: mixer, recorder: recorder)
        defer { client.stop() }
        let cancel = CancelMediaPayload(
            transmissionId: "tr_test", generation: 2,
            reason: "media_deleted", action: "disarm",
            resumeMain: false, fadeMs: 120)

        client.cancel(cancel)
        client.cancel(cancel)
        client.prepare(preparePayload(generation: 1))
        client.prepare(preparePayload(generation: 2))
        client.synchronize()
        #expect(recorder.events == [
            .init(kind: .cancelled, generation: 2, code: "media_deleted", timestamp: nil)
        ])
        #expect(fetcher.fetchCount == 0)
    }

    @Test func activeCancelSuppressesLateMixerCallbacks() async {
        let fetcher = StubMediaClipFetcher()
        let mixer = StubMediaClipMixer(deliveryCapabilities: [overlayMixCapability])
        let recorder = MediaEventRecorder()
        let client = makeClient(fetcher: fetcher, mixer: mixer, recorder: recorder)
        defer { client.stop() }

        client.prepare(preparePayload())
        #expect(await eventually { recorder.events.count == 1 })
        client.play(overlayPlayPayload())
        client.synchronize()
        let cancel = CancelMediaPayload(
            transmissionId: "tr_test", generation: 1,
            reason: "sender_cancelled", action: "disarm",
            resumeMain: false, fadeMs: 120)
        client.cancel(cancel)
        #expect(await eventually { recorder.events.count == 2 })
        mixer.fireStarted(11_000)
        mixer.fireEnded(15_000)
        client.synchronize()

        #expect(recorder.events.map(\.kind) == [.ready, .cancelled])
        #expect(mixer.cancelCount == 1)
        #expect(fetcher.removed == [fetcher.resultURL])
    }

    @Test func buildAdvertisesOnlyCapabilitiesItCanExecute() {
        let fetcher = StubMediaClipFetcher()
        let preparedOnly = MediaClipClient(
            fetcher: fetcher,
            mixer: PreparedOnlyMacMediaClipMixer(),
            log: Logger(level: .error, path: nil))
        defer { preparedOnly.stop() }
        #expect(preparedOnly.advertisedCapabilities == [mediaClipCapability])
        #expect(ProtocolCapabilities.areCanonical(
            [mediaClipCapability, seamlessAdoptionCapability]))
        let coordinator = CoordinatorClient(
            url: URL(string: "wss://coord.example/ws")!,
            identity: .init(
                nodeId: "a", token: "test", appVersion: "test",
                librespotVersion: "test"),
            capabilities: [mediaClipCapability, seamlessAdoptionCapability],
            log: Logger(level: .error, path: nil))
        #expect(coordinator.registeredCapabilities ==
                [mediaClipCapability, seamlessAdoptionCapability])
    }

    @Test func expiredPrepareNeverFetchesOrReportsReady() {
        let fetcher = StubMediaClipFetcher()
        let mixer = StubMediaClipMixer()
        let recorder = MediaEventRecorder()
        let client = makeClient(fetcher: fetcher, mixer: mixer, recorder: recorder)
        defer { client.stop() }

        client.prepare(preparePayload(deadline: 9_000, expiry: 9_500))
        client.synchronize()
        #expect(fetcher.fetchCount == 0)
        #expect(recorder.events == [
            .init(kind: .failed, generation: 1, code: "media_expired", timestamp: nil)
        ])
    }

    @Test func missedPrepareDeadlineIsLeftForCoordinatorTimeout() {
        let fetcher = StubMediaClipFetcher()
        let mixer = StubMediaClipMixer()
        let recorder = MediaEventRecorder()
        let client = makeClient(fetcher: fetcher, mixer: mixer, recorder: recorder)
        defer { client.stop() }

        client.prepare(preparePayload(deadline: 9_000, expiry: 30_000))
        client.synchronize()
        #expect(fetcher.fetchCount == 0)
        #expect(recorder.events.isEmpty)
    }
}

@Suite struct NodePresenceStoreTests {
    @Test func dndAndPresencePersistWithoutSensitiveFields() throws {
        let root = FileManager.default.temporaryDirectory
            .appendingPathComponent("node-presence-test-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: root) }
        let file = root.appendingPathComponent("state.json")
        let log = Logger(level: .error, path: nil)
        let store = NodePresenceStore(fileURL: file, log: log)

        let first = try store.nextLocalDND(
            mode: "messages_only", mutedUntilCoordMs: nil, coordinatorNowMs: 1_000)
        let second = try store.nextLocalDND(
            mode: "muted_until", mutedUntilCoordMs: 2_000, coordinatorNowMs: 1_000)
        #expect(first.revision == 1)
        #expect(second.revision == 2)

        let presence = PresenceUpdatePayload(
            revision: 9,
            generatedAtCoordMs: 1_100,
            nodes: [PresenceNode(
                orbitId: 42, slot: "a", online: true,
                lastSeenAtCoordMs: 1_099,
                outputState: "ready", playbackState: "main",
                dndMode: "muted_until", dndRevision: 2,
                dndUntilCoordMs: 2_000,
                capabilities: [mediaClipCapability],
                interruptResumeReady: false)])
        #expect(store.acceptPresence(presence))

        let reloaded = NodePresenceStore(fileURL: file, log: log)
        #expect(reloaded.currentLocalDND == second)
        #expect(reloaded.latestPresence == presence)
        let persistedText = try String(contentsOf: file, encoding: .utf8)
        for forbidden in ["microphone", "audio_level", "token", "local_path", "media_url"] {
            #expect(!persistedText.contains(forbidden))
        }
    }

    @Test func invalidDNDDoesNotAdvanceRevision() throws {
        let root = FileManager.default.temporaryDirectory
            .appendingPathComponent("node-presence-invalid-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: root) }
        let store = NodePresenceStore(
            fileURL: root.appendingPathComponent("state.json"),
            log: Logger(level: .error, path: nil))

        #expect(throws: NodePresenceStoreError.self) {
            _ = try store.nextLocalDND(
                mode: "muted_until", mutedUntilCoordMs: nil, coordinatorNowMs: 1_000)
        }
        let valid = try store.nextLocalDND(
            mode: "allow_all", mutedUntilCoordMs: nil, coordinatorNowMs: 1_000)
        #expect(valid.revision == 1)
    }
}

@Suite struct MediaHTTPOriginTests {
    @Test func websocketOriginMapsToExactAuthenticatedHTTPOrigin() throws {
        let origin = try #require(MediaHTTPOrigin(
            coordinatorWebSocketURL: URL(string: "wss://coord.example/ws")!))
        #expect(origin.permits(URL(string: "https://coord.example/v1/media/m")!))
        #expect(!origin.permits(URL(string: "http://coord.example/v1/media/m")!))
        #expect(!origin.permits(URL(string: "https://other.example/v1/media/m")!))
        #expect(!origin.permits(URL(string: "https://user:pass@coord.example/v1/media/m")!))
    }

    @Test func productionFetcherAuthenticatesHashesAndCleansOwnedFile() async throws {
        let root = FileManager.default.temporaryDirectory
            .appendingPathComponent("media-fetcher-test-\(UUID().uuidString)", isDirectory: true)
        defer { try? FileManager.default.removeItem(at: root) }
        let transport = StubMediaClipHTTPTransport()
        let fetcher = try AuthenticatedMediaClipFetcher(
            cacheDirectory: root,
            nodeToken: "node-test-token",
            coordinatorURL: URL(string: "wss://coord.example/ws")!,
            transport: transport)
        let request = MediaClipFetchRequest(
            remoteURL: "https://coord.example/v1/media/m_test",
            expectedSHA256: "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824",
            expectedSizeBytes: 5)

        let local = try await fetcher.fetch(request)
        #expect(try Data(contentsOf: local) == Data("hello".utf8))
        #expect(transport.requests.count == 1)
        #expect(transport.requests[0].value(forHTTPHeaderField: "Authorization") ==
                "Bearer node-test-token")
        fetcher.remove(local)
        #expect(!FileManager.default.fileExists(atPath: local.path))

        transport.statusCode = 401
        do {
            _ = try await fetcher.fetch(request)
            Issue.record("HTTP authentication failure must be typed")
        } catch let failure as MediaClipFailure {
            #expect(failure.code == "media_auth_failed")
        }
        #expect(transport.requests.count == 2)

        do {
            _ = try await fetcher.fetch(MediaClipFetchRequest(
                remoteURL: "https://other.example/v1/media/m_test",
                expectedSHA256: request.expectedSHA256,
                expectedSizeBytes: 5))
            Issue.record("cross-origin media URL must fail before transport")
        } catch let failure as MediaClipFailure {
            #expect(failure.code == "media_auth_failed")
        }
        #expect(transport.requests.count == 2)
    }
}
