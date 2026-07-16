package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"relux.works/duet/coordinator/internal/ulid"
)

const StreamAccountingDefaultStaleAfter = 30 * time.Minute

var (
	ErrStreamAccountingInvalid  = errors.New("stream accounting input is invalid")
	ErrStreamAccountingNotFound = errors.New("stream accounting item was not found")
	ErrStreamAccountingConflict = errors.New("stream accounting state changed")
	ErrStreamQuotaExceeded      = errors.New("stream quota exceeded")
	ErrStreamRangeAmplification = errors.New("stream range amplification limit exceeded")

	streamAccountingKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._:@/-]{16,128}$`)
	streamOperatorIDPattern    = regexp.MustCompile(`^[A-Za-z0-9._:@-]{1,128}$`)
	streamPolicyReasonPattern  = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
)

type StreamQuotaDimension string

const (
	StreamQuotaUploadStarts   StreamQuotaDimension = "upload_starts"
	StreamQuotaInputBytes     StreamQuotaDimension = "input_bytes"
	StreamQuotaCanonicalBytes StreamQuotaDimension = "canonical_bytes"
	StreamQuotaTempBytes      StreamQuotaDimension = "temp_processing_bytes"
	StreamQuotaConcurrentJobs StreamQuotaDimension = "concurrent_jobs"
	StreamQuotaRetainedBytes  StreamQuotaDimension = "retained_bytes"
	StreamQuotaEgressBytes    StreamQuotaDimension = "egress_bytes"
	StreamQuotaAmplification  StreamQuotaDimension = "range_amplification"
)

// StreamQuotaError deliberately omits current usage and the configured limit.
// Callers get one stable error while authenticated operator views retain the
// exact counters, preventing quota probing from becoming a cross-tenant oracle.
type StreamQuotaError struct {
	Dimension StreamQuotaDimension
}

func (err *StreamQuotaError) Error() string { return "stream quota exceeded: " + string(err.Dimension) }
func (err *StreamQuotaError) Unwrap() error { return ErrStreamQuotaExceeded }

type StreamQuotaPolicy struct {
	ScopeKind                string `json:"scope_kind"`
	ScopeID                  int64  `json:"scope_id"`
	MaxUploadStarts24h       int64  `json:"max_upload_starts_24h"`
	MaxInputBytes24h         int64  `json:"max_input_bytes_24h"`
	MaxCanonicalBytes        int64  `json:"max_canonical_bytes"`
	MaxTempProcessingBytes   int64  `json:"max_temp_processing_bytes"`
	MaxConcurrentJobs        int64  `json:"max_concurrent_jobs"`
	MaxRetainedBytes         int64  `json:"max_retained_bytes"`
	MaxEgressBytes24h        int64  `json:"max_egress_bytes_24h"`
	EgressAmplificationMilli int64  `json:"egress_amplification_milli"`
	Revision                 int64  `json:"revision"`
	UpdatedAt                int64  `json:"updated_at"`
}

type StreamAccountingUsage struct {
	ScopeKind                 string            `json:"scope_kind"`
	ScopeID                   int64             `json:"scope_id"`
	UploadStarts24h           int64             `json:"upload_starts_24h"`
	InputBytes24h             int64             `json:"input_bytes_24h"`
	CanonicalBytes            int64             `json:"canonical_bytes"`
	RetainedStorageBytes      int64             `json:"retained_storage_bytes"`
	TempProcessingBytes       int64             `json:"temp_processing_bytes"`
	TempProcessingReserved    int64             `json:"temp_processing_reserved_bytes"`
	ConcurrentJobs            int64             `json:"concurrent_jobs"`
	RangeRequests24h          int64             `json:"range_requests_24h"`
	ActualEgressBytes24h      int64             `json:"actual_egress_bytes_24h"`
	ActiveEgressReservedBytes int64             `json:"active_egress_reserved_bytes"`
	Policy                    StreamQuotaPolicy `json:"policy"`
}

type StreamAccountingSnapshot struct {
	Ready                       bool  `json:"ready"`
	Saturated                   bool  `json:"saturated"`
	LastReconciledAt            int64 `json:"last_reconciled_at"`
	ProcessingCrashReleases     int64 `json:"processing_crash_releases"`
	EgressCrashReleases         int64 `json:"egress_crash_releases"`
	UploadStarts24h             int64 `json:"upload_starts_24h"`
	InputBytes24h               int64 `json:"input_bytes_24h"`
	CanonicalBytes              int64 `json:"canonical_bytes"`
	RetainedStorageBytes        int64 `json:"retained_storage_bytes"`
	TempProcessingBytes         int64 `json:"temp_processing_bytes"`
	TempProcessingReservedBytes int64 `json:"temp_processing_reserved_bytes"`
	ConcurrentJobs              int64 `json:"concurrent_jobs"`
	RangeRequests24h            int64 `json:"range_requests_24h"`
	ActualEgressBytes24h        int64 `json:"actual_egress_bytes_24h"`
	ActiveEgressReservedBytes   int64 `json:"active_egress_reserved_bytes"`
	QuotaRejections24h          int64 `json:"quota_rejections_24h"`
	SaturatedScopes24h          int64 `json:"saturated_scopes_24h"`
}

type StreamAccountingOperatorView struct {
	Snapshot StreamAccountingSnapshot `json:"snapshot"`
	Usage    *StreamAccountingUsage   `json:"usage,omitempty"`
}

type StreamProcessingJob struct {
	ID, MediaID, IdempotencyKeyHash, State, Outcome      string
	OwnerOrbitID, ActorID, InputBytes, TempReservedBytes int64
	TempCurrentBytes, TempHighWaterBytes, Revision       int64
	CreatedAt, UpdatedAt, CompletedAt                    int64
	Reused                                               bool
}

type BeginStreamProcessingParams struct {
	MediaID, IdempotencyKey string
	TempReservedBytes       int64
	CreatedAt               int64
}

type StreamEgressSession struct {
	ID, MediaID, VariantID, IdempotencyKeyHash, State string
	OwnerOrbitID, ActorID, PlaybackGeneration         int64
	ReservedBytes, ActualBytes, RangeRequests         int64
	Revision, CreatedAt, UpdatedAt, CompletedAt       int64
	Reused                                            bool
}

type BeginStreamEgressParams struct {
	VariantID, IdempotencyKey string
	PlaybackGeneration        int64
	CreatedAt                 int64
}

type RecordStreamEgressParams struct {
	SessionID, RequestKey, Outcome string
	ExpectedRevision               int64
	RangeStart, RangeEnd           int64
	ActualBytes, CreatedAt         int64
}

type StreamEgressEvent struct {
	ID                                                          int64
	SessionID, RequestKeyHash, MediaID, VariantID, Outcome      string
	OwnerOrbitID, ActorID, RangeStart, RangeEnd, RequestedBytes int64
	ActualBytes, CreatedAt                                      int64
	Reused                                                      bool
}

type StreamAccountingReconcileResult struct {
	ProcessingCrashReleases int64 `json:"processing_crash_releases"`
	EgressCrashReleases     int64 `json:"egress_crash_releases"`
	ReconciledAt            int64 `json:"reconciled_at"`
}

type StreamAccountingPolicyAudit struct {
	ID, ScopeID, CreatedAt                    int64
	OperatorID, ScopeKind, PreviousPolicyJSON string
	NewPolicyJSON, Reason                     string
}

func streamAccountingHash(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func validateStreamQuotaPolicy(policy StreamQuotaPolicy) error {
	if (policy.ScopeKind != "actor" && policy.ScopeKind != "orbit") || policy.ScopeID < 0 ||
		policy.MaxUploadStarts24h <= 0 || policy.MaxInputBytes24h <= 0 ||
		policy.MaxCanonicalBytes <= 0 || policy.MaxTempProcessingBytes <= 0 ||
		policy.MaxConcurrentJobs <= 0 || policy.MaxRetainedBytes <= 0 ||
		policy.MaxEgressBytes24h <= 0 || policy.EgressAmplificationMilli < 1000 ||
		policy.EgressAmplificationMilli > 4000 {
		return ErrStreamAccountingInvalid
	}
	return nil
}

func scanStreamQuotaPolicy(row sqlScanner) (StreamQuotaPolicy, error) {
	var policy StreamQuotaPolicy
	err := row.Scan(&policy.ScopeKind, &policy.ScopeID, &policy.MaxUploadStarts24h,
		&policy.MaxInputBytes24h, &policy.MaxCanonicalBytes,
		&policy.MaxTempProcessingBytes, &policy.MaxConcurrentJobs,
		&policy.MaxRetainedBytes, &policy.MaxEgressBytes24h,
		&policy.EgressAmplificationMilli, &policy.Revision, &policy.UpdatedAt)
	return policy, err
}

const streamQuotaPolicyColumns = `scope_kind, scope_id, max_upload_starts_24h,
max_input_bytes_24h, max_canonical_bytes, max_temp_processing_bytes,
max_concurrent_jobs, max_retained_bytes, max_egress_bytes_24h,
egress_amplification_milli, revision, updated_at`

func streamQuotaPolicyTx(tx *sql.Tx, scopeKind string, scopeID int64) (StreamQuotaPolicy, error) {
	if (scopeKind != "actor" && scopeKind != "orbit") || scopeID <= 0 {
		return StreamQuotaPolicy{}, ErrStreamAccountingInvalid
	}
	policy, err := scanStreamQuotaPolicy(tx.QueryRow(`SELECT `+streamQuotaPolicyColumns+`
