import Foundation
import Testing
@testable import NodeAppUI

@Suite("macOS desktop experience contracts")
struct PulsarDesktopStyleTests {
    @Test("Window and sidebar metrics preserve a usable desktop hierarchy")
    func desktopMetrics() {
        #expect(PulsarDesktopMetrics.minimumWindowWidth >= 900)
        #expect(PulsarDesktopMetrics.minimumWindowHeight >= 640)
        #expect(PulsarDesktopMetrics.defaultWindowWidth > PulsarDesktopMetrics.minimumWindowWidth)
        #expect(PulsarDesktopMetrics.defaultWindowHeight > PulsarDesktopMetrics.minimumWindowHeight)
        #expect(PulsarDesktopMetrics.sidebarMinimumWidth >= 200)
        #expect(PulsarDesktopMetrics.sidebarIdealWidth > PulsarDesktopMetrics.sidebarMinimumWidth)
        #expect(PulsarDesktopMetrics.sidebarMaximumWidth > PulsarDesktopMetrics.sidebarIdealWidth)
    }

    @Test("Every status tone has an explicit non-color symbol")
    func statusSemantics() {
        let tones = PulsarStatusTone.allCases
        #expect(Set(tones.map(\.symbol)).count == tones.count)
        #expect(tones.allSatisfy { !$0.symbol.isEmpty })
        #expect(PulsarStatusTone.failure.symbol.contains("xmark"))
        #expect(PulsarStatusTone.warning.symbol.contains("exclamationmark"))
    }

    @Test("Production shell keeps native navigation, focus, accessibility, and previews")
    func shellSourceContract() throws {
        let source = try String(
            contentsOf: repositoryRoot.appendingPathComponent(
                "node-app/Sources/NodeAppUI/PulsarMainWindow.swift"),
            encoding: .utf8)
        #expect(source.contains("NavigationSplitView(columnVisibility:"))
        #expect(source.contains(".listStyle(.sidebar)"))
        #expect(source.contains(".toolbarRole(.editor)"))
        #expect(source.contains("@FocusState private var fieldFocused"))
        #expect(source.contains(".accessibilityLabel"))
        #expect(source.contains("PulsarStatusMessage("))
        #expect(source.contains("PulsarMainViewPreviews"))
        #expect(source.contains(".preferredColorScheme(.dark)"))
        #expect(source.contains("target.setFrameAutosaveName(\"PulsarMainWindow\")"))
    }

    private var repositoryRoot: URL {
        URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
    }
}
