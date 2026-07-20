import Foundation

public enum MacE2EELiveFailure: String, Error, Equatable, LocalizedError, Sendable {
  case invalidContext = "invalid_context"
  case invalidFrame = "invalid_frame"
  case providerNotApproved = "provider_not_approved"
  case malformedProviderOutput = "malformed_provider_output"
  case authenticationFailed = "authentication_failed"
  case replay = "replay"
  case nonceReuse = "nonce_reuse"
  case staleEpoch = "stale_epoch"
  case senderRemoved = "sender_removed"
  case membershipChanged = "membership_changed"
  case terminal = "terminal"

  public var errorDescription: String? { rawValue }
}

/// The coordinator-visible, keyless live envelope accepted by the dormant
/// opaque router. The ciphertext remains provider-owned; NodeCore only binds
/// and bounds it. This mirrors coordinator/internal/e2eecontract/opaque_live.go.
public struct MacE2EEOpaqueLiveFrame: Equatable, Sendable {
  public static let headerBytes = 84
  public static let maximumCiphertextBytes = 512
  public static let maximumMessageBytes = headerBytes + maximumCiphertextBytes
  public static let startFlag: UInt8 = 1
  public static let endFlag: UInt8 = 2

  public let flags: UInt8
  public let sessionID: [UInt8]
  public let epoch: UInt64
  public let generation: UInt64
  public let sequence: UInt32
  public let captureMonotonicUS: UInt64
  public let targetSnapshotDigest: String
  public let ciphertext: Data

  public init(
    flags: UInt8, sessionID: [UInt8], epoch: UInt64, generation: UInt64,
    sequence: UInt32, captureMonotonicUS: UInt64,
    targetSnapshotDigest: String, ciphertext: Data
  ) {
    self.flags = flags
    self.sessionID = sessionID
    self.epoch = epoch
    self.generation = generation
    self.sequence = sequence
    self.captureMonotonicUS = captureMonotonicUS
    self.targetSnapshotDigest = targetSnapshotDigest
    self.ciphertext = ciphertext
  }

  public func encoded() throws -> Data {
    try validate()
    guard let target = Self.hexDecode(targetSnapshotDigest) else {
      throw MacE2EELiveFailure.invalidFrame
    }
    var bytes = [UInt8](repeating: 0, count: Self.headerBytes + ciphertext.count)
    bytes[0] = 0x42
    bytes[1] = 0x45
    bytes[2] = 1
    bytes[3] = flags
    bytes.replaceSubrange(4..<20, with: sessionID)
    Self.put(epoch, into: &bytes, at: 20)
    Self.put(generation, into: &bytes, at: 28)
    Self.put(sequence, into: &bytes, at: 36)
    Self.put(captureMonotonicUS, into: &bytes, at: 40)
    bytes.replaceSubrange(48..<80, with: target)
    Self.put(UInt16(ciphertext.count), into: &bytes, at: 80)
    bytes.replaceSubrange(84..<bytes.count, with: ciphertext)
    return Data(bytes)
  }

  public static func decode(_ data: Data) throws -> Self {
    let bytes = [UInt8](data)
    guard bytes.count >= headerBytes, bytes.count <= maximumMessageBytes,
      bytes[0] == 0x42, bytes[1] == 0x45, bytes[2] == 1,
      bytes[82] == 0, bytes[83] == 0
    else { throw MacE2EELiveFailure.invalidFrame }
    let size: UInt16 = get(bytes, at: 80)
    guard size > 0, size <= maximumCiphertextBytes,
      bytes.count == headerBytes + Int(size)
    else { throw MacE2EELiveFailure.invalidFrame }
    let frame = Self(
      flags: bytes[3], sessionID: Array(bytes[4..<20]),
      epoch: get(bytes, at: 20), generation: get(bytes, at: 28),
      sequence: get(bytes, at: 36), captureMonotonicUS: get(bytes, at: 40),
      targetSnapshotDigest: Self.hex(Data(bytes[48..<80])),
      ciphertext: Data(bytes[84...]))
    try frame.validate()
    return frame
  }

