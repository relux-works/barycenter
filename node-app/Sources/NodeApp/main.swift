// NodeApp entry point (spec ch. 6): config -> logger -> audio graph ->
// go-librespot supervision -> coordinator link -> command execution.
// AirfoilBridge is wired in once Airfoil is installed (spike S4/S5).

import AppKit
import Foundation
import NodeCore

let appVersion = "0.1.0-dev"

// Full NSApplication lifecycle: Airfoil lists only *regular, fully launched*
// apps as capture sources, and LaunchServices bounces the Dock icon forever
// unless the app reaches finishLaunching (spike S4 findings). The event loop
// at the bottom of this file is NSApp.run(), not RunLoop.main.run().
let app = NSApplication.shared
app.setActivationPolicy(.regular)

// Audio process must never nap: App Nap throttling of a background NSApp
// caused audible dropouts (spike S4, live). Keep the token for process life.
let activityToken = ProcessInfo.processInfo.beginActivity(
    options: [.userInitiated, .latencyCritical, .idleSystemSleepDisabled],
    reason: "duet realtime audio"
)

// Minimal main menu so Cmd+Q works; quit routes through NSApp.terminate.
let mainMenu = NSMenu()
let appMenuItem = NSMenuItem()
mainMenu.addItem(appMenuItem)
let appMenu = NSMenu()
appMenu.addItem(NSMenuItem(title: "Quit NodeApp", action: #selector(NSApplication.terminate(_:)), keyEquivalent: "q"))
appMenuItem.submenu = appMenu
app.mainMenu = mainMenu

func parseArgs() -> String {
    var configPath = ("~/duet/node.yml" as NSString).expandingTildeInPath
    var pairCode: String?
    var coordinatorBase = "https://barycenter.relux.works"
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
    // Pairing mode (design §4): exchange the bot code for credentials,
    // store them beside the config, exit. The normal start picks them up.
    if let code = pairCode {
        switch pairNode(code: code.uppercased(), coordinatorBase: coordinatorBase) {
        case .success(let creds):
            do {
                try creds.save(besideConfig: configPath)
            } catch {
                FileHandle.standardError.write(Data("не смог сохранить креды: \(error)\n".utf8))
                exit(1)
            }
            print("спарено: орбит \(creds.orbitId), дом \(creds.slot) — запускай NodeApp как обычно")
            exit(0)
        case .failure(let err):
            FileHandle.standardError.write(Data((err.description + "\n").utf8))
            exit(1)
        }
    }
    return configPath
}

let configPath = parseArgs()

// A GUI launch (Finder/Dock double-click) has no visible stderr — a silent
// instant exit looks like "the app does not start" (spike S4, live). Show the
// config problem in an alert instead.
func failConfig(_ text: String) -> Never {
    FileHandle.standardError.write(Data((text + "\n").utf8))
    if isatty(STDERR_FILENO) == 0 {
        let alert = NSAlert()
        alert.messageText = "NodeApp cannot start"
        alert.informativeText = text + "\n\nConfig path: \(configPath)\n(runbook §1: install places the template at ~/duet/node.yml)"
        alert.runModal()
    }
    exit(1)
}

let config: NodeConfig
do {
    // Pairing credentials (node-credentials.json beside the yml) override the
    // yml's coordinator section — after `--pair` nobody edits configs.
    config = try ConfigLoader.load(
        path: configPath,
        credentials: NodeCredentials.load(besideConfig: configPath))
} catch let err as ConfigError {
    failConfig(err.description)
} catch {
    failConfig("config load failed: \(error)")
}

let log = Logger(level: Logger.Level(name: config.log.level), path: config.log.path)
// Startup snapshot without secrets (goal 3.5).
log.info("NodeApp starting", [
    "version": appVersion,
    "node_id": config.nodeId,
    "coordinator_url": config.coordinator.url,
    "fifo_path": config.audio.fifoPath,
    "ring_buffer_ms": config.audio.ringBufferMs,
    "output_latency_offset_ms": config.audio.outputLatencyOffsetMs,
    "speakers": config.airfoil.speakers.joined(separator: ","),
])

guard let wsURL = URL(string: config.coordinator.url) else {
    log.error("coordinator.url is not a URL", ["url": config.coordinator.url])
    exit(1)
}

// --- Audio graph ---

let engine = AudioEngine(fifoPath: config.audio.fifoPath, ringMs: config.audio.ringBufferMs, log: log)
do {
    try engine.start()
} catch {
    log.error("audio engine failed to start", ["err": "\(error)"])
    exit(1)
}

// --- go-librespot supervision + API client ---

let supervisor = LibrespotSupervisor(
    binary: config.librespot.binary,
    configDir: config.librespot.configDir ?? LibrespotConfigRenderer.defaultConfigDir,
    log: log
)
let librespot = LibrespotClient(apiPort: config.librespot.apiPort, log: log)
do {
    try supervisor.start(deviceName: config.effectiveDeviceName,
                         apiPort: config.librespot.apiPort,
                         fifoPath: config.audio.fifoPath)
} catch let err as ConfigError {
    FileHandle.standardError.write(Data((err.description + "\n").utf8))
    exit(1)
} catch {
    log.error("librespot start failed", ["err": "\(error)"])
    exit(1)
}
librespot.startEvents()

// --- Player core + coordinator link ---

let cache = VoiceCache(cacheDir: config.cacheDir, nodeToken: config.coordinator.token, log: log)
let player = PlayerCore(engine: engine, librespot: librespot, supervisor: supervisor,
                        cache: cache, outputLatencyOffsetMs: config.audio.outputLatencyOffsetMs, log: log)
player.setVolume(80) // spec 6.3 default; coordinator pushes the saved value after welcome

// --- Speaker delivery (spec v1.3, 6.2 item 8): direct by default, Airfoil opt-in ---

var airfoil: AirfoilBridge?
var outputMonitor: DirectOutputMonitor?
if config.airfoil.isEnabled {
    let bridge = AirfoilBridge(
        appPath: config.airfoil.appPath,
        sourceAppPath: Bundle.main.bundlePath, // the .app bundle Airfoil captures
        speakers: config.airfoil.speakers,
        pollS: config.airfoil.pollS,
        log: log
    )
    bridge.onStates = { states, degraded in
        player.updateSpeakers(states, degraded: degraded)
    }
    bridge.start()
    airfoil = bridge
    log.info("delivery mode: airfoil", ["speakers": config.airfoil.speakers.joined(separator: ",")])
} else {
    let monitor = DirectOutputMonitor(
        desiredDeviceName: config.audio.outputDevice,
        pollS: config.airfoil.pollS,
        log: log
    )
    monitor.onStates = { states, degraded in
        player.updateSpeakers(states, degraded: degraded)
    }
    monitor.start()
    outputMonitor = monitor
    log.info("delivery mode: direct", ["output_device": config.audio.outputDevice ?? "(any)"])
}

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

client.stateProvider = {
    let fallback: [SpeakerState]
    if config.airfoil.isEnabled {
        fallback = config.airfoil.speakers.map { SpeakerState(name: $0, connected: false) }
    } else {
        fallback = [SpeakerState(name: config.audio.outputDevice ?? "system output", connected: false)]
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

// --- Shutdown ---

// Signal sources live on a dedicated queue: a CLI RunLoop does not reliably
// drain the main dispatch queue, and launchd bootout must terminate us cleanly
// (proven by the two-node simulation run).
let signalQueue = DispatchQueue(label: "duet.signals")
let sigint = DispatchSource.makeSignalSource(signal: SIGINT, queue: signalQueue)
let sigterm = DispatchSource.makeSignalSource(signal: SIGTERM, queue: signalQueue)
signal(SIGINT, SIG_IGN)
signal(SIGTERM, SIG_IGN)
func shutdown() {
    log.info("shutting down")
    // Watchdog first: whatever hangs below, launchd gets its exit <= 2 s.
    DispatchQueue.global().asyncAfter(deadline: .now() + 2) { _exit(1) }
    client.stop()
    librespot.stopEvents()
    supervisor.stop()
    airfoil?.stop()
    outputMonitor?.stop()
    engine.stopEngine()
    ProcessInfo.processInfo.endActivity(activityToken)
    Thread.sleep(forTimeInterval: 0.2) // let the WS close frame flush
    exit(0)
}

for source in [sigint, sigterm] {
    source.setEventHandler { shutdown() }
    source.resume()
}

// Cmd+Q / AppleEvent quit path (NSApp.terminate) also shuts down cleanly.
_ = NotificationCenter.default.addObserver(
    forName: NSApplication.willTerminateNotification, object: nil, queue: nil
) { _ in shutdown() }

app.run()
