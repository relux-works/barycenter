import AppKit
import SwiftUI

public struct PulsarAirManagementView: View {
    let model: PulsarShellModel
    let actions: PulsarShellActions
    @State private var createTitle = ""
    @State private var inviteCode = ""
    @State private var pendingAction: PendingAirAction?

    public init(model: PulsarShellModel, actions: PulsarShellActions) {
        self.model = model
        self.actions = actions
    }

    public var body: some View {
        let copy = PulsarAirCopy(locale: model.locale)
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                header(copy)
                feedback(copy)
                if let pending = model.snapshot.airs.pendingJoin {
                    pendingJoin(pending, copy: copy)
                }
                if let secret = model.snapshot.airs.inviteSecret {
                    inviteSecret(secret, copy: copy)
                }
                lifecycleForms(copy)
                savedAirs(copy)
            }
            .padding(24)
            .frame(maxWidth: 760, alignment: .leading)
        }
        .navigationTitle(copy.airs)
        .toolbar {
            Button(copy.refresh, systemImage: "arrow.clockwise") { actions.refreshAirs() }
                .disabled(model.snapshot.airs.busy)
                .keyboardShortcut("r", modifiers: .command)
        }
        .confirmationDialog(
            pendingAction?.title(copy) ?? copy.confirm,
            isPresented: Binding(
                get: { pendingAction != nil },
                set: { if !$0 { pendingAction = nil } }),
            titleVisibility: .visible
        ) {
            if let pendingAction {
                Button(pendingAction.button(copy), role: pendingAction.isDestructive ? .destructive : nil) {
                    perform(pendingAction)
                }
            }
            Button(copy.cancel, role: .cancel) { pendingAction = nil }
        } message: {
            if let pendingAction { Text(pendingAction.message(copy)) }
        }
    }

    private func header(_ copy: PulsarAirCopy) -> some View {
        VStack(alignment: .leading, spacing: 6) {
            Label(copy.airs, systemImage: "person.3.sequence.fill")
                .font(.title2.bold())
            Text(copy.intro)
                .foregroundStyle(.secondary)
            if let current = model.snapshot.airs.current {
                Label("\(copy.current): \(current.title)", systemImage: "dot.radiowaves.left.and.right")
                    .font(.headline)
                    .accessibilityElement(children: .combine)
            } else {
                Label(copy.noCurrent, systemImage: "pause.circle")
                    .foregroundStyle(.secondary)
            }
        }
    }

    @ViewBuilder
    private func feedback(_ copy: PulsarAirCopy) -> some View {
        if model.snapshot.airs.busy {
            ProgressView(copy.working)
                .accessibilityLabel(copy.working)
        }
        if let outcome = model.snapshot.airs.outcome {
            Label(copy.outcome(outcome), systemImage: "checkmark.circle.fill")
                .foregroundStyle(.green)
                .accessibilityElement(children: .combine)
        }
        if let failure = model.snapshot.airs.failure {
            Label(copy.failure(failure), systemImage: "exclamationmark.triangle.fill")
                .foregroundStyle(.orange)
                .accessibilityElement(children: .combine)
        }
    }

    private func lifecycleForms(_ copy: PulsarAirCopy) -> some View {
        GroupBox(copy.addAir) {
            VStack(alignment: .leading, spacing: 12) {
                HStack {
                    TextField(copy.airName, text: $createTitle)
                        .textFieldStyle(.roundedBorder)
                        .accessibilityLabel(copy.airName)
                        .onSubmit(createAir)
                    Button(copy.create, action: createAir)
                        .buttonStyle(.borderedProminent)
                        .disabled(cleanCreateTitle.isEmpty || model.snapshot.airs.busy)
                }
                Divider()
                HStack {
                    SecureField(copy.inviteCode, text: $inviteCode)
                        .textFieldStyle(.roundedBorder)
                        .accessibilityLabel(copy.inviteCode)
                        .onSubmit(consumeInvite)
                    Button(copy.preview, action: consumeInvite)
                        .disabled(cleanInviteCode.isEmpty || model.snapshot.airs.busy)
                }
                Text(copy.invitePrivacy)
                    .font(.caption)
                    .foregroundStyle(.secondary)
            }
            .padding(.vertical, 4)
        }
    }

    private func pendingJoin(_ pending: PulsarPendingAirJoin, copy: PulsarAirCopy) -> some View {
        GroupBox(copy.confirmJoin) {
            VStack(alignment: .leading, spacing: 10) {
                Text(pending.title).font(.headline)
                if let owner = pending.ownerDisplayName, !owner.isEmpty {
                    LabeledContent(copy.owner, value: owner)
                }
                LabeledContent(copy.yourRole, value: copy.role(pending.role))
                LabeledContent(
                    copy.members,
                    value: "\(pending.memberCount)/\(pending.barycenterCapacity)")
                if pending.activationWouldSwitch {
                    Label(copy.switchWarning, systemImage: "exclamationmark.triangle")
                        .foregroundStyle(.orange)
                        .accessibilityElement(children: .combine)
                }
                HStack {
                    Button(copy.saveOnly) {
                        actions.confirmAirJoin(pending.airID, activate: false)
                    }
                    .buttonStyle(.borderedProminent)
                    Button(copy.joinAndActivate) {
                        if pending.activationWouldSwitch {
                            pendingAction = .init(kind: .confirmAndSwitch, airID: pending.airID, airTitle: pending.title)
                        } else {
                            actions.confirmAirJoin(pending.airID, activate: true)
                        }
                    }
                    Button(copy.decline, role: .destructive) {
                        actions.declineAirJoin(pending.airID)
                    }
                }
                .disabled(model.snapshot.airs.busy)
            }
            .padding(.vertical, 4)
        }
    }

    private func inviteSecret(_ secret: PulsarAirInviteSecret, copy: PulsarAirCopy) -> some View {
        GroupBox(copy.inviteReady) {
            VStack(alignment: .leading, spacing: 10) {
                Text(secret.airTitle).font(.headline)
                Text(secret.code)
                    .font(.body.monospaced())
                    .textSelection(.enabled)
                    .accessibilityLabel(copy.oneTimeCode)
                    .privacySensitive()
                Text("\(copy.expires) \(secret.expiresAt.formatted(date: .omitted, time: .shortened))")
                    .font(.caption)
                    .foregroundStyle(.secondary)
                Text(copy.oneTimeWarning)
                    .font(.caption)
                    .foregroundStyle(.secondary)
                HStack {
                    Button(copy.copy) {
                        copyInviteCode(secret.code)
                    }
                    .keyboardShortcut("c", modifiers: [.command, .shift])
                    Button(copy.hide) { actions.hideAirInvite() }
                    Button(copy.withdraw, role: .destructive) { actions.withdrawAirInvite() }
                }
                .disabled(model.snapshot.airs.busy)
            }
            .padding(.vertical, 4)
        }
    }

    @ViewBuilder
    private func savedAirs(_ copy: PulsarAirCopy) -> some View {
        Text(copy.saved).font(.title2.bold())
        if model.snapshot.airs.saved.isEmpty {
            ContentUnavailableView(copy.noSaved, systemImage: "person.3.sequence")
                .frame(maxWidth: .infinity, minHeight: 120)
        } else {
            LazyVStack(spacing: 12) {
                ForEach(model.snapshot.airs.saved) { air in
                    PulsarAirCard(
                        air: air, locale: model.locale, busy: model.snapshot.airs.busy,
                        activate: {
                            if model.snapshot.airs.current == nil {
                                actions.activateAir(air.id)
                            } else {
                                pendingAction = .init(kind: .switchAir, airID: air.id, airTitle: air.title)
                            }
                        },
                        deactivate: {
                            pendingAction = .init(kind: .deactivate, airID: air.id, airTitle: air.title)
                        },
                        leave: {
                            pendingAction = .init(kind: .leave, airID: air.id, airTitle: air.title)
                        },
                        dissolve: {
                            pendingAction = .init(kind: .dissolve, airID: air.id, airTitle: air.title)
                        },
                        issueInvite: { actions.issueAirInvite(air.id, role: $0) },
                        replacePolicy: { actions.replaceAirPolicy(air.id, policy: $0) })
                }
            }
        }
    }

    private var cleanCreateTitle: String {
        createTitle.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private var cleanInviteCode: String {
        inviteCode.trimmingCharacters(in: .whitespacesAndNewlines)
    }

    private func createAir() {
        guard !cleanCreateTitle.isEmpty else { return }
        actions.createAir(title: cleanCreateTitle)
        createTitle = ""
    }

    private func consumeInvite() {
        guard !cleanInviteCode.isEmpty else { return }
        actions.consumeAirInvite(code: cleanInviteCode)
        inviteCode = ""
    }

    private func perform(_ action: PendingAirAction) {
        switch action.kind {
        case .confirmAndSwitch: actions.confirmAirJoin(action.airID, activate: true)
        case .switchAir: actions.activateAir(action.airID)
        case .deactivate: actions.deactivateAir(action.airID)
        case .leave: actions.leaveAir(action.airID)
        case .dissolve: actions.dissolveAir(action.airID)
        }
        pendingAction = nil
    }

    private func copyInviteCode(_ code: String) {
        let pasteboard = NSPasteboard.general
        pasteboard.clearContents()
        pasteboard.setString(code, forType: .string)
        DispatchQueue.main.asyncAfter(deadline: .now() + 60) {
            guard pasteboard.string(forType: .string) == code else { return }
            pasteboard.clearContents()
        }
    }
}

