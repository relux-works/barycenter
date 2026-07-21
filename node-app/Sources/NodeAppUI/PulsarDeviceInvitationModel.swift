import Foundation
import Observation

public enum PulsarDeviceInvitationRole: String, Equatable, Sendable {
    case companion
}

public enum PulsarDeviceInvitationFailure: Equatable, Sendable {
    case notActivated
    case primaryRequired
    case unauthorized
    case insufficientCapability
    case rateLimited(seconds: Int)
    case serviceUnavailable
    case invalidResponse
    case copyFailed
    case cleanupFailed
}

public enum PulsarDeviceInvitationAuthorization: Equatable, Sendable {
    case notChecked
    case checking
    case authorizedPrimary
    case unavailable(PulsarDeviceInvitationFailure)
}

public struct PulsarDeviceInvitationMetadata: Equatable, Sendable {
    public let role: PulsarDeviceInvitationRole
    public let expiresAt: Date

    public init(role: PulsarDeviceInvitationRole, expiresAt: Date) {
        self.role = role
        self.expiresAt = expiresAt
    }
}

public enum PulsarDeviceInvitationFeedback: Equatable, Sendable {
    case copied(autoClearAt: Date)
    case hidden
    case expired
    case failure(PulsarDeviceInvitationFailure)
}

/// Durable-safe presentation state. The one-time code is intentionally absent;
/// it lives only in the transient model storage below while the invitation is
/// visibly presented.
public struct PulsarDeviceInvitationSnapshot: Equatable, Sendable,
    CustomStringConvertible, CustomDebugStringConvertible
{
    public var authorization: PulsarDeviceInvitationAuthorization
    public var isGenerating: Bool
    public var invitation: PulsarDeviceInvitationMetadata?
    public var feedback: PulsarDeviceInvitationFeedback?

    public init(
        authorization: PulsarDeviceInvitationAuthorization = .notChecked,
        isGenerating: Bool = false,
        invitation: PulsarDeviceInvitationMetadata? = nil,
        feedback: PulsarDeviceInvitationFeedback? = nil
    ) {
        self.authorization = authorization
        self.isGenerating = isGenerating
        self.invitation = invitation
        self.feedback = feedback
    }

    public var description: String {
        let invitationState = invitation == nil ? "none" : "present"
        return
            "PulsarDeviceInvitationSnapshot(authorization: \(authorization), generating: \(isGenerating), invitation: \(invitationState), secret: <redacted>)"
    }

    public var debugDescription: String { description }
}

@MainActor
@Observable
public final class PulsarDeviceInvitationModel: CustomStringConvertible,
    CustomDebugStringConvertible, CustomReflectable
{
    public private(set) var snapshot: PulsarDeviceInvitationSnapshot
    @ObservationIgnored private var secret: TransientDeviceInvitationSecret?

    public init(snapshot: PulsarDeviceInvitationSnapshot = .init()) {
        self.snapshot = snapshot
    }

    public var visibleCode: String? {
        guard snapshot.invitation != nil else { return nil }
        return secret?.reveal()
    }

    nonisolated public var description: String {
        "PulsarDeviceInvitationModel(state: <redacted>, secret: <redacted>)"
    }

    nonisolated public var debugDescription: String { description }

    nonisolated public var customMirror: Mirror {
        Mirror(self, children: ["state": "<redacted>"], displayStyle: .class)
    }

    public func beginAuthorization() {
        clearSecret()
        snapshot = .init(authorization: .checking)
    }

    public func authorizePrimary() {
        clearSecret()
        snapshot = .init(authorization: .authorizedPrimary)
    }

    public func denyAuthorization(_ failure: PulsarDeviceInvitationFailure) {
        clearSecret()
        snapshot = .init(authorization: .unavailable(failure))
    }

    @discardableResult
    public func beginGeneration() -> Bool {
        guard snapshot.authorization == .authorizedPrimary,
            snapshot.invitation == nil,
            !snapshot.isGenerating
        else { return false }
        snapshot.isGenerating = true
        snapshot.feedback = nil
        return true
    }

    public func show(
        code: String,
        role: PulsarDeviceInvitationRole,
        expiresAt: Date
    ) {
        clearSecret()
        secret = TransientDeviceInvitationSecret(code)
        snapshot.isGenerating = false
        snapshot.invitation = .init(role: role, expiresAt: expiresAt)
        snapshot.feedback = nil
    }

    public func generationFailed(_ failure: PulsarDeviceInvitationFailure) {
        snapshot.isGenerating = false
        snapshot.feedback = .failure(failure)
    }

    public func markCopied(autoClearAt: Date) {
        guard snapshot.invitation != nil, secret != nil else { return }
        snapshot.feedback = .copied(autoClearAt: autoClearAt)
    }

    public func copyFailed(_ failure: PulsarDeviceInvitationFailure = .copyFailed) {
        guard snapshot.invitation != nil, secret != nil else { return }
        snapshot.feedback = .failure(failure)
    }

    public func hide(feedback: PulsarDeviceInvitationFeedback? = .hidden) {
        clearSecret()
        snapshot.isGenerating = false
        snapshot.invitation = nil
        snapshot.feedback = feedback
    }

    public func reset() {
        clearSecret()
        snapshot = .init()
    }

    private func clearSecret() {
        secret?.clear()
        secret = nil
    }
}

