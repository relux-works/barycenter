import Foundation
import NodeAppUI
import NodeCore

/// Authenticated Phase 2 projection and command executor for the native macOS
/// surface. Opaque references/cursors remain inside typed model commands and
/// are never rendered or logged.
@MainActor
final class MacTargetsInboxAppComposition {
    private struct SelectionIntent {
        let audience: PulsarTargetsInboxAudienceKind?
        let references: [String]
        let includeOrigin: Bool
    }

    private let client: any TargetsInboxAppServicing
    private let shellModel: PulsarShellModel
    private let model: PulsarTargetsInboxModel
    private var refreshTask: Task<Void, Never>?
    private var commandTask: Task<Void, Never>?
    private var lastAutomaticRefresh = Date.distantPast
    private var generation = 0
    private var stopped = false

    init(
        bundle: CredentialBundle,
        shellModel: PulsarShellModel,
        model: PulsarTargetsInboxModel
    ) throws {
        client = try TargetsInboxAppClient(bundle: bundle)
        self.shellModel = shellModel
        self.model = model
    }

    init(
        client: any TargetsInboxAppServicing,
        shellModel: PulsarShellModel,
        model: PulsarTargetsInboxModel
    ) {
        self.client = client
        self.shellModel = shellModel
        self.model = model
    }

    func start() { refresh(force: true) }

    func shutdown() {
        stopped = true
        generation += 1
        refreshTask?.cancel()
        commandTask?.cancel()
        refreshTask = nil
        commandTask = nil
    }

    func refresh(force: Bool = false) {
        guard !stopped, commandTask == nil, refreshTask == nil else { return }
        // Target references are short-lived object capabilities. Do not replace
        // a user's current explicit selection just because the shell heartbeat
        // fired; manual refresh and post-mutation authority refresh stay explicit.
        guard force || model.snapshot.selectedReferences.isEmpty else { return }
        let now = Date()
        guard force || now.timeIntervalSince(lastAutomaticRefresh) >= 5 else { return }
        lastAutomaticRefresh = now
        generation += 1
        let requestGeneration = generation
        let selectionIntent = currentSelectionIntent
        setSurfaceState(hasRetainedRows ? .stale : .loading)
        refreshTask = Task { [weak self] in
            guard let self else { return }
            do {
                let projection = try await client.projection()
                guard !Task.isCancelled, requestGeneration == generation else { return }
                apply(projection, preservingSelection: selectionIntent)
            } catch {
                guard !Task.isCancelled, requestGeneration == generation else { return }
                applyFailure(error)
            }
            refreshTask = nil
        }
    }

    func perform(_ command: PulsarTargetsInboxCommand) {
        guard !stopped else { return }
        switch command {
        case .refresh:
            refresh(force: true)
        case .setAudience(let audience):
            guard model.setAudienceCommand(audience) == command else { return }
            updateLocal { $0.selectedAudience = audience }
        case .selectTargets(let references):
            guard model.selectTargetsCommand(references) == command else { return }
            updateLocal {
                $0.selectedReferences = references
                if $0.selectedAudience == nil { $0.selectedAudience = .explicit }
            }
        case .setIncludeOrigin(let include):
            guard model.setIncludeOriginCommand(include) == command else { return }
            updateLocal { $0.includeOrigin = include }
        default:
            performRemote(command)
        }
    }

    private func performRemote(_ command: PulsarTargetsInboxCommand) {
        guard commandTask == nil, refreshTask == nil, model.snapshot.state == .ready else { return }
        guard commandIsCurrent(command) else { return }
        var pending = model.snapshot
        pending.commandInFlight = true
        pending.actionOutcome = nil
        pending.actionFailure = nil
        model.replace(pending)
        commandTask = Task { [weak self] in
            guard let self else { return }
            do {
                let outcome = try await execute(command)
                guard !Task.isCancelled else { return }
                if let outcome { setActionOutcome(outcome) }
                if commandNeedsProjectionRefresh(command) {
                    let projection = try await client.projection()
                    guard !Task.isCancelled else { return }
                    apply(projection, preservingOutcome: outcome)
                }
            } catch let error as TargetsInboxClientError {
                guard !Task.isCancelled else { return }
                if case let .rejected(status, code, _) = error,
                   status == 410, code == "cursor_expired" {
                    commandTask = nil
                    refresh(force: true)
                    return
                }
                setActionFailure(failureLabel(error))
            } catch {
                guard !Task.isCancelled else { return }
                setActionFailure(label(
                    "history.outcome.failed", "The action failed. Try again.",
                    "Не удалось выполнить действие. Повторите попытку."))
            }
            clearCommandInFlight()
            commandTask = nil
        }
    }

