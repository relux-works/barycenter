import AppKit
import Carbon
import Foundation

public struct MacShortcutModifiers: OptionSet, Codable, Equatable, Sendable {
    public let rawValue: UInt32

    public init(rawValue: UInt32) { self.rawValue = rawValue }

    public static let control = MacShortcutModifiers(rawValue: 1 << 0)
    public static let option = MacShortcutModifiers(rawValue: 1 << 1)
    public static let shift = MacShortcutModifiers(rawValue: 1 << 2)
    public static let command = MacShortcutModifiers(rawValue: 1 << 3)
}

public struct MacRecordingShortcut: Codable, Equatable, Sendable {
    public enum Key: UInt32, Codable, CaseIterable, Sendable {
        case space = 49
        case r = 15
        case f1 = 122, f2 = 120, f3 = 99, f4 = 118
        case f5 = 96, f6 = 97, f7 = 98, f8 = 100
        case f9 = 101, f10 = 109, f11 = 103, f12 = 111
        case f13 = 105, f14 = 107, f15 = 113, f16 = 106
    }

    public let key: Key
    public let modifiers: MacShortcutModifiers

    public init?(key: Key, modifiers: MacShortcutModifiers) {
        let supportedMask: MacShortcutModifiers = [.control, .option, .shift, .command]
        guard !modifiers.isEmpty,
              modifiers.intersection(supportedMask) == modifiers else { return nil }
        self.key = key
        self.modifiers = modifiers
    }

    public static let defaultToggle = MacRecordingShortcut(
        key: .space,
        modifiers: [.control, .shift])!
}

public enum MacGlobalShortcutRegistrationError: Error, Equatable, Sendable {
    case conflict
    case unavailable(Int32)
}

public protocol MacGlobalShortcutRegistration: AnyObject {}

@MainActor
public protocol MacGlobalShortcutRegistering: AnyObject {
    func register(
        _ shortcut: MacRecordingShortcut,
        handler: @escaping () -> Void
    ) -> Result<MacGlobalShortcutRegistration, MacGlobalShortcutRegistrationError>

    func unregister(_ registration: MacGlobalShortcutRegistration)
}

public enum MacRecordingShortcutState: Equatable, Sendable {
    case inactive
    case registered(MacRecordingShortcut)
    case conflict(MacRecordingShortcut)
    case unavailable(MacRecordingShortcut)
    case suspended(MacRecordingShortcut)
}

@MainActor
public final class MacRecordingShortcutController {
    private let registrar: MacGlobalShortcutRegistering
    private let onToggleRecording: () -> Void
    private var registration: MacGlobalShortcutRegistration?
    private var generation: UInt64 = 0
    private var suspended = false

    public private(set) var shortcut: MacRecordingShortcut
    public private(set) var state: MacRecordingShortcutState = .inactive
    public var onStateChange: ((MacRecordingShortcutState) -> Void)?

    public init(
        registrar: MacGlobalShortcutRegistering,
        shortcut: MacRecordingShortcut = .defaultToggle,
        onToggleRecording: @escaping () -> Void
    ) {
        self.registrar = registrar
        self.shortcut = shortcut
        self.onToggleRecording = onToggleRecording
    }

    public func start() { registerConfiguredShortcut() }

    public func reconfigure(_ shortcut: MacRecordingShortcut) {
        unregisterCurrent()
        self.shortcut = shortcut
        if suspended {
            setState(.suspended(shortcut))
        } else {
            registerConfiguredShortcut()
        }
    }

    public func suspend() {
        guard !suspended else { return }
        suspended = true
        unregisterCurrent()
        setState(.suspended(shortcut))
    }

    public func resume() {
        guard suspended else { return }
        suspended = false
        registerConfiguredShortcut()
    }

    public func stop() {
        suspended = false
        unregisterCurrent()
        setState(.inactive)
    }

