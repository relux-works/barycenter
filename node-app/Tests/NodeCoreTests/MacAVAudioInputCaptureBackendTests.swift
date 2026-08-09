import AVFAudio
import CoreAudio
import Foundation
import Testing

@testable import NodeCore

@Suite(.serialized)
struct MacAVAudioInputCaptureBackendTests {
    private let builtInUID = "AppleHDAEngineInput:1B,0,1,0:1"

    @Test("Built-in 48 kHz input starts while a 44.1 kHz output aggregate has !dev")
    func builtInInputIgnoresOutputAggregateReceipt() throws {
        let discovery = FakeInputDeviceDiscovery(records: [
            record(id: 92, uid: builtInUID, isDefault: true)
        ])
        let session = try FakeInputOnlyAudioSession(sampleRate: 48_000)
        session.unrelatedOutputSampleRate = 44_100
        session.unrelatedOutputAggregateStatus = 560_227_702
        let backend = MacAVAudioCaptureBackend(
            deviceDiscovery: discovery,
            makeSession: { session })

        try backend.start(
            selectedDeviceID: nil,
            onSamples: { _ in },
            onFailure: {})

        #expect(session.startCount == 1)
        #expect(session.selectedDeviceIDs.isEmpty)
        #expect(session.unrelatedOutputSampleRate == 44_100)
        #expect(session.unrelatedOutputAggregateStatus == 560_227_702)
        backend.stop()
    }

    @Test("A stable microphone UID resolves after numeric AudioDeviceID churn")
    func stableUIDSurvivesDeviceChurn() throws {
        let discovery = FakeInputDeviceDiscovery(records: [
            record(id: 92, uid: builtInUID, isDefault: true)
        ])
        let session = try FakeInputOnlyAudioSession(sampleRate: 48_000)
        let backend = MacAVAudioCaptureBackend(
            deviceDiscovery: discovery,
            makeSession: { session })

        try backend.start(
            selectedDeviceID: builtInUID,
            onSamples: { _ in },
            onFailure: {})
        backend.stop()

        discovery.records = [
            record(id: 429, uid: builtInUID, isDefault: true)
        ]
        try backend.start(
            selectedDeviceID: builtInUID,
            onSamples: { _ in },
            onFailure: {})
        backend.stop()

        #expect(session.selectedDeviceIDs == [92, 429])
        #expect(backend.availableDevices().map(\.id) == [builtInUID])
    }

    @Test("A genuine input-only !dev start failure is typed and releases every resource")
    func genuineInputOnlyFailureCleansUp() throws {
        let discovery = FakeInputDeviceDiscovery(records: [
            record(id: 92, uid: builtInUID, isDefault: true)
        ])
        let session = try FakeInputOnlyAudioSession(sampleRate: 48_000)
        session.startError = NSError(
            domain: NSOSStatusErrorDomain,
            code: 560_227_702)
        let backend = MacAVAudioCaptureBackend(
            deviceDiscovery: discovery,
            makeSession: { session })

        do {
            try backend.start(
                selectedDeviceID: nil,
                onSamples: { _ in },
                onFailure: {})
            Issue.record("input-only startup unexpectedly succeeded")
        } catch MacCaptureEngineError.backendStartupFailed(let diagnostic) {
            #expect(diagnostic.stage == .inputOnly)
            #expect(diagnostic.attempt == 1)
            #expect(diagnostic.cause == .coreAudio(status: 560_227_702))
            #expect(!diagnostic.isBoundedRecoveryCandidate)
            #expect(!diagnostic.isEligibleForConsentedFallback)
        } catch {
            Issue.record("unexpected error: \(error)")
        }

        #expect(session.events == ["install-tap", "prepare", "start", "stop", "remove-tap", "reset"])
        #expect(session.stopCount == 1)
        #expect(session.removeTapCount == 1)
        #expect(session.resetCount == 1)
    }

    @Test("A selected microphone disappearing during attachment stays a device failure")
    func selectedDeviceDisappearsDuringAttachment() throws {
        let discovery = FakeInputDeviceDiscovery(records: [
            record(id: 137, uid: "USB:Vendor:Mic:serial-abc", isDefault: false)
        ])
        let session = try FakeInputOnlyAudioSession(sampleRate: 48_000)
        session.selectionError = NSError(
            domain: NSOSStatusErrorDomain,
            code: Int(kAudioHardwareBadDeviceError))
        let backend = MacAVAudioCaptureBackend(
            deviceDiscovery: discovery,
            makeSession: { session })

        #expect(throws: MacCaptureEngineError.selectedDeviceUnavailable) {
            try backend.start(
                selectedDeviceID: "USB:Vendor:Mic:serial-abc",
                onSamples: { _ in },
                onFailure: {})
        }

        #expect(session.events == ["select-input", "stop", "reset"])
        #expect(session.startCount == 0)
        #expect(session.stopCount == 1)
        #expect(session.resetCount == 1)
    }

    @Test("Only stable UIDs are exposed as microphone identities")
    func exposedIdentityIsStableUID() throws {
        let discovery = FakeInputDeviceDiscovery(records: [
            record(id: 92, uid: builtInUID, isDefault: true),
            record(id: 137, uid: "USB:Vendor:Mic:serial-abc", isDefault: false),
        ])
        let session = try FakeInputOnlyAudioSession(sampleRate: 48_000)
        let backend = MacAVAudioCaptureBackend(
            deviceDiscovery: discovery,
            makeSession: { session })

        let devices = backend.availableDevices()
        #expect(devices.map(\.id) == [builtInUID, "USB:Vendor:Mic:serial-abc"])
        #expect(!devices.contains { UInt32($0.id) != nil })
    }

    private func record(
        id: AudioDeviceID,
        uid: String,
        isDefault: Bool
    ) -> MacCaptureInputDeviceRecord {
        MacCaptureInputDeviceRecord(
            audioDeviceID: id,
            device: MacCaptureDevice(
                id: uid,
                name: isDefault ? "MacBook Pro Microphone" : "External Microphone",
                isDefault: isDefault))
    }
}

