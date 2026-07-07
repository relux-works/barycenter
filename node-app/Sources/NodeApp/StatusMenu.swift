// R2 (goal v2 D7): menu-bar presence — connection state, now playing,
// output picker, login item, version. No dock clutter.

import AppKit
import ServiceManagement
import Sparkle
import NodeCore

final class StatusMenuController: NSObject, NSMenuDelegate {
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

    func install() {
        item = NSStatusBar.system.statusItem(withLength: NSStatusItem.squareLength)
        if let button = item.button {
            button.image = NSImage(systemSymbolName: "dot.radiowaves.left.and.right",
                                   accessibilityDescription: "Pulsar")
        }
        menu.delegate = self
        item.menu = menu
    }

    // Menu is rebuilt on every open: cheap and always fresh.
    func menuNeedsUpdate(_ menu: NSMenu) {
        menu.removeAllItems()

        if let player {
            let st = player.menuStatus()
            let link = coordinatorConnected() ? "Барицентр: в сети" : "Барицентр: переподключение…"
            menu.addItem(disabled(link))
            if !connectionIdentity.isEmpty {
                menu.addItem(disabled(connectionIdentity))
            }
            let mode = st.mode == "shared" ? "периастрон — общий эфир" : "апоастрон — каждый своё"
            menu.addItem(disabled(mode))
            if st.playback == "playing", let uri = st.uri {
                menu.addItem(disabled("играет: " + shortURI(uri)))
            } else {
                menu.addItem(disabled("тишина"))
            }
        } else {
            menu.addItem(disabled("не спарен — введи код из @barycenter_bot"))
        }
        menu.addItem(.separator())

        // F3: re-pair anytime (after a coordinator move, a wrong pairing, or a
        // lost token). Opens the onboarding window; the core restarts on pair.
        if rePairAction != nil {
            let rp = NSMenuItem(title: "Подключить заново…", action: #selector(rePair), keyEquivalent: "")
            rp.target = self
            menu.addItem(rp)
        }

        // Output devices submenu.
        let outMenu = NSMenu()
        let current = DirectOutputMonitor.currentOutputName()
        for name in DirectOutputMonitor.listOutputDevices() {
            let mi = NSMenuItem(title: name, action: #selector(pickOutput(_:)), keyEquivalent: "")
            mi.target = self
            mi.state = (name == current) ? .on : .off
            outMenu.addItem(mi)
        }
        let outItem = NSMenuItem(title: "Колонка", action: nil, keyEquivalent: "")
        menu.addItem(outItem)
        menu.setSubmenu(outMenu, for: outItem)

        // Login item toggle (macOS 13+).
        let login = NSMenuItem(title: "Запускать при входе", action: #selector(toggleLogin), keyEquivalent: "")
        login.target = self
        login.state = (SMAppService.mainApp.status == .enabled) ? .on : .off
        menu.addItem(login)

        menu.addItem(.separator())
        if updater != nil {
            let upd = NSMenuItem(title: "Проверить обновления…", action: #selector(checkUpdates), keyEquivalent: "")
            upd.target = self
            menu.addItem(upd)
        }
        menu.addItem(disabled("Pulsar \(appVersion)"))
        let quit = NSMenuItem(title: "Выйти", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q")
        menu.addItem(quit)
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
