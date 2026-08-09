import AppKit
import Foundation
import NodeAppUI
import NodeCore

/// Self-service Barycenter/device binding for the macOS shell. Credential
/// writes are delegated to the reviewed onboarding service. Recovery export is
/// a persistent safety action, not a gate that prevents the new device from
/// becoming usable.
@MainActor
final class MacIdentityAppComposition {
    private static let attemptIDKey = "identity.create.attempt-id.v1"
    private static let attemptTitleKey = "identity.create.attempt-title.v1"

    private let service: OnboardingService
    private let model: PulsarShellModel
    private let defaults: UserDefaults
    private let onCredentialsActivated: () -> Void
    private var pendingRecovery: OneTimeRecoveryMaterial?
    private var pendingRecoveryTitle = ""
    private var operation: Task<Void, Never>?

    init(
        coordinator: String,
        model: PulsarShellModel,
        defaults: UserDefaults = .standard,
        onCredentialsActivated: @escaping () -> Void
    ) throws {
        service = OnboardingService(client: try OnboardingHTTPClient(coordinator: coordinator))
        self.model = model
        self.defaults = defaults
        self.onCredentialsActivated = onCredentialsActivated
    }

    func create(title: String) {
        let clean = title.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !clean.isEmpty else { return }
        operation?.cancel()
        model.setIdentityOperation(.busy)
        let attemptID = createAttemptID(for: clean)
        operation = Task { [weak self] in
            guard let self else { return }
            do {
                let outcome = try await service.createOrbit(
                    title: clean,
                    installationAttemptID: attemptID)
                guard !Task.isCancelled else { return }
                guard outcome.wasStored else {
                    model.setIdentityOperation(.failed("Protected credential storage failed"))
                    return
                }
                pendingRecovery = outcome.value.recovery
                pendingRecoveryTitle = outcome.value.title
                defaults.removeObject(forKey: Self.attemptIDKey)
                defaults.removeObject(forKey: Self.attemptTitleKey)
                model.setIdentityOperation(.recoveryExportRequired(""))
                onCredentialsActivated()
            } catch {
                guard !Task.isCancelled else { return }
                model.setIdentityOperation(.failed(identityFailure(error)))
            }
        }
    }

    func join(code: String) {
        let clean = code.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !clean.isEmpty else { return }
        operation?.cancel()
        model.setIdentityOperation(.busy)
        operation = Task { [weak self] in
            guard let self else { return }
            do {
                let outcome = try await service.joinOrbit(inviteCode: clean)
                guard !Task.isCancelled else { return }
                guard outcome.wasStored else {
                    model.setIdentityOperation(.failed("Protected credential storage failed"))
                    return
                }
                model.setIdentityOperation(.succeeded(outcome.value.title))
                onCredentialsActivated()
            } catch {
                guard !Task.isCancelled else { return }
                model.setIdentityOperation(.failed(identityFailure(error)))
            }
        }
    }

    func exportRecovery() {
        if let pendingRecovery {
            presentRecovery(pendingRecovery, title: pendingRecoveryTitle)
            return
        }
        operation?.cancel()
        model.setIdentityOperation(.recoveryExportRequired("Preparing a fresh recovery file…"))
        operation = Task { [weak self] in
            guard let self else { return }
            do {
                let outcome = try await service.rotateRecovery()
                guard !Task.isCancelled else { return }
                guard outcome.wasStored else {
                    model.setIdentityOperation(.failed("Protected recovery metadata could not be saved"))
                    return
                }
                pendingRecovery = outcome.value
                pendingRecoveryTitle = ""
                presentRecovery(outcome.value, title: "")
            } catch {
                guard !Task.isCancelled else { return }
                model.setIdentityOperation(.failed(identityFailure(error)))
            }
        }
    }

    func issueDeviceInvite() {
        operation?.cancel()
        model.setDeviceInviteState(.init(busy: true))
        operation = Task { [weak self] in
            guard let self else { return }
            do {
                let invite = try await service.issueDeviceInvite()
                guard !Task.isCancelled else { return }
                model.setDeviceInviteState(.init(secret: .init(
                    code: invite.code,
                    expiresAt: invite.expiresAt)))
            } catch {
                guard !Task.isCancelled else { return }
                model.setDeviceInviteState(.init(failure: identityFailure(error)))
            }
        }
    }

    func hideDeviceInvite() {
        model.setDeviceInviteState(.init())
    }

    private func presentRecovery(_ material: OneTimeRecoveryMaterial, title: String) {
        let panel = NSSavePanel()
        panel.nameFieldStringValue = "barycenter-recovery.json"
        panel.canCreateDirectories = true
        panel.isExtensionHidden = false
        panel.begin { [weak self] result in
            guard let self else { return }
            guard result == .OK, let url = panel.url else {
                model.setIdentityOperation(.recoveryExportRequired(""))
                return
            }
            do {
                try RecoveryExportHelper.save(material, to: url)
                try service.acknowledgeRecoveryBackup(
                    actorID: material.actorId,
                    recoveryID: material.recoveryId)
                pendingRecovery = nil
                pendingRecoveryTitle = ""
                model.setIdentityOperation(.succeeded(title))
            } catch {
                model.setIdentityOperation(.recoveryExportRequired(
                    "Recovery export failed. Choose a destination and try again."))
            }
        }
    }

    func shutdown() {
        operation?.cancel()
        operation = nil
        // One-time material is retained only for this live composition. After
        // restart an authenticated primary can rotate it and resume export.
    }

    private func createAttemptID(for title: String) -> String {
        if defaults.string(forKey: Self.attemptTitleKey) == title,
           let existing = defaults.string(forKey: Self.attemptIDKey),
           !existing.isEmpty {
            return existing
        }
        let value = "mac-create-" + UUID().uuidString.lowercased()
        defaults.set(title, forKey: Self.attemptTitleKey)
        defaults.set(value, forKey: Self.attemptIDKey)
        return value
    }

    private func identityFailure(_ error: Error) -> String {
        if let localized = error as? LocalizedError,
           let description = localized.errorDescription,
           !description.isEmpty {
            return description
        }
        return "Identity service is temporarily unavailable"
    }
}