private struct PulsarAirCard: View {
    let air: PulsarAirItem
    let locale: PulsarShellLocale
    let busy: Bool
    @State private var inviteRole = PulsarAirRole.member
    let activate: () -> Void
    let deactivate: () -> Void
    let leave: () -> Void
    let dissolve: () -> Void
    let issueInvite: (PulsarAirRole) -> Void
    let replacePolicy: (PulsarAirPolicy) -> Void

    var body: some View {
        let copy = PulsarAirCopy(locale: locale)
        VStack(alignment: .leading, spacing: 12) {
            HStack {
                Label(air.title, systemImage: air.isCurrent ? "dot.radiowaves.left.and.right" : "person.3")
                    .font(.headline)
                Spacer()
                Text(air.isCurrent ? copy.current : copy.savedOnly)
                    .font(.caption.bold())
                    .foregroundStyle(air.isCurrent ? Color.green : Color.secondary)
            }
            HStack(spacing: 16) {
                Label("\(air.memberCount)/\(air.barycenterCapacity) \(copy.members)", systemImage: "building.2")
                Label("\(air.activeMemberCount) \(copy.active)", systemImage: "waveform")
                Label("\(air.onlinePulsarCount)/\(air.onlinePulsarCapacity) \(copy.online)", systemImage: "desktopcomputer")
            }
            .font(.callout)
            .foregroundStyle(.secondary)
            .accessibilityElement(children: .combine)
            LabeledContent(copy.yourRole, value: copy.role(air.role))
            LabeledContent(copy.status, value: copy.status(air.status))
            if air.membershipStatus == .pendingConfirmation {
                Label(copy.pendingPrimary, systemImage: "person.badge.clock")
                    .foregroundStyle(.orange)
            }
            DisclosureGroup(copy.effectivePolicies) {
                VStack(alignment: .leading, spacing: 6) {
                    LabeledContent(copy.invites, value: copy.policy(air.policy.invite.rawValue))
                    LabeledContent(copy.overlays, value: copy.policy(air.policy.overlay.rawValue))
                    LabeledContent(copy.queue, value: copy.policy(air.policy.queue.rawValue))
                    LabeledContent(copy.replace, value: copy.policy(air.policy.replace.rawValue))
                    if air.role == .owner {
                        PulsarAirPolicyEditor(air: air, locale: locale, save: replacePolicy)
                    }
                }
                .padding(.top, 6)
            }
            if air.membershipStatus == .joined {
                HStack {
                    if air.isCurrent {
                        Button(copy.deactivate, action: deactivate)
                    } else {
                        Button(copy.activate, action: activate)
                            .buttonStyle(.borderedProminent)
                    }
                    Picker(copy.inviteRole, selection: $inviteRole) {
                        Text(copy.role(.member)).tag(PulsarAirRole.member)
                        if air.role == .owner { Text(copy.role(.admin)).tag(PulsarAirRole.admin) }
                    }
                    .labelsHidden()
                    .frame(width: 130)
                    Button(copy.invite) { issueInvite(inviteRole) }
                    if air.role != .owner {
                        Button(copy.leave, role: .destructive, action: leave)
                    } else {
                        Button(copy.dissolve, role: .destructive, action: dissolve)
                    }
                }
                .disabled(busy)
            }
        }
        .padding(14)
        .background(.quaternary, in: RoundedRectangle(cornerRadius: 12))
        .accessibilityElement(children: .contain)
    }
}

