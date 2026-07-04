// go-librespot supervision (spec 6.2 item 1): render config.yml, run as a
// child process, restart with exponential backoff 1,2,4..30 s, surface the
// daemon version from its startup log line.

import Foundation

public enum LibrespotConfigRenderer {
    /// Renders the daemon config (spec A.2). Pure function for testability.
    public static func render(deviceName: String, apiPort: Int, fifoPath: String) -> String {
        """
        # Rendered by NodeApp — do not edit; changes are overwritten on start.
        device_name: "\(deviceName)"
        device_type: speaker
        credentials:
          type: zeroconf
          zeroconf:
            # Confirmed live (spike 2026-07-03): without this the zeroconf
            # session is memory-only and every daemon restart needs the phone.
            persist_credentials: true
        server:
          enabled: true
          address: 127.0.0.1
          port: \(apiPort)
        audio_backend: pipe
        audio_output_pipe: \(fifoPath)
        audio_output_pipe_format: f32le
        external_volume: true
        """
    }

    /// Default daemon config dir; credentials.json from the manual first login
    /// (spec ch. 13) lives here and must be preserved — only config.yml is ours.
    public static var defaultConfigDir: String {
        (NSHomeDirectory() as NSString).appendingPathComponent("Library/Application Support/go-librespot")
    }
}

public final class LibrespotSupervisor {
    private let binary: String
    private let configDir: String
    private let log: Logger
    private let queue = DispatchQueue(label: "duet.librespot-supervisor")

    private var process: Process?
    private var backoffSeconds: Double = 1
    private var stopped = true

    /// Parsed from "running go-librespot X.Y.Z"; "unknown" until seen.
    public private(set) var version = "unknown"
    /// Called on supervisor queue after each crash (spec 6.6: error(librespot_restart)).
    public var onCrash: (() -> Void)?

    public init(binary: String, configDir: String = LibrespotConfigRenderer.defaultConfigDir, log: Logger) {
        self.binary = binary
        self.configDir = configDir
        self.log = log
    }

    public func start(deviceName: String, apiPort: Int, fifoPath: String) throws {
        guard FileManager.default.isExecutableFile(atPath: binary) else {
            throw ConfigError(problems: [
                "librespot.binary \(binary) does not exist or is not executable — brew install go-librespot (spec ch. 13)",
            ])
        }
        try FileManager.default.createDirectory(atPath: configDir, withIntermediateDirectories: true)
        let cfg = LibrespotConfigRenderer.render(deviceName: deviceName, apiPort: apiPort, fifoPath: fifoPath)
        try cfg.write(toFile: (configDir as NSString).appendingPathComponent("config.yml"),
                      atomically: true, encoding: .utf8)

        // The FIFO must exist before the daemon starts (it opens lazily, but
        // an absent path is a hard error).
        if !FileManager.default.fileExists(atPath: fifoPath) {
            guard mkfifo(fifoPath, 0o600) == 0 else {
                throw ConfigError(problems: ["cannot create FIFO at \(fifoPath): errno \(errno)"])
            }
        }

        queue.sync {
            stopped = false
            backoffSeconds = 1
        }
        queue.async { self.spawn() }
    }

    public func stop() {
        queue.sync {
            stopped = true
            process?.terminate()
            process = nil
        }
    }

    /// Kills the daemon; the supervisor restarts it (soft restart, spec 6.6
    /// audio_starvation recovery).
    public func softRestart() {
        queue.async {
            self.log.info("librespot soft restart requested")
            self.process?.terminate()
        }
    }

    private func spawn() {
        guard !stopped else { return }
        let p = Process()
        p.executableURL = URL(fileURLWithPath: binary)
        p.arguments = ["--config_dir", configDir]

        let pipe = Pipe()
        p.standardOutput = pipe
        p.standardError = pipe
        pipe.fileHandleForReading.readabilityHandler = { [weak self] handle in
            let data = handle.availableData
            guard let self, !data.isEmpty, let text = String(data: data, encoding: .utf8) else { return }
            for line in text.split(separator: "\n") {
                self.log.debug("librespot", ["line": String(line)])
                if let r = line.range(of: "running go-librespot ") {
                    self.version = String(line[r.upperBound...]).trimmingCharacters(in: .whitespaces)
                }
            }
        }

        p.terminationHandler = { [weak self] proc in
            guard let self else { return }
            self.queue.async {
                pipe.fileHandleForReading.readabilityHandler = nil
                guard !self.stopped else { return }
                self.log.warn("librespot exited", ["code": proc.terminationStatus])
                self.onCrash?()
                let delay = self.backoffSeconds
                self.backoffSeconds = min(self.backoffSeconds * 2, 30)
                self.queue.asyncAfter(deadline: .now() + delay) { self.spawn() }
            }
        }

        do {
            try p.run()
            process = p
            log.info("librespot started", ["pid": p.processIdentifier])
        } catch {
            log.error("librespot spawn failed", ["err": "\(error)"])
            let delay = backoffSeconds
            backoffSeconds = min(backoffSeconds * 2, 30)
            queue.asyncAfter(deadline: .now() + delay) { self.spawn() }
        }
    }
}
