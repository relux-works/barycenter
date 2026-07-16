import Foundation
import Testing
@testable import NodeAppUI

struct PulsarTargetsInboxModelTests {
    private let now = Date(timeIntervalSince1970: 1_800_000_000)
    private let reference = "trf_" + String(repeating: "A", count: 43)
    private let inboxID = "ib_01J00000000000000000000000"
    private let historyID = "hi_01J00000000000000000000000"

    private var replayLabel: PulsarLocalizedLabel {
        .init(key: "action.replay", en: "Replay", ru: "Повторить")
    }

    private func fixture(state: PulsarTargetsInboxSurfaceState = .ready) -> PulsarTargetsInboxSnapshot {
        .init(
            state: state,
            activeAirTitle: "Family",
            availableAudiences: [
                .init(kind: .thisPulsar, label: replayLabel),
                .init(kind: .ownBarycenter, label: replayLabel),
                .init(kind: .currentAir, label: replayLabel),
                .init(kind: .explicit, label: replayLabel),
            ],
            selectedAudience: .ownBarycenter,
            targets: [
                .init(
                    reference: reference, kind: "barycenter", expiresAt: now.addingTimeInterval(3600),
                    capabilityState: "known",
                    capabilities: ["overlay_mix_v1", "media_clip_v1", "media_clip_v1"],
                    label: .init(key: "target.orbit", en: "Family", ru: "Family")),
                .init(
                    reference: "trf_" + String(repeating: "B", count: 43), kind: "pulsar",
                    expiresAt: now.addingTimeInterval(-1), capabilityState: "unknown",
                    capabilities: [], label: replayLabel),
            ],
            selectedReferences: [reference, "trf_" + String(repeating: "B", count: 43)],
            targetedTrackPolicy: "clip",
            contentPolicyState: "current",
            inbox: [
                .init(
                    id: inboxID, historyItemID: historyID, title: "Voice",
                    expiresAt: now.addingTimeInterval(3600), availability: "available",
                    sender: replayLabel, source: replayLabel, requestedDelivery: replayLabel,
                    effectiveDelivery: replayLabel, receipt: replayLabel,
                    actions: [
                        .init(action: "replay", label: replayLabel),
                        .init(action: "dismiss", label: replayLabel),
                        .init(action: "report", label: replayLabel),
                        .init(action: "block_actor", label: replayLabel),
                    ]),
            ],
            inboxNextCursor: "ic_" + String(repeating: "a", count: 64),
            history: [
                .init(
                    id: historyID, title: "Voice", status: replayLabel,
                    actions: [
                        .init(action: "delete", label: replayLabel),
                        .init(action: "report", label: replayLabel),
                        .init(action: "block_actor", label: replayLabel),
                    ],
                    playedCount: 1, otherCount: 2,
                    receipts: .init(nextCursor: "rc_" + String(repeating: "b", count: 64))),
            ],
            historyNextCursor: "hc_" + String(repeating: "c", count: 64))
    }

    @MainActor
    @Test("Replacement prunes expired targets, duplicate capabilities, and stale authority")
    func replacementIsDeterministic() {
        let model = PulsarTargetsInboxModel()
        model.replace(fixture(), now: now)
        #expect(model.snapshot.targets.count == 1)
        #expect(model.snapshot.selectedReferences == [reference])
        #expect(model.snapshot.targets[0].capabilities == ["media_clip_v1", "overlay_mix_v1"])

        model.replace(fixture(state: .stale), now: now)
        #expect(model.snapshot.inbox.count == 1)
        #expect(model.snapshot.history.count == 1)
        #expect(model.snapshot.selectedReferences.isEmpty)
        #expect(model.deleteHistoryCommand(id: historyID) == nil)
        #expect(model.refreshCommand() == .refresh)
    }

    @MainActor
    @Test("Commands can only be built from current server capabilities")
    func commandsFailClosed() {
        let model = PulsarTargetsInboxModel()
        model.replace(fixture(), now: now)
        #expect(model.selectTargetsCommand([reference]) == .selectTargets([reference]))
        #expect(model.setAudienceCommand(.currentAir) == .setAudience(.currentAir))
        #expect(model.setAudienceCommand(.explicit) == .setAudience(.explicit))
        #expect(model.selectTargetsCommand(["trf_" + String(repeating: "Z", count: 43)]) == nil)
        #expect(model.loadMoreInboxCommand() != nil)
        #expect(model.loadMoreHistoryCommand() != nil)
        #expect(model.loadMoreReceiptsCommand(historyItemID: historyID) != nil)
        #expect(model.replayInboxCommand(id: inboxID, delivery: .overlay) != nil)
        #expect(model.replayInboxCommand(id: historyID, delivery: .overlay) == nil)
        #expect(model.dismissInboxCommand(id: inboxID) != nil)
        #expect(model.deleteHistoryCommand(id: historyID) != nil)
        #expect(model.reportHistoryCommand(id: historyID, reason: .spam, details: "") != nil)
        #expect(model.reportInboxCommand(id: inboxID, reason: .spam, details: "") ==
            .reportInbox(id: historyID, reason: .spam, details: ""))
        #expect(model.muteSenderCommand(id: historyID) != nil)
        #expect(model.muteSenderCommand(id: inboxID) == .muteSender(id: historyID))
    }

    @Test("Opaque references, object capabilities, and cursors are redacted")
    func descriptionsAreRedacted() {
        let target = fixture().targets[0]
        let command = PulsarTargetsInboxCommand.selectTargets([reference])
        for rendered in [target.description, target.debugDescription, command.description, command.debugDescription] {
            #expect(!rendered.contains(reference))
            #expect(!rendered.contains(inboxID))
            #expect(!rendered.contains("ic_"))
        }
    }

    @Test("Portable contract and Swift enums stay exact")
    func portableContractParity() throws {
        let root = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent().deletingLastPathComponent()
            .deletingLastPathComponent().deletingLastPathComponent()
        let data = try Data(contentsOf:
            root.appendingPathComponent("protocol/pulsar-targets-inbox-presentation-v1.json"))
        let contract = try #require(JSONSerialization.jsonObject(with: data) as? [String: Any])
        #expect(contract["contract_id"] as? String == "pulsar.targets-inbox-presentation.v1")
        #expect(contract["surface_states"] as? [String] == PulsarTargetsInboxSurfaceState.allCases.map(\.rawValue))
        let playback = try #require(contract["playback"] as? [String: Any])
        #expect(playback["late_inbox_autoplay_command_exists"] as? Bool == false)
        let commands = try #require(contract["commands"] as? [String: Any])
        for command in [
            "refresh", "set_audience", "select_targets", "set_include_origin", "load_more_inbox",
            "load_more_history", "load_more_receipts", "replay_inbox", "dismiss_inbox",
            "delete_history", "report_inbox", "report_history", "mute_sender",
        ] {
            #expect(commands[command] != nil)
        }
    }
}
