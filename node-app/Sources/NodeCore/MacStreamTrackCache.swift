// Candidate-neutral macOS streamed-track range/cache seam.
//
// The accepted codec ADR keeps the production decoder registry empty. This
// file therefore owns only authenticated bytes, integrity and bounded disk
// state. A decoder receives verified chunks through MacStreamChunkReading and
// never receives a credential, URLSession or cache directory.

import CryptoKit
import Darwin
import Foundation

public enum MacStreamFailure: Error, Equatable, Sendable {
    case frozen(stage: String, code: String)

    public var stage: String {
        if case .frozen(let stage, _) = self { return stage }
        return "internal"
    }

    public var code: String {
        if case .frozen(_, let code) = self { return code }
        return "internal_error"
    }

    static func sanitized(_ error: Error) -> MacStreamFailure {
        guard let failure = error as? MacStreamFailure,
              validToken(failure.stage), validToken(failure.code) else {
            return .frozen(stage: "internal", code: "internal_error")
        }
        return failure
    }

    private static func validToken(_ value: String) -> Bool {
        !value.isEmpty && value.utf8.count <= 64 && value.utf8.allSatisfy {
            ($0 >= 97 && $0 <= 122) || ($0 >= 48 && $0 <= 57) || $0 == 95
        }
    }
}

public struct MacStreamChunk: Codable, Equatable, Sendable {
    public var index: Int
    public var start: Int64
    public var end: Int64
    public var sha256: String

    public init(index: Int, start: Int64, end: Int64, sha256: String) {
        self.index = index
        self.start = start
        self.end = end
        self.sha256 = sha256
    }
}

public struct MacStreamSeekPoint: Codable, Equatable, Sendable {
    public var timeMs: Int64
    public var offset: Int64

    public init(timeMs: Int64, offset: Int64) {
        self.timeMs = timeMs
        self.offset = offset
    }
}

public struct MacStreamManifest: Codable, Equatable, Sendable {
    public var identity: String
    public var variantUrl: String
    public var etag: String
    public var sha256: String
    public var sizeBytes: Int64
    public var durationMs: Int64
    public var chunks: [MacStreamChunk]
    public var seekMap: [MacStreamSeekPoint]

    public init(
        identity: String, variantUrl: String, etag: String, sha256: String,
        sizeBytes: Int64, durationMs: Int64, chunks: [MacStreamChunk],
        seekMap: [MacStreamSeekPoint]
    ) {
        self.identity = identity
        self.variantUrl = variantUrl
        self.etag = etag
        self.sha256 = sha256
        self.sizeBytes = sizeBytes
        self.durationMs = durationMs
        self.chunks = chunks
        self.seekMap = seekMap
    }

    public func chunkIndex(forTimeMs positionMs: Int64) -> Int {
        var point = seekMap[0]
        for candidate in seekMap.dropFirst() where candidate.timeMs <= positionMs {
            point = candidate
        }
        return chunks.firstIndex(where: { $0.start >= point.offset }) ?? max(0, chunks.count - 1)
    }

