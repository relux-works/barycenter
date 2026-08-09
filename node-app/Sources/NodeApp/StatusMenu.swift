// R2 (goal v2 D7): menu-bar presence — connection state, now playing,
// output picker, login item, version. No dock clutter.

import AppKit
import ServiceManagement
import Sparkle
import NodeCore
import NodeAppUI

@MainActor
final class StatusMenuController: NSObject, NSMenuDelegate, NSMenuItemValidation {
    private var item: NSStatusItem!
    private let menu = NSMenu()

    // Wired after the core starts; nil while onboarding.
    var player: PlayerCore?
    var updater: SPUUpdater?
    var coordinatorConnected: () -> Bool = { false }
    var orbitLabel: String = ""
    // F3: where this Pulsar is connected ("barycenter.relux.works · дом a"),
    // shown in the menu so a wrong coordinator is obvious at a glance.
    var connectionIdentity: String = ""
    // F3: opens the onboarding window to re-pair at any time.
    var rePairAction: (() -> Void)?
    var shellModel: PulsarShellModel?
    var shellActions = PulsarShellActions()
    var targetsInboxModel: PulsarTargetsInboxModel?
    var showMainWindowAction: (() -> Void)?
    var triggerSelectedSoundboardCue: (() -> Void)?

    func install() {
        item = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
        if let button = item.button {
            button.image = NSImage(systemSymbolName: "dot.radiowaves.left.and.right",
                                   accessibilityDescription: "Pulsar")
        }
        menu.delegate = self
        item.menu = menu
        installMainMenuCommands()
    }

