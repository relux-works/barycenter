import Foundation

public enum PhaseOneRoute: String, Codable, CaseIterable, Sendable {
  case thisPulsar = "this_pulsar"
  case ownBarycenter = "own_barycenter"
  case currentAir = "current_air"
}

public enum PhaseOneDelivery: String, Codable, CaseIterable, Sendable {
  case overlay
  case interrupt
  case afterCurrent = "after_current"
}

public enum PhaseOneOriginKind: String, Codable, Sendable {
  case microphone
  case file
}

public enum ContentPolicyLocale: String, Codable, CaseIterable, Sendable {
  case en
  case ru
}

public struct ContentPolicyManifest: Equatable, Sendable {
  public let version: String
  public let policyHash: String
  public let locale: ContentPolicyLocale
  public let localeHash: String
  public let effectiveAt: Date
  public let termsURL: URL
  public let contentGuidelinesURL: URL
  public let title: String
  public let rightsText: String
  public let consentText: String
  public let controllingLanguage: String
}

public struct ContentPolicyGrant: Equatable, Sendable {
  public let version: String
  public let policyHash: String
  public let locale: ContentPolicyLocale
  public let acceptedAt: Date
  public let revokedAt: Date?
  public let revision: Int64
  public let current: Bool
  public let termsAccepted: Bool
}

public struct PhaseOneUploadConfirmation: Equatable, Sendable {
  public let mediaID: String
  public let reused: Bool

  public init(mediaID: String, reused: Bool) {
    self.mediaID = mediaID
    self.reused = reused
  }
}

public struct PhaseOneTransmissionReceipt: Equatable, Sendable {
  public let transmissionID: String
  public let requestedDelivery: PhaseOneDelivery
  public let effectiveDelivery: PhaseOneDelivery
  public let downgradeReason: String?
  public let status: String
  public let reused: Bool

  public init(
    transmissionID: String,
    requestedDelivery: PhaseOneDelivery,
    effectiveDelivery: PhaseOneDelivery,
    downgradeReason: String?,
    status: String,
    reused: Bool
  ) {
    self.transmissionID = transmissionID
    self.requestedDelivery = requestedDelivery
    self.effectiveDelivery = effectiveDelivery
    self.downgradeReason = downgradeReason
    self.status = status
    self.reused = reused
  }
}

public struct PhaseOnePresenceNode: Equatable, Sendable {
  public let slot: String
  public let online: Bool
  public let outputState: String
  public let playbackState: String
  public let effectiveDND: String

  public init(
    slot: String,
    online: Bool,
    outputState: String,
    playbackState: String,
    effectiveDND: String
  ) {
    self.slot = slot
    self.online = online
    self.outputState = outputState
    self.playbackState = playbackState
    self.effectiveDND = effectiveDND
  }
}

public struct PhaseOneHistoryItem: Equatable, Sendable {
  public let id: String
  public let direction: String
  public let occurredAt: Date
  public let title: String
  public let senderName: String?
  public let requestedDelivery: String?
  public let effectiveDelivery: String?
  public let downgradeReason: String?
  public let status: String
  public let reasonCode: String?
  public let playedCount: Int?
  public let otherCount: Int?
  public let actions: [String]
  public let automation: PhaseOneAutomationHistory?

  public init(
    id: String,
    direction: String,
    occurredAt: Date,
    title: String,
    senderName: String?,
    requestedDelivery: String?,
    effectiveDelivery: String?,
    downgradeReason: String?,
    status: String,
    reasonCode: String?,
    playedCount: Int?,
    otherCount: Int?,
    actions: [String],
    automation: PhaseOneAutomationHistory? = nil
  ) {
    self.id = id
    self.direction = direction
    self.occurredAt = occurredAt
    self.title = title
    self.senderName = senderName
    self.requestedDelivery = requestedDelivery
    self.effectiveDelivery = effectiveDelivery
    self.downgradeReason = downgradeReason
    self.status = status
    self.reasonCode = reasonCode
    self.playedCount = playedCount
    self.otherCount = otherCount
    self.actions = actions
    self.automation = automation
  }
}

public struct PhaseOneAutomationHistory: Equatable, Sendable {
  public let triggerKind: String
  public let principalRef: String?
  public let principalLabel: String?
  public let scheduleID: String?
  public let scheduleLabel: String?
  public let scheduleRevision: Int64?
  public let executionID: String?
  public let cueID: String
  public let cueLabel: String?
  public let cueRevision: Int64?
  public let audienceKind: String?
  public let resolvedTargetCount: Int
  public let outcome: String
  public let reasonCode: String?
}

public struct PhaseOneHistoryPage: Equatable, Sendable {
  public let items: [PhaseOneHistoryItem]
  public let nextCursor: String?

  public init(items: [PhaseOneHistoryItem], nextCursor: String?) {
    self.items = items
    self.nextCursor = nextCursor
  }
}

public enum PhaseOneModerationReason: String, Codable, CaseIterable, Sendable {
  case spam
  case harassment
  case illegal
  case sexualContent = "sexual_content"
  case violence
  case other
}

public struct PhaseOneHistoryActionReceipt: Equatable, Sendable {
  public let outcome: String
  public let reused: Bool

  public init(outcome: String, reused: Bool = false) {
    self.outcome = outcome
    self.reused = reused
  }
}

public enum PhaseOneClientError: Error, Equatable, Sendable {
  case invalidConfiguration
  case invalidRequest
  case transport
  case redirectRejected
  case responseTooLarge
  case invalidResponse
  case rejected(status: Int, code: String, retryAfterSeconds: Int?)
}

public protocol PhaseOneAppServicing: Sendable {
  func upload(
    fileURL: URL,
    title: String,
    idempotencyKey: String,
    rightsAcknowledged: Bool
  ) async throws -> PhaseOneUploadConfirmation

  func contentPolicy(locale: ContentPolicyLocale) async throws -> ContentPolicyManifest
  func currentContentPolicyGrant() async throws -> ContentPolicyGrant
  func acceptContentPolicy(_ manifest: ContentPolicyManifest) async throws -> ContentPolicyGrant
  func revokeContentPolicy(locale: ContentPolicyLocale) async throws -> ContentPolicyGrant

  func transmit(
    mediaID: String,
    route: PhaseOneRoute,
    delivery: PhaseOneDelivery,
    originKind: PhaseOneOriginKind,
    idempotencyKey: String
  ) async throws -> PhaseOneTransmissionReceipt
  func transmitExplicit(
    mediaID: String,
    targetReferences: [String],
    includeOrigin: Bool,
    delivery: PhaseOneDelivery,
    originKind: PhaseOneOriginKind,
    idempotencyKey: String
  ) async throws -> PhaseOneTransmissionReceipt

  func deleteMedia(_ mediaID: String) async throws
  func presence() async throws -> [PhaseOnePresenceNode]
  func history(limit: Int, cursor: String?) async throws -> PhaseOneHistoryPage
  func deleteHistoryItem(_ historyItemID: String) async throws -> PhaseOneHistoryActionReceipt
  func reportHistoryItem(
    _ historyItemID: String,
    reason: PhaseOneModerationReason,
    details: String
  ) async throws -> PhaseOneHistoryActionReceipt
  func blockHistoryActor(
    _ historyItemID: String,
    idempotencyKey: String
  ) async throws -> PhaseOneHistoryActionReceipt
  func replayHistoryItem(
    _ historyItemID: String,
    route: PhaseOneRoute,
    delivery: PhaseOneDelivery,
    idempotencyKey: String
  ) async throws -> PhaseOneTransmissionReceipt
}

