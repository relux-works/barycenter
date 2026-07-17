import AppKit
import Foundation
import Testing
@testable import NodeCore

@MainActor
@Suite("macOS recording shortcut controller")
struct MacRecordingShortcutControllerTests {
    @Test("Registered shortcut toggles through one callback and stale hooks are inert")
    func toggleAndStaleHookCleanup() throws {
        let registrar = FakeShortcutRegistrar()
        var toggles = 0
        let controller = MacRecordingShortcutController(registrar: registrar) { toggles += 1 }

        controller.start()
        #expect(controller.state == .registered(.defaultToggle))
        let first = try #require(registrar.lastTokenID)
        registrar.fire(first)
        #expect(toggles == 1)

        controller.stop()
        #expect(controller.state == .inactive)
        #expect(registrar.unregisteredIDs == [first])
        registrar.fire(first)
        #expect(toggles == 1, "an unregistered callback cannot toggle hidden recording")
    }

    @Test("Conflict remains explicit and reconfiguration never disables button ownership")
    func conflictAndReconfigure() throws {
        let registrar = FakeShortcutRegistrar()
        var toggles = 0
        let controller = MacRecordingShortcutController(registrar: registrar) { toggles += 1 }
        controller.start()
        let first = try #require(registrar.lastTokenID)

        registrar.nextError = .conflict
        let replacement = try #require(MacRecordingShortcut(
            key: .space, modifiers: [.command, .shift]))
        controller.reconfigure(replacement)
        #expect(controller.state == .conflict(replacement))
        #expect(registrar.unregisteredIDs == [first])
        registrar.fire(first)
        #expect(toggles == 0)

        controller.reconfigure(.defaultToggle)
        #expect(controller.state == .registered(.defaultToggle))
        let second = try #require(registrar.lastTokenID)
        registrar.fire(second)
        #expect(toggles == 1)
    }

    @Test("Unsupported registration is visible and leaves no hidden hook")
    func unavailableRegistration() throws {
        let registrar = FakeShortcutRegistrar()
        registrar.nextError = .unavailable(-50)
        let controller = MacRecordingShortcutController(registrar: registrar) {}
        controller.start()
        #expect(controller.state == .unavailable(.defaultToggle))
        #expect(registrar.liveTokenIDs.isEmpty)
        #expect(registrar.unregisteredIDs.isEmpty)
        controller.stop()
        #expect(controller.state == .inactive)
    }

    @Test("Repeated suspend, resume and teardown cycles own exactly one live hook")
    func repeatedLifecycleCycles() throws {
        let registrar = FakeShortcutRegistrar()
        let controller = MacRecordingShortcutController(registrar: registrar) {}
        controller.start()
        let first = try #require(registrar.lastTokenID)

        controller.suspend()
        controller.suspend()
        #expect(controller.state == .suspended(.defaultToggle))
        #expect(registrar.unregisteredIDs == [first])

        controller.resume()
        controller.resume()
        let second = try #require(registrar.lastTokenID)
        #expect(second != first)
        #expect(controller.state == .registered(.defaultToggle))

        controller.stop()
        controller.stop()
        #expect(registrar.unregisteredIDs == [first, second])
        #expect(registrar.liveTokenIDs.isEmpty)
    }

    @Test("Sleep and inactive session cancel once, unregister, and restore after wake")
    func notificationLifecycle() throws {
        let registrar = FakeShortcutRegistrar()
        let controller = MacRecordingShortcutController(registrar: registrar) {}
        let workspace = NotificationCenter()
        let application = NotificationCenter()
        var cancels = 0
        let lifecycle = MacRecordingShortcutLifecycle(
            controller: controller,
            cancelRecording: { cancels += 1 },
            workspaceCenter: workspace,
            applicationCenter: application)
        lifecycle.start()
        let first = try #require(registrar.lastTokenID)

        workspace.post(name: NSWorkspace.willSleepNotification, object: nil)
        workspace.post(name: NSWorkspace.sessionDidResignActiveNotification, object: nil)
        #expect(cancels == 1)
        #expect(controller.state == .suspended(.defaultToggle))
        #expect(registrar.unregisteredIDs == [first])

        workspace.post(name: NSWorkspace.didWakeNotification, object: nil)
        workspace.post(name: NSWorkspace.sessionDidBecomeActiveNotification, object: nil)
        let second = try #require(registrar.lastTokenID)
        #expect(controller.state == .registered(.defaultToggle))
        #expect(second != first)

        application.post(name: NSApplication.willTerminateNotification, object: nil)
        #expect(cancels == 2)
        #expect(controller.state == .inactive)
        #expect(registrar.unregisteredIDs == [first, second])
        lifecycle.stop()
        #expect(registrar.unregisteredIDs == [first, second])
    }

    @Test("Persisted shortcut is bounded and malformed values fall back safely")
    func persistenceValidation() throws {
        let suite = "mac-recording-shortcut-\(UUID().uuidString)"
        let defaults = try #require(UserDefaults(suiteName: suite))
        defer { defaults.removePersistentDomain(forName: suite) }
        let store = MacRecordingShortcutStore(defaults: defaults, storageKey: "shortcut")
        let replacement = try #require(MacRecordingShortcut(
            key: .r, modifiers: [.control, .option, .shift]))
        store.save(replacement)
        #expect(store.load() == replacement)

        defaults.set(Data(#"{"key":53,"modifiers":0}"#.utf8), forKey: "shortcut")
        #expect(store.load() == .defaultToggle)
    }

    @Test("Implementation never installs a global key monitor, event tap, or bare Escape")
    func noAccessibilityOrGlobalEscapeExpansion() throws {
        let source = try String(contentsOf:
            repositoryRoot.appendingPathComponent(
                "node-app/Sources/NodeCore/MacRecordingShortcutController.swift"),
            encoding: .utf8)
        #expect(source.contains("RegisterEventHotKey"))
        #expect(source.contains("kEventHotKeyExclusive"))
        #expect(!source.contains("addGlobalMonitorForEvents"))
        #expect(!source.contains("CGEvent.tapCreate"))
        #expect(!source.contains("AXIsProcessTrusted"))
        #expect(!source.contains("case escape"))
        #expect(MacRecordingShortcut.Key.allCases.prefix(2) == [.space, .r])
    }

    private var repositoryRoot: URL {
        URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
    }
}

@MainActor
private final class FakeShortcutRegistrar: MacGlobalShortcutRegistering {
    var nextError: MacGlobalShortcutRegistrationError?
    private var nextID = 0
    private var handlers: [Int: () -> Void] = [:]
    private(set) var unregisteredIDs: [Int] = []
    private(set) var liveTokenIDs: Set<Int> = []
    private(set) var lastTokenID: Int?

    func register(
        _ shortcut: MacRecordingShortcut,
        handler: @escaping () -> Void
    ) -> Result<MacGlobalShortcutRegistration, MacGlobalShortcutRegistrationError> {
        _ = shortcut
        if let error = nextError {
            nextError = nil
            return .failure(error)
        }
        nextID += 1
        let token = FakeShortcutRegistration(id: nextID)
        handlers[nextID] = handler
        liveTokenIDs.insert(nextID)
        lastTokenID = nextID
        return .success(token)
    }

    func unregister(_ registration: MacGlobalShortcutRegistration) {
        guard let token = registration as? FakeShortcutRegistration,
              liveTokenIDs.remove(token.id) != nil else { return }
        unregisteredIDs.append(token.id)
    }

    func fire(_ id: Int) { handlers[id]?() }
}

private final class FakeShortcutRegistration: MacGlobalShortcutRegistration {
    let id: Int
    init(id: Int) { self.id = id }
}