private struct PulsarAirPolicyEditor: View {
    let air: PulsarAirItem
    let locale: PulsarShellLocale
    let save: (PulsarAirPolicy) -> Void
    @State private var invite: PulsarAirInvitePolicy
    @State private var overlay: PulsarAirPlaybackPolicy
    @State private var queue: PulsarAirPlaybackPolicy
    @State private var replace: PulsarAirPlaybackPolicy

    init(air: PulsarAirItem, locale: PulsarShellLocale, save: @escaping (PulsarAirPolicy) -> Void) {
        self.air = air
        self.locale = locale
        self.save = save
        _invite = State(initialValue: air.policy.invite)
        _overlay = State(initialValue: air.policy.overlay)
        _queue = State(initialValue: air.policy.queue)
        _replace = State(initialValue: air.policy.replace)
    }

    var body: some View {
        let copy = PulsarAirCopy(locale: locale)
        Divider()
        Text(copy.changePolicies).font(.callout.bold())
        Picker(copy.invites, selection: $invite) {
            ForEach(PulsarAirInvitePolicy.allCases) { Text(copy.policy($0.rawValue)).tag($0) }
        }
        Picker(copy.overlays, selection: $overlay) {
            ForEach(playbackPolicies) { Text(copy.policy($0.rawValue)).tag($0) }
        }
        Picker(copy.queue, selection: $queue) {
            ForEach(playbackPolicies) { Text(copy.policy($0.rawValue)).tag($0) }
        }
        Picker(copy.replace, selection: $replace) {
            ForEach(replacePolicies) { Text(copy.policy($0.rawValue)).tag($0) }
        }
        Button(copy.savePolicies) {
            save(.init(
                revision: air.policy.revision, invite: invite,
                overlay: overlay, queue: queue, replace: replace))
        }
        .disabled(invite == air.policy.invite && overlay == air.policy.overlay
            && queue == air.policy.queue && replace == air.policy.replace)
    }

