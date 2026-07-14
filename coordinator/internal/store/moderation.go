package store

import (
	"database/sql"
	"errors"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"relux.works/duet/coordinator/internal/ulid"
)

var (
	ErrModerationInvalid          = errors.New("moderation input is invalid")
	ErrModerationNotFound         = errors.New("moderation item was not found")
	ErrModerationForbidden        = errors.New("moderation operation is forbidden")
	ErrModerationRateLimited      = errors.New("moderation report rate limit reached")
	ErrModerationDecisionConflict = errors.New("moderation decision conflicts with existing state")
	ErrModerationEvidenceExpired  = errors.New("moderation evidence is no longer available")
)

const (
	moderationEvidenceRetention = 30 * 24 * time.Hour
	moderationReportRateWindow  = time.Hour
	moderationReportRateLimit   = 10
)

var (
	moderationReportIDPattern   = regexp.MustCompile(`^rp_[0-9A-HJKMNP-TV-Z]{26}$`)
	moderationOperatorIDPattern = regexp.MustCompile(`^op_[0-9A-HJKMNP-TV-Z]{26}$`)
	moderationDecisionIDPattern = regexp.MustCompile(`^md_[0-9A-HJKMNP-TV-Z]{26}$`)
	moderationTokenPattern      = regexp.MustCompile(`^mod_[0-9a-f]{64}$`)
)

type ModerationReason string

const (
	ModerationReasonSpam          ModerationReason = "spam"
	ModerationReasonHarassment    ModerationReason = "harassment"
	ModerationReasonIllegal       ModerationReason = "illegal"
	ModerationReasonSexualContent ModerationReason = "sexual_content"
	ModerationReasonViolence      ModerationReason = "violence"
	ModerationReasonOther         ModerationReason = "other"
)

func validModerationReason(reason ModerationReason) bool {
	switch reason {
	case ModerationReasonSpam, ModerationReasonHarassment,
		ModerationReasonIllegal, ModerationReasonSexualContent,
		ModerationReasonViolence, ModerationReasonOther:
		return true
	default:
		return false
	}
}

type ModerationAction string

const (
	ModerationActionNoAction     ModerationAction = "no_action"
	ModerationActionDeleteMedia  ModerationAction = "delete_media"
	ModerationActionDisableActor ModerationAction = "disable_actor"
	ModerationActionDisableOrbit ModerationAction = "disable_orbit"
)

func validModerationAction(action ModerationAction) bool {
	switch action {
	case ModerationActionNoAction, ModerationActionDeleteMedia,
		ModerationActionDisableActor, ModerationActionDisableOrbit:
		return true
	default:
		return false
	}
}

type ModerationOperatorCapabilities struct {
	List     bool
	Evidence bool
	Decide   bool
}

type ModerationOperatorContext struct {
	ID           string
	DisplayName  string
	Capabilities ModerationOperatorCapabilities
}

type ModerationOperatorCredential struct {
	Operator  ModerationOperatorContext
	Token     string
	CreatedAt int64
}

type ModerationReport struct {
	ID                 string
	ReporterOrbitID    int64
	ReporterActorID    int64
	MediaID            string
	ReportedOrbitID    int64
	ReportedActorID    int64
	MediaKind          MediaKind
	MediaSource        MediaSource
	MediaTitle         string
	MediaDurationMS    int64
	TransmissionID     string
	TargetOrbitID      int64
	TargetActorID      int64
	TargetSlot         string
	AudienceKind       TransmissionAudienceKind
	PlaybackDomainKind PlaybackDomainKind
	PlaybackDomainID   int64
	AcceptedAt         int64
	Reason             ModerationReason
	Details            string
	Status             string
	EvidenceStorageKey string
	EvidenceSHA256     string
	EvidenceSizeBytes  int64
	EvidenceMIME       string
	EvidenceExpiresAt  int64
	CreatedAt          int64
	UpdatedAt          int64
	ResolvedAt         int64
}

