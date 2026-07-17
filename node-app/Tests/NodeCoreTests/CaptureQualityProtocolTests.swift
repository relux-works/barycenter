import Foundation
import Testing
@testable import NodeCore

private func captureQualityVectorURL() -> URL {
    URL(fileURLWithPath: #filePath)
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .appendingPathComponent("../protocol/capture-quality-v1-vectors.json")
        .standardizedFileURL
}

@Suite struct CaptureQualityProtocolTests {
    @Test func sharedMalformedAndGenerationVectorsFailClosed() throws {
        let data = try Data(contentsOf: captureQualityVectorURL())
        let root = try #require(JSONSerialization.jsonObject(with: data) as? [String: Any])
        #expect(root["schemaVersion"] as? Int == 1)
        #expect(root["contract"] as? String == "capture-quality.v1-vectors")
        let validObject = try #require(root["validState"] as? [String: Any])
        let validData = try JSONSerialization.data(withJSONObject: validObject)
        let valid = try JSONDecoder().decode(CaptureQualityState.self, from: validData)
        try CaptureQualityContract.validate(valid)

        let mutations = try #require(root["invalidMutations"] as? [[String: Any]])
        for mutation in mutations {
            var object = validObject
            let field = try #require(mutation["field"] as? String)
            object[field] = mutation["value"]
            let candidateData = try JSONSerialization.data(withJSONObject: object)
            var rejected = false
            do {
                let candidate = try JSONDecoder().decode(CaptureQualityState.self, from: candidateData)
                try CaptureQualityContract.validate(candidate)
            } catch {
                rejected = true
            }
            #expect(rejected, "accepted malformed vector \(mutation["name"] ?? "unknown")")
        }

        var guardState = CaptureQualityGenerationGuard()
        let sequence = try #require(root["generationSequence"] as? [[String: Any]])
        for vector in sequence {
            let generation = try #require(vector["generation"] as? Int64)
            let updated = try #require(vector["updated_monotonic_ms"] as? Int64)
            let expected = try #require(vector["expected"] as? String)
            #expect(guardState.accept(generation: generation, updatedMs: updated).rawValue == expected)
        }
    }

    @Test func stateCodecGuidanceAndDiagnosticsStayPrivacySafe() throws {
        let data = try Data(contentsOf: captureQualityVectorURL())
        let root = try #require(JSONSerialization.jsonObject(with: data) as? [String: Any])
        let validObject = try #require(root["validState"] as? [String: Any])
        let valid = try JSONDecoder().decode(
            CaptureQualityState.self,
            from: JSONSerialization.data(withJSONObject: validObject))

        let state = StatePayload(
            playback: "stopped", uri: nil, positionMs: 0, volume: 80,
            degraded: false, underruns: 0, rttMs: 20, speakers: [],
            captureQuality: valid)
        let encoded = try ProtocolCodec.encode(id: "msg_x", ts: 1, message: .state(state))
        let (_, decoded) = try ProtocolCodec.decode(encoded)
        guard case .state(let decodedState) = decoded else {
            Issue.record("state decoded as wrong message")
            return
        }
        #expect(decodedState.captureQuality == valid)

        let withheld = CaptureQualityContract.heartbeatState(
            state, advertisedCapabilities: [mediaClipCapability])
        #expect(withheld.captureQuality == nil)
        let advertised = CaptureQualityContract.heartbeatState(
            state, advertisedCapabilities: [captureQualityCapability])
        #expect(advertised.captureQuality == valid)

        let legacy = CaptureQualityPresentation.guidance(
            capabilities: [mediaClipCapability], state: nil)
        #expect(legacy == CaptureQualityGuidance(
            quality: "unsupported", reason: "mixed_version",
            key: "capture_quality.mixed_version"))
        let accepted = CaptureQualityPresentation.guidance(
            capabilities: [captureQualityCapability], state: valid)
        #expect(accepted.key == "capture_quality.accepted.none")
        #expect(accepted.available)
        #expect(accepted.requestedMode == "auto")
        #expect(accepted.resolvedMode == "speaker")
        #expect(accepted.aec == "active")
        #expect(accepted.ns == "active")
        #expect(accepted.agc == "active")
        #expect(accepted.inputHealth == "ok")
        #expect(accepted.inputCeilingDBFS == -3)
        #expect(accepted.outputCeilingDBFS == -1)

        let fields = CaptureQualityPresentation.diagnosticLogFields(valid)
        let text = String(decoding: try JSONSerialization.data(withJSONObject: fields), as: UTF8.self).lowercased()
        for forbidden in ["audio", "sample", "device", "path", "file", "transcript", "reference_age", "input_ceiling"] {
            #expect(!text.contains(forbidden), "diagnostic projection leaked \(forbidden)")
        }
    }
}
