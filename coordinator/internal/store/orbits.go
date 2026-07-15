// Multi-tenant layer (v2.1 M1, docs/v2-multitenant-design.md): orbits,
// members with star-system roles, node slots with hashed tokens, one-time
// invites (pairing codes and member invite links).
package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

const orbitSchema = `
CREATE TABLE IF NOT EXISTS orbits (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  takeover_policy TEXT NOT NULL DEFAULT 'user',
  voice_default TEXT NOT NULL DEFAULT 'personal',
  max_pulsars INTEGER NOT NULL DEFAULT 5,
  max_members INTEGER NOT NULL DEFAULT 10,
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS members (
  orbit_id INTEGER NOT NULL,
  tg_user_id INTEGER NOT NULL,
  role TEXT NOT NULL, -- primary | companion | satellite
  joined_at INTEGER NOT NULL,
  display_name TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (orbit_id, tg_user_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS members_user ON members(tg_user_id); -- MVP: one orbit per user
CREATE TABLE IF NOT EXISTS slots (
  orbit_id INTEGER NOT NULL,
  slot TEXT NOT NULL,
  token_hash TEXT NOT NULL,
  paired_by INTEGER NOT NULL DEFAULT 0,
  provider TEXT NOT NULL DEFAULT 'spotify',
  paired_at INTEGER,
  revoked_at INTEGER,
  PRIMARY KEY (orbit_id, slot)
);
CREATE UNIQUE INDEX IF NOT EXISTS slots_token ON slots(token_hash);
CREATE TABLE IF NOT EXISTS tracks (
  ctid TEXT PRIMARY KEY,
  title TEXT NOT NULL DEFAULT '',
  artists TEXT NOT NULL DEFAULT '[]',
  duration_ms INTEGER NOT NULL DEFAULT 0,
  isrc TEXT NOT NULL DEFAULT '',
  origin_provider TEXT NOT NULL,
  origin_ref TEXT NOT NULL,
  resolve_method TEXT NOT NULL DEFAULT 'same',
  resolve_score REAL NOT NULL DEFAULT 1,
  resolved_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS track_refs (
  ctid TEXT NOT NULL,
  provider TEXT NOT NULL,
  ref TEXT NOT NULL,
  duration_ms INTEGER NOT NULL DEFAULT 0,
  PRIMARY KEY (ctid, provider)
);
CREATE UNIQUE INDEX IF NOT EXISTS track_refs_by_ref ON track_refs(provider, ref);
CREATE TABLE IF NOT EXISTS availability (
  orbit_id INTEGER NOT NULL,
  slot TEXT NOT NULL,
  provider TEXT NOT NULL,
  ref TEXT NOT NULL,
  ok INTEGER,
  checked_at INTEGER NOT NULL,
  PRIMARY KEY (orbit_id, slot, provider, ref)
);
CREATE TABLE IF NOT EXISTS invites (
  code TEXT PRIMARY KEY,
  orbit_id INTEGER NOT NULL,
  kind TEXT NOT NULL, -- pair | member
  issued_by INTEGER NOT NULL,
  expires_at INTEGER NOT NULL,
  used_at INTEGER
);
`

type Orbit struct {
	ID             int64
	Title          string
	TakeoverPolicy string
	VoiceDefault   string
	MaxPulsars     int
	MaxMembers     int
}

type Member struct {
	OrbitID     int64
	TGUserID    int64
	Role        string // primary | companion | satellite
	DisplayName string // Telegram first name, refreshed on every command
}

var ErrLimit = errors.New("orbit limit reached")

