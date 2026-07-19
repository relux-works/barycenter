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

CREATE TABLE IF NOT EXISTS e2ee_protocol_actor_bindings (
  device_id TEXT PRIMARY KEY REFERENCES e2ee_device_public_state(device_id),
  actor_id INTEGER NOT NULL REFERENCES actors(id) CHECK(actor_id > 0),
  protocol_actor_id TEXT NOT NULL CHECK(length(protocol_actor_id) BETWEEN 8 AND 128),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  UNIQUE(actor_id, device_id),
  UNIQUE(actor_id, device_id, protocol_actor_id)
);
CREATE INDEX IF NOT EXISTS e2ee_protocol_actor_lookup
  ON e2ee_protocol_actor_bindings(protocol_actor_id, actor_id, device_id);
CREATE TRIGGER IF NOT EXISTS e2ee_protocol_actor_binding_consistent
BEFORE INSERT ON e2ee_protocol_actor_bindings
WHEN EXISTS(
  SELECT 1 FROM e2ee_protocol_actor_bindings b
  WHERE (b.actor_id = NEW.actor_id AND b.protocol_actor_id <> NEW.protocol_actor_id)
     OR (b.protocol_actor_id = NEW.protocol_actor_id AND b.actor_id <> NEW.actor_id)
)
BEGIN
  SELECT RAISE(ABORT, 'E2EE protocol actor binding conflicts');
END;
CREATE TRIGGER IF NOT EXISTS e2ee_protocol_actor_binding_immutable
BEFORE UPDATE ON e2ee_protocol_actor_bindings
BEGIN
  SELECT RAISE(ABORT, 'E2EE protocol actor binding is immutable');
END;

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

CREATE TABLE IF NOT EXISTS e2ee_group_members (
  group_id TEXT NOT NULL REFERENCES e2ee_groups(id),
  device_id TEXT NOT NULL REFERENCES e2ee_device_public_state(device_id),
  actor_id INTEGER NOT NULL REFERENCES actors(id) CHECK(actor_id > 0),
  protocol_actor_id TEXT NOT NULL CHECK(length(protocol_actor_id) BETWEEN 8 AND 128),
  actor_membership_role TEXT NOT NULL
    CHECK(actor_membership_role IN ('primary', 'companion', 'satellite')),
  actor_membership_joined_at INTEGER NOT NULL CHECK(actor_membership_joined_at > 0),
  orbit_id INTEGER NOT NULL REFERENCES orbits(id) CHECK(orbit_id > 0),
  air_membership_id TEXT NOT NULL REFERENCES air_members(public_id),
  air_role TEXT NOT NULL CHECK(air_role IN ('owner', 'admin', 'member')),
  air_membership_revision INTEGER NOT NULL CHECK(air_membership_revision > 0),
  state TEXT NOT NULL CHECK(state IN ('current', 'removed')),
  added_epoch INTEGER NOT NULL CHECK(added_epoch > 0),
  removed_epoch INTEGER NOT NULL DEFAULT 0 CHECK(removed_epoch >= 0),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at > 0),
  PRIMARY KEY(group_id, device_id),
  FOREIGN KEY(actor_id, device_id, protocol_actor_id)
    REFERENCES e2ee_protocol_actor_bindings(actor_id, device_id, protocol_actor_id),
  CHECK((state = 'current' AND removed_epoch = 0)
     OR (state = 'removed' AND removed_epoch >= added_epoch))
);
CREATE INDEX IF NOT EXISTS e2ee_group_members_current
  ON e2ee_group_members(group_id, state, device_id);

CREATE TABLE IF NOT EXISTS e2ee_rotation_requirements (
  group_id TEXT PRIMARY KEY REFERENCES e2ee_groups(id),
  base_epoch INTEGER NOT NULL CHECK(base_epoch > 0),
  observed_snapshot_digest TEXT NOT NULL CHECK(
    length(observed_snapshot_digest) = 64
      AND observed_snapshot_digest NOT GLOB '*[^0-9a-f]*'
  ),
  required_snapshot_digest TEXT NOT NULL CHECK(
    length(required_snapshot_digest) = 64
      AND required_snapshot_digest NOT GLOB '*[^0-9a-f]*'
  ),
  reason_code TEXT NOT NULL CHECK(reason_code IN (
    'actor_disable', 'air_join', 'air_leave', 'device_revoke',
    'membership_change', 'unsupported_client'
  )),
  state TEXT NOT NULL CHECK(state IN ('required', 'satisfied')),
  satisfied_epoch INTEGER NOT NULL DEFAULT 0 CHECK(satisfied_epoch >= 0),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  detected_at INTEGER NOT NULL CHECK(detected_at > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= detected_at),
  CHECK((state = 'required' AND satisfied_epoch = 0)
     OR (state = 'satisfied' AND satisfied_epoch > base_epoch))
);

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
  UNIQUE(group_id, event_digest),
  UNIQUE(group_id, id)
);
CREATE UNIQUE INDEX IF NOT EXISTS e2ee_one_accepted_commit_per_epoch
  ON e2ee_public_group_events(group_id, epoch)
  WHERE kind = 'commit' AND state = 'accepted';

