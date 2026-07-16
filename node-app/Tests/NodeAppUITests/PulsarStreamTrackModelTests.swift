import Foundation
import Testing
@testable import NodeAppUI

struct PulsarStreamTrackModelTests {
    private let now = Date(timeIntervalSince1970: 1_800_000_000)
    private let localID = String(repeating: "a", count: 32)
    private let mediaID = "med_opaque_server_value"
    private let streamID = "str_opaque_server_value"
    private let reference = "trf_" + String(repeating: "A", count: 43)

    private var label: PulsarLocalizedLabel {
        .init(key: "stream_track.action.upload", en: "Upload track", ru: "Загрузить трек")
    }

    private var allActions: [PulsarActionCapability] {
        ["accept_policy", "upload", "retry", "delete", "queue", "replace", "pause", "seek", "resume", "report"]
            .map {
                .init(
                    action: $0,
                    label: .init(key: "stream_track.action.\($0)", en: $0, ru: $0))
            }
    }

    private func draft(phase: PulsarStreamTrackDraftPhase = .ready) -> PulsarStreamTrackDraft {
        .init(
            localID: localID, localByteCount: 500_000_000, title: "Long track",
            clientMIME: "audio/mpeg", durationMS: 7_000_000, phase: phase,
            phaseLabel: .init(key: "stream_track.draft.\(phase.rawValue)", en: phase.rawValue, ru: phase.rawValue),
            mediaID: phase == .retained ? nil : mediaID,
            variantManifest: phase == .ready ? "opaque manifest" : nil,
            serverMetadataConfirmed: phase == .ready, uploadOffset: 250_000_000,
            processingPercent: phase == .processing ? 42 : 100)
    }

    private func fixture(
        state: PulsarTargetsInboxSurfaceState = .ready,
        draft: PulsarStreamTrackDraft? = nil,
        playbackGeneration: UInt64 = 7,
        seekGeneration: UInt64 = 3,
        position: Int64 = 1_000
    ) -> PulsarStreamTrackSnapshot {
        .init(
            state: state, draft: draft ?? self.draft(),
            playback: .init(
                phase: .playing, streamID: streamID, durationMS: 10_000,
                audiblePositionMS: position, playbackGeneration: playbackGeneration,
                seekGeneration: seekGeneration),
            targets: [
                .init(
                    reference: reference, kind: "pulsar", expiresAt: now.addingTimeInterval(3600),
                    capabilityState: "known", capabilities: ["stream_track", "stream_track"], label: label)
            ],
            selectedReferences: [reference], selectedAudience: .explicit,
            selectedInsertion: .queue, activeAirAvailable: true,
            contentPolicyState: "current", actions: allActions)
    }

    @MainActor
    @Test("Draft survives missing and offline server replacements")
    func durableDraftSurvivesOutage() throws {
        var uploading = draft(phase: .retained)
        uploading.phase = .uploading
        uploading.phaseLabel = nil
        uploading.mediaID = mediaID
        let model = PulsarStreamTrackModel(snapshot: fixture(draft: uploading))
        var outage = fixture(state: .offline)
        outage.draft = nil
        outage.actions = allActions
        model.replace(outage, now: now)
        let kept = try #require(model.snapshot.draft)
        #expect(kept.localID == localID)
        #expect(kept.retainedLocalBytes)
        #expect(model.snapshot.actions.isEmpty)
        #expect(model.buildCommand(.upload(localID: localID)) == nil)
    }

    @MainActor
    @Test("Client MIME and local progress never manufacture server readiness")
    func readinessIsServerOwned() throws {
        var candidate = draft()
        candidate.serverMetadataConfirmed = false
        candidate.variantManifest = nil
        candidate.processingPercent = 150
        candidate.uploadOffset = candidate.localByteCount + 1
        let model = PulsarStreamTrackModel()
        model.replace(fixture(draft: candidate), now: now)
        let normalized = try #require(model.snapshot.draft)
        #expect(normalized.phase == .processing)
        #expect(normalized.processingPercent == 100)
        #expect(normalized.uploadOffset == normalized.localByteCount)
        #expect(model.buildCommand(.queue(mediaID: mediaID, audience: .explicit, targets: [reference])) == nil)
    }

