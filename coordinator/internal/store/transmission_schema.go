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
  air_id TEXT NOT NULL DEFAULT '',
  air_policy_revision INTEGER NOT NULL DEFAULT 0 CHECK(air_policy_revision >= 0),
  air_policy_operation TEXT NOT NULL DEFAULT ''
    CHECK(air_policy_operation IN ('', 'overlay', 'queue', 'replace')),
  air_policy_result TEXT NOT NULL DEFAULT ''
    CHECK(air_policy_result IN ('', 'allowed')),
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

-- Caller idempotency is scoped to the resolved actor. Only digests are
-- durable: neither the Idempotency-Key nor the canonical request body is
-- persisted in plaintext.
CREATE TABLE IF NOT EXISTS transmission_requests (
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  idempotency_key_hash TEXT NOT NULL
    CHECK(length(idempotency_key_hash) = 64
      AND idempotency_key_hash NOT GLOB '*[^0-9a-f]*'),
  request_hash TEXT NOT NULL
    CHECK(length(request_hash) = 64
      AND request_hash NOT GLOB '*[^0-9a-f]*'),
  transmission_id TEXT NOT NULL REFERENCES transmissions(id) ON DELETE CASCADE,
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  PRIMARY KEY (actor_id, idempotency_key_hash),
  UNIQUE (transmission_id)
);

-- Interrupt fallback challenges are deliberately separate from accepted
-- transmissions: issuing a challenge must not reserve a FIFO position. The
-- opaque token is retained only as a digest and exact replay is rejected even
-- while the consumed row is retained for the contract's five-minute window.
CREATE TABLE IF NOT EXISTS transmission_fallback_confirmations (
  token_hash TEXT PRIMARY KEY
    CHECK(length(token_hash) = 64
      AND token_hash NOT GLOB '*[^0-9a-f]*'),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  idempotency_key_hash TEXT NOT NULL
    CHECK(length(idempotency_key_hash) = 64
      AND idempotency_key_hash NOT GLOB '*[^0-9a-f]*'),
  request_hash TEXT NOT NULL
    CHECK(length(request_hash) = 64
      AND request_hash NOT GLOB '*[^0-9a-f]*'),
  overlay_available INTEGER NOT NULL CHECK(overlay_available IN (0, 1)),
  after_current_available INTEGER NOT NULL
    CHECK(after_current_available IN (0, 1)),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  expires_at INTEGER NOT NULL CHECK(expires_at > created_at),
  consumed_at INTEGER NOT NULL DEFAULT 0 CHECK(consumed_at >= 0),
  CHECK(consumed_at = 0 OR consumed_at >= created_at)
);
CREATE INDEX IF NOT EXISTS transmission_confirmations_expiry
  ON transmission_fallback_confirmations(expires_at, consumed_at);