    private var playbackPolicies: [PulsarAirPlaybackPolicy] {
        [.airAdminPrimary, .allMemberPrimaries, .primaryCompanion, .disabled]
    }

    private var replacePolicies: [PulsarAirPlaybackPolicy] {
        [.ownerPrimary, .airAdminPrimary, .allMemberPrimaries, .disabled]
    }
}

private struct PendingAirAction: Identifiable {
    enum Kind { case confirmAndSwitch, switchAir, deactivate, leave, dissolve }
    let kind: Kind
    let airID: String
    let airTitle: String
    var id: String { "\(airID)-\(String(describing: kind))" }

    var isDestructive: Bool { kind == .leave || kind == .dissolve }
    func title(_ copy: PulsarAirCopy) -> String { "\(button(copy)): \(airTitle)?" }
    func button(_ copy: PulsarAirCopy) -> String {
        switch kind {
        case .confirmAndSwitch: copy.joinAndActivate
        case .switchAir: copy.switchTo
        case .deactivate: copy.deactivate
        case .leave: copy.leave
        case .dissolve: copy.dissolve
        }
    }
    func message(_ copy: PulsarAirCopy) -> String {
        switch kind {
        case .confirmAndSwitch, .switchAir: copy.switchEffects
        case .deactivate: copy.deactivateEffects
        case .leave: copy.leaveEffects
        case .dissolve: copy.dissolveEffects
        }
    }
}

private struct PulsarAirCopy {
    let locale: PulsarShellLocale
    private func l(_ en: String, _ ru: String) -> String { locale == .ru ? ru : en }