  private func validate() throws {
    guard flags & ~UInt8(3) == 0, sessionID.count == 16,
      sessionID.contains(where: { $0 != 0 }), epoch > 0, generation > 0,
      sequence > 0, sequence <= 15_000, captureMonotonicUS > 0,
      Self.validDigest(targetSnapshotDigest), !ciphertext.isEmpty,
      ciphertext.count <= Self.maximumCiphertextBytes,
      (sequence == 1) == (flags & Self.startFlag != 0)
    else { throw MacE2EELiveFailure.invalidFrame }
  }

  private static func put<T: FixedWidthInteger>(
    _ value: T, into bytes: inout [UInt8], at offset: Int
  ) {
    for index in 0..<MemoryLayout<T>.size {
      bytes[offset + index] = UInt8(
        truncatingIfNeeded:
          value >> T((MemoryLayout<T>.size - 1 - index) * 8))
    }
  }

  private static func get<T: FixedWidthInteger>(_ bytes: [UInt8], at offset: Int) -> T {
    var value: T = 0
    for index in 0..<MemoryLayout<T>.size {
      value = (value << 8) | T(bytes[offset + index])
    }
    return value
  }

  private static func validDigest(_ value: String) -> Bool {
    value.count == 64 && value.allSatisfy { "0123456789abcdef".contains($0) }
  }

  private static func hex(_ data: Data) -> String {
    data.map { String(format: "%02x", $0) }.joined()
  }

  private static func hexDecode(_ value: String) -> [UInt8]? {
    guard validDigest(value) else { return nil }
    var bytes: [UInt8] = []
    bytes.reserveCapacity(value.count / 2)
    var index = value.startIndex
    while index < value.endIndex {
      let next = value.index(index, offsetBy: 2)
      guard let byte = UInt8(value[index..<next], radix: 16) else { return nil }
      bytes.append(byte)
      index = next
    }
    return bytes
  }
}

public struct MacE2EELiveSessionContext: Equatable, Sendable {
  public let groupID: String
  public let authorDeviceID: String
  public let epoch: UInt64
  public let commitDigest: String
  public let sessionID: String
  public let generation: UInt64
  public let senderActorID: Int64
  public let senderOrbitID: Int64
  public let senderNodeID: String
  public let targetSnapshotDigest: String
  public let playbackDomain: String
  public let playbackDomainID: Int64
  public let codecProfile: String
  public let frameMS: Int
  public let maximumPlaintextBytes: Int
  public let jitterBufferMS: Int
  public let maximumDurationMS: Int64

  public init(
    groupID: String, authorDeviceID: String, epoch: UInt64,
    commitDigest: String, start: LivePTTStartPayload
  ) throws {
    guard (try? LivePTTValidation.validate(.livePTTStart(start))) != nil,
      groupID.count >= 8, groupID.count <= 128,
      authorDeviceID.count >= 8, authorDeviceID.count <= 128,
      epoch > 0, Self.validDigest(commitDigest), start.generation > 0,
      start.targetSha256.count == 64,
      start.targetSha256.allSatisfy({ "0123456789abcdef".contains($0) })
    else { throw MacE2EELiveFailure.invalidContext }
    self.groupID = groupID
    self.authorDeviceID = authorDeviceID
    self.epoch = epoch
    self.commitDigest = commitDigest
    self.sessionID = start.sessionId
    self.generation = UInt64(start.generation)
    self.senderActorID = start.senderActorId
    self.senderOrbitID = start.senderOrbitId
    self.senderNodeID = start.senderNodeId
    self.targetSnapshotDigest = start.targetSha256
    self.playbackDomain = start.playbackDomain
    self.playbackDomainID = start.playbackDomainId
    self.codecProfile = start.codecProfile
    self.frameMS = start.frameMs
    self.maximumPlaintextBytes = start.maxPayloadBytes
    self.jitterBufferMS = start.jitterBufferMs
    self.maximumDurationMS = start.maxDurationMs
  }

  private static func validDigest(_ value: String) -> Bool {
    value.count == 64 && value.allSatisfy { "0123456789abcdef".contains($0) }
  }
}

