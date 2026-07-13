import Foundation
import Testing

@Suite
struct CLIPairingSourceTests {
  @Test func protectedSaveFailureCannotFallBackOrReportPairingSuccess() throws {
    let testFile = URL(fileURLWithPath: #filePath)
    let mainFile =
      testFile
      .deletingLastPathComponent()
      .deletingLastPathComponent()
      .deletingLastPathComponent()
      .appendingPathComponent("Sources/NodeApp/main.swift")
    let source = try String(contentsOf: mainFile, encoding: .utf8)
    let successStart = try #require(source.range(of: "case .success(let creds):"))
    let failureStart = try #require(
      source.range(of: "case .failure(let err):", range: successStart.upperBound..<source.endIndex))
    let successBranch = source[successStart.lowerBound..<failureStart.lowerBound]

    #expect(successBranch.contains("try CredentialsStore.save(creds)"))
    #expect(successBranch.contains("не удалось безопасно сохранить"))
    #expect(successBranch.contains("exit(1)"))
    #expect(!successBranch.contains("creds.save"))
    #expect(
      successBranch.range(of: "exit(1)")!.lowerBound
        < successBranch.range(of: "спарено")!.lowerBound)
  }
}