type CreateModerationReportParams struct {
	MediaID   string
	Reason    ModerationReason
	Details   string
	CreatedAt int64
}

type ModerationReportCreation struct {
	Report ModerationReport
	Reused bool
}

type ModerationEvidence struct {
	ReportID   string
	MediaID    string
	StorageKey string
	SHA256     string
	SizeBytes  int64
	MIME       string
	ExpiresAt  int64
}

type ModerationDecision struct {
	ID                    string
	ReportID              string
	Action                ModerationAction
	State                 string
	RequestedByOperatorID string
	RequestedAt           int64
	AppliedAt             int64
}

type ModerationDecisionRequest struct {
	Decision ModerationDecision
	Report   ModerationReport
	Reused   bool
	Applied  bool
}

type ModerationReportBlock struct {
	Report ModerationReport
	Block  TransmissionBlockCreation
}

const moderationReportColumns = `id, reporter_orbit_id, reporter_actor_id,
media_id, reported_orbit_id, reported_actor_id, media_kind, media_source,
media_title, media_duration_ms, transmission_id,
target_orbit_id, target_actor_id, target_slot, audience_kind,
playback_domain_kind, playback_domain_id, accepted_at, reason_code, details,
status, evidence_storage_key, evidence_sha256, evidence_size_bytes,
evidence_mime, evidence_expires_at, created_at, updated_at, resolved_at`

func scanModerationReport(row sqlScanner) (ModerationReport, error) {
	var report ModerationReport
	err := row.Scan(
		&report.ID, &report.ReporterOrbitID, &report.ReporterActorID,
		&report.MediaID, &report.ReportedOrbitID, &report.ReportedActorID,
		&report.MediaKind, &report.MediaSource, &report.MediaTitle,
		&report.MediaDurationMS, &report.TransmissionID,
		&report.TargetOrbitID, &report.TargetActorID,
		&report.TargetSlot, &report.AudienceKind, &report.PlaybackDomainKind,
		&report.PlaybackDomainID, &report.AcceptedAt, &report.Reason,
		&report.Details, &report.Status, &report.EvidenceStorageKey,
		&report.EvidenceSHA256, &report.EvidenceSizeBytes, &report.EvidenceMIME,
		&report.EvidenceExpiresAt, &report.CreatedAt, &report.UpdatedAt,
		&report.ResolvedAt,
	)
	return report, err
}

const moderationDecisionColumns = `id, report_id, action, state,
requested_by_operator_id, requested_at, applied_at`

func scanModerationDecision(row sqlScanner) (ModerationDecision, error) {
	var decision ModerationDecision
	err := row.Scan(
		&decision.ID, &decision.ReportID, &decision.Action, &decision.State,
		&decision.RequestedByOperatorID, &decision.RequestedAt,
		&decision.AppliedAt,
	)
	return decision, err
}

func validateOperatorCapabilities(capabilities ModerationOperatorCapabilities) bool {
	return capabilities.List || capabilities.Evidence || capabilities.Decide
}

func (s *Store) ProvisionModerationOperator(
	displayName string,
	capabilities ModerationOperatorCapabilities,
	now int64,
) (ModerationOperatorCredential, error) {
	displayName = strings.TrimSpace(displayName)
	if now <= 0 || displayName == "" || len(displayName) > 120 ||
		!utf8.ValidString(displayName) || !validateOperatorCapabilities(capabilities) {
		return ModerationOperatorCredential{}, ErrModerationInvalid
	}
	raw, err := randomHexSecret(32)
	if err != nil {
		return ModerationOperatorCredential{}, err
	}
	token := "mod_" + raw
	id := ulid.NewModerationOperatorID(time.UnixMilli(now))
	tx, err := s.db.Begin()
	if err != nil {
		return ModerationOperatorCredential{}, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`INSERT INTO moderation_operators(
  id, display_name, token_hash, can_list, can_evidence, can_decide, created_at
) VALUES(?, ?, ?, ?, ?, ?, ?)`, id, displayName, hashToken(token),
		capabilities.List, capabilities.Evidence, capabilities.Decide, now); err != nil {
		return ModerationOperatorCredential{}, err
	}
	if _, err := tx.Exec(`INSERT INTO moderation_audit_events(
  operator_id, event_type, created_at
) VALUES(?, 'operator.provisioned', ?)`, id, now); err != nil {
		return ModerationOperatorCredential{}, err
	}
	if err := tx.Commit(); err != nil {
		return ModerationOperatorCredential{}, err
	}
	return ModerationOperatorCredential{
		Operator: ModerationOperatorContext{
			ID: id, DisplayName: displayName, Capabilities: capabilities,
		},
		Token: token, CreatedAt: now,
	}, nil
}

