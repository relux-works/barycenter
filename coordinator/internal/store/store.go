// Package store is the coordinator's SQLite persistence (spec 5.3):
// elements journal, media registry, settings KV (session snapshot, offsets,
// volumes), events log. Single file, pure-Go driver (modernc) so the linux
// release binary stays CGO-free.
package store

import (
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	sqlite "modernc.org/sqlite"

	"relux.works/duet/coordinator/internal/session"
)

const schema = `
CREATE TABLE IF NOT EXISTS elements (
  id TEXT PRIMARY KEY,
  kind TEXT NOT NULL,
  uri TEXT,
  media_id TEXT,
  title TEXT,
  duration_ms INTEGER,
  requested_by TEXT,
  target TEXT NOT NULL,
  created_at INTEGER NOT NULL,
  played_at INTEGER,
  status TEXT
);
CREATE TABLE IF NOT EXISTS media (
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
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS events (
  ts INTEGER NOT NULL,
  source TEXT NOT NULL,
  type TEXT NOT NULL,
  payload TEXT
);
`

type Store struct {
	db                    *sql.DB
	selfServiceOnboarding bool
	telegramLinkAttempts  *telegramLinkAttemptLimiter
	// testCheckpoint is nil in production. Store-package tests use it to
	// pause real SQLite transactions at deterministic concurrency/fault
	// boundaries without replacing the database with a mock.
	testCheckpoint func(string) error
}

func (s *Store) checkpoint(name string) error {
	if s.testCheckpoint == nil {
		return nil
	}
	return s.testCheckpoint(name)
}

// Options controls additive coordinator features. The zero value deliberately
// preserves the previous coordinator's behavior: identity tables may exist,
// but the actor resolver and dual-write paths do not serve traffic.
type Options struct {
	SelfServiceOnboarding bool
}

func Open(path string) (*Store, error) {
	return OpenWithOptions(path, Options{})
}

// OpenWithOptions opens the store, applies additive migrations, and, when
// enabled, reconciles the transport-neutral identity model before callers can
// serve requests.
func OpenWithOptions(path string, opts Options) (*Store, error) {
	return openWithOptionsAndCheckpoint(path, opts, nil)
}

// openWithOptionsAndCheckpoint is the production open path with a test-only
// fault seam installed before the first DDL transaction. Keeping one path is
// important: migration tests must exercise the same ordering as startup.
func openWithOptionsAndCheckpoint(path string, opts Options, checkpoint func(string) error) (*Store, error) {
	// WAL keeps concurrent readers (hub LookupToken, /media + /pair handlers,
	// retention sweep) from erroring against the single writer; busy_timeout
	// makes any lock contention wait instead of returning SQLITE_BUSY
	// (architecture review #10 / #1.7). Foreign keys protect only additive
	// tables (legacy tables declare no REFERENCES), and _txlock=immediate makes
	// every database/sql transaction acquire the SQLite writer lock up front.
	dsn := path + "?_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	db.SetMaxOpenConns(1) // single writer, keeps SQLITE_BUSY away
	// Apply busy_timeout on the sole connection before WAL negotiation. DSN
	// pragma hooks initialize the connection as one unit, so a concurrent WAL
	// opener could otherwise fail before the timeout became effective.
	for _, pragma := range []string{
		`PRAGMA busy_timeout = 5000`,
		`PRAGMA journal_mode = WAL`,
		`PRAGMA foreign_keys = ON`,
	} {
		if err := execStartupPragma(db, pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("store: initialize connection with %q: %w", pragma, err)
		}
	}
	if err := detectBrokenOrbitMigration(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: identity migration preflight: %w", err)
	}
	s := &Store{
		db:                    db,
		selfServiceOnboarding: opts.SelfServiceOnboarding,
		telegramLinkAttempts:  newTelegramLinkAttemptLimiter(),
		testCheckpoint:        checkpoint,
	}
	if err := s.initLegacySchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: init legacy schema: %w", err)
	}
	if err := s.initOrbits(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: init orbit schema: %w", err)
	}
	if err := s.initIdentitySchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: init identity schema: %w", err)
	}
	if err := s.initAirSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: init Air schema: %w", err)
	}
	if err := s.initMediaIngestSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: init media ingest schema: %w", err)
	}
	if err := s.initStreamTrackSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: init stream track schema: %w", err)
	}
	if err := s.initStreamAccountingSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: init stream accounting schema: %w", err)
	}
	if err := s.initTransmissionSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: init transmission schema: %w", err)
	}
	if err := s.initContentPolicySchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: init content policy schema: %w", err)
	}
	if err := s.initModerationSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: init moderation schema: %w", err)
	}
	if err := s.initSavedCueSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: init saved cue schema: %w", err)
	}
	if err := s.ReconcileSavedCues(time.Now().UnixMilli()); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: reconcile saved cues: %w", err)
	}
	if err := s.initAutomationSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: init automation schema: %w", err)
	}
	if _, err := s.ReconcileAutomationExecutionLeases(time.Now().UnixMilli()); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: reconcile automation execution leases: %w", err)
	}
	if _, err := s.ReconcileStreamAccounting(
		time.Now().UnixMilli(), StreamAccountingDefaultStaleAfter,
	); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: reconcile stream accounting: %w", err)
	}
	if opts.SelfServiceOnboarding {
		if err := s.ReconcileIdentity(); err != nil {
			db.Close()
			return nil, fmt.Errorf("store: reconcile identity: %w", err)
		}
	}
	return s, nil
}

