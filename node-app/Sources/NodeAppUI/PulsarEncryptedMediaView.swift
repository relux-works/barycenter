import SwiftUI

struct PulsarEncryptedMediaView: View {
  @Bindable var model: PulsarEncryptedMediaModel
  let locale: PulsarShellLocale
  let actions: PulsarEncryptedMediaActions

  @State private var confirmation: PulsarEncryptedMediaConfirmation?

  var body: some View {
    let copy = PulsarEncryptedMediaCopy(locale: locale)
    ScrollView {
      VStack(alignment: .leading, spacing: 16) {
        PulsarEncryptedMediaHeader(
          availability: model.availability(for: model.snapshot.selectedPath),
          path: model.snapshot.selectedPath,
          copy: copy,
          refresh: { actions.perform(model.refreshCommand()) })
        PulsarEncryptedMediaPathSection(
          model: model,
          copy: copy,
          actions: actions,
          confirmation: $confirmation)
        PulsarEncryptedMediaDevicesSection(
          model: model,
          copy: copy,
          actions: actions,
          confirmation: $confirmation)
        PulsarEncryptedMediaRecoverySection(
          model: model,
          copy: copy,
          confirmation: $confirmation)
        PulsarEncryptedMediaHistorySection(
          model: model,
          copy: copy,
          confirmation: $confirmation)
        PulsarEncryptedMediaReportSection(
          model: model,
          copy: copy,
          actions: actions,
          confirmation: $confirmation)
        PulsarEncryptedMediaOutcomeSection(snapshot: model.snapshot, copy: copy)
      }
      .padding(18)
      .frame(maxWidth: .infinity, alignment: .leading)
    }
    .confirmationDialog(
      copy.confirmationTitle(confirmation),
      isPresented: Binding(
        get: { confirmation != nil },
        set: { if !$0 { confirmation = nil } }),
      titleVisibility: .visible,
      presenting: confirmation
    ) { pending in
      confirmationActions(pending, copy: copy)
    } message: { pending in
      Text(copy.confirmationMessage(pending))
    }
  }

  @ViewBuilder
  private func confirmationActions(
    _ pending: PulsarEncryptedMediaConfirmation,
    copy: PulsarEncryptedMediaCopy
  ) -> some View {
    switch pending {
    case .excludeUnsupported:
      Button(copy.text("Exclude and keep encryption", "Исключить и сохранить шифрование")) {
        actions.perform(model.unsupportedExclusionCommand(confirmed: true))
        confirmation = nil
      }
    case .revokeDevice(let id):
      Button(copy.text("Revoke device", "Отозвать устройство"), role: .destructive) {
        actions.perform(model.revokeDeviceCommand(id, confirmed: true))
        confirmation = nil
      }
    case .deviceTransfer:
      Button(copy.text("Transfer current access", "Передать текущий доступ")) {
        actions.perform(model.deviceTransferCommand())
        confirmation = nil
      }
    case .userHeldRecovery:
      Button(copy.text("Use my recovery capability", "Использовать мой recovery-доступ")) {
        actions.perform(model.userHeldRecoveryCommand(confirmed: true))
        confirmation = nil
      }
    case .createHistoryGrant:
      Button(copy.text("Grant selected history", "Предоставить выбранную историю")) {
        actions.perform(model.createHistoryGrantCommand(confirmed: true))
        confirmation = nil
      }
    case .revokeHistoryGrant(let id):
      Button(copy.text("Revoke history grant", "Отозвать доступ к истории"), role: .destructive) {
        actions.perform(model.revokeHistoryGrantCommand(id, confirmed: true))
        confirmation = nil
      }
    case .exportEvidence:
      Button(copy.text("Disclose decrypted evidence", "Передать расшифрованное доказательство")) {
        actions.perform(model.decryptedEvidenceExportCommand(confirmed: true))
        confirmation = nil
      }
    }
    Button(copy.text("Cancel", "Отмена"), role: .cancel) { confirmation = nil }
  }
}

private enum PulsarEncryptedMediaConfirmation: Identifiable, Equatable {
  case excludeUnsupported
  case revokeDevice(String)
  case deviceTransfer
  case userHeldRecovery
  case createHistoryGrant
  case revokeHistoryGrant(String)
  case exportEvidence

