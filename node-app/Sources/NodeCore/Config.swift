// node.yml loading and validation (spec 6.4, appendix A.1).
// Validation failures must read like advice, not a stack trace (goal DoD-10).

import Foundation
import Yams

public struct NodeConfig: Codable, Equatable {
    public struct Coordinator: Codable, Equatable {
        public var url: String
        public var token: String
    }
    public struct Audio: Codable, Equatable {
        public var fifoPath: String
        public var sampleRate: Int
        public var format: String
        public var outputLatencyOffsetMs: Int
        public var ringBufferMs: Int
        /// v1.3 direct mode: the home's output device name (Control Center);
        /// empty/absent = accept whatever output is current.
        public var outputDevice: String?
        enum CodingKeys: String, CodingKey {
            case fifoPath = "fifo_path", sampleRate = "sample_rate", format,
                 outputLatencyOffsetMs = "output_latency_offset_ms",
                 ringBufferMs = "ring_buffer_ms",
                 outputDevice = "output_device"
        }
    }
    public struct Airfoil: Codable, Equatable {
        /// v1.3: false = direct mode (default); true = Airfoil (macOS 15 only).
        public var enabled: Bool?
        public var appPath: String
        public var speakers: [String]
        public var pollS: Int
        public var isEnabled: Bool { enabled ?? false }
        enum CodingKeys: String, CodingKey {
            case enabled, appPath = "app_path", speakers, pollS = "poll_s"
        }
    }
    public struct Librespot: Codable, Equatable {
        /// Daemon binary. Empty/absent = resolve automatically: bundled
        /// go-librespot inside Pulsar.app, then the brew path (R1: the app
        /// is self-contained; explicit value is a dev override).
        public var binary: String?
        public var apiPort: Int
        /// Spotify Connect device name. Optional: defaults to "Pulsar A"/"Pulsar B"
        /// by node_id — each user only sees their own node in their home Wi-Fi.
        public var deviceName: String?
        /// Optional override of the daemon config dir (default: the daemon's
        /// own ~/Library/Application Support/go-librespot). Used by local
        /// two-node simulations; production nodes keep the default so the
        /// first-login credentials are picked up (spec ch. 13).
        public var configDir: String?
        enum CodingKeys: String, CodingKey {
            case binary, apiPort = "api_port", deviceName = "device_name",
                 configDir = "config_dir"
        }
    }
    public struct Log: Codable, Equatable {
        public var level: String
        public var path: String
    }

    /// Effective daemon binary path: explicit config -> bundled -> brew.
    public var effectiveLibrespotBinary: String {
        if let b = librespot.binary, !b.isEmpty { return b }
        if let bundled = Bundle.main.path(forAuxiliaryExecutable: "go-librespot"),
           FileManager.default.isExecutableFile(atPath: bundled) {
            return bundled
        }
        // Unbundled (CLI/dev) fallback: next to our own executable.
        if let exe = Bundle.main.executablePath {
            let sibling = (exe as NSString).deletingLastPathComponent + "/go-librespot"
            if FileManager.default.isExecutableFile(atPath: sibling) { return sibling }
        }
        return "/opt/homebrew/opt/go-librespot/bin/go-librespot"
    }

    /// Effective Spotify Connect name: explicit config or "Pulsar A"/"Pulsar B".
    public var effectiveDeviceName: String {
        if let name = librespot.deviceName, !name.isEmpty { return name }
        return "Pulsar \(nodeId.uppercased())"
    }

    public var nodeId: String
    public var coordinator: Coordinator
    public var audio: Audio
    public var airfoil: Airfoil
    public var librespot: Librespot
    public var cacheDir: String
    public var log: Log

    enum CodingKeys: String, CodingKey {
        case nodeId = "node_id", coordinator, audio, airfoil, librespot,
             cacheDir = "cache_dir", log
    }
}

public struct ConfigError: Error, CustomStringConvertible {
    public let problems: [String]
    public var description: String {
        "config invalid:\n  - " + problems.joined(separator: "\n  - ")
    }
}

public enum ConfigLoader {
    /// Support directory for the zero-config mode (R1): everything the node
    /// needs lives under ~/Library/Application Support/Pulsar.
    public static var supportDir: String {
        (NSSearchPathForDirectoriesInDomains(.applicationSupportDirectory, .userDomainMask, true).first
            ?? NSHomeDirectory() + "/Library/Application Support") + "/Pulsar"
    }