-- Explicit target selectors are bearer capabilities, never serialized
-- actor/orbit/slot identities. Only a SHA-256 digest is durable. The current
-- ActorContext, credential scope, caller domain and target binding are all
-- revalidated when a transmission is accepted, so copied, stale and forged
-- references share the same non-existence surface.
CREATE TABLE IF NOT EXISTS transmission_target_references (
  reference_hash TEXT PRIMARY KEY
    CHECK(length(reference_hash) = 64
      AND reference_hash NOT GLOB '*[^0-9a-f]*'),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  authorization_hash TEXT NOT NULL
    CHECK(length(authorization_hash) = 64
      AND authorization_hash NOT GLOB '*[^0-9a-f]*'),
  target_kind TEXT NOT NULL CHECK(target_kind IN ('barycenter', 'pulsar')),
  target_orbit_id INTEGER NOT NULL CHECK(target_orbit_id > 0),
  target_actor_id INTEGER NOT NULL DEFAULT 0 CHECK(target_actor_id >= 0),
  target_slot TEXT NOT NULL DEFAULT '' CHECK(
    target_slot = '' OR (length(target_slot) = 1 AND target_slot GLOB '[a-z]')
  ),
  target_binding_paired_at INTEGER NOT NULL DEFAULT 0
    CHECK(target_binding_paired_at >= 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  expires_at INTEGER NOT NULL CHECK(expires_at > created_at),
  CHECK((target_kind = 'barycenter' AND target_actor_id = 0
      AND target_slot = '' AND target_binding_paired_at = 0)
    OR (target_kind = 'pulsar' AND target_actor_id > 0
      AND target_slot <> ''))
);
CREATE INDEX IF NOT EXISTS transmission_target_references_actor
  ON transmission_target_references(actor_id, expires_at, target_kind);

CREATE TABLE IF NOT EXISTS transmission_targets (
  transmission_id TEXT NOT NULL REFERENCES transmissions(id) ON DELETE CASCADE,
  orbit_id INTEGER NOT NULL CHECK(orbit_id > 0),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  slot TEXT NOT NULL CHECK(length(slot) = 1 AND slot GLOB '[a-z]'),
  binding_paired_at INTEGER NOT NULL CHECK(binding_paired_at >= 0),
  capability_set_hash TEXT NOT NULL DEFAULT
    'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855'
    CHECK(length(capability_set_hash) = 64
      AND capability_set_hash NOT GLOB '*[^0-9a-f]*'),
  resolved_at_ms INTEGER NOT NULL DEFAULT 0 CHECK(resolved_at_ms >= 0),
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
CREATE INDEX IF NOT EXISTS transmission_targets_receipt_history
  ON transmission_targets(
    actor_id, binding_paired_at, last_receipt_at DESC, transmission_id DESC
  );
CREATE UNIQUE INDEX IF NOT EXISTS transmission_targets_inbox_owner
  ON transmission_targets(
    transmission_id, orbit_id, actor_id, slot, binding_paired_at
  );

-- One inbox item belongs to one immutable target binding.  It is created in
-- the same transaction as the first eligible terminal receipt.  The unique
-- owner key is the database-level exactly-once guard; Air membership is not
-- represented anywhere in this graph and therefore cannot expand it later.
CREATE TABLE IF NOT EXISTS transmission_inbox_items (
  id TEXT PRIMARY KEY
    CHECK(length(id) = 29 AND substr(id, 1, 3) = 'ib_'),
  transmission_id TEXT NOT NULL,
  media_id TEXT NOT NULL REFERENCES media_items(id),
  orbit_id INTEGER NOT NULL CHECK(orbit_id > 0),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  slot TEXT NOT NULL CHECK(length(slot) = 1 AND slot GLOB '[a-z]'),
  binding_paired_at INTEGER NOT NULL CHECK(binding_paired_at >= 0),
  media_kind TEXT NOT NULL CHECK(media_kind IN (
    'voice_clip', 'audio_clip', 'audio_track', 'builtin_cue'
  )),
  requested_delivery TEXT NOT NULL
    CHECK(requested_delivery IN ('overlay', 'interrupt', 'after_current')),
  effective_delivery TEXT NOT NULL
    CHECK(effective_delivery IN ('overlay', 'interrupt', 'after_current')),
  missed_status TEXT NOT NULL CHECK(missed_status IN (
    'missed_offline', 'missed_dnd', 'missed_not_ready', 'failed'
  )),
  missed_reason TEXT NOT NULL CHECK(missed_reason IN (
    'offline_at_acceptance', 'offline_before_prepare', 'offline_before_start',
    'local_dnd', 'orbit_dnd', 'prepare_deadline', 'connection_lost',
    'device_unavailable', 'audio_graph_failed'
  )),
  availability TEXT NOT NULL DEFAULT 'available'
    CHECK(availability IN (
      'available', 'dismissed', 'replayed', 'unavailable', 'expired'
    )),
  replay_of_inbox_id TEXT NOT NULL DEFAULT '' CHECK(
    replay_of_inbox_id = '' OR
    (length(replay_of_inbox_id) = 29 AND substr(replay_of_inbox_id, 1, 3) = 'ib_')
  ),
  replay_of_transmission_id TEXT NOT NULL DEFAULT '' CHECK(
    replay_of_transmission_id = '' OR
    (length(replay_of_transmission_id) = 29
      AND substr(replay_of_transmission_id, 1, 3) = 'tr_')
  ),
  replay_root_transmission_id TEXT NOT NULL CHECK(
    length(replay_root_transmission_id) = 29
      AND substr(replay_root_transmission_id, 1, 3) = 'tr_'
  ),
  replay_depth INTEGER NOT NULL DEFAULT 0 CHECK(replay_depth BETWEEN 0 AND 8),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at),
  expires_at INTEGER NOT NULL CHECK(expires_at >= created_at),
  dismissed_at INTEGER NOT NULL DEFAULT 0 CHECK(dismissed_at >= 0),
  consumed_at INTEGER NOT NULL DEFAULT 0 CHECK(consumed_at >= 0),
  revoked_at INTEGER NOT NULL DEFAULT 0 CHECK(revoked_at >= 0),
  revocation_reason TEXT NOT NULL DEFAULT '' CHECK(revocation_reason IN (
    '', 'media_deleted', 'media_expired', 'moderation_disabled',
    'target_revoked', 'reported'
  )),
  UNIQUE(transmission_id, orbit_id, actor_id, slot, binding_paired_at),
  FOREIGN KEY(transmission_id, orbit_id, actor_id, slot, binding_paired_at)
    REFERENCES transmission_targets(
      transmission_id, orbit_id, actor_id, slot, binding_paired_at
    ),
  CHECK(availability <> 'dismissed' OR dismissed_at > 0),
  CHECK(availability <> 'replayed' OR consumed_at > 0),
  CHECK((availability = 'unavailable' AND revoked_at > 0
      AND revocation_reason <> '')
    OR (availability <> 'unavailable' AND revoked_at = 0
      AND revocation_reason = '')),
  CHECK(replay_depth > 0 OR (replay_of_inbox_id = ''
    AND replay_of_transmission_id = '')),
  CHECK(replay_depth = 0 OR (replay_of_inbox_id <> ''
    AND replay_of_transmission_id <> ''))
);
CREATE INDEX IF NOT EXISTS transmission_inbox_target_page
  ON transmission_inbox_items(
    actor_id, orbit_id, slot, binding_paired_at, created_at DESC, id DESC
  );
CREATE INDEX IF NOT EXISTS transmission_inbox_media_revocation
  ON transmission_inbox_items(media_id, availability, expires_at);

-- Replay lineage is attached to the newly accepted transmission instead of
-- rewriting its source inbox receipt.  A missed replay can therefore inherit
-- the same root/depth when its own inbox row is materialized.
CREATE TABLE IF NOT EXISTS transmission_replay_lineage (
  transmission_id TEXT PRIMARY KEY REFERENCES transmissions(id) ON DELETE CASCADE,
  replay_of_inbox_id TEXT NOT NULL REFERENCES transmission_inbox_items(id),
  replay_of_transmission_id TEXT NOT NULL REFERENCES transmissions(id),
  replay_root_transmission_id TEXT NOT NULL REFERENCES transmissions(id),
  replay_depth INTEGER NOT NULL CHECK(replay_depth BETWEEN 1 AND 8),
  created_at INTEGER NOT NULL CHECK(created_at > 0)
);
CREATE INDEX IF NOT EXISTS transmission_replay_lineage_root
  ON transmission_replay_lineage(
    replay_root_transmission_id, replay_depth, created_at, transmission_id
  );

-- Inbox cursors mirror the Phase 1 history capability design.  Only a digest
-- is durable and every page boundary remains bound to one actor credential,
-- current installation generation, view and limit.
CREATE TABLE IF NOT EXISTS transmission_inbox_cursors (
  token_hash TEXT PRIMARY KEY
    CHECK(length(token_hash) = 64 AND token_hash NOT GLOB '*[^0-9a-f]*'),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  authorization_hash TEXT NOT NULL
    CHECK(length(authorization_hash) = 64
      AND authorization_hash NOT GLOB '*[^0-9a-f]*'),
  binding_paired_at INTEGER NOT NULL CHECK(binding_paired_at >= 0),
  view TEXT NOT NULL CHECK(view IN ('all', 'available', 'dismissed')),
  page_limit INTEGER NOT NULL CHECK(page_limit BETWEEN 1 AND 100),
  upper_at INTEGER NOT NULL CHECK(upper_at > 0),
  upper_id TEXT NOT NULL CHECK(length(upper_id) = 29 AND substr(upper_id, 1, 3) = 'ib_'),
  last_at INTEGER NOT NULL CHECK(last_at > 0),
  last_id TEXT NOT NULL CHECK(length(last_id) = 29 AND substr(last_id, 1, 3) = 'ib_'),
  expires_at INTEGER NOT NULL CHECK(expires_at > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0)
);
CREATE INDEX IF NOT EXISTS transmission_inbox_cursors_expiry
  ON transmission_inbox_cursors(expires_at, actor_id);

-- Receipt cursors keep the immutable target ordering server-side. The client
-- receives only a random capability and therefore cannot recover a
-- transmission, actor, orbit, slot, or binding generation from pagination.
CREATE TABLE IF NOT EXISTS transmission_receipt_cursors (
  token_hash TEXT PRIMARY KEY
    CHECK(length(token_hash) = 64 AND token_hash NOT GLOB '*[^0-9a-f]*'),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  authorization_hash TEXT NOT NULL
    CHECK(length(authorization_hash) = 64
      AND authorization_hash NOT GLOB '*[^0-9a-f]*'),
  history_item_id TEXT NOT NULL
    CHECK(length(history_item_id) = 29
      AND substr(history_item_id, 1, 3) = 'hi_'),
  page_limit INTEGER NOT NULL CHECK(page_limit BETWEEN 1 AND 100),
  last_orbit_id INTEGER NOT NULL CHECK(last_orbit_id > 0),
  last_actor_id INTEGER NOT NULL CHECK(last_actor_id > 0),
  last_slot TEXT NOT NULL CHECK(length(last_slot) = 1 AND last_slot GLOB '[a-z]'),
  last_binding_paired_at INTEGER NOT NULL CHECK(last_binding_paired_at >= 0),
  expires_at INTEGER NOT NULL CHECK(expires_at > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0)
);
CREATE INDEX IF NOT EXISTS transmission_receipt_cursors_expiry
  ON transmission_receipt_cursors(expires_at, actor_id);

-- Scheduler timestamps live outside the immutable acceptance snapshot.  This
-- keeps the domain FIFO and barrier restart-safe while allowing a previous
-- coordinator binary to ignore the additive runtime state during rollback.
CREATE TABLE IF NOT EXISTS transmission_scheduler_state (
  transmission_id TEXT PRIMARY KEY
    REFERENCES transmissions(id) ON DELETE CASCADE,
  barrier_opened_at INTEGER NOT NULL DEFAULT 0 CHECK(barrier_opened_at >= 0),
  prepare_deadline_at INTEGER NOT NULL DEFAULT 0 CHECK(prepare_deadline_at >= 0),
  decision_at INTEGER NOT NULL DEFAULT 0 CHECK(decision_at >= 0),
  t_coord_ms INTEGER NOT NULL DEFAULT 0 CHECK(t_coord_ms >= 0),
  start_deadline_coord_ms INTEGER NOT NULL DEFAULT 0
    CHECK(start_deadline_coord_ms >= 0),
  legacy_element_id TEXT NOT NULL DEFAULT '' CHECK(length(legacy_element_id) <= 64),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at > 0),
  CHECK((barrier_opened_at = 0 AND prepare_deadline_at = 0)
    OR (barrier_opened_at > 0 AND prepare_deadline_at >= barrier_opened_at)),
  CHECK((t_coord_ms = 0 AND start_deadline_coord_ms = 0)
    OR (decision_at > 0 AND t_coord_ms >= decision_at
      AND start_deadline_coord_ms = t_coord_ms + 100))
);
CREATE INDEX IF NOT EXISTS transmission_scheduler_due
  ON transmission_scheduler_state(
    prepare_deadline_at, start_deadline_coord_ms, updated_at, transmission_id
  );

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
  OR NEW.air_id <> OLD.air_id
  OR NEW.air_policy_revision <> OLD.air_policy_revision
  OR NEW.air_policy_operation <> OLD.air_policy_operation
  OR NEW.air_policy_result <> OLD.air_policy_result
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
  OR NEW.capability_set_hash <> OLD.capability_set_hash
  OR NEW.resolved_at_ms <> OLD.resolved_at_ms
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

