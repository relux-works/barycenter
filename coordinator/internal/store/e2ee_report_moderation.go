package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"relux.works/duet/coordinator/internal/ulid"
)

const (
	e2eeReportRetention    = 30 * 24 * time.Hour
	E2EEReportEvidenceMIME = "application/vnd.barycenter.report-evidence+octet-stream"
)

var (
	ErrE2EEReportNotFound            = errors.New("E2EE moderation report was not found")
	ErrE2EEReportEvidenceUnavailable = errors.New("E2EE report evidence is unavailable")
)

type E2EEModerationReport struct {
	ID, ProtectedObjectID, GroupID                    string
	ReporterDeviceID, ReportedDeviceID                string
	ObjectKind, SourceObjectID                        string
	TargetSnapshotDigest, ManifestDigest              string
	CiphertextDigest, Status, EvidenceState           string
	Reason                                            ModerationReason
	Statement                                         string
	ReporterActorID, ReportedOrbitID, ReportedActorID int64
	Epoch, Generation, StatementExpiresAt             int64
	Revision, CreatedAt, UpdatedAt, ResolvedAt        int64
}

type CreateE2EEModerationReportParams struct {
	ProtectedObjectID, ReporterDeviceID string
	ReporterActorID                     int64
	Reason                              ModerationReason
	Statement                           string
	CreatedAt                           int64
}

type E2EEModerationReportCreation struct {
	Report E2EEModerationReport
	Reused bool
}

type E2EEReportEvidence struct {
	ID, ReportID, ConsentID, ProtectedObjectID   string
	ReporterDeviceID, ConsentVersion             string
	ConsentDigest, ManifestDigest                string
	AuthenticatedEvidenceDigest                  string
	EncryptedEvidenceRef, AtRestCiphertextDigest string
	MIME, Status                                 string
	ReporterActorID, SizeBytes                   int64
	ConsentConfirmedAt, ExpiresAt                int64
	Revision, CreatedAt, UpdatedAt, TerminalAt   int64
}

type E2EEReportEvidenceCreation struct {
	Evidence E2EEReportEvidence
	Reused   bool
}

type E2EEReportEvidenceDeletion struct {
	Evidence E2EEReportEvidence
	Changed  bool
}

type E2EEReportAuditEvent struct {
	ID         int64  `json:"id"`
	ReportID   string `json:"report_id"`
	EvidenceID string `json:"evidence_id,omitempty"`
	OperatorID string `json:"operator_id,omitempty"`
	ActorID    int64  `json:"actor_id,omitempty"`
	DeviceID   string `json:"device_id,omitempty"`
	EventType  string `json:"event_type"`
	Action     string `json:"action,omitempty"`
	CreatedAt  int64  `json:"created_at"`
}

type E2EEModerationDecision struct {
	ID, ReportID, State, RequestedByOperatorID string
	Action                                     ModerationAction
	RequestedAt, AppliedAt                     int64
}

type E2EEModerationDecisionRequest struct {
	Decision E2EEModerationDecision
	Report   E2EEModerationReport
	Reused   bool
	Applied  bool
}

const e2eeModerationReportColumns = `id, protected_object_id, group_id,
reporter_actor_id, reporter_device_id, reported_orbit_id, reported_actor_id,
reported_device_id, object_kind, source_object_id, epoch, generation,
target_snapshot_digest, manifest_digest, ciphertext_digest, reason_code,
statement, statement_expires_at, status, evidence_state, revision, created_at,
updated_at, resolved_at`

func scanE2EEModerationReport(row sqlScanner) (E2EEModerationReport, error) {
	var value E2EEModerationReport
	err := row.Scan(&value.ID, &value.ProtectedObjectID, &value.GroupID,
		&value.ReporterActorID, &value.ReporterDeviceID, &value.ReportedOrbitID,
		&value.ReportedActorID, &value.ReportedDeviceID, &value.ObjectKind,
		&value.SourceObjectID, &value.Epoch, &value.Generation,
		&value.TargetSnapshotDigest, &value.ManifestDigest,
		&value.CiphertextDigest, &value.Reason, &value.Statement,
		&value.StatementExpiresAt, &value.Status, &value.EvidenceState,
		&value.Revision, &value.CreatedAt, &value.UpdatedAt, &value.ResolvedAt)
	return value, err
}

func validE2EEModerationReportID(value string) bool {
	return len(value) == 30 && strings.HasPrefix(value, "erp_")
}

func validE2EEReportEvidenceID(value string) bool {
	return len(value) == 30 && strings.HasPrefix(value, "ere_")
}

