package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	automationcontract "relux.works/duet/coordinator/internal/automation"
	"relux.works/duet/coordinator/internal/ulid"
)

var (
	ErrAutomationRateLimited         = errors.New("automation attempt rate limit exceeded")
	ErrAutomationExecutionInProgress = errors.New("automation execution is already in progress")
	ErrAutomationQuietHours          = errors.New("automation is denied by quiet hours")
	ErrAutomationCueNotReady         = errors.New("automation cue is not ready")
	ErrAutomationCapabilityMissing   = errors.New("automation target capability is missing")
)

type AutomationRateLimitError struct {
	RetryAfter time.Duration
}

func (e *AutomationRateLimitError) Error() string { return ErrAutomationRateLimited.Error() }
func (e *AutomationRateLimitError) Unwrap() error { return ErrAutomationRateLimited }

type AutomationRuntimeTriggerParams struct {
	Secret           string
	IdempotencyKey   string
	RequestDigest    string
	CueID            string
	AudienceKind     automationcontract.AudienceKind
	TargetReferences []string
	Availability     []TransmissionTargetAvailability
	AttemptedAt      int64
}

type AutomationRuntimeResult struct {
	Execution    AutomationExecution
	Transmission TransmissionCreation
	Replayed     bool
}

type AutomationDueOccurrence struct {
	ScheduleID       string
	ScheduleRevision int64
	ScheduledUTC     int64
}

type automationAttempt struct {
	ID            int64
	RequestDigest string
	Outcome       string
	ReasonCode    string
	RetryAfterMS  int64
	ExecutionID   string
}

func scanAutomationAttempt(row interface{ Scan(...any) error }) (automationAttempt, error) {
	var attempt automationAttempt
	err := row.Scan(&attempt.ID, &attempt.RequestDigest, &attempt.Outcome,
		&attempt.ReasonCode, &attempt.RetryAfterMS, &attempt.ExecutionID)
	return attempt, err
}

func automationRuntimeError(reason string, retryMS int64) error {
	switch automationcontract.DenialReason(reason) {
	case automationcontract.DenyAutomationDisabled:
		return ErrAutomationDisabled
	case automationcontract.DenyInvalidCredential, automationcontract.DenyPrincipalDisabled,
		automationcontract.DenyPrincipalRevoked, automationcontract.DenyPrincipalExpired:
		return ErrAutomationInvalidCredential
	case automationcontract.DenyIdempotencyConflict:
		return ErrAutomationIdempotencyConflict
	case automationcontract.DenyTooManyAttempts:
		return &AutomationRateLimitError{RetryAfter: time.Duration(retryMS) * time.Millisecond}
	case automationcontract.DenyExecutionInProgress:
		return ErrAutomationExecutionInProgress
	case automationcontract.DenyInsufficientScope:
		return ErrInsufficientCapability
	case automationcontract.DenyCueNotFound:
		return ErrSavedCueNotFound
	case automationcontract.DenyCueNotReady, automationcontract.DenyCueNotEligible:
		return ErrAutomationCueNotReady
	case automationcontract.DenyQuietHours:
		return ErrAutomationQuietHours
	case automationcontract.DenyAudienceNotAllowed, automationcontract.DenyAirPolicy:
		return ErrAutomationAudienceNotAllowed
	case automationcontract.DenyAutomationCapabilityMissing,
		automationcontract.DenyDeliveryCapabilityMissing:
		return ErrAutomationCapabilityMissing
	default:
		return ErrAutomationInvalid
	}
}

func automationRuntimeActorTx(tx *sql.Tx, actorID, orbitID int64) (ActorContext, error) {
	var ctx ActorContext
	err := tx.QueryRow(`SELECT ic.slot_orbit_id, ic.actor_id, m.role, ic.slot_name
FROM installation_credentials ic
JOIN actors a ON a.id = ic.actor_id AND a.revoked_at IS NULL
JOIN memberships m ON m.actor_id = ic.actor_id AND m.orbit_id = ic.slot_orbit_id
  AND m.left_at IS NULL
JOIN orbits o ON o.id = m.orbit_id AND o.status = 'active'
JOIN slots sl ON sl.orbit_id = ic.slot_orbit_id AND sl.slot = ic.slot_name
  AND sl.revoked_at IS NULL AND sl.token_hash = ic.binding_token_hash
  AND COALESCE(sl.paired_at, 0) = ic.slot_paired_at
WHERE ic.actor_id = ? AND ic.slot_orbit_id = ?`, actorID, orbitID).Scan(
		&ctx.OrbitID, &ctx.ActorID, &ctx.Role, &ctx.Slot)
	if errors.Is(err, sql.ErrNoRows) {
		return ActorContext{}, ErrAutomationDisabled
	}
	if err != nil {
		return ActorContext{}, err
	}
	ctx.Capabilities = CapabilityNode | CapabilityControl
	return ctx, nil
}

func automationQuietAt(timezone, raw string, now int64) (bool, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return false, ErrAutomationDisabled
	}
	var windows []AutomationQuietWindow
	if err := json.Unmarshal([]byte(raw), &windows); err != nil {
		return false, ErrAutomationDisabled
	}
	local := time.UnixMilli(now).In(location)
	weekday, minute := int(local.Weekday()), local.Hour()*60+local.Minute()
	for _, window := range windows {
		if window.StartMinute < window.EndMinute {
			if weekday == window.Weekday && minute >= window.StartMinute && minute < window.EndMinute {
				return true, nil
			}
			continue
		}
		if (weekday == window.Weekday && minute >= window.StartMinute) ||
			(weekday == (window.Weekday+1)%7 && minute < window.EndMinute) {
			return true, nil
		}
	}
	return false, nil
}

func automationAttemptLimitsTx(tx *sql.Tx, principalID string, orbitID, now int64) (time.Duration, error) {
	minute := int64(time.Minute / time.Millisecond)
	hour := int64(time.Hour / time.Millisecond)
	var principalCount, orbitCount int
	if err := tx.QueryRow(`SELECT
  (SELECT COUNT(*) FROM automation_runtime_attempts
    WHERE principal_id = ? AND attempted_at > ? AND outcome <> 'reserved'),
  (SELECT COUNT(*) FROM automation_runtime_attempts
    WHERE owner_orbit_id = ? AND attempted_at > ? AND outcome <> 'reserved')`, principalID, now-minute,
		orbitID, now-hour).Scan(&principalCount, &orbitCount); err != nil {
		return 0, err
	}
	if principalCount >= automationcontract.MaxAcceptedPerMinute {
		var oldest int64
		if err := tx.QueryRow(`SELECT attempted_at FROM automation_runtime_attempts
WHERE principal_id = ? AND attempted_at > ? ORDER BY attempted_at, id LIMIT 1`,
			principalID, now-minute).Scan(&oldest); err != nil {
			return 0, err
		}
		return time.Duration(maxInt64(1, oldest+minute-now)) * time.Millisecond, nil
	}
	if orbitCount >= automationcontract.MaxAcceptedPerOrbitHour {
		var oldest int64
		if err := tx.QueryRow(`SELECT attempted_at FROM automation_runtime_attempts
WHERE owner_orbit_id = ? AND attempted_at > ? ORDER BY attempted_at, id LIMIT 1`,
			orbitID, now-hour).Scan(&oldest); err != nil {
			return 0, err
		}
		return time.Duration(maxInt64(1, oldest+hour-now)) * time.Millisecond, nil
	}
	return 0, nil
}

