package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"relux.works/duet/coordinator/internal/ulid"
)

var (
	ErrTransmissionInvalid       = errors.New("transmission input is invalid")
	ErrTransmissionNotFound      = errors.New("transmission was not found")
	ErrTransmissionStateConflict = errors.New("transmission state changed")
	ErrTransmissionMediaInvalid  = errors.New("transmission media is not ready or owned by the source")
	ErrTransmissionTargetInvalid = errors.New("transmission target binding is invalid")
)

type TransmissionDelivery string

const (
	TransmissionDeliveryOverlay      TransmissionDelivery = "overlay"
	TransmissionDeliveryInterrupt    TransmissionDelivery = "interrupt"
	TransmissionDeliveryAfterCurrent TransmissionDelivery = "after_current"
)

type TransmissionAudienceKind string

const (
	TransmissionAudienceThisPulsar    TransmissionAudienceKind = "this_pulsar"
	TransmissionAudienceOwnBarycenter TransmissionAudienceKind = "own_barycenter"
	TransmissionAudienceCurrentAir    TransmissionAudienceKind = "current_air"
	TransmissionAudienceExplicit      TransmissionAudienceKind = "explicit"
)

type TransmissionOriginKind string

const (
	TransmissionOriginMicrophone TransmissionOriginKind = "microphone"
	TransmissionOriginFile       TransmissionOriginKind = "file"
	TransmissionOriginTelegram   TransmissionOriginKind = "telegram"
	TransmissionOriginBuiltin    TransmissionOriginKind = "builtin"
)

type PlaybackDomainKind string

const (
	PlaybackDomainOrbit    PlaybackDomainKind = "orbit"
	PlaybackDomainApproach PlaybackDomainKind = "approach"
)

type TransmissionStatus string

const (
	TransmissionStatusAccepted   TransmissionStatus = "accepted"
	TransmissionStatusPreparing  TransmissionStatus = "preparing"
	TransmissionStatusScheduled  TransmissionStatus = "scheduled"
	TransmissionStatusPlaying    TransmissionStatus = "playing"
	TransmissionStatusCancelling TransmissionStatus = "cancelling"
	TransmissionStatusPlayed     TransmissionStatus = "played"
	TransmissionStatusPartial    TransmissionStatus = "partial"
	TransmissionStatusFailed     TransmissionStatus = "failed"
	TransmissionStatusCancelled  TransmissionStatus = "cancelled"
	TransmissionStatusExpired    TransmissionStatus = "expired"
)

type TransmissionTargetStatus string

const (
	TransmissionTargetAccepted       TransmissionTargetStatus = "accepted"
	TransmissionTargetPreparing      TransmissionTargetStatus = "preparing"
	TransmissionTargetReady          TransmissionTargetStatus = "ready"
	TransmissionTargetScheduled      TransmissionTargetStatus = "scheduled"
	TransmissionTargetPlaying        TransmissionTargetStatus = "playing"
	TransmissionTargetCancelling     TransmissionTargetStatus = "cancelling"
	TransmissionTargetPlayed         TransmissionTargetStatus = "played"
	TransmissionTargetMissedOffline  TransmissionTargetStatus = "missed_offline"
	TransmissionTargetMissedDND      TransmissionTargetStatus = "missed_dnd"
	TransmissionTargetMissedNotReady TransmissionTargetStatus = "missed_not_ready"
	TransmissionTargetBlocked        TransmissionTargetStatus = "blocked"
	TransmissionTargetFailed         TransmissionTargetStatus = "failed"
	TransmissionTargetCancelled      TransmissionTargetStatus = "cancelled"
	TransmissionTargetExpired        TransmissionTargetStatus = "expired"
)

type TransmissionReason string

const (
	TransmissionReasonCompleted               TransmissionReason = "completed"
	TransmissionReasonPartialDelivery         TransmissionReason = "partial_delivery"
	TransmissionReasonNoEligibleTargets       TransmissionReason = "no_eligible_targets"
	TransmissionReasonNoReadyTargets          TransmissionReason = "no_ready_targets"
	TransmissionReasonAllTargetsFailed        TransmissionReason = "all_targets_failed"
	TransmissionReasonOfflineAtAcceptance     TransmissionReason = "offline_at_acceptance"
	TransmissionReasonOfflineBeforePrepare    TransmissionReason = "offline_before_prepare"
	TransmissionReasonOfflineBeforeStart      TransmissionReason = "offline_before_start"
	TransmissionReasonLocalDND                TransmissionReason = "local_dnd"
	TransmissionReasonOrbitDND                TransmissionReason = "orbit_dnd"
	TransmissionReasonPrepareDeadline         TransmissionReason = "prepare_deadline"
	TransmissionReasonActorBlocked            TransmissionReason = "actor_blocked"
	TransmissionReasonOrbitBlocked            TransmissionReason = "orbit_blocked"
	TransmissionReasonMediaDownloadFailed     TransmissionReason = "media_download_failed"
	TransmissionReasonMediaAuthFailed         TransmissionReason = "media_auth_failed"
	TransmissionReasonMediaExpired            TransmissionReason = "media_expired"
	TransmissionReasonHashMismatch            TransmissionReason = "hash_mismatch"
	TransmissionReasonDecodeFailed            TransmissionReason = "decode_failed"
	TransmissionReasonDurationMismatch        TransmissionReason = "duration_mismatch"
	TransmissionReasonClockUnsynchronized     TransmissionReason = "clock_unsynchronized"
	TransmissionReasonStalePlay               TransmissionReason = "stale_play"
	TransmissionReasonDeviceUnavailable       TransmissionReason = "device_unavailable"
	TransmissionReasonAudioGraphFailed        TransmissionReason = "audio_graph_failed"
	TransmissionReasonConnectionLost          TransmissionReason = "connection_lost"
	TransmissionReasonCapabilityLost          TransmissionReason = "capability_lost"
	TransmissionReasonInterruptCapabilityLost TransmissionReason = "interrupt_capability_lost"
	TransmissionReasonCancelUnacknowledged    TransmissionReason = "cancel_unacknowledged"
	TransmissionReasonInternalError           TransmissionReason = "internal_error"
	TransmissionReasonSenderCancelled         TransmissionReason = "sender_cancelled"
	TransmissionReasonMediaDeleted            TransmissionReason = "media_deleted"
	TransmissionReasonModerationDisabled      TransmissionReason = "moderation_disabled"
	TransmissionReasonApproachLeft            TransmissionReason = "approach_left"
	TransmissionReasonApproachApart           TransmissionReason = "approach_apart"
	TransmissionReasonTargetRevoked           TransmissionReason = "target_revoked"
	TransmissionReasonDNDEnabled              TransmissionReason = "dnd_enabled"
	TransmissionReasonSenderBlocked           TransmissionReason = "sender_blocked"
	TransmissionReasonReported                TransmissionReason = "reported"
	TransmissionReasonCoordinatorRestarted    TransmissionReason = "coordinator_restarted"
	TransmissionReasonDeliveryExpired         TransmissionReason = "delivery_expired"
)

