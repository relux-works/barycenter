import Foundation
import Testing
@testable import NodeCore

private struct E2EEAuditVectors: Decodable {
    struct Baseline: Decodable {
        let groupId: String
        let airId: String
        let targetSnapshotDigest: String
        let epoch: UInt64
        let commitDigest: String
    }
    struct Malformed: Decodable {
        let name: String
        let mutation: String
        let value: String
        let expected: String
    }
    let status: String
    let fixtureSuite: String
    let baseline: Baseline
    let validContent: E2EEAuditMetadata
    let validCommit: E2EEAuditCommit
    let malformedVectors: [Malformed]
}

private func e2eeVectorsURL() -> URL {
    URL(fileURLWithPath: #filePath)
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .deletingLastPathComponent()
        .appendingPathComponent("../protocol/e2ee-media-audit-v1-vectors.json")
        .standardizedFileURL
}

private func loadE2EEVectors() throws -> (E2EEAuditVectors, Data) {
    let data = try Data(contentsOf: e2eeVectorsURL())
    return (try JSONDecoder().decode(E2EEAuditVectors.self, from: data), data)
}

private func auditConfiguration(_ suite: String) -> E2EEAuditConfiguration {
    E2EEAuditConfiguration(allowedSuites: [suite]) { _, signature in
        signature == "fixture-valid"
    }
}

private func auditState(_ fixture: E2EEAuditVectors) -> E2EEAuditState {
    E2EEAuditState(
        groupID: fixture.baseline.groupId,
        airID: fixture.baseline.airId,
        targetSnapshotDigest: fixture.baseline.targetSnapshotDigest,
        epoch: fixture.baseline.epoch,
        commitDigest: fixture.baseline.commitDigest
    )
}

@Suite struct E2EEAuditContractTests {
    @Test func sharedMalformedVectorsFailClosed() throws {
        let (fixture, raw) = try loadE2EEVectors()
        #expect(fixture.status == "audit-only-production-disabled")
        var state = auditState(fixture)
        try state.accept(fixture.validContent, trustedManifestDigest: fixture.validContent.manifestDigest, nowMS: 1000,
                         configuration: auditConfiguration(fixture.fixtureSuite))

        var disabled = auditState(fixture)
        #expect(throws: E2EEAuditFailure.unknownSuite) {
            try disabled.accept(fixture.validContent, trustedManifestDigest: fixture.validContent.manifestDigest, nowMS: 1000,
                                configuration: .productionDisabled)
        }

        let root = try #require(JSONSerialization.jsonObject(with: raw) as? [String: Any])
        let base = try #require(root["validContent"] as? [String: Any])
        for vector in fixture.malformedVectors {
            var object = base
            if vector.mutation == "epoch" || vector.mutation == "expires_at_ms" {
                object[vector.mutation] = Int64(vector.value)
            } else {
                object[vector.mutation] = vector.value
            }
            let data = try JSONSerialization.data(withJSONObject: object, options: [.sortedKeys])
            let candidate = try E2EEAuditMetadata.decodeCoordinatorVisible(data)
            var candidateState = auditState(fixture)
            if vector.mutation == "nonce" { candidateState.remember(nonce: vector.value) }
            if vector.mutation == "event_id" { candidateState.remember(eventID: vector.value) }
            let expected = try #require(E2EEAuditFailure(rawValue: vector.expected))
            #expect(throws: expected, "vector \(vector.name)") {
                try candidateState.accept(candidate, trustedManifestDigest: fixture.validContent.manifestDigest, nowMS: 1000,
                                          configuration: auditConfiguration(fixture.fixtureSuite))
            }
        }
    }

    @Test func coordinatorVisibleDecoderRejectsSecretsAndUnknownFields() throws {
        let (fixture, _) = try loadE2EEVectors()
        let encoded = try JSONEncoder().encode(fixture.validContent)
        var object = try #require(JSONSerialization.jsonObject(with: encoded) as? [String: Any])
        for key in ["content_key", "epoch_secret", "plaintext", "private_key"] {
            object[key] = "must-never-route"
            let raw = try JSONSerialization.data(withJSONObject: object)
            #expect(throws: E2EEAuditFailure.malformed) {
                _ = try E2EEAuditMetadata.decodeCoordinatorVisible(raw)
            }
            object.removeValue(forKey: key)
        }
    }

    @Test func sharedCommitVectorOrdersEpochAndRejectsFork() throws {
        let (fixture, _) = try loadE2EEVectors()
        var state = auditState(fixture)
        try state.apply(fixture.validCommit, configuration: auditConfiguration(fixture.fixtureSuite))
        #expect(state.epoch == 8)
        #expect(state.targetSnapshotDigest == fixture.validCommit.targetSnapshotDigest)
        #expect(throws: E2EEAuditFailure.replay) {
            try state.apply(fixture.validCommit, configuration: auditConfiguration(fixture.fixtureSuite))
        }

        var fork = fixture.validCommit
        fork.eventID = "fork"
        fork.previousCommitDigest = String(repeating: "a", count: 64)
        var fresh = auditState(fixture)
        #expect(throws: E2EEAuditFailure.forkedEpoch) {
            try fresh.apply(fork, configuration: auditConfiguration(fixture.fixtureSuite))
        }
    }
}
