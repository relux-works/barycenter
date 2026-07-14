// Generation-safe macOS client hooks for the frozen P1 media lifecycle.
// Network/file work stays on async worker paths; this controller's serial
// queue owns only lifecycle transitions and timers. The audio implementation
// is deliberately behind MediaClipMixer so later mixer tasks can add exact
// overlay/interrupt behavior without moving protocol state into render code.

import AVFAudio
import CryptoKit
import Foundation

enum MediaClipFailure: Error, Equatable {
    case frozenCode(String)

    var code: String {
        switch self {
        case .frozenCode(let code): return code
        }
    }
}

struct MediaClipFetchRequest {
    let remoteURL: String
    let expectedSHA256: String
    let expectedSizeBytes: Int64
}

protocol MediaClipFetching: AnyObject {
    func fetch(_ request: MediaClipFetchRequest) async throws -> URL
    func remove(_ localURL: URL)
}

private final class RejectRedirectsDelegate: NSObject, URLSessionTaskDelegate {
    func urlSession(
        _ session: URLSession,
        task: URLSessionTask,
        willPerformHTTPRedirection response: HTTPURLResponse,
        newRequest request: URLRequest,
        completionHandler: @escaping (URLRequest?) -> Void
    ) {
        // A same-origin redirect is unnecessary for the frozen endpoint and a
        // cross-origin redirect risks forwarding the node bearer. Refusing all
        // redirects is the smaller, safer client contract.
        completionHandler(nil)
    }
}

protocol MediaClipHTTPTransport: AnyObject {
    func download(_ request: URLRequest) async throws -> (URL, URLResponse)
}

private final class URLSessionMediaClipTransport: MediaClipHTTPTransport {
    private let redirectDelegate: RejectRedirectsDelegate
    private let session: URLSession

    init(configuration: URLSessionConfiguration) {
        let delegate = RejectRedirectsDelegate()
        redirectDelegate = delegate
        session = URLSession(configuration: configuration, delegate: delegate, delegateQueue: nil)
    }

    func download(_ request: URLRequest) async throws -> (URL, URLResponse) {
        try await session.download(for: request)
    }
}

struct MediaHTTPOrigin: Equatable {
    let scheme: String
    let host: String
    let port: Int

    init?(coordinatorWebSocketURL url: URL) {
        guard let rawScheme = url.scheme?.lowercased(),
              let host = url.host?.lowercased() else { return nil }
        let scheme: String
        switch rawScheme {
        case "ws": scheme = "http"
        case "wss": scheme = "https"
        case "http", "https": scheme = rawScheme
        default: return nil
        }
        self.scheme = scheme
        self.host = host
        self.port = url.port ?? (scheme == "https" ? 443 : 80)
    }

    func permits(_ url: URL) -> Bool {
        guard url.user == nil, url.password == nil,
              let scheme = url.scheme?.lowercased(),
              let host = url.host?.lowercased() else { return false }
        let port = url.port ?? (scheme == "https" ? 443 : 80)
        return scheme == self.scheme && host == self.host && port == self.port
    }
}

final class AuthenticatedMediaClipFetcher: MediaClipFetching {
    static let maximumCanonicalBytes = Int64(34 << 20)

    private let cacheDirectory: URL
    private let token: String
    private let origin: MediaHTTPOrigin
    private let transport: MediaClipHTTPTransport

    init(
        cacheDirectory: URL,
        nodeToken: String,
        coordinatorURL: URL,
        sessionConfiguration: URLSessionConfiguration? = nil,
        transport: MediaClipHTTPTransport? = nil
    ) throws {
        guard let origin = MediaHTTPOrigin(coordinatorWebSocketURL: coordinatorURL) else {
            throw MediaClipFailure.frozenCode("media_auth_failed")
        }
        self.cacheDirectory = cacheDirectory.standardizedFileURL
        self.token = nodeToken
        self.origin = origin
        let configuration = sessionConfiguration ?? URLSessionConfiguration.ephemeral
        configuration.timeoutIntervalForRequest = 10
        configuration.timeoutIntervalForResource = 30
        configuration.httpCookieStorage = nil
        configuration.urlCredentialStorage = nil
        configuration.requestCachePolicy = .reloadIgnoringLocalCacheData
        self.transport = transport ?? URLSessionMediaClipTransport(configuration: configuration)
        try FileManager.default.createDirectory(
            at: self.cacheDirectory, withIntermediateDirectories: true)
    }

