import Foundation

public enum PhaseOneDraftState: String, Codable, Sendable {
  case retained
  case uploading
  case uploaded
  case transmitting
  case accepted
  case retryableFailure = "retryable_failure"
}

public struct PhaseOneDraftSnapshot: Equatable, Sendable {
  public let draftID: String
  public let title: String
  public let state: PhaseOneDraftState
  public let route: PhaseOneRoute?
  public let requestedDelivery: PhaseOneDelivery?
  public let effectiveDelivery: PhaseOneDelivery?
  public let downgradeReason: String?
  public let status: String?
  public let failureCode: String?
  public let localBytesRetained: Bool

  public init(
    draftID: String,
    title: String,
    state: PhaseOneDraftState,
    route: PhaseOneRoute?,
    requestedDelivery: PhaseOneDelivery?,
    effectiveDelivery: PhaseOneDelivery?,
    downgradeReason: String?,
    status: String?,
    failureCode: String?,
    localBytesRetained: Bool
  ) {
    self.draftID = draftID
    self.title = title
    self.state = state
    self.route = route
    self.requestedDelivery = requestedDelivery
    self.effectiveDelivery = effectiveDelivery
    self.downgradeReason = downgradeReason
    self.status = status
    self.failureCode = failureCode
    self.localBytesRetained = localBytesRetained
  }
}

public enum PhaseOneDraftOutboxError: Error, Equatable, Sendable {
  case invalidDraft
  case busy
  case persistence
  case localCleanup
  case remoteDelete
  case service(String)
}