func scanModerationOperator(row sqlScanner) (ModerationOperatorContext, int64, error) {
	var operator ModerationOperatorContext
	var list, evidence, decide bool
	var revokedAt int64
	err := row.Scan(
		&operator.ID, &operator.DisplayName, &list, &evidence, &decide,
		&revokedAt,
	)
	operator.Capabilities = ModerationOperatorCapabilities{
		List: list, Evidence: evidence, Decide: decide,
	}
	return operator, revokedAt, err
}

func resolveModerationOperator(q rowQuerier, token string) (ModerationOperatorContext, error) {
	if !moderationTokenPattern.MatchString(token) {
		return ModerationOperatorContext{}, ErrUnauthorized
	}
	operator, revokedAt, err := scanModerationOperator(q.QueryRow(`SELECT
  id, display_name, can_list, can_evidence, can_decide, revoked_at
FROM moderation_operators WHERE token_hash = ?`, hashToken(token)))
	if errors.Is(err, sql.ErrNoRows) || revokedAt != 0 {
		return ModerationOperatorContext{}, ErrUnauthorized
	}
	return operator, err
}

func (s *Store) ResolveModerationOperator(token string) (ModerationOperatorContext, error) {
	return resolveModerationOperator(s.db, token)
}

func (s *Store) RevokeModerationOperator(operatorID string, now int64) (bool, error) {
	if !moderationOperatorIDPattern.MatchString(operatorID) || now <= 0 {
		return false, ErrModerationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE moderation_operators
SET revoked_at = ? WHERE id = ? AND revoked_at = 0`, now, operatorID)
	if err != nil {
		return false, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	if changed == 0 {
		var exists int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM moderation_operators WHERE id = ?`, operatorID).Scan(&exists); err != nil {
			return false, err
		}
		if exists == 0 {
			return false, nil
		}
		return false, tx.Commit()
	}
	if _, err := tx.Exec(`INSERT INTO moderation_audit_events(
  operator_id, event_type, created_at
) VALUES(?, 'operator.revoked', ?)`, operatorID, now); err != nil {
		return false, err
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return true, nil
}