    func fetch(_ request: MediaClipFetchRequest) async throws -> URL {
        guard request.expectedSizeBytes > 0,
              request.expectedSizeBytes <= Self.maximumCanonicalBytes,
              request.expectedSHA256.count == 64,
              request.expectedSHA256.utf8.allSatisfy({
                  ($0 >= 48 && $0 <= 57) || ($0 >= 97 && $0 <= 102)
              }),
              let remote = URL(string: request.remoteURL),
              origin.permits(remote) else {
            throw MediaClipFailure.frozenCode("media_auth_failed")
        }

        var urlRequest = URLRequest(url: remote)
        urlRequest.httpMethod = "GET"
        urlRequest.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        urlRequest.setValue("application/octet-stream, audio/wav", forHTTPHeaderField: "Accept")

        let local = cacheDirectory
            .appendingPathComponent(UUID().uuidString, isDirectory: false)
            .appendingPathExtension("media")
        var transportTemporary: URL?
        do {
            let (temporary, response) = try await transport.download(urlRequest)
            transportTemporary = temporary
            try Task.checkCancellation()
            guard let http = response as? HTTPURLResponse else {
                throw MediaClipFailure.frozenCode("media_download_failed")
            }
            switch http.statusCode {
            case 200: break
            case 401, 403:
                throw MediaClipFailure.frozenCode("media_auth_failed")
            case 404, 410:
                throw MediaClipFailure.frozenCode("media_expired")
            default:
                throw MediaClipFailure.frozenCode("media_download_failed")
            }
            if http.expectedContentLength > 0,
               http.expectedContentLength != request.expectedSizeBytes {
                throw MediaClipFailure.frozenCode("media_download_failed")
            }

            try? FileManager.default.removeItem(at: local)
            try FileManager.default.moveItem(at: temporary, to: local)
            transportTemporary = nil
            let attributes = try FileManager.default.attributesOfItem(atPath: local.path)
            guard let byteCount = attributes[.size] as? NSNumber,
                  byteCount.int64Value == request.expectedSizeBytes else {
                throw MediaClipFailure.frozenCode("media_download_failed")
            }
            guard try sha256(of: local) == request.expectedSHA256 else {
                throw MediaClipFailure.frozenCode("hash_mismatch")
            }
            return local
        } catch let failure as MediaClipFailure {
            if let transportTemporary { try? FileManager.default.removeItem(at: transportTemporary) }
            try? FileManager.default.removeItem(at: local)
            throw failure
        } catch is CancellationError {
            if let transportTemporary { try? FileManager.default.removeItem(at: transportTemporary) }
            try? FileManager.default.removeItem(at: local)
            throw CancellationError()
        } catch {
            if let transportTemporary { try? FileManager.default.removeItem(at: transportTemporary) }
            try? FileManager.default.removeItem(at: local)
            throw MediaClipFailure.frozenCode("media_download_failed")
        }
    }

    func remove(_ localURL: URL) {
        let standardized = localURL.standardizedFileURL
        let parent = standardized.deletingLastPathComponent()
        guard parent == cacheDirectory else { return }
        try? FileManager.default.removeItem(at: standardized)
    }

    private func sha256(of url: URL) throws -> String {
        let handle = try FileHandle(forReadingFrom: url)
        defer { try? handle.close() }
        var hasher = SHA256()
        while let chunk = try handle.read(upToCount: 64 * 1024), !chunk.isEmpty {
            hasher.update(data: chunk)
        }
        return hasher.finalize().map { String(format: "%02x", $0) }.joined()
    }
}

