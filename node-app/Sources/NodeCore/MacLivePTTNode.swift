import Foundation

protocol MacLiveCaptureSending: AnyObject {
    var onEvent: (@Sendable (MacLiveCaptureEvent) -> Void)? { get set }
    func currentPhase() -> MacLiveCapturePhase
    func localHoldBegan(
        source: MacLiveHoldSource,
        holdCapabilityAvailable: Bool,
        selectedDeviceID: String?
    ) -> UInt64?
    func localHoldHeartbeat(generation: UInt64)
    func acceptStart(
        _ payload: LivePTTStartPayload,
        localGeneration: UInt64,
        authorized: Bool
    ) async throws
    func localHoldEnded(generation: UInt64)
    func localStop()
    func handleSystemSleep()
    func handleSessionLock()
    func handleDisconnect()
    func handleCoordinatorCancel()
    func recheckPermission()
    func shutdown()
}

extension MacLiveCaptureSender: MacLiveCaptureSending {}

protocol MacLiveJitterReceiving: AnyObject {
    func start(_ payload: LivePTTStartPayload, authorized: Bool) -> Bool
    func receive(_ frame: LivePTTBinaryFrame) -> LivePTTFrameDecision
    func end(_ payload: LivePTTEndPayload)
    func cancel(_ payload: LivePTTCancelPayload)
    func revoke(reason: String)
    func snapshot() -> MacLiveJitterSnapshot
}

extension MacLiveJitterReceiver: MacLiveJitterReceiving {}

enum MacLivePTTNodeDirection: String, Equatable, Sendable {
    case idle, sending, receiving
}

enum MacLivePTTNodePhase: String, Equatable, Sendable {
    case idle, fallback, awaitingSession = "awaiting_session"
    case awaitingReceiver = "awaiting_receiver", capturing, buffering, playing
    case stopping, rejected, failed
}

struct MacLivePTTNodeStatus: Equatable, Sendable {
    var direction: MacLivePTTNodeDirection = .idle
    var phase: MacLivePTTNodePhase = .idle
    var sessionID: String?
    var generation: Int64?
    var acceptedReceivers = 0
    var rejectedReceivers = 0
    var lastError: String?
    var fallbackToClip = false
    var captureQuality: CaptureQualityState?
}

enum MacLivePTTIncomingDecision: Equatable, Sendable {
    case allow
    case reject(String)
}

/// Production-dark integration of the reviewed sender and receiver. Target
/// snapshot construction and incoming DND/policy decisions stay injected so
/// this class cannot broaden an audience or invent authorization locally.
final class MacLivePTTNode: @unchecked Sendable {
    typealias PrepareStart = @Sendable (UInt64, MacLiveHoldSource) -> LivePTTStartPayload?
    typealias AuthorizeIncoming = @Sendable (LivePTTStartPayload) -> MacLivePTTIncomingDecision

    private struct Outgoing: Sendable {
        var localGeneration: UInt64
        var payload: LivePTTStartPayload
        var captureStartRequested = false
        var accepted = 0
        var rejected = 0
    }

    private let sender: MacLiveCaptureSending
    private let receiver: MacLiveJitterReceiving
    private let featureEnabled: @Sendable () -> Bool
    private let prepareStart: PrepareStart
    private let authorizeIncoming: AuthorizeIncoming
    private let coordinatorNowMs: @Sendable () -> Int64
    private let send: @Sendable (Message) -> Void
    private let queue = DispatchQueue(label: "duet.mac-live-ptt-node")
    private let eventQueue = DispatchQueue(label: "duet.mac-live-ptt-node-events")
    private let statusHandlerLock = NSLock()
    private var outgoing: Outgoing?
    private var status = MacLivePTTNodeStatus()
    private var statusHandler: (@Sendable (MacLivePTTNodeStatus) -> Void)?
    var onStatus: (@Sendable (MacLivePTTNodeStatus) -> Void)? {
        get { statusHandlerLock.withLock { statusHandler } }
        set { statusHandlerLock.withLock { statusHandler = newValue } }
    }

    init(
        sender: MacLiveCaptureSending,
        receiver: MacLiveJitterReceiving,
        featureEnabled: @escaping @Sendable () -> Bool,
        prepareStart: @escaping PrepareStart,
        authorizeIncoming: @escaping AuthorizeIncoming,
        coordinatorNowMs: @escaping @Sendable () -> Int64,
        send: @escaping @Sendable (Message) -> Void
    ) {
        self.sender = sender
        self.receiver = receiver
        self.featureEnabled = featureEnabled
        self.prepareStart = prepareStart
        self.authorizeIncoming = authorizeIncoming
        self.coordinatorNowMs = coordinatorNowMs
        self.send = send
        sender.onEvent = { [weak self] event in self?.consumeSender(event) }
    }

    func snapshot() -> MacLivePTTNodeStatus { queue.sync { status } }

