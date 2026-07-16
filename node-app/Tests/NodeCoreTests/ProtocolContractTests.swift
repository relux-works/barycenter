// Contract tests against protocol/golden (spec 8.7): every golden file must
// decode into a typed Message and re-encode to a semantically identical JSON.

import Foundation
import Testing
@testable import NodeCore

private func goldenDir() -> URL {
    // node-app/Tests/NodeCoreTests -> repo root -> protocol/golden
    URL(fileURLWithPath: #filePath)
        .deletingLastPathComponent() // NodeCoreTests
        .deletingLastPathComponent() // Tests
        .deletingLastPathComponent() // node-app
        .appendingPathComponent("../protocol/golden")
        .standardizedFileURL
}

private func jsonObject(_ data: Data) throws -> NSDictionary {
    try #require(JSONSerialization.jsonObject(with: data) as? NSDictionary)
}

private func isMessageID(_ value: String) -> Bool {
    let alphabet = Set("0123456789ABCDEFGHJKMNPQRSTVWXYZ")
    guard value.hasPrefix("msg_") else { return false }
    let suffix = value.dropFirst(4)
    guard suffix.utf8.count == 26, let first = suffix.first,
          "01234567".contains(first) else { return false }
    return suffix.allSatisfy { alphabet.contains($0) }
}

@Suite struct ProtocolContractTests {
    static let expectedTypeCount = 51

    @Test func goldenDirComplete() throws {
        let files = try FileManager.default.contentsOfDirectory(at: goldenDir(), includingPropertiesForKeys: nil)
            .filter { $0.pathExtension == "json" }
        #expect(files.count == Self.expectedTypeCount,
                "golden dir has \(files.count) files, expected \(Self.expectedTypeCount)")
    }

    @Test func roundTripEveryGolden() throws {
        let files = try FileManager.default.contentsOfDirectory(at: goldenDir(), includingPropertiesForKeys: nil)
            .filter { $0.pathExtension == "json" }
            .sorted { $0.lastPathComponent < $1.lastPathComponent }
        #expect(!files.isEmpty)

        for file in files {
            let raw = try Data(contentsOf: file)
            let (head, message) = try ProtocolCodec.decode(raw)
            #expect(head.v == ProtocolConstants.version)
            #expect(isMessageID(head.id), "invalid golden message id \(head.id)")
            #expect(head.type == file.deletingPathExtension().lastPathComponent,
                    "file \(file.lastPathComponent) carries type \(head.type)")
            #expect(message.typeName == head.type)

            let encoded = try ProtocolCodec.encode(id: head.id, ts: head.ts, message: message)
            let want = try jsonObject(raw)
            let got = try jsonObject(encoded)
            #expect(want == got,
                    "round-trip mismatch for \(file.lastPathComponent):\n\(String(data: encoded, encoding: .utf8) ?? "")")
        }
    }

    @Test func optionalFieldsAreOmittedNotNull() throws {
        let data = try ProtocolCodec.encode(
            id: "msg_x", ts: 1,
            message: .error(ErrorPayload(code: "load_failed", message: "m", elementId: nil))
        )
        let text = String(decoding: data, as: UTF8.self)
        #expect(!text.contains("element_id"))

        let voice = try ProtocolCodec.encode(
            id: "msg_x", ts: 1,
            message: .playVoice(PlayVoicePayload(elementId: "el", fileUrl: "http://c/m.wav", tCoordMs: nil))
        )
        #expect(!String(decoding: voice, as: UTF8.self).contains("t_coord_ms"))

        let external = try ProtocolCodec.encode(
            id: "msg_x", ts: 1,
            message: .externalPlayback(ExternalPlaybackPayload(uri: "spotify:track:x", positionMs: nil))
        )
        #expect(!String(decoding: external, as: UTF8.self).contains("position_ms"))

        let interrupt = try ProtocolCodec.encode(
            id: "msg_x", ts: 1,
            message: .playMediaAt(PlayMediaAtPayload(
                transmissionId: "tr_x", generation: 1, tCoordMs: 2,
                startDeadlineCoordMs: 102, delivery: "interrupt",
                duckDb: nil, attackMs: nil, releaseMs: nil, fadeOutMs: 250, fadeInMs: 120))
        )
        let interruptText = String(decoding: interrupt, as: UTF8.self)
        #expect(!interruptText.contains("duck_db"))
        #expect(!interruptText.contains("attack_ms"))
        #expect(!interruptText.contains("release_ms"))
        let (_, decodedInterrupt) = try ProtocolCodec.decode(interrupt)
        #expect(decodedInterrupt.typeName == "play_media_at")

        let dnd = try ProtocolCodec.encode(
            id: "msg_x", ts: 1,
            message: .setDND(SetDNDPayload(
                revision: 1, mode: "allow_all", mutedUntilCoordMs: nil))
        )
        #expect(!String(decoding: dnd, as: UTF8.self).contains("muted_until_coord_ms"))

        let presence = try ProtocolCodec.encode(
            id: "msg_x", ts: 1,
            message: .presenceUpdate(PresenceUpdatePayload(
                revision: 1, generatedAtCoordMs: 1,
                nodes: [PresenceNode(
                    orbitId: 1, slot: "a", online: true, lastSeenAtCoordMs: 1,
                    outputState: "ready", playbackState: "main",
                    dndMode: "messages_only", dndRevision: 1, dndUntilCoordMs: nil,
                    capabilities: [], interruptResumeReady: false)]))
        )
        #expect(!String(decoding: presence, as: UTF8.self).contains("dnd_until_coord_ms"))
    }

    @Test func capabilityListsAreCanonicalAndAdditive() {
        let capabilities = [
            interruptResumeCapability,
            mediaClipCapability,
            overlayMixCapability,
            seamlessAdoptionCapability,
            streamTrackCapability,
            "unknown_future_v2"
        ]
        #expect(ProtocolCapabilities.areCanonical(capabilities))
        #expect(!ProtocolCapabilities.areCanonical([mediaClipCapability, mediaClipCapability]))
        #expect(!ProtocolCapabilities.areCanonical([overlayMixCapability, mediaClipCapability]))
        #expect(!ProtocolCapabilities.areCanonical(["media clip"]))
        #expect(!ProtocolCapabilities.areCanonical(["média_clip_v1"]))
    }

    @Test func streamGenerationGuardRejectsStaleReorderedAndEarlyStart() {
        var guardState = StreamGenerationGuard()
        #expect(guardState.acceptLoad(playback: 7, seek: 0, command: 1) == .apply)
        #expect(guardState.acceptLoad(playback: 7, seek: 0, command: 1) == .duplicate)
        #expect(guardState.acceptEvent(playback: 7, seek: 0, event: 1, kind: .started) == .invalid)
        #expect(guardState.acceptReady(playback: 7, seek: 0, event: 1,
                                      buffered: 1999, minimum: 2000) == .invalid)
        #expect(guardState.acceptReady(playback: 7, seek: 0, event: 1,
                                      buffered: 2500, minimum: 2000) == .apply)
        #expect(guardState.acceptCommand(playback: 7, seek: 0, command: 2,
                                         kind: "resume") == .apply)
        #expect(guardState.acceptEvent(playback: 7, seek: 0, event: 2,
                                      kind: .started) == .apply)
        #expect(guardState.acceptSeek(playback: 7, seek: 1, command: 3) == .apply)
        #expect(guardState.acceptEvent(playback: 7, seek: 0, event: 3,
                                      kind: .ended) == .stale)
        #expect(guardState.acceptReady(playback: 7, seek: 1, event: 1,
                                      buffered: 2000, minimum: 2000) == .apply)
        #expect(guardState.acceptCommand(playback: 7, seek: 1, command: 4,
                                         kind: "resume") == .apply)
        #expect(guardState.acceptEvent(playback: 7, seek: 1, event: 2,
                                      kind: .started) == .apply)
        #expect(guardState.acceptEvent(playback: 7, seek: 1, event: 4,
                                      kind: .progress) == .invalid)
        #expect(guardState.acceptEvent(playback: 7, seek: 1, event: 3,
                                      kind: .ended) == .apply)
        #expect(guardState.acceptLoad(playback: 8, seek: 0, command: 1) == .apply)
        #expect(guardState.acceptEvent(playback: 7, seek: 1, event: 4,
                                      kind: .ended) == .stale)

        var pausedDuringRebuffer = StreamGenerationGuard()
        _ = pausedDuringRebuffer.acceptLoad(playback: 1, seek: 0, command: 1)
        _ = pausedDuringRebuffer.acceptReady(playback: 1, seek: 0, event: 1,
                                             buffered: 2000, minimum: 2000)
        _ = pausedDuringRebuffer.acceptCommand(playback: 1, seek: 0, command: 2,
                                                kind: "resume")
        _ = pausedDuringRebuffer.acceptEvent(playback: 1, seek: 0, event: 2,
                                             kind: .started)
        _ = pausedDuringRebuffer.acceptEvent(playback: 1, seek: 0, event: 3,
                                             kind: .rebuffer)
        #expect(pausedDuringRebuffer.acceptCommand(playback: 1, seek: 0, command: 3,
                                                   kind: "pause") == .apply)
        #expect(pausedDuringRebuffer.acceptCommand(playback: 1, seek: 0, command: 4,
                                                   kind: "resume") == .invalid)
        #expect(pausedDuringRebuffer.acceptReady(playback: 1, seek: 0, event: 4,
                                                 buffered: 2000, minimum: 2000) == .apply)
        #expect(pausedDuringRebuffer.acceptCommand(playback: 1, seek: 0, command: 4,
                                                   kind: "resume") == .apply)
    }

    @Test func streamLoadAndReadyValidationFailsClosed() throws {
        let loadData = try Data(contentsOf: goldenDir().appendingPathComponent("stream_load.json"))
        let (_, loadMessage) = try ProtocolCodec.decode(loadData)
        guard case .streamLoad(let decodedLoad) = loadMessage else {
            Issue.record("stream_load golden decoded as wrong type")
            return
        }
        var load = decodedLoad
        #expect(StreamContract.validate(load: load))
        load.variantUrl = "https://token@example/v1/media/\(load.mediaId)/variants/sv_x"
        #expect(!StreamContract.validate(load: load))

        let readyData = try Data(contentsOf: goldenDir().appendingPathComponent("stream_ready.json"))
        let (_, readyMessage) = try ProtocolCodec.decode(readyData)
        guard case .streamReady(let decodedReady) = readyMessage else {
            Issue.record("stream_ready golden decoded as wrong type")
            return
        }
        var ready = decodedReady
        #expect(StreamContract.validate(ready: ready))
        ready.bufferedDurationMs = 1999
        #expect(!StreamContract.validate(ready: ready))
    }

    @Test func legacyVoiceMessagesRemainCompatible() throws {
        let messages: [Message] = [
            .playVoice(PlayVoicePayload(
                elementId: "el_x", fileUrl: "https://coord/media/x", tCoordMs: nil)),
            .soloVoice(SoloVoicePayload(
                elementId: "el_x", fileUrl: "https://coord/media/x"))
        ]
        for message in messages {
            let encoded = try ProtocolCodec.encode(id: "msg_x", ts: 1, message: message)
            let (_, decoded) = try ProtocolCodec.decode(encoded)
            #expect(decoded.typeName == message.typeName)
        }
    }

    @Test func unknownTypeIsDetectable() {
        let frame = Data(#"{"v":1,"id":"msg_x","ts":1,"type":"hologram","payload":{}}"#.utf8)
        #expect(throws: ProtocolError.self) {
            _ = try ProtocolCodec.decode(frame)
        }
    }

    @Test func versionMismatchRejected() {
        let frame = Data(#"{"v":2,"id":"msg_x","ts":1,"type":"ping","payload":{"t1":1}}"#.utf8)
        #expect(throws: ProtocolError.self) {
            _ = try ProtocolCodec.decode(frame)
        }
        switch CoordinatorClient.classifyIncoming(frame) {
        case .reconnectVersion(let version): #expect(version == 2)
        default: Issue.record("runtime client did not reconnect on a major-version mismatch")
        }
    }
}
