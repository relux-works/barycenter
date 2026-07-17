import Foundation
import Testing

@Suite("macOS capture quality UI source contracts")
struct PulsarCaptureQualityUISourceTests {
    private var repositoryRoot: URL {
        URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
    }

    @Test("Visible capture controls use native buttons, keyboard stop, and explicit accessibility semantics")
    func visibleLocalControls() throws {
        let source = try String(contentsOf:
            repositoryRoot.appendingPathComponent(
                "node-app/Sources/NodeAppUI/PulsarCaptureQualityView.swift"),
            encoding: .utf8)
        #expect(source.contains("Button(copy.text(.captureStopLocal), action: actions.stopActiveCapture)"))
        #expect(source.contains(".keyboardShortcut(\".\", modifiers: .command)"))
        #expect(source.contains(".accessibilityLabel(copy.text(.captureQuality))"))
        #expect(source.contains(".accessibilityValue("))
        #expect(source.contains(".accessibilityHint(copy.text(.degradedCaptureHelp))"))
        #expect(!source.contains(".onTapGesture"))

        let shell = try String(contentsOf:
            repositoryRoot.appendingPathComponent(
                "node-app/Sources/NodeAppUI/PulsarMainWindow.swift"),
            encoding: .utf8)
        #expect(shell.contains(".onExitCommand"))
        #expect(shell.contains("actions.stopActiveCapture()"))
    }

    @Test("Capture mode and Stop remain local actions with no coordinator start seam")
    func noRemoteStartAuthority() throws {
        let source = try String(contentsOf:
            repositoryRoot.appendingPathComponent("node-app/Sources/NodeApp/main.swift"),
            encoding: .utf8)
        let start = try #require(source.range(of: "setCaptureQuality: {"))
        let end = try #require(source.range(
            of: "playBuiltinCue:", range: start.upperBound..<source.endIndex))
        let captureActions = String(source[start.lowerBound..<end.lowerBound])
        #expect(captureActions.contains("macCaptureComposition?.setCaptureQualityMode"))
        #expect(captureActions.contains("macCaptureComposition?.stopActiveCapture()"))
        #expect(!captureActions.localizedCaseInsensitiveContains("coordinator"))
        #expect(!captureActions.contains("runtime?."))

        let menu = try String(contentsOf:
            repositoryRoot.appendingPathComponent("node-app/Sources/NodeApp/StatusMenu.swift"),
            encoding: .utf8)
        #expect(menu.contains("action: #selector(stopActiveCapture)"))
        #expect(menu.contains("shellActions.stopActiveCapture()"))
    }
}
