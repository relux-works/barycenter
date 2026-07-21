import Foundation
import Testing

@Suite("macOS first-run source lifecycle")
struct MacFirstRunLifecycleSourceTests {
    @Test("Fresh GUI launch enters the main Create and Join shell")
    func freshLaunchUsesMainShell() throws {
        let main = try source("node-app/Sources/NodeApp/main.swift")
        let unpairedStart = try #require(
            main.range(of: "if config.coordinator.token.isEmpty {")?.lowerBound)
        let unpairedEnd = try #require(
            main.range(of: "startCore(with: config)", range: unpairedStart..<main.endIndex)?
                .lowerBound)
        let unpaired = String(main[unpairedStart..<unpairedEnd])

        #expect(unpaired.contains("mainWindow.show(section: .home)"))
        #expect(!unpaired.contains("onboarding.show"))
        #expect(main.contains("createOrbit: { showShellSection(.create) }"))
        #expect(main.contains("joinOrbit: { showShellSection(.join) }"))
        #expect(main.contains("openOptionalTelegramPairing"))
        #expect(main.contains("promptForNetwork: true"))

        let legacy = try source("node-app/Sources/NodeApp/OnboardingWindow.swift")
        #expect(legacy.contains("Legacy optional Telegram pairing"))
    }

    @Test("Recovery acknowledgement gates activation and incomplete retries remain pinned")
    func recoveryGateSourceContract() throws {
        let main = try source("node-app/Sources/NodeApp/main.swift")
        let identity = try source(
            "node-app/Sources/NodeApp/MacIdentityAppComposition.swift")
        let credentials = try source(
            "node-app/Sources/NodeCore/OnboardingCredentials.swift")

        #expect(main.contains("activationEligibleCredentials()"))
        #expect(main.contains("hasUnacknowledgedRecovery()"))
        #expect(credentials.contains("recovery?.explicitBackupAcknowledged != false"))
        #expect(identity.contains("pendingCreatedOrbit == nil"))
        #expect(identity.contains("operation == nil"))
        #expect(identity.contains("recoveryUnavailableAfterRelaunch"))
        let acknowledgement = try #require(
            identity.range(of: "acknowledgeRecoveryBackup")?.lowerBound)
        let attemptRemoval = try #require(
            identity.range(of: "defaults.removeObject", range: acknowledgement..<identity.endIndex)?
                .lowerBound)
        #expect(attemptRemoval > acknowledgement)
    }

    @Test("Invitation sources have no durable or logging sink for the code")
    func noSecretPersistenceOrLoggingSink() throws {
        let files = [
            "node-app/Sources/NodeAppComposition/MacDeviceInvitationAppComposition.swift",
            "node-app/Sources/NodeAppUI/PulsarDeviceInvitationModel.swift",
            "node-app/Sources/NodeCore/DeviceInvitationPasteboard.swift",
        ]
        for file in files {
            let contents = try source(file)
            for forbidden in ["UserDefaults", "Logger", "NSLog(", "print(", "Codable"] {
                #expect(!contents.contains(forbidden), "Unexpected \(forbidden) sink in \(file)")
            }
        }
        let model = try source(
            "node-app/Sources/NodeAppUI/PulsarDeviceInvitationModel.swift")
        #expect(model.contains("@ObservationIgnored private var secret"))
        #expect(model.contains("bytes.resetBytes"))
        #expect(model.contains("secret: <redacted>"))
    }

    private func source(_ relativePath: String) throws -> String {
        let root = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
        return try String(
            contentsOf: root.appendingPathComponent(relativePath),
            encoding: .utf8)
    }
}
