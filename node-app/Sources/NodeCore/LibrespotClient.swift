// Local go-librespot API client (spec 6.2 item 2, daemon 0.7.4 OpenAPI):
// HTTP commands + /events WebSocket. Tolerant decoding — unknown fields and
// events are ignored (exact live shapes confirmed in the spike remainder).

import Foundation

public struct LibrespotStatus: Decodable {
    public struct Track: Decodable {
        public var uri: String?
        public var name: String?
        public var position: Int64?
        public var duration: Int64?
    }
    public var username: String?
    public var stopped: Bool?
    public var paused: Bool?
    public var buffering: Bool?
    public var volume: Int?
    public var track: Track?
}

public enum LibrespotEvent {
    case active
    case inactive
    case willPlay(uri: String?)
    case playing(uri: String?)
    case notPlaying(uri: String?)
    case paused(uri: String?)
    case stopped
    case metadata(uri: String?, name: String?, position: Int64?, duration: Int64?)
    case seek(position: Int64?, duration: Int64?)
    case volume(value: Int?, max: Int?)
    case other(String)
}

public final class LibrespotClient {
    private let base: URL
    private let log: Logger
    private let session: URLSession
    private let queue = DispatchQueue(label: "duet.librespot-client")

    private var eventsTask: URLSessionWebSocketTask?
    private var eventsStopped = false

    public var onEvent: ((LibrespotEvent) -> Void)?

    public init(apiPort: Int, log: Logger) {
        base = URL(string: "http://127.0.0.1:\(apiPort)")!
        self.log = log
        let cfg = URLSessionConfiguration.default
        cfg.timeoutIntervalForRequest = 5
        session = URLSession(configuration: cfg)
    }

    // MARK: HTTP commands (daemon api-spec.yml)

    public func playbackReady() async -> Bool {
        struct Root: Decodable { var playbackReady: Bool?
            enum CodingKeys: String, CodingKey { case playbackReady = "playback_ready" } }
        guard let data = try? await get("/") else { return false }
        return (try? JSONDecoder().decode(Root.self, from: data))?.playbackReady ?? false
    }

    public func status() async throws -> LibrespotStatus {
        let data = try await get("/status")
        guard !data.isEmpty else {
            // Pre-login /status returns an empty body (spike, spec 6.2).
            return LibrespotStatus()
        }
        return try JSONDecoder().decode(LibrespotStatus.self, from: data)
    }

    /// Two-step load, part 1 (spec 6.3): play the uri already paused.
    public func playPaused(uri: String) async throws {
        try await post("/player/play", ["uri": uri, "paused": true])
    }

    public func seek(positionMS: Int64) async throws {
        try await post("/player/seek", ["position": positionMS, "relative": false])
    }

    public func resume() async throws { try await post("/player/resume", [:]) }
    public func pause() async throws { try await post("/player/pause", [:]) }
    public func stop() async throws { try await post("/player/stop", [:]) }
    public func next() async throws { try await post("/player/next", [:]) }

    public func addToQueue(uri: String) async throws {
        try await post("/player/add_to_queue", ["uri": uri])
    }

    private func get(_ path: String) async throws -> Data {
        let (data, response) = try await session.data(from: base.appendingPathComponent(path))
        try Self.checkHTTP(response, path: path)
        return data
    }

    private func post(_ path: String, _ body: [String: Any]) async throws {
        var req = URLRequest(url: base.appendingPathComponent(path))
        req.httpMethod = "POST"
        req.setValue("application/json", forHTTPHeaderField: "Content-Type")
        req.httpBody = try JSONSerialization.data(withJSONObject: body)
        let (_, response) = try await session.data(for: req)
        try Self.checkHTTP(response, path: path)
    }

    private static func checkHTTP(_ response: URLResponse, path: String) throws {
        guard let http = response as? HTTPURLResponse else { return }
        guard (200..<300).contains(http.statusCode) else {
            throw NSError(domain: "librespot", code: http.statusCode,
                          userInfo: [NSLocalizedDescriptionKey: "librespot \(path) -> HTTP \(http.statusCode)"])
        }
    }

    // MARK: /events WebSocket

    public func startEvents() {
        queue.async {
            self.eventsStopped = false
            self.connectEvents()
        }
    }

    public func stopEvents() {
        queue.async {
            self.eventsStopped = true
            self.eventsTask?.cancel()
            self.eventsTask = nil
        }
    }

    private func connectEvents() {
        guard !eventsStopped else { return }
        var comps = URLComponents(url: base.appendingPathComponent("/events"), resolvingAgainstBaseURL: false)!
        comps.scheme = "ws"
        let task = session.webSocketTask(with: comps.url!)
        eventsTask = task
        task.resume()
        receiveEvents()
    }

    private func receiveEvents() {
        eventsTask?.receive { [weak self] result in
            guard let self else { return }
            self.queue.async {
                switch result {
                case .failure:
                    guard !self.eventsStopped else { return }
                    // Daemon restarting: retry until the supervisor brings it back.
                    self.queue.asyncAfter(deadline: .now() + 1) { self.connectEvents() }
                case .success(let msg):
                    let data: Data
                    switch msg {
                    case .data(let d): data = d
                    case .string(let s): data = Data(s.utf8)
                    @unknown default: data = Data()
                    }
                    if let event = Self.parseEvent(data) {
                        self.onEvent?(event)
                    }
                    self.receiveEvents()
                }
            }
        }
    }

    static func parseEvent(_ data: Data) -> LibrespotEvent? {
        guard let obj = try? JSONSerialization.jsonObject(with: data) as? [String: Any],
              let type = obj["type"] as? String else { return nil }
        let d = obj["data"] as? [String: Any] ?? [:]
        let uri = d["uri"] as? String
        func int64(_ key: String) -> Int64? { (d[key] as? NSNumber)?.int64Value }

        switch type {
        case "active": return .active
        case "inactive": return .inactive
        case "will_play": return .willPlay(uri: uri)
        case "playing": return .playing(uri: uri)
        case "not_playing": return .notPlaying(uri: uri)
        case "paused": return .paused(uri: uri)
        case "stopped": return .stopped
        case "metadata":
            return .metadata(uri: uri, name: d["name"] as? String,
                             position: int64("position"), duration: int64("duration"))
        case "seek": return .seek(position: int64("position"), duration: int64("duration"))
        case "volume":
            return .volume(value: (d["value"] as? NSNumber)?.intValue,
                           max: (d["max"] as? NSNumber)?.intValue)
        default: return .other(type)
        }
    }
}