func hashToken(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

// Pairing codes are human-typable: 8 chars from an unambiguous alphabet.
func randomCode() string {
	const alphabet = "ABCDEFGHJKMNPQRSTVWXYZ23456789"
	b := make([]byte, 8)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	out := make([]byte, 8)
	for i, v := range b {
		out[i] = alphabet[int(v)%len(alphabet)]
	}
	return string(out)
}

func (s *Store) initOrbits() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.Exec(orbitSchema); err != nil {
		return err
	}
	// Approaches between barycenters (design §12 L1): additive table.
	if _, err := tx.Exec(linksSchema); err != nil {
		return err
	}
	// Pre-provider databases lack slots.provider: additive migration.
	hasProvider, err := txColumnExists(tx, "slots", "provider")
	if err != nil {
		return err
	}
	if !hasProvider {
		if _, err := tx.Exec(`ALTER TABLE slots
ADD COLUMN provider TEXT NOT NULL DEFAULT 'spotify'`); err != nil {
			return err
		}
	}
	// Pre-M1.5 databases lack members.display_name: additive migration so
	// /home can render members by name instead of raw tg_user_id (bot-ux #4).
	hasDisplayName, err := txColumnExists(tx, "members", "display_name")
	if err != nil {
		return err
	}
	if !hasDisplayName {
		if _, err := tx.Exec(`ALTER TABLE members
ADD COLUMN display_name TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	// Do not run the global FK check here. A rollback binary may have removed
	// an orbit while leaving additive identity children; initIdentitySchema
	// owns the narrowly bounded cleanup and the subsequent global check.
	if err := s.checkpoint("orbit_ddl_before_commit"); err != nil {
		return err
	}
	return tx.Commit()
}

// --- Provider layer (spec-providers §2, behind DUET_PROVIDERS) ---

type Track struct {
	CTID          string
	Title         string
	Artists       []string
	DurationMS    int64
	ISRC          string
	OriginProv    string
	OriginRef     string
	ResolveMethod string
	ResolveScore  float64
}

func (s *Store) UpsertTrack(t Track, refs map[string]struct {
	Ref        string
	DurationMS int64
}) error {
	artists, _ := json.Marshal(t.Artists)
	if _, err := s.db.Exec(`INSERT OR REPLACE INTO tracks(ctid, title, artists, duration_ms, isrc, origin_provider, origin_ref, resolve_method, resolve_score, resolved_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)`,
		t.CTID, t.Title, string(artists), t.DurationMS, t.ISRC, t.OriginProv, t.OriginRef, t.ResolveMethod, t.ResolveScore, time.Now().UnixMilli()); err != nil {
		return err
	}
	for prov, r := range refs {
		if _, err := s.db.Exec(`INSERT OR REPLACE INTO track_refs(ctid, provider, ref, duration_ms) VALUES(?,?,?,?)`,
			t.CTID, prov, r.Ref, r.DurationMS); err != nil {
			return err
		}
	}
	return nil
}

// CTIDByRef finds an already-resolved canonical track by any provider ref.
func (s *Store) CTIDByRef(provider, ref string) (string, error) {
	var ctid string
	err := s.db.QueryRow(`SELECT ctid FROM track_refs WHERE provider = ? AND ref = ?`, provider, ref).Scan(&ctid)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return ctid, err
}

// TrackRef returns the ref of ctid for a provider ("" if unresolved).
func (s *Store) TrackRef(ctid, provider string) (ref string, durationMS int64, err error) {
	err = s.db.QueryRow(`SELECT ref, duration_ms FROM track_refs WHERE ctid = ? AND provider = ?`, ctid, provider).Scan(&ref, &durationMS)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, nil
	}
	return
}

// TrackByCTID returns cached canonical metadata, or nil when the id is not
// known. Bot rendering uses it so a forever-cache hit keeps the human title
// instead of falling back to a provider URI.
func (s *Store) TrackByCTID(ctid string) (*Track, error) {
	var t Track
	var artists string
	err := s.db.QueryRow(`SELECT ctid, title, artists, duration_ms, isrc, origin_provider, origin_ref, resolve_method, resolve_score
		FROM tracks WHERE ctid = ?`, ctid).Scan(
		&t.CTID, &t.Title, &artists, &t.DurationMS, &t.ISRC,
		&t.OriginProv, &t.OriginRef, &t.ResolveMethod, &t.ResolveScore,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal([]byte(artists), &t.Artists); err != nil {
		return nil, err
	}
	return &t, nil
}

func (s *Store) SetSlotProvider(orbitID int64, slot, provider string) error {
	_, err := s.db.Exec(`UPDATE slots SET provider = ? WHERE orbit_id = ? AND slot = ? AND revoked_at IS NULL`, provider, orbitID, slot)
	return err
}

// SlotProviders maps active slots to their provider for an orbit.
func (s *Store) SlotProviders(orbitID int64) (map[string]string, error) {
	rows, err := s.db.Query(`SELECT slot, provider FROM slots WHERE orbit_id = ? AND revoked_at IS NULL`, orbitID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var sl, p string
		if err := rows.Scan(&sl, &p); err != nil {
			return nil, err
		}
		out[sl] = p
	}
	return out, rows.Err()
}

// Availability cache (TTL enforced by caller; ok NULL = unknown).
func (s *Store) SetAvailability(orbitID int64, slot, provider, ref string, ok bool) error {
	v := 0
	if ok {
		v = 1
	}
	_, err := s.db.Exec(`INSERT OR REPLACE INTO availability(orbit_id, slot, provider, ref, ok, checked_at) VALUES(?,?,?,?,?,?)`,
		orbitID, slot, provider, ref, v, time.Now().UnixMilli())
	return err
}

// Availability returns (ok, known); stale rows (older than ttlMS) = unknown.
func (s *Store) Availability(orbitID int64, slot, provider, ref string, ttlMS int64) (bool, bool, error) {
	var ok int
	var at int64
	err := s.db.QueryRow(`SELECT ok, checked_at FROM availability WHERE orbit_id = ? AND slot = ? AND provider = ? AND ref = ?`,
		orbitID, slot, provider, ref).Scan(&ok, &at)
	if errors.Is(err, sql.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, err
	}
	if time.Now().UnixMilli()-at > ttlMS {
		return false, false, nil
	}
	return ok == 1, true, nil
}

// CreateOrbit makes a new orbit with its creator as primary.
func (s *Store) CreateOrbit(title string, creator int64) (*Orbit, error) {
	now := time.Now().UnixMilli()
	if s.selfServiceOnboarding {
		tx, err := s.db.Begin()
		if err != nil {
			return nil, err
		}
		defer tx.Rollback()
		res, err := tx.Exec(`INSERT INTO orbits(title, created_at) VALUES(?, ?)`, title, now)
		if err != nil {
			return nil, err
		}
		id, err := res.LastInsertId()
		if err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`INSERT INTO members(orbit_id, tg_user_id, role, joined_at) VALUES(?, ?, 'primary', ?)`,
			id, creator, now); err != nil {
			return nil, err
		}
		if err := s.reconcileIdentityTx(tx, now); err != nil {
			return nil, err
		}
		if err := tx.Commit(); err != nil {
			return nil, err
		}
		return s.GetOrbit(id)
	}
	res, err := s.db.Exec(`INSERT INTO orbits(title, created_at) VALUES(?, ?)`, title, now)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	if _, err := s.db.Exec(`INSERT INTO members(orbit_id, tg_user_id, role, joined_at) VALUES(?, ?, 'primary', ?)`,
		id, creator, now); err != nil {
		return nil, err
	}
	return s.GetOrbit(id)
}

func (s *Store) GetOrbit(id int64) (*Orbit, error) {
	o := &Orbit{}
	err := s.db.QueryRow(`SELECT id, title, takeover_policy, voice_default, max_pulsars, max_members FROM orbits WHERE id = ?`, id).
		Scan(&o.ID, &o.Title, &o.TakeoverPolicy, &o.VoiceDefault, &o.MaxPulsars, &o.MaxMembers)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return o, err
}

func (s *Store) SetOrbitSetting(orbitID int64, column, value string) error {
	switch column {
	case "takeover_policy", "voice_default":
	default:
		return fmt.Errorf("unknown orbit setting %q", column)
	}
	_, err := s.db.Exec(`UPDATE orbits SET `+column+` = ? WHERE id = ?`, value, orbitID)
	return err
}

// MemberOf resolves a telegram user to their orbit membership (MVP: max one).
func (s *Store) MemberOf(tgUserID int64) (*Member, error) {
	m := &Member{}
	err := s.db.QueryRow(`SELECT orbit_id, tg_user_id, role, display_name FROM members WHERE tg_user_id = ?`, tgUserID).
		Scan(&m.OrbitID, &m.TGUserID, &m.Role, &m.DisplayName)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return m, err
}

func (s *Store) Members(orbitID int64) ([]Member, error) {
	rows, err := s.db.Query(`SELECT orbit_id, tg_user_id, role, display_name FROM members WHERE orbit_id = ? ORDER BY joined_at`, orbitID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.OrbitID, &m.TGUserID, &m.Role, &m.DisplayName); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

// SetMemberName refreshes a member's display name (Telegram first name). The
// loop calls it on every command so /home and /make_primary can use names
// instead of raw ids (bot-ux #4/#5). A no-op for empty names or non-members.
func (s *Store) SetMemberName(orbitID, tgUserID int64, name string) error {
	if strings.TrimSpace(name) == "" {
		return nil
	}
	if s.selfServiceOnboarding {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err := tx.Exec(`UPDATE members SET display_name = ? WHERE orbit_id = ? AND tg_user_id = ?`, name, orbitID, tgUserID); err != nil {
			return err
		}
		if err := s.reconcileIdentityTx(tx, time.Now().UnixMilli()); err != nil {
			return err
		}
		return tx.Commit()
	}
	_, err := s.db.Exec(`UPDATE members SET display_name = ? WHERE orbit_id = ? AND tg_user_id = ?`, name, orbitID, tgUserID)
	return err
}

// MemberByName resolves a member by display name (case-insensitive; a leading
// @ is ignored). Returns 0 when there is no unique match (none or ambiguous).
// Matching is done in Go: SQLite's lower() folds ASCII only, and names are
// Cyrillic (strings.EqualFold handles Unicode).
func (s *Store) MemberByName(orbitID int64, name string) (int64, error) {
	name = strings.TrimPrefix(strings.TrimSpace(name), "@")
	if name == "" {
		return 0, nil
	}
	members, err := s.Members(orbitID)
	if err != nil {
		return 0, err
	}
	var id int64
	matches := 0
	for _, m := range members {
		if m.DisplayName != "" && strings.EqualFold(m.DisplayName, name) {
			id = m.TGUserID
			matches++
		}
	}
	if matches != 1 {
		return 0, nil // no match or ambiguous
	}
	return id, nil
}

func (s *Store) AddMember(orbitID int64, tgUserID int64, role string) error {
	if s.selfServiceOnboarding {
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		var count, maxM int
		var status string
		if err := tx.QueryRow(`SELECT COUNT(*) FROM members WHERE orbit_id = ?`, orbitID).Scan(&count); err != nil {
			return err
		}
		if err := tx.QueryRow(`SELECT max_members, status FROM orbits WHERE id = ?`, orbitID).Scan(&maxM, &status); err != nil {
			return err
		}
		if status != "active" {
			return ErrOrbitDisabled
		}
		if count >= maxM {
			return ErrLimit
		}
		if err := s.checkpoint("add_member_after_snapshot"); err != nil {
			return err
		}
		if _, err := tx.Exec(`INSERT INTO members(orbit_id, tg_user_id, role, joined_at) VALUES(?, ?, ?, ?)`,
			orbitID, tgUserID, role, time.Now().UnixMilli()); err != nil {
			return err
		}
		if err := s.reconcileIdentityTx(tx, time.Now().UnixMilli()); err != nil {
			return err
		}
		return tx.Commit()
	}
	var count, maxM int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM members WHERE orbit_id = ?`, orbitID).Scan(&count); err != nil {
		return err
	}
	if err := s.db.QueryRow(`SELECT max_members FROM orbits WHERE id = ?`, orbitID).Scan(&maxM); err != nil {
		return err
	}
	if count >= maxM {
		return ErrLimit
	}
	_, err := s.db.Exec(`INSERT INTO members(orbit_id, tg_user_id, role, joined_at) VALUES(?, ?, ?, ?)`,
		orbitID, tgUserID, role, time.Now().UnixMilli())
	return err
}

// TransferPrimary makes newPrimary the primary; the old primary becomes companion.
func (s *Store) TransferPrimary(orbitID, newPrimary int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := s.checkpoint("transfer_primary_after_begin"); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE members SET role = 'companion' WHERE orbit_id = ? AND role = 'primary'`, orbitID); err != nil {
		return err
	}
	res, err := tx.Exec(`UPDATE members SET role = 'primary' WHERE orbit_id = ? AND tg_user_id = ?`, orbitID, newPrimary)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return fmt.Errorf("user %d is not a member of orbit %d", newPrimary, orbitID)
	}
	if s.selfServiceOnboarding {
		if err := s.reconcileIdentityTx(tx, time.Now().UnixMilli()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// LeaveOrbit removes a member from their orbit (design §2: a member may leave).
// If they were the last member the orbit is dissolved (dissolved=true). If they
// were the primary and others remain, the earliest-joined member is promoted
// (promoted = their tg id). The leaver's own slots are revoked so their home
// drops out of the air.
func (s *Store) LeaveOrbit(orbitID, tgUserID int64) (dissolved bool, promoted int64, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, 0, err
	}
	defer tx.Rollback()
	var role, status string
	var count int
	err = tx.QueryRow(`SELECT m.role, o.status,
       (SELECT COUNT(*) FROM members all_members WHERE all_members.orbit_id = m.orbit_id)
FROM members m JOIN orbits o ON o.id = m.orbit_id
WHERE m.orbit_id = ? AND m.tg_user_id = ?`, orbitID, tgUserID).Scan(&role, &status, &count)
	if errors.Is(err, sql.ErrNoRows) {
		return false, 0, fmt.Errorf("user %d is not a member of orbit %d", tgUserID, orbitID)
	}
	if err != nil {
		return false, 0, err
	}
	if status != "active" && status != "disabled" {
		return false, 0, fmt.Errorf("orbit %d has invalid status %q", orbitID, status)
	}
	if s.selfServiceOnboarding {
		var primaries int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM members WHERE orbit_id = ? AND role = 'primary'`, orbitID).Scan(&primaries); err != nil {
			return false, 0, err
		}
		if primaries != 1 {
			return false, 0, fmt.Errorf("active orbit %d has %d primary members", orbitID, primaries)
		}
	}
	if err := s.checkpoint("leave_orbit_after_snapshot"); err != nil {
		return false, 0, err
	}
	if count <= 1 {
		if err := s.deleteOrbitTx(tx, orbitID); err != nil {
			return false, 0, err
		}
		if err := tx.Commit(); err != nil {
			return false, 0, err
		}
		return true, 0, nil
	}
	if role == "primary" {
		var newP int64
		// L1: companions before satellites (a listener must not be crowned
		// just for joining early); tg_user_id breaks joined_at ties so the
		// promotion is fully deterministic.
		if err := tx.QueryRow(`SELECT tg_user_id FROM members WHERE orbit_id = ? AND tg_user_id != ?
			ORDER BY CASE WHEN role = 'satellite' THEN 1 ELSE 0 END, joined_at, tg_user_id LIMIT 1`,
			orbitID, tgUserID).Scan(&newP); err != nil {
			return false, 0, err
		}
		if _, err := tx.Exec(`UPDATE members SET role = 'primary' WHERE orbit_id = ? AND tg_user_id = ?`, orbitID, newP); err != nil {
			return false, 0, err
		}
		promoted = newP
	}
	if _, err := tx.Exec(`DELETE FROM members WHERE orbit_id = ? AND tg_user_id = ?`, orbitID, tgUserID); err != nil {
		return false, 0, err
	}
	if _, err := tx.Exec(`UPDATE slots SET revoked_at = ? WHERE orbit_id = ? AND paired_by = ? AND revoked_at IS NULL`,
		time.Now().UnixMilli(), orbitID, tgUserID); err != nil {
		return false, 0, err
	}
	if s.selfServiceOnboarding {
		if err := s.reconcileIdentityTx(tx, time.Now().UnixMilli()); err != nil {
			return false, 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, 0, err
	}
	return false, promoted, nil
}

