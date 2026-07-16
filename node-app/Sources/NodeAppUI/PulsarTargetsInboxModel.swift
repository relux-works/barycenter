import Foundation
import Observation

public enum PulsarTargetsInboxSurfaceState: String, CaseIterable, Sendable {
    case loading
    case ready
    case stale
    case offline
    case coordinatorError = "coordinator_error"
}

public struct PulsarLocalizedLabel: Equatable, Sendable {
    public let key: String
    public let en: String
    public let ru: String

    public init(key: String, en: String, ru: String) {
        self.key = key
        self.en = en
        self.ru = ru
    }

    public func text(locale: PulsarShellLocale) -> String {
        locale == .ru ? ru : en
    }
}

public struct PulsarActionCapability: Equatable, Sendable {
    public let action: String
    public let label: PulsarLocalizedLabel

    public init(action: String, label: PulsarLocalizedLabel) {
        self.action = action
        self.label = label
    }
}

public enum PulsarTargetsInboxAudienceKind: String, CaseIterable, Sendable {
    case thisPulsar = "this_pulsar"
    case ownBarycenter = "own_barycenter"
    case currentAir = "current_air"
    case explicit
}

public struct PulsarTargetsInboxAudienceChoice: Equatable, Sendable {
    public let kind: PulsarTargetsInboxAudienceKind
    public let label: PulsarLocalizedLabel

    public init(kind: PulsarTargetsInboxAudienceKind, label: PulsarLocalizedLabel) {
        self.kind = kind
        self.label = label
    }
}

public struct PulsarTargetChoice: Equatable, Sendable, CustomStringConvertible,
    CustomDebugStringConvertible
{
    public let reference: String
    public let kind: String
    public let expiresAt: Date
    public let capabilityState: String
    public let capabilities: [String]
    public let label: PulsarLocalizedLabel

    public init(
        reference: String,
        kind: String,
        expiresAt: Date,
        capabilityState: String,
        capabilities: [String],
        label: PulsarLocalizedLabel
    ) {
        self.reference = reference
        self.kind = kind
        self.expiresAt = expiresAt
        self.capabilityState = capabilityState
        self.capabilities = capabilities
        self.label = label
    }

    public var description: String { "PulsarTargetChoice(<opaque>)" }
    public var debugDescription: String { description }
}

public struct PulsarInboxCapabilityItem: Equatable, Identifiable, Sendable {
    public let id: String
    public let historyItemID: String
    public let title: String
    public let expiresAt: Date
    public let availability: String
    public let sender: PulsarLocalizedLabel
    public let source: PulsarLocalizedLabel
    public let requestedDelivery: PulsarLocalizedLabel
    public let effectiveDelivery: PulsarLocalizedLabel
    public let receipt: PulsarLocalizedLabel
    public let actions: [PulsarActionCapability]

    public init(
        id: String,
        historyItemID: String,
        title: String,
        expiresAt: Date,
        availability: String,
        sender: PulsarLocalizedLabel,
        source: PulsarLocalizedLabel,
        requestedDelivery: PulsarLocalizedLabel,
        effectiveDelivery: PulsarLocalizedLabel,
        receipt: PulsarLocalizedLabel,
        actions: [PulsarActionCapability]
    ) {
        self.id = id
        self.historyItemID = historyItemID
        self.title = title
        self.expiresAt = expiresAt
        self.availability = availability
        self.sender = sender
        self.source = source
        self.requestedDelivery = requestedDelivery
        self.effectiveDelivery = effectiveDelivery
        self.receipt = receipt
        self.actions = actions
    }
}

public struct PulsarHistoryReceiptCapability: Equatable, Sendable {
    public let targetLabel: String
    public let status: PulsarLocalizedLabel

    public init(targetLabel: String, status: PulsarLocalizedLabel) {
        self.targetLabel = targetLabel
        self.status = status
    }
}

public struct PulsarHistoryReceiptCapabilityPage: Equatable, Sendable {
    public let items: [PulsarHistoryReceiptCapability]
    public let nextCursor: String?

    public init(items: [PulsarHistoryReceiptCapability] = [], nextCursor: String? = nil) {
        self.items = items
        self.nextCursor = nextCursor
    }
}

public struct PulsarHistoryCapabilityItem: Equatable, Identifiable, Sendable {
    public let id: String
    public let title: String
    public let status: PulsarLocalizedLabel
    public let actions: [PulsarActionCapability]
    public let playedCount: Int
    public let otherCount: Int
    public let receipts: PulsarHistoryReceiptCapabilityPage

