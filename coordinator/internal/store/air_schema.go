package store

import (
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"relux.works/duet/coordinator/internal/ulid"
)

const airSchema = `
CREATE TABLE IF NOT EXISTS airs (
  public_id TEXT PRIMARY KEY
    CHECK(length(public_id) = 30 AND substr(public_id, 1, 4) = 'air_'),
  title TEXT NOT NULL,
  status TEXT NOT NULL CHECK(status IN ('parked', 'active', 'dissolved')),
  owner_orbit_id INTEGER NOT NULL CHECK(owner_orbit_id > 0),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  created_at INTEGER NOT NULL,
  dissolved_at INTEGER,
  CHECK((status = 'dissolved') = (dissolved_at IS NOT NULL))
);

CREATE TABLE IF NOT EXISTS air_members (
  public_id TEXT PRIMARY KEY
    CHECK(length(public_id) = 30 AND substr(public_id, 1, 4) = 'aim_'),
  air_id TEXT NOT NULL REFERENCES airs(public_id),
  orbit_id INTEGER NOT NULL CHECK(orbit_id > 0),
  air_role TEXT NOT NULL CHECK(air_role IN ('owner', 'admin', 'member')),
  status TEXT NOT NULL CHECK(status IN ('pending_confirmation', 'joined', 'left')),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  joined_at INTEGER,
  left_at INTEGER,
  created_at INTEGER NOT NULL,
  CHECK((status = 'pending_confirmation' AND joined_at IS NULL AND left_at IS NULL)
     OR (status = 'joined' AND joined_at IS NOT NULL AND left_at IS NULL)
     OR (status = 'left' AND left_at IS NOT NULL))
);
CREATE UNIQUE INDEX IF NOT EXISTS air_members_one_live
  ON air_members(air_id, orbit_id)
  WHERE status IN ('pending_confirmation', 'joined');
CREATE UNIQUE INDEX IF NOT EXISTS air_members_one_owner
  ON air_members(air_id)
  WHERE air_role = 'owner' AND status = 'joined';
CREATE INDEX IF NOT EXISTS air_members_saved
  ON air_members(orbit_id, status, air_id);

CREATE TABLE IF NOT EXISTS air_active_pointers (
  orbit_id INTEGER PRIMARY KEY CHECK(orbit_id > 0),
  air_id TEXT NOT NULL REFERENCES airs(public_id),
  revision INTEGER NOT NULL CHECK(revision > 0),
  activated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS air_active_pointers_air
  ON air_active_pointers(air_id, orbit_id);

CREATE TABLE IF NOT EXISTS air_invites (
  public_id TEXT PRIMARY KEY
    CHECK(length(public_id) = 29 AND substr(public_id, 1, 3) = 'ai_'),
  air_id TEXT NOT NULL REFERENCES airs(public_id),
  code_hash TEXT NOT NULL UNIQUE
    CHECK(length(code_hash) = 64 AND code_hash NOT GLOB '*[^0-9a-f]*'),
  status TEXT NOT NULL CHECK(status IN ('open', 'consumed', 'expired', 'withdrawn')),
  intended_role TEXT NOT NULL CHECK(intended_role IN ('admin', 'member')),
  issued_by_actor_id INTEGER NOT NULL CHECK(issued_by_actor_id > 0),
  issued_by_orbit_id INTEGER NOT NULL CHECK(issued_by_orbit_id > 0),
  policy_revision INTEGER NOT NULL CHECK(policy_revision > 0),
  revision INTEGER NOT NULL DEFAULT 1 CHECK(revision > 0),
  expires_at INTEGER NOT NULL,
  consumed_membership_id TEXT REFERENCES air_members(public_id),
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  CHECK((status = 'consumed') = (consumed_membership_id IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS air_invites_air_status
  ON air_invites(air_id, status, expires_at);

CREATE TABLE IF NOT EXISTS air_policies (
  air_id TEXT PRIMARY KEY REFERENCES airs(public_id),
  revision INTEGER NOT NULL CHECK(revision > 0),
  invite_policy TEXT NOT NULL
    CHECK(invite_policy IN ('owner_primary', 'air_admin_primary', 'all_member_primaries')),
  overlay_policy TEXT NOT NULL
    CHECK(overlay_policy IN ('air_admin_primary', 'all_member_primaries', 'primary_companion', 'disabled')),
  queue_policy TEXT NOT NULL
    CHECK(queue_policy IN ('air_admin_primary', 'all_member_primaries', 'primary_companion', 'disabled')),
  replace_policy TEXT NOT NULL
    CHECK(replace_policy IN ('owner_primary', 'air_admin_primary', 'all_member_primaries', 'disabled')),
  updated_at INTEGER NOT NULL
);

-- Generic transmission scheduling still uses a compact integer domain key.
-- This map gives every Air one stable key without exposing that implementation
-- detail in the Air API or overloading a legacy link id.
CREATE TABLE IF NOT EXISTS air_playback_domains (
  id INTEGER PRIMARY KEY,
  air_id TEXT NOT NULL UNIQUE REFERENCES airs(public_id)
);

CREATE TABLE IF NOT EXISTS air_audit_events (
  id INTEGER PRIMARY KEY,
  air_id TEXT NOT NULL,
  membership_id TEXT NOT NULL DEFAULT '',
  invite_id TEXT NOT NULL DEFAULT '',
  actor_id INTEGER NOT NULL DEFAULT 0,
  orbit_id INTEGER NOT NULL DEFAULT 0,
  operation TEXT NOT NULL,
  old_value TEXT NOT NULL DEFAULT '',
  new_value TEXT NOT NULL DEFAULT '',
  authority_generation INTEGER NOT NULL CHECK(authority_generation > 0),
  result_code TEXT NOT NULL,
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS air_audit_events_air_created
  ON air_audit_events(air_id, created_at, id);

-- Air mutation retries are scoped to the authenticated actor. Only request
-- and key digests plus the non-secret response projection are durable. Invite
-- codes are deterministically derived from a coordinator HMAC key and are
-- therefore never written here in plaintext.
CREATE TABLE IF NOT EXISTS air_mutation_results (
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  idempotency_key_hash TEXT NOT NULL
    CHECK(length(idempotency_key_hash) = 64
      AND idempotency_key_hash NOT GLOB '*[^0-9a-f]*'),
  operation TEXT NOT NULL,
  request_hash TEXT NOT NULL
    CHECK(length(request_hash) = 64 AND request_hash NOT GLOB '*[^0-9a-f]*'),
  response_json TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  PRIMARY KEY(actor_id, idempotency_key_hash)
);

-- Telegram Air controls carry only an opaque random capability. Air ids,
-- revisions, actor/role authority and the requested action stay server-side.
-- Tokens are single-use and additionally fenced by Telegram query id.
CREATE TABLE IF NOT EXISTS telegram_air_callbacks (
  token_hash TEXT PRIMARY KEY CHECK(
    length(token_hash) = 64 AND token_hash NOT GLOB '*[^0-9a-f]*'
  ),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  orbit_id INTEGER NOT NULL CHECK(orbit_id > 0),
  role TEXT NOT NULL CHECK(role IN ('primary', 'companion', 'satellite')),
  chat_id INTEGER NOT NULL,
  message_id INTEGER NOT NULL CHECK(message_id > 0),
  action TEXT NOT NULL CHECK(action IN (
    'activate', 'deactivate', 'confirm_join', 'confirm_join_activate',
    'decline_join', 'leave', 'dissolve', 'issue_member', 'issue_admin',
    'withdraw_invite', 'policy_next'
  )),
  air_id TEXT NOT NULL REFERENCES airs(public_id),
  membership_id TEXT NOT NULL DEFAULT '',
  invite_id TEXT NOT NULL DEFAULT '',
  air_revision INTEGER NOT NULL DEFAULT 0 CHECK(air_revision >= 0),
  membership_revision INTEGER NOT NULL DEFAULT 0 CHECK(membership_revision >= 0),
  invite_revision INTEGER NOT NULL DEFAULT 0 CHECK(invite_revision >= 0),
  expected_active_air_id TEXT NOT NULL DEFAULT '',
  policy_revision INTEGER NOT NULL DEFAULT 0 CHECK(policy_revision >= 0),
  invite_policy TEXT NOT NULL DEFAULT '',
  overlay_policy TEXT NOT NULL DEFAULT '',
  queue_policy TEXT NOT NULL DEFAULT '',
  replace_policy TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  expires_at INTEGER NOT NULL CHECK(expires_at > created_at),
  consumed_at INTEGER NOT NULL DEFAULT 0 CHECK(consumed_at >= 0),
  outcome TEXT NOT NULL DEFAULT '' CHECK(outcome IN (
    '', 'claimed', 'applied', 'already_applied', 'too_late', 'expired',
    'forbidden', 'unsupported', 'failed'
  )),
  CHECK(consumed_at = 0 OR consumed_at >= created_at)
);
CREATE INDEX IF NOT EXISTS telegram_air_callbacks_expiry
  ON telegram_air_callbacks(expires_at, actor_id);

CREATE TABLE IF NOT EXISTS telegram_air_callback_queries (
  query_hash TEXT PRIMARY KEY CHECK(
    length(query_hash) = 64 AND query_hash NOT GLOB '*[^0-9a-f]*'
  ),
  actor_id INTEGER NOT NULL CHECK(actor_id > 0),
  orbit_id INTEGER NOT NULL CHECK(orbit_id > 0),
  role TEXT NOT NULL,
  chat_id INTEGER NOT NULL,
  message_id INTEGER NOT NULL CHECK(message_id > 0),
  outcome TEXT NOT NULL,
  created_at INTEGER NOT NULL CHECK(created_at > 0),
  expires_at INTEGER NOT NULL CHECK(expires_at > created_at)
);

-- /approach is a compatibility facade over the Air lifecycle. The facade
-- keeps only durable references; the user-facing code is derived from the
-- invite id and the Air HMAC key, so it is recoverable after restart without
-- ever being stored in plaintext.
CREATE TABLE IF NOT EXISTS air_approach_aliases (
  air_id TEXT PRIMARY KEY REFERENCES airs(public_id),
  invite_id TEXT NOT NULL UNIQUE REFERENCES air_invites(public_id),
  issuer_actor_id INTEGER NOT NULL CHECK(issuer_actor_id > 0),
  issuer_orbit_id INTEGER NOT NULL CHECK(issuer_orbit_id > 0),
  claimant_orbit_id INTEGER CHECK(claimant_orbit_id > 0),
  membership_id TEXT UNIQUE REFERENCES air_members(public_id),
  status TEXT NOT NULL CHECK(status IN ('open', 'pending_confirmation', 'joined', 'closed')),
  created_at INTEGER NOT NULL,
  updated_at INTEGER NOT NULL,
  CHECK((status = 'open' AND claimant_orbit_id IS NULL AND membership_id IS NULL)
     OR (status IN ('pending_confirmation', 'joined') AND claimant_orbit_id IS NOT NULL AND membership_id IS NOT NULL)
     OR status = 'closed')
);
CREATE INDEX IF NOT EXISTS air_approach_aliases_issuer_status
  ON air_approach_aliases(issuer_orbit_id, status, updated_at);
CREATE INDEX IF NOT EXISTS air_approach_aliases_claimant_status
  ON air_approach_aliases(claimant_orbit_id, status, updated_at);

CREATE TABLE IF NOT EXISTS air_legacy_link_mappings (
  link_id INTEGER PRIMARY KEY CHECK(link_id > 0),
  air_id TEXT NOT NULL UNIQUE REFERENCES airs(public_id),
  orbit_a INTEGER NOT NULL CHECK(orbit_a > 0),
  orbit_b INTEGER NOT NULL CHECK(orbit_b > 0 AND orbit_b <> orbit_a),
  link_created_at INTEGER NOT NULL,
  backfilled_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS air_legacy_runtime_snapshots (
  authority_generation INTEGER NOT NULL CHECK(authority_generation > 0),
  link_id INTEGER NOT NULL CHECK(link_id > 0),
  air_id TEXT NOT NULL REFERENCES airs(public_id),
  orbit_a INTEGER NOT NULL CHECK(orbit_a > 0),
  orbit_b INTEGER NOT NULL CHECK(orbit_b > 0 AND orbit_b <> orbit_a),
  link_created_at INTEGER NOT NULL,
  PRIMARY KEY (authority_generation, link_id)
);

CREATE TABLE IF NOT EXISTS air_authority (
  singleton INTEGER PRIMARY KEY CHECK(singleton = 1),
  mode TEXT NOT NULL
    CHECK(mode IN ('links_authoritative', 'airs_shadow', 'airs_authoritative', 'rollback_hold')),
  generation INTEGER NOT NULL CHECK(generation > 0),
  divergence_count INTEGER NOT NULL DEFAULT 0 CHECK(divergence_count >= 0),
  updated_at INTEGER NOT NULL
);
`

