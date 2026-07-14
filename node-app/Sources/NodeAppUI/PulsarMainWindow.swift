import AppKit
import SwiftUI

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
        case .create:
            PulsarFlowView(
                titleKey: .createTitle,
                bodyKey: .createBody,
                actionKey: .createAction,
                symbol: "plus.circle",
                locale: model.locale,
                action: actions.createOrbit
            )
        case .join:
            PulsarFlowView(
                titleKey: .joinTitle,
                bodyKey: .joinBody,
                actionKey: .joinAction,
                symbol: "person.2",
                locale: model.locale,
                action: actions.joinOrbit
            )
        case .tryLocally:
            PulsarSelfTestView(model: model, action: actions.tryLocally)
        case .history:
            PulsarHistoryView(model: model)
        case .settings:
            PulsarSettingsView(model: model)
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
            .disabled(!model.snapshot.recordingAvailable && model.snapshot.recording != .recording)
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
                PulsarHistoryRow(item: item)
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

    var body: some View {
        let copy = PulsarShellCopy(locale: model.locale)
        Group {
            if model.snapshot.history.isEmpty {
                ContentUnavailableView(copy.text(.noHistory), systemImage: "clock.arrow.circlepath")
            } else {
                List(model.snapshot.history) { item in PulsarHistoryRow(item: item) }
            }
        }
        .navigationTitle(copy.text(.historyTitle))
    }
}

private struct PulsarHistoryRow: View {
    let item: PulsarHistoryItem

    var body: some View {
        VStack(alignment: .leading, spacing: 4) {
            Text(item.title).font(.headline)
            Text(item.detail).foregroundStyle(.secondary)
            Text(item.occurredAt, format: .dateTime.year().month().day().hour().minute())
                .font(.caption)
                .foregroundStyle(.secondary)
        }
        .accessibilityElement(children: .combine)
    }
}

private struct PulsarFlowView: View {
    let titleKey: PulsarShellText
    let bodyKey: PulsarShellText
    let actionKey: PulsarShellText
    let symbol: String
    let locale: PulsarShellLocale
    let action: () -> Void

    var body: some View {
        let copy = PulsarShellCopy(locale: locale)
        ContentUnavailableView {
            Label(copy.text(titleKey), systemImage: symbol)
        } description: {
            Text(copy.text(bodyKey))
        } actions: {
            Button(copy.text(actionKey), action: action)
                .buttonStyle(.borderedProminent)
        }
        .navigationTitle(copy.text(titleKey))
    }
}

private struct PulsarSelfTestView: View {
    let model: PulsarShellModel
    let action: () -> Void

    var body: some View {
        let copy = PulsarShellCopy(locale: model.locale)
        ContentUnavailableView {
            Label(copy.text(.tryTitle), systemImage: "waveform.circle")
        } description: {
            VStack(spacing: 8) {
                Text(copy.text(.tryBody))
                if !model.snapshot.selfTestAvailable {
                    Text(copy.text(.selfTestUnavailable))
                }
            }
        } actions: {
            Button(copy.text(.tryAction), action: action)
                .buttonStyle(.borderedProminent)
                .disabled(!model.snapshot.selfTestAvailable)
                .keyboardShortcut("t", modifiers: [.command, .shift])
        }
        .navigationTitle(copy.text(.tryLocally))
    }
}

private struct PulsarSettingsView: View {
    @Bindable var model: PulsarShellModel

    var body: some View {
        let copy = PulsarShellCopy(locale: model.locale)
        Form {
            Picker(copy.text(.language), selection: $model.locale) {
                Text("English").tag(PulsarShellLocale.en)
                Text("Русский").tag(PulsarShellLocale.ru)
            }
            .pickerStyle(.segmented)
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
