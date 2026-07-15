// Pulsar entry point (spec ch. 6, goal v2 R1/R2): config or built-in
// defaults -> pairing (window or CLI) -> audio graph -> go-librespot
// supervision -> coordinator link. Menu-bar app; dock only during onboarding.

import AppKit
import Foundation
import NodeCore
import NodeAppUI
import Sparkle

let appVersion = "0.3.0-dev"

let app = NSApplication.shared

// Audio process must never nap: App Nap throttling of a background NSApp
// caused audible dropouts (spike S4, live). Keep the token for process life.
let activityToken = ProcessInfo.processInfo.beginActivity(
    options: [.userInitiated, .latencyCritical, .idleSystemSleepDisabled],
    reason: "pulsar realtime audio"
)

// Minimal main menu so Cmd+Q works while a window is up (onboarding).
let mainMenu = NSMenu()
let appMenuItem = NSMenuItem()
mainMenu.addItem(appMenuItem)
let appMenu = NSMenu()
appMenu.addItem(NSMenuItem(title: "Quit Pulsar", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q"))
appMenuItem.submenu = appMenu
// Edit menu: without it Cmd+V/C/X/A do nothing in the onboarding code field.
let editMenuItem = NSMenuItem()
mainMenu.addItem(editMenuItem)
let editMenu = NSMenu(title: "Edit")
editMenu.addItem(withTitle: "Undo", action: Selector(("undo:")), keyEquivalent: "z")
editMenu.addItem(withTitle: "Redo", action: Selector(("redo:")), keyEquivalent: "Z")
editMenu.addItem(.separator())
editMenu.addItem(withTitle: "Cut", action: #selector(NSText.cut(_:)), keyEquivalent: "x")
editMenu.addItem(withTitle: "Copy", action: #selector(NSText.copy(_:)), keyEquivalent: "c")
editMenu.addItem(withTitle: "Paste", action: #selector(NSText.paste(_:)), keyEquivalent: "v")
editMenu.addItem(withTitle: "Select All", action: #selector(NSText.selectAll(_:)), keyEquivalent: "a")
editMenuItem.submenu = editMenu
app.mainMenu = mainMenu

let defaultCoordinatorBase = "https://barycenter.relux.works"

func parseArgs() -> String {
    var configPath = ("~/duet/node.yml" as NSString).expandingTildeInPath
    var pairCode: String?
    var coordinatorBase = defaultCoordinatorBase
    var args = ArraySlice(CommandLine.arguments.dropFirst())
    while let arg = args.popFirst() {
        switch arg {
        case "--config":
            guard let value = args.popFirst() else {
                FileHandle.standardError.write(Data("--config requires a path\n".utf8))
                exit(2)
            }
            configPath = (value as NSString).expandingTildeInPath
        case "--pair":
            guard let value = args.popFirst() else {
                FileHandle.standardError.write(Data("--pair requires the code from the bot\n".utf8))
                exit(2)
            }
            pairCode = value
        case "--coordinator":
            guard let value = args.popFirst() else {
                FileHandle.standardError.write(Data("--coordinator requires a base URL\n".utf8))
                exit(2)
            }
            coordinatorBase = value
        case "--version":
            print(appVersion)
            exit(0)
        default:
            FileHandle.standardError.write(Data("unknown argument \(arg)\nusage: NodeApp [--config path] [--pair CODE [--coordinator https://…]] [--version]\n".utf8))
            exit(2)
        }
    }
    // CLI pairing mode (design §4): exchange the bot code, store, exit.
    if let code = pairCode {
        switch pairNode(code: code.uppercased(), coordinatorBase: coordinatorBase) {
        case .success(let creds):
            do {
                try CredentialsStore.save(creds)
            } catch {
                FileHandle.standardError.write(
                    Data("не удалось безопасно сохранить учётные данные в Keychain\n".utf8))
                exit(1)
            }
            print("спарено: орбит \(creds.orbitId), дом \(creds.slot) — запускай Pulsar как обычно")
            exit(0)
        case .failure(let err):
            FileHandle.standardError.write(Data((err.description + "\n").utf8))
            exit(1)
        }
    }
    return configPath
}

let configPath = parseArgs()

func failConfig(_ text: String) -> Never {
    FileHandle.standardError.write(Data((text + "\n").utf8))
    if isatty(STDERR_FILENO) == 0 {
        let alert = NSAlert()
        alert.messageText = "Pulsar не может запуститься"
        alert.informativeText = text + "\n\nConfig: \(configPath)"
        alert.runModal()
    }
    exit(1)
}

// CoreRuntime owns every live component; built once the node is paired.
final class CoreRuntime {
    let log: Logger
    let engine: AudioEngine
    let supervisor: LibrespotSupervisor
    let librespot: LibrespotClient
    let player: PlayerCore
    let client: CoordinatorClient
    var airfoil: AirfoilBridge?
    var outputMonitor: DirectOutputMonitor?

    private init(log: Logger, engine: AudioEngine, supervisor: LibrespotSupervisor,
                 librespot: LibrespotClient, player: PlayerCore, client: CoordinatorClient) {
        self.log = log
        self.engine = engine
        self.supervisor = supervisor
        self.librespot = librespot
        self.player = player
        self.client = client
    }

    static func start(config: NodeConfig) throws -> CoreRuntime {
        let log = Logger(level: Logger.Level(name: config.log.level), path: config.log.path)
        log.info("Pulsar starting", [
            "version": appVersion,
            "node_id": config.nodeId,
            "coordinator_url": config.coordinator.url,
            "fifo_path": config.audio.fifoPath,
        ])

        guard let wsURL = URL(string: config.coordinator.url) else {
            throw ConfigError(problems: ["coordinator.url is not a URL: \(config.coordinator.url)"])
        }

        let engine = AudioEngine(fifoPath: config.audio.fifoPath, ringMs: config.audio.ringBufferMs, log: log)
        try engine.start()

        let supervisor = LibrespotSupervisor(
            binary: config.effectiveLibrespotBinary,
            configDir: config.librespot.configDir ?? LibrespotConfigRenderer.defaultConfigDir,
            log: log
        )
        let librespot = LibrespotClient(apiPort: config.librespot.apiPort, log: log)
        try supervisor.start(deviceName: config.effectiveDeviceName,
                             apiPort: config.librespot.apiPort,
                             fifoPath: config.audio.fifoPath)
        librespot.startEvents()

        let cache = VoiceCache(cacheDir: config.cacheDir, nodeToken: config.coordinator.token, log: log)
        let player = PlayerCore(engine: engine, librespot: librespot, supervisor: supervisor,
                                cache: cache, outputLatencyOffsetMs: config.audio.outputLatencyOffsetMs, log: log)
        let cacheRoot = URL(fileURLWithPath: config.cacheDir, isDirectory: true)
        try player.configureTransmissionHooks(
            cacheDirectory: cacheRoot.appendingPathComponent("media-clips", isDirectory: true),
            nodeToken: config.coordinator.token,
            coordinatorURL: wsURL,
            localStateURL: cacheRoot.deletingLastPathComponent()
                .appendingPathComponent("node-state-\(config.nodeId).json"))
        player.setLocalVolume(80) // spec 6.3 default; coordinator pushes the saved value

        let client = CoordinatorClient(
            url: wsURL,
            identity: .init(
                nodeId: config.nodeId,
                token: config.coordinator.token,
                appVersion: appVersion,
                librespotVersion: supervisor.version
            ),
            capabilities: player.advertisedCapabilities,
            log: log
        )
        player.coordinator = client

        let rt = CoreRuntime(log: log, engine: engine, supervisor: supervisor,
                             librespot: librespot, player: player, client: client)

        // Menu-bar picker persists the choice; it overrides the yml value.
        let pickedOutput = UserDefaults.standard.string(forKey: "outputDevice")
        let desiredOutput = pickedOutput ?? config.audio.outputDevice

        if config.airfoil.isEnabled {
            let bridge = AirfoilBridge(
                appPath: config.airfoil.appPath,
                sourceAppPath: Bundle.main.bundlePath,
                speakers: config.airfoil.speakers,
                pollS: config.airfoil.pollS,
                log: log
            )
            bridge.onStates = { states, degraded in player.updateSpeakers(states, degraded: degraded) }
            bridge.start()
            rt.airfoil = bridge
            log.info("delivery mode: airfoil", ["speakers": config.airfoil.speakers.joined(separator: ",")])
        } else {
            let monitor = DirectOutputMonitor(
                desiredDeviceName: desiredOutput,
                pollS: config.airfoil.pollS,
                log: log
            )
            monitor.onStates = { states, degraded in player.updateSpeakers(states, degraded: degraded) }
            monitor.start()
            rt.outputMonitor = monitor
            log.info("delivery mode: direct", ["output_device": desiredOutput ?? "(any)"])
        }

        client.stateProvider = {
            let fallback: [SpeakerState]
            if config.airfoil.isEnabled {
                fallback = config.airfoil.speakers.map { SpeakerState(name: $0, connected: false) }
            } else {
                fallback = [SpeakerState(name: desiredOutput ?? "system output", connected: false)]
            }
            return player.statePayload(fallbackSpeakers: fallback, rttMs: client.clock.lastRttMs)
        }

        client.onMessage = { head, message in
            if case .welcome(let w) = message {
                client.markHealthy()
                log.info("welcome received", [
                    "mode": w.sessionSnapshot.mode,
                    "state": w.sessionSnapshot.state,
                    "volume": w.sessionSnapshot.volume,
                ])
                player.applyWelcome(w)
                return
            }
            player.handle(head, message)
        }
        client.onConnected = { player.resendLocalDND() }

        client.start()
        return rt
    }

    // teardown stops every live component WITHOUT exiting the process — for
    // re-pairing in place (F3). The activity token stays; the app lives on to
    // show the onboarding window and start a fresh core.
    func teardown() {
        log.info("core teardown (re-pair)")
        client.stop()
        player.stopTransmissionHooks()
        librespot.stopEvents()
        supervisor.stop()
        airfoil?.stop()
        outputMonitor?.stop()
        engine.stopEngine()
        Thread.sleep(forTimeInterval: 0.2)
    }

    func shutdown() {
        log.info("shutting down")
        // 5 s (was 2): the hard watchdog raced Sparkle's relaunch agent —
        // the host died before the updater coordinated the restart and the
        // Updater.app hung forever (beta finding, 2026-07-07).
        DispatchQueue.global().asyncAfter(deadline: .now() + 5) { _exit(1) }
        teardown()
        ProcessInfo.processInfo.endActivity(activityToken)
        exit(0)
    }
}

// --- Bootstrap: paired -> core + menu bar; unpaired -> onboarding window ---

var runtime: CoreRuntime?
@MainActor var macCaptureComposition: MacCaptureAppComposition?
@MainActor var macPhaseOneComposition: MacPhaseOneAppComposition?
@MainActor var macIdentityComposition: MacIdentityAppComposition?

final class LocalCaptureAudioRuntime {
    let log: Logger
    let engine: AudioEngine

    init(config: NodeConfig) throws {
        log = Logger(level: Logger.Level(name: config.log.level), path: config.log.path)
        engine = AudioEngine(
            fifoPath: config.audio.fifoPath,
            ringMs: config.audio.ringBufferMs,
            log: log)
        try engine.start()
    }

    func stop() { engine.stopEngine() }
}

@MainActor var localCaptureAudioRuntime: LocalCaptureAudioRuntime?
@MainActor var shellRefreshTimer: Timer?
@MainActor var shellConfiguredRoute: String?
@MainActor var shellModel: PulsarShellModel!
@MainActor var shellActions: PulsarShellActions!
@MainActor var mainWindow: PulsarMainWindowController!
@MainActor var statusMenu: StatusMenuController!
let onboarding = OnboardingWindowController()
// Sparkle: feed URL + EdDSA public key live in Info.plist (build-app.sh).
// Bare-binary runs (dev/CLI) have no bundle keys — the controller stays idle.
let updater = SPUStandardUpdaterController(startingUpdater: Bundle.main.bundleIdentifier != nil,
                                           updaterDelegate: nil, userDriverDelegate: nil)

func materializeSupportTree(_ config: NodeConfig) {
    for dir in [ConfigLoader.supportDir, config.cacheDir,
                config.librespot.configDir ?? ConfigLoader.supportDir + "/librespot",
                (config.log.path as NSString).deletingLastPathComponent] {
        try? FileManager.default.createDirectory(atPath: dir, withIntermediateDirectories: true)
    }
    if !FileManager.default.fileExists(atPath: config.audio.fifoPath) {
        mkfifo(config.audio.fifoPath, 0o600)
    }
}

@MainActor
func startCore(with config: NodeConfig) {
    // L6: never stack a second core on a live one (two librespots fighting
    // for one token = hub last-write-wins flapping). Any path that reaches
    // here with a runtime still up tears it down first.
    stopMacCaptureComposition()
    if let old = runtime {
        old.teardown()
        runtime = nil
    }
    materializeSupportTree(config)
    do {
        let rt = try CoreRuntime.start(config: config)
        runtime = rt
        statusMenu.player = rt.player
        statusMenu.updater = updater.updater
        statusMenu.coordinatorConnected = { [weak client = rt.client] in
            client?.isHealthy == true
        }
        statusMenu.connectionIdentity = connectionIdentity(config)
        statusMenu.rePairAction = { rePairFlow() }
        shellConfiguredRoute = config.airfoil.isEnabled
            ? (["Airfoil"] + config.airfoil.speakers).joined(separator: " · ")
            : nil
        app.setActivationPolicy(config.airfoil.isEnabled ? .regular : .accessory)
        startShellRefresh(identity: connectionIdentity(config))
        startMacCaptureComposition(audio: rt.engine, log: rt.log)
        startMacPhaseOneComposition(log: rt.log)
        mainWindow.show()
    } catch let err as ConfigError {
        failConfig(err.description)
    } catch {
        failConfig("запуск не удался: \(error)")
    }
}

// connectionIdentity renders "host · дом slot" for the menu (F3).
func connectionIdentity(_ config: NodeConfig) -> String {
    let host = URL(string: config.coordinator.url)?.host ?? config.coordinator.url
    return "\(host) · дом \(config.nodeId)"
}

// finishPairing starts the core after a successful pair and surfaces the
// one-time Spotify step (#4): pairing only links this mac to the air — nothing
// plays until Spotify picks "Pulsar" once (Premium required), so without this
// the first track fails as track_unavailable and reads as "broken". The same
// help stays in the menu bar afterwards ("Как включить звук").
@MainActor
func finishPairing(_ paired: NodeConfig) {
    startCore(with: paired)
    DispatchQueue.main.async { SpotifyHelp.presentHowToSound() }
}

// rePairFlow tears down the running core and reopens the onboarding window
// so the user can pair against a fresh code (F3). A successful pair starts a
// new core in place — no relaunch.
@MainActor
func rePairFlow() {
    shellRefreshTimer?.invalidate()
    shellRefreshTimer = nil
    shellConfiguredRoute = nil
    stopMacCaptureComposition()
    runtime?.teardown()
    runtime = nil
    statusMenu.player = nil
    // L6: hide "Подключить заново…" while already unpaired — clicking it in
    // that state opened a duplicate onboarding window. startCore restores it.
    statusMenu.rePairAction = nil
    shellModel.replaceSnapshot(.init(connection: .unpaired))
    app.setActivationPolicy(.regular)
    // Re-pair: LAN access was settled on the first run — don't re-prime it.
    onboarding.show(coordinatorBase: defaultCoordinatorBase, promptForNetwork: false) { _ in
        guard let paired = try? ConfigLoader.load(
            path: configPath,
            credentials: CredentialsStore.load(besideConfig: configPath)) else {
            failConfig("креды сохранены, но конфиг не собрался — перезапусти Pulsar")
        }
        finishPairing(paired)
    }
}

@MainActor
func bootstrap() {
    configureShell()
    statusMenu.shellModel = shellModel
    statusMenu.shellActions = shellActions
    statusMenu.showMainWindowAction = { mainWindow.show() }
    statusMenu.install()
    // F2b: a dev-era ~/duet/node.yml pointing at a local coordinator hijacks
    // a paired app onto a dead server after moving to prod — retire it.
    let defaultConfig = ("~/duet/node.yml" as NSString).expandingTildeInPath
    if configPath == defaultConfig, ConfigLoader.retireLegacyLocalConfig(path: configPath) {
        NSLog("legacy dev config retired: %@.retired", configPath)
    }
    // An explicit yml with its own coordinator.token wins over keychain
    // credentials (sandbox/dev nodes must not steal the paired slot).
    let config: NodeConfig
    do {
        let plain = try ConfigLoader.load(path: configPath)
        if plain.coordinator.token.isEmpty {
            config = try ConfigLoader.load(
                path: configPath,
                credentials: CredentialsStore.load(besideConfig: configPath))
        } else {
            config = plain
        }
    } catch let err as ConfigError {
        failConfig(err.description)
    } catch {
        failConfig("config load failed: \(error)")
    }

    if config.coordinator.token.isEmpty {
        // Unpaired: onboarding window (R2). CLI users can still --pair.
        if isatty(STDERR_FILENO) == 1 {
            failConfig("""
            Пульсар ещё не спарен с Барицентром.
            В Telegram: @barycenter_bot → /pair (или /create), затем: NodeApp --pair КОД
            """)
        }
        app.setActivationPolicy(.regular)
        shellModel.replaceSnapshot(.init(connection: .unpaired))
        startAccountlessMacCapture(config: config)
        // First launch: prime the Local Network permission before pairing, so the
        // system prompt lands on an explained button — not out of nowhere while a
        // headless daemon touches the LAN (the failure that hid Timur's speaker).
        onboarding.show(coordinatorBase: defaultCoordinatorBase, promptForNetwork: true) { _ in
            let paired = try? ConfigLoader.load(
                path: configPath,
                credentials: CredentialsStore.load(besideConfig: configPath))
            guard let paired else {
                failConfig("креды сохранены, но конфиг не собрался — перезапусти Pulsar")
            }
            finishPairing(paired)
        }
        return
    }
    startCore(with: config)
}

@MainActor
func configureShell() {
    guard shellModel == nil else { return }
    shellModel = PulsarShellModel()
    shellActions = PulsarShellActions(
        createOrbit: { showShellSection(.create) },
        joinOrbit: { showShellSection(.join) },
        tryLocally: { showShellSection(.tryLocally) },
        setDND: { mode in
            guard let player = runtime?.player else { return }
            do {
                try player.setLocalDND(mode: mode.rawValue)
                shellModel.setDNDMode(mode)
            } catch {
                shellModel.updateConnection(
                    .degraded("DND update failed"),
                    identity: shellModel.snapshot.connectionIdentity)
            }
        },
        setVolume: { volume in
            runtime?.player.setLocalVolume(volume)
            shellModel.setVolume(volume)
        },
        toggleRecording: { macCaptureComposition?.toggleRecording() },
        cancelRecording: { macCaptureComposition?.cancelRecording() },
        setCaptureDevice: { macCaptureComposition?.selectDevice($0) },
        setRecordingShortcut: { macCaptureComposition?.setShortcut($0) },
        playBuiltinCue: { macCaptureComposition?.playBuiltinCue() },
        recordFiveSeconds: { macCaptureComposition?.recordFiveSeconds() },
        reviewLocalFile: { macCaptureComposition?.reviewFile($0) },
        acceptLocalFile: { macCaptureComposition?.acceptFile($0) },
        deleteLocalDraft: { macCaptureComposition?.deleteLocalDraft() },
        closeSelfTest: { macCaptureComposition?.closeSelfTest() },
        sendDraft: { id, route, delivery in
            macPhaseOneComposition?.send(draftID: id, route: route, delivery: delivery)
        },
        deleteOutgoingDraft: { id in macPhaseOneComposition?.delete(draftID: id) },
        refreshPhaseOneData: { macPhaseOneComposition?.refresh(force: true) },
        historyAction: { id, request in
            macPhaseOneComposition?.performHistoryAction(id, request: request)
        },
        submitCreateOrbit: { macIdentityComposition?.create(title: $0) },
        submitJoinOrbit: { macIdentityComposition?.join(code: $0) },
        exportRecovery: { macIdentityComposition?.exportRecovery() }
    )
    mainWindow = PulsarMainWindowController(model: shellModel, actions: shellActions)
    statusMenu = StatusMenuController()
    startMacIdentityComposition()
}

@MainActor
func startMacIdentityComposition() {
    guard macIdentityComposition == nil else { return }
    do {
        macIdentityComposition = try MacIdentityAppComposition(
            coordinator: defaultCoordinatorBase,
            model: shellModel,
            onCredentialsActivated: { activateStoredCredentials() })
    } catch {
        shellModel.setIdentityOperation(.failed("Identity service is unavailable"))
    }
}

@MainActor
func activateStoredCredentials() {
    guard let credentials = CredentialsStore.load(besideConfig: configPath),
          let paired = try? ConfigLoader.load(path: configPath, credentials: credentials) else {
        shellModel.setIdentityOperation(.failed("Saved credentials could not be activated"))
        return
    }
    finishPairing(paired)
}

@MainActor
func startMacCaptureComposition(audio: AudioEngine, log: Logger) {
    do {
        let composition = try MacCaptureAppComposition(
            audio: audio,
            log: log,
            supportRoot: URL(fileURLWithPath: ConfigLoader.supportDir, isDirectory: true),
            model: shellModel)
        macCaptureComposition = composition
        composition.start()
    } catch {
        log.error("mac capture composition unavailable", ["reason": "initialization_failed"])
        shellModel.setRecording(.unavailable, available: false)
        shellModel.setSelfTestAvailable(false)
    }
}

@MainActor
func startMacPhaseOneComposition(log: Logger) {
    guard let capture = macCaptureComposition else { return }
    do {
        guard let bundle = try CredentialsStore.loadBundle(besideConfig: configPath) else {
            return
        }
        let composition = try MacPhaseOneAppComposition(
            bundle: bundle,
            supportRoot: URL(fileURLWithPath: ConfigLoader.supportDir, isDirectory: true),
            capture: capture,
            model: shellModel)
        macPhaseOneComposition = composition
        composition.start()
    } catch {
        log.error("phase one app data unavailable", ["reason": "initialization_failed"])
        shellModel.setPhaseOneData(
            presenceSummary: nil,
            failure: "Authenticated app data is unavailable")
    }
}

@MainActor
func startAccountlessMacCapture(config: NodeConfig) {
    materializeSupportTree(config)
    do {
        let local = try LocalCaptureAudioRuntime(config: config)
        localCaptureAudioRuntime = local
        startMacCaptureComposition(audio: local.engine, log: local.log)
    } catch {
        localCaptureAudioRuntime?.stop()
        localCaptureAudioRuntime = nil
        shellModel.setRecording(.unavailable, available: false)
        shellModel.setSelfTestAvailable(false)
    }
}

@MainActor
func stopMacCaptureComposition() {
    macPhaseOneComposition?.shutdown()
    macPhaseOneComposition = nil
    macCaptureComposition?.shutdown()
    macCaptureComposition = nil
    localCaptureAudioRuntime?.stop()
    localCaptureAudioRuntime = nil
}

@MainActor
func openBotFlow(_ flow: String) {
    _ = flow
    guard let url = URL(string: "https://t.me/barycenter_bot") else { return }
    NSWorkspace.shared.open(url)
}

@MainActor
func showShellSection(_ section: PulsarShellSection) {
    mainWindow.show(section: section)
}

@MainActor
func startShellRefresh(identity: String) {
    shellRefreshTimer?.invalidate()
    refreshShell(identity: identity)
    shellRefreshTimer = Timer.scheduledTimer(withTimeInterval: 1, repeats: true) { _ in
        Task { @MainActor in refreshShell(identity: identity) }
    }
}

@MainActor
func refreshShell(identity: String) {
    guard let runtime else {
        shellModel.replaceSnapshot(.init(connection: .unpaired))
        return
    }
    let status = runtime.player.menuStatus()
    let connection: PulsarConnectionState = runtime.client.isHealthy ? .online : .reconnecting
    let route = shellConfiguredRoute ?? DirectOutputMonitor.currentOutputName()
    let dnd = PulsarDNDMode(rawValue: runtime.player.localDNDMode) ?? .allowAll
    shellModel.updateConnection(connection, identity: identity)
    shellModel.updateRuntime(
        routeName: route,
        nowPlaying: status.title ?? status.uri.map(shortShellURI),
        playbackState: status.playback,
        dndMode: dnd,
        volume: status.volume)
    macPhaseOneComposition?.refresh()
}

func shortShellURI(_ uri: String) -> String {
    uri.hasPrefix("spotify:track:") ? String(uri.dropFirst("spotify:".count)) : uri
}

// --- Shutdown paths ---

let signalQueue = DispatchQueue(label: "pulsar.signals")
let sigint = DispatchSource.makeSignalSource(signal: SIGINT, queue: signalQueue)
let sigterm = DispatchSource.makeSignalSource(signal: SIGTERM, queue: signalQueue)
signal(SIGINT, SIG_IGN)
signal(SIGTERM, SIG_IGN)
func shutdown() {
    if Thread.isMainThread {
        MainActor.assumeIsolated {
            macIdentityComposition?.shutdown()
            macIdentityComposition = nil
            stopMacCaptureComposition()
        }
    } else {
        DispatchQueue.main.sync {
            macIdentityComposition?.shutdown()
            macIdentityComposition = nil
            stopMacCaptureComposition()
        }
    }
    if let rt = runtime {
        rt.shutdown()
    } else {
        exit(0)
    }
}
for source in [sigint, sigterm] {
    source.setEventHandler { shutdown() }
    source.resume()
}
_ = NotificationCenter.default.addObserver(
    forName: NSApplication.willTerminateNotification, object: nil, queue: nil
) { _ in shutdown() }

DispatchQueue.main.async { bootstrap() }
app.run()
