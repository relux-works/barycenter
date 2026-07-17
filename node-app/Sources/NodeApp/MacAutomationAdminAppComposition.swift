import AppKit
import Foundation
import NodeAppUI
import NodeCore

@MainActor
final class MacAutomationAdminAppComposition {
    private let admin: any AutomationAdminServicing
    private let phaseOne: PhaseOneAppClient
    private let model: PulsarShellModel
    private let clipboard: MacAutomationSecretClipboard
    private let now: () -> Date
    private var feature: AutomationFeature?
    private var cues: [SoundboardCue] = []
    private var schedules: [AutomationSchedule] = []
    private var principals: [AutomationPrincipal] = []
    private var history: [PhaseOneHistoryItem] = []
    private var selectedScheduleID: String?
    private var selectedPrincipalID: String?
    private var selectedHistoryID: String?
    private var secret: AutomationPrincipalSecret?
    private var task: Task<Void, Never>?
    private var epoch: UInt64 = 0
    private var busy = false
    private var stopped = false
    private var confirmation: PulsarAutomationConfirmation?
    private var outcome: String?
    private var failure: String? = "refresh_pending"
    private var lastRefresh = Date.distantPast

    init(bundle: CredentialBundle, model: PulsarShellModel) throws {
        admin = try AutomationAdminClient(bundle: bundle)
        phaseOne = try PhaseOneAppClient(bundle: bundle)
        self.model = model
        clipboard = MacAutomationSecretClipboard()
        now = Date.init
    }

    func start() { refresh(force: true) }

    func refresh(force: Bool = false) {
        guard !stopped, !busy else { return }
        let instant = now()
        guard force || instant.timeIntervalSince(lastRefresh) >= 15 else { return }
        lastRefresh = instant
        task?.cancel()
        let refreshEpoch = epoch
        task = Task { [weak self] in
            guard let self else { return }
            do {
                async let featureValue = admin.feature()
                async let cueValue = phaseOne.soundboardCues()
                async let scheduleValue = admin.schedules()
                async let principalValue = admin.principals()
                async let historyValue = phaseOne.history(limit: 30)
                let loaded = try await (
                    featureValue, cueValue, scheduleValue, principalValue, historyValue)
                guard !Task.isCancelled, !stopped, epoch == refreshEpoch else { return }
                feature = loaded.0
                cues = loaded.1.cues
                schedules = loaded.2
                principals = loaded.3
                history = loaded.4.items.filter { $0.automation != nil || $0.actions.contains("cancel") }
                normalizeSelection()
                failure = nil
                publish()
            } catch {
                guard !Task.isCancelled, !stopped, epoch == refreshEpoch else { return }
                clearAuthorizationProjection(failure: failureCode(error))
            }
        }
    }

    func selectSchedule(_ id: String?) {
        guard id == nil || schedules.contains(where: { $0.id == id }) else { return }
        selectedScheduleID = id
        publish()
    }

    func selectPrincipal(_ id: String) {
        guard principals.contains(where: { $0.id == id }) else { return }
        selectedPrincipalID = id
        publish()
    }

    func selectHistory(_ id: String) {
        guard history.contains(where: { $0.id == id }) else { return }
        selectedHistoryID = id
        publish()
    }

    func saveFeature(timezone: String, quietHours: String) {
        guard var feature,
              AutomationScheduleRules.validTimezone(timezone),
              let quiet = try? AutomationScheduleRules.parseQuietHours(quietHours) else {
            setFailure("invalid_quiet_hours")
            return
        }
        feature.timezone = timezone
        feature.quietHours = quiet
        mutate(kind: "feature", success: "feature_saved") { [admin] key in
            _ = try await admin.replaceFeature(feature, idempotencyKey: key)
        }
    }

