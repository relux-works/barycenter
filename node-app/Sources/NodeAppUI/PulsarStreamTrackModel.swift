import Foundation
import Observation

public enum PulsarStreamTrackDraftPhase: String, CaseIterable, Equatable, Sendable {
    case retained, uploading, uploaded, processing, ready, failed
}

public enum PulsarStreamTrackPlaybackPhase: String, CaseIterable, Equatable, Sendable {
    case idle, queued, loading, ready, playing, paused, seeking, rebuffering, ended, failed
}

public enum PulsarStreamTrackAudience: String, CaseIterable, Equatable, Sendable {
    case currentAir = "current_air"
    case explicit
}

public enum PulsarStreamTrackInsertion: String, CaseIterable, Equatable, Sendable {
    case queue, replace
}

public enum PulsarStreamTrackFailure: String, CaseIterable, Equatable, Sendable {
    case offline
    case quotaExceeded = "quota_exceeded"
    case unsupportedTargets = "unsupported_targets"
    case policyRequired = "policy_required"
    case processingFailed = "processing_failed"
    case variantUnavailable = "variant_unavailable"
    case staleGeneration = "stale_generation"
    case serviceUnavailable = "service_unavailable"
}

public struct PulsarStreamTrackDraft: Equatable, Sendable, CustomStringConvertible,
    CustomDebugStringConvertible
{
    public var localID: String
    public var localByteCount: Int64
    public var retainedLocalBytes: Bool
    public var title: String
    public var clientMIME: String?
    public var durationMS: Int64?
    public var phase: PulsarStreamTrackDraftPhase
    public var phaseLabel: PulsarLocalizedLabel?
    public var mediaID: String?
    public var variantManifest: String?
    public var serverMetadataConfirmed: Bool
    public var uploadOffset: Int64
    public var processingPercent: Int
    public var failure: PulsarStreamTrackFailure?
    public var failureLabel: PulsarLocalizedLabel?

    public init(
        localID: String,
        localByteCount: Int64,
        retainedLocalBytes: Bool = true,
        title: String,
        clientMIME: String? = nil,
        durationMS: Int64? = nil,
        phase: PulsarStreamTrackDraftPhase = .retained,
        phaseLabel: PulsarLocalizedLabel? = nil,
        mediaID: String? = nil,
        variantManifest: String? = nil,
        serverMetadataConfirmed: Bool = false,
        uploadOffset: Int64 = 0,
        processingPercent: Int = 0,
        failure: PulsarStreamTrackFailure? = nil,
        failureLabel: PulsarLocalizedLabel? = nil
    ) {
        self.localID = localID
        self.localByteCount = localByteCount
        self.retainedLocalBytes = retainedLocalBytes
        self.title = title
        self.clientMIME = clientMIME
        self.durationMS = durationMS
        self.phase = phase
        self.phaseLabel = phaseLabel
        self.mediaID = mediaID
        self.variantManifest = variantManifest
        self.serverMetadataConfirmed = serverMetadataConfirmed
        self.uploadOffset = uploadOffset
        self.processingPercent = processingPercent
        self.failure = failure
        self.failureLabel = failureLabel
    }

    public var description: String { "PulsarStreamTrackDraft(<opaque>)" }
    public var debugDescription: String { description }
}

public struct PulsarStreamTrackPlayback: Equatable, Sendable, CustomStringConvertible,
    CustomDebugStringConvertible
{
    public var phase: PulsarStreamTrackPlaybackPhase
    public var phaseLabel: PulsarLocalizedLabel?
    public var streamID: String?
    public var durationMS: Int64
    public var audiblePositionMS: Int64
    public var playbackGeneration: UInt64
    public var seekGeneration: UInt64
    public var failure: PulsarStreamTrackFailure?
    public var failureLabel: PulsarLocalizedLabel?

    public init(
        phase: PulsarStreamTrackPlaybackPhase = .idle,
        phaseLabel: PulsarLocalizedLabel? = nil,
        streamID: String? = nil,
        durationMS: Int64 = 0,
        audiblePositionMS: Int64 = 0,
        playbackGeneration: UInt64 = 0,
        seekGeneration: UInt64 = 0,
        failure: PulsarStreamTrackFailure? = nil,
        failureLabel: PulsarLocalizedLabel? = nil
    ) {
        self.phase = phase
        self.phaseLabel = phaseLabel
        self.streamID = streamID
        self.durationMS = durationMS
        self.audiblePositionMS = audiblePositionMS
        self.playbackGeneration = playbackGeneration
        self.seekGeneration = seekGeneration
        self.failure = failure
        self.failureLabel = failureLabel
    }

    public var description: String { "PulsarStreamTrackPlayback(<opaque>)" }
    public var debugDescription: String { description }
}