type Transmission struct {
	ID                 string
	MediaID            string
	SourceOrbitID      int64
	SourceActorID      int64
	SourceSlot         string
	PlaybackDomainKind PlaybackDomainKind
	PlaybackDomainID   int64
	AirID              string
	AirPolicyRevision  int64
	AirPolicyOperation AirPolicyOperation
	AirPolicyResult    string
	AudienceKind       TransmissionAudienceKind
	OriginKind         TransmissionOriginKind
	IncludeOrigin      bool
	RequestedDelivery  TransmissionDelivery
	EffectiveDelivery  TransmissionDelivery
	DowngradeReason    string
	Status             TransmissionStatus
	ReasonCode         TransmissionReason
	CancellationCause  TransmissionReason
	AcceptedAt         int64
	ExpiresAt          int64
	Revision           int64
	UpdatedAt          int64
	CompletedAt        int64
}

type TransmissionTarget struct {
	TransmissionID       string
	OrbitID              int64
	ActorID              int64
	Slot                 string
	BindingPairedAt      int64
	CapabilitySetHash    string
	ResolvedAtMS         int64
	OnlineAtAcceptance   bool
	MediaClipCapable     bool
	OverlayCapable       bool
	InterruptCapable     bool
	InterruptResumeReady bool
	Status               TransmissionTargetStatus
	ReasonCode           TransmissionReason
	Generation           int64
	Revision             int64
	ReadyAt              int64
	ScheduledAt          int64
	StartedAt            int64
	EndedAt              int64
	LastReceiptAt        int64
	UpdatedAt            int64
}

type CreateTransmissionTarget struct {
	OrbitID              int64
	ActorID              int64
	Slot                 string
	CapabilitySetHash    string
	OnlineAtAcceptance   bool
	MediaClipCapable     bool
	OverlayCapable       bool
	InterruptCapable     bool
	InterruptResumeReady bool
	Status               TransmissionTargetStatus
	ReasonCode           TransmissionReason
}

type CreateTransmissionParams struct {
	MediaID            string
	SourceOrbitID      int64
	SourceActorID      int64
	SourceSlot         string
	PlaybackDomainKind PlaybackDomainKind
	PlaybackDomainID   int64
	AirID              string
	AirPolicyRevision  int64
	AirPolicyOperation AirPolicyOperation
	AirPolicyResult    string
	AudienceKind       TransmissionAudienceKind
	OriginKind         TransmissionOriginKind
	IncludeOrigin      bool
	RequestedDelivery  TransmissionDelivery
	EffectiveDelivery  TransmissionDelivery
	DowngradeReason    string
	AcceptedAt         int64
	Targets            []CreateTransmissionTarget
}

type TransmissionCreation struct {
	Transmission Transmission
	Targets      []TransmissionTarget
}

type TransitionTransmissionTargetParams struct {
	TransmissionID   string
	OrbitID          int64
	ActorID          int64
	Slot             string
	ExpectedRevision int64
	Generation       int64
	Status           TransmissionTargetStatus
	ReasonCode       TransmissionReason
	OccurredAt       int64
}

type TransmissionTargetTransition struct {
	Transmission Transmission
	Target       TransmissionTarget
	Changed      bool
}

type CommitTransmissionCauseParams struct {
	TransmissionID   string
	ExpectedRevision int64
	Cause            TransmissionReason
	OccurredAt       int64
}

var (
	transmissionIDPattern   = regexp.MustCompile(`^tr_[0-9A-HJKMNP-TV-Z]{26}$`)
	transmissionSlotPattern = regexp.MustCompile(`^[a-z]$`)
)

const transmissionColumns = `id, media_id, source_orbit_id, source_actor_id,
source_slot, playback_domain_kind, playback_domain_id, audience_kind,
origin_kind, include_origin, requested_delivery, effective_delivery,
downgrade_reason, status, reason_code, cancellation_cause, accepted_at,
expires_at, revision, updated_at, completed_at, air_id, air_policy_revision,
air_policy_operation, air_policy_result`

const transmissionTargetColumns = `transmission_id, orbit_id, actor_id, slot,
binding_paired_at, capability_set_hash, resolved_at_ms, online_at_acceptance,
media_clip_capable, overlay_capable,
interrupt_capable, interrupt_resume_ready, status, reason_code, generation,
revision, ready_at, scheduled_at, started_at, ended_at, last_receipt_at,
updated_at`

func scanTransmission(row sqlScanner) (Transmission, error) {
	var transmission Transmission
	var includeOrigin int
	err := row.Scan(
		&transmission.ID, &transmission.MediaID, &transmission.SourceOrbitID,
		&transmission.SourceActorID, &transmission.SourceSlot,
		&transmission.PlaybackDomainKind, &transmission.PlaybackDomainID,
		&transmission.AudienceKind, &transmission.OriginKind, &includeOrigin,
		&transmission.RequestedDelivery, &transmission.EffectiveDelivery,
		&transmission.DowngradeReason, &transmission.Status,
		&transmission.ReasonCode, &transmission.CancellationCause,
		&transmission.AcceptedAt, &transmission.ExpiresAt,
		&transmission.Revision, &transmission.UpdatedAt,
		&transmission.CompletedAt, &transmission.AirID,
		&transmission.AirPolicyRevision, &transmission.AirPolicyOperation,
		&transmission.AirPolicyResult,
	)
	transmission.IncludeOrigin = includeOrigin != 0
	return transmission, err
}

func scanTransmissionTarget(row sqlScanner) (TransmissionTarget, error) {
	var target TransmissionTarget
	var online, mediaClip, overlay, interrupt, interruptResume int
	err := row.Scan(
		&target.TransmissionID, &target.OrbitID, &target.ActorID, &target.Slot,
		&target.BindingPairedAt, &target.CapabilitySetHash, &target.ResolvedAtMS,
		&online, &mediaClip, &overlay, &interrupt,
		&interruptResume, &target.Status, &target.ReasonCode,
		&target.Generation, &target.Revision, &target.ReadyAt,
		&target.ScheduledAt, &target.StartedAt, &target.EndedAt,
		&target.LastReceiptAt, &target.UpdatedAt,
	)
	target.OnlineAtAcceptance = online != 0
	target.MediaClipCapable = mediaClip != 0
	target.OverlayCapable = overlay != 0
	target.InterruptCapable = interrupt != 0
	target.InterruptResumeReady = interruptResume != 0
	return target, err
}

