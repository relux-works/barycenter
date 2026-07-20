package store

// e2eeReportModerationSchema extends the production-dark E2EE foundation with
// an explicit voluntary-disclosure boundary. A metadata-only report creates no
// evidence row. The evidence tables can be populated only by the consented
// store transition in e2ee_report_moderation.go and contain an opaque
// moderation-at-rest reference, never client E2EE keys or coordinator plaintext.
const e2eeReportModerationSchema = `
CREATE TABLE IF NOT EXISTS e2ee_moderation_reports (
  id TEXT PRIMARY KEY CHECK(length(id) = 30 AND substr(id, 1, 4) = 'erp_'),
  protected_object_id TEXT NOT NULL REFERENCES e2ee_protected_objects(id),
  group_id TEXT NOT NULL REFERENCES e2ee_groups(id),
  reporter_actor_id INTEGER NOT NULL REFERENCES actors(id) CHECK(reporter_actor_id > 0),
  reporter_device_id TEXT NOT NULL REFERENCES e2ee_device_public_state(device_id)
    CHECK(length(reporter_device_id) BETWEEN 8 AND 128),
  reported_orbit_id INTEGER NOT NULL REFERENCES orbits(id) CHECK(reported_orbit_id > 0),
  reported_actor_id INTEGER NOT NULL REFERENCES actors(id) CHECK(reported_actor_id > 0),
  reported_device_id TEXT NOT NULL REFERENCES e2ee_device_public_state(device_id)
    CHECK(length(reported_device_id) BETWEEN 8 AND 128),
  object_kind TEXT NOT NULL CHECK(object_kind IN ('clip', 'track', 'saved_cue', 'live_ptt')),
  source_object_id TEXT NOT NULL CHECK(length(source_object_id) BETWEEN 8 AND 128),
  epoch INTEGER NOT NULL CHECK(epoch > 0),
  generation INTEGER NOT NULL CHECK(generation > 0),
  target_snapshot_digest TEXT NOT NULL CHECK(
    length(target_snapshot_digest) = 64
      AND target_snapshot_digest NOT GLOB '*[^0-9a-f]*'
  ),
  manifest_digest TEXT NOT NULL CHECK(
    length(manifest_digest) = 64 AND manifest_digest NOT GLOB '*[^0-9a-f]*'
  ),
  ciphertext_digest TEXT NOT NULL CHECK(
    length(ciphertext_digest) = 64 AND ciphertext_digest NOT GLOB '*[^0-9a-f]*'
  ),
  reason_code TEXT NOT NULL CHECK(reason_code IN (
    'spam', 'harassment', 'illegal', 'sexual_content', 'violence', 'other'
  )),
  statement TEXT NOT NULL DEFAULT '' CHECK(length(statement) <= 2000),
  statement_expires_at INTEGER NOT NULL CHECK(statement_expires_at > 0),
  status TEXT NOT NULL DEFAULT 'open' CHECK(status IN ('open', 'resolved')),
  evidence_state TEXT NOT NULL DEFAULT 'metadata_only'
    CHECK(evidence_state IN ('metadata_only', 'provided', 'deleted', 'expired')),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at),
  resolved_at INTEGER NOT NULL DEFAULT 0 CHECK(resolved_at >= 0),
  UNIQUE(reporter_actor_id, protected_object_id),
  CHECK(reporter_actor_id <> reported_actor_id),
  CHECK(statement_expires_at > created_at),
  CHECK((status = 'open' AND resolved_at = 0)
     OR (status = 'resolved' AND resolved_at >= created_at))
);
CREATE INDEX IF NOT EXISTS e2ee_moderation_reports_queue
  ON e2ee_moderation_reports(status, created_at, id);

CREATE TABLE IF NOT EXISTS e2ee_report_evidence_consents (
  id TEXT PRIMARY KEY CHECK(length(id) = 30 AND substr(id, 1, 4) = 'erc_'),
  report_id TEXT NOT NULL UNIQUE REFERENCES e2ee_moderation_reports(id),
  protected_object_id TEXT NOT NULL REFERENCES e2ee_protected_objects(id),
  reporter_actor_id INTEGER NOT NULL REFERENCES actors(id) CHECK(reporter_actor_id > 0),
  reporter_device_id TEXT NOT NULL REFERENCES e2ee_device_public_state(device_id)
    CHECK(length(reporter_device_id) BETWEEN 8 AND 128),
  consent_version TEXT NOT NULL CHECK(length(consent_version) BETWEEN 1 AND 128),
  consent_digest TEXT NOT NULL CHECK(
    length(consent_digest) = 64 AND consent_digest NOT GLOB '*[^0-9a-f]*'
  ),
  manifest_digest TEXT NOT NULL CHECK(
    length(manifest_digest) = 64 AND manifest_digest NOT GLOB '*[^0-9a-f]*'
  ),
  authenticated_evidence_digest TEXT NOT NULL CHECK(
    length(authenticated_evidence_digest) = 64
      AND authenticated_evidence_digest NOT GLOB '*[^0-9a-f]*'
  ),
  action TEXT NOT NULL CHECK(action = 'explicit_report_evidence_export'),
  confirmed_at INTEGER NOT NULL CHECK(confirmed_at > 0)
);

CREATE TABLE IF NOT EXISTS e2ee_report_evidence_state (
  evidence_id TEXT PRIMARY KEY REFERENCES e2ee_report_evidence_metadata(id),
  report_id TEXT NOT NULL UNIQUE REFERENCES e2ee_moderation_reports(id),
  consent_id TEXT NOT NULL UNIQUE REFERENCES e2ee_report_evidence_consents(id),
  at_rest_ciphertext_digest TEXT NOT NULL CHECK(
    length(at_rest_ciphertext_digest) = 64
      AND at_rest_ciphertext_digest NOT GLOB '*[^0-9a-f]*'
  ),
  evidence_size_bytes INTEGER NOT NULL CHECK(evidence_size_bytes BETWEEN 1 AND 67108864),
  evidence_mime TEXT NOT NULL CHECK(
    evidence_mime = 'application/vnd.barycenter.report-evidence+octet-stream'
  ),
  status TEXT NOT NULL CHECK(status IN ('active', 'deleted', 'expired')),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at),
  terminal_at INTEGER NOT NULL DEFAULT 0 CHECK(terminal_at >= 0),
  CHECK((status = 'active' AND terminal_at = 0)
     OR (status <> 'active' AND terminal_at >= created_at))
);
CREATE INDEX IF NOT EXISTS e2ee_report_evidence_expiry
  ON e2ee_report_evidence_state(status, updated_at, evidence_id);

CREATE TABLE IF NOT EXISTS e2ee_moderation_decisions (
  id TEXT PRIMARY KEY CHECK(length(id) = 30 AND substr(id, 1, 4) = 'erd_'),
  report_id TEXT NOT NULL UNIQUE REFERENCES e2ee_moderation_reports(id),
  action TEXT NOT NULL CHECK(action IN (
    'no_action', 'delete_media', 'disable_actor', 'disable_orbit'
  )),
  state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending', 'applied')),
  requested_by_operator_id TEXT NOT NULL REFERENCES moderation_operators(id),
  requested_at INTEGER NOT NULL CHECK(requested_at > 0),
  applied_at INTEGER NOT NULL DEFAULT 0 CHECK(applied_at >= 0),
  CHECK((state = 'pending' AND applied_at = 0)
     OR (state = 'applied' AND applied_at >= requested_at))
);

CREATE TABLE IF NOT EXISTS e2ee_report_audit_events (
  id INTEGER PRIMARY KEY,
  report_id TEXT NOT NULL REFERENCES e2ee_moderation_reports(id),
  evidence_id TEXT REFERENCES e2ee_report_evidence_metadata(id),
  operator_id TEXT REFERENCES moderation_operators(id),
  actor_id INTEGER CHECK(actor_id IS NULL OR actor_id > 0),
  device_id TEXT NOT NULL DEFAULT '' CHECK(length(device_id) <= 128),
  event_type TEXT NOT NULL CHECK(event_type IN (
    'report.created', 'evidence.consent_recorded', 'evidence.created',
    'evidence.read', 'evidence.deleted', 'evidence.expired',
    'decision.requested', 'decision.applied'
  )),
  action TEXT NOT NULL DEFAULT '' CHECK(action = '' OR action IN (
    'no_action', 'delete_media', 'disable_actor', 'disable_orbit'
  )),
  created_at INTEGER NOT NULL CHECK(created_at > 0)
);
CREATE INDEX IF NOT EXISTS e2ee_report_audit_created
  ON e2ee_report_audit_events(report_id, created_at, id);

CREATE TRIGGER IF NOT EXISTS e2ee_report_audit_no_update
BEFORE UPDATE ON e2ee_report_audit_events BEGIN
  SELECT RAISE(ABORT, 'E2EE report audit is append-only');
END;
CREATE TRIGGER IF NOT EXISTS e2ee_report_audit_no_delete
BEFORE DELETE ON e2ee_report_audit_events BEGIN
  SELECT RAISE(ABORT, 'E2EE report audit is append-only');
END;

CREATE TRIGGER IF NOT EXISTS e2ee_moderation_report_snapshot_immutable
BEFORE UPDATE ON e2ee_moderation_reports
WHEN NEW.id <> OLD.id
  OR NEW.protected_object_id <> OLD.protected_object_id
  OR NEW.group_id <> OLD.group_id
  OR NEW.reporter_actor_id <> OLD.reporter_actor_id
  OR NEW.reporter_device_id <> OLD.reporter_device_id
  OR NEW.reported_orbit_id <> OLD.reported_orbit_id
  OR NEW.reported_actor_id <> OLD.reported_actor_id
  OR NEW.reported_device_id <> OLD.reported_device_id
  OR NEW.object_kind <> OLD.object_kind
  OR NEW.source_object_id <> OLD.source_object_id
  OR NEW.epoch <> OLD.epoch
  OR NEW.generation <> OLD.generation
  OR NEW.target_snapshot_digest <> OLD.target_snapshot_digest
  OR NEW.manifest_digest <> OLD.manifest_digest
  OR NEW.ciphertext_digest <> OLD.ciphertext_digest
  OR NEW.reason_code <> OLD.reason_code
  OR NEW.statement_expires_at <> OLD.statement_expires_at
  OR NEW.created_at <> OLD.created_at
BEGIN
  SELECT RAISE(ABORT, 'E2EE moderation report snapshot is immutable');
END;

CREATE TRIGGER IF NOT EXISTS e2ee_report_consent_immutable
BEFORE UPDATE ON e2ee_report_evidence_consents BEGIN
  SELECT RAISE(ABORT, 'E2EE report consent is immutable');
END;
CREATE TRIGGER IF NOT EXISTS e2ee_report_consent_no_delete
BEFORE DELETE ON e2ee_report_evidence_consents BEGIN
  SELECT RAISE(ABORT, 'E2EE report consent is immutable');
END;

CREATE TRIGGER IF NOT EXISTS e2ee_report_evidence_identity_immutable
BEFORE UPDATE ON e2ee_report_evidence_state
WHEN NEW.evidence_id <> OLD.evidence_id
  OR NEW.report_id <> OLD.report_id
  OR NEW.consent_id <> OLD.consent_id
  OR NEW.at_rest_ciphertext_digest <> OLD.at_rest_ciphertext_digest
  OR NEW.evidence_size_bytes <> OLD.evidence_size_bytes
  OR NEW.evidence_mime <> OLD.evidence_mime
  OR NEW.created_at <> OLD.created_at
BEGIN
  SELECT RAISE(ABORT, 'E2EE report evidence identity is immutable');
END;

CREATE TRIGGER IF NOT EXISTS e2ee_moderation_decision_identity_immutable
BEFORE UPDATE ON e2ee_moderation_decisions
WHEN NEW.id <> OLD.id OR NEW.report_id <> OLD.report_id
  OR NEW.action <> OLD.action
  OR NEW.requested_by_operator_id <> OLD.requested_by_operator_id
  OR NEW.requested_at <> OLD.requested_at
BEGIN
  SELECT RAISE(ABORT, 'E2EE moderation decision identity is immutable');
END;
`