    public func validate(load: StreamLoadPayload? = nil) throws {
        let validIdentity = identity.hasPrefix("svm1.") && identity.utf8.count <= 512 &&
            identity.utf8.allSatisfy {
                ($0 >= 48 && $0 <= 57) || ($0 >= 65 && $0 <= 90) ||
                ($0 >= 97 && $0 <= 122) || $0 == 46 || $0 == 95 || $0 == 45
            }
        guard validIdentity, variantUrl.hasPrefix("/v1/media/"),
              !variantUrl.contains("://"), !variantUrl.contains("?"),
              !variantUrl.contains("#"), !variantUrl.contains("@"),
              validLowerSHA256(sha256), etag == "\"sha256-\(sha256)\"",
              sizeBytes > 0, durationMs > 0, !chunks.isEmpty, !seekMap.isEmpty else {
            throw MacStreamFailure.frozen(stage: "manifest", code: "invalid_manifest")
        }
        var next: Int64 = 0
        var starts = Set<Int64>()
        for (expectedIndex, chunk) in chunks.enumerated() {
            guard chunk.index == expectedIndex, chunk.start == next, chunk.end >= chunk.start,
                  chunk.end - chunk.start + 1 <= MacStreamCacheLimits.maximumChunkBytes,
                  validLowerSHA256(chunk.sha256) else {
                throw MacStreamFailure.frozen(stage: "manifest", code: "invalid_manifest")
            }
            starts.insert(chunk.start)
            next = chunk.end + 1
        }
        guard next == sizeBytes else {
            throw MacStreamFailure.frozen(stage: "manifest", code: "invalid_manifest")
        }
        for (index, point) in seekMap.enumerated() {
            let previous = index > 0 ? seekMap[index - 1] : nil
            guard point.timeMs >= 0, point.timeMs <= durationMs, starts.contains(point.offset),
                  (index != 0 || (point.timeMs == 0 && point.offset == 0)),
                  previous == nil || (point.timeMs > previous!.timeMs &&
                    point.offset >= previous!.offset &&
                    point.timeMs - previous!.timeMs <= 10_000) else {
                throw MacStreamFailure.frozen(stage: "manifest", code: "invalid_manifest")
            }
        }
        if let load {
            guard StreamContract.validate(load: load), identity == load.variantManifest,
                  variantUrl == load.variantUrl, etag == load.variantEtag,
                  sha256 == load.variantSha256, sizeBytes == load.variantSizeBytes,
                  load.startPositionMs <= durationMs else {
                throw MacStreamFailure.frozen(stage: "manifest", code: "invalid_manifest")
            }
        }
    }
}

private func validLowerSHA256(_ value: String) -> Bool {
    value.count == 64 && value.utf8.allSatisfy {
        ($0 >= 48 && $0 <= 57) || ($0 >= 97 && $0 <= 102)
    }
}

private func lowerSHA256(_ data: Data) -> String {
    SHA256.hash(data: data).map { String(format: "%02x", $0) }.joined()
}

public protocol MacStreamRangeFetching: AnyObject, Sendable {
    func fetchRange(
        path: String, etag: String, start: Int64, end: Int64
    ) async throws -> (data: Data, etag: String)
}

private final class MacStreamBoundedRequest: NSObject, URLSessionDataDelegate, @unchecked Sendable {
    private let maximumBytes: Int
    private let lock = NSLock()
    private var data = Data()
    private var response: URLResponse?
    private var continuation: CheckedContinuation<(Data, URLResponse), Error>?
    private var session: URLSession?
    private var task: URLSessionDataTask?
    private var cancelled = false
    private var terminalError: Error?

    init(maximumBytes: Int) { self.maximumBytes = maximumBytes }

    func perform(
        request: URLRequest, configuration: URLSessionConfiguration
    ) async throws -> (Data, URLResponse) {
        try await withTaskCancellationHandler {
            try await withCheckedThrowingContinuation { continuation in
                let task: URLSessionDataTask? = lock.withLock {
                    guard !cancelled else { return nil }
                    self.continuation = continuation
                    let session = URLSession(
                        configuration: configuration, delegate: self, delegateQueue: nil)
                    self.session = session
                    let task = session.dataTask(with: request)
                    self.task = task
                    return task
                }
                if let task {
                    task.resume()
                } else {
                    continuation.resume(throwing: CancellationError())
                }
            }
        } onCancel: {
            let task = self.lock.withLock { () -> URLSessionDataTask? in
                self.cancelled = true
                return self.task
            }
            task?.cancel()
        }
    }

    func urlSession(
        _ session: URLSession, task: URLSessionTask,
        willPerformHTTPRedirection response: HTTPURLResponse,
        newRequest request: URLRequest,
        completionHandler: @escaping (URLRequest?) -> Void
    ) {
        completionHandler(nil)
    }