    @discardableResult
    func holdBegan(
        source: MacLiveHoldSource,
        holdAvailable: Bool,
        selectedDeviceID: String?
    ) -> UInt64? {
        queue.sync {
            guard outgoing == nil, sender.currentPhase() == .idle,
                  receiver.snapshot().phase == .idle
            else {
                updateStatus(.init(
                    direction: .idle, phase: .rejected, lastError: "busy"))
                return nil
            }
            let available = featureEnabled() && holdAvailable
            let generation = sender.localHoldBegan(
                source: source,
                holdCapabilityAvailable: available,
                selectedDeviceID: selectedDeviceID)
            if generation != nil {
                updateStatus(.init(
                    direction: .sending, phase: .awaitingSession,
                    lastError: nil, fallbackToClip: false))
            } else if !available {
                updateStatus(.init(
                    direction: .idle, phase: .fallback,
                    lastError: "hold_unavailable", fallbackToClip: true))
            }
            return generation
        }
    }

    func holdHeartbeat(generation: UInt64) {
        sender.localHoldHeartbeat(generation: generation)
    }

    func holdEnded(generation: UInt64) {
        sender.localHoldEnded(generation: generation)
    }

    func localStop() { sender.localStop() }

    func handle(_ message: Message) {
        queue.async { self.handleLocked(message) }
    }

    func handleBinary(_ data: Data) {
        guard let frame = try? LivePTTBinaryFrame.decode(data) else { return }
        handleFrame(frame)
    }

    func handleFrame(_ frame: LivePTTBinaryFrame) {
        queue.async {
            guard self.featureEnabled() else { return }
            let decision = self.receiver.receive(frame)
            guard decision == .apply else { return }
            let receiver = self.receiver.snapshot()
            self.updateStatus(.init(
                direction: .receiving,
                phase: receiver.phase == .playing ? .playing : .buffering,
                sessionID: receiver.sessionId,
                generation: receiver.generation))
        }
    }

    func handleSystemSleep() {
        sender.handleSystemSleep()
        receiver.revoke(reason: "system_sleep")
        resetStatus(error: "system_sleep")
    }

    func handleSessionLock() {
        sender.handleSessionLock()
        receiver.revoke(reason: "session_locked")
        resetStatus(error: "session_locked")
    }

    func handleDisconnect() {
        sender.handleDisconnect()
        receiver.revoke(reason: "disconnect")
        resetStatus(error: "disconnect")
    }

    func recheckPermission() { sender.recheckPermission() }

    func rollbackFeature() {
        sender.handleCoordinatorCancel()
        receiver.revoke(reason: "feature_rollback")
        resetStatus(error: "feature_rollback")
    }

    func shutdown() {
        sender.shutdown()
        receiver.revoke(reason: "app_quit")
        resetStatus(error: nil)
    }

    private func consumeSender(_ event: MacLiveCaptureEvent) {
        queue.async {
            switch event {
            case .requestStart(let localGeneration, let source):
                self.requestStartLocked(localGeneration: localGeneration, source: source)
            case .phase(let phase):
                guard self.outgoing != nil else { return }
                let mapped: MacLivePTTNodePhase = switch phase {
                case .idle: .idle
                case .awaitingStart: .awaitingSession
                case .requestingPermission: .awaitingReceiver
                case .capturing: .capturing
                case .stopping: .stopping
                }
                self.status.phase = mapped
                self.publishStatus()
            case .fallbackToClip:
                self.updateStatus(.init(
                    direction: .idle, phase: .fallback,
                    lastError: "hold_unavailable", fallbackToClip: true))
            case .terminal(let reason):
                self.outgoing = nil
                self.updateStatus(.init(
                    direction: .idle, phase: .idle,
                    lastError: reason == .released ? nil : reason.rawValue))
            case .failed(let code):
                self.status.phase = .failed
                self.status.lastError = code
                self.publishStatus()
            case .quality(let state):
                self.status.captureQuality = state
                self.publishStatus()
            case .meter, .playStartCue, .playStopCue:
                break
            }
        }
    }

    private func requestStartLocked(localGeneration: UInt64, source: MacLiveHoldSource) {
        guard featureEnabled(), outgoing == nil,
              let payload = prepareStart(localGeneration, source),
              (try? LivePTTValidation.validate(.livePTTStart(payload))) != nil
        else {
            sender.localStop()
            updateStatus(.init(
                direction: .idle, phase: .failed,
                lastError: "target_or_policy_unavailable"))
            return
        }
        outgoing = Outgoing(localGeneration: localGeneration, payload: payload)
        updateStatus(.init(
            direction: .sending, phase: .awaitingReceiver,
            sessionID: payload.sessionId, generation: payload.generation))
        send(.livePTTStart(payload))
    }