final class PreparedMediaClip: @unchecked Sendable {
    let localURL: URL
    let decodedDurationMs: Int64
    fileprivate let decoderHandle: AnyObject

    init(localURL: URL, decodedDurationMs: Int64, decoderHandle: AnyObject) {
        self.localURL = localURL
        self.decodedDurationMs = decodedDurationMs
        self.decoderHandle = decoderHandle
    }
}

struct MediaClipPlayPlan {
    let payload: PlayMediaAtPayload
    let localStartMs: Int64
    let localStartDeadlineMs: Int64
}

protocol MediaClipMixer: AnyObject {
    /// Only the delivery-specific capabilities whose exact behavior exists in
    /// this mixer build. media_clip_v1 is owned by the lifecycle client.
    var deliveryCapabilities: [String] { get }

    /// Opens/decodes enough state that arm() never has to perform file I/O.
    func prepare(localURL: URL, delivery: String) throws -> PreparedMediaClip
    func arm(
        _ clip: PreparedMediaClip,
        plan: MediaClipPlayPlan,
        onStarted: @escaping (Int64) -> Void,
        onEnded: @escaping (Int64) -> Void
    ) throws
    func cancel(
        _ clip: PreparedMediaClip,
        command: CancelMediaPayload,
        completion: @escaping (Result<Bool, MediaClipFailure>) -> Void
    )
    func dispose(_ clip: PreparedMediaClip)
}

/// The current macOS hook can prove authenticated bytes and decoder readiness.
/// Exact overlay/interrupt mixing is intentionally absent until the dedicated
/// mixer tasks land, so those two capabilities are not advertised here.
final class PreparedOnlyMacMediaClipMixer: MediaClipMixer {
    let deliveryCapabilities: [String] = []

    func prepare(localURL: URL, delivery: String) throws -> PreparedMediaClip {
        guard delivery == "overlay" || delivery == "interrupt" else {
            throw MediaClipFailure.frozenCode("decode_failed")
        }
        do {
            let file = try AVAudioFile(forReading: localURL)
            guard file.length > 0, file.processingFormat.sampleRate > 0 else {
                throw MediaClipFailure.frozenCode("decode_failed")
            }
            let duration = Int64(
                (Double(file.length) / file.processingFormat.sampleRate * 1000).rounded(.up))
            return PreparedMediaClip(
                localURL: localURL, decodedDurationMs: duration, decoderHandle: file)
        } catch let failure as MediaClipFailure {
            throw failure
        } catch {
            throw MediaClipFailure.frozenCode("decode_failed")
        }
    }

    func arm(
        _ clip: PreparedMediaClip,
        plan: MediaClipPlayPlan,
        onStarted: @escaping (Int64) -> Void,
        onEnded: @escaping (Int64) -> Void
    ) throws {
        let code = plan.payload.delivery == "interrupt"
            ? "interrupt_capability_lost" : "capability_lost"
        throw MediaClipFailure.frozenCode(code)
    }

    func cancel(
        _ clip: PreparedMediaClip,
        command: CancelMediaPayload,
        completion: @escaping (Result<Bool, MediaClipFailure>) -> Void
    ) {
        completion(.success(false))
    }

    func dispose(_ clip: PreparedMediaClip) {}
}

final class MediaClipClient: @unchecked Sendable {
    private enum Phase {
        case preparing, ready, armed, playing, cancelling, terminal
    }

    private final class Entry: @unchecked Sendable {
        let transmissionID: String
        let generation: Int64
        var preparePayload: PrepareMediaPayload?
        var playPayload: PlayMediaAtPayload?
        var phase: Phase
        var prepareTask: Task<Void, Never>?
        var prepared: PreparedMediaClip?
        var startDeadlineTimer: DispatchSourceTimer?
        var localStartDeadlineMs: Int64?
        var readySent = false
        var startedSent = false
        var terminalSent = false

