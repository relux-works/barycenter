package store

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

func createUnconstrainedOrbitsFixture(t *testing.T, path string, highWater int64) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`CREATE TABLE orbits (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  takeover_policy TEXT NOT NULL DEFAULT 'user',
  voice_default TEXT NOT NULL DEFAULT 'personal',
  max_pulsars INTEGER NOT NULL DEFAULT 5,
  max_members INTEGER NOT NULL DEFAULT 10,
  created_at INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'active'
)`); err != nil {
		t.Fatal(err)
	}
	if highWater > 0 {
		if _, err := db.Exec(`INSERT INTO orbits(id, title, created_at) VALUES(?, 'retired', 1)`, highWater); err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`DELETE FROM orbits WHERE id = ?`, highWater); err != nil {
			t.Fatal(err)
		}
	}
}

func openMigrationStore(t *testing.T, path string) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(ON)&_pragma=busy_timeout(5000)&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	t.Cleanup(func() { _ = db.Close() })
	return &Store{db: db}
}

// R3: rebuilding an unconstrained status column must retain deleted IDs in
// sqlite_sequence so a future tenant cannot reuse a historical orbit ID.
func TestR3OrbitStatusRebuildPreservesAutoincrementHighWater(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sequence.db")
	createUnconstrainedOrbitsFixture(t, path, 100)
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	orbit, err := s.CreateOrbit("After rebuild", 101)
	if err != nil {
		t.Fatal(err)
	}
	if orbit.ID != 101 {
		t.Fatalf("next orbit id = %d, want 101", orbit.ID)
	}
	var sequence int64
	if err := s.db.QueryRow(`SELECT seq FROM sqlite_sequence WHERE name = 'orbits'`).Scan(&sequence); err != nil || sequence != 101 {
		t.Fatalf("orbits sequence = %d err=%v", sequence, err)
	}
}

func TestR3OrbitStatusRebuildRollsBackSequenceOnFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "sequence-rollback.db")
	createUnconstrainedOrbitsFixture(t, path, 100)
	s := openMigrationStore(t, path)
	injected := errors.New("after sequence restore")
	s.testCheckpoint = func(name string) error {
		if name == "orbit_rebuild_after_sequence_restore" {
			return injected
		}
		return nil
	}
	if err := s.ensureOrbitStatusConstraint(); !errors.Is(err, injected) {
		t.Fatalf("rebuild failure = %v", err)
	}
	if constrained, err := orbitStatusConstrained(s.db); err != nil || constrained {
		t.Fatalf("failed rebuild committed constraint: constrained=%v err=%v", constrained, err)
	}
	var sequence int64
	if err := s.db.QueryRow(`SELECT seq FROM sqlite_sequence WHERE name = 'orbits'`).Scan(&sequence); err != nil || sequence != 100 {
		t.Fatalf("rolled-back sequence = %d err=%v", sequence, err)
	}

	s.testCheckpoint = nil
	if err := s.ensureOrbitStatusConstraint(); err != nil {
		t.Fatal(err)
	}
	res, err := s.db.Exec(`INSERT INTO orbits(title, created_at) VALUES('next', 2)`)
	if err != nil {
		t.Fatal(err)
	}
	if id, _ := res.LastInsertId(); id != 101 {
		t.Fatalf("post-retry id = %d, want 101", id)
	}
}

// R4: the migration must clean only old-binary dissolution children before
// rebuilding orbits, including when the additive schema was only partial.
func TestR4PartialSchemaAndOldBinaryDissolutionMigrates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "partial-dissolution.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(orbitSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ALTER TABLE orbits ADD COLUMN status TEXT NOT NULL DEFAULT 'active'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(identitySchema); err != nil {
		t.Fatal(err)
	}
	// Leave a deliberately partial identity install for the new atomic DDL to
	// complete while preserving the stale rows below.
	if _, err := db.Exec(`DROP TABLE device_invites; DROP TABLE telegram_link_codes`); err != nil {
		t.Fatal(err)
	}
	nodeToken := randomHex(32)
	nodeHash := hashToken(nodeToken)
	externalRef, err := installationExternalRef(1, "a", nodeHash)
	if err != nil {
		t.Fatal(err)
	}
	statements := []string{
		`INSERT INTO orbits(id, title, created_at) VALUES(1, 'dissolved', 1)`,
		`INSERT INTO members(orbit_id, tg_user_id, role, joined_at) VALUES(1, 101, 'primary', 1)`,
		fmt.Sprintf(`INSERT INTO slots(orbit_id, slot, token_hash, paired_by, paired_at) VALUES(1, 'a', '%s', 101, 1)`, nodeHash),
		fmt.Sprintf(`INSERT INTO actors(id, kind, display_name, external_ref, created_at) VALUES(10, 'app_installation', 'a', '%s', 1)`, externalRef),
		`INSERT INTO memberships(orbit_id, actor_id, role, joined_at) VALUES(1, 10, 'primary', 1)`,
		fmt.Sprintf(`INSERT INTO installation_credentials(actor_id, slot_orbit_id, slot_name, slot_paired_at, binding_token_hash, created_at) VALUES(10, 1, 'a', 1, '%s', 1)`, nodeHash),
		`DELETE FROM members WHERE orbit_id = 1`,
		`DELETE FROM slots WHERE orbit_id = 1`,
		`DELETE FROM orbits WHERE id = 1`,
	}
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			db.Close()
			t.Fatalf("fixture statement failed: %v", err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var revoked sql.NullInt64
	if err := s.db.QueryRow(`SELECT revoked_at FROM actors WHERE id = 10`).Scan(&revoked); err != nil || !revoked.Valid {
		t.Fatalf("dissolved installation actor revoked_at=%+v err=%v", revoked, err)
	}
	for _, table := range []string{"memberships", "installation_credentials"} {
		var count int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + table).Scan(&count); err != nil || count != 0 {
			t.Fatalf("stale %s rows=%d err=%v", table, count, err)
		}
	}
	if constrained, err := orbitStatusConstrained(s.db); err != nil || !constrained {
		t.Fatalf("status repair constrained=%v err=%v", constrained, err)
	}
	assertDatabaseHealthy(t, s)
}

