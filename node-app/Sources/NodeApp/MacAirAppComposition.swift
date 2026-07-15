import Foundation
import NodeAppUI
import NodeCore

/// Projects the common Air control-plane contract into stable macOS read
/// models. Opaque identifiers remain action handles and never become labels.
@MainActor
final class MacAirAppComposition {
    private let client: any AirAppServicing
    private let model: PulsarShellModel
    private var refreshTask: Task<Void, Never>?
    private var mutationTask: Task<Void, Never>?
    private var mutationInFlight = false
    private var lastAutomaticRefresh = Date.distantPast
    private var stopped = false

    init(bundle: CredentialBundle, model: PulsarShellModel) throws {
        client = try AirAppClient(bundle: bundle)
        self.model = model
    }

    func start() { refresh(force: true) }

    func refresh(force: Bool = false) {
        guard !stopped, !mutationInFlight else { return }
        let now = Date()
        guard force || now.timeIntervalSince(lastAutomaticRefresh) >= 5 else { return }
        lastAutomaticRefresh = now
        refreshTask?.cancel()
        refreshTask = Task { [weak self] in await self?.loadProjection() }
    }

    func create(title: String) {
        mutate(outcome: "created") { [client] in
            _ = try await client.create(title: title, idempotencyKey: Self.key("create"))
        }
    }

    func consumeInvite(code: String) {
        guard beginMutation() else { return }
        mutationTask = Task { [weak self] in
            guard let self else { return }
            defer { finishMutation() }
            do {
                let preview = try await client.consumeInvite(
                    code: code, idempotencyKey: Self.key("consume"))
                let pending = shellPending(preview)
                await loadProjection(pendingOverride: pending)
                guard active else { return }
                model.updateAirState(
                    pendingJoin: .some(pending), outcome: .some("invite_reviewed"), failure: .some(nil))
            } catch {
                showFailure(error)
            }
        }
    }

    func confirmJoin(airID: String, activate: Bool) {
        guard let pending = model.snapshot.airs.pendingJoin, pending.airID == airID else {
            model.updateAirState(failure: .some("membership_confirmation_required"))
            return
        }
        let expected = model.snapshot.airs.current?.id
        mutate(outcome: "join_confirmed") { [client] in
            try await client.confirmJoin(
                airID: airID, membershipRevision: pending.membershipRevision,
                activate: activate, expectedActiveAirID: expected,
                idempotencyKey: Self.key("confirm"))
        }
    }

    func declineJoin(airID: String) {
        guard let pending = model.snapshot.airs.pendingJoin, pending.airID == airID else { return }
        mutate(outcome: "join_declined") { [client] in
            try await client.declineJoin(
                airID: airID, membershipRevision: pending.membershipRevision,
                idempotencyKey: Self.key("decline"))
        }
    }

    func issueInvite(airID: String, role: PulsarAirRole) {
        guard let air = air(id: airID), let coreRole = AirRole(rawValue: role.rawValue) else { return }
        guard beginMutation() else { return }
        mutationTask = Task { [weak self] in
            guard let self else { return }
            defer { finishMutation() }
            do {
                let invite = try await client.issueInvite(
                    airID: airID, role: coreRole, idempotencyKey: Self.key("invite"))
                guard active else { return }
                model.updateAirState(
                    inviteSecret: .some(.init(
                        airID: airID, inviteID: invite.id, revision: invite.revision,
                        airTitle: air.title, code: invite.code, expiresAt: invite.expiresAt)),
                    outcome: .some("invite_issued"), failure: .some(nil))
            } catch {
                showFailure(error)
            }
        }
    }

    func withdrawInvite() {
        guard let invite = model.snapshot.airs.inviteSecret else { return }
        mutate(outcome: "invite_withdrawn") { [client] in
            try await client.withdrawInvite(
                airID: invite.airID, inviteID: invite.inviteID, revision: invite.revision,
                idempotencyKey: Self.key("withdraw"))
        }
        model.updateAirState(inviteSecret: .some(nil))
    }

