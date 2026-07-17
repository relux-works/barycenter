import Foundation
import Testing
@testable import NodeCore

@MainActor
@Suite("macOS soundboard preferences and hotkeys")
struct MacSoundboardTests {
    @Test("Preferences are bounded, secret-free, and malformed input fails closed")
    func safePreferences() throws {
        let suite = "mac-soundboard-\(UUID().uuidString)"
        let defaults = try #require(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }
        let store = MacSoundboardPreferenceStore(defaults: defaults, key: "soundboard")
        let cue = "cq_" + String(repeating: "A", count: 26)
        var prefs = MacSoundboardPreferences()
        prefs.selectedCueID = cue
        prefs.bindings = [.init(cueID: cue, shortcut: MacRecordingShortcut(
            key: .f1, modifiers: [.control, .option])!)]
        try store.save(prefs)
        #expect(store.load() == prefs)
        let raw = try #require(defaults.data(forKey: "soundboard"))
        let text = String(decoding: raw, as: UTF8.self).lowercased()
        for forbidden in ["token", "bearer", "media_id", "local_path", "microphone"] {
            #expect(!text.contains(forbidden))
        }
        defaults.set(Data(#"{"version":1,"bindings":[{"cueID":"bad"}]}"#.utf8), forKey: "soundboard")
        #expect(store.load() == MacSoundboardPreferences())
    }

    @Test("Recording conflicts stay visible while other cue shortcuts trigger and suspend cleanly")
    func conflictsAndLifecycle() throws {
        let registrar = FakeSoundboardRegistrar()
        let recording = MacRecordingShortcut.defaultToggle
        let cueA = "cq_" + String(repeating: "A", count: 26)
        let cueB = "cq_" + String(repeating: "B", count: 26)
        let hotkey = try #require(MacRecordingShortcut(key: .f2, modifiers: [.control, .option]))
        var fired: [String] = []
        let controller = MacSoundboardShortcutController(
            registrar: registrar, recordingShortcut: { recording }, trigger: { fired.append($0) })
        controller.configure([
            .init(cueID: cueA, shortcut: recording), .init(cueID: cueB, shortcut: hotkey)])
        #expect(controller.states.map(\.status) == [.conflict, .registered])
        registrar.fireLast()
        #expect(fired == [cueB])
        controller.suspend()
        registrar.fireLast()
        #expect(fired == [cueB])
        #expect(controller.states.allSatisfy { $0.status == .suspended })
        controller.resume(); controller.stop()
        #expect(registrar.live.isEmpty)
    }

    @Test("Soundboard sources use Carbon registration and never capture or monitor keys broadly")
    func sourceBoundary() throws {
        let root = URL(fileURLWithPath: #filePath).deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        let soundboardPaths = [
            "node-app/Sources/NodeCore/MacSoundboard.swift",
            "node-app/Sources/NodeApp/MacSoundboardAppComposition.swift"]
        let source = try soundboardPaths.map {
            try String(contentsOf: root.appendingPathComponent($0), encoding: .utf8)
        }.joined()
        let menu = try String(contentsOf: root.appendingPathComponent(
            "node-app/Sources/NodeApp/StatusMenu.swift"), encoding: .utf8)
        #expect(source.contains("CarbonGlobalShortcutRegistrar"))
        #expect(menu.contains("triggerSelectedSoundboardCue"))
        for forbidden in ["CGEvent.tapCreate", "addGlobalMonitorForEvents", "AXIsProcessTrusted",
                          "toggleRecording", "recordFiveSeconds", "MacMicrophoneCaptureEngine"] {
            #expect(!source.contains(forbidden))
        }
    }
}

@MainActor
private final class FakeSoundboardRegistrar: MacGlobalShortcutRegistering {
    private final class Token: MacGlobalShortcutRegistration { let id: Int; init(_ id: Int) { self.id = id } }
    private var next = 0
    private var callbacks: [Int: () -> Void] = [:]
    private(set) var live = Set<Int>()
    private var last: Int?
    func register(_ shortcut: MacRecordingShortcut, handler: @escaping () -> Void)
      -> Result<MacGlobalShortcutRegistration, MacGlobalShortcutRegistrationError> {
        _ = shortcut; next += 1; let token = Token(next)
        callbacks[next] = handler; live.insert(next); last = next
        return .success(token)
    }
    func unregister(_ registration: MacGlobalShortcutRegistration) {
        guard let token = registration as? Token else { return }; live.remove(token.id)
    }
    func fireLast() { if let last { callbacks[last]?() } }
}