        init(transmissionID: String, generation: Int64,
             preparePayload: PrepareMediaPayload?, phase: Phase) {
            self.transmissionID = transmissionID
            self.generation = generation
            self.preparePayload = preparePayload
            self.phase = phase
        }
    }

    private let fetcher: MediaClipFetching
    private let mixer: MediaClipMixer
    private let log: Logger
    private let queue = DispatchQueue(label: "duet.media-clip-client")
    private let nowLocalMs: () -> Int64
    private var entries: [String: Entry] = [:]
    private var sendMessage: ((Message) -> Void)?
    private var clockProvider: (() -> ClockSync?)?
    private var outputLatencyOffsetMs = 0

    init(
        fetcher: MediaClipFetching,
        mixer: MediaClipMixer,
        log: Logger,
        nowLocalMs: @escaping () -> Int64 = {
            Int64((Date().timeIntervalSince1970 * 1000).rounded())
        }
    ) {
        self.fetcher = fetcher
        self.mixer = mixer
        self.log = log
        self.nowLocalMs = nowLocalMs
    }

    var advertisedCapabilities: [String] {
        Array(Set([mediaClipCapability] + mixer.deliveryCapabilities)).sorted()
    }

    func bind(
        send: @escaping (Message) -> Void,
        clock: @escaping () -> ClockSync?,
        outputLatencyOffsetMs: Int
    ) {
        queue.async {
            self.sendMessage = send
            self.clockProvider = clock
            self.outputLatencyOffsetMs = outputLatencyOffsetMs
        }
    }

    func setOutputLatencyOffsetMs(_ value: Int) {
        queue.async { self.outputLatencyOffsetMs = value }
    }

    func prepare(_ payload: PrepareMediaPayload) {
        queue.async { self.beginPrepare(payload) }
    }

    func play(_ payload: PlayMediaAtPayload) {
        queue.async { self.beginPlay(payload) }
    }

    func cancel(_ payload: CancelMediaPayload) {
        queue.async { self.beginCancel(payload) }
    }

    func stop() {
        queue.sync {
            for entry in self.entries.values { self.discard(entry) }
            self.entries.removeAll()
        }
    }

    /// Test hook: waits until all already-enqueued lifecycle work has run.
    func synchronize() {
        queue.sync {}
    }

    private func beginPrepare(_ payload: PrepareMediaPayload) {
        guard validPrepare(payload) else {
            sendFailure(payload.transmissionId, payload.generation,
                        stage: "prepare", code: "internal_error")
            return
        }
        if let current = entries[payload.transmissionId] {
            if payload.generation < current.generation { return }
            if payload.generation == current.generation {
                guard current.preparePayload == payload else {
                    failAfterStoppingIfNeeded(
                        current, stage: "prepare", code: "internal_error")
                    return
                }
                return
            }
            discard(current)
        }

        let entry = Entry(
            transmissionID: payload.transmissionId,
            generation: payload.generation,
            preparePayload: payload,
            phase: .preparing)
        entries[payload.transmissionId] = entry

        let coordinatorNow = estimatedCoordinatorNowMs()
        if coordinatorNow >= payload.mediaExpiresAtCoordMs {
            fail(entry, stage: "prepare", code: "media_expired")
            return
        }
        if coordinatorNow >= payload.prepareDeadlineCoordMs {
            abandonLatePrepare(entry)
            return
        }

        let request = MediaClipFetchRequest(
            remoteURL: payload.fileUrl,
            expectedSHA256: payload.sha256,
            expectedSizeBytes: payload.sizeBytes)
        entry.prepareTask = Task { [weak self, weak entry] in
            guard let self, let entry else { return }
            var downloadedURL: URL?
            do {
                let localURL = try await self.fetcher.fetch(request)
                downloadedURL = localURL
                try Task.checkCancellation()
                let prepared = try self.mixer.prepare(
                    localURL: localURL, delivery: payload.delivery)
                downloadedURL = nil // ownership moves with PreparedMediaClip
                self.queue.async {
                    self.completePrepare(entry, prepared: prepared)
                }
            } catch is CancellationError {
                if let downloadedURL { self.fetcher.remove(downloadedURL) }
                return
            } catch let failure as MediaClipFailure {
                if let downloadedURL { self.fetcher.remove(downloadedURL) }
                self.queue.async { self.failIfCurrent(entry, stage: "prepare", code: failure.code) }
            } catch {
                if let downloadedURL { self.fetcher.remove(downloadedURL) }
                self.queue.async {
                    self.failIfCurrent(entry, stage: "prepare", code: "internal_error")
                }
            }
        }
    }

