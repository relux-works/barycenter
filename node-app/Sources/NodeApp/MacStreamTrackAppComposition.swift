import AppKit
import Foundation
import NodeAppUI
import NodeCore

/// Main-actor bridge for the accepted shared track model. It enables durable
/// intake and generic audio_track upload, but deliberately publishes no
/// queue/playback capability while the production codec ADR remains no-go.
@MainActor
final class MacStreamTrackAppComposition {
  private let client: PhaseOneAppClient
  private let store: MacStreamTrackDraftStore
  private let shellModel: PulsarShellModel
  private let targetsModel: PulsarTargetsInboxModel
  private let model: PulsarStreamTrackModel

  private var operation: Task<Void, Never>?
  private var manifest: ContentPolicyManifest?
  private var stopped = false

  init(
    bundle: CredentialBundle,
    supportRoot: URL,
    shellModel: PulsarShellModel,
    targetsModel: PulsarTargetsInboxModel,
    model: PulsarStreamTrackModel
  ) throws {
    client = try PhaseOneAppClient(bundle: bundle)
    store = try MacStreamTrackDraftStore(
      root: supportRoot.appendingPathComponent("StreamTrack", isDirectory: true))
    self.shellModel = shellModel
    self.targetsModel = targetsModel
    self.model = model
  }

  func start() {
    guard !stopped else { return }
    do {
      if let record = try store.load() {
        replaceDraft(record, phase: .retained, failure: nil)
      }
    } catch {
      var snapshot = model.snapshot
      snapshot.state = .coordinatorError
      snapshot.failure = .serviceUnavailable
      model.replace(snapshot)
    }
    refresh()
  }

  func shutdown() {
    stopped = true
    operation?.cancel()
    operation = nil
  }

  func intake(_ url: URL) {
    guard !stopped, operation == nil else { return }
    let accessed = url.startAccessingSecurityScopedResource()
    let store = self.store
    operation = Task { [weak self] in
      guard let self else { return }
      defer {
        if accessed { url.stopAccessingSecurityScopedResource() }
        operation = nil
      }
      do {
        let record = try await Task.detached(priority: .userInitiated) {
          try store.importFile(url)
        }.value
        guard !Task.isCancelled else { return }
        replaceDraft(record, phase: .retained, failure: nil)
      } catch {
        setFailure(.serviceUnavailable)
      }
    }
  }

  func refresh() {
    guard !stopped, operation == nil else { return }
    operation = Task { [weak self] in
      guard let self else { return }
      defer { operation = nil }
      do {
        let locale: ContentPolicyLocale = shellModel.locale == .ru ? .ru : .en
        let currentManifest = try await client.contentPolicy(locale: locale)
        let grant = try await client.currentContentPolicyGrant()
        guard !Task.isCancelled else { return }
        manifest = currentManifest
        var snapshot = model.snapshot
        snapshot.state = .ready
        snapshot.failure = nil
        snapshot.contentPolicyState =
          grant.current && grant.termsAccepted
            && grant.version == currentManifest.version
            && grant.policyHash == currentManifest.policyHash ? "current" : "required"
        snapshot.targets = targetsModel.snapshot.targets.filter {
          $0.capabilities.contains("stream_track")
        }
        snapshot.activeAirAvailable = shellModel.snapshot.airs.current != nil
        snapshot.actions = Self.uploadActions
        model.replace(snapshot)
      } catch {
        var snapshot = model.snapshot
        snapshot.state = .offline
        snapshot.failure = .offline
        snapshot.actions = []
        model.replace(snapshot)
      }
    }
  }

  func perform(
    _ command: PulsarStreamTrackCommand,
    rightsAcknowledged: Bool = false
  ) {
    guard !stopped, operation == nil, model.buildCommand(command) == command else { return }
    switch command {
    case .acceptPolicy:
      acceptPolicy()
    case .upload:
      guard rightsAcknowledged else { return }
      upload(command)
    case .retry:
      guard rightsAcknowledged else { return }
      guard model.applyOptimistic(command), let draft = model.snapshot.draft else { return }
      upload(.upload(localID: draft.localID))
    case .delete(let localID, let confirmed):
      guard confirmed else { return }
      delete(localID: localID)
    case .queue, .replace, .pause, .seek, .resume, .report:
      // These commands cannot pass buildCommand because this production
      // adapter publishes no corresponding server action under no-go.
      return
    }
  }

