import Foundation
import Testing

@testable import NodeAppComposition
@testable import NodeAppUI
@testable import NodeCore

@Suite("macOS device invitation lifecycle", .serialized)
struct MacDeviceInvitationAppCompositionTests {
    private let code = "ABCDEFGHJKMNPQRSTVWXYZ23456"
    private let now = Date(timeIntervalSince1970: 1_800_000_000)

    @MainActor
    @Test("Authorized primary generates exactly one companion invitation at a time")
    func authorizedPrimarySingleGeneration() async {
        let service = InvitationServiceStub(
            probe: .active(.init(orbitId: 1, actorId: 2, role: .primary)),
            invitation: invitation())
        let clipboard = InvitationClipboardStub()
        let model = PulsarDeviceInvitationModel()
        let composition = makeComposition(
            service: service,
            clipboard: clipboard,
            model: model)

        composition.start()
        #expect(await eventually { model.snapshot.authorization == .authorizedPrimary })
        await clipboard.waitForClears(1)
        composition.generate()
        #expect(await eventually { model.visibleCode == self.code })
        #expect(model.snapshot.invitation?.role == .companion)
        #expect(await service.issuedRoles() == [.companion])

        composition.generate()
        for _ in 0..<20 { await Task.yield() }
        #expect(await service.issuedRoles() == [.companion])
        composition.shutdown()
    }