FROM stream_accounting_policies
WHERE scope_kind = ? AND scope_id IN (0, ?)
ORDER BY CASE WHEN scope_id = ? THEN 0 ELSE 1 END LIMIT 1`, scopeKind, scopeID, scopeID))
	if errors.Is(err, sql.ErrNoRows) {
		return StreamQuotaPolicy{}, ErrStreamAccountingNotFound
	}
	return policy, err
}

func streamScopeColumn(scopeKind string) (string, error) {
	switch scopeKind {
	case "actor":
		return "actor_id", nil
	case "orbit":
		return "owner_orbit_id", nil
	default:
		return "", ErrStreamAccountingInvalid
	}
}

func streamUsageForScopeTx(tx *sql.Tx, scopeKind string, scopeID, now int64) (StreamAccountingUsage, error) {
	column, err := streamScopeColumn(scopeKind)
	if err != nil || scopeID <= 0 || now <= 0 {
		return StreamAccountingUsage{}, ErrStreamAccountingInvalid
	}
	usage := StreamAccountingUsage{ScopeKind: scopeKind, ScopeID: scopeID}
	usage.Policy, err = streamQuotaPolicyTx(tx, scopeKind, scopeID)
	if err != nil {
		return StreamAccountingUsage{}, err
	}
	cutoff := now - int64((24*time.Hour)/time.Millisecond)
	if err := tx.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE
  WHEN uploads.status IN ('open', 'finalizing') THEN uploads.declared_size_bytes
  ELSE uploads.received_size_bytes END), 0)
FROM media_upload_sessions uploads
JOIN media_items media ON media.id = uploads.media_id
WHERE media.kind = 'audio_track' AND uploads.created_at > ? AND media.`+column+` = ?`,
		cutoff, scopeID).Scan(&usage.UploadStarts24h, &usage.InputBytes24h); err != nil {
		return StreamAccountingUsage{}, err
	}
	var originalBytes int64
	if err := tx.QueryRow(`SELECT COALESCE(SUM(metadata.original_size_bytes), 0)
FROM stream_track_metadata metadata
JOIN media_items media ON media.id = metadata.media_id
WHERE media.status NOT IN ('failed', 'deleted', 'expired') AND media.`+column+` = ?`,
		scopeID).Scan(&originalBytes); err != nil {
		return StreamAccountingUsage{}, err
	}
	if err := tx.QueryRow(`SELECT COALESCE(SUM(variants.size_bytes), 0)
FROM stream_variants variants
JOIN media_items media ON media.id = variants.media_id
WHERE variants.purpose = 'canonical' AND variants.status IN ('staged', 'ready')
  AND media.status NOT IN ('failed', 'deleted', 'expired') AND media.`+column+` = ?`,
		scopeID).Scan(&usage.CanonicalBytes); err != nil {
		return StreamAccountingUsage{}, err
	}
	usage.RetainedStorageBytes = originalBytes + usage.CanonicalBytes
	if err := tx.QueryRow(`SELECT COALESCE(SUM(temp_current_bytes), 0),
  COALESCE(SUM(temp_reserved_bytes), 0), COUNT(*)
FROM stream_processing_jobs WHERE state = 'active' AND `+column+` = ?`, scopeID).Scan(
		&usage.TempProcessingBytes, &usage.TempProcessingReserved, &usage.ConcurrentJobs,
	); err != nil {
		return StreamAccountingUsage{}, err
	}
	if err := tx.QueryRow(`SELECT COUNT(*), COALESCE(SUM(actual_bytes), 0)
FROM stream_egress_events WHERE created_at > ? AND `+column+` = ?`, cutoff, scopeID).Scan(
		&usage.RangeRequests24h, &usage.ActualEgressBytes24h,
	); err != nil {
		return StreamAccountingUsage{}, err
	}
	if err := tx.QueryRow(`SELECT COALESCE(SUM(reserved_bytes - actual_bytes), 0)
FROM stream_egress_sessions WHERE state = 'active' AND `+column+` = ?`, scopeID).Scan(
		&usage.ActiveEgressReservedBytes,
	); err != nil {
		return StreamAccountingUsage{}, err
	}
	return usage, nil
}