private final class TransientDeviceInvitationSecret {
    private var bytes: Data

    init(_ value: String) {
        bytes = Data(value.utf8)
    }

    deinit { clear() }

    func reveal() -> String {
        String(decoding: bytes, as: UTF8.self)
    }

    func clear() {
        bytes.resetBytes(in: bytes.startIndex..<bytes.endIndex)
    }
}

public struct PulsarDeviceInvitationCopy: Sendable {
    public let locale: PulsarShellLocale

    public init(locale: PulsarShellLocale) {
        self.locale = locale
    }

    public var title: String { localized("Invite another device", "Пригласить другое устройство") }
    public var intro: String {
        localized(
            "An authorized primary can issue a single one-time invitation for a companion installation.",
            "Авторизованное основное устройство может выпустить одно одноразовое приглашение для дополнительной установки."
        )
    }
    public var checking: String { localized("Checking authorization…", "Проверяю полномочия…") }
    public var authorized: String {
        localized("Primary authorization confirmed", "Полномочия основного устройства подтверждены")
    }
    public var refreshAuthorization: String {
        localized("Check authorization", "Проверить полномочия")
    }
    public var generate: String { localized("Generate invitation", "Создать приглашение") }
    public var generating: String { localized("Generating invitation…", "Создаю приглашение…") }
    public var invitationReady: String {
        localized("One-time invitation is ready", "Одноразовое приглашение готово")
    }
    public var codeVisible: String {
        localized(
            "The one-time code is visible on screen. Use Copy code to place it on the clipboard temporarily.",
            "Одноразовый код показан на экране. Нажмите «Скопировать код», чтобы временно поместить его в буфер обмена."
        )
    }
    public var intendedRole: String { localized("Role", "Роль") }
    public var companion: String { localized("Companion", "Дополнительное устройство") }
    public var expires: String { localized("Expires", "Действует до") }
    public var oneTimeWarning: String {
        localized(
            "The code is shown only in this live session. Hiding, closing, expiry, or relaunch makes this copy unavailable.",
            "Код показывается только в этой текущей сессии. После скрытия, закрытия, истечения срока или перезапуска эта копия недоступна."
        )
    }
    public var copyCode: String { localized("Copy code", "Скопировать код") }
    public var hideCode: String { localized("Hide code", "Скрыть код") }
    public var copied: String {
        localized(
            "Copied. Pulsar will clear this exact clipboard value automatically.",
            "Скопировано. Пульсар автоматически очистит именно это значение буфера обмена.")
    }
    public var hidden: String {
        localized(
            "Invitation hidden and no longer recoverable here.",
            "Приглашение скрыто и больше не восстанавливается здесь.")
    }
    public var expired: String {
        localized("The invitation expired and was hidden.", "Срок приглашения истёк, код скрыт.")
    }

    public func failure(_ failure: PulsarDeviceInvitationFailure) -> String {
        switch failure {
        case .notActivated:
            localized(
                "Activate this Pulsar before inviting another device.",
                "Активируйте этот Пульсар, прежде чем приглашать другое устройство.")
        case .primaryRequired:
            localized(
                "Only an authorized primary device can issue invitations.",
                "Приглашения может выпускать только авторизованное основное устройство.")
        case .unauthorized:
            localized(
                "Authorization is no longer valid. Join or recover this installation again.",
                "Авторизация больше недействительна. Повторно присоедините или восстановите эту установку."
            )
        case .insufficientCapability:
            localized(
                "This installation does not have invitation permission.",
                "У этой установки нет права выпускать приглашения.")
        case .rateLimited(let seconds):
            locale == .ru
                ? "Слишком много попыток. Повторите через \(seconds) с."
                : "Too many attempts. Try again in \(seconds) seconds."
        case .serviceUnavailable:
            localized(
                "Barycenter is temporarily unavailable. Try again.",
                "Барицентр временно недоступен. Повторите попытку.")
        case .invalidResponse:
            localized(
                "Barycenter returned an invalid invitation response.",
                "Барицентр вернул некорректный ответ на запрос приглашения.")
        case .copyFailed:
            localized(
                "The invitation could not be copied. The on-screen code was not hidden.",
                "Не удалось скопировать приглашение. Код на экране не был скрыт.")
        case .cleanupFailed:
            localized(
                "Automatic clipboard cleanup failed. Replace the clipboard contents manually.",
                "Не удалось автоматически очистить буфер обмена. Замените его содержимое вручную.")
        }
    }

    private func localized(_ en: String, _ ru: String) -> String {
        locale == .ru ? ru : en
    }
}