func transmissionTargetCapabilityHash(
	mediaClip, overlay, interrupt, interruptResume bool,
) string {
	return transmissionTargetCapabilitySetHash(
		nil, mediaClip, overlay, interrupt, interruptResume,
	)
}

func transmissionTargetCapabilitySetHash(
	registered []string,
	mediaClip, overlay, interrupt, interruptResume bool,
) string {
	capabilitySet := make(map[string]struct{}, len(registered)+3)
	for _, capability := range registered {
		if capability != "" {
			capabilitySet[capability] = struct{}{}
		}
	}
	if mediaClip {
		capabilitySet[TransmissionCapabilityMediaClip] = struct{}{}
	}
	if overlay {
		capabilitySet[TransmissionCapabilityOverlayMix] = struct{}{}
	}
	if interrupt && interruptResume {
		capabilitySet[TransmissionCapabilityInterrupt] = struct{}{}
	}
	capabilities := make([]string, 0, len(capabilitySet))
	for capability := range capabilitySet {
		capabilities = append(capabilities, capability)
	}
	sort.Strings(capabilities)
	return hashToken(strings.Join(capabilities, ","))
}

func validTransmissionDelivery(delivery TransmissionDelivery) bool {
	switch delivery {
	case TransmissionDeliveryOverlay, TransmissionDeliveryInterrupt,
		TransmissionDeliveryAfterCurrent:
		return true
	default:
		return false
	}
}

func validTransmissionTargetReason(status TransmissionTargetStatus, reason TransmissionReason) bool {
	switch status {
	case TransmissionTargetAccepted, TransmissionTargetPreparing,
		TransmissionTargetReady, TransmissionTargetScheduled,
		TransmissionTargetPlaying:
		return reason == ""
	case TransmissionTargetCancelling:
		return isCancellationReason(reason)
	case TransmissionTargetPlayed:
		return reason == TransmissionReasonCompleted
	case TransmissionTargetMissedOffline:
		return reason == TransmissionReasonOfflineAtAcceptance ||
			reason == TransmissionReasonOfflineBeforePrepare ||
			reason == TransmissionReasonOfflineBeforeStart
	case TransmissionTargetMissedDND:
		return reason == TransmissionReasonLocalDND || reason == TransmissionReasonOrbitDND
	case TransmissionTargetMissedNotReady:
		return reason == TransmissionReasonPrepareDeadline
	case TransmissionTargetBlocked:
		return reason == TransmissionReasonActorBlocked || reason == TransmissionReasonOrbitBlocked ||
			reason == TransmissionReasonReported
	case TransmissionTargetFailed:
		return isFailureReason(reason)
	case TransmissionTargetCancelled:
		return isCancellationReason(reason)
	case TransmissionTargetExpired:
		return reason == TransmissionReasonDeliveryExpired
	default:
		return false
	}
}

func isFailureReason(reason TransmissionReason) bool {
	switch reason {
	case TransmissionReasonMediaDownloadFailed, TransmissionReasonMediaAuthFailed,
		TransmissionReasonMediaExpired, TransmissionReasonHashMismatch,
		TransmissionReasonDecodeFailed, TransmissionReasonDurationMismatch,
		TransmissionReasonClockUnsynchronized, TransmissionReasonStalePlay,
		TransmissionReasonDeviceUnavailable, TransmissionReasonAudioGraphFailed,
		TransmissionReasonConnectionLost, TransmissionReasonCapabilityLost,
		TransmissionReasonInterruptCapabilityLost,
		TransmissionReasonCancelUnacknowledged, TransmissionReasonInternalError:
		return true
	default:
		return false
	}
}

func isCancellationReason(reason TransmissionReason) bool {
	switch reason {
	case TransmissionReasonSenderCancelled, TransmissionReasonMediaDeleted,
		TransmissionReasonMediaExpired, TransmissionReasonModerationDisabled,
		TransmissionReasonApproachLeft, TransmissionReasonApproachApart,
		TransmissionReasonTargetRevoked, TransmissionReasonDNDEnabled,
		TransmissionReasonSenderBlocked, TransmissionReasonReported,
		TransmissionReasonCoordinatorRestarted:
		return true
	default:
		return false
	}
}

func validateCreateTransmission(params CreateTransmissionParams) error {
	if !mediaItemIDPattern.MatchString(params.MediaID) ||
		params.SourceOrbitID <= 0 || params.SourceActorID <= 0 ||
		params.PlaybackDomainID <= 0 || params.AcceptedAt <= 0 ||
		len(params.Targets) == 0 || len(params.Targets) > 64 ||
		len(params.DowngradeReason) > 64 {
		return fmt.Errorf("%w: invalid identity, time, or target count", ErrTransmissionInvalid)
	}
	if params.AirID == "" {
		if params.AirPolicyRevision != 0 {
			return fmt.Errorf("%w: personal work has an Air policy revision", ErrTransmissionInvalid)
		}
	} else if !airPolicyAirIDPattern.MatchString(params.AirID) || params.AirPolicyRevision <= 0 ||
		!validAirPolicyOperation(params.AirPolicyOperation) || params.AirPolicyResult != "allowed" {
		return fmt.Errorf("%w: invalid Air policy snapshot", ErrTransmissionInvalid)
	}
	if params.SourceSlot != "" && !transmissionSlotPattern.MatchString(params.SourceSlot) {
		return fmt.Errorf("%w: invalid source slot", ErrTransmissionInvalid)
	}
	switch params.PlaybackDomainKind {
	case PlaybackDomainOrbit, PlaybackDomainApproach:
	default:
		return fmt.Errorf("%w: invalid playback domain", ErrTransmissionInvalid)
	}
	switch params.AudienceKind {
	case TransmissionAudienceThisPulsar, TransmissionAudienceOwnBarycenter,
		TransmissionAudienceCurrentAir, TransmissionAudienceExplicit:
	default:
		return fmt.Errorf("%w: invalid audience", ErrTransmissionInvalid)
	}
	switch params.OriginKind {
	case TransmissionOriginMicrophone, TransmissionOriginFile,
		TransmissionOriginTelegram, TransmissionOriginBuiltin:
	default:
		return fmt.Errorf("%w: invalid origin", ErrTransmissionInvalid)
	}
	if !validTransmissionDelivery(params.RequestedDelivery) ||
		!validTransmissionDelivery(params.EffectiveDelivery) {
		return fmt.Errorf("%w: invalid delivery", ErrTransmissionInvalid)
	}
	if params.RequestedDelivery == params.EffectiveDelivery && params.DowngradeReason != "" {
		return fmt.Errorf("%w: downgrade reason without downgrade", ErrTransmissionInvalid)
	}
	if params.RequestedDelivery != params.EffectiveDelivery && params.DowngradeReason == "" {
		return fmt.Errorf("%w: missing downgrade reason", ErrTransmissionInvalid)
	}
	seenActors := make(map[int64]struct{}, len(params.Targets))
	seenSlots := make(map[string]struct{}, len(params.Targets))
	for _, target := range params.Targets {
		if target.OrbitID <= 0 || target.ActorID <= 0 ||
			!transmissionSlotPattern.MatchString(target.Slot) {
			return fmt.Errorf("%w: malformed target", ErrTransmissionTargetInvalid)
		}
		if target.CapabilitySetHash != "" &&
			!transmissionDigestPattern.MatchString(target.CapabilitySetHash) {
			return fmt.Errorf("%w: malformed capability snapshot", ErrTransmissionTargetInvalid)
		}
		if _, exists := seenActors[target.ActorID]; exists {
			return fmt.Errorf("%w: duplicate target actor", ErrTransmissionTargetInvalid)
		}
		seenActors[target.ActorID] = struct{}{}
		key := fmt.Sprintf("%d/%s", target.OrbitID, target.Slot)
		if _, exists := seenSlots[key]; exists {
			return fmt.Errorf("%w: duplicate target slot", ErrTransmissionTargetInvalid)
		}
		seenSlots[key] = struct{}{}
		status := target.Status
		if status == "" {
			status = TransmissionTargetAccepted
		}
		if !validTransmissionTargetReason(status, target.ReasonCode) {
			return fmt.Errorf("%w: invalid initial target status/reason", ErrTransmissionInvalid)
		}
	}
	return nil
}

