import Foundation

public enum MacStreamTrackDraftStoreError: Error, Equatable, Sendable {
  case invalidFile
  case persistence
}

public struct MacStreamTrackDraftRecord: Codable, Equatable, Sendable {
  public let localID: String
  public let displayName: String
  public let localByteCount: Int64
  public let clientMIME: String
  public var uploadOffset: Int64
  public var mediaID: String?

  public init(
    localID: String, displayName: String, localByteCount: Int64, clientMIME: String,
    uploadOffset: Int64 = 0, mediaID: String? = nil
  ) {
    self.localID = localID
    self.displayName = displayName
    self.localByteCount = localByteCount
    self.clientMIME = clientMIME
    self.uploadOffset = uploadOffset
    self.mediaID = mediaID
  }
}

/// Owns one app-private long-track draft. Intake never materializes the source
/// as Data: FileHandle copies fixed 64 KiB chunks and the previous draft stays
/// authoritative until the replacement metadata has been atomically renamed.
public final class MacStreamTrackDraftStore: @unchecked Sendable {
  public static let maximumFileBytes: Int64 = 524_288_000
  public static let copyChunkBytes = 64 * 1_024

  private let lock = NSLock()
  private let root: URL
  private let fileManager: FileManager

  public init(root: URL, fileManager: FileManager = .default) throws {
    guard root.isFileURL else { throw MacStreamTrackDraftStoreError.persistence }
    self.root = root
    self.fileManager = fileManager
    do {
      try fileManager.createDirectory(
        at: root, withIntermediateDirectories: true,
        attributes: [.posixPermissions: 0o700])
      try fileManager.setAttributes([.posixPermissions: 0o700], ofItemAtPath: root.path)
    } catch { throw MacStreamTrackDraftStoreError.persistence }
  }

  public func importFile(_ source: URL) throws -> MacStreamTrackDraftRecord {
    guard source.isFileURL else { throw MacStreamTrackDraftStoreError.invalidFile }
    let name = source.lastPathComponent.trimmingCharacters(in: .whitespacesAndNewlines)
    guard Self.validTitle(name), Self.eligibleExtension(name) else {
      throw MacStreamTrackDraftStoreError.invalidFile
    }
    let values: URLResourceValues
    do {
      values = try source.resourceValues(forKeys: [.fileSizeKey, .isRegularFileKey])
    } catch { throw MacStreamTrackDraftStoreError.invalidFile }
    guard values.isRegularFile == true, let rawSize = values.fileSize else {
      throw MacStreamTrackDraftStoreError.invalidFile
    }
    let size = Int64(rawSize)
    guard size > 0, size <= Self.maximumFileBytes else {
      throw MacStreamTrackDraftStoreError.invalidFile
    }

    lock.lock()
    defer { lock.unlock() }
    let localID = UUID().uuidString.replacingOccurrences(of: "-", with: "").lowercased()
    let temporary = root.appendingPathComponent(".track-\(UUID().uuidString)")
    let destination = dataURL(localID: localID)
    let previous = try? loadMetadataLocked()
    fileManager.createFile(
      atPath: temporary.path, contents: nil, attributes: [.posixPermissions: 0o600])
    do {
      let input = try FileHandle(forReadingFrom: source)
      let output = try FileHandle(forWritingTo: temporary)
      defer { try? input.close() }
      defer { try? output.close() }
      var copied: Int64 = 0
      while copied < size {
        let requested = min(Self.copyChunkBytes, Int(size - copied))
        guard let chunk = try input.read(upToCount: requested), !chunk.isEmpty else {
          throw MacStreamTrackDraftStoreError.persistence
        }
        try output.write(contentsOf: chunk)
        copied += Int64(chunk.count)
      }
      guard copied == size,
        (try input.read(upToCount: 1))?.isEmpty != false
      else { throw MacStreamTrackDraftStoreError.persistence }
      try output.synchronize()
      try output.close()
      try fileManager.moveItem(at: temporary, to: destination)
      let record = MacStreamTrackDraftRecord(
        localID: localID, displayName: name, localByteCount: size,
        clientMIME: Self.mime(name))
      do { try saveMetadataLocked(record) } catch {
        try? fileManager.removeItem(at: destination)
        throw error
      }
      if let previous, previous.localID != localID {
        try? fileManager.removeItem(at: dataURL(localID: previous.localID))
      }
      return record
    } catch {
      try? fileManager.removeItem(at: temporary)
      throw error is MacStreamTrackDraftStoreError
        ? error : MacStreamTrackDraftStoreError.persistence
    }
  }

