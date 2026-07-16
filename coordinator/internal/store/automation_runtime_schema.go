package store

// automationRuntimeSchema keeps the attempt guard and builtin-media bridge
// additive. Older coordinators ignore both tables and continue to understand
// every pre-existing cue, automation and transmission row.
const automationRuntimeSchema = `
CREATE TABLE IF NOT EXISTS automation_runtime_attempts (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  trigger_kind TEXT NOT NULL CHECK(trigger_kind IN ('scoped_api', 'schedule')),
  owner_orbit_id INTEGER NOT NULL CHECK(owner_orbit_id > 0),
  principal_id TEXT NOT NULL DEFAULT '' CHECK(
    principal_id = '' OR (length(principal_id) = 29 AND substr(principal_id, 1, 3) = 'ap_')
  ),
  schedule_id TEXT NOT NULL DEFAULT '' CHECK(
    schedule_id = '' OR (length(schedule_id) = 30 AND substr(schedule_id, 1, 4) = 'sch_')
  ),
  schedule_revision INTEGER NOT NULL DEFAULT 0 CHECK(schedule_revision >= 0),
  cue_id TEXT NOT NULL CHECK(length(cue_id) BETWEEN 1 AND 128),
  idempotency_digest TEXT NOT NULL DEFAULT '' CHECK(
    idempotency_digest = '' OR (length(idempotency_digest) = 64
      AND idempotency_digest NOT GLOB '*[^0-9a-f]*')
  ),
  request_digest TEXT NOT NULL DEFAULT '' CHECK(
    request_digest = '' OR (length(request_digest) = 64
      AND request_digest NOT GLOB '*[^0-9a-f]*')
  ),
  occurrence_key TEXT NOT NULL DEFAULT '' CHECK(length(occurrence_key) <= 256),
  attempted_at INTEGER NOT NULL CHECK(attempted_at > 0),
  outcome TEXT NOT NULL CHECK(outcome IN ('reserved', 'accepted', 'denied')),
  reason_code TEXT NOT NULL DEFAULT '' CHECK(length(reason_code) <= 64),
  retry_after_ms INTEGER NOT NULL DEFAULT 0 CHECK(retry_after_ms >= 0),
  execution_id TEXT REFERENCES automation_executions(id),
  retention_expires_at INTEGER NOT NULL CHECK(retention_expires_at > attempted_at),
  CHECK((outcome = 'reserved' AND reason_code = '' AND execution_id IS NULL)
    OR (outcome = 'accepted' AND reason_code = '' AND execution_id IS NOT NULL)
    OR (outcome = 'denied' AND reason_code <> '' AND execution_id IS NULL)),
  CHECK((trigger_kind = 'scoped_api' AND principal_id <> '' AND schedule_id = ''
      AND schedule_revision = 0 AND idempotency_digest <> ''
      AND request_digest <> '' AND occurrence_key = '')
    OR (trigger_kind = 'schedule' AND principal_id = '' AND schedule_id <> ''
      AND schedule_revision > 0 AND idempotency_digest = ''
      AND request_digest = '' AND occurrence_key <> ''))
);
CREATE UNIQUE INDEX IF NOT EXISTS automation_runtime_attempt_api_once
  ON automation_runtime_attempts(principal_id, idempotency_digest)
  WHERE trigger_kind = 'scoped_api';
CREATE UNIQUE INDEX IF NOT EXISTS automation_runtime_attempt_schedule_once
  ON automation_runtime_attempts(schedule_id, schedule_revision, occurrence_key)
  WHERE trigger_kind = 'schedule';
CREATE INDEX IF NOT EXISTS automation_runtime_attempt_principal_window
  ON automation_runtime_attempts(principal_id, attempted_at, id)
  WHERE principal_id <> '';
CREATE INDEX IF NOT EXISTS automation_runtime_attempt_orbit_window
  ON automation_runtime_attempts(owner_orbit_id, attempted_at, id);
CREATE INDEX IF NOT EXISTS automation_runtime_attempt_retention
  ON automation_runtime_attempts(retention_expires_at, id);

CREATE TABLE IF NOT EXISTS automation_builtin_media (
  owner_orbit_id INTEGER PRIMARY KEY CHECK(owner_orbit_id > 0),
  media_id TEXT NOT NULL UNIQUE REFERENCES media_items(id),
  storage_key TEXT NOT NULL UNIQUE CHECK(
    length(storage_key) = 73 AND substr(storage_key, 1, 9) = 'media/v1/'
      AND substr(storage_key, 10) NOT GLOB '*[^0-9a-f]*'
  ),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at)
);
`

func (s *Store) initAutomationRuntimeSchema() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(automationRuntimeSchema); err != nil {
		return err
	}
	if err := s.checkpoint("automation_runtime_schema_before_commit"); err != nil {
		return err
	}
	if err := foreignKeyCheck(tx); err != nil {
		return err
	}
	return tx.Commit()
}