    private func completePrepare(_ entry: Entry, prepared: PreparedMediaClip) {
        guard isCurrent(entry), entry.phase == .preparing,
              let payload = entry.preparePayload else {
            mixer.dispose(prepared)
            fetcher.remove(prepared.localURL)
            return
        }
        entry.prepareTask = nil
        let coordinatorNow = estimatedCoordinatorNowMs()
        if coordinatorNow >= payload.mediaExpiresAtCoordMs {
            entry.prepared = prepared
            fail(entry, stage: "prepare", code: "media_expired")
            return
        }
        if coordinatorNow >= payload.prepareDeadlineCoordMs {
            entry.prepared = prepared
            abandonLatePrepare(entry)
            return
        }
        guard prepared.decodedDurationMs == payload.durationMs else {
            entry.prepared = prepared
            fail(entry, stage: "prepare", code: "duration_mismatch")
            return
        }

        entry.prepared = prepared
        entry.phase = .ready
        entry.readySent = true
        sendMessage?(.mediaReady(MediaReadyPayload(
            transmissionId: entry.transmissionID,
            generation: entry.generation,
            decodedDurationMs: prepared.decodedDurationMs)))
    }

    private func beginPlay(_ payload: PlayMediaAtPayload) {
        guard let entry = entries[payload.transmissionId],
              entry.generation == payload.generation else {
            // Reordered play is never cached as an implicit authorization. A
            // same-generation resend after readiness is safe and idempotent.
            return
        }
        if entry.phase == .armed || entry.phase == .playing {
            if entry.playPayload != payload {
                failAfterStoppingIfNeeded(
                    entry, stage: "schedule", code: "internal_error")
            }
            return
        }
        guard entry.phase == .ready,
              let prepared = entry.prepared,
              let prepare = entry.preparePayload,
              prepare.delivery == payload.delivery,
              validPlay(payload) else { return }

        let requiredCapability = payload.delivery == "interrupt"
            ? interruptResumeCapability : overlayMixCapability
        guard mixer.deliveryCapabilities.contains(requiredCapability) else {
            let code = payload.delivery == "interrupt"
                ? "interrupt_capability_lost" : "capability_lost"
            fail(entry, stage: "schedule", code: code)
            return
        }
        guard let clock = clockProvider?(),
              let localStart = clock.localDeadline(
                forCoordinatorMs: payload.tCoordMs,
                outputLatencyOffsetMs: outputLatencyOffsetMs),
              let localDeadline = clock.localDeadline(
                forCoordinatorMs: payload.startDeadlineCoordMs,
                outputLatencyOffsetMs: outputLatencyOffsetMs) else {
            fail(entry, stage: "schedule", code: "clock_unsynchronized")
            return
        }
        guard payload.startDeadlineCoordMs >= payload.tCoordMs,
              estimatedCoordinatorNowMs() < prepare.mediaExpiresAtCoordMs,
              nowLocalMs() <= localDeadline else {
            fail(entry, stage: "schedule", code: "stale_play")
            return
        }

        entry.phase = .armed
        entry.playPayload = payload
        entry.localStartDeadlineMs = localDeadline
        armStartDeadline(entry, localDeadlineMs: localDeadline)
        let plan = MediaClipPlayPlan(
            payload: payload,
            localStartMs: localStart,
            localStartDeadlineMs: localDeadline)
        do {
            try mixer.arm(
                prepared,
                plan: plan,
                onStarted: { [weak self, weak entry] localMs in
                    guard let self, let entry else { return }
                    self.queue.async { self.handleStarted(entry, localMs: localMs) }
                },
                onEnded: { [weak self, weak entry] localMs in
                    guard let self, let entry else { return }
                    self.queue.async { self.handleEnded(entry, localMs: localMs) }
                })
        } catch let failure as MediaClipFailure {
            entry.phase = .ready
            fail(entry, stage: "schedule", code: failure.code)
        } catch {
            entry.phase = .ready
            fail(entry, stage: "schedule", code: "audio_graph_failed")
        }
    }

