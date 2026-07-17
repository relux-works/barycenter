package store

// automationHistorySchema is an additive, privacy-bounded ledger. Trigger
// attempts are copied only after they reach a terminal admission decision;
// control mutations are appended by finishAutomationControlMutation in the
// same writer transaction as the state change.
const automationHistorySchema = `
CREATE TABLE IF NOT EXISTS automation_audit_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  event_kind TEXT NOT NULL CHECK(event_kind IN ('trigger', 'control')),
  operation TEXT NOT NULL CHECK(length(operation) BETWEEN 1 AND 128),
  owner_orbit_id INTEGER NOT NULL CHECK(owner_orbit_id > 0),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  principal_id TEXT NOT NULL DEFAULT '' CHECK(length(principal_id) <= 128),
  principal_label TEXT NOT NULL DEFAULT '' CHECK(length(principal_label) <= 128),
  schedule_id TEXT NOT NULL DEFAULT '' CHECK(length(schedule_id) <= 128),
  schedule_label TEXT NOT NULL DEFAULT '' CHECK(length(schedule_label) <= 128),
  execution_id TEXT NOT NULL DEFAULT '' CHECK(length(execution_id) <= 128),
  transmission_id TEXT NOT NULL DEFAULT '' CHECK(length(transmission_id) <= 128),
  cue_id TEXT NOT NULL DEFAULT '' CHECK(length(cue_id) <= 128),
  cue_label TEXT NOT NULL DEFAULT '' CHECK(length(cue_label) <= 128),
  cue_revision INTEGER NOT NULL DEFAULT 0 CHECK(cue_revision >= 0),
  schedule_revision INTEGER NOT NULL DEFAULT 0 CHECK(schedule_revision >= 0),
  trigger_kind TEXT NOT NULL DEFAULT '' CHECK(length(trigger_kind) <= 64),
  audience_kind TEXT NOT NULL DEFAULT '' CHECK(length(audience_kind) <= 64),
  resolved_target_count INTEGER NOT NULL DEFAULT 0 CHECK(resolved_target_count BETWEEN 0 AND 64),
  outcome TEXT NOT NULL CHECK(length(outcome) BETWEEN 1 AND 64),
  reason_code TEXT NOT NULL DEFAULT '' CHECK(length(reason_code) <= 64),
  retry_after_ms INTEGER NOT NULL DEFAULT 0 CHECK(retry_after_ms >= 0),
  scheduled_at INTEGER NOT NULL DEFAULT 0 CHECK(scheduled_at >= 0),
  accepted_at INTEGER NOT NULL DEFAULT 0 CHECK(accepted_at >= 0),
  terminal_at INTEGER NOT NULL CHECK(terminal_at > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0)
);
CREATE INDEX IF NOT EXISTS automation_audit_orbit_history
  ON automation_audit_events(owner_orbit_id, created_at DESC, id DESC);
CREATE INDEX IF NOT EXISTS automation_audit_transmission
  ON automation_audit_events(transmission_id) WHERE transmission_id <> '';

-- Manual soundboard delivery intentionally uses the ordinary transmission
-- delivery matrix. Keeping its lineage beside (rather than inside) the
-- overlay-only automation execution table preserves the frozen scheduler
-- constraints while still giving canonical history one attribution source.
CREATE TABLE IF NOT EXISTS manual_soundboard_executions (
  id TEXT PRIMARY KEY CHECK(length(id) = 29 AND substr(id, 1, 3) = 'mx_'),
  transmission_id TEXT NOT NULL UNIQUE REFERENCES transmissions(id),
  owner_orbit_id INTEGER NOT NULL CHECK(owner_orbit_id > 0),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  cue_id TEXT NOT NULL REFERENCES saved_cues(id),
  cue_revision INTEGER NOT NULL CHECK(cue_revision > 0),
  cue_source_generation INTEGER NOT NULL CHECK(cue_source_generation > 0),
  feature_revision INTEGER NOT NULL CHECK(feature_revision > 0),
  audience_kind TEXT NOT NULL CHECK(audience_kind IN (
    'this_pulsar', 'own_barycenter', 'current_air', 'explicit')),
  delivery TEXT NOT NULL CHECK(delivery IN ('overlay', 'interrupt', 'after_current')),
  resolved_target_count INTEGER NOT NULL CHECK(resolved_target_count BETWEEN 1 AND 64),
  created_at INTEGER NOT NULL CHECK(created_at > 0)
);
CREATE INDEX IF NOT EXISTS manual_soundboard_history
  ON manual_soundboard_executions(owner_orbit_id, created_at DESC, transmission_id);

CREATE TRIGGER IF NOT EXISTS automation_audit_events_no_update
BEFORE UPDATE ON automation_audit_events
BEGIN
  SELECT RAISE(ABORT, 'automation audit events are immutable');
END;
CREATE TRIGGER IF NOT EXISTS automation_audit_events_no_delete
BEFORE DELETE ON automation_audit_events
BEGIN
  SELECT RAISE(ABORT, 'automation audit events are immutable');
END;

CREATE TRIGGER IF NOT EXISTS automation_runtime_attempt_terminal_audit
AFTER UPDATE OF outcome ON automation_runtime_attempts
WHEN OLD.outcome = 'reserved' AND NEW.outcome IN ('accepted', 'denied')
BEGIN
  INSERT INTO automation_audit_events(
    event_kind, operation, owner_orbit_id, actor_id,
    principal_id, principal_label, schedule_id, schedule_label,
    execution_id, transmission_id, cue_id, cue_label, cue_revision,
    schedule_revision, trigger_kind, audience_kind, resolved_target_count,
    outcome, reason_code, retry_after_ms, scheduled_at, accepted_at,
    terminal_at, created_at
  ) VALUES(
    'trigger', 'automation.trigger.' || NEW.trigger_kind || '.v1', NEW.owner_orbit_id,
    COALESCE((SELECT issued_by_actor_id FROM automation_principals WHERE id = NEW.principal_id),
      (SELECT created_by_actor_id FROM automation_schedules WHERE id = NEW.schedule_id), 0),
    NEW.principal_id,
    COALESCE((SELECT display_name FROM automation_principals WHERE id = NEW.principal_id), ''),
    NEW.schedule_id,
    COALESCE((SELECT display_name FROM automation_schedules WHERE id = NEW.schedule_id), ''),
    COALESCE(NEW.execution_id,
      (SELECT id FROM automation_executions WHERE schedule_id = NEW.schedule_id
        AND schedule_revision = NEW.schedule_revision AND occurrence_key = NEW.occurrence_key), ''),
    COALESCE((SELECT transmission_id FROM automation_executions WHERE id = NEW.execution_id), ''),
    NEW.cue_id,
    COALESCE((SELECT title FROM saved_cues WHERE id = NEW.cue_id), ''),
    COALESCE((SELECT revision FROM saved_cues WHERE id = NEW.cue_id), 0),
    NEW.schedule_revision, NEW.trigger_kind,
    COALESCE((SELECT audience_kind FROM automation_executions WHERE id = NEW.execution_id),
      (SELECT audience_kind FROM automation_schedules WHERE id = NEW.schedule_id), ''),
    COALESCE((SELECT resolved_target_count FROM automation_executions WHERE id = NEW.execution_id), 0),
    NEW.outcome, NEW.reason_code, NEW.retry_after_ms,
    COALESCE((SELECT scheduled_utc FROM automation_executions WHERE id = NEW.execution_id), 0),
    CASE WHEN NEW.outcome = 'accepted' THEN NEW.attempted_at ELSE 0 END,
    NEW.attempted_at, NEW.attempted_at
  );
END;
`

func (s *Store) initAutomationHistorySchema() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(automationHistorySchema); err != nil {
		return err
	}
	if err := s.checkpoint("automation_history_schema_before_commit"); err != nil {
		return err
	}
	if err := foreignKeyCheck(tx); err != nil {
		return err
	}
	return tx.Commit()
}
