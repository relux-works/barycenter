package store

// e2eeSchema is an additive, production-dark persistence boundary. Every
// payload column is either public protocol state or client-produced opaque
// ciphertext. There is deliberately no column for device private material,
// epoch/session/content keys, protected plaintext, filenames, captions,
// waveform data, or decrypted moderation evidence.
//
// The singleton CHECK keeps the capability disabled even if a future caller
// accidentally treats table existence as enablement. A later, independently
// reviewed migration must version and replace that policy before production
// activation can be possible.
const e2eeSchema = `
CREATE TABLE IF NOT EXISTS e2ee_feature_state (
  singleton INTEGER PRIMARY KEY CHECK(singleton = 1),
  contract_version TEXT NOT NULL CHECK(contract_version = 'e2ee-media-audit.v1'),
  enabled INTEGER NOT NULL DEFAULT 0 CHECK(enabled = 0),
  selected_suite TEXT NOT NULL DEFAULT '' CHECK(selected_suite = ''),
  selected_container TEXT NOT NULL DEFAULT '' CHECK(selected_container = ''),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at > 0)
);
INSERT OR IGNORE INTO e2ee_feature_state(
  singleton, contract_version, enabled, selected_suite, selected_container,
  revision, updated_at
) VALUES(1, 'e2ee-media-audit.v1', 0, '', '', 1, 1);

CREATE TABLE IF NOT EXISTS e2ee_device_public_state (
  device_id TEXT PRIMARY KEY CHECK(length(device_id) BETWEEN 8 AND 128),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  public_package BLOB NOT NULL CHECK(length(public_package) BETWEEN 1 AND 1048576),
  public_package_digest TEXT NOT NULL CHECK(
    length(public_package_digest) = 64 AND public_package_digest NOT GLOB '*[^0-9a-f]*'
  ),
  verification_state TEXT NOT NULL
    CHECK(verification_state IN ('unverified', 'verified', 'changed', 'revoked')),
  verification_digest TEXT NOT NULL DEFAULT '' CHECK(
    verification_digest = '' OR (length(verification_digest) = 64
      AND verification_digest NOT GLOB '*[^0-9a-f]*')
  ),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at),
  revoked_at INTEGER NOT NULL DEFAULT 0 CHECK(revoked_at >= 0),
  CHECK((verification_state = 'revoked' AND revoked_at > 0)
     OR (verification_state <> 'revoked' AND revoked_at = 0))
);
CREATE INDEX IF NOT EXISTS e2ee_device_public_actor_state
  ON e2ee_device_public_state(actor_id, verification_state, device_id);

CREATE TABLE IF NOT EXISTS e2ee_groups (
  id TEXT PRIMARY KEY CHECK(length(id) = 30 AND substr(id, 1, 4) = 'egp_'),
  air_id TEXT NOT NULL REFERENCES airs(public_id),
  target_snapshot_digest TEXT NOT NULL CHECK(
    length(target_snapshot_digest) = 64
      AND target_snapshot_digest NOT GLOB '*[^0-9a-f]*'
  ),
  current_epoch INTEGER NOT NULL CHECK(current_epoch > 0),
  commit_digest TEXT NOT NULL CHECK(
    length(commit_digest) = 64 AND commit_digest NOT GLOB '*[^0-9a-f]*'
  ),
  fork_state TEXT NOT NULL DEFAULT 'clean'
    CHECK(fork_state IN ('clean', 'forked', 'revoked')),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at),
  UNIQUE(air_id, id)
);
CREATE INDEX IF NOT EXISTS e2ee_groups_air_state
  ON e2ee_groups(air_id, fork_state, updated_at, id);

CREATE TABLE IF NOT EXISTS e2ee_public_group_events (
  id TEXT PRIMARY KEY CHECK(length(id) BETWEEN 8 AND 128),
  group_id TEXT NOT NULL REFERENCES e2ee_groups(id),
  kind TEXT NOT NULL CHECK(kind IN ('key_package', 'proposal', 'commit', 'welcome')),
  author_device_id TEXT NOT NULL CHECK(length(author_device_id) BETWEEN 8 AND 128),
  previous_epoch INTEGER NOT NULL CHECK(previous_epoch >= 0),
  epoch INTEGER NOT NULL CHECK(epoch > 0),
  previous_commit_digest TEXT NOT NULL DEFAULT '' CHECK(
    previous_commit_digest = '' OR (length(previous_commit_digest) = 64
      AND previous_commit_digest NOT GLOB '*[^0-9a-f]*')
  ),
  event_digest TEXT NOT NULL CHECK(
    length(event_digest) = 64 AND event_digest NOT GLOB '*[^0-9a-f]*'
  ),
  public_payload BLOB NOT NULL CHECK(length(public_payload) BETWEEN 1 AND 1048576),
  state TEXT NOT NULL CHECK(state IN ('pending', 'accepted', 'rejected', 'revoked')),
  reason_code TEXT NOT NULL DEFAULT '' CHECK(length(reason_code) <= 64),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at),
  UNIQUE(group_id, event_digest)
);
CREATE UNIQUE INDEX IF NOT EXISTS e2ee_one_accepted_commit_per_epoch
  ON e2ee_public_group_events(group_id, epoch)
  WHERE kind = 'commit' AND state = 'accepted';

CREATE TABLE IF NOT EXISTS e2ee_protected_objects (
  id TEXT PRIMARY KEY CHECK(length(id) = 29 AND substr(id, 1, 3) = 'em_'),
  group_id TEXT NOT NULL REFERENCES e2ee_groups(id),
  source_object_id TEXT NOT NULL CHECK(length(source_object_id) BETWEEN 8 AND 128),
  object_kind TEXT NOT NULL CHECK(object_kind IN ('clip', 'track', 'saved_cue', 'live_ptt')),
  author_device_id TEXT NOT NULL CHECK(length(author_device_id) BETWEEN 8 AND 128),
  epoch INTEGER NOT NULL CHECK(epoch > 0),
  generation INTEGER NOT NULL CHECK(generation > 0),
  target_snapshot_digest TEXT NOT NULL CHECK(
    length(target_snapshot_digest) = 64
      AND target_snapshot_digest NOT GLOB '*[^0-9a-f]*'
  ),
  manifest_digest TEXT NOT NULL CHECK(
    length(manifest_digest) = 64 AND manifest_digest NOT GLOB '*[^0-9a-f]*'
  ),
  encrypted_manifest BLOB NOT NULL CHECK(length(encrypted_manifest) BETWEEN 1 AND 1048576),
  opaque_key_envelopes BLOB NOT NULL CHECK(length(opaque_key_envelopes) BETWEEN 1 AND 1048576),
  ciphertext_ref TEXT NOT NULL UNIQUE CHECK(
    length(ciphertext_ref) BETWEEN 78 AND 512 AND substr(ciphertext_ref, 1, 14) = 'ciphertext/v1/'
  ),
  ciphertext_digest TEXT NOT NULL CHECK(
    length(ciphertext_digest) = 64 AND ciphertext_digest NOT GLOB '*[^0-9a-f]*'
  ),
  ciphertext_size INTEGER NOT NULL CHECK(ciphertext_size > 0),
  chunk_count INTEGER NOT NULL CHECK(chunk_count > 0),
  declared_duration_ms INTEGER NOT NULL DEFAULT 0 CHECK(declared_duration_ms >= 0),
  status TEXT NOT NULL DEFAULT 'staged'
    CHECK(status IN ('staged', 'ready', 'revoked', 'deleted')),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at),
  finalized_at INTEGER NOT NULL DEFAULT 0 CHECK(finalized_at >= 0),
  revoked_at INTEGER NOT NULL DEFAULT 0 CHECK(revoked_at >= 0),
  deleted_at INTEGER NOT NULL DEFAULT 0 CHECK(deleted_at >= 0),
  UNIQUE(group_id, source_object_id, generation),
  CHECK((status = 'staged' AND finalized_at = 0 AND revoked_at = 0 AND deleted_at = 0)
     OR (status = 'ready' AND finalized_at > 0 AND revoked_at = 0 AND deleted_at = 0)
     OR (status = 'revoked' AND finalized_at > 0 AND revoked_at > 0 AND deleted_at = 0)
     OR (status = 'deleted' AND deleted_at > 0))
);
CREATE INDEX IF NOT EXISTS e2ee_protected_objects_group_state
  ON e2ee_protected_objects(group_id, status, epoch, id);

CREATE TRIGGER IF NOT EXISTS e2ee_protected_object_payload_immutable
BEFORE UPDATE ON e2ee_protected_objects
WHEN NEW.group_id <> OLD.group_id OR NEW.source_object_id <> OLD.source_object_id
  OR NEW.object_kind <> OLD.object_kind OR NEW.author_device_id <> OLD.author_device_id
  OR NEW.epoch <> OLD.epoch OR NEW.generation <> OLD.generation
  OR NEW.target_snapshot_digest <> OLD.target_snapshot_digest
  OR NEW.manifest_digest <> OLD.manifest_digest
  OR NEW.encrypted_manifest <> OLD.encrypted_manifest
  OR NEW.opaque_key_envelopes <> OLD.opaque_key_envelopes
  OR NEW.ciphertext_ref <> OLD.ciphertext_ref
  OR NEW.ciphertext_digest <> OLD.ciphertext_digest
  OR NEW.ciphertext_size <> OLD.ciphertext_size
  OR NEW.chunk_count <> OLD.chunk_count
  OR NEW.declared_duration_ms <> OLD.declared_duration_ms
BEGIN
  SELECT RAISE(ABORT, 'protected object payload is immutable');
END;

CREATE TABLE IF NOT EXISTS e2ee_sender_replay_state (
  group_id TEXT NOT NULL REFERENCES e2ee_groups(id),
  author_device_id TEXT NOT NULL CHECK(length(author_device_id) BETWEEN 8 AND 128),
  source_object_id TEXT NOT NULL CHECK(length(source_object_id) BETWEEN 8 AND 128),
  generation INTEGER NOT NULL CHECK(generation > 0),
  last_sequence INTEGER NOT NULL CHECK(last_sequence > 0),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at > 0),
  PRIMARY KEY(group_id, author_device_id, source_object_id)
);
CREATE TABLE IF NOT EXISTS e2ee_replay_events (
  group_id TEXT NOT NULL REFERENCES e2ee_groups(id),
  event_id TEXT NOT NULL CHECK(length(event_id) BETWEEN 8 AND 128),
  nonce_digest TEXT NOT NULL CHECK(
    length(nonce_digest) = 64 AND nonce_digest NOT GLOB '*[^0-9a-f]*'
  ),
  epoch INTEGER NOT NULL CHECK(epoch > 0),
  generation INTEGER NOT NULL CHECK(generation > 0),
  sequence INTEGER NOT NULL CHECK(sequence > 0),
  accepted_at INTEGER NOT NULL CHECK(accepted_at > 0),
  PRIMARY KEY(group_id, event_id),
  UNIQUE(group_id, nonce_digest)
);

CREATE TABLE IF NOT EXISTS e2ee_history_grants (
  id TEXT PRIMARY KEY CHECK(length(id) = 30 AND substr(id, 1, 4) = 'ehg_'),
  group_id TEXT NOT NULL REFERENCES e2ee_groups(id),
  issued_by_device_id TEXT NOT NULL CHECK(length(issued_by_device_id) BETWEEN 8 AND 128),
  recipient_device_id TEXT NOT NULL CHECK(length(recipient_device_id) BETWEEN 8 AND 128),
  source_object_id TEXT NOT NULL CHECK(length(source_object_id) BETWEEN 8 AND 128),
  first_epoch INTEGER NOT NULL CHECK(first_epoch > 0),
  last_epoch INTEGER NOT NULL CHECK(last_epoch >= first_epoch),
  target_snapshot_digest TEXT NOT NULL CHECK(
    length(target_snapshot_digest) = 64
      AND target_snapshot_digest NOT GLOB '*[^0-9a-f]*'
  ),
  encrypted_grant BLOB NOT NULL CHECK(length(encrypted_grant) BETWEEN 1 AND 1048576),
  grant_digest TEXT NOT NULL CHECK(
    length(grant_digest) = 64 AND grant_digest NOT GLOB '*[^0-9a-f]*'
  ),
  status TEXT NOT NULL DEFAULT 'active' CHECK(status IN ('active', 'revoked', 'expired')),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  issued_at INTEGER NOT NULL CHECK(issued_at > 0),
  expires_at INTEGER NOT NULL CHECK(expires_at > issued_at),
  revoked_at INTEGER NOT NULL DEFAULT 0 CHECK(revoked_at >= 0),
  UNIQUE(group_id, grant_digest),
  CHECK((status = 'active' AND revoked_at = 0)
     OR (status = 'revoked' AND revoked_at > 0)
     OR status = 'expired')
);
CREATE INDEX IF NOT EXISTS e2ee_history_grants_recipient_state
  ON e2ee_history_grants(recipient_device_id, status, expires_at, id);

CREATE TABLE IF NOT EXISTS e2ee_transfer_packages (
  id TEXT PRIMARY KEY CHECK(length(id) = 30 AND substr(id, 1, 4) = 'etp_'),
  group_id TEXT NOT NULL REFERENCES e2ee_groups(id),
  package_kind TEXT NOT NULL CHECK(package_kind IN ('device_transfer', 'recovery', 'welcome')),
  issuer_device_id TEXT NOT NULL CHECK(length(issuer_device_id) BETWEEN 8 AND 128),
  recipient_device_id TEXT NOT NULL CHECK(length(recipient_device_id) BETWEEN 8 AND 128),
  epoch INTEGER NOT NULL CHECK(epoch > 0),
  encrypted_package BLOB NOT NULL CHECK(length(encrypted_package) BETWEEN 1 AND 1048576),
  package_digest TEXT NOT NULL CHECK(
    length(package_digest) = 64 AND package_digest NOT GLOB '*[^0-9a-f]*'
  ),
  status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending', 'consumed', 'revoked', 'expired')),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  expires_at INTEGER NOT NULL CHECK(expires_at > created_at),
  terminal_at INTEGER NOT NULL DEFAULT 0 CHECK(terminal_at >= 0),
  UNIQUE(group_id, package_digest),
  CHECK((status = 'pending' AND terminal_at = 0) OR (status <> 'pending' AND terminal_at > 0))
);

CREATE TABLE IF NOT EXISTS e2ee_report_evidence_metadata (
  id TEXT PRIMARY KEY CHECK(length(id) = 30 AND substr(id, 1, 4) = 'ere_'),
  report_id TEXT NOT NULL UNIQUE CHECK(length(report_id) BETWEEN 8 AND 128),
  protected_object_id TEXT NOT NULL REFERENCES e2ee_protected_objects(id),
  reporter_actor_id INTEGER NOT NULL CHECK(reporter_actor_id > 0),
  consent_version TEXT NOT NULL CHECK(length(consent_version) BETWEEN 1 AND 128),
  consent_digest TEXT NOT NULL CHECK(
    length(consent_digest) = 64 AND consent_digest NOT GLOB '*[^0-9a-f]*'
  ),
  authenticated_evidence_digest TEXT NOT NULL CHECK(
    length(authenticated_evidence_digest) = 64
      AND authenticated_evidence_digest NOT GLOB '*[^0-9a-f]*'
  ),
  encrypted_evidence_ref TEXT NOT NULL CHECK(
    length(encrypted_evidence_ref) BETWEEN 76 AND 512
      AND substr(encrypted_evidence_ref, 1, 12) = 'evidence/v1/'
  ),
  retention_expires_at INTEGER NOT NULL CHECK(retention_expires_at > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0)
);

CREATE TABLE IF NOT EXISTS e2ee_audit_events (
  id TEXT PRIMARY KEY CHECK(length(id) = 29 AND substr(id, 1, 3) = 'ea_'),
  group_id TEXT NOT NULL DEFAULT '',
  subject_kind TEXT NOT NULL CHECK(subject_kind IN (
    'device', 'group', 'public_event', 'protected_object', 'replay',
    'history_grant', 'transfer_package', 'report_evidence'
  )),
  subject_id TEXT NOT NULL CHECK(length(subject_id) BETWEEN 1 AND 128),
  operation TEXT NOT NULL CHECK(length(operation) BETWEEN 1 AND 128),
  outcome TEXT NOT NULL CHECK(outcome IN ('accepted', 'rejected', 'revoked', 'expired', 'deleted')),
  reason_code TEXT NOT NULL DEFAULT '' CHECK(length(reason_code) <= 64),
  actor_id INTEGER NOT NULL DEFAULT 0 CHECK(actor_id >= 0),
  device_id TEXT NOT NULL DEFAULT '' CHECK(length(device_id) <= 128),
  epoch INTEGER NOT NULL DEFAULT 0 CHECK(epoch >= 0),
  revision INTEGER NOT NULL DEFAULT 0 CHECK(revision >= 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0)
);
CREATE INDEX IF NOT EXISTS e2ee_audit_group_created
  ON e2ee_audit_events(group_id, created_at, id);
CREATE TRIGGER IF NOT EXISTS e2ee_audit_events_no_update
BEFORE UPDATE ON e2ee_audit_events BEGIN
  SELECT RAISE(ABORT, 'E2EE audit events are immutable');
END;
CREATE TRIGGER IF NOT EXISTS e2ee_audit_events_no_delete
BEFORE DELETE ON e2ee_audit_events BEGIN
  SELECT RAISE(ABORT, 'E2EE audit events are immutable');
END;
`

func (s *Store) initE2EESchema() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(e2eeSchema); err != nil {
		return err
	}
	if err := foreignKeyCheck(tx); err != nil {
		return err
	}
	if err := s.checkpoint("e2ee_ddl_before_commit"); err != nil {
		return err
	}
	return tx.Commit()
}
