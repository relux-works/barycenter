// WebSocket client to the coordinator (spec 6.2 item 4, ch. 8):
// register on connect, protocol ping every 10 s (clock sync 8.5), state
// heartbeat every 5 s, reconnect with exponential backoff, incoming commands
// delivered to a handler.
//
// Transport is Network.framework (NWConnection), not URLSessionWebSocketTask:
// the coordinator sits behind a reverse proxy that advertises HTTP/2 in ALPN,
// and URLSessionWebSocketTask negotiates h2 there — the WebSocket upgrade then
// fails ("Socket is not connected") and the node never registers. NWConnection
// lets us pin ALPN to http/1.1, which the RFC 6455 upgrade requires and which
// URLSession offers no public way to force.

import Foundation
import Network

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

    private var conn: NWConnection?
    private var reconnectPending = false
    private var backoffSeconds: Double = 1
    private var stopped = false
    private var healthy = false
    private let liveSendLock = NSLock()
    private var liveSendQueued = 0
    private var liveTransportReady = false

    private var pingTimer: DispatchSourceTimer?
    private var heartbeatTimer: DispatchSourceTimer?

    public private(set) var clock = ClockSync()

    /// Set by the owner: called on the client queue for every incoming message.
    public var onMessage: ((EnvelopeHead, Message) -> Void)?
    /// Validated BP binary frames. Remains unused unless live_ptt_v1 is
    /// explicitly present in the frozen registration capability set.
    public var onLivePTTFrame: ((LivePTTBinaryFrame) -> Void)?
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
    }

    /// TLS + WebSocket parameters with ALPN pinned to http/1.1. Advertising only
    /// http/1.1 prevents the reverse proxy from selecting HTTP/2, under which the
    /// RFC 6455 upgrade cannot complete.
    private static func makeParameters() -> NWParameters {
        let tls = NWProtocolTLS.Options()
        sec_protocol_options_add_tls_application_protocol(
            tls.securityProtocolOptions, "http/1.1")
        let params = NWParameters(tls: tls)
        let ws = NWProtocolWebSocket.Options()
        ws.autoReplyPing = true
        params.defaultProtocolStack.applicationProtocols.insert(ws, at: 0)
        return params
    }

    public func start() {
        queue.async { self.connect() }
    }

    public func stop() {
        queue.async {
            self.stopped = true
            self.healthy = false
            self.setLiveTransportReady(false)
            self.pingTimer?.cancel()
            self.heartbeatTimer?.cancel()
            self.conn?.cancel()
            self.conn = nil
        }
    }

    private func connect() {
        guard !stopped else { return }
        healthy = false
        reconnectPending = false
        log.info("connecting to coordinator", ["origin": URLRedactor.originOnly(url)])
        let connection = NWConnection(to: .url(url), using: Self.makeParameters())
        conn = connection
        connection.stateUpdateHandler = { [weak self] state in
            // Delivered on `queue` (see start(queue:)), matching every other
            // mutation of this client's state.
            guard let self else { return }
            switch state {
            case .ready:
                self.onConnectionReady()
            case .failed(let error):
                self.log.warn("ws connection failed, reconnecting", ["err": "\(error)"])
                self.scheduleReconnect()
            case .waiting(let error):
                self.log.warn("ws connection waiting, reconnecting", ["err": "\(error)"])
                self.scheduleReconnect()
            case .cancelled:
                break
            default:
                break
            }
        }
        connection.start(queue: queue)
    }

    /// Fires once the WebSocket upgrade to /ws has completed. Registration and
    /// the ping/heartbeat timers only run on a live socket.
    private func onConnectionReady() {
        guard !stopped else { conn?.cancel(); return }
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
            let provided = provider()
            let state = CaptureQualityContract.heartbeatState(
                provided, advertisedCapabilities: self.capabilities)
            if provided.captureQuality != nil && state.captureQuality == nil {
                self.log.warn("capture quality heartbeat withheld until capability is advertised")
            }
            self.sendMessageOnQueue(.state(state))
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

    /// Non-blocking bounded binary seam used by the production-dark live node.
    /// It fails closed while the capability is absent, the authenticated socket
    /// is unhealthy, the frame is malformed, or eight sends are already in flight.
    public func trySendLivePTTFrame(_ frame: LivePTTBinaryFrame) -> Bool {
        guard capabilities.contains(livePTTCapability),
              let data = try? frame.encoded()
        else { return false }
        liveSendLock.lock()
        guard liveTransportReady, liveSendQueued < 8 else {
            liveSendLock.unlock()
            return false
        }
        liveSendQueued += 1
        liveSendLock.unlock()
        queue.async { [weak self] in
            guard let self, self.conn != nil, self.healthy else {
                self?.releaseLiveSend()
                return
            }
            self.send(data, opcode: .binary) { [weak self] failed in
                self?.releaseLiveSend()
                if failed {
                    self?.log.warn("live binary send failed")
                }
            }
        }
        return true
    }

    /// Sends one WebSocket frame with the given opcode. `completion` reports
    /// whether the send failed. Must be called on `queue`.
    ///
    /// Frame type is protocol-significant: the coordinator only accepts JSON
    /// control messages (register, ping, state, …) as TEXT frames and reserves
    /// BINARY exclusively for LivePTT audio — a control message sent as binary
    /// is silently dropped at register and in the command loop.
    private func send(
        _ data: Data, opcode: NWProtocolWebSocket.Opcode,
        completion: @escaping (Bool) -> Void
    ) {
        guard let connection = conn else { completion(true); return }
        let metadata = NWProtocolWebSocket.Metadata(opcode: opcode)
        let context = NWConnection.ContentContext(identifier: "ws", metadata: [metadata])
        connection.send(
            content: data, contentContext: context, isComplete: true,
            completion: .contentProcessed { error in completion(error != nil) })
    }

    private func sendMessageOnQueue(_ message: Message) {
        let id = "msg_" + ULID.new()
        do {
            let data = try ProtocolCodec.encode(id: id, ts: Self.nowMs(), message: message)
            send(data, opcode: .text) { [weak self] failed in
                if failed {
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
        conn?.receiveMessage { [weak self] content, context, _, error in
            guard let self else { return }
            // Delivered on `queue` (start(queue:)).
            if error != nil {
                self.log.warn("ws receive failed, reconnecting")
                self.scheduleReconnect()
                return
            }
            let opcode = (context?.protocolMetadata(definition: NWProtocolWebSocket.definition)
                as? NWProtocolWebSocket.Metadata)?.opcode
            switch opcode {
            case .some(.close), .none:
                // A close frame — or a message we cannot classify — means the
                // socket has ended; a live text/binary frame always carries
                // WebSocket metadata, so absent metadata is treated as closed.
                self.log.warn("ws closed by peer, reconnecting")
                self.scheduleReconnect()
            case .some(.text), .some(.binary):
                let data = content ?? Data()
                let t4 = Self.nowMs()
                if opcode == .some(.binary), Self.isLivePTTBinary(data) {
                    self.handleIncomingLivePTT(data)
                    self.receiveNext()
                    return
                }
                if self.handleIncoming(data, t4: t4) { self.receiveNext() }
            case .some(.ping), .some(.pong), .some(.cont):
                self.receiveNext()
            case .some:
                self.log.warn("unknown websocket frame ignored")
                self.receiveNext()
            }
        }
    }

    enum IncomingFrameDecision {
        case deliver(EnvelopeHead, Message)
        case ignoreUnknown(String)
        case reconnectVersion(Int)
        case ignoreMalformed
    }

    static func classifyIncoming(_ data: Data) -> IncomingFrameDecision {
        do {
            let (head, message) = try ProtocolCodec.decode(data)
            return .deliver(head, message)
        } catch ProtocolError.unknownType(let type) {
            return .ignoreUnknown(type)
        } catch ProtocolError.versionMismatch(let version) {
            return .reconnectVersion(version)
        } catch {
            return .ignoreMalformed
        }
    }

    private func handleIncoming(_ data: Data, t4: Int64) -> Bool {
        let head: EnvelopeHead
        let message: Message
        switch Self.classifyIncoming(data) {
        case .deliver(let decodedHead, let decodedMessage):
            (head, message) = (decodedHead, decodedMessage)
        case .ignoreUnknown(let type):
            log.warn("unknown message type ignored", ["type": URLRedactor.safeProtocolType(type)]) // spec 8.6
            return true
        case .reconnectVersion(let version):
            log.warn("protocol version mismatch; reconnecting", [
                "got": version, "want": ProtocolConstants.version,
            ])
            scheduleReconnect()
            return false
        case .ignoreMalformed:
            log.warn("bad frame")
            return true
        }
        log.debug("received", ["type": URLRedactor.safeProtocolType(head.type), "id": head.id])

        if case .pong(let p) = message {
            let accepted = clock.addSample(t1: p.t1, t2: p.t2, t3: p.t3, t4: t4)
            log.debug("clock sample", [
                "rtt_ms": clock.lastRttMs,
                "offset_ms": clock.offsetMs.map { String(format: "%.1f", $0) } ?? "n/a",
                "accepted": accepted,
            ])
            return true
        }
        onMessage?(head, message)
        return true
    }

    static func isLivePTTBinary(_ data: Data) -> Bool {
        data.count >= 2 && data[data.startIndex] == 0x42 &&
            data[data.index(after: data.startIndex)] == 0x50
    }

    private func handleIncomingLivePTT(_ data: Data) {
        guard capabilities.contains(livePTTCapability) else {
            log.warn("unadvertised live binary ignored")
            return
        }
        guard let frame = try? LivePTTBinaryFrame.decode(data) else {
            log.warn("malformed live binary ignored")
            return
        }
        onLivePTTFrame?(frame)
    }

    private func scheduleReconnect() {
        guard !stopped, !reconnectPending else { return }
        reconnectPending = true
        healthy = false
        setLiveTransportReady(false)
        pingTimer?.cancel()
        heartbeatTimer?.cancel()
        conn?.cancel()
        conn = nil
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
            self.setLiveTransportReady(true)
        }
    }

    private func setLiveTransportReady(_ value: Bool) {
        liveSendLock.lock()
        liveTransportReady = value && capabilities.contains(livePTTCapability)
        liveSendLock.unlock()
    }

    private func releaseLiveSend() {
        liveSendLock.lock()
        liveSendQueued = max(0, liveSendQueued - 1)
        liveSendLock.unlock()
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