public extension PhaseOneAppServicing {
  func transmitExplicit(
    mediaID: String,
    targetReferences: [String],
    includeOrigin: Bool,
    delivery: PhaseOneDelivery,
    originKind: PhaseOneOriginKind,
    idempotencyKey: String
  ) async throws -> PhaseOneTransmissionReceipt {
    throw PhaseOneClientError.invalidRequest
  }
}

/// Authenticated HTTP binding for the frozen Phase 1 media, transmission,
/// presence and history contracts. It accepts only a canonical credential
/// origin and never follows redirects, sends credentials in URLs, or derives
/// status from a request that did not receive a successful coordinator reply.
public final class PhaseOneAppClient: PhaseOneAppServicing, SoundboardAppServicing, @unchecked Sendable {
  private static let maximumResponseBytes = 64 * 1_024
  public static let streamTrackChunkBytes = 4 * 1_024 * 1_024

  private let origin: CoordinatorOrigin
  private let controlToken: String
  private let transport: any OnboardingHTTPTransport

  public init(
    bundle: CredentialBundle,
    transport: any OnboardingHTTPTransport = URLSessionOnboardingTransport()
  ) throws {
    guard let origin = bundle.coordinatorOrigin,
      origin.isSecureForCredentials,
      let control = bundle.control,
      control.contextStrength == .active,
      control.orbitId != nil,
      CredentialSyntax.lowerHexToken(control.controlToken)
    else { throw PhaseOneClientError.invalidConfiguration }
    self.origin = origin
    self.controlToken = control.controlToken
    self.transport = transport
  }

  public func upload(
    fileURL: URL,
    title: String,
    idempotencyKey: String,
    rightsAcknowledged: Bool
  ) async throws -> PhaseOneUploadConfirmation {
    let cleanTitle = title.trimmingCharacters(in: .whitespacesAndNewlines)
    guard Self.validIdempotencyKey(idempotencyKey), !cleanTitle.isEmpty,
      cleanTitle.utf8.count <= 512,
      fileURL.isFileURL,
      let values = try? fileURL.resourceValues(forKeys: [.fileSizeKey, .isRegularFileKey]),
      values.isRegularFile == true,
      let size = values.fileSize,
      size > 0,
      rightsAcknowledged
    else { throw PhaseOneClientError.invalidRequest }

    let create = try await request(
      method: "POST",
      path: "/v1/media/uploads",
      bearer: controlToken,
      headers: ["Idempotency-Key": idempotencyKey],
      jsonBody: UploadCreateBody(
        kind: "voice_clip", title: cleanTitle, sizeBytes: Int64(size),
        rightsAcknowledged: rightsAcknowledged),
      success: [200, 201]
    )
    let session: UploadSessionResponse = try decode(create.data)
    guard Self.validUploadID(session.uploadID), Self.validMediaID(session.mediaID),
      session.uploadLength == Int64(size),
      session.uploadOffset >= 0,
      session.uploadOffset <= session.uploadLength
    else { throw PhaseOneClientError.invalidResponse }
    if session.status == "completed", session.uploadOffset == session.uploadLength {
      return PhaseOneUploadConfirmation(mediaID: session.mediaID, reused: true)
    }
    guard !session.uploadToken.isEmpty else { throw PhaseOneClientError.invalidResponse }

    let bytes: Data
    do { bytes = try Data(contentsOf: fileURL, options: .mappedIfSafe) }
    catch { throw PhaseOneClientError.invalidRequest }
    guard bytes.count == size else { throw PhaseOneClientError.invalidRequest }
    let offset = Int(session.uploadOffset)
    let remainder = bytes.subdata(in: offset..<bytes.count)
    let uploaded = try await request(
      method: "PUT",
      path: "/v1/media/uploads/\(session.uploadID)",
      bearer: session.uploadToken,
      headers: [
        "Upload-Offset": String(session.uploadOffset),
        "Content-Type": "application/octet-stream",
      ],
      rawBody: remainder,
      success: [200]
    )
    let completion: UploadSessionResponse = try decode(uploaded.data)
    guard completion.uploadID == session.uploadID,
      completion.mediaID == session.mediaID,
      completion.uploadOffset == completion.uploadLength,
      completion.uploadLength == Int64(size),
      completion.status == "completed"
    else { throw PhaseOneClientError.invalidResponse }
    return PhaseOneUploadConfirmation(mediaID: completion.mediaID, reused: session.reused ?? false)
  }

  /// Resumable long-track upload. Unlike the Phase 1 clip adapter above this
  /// method never maps or slices the whole file: each request owns at most one
  /// 4 MiB Data value and resumes from the coordinator's durable offset.
  public func uploadTrack(
    fileURL: URL,
    title: String,
    idempotencyKey: String,
    rightsAcknowledged: Bool,
    progress: @escaping @Sendable (Int64, Int64) -> Void = { _, _ in }
  ) async throws -> PhaseOneUploadConfirmation {
    let cleanTitle = title.trimmingCharacters(in: .whitespacesAndNewlines)
    guard Self.validIdempotencyKey(idempotencyKey), !cleanTitle.isEmpty,
      cleanTitle.utf8.count <= 512, fileURL.isFileURL,
      let values = try? fileURL.resourceValues(forKeys: [.fileSizeKey, .isRegularFileKey]),
      values.isRegularFile == true, let rawSize = values.fileSize,
      rightsAcknowledged
    else { throw PhaseOneClientError.invalidRequest }
    let size = Int64(rawSize)
    guard size > 0, size <= MacStreamTrackDraftStore.maximumFileBytes else {
      throw PhaseOneClientError.invalidRequest
    }
    let create = try await request(
      method: "POST", path: "/v1/media/uploads", bearer: controlToken,
      headers: ["Idempotency-Key": idempotencyKey],
      jsonBody: UploadCreateBody(
        kind: "audio_track", title: cleanTitle, sizeBytes: size,
        rightsAcknowledged: rightsAcknowledged), success: [200, 201])
    var session: UploadSessionResponse = try decode(create.data)
    guard Self.validUploadID(session.uploadID), Self.validMediaID(session.mediaID),
      session.uploadLength == size, session.uploadOffset >= 0,
      session.uploadOffset <= session.uploadLength
    else { throw PhaseOneClientError.invalidResponse }
    progress(session.uploadOffset, session.uploadLength)
    let reused = session.reused ?? false
    if session.status == "completed", session.uploadOffset == session.uploadLength {
      return PhaseOneUploadConfirmation(mediaID: session.mediaID, reused: reused)
    }
    guard session.status == "open", CredentialSyntax.lowerHexToken(session.uploadToken) else {
      throw PhaseOneClientError.invalidResponse
    }
    let uploadToken = session.uploadToken
    let handle: FileHandle
    do {
      handle = try FileHandle(forReadingFrom: fileURL)
      try handle.seek(toOffset: UInt64(session.uploadOffset))
    } catch { throw PhaseOneClientError.invalidRequest }
    defer { try? handle.close() }
    while session.uploadOffset < session.uploadLength {
      try Task.checkCancellation()
      let count = min(
        Self.streamTrackChunkBytes,
        Int(session.uploadLength - session.uploadOffset))
      let chunk: Data
      do { chunk = try handle.read(upToCount: count) ?? Data() }
      catch { throw PhaseOneClientError.invalidRequest }
      guard chunk.count == count else { throw PhaseOneClientError.invalidRequest }
      let response = try await request(
        method: "PUT", path: "/v1/media/uploads/\(session.uploadID)", bearer: uploadToken,
        headers: [
          "Upload-Offset": String(session.uploadOffset),
          "Content-Type": "application/octet-stream",
        ], rawBody: chunk, timeoutInterval: 15 * 60, success: [200])
      let next: UploadSessionResponse = try decode(response.data)
      let nextOffset = session.uploadOffset + Int64(count)
      guard next.uploadID == session.uploadID, next.mediaID == session.mediaID,
        next.uploadLength == session.uploadLength, next.uploadOffset == nextOffset,
        (nextOffset < next.uploadLength && next.status == "open")
          || (nextOffset == next.uploadLength && next.status == "completed")
      else { throw PhaseOneClientError.invalidResponse }
      session = next
      progress(session.uploadOffset, session.uploadLength)
    }
    guard session.status == "completed" else { throw PhaseOneClientError.invalidResponse }
    return PhaseOneUploadConfirmation(
      mediaID: session.mediaID, reused: reused)
  }