    private func registerConfiguredShortcut() {
        unregisterCurrent()
        generation &+= 1
        let expectedGeneration = generation
        let result = registrar.register(shortcut) { [weak self] in
            guard let self,
                  self.generation == expectedGeneration,
                  self.registration != nil,
                  self.state == .registered(self.shortcut) else { return }
            self.onToggleRecording()
        }
        switch result {
        case .success(let registration):
            self.registration = registration
            setState(.registered(shortcut))
        case .failure(.conflict):
            setState(.conflict(shortcut))
        case .failure(.unavailable):
            setState(.unavailable(shortcut))
        }
    }

    private func unregisterCurrent() {
        generation &+= 1
        if let registration { registrar.unregister(registration) }
        registration = nil
    }

    private func setState(_ state: MacRecordingShortcutState) {
        self.state = state
        onStateChange?(state)
    }
}

@MainActor
public final class CarbonGlobalShortcutRegistrar: MacGlobalShortcutRegistering {
    private static let signature: OSType = 0x504C5352 // PLSR
    private static var nextID: UInt32 = 0

    public init() {}

    public func register(
        _ shortcut: MacRecordingShortcut,
        handler: @escaping () -> Void
    ) -> Result<MacGlobalShortcutRegistration, MacGlobalShortcutRegistrationError> {
        Self.nextID &+= 1
        if Self.nextID == 0 { Self.nextID = 1 }
        let identifier = EventHotKeyID(signature: Self.signature, id: Self.nextID)
        let box = CarbonHotKeyBox(identifier: identifier, handler: handler)
        var eventSpec = EventTypeSpec(
            eventClass: OSType(kEventClassKeyboard),
            eventKind: UInt32(kEventHotKeyPressed))
        var eventHandler: EventHandlerRef?
        let installStatus = InstallEventHandler(
            GetApplicationEventTarget(),
            carbonRecordingHotKeyHandler,
            1,
            &eventSpec,
            Unmanaged.passUnretained(box).toOpaque(),
            &eventHandler)
        guard installStatus == noErr, let eventHandler else {
            return .failure(.unavailable(installStatus))
        }

        var hotKey: EventHotKeyRef?
        let registrationStatus = RegisterEventHotKey(
            shortcut.key.rawValue,
            Self.carbonModifiers(shortcut.modifiers),
            identifier,
            GetApplicationEventTarget(),
            OptionBits(kEventHotKeyExclusive),
            &hotKey)
        guard registrationStatus == noErr, let hotKey else {
            RemoveEventHandler(eventHandler)
            if registrationStatus == eventHotKeyExistsErr {
                return .failure(.conflict)
            }
            return .failure(.unavailable(registrationStatus))
        }
        return .success(CarbonHotKeyRegistration(
            hotKey: hotKey,
            eventHandler: eventHandler,
            box: box))
    }

    public func unregister(_ registration: MacGlobalShortcutRegistration) {
        guard let registration = registration as? CarbonHotKeyRegistration else { return }
        registration.invalidate()
    }

    private static func carbonModifiers(_ modifiers: MacShortcutModifiers) -> UInt32 {
        var result: UInt32 = 0
        if modifiers.contains(.control) { result |= UInt32(controlKey) }
        if modifiers.contains(.option) { result |= UInt32(optionKey) }
        if modifiers.contains(.shift) { result |= UInt32(shiftKey) }
        if modifiers.contains(.command) { result |= UInt32(cmdKey) }
        return result
    }
}

public final class MacRecordingShortcutStore {
    private struct StoredShortcut: Codable {
        let key: UInt32
        let modifiers: UInt32
    }

    private let defaults: UserDefaults
    private let storageKey: String

    public init(
        defaults: UserDefaults = .standard,
        storageKey: String = "recordingShortcut.v1"
    ) {
        self.defaults = defaults
        self.storageKey = storageKey
    }

    public func load() -> MacRecordingShortcut {
        guard let data = defaults.data(forKey: storageKey),
              let stored = try? JSONDecoder().decode(StoredShortcut.self, from: data),
              let key = MacRecordingShortcut.Key(rawValue: stored.key),
              let shortcut = MacRecordingShortcut(
                key: key,
                modifiers: MacShortcutModifiers(rawValue: stored.modifiers)) else {
            return .defaultToggle
        }
        return shortcut
    }