public struct PulsarStreamTrackSnapshot: Equatable, Sendable, CustomStringConvertible,
    CustomDebugStringConvertible
{
    public var state: PulsarTargetsInboxSurfaceState
    public var stateLabel: PulsarLocalizedLabel?
    public var draft: PulsarStreamTrackDraft?
    public var playback: PulsarStreamTrackPlayback
    public var targets: [PulsarTargetChoice]
    public var selectedReferences: [String]
    public var selectedAudience: PulsarStreamTrackAudience?
    public var selectedInsertion: PulsarStreamTrackInsertion?
    public var activeAirAvailable: Bool
    public var contentPolicyState: String
    public var actions: [PulsarActionCapability]
    public var failure: PulsarStreamTrackFailure?
    public var failureLabel: PulsarLocalizedLabel?
    public var confirmedDeletedLocalID: String?

    public init(
        state: PulsarTargetsInboxSurfaceState = .loading,
        stateLabel: PulsarLocalizedLabel? = nil,
        draft: PulsarStreamTrackDraft? = nil,
        playback: PulsarStreamTrackPlayback = .init(),
        targets: [PulsarTargetChoice] = [],
        selectedReferences: [String] = [],
        selectedAudience: PulsarStreamTrackAudience? = nil,
        selectedInsertion: PulsarStreamTrackInsertion? = nil,
        activeAirAvailable: Bool = false,
        contentPolicyState: String = "required",
        actions: [PulsarActionCapability] = [],
        failure: PulsarStreamTrackFailure? = nil,
        failureLabel: PulsarLocalizedLabel? = nil,
        confirmedDeletedLocalID: String? = nil
    ) {
        self.state = state
        self.stateLabel = stateLabel
        self.draft = draft
        self.playback = playback
        self.targets = targets
        self.selectedReferences = selectedReferences
        self.selectedAudience = selectedAudience
        self.selectedInsertion = selectedInsertion
        self.activeAirAvailable = activeAirAvailable
        self.contentPolicyState = contentPolicyState
        self.actions = actions
        self.failure = failure
        self.failureLabel = failureLabel
        self.confirmedDeletedLocalID = confirmedDeletedLocalID
    }

    public var description: String { "PulsarStreamTrackSnapshot(<opaque>)" }
    public var debugDescription: String { description }
}

public enum PulsarStreamTrackCommand: Equatable, Sendable, CustomStringConvertible,
    CustomDebugStringConvertible
{
    case acceptPolicy
    case upload(localID: String)
    case retry(localID: String)
    case delete(localID: String, confirmed: Bool)
    case queue(mediaID: String, audience: PulsarStreamTrackAudience, targets: [String])
    case replace(mediaID: String, audience: PulsarStreamTrackAudience, targets: [String])
    case pause(streamID: String, playbackGeneration: UInt64)
    case seek(streamID: String, positionMS: Int64, playbackGeneration: UInt64, seekGeneration: UInt64)
    case resume(streamID: String, playbackGeneration: UInt64)
    case report(mediaID: String, details: String)

    public var description: String {
        switch self {
        case .acceptPolicy: "PulsarStreamTrackCommand(accept_policy)"
        case .upload: "PulsarStreamTrackCommand(upload,<opaque>)"
        case .retry: "PulsarStreamTrackCommand(retry,<opaque>)"
        case .delete: "PulsarStreamTrackCommand(delete,<opaque>)"
        case .queue: "PulsarStreamTrackCommand(queue,<opaque>)"
        case .replace: "PulsarStreamTrackCommand(replace,<opaque>)"
        case .pause: "PulsarStreamTrackCommand(pause,<opaque>)"
        case .seek: "PulsarStreamTrackCommand(seek,<opaque>)"
        case .resume: "PulsarStreamTrackCommand(resume,<opaque>)"
        case .report: "PulsarStreamTrackCommand(report,<opaque>)"
        }
    }

    public var debugDescription: String { description }
}

@MainActor
@Observable
public final class PulsarStreamTrackModel {
    public static let maximumFileBytes: Int64 = 524_288_000
    public static let maximumDurationMS: Int64 = 7_200_000
    public static let maximumTargets = 64

    public private(set) var snapshot: PulsarStreamTrackSnapshot

    public init(snapshot: PulsarStreamTrackSnapshot = .init()) {
        self.snapshot = .init()
        replace(snapshot)
    }

