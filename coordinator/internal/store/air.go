package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"relux.works/duet/coordinator/internal/ulid"
)

var (
	ErrAirNotFound       = errors.New("air not found")
	ErrAirRevision       = errors.New("air revision conflict")
	ErrAirActiveChanged  = errors.New("active air changed")
	ErrAirNotJoined      = errors.New("air membership is not joined")
	ErrAirRollbackUnsafe = errors.New("air rollback unsafe")
	ErrAirCapacity       = errors.New("air barycenter capacity reached")
	ErrAirOwnerLeave     = errors.New("air owner must transfer or dissolve")
)

type Air struct {
	ID           string
	Title        string
	Status       string
	OwnerOrbitID int64
	Revision     int64
	CreatedAt    int64
	DissolvedAt  sql.NullInt64
}

type AirMember struct {
	ID        string
	AirID     string
	OrbitID   int64
	Role      string
	Status    string
	Revision  int64
	JoinedAt  sql.NullInt64
	LeftAt    sql.NullInt64
	CreatedAt int64
}

type AirPolicy struct {
	AirID     string
	Revision  int64
	Invite    string
	Overlay   string
	Queue     string
	Replace   string
	UpdatedAt int64
}

type AirInvite struct {
	ID                   string
	AirID                string
	CodeHash             string
	Status               string
	IntendedRole         string
	IssuedByActorID      int64
	IssuedByOrbitID      int64
	PolicyRevision       int64
	Revision             int64
	ExpiresAt            int64
	ConsumedMembershipID sql.NullString
	CreatedAt            int64
	UpdatedAt            int64
}

type AirAuthority struct {
	Mode            string
	Generation      int64
	DivergenceCount int64
	UpdatedAt       int64
}

type CreateAirParams struct {
	Title        string
	OwnerOrbitID int64
	CreatedAt    int64
}

