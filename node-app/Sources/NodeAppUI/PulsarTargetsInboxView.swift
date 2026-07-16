import SwiftUI

public struct PulsarTargetsInboxView: View {
    let shellModel: PulsarShellModel
    @Bindable var model: PulsarTargetsInboxModel
    let actions: PulsarTargetsInboxActions

    public init(
        shellModel: PulsarShellModel,
        model: PulsarTargetsInboxModel,
        actions: PulsarTargetsInboxActions
    ) {
        self.shellModel = shellModel
        self.model = model
        self.actions = actions
    }

    public var body: some View {
        let copy = PulsarTargetsInboxCopy(locale: shellModel.locale)
        ScrollView {
            LazyVStack(alignment: .leading, spacing: 20) {
                PulsarTargetsInboxStateHeader(
                    snapshot: model.snapshot, locale: shellModel.locale, copy: copy)
                PulsarTargetRoutingSection(
                    model: model, actions: actions, locale: shellModel.locale, copy: copy)
                PulsarInboxSection(
                    model: model, actions: actions, locale: shellModel.locale, copy: copy)
                PulsarCapabilityHistorySection(
                    model: model, actions: actions, locale: shellModel.locale, copy: copy)
            }
            .padding(24)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .navigationTitle(copy.inboxAndTargets)
        .toolbar {
            Button(copy.refresh) { actions.perform(model.refreshCommand()) }
                .keyboardShortcut("r", modifiers: .command)
                .disabled(model.snapshot.commandInFlight)
        }
    }
}

private struct PulsarTargetsInboxStateHeader: View {
    let snapshot: PulsarTargetsInboxSnapshot
    let locale: PulsarShellLocale
    let copy: PulsarTargetsInboxCopy

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Label(stateText, systemImage: stateSymbol)
                .font(.headline)
            if let activeAirTitle = snapshot.activeAirTitle {
                LabeledContent(copy.activeAir, value: activeAirTitle)
            }
            if let outcome = snapshot.actionOutcome {
                Label(outcome.text(locale: locale), systemImage: "checkmark.circle")
                    .foregroundStyle(.green)
            }
            if let failure = snapshot.actionFailure {
                Label(failure.text(locale: locale), systemImage: "exclamationmark.triangle")
                    .foregroundStyle(.orange)
            }
            if snapshot.commandInFlight { ProgressView().controlSize(.small) }
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.quaternary, in: .rect(cornerRadius: 12))
        .accessibilityElement(children: .contain)
        .accessibilityLabel(copy.surfaceStatus)
    }

    private var stateText: String {
        snapshot.stateLabel?.text(locale: locale) ?? copy.state(snapshot.state)
    }

    private var stateSymbol: String {
        switch snapshot.state {
        case .loading: "hourglass"
        case .ready: "checkmark.circle.fill"
        case .stale: "clock.badge.exclamationmark"
        case .offline: "wifi.slash"
        case .coordinatorError: "exclamationmark.triangle.fill"
        }
    }
}

private struct PulsarTargetRoutingSection: View {
    @Bindable var model: PulsarTargetsInboxModel
    let actions: PulsarTargetsInboxActions
    let locale: PulsarShellLocale
    let copy: PulsarTargetsInboxCopy

    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text(copy.recipients).font(.title2.bold())
            Picker(copy.audience, selection: audienceBinding) {
                Text(copy.chooseAudience).tag(PulsarTargetsInboxAudienceKind?.none)
                ForEach(model.snapshot.availableAudiences, id: \.kind.rawValue) { audience in
                    Text(audience.label.text(locale: locale)).tag(Optional(audience.kind))
                }
            }
            .disabled(!isReady || model.snapshot.commandInFlight)

            if !model.snapshot.targets.isEmpty {
                VStack(alignment: .leading, spacing: 8) {
                    Text(copy.explicitTargets).font(.headline)
                    ForEach(model.snapshot.targets, id: \.reference) { target in
                        PulsarTargetChoiceRow(
                            target: target,
                            selected: model.snapshot.selectedReferences.contains(target.reference),
                            locale: locale,
                            copy: copy,
                            disabled: !isReady || model.snapshot.commandInFlight
                        ) { selected in
                            var references = model.snapshot.selectedReferences
                            if selected {
                                references.append(target.reference)
                            } else {
                                references.removeAll { $0 == target.reference }
                            }
                            actions.perform(model.selectTargetsCommand(references))
                        }
                    }
                }
                .accessibilityElement(children: .contain)
                .accessibilityLabel(copy.explicitTargets)
            }