    public func save(_ shortcut: MacRecordingShortcut) {
        let stored = StoredShortcut(
            key: shortcut.key.rawValue,
            modifiers: shortcut.modifiers.rawValue)
        if let data = try? JSONEncoder().encode(stored) {
            defaults.set(data, forKey: storageKey)
        }
    }
}

@MainActor
public final class MacRecordingShortcutLifecycle {
    private let controller: MacRecordingShortcutController
    private let cancelRecording: () -> Void
    private let workspaceCenter: NotificationCenter
    private let applicationCenter: NotificationCenter
    private var observers: [NSObjectProtocol] = []

    public init(
        controller: MacRecordingShortcutController,
        cancelRecording: @escaping () -> Void,
        workspaceCenter: NotificationCenter = NSWorkspace.shared.notificationCenter,
        applicationCenter: NotificationCenter = .default
    ) {
        self.controller = controller
        self.cancelRecording = cancelRecording
        self.workspaceCenter = workspaceCenter
        self.applicationCenter = applicationCenter
    }

    public func start() {
        guard observers.isEmpty else { return }
        controller.start()
        observe(workspaceCenter, NSWorkspace.willSleepNotification) { [weak self] in
            self?.cancelAndSuspend()
        }
        observe(workspaceCenter, NSWorkspace.sessionDidResignActiveNotification) { [weak self] in
            self?.cancelAndSuspend()
        }
        observe(workspaceCenter, NSWorkspace.didWakeNotification) { [weak self] in
            self?.controller.resume()
        }
        observe(workspaceCenter, NSWorkspace.sessionDidBecomeActiveNotification) { [weak self] in
            self?.controller.resume()
        }
        observe(applicationCenter, NSApplication.willTerminateNotification) { [weak self] in
            self?.cancelRecording()
            self?.controller.stop()
        }
    }

    public func stop() {
        observers.forEach { observer in
            workspaceCenter.removeObserver(observer)
            applicationCenter.removeObserver(observer)
        }
        observers.removeAll()
        controller.stop()
    }

    private func cancelAndSuspend() {
        if case .suspended = controller.state { return }
        cancelRecording()
        controller.suspend()
    }

    private func observe(
        _ center: NotificationCenter,
        _ name: Notification.Name,
        action: @escaping () -> Void
    ) {
        observers.append(center.addObserver(
            forName: name,
            object: nil,
            queue: .main
        ) { _ in action() })
    }
}

private final class CarbonHotKeyBox {
    let identifier: EventHotKeyID
    let handler: () -> Void

    init(identifier: EventHotKeyID, handler: @escaping () -> Void) {
        self.identifier = identifier
        self.handler = handler
    }
}

private final class CarbonHotKeyRegistration: MacGlobalShortcutRegistration {
    let hotKey: EventHotKeyRef
    let eventHandler: EventHandlerRef
    let box: CarbonHotKeyBox
    private var active = true

    init(hotKey: EventHotKeyRef, eventHandler: EventHandlerRef, box: CarbonHotKeyBox) {
        self.hotKey = hotKey
        self.eventHandler = eventHandler
        self.box = box
    }

    func invalidate() {
        guard active else { return }
        active = false
        UnregisterEventHotKey(hotKey)
        RemoveEventHandler(eventHandler)
    }

    deinit { invalidate() }
}

private let carbonRecordingHotKeyHandler: EventHandlerUPP = { _, event, context in
    guard let event, let context else { return OSStatus(eventNotHandledErr) }
    let box = Unmanaged<CarbonHotKeyBox>.fromOpaque(context).takeUnretainedValue()
    var identifier = EventHotKeyID()
    let status = GetEventParameter(
        event,
        EventParamName(kEventParamDirectObject),
        EventParamType(typeEventHotKeyID),
        nil,
        MemoryLayout<EventHotKeyID>.size,
        nil,
        &identifier)
    guard status == noErr,
          identifier.signature == box.identifier.signature,
          identifier.id == box.identifier.id else { return OSStatus(eventNotHandledErr) }
    box.handler()
    return noErr
}
