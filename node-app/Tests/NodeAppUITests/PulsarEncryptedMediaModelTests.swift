import Foundation
import Testing

@testable import NodeAppUI

struct PulsarEncryptedMediaModelTests {
  private let now = Date(timeIntervalSince1970: 1_800_000_000)
  private let future = Date(timeIntervalSince1970: 1_802_000_000)
  private let thisDeviceID = "device_this_01"
  private let newDeviceID = "device_new_02"
  private let lostDeviceID = "device_lost_03"
  private let objectID = "object_protected_01"
  private let grantID = "grant_history_01"

  private var label: PulsarLocalizedLabel {
    .init(key: "fixture", en: "Fixture", ru: "Фикстура")
  }

  private var actions: [PulsarActionCapability] {
    [
      "refresh", "select_path", "verify_device", "revoke_device",
      "device_transfer", "user_held_recovery", "confirm_unsupported_exclusion",
      "create_history_grant", "revoke_history_grant", "report_metadata",
      "export_decrypted_evidence",
    ].map { .init(action: $0, label: label) }
  }

  private func device(
    _ id: String,
    verification: PulsarEncryptedMediaVerification = .verified,
    current: Bool = true,
    thisDevice: Bool = false,
    canRevoke: Bool = true
  ) -> PulsarEncryptedMediaDevice {
    .init(
      id: id,
      label: label,
      verification: verification,
      currentMember: current,
      isThisDevice: thisDevice,
      canRevoke: canRevoke,
      revision: 1)
  }

  private func readySnapshot() -> PulsarEncryptedMediaSnapshot {
    .init(
      state: .ready,
      selectedPath: .protectedClip,
      capabilityAdvertised: true,
      reviewedSuiteSelected: true,
      runtimeWiringApproved: true,
      ownership: .crossProcessSerialized,
      components: .init(
        keyStateReady: true,
        protectedSendReady: true,
        protectedPlaybackReady: true,
        protectedLiveReady: true,
        sameRepositoryWitness: true),
      thisDeviceVerification: .verified,
      membership: .current,
      epoch: 9,
      devices: [
        device(thisDeviceID, thisDevice: true, canRevoke: false),
        device(newDeviceID),
        device(lostDeviceID),
      ],
      unsupportedExclusionConfirmed: true,
      recoveryModes: [.deviceTransfer, .userHeldRecovery],
      recoveryTargetDeviceID: newDeviceID,
      historyRecoverable: true,
      historyGrants: [
        .init(
          id: grantID,
          title: "Selected voice",
          objectID: objectID,
          recipientDeviceID: newDeviceID,
          firstEpoch: 3,
          lastEpoch: 5,
          mode: .oneTime,
          expiresAt: future,
          status: .active)
      ],
      historyGrantDraft: .init(
        objectID: objectID,
        title: "Selected voice",
        recipientDeviceID: newDeviceID,
        firstEpoch: 3,
        lastEpoch: 5,
        mode: .oneTime,
        expiresAt: future),
      reportTarget: .init(
        objectID: objectID,
        title: "Selected voice",
        canReportMetadata: true,
        canExportDecryptedEvidence: true,
        decryptedEvidenceReady: true,
        consentVersion: "report-evidence-v1"),
      actions: actions)
  }

  @MainActor
  @Test("Protected status requires every runtime and ownership witness")
  func protectedStatusFailsClosed() {
    let model = PulsarEncryptedMediaModel()
    #expect(model.availability(for: .plaintext) == .plaintext)
    #expect(model.availability(for: .protectedClip) == .blocked)
    #expect(model.selectPathCommand(.protectedClip) == nil)

    var snapshot = readySnapshot()
    snapshot.ownership = .unattested
    model.replace(snapshot, now: now)
    #expect(model.snapshot.selectedPath == .protectedClip)
    #expect(model.snapshot.runtimeWiringApproved == false)
    #expect(model.snapshot.capabilityAdvertised == false)
    #expect(model.pathFailure(.protectedClip) == "runtime_disabled")

    snapshot = readySnapshot()
    snapshot.components.sameRepositoryWitness = false
    model.replace(snapshot, now: now)
    #expect(model.availability(for: .protectedLive) == .blocked)
    #expect(model.deviceTransferCommand() == nil)
    #expect(model.userHeldRecoveryCommand(confirmed: true) == nil)
    #expect(model.createHistoryGrantCommand(confirmed: true, now: now) == nil)
    #expect(model.decryptedEvidenceExportCommand(confirmed: true) == nil)
  }