    public func replace(_ replacement: PulsarStreamTrackSnapshot, now: Date = .now) {
        var normalized = replacement
        if let deleted = normalized.confirmedDeletedLocalID, !Self.validLocalID(deleted) {
            normalized.confirmedDeletedLocalID = nil
        }
        if !["current", "required", "stale"].contains(normalized.contentPolicyState) {
            normalized.contentPolicyState = "required"
        }
        if !Self.validLabel(normalized.stateLabel, key: "surface.\(normalized.state.rawValue)") {
            normalized.stateLabel = nil
        }
        if let failure = normalized.failure,
            !Self.validLabel(normalized.failureLabel, key: "stream_track.failure.\(failure.rawValue)")
        {
            normalized.failureLabel = nil
        } else if normalized.failure == nil {
            normalized.failureLabel = nil
        }
        normalized.actions = Self.canonicalActions(replacement.actions)
        normalized.targets = Self.canonicalTargets(replacement.targets, now: now)
        let currentReferences = Set(normalized.targets.map(\.reference))
        var seen = Set<String>()
        normalized.selectedReferences = replacement.selectedReferences.filter {
            currentReferences.contains($0) && seen.insert($0).inserted
        }.prefix(Self.maximumTargets).map { $0 }
        if normalized.state != .ready {
            normalized.actions = []
        }
        if normalized.selectedAudience == .currentAir && !normalized.activeAirAvailable {
            normalized.selectedAudience = nil
        }
        if normalized.selectedAudience == .explicit && normalized.selectedReferences.isEmpty {
            normalized.selectedAudience = nil
        }
        normalized.draft = normalized.draft.flatMap(Self.normalizeDraft)
        if normalized.draft == nil, let retained = snapshot.draft,
            retained.retainedLocalBytes,
            normalized.confirmedDeletedLocalID != retained.localID
        {
            // A missing/stale server projection never destroys an unsent local file.
            normalized.draft = retained
        }
        normalized.playback = Self.normalizePlayback(
            replacement.playback, current: snapshot.playback)
        snapshot = normalized
    }

    @discardableResult
    public func selectAudience(_ audience: PulsarStreamTrackAudience) -> Bool {
        guard snapshot.state == .ready else { return false }
        if audience == .currentAir {
            guard snapshot.activeAirAvailable else { return false }
            snapshot.selectedReferences = []
        } else if snapshot.selectedReferences.isEmpty {
            return false
        }
        snapshot.selectedAudience = audience
        return true
    }

    @discardableResult
    public func selectTargets(_ references: [String]) -> Bool {
        guard snapshot.state == .ready, references.count <= Self.maximumTargets,
            Set(references).count == references.count
        else { return false }
        let available = Set(snapshot.targets.map(\.reference))
        guard references.allSatisfy(available.contains) else { return false }
        snapshot.selectedReferences = references
        if references.isEmpty && snapshot.selectedAudience == .explicit {
            snapshot.selectedAudience = nil
        }
        return true
    }

    @discardableResult
    public func selectInsertion(_ insertion: PulsarStreamTrackInsertion) -> Bool {
        guard snapshot.state == .ready else { return false }
        snapshot.selectedInsertion = insertion
        return true
    }

    public func buildCommand(_ request: PulsarStreamTrackCommand) -> PulsarStreamTrackCommand? {
        let ready = snapshot.state == .ready
        switch request {
        case .acceptPolicy:
            return ready && hasAction("accept_policy")
                && ["required", "stale"].contains(snapshot.contentPolicyState) ? request : nil
        case let .upload(localID):
            guard ready, snapshot.contentPolicyState == "current", hasAction("upload"),
                let draft = snapshot.draft, draft.phase == .retained,
                draft.retainedLocalBytes, draft.localID == localID
            else { return nil }
            return request
        case let .retry(localID):
            guard ready, hasAction("retry"), let draft = snapshot.draft,
                draft.localID == localID,
                draft.phase == .failed || snapshot.playback.phase == .failed
            else { return nil }
            return request
        case let .delete(localID, confirmed):
            guard ready, confirmed, hasAction("delete"), snapshot.draft?.localID == localID else { return nil }
            return request
        case let .queue(mediaID, audience, targets):
            return deliveryCommand(request, action: "queue", mediaID: mediaID, audience: audience, targets: targets)
        case let .replace(mediaID, audience, targets):
            guard snapshot.playback.playbackGeneration < UInt64.max else { return nil }
            return deliveryCommand(request, action: "replace", mediaID: mediaID, audience: audience, targets: targets)
        case let .pause(streamID, generation):
            guard hasAction("pause"), snapshot.playback.phase == .playing,
                exactPlayback(streamID, generation: generation)
            else { return nil }
            return request
        case let .seek(streamID, position, generation, seekGeneration):
            guard hasAction("seek"), [.ready, .playing, .paused, .rebuffering].contains(snapshot.playback.phase),
                exactPlayback(streamID, generation: generation),
                seekGeneration == snapshot.playback.seekGeneration,
                seekGeneration < UInt64.max,
                (0...snapshot.playback.durationMS).contains(position)
            else { return nil }
            return request
        case let .resume(streamID, generation):
            guard hasAction("resume"), [.ready, .paused, .rebuffering].contains(snapshot.playback.phase),
                exactPlayback(streamID, generation: generation)
            else { return nil }
            return request
        case let .report(mediaID, details):
            guard ready, hasAction("report"), snapshot.draft?.mediaID == mediaID,
                details == details.trimmingCharacters(in: .whitespacesAndNewlines),
                details.utf8.count <= 2_000
            else { return nil }
            return request
        }
    }