CREATE TABLE IF NOT EXISTS e2ee_group_event_deliveries (
  event_id TEXT NOT NULL REFERENCES e2ee_public_group_events(id),
  group_id TEXT NOT NULL REFERENCES e2ee_groups(id),
  recipient_device_id TEXT NOT NULL REFERENCES e2ee_device_public_state(device_id),
  event_digest TEXT NOT NULL CHECK(
    length(event_digest) = 64 AND event_digest NOT GLOB '*[^0-9a-f]*'
  ),
  epoch INTEGER NOT NULL CHECK(epoch > 0),
  state TEXT NOT NULL CHECK(state IN ('pending', 'acknowledged', 'revoked')),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  acknowledged_at INTEGER NOT NULL DEFAULT 0 CHECK(acknowledged_at >= 0),
  revoked_at INTEGER NOT NULL DEFAULT 0 CHECK(revoked_at >= 0),
  PRIMARY KEY(event_id, recipient_device_id),
  FOREIGN KEY(group_id, event_id) REFERENCES e2ee_public_group_events(group_id, id),
  FOREIGN KEY(group_id, recipient_device_id)
    REFERENCES e2ee_group_members(group_id, device_id),
  CHECK((state = 'pending' AND acknowledged_at = 0 AND revoked_at = 0)
     OR (state = 'acknowledged' AND acknowledged_at > 0 AND revoked_at = 0)
     OR (state = 'revoked' AND acknowledged_at = 0 AND revoked_at > 0))
);
CREATE INDEX IF NOT EXISTS e2ee_group_event_delivery_queue
  ON e2ee_group_event_deliveries(recipient_device_id, state, created_at, event_id);
CREATE TRIGGER IF NOT EXISTS e2ee_group_event_delivery_binding_immutable
BEFORE UPDATE ON e2ee_group_event_deliveries
WHEN NEW.event_id <> OLD.event_id OR NEW.group_id <> OLD.group_id
  OR NEW.recipient_device_id <> OLD.recipient_device_id
  OR NEW.event_digest <> OLD.event_digest OR NEW.epoch <> OLD.epoch
  OR NEW.created_at <> OLD.created_at
BEGIN
  SELECT RAISE(ABORT, 'E2EE delivery binding is immutable');
END;

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

