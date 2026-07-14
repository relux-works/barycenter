package store

// mediaIngestSchema is deliberately additive. The legacy media table remains
// authoritative for the old-binary/Telegram WAV path during mixed rollout.
//
// owner_orbit_id and actor_id intentionally do not declare foreign keys to the
// legacy authority tables. A previous coordinator does not know about these
// rows and must still be able to run, mutate legacy state, and roll forward.
// Repository writes verify the live orbit/actor membership in the same writer
// transaction instead. All references wholly inside this additive model use
// foreign keys because old binaries never delete these rows.
const mediaIngestSchema = `
CREATE TABLE IF NOT EXISTS media_items (
  id TEXT PRIMARY KEY
    CHECK(length(id) = 28 AND substr(id, 1, 2) = 'm_'),
  owner_orbit_id INTEGER NOT NULL CHECK(owner_orbit_id > 0),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  kind TEXT NOT NULL
    CHECK(kind IN ('voice_clip', 'audio_clip', 'audio_track', 'builtin_cue')),
  source TEXT NOT NULL
    CHECK(source IN ('app', 'telegram', 'system')),
  title TEXT NOT NULL DEFAULT '' CHECK(length(title) <= 512),
  mime TEXT NOT NULL DEFAULT '' CHECK(length(mime) <= 128),
  codec TEXT NOT NULL DEFAULT '' CHECK(length(codec) <= 64),
  duration_ms INTEGER NOT NULL DEFAULT 0 CHECK(duration_ms >= 0),
  size_bytes INTEGER NOT NULL DEFAULT 0 CHECK(size_bytes >= 0),
  sha256 TEXT NOT NULL DEFAULT ''
    CHECK(sha256 = '' OR (
      length(sha256) = 64 AND sha256 NOT GLOB '*[^0-9a-f]*'
    )),
  storage_key TEXT NOT NULL DEFAULT ''
    CHECK(storage_key = '' OR (
      length(storage_key) = 73
      AND substr(storage_key, 1, 9) = 'media/v1/'
      AND substr(storage_key, 10) NOT GLOB '*[^0-9a-f]*'
    )),
  loudness_json TEXT NOT NULL DEFAULT '' CHECK(length(loudness_json) <= 16384),
  status TEXT NOT NULL DEFAULT 'processing'
    CHECK(status IN ('processing', 'ready', 'failed', 'deleted', 'expired')),
  failure_code TEXT NOT NULL DEFAULT '' CHECK(length(failure_code) <= 64),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at),
  expires_at INTEGER NOT NULL CHECK(expires_at > created_at),
  published_at INTEGER NOT NULL DEFAULT 0 CHECK(published_at >= 0),
  deleted_at INTEGER NOT NULL DEFAULT 0 CHECK(deleted_at >= 0),
  UNIQUE(id, owner_orbit_id, actor_id),
  CHECK(status <> 'ready' OR (
    storage_key <> '' AND size_bytes > 0 AND sha256 <> '' AND published_at > 0
  )),
  CHECK(status NOT IN ('failed', 'deleted', 'expired') OR storage_key = ''),
  CHECK(
    (status = 'failed' AND failure_code <> '')
    OR (status <> 'failed' AND failure_code = '')
  ),
  CHECK(
    (status IN ('deleted', 'expired') AND deleted_at > 0)
    OR (status NOT IN ('deleted', 'expired') AND deleted_at = 0)
  )
);
CREATE INDEX IF NOT EXISTS media_items_owner_created
  ON media_items(owner_orbit_id, created_at DESC);
CREATE INDEX IF NOT EXISTS media_items_retention
  ON media_items(status, expires_at);
CREATE UNIQUE INDEX IF NOT EXISTS media_items_storage_key
  ON media_items(storage_key) WHERE storage_key <> '';

CREATE TABLE IF NOT EXISTS media_upload_sessions (
  id TEXT PRIMARY KEY
    CHECK(length(id) = 29 AND substr(id, 1, 3) = 'up_'),
  media_id TEXT NOT NULL,
  owner_orbit_id INTEGER NOT NULL CHECK(owner_orbit_id > 0),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  token_hash TEXT NOT NULL UNIQUE
    CHECK(length(token_hash) = 64
      AND token_hash NOT GLOB '*[^0-9a-f]*'),
  idempotency_key_hash TEXT NOT NULL
    CHECK(length(idempotency_key_hash) = 64
      AND idempotency_key_hash NOT GLOB '*[^0-9a-f]*'),
  request_fingerprint TEXT NOT NULL
    CHECK(length(request_fingerprint) = 64
      AND request_fingerprint NOT GLOB '*[^0-9a-f]*'),
  declared_size_bytes INTEGER NOT NULL CHECK(declared_size_bytes > 0),
  received_size_bytes INTEGER NOT NULL DEFAULT 0
    CHECK(received_size_bytes >= 0 AND received_size_bytes <= declared_size_bytes),
  status TEXT NOT NULL DEFAULT 'open'
    CHECK(status IN ('open', 'finalizing', 'completed', 'failed', 'expired')),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at),
  expires_at INTEGER NOT NULL CHECK(expires_at > created_at),
  completed_at INTEGER NOT NULL DEFAULT 0 CHECK(completed_at >= 0),
  UNIQUE(media_id),
  UNIQUE(owner_orbit_id, actor_id, idempotency_key_hash),
  FOREIGN KEY(media_id, owner_orbit_id, actor_id)
    REFERENCES media_items(id, owner_orbit_id, actor_id),
  CHECK(status NOT IN ('finalizing', 'completed')
    OR received_size_bytes = declared_size_bytes),
  CHECK(
    (status = 'completed' AND completed_at > 0)
    OR (status <> 'completed' AND completed_at = 0)
  )
);
CREATE INDEX IF NOT EXISTS media_upload_sessions_expiry
  ON media_upload_sessions(status, expires_at);

CREATE TABLE IF NOT EXISTS media_storage_operations (
  id TEXT PRIMARY KEY
    CHECK(length(id) = 30 AND substr(id, 1, 4) = 'sop_'),
  media_id TEXT NOT NULL REFERENCES media_items(id),
  kind TEXT NOT NULL CHECK(kind IN ('publish', 'cleanup')),
  storage_key TEXT NOT NULL
    CHECK(length(storage_key) = 73
      AND substr(storage_key, 1, 9) = 'media/v1/'
      AND substr(storage_key, 10) NOT GLOB '*[^0-9a-f]*'),
  media_revision INTEGER NOT NULL CHECK(media_revision > 0),
  state TEXT NOT NULL DEFAULT 'pending'
    CHECK(state IN ('pending', 'done', 'cancelled')),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at),
  completed_at INTEGER NOT NULL DEFAULT 0 CHECK(completed_at >= 0),
  UNIQUE(media_id, kind, storage_key),
  CHECK(
    (state = 'pending' AND completed_at = 0)
    OR (state IN ('done', 'cancelled') AND completed_at > 0)
  )
);
CREATE INDEX IF NOT EXISTS media_storage_operations_pending
  ON media_storage_operations(kind, state, created_at);

CREATE TABLE IF NOT EXISTS media_legacy_wav_links (
  media_id TEXT PRIMARY KEY REFERENCES media_items(id),
  legacy_media_id TEXT NOT NULL UNIQUE,
  linked_at INTEGER NOT NULL CHECK(linked_at > 0)
);

CREATE TABLE IF NOT EXISTS media_ingest_audit_events (
  id INTEGER PRIMARY KEY,
  media_id TEXT NOT NULL REFERENCES media_items(id),
  owner_orbit_id INTEGER NOT NULL CHECK(owner_orbit_id > 0),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  event_type TEXT NOT NULL CHECK(event_type <> ''),
  from_status TEXT NOT NULL DEFAULT ''
    CHECK(from_status = '' OR from_status IN (
      'processing', 'ready', 'failed', 'deleted', 'expired'
    )),
  to_status TEXT NOT NULL DEFAULT ''
    CHECK(to_status = '' OR to_status IN (
      'processing', 'ready', 'failed', 'deleted', 'expired'
    )),
  created_at INTEGER NOT NULL CHECK(created_at > 0)
);
CREATE INDEX IF NOT EXISTS media_ingest_audit_media_created
  ON media_ingest_audit_events(media_id, created_at);
`

func (s *Store) initMediaIngestSchema() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(mediaIngestSchema); err != nil {
		return err
	}
	if err := foreignKeyCheck(tx); err != nil {
		return err
	}
	if err := s.checkpoint("media_ingest_ddl_before_commit"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	return s.reconcileOrphanedMediaItems()
}
