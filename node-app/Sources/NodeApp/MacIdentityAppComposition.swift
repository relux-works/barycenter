import AppKit
import Foundation
import NodeAppUI
import NodeCore

/// Self-service Create/Join binding for the macOS shell. Credential writes are
/// delegated to the reviewed onboarding service. A newly created orbit is not
/// activated until the one-time recovery payload is explicitly saved and the
/// protected metadata acknowledges that backup.
@MainActor
final class MacIdentityAppComposition {
    private static let attemptIDKey = "identity.create.attempt-id.v1"
    private static let attemptTitleKey = "identity.create.attempt-title.v1"

    private let service: OnboardingService
    private let model: PulsarShellModel
    private let defaults: UserDefaults
    private let onCredentialsActivated: () -> Void
    private var pendingCreatedOrbit: CreatedOrbit?
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
                pendingCreatedOrbit = outcome.value
                defaults.removeObject(forKey: Self.attemptIDKey)
                defaults.removeObject(forKey: Self.attemptTitleKey)
                model.setIdentityOperation(.recoveryExportRequired(""))
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
        guard let created = pendingCreatedOrbit else {
            model.setIdentityOperation(.failed("Recovery material is no longer available"))
            return
        }
        let panel = NSSavePanel()
        panel.nameFieldStringValue = "barycenter-recovery.json"
        panel.canCreateDirectories = true
        panel.isExtensionHidden = false
        panel.begin { [weak self] result in
            guard let self, result == .OK, let url = panel.url else { return }
            do {
                try RecoveryExportHelper.save(created.recovery, to: url)
                try service.acknowledgeRecoveryBackup(
                    actorID: created.recovery.actorId,
                    recoveryID: created.recovery.recoveryId)
                pendingCreatedOrbit = nil
                model.setIdentityOperation(.succeeded(created.title))
                onCredentialsActivated()
            } catch {
                model.setIdentityOperation(.failed("Recovery export failed"))
            }
        }
    }

    func shutdown() {
        operation?.cancel()
        operation = nil
        // Deliberately retain pending one-time material only for this live
        // composition. Replacing it before export would make activation unsafe.
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