    var airs: String { l("Airs", "Эфиры") }
    var intro: String { l("Saved Airs are separate from the one active playback room.", "Сохранённые эфиры не равны одному активному пространству воспроизведения.") }
    var current: String { l("Active Air", "Активный эфир") }
    var noCurrent: String { l("No Air is active; saved memberships remain available.", "Активного эфира нет; сохранённые участия остаются доступны.") }
    var refresh: String { l("Refresh Airs", "Обновить эфиры") }
    var working: String { l("Updating Airs…", "Обновляю эфиры…") }
    var addAir: String { l("Create or join", "Создать или присоединиться") }
    var airName: String { l("Air name", "Название эфира") }
    var create: String { l("Create Air", "Создать эфир") }
    var inviteCode: String { l("One-time invite code", "Одноразовый код приглашения") }
    var preview: String { l("Review invite", "Проверить приглашение") }
    var invitePrivacy: String { l("The code is sent only to the coordinator and is never stored in this app.", "Код отправляется только координатору и не сохраняется приложением.") }
    var confirmJoin: String { l("Primary confirmation required", "Требуется подтверждение primary") }
    var owner: String { l("Owner", "Владелец") }
    var yourRole: String { l("Your Air role", "Ваша роль в эфире") }
    var members: String { l("members", "участников") }
    var switchWarning: String { l("Activating this Air will switch playback away from the current Air.", "Активация переключит воспроизведение из текущего эфира.") }
    var saveOnly: String { l("Confirm and save", "Подтвердить и сохранить") }
    var joinAndActivate: String { l("Confirm and activate", "Подтвердить и активировать") }
    var decline: String { l("Decline", "Отклонить") }
    var inviteReady: String { l("One-time invite ready", "Одноразовое приглашение готово") }
    var oneTimeCode: String { l("One-time Air invitation code", "Одноразовый код приглашения в эфир") }
    var expires: String { l("Expires", "Истекает") }
    var oneTimeWarning: String { l("Shown only in this session. Copy it, then hide or withdraw it.", "Показывается только в этой сессии. Скопируйте, затем скройте или отзовите.") }
    var copy: String { l("Copy code", "Скопировать код") }
    var hide: String { l("Hide now", "Скрыть") }
    var withdraw: String { l("Withdraw", "Отозвать") }
    var saved: String { l("Saved Airs", "Сохранённые эфиры") }
    var noSaved: String { l("No saved Airs", "Нет сохранённых эфиров") }
    var savedOnly: String { l("Saved", "Сохранён") }
    var active: String { l("active", "активны") }
    var online: String { l("Pulsars online", "Пульсаров в сети") }
    var status: String { l("Room state", "Состояние комнаты") }
    var pendingPrimary: String { l("Waiting for this barycenter's primary confirmation", "Ожидает подтверждения primary этого барицентра") }
    var effectivePolicies: String { l("Effective policies", "Действующие правила") }
    var invites: String { l("Invites", "Приглашения") }
    var overlays: String { l("Overlays", "Оверлеи") }
    var queue: String { l("Queue", "Очередь") }
    var replace: String { l("Replace playback", "Замена воспроизведения") }
    var changePolicies: String { l("Owner policy changes", "Изменение правил владельцем") }
    var savePolicies: String { l("Save policy changes", "Сохранить правила") }
    var activate: String { l("Activate", "Активировать") }
    var deactivate: String { l("Deactivate", "Деактивировать") }
    var inviteRole: String { l("Invite role", "Роль приглашения") }
    var invite: String { l("Create invite", "Создать приглашение") }
    var leave: String { l("Leave Air", "Покинуть эфир") }
    var dissolve: String { l("Dissolve Air", "Распустить эфир") }
    var switchTo: String { l("Switch Air", "Переключить эфир") }
    var cancel: String { l("Cancel", "Отмена") }
    var confirm: String { l("Confirm Air action", "Подтвердить действие") }
    var switchEffects: String { l("The current Air stops receiving this barycenter's main track and overlays. Saved membership is retained.", "Текущий эфир перестанет получать основной трек и оверлеи этого барицентра. Участие сохранится.") }
    var deactivateEffects: String { l("Main track and overlays from this Air stop here. Membership remains saved.", "Основной трек и оверлеи этого эфира здесь остановятся. Участие сохранится.") }
    var leaveEffects: String { l("Playback stops here and this Air is removed from saved memberships.", "Воспроизведение остановится, а эфир исчезнет из сохранённых участий.") }
    var dissolveEffects: String { l("This permanently ends the Air for every member and stops its playback.", "Эфир навсегда завершится для всех участников, а воспроизведение остановится.") }

