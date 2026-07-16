package store

import "database/sql"

// savedCueSchema is an additive, rollback-safe pin registry. A saved cue is a
// reference to canonical media, never a second upload or storage authority.
// Usage is derived from active rows so a crash cannot strand mutable counters.
const savedCueSchema = `
CREATE TABLE IF NOT EXISTS saved_cues (
  id TEXT PRIMARY KEY
    CHECK(length(id) = 29 AND substr(id, 1, 3) = 'cq_'),
  owner_orbit_id INTEGER NOT NULL CHECK(owner_orbit_id > 0),
  created_by_actor_id INTEGER NOT NULL CHECK(created_by_actor_id > 0),
  title TEXT NOT NULL CHECK(length(title) BETWEEN 1 AND 128),
  source_kind TEXT NOT NULL CHECK(source_kind IN ('media', 'builtin')),
  media_id TEXT REFERENCES media_items(id),
  media_revision INTEGER NOT NULL DEFAULT 0 CHECK(media_revision >= 0),
  builtin_asset_id TEXT NOT NULL DEFAULT '' CHECK(length(builtin_asset_id) <= 128),
  builtin_sha256 TEXT NOT NULL DEFAULT '' CHECK(
    builtin_sha256 = '' OR (length(builtin_sha256) = 64
      AND builtin_sha256 NOT GLOB '*[^0-9a-f]*')
  ),
  source_sha256 TEXT NOT NULL CHECK(
    length(source_sha256) = 64 AND source_sha256 NOT GLOB '*[^0-9a-f]*'
  ),
  source_bytes INTEGER NOT NULL CHECK(source_bytes > 0),
  source_duration_ms INTEGER NOT NULL CHECK(source_duration_ms > 0),
  state TEXT NOT NULL DEFAULT 'active'
    CHECK(state IN ('active', 'deleted', 'source_revoked')),
  revoke_reason TEXT NOT NULL DEFAULT '' CHECK(length(revoke_reason) <= 64),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  source_generation INTEGER NOT NULL DEFAULT 1 CHECK(source_generation > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at),
  deleted_at INTEGER NOT NULL DEFAULT 0 CHECK(deleted_at >= 0),
  CHECK(
    (source_kind = 'media' AND media_id IS NOT NULL AND media_revision > 0
      AND builtin_asset_id = '' AND builtin_sha256 = '')
    OR
    (source_kind = 'builtin' AND media_id IS NULL AND media_revision = 0
      AND builtin_asset_id <> '' AND builtin_sha256 = source_sha256)
  ),
  CHECK((state = 'active' AND revoke_reason = '' AND deleted_at = 0)
    OR (state <> 'active' AND revoke_reason <> '' AND deleted_at > 0))
);
CREATE INDEX IF NOT EXISTS saved_cues_owner_state
  ON saved_cues(owner_orbit_id, state, created_at, id);
CREATE INDEX IF NOT EXISTS saved_cues_media_pin
  ON saved_cues(media_id, state) WHERE media_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS saved_cues_active_media_dedupe
  ON saved_cues(owner_orbit_id, media_id)
  WHERE state = 'active' AND source_kind = 'media';
CREATE UNIQUE INDEX IF NOT EXISTS saved_cues_active_builtin_dedupe
  ON saved_cues(owner_orbit_id, builtin_asset_id, builtin_sha256)
  WHERE state = 'active' AND source_kind = 'builtin';

CREATE TABLE IF NOT EXISTS saved_cue_revocations (
  cue_id TEXT NOT NULL REFERENCES saved_cues(id),
  invalidated_generation INTEGER NOT NULL CHECK(invalidated_generation > 0),
  reason TEXT NOT NULL CHECK(reason IN (
    'cue_replaced', 'cue_deleted', 'source_media_deleted',
    'source_media_expired', 'source_actor_disabled',
    'owner_orbit_disabled', 'builtin_version_unsupported'
  )),
  policy_version TEXT NOT NULL CHECK(policy_version = 'saved_cue_lifecycle_v1'),
  pending_action TEXT NOT NULL CHECK(pending_action = 'cancel'),
  active_action TEXT NOT NULL CHECK(active_action = 'fade_stop'),
  interrupted_main_action TEXT NOT NULL
    CHECK(interrupted_main_action = 'resume_once'),
  state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending', 'done')),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at),
  completed_at INTEGER NOT NULL DEFAULT 0 CHECK(completed_at >= 0),
  PRIMARY KEY(cue_id, invalidated_generation),
  CHECK((state = 'pending' AND completed_at = 0)
    OR (state = 'done' AND completed_at > 0))
);
CREATE INDEX IF NOT EXISTS saved_cue_revocations_pending
  ON saved_cue_revocations(state, created_at, cue_id);

CREATE TABLE IF NOT EXISTS saved_cue_audit_events (
  id INTEGER PRIMARY KEY,
  cue_id TEXT NOT NULL REFERENCES saved_cues(id),
  owner_orbit_id INTEGER NOT NULL CHECK(owner_orbit_id > 0),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  event_type TEXT NOT NULL CHECK(event_type <> ''),
  source_generation INTEGER NOT NULL CHECK(source_generation > 0),
  occurred_at INTEGER NOT NULL CHECK(occurred_at > 0)
);
CREATE INDEX IF NOT EXISTS saved_cue_audit_owner_time
  ON saved_cue_audit_events(owner_orbit_id, occurred_at DESC, id DESC);
`

func (s *Store) initSavedCueSchema() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(savedCueSchema); err != nil {
		return err
	}
	if err := foreignKeyCheck(tx); err != nil {
		return err
	}
	return tx.Commit()
}

func savedCuePinExistsTx(tx *sql.Tx, mediaID string) (bool, error) {
	var exists int
	err := tx.QueryRow(`SELECT EXISTS(
  SELECT 1 FROM saved_cues WHERE media_id = ? AND state = 'active'
)`, mediaID).Scan(&exists)
	return exists != 0, err
}
