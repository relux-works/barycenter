import AppKit
import Foundation
import NodeAppUI
import NodeCore

@MainActor
final class MacSoundboardAppComposition {
    private let client: PhaseOneAppClient
    private let model: PulsarShellModel
    private let preferences: MacSoundboardPreferenceStore
    private let shortcuts: MacSoundboardShortcutController
    private var prefs: MacSoundboardPreferences
    private var cues: [SoundboardCue] = []
    private var orderRevision: Int64 = 0
    private var task: Task<Void, Never>?
    private var observers: [NSObjectProtocol] = []
    private var busy = false
    private var stopped = false
    private var pending: (cueID: String, key: String, fallback: SoundboardFallbackConfirmation)?
    private var lastRefresh = Date.distantPast

    init(bundle: CredentialBundle, model: PulsarShellModel, recordingShortcut: @escaping () -> MacRecordingShortcut) throws {
        client = try PhaseOneAppClient(bundle: bundle)
        self.model = model
        preferences = MacSoundboardPreferenceStore()
        prefs = preferences.load()
        shortcuts = MacSoundboardShortcutController(
            registrar: CarbonGlobalShortcutRegistrar(), recordingShortcut: recordingShortcut,
            trigger: { _ in })
        shortcuts.setTrigger { [weak self] cueID in self?.trigger(cueID) }
        shortcuts.onChange = { [weak self] _ in self?.publish() }
    }

    func start() {
        guard !stopped else { return }
        shortcuts.configure(prefs.bindings)
        // Rebind after self exists; the controller callback remains bounded to cue IDs.
        installLifecycle()
        refresh(force: true)
    }

    func refresh(force: Bool = false) {
        guard !stopped, !busy else { return }
        let now = Date()
        guard force || now.timeIntervalSince(lastRefresh) >= 5 else { return }
        lastRefresh = now
        task?.cancel()
        task = Task { [weak self] in
            guard let self else { return }
            do {
                let result = try await client.soundboardCues()
                guard !Task.isCancelled, !stopped else { return }
                cues = result.cues
                orderRevision = result.orderRevision
                normalizeSelection()
                publish(outcome: nil, failure: nil)
            } catch { publish(outcome: nil, failure: failure(error)) }
        }
    }

    func select(_ cueID: String) {
        guard cues.contains(where: { $0.id == cueID }) else { return }
        prefs.selectedCueID = cueID
        savePreferences()
    }

    func setRoute(_ value: PulsarRouteTarget) {
        guard let route = PhaseOneRoute(rawValue: value.rawValue) else { return }
        prefs.route = route; savePreferences()
    }

    func setDelivery(_ value: PulsarDeliveryMode) {
        guard let delivery = PhaseOneDelivery(rawValue: value.rawValue) else { return }
        prefs.delivery = delivery; pending = nil; savePreferences()
    }

    func setIncludeOrigin(_ value: Bool) { prefs.includeOrigin = value; pending = nil; savePreferences() }

    func trigger(_ cueID: String) {
        guard cues.contains(where: { $0.id == cueID }), !busy else { return }
        let key = pending?.cueID == cueID ? pending!.key : operationKey("trigger")
        let fallback = pending?.cueID == cueID ? pending?.fallback : nil
        run { [self] in
            do {
                let receipt = try await client.triggerSoundboardCue(
                    cueID, intent: .init(route: prefs.route, delivery: prefs.delivery,
                                         includeOrigin: prefs.includeOrigin, fallback: fallback),
                    idempotencyKey: key)
                pending = nil
                return receipt.transmission.reused ? "already_accepted" : "accepted"
            } catch let challenge as PhaseOneConfirmationChallenge {
                guard challenge.alternatives.contains(.afterCurrent) else { throw challenge }
                pending = (cueID, key, .init(token: challenge.token, delivery: .afterCurrent))
                return "confirmation_required_press_trigger_again"
            }
        }
    }

    func create(from url: URL, rightsAcknowledged: Bool) {
        guard rightsAcknowledged, !busy else { return }
        run { [self] in
            let scoped = url.startAccessingSecurityScopedResource()
            defer { if scoped { url.stopAccessingSecurityScopedResource() } }
            let title = boundedTitle(url.deletingPathExtension().lastPathComponent)
            let suffix = operationKey("upload")
            let upload = try await client.upload(
                fileURL: url, title: title, idempotencyKey: suffix,
                rightsAcknowledged: true)
            do {
                _ = try await client.createSoundboardMediaCue(
                    title: title, mediaID: upload.mediaID,
                    idempotencyKey: operationKey("create"))
            } catch {
                try? await client.deleteMedia(upload.mediaID)
                throw error
            }
            return "cue_created"
        }
    }