-- Public policy surfaces never serialize the internal integer ids above.
-- History mints actor-bound subject references; blocks receive their own
-- opaque ids.  Keeping both mappings additive preserves rollback compatibility.
CREATE TABLE IF NOT EXISTS transmission_subject_refs (
  public_id TEXT PRIMARY KEY
    CHECK((length(public_id) = 29 AND substr(public_id, 1, 3) = 'ar_')
      OR (length(public_id) = 29 AND substr(public_id, 1, 3) = 'or_')),
  viewer_actor_id INTEGER NOT NULL CHECK(viewer_actor_id > 0),
  subject_kind TEXT NOT NULL CHECK(subject_kind IN ('actor', 'orbit')),
  subject_id INTEGER NOT NULL CHECK(subject_id > 0),
  display_name TEXT NOT NULL CHECK(length(display_name) BETWEEN 1 AND 480),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  expires_at INTEGER NOT NULL CHECK(expires_at > created_at)
);
CREATE INDEX IF NOT EXISTS transmission_subject_refs_viewer
  ON transmission_subject_refs(viewer_actor_id, expires_at, public_id);

CREATE TABLE IF NOT EXISTS transmission_block_public_ids (
  block_id INTEGER PRIMARY KEY REFERENCES blocks(id) ON DELETE CASCADE,
  public_id TEXT NOT NULL UNIQUE
    CHECK(length(public_id) = 29 AND substr(public_id, 1, 3) = 'bl_'),
  subject_ref TEXT NOT NULL REFERENCES transmission_subject_refs(public_id)
);