    private func installMainMenuCommands() {
        guard let mainMenu = NSApp.mainMenu else { return }
        let copy = PulsarShellCopy(locale: shellModel?.locale ?? .preferred())

        let windowRoot = NSMenuItem()
        let windowMenu = NSMenu(title: localized(en: "Window", ru: "Окно"))
        let open = NSMenuItem(
            title: copy.text(.openMainWindow), action: #selector(showMainWindow), keyEquivalent: "0")
        open.target = self
        windowMenu.addItem(open)
        let settings = NSMenuItem(
            title: copy.text(.settings), action: #selector(showSettings), keyEquivalent: ",")
        settings.target = self
        windowMenu.addItem(settings)
        windowRoot.submenu = windowMenu
        mainMenu.addItem(windowRoot)

        let actionRoot = NSMenuItem()
        let actionMenu = NSMenu(title: localized(en: "Actions", ru: "Действия"))
        for (title, selector, key) in [
            (copy.text(.create), #selector(createOrbit), "1"),
            (copy.text(.join), #selector(joinOrbit), "2"),
            (copy.text(.inbox), #selector(showInbox), "3"),
            (copy.text(.airs), #selector(showAirs), "4"),
        ] {
            let item = NSMenuItem(title: title, action: selector, keyEquivalent: key)
            item.target = self
            actionMenu.addItem(item)
        }
        let local = NSMenuItem(
            title: copy.text(.tryLocally), action: #selector(showSelfTest), keyEquivalent: "t")
        local.keyEquivalentModifierMask = [.command, .shift]
        local.target = self
        actionMenu.addItem(local)
        let record = NSMenuItem(
            title: copy.text(.startRecording), action: #selector(toggleRecording), keyEquivalent: "r")
        record.keyEquivalentModifierMask = [.command, .shift]
        record.target = self
        actionMenu.addItem(record)
        let cancel = NSMenuItem(
            title: copy.text(.cancelRecording),
            action: #selector(cancelRecording),
            keyEquivalent: "")
        cancel.target = self
        actionMenu.addItem(cancel)
        let stopCapture = NSMenuItem(
            title: copy.text(.captureStopLocal),
            action: #selector(stopActiveCapture),
            keyEquivalent: ".")
        stopCapture.keyEquivalentModifierMask = [.command]
        stopCapture.target = self
        actionMenu.addItem(stopCapture)
        let dnd = NSMenuItem(
            title: copy.text(.dnd), action: #selector(toggleDND), keyEquivalent: "d")
        dnd.keyEquivalentModifierMask = [.command, .shift]
        dnd.target = self
        actionMenu.addItem(dnd)
        actionRoot.submenu = actionMenu
        mainMenu.addItem(actionRoot)
    }

    // Menu is rebuilt on every open: cheap and always fresh.
    func menuNeedsUpdate(_ menu: NSMenu) {
        menu.removeAllItems()

        let copy = PulsarShellCopy(locale: shellModel?.locale ?? .preferred())
        let open = NSMenuItem(
            title: copy.text(.openMainWindow),
            action: #selector(showMainWindow),
            keyEquivalent: "0")
        open.target = self
        menu.addItem(open)

        let create = NSMenuItem(
            title: copy.text(.create),
            action: #selector(createOrbit),
            keyEquivalent: "1")
        create.target = self
        menu.addItem(create)
        let join = NSMenuItem(
            title: copy.text(.join),
            action: #selector(joinOrbit),
            keyEquivalent: "2")
        join.target = self
        menu.addItem(join)
        let inboxCount = targetsInboxModel?.snapshot.inbox.filter {
            $0.availability == "available"
        }.count ?? 0
        let inbox = NSMenuItem(
            title: inboxCount == 0 ? copy.text(.inbox) : "\(copy.text(.inbox)) (\(inboxCount))",
            action: #selector(showInbox),
            keyEquivalent: "3")
        inbox.target = self
        menu.addItem(inbox)
        let airs = NSMenuItem(
            title: copy.text(.airs),
            action: #selector(showAirs),
            keyEquivalent: "4")
        airs.target = self
        menu.addItem(airs)
        let local = NSMenuItem(
            title: copy.text(.tryLocally),
            action: #selector(showSelfTest),
            keyEquivalent: "t")
        local.keyEquivalentModifierMask = [.command, .shift]
        local.target = self
        menu.addItem(local)
        let soundboard = NSMenuItem(
            title: copy.text(.soundboard), action: #selector(showSoundboard), keyEquivalent: "5")
        soundboard.target = self
        menu.addItem(soundboard)
        let automation = NSMenuItem(
            title: copy.text(.automation), action: #selector(showAutomation), keyEquivalent: "6")
        automation.target = self
        menu.addItem(automation)
        let selectedCue = shellModel?.snapshot.soundboard.cues.first {
            $0.id == shellModel?.snapshot.soundboard.selectedCueID
        }
        let triggerCue = NSMenuItem(
            title: selectedCue.map { localized(en: "Trigger: ", ru: "Запустить: ") + $0.title }
                ?? localized(en: "Trigger selected cue", ru: "Запустить выбранный сигнал"),
            action: #selector(triggerSoundboard), keyEquivalent: "")
        triggerCue.target = self
        triggerCue.isEnabled = selectedCue != nil && shellModel?.snapshot.soundboard.busy == false
        menu.addItem(triggerCue)

        let record = NSMenuItem(
            title: shellModel?.snapshot.recording == .recording
                ? copy.text(.stopRecording)
                : copy.text(.startRecording),
            action: #selector(toggleRecording),
            keyEquivalent: "r")
        record.keyEquivalentModifierMask = [.command, .shift]
        record.target = self
        record.isEnabled = shellModel?.snapshot.recordingAvailable == true
            || shellModel?.snapshot.recording == .recording
        if shellModel?.snapshot.recording == .processing { record.isEnabled = false }
        if let selfTest = shellModel?.snapshot.selfTestState,
           ![.idle, .reviewingDraft, .failed].contains(selfTest) {
            record.isEnabled = false
        }
        menu.addItem(record)
        if shellModel?.snapshot.recording == .recording {
            let cancel = NSMenuItem(
                title: copy.text(.cancelRecording),
                action: #selector(cancelRecording),
                keyEquivalent: "")
            cancel.target = self
            menu.addItem(cancel)
        }
        if let snapshot = shellModel?.snapshot {
            let capture = PulsarLocalCapturePresentation(snapshot: snapshot)
            if capture.isActive {
                let stop = NSMenuItem(
                    title: copy.text(.captureStopLocal),
                    action: #selector(stopActiveCapture),
                    keyEquivalent: ".")
                stop.keyEquivalentModifierMask = [.command]
                stop.target = self
                stop.isEnabled = capture.canStop
                menu.addItem(stop)
            }
        }

        let dnd = NSMenuItem(
            title: copy.dndLabel(shellModel?.snapshot.dndMode ?? .allowAll),
            action: #selector(toggleDND),
            keyEquivalent: "d")
        dnd.keyEquivalentModifierMask = [.command, .shift]
        dnd.target = self
        dnd.isEnabled = shellModel?.snapshot.connection.isPaired == true
        menu.addItem(dnd)
        menu.addItem(.separator())

        if let player {
            let st = player.menuStatus()
            let link = coordinatorConnected()
                ? copy.text(.connectionOnline)
                : copy.text(.connectionReconnecting)
            menu.addItem(disabled(link))
            if !connectionIdentity.isEmpty {
                menu.addItem(disabled(connectionIdentity))
            }
            if let currentAir = shellModel?.snapshot.airs.current {
                menu.addItem(disabled(localized(en: "Active Air: ", ru: "Активный эфир: ") + currentAir.title))
            } else if shellModel?.snapshot.airs.saved.isEmpty == false {
                menu.addItem(disabled(localized(en: "No active Air", ru: "Нет активного эфира")))
            }
            let mode = st.mode == "shared"
                ? localized(en: "Shared air", ru: "периастрон — общий эфир")
                : localized(en: "Personal playback", ru: "апоастрон — каждый своё")
            menu.addItem(disabled(mode))
            if st.playback == "playing", let uri = st.uri {
                menu.addItem(disabled(localized(en: "Playing: ", ru: "Играет: ") + (st.title ?? shortURI(uri))))
            } else {
                menu.addItem(disabled(copy.text(.silence)))
            }
        } else {
            menu.addItem(disabled(copy.text(.connectionUnpaired)))
        }
        menu.addItem(.separator())

        // F3: re-pair anytime (after a coordinator move, a wrong pairing, or a
        // lost token). Opens the onboarding window; the core restarts on pair.
        if rePairAction != nil {
            let rp = NSMenuItem(
                title: localized(en: "Pair again…", ru: "Подключить заново…"),
                action: #selector(rePair), keyEquivalent: "")
            rp.target = self
            menu.addItem(rp)
        }

        // Optional Spotify help stays reachable without presenting the
        // integration as a prerequisite for Pulsar audio.
        let howto = NSMenuItem(
            title: localized(en: "Optional Spotify integration…", ru: "Необязательная интеграция Spotify…"),
            action: #selector(showSpotifyHelp), keyEquivalent: "")
        howto.target = self
        menu.addItem(howto)
        let noPulsar = NSMenuItem(
            title: localized(en: "Troubleshoot Spotify integration", ru: "Диагностика интеграции Spotify"),
            action: #selector(openGuide), keyEquivalent: "")
        noPulsar.target = self
        menu.addItem(noPulsar)

        let policies = NSMenu()
        for (title, url) in [
            (localized(en: "Privacy", ru: "Конфиденциальность"), PublicPolicyLinks.privacy),
            (localized(en: "Terms of Use", ru: "Условия использования"), PublicPolicyLinks.terms),
            (localized(en: "Content Guidelines", ru: "Правила содержимого"), PublicPolicyLinks.contentGuidelines),
            (localized(en: "Recording and upload rights", ru: "Права на запись и загрузку"), PublicPolicyLinks.uploadRights),
            (localized(en: "Support and safety", ru: "Поддержка и безопасность"), PublicPolicyLinks.support),
        ] {
            let policy = NSMenuItem(title: title, action: #selector(openPublicPolicy(_:)), keyEquivalent: "")
            policy.target = self
            policy.representedObject = url
            policies.addItem(policy)
        }
        let policiesItem = NSMenuItem(
            title: localized(en: "Policies and support", ru: "Правила и поддержка"),
            action: nil, keyEquivalent: "")
        menu.addItem(policiesItem)
        menu.setSubmenu(policies, for: policiesItem)

        // Output devices submenu.
        let outMenu = NSMenu()
        let current = DirectOutputMonitor.currentOutputName()
        for name in DirectOutputMonitor.listOutputDevices() {
            let mi = NSMenuItem(title: name, action: #selector(pickOutput(_:)), keyEquivalent: "")
            mi.target = self
            mi.state = (name == current) ? .on : .off
            outMenu.addItem(mi)
        }
        let outItem = NSMenuItem(
            title: localized(en: "Output", ru: "Колонка"), action: nil, keyEquivalent: "")
        menu.addItem(outItem)
        menu.setSubmenu(outMenu, for: outItem)

        // Login item toggle (macOS 13+).
        let login = NSMenuItem(
            title: localized(en: "Open at Login", ru: "Запускать при входе"),
            action: #selector(toggleLogin), keyEquivalent: "")
        login.target = self
        login.state = (SMAppService.mainApp.status == .enabled) ? .on : .off
        menu.addItem(login)

        menu.addItem(.separator())
        if updater != nil {
            let upd = NSMenuItem(
                title: localized(en: "Check for Updates…", ru: "Проверить обновления…"),
                action: #selector(checkUpdates), keyEquivalent: "")
            upd.target = self
            menu.addItem(upd)
        }
        menu.addItem(disabled("Pulsar \(appVersion)"))
        let quit = NSMenuItem(
            title: copy.text(.quit), action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        menu.addItem(quit)
    }

    private func localized(en: String, ru: String) -> String {
        shellModel?.locale == .ru ? ru : en
    }

    func validateMenuItem(_ menuItem: NSMenuItem) -> Bool {
        switch menuItem.action {
        case #selector(toggleRecording):
            shellModel?.snapshot.recording != .processing
                && [PulsarSelfTestState.idle, .reviewingDraft, .failed].contains(
                    shellModel?.snapshot.selfTestState ?? .idle)
                && (shellModel?.snapshot.recordingAvailable == true
                    || shellModel?.snapshot.recording == .recording)
        case #selector(cancelRecording):
            shellModel?.snapshot.recording == .recording
        case #selector(stopActiveCapture):
            shellModel.map {
                PulsarLocalCapturePresentation(snapshot: $0.snapshot).canStop
            } ?? false
        case #selector(toggleDND):
            shellModel?.snapshot.connection.isPaired == true
        default:
            true
        }
    }

    private func disabled(_ title: String) -> NSMenuItem {
        let mi = NSMenuItem(title: title, action: nil, keyEquivalent: "")
        mi.isEnabled = false
        return mi
    }

    private func shortURI(_ uri: String) -> String {
        uri.hasPrefix("spotify:track:") ? String(uri.dropFirst("spotify:".count)) : uri
    }

    @objc private func pickOutput(_ sender: NSMenuItem) {
        let name = sender.title
        DispatchQueue.global().async {
            _ = DirectOutputMonitor.setDefaultOutput(named: name)
            UserDefaults.standard.set(name, forKey: "outputDevice")
        }
    }

    @objc private func checkUpdates() {
        updater?.checkForUpdates()
    }

    @objc private func rePair() {
        rePairAction?()
    }

    @objc private func showMainWindow() {
        showMainWindowAction?()
    }

    @objc private func showSettings() {
        shellModel?.selectedSection = .settings
        showMainWindowAction?()
    }

    @objc private func createOrbit() {
        shellActions.createOrbit()
    }

    @objc private func joinOrbit() {
        shellActions.joinOrbit()
    }

    @objc private func showAirs() {
        shellModel?.selectedSection = .airs
        showMainWindowAction?()
    }

    @objc private func showInbox() {
        shellModel?.selectedSection = .inbox
        showMainWindowAction?()
    }

    @objc private func showSelfTest() {
        shellModel?.selectedSection = .tryLocally
        showMainWindowAction?()
    }

    @objc private func showSoundboard() {
        shellModel?.selectedSection = .soundboard
        showMainWindowAction?()
    }

    @objc private func showAutomation() {
        shellModel?.selectedSection = .automation
        showMainWindowAction?()
    }

    @objc private func triggerSoundboard() { triggerSelectedSoundboardCue?() }

    @objc private func toggleRecording() {
        shellActions.toggleRecording()
    }

    @objc private func cancelRecording() {
        shellActions.cancelRecording()
    }

    @objc private func stopActiveCapture() {
        shellActions.stopActiveCapture()
    }

    @objc private func toggleDND() {
        let next: PulsarDNDMode = shellModel?.snapshot.dndMode == .allowAll
            ? .messagesOnly : .allowAll
        shellActions.setDND(next)
    }

    @objc private func showSpotifyHelp() {
        SpotifyHelp.presentHowToSound()
    }

    @objc private func openGuide() {
        SpotifyHelp.openGuide()
    }

    @objc private func openPublicPolicy(_ sender: NSMenuItem) {
        guard let url = sender.representedObject as? URL else { return }
        NSWorkspace.shared.open(url)
    }

    @objc private func toggleLogin() {
        do {
            if SMAppService.mainApp.status == .enabled {
                try SMAppService.mainApp.unregister()
            } else {
                try SMAppService.mainApp.register()
            }
        } catch {
            NSLog("login item toggle failed: \(error)")
        }
    }
}

// SpotifyHelp describes an optional music source and remains user-invoked.
enum SpotifyHelp {
    static let guideURL = URL(string: "https://barycenter.live/guide/")!

    static func presentHowToSound() {
        NSApp.activate(ignoringOtherApps: true)
        let alert = NSAlert()
        alert.messageText = "Необязательная интеграция Spotify"
        alert.informativeText = """
        Звук Пульсара и локальная проверка работают без Spotify. Если хочешь использовать Spotify как источник музыки:

        1. Открой Spotify (для Spotify Connect нужен Spotify Premium).
        2. В списке устройств выбери «Pulsar».
        3. Включи любой трек — это нужно один раз, чтобы Spotify запомнил колонку.

        Не видно «Pulsar»? Телефон и компьютер — в одной Wi-Fi; проверь файрвол macOS и выключи VPN.
        """
        alert.addButton(withTitle: "Понятно")
        alert.addButton(withTitle: "Открыть гид")
        if alert.runModal() == .alertSecondButtonReturn {
            openGuide()
        }
    }

    static func openGuide() {
        NSWorkspace.shared.open(guideURL)
    }
}
