import Foundation

public enum PulsarAirRole: String, CaseIterable, Identifiable, Sendable {
    case owner, admin, member
    public var id: Self { self }
}

public enum PulsarAirMembershipStatus: String, Sendable {
    case pendingConfirmation = "pending_confirmation"
    case joined
}

public enum PulsarAirInvitePolicy: String, CaseIterable, Identifiable, Sendable {
    case ownerPrimary = "owner_primary"
    case airAdminPrimary = "air_admin_primary"
    case allMemberPrimaries = "all_member_primaries"
    public var id: Self { self }
}

public enum PulsarAirPlaybackPolicy: String, CaseIterable, Identifiable, Sendable {
    case ownerPrimary = "owner_primary"
    case airAdminPrimary = "air_admin_primary"
    case allMemberPrimaries = "all_member_primaries"
    case primaryCompanion = "primary_companion"
    case disabled
    public var id: Self { self }
}

public enum PulsarAirAvailability: Equatable, Sendable {
    case checking
    case enabled
    case disabled
}

public struct PulsarAirPolicy: Equatable, Sendable {
    public let revision: Int64
    public let invite: PulsarAirInvitePolicy
    public let overlay: PulsarAirPlaybackPolicy
    public let queue: PulsarAirPlaybackPolicy
    public let replace: PulsarAirPlaybackPolicy

    public init(
        revision: Int64,
        invite: PulsarAirInvitePolicy,
        overlay: PulsarAirPlaybackPolicy,
        queue: PulsarAirPlaybackPolicy,
        replace: PulsarAirPlaybackPolicy
    ) {
        self.revision = revision
        self.invite = invite
        self.overlay = overlay
        self.queue = queue
        self.replace = replace
    }
}

/// Opaque identifiers are retained only as action handles. Views render title,
/// role and aggregate membership state and never show `id` or `membershipID`.
public struct PulsarAirItem: Equatable, Identifiable, Sendable {
    public let id: String
    public let title: String
    public let status: String
    public let revision: Int64
    public let membershipID: String
    public let membershipStatus: PulsarAirMembershipStatus
    public let membershipRevision: Int64
    public let role: PulsarAirRole
    public let memberCount: Int
    public let activeMemberCount: Int
    public let onlinePulsarCount: Int
    public let barycenterCapacity: Int
    public let onlinePulsarCapacity: Int
    public let policy: PulsarAirPolicy
    public let isCurrent: Bool

    public init(
        id: String, title: String, status: String, revision: Int64,
        membershipID: String, membershipStatus: PulsarAirMembershipStatus,
        membershipRevision: Int64, role: PulsarAirRole, memberCount: Int,
        activeMemberCount: Int, onlinePulsarCount: Int, barycenterCapacity: Int,
        onlinePulsarCapacity: Int, policy: PulsarAirPolicy, isCurrent: Bool
    ) {
        self.id = id
        self.title = title
        self.status = status
        self.revision = revision
        self.membershipID = membershipID
        self.membershipStatus = membershipStatus
        self.membershipRevision = membershipRevision
        self.role = role
        self.memberCount = memberCount
        self.activeMemberCount = activeMemberCount
        self.onlinePulsarCount = onlinePulsarCount
        self.barycenterCapacity = barycenterCapacity
        self.onlinePulsarCapacity = onlinePulsarCapacity
        self.policy = policy
        self.isCurrent = isCurrent
    }
}

public struct PulsarPendingAirJoin: Equatable, Sendable {
    public let airID: String
    public let title: String
    public let ownerDisplayName: String?
    public let role: PulsarAirRole
    public let membershipRevision: Int64
    public let memberCount: Int
    public let barycenterCapacity: Int
    public let activationWouldSwitch: Bool

    public init(
        airID: String, title: String, ownerDisplayName: String?, role: PulsarAirRole,
        membershipRevision: Int64, memberCount: Int, barycenterCapacity: Int,
        activationWouldSwitch: Bool
    ) {
        self.airID = airID
        self.title = title
        self.ownerDisplayName = ownerDisplayName
        self.role = role
        self.membershipRevision = membershipRevision
        self.memberCount = memberCount
        self.barycenterCapacity = barycenterCapacity
        self.activationWouldSwitch = activationWouldSwitch
    }
}

public struct PulsarAirInviteSecret: Equatable, Sendable,
    CustomStringConvertible, CustomDebugStringConvertible {
    public let airID: String
    public let inviteID: String
    public let revision: Int64
    public let airTitle: String
    public let code: String
    public let expiresAt: Date

    public init(
        airID: String, inviteID: String, revision: Int64, airTitle: String,
        code: String, expiresAt: Date
    ) {
        self.airID = airID
        self.inviteID = inviteID
        self.revision = revision
        self.airTitle = airTitle
        self.code = code
        self.expiresAt = expiresAt
    }

    public var description: String {
        "PulsarAirInviteSecret(airTitle: \(airTitle), expiresAt: \(expiresAt), code: <redacted>)"
    }

    public var debugDescription: String { description }
}

public struct PulsarAirState: Equatable, Sendable {
    public var availability: PulsarAirAvailability
    public var saved: [PulsarAirItem]
    public var pendingJoin: PulsarPendingAirJoin?
    public var inviteSecret: PulsarAirInviteSecret?
    public var busy: Bool
    public var outcome: String?
    public var failure: String?

    public init(
        availability: PulsarAirAvailability = .checking,
        saved: [PulsarAirItem] = [], pendingJoin: PulsarPendingAirJoin? = nil,
        inviteSecret: PulsarAirInviteSecret? = nil, busy: Bool = false,
        outcome: String? = nil, failure: String? = nil
    ) {
        self.availability = availability
        self.saved = saved
        self.pendingJoin = pendingJoin
        self.inviteSecret = inviteSecret
        self.busy = busy
        self.outcome = outcome
        self.failure = failure
    }

    public var current: PulsarAirItem? { saved.first(where: \.isCurrent) }
}