func execStartupPragma(db *sql.DB, pragma string) error {
	deadline := time.Now().Add(5 * time.Second)
	for {
		_, err := db.Exec(pragma)
		if err == nil {
			return nil
		}
		var sqliteErr *sqlite.Error
		if !errors.As(err, &sqliteErr) || sqliteErr.Code()&0xff != 5 || time.Now().After(deadline) {
			return err
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// initLegacySchema keeps the original elements/media/settings/events model
// writable by rollback binaries while installing its one additive media
// ownership column atomically. Older startup code ignored ALTER failures and
// could continue with a partially usable database.
func (s *Store) initLegacySchema() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(schema); err != nil {
		return err
	}
	hasOrbitID, err := txColumnExists(tx, "media", "orbit_id")
	if err != nil {
		return err
	}
	if !hasOrbitID {
		if _, err := tx.Exec(`ALTER TABLE media
ADD COLUMN orbit_id INTEGER NOT NULL DEFAULT 0`); err != nil {
			return err
		}
	}
	if err := s.checkpoint("legacy_ddl_before_commit"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) Close() error { return s.db.Close() }

// --- Session snapshot (spec 7.2: survives coordinator restart) ---

// SessionSnapshot is the persisted shape of the FSM state.
type SessionSnapshot struct {
	Mode            session.Mode      `json:"mode"`
	State           session.State     `json:"state"`
	Current         *session.Element  `json:"current"`
	SavedPositionMS int64             `json:"saved_position_ms"`
	Queue           []session.Element `json:"queue"`
	Playlist        *session.Playlist `json:"playlist,omitempty"` // U10 base layer
}

func (s *Store) SaveSession(orbitID int64, snap SessionSnapshot) error {
	return s.saveSessionKey(fmt.Sprintf("session_state_%d", orbitID), snap)
}

// SaveAirSession persists a shared runtime by stable Air ID. It deliberately
// does not reuse the legacy negative-link key: link IDs are migration input,
// never Phase 2 runtime ownership.
func (s *Store) SaveAirSession(airID string, snap SessionSnapshot) error {
	return s.saveSessionKey("session_state_"+airID, snap)
}

func (s *Store) saveSessionKey(key string, snap SessionSnapshot) error {
	raw, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	return s.SetSetting(key, string(raw))
}

// ClearSession drops a persisted snapshot (a dissolved approach must not
// resurrect its group session under a future link id).
func (s *Store) ClearSession(orbitID int64) error {
	return s.SetSetting(fmt.Sprintf("session_state_%d", orbitID), "")
}

func (s *Store) ClearAirSession(airID string) error {
	return s.SetSetting("session_state_"+airID, "")
}

// LoadSession restores the snapshot; a PLAYING/ARMED/LOADING session comes
// back as PAUSED (spec 7.2 restart rule). Returns nil if nothing was saved.
func (s *Store) LoadSession(orbitID int64) (*SessionSnapshot, error) {
	return s.loadSessionKey(fmt.Sprintf("session_state_%d", orbitID))
}

func (s *Store) LoadAirSession(airID string) (*SessionSnapshot, error) {
	return s.loadSessionKey("session_state_" + airID)
}

func (s *Store) loadSessionKey(key string) (*SessionSnapshot, error) {
	val, err := s.GetSetting(key)
	if err != nil {
		return nil, err
	}
	if val == "" {
		return nil, nil
	}
	var snap SessionSnapshot
	if err := json.Unmarshal([]byte(val), &snap); err != nil {
		return nil, fmt.Errorf("store: session_state corrupt: %w", err)
	}
	switch snap.State {
	case session.StatePlaying, session.StateArmed, session.StateLoading, session.StateVoice, session.StateDegraded:
		snap.State = session.StatePaused
	}
	return &snap, nil
}

// --- Settings KV ---

func (s *Store) SetSetting(key, value string) error {
	_, err := s.db.Exec(
		`INSERT INTO settings(key, value) VALUES(?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value)
	return err
}

// GetSetting returns "" when the key is absent.
func (s *Store) GetSetting(key string) (string, error) {
	var v string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

// --- Elements journal ---

func (s *Store) InsertElement(el session.Element) error {
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO elements(id, kind, uri, media_id, title, duration_ms, requested_by, target, created_at)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		el.ID, string(el.Kind), el.URI, el.MediaID, el.Title, el.DurationMS, string(el.RequestedBy), el.Target, el.CreatedAt)
	return err
}

func (s *Store) MarkElementDone(id, status string, playedAt int64) error {
	_, err := s.db.Exec(`UPDATE elements SET status = ?, played_at = ? WHERE id = ?`, status, playedAt, id)
	return err
}

// --- Media registry (spec 10) ---

type MediaRecord struct {
	ID           string
	TGFileID     string
	DurationMS   int64
	PathWAV      string
	LoudnormJSON string
	CreatedAt    int64
	ExpiresAt    int64
	Status       string // processing | ready | failed
	OrbitID      int64  // owning tenant (security review #4.1: /media scoping)
}

func (s *Store) InsertMedia(m MediaRecord) error {
	_, err := s.db.Exec(
		`INSERT INTO media(id, tg_file_id, duration_ms, path_wav, loudnorm_json, created_at, expires_at, status, orbit_id)
		 VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		m.ID, m.TGFileID, m.DurationMS, m.PathWAV, m.LoudnormJSON, m.CreatedAt, m.ExpiresAt, m.Status, m.OrbitID)
	return err
}

func (s *Store) UpdateMedia(m MediaRecord) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var genericStatus MediaItemStatus
	err = tx.QueryRow(`SELECT i.status
FROM media_legacy_wav_links l
JOIN media_items i ON i.id = l.media_id
WHERE l.legacy_media_id = ?`, m.ID).Scan(&genericStatus)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	linked := err == nil
	if linked {
		allowed := false
		legacyStates := ""
		switch m.Status {
		case "processing":
			allowed = genericStatus == MediaStatusProcessing
			legacyStates = "'processing'"
		case "ready":
			allowed = genericStatus == MediaStatusReady
			legacyStates = "'processing', 'ready'"
		case "failed":
			allowed = genericStatus == MediaStatusFailed
			legacyStates = "'processing', 'failed'"
		case "deleted":
			allowed = genericStatus == MediaStatusDeleted || genericStatus == MediaStatusExpired
			legacyStates = "'processing', 'ready', 'failed', 'deleted', 'expired'"
		case "expired":
			allowed = genericStatus == MediaStatusExpired
			legacyStates = "'processing', 'ready', 'failed', 'expired'"
		}
		if !allowed {
			return ErrMediaStateConflict
		}
		result, err := tx.Exec(
			`UPDATE media SET duration_ms = ?, path_wav = ?, loudnorm_json = ?, status = ?
WHERE id = ? AND status IN (`+legacyStates+`)`,
			m.DurationMS, m.PathWAV, m.LoudnormJSON, m.Status, m.ID,
		)
		if err != nil {
			return err
		}
		if changed, err := result.RowsAffected(); err != nil || changed != 1 {
			if err != nil {
				return err
			}
			return ErrMediaStateConflict
		}
	} else if _, err := tx.Exec(
		`UPDATE media SET duration_ms = ?, path_wav = ?, loudnorm_json = ?, status = ? WHERE id = ?`,
		m.DurationMS, m.PathWAV, m.LoudnormJSON, m.Status, m.ID,
	); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) GetMedia(id string) (*MediaRecord, error) {
	var m MediaRecord
	err := s.db.QueryRow(
		`SELECT id, tg_file_id, duration_ms, path_wav, loudnorm_json, created_at, expires_at, status, orbit_id
		 FROM media WHERE id = ?`, id).
		Scan(&m.ID, &m.TGFileID, &m.DurationMS, &m.PathWAV, &m.LoudnormJSON, &m.CreatedAt, &m.ExpiresAt, &m.Status, &m.OrbitID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// GetMediaForOrbit returns the media record only when the requesting orbit
// may hear it: the owner, or an orbit currently linked to the owner by an
// active approach (group voice) — the tenant-isolation gate for GET /media
// (security review #4.1, link-aware since L7: the policy lives HERE, not
// inline in the handler, so it cannot drift). GetMedia stays unscoped for
// the retention sweep.
func (s *Store) GetMediaForOrbit(id string, orbitID int64) (*MediaRecord, error) {
	m, err := s.GetMedia(id)
	if err != nil || m == nil {
		return nil, err
	}
	// Once a legacy WAV is linked into the generic ingest model, that model is
	// the revocation authority. This defensive read-side join keeps an old or
	// racing compatibility write from serving a failed/deleted/expired item.
	linked, err := s.MediaItemForLegacyWAV(id)
	if err != nil {
		return nil, err
	}
	if linked != nil && linked.Status != MediaStatusReady {
		return nil, nil
	}
	if m.OrbitID != orbitID {
		if _, other, ok, err := s.ActiveLink(orbitID); err != nil || !ok || other != m.OrbitID {
			return nil, err // not yours and not linked: indistinguishable from missing
		}
	}
	return m, nil
}

// ExpiredMedia lists media past expires_at for the daily retention sweep.
func (s *Store) ExpiredMedia(now int64) ([]MediaRecord, error) {
	// Linked rows are owned by the generic lifecycle worker, which validates
	// storage roots, fsyncs the unlink and acknowledges a durable receipt. The
	// legacy daily sweeper remains only for rows that predate generic ingest;
	// allowing both workers to touch a linked path would bypass those safety
	// and retry guarantees.
	rows, err := s.db.Query(`SELECT id, path_wav
FROM media
WHERE expires_at < ? AND status != 'deleted'
  AND NOT EXISTS (
    SELECT 1 FROM media_legacy_wav_links l WHERE l.legacy_media_id = media.id
  )`, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []MediaRecord
	for rows.Next() {
		var m MediaRecord
		if err := rows.Scan(&m.ID, &m.PathWAV); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) MarkMediaDeleted(id string) error {
	_, err := s.db.Exec(`UPDATE media SET status = 'deleted' WHERE id = ?`, id)
	return err
}

// --- Events log (spec 5.3, debugging) ---

func (s *Store) LogEvent(source, eventType string, payload any) {
	raw, _ := json.Marshal(payload)
	s.db.Exec(`INSERT INTO events(ts, source, type, payload) VALUES(?, ?, ?, ?)`,
		time.Now().UnixMilli(), source, eventType, string(raw))
}
