import Foundation
import Testing

@testable import NodeCore

@Suite("macOS durable long-track draft store")
struct MacStreamTrackDraftStoreTests {
  @Test("Intake survives restart, checkpoints offset, and replaces the old draft")
  func durableReplacement() throws {
    let fixture = try Fixture()
    defer { fixture.remove() }
    let firstSource = try fixture.source(named: "first.mp3", bytes: 3 * 65_536 + 17)
    let secondSource = try fixture.source(named: "second.flac", bytes: 2 * 65_536 + 31)
    let store = try MacStreamTrackDraftStore(root: fixture.storeRoot)

    var first = try store.importFile(firstSource)
    #expect(first.displayName == "first.mp3")
    #expect(first.localByteCount == 3 * 65_536 + 17)
    #expect(
      Int64(try Data(contentsOf: store.fileURL(localID: first.localID)).count)
        == first.localByteCount)
    first.uploadOffset = 65_536
    first.mediaID = "m_" + String(repeating: "A", count: 26)
    try store.update(first)

    let restarted = try MacStreamTrackDraftStore(root: fixture.storeRoot)
    #expect(try restarted.load() == first)
    let oldURL = restarted.fileURL(localID: first.localID)
    let second = try restarted.importFile(secondSource)
    #expect(try restarted.load() == second)
    #expect(!FileManager.default.fileExists(atPath: oldURL.path))
    #expect(FileManager.default.fileExists(atPath: restarted.fileURL(localID: second.localID).path))

    try restarted.delete(localID: second.localID)
    #expect(try restarted.load() == nil)
  }

  @Test("Invalid, empty, and oversized files fail closed without replacing the draft")
  func invalidFilesDoNotReplaceDraft() throws {
    let fixture = try Fixture()
    defer { fixture.remove() }
    let store = try MacStreamTrackDraftStore(root: fixture.storeRoot)
    let valid = try store.importFile(try fixture.source(named: "kept.wav", bytes: 257))
    let empty = try fixture.source(named: "empty.mp3", bytes: 0)
    let executable = try fixture.source(named: "track.exe", bytes: 32)
    let oversized = fixture.root.appendingPathComponent("oversized.mp3")
    FileManager.default.createFile(atPath: oversized.path, contents: nil)
    let handle = try FileHandle(forWritingTo: oversized)
    try handle.truncate(atOffset: UInt64(MacStreamTrackDraftStore.maximumFileBytes + 1))
    try handle.close()

    #expect(throws: MacStreamTrackDraftStoreError.invalidFile) { try store.importFile(empty) }
    #expect(throws: MacStreamTrackDraftStoreError.invalidFile) { try store.importFile(executable) }
    #expect(throws: MacStreamTrackDraftStoreError.invalidFile) { try store.importFile(oversized) }
    let forged = MacStreamTrackDraftRecord(
      localID: String(repeating: "b", count: 32),
      displayName: valid.displayName,
      localByteCount: valid.localByteCount,
      clientMIME: valid.clientMIME)
    #expect(throws: MacStreamTrackDraftStoreError.persistence) { try store.update(forged) }
    #expect(try store.load() == valid)
  }

  @Test("Production intake and upload paths do not materialize the full track")
  func sourceMemoryBoundary() throws {
    let repositoryRoot = URL(fileURLWithPath: #filePath)
      .deletingLastPathComponent().deletingLastPathComponent()
      .deletingLastPathComponent().deletingLastPathComponent()
    let storeSource = try String(
      contentsOf: repositoryRoot.appendingPathComponent(
        "node-app/Sources/NodeCore/MacStreamTrackDraftStore.swift"))
    let clientSource = try String(
      contentsOf: repositoryRoot.appendingPathComponent(
        "node-app/Sources/NodeCore/PhaseOneAppClient.swift"))
    let intake = try #require(
      storeSource.split(separator: "public func importFile", maxSplits: 1).last
    )
    .split(separator: "public func load", maxSplits: 1).first.map(String.init)
    let upload = try #require(
      clientSource.split(separator: "public func uploadTrack", maxSplits: 1).last
    )
    .split(separator: "public func contentPolicy", maxSplits: 1).first.map(String.init)

    #expect(intake.contains("FileHandle"))
    #expect(intake.contains("copyChunkBytes"))
    #expect(!intake.contains("Data(contentsOf:"))
    #expect(upload.contains("FileHandle"))
    #expect(upload.contains("streamTrackChunkBytes"))
    #expect(!upload.contains("Data(contentsOf:"))
    #expect(!upload.contains("subdata"))
  }
}

private struct Fixture {
  let root: URL
  let storeRoot: URL

  init() throws {
    root = FileManager.default.temporaryDirectory
      .appendingPathComponent("mac-stream-track-store-\(UUID().uuidString)", isDirectory: true)
    storeRoot = root.appendingPathComponent("store", isDirectory: true)
    try FileManager.default.createDirectory(at: root, withIntermediateDirectories: true)
  }

  func source(named name: String, bytes: Int) throws -> URL {
    let url = root.appendingPathComponent(name)
    let data = Data((0..<bytes).map { UInt8($0 % 251) })
    try data.write(to: url)
    return url
  }

  func remove() { try? FileManager.default.removeItem(at: root) }
}