func automationConcurrentTx(tx *sql.Tx, principalID string, orbitID int64) (bool, error) {
	var principalCount, orbitCount int
	err := tx.QueryRow(`SELECT
  (SELECT COUNT(*) FROM automation_executions e
    LEFT JOIN transmissions t ON t.id = e.transmission_id
    WHERE e.principal_id = ? AND e.status IN ('claimed','leased','accepted','cancelling')
      AND (e.transmission_id IS NULL OR t.completed_at = 0)),
  (SELECT COUNT(*) FROM automation_executions e
    LEFT JOIN transmissions t ON t.id = e.transmission_id
    WHERE e.owner_orbit_id = ? AND e.status IN ('claimed','leased','accepted','cancelling')
      AND (e.transmission_id IS NULL OR t.completed_at = 0))`, principalID, orbitID).Scan(
		&principalCount, &orbitCount)
	return principalCount >= automationcontract.MaxConcurrentPerPrincipal ||
		orbitCount >= automationcontract.MaxConcurrentPerOrbit, err
}

func loadAutomationReferenceScopesTx(tx *sql.Tx, principal AutomationPrincipal, references []string, now int64) ([]AutomationTargetScope, error) {
	if len(references) == 0 || len(references) > principal.MaxTargetCount {
		return nil, ErrInsufficientCapability
	}
	seen := make(map[string]struct{}, len(references))
	result := make([]AutomationTargetScope, 0, len(references))
	for _, reference := range references {
		if !transmissionTargetReferencePattern.MatchString(reference) {
			return nil, ErrAutomationInvalid
		}
		var scope AutomationTargetScope
		err := tx.QueryRow(`SELECT target_kind, target_orbit_id, target_actor_id,
       target_slot, target_binding_paired_at
FROM transmission_target_references
WHERE reference_hash = ? AND actor_id = ? AND expires_at > ?`, hashToken(reference),
			principal.IssuedByActorID, now).Scan(&scope.Kind, &scope.OrbitID,
			&scope.ActorID, &scope.Slot, &scope.BindingPairedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrInsufficientCapability
		}
		if err != nil {
			return nil, err
		}
		scope.Digest = automationTargetScopeDigest(scope)
		if !containsString(principal.TargetRefDigests, scope.Digest) {
			return nil, ErrInsufficientCapability
		}
		if _, ok := seen[scope.Digest]; ok {
			continue
		}
		seen[scope.Digest] = struct{}{}
		result = append(result, scope)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Digest < result[j].Digest })
	return result, nil
}

func automationResolvedScopesTx(tx *sql.Tx, scopes []AutomationTargetScope, allowed map[int64]struct{}) ([]resolvedTransmissionTarget, error) {
	resolved := make(map[string]resolvedTransmissionTarget)
	for _, scope := range scopes {
		if _, ok := allowed[scope.OrbitID]; !ok {
			return nil, ErrAutomationAudienceNotAllowed
		}
		targets, err := liveTransmissionTargetsTx(tx, scope.OrbitID, scope.Slot)
		if err != nil {
			return nil, err
		}
		if scope.Kind == TransmissionSelectorPulsar {
			if len(targets) != 1 || targets[0].ActorID != scope.ActorID ||
				targets[0].BindingPairedAt != scope.BindingPairedAt {
				return nil, ErrAutomationAudienceNotAllowed
			}
		} else if scope.Kind != TransmissionSelectorBarycenter || len(targets) == 0 {
			return nil, ErrAutomationAudienceNotAllowed
		}
		for _, target := range targets {
			target.Reference = scope.Digest
			resolved[transmissionTargetKey(target.OrbitID, target.Slot)] = target
		}
	}
	result := make([]resolvedTransmissionTarget, 0, len(resolved))
	for _, target := range resolved {
		result = append(result, target)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].OrbitID == result[j].OrbitID {
			return result[i].Slot < result[j].Slot
		}
		return result[i].OrbitID < result[j].OrbitID
	})
	return result, nil
}

func automationAudienceTx(tx *sql.Tx, ctx ActorContext, audience automationcontract.AudienceKind,
	boundAirID string, scopes []AutomationTargetScope) ([]resolvedTransmissionTarget, PlaybackDomainKind,
	int64, *airPolicyContext, error) {
	policy, err := activeAirPolicyContextTx(tx, ctx.OrbitID)
	if err != nil {
		return nil, "", 0, nil, err
	}
	domainKind, domainID, allowed, err := transmissionDomainTx(tx, ctx.OrbitID, policy)
	if err != nil {
		return nil, "", 0, nil, err
	}
	var resolved []resolvedTransmissionTarget
	switch audience {
	case automationcontract.AudienceOwnBarycenter:
		resolved, err = liveTransmissionTargetsTx(tx, ctx.OrbitID, "")
	case automationcontract.AudienceCurrentAir:
		if policy == nil || policy.AirID != boundAirID || policy.AirStatus != "active" {
			return nil, "", 0, nil, ErrAutomationAudienceNotAllowed
		}
		orbits := make([]int64, 0, len(allowed))
		for orbitID := range allowed {
			orbits = append(orbits, orbitID)
		}
		sort.Slice(orbits, func(i, j int) bool { return orbits[i] < orbits[j] })
		for _, orbitID := range orbits {
			items, loadErr := liveTransmissionTargetsTx(tx, orbitID, "")
			if loadErr != nil {
				return nil, "", 0, nil, loadErr
			}
			resolved = append(resolved, items...)
		}
	case automationcontract.AudienceExplicit:
		resolved, err = automationResolvedScopesTx(tx, scopes, allowed)
	default:
		return nil, "", 0, nil, ErrAutomationInvalid
	}
	if err != nil {
		return nil, "", 0, nil, err
	}
	if len(resolved) == 0 {
		return nil, "", 0, nil, ErrTransmissionAudienceEmpty
	}
	return resolved, domainKind, domainID, policy, nil
}