    func urlSession(
        _ session: URLSession, dataTask: URLSessionDataTask,
        didReceive response: URLResponse,
        completionHandler: @escaping (URLSession.ResponseDisposition) -> Void
    ) {
        let accept = lock.withLock {
            self.response = response
            if response.expectedContentLength > Int64(maximumBytes) {
                terminalError = MacStreamFailure.frozen(stage: "fetch", code: "range_too_large")
                return false
            }
            return true
        }
        completionHandler(accept ? .allow : .cancel)
    }

    func urlSession(_ session: URLSession, dataTask: URLSessionDataTask, didReceive bytes: Data) {
        let overflow = lock.withLock {
            guard terminalError == nil else { return true }
            guard data.count + bytes.count <= maximumBytes else {
                terminalError = MacStreamFailure.frozen(stage: "fetch", code: "range_too_large")
                return true
            }
            data.append(bytes)
            return false
        }
        if overflow { dataTask.cancel() }
    }

    func urlSession(
        _ session: URLSession, task: URLSessionTask,
        didCompleteWithError error: Error?
    ) {
        let result: Result<(Data, URLResponse), Error> = lock.withLock {
            if let terminalError { return .failure(terminalError) }
            if cancelled { return .failure(CancellationError()) }
            if let error { return .failure(error) }
            guard let response else {
                return .failure(MacStreamFailure.frozen(
                    stage: "fetch", code: "network_failed"))
            }
            return .success((data, response))
        }
        finish(result)
    }

    private func finish(_ result: Result<(Data, URLResponse), Error>) {
        let state = lock.withLock { () -> (CheckedContinuation<(Data, URLResponse), Error>?, URLSession?) in
            let continuation = self.continuation
            self.continuation = nil
            let session = self.session
            self.session = nil
            task = nil
            return (continuation, session)
        }
        state.1?.finishTasksAndInvalidate()
        state.0?.resume(with: result)
    }
}

/// Exact authenticated single-range client. It is candidate infrastructure,
/// not a registered decoder or production capability.
public final class MacStreamHTTPRangeFetcher: MacStreamRangeFetching, @unchecked Sendable {
    private let origin: MediaHTTPOrigin
    private let baseURL: URL
    private let token: String
    private let configuration: URLSessionConfiguration

    public init(
        coordinatorURL: URL, nodeToken: String,
        sessionConfiguration: URLSessionConfiguration? = nil
    ) throws {
        guard !nodeToken.isEmpty,
              let origin = MediaHTTPOrigin(coordinatorWebSocketURL: coordinatorURL) else {
            throw MacStreamFailure.frozen(stage: "fetch", code: "invalid_configuration")
        }
        var components = URLComponents(url: coordinatorURL, resolvingAgainstBaseURL: false)
        if components?.scheme == "wss" { components?.scheme = "https" }
        if components?.scheme == "ws" { components?.scheme = "http" }
        components?.path = ""
        components?.query = nil
        components?.fragment = nil
        guard let baseURL = components?.url else {
            throw MacStreamFailure.frozen(stage: "fetch", code: "invalid_configuration")
        }
        self.origin = origin
        self.baseURL = baseURL
        token = nodeToken
        let configuration = sessionConfiguration ?? URLSessionConfiguration.ephemeral
        configuration.timeoutIntervalForRequest = 10
        configuration.timeoutIntervalForResource = 30
        configuration.httpCookieStorage = nil
        configuration.urlCredentialStorage = nil
        configuration.requestCachePolicy = .reloadIgnoringLocalCacheData
        self.configuration = configuration
    }

