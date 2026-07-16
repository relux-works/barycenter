package store

// automationSchema is additive and production-dark. It records immutable
// scope and execution lineage but registers no route and starts no worker.
// Legacy coordinators ignore these tables and retain authority over their
// existing media, identity and transmission rows.
const automationSchema = `
CREATE TABLE IF NOT EXISTS automation_feature_state (
  owner_orbit_id INTEGER PRIMARY KEY CHECK(owner_orbit_id > 0),
  soundboard_enabled INTEGER NOT NULL DEFAULT 0 CHECK(soundboard_enabled IN (0, 1)),
  automation_enabled INTEGER NOT NULL DEFAULT 0 CHECK(automation_enabled IN (0, 1)),
  emergency_disabled INTEGER NOT NULL DEFAULT 0 CHECK(emergency_disabled IN (0, 1)),
  timezone TEXT NOT NULL DEFAULT '' CHECK(length(timezone) <= 128),
  quiet_hours_json TEXT NOT NULL DEFAULT '' CHECK(length(quiet_hours_json) <= 16384),
  quiet_hours_hash TEXT NOT NULL DEFAULT '' CHECK(
    quiet_hours_hash = '' OR (length(quiet_hours_hash) = 64
      AND quiet_hours_hash NOT GLOB '*[^0-9a-f]*')
  ),
  policy_version TEXT NOT NULL DEFAULT 'automation-safety-v1'
    CHECK(policy_version = 'automation-safety-v1'),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  updated_by_actor_id INTEGER NOT NULL CHECK(updated_by_actor_id > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at > 0),
  emergency_disabled_at INTEGER NOT NULL DEFAULT 0 CHECK(emergency_disabled_at >= 0),
  CHECK((emergency_disabled = 0 AND emergency_disabled_at = 0)
    OR (emergency_disabled = 1 AND emergency_disabled_at > 0)),
  CHECK(automation_enabled = 0 OR (
    timezone <> '' AND quiet_hours_json <> '' AND quiet_hours_hash <> ''
  ))
);

CREATE TABLE IF NOT EXISTS automation_schedules (
  id TEXT PRIMARY KEY CHECK(length(id) = 30 AND substr(id, 1, 4) = 'sch_'),
  owner_orbit_id INTEGER NOT NULL CHECK(owner_orbit_id > 0),
  created_by_actor_id INTEGER NOT NULL CHECK(created_by_actor_id > 0),
  cue_id TEXT NOT NULL REFERENCES saved_cues(id),
  display_name TEXT NOT NULL CHECK(length(display_name) BETWEEN 1 AND 128),
  timezone TEXT NOT NULL CHECK(length(timezone) BETWEEN 1 AND 128),
  weekdays_mask INTEGER NOT NULL CHECK(weekdays_mask BETWEEN 1 AND 127),
  local_minute INTEGER NOT NULL CHECK(local_minute BETWEEN 0 AND 1439),
  audience_kind TEXT NOT NULL
    CHECK(audience_kind IN ('own_barycenter', 'current_air', 'explicit')),
  selector_digest TEXT NOT NULL DEFAULT '' CHECK(
    selector_digest = '' OR (length(selector_digest) = 64
      AND selector_digest NOT GLOB '*[^0-9a-f]*')
  ),
  bound_air_id TEXT NOT NULL DEFAULT '' CHECK(length(bound_air_id) <= 128),
  delivery TEXT NOT NULL DEFAULT 'overlay' CHECK(delivery = 'overlay'),
  policy_version TEXT NOT NULL CHECK(policy_version = 'automation-safety-v1'),
  policy_revision INTEGER NOT NULL CHECK(policy_revision > 0),
  enabled INTEGER NOT NULL DEFAULT 1 CHECK(enabled IN (0, 1)),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at),
  disabled_at INTEGER NOT NULL DEFAULT 0 CHECK(disabled_at >= 0),
  CHECK((enabled = 1 AND disabled_at = 0) OR (enabled = 0 AND disabled_at > 0)),
  CHECK((audience_kind = 'own_barycenter' AND selector_digest = '' AND bound_air_id = '')
    OR (audience_kind = 'current_air' AND selector_digest = '' AND bound_air_id <> '')
    OR (audience_kind = 'explicit' AND selector_digest <> '' AND bound_air_id = ''))
);
CREATE INDEX IF NOT EXISTS automation_schedules_due
  ON automation_schedules(enabled, timezone, local_minute, id);
CREATE INDEX IF NOT EXISTS automation_schedules_owner
  ON automation_schedules(owner_orbit_id, updated_at DESC, id);

CREATE TABLE IF NOT EXISTS automation_principals (
  id TEXT PRIMARY KEY CHECK(length(id) = 29 AND substr(id, 1, 3) = 'ap_'),
  owner_orbit_id INTEGER NOT NULL CHECK(owner_orbit_id > 0),
  issued_by_actor_id INTEGER NOT NULL CHECK(issued_by_actor_id > 0),
  display_name TEXT NOT NULL CHECK(length(display_name) BETWEEN 1 AND 128),
  secret_hash TEXT NOT NULL UNIQUE CHECK(
    length(secret_hash) = 64 AND secret_hash NOT GLOB '*[^0-9a-f]*'
  ),
  secret_hash_version TEXT NOT NULL CHECK(secret_hash_version = 'sha256-domain-v1'),
  permission TEXT NOT NULL CHECK(permission = 'automation:trigger'),
  bound_air_id TEXT NOT NULL DEFAULT '' CHECK(length(bound_air_id) <= 128),
  max_target_count INTEGER NOT NULL CHECK(max_target_count BETWEEN 1 AND 64),
  issued_at INTEGER NOT NULL CHECK(issued_at > 0),
  expires_at INTEGER NOT NULL CHECK(expires_at > issued_at),
  disabled_at INTEGER NOT NULL DEFAULT 0 CHECK(disabled_at >= 0),
  disabled_by_actor_id INTEGER NOT NULL DEFAULT 0 CHECK(disabled_by_actor_id >= 0),
  revoked_at INTEGER NOT NULL DEFAULT 0 CHECK(revoked_at >= 0),
  revoked_by_actor_id INTEGER NOT NULL DEFAULT 0 CHECK(revoked_by_actor_id >= 0),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  CHECK(expires_at - issued_at <= 7776000000),
  CHECK((disabled_at = 0 AND disabled_by_actor_id = 0)
    OR (disabled_at >= issued_at AND disabled_by_actor_id > 0)),
  CHECK((revoked_at = 0 AND revoked_by_actor_id = 0)
    OR (revoked_at >= issued_at AND revoked_by_actor_id > 0))
);
CREATE INDEX IF NOT EXISTS automation_principals_owner_state
  ON automation_principals(owner_orbit_id, revoked_at, disabled_at, expires_at, id);

CREATE TABLE IF NOT EXISTS automation_principal_cues (
  principal_id TEXT NOT NULL REFERENCES automation_principals(id),
  cue_id TEXT NOT NULL REFERENCES saved_cues(id),
  PRIMARY KEY(principal_id, cue_id)
);
CREATE TABLE IF NOT EXISTS automation_principal_audiences (
  principal_id TEXT NOT NULL REFERENCES automation_principals(id),
  audience_kind TEXT NOT NULL
    CHECK(audience_kind IN ('own_barycenter', 'current_air', 'explicit')),
  PRIMARY KEY(principal_id, audience_kind)
);
CREATE TABLE IF NOT EXISTS automation_principal_target_refs (
  principal_id TEXT NOT NULL REFERENCES automation_principals(id),
  target_ref_digest TEXT NOT NULL CHECK(
    length(target_ref_digest) = 64 AND target_ref_digest NOT GLOB '*[^0-9a-f]*'
  ),
  PRIMARY KEY(principal_id, target_ref_digest)
);

CREATE TABLE IF NOT EXISTS automation_executions (
  id TEXT PRIMARY KEY CHECK(length(id) = 29 AND substr(id, 1, 3) = 'ax_'),
  trigger_kind TEXT NOT NULL
    CHECK(trigger_kind IN ('manual_soundboard', 'scoped_api', 'schedule')),
  owner_orbit_id INTEGER NOT NULL CHECK(owner_orbit_id > 0),
  principal_id TEXT REFERENCES automation_principals(id),
  schedule_id TEXT REFERENCES automation_schedules(id),
  schedule_revision INTEGER NOT NULL DEFAULT 0 CHECK(schedule_revision >= 0),
  issued_by_actor_id INTEGER NOT NULL CHECK(issued_by_actor_id > 0),
  cue_id TEXT NOT NULL REFERENCES saved_cues(id),
  cue_revision INTEGER NOT NULL CHECK(cue_revision > 0),
  cue_source_generation INTEGER NOT NULL CHECK(cue_source_generation > 0),
  media_identity TEXT NOT NULL CHECK(length(media_identity) BETWEEN 1 AND 128),
  audience_kind TEXT NOT NULL
    CHECK(audience_kind IN ('own_barycenter', 'current_air', 'explicit')),
  selector_digest TEXT NOT NULL DEFAULT '' CHECK(
    selector_digest = '' OR (length(selector_digest) = 64
      AND selector_digest NOT GLOB '*[^0-9a-f]*')
  ),
  bound_air_id TEXT NOT NULL DEFAULT '' CHECK(length(bound_air_id) <= 128),
  target_snapshot_digest TEXT NOT NULL DEFAULT '' CHECK(
    target_snapshot_digest = '' OR (length(target_snapshot_digest) = 64
      AND target_snapshot_digest NOT GLOB '*[^0-9a-f]*')
  ),
  resolved_target_count INTEGER NOT NULL DEFAULT 0
    CHECK(resolved_target_count BETWEEN 0 AND 64),
  delivery TEXT NOT NULL CHECK(delivery = 'overlay'),
  idempotency_digest TEXT NOT NULL DEFAULT '' CHECK(
    idempotency_digest = '' OR (length(idempotency_digest) = 64
      AND idempotency_digest NOT GLOB '*[^0-9a-f]*')
  ),
  request_digest TEXT NOT NULL DEFAULT '' CHECK(
    request_digest = '' OR (length(request_digest) = 64
      AND request_digest NOT GLOB '*[^0-9a-f]*')
  ),
  occurrence_key TEXT NOT NULL DEFAULT '' CHECK(length(occurrence_key) <= 256),
  scheduled_local_date TEXT NOT NULL DEFAULT '' CHECK(
    scheduled_local_date = '' OR scheduled_local_date GLOB '????-??-??'
  ),
  scheduled_local_minute INTEGER NOT NULL DEFAULT -1
    CHECK(scheduled_local_minute BETWEEN -1 AND 1439),
  scheduled_utc INTEGER NOT NULL DEFAULT 0 CHECK(scheduled_utc >= 0),
  feature_revision INTEGER NOT NULL CHECK(feature_revision > 0),
  policy_revision INTEGER NOT NULL CHECK(policy_revision > 0),
  claimed_at INTEGER NOT NULL CHECK(claimed_at > 0),
  transmission_id TEXT REFERENCES transmissions(id),
  status TEXT NOT NULL DEFAULT 'claimed'
    CHECK(status IN ('claimed', 'leased', 'accepted', 'denied', 'cancelling',
      'cancelled', 'completed', 'failed')),
  outcome TEXT NOT NULL DEFAULT '' CHECK(length(outcome) <= 64),
  reason_code TEXT NOT NULL DEFAULT '' CHECK(length(reason_code) <= 64),
  retry_generation INTEGER NOT NULL DEFAULT 0 CHECK(retry_generation >= 0),
  lease_owner_hash TEXT NOT NULL DEFAULT '' CHECK(
    lease_owner_hash = '' OR (length(lease_owner_hash) = 64
      AND lease_owner_hash NOT GLOB '*[^0-9a-f]*')
  ),
  lease_generation INTEGER NOT NULL DEFAULT 0 CHECK(lease_generation >= 0),
  lease_expires_at INTEGER NOT NULL DEFAULT 0 CHECK(lease_expires_at >= 0),
  completed_at INTEGER NOT NULL DEFAULT 0 CHECK(completed_at >= 0),
  retention_expires_at INTEGER NOT NULL CHECK(retention_expires_at > claimed_at),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  CHECK((status = 'leased' AND lease_owner_hash <> '' AND lease_expires_at > claimed_at)
    OR (status <> 'leased' AND lease_owner_hash = '' AND lease_expires_at = 0)),
  CHECK(status NOT IN ('cancelled', 'completed', 'failed') OR completed_at > 0),
  CHECK((audience_kind = 'own_barycenter' AND selector_digest = '' AND bound_air_id = '')
    OR (audience_kind = 'current_air' AND selector_digest = '' AND bound_air_id <> '')
    OR (audience_kind = 'explicit' AND selector_digest <> '' AND bound_air_id = '')),
  CHECK((trigger_kind = 'schedule' AND schedule_id IS NOT NULL
      AND principal_id IS NULL AND schedule_revision > 0
      AND occurrence_key <> '' AND scheduled_local_date <> ''
      AND scheduled_local_minute >= 0 AND scheduled_utc > 0
      AND idempotency_digest = '' AND request_digest = '')
    OR (trigger_kind = 'scoped_api' AND principal_id IS NOT NULL
      AND schedule_id IS NULL AND schedule_revision = 0
      AND occurrence_key = '' AND scheduled_local_date = ''
      AND scheduled_local_minute = -1 AND scheduled_utc = 0
      AND idempotency_digest <> '' AND request_digest <> '')
    OR (trigger_kind = 'manual_soundboard' AND principal_id IS NULL
      AND schedule_id IS NULL AND schedule_revision = 0
      AND occurrence_key = '' AND idempotency_digest = ''))
);
CREATE UNIQUE INDEX IF NOT EXISTS automation_execution_occurrence_once
  ON automation_executions(schedule_id, schedule_revision,
    scheduled_local_date, scheduled_local_minute)
  WHERE trigger_kind = 'schedule';
CREATE UNIQUE INDEX IF NOT EXISTS automation_execution_api_idempotency
  ON automation_executions(principal_id, idempotency_digest)
  WHERE trigger_kind = 'scoped_api';
CREATE INDEX IF NOT EXISTS automation_execution_claim_queue
  ON automation_executions(status, lease_expires_at, claimed_at, id);
CREATE INDEX IF NOT EXISTS automation_execution_principal_pending
  ON automation_executions(principal_id, status, claimed_at, id);
CREATE INDEX IF NOT EXISTS automation_execution_schedule_pending
  ON automation_executions(schedule_id, status, claimed_at, id);
CREATE INDEX IF NOT EXISTS automation_execution_orbit_pending
  ON automation_executions(owner_orbit_id, status, claimed_at, id);
CREATE INDEX IF NOT EXISTS automation_execution_retention
  ON automation_executions(retention_expires_at, status, id);
`

func (s *Store) initAutomationSchema() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(automationSchema); err != nil {
		return err
	}
	if err := s.checkpoint("automation_schema_before_commit"); err != nil {
		return err
	}
	if err := foreignKeyCheck(tx); err != nil {
		return err
	}
	return tx.Commit()
}