CREATE TABLE IF NOT EXISTS e2ee_protected_object_recipients (
  protected_object_id TEXT NOT NULL REFERENCES e2ee_protected_objects(id),
  recipient_device_id TEXT NOT NULL REFERENCES e2ee_device_public_state(device_id),
  actor_id INTEGER NOT NULL REFERENCES actors(id),
  protocol_actor_id TEXT NOT NULL CHECK(length(protocol_actor_id) BETWEEN 8 AND 128),
  actor_membership_role TEXT NOT NULL CHECK(length(actor_membership_role) BETWEEN 1 AND 32),
  actor_membership_joined_at INTEGER NOT NULL CHECK(actor_membership_joined_at > 0),
  orbit_id INTEGER NOT NULL REFERENCES orbits(id),
  air_membership_id TEXT NOT NULL CHECK(length(air_membership_id) BETWEEN 8 AND 128),
  air_role TEXT NOT NULL CHECK(air_role IN ('owner', 'member')),
  air_membership_revision INTEGER NOT NULL CHECK(air_membership_revision > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  PRIMARY KEY(protected_object_id, recipient_device_id)
);
CREATE INDEX IF NOT EXISTS e2ee_protected_object_recipient_device
  ON e2ee_protected_object_recipients(recipient_device_id, protected_object_id);
CREATE TRIGGER IF NOT EXISTS e2ee_protected_object_recipient_immutable
BEFORE UPDATE ON e2ee_protected_object_recipients BEGIN
  SELECT RAISE(ABORT, 'protected object recipient is immutable');
END;

CREATE TABLE IF NOT EXISTS e2ee_protected_object_chunks (
  protected_object_id TEXT NOT NULL REFERENCES e2ee_protected_objects(id),
  chunk_index INTEGER NOT NULL CHECK(chunk_index >= 0 AND chunk_index < 1024),
  byte_offset INTEGER NOT NULL CHECK(byte_offset >= 0),
  ciphertext_size INTEGER NOT NULL CHECK(ciphertext_size BETWEEN 1 AND 1048576),
  ciphertext_digest TEXT NOT NULL CHECK(
    length(ciphertext_digest) = 64 AND ciphertext_digest NOT GLOB '*[^0-9a-f]*'
  ),
  ciphertext BLOB NOT NULL CHECK(length(ciphertext) = ciphertext_size),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  PRIMARY KEY(protected_object_id, chunk_index),
  UNIQUE(protected_object_id, byte_offset)
);
CREATE INDEX IF NOT EXISTS e2ee_protected_object_chunk_range
  ON e2ee_protected_object_chunks(protected_object_id, byte_offset, chunk_index);
CREATE TRIGGER IF NOT EXISTS e2ee_protected_object_chunk_immutable
BEFORE UPDATE ON e2ee_protected_object_chunks BEGIN
  SELECT RAISE(ABORT, 'protected object chunk is immutable');
END;

CREATE TABLE IF NOT EXISTS e2ee_protected_egress_usage (
  recipient_device_id TEXT PRIMARY KEY REFERENCES e2ee_device_public_state(device_id),
  window_started_at INTEGER NOT NULL CHECK(window_started_at > 0),
  charged_bytes INTEGER NOT NULL DEFAULT 0 CHECK(charged_bytes >= 0),
  actual_bytes INTEGER NOT NULL DEFAULT 0 CHECK(actual_bytes >= 0),
  range_requests INTEGER NOT NULL DEFAULT 0 CHECK(range_requests >= 0),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= window_started_at)
);

CREATE TABLE IF NOT EXISTS e2ee_opaque_live_sessions (
  session_id TEXT PRIMARY KEY CHECK(
    length(session_id) = 32 AND session_id NOT GLOB '*[^0-9a-f]*'
  ),
  group_id TEXT NOT NULL REFERENCES e2ee_groups(id),
  author_device_id TEXT NOT NULL REFERENCES e2ee_device_public_state(device_id),
  epoch INTEGER NOT NULL CHECK(epoch > 0),
  generation INTEGER NOT NULL CHECK(generation > 0),
  target_snapshot_digest TEXT NOT NULL CHECK(
    length(target_snapshot_digest) = 64
      AND target_snapshot_digest NOT GLOB '*[^0-9a-f]*'
  ),
  header_digest TEXT NOT NULL CHECK(
    length(header_digest) = 64 AND header_digest NOT GLOB '*[^0-9a-f]*'
  ),
  opaque_header BLOB NOT NULL CHECK(length(opaque_header) BETWEEN 1 AND 4096),
  state TEXT NOT NULL CHECK(state IN ('active', 'terminal')),
  terminal_reason TEXT NOT NULL DEFAULT '' CHECK(length(terminal_reason) <= 64),
  last_sequence INTEGER NOT NULL DEFAULT 0 CHECK(last_sequence >= 0),
  last_capture_us INTEGER NOT NULL DEFAULT 0 CHECK(last_capture_us >= 0),
  last_frame_digest TEXT NOT NULL DEFAULT '' CHECK(
    last_frame_digest = '' OR (length(last_frame_digest) = 64
      AND last_frame_digest NOT GLOB '*[^0-9a-f]*')
  ),
  rate_tokens_milli INTEGER NOT NULL DEFAULT 8000 CHECK(rate_tokens_milli BETWEEN 0 AND 8000),
  rate_at INTEGER NOT NULL CHECK(rate_at > 0),
  relayed_bytes INTEGER NOT NULL DEFAULT 0 CHECK(relayed_bytes >= 0),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  started_at INTEGER NOT NULL CHECK(started_at > 0),
  expires_at INTEGER NOT NULL CHECK(expires_at > started_at),
  updated_at INTEGER NOT NULL CHECK(updated_at >= started_at),
  ended_at INTEGER NOT NULL DEFAULT 0 CHECK(ended_at >= 0),
  CHECK((state = 'active' AND terminal_reason = '' AND ended_at = 0)
     OR (state = 'terminal' AND terminal_reason <> '' AND ended_at > 0))
);
CREATE UNIQUE INDEX IF NOT EXISTS e2ee_one_active_live_session_per_group
  ON e2ee_opaque_live_sessions(group_id) WHERE state = 'active';
