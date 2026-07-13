package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// identitySchema is additive. Legacy members, slots, invites, and their
// indexes remain authoritative for the feature-off/old-binary path.
const identitySchema = `
CREATE TABLE IF NOT EXISTS actors (
  id INTEGER PRIMARY KEY,
  kind TEXT NOT NULL CHECK(kind IN ('app_installation', 'telegram_user')),
  display_name TEXT NOT NULL DEFAULT '',
  external_ref TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL,
  revoked_at INTEGER
);
CREATE UNIQUE INDEX IF NOT EXISTS actors_identity
  ON actors(kind, external_ref);

CREATE TABLE IF NOT EXISTS memberships (
  orbit_id INTEGER NOT NULL REFERENCES orbits(id),
  actor_id INTEGER NOT NULL REFERENCES actors(id),
  role TEXT NOT NULL CHECK(role IN ('primary', 'companion', 'satellite')),
  joined_at INTEGER NOT NULL,
  left_at INTEGER,
  PRIMARY KEY (orbit_id, actor_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS memberships_one_active
  ON memberships(actor_id) WHERE left_at IS NULL;

CREATE TABLE IF NOT EXISTS installation_credentials (
  actor_id INTEGER PRIMARY KEY REFERENCES actors(id),
  slot_orbit_id INTEGER NOT NULL,
  slot_name TEXT NOT NULL,
  slot_paired_at INTEGER NOT NULL,
  binding_token_hash TEXT NOT NULL
    CHECK(length(binding_token_hash) = 64
      AND binding_token_hash NOT GLOB '*[^0-9a-f]*'),
  control_token_hash TEXT
    CHECK(control_token_hash IS NULL
      OR (length(control_token_hash) = 64
          AND control_token_hash NOT GLOB '*[^0-9a-f]*')),
  recovery_id TEXT
    CHECK(recovery_id IS NULL
      OR (length(recovery_id) = 36
          AND substr(recovery_id, 1, 4) = 'rec_'
          AND length(substr(recovery_id, 5)) = 32
          AND substr(recovery_id, 5) NOT GLOB '*[^0-9a-f]*')),
  recovery_secret_hash TEXT
    CHECK(recovery_secret_hash IS NULL
      OR (length(recovery_secret_hash) = 64
          AND recovery_secret_hash NOT GLOB '*[^0-9a-f]*')),
  consumed_at INTEGER,
  created_at INTEGER NOT NULL,
  FOREIGN KEY (slot_orbit_id, slot_name) REFERENCES slots(orbit_id, slot),
  UNIQUE(slot_orbit_id, slot_name),
  CHECK(
    (recovery_id IS NULL AND recovery_secret_hash IS NULL)
    OR (recovery_id IS NOT NULL AND recovery_secret_hash IS NOT NULL)
  ),
  CHECK(consumed_at IS NULL OR recovery_id IS NOT NULL)
);
CREATE UNIQUE INDEX IF NOT EXISTS installation_credentials_recovery
  ON installation_credentials(recovery_id) WHERE recovery_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS installation_credentials_control
  ON installation_credentials(control_token_hash)
  WHERE control_token_hash IS NOT NULL;

CREATE TABLE IF NOT EXISTS device_invites (
  code_hash TEXT PRIMARY KEY
    CHECK(length(code_hash) = 64
      AND code_hash NOT GLOB '*[^0-9a-f]*'),
  orbit_id INTEGER NOT NULL REFERENCES orbits(id),
  issued_by_actor_id INTEGER NOT NULL REFERENCES actors(id),
  intended_role TEXT NOT NULL
    CHECK(intended_role IN ('companion', 'satellite')),
  expires_at INTEGER NOT NULL,
  consumed_at INTEGER,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS telegram_link_codes (
  id INTEGER PRIMARY KEY,
  code_hash TEXT NOT NULL
    CHECK(length(code_hash) = 64
      AND code_hash NOT GLOB '*[^0-9a-f]*'),
  issuer_actor_id INTEGER NOT NULL REFERENCES actors(id),
  orbit_id INTEGER NOT NULL REFERENCES orbits(id),
  desired_role TEXT NOT NULL DEFAULT 'companion'
    CHECK(desired_role IN ('companion', 'satellite')),
  expires_at INTEGER NOT NULL,
  invalidated_at INTEGER,
  consumed_at INTEGER,
  consuming_actor_id INTEGER REFERENCES actors(id),
  created_at INTEGER NOT NULL,
  CHECK(
    (consumed_at IS NULL AND consuming_actor_id IS NULL)
    OR (consumed_at IS NOT NULL AND consuming_actor_id IS NOT NULL)
  )
);
CREATE UNIQUE INDEX IF NOT EXISTS telegram_link_codes_hash
  ON telegram_link_codes(code_hash);

CREATE TABLE IF NOT EXISTS audit_events (
  id INTEGER PRIMARY KEY,
  orbit_id INTEGER NOT NULL,
  actor_id INTEGER,
  type TEXT NOT NULL,
  created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS recovery_rotation_audit_details (
  audit_event_id INTEGER PRIMARY KEY REFERENCES audit_events(id),
  old_recovery_id TEXT
    CHECK(old_recovery_id IS NULL
      OR (length(old_recovery_id) = 36
          AND substr(old_recovery_id, 1, 4) = 'rec_'
          AND length(substr(old_recovery_id, 5)) = 32
          AND substr(old_recovery_id, 5) NOT GLOB '*[^0-9a-f]*')),
  new_recovery_id TEXT NOT NULL
    CHECK(length(new_recovery_id) = 36
      AND substr(new_recovery_id, 1, 4) = 'rec_'
      AND length(substr(new_recovery_id, 5)) = 32
      AND substr(new_recovery_id, 5) NOT GLOB '*[^0-9a-f]*'),
  CHECK(old_recovery_id IS NULL OR old_recovery_id <> new_recovery_id)
);

CREATE TABLE IF NOT EXISTS rate_limit_audit_events (
  id INTEGER PRIMARY KEY,
  event_type TEXT NOT NULL
    CHECK(event_type = 'security.rate_limited'),
  limiter_class TEXT NOT NULL
    CHECK(limiter_class IN (
      'create/source-ip',
      'create/installation-attempt',
      'invite-consume/source-ip',
      'recovery-consume/source-ip',
      'recovery-consume/recovery-id',
      'recovery-rotate/actor',
      'telegram-link-issue/actor',
      'telegram-link-consume/telegram-user'
    )),
  subject_digest TEXT NOT NULL
    CHECK(length(subject_digest) = 64
      AND subject_digest NOT GLOB '*[^0-9a-f]*'),
  orbit_id INTEGER CHECK(orbit_id IS NULL OR orbit_id > 0),
  actor_id INTEGER CHECK(actor_id IS NULL OR actor_id > 0),
  created_at INTEGER NOT NULL,
  CHECK(
    (orbit_id IS NULL AND actor_id IS NULL AND limiter_class IN (
      'create/source-ip',
      'create/installation-attempt',
      'invite-consume/source-ip',
      'recovery-consume/source-ip',
      'recovery-consume/recovery-id',
      'telegram-link-consume/telegram-user'
    ))
    OR
    (orbit_id IS NOT NULL AND actor_id IS NOT NULL AND limiter_class IN (
      'recovery-rotate/actor',
      'telegram-link-issue/actor'
    ))
  )
);
CREATE INDEX IF NOT EXISTS rate_limit_audit_events_created
  ON rate_limit_audit_events(created_at);

CREATE TABLE IF NOT EXISTS rollback_projections (
  orbit_id INTEGER NOT NULL,
  original_max_pulsars INTEGER NOT NULL,
  original_max_members INTEGER NOT NULL,
  projected_at INTEGER NOT NULL,
  restored_at INTEGER,
  PRIMARY KEY (orbit_id)
);
`