    public func fetchRange(
        path: String, etag: String, start: Int64, end: Int64
    ) async throws -> (data: Data, etag: String) {
        guard path.hasPrefix("/v1/media/"), !path.contains("://"),
              !path.contains("?"), !path.contains("#"), !path.contains("@"),
              validLowerSHA256(String(etag.dropFirst(8).dropLast())),
              start >= 0, end >= start,
              end - start + 1 <= MacStreamCacheLimits.maximumNetworkBytes,
              let url = URL(string: path, relativeTo: baseURL)?.absoluteURL,
              origin.permits(url) else {
            throw MacStreamFailure.frozen(stage: "fetch", code: "invalid_range")
        }
        var request = URLRequest(url: url)
        request.httpMethod = "GET"
        request.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        request.setValue("bytes=\(start)-\(end)", forHTTPHeaderField: "Range")
        request.setValue(etag, forHTTPHeaderField: "If-Range")
        request.setValue("application/octet-stream", forHTTPHeaderField: "Accept")
        do {
            let bounded = MacStreamBoundedRequest(
                maximumBytes: MacStreamCacheLimits.maximumNetworkBytes)
            let (data, response) = try await bounded.perform(
                request: request, configuration: configuration)
            guard let http = response as? HTTPURLResponse else {
                throw MacStreamFailure.frozen(stage: "fetch", code: "network_failed")
            }
            switch http.statusCode {
            case 206: break
            case 401, 403, 404, 410:
                throw MacStreamFailure.frozen(stage: "fetch", code: "revoked")
            case 200, 412, 416:
                throw MacStreamFailure.frozen(stage: "fetch", code: "etag_changed")
            default:
                throw MacStreamFailure.frozen(stage: "fetch", code: "network_failed")
            }
            guard http.value(forHTTPHeaderField: "ETag") == etag else {
                throw MacStreamFailure.frozen(stage: "fetch", code: "etag_changed")
            }
            guard validContentRange(
                    http.value(forHTTPHeaderField: "Content-Range"),
                    start: start, end: end),
                  data.count == Int(end - start + 1) else {
                throw MacStreamFailure.frozen(stage: "fetch", code: "invalid_range")
            }
            return (data, etag)
        } catch let failure as MacStreamFailure {
            throw failure
        } catch is CancellationError {
            throw CancellationError()
        } catch {
            throw MacStreamFailure.frozen(stage: "fetch", code: "network_failed")
        }
    }

    private func validContentRange(_ value: String?, start: Int64, end: Int64) -> Bool {
        guard let value else { return false }
        let parts = value.split(separator: "/", omittingEmptySubsequences: false)
        guard parts.count == 2, parts[0] == "bytes \(start)-\(end)",
              let total = Int64(parts[1]), total > end else { return false }
        return true
    }
}

public struct MacStreamCacheLimits: Equatable, Sendable {
    public static let maximumGlobalBytes: Int64 = 512 << 20
    public static let maximumPerVariantBytes: Int64 = 64 << 20
    public static let maximumPinnedBytes: Int64 = 128 << 20
    public static let maximumChunkBytes: Int64 = 1 << 20
    public static let maximumNetworkBytes = 1 << 20

    public var globalBytes: Int64
    public var perVariantBytes: Int64
    public var pinnedBytes: Int64
    public var chunkBytes: Int64
    public var networkBytes: Int64

    public init(
        globalBytes: Int64 = maximumGlobalBytes,
        perVariantBytes: Int64 = maximumPerVariantBytes,
        pinnedBytes: Int64 = maximumPinnedBytes,
        chunkBytes: Int64 = maximumChunkBytes,
        networkBytes: Int64 = Int64(maximumNetworkBytes)
    ) {
        self.globalBytes = globalBytes
        self.perVariantBytes = perVariantBytes
        self.pinnedBytes = pinnedBytes
        self.chunkBytes = chunkBytes
        self.networkBytes = networkBytes
    }
}

public struct MacStreamCacheStats: Equatable, Sendable {
    public var hits = 0
    public var fetches = 0
    public var evictions = 0
    public var integrityFailures = 0
    public var repairs = 0
    public var bytes: Int64 = 0
    public var pinnedBytes: Int64 = 0
}

private struct MacStreamCacheEntry: Codable, Sendable {
    var key: String
    var variantKey: String
    var chunkIndex: Int
    var size: Int64
    var lastUse: Int64
    var pinned = false

    enum CodingKeys: String, CodingKey { case key, variantKey, chunkIndex, size, lastUse }
}

private struct MacStreamCacheIndex: Codable {
    var version = 1
    var entries: [MacStreamCacheEntry] = []
    var tombstones: [String] = []
}

