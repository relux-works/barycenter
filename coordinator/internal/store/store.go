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

	_ "modernc.org/sqlite"

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
	// WAL keeps concurrent readers (hub LookupToken, /media + /pair handlers,
	// retention sweep) from erroring against the single writer; busy_timeout
	// makes any lock contention wait instead of returning SQLITE_BUSY
	// (architecture review #10 / #1.7). Foreign keys protect only additive
	// tables (legacy tables declare no REFERENCES), and _txlock=immediate makes
	// every database/sql transaction acquire the SQLite writer lock up front.
	dsn := path + "?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)&_txlock=immediate"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("store: open %s: %w", path, err)
	}
	db.SetMaxOpenConns(1) // single writer, keeps SQLITE_BUSY away
	if err := detectBrokenOrbitMigration(db); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: identity migration preflight: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: init schema: %w", err)
	}
	// Pre-media-scoping databases lack media.orbit_id: additive migration so
	// the /media handler can enforce tenant isolation (security review #4.1).
	db.Exec(`ALTER TABLE media ADD COLUMN orbit_id INTEGER NOT NULL DEFAULT 0`)
	s := &Store{
		db:                    db,
		selfServiceOnboarding: opts.SelfServiceOnboarding,
		telegramLinkAttempts:  newTelegramLinkAttemptLimiter(),
	}
	if err := s.initOrbits(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: init orbit schema: %w", err)
	}
	if err := s.initIdentitySchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: init identity schema: %w", err)
	}
	if err := s.initMediaIngestSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: init media ingest schema: %w", err)
	}
	if err := s.initTransmissionSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: init transmission schema: %w", err)
	}
	if err := s.initModerationSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("store: init moderation schema: %w", err)
	}
	if opts.SelfServiceOnboarding {
		if err := s.ReconcileIdentity(); err != nil {
			db.Close()
			return nil, fmt.Errorf("store: reconcile identity: %w", err)
		}
	}
	return s, nil
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
	raw, err := json.Marshal(snap)
	if err != nil {
		return err
	}
	return s.SetSetting(fmt.Sprintf("session_state_%d", orbitID), string(raw))
}

// ClearSession drops a persisted snapshot (a dissolved approach must not
// resurrect its group session under a future link id).
func (s *Store) ClearSession(orbitID int64) error {
	return s.SetSetting(fmt.Sprintf("session_state_%d", orbitID), "")
}

// LoadSession restores the snapshot; a PLAYING/ARMED/LOADING session comes
// back as PAUSED (spec 7.2 restart rule). Returns nil if nothing was saved.
func (s *Store) LoadSession(orbitID int64) (*SessionSnapshot, error) {
	val, err := s.GetSetting(fmt.Sprintf("session_state_%d", orbitID))
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
