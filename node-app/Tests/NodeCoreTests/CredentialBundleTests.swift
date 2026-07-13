import Foundation
import Testing

@testable import NodeCore

@Suite(.serialized)
struct CredentialBundleTests {
  private let origin = try! CoordinatorOrigin("https://coord.example.com")

  @Test func versionedRoundTripAndSplitCapabilities() throws {
    let bundle = CredentialBundle(
      coordinatorOrigin: origin,
      node: NodeCapability(
        orbitId: 4, slot: "b", nodeToken: testNodeToken, wsUrl: "wss://coord.example.com/ws"),
      control: ControlCapability(
        actorId: 9, orbitId: 4, role: .companion, controlToken: testControlToken),
      recovery: RecoveryMetadata(actorId: 9, recoveryId: testRecoveryID)
    )
    let roundTrip = try JSONDecoder().decode(CredentialBundle.self, from: encodedBundle(bundle))
    #expect(roundTrip == bundle)
    #expect(roundTrip.version == 2)
    #expect(roundTrip.control?.contextStrength == .active)
    #expect(roundTrip.nodeCredentials?.token == testNodeToken)

    let controlOnly = CredentialBundle(
      coordinatorOrigin: origin, control: bundle.control, recovery: bundle.recovery)
    #expect(controlOnly.nodeCredentials == nil)
    #expect(controlOnly.control?.controlToken == testControlToken)

    let secret = try RecoverySecret(validated: testRecoverySecret)
    #expect(!(secret is any Encodable))
    #expect(!(secret is any CustomStringConvertible))
    #expect(!String(describing: secret).contains(testRecoverySecret))
    #expect(!String(reflecting: secret).contains(testRecoverySecret))
    #expect(!String(reflecting: bundle).contains(testNodeToken))
    #expect(!String(reflecting: bundle).contains(testControlToken))
  }

