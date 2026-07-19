package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	_ "modernc.org/sqlite"
)

func TestLegacyBootstrapDDLFailureRollsBackEveryStatementAndReruns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy-bootstrap-failure.db")
	db := openRawMigrationFixture(t, path)
	if _, err := db.Exec(`CREATE TABLE media (
  id TEXT PRIMARY KEY,
  tg_file_id TEXT,
  duration_ms INTEGER,
  path_wav TEXT,
  loudnorm_json TEXT,
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'ready'
)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO media(
  id, created_at, expires_at, status
) VALUES('legacy-media', 10, 20, 'ready')`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	wantErr := errors.New("legacy ddl fault")
	if store, err := openWithOptionsAndCheckpoint(path, Options{}, func(name string) error {
		if name == "legacy_ddl_before_commit" {
			return wantErr
		}
		return nil
	}); !errors.Is(err, wantErr) || store != nil {
		t.Fatalf("failed open store=%v err=%v", store, err)
	}

	db = openRawMigrationFixture(t, path)
	assertMigrationColumn(t, db, "media", "orbit_id", false)
	assertMigrationTable(t, db, "elements", false)
	assertMigrationTable(t, db, "settings", false)
	assertMigrationTable(t, db, "events", false)
	var status string
	if err := db.QueryRow(`SELECT status FROM media WHERE id = 'legacy-media'`).Scan(&status); err != nil || status != "ready" {
		t.Fatalf("legacy row after rollback status=%q err=%v", status, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	assertMigrationColumn(t, store.db, "media", "orbit_id", true)
	assertMigrationTable(t, store.db, "elements", true)
	var orbitID int64
	if err := store.db.QueryRow(`SELECT orbit_id FROM media WHERE id = 'legacy-media'`).Scan(&orbitID); err != nil || orbitID != 0 {
		t.Fatalf("legacy orbit backfill=%d err=%v", orbitID, err)
	}
}

func TestOrbitBootstrapDDLFailureRollsBackColumnsAndLinksAndReruns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "orbit-bootstrap-failure.db")
	db := openRawMigrationFixture(t, path)
	if _, err := db.Exec(`CREATE TABLE orbits (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  takeover_policy TEXT NOT NULL DEFAULT 'user',
  voice_default TEXT NOT NULL DEFAULT 'personal',
  max_pulsars INTEGER NOT NULL DEFAULT 5,
  max_members INTEGER NOT NULL DEFAULT 10,
  created_at INTEGER NOT NULL
);
CREATE TABLE members (
  orbit_id INTEGER NOT NULL,
  tg_user_id INTEGER NOT NULL,
  role TEXT NOT NULL,
  joined_at INTEGER NOT NULL,
  PRIMARY KEY (orbit_id, tg_user_id)
);
CREATE TABLE slots (
  orbit_id INTEGER NOT NULL,
  slot TEXT NOT NULL,
  token_hash TEXT NOT NULL,
  paired_by INTEGER NOT NULL DEFAULT 0,
  paired_at INTEGER,
  revoked_at INTEGER,
  PRIMARY KEY (orbit_id, slot)
);
INSERT INTO orbits(id, title, created_at) VALUES(7, 'legacy', 10);
INSERT INTO members(orbit_id, tg_user_id, role, joined_at)
VALUES(7, 7001, 'primary', 10);
INSERT INTO slots(orbit_id, slot, token_hash, paired_by)
VALUES(7, 'a', 'legacy-hash', 7001)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	wantErr := errors.New("orbit ddl fault")
	if store, err := openWithOptionsAndCheckpoint(path, Options{}, func(name string) error {
		if name == "orbit_ddl_before_commit" {
			return wantErr
		}
		return nil
	}); !errors.Is(err, wantErr) || store != nil {
		t.Fatalf("failed open store=%v err=%v", store, err)
	}

	db = openRawMigrationFixture(t, path)
	assertMigrationColumn(t, db, "slots", "provider", false)
	assertMigrationColumn(t, db, "members", "display_name", false)
	assertMigrationTable(t, db, "links", false)
	var members, slots int
	if err := db.QueryRow(`SELECT
  (SELECT COUNT(*) FROM members WHERE orbit_id = 7),
  (SELECT COUNT(*) FROM slots WHERE orbit_id = 7)`).Scan(&members, &slots); err != nil || members != 1 || slots != 1 {
		t.Fatalf("legacy authority after rollback members=%d slots=%d err=%v", members, slots, err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	assertMigrationColumn(t, store.db, "slots", "provider", true)
	assertMigrationColumn(t, store.db, "members", "display_name", true)
	assertMigrationTable(t, store.db, "links", true)
	var provider, displayName string
	if err := store.db.QueryRow(`SELECT provider FROM slots WHERE orbit_id = 7 AND slot = 'a'`).Scan(&provider); err != nil || provider != "spotify" {
		t.Fatalf("provider backfill=%q err=%v", provider, err)
	}
	if err := store.db.QueryRow(`SELECT display_name FROM members WHERE orbit_id = 7 AND tg_user_id = 7001`).Scan(&displayName); err != nil || displayName != "" {
		t.Fatalf("display-name backfill=%q err=%v", displayName, err)
	}
}

func TestConcurrentBootstrapMigrationSerializesAndPreservesLegacyAuthority(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrent-bootstrap.db")
	db := openRawMigrationFixture(t, path)
	if _, err := db.Exec(`CREATE TABLE media (
  id TEXT PRIMARY KEY,
  tg_file_id TEXT,
  duration_ms INTEGER,
  path_wav TEXT,
  loudnorm_json TEXT,
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'ready'
);
CREATE TABLE orbits (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  takeover_policy TEXT NOT NULL DEFAULT 'user',
  voice_default TEXT NOT NULL DEFAULT 'personal',
  max_pulsars INTEGER NOT NULL DEFAULT 5,
  max_members INTEGER NOT NULL DEFAULT 10,
  created_at INTEGER NOT NULL
);
CREATE TABLE members (
  orbit_id INTEGER NOT NULL,
  tg_user_id INTEGER NOT NULL,
  role TEXT NOT NULL,
  joined_at INTEGER NOT NULL,
  PRIMARY KEY (orbit_id, tg_user_id)
);
CREATE TABLE slots (
  orbit_id INTEGER NOT NULL,
  slot TEXT NOT NULL,
  token_hash TEXT NOT NULL,
  paired_by INTEGER NOT NULL DEFAULT 0,
  paired_at INTEGER,
  revoked_at INTEGER,
  PRIMARY KEY (orbit_id, slot)
);
INSERT INTO media(id, created_at, expires_at, status)
VALUES('legacy-concurrent-media', 10, 20, 'ready');
INSERT INTO orbits(id, title, created_at) VALUES(9, 'legacy concurrent', 10);
INSERT INTO members(orbit_id, tg_user_id, role, joined_at)
VALUES(9, 9001, 'primary', 10);
INSERT INTO slots(orbit_id, slot, token_hash, paired_by)
VALUES(9, 'a', 'concurrent-legacy-hash', 9001)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	stores := make(chan *Store, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			store, err := Open(path)
			if err != nil {
				errs <- err
				return
			}
			stores <- store
		}()
	}
	close(start)
	wg.Wait()
	close(stores)
	close(errs)
	for err := range errs {
		t.Errorf("concurrent open: %v", err)
	}
	opened := 0
	for store := range stores {
		opened++
		if err := store.Close(); err != nil {
			t.Error(err)
		}
	}
	if opened != 2 {
		t.Fatalf("successful concurrent opens=%d want=2", opened)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	var mediaRows, memberRows, slotRows int
	if err := store.db.QueryRow(`SELECT
  (SELECT COUNT(*) FROM media WHERE id = 'legacy-concurrent-media'),
  (SELECT COUNT(*) FROM members WHERE orbit_id = 9 AND tg_user_id = 9001),
  (SELECT COUNT(*) FROM slots WHERE orbit_id = 9 AND slot = 'a')`).Scan(
		&mediaRows, &memberRows, &slotRows,
	); err != nil || mediaRows != 1 || memberRows != 1 || slotRows != 1 {
		t.Fatalf("legacy rows media=%d members=%d slots=%d err=%v", mediaRows, memberRows, slotRows, err)
	}
	assertMigrationColumn(t, store.db, "media", "orbit_id", true)
	assertMigrationColumn(t, store.db, "members", "display_name", true)
	assertMigrationColumn(t, store.db, "slots", "provider", true)
	assertDatabaseHealthy(t, store)
}

func TestPartiallyAppliedLegacyColumnsResumeWithoutRewritingRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "partial-bootstrap.db")
	db := openRawMigrationFixture(t, path)
	if _, err := db.Exec(`CREATE TABLE media (
  id TEXT PRIMARY KEY,
  tg_file_id TEXT,
  duration_ms INTEGER,
  path_wav TEXT,
  loudnorm_json TEXT,
  created_at INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'ready',
  orbit_id INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE orbits (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  takeover_policy TEXT NOT NULL DEFAULT 'user',
  voice_default TEXT NOT NULL DEFAULT 'personal',
  max_pulsars INTEGER NOT NULL DEFAULT 5,
  max_members INTEGER NOT NULL DEFAULT 10,
  created_at INTEGER NOT NULL
);
CREATE TABLE members (
  orbit_id INTEGER NOT NULL,
  tg_user_id INTEGER NOT NULL,
  role TEXT NOT NULL,
  joined_at INTEGER NOT NULL,
  PRIMARY KEY (orbit_id, tg_user_id)
);
CREATE TABLE slots (
  orbit_id INTEGER NOT NULL,
  slot TEXT NOT NULL,
  token_hash TEXT NOT NULL,
  paired_by INTEGER NOT NULL DEFAULT 0,
  provider TEXT NOT NULL DEFAULT 'spotify',
  paired_at INTEGER,
  revoked_at INTEGER,
  PRIMARY KEY (orbit_id, slot)
);
CREATE TABLE links (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  orbit_a INTEGER NOT NULL,
  orbit_b INTEGER NOT NULL DEFAULT 0,
  state TEXT NOT NULL DEFAULT 'proposed',
  proposed_by INTEGER NOT NULL,
  pending_orbit INTEGER NOT NULL DEFAULT 0,
  code TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
INSERT INTO media(id, created_at, expires_at, status, orbit_id)
VALUES('partial-media', 10, 20, 'ready', 11);
INSERT INTO orbits(id, title, created_at) VALUES(11, 'partial orbit', 10);
INSERT INTO members(orbit_id, tg_user_id, role, joined_at)
VALUES(11, 11001, 'primary', 10);
INSERT INTO slots(orbit_id, slot, token_hash, paired_by, provider)
VALUES(11, 'a', 'partial-hash', 11001, 'apple_music');
INSERT INTO links(id, orbit_a, state, proposed_by, code, created_at)
VALUES(3, 11, 'proposed', 11001, 'PARTIAL1', 10)`); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	assertMigrationColumn(t, store.db, "members", "display_name", true)
	var orbitID int64
	var provider, displayName, code string
	if err := store.db.QueryRow(`SELECT orbit_id FROM media WHERE id = 'partial-media'`).Scan(&orbitID); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT provider FROM slots WHERE orbit_id = 11 AND slot = 'a'`).Scan(&provider); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT display_name FROM members WHERE orbit_id = 11 AND tg_user_id = 11001`).Scan(&displayName); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT code FROM links WHERE id = 3`).Scan(&code); err != nil {
		t.Fatal(err)
	}
	if orbitID != 11 || provider != "apple_music" || displayName != "" || code != "PARTIAL1" {
		t.Fatalf("partial resume orbit=%d provider=%q display=%q code=%q", orbitID, provider, displayName, code)
	}
	assertDatabaseHealthy(t, store)
}

func TestGenerationSkippingMediaReconcileWaitsForLaterSchemas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "generation-skipping-media.db")
	seed, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	mediaID := "m_" + strings.Repeat("g", 26)
	storageKey := "media/v1/" + strings.Repeat("b", 64)
	if _, err := seed.db.Exec(`INSERT INTO media_items(
  id, owner_orbit_id, actor_id, kind, source, title, mime, codec,
  duration_ms, size_bytes, sha256, storage_key, status, revision,
  created_at, updated_at, expires_at, published_at
) VALUES(?, 77001, 77002, 'audio_clip', 'app', 'generation skip',
  'audio/wav', 'pcm_s16le', 1000, 32000, ?, ?, 'ready', 1,
  100, 101, 1000, 101)`, mediaID, strings.Repeat("a", 64), storageKey); err != nil {
		t.Fatal(err)
	}
	if err := seed.Close(); err != nil {
		t.Fatal(err)
	}

	// Model a direct roll-forward from a generation that had media ingest but
	// predated inbox and saved-cue persistence. The active media owner is absent
	// because a rollback binary dissolved the orbit while those additive rows
	// remained intentionally foreign-key-free.
	raw, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(OFF)&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	raw.SetMaxOpenConns(1)
	if _, err := raw.Exec(`DROP TABLE transmission_inbox_items;
