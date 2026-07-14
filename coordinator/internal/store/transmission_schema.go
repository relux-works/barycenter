package store

// transmissionSchema is additive and deliberately keeps legacy authority
// tables out of its foreign-key graph. A rollback coordinator can continue to
// mutate or dissolve legacy orbits without knowing about accepted
// transmissions. References wholly inside the generic-media/transmission
// model remain protected because rollback binaries never delete those rows.
const transmissionSchema = `
CREATE TABLE IF NOT EXISTS transmissions (
  id TEXT PRIMARY KEY
    CHECK(length(id) = 29 AND substr(id, 1, 3) = 'tr_'),
  media_id TEXT NOT NULL REFERENCES media_items(id),
  source_orbit_id INTEGER NOT NULL CHECK(source_orbit_id > 0),
  source_actor_id INTEGER NOT NULL CHECK(source_actor_id > 0),
  source_slot TEXT NOT NULL DEFAULT ''
    CHECK(source_slot = '' OR (length(source_slot) = 1 AND source_slot GLOB '[a-z]')),
  playback_domain_kind TEXT NOT NULL
    CHECK(playback_domain_kind IN ('orbit', 'approach')),
  playback_domain_id INTEGER NOT NULL CHECK(playback_domain_id > 0),
  audience_kind TEXT NOT NULL
    CHECK(audience_kind IN ('this_pulsar', 'own_barycenter', 'current_air', 'explicit')),
  origin_kind TEXT NOT NULL
    CHECK(origin_kind IN ('microphone', 'file', 'telegram', 'builtin')),
  include_origin INTEGER NOT NULL CHECK(include_origin IN (0, 1)),
  requested_delivery TEXT NOT NULL
    CHECK(requested_delivery IN ('overlay', 'interrupt', 'after_current')),
  effective_delivery TEXT NOT NULL
    CHECK(effective_delivery IN ('overlay', 'interrupt', 'after_current')),
  downgrade_reason TEXT NOT NULL DEFAULT '' CHECK(length(downgrade_reason) <= 64),
  status TEXT NOT NULL DEFAULT 'accepted'
    CHECK(status IN (
      'accepted', 'preparing', 'scheduled', 'playing', 'cancelling',
      'played', 'partial', 'failed', 'cancelled', 'expired'
    )),
  reason_code TEXT NOT NULL DEFAULT '' CHECK(length(reason_code) <= 64),
  cancellation_cause TEXT NOT NULL DEFAULT '' CHECK(length(cancellation_cause) <= 64),
  accepted_at INTEGER NOT NULL CHECK(accepted_at > 0),
  expires_at INTEGER NOT NULL CHECK(expires_at > accepted_at),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= accepted_at),
  completed_at INTEGER NOT NULL DEFAULT 0 CHECK(completed_at >= 0),
  CHECK(completed_at = 0 OR completed_at >= accepted_at),
  CHECK(status NOT IN ('played', 'partial', 'failed', 'cancelled', 'expired')
    OR completed_at > 0)
);
CREATE INDEX IF NOT EXISTS transmissions_domain_fifo
  ON transmissions(playback_domain_kind, playback_domain_id, accepted_at, id);
CREATE INDEX IF NOT EXISTS transmissions_media_lifecycle
  ON transmissions(media_id, status, accepted_at, id);
CREATE INDEX IF NOT EXISTS transmissions_source_history
  ON transmissions(source_actor_id, accepted_at DESC, id DESC);

CREATE TABLE IF NOT EXISTS transmission_targets (
  transmission_id TEXT NOT NULL REFERENCES transmissions(id) ON DELETE CASCADE,
  orbit_id INTEGER NOT NULL CHECK(orbit_id > 0),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  slot TEXT NOT NULL CHECK(length(slot) = 1 AND slot GLOB '[a-z]'),
  binding_paired_at INTEGER NOT NULL CHECK(binding_paired_at >= 0),
  online_at_acceptance INTEGER NOT NULL CHECK(online_at_acceptance IN (0, 1)),
  media_clip_capable INTEGER NOT NULL CHECK(media_clip_capable IN (0, 1)),
  overlay_capable INTEGER NOT NULL CHECK(overlay_capable IN (0, 1)),
  interrupt_capable INTEGER NOT NULL CHECK(interrupt_capable IN (0, 1)),
  interrupt_resume_ready INTEGER NOT NULL CHECK(interrupt_resume_ready IN (0, 1)),
  status TEXT NOT NULL DEFAULT 'accepted'
    CHECK(status IN (
      'accepted', 'preparing', 'ready', 'scheduled', 'playing', 'cancelling',
      'played', 'missed_offline', 'missed_dnd', 'missed_not_ready', 'blocked',
      'failed', 'cancelled', 'expired'
    )),
  reason_code TEXT NOT NULL DEFAULT '' CHECK(length(reason_code) <= 64),
  generation INTEGER NOT NULL DEFAULT 1 CHECK(generation > 0),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  ready_at INTEGER NOT NULL DEFAULT 0 CHECK(ready_at >= 0),
  scheduled_at INTEGER NOT NULL DEFAULT 0 CHECK(scheduled_at >= 0),
  started_at INTEGER NOT NULL DEFAULT 0 CHECK(started_at >= 0),
  ended_at INTEGER NOT NULL DEFAULT 0 CHECK(ended_at >= 0),
  last_receipt_at INTEGER NOT NULL DEFAULT 0 CHECK(last_receipt_at >= 0),
  updated_at INTEGER NOT NULL CHECK(updated_at > 0),
  PRIMARY KEY (transmission_id, orbit_id, slot),
  UNIQUE (transmission_id, actor_id),
  CHECK(ready_at = 0 OR ready_at <= updated_at),
  CHECK(scheduled_at = 0 OR scheduled_at <= updated_at),
  CHECK(started_at = 0 OR started_at <= updated_at),
  CHECK(ended_at = 0 OR ended_at <= updated_at),
  CHECK(last_receipt_at = 0 OR last_receipt_at <= updated_at)
);
CREATE INDEX IF NOT EXISTS transmission_targets_actor_acl
  ON transmission_targets(actor_id, orbit_id, slot, transmission_id);
CREATE INDEX IF NOT EXISTS transmission_targets_work
  ON transmission_targets(status, updated_at, transmission_id);

-- Only lifecycle/receipt columns may change after acceptance. SQLite triggers
-- protect the invariant even when later workers use direct SQL accidentally.
CREATE TRIGGER IF NOT EXISTS transmissions_acceptance_immutable
BEFORE UPDATE ON transmissions
WHEN NEW.id <> OLD.id
  OR NEW.media_id <> OLD.media_id
  OR NEW.source_orbit_id <> OLD.source_orbit_id
  OR NEW.source_actor_id <> OLD.source_actor_id
  OR NEW.source_slot <> OLD.source_slot
  OR NEW.playback_domain_kind <> OLD.playback_domain_kind
  OR NEW.playback_domain_id <> OLD.playback_domain_id
  OR NEW.audience_kind <> OLD.audience_kind
  OR NEW.origin_kind <> OLD.origin_kind
  OR NEW.include_origin <> OLD.include_origin
  OR NEW.requested_delivery <> OLD.requested_delivery
  OR NEW.effective_delivery <> OLD.effective_delivery
  OR NEW.downgrade_reason <> OLD.downgrade_reason
  OR NEW.accepted_at <> OLD.accepted_at
  OR NEW.expires_at <> OLD.expires_at
BEGIN
  SELECT RAISE(ABORT, 'transmission acceptance snapshot is immutable');
END;

CREATE TRIGGER IF NOT EXISTS transmission_targets_snapshot_immutable
BEFORE UPDATE ON transmission_targets
WHEN NEW.transmission_id <> OLD.transmission_id
  OR NEW.orbit_id <> OLD.orbit_id
  OR NEW.actor_id <> OLD.actor_id
  OR NEW.slot <> OLD.slot
  OR NEW.binding_paired_at <> OLD.binding_paired_at
  OR NEW.online_at_acceptance <> OLD.online_at_acceptance
  OR NEW.media_clip_capable <> OLD.media_clip_capable
  OR NEW.overlay_capable <> OLD.overlay_capable
  OR NEW.interrupt_capable <> OLD.interrupt_capable
  OR NEW.interrupt_resume_ready <> OLD.interrupt_resume_ready
BEGIN
  SELECT RAISE(ABORT, 'transmission target snapshot is immutable');
END;

CREATE TABLE IF NOT EXISTS blocks (
  id INTEGER PRIMARY KEY,
  owner_scope TEXT NOT NULL CHECK(owner_scope IN ('actor', 'orbit')),
  owner_orbit_id INTEGER NOT NULL CHECK(owner_orbit_id > 0),
  owner_actor_id INTEGER NOT NULL DEFAULT 0 CHECK(owner_actor_id >= 0),
  blocked_kind TEXT NOT NULL CHECK(blocked_kind IN ('actor', 'orbit')),
  blocked_actor_id INTEGER NOT NULL DEFAULT 0 CHECK(blocked_actor_id >= 0),
  blocked_orbit_id INTEGER NOT NULL DEFAULT 0 CHECK(blocked_orbit_id >= 0),
  created_by_actor_id INTEGER NOT NULL CHECK(created_by_actor_id > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  revoked_at INTEGER NOT NULL DEFAULT 0 CHECK(revoked_at >= 0),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  CHECK((owner_scope = 'actor' AND owner_actor_id > 0)
    OR (owner_scope = 'orbit' AND owner_actor_id = 0)),
  CHECK((blocked_kind = 'actor' AND blocked_actor_id > 0 AND blocked_orbit_id = 0)
    OR (blocked_kind = 'orbit' AND blocked_actor_id = 0 AND blocked_orbit_id > 0)),
  CHECK(revoked_at = 0 OR revoked_at >= created_at)
);
CREATE UNIQUE INDEX IF NOT EXISTS blocks_one_active
  ON blocks(
    owner_scope, owner_orbit_id, owner_actor_id,
    blocked_kind, blocked_actor_id, blocked_orbit_id
  ) WHERE revoked_at = 0;
CREATE INDEX IF NOT EXISTS blocks_recipient_lookup
  ON blocks(owner_orbit_id, owner_actor_id, revoked_at, blocked_kind);

CREATE TABLE IF NOT EXISTS node_dnd_settings (
  orbit_id INTEGER NOT NULL CHECK(orbit_id > 0),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  slot TEXT NOT NULL CHECK(length(slot) = 1 AND slot GLOB '[a-z]'),
  binding_paired_at INTEGER NOT NULL CHECK(binding_paired_at >= 0),
  mode TEXT NOT NULL CHECK(mode IN ('allow_all', 'messages_only', 'muted_until')),
  muted_until INTEGER NOT NULL DEFAULT 0 CHECK(muted_until >= 0),
  revision INTEGER NOT NULL CHECK(revision > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at > 0),
  PRIMARY KEY (orbit_id, actor_id, slot),
  CHECK((mode = 'muted_until' AND muted_until > updated_at)
    OR (mode <> 'muted_until' AND muted_until = 0))
);

CREATE TABLE IF NOT EXISTS orbit_dnd_settings (
  orbit_id INTEGER PRIMARY KEY CHECK(orbit_id > 0),
  mode TEXT NOT NULL CHECK(mode IN ('allow_all', 'messages_only', 'muted_until')),
  muted_until INTEGER NOT NULL DEFAULT 0 CHECK(muted_until >= 0),
  revision INTEGER NOT NULL CHECK(revision > 0),
  updated_by_actor_id INTEGER NOT NULL CHECK(updated_by_actor_id > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at > 0),
  CHECK((mode = 'muted_until' AND muted_until > updated_at)
    OR (mode <> 'muted_until' AND muted_until = 0))
);
`

func (s *Store) initTransmissionSchema() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(transmissionSchema); err != nil {
		return err
	}
	if err := foreignKeyCheck(tx); err != nil {
		return err
	}
	if err := s.checkpoint("transmission_ddl_before_commit"); err != nil {
		return err
	}
	return tx.Commit()
}