    public init(
        id: String,
        title: String,
        status: PulsarLocalizedLabel,
        actions: [PulsarActionCapability],
        playedCount: Int,
        otherCount: Int,
        receipts: PulsarHistoryReceiptCapabilityPage = .init()
    ) {
        self.id = id
        self.title = title
        self.status = status
        self.actions = actions
        self.playedCount = max(0, playedCount)
        self.otherCount = max(0, otherCount)
        self.receipts = receipts
    }
}

public struct PulsarTargetsInboxSnapshot: Equatable, Sendable {
    public var state: PulsarTargetsInboxSurfaceState
    public var stateLabel: PulsarLocalizedLabel?
    public var activeAirTitle: String?
    public var availableAudiences: [PulsarTargetsInboxAudienceChoice]
    public var selectedAudience: PulsarTargetsInboxAudienceKind?
    public var targets: [PulsarTargetChoice]
    public var selectedReferences: [String]
    public var includeOrigin: Bool
    public var targetedTrackPolicy: String
    public var contentPolicyState: String
    public var inbox: [PulsarInboxCapabilityItem]
    public var inboxNextCursor: String?
    public var history: [PulsarHistoryCapabilityItem]
    public var historyNextCursor: String?

    public init(
        state: PulsarTargetsInboxSurfaceState = .loading,
        stateLabel: PulsarLocalizedLabel? = nil,
        activeAirTitle: String? = nil,
        availableAudiences: [PulsarTargetsInboxAudienceChoice] = [],
        selectedAudience: PulsarTargetsInboxAudienceKind? = nil,
        targets: [PulsarTargetChoice] = [],
        selectedReferences: [String] = [],
        includeOrigin: Bool = true,
        targetedTrackPolicy: String = "unsupported",
        contentPolicyState: String = "required",
        inbox: [PulsarInboxCapabilityItem] = [],
        inboxNextCursor: String? = nil,
        history: [PulsarHistoryCapabilityItem] = [],
        historyNextCursor: String? = nil
    ) {
        self.state = state
        self.stateLabel = stateLabel
        self.activeAirTitle = activeAirTitle
        self.availableAudiences = availableAudiences
        self.selectedAudience = selectedAudience
        self.targets = targets
        self.selectedReferences = selectedReferences
        self.includeOrigin = includeOrigin
        self.targetedTrackPolicy = targetedTrackPolicy
        self.contentPolicyState = contentPolicyState
        self.inbox = inbox
        self.inboxNextCursor = inboxNextCursor
        self.history = history
        self.historyNextCursor = historyNextCursor
    }
}

public enum PulsarTargetsInboxCommand: Equatable, Sendable, CustomStringConvertible,
    CustomDebugStringConvertible
{
    case refresh
    case setAudience(PulsarTargetsInboxAudienceKind)
    case selectTargets([String])
    case setIncludeOrigin(Bool)
    case loadMoreInbox(String)
    case loadMoreHistory(String)
    case loadMoreReceipts(historyItemID: String, cursor: String)
    case replayInbox(id: String, delivery: PulsarDeliveryMode)
    case dismissInbox(id: String)
    case deleteHistory(id: String)
    case reportInbox(id: String, reason: PulsarModerationReason, details: String)
    case reportHistory(id: String, reason: PulsarModerationReason, details: String)
    case muteSender(id: String)

    public var description: String {
        switch self {
        case .refresh: "PulsarTargetsInboxCommand(refresh)"
        case .setAudience: "PulsarTargetsInboxCommand(set_audience)"
        case .selectTargets: "PulsarTargetsInboxCommand(select_targets,<opaque>)"
        case .setIncludeOrigin: "PulsarTargetsInboxCommand(set_include_origin)"
        case .loadMoreInbox: "PulsarTargetsInboxCommand(load_more_inbox,<opaque>)"
        case .loadMoreHistory: "PulsarTargetsInboxCommand(load_more_history,<opaque>)"
        case .loadMoreReceipts: "PulsarTargetsInboxCommand(load_more_receipts,<opaque>)"
        case .replayInbox: "PulsarTargetsInboxCommand(replay_inbox,<opaque>)"
        case .dismissInbox: "PulsarTargetsInboxCommand(dismiss_inbox,<opaque>)"
        case .deleteHistory: "PulsarTargetsInboxCommand(delete_history,<opaque>)"
        case .reportInbox: "PulsarTargetsInboxCommand(report_inbox,<opaque>)"
        case .reportHistory: "PulsarTargetsInboxCommand(report_history,<opaque>)"
        case .muteSender: "PulsarTargetsInboxCommand(mute_sender,<opaque>)"
        }
    }

    public var debugDescription: String { description }
}

@MainActor
@Observable
public final class PulsarTargetsInboxModel {
    public private(set) var snapshot: PulsarTargetsInboxSnapshot

