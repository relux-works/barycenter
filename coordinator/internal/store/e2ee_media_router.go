package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"hash"
)

const (
	e2eeMaxObjectBytes       int64 = 64 << 20
	e2eeMaxObjectChunks      int64 = 1024
	e2eeMaxChunkBytes        int64 = 1 << 20
	e2eeMaxRangeBytes        int64 = 4 << 20
	e2eeMaxConcurrentObjects       = 4
	e2eeMaxDailyObjectBytes  int64 = 512 << 20
	e2eeObjectDailyWindowMS  int64 = 24 * 60 * 60 * 1000
)

var (
	ErrE2EEQuotaExceeded    = errors.New("E2EE ciphertext quota exceeded")
	ErrE2EEChunkOrder       = errors.New("E2EE ciphertext chunk order mismatch")
	ErrE2EEObjectIncomplete = errors.New("E2EE ciphertext object incomplete")
	ErrE2EEIfRangeMismatch  = errors.New("E2EE ciphertext If-Range mismatch")
)

type E2EEProtectedChunk struct {
	ProtectedObjectID, CiphertextDigest               string
	ChunkIndex, ByteOffset, CiphertextSize, CreatedAt int64
	Ciphertext                                        []byte
}

type PutE2EEProtectedChunkParams struct {
	ProtectedObjectID, AuthorDeviceID, CiphertextDigest string
	ChunkIndex, ByteOffset, CreatedAt                   int64
	Ciphertext                                          []byte
}

type E2EEProtectedManifestRoute struct {
	Object                                E2EEProtectedObject
	EncryptedManifest, OpaqueKeyEnvelopes []byte
}

type FetchE2EEProtectedRangeParams struct {
	ProtectedObjectID, RecipientDeviceID   string
	TargetSnapshotDigest, ManifestDigest   string
	Epoch, Generation, Start, EndExclusive int64
	RequestedAt                            int64
	IfRangeManifestDigest                  string
}

type E2EEProtectedRange struct {
	ObjectID, ManifestDigest, CiphertextDigest string
	Start, EndExclusive, TotalSize             int64
	Chunks                                     []E2EEProtectedChunk
}

func chargeE2EEProtectedRangeTx(tx *sql.Tx, deviceID string, actualBytes, now int64) error {
	charged := actualBytes
	if charged < StreamRangeRequestChargeBytes {
		charged = StreamRangeRequestChargeBytes
	}
	var window, used int64
	err := tx.QueryRow(`SELECT window_started_at, charged_bytes
FROM e2ee_protected_egress_usage WHERE recipient_device_id = ?`, deviceID).Scan(&window, &used)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.Exec(`INSERT INTO e2ee_protected_egress_usage(
recipient_device_id, window_started_at, charged_bytes, actual_bytes,
range_requests, revision, updated_at
) VALUES(?, ?, ?, ?, 1, 1, ?)`, deviceID, now, charged, actualBytes, now)
		return err
	}
	if err != nil {
		return err
	}
	if now-window >= e2eeObjectDailyWindowMS {
		_, err = tx.Exec(`UPDATE e2ee_protected_egress_usage
SET window_started_at = ?, charged_bytes = ?, actual_bytes = ?,
range_requests = 1, revision = revision + 1, updated_at = ?
WHERE recipient_device_id = ?`, now, charged, actualBytes, now, deviceID)
		return err
	}
	if now < window || charged > e2eeMaxDailyObjectBytes-used {
		return ErrE2EEQuotaExceeded
	}
	_, err = tx.Exec(`UPDATE e2ee_protected_egress_usage
SET charged_bytes = charged_bytes + ?, actual_bytes = actual_bytes + ?,
range_requests = range_requests + 1, revision = revision + 1, updated_at = ?
WHERE recipient_device_id = ?`, charged, actualBytes, now, deviceID)
	return err
}

func e2eeProtectedObjectCapacityTx(tx *sql.Tx, actorID, requestedBytes, now int64) error {
	var concurrent int
	var used int64
	err := tx.QueryRow(`SELECT
  COALESCE(SUM(CASE WHEN o.status = 'staged' THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN o.created_at > ? AND o.status <> 'deleted'
    THEN o.ciphertext_size ELSE 0 END), 0)
FROM e2ee_protected_objects o
JOIN e2ee_device_public_state d ON d.device_id = o.author_device_id
WHERE d.actor_id = ?`, now-e2eeObjectDailyWindowMS, actorID).Scan(&concurrent, &used)
	if err != nil {
		return err
	}
	if concurrent >= e2eeMaxConcurrentObjects || requestedBytes > e2eeMaxDailyObjectBytes-used {
		return ErrE2EEQuotaExceeded
	}
	return nil
}