    func rename(_ cueID: String, title: String) {
        guard let cue = cues.first(where: { $0.id == cueID }) else { return }
        run { [self] in
            _ = try await client.renameSoundboardCue(
                cueID, title: boundedTitle(title), revision: cue.revision,
                idempotencyKey: operationKey("rename"))
            return "cue_renamed"
        }
    }

    func delete(_ cueID: String) {
        guard let cue = cues.first(where: { $0.id == cueID }) else { return }
        run { [self] in
            _ = try await client.deleteSoundboardCue(
                cueID, revision: cue.revision, idempotencyKey: operationKey("delete"))
            prefs.bindings.removeAll { $0.cueID == cueID }
            if prefs.selectedCueID == cueID { prefs.selectedCueID = nil }
            try preferences.save(prefs)
            shortcuts.configure(prefs.bindings)
            return "cue_deleted"
        }
    }

    func move(_ cueID: String, delta: Int) {
        guard let source = cues.firstIndex(where: { $0.id == cueID }) else { return }
        let target = source + delta
        guard cues.indices.contains(target), orderRevision > 0 else { return }
        var ids = cues.map(\.id)
        ids.swapAt(source, target)
        run { [self] in
            _ = try await client.reorderSoundboardCues(
                ids, revision: orderRevision, idempotencyKey: operationKey("order"))
            return "cue_reordered"
        }
    }

    func cycleShortcut(_ cueID: String) {
        guard let index = cues.firstIndex(where: { $0.id == cueID }) else { return }
        let keyCases: [MacRecordingShortcut.Key] = [
            .f1, .f2, .f3, .f4, .f5, .f6, .f7, .f8,
            .f9, .f10, .f11, .f12, .f13, .f14, .f15, .f16]
        let primary = MacRecordingShortcut(key: keyCases[index % keyCases.count], modifiers: [.control, .option])!
        let secondary = MacRecordingShortcut(key: keyCases[index % keyCases.count], modifiers: [.control, .shift])!
        if let existing = prefs.bindings.firstIndex(where: { $0.cueID == cueID }) {
            if prefs.bindings[existing].shortcut == primary {
                prefs.bindings[existing] = .init(cueID: cueID, shortcut: secondary)
            } else {
                prefs.bindings.remove(at: existing)
            }
        } else if prefs.bindings.count < MacSoundboardPreferences.maximumBindings {
            prefs.bindings.append(.init(cueID: cueID, shortcut: primary))
        }
        savePreferences()
        shortcuts.configure(prefs.bindings)
    }

    func recordingShortcutChanged() { shortcuts.configure(prefs.bindings) }

    func triggerSelected() {
        guard let cueID = prefs.selectedCueID else { return }
        trigger(cueID)
    }

    func shutdown() {
        guard !stopped else { return }
        stopped = true; task?.cancel(); task = nil
        observers.forEach { NSWorkspace.shared.notificationCenter.removeObserver($0) }
        observers.removeAll(); shortcuts.stop()
        model.setSoundboard(.init())
    }

    private func run(_ operation: @escaping () async throws -> String) {
        guard !busy, !stopped else { return }
        busy = true; publish(outcome: nil, failure: nil)
        task = Task { [weak self] in
            guard let self else { return }
            do {
                let outcome = try await operation()
                busy = false
                if outcome.hasPrefix("confirmation_required") { publish(outcome: outcome, failure: nil) }
                else { await reload(outcome: outcome) }
            } catch {
                busy = false; publish(outcome: nil, failure: failure(error))
            }
        }
    }

    private func reload(outcome: String) async {
        do {
            let result = try await client.soundboardCues()
            cues = result.cues; orderRevision = result.orderRevision
            normalizeSelection(); publish(outcome: outcome, failure: nil)
        } catch { publish(outcome: outcome, failure: failure(error)) }
    }

    private func normalizeSelection() {
        if !cues.contains(where: { $0.id == prefs.selectedCueID }) {
            prefs.selectedCueID = cues.first?.id
            try? preferences.save(prefs)
        }
        let ids = Set(cues.map(\.id))
        let filtered = prefs.bindings.filter { ids.contains($0.cueID) }
        if filtered != prefs.bindings {
            prefs.bindings = filtered; try? preferences.save(prefs); shortcuts.configure(filtered)
        }
    }

