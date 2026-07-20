package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"relux.works/duet/coordinator/internal/ulid"
)

var (
	ErrE2EEInvalid           = errors.New("invalid dormant E2EE state")
	ErrE2EENotFound          = errors.New("dormant E2EE state not found")
	ErrE2EEConflict          = errors.New("dormant E2EE state conflict")
	ErrE2EEStaleEpoch        = errors.New("stale E2EE epoch")
	ErrE2EEForked            = errors.New("forked E2EE epoch")
	ErrE2EEReplay            = errors.New("replayed E2EE event")
	ErrE2EERevoked           = errors.New("revoked E2EE state")
	ErrE2EEDuplicateFinalize = errors.New("protected object already finalized")
)

type E2EEPublicDevice struct {
	DeviceID, ProtocolActorID, PublicPackageDigest, VerificationState, VerificationDigest string
	ActorID, Revision, CreatedAt, UpdatedAt, RevokedAt                                    int64
}

type RegisterE2EEPublicDeviceParams struct {
	DeviceID, ProtocolActorID, PublicPackageDigest, VerificationState, VerificationDigest string
	ActorID, CreatedAt                                                                    int64
	PublicPackage                                                                         []byte
}

type E2EEGroup struct {
	ID, AirID, TargetSnapshotDigest, CommitDigest, ForkState string
	CurrentEpoch, Revision, CreatedAt, UpdatedAt             int64
}

type CreateE2EEGroupParams struct {
	AirID, AuthorDeviceID, TargetSnapshotDigest, CommitDigest string
	Epoch, CreatedAt                                          int64
}

type ApplyVerifiedE2EECommitParams struct {
	EventID, GroupID, AuthorDeviceID, PreviousCommitDigest string
	CommitDigest, TargetSnapshotDigest, EventDigest        string
	PreviousEpoch, Epoch, CreatedAt                        int64
	PublicPayload                                          []byte
}

type E2EEProtectedObject struct {
	ID, GroupID, SourceObjectID, ObjectKind, AuthorDeviceID string
	TargetSnapshotDigest, ManifestDigest, CiphertextRef     string
	CiphertextDigest, Status                                string
	Epoch, Generation, CiphertextSize, ChunkCount           int64
	DeclaredDurationMS, Revision, CreatedAt, UpdatedAt      int64
	FinalizedAt, RevokedAt, DeletedAt                       int64
}

type StageE2EEProtectedObjectParams struct {
	GroupID, SourceObjectID, ObjectKind, AuthorDeviceID string
	TargetSnapshotDigest, ManifestDigest, CiphertextRef string
	CiphertextDigest                                    string
	Epoch, Generation, CiphertextSize, ChunkCount       int64
	DeclaredDurationMS, CreatedAt                       int64
	EncryptedManifest, OpaqueKeyEnvelopes               []byte
}

type AcceptE2EEReplayStateParams struct {
	GroupID, EventID, AuthorDeviceID, SourceObjectID, NonceDigest string
	Epoch, Generation, Sequence, AcceptedAt                       int64
}

type E2EEHistoryGrant struct {
	ID, GroupID, IssuedByDeviceID, RecipientDeviceID, SourceObjectID string
	TargetSnapshotDigest, GrantDigest, Status                        string
	FirstEpoch, LastEpoch, Revision, IssuedAt, ExpiresAt, RevokedAt  int64
}

type CreateE2EEHistoryGrantParams struct {
	GroupID, IssuedByDeviceID, RecipientDeviceID, SourceObjectID string
	TargetSnapshotDigest, GrantDigest                            string
	FirstEpoch, LastEpoch, IssuedAt, ExpiresAt                   int64
	EncryptedGrant                                               []byte
}

type CreateE2EETransferPackageParams struct {
	GroupID, PackageKind, IssuerDeviceID, RecipientDeviceID string
	PackageDigest                                           string
	Epoch, CreatedAt, ExpiresAt                             int64
	EncryptedPackage                                        []byte
}

type CreateE2EEReportEvidenceParams struct {
	ReportID, ProtectedObjectID, ConsentVersion, ConsentDigest string
	ReporterDeviceID, AuthenticatedEvidenceDigest              string
	EncryptedEvidenceRef, AtRestCiphertextDigest               string
	EvidenceMIME                                               string
	ReporterActorID, EvidenceSizeBytes, ExpectedReportRevision int64
	ConsentConfirmedAt, RetentionExpiresAt, CreatedAt          int64
}

func validE2EEDigest(value string) bool {
	return len(value) == 64 && lowerHexTokenPattern.MatchString(value)
}

func validE2EEPayload(payload []byte) bool {
	return len(payload) > 0 && len(payload) <= 1<<20
}

func payloadDigestMatches(payload []byte, digest string) bool {
	if !validE2EEDigest(digest) {
		return false
	}
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:]) == digest
}

func verifiedE2EEDeviceTx(tx *sql.Tx, deviceID string) (int64, error) {
	var actorID int64
	err := tx.QueryRow(`SELECT actor_id FROM e2ee_device_public_state
WHERE device_id = ? AND verification_state = 'verified' AND revoked_at = 0`, deviceID).Scan(&actorID)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrE2EEInvalid
	}
	return actorID, err
}