    func hideInvite() {
        model.updateAirState(inviteSecret: .some(nil))
    }

    func activate(airID: String) {
        guard let air = air(id: airID), air.membershipStatus == .joined else { return }
        let expected = model.snapshot.airs.current?.id
        mutate(outcome: "activated") { [client] in
            try await client.activate(
                airID: airID, membershipRevision: air.membershipRevision,
                expectedActiveAirID: expected, idempotencyKey: Self.key("activate"))
        }
    }

    func deactivate(airID: String) {
        guard let air = air(id: airID), air.isCurrent else { return }
        mutate(outcome: "deactivated") { [client] in
            try await client.deactivate(
                airID: airID, membershipRevision: air.membershipRevision,
                expectedActiveAirID: airID, idempotencyKey: Self.key("deactivate"))
        }
    }

    func leave(airID: String) {
        guard let air = air(id: airID) else { return }
        let expected = model.snapshot.airs.current?.id
        mutate(outcome: "left") { [client] in
            try await client.leave(
                airID: airID, membershipRevision: air.membershipRevision,
                expectedActiveAirID: expected, idempotencyKey: Self.key("leave"))
        }
    }

    func dissolve(airID: String) {
        guard let air = air(id: airID) else { return }
        mutate(outcome: "dissolved") { [client] in
            try await client.dissolve(
                airID: airID, revision: air.revision, idempotencyKey: Self.key("dissolve"))
        }
    }

    func replacePolicy(airID: String, policy: PulsarAirPolicy) {
        guard let core = corePolicy(policy), air(id: airID)?.role == .owner else { return }
        mutate(outcome: "policy_updated") { [client] in
            try await client.replacePolicy(
                airID: airID, policy: core, idempotencyKey: Self.key("policy"))
        }
    }

    func shutdown() {
        guard !stopped else { return }
        stopped = true
        refreshTask?.cancel()
        mutationTask?.cancel()
        refreshTask = nil
        mutationTask = nil
        mutationInFlight = false
        model.setAirState(.init())
    }

    private var active: Bool { !Task.isCancelled && !stopped }

    private func beginMutation() -> Bool {
        guard !stopped, !mutationInFlight else { return false }
        mutationInFlight = true
        refreshTask?.cancel()
        model.updateAirState(busy: true, outcome: .some(nil), failure: .some(nil))
        return true
    }

    private func finishMutation() {
        mutationInFlight = false
        model.updateAirState(busy: false)
    }

    private func mutate(
        outcome: String,
        operation: @escaping @MainActor () async throws -> Void
    ) {
        guard beginMutation() else { return }
        mutationTask = Task { [weak self] in
            guard let self else { return }
            defer { finishMutation() }
            do {
                try await operation()
                await loadProjection()
                guard active else { return }
                model.updateAirState(outcome: .some(outcome), failure: .some(nil))
            } catch {
                showFailure(error)
            }
        }
    }

    private func loadProjection(pendingOverride: PulsarPendingAirJoin? = nil) async {
        do {
            let list = try await client.list()
            var byID: [String: AirDetail] = [:]
            try await withThrowingTaskGroup(of: AirDetail.self) { group in
                for item in list.saved {
                    group.addTask { [client] in try await client.detail(id: item.id) }
                }
                for try await detail in group { byID[detail.id] = detail }
            }
            guard active else { return }
            let saved = list.saved.compactMap { byID[$0.id] }.compactMap(shellAir)
            let pendingDetail = saved.first { $0.membershipStatus == .pendingConfirmation }
            let prior = pendingOverride ?? model.snapshot.airs.pendingJoin
            let pending: PulsarPendingAirJoin?
            if let pendingDetail {
                pending = prior?.airID == pendingDetail.id ? prior : .init(
                    airID: pendingDetail.id, title: pendingDetail.title,
                    ownerDisplayName: nil, role: pendingDetail.role,
                    membershipRevision: pendingDetail.membershipRevision,
                    memberCount: pendingDetail.memberCount,
                    barycenterCapacity: pendingDetail.barycenterCapacity,
                    activationWouldSwitch: list.currentAirID != nil && list.currentAirID != pendingDetail.id)
            } else {
                pending = nil
            }
            model.updateAirState(
                saved: saved, pendingJoin: .some(pending), failure: .some(nil))
        } catch {
            guard active else { return }
            model.updateAirState(failure: .some(failureCode(error)))
        }
    }

