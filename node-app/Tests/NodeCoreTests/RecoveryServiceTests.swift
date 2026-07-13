import Foundation
import Testing

@testable import NodeCore

@Suite(.serialized)
struct RecoveryServiceTests {
  private let origin = try! CoordinatorOrigin("https://coord.example.com")

  @Test func verifiedSendBarrierAndSuccessfulPromotionPreserveNodeBytes() async throws {
    let activeStore = MemoryProtectedStore()
    let pendingStore = MemoryProtectedStore()
    let credentials = CredentialRepository(store: activeStore, files: MemoryCredentialFiles())
    let original = initialBundle(actorID: 2)
    try credentials.saveBundle(original)
    let pendingRepository = PendingRecoveryRepository(store: pendingStore)
    let transport = ScriptedTransport { request, _ in
      let records = pendingStore.allData().compactMap {
        try? JSONDecoder().decode(PendingRecoveryRecord.self, from: $0)
      }
      #expect(records.count == 1)
      #expect(records.first?.everSent == true)
      let operations = pendingStore.operations()
      let update = operations.lastIndex { $0.kind == .update }
      let verifiedRead = operations.lastIndex { $0.kind == .read }
      #expect(update != nil && verifiedRead != nil && update! < verifiedRead!)
      return testHTTPResponse(request: request, status: 200, json: contextJSON(actorID: 2))
    }
    let service = try recoveryService(
      transport: transport, credentials: credentials, pending: pendingRepository,
      generator: FixedTokenGenerator(token: testPendingToken)
    )

    let outcome = try await service.recover(
      actorID: 2, recoveryID: testRecoveryID,
      secret: RecoverySecret(validated: testRecoverySecret)
    )
    guard case .recovered(let recovered) = outcome else {
      Issue.record("Expected recovered outcome")
      return
    }
    #expect(recovered.bundle.node == original.node)
    #expect(recovered.bundle.node?.nodeToken == testNodeToken)
    #expect(recovered.bundle.node?.wsUrl == original.node?.wsUrl)
    #expect(recovered.bundle.control?.controlToken == testPendingToken)
    #expect(recovered.bundle.control?.actorId == 2)
    #expect(recovered.bundle.control?.contextStrength == .active)
    #expect(!recovered.hasLimitedContext)
    #expect(try service.inspectPending(actorID: 2) == nil)
    #expect(transport.requests().count == 1)
  }