    private func savePreferences() {
        do { try preferences.save(prefs); publish(outcome: "preferences_updated", failure: nil) }
        catch { publish(outcome: nil, failure: "preferences_unavailable") }
    }

    private func publish(outcome: String? = nil, failure: String? = nil) {
        let states = Dictionary(uniqueKeysWithValues: shortcuts.states.map { ($0.cueID, $0) })
        let bindings = Dictionary(uniqueKeysWithValues: prefs.bindings.map { ($0.cueID, $0.shortcut) })
        var state = PulsarSoundboardState()
        state.cues = cues.map { cue in
            PulsarSoundboardCue(
                id: cue.id, title: cue.title, sourceKind: cue.sourceKind,
                durationMS: cue.sourceDurationMS,
                shortcutLabel: bindings[cue.id].map(shortcutLabel),
                shortcutStatus: states[cue.id]?.status.rawValue ?? "inactive")
        }
        state.selectedCueID = prefs.selectedCueID
        state.route = PulsarRouteTarget(rawValue: prefs.route.rawValue) ?? .ownBarycenter
        state.delivery = PulsarDeliveryMode(rawValue: prefs.delivery.rawValue) ?? .overlay
        state.includeOrigin = prefs.includeOrigin; state.busy = busy
        state.outcome = outcome; state.failure = failure
        model.setSoundboard(state)
    }

    private func installLifecycle() {
        let center = NSWorkspace.shared.notificationCenter
        for name in [NSWorkspace.willSleepNotification, NSWorkspace.sessionDidResignActiveNotification] {
            observers.append(center.addObserver(forName: name, object: nil, queue: .main) { [weak self] _ in
                MainActor.assumeIsolated { self?.shortcuts.suspend() }
            })
        }
        for name in [NSWorkspace.didWakeNotification, NSWorkspace.sessionDidBecomeActiveNotification] {
            observers.append(center.addObserver(forName: name, object: nil, queue: .main) { [weak self] _ in
                MainActor.assumeIsolated { self?.shortcuts.resume() }
            })
        }
    }

    private func shortcutLabel(_ shortcut: MacRecordingShortcut) -> String {
        var value = ""
        if shortcut.modifiers.contains(.control) { value += "⌃" }
        if shortcut.modifiers.contains(.option) { value += "⌥" }
        if shortcut.modifiers.contains(.shift) { value += "⇧" }
        if shortcut.modifiers.contains(.command) { value += "⌘" }
        return value + shortcut.keyLabel
    }

    private func operationKey(_ kind: String) -> String {
        "mac-soundboard-\(kind)-\(UUID().uuidString.lowercased())"
    }

    private func boundedTitle(_ value: String) -> String {
        let clean = value.trimmingCharacters(in: .whitespacesAndNewlines)
        var bytes = 0
        let bounded = String(clean.prefix { character in
            let count = String(character).utf8.count
            guard bytes + count <= 128 else { return false }
            bytes += count; return true
        })
        return bounded.isEmpty ? "Soundboard cue" : bounded
    }

    private func failure(_ error: Error) -> String {
        if error is PhaseOneConfirmationChallenge { return "confirmation_unavailable" }
        guard let value = error as? PhaseOneClientError else { return "coordinator_unavailable" }
        switch value {
        case .transport: return "coordinator_unavailable"
        case .rejected(_, let code, _): return code
        case .invalidConfiguration: return "credential_unavailable"
        case .invalidRequest: return "invalid_request"
        case .invalidResponse: return "invalid_response"
        case .redirectRejected: return "redirect_rejected"
        case .responseTooLarge: return "response_too_large"
        }
    }
}

private extension MacRecordingShortcut {
    var keyLabel: String {
        switch key {
        case .space: "Space"
        case .r: "R"
        case .f1: "F1"; case .f2: "F2"; case .f3: "F3"; case .f4: "F4"
        case .f5: "F5"; case .f6: "F6"; case .f7: "F7"; case .f8: "F8"
        case .f9: "F9"; case .f10: "F10"; case .f11: "F11"; case .f12: "F12"
        case .f13: "F13"; case .f14: "F14"; case .f15: "F15"; case .f16: "F16"
        }
    }
}
