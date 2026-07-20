import Foundation
import NodeAppUI
import NodeCore

/// A future runtime must retain an operating-system-backed exclusive lease for
/// the entire lifetime of the encrypted-media services. No concrete lease is
/// supplied here, so this production-dark composition cannot be reached from
/// the app entry point by configuration, environment, or a feature flag.
protocol MacEncryptedMediaGenerationOwnershipLease: AnyObject, Sendable {
  var scope: String { get }
  var coversOtherProcesses: Bool { get }
}

enum MacEncryptedMediaClientPathCompositionFailure: Error, Equatable {
  case ownershipUnattested
}

/// Creates every macOS E2EE client service around exactly one repository.
/// Keeping construction in one place prevents independent repository loads or
/// duplicate generation owners. The retained cross-process lease is required
/// before the live factory can accept its own ownership attestation.
final class MacEncryptedMediaClientPathComposition: @unchecked Sendable {
  let keyState: MacE2EEKeyStateRepository
  let protectedSend: MacProtectedMediaSendService
  let protectedPlayback: MacProtectedMediaPlaybackService
  let protectedLive: MacE2EELiveSessionFactory

  private let ownershipLease: any MacEncryptedMediaGenerationOwnershipLease

  init(
    ownershipLease: any MacEncryptedMediaGenerationOwnershipLease,
    sealer: any MacProtectedMediaSealing,
    uploader: any MacProtectedMediaUploading,
    opener: any MacProtectedMediaOpening,
    playbackTransport: any MacProtectedMediaPlaybackTransport,
    liveDerivation: any MacE2EELiveSessionDeriving,
    liveAuthorization: any MacE2EELiveAuthorizationChecking,
    ciphertextRoot: URL,
    plaintextDraftRoot: URL,
    playbackCacheRoot: URL,
    playbackCacheInstallationSecret: Data
  ) throws {
    guard ownershipLease.coversOtherProcesses,
      !ownershipLease.scope.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty
    else { throw MacEncryptedMediaClientPathCompositionFailure.ownershipUnattested }

    let repository = MacE2EEKeyStateRepository()
    let sender = try MacProtectedMediaSendService(
      keyState: repository,
      sealer: sealer,
      uploader: uploader,
      ciphertextRoot: ciphertextRoot,
      plaintextDraftRoot: plaintextDraftRoot)
    let playback = try MacProtectedMediaPlaybackService(
      keyState: repository,
      opener: opener,
      transport: playbackTransport,
      cacheRoot: playbackCacheRoot,
      cacheInstallationSecret: playbackCacheInstallationSecret)
    let live = try MacE2EELiveSessionFactory(
      keyState: repository,
      derivation: liveDerivation,
      authorization: liveAuthorization,
      crossProcessGenerationSerializationApproved: true)

    self.ownershipLease = ownershipLease
    keyState = repository
    protectedSend = sender
    protectedPlayback = playback
    protectedLive = live
  }

  @MainActor
  func makeProductionDarkModel() -> PulsarEncryptedMediaModel {
    PulsarEncryptedMediaModel(
      snapshot: .init(
        state: .loading,
        selectedPath: .plaintext,
        capabilityAdvertised: false,
        reviewedSuiteSelected: false,
        runtimeWiringApproved: false,
        ownership: .crossProcessSerialized,
        components: .init(
          keyStateReady: true,
          protectedSendReady: true,
          protectedPlaybackReady: true,
          protectedLiveReady: true,
          sameRepositoryWitness: true)))
  }
}