            Toggle(copy.includeOrigin, isOn: includeOriginBinding)
                .disabled(!isReady || model.snapshot.commandInFlight)
            LabeledContent(copy.trackPolicy, value: copy.trackPolicy(model.snapshot.targetedTrackPolicy))
            LabeledContent(copy.contentPolicy, value: copy.contentPolicy(model.snapshot.contentPolicyState))
            if model.snapshot.contentPolicyState != "current" {
                Text(copy.contentPolicyHelp)
                    .font(.callout)
                    .foregroundStyle(.secondary)
            }
        }
        .focusSection()
    }

    private var isReady: Bool { model.snapshot.state == .ready }

    private var audienceBinding: Binding<PulsarTargetsInboxAudienceKind?> {
        Binding(
            get: { model.snapshot.selectedAudience },
            set: { value in
                guard let value else { return }
                actions.perform(model.setAudienceCommand(value))
            })
    }

    private var includeOriginBinding: Binding<Bool> {
        Binding(
            get: { model.snapshot.includeOrigin },
            set: { actions.perform(model.setIncludeOriginCommand($0)) })
    }
}

private struct PulsarTargetChoiceRow: View {
    let target: PulsarTargetChoice
    let selected: Bool
    let locale: PulsarShellLocale
    let copy: PulsarTargetsInboxCopy
    let disabled: Bool
    let onSelection: (Bool) -> Void

    var body: some View {
        Toggle(isOn: Binding(get: { selected }, set: onSelection)) {
            VStack(alignment: .leading, spacing: 3) {
                Text(target.label.text(locale: locale))
                Text(copy.capability(target.capabilityState))
                    .font(.caption)
                    .foregroundStyle(target.capabilityState == "known" ? Color.secondary : Color.orange)
                if !target.capabilities.isEmpty {
                    Text(target.capabilities.formatted())
                        .font(.caption2.monospaced())
                        .foregroundStyle(.secondary)
                }
            }
        }
        .disabled(disabled)
        .accessibilityHint(copy.targetSelectionHint)
    }
}

private struct PulsarInboxSection: View {
    @Bindable var model: PulsarTargetsInboxModel
    let actions: PulsarTargetsInboxActions
    let locale: PulsarShellLocale
    let copy: PulsarTargetsInboxCopy

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(copy.inbox).font(.title2.bold())
            if model.snapshot.inbox.isEmpty {
                ContentUnavailableView(copy.emptyInbox, systemImage: "tray")
                    .frame(maxWidth: .infinity, minHeight: 100)
            } else {
                ForEach(model.snapshot.inbox) { item in
                    PulsarInboxCapabilityRow(
                        item: item, model: model, actions: actions,
                        locale: locale, copy: copy)
                }
            }
            if model.snapshot.inboxNextCursor != nil {
                Button(copy.loadMore) { actions.perform(model.loadMoreInboxCommand()) }
                    .disabled(model.snapshot.commandInFlight || model.snapshot.state != .ready)
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityLabel(copy.inbox)
    }
}