func insertAutomationAttemptTx(tx *sql.Tx, principal AutomationPrincipal, cueID,
	idempotencyDigest, requestDigest string, now int64) (int64, error) {
	result, err := tx.Exec(`INSERT INTO automation_runtime_attempts(
  trigger_kind, owner_orbit_id, principal_id, cue_id, idempotency_digest,
  request_digest, attempted_at, outcome, retention_expires_at
) VALUES('scoped_api', ?, ?, ?, ?, ?, ?, 'reserved', ?)`, principal.OwnerOrbitID,
		principal.ID, cueID, idempotencyDigest, requestDigest, now,
		now+AutomationExecutionRetention.Milliseconds())
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

func denyAutomationAttemptTx(tx *sql.Tx, attemptID int64, reason automationcontract.DenialReason,
	retry time.Duration) error {
	_, err := tx.Exec(`UPDATE automation_runtime_attempts SET outcome = 'denied',
reason_code = ?, retry_after_ms = ? WHERE id = ? AND outcome = 'reserved'`, reason,
		retry.Milliseconds(), attemptID)
	return err
}

func automationRuntimeCueTx(tx *sql.Tx, cueID string, orbitID int64, now int64) (SavedCue, MediaItem, error) {
	cue, err := savedCueOwnedActiveTx(tx, cueID, orbitID)
	if err != nil {
		return SavedCue{}, MediaItem{}, err
	}
	mediaID := cue.MediaID
	if cue.SourceKind == SavedCueSourceBuiltin {
		if err := tx.QueryRow(`SELECT media_id FROM automation_builtin_media
WHERE owner_orbit_id = ?`, orbitID).Scan(&mediaID); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				return SavedCue{}, MediaItem{}, ErrAutomationCueNotReady
			}
			return SavedCue{}, MediaItem{}, err
		}
	}
	mediaItem, err := scanMediaItem(tx.QueryRow(`SELECT `+mediaItemColumns+`
FROM media_items WHERE id = ? AND owner_orbit_id = ?`, mediaID, orbitID))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (mediaItem.Status != MediaStatusReady ||
		((cue.SourceKind == SavedCueSourceMedia && (mediaItem.Kind != MediaKindAudioClip || mediaItem.Source != MediaSourceApp)) ||
			(cue.SourceKind == SavedCueSourceBuiltin && (mediaItem.Kind != MediaKindBuiltinCue || mediaItem.Source != MediaSourceSystem ||
				mediaItem.SHA256 != BuiltinRecordingCueSHA256))) ||
		mediaItem.ExpiresAt <= now)) {
		return SavedCue{}, MediaItem{}, ErrAutomationCueNotReady
	}
	return cue, mediaItem, err
}

func automationTransmissionOrigin(mediaItem MediaItem) TransmissionOriginKind {
	if mediaItem.Kind == MediaKindBuiltinCue && mediaItem.Source == MediaSourceSystem {
		return TransmissionOriginBuiltin
	}
	return TransmissionOriginFile
}

func AutomationBuiltinStorageKey(ownerOrbitID int64) string {
	return "media/v1/" + hashToken(fmt.Sprintf("barycenter/automation-builtin-media/v1:%d", ownerOrbitID))
}

// EnsureAutomationBuiltinMedia publishes the deterministic system cue into
// the generic media model after the adapter has atomically materialized its
// exact bytes at AutomationBuiltinStorageKey. The principal, cue scope,
// feature switch and content policy are rechecked in this writer transaction.
func (s *Store) EnsureAutomationBuiltinMedia(secret, cueID string, now int64) (MediaItem, error) {
	if now <= 0 || !savedCueIDPattern.MatchString(cueID) {
		return MediaItem{}, ErrAutomationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return MediaItem{}, err
	}
	defer tx.Rollback()
	principal, err := resolveAutomationPrincipalSecretTx(tx, secret, now)
	if err != nil {
		return MediaItem{}, err
	}
	if !containsString(principal.AllowedCueIDs, cueID) {
		return MediaItem{}, ErrInsufficientCapability
	}
	cue, err := savedCueOwnedActiveTx(tx, cueID, principal.OwnerOrbitID)
	if err != nil {
		return MediaItem{}, err
	}
	if cue.SourceKind != SavedCueSourceBuiltin || cue.BuiltinAssetID != BuiltinRecordingCueAssetID ||
		cue.BuiltinSHA256 != BuiltinRecordingCueSHA256 {
		return MediaItem{}, ErrAutomationCueNotReady
	}
	ctx, err := automationRuntimeActorTx(tx, principal.IssuedByActorID, principal.OwnerOrbitID)
	if err != nil {
		return MediaItem{}, err
	}
	if err := requireCurrentContentPolicyTx(tx, ctx); err != nil {
		return MediaItem{}, ErrAutomationDisabled
	}
	feature, err := scanAutomationFeatureState(tx.QueryRow(`SELECT `+automationFeatureColumns+`
FROM automation_feature_state WHERE owner_orbit_id = ?`, principal.OwnerOrbitID))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (!feature.AutomationEnabled || feature.EmergencyDisabled)) {
		return MediaItem{}, ErrAutomationDisabled
	}
	if err != nil {
		return MediaItem{}, err
	}
	item, err := ensureAutomationBuiltinMediaTx(tx, principal.OwnerOrbitID,
		principal.IssuedByActorID, now)
	if err != nil {
		return MediaItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaItem{}, err
	}
	return item, nil
}

func ensureAutomationBuiltinMediaTx(tx *sql.Tx, ownerOrbitID, actorID, now int64) (MediaItem, error) {
	var mediaID string
	err := tx.QueryRow(`SELECT media_id FROM automation_builtin_media
WHERE owner_orbit_id = ?`, ownerOrbitID).Scan(&mediaID)
	if err == nil {
		return scanMediaItem(tx.QueryRow(`SELECT `+mediaItemColumns+`
FROM media_items WHERE id = ?`, mediaID))
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return MediaItem{}, err
	}
	mediaID = ulid.NewMediaID(time.UnixMilli(now))
	storageKey := AutomationBuiltinStorageKey(ownerOrbitID)
	expiresAt := now + (10 * 365 * 24 * time.Hour).Milliseconds()
	_, err = tx.Exec(`INSERT INTO media_items(
  id, owner_orbit_id, actor_id, kind, source, title, mime, codec, duration_ms,
  size_bytes, sha256, storage_key, status, created_at, updated_at, expires_at, published_at
) VALUES(?, ?, ?, 'builtin_cue', 'system', 'Pulsar recording cue', 'audio/wav',
  'pcm_s16le', ?, ?, ?, ?, 'ready', ?, ?, ?, ?)`, mediaID, ownerOrbitID,
		actorID, BuiltinRecordingCueDuration, BuiltinRecordingCueBytes,
		BuiltinRecordingCueSHA256, storageKey, now, now, expiresAt, now)
	if err != nil {
		return MediaItem{}, err
	}
	// owner_orbit_id is also the singleton key preventing duplicate system
	// media when scoped automation and manual soundboard race.
	if _, err := tx.Exec(`INSERT INTO automation_builtin_media(
owner_orbit_id, media_id, storage_key, created_at, updated_at)
VALUES(?, ?, ?, ?, ?)`, ownerOrbitID, mediaID, storageKey, now, now); err != nil {
		return MediaItem{}, err
	}
	item, err := scanMediaItem(tx.QueryRow(`SELECT `+mediaItemColumns+`
FROM media_items WHERE id = ?`, mediaID))
	if err != nil {
		return MediaItem{}, err
	}
	return item, nil
}

