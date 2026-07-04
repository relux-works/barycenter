// Multi-tenant layer (v2.1 M1, docs/v2-multitenant-design.md): orbits,
// members with star-system roles, node slots with hashed tokens, one-time
// invites (pairing codes and member invite links).
package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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
  PRIMARY KEY (orbit_id, tg_user_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS members_user ON members(tg_user_id); -- MVP: one orbit per user
CREATE TABLE IF NOT EXISTS slots (
  orbit_id INTEGER NOT NULL,
  slot TEXT NOT NULL,
  token_hash TEXT NOT NULL,
  paired_by INTEGER NOT NULL DEFAULT 0,
  paired_at INTEGER,
  revoked_at INTEGER,
  PRIMARY KEY (orbit_id, slot)
);
CREATE UNIQUE INDEX IF NOT EXISTS slots_token ON slots(token_hash);
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
	OrbitID  int64
	TGUserID int64
	Role     string // primary | companion | satellite
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
	_, err := s.db.Exec(orbitSchema)
	return err
}

// CreateOrbit makes a new orbit with its creator as primary.
func (s *Store) CreateOrbit(title string, creator int64) (*Orbit, error) {
	now := time.Now().UnixMilli()
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
	err := s.db.QueryRow(`SELECT orbit_id, tg_user_id, role FROM members WHERE tg_user_id = ?`, tgUserID).
		Scan(&m.OrbitID, &m.TGUserID, &m.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return m, err
}

func (s *Store) Members(orbitID int64) ([]Member, error) {
	rows, err := s.db.Query(`SELECT orbit_id, tg_user_id, role FROM members WHERE orbit_id = ? ORDER BY joined_at`, orbitID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Member
	for rows.Next() {
		var m Member
		if err := rows.Scan(&m.OrbitID, &m.TGUserID, &m.Role); err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) AddMember(orbitID int64, tgUserID int64, role string) error {
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
	return tx.Commit()
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
	if _, err := s.db.Exec(`UPDATE invites SET used_at = ? WHERE code = ?`, now, code); err != nil {
		return 0, 0, err
	}
	return orbitID, issuedBy, nil
}

// --- Slots & node tokens ---

// PairSlot allocates the next free slot in the orbit and mints its token.
// Returns (slot, plaintext token). The plaintext is never stored.
// pairedBy records which member owns the node (used for "my home" defaults).
func (s *Store) PairSlot(orbitID int64, pairedBy int64) (string, string, error) {
	var used int
	var maxP int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM slots WHERE orbit_id = ? AND revoked_at IS NULL`, orbitID).Scan(&used); err != nil {
		return "", "", err
	}
	if err := s.db.QueryRow(`SELECT max_pulsars FROM orbits WHERE id = ?`, orbitID).Scan(&maxP); err != nil {
		return "", "", err
	}
	if used >= maxP {
		return "", "", ErrLimit
	}
	// Slots are letters a, b, c… — reuse revoked letters last.
	slot := ""
	for i := 0; i < maxP; i++ {
		candidate := string(rune('a' + i))
		var n int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM slots WHERE orbit_id = ? AND slot = ? AND revoked_at IS NULL`,
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
	token := randomHex(32)
	_, err := s.db.Exec(`INSERT OR REPLACE INTO slots(orbit_id, slot, token_hash, paired_by, paired_at, revoked_at) VALUES(?, ?, ?, ?, ?, NULL)`,
		orbitID, slot, hashToken(token), pairedBy, time.Now().UnixMilli())
	return slot, token, err
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
func (s *Store) RevokeSlot(orbitID int64, slot string) error {
	_, err := s.db.Exec(`UPDATE slots SET revoked_at = ? WHERE orbit_id = ? AND slot = ? AND revoked_at IS NULL`,
		time.Now().UnixMilli(), orbitID, slot)
	return err
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
	var n int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM orbits`).Scan(&n); err != nil {
		return nil, err
	}
	// Seed only when a real (non-empty) legacy token exists — a container
	// config ships empty placeholders and must NOT spawn a ghost orbit.
	seeded := false
	for _, t := range tokens {
		if t != "" {
			seeded = true
		}
	}
	if n > 0 || !seeded {
		return nil, nil
	}
	now := time.Now().UnixMilli()
	res, err := s.db.Exec(`INSERT INTO orbits(title, created_at) VALUES('Барицентр', ?)`, now)
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
		if _, err := s.db.Exec(`INSERT INTO members(orbit_id, tg_user_id, role, joined_at) VALUES(?, ?, ?, ?)`,
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
		if _, err := s.db.Exec(`INSERT INTO slots(orbit_id, slot, token_hash, paired_by, paired_at) VALUES(?, ?, ?, ?, ?)`,
			orbitID, slot, hashToken(token), slotOwner[slot], now); err != nil {
			return nil, err
		}
	}
	// Legacy settings carried over: takeover policy and per-slot knobs.
	if v, _ := s.GetSetting("takeover_policy"); v == "user" || v == "coordinator" {
		s.SetOrbitSetting(orbitID, "takeover_policy", v)
	}
	for _, slot := range []string{"a", "b"} {
		for _, kind := range []string{"volume_", "offset_"} {
			if v, _ := s.GetSetting(kind + slot); v != "" {
				s.SetSetting(fmt.Sprintf("%s%d_%s", kind, orbitID, slot), v)
			}
		}
	}
	return s.GetOrbit(orbitID)
}
