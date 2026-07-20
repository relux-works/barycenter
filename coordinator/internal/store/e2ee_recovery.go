package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	"relux.works/duet/coordinator/internal/ulid"
)

const (
	e2eeTransferPackageMaxTTL = 15 * time.Minute
	e2eeHistoryGrantMaxTTL    = 30 * 24 * time.Hour
)

var (
	ErrE2EETransferUnavailable = errors.New("E2EE transfer package is unavailable")
	ErrE2EEHistoryUnavailable  = errors.New("E2EE history grant is unavailable")
)

type E2EETransferPackage struct {
	ID, GroupID, PackageKind, IssuerDeviceID, RecipientDeviceID string
	PackageDigest, Status, TargetSnapshotDigest                 string
	IssuerActorID, IssuerOrbitID, RecipientActorID              int64
	RecipientOrbitID, Epoch, Revision, CreatedAt, ExpiresAt     int64
	TerminalAt, ConsumedAt                                      int64
	EncryptedPackage                                            []byte
}

type E2EEHistoryGrantAccess struct {
	Grant            E2EEHistoryGrant
	IssuerActorID    int64
	IssuerOrbitID    int64
	RecipientActorID int64
	RecipientOrbitID int64
	AccessMode       string
	MaxReads         int64
	ReadCount        int64
	ApprovedAt       int64
	FirstAccessedAt  int64
	LastAccessedAt   int64
	EncryptedGrant   []byte
}

type E2EERecoveryExpiryResult struct {
	TransferPackages int
	HistoryGrants    int
}

func validE2EETransferPackageID(value string) bool {
	return len(value) == 30 && strings.HasPrefix(value, "etp_")
}

func validE2EEHistoryGrantID(value string) bool {
	return len(value) == 30 && strings.HasPrefix(value, "ehg_")
}

type e2eeRecoveryMember struct {
	Member             E2EEGroupMember
	DeviceRevision     int64
	VerificationDigest string
}

func authorizedE2EERecoveryMemberTx(
	tx *sql.Tx, group E2EEGroup, deviceID string,
) (e2eeRecoveryMember, error) {
	if err := requireExactCurrentE2EESnapshotTx(tx, group); err != nil {
		return e2eeRecoveryMember{}, err
	}
	var protocolActorID, verificationDigest string
	var revision int64
	if err := tx.QueryRow(`SELECT b.protocol_actor_id, d.revision, d.verification_digest
FROM e2ee_device_public_state d
JOIN e2ee_protocol_actor_bindings b ON b.device_id = d.device_id
  AND b.actor_id = d.actor_id
JOIN actors a ON a.id = d.actor_id AND a.revoked_at IS NULL
WHERE d.device_id = ? AND d.verification_state = 'verified' AND d.revoked_at = 0`,
		deviceID).Scan(&protocolActorID, &revision, &verificationDigest); err != nil {
		return e2eeRecoveryMember{}, ErrE2EEInvalid
	}
	member, err := authorizedE2EEGroupMemberTx(tx, group, deviceID, protocolActorID)
	if err != nil {
		return e2eeRecoveryMember{}, err
	}
	return e2eeRecoveryMember{Member: member, DeviceRevision: revision,
		VerificationDigest: verificationDigest}, nil
}

func scanE2EETransferPackage(row sqlScanner) (E2EETransferPackage, error) {
	var value E2EETransferPackage
	err := row.Scan(&value.ID, &value.GroupID, &value.PackageKind,
		&value.IssuerDeviceID, &value.RecipientDeviceID, &value.Epoch,
		&value.EncryptedPackage, &value.PackageDigest, &value.Status,
		&value.Revision, &value.CreatedAt, &value.ExpiresAt, &value.TerminalAt,
		&value.IssuerActorID, &value.IssuerOrbitID, &value.RecipientActorID,
		&value.RecipientOrbitID, &value.TargetSnapshotDigest, &value.ConsumedAt)
	if err == nil {
		value.EncryptedPackage = append([]byte(nil), value.EncryptedPackage...)
	}
	return value, err
}

const e2eeTransferPackageQuery = `SELECT p.id, p.group_id, p.package_kind,
p.issuer_device_id, p.recipient_device_id, p.epoch, p.encrypted_package,
p.package_digest, p.status, p.revision, p.created_at, p.expires_at, p.terminal_at,
b.issuer_actor_id, b.issuer_orbit_id, b.recipient_actor_id, b.recipient_orbit_id,
b.target_snapshot_digest, b.consumed_at
FROM e2ee_transfer_packages p
JOIN e2ee_transfer_package_bindings b ON b.package_id = p.id `

