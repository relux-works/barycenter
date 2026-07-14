// WebSocket client to the coordinator (spec 6.2 item 4, ch. 8):
// register on connect, protocol ping every 10 s (clock sync 8.5), state
// heartbeat every 5 s, reconnect with exponential backoff, incoming commands
// delivered to a handler. URLSessionWebSocketTask keeps us dependency-free.

import Foundation

public final class CoordinatorClient: NSObject {
    public struct Identity {
        public let nodeId: String
        public let token: String
        public let appVersion: String
        public let librespotVersion: String
        public init(nodeId: String, token: String, appVersion: String, librespotVersion: String) {
            self.nodeId = nodeId
            self.token = token
            self.appVersion = appVersion
            self.librespotVersion = librespotVersion
        }
    }

    private let url: URL
    private let identity: Identity
    private let capabilities: [String]
    private let log: Logger
    private let queue = DispatchQueue(label: "duet.coordinator-client")

    private var task: URLSessionWebSocketTask?
    private var session: URLSession!
    private var backoffSeconds: Double = 1
    private var stopped = false
    private var healthy = false

    private var pingTimer: DispatchSourceTimer?
    private var heartbeatTimer: DispatchSourceTimer?

    public private(set) var clock = ClockSync()

    /// Set by the owner: called on the client queue for every incoming message.
    public var onMessage: ((EnvelopeHead, Message) -> Void)?
    /// Called after register is sent on a fresh connection (spec 8.6: the node
    /// then receives welcome and reconciles).
    public var onConnected: (() -> Void)?
    /// Supplies the current state snapshot for the 5 s heartbeat (spec 8.4).
    public var stateProvider: (() -> StatePayload)?

    public init(
        url: URL,
        identity: Identity,
        capabilities: [String] = [seamlessAdoptionCapability],
        log: Logger
    ) {
        precondition(ProtocolCapabilities.areCanonical(capabilities),
                     "coordinator capabilities must be unique canonical ASCII")
        self.url = url
        self.identity = identity
        self.capabilities = capabilities
        self.log = log
        super.init()
        let cfg = URLSessionConfiguration.default
        cfg.timeoutIntervalForRequest = 10
        session = URLSession(configuration: cfg, delegate: nil, delegateQueue: nil)
    }

    public func start() {
        queue.async { self.connect() }
    }

    public func stop() {
        queue.async {
            self.stopped = true
            self.healthy = false
            self.pingTimer?.cancel()
            self.heartbeatTimer?.cancel()
            self.task?.cancel(with: .goingAway, reason: nil)
        }
    }

    private func connect() {
        guard !stopped else { return }
        healthy = false
        log.info("connecting to coordinator", ["origin": URLRedactor.originOnly(url)])
        let t = session.webSocketTask(with: url)
        task = t
        t.resume()
        receiveNext()
        sendMessageOnQueue(.register(RegisterPayload(
            nodeId: identity.nodeId,
            token: identity.token,
            appVersion: identity.appVersion,
            librespotVersion: identity.librespotVersion,
            capabilities: capabilities
        )))
        startTimers()
        onConnected?()
    }

    private func startTimers() {
        pingTimer?.cancel()
        let ping = DispatchSource.makeTimerSource(queue: queue)
        ping.schedule(deadline: .now() + 1, repeating: 10) // spec 8.5: every 10 s
        ping.setEventHandler { [weak self] in
            guard let self else { return }
            self.sendMessageOnQueue(.ping(PingPayload(t1: Self.nowMs())))
        }
        ping.resume()
        pingTimer = ping

        heartbeatTimer?.cancel()
        let hb = DispatchSource.makeTimerSource(queue: queue)
        hb.schedule(deadline: .now() + 2, repeating: 5) // spec 8.4: every 5 s
        hb.setEventHandler { [weak self] in
            guard let self, let provider = self.stateProvider else { return }
            self.sendMessageOnQueue(.state(provider()))
        }
        hb.resume()
        heartbeatTimer = hb
    }

    public static func nowMs() -> Int64 {
        Int64((Date().timeIntervalSince1970 * 1000).rounded())
    }

    public func sendMessage(_ message: Message) {
        // Every caller, including PlayerCore and async media workers, crosses
        // this non-blocking boundary before touching the WebSocket task.
        queue.async { self.sendMessageOnQueue(message) }
    }