private struct PulsarInboxCapabilityRow: View {
    let item: PulsarInboxCapabilityItem
    let model: PulsarTargetsInboxModel
    let actions: PulsarTargetsInboxActions
    let locale: PulsarShellLocale
    let copy: PulsarTargetsInboxCopy
    @State private var delivery = PulsarDeliveryMode.afterCurrent
    @State private var reportReason = PulsarModerationReason.spam
    @State private var reportDetails = ""
    @State private var reporting = false
    @State private var pendingDestructiveAction: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(item.title).font(.headline)
            Text(item.sender.text(locale: locale))
            Text(item.source.text(locale: locale)).foregroundStyle(.secondary)
            LabeledContent(copy.delivery) {
                Text("\(item.requestedDelivery.text(locale: locale)) → \(item.effectiveDelivery.text(locale: locale))")
            }
            LabeledContent(copy.receipt, value: item.receipt.text(locale: locale))
            LabeledContent(copy.availability, value: copy.availability(item.availability))
            LabeledContent(copy.availableUntil) {
                Text(item.expiresAt, format: .dateTime.year().month().day().hour().minute())
            }
            if hasAction("replay") {
                Picker(copy.replayDelivery, selection: $delivery) {
                    ForEach(PulsarDeliveryMode.allCases) { mode in
                        Text(copy.delivery(mode)).tag(mode)
                    }
                }
                Button(actionLabel("replay")) {
                    actions.perform(model.replayInboxCommand(id: item.id, delivery: delivery))
                }
                .keyboardShortcut(.return, modifiers: [.command, .shift])
                .accessibilityHint(copy.explicitReplayHint)
            }
            ViewThatFits {
                HStack { mutationButtons }
                VStack(alignment: .leading) { mutationButtons }
            }
            if reporting {
                PulsarCapabilityReportForm(
                    reason: $reportReason, details: $reportDetails,
                    locale: locale, copy: copy
                ) {
                    actions.perform(model.reportInboxCommand(
                        id: item.id, reason: reportReason,
                        details: boundedReportDetails(reportDetails)))
                    reporting = false
                }
            }
        }
        .padding(14)
        .background(.background, in: .rect(cornerRadius: 10))
        .overlay { RoundedRectangle(cornerRadius: 10).stroke(.separator) }
        .disabled(model.snapshot.commandInFlight || model.snapshot.state != .ready)
        .accessibilityElement(children: .contain)
        .accessibilityLabel("\(item.title), \(item.sender.text(locale: locale))")
        .confirmationDialog(
            copy.confirmMute,
            isPresented: Binding(
                get: { pendingDestructiveAction != nil },
                set: { if !$0 { pendingDestructiveAction = nil } })
        ) {
            Button(actionLabel("block_actor"), role: .destructive) {
                actions.perform(model.muteSenderCommand(id: item.id))
                pendingDestructiveAction = nil
            }
            Button(copy.cancel, role: .cancel) { pendingDestructiveAction = nil }
        }
    }

    @ViewBuilder private var mutationButtons: some View {
        if hasAction("dismiss") {
            Button(actionLabel("dismiss")) {
                actions.perform(model.dismissInboxCommand(id: item.id))
            }
        }
        if hasAction("report") { Button(actionLabel("report")) { reporting.toggle() } }
        if hasAction("block_actor") {
            Button(actionLabel("block_actor"), role: .destructive) {
                pendingDestructiveAction = "block_actor"
            }
        }
    }

    private func hasAction(_ value: String) -> Bool { item.actions.contains { $0.action == value } }
    private func actionLabel(_ value: String) -> String {
        item.actions.first { $0.action == value }?.label.text(locale: locale) ?? copy.unsupported
    }
}

private struct PulsarCapabilityHistorySection: View {
    @Bindable var model: PulsarTargetsInboxModel
    let actions: PulsarTargetsInboxActions
    let locale: PulsarShellLocale
    let copy: PulsarTargetsInboxCopy

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            Text(copy.history).font(.title2.bold())
            if model.snapshot.history.isEmpty {
                ContentUnavailableView(copy.emptyHistory, systemImage: "clock.arrow.circlepath")
                    .frame(maxWidth: .infinity, minHeight: 100)
            } else {
                ForEach(model.snapshot.history) { item in
                    PulsarCapabilityHistoryRow(
                        item: item, model: model, actions: actions,
                        locale: locale, copy: copy)
                }
            }
            if model.snapshot.historyNextCursor != nil {
                Button(copy.loadMore) { actions.perform(model.loadMoreHistoryCommand()) }
                    .disabled(model.snapshot.commandInFlight || model.snapshot.state != .ready)
            }
        }
        .accessibilityElement(children: .contain)
        .accessibilityLabel(copy.history)
    }
}

