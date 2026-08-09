import Foundation
import Testing

@Suite("macOS ordinary recording UI source contracts")
struct PulsarCaptureQualityUISourceTests {
    private var repositoryRoot: URL {
        URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
    }

    @Test("Visible recording controls use native buttons, keyboard stop, and explicit accessibility semantics")
    func visibleLocalControls() throws {
        let source = try String(
            contentsOf:
                repositoryRoot.appendingPathComponent(
                    "node-app/Sources/NodeAppUI/PulsarRecordingView.swift"),
            encoding: .utf8)
        #expect(source.contains("Button(copy.text(.captureStopLocal), action: actions.stopActiveCapture)"))
        #expect(source.contains(".keyboardShortcut(\".\", modifiers: .command)"))
        #expect(source.contains(".accessibilityLabel(copy.text(.recording))"))
        #expect(source.contains(".accessibilityValue("))
        #expect(source.contains(".accessibilityHint(copy.text(.recordingHelp))"))
        #expect(!source.contains(".onTapGesture"))
        for forbidden in [
            "captureQuality",
            "degradedCapture",
            "AEC",
            "Echo cancellation",
            "VPIO",
            "output route",
        ] {
            #expect(!source.localizedCaseInsensitiveContains(forbidden))
        }

        let shell = try String(
            contentsOf:
                repositoryRoot.appendingPathComponent(
                    "node-app/Sources/NodeAppUI/PulsarMainWindow.swift"),
            encoding: .utf8)
        #expect(shell.contains(".onExitCommand"))
        #expect(shell.contains("actions.stopActiveCapture()"))
    }

    @Test("Record and Stop remain local actions with no processing or coordinator seam")
    func noRemoteStartAuthority() throws {
        let source = try String(
            contentsOf:
                repositoryRoot.appendingPathComponent("node-app/Sources/NodeApp/main.swift"),
            encoding: .utf8)
        let start = try #require(source.range(of: "stopActiveCapture: {"))
        let end = try #require(
            source.range(
                of: "playBuiltinCue:", range: start.upperBound..<source.endIndex))
        let captureActions = String(source[start.lowerBound..<end.lowerBound])
        #expect(captureActions.contains("macCaptureComposition?.stopActiveCapture()"))
        #expect(!source.contains("macCaptureComposition?.setCaptureQualityMode"))
        #expect(!source.contains("macCaptureComposition?.resolveCaptureConsent"))
        #expect(!captureActions.localizedCaseInsensitiveContains("coordinator"))
        #expect(!captureActions.contains("runtime?."))

        let menu = try String(
            contentsOf:
                repositoryRoot.appendingPathComponent("node-app/Sources/NodeApp/StatusMenu.swift"),
            encoding: .utf8)
        #expect(menu.contains("action: #selector(stopActiveCapture)"))
        #expect(menu.contains("shellActions.stopActiveCapture()"))
        #expect(!menu.contains("PulsarCaptureQualityPresentation"))
    }
}