func (s *Store) CreateAir(params CreateAirParams) (*Air, error) {
	title := strings.TrimSpace(params.Title)
	if title == "" || len([]rune(title)) > 80 || params.OwnerOrbitID <= 0 {
		return nil, fmt.Errorf("%w: invalid title or owner", ErrAirNotFound)
	}
	if params.CreatedAt <= 0 {
		params.CreatedAt = time.Now().UnixMilli()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := requireAirsAuthoritativeTx(tx); err != nil {
		return nil, err
	}
	var orbitExists int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM orbits WHERE id = ?`, params.OwnerOrbitID).Scan(&orbitExists); err != nil {
		return nil, err
	}
	if orbitExists != 1 {
		return nil, ErrAirNotFound
	}
	airID := "air_" + ulid.New(time.UnixMilli(params.CreatedAt))
	memberID := "aim_" + ulid.New(time.UnixMilli(params.CreatedAt))
	if _, err := tx.Exec(`INSERT INTO airs(
  public_id, title, status, owner_orbit_id, revision, created_at
) VALUES(?, ?, 'parked', ?, 1, ?)`, airID, title, params.OwnerOrbitID, params.CreatedAt); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`INSERT INTO air_members(
  public_id, air_id, orbit_id, air_role, status, revision, joined_at, created_at
) VALUES(?, ?, ?, 'owner', 'joined', 1, ?, ?)`,
		memberID, airID, params.OwnerOrbitID, params.CreatedAt, params.CreatedAt); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`INSERT INTO air_policies(
  air_id, revision, invite_policy, overlay_policy, queue_policy, replace_policy, updated_at
) VALUES(?, 1, 'air_admin_primary', 'primary_companion', 'primary_companion', 'air_admin_primary', ?)`,
		airID, params.CreatedAt); err != nil {
		return nil, err
	}
	if err := markAirDivergenceTx(tx, params.CreatedAt); err != nil {
		return nil, err
	}
	if err := appendAirAuditTx(tx, airID, memberID, "", 0, params.OwnerOrbitID,
		"air.create", "", "parked", "ok", params.CreatedAt); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.AirByID(airID)
}

func (s *Store) AirByID(airID string) (*Air, error) {
	var air Air
	err := s.db.QueryRow(`SELECT public_id, title, status, owner_orbit_id,
  revision, created_at, dissolved_at
FROM airs WHERE public_id = ?`, airID).Scan(
		&air.ID, &air.Title, &air.Status, &air.OwnerOrbitID,
		&air.Revision, &air.CreatedAt, &air.DissolvedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAirNotFound
	}
	return &air, err
}

func (s *Store) AirMembers(airID string) ([]AirMember, error) {
	rows, err := s.db.Query(`SELECT public_id, air_id, orbit_id, air_role,
  status, revision, joined_at, left_at, created_at
FROM air_members WHERE air_id = ? ORDER BY created_at, public_id`, airID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []AirMember
	for rows.Next() {
		var member AirMember
		if err := rows.Scan(&member.ID, &member.AirID, &member.OrbitID,
			&member.Role, &member.Status, &member.Revision, &member.JoinedAt,
			&member.LeftAt, &member.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, member)
	}
	return out, rows.Err()
}

func (s *Store) SavedAirsForOrbit(orbitID int64) ([]Air, error) {
	rows, err := s.db.Query(`SELECT a.public_id, a.title, a.status,
  a.owner_orbit_id, a.revision, a.created_at, a.dissolved_at
FROM airs a JOIN air_members m ON m.air_id = a.public_id
WHERE m.orbit_id = ? AND m.status = 'joined'
ORDER BY lower(a.title), a.public_id`, orbitID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Air
	for rows.Next() {
		var air Air
		if err := rows.Scan(&air.ID, &air.Title, &air.Status, &air.OwnerOrbitID,
			&air.Revision, &air.CreatedAt, &air.DissolvedAt); err != nil {
			return nil, err
		}
		out = append(out, air)
	}
	return out, rows.Err()
}

func (s *Store) AirPolicy(airID string) (*AirPolicy, error) {
	var policy AirPolicy
	err := s.db.QueryRow(`SELECT air_id, revision, invite_policy,
  overlay_policy, queue_policy, replace_policy, updated_at
FROM air_policies WHERE air_id = ?`, airID).Scan(
		&policy.AirID, &policy.Revision, &policy.Invite, &policy.Overlay,
		&policy.Queue, &policy.Replace, &policy.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAirNotFound
	}
	return &policy, err
}

func (s *Store) AddPendingAirMember(airID string, orbitID int64, role string, now int64) (*AirMember, error) {
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := requireAirsAuthoritativeTx(tx); err != nil {
		return nil, err
	}
	if role != "admin" && role != "member" {
		return nil, ErrAirNotJoined
	}
	var airStatus string
	if err := tx.QueryRow(`SELECT status FROM airs WHERE public_id = ?`, airID).Scan(&airStatus); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrAirNotFound
		}
		return nil, err
	}
	if airStatus == "dissolved" {
		return nil, ErrAirNotFound
	}
	var orbitExists, liveMembers int
	if err := tx.QueryRow(`SELECT
  (SELECT COUNT(*) FROM orbits WHERE id = ?),
  (SELECT COUNT(*) FROM air_members WHERE air_id = ?
     AND status IN ('pending_confirmation', 'joined'))`, orbitID, airID).Scan(&orbitExists, &liveMembers); err != nil {
		return nil, err
	}
	if orbitExists != 1 {
		return nil, ErrAirNotFound
	}
	if liveMembers >= 8 {
		return nil, ErrAirCapacity
	}
	memberID := "aim_" + ulid.New(time.UnixMilli(now))
	if _, err := tx.Exec(`INSERT INTO air_members(
  public_id, air_id, orbit_id, air_role, status, revision, created_at
) VALUES(?, ?, ?, ?, 'pending_confirmation', 1, ?)`, memberID, airID, orbitID, role, now); err != nil {
		return nil, err
	}
	if err := markAirDivergenceTx(tx, now); err != nil {
		return nil, err
	}
	if err := appendAirAuditTx(tx, airID, memberID, "", 0, orbitID,
		"air.member.pending", "", role, "ok", now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	members, err := s.AirMembers(airID)
	if err != nil {
		return nil, err
	}
	for i := range members {
		if members[i].ID == memberID {
			return &members[i], nil
		}
	}
	return nil, ErrAirNotFound
}

func (s *Store) ConfirmAirMember(memberID string, expectedRevision int64, activate bool, expectedAirID string, now int64) error {
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := requireAirsAuthoritativeTx(tx); err != nil {
		return err
	}
	var airID, status string
	var orbitID, revision int64
	if err := tx.QueryRow(`SELECT air_id, orbit_id, status, revision
FROM air_members WHERE public_id = ?`, memberID).Scan(&airID, &orbitID, &status, &revision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAirNotJoined
		}
		return err
	}
	if status != "pending_confirmation" || revision != expectedRevision {
		return ErrAirRevision
	}
	res, err := tx.Exec(`UPDATE air_members SET status = 'joined', joined_at = ?,
  revision = revision + 1 WHERE public_id = ? AND status = 'pending_confirmation' AND revision = ?`,
		now, memberID, expectedRevision)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		return ErrAirRevision
	}
	if activate {
		current, pointerRevision, ok, err := activeAirForOrbitTx(tx, orbitID)
		if err != nil {
			return err
		}
		if !activeAirExpectationMatches(current, ok, expectedAirID) {
			return ErrAirActiveChanged
		}
		if ok {
			res, err := tx.Exec(`DELETE FROM air_active_pointers WHERE orbit_id = ? AND revision = ?`, orbitID, pointerRevision)
			if err != nil {
				return err
			}
			if n, err := res.RowsAffected(); err != nil || n != 1 {
				return ErrAirActiveChanged
			}
		}
		if _, err := tx.Exec(`INSERT INTO air_active_pointers(orbit_id, air_id, revision, activated_at)
VALUES(?, ?, ?, ?)`, orbitID, airID, pointerRevision+1, now); err != nil {
			return err
		}
		if ok && current != airID {
			if err := refreshAirStatusTx(tx, current); err != nil {
				return err
			}
		}
		if err := refreshAirStatusTx(tx, airID); err != nil {
			return err
		}
	}
	if err := markAirDivergenceTx(tx, now); err != nil {
		return err
	}
	if err := appendAirAuditTx(tx, airID, memberID, "", 0, orbitID,
		"air.member.confirm", "pending_confirmation", "joined", "ok", now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) LeaveAirMember(memberID string, expectedRevision int64, now int64) error {
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := requireAirsAuthoritativeTx(tx); err != nil {
		return err
	}
	var airID, role, status string
	var orbitID, revision int64
	if err := tx.QueryRow(`SELECT air_id, orbit_id, air_role, status, revision
FROM air_members WHERE public_id = ?`, memberID).Scan(&airID, &orbitID, &role, &status, &revision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrAirNotJoined
		}
		return err
	}
	if role == "owner" && status == "joined" {
		return ErrAirOwnerLeave
	}
	if status != "joined" || revision != expectedRevision {
		return ErrAirRevision
	}
	if _, err := tx.Exec(`DELETE FROM air_active_pointers WHERE orbit_id = ? AND air_id = ?`, orbitID, airID); err != nil {
		return err
	}
	res, err := tx.Exec(`UPDATE air_members SET status = 'left', left_at = ?,
  revision = revision + 1 WHERE public_id = ? AND status = 'joined' AND revision = ?`,
		now, memberID, expectedRevision)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		return ErrAirRevision
	}
	if err := refreshAirStatusTx(tx, airID); err != nil {
		return err
	}
	if err := markAirDivergenceTx(tx, now); err != nil {
		return err
	}
	if err := appendAirAuditTx(tx, airID, memberID, "", 0, orbitID,
		"air.member.leave", "joined", "left", "ok", now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ActiveAirForOrbit(orbitID int64) (airID string, revision int64, ok bool, err error) {
	err = s.db.QueryRow(`SELECT air_id, revision FROM air_active_pointers WHERE orbit_id = ?`, orbitID).Scan(&airID, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, false, nil
	}
	return airID, revision, err == nil, err
}

func (s *Store) ActivateAir(orbitID int64, airID, expectedAirID string, now int64) error {
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := requireAirsAuthoritativeTx(tx); err != nil {
		return err
	}
	current, revision, ok, err := activeAirForOrbitTx(tx, orbitID)
	if err != nil {
		return err
	}
	if !activeAirExpectationMatches(current, ok, expectedAirID) {
		return ErrAirActiveChanged
	}
	var joined int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM air_members m JOIN airs a ON a.public_id = m.air_id
WHERE m.air_id = ? AND m.orbit_id = ? AND m.status = 'joined' AND a.status <> 'dissolved'`,
		airID, orbitID).Scan(&joined); err != nil {
		return err
	}
	if joined != 1 {
		return ErrAirNotJoined
	}
	if ok && current == airID {
		return tx.Commit()
	}
	if ok {
		res, err := tx.Exec(`DELETE FROM air_active_pointers WHERE orbit_id = ? AND revision = ?`, orbitID, revision)
		if err != nil {
			return err
		}
		if n, err := res.RowsAffected(); err != nil || n != 1 {
			return ErrAirActiveChanged
		}
	}
	nextRevision := revision + 1
	if nextRevision < 1 {
		nextRevision = 1
	}
	if _, err := tx.Exec(`INSERT INTO air_active_pointers(orbit_id, air_id, revision, activated_at)
VALUES(?, ?, ?, ?)`, orbitID, airID, nextRevision, now); err != nil {
		return err
	}
	if ok {
		if err := refreshAirStatusTx(tx, current); err != nil {
			return err
		}
	}
	if err := refreshAirStatusTx(tx, airID); err != nil {
		return err
	}
	if err := markAirDivergenceTx(tx, now); err != nil {
		return err
	}
	if err := appendAirAuditTx(tx, airID, "", "", 0, orbitID,
		"air.activate", current, airID, "ok", now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) DeactivateAir(orbitID int64, airID string, now int64) error {
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := requireAirsAuthoritativeTx(tx); err != nil {
		return err
	}
	current, revision, ok, err := activeAirForOrbitTx(tx, orbitID)
	if err != nil {
		return err
	}
	if !ok {
		return tx.Commit()
	}
	if current != airID {
		return ErrAirActiveChanged
	}
	res, err := tx.Exec(`DELETE FROM air_active_pointers WHERE orbit_id = ? AND revision = ?`, orbitID, revision)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		return ErrAirActiveChanged
	}
	if err := refreshAirStatusTx(tx, airID); err != nil {
		return err
	}
	if err := markAirDivergenceTx(tx, now); err != nil {
		return err
	}
	if err := appendAirAuditTx(tx, airID, "", "", 0, orbitID,
		"air.deactivate", airID, "", "ok", now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) ReplaceAirPolicy(policy AirPolicy, expectedRevision, now int64) error {
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := requireAirsAuthoritativeTx(tx); err != nil {
		return err
	}
	res, err := tx.Exec(`UPDATE air_policies SET revision = revision + 1,
  invite_policy = ?, overlay_policy = ?, queue_policy = ?, replace_policy = ?, updated_at = ?
WHERE air_id = ? AND revision = ?`, policy.Invite, policy.Overlay, policy.Queue,
		policy.Replace, now, policy.AirID, expectedRevision)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		return ErrAirRevision
	}
	if err := markAirDivergenceTx(tx, now); err != nil {
		return err
	}
	if err := appendAirAuditTx(tx, policy.AirID, "", "", 0, 0,
		"air.policy.replace", fmt.Sprintf("%d", expectedRevision),
		fmt.Sprintf("%d", expectedRevision+1), "ok", now); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) InsertAirInvite(inviteID, airID, codeHash, role string, actorID, orbitID, policyRevision, expiresAt, now int64) error {
	_, err := s.CreateAirInvite(AirInvite{
		ID: inviteID, AirID: airID, CodeHash: codeHash, IntendedRole: role,
		IssuedByActorID: actorID, IssuedByOrbitID: orbitID,
		PolicyRevision: policyRevision, ExpiresAt: expiresAt, CreatedAt: now,
	})
	return err
}

