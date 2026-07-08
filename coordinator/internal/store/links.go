// Approaches (сближения) between personal barycenters (design §12, L1):
// a link row ties two orbits into one shared broadcast. Lifecycle:
// proposed (one-time code, TTL 15 min) -> awaiting (second primary sent the
// code, initiator must confirm) -> active (group session live). L1 allows at
// most one non-proposed link per orbit.
package store

import (
	"database/sql"
	"errors"
	"time"
)

const linksSchema = `
CREATE TABLE IF NOT EXISTS links (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  orbit_a INTEGER NOT NULL,
  orbit_b INTEGER NOT NULL DEFAULT 0,
  state TEXT NOT NULL DEFAULT 'proposed', -- proposed | awaiting | active
  proposed_by INTEGER NOT NULL,
  pending_orbit INTEGER NOT NULL DEFAULT 0,
  code TEXT NOT NULL DEFAULT '',
  created_at INTEGER NOT NULL
);
`

// linkCodeTTL: an approach code lives 15 minutes (design §12 bot flow).
const linkCodeTTL = 15 * time.Minute

// awaitingTTL (M4): a claimed-but-unconfirmed approach expires after 48 h.
// Without it a claim the initiator never answers link-locks BOTH orbits
// forever: /approach says "сначала /apart" while /apart needs an ACTIVE link.
// Measured from created_at — the claim happens within the 15-minute code
// window, so the skew is negligible and no schema change is needed.
const awaitingTTL = 48 * time.Hour

var (
	// ErrLinkBusy: one of the orbits already has an active or awaiting link
	// (L1: one approach per orbit).
	ErrLinkBusy = errors.New("orbit already has a link")
	// ErrLinkSelf: an orbit cannot approach itself.
	ErrLinkSelf = errors.New("orbit cannot link to itself")
)

// Link is one approach between two personal barycenters.
type Link struct {
	ID           int64
	OrbitA       int64
	OrbitB       int64
	State        string // proposed | awaiting | active
	ProposedBy   int64
	PendingOrbit int64
	Code         string
	CreatedAt    int64
}