public struct MacE2EELiveAuthorizationSnapshot: Equatable, Sendable {
  public let groupID: String
  public let epoch: UInt64
  public let commitDigest: String
  public let targetSnapshotDigest: String
  public let authorizedSenderDeviceIDs: Set<String>

  public init(
    groupID: String, epoch: UInt64, commitDigest: String,
    targetSnapshotDigest: String, authorizedSenderDeviceIDs: Set<String>
  ) {
    self.groupID = groupID
    self.epoch = epoch
    self.commitDigest = commitDigest
    self.targetSnapshotDigest = targetSnapshotDigest
    self.authorizedSenderDeviceIDs = authorizedSenderDeviceIDs
  }
}

/// Updated only from a verified membership/epoch control-plane transition.
/// Per-frame reads must be in-memory and bounded; Keychain or network I/O does
/// not belong on the capture worker or jitter queue.
public protocol MacE2EELiveAuthorizationChecking: Sendable {
  func currentAuthorization() -> MacE2EELiveAuthorizationSnapshot
}

public struct MacE2EELiveSealedPayload: Equatable, Sendable {
  public let nonceToken: Data
  public let wireCiphertext: Data

  public init(nonceToken: Data, wireCiphertext: Data) {
    self.nonceToken = nonceToken
    self.wireCiphertext = wireCiphertext
  }
}

public struct MacE2EELiveOpenedPayload: Equatable, Sendable {
  public let nonceToken: Data
  public let plaintext: Data

  public init(nonceToken: Data, plaintext: Data) {
    self.nonceToken = nonceToken
    self.plaintext = plaintext
  }
}

/// Implemented by the future independently reviewed suite. Its opaque wire
/// ciphertext must carry and authenticate its nonce. The nonce is returned as
/// a stable token solely so NodeCore can fail closed on reuse.
public protocol MacE2EELiveCryptographicSession: AnyObject, Sendable {
  var productionApproved: Bool { get }
  func seal(
    plaintext: Data, sequence: UInt32, authenticatedData: Data
  ) throws -> MacE2EELiveSealedPayload
  func open(
    wireCiphertext: Data, sequence: UInt32, authenticatedData: Data
  ) throws -> MacE2EELiveOpenedPayload
  func destroy()
}

/// The derivation boundary receives the witnessed epoch state and exact live
/// context. NodeCore deliberately contains no candidate production cipher or
/// group-key implementation while EPC-001/002 remain open.
public protocol MacE2EELiveSessionDeriving: Sendable {
  var productionApproved: Bool { get }
  func derive(
    context: MacE2EELiveSessionContext,
    identity: MacE2EEDeviceIdentityLease,
    groupState: MacE2EEGroupStateLease
  ) throws -> any MacE2EELiveCryptographicSession
}

public struct MacE2EELiveOutgoingRequest: Sendable {
  public let groupID: String
  public let authorDeviceID: String
  public let expectedGroupRevision: UInt64
  public let expectedTargetSnapshotDigest: String
  public let nowMS: Int64

  public init(
    groupID: String, authorDeviceID: String, expectedGroupRevision: UInt64,
    expectedTargetSnapshotDigest: String, nowMS: Int64
  ) {
    self.groupID = groupID
    self.authorDeviceID = authorDeviceID
    self.expectedGroupRevision = expectedGroupRevision
    self.expectedTargetSnapshotDigest = expectedTargetSnapshotDigest
    self.nowMS = nowMS
  }
}

public struct MacE2EELiveIncomingRequest: Sendable {
  public let groupID: String
  public let localDeviceID: String
  public let authorDeviceID: String
  public let epoch: UInt64
  public let expectedLocalGroupRevision: UInt64
  public let start: LivePTTStartPayload

  public init(
    groupID: String, localDeviceID: String, authorDeviceID: String,
    epoch: UInt64, expectedLocalGroupRevision: UInt64,
    start: LivePTTStartPayload
  ) {
    self.groupID = groupID
    self.localDeviceID = localDeviceID
    self.authorDeviceID = authorDeviceID
    self.epoch = epoch
    self.expectedLocalGroupRevision = expectedLocalGroupRevision
    self.start = start
  }
}

