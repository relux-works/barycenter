package store

// automationControlSchema is an additive control-plane companion to the
// production-dark automation lineage schema. Older coordinators ignore these
// tables; no existing media, cue, transmission or identity row is rewritten.
const automationControlSchema = `
CREATE TABLE IF NOT EXISTS automation_control_mutation_results (
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  idempotency_key_hash TEXT NOT NULL CHECK(
    length(idempotency_key_hash) = 64
      AND idempotency_key_hash NOT GLOB '*[^0-9a-f]*'
  ),
  operation TEXT NOT NULL CHECK(length(operation) BETWEEN 1 AND 128),
  request_hash TEXT NOT NULL CHECK(
    length(request_hash) = 64 AND request_hash NOT GLOB '*[^0-9a-f]*'
  ),
  response_json TEXT NOT NULL CHECK(length(response_json) <= 65536),
  resource_id TEXT NOT NULL DEFAULT '' CHECK(length(resource_id) <= 128),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  PRIMARY KEY(actor_id, idempotency_key_hash)
);

CREATE TABLE IF NOT EXISTS saved_cue_order_state (
  owner_orbit_id INTEGER PRIMARY KEY CHECK(owner_orbit_id > 0),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  updated_by_actor_id INTEGER NOT NULL CHECK(updated_by_actor_id > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at > 0)
);
CREATE TABLE IF NOT EXISTS saved_cue_order_items (
  owner_orbit_id INTEGER NOT NULL CHECK(owner_orbit_id > 0),
  cue_id TEXT NOT NULL REFERENCES saved_cues(id),
  position INTEGER NOT NULL CHECK(position >= 0),
  PRIMARY KEY(owner_orbit_id, cue_id),
  UNIQUE(owner_orbit_id, position)
);

CREATE TABLE IF NOT EXISTS automation_schedule_controls (
  schedule_id TEXT PRIMARY KEY REFERENCES automation_schedules(id),
  additional_quiet_hours_json TEXT NOT NULL CHECK(
    length(additional_quiet_hours_json) BETWEEN 2 AND 16384
  ),
  additional_quiet_hours_hash TEXT NOT NULL CHECK(
    length(additional_quiet_hours_hash) = 64
      AND additional_quiet_hours_hash NOT GLOB '*[^0-9a-f]*'
  ),
  deleted_at INTEGER NOT NULL DEFAULT 0 CHECK(deleted_at >= 0),
  deleted_by_actor_id INTEGER NOT NULL DEFAULT 0 CHECK(deleted_by_actor_id >= 0),
  CHECK((deleted_at = 0 AND deleted_by_actor_id = 0)
    OR (deleted_at > 0 AND deleted_by_actor_id > 0))
);

CREATE TABLE IF NOT EXISTS automation_schedule_targets (
  schedule_id TEXT NOT NULL REFERENCES automation_schedules(id),
  target_digest TEXT NOT NULL CHECK(
    length(target_digest) = 64 AND target_digest NOT GLOB '*[^0-9a-f]*'
  ),
  target_kind TEXT NOT NULL CHECK(target_kind IN ('barycenter', 'pulsar')),
  target_orbit_id INTEGER NOT NULL CHECK(target_orbit_id > 0),
  target_actor_id INTEGER NOT NULL DEFAULT 0 CHECK(target_actor_id >= 0),
  target_slot TEXT NOT NULL DEFAULT '' CHECK(length(target_slot) <= 128),
  target_binding_paired_at INTEGER NOT NULL DEFAULT 0 CHECK(target_binding_paired_at >= 0),
  PRIMARY KEY(schedule_id, target_digest),
  CHECK((target_kind = 'barycenter' AND target_actor_id = 0
      AND target_slot = '' AND target_binding_paired_at = 0)
    OR (target_kind = 'pulsar' AND target_actor_id > 0
      AND target_slot <> '' AND target_binding_paired_at > 0))
);
CREATE INDEX IF NOT EXISTS automation_schedule_targets_subject
  ON automation_schedule_targets(target_kind, target_orbit_id, target_slot, schedule_id);
`

func (s *Store) initAutomationControlSchema() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(automationControlSchema); err != nil {
		return err
	}
	if err := s.checkpoint("automation_control_schema_before_commit"); err != nil {
		return err
	}
	if err := foreignKeyCheck(tx); err != nil {
		return err
	}
	return tx.Commit()
}
