import Foundation
import SwiftUI
import UniformTypeIdentifiers

struct PulsarStreamTrackView: View {
  @Bindable var model: PulsarStreamTrackModel
  let locale: PulsarShellLocale
  let actions: PulsarStreamTrackActions

  @State private var showingImporter = false
  @State private var confirmingDelete = false
  @State private var rightsCommand: PulsarStreamTrackCommand?
  @State private var reportDetails = ""

  var body: some View {
    ScrollView {
      VStack(alignment: .leading, spacing: 14) {
        HStack {
          Label(text("Long track", "Длинный трек"), systemImage: "waveform.badge.plus")
            .font(.title3.bold())
          Spacer()
          Button(text("Refresh", "Обновить"), action: actions.refresh)
        }
        Text(
          text(
            "Large audio stays on this Mac and uploads in resumable chunks.",
            "Большой аудиофайл хранится на этом Mac и загружается возобновляемыми частями.")
        )
        .foregroundStyle(.secondary)

        intake
        if let draft = model.snapshot.draft { draftSection(draft) }
        consentAndTargets
        playback
        failure
      }
      .padding(18)
      .frame(maxWidth: .infinity, alignment: .leading)
    }
    .fileImporter(
      isPresented: $showingImporter,
      allowedContentTypes: [.audio],
      allowsMultipleSelection: false
    ) { result in
      guard case .success(let urls) = result, let url = urls.first else { return }
      actions.intake(url)
    }
    .confirmationDialog(
      text("Delete this long-track draft?", "Удалить черновик длинного трека?"),
      isPresented: $confirmingDelete, titleVisibility: .visible
    ) {
      Button(text("Delete permanently", "Удалить навсегда"), role: .destructive) {
        guard let draft = model.snapshot.draft else { return }
        actions.perform(model.buildCommand(.delete(localID: draft.localID, confirmed: true)))
      }
      Button(text("Cancel", "Отмена"), role: .cancel) {}
    }
    .confirmationDialog(
      text(
        "Confirm that you have the rights to upload and share this audio.",
        "Подтвердите право загружать и распространять это аудио."),
      isPresented: Binding(
        get: { rightsCommand != nil },
        set: { if !$0 { rightsCommand = nil } }),
      titleVisibility: .visible
    ) {
      Button(text("I confirm the rights", "Подтверждаю права")) {
        actions.perform(rightsCommand, rightsAcknowledged: true)
        rightsCommand = nil
      }
      Button(text("Cancel", "Отмена"), role: .cancel) { rightsCommand = nil }
    } message: {
      Text(
        text(
          "This confirmation applies to this upload attempt and does not prove ownership.",
          "Подтверждение относится к этой попытке загрузки и не доказывает владение."))
    }
  }

  private var intake: some View {
    HStack(spacing: 10) {
      Button(text("Choose long audio…", "Выбрать длинное аудио…")) {
        showingImporter = true
      }
      .buttonStyle(.borderedProminent)
      .keyboardShortcut("l", modifiers: [.command, .shift])
      Label(
        text("or drop audio here", "или перетащите аудио сюда"),
        systemImage: "square.and.arrow.down"
      )
      .frame(maxWidth: .infinity, minHeight: 44, alignment: .leading)
      .dropDestination(for: URL.self) { urls, _ in
        guard let url = urls.first else { return false }
        actions.intake(url)
        return true
      }
    }
    .accessibilityElement(children: .contain)
  }

  @ViewBuilder
  private func draftSection(_ draft: PulsarStreamTrackDraft) -> some View {
    GroupBox(text("Draft and upload", "Черновик и загрузка")) {
      VStack(alignment: .leading, spacing: 9) {
        LabeledContent(text("File", "Файл"), value: draft.title)
        LabeledContent(
          text("Size", "Размер"),
          value: ByteCountFormatter.string(fromByteCount: draft.localByteCount, countStyle: .file))
        LabeledContent(
          text("State", "Состояние"), value: label(draft.phaseLabel, fallback: draft.phase.rawValue)
        )
        ProgressView(value: Double(draft.uploadOffset), total: Double(max(1, draft.localByteCount)))
        {
          Text(text("Upload", "Загрузка"))
        } currentValueLabel: {
          Text("\(draft.uploadOffset) / \(draft.localByteCount) bytes")
        }
        .accessibilityValue("\(draft.uploadOffset) of \(draft.localByteCount) bytes")
        if draft.phase == .processing || draft.phase == .ready {
          ProgressView(value: Double(draft.processingPercent), total: 100) {
            Text(text("Server processing", "Обработка на сервере"))
          } currentValueLabel: {
            Text("\(draft.processingPercent)%")
          }
          .accessibilityValue("\(draft.processingPercent) percent")
        }
        HStack {
          Button(actionLabel("upload", fallback: text("Upload track", "Загрузить трек"))) {
            rightsCommand = model.buildCommand(.upload(localID: draft.localID))
          }
          .disabled(model.buildCommand(.upload(localID: draft.localID)) == nil)
          Button(actionLabel("retry", fallback: text("Try again", "Повторить"))) {
            rightsCommand = model.buildCommand(.retry(localID: draft.localID))
          }
          .disabled(model.buildCommand(.retry(localID: draft.localID)) == nil)
          Button(text("Delete", "Удалить"), role: .destructive) { confirmingDelete = true }
            .disabled(model.buildCommand(.delete(localID: draft.localID, confirmed: true)) == nil)
        }
      }
      .frame(maxWidth: .infinity, alignment: .leading)
    }
  }