func (s *Store) GetStreamAccountingUsage(scopeKind string, scopeID, now int64) (StreamAccountingUsage, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return StreamAccountingUsage{}, err
	}
	defer tx.Rollback()
	usage, err := streamUsageForScopeTx(tx, scopeKind, scopeID, now)
	if err != nil {
		return StreamAccountingUsage{}, err
	}
	if err := tx.Commit(); err != nil {
		return StreamAccountingUsage{}, err
	}
	return usage, nil
}

func streamQuotaFailure(usage StreamAccountingUsage, dimension StreamQuotaDimension, delta int64) bool {
	switch dimension {
	case StreamQuotaUploadStarts:
		return usage.UploadStarts24h+delta > usage.Policy.MaxUploadStarts24h
	case StreamQuotaInputBytes:
		return usage.InputBytes24h+delta > usage.Policy.MaxInputBytes24h
	case StreamQuotaCanonicalBytes:
		return usage.CanonicalBytes+delta > usage.Policy.MaxCanonicalBytes
	case StreamQuotaTempBytes:
		return usage.TempProcessingReserved+delta > usage.Policy.MaxTempProcessingBytes
	case StreamQuotaConcurrentJobs:
		return usage.ConcurrentJobs+delta > usage.Policy.MaxConcurrentJobs
	case StreamQuotaRetainedBytes:
		return usage.RetainedStorageBytes+delta > usage.Policy.MaxRetainedBytes
	case StreamQuotaEgressBytes:
		return usage.ActualEgressBytes24h+usage.ActiveEgressReservedBytes+delta > usage.Policy.MaxEgressBytes24h
	default:
		return true
	}
}

func recordStreamQuotaRejectionTx(tx *sql.Tx, scopeKind string, scopeID int64, dimension StreamQuotaDimension, now int64) error {
	_, err := tx.Exec(`INSERT INTO stream_quota_rejections(scope_kind, scope_id, dimension, created_at)
VALUES(?, ?, ?, ?)`, scopeKind, scopeID, dimension, now)
	return err
}

func rejectStreamQuotaTx(tx *sql.Tx, scopeKind string, scopeID int64, dimension StreamQuotaDimension, now int64) error {
	if err := recordStreamQuotaRejectionTx(tx, scopeKind, scopeID, dimension, now); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return &StreamQuotaError{Dimension: dimension}
}

func streamTrackOwnerTx(tx *sql.Tx, mediaID string) (orbitID, actorID, inputBytes int64, err error) {
	var status MediaItemStatus
	err = tx.QueryRow(`SELECT media.owner_orbit_id, media.actor_id,
metadata.original_size_bytes, media.status
FROM stream_track_metadata metadata
JOIN media_items media ON media.id = metadata.media_id
WHERE metadata.media_id = ?`, mediaID).Scan(&orbitID, &actorID, &inputBytes, &status)
	if errors.Is(err, sql.ErrNoRows) {
		err = ErrStreamAccountingNotFound
		return
	}
	if err == nil && (status == MediaStatusFailed || status == MediaStatusDeleted || status == MediaStatusExpired) {
		err = ErrStreamAccountingNotFound
	}
	return
}

func scanStreamProcessingJob(row sqlScanner) (StreamProcessingJob, error) {
	var job StreamProcessingJob
	err := row.Scan(&job.ID, &job.MediaID, &job.OwnerOrbitID, &job.ActorID,
		&job.IdempotencyKeyHash, &job.InputBytes, &job.TempReservedBytes,
		&job.TempCurrentBytes, &job.TempHighWaterBytes, &job.State, &job.Outcome,
		&job.Revision, &job.CreatedAt, &job.UpdatedAt, &job.CompletedAt)
	return job, err
}

const streamProcessingJobColumns = `id, media_id, owner_orbit_id, actor_id,
idempotency_key_hash, input_bytes, temp_reserved_bytes, temp_current_bytes,
temp_high_water_bytes, state, outcome, revision, created_at, updated_at, completed_at`