    public init(snapshot: PulsarTargetsInboxSnapshot = .init()) {
        self.snapshot = snapshot
    }

    public func replace(_ replacement: PulsarTargetsInboxSnapshot, now: Date = .now) {
        var normalized = replacement
        var known = Set<String>()
        normalized.targets = replacement.targets.compactMap { target in
            guard Self.validTargetReference(target.reference), target.expiresAt > now,
                known.insert(target.reference).inserted
            else { return nil }
            return PulsarTargetChoice(
                reference: target.reference, kind: target.kind, expiresAt: target.expiresAt,
                capabilityState: target.capabilityState,
                capabilities: Array(Set(target.capabilities.filter(Self.validEnum))).sorted(),
                label: target.label)
        }
        var audiences = Set<PulsarTargetsInboxAudienceKind>()
        normalized.availableAudiences = replacement.availableAudiences.filter { audience in
            guard audiences.insert(audience.kind).inserted else { return false }
            if audience.kind == .currentAir {
                return !(replacement.activeAirTitle?.trimmingCharacters(in: .whitespacesAndNewlines).isEmpty ?? true)
            }
            if audience.kind == .explicit { return !normalized.targets.isEmpty }
            return true
        }
        if let selected = replacement.selectedAudience,
            !audiences.contains(selected) || !normalized.availableAudiences.contains(where: { $0.kind == selected })
        {
            normalized.selectedAudience = nil
        }
        var selected = Set<String>()
        normalized.selectedReferences = replacement.selectedReferences.filter {
            known.contains($0) && selected.insert($0).inserted
        }.prefix(64).map { $0 }
        if normalized.state != .ready {
            normalized.selectedReferences = []
        }
        normalized.inbox = replacement.inbox.map { item in
            let structurallyValid = Self.validPublicID(item.id, prefix: "ib_")
                && Self.validPublicID(item.historyItemID, prefix: "hi_")
            return PulsarInboxCapabilityItem(
                id: item.id, historyItemID: item.historyItemID, title: item.title,
                expiresAt: item.expiresAt,
                availability: item.expiresAt > now ? item.availability : "expired",
                sender: item.sender, source: item.source,
                requestedDelivery: item.requestedDelivery,
                effectiveDelivery: item.effectiveDelivery, receipt: item.receipt,
                actions: structurallyValid && item.expiresAt > now
                    ? Self.canonicalActions(item.actions) : [])
        }
        normalized.history = replacement.history.map { item in
            PulsarHistoryCapabilityItem(
                id: item.id, title: item.title, status: item.status,
                actions: Self.validPublicID(item.id, prefix: "hi_")
                    ? Self.canonicalActions(item.actions) : [],
                playedCount: item.playedCount, otherCount: item.otherCount,
                receipts: item.receipts)
        }
        snapshot = normalized
    }

    public func refreshCommand() -> PulsarTargetsInboxCommand { .refresh }

    public func setAudienceCommand(
        _ audience: PulsarTargetsInboxAudienceKind
    ) -> PulsarTargetsInboxCommand? {
        guard isReady, snapshot.availableAudiences.contains(where: { $0.kind == audience }),
            audience != .explicit || !snapshot.selectedReferences.isEmpty
        else { return nil }
        return .setAudience(audience)
    }

    public func selectTargetsCommand(_ references: [String]) -> PulsarTargetsInboxCommand? {
        guard isReady, !references.isEmpty, references.count <= 64,
            Set(references).count == references.count
        else { return nil }
        let available = Set(snapshot.targets.map(\.reference))
        guard references.allSatisfy(available.contains) else { return nil }
        return .selectTargets(references)
    }

    public func setIncludeOriginCommand(_ include: Bool) -> PulsarTargetsInboxCommand? {
        isReady ? .setIncludeOrigin(include) : nil
    }

    public func loadMoreInboxCommand() -> PulsarTargetsInboxCommand? {
        guard isReady, let cursor = snapshot.inboxNextCursor, Self.validCursor(cursor, prefix: "ic_")
        else { return nil }
        return .loadMoreInbox(cursor)
    }

    public func loadMoreHistoryCommand() -> PulsarTargetsInboxCommand? {
        guard isReady, let cursor = snapshot.historyNextCursor, Self.validCursor(cursor, prefix: "hc_")
        else { return nil }
        return .loadMoreHistory(cursor)
    }

    public func loadMoreReceiptsCommand(historyItemID: String) -> PulsarTargetsInboxCommand? {
        guard isReady, Self.validPublicID(historyItemID, prefix: "hi_"),
            let item = snapshot.history.first(where: { $0.id == historyItemID }),
            let cursor = item.receipts.nextCursor, Self.validCursor(cursor, prefix: "rc_")
        else { return nil }
        return .loadMoreReceipts(historyItemID: historyItemID, cursor: cursor)
    }