-- Idempotency keys and request bodies are retained only as SHA-256 digests.
-- A response revision/public id is enough to reconstruct an exact replay.
CREATE TABLE IF NOT EXISTS transmission_policy_requests (
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  operation TEXT NOT NULL CHECK(operation IN (
    'dnd_local', 'dnd_orbit', 'block_create', 'block_delete'
  )),
  idempotency_key_hash TEXT NOT NULL
    CHECK(length(idempotency_key_hash) = 64
      AND idempotency_key_hash NOT GLOB '*[^0-9a-f]*'),
  request_hash TEXT NOT NULL
    CHECK(length(request_hash) = 64
      AND request_hash NOT GLOB '*[^0-9a-f]*'),
  resource_id TEXT NOT NULL DEFAULT '',
  resource_revision INTEGER NOT NULL DEFAULT 0 CHECK(resource_revision >= 0),
  response_json TEXT NOT NULL DEFAULT '' CHECK(length(response_json) <= 4096),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  PRIMARY KEY(actor_id, operation, idempotency_key_hash)
);

-- History cursors are random capabilities; only their digest and server-side
-- pagination state are durable. They carry no readable tenant or media ids.
CREATE TABLE IF NOT EXISTS transmission_history_cursors (
  token_hash TEXT PRIMARY KEY
    CHECK(length(token_hash) = 64 AND token_hash NOT GLOB '*[^0-9a-f]*'),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  authorization_hash TEXT NOT NULL
    CHECK(length(authorization_hash) = 64 AND authorization_hash NOT GLOB '*[^0-9a-f]*'),
  view TEXT NOT NULL CHECK(view IN ('all', 'sent', 'received')),
  page_limit INTEGER NOT NULL CHECK(page_limit BETWEEN 1 AND 100),
  upper_at INTEGER NOT NULL CHECK(upper_at > 0),
  upper_id TEXT NOT NULL CHECK(length(upper_id) = 29 AND substr(upper_id, 1, 3) = 'hi_'),
  last_at INTEGER NOT NULL CHECK(last_at > 0),
  last_id TEXT NOT NULL CHECK(length(last_id) = 29 AND substr(last_id, 1, 3) = 'hi_'),
  expires_at INTEGER NOT NULL CHECK(expires_at > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0)
);
CREATE INDEX IF NOT EXISTS transmission_history_cursors_expiry
  ON transmission_history_cursors(expires_at, actor_id);

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

