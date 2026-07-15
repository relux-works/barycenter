import AppKit
import SwiftUI
import UniformTypeIdentifiers

public struct PulsarMainView: View {
    @Bindable private var model: PulsarShellModel
    private let actions: PulsarShellActions

    public init(model: PulsarShellModel, actions: PulsarShellActions) {
        self.model = model
        self.actions = actions
    }

    public var body: some View {
        NavigationSplitView {
            PulsarSidebar(model: model)
                .navigationSplitViewColumnWidth(min: 190, ideal: 220, max: 280)
        } detail: {
            PulsarDetail(model: model, actions: actions)
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
        case .create: "plus.circle"
        case .join: "person.2"
        case .tryLocally: "waveform.circle"
        case .history: "clock.arrow.circlepath"
        case .settings: "gear"
        }
    }
}

private struct PulsarDetail: View {
    let model: PulsarShellModel
    let actions: PulsarShellActions

    var body: some View {
        switch model.selectedSection {
        case .home:
            PulsarHomeView(model: model, actions: actions)
        case .airs:
            PulsarAirManagementView(model: model, actions: actions)
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
        case .history:
            PulsarHistoryView(model: model, actions: actions)
        case .settings:
            PulsarSettingsView(model: model, actions: actions)
        }
    }
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
                PulsarOutgoingDraftsView(model: model, actions: actions)
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

    var body: some View {
        let copy = PulsarShellCopy(locale: model.locale)
        Group {
            if model.snapshot.history.isEmpty {
                ContentUnavailableView(copy.text(.noHistory), systemImage: "clock.arrow.circlepath")
            } else {
                List(model.snapshot.history) { item in
                    PulsarHistoryRow(item: item, locale: model.locale, actions: actions)
                }
            }
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
                    draft: draft, copy: copy, actions: actions)
            }
        }
    }
}

private struct PulsarOutgoingDraftRow: View {
    let draft: PulsarOutgoingDraft
    let copy: PulsarShellCopy
    let actions: PulsarShellActions
    @State private var route: PulsarRouteTarget = .ownBarycenter
    @State private var delivery: PulsarDeliveryMode = .overlay

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
            .disabled(draft.route != nil)
            Picker(copy.text(.deliveryMode), selection: $delivery) {
                ForEach(PulsarDeliveryMode.allCases) { value in
                    Text(copy.deliveryLabel(value)).tag(value)
                }
            }
            .disabled(draft.requestedDelivery != nil)
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
            HStack {
                Button(draft.state == .retryableFailure ? copy.text(.retry) : copy.text(.send)) {
                    actions.sendDraft(
                        draft.id,
                        route: draft.route ?? route,
                        delivery: draft.requestedDelivery ?? delivery)
                }
                .buttonStyle(.borderedProminent)
                .disabled([.uploading, .transmitting, .accepted].contains(draft.state))
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
    private var window: NSWindow?

    public init(model: PulsarShellModel, actions: PulsarShellActions) {
        self.model = model
        self.actions = actions
    }

    public func show(section: PulsarShellSection = .home) {
        model.selectedSection = section
        let target = window ?? makeWindow()
        NSApp.activate(ignoringOtherApps: true)
        target.makeKeyAndOrderFront(nil)
    }

    private func makeWindow() -> NSWindow {
        let root = PulsarMainView(model: model, actions: actions)
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