func (s *Store) BeginStreamProcessing(params BeginStreamProcessingParams) (StreamProcessingJob, error) {
	if params.MediaID == "" || !streamAccountingKeyPattern.MatchString(params.IdempotencyKey) ||
		params.TempReservedBytes <= 0 || params.CreatedAt <= 0 {
		return StreamProcessingJob{}, ErrStreamAccountingInvalid
	}
	hash := streamAccountingHash(params.IdempotencyKey)
	tx, err := s.db.Begin()
	if err != nil {
		return StreamProcessingJob{}, err
	}
	defer tx.Rollback()
	existing, err := scanStreamProcessingJob(tx.QueryRow(`SELECT `+streamProcessingJobColumns+`
FROM stream_processing_jobs WHERE media_id = ? AND idempotency_key_hash = ?`, params.MediaID, hash))
	if err == nil {
		if existing.TempReservedBytes != params.TempReservedBytes {
			return StreamProcessingJob{}, ErrStreamAccountingConflict
		}
		existing.Reused = true
		if err := tx.Commit(); err != nil {
			return StreamProcessingJob{}, err
		}
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return StreamProcessingJob{}, err
	}
	orbitID, actorID, inputBytes, err := streamTrackOwnerTx(tx, params.MediaID)
	if err != nil {
		return StreamProcessingJob{}, err
	}
	for _, scope := range []struct {
		kind string
		id   int64
	}{{"actor", actorID}, {"orbit", orbitID}} {
		usage, err := streamUsageForScopeTx(tx, scope.kind, scope.id, params.CreatedAt)
		if err != nil {
			return StreamProcessingJob{}, err
		}
		for _, check := range []struct {
			dimension StreamQuotaDimension
			delta     int64
		}{{StreamQuotaConcurrentJobs, 1}, {StreamQuotaTempBytes, params.TempReservedBytes}} {
			if streamQuotaFailure(usage, check.dimension, check.delta) {
				return StreamProcessingJob{}, rejectStreamQuotaTx(tx, scope.kind, scope.id, check.dimension, params.CreatedAt)
			}
		}
	}
	id := "spj_" + ulid.New(time.UnixMilli(params.CreatedAt))
	_, err = tx.Exec(`INSERT INTO stream_processing_jobs(
id, media_id, owner_orbit_id, actor_id, idempotency_key_hash, input_bytes,
temp_reserved_bytes, created_at, updated_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, params.MediaID, orbitID, actorID,
		hash, inputBytes, params.TempReservedBytes, params.CreatedAt, params.CreatedAt)
	if err != nil {
		return StreamProcessingJob{}, err
	}
	job, err := scanStreamProcessingJob(tx.QueryRow(`SELECT `+streamProcessingJobColumns+`
FROM stream_processing_jobs WHERE id = ?`, id))
	if err != nil {
		return StreamProcessingJob{}, err
	}
	if err := tx.Commit(); err != nil {
		return StreamProcessingJob{}, err
	}
	return job, nil
}

func (s *Store) RecordStreamProcessingTemp(jobID string, expectedRevision, currentBytes, now int64) (StreamProcessingJob, error) {
	if jobID == "" || expectedRevision <= 0 || currentBytes < 0 || now <= 0 {
		return StreamProcessingJob{}, ErrStreamAccountingInvalid
	}
	result, err := s.db.Exec(`UPDATE stream_processing_jobs SET
temp_current_bytes = ?, temp_high_water_bytes = MAX(temp_high_water_bytes, ?),
revision = revision + 1, updated_at = ?
WHERE id = ? AND state = 'active' AND revision = ? AND temp_reserved_bytes >= ?`,
		currentBytes, currentBytes, now, jobID, expectedRevision, currentBytes)
	if err != nil {
		return StreamProcessingJob{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return StreamProcessingJob{}, ErrStreamAccountingConflict
	}
	job, err := scanStreamProcessingJob(s.db.QueryRow(`SELECT `+streamProcessingJobColumns+`
FROM stream_processing_jobs WHERE id = ?`, jobID))
	return job, err
}

func validStreamProcessingOutcome(outcome string) (state string, ok bool) {
	switch outcome {
	case "published":
		return "succeeded", true
	case "validation_failed", "processor_failed", "cancelled":
		return "failed", true
	default:
		return "", false
	}
}

func (s *Store) CompleteStreamProcessing(jobID string, expectedRevision int64, outcome string, now int64) (StreamProcessingJob, error) {
	state, ok := validStreamProcessingOutcome(outcome)
	if jobID == "" || expectedRevision <= 0 || !ok || now <= 0 {
		return StreamProcessingJob{}, ErrStreamAccountingInvalid
	}
	result, err := s.db.Exec(`UPDATE stream_processing_jobs SET state = ?, outcome = ?,
temp_current_bytes = 0, revision = revision + 1, updated_at = ?, completed_at = ?
WHERE id = ? AND state = 'active' AND revision = ?`, state, outcome, now, now, jobID, expectedRevision)
	if err != nil {
		return StreamProcessingJob{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return StreamProcessingJob{}, ErrStreamAccountingConflict
	}
	return scanStreamProcessingJob(s.db.QueryRow(`SELECT `+streamProcessingJobColumns+`
FROM stream_processing_jobs WHERE id = ?`, jobID))
}

func scanStreamEgressSession(row sqlScanner) (StreamEgressSession, error) {
	var session StreamEgressSession
	err := row.Scan(&session.ID, &session.MediaID, &session.VariantID,
		&session.OwnerOrbitID, &session.ActorID, &session.PlaybackGeneration,
		&session.IdempotencyKeyHash, &session.ReservedBytes, &session.ActualBytes,
		&session.RangeRequests, &session.State, &session.Revision,
		&session.CreatedAt, &session.UpdatedAt, &session.CompletedAt)
	return session, err
}

const streamEgressSessionColumns = `id, media_id, variant_id, owner_orbit_id,
actor_id, playback_generation, idempotency_key_hash, reserved_bytes,
actual_bytes, range_requests, state, revision, created_at, updated_at, completed_at`

func (s *Store) BeginStreamEgress(params BeginStreamEgressParams) (StreamEgressSession, error) {
	if params.VariantID == "" || !streamAccountingKeyPattern.MatchString(params.IdempotencyKey) ||
		params.PlaybackGeneration <= 0 || params.CreatedAt <= 0 {
		return StreamEgressSession{}, ErrStreamAccountingInvalid
	}
	hash := streamAccountingHash(params.IdempotencyKey)
	tx, err := s.db.Begin()
	if err != nil {
		return StreamEgressSession{}, err
	}
	defer tx.Rollback()
	existing, err := scanStreamEgressSession(tx.QueryRow(`SELECT `+streamEgressSessionColumns+`
FROM stream_egress_sessions WHERE variant_id = ? AND idempotency_key_hash = ?`, params.VariantID, hash))
	if err == nil {
		if existing.PlaybackGeneration != params.PlaybackGeneration {
			return StreamEgressSession{}, ErrStreamAccountingConflict
		}
		existing.Reused = true
		if err := tx.Commit(); err != nil {
			return StreamEgressSession{}, err
		}
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return StreamEgressSession{}, err
	}
	var mediaID string
	var variantBytes, orbitID, actorID int64
	var variantStatus string
	var mediaStatus MediaItemStatus
	if err := tx.QueryRow(`SELECT variants.media_id, variants.size_bytes, variants.status,
media.owner_orbit_id, media.actor_id, media.status
FROM stream_variants variants JOIN media_items media ON media.id = variants.media_id
WHERE variants.id = ?`, params.VariantID).Scan(&mediaID, &variantBytes, &variantStatus,
		&orbitID, &actorID, &mediaStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return StreamEgressSession{}, ErrStreamAccountingNotFound
		}
		return StreamEgressSession{}, err
	}
	if variantStatus != "ready" || mediaStatus == MediaStatusDeleted ||
		mediaStatus == MediaStatusExpired || mediaStatus == MediaStatusFailed {
		return StreamEgressSession{}, ErrStreamAccountingNotFound
	}
	actorPolicy, err := streamQuotaPolicyTx(tx, "actor", actorID)
	if err != nil {
		return StreamEgressSession{}, err
	}
	orbitPolicy, err := streamQuotaPolicyTx(tx, "orbit", orbitID)
	if err != nil {
		return StreamEgressSession{}, err
	}
	amplification := actorPolicy.EgressAmplificationMilli
	if orbitPolicy.EgressAmplificationMilli < amplification {
		amplification = orbitPolicy.EgressAmplificationMilli
	}
	reserved := (variantBytes*amplification + 999) / 1000
	for _, scope := range []struct {
		kind string
		id   int64
	}{{"actor", actorID}, {"orbit", orbitID}} {
		usage, err := streamUsageForScopeTx(tx, scope.kind, scope.id, params.CreatedAt)
		if err != nil {
			return StreamEgressSession{}, err
		}
		if streamQuotaFailure(usage, StreamQuotaEgressBytes, reserved) {
			return StreamEgressSession{}, rejectStreamQuotaTx(tx, scope.kind, scope.id, StreamQuotaEgressBytes, params.CreatedAt)
		}
	}
	id := "seg_" + ulid.New(time.UnixMilli(params.CreatedAt))
	_, err = tx.Exec(`INSERT INTO stream_egress_sessions(
id, media_id, variant_id, owner_orbit_id, actor_id, playback_generation,
idempotency_key_hash, reserved_bytes, created_at, updated_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, mediaID, params.VariantID,
		orbitID, actorID, params.PlaybackGeneration, hash, reserved,
		params.CreatedAt, params.CreatedAt)
	if err != nil {
		return StreamEgressSession{}, err
	}
	session, err := scanStreamEgressSession(tx.QueryRow(`SELECT `+streamEgressSessionColumns+`
FROM stream_egress_sessions WHERE id = ?`, id))
	if err != nil {
		return StreamEgressSession{}, err
	}
	if err := tx.Commit(); err != nil {
		return StreamEgressSession{}, err
	}
	return session, nil
}

func scanStreamEgressEvent(row sqlScanner) (StreamEgressEvent, error) {
	var event StreamEgressEvent
	err := row.Scan(&event.ID, &event.SessionID, &event.RequestKeyHash,
		&event.MediaID, &event.VariantID, &event.OwnerOrbitID, &event.ActorID,
		&event.RangeStart, &event.RangeEnd, &event.RequestedBytes,
		&event.ActualBytes, &event.Outcome, &event.CreatedAt)
	return event, err
}

const streamEgressEventColumns = `id, session_id, request_key_hash, media_id,
variant_id, owner_orbit_id, actor_id, range_start, range_end, requested_bytes,
actual_bytes, outcome, created_at`

func validStreamEgressOutcome(outcome string) bool {
	switch outcome {
	case "served", "cache_refill", "failed", "revoked", "client_cancelled":
		return true
	default:
		return false
	}
}

func (s *Store) RecordStreamEgress(params RecordStreamEgressParams) (StreamEgressSession, StreamEgressEvent, error) {
	requested := params.RangeEnd - params.RangeStart + 1
	if params.SessionID == "" || !streamAccountingKeyPattern.MatchString(params.RequestKey) ||
		params.ExpectedRevision <= 0 || params.RangeStart < 0 || params.RangeEnd < params.RangeStart ||
		requested <= 0 || params.ActualBytes < 0 || params.ActualBytes > requested ||
		!validStreamEgressOutcome(params.Outcome) || params.CreatedAt <= 0 {
		return StreamEgressSession{}, StreamEgressEvent{}, ErrStreamAccountingInvalid
	}
	hash := streamAccountingHash(params.RequestKey)
	tx, err := s.db.Begin()
	if err != nil {
		return StreamEgressSession{}, StreamEgressEvent{}, err
	}
	defer tx.Rollback()
	session, err := scanStreamEgressSession(tx.QueryRow(`SELECT `+streamEgressSessionColumns+`
FROM stream_egress_sessions WHERE id = ?`, params.SessionID))
	if errors.Is(err, sql.ErrNoRows) {
		return StreamEgressSession{}, StreamEgressEvent{}, ErrStreamAccountingNotFound
	}
	if err != nil {
		return StreamEgressSession{}, StreamEgressEvent{}, err
	}
	existing, err := scanStreamEgressEvent(tx.QueryRow(`SELECT `+streamEgressEventColumns+`
FROM stream_egress_events WHERE session_id = ? AND request_key_hash = ?`, params.SessionID, hash))
	if err == nil {
		if existing.RangeStart != params.RangeStart || existing.RangeEnd != params.RangeEnd ||
			existing.ActualBytes != params.ActualBytes || existing.Outcome != params.Outcome {
			return StreamEgressSession{}, StreamEgressEvent{}, ErrStreamAccountingConflict
		}
		existing.Reused = true
		if err := tx.Commit(); err != nil {
			return StreamEgressSession{}, StreamEgressEvent{}, err
		}
		return session, existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return StreamEgressSession{}, StreamEgressEvent{}, err
	}
	var variantBytes int64
	if err := tx.QueryRow(`SELECT size_bytes FROM stream_variants WHERE id = ?`, session.VariantID).Scan(&variantBytes); err != nil {
		return StreamEgressSession{}, StreamEgressEvent{}, err
	}
	if session.State != "active" || session.Revision != params.ExpectedRevision ||
		params.RangeEnd >= variantBytes {
		return StreamEgressSession{}, StreamEgressEvent{}, ErrStreamAccountingConflict
	}
	if session.ActualBytes+params.ActualBytes > session.ReservedBytes {
		if err := recordStreamQuotaRejectionTx(tx, "actor", session.ActorID, StreamQuotaAmplification, params.CreatedAt); err != nil {
			return StreamEgressSession{}, StreamEgressEvent{}, err
		}
		if err := recordStreamQuotaRejectionTx(tx, "orbit", session.OwnerOrbitID, StreamQuotaAmplification, params.CreatedAt); err != nil {
			return StreamEgressSession{}, StreamEgressEvent{}, err
		}
		if err := tx.Commit(); err != nil {
			return StreamEgressSession{}, StreamEgressEvent{}, err
		}
		return StreamEgressSession{}, StreamEgressEvent{}, ErrStreamRangeAmplification
	}
	result, err := tx.Exec(`UPDATE stream_egress_sessions SET actual_bytes = actual_bytes + ?,
range_requests = range_requests + 1, revision = revision + 1, updated_at = ?
WHERE id = ? AND state = 'active' AND revision = ?`, params.ActualBytes,
		params.CreatedAt, params.SessionID, params.ExpectedRevision)
	if err != nil {
		return StreamEgressSession{}, StreamEgressEvent{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return StreamEgressSession{}, StreamEgressEvent{}, ErrStreamAccountingConflict
	}
	result, err = tx.Exec(`INSERT INTO stream_egress_events(
session_id, request_key_hash, media_id, variant_id, owner_orbit_id, actor_id,
range_start, range_end, requested_bytes, actual_bytes, outcome, created_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, params.SessionID, hash,
		session.MediaID, session.VariantID, session.OwnerOrbitID, session.ActorID,
		params.RangeStart, params.RangeEnd, requested, params.ActualBytes,
		params.Outcome, params.CreatedAt)
	if err != nil {
		return StreamEgressSession{}, StreamEgressEvent{}, err
	}
	eventID, err := result.LastInsertId()
	if err != nil {
		return StreamEgressSession{}, StreamEgressEvent{}, err
	}
	session, err = scanStreamEgressSession(tx.QueryRow(`SELECT `+streamEgressSessionColumns+`
FROM stream_egress_sessions WHERE id = ?`, params.SessionID))
	if err != nil {
		return StreamEgressSession{}, StreamEgressEvent{}, err
	}
	event, err := scanStreamEgressEvent(tx.QueryRow(`SELECT `+streamEgressEventColumns+`
FROM stream_egress_events WHERE id = ?`, eventID))
	if err != nil {
		return StreamEgressSession{}, StreamEgressEvent{}, err
	}
	if err := tx.Commit(); err != nil {
		return StreamEgressSession{}, StreamEgressEvent{}, err
	}
	return session, event, nil
}

func (s *Store) CompleteStreamEgress(sessionID string, expectedRevision int64, state string, now int64) (StreamEgressSession, error) {
	if sessionID == "" || expectedRevision <= 0 || now <= 0 ||
		(state != "completed" && state != "cancelled" && state != "revoked") {
		return StreamEgressSession{}, ErrStreamAccountingInvalid
	}
	result, err := s.db.Exec(`UPDATE stream_egress_sessions SET state = ?,
revision = revision + 1, updated_at = ?, completed_at = ?
WHERE id = ? AND state = 'active' AND revision = ?`, state, now, now,
		sessionID, expectedRevision)
	if err != nil {
		return StreamEgressSession{}, err
	}
	if changed, _ := result.RowsAffected(); changed != 1 {
		return StreamEgressSession{}, ErrStreamAccountingConflict
	}
	return scanStreamEgressSession(s.db.QueryRow(`SELECT `+streamEgressSessionColumns+`
FROM stream_egress_sessions WHERE id = ?`, sessionID))
}

func (s *Store) ReconcileStreamAccounting(now int64, staleAfter time.Duration) (StreamAccountingReconcileResult, error) {
	if now <= 0 || staleAfter <= 0 {
		return StreamAccountingReconcileResult{}, ErrStreamAccountingInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return StreamAccountingReconcileResult{}, err
	}
	defer tx.Rollback()
	cutoff := now - staleAfter.Milliseconds()
	processing, err := tx.Exec(`UPDATE stream_processing_jobs SET state = 'expired',
outcome = 'crash_released', temp_current_bytes = 0, revision = revision + 1,
updated_at = ?, completed_at = ? WHERE state = 'active' AND updated_at < ?`, now, now, cutoff)
	if err != nil {
		return StreamAccountingReconcileResult{}, err
	}
	egress, err := tx.Exec(`UPDATE stream_egress_sessions SET state = 'expired',
revision = revision + 1, updated_at = ?, completed_at = ?
WHERE state = 'active' AND updated_at < ?`, now, now, cutoff)
	if err != nil {
		return StreamAccountingReconcileResult{}, err
	}
	processingCount, _ := processing.RowsAffected()
	egressCount, _ := egress.RowsAffected()
	_, err = tx.Exec(`UPDATE stream_accounting_state SET last_reconciled_at = ?,
processing_crash_releases = processing_crash_releases + ?,
egress_crash_releases = egress_crash_releases + ?, revision = revision + 1
WHERE singleton = 1`, now, processingCount, egressCount)
	if err != nil {
		return StreamAccountingReconcileResult{}, err
	}
	if err := s.checkpoint("stream_accounting_reconcile_before_commit"); err != nil {
		return StreamAccountingReconcileResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return StreamAccountingReconcileResult{}, err
	}
	return StreamAccountingReconcileResult{
		ProcessingCrashReleases: processingCount,
		EgressCrashReleases:     egressCount,
		ReconciledAt:            now,
	}, nil
}

func streamAccountingSnapshot(q rowQuerier, now int64) (StreamAccountingSnapshot, error) {
	if now <= 0 {
		return StreamAccountingSnapshot{}, ErrStreamAccountingInvalid
	}
	var snapshot StreamAccountingSnapshot
	cutoff := now - int64((24*time.Hour)/time.Millisecond)
	var defaults int64
	if err := q.QueryRow(`SELECT COUNT(*) FROM stream_accounting_policies
WHERE scope_id = 0 AND scope_kind IN ('actor', 'orbit')`).Scan(&defaults); err != nil {
		return StreamAccountingSnapshot{}, err
	}
	if err := q.QueryRow(`SELECT last_reconciled_at, processing_crash_releases,
egress_crash_releases FROM stream_accounting_state WHERE singleton = 1`).Scan(
		&snapshot.LastReconciledAt, &snapshot.ProcessingCrashReleases,
		&snapshot.EgressCrashReleases,
	); err != nil {
		return StreamAccountingSnapshot{}, err
	}
	snapshot.Ready = defaults == 2 && snapshot.LastReconciledAt > 0
	if err := q.QueryRow(`SELECT COUNT(*), COALESCE(SUM(CASE
  WHEN uploads.status IN ('open', 'finalizing') THEN uploads.declared_size_bytes
  ELSE uploads.received_size_bytes END), 0)
FROM media_upload_sessions uploads JOIN media_items media ON media.id = uploads.media_id
WHERE media.kind = 'audio_track' AND uploads.created_at > ?`, cutoff).Scan(
		&snapshot.UploadStarts24h, &snapshot.InputBytes24h,
	); err != nil {
		return StreamAccountingSnapshot{}, err
	}
	var originalBytes int64
	if err := q.QueryRow(`SELECT COALESCE(SUM(metadata.original_size_bytes), 0)
FROM stream_track_metadata metadata JOIN media_items media ON media.id = metadata.media_id
WHERE media.status NOT IN ('failed', 'deleted', 'expired')`).Scan(&originalBytes); err != nil {
		return StreamAccountingSnapshot{}, err
	}
	if err := q.QueryRow(`SELECT COALESCE(SUM(variants.size_bytes), 0)
FROM stream_variants variants JOIN media_items media ON media.id = variants.media_id
WHERE variants.purpose = 'canonical' AND variants.status IN ('staged', 'ready')
AND media.status NOT IN ('failed', 'deleted', 'expired')`).Scan(&snapshot.CanonicalBytes); err != nil {
		return StreamAccountingSnapshot{}, err
	}
	snapshot.RetainedStorageBytes = originalBytes + snapshot.CanonicalBytes
	if err := q.QueryRow(`SELECT COALESCE(SUM(temp_current_bytes), 0),
COALESCE(SUM(temp_reserved_bytes), 0), COUNT(*) FROM stream_processing_jobs
WHERE state = 'active'`).Scan(&snapshot.TempProcessingBytes,
		&snapshot.TempProcessingReservedBytes, &snapshot.ConcurrentJobs); err != nil {
		return StreamAccountingSnapshot{}, err
	}
	if err := q.QueryRow(`SELECT COUNT(*), COALESCE(SUM(actual_bytes), 0)
FROM stream_egress_events WHERE created_at > ?`, cutoff).Scan(
		&snapshot.RangeRequests24h, &snapshot.ActualEgressBytes24h,
	); err != nil {
		return StreamAccountingSnapshot{}, err
	}
	if err := q.QueryRow(`SELECT COALESCE(SUM(reserved_bytes - actual_bytes), 0)
FROM stream_egress_sessions WHERE state = 'active'`).Scan(&snapshot.ActiveEgressReservedBytes); err != nil {
		return StreamAccountingSnapshot{}, err
	}
	if err := q.QueryRow(`SELECT COUNT(*), COUNT(DISTINCT scope_kind || ':' || scope_id)
FROM stream_quota_rejections WHERE created_at > ?`, cutoff).Scan(
		&snapshot.QuotaRejections24h, &snapshot.SaturatedScopes24h,
	); err != nil {
		return StreamAccountingSnapshot{}, err
	}
	snapshot.Saturated = snapshot.QuotaRejections24h > 0
	return snapshot, nil
}

func (s *Store) StreamAccountingSnapshot(now int64) (StreamAccountingSnapshot, error) {
	return streamAccountingSnapshot(s.db, now)
}

func (s *Store) GetAuthorizedStreamAccounting(
	operatorID, bearer, scopeKind string,
	scopeID, now int64,
) (StreamAccountingOperatorView, error) {
	if operatorID == "" || now <= 0 ||
		!((scopeKind == "" && scopeID == 0) ||
			((scopeKind == "actor" || scopeKind == "orbit") && scopeID > 0)) {
		return StreamAccountingOperatorView{}, ErrStreamAccountingInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return StreamAccountingOperatorView{}, err
	}
	defer tx.Rollback()
	operator, err := resolveModerationOperator(tx, bearer)
	if err != nil {
		return StreamAccountingOperatorView{}, err
	}
	if operator.ID != operatorID || !operator.Capabilities.List {
		return StreamAccountingOperatorView{}, ErrModerationForbidden
	}
	snapshot, err := streamAccountingSnapshot(tx, now)
	if err != nil {
		return StreamAccountingOperatorView{}, err
	}
	view := StreamAccountingOperatorView{Snapshot: snapshot}
	if scopeKind != "" {
		usage, err := streamUsageForScopeTx(tx, scopeKind, scopeID, now)
		if err != nil {
			return StreamAccountingOperatorView{}, err
		}
		view.Usage = &usage
	}
	if err := tx.Commit(); err != nil {
		return StreamAccountingOperatorView{}, err
	}
	return view, nil
}

func (s *Store) SetStreamQuotaPolicy(operatorID, bearer string, policy StreamQuotaPolicy, expectedRevision int64, reason string, now int64) (StreamQuotaPolicy, error) {
	if !streamOperatorIDPattern.MatchString(operatorID) || validateStreamQuotaPolicy(policy) != nil ||
		expectedRevision < 0 || !streamPolicyReasonPattern.MatchString(reason) || now <= 0 {
		return StreamQuotaPolicy{}, ErrStreamAccountingInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return StreamQuotaPolicy{}, err
	}
	defer tx.Rollback()
	operator, err := resolveModerationOperator(tx, bearer)
	if err != nil {
		return StreamQuotaPolicy{}, err
	}
	if operator.ID != operatorID || !operator.Capabilities.Decide {
		return StreamQuotaPolicy{}, ErrModerationForbidden
	}
	previous, err := scanStreamQuotaPolicy(tx.QueryRow(`SELECT `+streamQuotaPolicyColumns+`
FROM stream_accounting_policies WHERE scope_kind = ? AND scope_id = ?`, policy.ScopeKind, policy.ScopeID))
	exists := err == nil
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return StreamQuotaPolicy{}, err
	}
	if (exists && previous.Revision != expectedRevision) || (!exists && expectedRevision != 0) {
		return StreamQuotaPolicy{}, ErrStreamAccountingConflict
	}
	previousJSON := `{}`
	if exists {
		raw, _ := json.Marshal(previous)
		previousJSON = string(raw)
		policy.Revision = previous.Revision + 1
	} else {
		policy.Revision = 1
	}
	policy.UpdatedAt = now
	if exists {
		result, err := tx.Exec(`UPDATE stream_accounting_policies SET
max_upload_starts_24h = ?, max_input_bytes_24h = ?, max_canonical_bytes = ?,
max_temp_processing_bytes = ?, max_concurrent_jobs = ?, max_retained_bytes = ?,
max_egress_bytes_24h = ?, egress_amplification_milli = ?, revision = ?, updated_at = ?
WHERE scope_kind = ? AND scope_id = ? AND revision = ?`, policy.MaxUploadStarts24h,
			policy.MaxInputBytes24h, policy.MaxCanonicalBytes, policy.MaxTempProcessingBytes,
			policy.MaxConcurrentJobs, policy.MaxRetainedBytes, policy.MaxEgressBytes24h,
			policy.EgressAmplificationMilli, policy.Revision, now, policy.ScopeKind,
			policy.ScopeID, expectedRevision)
		if err != nil {
			return StreamQuotaPolicy{}, err
		}
		if changed, _ := result.RowsAffected(); changed != 1 {
			return StreamQuotaPolicy{}, ErrStreamAccountingConflict
		}
	} else {
		_, err := tx.Exec(`INSERT INTO stream_accounting_policies(
scope_kind, scope_id, max_upload_starts_24h, max_input_bytes_24h,
max_canonical_bytes, max_temp_processing_bytes, max_concurrent_jobs,
max_retained_bytes, max_egress_bytes_24h, egress_amplification_milli,
revision, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?)`,
			policy.ScopeKind, policy.ScopeID, policy.MaxUploadStarts24h,
			policy.MaxInputBytes24h, policy.MaxCanonicalBytes,
			policy.MaxTempProcessingBytes, policy.MaxConcurrentJobs,
			policy.MaxRetainedBytes, policy.MaxEgressBytes24h,
			policy.EgressAmplificationMilli, now)
		if err != nil {
			return StreamQuotaPolicy{}, err
		}
	}
	newJSON, _ := json.Marshal(policy)
	_, err = tx.Exec(`INSERT INTO stream_accounting_policy_audit(
operator_id, scope_kind, scope_id, previous_policy_json, new_policy_json,
reason, created_at) VALUES(?, ?, ?, ?, ?, ?, ?)`, operatorID,
		policy.ScopeKind, policy.ScopeID, previousJSON, string(newJSON), reason, now)
	if err != nil {
		return StreamQuotaPolicy{}, err
	}
	if err := tx.Commit(); err != nil {
		return StreamQuotaPolicy{}, err
	}
	return policy, nil
}

type streamAccountingRowsQuerier interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

func listStreamQuotaPolicyAudit(q streamAccountingRowsQuerier, scopeKind string, scopeID int64, limit int) ([]StreamAccountingPolicyAudit, error) {
	if (scopeKind != "actor" && scopeKind != "orbit") || scopeID < 0 || limit < 1 || limit > 500 {
		return nil, ErrStreamAccountingInvalid
	}
	rows, err := q.Query(`SELECT id, operator_id, scope_kind, scope_id,
previous_policy_json, new_policy_json, reason, created_at
FROM stream_accounting_policy_audit WHERE scope_kind = ? AND scope_id = ?
ORDER BY id DESC LIMIT ?`, scopeKind, scopeID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []StreamAccountingPolicyAudit
	for rows.Next() {
		var event StreamAccountingPolicyAudit
		if err := rows.Scan(&event.ID, &event.OperatorID, &event.ScopeKind,
			&event.ScopeID, &event.PreviousPolicyJSON, &event.NewPolicyJSON,
			&event.Reason, &event.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, event)
	}
	return out, rows.Err()
}

func (s *Store) ListStreamQuotaPolicyAudit(scopeKind string, scopeID int64, limit int) ([]StreamAccountingPolicyAudit, error) {
	return listStreamQuotaPolicyAudit(s.db, scopeKind, scopeID, limit)
}

func (s *Store) ListAuthorizedStreamQuotaPolicyAudit(
	operatorID, bearer, scopeKind string, scopeID int64, limit int,
) ([]StreamAccountingPolicyAudit, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	operator, err := resolveModerationOperator(tx, bearer)
	if err != nil {
		return nil, err
	}
	if operator.ID != operatorID || !operator.Capabilities.List {
		return nil, ErrModerationForbidden
	}
	events, err := listStreamQuotaPolicyAudit(tx, scopeKind, scopeID, limit)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return events, nil
}

func streamUploadCapacityTx(tx *sql.Tx, params CreateMediaUploadParams) error {
	if params.Media.Kind != MediaKindAudioTrack {
		return nil
	}
	for _, scope := range []struct {
		kind string
		id   int64
	}{{"actor", params.Media.ActorID}, {"orbit", params.Media.OwnerOrbitID}} {
		usage, err := streamUsageForScopeTx(tx, scope.kind, scope.id, params.Media.CreatedAt)
		if err != nil {
			return err
		}
		if streamQuotaFailure(usage, StreamQuotaUploadStarts, 1) {
			if err := recordStreamQuotaRejectionTx(tx, scope.kind, scope.id, StreamQuotaUploadStarts, params.Media.CreatedAt); err != nil {
				return err
			}
			return &StreamQuotaError{Dimension: StreamQuotaUploadStarts}
		}
		if streamQuotaFailure(usage, StreamQuotaInputBytes, params.DeclaredSizeBytes) {
			if err := recordStreamQuotaRejectionTx(tx, scope.kind, scope.id, StreamQuotaInputBytes, params.Media.CreatedAt); err != nil {
				return err
			}
			return &StreamQuotaError{Dimension: StreamQuotaInputBytes}
		}
	}
	return nil
}

func streamVariantCapacityTx(tx *sql.Tx, mediaID string, variantBytes, now int64) error {
	orbitID, actorID, _, err := streamTrackOwnerTx(tx, mediaID)
	if err != nil {
		return err
	}
	for _, scope := range []struct {
		kind string
		id   int64
	}{{"actor", actorID}, {"orbit", orbitID}} {
		usage, err := streamUsageForScopeTx(tx, scope.kind, scope.id, now)
		if err != nil {
			return err
		}
		if streamQuotaFailure(usage, StreamQuotaCanonicalBytes, variantBytes) {
			if err := recordStreamQuotaRejectionTx(tx, scope.kind, scope.id, StreamQuotaCanonicalBytes, now); err != nil {
				return err
			}
			return &StreamQuotaError{Dimension: StreamQuotaCanonicalBytes}
		}
		if streamQuotaFailure(usage, StreamQuotaRetainedBytes, variantBytes) {
			if err := recordStreamQuotaRejectionTx(tx, scope.kind, scope.id, StreamQuotaRetainedBytes, now); err != nil {
				return err
			}
			return &StreamQuotaError{Dimension: StreamQuotaRetainedBytes}
		}
	}
	return nil
}

func (audit StreamAccountingPolicyAudit) MarshalJSON() ([]byte, error) {
	var previous, next any
	if err := json.Unmarshal([]byte(audit.PreviousPolicyJSON), &previous); err != nil {
		return nil, fmt.Errorf("decode previous stream quota policy: %w", err)
	}
	if err := json.Unmarshal([]byte(audit.NewPolicyJSON), &next); err != nil {
		return nil, fmt.Errorf("decode new stream quota policy: %w", err)
	}
	return json.Marshal(struct {
		ID             int64  `json:"id"`
		ScopeID        int64  `json:"scope_id"`
		CreatedAt      int64  `json:"created_at"`
		OperatorID     string `json:"operator_id"`
		ScopeKind      string `json:"scope_kind"`
		Reason         string `json:"reason"`
		PreviousPolicy any    `json:"previous_policy"`
		NewPolicy      any    `json:"new_policy"`
	}{audit.ID, audit.ScopeID, audit.CreatedAt, audit.OperatorID,
		audit.ScopeKind, audit.Reason, previous, next})
}