    func role(_ role: PulsarAirRole) -> String {
        switch role {
        case .owner: l("Owner", "Владелец")
        case .admin: l("Admin", "Администратор")
        case .member: l("Member", "Участник")
        }
    }
    func status(_ value: String) -> String {
        value == "active" ? l("Active", "Активен") : l("Parked", "Приостановлен")
    }
    func policy(_ value: String) -> String {
        switch value {
        case "owner_primary": l("Owner primary", "Primary владельца")
        case "air_admin_primary": l("Owner/admin primary", "Primary владельца/админа")
        case "all_member_primaries": l("All member primaries", "Все primary участников")
        case "primary_companion": l("Primary and companions", "Primary и companions")
        case "disabled": l("Disabled", "Выключено")
        default: l("Unavailable", "Недоступно")
        }
    }
    func outcome(_ code: String) -> String {
        let values = [
            "created": l("Air created and saved.", "Эфир создан и сохранён."),
            "invite_issued": l("One-time invite created.", "Одноразовое приглашение создано."),
            "invite_withdrawn": l("Invite withdrawn.", "Приглашение отозвано."),
            "invite_reviewed": l("Invite consumed; primary confirmation is still required.", "Приглашение принято; всё ещё нужно подтверждение primary."),
            "join_confirmed": l("Air membership confirmed.", "Участие в эфире подтверждено."),
            "join_declined": l("Pending membership declined.", "Ожидающее участие отклонено."),
            "activated": l("Active Air updated.", "Активный эфир обновлён."),
            "deactivated": l("Air deactivated; membership remains saved.", "Эфир деактивирован; участие сохранено."),
            "left": l("Air left.", "Вы покинули эфир."),
            "policy_updated": l("Air policies updated.", "Правила эфира обновлены."),
            "dissolved": l("Air dissolved.", "Эфир распущен."),
        ]
        return values[code] ?? code.replacingOccurrences(of: "_", with: " ")
    }
    func failure(_ code: String) -> String {
        let values = [
            "coordinator_unavailable": l("Coordinator is offline. Nothing was changed locally.", "Координатор недоступен. Локально ничего не изменено."),
            "forbidden": l("Your current role does not permit this Air action.", "Текущая роль не разрешает это действие с эфиром."),
            "invite_unavailable": l("This invite is expired, used, withdrawn or unknown.", "Приглашение истекло, использовано, отозвано или неизвестно."),
            "too_many_attempts": l("Too many invite attempts. Wait before retrying.", "Слишком много попыток. Подождите перед повтором."),
            "revision_conflict": l("The Air changed elsewhere. Refresh and try again.", "Эфир изменился в другом месте. Обновите и повторите."),
            "active_air_changed": l("The active Air changed elsewhere. Refresh before switching.", "Активный эфир изменился в другом месте. Обновите перед переключением."),
            "air_barycenter_capacity_reached": l("This Air reached its barycenter capacity.", "Эфир достиг лимита барицентров."),
            "air_online_pulsar_capacity_reached": l("This Air reached its online Pulsar capacity.", "Эфир достиг лимита Пульсаров в сети."),
            "owner_transfer_required": l("Transfer ownership or dissolve the Air before leaving.", "Перед выходом передайте владение или распустите эфир."),
            "membership_confirmation_required": l("The joining barycenter primary must confirm first.", "Сначала участие должен подтвердить primary присоединяющегося барицентра."),
            "air_dissolved": l("This Air has already been dissolved.", "Этот эфир уже распущен."),
            "service_unavailable": l("The change was saved, but runtime synchronization is unavailable. Refresh before retrying.", "Изменение сохранено, но синхронизация runtime недоступна. Обновите перед повтором."),
        ]
        return values[code] ?? l("Air action failed. Refresh and try again.", "Действие не выполнено. Обновите и повторите.")
    }
}
