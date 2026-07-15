package store

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/base32"
	"errors"
	"fmt"
	"strings"
	"time"

	"relux.works/duet/coordinator/internal/ulid"
)

var (
	ErrAirApproachBusy               = errors.New("barycenter already has a current Air")
	ErrAirApproachNothingPending     = errors.New("no Air approach is pending")
	ErrAirApproachAmbiguous          = errors.New("multiple Air approaches need explicit selection")
	ErrAirApproachSwitchConfirmation = errors.New("another current Air requires explicit switch confirmation")
)

type AirApproachAliasResult struct {
	AirID         string
	InviteID      string
	MembershipID  string
	Code          string
	OwnerOrbitID  int64
	CallerOrbitID int64
	OtherOrbitID  int64
	OwnerTitle    string
	CallerTitle   string
	OtherTitle    string
	Outcome       string
	Replayed      bool
}

func airApproachCode(key []byte, inviteID string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("pulsar-air-approach-alias/v1:" + inviteID))
	return base32.StdEncoding.WithPadding(base32.NoPadding).EncodeToString(mac.Sum(nil))[:12]
}

func telegramAirAliasContextTx(tx *sql.Tx, telegramUserID int64) (ActorContext, error) {
	if _, err := requireAirsAuthoritativeTx(tx); err != nil {
		return ActorContext{}, err
	}
	ctx, err := resolveTelegramActorContext(tx, telegramUserID)
	if err != nil {
		return ActorContext{}, err
	}
	if !currentPrimary(ctx) {
		return ActorContext{}, ErrAirForbidden
	}
	return ctx, nil
}

func orbitTitleTx(tx *sql.Tx, orbitID int64) (string, error) {
	var title string
	err := tx.QueryRow(`SELECT title FROM orbits WHERE id = ?`, orbitID).Scan(&title)
	return title, err
}