  public func load() throws -> MacStreamTrackDraftRecord? {
    lock.lock()
    defer { lock.unlock() }
    guard fileManager.fileExists(atPath: metadataURL.path) else { return nil }
    let record = try loadMetadataLocked()
    let attributes: [FileAttributeKey: Any]
    do {
      attributes = try fileManager.attributesOfItem(atPath: dataURL(localID: record.localID).path)
    } catch { throw MacStreamTrackDraftStoreError.persistence }
    guard (attributes[.type] as? FileAttributeType) == .typeRegular,
      (attributes[.size] as? NSNumber)?.int64Value == record.localByteCount
    else { throw MacStreamTrackDraftStoreError.persistence }
    return record
  }

  public func update(_ record: MacStreamTrackDraftRecord) throws {
    lock.lock()
    defer { lock.unlock() }
    guard Self.valid(record) else { throw MacStreamTrackDraftStoreError.persistence }
    let current = try loadMetadataLocked()
    guard current.localID == record.localID,
      current.displayName == record.displayName,
      current.localByteCount == record.localByteCount,
      current.clientMIME == record.clientMIME
    else { throw MacStreamTrackDraftStoreError.persistence }
    try saveMetadataLocked(record)
  }

  public func delete(localID: String) throws {
    lock.lock()
    defer { lock.unlock() }
    let current = try loadMetadataLocked()
    guard current.localID == localID else { throw MacStreamTrackDraftStoreError.invalidFile }
    do {
      if fileManager.fileExists(atPath: dataURL(localID: localID).path) {
        try fileManager.removeItem(at: dataURL(localID: localID))
      }
      if fileManager.fileExists(atPath: metadataURL.path) {
        try fileManager.removeItem(at: metadataURL)
      }
    } catch { throw MacStreamTrackDraftStoreError.persistence }
  }

  public func fileURL(localID: String) -> URL { dataURL(localID: localID) }

  private var metadataURL: URL { root.appendingPathComponent("draft.v1.json") }
  private func dataURL(localID: String) -> URL { root.appendingPathComponent("\(localID).bin") }

  private func loadMetadataLocked() throws -> MacStreamTrackDraftRecord {
    do {
      let data = try Data(contentsOf: metadataURL)
      guard data.count <= 4_096 else { throw MacStreamTrackDraftStoreError.persistence }
      let record = try JSONDecoder().decode(MacStreamTrackDraftRecord.self, from: data)
      guard Self.valid(record) else { throw MacStreamTrackDraftStoreError.persistence }
      return record
    } catch { throw MacStreamTrackDraftStoreError.persistence }
  }

  private func saveMetadataLocked(_ record: MacStreamTrackDraftRecord) throws {
    guard Self.valid(record) else { throw MacStreamTrackDraftStoreError.persistence }
    let temporary = root.appendingPathComponent("draft.v1.json.tmp")
    do {
      let data = try JSONEncoder().encode(record)
      try data.write(to: temporary, options: .atomic)
      try fileManager.setAttributes([.posixPermissions: 0o600], ofItemAtPath: temporary.path)
      if fileManager.fileExists(atPath: metadataURL.path) {
        _ = try fileManager.replaceItemAt(metadataURL, withItemAt: temporary)
      } else {
        try fileManager.moveItem(at: temporary, to: metadataURL)
      }
    } catch {
      try? fileManager.removeItem(at: temporary)
      throw MacStreamTrackDraftStoreError.persistence
    }
  }

  private static func valid(_ record: MacStreamTrackDraftRecord) -> Bool {
    record.localID.count == 32
      && record.localID.allSatisfy { $0.isHexDigit && !$0.isUppercase }
      && validTitle(record.displayName) && eligibleExtension(record.displayName)
      && record.localByteCount > 0 && record.localByteCount <= maximumFileBytes
      && record.uploadOffset >= 0 && record.uploadOffset <= record.localByteCount
      && (record.mediaID == nil || validPublicID(record.mediaID!, prefix: "m_"))
  }

  private static func validTitle(_ value: String) -> Bool {
    !value.isEmpty && value == value.trimmingCharacters(in: .whitespacesAndNewlines)
      && value.utf8.count <= 512
  }

  private static func eligibleExtension(_ value: String) -> Bool {
    ["aac", "flac", "m4a", "mp3", "ogg", "opus", "wav"]
      .contains((value as NSString).pathExtension.lowercased())
  }

  private static func mime(_ value: String) -> String {
    switch (value as NSString).pathExtension.lowercased() {
    case "aac": "audio/aac"
    case "flac": "audio/flac"
    case "m4a": "audio/mp4"
    case "mp3": "audio/mpeg"
    case "ogg", "opus": "audio/ogg"
    case "wav": "audio/wav"
    default: "application/octet-stream"
    }
  }

  private static func validPublicID(_ value: String, prefix: String) -> Bool {
    value.hasPrefix(prefix) && value.count == prefix.count + 26
      && value.dropFirst(prefix.count).allSatisfy {
        "0123456789ABCDEFGHJKMNPQRSTVWXYZ".contains($0)
      }
  }
}
