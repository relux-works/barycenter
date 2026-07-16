package store

import (
	"database/sql"
	"errors"
	"time"

	"relux.works/duet/coordinator/internal/ulid"
)

type SavedCueControlMutation struct {
	Cue           SavedCue `json:"cue"`
	OrderRevision int64    `json:"order_revision"`
	Replayed      bool     `json:"replayed"`
}

type CreateSavedCueControlParams struct {
	Title          string
	MediaID        string
	BuiltinAssetID string
	BuiltinSHA256  string
}

func ensureSavedCueOrderTx(tx *sql.Tx, ctx ActorContext, now int64) (int64, error) {
	var revision int64
	err := tx.QueryRow(`SELECT revision FROM saved_cue_order_state
WHERE owner_orbit_id = ?`, ctx.OrbitID).Scan(&revision)
	if err == nil {
		return revision, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	rows, err := tx.Query(`SELECT id FROM saved_cues
WHERE owner_orbit_id = ? AND state = 'active' ORDER BY created_at, id`, ctx.OrbitID)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`INSERT INTO saved_cue_order_state(
  owner_orbit_id, revision, updated_by_actor_id, updated_at
) VALUES(?, 1, ?, ?)`, ctx.OrbitID, ctx.ActorID, now); err != nil {
		return 0, err
	}
	for position, id := range ids {
		if _, err := tx.Exec(`INSERT INTO saved_cue_order_items(
  owner_orbit_id, cue_id, position
) VALUES(?, ?, ?)`, ctx.OrbitID, id, position); err != nil {
			return 0, err
		}
	}
	return 1, nil
}