  private func acceptPolicy() {
    guard let manifest, model.buildCommand(.acceptPolicy) != nil else { return }
    let ru = shellModel.locale == .ru
    let alert = NSAlert()
    alert.alertStyle = .informational
    alert.messageText = manifest.title
    alert.informativeText = """
      \(manifest.rightsText)

      \(manifest.consentText)

      Terms: \(manifest.termsURL.absoluteString)
      Content Guidelines: \(manifest.contentGuidelinesURL.absoluteString)
      Version: \(manifest.version)
      """
    alert.addButton(withTitle: ru ? "Принять" : "Accept")
    alert.addButton(withTitle: ru ? "Отмена" : "Cancel")
    guard alert.runModal() == .alertFirstButtonReturn else { return }
    operation = Task { [weak self] in
      guard let self else { return }
      defer { operation = nil }
      do {
        let grant = try await client.acceptContentPolicy(manifest)
        guard grant.current, grant.termsAccepted,
          grant.version == manifest.version, grant.policyHash == manifest.policyHash
        else { throw PhaseOneClientError.invalidResponse }
        var snapshot = model.snapshot
        snapshot.contentPolicyState = "current"
        snapshot.failure = nil
        model.replace(snapshot)
      } catch { setFailure(.policyRequired) }
    }
  }

  private func upload(_ command: PulsarStreamTrackCommand) {
    guard case .upload(let localID) = command,
      model.buildCommand(command) != nil,
      model.applyOptimistic(command),
      let draft = model.snapshot.draft
    else { return }
    let client = self.client
    let store = self.store
    operation = Task { [weak self] in
      guard let self else { return }
      defer { operation = nil }
      do {
        let confirmation = try await client.uploadTrack(
          fileURL: store.fileURL(localID: localID),
          title: draft.title,
          idempotencyKey: "track:\(localID)",
          rightsAcknowledged: true
        ) { [weak self] offset, _ in
          guard let self else { return }
          if var record = try? store.load(), record.localID == localID {
            record.uploadOffset = offset
            try? store.update(record)
            Task { @MainActor [weak self] in
              self?.replaceDraft(record, phase: .uploading, failure: nil)
            }
          }
        }
        guard !Task.isCancelled,
          var record = try store.load(), record.localID == localID
        else { return }
        record.uploadOffset = record.localByteCount
        record.mediaID = confirmation.mediaID
        try store.update(record)
        // Generic upload is accepted. A production variant manifest is
        // not, so processing can never be promoted to ready here.
        replaceDraft(record, phase: .processing, failure: .variantUnavailable)
      } catch {
        if let record = try? store.load(), record.localID == localID {
          replaceDraft(record, phase: .failed, failure: .serviceUnavailable)
        } else {
          setFailure(.serviceUnavailable)
        }
      }
    }
  }

  private func delete(localID: String) {
    guard model.buildCommand(.delete(localID: localID, confirmed: true)) != nil else { return }
    let mediaID = model.snapshot.draft?.mediaID
    operation = Task { [weak self] in
      guard let self else { return }
      defer { operation = nil }
      do {
        if let mediaID { try await client.deleteMedia(mediaID) }
        try store.delete(localID: localID)
        var snapshot = model.snapshot
        snapshot.draft = nil
        snapshot.confirmedDeletedLocalID = localID
        snapshot.failure = nil
        model.replace(snapshot)
      } catch { setFailure(.serviceUnavailable) }
    }
  }

  private func replaceDraft(
    _ record: MacStreamTrackDraftRecord,
    phase: PulsarStreamTrackDraftPhase,
    failure: PulsarStreamTrackFailure?
  ) {
    var snapshot = model.snapshot
    snapshot.draft = PulsarStreamTrackDraft(
      localID: record.localID,
      localByteCount: record.localByteCount,
      title: record.displayName,
      clientMIME: record.clientMIME,
      phase: phase,
      mediaID: record.mediaID,
      uploadOffset: record.uploadOffset,
      processingPercent: 0,
      failure: failure)
    snapshot.failure = failure
    model.replace(snapshot)
  }

  private func setFailure(_ failure: PulsarStreamTrackFailure) {
    var snapshot = model.snapshot
    snapshot.failure = failure
    model.replace(snapshot)
  }

  private static let uploadActions: [PulsarActionCapability] = [
    action("accept_policy", "Review and accept", "Ознакомиться и принять"),
    action("upload", "Upload track", "Загрузить трек"),
    action("retry", "Try again", "Повторить"),
    action("delete", "Delete", "Удалить"),
  ]

  private static func action(_ name: String, _ en: String, _ ru: String) -> PulsarActionCapability {
    PulsarActionCapability(
      action: name,
      label: .init(key: "stream_track.action.\(name)", en: en, ru: ru))
  }
}