const constrainedOrbitsDDL = `CREATE TABLE orbits_new (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  takeover_policy TEXT NOT NULL DEFAULT 'user',
  voice_default TEXT NOT NULL DEFAULT 'personal',
  max_pulsars INTEGER NOT NULL DEFAULT 5,
  max_members INTEGER NOT NULL DEFAULT 10,
  created_at INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'active'
    CHECK(status IN ('active', 'disabled'))
)`

var appExternalRefPattern = regexp.MustCompile(`^[0-9]+:[a-z]:[0-9a-f]{64}$`)

func detectBrokenOrbitMigration(db *sql.DB) error {
	orbits, err := tableExists(db, "orbits")
	if err != nil {
		return err
	}
	orbitsNew, err := tableExists(db, "orbits_new")
	if err != nil {
		return err
	}
	if orbitsNew && !orbits {
		return errors.New("orbits_new exists while orbits is missing; manual migration repair required")
	}
	return nil
}

func tableExists(q interface {
	QueryRow(query string, args ...any) *sql.Row
}, name string) (bool, error) {
	var n int
	err := q.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&n)
	return n > 0, err
}

func (s *Store) initIdentitySchema() error {
	// Install every additive table/index as one immediate transaction. A late
	// DDL failure therefore cannot expose a partially installed identity model
	// to either the feature-on or feature-off coordinator.
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(identitySchema); err != nil {
		return err
	}
	if err := s.checkpoint("identity_ddl_before_commit"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	// A previous binary may dissolve an orbit with foreign_keys disabled.
	// Repair only that contract-authorized stale-child shape before a possible
	// orbits table rebuild performs its global foreign_key_check. Unrelated FK
	// corruption remains and fails closed below.
	tx, err = s.db.Begin()
	if err != nil {
		return err
	}
	if err := cleanupMissingOrbitIdentityTx(tx, time.Now().UnixMilli()); err != nil {
		_ = tx.Rollback()
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}

	if err := s.ensureOrbitStatusConstraint(); err != nil {
		return err
	}
	if err := assertForeignKeys(s.db); err != nil {
		return err
	}
	return foreignKeyCheck(s.db)
}

func (s *Store) ensureOrbitStatusConstraint() error {
	if exists, err := tableExists(s.db, "orbits_new"); err != nil {
		return err
	} else if exists {
		if _, err := s.db.Exec(`DROP TABLE orbits_new`); err != nil {
			return fmt.Errorf("remove interrupted orbits_new: %w", err)
		}
	}

	hasStatus, err := columnExists(s.db, "orbits", "status")
	if err != nil {
		return err
	}
	if !hasStatus {
		_, err := s.db.Exec(`ALTER TABLE orbits ADD COLUMN status TEXT NOT NULL DEFAULT 'active'
  CHECK(status IN ('active', 'disabled'))`)
		return err
	}

	constrained, err := orbitStatusConstrained(s.db)
	if err != nil {
		return err
	}
	if constrained {
		return nil
	}

	var invalid int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM orbits WHERE status NOT IN ('active', 'disabled')`).Scan(&invalid); err != nil {
		return err
	}
	if invalid != 0 {
		return fmt.Errorf("orbits.status has %d invalid row(s); refusing automatic repair", invalid)
	}
	return s.rebuildOrbitsWithStatusConstraint()
}

func columnExists(db *sql.DB, table, column string) (bool, error) {
	rows, err := db.Query(`PRAGMA table_info(` + quoteIdent(table) + `)`)
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid, notNull, pk int
		var name, typ string
		var defaultValue any
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

// orbitStatusConstrained probes behavior inside a rolled-back transaction.
// It intentionally does not parse sqlite_master.sql.
func orbitStatusConstrained(db *sql.DB) (bool, error) {
	tx, err := db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(`SAVEPOINT identity_status_probe`); err != nil {
		return false, err
	}

	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM orbits`).Scan(&count); err != nil {
		return false, err
	}
	var probeErr error
	if count == 0 {
		_, probeErr = tx.Exec(`INSERT INTO orbits(title, created_at, status) VALUES('__identity_status_probe__', 0, 'bogus')`)
	} else {
		_, probeErr = tx.Exec(`UPDATE orbits SET status = 'bogus' WHERE id = (SELECT MIN(id) FROM orbits)`)
	}
	if _, err := tx.Exec(`ROLLBACK TO identity_status_probe`); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`RELEASE identity_status_probe`); err != nil {
		return false, err
	}
	if probeErr == nil {
		return false, nil
	}
	if isCheckConstraintError(probeErr) {
		return true, nil
	}
	return false, fmt.Errorf("orbits.status behavior probe: %w", probeErr)
}