  var id: String {
    switch self {
    case .excludeUnsupported: "exclude-unsupported"
    case .revokeDevice: "revoke-device"
    case .deviceTransfer: "device-transfer"
    case .userHeldRecovery: "user-held-recovery"
    case .createHistoryGrant: "create-history-grant"
    case .revokeHistoryGrant: "revoke-history-grant"
    case .exportEvidence: "export-evidence"
    }
  }
}

private struct PulsarEncryptedMediaHeader: View {
  let availability: PulsarEncryptedMediaAvailability
  let path: PulsarEncryptedMediaPath
  let copy: PulsarEncryptedMediaCopy
  let refresh: () -> Void

  var body: some View {
    HStack(alignment: .top, spacing: 12) {
      VStack(alignment: .leading, spacing: 5) {
        Label(copy.text("Encrypted media", "Зашифрованные медиа"), systemImage: icon)
          .font(.title3.bold())
          .foregroundStyle(color)
        Text(copy.pathStatus(path: path, availability: availability))
          .font(.callout)
          .foregroundStyle(.secondary)
      }
      Spacer()
      Button(copy.text("Refresh", "Обновить"), action: refresh)
        .keyboardShortcut("r", modifiers: .command)
    }
    .accessibilityElement(children: .contain)
    .accessibilityLabel(copy.text("Encrypted media status", "Статус зашифрованных медиа"))
    .accessibilityValue(copy.pathStatus(path: path, availability: availability))
  }

  private var icon: String {
    switch availability {
    case .plaintext: "lock.open"
    case .encrypted: "lock.shield.fill"
    case .blocked: "exclamationmark.lock.fill"
    }
  }

  private var color: Color {
    switch availability {
    case .plaintext: .secondary
    case .encrypted: .green
    case .blocked: .orange
    }
  }
}

private struct PulsarEncryptedMediaPathSection: View {
  let model: PulsarEncryptedMediaModel
  let copy: PulsarEncryptedMediaCopy
  let actions: PulsarEncryptedMediaActions
  @Binding var confirmation: PulsarEncryptedMediaConfirmation?

  var body: some View {
    GroupBox(copy.text("Delivery path", "Путь доставки")) {
      VStack(alignment: .leading, spacing: 10) {
        ForEach(PulsarEncryptedMediaPath.allCases) { path in
          PulsarEncryptedMediaPathRow(
            path: path,
            selected: model.snapshot.selectedPath == path,
            availability: model.availability(for: path),
            reason: copy.failure(model.pathFailure(path)),
            copy: copy,
            select: { actions.perform(model.selectPathCommand(path)) })
        }

        if !model.snapshot.unsupportedRecipients.isEmpty {
          Divider()
          Label(
            copy.text(
              "Some recipients do not support protected media.",
              "Некоторые получатели не поддерживают защищённые медиа."),
            systemImage: "person.crop.circle.badge.exclamationmark"
          )
          .foregroundStyle(.orange)
          ForEach(model.snapshot.unsupportedRecipients) { recipient in
            Text(recipient.label.text(locale: copy.locale))
              .font(.callout)
          }
          Button(
            copy.text(
              "Choose encrypted-only recipients…", "Выбрать только получателей с шифрованием…")
          ) {
            confirmation = .excludeUnsupported
          }
          .disabled(model.unsupportedExclusionCommand(confirmed: true) == nil)
          Text(
            copy.text(
              "Protected send remains blocked until you explicitly exclude them. It never falls back to plaintext.",
              "Защищённая отправка заблокирована, пока вы явно не исключите их. Перехода на plaintext не будет."
            )
          )
          .font(.caption)
          .foregroundStyle(.secondary)
        }
      }
      .frame(maxWidth: .infinity, alignment: .leading)
      .padding(.vertical, 4)
    }
    .accessibilityElement(children: .contain)
  }
}

private struct PulsarEncryptedMediaPathRow: View {
  let path: PulsarEncryptedMediaPath
  let selected: Bool
  let availability: PulsarEncryptedMediaAvailability
  let reason: String
  let copy: PulsarEncryptedMediaCopy
  let select: () -> Void