// CreateAirApproachAlias implements the no-argument /approach operation as
// one transaction: parked Air, owner membership, creator pointer and invite.
// A Telegram retry returns the same derived code while the invite is open.
func (s *Store) CreateAirApproachAlias(telegramUserID, now int64) (AirApproachAliasResult, error) {
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AirApproachAliasResult{}, err
	}
	defer tx.Rollback()
	ctx, err := telegramAirAliasContextTx(tx, telegramUserID)
	if err != nil {
		return AirApproachAliasResult{}, err
	}
	key, err := airControlKeyTx(tx)
	if err != nil {
		return AirApproachAliasResult{}, err
	}
	currentAir, _, hasCurrent, err := activeAirForOrbitTx(tx, ctx.OrbitID)
	if err != nil {
		return AirApproachAliasResult{}, err
	}
	if hasCurrent {
		var inviteID, status string
		var expiresAt int64
		err := tx.QueryRow(`SELECT x.invite_id, i.status, i.expires_at
FROM air_approach_aliases x JOIN air_invites i ON i.public_id = x.invite_id
WHERE x.air_id = ? AND x.issuer_actor_id = ? AND x.issuer_orbit_id = ? AND x.status = 'open'`,
			currentAir, ctx.ActorID, ctx.OrbitID).Scan(&inviteID, &status, &expiresAt)
		if err == nil && status == "open" && expiresAt > now {
			title, titleErr := orbitTitleTx(tx, ctx.OrbitID)
			if titleErr != nil {
				return AirApproachAliasResult{}, titleErr
			}
			result := AirApproachAliasResult{AirID: currentAir, InviteID: inviteID,
				Code: airApproachCode(key, inviteID), OwnerOrbitID: ctx.OrbitID,
				CallerOrbitID: ctx.OrbitID, OwnerTitle: title, CallerTitle: title,
				Outcome: "invite_open", Replayed: true}
			if err := tx.Commit(); err != nil {
				return AirApproachAliasResult{}, err
			}
			return result, nil
		}
		if err == nil && status == "open" && expiresAt <= now {
			if _, err := tx.Exec(`UPDATE air_invites SET status = 'expired', revision = revision + 1,
  updated_at = ? WHERE public_id = ? AND status = 'open'`, now, inviteID); err != nil {
				return AirApproachAliasResult{}, err
			}
			if _, err := tx.Exec(`DELETE FROM air_active_pointers WHERE orbit_id = ? AND air_id = ?`,
				ctx.OrbitID, currentAir); err != nil {
				return AirApproachAliasResult{}, err
			}
			if _, err := tx.Exec(`UPDATE air_members SET status = 'left', left_at = ?, revision = revision + 1
WHERE air_id = ? AND status = 'joined'`, now, currentAir); err != nil {
				return AirApproachAliasResult{}, err
			}
			if _, err := tx.Exec(`UPDATE airs SET status = 'dissolved', dissolved_at = ?, revision = revision + 1
WHERE public_id = ? AND status = 'parked'`, now, currentAir); err != nil {
				return AirApproachAliasResult{}, err
			}
			if _, err := tx.Exec(`UPDATE air_approach_aliases SET status = 'closed', updated_at = ?
WHERE air_id = ? AND status = 'open'`, now, currentAir); err != nil {
				return AirApproachAliasResult{}, err
			}
			if err := markAirDivergenceTx(tx, now); err != nil {
				return AirApproachAliasResult{}, err
			}
			if err := appendAirAuditTx(tx, currentAir, "", inviteID, ctx.ActorID, ctx.OrbitID,
				"air.alias.approach.expire", "open", "expired", "ok", now); err != nil {
				return AirApproachAliasResult{}, err
			}
			hasCurrent = false
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return AirApproachAliasResult{}, err
		}
		if hasCurrent {
			return AirApproachAliasResult{}, ErrAirApproachBusy
		}
	}

	title, err := orbitTitleTx(tx, ctx.OrbitID)
	if err != nil {
		return AirApproachAliasResult{}, err
	}
	stamp := time.UnixMilli(now)
	airID := "air_" + ulid.New(stamp)
	memberID := "aim_" + ulid.New(stamp)
	inviteID := "ai_" + ulid.New(stamp)
	code := airApproachCode(key, inviteID)
	if _, err := tx.Exec(`INSERT INTO airs(public_id, title, status, owner_orbit_id, revision, created_at)
VALUES(?, ?, 'parked', ?, 1, ?)`, airID, title+" ⇄ сближение", ctx.OrbitID, now); err != nil {
		return AirApproachAliasResult{}, err
	}
	if _, err := tx.Exec(`INSERT INTO air_members(
  public_id, air_id, orbit_id, air_role, status, revision, joined_at, created_at
) VALUES(?, ?, ?, 'owner', 'joined', 1, ?, ?)`, memberID, airID, ctx.OrbitID, now, now); err != nil {
		return AirApproachAliasResult{}, err
	}
	if _, err := tx.Exec(`INSERT INTO air_policies(
  air_id, revision, invite_policy, overlay_policy, queue_policy, replace_policy, updated_at
) VALUES(?, 1, 'air_admin_primary', 'primary_companion', 'primary_companion', 'air_admin_primary', ?)`, airID, now); err != nil {
		return AirApproachAliasResult{}, err
	}
	if _, err := tx.Exec(`INSERT INTO air_active_pointers(orbit_id, air_id, revision, activated_at)
VALUES(?, ?, 1, ?)`, ctx.OrbitID, airID, now); err != nil {
		return AirApproachAliasResult{}, err
	}
	expiresAt := now + AirInviteTTL.Milliseconds()
	if _, err := tx.Exec(`INSERT INTO air_invites(
  public_id, air_id, code_hash, status, intended_role, issued_by_actor_id,
  issued_by_orbit_id, policy_revision, revision, expires_at, created_at, updated_at
) VALUES(?, ?, ?, 'open', 'member', ?, ?, 1, 1, ?, ?, ?)`, inviteID, airID,
		hashAirInviteCode(key, code), ctx.ActorID, ctx.OrbitID, expiresAt, now, now); err != nil {
		return AirApproachAliasResult{}, err
	}
	if _, err := tx.Exec(`INSERT INTO air_approach_aliases(
  air_id, invite_id, issuer_actor_id, issuer_orbit_id, status, created_at, updated_at
) VALUES(?, ?, ?, ?, 'open', ?, ?)`, airID, inviteID, ctx.ActorID, ctx.OrbitID, now, now); err != nil {
		return AirApproachAliasResult{}, err
	}
	if err := markAirDivergenceTx(tx, now); err != nil {
		return AirApproachAliasResult{}, err
	}
	if err := appendAirAuditTx(tx, airID, memberID, inviteID, ctx.ActorID, ctx.OrbitID,
		"air.alias.approach.create", "", "parked_invite", "ok", now); err != nil {
		return AirApproachAliasResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return AirApproachAliasResult{}, err
	}
	return AirApproachAliasResult{AirID: airID, InviteID: inviteID, MembershipID: memberID,
		Code: code, OwnerOrbitID: ctx.OrbitID, CallerOrbitID: ctx.OrbitID,
		OwnerTitle: title, CallerTitle: title, Outcome: "invite_open"}, nil
}