    private func execute(_ command: PulsarTargetsInboxCommand) async throws -> String? {
        switch command {
        case .loadMoreInbox(let cursor):
            let page = try await client.inbox(cursor: cursor)
            appendInbox(page)
            return nil
        case .loadMoreHistory(let cursor):
            let page = try await client.history(cursor: cursor)
            appendHistory(page)
            return nil
        case .loadMoreReceipts(let id, let cursor):
            let page = try await client.receipts(historyItemID: id, cursor: cursor)
            appendReceipts(page)
            return nil
        case .replayInbox(let id, let delivery):
            return try await client.replayInbox(
                id, delivery: delivery.rawValue, idempotencyKey: idempotencyKey("inbox-replay"))
        case .dismissInbox(let id):
            return try await client.dismissInbox(id)
        case .deleteHistory(let id):
            return try await client.deleteHistory(id)
        case .reportInbox(let id, let reason, let details),
             .reportHistory(let id, let reason, let details):
            return try await client.reportHistory(id, reason: reason.rawValue, details: details)
        case .muteSender(let id):
            return try await client.muteHistorySender(
                id, idempotencyKey: idempotencyKey("mute-sender"))
        case .refresh, .setAudience, .selectTargets, .setIncludeOrigin:
            return nil
        }
    }

    private func commandIsCurrent(_ command: PulsarTargetsInboxCommand) -> Bool {
        switch command {
        case .loadMoreInbox: model.loadMoreInboxCommand() == command
        case .loadMoreHistory: model.loadMoreHistoryCommand() == command
        case .loadMoreReceipts(let id, _): model.loadMoreReceiptsCommand(historyItemID: id) == command
        case .replayInbox(let id, let delivery): model.replayInboxCommand(id: id, delivery: delivery) == command
        case .dismissInbox(let id): model.dismissInboxCommand(id: id) == command
        case .deleteHistory(let id): model.deleteHistoryCommand(id: id) == command
        case .reportInbox(let id, let reason, let details):
            inboxID(forHistoryID: id).map {
                model.reportInboxCommand(id: $0, reason: reason, details: details) == command
            } == true
        case .reportHistory(let id, let reason, let details):
            model.reportHistoryCommand(id: id, reason: reason, details: details) == command
        case .muteSender(let id):
            model.muteSenderCommand(id: id) == command || inboxID(forHistoryID: id).map {
                model.muteSenderCommand(id: $0) == command
            } == true
        case .refresh, .setAudience, .selectTargets, .setIncludeOrigin: true
        }
    }

    private func inboxID(forHistoryID historyID: String) -> String? {
        model.snapshot.inbox.first { $0.historyItemID == historyID }?.id
    }

    private func commandNeedsProjectionRefresh(_ command: PulsarTargetsInboxCommand) -> Bool {
        switch command {
        case .loadMoreInbox, .loadMoreHistory, .loadMoreReceipts: false
        default: true
        }
    }

    private func apply(
        _ projection: TargetsInboxProjection,
        preservingOutcome outcome: String? = nil,
        preservingSelection selectionIntent: SelectionIntent? = nil
    ) {
        let previous = model.snapshot
        let selectionIntent = selectionIntent ?? SelectionIntent(
            audience: previous.selectedAudience,
            references: previous.selectedReferences,
            includeOrigin: previous.includeOrigin)
        let targets = projection.targets.map {
            PulsarTargetChoice(
                reference: $0.reference, kind: $0.kind, expiresAt: $0.expiresAt,
                capabilityState: $0.capabilityState, capabilities: $0.capabilities,
                label: map($0.label))
        }
        let targetReferences = Set(targets.map(\.reference))
        let selectedReferences = selectionIntent.references.filter(targetReferences.contains)
        let audiences = audienceChoices(hasTargets: !targets.isEmpty)
        var selectedAudience = selectionIntent.audience
        if selectedAudience == .explicit && selectedReferences.isEmpty { selectedAudience = nil }
        if !audiences.contains(where: { $0.kind == selectedAudience }) { selectedAudience = nil }
        let snapshot = PulsarTargetsInboxSnapshot(
            state: .ready,
            stateLabel: stateLabel(.ready),
            activeAirTitle: shellModel.snapshot.airs.current?.title,
            availableAudiences: audiences,
            selectedAudience: selectedAudience,
            targets: targets,
            selectedReferences: selectedReferences,
            includeOrigin: selectionIntent.includeOrigin,
            targetedTrackPolicy: "unsupported",
            contentPolicyState: projection.contentPolicyState,
            inbox: projection.inbox.items.map(map),
            inboxNextCursor: projection.inbox.nextCursor,
            history: projection.history.items.map(map),
            historyNextCursor: projection.history.nextCursor,
            actionOutcome: outcome.map(outcomeLabel),
            actionFailure: nil,
            commandInFlight: false)
        model.replace(snapshot)
    }