/// Durable, idempotent boundary between finalized user recordings and the
/// coordinator. Self-test handles are rejected. The operation record is
/// fsynced before network work, survives process restart, and retains the same
/// upload/transmission keys for every retry.
public actor PhaseOneDraftOutbox {
  private struct Record: Codable, Equatable {
    let draftID: String
    var title: String
    let uploadKey: String
    let transmissionKey: String
    var state: PhaseOneDraftState
    var route: PhaseOneRoute?
    var requestedDelivery: PhaseOneDelivery?
    var mediaID: String?
    var transmissionID: String?
    var effectiveDelivery: PhaseOneDelivery?
    var downgradeReason: String?
    var status: String?
    var failureCode: String?
    var localBytesRetained: Bool
  }

  private struct Envelope: Codable {
    let version: Int
    var records: [Record]
  }

  private let service: any PhaseOneAppServicing
  private let mediaStore: CaptureMediaStore
  private let stateURL: URL
  private let fileManager: FileManager
  private var records: [String: Record] = [:]
  private var handles: [String: CaptureMediaHandle] = [:]
  private var activeDraftIDs: Set<String> = []

  public init(
    service: any PhaseOneAppServicing,
    mediaStore: CaptureMediaStore,
    stateURL: URL,
    recoveredDrafts: [CaptureMediaHandle],
    fileManager: FileManager = .default
  ) throws {
    self.service = service
    self.mediaStore = mediaStore
    self.stateURL = stateURL
    self.fileManager = fileManager

    let recovered = try Self.loadRecords(stateURL: stateURL, fileManager: fileManager)
    var initialRecords = Dictionary(uniqueKeysWithValues: recovered.map { ($0.draftID, $0) })
    var initialHandles: [String: CaptureMediaHandle] = [:]
    for handle in recoveredDrafts where
      handle.storageClass == .userRecording && handle.state == .durableUnsent
    {
      initialHandles[handle.id] = handle
      if initialRecords[handle.id] == nil {
        initialRecords[handle.id] = Self.newRecord(for: handle.id, title: "Pulsar recording")
      } else {
        initialRecords[handle.id]?.localBytesRetained = true
      }
    }
    for id in Array(initialRecords.keys) {
      guard initialHandles[id] == nil, initialRecords[id]?.mediaID == nil else { continue }
      // A record without bytes and without server confirmation cannot be
      // retried honestly. Drop only the corrupt metadata, never another file.
      initialRecords.removeValue(forKey: id)
    }
    records = initialRecords
    handles = initialHandles
    try Self.writeRecords(
      Array(initialRecords.values), stateURL: stateURL, fileManager: fileManager)
  }

  public func attach(_ handle: CaptureMediaHandle, title: String = "Pulsar recording") throws {
    guard handle.storageClass == .userRecording, handle.state == .durableUnsent else {
      throw PhaseOneDraftOutboxError.invalidDraft
    }
    handles[handle.id] = handle
    if records[handle.id] == nil {
      records[handle.id] = Self.newRecord(for: handle.id, title: title)
    } else {
      records[handle.id]?.localBytesRetained = true
    }
    try persist()
  }

  public func snapshots() -> [PhaseOneDraftSnapshot] {
    records.values.sorted { $0.draftID < $1.draftID }.map(Self.snapshot)
  }

  public func send(
    draftID: String,
    route: PhaseOneRoute,
    delivery: PhaseOneDelivery,
    originKind: PhaseOneOriginKind = .microphone
  ) async throws -> PhaseOneDraftSnapshot {
    guard var record = records[draftID] else { throw PhaseOneDraftOutboxError.invalidDraft }
    guard !activeDraftIDs.contains(draftID) else { throw PhaseOneDraftOutboxError.busy }
    if let frozenRoute = record.route, frozenRoute != route {
      throw PhaseOneDraftOutboxError.invalidDraft
    }
    if let frozenDelivery = record.requestedDelivery, frozenDelivery != delivery {
      throw PhaseOneDraftOutboxError.invalidDraft
    }
    record.route = route
    record.requestedDelivery = delivery
    record.failureCode = nil
    records[draftID] = record
    activeDraftIDs.insert(draftID)
    defer { activeDraftIDs.remove(draftID) }

    do {
      try persist()
      if record.mediaID == nil {
        guard let handle = handles[draftID] else {
          throw PhaseOneDraftOutboxError.invalidDraft
        }
        record.state = .uploading
        records[draftID] = record
        try persist()
        let uploaded = try await service.upload(
          fileURL: handle.fileURL,
          title: record.title,
          idempotencyKey: record.uploadKey)
        record.mediaID = uploaded.mediaID
        record.state = .uploaded
        record.status = "upload_confirmed"
        records[draftID] = record
        try persist()

        // The durable server confirmation is persisted before local cleanup.
        // If cleanup fails, retry resumes at transmission and never uploads a
        // duplicate; the stale bytes remain explicitly visible in the state.
        do {
          try mediaStore.confirmUploadAndDelete(handle)
          handles.removeValue(forKey: draftID)
          record.localBytesRetained = false
          records[draftID] = record
          try persist()
        } catch {
          record.state = .retryableFailure
          record.failureCode = "local_cleanup_failed"
          records[draftID] = record
          try? persist()
          throw PhaseOneDraftOutboxError.localCleanup
        }
      }

      guard let mediaID = record.mediaID else { throw PhaseOneDraftOutboxError.invalidDraft }
      record.state = .transmitting
      record.status = "requesting_acceptance"
      records[draftID] = record
      try persist()
      let receipt = try await service.transmit(
        mediaID: mediaID,
        route: route,
        delivery: delivery,
        originKind: originKind,
        idempotencyKey: record.transmissionKey)
      record.transmissionID = receipt.transmissionID
      record.effectiveDelivery = receipt.effectiveDelivery
      record.downgradeReason = receipt.downgradeReason
      record.status = receipt.status
      record.state = .accepted
      record.failureCode = nil
      records[draftID] = record
      try persist()
      return Self.snapshot(record)
    } catch let error as PhaseOneDraftOutboxError {
      if error != .localCleanup {
        record = records[draftID] ?? record
        record.state = .retryableFailure
        record.failureCode = Self.failureCode(error)
        records[draftID] = record
        try? persist()
      }
      throw error
    } catch let error as PhaseOneClientError {
      record = records[draftID] ?? record
      record.state = .retryableFailure
      record.failureCode = Self.failureCode(error)
      records[draftID] = record
      try? persist()
      throw PhaseOneDraftOutboxError.service(Self.failureCode(error))
    } catch {
      record = records[draftID] ?? record
      record.state = .retryableFailure
      record.failureCode = "service_unavailable"
      records[draftID] = record
      try? persist()
      throw PhaseOneDraftOutboxError.service("service_unavailable")
    }
  }

  public func delete(draftID: String) async throws {
    guard let record = records[draftID] else { throw PhaseOneDraftOutboxError.invalidDraft }
    guard !activeDraftIDs.contains(draftID) else { throw PhaseOneDraftOutboxError.busy }
    activeDraftIDs.insert(draftID)
    defer { activeDraftIDs.remove(draftID) }

    if let mediaID = record.mediaID {
      do { try await service.deleteMedia(mediaID) }
      catch { throw PhaseOneDraftOutboxError.remoteDelete }
    }
    if let handle = handles[draftID] {
      do { try mediaStore.explicitlyDelete(handle) }
      catch { throw PhaseOneDraftOutboxError.localCleanup }
      handles.removeValue(forKey: draftID)
    }
    records.removeValue(forKey: draftID)
    try persist()
  }

  private func persist() throws {
    try Self.writeRecords(
      Array(records.values), stateURL: stateURL, fileManager: fileManager)
  }

  private static func writeRecords(
    _ records: [Record],
    stateURL: URL,
    fileManager: FileManager
  ) throws {
    let envelope = Envelope(version: 1, records: records.sorted { $0.draftID < $1.draftID })
    let encoder = JSONEncoder()
    encoder.outputFormatting = [.sortedKeys]
    let data: Data
    do { data = try encoder.encode(envelope) }
    catch { throw PhaseOneDraftOutboxError.persistence }
    do {
      let directory = stateURL.deletingLastPathComponent()
      try fileManager.createDirectory(
        at: directory, withIntermediateDirectories: true,
        attributes: [.posixPermissions: 0o700])
      try fileManager.setAttributes([.posixPermissions: 0o700], ofItemAtPath: directory.path)
      try data.write(to: stateURL, options: [.atomic, .completeFileProtection])
      try fileManager.setAttributes([.posixPermissions: 0o600], ofItemAtPath: stateURL.path)
    } catch { throw PhaseOneDraftOutboxError.persistence }
  }

  private static func loadRecords(stateURL: URL, fileManager: FileManager) throws -> [Record] {
    guard fileManager.fileExists(atPath: stateURL.path) else { return [] }
    do {
      let data = try Data(contentsOf: stateURL)
      let envelope = try JSONDecoder().decode(Envelope.self, from: data)
      guard envelope.version == 1,
        Set(envelope.records.map(\.draftID)).count == envelope.records.count,
        envelope.records.allSatisfy({ validRecord($0) })
      else { throw PhaseOneDraftOutboxError.persistence }
      return envelope.records
    } catch let error as PhaseOneDraftOutboxError { throw error }
    catch { throw PhaseOneDraftOutboxError.persistence }
  }

  private static func validRecord(_ value: Record) -> Bool {
    value.draftID.count == 32
      && value.draftID.unicodeScalars.allSatisfy {
        CharacterSet(charactersIn: "0123456789abcdef").contains($0)
      }
      && value.uploadKey == "mac-upload-\(value.draftID)"
      && value.transmissionKey == "mac-transmission-\(value.draftID)"
      && !value.title.isEmpty && value.title.utf8.count <= 512
      && (value.mediaID.map { validPublicID($0, prefix: "m_") } ?? true)
      && (value.transmissionID.map { validPublicID($0, prefix: "tr_") } ?? true)
      && (value.transmissionID == nil || value.mediaID != nil)
  }

  private static func validPublicID(_ value: String, prefix: String) -> Bool {
    guard value.hasPrefix(prefix) else { return false }
    let suffix = value.dropFirst(prefix.count)
    return suffix.count == 26 && suffix.unicodeScalars.allSatisfy {
      CharacterSet(charactersIn: "0123456789ABCDEFGHJKMNPQRSTVWXYZ").contains($0)
    }
  }

  private static func newRecord(for draftID: String, title: String) -> Record {
    Record(
      draftID: draftID,
      title: title,
      uploadKey: "mac-upload-\(draftID)",
      transmissionKey: "mac-transmission-\(draftID)",
      state: .retained,
      route: nil,
      requestedDelivery: nil,
      mediaID: nil,
      transmissionID: nil,
      effectiveDelivery: nil,
      downgradeReason: nil,
      status: "ready_to_send",
      failureCode: nil,
      localBytesRetained: true)
  }

  private static func snapshot(_ value: Record) -> PhaseOneDraftSnapshot {
    PhaseOneDraftSnapshot(
      draftID: value.draftID,
      title: value.title,
      state: value.state,
      route: value.route,
      requestedDelivery: value.requestedDelivery,
      effectiveDelivery: value.effectiveDelivery,
      downgradeReason: value.downgradeReason,
      status: value.status,
      failureCode: value.failureCode,
      localBytesRetained: value.localBytesRetained)
  }

  private static func failureCode(_ error: PhaseOneDraftOutboxError) -> String {
    switch error {
    case .invalidDraft: "invalid_draft"
    case .busy: "operation_in_progress"
    case .persistence: "persistence_failed"
    case .localCleanup: "local_cleanup_failed"
    case .remoteDelete: "remote_delete_failed"
    case .service(let code): code
    }
  }

  private static func failureCode(_ error: PhaseOneClientError) -> String {
    switch error {
    case .rejected(_, let code, _): code
    case .transport: "coordinator_unavailable"
    case .redirectRejected: "redirect_rejected"
    case .responseTooLarge: "response_too_large"
    case .invalidConfiguration: "credential_unavailable"
    case .invalidRequest: "invalid_request"
    case .invalidResponse: "invalid_response"
    }
  }
}
