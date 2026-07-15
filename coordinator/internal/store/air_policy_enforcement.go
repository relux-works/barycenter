package store

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
)

var airPolicyAirIDPattern = regexp.MustCompile(`^air_[0-9A-HJKMNP-TV-Z]{26}$`)

// AirPolicyOperation is the frozen authorization vocabulary shared by app,
// Telegram and compatibility transports.
type AirPolicyOperation string

const (
	AirPolicyOverlay AirPolicyOperation = "overlay"
	AirPolicyQueue   AirPolicyOperation = "queue"
	AirPolicyReplace AirPolicyOperation = "replace"
)

type airPolicyContext struct {
	AirID          string
	AirStatus      string
	MemberID       string
	MemberRole     string
	PolicyRevision int64
	InvitePolicy   string
	OverlayPolicy  string
	QueuePolicy    string
	ReplacePolicy  string
}

type AirPolicyAuthorization struct {
	AirID          string
	PolicyRevision int64
	Operation      AirPolicyOperation
	Result         string
}

func (s *Store) AuthorizeAirActionForIdentity(
	identity Identity,
	operation AirPolicyOperation,
) (AirPolicyAuthorization, error) {
	if !validAirPolicyOperation(operation) {
		return AirPolicyAuthorization{}, ErrAirInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AirPolicyAuthorization{}, err
	}
	defer tx.Rollback()
	authority, err := airAuthorityTx(tx)
	if err != nil {
		return AirPolicyAuthorization{}, err
	}
	if authority.Mode != "airs_authoritative" {
		return AirPolicyAuthorization{Operation: operation, Result: "allowed"}, tx.Commit()
	}
	ctx, err := resolveActorContext(tx, identity)
	if err != nil && !errors.Is(err, ErrInsufficientCapability) {
		return AirPolicyAuthorization{}, err
	}
	if (!ctx.Capabilities.Has(CapabilityControl) &&
		!ctx.Capabilities.Has(CapabilityTelegram)) || ctx.OrbitID <= 0 || ctx.ActorID <= 0 {
		return AirPolicyAuthorization{}, ErrInsufficientCapability
	}
	context, err := activeAirPolicyContextTx(tx, ctx.OrbitID)
	if err != nil {
		return AirPolicyAuthorization{}, err
	}
	authorization, err := authorizeAirPolicyTx(ctx, context, operation)
	if err != nil {
		return AirPolicyAuthorization{}, err
	}
	return authorization, tx.Commit()
}

// AuthorizeInstallationAirAction re-resolves the current installation and
// membership behind an already authenticated node event. A stale or replaced
// slot cannot inherit the new installation's Air permission.
func (s *Store) AuthorizeInstallationAirAction(
	orbitID int64,
	slot string,
	operation AirPolicyOperation,
) (AirPolicyAuthorization, error) {
	if orbitID <= 0 || !transmissionSlotPattern.MatchString(slot) ||
		!validAirPolicyOperation(operation) {
		return AirPolicyAuthorization{}, ErrAirInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AirPolicyAuthorization{}, err
	}
	defer tx.Rollback()
	authority, err := airAuthorityTx(tx)
	if err != nil {
		return AirPolicyAuthorization{}, err
	}
	if authority.Mode != "airs_authoritative" {
		return AirPolicyAuthorization{Operation: operation, Result: "allowed"}, tx.Commit()
	}
	var ctx ActorContext
	err = tx.QueryRow(`SELECT ic.slot_orbit_id, ic.actor_id, ic.slot_name, m.role
FROM installation_credentials ic
JOIN actors a ON a.id = ic.actor_id AND a.revoked_at IS NULL
JOIN memberships m ON m.actor_id = ic.actor_id
  AND m.orbit_id = ic.slot_orbit_id AND m.left_at IS NULL
JOIN orbits o ON o.id = ic.slot_orbit_id AND o.status = 'active'
JOIN slots sl ON sl.orbit_id = ic.slot_orbit_id AND sl.slot = ic.slot_name
  AND sl.revoked_at IS NULL AND sl.token_hash = ic.binding_token_hash
  AND COALESCE(sl.paired_at, 0) = ic.slot_paired_at
WHERE ic.slot_orbit_id = ? AND ic.slot_name = ?`, orbitID, slot).Scan(
		&ctx.OrbitID, &ctx.ActorID, &ctx.Slot, &ctx.Role,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return AirPolicyAuthorization{}, ErrUnauthorized
	}
	if err != nil {
		return AirPolicyAuthorization{}, err
	}
	ctx.Capabilities = CapabilityNode
	context, err := activeAirPolicyContextTx(tx, ctx.OrbitID)
	if err != nil {
		return AirPolicyAuthorization{}, err
	}
	authorization, err := authorizeAirPolicyTx(ctx, context, operation)
	if err != nil {
		return AirPolicyAuthorization{}, err
	}
	return authorization, tx.Commit()
}