// DeleteOrbit removes an orbit and every row scoped to it: members, slots,
// invites, availability, links (design §2: primary may dissolve the
// barycenter). The caller clears the session snapshot and in-memory state.
func (s *Store) DeleteOrbit(orbitID int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := s.deleteOrbitTx(tx, orbitID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) deleteOrbitTx(tx *sql.Tx, orbitID int64) error {
	now := time.Now().UnixMilli()
	if err := revokeOrbitMediaTx(tx, orbitID, now); err != nil {
		return err
	}
	// Additive children are removed even when the flag is off. This lets the
	// current binary run legacy mutations with foreign_keys enabled and lets a
	// previous binary safely ignore the same rows with foreign_keys disabled.
	if _, err := tx.Exec(`UPDATE actors SET revoked_at = COALESCE(revoked_at, ?)
WHERE id IN (SELECT actor_id FROM installation_credentials WHERE slot_orbit_id = ?)`,
		now, orbitID); err != nil {
		return err
	}
	stmts := []string{
		`DELETE FROM device_invites WHERE orbit_id = ?`,
		`DELETE FROM telegram_link_codes WHERE orbit_id = ?`,
		`DELETE FROM installation_credentials WHERE slot_orbit_id = ?`,
		`DELETE FROM memberships WHERE orbit_id = ?`,
		`DELETE FROM rollback_projections WHERE orbit_id = ?`,
		`DELETE FROM members WHERE orbit_id = ?`,
		`DELETE FROM slots WHERE orbit_id = ?`,
		`DELETE FROM invites WHERE orbit_id = ?`,
		`DELETE FROM availability WHERE orbit_id = ?`,
		`DELETE FROM links WHERE orbit_a = ? OR orbit_b = ?`,
		`DELETE FROM orbits WHERE id = ?`,
	}
	for _, q := range stmts {
		args := []any{orbitID}
		if strings.Contains(q, "orbit_a") {
			args = []any{orbitID, orbitID}
		}
		if _, err := tx.Exec(q, args...); err != nil {
			return err
		}
	}
	return nil
}

// --- Invites (member links) and pairing codes ---

// NewInvite issues a member-invite code (deep-link payload), TTL 48h.
func (s *Store) NewInvite(orbitID, issuedBy int64) (string, error) {
	code := "inv" + randomHex(8)
	_, err := s.db.Exec(`INSERT INTO invites(code, orbit_id, kind, issued_by, expires_at) VALUES(?, ?, 'member', ?, ?)`,
		code, orbitID, issuedBy, time.Now().Add(48*time.Hour).UnixMilli())
	return code, err
}

// NewPairCode issues a node-pairing code, TTL 5 min (spec: one-time).
func (s *Store) NewPairCode(orbitID, issuedBy int64) (string, error) {
	code := randomCode()
	_, err := s.db.Exec(`INSERT INTO invites(code, orbit_id, kind, issued_by, expires_at) VALUES(?, ?, 'pair', ?, ?)`,
		code, orbitID, issuedBy, time.Now().Add(5*time.Minute).UnixMilli())
	return code, err
}

// ConsumeInvite validates and burns a code of the given kind.
// Returns the orbit id (0 when invalid/expired/used) and who issued it.
func (s *Store) ConsumeInvite(code, kind string) (int64, int64, error) {
	code = strings.TrimSpace(code)
	if s.selfServiceOnboarding {
		return s.consumeInviteFeatureOn(code, kind)
	}
	var orbitID, issuedBy int64
	var expires int64
	var used sql.NullInt64
	err := s.db.QueryRow(`SELECT orbit_id, issued_by, expires_at, used_at FROM invites WHERE code = ? AND kind = ?`,
		code, kind).Scan(&orbitID, &issuedBy, &expires, &used)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	now := time.Now().UnixMilli()
	if used.Valid || now > expires {
		return 0, 0, nil
	}
	// Guarded burn (L2): two racing consumers both passed the SELECT above —
	// the UPDATE's used_at IS NULL predicate lets exactly one of them win,
	// so one code can never mint two slots.
	res, err := s.db.Exec(`UPDATE invites SET used_at = ? WHERE code = ? AND used_at IS NULL`, now, code)
	if err != nil {
		return 0, 0, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return 0, 0, nil
	}
	return orbitID, issuedBy, nil
}

func (s *Store) consumeInviteFeatureOn(code, kind string) (int64, int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, 0, err
	}
	defer tx.Rollback()
	var orbitID, issuedBy int64
	var expires int64
	var used sql.NullInt64
	var status string
	err = tx.QueryRow(`SELECT i.orbit_id, i.issued_by, i.expires_at, i.used_at, o.status
FROM invites i JOIN orbits o ON o.id = i.orbit_id
WHERE i.code = ? AND i.kind = ?`, code, kind).Scan(&orbitID, &issuedBy, &expires, &used, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	if status != "active" {
		return 0, 0, ErrOrbitDisabled
	}
	now := time.Now().UnixMilli()
	if used.Valid || now > expires {
		return 0, 0, nil
	}
	res, err := tx.Exec(`UPDATE invites SET used_at = ? WHERE code = ? AND used_at IS NULL`, now, code)
	if err != nil {
		return 0, 0, err
	}
	if n, err := res.RowsAffected(); err != nil {
		return 0, 0, err
	} else if n != 1 {
		return 0, 0, nil
	}
	if err := tx.Commit(); err != nil {
		return 0, 0, err
	}
	return orbitID, issuedBy, nil
}

// --- Slots & node tokens ---

// PairSlot allocates the next free slot in the orbit and mints its token.
// Returns (slot, plaintext token). The plaintext is never stored.
// pairedBy records which member owns the node (used for "my home" defaults).
func (s *Store) PairSlot(orbitID int64, pairedBy int64) (string, string, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()
	var used int
	var maxP int
	var status string
	if err := tx.QueryRow(`SELECT COUNT(*) FROM slots WHERE orbit_id = ? AND revoked_at IS NULL`, orbitID).Scan(&used); err != nil {
		return "", "", err
	}
	if err := tx.QueryRow(`SELECT max_pulsars, status FROM orbits WHERE id = ?`, orbitID).Scan(&maxP, &status); err != nil {
		return "", "", err
	}
	if s.selfServiceOnboarding && status != "active" {
		return "", "", ErrOrbitDisabled
	}
	if used >= maxP {
		return "", "", ErrLimit
	}
	// Slots are letters a, b, c… — reuse revoked letters last.
	slot := ""
	for i := 0; i < maxP; i++ {
		candidate := string(rune('a' + i))
		var n int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM slots WHERE orbit_id = ? AND slot = ? AND revoked_at IS NULL`,
			orbitID, candidate).Scan(&n); err != nil {
			return "", "", err
		}
		if n == 0 {
			slot = candidate
			break
		}
	}
	if slot == "" {
		return "", "", ErrLimit
	}
	token, tokenHash, err := mintNodeTokenTx(tx)
	if err != nil {
		return "", "", err
	}
	now := time.Now().UnixMilli()
	// A reused coordinate may still have an additive credential left by a
	// feature-off interval. Retire it before changing the authoritative slot
	// binding so the composite FK and generation identity remain valid.
	var oldActorID int64
	err = tx.QueryRow(`SELECT actor_id FROM installation_credentials
WHERE slot_orbit_id = ? AND slot_name = ?`, orbitID, slot).Scan(&oldActorID)
	if err == nil {
		if err := retireInstallationTx(tx, oldActorID, now); err != nil {
			return "", "", err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return "", "", err
	}
	_, err = tx.Exec(`INSERT INTO slots(orbit_id, slot, token_hash, paired_by, provider, paired_at, revoked_at)
VALUES(?, ?, ?, ?, 'spotify', ?, NULL)
ON CONFLICT(orbit_id, slot) DO UPDATE SET
  token_hash = excluded.token_hash,
  paired_by = excluded.paired_by,
  provider = excluded.provider,
  paired_at = excluded.paired_at,
  revoked_at = NULL`, orbitID, slot, tokenHash, pairedBy, now)
	if err != nil {
		return "", "", err
	}
	if s.selfServiceOnboarding {
		if err := s.reconcileIdentityTx(tx, now); err != nil {
			return "", "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", "", err
	}
	return slot, token, nil
}

func mintNodeTokenTx(tx *sql.Tx) (token, digest string, err error) {
	for attempt := 0; attempt < 16; attempt++ {
		token = randomHex(32)
		digest = hashToken(token)
		if err := assertNodeDigestAvailableTx(tx, digest); err == nil {
			return token, digest, nil
		} else if !errors.Is(err, ErrCredentialDomainConflict) {
			return "", "", err
		}
	}
	return "", "", errors.New("could not mint a unique node credential")
}

func assertNodeDigestAvailableTx(tx *sql.Tx, digest string) error {
	var matches int
	if err := tx.QueryRow(`SELECT
  (SELECT COUNT(*) FROM slots WHERE token_hash = ?) +
  (SELECT COUNT(*) FROM installation_credentials WHERE control_token_hash = ?)`,
		digest, digest).Scan(&matches); err != nil {
		return err
	}
	if matches != 0 {
		return ErrCredentialDomainConflict
	}
	return nil
}

// SlotOf returns the slot paired by this member ("" if none).
func (s *Store) SlotOf(orbitID, tgUserID int64) (string, error) {
	var slot string
	err := s.db.QueryRow(`SELECT slot FROM slots WHERE orbit_id = ? AND paired_by = ? AND revoked_at IS NULL`,
		orbitID, tgUserID).Scan(&slot)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return slot, err
}

// OrbitIDs lists all orbit ids (startup session warm-up).
func (s *Store) OrbitIDs() ([]int64, error) {
	rows, err := s.db.Query(`SELECT id FROM orbits ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// LookupToken resolves a node token to (orbit, slot). ok=false for unknown/revoked.
func (s *Store) LookupToken(token string) (orbitID int64, slot string, ok bool, err error) {
	err = s.db.QueryRow(`SELECT orbit_id, slot FROM slots WHERE token_hash = ? AND revoked_at IS NULL`,
		hashToken(token)).Scan(&orbitID, &slot)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, err
	}
	return orbitID, slot, true, nil
}

// RevokeSlot invalidates a slot's token; the slot letter becomes reusable.
// found=false when the orbit has no such live slot (L11: /revoke of a
// nonexistent home used to report success).
func (s *Store) RevokeSlot(orbitID int64, slot string) (found bool, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	now := time.Now().UnixMilli()
	res, err := tx.Exec(`UPDATE slots SET revoked_at = ? WHERE orbit_id = ? AND slot = ? AND revoked_at IS NULL`,
		now, orbitID, slot)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	if n > 0 && s.selfServiceOnboarding {
		if err := s.reconcileIdentityTx(tx, now); err != nil {
			return false, err
		}
	}
	if err := tx.Commit(); err != nil {
		return false, err
	}
	return n > 0, nil
}

// ActiveSlots lists non-revoked slots of an orbit.
func (s *Store) ActiveSlots(orbitID int64) ([]string, error) {
	rows, err := s.db.Query(`SELECT slot FROM slots WHERE orbit_id = ? AND revoked_at IS NULL ORDER BY slot`, orbitID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var sl string
		if err := rows.Scan(&sl); err != nil {
			return nil, err
		}
		out = append(out, sl)
	}
	return out, rows.Err()
}

// BootstrapLegacyOrbit migrates env-configured tokens/users into orbit #1 so
// a pre-M1 node keeps working without re-pairing. No-op when orbits exist.
func (s *Store) BootstrapLegacyOrbit(tokens map[string]string, users map[int64]string) (*Orbit, error) {
	// Seed only when a real (non-empty) legacy token exists — a container
	// config ships empty placeholders and must NOT spawn a ghost orbit.
	seeded := false
	for _, t := range tokens {
		if t != "" {
			seeded = true
		}
	}
	if !seeded {
		return nil, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM orbits`).Scan(&n); err != nil {
		return nil, err
	}
	if err := s.checkpoint("bootstrap_legacy_after_eligibility"); err != nil {
		return nil, err
	}
	if n > 0 {
		return nil, nil
	}
	now := time.Now().UnixMilli()
	res, err := tx.Exec(`INSERT INTO orbits(title, created_at) VALUES('Барицентр', ?)`, now)
	if err != nil {
		return nil, err
	}
	orbitID, _ := res.LastInsertId()
	first := true
	for uid := range users {
		role := "companion"
		if first {
			role = "primary"
			first = false
		}
		if _, err := tx.Exec(`INSERT INTO members(orbit_id, tg_user_id, role, joined_at) VALUES(?, ?, ?, ?)`,
			orbitID, uid, role, now); err != nil {
			return nil, err
		}
	}
	slotOwner := map[string]int64{}
	for uid, slot := range users {
		slotOwner[slot] = uid
	}
	for slot, token := range tokens {
		if token == "" {
			continue
		}
		tokenHash := hashToken(token)
		if err := assertNodeDigestAvailableTx(tx, tokenHash); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`INSERT INTO slots(orbit_id, slot, token_hash, paired_by, paired_at) VALUES(?, ?, ?, ?, ?)`,
			orbitID, slot, tokenHash, slotOwner[slot], now); err != nil {
			return nil, err
		}
	}
	// Legacy settings carried over: takeover policy and per-slot knobs.
	var takeover string
	if err := tx.QueryRow(`SELECT value FROM settings WHERE key = 'takeover_policy'`).Scan(&takeover); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if takeover == "user" || takeover == "coordinator" {
		if _, err := tx.Exec(`UPDATE orbits SET takeover_policy = ? WHERE id = ?`, takeover, orbitID); err != nil {
			return nil, err
		}
	}
	for _, slot := range []string{"a", "b"} {
		for _, kind := range []string{"volume_", "offset_"} {
			var value string
			err := tx.QueryRow(`SELECT value FROM settings WHERE key = ?`, kind+slot).Scan(&value)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				return nil, err
			}
			if value != "" {
				if _, err := tx.Exec(`INSERT INTO settings(key, value) VALUES(?, ?)
ON CONFLICT(key) DO UPDATE SET value = excluded.value`, fmt.Sprintf("%s%d_%s", kind, orbitID, slot), value); err != nil {
					return nil, err
				}
			}
		}
	}
	if s.selfServiceOnboarding {
		if err := s.reconcileIdentityTx(tx, now); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.GetOrbit(orbitID)
}