  private var consentAndTargets: some View {
    GroupBox(text("Rights and destination", "Права и получатели")) {
      VStack(alignment: .leading, spacing: 10) {
        HStack {
          Label(
            model.snapshot.contentPolicyState == "current"
              ? text("Current policy accepted", "Текущие правила приняты")
              : text("Policy acceptance required", "Нужно принять правила"),
            systemImage: model.snapshot.contentPolicyState == "current"
              ? "checkmark.shield" : "exclamationmark.shield")
          Spacer()
          Button(
            actionLabel(
              "accept_policy", fallback: text("Review and accept", "Ознакомиться и принять"))
          ) {
            actions.perform(model.buildCommand(.acceptPolicy))
          }
          .disabled(model.buildCommand(.acceptPolicy) == nil)
        }
        HStack {
          Button(text("Current Air", "Текущий эфир")) {
            _ = model.selectAudience(.currentAir)
          }
          .disabled(!model.snapshot.activeAirAvailable)
          Button(text("Explicit targets", "Выбранные получатели")) {
            _ = model.selectAudience(.explicit)
          }
          .disabled(model.snapshot.selectedReferences.isEmpty)
          Spacer()
          Picker(text("Insertion", "Режим"), selection: insertionBinding) {
            Text(text("Queue", "В очередь")).tag(PulsarStreamTrackInsertion.queue)
            Text(text("Replace", "Заменить")).tag(PulsarStreamTrackInsertion.replace)
          }
          .pickerStyle(.segmented)
          .frame(maxWidth: 260)
        }
        if model.snapshot.targets.isEmpty {
          Text(
            text(
              "No current target advertises streamed-track playback.",
              "Нет получателя с поддержкой потоковых треков.")
          )
          .font(.callout)
          .foregroundStyle(.secondary)
        } else {
          ForEach(Array(model.snapshot.targets.enumerated()), id: \.element.reference) {
            _, target in
            Toggle(
              target.label.text(locale: locale),
              isOn: targetBinding(target.reference)
            )
            .accessibilityHint(
              text(
                "Select this exact capability-bound recipient",
                "Выбрать этого получателя с точной capability"))
          }
        }
        if let draft = model.snapshot.draft, let mediaID = draft.mediaID,
          let audience = model.snapshot.selectedAudience
        {
          HStack {
            Button(actionLabel("queue", fallback: text("Add to queue", "Добавить в очередь"))) {
              actions.perform(
                model.buildCommand(
                  .queue(
                    mediaID: mediaID, audience: audience,
                    targets: commandTargets(audience))))
            }
            .disabled(
              model.buildCommand(
                .queue(
                  mediaID: mediaID, audience: audience,
                  targets: commandTargets(audience))) == nil)
            Button(actionLabel("replace", fallback: text("Replace current", "Заменить текущий"))) {
              actions.perform(
                model.buildCommand(
                  .replace(
                    mediaID: mediaID, audience: audience,
                    targets: commandTargets(audience))))
            }
            .disabled(
              model.buildCommand(
                .replace(
                  mediaID: mediaID, audience: audience,
                  targets: commandTargets(audience))) == nil)
          }
        }
      }
      .frame(maxWidth: .infinity, alignment: .leading)
    }
  }