func (s *Store) CreateAuthorizedE2EETransferPackage(
	params CreateE2EETransferPackageParams,
) (E2EETransferPackage, error) {
	if len(params.GroupID) != 30 || len(params.IssuerDeviceID) < 8 ||
		len(params.RecipientDeviceID) < 8 || params.IssuerDeviceID == params.RecipientDeviceID ||
		params.Epoch <= 0 || params.ExpectedGroupRevision <= 0 ||
		params.ExpectedRecipientDeviceRevision <= 0 || params.CreatedAt <= 0 ||
		params.ExpiresAt <= params.CreatedAt ||
		params.ExpiresAt > params.CreatedAt+e2eeTransferPackageMaxTTL.Milliseconds() ||
		(params.PackageKind != "device_transfer" && params.PackageKind != "recovery" &&
			params.PackageKind != "welcome") ||
		!validE2EEDigest(params.TargetSnapshotDigest) ||
		!payloadDigestMatches(params.EncryptedPackage, params.PackageDigest) {
		return E2EETransferPackage{}, ErrE2EEInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EETransferPackage{}, err
	}
	defer tx.Rollback()
	group, err := e2eeGroupTx(tx, params.GroupID)
	if err != nil {
		return E2EETransferPackage{}, err
	}
	if group.ForkState != "clean" || group.Revision != params.ExpectedGroupRevision ||
		group.CurrentEpoch != params.Epoch ||
		group.TargetSnapshotDigest != params.TargetSnapshotDigest {
		return E2EETransferPackage{}, ErrE2EEStaleEpoch
	}
	issuer, err := authorizedE2EERecoveryMemberTx(tx, group, params.IssuerDeviceID)
	if err != nil {
		return E2EETransferPackage{}, err
	}
	recipient, err := authorizedE2EERecoveryMemberTx(tx, group, params.RecipientDeviceID)
	if err != nil {
		return E2EETransferPackage{}, err
	}
	if recipient.DeviceRevision != params.ExpectedRecipientDeviceRevision {
		return E2EETransferPackage{}, ErrE2EEConflict
	}
	if (params.PackageKind == "device_transfer" || params.PackageKind == "recovery") &&
		issuer.Member.OrbitID != recipient.Member.OrbitID {
		return E2EETransferPackage{}, ErrE2EEInvalid
	}
	id := "etp_" + ulid.New(time.UnixMilli(params.CreatedAt))
	if _, err := tx.Exec(`INSERT INTO e2ee_transfer_packages(
id, group_id, package_kind, issuer_device_id, recipient_device_id, epoch,
encrypted_package, package_digest, status, revision, created_at, expires_at, terminal_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, 'pending', 1, ?, ?, 0)`, id,
		group.ID, params.PackageKind, params.IssuerDeviceID, params.RecipientDeviceID,
		params.Epoch, params.EncryptedPackage, params.PackageDigest,
		params.CreatedAt, params.ExpiresAt); err != nil {
		return E2EETransferPackage{}, ErrE2EEConflict
	}
	if _, err := tx.Exec(`INSERT INTO e2ee_transfer_package_bindings(
package_id, group_id, issuer_actor_id, issuer_orbit_id, recipient_actor_id,
recipient_orbit_id, target_snapshot_digest, issuer_member_revision,
recipient_member_revision, issuer_device_revision, issuer_verification_digest,
recipient_device_revision, recipient_verification_digest, consumed_at, created_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?)`, id, group.ID,
		issuer.Member.ActorID, issuer.Member.OrbitID, recipient.Member.ActorID,
		recipient.Member.OrbitID, group.TargetSnapshotDigest, issuer.Member.Revision,
		recipient.Member.Revision, issuer.DeviceRevision, issuer.VerificationDigest,
		recipient.DeviceRevision,
		recipient.VerificationDigest, params.CreatedAt); err != nil {
		return E2EETransferPackage{}, ErrE2EEConflict
	}
	if err := appendE2EEAuditTx(tx, group.ID, "transfer_package", id,
		"transfer_package.create", "accepted", params.PackageKind,
		issuer.Member.ActorID, params.IssuerDeviceID, group.CurrentEpoch, 1,
		params.CreatedAt); err != nil {
		return E2EETransferPackage{}, err
	}
	if err := s.checkpoint("e2ee_transfer_package_before_commit"); err != nil {
		return E2EETransferPackage{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EETransferPackage{}, err
	}
	return s.GetE2EETransferPackage(id)
}

func (s *Store) GetE2EETransferPackage(id string) (E2EETransferPackage, error) {
	if !validE2EETransferPackageID(id) {
		return E2EETransferPackage{}, ErrE2EETransferUnavailable
	}
	value, err := scanE2EETransferPackage(s.db.QueryRow(
		e2eeTransferPackageQuery+`WHERE p.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return E2EETransferPackage{}, ErrE2EETransferUnavailable
	}
	return value, err
}

func (s *Store) ConsumeAuthorizedE2EETransferPackage(
	id, recipientDeviceID string, expectedRevision, now int64,
) (E2EETransferPackage, error) {
	if !validE2EETransferPackageID(id) || len(recipientDeviceID) < 8 ||
		expectedRevision <= 0 || now <= 0 {
		return E2EETransferPackage{}, ErrE2EETransferUnavailable
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EETransferPackage{}, err
	}
	defer tx.Rollback()
	value, err := scanE2EETransferPackage(tx.QueryRow(
		e2eeTransferPackageQuery+`WHERE p.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return E2EETransferPackage{}, ErrE2EETransferUnavailable
	}
	if err != nil {
		return E2EETransferPackage{}, err
	}
	if value.Status != "pending" || value.RecipientDeviceID != recipientDeviceID {
		return E2EETransferPackage{}, ErrE2EETransferUnavailable
	}
	if value.ExpiresAt <= now {
		if err := expireE2EETransferPackageTx(tx, value, now); err != nil {
			return E2EETransferPackage{}, err
		}
		if err := tx.Commit(); err != nil {
			return E2EETransferPackage{}, err
		}
		return E2EETransferPackage{}, ErrE2EETransferUnavailable
	}
	if value.Revision != expectedRevision {
		return E2EETransferPackage{}, ErrE2EEConflict
	}
	group, err := e2eeGroupTx(tx, value.GroupID)
	if err != nil || group.CurrentEpoch != value.Epoch ||
		group.TargetSnapshotDigest != value.TargetSnapshotDigest || group.ForkState != "clean" {
		return E2EETransferPackage{}, ErrE2EEStaleEpoch
	}
	recipient, err := authorizedE2EERecoveryMemberTx(tx, group, recipientDeviceID)
	if err != nil || recipient.Member.ActorID != value.RecipientActorID ||
		recipient.Member.OrbitID != value.RecipientOrbitID {
		return E2EETransferPackage{}, ErrE2EETransferUnavailable
	}
	issuer, err := authorizedE2EERecoveryMemberTx(tx, group, value.IssuerDeviceID)
	if err != nil || issuer.Member.ActorID != value.IssuerActorID ||
		issuer.Member.OrbitID != value.IssuerOrbitID {
		return E2EETransferPackage{}, ErrE2EETransferUnavailable
	}
	var recipientDeviceRevision, recipientMemberRevision int64
	var issuerDeviceRevision, issuerMemberRevision int64
	var recipientVerificationDigest, issuerVerificationDigest string
	if err := tx.QueryRow(`SELECT recipient_device_revision,
recipient_member_revision, recipient_verification_digest, issuer_device_revision,
issuer_member_revision, issuer_verification_digest
FROM e2ee_transfer_package_bindings WHERE package_id = ?`, id).Scan(
		&recipientDeviceRevision, &recipientMemberRevision, &recipientVerificationDigest,
		&issuerDeviceRevision, &issuerMemberRevision, &issuerVerificationDigest); err != nil {
		return E2EETransferPackage{}, err
	}
	if recipient.DeviceRevision != recipientDeviceRevision ||
		recipient.Member.Revision != recipientMemberRevision ||
		recipient.VerificationDigest != recipientVerificationDigest ||
		issuer.DeviceRevision != issuerDeviceRevision ||
		issuer.Member.Revision != issuerMemberRevision ||
		issuer.VerificationDigest != issuerVerificationDigest {
		return E2EETransferPackage{}, ErrE2EETransferUnavailable
	}
	result, err := tx.Exec(`UPDATE e2ee_transfer_packages
SET status = 'consumed', revision = revision + 1, terminal_at = ?
WHERE id = ? AND status = 'pending' AND revision = ?`, now, id, expectedRevision)
	if err != nil {
		return E2EETransferPackage{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return E2EETransferPackage{}, err
		}
		return E2EETransferPackage{}, ErrE2EEConflict
	}
	if _, err := tx.Exec(`UPDATE e2ee_transfer_package_bindings
SET consumed_at = ? WHERE package_id = ? AND consumed_at = 0`, now, id); err != nil {
		return E2EETransferPackage{}, err
	}
	if err := appendE2EEAuditTx(tx, value.GroupID, "transfer_package", id,
		"transfer_package.consume", "accepted", value.PackageKind,
		value.RecipientActorID, recipientDeviceID, value.Epoch,
		expectedRevision+1, now); err != nil {
		return E2EETransferPackage{}, err
	}
	if err := s.checkpoint("e2ee_transfer_consume_before_commit"); err != nil {
		return E2EETransferPackage{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EETransferPackage{}, err
	}
	value.Status, value.TerminalAt, value.ConsumedAt = "consumed", now, now
	value.Revision++
	return value, nil
}

func (s *Store) RevokeAuthorizedE2EETransferPackage(
	id, issuerDeviceID string, expectedRevision, now int64,
) (E2EETransferPackage, error) {
	if !validE2EETransferPackageID(id) || len(issuerDeviceID) < 8 ||
		expectedRevision <= 0 || now <= 0 {
		return E2EETransferPackage{}, ErrE2EETransferUnavailable
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EETransferPackage{}, err
	}
	defer tx.Rollback()
	value, err := scanE2EETransferPackage(tx.QueryRow(
		e2eeTransferPackageQuery+`WHERE p.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) || value.Status != "pending" ||
		value.IssuerDeviceID != issuerDeviceID {
		return E2EETransferPackage{}, ErrE2EETransferUnavailable
	}
	if err != nil {
		return E2EETransferPackage{}, err
	}
	if value.Revision != expectedRevision {
		return E2EETransferPackage{}, ErrE2EEConflict
	}
	group, err := e2eeGroupTx(tx, value.GroupID)
	if err != nil || group.ForkState != "clean" {
		return E2EETransferPackage{}, ErrE2EETransferUnavailable
	}
	issuer, err := authorizedE2EERecoveryMemberTx(tx, group, issuerDeviceID)
	if err != nil || issuer.Member.ActorID != value.IssuerActorID ||
		issuer.Member.OrbitID != value.IssuerOrbitID {
		return E2EETransferPackage{}, ErrE2EETransferUnavailable
	}
	result, err := tx.Exec(`UPDATE e2ee_transfer_packages
SET status = 'revoked', revision = revision + 1, terminal_at = ?
WHERE id = ? AND status = 'pending' AND revision = ?`, now, id, expectedRevision)
	if err != nil {
		return E2EETransferPackage{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return E2EETransferPackage{}, err
		}
		return E2EETransferPackage{}, ErrE2EEConflict
	}
	if err := appendE2EEAuditTx(tx, value.GroupID, "transfer_package", id,
		"transfer_package.revoke", "revoked", "issuer_revoke", value.IssuerActorID,
		issuerDeviceID, value.Epoch, expectedRevision+1, now); err != nil {
		return E2EETransferPackage{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EETransferPackage{}, err
	}
	value.Status, value.TerminalAt = "revoked", now
	value.Revision++
	return value, nil
}

func expireE2EETransferPackageTx(tx *sql.Tx, value E2EETransferPackage, now int64) error {
	result, err := tx.Exec(`UPDATE e2ee_transfer_packages
SET status = 'expired', revision = revision + 1, terminal_at = ?
WHERE id = ? AND status = 'pending'`, now, value.ID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrE2EEConflict
	}
	return appendE2EEAuditTx(tx, value.GroupID, "transfer_package", value.ID,
		"transfer_package.expire", "expired", "ttl", 0, "", value.Epoch,
		value.Revision+1, now)
}

func scanE2EEHistoryGrantAccess(row sqlScanner) (E2EEHistoryGrantAccess, error) {
	var value E2EEHistoryGrantAccess
	err := row.Scan(&value.Grant.ID, &value.Grant.GroupID,
		&value.Grant.IssuedByDeviceID, &value.Grant.RecipientDeviceID,
		&value.Grant.SourceObjectID, &value.Grant.TargetSnapshotDigest,
		&value.Grant.GrantDigest, &value.Grant.Status, &value.Grant.FirstEpoch,
		&value.Grant.LastEpoch, &value.Grant.Revision, &value.Grant.IssuedAt,
		&value.Grant.ExpiresAt, &value.Grant.RevokedAt, &value.EncryptedGrant,
		&value.IssuerActorID, &value.IssuerOrbitID, &value.RecipientActorID,
		&value.RecipientOrbitID, &value.AccessMode, &value.MaxReads,
		&value.ReadCount, &value.ApprovedAt, &value.FirstAccessedAt,
		&value.LastAccessedAt)
	if err == nil {
		value.EncryptedGrant = append([]byte(nil), value.EncryptedGrant...)
	}
	return value, err
}

const e2eeHistoryGrantAccessQuery = `SELECT g.id, g.group_id,
g.issued_by_device_id, g.recipient_device_id, g.source_object_id,
g.target_snapshot_digest, g.grant_digest, g.status, g.first_epoch,
g.last_epoch, g.revision, g.issued_at, g.expires_at, g.revoked_at,
g.encrypted_grant, b.issuer_actor_id, b.issuer_orbit_id,
b.recipient_actor_id, b.recipient_orbit_id, b.access_mode, b.max_reads,
b.read_count, b.approved_at, b.first_accessed_at, b.last_accessed_at
FROM e2ee_history_grants g
JOIN e2ee_history_grant_bindings b ON b.grant_id = g.id `

func (s *Store) CreateAuthorizedE2EEHistoryGrant(
	params CreateE2EEHistoryGrantParams,
) (E2EEHistoryGrant, error) {
	if len(params.GroupID) != 30 || len(params.IssuedByDeviceID) < 8 ||
		len(params.RecipientDeviceID) < 8 || len(params.SourceObjectID) < 8 ||
		params.FirstEpoch <= 0 || params.LastEpoch < params.FirstEpoch ||
		params.ExpectedGroupRevision <= 0 || params.ExpectedRecipientDeviceRevision <= 0 ||
		params.ApprovedAt <= 0 || params.IssuedAt < params.ApprovedAt ||
		params.ExpiresAt <= params.IssuedAt ||
		params.ExpiresAt > params.IssuedAt+e2eeHistoryGrantMaxTTL.Milliseconds() ||
		(params.AccessMode != "one_time" && params.AccessMode != "time_bound") ||
		params.MaxReads <= 0 || params.MaxReads > 32 ||
		(params.AccessMode == "one_time" && params.MaxReads != 1) ||
		!validE2EEDigest(params.TargetSnapshotDigest) ||
		!payloadDigestMatches(params.EncryptedGrant, params.GrantDigest) {
		return E2EEHistoryGrant{}, ErrE2EEInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEHistoryGrant{}, err
	}
	defer tx.Rollback()
	group, err := e2eeGroupTx(tx, params.GroupID)
	if err != nil {
		return E2EEHistoryGrant{}, err
	}
	if group.ForkState != "clean" || group.Revision != params.ExpectedGroupRevision ||
		group.TargetSnapshotDigest != params.TargetSnapshotDigest ||
		params.LastEpoch > group.CurrentEpoch {
		return E2EEHistoryGrant{}, ErrE2EEStaleEpoch
	}
	issuer, err := authorizedE2EERecoveryMemberTx(tx, group, params.IssuedByDeviceID)
	if err != nil {
		return E2EEHistoryGrant{}, err
	}
	recipient, err := authorizedE2EERecoveryMemberTx(tx, group, params.RecipientDeviceID)
	if err != nil {
		return E2EEHistoryGrant{}, err
	}
	if recipient.DeviceRevision != params.ExpectedRecipientDeviceRevision {
		return E2EEHistoryGrant{}, ErrE2EEConflict
	}
	object, err := scanE2EEProtectedObject(tx.QueryRow(
		`SELECT `+e2eeProtectedObjectColumns+` FROM e2ee_protected_objects
WHERE group_id = ? AND source_object_id = ?`, group.ID, params.SourceObjectID))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && object.Status != "ready") {
		return E2EEHistoryGrant{}, ErrE2EEHistoryUnavailable
	}
	if err != nil {
		return E2EEHistoryGrant{}, err
	}
	if object.Epoch < params.FirstEpoch || object.Epoch > params.LastEpoch ||
		(object.AuthorDeviceID != params.IssuedByDeviceID && issuer.Member.AirRole != "owner") {
		return E2EEHistoryGrant{}, ErrE2EEInvalid
	}
	id := "ehg_" + ulid.New(time.UnixMilli(params.IssuedAt))
	if _, err := tx.Exec(`INSERT INTO e2ee_history_grants(
id, group_id, issued_by_device_id, recipient_device_id, source_object_id,
first_epoch, last_epoch, target_snapshot_digest, encrypted_grant, grant_digest,
status, revision, issued_at, expires_at, revoked_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'active', 1, ?, ?, 0)`, id,
		group.ID, params.IssuedByDeviceID, params.RecipientDeviceID,
		params.SourceObjectID, params.FirstEpoch, params.LastEpoch,
		group.TargetSnapshotDigest, params.EncryptedGrant, params.GrantDigest,
		params.IssuedAt, params.ExpiresAt); err != nil {
		return E2EEHistoryGrant{}, ErrE2EEConflict
	}
	if _, err := tx.Exec(`INSERT INTO e2ee_history_grant_bindings(
grant_id, group_id, issuer_actor_id, issuer_orbit_id, recipient_actor_id,
recipient_orbit_id, recipient_device_revision, recipient_verification_digest,
issuer_member_revision, recipient_member_revision, access_mode, max_reads,
issuer_device_revision, issuer_verification_digest, read_count, approved_at,
first_accessed_at, last_accessed_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 0, ?, 0, 0)`, id, group.ID,
		issuer.Member.ActorID, issuer.Member.OrbitID, recipient.Member.ActorID,
		recipient.Member.OrbitID, recipient.DeviceRevision,
		recipient.VerificationDigest, issuer.Member.Revision,
		recipient.Member.Revision, params.AccessMode, params.MaxReads,
		issuer.DeviceRevision, issuer.VerificationDigest,
		params.ApprovedAt); err != nil {
		return E2EEHistoryGrant{}, ErrE2EEConflict
	}
	if err := appendE2EEAuditTx(tx, group.ID, "history_grant", id,
		"history_grant.create", "accepted", params.AccessMode,
		issuer.Member.ActorID, params.IssuedByDeviceID, group.CurrentEpoch, 1,
		params.IssuedAt); err != nil {
		return E2EEHistoryGrant{}, err
	}
	if err := s.checkpoint("e2ee_history_grant_before_commit"); err != nil {
		return E2EEHistoryGrant{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEHistoryGrant{}, err
	}
	return s.GetE2EEHistoryGrant(id)
}

func (s *Store) AuthorizeE2EEHistoryGrant(
	id, recipientDeviceID string, now int64,
) (E2EEHistoryGrantAccess, error) {
	if !validE2EEHistoryGrantID(id) || len(recipientDeviceID) < 8 || now <= 0 {
		return E2EEHistoryGrantAccess{}, ErrE2EEHistoryUnavailable
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEHistoryGrantAccess{}, err
	}
	defer tx.Rollback()
	value, err := scanE2EEHistoryGrantAccess(tx.QueryRow(
		e2eeHistoryGrantAccessQuery+`WHERE g.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return E2EEHistoryGrantAccess{}, ErrE2EEHistoryUnavailable
	}
	if err != nil {
		return E2EEHistoryGrantAccess{}, err
	}
	if value.Grant.Status != "active" || value.Grant.RecipientDeviceID != recipientDeviceID ||
		value.ReadCount >= value.MaxReads {
		return E2EEHistoryGrantAccess{}, ErrE2EEHistoryUnavailable
	}
	if value.Grant.ExpiresAt <= now {
		if err := expireE2EEHistoryGrantTx(tx, value, now); err != nil {
			return E2EEHistoryGrantAccess{}, err
		}
		if err := tx.Commit(); err != nil {
			return E2EEHistoryGrantAccess{}, err
		}
		return E2EEHistoryGrantAccess{}, ErrE2EEHistoryUnavailable
	}
	group, err := e2eeGroupTx(tx, value.Grant.GroupID)
	if err != nil || group.ForkState != "clean" ||
		group.TargetSnapshotDigest != value.Grant.TargetSnapshotDigest {
		return E2EEHistoryGrantAccess{}, ErrE2EEStaleEpoch
	}
	recipient, err := authorizedE2EERecoveryMemberTx(tx, group, recipientDeviceID)
	if err != nil || recipient.Member.ActorID != value.RecipientActorID ||
		recipient.Member.OrbitID != value.RecipientOrbitID {
		return E2EEHistoryGrantAccess{}, ErrE2EEHistoryUnavailable
	}
	issuer, err := authorizedE2EERecoveryMemberTx(tx, group, value.Grant.IssuedByDeviceID)
	if err != nil || issuer.Member.ActorID != value.IssuerActorID ||
		issuer.Member.OrbitID != value.IssuerOrbitID {
		return E2EEHistoryGrantAccess{}, ErrE2EEHistoryUnavailable
	}
	var deviceRevision, memberRevision, issuerDeviceRevision, issuerMemberRevision int64
	var verificationDigest, issuerVerificationDigest string
	if err := tx.QueryRow(`SELECT recipient_device_revision,
recipient_member_revision, recipient_verification_digest, issuer_device_revision,
issuer_member_revision, issuer_verification_digest
FROM e2ee_history_grant_bindings WHERE grant_id = ?`, id).Scan(
		&deviceRevision, &memberRevision, &verificationDigest, &issuerDeviceRevision,
		&issuerMemberRevision, &issuerVerificationDigest); err != nil {
		return E2EEHistoryGrantAccess{}, err
	}
	if recipient.DeviceRevision != deviceRevision || recipient.Member.Revision != memberRevision ||
		recipient.VerificationDigest != verificationDigest ||
		issuer.DeviceRevision != issuerDeviceRevision ||
		issuer.Member.Revision != issuerMemberRevision ||
		issuer.VerificationDigest != issuerVerificationDigest {
		return E2EEHistoryGrantAccess{}, ErrE2EEHistoryUnavailable
	}
	result, err := tx.Exec(`UPDATE e2ee_history_grant_bindings
SET read_count = read_count + 1,
first_accessed_at = CASE WHEN first_accessed_at = 0 THEN ? ELSE first_accessed_at END,
last_accessed_at = ? WHERE grant_id = ? AND read_count < max_reads`, now, now, id)
	if err != nil {
		return E2EEHistoryGrantAccess{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return E2EEHistoryGrantAccess{}, err
		}
		return E2EEHistoryGrantAccess{}, ErrE2EEConflict
	}
	if err := appendE2EEAuditTx(tx, group.ID, "history_grant", id,
		"history_grant.read", "accepted", value.AccessMode,
		value.RecipientActorID, recipientDeviceID, group.CurrentEpoch,
		value.Grant.Revision, now); err != nil {
		return E2EEHistoryGrantAccess{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEHistoryGrantAccess{}, err
	}
	value.ReadCount++
	if value.FirstAccessedAt == 0 {
		value.FirstAccessedAt = now
	}
	value.LastAccessedAt = now
	return value, nil
}

func (s *Store) RevokeAuthorizedE2EEHistoryGrant(
	id, requesterDeviceID string, expectedRevision, now int64,
) (E2EEHistoryGrant, error) {
	if !validE2EEHistoryGrantID(id) || len(requesterDeviceID) < 8 ||
		expectedRevision <= 0 || now <= 0 {
		return E2EEHistoryGrant{}, ErrE2EEHistoryUnavailable
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEHistoryGrant{}, err
	}
	defer tx.Rollback()
	value, err := scanE2EEHistoryGrantAccess(tx.QueryRow(
		e2eeHistoryGrantAccessQuery+`WHERE g.id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) || value.Grant.Status != "active" {
		return E2EEHistoryGrant{}, ErrE2EEHistoryUnavailable
	}
	if err != nil {
		return E2EEHistoryGrant{}, err
	}
	if value.Grant.Revision != expectedRevision {
		return E2EEHistoryGrant{}, ErrE2EEConflict
	}
	group, err := e2eeGroupTx(tx, value.Grant.GroupID)
	if err != nil || group.ForkState != "clean" {
		return E2EEHistoryGrant{}, ErrE2EEHistoryUnavailable
	}
	requester, err := authorizedE2EERecoveryMemberTx(tx, group, requesterDeviceID)
	if err != nil {
		return E2EEHistoryGrant{}, ErrE2EEHistoryUnavailable
	}
	isIssuer := requesterDeviceID == value.Grant.IssuedByDeviceID &&
		requester.Member.ActorID == value.IssuerActorID &&
		requester.Member.OrbitID == value.IssuerOrbitID
	isRecipient := requesterDeviceID == value.Grant.RecipientDeviceID &&
		requester.Member.ActorID == value.RecipientActorID &&
		requester.Member.OrbitID == value.RecipientOrbitID
	if !isIssuer && !isRecipient {
		return E2EEHistoryGrant{}, ErrE2EEHistoryUnavailable
	}
	result, err := tx.Exec(`UPDATE e2ee_history_grants
SET status = 'revoked', revision = revision + 1, revoked_at = ?
WHERE id = ? AND status = 'active' AND revision = ?`, now, id, expectedRevision)
	if err != nil {
		return E2EEHistoryGrant{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return E2EEHistoryGrant{}, err
		}
		return E2EEHistoryGrant{}, ErrE2EEConflict
	}
	if err := appendE2EEAuditTx(tx, value.Grant.GroupID, "history_grant", id,
		"history_grant.revoke", "revoked", "requester_revoke", requester.Member.ActorID,
		requesterDeviceID, value.Grant.LastEpoch, expectedRevision+1, now); err != nil {
		return E2EEHistoryGrant{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEHistoryGrant{}, err
	}
	value.Grant.Status, value.Grant.RevokedAt = "revoked", now
	value.Grant.Revision++
	return value.Grant, nil
}

func expireE2EEHistoryGrantTx(tx *sql.Tx, value E2EEHistoryGrantAccess, now int64) error {
	result, err := tx.Exec(`UPDATE e2ee_history_grants
SET status = 'expired', revision = revision + 1
WHERE id = ? AND status = 'active'`, value.Grant.ID)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrE2EEConflict
	}
	return appendE2EEAuditTx(tx, value.Grant.GroupID, "history_grant",
		value.Grant.ID, "history_grant.expire", "expired", "ttl", 0, "",
		value.Grant.LastEpoch, value.Grant.Revision+1, now)
}

func (s *Store) ExpireE2EERecoveryArtifacts(now int64, limit int) (E2EERecoveryExpiryResult, error) {
	if now <= 0 || limit <= 0 || limit > 1000 {
		return E2EERecoveryExpiryResult{}, ErrE2EEInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EERecoveryExpiryResult{}, err
	}
	defer tx.Rollback()
	result := E2EERecoveryExpiryResult{}
	rows, err := tx.Query(e2eeTransferPackageQuery+
		`WHERE p.status = 'pending' AND p.expires_at <= ? ORDER BY p.expires_at, p.id LIMIT ?`,
		now, limit)
	if err != nil {
		return result, err
	}
	var packages []E2EETransferPackage
	for rows.Next() {
		value, err := scanE2EETransferPackage(rows)
		if err != nil {
			rows.Close()
			return result, err
		}
		packages = append(packages, value)
	}
	if err := rows.Close(); err != nil {
		return result, err
	}
	for _, value := range packages {
		if err := expireE2EETransferPackageTx(tx, value, now); err != nil {
			return result, err
		}
		result.TransferPackages++
	}
	remaining := limit - result.TransferPackages
	if remaining > 0 {
		rows, err := tx.Query(e2eeHistoryGrantAccessQuery+
			`WHERE g.status = 'active' AND g.expires_at <= ? ORDER BY g.expires_at, g.id LIMIT ?`,
			now, remaining)
		if err != nil {
			return result, err
		}
		var grants []E2EEHistoryGrantAccess
		for rows.Next() {
			value, err := scanE2EEHistoryGrantAccess(rows)
			if err != nil {
				rows.Close()
				return result, err
			}
			grants = append(grants, value)
		}
		if err := rows.Close(); err != nil {
			return result, err
		}
		for _, value := range grants {
			if err := expireE2EEHistoryGrantTx(tx, value, now); err != nil {
				return result, err
			}
			result.HistoryGrants++
		}
	}
	if err := tx.Commit(); err != nil {
		return E2EERecoveryExpiryResult{}, err
	}
	return result, nil
}

// revokeE2EERecoveryForDeviceTx makes every outstanding artifact involving a
// lost device terminal in the same transaction as public-device revocation.
func revokeE2EERecoveryForDeviceTx(tx *sql.Tx, deviceID string, now int64) error {
	rows, err := tx.Query(e2eeTransferPackageQuery+
		`WHERE p.status = 'pending' AND (p.issuer_device_id = ? OR p.recipient_device_id = ?)`,
		deviceID, deviceID)
	if err != nil {
		return err
	}
	var packages []E2EETransferPackage
	for rows.Next() {
		value, err := scanE2EETransferPackage(rows)
		if err != nil {
			rows.Close()
			return err
		}
		packages = append(packages, value)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, value := range packages {
		if _, err := tx.Exec(`UPDATE e2ee_transfer_packages
SET status = 'revoked', revision = revision + 1, terminal_at = ?
WHERE id = ? AND status = 'pending'`, now, value.ID); err != nil {
			return err
		}
		if err := appendE2EEAuditTx(tx, value.GroupID, "transfer_package", value.ID,
			"transfer_package.revoke", "revoked", "device_revoke", 0, deviceID,
			value.Epoch, value.Revision+1, now); err != nil {
			return err
		}
	}
	grantRows, err := tx.Query(e2eeHistoryGrantAccessQuery+
		`WHERE g.status = 'active' AND (g.issued_by_device_id = ? OR g.recipient_device_id = ?)`,
		deviceID, deviceID)
	if err != nil {
		return err
	}
	var grants []E2EEHistoryGrantAccess
	for grantRows.Next() {
		value, err := scanE2EEHistoryGrantAccess(grantRows)
		if err != nil {
			grantRows.Close()
			return err
		}
		grants = append(grants, value)
	}
	if err := grantRows.Close(); err != nil {
		return err
	}
	for _, value := range grants {
		if _, err := tx.Exec(`UPDATE e2ee_history_grants
SET status = 'revoked', revision = revision + 1, revoked_at = ?
WHERE id = ? AND status = 'active'`, now, value.Grant.ID); err != nil {
			return err
		}
		if err := appendE2EEAuditTx(tx, value.Grant.GroupID, "history_grant",
			value.Grant.ID, "history_grant.revoke", "revoked", "device_revoke",
			0, deviceID, value.Grant.LastEpoch, value.Grant.Revision+1, now); err != nil {
			return err
		}
	}
	return nil
}
