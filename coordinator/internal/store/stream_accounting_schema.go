package store

// streamAccountingSchema keeps Phase 2 economics additive and independent of
// the Phase 1 media authorities. Usage projections are read from authoritative
// upload, track, processing and egress rows; the audit tables retain only
// identifiers, dimensions and byte counts, never filenames or content.
const streamAccountingSchema = `
CREATE TABLE IF NOT EXISTS stream_accounting_policies (
  scope_kind TEXT NOT NULL CHECK(scope_kind IN ('actor', 'orbit')),
  scope_id INTEGER NOT NULL CHECK(scope_id >= 0),
  max_upload_starts_24h INTEGER NOT NULL CHECK(max_upload_starts_24h > 0),
  max_input_bytes_24h INTEGER NOT NULL CHECK(max_input_bytes_24h > 0),
  max_canonical_bytes INTEGER NOT NULL CHECK(max_canonical_bytes > 0),
  max_temp_processing_bytes INTEGER NOT NULL CHECK(max_temp_processing_bytes > 0),
  max_concurrent_jobs INTEGER NOT NULL CHECK(max_concurrent_jobs > 0),
  max_retained_bytes INTEGER NOT NULL CHECK(max_retained_bytes > 0),
  max_egress_bytes_24h INTEGER NOT NULL CHECK(max_egress_bytes_24h > 0),
  egress_amplification_milli INTEGER NOT NULL
    CHECK(egress_amplification_milli BETWEEN 1000 AND 4000),
  revision INTEGER NOT NULL CHECK(revision > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at > 0),
  PRIMARY KEY(scope_kind, scope_id)
);
INSERT OR IGNORE INTO stream_accounting_policies VALUES(
  'actor', 0, 100, 5368709120, 10737418240, 2147483648, 2,
  21474836480, 107374182400, 2000, 1, 1
);
INSERT OR IGNORE INTO stream_accounting_policies VALUES(
  'orbit', 0, 500, 26843545600, 53687091200, 8589934592, 8,
  107374182400, 536870912000, 2000, 1, 1
);

CREATE TABLE IF NOT EXISTS stream_accounting_policy_audit (
  id INTEGER PRIMARY KEY,
  operator_id TEXT NOT NULL CHECK(length(operator_id) BETWEEN 1 AND 128),
  scope_kind TEXT NOT NULL CHECK(scope_kind IN ('actor', 'orbit')),
  scope_id INTEGER NOT NULL CHECK(scope_id >= 0),
  previous_policy_json TEXT NOT NULL CHECK(length(previous_policy_json) BETWEEN 2 AND 4096),
  new_policy_json TEXT NOT NULL CHECK(length(new_policy_json) BETWEEN 2 AND 4096),
  reason TEXT NOT NULL CHECK(length(reason) BETWEEN 1 AND 64),
  created_at INTEGER NOT NULL CHECK(created_at > 0)
);
CREATE INDEX IF NOT EXISTS stream_accounting_policy_audit_scope
  ON stream_accounting_policy_audit(scope_kind, scope_id, created_at DESC);

CREATE TABLE IF NOT EXISTS stream_processing_jobs (
  id TEXT PRIMARY KEY CHECK(length(id) = 30 AND substr(id, 1, 4) = 'spj_'),
  media_id TEXT NOT NULL REFERENCES stream_track_metadata(media_id),
  owner_orbit_id INTEGER NOT NULL CHECK(owner_orbit_id > 0),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  idempotency_key_hash TEXT NOT NULL
    CHECK(length(idempotency_key_hash) = 64 AND idempotency_key_hash NOT GLOB '*[^0-9a-f]*'),
  input_bytes INTEGER NOT NULL CHECK(input_bytes > 0),
  temp_reserved_bytes INTEGER NOT NULL CHECK(temp_reserved_bytes > 0),
  temp_current_bytes INTEGER NOT NULL DEFAULT 0
    CHECK(temp_current_bytes >= 0 AND temp_current_bytes <= temp_reserved_bytes),
  temp_high_water_bytes INTEGER NOT NULL DEFAULT 0
    CHECK(temp_high_water_bytes >= temp_current_bytes AND temp_high_water_bytes <= temp_reserved_bytes),
  state TEXT NOT NULL DEFAULT 'active'
    CHECK(state IN ('active', 'succeeded', 'failed', 'expired')),
  outcome TEXT NOT NULL DEFAULT ''
    CHECK(outcome IN ('', 'published', 'validation_failed', 'processor_failed', 'cancelled', 'crash_released')),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at),
  completed_at INTEGER NOT NULL DEFAULT 0 CHECK(completed_at >= 0),
  UNIQUE(media_id, idempotency_key_hash),
  CHECK((state = 'active' AND outcome = '' AND completed_at = 0)
     OR (state <> 'active' AND outcome <> '' AND completed_at > 0)),
  CHECK(state = 'active' OR temp_current_bytes = 0)
);
CREATE INDEX IF NOT EXISTS stream_processing_jobs_active_scope
  ON stream_processing_jobs(state, owner_orbit_id, actor_id, updated_at);

CREATE TABLE IF NOT EXISTS stream_egress_sessions (
  id TEXT PRIMARY KEY CHECK(length(id) = 30 AND substr(id, 1, 4) = 'seg_'),
  media_id TEXT NOT NULL REFERENCES stream_track_metadata(media_id),
  variant_id TEXT NOT NULL REFERENCES stream_variants(id),
  owner_orbit_id INTEGER NOT NULL CHECK(owner_orbit_id > 0),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  playback_generation INTEGER NOT NULL CHECK(playback_generation > 0),
  idempotency_key_hash TEXT NOT NULL
    CHECK(length(idempotency_key_hash) = 64 AND idempotency_key_hash NOT GLOB '*[^0-9a-f]*'),
  reserved_bytes INTEGER NOT NULL CHECK(reserved_bytes > 0),
  actual_bytes INTEGER NOT NULL DEFAULT 0
    CHECK(actual_bytes >= 0 AND actual_bytes <= reserved_bytes),
  range_requests INTEGER NOT NULL DEFAULT 0 CHECK(range_requests >= 0),
  state TEXT NOT NULL DEFAULT 'active'
    CHECK(state IN ('active', 'completed', 'cancelled', 'revoked', 'expired')),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at),
  completed_at INTEGER NOT NULL DEFAULT 0 CHECK(completed_at >= 0),
  UNIQUE(variant_id, idempotency_key_hash),
  CHECK((state = 'active' AND completed_at = 0)
     OR (state <> 'active' AND completed_at > 0))
);
CREATE INDEX IF NOT EXISTS stream_egress_sessions_active_scope
  ON stream_egress_sessions(state, owner_orbit_id, actor_id, updated_at);

CREATE TABLE IF NOT EXISTS stream_egress_events (
  id INTEGER PRIMARY KEY,
  session_id TEXT NOT NULL REFERENCES stream_egress_sessions(id),
  request_key_hash TEXT NOT NULL
    CHECK(length(request_key_hash) = 64 AND request_key_hash NOT GLOB '*[^0-9a-f]*'),
  media_id TEXT NOT NULL,
  variant_id TEXT NOT NULL,
  owner_orbit_id INTEGER NOT NULL CHECK(owner_orbit_id > 0),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  range_start INTEGER NOT NULL CHECK(range_start >= 0),
  range_end INTEGER NOT NULL CHECK(range_end >= range_start),
  requested_bytes INTEGER NOT NULL CHECK(requested_bytes > 0),
  actual_bytes INTEGER NOT NULL CHECK(actual_bytes >= 0 AND actual_bytes <= requested_bytes),
  outcome TEXT NOT NULL CHECK(outcome IN ('served', 'cache_refill', 'failed', 'revoked', 'client_cancelled')),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  UNIQUE(session_id, request_key_hash)
);
CREATE INDEX IF NOT EXISTS stream_egress_events_scope_created
  ON stream_egress_events(owner_orbit_id, actor_id, created_at);

CREATE TABLE IF NOT EXISTS stream_quota_rejections (
  id INTEGER PRIMARY KEY,
  scope_kind TEXT NOT NULL CHECK(scope_kind IN ('actor', 'orbit')),
  scope_id INTEGER NOT NULL CHECK(scope_id > 0),
  dimension TEXT NOT NULL CHECK(dimension IN (
    'upload_starts', 'input_bytes', 'canonical_bytes', 'temp_processing_bytes',
    'concurrent_jobs', 'retained_bytes', 'egress_bytes', 'range_amplification'
  )),
  created_at INTEGER NOT NULL CHECK(created_at > 0)
);
CREATE INDEX IF NOT EXISTS stream_quota_rejections_created
  ON stream_quota_rejections(created_at, scope_kind, scope_id);

CREATE TABLE IF NOT EXISTS stream_accounting_state (
  singleton INTEGER PRIMARY KEY CHECK(singleton = 1),
  last_reconciled_at INTEGER NOT NULL CHECK(last_reconciled_at >= 0),
  processing_crash_releases INTEGER NOT NULL DEFAULT 0 CHECK(processing_crash_releases >= 0),
  egress_crash_releases INTEGER NOT NULL DEFAULT 0 CHECK(egress_crash_releases >= 0),
  revision INTEGER NOT NULL CHECK(revision > 0)
);
INSERT OR IGNORE INTO stream_accounting_state(
  singleton, last_reconciled_at, processing_crash_releases,
  egress_crash_releases, revision
) VALUES(1, 0, 0, 0, 1);
`

func (s *Store) initStreamAccountingSchema() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(streamAccountingSchema); err != nil {
		return err
	}
	if err := foreignKeyCheck(tx); err != nil {
		return err
	}
	if err := s.checkpoint("stream_accounting_ddl_before_commit"); err != nil {
		return err
	}
	return tx.Commit()
}