// ConsumeAirApproachAlias burns the invite and creates a pending membership;
// it deliberately does not change the claimant's current Air pointer.
func (s *Store) ConsumeAirApproachAlias(telegramUserID int64, code string, now int64) (AirApproachAliasResult, error) {
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	if len(code) != 12 {
		return AirApproachAliasResult{}, ErrAirInviteUnavailable
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AirApproachAliasResult{}, err
	}
	defer tx.Rollback()
	ctx, err := telegramAirAliasContextTx(tx, telegramUserID)
	if err != nil {
		return AirApproachAliasResult{}, err
	}
	key, err := airControlKeyTx(tx)
	if err != nil {
		return AirApproachAliasResult{}, err
	}
	var invite AirInvite
	var aliasStatus string
	var claimant sql.NullInt64
	err = tx.QueryRow(`SELECT i.public_id, i.air_id, i.status, i.intended_role,
  i.issued_by_actor_id, i.issued_by_orbit_id, i.policy_revision, i.revision,
  i.expires_at, i.consumed_membership_id, i.created_at, i.updated_at,
  x.status, x.claimant_orbit_id
FROM air_invites i JOIN air_approach_aliases x ON x.invite_id = i.public_id
WHERE i.code_hash = ?`, hashAirInviteCode(key, code)).Scan(
		&invite.ID, &invite.AirID, &invite.Status, &invite.IntendedRole,
		&invite.IssuedByActorID, &invite.IssuedByOrbitID, &invite.PolicyRevision,
		&invite.Revision, &invite.ExpiresAt, &invite.ConsumedMembershipID,
		&invite.CreatedAt, &invite.UpdatedAt, &aliasStatus, &claimant)
	if errors.Is(err, sql.ErrNoRows) {
		return AirApproachAliasResult{}, ErrAirInviteUnavailable
	}
	if err != nil {
		return AirApproachAliasResult{}, err
	}
	ownerTitle, err := orbitTitleTx(tx, invite.IssuedByOrbitID)
	if err != nil {
		return AirApproachAliasResult{}, err
	}
	callerTitle, err := orbitTitleTx(tx, ctx.OrbitID)
	if err != nil {
		return AirApproachAliasResult{}, err
	}
	base := AirApproachAliasResult{AirID: invite.AirID, InviteID: invite.ID,
		OwnerOrbitID: invite.IssuedByOrbitID, CallerOrbitID: ctx.OrbitID,
		OtherOrbitID: invite.IssuedByOrbitID, OwnerTitle: ownerTitle,
		CallerTitle: callerTitle, OtherTitle: ownerTitle, Outcome: "pending_confirmation"}
	if invite.Status == "consumed" && aliasStatus == "pending_confirmation" &&
		claimant.Valid && claimant.Int64 == ctx.OrbitID && invite.ConsumedMembershipID.Valid {
		base.MembershipID = invite.ConsumedMembershipID.String
		base.Replayed = true
		if err := tx.Commit(); err != nil {
			return AirApproachAliasResult{}, err
		}
		return base, nil
	}
	if invite.Status != "open" || invite.ExpiresAt <= now || aliasStatus != "open" {
		return AirApproachAliasResult{}, ErrAirInviteUnavailable
	}
	if invite.IssuedByOrbitID == ctx.OrbitID {
		return AirApproachAliasResult{}, ErrAirInvalid
	}
	var existing, occupants int
	if err := tx.QueryRow(`SELECT
  (SELECT COUNT(*) FROM air_members WHERE air_id = ? AND orbit_id = ? AND status IN ('pending_confirmation','joined')),
  (SELECT COUNT(*) FROM air_members WHERE air_id = ? AND status IN ('pending_confirmation','joined'))`,
		invite.AirID, ctx.OrbitID, invite.AirID).Scan(&existing, &occupants); err != nil {
		return AirApproachAliasResult{}, err
	}
	if existing != 0 {
		return AirApproachAliasResult{}, ErrAirAlreadyMember
	}
	if occupants >= AirBarycenterCapacity {
		return AirApproachAliasResult{}, ErrAirCapacity
	}
	membershipID := "aim_" + ulid.New(time.UnixMilli(now))
	if _, err := tx.Exec(`INSERT INTO air_members(
  public_id, air_id, orbit_id, air_role, status, revision, created_at
) VALUES(?, ?, ?, 'member', 'pending_confirmation', 1, ?)`, membershipID, invite.AirID, ctx.OrbitID, now); err != nil {
		return AirApproachAliasResult{}, err
	}
	changed, err := tx.Exec(`UPDATE air_invites SET status = 'consumed', consumed_membership_id = ?,
  revision = revision + 1, updated_at = ? WHERE public_id = ? AND status = 'open' AND expires_at > ?`,
		membershipID, now, invite.ID, now)
	if err != nil {
		return AirApproachAliasResult{}, err
	}
	if n, _ := changed.RowsAffected(); n != 1 {
		return AirApproachAliasResult{}, ErrAirInviteUnavailable
	}
	if _, err := tx.Exec(`UPDATE air_approach_aliases SET claimant_orbit_id = ?, membership_id = ?,
  status = 'pending_confirmation', updated_at = ? WHERE air_id = ? AND status = 'open'`,
		ctx.OrbitID, membershipID, now, invite.AirID); err != nil {
		return AirApproachAliasResult{}, err
	}
	if _, err := tx.Exec(`UPDATE airs SET title = ?, revision = revision + 1 WHERE public_id = ?`,
		ownerTitle+" ⇄ "+callerTitle, invite.AirID); err != nil {
		return AirApproachAliasResult{}, err
	}
	if err := markAirDivergenceTx(tx, now); err != nil {
		return AirApproachAliasResult{}, err
	}
	if err := appendAirAuditTx(tx, invite.AirID, membershipID, invite.ID, ctx.ActorID,
		ctx.OrbitID, "air.alias.approach.consume", "open", "pending_confirmation", "ok", now); err != nil {
		return AirApproachAliasResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return AirApproachAliasResult{}, err
	}
	base.MembershipID = membershipID
	return base, nil
}