-- Telegram media routing is a durable, server-side choice.  The default
-- after_current transmission is recorded here before any inline keyboard is
-- rendered, so a coordinator restart never creates a callback grace period.
CREATE TABLE IF NOT EXISTS telegram_inline_routes (
  media_id TEXT PRIMARY KEY REFERENCES media_items(id),
  media_generation INTEGER NOT NULL CHECK(media_generation > 0),
  source_actor_id INTEGER NOT NULL CHECK(source_actor_id > 0),
  source_orbit_id INTEGER NOT NULL CHECK(source_orbit_id > 0),
  original_update_id INTEGER NOT NULL CHECK(original_update_id > 0),
  attachment_kind TEXT NOT NULL CHECK(attachment_kind IN ('voice', 'audio', 'document')),
  default_transmission_id TEXT NOT NULL DEFAULT '' CHECK(
    default_transmission_id = '' OR
    (length(default_transmission_id) = 29 AND substr(default_transmission_id, 1, 3) = 'tr_')
  ),
  selected_transmission_id TEXT NOT NULL DEFAULT '' CHECK(
    selected_transmission_id = '' OR
    (length(selected_transmission_id) = 29 AND substr(selected_transmission_id, 1, 3) = 'tr_')
  ),
  state TEXT NOT NULL DEFAULT 'pending' CHECK(state IN ('pending', 'selected', 'dismissed')),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  updated_at INTEGER NOT NULL CHECK(updated_at >= created_at),
  CHECK(attachment_kind = 'voice' OR default_transmission_id = ''),
  CHECK((state = 'selected' AND selected_transmission_id <> '')
    OR (state <> 'selected' AND selected_transmission_id = ''))
);

