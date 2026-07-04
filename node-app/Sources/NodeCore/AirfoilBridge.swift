// AirfoilBridge (spec 6.2 item 8, 4.4): drives Airfoil over AppleScript —
// launch, set NodeApp as the application source, connect configured speakers,
// poll connected/volume, reconnect drops with backoff, fall back to the local
// "Computer" output when every speaker is gone.
//
// Script texts and parsing are pure static functions (unit-tested). The exact
// dictionary wording gets its live confirmation in spike S5; the surrounding
// logic (polling, backoff, degradation) is final.
//
// NSAppleScript runs in-process so the Automation grant attributes to
// NodeApp.app (spec ch. 14 item 3).

import AppKit
import Foundation

public struct AirfoilSpeaker: Equatable {
    public let name: String
    public let connected: Bool
    public let volume: Double

    public init(name: String, connected: Bool, volume: Double) {
        self.name = name
        self.connected = connected
        self.volume = volume
    }
}

public final class AirfoilBridge {
    private let appPath: String
    private let sourceAppPath: String
    private let configured: [String]
    private let pollS: Int
    private let log: Logger
    private let queue = DispatchQueue(label: "duet.airfoil-bridge")

    private var pollTimer: DispatchSourceTimer?
    private var reconnectAttempts: [String: Int] = [:]
    private var nextReconnectAt: [String: Date] = [:]
    private var computerFallbackActive = false

    /// Published after every poll: configured speakers' states + degraded flag.
    public var onStates: (([SpeakerState], _ degraded: Bool) -> Void)?

    public init(appPath: String, sourceAppPath: String, speakers: [String], pollS: Int, log: Logger) {
        self.appPath = appPath
        self.sourceAppPath = sourceAppPath
        configured = speakers
        self.pollS = max(1, pollS)
        self.log = log
    }

    // MARK: Script builders (pure, unit-tested)

    static func escape(_ s: String) -> String {
        s.replacingOccurrences(of: "\\", with: "\\\\")
            .replacingOccurrences(of: "\"", with: "\\\"")
    }

    static func scriptSetSource(nodeAppPath: String) -> String {
        """
        tell application "Airfoil"
            set theSource to make new application source with properties {application file:POSIX file "\(escape(nodeAppPath))"}
            set current audio source to theSource
        end tell
        """
    }

    // Dictionary verified live against Airfoil 5.12.6 (spike S5, 2026-07-03):
    // the commands are "connect to" / "disconnect from".
    static func scriptConnect(speaker: String) -> String {
        """
        tell application "Airfoil"
            connect to (every speaker whose name is "\(escape(speaker))")
        end tell
        """
    }

    static func scriptDisconnect(speaker: String) -> String {
        """
        tell application "Airfoil"
            disconnect from (every speaker whose name is "\(escape(speaker))")
        end tell
        """
    }

    static func scriptStates() -> String {
        """
        tell application "Airfoil"
            set out to ""
            repeat with s in (every speaker)
                set out to out & (name of s) & tab & ((connected of s) as text) & tab & ((volume of s) as text) & linefeed
            end repeat
            return out
        end tell
        """
    }

    /// Parses the tab/linefeed table returned by scriptStates().
    static func parseStates(_ raw: String) -> [AirfoilSpeaker] {
        raw.split(separator: "\n").compactMap { line in
            let parts = line.split(separator: "\t", omittingEmptySubsequences: false)
            guard parts.count >= 3 else { return nil }
            return AirfoilSpeaker(
                name: String(parts[0]),
                connected: parts[1].lowercased() == "true",
                volume: Double(parts[2].replacingOccurrences(of: ",", with: ".")) ?? 0
            )
        }
    }

    /// Reconnect backoff: 5 s * 2^attempt, capped at 60 s (spec 4.4/6.6).
    static func backoffDelay(attempt: Int) -> TimeInterval {
        min(5 * pow(2, Double(attempt)), 60)
    }

    // MARK: Lifecycle

    public var installed: Bool {
        FileManager.default.fileExists(atPath: appPath)
    }

    public func start() {
        guard installed else {
            log.warn("Airfoil not installed — speaker delivery disabled until spike S4", ["app_path": appPath])
            return
        }
        queue.async {
            self.launchAirfoil()
            _ = self.run(Self.scriptSetSource(nodeAppPath: self.sourceAppPath), label: "set source")
            for s in self.configured {
                _ = self.run(Self.scriptConnect(speaker: s), label: "connect \(s)")
            }
            self.startPolling()
        }
    }

