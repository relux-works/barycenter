import Foundation
import NodeAppUI
import NodeCore

/// Main-actor projection of the authenticated Phase 1 APIs into the shared
/// shell. Network and outbox work stays in NodeCore; this layer only maps
/// canonical values to human presentation and never exposes opaque IDs as
/// labels.
@MainActor
final class MacPhaseOneAppComposition {
    private let client: PhaseOneAppClient
    private let outbox: PhaseOneDraftOutbox
    private let model: PulsarShellModel
    private weak var capture: MacCaptureAppComposition?
    private var refreshTask: Task<Void, Never>?
    private var mutationTask: Task<Void, Never>?
    private var mutationInFlight = false
    private var lastAutomaticRefresh = Date.distantPast
    private var stopped = false

    init(
        bundle: CredentialBundle,
        supportRoot: URL,
        capture: MacCaptureAppComposition,
        model: PulsarShellModel
    ) throws {
        let client = try PhaseOneAppClient(bundle: bundle)
        self.client = client
        outbox = try PhaseOneDraftOutbox(
            service: client,
            mediaStore: capture.mediaStore,
            stateURL: supportRoot
                .appendingPathComponent("PhaseOne", isDirectory: true)
                .appendingPathComponent("draft-outbox-v1.json"),
            recoveredDrafts: capture.recoveredDrafts)
        self.capture = capture
        self.model = model
    }

    func start() {
        guard !stopped else { return }
        capture?.onNormalDraft = { [weak self] handle in
            self?.attach(handle)
        }
        refresh(force: true)
    }

    func refresh(force: Bool = false) {
        guard !stopped, !mutationInFlight else { return }
        let now = Date()
        guard force || now.timeIntervalSince(lastAutomaticRefresh) >= 5 else { return }
        lastAutomaticRefresh = now
        refreshTask?.cancel()
        refreshTask = Task { [weak self] in await self?.loadProjection() }
    }

    func send(
        draftID: String,
        route: PulsarRouteTarget,
        delivery: PulsarDeliveryMode
    ) {
        guard !stopped, !mutationInFlight,
              let coreRoute = PhaseOneRoute(rawValue: route.rawValue),
              let coreDelivery = PhaseOneDelivery(rawValue: delivery.rawValue) else { return }
        mutationInFlight = true
        refreshTask?.cancel()
        mutationTask = Task { [weak self] in
            guard let self else { return }
            defer { mutationInFlight = false }
            do {
                _ = try await outbox.send(
                    draftID: draftID,
                    route: coreRoute,
                    delivery: coreDelivery)
                await loadProjection()
            } catch {
                await loadOutbox(failure: shellFailure(error))
            }
        }
    }

    func delete(draftID: String) {
        guard !stopped, !mutationInFlight else { return }
        mutationInFlight = true
        refreshTask?.cancel()
        mutationTask = Task { [weak self] in
            guard let self else { return }
            defer { mutationInFlight = false }
            do {
                try await outbox.delete(draftID: draftID)
                await loadProjection()
            } catch {
                await loadOutbox(failure: shellFailure(error))
            }
        }
    }

    func performHistoryAction(_ historyID: String, action: PulsarHistoryAction) {
        guard !stopped, !mutationInFlight else { return }
        mutationInFlight = true
        refreshTask?.cancel()
        mutationTask = Task { [weak self] in
            guard let self else { return }
            defer { mutationInFlight = false }
            do {
                switch action {
                case .delete:
                    try await client.deleteHistoryItem(historyID)
                case .blockActor:
                    try await client.blockHistoryActor(
                        historyID,
                        idempotencyKey: "mac-history-block-\(historyID)")
                case .replay:
                    _ = try await client.replayHistoryItem(
                        historyID,
                        route: .currentAir,
                        delivery: .overlay,
                        idempotencyKey: "mac-history-replay-\(historyID)")
                }
                await loadProjection()
            } catch {
                await loadOutbox(failure: shellFailure(error))
            }
        }
    }

    func shutdown() {
        guard !stopped else { return }
        stopped = true
        capture?.onNormalDraft = nil
        refreshTask?.cancel()
        mutationTask?.cancel()
        refreshTask = nil
        mutationTask = nil
        mutationInFlight = false
    }

