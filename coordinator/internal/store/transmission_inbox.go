package store

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"time"

	"relux.works/duet/coordinator/internal/ulid"
)

const DefaultTransmissionInboxTTL = 30 * 24 * time.Hour

var (
	ErrTransmissionInboxNotFound = errors.New("transmission inbox item was not found")
	ErrTransmissionInboxConflict = errors.New("transmission inbox item state changed")
	transmissionInboxIDPattern   = regexp.MustCompile(`^ib_[0-9A-HJKMNP-TV-Z]{26}$`)
)

type TransmissionInboxAvailability string

const (
	TransmissionInboxAvailable   TransmissionInboxAvailability = "available"
	TransmissionInboxDismissed   TransmissionInboxAvailability = "dismissed"
	TransmissionInboxReplayed    TransmissionInboxAvailability = "replayed"
	TransmissionInboxUnavailable TransmissionInboxAvailability = "unavailable"
	TransmissionInboxExpired     TransmissionInboxAvailability = "expired"
)

type TransmissionInboxItem struct {
	ID                       string
	TransmissionID           string
	MediaID                  string
	OrbitID                  int64
	ActorID                  int64
	Slot                     string
	BindingPairedAt          int64
	MediaKind                MediaKind
	RequestedDelivery        TransmissionDelivery
	EffectiveDelivery        TransmissionDelivery
	MissedStatus             TransmissionTargetStatus
	MissedReason             TransmissionReason
	Availability             TransmissionInboxAvailability
	ReplayOfInboxID          string
	ReplayOfTransmissionID   string
	ReplayRootTransmissionID string
	ReplayDepth              int
	Revision                 int64
	CreatedAt                int64
	UpdatedAt                int64
	ExpiresAt                int64
	DismissedAt              int64
	ConsumedAt               int64
	RevokedAt                int64
	RevocationReason         TransmissionReason
}

type TransmissionInboxPageKey struct {
	CreatedAt int64
	ID        string
}

type ListTransmissionInboxParams struct {
	Target ActorContext
	View   string
	Limit  int
	Upper  TransmissionInboxPageKey
	After  TransmissionInboxPageKey
	Now    int64
}

type TransmissionInboxPage struct {
	Items []TransmissionInboxItem
	Upper TransmissionInboxPageKey
	Next  TransmissionInboxPageKey
}

const transmissionInboxColumns = `id, transmission_id, media_id, orbit_id,
actor_id, slot, binding_paired_at, media_kind, requested_delivery,
effective_delivery, missed_status, missed_reason, availability,
replay_of_inbox_id, replay_of_transmission_id, replay_root_transmission_id,
replay_depth, revision, created_at, updated_at, expires_at, dismissed_at,
consumed_at, revoked_at, revocation_reason`

func scanTransmissionInboxItem(row sqlScanner) (TransmissionInboxItem, error) {
	var item TransmissionInboxItem
	err := row.Scan(
		&item.ID, &item.TransmissionID, &item.MediaID, &item.OrbitID,
		&item.ActorID, &item.Slot, &item.BindingPairedAt, &item.MediaKind,
		&item.RequestedDelivery, &item.EffectiveDelivery, &item.MissedStatus,
		&item.MissedReason, &item.Availability, &item.ReplayOfInboxID,
		&item.ReplayOfTransmissionID, &item.ReplayRootTransmissionID,
		&item.ReplayDepth, &item.Revision, &item.CreatedAt, &item.UpdatedAt,
		&item.ExpiresAt, &item.DismissedAt, &item.ConsumedAt, &item.RevokedAt,
		&item.RevocationReason,
	)
	return item, err
}

func eligibleTransmissionInboxReceipt(
	status TransmissionTargetStatus,
	reason TransmissionReason,
) bool {
	switch status {
	case TransmissionTargetMissedOffline:
		return reason == TransmissionReasonOfflineAtAcceptance ||
			reason == TransmissionReasonOfflineBeforePrepare ||
			reason == TransmissionReasonOfflineBeforeStart
	case TransmissionTargetMissedDND:
		return reason == TransmissionReasonLocalDND || reason == TransmissionReasonOrbitDND
	case TransmissionTargetMissedNotReady:
		return reason == TransmissionReasonPrepareDeadline
	case TransmissionTargetFailed:
		return reason == TransmissionReasonConnectionLost ||
			reason == TransmissionReasonDeviceUnavailable ||
			reason == TransmissionReasonAudioGraphFailed
	default:
		return false
	}
}