func newestAliasCandidateTx(tx *sql.Tx, orbitID int64, status string) (AirApproachAliasResult, int64, error) {
	var newest int64
	if err := tx.QueryRow(`SELECT COALESCE(MAX(updated_at), 0) FROM air_approach_aliases
WHERE status = ? AND (issuer_orbit_id = ? OR claimant_orbit_id = ?)`, status, orbitID, orbitID).Scan(&newest); err != nil {
		return AirApproachAliasResult{}, 0, err
	}
	if newest == 0 {
		return AirApproachAliasResult{}, 0, ErrAirApproachNothingPending
	}
	var count int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM air_approach_aliases
WHERE status = ? AND updated_at = ? AND (issuer_orbit_id = ? OR claimant_orbit_id = ?)`,
		status, newest, orbitID, orbitID).Scan(&count); err != nil {
		return AirApproachAliasResult{}, 0, err
	}
	if count != 1 {
		return AirApproachAliasResult{}, 0, ErrAirApproachAmbiguous
	}
	var result AirApproachAliasResult
	err := tx.QueryRow(`SELECT x.air_id, x.invite_id, x.membership_id,
  x.issuer_orbit_id, x.claimant_orbit_id, owner.title, caller.title
FROM air_approach_aliases x
JOIN orbits owner ON owner.id = x.issuer_orbit_id
JOIN orbits caller ON caller.id = x.claimant_orbit_id
WHERE x.status = ? AND x.updated_at = ? AND (x.issuer_orbit_id = ? OR x.claimant_orbit_id = ?)`,
		status, newest, orbitID, orbitID).Scan(&result.AirID, &result.InviteID,
		&result.MembershipID, &result.OwnerOrbitID, &result.CallerOrbitID,
		&result.OwnerTitle, &result.CallerTitle)
	if err != nil {
		return AirApproachAliasResult{}, 0, err
	}
	if result.OwnerOrbitID == orbitID {
		result.OtherOrbitID, result.OtherTitle = result.CallerOrbitID, result.CallerTitle
	} else {
		result.OtherOrbitID, result.OtherTitle = result.OwnerOrbitID, result.OwnerTitle
	}
	return result, newest, nil
}

// ConfirmAirApproachAlias lets only the joining primary perform final
// confirmation. It never silently switches away from another current Air.
func (s *Store) ConfirmAirApproachAlias(telegramUserID, now int64) (AirApproachAliasResult, error) {
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AirApproachAliasResult{}, err
	}
	defer tx.Rollback()
	ctx, err := telegramAirAliasContextTx(tx, telegramUserID)
	if err != nil {
		return AirApproachAliasResult{}, err
	}
	result, _, err := newestAliasCandidateTx(tx, ctx.OrbitID, "pending_confirmation")
	if err != nil {
		// A duplicate Telegram delivery after a successful confirmation is a
		// stable replay only for the same joining barycenter and current Air.
		var joined AirApproachAliasResult
		joined, _, joinedErr := newestAliasCandidateTx(tx, ctx.OrbitID, "joined")
		if errors.Is(err, ErrAirApproachNothingPending) && joinedErr == nil &&
			joined.CallerOrbitID == ctx.OrbitID {
			current, _, ok, activeErr := activeAirForOrbitTx(tx, ctx.OrbitID)
			if activeErr == nil && ok && current == joined.AirID {
				joined.Outcome, joined.Replayed = "joined", true
				if commitErr := tx.Commit(); commitErr != nil {
					return AirApproachAliasResult{}, commitErr
				}
				return joined, nil
			}
		}
		return AirApproachAliasResult{}, err
	}
	if result.CallerOrbitID != ctx.OrbitID {
		return AirApproachAliasResult{}, ErrAirForbidden
	}
	current, pointerRevision, hasPointer, err := activeAirForOrbitTx(tx, ctx.OrbitID)
	if err != nil {
		return AirApproachAliasResult{}, err
	}
	if hasPointer && current != result.AirID {
		return AirApproachAliasResult{}, ErrAirApproachSwitchConfirmation
	}
	var memberRevision int64
	if err := tx.QueryRow(`SELECT revision FROM air_members WHERE public_id = ? AND status = 'pending_confirmation'`,
		result.MembershipID).Scan(&memberRevision); err != nil {
		return AirApproachAliasResult{}, ErrAirApproachNothingPending
	}
	if _, err := tx.Exec(`UPDATE air_members SET status = 'joined', joined_at = ?, revision = revision + 1
WHERE public_id = ? AND status = 'pending_confirmation' AND revision = ?`, now, result.MembershipID, memberRevision); err != nil {
		return AirApproachAliasResult{}, err
	}
	if !hasPointer {
		if _, err := tx.Exec(`INSERT INTO air_active_pointers(orbit_id, air_id, revision, activated_at)
VALUES(?, ?, ?, ?)`, ctx.OrbitID, result.AirID, pointerRevision+1, now); err != nil {
			return AirApproachAliasResult{}, err
		}
	}
	if err := refreshAirStatusTx(tx, result.AirID); err != nil {
		return AirApproachAliasResult{}, err
	}
	if err := bumpAirRevisionTx(tx, result.AirID); err != nil {
		return AirApproachAliasResult{}, err
	}
	if _, err := tx.Exec(`UPDATE air_approach_aliases SET status = 'joined', updated_at = ?
WHERE air_id = ? AND status = 'pending_confirmation'`, now, result.AirID); err != nil {
		return AirApproachAliasResult{}, err
	}
	if err := markAirDivergenceTx(tx, now); err != nil {
		return AirApproachAliasResult{}, err
	}
	if err := appendAirAuditTx(tx, result.AirID, result.MembershipID, result.InviteID,
		ctx.ActorID, ctx.OrbitID, "air.alias.approach.confirm", "pending_confirmation", "joined", "ok", now); err != nil {
		return AirApproachAliasResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return AirApproachAliasResult{}, err
	}
	result.Outcome = "joined"
	return result, nil
}

func closeAirApproachTx(tx *sql.Tx, result AirApproachAliasResult, ctx ActorContext, outcome string, now int64) error {
	if result.MembershipID != "" {
		if _, err := tx.Exec(`UPDATE air_members SET status = 'left', left_at = ?, revision = revision + 1
WHERE public_id = ? AND status = 'pending_confirmation'`, now, result.MembershipID); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`UPDATE air_invites SET status = CASE WHEN status = 'open' THEN 'withdrawn' ELSE status END,
  revision = revision + CASE WHEN status = 'open' THEN 1 ELSE 0 END, updated_at = ? WHERE public_id = ?`,
		now, result.InviteID); err != nil {
		return err
	}
	if _, err := tx.Exec(`DELETE FROM air_active_pointers WHERE air_id = ?`, result.AirID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE air_members SET status = 'left', left_at = ?, revision = revision + 1
WHERE air_id = ? AND status = 'joined'`, now, result.AirID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE airs SET status = 'dissolved', dissolved_at = ?, revision = revision + 1
WHERE public_id = ? AND status <> 'dissolved'`, now, result.AirID); err != nil {
		return err
	}
	if _, err := tx.Exec(`UPDATE air_approach_aliases SET status = 'closed', updated_at = ? WHERE air_id = ?`,
		now, result.AirID); err != nil {
		return err
	}
	if err := markAirDivergenceTx(tx, now); err != nil {
		return err
	}
	return appendAirAuditTx(tx, result.AirID, result.MembershipID, result.InviteID,
		ctx.ActorID, ctx.OrbitID, "air.alias.approach.decline", "pending", outcome, "ok", now)
}

// DeclineAirApproachAlias handles claimant decline, issuer cancellation of a
// claim, and issuer withdrawal of its newest still-open invite.
func (s *Store) DeclineAirApproachAlias(telegramUserID, now int64) (AirApproachAliasResult, error) {
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AirApproachAliasResult{}, err
	}
	defer tx.Rollback()
	ctx, err := telegramAirAliasContextTx(tx, telegramUserID)
	if err != nil {
		return AirApproachAliasResult{}, err
	}
	result, _, pendingErr := newestAliasCandidateTx(tx, ctx.OrbitID, "pending_confirmation")
	if pendingErr == nil {
		if result.CallerOrbitID == ctx.OrbitID {
			result.Outcome = "declined"
		} else if result.OwnerOrbitID == ctx.OrbitID {
			result.Outcome = "cancelled"
		} else {
			return AirApproachAliasResult{}, ErrAirForbidden
		}
		if err := closeAirApproachTx(tx, result, ctx, result.Outcome, now); err != nil {
			return AirApproachAliasResult{}, err
		}
		if err := tx.Commit(); err != nil {
			return AirApproachAliasResult{}, err
		}
		return result, nil
	}
	if !errors.Is(pendingErr, ErrAirApproachNothingPending) {
		return AirApproachAliasResult{}, pendingErr
	}
	var resultOpen AirApproachAliasResult
	err = tx.QueryRow(`SELECT x.air_id, x.invite_id, x.issuer_orbit_id, owner.title
FROM air_approach_aliases x JOIN orbits owner ON owner.id = x.issuer_orbit_id
WHERE x.status = 'open' AND x.issuer_orbit_id = ? ORDER BY x.updated_at DESC, x.air_id DESC LIMIT 1`,
		ctx.OrbitID).Scan(&resultOpen.AirID, &resultOpen.InviteID, &resultOpen.OwnerOrbitID, &resultOpen.OwnerTitle)
	if errors.Is(err, sql.ErrNoRows) {
		return AirApproachAliasResult{}, ErrAirApproachNothingPending
	}
	if err != nil {
		return AirApproachAliasResult{}, err
	}
	resultOpen.CallerOrbitID = ctx.OrbitID
	resultOpen.Outcome = "withdrawn"
	if err := closeAirApproachTx(tx, resultOpen, ctx, resultOpen.Outcome, now); err != nil {
		return AirApproachAliasResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return AirApproachAliasResult{}, err
	}
	return resultOpen, nil
}

// LeaveCurrentAirAlias implements /apart for both migrated pairwise Airs and
// newly-created alias Airs. Only the caller leaves; remaining members and
// their pointers are untouched. Owner departure transfers ownership to the
// oldest remaining joined member so the Air stays structurally valid.
func (s *Store) LeaveCurrentAirAlias(telegramUserID, now int64) (AirApproachAliasResult, error) {
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AirApproachAliasResult{}, err
	}
	defer tx.Rollback()
	ctx, err := telegramAirAliasContextTx(tx, telegramUserID)
	if err != nil {
		return AirApproachAliasResult{}, err
	}
	airID, pointerRevision, ok, err := activeAirForOrbitTx(tx, ctx.OrbitID)
	if err != nil {
		return AirApproachAliasResult{}, err
	}
	if !ok {
		return AirApproachAliasResult{}, ErrAirApproachNothingPending
	}
	var member AirMember
	var airStatus string
	err = tx.QueryRow(`SELECT m.public_id, m.air_role, m.revision, a.status
FROM air_members m JOIN airs a ON a.public_id = m.air_id
WHERE m.air_id = ? AND m.orbit_id = ? AND m.status = 'joined'`, airID, ctx.OrbitID).
		Scan(&member.ID, &member.Role, &member.Revision, &airStatus)
	if err != nil || airStatus != "active" {
		return AirApproachAliasResult{}, ErrAirApproachNothingPending
	}
	result := AirApproachAliasResult{AirID: airID, CallerOrbitID: ctx.OrbitID, Outcome: "left"}
	result.CallerTitle, err = orbitTitleTx(tx, ctx.OrbitID)
	if err != nil {
		return AirApproachAliasResult{}, err
	}
	_ = tx.QueryRow(`SELECT m.orbit_id, o.title FROM air_members m JOIN orbits o ON o.id = m.orbit_id
WHERE m.air_id = ? AND m.status = 'joined' AND m.orbit_id <> ? ORDER BY m.joined_at, m.orbit_id LIMIT 1`,
		airID, ctx.OrbitID).Scan(&result.OtherOrbitID, &result.OtherTitle)
	changed, err := tx.Exec(`DELETE FROM air_active_pointers WHERE orbit_id = ? AND air_id = ? AND revision = ?`,
		ctx.OrbitID, airID, pointerRevision)
	if err != nil {
		return AirApproachAliasResult{}, err
	}
	if n, _ := changed.RowsAffected(); n != 1 {
		return AirApproachAliasResult{}, ErrAirActiveChanged
	}
	if _, err := tx.Exec(`UPDATE air_members SET status = 'left', left_at = ?, revision = revision + 1
WHERE public_id = ? AND status = 'joined' AND revision = ?`, now, member.ID, member.Revision); err != nil {
		return AirApproachAliasResult{}, err
	}
	if member.Role == "owner" && result.OtherOrbitID != 0 {
		if _, err := tx.Exec(`UPDATE air_members SET air_role = 'owner', revision = revision + 1
WHERE air_id = ? AND orbit_id = ? AND status = 'joined'`, airID, result.OtherOrbitID); err != nil {
			return AirApproachAliasResult{}, err
		}
		if _, err := tx.Exec(`UPDATE airs SET owner_orbit_id = ? WHERE public_id = ?`, result.OtherOrbitID, airID); err != nil {
			return AirApproachAliasResult{}, err
		}
	}
	if err := refreshAirStatusTx(tx, airID); err != nil {
		return AirApproachAliasResult{}, err
	}
	if err := bumpAirRevisionTx(tx, airID); err != nil {
		return AirApproachAliasResult{}, err
	}
	if _, err := tx.Exec(`UPDATE air_approach_aliases SET status = 'closed', updated_at = ?
WHERE air_id = ? AND status = 'joined'`, now, airID); err != nil {
		return AirApproachAliasResult{}, err
	}
	if err := markAirDivergenceTx(tx, now); err != nil {
		return AirApproachAliasResult{}, err
	}
	if err := appendAirAuditTx(tx, airID, member.ID, "", ctx.ActorID, ctx.OrbitID,
		"air.alias.apart", "joined", fmt.Sprintf("left:pointer_revision=%d", pointerRevision), "ok", now); err != nil {
		return AirApproachAliasResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return AirApproachAliasResult{}, err
	}
	return result, nil
}