DROP TABLE saved_cues`); err != nil {
		raw.Close()
		t.Fatal(err)
	}
	assertMigrationTable(t, raw, "transmission_inbox_items", false)
	assertMigrationTable(t, raw, "saved_cues", false)
	if err := raw.Close(); err != nil {
		t.Fatal(err)
	}

	store, err := Open(path)
	if err != nil {
		t.Fatalf("generation-skipping roll-forward: %v", err)
	}
	item, err := store.GetMediaItem(mediaID)
	if err != nil || item == nil || item.Status != MediaStatusDeleted ||
		item.StorageKey != "" || item.Revision != 2 {
		store.Close()
		t.Fatalf("orphan after roll-forward=%+v err=%v", item, err)
	}
	assertMigrationTable(t, store.db, "transmission_inbox_items", true)
	assertMigrationTable(t, store.db, "saved_cues", true)
	var cleanups int
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM media_storage_operations
WHERE media_id = ? AND kind = 'cleanup'`, mediaID).Scan(&cleanups); err != nil || cleanups != 1 {
		store.Close()
		t.Fatalf("cleanup receipts=%d err=%v", cleanups, err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}

	// The reordered reconciliation remains restart-safe and idempotent.
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if err := store.db.QueryRow(`SELECT COUNT(*) FROM media_storage_operations
WHERE media_id = ? AND kind = 'cleanup'`, mediaID).Scan(&cleanups); err != nil || cleanups != 1 {
		t.Fatalf("cleanup receipts after restart=%d err=%v", cleanups, err)
	}
	item, err = store.GetMediaItem(mediaID)
	if err != nil || item == nil || item.Status != MediaStatusDeleted || item.Revision != 2 {
		t.Fatalf("orphan after restart=%+v err=%v", item, err)
	}
	assertDatabaseHealthy(t, store)
}

func openRawMigrationFixture(t *testing.T, path string) *sql.DB {
	t.Helper()
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(ON)&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	return db
}

func assertMigrationTable(t *testing.T, db *sql.DB, table string, want bool) {
	t.Helper()
	found, err := tableExists(db, table)
	if err != nil {
		t.Fatal(err)
	}
	if found != want {
		t.Fatalf("table %s found=%v want=%v", table, found, want)
	}
}

func assertMigrationColumn(t *testing.T, db *sql.DB, table, column string, want bool) {
	t.Helper()
	found, err := columnExists(db, table, column)
	if err != nil {
		t.Fatal(err)
	}
	if found != want {
		t.Fatalf("column %s.%s found=%v want=%v", table, column, found, want)
	}
}