func appendE2EEAuditTx(tx *sql.Tx, groupID, subjectKind, subjectID, operation,
	outcome, reason string, actorID int64, deviceID string, epoch, revision, now int64,
) error {
	_, err := tx.Exec(`INSERT INTO e2ee_audit_events(
id, group_id, subject_kind, subject_id, operation, outcome, reason_code,
actor_id, device_id, epoch, revision, created_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		"ea_"+ulid.New(time.UnixMilli(now)), groupID, subjectKind, subjectID,
		operation, outcome, reason, actorID, deviceID, epoch, revision, now)
	return err
}

func (s *Store) RegisterE2EEPublicDevice(params RegisterE2EEPublicDeviceParams) (E2EEPublicDevice, error) {
	if len(params.DeviceID) < 8 || len(params.DeviceID) > 128 ||
		len(params.ProtocolActorID) < 8 || len(params.ProtocolActorID) > 128 || params.ActorID <= 0 ||
		params.CreatedAt <= 0 || !validE2EEPayload(params.PublicPackage) ||
		!payloadDigestMatches(params.PublicPackage, params.PublicPackageDigest) ||
		(params.VerificationState != "unverified" && params.VerificationState != "verified") ||
		(params.VerificationState == "verified" && !validE2EEDigest(params.VerificationDigest)) ||
		(params.VerificationState == "unverified" && params.VerificationDigest != "") {
		return E2EEPublicDevice{}, ErrE2EEInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEPublicDevice{}, err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM actors WHERE id = ? AND revoked_at IS NULL`, params.ActorID).Scan(&exists); err != nil {
		return E2EEPublicDevice{}, err
	}
	if exists != 1 {
		return E2EEPublicDevice{}, ErrE2EEInvalid
	}
	_, err = tx.Exec(`INSERT INTO e2ee_device_public_state(
device_id, actor_id, public_package, public_package_digest, verification_state,
verification_digest, revision, created_at, updated_at, revoked_at
) VALUES(?, ?, ?, ?, ?, ?, 1, ?, ?, 0)`, params.DeviceID, params.ActorID,
		params.PublicPackage, params.PublicPackageDigest, params.VerificationState,
		params.VerificationDigest, params.CreatedAt, params.CreatedAt)
	if err != nil {
		return E2EEPublicDevice{}, ErrE2EEConflict
	}
	if _, err := tx.Exec(`INSERT INTO e2ee_protocol_actor_bindings(
device_id, actor_id, protocol_actor_id, created_at
) VALUES(?, ?, ?, ?)`, params.DeviceID, params.ActorID, params.ProtocolActorID,
		params.CreatedAt); err != nil {
		return E2EEPublicDevice{}, ErrE2EEConflict
	}
	if err := appendE2EEAuditTx(tx, "", "device", params.DeviceID,
		"device.public_state.register", "accepted", "", params.ActorID,
		params.DeviceID, 0, 1, params.CreatedAt); err != nil {
		return E2EEPublicDevice{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEPublicDevice{}, err
	}
	return s.GetE2EEPublicDevice(params.DeviceID)
}

func (s *Store) GetE2EEPublicDevice(deviceID string) (E2EEPublicDevice, error) {
	var value E2EEPublicDevice
	err := s.db.QueryRow(`SELECT d.device_id, d.actor_id, COALESCE(b.protocol_actor_id, ''),
d.public_package_digest, d.verification_state, d.verification_digest,
d.revision, d.created_at, d.updated_at, d.revoked_at
FROM e2ee_device_public_state d
LEFT JOIN e2ee_protocol_actor_bindings b ON b.device_id = d.device_id
WHERE d.device_id = ?`, deviceID).Scan(
		&value.DeviceID, &value.ActorID, &value.ProtocolActorID, &value.PublicPackageDigest,
		&value.VerificationState, &value.VerificationDigest, &value.Revision,
		&value.CreatedAt, &value.UpdatedAt, &value.RevokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return E2EEPublicDevice{}, ErrE2EENotFound
	}
	return value, err
}

func (s *Store) CreateE2EEGroup(params CreateE2EEGroupParams) (E2EEGroup, error) {
	if params.AirID == "" || len(params.AuthorDeviceID) < 8 || params.Epoch <= 0 || params.CreatedAt <= 0 ||
		!validE2EEDigest(params.TargetSnapshotDigest) || !validE2EEDigest(params.CommitDigest) {
		return E2EEGroup{}, ErrE2EEInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEGroup{}, err
	}
	defer tx.Rollback()
	actorID, err := verifiedE2EEDeviceTx(tx, params.AuthorDeviceID)
	if err != nil {
		return E2EEGroup{}, err
	}
	var airStatus string
	if err := tx.QueryRow(`SELECT status FROM airs WHERE public_id = ?`, params.AirID).Scan(&airStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return E2EEGroup{}, ErrE2EEInvalid
		}
		return E2EEGroup{}, err
	}
	if airStatus == "dissolved" {
		return E2EEGroup{}, ErrE2EEInvalid
	}
	id := "egp_" + ulid.New(time.UnixMilli(params.CreatedAt))
	_, err = tx.Exec(`INSERT INTO e2ee_groups(
id, air_id, target_snapshot_digest, current_epoch, commit_digest,
fork_state, revision, created_at, updated_at
) VALUES(?, ?, ?, ?, ?, 'clean', 1, ?, ?)`, id, params.AirID,
		params.TargetSnapshotDigest, params.Epoch, params.CommitDigest,
		params.CreatedAt, params.CreatedAt)
	if err != nil {
		return E2EEGroup{}, err
	}
	if err := appendE2EEAuditTx(tx, id, "group", id, "group.create", "accepted", "",
		actorID, params.AuthorDeviceID, params.Epoch, 1, params.CreatedAt); err != nil {
		return E2EEGroup{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEGroup{}, err
	}
	return s.GetE2EEGroup(id)
}

func (s *Store) GetE2EEGroup(id string) (E2EEGroup, error) {
	var value E2EEGroup
	err := s.db.QueryRow(`SELECT id, air_id, target_snapshot_digest, current_epoch,
commit_digest, fork_state, revision, created_at, updated_at
FROM e2ee_groups WHERE id = ?`, id).Scan(&value.ID, &value.AirID,
		&value.TargetSnapshotDigest, &value.CurrentEpoch, &value.CommitDigest,
		&value.ForkState, &value.Revision, &value.CreatedAt, &value.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return E2EEGroup{}, ErrE2EENotFound
	}
	return value, err
}

func (s *Store) ApplyVerifiedE2EECommit(params ApplyVerifiedE2EECommitParams) (E2EEGroup, error) {
	if len(params.EventID) < 8 || len(params.EventID) > 128 || len(params.GroupID) != 30 ||
		len(params.AuthorDeviceID) < 8 || params.PreviousEpoch <= 0 ||
		params.Epoch <= 0 || params.CreatedAt <= 0 || !validE2EEPayload(params.PublicPayload) ||
		!payloadDigestMatches(params.PublicPayload, params.EventDigest) ||
		!validE2EEDigest(params.PreviousCommitDigest) || !validE2EEDigest(params.CommitDigest) ||
		!validE2EEDigest(params.TargetSnapshotDigest) {
		return E2EEGroup{}, ErrE2EEInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEGroup{}, err
	}
	defer tx.Rollback()
	actorID, err := verifiedE2EEDeviceTx(tx, params.AuthorDeviceID)
	if err != nil {
		return E2EEGroup{}, err
	}
	var group E2EEGroup
	if err := tx.QueryRow(`SELECT id, air_id, target_snapshot_digest, current_epoch,
commit_digest, fork_state, revision, created_at, updated_at
FROM e2ee_groups WHERE id = ?`, params.GroupID).Scan(&group.ID, &group.AirID,
		&group.TargetSnapshotDigest, &group.CurrentEpoch, &group.CommitDigest,
		&group.ForkState, &group.Revision, &group.CreatedAt, &group.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return E2EEGroup{}, ErrE2EENotFound
		}
		return E2EEGroup{}, err
	}
	if group.ForkState == "revoked" {
		return E2EEGroup{}, ErrE2EERevoked
	}
	if group.ForkState == "forked" {
		return E2EEGroup{}, ErrE2EEForked
	}
	var duplicate int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM e2ee_public_group_events
WHERE group_id = ? AND (id = ? OR event_digest = ?)`, params.GroupID,
		params.EventID, params.EventDigest).Scan(&duplicate); err != nil {
		return E2EEGroup{}, err
	}
	if duplicate != 0 {
		if err := appendE2EEAuditTx(tx, params.GroupID, "public_event", params.EventID,
			"commit.apply", "rejected", "replay", actorID, params.AuthorDeviceID,
			params.Epoch, group.Revision, params.CreatedAt); err != nil {
			return E2EEGroup{}, err
		}
		if err := tx.Commit(); err != nil {
			return E2EEGroup{}, err
		}
		return E2EEGroup{}, ErrE2EEReplay
	}
	if params.PreviousEpoch < group.CurrentEpoch || params.Epoch <= group.CurrentEpoch {
		if err := appendE2EEAuditTx(tx, params.GroupID, "public_event", params.EventID,
			"commit.apply", "rejected", "stale_epoch", actorID, params.AuthorDeviceID,
			params.Epoch, group.Revision, params.CreatedAt); err != nil {
			return E2EEGroup{}, err
		}
		if err := tx.Commit(); err != nil {
			return E2EEGroup{}, err
		}
		return E2EEGroup{}, ErrE2EEStaleEpoch
	}
	if params.PreviousEpoch != group.CurrentEpoch || params.Epoch != group.CurrentEpoch+1 ||
		params.PreviousCommitDigest != group.CommitDigest {
		// Only an exact-current competing predecessor is evidence of a fork.
		// Future/gapped traffic is rejected without letting delivery order poison
		// the authenticated current state.
		if params.PreviousEpoch == group.CurrentEpoch && params.Epoch == group.CurrentEpoch+1 &&
			params.PreviousCommitDigest != group.CommitDigest {
			if _, err := tx.Exec(`UPDATE e2ee_groups SET fork_state = 'forked',
revision = revision + 1, updated_at = ? WHERE id = ? AND revision = ? AND fork_state = 'clean'`,
				params.CreatedAt, group.ID, group.Revision); err != nil {
				return E2EEGroup{}, err
			}
			group.Revision++
		}
		if err := appendE2EEAuditTx(tx, params.GroupID, "public_event", params.EventID,
			"commit.apply", "rejected", "forked_epoch", actorID, params.AuthorDeviceID,
			params.Epoch, group.Revision, params.CreatedAt); err != nil {
			return E2EEGroup{}, err
		}
		if err := tx.Commit(); err != nil {
			return E2EEGroup{}, err
		}
		return E2EEGroup{}, ErrE2EEForked
	}
	if _, err := tx.Exec(`INSERT INTO e2ee_public_group_events(
id, group_id, kind, author_device_id, previous_epoch, epoch,
previous_commit_digest, event_digest, public_payload, state, reason_code,
created_at, updated_at
) VALUES(?, ?, 'commit', ?, ?, ?, ?, ?, ?, 'accepted', '', ?, ?)`,
		params.EventID, params.GroupID, params.AuthorDeviceID, params.PreviousEpoch,
		params.Epoch, params.PreviousCommitDigest, params.EventDigest,
		params.PublicPayload, params.CreatedAt, params.CreatedAt); err != nil {
		return E2EEGroup{}, err
	}
	result, err := tx.Exec(`UPDATE e2ee_groups SET current_epoch = ?, commit_digest = ?,
target_snapshot_digest = ?, revision = revision + 1, updated_at = ?
WHERE id = ? AND current_epoch = ? AND commit_digest = ? AND revision = ? AND fork_state = 'clean'`,
		params.Epoch, params.CommitDigest, params.TargetSnapshotDigest, params.CreatedAt,
		params.GroupID, params.PreviousEpoch, params.PreviousCommitDigest, group.Revision)
	if err != nil {
		return E2EEGroup{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return E2EEGroup{}, err
	}
	if changed != 1 {
		return E2EEGroup{}, ErrE2EEConflict
	}
	if err := appendE2EEAuditTx(tx, params.GroupID, "public_event", params.EventID,
		"commit.apply", "accepted", "", actorID, params.AuthorDeviceID,
		params.Epoch, group.Revision+1, params.CreatedAt); err != nil {
		return E2EEGroup{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEGroup{}, err
	}
	return s.GetE2EEGroup(params.GroupID)
}

func (s *Store) StageE2EEProtectedObject(params StageE2EEProtectedObjectParams) (E2EEProtectedObject, error) {
	if len(params.GroupID) != 30 || len(params.SourceObjectID) < 8 || len(params.SourceObjectID) > 128 ||
		len(params.AuthorDeviceID) < 8 || params.Epoch <= 0 || params.Generation <= 0 ||
		params.CiphertextSize <= 0 || params.CiphertextSize > e2eeMaxObjectBytes ||
		params.ChunkCount <= 0 || params.ChunkCount > e2eeMaxObjectChunks || params.DeclaredDurationMS < 0 ||
		params.CreatedAt <= 0 || !validE2EEDigest(params.TargetSnapshotDigest) ||
		!validE2EEDigest(params.ManifestDigest) || !validE2EEDigest(params.CiphertextDigest) ||
		!validE2EEPayload(params.EncryptedManifest) || !validE2EEPayload(params.OpaqueKeyEnvelopes) ||
		params.CiphertextRef != "ciphertext/v1/"+params.CiphertextDigest ||
		(params.ObjectKind != "clip" && params.ObjectKind != "track" &&
			params.ObjectKind != "saved_cue" && params.ObjectKind != "live_ptt") {
		return E2EEProtectedObject{}, ErrE2EEInvalid
	}
	// Routing-aware groups must rotate before any later protected object can
	// be sealed. Legacy foundation fixtures without an initialized routing
	// snapshot keep their production-dark repository behavior.
	if requirement, err := s.ReconcileE2EERotation(params.GroupID, params.CreatedAt); err != nil {
		return E2EEProtectedObject{}, err
	} else if requirement != nil && requirement.State == "required" {
		return E2EEProtectedObject{}, ErrE2EERotationRequired
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEProtectedObject{}, err
	}
	defer tx.Rollback()
	actorID, err := verifiedE2EEDeviceTx(tx, params.AuthorDeviceID)
	if err != nil {
		return E2EEProtectedObject{}, err
	}
	var epoch int64
	var target, forkState string
	if err := tx.QueryRow(`SELECT current_epoch, target_snapshot_digest, fork_state
FROM e2ee_groups WHERE id = ?`, params.GroupID).Scan(&epoch, &target, &forkState); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return E2EEProtectedObject{}, ErrE2EENotFound
		}
		return E2EEProtectedObject{}, err
	}
	if forkState == "revoked" {
		return E2EEProtectedObject{}, ErrE2EERevoked
	}
	if forkState != "clean" || params.Epoch != epoch || params.TargetSnapshotDigest != target {
		return E2EEProtectedObject{}, ErrE2EEStaleEpoch
	}
	var routingMembers []E2EEGroupMember
	if initialized, err := e2eeRoutingInitializedTx(tx, params.GroupID); err != nil {
		return E2EEProtectedObject{}, err
	} else if initialized {
		if !payloadDigestMatches(params.EncryptedManifest, params.ManifestDigest) {
			return E2EEProtectedObject{}, ErrE2EEInvalid
		}
		group, err := e2eeGroupTx(tx, params.GroupID)
		if err != nil {
			return E2EEProtectedObject{}, err
		}
		var protocolActorID string
		if err := tx.QueryRow(`SELECT protocol_actor_id
FROM e2ee_protocol_actor_bindings WHERE device_id = ?`,
			params.AuthorDeviceID).Scan(&protocolActorID); err != nil {
			return E2EEProtectedObject{}, err
		}
		if _, err := authorizedE2EEGroupMemberTx(tx, group, params.AuthorDeviceID,
			protocolActorID); err != nil {
			return E2EEProtectedObject{}, ErrE2EEInvalid
		}
		current, err := e2eeCurrentMembersTx(tx, params.GroupID)
		if err != nil {
			return E2EEProtectedObject{}, err
		}
		snapshot, err := e2eeAirSnapshotTx(tx, group.AirID)
		if err != nil {
			return E2EEProtectedObject{}, err
		}
		if len(snapshot.UnsupportedActorIDs) > 0 || snapshot.Digest != target ||
			!sameE2EEMemberSet(current, snapshot.Members) {
			return E2EEProtectedObject{}, ErrE2EERotationRequired
		}
		routingMembers = current
		if err := e2eeProtectedObjectCapacityTx(tx, actorID, params.CiphertextSize,
			params.CreatedAt); err != nil {
			return E2EEProtectedObject{}, err
		}
	}
	id := "em_" + ulid.New(time.UnixMilli(params.CreatedAt))
	_, err = tx.Exec(`INSERT INTO e2ee_protected_objects(
id, group_id, source_object_id, object_kind, author_device_id, epoch, generation,
target_snapshot_digest, manifest_digest, encrypted_manifest, opaque_key_envelopes,
ciphertext_ref, ciphertext_digest, ciphertext_size, chunk_count,
declared_duration_ms, status, revision, created_at, updated_at,
finalized_at, revoked_at, deleted_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'staged', 1, ?, ?, 0, 0, 0)`,
		id, params.GroupID, params.SourceObjectID, params.ObjectKind,
		params.AuthorDeviceID, params.Epoch, params.Generation,
		params.TargetSnapshotDigest, params.ManifestDigest, params.EncryptedManifest,
		params.OpaqueKeyEnvelopes, params.CiphertextRef, params.CiphertextDigest,
		params.CiphertextSize, params.ChunkCount, params.DeclaredDurationMS,
		params.CreatedAt, params.CreatedAt)
	if err != nil {
		return E2EEProtectedObject{}, ErrE2EEConflict
	}
	for _, member := range routingMembers {
		if _, err := tx.Exec(`INSERT INTO e2ee_protected_object_recipients(
protected_object_id, recipient_device_id, actor_id, protocol_actor_id,
actor_membership_role, actor_membership_joined_at, orbit_id, air_membership_id,
air_role, air_membership_revision, created_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, member.DeviceID, member.ActorID,
			member.ProtocolActorID, member.ActorMembershipRole,
			member.ActorMembershipJoinedAt, member.OrbitID, member.AirMembershipID,
			member.AirRole, member.AirMembershipRevision, params.CreatedAt); err != nil {
			return E2EEProtectedObject{}, err
		}
	}
	if err := appendE2EEAuditTx(tx, params.GroupID, "protected_object", id,
		"protected_object.stage", "accepted", "", actorID, params.AuthorDeviceID,
		params.Epoch, 1, params.CreatedAt); err != nil {
		return E2EEProtectedObject{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEProtectedObject{}, err
	}
	return s.GetE2EEProtectedObject(id)
}