func (s *Store) CreateModerationReport(
	expectedActorID int64,
	bearer string,
	params CreateModerationReportParams,
) (ModerationReportCreation, error) {
	if !s.selfServiceOnboarding {
		return ModerationReportCreation{}, ErrSelfServiceOnboardingDisabled
	}
	if expectedActorID <= 0 || !lowerHexTokenPattern.MatchString(bearer) {
		return ModerationReportCreation{}, ErrUnauthorized
	}
	params.Details = strings.TrimSpace(params.Details)
	if !mediaItemIDPattern.MatchString(params.MediaID) ||
		!validModerationReason(params.Reason) || params.CreatedAt <= 0 ||
		len(params.Details) > 2000 || !utf8.ValidString(params.Details) {
		return ModerationReportCreation{}, ErrModerationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return ModerationReportCreation{}, err
	}
	defer tx.Rollback()
	ctx, err := mutationActorContextTx(tx, expectedActorID, hashToken(bearer))
	if err != nil {
		return ModerationReportCreation{}, err
	}
	existing, err := scanModerationReport(tx.QueryRow(
		`SELECT `+moderationReportColumns+` FROM moderation_reports
WHERE reporter_actor_id = ? AND media_id = ?`, ctx.ActorID, params.MediaID,
	))
	if err == nil {
		if err := tx.Commit(); err != nil {
			return ModerationReportCreation{}, err
		}
		return ModerationReportCreation{Report: existing, Reused: true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ModerationReportCreation{}, err
	}
	windowStart := params.CreatedAt - moderationReportRateWindow.Milliseconds()
	if _, err := tx.Exec(`DELETE FROM moderation_report_attempts WHERE created_at <= ?`,
		params.CreatedAt-int64((24*time.Hour)/time.Millisecond)); err != nil {
		return ModerationReportCreation{}, err
	}
	var attempts int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM moderation_report_attempts
WHERE reporter_actor_id = ? AND created_at > ?`, ctx.ActorID, windowStart).Scan(&attempts); err != nil {
		return ModerationReportCreation{}, err
	}
	if attempts >= moderationReportRateLimit {
		if _, err := tx.Exec(`INSERT INTO moderation_report_attempts(
  reporter_orbit_id, reporter_actor_id, allowed, created_at
) VALUES(?, ?, 0, ?)`, ctx.OrbitID, ctx.ActorID, params.CreatedAt); err != nil {
			return ModerationReportCreation{}, err
		}
		if _, err := tx.Exec(`INSERT INTO moderation_audit_events(
  actor_id, event_type, created_at
) VALUES(?, 'report.rate_limited', ?)`, ctx.ActorID, params.CreatedAt); err != nil {
			return ModerationReportCreation{}, err
		}
		if err := tx.Commit(); err != nil {
			return ModerationReportCreation{}, err
		}
		return ModerationReportCreation{}, ErrModerationRateLimited
	}
	var report ModerationReport
	err = tx.QueryRow(`SELECT
  '', ?, ?, i.id, i.owner_orbit_id, i.actor_id,
  i.kind, i.source, i.title, i.duration_ms, tr.id,
  tt.orbit_id, tt.actor_id, tt.slot, tr.audience_kind,
  tr.playback_domain_kind, tr.playback_domain_id, tr.accepted_at,
  ?, ?, 'open', i.storage_key, i.sha256, i.size_bytes, i.mime,
  ?, ?, ?, 0
FROM media_items i
JOIN transmissions tr ON tr.media_id = i.id
JOIN transmission_targets tt ON tt.transmission_id = tr.id
WHERE i.id = ? AND i.actor_id <> ? AND i.status = 'ready'
  AND i.expires_at > ? AND tr.accepted_at <= ?
  AND tt.orbit_id = ? AND tt.actor_id = ? AND tt.slot = ?
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
ORDER BY tr.accepted_at DESC, tr.id DESC
LIMIT 1`,
		ctx.OrbitID, ctx.ActorID, params.Reason, params.Details,
		params.CreatedAt+moderationEvidenceRetention.Milliseconds(),
		params.CreatedAt, params.CreatedAt, params.MediaID, ctx.ActorID,
		params.CreatedAt, params.CreatedAt, ctx.OrbitID, ctx.ActorID, ctx.Slot,
	).Scan(
		&report.ID, &report.ReporterOrbitID, &report.ReporterActorID,
		&report.MediaID, &report.ReportedOrbitID, &report.ReportedActorID,
		&report.MediaKind, &report.MediaSource, &report.MediaTitle,
		&report.MediaDurationMS, &report.TransmissionID,
		&report.TargetOrbitID, &report.TargetActorID,
		&report.TargetSlot, &report.AudienceKind, &report.PlaybackDomainKind,
		&report.PlaybackDomainID, &report.AcceptedAt, &report.Reason,
		&report.Details, &report.Status, &report.EvidenceStorageKey,
		&report.EvidenceSHA256, &report.EvidenceSizeBytes, &report.EvidenceMIME,
		&report.EvidenceExpiresAt, &report.CreatedAt, &report.UpdatedAt,
		&report.ResolvedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ModerationReportCreation{}, ErrModerationNotFound
	}
	if err != nil {
		return ModerationReportCreation{}, err
	}
	report.ID = ulid.NewModerationReportID(time.UnixMilli(params.CreatedAt))
	if _, err := tx.Exec(`INSERT INTO moderation_reports(
  id, reporter_orbit_id, reporter_actor_id, media_id,
  reported_orbit_id, reported_actor_id, media_kind, media_source,
  media_title, media_duration_ms, transmission_id,
  target_orbit_id, target_actor_id, target_slot, audience_kind,
  playback_domain_kind, playback_domain_id, accepted_at, reason_code, details,
  evidence_storage_key, evidence_sha256, evidence_size_bytes, evidence_mime,
  evidence_expires_at, created_at, updated_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		report.ID, report.ReporterOrbitID, report.ReporterActorID, report.MediaID,
		report.ReportedOrbitID, report.ReportedActorID, report.MediaKind,
		report.MediaSource, report.MediaTitle, report.MediaDurationMS,
		report.TransmissionID,
		report.TargetOrbitID, report.TargetActorID, report.TargetSlot,
		report.AudienceKind, report.PlaybackDomainKind, report.PlaybackDomainID,
		report.AcceptedAt, report.Reason, report.Details,
		report.EvidenceStorageKey, report.EvidenceSHA256,
		report.EvidenceSizeBytes, report.EvidenceMIME,
		report.EvidenceExpiresAt, report.CreatedAt, report.UpdatedAt,
	); err != nil {
		return ModerationReportCreation{}, err
	}
	if _, err := tx.Exec(`INSERT INTO moderation_report_attempts(
  reporter_orbit_id, reporter_actor_id, allowed, created_at
) VALUES(?, ?, 1, ?)`, ctx.OrbitID, ctx.ActorID, params.CreatedAt); err != nil {
		return ModerationReportCreation{}, err
	}
	if _, err := tx.Exec(`INSERT INTO moderation_audit_events(
  report_id, actor_id, event_type, created_at
) VALUES(?, ?, 'report.created', ?)`, report.ID, ctx.ActorID, params.CreatedAt); err != nil {
		return ModerationReportCreation{}, err
	}
	if err := s.checkpoint("moderation_report_create_before_commit"); err != nil {
		return ModerationReportCreation{}, err
	}
	if err := tx.Commit(); err != nil {
		return ModerationReportCreation{}, err
	}
	return ModerationReportCreation{Report: report}, nil
}

func (s *Store) GetAuthorizedModerationReport(
	expectedActorID int64,
	bearer, reportID string,
) (ModerationReport, error) {
	if expectedActorID <= 0 || !lowerHexTokenPattern.MatchString(bearer) ||
		!moderationReportIDPattern.MatchString(reportID) {
		return ModerationReport{}, ErrModerationNotFound
	}
	tx, err := s.db.Begin()
	if err != nil {
		return ModerationReport{}, err
	}
	defer tx.Rollback()
	ctx, err := mutationActorContextTx(tx, expectedActorID, hashToken(bearer))
	if err != nil {
		return ModerationReport{}, err
	}
	report, err := scanModerationReport(tx.QueryRow(
		`SELECT `+moderationReportColumns+` FROM moderation_reports
WHERE id = ? AND reporter_actor_id = ? AND reporter_orbit_id = ?`,
		reportID, ctx.ActorID, ctx.OrbitID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return ModerationReport{}, ErrModerationNotFound
	}
	if err != nil {
		return ModerationReport{}, err
	}
	return report, tx.Commit()
}

// CreateAuthorizedModerationReportBlock rechecks the exact control bearer,
// report ownership and canonical recipient-block policy in one writer
// transaction. A rotated credential cannot win between those checks.
func (s *Store) CreateAuthorizedModerationReportBlock(
	expectedActorID int64,
	bearer, reportID string,
	now int64,
) (ModerationReportBlock, error) {
	if expectedActorID <= 0 || !lowerHexTokenPattern.MatchString(bearer) ||
		!moderationReportIDPattern.MatchString(reportID) || now <= 0 {
		return ModerationReportBlock{}, ErrModerationNotFound
	}
	tx, err := s.db.Begin()
	if err != nil {
		return ModerationReportBlock{}, err
	}
	defer tx.Rollback()
	ctx, err := mutationActorContextTx(tx, expectedActorID, hashToken(bearer))
	if err != nil {
		return ModerationReportBlock{}, err
	}
	report, err := scanModerationReport(tx.QueryRow(
		`SELECT `+moderationReportColumns+` FROM moderation_reports
WHERE id = ? AND reporter_actor_id = ? AND reporter_orbit_id = ?`,
		reportID, ctx.ActorID, ctx.OrbitID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return ModerationReportBlock{}, ErrModerationNotFound
	}
	if err != nil {
		return ModerationReportBlock{}, err
	}
	params := CreateTransmissionBlockParams{
		OwnerScope: BlockOwnerActor, OwnerOrbitID: report.ReporterOrbitID,
		OwnerActorID:        report.ReporterActorID,
		BlockedKind:         BlockedSubjectActor,
		BlockedActorID:      report.ReportedActorID,
		AuthorizedByActorID: report.ReporterActorID,
		CreatedAt:           now,
	}
	if err := validateCreateTransmissionBlock(params); err != nil {
		return ModerationReportBlock{}, err
	}
	block, err := createTransmissionBlockTx(tx, params)
	if err != nil {
		return ModerationReportBlock{}, err
	}
	if !block.Reused {
		if _, err := tx.Exec(`INSERT INTO moderation_audit_events(
  report_id, actor_id, event_type, created_at
) VALUES(?, ?, 'report.blocked_sender', ?)`, report.ID,
			report.ReporterActorID, now); err != nil {
			return ModerationReportBlock{}, err
		}
	}
	if err := s.checkpoint("moderation_report_block_before_commit"); err != nil {
		return ModerationReportBlock{}, err
	}
	if err := tx.Commit(); err != nil {
		return ModerationReportBlock{}, err
	}
	return ModerationReportBlock{Report: report, Block: block}, nil
}

func (s *Store) ListModerationReports(
	operatorID, token, status string,
	limit int,
) ([]ModerationReport, error) {
	if !moderationOperatorIDPattern.MatchString(operatorID) ||
		(status != "" && status != "open" && status != "resolved") ||
		limit <= 0 || limit > 100 {
		return nil, ErrModerationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	operator, err := resolveModerationOperator(tx, token)
	if err != nil {
		return nil, err
	}
	if operator.ID != operatorID || !operator.Capabilities.List {
		return nil, ErrModerationForbidden
	}
	query := `SELECT ` + moderationReportColumns + ` FROM moderation_reports`
	args := []any{}
	if status != "" {
		query += ` WHERE status = ?`
		args = append(args, status)
	}
	query += ` ORDER BY created_at, id LIMIT ?`
	args = append(args, limit)
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	reports := make([]ModerationReport, 0)
	for rows.Next() {
		report, err := scanModerationReport(rows)
		if err != nil {
			return nil, err
		}
		reports = append(reports, report)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return reports, nil
}

func (s *Store) AuthorizeModerationEvidence(
	operatorID, token, reportID string,
	now int64,
) (ModerationEvidence, error) {
	if !moderationOperatorIDPattern.MatchString(operatorID) ||
		!moderationReportIDPattern.MatchString(reportID) || now <= 0 {
		return ModerationEvidence{}, ErrModerationNotFound
	}
	tx, err := s.db.Begin()
	if err != nil {
		return ModerationEvidence{}, err
	}
	defer tx.Rollback()
	operator, err := resolveModerationOperator(tx, token)
	if err != nil {
		return ModerationEvidence{}, err
	}
	if operator.ID != operatorID || !operator.Capabilities.Evidence {
		return ModerationEvidence{}, ErrModerationForbidden
	}
	var evidence ModerationEvidence
	err = tx.QueryRow(`SELECT id, media_id, evidence_storage_key,
evidence_sha256, evidence_size_bytes, evidence_mime, evidence_expires_at
FROM moderation_reports WHERE id = ?`, reportID).Scan(
		&evidence.ReportID, &evidence.MediaID, &evidence.StorageKey,
		&evidence.SHA256, &evidence.SizeBytes, &evidence.MIME,
		&evidence.ExpiresAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return ModerationEvidence{}, ErrModerationNotFound
	}
	if err != nil {
		return ModerationEvidence{}, err
	}
	if evidence.ExpiresAt <= now {
		return ModerationEvidence{}, ErrModerationEvidenceExpired
	}
	if _, err := tx.Exec(`INSERT INTO moderation_audit_events(
  report_id, operator_id, event_type, created_at
) VALUES(?, ?, 'evidence.read', ?)`, reportID, operatorID, now); err != nil {
		return ModerationEvidence{}, err
	}
	if err := tx.Commit(); err != nil {
		return ModerationEvidence{}, err
	}
	return evidence, nil
}

func (s *Store) BeginModerationDecision(
	operatorID, token, reportID string,
	action ModerationAction,
	now int64,
) (ModerationDecisionRequest, error) {
	if !moderationOperatorIDPattern.MatchString(operatorID) ||
		!moderationReportIDPattern.MatchString(reportID) ||
		!validModerationAction(action) || now <= 0 {
		return ModerationDecisionRequest{}, ErrModerationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return ModerationDecisionRequest{}, err
	}
	defer tx.Rollback()
	operator, err := resolveModerationOperator(tx, token)
	if err != nil {
		return ModerationDecisionRequest{}, err
	}
	if operator.ID != operatorID || !operator.Capabilities.Decide {
		return ModerationDecisionRequest{}, ErrModerationForbidden
	}
	report, err := scanModerationReport(tx.QueryRow(
		`SELECT `+moderationReportColumns+` FROM moderation_reports WHERE id = ?`, reportID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return ModerationDecisionRequest{}, ErrModerationNotFound
	}
	if err != nil {
		return ModerationDecisionRequest{}, err
	}
	existing, err := scanModerationDecision(tx.QueryRow(
		`SELECT `+moderationDecisionColumns+` FROM moderation_decisions WHERE report_id = ?`, reportID,
	))
	if err == nil {
		if existing.Action != action {
			return ModerationDecisionRequest{}, ErrModerationDecisionConflict
		}
		if err := tx.Commit(); err != nil {
			return ModerationDecisionRequest{}, err
		}
		return ModerationDecisionRequest{
			Decision: existing, Report: report, Reused: true,
			Applied: existing.State == "applied",
		}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return ModerationDecisionRequest{}, err
	}
	decision := ModerationDecision{
		ID:       ulid.NewModerationDecisionID(time.UnixMilli(now)),
		ReportID: reportID, Action: action, State: "pending",
		RequestedByOperatorID: operatorID, RequestedAt: now,
	}
	if _, err := tx.Exec(`INSERT INTO moderation_decisions(
  id, report_id, action, requested_by_operator_id, requested_at
) VALUES(?, ?, ?, ?, ?)`, decision.ID, decision.ReportID, decision.Action,
		decision.RequestedByOperatorID, decision.RequestedAt); err != nil {
		return ModerationDecisionRequest{}, err
	}
	if _, err := tx.Exec(`INSERT INTO moderation_audit_events(
  report_id, operator_id, event_type, action, created_at
) VALUES(?, ?, 'decision.requested', ?, ?)`, reportID, operatorID, action, now); err != nil {
		return ModerationDecisionRequest{}, err
	}
	if err := s.checkpoint("moderation_decision_begin_before_commit"); err != nil {
		return ModerationDecisionRequest{}, err
	}
	if err := tx.Commit(); err != nil {
		return ModerationDecisionRequest{}, err
	}
	return ModerationDecisionRequest{Decision: decision, Report: report}, nil
}

func (s *Store) CompleteModerationDecision(
	decisionID string,
	now int64,
) (ModerationDecision, error) {
	if !moderationDecisionIDPattern.MatchString(decisionID) || now <= 0 {
		return ModerationDecision{}, ErrModerationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return ModerationDecision{}, err
	}
	defer tx.Rollback()
	decision, err := scanModerationDecision(tx.QueryRow(
		`SELECT `+moderationDecisionColumns+` FROM moderation_decisions WHERE id = ?`, decisionID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return ModerationDecision{}, ErrModerationNotFound
	}
	if err != nil {
		return ModerationDecision{}, err
	}
	if decision.State == "applied" {
		if err := tx.Commit(); err != nil {
			return ModerationDecision{}, err
		}
		return decision, nil
	}
	if now < decision.RequestedAt {
		return ModerationDecision{}, ErrModerationDecisionConflict
	}
	result, err := tx.Exec(`UPDATE moderation_decisions
SET state = 'applied', applied_at = ? WHERE id = ? AND state = 'pending'`,
		now, decisionID)
	if err != nil {
		return ModerationDecision{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return ModerationDecision{}, err
		}
		return ModerationDecision{}, ErrModerationDecisionConflict
	}
	if _, err := tx.Exec(`UPDATE moderation_reports
SET status = 'resolved', updated_at = ?, resolved_at = ?
WHERE id = ? AND status = 'open'`, now, now, decision.ReportID); err != nil {
		return ModerationDecision{}, err
	}
	if _, err := tx.Exec(`INSERT INTO moderation_audit_events(
  report_id, operator_id, event_type, action, created_at
) VALUES(?, ?, 'decision.applied', ?, ?)`, decision.ReportID,
		decision.RequestedByOperatorID, decision.Action, now); err != nil {
		return ModerationDecision{}, err
	}
	decision, err = scanModerationDecision(tx.QueryRow(
		`SELECT `+moderationDecisionColumns+` FROM moderation_decisions WHERE id = ?`, decisionID,
	))
	if err != nil {
		return ModerationDecision{}, err
	}
	if err := s.checkpoint("moderation_decision_complete_before_commit"); err != nil {
		return ModerationDecision{}, err
	}
	if err := tx.Commit(); err != nil {
		return ModerationDecision{}, err
	}
	return decision, nil
}

func (s *Store) GetModerationDecision(reportID string) (*ModerationDecision, error) {
	if !moderationReportIDPattern.MatchString(reportID) {
		return nil, ErrModerationNotFound
	}
	decision, err := scanModerationDecision(s.db.QueryRow(
		`SELECT `+moderationDecisionColumns+` FROM moderation_decisions WHERE report_id = ?`, reportID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &decision, nil
}

// ModerationAuditCount is intentionally narrow: it supports health/tests
// without exposing request details or credential material.
func (s *Store) ModerationAuditCount(reportID, eventType string) (int, error) {
	var count int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM moderation_audit_events
WHERE (? = '' OR report_id = ?) AND event_type = ?`, reportID, reportID, eventType).Scan(&count)
	return count, err
}

// PruneModerationRetention removes free-form reporter text when the evidence
// window closes and bounds rate-control state. Immutable target/evidence
// metadata and content-free audit rows remain for attributable review.
func (s *Store) PruneModerationRetention(now int64) (scrubbed, attempts int64, err error) {
	if now <= 0 {
		return 0, 0, ErrModerationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE moderation_reports
SET details = '', updated_at = CASE WHEN updated_at < ? THEN ? ELSE updated_at END
WHERE evidence_expires_at <= ? AND details <> ''`, now, now, now)
	if err != nil {
		return 0, 0, err
	}
	scrubbed, err = result.RowsAffected()
	if err != nil {
		return 0, 0, err
	}
	result, err = tx.Exec(`DELETE FROM moderation_report_attempts WHERE created_at <= ?`,
		now-int64((24*time.Hour)/time.Millisecond))
	if err != nil {
		return 0, 0, err
	}
	attempts, err = result.RowsAffected()
	if err != nil {
		return 0, 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return scrubbed, attempts, nil
}