  public func contentPolicy(locale: ContentPolicyLocale) async throws -> ContentPolicyManifest {
    let response = try await request(
      method: "GET",
      path: "/v1/content-policy?locale=\(locale.rawValue)",
      bearer: controlToken,
      allowQuery: true,
      success: [200])
    let value: ContentPolicyResponse = try decode(response.data)
    return try Self.validatedContentPolicy(value, expectedLocale: locale)
  }

  public func acceptContentPolicy(
    _ manifest: ContentPolicyManifest
  ) async throws -> ContentPolicyGrant {
    guard Self.validPolicyVersion(manifest.version), Self.validPolicyHash(manifest.policyHash)
    else { throw PhaseOneClientError.invalidRequest }
    let response = try await request(
      method: "PUT",
      path: "/v1/content-policy/acceptance",
      bearer: controlToken,
      jsonBody: ContentPolicyAcceptanceBody(
        version: manifest.version, policyHash: manifest.policyHash,
        locale: manifest.locale.rawValue, termsAccepted: true),
      success: [200])
    return try Self.validatedContentPolicyGrant(
      decode(response.data), expectedVersion: manifest.version,
      expectedHash: manifest.policyHash, expectedLocale: manifest.locale)
  }

  public func currentContentPolicyGrant() async throws -> ContentPolicyGrant {
    let response = try await request(
      method: "GET", path: "/v1/content-policy/acceptance",
      bearer: controlToken, success: [200])
    let value: ContentPolicyGrantResponse = try decode(response.data)
    guard let locale = ContentPolicyLocale(rawValue: value.locale) else {
      throw PhaseOneClientError.invalidResponse
    }
    return try Self.validatedContentPolicyGrant(
      value, expectedVersion: nil, expectedHash: nil, expectedLocale: locale)
  }

  public func revokeContentPolicy(
    locale: ContentPolicyLocale
  ) async throws -> ContentPolicyGrant {
    let response = try await request(
      method: "DELETE",
      path: "/v1/content-policy/acceptance?locale=\(locale.rawValue)",
      bearer: controlToken,
      allowQuery: true,
      success: [200])
    return try Self.validatedContentPolicyGrant(
      decode(response.data), expectedVersion: nil, expectedHash: nil,
      expectedLocale: locale)
  }

  public func transmit(
    mediaID: String,
    route: PhaseOneRoute,
    delivery: PhaseOneDelivery,
    originKind: PhaseOneOriginKind,
    idempotencyKey: String
  ) async throws -> PhaseOneTransmissionReceipt {
    guard Self.validMediaID(mediaID), Self.validIdempotencyKey(idempotencyKey) else {
      throw PhaseOneClientError.invalidRequest
    }
    let response = try await request(
      method: "POST",
      path: "/v1/transmissions",
      bearer: controlToken,
      headers: ["Idempotency-Key": idempotencyKey],
      jsonBody: TransmissionCreateBody(
        mediaID: mediaID,
        audience: .init(kind: route.rawValue),
        delivery: delivery.rawValue,
        originKind: originKind.rawValue,
        includeOrigin: route == .thisPulsar),
      success: [200, 201]
    )
    let value: TransmissionResponse = try decode(response.data)
    guard Self.validTransmissionID(value.transmissionID), value.mediaID == mediaID,
      let requested = PhaseOneDelivery(rawValue: value.requestedDelivery),
      let effective = PhaseOneDelivery(rawValue: value.effectiveDelivery),
      !value.status.isEmpty
    else { throw PhaseOneClientError.invalidResponse }
    return PhaseOneTransmissionReceipt(
      transmissionID: value.transmissionID,
      requestedDelivery: requested,
      effectiveDelivery: effective,
      downgradeReason: value.downgradeReason,
      status: value.status,
      reused: value.reused ?? false)
  }

  public func transmitExplicit(
    mediaID: String,
    targetReferences: [String],
    includeOrigin: Bool,
    delivery: PhaseOneDelivery,
    originKind: PhaseOneOriginKind,
    idempotencyKey: String
  ) async throws -> PhaseOneTransmissionReceipt {
    guard Self.validMediaID(mediaID), Self.validIdempotencyKey(idempotencyKey),
      (1...64).contains(targetReferences.count),
      Set(targetReferences).count == targetReferences.count,
      targetReferences.allSatisfy(Self.validTargetReference)
    else { throw PhaseOneClientError.invalidRequest }
    let response = try await request(
      method: "POST",
      path: "/v1/transmissions",
      bearer: controlToken,
      headers: ["Idempotency-Key": idempotencyKey],
      jsonBody: ExplicitTransmissionCreateBody(
        mediaID: mediaID,
        audience: .init(
          kind: "explicit",
          targets: targetReferences.sorted().map { .init(reference: $0) }),
        delivery: delivery.rawValue,
        originKind: originKind.rawValue,
        includeOrigin: includeOrigin),
      success: [200, 201])
    let value: TransmissionResponse = try decode(response.data)
    guard Self.validTransmissionID(value.transmissionID), value.mediaID == mediaID,
      let requested = PhaseOneDelivery(rawValue: value.requestedDelivery),
      let effective = PhaseOneDelivery(rawValue: value.effectiveDelivery),
      !value.status.isEmpty
    else { throw PhaseOneClientError.invalidResponse }
    return PhaseOneTransmissionReceipt(
      transmissionID: value.transmissionID,
      requestedDelivery: requested,
      effectiveDelivery: effective,
      downgradeReason: value.downgradeReason,
      status: value.status,
      reused: value.reused ?? false)
  }

  public func deleteMedia(_ mediaID: String) async throws {
    guard Self.validMediaID(mediaID) else { throw PhaseOneClientError.invalidRequest }
    do {
      _ = try await request(
        method: "DELETE", path: "/v1/media/\(mediaID)", bearer: controlToken,
        requiresJSONResponse: false, success: [204])
    } catch PhaseOneClientError.rejected(let status, let code, _) where
      status == 404 && code == "media_not_found"
    {
      // A retry after the remote delete committed but before local outbox
      // metadata was removed is still a successful explicit delete.
      return
    }
  }

