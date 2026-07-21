import Testing

@testable import NodeCore

@Suite("Credential activation gate")
struct CredentialActivationGateTests {
  @Test("Created credentials require explicit recovery acknowledgement")
  func createdBundleRequiresRecoveryExport() throws {
    let node = NodeCapability(
      orbitId: 1,
      slot: "alpha",
      nodeToken: String(repeating: "a", count: 64),
      wsUrl: "wss://coord.example.com/ws")
    var bundle = CredentialBundle(
      coordinatorOrigin: try CoordinatorOrigin("https://coord.example.com"),
      node: node,
      recovery: RecoveryMetadata(
        actorId: 2,
        recoveryId: "rec_" + String(repeating: "d", count: 32),
        explicitBackupAcknowledged: false))

    #expect(bundle.activationEligibleNodeCredentials == nil)
    bundle.recovery?.explicitBackupAcknowledged = true
    #expect(bundle.activationEligibleNodeCredentials == node.legacyView)
  }

  @Test("Joined and legacy credentials are not given a synthetic recovery gate")
  func bundlesWithoutRecoveryRemainEligible() {
    let credentials = NodeCredentials(
      orbitId: 3,
      slot: "beta",
      token: String(repeating: "b", count: 64),
      wsUrl: "wss://coord.example.com/ws")
    #expect(
      CredentialBundle.legacy(credentials).activationEligibleNodeCredentials
        == credentials)
  }
}
