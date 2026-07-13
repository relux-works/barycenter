import Foundation
import Testing

@testable import NodeCore

@Suite(.serialized)
struct OnboardingServiceTests {
  @Test func createReturnsOneTimeMaterialEvenWhenProtectedSaveFailsWithoutPersistingSecret()
    async throws
  {
    let store = MemoryProtectedStore()
    store.failure = { kind, _, _ in
      kind == .add ? ProtectedStoreFailure.unavailable : nil
    }
    let transport = ScriptedTransport { request, _ in
      testHTTPResponse(request: request, status: 201, json: createdJSON)
    }
    let service = OnboardingService(
      client: try OnboardingHTTPClient(
        coordinator: "https://coord.example.com", transport: transport
      ),
      credentials: CredentialRepository(store: store, files: MemoryCredentialFiles())
    )
    let outcome = try await service.createOrbit(
      title: "Orbit", installationAttemptID: "installation-attempt-0001"
    )
    #expect(!outcome.wasStored)
    #expect(outcome.value.bundle.node?.nodeToken == testNodeToken)
    let export = try RecoveryExportHelper.payload(for: outcome.value.recovery)
    #expect(export.contains(testRecoverySecret))
    #expect(store.allData().isEmpty)
  }

  @Test func rotateStoresOnlyMetadataAndAcknowledgesBackupOnlyOnExplicitAction() async throws {
    let store = MemoryProtectedStore()
    let repository = CredentialRepository(store: store, files: MemoryCredentialFiles())
    let origin = try CoordinatorOrigin("https://coord.example.com")
    let bundle = CredentialBundle(
      coordinatorOrigin: origin,
      node: NodeCapability(
        orbitId: 1, slot: "a", nodeToken: testNodeToken,
        wsUrl: "wss://coord.example.com/ws"
      ),
      control: ControlCapability(
        actorId: 2, orbitId: 1, role: .primary, controlToken: testControlToken
      ),
      recovery: RecoveryMetadata(
        actorId: 2, recoveryId: "rec_" + String(repeating: "e", count: 32),
        explicitBackupAcknowledged: true
      )
    )
    try repository.saveBundle(bundle)
    let transport = ScriptedTransport { request, _ in
      testHTTPResponse(
        request: request, status: 200,
        json:
          #"{"actor_id":2,"recovery_id":"\#(testRecoveryID)","recovery_secret":"\#(testRecoverySecret)","shown_once":true}"#
      )
    }
    let service = OnboardingService(
      client: try OnboardingHTTPClient(
        coordinator: origin.rawValue, transport: transport
      ),
      credentials: repository
    )
    let rotated = try await service.rotateRecovery()
    #expect(rotated.wasStored)
    var saved = try #require(try repository.loadBundleWithoutMigration())
    #expect(saved.node == bundle.node)
    #expect(saved.control == bundle.control)
    #expect(saved.recovery?.recoveryId == testRecoveryID)
    #expect(saved.recovery?.explicitBackupAcknowledged == false)
    let protectedText = store.allData().map { String(decoding: $0, as: UTF8.self) }.joined()
    #expect(!protectedText.contains(testRecoverySecret))

    _ = try RecoveryExportHelper.payload(for: rotated.value)
    saved = try #require(try repository.loadBundleWithoutMigration())
    #expect(saved.recovery?.explicitBackupAcknowledged == false)
    try service.acknowledgeRecoveryBackup(actorID: 2, recoveryID: testRecoveryID)
    #expect(
      try repository.loadBundleWithoutMigration()?.recovery?.explicitBackupAcknowledged == true)
  }

  @Test func authenticatedHooksNeverSendControlBearerToWrongOrigin() async throws {
    let repository = CredentialRepository(
      store: MemoryProtectedStore(), files: MemoryCredentialFiles())
    try repository.saveBundle(
      CredentialBundle(
        coordinatorOrigin: try CoordinatorOrigin("https://coord.example.com"),
        node: NodeCapability(
          orbitId: 1, slot: "a", nodeToken: testNodeToken,
          wsUrl: "wss://coord.example.com/ws"),
        control: ControlCapability(
          actorId: 2, orbitId: 1, role: .primary, controlToken: testControlToken)))
    let transport = ScriptedTransport { _, _ in throw ServiceTransportError.unexpectedSend }
    let service = OnboardingService(
      client: try OnboardingHTTPClient(
        coordinator: "https://other.example.com", transport: transport),
      credentials: repository)

    await #expect(throws: OnboardingClientError.storage) {
      try await service.issueDeviceInvite()
    }
    await #expect(throws: OnboardingClientError.storage) {
      try await service.rotateRecovery()
    }
    await #expect(throws: OnboardingClientError.storage) {
      try await service.issueTelegramLink()
    }
    await #expect(throws: OnboardingClientError.storage) {
      try await service.probeControlContext()
    }
    await #expect(throws: OnboardingClientError.storage) {
      try await service.probeNodeContext()
    }
    #expect(transport.requests().isEmpty)
  }

  @Test func backupAcknowledgementRequiresExactServiceOriginAndMutatesOnlyFlag() throws {
    let store = MemoryProtectedStore()
    let repository = CredentialRepository(store: store, files: MemoryCredentialFiles())
    let storedOrigin = try CoordinatorOrigin("https://coord.example.com")
    let recoveryID = "rec_" + String(repeating: "e", count: 32)
    let bundle = CredentialBundle(
      coordinatorOrigin: storedOrigin,
      node: NodeCapability(
        orbitId: 1, slot: "a", nodeToken: testNodeToken,
        wsUrl: "wss://coord.example.com/ws"),
      control: ControlCapability(
        actorId: 2, orbitId: 1, role: .primary, controlToken: testControlToken),
      recovery: RecoveryMetadata(actorId: 2, recoveryId: recoveryID))
    try repository.saveBundle(bundle)
    let destinationKey = ProtectedStoreKey(
      service: CredentialRepository.service,
      account: CredentialRepository.destinationAccount,
      location: .dataProtection)
    let originalData = try #require(store.data(destinationKey))

    let wrongOriginService = OnboardingService(
      client: try OnboardingHTTPClient(
        coordinator: "https://other.example.com",
        transport: ScriptedTransport { _, _ in throw ServiceTransportError.unexpectedSend }),
      credentials: repository)
    #expect(throws: CredentialStorageError.conflict) {
      try wrongOriginService.acknowledgeRecoveryBackup(actorID: 2, recoveryID: recoveryID)
    }
    #expect(store.data(destinationKey) == originalData)

    let matchingService = OnboardingService(
      client: try OnboardingHTTPClient(
        coordinator: storedOrigin.rawValue,
        transport: ScriptedTransport { _, _ in throw ServiceTransportError.unexpectedSend }),
      credentials: repository)
    try matchingService.acknowledgeRecoveryBackup(actorID: 2, recoveryID: recoveryID)
    var expected = bundle
    expected.recovery?.explicitBackupAcknowledged = true
    #expect(store.data(destinationKey) == encodedBundle(expected))
  }

  @Test func backupAcknowledgementFailsClosedForMissingOrCorruptOriginState() throws {
    let origin = try CoordinatorOrigin("https://coord.example.com")
    let recoveryID = "rec_" + String(repeating: "e", count: 32)

    let emptyStore = MemoryProtectedStore()
    let emptyService = OnboardingService(
      client: try OnboardingHTTPClient(
        coordinator: origin.rawValue,
        transport: ScriptedTransport { _, _ in throw ServiceTransportError.unexpectedSend }),
      credentials: CredentialRepository(store: emptyStore, files: MemoryCredentialFiles()))
    #expect(throws: CredentialStorageError.conflict) {
      try emptyService.acknowledgeRecoveryBackup(actorID: 2, recoveryID: recoveryID)
    }
    #expect(emptyStore.allData().isEmpty)

    let corruptStore = MemoryProtectedStore()
    let corruptData = encodedBundle(
      CredentialBundle(
        coordinatorOrigin: nil,
        control: ControlCapability(
          actorId: 2, orbitId: 1, role: .primary, controlToken: testControlToken),
        recovery: RecoveryMetadata(actorId: 2, recoveryId: recoveryID)))
    let destinationKey = ProtectedStoreKey(
      service: CredentialRepository.service,
      account: CredentialRepository.destinationAccount,
      location: .dataProtection)
    corruptStore.seed(corruptData, key: destinationKey)
    let corruptService = OnboardingService(
      client: try OnboardingHTTPClient(
        coordinator: origin.rawValue,
        transport: ScriptedTransport { _, _ in throw ServiceTransportError.unexpectedSend }),
      credentials: CredentialRepository(store: corruptStore, files: MemoryCredentialFiles()))
    #expect(throws: CredentialStorageError.corrupt) {
      try corruptService.acknowledgeRecoveryBackup(actorID: 2, recoveryID: recoveryID)
    }
    #expect(corruptStore.data(destinationKey) == corruptData)
  }

  @Test func publicContextProbesUseOnlyOriginBoundStoredCapabilities() async throws {
    let repository = CredentialRepository(
      store: MemoryProtectedStore(), files: MemoryCredentialFiles())
    try repository.saveBundle(
      CredentialBundle(
        coordinatorOrigin: try CoordinatorOrigin("https://coord.example.com"),
        node: NodeCapability(
          orbitId: 1, slot: "a", nodeToken: testNodeToken,
          wsUrl: "wss://coord.example.com/ws"),
        control: ControlCapability(
          actorId: 2, orbitId: 1, role: .primary, controlToken: testControlToken)))
    let transport = ScriptedTransport { request, _ in
      testHTTPResponse(
        request: request, status: 200,
        json: #"{"orbit_id":1,"actor_id":2,"role":"primary"}"#)
    }
    let service = OnboardingService(
      client: try OnboardingHTTPClient(
        coordinator: "https://coord.example.com", transport: transport),
      credentials: repository)

    _ = try await service.probeControlContext()
    _ = try await service.probeNodeContext()
    #expect(
      transport.requests().map { $0.value(forHTTPHeaderField: "Authorization") } == [
        "Bearer \(testControlToken)", "Bearer \(testNodeToken)",
      ])
  }
}

private let createdJSON =
  #"{"orbit_id":1,"title":"Orbit","actor_id":2,"role":"primary","slot":"a","node_token":"\#(testNodeToken)","control_token":"\#(testControlToken)","recovery_id":"\#(testRecoveryID)","recovery_secret":"\#(testRecoverySecret)","shown_once":true}"#

private enum ServiceTransportError: Error { case unexpectedSend }