    private func handleStarted(_ entry: Entry, localMs: Int64) {
        guard isCurrent(entry), entry.phase == .armed, !entry.startedSent else { return }
        guard let deadline = entry.localStartDeadlineMs, localMs <= deadline else {
            failAfterStoppingIfNeeded(
                entry, stage: "schedule", code: "stale_play")
            return
        }
        entry.startDeadlineTimer?.cancel()
        entry.startDeadlineTimer = nil
        entry.phase = .playing
        entry.startedSent = true
        sendMessage?(.mediaStarted(MediaStartedPayload(
            transmissionId: entry.transmissionID,
            generation: entry.generation,
            tFirstSampleCoordMs: coordinatorTimestamp(forLocalMs: localMs))))
    }

    private func handleEnded(_ entry: Entry, localMs: Int64) {
        guard isCurrent(entry), entry.phase == .playing,
              entry.startedSent, !entry.terminalSent else { return }
        finish(entry, message: .mediaEnded(MediaEndedPayload(
            transmissionId: entry.transmissionID,
            generation: entry.generation,
            tLastSampleCoordMs: coordinatorTimestamp(forLocalMs: localMs),
            reason: "completed")))
    }

    private func beginCancel(_ payload: CancelMediaPayload) {
        guard payload.generation > 0,
              payload.action == "disarm" || payload.action == "fade_stop" else {
            sendFailure(payload.transmissionId, payload.generation,
                        stage: "cancel", code: "internal_error")
            return
        }

        if let current = entries[payload.transmissionId] {
            if payload.generation < current.generation { return }
            if payload.generation > current.generation {
                discard(current)
                let tombstone = Entry(
                    transmissionID: payload.transmissionId,
                    generation: payload.generation,
                    preparePayload: nil,
                    phase: .terminal)
                tombstone.terminalSent = true
                entries[payload.transmissionId] = tombstone
                sendCancelled(payload, mainResumed: false)
                return
            }
            if current.phase == .terminal {
                if !current.terminalSent {
                    current.terminalSent = true
                    sendCancelled(payload, mainResumed: false)
                }
                return
            }
            if current.phase == .cancelling { return }

            current.prepareTask?.cancel()
            current.prepareTask = nil
            current.startDeadlineTimer?.cancel()
            current.startDeadlineTimer = nil
            guard let prepared = current.prepared else {
                finish(current, message: .mediaCancelled(MediaCancelledPayload(
                    transmissionId: payload.transmissionId,
                    generation: payload.generation,
                    reason: payload.reason,
                    action: payload.action,
                    mainResumed: false)))
                return
            }
            current.phase = .cancelling
            mixer.cancel(prepared, command: payload) { [weak self, weak current] result in
                guard let self, let current else { return }
                self.queue.async {
                    guard self.isCurrent(current), current.phase == .cancelling else { return }
                    switch result {
                    case .success(let mainResumed):
                        self.finish(current, message: .mediaCancelled(MediaCancelledPayload(
                            transmissionId: payload.transmissionId,
                            generation: payload.generation,
                            reason: payload.reason,
                            action: payload.action,
                            mainResumed: mainResumed)))
                    case .failure(let failure):
                        self.fail(current, stage: "cancel", code: failure.code)
                    }
                }
            }
            return
        }

        let tombstone = Entry(
            transmissionID: payload.transmissionId,
            generation: payload.generation,
            preparePayload: nil,
            phase: .terminal)
        tombstone.terminalSent = true
        entries[payload.transmissionId] = tombstone
        sendCancelled(payload, mainResumed: false)
    }