func isCheckConstraintError(err error) bool {
	return err != nil && strings.Contains(strings.ToLower(err.Error()), "check constraint failed")
}

type schemaObject struct {
	typ     string
	name    string
	table   string
	ddl     string
	group   int
	ordinal int
}

func (s *Store) rebuildOrbitsWithStatusConstraint() (retErr error) {
	ctx := context.Background()
	conn, err := s.db.Conn(ctx)
	if err != nil {
		return err
	}
	defer conn.Close()

	var fkBefore int
	if err := conn.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&fkBefore); err != nil {
		return err
	}
	if fkBefore != 1 {
		return fmt.Errorf("foreign_keys = %d before orbits rebuild, want 1", fkBefore)
	}
	var tx *sql.Tx
	defer func() {
		var cleanupErr error
		if tx != nil {
			if err := s.checkpoint("orbit_rebuild_rollback_cleanup"); err != nil {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("rollback checkpoint: %w", err))
			}
			if err := tx.Rollback(); err != nil && !errors.Is(err, sql.ErrTxDone) {
				cleanupErr = errors.Join(cleanupErr, fmt.Errorf("rollback orbits rebuild: %w", err))
			}
			tx = nil
		}
		if err := s.checkpoint("orbit_rebuild_restore_foreign_keys"); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("restore foreign_keys checkpoint: %w", err))
		}
		if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("restore foreign_keys after orbits rebuild: %w", err))
		}
		if err := s.checkpoint("orbit_rebuild_read_foreign_keys"); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("read restored foreign_keys checkpoint: %w", err))
		}
		var restored int
		if err := conn.QueryRowContext(ctx, `PRAGMA foreign_keys`).Scan(&restored); err != nil {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("read foreign_keys after orbits rebuild: %w", err))
		} else if restored != 1 {
			cleanupErr = errors.Join(cleanupErr, fmt.Errorf("foreign_keys = %d after orbits rebuild, want 1", restored))
		}
		if recovered := recover(); recovered != nil {
			if cleanupErr != nil {
				panic(errors.Join(fmt.Errorf("orbits rebuild panic: %v", recovered), cleanupErr))
			}
			panic(recovered)
		}
		retErr = errors.Join(retErr, cleanupErr)
	}()

	objects, err := captureOrbitsSchemaObjects(conn)
	if err != nil {
		return err
	}
	if _, err := conn.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	tx, err = conn.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := s.checkpoint("orbit_rebuild_after_begin"); err != nil {
		return err
	}

	var oldSequence sql.NullInt64
	if err := tx.QueryRow(`SELECT seq FROM sqlite_sequence WHERE name = 'orbits'`).Scan(&oldSequence); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return fmt.Errorf("capture orbits autoincrement sequence: %w", err)
	}

	for _, obj := range objects {
		if obj.typ == "view" || (obj.typ == "trigger" && obj.table != "orbits") {
			if _, err := tx.Exec(`DROP ` + strings.ToUpper(obj.typ) + ` IF EXISTS ` + quoteIdent(obj.name)); err != nil {
				return err
			}
		}
	}
	if _, err := tx.Exec(constrainedOrbitsDDL); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO orbits_new
  (id, title, takeover_policy, voice_default, max_pulsars, max_members, created_at, status)
  SELECT id, title, takeover_policy, voice_default, max_pulsars, max_members, created_at, status FROM orbits`); err != nil {
		return err
	}
	if _, err := tx.Exec(`DROP TABLE orbits`); err != nil {
		return err
	}
	if _, err := tx.Exec(`ALTER TABLE orbits_new RENAME TO orbits`); err != nil {
		return err
	}
	var currentMax int64
	if err := tx.QueryRow(`SELECT COALESCE(MAX(id), 0) FROM orbits`).Scan(&currentMax); err != nil {
		return err
	}
	highWater := currentMax
	if oldSequence.Valid && oldSequence.Int64 > highWater {
		highWater = oldSequence.Int64
	}
	if _, err := tx.Exec(`DELETE FROM sqlite_sequence WHERE name IN ('orbits', 'orbits_new')`); err != nil {
		return fmt.Errorf("clear rebuilt orbits sequence: %w", err)
	}
	if highWater > 0 {
		if _, err := tx.Exec(`INSERT INTO sqlite_sequence(name, seq) VALUES('orbits', ?)`, highWater); err != nil {
			return fmt.Errorf("restore orbits autoincrement sequence: %w", err)
		}
	}
	if err := s.checkpoint("orbit_rebuild_after_sequence_restore"); err != nil {
		return err
	}
	sort.SliceStable(objects, func(i, j int) bool {
		if objects[i].group == objects[j].group {
			return objects[i].ordinal < objects[j].ordinal
		}
		return objects[i].group < objects[j].group
	})
	for _, obj := range objects {
		if _, err := tx.Exec(obj.ddl); err != nil {
			return fmt.Errorf("recreate %s %q: %w", obj.typ, obj.name, err)
		}
	}
	if err := s.checkpoint("orbit_rebuild_before_foreign_key_check"); err != nil {
		return err
	}
	if err := foreignKeyCheck(tx); err != nil {
		return err
	}
	if err := s.checkpoint("orbit_rebuild_before_behavior_probe"); err != nil {
		return err
	}
	constrained, err := orbitStatusConstrainedTx(tx)
	if err != nil {
		return err
	}
	if !constrained {
		return errors.New("rebuilt orbits.status still accepts invalid values")
	}
	if err := s.checkpoint("orbit_rebuild_before_commit"); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	tx = nil
	return nil
}

func captureOrbitsSchemaObjects(conn *sql.Conn) ([]schemaObject, error) {
	rows, err := conn.QueryContext(context.Background(), `SELECT type, name, tbl_name, sql