    func saveSchedule(_ editor: PulsarAutomationScheduleEditor) {
        let cleanName = editor.name.trimmingCharacters(in: .whitespacesAndNewlines)
        guard !cleanName.isEmpty, cleanName.utf8.count <= 128,
              AutomationScheduleRules.validTimezone(editor.timezone),
              AutomationScheduleRules.validLocalTime(editor.localTime),
              let weekdays = try? AutomationScheduleRules.parseWeekdays(editor.weekdays),
              let quiet = try? AutomationScheduleRules.parseQuietHours(editor.quietHours),
              let feature, let firstCue = cues.first else {
            setFailure("invalid_schedule")
            return
        }
        let current = schedules.first { $0.id == selectedScheduleID }
        let draft = AutomationScheduleDraft(
            cueID: current?.cueID ?? firstCue.id,
            displayName: cleanName,
            timezone: editor.timezone,
            weekdays: weekdays,
            localTime: editor.localTime,
            audience: current?.audience ?? .ownBarycenter,
            airID: current?.airID,
            additionalQuietHours: quiet,
            policyRevision: feature.revision)
        mutate(
            kind: "schedule-save",
            success: current == nil ? "schedule_created_disarmed" : "schedule_saved"
        ) { [admin] key in
            if let current {
                _ = try await admin.replaceSchedule(current, with: draft, idempotencyKey: key)
            } else {
                _ = try await admin.createSchedule(draft, idempotencyKey: key)
            }
        }
    }

    func request(_ action: PulsarAutomationConfirmation) {
        guard !busy else { return }
        confirmation = action
        outcome = nil
        failure = nil
        publish()
    }

    func cancelConfirmation() {
        confirmation = nil
        publish()
    }

    func confirm(principalName: String) {
        guard let action = confirmation else { return }
        switch action {
        case .scheduleToggle:
            guard let current = schedules.first(where: { $0.id == selectedScheduleID }) else {
                setFailure("schedule_unavailable")
                return
            }
            mutate(kind: "schedule-toggle", success: current.enabled ? "schedule_disabled" : "schedule_enabled") { [admin] key in
                _ = try await admin.setScheduleEnabled(
                    current, enabled: !current.enabled, idempotencyKey: key)
            }
        case .scheduleDelete:
            guard let current = schedules.first(where: { $0.id == selectedScheduleID }) else {
                setFailure("schedule_unavailable")
                return
            }
            mutate(kind: "schedule-delete", success: "schedule_deleted") { [admin] key in
                _ = try await admin.deleteSchedule(current, idempotencyKey: key)
            }
        case .principalIssue:
            let name = principalName.trimmingCharacters(in: .whitespacesAndNewlines)
            guard !name.isEmpty, name.utf8.count <= 128, let firstCue = cues.first else {
                setFailure("invalid_principal")
                return
            }
            let draft = AutomationPrincipalDraft(
                displayName: name, allowedCueIDs: [firstCue.id],
                expiresAt: now().addingTimeInterval(30 * 24 * 3_600))
            mutate(
                kind: "principal-issue",
                success: "principal_issued",
                operation: { [admin] key in
                    try await admin.issuePrincipal(draft, idempotencyKey: key)
                },
                apply: { [weak self] issue in
                    guard let self else { return }
                    secret = issue.secret
                    outcome = issue.secretAvailable
                        ? "principal_issued_secret_once" : "principal_issued_secret_unavailable"
                })
        case .principalRevoke:
            guard let current = principals.first(where: { $0.id == selectedPrincipalID }) else {
                setFailure("principal_unavailable")
                return
            }
            mutate(kind: "principal-revoke", success: "principal_revoked") { [admin] key in
                _ = try await admin.revokePrincipal(current, idempotencyKey: key)
            }
        case .automationToggle:
            guard var feature else {
                setFailure("automation_unavailable")
                return
            }
            let wasEnabled = feature.automationEnabled
            feature.automationEnabled.toggle()
            if feature.automationEnabled { feature.emergencyDisabled = false }
            mutate(
                kind: "automation-toggle",
                success: wasEnabled
                    ? "automation_disabled_manual_soundboard_available" : "automation_enabled"
            ) { [admin] key in
                _ = try await admin.replaceFeature(feature, idempotencyKey: key)
            }
        case .emergencyDisable:
            guard var feature else {
                setFailure("automation_unavailable")
                return
            }
            feature.emergencyDisabled = true
            mutate(kind: "emergency-disable", success: "automation_emergency_disabled") { [admin] key in
                _ = try await admin.replaceFeature(feature, idempotencyKey: key)
            }
        case .historyCancel:
            guard let current = history.first(where: { $0.id == selectedHistoryID }),
                  current.actions.contains("cancel") else {
                setFailure("history_action_unavailable")
                return
            }
            mutate(kind: "history-cancel", success: "pending_cancelled") { [admin] _ in
                try await admin.cancelHistory(current.id)
            }
        }
    }