    private func appendInbox(_ page: TargetsInboxPage) {
        var snapshot = model.snapshot
        var known = Set(snapshot.inbox.map(\.id))
        for item in page.items where known.insert(item.id).inserted {
            snapshot.inbox.append(map(item))
        }
        snapshot.inboxNextCursor = page.nextCursor
        model.replace(snapshot)
    }

    private func appendHistory(_ page: TargetsInboxHistoryPage) {
        var snapshot = model.snapshot
        var known = Set(snapshot.history.map(\.id))
        for item in page.items where known.insert(item.id).inserted {
            snapshot.history.append(map(item))
        }
        snapshot.historyNextCursor = page.nextCursor
        model.replace(snapshot)
    }

    private func appendReceipts(_ page: TargetsInboxReceiptPage) {
        var snapshot = model.snapshot
        guard let index = snapshot.history.firstIndex(where: { $0.id == page.historyItemID }) else { return }
        let item = snapshot.history[index]
        var known = Set(item.receipts.items.map(\.targetLabel))
        var additions: [PulsarHistoryReceiptCapability] = []
        for receipt in page.items where known.insert(receipt.targetLabel).inserted {
            additions.append(.init(targetLabel: receipt.targetLabel, status: map(receipt.status)))
        }
        snapshot.history[index] = PulsarHistoryCapabilityItem(
            id: item.id, title: item.title, status: item.status, actions: item.actions,
            playedCount: item.playedCount, otherCount: item.otherCount,
            receipts: .init(items: item.receipts.items + additions, nextCursor: page.nextCursor))
        model.replace(snapshot)
    }

    private func updateLocal(_ update: (inout PulsarTargetsInboxSnapshot) -> Void) {
        var snapshot = model.snapshot
        update(&snapshot)
        snapshot.actionFailure = nil
        model.replace(snapshot)
    }

    private func setSurfaceState(_ state: PulsarTargetsInboxSurfaceState) {
        var snapshot = model.snapshot
        snapshot.state = state
        snapshot.stateLabel = stateLabel(state)
        snapshot.commandInFlight = false
        model.replace(snapshot)
    }

    private func applyFailure(_ error: Error) {
        let state: PulsarTargetsInboxSurfaceState
        if let value = error as? TargetsInboxClientError, value == .transport {
            state = .offline
        } else {
            state = .coordinatorError
        }
        var snapshot = model.snapshot
        snapshot.state = state
        snapshot.stateLabel = stateLabel(state)
        snapshot.commandInFlight = false
        snapshot.actionFailure = failureLabel(error)
        model.replace(snapshot)
    }

    private func setActionOutcome(_ code: String) {
        var snapshot = model.snapshot
        snapshot.actionOutcome = outcomeLabel(code)
        snapshot.actionFailure = nil
        model.replace(snapshot)
    }

    private func setActionFailure(_ failure: PulsarLocalizedLabel) {
        var snapshot = model.snapshot
        snapshot.actionOutcome = nil
        snapshot.actionFailure = failure
        model.replace(snapshot)
    }

    private func clearCommandInFlight() {
        var snapshot = model.snapshot
        snapshot.commandInFlight = false
        model.replace(snapshot)
    }

    private var hasRetainedRows: Bool {
        !model.snapshot.inbox.isEmpty || !model.snapshot.history.isEmpty || !model.snapshot.targets.isEmpty
    }

    private var currentSelectionIntent: SelectionIntent {
        SelectionIntent(
            audience: model.snapshot.selectedAudience,
            references: model.snapshot.selectedReferences,
            includeOrigin: model.snapshot.includeOrigin)
    }