  var body: some View {
    HStack(spacing: 10) {
      Image(systemName: icon)
        .foregroundStyle(color)
        .accessibilityHidden(true)
      VStack(alignment: .leading, spacing: 2) {
        Text(copy.path(path)).font(.headline)
        Text(reason).font(.caption).foregroundStyle(.secondary)
      }
      Spacer()
      Button(
        selected ? copy.text("Selected", "Выбрано") : copy.text("Select", "Выбрать"), action: select
      )
      .disabled(selected || (path.isProtected && availability != .encrypted))
    }
    .accessibilityElement(children: .contain)
    .accessibilityAddTraits(selected ? .isSelected : [])
  }

  private var icon: String {
    switch availability {
    case .plaintext: "lock.open"
    case .encrypted: "lock.fill"
    case .blocked: "exclamationmark.lock.fill"
    }
  }

  private var color: Color {
    switch availability {
    case .plaintext: .secondary
    case .encrypted: .green
    case .blocked: .orange
    }
  }
}

private struct PulsarEncryptedMediaDevicesSection: View {
  let model: PulsarEncryptedMediaModel
  let copy: PulsarEncryptedMediaCopy
  let actions: PulsarEncryptedMediaActions
  @Binding var confirmation: PulsarEncryptedMediaConfirmation?

  var body: some View {
    GroupBox(copy.text("Device verification", "Верификация устройств")) {
      VStack(alignment: .leading, spacing: 10) {
        LabeledContent(copy.text("Membership", "Участие")) {
          Text(copy.membership(model.snapshot.membership))
        }
        LabeledContent(copy.text("Current epoch", "Текущая эпоха")) {
          Text(model.snapshot.epoch, format: .number)
        }
        if model.snapshot.devices.isEmpty {
          Text(
            copy.text(
              "No verified device projection is available.",
              "Нет доступной проекции верифицированных устройств.")
          )
          .foregroundStyle(.secondary)
        }
        ForEach(model.snapshot.devices) { device in
          HStack(spacing: 10) {
            Label(
              device.label.text(locale: copy.locale),
              systemImage: copy.verificationIcon(device.verification))
            if device.isThisDevice {
              Text(copy.text("This Mac", "Этот Mac"))
                .font(.caption)
                .foregroundStyle(.secondary)
            }
            Spacer()
            if device.verification == .unverified {
              Button(copy.text("Verify", "Проверить")) {
                actions.perform(model.verifyDeviceCommand(device.id))
              }
              .disabled(model.verifyDeviceCommand(device.id) == nil)
            }
            if device.canRevoke && device.verification == .verified {
              Button(copy.text("Revoke…", "Отозвать…"), role: .destructive) {
                confirmation = .revokeDevice(device.id)
              }
              .disabled(model.revokeDeviceCommand(device.id, confirmed: true) == nil)
            }
          }
          .accessibilityElement(children: .contain)
          .accessibilityValue(copy.verification(device.verification))
        }
        if model.snapshot.membership == .rotationRequired {
          Label(
            copy.text(
              "A lost-device revoke requires a new group epoch before protected send resumes.",
              "После отзыва потерянного устройства нужна новая эпоха группы до возобновления защищённой отправки."
            ),
            systemImage: "arrow.triangle.2.circlepath"
          )
          .foregroundStyle(.orange)
        }
      }
      .frame(maxWidth: .infinity, alignment: .leading)
      .padding(.vertical, 4)
    }
    .accessibilityElement(children: .contain)
  }
}

private struct PulsarEncryptedMediaRecoverySection: View {
  let model: PulsarEncryptedMediaModel
  let copy: PulsarEncryptedMediaCopy
  @Binding var confirmation: PulsarEncryptedMediaConfirmation?

