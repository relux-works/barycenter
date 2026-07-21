import Foundation
import NodeAppUI
import NodeCore

public protocol DeviceInvitationServicing: Sendable {
    func probeControlContext() async throws -> ActorContextProbe
    func issueDeviceInvite(intendedRole: CredentialRole) async throws -> DeviceInvite
}

extension OnboardingService: DeviceInvitationServicing {}

public protocol DeviceInvitationClipboard: Sendable {
    func copy(_ code: String, timeToLive: Duration) async throws
    func clear() async throws
    func clearSynchronouslyForTermination() throws
    func statusUpdates() async -> AsyncStream<DeviceInvitationPasteboardCleanupStatus>
}

extension DeviceInvitationClipboard {
    public func clearSynchronouslyForTermination() throws {}
}

public struct SystemDeviceInvitationClipboard: DeviceInvitationClipboard, Sendable {
    private let lease: DeviceInvitationPasteboardLease

    public init(lease: DeviceInvitationPasteboardLease = .init()) {
        self.lease = lease
    }

    public func copy(_ code: String, timeToLive: Duration) async throws {
        try await lease.copyExplicitly(code, timeToLive: timeToLive)
    }

    public func clear() async throws {
        try await lease.clearExplicitly()
    }

    public func clearSynchronouslyForTermination() throws {
        try lease.clearSynchronouslyForTermination()
    }

    public func statusUpdates() async -> AsyncStream<DeviceInvitationPasteboardCleanupStatus> {
        await lease.cleanupStatusUpdates()
    }
}

/// Owns the live-only device invitation lifecycle. The durable-safe UI
/// snapshot contains authorization, role, expiry, and generic feedback only;
/// the plaintext code never leaves the transient model and conditional
/// pasteboard lease.
@MainActor
public final class MacDeviceInvitationAppComposition {
    public static let maximumClipboardLeaseSeconds = 120.0

    private let service: any DeviceInvitationServicing
    private let clipboard: any DeviceInvitationClipboard
    private let model: PulsarDeviceInvitationModel
    private let now: @Sendable () -> Date
    private let sleepUntil: @Sendable (Date) async throws -> Void

    private var authorizationTask: Task<Void, Never>?
    private var invitationTask: Task<Void, Never>?
    private var copyTask: Task<Void, Never>?
    private var expiryTask: Task<Void, Never>?
    private var cleanupObserverTask: Task<Void, Never>?
    private var clipboardClearTask: Task<Void, Never>?
    private var epoch: UInt64 = 0
    private var stopped = false

    public convenience init(
        coordinator: String,
        model: PulsarDeviceInvitationModel
    ) throws {
        let service = OnboardingService(
            client: try OnboardingHTTPClient(coordinator: coordinator))
        self.init(
            service: service,
            clipboard: SystemDeviceInvitationClipboard(),
            model: model,
            now: { Date() },
            sleepUntil: { deadline in
                let milliseconds = max(
                    1,
                    Int64((deadline.timeIntervalSinceNow * 1_000).rounded(.up)))
                try await Task.sleep(for: .milliseconds(milliseconds))
            })
    }

    init(
        service: any DeviceInvitationServicing,
        clipboard: any DeviceInvitationClipboard,
        model: PulsarDeviceInvitationModel,
        now: @escaping @Sendable () -> Date,
        sleepUntil: @escaping @Sendable (Date) async throws -> Void
    ) {
        self.service = service
        self.clipboard = clipboard
        self.model = model
        self.now = now
        self.sleepUntil = sleepUntil
    }

    public func start() {
        guard !stopped else { return }
        observeClipboardCleanup()
        refreshAuthorization()
    }

    public func refreshAuthorization() {
        guard !stopped else { return }
        cancelSensitiveWork(clearClipboard: true)
        authorizationTask?.cancel()
        model.beginAuthorization()
        let operationEpoch = nextEpoch()
        authorizationTask = Task { [weak self] in
            guard let self else { return }
            do {
                let context = try await service.probeControlContext()
                guard isActive(operationEpoch) else { return }
                switch context {
                case .active(let active) where active.role == .primary:
                    model.authorizePrimary()
                case .active:
                    model.denyAuthorization(.primaryRequired)
                case .limited:
                    model.denyAuthorization(.insufficientCapability)
                case .unauthorized:
                    model.denyAuthorization(.unauthorized)
                case .rateLimited(let seconds):
                    model.denyAuthorization(.rateLimited(seconds: seconds))
                }
            } catch {
                guard isActive(operationEpoch) else { return }
                model.denyAuthorization(Self.failure(for: error, authorizing: true))
            }
        }
    }

    public func generate() {
        guard !stopped, model.beginGeneration() else { return }
        invitationTask?.cancel()
        let operationEpoch = nextEpoch()
        invitationTask = Task { [weak self] in
            guard let self else { return }
            do {
                let invitation = try await service.issueDeviceInvite(intendedRole: .companion)
                guard isActive(operationEpoch) else { return }
                guard invitation.intendedRole == .companion,
                    invitation.expiresAt > now()
                else {
                    model.generationFailed(.invalidResponse)
                    return
                }
                model.show(
                    code: invitation.code,
                    role: .companion,
                    expiresAt: invitation.expiresAt)
                scheduleExpiry(at: invitation.expiresAt, epoch: operationEpoch)
            } catch {
                guard isActive(operationEpoch) else { return }
                model.generationFailed(Self.failure(for: error, authorizing: false))
            }
        }
    }