func appendSavedCueOrderTx(tx *sql.Tx, ctx ActorContext, cueID string, now int64) (int64, error) {
	revision, err := ensureSavedCueOrderTx(tx, ctx, now)
	if err != nil {
		return 0, err
	}
	var position int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM saved_cue_order_items
WHERE owner_orbit_id = ?`, ctx.OrbitID).Scan(&position); err != nil {
		return 0, err
	}
	_, err = tx.Exec(`INSERT OR IGNORE INTO saved_cue_order_items(
  owner_orbit_id, cue_id, position
) VALUES(?, ?, ?)`, ctx.OrbitID, cueID, position)
	return revision, err
}

func compactSavedCueOrderTx(tx *sql.Tx, ctx ActorContext, now int64) (int64, error) {
	current, err := ensureSavedCueOrderTx(tx, ctx, now)
	if err != nil {
		return 0, err
	}
	rows, err := tx.Query(`SELECT o.cue_id FROM saved_cue_order_items o
JOIN saved_cues c ON c.id = o.cue_id
WHERE o.owner_orbit_id = ? AND c.state = 'active'
ORDER BY o.position, o.cue_id`, ctx.OrbitID)
	if err != nil {
		return 0, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return 0, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if _, err := tx.Exec(`DELETE FROM saved_cue_order_items WHERE owner_orbit_id = ?`, ctx.OrbitID); err != nil {
		return 0, err
	}
	for position, id := range ids {
		if _, err := tx.Exec(`INSERT INTO saved_cue_order_items(
  owner_orbit_id, cue_id, position
) VALUES(?, ?, ?)`, ctx.OrbitID, id, position); err != nil {
			return 0, err
		}
	}
	result, err := tx.Exec(`UPDATE saved_cue_order_state SET revision = revision + 1,
updated_by_actor_id = ?, updated_at = ? WHERE owner_orbit_id = ? AND revision = ?`,
		ctx.ActorID, now, ctx.OrbitID, current)
	if err != nil {
		return 0, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return 0, err
		}
		return 0, ErrSavedCueStateConflict
	}
	return current + 1, nil
}

func (s *Store) CreateAuthorizedSavedCue(auth AutomationControlAuth, params CreateSavedCueControlParams) (SavedCueControlMutation, error) {
	const operation = "saved_cue.create.v1"
	state, auth, err := s.beginAutomationControlMutation(auth, operation)
	if err != nil {
		return SavedCueControlMutation{}, err
	}
	defer state.tx.Rollback()
	if state.replayed {
		result, err := replayAutomationControlMutation[SavedCueControlMutation](state)
		result.Replayed = true
		return result, err
	}
	title, valid := validSavedCueTitle(params.Title)
	if !valid {
		return SavedCueControlMutation{}, ErrSavedCueInvalid
	}
	if err := requireCurrentContentPolicyTx(state.tx, state.ctx); err != nil {
		return SavedCueControlMutation{}, err
	}
	source, err := resolveSavedCueSourceTx(state.tx, state.ctx.OrbitID, SavedCueMutationParams{
		Title: title, MediaID: params.MediaID, BuiltinAssetID: params.BuiltinAssetID,
		BuiltinSHA256: params.BuiltinSHA256, OccurredAt: auth.Now,
	})
	if err != nil {
		return SavedCueControlMutation{}, err
	}
	if existing, err := existingSavedCueForSourceTx(state.tx, state.ctx.OrbitID, source, ""); err != nil {
		return SavedCueControlMutation{}, err
	} else if existing != nil {
		revision, err := appendSavedCueOrderTx(state.tx, state.ctx, existing.ID, auth.Now)
		if err != nil {
			return SavedCueControlMutation{}, err
		}
		result := SavedCueControlMutation{Cue: *existing, OrderRevision: revision}
		if err := finishAutomationControlMutation(state, auth, operation, existing.ID, result); err != nil {
			return SavedCueControlMutation{}, err
		}
		return result, nil
	}
	usage, err := savedCueUsageTx(state.tx, state.ctx.OrbitID, "")
	if err != nil {
		return SavedCueControlMutation{}, err
	}
	if err := enforceSavedCueQuota(usage, source); err != nil {
		return SavedCueControlMutation{}, err
	}
	id := newSavedCueID(auth.Now)
	var mediaID any
	if source.mediaID != "" {
		mediaID = source.mediaID
	}
	_, err = state.tx.Exec(`INSERT INTO saved_cues(
  id, owner_orbit_id, created_by_actor_id, title, source_kind, media_id,
  media_revision, builtin_asset_id, builtin_sha256, source_sha256,
  source_bytes, source_duration_ms, created_at, updated_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, id, state.ctx.OrbitID,
		state.ctx.ActorID, title, source.kind, mediaID, source.mediaRev, source.builtinID,
		source.builtinSHA, source.sha, source.bytes, source.durationMS, auth.Now, auth.Now)
	if err != nil {
		return SavedCueControlMutation{}, err
	}
	cue, err := scanSavedCue(state.tx.QueryRow(`SELECT `+savedCueColumns+`
FROM saved_cues WHERE id = ?`, id))
	if err != nil {
		return SavedCueControlMutation{}, err
	}
	if err := insertSavedCueAuditTx(state.tx, cue, state.ctx.ActorID, "saved_cue.created", auth.Now); err != nil {
		return SavedCueControlMutation{}, err
	}
	orderRevision, err := appendSavedCueOrderTx(state.tx, state.ctx, cue.ID, auth.Now)
	if err != nil {
		return SavedCueControlMutation{}, err
	}
	result := SavedCueControlMutation{Cue: cue, OrderRevision: orderRevision}
	if err := s.checkpoint("automation_control_saved_cue_create_before_commit"); err != nil {
		return SavedCueControlMutation{}, err
	}
	if err := finishAutomationControlMutation(state, auth, operation, cue.ID, result); err != nil {
		return SavedCueControlMutation{}, err
	}
	return result, nil
}

func newSavedCueID(now int64) string {
	return ulid.NewSavedCueID(time.UnixMilli(now))
}