-- callback_data contains only tg1_<random>.  The HMAC digest and every
-- security binding remain server-side; no actor, orbit, media or action id is
-- serialized into Telegram-visible data.
CREATE TABLE IF NOT EXISTS telegram_inline_callbacks (
  token_hash TEXT PRIMARY KEY CHECK(
    length(token_hash) = 64 AND token_hash NOT GLOB '*[^0-9a-f]*'
  ),
  media_id TEXT NOT NULL REFERENCES telegram_inline_routes(media_id),
  media_generation INTEGER NOT NULL CHECK(media_generation > 0),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  orbit_id INTEGER NOT NULL CHECK(orbit_id > 0),
  authorization TEXT NOT NULL CHECK(authorization IN ('initiator_only', 'source_primary')),
  chat_id INTEGER NOT NULL,
  message_id INTEGER NOT NULL CHECK(message_id > 0),
  original_update_id INTEGER NOT NULL CHECK(original_update_id > 0),
  action TEXT NOT NULL CHECK(action IN (
    'choose_overlay', 'choose_interrupt', 'choose_after_current',
    'choose_own_barycenter', 'choose_current_air',
    'confirm_overlay', 'confirm_after_current', 'dismiss'
  )),
  delivery TEXT NOT NULL DEFAULT '' CHECK(delivery IN ('', 'overlay', 'interrupt', 'after_current')),
  audience TEXT NOT NULL DEFAULT '' CHECK(audience IN ('', 'own_barycenter', 'current_air')),
  confirmation_token_hash TEXT NOT NULL DEFAULT '' CHECK(
    confirmation_token_hash = '' OR
    (length(confirmation_token_hash) = 64
      AND confirmation_token_hash NOT GLOB '*[^0-9a-f]*')
  ),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  expires_at INTEGER NOT NULL CHECK(expires_at > created_at),
  consumed_at INTEGER NOT NULL DEFAULT 0 CHECK(consumed_at >= 0),
  outcome TEXT NOT NULL DEFAULT '' CHECK(outcome IN (
    '', 'applied', 'already_applied', 'requires_confirmation', 'too_late',
    'expired', 'forbidden', 'unsupported', 'failed'
  )),
  CHECK(consumed_at = 0 OR consumed_at >= created_at)
);
CREATE INDEX IF NOT EXISTS telegram_inline_callbacks_route
  ON telegram_inline_callbacks(media_id, media_generation, expires_at);

CREATE TABLE IF NOT EXISTS telegram_inline_callback_queries (
  query_hash TEXT PRIMARY KEY CHECK(
    length(query_hash) = 64 AND query_hash NOT GLOB '*[^0-9a-f]*'
  ),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  orbit_id INTEGER NOT NULL CHECK(orbit_id > 0),
  chat_id INTEGER NOT NULL,
  message_id INTEGER NOT NULL CHECK(message_id > 0),
  outcome TEXT NOT NULL CHECK(outcome IN (
    'applied', 'already_applied', 'requires_confirmation', 'too_late',
    'expired', 'forbidden', 'unsupported', 'failed'
  )),
  clear_keyboard INTEGER NOT NULL CHECK(clear_keyboard IN (0, 1)),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  expires_at INTEGER NOT NULL CHECK(expires_at > created_at)
);
CREATE INDEX IF NOT EXISTS telegram_inline_callback_queries_expiry
  ON telegram_inline_callback_queries(expires_at, actor_id);