    func copySecret() {
        guard let secret else {
            setFailure("secret_unavailable")
            return
        }
        do {
            try clipboard.copy(secret.revealForExplicitCopy(), timeToLive: 60)
            outcome = "secret_copied_auto_clear"
            failure = nil
            publish()
        } catch { setFailure("secret_copy_failed") }
    }

    func hideSecret() {
        secret = nil
        clipboard.clear()
        outcome = "secret_hidden_not_recoverable"
        failure = nil
        publish()
    }

    func shutdown() {
        guard !stopped else { return }
        stopped = true
        epoch &+= 1
        task?.cancel()
        task = nil
        secret = nil
        clipboard.clear()
        model.setAutomation(.init())
    }

    private func mutate(
        kind: String, success: String,
        operation: @escaping @MainActor (String) async throws -> Void
    ) {
        mutate(kind: kind, success: success, operation: operation) { _ in }
    }

    private func mutate<Result>(
        kind: String, success: String,
        operation: @escaping @MainActor (String) async throws -> Result,
        apply: @escaping @MainActor (Result) -> Void
    ) {
        guard !busy, !stopped else { return }
        busy = true
        epoch &+= 1
        let mutationEpoch = epoch
        confirmation = nil
        outcome = nil
        failure = nil
        task?.cancel()
        publish()
        let key = "mac-automation-\(kind)-\(UUID().uuidString.lowercased())"
        task = Task { [weak self] in
            guard let self else { return }
            do {
                let result = try await operation(key)
                guard !Task.isCancelled, !stopped, epoch == mutationEpoch else { return }
                apply(result)
                busy = false
                if outcome == nil { outcome = success }
                publish()
                refresh(force: true)
            } catch {
                guard !Task.isCancelled, !stopped, epoch == mutationEpoch else { return }
                busy = false
                if isAuthorizationFailure(error) {
                    clearAuthorizationProjection(failure: failureCode(error))
                } else {
                    failure = failureCode(error)
                    publish()
                }
            }
        }
    }

    private func normalizeSelection() {
        if !schedules.contains(where: { $0.id == selectedScheduleID }) {
            selectedScheduleID = schedules.first?.id
        }
        if !principals.contains(where: { $0.id == selectedPrincipalID }) {
            selectedPrincipalID = principals.first?.id
        }
        if !history.contains(where: { $0.id == selectedHistoryID }) {
            selectedHistoryID = history.first?.id
        }
    }

