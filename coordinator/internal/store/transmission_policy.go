package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

var (
	ErrTransmissionPolicyForbidden = errors.New("transmission policy mutation is forbidden")
	ErrDNDRevisionConflict         = errors.New("DND revision is stale or conflicting")
)

type BlockOwnerScope string

const (
	BlockOwnerActor BlockOwnerScope = "actor"
	BlockOwnerOrbit BlockOwnerScope = "orbit"
)

type BlockedSubjectKind string

const (
	BlockedSubjectActor BlockedSubjectKind = "actor"
	BlockedSubjectOrbit BlockedSubjectKind = "orbit"
)

type TransmissionBlock struct {
	ID               int64
	OwnerScope       BlockOwnerScope
	OwnerOrbitID     int64
	OwnerActorID     int64
	BlockedKind      BlockedSubjectKind
	BlockedActorID   int64
	BlockedOrbitID   int64
	CreatedByActorID int64
	CreatedAt        int64
	RevokedAt        int64
	Revision         int64
}

type CreateTransmissionBlockParams struct {
	OwnerScope          BlockOwnerScope
	OwnerOrbitID        int64
	OwnerActorID        int64
	BlockedKind         BlockedSubjectKind
	BlockedActorID      int64
	BlockedOrbitID      int64
	AuthorizedByActorID int64
	CreatedAt           int64
}

type TransmissionBlockCreation struct {
	Block  TransmissionBlock
	Reused bool
}

type TransmissionBlockDecision struct {
	Blocked bool
	Reason  TransmissionReason
}

const transmissionBlockColumns = `id, owner_scope, owner_orbit_id,
owner_actor_id, blocked_kind, blocked_actor_id, blocked_orbit_id,
created_by_actor_id, created_at, revoked_at, revision`

func scanTransmissionBlock(row sqlScanner) (TransmissionBlock, error) {
	var block TransmissionBlock
	err := row.Scan(
		&block.ID, &block.OwnerScope, &block.OwnerOrbitID, &block.OwnerActorID,
		&block.BlockedKind, &block.BlockedActorID, &block.BlockedOrbitID,
		&block.CreatedByActorID, &block.CreatedAt, &block.RevokedAt,
		&block.Revision,
	)
	return block, err
}

func validateCreateTransmissionBlock(params CreateTransmissionBlockParams) error {
	if params.OwnerOrbitID <= 0 || params.AuthorizedByActorID <= 0 ||
		params.CreatedAt <= 0 {
		return ErrTransmissionInvalid
	}
	switch params.OwnerScope {
	case BlockOwnerActor:
		if params.OwnerActorID <= 0 ||
			params.AuthorizedByActorID != params.OwnerActorID {
			return ErrTransmissionPolicyForbidden
		}
	case BlockOwnerOrbit:
		if params.OwnerActorID != 0 {
			return ErrTransmissionInvalid
		}
	default:
		return ErrTransmissionInvalid
	}
	switch params.BlockedKind {
	case BlockedSubjectActor:
		if params.BlockedActorID <= 0 || params.BlockedOrbitID != 0 ||
			params.BlockedActorID == params.OwnerActorID {
			return ErrTransmissionInvalid
		}
	case BlockedSubjectOrbit:
		if params.BlockedActorID != 0 || params.BlockedOrbitID <= 0 ||
			params.BlockedOrbitID == params.OwnerOrbitID {
			return ErrTransmissionInvalid
		}
	default:
		return ErrTransmissionInvalid
	}
	return nil
}

func authorizeBlockOwnerTx(
	tx *sql.Tx,
	ownerScope BlockOwnerScope,
	ownerOrbitID, ownerActorID, authorizedByActorID int64,
) error {
	var role string
	var revoked sql.NullInt64
	err := tx.QueryRow(`SELECT m.role, a.revoked_at
FROM memberships m
JOIN actors a ON a.id = m.actor_id
JOIN orbits o ON o.id = m.orbit_id AND o.status = 'active'
WHERE m.orbit_id = ? AND m.actor_id = ? AND m.left_at IS NULL`,
		ownerOrbitID, authorizedByActorID,
	).Scan(&role, &revoked)
	if errors.Is(err, sql.ErrNoRows) || revoked.Valid {
		return ErrTransmissionPolicyForbidden
	}
	if err != nil {
		return err
	}
	if ownerScope == BlockOwnerActor {
		if authorizedByActorID != ownerActorID {
			return ErrTransmissionPolicyForbidden
		}
		return nil
	}
	if role != "primary" {
		return ErrTransmissionPolicyForbidden
	}
	return nil
}

