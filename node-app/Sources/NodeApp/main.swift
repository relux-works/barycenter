// Pulsar entry point (spec ch. 6, goal v2 R1/R2): config or built-in
// defaults -> pairing (window or CLI) -> audio graph -> go-librespot
// supervision -> coordinator link. Menu-bar app; dock only during onboarding.

import AppKit
import Foundation
import NodeCore
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
                // Headless keychain (rare): fall back to the legacy file.
                try? creds.save(besideConfig: configPath)
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
        player.setVolume(80) // spec 6.3 default; coordinator pushes the saved value

        let client = CoordinatorClient(
            url: wsURL,
            identity: .init(
                nodeId: config.nodeId,
                token: config.coordinator.token,
                appVersion: appVersion,
                librespotVersion: supervisor.version
            ),
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
                player.setVolume(w.sessionSnapshot.volume)
                return
            }
            player.handle(head, message)
        }

        client.start()
        return rt
    }

    func shutdown() {
        log.info("shutting down")
        DispatchQueue.global().asyncAfter(deadline: .now() + 2) { _exit(1) }
        client.stop()
        librespot.stopEvents()
        supervisor.stop()
        airfoil?.stop()
        outputMonitor?.stop()
        engine.stopEngine()
        ProcessInfo.processInfo.endActivity(activityToken)
        Thread.sleep(forTimeInterval: 0.2)
        exit(0)
    }
}

// --- Bootstrap: paired -> core + menu bar; unpaired -> onboarding window ---

var runtime: CoreRuntime?
let statusMenu = StatusMenuController()
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

func startCore(with config: NodeConfig) {
    materializeSupportTree(config)
    do {
        let rt = try CoreRuntime.start(config: config)
        runtime = rt
        statusMenu.player = rt.player
        statusMenu.updater = updater.updater
        statusMenu.coordinatorConnected = { true } // refined by heartbeat later
        app.setActivationPolicy(config.airfoil.isEnabled ? .regular : .accessory)
    } catch let err as ConfigError {
        failConfig(err.description)
    } catch {
        failConfig("запуск не удался: \(error)")
    }
}

func bootstrap() {
    statusMenu.install()
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
        onboarding.show(coordinatorBase: defaultCoordinatorBase) { _ in
            let paired = try? ConfigLoader.load(
                path: configPath,
                credentials: CredentialsStore.load(besideConfig: configPath))
            guard let paired else {
                failConfig("креды сохранены, но конфиг не собрался — перезапусти Pulsar")
            }
            startCore(with: paired)
        }
        return
    }
    startCore(with: config)
}

// --- Shutdown paths ---

let signalQueue = DispatchQueue(label: "pulsar.signals")
let sigint = DispatchSource.makeSignalSource(signal: SIGINT, queue: signalQueue)
let sigterm = DispatchSource.makeSignalSource(signal: SIGTERM, queue: signalQueue)
signal(SIGINT, SIG_IGN)
signal(SIGTERM, SIG_IGN)
func shutdown() {
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