@Suite(.serialized)
struct MacCaptureInputSelectionStoreTests {
    private let builtIn = MacCaptureDevice(
        id: "AppleHDAEngineInput:1B,0,1,0:1",
        name: "MacBook Pro Microphone",
        isDefault: true)
    private let external = MacCaptureDevice(
        id: "USB:Vendor:Mic:serial-abc",
        name: "External Microphone",
        isDefault: false)

    @Test("Legacy numeric AudioDeviceID 92 is cleared and falls back to default")
    func numericLegacySelectionIsCleared() throws {
        let fixture = try DefaultsFixture()
        let store = MacCaptureInputSelectionStore(defaults: fixture.defaults)

        fixture.defaults.set(92, forKey: MacCaptureInputSelectionStore.legacyKey)
        #expect(store.load(availableDevices: [builtIn]) == nil)
        #expect(fixture.defaults.object(forKey: MacCaptureInputSelectionStore.legacyKey) == nil)

        fixture.defaults.set("92", forKey: MacCaptureInputSelectionStore.legacyKey)
        #expect(store.load(availableDevices: [builtIn]) == nil)
        #expect(fixture.defaults.object(forKey: MacCaptureInputSelectionStore.legacyKey) == nil)
        #expect(fixture.defaults.object(forKey: MacCaptureInputSelectionStore.stableKey) == nil)
    }

    @Test("Stable UID persists while the current numeric device ID changes")
    func stableSelectionPersistsAcrossChurn() throws {
        let fixture = try DefaultsFixture()
        let store = MacCaptureInputSelectionStore(defaults: fixture.defaults)

        store.save(external.id)
        #expect(store.load(availableDevices: [builtIn, external]) == external.id)
        #expect(
            fixture.defaults.string(forKey: MacCaptureInputSelectionStore.stableKey)
                == external.id)
    }

    @Test("A stale external UID is cleared and falls back to the current default")
    func staleStableSelectionFallsBack() throws {
        let fixture = try DefaultsFixture()
        let store = MacCaptureInputSelectionStore(defaults: fixture.defaults)
        store.save(external.id)

        #expect(store.load(availableDevices: [builtIn]) == nil)
        #expect(fixture.defaults.object(forKey: MacCaptureInputSelectionStore.stableKey) == nil)
    }

    @Test("A prerelease nonnumeric v1 UID migrates only when currently available")
    func nonnumericLegacyUIDMigrates() throws {
        let fixture = try DefaultsFixture()
        fixture.defaults.set(
            external.id,
            forKey: MacCaptureInputSelectionStore.legacyKey)
        let store = MacCaptureInputSelectionStore(defaults: fixture.defaults)

        #expect(store.load(availableDevices: [builtIn, external]) == external.id)
        #expect(fixture.defaults.object(forKey: MacCaptureInputSelectionStore.legacyKey) == nil)
        #expect(
            fixture.defaults.string(forKey: MacCaptureInputSelectionStore.stableKey)
                == external.id)
    }
}

private final class FakeInputDeviceDiscovery:
    MacCaptureInputDeviceDiscovering, @unchecked Sendable
{
    var records: [MacCaptureInputDeviceRecord]

    init(records: [MacCaptureInputDeviceRecord]) {
        self.records = records
    }

    func inputDevices() -> [MacCaptureInputDeviceRecord] {
        records
    }
}

private final class FakeInputOnlyAudioSession:
    MacInputOnlyAudioSession, @unchecked Sendable
{
    let configurationChangeObject: AnyObject = NSObject()
    private let format: AVAudioFormat
    var selectionError: Error?
    var startError: Error?
    var unrelatedOutputSampleRate: Double?
    var unrelatedOutputAggregateStatus: Int32?
    private(set) var selectedDeviceIDs: [AudioDeviceID] = []
    private(set) var events: [String] = []
    private(set) var startCount = 0
    private(set) var stopCount = 0
    private(set) var removeTapCount = 0
    private(set) var resetCount = 0

    init(sampleRate: Double) throws {
        format = try #require(
            AVAudioFormat(
                commonFormat: .pcmFormatFloat32,
                sampleRate: sampleRate,
                channels: 1,
                interleaved: false))
    }

    func selectInputDevice(_ id: AudioDeviceID) throws {
        events.append("select-input")
        if let selectionError {
            throw selectionError
        }
        selectedDeviceIDs.append(id)
    }

    func inputFormat() -> AVAudioFormat {
        format
    }

    func installTap(
        format _: AVAudioFormat,
        handler _: @escaping @Sendable (AVAudioPCMBuffer) -> Void
    ) {
        events.append("install-tap")
    }

    func removeTap() {
        events.append("remove-tap")
        removeTapCount += 1
    }

    func prepare() {
        events.append("prepare")
    }

    func start() throws {
        events.append("start")
        startCount += 1
        if let startError {
            throw startError
        }
    }

    func stop() {
        events.append("stop")
        stopCount += 1
    }

    func reset() {
        events.append("reset")
        resetCount += 1
    }
}

private final class DefaultsFixture {
    let suiteName: String
    let defaults: UserDefaults

    init() throws {
        suiteName = "MacCaptureInputSelectionStoreTests.\(UUID().uuidString)"
        defaults = try #require(UserDefaults(suiteName: suiteName))
        defaults.removePersistentDomain(forName: suiteName)
    }

    deinit {
        defaults.removePersistentDomain(forName: suiteName)
    }
}