func scanE2EEProtectedChunk(scanner sqlScanner) (E2EEProtectedChunk, error) {
	var value E2EEProtectedChunk
	err := scanner.Scan(&value.ProtectedObjectID, &value.ChunkIndex, &value.ByteOffset,
		&value.CiphertextDigest, &value.CiphertextSize, &value.Ciphertext,
		&value.CreatedAt)
	if err == nil {
		value.Ciphertext = append([]byte(nil), value.Ciphertext...)
	}
	return value, err
}

func e2eeProtectedObjectRouterEnabledTx(tx *sql.Tx, objectID string) (bool, error) {
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM e2ee_protected_object_recipients
WHERE protected_object_id = ?`, objectID).Scan(&count); err != nil {
		return false, err
	}
	return count > 0, nil
}

func requireExactCurrentE2EESnapshotTx(tx *sql.Tx, group E2EEGroup) error {
	current, err := e2eeCurrentMembersTx(tx, group.ID)
	if err != nil {
		return err
	}
	snapshot, err := e2eeAirSnapshotTx(tx, group.AirID)
	if err != nil {
		return err
	}
	if len(snapshot.UnsupportedActorIDs) > 0 || snapshot.Digest != group.TargetSnapshotDigest ||
		!sameE2EEMemberSet(current, snapshot.Members) {
		return ErrE2EERotationRequired
	}
	return nil
}

func authorizeE2EEProtectedAuthorTx(tx *sql.Tx, object E2EEProtectedObject, deviceID string) error {
	if object.AuthorDeviceID != deviceID {
		return ErrE2EEInvalid
	}
	group, err := e2eeGroupTx(tx, object.GroupID)
	if err != nil {
		return err
	}
	if group.ForkState != "clean" {
		return ErrE2EEForked
	}
	if err := requireExactCurrentE2EESnapshotTx(tx, group); err != nil {
		return err
	}
	var protocolActorID string
	if err := tx.QueryRow(`SELECT protocol_actor_id FROM e2ee_protocol_actor_bindings
WHERE device_id = ?`, deviceID).Scan(&protocolActorID); err != nil {
		return ErrE2EEInvalid
	}
	if _, err := authorizedE2EEGroupMemberTx(tx, group, deviceID, protocolActorID); err != nil {
		return ErrE2EEInvalid
	}
	if group.CurrentEpoch != object.Epoch || group.TargetSnapshotDigest != object.TargetSnapshotDigest {
		return ErrE2EEStaleEpoch
	}
	return nil
}

func (s *Store) PutE2EEProtectedChunk(params PutE2EEProtectedChunkParams) (E2EEProtectedChunk, error) {
	if len(params.ProtectedObjectID) != 29 || len(params.AuthorDeviceID) < 8 ||
		params.ChunkIndex < 0 || params.ChunkIndex >= e2eeMaxObjectChunks ||
		params.ByteOffset < 0 || params.CreatedAt <= 0 || len(params.Ciphertext) == 0 ||
		int64(len(params.Ciphertext)) > e2eeMaxChunkBytes ||
		!payloadDigestMatches(params.Ciphertext, params.CiphertextDigest) {
		return E2EEProtectedChunk{}, ErrE2EEInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEProtectedChunk{}, err
	}
	defer tx.Rollback()
	object, err := scanE2EEProtectedObject(tx.QueryRow(
		`SELECT `+e2eeProtectedObjectColumns+` FROM e2ee_protected_objects WHERE id = ?`,
		params.ProtectedObjectID))
	if errors.Is(err, sql.ErrNoRows) {
		return E2EEProtectedChunk{}, ErrE2EENotFound
	}
	if err != nil {
		return E2EEProtectedChunk{}, err
	}
	if object.Status != "staged" {
		return E2EEProtectedChunk{}, ErrE2EERevoked
	}
	if enabled, err := e2eeProtectedObjectRouterEnabledTx(tx, object.ID); err != nil {
		return E2EEProtectedChunk{}, err
	} else if !enabled {
		return E2EEProtectedChunk{}, ErrE2EEInvalid
	}
	if err := authorizeE2EEProtectedAuthorTx(tx, object, params.AuthorDeviceID); err != nil {
		return E2EEProtectedChunk{}, err
	}
	var count, received int64
	if err := tx.QueryRow(`SELECT COUNT(*), COALESCE(SUM(ciphertext_size), 0)
FROM e2ee_protected_object_chunks WHERE protected_object_id = ?`, object.ID).Scan(&count, &received); err != nil {
		return E2EEProtectedChunk{}, err
	}
	if params.ChunkIndex < count {
		existing, err := scanE2EEProtectedChunk(tx.QueryRow(`SELECT protected_object_id,
chunk_index, byte_offset, ciphertext_digest, ciphertext_size, ciphertext, created_at
FROM e2ee_protected_object_chunks WHERE protected_object_id = ? AND chunk_index = ?`,
			object.ID, params.ChunkIndex))
		if err != nil {
			return E2EEProtectedChunk{}, err
		}
		if existing.ByteOffset != params.ByteOffset ||
			existing.CiphertextDigest != params.CiphertextDigest ||
			string(existing.Ciphertext) != string(params.Ciphertext) {
			return E2EEProtectedChunk{}, ErrE2EEConflict
		}
		if err := tx.Commit(); err != nil {
			return E2EEProtectedChunk{}, err
		}
		return existing, nil
	}
	if params.ChunkIndex != count || params.ByteOffset != received ||
		count >= object.ChunkCount || received+int64(len(params.Ciphertext)) > object.CiphertextSize {
		return E2EEProtectedChunk{}, ErrE2EEChunkOrder
	}
	if _, err := tx.Exec(`INSERT INTO e2ee_protected_object_chunks(
protected_object_id, chunk_index, byte_offset, ciphertext_size,
ciphertext_digest, ciphertext, created_at
) VALUES(?, ?, ?, ?, ?, ?, ?)`, object.ID, params.ChunkIndex, params.ByteOffset,
		len(params.Ciphertext), params.CiphertextDigest, params.Ciphertext,
		params.CreatedAt); err != nil {
		return E2EEProtectedChunk{}, ErrE2EEConflict
	}
	if err := tx.Commit(); err != nil {
		return E2EEProtectedChunk{}, err
	}
	return E2EEProtectedChunk{ProtectedObjectID: object.ID, ChunkIndex: params.ChunkIndex,
		ByteOffset: params.ByteOffset, CiphertextDigest: params.CiphertextDigest,
		CiphertextSize: int64(len(params.Ciphertext)), Ciphertext: append([]byte(nil), params.Ciphertext...),
		CreatedAt: params.CreatedAt}, nil
}

func validateE2EEProtectedObjectChunksTx(tx *sql.Tx, object E2EEProtectedObject) error {
	rows, err := tx.Query(`SELECT chunk_index, byte_offset, ciphertext_size,
ciphertext_digest, ciphertext FROM e2ee_protected_object_chunks
WHERE protected_object_id = ? ORDER BY chunk_index`, object.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	var index, offset int64
	var digest hash.Hash = sha256.New()
	for rows.Next() {
		var chunkIndex, byteOffset, size int64
		var chunkDigest string
		var ciphertext []byte
		if err := rows.Scan(&chunkIndex, &byteOffset, &size, &chunkDigest, &ciphertext); err != nil {
			return err
		}
		if chunkIndex != index || byteOffset != offset || int64(len(ciphertext)) != size ||
			!payloadDigestMatches(ciphertext, chunkDigest) {
			return ErrE2EEObjectIncomplete
		}
		_, _ = digest.Write(ciphertext)
		index++
		offset += size
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if index != object.ChunkCount || offset != object.CiphertextSize ||
		hex.EncodeToString(digest.Sum(nil)) != object.CiphertextDigest {
		return ErrE2EEObjectIncomplete
	}
	return nil
}

func authorizeE2EEProtectedRecipientTx(tx *sql.Tx, object E2EEProtectedObject, deviceID string) error {
	if object.Status != "ready" {
		if object.Status == "revoked" || object.Status == "deleted" {
			return ErrE2EERevoked
		}
		return ErrE2EEObjectIncomplete
	}
	group, err := e2eeGroupTx(tx, object.GroupID)
	if err != nil {
		return err
	}
	if group.ForkState != "clean" {
		return ErrE2EEForked
	}
	if err := requireExactCurrentE2EESnapshotTx(tx, group); err != nil {
		return err
	}
	var count int
	err = tx.QueryRow(`SELECT COUNT(*)
FROM e2ee_protected_object_recipients r
JOIN e2ee_group_members gm ON gm.group_id = ? AND gm.device_id = r.recipient_device_id
  AND gm.state = 'current' AND gm.actor_id = r.actor_id
  AND gm.protocol_actor_id = r.protocol_actor_id
  AND gm.actor_membership_joined_at = r.actor_membership_joined_at
  AND gm.air_membership_id = r.air_membership_id
  AND gm.air_membership_revision = r.air_membership_revision
JOIN e2ee_device_public_state d ON d.device_id = r.recipient_device_id
  AND d.verification_state = 'verified' AND d.revoked_at = 0
JOIN actors a ON a.id = r.actor_id AND a.revoked_at IS NULL
WHERE r.protected_object_id = ? AND r.recipient_device_id = ?`,
		object.GroupID, object.ID, deviceID).Scan(&count)
	if err != nil {
		return err
	}
	if count != 1 {
		return ErrE2EEInvalid
	}
	return nil
}

func (s *Store) GetAuthorizedE2EEProtectedManifest(objectID, recipientDeviceID string,
	requestedAt int64,
) (E2EEProtectedManifestRoute, error) {
	if len(objectID) != 29 || len(recipientDeviceID) < 8 || requestedAt <= 0 {
		return E2EEProtectedManifestRoute{}, ErrE2EEInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEProtectedManifestRoute{}, err
	}
	defer tx.Rollback()
	object, err := scanE2EEProtectedObject(tx.QueryRow(
		`SELECT `+e2eeProtectedObjectColumns+` FROM e2ee_protected_objects WHERE id = ?`, objectID))
	if errors.Is(err, sql.ErrNoRows) {
		return E2EEProtectedManifestRoute{}, ErrE2EENotFound
	}
	if err != nil {
		return E2EEProtectedManifestRoute{}, err
	}
	if err := authorizeE2EEProtectedRecipientTx(tx, object, recipientDeviceID); err != nil {
		return E2EEProtectedManifestRoute{}, err
	}
	var manifest, envelopes []byte
	if err := tx.QueryRow(`SELECT encrypted_manifest, opaque_key_envelopes
FROM e2ee_protected_objects WHERE id = ?`, object.ID).Scan(&manifest, &envelopes); err != nil {
		return E2EEProtectedManifestRoute{}, err
	}
	if err := chargeE2EEProtectedRangeTx(tx, recipientDeviceID,
		int64(len(manifest)+len(envelopes)), requestedAt); err != nil {
		return E2EEProtectedManifestRoute{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEProtectedManifestRoute{}, err
	}
	return E2EEProtectedManifestRoute{Object: object,
		EncryptedManifest:  append([]byte(nil), manifest...),
		OpaqueKeyEnvelopes: append([]byte(nil), envelopes...)}, nil
}

func (s *Store) FetchAuthorizedE2EEProtectedRange(params FetchE2EEProtectedRangeParams) (E2EEProtectedRange, error) {
	if len(params.ProtectedObjectID) != 29 || len(params.RecipientDeviceID) < 8 ||
		params.Epoch <= 0 || params.Generation <= 0 || params.Start < 0 ||
		params.EndExclusive <= params.Start || params.EndExclusive-params.Start > e2eeMaxRangeBytes ||
		params.RequestedAt <= 0 ||
		!validE2EEDigest(params.TargetSnapshotDigest) || !validE2EEDigest(params.ManifestDigest) ||
		(params.IfRangeManifestDigest != "" && !validE2EEDigest(params.IfRangeManifestDigest)) {
		return E2EEProtectedRange{}, ErrE2EEInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEProtectedRange{}, err
	}
	defer tx.Rollback()
	object, err := scanE2EEProtectedObject(tx.QueryRow(
		`SELECT `+e2eeProtectedObjectColumns+` FROM e2ee_protected_objects WHERE id = ?`,
		params.ProtectedObjectID))
	if errors.Is(err, sql.ErrNoRows) {
		return E2EEProtectedRange{}, ErrE2EENotFound
	}
	if err != nil {
		return E2EEProtectedRange{}, err
	}
	if object.Epoch != params.Epoch || object.Generation != params.Generation ||
		object.TargetSnapshotDigest != params.TargetSnapshotDigest ||
		object.ManifestDigest != params.ManifestDigest {
		return E2EEProtectedRange{}, ErrE2EEStaleEpoch
	}
	if params.IfRangeManifestDigest != "" && params.IfRangeManifestDigest != object.ManifestDigest {
		return E2EEProtectedRange{}, ErrE2EEIfRangeMismatch
	}
	if params.EndExclusive > object.CiphertextSize {
		return E2EEProtectedRange{}, ErrE2EEInvalid
	}
	if err := authorizeE2EEProtectedRecipientTx(tx, object, params.RecipientDeviceID); err != nil {
		return E2EEProtectedRange{}, err
	}
	if err := chargeE2EEProtectedRangeTx(tx, params.RecipientDeviceID,
		params.EndExclusive-params.Start, params.RequestedAt); err != nil {
		return E2EEProtectedRange{}, err
	}
	rows, err := tx.Query(`SELECT protected_object_id, chunk_index, byte_offset,
ciphertext_digest, ciphertext_size, ciphertext, created_at
FROM e2ee_protected_object_chunks
WHERE protected_object_id = ? AND byte_offset >= ?
  AND byte_offset + ciphertext_size <= ? ORDER BY byte_offset`,
		object.ID, params.Start, params.EndExclusive)
	if err != nil {
		return E2EEProtectedRange{}, err
	}
	defer rows.Close()
	result := E2EEProtectedRange{ObjectID: object.ID, ManifestDigest: object.ManifestDigest,
		CiphertextDigest: object.CiphertextDigest, Start: params.Start,
		EndExclusive: params.EndExclusive, TotalSize: object.CiphertextSize}
	next := params.Start
	for rows.Next() {
		chunk, err := scanE2EEProtectedChunk(rows)
		if err != nil {
			return E2EEProtectedRange{}, err
		}
		if chunk.ByteOffset != next || !payloadDigestMatches(chunk.Ciphertext, chunk.CiphertextDigest) {
			return E2EEProtectedRange{}, ErrE2EEObjectIncomplete
		}
		next += chunk.CiphertextSize
		result.Chunks = append(result.Chunks, chunk)
	}
	if err := rows.Err(); err != nil {
		return E2EEProtectedRange{}, err
	}
	if next != params.EndExclusive || len(result.Chunks) == 0 {
		return E2EEProtectedRange{}, ErrE2EEChunkOrder
	}
	if err := tx.Commit(); err != nil {
		return E2EEProtectedRange{}, err
	}
	return result, nil
}

func (s *Store) DeleteE2EEProtectedObject(id, requesterDeviceID string,
	expectedRevision, now int64,
) (E2EEProtectedObject, error) {
	if len(id) != 29 || len(requesterDeviceID) < 8 || expectedRevision <= 0 || now <= 0 {
		return E2EEProtectedObject{}, ErrE2EEInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return E2EEProtectedObject{}, err
	}
	defer tx.Rollback()
	object, err := scanE2EEProtectedObject(tx.QueryRow(
		`SELECT `+e2eeProtectedObjectColumns+` FROM e2ee_protected_objects WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return E2EEProtectedObject{}, ErrE2EENotFound
	}
	if err != nil {
		return E2EEProtectedObject{}, err
	}
	if object.Status == "deleted" {
		return E2EEProtectedObject{}, ErrE2EERevoked
	}
	if object.AuthorDeviceID != requesterDeviceID {
		return E2EEProtectedObject{}, ErrE2EEInvalid
	}
	if _, err := verifiedE2EEDeviceTx(tx, requesterDeviceID); err != nil {
		return E2EEProtectedObject{}, ErrE2EEInvalid
	}
	if object.Revision != expectedRevision {
		return E2EEProtectedObject{}, ErrE2EEConflict
	}
	if _, err := tx.Exec(`DELETE FROM e2ee_protected_object_chunks
WHERE protected_object_id = ?`, object.ID); err != nil {
		return E2EEProtectedObject{}, err
	}
	result, err := tx.Exec(`UPDATE e2ee_protected_objects SET status = 'deleted',
revision = revision + 1, updated_at = ?, deleted_at = ?
WHERE id = ? AND revision = ? AND status <> 'deleted'`, now, now, object.ID,
		expectedRevision)
	if err != nil {
		return E2EEProtectedObject{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return E2EEProtectedObject{}, err
		}
		return E2EEProtectedObject{}, ErrE2EEConflict
	}
	if err := appendE2EEAuditTx(tx, object.GroupID, "protected_object", object.ID,
		"protected_object.delete", "deleted", "server_access_revoked", 0,
		requesterDeviceID, object.Epoch, expectedRevision+1, now); err != nil {
		return E2EEProtectedObject{}, err
	}
	if err := tx.Commit(); err != nil {
		return E2EEProtectedObject{}, err
	}
	return s.GetE2EEProtectedObject(object.ID)
}

func e2eeFrameDigest(raw []byte) string {
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func validateOpaqueReason(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' {
			return false
		}
	}
	return true
}
