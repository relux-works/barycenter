// Voice insert cache (spec 6.3 play_voice item 3): downloads WAVs from the
// coordinator's authed media endpoint, keeps an LRU of `capacity` files.

import Foundation

public final class VoiceCache {
    private let dir: URL
    private let token: String
    private let capacity: Int
    private let log: Logger
    private let session: URLSession
    private let queue = DispatchQueue(label: "duet.voice-cache")
    private var order: [String] = [] // most recent last

    public init(cacheDir: String, nodeToken: String, capacity: Int = 50, log: Logger) {
        dir = URL(fileURLWithPath: cacheDir, isDirectory: true).appendingPathComponent("voice", isDirectory: true)
        token = nodeToken
        self.capacity = capacity
        self.log = log
        session = URLSession(configuration: .default)
        try? FileManager.default.createDirectory(at: dir, withIntermediateDirectories: true)
    }

    static func mediaID(fromFileURL url: String) -> String {
        let last = url.split(separator: "/").last.map(String.init) ?? url
        return last.hasSuffix(".wav") ? String(last.dropLast(4)) : last
    }

    /// Returns a local file URL, downloading with the node token if not cached.
    public func fetch(fileURL: String) async throws -> URL {
        let id = Self.mediaID(fromFileURL: fileURL)
        let local = dir.appendingPathComponent(id + ".wav")
        if FileManager.default.fileExists(atPath: local.path) {
            touch(id)
            return local
        }
        guard let remote = URL(string: fileURL) else {
            throw NSError(domain: "voice-cache", code: 1,
                          userInfo: [NSLocalizedDescriptionKey: "bad media url \(fileURL)"])
        }
        var req = URLRequest(url: remote)
        req.setValue("Bearer \(token)", forHTTPHeaderField: "Authorization")
        let (tmp, response) = try await session.download(for: req)
        if let http = response as? HTTPURLResponse, http.statusCode != 200 {
            throw NSError(domain: "voice-cache", code: http.statusCode,
                          userInfo: [NSLocalizedDescriptionKey: "media download HTTP \(http.statusCode)"])
        }
        try? FileManager.default.removeItem(at: local)
        try FileManager.default.moveItem(at: tmp, to: local)
        touch(id)
        evictIfNeeded()
        return local
    }

    private func touch(_ id: String) {
        queue.sync {
            order.removeAll { $0 == id }
            order.append(id)
        }
    }

    private func evictIfNeeded() {
        queue.sync {
            while order.count > capacity {
                let victim = order.removeFirst()
                let path = dir.appendingPathComponent(victim + ".wav")
                try? FileManager.default.removeItem(at: path)
                log.debug("voice cache evicted", ["media_id": victim])
            }
        }
    }

    // Test hook: current LRU order.
    public var cachedIDs: [String] { queue.sync { order } }

    /// Test hook: register an existing local file as cached.
    public func seed(id: String, data: Data) throws {
        try data.write(to: dir.appendingPathComponent(id + ".wav"))
        touch(id)
        evictIfNeeded()
    }
}
