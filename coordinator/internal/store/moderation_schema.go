package store

// moderationSchema is an additive control-plane companion. It intentionally
// does not reference legacy orbit/identity tables: a rollback coordinator may
// still dissolve legacy state while these historical reports remain readable
// after rolling forward. References into additive media/transmission state are
// protected because predecessor binaries do not delete those rows.
const moderationSchema = `
CREATE TABLE IF NOT EXISTS moderation_operators (
  id TEXT PRIMARY KEY
    CHECK(length(id) = 29 AND substr(id, 1, 3) = 'op_'),
  display_name TEXT NOT NULL CHECK(length(display_name) BETWEEN 1 AND 120),
  token_hash TEXT NOT NULL UNIQUE
    CHECK(length(token_hash) = 64 AND token_hash NOT GLOB '*[^0-9a-f]*'),
  can_list INTEGER NOT NULL CHECK(can_list IN (0, 1)),
  can_evidence INTEGER NOT NULL CHECK(can_evidence IN (0, 1)),
  can_decide INTEGER NOT NULL CHECK(can_decide IN (0, 1)),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  revoked_at INTEGER NOT NULL DEFAULT 0 CHECK(revoked_at >= 0),
  CHECK(can_list + can_evidence + can_decide > 0),
  CHECK(revoked_at = 0 OR revoked_at >= created_at)
);

CREATE TABLE IF NOT EXISTS moderation_reports (
  id TEXT PRIMARY KEY
    CHECK(length(id) = 29 AND substr(id, 1, 3) = 'rp_'),
  reporter_orbit_id INTEGER NOT NULL CHECK(reporter_orbit_id > 0),
  reporter_actor_id INTEGER NOT NULL CHECK(reporter_actor_id > 0),
  media_id TEXT NOT NULL REFERENCES media_items(id),
  reported_orbit_id INTEGER NOT NULL CHECK(reported_orbit_id > 0),
  reported_actor_id INTEGER NOT NULL CHECK(reported_actor_id > 0),
  media_kind TEXT NOT NULL
    CHECK(media_kind IN ('voice_clip', 'audio_clip', 'audio_track', 'builtin_cue')),
  media_source TEXT NOT NULL CHECK(media_source IN ('app', 'telegram', 'system')),
  media_title TEXT NOT NULL DEFAULT '' CHECK(length(media_title) <= 512),
  media_duration_ms INTEGER NOT NULL CHECK(media_duration_ms >= 0),
  transmission_id TEXT NOT NULL REFERENCES transmissions(id),
  target_orbit_id INTEGER NOT NULL CHECK(target_orbit_id > 0),
  target_actor_id INTEGER NOT NULL CHECK(target_actor_id > 0),
  target_slot TEXT NOT NULL CHECK(length(target_slot) = 1 AND target_slot GLOB '[a-z]'),
  audience_kind TEXT NOT NULL
    CHECK(audience_kind IN ('this_pulsar', 'own_barycenter', 'current_air', 'explicit')),
  playback_domain_kind TEXT NOT NULL CHECK(playback_domain_kind IN ('orbit', 'approach')),
  playback_domain_id INTEGER NOT NULL CHECK(playback_domain_id > 0),
  accepted_at INTEGER NOT NULL CHECK(accepted_at > 0),
  reason_code TEXT NOT NULL
    CHECK(reason_code IN ('spam', 'harassment', 'illegal', 'sexual_content', 'violence', 'other')),
  details TEXT NOT NULL DEFAULT '' CHECK(length(details) <= 2000),
  status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open', 'resolved')),
  evidence_storage_key TEXT NOT NULL
    CHECK(length(evidence_storage_key) = 73
      AND substr(evidence_storage_key, 1, 9) = 'media/v1/'
      AND substr(evidence_storage_key, 10) NOT GLOB '*[^0-9a-f]*'),
  evidence_sha256 TEXT NOT NULL
    CHECK(length(evidence_sha256) = 64 AND evidence_sha256 NOT GLOB '*[^0-9a-f]*'),
  evidence_size_bytes INTEGER NOT NULL CHECK(evidence_size_bytes > 0),
  evidence_mime TEXT NOT NULL CHECK(length(evidence_mime) BETWEEN 1 AND 128),
  evidence_expires_at INTEGER NOT NULL CHECK(evidence_expires_at > accepted_at),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at),
  resolved_at INTEGER NOT NULL DEFAULT 0 CHECK(resolved_at >= 0),
  UNIQUE(reporter_actor_id, media_id),
  CHECK(reporter_actor_id <> reported_actor_id),
  CHECK(target_actor_id = reporter_actor_id),
  CHECK((status = 'open' AND resolved_at = 0)
    OR (status = 'resolved' AND resolved_at >= created_at))
);
CREATE INDEX IF NOT EXISTS moderation_reports_queue
  ON moderation_reports(status, created_at, id);
CREATE INDEX IF NOT EXISTS moderation_reports_evidence_hold
  ON moderation_reports(media_id, evidence_storage_key, evidence_expires_at);

CREATE TABLE IF NOT EXISTS moderation_report_attempts (
  id INTEGER PRIMARY KEY,
  reporter_orbit_id INTEGER NOT NULL CHECK(reporter_orbit_id > 0),
  reporter_actor_id INTEGER NOT NULL CHECK(reporter_actor_id > 0),
  allowed INTEGER NOT NULL CHECK(allowed IN (0, 1)),
  created_at INTEGER NOT NULL CHECK(created_at > 0)
);
CREATE INDEX IF NOT EXISTS moderation_report_attempts_actor_time
  ON moderation_report_attempts(reporter_actor_id, created_at);

CREATE TABLE IF NOT EXISTS moderation_decisions (
  id TEXT PRIMARY KEY
    CHECK(length(id) = 29 AND substr(id, 1, 3) = 'md_'),
  report_id TEXT NOT NULL UNIQUE REFERENCES moderation_reports(id),
  action TEXT NOT NULL
    CHECK(action IN ('no_action', 'delete_media', 'disable_actor', 'disable_orbit')),
  state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending', 'applied')),
  requested_by_operator_id TEXT NOT NULL REFERENCES moderation_operators(id),
  requested_at INTEGER NOT NULL CHECK(requested_at > 0),
  applied_at INTEGER NOT NULL DEFAULT 0 CHECK(applied_at >= 0),
  CHECK((state = 'pending' AND applied_at = 0)
    OR (state = 'applied' AND applied_at >= requested_at))
);

CREATE TABLE IF NOT EXISTS moderation_audit_events (
  id INTEGER PRIMARY KEY,
  report_id TEXT REFERENCES moderation_reports(id),
  operator_id TEXT REFERENCES moderation_operators(id),
  actor_id INTEGER CHECK(actor_id IS NULL OR actor_id > 0),
  event_type TEXT NOT NULL CHECK(event_type IN (
    'operator.provisioned', 'operator.revoked',
    'report.created', 'report.rate_limited', 'report.blocked_sender',
    'evidence.read', 'decision.requested', 'decision.applied'
  )),
  action TEXT NOT NULL DEFAULT ''
    CHECK(action = '' OR action IN ('no_action', 'delete_media', 'disable_actor', 'disable_orbit')),
  created_at INTEGER NOT NULL CHECK(created_at > 0)
);
CREATE INDEX IF NOT EXISTS moderation_audit_created
  ON moderation_audit_events(created_at, id);

CREATE TRIGGER IF NOT EXISTS moderation_audit_no_update
BEFORE UPDATE ON moderation_audit_events
BEGIN
  SELECT RAISE(ABORT, 'moderation audit is append-only');
END;
CREATE TRIGGER IF NOT EXISTS moderation_audit_no_delete
BEFORE DELETE ON moderation_audit_events
BEGIN
  SELECT RAISE(ABORT, 'moderation audit is append-only');
END;

CREATE TRIGGER IF NOT EXISTS moderation_report_snapshot_immutable
BEFORE UPDATE ON moderation_reports
WHEN NEW.id <> OLD.id
  OR NEW.reporter_orbit_id <> OLD.reporter_orbit_id
  OR NEW.reporter_actor_id <> OLD.reporter_actor_id
  OR NEW.media_id <> OLD.media_id
  OR NEW.reported_orbit_id <> OLD.reported_orbit_id
  OR NEW.reported_actor_id <> OLD.reported_actor_id
  OR NEW.media_kind <> OLD.media_kind
  OR NEW.media_source <> OLD.media_source
  OR NEW.media_title <> OLD.media_title
  OR NEW.media_duration_ms <> OLD.media_duration_ms
  OR NEW.transmission_id <> OLD.transmission_id
  OR NEW.target_orbit_id <> OLD.target_orbit_id
  OR NEW.target_actor_id <> OLD.target_actor_id
  OR NEW.target_slot <> OLD.target_slot
  OR NEW.audience_kind <> OLD.audience_kind
  OR NEW.playback_domain_kind <> OLD.playback_domain_kind
  OR NEW.playback_domain_id <> OLD.playback_domain_id
  OR NEW.accepted_at <> OLD.accepted_at
  OR NEW.reason_code <> OLD.reason_code
  OR NEW.evidence_storage_key <> OLD.evidence_storage_key
  OR NEW.evidence_sha256 <> OLD.evidence_sha256
  OR NEW.evidence_size_bytes <> OLD.evidence_size_bytes
  OR NEW.evidence_mime <> OLD.evidence_mime
  OR NEW.evidence_expires_at <> OLD.evidence_expires_at
  OR NEW.created_at <> OLD.created_at
BEGIN
  SELECT RAISE(ABORT, 'moderation report evidence snapshot is immutable');
END;

CREATE TRIGGER IF NOT EXISTS moderation_decision_identity_immutable
BEFORE UPDATE ON moderation_decisions
WHEN NEW.id <> OLD.id OR NEW.report_id <> OLD.report_id
  OR NEW.action <> OLD.action
  OR NEW.requested_by_operator_id <> OLD.requested_by_operator_id
  OR NEW.requested_at <> OLD.requested_at
BEGIN
  SELECT RAISE(ABORT, 'moderation decision identity is immutable');
END;
`

func (s *Store) initModerationSchema() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(moderationSchema); err != nil {
		return err
	}
	if err := foreignKeyCheck(tx); err != nil {
		return err
	}
	if err := s.checkpoint("moderation_ddl_before_commit"); err != nil {
		return err
	}
	return tx.Commit()
}