  public func presence() async throws -> [PhaseOnePresenceNode] {
    let response = try await request(
      method: "GET", path: "/v1/presence", bearer: controlToken, success: [200])
    let value: PresenceResponse = try decode(response.data)
    guard value.contract == "p1-history-presence-telegram-v1" else {
      throw PhaseOneClientError.invalidResponse
    }
    return value.nodes.map {
      PhaseOnePresenceNode(
        slot: $0.slot,
        online: $0.online,
        outputState: $0.outputState,
        playbackState: $0.playbackState,
        effectiveDND: $0.effectiveDND.mode)
    }
  }

  public func history(limit: Int = 30, cursor: String? = nil) async throws
    -> PhaseOneHistoryPage
  {
    guard (1...100).contains(limit), cursor?.isEmpty != true else {
      throw PhaseOneClientError.invalidRequest
    }
    var components = URLComponents()
    components.queryItems = [URLQueryItem(name: "limit", value: String(limit))]
    if let cursor { components.queryItems?.append(.init(name: "cursor", value: cursor)) }
    guard let query = components.percentEncodedQuery else { throw PhaseOneClientError.invalidRequest }
    let response = try await request(
      method: "GET", path: "/v1/history?\(query)", bearer: controlToken,
      allowQuery: true, success: [200])
    let value: HistoryResponse = try decode(response.data)
    guard value.contract == "p1-history-presence-telegram-v1" else {
      throw PhaseOneClientError.invalidResponse
    }
    let formatter = ISO8601DateFormatter()
    formatter.formatOptions = [.withInternetDateTime, .withFractionalSeconds]
    let items = try value.items.map { item -> PhaseOneHistoryItem in
      guard Self.validHistoryID(item.historyItemID),
        let date = formatter.date(from: item.occurredAt),
        ["sent", "received"].contains(item.direction),
        !item.media.title.isEmpty,
        !item.status.isEmpty,
        Self.validHistoryActions(item.actions)
      else { throw PhaseOneClientError.invalidResponse }
      return PhaseOneHistoryItem(
        id: item.historyItemID,
        direction: item.direction,
        occurredAt: date,
        title: item.media.title,
        senderName: item.sender?.displayName,
        requestedDelivery: item.requestedDelivery,
        effectiveDelivery: item.effectiveDelivery,
        downgradeReason: item.downgradeReason,
        status: item.status,
        reasonCode: item.reasonCode,
        playedCount: item.targetCounts?.played,
        otherCount: item.targetCounts?.other,
        actions: item.actions,
        automation: try Self.validatedAutomation(item.automation))
    }
    return PhaseOneHistoryPage(items: items, nextCursor: value.nextCursor)
  }

  public func deleteHistoryItem(_ historyItemID: String) async throws -> PhaseOneHistoryActionReceipt {
    guard Self.validHistoryID(historyItemID) else { throw PhaseOneClientError.invalidRequest }
    let response = try await request(
      method: "POST",
      path: "/v1/history/\(historyItemID)/actions/delete",
      bearer: controlToken,
      jsonBody: EmptyBody(),
      success: [200])
    let value: HistoryDeleteResponse = try decode(response.data)
    guard value.historyItemID == historyItemID, value.deleted else {
      throw PhaseOneClientError.invalidResponse
    }
    return .init(outcome: "media_deleted")
  }

  public func reportHistoryItem(
    _ historyItemID: String,
    reason: PhaseOneModerationReason,
    details: String
  ) async throws -> PhaseOneHistoryActionReceipt {
    guard Self.validHistoryID(historyItemID), Self.validReportDetails(details) else {
      throw PhaseOneClientError.invalidRequest
    }
    let response = try await request(
      method: "POST",
      path: "/v1/history/\(historyItemID)/actions/report",
      bearer: controlToken,
      jsonBody: HistoryReportBody(reason: reason, details: details),
      success: [200, 201])
    let value: HistoryReportResponse = try decode(response.data)
    guard value.historyItemID == historyItemID,
      Self.validPublicID(value.id, prefix: "rp_"),
      value.reason == reason,
      ["received", "reviewed"].contains(value.status)
    else { throw PhaseOneClientError.invalidResponse }
    return .init(
      outcome: value.reused ? "report_already_received" : "report_received",
      reused: value.reused)
  }

  public func blockHistoryActor(
    _ historyItemID: String,
    idempotencyKey: String
  ) async throws -> PhaseOneHistoryActionReceipt {
    guard Self.validHistoryID(historyItemID), Self.validIdempotencyKey(idempotencyKey) else {
      throw PhaseOneClientError.invalidRequest
    }
    let response = try await request(
      method: "POST",
      path: "/v1/history/\(historyItemID)/actions/block_actor",
      bearer: controlToken,
      headers: ["Idempotency-Key": idempotencyKey],
      jsonBody: EmptyBody(),
      success: [200, 201])
    let value: HistoryBlockResponse = try decode(response.data)
    guard Self.validPublicID(value.blockID, prefix: "bl_") else {
      throw PhaseOneClientError.invalidResponse
    }
    return .init(
      outcome: value.reused ? "sender_already_blocked" : "sender_blocked",
      reused: value.reused)
  }

  public func replayHistoryItem(
    _ historyItemID: String,
    route: PhaseOneRoute,
    delivery: PhaseOneDelivery,
    idempotencyKey: String
  ) async throws -> PhaseOneTransmissionReceipt {
    guard Self.validHistoryID(historyItemID), Self.validIdempotencyKey(idempotencyKey) else {
      throw PhaseOneClientError.invalidRequest
    }
    let response = try await request(
      method: "POST",
      path: "/v1/history/\(historyItemID)/actions/replay",
      bearer: controlToken,
      headers: ["Idempotency-Key": idempotencyKey],
      jsonBody: HistoryReplayBody(
        audience: .init(kind: route.rawValue),
        delivery: delivery.rawValue,
        includeOrigin: route == .thisPulsar),
      success: [200, 201])
    let value: TransmissionResponse = try decode(response.data)
    guard Self.validTransmissionID(value.transmissionID),
      let requested = PhaseOneDelivery(rawValue: value.requestedDelivery),
      let effective = PhaseOneDelivery(rawValue: value.effectiveDelivery),
      !value.status.isEmpty
    else { throw PhaseOneClientError.invalidResponse }
    return PhaseOneTransmissionReceipt(
      transmissionID: value.transmissionID,
      requestedDelivery: requested,
      effectiveDelivery: effective,
      downgradeReason: value.downgradeReason,
      status: value.status,
      reused: value.reused ?? false)
  }

  public func soundboardCues() async throws -> SoundboardCueList {
    let response = try await request(
      method: "GET", path: "/v1/soundboard/cues", bearer: controlToken, success: [200])
    return try Self.validatedCueList(decode(response.data))
  }

  public func createSoundboardMediaCue(
    title: String, mediaID: String, idempotencyKey: String
  ) async throws -> SoundboardCueList {
    guard Self.validCueTitle(title), Self.validMediaID(mediaID),
      Self.validIdempotencyKey(idempotencyKey) else { throw PhaseOneClientError.invalidRequest }
    let response = try await request(
      method: "POST", path: "/v1/soundboard/cues", bearer: controlToken,
      headers: ["Idempotency-Key": idempotencyKey],
      jsonBody: SoundboardCreateBody(title: title, source: .init(kind: "media", mediaID: mediaID)),
      success: [201])
    return try Self.validatedCueMutation(decode(response.data))
  }