func (s *Store) initAirSchema() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(airSchema); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	if _, err := tx.Exec(`INSERT INTO air_authority(
  singleton, mode, generation, divergence_count, updated_at
) VALUES(1, 'links_authoritative', 1, 0, ?)
ON CONFLICT(singleton) DO NOTHING`, now); err != nil {
		return err
	}
	if err := s.checkpoint("air_schema_after_ddl"); err != nil {
		return err
	}
	authorityBefore, err := airAuthorityTx(tx)
	if err != nil {
		return err
	}
	if authorityBefore.Mode == "airs_authoritative" {
		legacyChanged, err := airLegacySnapshotChangedTx(tx, authorityBefore.Generation)
		if err != nil {
			return err
		}
		if legacyChanged {
			if _, err := tx.Exec(`UPDATE air_authority SET mode = 'rollback_hold',
  generation = generation + 1, updated_at = ? WHERE singleton = 1`, now); err != nil {
				return err
			}
			if err := appendAirAuditTx(tx, "", "", "", 0, 0,
				"air.authority.startup_validation", "airs_authoritative", "rollback_hold",
				"rollback_unsafe", now); err != nil {
				return err
			}
			if err := tx.Commit(); err != nil {
				return err
			}
			return fmt.Errorf("%w: legacy links changed after Air cutover", ErrAirRollbackUnsafe)
		}
	}
	backfilled, err := s.backfillActiveLinksTx(tx, now)
	if err != nil {
		return err
	}
	if authorityBefore.Mode == "airs_authoritative" && backfilled > 0 {
		return fmt.Errorf("%w: authoritative link mapping was missing", ErrAirRollbackUnsafe)
	}
	// A migrated pair keeps its legacy numeric scheduler domain so work
	// accepted immediately before and after cutover remains in one FIFO.
	if _, err := tx.Exec(`INSERT OR IGNORE INTO air_playback_domains(id, air_id)
SELECT link_id, air_id FROM air_legacy_link_mappings ORDER BY link_id`); err != nil {
		return err
	}
	if backfilled > 0 {
		if _, err := tx.Exec(`UPDATE air_authority
SET mode = CASE WHEN mode = 'links_authoritative' THEN 'airs_shadow' ELSE mode END,
    updated_at = ?
WHERE singleton = 1`, now); err != nil {
			return err
		}
	}
	if err := s.checkpoint("air_backfill_before_commit"); err != nil {
		return err
	}
	return tx.Commit()
}