    private func sendMessageOnQueue(_ message: Message) {
        let id = "msg_" + ULID.new()
        do {
            let data = try ProtocolCodec.encode(id: id, ts: Self.nowMs(), message: message)
            task?.send(.data(data)) { [weak self] error in
                if error != nil {
                    self?.log.warn("ws send failed", ["type": URLRedactor.safeProtocolType(message.typeName)])
                }
            }
            log.debug("sent", ["type": URLRedactor.safeProtocolType(message.typeName), "id": id])
        } catch {
            _ = error
            log.error("encode failed", ["type": URLRedactor.safeProtocolType(message.typeName)])
        }
    }

    private func receiveNext() {
        task?.receive { [weak self] result in
            guard let self else { return }
            self.queue.async {
                switch result {
                case .failure:
                    self.log.warn("ws receive failed, reconnecting")
                    self.scheduleReconnect()
                case .success(let wsMessage):
                    let t4 = Self.nowMs()
                    let data: Data
                    switch wsMessage {
                    case .data(let d): data = d
                    case .string(let s): data = Data(s.utf8)
                    @unknown default: data = Data()
                    }
                    self.handleIncoming(data, t4: t4)
                    self.receiveNext()
                }
            }
        }
    }

    private func handleIncoming(_ data: Data, t4: Int64) {
        let head: EnvelopeHead
        let message: Message
        do {
            (head, message) = try ProtocolCodec.decode(data)
        } catch ProtocolError.unknownType(let t) {
            log.warn("unknown message type ignored", ["type": URLRedactor.safeProtocolType(t)]) // spec 8.6
            return
        } catch {
            log.warn("bad frame")
            return
        }
        log.debug("received", ["type": URLRedactor.safeProtocolType(head.type), "id": head.id])

        if case .pong(let p) = message {
            let accepted = clock.addSample(t1: p.t1, t2: p.t2, t3: p.t3, t4: t4)
            log.debug("clock sample", [
                "rtt_ms": clock.lastRttMs,
                "offset_ms": clock.offsetMs.map { String(format: "%.1f", $0) } ?? "n/a",
                "accepted": accepted,
            ])
            return
        }
        onMessage?(head, message)
    }

    private func scheduleReconnect() {
        guard !stopped else { return }
        healthy = false
        pingTimer?.cancel()
        heartbeatTimer?.cancel()
        task?.cancel()
        task = nil
        let delay = backoffSeconds
        backoffSeconds = min(backoffSeconds * 2, 30)
        log.info("reconnect scheduled", ["after_s": delay])
        queue.asyncAfter(deadline: .now() + delay) { [weak self] in
            self?.connect()
        }
    }

    /// Reset backoff after a confirmed healthy exchange (welcome received).
    public func markHealthy() {
        queue.async {
            self.backoffSeconds = 1
            self.healthy = true
        }
    }

    /// Thread-safe UI/diagnostic projection. True only after an authenticated
    /// welcome on the current socket; reconnect immediately clears it.
    public var isHealthy: Bool { queue.sync { healthy } }

    /// Immutable snapshot for PlayerCore scheduling; avoids sharing mutable
    /// ClockSync storage across the coordinator and player queues.
    public func clockSnapshot() -> ClockSync {
        queue.sync { clock }
    }

    /// Test/diagnostic hook. Values are already public protocol names and do
    /// not expose identity or bearer material.
    public var registeredCapabilities: [String] { capabilities }
}

// Minimal ULID (Crockford base32, 48-bit ms time + 80-bit random).
public enum ULID {
    private static let alphabet = Array("0123456789ABCDEFGHJKMNPQRSTVWXYZ")

    public static func new(now: Date = Date()) -> String {
        var bytes = [UInt8](repeating: 0, count: 16)
        let ms = UInt64(now.timeIntervalSince1970 * 1000)
        for i in 0..<6 {
            bytes[i] = UInt8((ms >> (8 * UInt64(5 - i))) & 0xFF)
        }
        for i in 6..<16 {
            bytes[i] = UInt8.random(in: 0...255)
        }
        // Canonical ULID layout: 26 chars, the first encodes only the top
        // 3 bits (matches the coordinator's Go encoder, keeps ids sortable).
        var out = ""
        out.reserveCapacity(26)
        var acc: UInt32 = 0
        var bits: UInt32 = 2 // left-pad 128 bits to 130 so 26*5 aligns MSB-first
        for b in bytes {
            acc = (acc << 8) | UInt32(b)
            bits += 8
            while bits >= 5 {
                bits -= 5
                out.append(alphabet[Int((acc >> bits) & 31)])
            }
        }
        return out
    }
}