FROM sqlite_master
WHERE type IN ('index', 'trigger', 'view') AND sql IS NOT NULL
ORDER BY rowid`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []schemaObject
	ordinal := 0
	for rows.Next() {
		var obj schemaObject
		if err := rows.Scan(&obj.typ, &obj.name, &obj.table, &obj.ddl); err != nil {
			return nil, err
		}
		if strings.HasPrefix(obj.name, "sqlite_") {
			continue
		}
		switch {
		case (obj.typ == "index" || obj.typ == "trigger") && obj.table == "orbits":
			obj.group = 0
		case obj.typ == "view":
			obj.group = 1
		case obj.typ == "trigger" && obj.table != "orbits":
			obj.group = 2
		default:
			continue
		}
		obj.ordinal = ordinal
		ordinal++
		out = append(out, obj)
	}
	return out, rows.Err()
}

func orbitStatusConstrainedTx(tx *sql.Tx) (bool, error) {
	if _, err := tx.Exec(`SAVEPOINT identity_status_probe`); err != nil {
		return false, err
	}
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM orbits`).Scan(&count); err != nil {
		return false, err
	}
	var probeErr error
	if count == 0 {
		_, probeErr = tx.Exec(`INSERT INTO orbits(title, created_at, status) VALUES('__identity_status_probe__', 0, 'bogus')`)
	} else {
		_, probeErr = tx.Exec(`UPDATE orbits SET status = 'bogus' WHERE id = (SELECT MIN(id) FROM orbits)`)
	}
	if _, err := tx.Exec(`ROLLBACK TO identity_status_probe`); err != nil {
		return false, err
	}
	if _, err := tx.Exec(`RELEASE identity_status_probe`); err != nil {
		return false, err
	}
	if probeErr == nil {
		return false, nil
	}
	if isCheckConstraintError(probeErr) {
		return true, nil
	}
	return false, probeErr
}

func quoteIdent(v string) string {
	return `"` + strings.ReplaceAll(v, `"`, `""`) + `"`
}

func assertForeignKeys(q interface {
	QueryRow(query string, args ...any) *sql.Row
}) error {
	var enabled int
	if err := q.QueryRow(`PRAGMA foreign_keys`).Scan(&enabled); err != nil {
		return err
	}
	if enabled != 1 {
		return fmt.Errorf("PRAGMA foreign_keys = %d, want 1", enabled)
	}
	return nil
}

type rowQueryer interface {
	Query(query string, args ...any) (*sql.Rows, error)
}

func foreignKeyCheck(q rowQueryer) error {
	rows, err := q.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	defer rows.Close()
	if rows.Next() {
		var table string
		var rowID sql.NullInt64
		var parent string
		var fkID int
		if err := rows.Scan(&table, &rowID, &parent, &fkID); err != nil {
			return err
		}
		return fmt.Errorf("foreign key violation in table %s row %s referencing %s", table, nullableInt(rowID), parent)
	}
	return rows.Err()
}

func nullableInt(v sql.NullInt64) string {
	if !v.Valid {
		return "unknown"
	}
	return strconv.FormatInt(v.Int64, 10)
}

// ReconcileIdentity projects legacy members and slots into additive identity
// rows. It is the serving gate for self_service_onboarding.
func (s *Store) ReconcileIdentity() error {
	if !s.selfServiceOnboarding {
		return ErrSelfServiceOnboardingDisabled
	}
	now := time.Now().UnixMilli()
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if reconcileErr := s.reconcileIdentityTx(tx, now); reconcileErr != nil {
		rollbackErr := tx.Rollback()
		var violation *identityAlignmentViolation
		if !errors.As(reconcileErr, &violation) {
			return errors.Join(reconcileErr, rollbackErr)
		}
		quarantineErr := s.quarantineIdentityAlignment(violation, now)
		if rollbackErr != nil {
			rollbackErr = fmt.Errorf("rollback identity reconciliation before quarantine: %w", rollbackErr)
		}
		if quarantineErr != nil {
			quarantineErr = fmt.Errorf("quarantine identity alignment violation: %w", quarantineErr)
		}
		return errors.Join(reconcileErr, rollbackErr, quarantineErr)
	}
	if err := foreignKeyCheck(tx); err != nil {
		return err
	}
	return tx.Commit()
}

type legacyMember struct {
	orbitID     int64
	telegramID  int64
	role        string
	joinedAt    int64
	displayName string
}