func airLegacySnapshotChangedTx(tx *sql.Tx, generation int64) (bool, error) {
	var snapshotRows, activeLinks, mismatches int
	if err := tx.QueryRow(`SELECT
  (SELECT COUNT(*) FROM air_legacy_runtime_snapshots WHERE authority_generation = ?),
  (SELECT COUNT(*) FROM links WHERE state = 'active'),
  (SELECT COUNT(*) FROM air_legacy_runtime_snapshots s
     LEFT JOIN links l ON l.id = s.link_id
       AND l.state = 'active'
       AND l.orbit_a = s.orbit_a
       AND l.orbit_b = s.orbit_b
       AND l.created_at = s.link_created_at
     WHERE s.authority_generation = ? AND l.id IS NULL)`, generation, generation).Scan(
		&snapshotRows, &activeLinks, &mismatches,
	); err != nil {
		return false, err
	}
	return snapshotRows != activeLinks || mismatches != 0, nil
}

func (s *Store) backfillActiveLinksTx(tx *sql.Tx, now int64) (int, error) {
	rows, err := tx.Query(`SELECT id, orbit_a, orbit_b, created_at
FROM links WHERE state = 'active' ORDER BY id`)
	if err != nil {
		return 0, err
	}
	type legacyLink struct{ id, a, b, createdAt int64 }
	var links []legacyLink
	seenOrbit := map[int64]int64{}
	for rows.Next() {
		var link legacyLink
		if err := rows.Scan(&link.id, &link.a, &link.b, &link.createdAt); err != nil {
			rows.Close()
			return 0, err
		}
		if link.a <= 0 || link.b <= 0 || link.a == link.b {
			rows.Close()
			return 0, fmt.Errorf("active link %d has invalid orbits %d/%d", link.id, link.a, link.b)
		}
		for _, orbitID := range []int64{link.a, link.b} {
			if prior := seenOrbit[orbitID]; prior != 0 {
				rows.Close()
				return 0, fmt.Errorf("orbit %d has active links %d and %d", orbitID, prior, link.id)
			}
			seenOrbit[orbitID] = link.id
		}
		links = append(links, link)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}

	backfilled := 0
	for i, link := range links {
		var mappedAir string
		err := tx.QueryRow(`SELECT air_id FROM air_legacy_link_mappings WHERE link_id = ?`, link.id).Scan(&mappedAir)
		if err == nil {
			if err := validateLegacyAirMappingTx(tx, link.id, mappedAir, link.a, link.b); err != nil {
				return 0, err
			}
			continue
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return 0, err
		}
		var titleA, titleB string
		if err := tx.QueryRow(`SELECT title FROM orbits WHERE id = ?`, link.a).Scan(&titleA); err != nil {
			return 0, fmt.Errorf("active link %d orbit_a: %w", link.id, err)
		}
		if err := tx.QueryRow(`SELECT title FROM orbits WHERE id = ?`, link.b).Scan(&titleB); err != nil {
			return 0, fmt.Errorf("active link %d orbit_b: %w", link.id, err)
		}
		airID := deterministicAirMigrationID("air", link)
		ownerMemberID := deterministicAirMigrationID("owner", link)
		peerMemberID := deterministicAirMigrationID("member", link)
		if _, err := tx.Exec(`INSERT INTO airs(
  public_id, title, status, owner_orbit_id, revision, created_at
) VALUES(?, ?, 'parked', ?, 1, ?)`, airID, titleA+" ⇄ "+titleB, link.a, link.createdAt); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(`INSERT INTO air_members(
  public_id, air_id, orbit_id, air_role, status, revision, joined_at, created_at
) VALUES
  (?, ?, ?, 'owner', 'joined', 1, ?, ?),
  (?, ?, ?, 'member', 'joined', 1, ?, ?)`,
			ownerMemberID, airID, link.a, link.createdAt, link.createdAt,
			peerMemberID, airID, link.b, link.createdAt, link.createdAt); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(`INSERT INTO air_policies(
  air_id, revision, invite_policy, overlay_policy, queue_policy, replace_policy, updated_at
) VALUES(?, 1, 'air_admin_primary', 'primary_companion', 'primary_companion', 'air_admin_primary', ?)`, airID, link.createdAt); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(`INSERT INTO air_legacy_link_mappings(
  link_id, air_id, orbit_a, orbit_b, link_created_at, backfilled_at
) VALUES(?, ?, ?, ?, ?, ?)`, link.id, airID, link.a, link.b, link.createdAt, now); err != nil {
			return 0, err
		}
		if err := appendAirAuditTx(tx, airID, "", "", 0, link.a,
			"air.migration.link_backfill", "", fmt.Sprintf("link:%d", link.id), "ok", now); err != nil {
			return 0, err
		}
		backfilled++
		if i == 0 {
			if err := s.checkpoint("air_backfill_after_first_link"); err != nil {
				return 0, err
			}
		}
	}
	return backfilled, nil
}

