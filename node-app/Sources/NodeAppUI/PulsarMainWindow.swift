import AppKit
import SwiftUI
import UniformTypeIdentifiers

public struct PulsarMainView: View {
    @Bindable private var model: PulsarShellModel
    @Bindable private var targetsInboxModel: PulsarTargetsInboxModel
    @Bindable private var streamTrackModel: PulsarStreamTrackModel
    private let actions: PulsarShellActions
    private let targetsInboxActions: PulsarTargetsInboxActions
    private let streamTrackActions: PulsarStreamTrackActions

    public init(
        model: PulsarShellModel,
        actions: PulsarShellActions,
        targetsInboxModel: PulsarTargetsInboxModel,
        targetsInboxActions: PulsarTargetsInboxActions,
        streamTrackModel: PulsarStreamTrackModel,
        streamTrackActions: PulsarStreamTrackActions
    ) {
        self.model = model
        self.actions = actions
        self.targetsInboxModel = targetsInboxModel
        self.targetsInboxActions = targetsInboxActions
        self.streamTrackModel = streamTrackModel
        self.streamTrackActions = streamTrackActions
    }

    public var body: some View {
        NavigationSplitView {
            PulsarSidebar(model: model)
                .navigationSplitViewColumnWidth(min: 190, ideal: 220, max: 280)
        } detail: {
            PulsarDetail(
                model: model, actions: actions,
                targetsInboxModel: targetsInboxModel,
                targetsInboxActions: targetsInboxActions,
                streamTrackModel: streamTrackModel,
                streamTrackActions: streamTrackActions)
        }
        .navigationSplitViewStyle(.balanced)
        .toolbar {
            PulsarToolbar(model: model, actions: actions)
        }
        .onExitCommand {
            if model.snapshot.recording == .recording {
                actions.cancelRecording()
            }
        }
        .frame(minWidth: 760, minHeight: 520)
    }
}

private struct PulsarSidebar: View {
    @Bindable var model: PulsarShellModel

    var body: some View {
        let copy = PulsarShellCopy(locale: model.locale)
        List(PulsarShellSection.allCases, selection: $model.selectedSection) { section in
            Label(copy.title(for: section), systemImage: symbol(for: section))
                .tag(section)
        }
        .navigationTitle(copy.text(.appName))
        .safeAreaInset(edge: .bottom) {
            Label(
                copy.connectionLabel(model.snapshot.connection),
                systemImage: copy.connectionSymbol(model.snapshot.connection)
            )
            .font(.callout)
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.horizontal, 12)
            .padding(.vertical, 10)
            .accessibilityElement(children: .combine)
        }
    }

    private func symbol(for section: PulsarShellSection) -> String {
        switch section {
        case .home: "house"
        case .airs: "person.3.sequence"
        case .inbox: "tray.full"
        case .create: "plus.circle"
        case .join: "person.2"
        case .tryLocally: "waveform.circle"
        case .soundboard: "square.grid.3x3"
        case .automation: "calendar.badge.clock"
        case .history: "clock.arrow.circlepath"
        case .settings: "gear"
        }
    }
}

private struct PulsarDetail: View {
    let model: PulsarShellModel
    let actions: PulsarShellActions
    let targetsInboxModel: PulsarTargetsInboxModel
    let targetsInboxActions: PulsarTargetsInboxActions
    let streamTrackModel: PulsarStreamTrackModel
    let streamTrackActions: PulsarStreamTrackActions

    var body: some View {
        switch model.selectedSection {
        case .home:
            PulsarHomeView(
                model: model, actions: actions, targetsInboxModel: targetsInboxModel)
        case .airs:
            PulsarAirManagementView(model: model, actions: actions)
        case .inbox:
            PulsarTargetsInboxView(
                shellModel: model,
                model: targetsInboxModel,
                actions: targetsInboxActions)
        case .create:
            PulsarIdentityFlowView(
                mode: .create,
                titleKey: .createTitle,
                bodyKey: .createBody,
                symbol: "plus.circle",
                model: model,
                actions: actions
            )
        case .join:
            PulsarIdentityFlowView(
                mode: .join,
                titleKey: .joinTitle,
                bodyKey: .joinBody,
                symbol: "person.2",
                model: model,
                actions: actions
            )
        case .tryLocally:
            PulsarSelfTestView(model: model, actions: actions)
        case .soundboard:
            PulsarSoundboardView(model: model, actions: actions)
        case .automation:
            PulsarAutomationAdminView(model: model, actions: actions)
        case .history:
            PulsarHistoryView(
                model: model, actions: actions,
                streamTrackModel: streamTrackModel,
                streamTrackActions: streamTrackActions)
        case .settings:
            PulsarSettingsView(model: model, actions: actions)
        }
    }
}

private struct PulsarSoundboardView: View {
    let model: PulsarShellModel
    let actions: PulsarShellActions
    @State private var showingImporter = false
    @State private var rightsAcknowledged = false

    var body: some View {
        let state = model.snapshot.soundboard
        VStack(alignment: .leading, spacing: 14) {
            HStack {
                Text(localized("Saved cues", "Сохранённые сигналы")).font(.title2.bold())
                Spacer()
                Button(localized("Refresh", "Обновить"), action: actions.refreshSoundboard)
                Button(localized("Automation schedules →", "Расписания automation →")) {
                    model.selectedSection = .automation
                }
            }
            HStack {
                Picker(localized("Target", "Цель"), selection: Binding(
                    get: { state.route }, set: actions.setSoundboardRoute)) {
                    ForEach(PulsarRouteTarget.allCases) { Text($0.rawValue).tag($0) }
                }
                Picker(localized("Delivery", "Доставка"), selection: Binding(
                    get: { state.delivery }, set: actions.setSoundboardDelivery)) {
                    ForEach(PulsarDeliveryMode.allCases) { Text($0.rawValue).tag($0) }
                }
                Toggle(localized("Include this Pulsar", "Включить этот Pulsar"), isOn: Binding(
                    get: { state.includeOrigin }, set: actions.setSoundboardIncludeOrigin))
            }
            if state.cues.isEmpty {
                ContentUnavailableView(
                    localized("No saved cues", "Нет сохранённых сигналов"),
                    systemImage: "square.grid.3x3")
            } else {
                List(state.cues) { cue in
                    PulsarSoundboardCueRow(
                        cue: cue, selected: cue.id == state.selectedCueID,
                        busy: state.busy, locale: model.locale, actions: actions)
                }
            }
            HStack {
                Toggle(localized("I have rights to upload this audio", "У меня есть права на загрузку"),
                       isOn: $rightsAcknowledged)
                Button(localized("Add audio cue…", "Добавить аудиосигнал…")) { showingImporter = true }
                    .disabled(!rightsAcknowledged || state.busy)
            }
            if let outcome = state.outcome {
                Label(outcome.replacingOccurrences(of: "_", with: " "), systemImage: "checkmark.circle")
                    .foregroundStyle(.green)
            }
            if let failure = state.failure {
                Label(failure.replacingOccurrences(of: "_", with: " "), systemImage: "exclamationmark.triangle")
                    .foregroundStyle(.orange)
            }
        }
        .padding(24)
        .navigationTitle(PulsarShellCopy(locale: model.locale).text(.soundboard))
        .fileImporter(isPresented: $showingImporter, allowedContentTypes: [.audio], allowsMultipleSelection: false) {
            guard case .success(let urls) = $0, let url = urls.first else { return }
            actions.createSoundboardCue(url, rightsAcknowledged: rightsAcknowledged)
        }
    }