    private func attach(_ handle: CaptureMediaHandle) {
        guard !stopped else { return }
        Task { [weak self] in
            guard let self else { return }
            do {
                try await outbox.attach(handle)
                await loadOutbox(failure: nil)
            } catch {
                await loadOutbox(failure: shellFailure(error))
            }
        }
    }

    private func loadProjection() async {
        guard !Task.isCancelled, !stopped else { return }
        let drafts = await outbox.snapshots()
        do {
            async let presence = client.presence()
            async let history = client.history(limit: 30, cursor: nil)
            let (nodes, page) = try await (presence, history)
            guard !Task.isCancelled, !stopped else { return }
            model.setPhaseOneData(
                presenceSummary: presenceSummary(nodes),
                history: page.items.map(shellHistory),
                outgoingDrafts: drafts.map(shellDraft),
                failure: nil)
        } catch {
            guard !Task.isCancelled, !stopped else { return }
            model.setPhaseOneData(
                presenceSummary: nil,
                outgoingDrafts: drafts.map(shellDraft),
                failure: shellFailure(error))
        }
    }

    private func loadOutbox(failure: String?) async {
        let drafts = await outbox.snapshots()
        guard !Task.isCancelled, !stopped else { return }
        model.setPhaseOneData(
            presenceSummary: model.snapshot.presenceSummary,
            outgoingDrafts: drafts.map(shellDraft),
            failure: failure)
    }

    private func presenceSummary(_ nodes: [PhaseOnePresenceNode]) -> String {
        let online = nodes.filter(\.online).count
        let ready = nodes.filter { $0.online && $0.outputState == "ready" }.count
        switch model.locale {
        case .en: return "\(online)/\(nodes.count) online · \(ready) ready"
        case .ru: return "\(online)/\(nodes.count) в сети · \(ready) готовы"
        }
    }

    private func shellHistory(_ item: PhaseOneHistoryItem) -> PulsarHistoryItem {
        var detail: [String] = []
        if let sender = item.senderName, !sender.isEmpty { detail.append(sender) }
        detail.append(item.status.replacingOccurrences(of: "_", with: " "))
        if let played = item.playedCount, let other = item.otherCount {
            switch model.locale {
            case .en: detail.append("\(played) played · \(other) other")
            case .ru: detail.append("\(played) воспроизведено · \(other) прочее")
            }
        }
        let actions = item.actions.compactMap(PulsarHistoryAction.init(rawValue:))
        return PulsarHistoryItem(
            id: item.id,
            title: item.title,
            detail: detail.joined(separator: " · "),
            occurredAt: item.occurredAt,
            direction: item.direction,
            senderName: item.senderName,
            status: item.status,
            requestedDelivery: item.requestedDelivery,
            effectiveDelivery: item.effectiveDelivery,
            downgradeReason: item.downgradeReason,
            allowedActions: actions)
    }

    private func shellDraft(_ draft: PhaseOneDraftSnapshot) -> PulsarOutgoingDraft {
        PulsarOutgoingDraft(
            id: draft.draftID,
            title: draft.title,
            state: PulsarOutgoingDraftState(rawValue: draft.state.rawValue) ?? .retryableFailure,
            route: draft.route.flatMap { PulsarRouteTarget(rawValue: $0.rawValue) },
            requestedDelivery: draft.requestedDelivery.flatMap {
                PulsarDeliveryMode(rawValue: $0.rawValue)
            },
            effectiveDelivery: draft.effectiveDelivery.flatMap {
                PulsarDeliveryMode(rawValue: $0.rawValue)
            },
            downgradeReason: draft.downgradeReason,
            status: draft.status,
            failureCode: draft.failureCode,
            localBytesRetained: draft.localBytesRetained)
    }

    private func shellFailure(_ error: Error) -> String {
        switch error {
        case let value as PhaseOneDraftOutboxError:
            return String(describing: value).replacingOccurrences(of: "_", with: " ")
        case let value as PhaseOneClientError:
            return String(describing: value).replacingOccurrences(of: "_", with: " ")
        default:
            return "Coordinator data is temporarily unavailable"
        }
    }
}