public struct MacE2EELiveOutgoingPreparation: @unchecked Sendable {
  public let start: LivePTTStartPayload
  public let reservation: MacE2EESendReservation
  public let channel: MacE2EELiveFrameChannel
}

/// Loads witnessed identity/group state, reserves a crash-safe live generation,
/// and invokes the reviewed provider with the exact authorized epoch. There is
/// intentionally no NodeApp composition while the production crypto/container
/// and cross-process ownership gates remain open.
public final class MacE2EELiveSessionFactory: @unchecked Sendable {
  public typealias MakeStart =
    @Sendable (MacE2EESendReservation) throws
    -> LivePTTStartPayload

  private let keyState: MacE2EEKeyStateRepository
  private let derivation: any MacE2EELiveSessionDeriving
  private let authorization: any MacE2EELiveAuthorizationChecking
  private let fixtureMode: Bool

  public init(
    keyState: MacE2EEKeyStateRepository,
    derivation: any MacE2EELiveSessionDeriving,
    authorization: any MacE2EELiveAuthorizationChecking,
    crossProcessGenerationSerializationApproved: Bool
  ) throws {
    guard derivation.productionApproved,
      crossProcessGenerationSerializationApproved
    else { throw MacE2EELiveFailure.providerNotApproved }
    try keyState.claimE2EELiveSendOwnership()
    self.keyState = keyState
    self.derivation = derivation
    self.authorization = authorization
    self.fixtureMode = false
  }

  /// Internal audit-only constructor; cannot be reached by NodeApp runtime.
  init(
    auditFixtureKeyState keyState: MacE2EEKeyStateRepository,
    derivation: any MacE2EELiveSessionDeriving,
    authorization: any MacE2EELiveAuthorizationChecking
  ) throws {
    try keyState.claimE2EELiveSendOwnership()
    self.keyState = keyState
    self.derivation = derivation
    self.authorization = authorization
    self.fixtureMode = true
  }

  public func prepareOutgoing(
    _ request: MacE2EELiveOutgoingRequest,
    makeStart: MakeStart
  ) throws -> MacE2EELiveOutgoingPreparation {
    guard request.nowMS > 0 else { throw MacE2EELiveFailure.invalidContext }
    let identity: MacE2EEDeviceIdentityLease
    let initial: MacE2EEGroupStateLease
    do {
      identity = try keyState.loadDeviceIdentity(deviceID: request.authorDeviceID)
      initial = try keyState.loadGroupState(
        installationID: identity.metadata.installationID, groupID: request.groupID)
    } catch { throw MacE2EELiveFailure.staleEpoch }
    defer {
      identity.destroy()
      initial.destroy()
    }
    guard initial.metadata.revision == request.expectedGroupRevision,
      initial.metadata.targetSnapshotDigest == request.expectedTargetSnapshotDigest
    else { throw MacE2EELiveFailure.membershipChanged }
    let reservation: MacE2EESendReservation
    do {
      reservation = try keyState.reserveSendGeneration(
        installationID: initial.metadata.installationID, groupID: request.groupID,
        domain: "live_ptt", expectedRevision: initial.metadata.revision,
        nowMS: request.nowMS)
    } catch { throw MacE2EELiveFailure.staleEpoch }
    let current: MacE2EEGroupStateLease
    do {
      current = try keyState.loadGroupState(
        installationID: initial.metadata.installationID, groupID: request.groupID)
    } catch { throw MacE2EELiveFailure.staleEpoch }
    defer { current.destroy() }
    guard current.metadata.revision == reservation.revision,
      current.metadata.epoch == reservation.epoch,
      current.metadata.targetSnapshotDigest == request.expectedTargetSnapshotDigest
    else { throw MacE2EELiveFailure.membershipChanged }
    let start = try makeStart(reservation)
    guard start.generation > 0, UInt64(start.generation) == reservation.generation,
      start.targetSha256 == request.expectedTargetSnapshotDigest
    else { throw MacE2EELiveFailure.invalidContext }
    let context = try MacE2EELiveSessionContext(
      groupID: request.groupID, authorDeviceID: request.authorDeviceID,
      epoch: reservation.epoch, commitDigest: current.metadata.commitDigest,
      start: start)
    let crypto = try derivation.derive(
      context: context, identity: identity, groupState: current)
    let channel: MacE2EELiveFrameChannel
    if fixtureMode {
      channel = try MacE2EELiveFrameChannel(
        auditFixtureContext: context, crypto: crypto, authorization: authorization)
    } else {
      channel = try MacE2EELiveFrameChannel(
        context: context, crypto: crypto, authorization: authorization)
    }
    return .init(start: start, reservation: reservation, channel: channel)
  }