func transmissionExpiry(acceptedAt, mediaExpiresAt int64, requested TransmissionDelivery) int64 {
	if requested == TransmissionDeliveryAfterCurrent {
		return mediaExpiresAt
	}
	deadline := acceptedAt + int64((5*time.Minute)/time.Millisecond)
	if mediaExpiresAt < deadline {
		return mediaExpiresAt
	}
	return deadline
}

func validateTransmissionSourceTx(
	tx *sql.Tx,
	params CreateTransmissionParams,
) (MediaItem, error) {
	mediaItem, err := scanMediaItem(tx.QueryRow(`SELECT `+mediaItemColumns+`
FROM media_items
WHERE id = ? AND owner_orbit_id = ?`, params.MediaID, params.SourceOrbitID))
	if errors.Is(err, sql.ErrNoRows) {
		return MediaItem{}, ErrTransmissionMediaInvalid
	}
	if err != nil {
		return MediaItem{}, err
	}
	if mediaItem.Status != MediaStatusReady || mediaItem.PublishedAt > params.AcceptedAt ||
		mediaItem.ExpiresAt <= params.AcceptedAt || mediaItem.Kind == MediaKindAudioTrack {
		return MediaItem{}, ErrTransmissionMediaInvalid
	}

	var actorKind, role, orbitStatus string
	var revokedAt sql.NullInt64
	err = tx.QueryRow(`SELECT a.kind, a.revoked_at, m.role, o.status
FROM actors a
JOIN memberships m ON m.actor_id = a.id AND m.orbit_id = ? AND m.left_at IS NULL
JOIN orbits o ON o.id = m.orbit_id
WHERE a.id = ?`, params.SourceOrbitID, params.SourceActorID).Scan(
		&actorKind, &revokedAt, &role, &orbitStatus,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return MediaItem{}, ErrTransmissionMediaInvalid
	}
	if err != nil {
		return MediaItem{}, err
	}
	if revokedAt.Valid || orbitStatus != "active" || role == "satellite" {
		return MediaItem{}, ErrTransmissionMediaInvalid
	}
	switch actorKind {
	case "app_installation":
		if params.SourceSlot == "" {
			return MediaItem{}, ErrTransmissionMediaInvalid
		}
		var matches int
		if err := tx.QueryRow(`SELECT COUNT(*)
FROM installation_credentials ic
JOIN slots sl ON sl.orbit_id = ic.slot_orbit_id AND sl.slot = ic.slot_name
  AND sl.revoked_at IS NULL AND sl.token_hash = ic.binding_token_hash
  AND COALESCE(sl.paired_at, 0) = ic.slot_paired_at
WHERE ic.actor_id = ? AND ic.slot_orbit_id = ? AND ic.slot_name = ?`,
			params.SourceActorID, params.SourceOrbitID, params.SourceSlot,
		).Scan(&matches); err != nil {
			return MediaItem{}, err
		} else if matches != 1 {
			return MediaItem{}, ErrTransmissionMediaInvalid
		}
	case "telegram_user":
		if params.SourceSlot != "" {
			return MediaItem{}, ErrTransmissionMediaInvalid
		}
	default:
		return MediaItem{}, ErrTransmissionMediaInvalid
	}
	return mediaItem, nil
}

func resolveTransmissionTargetBindingTx(
	tx *sql.Tx,
	target CreateTransmissionTarget,
) (int64, error) {
	var pairedAt int64
	err := tx.QueryRow(`SELECT ic.slot_paired_at
FROM installation_credentials ic
JOIN actors a ON a.id = ic.actor_id AND a.revoked_at IS NULL
JOIN memberships m ON m.actor_id = ic.actor_id
  AND m.orbit_id = ic.slot_orbit_id AND m.left_at IS NULL
JOIN orbits o ON o.id = ic.slot_orbit_id AND o.status = 'active'
JOIN slots sl ON sl.orbit_id = ic.slot_orbit_id AND sl.slot = ic.slot_name
  AND sl.revoked_at IS NULL AND sl.token_hash = ic.binding_token_hash
  AND COALESCE(sl.paired_at, 0) = ic.slot_paired_at
WHERE ic.slot_orbit_id = ? AND ic.actor_id = ? AND ic.slot_name = ?`,
		target.OrbitID, target.ActorID, target.Slot,
	).Scan(&pairedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrTransmissionTargetInvalid
	}
	return pairedAt, err
}