func TestR4UnrelatedForeignKeyCorruptionStillFailsClosed(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unrelated-corruption.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(orbitSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`ALTER TABLE orbits ADD COLUMN status TEXT NOT NULL DEFAULT 'active'`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(identitySchema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO orbits(id, title, created_at) VALUES(1, 'alive', 1);
INSERT INTO memberships(orbit_id, actor_id, role, joined_at) VALUES(1, 999, 'satellite', 1)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if s, err := Open(path); err == nil || !strings.Contains(err.Error(), "foreign key violation") {
		if s != nil {
			s.Close()
		}
		t.Fatalf("unrelated corruption open error = %v", err)
	}
}

// R6: every rebuild exit restores and re-reads foreign_keys. Cleanup failures
// are joined with the primary failure instead of being suppressed.
func TestR6OrbitStatusRebuildCleanupFaultMatrix(t *testing.T) {
	primaryStages := []string{
		"orbit_rebuild_after_begin",
		"orbit_rebuild_after_sequence_restore",
		"orbit_rebuild_before_foreign_key_check",
		"orbit_rebuild_before_behavior_probe",
		"orbit_rebuild_before_commit",
	}
	for _, stage := range primaryStages {
		t.Run(stage, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "fault.db")
			createUnconstrainedOrbitsFixture(t, path, 7)
			s := openMigrationStore(t, path)
			injected := errors.New(stage)
			s.testCheckpoint = func(name string) error {
				if name == stage {
					return injected
				}
				return nil
			}
			if err := s.ensureOrbitStatusConstraint(); !errors.Is(err, injected) {
				t.Fatalf("fault result = %v", err)
			}
			if err := assertForeignKeys(s.db); err != nil {
				t.Fatalf("foreign_keys not restored: %v", err)
			}
		})
	}

	for _, cleanupStage := range []string{
		"orbit_rebuild_rollback_cleanup",
		"orbit_rebuild_restore_foreign_keys",
		"orbit_rebuild_read_foreign_keys",
	} {
		t.Run("joined_"+cleanupStage, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "joined.db")
			createUnconstrainedOrbitsFixture(t, path, 7)
			s := openMigrationStore(t, path)
			primary := errors.New("primary migration failure")
			cleanup := errors.New("cleanup migration failure")
			s.testCheckpoint = func(name string) error {
				switch name {
				case "orbit_rebuild_after_sequence_restore":
					return primary
				case cleanupStage:
					return cleanup
				default:
					return nil
				}
			}
			err := s.ensureOrbitStatusConstraint()
			if !errors.Is(err, primary) || !errors.Is(err, cleanup) {
				t.Fatalf("joined cleanup error = %v", err)
			}
			if err := assertForeignKeys(s.db); err != nil {
				t.Fatalf("foreign_keys not restored after joined error: %v", err)
			}
		})
	}

	t.Run("panic cleanup", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "panic.db")
		createUnconstrainedOrbitsFixture(t, path, 7)
		s := openMigrationStore(t, path)
		s.testCheckpoint = func(name string) error {
			if name == "orbit_rebuild_after_sequence_restore" {
				panic("injected migration panic")
			}
			return nil
		}
		var recovered any
		func() {
			defer func() { recovered = recover() }()
			_ = s.ensureOrbitStatusConstraint()
		}()
		if recovered == nil {
			t.Fatal("migration panic was swallowed")
		}
		if err := assertForeignKeys(s.db); err != nil {
			t.Fatalf("foreign_keys not restored after panic: %v", err)
		}
	})
}

// R7: a failure at a late identity DDL statement rolls back every earlier
// table/index creation. Removing the obstacle permits an idempotent feature-off
// reopen with the full additive schema.
func TestR7IdentityDDLIsAtomicAndFeatureOffRecoveryIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "atomic-ddl.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(schema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(orbitSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE VIEW telegram_link_codes AS SELECT 1 AS value`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	if s, err := Open(path); err == nil {
		if s != nil {
			s.Close()
		}
		t.Fatal("late identity DDL conflict unexpectedly succeeded")
	}
	inspect, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"actors", "memberships", "installation_credentials", "device_invites"} {
		var count int
		if err := inspect.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = ?`, name).Scan(&count); err != nil || count != 0 {
			inspect.Close()
			t.Fatalf("partial DDL table %s count=%d err=%v", name, count, err)
		}
	}
	if _, err := inspect.Exec(`DROP VIEW telegram_link_codes`); err != nil {
		inspect.Close()
		t.Fatal(err)
	}
	if err := inspect.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, name := range []string{"actors", "memberships", "installation_credentials", "device_invites", "telegram_link_codes", "audit_events", "recovery_rotation_audit_details", "rate_limit_audit_events"} {
		if exists, err := tableExists(s.db, name); err != nil || !exists {
			t.Fatalf("recovered schema table %s exists=%v err=%v", name, exists, err)
		}
	}
	if _, err := s.ResolveTokenActorContext(randomHex(32)); !errors.Is(err, ErrSelfServiceOnboardingDisabled) {
		t.Fatalf("feature-off resolver error = %v", err)
	}
	assertDatabaseHealthy(t, s)
}