const e2eeProtectedObjectColumns = `id, group_id, source_object_id, object_kind,
author_device_id, target_snapshot_digest, manifest_digest, ciphertext_ref,
ciphertext_digest, status, epoch, generation, ciphertext_size, chunk_count,
declared_duration_ms, revision, created_at, updated_at, finalized_at, revoked_at, deleted_at`

func scanE2EEProtectedObject(row *sql.Row) (E2EEProtectedObject, error) {
	var value E2EEProtectedObject
	err := row.Scan(&value.ID, &value.GroupID, &value.SourceObjectID, &value.ObjectKind,
		&value.AuthorDeviceID, &value.TargetSnapshotDigest, &value.ManifestDigest,
		&value.CiphertextRef, &value.CiphertextDigest, &value.Status, &value.Epoch,
		&value.Generation, &value.CiphertextSize, &value.ChunkCount,
		&value.DeclaredDurationMS, &value.Revision, &value.CreatedAt, &value.UpdatedAt,
		&value.FinalizedAt, &value.RevokedAt, &value.DeletedAt)
	return value, err
}

func (s *Store) GetE2EEProtectedObject(id string) (E2EEProtectedObject, error) {
	value, err := scanE2EEProtectedObject(s.db.QueryRow(
		`SELECT `+e2eeProtectedObjectColumns+` FROM e2ee_protected_objects WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return E2EEProtectedObject{}, ErrE2EENotFound
	}
	return value, err
}

func (s *Store) transitionE2EEProtectedObject(id, operation, status, outcome string,
	expectedRevision, now int64,
) (E2EEProtectedObject, error) {
	if len(id) != 29 || expectedRevision <= 0 || now <= 0 {
		return E2EEProtectedObject{}, ErrE2EEInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEProtectedObject{}, err
	}
	defer tx.Rollback()
	value, err := scanE2EEProtectedObject(tx.QueryRow(
		`SELECT `+e2eeProtectedObjectColumns+` FROM e2ee_protected_objects WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return E2EEProtectedObject{}, ErrE2EENotFound
	}
	if err != nil {
		return E2EEProtectedObject{}, err
	}
	if operation == "protected_object.finalize" && value.Status != "staged" {
		return E2EEProtectedObject{}, ErrE2EEDuplicateFinalize
	}
	if operation == "protected_object.revoke" && value.Status != "ready" {
		return E2EEProtectedObject{}, ErrE2EERevoked
	}
	if value.Revision != expectedRevision {
		return E2EEProtectedObject{}, ErrE2EEConflict
	}
	if operation == "protected_object.finalize" {
		if enabled, err := e2eeProtectedObjectRouterEnabledTx(tx, value.ID); err != nil {
			return E2EEProtectedObject{}, err
		} else if enabled {
			group, err := e2eeGroupTx(tx, value.GroupID)
			if err != nil {
				return E2EEProtectedObject{}, err
			}
			if group.ForkState != "clean" {
				return E2EEProtectedObject{}, ErrE2EEForked
			}
			if err := requireExactCurrentE2EESnapshotTx(tx, group); err != nil {
				return E2EEProtectedObject{}, err
			}
			if err := validateE2EEProtectedObjectChunksTx(tx, value); err != nil {
				return E2EEProtectedObject{}, err
			}
		}
	}
	stampColumn := "finalized_at"
	if status == "revoked" {
		stampColumn = "revoked_at"
	}
	result, err := tx.Exec(`UPDATE e2ee_protected_objects SET status = ?, revision = revision + 1,
updated_at = ?, `+stampColumn+` = ? WHERE id = ? AND revision = ? AND status = ?`,
		status, now, now, id, expectedRevision, value.Status)
	if err != nil {
		return E2EEProtectedObject{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return E2EEProtectedObject{}, err
	}
	if changed != 1 {
		return E2EEProtectedObject{}, ErrE2EEConflict
	}
	if err := appendE2EEAuditTx(tx, value.GroupID, "protected_object", id,
		operation, outcome, "", 0, value.AuthorDeviceID, value.Epoch,
		expectedRevision+1, now); err != nil {
		return E2EEProtectedObject{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEProtectedObject{}, err
	}
	return s.GetE2EEProtectedObject(id)
}

func (s *Store) FinalizeE2EEProtectedObject(id string, expectedRevision, now int64) (E2EEProtectedObject, error) {
	return s.transitionE2EEProtectedObject(id, "protected_object.finalize", "ready", "accepted", expectedRevision, now)
}

func (s *Store) RevokeE2EEProtectedObject(id string, expectedRevision, now int64) (E2EEProtectedObject, error) {
	return s.transitionE2EEProtectedObject(id, "protected_object.revoke", "revoked", "revoked", expectedRevision, now)
}

func (s *Store) AcceptE2EEReplayState(params AcceptE2EEReplayStateParams) error {
	if len(params.GroupID) != 30 || len(params.EventID) < 8 || len(params.EventID) > 128 ||
		len(params.AuthorDeviceID) < 8 || len(params.SourceObjectID) < 8 ||
		params.Epoch <= 0 || params.Generation <= 0 || params.Sequence <= 0 ||
		params.AcceptedAt <= 0 || !validE2EEDigest(params.NonceDigest) {
		return ErrE2EEInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var currentEpoch int64
	var forkState string
	if err := tx.QueryRow(`SELECT current_epoch, fork_state FROM e2ee_groups WHERE id = ?`,
		params.GroupID).Scan(&currentEpoch, &forkState); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrE2EENotFound
		}
		return err
	}
	if forkState != "clean" {
		return ErrE2EEForked
	}
	if params.Epoch != currentEpoch {
		return ErrE2EEStaleEpoch
	}
	var duplicate int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM e2ee_replay_events
WHERE group_id = ? AND (event_id = ? OR nonce_digest = ?)`, params.GroupID,
		params.EventID, params.NonceDigest).Scan(&duplicate); err != nil {
		return err
	}
	if duplicate != 0 {
		return ErrE2EEReplay
	}
	var generation, sequence, revision int64
	err = tx.QueryRow(`SELECT generation, last_sequence, revision
FROM e2ee_sender_replay_state
WHERE group_id = ? AND author_device_id = ? AND source_object_id = ?`,
		params.GroupID, params.AuthorDeviceID, params.SourceObjectID).Scan(
		&generation, &sequence, &revision)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if err == nil {
		if params.Generation < generation ||
			(params.Generation == generation && params.Sequence <= sequence) ||
			params.Generation > generation+1 ||
			(params.Generation == generation+1 && params.Sequence != 1) {
			return ErrE2EEReplay
		}
		result, err := tx.Exec(`UPDATE e2ee_sender_replay_state
SET generation = ?, last_sequence = ?, revision = revision + 1, updated_at = ?
WHERE group_id = ? AND author_device_id = ? AND source_object_id = ? AND revision = ?`,
			params.Generation, params.Sequence, params.AcceptedAt, params.GroupID,
			params.AuthorDeviceID, params.SourceObjectID, revision)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil || changed != 1 {
			if err != nil {
				return err
			}
			return ErrE2EEConflict
		}
	} else {
		if params.Sequence != 1 {
			return ErrE2EEReplay
		}
		if _, err := tx.Exec(`INSERT INTO e2ee_sender_replay_state(
group_id, author_device_id, source_object_id, generation, last_sequence,
revision, updated_at
) VALUES(?, ?, ?, ?, ?, 1, ?)`, params.GroupID, params.AuthorDeviceID,
			params.SourceObjectID, params.Generation, params.Sequence, params.AcceptedAt); err != nil {
			return ErrE2EEConflict
		}
	}
	if _, err := tx.Exec(`INSERT INTO e2ee_replay_events(
group_id, event_id, nonce_digest, epoch, generation, sequence, accepted_at
) VALUES(?, ?, ?, ?, ?, ?, ?)`, params.GroupID, params.EventID,
		params.NonceDigest, params.Epoch, params.Generation, params.Sequence,
		params.AcceptedAt); err != nil {
		return ErrE2EEReplay
	}
	if err := appendE2EEAuditTx(tx, params.GroupID, "replay", params.EventID,
		"replay.accept", "accepted", "", 0, params.AuthorDeviceID,
		params.Epoch, revision+1, params.AcceptedAt); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) CreateE2EEHistoryGrant(params CreateE2EEHistoryGrantParams) (E2EEHistoryGrant, error) {
	if len(params.GroupID) != 30 || len(params.IssuedByDeviceID) < 8 ||
		len(params.RecipientDeviceID) < 8 || len(params.SourceObjectID) < 8 ||
		params.FirstEpoch <= 0 || params.LastEpoch < params.FirstEpoch ||
		params.IssuedAt <= 0 || params.ExpiresAt <= params.IssuedAt ||
		!validE2EEDigest(params.TargetSnapshotDigest) ||
		!payloadDigestMatches(params.EncryptedGrant, params.GrantDigest) {
		return E2EEHistoryGrant{}, ErrE2EEInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEHistoryGrant{}, err
	}
	defer tx.Rollback()
	actorID, err := verifiedE2EEDeviceTx(tx, params.IssuedByDeviceID)
	if err != nil {
		return E2EEHistoryGrant{}, err
	}
	if _, err := verifiedE2EEDeviceTx(tx, params.RecipientDeviceID); err != nil {
		return E2EEHistoryGrant{}, err
	}
	var epoch int64
	var target, state string
	if err := tx.QueryRow(`SELECT current_epoch, target_snapshot_digest, fork_state
FROM e2ee_groups WHERE id = ?`, params.GroupID).Scan(&epoch, &target, &state); err != nil {
		return E2EEHistoryGrant{}, err
	}
	if state != "clean" || params.LastEpoch > epoch || params.TargetSnapshotDigest != target {
		return E2EEHistoryGrant{}, ErrE2EEStaleEpoch
	}
	id := "ehg_" + ulid.New(time.UnixMilli(params.IssuedAt))
	_, err = tx.Exec(`INSERT INTO e2ee_history_grants(
id, group_id, issued_by_device_id, recipient_device_id, source_object_id,
first_epoch, last_epoch, target_snapshot_digest, encrypted_grant, grant_digest,
status, revision, issued_at, expires_at, revoked_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'active', 1, ?, ?, 0)`, id,
		params.GroupID, params.IssuedByDeviceID, params.RecipientDeviceID,
		params.SourceObjectID, params.FirstEpoch, params.LastEpoch,
		params.TargetSnapshotDigest, params.EncryptedGrant, params.GrantDigest,
		params.IssuedAt, params.ExpiresAt)
	if err != nil {
		return E2EEHistoryGrant{}, ErrE2EEConflict
	}
	if err := appendE2EEAuditTx(tx, params.GroupID, "history_grant", id,
		"history_grant.create", "accepted", "", actorID, params.IssuedByDeviceID,
		epoch, 1, params.IssuedAt); err != nil {
		return E2EEHistoryGrant{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEHistoryGrant{}, err
	}
	return s.GetE2EEHistoryGrant(id)
}

func (s *Store) GetE2EEHistoryGrant(id string) (E2EEHistoryGrant, error) {
	var value E2EEHistoryGrant
	err := s.db.QueryRow(`SELECT id, group_id, issued_by_device_id, recipient_device_id,
source_object_id, target_snapshot_digest, grant_digest, status, first_epoch,
last_epoch, revision, issued_at, expires_at, revoked_at
FROM e2ee_history_grants WHERE id = ?`, id).Scan(&value.ID, &value.GroupID,
		&value.IssuedByDeviceID, &value.RecipientDeviceID, &value.SourceObjectID,
		&value.TargetSnapshotDigest, &value.GrantDigest, &value.Status,
		&value.FirstEpoch, &value.LastEpoch, &value.Revision, &value.IssuedAt,
		&value.ExpiresAt, &value.RevokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return E2EEHistoryGrant{}, ErrE2EENotFound
	}
	return value, err
}

func (s *Store) RevokeE2EEHistoryGrant(id string, expectedRevision, now int64) (E2EEHistoryGrant, error) {
	if !strings.HasPrefix(id, "ehg_") || expectedRevision <= 0 || now <= 0 {
		return E2EEHistoryGrant{}, ErrE2EEInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEHistoryGrant{}, err
	}
	defer tx.Rollback()
	var groupID, issuer, status string
	var epoch int64
	if err := tx.QueryRow(`SELECT group_id, issued_by_device_id, status, last_epoch
FROM e2ee_history_grants WHERE id = ?`, id).Scan(&groupID, &issuer, &status, &epoch); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return E2EEHistoryGrant{}, ErrE2EENotFound
		}
		return E2EEHistoryGrant{}, err
	}
	if status != "active" {
		return E2EEHistoryGrant{}, ErrE2EERevoked
	}
	result, err := tx.Exec(`UPDATE e2ee_history_grants SET status = 'revoked',
revision = revision + 1, revoked_at = ? WHERE id = ? AND revision = ? AND status = 'active'`,
		now, id, expectedRevision)
	if err != nil {
		return E2EEHistoryGrant{}, err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		if err != nil {
			return E2EEHistoryGrant{}, err
		}
		return E2EEHistoryGrant{}, ErrE2EEConflict
	}
	if err := appendE2EEAuditTx(tx, groupID, "history_grant", id,
		"history_grant.revoke", "revoked", "", 0, issuer, epoch,
		expectedRevision+1, now); err != nil {
		return E2EEHistoryGrant{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEHistoryGrant{}, err
	}
	return s.GetE2EEHistoryGrant(id)
}

func (s *Store) CreateE2EETransferPackage(params CreateE2EETransferPackageParams) (string, error) {
	if len(params.GroupID) != 30 || len(params.IssuerDeviceID) < 8 ||
		len(params.RecipientDeviceID) < 8 || params.Epoch <= 0 ||
		params.CreatedAt <= 0 || params.ExpiresAt <= params.CreatedAt ||
		(params.PackageKind != "device_transfer" && params.PackageKind != "recovery" && params.PackageKind != "welcome") ||
		!payloadDigestMatches(params.EncryptedPackage, params.PackageDigest) {
		return "", ErrE2EEInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	actorID, err := verifiedE2EEDeviceTx(tx, params.IssuerDeviceID)
	if err != nil {
		return "", err
	}
	if _, err := verifiedE2EEDeviceTx(tx, params.RecipientDeviceID); err != nil {
		return "", err
	}
	id := "etp_" + ulid.New(time.UnixMilli(params.CreatedAt))
	if _, err := tx.Exec(`INSERT INTO e2ee_transfer_packages(
id, group_id, package_kind, issuer_device_id, recipient_device_id, epoch,
encrypted_package, package_digest, status, revision, created_at, expires_at, terminal_at
) SELECT ?, id, ?, ?, ?, ?, ?, ?, 'pending', 1, ?, ?, 0
FROM e2ee_groups WHERE id = ? AND current_epoch = ? AND fork_state = 'clean'`, id,
		params.PackageKind, params.IssuerDeviceID, params.RecipientDeviceID,
		params.Epoch, params.EncryptedPackage, params.PackageDigest,
		params.CreatedAt, params.ExpiresAt, params.GroupID, params.Epoch); err != nil {
		return "", err
	}
	var inserted int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM e2ee_transfer_packages WHERE id = ?`, id).Scan(&inserted); err != nil {
		return "", err
	}
	if inserted != 1 {
		return "", ErrE2EEStaleEpoch
	}
	if err := appendE2EEAuditTx(tx, params.GroupID, "transfer_package", id,
		"transfer_package.create", "accepted", "", actorID, params.IssuerDeviceID,
		params.Epoch, 1, params.CreatedAt); err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Store) CreateE2EEReportEvidenceMetadata(params CreateE2EEReportEvidenceParams) (string, error) {
	created, err := s.AttachE2EEReportEvidence(params)
	if err != nil {
		return "", err
	}
	return created.Evidence.ID, nil
}