  public func prepareIncoming(
    _ request: MacE2EELiveIncomingRequest
  ) throws -> MacE2EELiveFrameChannel {
    let identity: MacE2EEDeviceIdentityLease
    let group: MacE2EEGroupStateLease
    do {
      identity = try keyState.loadDeviceIdentity(deviceID: request.localDeviceID)
      group = try keyState.loadGroupState(
        installationID: identity.metadata.installationID, groupID: request.groupID)
    } catch { throw MacE2EELiveFailure.staleEpoch }
    defer {
      identity.destroy()
      group.destroy()
    }
    guard group.metadata.epoch == request.epoch,
      group.metadata.revision == request.expectedLocalGroupRevision,
      group.metadata.targetSnapshotDigest == request.start.targetSha256
    else { throw MacE2EELiveFailure.staleEpoch }
    let context = try MacE2EELiveSessionContext(
      groupID: request.groupID, authorDeviceID: request.authorDeviceID,
      epoch: request.epoch, commitDigest: group.metadata.commitDigest,
      start: request.start)
    let crypto = try derivation.derive(
      context: context, identity: identity, groupState: group)
    if fixtureMode {
      return try MacE2EELiveFrameChannel(
        auditFixtureContext: context, crypto: crypto, authorization: authorization)
    }
    return try MacE2EELiveFrameChannel(
      context: context, crypto: crypto, authorization: authorization)
  }
}

/// Thread-safe frame protection state. Crypto runs on the caller's sender or
/// jitter worker, never on microphone capture or audio render callbacks.
public final class MacE2EELiveFrameChannel: @unchecked Sendable {
  private let context: MacE2EELiveSessionContext
  private let crypto: any MacE2EELiveCryptographicSession
  private let authorization: any MacE2EELiveAuthorizationChecking
  private let lock = NSLock()
  private var terminal = false
  private var cryptoDestroyed = false
  private var outgoingSequence: UInt32 = 0
  private var outgoingCaptureBaseUS: UInt64?
  private var outgoingNonces: Set<Data> = []
  private var lastPlaintextFrame: LivePTTBinaryFrame?
  private var lastOpaqueFrame: MacE2EEOpaqueLiveFrame?
  private var incomingHighestSequence: UInt32 = 0
  private var incomingCaptureBaseUS: UInt64?
  private var incomingSequences: Set<UInt32> = []
  private var incomingNonces: Set<Data> = []

  init(
    context: MacE2EELiveSessionContext,
    crypto: any MacE2EELiveCryptographicSession,
    authorization: any MacE2EELiveAuthorizationChecking
  ) throws {
    guard crypto.productionApproved else {
      crypto.destroy()
      throw MacE2EELiveFailure.providerNotApproved
    }
    self.context = context
    self.crypto = crypto
    self.authorization = authorization
    try Self.validateAuthorization(authorization.currentAuthorization(), context: context)
  }

  /// Repository-only audit fixture constructor. It cannot be called by the app
  /// target and therefore cannot promote a test cipher through configuration.
  init(
    auditFixtureContext context: MacE2EELiveSessionContext,
    crypto: any MacE2EELiveCryptographicSession,
    authorization: any MacE2EELiveAuthorizationChecking
  ) throws {
    self.context = context
    self.crypto = crypto
    self.authorization = authorization
    try Self.validateAuthorization(authorization.currentAuthorization(), context: context)
  }