    public func copy() {
        guard !stopped,
            let invitation = model.snapshot.invitation
        else { return }
        let operationEpoch = epoch
        let precedingClear = clipboardClearTask
        copyTask?.cancel()
        copyTask = Task { [weak self] in
            guard let self else { return }
            await precedingClear?.value
            guard isActive(operationEpoch), model.snapshot.invitation == invitation,
                let code = model.visibleCode
            else { return }
            let current = now()
            let remaining = invitation.expiresAt.timeIntervalSince(current)
            guard remaining > 0 else {
                expireCurrentInvitation()
                return
            }
            let leaseSeconds = min(Self.maximumClipboardLeaseSeconds, remaining)
            let leaseMilliseconds = max(1, Int64((leaseSeconds * 1_000).rounded(.down)))
            let clearAt = current.addingTimeInterval(Double(leaseMilliseconds) / 1_000)
            do {
                try await clipboard.copy(
                    code,
                    timeToLive: .milliseconds(leaseMilliseconds))
                guard isActive(operationEpoch), model.snapshot.invitation == invitation else {
                    try? await clipboard.clear()
                    return
                }
                model.markCopied(autoClearAt: clearAt)
            } catch {
                if !isActive(operationEpoch) {
                    try? await clipboard.clear()
                    return
                }
                model.copyFailed()
            }
        }
    }

    public func hide() {
        guard !stopped else { return }
        let hadSensitiveWork = model.snapshot.invitation != nil || model.snapshot.isGenerating
        cancelSensitiveWork(clearClipboard: true)
        if hadSensitiveWork {
            model.hide(feedback: .hidden)
        }
    }

    public func shutdown() {
        guard !stopped else { return }
        stopped = true
        _ = nextEpoch()
        authorizationTask?.cancel()
        invitationTask?.cancel()
        copyTask?.cancel()
        expiryTask?.cancel()
        cleanupObserverTask?.cancel()
        authorizationTask = nil
        invitationTask = nil
        copyTask = nil
        expiryTask = nil
        cleanupObserverTask = nil
        model.reset()
        try? clipboard.clearSynchronouslyForTermination()
        let clipboard = clipboard
        Task { try? await clipboard.clear() }
    }

    private func observeClipboardCleanup() {
        guard cleanupObserverTask == nil else { return }
        cleanupObserverTask = Task { [weak self, clipboard] in
            let updates = await clipboard.statusUpdates()
            for await status in updates {
                guard let self, !Task.isCancelled, !stopped else { return }
                if status == .automaticCleanupFailed {
                    model.copyFailed(.cleanupFailed)
                }
            }
        }
    }

    private func scheduleExpiry(at expiresAt: Date, epoch operationEpoch: UInt64) {
        expiryTask?.cancel()
        expiryTask = Task { [weak self, sleepUntil] in
            do { try await sleepUntil(expiresAt) } catch { return }
            guard let self, isActive(operationEpoch) else { return }
            expireCurrentInvitation()
        }
    }

    private func expireCurrentInvitation() {
        guard model.snapshot.invitation != nil else { return }
        _ = nextEpoch()
        invitationTask?.cancel()
        copyTask?.cancel()
        expiryTask?.cancel()
        invitationTask = nil
        copyTask = nil
        expiryTask = nil
        model.hide(feedback: .expired)
        clearClipboardReportingFailure()
    }

    private func cancelSensitiveWork(clearClipboard: Bool) {
        _ = nextEpoch()
        invitationTask?.cancel()
        copyTask?.cancel()
        expiryTask?.cancel()
        invitationTask = nil
        copyTask = nil
        expiryTask = nil
        if clearClipboard { clearClipboardReportingFailure() }
    }

    private func clearClipboardReportingFailure() {
        let clipboard = clipboard
        let precedingClear = clipboardClearTask
        clipboardClearTask = Task { [weak self] in
            await precedingClear?.value
            do {
                try await clipboard.clear()
            } catch {
                guard let self, !stopped else { return }
                if model.snapshot.invitation == nil {
                    model.hide(feedback: .failure(.cleanupFailed))
                } else {
                    model.copyFailed(.cleanupFailed)
                }
            }
        }
    }

    private func nextEpoch() -> UInt64 {
        epoch &+= 1
        return epoch
    }

    private func isActive(_ operationEpoch: UInt64) -> Bool {
        !stopped && !Task.isCancelled && epoch == operationEpoch
    }

    private static func failure(
        for error: Error,
        authorizing: Bool
    ) -> PulsarDeviceInvitationFailure {
        guard let error = error as? OnboardingClientError else {
            return .serviceUnavailable
        }
        switch error {
        case .storage:
            return authorizing ? .notActivated : .serviceUnavailable
        case .api(_, .unauthorized, _):
            return .unauthorized
        case .api(_, .insufficientCapability, _):
            return .insufficientCapability
        case .api(_, .tooManyAttempts, let retry):
            return .rateLimited(seconds: max(1, retry ?? 1))
        case .invalidRequest, .invalidResponse, .responseTooLarge, .redirectRejected:
            return .invalidResponse
        case .invalidOrigin, .insecureTransport, .transport, .cancelled,
            .api:
            return .serviceUnavailable
        }
    }
}