  private var playback: some View {
    let value = model.snapshot.playback
    return GroupBox(text("Now playing", "Сейчас играет")) {
      VStack(alignment: .leading, spacing: 9) {
        LabeledContent(
          text("State", "Состояние"), value: label(value.phaseLabel, fallback: value.phase.rawValue)
        )
        ProgressView(
          value: Double(value.audiblePositionMS), total: Double(max(1, value.durationMS))
        ) {
          Text(text("Audible position", "Слышимая позиция"))
        } currentValueLabel: {
          Text("\(duration(value.audiblePositionMS)) / \(duration(value.durationMS))")
        }
        .accessibilityValue(
          "\(value.audiblePositionMS) of \(value.durationMS) milliseconds")
        HStack {
          Button(actionLabel("pause", fallback: text("Pause", "Пауза"))) {
            guard let id = value.streamID else { return }
            actions.perform(
              model.buildCommand(
                .pause(
                  streamID: id, playbackGeneration: value.playbackGeneration)))
          }
          .disabled(pauseCommand == nil)
          Button(actionLabel("seek", fallback: text("Seek +30 s", "Вперёд 30 с"))) {
            guard let id = value.streamID else { return }
            let target = min(value.durationMS, value.audiblePositionMS + 30_000)
            actions.perform(
              model.buildCommand(
                .seek(
                  streamID: id, positionMS: target,
                  playbackGeneration: value.playbackGeneration,
                  seekGeneration: value.seekGeneration)))
          }
          .disabled(seekCommand == nil)
          Button(actionLabel("resume", fallback: text("Resume", "Продолжить"))) {
            guard let id = value.streamID else { return }
            actions.perform(
              model.buildCommand(
                .resume(
                  streamID: id, playbackGeneration: value.playbackGeneration)))
          }
          .disabled(resumeCommand == nil)
        }
        if let mediaID = model.snapshot.draft?.mediaID {
          TextField(text("Report details", "Детали жалобы"), text: $reportDetails)
          Button(actionLabel("report", fallback: text("Report", "Пожаловаться"))) {
            actions.perform(
              model.buildCommand(
                .report(
                  mediaID: mediaID, details: reportDetails)))
          }
          .disabled(model.buildCommand(.report(mediaID: mediaID, details: reportDetails)) == nil)
        }
      }
      .frame(maxWidth: .infinity, alignment: .leading)
    }
  }

  @ViewBuilder private var failure: some View {
    if let failure = model.snapshot.failure {
      Label(
        label(model.snapshot.failureLabel, fallback: failure.rawValue),
        systemImage: "exclamationmark.triangle.fill"
      )
      .foregroundStyle(.orange)
      .accessibilityElement(children: .combine)
    }
  }

  private var insertionBinding: Binding<PulsarStreamTrackInsertion> {
    Binding(
      get: { model.snapshot.selectedInsertion ?? .queue },
      set: { _ = model.selectInsertion($0) })
  }

  private func targetBinding(_ reference: String) -> Binding<Bool> {
    Binding(
      get: { model.snapshot.selectedReferences.contains(reference) },
      set: { selected in
        var references = model.snapshot.selectedReferences.filter { $0 != reference }
        if selected { references.append(reference) }
        if model.selectTargets(references), !references.isEmpty {
          _ = model.selectAudience(.explicit)
        }
      })
  }

  private var pauseCommand: PulsarStreamTrackCommand? {
    guard let id = model.snapshot.playback.streamID else { return nil }
    return model.buildCommand(
      .pause(
        streamID: id, playbackGeneration: model.snapshot.playback.playbackGeneration))
  }

  private var seekCommand: PulsarStreamTrackCommand? {
    let value = model.snapshot.playback
    guard let id = value.streamID else { return nil }
    return model.buildCommand(
      .seek(
        streamID: id, positionMS: min(value.durationMS, value.audiblePositionMS + 30_000),
        playbackGeneration: value.playbackGeneration, seekGeneration: value.seekGeneration))
  }

  private var resumeCommand: PulsarStreamTrackCommand? {
    guard let id = model.snapshot.playback.streamID else { return nil }
    return model.buildCommand(
      .resume(
        streamID: id, playbackGeneration: model.snapshot.playback.playbackGeneration))
  }

  private func commandTargets(_ audience: PulsarStreamTrackAudience) -> [String] {
    audience == .explicit ? model.snapshot.selectedReferences : []
  }

  private func actionLabel(_ action: String, fallback: String) -> String {
    model.snapshot.actions.first { $0.action == action }?.label.text(locale: locale) ?? fallback
  }

  private func label(_ value: PulsarLocalizedLabel?, fallback: String) -> String {
    value?.text(locale: locale) ?? fallback
  }

  private func text(_ en: String, _ ru: String) -> String { locale == .ru ? ru : en }

  private func duration(_ milliseconds: Int64) -> String {
    let seconds = max(0, milliseconds / 1_000)
    return String(format: "%d:%02d", seconds / 60, seconds % 60)
  }
}