  public func renameSoundboardCue(
    _ cueID: String, title: String, revision: Int64, idempotencyKey: String
  ) async throws -> SoundboardCueList {
    guard Self.validCueID(cueID), Self.validCueTitle(title), revision > 0,
      Self.validIdempotencyKey(idempotencyKey) else { throw PhaseOneClientError.invalidRequest }
    let response = try await request(
      method: "PATCH", path: "/v1/soundboard/cues/\(cueID)", bearer: controlToken,
      headers: ["Idempotency-Key": idempotencyKey],
      jsonBody: SoundboardRenameBody(title: title, expectedRevision: revision), success: [200])
    return try Self.validatedCueMutation(decode(response.data))
  }

  public func deleteSoundboardCue(
    _ cueID: String, revision: Int64, idempotencyKey: String
  ) async throws -> SoundboardCueList {
    guard Self.validCueID(cueID), revision > 0, Self.validIdempotencyKey(idempotencyKey)
    else { throw PhaseOneClientError.invalidRequest }
    let response = try await request(
      method: "DELETE", path: "/v1/soundboard/cues/\(cueID)", bearer: controlToken,
      headers: ["Idempotency-Key": idempotencyKey],
      jsonBody: ExpectedRevisionBody(expectedRevision: revision), success: [200])
    return try Self.validatedCueMutation(decode(response.data))
  }

  public func reorderSoundboardCues(
    _ cueIDs: [String], revision: Int64, idempotencyKey: String
  ) async throws -> SoundboardCueList {
    guard revision > 0, cueIDs.count <= 64, Set(cueIDs).count == cueIDs.count,
      cueIDs.allSatisfy(Self.validCueID), Self.validIdempotencyKey(idempotencyKey)
    else { throw PhaseOneClientError.invalidRequest }
    _ = try await request(
      method: "PUT", path: "/v1/soundboard/cues/order", bearer: controlToken,
      headers: ["Idempotency-Key": idempotencyKey],
      jsonBody: SoundboardOrderBody(expectedOrderRevision: revision, cueIDs: cueIDs),
      success: [200])
    return try await soundboardCues()
  }

  public func triggerSoundboardCue(
    _ cueID: String, intent: SoundboardTriggerIntent, idempotencyKey: String
  ) async throws -> SoundboardTriggerReceipt {
    guard Self.validCueID(cueID), Self.validIdempotencyKey(idempotencyKey)
    else { throw PhaseOneClientError.invalidRequest }
    let response = try await request(
      method: "POST", path: "/v1/soundboard/cues/\(cueID)/trigger", bearer: controlToken,
      headers: ["Idempotency-Key": idempotencyKey],
      jsonBody: SoundboardTriggerBody(
        audience: .init(kind: intent.route.rawValue), delivery: intent.delivery.rawValue,
        includeOrigin: intent.includeOrigin,
        fallbackConfirmation: intent.fallback.map {
          .init(token: $0.token, delivery: $0.delivery.rawValue)
        }), success: [200, 201])
    let value: SoundboardTriggerResponse = try decode(response.data)
    guard Self.validPublicID(value.executionID, prefix: "mx_"),
      Self.validTransmissionID(value.transmissionID),
      let requested = PhaseOneDelivery(rawValue: value.requestedDelivery),
      let effective = PhaseOneDelivery(rawValue: value.effectiveDelivery), !value.status.isEmpty
    else { throw PhaseOneClientError.invalidResponse }
    return SoundboardTriggerReceipt(
      executionID: value.executionID,
      transmission: .init(
        transmissionID: value.transmissionID, requestedDelivery: requested,
        effectiveDelivery: effective, downgradeReason: value.downgradeReason,
        status: value.status, reused: value.reused ?? false))
  }

  private func request<Body: Encodable>(
    method: String,
    path: String,
    bearer: String,
    headers: [String: String] = [:],
    jsonBody: Body,
    success: Set<Int>
  ) async throws -> HTTPTransportResponse {
    let encoder = JSONEncoder()
    encoder.outputFormatting = [.sortedKeys]
    let body: Data
    do { body = try encoder.encode(jsonBody) }
    catch { throw PhaseOneClientError.invalidRequest }
    var allHeaders = headers
    allHeaders["Content-Type"] = "application/json"
    return try await request(
      method: method, path: path, bearer: bearer, headers: allHeaders,
      rawBody: body, success: success)
  }

  private func request(
    method: String,
    path: String,
    bearer: String,
    headers: [String: String] = [:],
    rawBody: Data? = nil,
    allowQuery: Bool = false,
    requiresJSONResponse: Bool = true,
    timeoutInterval: TimeInterval = 30,
    success: Set<Int>
  ) async throws -> HTTPTransportResponse {
    let url: URL?
    if allowQuery, let marker = path.firstIndex(of: "?") {
      let endpointPath = String(path[..<marker])
      let query = String(path[path.index(after: marker)...])
      if let base = origin.endpoint(path: endpointPath) {
        var components = URLComponents(url: base, resolvingAgainstBaseURL: false)
        components?.percentEncodedQuery = query
        url = components?.url
      } else { url = nil }
    } else {
      url = origin.endpoint(path: path)
    }
    guard let url, CredentialSyntax.lowerHexToken(bearer) else {
      throw PhaseOneClientError.invalidRequest
    }
    var request = URLRequest(
      url: url, cachePolicy: .reloadIgnoringLocalAndRemoteCacheData,
      timeoutInterval: timeoutInterval)
    request.httpMethod = method
    request.httpBody = rawBody
    request.setValue("application/json", forHTTPHeaderField: "Accept")
    request.setValue("Bearer \(bearer)", forHTTPHeaderField: "Authorization")
    for (name, value) in headers { request.setValue(value, forHTTPHeaderField: name) }

    let received: HTTPTransportResponse
    do {
      received = try await transport.send(
        request, maximumResponseBytes: Self.maximumResponseBytes)
    } catch let error as OnboardingClientError where error == .responseTooLarge {
      throw PhaseOneClientError.responseTooLarge
    } catch {
      throw PhaseOneClientError.transport
    }
    guard received.data.count <= Self.maximumResponseBytes,
      let finalURL = received.response.url,
      let finalOrigin = try? CoordinatorOrigin(finalURL.absoluteString),
      finalOrigin == origin
    else { throw PhaseOneClientError.redirectRejected }
    if (300..<400).contains(received.response.statusCode) {
      throw PhaseOneClientError.redirectRejected
    }
    if requiresJSONResponse {
      guard received.response.value(forHTTPHeaderField: "Content-Type")?
        .split(separator: ";", maxSplits: 1).first?
        .trimmingCharacters(in: .whitespacesAndNewlines).lowercased() == "application/json"
      else { throw PhaseOneClientError.invalidResponse }
    }
    guard success.contains(received.response.statusCode) else {
      throw try apiError(received)
    }
    return received
  }

  private func request(
    method: String,
    path: String,
    bearer: String,
    headers: [String: String] = [:],
    allowQuery: Bool = false,
    requiresJSONResponse: Bool = true,
    success: Set<Int>
  ) async throws -> HTTPTransportResponse {
    try await request(
      method: method, path: path, bearer: bearer, headers: headers,
      rawBody: nil, allowQuery: allowQuery,
      requiresJSONResponse: requiresJSONResponse, success: success)
  }