  var body: some View {
    GroupBox(copy.text("New device and recovery", "Новое устройство и восстановление")) {
      VStack(alignment: .leading, spacing: 10) {
        Text(
          copy.text(
            "A verified peer can transfer only the current epoch. Protected history needs separate grants.",
            "Верифицированное устройство может передать только текущую эпоху. Для защищённой истории нужны отдельные разрешения."
          )
        )
        .font(.callout)
        HStack {
          Button(copy.text("Transfer current access…", "Передать текущий доступ…")) {
            confirmation = .deviceTransfer
          }
          .disabled(model.deviceTransferCommand() == nil)
          Button(copy.text("Use my recovery capability…", "Использовать мой recovery-доступ…")) {
            confirmation = .userHeldRecovery
          }
          .disabled(model.userHeldRecoveryCommand(confirmed: true) == nil)
        }
        if !model.snapshot.historyRecoverable {
          Label(
            copy.text(
              "No surviving authorized device or reviewed user-held recovery capability exists. Protected history is unrecoverable.",
              "Нет доступного авторизованного устройства или проверенного пользовательского recovery-доступа. Защищённая история невосстановима."
            ),
            systemImage: "exclamationmark.triangle.fill"
          )
          .foregroundStyle(.red)
          .accessibilityElement(children: .combine)
        }
      }
      .frame(maxWidth: .infinity, alignment: .leading)
      .padding(.vertical, 4)
    }
    .accessibilityElement(children: .contain)
  }
}

private struct PulsarEncryptedMediaHistorySection: View {
  let model: PulsarEncryptedMediaModel
  let copy: PulsarEncryptedMediaCopy
  @Binding var confirmation: PulsarEncryptedMediaConfirmation?

  var body: some View {
    GroupBox(copy.text("Explicit history grants", "Явный доступ к истории")) {
      VStack(alignment: .leading, spacing: 10) {
        if let draft = model.snapshot.historyGrantDraft {
          LabeledContent(copy.text("Selected item", "Выбранный объект"), value: draft.title)
          LabeledContent(copy.text("Epoch range", "Диапазон эпох")) {
            Text("\(draft.firstEpoch)–\(draft.lastEpoch)")
          }
          LabeledContent(copy.text("Access", "Доступ"), value: copy.grantMode(draft.mode))
          LabeledContent(copy.text("Expires", "Истекает")) {
            Text(draft.expiresAt, format: .dateTime.year().month().day().hour().minute())
          }
          Button(copy.text("Grant selected history…", "Предоставить выбранную историю…")) {
            confirmation = .createHistoryGrant
          }
          .disabled(model.createHistoryGrantCommand(confirmed: true) == nil)
        } else {
          Text(
            copy.text(
              "Select one ready protected item before granting history.",
              "Перед выдачей доступа выберите один готовый защищённый объект.")
          )
          .foregroundStyle(.secondary)
        }

        ForEach(model.snapshot.historyGrants) { grant in
          Divider()
          HStack(alignment: .top, spacing: 10) {
            VStack(alignment: .leading, spacing: 3) {
              Text(grant.title).font(.headline)
              Text(copy.grantStatus(grant.status))
                .font(.caption)
                .foregroundStyle(.secondary)
              Text(grant.expiresAt, format: .dateTime.year().month().day().hour().minute())
                .font(.caption)
            }
            Spacer()
            Button(copy.text("Revoke…", "Отозвать…"), role: .destructive) {
              confirmation = .revokeHistoryGrant(grant.id)
            }
            .disabled(model.revokeHistoryGrantCommand(grant.id, confirmed: true) == nil)
          }
          .accessibilityElement(children: .contain)
        }
      }
      .frame(maxWidth: .infinity, alignment: .leading)
      .padding(.vertical, 4)
    }
    .accessibilityElement(children: .contain)
  }
}

private struct PulsarEncryptedMediaReportSection: View {
  let model: PulsarEncryptedMediaModel
  let copy: PulsarEncryptedMediaCopy
  let actions: PulsarEncryptedMediaActions
  @Binding var confirmation: PulsarEncryptedMediaConfirmation?