    public func replayInboxCommand(id: String, delivery: PulsarDeliveryMode) -> PulsarTargetsInboxCommand? {
        guard isReady, snapshot.contentPolicyState == "current",
            let item = snapshot.inbox.first(where: { $0.id == id }),
            Self.validPublicID(id, prefix: "ib_"), Self.hasAction(item.actions, "replay")
        else { return nil }
        return .replayInbox(id: id, delivery: delivery)
    }

    public func dismissInboxCommand(id: String) -> PulsarTargetsInboxCommand? {
        guard isReady, let item = snapshot.inbox.first(where: { $0.id == id }),
            Self.validPublicID(id, prefix: "ib_"), Self.hasAction(item.actions, "dismiss")
        else { return nil }
        return .dismissInbox(id: id)
    }

    public func deleteHistoryCommand(id: String) -> PulsarTargetsInboxCommand? {
        historyCommand(id: id, action: "delete").map { .deleteHistory(id: $0) }
    }

    public func reportHistoryCommand(
        id: String,
        reason: PulsarModerationReason,
        details: String
    ) -> PulsarTargetsInboxCommand? {
        guard Self.validDetails(details),
            let id = historyCommand(id: id, action: "report")
        else { return nil }
        return .reportHistory(id: id, reason: reason, details: details)
    }

    public func reportInboxCommand(
        id: String,
        reason: PulsarModerationReason,
        details: String
    ) -> PulsarTargetsInboxCommand? {
        guard Self.validDetails(details), isReady,
            let item = snapshot.inbox.first(where: { $0.id == id }),
            Self.validPublicID(id, prefix: "ib_"),
            Self.validPublicID(item.historyItemID, prefix: "hi_"),
            Self.hasAction(item.actions, "report")
        else { return nil }
        return .reportInbox(id: item.historyItemID, reason: reason, details: details)
    }

    public func muteSenderCommand(id: String) -> PulsarTargetsInboxCommand? {
        if let historyID = historyCommand(id: id, action: "block_actor") {
            return .muteSender(id: historyID)
        }
        guard isReady, Self.validPublicID(id, prefix: "ib_"),
            let item = snapshot.inbox.first(where: { $0.id == id }),
            Self.validPublicID(item.historyItemID, prefix: "hi_"),
            Self.hasAction(item.actions, "block_actor")
        else { return nil }
        return .muteSender(id: item.historyItemID)
    }

    private var isReady: Bool { snapshot.state == .ready }

    private func historyCommand(id: String, action: String) -> String? {
        guard isReady, Self.validPublicID(id, prefix: "hi_"),
            let item = snapshot.history.first(where: { $0.id == id }),
            Self.hasAction(item.actions, action)
        else { return nil }
        return id
    }

    private static func hasAction(_ actions: [PulsarActionCapability], _ action: String) -> Bool {
        actions.contains { $0.action == action }
    }

    private static func canonicalActions(_ actions: [PulsarActionCapability]) -> [PulsarActionCapability] {
        var seen = Set<String>()
        return actions.filter { validEnum($0.action) && seen.insert($0.action).inserted }
    }

    private static func validDetails(_ details: String) -> Bool {
        details == details.trimmingCharacters(in: .whitespacesAndNewlines)
            && details.utf8.count <= 2_000
    }

    private static func validTargetReference(_ value: String) -> Bool {
        guard value.hasPrefix("trf_"), value.utf8.count == 47 else { return false }
        return value.dropFirst(4).unicodeScalars.allSatisfy {
            CharacterSet(charactersIn: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-")
                .contains($0)
        }
    }

    private static func validPublicID(_ value: String, prefix: String) -> Bool {
        guard value.hasPrefix(prefix), value.utf8.count == prefix.utf8.count + 26 else { return false }
        return value.dropFirst(prefix.count).unicodeScalars.allSatisfy {
            CharacterSet(charactersIn: "0123456789ABCDEFGHJKMNPQRSTVWXYZ").contains($0)
        }
    }

    private static func validCursor(_ value: String, prefix: String) -> Bool {
        guard value.hasPrefix(prefix), value.utf8.count == prefix.utf8.count + 64 else { return false }
        return value.dropFirst(prefix.count).unicodeScalars.allSatisfy {
            CharacterSet(charactersIn: "0123456789abcdef").contains($0)
        }
    }

    private static func validEnum(_ value: String) -> Bool {
        guard (1...64).contains(value.utf8.count), value.first?.isLetter == true else { return false }
        return value.unicodeScalars.allSatisfy {
            CharacterSet(charactersIn: "abcdefghijklmnopqrstuvwxyz0123456789_").contains($0)
        }
    }
}