func validateBlockedSubjectTx(tx *sql.Tx, params CreateTransmissionBlockParams) error {
	var matches int
	var err error
	if params.BlockedKind == BlockedSubjectActor {
		err = tx.QueryRow(`SELECT COUNT(*) FROM actors WHERE id = ?`,
			params.BlockedActorID).Scan(&matches)
	} else {
		err = tx.QueryRow(`SELECT COUNT(*) FROM orbits WHERE id = ?`,
			params.BlockedOrbitID).Scan(&matches)
	}
	if err != nil {
		return err
	}
	if matches != 1 {
		return ErrTransmissionInvalid
	}
	return nil
}

func (s *Store) CreateTransmissionBlock(
	params CreateTransmissionBlockParams,
) (TransmissionBlockCreation, error) {
	if err := validateCreateTransmissionBlock(params); err != nil {
		return TransmissionBlockCreation{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return TransmissionBlockCreation{}, err
	}
	defer tx.Rollback()
	creation, err := createTransmissionBlockTx(tx, params)
	if err != nil {
		return TransmissionBlockCreation{}, err
	}
	if err := s.checkpoint("transmission_block_create_before_commit"); err != nil {
		return TransmissionBlockCreation{}, err
	}
	if err := tx.Commit(); err != nil {
		return TransmissionBlockCreation{}, err
	}
	return creation, nil
}

func createTransmissionBlockTx(
	tx *sql.Tx,
	params CreateTransmissionBlockParams,
) (TransmissionBlockCreation, error) {
	if err := authorizeBlockOwnerTx(
		tx, params.OwnerScope, params.OwnerOrbitID, params.OwnerActorID,
		params.AuthorizedByActorID,
	); err != nil {
		return TransmissionBlockCreation{}, err
	}
	if err := validateBlockedSubjectTx(tx, params); err != nil {
		return TransmissionBlockCreation{}, err
	}
	existing, err := scanTransmissionBlock(tx.QueryRow(
		`SELECT `+transmissionBlockColumns+` FROM blocks
WHERE owner_scope = ? AND owner_orbit_id = ? AND owner_actor_id = ?
  AND blocked_kind = ? AND blocked_actor_id = ? AND blocked_orbit_id = ?
  AND revoked_at = 0`,
		params.OwnerScope, params.OwnerOrbitID, params.OwnerActorID,
		params.BlockedKind, params.BlockedActorID, params.BlockedOrbitID,
	))
	if err == nil {
		return TransmissionBlockCreation{Block: existing, Reused: true}, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return TransmissionBlockCreation{}, err
	}
	result, err := tx.Exec(`INSERT INTO blocks(
  owner_scope, owner_orbit_id, owner_actor_id, blocked_kind,
  blocked_actor_id, blocked_orbit_id, created_by_actor_id, created_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`,
		params.OwnerScope, params.OwnerOrbitID, params.OwnerActorID,
		params.BlockedKind, params.BlockedActorID, params.BlockedOrbitID,
		params.AuthorizedByActorID, params.CreatedAt,
	)
	if err != nil {
		return TransmissionBlockCreation{}, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return TransmissionBlockCreation{}, err
	}
	block, err := scanTransmissionBlock(tx.QueryRow(
		`SELECT `+transmissionBlockColumns+` FROM blocks WHERE id = ?`, id,
	))
	if err != nil {
		return TransmissionBlockCreation{}, err
	}
	return TransmissionBlockCreation{Block: block}, nil
}

func (s *Store) RevokeTransmissionBlock(
	id, authorizedByActorID, expectedRevision, now int64,
) (TransmissionBlock, error) {
	if id <= 0 || authorizedByActorID <= 0 || expectedRevision <= 0 || now <= 0 {
		return TransmissionBlock{}, ErrTransmissionInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return TransmissionBlock{}, err
	}
	defer tx.Rollback()
	block, err := scanTransmissionBlock(tx.QueryRow(
		`SELECT `+transmissionBlockColumns+` FROM blocks WHERE id = ?`, id,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return TransmissionBlock{}, ErrTransmissionNotFound
	}
	if err != nil {
		return TransmissionBlock{}, err
	}
	if block.RevokedAt != 0 || block.Revision != expectedRevision || now < block.CreatedAt {
		return TransmissionBlock{}, ErrTransmissionStateConflict
	}
	if err := authorizeBlockOwnerTx(
		tx, block.OwnerScope, block.OwnerOrbitID, block.OwnerActorID,
		authorizedByActorID,
	); err != nil {
		return TransmissionBlock{}, err
	}
	result, err := tx.Exec(`UPDATE blocks
SET revoked_at = ?, revision = revision + 1
WHERE id = ? AND revoked_at = 0 AND revision = ?`, now, id, expectedRevision)
	if err != nil {
		return TransmissionBlock{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return TransmissionBlock{}, err
		}
		return TransmissionBlock{}, ErrTransmissionStateConflict
	}
	block, err = scanTransmissionBlock(tx.QueryRow(
		`SELECT `+transmissionBlockColumns+` FROM blocks WHERE id = ?`, id,
	))
	if err != nil {
		return TransmissionBlock{}, err
	}
	if err := tx.Commit(); err != nil {
		return TransmissionBlock{}, err
	}
	return block, nil
}

func (s *Store) TransmissionBlockDecision(
	ctx context.Context,
	recipientOrbitID, recipientActorID, sourceOrbitID, sourceActorID int64,
) (TransmissionBlockDecision, error) {
	if ctx == nil {
		return TransmissionBlockDecision{}, errors.New("nil block lookup context")
	}
	if recipientOrbitID <= 0 || recipientActorID <= 0 ||
		sourceOrbitID <= 0 || sourceActorID <= 0 {
		return TransmissionBlockDecision{}, ErrTransmissionInvalid
	}
	var actorBlocks, orbitBlocks int
	err := s.db.QueryRowContext(ctx, `SELECT
  COALESCE(SUM(CASE WHEN blocked_kind = 'actor' AND blocked_actor_id = ? THEN 1 ELSE 0 END), 0),
  COALESCE(SUM(CASE WHEN blocked_kind = 'orbit' AND blocked_orbit_id = ? THEN 1 ELSE 0 END), 0)
FROM blocks
WHERE revoked_at = 0 AND owner_orbit_id = ?
  AND (owner_scope = 'orbit' OR (owner_scope = 'actor' AND owner_actor_id = ?))`,
		sourceActorID, sourceOrbitID, recipientOrbitID, recipientActorID,
	).Scan(&actorBlocks, &orbitBlocks)
	if err != nil {
		return TransmissionBlockDecision{}, err
	}
	if actorBlocks > 0 {
		return TransmissionBlockDecision{
			Blocked: true, Reason: TransmissionReasonActorBlocked,
		}, nil
	}
	if orbitBlocks > 0 {
		return TransmissionBlockDecision{
			Blocked: true, Reason: TransmissionReasonOrbitBlocked,
		}, nil
	}
	return TransmissionBlockDecision{}, nil
}

type DNDMode string

const (
	DNDAllowAll     DNDMode = "allow_all"
	DNDMessagesOnly DNDMode = "messages_only"
	DNDMutedUntil   DNDMode = "muted_until"
)

type DNDSetting struct {
	OrbitID          int64
	ActorID          int64
	Slot             string
	BindingPairedAt  int64
	Mode             DNDMode
	MutedUntil       int64
	Revision         int64
	UpdatedByActorID int64
	UpdatedAt        int64
}

type SetNodeDNDParams struct {
	OrbitID    int64
	ActorID    int64
	Slot       string
	Mode       DNDMode
	MutedUntil int64
	Revision   int64
	UpdatedAt  int64
}

type SetOrbitDNDParams struct {
	OrbitID             int64
	AuthorizedByActorID int64
	Mode                DNDMode
	MutedUntil          int64
	Revision            int64
	UpdatedAt           int64
}

type DNDMutation struct {
	Setting DNDSetting
	Changed bool
}

type EffectiveDND struct {
	Mode          DNDMode
	MutedUntil    int64
	Reason        TransmissionReason
	NodeRevision  int64
	OrbitRevision int64
}

func validateDND(mode DNDMode, mutedUntil, now int64) error {
	if now <= 0 {
		return ErrTransmissionInvalid
	}
	switch mode {
	case DNDAllowAll, DNDMessagesOnly:
		if mutedUntil != 0 {
			return ErrTransmissionInvalid
		}
	case DNDMutedUntil:
		if mutedUntil <= now ||
			mutedUntil > now+int64((30*24*time.Hour)/time.Millisecond) {
			return ErrTransmissionInvalid
		}
	default:
		return ErrTransmissionInvalid
	}
	return nil
}

func resolveLiveBindingPairedAtTx(
	tx *sql.Tx,
	orbitID, actorID int64,
	slot string,
) (int64, error) {
	return resolveTransmissionTargetBindingTx(tx, CreateTransmissionTarget{
		OrbitID: orbitID, ActorID: actorID, Slot: slot,
	})
}

func (s *Store) SetNodeDND(params SetNodeDNDParams) (DNDMutation, error) {
	if params.OrbitID <= 0 || params.ActorID <= 0 || params.Revision <= 0 ||
		!transmissionSlotPattern.MatchString(params.Slot) {
		return DNDMutation{}, ErrTransmissionInvalid
	}
	if err := validateDND(params.Mode, params.MutedUntil, params.UpdatedAt); err != nil {
		return DNDMutation{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return DNDMutation{}, err
	}
	defer tx.Rollback()
	pairedAt, err := resolveLiveBindingPairedAtTx(
		tx, params.OrbitID, params.ActorID, params.Slot,
	)
	if err != nil {
		if errors.Is(err, ErrTransmissionTargetInvalid) {
			return DNDMutation{}, ErrTransmissionPolicyForbidden
		}
		return DNDMutation{}, err
	}
	var current DNDSetting
	err = tx.QueryRow(`SELECT orbit_id, actor_id, slot, binding_paired_at,
       mode, muted_until, revision, updated_at
FROM node_dnd_settings
WHERE orbit_id = ? AND actor_id = ? AND slot = ?`,
		params.OrbitID, params.ActorID, params.Slot,
	).Scan(&current.OrbitID, &current.ActorID, &current.Slot,
		&current.BindingPairedAt, &current.Mode, &current.MutedUntil,
		&current.Revision, &current.UpdatedAt)
	if err == nil {
		if params.Revision < current.Revision ||
			(params.Revision == current.Revision &&
				(params.Mode != current.Mode || params.MutedUntil != current.MutedUntil)) {
			return DNDMutation{}, ErrDNDRevisionConflict
		}
		if params.Revision == current.Revision {
			if err := tx.Commit(); err != nil {
				return DNDMutation{}, err
			}
			return DNDMutation{Setting: current}, nil
		}
		if params.UpdatedAt < current.UpdatedAt {
			return DNDMutation{}, ErrDNDRevisionConflict
		}
		_, err = tx.Exec(`UPDATE node_dnd_settings
SET binding_paired_at = ?, mode = ?, muted_until = ?, revision = ?, updated_at = ?
WHERE orbit_id = ? AND actor_id = ? AND slot = ?`,
			pairedAt, params.Mode, params.MutedUntil, params.Revision,
			params.UpdatedAt, params.OrbitID, params.ActorID, params.Slot)
	} else if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.Exec(`INSERT INTO node_dnd_settings(
  orbit_id, actor_id, slot, binding_paired_at, mode, muted_until, revision, updated_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, params.OrbitID, params.ActorID,
			params.Slot, pairedAt, params.Mode, params.MutedUntil,
			params.Revision, params.UpdatedAt)
	}
	if err != nil {
		return DNDMutation{}, err
	}
	current = DNDSetting{}
	err = tx.QueryRow(`SELECT orbit_id, actor_id, slot, binding_paired_at,
       mode, muted_until, revision, updated_at
FROM node_dnd_settings
WHERE orbit_id = ? AND actor_id = ? AND slot = ?`,
		params.OrbitID, params.ActorID, params.Slot,
	).Scan(&current.OrbitID, &current.ActorID, &current.Slot,
		&current.BindingPairedAt, &current.Mode, &current.MutedUntil,
		&current.Revision, &current.UpdatedAt)
	if err != nil {
		return DNDMutation{}, err
	}
	if err := tx.Commit(); err != nil {
		return DNDMutation{}, err
	}
	return DNDMutation{Setting: current, Changed: true}, nil
}

func authorizeOrbitDNDMutationTx(tx *sql.Tx, orbitID, actorID int64) error {
	var role string
	var revoked sql.NullInt64
	err := tx.QueryRow(`SELECT m.role, a.revoked_at
FROM memberships m
JOIN actors a ON a.id = m.actor_id
JOIN orbits o ON o.id = m.orbit_id AND o.status = 'active'
WHERE m.orbit_id = ? AND m.actor_id = ? AND m.left_at IS NULL`,
		orbitID, actorID,
	).Scan(&role, &revoked)
	if errors.Is(err, sql.ErrNoRows) || revoked.Valid {
		return ErrTransmissionPolicyForbidden
	}
	if err != nil {
		return err
	}
	if role != "primary" {
		return ErrTransmissionPolicyForbidden
	}
	return nil
}

func (s *Store) SetOrbitDND(params SetOrbitDNDParams) (DNDMutation, error) {
	if params.OrbitID <= 0 || params.AuthorizedByActorID <= 0 || params.Revision <= 0 {
		return DNDMutation{}, ErrTransmissionInvalid
	}
	if err := validateDND(params.Mode, params.MutedUntil, params.UpdatedAt); err != nil {
		return DNDMutation{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return DNDMutation{}, err
	}
	defer tx.Rollback()
	if err := authorizeOrbitDNDMutationTx(
		tx, params.OrbitID, params.AuthorizedByActorID,
	); err != nil {
		return DNDMutation{}, err
	}
	var current DNDSetting
	err = tx.QueryRow(`SELECT orbit_id, mode, muted_until, revision,
       updated_by_actor_id, updated_at
FROM orbit_dnd_settings WHERE orbit_id = ?`, params.OrbitID).Scan(
		&current.OrbitID, &current.Mode, &current.MutedUntil,
		&current.Revision, &current.UpdatedByActorID, &current.UpdatedAt,
	)
	if err == nil {
		if params.Revision < current.Revision ||
			(params.Revision == current.Revision &&
				(params.Mode != current.Mode || params.MutedUntil != current.MutedUntil)) {
			return DNDMutation{}, ErrDNDRevisionConflict
		}
		if params.Revision == current.Revision {
			if err := tx.Commit(); err != nil {
				return DNDMutation{}, err
			}
			return DNDMutation{Setting: current}, nil
		}
		if params.UpdatedAt < current.UpdatedAt {
			return DNDMutation{}, ErrDNDRevisionConflict
		}
		_, err = tx.Exec(`UPDATE orbit_dnd_settings
SET mode = ?, muted_until = ?, revision = ?, updated_by_actor_id = ?, updated_at = ?
WHERE orbit_id = ?`, params.Mode, params.MutedUntil, params.Revision,
			params.AuthorizedByActorID, params.UpdatedAt, params.OrbitID)
	} else if errors.Is(err, sql.ErrNoRows) {
		_, err = tx.Exec(`INSERT INTO orbit_dnd_settings(
  orbit_id, mode, muted_until, revision, updated_by_actor_id, updated_at
) VALUES(?, ?, ?, ?, ?, ?)`, params.OrbitID, params.Mode,
			params.MutedUntil, params.Revision, params.AuthorizedByActorID,
			params.UpdatedAt)
	}
	if err != nil {
		return DNDMutation{}, err
	}
	current = DNDSetting{}
	err = tx.QueryRow(`SELECT orbit_id, mode, muted_until, revision,
       updated_by_actor_id, updated_at
FROM orbit_dnd_settings WHERE orbit_id = ?`, params.OrbitID).Scan(
		&current.OrbitID, &current.Mode, &current.MutedUntil,
		&current.Revision, &current.UpdatedByActorID, &current.UpdatedAt,
	)
	if err != nil {
		return DNDMutation{}, err
	}
	if err := tx.Commit(); err != nil {
		return DNDMutation{}, err
	}
	return DNDMutation{Setting: current, Changed: true}, nil
}

func activeDND(mode DNDMode, mutedUntil, now int64) (DNDMode, int64) {
	if mode == DNDMutedUntil && mutedUntil <= now {
		return DNDAllowAll, 0
	}
	return mode, mutedUntil
}

func dndRank(mode DNDMode) int {
	switch mode {
	case DNDMutedUntil:
		return 2
	case DNDMessagesOnly:
		return 1
	default:
		return 0
	}
}

func (s *Store) EffectiveDND(
	ctx context.Context,
	target MediaTargetIdentity,
	now int64,
) (EffectiveDND, error) {
	if ctx == nil {
		return EffectiveDND{}, errors.New("nil DND lookup context")
	}
	if target.OrbitID <= 0 || target.ActorID <= 0 ||
		!transmissionSlotPattern.MatchString(target.Slot) || now <= 0 {
		return EffectiveDND{}, ErrTransmissionInvalid
	}
	decision := EffectiveDND{Mode: DNDAllowAll}
	var nodeMode DNDMode
	var nodeUntil, nodeRevision int64
	err := s.db.QueryRowContext(ctx, `SELECT d.mode, d.muted_until, d.revision
FROM node_dnd_settings d
JOIN installation_credentials ic ON ic.actor_id = d.actor_id
  AND ic.slot_orbit_id = d.orbit_id AND ic.slot_name = d.slot
  AND ic.slot_paired_at = d.binding_paired_at
JOIN actors a ON a.id = ic.actor_id AND a.revoked_at IS NULL
JOIN slots sl ON sl.orbit_id = ic.slot_orbit_id AND sl.slot = ic.slot_name
  AND sl.revoked_at IS NULL AND sl.token_hash = ic.binding_token_hash
  AND COALESCE(sl.paired_at, 0) = ic.slot_paired_at
WHERE d.orbit_id = ? AND d.actor_id = ? AND d.slot = ?`,
		target.OrbitID, target.ActorID, target.Slot,
	).Scan(&nodeMode, &nodeUntil, &nodeRevision)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return EffectiveDND{}, err
	}
	if err == nil {
		nodeMode, nodeUntil = activeDND(nodeMode, nodeUntil, now)
		decision.NodeRevision = nodeRevision
	}
	var orbitMode DNDMode
	var orbitUntil, orbitRevision int64
	err = s.db.QueryRowContext(ctx, `SELECT mode, muted_until, revision
FROM orbit_dnd_settings WHERE orbit_id = ?`, target.OrbitID).Scan(
		&orbitMode, &orbitUntil, &orbitRevision,
	)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return EffectiveDND{}, err
	}
	if err == nil {
		orbitMode, orbitUntil = activeDND(orbitMode, orbitUntil, now)
		decision.OrbitRevision = orbitRevision
	}
	// Local wins equal severity except that two muted-until layers use the
	// later coordinator deadline, as frozen by the contract.
	decision.Mode, decision.MutedUntil = nodeMode, nodeUntil
	if decision.Mode == "" {
		decision.Mode = DNDAllowAll
	}
	if dndRank(orbitMode) > dndRank(decision.Mode) ||
		(orbitMode == DNDMutedUntil && decision.Mode == DNDMutedUntil &&
			orbitUntil > decision.MutedUntil) {
		decision.Mode, decision.MutedUntil = orbitMode, orbitUntil
		decision.Reason = TransmissionReasonOrbitDND
	} else if decision.Mode != DNDAllowAll {
		decision.Reason = TransmissionReasonLocalDND
	}
	if decision.Mode == DNDAllowAll {
		decision.MutedUntil = 0
		decision.Reason = ""
	}
	return decision, nil
}
