// Structured JSON-line logging (spec 6.5): file + stderr mirror.
// Rotation (10 MB x 5) is handled here; os_log integration can come later —
// the file is the primary channel for headless operation.

import Foundation

public final class Logger {
    public enum Level: Int, Comparable, CustomStringConvertible {
        case debug = 0, info, warn, error
        public static func < (l: Level, r: Level) -> Bool { l.rawValue < r.rawValue }
        public var description: String {
            switch self {
            case .debug: return "debug"
            case .info: return "info"
            case .warn: return "warn"
            case .error: return "error"
            }
        }
        public init(name: String) {
            switch name {
            case "debug": self = .debug
            case "warn": self = .warn
            case "error": self = .error
            default: self = .info
            }
        }
    }

    private let level: Level
    private let path: String?
    private let queue = DispatchQueue(label: "duet.logger")
    private var handle: FileHandle?
    private let maxBytes: UInt64 = 10 * 1024 * 1024
    private let keptRotations = 5

    public init(level: Level, path: String?) {
        self.level = level
        self.path = path
        if let path {
            // Append across restarts; rotation (10 MB x 5) is the only truncation.
            if !FileManager.default.fileExists(atPath: path) {
                FileManager.default.createFile(atPath: path, contents: nil)
            }
            handle = FileHandle(forWritingAtPath: path)
            handle?.seekToEndOfFile()
        }
    }

    public func debug(_ msg: String, _ fields: [String: Any] = [:]) { log(.debug, msg, fields) }
    public func info(_ msg: String, _ fields: [String: Any] = [:]) { log(.info, msg, fields) }
    public func warn(_ msg: String, _ fields: [String: Any] = [:]) { log(.warn, msg, fields) }
    public func error(_ msg: String, _ fields: [String: Any] = [:]) { log(.error, msg, fields) }

    private func log(_ lvl: Level, _ msg: String, _ fields: [String: Any]) {
        guard lvl >= level else { return }
        var record: [String: Any] = [
            "ts": ISO8601DateFormatter().string(from: Date()),
            "level": lvl.description,
            "msg": msg,
        ]
        for (k, v) in fields { record[k] = v }
        guard let data = try? JSONSerialization.data(withJSONObject: record, options: [.sortedKeys]) else { return }
        queue.async { [weak self] in
            guard let self else { return }
            FileHandle.standardError.write(data)
            FileHandle.standardError.write(Data([0x0A]))
            if let h = self.handle {
                h.write(data)
                h.write(Data([0x0A]))
                self.rotateIfNeeded()
            }
        }
    }

    private func rotateIfNeeded() {
        guard let path, let h = handle, h.offsetInFile > maxBytes else { return }
        try? h.close()
        let fm = FileManager.default
        try? fm.removeItem(atPath: "\(path).\(keptRotations)")
        for i in stride(from: keptRotations - 1, through: 1, by: -1) {
            try? fm.moveItem(atPath: "\(path).\(i)", toPath: "\(path).\(i + 1)")
        }
        try? fm.moveItem(atPath: path, toPath: "\(path).1")
        fm.createFile(atPath: path, contents: nil)
        handle = FileHandle(forWritingAtPath: path)
    }
}
