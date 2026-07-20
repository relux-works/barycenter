import CryptoKit
import Foundation
import Testing

@testable import NodeCore

private struct LivePTTVectors: Decodable {
  struct OpaqueFrame: Decodable {
    let flags: UInt8
    let sessionID: String
    let epoch: UInt64
    let generation: UInt64
    let sequence: UInt32
    let captureMonotonicUS: UInt64
    let targetSnapshotDigest: String
    let ciphertextHex: String
    let encodedHex: String
  }
  let opaqueFrame: OpaqueFrame
}

private func loadLivePTTVectors() throws -> LivePTTVectors {
  let url = URL(fileURLWithPath: #filePath)
    .deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
    .appendingPathComponent("../protocol/macos-e2ee-live-ptt-v1-vectors.json")
    .standardizedFileURL
  return try JSONDecoder().decode(LivePTTVectors.self, from: Data(contentsOf: url))
}

private func liveHex(_ value: String) -> Data {
  var result = Data()
  var index = value.startIndex
  while index < value.endIndex {
    let next = value.index(index, offsetBy: 2)
    result.append(UInt8(value[index..<next], radix: 16)!)
    index = next
  }
  return result
}

private final class LiveAuthorizationBox: MacE2EELiveAuthorizationChecking,
  @unchecked Sendable
{
  private let lock = NSLock()
  private var value: MacE2EELiveAuthorizationSnapshot

  init(_ value: MacE2EELiveAuthorizationSnapshot) { self.value = value }

  func currentAuthorization() -> MacE2EELiveAuthorizationSnapshot {
    lock.withLock { value }
  }

  func update(_ value: MacE2EELiveAuthorizationSnapshot) {
    lock.withLock { self.value = value }
  }
}

private final class LiveFixtureCrypto: MacE2EELiveCryptographicSession,
  @unchecked Sendable
{
  enum Failure: Error { case invalid }
  let productionApproved = false
  private let lock = NSLock()
  private let key = Data("AUDIT_FIXTURE_KEY_NOT_FOR_PRODUCTION".utf8)
  private(set) var sealCount = 0
  private(set) var openCount = 0
  private(set) var destroyCount = 0
  var reuseNonce = false
  var malformedCiphertext = false

  func seal(
    plaintext: Data, sequence: UInt32, authenticatedData: Data
  ) throws -> MacE2EELiveSealedPayload {
    lock.withLock {
      sealCount += 1
      let nonce = nonce(sequence: reuseNonce ? 1 : sequence)
      let stream = digest(key + authenticatedData + nonce)
      let encrypted = xor(plaintext, stream: stream)
      let tag = digest(key + authenticatedData + nonce + encrypted).prefix(16)
      return .init(
        nonceToken: nonce,
        wireCiphertext: malformedCiphertext ? Data() : nonce + encrypted + Data(tag))
    }
  }

  func open(
    wireCiphertext: Data, sequence: UInt32, authenticatedData: Data
  ) throws -> MacE2EELiveOpenedPayload {
    try lock.withLock {
      openCount += 1
      guard wireCiphertext.count > 20 else { throw Failure.invalid }
      let nonce = Data(wireCiphertext.prefix(4))
      let encrypted = Data(wireCiphertext.dropFirst(4).dropLast(16))
      let supplied = Data(wireCiphertext.suffix(16))
      let expected = Data(digest(key + authenticatedData + nonce + encrypted).prefix(16))
      guard supplied == expected else { throw Failure.invalid }
      return .init(
        nonceToken: nonce,
        plaintext: xor(
          encrypted,
          stream: digest(
            key + authenticatedData + nonce)))
    }
  }

  func destroy() { lock.withLock { destroyCount += 1 } }

  private func nonce(sequence: UInt32) -> Data {
    var value = sequence.bigEndian
    return withUnsafeBytes(of: &value) { Data($0) }
  }

  private func digest(_ value: Data) -> Data { Data(SHA256.hash(data: value)) }

  private func xor(_ value: Data, stream: Data) -> Data {
    Data(value.enumerated().map { index, byte in byte ^ stream[index % stream.count] })
  }
}

private final class LiveWireAttempts: @unchecked Sendable {
  private let lock = NSLock()
  private var values: [Data] = []

  func append(_ value: Data) -> Bool {
    lock.withLock {
      values.append(value)
      return values.count > 1
    }
  }

  func snapshot() -> [Data] { lock.withLock { values } }
}

private final class LiveMemoryKeychain: MacE2EEKeychainByteStore, @unchecked Sendable {
  private let lock = NSLock()
  private var values: [String: Data] = [:]

  func read(account: String) throws -> Data? { lock.withLock { values[account] } }
  func add(_ data: Data, account: String) throws {
    try lock.withLock {
      guard values[account] == nil else { throw MacE2EEKeyStateFailure.conflict }
      values[account] = data
    }
  }
  func update(_ data: Data, account: String) throws {
    try lock.withLock {
      guard values[account] != nil else { throw MacE2EEKeyStateFailure.unavailable }
      values[account] = data
    }
  }
  func delete(account: String) throws { lock.withLock { _ = values.removeValue(forKey: account) } }
}

private struct LiveFixedRandom: MacE2EERandomSource {
  func bytes(count: Int) throws -> Data { Data(repeating: 0x51, count: count) }
}

private final class LiveFixtureDerivation: MacE2EELiveSessionDeriving,
  @unchecked Sendable
{
  let productionApproved = false
  private let lock = NSLock()
  private(set) var contexts: [MacE2EELiveSessionContext] = []

  func derive(
    context: MacE2EELiveSessionContext,
    identity: MacE2EEDeviceIdentityLease,
    groupState: MacE2EEGroupStateLease
  ) throws -> any MacE2EELiveCryptographicSession {
    #expect(identity.metadata.deviceID.hasPrefix("mac-device"))
    #expect(groupState.metadata.epoch == context.epoch)
    lock.withLock { contexts.append(context) }
    return LiveFixtureCrypto()
  }
}

private final class LiveReceiverSpy: MacLiveJitterReceiving, @unchecked Sendable {
  private let lock = NSLock()
  private(set) var frames: [LivePTTBinaryFrame] = []
  private(set) var revocations: [String] = []

  func start(_ payload: LivePTTStartPayload, authorized: Bool) -> Bool { authorized }
  func receive(_ frame: LivePTTBinaryFrame) -> LivePTTFrameDecision {
    lock.withLock { frames.append(frame) }
    return .apply
  }
  func end(_ payload: LivePTTEndPayload) {}
  func cancel(_ payload: LivePTTCancelPayload) {}
  func revoke(reason: String) { lock.withLock { revocations.append(reason) } }
  func snapshot() -> MacLiveJitterSnapshot {
    .init(
      phase: .idle, sessionId: nil, generation: nil, expectedSequence: 0,
      highestSequence: 0, encodedFrames: 0, encodedBytes: 0, pcmFrames: 0,
      pcmCapacityFrames: 4_800, receivedFrames: frames.count, decodedFrames: 0,
      duplicateFrames: 0, lateFrames: 0, fecFrames: 0, plcFrames: 0,
      failedFrames: 0, underrunCallbacks: 0)
  }
}

private func liveStart(
  generation: Int64 = 7, target: String = String(repeating: "a", count: 64),
  playbackDomainID: Int64 = 44
) -> LivePTTStartPayload {
  .init(
    sessionId: "00112233445566778899aabbccddeeff", generation: generation,
    senderActorId: 10, senderOrbitId: 20, senderNodeId: "mac-node-1",
    targetSnapshot: "lts1.fixture", targetSha256: target, targetCount: 2,
    playbackDomain: "air", playbackDomainId: playbackDomainID,
    codecProfile: LivePTTConstants.codecProfile, frameMs: 20,
    maxPayloadBytes: 400, jitterBufferMs: 60,
    startedAtCoordMs: 10_000, acceptDeadlineCoordMs: 11_000,
    maxDurationMs: 300_000, mixedVersionPolicy: "require_all",
    lateJoinPolicy: LivePTTConstants.lateJoinPolicy,
    captureAuthority: LivePTTConstants.captureAuthority)
}

private func liveContext(start: LivePTTStartPayload = liveStart()) throws
  -> MacE2EELiveSessionContext
{
  try .init(
    groupID: "air-group-fixture-00000000000001",
    authorDeviceID: "mac-device-fixture-0001", epoch: 9,
    commitDigest: String(repeating: "b", count: 64), start: start)
}

private func liveAuthorization(
  context: MacE2EELiveSessionContext,
  senders: Set<String>? = nil,
  epoch: UInt64? = nil,
  commitDigest: String? = nil
) -> MacE2EELiveAuthorizationSnapshot {
  .init(
    groupID: context.groupID, epoch: epoch ?? context.epoch,
    commitDigest: commitDigest ?? context.commitDigest,
    targetSnapshotDigest: context.targetSnapshotDigest,
    authorizedSenderDeviceIDs: senders ?? [context.authorDeviceID])
}

private func liveFrame(
  _ sequence: UInt32, payload: Data = Data("opus-plaintext".utf8),
  end: Bool = false
) -> LivePTTBinaryFrame {
  var flags = LivePTTBinaryFrame.fecFlag
  if sequence == 1 { flags |= LivePTTBinaryFrame.startFlag }
  if end { flags |= LivePTTBinaryFrame.endFlag }
  return .init(
    flags: flags,
    sessionId: [
      0x00, 0x11, 0x22, 0x33, 0x44, 0x55, 0x66, 0x77,
      0x88, 0x99, 0xaa, 0xbb, 0xcc, 0xdd, 0xee, 0xff,
    ], sequence: sequence,
    captureMonotonicUs: 1_000_000 + UInt64(sequence - 1) * 20_000,
    payload: payload)
}

@Suite("macOS E2EE live PTT production-dark bridge")
struct MacE2EELivePTTTests {
  @Test("BE frame matches the accepted opaque router wire contract")
  func wireContract() throws {
    let vector = try loadLivePTTVectors().opaqueFrame
    let frame = MacE2EEOpaqueLiveFrame(
      flags: vector.flags, sessionID: [UInt8](liveHex(vector.sessionID)),
      epoch: vector.epoch, generation: vector.generation,
      sequence: vector.sequence, captureMonotonicUS: vector.captureMonotonicUS,
      targetSnapshotDigest: vector.targetSnapshotDigest,
      ciphertext: liveHex(vector.ciphertextHex))
    let wire = try frame.encoded()

    #expect(wire.count == 88)
    #expect(wire == liveHex(vector.encodedHex))
    #expect(Array(wire.prefix(4)) == [0x42, 0x45, 1, 1])
    #expect(Array(wire[20..<28]) == [0, 0, 0, 0, 0, 0, 0, 9])
    #expect(Array(wire[28..<36]) == [0, 0, 0, 0, 0, 0, 0, 7])
    #expect(Array(wire[80..<84]) == [0, 4, 0, 0])
    #expect(try MacE2EEOpaqueLiveFrame.decode(wire) == frame)
  }

  @Test("sender retry reuses ciphertext and receiver authenticates before jitter")
  func retryAndAuthenticationBarrier() throws {
    let context = try liveContext()
    let authorization = LiveAuthorizationBox(liveAuthorization(context: context))
    let senderCrypto = LiveFixtureCrypto()
    let receiverCrypto = LiveFixtureCrypto()
    let senderChannel = try MacE2EELiveFrameChannel(
      auditFixtureContext: context, crypto: senderCrypto, authorization: authorization)
    let receiverChannel = try MacE2EELiveFrameChannel(
      auditFixtureContext: context, crypto: receiverCrypto, authorization: authorization)
    let transport = LiveWireAttempts()
    let sender = MacE2EELiveSenderBridge(channel: senderChannel) { wire in
      transport.append(wire)
    }
    let receiverSpy = LiveReceiverSpy()
    let receiver = MacE2EELiveReceiverBridge(
      channel: receiverChannel, receiver: receiverSpy)
    let plaintext = Data("recognizable-opus-plaintext".utf8)
    let frame = liveFrame(1, payload: plaintext)

    #expect(sender.trySend(frame) == false)
    #expect(sender.trySend(frame) == true)
    let attempts = transport.snapshot()
    #expect(senderCrypto.sealCount == 1)
    #expect(attempts.count == 2)
    #expect(attempts[0] == attempts[1])
    #expect(attempts[0].range(of: plaintext) == nil)
    #expect(receiver.receiveOpaque(attempts[1]) == .apply)
    #expect(receiverSpy.frames == [frame])
    #expect(receiverCrypto.openCount == 1)
  }

  @Test("tamper never reaches Opus jitter decoder and terminates the session")
  func tamperFailsClosed() throws {
    let context = try liveContext()
    let authorization = LiveAuthorizationBox(liveAuthorization(context: context))
    let senderChannel = try MacE2EELiveFrameChannel(
      auditFixtureContext: context, crypto: LiveFixtureCrypto(),
      authorization: authorization)
    let receiverChannel = try MacE2EELiveFrameChannel(
      auditFixtureContext: context, crypto: LiveFixtureCrypto(),
      authorization: authorization)
    let opaque = try senderChannel.protect(liveFrame(1))
    var wire = [UInt8](try opaque.encoded())
    wire[wire.count - 1] ^= 0x01
    let spy = LiveReceiverSpy()
    let bridge = MacE2EELiveReceiverBridge(channel: receiverChannel, receiver: spy)

    #expect(bridge.receiveOpaque(Data(wire)) == .invalid)
    #expect(spy.frames.isEmpty)
    #expect(spy.revocations == ["e2ee_authentication_failed"])
    #expect(receiverChannel.isTerminal())
  }

  @Test("replay and nonce reuse are rejected before jitter")
  func replayAndNonceReuse() throws {
    let context = try liveContext()
    let authorization = LiveAuthorizationBox(liveAuthorization(context: context))
    let senderCrypto = LiveFixtureCrypto()
    let senderChannel = try MacE2EELiveFrameChannel(
      auditFixtureContext: context, crypto: senderCrypto, authorization: authorization)
    let receiverChannel = try MacE2EELiveFrameChannel(
      auditFixtureContext: context, crypto: LiveFixtureCrypto(),
      authorization: authorization)
    let first = try senderChannel.protect(liveFrame(1))
    let spy = LiveReceiverSpy()
    let bridge = MacE2EELiveReceiverBridge(channel: receiverChannel, receiver: spy)
    let wire = try first.encoded()
    #expect(bridge.receiveOpaque(wire) == .apply)
    #expect(bridge.receiveOpaque(wire) == .duplicate)
    #expect(spy.frames.count == 1)
    #expect(spy.revocations == ["e2ee_replay"])

    let reuseCrypto = LiveFixtureCrypto()
    reuseCrypto.reuseNonce = true
    let reuseChannel = try MacE2EELiveFrameChannel(
      auditFixtureContext: context, crypto: reuseCrypto, authorization: authorization)
    _ = try reuseChannel.protect(liveFrame(1))
    #expect(throws: MacE2EELiveFailure.nonceReuse) {
      try reuseChannel.protect(liveFrame(2))
    }
    #expect(reuseChannel.isTerminal())
  }

  @Test("malformed provider output is distinct and duration bound precedes sealing")
  func providerOutputAndDurationBounds() throws {
    let context = try liveContext()
    let authorization = LiveAuthorizationBox(liveAuthorization(context: context))
    let malformedCrypto = LiveFixtureCrypto()
    malformedCrypto.malformedCiphertext = true
    let malformedChannel = try MacE2EELiveFrameChannel(
      auditFixtureContext: context, crypto: malformedCrypto,
      authorization: authorization)
    #expect(throws: MacE2EELiveFailure.malformedProviderOutput) {
      try malformedChannel.protect(liveFrame(1))
    }
    #expect(malformedChannel.isTerminal())

    let boundedCrypto = LiveFixtureCrypto()
    let boundedChannel = try MacE2EELiveFrameChannel(
      auditFixtureContext: context, crypto: boundedCrypto,
      authorization: authorization)
    for sequence in UInt32(1)...15_000 {
      _ = try boundedChannel.protect(liveFrame(sequence, payload: Data([0x01])))
    }
    #expect(boundedCrypto.sealCount == 15_000)
    #expect(throws: MacE2EELiveFailure.invalidFrame) {
      try boundedChannel.protect(liveFrame(15_001, payload: Data([0x01])))
    }
    #expect(boundedCrypto.sealCount == 15_000)
    #expect(!boundedChannel.isTerminal())
  }

  @Test("verified membership or epoch change terminates exactly")
  func membershipChangeTerminates() throws {
    let context = try liveContext()
    let authorization = LiveAuthorizationBox(liveAuthorization(context: context))
    let sender = try MacE2EELiveFrameChannel(
      auditFixtureContext: context, crypto: LiveFixtureCrypto(),
      authorization: authorization)
    _ = try sender.protect(liveFrame(1))
    authorization.update(
      liveAuthorization(
        context: context, senders: [], epoch: context.epoch + 1,
        commitDigest: String(repeating: "c", count: 64)))

    #expect(throws: MacE2EELiveFailure.staleEpoch) {
      try sender.protect(liveFrame(2))
    }
    #expect(sender.isTerminal())
    #expect(throws: MacE2EELiveFailure.terminal) {
      try sender.protect(liveFrame(2))
    }
  }

  @Test("AAD binds Air target sender epoch generation sequence codec and timing")
  func aadBinding() throws {
    let first = try liveContext()
    let changedAir = try liveContext(start: liveStart(playbackDomainID: 45))
    let aad = try MacE2EELiveFrameChannel.authenticatedData(
      context: first, flags: 1, sequence: 1, captureMonotonicUS: 1_000_000)
    let airAAD = try MacE2EELiveFrameChannel.authenticatedData(
      context: changedAir, flags: 1, sequence: 1, captureMonotonicUS: 1_000_000)
    let sequenceAAD = try MacE2EELiveFrameChannel.authenticatedData(
      context: first, flags: 0, sequence: 2, captureMonotonicUS: 1_020_000)

    #expect(aad != airAAD)
    #expect(aad != sequenceAAD)
    #expect(aad.range(of: Data(first.groupID.utf8)) != nil)
    #expect(aad.range(of: Data(first.authorDeviceID.utf8)) != nil)
    #expect(aad.range(of: Data(first.targetSnapshotDigest.utf8)) != nil)
    #expect(aad.range(of: Data(first.codecProfile.utf8)) != nil)
  }

  @Test("unreviewed provider cannot cross the production factory")
  func productionProviderGate() throws {
    let context = try liveContext()
    let authorization = LiveAuthorizationBox(liveAuthorization(context: context))
    #expect(throws: MacE2EELiveFailure.providerNotApproved) {
      try MacE2EELiveSessionFactory(
        keyState: MacE2EEKeyStateRepository(
          store: LiveMemoryKeychain(), random: LiveFixedRandom()),
        derivation: LiveFixtureDerivation(), authorization: authorization,
        crossProcessGenerationSerializationApproved: true)
    }
  }

  @Test("factory reserves live generation and derives from witnessed epoch exactly once")
  func witnessedEpochDerivation() throws {
    let target = String(repeating: "a", count: 64)
    let keyState = MacE2EEKeyStateRepository(
      store: LiveMemoryKeychain(), random: LiveFixedRandom())
    let identity = try keyState.installDeviceIdentity(
      deviceID: "mac-device-fixture-0001", keyFormat: "fixture-v1",
      signingPrivateKey: Data(repeating: 0x11, count: 32),
      keyAgreementPrivateKey: Data(repeating: 0x22, count: 32),
      createdAtMS: 1_000)
    let group = try keyState.persistGroupState(
      installationID: identity.installationID,
      groupID: "air-group-fixture-00000000000001", epoch: 9,
      previousCommitDigest: "", commitDigest: String(repeating: "b", count: 64),
      targetSnapshotDigest: target, opaqueState: Data(repeating: 0x33, count: 64),
      expectedRevision: 0, nowMS: 2_000)
    let expectedContext = try liveContext()
    let authorization = LiveAuthorizationBox(
      .init(
        groupID: group.groupID, epoch: group.epoch, commitDigest: group.commitDigest,
        targetSnapshotDigest: target,
        authorizedSenderDeviceIDs: [identity.deviceID]))
    let derivation = LiveFixtureDerivation()
    let factory = try MacE2EELiveSessionFactory(
      auditFixtureKeyState: keyState, derivation: derivation,
      authorization: authorization)
    let preparation = try factory.prepareOutgoing(
      .init(
        groupID: group.groupID, authorDeviceID: identity.deviceID,
        expectedGroupRevision: group.revision,
        expectedTargetSnapshotDigest: target, nowMS: 3_000
      )
    ) { reservation in
      liveStart(generation: Int64(reservation.generation))
    }

    #expect(preparation.reservation.domain == "live_ptt")
    #expect(preparation.reservation.epoch == group.epoch)
    #expect(preparation.reservation.generation == 1)
    #expect(preparation.reservation.revision == group.revision + 1)
    #expect(preparation.start.generation == 1)
    #expect(derivation.contexts.count == 1)
    #expect(derivation.contexts[0].epoch == expectedContext.epoch)
    #expect(derivation.contexts[0].commitDigest == group.commitDigest)
    #expect(throws: MacE2EEKeyStateFailure.conflict) {
      try MacE2EELiveSessionFactory(
        auditFixtureKeyState: keyState, derivation: derivation,
        authorization: authorization)
    }
  }

  @Test("two installations with skewed local revisions share commit-bound AAD")
  func crossInstallationRoundTrip() throws {
    let target = String(repeating: "a", count: 64)
    let commit = String(repeating: "b", count: 64)
    let groupID = "air-group-fixture-00000000000001"
    let senderDeviceID = "mac-device-fixture-0001"
    let receiverDeviceID = "mac-device-fixture-0002"
    let senderState = MacE2EEKeyStateRepository(
      store: LiveMemoryKeychain(), random: LiveFixedRandom())
    let receiverState = MacE2EEKeyStateRepository(
      store: LiveMemoryKeychain(), random: LiveFixedRandom())
    let senderIdentity = try senderState.installDeviceIdentity(
      deviceID: senderDeviceID, keyFormat: "fixture-v1",
      signingPrivateKey: Data(repeating: 0x11, count: 32),
      keyAgreementPrivateKey: Data(repeating: 0x22, count: 32),
      createdAtMS: 1_000)
    let receiverIdentity = try receiverState.installDeviceIdentity(
      deviceID: receiverDeviceID, keyFormat: "fixture-v1",
      signingPrivateKey: Data(repeating: 0x44, count: 32),
      keyAgreementPrivateKey: Data(repeating: 0x55, count: 32),
      createdAtMS: 1_000)
    let senderGroup = try senderState.persistGroupState(
      installationID: senderIdentity.installationID, groupID: groupID, epoch: 9,
      previousCommitDigest: "", commitDigest: commit,
      targetSnapshotDigest: target, opaqueState: Data(repeating: 0x33, count: 64),
      expectedRevision: 0, nowMS: 2_000)
    let receiverGroup = try receiverState.persistGroupState(
      installationID: receiverIdentity.installationID, groupID: groupID, epoch: 9,
      previousCommitDigest: "", commitDigest: commit,
      targetSnapshotDigest: target, opaqueState: Data(repeating: 0x66, count: 64),
      expectedRevision: 0, nowMS: 2_000)
    let authorization = LiveAuthorizationBox(
      .init(
        groupID: groupID, epoch: 9, commitDigest: commit,
        targetSnapshotDigest: target,
        authorizedSenderDeviceIDs: [senderDeviceID]))
    let senderFactory = try MacE2EELiveSessionFactory(
      auditFixtureKeyState: senderState, derivation: LiveFixtureDerivation(),
      authorization: authorization)
    let receiverFactory = try MacE2EELiveSessionFactory(
      auditFixtureKeyState: receiverState, derivation: LiveFixtureDerivation(),
      authorization: authorization)
    let outgoing = try senderFactory.prepareOutgoing(
      .init(
        groupID: groupID, authorDeviceID: senderDeviceID,
        expectedGroupRevision: senderGroup.revision,
        expectedTargetSnapshotDigest: target, nowMS: 3_000
      )
    ) { reservation in
      liveStart(generation: Int64(reservation.generation))
    }
    #expect(outgoing.reservation.revision == senderGroup.revision + 1)
    #expect(receiverGroup.revision != outgoing.reservation.revision)

    let incoming = try receiverFactory.prepareIncoming(
      .init(
        groupID: groupID, localDeviceID: receiverDeviceID,
        authorDeviceID: senderDeviceID, epoch: receiverGroup.epoch,
        expectedLocalGroupRevision: receiverGroup.revision,
        start: outgoing.start))
    let plaintext = liveFrame(1, payload: Data("cross-installation-opus".utf8))
    let opaque = try outgoing.channel.protect(plaintext)

    #expect(try incoming.open(opaque) == plaintext)
  }
}