func appendE2EEReportAuditTx(tx *sql.Tx, reportID, evidenceID, operatorID string,
	actorID int64, deviceID, eventType string, action ModerationAction, now int64,
) error {
	var evidence, operator, actor any
	if evidenceID != "" {
		evidence = evidenceID
	}
	if operatorID != "" {
		operator = operatorID
	}
	if actorID != 0 {
		actor = actorID
	}
	_, err := tx.Exec(`INSERT INTO e2ee_report_audit_events(
report_id, evidence_id, operator_id, actor_id, device_id, event_type, action, created_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, reportID, evidence, operator, actor,
		deviceID, eventType, action, now)
	return err
}

func (s *Store) CreateE2EEModerationReport(
	params CreateE2EEModerationReportParams,
) (E2EEModerationReportCreation, error) {
	params.Statement = strings.TrimSpace(params.Statement)
	if len(params.ProtectedObjectID) != 29 || len(params.ReporterDeviceID) < 8 ||
		len(params.ReporterDeviceID) > 128 || params.ReporterActorID <= 0 ||
		!validModerationReason(params.Reason) || params.CreatedAt <= 0 ||
		len(params.Statement) > 2000 || !utf8.ValidString(params.Statement) {
		return E2EEModerationReportCreation{}, ErrModerationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEModerationReportCreation{}, err
	}
	defer tx.Rollback()
	existing, err := scanE2EEModerationReport(tx.QueryRow(
		`SELECT `+e2eeModerationReportColumns+` FROM e2ee_moderation_reports
WHERE reporter_actor_id = ? AND protected_object_id = ?`,
		params.ReporterActorID, params.ProtectedObjectID))
	if err == nil {
		if err := tx.Commit(); err != nil {
			return E2EEModerationReportCreation{}, err
		}
		return E2EEModerationReportCreation{Report: existing, Reused: true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return E2EEModerationReportCreation{}, err
	}
	object, err := scanE2EEProtectedObject(tx.QueryRow(
		`SELECT `+e2eeProtectedObjectColumns+` FROM e2ee_protected_objects WHERE id = ?`,
		params.ProtectedObjectID))
	if errors.Is(err, sql.ErrNoRows) {
		return E2EEModerationReportCreation{}, ErrE2EEReportNotFound
	}
	if err != nil {
		return E2EEModerationReportCreation{}, err
	}
	if object.Status == "staged" || object.FinalizedAt == 0 {
		return E2EEModerationReportCreation{}, ErrE2EEReportNotFound
	}
	var recipientCount int
	if err := tx.QueryRow(`SELECT COUNT(*)
FROM e2ee_protected_object_recipients r
JOIN actors a ON a.id = r.actor_id AND a.revoked_at IS NULL
WHERE r.protected_object_id = ? AND r.recipient_device_id = ? AND r.actor_id = ?`,
		object.ID, params.ReporterDeviceID, params.ReporterActorID).Scan(&recipientCount); err != nil {
		return E2EEModerationReportCreation{}, err
	}
	if recipientCount != 1 {
		return E2EEModerationReportCreation{}, ErrE2EEReportNotFound
	}
	var reportedActorID, reportedOrbitID int64
	if err := tx.QueryRow(`SELECT d.actor_id, gm.orbit_id
FROM e2ee_device_public_state d
JOIN e2ee_group_members gm ON gm.group_id = ? AND gm.device_id = d.device_id
  AND gm.actor_id = d.actor_id
WHERE d.device_id = ?`, object.GroupID, object.AuthorDeviceID).Scan(
		&reportedActorID, &reportedOrbitID); err != nil {
		return E2EEModerationReportCreation{}, ErrE2EEReportNotFound
	}
	if reportedActorID == params.ReporterActorID {
		return E2EEModerationReportCreation{}, ErrModerationInvalid
	}
	value := E2EEModerationReport{
		ID:                "erp_" + ulid.New(time.UnixMilli(params.CreatedAt)),
		ProtectedObjectID: object.ID, GroupID: object.GroupID,
		ReporterActorID: params.ReporterActorID, ReporterDeviceID: params.ReporterDeviceID,
		ReportedOrbitID: reportedOrbitID, ReportedActorID: reportedActorID,
		ReportedDeviceID: object.AuthorDeviceID, ObjectKind: object.ObjectKind,
		SourceObjectID: object.SourceObjectID, Epoch: object.Epoch,
		Generation: object.Generation, TargetSnapshotDigest: object.TargetSnapshotDigest,
		ManifestDigest: object.ManifestDigest, CiphertextDigest: object.CiphertextDigest,
		Reason: params.Reason, Statement: params.Statement,
		StatementExpiresAt: params.CreatedAt + e2eeReportRetention.Milliseconds(),
		Status:             "open", EvidenceState: "metadata_only", Revision: 1,
		CreatedAt: params.CreatedAt, UpdatedAt: params.CreatedAt,
	}
	if _, err := tx.Exec(`INSERT INTO e2ee_moderation_reports(
id, protected_object_id, group_id, reporter_actor_id, reporter_device_id,
reported_orbit_id, reported_actor_id, reported_device_id, object_kind,
source_object_id, epoch, generation, target_snapshot_digest, manifest_digest,
ciphertext_digest, reason_code, statement, statement_expires_at, status,
evidence_state, revision, created_at, updated_at, resolved_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'open',
'metadata_only', 1, ?, ?, 0)`, value.ID, value.ProtectedObjectID, value.GroupID,
		value.ReporterActorID, value.ReporterDeviceID, value.ReportedOrbitID,
		value.ReportedActorID, value.ReportedDeviceID, value.ObjectKind,
		value.SourceObjectID, value.Epoch, value.Generation,
		value.TargetSnapshotDigest, value.ManifestDigest, value.CiphertextDigest,
		value.Reason, value.Statement, value.StatementExpiresAt,
		value.CreatedAt, value.UpdatedAt); err != nil {
		return E2EEModerationReportCreation{}, ErrE2EEConflict
	}
	if err := appendE2EEReportAuditTx(tx, value.ID, "", "",
		value.ReporterActorID, value.ReporterDeviceID, "report.created", "",
		params.CreatedAt); err != nil {
		return E2EEModerationReportCreation{}, err
	}
	if err := appendE2EEAuditTx(tx, value.GroupID, "report_evidence", value.ID,
		"report.metadata.create", "accepted", "metadata_only",
		value.ReporterActorID, value.ReporterDeviceID, value.Epoch, value.Revision,
		params.CreatedAt); err != nil {
		return E2EEModerationReportCreation{}, err
	}
	if err := s.checkpoint("e2ee_report_metadata_before_commit"); err != nil {
		return E2EEModerationReportCreation{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEModerationReportCreation{}, err
	}
	return E2EEModerationReportCreation{Report: value}, nil
}

func (s *Store) GetE2EEModerationReport(id string) (E2EEModerationReport, error) {
	if !validE2EEModerationReportID(id) {
		return E2EEModerationReport{}, ErrE2EEReportNotFound
	}
	value, err := scanE2EEModerationReport(s.db.QueryRow(
		`SELECT `+e2eeModerationReportColumns+` FROM e2ee_moderation_reports WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return E2EEModerationReport{}, ErrE2EEReportNotFound
	}
	return value, err
}