  deinit { lock.withLock { destroyCryptoLocked() } }

  public func protect(_ frame: LivePTTBinaryFrame) throws -> MacE2EEOpaqueLiveFrame {
    try lock.withLock {
      guard !terminal else { throw MacE2EELiveFailure.terminal }
      do {
        try checkAuthorization()
        if frame == lastPlaintextFrame, let cached = lastOpaqueFrame { return cached }
        guard frame.sessionId == Self.sessionBytes(context.sessionID),
          frame.sequence == outgoingSequence + 1,
          frame.sequence <= 15_000,
          frame.payload.count <= context.maximumPlaintextBytes,
          (try? frame.encoded()) != nil
        else { throw MacE2EELiveFailure.invalidFrame }
        if frame.sequence == 1 { outgoingCaptureBaseUS = frame.captureMonotonicUs }
        guard let base = outgoingCaptureBaseUS,
          frame.captureMonotonicUs == base + UInt64(frame.sequence - 1)
            * UInt64(context.frameMS * 1_000)
        else { throw MacE2EELiveFailure.invalidFrame }
        let flags =
          (frame.flags & LivePTTBinaryFrame.startFlag != 0
            ? MacE2EEOpaqueLiveFrame.startFlag : 0)
          | (frame.flags & LivePTTBinaryFrame.endFlag != 0
            ? MacE2EEOpaqueLiveFrame.endFlag : 0)
        let aad = try Self.authenticatedData(
          context: context, flags: flags, sequence: frame.sequence,
          captureMonotonicUS: frame.captureMonotonicUs)
        let sealed = try crypto.seal(
          plaintext: frame.payload, sequence: frame.sequence, authenticatedData: aad)
        guard !sealed.nonceToken.isEmpty, sealed.nonceToken.count <= 256,
          !sealed.wireCiphertext.isEmpty,
          sealed.wireCiphertext.count <= MacE2EEOpaqueLiveFrame.maximumCiphertextBytes
        else { throw MacE2EELiveFailure.malformedProviderOutput }
        guard outgoingNonces.insert(sealed.nonceToken).inserted else {
          throw MacE2EELiveFailure.nonceReuse
        }
        let opaque = MacE2EEOpaqueLiveFrame(
          flags: flags, sessionID: frame.sessionId, epoch: context.epoch,
          generation: context.generation, sequence: frame.sequence,
          captureMonotonicUS: frame.captureMonotonicUs,
          targetSnapshotDigest: context.targetSnapshotDigest,
          ciphertext: sealed.wireCiphertext)
        _ = try opaque.encoded()
        outgoingSequence = frame.sequence
        lastPlaintextFrame = frame
        lastOpaqueFrame = opaque
        return opaque
      } catch {
        if (error as? MacE2EELiveFailure) != .invalidFrame { terminateLocked() }
        throw error
      }
    }
  }