  var body: some View {
    GroupBox(copy.text("Report boundary", "Граница жалобы")) {
      VStack(alignment: .leading, spacing: 10) {
        if let target = model.snapshot.reportTarget {
          Text(target.title).font(.headline)
          HStack {
            Button(copy.text("Report metadata only", "Отправить только метаданные")) {
              actions.perform(model.metadataReportCommand())
            }
            .disabled(model.metadataReportCommand() == nil)
            Button(
              copy.text("Include decrypted evidence…", "Добавить расшифрованное доказательство…")
            ) {
              confirmation = .exportEvidence
            }
            .disabled(model.decryptedEvidenceExportCommand(confirmed: true) == nil)
          }
          Text(
            copy.text(
              "Metadata reporting does not disclose audio. Evidence export is a separate voluntary copy of content you decrypted locally.",
              "Жалоба с метаданными не раскрывает аудио. Экспорт доказательства — отдельная добровольная копия локально расшифрованного содержимого."
            )
          )
          .font(.caption)
          .foregroundStyle(.secondary)
        } else {
          Text(
            copy.text(
              "No protected item is selected for reporting.",
              "Для жалобы не выбран защищённый объект.")
          )
          .foregroundStyle(.secondary)
        }
      }
      .frame(maxWidth: .infinity, alignment: .leading)
      .padding(.vertical, 4)
    }
    .accessibilityElement(children: .contain)
  }
}

private struct PulsarEncryptedMediaOutcomeSection: View {
  let snapshot: PulsarEncryptedMediaSnapshot
  let copy: PulsarEncryptedMediaCopy

  var body: some View {
    VStack(alignment: .leading, spacing: 6) {
      if let outcome = snapshot.actionOutcome {
        Label(outcome.text(locale: copy.locale), systemImage: "checkmark.circle.fill")
          .foregroundStyle(.green)
      }
      if let failure = snapshot.actionFailure {
        Label(failure.text(locale: copy.locale), systemImage: "exclamationmark.triangle.fill")
          .foregroundStyle(.red)
      }
      if snapshot.commandInFlight {
        ProgressView(
          copy.text("Applying protected-media change…", "Применение изменения защищённых медиа…"))
      }
    }
    .accessibilityElement(children: .contain)
  }
}

private struct PulsarEncryptedMediaCopy {
  let locale: PulsarShellLocale

  func text(_ en: String, _ ru: String) -> String { locale == .ru ? ru : en }

  func path(_ path: PulsarEncryptedMediaPath) -> String {
    switch path {
    case .plaintext: text("Plaintext", "Без шифрования")
    case .protectedClip: text("Protected clip", "Защищённый клип")
    case .protectedTrack: text("Protected track", "Защищённый трек")
    case .protectedLive: text("Protected live PTT", "Защищённый live PTT")
    }
  }

  func pathStatus(
    path: PulsarEncryptedMediaPath,
    availability: PulsarEncryptedMediaAvailability
  ) -> String {
    switch availability {
    case .plaintext:
      text("Plaintext path selected", "Выбран путь без шифрования")
    case .encrypted:
      text("End-to-end encrypted path is ready", "Путь со сквозным шифрованием готов")
    case .blocked:
      text(
        "Protected path is blocked; no plaintext fallback",
        "Защищённый путь заблокирован; перехода на plaintext нет")
    }
  }

  func failure(_ code: String?) -> String {
    switch code {
    case nil: text("Ready", "Готово")
    case "surface_not_ready": text("Status is not current", "Статус не актуален")
    case "runtime_disabled": text("Production runtime is disabled", "Production runtime отключён")
    case "ownership_unattested":
      text(
        "Single-owner serialization is not attested",
        "Сериализация единственного владельца не подтверждена")
    case "capability_unavailable":
      text(
        "Reviewed suite and capability are unavailable", "Проверенные suite и capability недоступны"
      )
    case "secure_key_state_unavailable":
      text("Secure key state is unavailable", "Безопасное состояние ключей недоступно")
    case "device_unverified":
      text("This device is not verified", "Это устройство не верифицировано")
    case "membership_stale": text("Membership or epoch is stale", "Участие или эпоха устарели")
    case "unsupported_recipients_require_choice":
      text(
        "Unsupported recipients require an explicit choice",
        "Нужен явный выбор для неподдерживаемых получателей")
    case "protected_send_unavailable":
      text("Protected sender is unavailable", "Защищённая отправка недоступна")
    case "protected_playback_unavailable":
      text("Protected playback is unavailable", "Защищённое воспроизведение недоступно")
    case "protected_live_unavailable":
      text("Protected live PTT is unavailable", "Защищённый live PTT недоступен")
    default: text("Blocked", "Заблокировано")
    }
  }