func (s *Store) EnsureAuthorizedAutomationBuiltinMedia(expectedActorID int64, bearer, cueID string, now int64) (MediaItem, error) {
	return s.ensureAuthorizedAutomationBuiltinMedia(expectedActorID, bearer, Identity{}, cueID, now)
}

func (s *Store) EnsureAuthorizedAutomationBuiltinMediaForIdentity(expectedActorID int64, identity Identity, cueID string, now int64) (MediaItem, error) {
	return s.ensureAuthorizedAutomationBuiltinMedia(expectedActorID, "", identity, cueID, now)
}

func (s *Store) ensureAuthorizedAutomationBuiltinMedia(expectedActorID int64, bearer string, identity Identity, cueID string, now int64) (MediaItem, error) {
	if expectedActorID <= 0 || now <= 0 || !savedCueIDPattern.MatchString(cueID) {
		return MediaItem{}, ErrAutomationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return MediaItem{}, err
	}
	defer tx.Rollback()
	ctx, _, err := authorizeTransmissionProofTx(tx, expectedActorID, bearer, identity)
	if err != nil {
		return MediaItem{}, err
	}
	if ctx.Role != "primary" || (!ctx.Capabilities.Has(CapabilityControl) &&
		!ctx.Capabilities.Has(CapabilityTelegram)) {
		return MediaItem{}, ErrInsufficientCapability
	}
	feature, err := scanAutomationFeatureState(tx.QueryRow(`SELECT `+automationFeatureColumns+`
FROM automation_feature_state WHERE owner_orbit_id = ?`, ctx.OrbitID))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && !feature.SoundboardEnabled) {
		return MediaItem{}, ErrAutomationDisabled
	}
	if err != nil {
		return MediaItem{}, err
	}
	cue, err := savedCueOwnedActiveTx(tx, cueID, ctx.OrbitID)
	if err != nil || cue.SourceKind != SavedCueSourceBuiltin ||
		cue.BuiltinAssetID != BuiltinRecordingCueAssetID || cue.BuiltinSHA256 != BuiltinRecordingCueSHA256 {
		if err != nil {
			return MediaItem{}, err
		}
		return MediaItem{}, ErrAutomationCueNotReady
	}
	item, err := ensureAutomationBuiltinMediaTx(tx, ctx.OrbitID, ctx.ActorID, now)
	if err != nil {
		return MediaItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaItem{}, err
	}
	return item, nil
}

