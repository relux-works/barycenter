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

-- Telegram automation callbacks are opaque one-shot transport capabilities.
-- Domain identifiers and target references stay server-side, and current
-- actor authority plus domain revisions are rechecked when they are claimed.
CREATE TABLE IF NOT EXISTS telegram_automation_callbacks (
  token_hash TEXT PRIMARY KEY CHECK(
    length(token_hash) = 64 AND token_hash NOT GLOB '*[^0-9a-f]*'
  ),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  orbit_id INTEGER NOT NULL CHECK(orbit_id > 0),
  role TEXT NOT NULL CHECK(role IN ('primary', 'companion', 'satellite')),
  chat_id INTEGER NOT NULL CHECK(chat_id <> 0),
  message_id INTEGER NOT NULL CHECK(message_id > 0),
  action TEXT NOT NULL CHECK(action IN (
    'cue_select', 'trigger', 'schedule_enable', 'schedule_disable', 'emergency_disable'
  )),
  cue_id TEXT NOT NULL DEFAULT '' CHECK(length(cue_id) <= 64),
  cue_revision INTEGER NOT NULL DEFAULT 0 CHECK(cue_revision >= 0),
  cue_source_generation INTEGER NOT NULL DEFAULT 0 CHECK(cue_source_generation >= 0),
  audience_kind TEXT NOT NULL DEFAULT '' CHECK(audience_kind IN (
    '', 'this_pulsar', 'own_barycenter', 'current_air', 'explicit'
  )),
  target_reference TEXT NOT NULL DEFAULT '' CHECK(length(target_reference) <= 64),
  include_origin INTEGER NOT NULL DEFAULT 1 CHECK(include_origin IN (0, 1)),
  delivery TEXT NOT NULL DEFAULT '' CHECK(delivery IN (
    '', 'overlay', 'interrupt', 'after_current'
  )),
  schedule_id TEXT NOT NULL DEFAULT '' CHECK(length(schedule_id) <= 64),
  schedule_revision INTEGER NOT NULL DEFAULT 0 CHECK(schedule_revision >= 0),
  feature_revision INTEGER NOT NULL DEFAULT 0 CHECK(feature_revision >= 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  expires_at INTEGER NOT NULL CHECK(expires_at > created_at),
  consumed_at INTEGER NOT NULL DEFAULT 0 CHECK(consumed_at >= 0),
  outcome TEXT NOT NULL DEFAULT '' CHECK(outcome IN (
    '', 'applied', 'already_applied', 'requires_confirmation', 'too_late',
    'expired', 'forbidden', 'unsupported', 'failed'
  )),
  CHECK((action = 'cue_select' AND cue_id <> '' AND cue_revision > 0
      AND cue_source_generation > 0 AND audience_kind = '' AND delivery = ''
      AND schedule_id = '' AND schedule_revision = 0)
    OR (action = 'trigger' AND cue_id <> '' AND cue_revision > 0
      AND cue_source_generation > 0 AND audience_kind <> '' AND delivery <> ''
      AND schedule_id = '' AND schedule_revision = 0)
    OR (action IN ('schedule_enable', 'schedule_disable') AND schedule_id <> ''
      AND schedule_revision > 0 AND cue_id = '' AND audience_kind = ''
      AND delivery = '')
    OR (action = 'emergency_disable' AND feature_revision >= 0
      AND cue_id = '' AND schedule_id = '' AND audience_kind = ''
      AND delivery = ''))
);
CREATE INDEX IF NOT EXISTS telegram_automation_callbacks_expiry
  ON telegram_automation_callbacks(expires_at, consumed_at);

CREATE TABLE IF NOT EXISTS telegram_automation_callback_queries (
  query_hash TEXT PRIMARY KEY CHECK(
    length(query_hash) = 64 AND query_hash NOT GLOB '*[^0-9a-f]*'
  ),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  orbit_id INTEGER NOT NULL CHECK(orbit_id > 0),
  role TEXT NOT NULL CHECK(role IN ('primary', 'companion', 'satellite')),
  chat_id INTEGER NOT NULL CHECK(chat_id <> 0),
  message_id INTEGER NOT NULL CHECK(message_id > 0),
  token_hash TEXT NOT NULL REFERENCES telegram_automation_callbacks(token_hash),
  outcome TEXT NOT NULL CHECK(outcome IN (
    'applied', 'already_applied', 'requires_confirmation', 'too_late',
    'expired', 'forbidden', 'unsupported', 'failed'
  )),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  expires_at INTEGER NOT NULL CHECK(expires_at > created_at)
);
CREATE INDEX IF NOT EXISTS telegram_automation_callback_queries_expiry
  ON telegram_automation_callback_queries(expires_at);
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