-- History moderation callbacks share the opaque tg1_ transport but are kept
-- separate from delivery-route callbacks: foreign history media does not own
-- a telegram_inline_routes row. Every authority and action value remains
-- server-side and is rechecked by the canonical history action service.
CREATE TABLE IF NOT EXISTS telegram_history_callbacks (
  token_hash TEXT PRIMARY KEY CHECK(
    length(token_hash) = 64 AND token_hash NOT GLOB '*[^0-9a-f]*'
  ),
  history_item_id TEXT NOT NULL CHECK(
    length(history_item_id) = 29 AND substr(history_item_id, 1, 3) = 'hi_'
  ),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  orbit_id INTEGER NOT NULL CHECK(orbit_id > 0),
  role TEXT NOT NULL CHECK(role IN ('primary', 'companion', 'satellite')),
  chat_id INTEGER NOT NULL,
  message_id INTEGER NOT NULL CHECK(message_id > 0),
  action TEXT NOT NULL CHECK(action IN ('replay', 'delete', 'report', 'block_actor')),
  reason TEXT NOT NULL DEFAULT '' CHECK(reason IN (
    '', 'spam', 'harassment', 'illegal', 'sexual_content', 'violence', 'other'
  )),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  expires_at INTEGER NOT NULL CHECK(expires_at > created_at),
  consumed_at INTEGER NOT NULL DEFAULT 0 CHECK(consumed_at >= 0),
  callback_outcome TEXT NOT NULL DEFAULT '' CHECK(callback_outcome IN (
    '', 'applied', 'already_applied', 'too_late', 'expired', 'forbidden',
    'unsupported', 'failed'
  )),
  action_outcome TEXT NOT NULL DEFAULT '' CHECK(action_outcome IN (
    '', 'media_deleted', 'report_received', 'report_already_received',
    'sender_blocked', 'sender_already_blocked', 'replay_accepted',
    'replay_already_accepted', 'history_action_unavailable'
  )),
  CHECK((action = 'report' AND reason <> '') OR (action <> 'report' AND reason = '')),
  CHECK(consumed_at = 0 OR consumed_at >= created_at)
);
CREATE INDEX IF NOT EXISTS telegram_history_callbacks_expiry
  ON telegram_history_callbacks(actor_id, expires_at);