func (s *Store) RenameAuthorizedSavedCue(auth AutomationControlAuth, cueID, title string, expectedRevision int64) (SavedCueControlMutation, error) {
	const operation = "saved_cue.rename.v1"
	state, auth, err := s.beginAutomationControlMutation(auth, operation)
	if err != nil {
		return SavedCueControlMutation{}, err
	}
	defer state.tx.Rollback()
	if state.replayed {
		result, err := replayAutomationControlMutation[SavedCueControlMutation](state)
		result.Replayed = true
		return result, err
	}
	title, valid := validSavedCueTitle(title)
	if !valid || expectedRevision <= 0 {
		return SavedCueControlMutation{}, ErrSavedCueInvalid
	}
	cue, err := savedCueOwnedTx(state.tx, cueID, state.ctx.OrbitID)
	if err != nil {
		return SavedCueControlMutation{}, err
	}
	if cue.State != SavedCueActive || cue.Revision != expectedRevision {
		return SavedCueControlMutation{}, ErrSavedCueStateConflict
	}
	if _, err := state.tx.Exec(`UPDATE saved_cues SET title = ?, revision = revision + 1,
updated_at = ? WHERE id = ? AND revision = ?`, title, auth.Now, cue.ID, cue.Revision); err != nil {
		return SavedCueControlMutation{}, err
	}
	cue, err = scanSavedCue(state.tx.QueryRow(`SELECT `+savedCueColumns+`
FROM saved_cues WHERE id = ?`, cue.ID))
	if err != nil {
		return SavedCueControlMutation{}, err
	}
	if err := insertSavedCueAuditTx(state.tx, cue, state.ctx.ActorID, "saved_cue.renamed", auth.Now); err != nil {
		return SavedCueControlMutation{}, err
	}
	orderRevision, err := ensureSavedCueOrderTx(state.tx, state.ctx, auth.Now)
	if err != nil {
		return SavedCueControlMutation{}, err
	}
	result := SavedCueControlMutation{Cue: cue, OrderRevision: orderRevision}
	if err := finishAutomationControlMutation(state, auth, operation, cue.ID, result); err != nil {
		return SavedCueControlMutation{}, err
	}
	return result, nil
}

type SavedCueOrderMutation struct {
	OrderRevision int64    `json:"order_revision"`
	CueIDs        []string `json:"cue_ids"`
	Replayed      bool     `json:"replayed"`
}

func (s *Store) ReorderAuthorizedSavedCues(auth AutomationControlAuth, cueIDs []string, expectedRevision int64) (SavedCueOrderMutation, error) {
	const operation = "saved_cue.reorder.v1"
	state, auth, err := s.beginAutomationControlMutation(auth, operation)
	if err != nil {
		return SavedCueOrderMutation{}, err
	}
	defer state.tx.Rollback()
	if state.replayed {
		result, err := replayAutomationControlMutation[SavedCueOrderMutation](state)
		result.Replayed = true
		return result, err
	}
	current, err := ensureSavedCueOrderTx(state.tx, state.ctx, auth.Now)
	if err != nil {
		return SavedCueOrderMutation{}, err
	}
	if expectedRevision != current {
		return SavedCueOrderMutation{}, ErrSavedCueStateConflict
	}
	rows, err := state.tx.Query(`SELECT id FROM saved_cues
WHERE owner_orbit_id = ? AND state = 'active' ORDER BY id`, state.ctx.OrbitID)
	if err != nil {
		return SavedCueOrderMutation{}, err
	}
	active := make(map[string]struct{})
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return SavedCueOrderMutation{}, err
		}
		active[id] = struct{}{}
	}
	if err := rows.Close(); err != nil || len(active) != len(cueIDs) {
		if err != nil {
			return SavedCueOrderMutation{}, err
		}
		return SavedCueOrderMutation{}, ErrSavedCueInvalid
	}
	seen := make(map[string]struct{}, len(cueIDs))
	for _, id := range cueIDs {
		if _, ok := active[id]; !ok {
			return SavedCueOrderMutation{}, ErrSavedCueNotFound
		}
		if _, duplicate := seen[id]; duplicate {
			return SavedCueOrderMutation{}, ErrSavedCueInvalid
		}
		seen[id] = struct{}{}
	}
	if _, err := state.tx.Exec(`DELETE FROM saved_cue_order_items WHERE owner_orbit_id = ?`, state.ctx.OrbitID); err != nil {
		return SavedCueOrderMutation{}, err
	}
	for position, id := range cueIDs {
		if _, err := state.tx.Exec(`INSERT INTO saved_cue_order_items(
  owner_orbit_id, cue_id, position
) VALUES(?, ?, ?)`, state.ctx.OrbitID, id, position); err != nil {
			return SavedCueOrderMutation{}, err
		}
	}
	if _, err := state.tx.Exec(`UPDATE saved_cue_order_state SET revision = revision + 1,
updated_by_actor_id = ?, updated_at = ? WHERE owner_orbit_id = ? AND revision = ?`,
		state.ctx.ActorID, auth.Now, state.ctx.OrbitID, current); err != nil {
		return SavedCueOrderMutation{}, err
	}
	result := SavedCueOrderMutation{
		OrderRevision: current + 1, CueIDs: append([]string(nil), cueIDs...),
	}
	if err := finishAutomationControlMutation(state, auth, operation, "", result); err != nil {
		return SavedCueOrderMutation{}, err
	}
	return result, nil
}