    @MainActor
    @Test("Non-primary context is denied before issuing an invitation")
    func primaryAuthorizationRequired() async {
        let service = InvitationServiceStub(
            probe: .active(.init(orbitId: 1, actorId: 3, role: .companion)),
            invitation: invitation())
        let model = PulsarDeviceInvitationModel()
        let composition = makeComposition(
            service: service,
            clipboard: InvitationClipboardStub(),
            model: model)

        composition.start()
        #expect(
            await eventually {
                model.snapshot.authorization == .unavailable(.primaryRequired)
            })
        composition.generate()
        for _ in 0..<20 { await Task.yield() }
        #expect(await service.issuedRoles().isEmpty)
        #expect(model.visibleCode == nil)
        composition.shutdown()
    }

    @MainActor
    @Test("Copy is deliberate, bounded, and hide clears transient and clipboard state")
    func explicitCopyAndHide() async {
        let service = InvitationServiceStub(
            probe: .active(.init(orbitId: 1, actorId: 2, role: .primary)),
            invitation: invitation(expiresAfter: 600))
        let clipboard = InvitationClipboardStub()
        let model = PulsarDeviceInvitationModel()
        let composition = makeComposition(
            service: service,
            clipboard: clipboard,
            model: model)

        composition.start()
        #expect(await eventually { model.snapshot.authorization == .authorizedPrimary })
        composition.generate()
        #expect(await eventually { model.visibleCode == self.code })
        #expect(await clipboard.copiedCodes().isEmpty)

        composition.copy()
        await clipboard.waitForCopies(1)
        #expect(await clipboard.copiedCodes() == [code])
        #expect(await clipboard.leaseDurations() == [.seconds(120)])
        #expect(
            await eventually {
                if case .copied = model.snapshot.feedback { return true }
                return false
            })

        composition.hide()
        await clipboard.waitForClears(2)
        #expect(model.visibleCode == nil)
        #expect(model.snapshot.invitation == nil)
        #expect(model.snapshot.feedback == .hidden)
        composition.shutdown()
    }

    @MainActor
    @Test("Hide cancels a late non-cooperative generation response")
    func cancellationRejectsLateMaterial() async {
        let service = InvitationServiceStub(
            probe: .active(.init(orbitId: 1, actorId: 2, role: .primary)),
            invitation: invitation(),
            suspendInvitation: true)
        let model = PulsarDeviceInvitationModel()
        let composition = makeComposition(
            service: service,
            clipboard: InvitationClipboardStub(),
            model: model)

        composition.start()
        #expect(await eventually { model.snapshot.authorization == .authorizedPrimary })
        composition.generate()
        await service.waitForIssueStart()
        composition.hide()
        await service.releaseInvitation()
        for _ in 0..<40 { await Task.yield() }

        #expect(model.visibleCode == nil)
        #expect(model.snapshot.invitation == nil)
        #expect(model.snapshot.feedback == .hidden)
        composition.shutdown()
    }

    @MainActor
    @Test("A delayed old cleanup cannot clear a newer invitation copy")
    func oldCleanupPrecedesNewCopy() async {
        let service = InvitationServiceStub(
            probe: .active(.init(orbitId: 1, actorId: 2, role: .primary)),
            invitation: invitation())
        let clipboard = InvitationClipboardStub()
        let model = PulsarDeviceInvitationModel()
        let composition = makeComposition(
            service: service,
            clipboard: clipboard,
            model: model)

        composition.start()
        #expect(await eventually { model.snapshot.authorization == .authorizedPrimary })
        await clipboard.waitForClears(1)
        composition.generate()
        #expect(await eventually { model.visibleCode == self.code })

        await clipboard.suspendNextClear()
        composition.hide()
        await clipboard.waitForSuspendedClear()
        composition.generate()
        #expect(await eventually { model.visibleCode == self.code })

        composition.copy()
        for _ in 0..<20 { await Task.yield() }
        #expect(await clipboard.copiedCodes().isEmpty)

        await clipboard.releaseSuspendedClear()
        await clipboard.waitForCopies(1)
        #expect(await clipboard.currentPayload() == code)
        #expect(await service.issuedRoles() == [.companion, .companion])
        composition.shutdown()
    }

    @MainActor
    @Test("Expiry hides material and a fresh composition cannot reuse it")
    func expiryAndRelaunch() async {
        let service = InvitationServiceStub(
            probe: .active(.init(orbitId: 1, actorId: 2, role: .primary)),
            invitation: invitation())
        let clipboard = InvitationClipboardStub()
        let expiry = InvitationExpiryGate()
        let firstModel = PulsarDeviceInvitationModel()
        let first = MacDeviceInvitationAppComposition(
            service: service,
            clipboard: clipboard,
            model: firstModel,
            now: { self.now },
            sleepUntil: { _ in try await expiry.sleep() })

        first.start()
        #expect(await eventually { firstModel.snapshot.authorization == .authorizedPrimary })
        await clipboard.waitForClears(1)
        first.generate()
        #expect(await eventually { firstModel.visibleCode == self.code })
        await expiry.waitUntilSleeping()
        await expiry.release()
        #expect(await eventually { firstModel.snapshot.feedback == .expired })
        #expect(firstModel.visibleCode == nil)
        await clipboard.waitForClears(2)
        first.shutdown()

        let secondModel = PulsarDeviceInvitationModel()
        let second = makeComposition(
            service: service,
            clipboard: clipboard,
            model: secondModel)
        second.start()
        #expect(await eventually { secondModel.snapshot.authorization == .authorizedPrimary })
        #expect(secondModel.visibleCode == nil)
        #expect(secondModel.snapshot.invitation == nil)
        #expect(await service.issuedRoles().count == 1)
        second.shutdown()
    }

    @MainActor
    @Test("Rate limits and service errors become typed presentation failures")
    func typedFailures() async {
        let rateLimited = InvitationServiceStub(
            probeError: .api(
                status: 429,
                code: .tooManyAttempts,
                retryAfterSeconds: 17),
            invitation: invitation())
        let model = PulsarDeviceInvitationModel()
        let composition = makeComposition(
            service: rateLimited,
            clipboard: InvitationClipboardStub(),
            model: model)
        composition.start()
        #expect(
            await eventually {
                model.snapshot.authorization == .unavailable(.rateLimited(seconds: 17))
            })
        composition.shutdown()
    }

    @MainActor
    private func makeComposition(
        service: InvitationServiceStub,
        clipboard: InvitationClipboardStub,
        model: PulsarDeviceInvitationModel
    ) -> MacDeviceInvitationAppComposition {
        MacDeviceInvitationAppComposition(
            service: service,
            clipboard: clipboard,
            model: model,
            now: { self.now },
            sleepUntil: { _ in try await Task.sleep(for: .seconds(3_600)) })
    }

    private func invitation(expiresAfter seconds: TimeInterval = 300) -> DeviceInvite {
        DeviceInvite(
            code: code,
            intendedRole: .companion,
            expiresAt: now.addingTimeInterval(seconds))
    }
}

@MainActor
private func eventually(
    attempts: Int = 500,
    _ condition: @MainActor () -> Bool
) async -> Bool {
    for _ in 0..<attempts {
        if condition() { return true }
        await Task.yield()
    }
    return condition()
}