    private func audienceChoices(hasTargets: Bool) -> [PulsarTargetsInboxAudienceChoice] {
        var values = [
            PulsarTargetsInboxAudienceChoice(
                kind: .thisPulsar,
                label: label("audience.this_pulsar", "This Pulsar", "Этот Пульсар")),
            PulsarTargetsInboxAudienceChoice(
                kind: .ownBarycenter,
                label: label("audience.own_barycenter", "My Barycenter", "Мой Барицентр")),
        ]
        if let title = shellModel.snapshot.airs.current?.title {
            values.append(.init(
                kind: .currentAir,
                label: label(
                    "audience.current_air_named", "Current Air with «\(title)»",
                    "Текущий эфир с «\(title)»")))
        }
        if hasTargets {
            values.append(.init(
                kind: .explicit,
                label: label("audience.explicit", "Selected recipients", "Выбранные получатели")))
        }
        return values
    }

    private func map(_ value: TargetsInboxWireLabel) -> PulsarLocalizedLabel {
        .init(key: value.key, en: value.en, ru: value.ru)
    }

    private func map(_ value: TargetsInboxWireAction) -> PulsarActionCapability {
        .init(action: value.action, label: map(value.label))
    }

    private func map(_ value: TargetsInboxItem) -> PulsarInboxCapabilityItem {
        .init(
            id: value.id, historyItemID: value.historyItemID, title: value.title,
            expiresAt: value.expiresAt, availability: value.availability,
            sender: map(value.sender), source: map(value.source),
            requestedDelivery: map(value.requestedDelivery),
            effectiveDelivery: map(value.effectiveDelivery), receipt: map(value.receipt),
            actions: value.actions.map(map))
    }

    private func map(_ value: TargetsInboxHistoryItem) -> PulsarHistoryCapabilityItem {
        .init(
            id: value.id, title: value.title, status: map(value.status),
            actions: value.actions.map(map), playedCount: value.playedCount,
            otherCount: value.otherCount)
    }

    private func stateLabel(_ state: PulsarTargetsInboxSurfaceState) -> PulsarLocalizedLabel {
        switch state {
        case .loading: label("surface.loading", "Loading", "Загрузка")
        case .ready: label("surface.ready", "Up to date", "Актуально")
        case .stale: label("surface.stale", "May be out of date", "Данные могут быть устаревшими")
        case .offline: label("surface.offline", "Offline", "Нет сети")
        case .coordinatorError:
            label("surface.coordinator_error", "Coordinator unavailable", "Координатор недоступен")
        }
    }

    private func outcomeLabel(_ code: String) -> PulsarLocalizedLabel {
        switch code {
        case "media_deleted":
            label("history.outcome.media_deleted", "Media deleted. It can no longer be replayed.",
                  "Медиа удалено. Его больше нельзя повторно воспроизвести.")
        case "report_received":
            label("history.outcome.report_received", "Report received for moderation.",
                  "Жалоба принята на модерацию.")
        case "report_already_received":
            label("history.outcome.report_already_received", "This item was already reported.",
                  "На этот материал уже подана жалоба.")
        case "sender_blocked":
            label("history.outcome.sender_blocked", "Sender muted.", "Отправитель заглушен.")
        case "sender_already_blocked":
            label("history.outcome.sender_already_blocked", "Sender was already muted.",
                  "Отправитель уже был заглушен.")
        case "replay_accepted":
            label("history.outcome.replay_accepted", "Replay accepted.", "Повтор принят.")
        case "replay_already_accepted":
            label("history.outcome.replay_already_accepted", "Replay was already accepted.",
                  "Повтор уже был принят.")
        case "inbox_dismissed":
            label("inbox.outcome.dismissed", "Inbox item dismissed.", "Входящий материал убран.")
        default:
            label("history.outcome.failed", "The action failed. Try again.",
                  "Не удалось выполнить действие. Повторите попытку.")
        }
    }

    private func failureLabel(_ error: Error) -> PulsarLocalizedLabel {
        guard let value = error as? TargetsInboxClientError else {
            return label("history.outcome.failed", "The action failed. Try again.",
                         "Не удалось выполнить действие. Повторите попытку.")
        }
        switch value {
        case .transport:
            return label("surface.offline", "Offline. Check the connection and try again.",
                         "Нет сети. Проверьте подключение и повторите попытку.")
        case let .rejected(_, code, _):
            return label("coordinator.\(code)", "The coordinator rejected this current action.",
                         "Координатор отклонил это текущее действие.")
        default:
            return label("surface.coordinator_error", "Coordinator unavailable.",
                         "Координатор недоступен.")
        }
    }

    private func label(_ key: String, _ en: String, _ ru: String) -> PulsarLocalizedLabel {
        .init(key: key, en: en, ru: ru)
    }

    private func idempotencyKey(_ prefix: String) -> String {
        "mac-\(prefix)-\(UUID().uuidString.lowercased())"
    }
}