// createTransmissionInboxItemTx is called only after the exact target receipt
// is durable in the caller's writer transaction.  The unique target tuple is
// the final idempotency boundary, including across coordinator restart.
func createTransmissionInboxItemTx(
	tx *sql.Tx,
	target TransmissionTarget,
	occurredAt int64,
) (*TransmissionInboxItem, error) {
	if !eligibleTransmissionInboxReceipt(target.Status, target.ReasonCode) {
		return nil, nil
	}
	var mediaID string
	var mediaKind MediaKind
	var requested, effective TransmissionDelivery
	var mediaExpiresAt, mediaDeletedAt int64
	var mediaStatus MediaItemStatus
	var sourceActorRevokedAt, targetActorRevokedAt sql.NullInt64
	var sourceOrbitStatus, targetOrbitStatus string
	var replayOfInboxID, replayOfTransmissionID, replayRootTransmissionID string
	var replayDepth int
	err := tx.QueryRow(`SELECT tr.media_id, mi.kind, tr.requested_delivery,
       tr.effective_delivery, mi.expires_at, mi.status, mi.deleted_at,
       source_actor.revoked_at, source_orbit.status,
       target_actor.revoked_at, target_orbit.status,
       COALESCE(rl.replay_of_inbox_id, ''),
       COALESCE(rl.replay_of_transmission_id, ''),
       COALESCE(rl.replay_root_transmission_id, tr.id),
       COALESCE(rl.replay_depth, 0)
FROM transmissions tr
JOIN media_items mi ON mi.id = tr.media_id
JOIN actors source_actor ON source_actor.id = tr.source_actor_id
JOIN orbits source_orbit ON source_orbit.id = tr.source_orbit_id
JOIN actors target_actor ON target_actor.id = ?
JOIN orbits target_orbit ON target_orbit.id = ?
LEFT JOIN transmission_replay_lineage rl ON rl.transmission_id = tr.id
WHERE tr.id = ?`, target.ActorID, target.OrbitID, target.TransmissionID).Scan(
		&mediaID, &mediaKind, &requested, &effective, &mediaExpiresAt,
		&mediaStatus, &mediaDeletedAt, &sourceActorRevokedAt, &sourceOrbitStatus,
		&targetActorRevokedAt, &targetOrbitStatus,
		&replayOfInboxID, &replayOfTransmissionID, &replayRootTransmissionID,
		&replayDepth,
	)
	if err != nil {
		return nil, err
	}
	expiresAt := occurredAt + int64(DefaultTransmissionInboxTTL/time.Millisecond)
	if mediaExpiresAt < expiresAt {
		expiresAt = mediaExpiresAt
	}
	availability := TransmissionInboxAvailable
	revokedAt := int64(0)
	revocationReason := TransmissionReason("")
	if expiresAt <= occurredAt {
		expiresAt = occurredAt
		availability = TransmissionInboxExpired
	}
	if mediaStatus == MediaStatusDeleted || mediaStatus == MediaStatusExpired {
		availability = TransmissionInboxUnavailable
		revokedAt = mediaDeletedAt
		if revokedAt == 0 {
			revokedAt = occurredAt
		}
		revocationReason = TransmissionReasonMediaDeleted
		if mediaStatus == MediaStatusExpired {
			revocationReason = TransmissionReasonMediaExpired
		}
	} else if sourceActorRevokedAt.Valid || targetActorRevokedAt.Valid ||
		sourceOrbitStatus != "active" || targetOrbitStatus != "active" {
		availability = TransmissionInboxUnavailable
		revokedAt = occurredAt
		if sourceActorRevokedAt.Valid && sourceActorRevokedAt.Int64 > revokedAt {
			revokedAt = sourceActorRevokedAt.Int64
		}
		if targetActorRevokedAt.Valid && targetActorRevokedAt.Int64 > revokedAt {
			revokedAt = targetActorRevokedAt.Int64
		}
		revocationReason = TransmissionReasonModerationDisabled
	}
	updatedAt := occurredAt
	if revokedAt > updatedAt {
		updatedAt = revokedAt
	}
	id := ulid.NewInboxID(time.UnixMilli(occurredAt))
	if _, err := tx.Exec(`INSERT INTO transmission_inbox_items(
  id, transmission_id, media_id, orbit_id, actor_id, slot, binding_paired_at,
  media_kind, requested_delivery, effective_delivery, missed_status,
  missed_reason, availability, replay_of_inbox_id, replay_of_transmission_id,
  replay_root_transmission_id, replay_depth, revision, created_at, updated_at,
  expires_at, revoked_at, revocation_reason
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?)
ON CONFLICT(transmission_id, orbit_id, actor_id, slot, binding_paired_at)
DO NOTHING`, id, target.TransmissionID, mediaID, target.OrbitID,
		target.ActorID, target.Slot, target.BindingPairedAt, mediaKind, requested,
		effective, target.Status, target.ReasonCode, availability,
		replayOfInboxID, replayOfTransmissionID, replayRootTransmissionID,
		replayDepth, occurredAt, updatedAt, expiresAt, revokedAt,
		revocationReason); err != nil {
		return nil, err
	}
	item, err := scanTransmissionInboxItem(tx.QueryRow(
		`SELECT `+transmissionInboxColumns+` FROM transmission_inbox_items
WHERE transmission_id = ? AND orbit_id = ? AND actor_id = ? AND slot = ?
  AND binding_paired_at = ?`, target.TransmissionID, target.OrbitID,
		target.ActorID, target.Slot, target.BindingPairedAt,
	))
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func backfillTransmissionInboxItemsTx(tx *sql.Tx) error {
	rows, err := tx.Query(`SELECT ` + transmissionTargetColumns + `
FROM transmission_targets tt
WHERE tt.last_receipt_at > 0
  AND NOT EXISTS (
    SELECT 1 FROM transmission_inbox_items ii
    WHERE ii.transmission_id = tt.transmission_id
      AND ii.orbit_id = tt.orbit_id AND ii.actor_id = tt.actor_id
      AND ii.slot = tt.slot AND ii.binding_paired_at = tt.binding_paired_at
  )
ORDER BY tt.last_receipt_at, tt.transmission_id, tt.orbit_id, tt.slot`)
	if err != nil {
		return err
	}
	var targets []TransmissionTarget
	for rows.Next() {
		target, err := scanTransmissionTarget(rows)
		if err != nil {
			rows.Close()
			return err
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, target := range targets {
		if _, err := createTransmissionInboxItemTx(tx, target, target.LastReceiptAt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) GetTransmissionInboxItem(id string) (*TransmissionInboxItem, error) {
	if !transmissionInboxIDPattern.MatchString(id) {
		return nil, nil
	}
	item, err := scanTransmissionInboxItem(s.db.QueryRow(
		`SELECT `+transmissionInboxColumns+` FROM transmission_inbox_items WHERE id = ?`, id,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func currentInboxBindingTx(tx *sql.Tx, target ActorContext) (int64, error) {
	if target.ActorID <= 0 || target.OrbitID <= 0 ||
		!transmissionSlotPattern.MatchString(target.Slot) ||
		!target.Capabilities.Has(CapabilityNode) {
		return 0, ErrTransmissionInboxNotFound
	}
	var pairedAt int64
	err := tx.QueryRow(`SELECT ic.slot_paired_at
FROM installation_credentials ic
JOIN actors a ON a.id = ic.actor_id AND a.revoked_at IS NULL
JOIN orbits o ON o.id = ic.slot_orbit_id AND o.status = 'active'
JOIN slots sl ON sl.orbit_id = ic.slot_orbit_id AND sl.slot = ic.slot_name
  AND sl.revoked_at IS NULL AND sl.token_hash = ic.binding_token_hash
  AND COALESCE(sl.paired_at, 0) = ic.slot_paired_at
WHERE ic.actor_id = ? AND ic.slot_orbit_id = ? AND ic.slot_name = ?`,
		target.ActorID, target.OrbitID, target.Slot,
	).Scan(&pairedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, ErrTransmissionInboxNotFound
	}
	return pairedAt, err
}

func validInboxPageKey(key TransmissionInboxPageKey) bool {
	return (key.CreatedAt == 0 && key.ID == "") ||
		(key.CreatedAt > 0 && transmissionInboxIDPattern.MatchString(key.ID))
}

func inboxViewSQL(view string) (string, bool) {
	switch view {
	case "all":
		return "", true
	case "available":
		return " AND availability = 'available'", true
	case "dismissed":
		return " AND availability = 'dismissed'", true
	default:
		return "", false
	}
}

// ListTransmissionInboxItems is a deterministic keyset repository read.  It
// revalidates the exact live installation binding but deliberately never joins
// memberships or Air state, so a later member cannot inherit an older item.
func (s *Store) ListTransmissionInboxItems(
	params ListTransmissionInboxParams,
) (TransmissionInboxPage, error) {
	viewClause, ok := inboxViewSQL(params.View)
	if !ok || params.Limit < 1 || params.Limit > 100 || params.Now <= 0 ||
		!validInboxPageKey(params.Upper) || !validInboxPageKey(params.After) {
		return TransmissionInboxPage{}, fmt.Errorf("%w: invalid inbox page", ErrTransmissionInvalid)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return TransmissionInboxPage{}, err
	}
	defer tx.Rollback()
	pairedAt, err := currentInboxBindingTx(tx, params.Target)
	if err != nil {
		return TransmissionInboxPage{}, err
	}
	if _, err := tx.Exec(`UPDATE transmission_inbox_items
SET availability = 'expired', revision = revision + 1, updated_at = ?
WHERE actor_id = ? AND orbit_id = ? AND slot = ? AND binding_paired_at = ?
  AND expires_at <= ? AND availability <> 'expired'
  AND availability <> 'unavailable'`, params.Now, params.Target.ActorID,
		params.Target.OrbitID, params.Target.Slot, pairedAt, params.Now); err != nil {
		return TransmissionInboxPage{}, err
	}
	upper := params.Upper
	if upper.CreatedAt == 0 {
		err := tx.QueryRow(`SELECT created_at, id FROM transmission_inbox_items
WHERE actor_id = ? AND orbit_id = ? AND slot = ? AND binding_paired_at = ?`+
			viewClause+` ORDER BY created_at DESC, id DESC LIMIT 1`,
			params.Target.ActorID, params.Target.OrbitID, params.Target.Slot,
			pairedAt).Scan(&upper.CreatedAt, &upper.ID)
		if errors.Is(err, sql.ErrNoRows) {
			if err := tx.Commit(); err != nil {
				return TransmissionInboxPage{}, err
			}
			return TransmissionInboxPage{}, nil
		}
		if err != nil {
			return TransmissionInboxPage{}, err
		}
	}
	query := `SELECT ` + transmissionInboxColumns + `
FROM transmission_inbox_items
WHERE actor_id = ? AND orbit_id = ? AND slot = ? AND binding_paired_at = ?
  AND (created_at < ? OR (created_at = ? AND id <= ?))` + viewClause
	args := []any{params.Target.ActorID, params.Target.OrbitID,
		params.Target.Slot, pairedAt, upper.CreatedAt, upper.CreatedAt, upper.ID}
	if params.After.CreatedAt > 0 {
		query += ` AND (created_at < ? OR (created_at = ? AND id < ?))`
		args = append(args, params.After.CreatedAt, params.After.CreatedAt, params.After.ID)
	}
	query += ` ORDER BY created_at DESC, id DESC LIMIT ?`
	args = append(args, params.Limit+1)
	rows, err := tx.Query(query, args...)
	if err != nil {
		return TransmissionInboxPage{}, err
	}
	defer rows.Close()
	items := make([]TransmissionInboxItem, 0, params.Limit+1)
	for rows.Next() {
		item, err := scanTransmissionInboxItem(rows)
		if err != nil {
			return TransmissionInboxPage{}, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return TransmissionInboxPage{}, err
	}
	page := TransmissionInboxPage{Upper: upper}
	if len(items) > params.Limit {
		items = items[:params.Limit]
		last := items[len(items)-1]
		page.Next = TransmissionInboxPageKey{CreatedAt: last.CreatedAt, ID: last.ID}
	}
	page.Items = items
	if err := tx.Commit(); err != nil {
		return TransmissionInboxPage{}, err
	}
	return page, nil
}

// commitTransmissionReplayLineageTx seals lineage and consumes the original
// item in the same transaction that accepted the new transmission.  The
// higher-level replay service supplies the already-created transmission.
func commitTransmissionReplayLineageTx(
	tx *sql.Tx,
	inboxID string,
	replayTransmissionID string,
	now int64,
) error {
	if !transmissionInboxIDPattern.MatchString(inboxID) ||
		!transmissionIDPattern.MatchString(replayTransmissionID) || now <= 0 {
		return ErrTransmissionInboxNotFound
	}
	item, err := scanTransmissionInboxItem(tx.QueryRow(
		`SELECT `+transmissionInboxColumns+` FROM transmission_inbox_items WHERE id = ?`, inboxID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return ErrTransmissionInboxNotFound
	}
	if err != nil {
		return err
	}
	if item.Availability != TransmissionInboxAvailable || item.ExpiresAt <= now ||
		item.ReplayDepth >= 8 {
		return ErrTransmissionInboxConflict
	}
	var matches int
	if err := tx.QueryRow(`SELECT COUNT(*)
FROM transmissions tr
JOIN transmission_targets tt ON tt.transmission_id = tr.id
WHERE tr.id = ? AND tr.media_id = ? AND tt.orbit_id = ? AND tt.actor_id = ?
  AND tt.slot = ? AND tt.binding_paired_at = ?`, replayTransmissionID,
		item.MediaID, item.OrbitID, item.ActorID, item.Slot,
		item.BindingPairedAt).Scan(&matches); err != nil {
		return err
	}
	if matches != 1 {
		return ErrTransmissionInboxConflict
	}
	if _, err := tx.Exec(`INSERT INTO transmission_replay_lineage(
  transmission_id, replay_of_inbox_id, replay_of_transmission_id,
  replay_root_transmission_id, replay_depth, created_at
) VALUES(?, ?, ?, ?, ?, ?)`, replayTransmissionID, item.ID,
		item.TransmissionID, item.ReplayRootTransmissionID,
		item.ReplayDepth+1, now); err != nil {
		return err
	}
	result, err := tx.Exec(`UPDATE transmission_inbox_items
SET availability = 'replayed', consumed_at = ?, revision = revision + 1,
    updated_at = ?
WHERE id = ? AND revision = ? AND availability = 'available'`,
		now, now, item.ID, item.Revision)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return err
		}
		return ErrTransmissionInboxConflict
	}
	return nil
}

func revokeTransmissionInboxByMediaTx(
	tx *sql.Tx,
	mediaID string,
	reason TransmissionReason,
	now int64,
) error {
	_, err := tx.Exec(`UPDATE transmission_inbox_items
SET availability = 'unavailable', revoked_at = ?, revocation_reason = ?,
    revision = revision + 1, updated_at = ?
WHERE media_id = ? AND availability <> 'unavailable'`,
		now, reason, now, mediaID)
	return err
}

func revokeTransmissionInboxByActorTx(tx *sql.Tx, actorID, now int64) error {
	_, err := tx.Exec(`UPDATE transmission_inbox_items
SET availability = 'unavailable', revoked_at = ?,
    revocation_reason = 'moderation_disabled', revision = revision + 1,
    updated_at = ?
WHERE availability <> 'unavailable' AND (
  actor_id = ? OR transmission_id IN (
    SELECT id FROM transmissions WHERE source_actor_id = ?
  )
)`, now, now, actorID, actorID)
	return err
}

func revokeTransmissionInboxByOrbitTx(tx *sql.Tx, orbitID, now int64) error {
	_, err := tx.Exec(`UPDATE transmission_inbox_items
SET availability = 'unavailable', revoked_at = ?,
    revocation_reason = 'moderation_disabled', revision = revision + 1,
    updated_at = ?
WHERE availability <> 'unavailable' AND (
  orbit_id = ? OR transmission_id IN (
    SELECT id FROM transmissions WHERE source_orbit_id = ?
  )
)`, now, now, orbitID, orbitID)
	return err
}