  public func open(_ opaque: MacE2EEOpaqueLiveFrame) throws -> LivePTTBinaryFrame {
    try lock.withLock {
      guard !terminal else { throw MacE2EELiveFailure.terminal }
      do {
        try checkAuthorization()
        guard opaque.sessionID == Self.sessionBytes(context.sessionID),
          opaque.epoch == context.epoch,
          opaque.generation == context.generation,
          opaque.targetSnapshotDigest == context.targetSnapshotDigest,
          (try? opaque.encoded()) != nil
        else {
          if opaque.epoch != context.epoch { throw MacE2EELiveFailure.staleEpoch }
          throw MacE2EELiveFailure.invalidFrame
        }
        guard !incomingSequences.contains(opaque.sequence) else {
          throw MacE2EELiveFailure.replay
        }
        if incomingHighestSequence > 0 {
          guard opaque.sequence + LivePTTConstants.maxGapFrames >= incomingHighestSequence,
            opaque.sequence <= incomingHighestSequence + LivePTTConstants.maxGapFrames
          else { throw MacE2EELiveFailure.replay }
        } else {
          guard opaque.sequence == 1 else { throw MacE2EELiveFailure.invalidFrame }
        }
        if opaque.sequence == 1 { incomingCaptureBaseUS = opaque.captureMonotonicUS }
        guard let base = incomingCaptureBaseUS,
          opaque.captureMonotonicUS == base
            + UInt64(opaque.sequence - 1) * UInt64(context.frameMS * 1_000)
        else { throw MacE2EELiveFailure.invalidFrame }
        let aad = try Self.authenticatedData(
          context: context, flags: opaque.flags, sequence: opaque.sequence,
          captureMonotonicUS: opaque.captureMonotonicUS)
        let opened: MacE2EELiveOpenedPayload
        do {
          opened = try crypto.open(
            wireCiphertext: opaque.ciphertext, sequence: opaque.sequence,
            authenticatedData: aad)
        } catch {
          throw MacE2EELiveFailure.authenticationFailed
        }
        guard !opened.nonceToken.isEmpty, opened.nonceToken.count <= 256 else {
          throw MacE2EELiveFailure.malformedProviderOutput
        }
        guard incomingNonces.insert(opened.nonceToken).inserted else {
          throw MacE2EELiveFailure.nonceReuse
        }
        guard !opened.plaintext.isEmpty,
          opened.plaintext.count <= context.maximumPlaintextBytes
        else { throw MacE2EELiveFailure.malformedProviderOutput }
        incomingSequences.insert(opaque.sequence)
        incomingHighestSequence = max(incomingHighestSequence, opaque.sequence)
        var flags = LivePTTBinaryFrame.fecFlag
        if opaque.flags & MacE2EEOpaqueLiveFrame.startFlag != 0 {
          flags |= LivePTTBinaryFrame.startFlag
        }
        if opaque.flags & MacE2EEOpaqueLiveFrame.endFlag != 0 {
          flags |= LivePTTBinaryFrame.endFlag
        }
        let plaintext = LivePTTBinaryFrame(
          flags: flags, sessionId: opaque.sessionID, sequence: opaque.sequence,
          captureMonotonicUs: opaque.captureMonotonicUS, payload: opened.plaintext)
        _ = try plaintext.encoded()
        return plaintext
      } catch {
        terminateLocked()
        throw error
      }
    }
  }

  public func terminate() { lock.withLock { terminateLocked() } }

  public func isTerminal() -> Bool { lock.withLock { terminal } }

  private func checkAuthorization() throws {
    try Self.validateAuthorization(authorization.currentAuthorization(), context: context)
  }

  private static func validateAuthorization(
    _ current: MacE2EELiveAuthorizationSnapshot,
    context: MacE2EELiveSessionContext
  ) throws {
    guard current.groupID == context.groupID else {
      throw MacE2EELiveFailure.membershipChanged
    }
    guard current.epoch == context.epoch else { throw MacE2EELiveFailure.staleEpoch }
    guard current.commitDigest == context.commitDigest,
      current.targetSnapshotDigest == context.targetSnapshotDigest
    else { throw MacE2EELiveFailure.membershipChanged }
    guard current.authorizedSenderDeviceIDs.contains(context.authorDeviceID) else {
      throw MacE2EELiveFailure.senderRemoved
    }
  }

  private func terminateLocked() {
    guard !terminal else { return }
    terminal = true
    destroyCryptoLocked()
    outgoingNonces.removeAll(keepingCapacity: false)
    incomingNonces.removeAll(keepingCapacity: false)
    incomingSequences.removeAll(keepingCapacity: false)
    lastPlaintextFrame = nil
    lastOpaqueFrame = nil
  }

  private func destroyCryptoLocked() {
    guard !cryptoDestroyed else { return }
    cryptoDestroyed = true
    crypto.destroy()
  }