func (s *Store) CreateTransmission(params CreateTransmissionParams) (TransmissionCreation, error) {
	if err := validateCreateTransmission(params); err != nil {
		return TransmissionCreation{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return TransmissionCreation{}, err
	}
	defer tx.Rollback()
	mediaItem, err := validateTransmissionSourceTx(tx, params)
	if err != nil {
		return TransmissionCreation{}, err
	}
	creation, err := s.createTransmissionTx(tx, params, mediaItem)
	if err != nil {
		return TransmissionCreation{}, err
	}
	if err := s.checkpoint("transmission_create_before_commit"); err != nil {
		return TransmissionCreation{}, err
	}
	if err := tx.Commit(); err != nil {
		return TransmissionCreation{}, err
	}
	return creation, nil
}

// createTransmissionTx writes an already-authorized acceptance snapshot into
// the caller's immediate writer transaction. Audience resolution, policy
// decisions, confirmation consumption and idempotency records can therefore
// be sealed atomically by higher-level repository entry points.
func (s *Store) createTransmissionTx(
	tx *sql.Tx,
	params CreateTransmissionParams,
	mediaItem MediaItem,
) (TransmissionCreation, error) {
	expiresAt := transmissionExpiry(params.AcceptedAt, mediaItem.ExpiresAt, params.RequestedDelivery)
	if expiresAt <= params.AcceptedAt {
		return TransmissionCreation{}, ErrTransmissionMediaInvalid
	}
	id := ulid.NewTransmissionID(time.UnixMilli(params.AcceptedAt))
	if _, err := tx.Exec(`INSERT INTO transmissions(
  id, media_id, source_orbit_id, source_actor_id, source_slot,
  playback_domain_kind, playback_domain_id, audience_kind, origin_kind,
  include_origin, requested_delivery, effective_delivery, downgrade_reason,
  status, accepted_at, expires_at, revision, updated_at,
  air_id, air_policy_revision, air_policy_operation, air_policy_result
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'accepted', ?, ?, 1, ?, ?, ?, ?, ?)`,
		id, params.MediaID, params.SourceOrbitID, params.SourceActorID,
		params.SourceSlot, params.PlaybackDomainKind, params.PlaybackDomainID,
		params.AudienceKind, params.OriginKind, params.IncludeOrigin,
		params.RequestedDelivery, params.EffectiveDelivery, params.DowngradeReason,
		params.AcceptedAt, expiresAt, params.AcceptedAt, params.AirID,
		params.AirPolicyRevision, params.AirPolicyOperation, params.AirPolicyResult,
	); err != nil {
		return TransmissionCreation{}, err
	}
	if _, err := tx.Exec(`INSERT INTO transmission_scheduler_state(
  transmission_id, updated_at
) VALUES(?, ?)`, id, params.AcceptedAt); err != nil {
		return TransmissionCreation{}, err
	}
	createdTargets := make([]TransmissionTarget, 0, len(params.Targets))
	for _, candidate := range params.Targets {
		pairedAt, err := resolveTransmissionTargetBindingTx(tx, candidate)
		if err != nil {
			return TransmissionCreation{}, err
		}
		status := candidate.Status
		if status == "" {
			status = TransmissionTargetAccepted
		}
		endedAt, lastReceiptAt := int64(0), int64(0)
		if terminalTransmissionTargetStatus(status) {
			endedAt, lastReceiptAt = params.AcceptedAt, params.AcceptedAt
		}
		if _, err := tx.Exec(`INSERT INTO transmission_targets(
  transmission_id, orbit_id, actor_id, slot, binding_paired_at,
  capability_set_hash, resolved_at_ms,
  online_at_acceptance, media_clip_capable, overlay_capable,
  interrupt_capable, interrupt_resume_ready, status, reason_code,
  generation, revision, ended_at, last_receipt_at, updated_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, 1, ?, ?, ?)`,
			id, candidate.OrbitID, candidate.ActorID, candidate.Slot, pairedAt,
			func() string {
				if candidate.CapabilitySetHash != "" {
					return candidate.CapabilitySetHash
				}
				return transmissionTargetCapabilityHash(
					candidate.MediaClipCapable, candidate.OverlayCapable,
					candidate.InterruptCapable, candidate.InterruptResumeReady,
				)
			}(), params.AcceptedAt,
			candidate.OnlineAtAcceptance, candidate.MediaClipCapable,
			candidate.OverlayCapable, candidate.InterruptCapable,
			candidate.InterruptResumeReady, status, candidate.ReasonCode,
			endedAt, lastReceiptAt, params.AcceptedAt,
		); err != nil {
			return TransmissionCreation{}, err
		}
		target, err := scanTransmissionTarget(tx.QueryRow(
			`SELECT `+transmissionTargetColumns+` FROM transmission_targets
WHERE transmission_id = ? AND orbit_id = ? AND slot = ?`,
			id, candidate.OrbitID, candidate.Slot,
		))
		if err != nil {
			return TransmissionCreation{}, err
		}
		if _, err := createTransmissionInboxItemTx(tx, target, params.AcceptedAt); err != nil {
			return TransmissionCreation{}, err
		}
		createdTargets = append(createdTargets, target)
	}
	transmission, err := recomputeTransmissionTx(tx, id, params.AcceptedAt)
	if err != nil {
		return TransmissionCreation{}, err
	}
	return TransmissionCreation{Transmission: transmission, Targets: createdTargets}, nil
}

func (s *Store) GetTransmission(id string) (*Transmission, error) {
	if !transmissionIDPattern.MatchString(id) {
		return nil, nil
	}
	transmission, err := scanTransmission(s.db.QueryRow(
		`SELECT `+transmissionColumns+` FROM transmissions WHERE id = ?`, id,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &transmission, nil
}

func (s *Store) TransmissionTargets(id string) ([]TransmissionTarget, error) {
	if !transmissionIDPattern.MatchString(id) {
		return nil, ErrTransmissionNotFound
	}
	rows, err := s.db.Query(`SELECT `+transmissionTargetColumns+`
FROM transmission_targets WHERE transmission_id = ? ORDER BY orbit_id, slot`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var targets []TransmissionTarget
	for rows.Next() {
		target, err := scanTransmissionTarget(rows)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(targets) == 0 {
		if transmission, err := s.GetTransmission(id); err != nil {
			return nil, err
		} else if transmission == nil {
			return nil, ErrTransmissionNotFound
		}
	}
	return targets, nil
}

func (s *Store) ListTransmissionDomainFIFO(
	domainKind PlaybackDomainKind,
	domainID int64,
	limit int,
) ([]Transmission, error) {
	if (domainKind != PlaybackDomainOrbit && domainKind != PlaybackDomainApproach) ||
		domainID <= 0 || limit <= 0 || limit > 1000 {
		return nil, ErrTransmissionInvalid
	}
	rows, err := s.db.Query(`SELECT `+transmissionColumns+`
FROM transmissions
WHERE playback_domain_kind = ? AND playback_domain_id = ?
ORDER BY accepted_at, id LIMIT ?`, domainKind, domainID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var transmissions []Transmission
	for rows.Next() {
		transmission, err := scanTransmission(rows)
		if err != nil {
			return nil, err
		}
		transmissions = append(transmissions, transmission)
	}
	return transmissions, rows.Err()
}

func terminalTransmissionTargetStatus(status TransmissionTargetStatus) bool {
	switch status {
	case TransmissionTargetPlayed, TransmissionTargetMissedOffline,
		TransmissionTargetMissedDND, TransmissionTargetMissedNotReady,
		TransmissionTargetBlocked, TransmissionTargetFailed,
		TransmissionTargetCancelled, TransmissionTargetExpired:
		return true
	default:
		return false
	}
}

func validTransmissionTargetTransition(from, to TransmissionTargetStatus) bool {
	if from == to || terminalTransmissionTargetStatus(from) {
		return false
	}
	switch from {
	case TransmissionTargetAccepted:
		return to == TransmissionTargetPreparing || to == TransmissionTargetScheduled ||
			to == TransmissionTargetPlaying || terminalWithoutPlayback(to)
	case TransmissionTargetPreparing:
		return to == TransmissionTargetReady || to == TransmissionTargetCancelling ||
			terminalWithoutPlayback(to)
	case TransmissionTargetReady:
		return to == TransmissionTargetScheduled || to == TransmissionTargetCancelling ||
			terminalWithoutPlayback(to)
	case TransmissionTargetScheduled:
		return to == TransmissionTargetPlaying || to == TransmissionTargetCancelling ||
			terminalWithoutPlayback(to)
	case TransmissionTargetPlaying:
		return to == TransmissionTargetPlayed || to == TransmissionTargetCancelling ||
			to == TransmissionTargetCancelled || to == TransmissionTargetFailed
	case TransmissionTargetCancelling:
		return to == TransmissionTargetCancelled || to == TransmissionTargetFailed
	default:
		return false
	}
}

func terminalWithoutPlayback(status TransmissionTargetStatus) bool {
	return terminalTransmissionTargetStatus(status) && status != TransmissionTargetPlayed
}

func (s *Store) TransitionTransmissionTarget(
	params TransitionTransmissionTargetParams,
) (TransmissionTargetTransition, error) {
	if !transmissionIDPattern.MatchString(params.TransmissionID) ||
		params.OrbitID <= 0 || params.ActorID <= 0 ||
		!transmissionSlotPattern.MatchString(params.Slot) || params.ExpectedRevision <= 0 ||
		params.Generation <= 0 || params.OccurredAt <= 0 ||
		!validTransmissionTargetReason(params.Status, params.ReasonCode) {
		return TransmissionTargetTransition{}, ErrTransmissionInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return TransmissionTargetTransition{}, err
	}
	defer tx.Rollback()
	target, err := scanTransmissionTarget(tx.QueryRow(
		`SELECT `+transmissionTargetColumns+` FROM transmission_targets
WHERE transmission_id = ? AND orbit_id = ? AND actor_id = ? AND slot = ?`,
		params.TransmissionID, params.OrbitID, params.ActorID, params.Slot,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return TransmissionTargetTransition{}, ErrTransmissionNotFound
	}
	if err != nil {
		return TransmissionTargetTransition{}, err
	}
	if target.Generation != params.Generation {
		return TransmissionTargetTransition{}, ErrTransmissionStateConflict
	}
	if target.Status == params.Status && target.ReasonCode == params.ReasonCode {
		transmission, err := scanTransmission(tx.QueryRow(
			`SELECT `+transmissionColumns+` FROM transmissions WHERE id = ?`,
			params.TransmissionID,
		))
		if err != nil {
			return TransmissionTargetTransition{}, err
		}
		if err := tx.Commit(); err != nil {
			return TransmissionTargetTransition{}, err
		}
		return TransmissionTargetTransition{
			Transmission: transmission, Target: target, Changed: false,
		}, nil
	}
	if target.Revision != params.ExpectedRevision ||
		params.OccurredAt < target.UpdatedAt ||
		!validTransmissionTargetTransition(target.Status, params.Status) {
		return TransmissionTargetTransition{}, ErrTransmissionStateConflict
	}
	readyAt, scheduledAt := target.ReadyAt, target.ScheduledAt
	startedAt, endedAt := target.StartedAt, target.EndedAt
	switch params.Status {
	case TransmissionTargetReady:
		if readyAt == 0 {
			readyAt = params.OccurredAt
		}
	case TransmissionTargetScheduled:
		if scheduledAt == 0 {
			scheduledAt = params.OccurredAt
		}
	case TransmissionTargetPlaying:
		if startedAt == 0 {
			startedAt = params.OccurredAt
		}
	default:
		if terminalTransmissionTargetStatus(params.Status) && endedAt == 0 {
			endedAt = params.OccurredAt
		}
	}
	result, err := tx.Exec(`UPDATE transmission_targets
SET status = ?, reason_code = ?, revision = revision + 1,
    ready_at = ?, scheduled_at = ?, started_at = ?, ended_at = ?,
    last_receipt_at = ?, updated_at = ?
WHERE transmission_id = ? AND orbit_id = ? AND actor_id = ? AND slot = ?
  AND revision = ? AND generation = ?`,
		params.Status, params.ReasonCode, readyAt, scheduledAt, startedAt,
		endedAt, params.OccurredAt, params.OccurredAt, params.TransmissionID,
		params.OrbitID, params.ActorID, params.Slot, params.ExpectedRevision,
		params.Generation,
	)
	if err != nil {
		return TransmissionTargetTransition{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return TransmissionTargetTransition{}, err
		}
		return TransmissionTargetTransition{}, ErrTransmissionStateConflict
	}
	target, err = scanTransmissionTarget(tx.QueryRow(
		`SELECT `+transmissionTargetColumns+` FROM transmission_targets
WHERE transmission_id = ? AND orbit_id = ? AND slot = ?`,
		params.TransmissionID, params.OrbitID, params.Slot,
	))
	if err != nil {
		return TransmissionTargetTransition{}, err
	}
	if _, err := createTransmissionInboxItemTx(tx, target, params.OccurredAt); err != nil {
		return TransmissionTargetTransition{}, err
	}
	transmission, err := recomputeTransmissionTx(tx, params.TransmissionID, params.OccurredAt)
	if err != nil {
		return TransmissionTargetTransition{}, err
	}
	if err := s.checkpoint("transmission_target_transition_before_commit"); err != nil {
		return TransmissionTargetTransition{}, err
	}
	if err := tx.Commit(); err != nil {
		return TransmissionTargetTransition{}, err
	}
	return TransmissionTargetTransition{
		Transmission: transmission, Target: target, Changed: true,
	}, nil
}

func validTransmissionCause(reason TransmissionReason) bool {
	switch reason {
	case TransmissionReasonSenderCancelled, TransmissionReasonMediaDeleted,
		TransmissionReasonMediaExpired, TransmissionReasonModerationDisabled,
		TransmissionReasonApproachLeft, TransmissionReasonApproachApart,
		TransmissionReasonCoordinatorRestarted, TransmissionReasonDeliveryExpired:
		return true
	default:
		return false
	}
}

// CommitTransmissionCause records the one transmission-level cancellation or
// expiry cause before workers move individual targets. It never rewrites the
// accepted snapshot and is exact-retry idempotent.
func (s *Store) CommitTransmissionCause(
	params CommitTransmissionCauseParams,
) (Transmission, error) {
	if !transmissionIDPattern.MatchString(params.TransmissionID) ||
		params.ExpectedRevision <= 0 || params.OccurredAt <= 0 ||
		!validTransmissionCause(params.Cause) {
		return Transmission{}, ErrTransmissionInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return Transmission{}, err
	}
	defer tx.Rollback()
	transmission, err := scanTransmission(tx.QueryRow(
		`SELECT `+transmissionColumns+` FROM transmissions WHERE id = ?`,
		params.TransmissionID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return Transmission{}, ErrTransmissionNotFound
	}
	if err != nil {
		return Transmission{}, err
	}
	if transmission.CancellationCause == params.Cause {
		if err := tx.Commit(); err != nil {
			return Transmission{}, err
		}
		return transmission, nil
	}
	if transmission.Revision != params.ExpectedRevision ||
		transmission.CancellationCause != "" ||
		params.OccurredAt < transmission.UpdatedAt || transmission.CompletedAt != 0 {
		return Transmission{}, ErrTransmissionStateConflict
	}
	result, err := tx.Exec(`UPDATE transmissions
SET cancellation_cause = ?, revision = revision + 1, updated_at = ?
WHERE id = ? AND revision = ? AND cancellation_cause = '' AND completed_at = 0`,
		params.Cause, params.OccurredAt, params.TransmissionID,
		params.ExpectedRevision,
	)
	if err != nil {
		return Transmission{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return Transmission{}, err
		}
		return Transmission{}, ErrTransmissionStateConflict
	}
	transmission, err = recomputeTransmissionTx(
		tx, params.TransmissionID, params.OccurredAt,
	)
	if err != nil {
		return Transmission{}, err
	}
	if err := tx.Commit(); err != nil {
		return Transmission{}, err
	}
	return transmission, nil
}

// AdvanceTransmissionTargetGeneration invalidates stale prepare/play receipts
// before a scheduler retry. Receipt generation is lifecycle state, separate
// from the immutable credential binding generation.
func (s *Store) AdvanceTransmissionTargetGeneration(
	transmissionID string,
	orbitID, actorID int64,
	slot string,
	expectedRevision, expectedGeneration, now int64,
) (TransmissionTarget, error) {
	if !transmissionIDPattern.MatchString(transmissionID) || orbitID <= 0 ||
		actorID <= 0 || !transmissionSlotPattern.MatchString(slot) ||
		expectedRevision <= 0 || expectedGeneration <= 0 || now <= 0 {
		return TransmissionTarget{}, ErrTransmissionInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return TransmissionTarget{}, err
	}
	defer tx.Rollback()
	target, err := scanTransmissionTarget(tx.QueryRow(
		`SELECT `+transmissionTargetColumns+` FROM transmission_targets
WHERE transmission_id = ? AND orbit_id = ? AND actor_id = ? AND slot = ?`,
		transmissionID, orbitID, actorID, slot,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return TransmissionTarget{}, ErrTransmissionNotFound
	}
	if err != nil {
		return TransmissionTarget{}, err
	}
	if target.Revision != expectedRevision || target.Generation != expectedGeneration ||
		now < target.UpdatedAt || terminalTransmissionTargetStatus(target.Status) ||
		target.Status == TransmissionTargetPlaying ||
		target.Status == TransmissionTargetCancelling {
		return TransmissionTarget{}, ErrTransmissionStateConflict
	}
	result, err := tx.Exec(`UPDATE transmission_targets
SET generation = generation + 1, revision = revision + 1, updated_at = ?
WHERE transmission_id = ? AND orbit_id = ? AND actor_id = ? AND slot = ?
  AND revision = ? AND generation = ?`, now, transmissionID, orbitID,
		actorID, slot, expectedRevision, expectedGeneration)
	if err != nil {
		return TransmissionTarget{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return TransmissionTarget{}, err
		}
		return TransmissionTarget{}, ErrTransmissionStateConflict
	}
	target, err = scanTransmissionTarget(tx.QueryRow(
		`SELECT `+transmissionTargetColumns+` FROM transmission_targets
WHERE transmission_id = ? AND orbit_id = ? AND slot = ?`,
		transmissionID, orbitID, slot,
	))
	if err != nil {
		return TransmissionTarget{}, err
	}
	if err := tx.Commit(); err != nil {
		return TransmissionTarget{}, err
	}
	return target, nil
}

func recomputeTransmissionTx(tx *sql.Tx, id string, now int64) (Transmission, error) {
	transmission, err := scanTransmission(tx.QueryRow(
		`SELECT `+transmissionColumns+` FROM transmissions WHERE id = ?`, id,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return Transmission{}, ErrTransmissionNotFound
	}
	if err != nil {
		return Transmission{}, err
	}
	rows, err := tx.Query(`SELECT status, online_at_acceptance
FROM transmission_targets WHERE transmission_id = ?`, id)
	if err != nil {
		return Transmission{}, err
	}
	defer rows.Close()
	counts := make(map[TransmissionTargetStatus]int)
	total, onlineAtAcceptance := 0, 0
	for rows.Next() {
		var status TransmissionTargetStatus
		var online int
		if err := rows.Scan(&status, &online); err != nil {
			return Transmission{}, err
		}
		counts[status]++
		total++
		if online != 0 {
			onlineAtAcceptance++
		}
	}
	if err := rows.Err(); err != nil {
		return Transmission{}, err
	}
	if err := rows.Close(); err != nil {
		return Transmission{}, err
	}
	if total == 0 {
		return Transmission{}, ErrTransmissionTargetInvalid
	}
	status, reason, completedAt := deriveTransmissionAggregate(
		counts, total, onlineAtAcceptance, transmission.CancellationCause, now,
	)
	if status != transmission.Status || reason != transmission.ReasonCode ||
		completedAt != transmission.CompletedAt {
		result, err := tx.Exec(`UPDATE transmissions
SET status = ?, reason_code = ?, completed_at = ?,
    revision = revision + 1, updated_at = ?
WHERE id = ? AND revision = ?`, status, reason, completedAt, now, id, transmission.Revision)
		if err != nil {
			return Transmission{}, err
		}
		if changed, err := result.RowsAffected(); err != nil || changed != 1 {
			if err != nil {
				return Transmission{}, err
			}
			return Transmission{}, ErrTransmissionStateConflict
		}
		return scanTransmission(tx.QueryRow(
			`SELECT `+transmissionColumns+` FROM transmissions WHERE id = ?`, id,
		))
	}
	return transmission, nil
}

func deriveTransmissionAggregate(
	counts map[TransmissionTargetStatus]int,
	total, onlineAtAcceptance int,
	cancellationCause TransmissionReason,
	now int64,
) (TransmissionStatus, TransmissionReason, int64) {
	terminal := 0
	for status, count := range counts {
		if terminalTransmissionTargetStatus(status) {
			terminal += count
		}
	}
	if terminal != total {
		switch {
		case counts[TransmissionTargetPlaying] > 0:
			return TransmissionStatusPlaying, "", 0
		case counts[TransmissionTargetCancelling] > 0:
			return TransmissionStatusCancelling, "", 0
		case counts[TransmissionTargetScheduled] > 0:
			return TransmissionStatusScheduled, "", 0
		case counts[TransmissionTargetPreparing]+counts[TransmissionTargetReady] > 0:
			return TransmissionStatusPreparing, "", 0
		default:
			return TransmissionStatusAccepted, "", 0
		}
	}
	played := counts[TransmissionTargetPlayed]
	switch {
	case played == total:
		return TransmissionStatusPlayed, TransmissionReasonCompleted, now
	case played > 0:
		return TransmissionStatusPartial, TransmissionReasonPartialDelivery, now
	case cancellationCause != "" && cancellationCause != TransmissionReasonDeliveryExpired:
		return TransmissionStatusCancelled, cancellationCause, now
	case cancellationCause == TransmissionReasonDeliveryExpired ||
		counts[TransmissionTargetExpired] == total:
		return TransmissionStatusExpired, TransmissionReasonDeliveryExpired, now
	case onlineAtAcceptance == 0 ||
		counts[TransmissionTargetMissedOffline]+counts[TransmissionTargetMissedDND]+
			counts[TransmissionTargetBlocked] == total:
		return TransmissionStatusFailed, TransmissionReasonNoEligibleTargets, now
	case counts[TransmissionTargetMissedNotReady] > 0 &&
		counts[TransmissionTargetFailed]+counts[TransmissionTargetCancelled] == 0:
		return TransmissionStatusFailed, TransmissionReasonNoReadyTargets, now
	default:
		return TransmissionStatusFailed, TransmissionReasonAllTargetsFailed, now
	}
}

// AuthorizePersistedMediaDownload performs the exact target-row check inside
// the same immediate transaction as live bearer and media authorization.
func (s *Store) AuthorizePersistedMediaDownload(
	expected ActorContext,
	bearer string,
	target MediaTargetIdentity,
	now int64,
) (MediaItem, error) {
	return s.authorizeMediaDownload(
		expected, bearer, target.MediaID, true, &target, now, nil,
	)
}

// WithAuthorizedPersistedMediaDownload keeps the persisted target and active
// block decision pinned until authorized has acquired the canonical file
// descriptor. The callback follows the same constraints as
// WithAuthorizedMediaDownload.
func (s *Store) WithAuthorizedPersistedMediaDownload(
	expected ActorContext,
	bearer string,
	target MediaTargetIdentity,
	now int64,
	authorized func(MediaItem) error,
) (MediaItem, error) {
	if authorized == nil {
		return MediaItem{}, fmt.Errorf("%w: nil media download callback", ErrMediaInvalid)
	}
	return s.authorizeMediaDownload(
		expected, bearer, target.MediaID, true, &target, now, authorized,
	)
}

const mediaTargetACLQuery = `SELECT COUNT(*)
FROM transmission_targets tt
JOIN transmissions tr ON tr.id = tt.transmission_id
JOIN installation_credentials ic ON ic.actor_id = tt.actor_id
  AND ic.slot_orbit_id = tt.orbit_id AND ic.slot_name = tt.slot
  AND ic.slot_paired_at = tt.binding_paired_at
JOIN actors a ON a.id = ic.actor_id AND a.revoked_at IS NULL
JOIN slots sl ON sl.orbit_id = ic.slot_orbit_id AND sl.slot = ic.slot_name
  AND sl.revoked_at IS NULL AND sl.token_hash = ic.binding_token_hash
  AND COALESCE(sl.paired_at, 0) = ic.slot_paired_at
WHERE tr.media_id = ? AND tt.orbit_id = ? AND tt.actor_id = ? AND tt.slot = ?
  AND tt.status <> 'blocked'
  AND tt.reason_code NOT IN ('actor_blocked', 'orbit_blocked', 'sender_blocked')
  AND NOT EXISTS (
    SELECT 1 FROM blocks b
    WHERE b.revoked_at = 0 AND b.owner_orbit_id = tt.orbit_id
      AND (b.owner_scope = 'orbit'
        OR (b.owner_scope = 'actor' AND b.owner_actor_id = tt.actor_id))
      AND ((b.blocked_kind = 'actor' AND b.blocked_actor_id = tr.source_actor_id)
        OR (b.blocked_kind = 'orbit' AND b.blocked_orbit_id = tr.source_orbit_id))
  )
  AND NOT EXISTS (
    SELECT 1 FROM moderation_reports mr
    WHERE mr.reporter_actor_id = tt.actor_id AND mr.media_id = tr.media_id
  )`

func allowsMediaDownloadRow(row *sql.Row) (bool, error) {
	var matches int
	if err := row.Scan(&matches); err != nil {
		return false, err
	}
	return matches > 0, nil
}

// AllowsMediaDownload implements media.MediaTargetSnapshotReader without an
// import cycle. It grants only an exact still-live installation generation
// present in an accepted immutable target row. Current membership or approach
// state is never consulted, and active recipient blocks revoke the grant.
func (s *Store) AllowsMediaDownload(
	ctx context.Context,
	target MediaTargetIdentity,
) (bool, error) {
	if ctx == nil {
		return false, errors.New("nil transmission ACL context")
	}
	if !mediaItemIDPattern.MatchString(target.MediaID) || target.OrbitID <= 0 ||
		target.ActorID <= 0 || !transmissionSlotPattern.MatchString(target.Slot) {
		return false, nil
	}
	return allowsMediaDownloadRow(s.db.QueryRowContext(
		ctx, mediaTargetACLQuery,
		target.MediaID, target.OrbitID, target.ActorID, target.Slot,
	))
}