func validateLegacyAirMappingTx(tx *sql.Tx, linkID int64, airID string, orbitA, orbitB int64) error {
	var mappedA, mappedB int64
	if err := tx.QueryRow(`SELECT orbit_a, orbit_b FROM air_legacy_link_mappings
WHERE link_id = ? AND air_id = ?`, linkID, airID).Scan(&mappedA, &mappedB); err != nil {
		return err
	}
	if mappedA != orbitA || mappedB != orbitB {
		return fmt.Errorf("legacy link %d mapping changed from %d/%d to %d/%d", linkID, mappedA, mappedB, orbitA, orbitB)
	}
	var members, policies int
	if err := tx.QueryRow(`SELECT
  (SELECT COUNT(*) FROM air_members WHERE air_id = ? AND status = 'joined'
     AND ((orbit_id = ? AND air_role = 'owner') OR (orbit_id = ? AND air_role = 'member'))),
  (SELECT COUNT(*) FROM air_policies WHERE air_id = ?)`,
		airID, orbitA, orbitB, airID).Scan(&members, &policies); err != nil {
		return err
	}
	if members != 2 || policies != 1 {
		return fmt.Errorf("legacy link %d mapped Air %s is incomplete: members=%d policies=%d", linkID, airID, members, policies)
	}
	return nil
}