func (s *Store) CreateAirInvite(invite AirInvite) (*AirInvite, error) {
	now := invite.CreatedAt
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	if invite.ID == "" {
		invite.ID = "ai_" + ulid.New(time.UnixMilli(now))
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	if _, err := requireAirsAuthoritativeTx(tx); err != nil {
		return nil, err
	}
	if invite.IntendedRole != "admin" && invite.IntendedRole != "member" {
		return nil, ErrAirNotJoined
	}
	if invite.ExpiresAt <= now {
		return nil, ErrAirRevision
	}
	var eligible int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM air_policies p
JOIN air_members m ON m.air_id = p.air_id
WHERE p.air_id = ? AND p.revision = ? AND m.orbit_id = ? AND m.status = 'joined'`,
		invite.AirID, invite.PolicyRevision, invite.IssuedByOrbitID).Scan(&eligible); err != nil {
		return nil, err
	}
	if eligible != 1 {
		return nil, ErrAirRevision
	}
	if _, err := tx.Exec(`INSERT INTO air_invites(
  public_id, air_id, code_hash, status, intended_role, issued_by_actor_id,
  issued_by_orbit_id, policy_revision, revision, expires_at, created_at, updated_at
) VALUES(?, ?, ?, 'open', ?, ?, ?, ?, 1, ?, ?, ?)`, invite.ID, invite.AirID, invite.CodeHash,
		invite.IntendedRole, invite.IssuedByActorID, invite.IssuedByOrbitID,
		invite.PolicyRevision, invite.ExpiresAt, now, now); err != nil {
		return nil, err
	}
	if err := markAirDivergenceTx(tx, now); err != nil {
		return nil, err
	}
	if err := appendAirAuditTx(tx, invite.AirID, "", invite.ID,
		invite.IssuedByActorID, invite.IssuedByOrbitID,
		"air.invite.issue", "", invite.IntendedRole, "ok", now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.AirInviteByID(invite.ID)
}

func (s *Store) AirInviteByID(inviteID string) (*AirInvite, error) {
	return s.airInviteBy(`public_id = ?`, inviteID)
}

func (s *Store) AirInviteByCodeHash(codeHash string) (*AirInvite, error) {
	return s.airInviteBy(`code_hash = ?`, codeHash)
}

func (s *Store) airInviteBy(where string, value any) (*AirInvite, error) {
	var invite AirInvite
	err := s.db.QueryRow(`SELECT public_id, air_id, code_hash, status,
  intended_role, issued_by_actor_id, issued_by_orbit_id, policy_revision,
  revision, expires_at, consumed_membership_id, created_at, updated_at
FROM air_invites WHERE `+where, value).Scan(
		&invite.ID, &invite.AirID, &invite.CodeHash, &invite.Status,
		&invite.IntendedRole, &invite.IssuedByActorID, &invite.IssuedByOrbitID,
		&invite.PolicyRevision, &invite.Revision, &invite.ExpiresAt,
		&invite.ConsumedMembershipID, &invite.CreatedAt, &invite.UpdatedAt,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrAirNotFound
	}
	return &invite, err
}

func (s *Store) AirAuthority() (AirAuthority, error) {
	var authority AirAuthority
	err := s.db.QueryRow(`SELECT mode, generation, divergence_count, updated_at
FROM air_authority WHERE singleton = 1`).Scan(&authority.Mode, &authority.Generation,
		&authority.DivergenceCount, &authority.UpdatedAt)
	return authority, err
}

func (s *Store) CutoverLinksToAirs(expectedGeneration, now int64) (AirAuthority, error) {
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AirAuthority{}, err
	}
	defer tx.Rollback()
	authority, err := airAuthorityTx(tx)
	if err != nil {
		return AirAuthority{}, err
	}
	if authority.Generation != expectedGeneration || (authority.Mode != "airs_shadow" && authority.Mode != "links_authoritative") {
		return AirAuthority{}, ErrAirRevision
	}
	backfilled, err := s.backfillActiveLinksTx(tx, now)
	if err != nil {
		return AirAuthority{}, err
	}
	_ = backfilled
	rows, err := tx.Query(`SELECT m.air_id, m.orbit_a, m.orbit_b
FROM air_legacy_link_mappings m JOIN links l ON l.id = m.link_id
WHERE l.state = 'active' ORDER BY m.link_id`)
	if err != nil {
		return AirAuthority{}, err
	}
	type mapping struct {
		airID string
		a, b  int64
	}
	var mappings []mapping
	for rows.Next() {
		var mapped mapping
		if err := rows.Scan(&mapped.airID, &mapped.a, &mapped.b); err != nil {
			rows.Close()
			return AirAuthority{}, err
		}
		mappings = append(mappings, mapped)
	}
	if err := rows.Close(); err != nil {
		return AirAuthority{}, err
	}
	if err := rows.Err(); err != nil {
		return AirAuthority{}, err
	}
	var activeLinks int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM links WHERE state = 'active'`).Scan(&activeLinks); err != nil {
		return AirAuthority{}, err
	}
	if activeLinks != len(mappings) {
		return AirAuthority{}, fmt.Errorf("active links=%d mapped=%d", activeLinks, len(mappings))
	}
	var existingPointers int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM air_active_pointers`).Scan(&existingPointers); err != nil {
		return AirAuthority{}, err
	}
	if existingPointers != 0 {
		return AirAuthority{}, fmt.Errorf("Air cutover found %d preexisting active pointer(s)", existingPointers)
	}
	if _, err := tx.Exec(`DELETE FROM air_active_pointers`); err != nil {
		return AirAuthority{}, err
	}
	if _, err := tx.Exec(`UPDATE airs SET status = 'parked', revision = revision + (status <> 'parked')
WHERE status <> 'dissolved'`); err != nil {
		return AirAuthority{}, err
	}
	for _, mapped := range mappings {
		for _, orbitID := range []int64{mapped.a, mapped.b} {
			if _, err := tx.Exec(`INSERT INTO air_active_pointers(orbit_id, air_id, revision, activated_at)
VALUES(?, ?, 1, ?)`, orbitID, mapped.airID, now); err != nil {
				return AirAuthority{}, err
			}
		}
		if _, err := tx.Exec(`UPDATE airs SET status = 'active', revision = revision + 1
WHERE public_id = ? AND status = 'parked'`, mapped.airID); err != nil {
			return AirAuthority{}, err
		}
	}
	if _, err := tx.Exec(`INSERT INTO air_legacy_runtime_snapshots(
  authority_generation, link_id, air_id, orbit_a, orbit_b, link_created_at
)
SELECT ?, m.link_id, m.air_id, m.orbit_a, m.orbit_b, m.link_created_at
FROM air_legacy_link_mappings m JOIN links l ON l.id = m.link_id
WHERE l.state = 'active' ORDER BY m.link_id`, expectedGeneration+1); err != nil {
		return AirAuthority{}, err
	}
	if err := s.checkpoint("air_cutover_before_authority_flip"); err != nil {
		return AirAuthority{}, err
	}
	res, err := tx.Exec(`UPDATE air_authority SET mode = 'airs_authoritative',
  generation = generation + 1, divergence_count = 0, updated_at = ?
WHERE singleton = 1 AND generation = ?`, now, expectedGeneration)
	if err != nil {
		return AirAuthority{}, err
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		return AirAuthority{}, ErrAirRevision
	}
	if err := appendAirAuditTx(tx, "", "", "", 0, 0,
		"air.authority.cutover", authority.Mode, "airs_authoritative", "ok", now); err != nil {
		return AirAuthority{}, err
	}
	if err := tx.Commit(); err != nil {
		return AirAuthority{}, err
	}
	return s.AirAuthority()
}

func (s *Store) RollbackAirsToLinks(expectedGeneration, now int64) (AirAuthority, error) {
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AirAuthority{}, err
	}
	defer tx.Rollback()
	authority, err := airAuthorityTx(tx)
	if err != nil {
		return AirAuthority{}, err
	}
	if authority.Generation != expectedGeneration || authority.Mode != "airs_authoritative" {
		return AirAuthority{}, ErrAirRevision
	}
	if authority.DivergenceCount != 0 {
		return commitAirRollbackHold(tx, authority.Mode, now)
	}
	unsafeShape, err := airAuthorityShapeUnsafeTx(tx)
	if err != nil {
		return AirAuthority{}, err
	}
	if unsafeShape {
		return commitAirRollbackHold(tx, authority.Mode, now)
	}
	if _, err := tx.Exec(`DELETE FROM air_active_pointers`); err != nil {
		return AirAuthority{}, err
	}
	if _, err := tx.Exec(`UPDATE airs SET status = 'parked', revision = revision + 1
WHERE status = 'active'`); err != nil {
		return AirAuthority{}, err
	}
	if err := s.checkpoint("air_rollback_before_authority_flip"); err != nil {
		return AirAuthority{}, err
	}
	if _, err := tx.Exec(`UPDATE air_authority SET mode = 'links_authoritative',
  generation = generation + 1, updated_at = ? WHERE singleton = 1`, now); err != nil {
		return AirAuthority{}, err
	}
	if err := appendAirAuditTx(tx, "", "", "", 0, 0,
		"air.authority.rollback", authority.Mode, "links_authoritative", "ok", now); err != nil {
		return AirAuthority{}, err
	}
	if err := tx.Commit(); err != nil {
		return AirAuthority{}, err
	}
	return s.AirAuthority()
}

func activeAirForOrbitTx(tx *sql.Tx, orbitID int64) (string, int64, bool, error) {
	var airID string
	var revision int64
	err := tx.QueryRow(`SELECT air_id, revision FROM air_active_pointers WHERE orbit_id = ?`, orbitID).Scan(&airID, &revision)
	if errors.Is(err, sql.ErrNoRows) {
		return "", 0, false, nil
	}
	return airID, revision, err == nil, err
}

func activeAirExpectationMatches(current string, ok bool, expected string) bool {
	if expected == "none" {
		return !ok
	}
	return ok && current == expected
}

func refreshAirStatusTx(tx *sql.Tx, airID string) error {
	var activeMembers int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM air_active_pointers p
JOIN air_members m ON m.air_id = p.air_id AND m.orbit_id = p.orbit_id
WHERE p.air_id = ? AND m.status = 'joined'`, airID).Scan(&activeMembers); err != nil {
		return err
	}
	status := "parked"
	if activeMembers >= 2 {
		status = "active"
	}
	_, err := tx.Exec(`UPDATE airs SET status = ?, revision = revision + (status <> ?)
WHERE public_id = ? AND status <> 'dissolved'`, status, status, airID)
	return err
}

