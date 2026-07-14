// Durable node-local DND intent and the last privacy-bounded presence
// projection. The persisted schema deliberately reuses the frozen typed
// payloads, which have no microphone, level, device, token, path or URL fields.

import Foundation

enum NodePresenceStoreError: Error {
    case invalidDND
    case persistenceFailed
}

final class NodePresenceStore {
    private struct Persisted: Codable {
        var localDND: SetDNDPayload?
        var presence: PresenceUpdatePayload?
    }

    private let fileURL: URL
    private let log: Logger
    private let queue = DispatchQueue(label: "duet.node-presence-store")
    private var persisted: Persisted

    init(fileURL: URL, log: Logger) {
        self.fileURL = fileURL
        self.log = log
        if let data = try? Data(contentsOf: fileURL),
           let value = try? JSONDecoder().decode(Persisted.self, from: data) {
            persisted = value
        } else {
            persisted = Persisted(localDND: nil, presence: nil)
        }
    }

    var currentLocalDND: SetDNDPayload? {
        queue.sync { persisted.localDND }
    }

    var latestPresence: PresenceUpdatePayload? {
        queue.sync { persisted.presence }
    }

    /// Persists before returning, so a caller never announces a revision that
    /// would disappear on a crash. Coordinator time bounds muted_until.
    func nextLocalDND(
        mode: String,
        mutedUntilCoordMs: Int64?,
        coordinatorNowMs: Int64
    ) throws -> SetDNDPayload {
        try queue.sync {
            switch mode {
            case "allow_all", "messages_only":
                guard mutedUntilCoordMs == nil else { throw NodePresenceStoreError.invalidDND }
            case "muted_until":
                guard let until = mutedUntilCoordMs,
                      until > coordinatorNowMs,
                      until <= coordinatorNowMs + 30 * 24 * 60 * 60 * 1000 else {
                    throw NodePresenceStoreError.invalidDND
                }
            default:
                throw NodePresenceStoreError.invalidDND
            }
            let revision = max(0, persisted.localDND?.revision ?? 0) + 1
            let payload = SetDNDPayload(
                revision: revision,
                mode: mode,
                mutedUntilCoordMs: mutedUntilCoordMs)
            let previous = persisted
            persisted.localDND = payload
            do {
                try writeLocked()
            } catch {
                persisted = previous
                throw NodePresenceStoreError.persistenceFailed
            }
            return payload
        }
    }

    /// Global projection revisions are monotonic. Equal revisions are an
    /// idempotent resend only when their typed body is identical.
    @discardableResult
    func acceptPresence(_ update: PresenceUpdatePayload) -> Bool {
        queue.sync {
            guard update.revision > 0 else { return false }
            if let current = persisted.presence {
                if update.revision < current.revision { return false }
                if update.revision == current.revision { return update == current }
            }
            let previous = persisted.presence
            persisted.presence = update
            do {
                try writeLocked()
                return true
            } catch {
                persisted.presence = previous
                log.warn("presence state persistence failed")
                return false
            }
        }
    }

    private func writeLocked() throws {
        let parent = fileURL.deletingLastPathComponent()
        try FileManager.default.createDirectory(at: parent, withIntermediateDirectories: true)
        let data = try JSONEncoder().encode(persisted)
        try data.write(to: fileURL, options: .atomic)
        try? FileManager.default.setAttributes(
            [.posixPermissions: NSNumber(value: Int16(0o600))],
            ofItemAtPath: fileURL.path)
    }
}