    private func showFailure(_ error: Error) {
        guard active else { return }
        model.updateAirState(outcome: .some(nil), failure: .some(failureCode(error)))
        if case AirClientError.rejected(_, let code, _) = error,
           ["revision_conflict", "active_air_changed", "air_dissolved"].contains(code) {
            refresh(force: true)
        }
    }

    private func failureCode(_ error: Error) -> String {
        guard let value = error as? AirClientError else { return "coordinator_unavailable" }
        switch value {
        case .transport: return "coordinator_unavailable"
        case .redirectRejected: return "redirect_rejected"
        case .responseTooLarge: return "response_too_large"
        case .invalidConfiguration: return "credential_unavailable"
        case .invalidRequest: return "invalid_request"
        case .invalidResponse: return "invalid_response"
        case .rejected(_, let code, _): return code
        }
    }

    private func air(id: String) -> PulsarAirItem? {
        model.snapshot.airs.saved.first(where: { $0.id == id })
    }

    private func shellAir(_ detail: AirDetail) -> PulsarAirItem? {
        guard let membership = PulsarAirMembershipStatus(rawValue: detail.membershipStatus.rawValue),
              let role = PulsarAirRole(rawValue: detail.role.rawValue),
              let policy = shellPolicy(detail.policy) else { return nil }
        return PulsarAirItem(
            id: detail.id, title: detail.title, status: detail.status,
            revision: detail.revision, membershipID: detail.membershipID,
            membershipStatus: membership, membershipRevision: detail.membershipRevision,
            role: role, memberCount: detail.memberCount,
            activeMemberCount: detail.activeMemberCount,
            onlinePulsarCount: detail.onlinePulsarCount,
            barycenterCapacity: detail.capacity.barycenters,
            onlinePulsarCapacity: detail.capacity.onlinePulsars,
            policy: policy, isCurrent: detail.isCurrent)
    }

    private func shellPending(_ preview: AirJoinPreview) -> PulsarPendingAirJoin {
        .init(
            airID: preview.airID, title: preview.title,
            ownerDisplayName: preview.ownerDisplayName,
            role: PulsarAirRole(rawValue: preview.intendedRole.rawValue) ?? .member,
            membershipRevision: preview.membershipRevision,
            memberCount: preview.memberCount,
            barycenterCapacity: preview.capacity.barycenters,
            activationWouldSwitch: preview.activationWouldSwitch)
    }

    private func shellPolicy(_ policy: AirPolicy) -> PulsarAirPolicy? {
        guard let invite = PulsarAirInvitePolicy(rawValue: policy.invite.rawValue),
              let overlay = PulsarAirPlaybackPolicy(rawValue: policy.overlay.rawValue),
              let queue = PulsarAirPlaybackPolicy(rawValue: policy.queue.rawValue),
              let replace = PulsarAirPlaybackPolicy(rawValue: policy.replace.rawValue) else { return nil }
        return .init(
            revision: policy.revision, invite: invite,
            overlay: overlay, queue: queue, replace: replace)
    }

    private func corePolicy(_ policy: PulsarAirPolicy) -> AirPolicy? {
        guard let invite = AirInvitePolicy(rawValue: policy.invite.rawValue),
              let overlay = AirPlaybackPolicy(rawValue: policy.overlay.rawValue),
              let queue = AirPlaybackPolicy(rawValue: policy.queue.rawValue),
              let replace = AirPlaybackPolicy(rawValue: policy.replace.rawValue) else { return nil }
        return .init(
            revision: policy.revision, invite: invite,
            overlay: overlay, queue: queue, replace: replace)
    }

    private static func key(_ operation: String) -> String {
        "mac-air-\(operation)-\(UUID().uuidString.lowercased())"
    }
}