func validAirPolicyOperation(operation AirPolicyOperation) bool {
	return operation == AirPolicyOverlay || operation == AirPolicyQueue ||
		operation == AirPolicyReplace
}

func airPolicyOperationForDelivery(delivery TransmissionDelivery) AirPolicyOperation {
	switch delivery {
	case TransmissionDeliveryAfterCurrent:
		return AirPolicyQueue
	default:
		return AirPolicyOverlay
	}
}

// activeAirPolicyContextTx intentionally returns no context before Air
// authority cutover. That preserves the frozen legacy migration window while
// making Air policy the only shared authority after cutover.
func activeAirPolicyContextTx(tx *sql.Tx, orbitID int64) (*airPolicyContext, error) {
	authority, err := airAuthorityTx(tx)
	if err != nil {
		return nil, err
	}
	if authority.Mode != "airs_authoritative" {
		return nil, nil
	}
	var context airPolicyContext
	err = tx.QueryRow(`SELECT a.public_id, a.status, m.public_id, m.air_role,
  p.revision, p.invite_policy, p.overlay_policy, p.queue_policy, p.replace_policy
FROM air_active_pointers ap
JOIN airs a ON a.public_id = ap.air_id AND a.status <> 'dissolved'
JOIN air_members m ON m.air_id = ap.air_id AND m.orbit_id = ap.orbit_id
  AND m.status = 'joined'
JOIN air_policies p ON p.air_id = ap.air_id
WHERE ap.orbit_id = ?`, orbitID).Scan(
		&context.AirID, &context.AirStatus, &context.MemberID, &context.MemberRole,
		&context.PolicyRevision, &context.InvitePolicy, &context.OverlayPolicy,
		&context.QueuePolicy, &context.ReplacePolicy,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	return &context, err
}

func airPolicyValue(context airPolicyContext, operation AirPolicyOperation) string {
	switch operation {
	case AirPolicyOverlay:
		return context.OverlayPolicy
	case AirPolicyQueue:
		return context.QueuePolicy
	case AirPolicyReplace:
		return context.ReplacePolicy
	default:
		return "disabled"
	}
}

func airPolicyAllows(ctx ActorContext, memberRole, policy string) bool {
	switch policy {
	case "owner_primary":
		return ctx.Role == "primary" && memberRole == "owner"
	case "air_admin_primary":
		return ctx.Role == "primary" && (memberRole == "owner" || memberRole == "admin")
	case "all_member_primaries":
		return ctx.Role == "primary"
	case "primary_companion":
		return ctx.Role == "primary" || ctx.Role == "companion"
	default:
		return false
	}
}

func authorizeAirPolicyTx(
	ctx ActorContext,
	context *airPolicyContext,
	operation AirPolicyOperation,
) (AirPolicyAuthorization, error) {
	if !validAirPolicyOperation(operation) {
		return AirPolicyAuthorization{}, ErrAirInvalid
	}
	// A personal barycenter has no Air permission layer. Local ACL/DND/block
	// still apply at the common target acceptance boundary.
	if context == nil {
		return AirPolicyAuthorization{Operation: operation, Result: "allowed"}, nil
	}
	if !airPolicyAllows(ctx, context.MemberRole, airPolicyValue(*context, operation)) {
		return AirPolicyAuthorization{}, ErrAirPolicyDenied
	}
	return AirPolicyAuthorization{
		AirID: context.AirID, PolicyRevision: context.PolicyRevision,
		Operation: operation, Result: "allowed",
	}, nil
}

func airPlaybackDomainTx(tx *sql.Tx, airID string) (int64, error) {
	if _, err := tx.Exec(`INSERT INTO air_playback_domains(air_id)
VALUES(?) ON CONFLICT(air_id) DO NOTHING`, airID); err != nil {
		return 0, err
	}
	var id int64
	if err := tx.QueryRow(`SELECT id FROM air_playback_domains WHERE air_id = ?`, airID).Scan(&id); err != nil {
		return 0, err
	}
	if id <= 0 {
		return 0, fmt.Errorf("Air %s has invalid playback domain", airID)
	}
	return id, nil
}