    private func armStartDeadline(_ entry: Entry, localDeadlineMs: Int64) {
        entry.startDeadlineTimer?.cancel()
        let timer = DispatchSource.makeTimerSource(flags: .strict, queue: queue)
        let delay = max(0, localDeadlineMs - nowLocalMs())
        timer.schedule(deadline: .now() + .milliseconds(Int(delay)), leeway: .milliseconds(1))
        timer.setEventHandler { [weak self, weak entry] in
            guard let self, let entry, self.isCurrent(entry), entry.phase == .armed else { return }
            self.failAfterStoppingIfNeeded(
                entry, stage: "schedule", code: "stale_play")
        }
        timer.resume()
        entry.startDeadlineTimer = timer
    }

    private func failIfCurrent(_ entry: Entry, stage: String, code: String) {
        guard isCurrent(entry), entry.phase != .terminal else { return }
        fail(entry, stage: stage, code: code)
    }

    private func fail(_ entry: Entry, stage: String, code: String) {
        guard !entry.terminalSent else { return }
        finish(entry, message: .mediaFailed(MediaFailedPayload(
            transmissionId: entry.transmissionID,
            generation: entry.generation,
            stage: stage,
            code: code)))
    }

    /// Once a mixer accepted arm(), terminal cleanup waits for its stop
    /// acknowledgement. This prevents a stale timer or conflicting duplicate
    /// from dropping the prepared file while a clip branch can still start.
    private func failAfterStoppingIfNeeded(_ entry: Entry, stage: String, code: String) {
        guard (entry.phase == .armed || entry.phase == .playing),
              let prepared = entry.prepared else {
            fail(entry, stage: stage, code: code)
            return
        }
        let wasPlaying = entry.phase == .playing
        entry.phase = .cancelling
        entry.startDeadlineTimer?.cancel()
        entry.startDeadlineTimer = nil
        mixer.cancel(
            prepared,
            command: CancelMediaPayload(
                transmissionId: entry.transmissionID,
                generation: entry.generation,
                reason: "coordinator_restarted",
                action: wasPlaying ? "fade_stop" : "disarm",
                resumeMain: wasPlaying && entry.playPayload?.delivery == "interrupt",
                fadeMs: 0),
            completion: { [weak self, weak entry] _ in
                guard let self, let entry else { return }
                self.queue.async {
                    guard self.isCurrent(entry), entry.phase == .cancelling else { return }
                    self.fail(entry, stage: stage, code: code)
                }
            })
    }

    private func finish(_ entry: Entry, message: Message) {
        guard !entry.terminalSent else { return }
        entry.terminalSent = true
        entry.phase = .terminal
        entry.prepareTask?.cancel()
        entry.prepareTask = nil
        entry.startDeadlineTimer?.cancel()
        entry.startDeadlineTimer = nil
        if let prepared = entry.prepared {
            mixer.dispose(prepared)
            fetcher.remove(prepared.localURL)
            entry.prepared = nil
        }
        sendMessage?(message)
    }

    private func abandonLatePrepare(_ entry: Entry) {
        entry.phase = .terminal
        entry.prepareTask?.cancel()
        entry.prepareTask = nil
        if let prepared = entry.prepared {
            mixer.dispose(prepared)
            fetcher.remove(prepared.localURL)
            entry.prepared = nil
        }
        log.debug("late media prepare discarded", [
            "transmission_id": entry.transmissionID,
            "generation": entry.generation,
        ])
    }