func deterministicAirMigrationID(kind string, link struct{ id, a, b, createdAt int64 }) string {
	digest := sha256.Sum256([]byte(fmt.Sprintf("pulsar-air-v1:%s:%d:%d:%d:%d", kind, link.id, link.a, link.b, link.createdAt)))
	var entropy [10]byte
	copy(entropy[:], digest[:10])
	prefix := "aim_"
	if kind == "air" {
		prefix = "air_"
	}
	return prefix + ulid.FromEntropy(time.UnixMilli(link.createdAt), entropy)
}

func airAuthorityShapeUnsafeTx(tx *sql.Tx) (bool, error) {
	var invalidPointers, activeLinks, pointers int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM air_active_pointers p
LEFT JOIN air_legacy_link_mappings m ON m.air_id = p.air_id
LEFT JOIN links l ON l.id = m.link_id AND l.state = 'active'
WHERE m.link_id IS NULL OR l.id IS NULL
   OR (p.orbit_id <> m.orbit_a AND p.orbit_id <> m.orbit_b)`).Scan(&invalidPointers); err != nil {
		return false, err
	}
	if err := tx.QueryRow(`SELECT
  (SELECT COUNT(*) FROM links WHERE state = 'active'),
  (SELECT COUNT(*) FROM air_active_pointers)`).Scan(&activeLinks, &pointers); err != nil {
		return false, err
	}
	return invalidPointers != 0 || pointers != activeLinks*2, nil
}