private struct PulsarCapabilityHistoryRow: View {
    let item: PulsarHistoryCapabilityItem
    let model: PulsarTargetsInboxModel
    let actions: PulsarTargetsInboxActions
    let locale: PulsarShellLocale
    let copy: PulsarTargetsInboxCopy
    @State private var reportReason = PulsarModerationReason.spam
    @State private var reportDetails = ""
    @State private var reporting = false
    @State private var pendingDestructiveAction: String?

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Text(item.title).font(.headline)
            Text(item.status.text(locale: locale)).foregroundStyle(.secondary)
            Text(copy.deliveryCounts(played: item.playedCount, other: item.otherCount))
                .font(.callout)
            ForEach(item.receipts.items, id: \.targetLabel) { receipt in
                LabeledContent(receipt.targetLabel, value: receipt.status.text(locale: locale))
            }
            if item.receipts.nextCursor != nil {
                Button(copy.loadMoreReceipts) {
                    actions.perform(model.loadMoreReceiptsCommand(historyItemID: item.id))
                }
            }
            ViewThatFits {
                HStack { mutationButtons }
                VStack(alignment: .leading) { mutationButtons }
            }
            if reporting {
                PulsarCapabilityReportForm(
                    reason: $reportReason, details: $reportDetails,
                    locale: locale, copy: copy
                ) {
                    actions.perform(model.reportHistoryCommand(
                        id: item.id, reason: reportReason,
                        details: boundedReportDetails(reportDetails)))
                    reporting = false
                }
            }
        }
        .padding(14)
        .background(.background, in: .rect(cornerRadius: 10))
        .overlay { RoundedRectangle(cornerRadius: 10).stroke(.separator) }
        .disabled(model.snapshot.commandInFlight || model.snapshot.state != .ready)
        .accessibilityElement(children: .contain)
        .accessibilityLabel("\(item.title), \(item.status.text(locale: locale))")
        .confirmationDialog(
            pendingDestructiveAction == "delete" ? copy.confirmDelete : copy.confirmMute,
            isPresented: Binding(
                get: { pendingDestructiveAction != nil },
                set: { if !$0 { pendingDestructiveAction = nil } })
        ) {
            if pendingDestructiveAction == "delete" {
                Button(actionLabel("delete"), role: .destructive) {
                    actions.perform(model.deleteHistoryCommand(id: item.id))
                    pendingDestructiveAction = nil
                }
            } else {
                Button(actionLabel("block_actor"), role: .destructive) {
                    actions.perform(model.muteSenderCommand(id: item.id))
                    pendingDestructiveAction = nil
                }
            }
            Button(copy.cancel, role: .cancel) { pendingDestructiveAction = nil }
        }
    }

    @ViewBuilder private var mutationButtons: some View {
        if hasAction("delete") {
            Button(actionLabel("delete"), role: .destructive) {
                pendingDestructiveAction = "delete"
            }
        }
        if hasAction("report") { Button(actionLabel("report")) { reporting.toggle() } }
        if hasAction("block_actor") {
            Button(actionLabel("block_actor"), role: .destructive) {
                pendingDestructiveAction = "block_actor"
            }
        }
    }

    private func hasAction(_ value: String) -> Bool { item.actions.contains { $0.action == value } }
    private func actionLabel(_ value: String) -> String {
        item.actions.first { $0.action == value }?.label.text(locale: locale) ?? copy.unsupported
    }
}

private struct PulsarCapabilityReportForm: View {
    @Binding var reason: PulsarModerationReason
    @Binding var details: String
    let locale: PulsarShellLocale
    let copy: PulsarTargetsInboxCopy
    let submit: () -> Void

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Picker(copy.reportReason, selection: $reason) {
                ForEach(PulsarModerationReason.allCases) { value in
                    Text(PulsarShellCopy(locale: locale).moderationReasonLabel(value)).tag(value)
                }
            }
            TextField(copy.reportDetails, text: $details, axis: .vertical)
                .lineLimit(2...5)
                .accessibilityLabel(copy.reportDetails)
            Button(copy.submitReport, action: submit)
        }
    }
}

private func boundedReportDetails(_ value: String) -> String {
    let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
    var bytes = 0
    return String(trimmed.prefix { character in
        let count = String(character).utf8.count
        guard bytes + count <= 2_000 else { return false }
        bytes += count
        return true
    })
}

private struct PulsarTargetsInboxCopy: Sendable {
    let locale: PulsarShellLocale
    private func text(_ en: String, _ ru: String) -> String { locale == .ru ? ru : en }