    /// defaults: the built-in config used when no yml exists (R1 zero-yml).
    /// Coordinator url/token stay empty — pairing credentials fill them.
    public static func defaults() -> NodeConfig {
        let dir = supportDir
        return NodeConfig(
            nodeId: "a",
            coordinator: .init(url: "", token: ""),
            audio: .init(fifoPath: dir + "/spotify.fifo", sampleRate: 44100,
                         format: "f32le", outputLatencyOffsetMs: 0,
                         ringBufferMs: 1000, outputDevice: nil),
            airfoil: .init(enabled: false, appPath: "/Applications/Airfoil.app",
                           speakers: [], pollS: 10),
            librespot: .init(binary: nil, apiPort: 3678, deviceName: nil,
                             configDir: dir + "/librespot"),
            cacheDir: dir + "/cache",
            log: .init(level: "info", path: dir + "/pulsar.log"))
    }

    public static func load(path: String, credentials: NodeCredentials? = nil) throws -> NodeConfig {
        if !FileManager.default.fileExists(atPath: path) {
            // Zero-yml mode (R1): built-in defaults + pairing credentials.
            var cfg = defaults()
            if let creds = credentials {
                cfg.coordinator.url = creds.wsUrl
                cfg.coordinator.token = creds.token
                cfg.nodeId = creds.slot
            }
            try validate(cfg)
            return cfg
        }
        let text: String
        do {
            text = try String(contentsOfFile: path, encoding: .utf8)
        } catch {
            throw ConfigError(problems: ["cannot read \(path): \(error.localizedDescription)"])
        }
        var cfg: NodeConfig
        do {
            cfg = try YAMLDecoder().decode(NodeConfig.self, from: text)
        } catch {
            throw ConfigError(problems: ["\(path) is not a valid node.yml: \(error)"])
        }
        // Pairing credentials override the yml (v2.1 M1): the file appears
        // after `NodeApp --pair CODE` and carries url+token+slot.
        if let creds = credentials {
            cfg.coordinator.url = creds.wsUrl
            cfg.coordinator.token = creds.token
            cfg.nodeId = creds.slot
        }
        try validate(cfg)
        return cfg
    }

    public static func validate(_ c: NodeConfig) throws {
        var problems: [String] = []

        // v2.1: slots are single letters a..z (orbit-assigned at pairing).
        if c.nodeId.count != 1 || !("a"..."z").contains(c.nodeId) {
            problems.append("node_id is \"\(c.nodeId)\", must be a single slot letter (a…z)")
        }
        // Unpaired state (R1): empty url+token together is legal — the app
        // starts into onboarding and asks for a pairing code instead.
        let unpaired = c.coordinator.url.isEmpty && c.coordinator.token.isEmpty
        if !unpaired {
            if let url = URL(string: c.coordinator.url), let scheme = url.scheme {
                if scheme != "ws" && scheme != "wss" {
                    problems.append("coordinator.url scheme is \(scheme)://, must be ws:// or wss://")
                }
            } else {
                problems.append("coordinator.url \"\(c.coordinator.url)\" is not a URL (expected ws://coord:8080/ws)")
            }
            let hex = CharacterSet(charactersIn: "0123456789abcdefABCDEF")
            if c.coordinator.token.count != 64 || c.coordinator.token.unicodeScalars.contains(where: { !hex.contains($0) }) {
                problems.append("coordinator.token must be 64 hex chars (32 random bytes), got \(c.coordinator.token.count) chars")
            }
        }
        if c.audio.fifoPath.isEmpty {
            problems.append("audio.fifo_path is required (e.g. /Users/duet/duet/spotify.fifo)")
        }
        if c.audio.sampleRate != 44100 {
            problems.append("audio.sample_rate is \(c.audio.sampleRate), the pipeline is fixed at 44100 (spec 6.3)")
        }
        if c.audio.format != "f32le" {
            problems.append("audio.format is \"\(c.audio.format)\", the pipeline is fixed at f32le (spec 6.3)")
        }
        if !(100...5000).contains(c.audio.ringBufferMs) {
            problems.append("audio.ring_buffer_ms is \(c.audio.ringBufferMs), expected 100..5000")
        }
        if c.airfoil.isEnabled && c.airfoil.speakers.isEmpty {
            problems.append("airfoil.enabled is true but airfoil.speakers is empty: list at least one speaker exactly as named in Airfoil")
        }
        if c.airfoil.pollS < 1 {
            problems.append("airfoil.poll_s must be >= 1 second")
        }
        if !(1...65535).contains(c.librespot.apiPort) {
            problems.append("librespot.api_port \(c.librespot.apiPort) is not a valid port")
        }
        if let name = c.librespot.deviceName, name.isEmpty {
            problems.append("librespot.device_name is empty — remove the key to get the default (Pulsar A/B) or set a name")
        }
        if c.cacheDir.isEmpty {
            problems.append("cache_dir is required (voice insert cache)")
        }
        if !["debug", "info", "warn", "error"].contains(c.log.level) {
            problems.append("log.level \"\(c.log.level)\" unknown, use debug|info|warn|error")
        }

        if !problems.isEmpty {
            throw ConfigError(problems: problems)
        }
    }
}