  private func apiError(_ response: HTTPTransportResponse) throws -> Error {
    let envelope: ErrorEnvelope
    do { envelope = try JSONDecoder().decode(ErrorEnvelope.self, from: response.data) }
    catch { throw PhaseOneClientError.invalidResponse }
    if envelope.error.code == "requires_confirmation" {
      guard let details = envelope.error.details, let token = details.confirmationToken,
        token.hasPrefix("fc_"), token.count == 67
      else { throw PhaseOneClientError.invalidResponse }
      let alternatives = (details.alternatives ?? []).compactMap {
        $0.available ? PhaseOneDelivery(rawValue: $0.delivery) : nil
      }
      guard !alternatives.isEmpty else { throw PhaseOneClientError.invalidResponse }
      return PhaseOneConfirmationChallenge(token: token, alternatives: alternatives)
    }
    return PhaseOneClientError.rejected(
      status: response.response.statusCode,
      code: envelope.error.code,
      retryAfterSeconds: envelope.error.retryAfterSeconds)
  }

  private func decode<Value: Decodable>(_ data: Data) throws -> Value {
    do { return try JSONDecoder().decode(Value.self, from: data) }
    catch { throw PhaseOneClientError.invalidResponse }
  }

  private static func validIdempotencyKey(_ value: String) -> Bool {
    (16...128).contains(value.utf8.count) && value.unicodeScalars.allSatisfy {
      CharacterSet(charactersIn: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._:-")
        .contains($0)
    } && value.unicodeScalars.first.map {
      CharacterSet.alphanumerics.contains($0)
    } == true
  }

  private static func validPolicyVersion(_ value: String) -> Bool {
    (1...32).contains(value.utf8.count) && value == value.trimmingCharacters(in: .whitespacesAndNewlines)
  }

  private static func validPolicyHash(_ value: String) -> Bool {
    value.utf8.count == 64 && value.unicodeScalars.allSatisfy {
      CharacterSet(charactersIn: "0123456789abcdef").contains($0)
    }
  }

  private static func validatedContentPolicy(
    _ value: ContentPolicyResponse,
    expectedLocale: ContentPolicyLocale
  ) throws -> ContentPolicyManifest {
    guard value.contract == "p2-content-policy-consent.v1",
      validPolicyVersion(value.version), validPolicyHash(value.policyHash),
      validPolicyHash(value.localeHash), value.locale == expectedLocale.rawValue,
      value.controllingLanguage == "en",
      !value.title.isEmpty, !value.rightsText.isEmpty, !value.consentText.isEmpty,
      let effectiveAt = ISO8601DateFormatter().date(from: value.effectiveAt),
      let termsURL = URL(string: value.termsURL),
      let guidelinesURL = URL(string: value.contentGuidelinesURL),
      termsURL.absoluteString == "https://barycenter.live/legal/terms",
      guidelinesURL.absoluteString == "https://barycenter.live/legal/content-guidelines"
    else { throw PhaseOneClientError.invalidResponse }
    return ContentPolicyManifest(
      version: value.version, policyHash: value.policyHash, locale: expectedLocale,
      localeHash: value.localeHash, effectiveAt: effectiveAt, termsURL: termsURL,
      contentGuidelinesURL: guidelinesURL, title: value.title,
      rightsText: value.rightsText, consentText: value.consentText,
      controllingLanguage: value.controllingLanguage)
  }

  private static func validatedContentPolicyGrant(
    _ value: ContentPolicyGrantResponse,
    expectedVersion: String?,
    expectedHash: String?,
    expectedLocale: ContentPolicyLocale
  ) throws -> ContentPolicyGrant {
    guard value.contract == "p2-content-policy-consent.v1",
      validPolicyVersion(value.version), validPolicyHash(value.policyHash),
      expectedVersion == nil || value.version == expectedVersion,
      expectedHash == nil || value.policyHash == expectedHash,
      value.locale == expectedLocale.rawValue,
      let acceptedAt = ISO8601DateFormatter().date(from: value.acceptedAt),
      value.revision > 0,
      let locale = ContentPolicyLocale(rawValue: value.locale)
    else { throw PhaseOneClientError.invalidResponse }
    let revokedAt: Date?
    if let raw = value.revokedAt {
      guard let parsed = ISO8601DateFormatter().date(from: raw) else {
        throw PhaseOneClientError.invalidResponse
      }
      revokedAt = parsed
    } else {
      revokedAt = nil
    }
    guard value.current == (value.termsAccepted && revokedAt == nil) else {
      throw PhaseOneClientError.invalidResponse
    }
    return ContentPolicyGrant(
      version: value.version, policyHash: value.policyHash, locale: locale,
      acceptedAt: acceptedAt, revokedAt: revokedAt, revision: value.revision,
      current: value.current, termsAccepted: value.termsAccepted)
  }

  private static func validUploadID(_ value: String) -> Bool {
    validPublicID(value, prefix: "up_")
  }
  private static func validMediaID(_ value: String) -> Bool {
    validPublicID(value, prefix: "m_")
  }
  private static func validTransmissionID(_ value: String) -> Bool {
    validPublicID(value, prefix: "tr_")
  }
  private static func validHistoryID(_ value: String) -> Bool {
    if validPublicID(value, prefix: "hi_") { return true }
    let suffix = value.dropFirst(4)
    let lowerHex = CharacterSet(charactersIn: "0123456789abcdef")
    return value.hasPrefix("hi_a") && suffix.count == 25 && suffix.unicodeScalars.allSatisfy {
      lowerHex.contains($0)
    }
  }

  private static func validReportDetails(_ value: String) -> Bool {
    value == value.trimmingCharacters(in: .whitespacesAndNewlines) &&
      value.utf8.count <= 2_000 && value.unicodeScalars.allSatisfy {
        $0.value >= 0x20 && $0.value != 0x7f
      }
  }

  private static func validHistoryActions(_ values: [String]) -> Bool {
    let allowed = Set([
      "cancel", "delete", "replay", "report", "block_actor", "block_orbit", "unblock",
      "disable_schedule", "revoke_principal", "emergency_disable_automation",
    ])
    return Set(values).count == values.count && values.allSatisfy(allowed.contains)
  }

  private static func validPublicID(_ value: String, prefix: String) -> Bool {
    guard value.hasPrefix(prefix) else { return false }
    let suffix = value.dropFirst(prefix.count)
    return suffix.count == 26 && suffix.unicodeScalars.allSatisfy {
      CharacterSet(charactersIn: "0123456789ABCDEFGHJKMNPQRSTVWXYZ").contains($0)
    }
  }

  private static func validTargetReference(_ value: String) -> Bool {
    guard value.hasPrefix("trf_") else { return false }
    let suffix = value.dropFirst(4)
    return suffix.utf8.count == 43 && suffix.unicodeScalars.allSatisfy {
      CharacterSet(charactersIn: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-")
        .contains($0)
    }
  }

  private static func validCueID(_ value: String) -> Bool { validPublicID(value, prefix: "cq_") }
  private static func validCueTitle(_ value: String) -> Bool {
    let clean = value.trimmingCharacters(in: .whitespacesAndNewlines)
    return value == clean && !clean.isEmpty && clean.utf8.count <= 128
  }

  private static func validatedCue(_ value: SoundboardCueResponse) throws -> SoundboardCue {
    guard validCueID(value.cueID), validCueTitle(value.title), value.state == "active",
      value.revision > 0, value.sourceGeneration > 0, value.sourceBytes > 0,
      value.sourceDurationMS > 0, value.sourceSHA256.count == 64,
      value.sourceSHA256.allSatisfy({ $0.isHexDigit && !$0.isUppercase }),
      (value.sourceKind == "media" && value.mediaID.map(validMediaID) == true && value.builtinAssetID == nil
        || value.sourceKind == "builtin" && value.mediaID == nil && value.builtinAssetID == "pulsar.recording-cue.v1")
    else { throw PhaseOneClientError.invalidResponse }
    return .init(id: value.cueID, title: value.title, sourceKind: value.sourceKind,
      mediaID: value.mediaID, builtinAssetID: value.builtinAssetID,
      sourceSHA256: value.sourceSHA256, sourceBytes: value.sourceBytes,
      sourceDurationMS: value.sourceDurationMS, revision: value.revision,
      sourceGeneration: value.sourceGeneration, position: value.position ?? -1)
  }

  private static func validatedCueList(_ value: SoundboardListResponse) throws -> SoundboardCueList {
    guard value.orderRevision >= 0, value.cues.count <= 64 else { throw PhaseOneClientError.invalidResponse }
    let cues = try value.cues.map(validatedCue).sorted { $0.position < $1.position }
    guard Set(cues.map(\.id)).count == cues.count, cues.allSatisfy({ $0.position >= 0 })
    else { throw PhaseOneClientError.invalidResponse }
    return .init(orderRevision: value.orderRevision, cues: cues)
  }

  private static func validatedCueMutation(_ value: SoundboardMutationResponse) throws -> SoundboardCueList {
    guard value.orderRevision >= 0 else { throw PhaseOneClientError.invalidResponse }
    return .init(orderRevision: value.orderRevision, cues: [try validatedCue(value.cue)])
  }

  private static func validatedAutomation(
    _ value: HistoryResponse.Item.Automation?
  ) throws -> PhaseOneAutomationHistory? {
    guard let value else { return nil }
    guard ["manual_soundboard", "scoped_api", "schedule"].contains(value.triggerKind),
      validCueID(value.cueID), !value.outcome.isEmpty, value.resolvedTargetCount >= 0,
      value.executionID.map({ validPublicID($0, prefix: $0.hasPrefix("mx_") ? "mx_" : "ax_") }) != false,
      value.scheduleID.map({ validPublicID($0, prefix: "sch_") }) != false
    else { throw PhaseOneClientError.invalidResponse }
    return .init(triggerKind: value.triggerKind, principalRef: value.principalRef,
      principalLabel: value.principalLabel, scheduleID: value.scheduleID,
      scheduleLabel: value.scheduleLabel, scheduleRevision: value.scheduleRevision,
      executionID: value.executionID, cueID: value.cueID, cueLabel: value.cueLabel,
      cueRevision: value.cueRevision, audienceKind: value.audienceKind,
      resolvedTargetCount: value.resolvedTargetCount, outcome: value.outcome,
      reasonCode: value.reasonCode)
  }
}

private struct EmptyBody: Encodable {}

private struct UploadCreateBody: Encodable {
  let kind: String
  let title: String
  let sizeBytes: Int64
  let rightsAcknowledged: Bool

  enum CodingKeys: String, CodingKey {
    case kind, title
    case sizeBytes = "size_bytes"
    case rightsAcknowledged = "rights_acknowledged"
  }
}

private struct ContentPolicyResponse: Decodable {
  let contract: String
  let version: String
  let policyHash: String
  let locale: String
  let localeHash: String
  let effectiveAt: String
  let termsURL: String
  let contentGuidelinesURL: String
  let title: String
  let rightsText: String
  let consentText: String
  let controllingLanguage: String

  enum CodingKeys: String, CodingKey {
    case contract, version, locale, title
    case policyHash = "policy_hash"
    case localeHash = "locale_hash"
    case effectiveAt = "effective_at"
    case termsURL = "terms_url"
    case contentGuidelinesURL = "content_guidelines_url"
    case rightsText = "rights_text"
    case consentText = "consent_text"
    case controllingLanguage = "controlling_language"
  }
}

private struct ContentPolicyAcceptanceBody: Encodable {
  let version: String
  let policyHash: String
  let locale: String
  let termsAccepted: Bool

  enum CodingKeys: String, CodingKey {
    case version, locale
    case policyHash = "policy_hash"
    case termsAccepted = "terms_accepted"
  }
}

private struct ContentPolicyGrantResponse: Decodable {
  let contract: String
  let version: String
  let policyHash: String
  let locale: String
  let acceptedAt: String
  let revokedAt: String?
  let revision: Int64
  let current: Bool
  let termsAccepted: Bool

  enum CodingKeys: String, CodingKey {
    case contract, version, locale, revision, current
    case policyHash = "policy_hash"
    case acceptedAt = "accepted_at"
    case revokedAt = "revoked_at"
    case termsAccepted = "terms_accepted"
  }
}

private struct UploadSessionResponse: Decodable {
  let uploadID: String
  let mediaID: String
  let uploadToken: String
  let uploadOffset: Int64
  let uploadLength: Int64
  let status: String
  let reused: Bool?

  enum CodingKeys: String, CodingKey {
    case uploadID = "upload_id"
    case mediaID = "media_id"
    case uploadToken = "upload_token"
    case uploadOffset = "upload_offset"
    case uploadLength = "upload_length"
    case status, reused
  }

  init(from decoder: Decoder) throws {
    let values = try decoder.container(keyedBy: CodingKeys.self)
    uploadID = try values.decode(String.self, forKey: .uploadID)
    mediaID = try values.decode(String.self, forKey: .mediaID)
    uploadToken = try values.decodeIfPresent(String.self, forKey: .uploadToken) ?? ""
    uploadOffset = try values.decode(Int64.self, forKey: .uploadOffset)
    uploadLength = try values.decode(Int64.self, forKey: .uploadLength)
    status = try values.decode(String.self, forKey: .status)
    reused = try values.decodeIfPresent(Bool.self, forKey: .reused)
  }
}

private struct TransmissionCreateBody: Encodable {
  struct Audience: Encodable { let kind: String }
  let mediaID: String
  let audience: Audience
  let delivery: String
  let originKind: String
  let includeOrigin: Bool

  enum CodingKeys: String, CodingKey {
    case mediaID = "media_id"
    case audience, delivery
    case originKind = "origin_kind"
    case includeOrigin = "include_origin"
  }
}

private struct ExplicitTransmissionCreateBody: Encodable {
  struct Audience: Encodable {
    struct Target: Encodable { let reference: String }
    let kind: String
    let targets: [Target]
  }
  let mediaID: String
  let audience: Audience
  let delivery: String
  let originKind: String
  let includeOrigin: Bool

  enum CodingKeys: String, CodingKey {
    case mediaID = "media_id"
    case audience, delivery
    case originKind = "origin_kind"
    case includeOrigin = "include_origin"
  }
}

private struct HistoryReplayBody: Encodable {
  struct Audience: Encodable { let kind: String }
  let audience: Audience
  let delivery: String
  let includeOrigin: Bool

  enum CodingKeys: String, CodingKey {
    case audience, delivery
    case includeOrigin = "include_origin"
  }
}

private struct HistoryReportBody: Encodable {
  let reason: PhaseOneModerationReason
  let details: String
}

private struct HistoryDeleteResponse: Decodable {
  let historyItemID: String
  let deleted: Bool

  enum CodingKeys: String, CodingKey {
    case historyItemID = "history_item_id"
    case deleted
  }
}

private struct HistoryReportResponse: Decodable {
  let id: String
  let historyItemID: String
  let reason: PhaseOneModerationReason
  let status: String
  let reused: Bool

  enum CodingKeys: String, CodingKey {
    case id, reason, status, reused
    case historyItemID = "history_item_id"
  }
}

private struct HistoryBlockResponse: Decodable {
  let blockID: String
  let reused: Bool

  enum CodingKeys: String, CodingKey {
    case blockID = "block_id"
    case reused
  }
}

private struct TransmissionResponse: Decodable {
  let transmissionID: String
  let mediaID: String
  let requestedDelivery: String
  let effectiveDelivery: String
  let downgradeReason: String?
  let status: String
  let reused: Bool?

  enum CodingKeys: String, CodingKey {
    case transmissionID = "transmission_id"
    case mediaID = "media_id"
    case requestedDelivery = "requested_delivery"
    case effectiveDelivery = "effective_delivery"
    case downgradeReason = "downgrade_reason"
    case status, reused
  }
}

private struct PresenceResponse: Decodable {
  struct Node: Decodable {
    struct DND: Decodable { let mode: String }
    let slot: String
    let online: Bool
    let outputState: String
    let playbackState: String
    let effectiveDND: DND

    enum CodingKeys: String, CodingKey {
      case slot, online
      case outputState = "output_state"
      case playbackState = "playback_state"
      case effectiveDND = "effective_dnd"
    }
  }
  let contract: String
  let nodes: [Node]
}

private struct HistoryResponse: Decodable {
  struct Item: Decodable {
    struct Media: Decodable { let title: String }
    struct Sender: Decodable { let displayName: String }
    struct Counts: Decodable { let played: Int; let other: Int }
    struct Automation: Decodable {
      let triggerKind: String
      let principalRef: String?
      let principalLabel: String?
      let scheduleID: String?
      let scheduleLabel: String?
      let scheduleRevision: Int64?
      let executionID: String?
      let cueID: String
      let cueLabel: String?
      let cueRevision: Int64?
      let audienceKind: String?
      let resolvedTargetCount: Int
      let outcome: String
      let reasonCode: String?
      enum CodingKeys: String, CodingKey {
        case outcome
        case triggerKind = "trigger_kind", principalRef = "principal_ref"
        case principalLabel = "principal_label", scheduleID = "schedule_id"
        case scheduleLabel = "schedule_label", scheduleRevision = "schedule_revision"
        case executionID = "execution_id", cueID = "cue_id", cueLabel = "cue_label"
        case cueRevision = "cue_revision", audienceKind = "audience_kind"
        case resolvedTargetCount = "resolved_target_count", reasonCode = "reason_code"
      }
    }
    let historyItemID: String
    let direction: String
    let occurredAt: String
    let media: Media
    let sender: Sender?
    let requestedDelivery: String?
    let effectiveDelivery: String?
    let downgradeReason: String?
    let status: String
    let reasonCode: String?
    let targetCounts: Counts?
    let actions: [String]
    let automation: Automation?

    enum CodingKeys: String, CodingKey {
      case historyItemID = "history_item_id"
      case direction
      case occurredAt = "occurred_at"
      case media, sender
      case requestedDelivery = "requested_delivery"
      case effectiveDelivery = "effective_delivery"
      case downgradeReason = "downgrade_reason"
      case status
      case reasonCode = "reason_code"
      case targetCounts = "target_counts"
      case actions, automation
    }
  }
  let contract: String
  let items: [Item]
  let nextCursor: String?

  enum CodingKeys: String, CodingKey {
    case contract, items
    case nextCursor = "next_cursor"
  }
}

private struct ErrorEnvelope: Decodable {
  struct APIError: Decodable {
    struct Details: Decodable {
      struct Alternative: Decodable { let delivery: String; let available: Bool }
      let confirmationToken: String?
      let alternatives: [Alternative]?
      enum CodingKeys: String, CodingKey {
        case confirmationToken = "confirmation_token", alternatives
      }
    }
    let code: String
    let retryAfterSeconds: Int?
    let details: Details?
    enum CodingKeys: String, CodingKey {
      case code
      case retryAfterSeconds = "retry_after_seconds"
      case details
    }
  }
  let error: APIError
}

private struct SoundboardCreateBody: Encodable {
  struct Source: Encodable {
    let kind: String
    let mediaID: String
    enum CodingKeys: String, CodingKey { case kind; case mediaID = "media_id" }
  }
  let title: String
  let source: Source
}

private struct SoundboardRenameBody: Encodable {
  let title: String
  let expectedRevision: Int64
  enum CodingKeys: String, CodingKey { case title; case expectedRevision = "expected_revision" }
}

private struct ExpectedRevisionBody: Encodable {
  let expectedRevision: Int64
  enum CodingKeys: String, CodingKey { case expectedRevision = "expected_revision" }
}

private struct SoundboardOrderBody: Encodable {
  let expectedOrderRevision: Int64
  let cueIDs: [String]
  enum CodingKeys: String, CodingKey {
    case expectedOrderRevision = "expected_order_revision", cueIDs = "cue_ids"
  }
}

private struct SoundboardTriggerBody: Encodable {
  struct Audience: Encodable { let kind: String }
  struct Fallback: Encodable { let token: String; let delivery: String }
  let audience: Audience
  let delivery: String
  let includeOrigin: Bool
  let fallbackConfirmation: Fallback?
  enum CodingKeys: String, CodingKey {
    case audience, delivery, includeOrigin = "include_origin"
    case fallbackConfirmation = "fallback_confirmation"
  }
}

private struct SoundboardCueResponse: Decodable {
  let cueID: String
  let title: String
  let sourceKind: String
  let mediaID: String?
  let builtinAssetID: String?
  let sourceSHA256: String
  let sourceBytes: Int64
  let sourceDurationMS: Int64
  let state: String
  let revision: Int64
  let sourceGeneration: Int64
  let position: Int?
  enum CodingKeys: String, CodingKey {
    case title, state, revision, position
    case cueID = "cue_id", sourceKind = "source_kind", mediaID = "media_id"
    case builtinAssetID = "builtin_asset_id", sourceSHA256 = "source_sha256"
    case sourceBytes = "source_bytes", sourceDurationMS = "source_duration_ms"
    case sourceGeneration = "source_generation"
  }
}

private struct SoundboardListResponse: Decodable {
  let orderRevision: Int64
  let cues: [SoundboardCueResponse]
  enum CodingKeys: String, CodingKey { case orderRevision = "order_revision", cues }
}

private struct SoundboardMutationResponse: Decodable {
  let cue: SoundboardCueResponse
  let orderRevision: Int64
  enum CodingKeys: String, CodingKey { case cue; case orderRevision = "order_revision" }
}

private struct SoundboardTriggerResponse: Decodable {
  let executionID: String
  let transmissionID: String
  let requestedDelivery: String
  let effectiveDelivery: String
  let downgradeReason: String?
  let status: String
  let reused: Bool?
  enum CodingKeys: String, CodingKey {
    case status, reused
    case executionID = "execution_id", transmissionID = "transmission_id"
    case requestedDelivery = "requested_delivery", effectiveDelivery = "effective_delivery"
    case downgradeReason = "downgrade_reason"
  }
}