    private func localized(_ en: String, _ ru: String) -> String { model.locale == .ru ? ru : en }
}

private struct PulsarSoundboardCueRow: View {
    let cue: PulsarSoundboardCue
    let selected: Bool
    let busy: Bool
    let locale: PulsarShellLocale
    let actions: PulsarShellActions
    @State private var title = ""

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            HStack {
                Button {
                    actions.selectSoundboardCue(cue.id)
                } label: {
                    Label(cue.title, systemImage: selected ? "checkmark.circle.fill" : "circle")
                }
                .buttonStyle(.borderless)
                Spacer()
                Text(cue.shortcutLabel ?? localized("No hotkey", "Без хоткея"))
                    .font(.caption).foregroundStyle(.secondary)
                Text(cue.shortcutStatus).font(.caption)
            }
            HStack {
                Button(localized("Trigger", "Запустить")) { actions.triggerSoundboardCue(cue.id) }
                    .buttonStyle(.borderedProminent)
                TextField(localized("Cue name", "Название"), text: $title)
                    .onAppear { title = cue.title }
                    .onSubmit { actions.renameSoundboardCue(cue.id, title: title) }
                Button(localized("Rename", "Переименовать")) {
                    actions.renameSoundboardCue(cue.id, title: title)
                }
                Button("↑") { actions.moveSoundboardCue(cue.id, delta: -1) }
                    .accessibilityLabel(localized("Move up", "Переместить вверх"))
                Button("↓") { actions.moveSoundboardCue(cue.id, delta: 1) }
                    .accessibilityLabel(localized("Move down", "Переместить вниз"))
                Button(localized("Hotkey", "Хоткей")) { actions.cycleSoundboardShortcut(cue.id) }
                Button(localized("Delete", "Удалить"), role: .destructive) {
                    actions.deleteSoundboardCue(cue.id)
                }
            }
            .disabled(busy)
        }
        .accessibilityElement(children: .contain)
    }

    private func localized(_ en: String, _ ru: String) -> String { locale == .ru ? ru : en }
}

private struct PulsarAutomationAdminView: View {
    let model: PulsarShellModel
    let actions: PulsarShellActions
    @State private var featureTimezone = "UTC"
    @State private var featureQuietHours = ""
    @State private var scheduleName = ""
    @State private var scheduleTimezone = "UTC"
    @State private var scheduleWeekdays = "Mon,Tue,Wed,Thu,Fri"
    @State private var scheduleLocalTime = "09:00"
    @State private var scheduleQuietHours = ""
    @State private var principalName = ""

    var body: some View {
        let state = model.snapshot.automation
        ScrollView {
            VStack(alignment: .leading, spacing: 16) {
                HStack {
                    Text(localized("Automation administration", "Управление автоматизацией"))
                        .font(.title2.bold())
                    Spacer()
                    Button(localized("Manual soundboard →", "Ручной саундборд →")) {
                        model.selectedSection = .soundboard
                    }
                    Button(localized("Refresh", "Обновить"), action: actions.refreshAutomation)
                }

                if !state.available {
                    ContentUnavailableView(
                        localized("Automation controls unavailable", "Управление автоматизацией недоступно"),
                        systemImage: "lock.shield",
                        description: Text(localized(
                            "Configuration remains hidden until an authorized refresh succeeds.",
                            "Конфигурация скрыта до успешного авторизованного обновления.")))
                } else {
                    PulsarAutomationFeatureSection(
                        state: state, locale: model.locale,
                        timezone: $featureTimezone, quietHours: $featureQuietHours,
                        actions: actions)
                    PulsarAutomationScheduleSection(
                        state: state, locale: model.locale,
                        name: $scheduleName, timezone: $scheduleTimezone,
                        weekdays: $scheduleWeekdays, localTime: $scheduleLocalTime,
                        quietHours: $scheduleQuietHours, actions: actions)
                    PulsarAutomationPrincipalSection(
                        state: state, locale: model.locale,
                        principalName: $principalName, actions: actions)
                    PulsarAutomationHistorySection(
                        state: state, locale: model.locale, actions: actions)
                }

                if let outcome = state.outcome {
                    Label(displayCode(outcome), systemImage: "checkmark.circle")
                        .foregroundStyle(.green)
                        .accessibilityLabel(localized("Automation action completed", "Действие автоматизации выполнено"))
                        .accessibilityValue(displayCode(outcome))
                }
                if let failure = state.failure {
                    Label(displayCode(failure), systemImage: "exclamationmark.triangle")
                        .foregroundStyle(.orange)
                        .accessibilityLabel(localized("Automation action failed", "Ошибка действия автоматизации"))
                        .accessibilityValue(displayCode(failure))
                }
            }
            .padding(24)
        }
        .navigationTitle(PulsarShellCopy(locale: model.locale).text(.automation))
        .onAppear { loadEditors(from: state) }
        .onChange(of: state.feature) { _, _ in loadFeature(from: state) }
        .onChange(of: state.schedules) { _, _ in loadSchedule(from: state) }
        .onChange(of: state.selectedScheduleID) { _, _ in loadSchedule(from: state) }
        .confirmationDialog(
            confirmationTitle(state.confirmation),
            isPresented: Binding(
                get: { state.confirmation != nil },
                set: { if !$0 { actions.cancelAutomationConfirmation() } })
        ) {
            Button(confirmationButton(state.confirmation), role: confirmationRole(state.confirmation)) {
                actions.confirmAutomationAction(principalName: principalName)
            }
            Button(localized("Cancel", "Отмена"), role: .cancel) {
                actions.cancelAutomationConfirmation()
            }
        } message: {
            Text(localized(
                "The coordinator will re-check your current role, revision and policy before applying this action.",
                "Координатор повторно проверит роль, ревизию и политику перед выполнением действия."))
        }
    }

    private func loadEditors(from state: PulsarAutomationState) {
        loadFeature(from: state)
        loadSchedule(from: state)
    }

    private func loadFeature(from state: PulsarAutomationState) {
        guard let feature = state.feature else { return }
        featureTimezone = feature.timezone
        featureQuietHours = feature.quietHours
    }

    private func loadSchedule(from state: PulsarAutomationState) {
        guard let id = state.selectedScheduleID,
              let schedule = state.schedules.first(where: { $0.id == id }) else {
            scheduleName = ""
            scheduleTimezone = state.feature?.timezone ?? "UTC"
            scheduleWeekdays = "Mon,Tue,Wed,Thu,Fri"
            scheduleLocalTime = "09:00"
            scheduleQuietHours = ""
            return
        }
        scheduleName = schedule.displayName
        scheduleTimezone = schedule.timezone
        scheduleWeekdays = schedule.weekdays
        scheduleLocalTime = schedule.localTime
        scheduleQuietHours = schedule.quietHours
    }

    private func confirmationTitle(_ value: PulsarAutomationConfirmation?) -> String {
        switch value {
        case .scheduleToggle: localized("Change schedule state?", "Изменить состояние расписания?")
        case .scheduleDelete: localized("Delete schedule?", "Удалить расписание?")
        case .principalIssue: localized("Issue a one-time secret?", "Выдать одноразовый секрет?")
        case .principalRevoke: localized("Revoke principal?", "Отозвать principal?")
        case .automationToggle: localized("Change automation state?", "Изменить состояние автоматизации?")
        case .emergencyDisable: localized("Emergency-disable automation?", "Экстренно отключить автоматизацию?")
        case .historyCancel: localized("Cancel pending delivery?", "Отменить ожидающую доставку?")
        case nil: localized("Confirm action", "Подтвердите действие")
        }
    }