func airAuthorityTx(tx *sql.Tx) (AirAuthority, error) {
	var authority AirAuthority
	err := tx.QueryRow(`SELECT mode, generation, divergence_count, updated_at
FROM air_authority WHERE singleton = 1`).Scan(&authority.Mode, &authority.Generation,
		&authority.DivergenceCount, &authority.UpdatedAt)
	return authority, err
}

func markAirDivergenceTx(tx *sql.Tx, now int64) error {
	_, err := tx.Exec(`UPDATE air_authority SET
  divergence_count = divergence_count + CASE WHEN mode = 'airs_authoritative' THEN 1 ELSE 0 END,
  updated_at = CASE WHEN mode = 'airs_authoritative' THEN ? ELSE updated_at END
WHERE singleton = 1`, now)
	return err
}

func requireAirsAuthoritativeTx(tx *sql.Tx) (AirAuthority, error) {
	authority, err := airAuthorityTx(tx)
	if err != nil {
		return AirAuthority{}, err
	}
	if authority.Mode != "airs_authoritative" {
		return AirAuthority{}, fmt.Errorf("%w: authority mode %s", ErrAirRevision, authority.Mode)
	}
	return authority, nil
}

func appendAirAuditTx(tx *sql.Tx, airID, membershipID, inviteID string,
	actorID, orbitID int64, operation, oldValue, newValue, result string, now int64,
) error {
	var generation int64
	if err := tx.QueryRow(`SELECT generation FROM air_authority WHERE singleton = 1`).Scan(&generation); err != nil {
		return err
	}
	_, err := tx.Exec(`INSERT INTO air_audit_events(
  air_id, membership_id, invite_id, actor_id, orbit_id, operation,
  old_value, new_value, authority_generation, result_code, created_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, airID, membershipID, inviteID,
		actorID, orbitID, operation, oldValue, newValue, generation, result, now)
	return err
}

func commitAirRollbackHold(tx *sql.Tx, oldMode string, now int64) (AirAuthority, error) {
	if _, err := tx.Exec(`UPDATE air_authority SET mode = 'rollback_hold',
  generation = generation + 1, updated_at = ? WHERE singleton = 1`, now); err != nil {
		return AirAuthority{}, err
	}
	if err := appendAirAuditTx(tx, "", "", "", 0, 0,
		"air.authority.rollback", oldMode, "rollback_hold", "rollback_unsafe", now); err != nil {
		return AirAuthority{}, err
	}
	if err := tx.Commit(); err != nil {
		return AirAuthority{}, err
	}
	return AirAuthority{}, ErrAirRollbackUnsafe
}