func (s *Store) DeleteAuthorizedSavedCue(auth AutomationControlAuth, cueID string, expectedRevision int64) (SavedCueControlMutation, error) {
	const operation = "saved_cue.delete.v1"
	state, auth, err := s.beginAutomationControlMutation(auth, operation)
	if err != nil {
		return SavedCueControlMutation{}, err
	}
	defer state.tx.Rollback()
	if state.replayed {
		result, err := replayAutomationControlMutation[SavedCueControlMutation](state)
		result.Replayed = true
		return result, err
	}
	if expectedRevision <= 0 {
		return SavedCueControlMutation{}, ErrSavedCueInvalid
	}
	cue, err := savedCueOwnedTx(state.tx, cueID, state.ctx.OrbitID)
	if err != nil {
		return SavedCueControlMutation{}, err
	}
	if cue.State == SavedCueDeleted {
		return SavedCueControlMutation{}, ErrSavedCueStateConflict
	}
	if cue.State != SavedCueActive || cue.Revision != expectedRevision {
		return SavedCueControlMutation{}, ErrSavedCueStateConflict
	}
	if err := scheduleSavedCueRevocationTx(state.tx, cue, "cue_deleted", auth.Now); err != nil {
		return SavedCueControlMutation{}, err
	}
	_, err = state.tx.Exec(`UPDATE saved_cues SET state = 'deleted',
revoke_reason = 'cue_deleted', revision = revision + 1,
source_generation = source_generation + 1, updated_at = ?, deleted_at = ?
WHERE id = ? AND revision = ?`, auth.Now, auth.Now, cue.ID, cue.Revision)
	if err != nil {
		return SavedCueControlMutation{}, err
	}
	mediaID := cue.MediaID
	cue, err = scanSavedCue(state.tx.QueryRow(`SELECT `+savedCueColumns+`
FROM saved_cues WHERE id = ?`, cue.ID))
	if err != nil {
		return SavedCueControlMutation{}, err
	}
	if err := insertSavedCueAuditTx(state.tx, cue, state.ctx.ActorID, "saved_cue.deleted", auth.Now); err != nil {
		return SavedCueControlMutation{}, err
	}
	if err := expireUnpinnedSavedCueMediaTx(state.tx, mediaID, auth.Now); err != nil {
		return SavedCueControlMutation{}, err
	}
	orderRevision, err := compactSavedCueOrderTx(state.tx, state.ctx, auth.Now)
	if err != nil {
		return SavedCueControlMutation{}, err
	}
	result := SavedCueControlMutation{Cue: cue, OrderRevision: orderRevision}
	if err := s.checkpoint("automation_control_saved_cue_delete_before_commit"); err != nil {
		return SavedCueControlMutation{}, err
	}
	if err := finishAutomationControlMutation(state, auth, operation, cue.ID, result); err != nil {
		return SavedCueControlMutation{}, err
	}
	return result, nil
}