  static func authenticatedData(
    context: MacE2EELiveSessionContext, flags: UInt8, sequence: UInt32,
    captureMonotonicUS: UInt64
  ) throws -> Data {
    guard let session = sessionBytes(context.sessionID) else {
      throw MacE2EELiveFailure.invalidContext
    }
    var data = Data("barycenter.e2ee.live.aad.v1".utf8)
    append(context.groupID, to: &data)
    append(context.authorDeviceID, to: &data)
    append(context.senderNodeID, to: &data)
    append(context.targetSnapshotDigest, to: &data)
    append(context.playbackDomain, to: &data)
    append(context.codecProfile, to: &data)
    data.append(contentsOf: session)
    append(context.commitDigest, to: &data)
    append(context.epoch, to: &data)
    append(context.generation, to: &data)
    append(UInt64(bitPattern: context.senderActorID), to: &data)
    append(UInt64(bitPattern: context.senderOrbitID), to: &data)
    append(UInt64(bitPattern: context.playbackDomainID), to: &data)
    append(UInt64(context.frameMS), to: &data)
    append(UInt64(context.maximumPlaintextBytes), to: &data)
    append(UInt64(context.jitterBufferMS), to: &data)
    append(UInt64(bitPattern: context.maximumDurationMS), to: &data)
    append(UInt64(flags), to: &data)
    append(UInt64(sequence), to: &data)
    append(captureMonotonicUS, to: &data)
    return data
  }

  private static func append(_ value: String, to data: inout Data) {
    let bytes = Data(value.utf8)
    append(UInt64(bytes.count), to: &data)
    data.append(bytes)
  }

  private static func append(_ value: UInt64, to data: inout Data) {
    var big = value.bigEndian
    withUnsafeBytes(of: &big) { data.append(contentsOf: $0) }
  }

  private static func sessionBytes(_ value: String) -> [UInt8]? {
    guard value.count == 32 else { return nil }
    var bytes: [UInt8] = []
    var index = value.startIndex
    while index < value.endIndex {
      let next = value.index(index, offsetBy: 2)
      guard let byte = UInt8(value[index..<next], radix: 16) else { return nil }
      bytes.append(byte)
      index = next
    }
    return bytes
  }
}

/// Retry-safe sender shim for MacLiveCaptureSender.trySendFrame. A transport
/// retry reuses the exact sealed bytes and nonce instead of resealing.
public final class MacE2EELiveSenderBridge: @unchecked Sendable {
  private let channel: MacE2EELiveFrameChannel
  private let trySendOpaque: @Sendable (Data) -> Bool

  public init(
    channel: MacE2EELiveFrameChannel,
    trySendOpaque: @escaping @Sendable (Data) -> Bool
  ) {
    self.channel = channel
    self.trySendOpaque = trySendOpaque
  }

  public func trySend(_ frame: LivePTTBinaryFrame) -> Bool {
    guard let opaque = try? channel.protect(frame),
      let wire = try? opaque.encoded()
    else { return false }
    return trySendOpaque(wire)
  }

  public func terminate() { channel.terminate() }
}

/// Authentication barrier placed before MacLiveJitterReceiver.receive. Any
/// tamper, replay, stale epoch or membership transition revokes buffered PCM;
/// unauthenticated bytes are never passed to Opus/FEC/PLC.
public final class MacE2EELiveReceiverBridge: @unchecked Sendable {
  private let channel: MacE2EELiveFrameChannel
  private let receiver: MacLiveJitterReceiving

  init(channel: MacE2EELiveFrameChannel, receiver: MacLiveJitterReceiving) {
    self.channel = channel
    self.receiver = receiver
  }

  @discardableResult
  func receiveOpaque(_ data: Data) -> LivePTTFrameDecision {
    do {
      let opaque = try MacE2EEOpaqueLiveFrame.decode(data)
      return receiver.receive(try channel.open(opaque))
    } catch MacE2EELiveFailure.replay {
      receiver.revoke(reason: "e2ee_replay")
      return .duplicate
    } catch MacE2EELiveFailure.staleEpoch {
      receiver.revoke(reason: "e2ee_stale_epoch")
      return .stale
    } catch MacE2EELiveFailure.membershipChanged,
      MacE2EELiveFailure.senderRemoved
    {
      receiver.revoke(reason: "e2ee_membership_changed")
      return .stale
    } catch {
      receiver.revoke(reason: "e2ee_authentication_failed")
      return .invalid
    }
  }

  func terminate(reason: String) {
    channel.terminate()
    receiver.revoke(reason: reason)
  }
}
