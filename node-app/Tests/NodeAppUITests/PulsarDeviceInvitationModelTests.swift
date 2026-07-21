import Foundation
import Testing

@testable import NodeAppUI

@Suite("Device invitation presentation")
struct PulsarDeviceInvitationModelTests {
    private let canary = "ABCDEFGHJKMNPQRSTVWXYZ23456"

    @MainActor
    @Test("One-time code is live-only and absent from snapshots and reflection")
    func liveOnlySecretProjection() throws {
        let model = PulsarDeviceInvitationModel()
        model.authorizePrimary()
        #expect(model.beginGeneration())
        model.show(
            code: canary,
            role: .companion,
            expiresAt: Date(timeIntervalSince1970: 1_900_000_000))

        #expect(model.visibleCode == canary)
        #expect(model.snapshot.invitation?.role == .companion)
        #expect(!String(describing: model.snapshot).contains(canary))
        #expect(!String(reflecting: model.snapshot).contains(canary))
        #expect(!String(describing: model).contains(canary))
        #expect(!String(reflecting: model).contains(canary))
        for child in Mirror(reflecting: model).children {
            #expect(!String(reflecting: child.value).contains(canary))
        }

        model.hide()
        #expect(model.visibleCode == nil)
        #expect(model.snapshot.invitation == nil)
        #expect(model.snapshot.feedback == .hidden)
        #expect(!String(reflecting: model).contains(canary))
    }

    @MainActor
    @Test("Generation is single-flight while an invitation is visible")
    func singleFlightGeneration() {
        let model = PulsarDeviceInvitationModel()
        #expect(!model.beginGeneration())
        model.authorizePrimary()
        #expect(model.beginGeneration())
        #expect(!model.beginGeneration())
        model.show(
            code: canary,
            role: .companion,
            expiresAt: Date(timeIntervalSince1970: 1_900_000_000))
        #expect(!model.beginGeneration())
        model.hide()
        #expect(model.beginGeneration())
    }

    @Test("Every invitation status has distinct English and Russian copy")
    func localizedStatusCopy() {
        let failures: [PulsarDeviceInvitationFailure] = [
            .notActivated, .primaryRequired, .unauthorized, .insufficientCapability,
            .rateLimited(seconds: 17), .serviceUnavailable, .invalidResponse,
            .copyFailed, .cleanupFailed,
        ]
        let en = PulsarDeviceInvitationCopy(locale: .en)
        let ru = PulsarDeviceInvitationCopy(locale: .ru)
        for failure in failures {
            #expect(!en.failure(failure).isEmpty)
            #expect(!ru.failure(failure).isEmpty)
            #expect(en.failure(failure) != ru.failure(failure))
        }
        #expect(en.companion != ru.companion)
        #expect(en.expired != ru.expired)
        #expect(en.copied != ru.copied)
    }

    @MainActor
    @Test("Shell actions expose explicit invitation and optional pairing seams")
    func actionSeams() {
        var calls: [String] = []
        let actions = PulsarShellActions(
            refreshDeviceInvitationAuthorization: { calls.append("refresh") },
            generateDeviceInvitation: { calls.append("generate") },
            copyDeviceInvitation: { calls.append("copy") },
            hideDeviceInvitation: { calls.append("hide") },
            openOptionalTelegramPairing: { calls.append("telegram") })

        actions.refreshDeviceInvitationAuthorization()
        actions.generateDeviceInvitation()
        actions.copyDeviceInvitation()
        actions.hideDeviceInvitation()
        actions.openOptionalTelegramPairing()
        #expect(calls == ["refresh", "generate", "copy", "hide", "telegram"])
    }

    @Test("SwiftUI source hides the code from accessibility and marks it sensitive")
    func sourcePrivacyContract() throws {
        let root = URL(fileURLWithPath: #filePath)
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
            .deletingLastPathComponent()
        let source = try String(
            contentsOf: root.appendingPathComponent(
                "node-app/Sources/NodeAppUI/PulsarDeviceInvitationView.swift"),
            encoding: .utf8)
        #expect(source.contains(".privacySensitive()"))
        #expect(source.contains(".accessibilityHidden(true)"))
        #expect(!source.contains("accessibilityValue(code"))
        #expect(source.contains("Button(copy.copyCode)"))
        #expect(source.contains("Button(copy.hideCode)"))
        #expect(source.contains("keyboardShortcut"))
    }
}