CREATE TABLE IF NOT EXISTS telegram_history_callback_queries (
  query_hash TEXT PRIMARY KEY CHECK(
    length(query_hash) = 64 AND query_hash NOT GLOB '*[^0-9a-f]*'
  ),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  orbit_id INTEGER NOT NULL CHECK(orbit_id > 0),
  role TEXT NOT NULL CHECK(role IN ('primary', 'companion', 'satellite')),
  chat_id INTEGER NOT NULL,
  message_id INTEGER NOT NULL CHECK(message_id > 0),
  callback_outcome TEXT NOT NULL CHECK(callback_outcome IN (
    'applied', 'already_applied', 'too_late', 'expired', 'forbidden',
    'unsupported', 'failed'
  )),
  action_outcome TEXT NOT NULL DEFAULT '' CHECK(action_outcome IN (
    '', 'media_deleted', 'report_received', 'report_already_received',
    'sender_blocked', 'sender_already_blocked', 'replay_accepted',
    'replay_already_accepted', 'history_action_unavailable'
  )),
  clear_keyboard INTEGER NOT NULL CHECK(clear_keyboard IN (0, 1)),
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  expires_at INTEGER NOT NULL CHECK(expires_at > created_at)
);
CREATE INDEX IF NOT EXISTS telegram_history_callback_queries_expiry
  ON telegram_history_callback_queries(expires_at, actor_id);
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
	// These acceptance fields were introduced after the transmission table.
	// Defaults keep the immediately preceding coordinator rollback-compatible.
	for _, column := range []struct {
		table string
		name  string
		ddl   string
	}{
		{"transmissions", "air_id", `ALTER TABLE transmissions ADD COLUMN air_id TEXT NOT NULL DEFAULT ''`},
		{"transmissions", "air_policy_revision", `ALTER TABLE transmissions ADD COLUMN air_policy_revision INTEGER NOT NULL DEFAULT 0 CHECK(air_policy_revision >= 0)`},
		{"transmissions", "air_policy_operation", `ALTER TABLE transmissions ADD COLUMN air_policy_operation TEXT NOT NULL DEFAULT '' CHECK(air_policy_operation IN ('', 'overlay', 'queue', 'replace'))`},
		{"transmissions", "air_policy_result", `ALTER TABLE transmissions ADD COLUMN air_policy_result TEXT NOT NULL DEFAULT '' CHECK(air_policy_result IN ('', 'allowed'))`},
		{"transmission_targets", "capability_set_hash", `ALTER TABLE transmission_targets ADD COLUMN capability_set_hash TEXT NOT NULL DEFAULT 'e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855' CHECK(length(capability_set_hash) = 64 AND capability_set_hash NOT GLOB '*[^0-9a-f]*')`},
		{"transmission_targets", "resolved_at_ms", `ALTER TABLE transmission_targets ADD COLUMN resolved_at_ms INTEGER NOT NULL DEFAULT 0 CHECK(resolved_at_ms >= 0)`},
	} {
		exists, err := txColumnExists(tx, column.table, column.name)
		if err != nil {
			return err
		}
		if !exists {
			if _, err := tx.Exec(column.ddl); err != nil {
				return err
			}
		}
	}
	// A previous binary can insert rollback-era rows using the defaults above.
	// Drop either generation of the immutable trigger before reconciling those
	// rows, then reinstall the current trigger after the snapshot is complete.
	if _, err := tx.Exec(`DROP TRIGGER IF EXISTS transmission_targets_snapshot_immutable`); err != nil {
		return err
	}
	// Upgrade rows freeze the only capability and resolution evidence the
	// previous schema retained.  Hashes use the same canonical capability list
	// as fresh acceptance; accepted_at is the exact safe resolution boundary.
	for mediaClip := 0; mediaClip <= 1; mediaClip++ {
		for overlay := 0; overlay <= 1; overlay++ {
			for interrupt := 0; interrupt <= 1; interrupt++ {
				for resume := 0; resume <= 1; resume++ {
					capabilityHash := transmissionTargetCapabilityHash(
						mediaClip != 0, overlay != 0, interrupt != 0, resume != 0,
					)
					if _, err := tx.Exec(`UPDATE transmission_targets
SET capability_set_hash = ?
WHERE resolved_at_ms = 0 AND media_clip_capable = ?
  AND overlay_capable = ? AND interrupt_capable = ?
  AND interrupt_resume_ready = ?`, capabilityHash, mediaClip, overlay,
						interrupt, resume); err != nil {
						return err
					}
				}
			}
		}
	}
	if _, err := tx.Exec(`UPDATE transmission_targets
SET resolved_at_ms = (
  SELECT accepted_at FROM transmissions
  WHERE transmissions.id = transmission_targets.transmission_id
)
WHERE resolved_at_ms = 0`); err != nil {
		return err
	}
	// Existing databases already have the trigger created by the prior schema;
	// replace it so the additive policy snapshot is immutable there too.
	if _, err := tx.Exec(`DROP TRIGGER IF EXISTS transmissions_acceptance_immutable`); err != nil {
		return err
	}
	if _, err := tx.Exec(transmissionAcceptanceImmutableTrigger); err != nil {
		return err
	}
	if _, err := tx.Exec(transmissionTargetSnapshotImmutableTrigger); err != nil {
		return err
	}
	// Rows accepted by the immediately preceding schema did not yet have the
	// additive scheduler companion. Their immutable acceptance time is the only
	// safe backfill timestamp; no barrier or schedule is invented here.
	if _, err := tx.Exec(`INSERT OR IGNORE INTO transmission_scheduler_state(
  transmission_id, updated_at
) SELECT id, accepted_at FROM transmissions`); err != nil {
		return err
	}
	if err := backfillTransmissionInboxItemsTx(tx); err != nil {
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

const transmissionAcceptanceImmutableTrigger = `
CREATE TRIGGER transmissions_acceptance_immutable
BEFORE UPDATE ON transmissions
WHEN NEW.id <> OLD.id
  OR NEW.media_id <> OLD.media_id
  OR NEW.source_orbit_id <> OLD.source_orbit_id
  OR NEW.source_actor_id <> OLD.source_actor_id
  OR NEW.source_slot <> OLD.source_slot
  OR NEW.playback_domain_kind <> OLD.playback_domain_kind
  OR NEW.playback_domain_id <> OLD.playback_domain_id
  OR NEW.air_id <> OLD.air_id
  OR NEW.air_policy_revision <> OLD.air_policy_revision
  OR NEW.air_policy_operation <> OLD.air_policy_operation
  OR NEW.air_policy_result <> OLD.air_policy_result
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
`

const transmissionTargetSnapshotImmutableTrigger = `
CREATE TRIGGER transmission_targets_snapshot_immutable
BEFORE UPDATE ON transmission_targets
WHEN NEW.transmission_id <> OLD.transmission_id
  OR NEW.orbit_id <> OLD.orbit_id
  OR NEW.actor_id <> OLD.actor_id
  OR NEW.slot <> OLD.slot
  OR NEW.binding_paired_at <> OLD.binding_paired_at
  OR NEW.capability_set_hash <> OLD.capability_set_hash
  OR NEW.resolved_at_ms <> OLD.resolved_at_ms
  OR NEW.online_at_acceptance <> OLD.online_at_acceptance
  OR NEW.media_clip_capable <> OLD.media_clip_capable
  OR NEW.overlay_capable <> OLD.overlay_capable
  OR NEW.interrupt_capable <> OLD.interrupt_capable
  OR NEW.interrupt_resume_ready <> OLD.interrupt_resume_ready
BEGIN
  SELECT RAISE(ABORT, 'transmission target snapshot is immutable');
END;
`