CREATE UNIQUE INDEX IF NOT EXISTS e2ee_live_sender_generation
  ON e2ee_opaque_live_sessions(group_id, author_device_id, generation);
CREATE INDEX IF NOT EXISTS e2ee_live_terminal_cleanup
  ON e2ee_opaque_live_sessions(state, ended_at, session_id);
CREATE TRIGGER IF NOT EXISTS e2ee_opaque_live_binding_immutable
BEFORE UPDATE ON e2ee_opaque_live_sessions
WHEN NEW.session_id <> OLD.session_id OR NEW.group_id <> OLD.group_id
  OR NEW.author_device_id <> OLD.author_device_id OR NEW.epoch <> OLD.epoch
  OR NEW.generation <> OLD.generation
  OR NEW.target_snapshot_digest <> OLD.target_snapshot_digest
  OR NEW.header_digest <> OLD.header_digest OR NEW.opaque_header <> OLD.opaque_header
  OR NEW.started_at <> OLD.started_at OR NEW.expires_at <> OLD.expires_at
BEGIN
  SELECT RAISE(ABORT, 'opaque live session binding is immutable');
END;

CREATE TABLE IF NOT EXISTS e2ee_opaque_live_recipients (
  session_id TEXT NOT NULL REFERENCES e2ee_opaque_live_sessions(session_id),
  recipient_device_id TEXT NOT NULL REFERENCES e2ee_device_public_state(device_id),
  actor_id INTEGER NOT NULL REFERENCES actors(id),
  protocol_actor_id TEXT NOT NULL CHECK(length(protocol_actor_id) BETWEEN 8 AND 128),
  actor_membership_joined_at INTEGER NOT NULL CHECK(actor_membership_joined_at > 0),
  air_membership_id TEXT NOT NULL CHECK(length(air_membership_id) BETWEEN 8 AND 128),
  air_membership_revision INTEGER NOT NULL CHECK(air_membership_revision > 0),
  state TEXT NOT NULL DEFAULT 'active' CHECK(state IN ('active', 'terminal')),
  terminal_reason TEXT NOT NULL DEFAULT '' CHECK(length(terminal_reason) <= 64),
  last_event_sequence INTEGER NOT NULL DEFAULT 0 CHECK(last_event_sequence >= 0),
  last_receipt_state TEXT NOT NULL DEFAULT '' CHECK(last_receipt_state IN (
    '', 'accepted', 'audible_started', 'ended', 'failed', 'rejected', 'unsupported'
  )),
  last_receipt_at INTEGER NOT NULL DEFAULT 0 CHECK(last_receipt_at >= 0),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at),
  ended_at INTEGER NOT NULL DEFAULT 0 CHECK(ended_at >= 0),
  PRIMARY KEY(session_id, recipient_device_id),
  CHECK((state = 'active' AND terminal_reason = '' AND ended_at = 0)
     OR (state = 'terminal' AND terminal_reason <> '' AND ended_at > 0))
);
CREATE INDEX IF NOT EXISTS e2ee_opaque_live_recipient_active
  ON e2ee_opaque_live_recipients(session_id, state, recipient_device_id);
CREATE TRIGGER IF NOT EXISTS e2ee_opaque_live_recipient_binding_immutable
BEFORE UPDATE ON e2ee_opaque_live_recipients
WHEN NEW.session_id <> OLD.session_id
  OR NEW.recipient_device_id <> OLD.recipient_device_id
  OR NEW.actor_id <> OLD.actor_id OR NEW.protocol_actor_id <> OLD.protocol_actor_id
  OR NEW.actor_membership_joined_at <> OLD.actor_membership_joined_at
  OR NEW.air_membership_id <> OLD.air_membership_id
  OR NEW.air_membership_revision <> OLD.air_membership_revision
  OR NEW.created_at <> OLD.created_at
BEGIN
  SELECT RAISE(ABORT, 'opaque live recipient binding is immutable');
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