public actor MacStreamChunkCache {
    private nonisolated static let processLock = NSRecursiveLock()
    private let directory: URL
    private let indexURL: URL
    private let secret: SymmetricKey
    private let fetcher: MacStreamRangeFetching
    private let limits: MacStreamCacheLimits
    private var entries: [String: MacStreamCacheEntry] = [:]
    private var tombstones = Set<String>()
    private var clock: Int64 = 0
    private var counters = MacStreamCacheStats()

    public init(
        root: URL, installationSecret: Data, fetcher: MacStreamRangeFetching,
        limits: MacStreamCacheLimits = .init()
    ) throws {
        guard installationSecret.count >= 16, limits.globalBytes > 0,
              limits.perVariantBytes > 0, limits.pinnedBytes > 0,
              limits.chunkBytes > 0, limits.chunkBytes <= MacStreamCacheLimits.maximumChunkBytes,
              limits.networkBytes > 0,
              limits.networkBytes <= MacStreamCacheLimits.maximumNetworkBytes,
              limits.perVariantBytes <= limits.globalBytes,
              limits.pinnedBytes <= limits.globalBytes else {
            throw MacStreamFailure.frozen(stage: "cache", code: "invalid_configuration")
        }
        directory = root.standardizedFileURL.appendingPathComponent("stream-v1", isDirectory: true)
        indexURL = directory.appendingPathComponent("index-v1.json")
        secret = SymmetricKey(data: installationSecret)
        self.fetcher = fetcher
        self.limits = limits
        Self.processLock.lock()
        defer { Self.processLock.unlock() }
        try FileManager.default.createDirectory(
            at: directory, withIntermediateDirectories: true,
            attributes: [.posixPermissions: 0o700])
        var loadedEntries: [String: MacStreamCacheEntry] = [:]
        var loadedTombstones = Set<String>()
        var loadedClock: Int64 = 0
        var loadedCounters = MacStreamCacheStats()
        var decoded = MacStreamCacheIndex()
        if FileManager.default.fileExists(atPath: indexURL.path) {
            do {
                decoded = try JSONDecoder().decode(
                    MacStreamCacheIndex.self, from: Data(contentsOf: indexURL))
            } catch {
                loadedCounters.repairs += 1
            }
        }
        if decoded.version != 1 {
            decoded = MacStreamCacheIndex()
            loadedCounters.repairs += 1
        }
        loadedTombstones = Set(decoded.tombstones.filter { validLowerSHA256($0) })
        var owned = Set<String>()
        for var entry in decoded.entries {
            let url = directory.appendingPathComponent(entry.key).appendingPathExtension("chunk")
            var isDirectory: ObjCBool = false
            let exists = FileManager.default.fileExists(atPath: url.path, isDirectory: &isDirectory)
            guard exists, !isDirectory.boolValue, validLowerSHA256(entry.key),
                  validLowerSHA256(entry.variantKey), entry.size > 0,
                  entry.size <= limits.chunkBytes, !loadedTombstones.contains(entry.variantKey),
                  (try? url.resourceValues(forKeys: [.isSymbolicLinkKey]).isSymbolicLink) != true,
                  (try? url.resourceValues(forKeys: [.fileSizeKey]).fileSize) == Int(entry.size) else {
                try? FileManager.default.removeItem(at: url)
                loadedCounters.repairs += 1
                continue
            }
            entry.pinned = false
            loadedEntries[entry.key] = entry
            owned.insert(url.lastPathComponent)
            loadedClock = max(loadedClock, entry.lastUse)
        }
        for url in (try? FileManager.default.contentsOfDirectory(
            at: directory, includingPropertiesForKeys: nil)) ?? [] {
            if url == indexURL { continue }
            if url.pathExtension == "part" ||
                (url.pathExtension == "chunk" && !owned.contains(url.lastPathComponent)) {
                try? FileManager.default.removeItem(at: url)
                loadedCounters.repairs += 1
            }
        }
        while loadedEntries.values.reduce(Int64(0), { $0 + $1.size }) > limits.globalBytes ||
                Dictionary(grouping: loadedEntries.values, by: \.variantKey).values.contains(where: {
                    $0.reduce(Int64(0), { $0 + $1.size }) > limits.perVariantBytes
                }) {
            guard let victim = loadedEntries.values.min(by: { $0.lastUse < $1.lastUse }) else { break }
            loadedEntries.removeValue(forKey: victim.key)
            try? FileManager.default.removeItem(
                at: directory.appendingPathComponent(victim.key).appendingPathExtension("chunk"))
            loadedCounters.repairs += 1
        }
        let repairedIndex = MacStreamCacheIndex(
            entries: loadedEntries.values.sorted { $0.key < $1.key },
            tombstones: loadedTombstones.sorted())
        try JSONEncoder().encode(repairedIndex).write(to: indexURL, options: .atomic)
        entries = loadedEntries
        tombstones = loadedTombstones
        clock = loadedClock
        counters = loadedCounters
    }

    public func chunk(_ manifest: MacStreamManifest, index: Int) async throws -> Data {
        try manifest.validate()
        guard chunksRange(manifest.chunks).contains(index) else {
            throw MacStreamFailure.frozen(stage: "manifest", code: "invalid_manifest")
        }
        let variantKey = variantKey(for: manifest)
        let chunk = manifest.chunks[index]
        let key = chunkKey(variantKey: variantKey, index: index)
        if let cached: Data = try Self.withProcessLock({
            try synchronizeLocked()
            guard !tombstones.contains(variantKey) else {
                throw MacStreamFailure.frozen(stage: "fetch", code: "revoked")
            }
            if var entry = entries[key], let data = try? Data(contentsOf: chunkURL(key)),
               Int64(data.count) == entry.size, lowerSHA256(data) == chunk.sha256 {
                clock += 1
                entry.lastUse = clock
                entries[key] = entry
                counters.hits += 1
                try persistLocked()
                return data
            }
            if entries[key] != nil {
                removeEntry(key)
                counters.integrityFailures += 1
                try persistLocked()
            }
            return nil
        }) {
            return cached
        }

        let expected = chunk.end - chunk.start + 1
        guard expected <= limits.chunkBytes, expected <= limits.networkBytes else {
            throw MacStreamFailure.frozen(stage: "fetch", code: "range_too_large")
        }
        var accepted: Data?
        for attempt in 0..<2 {
            counters.fetches += 1
            do {
                let response = try await fetcher.fetchRange(
                    path: manifest.variantUrl, etag: manifest.etag,
                    start: chunk.start, end: chunk.end)
                guard response.etag == manifest.etag else {
                    try invalidate(manifest)
                    throw MacStreamFailure.frozen(stage: "fetch", code: "etag_changed")
                }
                if Int64(response.data.count) == expected,
                   lowerSHA256(response.data) == chunk.sha256 {
                    accepted = response.data
                    break
                }
                counters.integrityFailures += 1
                if attempt == 1 {
                    throw MacStreamFailure.frozen(stage: "integrity", code: "chunk_hash_mismatch")
                }
            } catch let failure as MacStreamFailure {
                if failure.code == "revoked" { try tombstone(manifest) }
                if failure.code == "etag_changed" { try invalidate(manifest) }
                if failure.code != "network_failed" || attempt == 1 { throw failure }
            }
        }
        guard let data = accepted else {
            throw MacStreamFailure.frozen(stage: "integrity", code: "chunk_hash_mismatch")
        }
        return try Self.withProcessLock {
            try synchronizeLocked()
            guard !tombstones.contains(variantKey) else {
                throw MacStreamFailure.frozen(stage: "fetch", code: "revoked")
            }
            try atomicWrite(data, to: chunkURL(key))
            clock += 1
            entries[key] = MacStreamCacheEntry(
                key: key, variantKey: variantKey, chunkIndex: index,
                size: Int64(data.count), lastUse: clock)
            do {
                try enforceLimits(protecting: key)
                try persistLocked()
            } catch {
                removeEntry(key)
                try? persistLocked()
                throw error
            }
            return data
        }
    }

    public func setPinned(_ manifest: MacStreamManifest, indexes: [Int]) throws {
        try manifest.validate()
        try Self.withProcessLock {
            try synchronizeLocked()
            let variantKey = variantKey(for: manifest)
            let desired = Set(indexes.map { chunkKey(variantKey: variantKey, index: $0) })
            for key in entries.keys {
                guard var entry = entries[key], entry.variantKey == variantKey else { continue }
                entry.pinned = desired.contains(key)
                entries[key] = entry
            }
            let pinned = entries.values.filter(\.pinned).reduce(Int64(0)) { $0 + $1.size }
            guard pinned <= limits.pinnedBytes else {
                for key in entries.keys {
                    guard var entry = entries[key], entry.variantKey == variantKey else { continue }
                    entry.pinned = false
                    entries[key] = entry
                }
                throw MacStreamFailure.frozen(stage: "cache", code: "pinned_limit")
            }
            try persistLocked()
        }
    }

    public func invalidate(_ manifest: MacStreamManifest) throws {
        try Self.withProcessLock {
            try synchronizeLocked()
            let key = variantKey(for: manifest)
            for entryKey in entries.keys where entries[entryKey]?.variantKey == key {
                removeEntry(entryKey)
            }
            try persistLocked()
        }
    }

    public func tombstone(_ manifest: MacStreamManifest) throws {
        try Self.withProcessLock {
            try synchronizeLocked()
            let key = variantKey(for: manifest)
            tombstones.insert(key)
            for entryKey in entries.keys where entries[entryKey]?.variantKey == key {
                removeEntry(entryKey)
            }
            try persistLocked()
        }
    }

    public func stats() -> MacStreamCacheStats {
        Self.withProcessLock { try? synchronizeLocked() }
        var result = counters
        result.bytes = entries.values.reduce(Int64(0)) { $0 + $1.size }
        result.pinnedBytes = entries.values.filter(\.pinned).reduce(Int64(0)) { $0 + $1.size }
        return result
    }

    private func enforceLimits(protecting protected: String?) throws {
        while totalBytes() > limits.globalBytes || variantOverflowExists() {
            guard let victim = entries.values
                .filter({ !$0.pinned && $0.key != protected })
                .min(by: { $0.lastUse < $1.lastUse }) else {
                throw MacStreamFailure.frozen(stage: "cache", code: "cache_limit")
            }
            removeEntry(victim.key)
            counters.evictions += 1
        }
    }

    private func variantOverflowExists() -> Bool {
        Dictionary(grouping: entries.values, by: \.variantKey).values.contains {
            $0.reduce(Int64(0)) { $0 + $1.size } > limits.perVariantBytes
        }
    }

    private func totalBytes() -> Int64 { entries.values.reduce(0) { $0 + $1.size } }

    private func removeEntry(_ key: String) {
        entries.removeValue(forKey: key)
        try? FileManager.default.removeItem(at: chunkURL(key))
    }

    private func variantKey(for manifest: MacStreamManifest) -> String {
        hmac(["variant", manifest.identity, manifest.etag])
    }

    private func chunkKey(variantKey: String, index: Int) -> String {
        hmac(["chunk", variantKey, String(index)])
    }

    private func hmac(_ parts: [String]) -> String {
        var data = Data()
        for (index, part) in parts.enumerated() {
            if index > 0 { data.append(0) }
            data.append(contentsOf: part.utf8)
        }
        return HMAC<SHA256>.authenticationCode(for: data, using: secret)
            .map { String(format: "%02x", $0) }.joined()
    }

    private func chunkURL(_ key: String) -> URL {
        directory.appendingPathComponent(key).appendingPathExtension("chunk")
    }

    private nonisolated static func withProcessLock<Result>(
        _ body: () throws -> Result
    ) rethrows -> Result {
        processLock.lock()
        defer { processLock.unlock() }
        return try body()
    }

    /// Refreshes this actor's view while the process-wide cache lock is held.
    /// Tombstones are a monotonic union; entries survive only while their
    /// immutable chunk file still exists and no instance has revoked them.
    private func synchronizeLocked() throws {
        guard FileManager.default.fileExists(atPath: indexURL.path) else {
            entries = entries.filter {
                !tombstones.contains($0.value.variantKey)
                    && FileManager.default.fileExists(atPath: chunkURL($0.key).path)
            }
            return
        }
        let decoded: MacStreamCacheIndex
        do {
            decoded = try JSONDecoder().decode(
                MacStreamCacheIndex.self, from: Data(contentsOf: indexURL))
        } catch {
            throw MacStreamFailure.frozen(stage: "cache", code: "cache_unavailable")
        }
        guard decoded.version == 1 else {
            throw MacStreamFailure.frozen(stage: "cache", code: "cache_unavailable")
        }
        tombstones.formUnion(decoded.tombstones.filter { validLowerSHA256($0) })
        var candidates: [String: MacStreamCacheEntry] = [:]
        for entry in decoded.entries {
            if let stored = candidates[entry.key], stored.lastUse >= entry.lastUse { continue }
            candidates[entry.key] = entry
        }
        for entry in entries.values {
            if let stored = candidates[entry.key], stored.lastUse > entry.lastUse {
                continue
            }
            candidates[entry.key] = entry
        }
        var refreshed: [String: MacStreamCacheEntry] = [:]
        for var entry in candidates.values {
            let url = chunkURL(entry.key)
            var isDirectory: ObjCBool = false
            let exists = FileManager.default.fileExists(atPath: url.path, isDirectory: &isDirectory)
            guard exists, !isDirectory.boolValue, validLowerSHA256(entry.key),
                  validLowerSHA256(entry.variantKey), entry.size > 0,
                  entry.size <= limits.chunkBytes, !tombstones.contains(entry.variantKey),
                  (try? url.resourceValues(forKeys: [.isSymbolicLinkKey]).isSymbolicLink) != true,
                  (try? url.resourceValues(forKeys: [.fileSizeKey]).fileSize) == Int(entry.size)
            else {
                try? FileManager.default.removeItem(at: url)
                continue
            }
            entry.pinned = entries[entry.key]?.pinned ?? false
            refreshed[entry.key] = entry
            clock = max(clock, entry.lastUse)
        }
        entries = refreshed
    }

    private func persistLocked() throws {
        try synchronizeLocked()
        for key in entries.keys where tombstones.contains(entries[key]!.variantKey) {
            removeEntry(key)
        }
        let index = MacStreamCacheIndex(
            entries: entries.values.sorted { $0.key < $1.key },
            tombstones: tombstones.sorted())
        try atomicWrite(try JSONEncoder().encode(index), to: indexURL)
    }

    private func atomicWrite(_ data: Data, to destination: URL) throws {
        let temporary = destination.deletingLastPathComponent()
            .appendingPathComponent(
                destination.lastPathComponent + ".\(UUID().uuidString).part")
        do {
            try data.write(to: temporary, options: .withoutOverwriting)
            let handle = try FileHandle(forWritingTo: temporary)
            try handle.synchronize()
            try handle.close()
            guard Darwin.rename(temporary.path, destination.path) == 0 else {
                throw MacStreamFailure.frozen(stage: "cache", code: "cache_unavailable")
            }
            try FileManager.default.setAttributes([.posixPermissions: 0o600], ofItemAtPath: destination.path)
            let directoryFD = Darwin.open(
                destination.deletingLastPathComponent().path, O_RDONLY | O_DIRECTORY)
            if directoryFD >= 0 {
                _ = Darwin.fsync(directoryFD)
                _ = Darwin.close(directoryFD)
            }
        } catch {
            try? FileManager.default.removeItem(at: temporary)
            throw MacStreamFailure.frozen(stage: "cache", code: "cache_unavailable")
        }
    }
}

private func chunksRange(_ chunks: [MacStreamChunk]) -> Range<Int> { 0..<chunks.count }
