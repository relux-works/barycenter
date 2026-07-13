import Foundation

public enum ProtectedPersistenceOutcome<Value: Sendable>: Sendable, CustomStringConvertible,
  CustomDebugStringConvertible
{
  case stored(Value)
  case storageFailed(Value)

  public var value: Value {
    switch self {
    case .stored(let value), .storageFailed(let value): return value
    }
  }

  public var wasStored: Bool {
    if case .stored = self { return true }
    return false
  }

  public var description: String {
    wasStored
      ? "ProtectedPersistenceOutcome(stored: <redacted>)"
      : "ProtectedPersistenceOutcome(storageFailed: <redacted>)"
  }
  public var debugDescription: String { description }
}

/// UI-independent hooks for the future onboarding window.
public final class OnboardingService: @unchecked Sendable {
  public let client: OnboardingHTTPClient
  private let credentials: CredentialRepository

  public init(
    client: OnboardingHTTPClient, credentials: CredentialRepository = CredentialRepository()
  ) {
    self.client = client
    self.credentials = credentials
  }

  public func createOrbit(
    title: String,
    installationAttemptID: String
  ) async throws -> ProtectedPersistenceOutcome<CreatedOrbit> {
    let result = try await client.createOrbit(
      title: title, installationAttemptID: installationAttemptID)
    do {
      try credentials.saveBundle(result.bundle)
      return .stored(result)
    } catch {
      // Keep the one-time material in the typed outcome so the UI can
      // still offer explicit export and explain that protected storage
      // failed. It is not retained in an Error or persisted silently.
      return .storageFailed(result)
    }
  }

  public func joinOrbit(inviteCode: String) async throws -> ProtectedPersistenceOutcome<JoinedOrbit>
  {
    let result = try await client.consumeDeviceInvite(inviteCode)
    do {
      try credentials.saveBundle(result.bundle)
      return .stored(result)
    } catch {
      return .storageFailed(result)
    }
  }

  public func issueDeviceInvite(
    intendedRole: CredentialRole = .companion
  ) async throws -> DeviceInvite {
    let control = try boundControl()
    return try await client.issueDeviceInvite(control: control, intendedRole: intendedRole)
  }

  public func rotateRecovery() async throws -> ProtectedPersistenceOutcome<OneTimeRecoveryMaterial>
  {
    let control = try boundControl()
    let material = try await client.rotateRecovery(control: control)
    do {
      try credentials.mutateBundle { bundle in
        guard bundle.coordinatorOrigin == client.origin, bundle.control == control else {
          throw CredentialStorageError.conflict
        }
        bundle.recovery = RecoveryMetadata(
          actorId: material.actorId, recoveryId: material.recoveryId)
      }
      return .stored(material)
    } catch {
      return .storageFailed(material)
    }
  }

  public func issueTelegramLink(
    desiredRole: CredentialRole = .companion
  ) async throws -> TelegramLinkCode {
    let control = try boundControl()
    return try await client.issueTelegramLink(control: control, desiredRole: desiredRole)
  }

  public func probeControlContext() async throws -> ActorContextProbe {
    try await client.probe(token: boundControl().controlToken)
  }

  public func probeNodeContext() async throws -> ActorContextProbe {
    guard let bundle = try credentials.loadBundleWithoutMigration(),
      let node = bundle.node,
      CredentialSyntax.canonicalWebSocketURL(node.wsUrl, origin: client.origin)
    else { throw OnboardingClientError.storage }
    return try await client.probe(token: node.nodeToken)
  }

  public func acknowledgeRecoveryBackup(actorID: Int64, recoveryID: String) throws {
    try credentials.mutateBundle { bundle in
      guard bundle.coordinatorOrigin == client.origin,
        bundle.recovery?.actorId == actorID,
        bundle.recovery?.recoveryId == recoveryID
      else {
        throw CredentialStorageError.conflict
      }
      bundle.recovery?.explicitBackupAcknowledged = true
    }
  }

  private func boundControl() throws -> ControlCapability {
    guard let bundle = try credentials.loadBundleWithoutMigration(),
      bundle.coordinatorOrigin == client.origin,
      let control = bundle.control
    else { throw OnboardingClientError.storage }
    return control
  }
}