const e2eeReportEvidenceColumns = `m.id, m.report_id, c.id,
m.protected_object_id, m.reporter_actor_id, c.reporter_device_id,
m.consent_version, m.consent_digest, c.manifest_digest,
m.authenticated_evidence_digest, m.encrypted_evidence_ref,
s.at_rest_ciphertext_digest, s.evidence_size_bytes, s.evidence_mime,
s.status, c.confirmed_at, m.retention_expires_at, s.revision, s.created_at,
s.updated_at, s.terminal_at`

func scanE2EEReportEvidence(row sqlScanner) (E2EEReportEvidence, error) {
	var value E2EEReportEvidence
	err := row.Scan(&value.ID, &value.ReportID, &value.ConsentID,
		&value.ProtectedObjectID, &value.ReporterActorID, &value.ReporterDeviceID,
		&value.ConsentVersion, &value.ConsentDigest, &value.ManifestDigest,
		&value.AuthenticatedEvidenceDigest, &value.EncryptedEvidenceRef,
		&value.AtRestCiphertextDigest, &value.SizeBytes, &value.MIME,
		&value.Status, &value.ConsentConfirmedAt, &value.ExpiresAt,
		&value.Revision, &value.CreatedAt, &value.UpdatedAt, &value.TerminalAt)
	return value, err
}

func e2eeReportEvidenceQuery(suffix string) string {
	return `SELECT ` + e2eeReportEvidenceColumns + `
FROM e2ee_report_evidence_metadata m
JOIN e2ee_report_evidence_state s ON s.evidence_id = m.id
JOIN e2ee_report_evidence_consents c ON c.id = s.consent_id
` + suffix
}