    private func publish() {
        guard let feature else {
            var state = PulsarAutomationState()
            state.busy = busy
            state.confirmation = confirmation
            state.outcome = outcome
            state.failure = failure
            model.setAutomation(state)
            return
        }
        var state = PulsarAutomationState()
        state.available = true
        state.feature = .init(
            soundboardEnabled: feature.soundboardEnabled,
            automationEnabled: feature.automationEnabled,
            emergencyDisabled: feature.emergencyDisabled,
            timezone: feature.timezone,
            quietHours: AutomationScheduleRules.formatQuietHours(feature.quietHours),
            policyVersion: feature.policyVersion,
            revision: feature.revision)
        state.cueCount = cues.count
        state.schedules = schedules.map { schedule in
            let next = AutomationScheduleRules.nextRun(for: schedule, after: now())
            let allQuiet = feature.quietHours + schedule.additionalQuietHours
            return PulsarAutomationScheduleState(
                id: schedule.id, cueID: schedule.cueID, displayName: schedule.displayName,
                timezone: schedule.timezone,
                weekdays: AutomationScheduleRules.formatWeekdays(schedule.weekdays),
                localTime: schedule.localTime,
                quietHours: AutomationScheduleRules.formatQuietHours(schedule.additionalQuietHours),
                audience: schedule.audience.rawValue, enabled: schedule.enabled,
                nextRun: next,
                quietHoursSkip: next.map {
                    AutomationScheduleRules.isQuiet(
                        at: $0, timezone: schedule.timezone, windows: allQuiet)
                } ?? false,
                revision: schedule.revision)
        }
        state.principals = principals.map {
            .init(
                id: $0.id, displayName: $0.displayName, permission: $0.permission,
                allowedCueCount: $0.allowedCueIDs.count,
                allowedAudiences: $0.allowedAudiences.map(\.rawValue),
                expiresAt: $0.expiresAt, revoked: $0.revokedAt != nil,
                revision: $0.revision)
        }
        state.history = history.map { item in
            .init(
                id: item.id, title: item.title, status: item.status,
                triggerKind: item.automation?.triggerKind ?? "pending_delivery",
                actorLabel: item.automation?.principalLabel,
                scheduleLabel: item.automation?.scheduleLabel,
                reasonCode: item.automation?.reasonCode ?? item.reasonCode,
                occurredAt: item.occurredAt, canCancel: item.actions.contains("cancel"))
        }
        state.selectedScheduleID = selectedScheduleID
        state.selectedPrincipalID = selectedPrincipalID
        state.selectedHistoryID = selectedHistoryID
        state.secretAvailable = secret != nil
        state.busy = busy
        state.confirmation = confirmation
        state.outcome = outcome
        state.failure = failure
        model.setAutomation(state)
    }

    private func clearAuthorizationProjection(failure code: String) {
        feature = nil
        cues = []
        schedules = []
        principals = []
        history = []
        selectedScheduleID = nil
        selectedPrincipalID = nil
        selectedHistoryID = nil
        secret = nil
        clipboard.clear()
        busy = false
        confirmation = nil
        outcome = nil
        failure = code
        publish()
    }

    private func setFailure(_ code: String) {
        failure = code
        outcome = nil
        confirmation = nil
        publish()
    }

    private func isAuthorizationFailure(_ error: Error) -> Bool {
        guard case .rejected(let status, let code, _) = error as? PhaseOneClientError else {
            return false
        }
        return status == 401 || status == 403
            || ["unauthorized", "credential_invalid", "insufficient_capability"].contains(code)
    }

    private func failureCode(_ error: Error) -> String {
        guard let value = error as? PhaseOneClientError else { return "coordinator_unavailable" }
        switch value {
        case .invalidConfiguration: return "credential_unavailable"
        case .invalidRequest: return "invalid_request"
        case .transport: return "coordinator_unavailable"
        case .redirectRejected: return "redirect_rejected"
        case .responseTooLarge: return "response_too_large"
        case .invalidResponse: return "invalid_response"
        case .rejected(_, let code, _): return code
        }
    }
}

@MainActor
private final class MacAutomationSecretClipboard {
    private let pasteboard: NSPasteboard
    private var payload: String?
    private var changeCount: Int?
    private var clearTask: Task<Void, Never>?

    init(pasteboard: NSPasteboard = .general) { self.pasteboard = pasteboard }

    func copy(_ value: String, timeToLive: TimeInterval) throws {
        guard timeToLive > 0, timeToLive <= 60 else { throw PhaseOneClientError.invalidRequest }
        clearTask?.cancel()
        clearTask = nil
        payload = nil
        changeCount = nil
        pasteboard.clearContents()
        guard pasteboard.setString(value, forType: .string) else {
            throw PhaseOneClientError.invalidRequest
        }
        payload = value
        changeCount = pasteboard.changeCount
        clearTask = Task { [weak self] in
            try? await Task.sleep(for: .seconds(timeToLive))
            guard !Task.isCancelled else { return }
            self?.clearIfUnchanged()
        }
    }

    func clear() {
        clearTask?.cancel()
        clearTask = nil
        clearIfUnchanged()
    }

    private func clearIfUnchanged() {
        defer {
            payload = nil
            changeCount = nil
        }
        guard let payload, let changeCount,
              pasteboard.changeCount == changeCount,
              pasteboard.string(forType: .string) == payload else { return }
        pasteboard.clearContents()
    }
}
