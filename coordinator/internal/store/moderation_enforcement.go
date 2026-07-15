package store

import (
	"database/sql"
	"errors"
)

// ModerationNodeIdentity is the minimum non-secret coordinate required to
// disconnect a live socket and cancel accepted target work after credential
// revocation.
type ModerationNodeIdentity struct {
	OrbitID int64
	ActorID int64
	Slot    string
}

type ModerationDisableResult struct {
	Changed bool
	Nodes   []ModerationNodeIdentity
}

func moderationNodesForActorTx(tx *sql.Tx, actorID int64) ([]ModerationNodeIdentity, error) {
	rows, err := tx.Query(`SELECT ic.slot_orbit_id, ic.actor_id, ic.slot_name
FROM installation_credentials ic WHERE ic.actor_id = ?
ORDER BY ic.slot_orbit_id, ic.slot_name`, actorID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nodes []ModerationNodeIdentity
	for rows.Next() {
		var node ModerationNodeIdentity
		if err := rows.Scan(&node.OrbitID, &node.ActorID, &node.Slot); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

func moderationNodesForOrbitTx(tx *sql.Tx, orbitID int64) ([]ModerationNodeIdentity, error) {
	rows, err := tx.Query(`SELECT ic.slot_orbit_id, ic.actor_id, ic.slot_name
FROM installation_credentials ic WHERE ic.slot_orbit_id = ?
ORDER BY ic.slot_name`, orbitID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var nodes []ModerationNodeIdentity
	for rows.Next() {
		var node ModerationNodeIdentity
		if err := rows.Scan(&node.OrbitID, &node.ActorID, &node.Slot); err != nil {
			return nil, err
		}
		nodes = append(nodes, node)
	}
	return nodes, rows.Err()
}

// DisableActorForModeration is the canonical identity revocation mutation.
// It revokes the actor and every installation binding in one writer
// transaction. Historical memberships remain for report attribution.
func (s *Store) DisableActorForModeration(actorID, now int64) (ModerationDisableResult, error) {
	if actorID <= 0 || now <= 0 {
		return ModerationDisableResult{}, ErrModerationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return ModerationDisableResult{}, err
	}
	defer tx.Rollback()
	nodes, err := moderationNodesForActorTx(tx, actorID)
	if err != nil {
		return ModerationDisableResult{}, err
	}
	var revokedAt sql.NullInt64
	if err := tx.QueryRow(`SELECT revoked_at FROM actors WHERE id = ?`, actorID).Scan(&revokedAt); errors.Is(err, sql.ErrNoRows) {
		return ModerationDisableResult{}, ErrModerationNotFound
	} else if err != nil {
		return ModerationDisableResult{}, err
	}
	changed := !revokedAt.Valid
	if _, err := tx.Exec(`UPDATE actors SET revoked_at = COALESCE(revoked_at, ?) WHERE id = ?`, now, actorID); err != nil {
		return ModerationDisableResult{}, err
	}
	if _, err := tx.Exec(`UPDATE slots SET revoked_at = COALESCE(revoked_at, ?)
WHERE (orbit_id, slot) IN (
  SELECT slot_orbit_id, slot_name FROM installation_credentials WHERE actor_id = ?
)`, now, actorID); err != nil {
		return ModerationDisableResult{}, err
	}
	if _, err := tx.Exec(`UPDATE device_invites SET consumed_at = COALESCE(consumed_at, ?)
WHERE issued_by_actor_id = ?`, now, actorID); err != nil {
		return ModerationDisableResult{}, err
	}
	if _, err := tx.Exec(`UPDATE telegram_link_codes SET invalidated_at = COALESCE(invalidated_at, ?)
WHERE issuer_actor_id = ?`, now, actorID); err != nil {
		return ModerationDisableResult{}, err
	}
	if err := revokeTransmissionInboxByActorTx(tx, actorID, now); err != nil {
		return ModerationDisableResult{}, err
	}
	if err := s.checkpoint("moderation_disable_actor_before_commit"); err != nil {
		return ModerationDisableResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ModerationDisableResult{}, err
	}
	return ModerationDisableResult{Changed: changed, Nodes: nodes}, nil
}

// DisableOrbitForModeration is the canonical tenant disable mutation. The
// status gate denies every future actor and media fetch; slot revocation makes
// the same decision enforceable by the live hub and legacy rollback projection.
func (s *Store) DisableOrbitForModeration(orbitID, now int64) (ModerationDisableResult, error) {
	if orbitID <= 0 || now <= 0 {
		return ModerationDisableResult{}, ErrModerationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return ModerationDisableResult{}, err
	}
	defer tx.Rollback()
	nodes, err := moderationNodesForOrbitTx(tx, orbitID)
	if err != nil {
		return ModerationDisableResult{}, err
	}
	var status string
	if err := tx.QueryRow(`SELECT status FROM orbits WHERE id = ?`, orbitID).Scan(&status); errors.Is(err, sql.ErrNoRows) {
		return ModerationDisableResult{}, ErrModerationNotFound
	} else if err != nil {
		return ModerationDisableResult{}, err
	}
	changed := status != "disabled"
	if _, err := tx.Exec(`UPDATE orbits SET status = 'disabled' WHERE id = ?`, orbitID); err != nil {
		return ModerationDisableResult{}, err
	}
	if _, err := tx.Exec(`UPDATE slots SET revoked_at = COALESCE(revoked_at, ?)
WHERE orbit_id = ?`, now, orbitID); err != nil {
		return ModerationDisableResult{}, err
	}
	if _, err := tx.Exec(`UPDATE invites SET used_at = COALESCE(used_at, ?) WHERE orbit_id = ?`, now, orbitID); err != nil {
		return ModerationDisableResult{}, err
	}
	if _, err := tx.Exec(`UPDATE device_invites SET consumed_at = COALESCE(consumed_at, ?) WHERE orbit_id = ?`, now, orbitID); err != nil {
		return ModerationDisableResult{}, err
	}
	if _, err := tx.Exec(`UPDATE telegram_link_codes SET invalidated_at = COALESCE(invalidated_at, ?) WHERE orbit_id = ?`, now, orbitID); err != nil {
		return ModerationDisableResult{}, err
	}
	if err := revokeTransmissionInboxByOrbitTx(tx, orbitID, now); err != nil {
		return ModerationDisableResult{}, err
	}
	if err := s.checkpoint("moderation_disable_orbit_before_commit"); err != nil {
		return ModerationDisableResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ModerationDisableResult{}, err
	}
	return ModerationDisableResult{Changed: changed, Nodes: nodes}, nil
}

// DeleteMediaForModeration reuses the exact media terminal transition and
// delivery-cancellation outbox as owner deletion. Replays of an existing
// tombstone are successful and refresh no policy state.
func (s *Store) DeleteMediaForModeration(mediaID string, now int64) (MediaItem, error) {
	if !mediaItemIDPattern.MatchString(mediaID) || now <= 0 {
		return MediaItem{}, ErrMediaNotFound
	}
	tx, err := s.db.Begin()
	if err != nil {
		return MediaItem{}, err
	}
	defer tx.Rollback()
	item, err := scanMediaItem(tx.QueryRow(
		`SELECT `+mediaItemColumns+` FROM media_items WHERE id = ?`, mediaID,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return MediaItem{}, ErrMediaNotFound
	}
	if err != nil {
		return MediaItem{}, err
	}
	if item.Status == MediaStatusExpired {
		return item, tx.Commit()
	}
	if item.Status == MediaStatusDeleted {
		if err := scheduleMediaDeliveryCancellationTx(
			tx, item.ID, item.Revision, MediaCancellationDeleted, item.DeletedAt,
		); err != nil {
			return MediaItem{}, err
		}
		return item, tx.Commit()
	}
	deleted, err := transitionMediaTerminalTx(
		tx, item.ID, item.Revision, MediaStatusDeleted, "", now, false,
	)
	if err != nil {
		return MediaItem{}, err
	}
	if err := s.checkpoint("media_moderation_delete_before_commit"); err != nil {
		return MediaItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaItem{}, err
	}
	return deleted, nil
}