    public func stop() {
        queue.sync { pollTimer?.cancel(); pollTimer = nil }
    }

    private func launchAirfoil() {
        // Idempotent: launches or brings up the already-running app (spec 6.6).
        let ok = NSWorkspace.shared.open(URL(fileURLWithPath: appPath))
        if !ok { log.error("cannot launch Airfoil", ["app_path": appPath]) }
    }

    private func startPolling() {
        let t = DispatchSource.makeTimerSource(queue: queue)
        t.schedule(deadline: .now() + .seconds(pollS), repeating: .seconds(pollS))
        t.setEventHandler { [weak self] in self?.poll() }
        t.resume()
        pollTimer = t
    }

    private func poll() {
        guard let raw = run(Self.scriptStates(), label: "poll states") else {
            // Airfoil gone: relaunch and re-drive the setup (spec 6.6).
            log.warn("Airfoil poll failed, relaunching")
            launchAirfoil()
            _ = run(Self.scriptSetSource(nodeAppPath: sourceAppPath), label: "set source")
            return
        }
        let all = Self.parseStates(raw)
        let byName = Dictionary(uniqueKeysWithValues: all.map { ($0.name, $0) })

        var states: [SpeakerState] = []
        var anyConnected = false
        var degraded = false
        let now = Date()

        for name in configured {
            let connected = byName[name]?.connected ?? false
            states.append(SpeakerState(name: name, connected: connected))
            if connected {
                anyConnected = true
                reconnectAttempts[name] = 0
                nextReconnectAt[name] = nil
            } else {
                degraded = true
                let due = nextReconnectAt[name] ?? .distantPast
                if now >= due {
                    let attempt = reconnectAttempts[name, default: 0]
                    log.info("reconnecting speaker", ["name": name, "attempt": attempt])
                    _ = run(Self.scriptConnect(speaker: name), label: "reconnect \(name)")
                    reconnectAttempts[name] = attempt + 1
                    nextReconnectAt[name] = now.addingTimeInterval(Self.backoffDelay(attempt: attempt))
                }
            }
        }

        // Emergency local output when the whole home is silent (spec 4.4).
        if !anyConnected && !computerFallbackActive {
            log.warn("no speakers connected, engaging local Computer output")
            _ = run(Self.scriptConnect(speaker: "Computer"), label: "computer fallback")
            computerFallbackActive = true
        } else if anyConnected && computerFallbackActive {
            _ = run(Self.scriptDisconnect(speaker: "Computer"), label: "computer fallback off")
            computerFallbackActive = false
        }

        onStates?(states, degraded)
    }

    // osascript subprocess, not NSAppleScript: in a CLI process NSAppleScript
    // silently hangs off the main thread (observed live, spike S5). The child
    // inherits NodeApp as the TCC-responsible process, so the Automation
    // prompt/grant still binds to NodeApp.app (spec ch. 14).
    @discardableResult
    private func run(_ source: String, label: String) -> String? {
        let p = Process()
        p.executableURL = URL(fileURLWithPath: "/usr/bin/osascript")
        p.arguments = ["-e", source]
        let outPipe = Pipe()
        let errPipe = Pipe()
        p.standardOutput = outPipe
        p.standardError = errPipe
        do {
            try p.run()
        } catch {
            log.warn("osascript spawn failed", ["label": label, "err": "\(error)"])
            return nil
        }
        let deadline = Date().addingTimeInterval(10)
        while p.isRunning && Date() < deadline {
            usleep(50_000)
        }
        if p.isRunning {
            // 10 s: covers slow AppleScript; a pending TCC prompt takes longer —
            // kill and retry next poll so the queue never wedges (-1743 follows
            // once the user answers the dialog).
            p.terminate()
            log.warn("applescript timeout (TCC prompt pending?)", ["label": label])
            return nil
        }
        let out = String(data: outPipe.fileHandleForReading.readDataToEndOfFile(), encoding: .utf8) ?? ""
        let err = String(data: errPipe.fileHandleForReading.readDataToEndOfFile(), encoding: .utf8) ?? ""
        if p.terminationStatus != 0 {
            // -1743 = Automation permission missing/denied (spec ch. 14 item 3).
            log.warn("applescript failed", ["label": label, "err": err.trimmingCharacters(in: .whitespacesAndNewlines)])
            return nil
        }
        return out
    }
}