type installationBinding struct {
	actorID        int64
	orbitID        int64
	slot           string
	pairedAt       int64
	bindingHash    string
	controlHash    sql.NullString
	externalRef    string
	actorRevokedAt sql.NullInt64
}

type legacySlot struct {
	orbitID   int64
	slot      string
	tokenHash string
	pairedBy  int64
	pairedAt  int64
	revokedAt sql.NullInt64
}

type identityAlignmentViolation struct {
	ActorID          int64
	CredentialOrbit  int64
	ActiveOrbit      int64
	ActiveMembership bool
}

func (e *identityAlignmentViolation) Error() string {
	if !e.ActiveMembership {
		return fmt.Sprintf("%v: actor %d credential orbit %d has no active membership",
			ErrIdentityAlignmentViolation, e.ActorID, e.CredentialOrbit)
	}
	return fmt.Sprintf("%v: actor %d credential orbit %d has active membership in orbit %d",
		ErrIdentityAlignmentViolation, e.ActorID, e.CredentialOrbit, e.ActiveOrbit)
}

func (e *identityAlignmentViolation) Unwrap() error {
	return ErrIdentityAlignmentViolation
}

func identityAlignmentViolationTx(tx *sql.Tx, actorID, credentialOrbit int64) error {
	var count int
	var activeOrbit sql.NullInt64
	if err := tx.QueryRow(`SELECT COUNT(*), MIN(orbit_id)
FROM memberships WHERE actor_id = ? AND left_at IS NULL`, actorID).Scan(&count, &activeOrbit); err != nil {
		return err
	}
	if count == 1 && activeOrbit.Valid && activeOrbit.Int64 == credentialOrbit {
		return nil
	}
	return &identityAlignmentViolation{
		ActorID:          actorID,
		CredentialOrbit:  credentialOrbit,
		ActiveOrbit:      activeOrbit.Int64,
		ActiveMembership: count != 0 && activeOrbit.Valid,
	}
}

func (s *Store) quarantineIdentityAlignment(expected *identityAlignmentViolation, now int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var credentialOrbit int64
	if err := tx.QueryRow(`SELECT slot_orbit_id FROM installation_credentials WHERE actor_id = ?`, expected.ActorID).Scan(&credentialOrbit); err != nil {
		return err
	}
	if credentialOrbit != expected.CredentialOrbit {
		return fmt.Errorf("actor %d credential orbit changed before quarantine", expected.ActorID)
	}
	if err := identityAlignmentViolationTx(tx, expected.ActorID, credentialOrbit); err == nil {
		return fmt.Errorf("actor %d alignment violation no longer exists", expected.ActorID)
	} else if !errors.Is(err, ErrIdentityAlignmentViolation) {
		return err
	}
	res, err := tx.Exec(`UPDATE actors SET revoked_at = COALESCE(revoked_at, ?) WHERE id = ?`, now, expected.ActorID)
	if err != nil {
		return err
	}
	if changed, err := res.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return err
		}
		return fmt.Errorf("actor %d disappeared before alignment quarantine", expected.ActorID)
	}
	if _, err := tx.Exec(`INSERT INTO audit_events(orbit_id, actor_id, type, created_at)
VALUES(?, ?, 'identity.alignment_quarantined', ?)`, credentialOrbit, expected.ActorID, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) reconcileIdentityTx(tx *sql.Tx, now int64) error {
	if err := s.restoreRollbackProjectionsTx(tx, now); err != nil {
		return err
	}
	if err := cleanupMissingOrbitIdentityTx(tx, now); err != nil {
		return err
	}
	if err := reconcileTelegramMembersTx(tx, now); err != nil {
		return err
	}
	if err := reconcileInstallationBindingsTx(tx, now); err != nil {
		return err
	}
	if err := assertCredentialDomainsDisjoint(tx); err != nil {
		return err
	}
	if err := assertIdentityServingGate(tx); err != nil {
		return err
	}
	return nil
}

// cleanupMissingOrbitIdentityTx repairs only the stale-child shape produced
// when a previous coordinator (foreign_keys off) dissolved an orbit. It does
// not touch any row whose orbit still exists, so unrelated FK corruption is
// preserved for foreign_key_check to reject.
func cleanupMissingOrbitIdentityTx(tx *sql.Tx, now int64) error {
	if _, err := tx.Exec(`UPDATE actors SET revoked_at = COALESCE(revoked_at, ?)
WHERE id IN (
  SELECT actor_id FROM installation_credentials
  WHERE slot_orbit_id NOT IN (SELECT id FROM orbits)
)`, now); err != nil {
		return err
	}
	cleanup := []string{
		`DELETE FROM device_invites WHERE orbit_id NOT IN (SELECT id FROM orbits)`,
		`DELETE FROM telegram_link_codes WHERE orbit_id NOT IN (SELECT id FROM orbits)`,
		`DELETE FROM installation_credentials WHERE slot_orbit_id NOT IN (SELECT id FROM orbits)`,
		`DELETE FROM memberships WHERE orbit_id NOT IN (SELECT id FROM orbits)`,
		`DELETE FROM rollback_projections WHERE orbit_id NOT IN (SELECT id FROM orbits)`,
	}
	for _, stmt := range cleanup {
		if _, err := tx.Exec(stmt); err != nil {
			return err
		}
	}
	return nil
}

// ProjectIdentityForLegacyRollback makes security-relevant additive state
// enforceable by the previous coordinator before an operator rolls back. The
// operation is idempotent while a projection generation is pending.
func (s *Store) ProjectIdentityForLegacyRollback() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	if _, err := tx.Exec(`DELETE FROM rollback_projections
WHERE restored_at IS NOT NULL
  AND orbit_id IN (SELECT id FROM orbits WHERE status = 'disabled')`); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO rollback_projections
  (orbit_id, original_max_pulsars, original_max_members, projected_at, restored_at)