private actor InvitationServiceStub: DeviceInvitationServicing {
    private let probe: ActorContextProbe?
    private let probeError: OnboardingClientError?
    private let invitation: DeviceInvite
    private let suspendInvitation: Bool
    private var roles: [CredentialRole] = []
    private var invitationContinuation: CheckedContinuation<DeviceInvite, Error>?
    private var startWaiters: [CheckedContinuation<Void, Never>] = []

    init(
        probe: ActorContextProbe,
        invitation: DeviceInvite,
        suspendInvitation: Bool = false
    ) {
        self.probe = probe
        probeError = nil
        self.invitation = invitation
        self.suspendInvitation = suspendInvitation
    }

    init(probeError: OnboardingClientError, invitation: DeviceInvite) {
        probe = nil
        self.probeError = probeError
        self.invitation = invitation
        suspendInvitation = false
    }

    func probeControlContext() async throws -> ActorContextProbe {
        if let probeError { throw probeError }
        return probe!
    }

    func issueDeviceInvite(intendedRole: CredentialRole) async throws -> DeviceInvite {
        roles.append(intendedRole)
        let waiters = startWaiters
        startWaiters.removeAll()
        for waiter in waiters { waiter.resume() }
        guard suspendInvitation else { return invitation }
        return try await withCheckedThrowingContinuation {
            invitationContinuation = $0
        }
    }

    func issuedRoles() -> [CredentialRole] { roles }

    func waitForIssueStart() async {
        guard roles.isEmpty else { return }
        await withCheckedContinuation { startWaiters.append($0) }
    }

    func releaseInvitation() {
        invitationContinuation?.resume(returning: invitation)
        invitationContinuation = nil
    }
}

private actor InvitationClipboardStub: DeviceInvitationClipboard {
    private var codes: [String] = []
    private var durations: [Duration] = []
    private var clears = 0
    private var payload: String?
    private var suspendNextClearFlag = false
    private var suspendedClearEntered = false
    private var suspendedClearContinuation: CheckedContinuation<Void, Never>?
    private var suspendedClearWaiters: [CheckedContinuation<Void, Never>] = []
    private var copyWaiters: [(Int, CheckedContinuation<Void, Never>)] = []
    private var clearWaiters: [(Int, CheckedContinuation<Void, Never>)] = []

    func copy(_ code: String, timeToLive: Duration) async throws {
        codes.append(code)
        durations.append(timeToLive)
        payload = code
        let waiters = copyWaiters.filter { codes.count >= $0.0 }
        copyWaiters.removeAll { codes.count >= $0.0 }
        for waiter in waiters { waiter.1.resume() }
    }

    func clear() async throws {
        if suspendNextClearFlag {
            suspendNextClearFlag = false
            suspendedClearEntered = true
            let waiters = suspendedClearWaiters
            suspendedClearWaiters.removeAll()
            for waiter in waiters { waiter.resume() }
            await withCheckedContinuation { suspendedClearContinuation = $0 }
        }
        clears += 1
        payload = nil
        let waiters = clearWaiters.filter { clears >= $0.0 }
        clearWaiters.removeAll { clears >= $0.0 }
        for waiter in waiters { waiter.1.resume() }
    }

    func statusUpdates() async -> AsyncStream<DeviceInvitationPasteboardCleanupStatus> {
        AsyncStream { continuation in continuation.yield(.idle) }
    }

    func copiedCodes() -> [String] { codes }
    func leaseDurations() -> [Duration] { durations }
    func clearCount() -> Int { clears }
    func currentPayload() -> String? { payload }

    func suspendNextClear() {
        suspendNextClearFlag = true
        suspendedClearEntered = false
    }

    func waitForSuspendedClear() async {
        guard !suspendedClearEntered else { return }
        await withCheckedContinuation { suspendedClearWaiters.append($0) }
    }

    func releaseSuspendedClear() {
        suspendedClearContinuation?.resume()
        suspendedClearContinuation = nil
    }

    func waitForCopies(_ count: Int) async {
        guard codes.count < count else { return }
        await withCheckedContinuation { copyWaiters.append((count, $0)) }
    }

    func waitForClears(_ count: Int) async {
        guard clears < count else { return }
        await withCheckedContinuation { clearWaiters.append((count, $0)) }
    }
}

private actor InvitationExpiryGate {
    private var continuation: CheckedContinuation<Void, Error>?
    private var waiters: [CheckedContinuation<Void, Never>] = []

    func sleep() async throws {
        let current = waiters
        waiters.removeAll()
        for waiter in current { waiter.resume() }
        try await withCheckedThrowingContinuation { continuation = $0 }
    }

    func waitUntilSleeping() async {
        guard continuation == nil else { return }
        await withCheckedContinuation { waiters.append($0) }
    }

    func release() {
        continuation?.resume()
        continuation = nil
    }
}