  @Test(arguments: [false, true])
  func zeroSendsWhenSentUpdateOrReadbackFails(failReadback: Bool) async throws {
    let activeStore = MemoryProtectedStore()
    let pendingStore = MemoryProtectedStore()
    if failReadback {
      pendingStore.failReadAfterEveryUpdate = true
    } else {
      pendingStore.failure = { kind, _, _ in
        kind == .update ? ProtectedStoreFailure.unavailable : nil
      }
    }
    let transport = ScriptedTransport { _, _ in throw RecoveryTransportError.unexpectedSend }
    let pendingRepository = PendingRecoveryRepository(store: pendingStore)
    let service = try recoveryService(
      transport: transport,
      credentials: CredentialRepository(store: activeStore, files: MemoryCredentialFiles()),
      pending: pendingRepository,
      generator: FixedTokenGenerator(token: testPendingToken)
    )
    await #expect(throws: OnboardingClientError.storage) {
      try await service.recover(
        actorID: failReadback ? 4 : 3, recoveryID: testRecoveryID,
        secret: RecoverySecret(validated: testRecoverySecret)
      )
    }
    #expect(transport.requests().isEmpty)
    let record = try pendingRepository.load(origin: origin, actorID: failReadback ? 4 : 3)
    #expect(record?.pendingControlToken == testPendingToken)
    #expect(record?.everSent == failReadback)
  }

  @Test func duplicatePendingDestinationsTransitionTogetherAndPartialFailureFailsClosed() throws {
    let store = MemoryProtectedStore()
    let repository = PendingRecoveryRepository(store: store)
    let record = PendingRecoveryRecord(
      canonicalCoordinatorOrigin: origin, actorId: 29,
      recoveryId: testRecoveryID, pendingControlToken: testPendingToken,
      everSent: false
    )
    try repository.createUnsent(record)
    let dp = try #require(store.operations().first { $0.kind == .add }?.key)
    let login = ProtectedStoreKey(
      service: dp.service, account: dp.account, location: .login
    )
    let falseData = try #require(store.data(dp))
    store.seed(falseData, key: login)
    let sent = try repository.markSent(record)
    #expect(try repository.load(origin: origin, actorID: 29) == sent)
    #expect(store.data(dp) == store.data(login))
    try repository.deleteExact(sent)
    #expect(store.data(dp) == nil)
    #expect(store.data(login) == nil)

    let partialStore = MemoryProtectedStore()
    let partialRepository = PendingRecoveryRepository(store: partialStore)
    try partialRepository.createUnsent(record)
    let partialDP = try #require(partialStore.operations().first { $0.kind == .add }?.key)
    let partialLogin = ProtectedStoreKey(
      service: partialDP.service, account: partialDP.account, location: .login)
    partialStore.seed(try #require(partialStore.data(partialDP)), key: partialLogin)
    partialStore.failure = { kind, key, _ in
      kind == .update && key.location == .login ? ProtectedStoreFailure.unavailable : nil
    }
    #expect(throws: CredentialStorageError.unavailable) {
      try partialRepository.markSent(record)
    }
    #expect(throws: CredentialStorageError.conflict) {
      try partialRepository.load(origin: origin, actorID: 29)
    }
    partialStore.failure = nil
    #expect(throws: CredentialStorageError.conflict) {
      try partialRepository.load(origin: origin, actorID: 29)
    }
    #expect(partialStore.data(partialDP) != partialStore.data(partialLogin))
  }

  @Test func partialDuplicateSentTransitionCannotReachTransportAndRemainsConflict() async throws {
    let actorID: Int64 = 30
    let pendingStore = MemoryProtectedStore()
    let pendingRepository = PendingRecoveryRepository(store: pendingStore)
    let record = PendingRecoveryRecord(
      canonicalCoordinatorOrigin: origin,
      actorId: actorID,
      recoveryId: testRecoveryID,
      pendingControlToken: testPendingToken,
      everSent: false)
    try pendingRepository.createUnsent(record)
    let dp = try #require(pendingStore.operations().first { $0.kind == .add }?.key)
    let login = ProtectedStoreKey(
      service: dp.service, account: dp.account, location: .login)
    pendingStore.seed(try #require(pendingStore.data(dp)), key: login)
    pendingStore.failure = { kind, key, _ in
      kind == .update && key.location == .login ? ProtectedStoreFailure.unavailable : nil
    }

    let transport = ScriptedTransport { request, _ in
      testHTTPResponse(request: request, status: 200, json: contextJSON(actorID: actorID))
    }
    let service = try recoveryService(
      transport: transport,
      credentials: CredentialRepository(
        store: MemoryProtectedStore(), files: MemoryCredentialFiles()),
      pending: pendingRepository,
      generator: FixedTokenGenerator(token: testPendingToken))

    await #expect(throws: OnboardingClientError.storage) {
      try await service.recover(
        actorID: actorID,
        recoveryID: testRecoveryID,
        secret: RecoverySecret(validated: testRecoverySecret))
    }
    #expect(transport.requests().isEmpty)

    pendingStore.failure = nil
    await #expect(throws: CredentialStorageError.conflict) {
      try await service.resumePending(actorID: actorID)
    }
    #expect(transport.requests().isEmpty)
    #expect(pendingStore.data(dp) != nil)
    #expect(pendingStore.data(login) != nil)
  }

  @Test(arguments: [false, true])
  func unsentMatchingActiveFailsClosedAtBothPublicEntryPoints(useRecover: Bool) async throws {
    let actorID: Int64 = useRecover ? 102 : 101
    let activeStore = MemoryProtectedStore()
    let pendingStore = MemoryProtectedStore()
    let credentials = CredentialRepository(store: activeStore, files: MemoryCredentialFiles())
    var active = initialBundle(actorID: actorID)
    active.control?.controlToken = testPendingToken
    try credentials.saveBundle(active)
    let pendingRepository = PendingRecoveryRepository(store: pendingStore)
    let unsent = PendingRecoveryRecord(
      canonicalCoordinatorOrigin: origin, actorId: actorID,
      recoveryId: testRecoveryID, pendingControlToken: testPendingToken, everSent: false)
    try pendingRepository.createUnsent(unsent)
    let activeBytes = Set(activeStore.allData())
    let pendingBytes = Set(pendingStore.allData())
    let transport = ScriptedTransport { _, _ in throw RecoveryTransportError.unexpectedSend }
    let service = try recoveryService(
      transport: transport, credentials: credentials, pending: pendingRepository,
      generator: FixedTokenGenerator(token: String(repeating: "e", count: 64)))

    if useRecover {
      await #expect(throws: CredentialStorageError.conflict) {
        try await service.recover(
          actorID: actorID, recoveryID: testRecoveryID,
          secret: RecoverySecret(validated: testRecoverySecret))
      }
    } else {
      await #expect(throws: CredentialStorageError.conflict) {
        try await service.resumePending(actorID: actorID)
      }
    }

    #expect(transport.requests().isEmpty)
    #expect(Set(activeStore.allData()) == activeBytes)
    #expect(Set(pendingStore.allData()) == pendingBytes)
  }

  @Test func exactPendingSchemaAndByteDifferentCopiesFailClosedWithoutMutation() throws {
    let record = PendingRecoveryRecord(
      canonicalCoordinatorOrigin: origin, actorId: 103,
      recoveryId: testRecoveryID, pendingControlToken: testPendingToken, everSent: false)
    let fixtureStore = MemoryProtectedStore()
    let fixtureRepository = PendingRecoveryRepository(store: fixtureStore)
    try fixtureRepository.createUnsent(record)
    let dp = try #require(fixtureStore.operations().first { $0.kind == .add }?.key)
    let canonical = try #require(fixtureStore.data(dp))
    let canonicalString = String(decoding: canonical, as: UTF8.self)
    let malformed = [
      String(canonicalString.dropLast()) + ",\"unknown\":true}",
      canonicalString.replacingOccurrences(
        of: "\"actor_id\":103", with: "\"actor_id\":103,\"actor_id\":103"),
      canonicalString + " false",
      canonicalString.replacingOccurrences(of: "\"actor_id\":103", with: "\"actor_id\":103.0"),
      canonicalString.replacingOccurrences(
        of: "https:\\/\\/coord.example.com", with: "https://coord.example.com"),
    ]

    for bytes in malformed.map({ Data($0.utf8) }) {
      let store = MemoryProtectedStore()
      store.seed(bytes, key: dp)
      let repository = PendingRecoveryRepository(store: store)
      #expect(throws: CredentialStorageError.corrupt) {
        try repository.load(origin: origin, actorID: record.actorId)
      }
      #expect(store.data(dp) == bytes)
      #expect(!store.operations().contains { $0.kind == .delete || $0.kind == .update })
    }

    let login = ProtectedStoreKey(
      service: dp.service, account: dp.account, location: .login)
    for bytes in malformed.map({ Data($0.utf8) }) {
      let store = MemoryProtectedStore()
      store.seed(canonical, key: dp)
      store.seed(bytes, key: login)
      let repository = PendingRecoveryRepository(store: store)
      #expect(throws: CredentialStorageError.conflict) {
        try repository.load(origin: origin, actorID: record.actorId)
      }
      #expect(store.data(dp) == canonical)
      #expect(store.data(login) == bytes)
      #expect(!store.operations().contains { $0.kind == .delete || $0.kind == .update })
    }

    let reordered = Data(
      ("{\"recovery_id\":\"" + testRecoveryID + "\",\"ever_sent\":false,\"actor_id\":103,"
        + "\"pending_control_token\":\"" + testPendingToken + "\","
        + "\"canonical_coordinator_origin\":\"https://coord.example.com\"}").utf8)
    for variant in [Data(" \n".utf8) + canonical, reordered] {
      let store = MemoryProtectedStore()
      store.seed(canonical, key: dp)
      store.seed(variant, key: login)
      let repository = PendingRecoveryRepository(store: store)
      #expect(throws: CredentialStorageError.conflict) {
        try repository.load(origin: origin, actorID: record.actorId)
      }
      #expect(store.data(dp) == canonical)
      #expect(store.data(login) == variant)
      #expect(!store.operations().contains { $0.kind == .delete || $0.kind == .update })
    }
  }

  @Test func consumeFailureMatrixRetainsTheSentCandidate() async throws {
    let scenarios: [(Int, RecoveryFailureScenario, RecoveryPendingReason)] = [
      (31, .invalid400, .ambiguousResponse),
      (32, .invalid403, .credentialRejected),
      (33, .rateLimited, .rateLimited(19)),
      (34, .serverFailure, .ambiguousResponse),
      (35, .network, .ambiguousResponse),
      (36, .cancelled, .ambiguousResponse),
      (37, .decoderAmbiguity, .ambiguousResponse),
    ]
    for (actorID, scenario, expected) in scenarios {
      let pendingStore = MemoryProtectedStore()
      let transport = ScriptedTransport { request, _ in
        switch scenario {
        case .invalid400:
          return testHTTPResponse(
            request: request, status: 400,
            json: apiError(
              code: "invalid_request",
              message: "The request is malformed or contains invalid parameters."
            ))
        case .invalid403:
          return testHTTPResponse(
            request: request, status: 403,
            json: apiError(
              code: "credential_invalid", message: "The provided credential is not valid."
            ))
        case .rateLimited:
          return testHTTPResponse(
            request: request, status: 429,
            json: apiError(
              code: "too_many_attempts",
              message: "Too many attempts. Please wait before retrying.", retry: 19
            ),
            headers: ["Retry-After": "19"]
          )
        case .serverFailure:
          return testHTTPResponse(
            request: request, status: 503,
            json: apiError(
              code: "internal_error", message: "An internal error occurred."
            ))
        case .network: throw URLError(.timedOut)
        case .cancelled: throw CancellationError()
        case .decoderAmbiguity:
          return testHTTPResponse(request: request, status: 200, json: "{}")
        }
      }
      let pendingRepository = PendingRecoveryRepository(store: pendingStore)
      let service = try recoveryService(
        transport: transport,
        credentials: CredentialRepository(
          store: MemoryProtectedStore(), files: MemoryCredentialFiles()),
        pending: pendingRepository,
        generator: FixedTokenGenerator(token: testPendingToken)
      )
      let outcome = try await service.recover(
        actorID: Int64(actorID), recoveryID: testRecoveryID,
        secret: RecoverySecret(validated: testRecoverySecret)
      )
      guard case .pendingRetained(let reason) = outcome else {
        Issue.record("Expected retained pending state for \(scenario)")
        continue
      }
      #expect(reason == expected)
      let retained = try pendingRepository.load(origin: origin, actorID: Int64(actorID))
      #expect(retained?.everSent == true)
      #expect(retained?.pendingControlToken == testPendingToken)
      #expect(transport.requests().count == 1)
    }
  }

  @Test func restartProbeMatrixAndExplicitDestructiveAbandon() async throws {
    try await assertRestartProbeActive(actorID: 41)
    try await assertRestartProbeLimited(actorID: 42)
    try await assertRestartProbeUnauthorizedThenSuccess(actorID: 43)

    let actorID: Int64 = 44
    let pendingStore = MemoryProtectedStore()
    let pendingRepository = PendingRecoveryRepository(store: pendingStore)
    try seedSentPending(pendingRepository, actorID: actorID)
    let transport = ScriptedTransport { request, _ in
      if request.url?.path == "/v1/actor/context" {
        return testHTTPResponse(
          request: request, status: 401,
          json: apiError(
            code: "unauthorized", message: "Authentication is required."
          ))
      }
      return testHTTPResponse(
        request: request, status: 403,
        json: apiError(
          code: "credential_invalid", message: "The provided credential is not valid."
        ))
    }
    let service = try recoveryService(
      transport: transport,
      credentials: CredentialRepository(
        store: MemoryProtectedStore(), files: MemoryCredentialFiles()),
      pending: pendingRepository,
      generator: FixedTokenGenerator(token: String(repeating: "e", count: 64))
    )
    let outcome = try await service.recover(
      actorID: actorID, recoveryID: testRecoveryID,
      secret: RecoverySecret(validated: testRecoverySecret)
    )
    guard case .destructiveAbandonAvailable = outcome else {
      Issue.record("Expected destructive abandon outcome")
      return
    }
    #expect(try service.inspectPending(actorID: actorID)?.pendingControlToken == testPendingToken)
    await #expect(throws: CredentialStorageError.conflict) {
      try await service.abandonPending(
        actorID: actorID,
        confirmation: DestructiveAbandonConfirmation(acknowledgedWarning: false)
      )
    }
    try await service.abandonPending(
      actorID: actorID,
      confirmation: DestructiveAbandonConfirmation(acknowledgedWarning: true)
    )
    #expect(try service.inspectPending(actorID: actorID) == nil)
  }

  @Test func promotionCrashRestartConvergesWithoutSecondRequest() async throws {
    let actorID: Int64 = 51
    let activeStore = MemoryProtectedStore()
    let pendingStore = MemoryProtectedStore()
    let credentials = CredentialRepository(store: activeStore, files: MemoryCredentialFiles())
    let pendingRepository = PendingRecoveryRepository(store: pendingStore)
    let firstTransport = ScriptedTransport { request, _ in
      testHTTPResponse(request: request, status: 200, json: contextJSON(actorID: actorID))
    }
    let first = try recoveryService(
      transport: firstTransport, credentials: credentials, pending: pendingRepository,
      generator: FixedTokenGenerator(token: testPendingToken)
    )
    pendingStore.failure = { kind, _, _ in
      kind == .delete ? ProtectedStoreFailure.unavailable : nil
    }
    await #expect(throws: OnboardingClientError.storage) {
      try await first.recover(
        actorID: actorID, recoveryID: testRecoveryID,
        secret: RecoverySecret(validated: testRecoverySecret)
      )
    }
    #expect(try credentials.loadBundleWithoutMigration()?.control?.controlToken == testPendingToken)
    #expect(try pendingRepository.load(origin: origin, actorID: actorID)?.everSent == true)

    pendingStore.failure = nil
    let restartTransport = ScriptedTransport { _, _ in throw RecoveryTransportError.unexpectedSend }
    let restart = try recoveryService(
      transport: restartTransport, credentials: credentials, pending: pendingRepository,
      generator: FixedTokenGenerator(token: String(repeating: "e", count: 64))
    )
    let outcome = try await restart.resumePending(actorID: actorID)
    guard case .recovered(let recovered) = outcome else {
      Issue.record("Expected crash convergence")
      return
    }
    #expect(!recovered.hasLimitedContext)
    #expect(recovered.bundle.control?.contextStrength == .active)
    #expect(restartTransport.requests().isEmpty)
    #expect(try restart.inspectPending(actorID: actorID) == nil)
  }

  @Test func limitedContextSurvivesPromotionDeleteFailureAndRestartWithoutNetwork() async throws {
    let actorID: Int64 = 52
    let activeStore = MemoryProtectedStore()
    let pendingStore = MemoryProtectedStore()
    let credentials = CredentialRepository(store: activeStore, files: MemoryCredentialFiles())
    let original = initialBundle(actorID: actorID)
    try credentials.saveBundle(original)
    let pendingRepository = PendingRecoveryRepository(store: pendingStore)
    try seedSentPending(pendingRepository, actorID: actorID)
    let limitedTransport = ScriptedTransport { request, _ in
      testHTTPResponse(
        request: request, status: 403,
        json: apiError(
          code: "insufficient_capability",
          message: "This token does not have the required capability."))
    }
    let first = try recoveryService(
      transport: limitedTransport, credentials: credentials, pending: pendingRepository,
      generator: FixedTokenGenerator(token: String(repeating: "e", count: 64)))
    pendingStore.failure = { kind, _, _ in
      kind == .delete ? ProtectedStoreFailure.unavailable : nil
    }

    await #expect(throws: OnboardingClientError.storage) {
      try await first.resumePending(actorID: actorID)
    }
    let loaded = try credentials.loadBundleWithoutMigration()
    let promoted = try #require(loaded)
    #expect(promoted.node == original.node)
    #expect(promoted.control?.orbitId == original.control?.orbitId)
    #expect(promoted.control?.role == original.control?.role)
    #expect(promoted.control?.contextStrength == .limited)
    #expect(promoted.control?.controlToken == testPendingToken)
    #expect(try pendingRepository.load(origin: origin, actorID: actorID)?.everSent == true)
    #expect(limitedTransport.requests().count == 1)

    pendingStore.failure = nil
    let noSend = ScriptedTransport { _, _ in throw RecoveryTransportError.unexpectedSend }
    let restart = try recoveryService(
      transport: noSend, credentials: credentials, pending: pendingRepository,
      generator: FixedTokenGenerator(token: String(repeating: "f", count: 64)))
    guard case .recovered(let recovered) = try await restart.resumePending(actorID: actorID) else {
      Issue.record("Expected limited crash convergence")
      return
    }
    #expect(recovered.hasLimitedContext)
    #expect(recovered.bundle.control?.contextStrength == .limited)
    #expect(recovered.bundle.node == original.node)
    #expect(noSend.requests().isEmpty)
    #expect(try restart.inspectPending(actorID: actorID) == nil)
  }

  @Test func resumePendingProbesWithoutSecretAndRequestsItOnlyWhenRequired() async throws {
    let activeActor: Int64 = 91
    let activePending = PendingRecoveryRepository(store: MemoryProtectedStore())
    try seedSentPending(activePending, actorID: activeActor)
    let activeTransport = ScriptedTransport { request, _ in
      testHTTPResponse(request: request, status: 200, json: contextJSON(actorID: activeActor))
    }
    let activeService = try recoveryService(
      transport: activeTransport,
      credentials: CredentialRepository(
        store: MemoryProtectedStore(), files: MemoryCredentialFiles()),
      pending: activePending,
      generator: FixedTokenGenerator(token: String(repeating: "e", count: 64)))
    guard case .recovered = try await activeService.resumePending(actorID: activeActor) else {
      Issue.record("Sent pending credential did not resume through a probe")
      return
    }
    #expect(activeTransport.requests().map { $0.url?.path } == ["/v1/actor/context"])

    let unauthorizedActor: Int64 = 92
    let unauthorizedPending = PendingRecoveryRepository(store: MemoryProtectedStore())
    try seedSentPending(unauthorizedPending, actorID: unauthorizedActor)
    let unauthorizedTransport = ScriptedTransport { request, _ in
      testHTTPResponse(
        request: request, status: 401,
        json: apiError(code: "unauthorized", message: "Authentication is required."))
    }
    let unauthorizedService = try recoveryService(
      transport: unauthorizedTransport,
      credentials: CredentialRepository(
        store: MemoryProtectedStore(), files: MemoryCredentialFiles()),
      pending: unauthorizedPending,
      generator: FixedTokenGenerator(token: String(repeating: "e", count: 64)))
    guard
      case .needsSecretForExistingGeneration(let recoveryID) =
        try await unauthorizedService.resumePending(actorID: unauthorizedActor)
    else {
      Issue.record("Definitive 401 did not request the existing recovery secret")
      return
    }
    #expect(recoveryID == testRecoveryID)
    #expect(unauthorizedTransport.requests().map { $0.url?.path } == ["/v1/actor/context"])
    #expect(try unauthorizedService.inspectPending(actorID: unauthorizedActor) != nil)

    let unsentActor: Int64 = 93
    let unsentPending = PendingRecoveryRepository(store: MemoryProtectedStore())
    let unsent = PendingRecoveryRecord(
      canonicalCoordinatorOrigin: origin, actorId: unsentActor,
      recoveryId: testRecoveryID, pendingControlToken: testPendingToken, everSent: false)
    try unsentPending.createUnsent(unsent)
    let noSend = ScriptedTransport { _, _ in throw RecoveryTransportError.unexpectedSend }
    let unsentService = try recoveryService(
      transport: noSend,
      credentials: CredentialRepository(
        store: MemoryProtectedStore(), files: MemoryCredentialFiles()),
      pending: unsentPending,
      generator: FixedTokenGenerator(token: String(repeating: "e", count: 64)))
    guard
      case .needsSecretForExistingGeneration =
        try await unsentService.resumePending(actorID: unsentActor)
    else {
      Issue.record("Unsent pending candidate did not request its secret")
      return
    }
    #expect(noSend.requests().isEmpty)

    let limitedActor: Int64 = 94
    let limitedPending = PendingRecoveryRepository(store: MemoryProtectedStore())
    try seedSentPending(limitedPending, actorID: limitedActor)
    let limitedTransport = ScriptedTransport { request, _ in
      testHTTPResponse(
        request: request, status: 403,
        json: apiError(
          code: "insufficient_capability",
          message: "This token does not have the required capability."))
    }
    let limitedService = try recoveryService(
      transport: limitedTransport,
      credentials: CredentialRepository(
        store: MemoryProtectedStore(), files: MemoryCredentialFiles()),
      pending: limitedPending,
      generator: FixedTokenGenerator(token: String(repeating: "e", count: 64)))
    guard
      case .recovered(let limited) = try await limitedService.resumePending(actorID: limitedActor)
    else {
      Issue.record("Authenticated limited pending credential was not promoted")
      return
    }
    #expect(limited.hasLimitedContext)

    for (actorID, response, expected) in [
      (Int64(95), "rate", RecoveryPendingReason.rateLimited(11)),
      (Int64(96), "network", RecoveryPendingReason.ambiguousResponse),
    ] {
      let pending = PendingRecoveryRepository(store: MemoryProtectedStore())
      try seedSentPending(pending, actorID: actorID)
      let transport = ScriptedTransport { request, _ in
        if response == "network" { throw URLError(.timedOut) }
        return testHTTPResponse(
          request: request, status: 429,
          json: apiError(
            code: "too_many_attempts",
            message: "Too many attempts. Please wait before retrying.", retry: 11),
          headers: ["Retry-After": "11"])
      }
      let service = try recoveryService(
        transport: transport,
        credentials: CredentialRepository(
          store: MemoryProtectedStore(), files: MemoryCredentialFiles()),
        pending: pending,
        generator: FixedTokenGenerator(token: String(repeating: "e", count: 64)))
      guard case .pendingRetained(let reason) = try await service.resumePending(actorID: actorID)
      else {
        Issue.record("Ambiguous resume did not retain pending state")
        continue
      }
      #expect(reason == expected)
      #expect(try service.inspectPending(actorID: actorID) != nil)
    }

    await #expect(throws: OnboardingClientError.invalidRequest) {
      try await activeService.resumePending(actorID: 999)
    }
  }

  @Test func concurrentPairSaveCannotBeLostByControlPromotion() async throws {
    let actorID: Int64 = 56
    let activeStore = MemoryProtectedStore()
    let credentialsA = CredentialRepository(store: activeStore, files: MemoryCredentialFiles())
    let credentialsB = CredentialRepository(store: activeStore, files: MemoryCredentialFiles())
    try credentialsA.saveBundle(initialBundle(actorID: actorID))
    let pendingRepository = PendingRecoveryRepository(store: MemoryProtectedStore())
    let barrier = RequestBarrier()
    let transport = ScriptedTransport { request, _ in
      await barrier.arriveAndWait()
      return testHTTPResponse(
        request: request, status: 200, json: contextJSON(actorID: actorID))
    }
    let service = try recoveryService(
      transport: transport, credentials: credentialsA, pending: pendingRepository,
      generator: FixedTokenGenerator(token: testPendingToken))
    let recovery = Task {
      try await service.recover(
        actorID: actorID, recoveryID: testRecoveryID,
        secret: RecoverySecret(validated: testRecoverySecret))
    }
    await barrier.waitForArrivals(1)
    let newlyPaired = NodeCredentials(
      orbitId: 1, slot: "b", token: String(repeating: "f", count: 64),
      wsUrl: "wss://coord.example.com/ws")
    try credentialsB.saveNode(newlyPaired)
    await barrier.releaseAll()
    guard case .recovered(let result) = try await recovery.value else {
      Issue.record("Expected recovery promotion")
      return
    }
    #expect(result.bundle.nodeCredentials == newlyPaired)
    #expect(result.bundle.control?.controlToken == testPendingToken)
  }

  @Test func mismatchedRecoveryContextAndCorruptPromotionMetadataRetainPending() async throws {
    let actorID: Int64 = 57
    let pendingStore = MemoryProtectedStore()
    let pendingRepository = PendingRecoveryRepository(store: pendingStore)
    let transport = ScriptedTransport { request, _ in
      testHTTPResponse(request: request, status: 200, json: contextJSON(actorID: 999))
    }
    let credentials = CredentialRepository(
      store: MemoryProtectedStore(), files: MemoryCredentialFiles())
    let service = try recoveryService(
      transport: transport, credentials: credentials, pending: pendingRepository,
      generator: FixedTokenGenerator(token: testPendingToken))
    let outcome = try await service.recover(
      actorID: actorID, recoveryID: testRecoveryID,
      secret: RecoverySecret(validated: testRecoverySecret))
    guard case .pendingRetained(.ambiguousResponse) = outcome else {
      Issue.record("Mismatched actor context was not retained as ambiguous")
      return
    }
    #expect(try pendingRepository.load(origin: origin, actorID: actorID)?.everSent == true)
    #expect(try credentials.loadBundleWithoutMigration() == nil)

    let crashActor: Int64 = 58
    let crashPending = PendingRecoveryRepository(store: MemoryProtectedStore())
    try seedSentPending(crashPending, actorID: crashActor)
    let crashCredentials = CredentialRepository(
      store: MemoryProtectedStore(), files: MemoryCredentialFiles())
    try crashCredentials.saveBundle(
      CredentialBundle(
        coordinatorOrigin: origin,
        control: ControlCapability(
          actorId: crashActor, orbitId: 1, role: .primary,
          controlToken: testPendingToken),
        recovery: RecoveryMetadata(
          actorId: crashActor, recoveryId: "rec_" + String(repeating: "e", count: 32))))
    let noSend = ScriptedTransport { _, _ in throw RecoveryTransportError.unexpectedSend }
    let restart = try recoveryService(
      transport: noSend, credentials: crashCredentials, pending: crashPending,
      generator: FixedTokenGenerator(token: String(repeating: "e", count: 64)))
    await #expect(throws: CredentialStorageError.conflict) {
      try await restart.recover(
        actorID: crashActor, recoveryID: testRecoveryID,
        secret: RecoverySecret(validated: testRecoverySecret))
    }
    #expect(noSend.requests().isEmpty)
    #expect(try crashPending.load(origin: origin, actorID: crashActor) != nil)
  }

  @Test func concurrentServicesShareOneCandidateAndDifferentScopesProceedIndependently()
    async throws
  {
    let actorID: Int64 = 61
    let pendingStore = MemoryProtectedStore()
    let pendingRepository = PendingRecoveryRepository(store: pendingStore)
    let barrier = RequestBarrier()
    let transport = ScriptedTransport { request, _ in
      if request.url?.path == "/v1/recovery/consume" {
        await barrier.arriveAndWait()
        throw RecoveryTransportError.network
      }
      return testHTTPResponse(request: request, status: 200, json: contextJSON(actorID: actorID))
    }
    let credentials = CredentialRepository(
      store: MemoryProtectedStore(), files: MemoryCredentialFiles())
    let first = try recoveryService(
      transport: transport, credentials: credentials, pending: pendingRepository,
      generator: FixedTokenGenerator(token: testPendingToken)
    )
    let secondToken = String(repeating: "e", count: 64)
    let second = try recoveryService(
      transport: transport, credentials: credentials, pending: pendingRepository,
      generator: FixedTokenGenerator(token: secondToken)
    )
    let firstTask = Task {
      try await first.recover(
        actorID: actorID, recoveryID: testRecoveryID,
        secret: RecoverySecret(validated: testRecoverySecret)
      )
    }
    await barrier.waitForArrivals(1)
    let secondTask = Task {
      try await second.recover(
        actorID: actorID, recoveryID: testRecoveryID,
        secret: RecoverySecret(validated: testRecoverySecret)
      )
    }
    await barrier.releaseAll()
    _ = try await firstTask.value
    let secondOutcome = try await secondTask.value
    guard case .recovered = secondOutcome else {
      Issue.record("Expected probe convergence")
      return
    }
    let requests = transport.requests()
    #expect(requests.filter { $0.url?.path == "/v1/recovery/consume" }.count == 1)
    #expect(requests.filter { $0.url?.path == "/v1/actor/context" }.count == 1)
    let bodies = requests.compactMap(\.httpBody).map { String(decoding: $0, as: UTF8.self) }
    #expect(bodies.contains { $0.contains(testPendingToken) })
    #expect(!bodies.contains { $0.contains(secondToken) })

    let parallelBarrier = RequestBarrier()
    let parallelTransport = ScriptedTransport { request, _ in
      await parallelBarrier.arriveAndWait()
      let body = request.httpBody.flatMap { data -> [String: Any]? in
        (try? JSONSerialization.jsonObject(with: data)) as? [String: Any]
      }
      let recoveryID = body?["recovery_id"] as? String
      let responseActor: Int64 = recoveryID == testRecoveryID ? 71 : 72
      return testHTTPResponse(
        request: request, status: 200, json: contextJSON(actorID: responseActor))
    }
    let otherRecoveryID = "rec_" + String(repeating: "e", count: 32)
    let serviceA = try recoveryService(
      transport: parallelTransport,
      credentials: CredentialRepository(
        store: MemoryProtectedStore(), files: MemoryCredentialFiles()),
      pending: PendingRecoveryRepository(store: pendingStore),
      generator: FixedTokenGenerator(token: String(repeating: "1", count: 64))
    )
    let serviceB = try recoveryService(
      transport: parallelTransport,
      credentials: CredentialRepository(
        store: MemoryProtectedStore(), files: MemoryCredentialFiles()),
      pending: PendingRecoveryRepository(store: pendingStore),
      generator: FixedTokenGenerator(token: String(repeating: "2", count: 64))
    )
    let a = Task {
      try await serviceA.recover(
        actorID: 71, recoveryID: testRecoveryID,
        secret: RecoverySecret(validated: testRecoverySecret)
      )
    }
    let b = Task {
      try await serviceB.recover(
        actorID: 72, recoveryID: otherRecoveryID,
        secret: RecoverySecret(validated: testRecoverySecret)
      )
    }
    await parallelBarrier.waitForArrivals(2)
    await parallelBarrier.releaseAll()
    _ = try await (a.value, b.value)
    #expect(parallelTransport.requests().count == 2)
  }

  @Test func queuedCancellationHandlerBeforeReleaseDoesNotAcquireOrDelayRelease() async throws {
    let serializer = RecoveryScopeSerializer()
    let scope = RecoveryScope(origin: origin, actorID: 81)
    let firstID = UUID()
    let cancelledID = UUID()
    let nextID = UUID()
    try await serializer.acquire(scope, id: firstID)

    let cancelled = Task {
      try await withTaskCancellationHandler {
        try await serializer.acquire(scope, id: cancelledID)
      } onCancel: {
        Task { await serializer.cancel(scope, id: cancelledID) }
      }
    }
    await serializer.waitUntilQueued(scope, id: cancelledID)

    let next = Task { try await serializer.acquire(scope, id: nextID) }
    await serializer.waitUntilQueued(scope, id: nextID)
    cancelled.cancel()
    await #expect(throws: CancellationError.self) { try await cancelled.value }

    await serializer.release(scope, id: firstID)
    try await next.value
    await serializer.release(scope, id: nextID)

    let proofID = UUID()
    try await serializer.acquire(scope, id: proofID)
    await serializer.release(scope, id: proofID)
  }

  @Test func cancellationBeforeEnqueueLeavesNoOwnerOrTombstone() async throws {
    let serializer = RecoveryScopeSerializer()
    let scope = RecoveryScope(origin: origin, actorID: 82)
    let gate = RequestBarrier()
    let cancelledID = UUID()
    let cancelled = Task {
      await gate.arriveAndWait()
      try await serializer.acquire(scope, id: cancelledID)
    }
    await gate.waitForArrivals(1)
    cancelled.cancel()
    await gate.releaseAll()
    await #expect(throws: CancellationError.self) { try await cancelled.value }

    let proofID = UUID()
    try await serializer.acquire(scope, id: proofID)
    await serializer.release(scope, id: proofID)
  }

  @Test func queuedCancellationReleaseBeforeHandlerSkipsSideEffectsAndAdvancesQueue()
    async throws
  {
    let serializer = RecoveryScopeSerializer()
    let scope = RecoveryScope(origin: origin, actorID: 83)
    let firstID = UUID()
    let cancelledID = UUID()
    let nextID = UUID()
    try await serializer.acquire(scope, id: firstID)

    // This task intentionally has no onCancel callback. Owner release must
    // promote it first; acquire's post-resume cancellation check must then
    // release the scope before any code after acquire can execute.
    let sideEffects = TestCounter()
    let cancelled = Task {
      try await serializer.acquire(scope, id: cancelledID)
      await sideEffects.increment()
      await serializer.release(scope, id: cancelledID)
    }
    await serializer.waitUntilQueued(scope, id: cancelledID)
    cancelled.cancel()

    let next = Task { try await serializer.acquire(scope, id: nextID) }
    await serializer.waitUntilQueued(scope, id: nextID)
    await serializer.release(scope, id: firstID)

    await #expect(throws: CancellationError.self) { try await cancelled.value }
    #expect(await sideEffects.value == 0)
    try await next.value
    await serializer.release(scope, id: nextID)
  }

  private func assertRestartProbeActive(actorID: Int64) async throws {
    let pendingRepository = PendingRecoveryRepository(store: MemoryProtectedStore())
    try seedSentPending(pendingRepository, actorID: actorID)
    let transport = ScriptedTransport { request, _ in
      testHTTPResponse(request: request, status: 200, json: contextJSON(actorID: actorID))
    }
    let service = try recoveryService(
      transport: transport,
      credentials: CredentialRepository(
        store: MemoryProtectedStore(), files: MemoryCredentialFiles()),
      pending: pendingRepository,
      generator: FixedTokenGenerator(token: String(repeating: "e", count: 64))
    )
    let result = try await service.recover(
      actorID: actorID, recoveryID: testRecoveryID,
      secret: RecoverySecret(validated: testRecoverySecret)
    )
    guard case .recovered(let recovered) = result else {
      Issue.record("Expected active probe")
      return
    }
    #expect(!recovered.hasLimitedContext)
    #expect(transport.requests().map { $0.url?.path } == ["/v1/actor/context"])
  }

  private func assertRestartProbeLimited(actorID: Int64) async throws {
    let pendingRepository = PendingRecoveryRepository(store: MemoryProtectedStore())
    try seedSentPending(pendingRepository, actorID: actorID)
    let activeStore = MemoryProtectedStore()
    let credentials = CredentialRepository(store: activeStore, files: MemoryCredentialFiles())
    let original = initialBundle(actorID: actorID)
    try credentials.saveBundle(original)
    let transport = ScriptedTransport { request, _ in
      testHTTPResponse(
        request: request, status: 403,
        json: apiError(
          code: "insufficient_capability",
          message: "This token does not have the required capability."
        ))
    }
    let service = try recoveryService(
      transport: transport, credentials: credentials, pending: pendingRepository,
      generator: FixedTokenGenerator(token: String(repeating: "e", count: 64))
    )
    let result = try await service.recover(
      actorID: actorID, recoveryID: testRecoveryID,
      secret: RecoverySecret(validated: testRecoverySecret)
    )
    guard case .recovered(let recovered) = result else {
      Issue.record("Expected limited probe")
      return
    }
    #expect(recovered.hasLimitedContext)
    #expect(recovered.bundle.control?.contextStrength == .limited)
    #expect(recovered.bundle.control?.orbitId == original.control?.orbitId)
    #expect(recovered.bundle.control?.role == original.control?.role)
    #expect(recovered.bundle.node == original.node)
  }

  private func assertRestartProbeUnauthorizedThenSuccess(actorID: Int64) async throws {
    let pendingRepository = PendingRecoveryRepository(store: MemoryProtectedStore())
    try seedSentPending(pendingRepository, actorID: actorID)
    let transport = ScriptedTransport { request, _ in
      if request.url?.path == "/v1/actor/context" {
        return testHTTPResponse(
          request: request, status: 401,
          json: apiError(
            code: "unauthorized", message: "Authentication is required."
          ))
      }
      return testHTTPResponse(request: request, status: 200, json: contextJSON(actorID: actorID))
    }
    let service = try recoveryService(
      transport: transport,
      credentials: CredentialRepository(
        store: MemoryProtectedStore(), files: MemoryCredentialFiles()),
      pending: pendingRepository,
      generator: FixedTokenGenerator(token: String(repeating: "e", count: 64))
    )
    let result = try await service.recover(
      actorID: actorID, recoveryID: testRecoveryID,
      secret: RecoverySecret(validated: testRecoverySecret)
    )
    guard case .recovered = result else {
      Issue.record("Expected same-token retry")
      return
    }
    #expect(
      transport.requests().map { $0.url?.path } == [
        "/v1/actor/context", "/v1/recovery/consume",
      ])
    let replacementBody = String(decoding: transport.requests()[1].httpBody!, as: UTF8.self)
    #expect(replacementBody.contains(testPendingToken))
  }

  private func recoveryService(
    transport: ScriptedTransport,
    credentials: CredentialRepository,
    pending: PendingRecoveryRepository,
    generator: FixedTokenGenerator
  ) throws -> RecoveryService {
    RecoveryService(
      client: try OnboardingHTTPClient(coordinator: origin.rawValue, transport: transport),
      credentials: credentials, pending: pending, generator: generator
    )
  }

  private func seedSentPending(_ repository: PendingRecoveryRepository, actorID: Int64) throws {
    let record = PendingRecoveryRecord(
      canonicalCoordinatorOrigin: origin, actorId: actorID,
      recoveryId: testRecoveryID, pendingControlToken: testPendingToken, everSent: false
    )
    try repository.createUnsent(record)
    #expect(try repository.markSent(record) == record.sent)
  }

  private func initialBundle(actorID: Int64) -> CredentialBundle {
    CredentialBundle(
      coordinatorOrigin: origin,
      node: NodeCapability(
        orbitId: 1, slot: "a", nodeToken: testNodeToken,
        wsUrl: "wss://coord.example.com/ws"
      ),
      control: ControlCapability(
        actorId: actorID, orbitId: 1, role: .primary, controlToken: testControlToken
      ),
      recovery: RecoveryMetadata(actorId: actorID, recoveryId: testRecoveryID)
    )
  }
}

private enum RecoveryFailureScenario: Sendable {
  case invalid400, invalid403, rateLimited, serverFailure, network, cancelled, decoderAmbiguity
}

private enum RecoveryTransportError: Error { case network, unexpectedSend }

private func contextJSON(actorID: Int64) -> String {
  #"{"orbit_id":1,"actor_id":\#(actorID),"role":"primary"}"#
}

private func apiError(code: String, message: String, retry: Int? = nil) -> String {
  let retryValue = retry.map(String.init) ?? "null"
  return
    #"{"error":{"code":"\#(code)","message":"\#(message)","retry_after_seconds":\#(retryValue)}}"#
}