  func verification(_ value: PulsarEncryptedMediaVerification) -> String {
    switch value {
    case .verified: text("Verified", "Верифицировано")
    case .unverified: text("Not verified", "Не верифицировано")
    case .revoked: text("Revoked", "Отозвано")
    }
  }

  func verificationIcon(_ value: PulsarEncryptedMediaVerification) -> String {
    switch value {
    case .verified: "checkmark.shield.fill"
    case .unverified: "questionmark.diamond.fill"
    case .revoked: "xmark.shield.fill"
    }
  }

  func membership(_ value: PulsarEncryptedMediaMembership) -> String {
    switch value {
    case .current: text("Current", "Актуально")
    case .rotationRequired: text("Rotation required", "Нужна ротация")
    case .removed: text("Removed", "Исключено")
    case .forked: text("Fork detected", "Обнаружено расхождение")
    }
  }

  func grantMode(_ value: PulsarEncryptedMediaGrantMode) -> String {
    switch value {
    case .oneTime: text("One read", "Одно чтение")
    case .timeBound: text("Time-bound", "Ограничено по времени")
    }
  }

  func grantStatus(_ value: PulsarEncryptedMediaGrantStatus) -> String {
    switch value {
    case .active: text("Active", "Активно")
    case .expired: text("Expired", "Истекло")
    case .revoked: text("Revoked", "Отозвано")
    }
  }

  func confirmationTitle(_ value: PulsarEncryptedMediaConfirmation?) -> String {
    switch value {
    case .excludeUnsupported: text("Keep this send encrypted?", "Сохранить шифрование отправки?")
    case .revokeDevice: text("Revoke this device?", "Отозвать это устройство?")
    case .deviceTransfer: text("Transfer current access?", "Передать текущий доступ?")
    case .userHeldRecovery:
      text("Use user-held recovery?", "Использовать пользовательское восстановление?")
    case .createHistoryGrant: text("Grant selected history?", "Предоставить выбранную историю?")
    case .revokeHistoryGrant: text("Revoke this history grant?", "Отозвать этот доступ к истории?")
    case .exportEvidence:
      text("Disclose decrypted evidence?", "Передать расшифрованное доказательство?")
    case nil: text("Confirm protected-media action", "Подтвердите действие с защищёнными медиа")
    }
  }

  func confirmationMessage(_ value: PulsarEncryptedMediaConfirmation) -> String {
    switch value {
    case .excludeUnsupported:
      text(
        "Unsupported recipients will be removed from this exact target set. No plaintext copy will be sent.",
        "Неподдерживаемые получатели будут исключены из этого набора. Копия без шифрования не отправится."
      )
    case .revokeDevice:
      text(
        "Pending transfers and grants involving this device will be revoked. A group rotation is required afterward.",
        "Ожидающие передачи и разрешения для этого устройства будут отозваны. После этого потребуется ротация группы."
      )
    case .deviceTransfer:
      text(
        "Only the current epoch is transferred to the verified device. History is not included.",
        "Верифицированному устройству передаётся только текущая эпоха. История не включается.")
    case .userHeldRecovery:
      text(
        "Continue only with a separately reviewed recovery capability you control. The coordinator cannot recover keys.",
        "Продолжайте только с отдельно проверенным recovery-доступом под вашим контролем. Координатор не может восстановить ключи."
      )
    case .createHistoryGrant:
      text(
        "This grants only the selected object and epoch range to the named verified device until expiry.",
        "Доступ выдаётся только к выбранному объекту и диапазону эпох указанного верифицированного устройства до истечения срока."
      )
    case .revokeHistoryGrant:
      text(
        "Future reads through this grant will fail. Content already copied by the recipient cannot be revoked.",
        "Будущие чтения по этому разрешению будут отклонены. Уже скопированное получателем содержимое отозвать нельзя."
      )
    case .exportEvidence:
      text(
        "A separate decrypted copy selected by you will leave the E2EE boundary for moderation storage. Metadata-only reporting remains available without this disclosure.",
        "Отдельная выбранная вами расшифрованная копия покинет границу E2EE и попадёт в хранилище модерации. Жалоба только с метаданными доступна без раскрытия."
      )
    }
  }
}