    var inboxAndTargets: String { text("Inbox & targets", "Входящие и адресаты") }
    var surfaceStatus: String { text("Inbox data status", "Состояние входящих данных") }
    var activeAir: String { text("Active Air", "Активный эфир") }
    var refresh: String { text("Refresh", "Обновить") }
    var recipients: String { text("Recipients", "Адресаты") }
    var audience: String { text("Audience", "Аудитория") }
    var chooseAudience: String { text("Choose audience", "Выберите аудиторию") }
    var explicitTargets: String { text("Permitted recipients", "Разрешённые адресаты") }
    var includeOrigin: String { text("Include this Pulsar", "Включая этот Пульсар") }
    var trackPolicy: String { text("Track action", "Действие с треком") }
    var contentPolicy: String { text("Content policy", "Правила содержимого") }
    var contentPolicyHelp: String { text(
        "Review and accept the current version when you send or replay.",
        "Ознакомьтесь и примите текущую версию при отправке или повторе.") }
    var inbox: String { text("Inbox", "Входящие") }
    var emptyInbox: String { text("No missed audio", "Нет пропущенного аудио") }
    var history: String { text("Delivery history", "История доставки") }
    var emptyHistory: String { text("History is empty", "История пуста") }
    var loadMore: String { text("Load more", "Загрузить ещё") }
    var loadMoreReceipts: String { text("Load more receipts", "Загрузить ещё квитанции") }
    var delivery: String { text("Delivery", "Доставка") }
    var receipt: String { text("Receipt", "Квитанция") }
    var availability: String { text("Availability", "Доступность") }
    var availableUntil: String { text("Available until", "Доступно до") }
    var replayDelivery: String { text("Replay mode", "Режим повтора") }
    var reportReason: String { text("Reason", "Причина") }
    var reportDetails: String { text("Details", "Подробности") }
    var submitReport: String { text("Submit report", "Отправить жалобу") }
    var confirmDelete: String { text("Delete this media permanently?", "Удалить это медиа навсегда?") }
    var confirmMute: String { text("Mute this sender?", "Заглушить этого отправителя?") }
    var cancel: String { text("Cancel", "Отмена") }
    var unsupported: String { text("Unsupported action", "Неподдерживаемое действие") }
    var targetSelectionHint: String { text(
        "Selection uses an opaque, expiring coordinator reference.",
        "Выбор использует непрозрачную временную ссылку координатора.") }
    var explicitReplayHint: String { text(
        "Starts playback only after this explicit action.",
        "Запускает воспроизведение только после этого явного действия.") }

    func state(_ value: PulsarTargetsInboxSurfaceState) -> String {
        switch value {
        case .loading: text("Loading", "Загрузка")
        case .ready: text("Up to date", "Актуально")
        case .stale: text("May be out of date", "Данные могут быть устаревшими")
        case .offline: text("Offline", "Нет сети")
        case .coordinatorError: text("Coordinator unavailable", "Координатор недоступен")
        }
    }

    func capability(_ value: String) -> String {
        switch value {
        case "known": text("Capabilities confirmed", "Возможности подтверждены")
        case "mixed": text("Some recipients differ or are offline", "Часть адресатов отличается или офлайн")
        default: text("Checked again when sending", "Будет проверено при отправке")
        }
    }

    func trackPolicy(_ value: String) -> String {
        switch value {
        case "clip": text("Send a clip", "Отправить клип")
        case "queue": text("Queue track", "Поставить трек в очередь")
        case "replace": text("Replace current track", "Заменить текущий трек")
        default: text("Track delivery is unavailable", "Доставка трека недоступна")
        }
    }

    func contentPolicy(_ value: String) -> String {
        switch value {
        case "current": text("Accepted", "Приняты")
        case "stale": text("Changed — review again", "Изменились — ознакомьтесь снова")
        default: text("Acceptance required", "Нужно принять")
        }
    }

    func availability(_ value: String) -> String {
        switch value {
        case "available": text("Available", "Доступно")
        case "dismissed": text("Dismissed", "Убрано")
        case "replayed": text("Replayed", "Повторено")
        case "expired": text("Expired", "Истекло")
        default: text("Unavailable", "Недоступно")
        }
    }

    func delivery(_ value: PulsarDeliveryMode) -> String {
        switch value {
        case .overlay: text("Overlay", "Поверх эфира")
        case .interrupt: text("Interrupt and resume", "Прервать и продолжить")
        case .afterCurrent: text("After current", "После текущего")
        }
    }

    func deliveryCounts(played: Int, other: Int) -> String {
        text("Played: \(played); other outcomes: \(other)",
             "Воспроизведено: \(played); другие исходы: \(other)")
    }
}