    private func handleLocked(_ message: Message) {
        switch message {
        case .livePTTStart(let payload):
            handleIncomingStartLocked(payload)
        case .livePTTAccept(let payload):
            handleAcceptLocked(payload)
        case .livePTTReject(let payload):
            guard var active = matchingOutgoing(payload.sessionId, payload.generation) else { return }
            active.rejected += 1
            outgoing = active
            status.rejectedReceivers = active.rejected
            status.lastError = payload.code
            publishStatus()
        case .livePTTEnd(let payload):
            receiver.end(payload)
            updateReceiverStatus()
        case .livePTTCancel(let payload):
            if matchingOutgoing(payload.sessionId, payload.generation) != nil {
                sender.handleCoordinatorCancel()
            }
            receiver.cancel(payload)
            updateReceiverStatus()
        case .livePTTFailed(let payload):
            if matchingOutgoing(payload.sessionId, payload.generation) != nil {
                sender.handleCoordinatorCancel()
                status.phase = .failed
                status.lastError = payload.code
                publishStatus()
            } else if receiver.snapshot().sessionId == payload.sessionId {
                receiver.revoke(reason: payload.code)
                updateReceiverStatus(error: payload.code)
            }
        case .livePTTReceipt(let payload):
            guard matchingOutgoing(payload.sessionId, payload.generation) != nil else { return }
            if payload.state == "failed" || payload.state == "cancelled" {
                status.lastError = payload.state
            }
            publishStatus()
        case .livePTTState(let payload):
            if payload.phase == "idle" || payload.phase == "cancelled" || payload.phase == "terminal" {
                if let active = outgoing,
                   payload.activeSessionId == nil || payload.activeSessionId == active.payload.sessionId {
                    sender.handleCoordinatorCancel()
                }
            }
        default:
            break
        }
    }

    private func handleIncomingStartLocked(_ payload: LivePTTStartPayload) {
        guard featureEnabled() else {
            reject(payload, code: "unsupported")
            return
        }
        guard outgoing == nil, sender.currentPhase() == .idle,
              receiver.snapshot().phase == .idle
        else {
            reject(payload, code: "busy")
            return
        }
        switch authorizeIncoming(payload) {
        case .allow:
            if receiver.start(payload, authorized: true) {
                updateStatus(.init(
                    direction: .receiving, phase: .buffering,
                    sessionID: payload.sessionId, generation: payload.generation))
            }
        case .reject(let code):
            reject(payload, code: Self.allowedRejectCode(code))
        }
    }

    private func handleAcceptLocked(_ payload: LivePTTAcceptPayload) {
        guard var active = matchingOutgoing(payload.sessionId, payload.generation),
              (try? LivePTTValidation.validate(.livePTTAccept(payload))) != nil
        else { return }
        let shouldStartCapture = !active.captureStartRequested
        active.captureStartRequested = true
        active.accepted += 1
        outgoing = active
        status.acceptedReceivers = active.accepted
        publishStatus()
        guard shouldStartCapture else { return }
        Task { [weak self] in
            guard let self else { return }
            do {
                try await self.sender.acceptStart(
                    active.payload,
                    localGeneration: active.localGeneration,
                    authorized: true)
            } catch {
                self.queue.async {
                    guard self.matchingOutgoing(
                        active.payload.sessionId, active.payload.generation) != nil
                    else { return }
                    self.sender.handleCoordinatorCancel()
                    self.status.phase = .failed
                    self.status.lastError = "capture_start_failed"
                    self.publishStatus()
                }
            }
        }
    }

    private func matchingOutgoing(_ sessionID: String, _ generation: Int64) -> Outgoing? {
        guard let outgoing,
              outgoing.payload.sessionId == sessionID,
              outgoing.payload.generation == generation
        else { return nil }
        return outgoing
    }

    private func reject(_ payload: LivePTTStartPayload, code: String) {
        let rejection = LivePTTRejectPayload(
            sessionId: payload.sessionId,
            generation: payload.generation,
            eventSequence: 1,
            code: code,
            rejectedAtCoordMs: max(1, coordinatorNowMs()))
        guard (try? LivePTTValidation.validate(.livePTTReject(rejection))) != nil else { return }
        send(.livePTTReject(rejection))
    }

    private static func allowedRejectCode(_ value: String) -> String {
        ["blocked", "busy", "dnd", "expired", "policy", "unauthorized", "unsupported"]
            .contains(value) ? value : "policy"
    }

    private func updateReceiverStatus(error: String? = nil) {
        let receiver = receiver.snapshot()
        guard receiver.phase != .idle else {
            updateStatus(.init(
                direction: .idle, phase: error == nil ? .idle : .failed,
                lastError: error))
            return
        }
        updateStatus(.init(
            direction: .receiving,
            phase: receiver.phase == .playing ? .playing :
                receiver.phase == .draining ? .stopping : .buffering,
            sessionID: receiver.sessionId,
            generation: receiver.generation,
            lastError: error))
    }

    private func resetStatus(error: String?) {
        queue.async {
            self.outgoing = nil
            self.updateStatus(.init(
                direction: .idle, phase: error == nil ? .idle : .failed,
                lastError: error))
        }
    }

    private func updateStatus(_ value: MacLivePTTNodeStatus) {
        status = value
        publishStatus()
    }

    private func publishStatus() {
        let value = status
        let handler = onStatus
        eventQueue.async { handler?(value) }
    }
}