SELECT o.id, o.max_pulsars, o.max_members, ?, NULL
FROM orbits o
WHERE o.status = 'disabled'
  AND NOT EXISTS (
    SELECT 1 FROM rollback_projections rp
    WHERE rp.orbit_id = o.id AND rp.restored_at IS NULL
  )`, now); err != nil {
		return err
	}
	if err := s.checkpoint("rollback_projection_after_journal"); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE orbits SET max_pulsars = 0, max_members = 0
WHERE status = 'disabled'`); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE slots SET revoked_at = COALESCE(revoked_at, ?)
WHERE orbit_id IN (SELECT id FROM orbits WHERE status = 'disabled')`, now); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE slots SET revoked_at = COALESCE(revoked_at, ?)
WHERE (orbit_id, slot) IN (
  SELECT ic.slot_orbit_id, ic.slot_name
  FROM installation_credentials ic
  JOIN actors a ON a.id = ic.actor_id
  WHERE a.revoked_at IS NOT NULL
)`, now); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE invites SET used_at = COALESCE(used_at, ?)
WHERE orbit_id IN (SELECT id FROM orbits WHERE status = 'disabled')`, now); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE device_invites SET consumed_at = COALESCE(consumed_at, ?)
WHERE orbit_id IN (SELECT id FROM orbits WHERE status = 'disabled')`, now); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE telegram_link_codes SET invalidated_at = COALESCE(invalidated_at, ?)
WHERE orbit_id IN (SELECT id FROM orbits WHERE status = 'disabled')`, now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) restoreRollbackProjectionsTx(tx *sql.Tx, now int64) error {
	if _, err := tx.Exec(`UPDATE orbits SET
  max_pulsars = (SELECT rp.original_max_pulsars FROM rollback_projections rp
                 WHERE rp.orbit_id = orbits.id AND rp.restored_at IS NULL),
  max_members = (SELECT rp.original_max_members FROM rollback_projections rp
                 WHERE rp.orbit_id = orbits.id AND rp.restored_at IS NULL)
WHERE id IN (SELECT orbit_id FROM rollback_projections WHERE restored_at IS NULL)`); err != nil {
		return err
	}
	if err := s.checkpoint("rollback_restoration_after_quota_update"); err != nil {
		return err
	}
	_, err := tx.Exec(`UPDATE rollback_projections SET restored_at = ? WHERE restored_at IS NULL`, now)
	return err
}

func reconcileTelegramMembersTx(tx *sql.Tx, now int64) error {
	legacy, err := loadLegacyMembers(tx)
	if err != nil {
		return err
	}
	legacyByActorRef := make(map[string]legacyMember, len(legacy))
	for _, member := range legacy {
		legacyByActorRef[strconv.FormatInt(member.telegramID, 10)] = member
	}

	// Legacy rows are authoritative after an old-binary interval. Mark any
	// additive Telegram membership that no longer has its exact legacy row left.
	rows, err := tx.Query(`SELECT a.id, a.external_ref, m.orbit_id
FROM actors a JOIN memberships m ON m.actor_id = a.id
WHERE a.kind = 'telegram_user' AND m.left_at IS NULL`)
	if err != nil {
		return err
	}
	type activeTelegramMembership struct {
		actorID int64
		ref     string
		orbitID int64
	}
	var active []activeTelegramMembership
	for rows.Next() {
		var item activeTelegramMembership
		if err := rows.Scan(&item.actorID, &item.ref, &item.orbitID); err != nil {
			rows.Close()
			return err
		}
		active = append(active, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, item := range active {
		member, ok := legacyByActorRef[item.ref]
		if ok && member.orbitID == item.orbitID {
			continue
		}
		if _, err := tx.Exec(`UPDATE memberships SET left_at = ?
WHERE orbit_id = ? AND actor_id = ? AND left_at IS NULL`, now, item.orbitID, item.actorID); err != nil {
			return err
		}
	}

	for _, member := range legacy {
		actorID, revoked, err := findActorTx(tx, "telegram_user", strconv.FormatInt(member.telegramID, 10))
		if err != nil {
			return err
		}
		if actorID == 0 {
			res, err := tx.Exec(`INSERT INTO actors(kind, display_name, external_ref, created_at)
VALUES('telegram_user', ?, ?, ?)`, member.displayName, strconv.FormatInt(member.telegramID, 10), member.joinedAt)
			if err != nil {
				return err
			}
			actorID, err = res.LastInsertId()
			if err != nil {
				return err
			}
		} else {
			if revoked {
				return fmt.Errorf("telegram actor %d is revoked but legacy member %d is active", actorID, member.telegramID)
			}
			if _, err := tx.Exec(`UPDATE actors SET display_name = ? WHERE id = ?`, member.displayName, actorID); err != nil {
				return err
			}
		}
		if err := ensureMembershipTx(tx, member.orbitID, actorID, member.role, member.joinedAt); err != nil {
			return err
		}
	}
	return nil
}

func loadLegacyMembers(tx *sql.Tx) ([]legacyMember, error) {
	rows, err := tx.Query(`SELECT orbit_id, tg_user_id, role, joined_at, display_name