  @Test func recoveryAndHumanCodeNormalizationIsASCIIOnlyAndCanonical() throws {
    let grouped = "abcd-efgh-jkmn-pqrs-tvwx-yz23-456"
    let secret = try RecoverySecret(validated: grouped)
    #expect(secret.reveal() == testRecoverySecret)
    #expect(CredentialSyntax.humanCode(grouped) == testRecoverySecret)
    #expect(CredentialSyntax.canonicalHumanCode(testRecoverySecret))
    #expect(!CredentialSyntax.canonicalHumanCode(grouped))
    for invalid in [
      "ßBCDEFGHJKMNPQRSTVWXYZ23456",
      "ＡBCDEFGHJKMNPQRSTVWXYZ23456",
      "ABCD\u{00A0}EFGHJKMNPQRSTVWXYZ23456",
      "ABCD\u{200D}EFGHJKMNPQRSTVWXYZ23456",
    ] {
      #expect(throws: OnboardingClientError.invalidRequest) {
        try RecoverySecret(validated: invalid)
      }
      #expect(CredentialSyntax.humanCode(invalid) == nil)
    }
    let material = OneTimeRecoveryMaterial(
      actorId: 9, recoveryId: testRecoveryID, secret: secret)
    #expect(try RecoveryExportHelper.payload(for: material).contains(testRecoverySecret))
    #expect(!(try RecoveryExportHelper.payload(for: material)).contains("-"))
  }

  @Test func publicCredentialResultsRedactOrdinaryAndDebugDescriptions() throws {
    let node = NodeCredentials(
      orbitId: 4, slot: "z", token: testNodeToken,
      wsUrl: "wss://coord.example.com/ws")
    let bundle = CredentialBundle(
      coordinatorOrigin: origin,
      node: NodeCapability(
        orbitId: 4, slot: "z", nodeToken: testNodeToken,
        wsUrl: "wss://coord.example.com/ws"),
      control: ControlCapability(
        actorId: 9, orbitId: 4, role: .primary, controlToken: testControlToken),
      recovery: RecoveryMetadata(actorId: 9, recoveryId: testRecoveryID))
    let material = OneTimeRecoveryMaterial(
      actorId: 9, recoveryId: testRecoveryID,
      secret: try RecoverySecret(validated: testRecoverySecret))
    let created = CreatedOrbit(title: "Orbit", bundle: bundle, recovery: material)
    let joined = JoinedOrbit(title: "Orbit", bundle: bundle)
    let recovered = RecoveryServiceOutcome.recovered(
      RecoveredCredential(bundle: bundle, hasLimitedContext: false))
    let persistence = ProtectedPersistenceOutcome<CreatedOrbit>.stored(created)

    for value in [
      String(describing: node), String(reflecting: node),
      String(reflecting: Result<NodeCredentials, PairingError>.success(node)),
      String(describing: created), String(reflecting: created),
      String(describing: joined), String(reflecting: joined),
      String(describing: recovered), String(reflecting: recovered),
      String(describing: persistence), String(reflecting: persistence),
    ] {
      #expect(!value.contains(testNodeToken))
      #expect(!value.contains(testControlToken))
      #expect(!value.contains(testRecoverySecret))
      #expect(!value.contains("wss://coord.example.com/ws"))
      #expect(!value.contains("slot: z"))
    }
  }

  @Test func maliciousInitializerValuesNeverReachCrashFriendlyDescriptions() throws {
    let canary = "TASK_260712_SECRET_CANARY_R3"
    let node = NodeCapability(
      orbitId: 4, slot: canary, nodeToken: canary, wsUrl: "wss://example.invalid/\(canary)")
    let control = ControlCapability(
      actorId: 9, orbitId: nil, role: nil, controlToken: canary, contextStrength: .limited)
    let pending = PendingRecoveryRecord(
      canonicalCoordinatorOrigin: origin, actorId: 9, recoveryId: canary,
      pendingControlToken: canary, everSent: false)
    let telegram = TelegramLinkCode(
      code: canary, desiredRole: .companion, expiresAt: .distantFuture, botUsername: canary)

    for value in [
      String(describing: node), String(reflecting: node),
      String(describing: control), String(reflecting: control),
      String(describing: pending), String(reflecting: pending),
      String(describing: telegram), String(reflecting: telegram),
    ] {
      #expect(!value.contains(canary))
      #expect(!value.contains("wss://"))
    }
  }

  @Test func canonicalV1DestinationsMigrateToDistinctV2WithDurableContextStrength() throws {
    let cases: [(Int64, Int64?, CredentialRole?, ControlContextStrength)] = [
      (Int64(81), Int64(4), CredentialRole.companion, ControlContextStrength.active),
      (Int64(82), nil, nil, ControlContextStrength.limited),
    ]
    for (actorID, orbitID, role, expectedStrength) in cases {
      let store = MemoryProtectedStore()
      let source = ProtectedStoreKey(
        service: CredentialRepository.service,
        account: CredentialRepository.previousDestinationAccount,
        location: .dataProtection)
      let previous = encodedPreviousBundle(
        coordinatorOrigin: origin, node: nil, actorID: actorID, orbitID: orbitID,
        role: role, controlToken: testControlToken,
        recovery: RecoveryMetadata(actorId: actorID, recoveryId: testRecoveryID))
      store.seed(previous, key: source)
      let repository = CredentialRepository(store: store, files: MemoryCredentialFiles())

      let loaded = try repository.loadBundleWithoutMigration()
      let migrated = try #require(loaded)
      #expect(migrated.version == CredentialBundle.currentVersion)
      #expect(migrated.control?.contextStrength == expectedStrength)
      #expect(migrated.control?.controlToken == testControlToken)
      #expect(store.data(source) == nil)

      let operations = store.operations()
      let destinationAdd = operations.firstIndex {
        $0.kind == .add && $0.key.account == CredentialRepository.destinationAccount
      }
      let sourceDelete = operations.firstIndex { $0.kind == .delete && $0.key == source }
      let verifiedRead = operations.indices.first { index in
        operations[index].kind == .read
          && operations[index].key.account == CredentialRepository.destinationAccount
          && destinationAdd.map({ $0 < index }) == true
      }
      #expect(
        destinationAdd != nil && verifiedRead != nil && sourceDelete != nil
          && destinationAdd! < verifiedRead! && verifiedRead! < sourceDelete!)
      #expect(
        CredentialRepository.previousDestinationAccount != CredentialRepository.destinationAccount)
    }
  }

  @Test func exactProtectedBundleSchemaRejectsNoncanonicalPayloadsWithoutMutation() throws {
    let bundle = CredentialBundle(
      node: NodeCapability(
        orbitId: 4, slot: "b", nodeToken: testNodeToken,
        wsUrl: "wss://coord.example.com/ws"))
    let canonical = String(decoding: encodedBundle(bundle), as: UTF8.self)
    let malformed = [
      String(canonical.dropLast()) + ",\"unknown\":true}",
      canonical.replacingOccurrences(
        of: "\"version\":2", with: "\"version\":2,\"version\":2"),
      canonical + " true",
      canonical.replacingOccurrences(of: "\"version\":2", with: "\"version\":2.0"),
      canonical.replacingOccurrences(of: "wss:\\/\\/", with: "wss://"),
    ]

    for (index, value) in malformed.enumerated() {
      let store = MemoryProtectedStore()
      let key = ProtectedStoreKey(
        service: CredentialRepository.service, account: CredentialRepository.destinationAccount,
        location: .dataProtection)
      let bytes = Data(value.utf8)
      store.seed(bytes, key: key)
      let repository = CredentialRepository(store: store, files: MemoryCredentialFiles())
      #expect(throws: CredentialStorageError.corrupt, "variant \(index)") {
        try repository.loadBundleWithoutMigration()
      }
      #expect(store.data(key) == bytes)
      #expect(!store.operations().contains { $0.kind == .delete || $0.kind == .update })
    }

    let dp = ProtectedStoreKey(
      service: CredentialRepository.service, account: CredentialRepository.destinationAccount,
      location: .dataProtection)
    let login = ProtectedStoreKey(
      service: dp.service, account: dp.account, location: .login)
    for value in malformed {
      let store = MemoryProtectedStore()
      let divergent = Data(value.utf8)
      store.seed(Data(canonical.utf8), key: dp)
      store.seed(divergent, key: login)
      let repository = CredentialRepository(store: store, files: MemoryCredentialFiles())
      #expect(throws: CredentialStorageError.conflict) {
        try repository.loadBundleWithoutMigration()
      }
      #expect(store.data(dp) == Data(canonical.utf8))
      #expect(store.data(login) == divergent)
      #expect(!store.operations().contains { $0.kind == .delete || $0.kind == .update })
    }
  }

  @Test func previousVersionMigrationSourcesAlsoRequireCanonicalByteEquivalentPayloads() throws {
    let canonical = encodedPreviousBundle(
      coordinatorOrigin: origin, node: nil, actorID: 83, orbitID: 4,
      role: .companion, controlToken: testControlToken,
      recovery: RecoveryMetadata(actorId: 83, recoveryId: testRecoveryID))
    let malformed = Data(
      (String(decoding: canonical.dropLast(), as: UTF8.self) + ",\"unknown\":true}").utf8)
    let dp = ProtectedStoreKey(
      service: CredentialRepository.service,
      account: CredentialRepository.previousDestinationAccount,
      location: .dataProtection)

    let malformedStore = MemoryProtectedStore()
    malformedStore.seed(malformed, key: dp)
    let malformedRepository = CredentialRepository(
      store: malformedStore, files: MemoryCredentialFiles())
    #expect(throws: CredentialStorageError.corrupt) {
      try malformedRepository.loadBundleWithoutMigration()
    }
    #expect(malformedStore.data(dp) == malformed)
    #expect(
      !malformedStore.operations().contains { $0.kind == .delete || $0.kind == .update })

    let differingStore = MemoryProtectedStore()
    let login = ProtectedStoreKey(service: dp.service, account: dp.account, location: .login)
    let spaced = Data(" \n".utf8) + canonical
    differingStore.seed(canonical, key: dp)
    differingStore.seed(spaced, key: login)
    let differingRepository = CredentialRepository(
      store: differingStore, files: MemoryCredentialFiles())
    #expect(throws: CredentialStorageError.conflict) {
      try differingRepository.loadBundleWithoutMigration()
    }
    #expect(differingStore.data(dp) == canonical)
    #expect(differingStore.data(login) == spaced)
    #expect(!differingStore.operations().contains { $0.kind == .delete || $0.kind == .update })
  }

  @Test func byteDifferentDestinationCopiesFailClosedEvenWhenModelsMatch() throws {
    let bundle = CredentialBundle(
      node: NodeCapability(
        orbitId: 4, slot: "b", nodeToken: testNodeToken,
        wsUrl: "wss://coord.example.com/ws"))
    let canonical = encodedBundle(bundle)
    let reordered = Data(
      ("{\"version\":2,\"node\":{\"orbit_id\":4,\"slot\":\"b\",\"node_token\":\""
        + testNodeToken + "\",\"ws_url\":\"wss://coord.example.com/ws\"}}").utf8)
    for variant in [Data(" \n".utf8) + canonical, reordered] {
      let store = MemoryProtectedStore()
      let dp = ProtectedStoreKey(
        service: CredentialRepository.service, account: CredentialRepository.destinationAccount,
        location: .dataProtection)
      let login = ProtectedStoreKey(
        service: CredentialRepository.service, account: CredentialRepository.destinationAccount,
        location: .login)
      store.seed(canonical, key: dp)
      store.seed(variant, key: login)
      let repository = CredentialRepository(store: store, files: MemoryCredentialFiles())
      #expect(throws: CredentialStorageError.conflict) {
        try repository.loadBundleWithoutMigration()
      }
      #expect(store.data(dp) == canonical)
      #expect(store.data(login) == variant)
      #expect(!store.operations().contains { $0.kind == .delete || $0.kind == .update })
    }
  }

  @Test(arguments: [ProtectedStoreLocation.dataProtection, .login])
  func migratesLegacyKeychainReadbackBeforeDelete(location: ProtectedStoreLocation) throws {
    let store = MemoryProtectedStore()
    let files = MemoryCredentialFiles()
    let legacy = NodeCredentials(
      orbitId: 7, slot: "c", token: testNodeToken, wsUrl: "wss://old.example/ws")
    let source = ProtectedStoreKey(
      service: CredentialRepository.service,
      account: CredentialRepository.legacyAccount,
      location: location
    )
    store.seed(encodedLegacy(legacy), key: source)
    if location == .login {
      store.failure = { kind, key, _ in
        kind == .add && key.location == .dataProtection
          ? ProtectedStoreFailure.missingEntitlement : nil
      }
    }
    let repository = CredentialRepository(store: store, files: files)
    let loaded = try repository.loadBundle(besideConfig: "/unused/node.yml")
    #expect(loaded?.nodeCredentials == legacy)
    #expect(store.data(source) == nil)

    let operations = store.operations()
    let destinationAccount = CredentialRepository.destinationAccount
    let verifyIndex = operations.lastIndex {
      $0.kind == .read && $0.key.account == destinationAccount
    }
    let deleteIndex = operations.firstIndex {
      $0.kind == .delete && $0.key == source
    }
    #expect(verifyIndex != nil && deleteIndex != nil && verifyIndex! < deleteIndex!)
    #expect(operations.contains { $0.kind == .add && $0.key.account == destinationAccount })
    #expect(source.account != destinationAccount)
  }

  @Test func migratesLegacyFileOnlyAfterVerifiedDestination() throws {
    let store = MemoryProtectedStore()
    let legacy = NodeCredentials(
      orbitId: 8, slot: "d", token: testNodeToken, wsUrl: "ws://127.0.0.1/ws")
    let files = MemoryCredentialFiles(data: encodedLegacy(legacy))
    let repository = CredentialRepository(store: store, files: files)
    #expect(
      try repository.loadBundle(besideConfig: "/tmp/config/node.yml")?.nodeCredentials == legacy)
    #expect(files.events == ["read", "delete"])
    #expect(files.data == nil)
    #expect(
      store.operations().contains {
        $0.kind == .read && $0.key.account == CredentialRepository.destinationAccount
      })
  }

  @Test func everyMigrationFailureLeavesSourceAndRestartIsIdempotent() throws {
    let legacy = NodeCredentials(
      orbitId: 11, slot: "a", token: testNodeToken, wsUrl: "wss://coord/ws")
    for failureKind in [MemoryProtectedStore.Kind.add, .read] {
      let store = MemoryProtectedStore()
      let source = ProtectedStoreKey(
        service: CredentialRepository.service,
        account: CredentialRepository.legacyAccount,
        location: .login
      )
      store.seed(encodedLegacy(legacy), key: source)
      store.failure = { kind, key, count in
        if failureKind == .add, kind == .add { return ProtectedStoreFailure.unavailable }
        if failureKind == .read, kind == .read,
          key.account == CredentialRepository.destinationAccount, count >= 3
        {
          return ProtectedStoreFailure.unavailable
        }
        return nil
      }
      let repository = CredentialRepository(store: store, files: MemoryCredentialFiles())
      #expect(throws: Error.self) { try repository.loadBundle(besideConfig: "/tmp/node.yml") }
      #expect(store.data(source) != nil)

      store.failure = nil
      #expect(try repository.loadBundle(besideConfig: "/tmp/node.yml")?.nodeCredentials == legacy)
      #expect(store.data(source) == nil)
      #expect(try repository.loadBundle(besideConfig: "/tmp/node.yml")?.nodeCredentials == legacy)
    }
  }

  @Test func conflictsFailClosedAndPreserveEveryCopy() throws {
    let store = MemoryProtectedStore()
    let first = NodeCredentials(orbitId: 1, slot: "a", token: testNodeToken, wsUrl: "wss://one/ws")
    let second = NodeCredentials(
      orbitId: 1, slot: "a", token: String(repeating: "f", count: 64), wsUrl: "wss://one/ws")
    let dp = ProtectedStoreKey(
      service: CredentialRepository.service, account: CredentialRepository.legacyAccount,
      location: .dataProtection)
    let login = ProtectedStoreKey(
      service: CredentialRepository.service, account: CredentialRepository.legacyAccount,
      location: .login)
    store.seed(encodedLegacy(first), key: dp)
    store.seed(encodedLegacy(second), key: login)
    let repository = CredentialRepository(store: store, files: MemoryCredentialFiles())
    #expect(throws: CredentialStorageError.conflict) {
      try repository.loadBundle(besideConfig: "/tmp/node.yml")
    }
    #expect(store.data(dp) != nil)
    #expect(store.data(login) != nil)
    #expect(!store.operations().contains { $0.kind == .delete })
  }

  @Test func migrationUpdateAndDeleteFailuresRemainRestartSafe() throws {
    let legacy = NodeCredentials(
      orbitId: 12, slot: "c", token: testNodeToken,
      wsUrl: "wss://coord.example.com/ws"
    )
    let store = MemoryProtectedStore()
    let repository = CredentialRepository(store: store, files: MemoryCredentialFiles())
    let controlOnly = CredentialBundle(
      coordinatorOrigin: origin,
      control: ControlCapability(
        actorId: 7, orbitId: 12, role: .primary, controlToken: testControlToken
      ),
      recovery: RecoveryMetadata(actorId: 7, recoveryId: testRecoveryID)
    )
    try repository.saveBundle(controlOnly)
    let source = ProtectedStoreKey(
      service: CredentialRepository.service,
      account: CredentialRepository.legacyAccount,
      location: .login
    )
    store.seed(encodedLegacy(legacy), key: source)
    store.failure = { kind, _, _ in
      kind == .update ? ProtectedStoreFailure.unavailable : nil
    }
    #expect(throws: CredentialStorageError.unavailable) {
      try repository.loadBundle(besideConfig: "/tmp/node.yml")
    }
    #expect(store.data(source) != nil)
    #expect(try repository.loadBundleWithoutMigration() == controlOnly)

    store.failure = { kind, key, _ in
      kind == .delete && key == source ? ProtectedStoreFailure.unavailable : nil
    }
    #expect(throws: Error.self) {
      try repository.loadBundle(besideConfig: "/tmp/node.yml")
    }
    #expect(store.data(source) != nil)
    let merged = try repository.loadBundleWithoutMigration()
    #expect(merged?.nodeCredentials == legacy)
    #expect(merged?.control == controlOnly.control)

    store.failure = nil
    #expect(try repository.loadBundle(besideConfig: "/tmp/node.yml") == merged)
    #expect(store.data(source) == nil)
  }

  @Test func corruptedDestinationAndControlOnlyNodeViewFailClosed() throws {
    let store = MemoryProtectedStore()
    let destination = ProtectedStoreKey(
      service: CredentialRepository.service,
      account: CredentialRepository.destinationAccount,
      location: .dataProtection
    )
    let forged =
      #"{"version":1,"coordinator_origin":"https://user:secret@coord.example.com/path","control":{"actor_id":7,"orbit_id":1,"role":"primary","control_token":"\#(testControlToken)"}}"#
    store.seed(Data(forged.utf8), key: destination)
    let repository = CredentialRepository(store: store, files: MemoryCredentialFiles())
    #expect(throws: CredentialStorageError.corrupt) {
      try repository.loadBundleWithoutMigration()
    }
    #expect(store.data(destination) != nil)

    let cleanStore = MemoryProtectedStore()
    let cleanRepository = CredentialRepository(store: cleanStore, files: MemoryCredentialFiles())
    let controlOnly = CredentialBundle(
      coordinatorOrigin: origin,
      control: ControlCapability(
        actorId: 8, orbitId: nil, role: nil, controlToken: testControlToken,
        contextStrength: .limited
      )
    )
    try cleanRepository.saveBundle(controlOnly)
    #expect(try cleanRepository.loadBundleWithoutMigration()?.nodeCredentials == nil)
  }

  @Test func pairingSavePreservesControlAndUpdateFailurePreservesPriorItem() throws {
    let store = MemoryProtectedStore()
    let repository = CredentialRepository(store: store, files: MemoryCredentialFiles())
    let initial = CredentialBundle(
      coordinatorOrigin: origin,
      control: ControlCapability(
        actorId: 5, orbitId: 2, role: .primary, controlToken: testControlToken),
      recovery: RecoveryMetadata(actorId: 5, recoveryId: testRecoveryID)
    )
    try repository.saveBundle(initial)
    let paired = NodeCredentials(
      orbitId: 2, slot: "z", token: testNodeToken, wsUrl: "wss://coord.example.com/ws")
    try repository.saveNode(paired)
    let merged = try repository.loadBundleWithoutMigration()
    #expect(merged?.nodeCredentials == paired)
    #expect(merged?.control == initial.control)

    let destination = store.operations().first { $0.kind == .add }!.key
    let priorData = store.data(destination)
    store.failure = { kind, _, _ in kind == .update ? ProtectedStoreFailure.unavailable : nil }
    var changed = merged!
    changed.control?.controlToken = String(repeating: "e", count: 64)
    #expect(throws: CredentialStorageError.unavailable) { try repository.saveBundle(changed) }
    #expect(store.data(destination) == priorData)
    #expect(!store.operations().suffix(2).contains { $0.kind == .delete })
  }

  @Test func duplicateDestinationKeepsPriorGoodCopyUntilUpdateVerification() throws {
    let store = MemoryProtectedStore()
    let repository = CredentialRepository(store: store, files: MemoryCredentialFiles())
    let original = CredentialBundle(
      coordinatorOrigin: origin,
      node: NodeCapability(
        orbitId: 2, slot: "a", nodeToken: testNodeToken,
        wsUrl: "wss://coord.example.com/ws"),
      control: ControlCapability(
        actorId: 5, orbitId: 2, role: .primary, controlToken: testControlToken)
    )
    try repository.saveBundle(original)
    let dp = try #require(store.operations().first { $0.kind == .add }?.key)
    let login = ProtectedStoreKey(
      service: dp.service, account: dp.account, location: .login)
    let priorData = try #require(store.data(dp))
    store.seed(priorData, key: login)
    store.updateTransform = { data, key in
      key.location == .dataProtection ? Data("{".utf8) : data
    }
    var changed = original
    changed.control?.controlToken = String(repeating: "e", count: 64)
    #expect(throws: Error.self) { try repository.saveBundle(changed) }
    #expect(store.data(login) == priorData)
    #expect(!store.operations().contains { $0.kind == .delete && $0.key == login })
  }

  @Test func roleOrbitAndOriginRelationshipsFailClosed() throws {
    let repository = CredentialRepository(
      store: MemoryProtectedStore(), files: MemoryCredentialFiles())
    let roleWithoutOrbit = CredentialBundle(
      coordinatorOrigin: origin,
      control: ControlCapability(
        actorId: 4, orbitId: nil, role: .primary, controlToken: testControlToken)
    )
    #expect(throws: CredentialStorageError.corrupt) {
      try repository.saveBundle(roleWithoutOrbit)
    }
    let activeWithoutContext = CredentialBundle(
      coordinatorOrigin: origin,
      control: ControlCapability(
        actorId: 4, orbitId: nil, role: nil, controlToken: testControlToken)
    )
    #expect(throws: CredentialStorageError.corrupt) {
      try repository.saveBundle(activeWithoutContext)
    }
    let mismatchedOrbit = CredentialBundle(
      coordinatorOrigin: origin,
      node: NodeCapability(
        orbitId: 2, slot: "a", nodeToken: testNodeToken,
        wsUrl: "wss://coord.example.com/ws"),
      control: ControlCapability(
        actorId: 4, orbitId: 3, role: .primary, controlToken: testControlToken)
    )
    #expect(throws: CredentialStorageError.corrupt) {
      try repository.saveBundle(mismatchedOrbit)
    }
  }
}