    public func applyOptimistic(_ request: PulsarStreamTrackCommand) -> Bool {
        guard let command = buildCommand(request) else { return false }
        switch command {
        case .acceptPolicy:
            break // The coordinator remains the consent authority.
        case .upload:
            snapshot.draft?.phase = .uploading
            snapshot.draft?.phaseLabel = nil
        case .retry:
            if snapshot.draft?.phase == .failed {
                snapshot.draft?.phase = .retained
                snapshot.draft?.phaseLabel = nil
            }
            if snapshot.playback.phase == .failed {
                snapshot.playback.phase = .loading
                snapshot.playback.phaseLabel = nil
            }
        case .delete:
            break // Local bytes survive until confirmed deletion arrives.
        case .queue:
            snapshot.playback.phase = .queued
            snapshot.playback.phaseLabel = nil
        case .replace:
            snapshot.playback.phase = .loading
            snapshot.playback.phaseLabel = nil
            snapshot.playback.playbackGeneration &+= 1
            snapshot.playback.seekGeneration = 0
            snapshot.playback.audiblePositionMS = 0
        case .pause:
            snapshot.playback.phase = .paused
            snapshot.playback.phaseLabel = nil
        case let .seek(_, position, _, _):
            snapshot.playback.phase = .seeking
            snapshot.playback.phaseLabel = nil
            snapshot.playback.seekGeneration &+= 1
            snapshot.playback.audiblePositionMS = position
        case .resume:
            snapshot.playback.phase = .playing
            snapshot.playback.phaseLabel = nil
        case .report:
            break
        }
        return true
    }

    private func deliveryCommand(
        _ request: PulsarStreamTrackCommand,
        action: String,
        mediaID: String,
        audience: PulsarStreamTrackAudience,
        targets: [String]
    ) -> PulsarStreamTrackCommand? {
        guard snapshot.state == .ready, snapshot.contentPolicyState == "current", hasAction(action),
            let draft = snapshot.draft, draft.phase == .ready, draft.mediaID == mediaID,
            draft.serverMetadataConfirmed, !(draft.variantManifest?.isEmpty ?? true),
            snapshot.selectedAudience == audience,
            snapshot.selectedInsertion?.rawValue == action
        else { return nil }
        switch audience {
        case .currentAir:
            guard snapshot.activeAirAvailable, targets.isEmpty else { return nil }
        case .explicit:
            guard !targets.isEmpty, targets.count <= Self.maximumTargets,
                Set(targets).count == targets.count,
                targets == snapshot.selectedReferences
            else { return nil }
            let byReference = Dictionary(uniqueKeysWithValues: snapshot.targets.map { ($0.reference, $0) })
            guard targets.allSatisfy({ byReference[$0]?.capabilities.contains("stream_track") == true }) else { return nil }
        }
        return request
    }

    private func hasAction(_ action: String) -> Bool {
        snapshot.actions.contains { $0.action == action }
    }

    private func exactPlayback(_ streamID: String, generation: UInt64) -> Bool {
        snapshot.state == .ready && snapshot.playback.streamID == streamID
            && snapshot.playback.playbackGeneration == generation
    }