// linkEngaged reports whether the orbit participates in an awaiting or
// active link (proposed codes are cheap and do not count). Awaiting links
// past their TTL no longer engage (M4) — they are garbage awaiting hygiene.
func (s *Store) linkEngaged(orbitID int64) (bool, error) {
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM links
		WHERE (state = 'active' OR (state = 'awaiting' AND created_at >= ?))
		  AND (orbit_a = ? OR orbit_b = ?)`,
		time.Now().Add(-awaitingTTL).UnixMilli(), orbitID, orbitID).Scan(&n)
	return n > 0, err
}

// ProposeLink issues a one-time approach code for fromOrbit (invites-style,
// TTL 15 min). A fresh code supersedes the orbit's earlier unclaimed codes.
func (s *Store) ProposeLink(fromOrbit, byUser int64) (string, error) {
	if busy, err := s.linkEngaged(fromOrbit); err != nil {
		return "", err
	} else if busy {
		return "", ErrLinkBusy
	}
	now := time.Now().UnixMilli()
	// Hygiene: drop this orbit's stale codes, everyone's expired codes and
	// everyone's expired awaiting claims (M4).
	if _, err := s.db.Exec(`DELETE FROM links
		WHERE (state = 'proposed' AND (orbit_a = ? OR created_at < ?))
		   OR (state = 'awaiting' AND created_at < ?)`,
		fromOrbit, now-linkCodeTTL.Milliseconds(), now-awaitingTTL.Milliseconds()); err != nil {
		return "", err
	}
	code := randomCode()
	_, err := s.db.Exec(`INSERT INTO links(orbit_a, state, proposed_by, code, created_at) VALUES(?, 'proposed', ?, ?, ?)`,
		fromOrbit, byUser, code, now)
	return code, err
}

// AcceptByCode claims an approach code for toOrbit: the link moves to
// 'awaiting' (the initiator still confirms with /accept) and the code burns.
// Returns (0, 0, nil) for an unknown/expired/claimed code, ErrLinkSelf for a
// self-approach and ErrLinkBusy when either orbit already has a link (L1).
func (s *Store) AcceptByCode(code string, toOrbit int64) (linkID, orbitA int64, err error) {
	var createdAt int64
	err = s.db.QueryRow(`SELECT id, orbit_a, created_at FROM links WHERE state = 'proposed' AND code = ?`,
		code).Scan(&linkID, &orbitA, &createdAt)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, nil
	}
	if err != nil {
		return 0, 0, err
	}
	now := time.Now().UnixMilli()
	if now-createdAt > linkCodeTTL.Milliseconds() {
		return 0, 0, nil
	}
	if orbitA == toOrbit {
		return 0, 0, ErrLinkSelf
	}
	for _, orbit := range []int64{orbitA, toOrbit} {
		if busy, err := s.linkEngaged(orbit); err != nil {
			return 0, 0, err
		} else if busy {
			return 0, 0, ErrLinkBusy
		}
	}
	if _, err := s.db.Exec(`UPDATE links SET orbit_b = ?, state = 'awaiting', pending_orbit = ?, code = '' WHERE id = ?`,
		toOrbit, toOrbit, linkID); err != nil {
		return 0, 0, err
	}
	return linkID, orbitA, nil
}

// AwaitingLink finds the confirmation pending on this orbit: the link this
// orbit initiated that another orbit has claimed (state 'awaiting'). Expired
// claims are invisible (M4) — /accept must not activate a fossil.
func (s *Store) AwaitingLink(orbitID int64) (linkID, otherOrbit int64, ok bool, err error) {
	err = s.db.QueryRow(`SELECT id, orbit_b FROM links WHERE state = 'awaiting' AND orbit_a = ? AND created_at >= ?`,
		orbitID, time.Now().Add(-awaitingTTL).UnixMilli()).Scan(&linkID, &otherOrbit)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	return linkID, otherOrbit, true, nil
}

// AwaitingLinkAnySide finds the awaiting link this orbit participates in from
// EITHER side, reporting whether the caller is the initiator (M4): the
// claimant must be able to /decline too — an ignored claim otherwise
// link-locks both orbits until the TTL, with the busy-reply's "сначала
// /apart" advice impossible to follow.
func (s *Store) AwaitingLinkAnySide(orbitID int64) (linkID, otherOrbit int64, initiator, ok bool, err error) {
	var a, b int64
	err = s.db.QueryRow(`SELECT id, orbit_a, orbit_b FROM links WHERE state = 'awaiting' AND created_at >= ? AND (orbit_a = ? OR orbit_b = ?)`,
		time.Now().Add(-awaitingTTL).UnixMilli(), orbitID, orbitID).Scan(&linkID, &a, &b)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, false, false, nil
	}
	if err != nil {
		return 0, 0, false, false, err
	}
	if a == orbitID {
		return linkID, b, true, true, nil
	}
	return linkID, a, false, true, nil
}

// ActivateLink flips an awaiting link to active (both sides consented).
func (s *Store) ActivateLink(linkID int64) error {
	res, err := s.db.Exec(`UPDATE links SET state = 'active', pending_orbit = 0 WHERE id = ? AND state = 'awaiting'`, linkID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return errors.New("link is not awaiting confirmation")
	}
	return nil
}

// ActiveLink returns the orbit's active approach and the other side.
func (s *Store) ActiveLink(orbitID int64) (linkID, otherOrbit int64, ok bool, err error) {
	var a, b int64
	err = s.db.QueryRow(`SELECT id, orbit_a, orbit_b FROM links WHERE state = 'active' AND (orbit_a = ? OR orbit_b = ?)`,
		orbitID, orbitID).Scan(&linkID, &a, &b)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, 0, false, nil
	}
	if err != nil {
		return 0, 0, false, err
	}
	other := a
	if a == orbitID {
		other = b
	}
	return linkID, other, true, nil
}

// ActiveLinks lists every active approach (loop warm-up).
func (s *Store) ActiveLinks() ([]Link, error) {
	rows, err := s.db.Query(`SELECT id, orbit_a, orbit_b, state, proposed_by, pending_orbit, code, created_at FROM links WHERE state = 'active' ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Link
	for rows.Next() {
		var lk Link
		if err := rows.Scan(&lk.ID, &lk.OrbitA, &lk.OrbitB, &lk.State, &lk.ProposedBy, &lk.PendingOrbit, &lk.Code, &lk.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, lk)
	}
	return out, rows.Err()
}

// GetLink fetches one link row by id (nil when gone).
func (s *Store) GetLink(linkID int64) (*Link, error) {
	lk := &Link{}
	err := s.db.QueryRow(`SELECT id, orbit_a, orbit_b, state, proposed_by, pending_orbit, code, created_at FROM links WHERE id = ?`,
		linkID).Scan(&lk.ID, &lk.OrbitA, &lk.OrbitB, &lk.State, &lk.ProposedBy, &lk.PendingOrbit, &lk.Code, &lk.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return lk, nil
}

// BreakLink dissolves an approach: the row is deleted, each barycenter keeps
// everything of its own (design §12: breaking up is painless).
func (s *Store) BreakLink(linkID int64) error {
	_, err := s.db.Exec(`DELETE FROM links WHERE id = ?`, linkID)
	return err
}