func (s *Store) TriggerAutomationRuntime(params AutomationRuntimeTriggerParams) (AutomationRuntimeResult, error) {
	if params.AttemptedAt <= 0 || !savedCueIDPattern.MatchString(params.CueID) ||
		!validAutomationIdempotencyKey(params.IdempotencyKey) ||
		!lowerHexTokenPattern.MatchString(params.RequestDigest) {
		return AutomationRuntimeResult{}, ErrAutomationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AutomationRuntimeResult{}, err
	}
	defer tx.Rollback()
	principal, err := resolveAutomationPrincipalSecretTx(tx, params.Secret, params.AttemptedAt)
	if err != nil {
		return AutomationRuntimeResult{}, err
	}
	idempotencyDigest := hashToken("barycenter/automation-idempotency/v1:" + params.IdempotencyKey)
	if existing, loadErr := scanAutomationAttempt(tx.QueryRow(`SELECT id, request_digest,
outcome, reason_code, retry_after_ms, COALESCE(execution_id, '')
FROM automation_runtime_attempts WHERE principal_id = ? AND idempotency_digest = ?`,
		principal.ID, idempotencyDigest)); loadErr == nil {
		if existing.RequestDigest != params.RequestDigest {
			if _, err := tx.Exec(`INSERT INTO automation_audit_events(
  event_kind, operation, owner_orbit_id, actor_id, principal_id,
  principal_label, cue_id, cue_label, trigger_kind, outcome, reason_code,
  terminal_at, created_at
) VALUES('trigger', 'automation.trigger.scoped_api.v1', ?, ?, ?, ?, ?,
  COALESCE((SELECT title FROM saved_cues WHERE id = ?), ''), 'scoped_api',
  'denied', 'idempotency_conflict', ?, ?)`, principal.OwnerOrbitID,
				principal.IssuedByActorID, principal.ID, principal.DisplayName,
				params.CueID, params.CueID, params.AttemptedAt, params.AttemptedAt); err != nil {
				return AutomationRuntimeResult{}, err
			}
			if err := tx.Commit(); err != nil {
				return AutomationRuntimeResult{}, err
			}
			return AutomationRuntimeResult{}, ErrAutomationIdempotencyConflict
		}
		if existing.Outcome == "denied" {
			return AutomationRuntimeResult{}, automationRuntimeError(existing.ReasonCode, existing.RetryAfterMS)
		}
		execution, loadErr := scanAutomationExecution(tx.QueryRow(`SELECT `+automationExecutionColumns+`
FROM automation_executions WHERE id = ?`, existing.ExecutionID))
		if loadErr != nil {
			return AutomationRuntimeResult{}, loadErr
		}
		creation, loadErr := loadTransmissionCreationTx(tx, execution.TransmissionID)
		if loadErr != nil {
			return AutomationRuntimeResult{}, loadErr
		}
		if err := tx.Commit(); err != nil {
			return AutomationRuntimeResult{}, err
		}
		return AutomationRuntimeResult{Execution: execution, Transmission: creation, Replayed: true}, nil
	} else if !errors.Is(loadErr, sql.ErrNoRows) {
		return AutomationRuntimeResult{}, loadErr
	}
	attemptID, err := insertAutomationAttemptTx(tx, principal, params.CueID,
		idempotencyDigest, params.RequestDigest, params.AttemptedAt)
	if err != nil {
		return AutomationRuntimeResult{}, err
	}
	deny := func(reason automationcontract.DenialReason, cause error, retry time.Duration) (AutomationRuntimeResult, error) {
		if err := denyAutomationAttemptTx(tx, attemptID, reason, retry); err != nil {
			return AutomationRuntimeResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return AutomationRuntimeResult{}, err
		}
		return AutomationRuntimeResult{}, cause
	}
	feature, err := scanAutomationFeatureState(tx.QueryRow(`SELECT `+automationFeatureColumns+`
FROM automation_feature_state WHERE owner_orbit_id = ?`, principal.OwnerOrbitID))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (!feature.AutomationEnabled || feature.EmergencyDisabled)) {
		return deny(automationcontract.DenyAutomationDisabled, ErrAutomationDisabled, 0)
	}
	if err != nil {
		return AutomationRuntimeResult{}, err
	}
	if !AutomationQuietPolicyValid(feature.Timezone, feature.QuietHoursJSON, feature.QuietHoursHash) {
		return deny(automationcontract.DenyAutomationDisabled, ErrAutomationDisabled, 0)
	}
	retry, err := automationAttemptLimitsTx(tx, principal.ID, principal.OwnerOrbitID, params.AttemptedAt)
	if err != nil {
		return AutomationRuntimeResult{}, err
	}
	if retry > 0 {
		return deny(automationcontract.DenyTooManyAttempts,
			&AutomationRateLimitError{RetryAfter: retry}, retry)
	}
	limited, err := automationConcurrentTx(tx, principal.ID, principal.OwnerOrbitID)
	if err != nil {
		return AutomationRuntimeResult{}, err
	}
	if limited {
		return deny(automationcontract.DenyExecutionInProgress,
			ErrAutomationExecutionInProgress, 0)
	}
	if !containsString(principal.AllowedCueIDs, params.CueID) ||
		!containsAudience(principal.AllowedAudiences, params.AudienceKind) {
		return deny(automationcontract.DenyInsufficientScope, ErrInsufficientCapability, 0)
	}
	quiet, err := automationQuietAt(feature.Timezone, feature.QuietHoursJSON, params.AttemptedAt)
	if err != nil {
		return AutomationRuntimeResult{}, err
	}
	if quiet {
		return deny(automationcontract.DenyQuietHours, ErrAutomationQuietHours, 0)
	}
	var scopes []AutomationTargetScope
	if params.AudienceKind == automationcontract.AudienceExplicit {
		scopes, err = loadAutomationReferenceScopesTx(tx, principal, params.TargetReferences, params.AttemptedAt)
		if err != nil {
			return deny(automationcontract.DenyInsufficientScope, ErrInsufficientCapability, 0)
		}
	} else if len(params.TargetReferences) != 0 {
		return deny(automationcontract.DenyAudienceNotAllowed, ErrAutomationAudienceNotAllowed, 0)
	}
	ctx, err := automationRuntimeActorTx(tx, principal.IssuedByActorID, principal.OwnerOrbitID)
	if err != nil {
		return deny(automationcontract.DenyAutomationDisabled, ErrAutomationDisabled, 0)
	}
	if err := requireCurrentContentPolicyTx(tx, ctx); err != nil {
		return deny(automationcontract.DenyAutomationDisabled, ErrAutomationDisabled, 0)
	}
	cue, mediaItem, err := automationRuntimeCueTx(tx, params.CueID, principal.OwnerOrbitID, params.AttemptedAt)
	if err != nil {
		return deny(automationcontract.DenyCueNotReady, ErrAutomationCueNotReady, 0)
	}
	resolved, domainKind, domainID, policy, err := automationAudienceTx(tx, ctx,
		params.AudienceKind, principal.BoundAirID, scopes)
	if err != nil {
		return deny(automationcontract.DenyAudienceNotAllowed, err, 0)
	}
	transmissionParams := CreateResolvedTransmissionParams{
		AudienceKind: TransmissionAudienceKind(params.AudienceKind),
		OriginKind:   automationTransmissionOrigin(mediaItem), IncludeOrigin: true,
		RequestedDelivery: TransmissionDeliveryOverlay, AcceptedAt: params.AttemptedAt,
		PolicyAt: params.AttemptedAt, Availability: params.Availability,
	}
	targets, missingOverlay, _, unsupported, err := evaluateTransmissionTargetsTx(
		tx, ctx, mediaItem, resolved, transmissionParams)
	if err != nil {
		return AutomationRuntimeResult{}, err
	}
	acceptedTargets := 0
	for _, target := range targets {
		if target.Status == TransmissionTargetAccepted {
			acceptedTargets++
		}
	}
	if acceptedTargets == 0 {
		return deny(automationcontract.DenyAudienceNotAllowed, ErrTransmissionAudienceEmpty, 0)
	}
	if missingOverlay || len(unsupported) != 0 {
		return deny(automationcontract.DenyDeliveryCapabilityMissing, ErrAutomationCapabilityMissing, 0)
	}
	policyContext := policy
	authorization, err := authorizeAirPolicyTx(ctx, policyContext,
		airPolicyOperationForDelivery(TransmissionDeliveryOverlay))
	if err != nil {
		return deny(automationcontract.DenyAirPolicy, ErrAutomationAudienceNotAllowed, 0)
	}
	create := CreateTransmissionParams{
		MediaID: mediaItem.ID, SourceOrbitID: ctx.OrbitID, SourceActorID: ctx.ActorID,
		SourceSlot: ctx.Slot, PlaybackDomainKind: domainKind, PlaybackDomainID: domainID,
		AudienceKind: TransmissionAudienceKind(params.AudienceKind),
		OriginKind:   automationTransmissionOrigin(mediaItem), IncludeOrigin: true,
		RequestedDelivery: TransmissionDeliveryOverlay,
		EffectiveDelivery: TransmissionDeliveryOverlay, AcceptedAt: params.AttemptedAt,
		Targets: targets, AirID: authorization.AirID,
		AirPolicyRevision:  authorization.PolicyRevision,
		AirPolicyOperation: authorization.Operation, AirPolicyResult: authorization.Result,
	}
	creation, err := s.createTransmissionTx(tx, create, mediaItem)
	if err != nil {
		return AutomationRuntimeResult{}, err
	}
	executionID := ulid.NewAutomationExecutionID(time.UnixMilli(params.AttemptedAt))
	selectorDigest := ""
	if len(scopes) > 0 {
		selectorDigest = automationTargetSetDigest(scopes)
	}
	_, err = tx.Exec(`INSERT INTO automation_executions(
  id, trigger_kind, owner_orbit_id, principal_id, issued_by_actor_id, cue_id,
  cue_revision, cue_source_generation, media_identity, audience_kind,
  selector_digest, bound_air_id, target_snapshot_digest, resolved_target_count,
  delivery, idempotency_digest, request_digest, feature_revision, policy_revision,
  claimed_at, transmission_id, status, outcome, retention_expires_at
) VALUES(?, 'scoped_api', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'overlay', ?, ?,
  ?, ?, ?, ?, 'accepted', 'accepted', ?)`, executionID, principal.OwnerOrbitID,
		principal.ID, principal.IssuedByActorID, cue.ID, cue.Revision,
		cue.SourceGeneration, savedCueMediaIdentity(cue), params.AudienceKind,
		selectorDigest, principal.BoundAirID, automationTargetSnapshotDigest(creation.Targets),
		len(targets), idempotencyDigest, params.RequestDigest, feature.Revision,
		feature.Revision, params.AttemptedAt, creation.Transmission.ID,
		params.AttemptedAt+AutomationExecutionRetention.Milliseconds())
	if err != nil {
		return AutomationRuntimeResult{}, err
	}
	if _, err := tx.Exec(`UPDATE automation_runtime_attempts SET outcome = 'accepted',
execution_id = ? WHERE id = ? AND outcome = 'reserved'`, executionID, attemptID); err != nil {
		return AutomationRuntimeResult{}, err
	}
	execution, err := scanAutomationExecution(tx.QueryRow(`SELECT `+automationExecutionColumns+`
FROM automation_executions WHERE id = ?`, executionID))
	if err != nil {
		return AutomationRuntimeResult{}, err
	}
	if err := s.checkpoint("automation_runtime_trigger_before_commit"); err != nil {
		return AutomationRuntimeResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return AutomationRuntimeResult{}, err
	}
	return AutomationRuntimeResult{Execution: execution, Transmission: creation}, nil
}