    private static func normalizeDraft(_ value: PulsarStreamTrackDraft) -> PulsarStreamTrackDraft? {
        guard validLocalID(value.localID), value.localByteCount > 0,
            value.localByteCount <= maximumFileBytes,
            validTitle(value.title)
        else { return nil }
        var result = value
        result.uploadOffset = min(max(0, value.uploadOffset), value.localByteCount)
        result.processingPercent = min(max(0, value.processingPercent), 100)
        if let duration = value.durationMS {
            result.durationMS = min(max(0, duration), maximumDurationMS)
        }
        if result.phase == .ready && (!result.serverMetadataConfirmed
            || result.mediaID?.isEmpty != false || result.variantManifest?.isEmpty != false)
        {
            result.phase = .processing
            result.failure = nil
        }
        if !validLabel(result.phaseLabel, key: "stream_track.draft.\(result.phase.rawValue)") {
            result.phaseLabel = nil
        }
        if let failure = result.failure,
            !validLabel(result.failureLabel, key: "stream_track.failure.\(failure.rawValue)")
        {
            result.failureLabel = nil
        } else if result.failure == nil {
            result.failureLabel = nil
        }
        return result
    }

    private static func normalizePlayback(
        _ value: PulsarStreamTrackPlayback,
        current: PulsarStreamTrackPlayback
    ) -> PulsarStreamTrackPlayback {
        if value.playbackGeneration < current.playbackGeneration { return current }
        if value.playbackGeneration == current.playbackGeneration,
            value.seekGeneration < current.seekGeneration { return current }
        var result = value
        result.durationMS = min(max(0, value.durationMS), maximumDurationMS)
        result.audiblePositionMS = min(max(0, value.audiblePositionMS), result.durationMS)
        if value.playbackGeneration == current.playbackGeneration,
            value.seekGeneration == current.seekGeneration,
            result.audiblePositionMS < current.audiblePositionMS
        {
            result.audiblePositionMS = current.audiblePositionMS
        }
        if result.phase != .idle && result.streamID?.isEmpty != false {
            result.phase = .failed
            result.failure = .variantUnavailable
        }
        if !validLabel(result.phaseLabel, key: "stream_track.playback.\(result.phase.rawValue)") {
            result.phaseLabel = nil
        }
        if let failure = result.failure,
            !validLabel(result.failureLabel, key: "stream_track.failure.\(failure.rawValue)")
        {
            result.failureLabel = nil
        } else if result.failure == nil {
            result.failureLabel = nil
        }
        return result
    }

    private static func canonicalTargets(_ values: [PulsarTargetChoice], now: Date) -> [PulsarTargetChoice] {
        var seen = Set<String>()
        return values.compactMap { target in
            guard validTargetReference(target.reference), target.expiresAt > now,
                seen.insert(target.reference).inserted
            else { return nil }
            let capabilities = Set(target.capabilities.filter(validEnum))
            return PulsarTargetChoice(
                reference: target.reference, kind: target.kind, expiresAt: target.expiresAt,
                capabilityState: target.capabilityState,
                capabilities: capabilities.sorted(), label: target.label)
        }
    }

    private static func canonicalActions(_ values: [PulsarActionCapability]) -> [PulsarActionCapability] {
        let allowed = Set(["accept_policy", "upload", "retry", "delete", "queue", "replace", "pause", "seek", "resume", "report"])
        var seen = Set<String>()
        return values.filter {
            allowed.contains($0.action)
                && $0.label.key == "stream_track.action.\($0.action)"
                && !$0.label.en.isEmpty && !$0.label.ru.isEmpty
                && seen.insert($0.action).inserted
        }
    }

    private static func validTitle(_ value: String) -> Bool {
        let trimmed = value.trimmingCharacters(in: .whitespacesAndNewlines)
        return value == trimmed && !value.isEmpty && value.utf8.count <= 512
    }

    private static func validLabel(_ value: PulsarLocalizedLabel?, key: String) -> Bool {
        value?.key == key && value?.en.isEmpty == false && value?.ru.isEmpty == false
    }

    private static func validLocalID(_ value: String) -> Bool {
        value.utf8.count == 32 && value.unicodeScalars.allSatisfy {
            CharacterSet(charactersIn: "0123456789abcdef").contains($0)
        }
    }

    private static func validTargetReference(_ value: String) -> Bool {
        guard value.hasPrefix("trf_"), value.utf8.count == 47 else { return false }
        return value.dropFirst(4).unicodeScalars.allSatisfy {
            CharacterSet(charactersIn: "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789_-")
                .contains($0)
        }
    }

    private static func validEnum(_ value: String) -> Bool {
        guard (1...64).contains(value.utf8.count), value.first?.isLetter == true else { return false }
        return value.unicodeScalars.allSatisfy {
            CharacterSet(charactersIn: "abcdefghijklmnopqrstuvwxyz0123456789_").contains($0)
        }
    }
}