  @MainActor
  @Test("Unsupported targets block encryption without selecting plaintext")
  func unsupportedTargetsNeverDowngrade() {
    var snapshot = readySnapshot()
    snapshot.unsupportedRecipients = [
      .init(id: "device_legacy_04", label: label),
      .init(id: "device_legacy_04", label: label),
    ]
    snapshot.unsupportedExclusionConfirmed = false
    let model = PulsarEncryptedMediaModel(snapshot: snapshot)
    model.replace(snapshot, now: now)

    #expect(model.snapshot.selectedPath == .protectedClip)
    #expect(model.snapshot.unsupportedRecipients.count == 1)
    #expect(model.availability(for: .protectedClip) == .blocked)
    #expect(model.selectPathCommand(.protectedClip) == nil)
    #expect(model.unsupportedExclusionCommand(confirmed: false) == nil)
    #expect(
      model.unsupportedExclusionCommand(confirmed: true)
        == .confirmUnsupportedExclusion(["device_legacy_04"]))

    snapshot.unsupportedExclusionConfirmed = true
    model.replace(snapshot, now: now)
    #expect(model.availability(for: .protectedClip) == .encrypted)
    #expect(model.selectPathCommand(.plaintext) == .selectPath(.plaintext))
  }

  @MainActor
  @Test("Verification revoke transfer and recovery intents are exact and confirmed")
  func deviceLifecycleCommandsFailClosed() {
    var snapshot = readySnapshot()
    snapshot.devices.append(
      device("device_pending_05", verification: .unverified, canRevoke: false))
    let model = PulsarEncryptedMediaModel(snapshot: snapshot)
    model.replace(snapshot, now: now)

    #expect(model.verifyDeviceCommand("device_pending_05") == .verifyDevice("device_pending_05"))
    #expect(model.verifyDeviceCommand(newDeviceID) == nil)
    #expect(model.revokeDeviceCommand(lostDeviceID, confirmed: false) == nil)
    #expect(model.revokeDeviceCommand(lostDeviceID, confirmed: true) == .revokeDevice(lostDeviceID))
    #expect(model.revokeDeviceCommand(thisDeviceID, confirmed: true) == nil)
    #expect(model.deviceTransferCommand() == .beginDeviceTransfer(newDeviceID))
    #expect(model.userHeldRecoveryCommand(confirmed: false) == nil)
    #expect(model.userHeldRecoveryCommand(confirmed: true) == .beginUserHeldRecovery)

    snapshot.membership = .rotationRequired
    model.replace(snapshot, now: now)
    #expect(model.deviceTransferCommand() == nil)
    #expect(model.availability(for: .protectedClip) == .blocked)
  }

  @MainActor
  @Test("History and decrypted report evidence require separate explicit consent")
  func historyAndReportConsentAreSeparate() {
    let snapshot = readySnapshot()
    let model = PulsarEncryptedMediaModel(snapshot: snapshot)
    model.replace(snapshot, now: now)

    #expect(model.createHistoryGrantCommand(confirmed: false, now: now) == nil)
    #expect(
      model.createHistoryGrantCommand(confirmed: true, now: now)
        == .createHistoryGrant(
          objectID: objectID,
          recipientDeviceID: newDeviceID,
          firstEpoch: 3,
          lastEpoch: 5,
          mode: .oneTime,
          expiresAt: future))
    #expect(model.revokeHistoryGrantCommand(grantID, confirmed: false) == nil)
    #expect(
      model.revokeHistoryGrantCommand(grantID, confirmed: true)
        == .revokeHistoryGrant(grantID))
    #expect(model.metadataReportCommand() == .reportMetadata(objectID))
    #expect(model.decryptedEvidenceExportCommand(confirmed: false) == nil)
    #expect(
      model.decryptedEvidenceExportCommand(confirmed: true)
        == .exportDecryptedEvidence(
          objectID: objectID, consentVersion: "report-evidence-v1"))

    var denied = snapshot
    denied.reportTarget = .init(
      objectID: objectID,
      title: "Selected voice",
      canReportMetadata: true,
      canExportDecryptedEvidence: false,
      decryptedEvidenceReady: true,
      consentVersion: "report-evidence-v1")
    model.replace(denied, now: now)
    #expect(model.metadataReportCommand() != nil)
    #expect(model.decryptedEvidenceExportCommand(confirmed: true) == nil)
  }

  @Test("Opaque device object grant and report identifiers are redacted")
  func descriptionsAreRedacted() {
    let snapshot = readySnapshot()
    let values = [
      snapshot.description,
      snapshot.devices[0].description,
      snapshot.historyGrants[0].description,
      snapshot.reportTarget?.description ?? "",
      PulsarEncryptedMediaCommand.exportDecryptedEvidence(
        objectID: objectID, consentVersion: "report-evidence-v1"
      ).description,
    ]
    for rendered in values {
      #expect(!rendered.contains(thisDeviceID))
      #expect(!rendered.contains(newDeviceID))
      #expect(!rendered.contains(objectID))
      #expect(!rendered.contains(grantID))
    }
  }

  @Test("Portable contract and macOS source keep runtime dark and consent explicit")
  func portableAndSourceContracts() throws {
    let package = URL(fileURLWithPath: #filePath)
      .deletingLastPathComponent().deletingLastPathComponent()
      .deletingLastPathComponent()
    let root = package.deletingLastPathComponent()
    let data = try Data(
      contentsOf:
        root.appendingPathComponent("protocol/macos-encrypted-media-client-path-v1.json"))
    let contract = try #require(JSONSerialization.jsonObject(with: data) as? [String: Any])
    #expect(contract["contract"] as? String == "macos-encrypted-media-client-path.v1")
    #expect(contract["status"] as? String == "production-dark")
    #expect(contract["paths"] as? [String] == PulsarEncryptedMediaPath.allCases.map(\.rawValue))
    #expect(contract["runtime_wired"] as? Bool == false)
    #expect(contract["capability_advertised"] as? Bool == false)

    let view = try String(
      contentsOf: package.appendingPathComponent(
        "Sources/NodeAppUI/PulsarEncryptedMediaView.swift"),
      encoding: .utf8)
    let composition = try String(
      contentsOf: package.appendingPathComponent(
        "Sources/NodeApp/MacEncryptedMediaClientPathComposition.swift"),
      encoding: .utf8)
    let main = try String(
      contentsOf: package.appendingPathComponent("Sources/NodeApp/main.swift"),
      encoding: .utf8)
    for seam in [
      "PulsarEncryptedMediaView", ".confirmationDialog(",
      "model.unsupportedExclusionCommand(confirmed: true)",
      "model.createHistoryGrantCommand(confirmed: true)",
      "model.decryptedEvidenceExportCommand(confirmed: true)",
      ".keyboardShortcut(\"r\", modifiers: .command)",
      ".accessibilityElement(children: .contain)",
    ] {
      #expect(view.contains(seam))
    }
    for seam in [
      "MacE2EEKeyStateRepository()", "MacProtectedMediaSendService(",
      "MacProtectedMediaPlaybackService(", "MacE2EELiveSessionFactory(",
      "keyState: repository", "private let ownershipLease",
      "ownershipLease.coversOtherProcesses", "sameRepositoryWitness: true",
    ] {
      #expect(composition.contains(seam))
    }
    #expect(!main.contains("MacEncryptedMediaClientPathComposition"))
    #expect(!main.contains("PulsarEncryptedMediaView"))
    #expect(!view.contains("onTapGesture"))
    #expect(!view.contains("Text(device.id)"))
    #expect(!view.contains("Text(grant.id)"))
    #expect(!view.contains("Text(target.objectID)"))
  }
}
