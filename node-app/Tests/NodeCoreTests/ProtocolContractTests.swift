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

@Suite struct ProtocolContractTests {
    static let expectedTypeCount = 26

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
    }
}
