package store

// e2eeRecoverySchema binds the ciphertext-only foundation tables to exact
// verified device and group-member lineage. The coordinator stores opaque
// packages only; it never receives a recovery secret, group state, or media key.
const e2eeRecoverySchema = `
CREATE TABLE IF NOT EXISTS e2ee_transfer_package_bindings (
  package_id TEXT PRIMARY KEY REFERENCES e2ee_transfer_packages(id),
  group_id TEXT NOT NULL REFERENCES e2ee_groups(id),
  issuer_actor_id INTEGER NOT NULL REFERENCES actors(id) CHECK(issuer_actor_id > 0),
  issuer_orbit_id INTEGER NOT NULL REFERENCES orbits(id) CHECK(issuer_orbit_id > 0),
  recipient_actor_id INTEGER NOT NULL REFERENCES actors(id) CHECK(recipient_actor_id > 0),
  recipient_orbit_id INTEGER NOT NULL REFERENCES orbits(id) CHECK(recipient_orbit_id > 0),
  target_snapshot_digest TEXT NOT NULL CHECK(
    length(target_snapshot_digest) = 64
      AND target_snapshot_digest NOT GLOB '*[^0-9a-f]*'
  ),
  issuer_member_revision INTEGER NOT NULL CHECK(issuer_member_revision > 0),
  recipient_member_revision INTEGER NOT NULL CHECK(recipient_member_revision > 0),
  issuer_device_revision INTEGER NOT NULL CHECK(issuer_device_revision > 0),
  issuer_verification_digest TEXT NOT NULL CHECK(
    length(issuer_verification_digest) = 64
      AND issuer_verification_digest NOT GLOB '*[^0-9a-f]*'
  ),
  recipient_device_revision INTEGER NOT NULL CHECK(recipient_device_revision > 0),
  recipient_verification_digest TEXT NOT NULL CHECK(
    length(recipient_verification_digest) = 64
      AND recipient_verification_digest NOT GLOB '*[^0-9a-f]*'
  ),
  consumed_at INTEGER NOT NULL DEFAULT 0 CHECK(consumed_at >= 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0)
);
CREATE INDEX IF NOT EXISTS e2ee_transfer_packages_expiry
  ON e2ee_transfer_packages(status, expires_at, id);

CREATE TABLE IF NOT EXISTS e2ee_history_grant_bindings (
  grant_id TEXT PRIMARY KEY REFERENCES e2ee_history_grants(id),
  group_id TEXT NOT NULL REFERENCES e2ee_groups(id),
  issuer_actor_id INTEGER NOT NULL REFERENCES actors(id) CHECK(issuer_actor_id > 0),
  issuer_orbit_id INTEGER NOT NULL REFERENCES orbits(id) CHECK(issuer_orbit_id > 0),
  recipient_actor_id INTEGER NOT NULL REFERENCES actors(id) CHECK(recipient_actor_id > 0),
  recipient_orbit_id INTEGER NOT NULL REFERENCES orbits(id) CHECK(recipient_orbit_id > 0),
  recipient_device_revision INTEGER NOT NULL CHECK(recipient_device_revision > 0),
  recipient_verification_digest TEXT NOT NULL CHECK(
    length(recipient_verification_digest) = 64
      AND recipient_verification_digest NOT GLOB '*[^0-9a-f]*'
  ),
  issuer_member_revision INTEGER NOT NULL CHECK(issuer_member_revision > 0),
  recipient_member_revision INTEGER NOT NULL CHECK(recipient_member_revision > 0),
  issuer_device_revision INTEGER NOT NULL CHECK(issuer_device_revision > 0),
  issuer_verification_digest TEXT NOT NULL CHECK(
    length(issuer_verification_digest) = 64
      AND issuer_verification_digest NOT GLOB '*[^0-9a-f]*'
  ),
  access_mode TEXT NOT NULL CHECK(access_mode IN ('one_time', 'time_bound')),
  max_reads INTEGER NOT NULL CHECK(max_reads BETWEEN 1 AND 32),
  read_count INTEGER NOT NULL DEFAULT 0 CHECK(read_count BETWEEN 0 AND max_reads),
  approved_at INTEGER NOT NULL CHECK(approved_at > 0),
  first_accessed_at INTEGER NOT NULL DEFAULT 0 CHECK(first_accessed_at >= 0),
  last_accessed_at INTEGER NOT NULL DEFAULT 0 CHECK(last_accessed_at >= 0),
  CHECK((read_count = 0 AND first_accessed_at = 0 AND last_accessed_at = 0)
     OR (read_count > 0 AND first_accessed_at >= approved_at
       AND last_accessed_at >= first_accessed_at))
);

CREATE TRIGGER IF NOT EXISTS e2ee_transfer_package_binding_immutable
BEFORE UPDATE ON e2ee_transfer_package_bindings
WHEN NEW.package_id <> OLD.package_id OR NEW.group_id <> OLD.group_id
  OR NEW.issuer_actor_id <> OLD.issuer_actor_id
  OR NEW.issuer_orbit_id <> OLD.issuer_orbit_id
  OR NEW.recipient_actor_id <> OLD.recipient_actor_id
  OR NEW.recipient_orbit_id <> OLD.recipient_orbit_id
  OR NEW.target_snapshot_digest <> OLD.target_snapshot_digest
  OR NEW.issuer_member_revision <> OLD.issuer_member_revision
  OR NEW.recipient_member_revision <> OLD.recipient_member_revision
  OR NEW.issuer_device_revision <> OLD.issuer_device_revision
  OR NEW.issuer_verification_digest <> OLD.issuer_verification_digest
  OR NEW.recipient_device_revision <> OLD.recipient_device_revision
  OR NEW.recipient_verification_digest <> OLD.recipient_verification_digest
  OR NEW.created_at <> OLD.created_at
BEGIN
  SELECT RAISE(ABORT, 'E2EE transfer package binding is immutable');
END;

CREATE TRIGGER IF NOT EXISTS e2ee_history_grant_binding_immutable
BEFORE UPDATE ON e2ee_history_grant_bindings
WHEN NEW.grant_id <> OLD.grant_id OR NEW.group_id <> OLD.group_id
  OR NEW.issuer_actor_id <> OLD.issuer_actor_id
  OR NEW.issuer_orbit_id <> OLD.issuer_orbit_id
  OR NEW.recipient_actor_id <> OLD.recipient_actor_id
  OR NEW.recipient_orbit_id <> OLD.recipient_orbit_id
  OR NEW.recipient_device_revision <> OLD.recipient_device_revision
  OR NEW.recipient_verification_digest <> OLD.recipient_verification_digest
  OR NEW.issuer_member_revision <> OLD.issuer_member_revision
  OR NEW.recipient_member_revision <> OLD.recipient_member_revision
  OR NEW.issuer_device_revision <> OLD.issuer_device_revision
  OR NEW.issuer_verification_digest <> OLD.issuer_verification_digest
  OR NEW.access_mode <> OLD.access_mode OR NEW.max_reads <> OLD.max_reads
  OR NEW.approved_at <> OLD.approved_at
BEGIN
  SELECT RAISE(ABORT, 'E2EE history grant binding is immutable');
END;
`