FROM members ORDER BY orbit_id, joined_at, tg_user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []legacyMember
	for rows.Next() {
		var item legacyMember
		if err := rows.Scan(&item.orbitID, &item.telegramID, &item.role, &item.joinedAt, &item.displayName); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func reconcileInstallationBindingsTx(tx *sql.Tx, now int64) error {
	bindings, err := loadInstallationBindings(tx)
	if err != nil {
		return err
	}
	for _, binding := range bindings {
		slot, found, err := findLegacySlotTx(tx, binding.orbitID, binding.slot)
		if err != nil {
			return err
		}
		if !found || slot.revokedAt.Valid {
			if err := retireInstallationTx(tx, binding.actorID, now); err != nil {
				return err
			}
			continue
		}
		expectedRef, err := installationExternalRef(slot.orbitID, slot.slot, slot.tokenHash)
		if err != nil {
			return err
		}
		if binding.externalRef == expectedRef && binding.bindingHash != slot.tokenHash {
			return fmt.Errorf("installation actor %d binding collision for orbit %d slot %s", binding.actorID, binding.orbitID, binding.slot)
		}
		if binding.pairedAt != slot.pairedAt || binding.bindingHash != slot.tokenHash || binding.externalRef != expectedRef {
			if err := retireInstallationTx(tx, binding.actorID, now); err != nil {
				return err
			}
			continue
		}
		if binding.actorRevokedAt.Valid {
			return fmt.Errorf("active slot %d/%s is bound to revoked actor %d", binding.orbitID, binding.slot, binding.actorID)
		}
		// App-first installations deliberately have no legacy Telegram
		// paired_by owner. Once a control credential has been provisioned, the
		// additive membership is authoritative for that installation and must
		// not be downgraded to the conservative legacy-orphan satellite role.
		// Unprovisioned paired_by=0 bindings still follow the legacy orphan path.
		if slot.pairedBy == 0 && binding.controlHash.Valid {
			if err := identityAlignmentViolationTx(tx, binding.actorID, binding.orbitID); err != nil {
				return err
			}
			continue
		}
		role, _, err := installationRoleTx(tx, slot.orbitID, slot.pairedBy)
		if err != nil {
			return err
		}
		if err := ensureMembershipTx(tx, slot.orbitID, binding.actorID, role, now); err != nil {
			return err
		}
	}

	// Phase B: create only from unrevoked slots in active orbits. Disabled
	// orbits keep legacy bindings intact but do not become newly servable.
	rows, err := tx.Query(`SELECT s.orbit_id, s.slot, s.token_hash, s.paired_by,
       COALESCE(s.paired_at, 0), s.revoked_at
FROM slots s
JOIN orbits o ON o.id = s.orbit_id AND o.status = 'active'
LEFT JOIN installation_credentials ic
  ON ic.slot_orbit_id = s.orbit_id AND ic.slot_name = s.slot
  AND ic.binding_token_hash = s.token_hash
WHERE s.revoked_at IS NULL AND ic.actor_id IS NULL
ORDER BY s.orbit_id, s.slot`)
	if err != nil {
		return err
	}
	var missing []legacySlot
	for rows.Next() {
		var item legacySlot
		if err := rows.Scan(&item.orbitID, &item.slot, &item.tokenHash, &item.pairedBy, &item.pairedAt, &item.revokedAt); err != nil {
			rows.Close()
			return err
		}
		missing = append(missing, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, slot := range missing {
		if err := createInstallationBindingTx(tx, slot, now); err != nil {
			return err
		}
	}
	return nil
}

func loadInstallationBindings(tx *sql.Tx) ([]installationBinding, error) {
	rows, err := tx.Query(`SELECT ic.actor_id, ic.slot_orbit_id, ic.slot_name,
	       ic.slot_paired_at, ic.binding_token_hash, ic.control_token_hash,
	       a.external_ref, a.revoked_at
FROM installation_credentials ic
JOIN actors a ON a.id = ic.actor_id
ORDER BY ic.slot_orbit_id, ic.slot_name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []installationBinding
	for rows.Next() {
		var item installationBinding
		if err := rows.Scan(&item.actorID, &item.orbitID, &item.slot, &item.pairedAt,
			&item.bindingHash, &item.controlHash, &item.externalRef, &item.actorRevokedAt); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func findLegacySlotTx(tx *sql.Tx, orbitID int64, slotName string) (legacySlot, bool, error) {
	var slot legacySlot
	err := tx.QueryRow(`SELECT orbit_id, slot, token_hash, paired_by,
       COALESCE(paired_at, 0), revoked_at
FROM slots WHERE orbit_id = ? AND slot = ?`, orbitID, slotName).Scan(
		&slot.orbitID, &slot.slot, &slot.tokenHash, &slot.pairedBy, &slot.pairedAt, &slot.revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return legacySlot{}, false, nil
	}
	return slot, err == nil, err
}

func createInstallationBindingTx(tx *sql.Tx, slot legacySlot, now int64) error {
	externalRef, err := installationExternalRef(slot.orbitID, slot.slot, slot.tokenHash)
	if err != nil {
		return err
	}
	actorID, revoked, err := findActorTx(tx, "app_installation", externalRef)
	if err != nil {
		return err
	}
	if actorID != 0 {
		var existingHash string
		err := tx.QueryRow(`SELECT binding_token_hash FROM installation_credentials WHERE actor_id = ?`, actorID).Scan(&existingHash)
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("installation actor %d exists without verifiable credential binding", actorID)
		}
		if err != nil {
			return err
		}
		if existingHash != slot.tokenHash {
			return fmt.Errorf("installation actor %d binding collision for orbit %d slot %s", actorID, slot.orbitID, slot.slot)
		}
		if revoked {
			return fmt.Errorf("installation actor %d is revoked for active slot %d/%s", actorID, slot.orbitID, slot.slot)
		}
		return nil
	}

	res, err := tx.Exec(`INSERT INTO actors(kind, display_name, external_ref, created_at)
VALUES('app_installation', ?, ?, ?)`, slot.slot, externalRef, now)
	if err != nil {
		return err
	}
	actorID, err = res.LastInsertId()
	if err != nil {
		return err
	}
	role, orphan, err := installationRoleTx(tx, slot.orbitID, slot.pairedBy)
	if err != nil {
		return err
	}
	if err := ensureMembershipTx(tx, slot.orbitID, actorID, role, now); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO installation_credentials
  (actor_id, slot_orbit_id, slot_name, slot_paired_at, binding_token_hash,
   control_token_hash, recovery_id, recovery_secret_hash, consumed_at, created_at)
VALUES(?, ?, ?, ?, ?, NULL, NULL, NULL, NULL, ?)`,
		actorID, slot.orbitID, slot.slot, slot.pairedAt, slot.tokenHash, now); err != nil {
		return err
	}
	if orphan {
		_, err = tx.Exec(`INSERT INTO audit_events(orbit_id, actor_id, type, created_at)
VALUES(?, ?, 'identity.backfill_orphan_slot', ?)`, slot.orbitID, actorID, now)
		return err
	}
	return nil
}

func installationRoleTx(tx *sql.Tx, orbitID, pairedBy int64) (role string, orphan bool, err error) {
	if pairedBy == 0 {
		return "satellite", true, nil
	}
	var memberRole string
	err = tx.QueryRow(`SELECT role FROM members WHERE orbit_id = ? AND tg_user_id = ?`, orbitID, pairedBy).Scan(&memberRole)
	if errors.Is(err, sql.ErrNoRows) {
		return "satellite", true, nil
	}
	if err != nil {
		return "", false, err
	}
	if memberRole == "primary" {
		return "primary", false, nil
	}
	return "companion", false, nil
}

func findActorTx(tx *sql.Tx, kind, externalRef string) (actorID int64, revoked bool, err error) {
	var revokedAt sql.NullInt64
	err = tx.QueryRow(`SELECT id, revoked_at FROM actors WHERE kind = ? AND external_ref = ?`, kind, externalRef).Scan(&actorID, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, nil
	}
	return actorID, revokedAt.Valid, err
}

func ensureMembershipTx(tx *sql.Tx, orbitID, actorID int64, role string, joinedAt int64) error {
	var activeOrbit int64
	err := tx.QueryRow(`SELECT orbit_id FROM memberships WHERE actor_id = ? AND left_at IS NULL`, actorID).Scan(&activeOrbit)
	if err == nil && activeOrbit != orbitID {
		return fmt.Errorf("actor %d has active membership in orbit %d, expected %d", actorID, activeOrbit, orbitID)
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	_, err = tx.Exec(`INSERT INTO memberships(orbit_id, actor_id, role, joined_at, left_at)
VALUES(?, ?, ?, ?, NULL)
ON CONFLICT(orbit_id, actor_id) DO UPDATE SET
  role = excluded.role,
  left_at = NULL`, orbitID, actorID, role, joinedAt)
	return err
}

func retireInstallationTx(tx *sql.Tx, actorID, now int64) error {
	if _, err := tx.Exec(`UPDATE actors SET revoked_at = COALESCE(revoked_at, ?) WHERE id = ?`, now, actorID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM installation_credentials WHERE actor_id = ?`, actorID); err != nil {
		return err
	}
	_, err := tx.Exec(`UPDATE memberships SET left_at = COALESCE(left_at, ?) WHERE actor_id = ? AND left_at IS NULL`, now, actorID)
	return err
}

func assertIdentityServingGate(tx *sql.Tx) error {
	var orbitID int64
	var slot string
	err := tx.QueryRow(`SELECT s.orbit_id, s.slot
FROM slots s
JOIN orbits o ON o.id = s.orbit_id AND o.status = 'active'
LEFT JOIN installation_credentials ic
  ON ic.slot_orbit_id = s.orbit_id AND ic.slot_name = s.slot
  AND ic.binding_token_hash = s.token_hash
WHERE s.revoked_at IS NULL AND ic.actor_id IS NULL
LIMIT 1`).Scan(&orbitID, &slot)
	if err == nil {
		return fmt.Errorf("identity serving gate: active slot %d/%s lacks a current installation credential", orbitID, slot)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}

	var actorID, credentialOrbit int64
	err = tx.QueryRow(`SELECT ic.actor_id, ic.slot_orbit_id
FROM installation_credentials ic
WHERE (SELECT COUNT(*) FROM memberships m
       WHERE m.actor_id = ic.actor_id AND m.left_at IS NULL) <> 1
   OR NOT EXISTS (
       SELECT 1 FROM memberships m
       WHERE m.actor_id = ic.actor_id
         AND m.orbit_id = ic.slot_orbit_id
         AND m.left_at IS NULL
   )
ORDER BY ic.actor_id
LIMIT 1`).Scan(&actorID, &credentialOrbit)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return identityAlignmentViolationTx(tx, actorID, credentialOrbit)
}

func assertCredentialDomainsDisjoint(q interface {
	QueryRow(query string, args ...any) *sql.Row
}) error {
	var actorID, orbitID int64
	var slot string
	err := q.QueryRow(`SELECT ic.actor_id, s.orbit_id, s.slot
FROM installation_credentials ic
JOIN slots s ON s.token_hash = ic.control_token_hash
WHERE ic.control_token_hash IS NOT NULL
LIMIT 1`).Scan(&actorID, &orbitID, &slot)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	return fmt.Errorf("%w: actor %d and node slot %d/%s", ErrCredentialDomainConflict, actorID, orbitID, slot)
}