// DueAutomationOccurrences returns only schedules whose canonical local
// minute is the current UTC minute. It deliberately never scans backwards, so
// downtime and wall-clock jumps cannot create catch-up bursts.
func (s *Store) DueAutomationOccurrences(now int64, limit int) ([]AutomationDueOccurrence, error) {
	if now <= 0 || limit <= 0 || limit > 1000 {
		return nil, ErrAutomationInvalid
	}
	rows, err := s.db.Query(`SELECT `+automationScheduleColumns+`
FROM automation_schedules s
WHERE s.enabled = 1 AND NOT EXISTS(
  SELECT 1 FROM automation_schedule_controls c
  WHERE c.schedule_id = s.id AND c.deleted_at > 0)
ORDER BY s.updated_at, s.id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	currentUTC := now - now%int64(time.Minute/time.Millisecond)
	var result []AutomationDueOccurrence
	for rows.Next() {
		schedule, err := scanAutomationSchedule(rows)
		if err != nil {
			return nil, err
		}
		if _, err := canonicalScheduledMinute(schedule, currentUTC, now); err == nil {
			result = append(result, AutomationDueOccurrence{
				ScheduleID: schedule.ID, ScheduleRevision: schedule.Revision,
				ScheduledUTC: currentUTC,
			})
		} else if !errors.Is(err, ErrAutomationOccurrenceNotCurrent) &&
			!errors.Is(err, ErrAutomationOccurrenceLaterFold) {
			return nil, err
		}
	}
	return result, rows.Err()
}

func loadAutomationScheduleScopesTx(tx *sql.Tx, scheduleID string) ([]AutomationTargetScope, error) {
	rows, err := tx.Query(`SELECT target_digest, target_kind, target_orbit_id,
target_actor_id, target_slot, target_binding_paired_at
FROM automation_schedule_targets WHERE schedule_id = ? ORDER BY target_digest`, scheduleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []AutomationTargetScope
	for rows.Next() {
		var scope AutomationTargetScope
		if err := rows.Scan(&scope.Digest, &scope.Kind, &scope.OrbitID,
			&scope.ActorID, &scope.Slot, &scope.BindingPairedAt); err != nil {
			return nil, err
		}
		result = append(result, scope)
	}
	return result, rows.Err()
}

func (s *Store) dispatchScheduledAutomationRuntime(executionID string,
	availability []TransmissionTargetAvailability, now int64) (AutomationRuntimeResult, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return AutomationRuntimeResult{}, err
	}
	defer tx.Rollback()
	execution, err := scanAutomationExecution(tx.QueryRow(`SELECT `+automationExecutionColumns+`
FROM automation_executions WHERE id = ?`, executionID))
	if err != nil {
		return AutomationRuntimeResult{}, err
	}
	if execution.Status == "accepted" && execution.TransmissionID != "" {
		creation, err := loadTransmissionCreationTx(tx, execution.TransmissionID)
		if err != nil {
			return AutomationRuntimeResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return AutomationRuntimeResult{}, err
		}
		return AutomationRuntimeResult{Execution: execution, Transmission: creation, Replayed: true}, nil
	}
	if execution.Status != "claimed" {
		return AutomationRuntimeResult{}, automationRuntimeError(execution.ReasonCode, 0)
	}
	schedule, err := scanAutomationSchedule(tx.QueryRow(`SELECT `+automationScheduleColumns+`
FROM automation_schedules WHERE id = ?`, execution.ScheduleID))
	if err != nil {
		return AutomationRuntimeResult{}, err
	}
	occurrence := execution.OccurrenceKey
	if existing, loadErr := scanAutomationAttempt(tx.QueryRow(`SELECT id, request_digest,
outcome, reason_code, retry_after_ms, COALESCE(execution_id, '')
FROM automation_runtime_attempts WHERE schedule_id = ? AND schedule_revision = ?
  AND occurrence_key = ?`, schedule.ID, execution.ScheduleRevision, occurrence)); loadErr == nil {
		if existing.Outcome == "denied" {
			return AutomationRuntimeResult{}, automationRuntimeError(existing.ReasonCode, existing.RetryAfterMS)
		}
	} else if !errors.Is(loadErr, sql.ErrNoRows) {
		return AutomationRuntimeResult{}, loadErr
	}
	result, err := tx.Exec(`INSERT INTO automation_runtime_attempts(
  trigger_kind, owner_orbit_id, schedule_id, schedule_revision, cue_id,
  occurrence_key, attempted_at, outcome, retention_expires_at
) VALUES('schedule', ?, ?, ?, ?, ?, ?, 'reserved', ?)`, schedule.OwnerOrbitID,
		schedule.ID, execution.ScheduleRevision, schedule.CueID, occurrence, now,
		now+AutomationExecutionRetention.Milliseconds())
	if err != nil {
		return AutomationRuntimeResult{}, err
	}
	attemptID, err := result.LastInsertId()
	if err != nil {
		return AutomationRuntimeResult{}, err
	}
	deny := func(reason automationcontract.DenialReason, cause error, retry time.Duration) (AutomationRuntimeResult, error) {
		if err := denyAutomationAttemptTx(tx, attemptID, reason, retry); err != nil {
			return AutomationRuntimeResult{}, err
		}
		if _, err := tx.Exec(`UPDATE automation_executions SET status = 'denied',
outcome = 'denied', reason_code = ?, completed_at = ?, revision = revision + 1
WHERE id = ? AND status = 'claimed'`, reason, now, execution.ID); err != nil {
			return AutomationRuntimeResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return AutomationRuntimeResult{}, err
		}
		return AutomationRuntimeResult{}, cause
	}
	feature, err := scanAutomationFeatureState(tx.QueryRow(`SELECT `+automationFeatureColumns+`
FROM automation_feature_state WHERE owner_orbit_id = ?`, schedule.OwnerOrbitID))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (!feature.AutomationEnabled ||
		feature.EmergencyDisabled || feature.Revision != schedule.PolicyRevision || !schedule.Enabled ||
		schedule.Revision != execution.ScheduleRevision)) {
		return deny(automationcontract.DenyAutomationDisabled, ErrAutomationDisabled, 0)
	}
	if err != nil {
		return AutomationRuntimeResult{}, err
	}
	var orbitCount int
	hour := int64(time.Hour / time.Millisecond)
	if err := tx.QueryRow(`SELECT COUNT(*) FROM automation_runtime_attempts
WHERE owner_orbit_id = ? AND attempted_at > ?`, schedule.OwnerOrbitID, now-hour).Scan(&orbitCount); err != nil {
		return AutomationRuntimeResult{}, err
	}
	if orbitCount > automationcontract.MaxAcceptedPerOrbitHour {
		return deny(automationcontract.DenyTooManyAttempts,
			&AutomationRateLimitError{RetryAfter: time.Minute}, time.Minute)
	}
	limited, err := automationConcurrentTx(tx, "", schedule.OwnerOrbitID)
	if err != nil {
		return AutomationRuntimeResult{}, err
	}
	if limited {
		return deny(automationcontract.DenyExecutionInProgress, ErrAutomationExecutionInProgress, 0)
	}
	quiet, err := automationQuietAt(feature.Timezone, feature.QuietHoursJSON, now)
	if err != nil {
		return AutomationRuntimeResult{}, err
	}
	var additionalRaw string
	if err := tx.QueryRow(`SELECT additional_quiet_hours_json
FROM automation_schedule_controls WHERE schedule_id = ? AND deleted_at = 0`,
		schedule.ID).Scan(&additionalRaw); err != nil {
		return AutomationRuntimeResult{}, err
	}
	additionalQuiet, err := automationQuietAt(schedule.Timezone, additionalRaw, now)
	if err != nil {
		return AutomationRuntimeResult{}, err
	}
	if quiet || additionalQuiet {
		return deny(automationcontract.DenyQuietHours, ErrAutomationQuietHours, 0)
	}
	ctx, err := automationRuntimeActorTx(tx, schedule.CreatedByActorID, schedule.OwnerOrbitID)
	if err != nil {
		return deny(automationcontract.DenyAutomationDisabled, ErrAutomationDisabled, 0)
	}
	if err := requireCurrentContentPolicyTx(tx, ctx); err != nil {
		return deny(automationcontract.DenyAutomationDisabled, ErrAutomationDisabled, 0)
	}
	_, mediaItem, err := automationRuntimeCueTx(tx, schedule.CueID, schedule.OwnerOrbitID, now)
	if err != nil {
		return deny(automationcontract.DenyCueNotReady, ErrAutomationCueNotReady, 0)
	}
	scopes, err := loadAutomationScheduleScopesTx(tx, schedule.ID)
	if err != nil {
		return AutomationRuntimeResult{}, err
	}
	resolved, domainKind, domainID, policy, err := automationAudienceTx(tx, ctx,
		schedule.AudienceKind, schedule.BoundAirID, scopes)
	if err != nil {
		return deny(automationcontract.DenyAudienceNotAllowed, err, 0)
	}
	transmissionParams := CreateResolvedTransmissionParams{
		AudienceKind: TransmissionAudienceKind(schedule.AudienceKind),
		OriginKind:   automationTransmissionOrigin(mediaItem), IncludeOrigin: true,
		RequestedDelivery: TransmissionDeliveryOverlay, AcceptedAt: now,
		PolicyAt: now, Availability: availability,
	}
	targets, missingOverlay, _, unsupported, err := evaluateTransmissionTargetsTx(
		tx, ctx, mediaItem, resolved, transmissionParams)
	if err != nil {
		return AutomationRuntimeResult{}, err
	}
	accepted := 0
	for _, target := range targets {
		if target.Status == TransmissionTargetAccepted {
			accepted++
		}
	}
	if accepted == 0 {
		return deny(automationcontract.DenyAudienceNotAllowed, ErrTransmissionAudienceEmpty, 0)
	}
	if missingOverlay || len(unsupported) != 0 {
		return deny(automationcontract.DenyDeliveryCapabilityMissing, ErrAutomationCapabilityMissing, 0)
	}
	authorization, err := authorizeAirPolicyTx(ctx, policy,
		airPolicyOperationForDelivery(TransmissionDeliveryOverlay))
	if err != nil {
		return deny(automationcontract.DenyAirPolicy, ErrAutomationAudienceNotAllowed, 0)
	}
	creation, err := s.createTransmissionTx(tx, CreateTransmissionParams{
		MediaID: mediaItem.ID, SourceOrbitID: ctx.OrbitID, SourceActorID: ctx.ActorID,
		SourceSlot: ctx.Slot, PlaybackDomainKind: domainKind, PlaybackDomainID: domainID,
		AudienceKind: TransmissionAudienceKind(schedule.AudienceKind),
		OriginKind:   automationTransmissionOrigin(mediaItem), IncludeOrigin: true,
		RequestedDelivery: TransmissionDeliveryOverlay,
		EffectiveDelivery: TransmissionDeliveryOverlay, AcceptedAt: now, Targets: targets,
		AirID: authorization.AirID, AirPolicyRevision: authorization.PolicyRevision,
		AirPolicyOperation: authorization.Operation, AirPolicyResult: authorization.Result,
	}, mediaItem)
	if err != nil {
		return AutomationRuntimeResult{}, err
	}
	if _, err := tx.Exec(`UPDATE automation_executions SET transmission_id = ?,
target_snapshot_digest = ?, resolved_target_count = ?, status = 'accepted',
outcome = 'accepted', revision = revision + 1
WHERE id = ? AND status = 'claimed'`, creation.Transmission.ID,
		automationTargetSnapshotDigest(creation.Targets), len(targets), execution.ID); err != nil {
		return AutomationRuntimeResult{}, err
	}
	if _, err := tx.Exec(`UPDATE automation_runtime_attempts SET outcome = 'accepted',
execution_id = ? WHERE id = ? AND outcome = 'reserved'`, execution.ID, attemptID); err != nil {
		return AutomationRuntimeResult{}, err
	}
	execution, err = scanAutomationExecution(tx.QueryRow(`SELECT `+automationExecutionColumns+`
FROM automation_executions WHERE id = ?`, execution.ID))
	if err != nil {
		return AutomationRuntimeResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return AutomationRuntimeResult{}, err
	}
	return AutomationRuntimeResult{Execution: execution, Transmission: creation}, nil
}

func (s *Store) RunDueAutomationRuntime(availability []TransmissionTargetAvailability,
	now int64, limit int) ([]AutomationRuntimeResult, error) {
	due, err := s.DueAutomationOccurrences(now, limit)
	if err != nil {
		return nil, err
	}
	results := make([]AutomationRuntimeResult, 0, len(due))
	for _, occurrence := range due {
		execution, _, err := s.ClaimScheduledAutomationOccurrence(occurrence.ScheduleID,
			occurrence.ScheduleRevision, occurrence.ScheduledUTC, now)
		if err != nil {
			if errors.Is(err, ErrAutomationDisabled) || errors.Is(err, ErrAutomationOccurrenceNotCurrent) ||
				errors.Is(err, ErrAutomationOccurrenceLaterFold) || errors.Is(err, ErrAutomationStateConflict) {
				continue
			}
			return results, err
		}
		result, err := s.dispatchScheduledAutomationRuntime(execution.ID, availability, now)
		if err != nil {
			if errors.Is(err, ErrAutomationDisabled) || errors.Is(err, ErrAutomationRateLimited) ||
				errors.Is(err, ErrAutomationExecutionInProgress) || errors.Is(err, ErrAutomationQuietHours) ||
				errors.Is(err, ErrAutomationCueNotReady) || errors.Is(err, ErrAutomationAudienceNotAllowed) ||
				errors.Is(err, ErrAutomationCapabilityMissing) || errors.Is(err, ErrTransmissionAudienceEmpty) {
				continue
			}
			return results, err
		}
		if !result.Replayed {
			results = append(results, result)
		}
	}
	return results, nil
}

func (s *Store) PruneAutomationRuntimeAttempts(cutoff int64, limit int) (int64, error) {
	if cutoff <= 0 || limit <= 0 || limit > 1000 {
		return 0, ErrAutomationInvalid
	}
	result, err := s.db.Exec(`DELETE FROM automation_runtime_attempts WHERE id IN (
  SELECT id FROM automation_runtime_attempts WHERE retention_expires_at < ?
  ORDER BY retention_expires_at, id LIMIT ?
)`, cutoff, limit)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// ReconcileAutomationRuntimeClaims fails only pre-acceptance work from a past
// minute. Current-minute schedule claims remain resumable, while a crash can
// never leave an unlinked execution consuming concurrency forever.
func (s *Store) ReconcileAutomationRuntimeClaims(now int64) (int64, error) {
	if now <= 0 {
		return 0, ErrAutomationInvalid
	}
	currentMinute := now - now%int64(time.Minute/time.Millisecond)
	result, err := s.db.Exec(`UPDATE automation_executions
SET status = 'failed', outcome = 'failed', reason_code = 'runtime_restart_abandoned',
    lease_owner_hash = '', lease_expires_at = 0, completed_at = ?, revision = revision + 1
WHERE transmission_id IS NULL AND status IN ('claimed','leased') AND claimed_at < ?`,
		now, currentMinute)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

func (s *Store) AutomationRuntimeAttemptCount(ownerOrbitID int64) (int, error) {
	if ownerOrbitID <= 0 {
		return 0, ErrAutomationInvalid
	}
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM automation_runtime_attempts
WHERE owner_orbit_id = ?`, ownerOrbitID).Scan(&count)
	return count, err
}