    private func discard(_ entry: Entry) {
        entry.prepareTask?.cancel()
        entry.prepareTask = nil
        entry.startDeadlineTimer?.cancel()
        entry.startDeadlineTimer = nil
        if let prepared = entry.prepared {
            let action = entry.phase == .playing ? "fade_stop" : "disarm"
            entry.prepared = nil
            mixer.cancel(
                prepared,
                command: CancelMediaPayload(
                    transmissionId: entry.transmissionID,
                    generation: entry.generation,
                    reason: "coordinator_restarted",
                    action: action,
                    resumeMain: entry.playPayload?.delivery == "interrupt" && entry.phase == .playing,
                    fadeMs: 0),
                completion: { [mixer, fetcher] _ in
                    mixer.dispose(prepared)
                    fetcher.remove(prepared.localURL)
                })
        }
        entry.phase = .terminal
    }

    private func isCurrent(_ entry: Entry) -> Bool {
        entries[entry.transmissionID] === entry
    }

    private func validPrepare(_ payload: PrepareMediaPayload) -> Bool {
        payload.generation > 0 &&
        !payload.transmissionId.isEmpty &&
        !payload.mediaId.isEmpty &&
        (payload.kind == "voice_clip" || payload.kind == "audio_clip") &&
        (payload.delivery == "overlay" || payload.delivery == "interrupt") &&
        payload.sizeBytes > 0 &&
        payload.sizeBytes <= AuthenticatedMediaClipFetcher.maximumCanonicalBytes &&
        payload.durationMs > 0 &&
        (payload.delivery != "overlay" || payload.durationMs <= 60_000) &&
        payload.mediaExpiresAtCoordMs > 0 &&
        payload.prepareDeadlineCoordMs > 0 &&
        payload.prepareDeadlineCoordMs <= payload.mediaExpiresAtCoordMs &&
        payload.sha256.count == 64 &&
        payload.sha256.utf8.allSatisfy({
            ($0 >= 48 && $0 <= 57) || ($0 >= 97 && $0 <= 102)
        })
    }

    private func validPlay(_ payload: PlayMediaAtPayload) -> Bool {
        let (lateWindow, overflow) = payload.startDeadlineCoordMs
            .subtractingReportingOverflow(payload.tCoordMs)
        guard !overflow, payload.tCoordMs > 0, lateWindow == 100 else { return false }
        switch payload.delivery {
        case "overlay":
            return payload.duckDb?.isFinite == true && payload.duckDb! <= 0 &&
                payload.attackMs.map { $0 >= 0 } == true &&
                payload.releaseMs.map { $0 >= 0 } == true &&
                payload.fadeOutMs == nil && payload.fadeInMs == nil
        case "interrupt":
            return payload.duckDb == nil && payload.attackMs == nil && payload.releaseMs == nil &&
                payload.fadeOutMs.map { $0 >= 0 } == true &&
                payload.fadeInMs.map { $0 >= 0 } == true
        default:
            return false
        }
    }

    private func estimatedCoordinatorNowMs() -> Int64 {
        coordinatorTimestamp(forLocalMs: nowLocalMs())
    }

    private func coordinatorTimestamp(forLocalMs localMs: Int64) -> Int64 {
        guard let offset = clockProvider?()?.offsetMs else { return localMs }
        return localMs - Int64(offset.rounded())
    }

    private func sendFailure(_ transmissionID: String, _ generation: Int64,
                             stage: String, code: String) {
        sendMessage?(.mediaFailed(MediaFailedPayload(
            transmissionId: transmissionID,
            generation: generation,
            stage: stage,
            code: code)))
    }

    private func sendCancelled(_ payload: CancelMediaPayload, mainResumed: Bool) {
        sendMessage?(.mediaCancelled(MediaCancelledPayload(
            transmissionId: payload.transmissionId,
            generation: payload.generation,
            reason: payload.reason,
            action: payload.action,
            mainResumed: mainResumed)))
    }
}