    @MainActor
    @Test("Commands require fresh capabilities, policy, exact targets, and generations")
    func commandsFailClosed() {
        let model = PulsarStreamTrackModel(snapshot: fixture())
        #expect(model.buildCommand(.queue(mediaID: mediaID, audience: .explicit, targets: [reference])) != nil)
        #expect(model.buildCommand(.queue(mediaID: mediaID, audience: .explicit, targets: [])) == nil)
        #expect(model.buildCommand(.pause(streamID: streamID, playbackGeneration: 7)) != nil)
        #expect(model.buildCommand(.pause(streamID: streamID, playbackGeneration: 6)) == nil)
        #expect(model.buildCommand(.seek(
            streamID: streamID, positionMS: 5_000, playbackGeneration: 7, seekGeneration: 3)) != nil)
        #expect(model.buildCommand(.delete(localID: localID, confirmed: false)) == nil)
        #expect(model.buildCommand(.delete(localID: localID, confirmed: true)) != nil)
        #expect(model.selectAudience(.currentAir))
        #expect(model.selectInsertion(.replace))
        #expect(model.buildCommand(.replace(mediaID: mediaID, audience: .currentAir, targets: [])) != nil)
        #expect(model.selectTargets([reference]))
        #expect(model.selectAudience(.explicit))
        #expect(model.selectInsertion(.queue))
    }

    @MainActor
    @Test("Stale playback and seek generations cannot move audible progress backward")
    func generationFencing() {
        let model = PulsarStreamTrackModel(snapshot: fixture(position: 5_000))
        model.replace(fixture(playbackGeneration: 6, position: 9_000), now: now)
        #expect(model.snapshot.playback.playbackGeneration == 7)
        #expect(model.snapshot.playback.audiblePositionMS == 5_000)

        model.replace(fixture(seekGeneration: 3, position: 1_000), now: now)
        #expect(model.snapshot.playback.audiblePositionMS == 5_000)

        model.replace(fixture(seekGeneration: 4, position: 1_000), now: now)
        #expect(model.snapshot.playback.seekGeneration == 4)
        #expect(model.snapshot.playback.audiblePositionMS == 1_000)
        #expect(model.applyOptimistic(.seek(
            streamID: streamID, positionMS: 2_000, playbackGeneration: 7, seekGeneration: 4)))
        #expect(model.snapshot.playback.phase == .seeking)
        #expect(model.snapshot.playback.seekGeneration == 5)
    }

    @MainActor
    @Test("Delete stays pending until server confirmation")
    func deleteIsNotOptimistic() {
        let model = PulsarStreamTrackModel(snapshot: fixture())
        #expect(model.applyOptimistic(.delete(localID: localID, confirmed: true)))
        #expect(model.snapshot.draft?.localID == localID)
        #expect(model.snapshot.draft?.retainedLocalBytes == true)
        var confirmed = fixture()
        confirmed.draft = nil
        confirmed.confirmedDeletedLocalID = localID
        model.replace(confirmed, now: now)
        #expect(model.snapshot.draft == nil)
    }

    @Test("Opaque values are absent from descriptions")
    func opaqueDescriptions() {
        let command = PulsarStreamTrackCommand.queue(
            mediaID: mediaID, audience: .explicit, targets: [reference])
        let snapshot = fixture()
        for text in [command.description, command.debugDescription, draft().description, snapshot.description] {
            #expect(!text.contains(mediaID))
            #expect(!text.contains(reference))
            #expect(!text.contains(localID))
        }
    }

    @Test("Portable contract and Swift enums remain exact")
    func contractParity() throws {
        let root = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent()
        let data = try Data(contentsOf: root.appendingPathComponent("protocol/pulsar-stream-track-ui-model-v1.json"))
        let contract = try #require(JSONSerialization.jsonObject(with: data) as? [String: Any])
        #expect(contract["contract_id"] as? String == "pulsar.stream-track-ui-model.v1")
        #expect(contract["draft_phases"] as? [String] == PulsarStreamTrackDraftPhase.allCases.map(\.rawValue))
        #expect(contract["playback_phases"] as? [String] == PulsarStreamTrackPlaybackPhase.allCases.map(\.rawValue))
        #expect(contract["audiences"] as? [String] == PulsarStreamTrackAudience.allCases.map(\.rawValue))
        #expect(contract["insertions"] as? [String] == PulsarStreamTrackInsertion.allCases.map(\.rawValue))
        #expect(contract["failure_codes"] as? [String] == PulsarStreamTrackFailure.allCases.map(\.rawValue))
    }

    @MainActor
    @Test("Native actions preserve explicit per-attempt rights acknowledgement")
    func explicitRightsAction() {
        var events: [(PulsarStreamTrackCommand, Bool)] = []
        let actions = PulsarStreamTrackActions { command, rights in
            events.append((command, rights))
        }
        let upload = PulsarStreamTrackCommand.upload(localID: localID)
        actions.perform(upload)
        actions.perform(upload, rightsAcknowledged: true)
        #expect(events.count == 2)
        #expect(events[0].0 == upload && events[0].1 == false)
        #expect(events[1].0 == upload && events[1].1 == true)
    }

    @Test("macOS surface keeps deterministic keyboard, drop, rights, and accessibility evidence")
    func macSurfaceSourceEvidence() throws {
        let root = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent()
        let view = try String(contentsOf: root.appendingPathComponent(
            "node-app/Sources/NodeAppUI/PulsarStreamTrackView.swift"))
        let composition = try String(contentsOf: root.appendingPathComponent(
            "node-app/Sources/NodeApp/MacStreamTrackAppComposition.swift"))

        for marker in [
            ".fileImporter(", ".dropDestination(for: URL.self)",
            ".keyboardShortcut(\"l\"", ".accessibilityValue(",
            "I confirm the rights", "Подтверждаю права", "rightsAcknowledged: true",
        ] {
            #expect(view.contains(marker))
        }
        #expect(composition.contains("guard rightsAcknowledged else { return }"))
        #expect(composition.contains("publishes no corresponding server action under no-go"))
    }
}
