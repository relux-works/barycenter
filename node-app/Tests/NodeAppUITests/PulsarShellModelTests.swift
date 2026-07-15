import Foundation
import Testing
@testable import NodeAppUI

@Suite("Pulsar macOS shell")
struct PulsarShellModelTests {
    @Test("English and Russian catalogs cover every shell key")
    func catalogsAreComplete() {
        for locale in PulsarShellLocale.allCases {
            let copy = PulsarShellCopy(locale: locale)
            for key in PulsarShellText.allCases {
                #expect(!copy.text(key).trimmingCharacters(in: .whitespacesAndNewlines).isEmpty)
            }
            for section in PulsarShellSection.allCases {
                #expect(!copy.title(for: section).isEmpty)
            }
        }
        #expect(PulsarShellCopy(locale: .en).text(.connectionOnline) !=
                PulsarShellCopy(locale: .ru).text(.connectionOnline))
    }

    @Test("Every connection and recording state has non-color semantics")
    func stateSemanticsAreTextAndSymbolBased() {
        let copy = PulsarShellCopy(locale: .en)
        let connections: [PulsarConnectionState] = [
            .unpaired, .reconnecting, .online, .degraded("output unavailable"),
        ]
        for state in connections {
            #expect(!copy.connectionLabel(state).isEmpty)
            #expect(!copy.connectionSymbol(state).isEmpty)
        }

        let recordings: [PulsarRecordingState] = [
            .unavailable, .idle, .recording, .processing, .failed("permission denied"),
        ]
        for state in recordings {
            #expect(!copy.recordingLabel(state).isEmpty)
            #expect(!copy.recordingSymbol(state).isEmpty)
        }
    }

    @MainActor
    @Test("Unpaired and degraded snapshots preserve shell navigation")
    func navigationSurvivesConnectionChanges() {
        let model = PulsarShellModel(locale: .en)
        #expect(model.snapshot.connection == .unpaired)
        #expect(PulsarShellSection.allCases == [.home, .create, .join, .tryLocally, .history, .settings])

        model.selectedSection = .tryLocally
        model.updateConnection(.degraded("coordinator unavailable"), identity: "example · home")
        #expect(model.selectedSection == .tryLocally)
        #expect(model.snapshot.connection.isPaired)

        model.setRecording(.recording, available: true)
        model.selectedSection = .settings
        #expect(model.snapshot.recording == .recording)
        #expect(model.selectedSection == .settings)
    }

    @MainActor
    @Test("Runtime updates clamp volume and preserve unrelated shell state")
    func runtimeProjectionIsBounded() {
        let history = PulsarHistoryItem(
            id: "hi_public",
            title: "Voice message",
            detail: "Delivered",
            occurredAt: Date(timeIntervalSince1970: 1_700_000_000)
        )
        let model = PulsarShellModel(snapshot: .init(history: [history], volume: 80))

        model.updateRuntime(
            routeName: "Studio Display",
            nowPlaying: "Track — Artist",
            playbackState: "playing",
            dndMode: .messagesOnly,
            volume: 140
        )

        #expect(model.snapshot.routeName == "Studio Display")
        #expect(model.snapshot.nowPlaying == "Track — Artist")
        #expect(model.snapshot.playbackState == "playing")
        #expect(model.snapshot.dndMode == .messagesOnly)
        #expect(model.snapshot.volume == 100)
        #expect(model.snapshot.history == [history])
    }

    @MainActor
    @Test("Self-test projection clamps meter and preserves the complete file review")
    func selfTestProjection() {
        let model = PulsarShellModel(snapshot: .init(selfTestAvailable: true))
        let review = PulsarLocalFileReview(
            filename: "voice.wav",
            format: "wav",
            durationMs: 5_000,
            sizeBytes: 12_345,
            audience: ["this_pulsar"],
            deliveryModes: ["overlay", "interrupt"],
            rightsReminder: "Only audio you may share.",
            serverValidationRequired: true,
            rejection: nil)

        model.updateSelfTest(state: .recording, meter: 1.5, draftAvailable: false)
        model.setLocalFileReview(review)
        #expect(model.snapshot.selfTestState == .recording)
        #expect(model.snapshot.selfTestMeter == 1)
        #expect(model.snapshot.localFileReview == review)
        #expect(model.snapshot.localFileReview?.isEligible == true)
        #expect(!model.snapshot.localDraftAvailable)
        for locale in PulsarShellLocale.allCases {
            #expect(!PulsarShellCopy(locale: locale).selfTestLabel(.recording).isEmpty)
        }
    }

    @Test("DND labels map the frozen wire values in both languages")
    func dndWireValuesStayStable() {
        #expect(PulsarDNDMode.allowAll.rawValue == "allow_all")
        #expect(PulsarDNDMode.messagesOnly.rawValue == "messages_only")
        #expect(PulsarDNDMode.mutedUntil.rawValue == "muted_until")
        for locale in PulsarShellLocale.allCases {
            let copy = PulsarShellCopy(locale: locale)
            for mode in PulsarDNDMode.allCases {
                #expect(!copy.dndLabel(mode).isEmpty)
            }
        }
    }

    @MainActor
    @Test("Stable action object forwards every shell capability seam")
    func actionsForwardWithoutOwningFeatureState() {
        var calls: [String] = []
        let actions = PulsarShellActions(
            createOrbit: { calls.append("create") },
            joinOrbit: { calls.append("join") },
            tryLocally: { calls.append("self-test") },
            setDND: { calls.append($0.rawValue) },
            setVolume: { calls.append("volume:\($0)") },
            toggleRecording: { calls.append("record") },
            playBuiltinCue: { calls.append("cue") },
            recordFiveSeconds: { calls.append("five") },
            reviewLocalFile: { calls.append("review:\($0.lastPathComponent)") },
            acceptLocalFile: { calls.append("accept:\($0.lastPathComponent)") },
            deleteLocalDraft: { calls.append("delete") },
            closeSelfTest: { calls.append("close") }
        )

        actions.createOrbit()
        actions.joinOrbit()
        actions.tryLocally()
        actions.setDND(.messagesOnly)
        actions.setVolume(45)
        actions.toggleRecording()
        let file = URL(fileURLWithPath: "/tmp/voice.wav")
        actions.playBuiltinCue()
        actions.recordFiveSeconds()
        actions.reviewLocalFile(file)
        actions.acceptLocalFile(file)
        actions.deleteLocalDraft()
        actions.closeSelfTest()

        #expect(calls == [
            "create", "join", "self-test", "messages_only", "volume:45", "record",
            "cue", "five", "review:voice.wav", "accept:voice.wav", "delete", "close",
        ])
    }
}