// CancelInvalidAutomationRuntime rechecks the durable kill switches and
// revocations and cancels linked scheduler work in the same writer
// transaction. It is safe to call after every control mutation and on every
// scheduler tick; terminal work is ignored and repeated calls are idempotent.
func (s *Store) CancelInvalidAutomationRuntime(now int64, limit int) ([]CancelTransmissionResult, error) {
	if now <= 0 || limit <= 0 || limit > 1000 {
		return nil, ErrAutomationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT e.id, e.transmission_id,
CASE
  WHEN p.revoked_at > 0 OR p.disabled_at > 0 OR pa.revoked_at IS NOT NULL
    THEN 'principal_revoked'
  WHEN s.enabled = 0 OR sa.revoked_at IS NOT NULL THEN 'schedule_disabled'
  ELSE 'automation_disabled'
END
FROM automation_executions e
JOIN automation_feature_state f ON f.owner_orbit_id = e.owner_orbit_id
JOIN transmissions t ON t.id = e.transmission_id AND t.completed_at = 0
LEFT JOIN automation_principals p ON p.id = e.principal_id
LEFT JOIN automation_schedules s ON s.id = e.schedule_id
LEFT JOIN actors pa ON pa.id = p.issued_by_actor_id
LEFT JOIN actors sa ON sa.id = s.created_by_actor_id
WHERE e.transmission_id IS NOT NULL
  AND e.status IN ('accepted','cancelling')
  AND (f.automation_enabled = 0 OR f.emergency_disabled = 1
    OR p.revoked_at > 0 OR p.disabled_at > 0 OR pa.revoked_at IS NOT NULL
    OR s.enabled = 0 OR sa.revoked_at IS NOT NULL)
ORDER BY e.claimed_at, e.id LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	type candidate struct{ executionID, transmissionID, reason string }
	var candidates []candidate
	for rows.Next() {
		var value candidate
		if err := rows.Scan(&value.executionID, &value.transmissionID, &value.reason); err != nil {
			rows.Close()
			return nil, err
		}
		candidates = append(candidates, value)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	results := make([]CancelTransmissionResult, 0, len(candidates))
	for _, candidate := range candidates {
		work, err := loadTransmissionSchedulerWorkTx(tx, candidate.transmissionID)
		if err != nil {
			return nil, err
		}
		reason := TransmissionReason(candidate.reason)
		result, err := cancelTransmissionWorkTx(tx, work, reason, now, true,
			func(TransmissionTarget) bool { return true })
		if err != nil {
			return nil, err
		}
		status, completedAt := "cancelling", int64(0)
		if result.Transmission.CompletedAt != 0 {
			status, completedAt = "cancelled", result.Transmission.CompletedAt
		}
		if _, err := tx.Exec(`UPDATE automation_executions SET status = ?, outcome = 'cancelled',
reason_code = ?, completed_at = ?, revision = revision + 1
WHERE id = ? AND status IN ('accepted','cancelling')`, status, candidate.reason,
			completedAt, candidate.executionID); err != nil {
			return nil, err
		}
		if result.Changed || len(result.DisarmTargets) > 0 {
			results = append(results, result)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return results, nil
}

func automationOccurrenceKey(schedule AutomationSchedule, local time.Time) string {
	return fmt.Sprintf("%s/%d/%s/%02d:%02d", schedule.ID, schedule.Revision,
		local.Format("2006-01-02"), local.Hour(), local.Minute())
}

func automationTargetSnapshotDigest(targets []TransmissionTarget) string {
	parts := make([]string, 0, len(targets))
	for _, target := range targets {
		parts = append(parts, fmt.Sprintf("%d/%d/%s/%d/%s/%t/%s/%s",
			target.OrbitID, target.ActorID, target.Slot, target.BindingPairedAt,
			target.CapabilitySetHash, target.OnlineAtAcceptance, target.Status,
			target.ReasonCode))
	}
	sort.Strings(parts)
	return hashToken("barycenter/automation-target-snapshot/v1:" + strings.Join(parts, "\n"))
}