func (s *Store) AttachE2EEReportEvidence(
	params CreateE2EEReportEvidenceParams,
) (E2EEReportEvidenceCreation, error) {
	if !validE2EEModerationReportID(params.ReportID) || len(params.ProtectedObjectID) != 29 ||
		params.ReporterActorID <= 0 || len(params.ReporterDeviceID) < 8 ||
		len(params.ReporterDeviceID) > 128 || params.ExpectedReportRevision <= 0 ||
		len(params.ConsentVersion) == 0 || len(params.ConsentVersion) > 128 ||
		!validE2EEDigest(params.ConsentDigest) || !validE2EEDigest(params.AuthenticatedEvidenceDigest) ||
		!validE2EEDigest(params.AtRestCiphertextDigest) ||
		params.EncryptedEvidenceRef != "evidence/v1/"+params.AtRestCiphertextDigest ||
		params.EvidenceSizeBytes <= 0 || params.EvidenceSizeBytes > 64<<20 ||
		params.EvidenceMIME != E2EEReportEvidenceMIME ||
		params.ConsentConfirmedAt <= 0 || params.CreatedAt < params.ConsentConfirmedAt ||
		params.RetentionExpiresAt <= params.CreatedAt ||
		params.RetentionExpiresAt > params.CreatedAt+e2eeReportRetention.Milliseconds() {
		return E2EEReportEvidenceCreation{}, ErrModerationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEReportEvidenceCreation{}, err
	}
	defer tx.Rollback()
	existing, err := scanE2EEReportEvidence(tx.QueryRow(e2eeReportEvidenceQuery(
		"WHERE m.report_id = ?"), params.ReportID))
	if err == nil {
		if existing.ProtectedObjectID != params.ProtectedObjectID ||
			existing.ReporterActorID != params.ReporterActorID ||
			existing.ReporterDeviceID != params.ReporterDeviceID ||
			existing.ConsentVersion != params.ConsentVersion ||
			existing.ConsentDigest != params.ConsentDigest ||
			existing.AuthenticatedEvidenceDigest != params.AuthenticatedEvidenceDigest ||
			existing.EncryptedEvidenceRef != params.EncryptedEvidenceRef ||
			existing.AtRestCiphertextDigest != params.AtRestCiphertextDigest ||
			existing.SizeBytes != params.EvidenceSizeBytes || existing.MIME != params.EvidenceMIME {
			return E2EEReportEvidenceCreation{}, ErrE2EEConflict
		}
		if err := tx.Commit(); err != nil {
			return E2EEReportEvidenceCreation{}, err
		}
		return E2EEReportEvidenceCreation{Evidence: existing, Reused: true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return E2EEReportEvidenceCreation{}, err
	}
	report, err := scanE2EEModerationReport(tx.QueryRow(
		`SELECT `+e2eeModerationReportColumns+` FROM e2ee_moderation_reports WHERE id = ?`,
		params.ReportID))
	if errors.Is(err, sql.ErrNoRows) {
		return E2EEReportEvidenceCreation{}, ErrE2EEReportNotFound
	}
	if err != nil {
		return E2EEReportEvidenceCreation{}, err
	}
	if report.ProtectedObjectID != params.ProtectedObjectID ||
		report.ReporterActorID != params.ReporterActorID ||
		report.ReporterDeviceID != params.ReporterDeviceID ||
		report.Revision != params.ExpectedReportRevision || report.Status != "open" ||
		report.EvidenceState != "metadata_only" || params.ConsentConfirmedAt < report.CreatedAt {
		return E2EEReportEvidenceCreation{}, ErrE2EEConflict
	}
	object, err := scanE2EEProtectedObject(tx.QueryRow(
		`SELECT `+e2eeProtectedObjectColumns+` FROM e2ee_protected_objects WHERE id = ?`,
		report.ProtectedObjectID))
	if err != nil {
		return E2EEReportEvidenceCreation{}, ErrE2EEReportNotFound
	}
	if object.ManifestDigest != report.ManifestDigest || object.TargetSnapshotDigest != report.TargetSnapshotDigest ||
		object.Epoch != report.Epoch || object.Generation != report.Generation {
		return E2EEReportEvidenceCreation{}, ErrE2EEConflict
	}
	if err := authorizeE2EEProtectedRecipientTx(tx, object, report.ReporterDeviceID); err != nil {
		return E2EEReportEvidenceCreation{}, ErrE2EERevoked
	}
	consentID := "erc_" + ulid.New(time.UnixMilli(params.ConsentConfirmedAt))
	evidenceID := "ere_" + ulid.New(time.UnixMilli(params.CreatedAt))
	if _, err := tx.Exec(`INSERT INTO e2ee_report_evidence_consents(
id, report_id, protected_object_id, reporter_actor_id, reporter_device_id,
consent_version, consent_digest, manifest_digest, authenticated_evidence_digest,
action, confirmed_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, 'explicit_report_evidence_export', ?)`,
		consentID, report.ID, report.ProtectedObjectID, report.ReporterActorID,
		report.ReporterDeviceID, params.ConsentVersion, params.ConsentDigest,
		report.ManifestDigest, params.AuthenticatedEvidenceDigest,
		params.ConsentConfirmedAt); err != nil {
		return E2EEReportEvidenceCreation{}, ErrE2EEConflict
	}
	if _, err := tx.Exec(`INSERT INTO e2ee_report_evidence_metadata(
id, report_id, protected_object_id, reporter_actor_id, consent_version,
consent_digest, authenticated_evidence_digest, encrypted_evidence_ref,
retention_expires_at, created_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, evidenceID, report.ID,
		report.ProtectedObjectID, report.ReporterActorID, params.ConsentVersion,
		params.ConsentDigest, params.AuthenticatedEvidenceDigest,
		params.EncryptedEvidenceRef, params.RetentionExpiresAt, params.CreatedAt); err != nil {
		return E2EEReportEvidenceCreation{}, ErrE2EEConflict
	}
	if _, err := tx.Exec(`INSERT INTO e2ee_report_evidence_state(
evidence_id, report_id, consent_id, at_rest_ciphertext_digest,
evidence_size_bytes, evidence_mime, status, revision, created_at, updated_at, terminal_at
) VALUES(?, ?, ?, ?, ?, ?, 'active', 1, ?, ?, 0)`, evidenceID, report.ID,
		consentID, params.AtRestCiphertextDigest, params.EvidenceSizeBytes,
		params.EvidenceMIME, params.CreatedAt, params.CreatedAt); err != nil {
		return E2EEReportEvidenceCreation{}, ErrE2EEConflict
	}
	result, err := tx.Exec(`UPDATE e2ee_moderation_reports
SET evidence_state = 'provided', revision = revision + 1, updated_at = ?
WHERE id = ? AND revision = ? AND status = 'open' AND evidence_state = 'metadata_only'`,
		params.CreatedAt, report.ID, report.Revision)
	if err != nil {
		return E2EEReportEvidenceCreation{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return E2EEReportEvidenceCreation{}, err
		}
		return E2EEReportEvidenceCreation{}, ErrE2EEConflict
	}
	for _, eventType := range []string{"evidence.consent_recorded", "evidence.created"} {
		if err := appendE2EEReportAuditTx(tx, report.ID, evidenceID, "",
			report.ReporterActorID, report.ReporterDeviceID, eventType, "",
			params.CreatedAt); err != nil {
			return E2EEReportEvidenceCreation{}, err
		}
	}
	if err := appendE2EEAuditTx(tx, report.GroupID, "report_evidence", evidenceID,
		"report_evidence.record", "accepted", "explicit_consent_moderation_at_rest",
		report.ReporterActorID, report.ReporterDeviceID, report.Epoch,
		report.Revision+1, params.CreatedAt); err != nil {
		return E2EEReportEvidenceCreation{}, err
	}
	if err := s.checkpoint("e2ee_report_evidence_before_commit"); err != nil {
		return E2EEReportEvidenceCreation{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEReportEvidenceCreation{}, err
	}
	value, err := s.getE2EEReportEvidence(report.ID)
	return E2EEReportEvidenceCreation{Evidence: value}, err
}

func (s *Store) getE2EEReportEvidence(reportID string) (E2EEReportEvidence, error) {
	value, err := scanE2EEReportEvidence(s.db.QueryRow(e2eeReportEvidenceQuery(
		"WHERE m.report_id = ?"), reportID))
	if errors.Is(err, sql.ErrNoRows) {
		return E2EEReportEvidence{}, ErrE2EEReportEvidenceUnavailable
	}
	return value, err
}

func markE2EEReportEvidenceTerminalTx(tx *sql.Tx, reportID, status, eventType,
	operatorID string, now int64,
) (E2EEReportEvidence, bool, error) {
	value, err := scanE2EEReportEvidence(tx.QueryRow(e2eeReportEvidenceQuery(
		"WHERE m.report_id = ?"), reportID))
	if errors.Is(err, sql.ErrNoRows) {
		return E2EEReportEvidence{}, false, ErrE2EEReportEvidenceUnavailable
	}
	if err != nil {
		return E2EEReportEvidence{}, false, err
	}
	if value.Status != "active" {
		return value, false, nil
	}
	result, err := tx.Exec(`UPDATE e2ee_report_evidence_state
SET status = ?, revision = revision + 1, updated_at = ?, terminal_at = ?
WHERE evidence_id = ? AND status = 'active' AND revision = ?`, status, now, now,
		value.ID, value.Revision)
	if err != nil {
		return E2EEReportEvidence{}, false, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return E2EEReportEvidence{}, false, err
		}
		return E2EEReportEvidence{}, false, ErrE2EEConflict
	}
	if _, err := tx.Exec(`UPDATE e2ee_moderation_reports
SET evidence_state = ?, revision = revision + 1, updated_at = ? WHERE id = ?`,
		status, now, reportID); err != nil {
		return E2EEReportEvidence{}, false, err
	}
	if err := appendE2EEReportAuditTx(tx, reportID, value.ID, operatorID,
		0, "", eventType, "", now); err != nil {
		return E2EEReportEvidence{}, false, err
	}
	value.Status, value.UpdatedAt, value.TerminalAt = status, now, now
	value.Revision++
	return value, true, nil
}

func (s *Store) AuthorizeE2EEReportEvidence(operatorID, token, reportID string,
	now int64,
) (E2EEReportEvidence, error) {
	if !moderationOperatorIDPattern.MatchString(operatorID) ||
		!validE2EEModerationReportID(reportID) || now <= 0 {
		return E2EEReportEvidence{}, ErrE2EEReportEvidenceUnavailable
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEReportEvidence{}, err
	}
	defer tx.Rollback()
	operator, err := resolveModerationOperator(tx, token)
	if err != nil {
		return E2EEReportEvidence{}, err
	}
	if operator.ID != operatorID || !operator.Capabilities.Evidence {
		return E2EEReportEvidence{}, ErrModerationForbidden
	}
	value, err := scanE2EEReportEvidence(tx.QueryRow(e2eeReportEvidenceQuery(
		"WHERE m.report_id = ?"), reportID))
	if errors.Is(err, sql.ErrNoRows) {
		return E2EEReportEvidence{}, ErrE2EEReportEvidenceUnavailable
	}
	if err != nil {
		return E2EEReportEvidence{}, err
	}
	if value.Status != "active" {
		return E2EEReportEvidence{}, ErrModerationEvidenceExpired
	}
	if value.ExpiresAt <= now {
		if _, _, err := markE2EEReportEvidenceTerminalTx(tx, reportID, "expired",
			"evidence.expired", "", now); err != nil {
			return E2EEReportEvidence{}, err
		}
		if err := tx.Commit(); err != nil {
			return E2EEReportEvidence{}, err
		}
		return E2EEReportEvidence{}, ErrModerationEvidenceExpired
	}
	if err := appendE2EEReportAuditTx(tx, reportID, value.ID, operatorID,
		0, "", "evidence.read", "", now); err != nil {
		return E2EEReportEvidence{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEReportEvidence{}, err
	}
	return value, nil
}

func (s *Store) DeleteE2EEReportEvidence(operatorID, token, reportID string,
	now int64,
) (E2EEReportEvidenceDeletion, error) {
	if !moderationOperatorIDPattern.MatchString(operatorID) ||
		!validE2EEModerationReportID(reportID) || now <= 0 {
		return E2EEReportEvidenceDeletion{}, ErrE2EEReportEvidenceUnavailable
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEReportEvidenceDeletion{}, err
	}
	defer tx.Rollback()
	operator, err := resolveModerationOperator(tx, token)
	if err != nil {
		return E2EEReportEvidenceDeletion{}, err
	}
	if operator.ID != operatorID || !operator.Capabilities.Evidence {
		return E2EEReportEvidenceDeletion{}, ErrModerationForbidden
	}
	value, changed, err := markE2EEReportEvidenceTerminalTx(tx, reportID,
		"deleted", "evidence.deleted", operatorID, now)
	if err != nil {
		return E2EEReportEvidenceDeletion{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEReportEvidenceDeletion{}, err
	}
	return E2EEReportEvidenceDeletion{Evidence: value, Changed: changed}, nil
}

func (s *Store) ExpireE2EEReportEvidence(now int64, limit int) ([]E2EEReportEvidence, error) {
	if now <= 0 || limit <= 0 || limit > 1000 {
		return nil, ErrModerationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT m.report_id
FROM e2ee_report_evidence_metadata m
JOIN e2ee_report_evidence_state s ON s.evidence_id = m.id
WHERE s.status = 'active' AND m.retention_expires_at <= ?
ORDER BY m.retention_expires_at, m.id LIMIT ?`, now, limit)
	if err != nil {
		return nil, err
	}
	var reportIDs []string
	for rows.Next() {
		var reportID string
		if err := rows.Scan(&reportID); err != nil {
			rows.Close()
			return nil, err
		}
		reportIDs = append(reportIDs, reportID)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	values := make([]E2EEReportEvidence, 0, len(reportIDs))
	for _, reportID := range reportIDs {
		value, changed, err := markE2EEReportEvidenceTerminalTx(tx, reportID,
			"expired", "evidence.expired", "", now)
		if err != nil {
			return nil, err
		}
		if changed {
			values = append(values, value)
		}
	}
	if _, err := tx.Exec(`UPDATE e2ee_moderation_reports
SET statement = '', updated_at = CASE WHEN updated_at < ? THEN ? ELSE updated_at END
WHERE statement_expires_at <= ? AND statement <> ''`, now, now, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return values, nil
}

func (s *Store) ListE2EEModerationReports(operatorID, token, status string,
	limit int,
) ([]E2EEModerationReport, error) {
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
	query := `SELECT ` + e2eeModerationReportColumns + ` FROM e2ee_moderation_reports`
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
	values := make([]E2EEModerationReport, 0)
	for rows.Next() {
		value, err := scanE2EEModerationReport(rows)
		if err != nil {
			return nil, err
		}
		values = append(values, value)
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
	return values, nil
}

func (s *Store) ListE2EEReportAuditEvents(operatorID, token, reportID string,
	limit int,
) ([]E2EEReportAuditEvent, error) {
	if !moderationOperatorIDPattern.MatchString(operatorID) ||
		!validE2EEModerationReportID(reportID) || limit <= 0 || limit > 500 {
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
	var reportExists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM e2ee_moderation_reports WHERE id = ?`,
		reportID).Scan(&reportExists); err != nil {
		return nil, err
	}
	if reportExists != 1 {
		return nil, ErrE2EEReportNotFound
	}
	rows, err := tx.Query(`SELECT id, report_id, evidence_id, operator_id,
actor_id, device_id, event_type, action, created_at
FROM e2ee_report_audit_events WHERE report_id = ? ORDER BY id LIMIT ?`, reportID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	values := make([]E2EEReportAuditEvent, 0)
	for rows.Next() {
		var value E2EEReportAuditEvent
		var evidence, operator sql.NullString
		var actor sql.NullInt64
		if err := rows.Scan(&value.ID, &value.ReportID, &evidence, &operator,
			&actor, &value.DeviceID, &value.EventType, &value.Action,
			&value.CreatedAt); err != nil {
			return nil, err
		}
		if evidence.Valid {
			value.EvidenceID = evidence.String
		}
		if operator.Valid {
			value.OperatorID = operator.String
		}
		if actor.Valid {
			value.ActorID = actor.Int64
		}
		values = append(values, value)
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
	return values, nil
}

const e2eeModerationDecisionColumns = `id, report_id, action, state,
requested_by_operator_id, requested_at, applied_at`

func scanE2EEModerationDecision(row sqlScanner) (E2EEModerationDecision, error) {
	var value E2EEModerationDecision
	err := row.Scan(&value.ID, &value.ReportID, &value.Action, &value.State,
		&value.RequestedByOperatorID, &value.RequestedAt, &value.AppliedAt)
	return value, err
}

func (s *Store) BeginE2EEModerationDecision(operatorID, token, reportID string,
	action ModerationAction, now int64,
) (E2EEModerationDecisionRequest, error) {
	if !moderationOperatorIDPattern.MatchString(operatorID) ||
		!validE2EEModerationReportID(reportID) || !validModerationAction(action) || now <= 0 {
		return E2EEModerationDecisionRequest{}, ErrModerationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEModerationDecisionRequest{}, err
	}
	defer tx.Rollback()
	operator, err := resolveModerationOperator(tx, token)
	if err != nil {
		return E2EEModerationDecisionRequest{}, err
	}
	if operator.ID != operatorID || !operator.Capabilities.Decide {
		return E2EEModerationDecisionRequest{}, ErrModerationForbidden
	}
	report, err := scanE2EEModerationReport(tx.QueryRow(
		`SELECT `+e2eeModerationReportColumns+` FROM e2ee_moderation_reports WHERE id = ?`, reportID))
	if errors.Is(err, sql.ErrNoRows) {
		return E2EEModerationDecisionRequest{}, ErrE2EEReportNotFound
	}
	if err != nil {
		return E2EEModerationDecisionRequest{}, err
	}
	existing, err := scanE2EEModerationDecision(tx.QueryRow(
		`SELECT `+e2eeModerationDecisionColumns+` FROM e2ee_moderation_decisions WHERE report_id = ?`, reportID))
	if err == nil {
		if existing.Action != action {
			return E2EEModerationDecisionRequest{}, ErrModerationDecisionConflict
		}
		if err := tx.Commit(); err != nil {
			return E2EEModerationDecisionRequest{}, err
		}
		return E2EEModerationDecisionRequest{Decision: existing, Report: report,
			Reused: true, Applied: existing.State == "applied"}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return E2EEModerationDecisionRequest{}, err
	}
	if report.Status != "open" {
		return E2EEModerationDecisionRequest{}, ErrModerationDecisionConflict
	}
	decision := E2EEModerationDecision{
		ID: "erd_" + ulid.New(time.UnixMilli(now)), ReportID: reportID,
		Action: action, State: "pending", RequestedByOperatorID: operatorID,
		RequestedAt: now,
	}
	if _, err := tx.Exec(`INSERT INTO e2ee_moderation_decisions(
id, report_id, action, state, requested_by_operator_id, requested_at, applied_at
) VALUES(?, ?, ?, 'pending', ?, ?, 0)`, decision.ID, decision.ReportID,
		decision.Action, decision.RequestedByOperatorID, decision.RequestedAt); err != nil {
		return E2EEModerationDecisionRequest{}, ErrE2EEConflict
	}
	if err := appendE2EEReportAuditTx(tx, reportID, "", operatorID, 0, "",
		"decision.requested", action, now); err != nil {
		return E2EEModerationDecisionRequest{}, err
	}
	if err := s.checkpoint("e2ee_moderation_decision_begin_before_commit"); err != nil {
		return E2EEModerationDecisionRequest{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEModerationDecisionRequest{}, err
	}
	return E2EEModerationDecisionRequest{Decision: decision, Report: report}, nil
}

func (s *Store) CompleteE2EEModerationDecision(decisionID string,
	now int64,
) (E2EEModerationDecision, error) {
	if len(decisionID) != 30 || !strings.HasPrefix(decisionID, "erd_") || now <= 0 {
		return E2EEModerationDecision{}, ErrModerationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEModerationDecision{}, err
	}
	defer tx.Rollback()
	decision, err := scanE2EEModerationDecision(tx.QueryRow(
		`SELECT `+e2eeModerationDecisionColumns+` FROM e2ee_moderation_decisions WHERE id = ?`, decisionID))
	if errors.Is(err, sql.ErrNoRows) {
		return E2EEModerationDecision{}, ErrE2EEReportNotFound
	}
	if err != nil {
		return E2EEModerationDecision{}, err
	}
	if decision.State == "applied" {
		return decision, tx.Commit()
	}
	if now < decision.RequestedAt {
		return E2EEModerationDecision{}, ErrModerationDecisionConflict
	}
	result, err := tx.Exec(`UPDATE e2ee_moderation_decisions
SET state = 'applied', applied_at = ? WHERE id = ? AND state = 'pending'`, now, decisionID)
	if err != nil {
		return E2EEModerationDecision{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return E2EEModerationDecision{}, err
		}
		return E2EEModerationDecision{}, ErrE2EEConflict
	}
	if _, err := tx.Exec(`UPDATE e2ee_moderation_reports
SET status = 'resolved', resolved_at = ?, revision = revision + 1, updated_at = ?
WHERE id = ? AND status = 'open'`, now, now, decision.ReportID); err != nil {
		return E2EEModerationDecision{}, err
	}
	if err := appendE2EEReportAuditTx(tx, decision.ReportID, "",
		decision.RequestedByOperatorID, 0, "", "decision.applied",
		decision.Action, now); err != nil {
		return E2EEModerationDecision{}, err
	}
	decision.State, decision.AppliedAt = "applied", now
	if err := s.checkpoint("e2ee_moderation_decision_complete_before_commit"); err != nil {
		return E2EEModerationDecision{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEModerationDecision{}, err
	}
	return decision, nil
}

// DeleteE2EEProtectedObjectForModeration reuses the opaque object's canonical
// chunk purge and terminal transition. It is deliberately unavailable unless
// an authenticated Decide-capable operator has already created the matching
// pending delete_media decision for this exact report.
func (s *Store) DeleteE2EEProtectedObjectForModeration(
	operatorID, token, reportID string,
	expectedRevision, now int64,
) (E2EEProtectedObject, error) {
	if !moderationOperatorIDPattern.MatchString(operatorID) ||
		!validE2EEModerationReportID(reportID) || expectedRevision <= 0 || now <= 0 {
		return E2EEProtectedObject{}, ErrModerationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEProtectedObject{}, err
	}
	defer tx.Rollback()
	operator, err := resolveModerationOperator(tx, token)
	if err != nil {
		return E2EEProtectedObject{}, err
	}
	if operator.ID != operatorID || !operator.Capabilities.Decide {
		return E2EEProtectedObject{}, ErrModerationForbidden
	}
	var protectedObjectID string
	if err := tx.QueryRow(`SELECT r.protected_object_id
FROM e2ee_moderation_reports r
JOIN e2ee_moderation_decisions d ON d.report_id = r.id
WHERE r.id = ? AND d.action = 'delete_media' AND d.state = 'pending'
  AND d.requested_by_operator_id = ?`, reportID, operatorID).Scan(&protectedObjectID); errors.Is(err, sql.ErrNoRows) {
		return E2EEProtectedObject{}, ErrModerationDecisionConflict
	} else if err != nil {
		return E2EEProtectedObject{}, err
	}
	object, err := scanE2EEProtectedObject(tx.QueryRow(
		`SELECT `+e2eeProtectedObjectColumns+` FROM e2ee_protected_objects WHERE id = ?`,
		protectedObjectID))
	if errors.Is(err, sql.ErrNoRows) {
		return E2EEProtectedObject{}, ErrE2EENotFound
	}
	if err != nil {
		return E2EEProtectedObject{}, err
	}
	if object.Status == "deleted" {
		if err := tx.Commit(); err != nil {
			return E2EEProtectedObject{}, err
		}
		return object, nil
	}
	if object.Revision != expectedRevision {
		return E2EEProtectedObject{}, ErrE2EEConflict
	}
	object, err = deleteE2EEProtectedObjectTx(
		tx, object, expectedRevision, now, 0, "", "moderation_decision",
	)
	if err != nil {
		return E2EEProtectedObject{}, err
	}
	if err := s.checkpoint("e2ee_moderation_delete_before_commit"); err != nil {
		return E2EEProtectedObject{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEProtectedObject{}, err
	}
	return object, nil
}