    private func confirmationButton(_ value: PulsarAutomationConfirmation?) -> String {
        value == .principalIssue ? localized("Issue once", "Выдать один раз")
            : localized("Confirm", "Подтвердить")
    }

    private func confirmationRole(_ value: PulsarAutomationConfirmation?) -> ButtonRole? {
        value == .principalIssue ? nil : .destructive
    }

    private func localized(_ en: String, _ ru: String) -> String { model.locale == .ru ? ru : en }
    private func displayCode(_ value: String) -> String { value.replacingOccurrences(of: "_", with: " ") }
}

private struct PulsarAutomationFeatureSection: View {
    let state: PulsarAutomationState
    let locale: PulsarShellLocale
    @Binding var timezone: String
    @Binding var quietHours: String
    let actions: PulsarShellActions

    var body: some View {
        GroupBox(localized("Orbit automation", "Автоматизация орбиты")) {
            VStack(alignment: .leading, spacing: 10) {
                if let feature = state.feature {
                    LabeledContent(localized("Automation", "Автоматизация"), value: feature.automationEnabled ? localized("Enabled", "Включена") : localized("Disabled", "Отключена"))
                    LabeledContent(localized("Emergency stop", "Экстренная остановка"), value: feature.emergencyDisabled ? localized("Active", "Активна") : localized("Clear", "Не активна"))
                    LabeledContent(localized("Manual soundboard", "Ручной саундборд"), value: feature.soundboardEnabled ? localized("Available", "Доступен") : localized("Disabled by policy", "Отключён политикой"))
                    LabeledContent(localized("Policy", "Политика"), value: feature.policyVersion)
                    TextField(localized("IANA timezone", "Часовой пояс IANA"), text: $timezone)
                        .accessibilityHint(localized("For example Asia/Yerevan", "Например Asia/Yerevan"))
                    TextField(localized("Quiet hours", "Тихие часы"), text: $quietHours)
                        .accessibilityHint(localized("Example: Mon 22:00-07:00; Fri 23:00-08:00", "Пример: Mon 22:00-07:00; Fri 23:00-08:00"))
                    HStack {
                        Button(localized("Save policy settings", "Сохранить настройки политики")) {
                            actions.saveAutomationFeature(timezone: timezone, quietHours: quietHours)
                        }
                        Button(feature.automationEnabled ? localized("Disable automation", "Отключить автоматизацию") : localized("Enable automation", "Включить автоматизацию")) {
                            actions.requestAutomationAction(.automationToggle)
                        }
                        Button(localized("Emergency disable", "Экстренно отключить"), role: .destructive) {
                            actions.requestAutomationAction(.emergencyDisable)
                        }
                        .disabled(feature.emergencyDisabled)
                    }
                    .disabled(state.busy)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private func localized(_ en: String, _ ru: String) -> String { locale == .ru ? ru : en }
}

private struct PulsarAutomationScheduleSection: View {
    let state: PulsarAutomationState
    let locale: PulsarShellLocale
    @Binding var name: String
    @Binding var timezone: String
    @Binding var weekdays: String
    @Binding var localTime: String
    @Binding var quietHours: String
    let actions: PulsarShellActions

    var body: some View {
        GroupBox(localized("Schedules", "Расписания")) {
            VStack(alignment: .leading, spacing: 10) {
                Picker(localized("Selected schedule", "Выбранное расписание"), selection: Binding(
                    get: { state.selectedScheduleID },
                    set: actions.selectAutomationSchedule)) {
                    Text(localized("New schedule", "Новое расписание")).tag(String?.none)
                    ForEach(state.schedules) { schedule in
                        Text(schedule.displayName).tag(Optional(schedule.id))
                    }
                }
                if let selected {
                    LabeledContent(localized("State", "Состояние"), value: selected.enabled ? localized("Enabled", "Включено") : localized("Disabled", "Отключено"))
                    LabeledContent(localized("Audience", "Аудитория"), value: selected.audience)
                    if let nextRun = selected.nextRun {
                        HStack {
                            Text(localized("Next fire", "Следующий запуск"))
                            Spacer()
                            Text(nextRun, format: .dateTime.year().month().day().hour().minute().timeZone())
                        }
                        Text(selected.quietHoursSkip
                            ? localized("This candidate is skipped by quiet hours.", "Этот запуск пропускается из-за тихих часов.")
                            : localized("Spring gaps do not fire; the earliest fall-fold instant wins.", "Весенний разрыв не запускается; при осеннем повторе выбирается первый момент."))
                            .font(.caption).foregroundStyle(selected.quietHoursSkip ? .orange : .secondary)
                    } else {
                        Text(localized("No next fire while this schedule is disabled.", "Нет следующего запуска, пока расписание отключено."))
                            .font(.caption).foregroundStyle(.secondary)
                    }
                }
                Grid(alignment: .leading, horizontalSpacing: 12, verticalSpacing: 8) {
                    editorRow(localized("Name", "Название")) { TextField("Morning", text: $name) }
                    editorRow(localized("IANA timezone", "Часовой пояс IANA")) { TextField("Asia/Yerevan", text: $timezone) }
                    editorRow(localized("Weekdays", "Дни недели")) { TextField("Mon,Tue,Wed,Thu,Fri", text: $weekdays) }
                    editorRow(localized("Local time", "Местное время")) { TextField("09:00", text: $localTime) }
                    editorRow(localized("Extra quiet hours", "Доп. тихие часы")) { TextField("Fri 23:00-08:00", text: $quietHours) }
                }
                HStack {
                    Button(selected == nil ? localized("Create disabled schedule", "Создать отключённое расписание") : localized("Save schedule", "Сохранить расписание")) {
                        actions.saveAutomationSchedule(.init(
                            name: name, timezone: timezone, weekdays: weekdays,
                            localTime: localTime, quietHours: quietHours))
                    }
                    Button(selected?.enabled == true ? localized("Disable", "Отключить") : localized("Enable", "Включить")) {
                        actions.requestAutomationAction(.scheduleToggle)
                    }.disabled(selected == nil)
                    Button(localized("Delete", "Удалить"), role: .destructive) {
                        actions.requestAutomationAction(.scheduleDelete)
                    }.disabled(selected == nil)
                }
                .disabled(state.busy || state.cueCount == 0)
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private var selected: PulsarAutomationScheduleState? {
        state.schedules.first { $0.id == state.selectedScheduleID }
    }

    private func editorRow<Content: View>(_ title: String, @ViewBuilder content: () -> Content) -> some View {
        GridRow { Text(title); content() }
    }

    private func localized(_ en: String, _ ru: String) -> String { locale == .ru ? ru : en }
}

private struct PulsarAutomationPrincipalSection: View {
    let state: PulsarAutomationState
    let locale: PulsarShellLocale
    @Binding var principalName: String
    let actions: PulsarShellActions

    var body: some View {
        GroupBox(localized("Scoped principals", "Ограниченные principals")) {
            VStack(alignment: .leading, spacing: 10) {
                Picker(localized("Selected principal", "Выбранный principal"), selection: Binding(
                    get: { state.selectedPrincipalID },
                    set: { if let id = $0 { actions.selectAutomationPrincipal(id) } })) {
                    Text(localized("None", "Нет")).tag(String?.none)
                    ForEach(state.principals) { principal in
                        Text(principal.displayName).tag(Optional(principal.id))
                    }
                }
                if let selected {
                    LabeledContent(localized("Permission", "Разрешение"), value: selected.permission)
                    LabeledContent(localized("Cue scope", "Область сигналов"), value: String(selected.allowedCueCount))
                    LabeledContent(localized("Audience scope", "Область аудитории"), value: selected.allowedAudiences.joined(separator: ", "))
                    HStack { Text(localized("Expires", "Истекает")); Spacer(); Text(selected.expiresAt, format: .dateTime.year().month().day().hour().minute()) }
                }
                HStack {
                    TextField(localized("New principal name", "Имя нового principal"), text: $principalName)
                    Button(localized("Issue one-time secret", "Выдать одноразовый секрет")) {
                        actions.requestAutomationAction(.principalIssue)
                    }
                    Button(localized("Revoke", "Отозвать"), role: .destructive) {
                        actions.requestAutomationAction(.principalRevoke)
                    }.disabled(selected == nil)
                }.disabled(state.busy || state.cueCount == 0)
                if state.secretAvailable {
                    HStack {
                        Label(localized("One-time secret is available in memory", "Одноразовый секрет доступен в памяти"), systemImage: "key")
                            .accessibilityLabel(localized("One-time secret available", "Одноразовый секрет доступен"))
                        Spacer()
                        Button(localized("Copy for 60 seconds", "Копировать на 60 секунд"), action: actions.copyAutomationSecret)
                        Button(localized("Hide permanently", "Скрыть навсегда"), role: .destructive, action: actions.hideAutomationSecret)
                    }
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private var selected: PulsarAutomationPrincipalState? {
        state.principals.first { $0.id == state.selectedPrincipalID }
    }
    private func localized(_ en: String, _ ru: String) -> String { locale == .ru ? ru : en }
}

private struct PulsarAutomationHistorySection: View {
    let state: PulsarAutomationState
    let locale: PulsarShellLocale
    let actions: PulsarShellActions

    var body: some View {
        GroupBox(localized("Automation history", "История автоматизации")) {
            VStack(alignment: .leading, spacing: 10) {
                Picker(localized("Selected event", "Выбранное событие"), selection: Binding(
                    get: { state.selectedHistoryID },
                    set: { if let id = $0 { actions.selectAutomationHistory(id) } })) {
                    Text(localized("None", "Нет")).tag(String?.none)
                    ForEach(state.history) { item in Text(item.title).tag(Optional(item.id)) }
                }
                if let selected {
                    LabeledContent(localized("Status", "Статус"), value: selected.status)
                    LabeledContent(localized("Trigger", "Триггер"), value: selected.triggerKind)
                    if let actor = selected.actorLabel { LabeledContent(localized("Actor", "Инициатор"), value: actor) }
                    if let schedule = selected.scheduleLabel { LabeledContent(localized("Schedule", "Расписание"), value: schedule) }
                    if let reason = selected.reasonCode { LabeledContent(localized("Reason", "Причина"), value: reason) }
                    HStack { Text(localized("Occurred", "Время")); Spacer(); Text(selected.occurredAt, format: .dateTime.year().month().day().hour().minute()) }
                    Button(localized("Cancel pending delivery", "Отменить ожидающую доставку"), role: .destructive) {
                        actions.requestAutomationAction(.historyCancel)
                    }.disabled(!selected.canCancel || state.busy)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
        }
    }

    private var selected: PulsarAutomationHistoryState? {
        state.history.first { $0.id == state.selectedHistoryID }
    }
    private func localized(_ en: String, _ ru: String) -> String { locale == .ru ? ru : en }
}

private struct PulsarToolbar: ToolbarContent {
    let model: PulsarShellModel
    let actions: PulsarShellActions

    var body: some ToolbarContent {
        let copy = PulsarShellCopy(locale: model.locale)
        ToolbarItemGroup {
            Button {
                actions.toggleRecording()
            } label: {
                Label(
                    model.snapshot.recording == .recording
                        ? copy.text(.stopRecording)
                        : copy.text(.startRecording),
                    systemImage: copy.recordingSymbol(model.snapshot.recording)
                )
            }
            .disabled(
                model.snapshot.recording == .processing
                    || isSelfTestBusy(model.snapshot.selfTestState)
                    || (!model.snapshot.recordingAvailable && model.snapshot.recording != .recording))
            .keyboardShortcut("r", modifiers: [.command, .shift])
            .accessibilityLabel(copy.recordingLabel(model.snapshot.recording))

            Button {
                model.selectedSection = .settings
            } label: {
                Label(copy.text(.settings), systemImage: "gear")
            }
            .keyboardShortcut(",", modifiers: .command)
        }
    }

    private func isSelfTestBusy(_ state: PulsarSelfTestState) -> Bool {
        ![.idle, .reviewingDraft, .failed].contains(state)
    }
}

private struct PulsarHomeView: View {
    let model: PulsarShellModel
    let actions: PulsarShellActions
    let targetsInboxModel: PulsarTargetsInboxModel

    var body: some View {
        let copy = PulsarShellCopy(locale: model.locale)
        ScrollView {
            VStack(alignment: .leading, spacing: 20) {
                PulsarStateBanner(model: model)
                Text(copy.text(.primaryActions))
                    .font(.title2.bold())
                ViewThatFits {
                    HStack(spacing: 12) { actionCards(copy: copy) }
                    VStack(spacing: 12) { actionCards(copy: copy) }
                }
                Text(copy.text(.status))
                    .font(.title2.bold())
                LazyVGrid(columns: [GridItem(.adaptive(minimum: 210), spacing: 12)], spacing: 12) {
                    PulsarStatusCard(
                        title: copy.text(.presence),
                        value: model.snapshot.presenceSummary
                            ?? copy.connectionLabel(model.snapshot.connection),
                        symbol: copy.connectionSymbol(model.snapshot.connection)
                    )
                    PulsarStatusCard(
                        title: copy.text(.routing),
                        value: model.snapshot.routeName ?? copy.text(.noRoute),
                        symbol: "hifispeaker.2"
                    )
                    PulsarStatusCard(
                        title: copy.text(.nowPlaying),
                        value: model.snapshot.nowPlaying ?? copy.text(.silence),
                        symbol: "waveform"
                    )
                }
                PulsarLocalControls(model: model, actions: actions)
                PulsarOutgoingDraftsView(
                    model: model, actions: actions, targetsInboxModel: targetsInboxModel)
                PulsarHistoryPreview(model: model)
            }
            .padding(24)
            .frame(maxWidth: .infinity, alignment: .leading)
        }
        .navigationTitle(copy.text(.home))
    }

    @ViewBuilder
    private func actionCards(copy: PulsarShellCopy) -> some View {
        PulsarActionCard(
            title: copy.text(.create),
            detail: copy.text(.createBody),
            symbol: "plus.circle",
            shortcut: "⌘1"
        ) {
            model.selectedSection = .create
        }
        .keyboardShortcut("1", modifiers: .command)
        PulsarActionCard(
            title: copy.text(.join),
            detail: copy.text(.joinBody),
            symbol: "person.2",
            shortcut: "⌘2"
        ) {
            model.selectedSection = .join
        }
        .keyboardShortcut("2", modifiers: .command)
        PulsarActionCard(
            title: copy.text(.tryLocally),
            detail: copy.text(.tryBody),
            symbol: "waveform.circle",
            shortcut: "⇧⌘T"
        ) {
            model.selectedSection = .tryLocally
        }
        .keyboardShortcut("t", modifiers: [.command, .shift])
    }
}

private struct PulsarStateBanner: View {
    let model: PulsarShellModel

    var body: some View {
        let copy = PulsarShellCopy(locale: model.locale)
        VStack(alignment: .leading, spacing: 8) {
            Label(
                copy.connectionLabel(model.snapshot.connection),
                systemImage: copy.connectionSymbol(model.snapshot.connection)
            )
            .font(.headline)
            if case .unpaired = model.snapshot.connection {
                Text(copy.text(.unpairedHelp))
            } else if model.snapshot.connection != .online {
                Text(copy.text(.degradedHelp))
            }
            if model.snapshot.recording == .recording {
                Label(copy.text(.recordingHelp), systemImage: "record.circle.fill")
                    .font(.callout.bold())
            }
        }
        .padding(14)
        .frame(maxWidth: .infinity, alignment: .leading)
        .background(.quaternary, in: .rect(cornerRadius: 12))
        .accessibilityElement(children: .combine)
    }
}

private struct PulsarActionCard: View {
    let title: String
    let detail: String
    let symbol: String
    let shortcut: String
    let action: () -> Void

    var body: some View {
        Button(action: action) {
            VStack(alignment: .leading, spacing: 8) {
                Label(title, systemImage: symbol)
                    .font(.headline)
                Text(detail)
                    .font(.callout)
                    .foregroundStyle(.secondary)
                    .multilineTextAlignment(.leading)
                Text(shortcut)
                    .font(.caption.monospaced())
                    .foregroundStyle(.secondary)
            }
            .frame(maxWidth: .infinity, minHeight: 112, alignment: .leading)
            .padding(14)
        }
        .buttonStyle(.plain)
        .background(.background, in: .rect(cornerRadius: 12))
        .overlay {
            RoundedRectangle(cornerRadius: 12)
                .stroke(.separator, lineWidth: 1)
        }
        .accessibilityLabel(title)
        .accessibilityHint(detail)
    }
}

private struct PulsarStatusCard: View {
    let title: String
    let value: String
    let symbol: String

    var body: some View {
        VStack(alignment: .leading, spacing: 8) {
            Label(title, systemImage: symbol)
                .font(.headline)
            Text(value)
                .font(.body)
                .foregroundStyle(.secondary)
                .lineLimit(2)
        }
        .frame(maxWidth: .infinity, minHeight: 84, alignment: .leading)
        .padding(14)
        .background(.quaternary, in: .rect(cornerRadius: 12))
        .accessibilityElement(children: .combine)
    }
}

private struct PulsarLocalControls: View {
    @Bindable var model: PulsarShellModel
    let actions: PulsarShellActions

    var body: some View {
        let copy = PulsarShellCopy(locale: model.locale)
        VStack(alignment: .leading, spacing: 14) {
            Text(copy.text(.localControls))
                .font(.title2.bold())
            Picker(copy.text(.dnd), selection: Binding(
                get: { model.snapshot.dndMode },
                set: { actions.setDND($0) }
            )) {
                ForEach(PulsarDNDMode.allCases) { mode in
                    Text(copy.dndLabel(mode)).tag(mode)
                        .disabled(mode == .mutedUntil)
                }
            }
            .disabled(!model.snapshot.connection.isPaired)
            HStack {
                Text(copy.text(.volume))
                Slider(
                    value: Binding(
                        get: { Double(model.snapshot.volume) },
                        set: { actions.setVolume(Int($0.rounded())) }
                    ),
                    in: 0...100,
                    step: 1
                )
                Text(model.snapshot.volume, format: .number)
                    .monospacedDigit()
                    .frame(minWidth: 28, alignment: .trailing)
            }
            .disabled(!model.snapshot.connection.isPaired)
            Label(
                copy.recordingLabel(model.snapshot.recording),
                systemImage: copy.recordingSymbol(model.snapshot.recording)
            )
            .accessibilityElement(children: .combine)
            if model.snapshot.recording == .recording {
                ProgressView(value: Double(model.snapshot.recordingMeter))
                    .accessibilityLabel(copy.text(.recording))
            }
            Label(
                "\(model.snapshot.recordingShortcut.displayValue) — \(copy.recordingShortcutLabel(model.snapshot.recordingShortcutState))",
                systemImage: shortcutSymbol(model.snapshot.recordingShortcutState)
            )
            .font(.callout)
            .accessibilityElement(children: .combine)
        }
    }

    private func shortcutSymbol(_ state: PulsarRecordingShortcutState) -> String {
        switch state {
        case .registered: "keyboard"
        case .conflict, .unavailable: "exclamationmark.triangle"
        case .suspended: "pause.circle"
        case .inactive: "keyboard"
        }
    }
}

private struct PulsarHistoryPreview: View {
    let model: PulsarShellModel

    var body: some View {
        let copy = PulsarShellCopy(locale: model.locale)
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text(copy.text(.historyTitle))
                    .font(.title2.bold())
                Spacer()
                Button(copy.text(.history)) { model.selectedSection = .history }
            }
            if let item = model.snapshot.history.first {
                PulsarHistoryRow(item: item, locale: model.locale)
            } else {
                ContentUnavailableView(
                    copy.text(.noHistory),
                    systemImage: "clock.arrow.circlepath"
                )
                .frame(maxWidth: .infinity, minHeight: 100)
            }
        }
    }
}

private struct PulsarHistoryView: View {
    let model: PulsarShellModel
    let actions: PulsarShellActions
    let streamTrackModel: PulsarStreamTrackModel
    let streamTrackActions: PulsarStreamTrackActions

    var body: some View {
        let copy = PulsarShellCopy(locale: model.locale)
        VSplitView {
            PulsarStreamTrackView(
                model: streamTrackModel,
                locale: model.locale,
                actions: streamTrackActions)
                .frame(minHeight: 300)
            Group {
                if model.snapshot.history.isEmpty {
                    ContentUnavailableView(copy.text(.noHistory), systemImage: "clock.arrow.circlepath")
                } else {
                    List(model.snapshot.history) { item in
                        PulsarHistoryRow(item: item, locale: model.locale, actions: actions)
                    }
                }
            }
            .frame(minHeight: 150)
        }
        .navigationTitle(copy.text(.historyTitle))
        .safeAreaInset(edge: .top) {
            VStack(alignment: .leading, spacing: 4) {
                if let outcome = model.snapshot.phaseOneActionOutcome {
                    Label(copy.historyActionMessage(outcome), systemImage: "checkmark.circle")
                        .foregroundStyle(.green)
                        .accessibilityElement(children: .combine)
                }
                if let failure = model.snapshot.phaseOneFailure {
                    Label(copy.historyActionMessage(failure), systemImage: "exclamationmark.triangle")
                        .foregroundStyle(.orange)
                        .accessibilityElement(children: .combine)
                }
            }
            .padding(.horizontal)
        }
        .toolbar {
            Button(copy.text(.refresh)) { actions.refreshPhaseOneData() }
        }
    }
}

private struct PulsarHistoryRow: View {
    let item: PulsarHistoryItem
    let locale: PulsarShellLocale
    var actions: PulsarShellActions?
    @State private var reportReason = PulsarModerationReason.spam
    @State private var reportDetails = ""
    @State private var pendingAction: PulsarHistoryAction?

    var body: some View {
        let copy = PulsarShellCopy(locale: locale)
        VStack(alignment: .leading, spacing: 4) {
            Text(item.title).font(.headline)
            Text(item.detail).foregroundStyle(.secondary)
            if let requested = item.requestedDelivery,
               let effective = item.effectiveDelivery {
                Text("\(requested) → \(effective)")
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            if let downgrade = item.downgradeReason {
                Text(downgrade)
                    .font(.caption)
                    .foregroundStyle(.orange)
            }
            if let automation = item.automation {
                VStack(alignment: .leading, spacing: 2) {
                    Text("Automation: \(automation.triggerKind) · \(automation.cueLabel ?? item.title)")
                    if let actor = automation.principalLabel { Text("By: \(actor)") }
                    if let schedule = automation.scheduleLabel { Text("Schedule: \(schedule)") }
                    if let reason = automation.reasonCode { Text(reason).foregroundStyle(.orange) }
                    if automation.canDisableSchedule || automation.canRevokePrincipal
                        || automation.canEmergencyDisable {
                        Button(locale == .ru ? "Открыть управление automation →" : "Open automation controls →") {
                            actions?.openAutomationAdmin()
                        }
                        .buttonStyle(.borderless)
                        .disabled(actions == nil)
                    }
                }
                .font(.caption)
                .foregroundStyle(.secondary)
            }
            Text(item.occurredAt, format: .dateTime.year().month().day().hour().minute())
                .font(.caption)
                .foregroundStyle(.secondary)
            if let actions, !item.allowedActions.isEmpty {
                HStack {
                    ForEach(item.allowedActions.filter { $0 != .report }, id: \.rawValue) { action in
                        Button(label(action)) {
                            if action == .delete || action == .blockActor {
                                pendingAction = action
                            } else {
                                actions.performHistoryAction(item.id, action: action)
                            }
                        }
                        .buttonStyle(.borderless)
                    }
                }
                if item.allowedActions.contains(.report) {
                    HStack {
                        Picker(copy.text(.reportReason), selection: $reportReason) {
                            ForEach(PulsarModerationReason.allCases) { reason in
                                Text(copy.moderationReasonLabel(reason)).tag(reason)
                            }
                        }
                        .pickerStyle(.menu)
                        .accessibilityLabel(copy.text(.reportReason))
                        TextField(copy.text(.reportDetails), text: $reportDetails)
                            .textFieldStyle(.roundedBorder)
                            .accessibilityLabel(copy.text(.reportDetails))
                            .onChange(of: reportDetails) { _, value in
                                reportDetails = boundedReportDetails(value)
                            }
                        Button(copy.text(.submitReport)) {
                            actions.performHistoryAction(
                                item.id,
                                request: .init(
                                    action: .report,
                                    reason: reportReason,
                                    details: reportDetails.trimmingCharacters(
                                        in: .whitespacesAndNewlines)))
                        }
                        .buttonStyle(.borderedProminent)
                    }
                }
            }
        }
        .accessibilityElement(children: item.allowedActions.isEmpty ? .combine : .contain)
        .confirmationDialog(
            confirmationTitle(copy),
            isPresented: Binding(
                get: { pendingAction != nil },
                set: { if !$0 { pendingAction = nil } }),
            titleVisibility: .visible
        ) {
            if let pendingAction, let actions {
                Button(label(pendingAction), role: pendingAction == .delete ? .destructive : nil) {
                    actions.performHistoryAction(item.id, action: pendingAction)
                    self.pendingAction = nil
                }
            }
            Button(copy.text(.cancel), role: .cancel) { pendingAction = nil }
        }
    }

    private func label(_ action: PulsarHistoryAction) -> String {
        let copy = PulsarShellCopy(locale: locale)
        switch action {
        case .delete: return copy.text(.deleteHistory)
        case .replay: return copy.text(.replay)
        case .report: return copy.text(.report)
        case .blockActor: return copy.text(.blockSender)
        }
    }

    private func confirmationTitle(_ copy: PulsarShellCopy) -> String {
        pendingAction == .delete ? copy.text(.confirmDelete) : copy.text(.confirmBlock)
    }

    private func boundedReportDetails(_ value: String) -> String {
        var bytes = 0
        return String(value.prefix { character in
            let count = String(character).utf8.count
            guard bytes + count <= 2_000 else { return false }
            bytes += count
            return true
        })
    }
}

private struct PulsarOutgoingDraftsView: View {
    let model: PulsarShellModel
    let actions: PulsarShellActions
    let targetsInboxModel: PulsarTargetsInboxModel

    var body: some View {
        let copy = PulsarShellCopy(locale: model.locale)
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Text(copy.text(.outgoingDrafts)).font(.title2.bold())
                Spacer()
                Button(copy.text(.refresh)) { actions.refreshPhaseOneData() }
            }
            if let failure = model.snapshot.phaseOneFailure {
                Label(
                    failure.isEmpty ? copy.text(.coordinatorFailure) : failure,
                    systemImage: "exclamationmark.triangle")
                    .font(.callout)
                    .foregroundStyle(.orange)
                    .accessibilityElement(children: .combine)
            }
            ForEach(model.snapshot.outgoingDrafts) { draft in
                PulsarOutgoingDraftRow(
                    draft: draft, copy: copy, actions: actions,
                    targetsInboxModel: targetsInboxModel)
            }
        }
    }
}

private struct PulsarOutgoingDraftRow: View {
    let draft: PulsarOutgoingDraft
    let copy: PulsarShellCopy
    let actions: PulsarShellActions
    let targetsInboxModel: PulsarTargetsInboxModel
    @State private var route: PulsarRouteTarget = .ownBarycenter
    @State private var delivery: PulsarDeliveryMode = .overlay
    @State private var rightsAcknowledged = false

    var body: some View {
        VStack(alignment: .leading, spacing: 10) {
            HStack {
                Label(draft.title, systemImage: stateSymbol)
                    .font(.headline)
                Spacer()
                Text(copy.draftStateLabel(draft.state))
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            Picker(copy.text(.routeTarget), selection: $route) {
                ForEach(PulsarRouteTarget.allCases) { value in
                    Text(copy.routeLabel(value)).tag(value)
                }
            }
            .disabled(draft.route != nil || draft.explicitTargetCount != nil)
            Picker(copy.text(.deliveryMode), selection: $delivery) {
                ForEach(PulsarDeliveryMode.allCases) { value in
                    Text(copy.deliveryLabel(value)).tag(value)
                }
            }
            .disabled(draft.requestedDelivery != nil)
            if let count = draft.explicitTargetCount {
                LabeledContent(copy.text(.selectedRecipients), value: String(count))
            } else if targetedSendAvailable {
                LabeledContent(
                    copy.text(.selectedRecipients),
                    value: String(targetsInboxModel.snapshot.selectedReferences.count))
            }
            if let requested = draft.requestedDelivery {
                LabeledContent(copy.text(.requestedDelivery), value: copy.deliveryLabel(requested))
            }
            if let effective = draft.effectiveDelivery {
                LabeledContent(copy.text(.effectiveDelivery), value: copy.deliveryLabel(effective))
            }
            if let detail = draft.downgradeReason ?? draft.failureCode ?? draft.status {
                Text(detail.replacingOccurrences(of: "_", with: " "))
                    .font(.caption)
                    .foregroundStyle(draft.failureCode == nil ? Color.secondary : Color.orange)
            }
            Toggle(copy.text(.uploadRightsConfirm), isOn: $rightsAcknowledged)
                .font(.callout)
            HStack {
                Button(draft.state == .retryableFailure ? copy.text(.retry) : copy.text(.send)) {
                    actions.sendDraft(
                        draft.id,
                        route: draft.route ?? route,
                        delivery: draft.requestedDelivery ?? delivery,
                        rightsAcknowledged: rightsAcknowledged)
                }
                .buttonStyle(.borderedProminent)
                .disabled(
                    draft.explicitTargetCount != nil || !rightsAcknowledged
                        || [.uploading, .transmitting, .accepted].contains(draft.state))
                if targetedSendAvailable || draft.explicitTargetCount != nil {
                    Button(copy.text(.sendSelectedRecipients)) {
                        actions.sendTargetedDraft(
                            draft.id,
                            delivery: draft.requestedDelivery ?? delivery,
                            rightsAcknowledged: rightsAcknowledged)
                    }
                    .buttonStyle(.borderedProminent)
                    .disabled(
                        !rightsAcknowledged
                            || [.uploading, .transmitting, .accepted].contains(draft.state))
                }
                Button(copy.text(.deleteDraft), role: .destructive) {
                    actions.deleteOutgoingDraft(draft.id)
                }
                .disabled([.uploading, .transmitting].contains(draft.state))
            }
        }
        .padding(12)
        .background(.quaternary, in: RoundedRectangle(cornerRadius: 12))
        .onAppear {
            if let frozen = draft.route { route = frozen }
            if let frozen = draft.requestedDelivery { delivery = frozen }
        }
    }

    private var stateSymbol: String {
        switch draft.state {
        case .retained: "tray.and.arrow.up"
        case .uploading, .transmitting: "arrow.triangle.2.circlepath"
        case .uploaded: "checkmark.icloud"
        case .accepted: "checkmark.circle.fill"
        case .retryableFailure: "exclamationmark.triangle"
        }
    }

    private var targetedSendAvailable: Bool {
        draft.route == nil && draft.explicitTargetCount == nil
            && targetsInboxModel.snapshot.state == .ready
            && targetsInboxModel.snapshot.selectedAudience == .explicit
            && !targetsInboxModel.snapshot.selectedReferences.isEmpty
    }
}

private struct PulsarIdentityFlowView: View {
    enum Mode { case create, join }

    let mode: Mode
    let titleKey: PulsarShellText
    let bodyKey: PulsarShellText
    let symbol: String
    let model: PulsarShellModel
    let actions: PulsarShellActions
    @State private var value = ""

    var body: some View {
        let copy = PulsarShellCopy(locale: model.locale)
        VStack(spacing: 16) {
            Label(copy.text(titleKey), systemImage: symbol)
                .font(.title2.bold())
            Text(copy.text(bodyKey))
                .foregroundStyle(.secondary)
                .multilineTextAlignment(.center)
                .frame(maxWidth: 440)
            TextField(
                copy.text(mode == .create ? .orbitTitle : .inviteCode),
                text: $value)
                .textFieldStyle(.roundedBorder)
                .frame(maxWidth: 360)
                .disabled(isBusy)
                .onSubmit(submit)
            Button(
                copy.text(mode == .create ? .createWithAPI : .joinWithAPI),
                action: submit)
                .buttonStyle(.borderedProminent)
                .disabled(value.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty || isBusy)
            identityStatus(copy)
        }
        .frame(maxWidth: .infinity, maxHeight: .infinity)
        .padding(32)
        .navigationTitle(copy.text(titleKey))
    }

    @ViewBuilder
    private func identityStatus(_ copy: PulsarShellCopy) -> some View {
        switch model.snapshot.identityOperation {
        case .idle:
            EmptyView()
        case .busy:
            ProgressView(copy.text(.identityBusy))
        case .succeeded(let message):
            Label(message.isEmpty ? copy.text(.identitySucceeded) : message,
                  systemImage: "checkmark.circle.fill")
                .foregroundStyle(.green)
        case .recoveryExportRequired(let message):
            VStack(spacing: 8) {
                Label(
                    message.isEmpty ? copy.text(.recoveryRequired) : message,
                    systemImage: "key.fill")
                    .multilineTextAlignment(.center)
                Button(copy.text(.exportRecovery)) { actions.exportRecovery() }
                    .buttonStyle(.borderedProminent)
            }
        case .failed(let message):
            Label(message.isEmpty ? copy.text(.identityFailed) : message,
                  systemImage: "exclamationmark.triangle")
                .foregroundStyle(.red)
        }
    }

    private var isBusy: Bool {
        if case .busy = model.snapshot.identityOperation { return true }
        return false
    }

    private func submit() {
        let clean = value.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !clean.isEmpty else { return }
        switch mode {
        case .create: actions.submitCreateOrbit(title: clean)
        case .join: actions.submitJoinOrbit(code: clean)
        }
    }
}

private struct PulsarSelfTestView: View {
    let model: PulsarShellModel
    let actions: PulsarShellActions
    @State private var showingImporter = false
    @State private var pendingFileURL: URL?

    var body: some View {
        let copy = PulsarShellCopy(locale: model.locale)
        Group {
            if !model.snapshot.selfTestAvailable {
                ContentUnavailableView {
                    Label(copy.text(.tryTitle), systemImage: "waveform.circle")
                } description: {
                    VStack(spacing: 8) {
                        Text(copy.text(.tryBody))
                        Text(copy.text(.selfTestUnavailable))
                    }
                }
            } else {
                ScrollView {
                    VStack(alignment: .leading, spacing: 18) {
                        Label(copy.text(.tryTitle), systemImage: "waveform.circle")
                            .font(.title2.bold())
                        Text(copy.text(.tryBody))
                            .foregroundStyle(.secondary)
                        Label(
                            copy.selfTestLabel(model.snapshot.selfTestState),
                            systemImage: selfTestSymbol(model.snapshot.selfTestState))
                            .accessibilityElement(children: .combine)
                        if model.snapshot.selfTestState == .recording {
                            ProgressView(value: Double(model.snapshot.selfTestMeter))
                                .accessibilityLabel(copy.text(.selfTestRecording))
                        }
                        HStack {
                            Button(copy.text(.playBuiltinCue), action: actions.playBuiltinCue)
                            Button(copy.text(.recordFiveSeconds), action: actions.recordFiveSeconds)
                                .buttonStyle(.borderedProminent)
                                .keyboardShortcut("t", modifiers: [.command, .shift])
                        }
                        .disabled(isSelfTestBusy(model.snapshot.selfTestState)
                            || model.snapshot.recording == .recording
                            || model.snapshot.recording == .processing)

                        Divider()

                        Button(copy.text(.chooseAudioFile)) { showingImporter = true }
                        Label(copy.text(.dropAudioFile), systemImage: "square.and.arrow.down")
                            .frame(maxWidth: .infinity, minHeight: 74)
                            .background(.quaternary, in: RoundedRectangle(cornerRadius: 10))
                            .dropDestination(for: URL.self) { urls, _ in
                                guard let url = urls.first else { return false }
                                select(url)
                                return true
                            }

                        if let review = model.snapshot.localFileReview {
                            reviewView(review, copy: copy)
                        }
                        if model.snapshot.localDraftAvailable {
                            Button(copy.text(.deleteDraft), role: .destructive) {
                                actions.deleteLocalDraft()
                            }
                        }
                    }
                    .frame(maxWidth: 620, alignment: .leading)
                    .padding(24)
                }
            }
        }
        .navigationTitle(copy.text(.tryLocally))
        .fileImporter(
            isPresented: $showingImporter,
            allowedContentTypes: [.audio],
            allowsMultipleSelection: false
        ) { result in
            guard case .success(let urls) = result, let url = urls.first else { return }
            select(url)
        }
        .onDisappear(perform: actions.closeSelfTest)
    }

    @ViewBuilder
    private func reviewView(_ review: PulsarLocalFileReview, copy: PulsarShellCopy) -> some View {
        GroupBox(copy.text(.fileReview)) {
            VStack(alignment: .leading, spacing: 8) {
                LabeledContent(copy.text(.filename), value: review.filename)
                LabeledContent(copy.text(.format), value: review.format ?? "—")
                LabeledContent(copy.text(.duration), value: duration(review.durationMs))
                LabeledContent(copy.text(.size), value: ByteCountFormatter.string(
                    fromByteCount: review.sizeBytes, countStyle: .file))
                LabeledContent(copy.text(.audience), value: review.audience.joined(separator: ", "))
                LabeledContent(
                    copy.text(.deliveryModes),
                    value: review.deliveryModes.isEmpty ? "—" : review.deliveryModes.joined(separator: ", "))
                Text("\(copy.text(.rightsReminder)): \(review.rightsReminder)")
                    .font(.callout)
                if review.serverValidationRequired {
                    Text(copy.text(.serverWillRecheck))
                        .font(.caption)
                        .foregroundStyle(.secondary)
                }
                if let rejection = review.rejection {
                    Label(rejection, systemImage: "exclamationmark.triangle.fill")
                        .foregroundStyle(.red)
                    Text(copy.text(.p2FileGuidance))
                        .font(.callout)
                } else if let pendingFileURL {
                    Button(copy.text(.acceptDraft)) {
                        actions.acceptLocalFile(pendingFileURL)
                    }
                    .buttonStyle(.borderedProminent)
                }
            }
            .frame(maxWidth: .infinity, alignment: .leading)
            .padding(.vertical, 4)
        }
    }

    private func select(_ url: URL) {
        pendingFileURL = url
        actions.reviewLocalFile(url)
    }

    private func duration(_ milliseconds: Int64?) -> String {
        guard let milliseconds else { return "—" }
        return String(format: "%.1f s", Double(milliseconds) / 1_000)
    }

    private func isSelfTestBusy(_ state: PulsarSelfTestState) -> Bool {
        ![.idle, .reviewingDraft, .failed].contains(state)
    }

    private func selfTestSymbol(_ state: PulsarSelfTestState) -> String {
        switch state {
        case .idle: "checkmark.circle"
        case .playingBuiltinCue, .playingStopCue, .playingRecording: "speaker.wave.2"
        case .requestingPermission: "mic.badge.plus"
        case .recording: "record.circle.fill"
        case .reviewingDraft: "doc.badge.checkmark"
        case .failed: "exclamationmark.triangle.fill"
        }
    }
}

private struct PulsarSettingsView: View {
    @Bindable var model: PulsarShellModel
    let actions: PulsarShellActions

    var body: some View {
        let copy = PulsarShellCopy(locale: model.locale)
        Form {
            Picker(copy.text(.language), selection: $model.locale) {
                Text("English").tag(PulsarShellLocale.en)
                Text("Русский").tag(PulsarShellLocale.ru)
            }
            .pickerStyle(.segmented)
            Picker(copy.text(.inputDevice), selection: Binding(
                get: { model.snapshot.selectedCaptureDeviceID },
                set: { actions.setCaptureDevice($0) }
            )) {
                Text(copy.text(.defaultInput)).tag(String?.none)
                ForEach(model.snapshot.captureDevices) { device in
                    Text(device.name + (device.isDefault ? " · " + copy.text(.defaultInput) : ""))
                        .tag(Optional(device.id))
                }
            }
            .disabled(model.snapshot.recording == .recording
                || model.snapshot.selfTestState == .recording)
            Picker(copy.text(.recordingShortcut), selection: Binding(
                get: { model.snapshot.recordingShortcut },
                set: { actions.setRecordingShortcut($0) }
            )) {
                ForEach(PulsarRecordingShortcutChoice.allCases) { shortcut in
                    Text(shortcut.displayValue).tag(shortcut)
                }
            }
            LabeledContent(copy.text(.recordingShortcut)) {
                VStack(alignment: .trailing, spacing: 3) {
                    Text(copy.recordingShortcutLabel(model.snapshot.recordingShortcutState))
                    if model.snapshot.recordingShortcutState == .conflict
                        || model.snapshot.recordingShortcutState == .unavailable {
                        Text(copy.text(.shortcutFallback))
                            .font(.caption)
                            .foregroundStyle(.secondary)
                    }
                }
            }
            Section(copy.text(.integrations)) {
                Label(copy.text(.spotifyOptional), systemImage: "music.note")
                Label(copy.text(.telegramOptional), systemImage: "paperplane")
            }
        }
        .formStyle(.grouped)
        .padding()
        .navigationTitle(copy.text(.settingsTitle))
    }
}

@MainActor
public final class PulsarMainWindowController: NSObject, NSWindowDelegate {
    private let model: PulsarShellModel
    private let actions: PulsarShellActions
    private let targetsInboxModel: PulsarTargetsInboxModel
    private let targetsInboxActions: PulsarTargetsInboxActions
    private let streamTrackModel: PulsarStreamTrackModel
    private let streamTrackActions: PulsarStreamTrackActions
    private var window: NSWindow?

    public init(
        model: PulsarShellModel,
        actions: PulsarShellActions,
        targetsInboxModel: PulsarTargetsInboxModel,
        targetsInboxActions: PulsarTargetsInboxActions,
        streamTrackModel: PulsarStreamTrackModel,
        streamTrackActions: PulsarStreamTrackActions
    ) {
        self.model = model
        self.actions = actions
        self.targetsInboxModel = targetsInboxModel
        self.targetsInboxActions = targetsInboxActions
        self.streamTrackModel = streamTrackModel
        self.streamTrackActions = streamTrackActions
    }

    public func show(section: PulsarShellSection = .home) {
        model.selectedSection = section
        let target = window ?? makeWindow()
        NSApp.activate(ignoringOtherApps: true)
        target.makeKeyAndOrderFront(nil)
    }

    private func makeWindow() -> NSWindow {
        let root = PulsarMainView(
            model: model,
            actions: actions,
            targetsInboxModel: targetsInboxModel,
            targetsInboxActions: targetsInboxActions,
            streamTrackModel: streamTrackModel,
            streamTrackActions: streamTrackActions)
        let hosting = NSHostingController(rootView: root)
        let target = NSWindow(contentViewController: hosting)
        target.title = "Pulsar"
        target.styleMask = [.titled, .closable, .miniaturizable, .resizable]
        target.minSize = NSSize(width: 760, height: 520)
        target.setContentSize(NSSize(width: 960, height: 680))
        target.center()
        target.isReleasedWhenClosed = false
        target.delegate = self
        target.setAccessibilityLabel("Pulsar")
        window = target
        return target
    }
}
