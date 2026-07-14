package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"relux.works/duet/coordinator/internal/ulid"
)

var ErrTransmissionPolicyIdempotency = errors.New("transmission policy idempotency key conflicts")

type PresencePolicyTarget struct {
	OrbitID       int64
	ActorID       int64
	Slot          string
	NodeTokenHash string
}

type PresencePolicyDomain struct {
	Kind          PlaybackDomainKind
	ID            int64
	CallerOrbitID int64
	Targets       []PresencePolicyTarget
}

// AuthorizedPresenceDomain resolves only the caller's own orbit and its one
// current pairwise approach. It deliberately returns no credential, device or
// process metadata.
func (s *Store) AuthorizedPresenceDomain(
	expectedActorID int64, bearer string,
) (PresencePolicyDomain, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return PresencePolicyDomain{}, err
	}
	defer tx.Rollback()
	ctx, err := authorizeTransmissionActorTx(tx, expectedActorID, bearer)
	if err != nil {
		return PresencePolicyDomain{}, err
	}
	kind, id, allowed, err := transmissionDomainTx(tx, ctx.OrbitID)
	if err != nil {
		return PresencePolicyDomain{}, err
	}
	orbits := make([]int64, 0, len(allowed))
	for orbitID := range allowed {
		orbits = append(orbits, orbitID)
	}
	sort.Slice(orbits, func(i, j int) bool { return orbits[i] < orbits[j] })
	result := PresencePolicyDomain{Kind: kind, ID: id, CallerOrbitID: ctx.OrbitID}
	for _, orbitID := range orbits {
		targets, err := liveTransmissionTargetsTx(tx, orbitID, "")
		if err != nil {
			return PresencePolicyDomain{}, err
		}
		for _, target := range targets {
			result.Targets = append(result.Targets, PresencePolicyTarget{
				OrbitID: target.OrbitID, ActorID: target.ActorID, Slot: target.Slot,
				NodeTokenHash: target.NodeTokenHash,
			})
		}
	}
	if err := tx.Commit(); err != nil {
		return PresencePolicyDomain{}, err
	}
	return result, nil
}

type AuthorizedDNDMutationParams struct {
	ExpectedActorID    int64
	Bearer             string
	Identity           Identity
	Layer              string
	Mode               DNDMode
	MutedUntil         int64
	ExpectedRevision   int64
	IdempotencyKeyHash string
	RequestHash        string
	UpdatedAt          int64
}

type DNDLayers struct {
	Local     DNDSetting
	HasLocal  bool
	Orbit     DNDSetting
	HasOrbit  bool
	Effective EffectiveDND
}

func (s *Store) PresenceDNDLayers(target MediaTargetIdentity, now int64) (DNDLayers, error) {
	result := DNDLayers{}
	err := s.db.QueryRow(`SELECT d.orbit_id, d.actor_id, d.slot, d.binding_paired_at,
       d.mode, d.muted_until, d.revision, d.updated_at
FROM node_dnd_settings d
JOIN installation_credentials ic ON ic.actor_id = d.actor_id
  AND ic.slot_orbit_id = d.orbit_id AND ic.slot_name = d.slot
  AND ic.slot_paired_at = d.binding_paired_at
JOIN slots sl ON sl.orbit_id = ic.slot_orbit_id AND sl.slot = ic.slot_name
  AND sl.revoked_at IS NULL AND sl.token_hash = ic.binding_token_hash
  AND COALESCE(sl.paired_at, 0) = ic.slot_paired_at
WHERE d.orbit_id = ? AND d.actor_id = ? AND d.slot = ?`,
		target.OrbitID, target.ActorID, target.Slot).Scan(&result.Local.OrbitID,
		&result.Local.ActorID, &result.Local.Slot, &result.Local.BindingPairedAt,
		&result.Local.Mode, &result.Local.MutedUntil, &result.Local.Revision,
		&result.Local.UpdatedAt)
	if err == nil {
		result.HasLocal = true
		result.Local.Mode, result.Local.MutedUntil = activeDND(result.Local.Mode, result.Local.MutedUntil, now)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return DNDLayers{}, err
	}
	err = s.db.QueryRow(`SELECT orbit_id, mode, muted_until, revision,
       updated_by_actor_id, updated_at FROM orbit_dnd_settings WHERE orbit_id = ?`,
		target.OrbitID).Scan(&result.Orbit.OrbitID, &result.Orbit.Mode,
		&result.Orbit.MutedUntil, &result.Orbit.Revision,
		&result.Orbit.UpdatedByActorID, &result.Orbit.UpdatedAt)
	if err == nil {
		result.HasOrbit = true
		result.Orbit.Mode, result.Orbit.MutedUntil = activeDND(result.Orbit.Mode, result.Orbit.MutedUntil, now)
	} else if !errors.Is(err, sql.ErrNoRows) {
		return DNDLayers{}, err
	}
	result.Effective, err = s.EffectiveDND(context.Background(), target, now)
	return result, err
}

func loadPolicyReplay(db rowQuerier, actorID int64, operation, keyHash, requestHash string, target any) (bool, error) {
	var existingHash, response string
	err := db.QueryRow(`SELECT request_hash, response_json
FROM transmission_policy_requests
WHERE actor_id = ? AND operation = ? AND idempotency_key_hash = ?`,
		actorID, operation, keyHash).Scan(&existingHash, &response)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if existingHash != requestHash {
		return false, ErrTransmissionPolicyIdempotency
	}
	if err := json.Unmarshal([]byte(response), target); err != nil {
		return false, err
	}
	return true, nil
}

type policySQL interface {
	rowQuerier
	Exec(query string, args ...any) (sql.Result, error)
}

func recordPolicyResponse(db policySQL, actorID int64, operation, keyHash, requestHash, resourceID string, revision, now int64, response any) error {
	raw, err := json.Marshal(response)
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO transmission_policy_requests(
  actor_id, operation, idempotency_key_hash, request_hash,
  resource_id, resource_revision, response_json, created_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, actorID, operation, keyHash,
		requestHash, resourceID, revision, string(raw), now)
	if err == nil {
		return nil
	}
	var replay any
	if ok, replayErr := loadPolicyReplay(db, actorID, operation, keyHash, requestHash, &replay); ok {
		return nil
	} else if replayErr != nil {
		return replayErr
	}
	return err
}

func (s *Store) AuthorizedSetDND(params AuthorizedDNDMutationParams) (DNDMutation, error) {
	if params.Identity.Kind == "" {
		params.Identity = Identity{Kind: IdentityBearer, Token: params.Bearer}
	}
	if params.ExpectedActorID <= 0 || params.ExpectedRevision < 0 ||
		len(params.IdempotencyKeyHash) != 64 || len(params.RequestHash) != 64 {
		return DNDMutation{}, ErrTransmissionInvalid
	}
	operation := "dnd_" + params.Layer
	if operation != "dnd_local" && operation != "dnd_orbit" {
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
	ctx, err := resolveActorContext(tx, params.Identity)
	allowedTransport := ctx.Capabilities.Has(CapabilityControl)
	if params.Layer == "orbit" {
		allowedTransport = allowedTransport || ctx.Capabilities.Has(CapabilityTelegram)
	}
	if err != nil || ctx.ActorID != params.ExpectedActorID || !allowedTransport {
		if err == nil {
			err = ErrTransmissionPolicyForbidden
		}
		return DNDMutation{}, err
	}
	var replay DNDMutation
	if ok, err := loadPolicyReplay(tx, params.ExpectedActorID, operation,
		params.IdempotencyKeyHash, params.RequestHash, &replay); ok || err != nil {
		return replay, err
	}
	var currentRevision int64
	var revisionErr error
	if params.Layer == "local" {
		revisionErr = tx.QueryRow(`SELECT revision FROM node_dnd_settings
WHERE orbit_id = ? AND actor_id = ? AND slot = ?`, ctx.OrbitID, ctx.ActorID,
			ctx.Slot).Scan(&currentRevision)
	} else {
		revisionErr = tx.QueryRow(`SELECT revision FROM orbit_dnd_settings
WHERE orbit_id = ?`, ctx.OrbitID).Scan(&currentRevision)
	}
	if errors.Is(revisionErr, sql.ErrNoRows) {
		if params.ExpectedRevision != 0 {
			return DNDMutation{}, ErrDNDRevisionConflict
		}
	} else if revisionErr != nil {
		return DNDMutation{}, revisionErr
	} else if params.ExpectedRevision != currentRevision {
		return DNDMutation{}, ErrDNDRevisionConflict
	}
	nextRevision := params.ExpectedRevision + 1
	if params.Layer == "local" {
		replay, err = setNodeDNDTx(tx, SetNodeDNDParams{
			OrbitID: ctx.OrbitID, ActorID: ctx.ActorID, Slot: ctx.Slot,
			Mode: params.Mode, MutedUntil: params.MutedUntil,
			Revision: nextRevision, UpdatedAt: params.UpdatedAt,
		})
	} else {
		replay, err = setOrbitDNDTx(tx, SetOrbitDNDParams{
			OrbitID: ctx.OrbitID, AuthorizedByActorID: ctx.ActorID,
			Mode: params.Mode, MutedUntil: params.MutedUntil,
			Revision: nextRevision, UpdatedAt: params.UpdatedAt,
		})
	}
	if err != nil {
		return DNDMutation{}, err
	}
	if err := recordPolicyResponse(tx, params.ExpectedActorID, operation,
		params.IdempotencyKeyHash, params.RequestHash, "", replay.Setting.Revision,
		params.UpdatedAt, replay); err != nil {
		return DNDMutation{}, err
	}
	if err := tx.Commit(); err != nil {
		return DNDMutation{}, err
	}
	return replay, nil
}

type TransmissionSubjectReference struct {
	PublicID    string
	SubjectKind BlockedSubjectKind
	SubjectID   int64
	DisplayName string
	ExpiresAt   int64
}

// MintTransmissionSubjectReference is the server-side seam used by history.
// It is intentionally not an HTTP route, so numeric identity never becomes a
// client-selected blocking primitive.
func (s *Store) MintTransmissionSubjectReference(expectedActorID int64, bearer string, kind BlockedSubjectKind, subjectID, now int64) (TransmissionSubjectReference, error) {
	return s.MintTransmissionSubjectReferenceForIdentity(expectedActorID,
		Identity{Kind: IdentityBearer, Token: bearer}, kind, subjectID, now)
}

func (s *Store) MintTransmissionSubjectReferenceForIdentity(expectedActorID int64, identity Identity, kind BlockedSubjectKind, subjectID, now int64) (TransmissionSubjectReference, error) {
	if expectedActorID <= 0 || subjectID <= 0 || now <= 0 ||
		(kind != BlockedSubjectActor && kind != BlockedSubjectOrbit) {
		return TransmissionSubjectReference{}, ErrTransmissionInvalid
	}
	ctx, err := s.ResolveActorContext(identity)
	if err != nil || ctx.ActorID != expectedActorID {
		if err == nil {
			err = ErrUnauthorized
		}
		return TransmissionSubjectReference{}, err
	}
	var displayName string
	query := `SELECT display_name FROM actors WHERE id = ?`
	if kind == BlockedSubjectOrbit {
		query = `SELECT title FROM orbits WHERE id = ?`
	}
	if err := s.db.QueryRow(query, subjectID).Scan(&displayName); errors.Is(err, sql.ErrNoRows) {
		return TransmissionSubjectReference{}, ErrTransmissionNotFound
	} else if err != nil {
		return TransmissionSubjectReference{}, err
	}
	var existing TransmissionSubjectReference
	existing.SubjectKind = kind
	existing.SubjectID = subjectID
	err = s.db.QueryRow(`SELECT public_id, display_name, expires_at
FROM transmission_subject_refs
WHERE viewer_actor_id = ? AND subject_kind = ? AND subject_id = ?
  AND display_name = ? AND expires_at > ?
ORDER BY expires_at DESC, public_id DESC LIMIT 1`, expectedActorID, kind,
		subjectID, displayName, now+int64((5*time.Minute)/time.Millisecond)).Scan(
		&existing.PublicID, &existing.DisplayName, &existing.ExpiresAt)
	if err == nil {
		return existing, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return TransmissionSubjectReference{}, err
	}
	prefix := "ar_"
	if kind == BlockedSubjectOrbit {
		prefix = "or_"
	}
	ref := TransmissionSubjectReference{
		PublicID: prefix + ulid.New(time.UnixMilli(now)), SubjectKind: kind,
		SubjectID: subjectID, DisplayName: displayName,
		ExpiresAt: now + int64((24*time.Hour)/time.Millisecond),
	}
	_, err = s.db.Exec(`INSERT INTO transmission_subject_refs(
  public_id, viewer_actor_id, subject_kind, subject_id, display_name, created_at, expires_at
) VALUES(?, ?, ?, ?, ?, ?, ?)`, ref.PublicID, expectedActorID, kind, subjectID,
		ref.DisplayName, now, ref.ExpiresAt)
	return ref, err
}

type PublicTransmissionBlock struct {
	ID          string             `json:"id"`
	OwnerScope  BlockOwnerScope    `json:"owner_scope"`
	SubjectKind BlockedSubjectKind `json:"subject_kind"`
	SubjectRef  string             `json:"subject_ref"`
	DisplayName string             `json:"display_name"`
	CreatedAt   int64              `json:"created_at"`
	Revision    int64              `json:"revision"`
	Revoked     bool               `json:"revoked"`
	Internal    TransmissionBlock  `json:"-"`
	Reused      bool               `json:"reused"`
}

const qualifiedTransmissionBlockColumns = `b.id, b.owner_scope, b.owner_orbit_id,
b.owner_actor_id, b.blocked_kind, b.blocked_actor_id, b.blocked_orbit_id,
b.created_by_actor_id, b.created_at, b.revoked_at, b.revision`

type AuthorizedCreateBlockParams struct {
	ExpectedActorID    int64
	Bearer             string
	Identity           Identity
	OwnerScope         BlockOwnerScope
	SubjectRef         string
	IdempotencyKeyHash string
	RequestHash        string
	CreatedAt          int64
}

func publicBlockFrom(block TransmissionBlock, publicID, subjectRef, displayName string) PublicTransmissionBlock {
	return PublicTransmissionBlock{ID: publicID, OwnerScope: block.OwnerScope,
		SubjectKind: block.BlockedKind, SubjectRef: subjectRef, DisplayName: displayName,
		CreatedAt: block.CreatedAt, Revision: block.Revision,
		Revoked: block.RevokedAt != 0, Internal: block}
}

func (s *Store) AuthorizedCreateTransmissionBlock(params AuthorizedCreateBlockParams) (PublicTransmissionBlock, error) {
	if params.ExpectedActorID <= 0 || params.CreatedAt <= 0 ||
		(params.OwnerScope != BlockOwnerActor && params.OwnerScope != BlockOwnerOrbit) ||
		len(params.SubjectRef) != 29 || len(params.IdempotencyKeyHash) != 64 ||
		len(params.RequestHash) != 64 {
		return PublicTransmissionBlock{}, ErrTransmissionInvalid
	}
	if (params.OwnerScope == BlockOwnerActor && params.SubjectRef[:3] != "ar_") ||
		(params.OwnerScope == BlockOwnerOrbit && params.SubjectRef[:3] != "or_") {
		return PublicTransmissionBlock{}, ErrTransmissionInvalid
	}
	if params.Identity.Kind == "" {
		params.Identity = Identity{Kind: IdentityBearer, Token: params.Bearer}
	}
	tx, err := s.db.Begin()
	if err != nil {
		return PublicTransmissionBlock{}, err
	}
	defer tx.Rollback()
	ctx, err := resolveActorContext(tx, params.Identity)
	if err != nil || ctx.ActorID != params.ExpectedActorID ||
		(!ctx.Capabilities.Has(CapabilityControl) && !ctx.Capabilities.Has(CapabilityTelegram)) {
		if err == nil {
			err = ErrTransmissionPolicyForbidden
		}
		return PublicTransmissionBlock{}, err
	}
	var replay PublicTransmissionBlock
	if ok, err := loadPolicyReplay(tx, params.ExpectedActorID, "block_create",
		params.IdempotencyKeyHash, params.RequestHash, &replay); ok || err != nil {
		if ok {
			replay.Reused = true
		}
		return replay, err
	}
	var kind BlockedSubjectKind
	var subjectID, expiresAt int64
	var displayName string
	err = tx.QueryRow(`SELECT subject_kind, subject_id, display_name, expires_at
FROM transmission_subject_refs WHERE public_id = ? AND viewer_actor_id = ?`,
		params.SubjectRef, ctx.ActorID).Scan(&kind, &subjectID, &displayName, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) || expiresAt <= params.CreatedAt {
		return PublicTransmissionBlock{}, ErrTransmissionNotFound
	}
	if err != nil {
		return PublicTransmissionBlock{}, err
	}
	create := CreateTransmissionBlockParams{OwnerScope: params.OwnerScope,
		OwnerOrbitID: ctx.OrbitID, AuthorizedByActorID: ctx.ActorID,
		BlockedKind: kind, CreatedAt: params.CreatedAt}
	if params.OwnerScope == BlockOwnerActor {
		create.OwnerActorID = ctx.ActorID
	}
	if kind == BlockedSubjectActor {
		create.BlockedActorID = subjectID
	} else {
		create.BlockedOrbitID = subjectID
	}
	if err := validateCreateTransmissionBlock(create); err != nil {
		return PublicTransmissionBlock{}, err
	}
	created, err := createTransmissionBlockTx(tx, create)
	if err != nil {
		return PublicTransmissionBlock{}, err
	}
	publicID := ""
	err = tx.QueryRow(`SELECT public_id FROM transmission_block_public_ids WHERE block_id = ?`, created.Block.ID).Scan(&publicID)
	if errors.Is(err, sql.ErrNoRows) {
		publicID = "bl_" + ulid.New(time.UnixMilli(params.CreatedAt))
		_, err = tx.Exec(`INSERT INTO transmission_block_public_ids(block_id, public_id, subject_ref) VALUES(?, ?, ?)`, created.Block.ID, publicID, params.SubjectRef)
	}
	if err != nil {
		return PublicTransmissionBlock{}, err
	}
	replay = publicBlockFrom(created.Block, publicID, params.SubjectRef, displayName)
	replay.Reused = created.Reused
	if err := recordPolicyResponse(tx, ctx.ActorID, "block_create", params.IdempotencyKeyHash,
		params.RequestHash, publicID, replay.Revision, params.CreatedAt, replay); err != nil {
		return PublicTransmissionBlock{}, err
	}
	if err := tx.Commit(); err != nil {
		return PublicTransmissionBlock{}, err
	}
	return replay, nil
}

func (s *Store) AuthorizedListTransmissionBlocks(expectedActorID int64, bearer string) ([]PublicTransmissionBlock, error) {
	return s.AuthorizedListTransmissionBlocksForIdentity(expectedActorID,
		Identity{Kind: IdentityBearer, Token: bearer})
}

func (s *Store) AuthorizedListTransmissionBlocksForIdentity(expectedActorID int64, identity Identity) ([]PublicTransmissionBlock, error) {
	if expectedActorID <= 0 {
		return nil, ErrUnauthorized
	}
	ctx, err := s.ResolveActorContext(identity)
	if err != nil || ctx.ActorID != expectedActorID {
		if err == nil {
			err = ErrUnauthorized
		}
		return nil, err
	}
	rows, err := s.db.Query(`SELECT `+qualifiedTransmissionBlockColumns+`, p.public_id, p.subject_ref, r.display_name
FROM blocks b JOIN transmission_block_public_ids p ON p.block_id = b.id
JOIN transmission_subject_refs r ON r.public_id = p.subject_ref
WHERE b.revoked_at = 0 AND b.owner_orbit_id = ?
  AND ((b.owner_scope = 'actor' AND b.owner_actor_id = ?)
    OR (b.owner_scope = 'orbit' AND ? = 'primary'))
ORDER BY b.created_at DESC, p.public_id DESC`, ctx.OrbitID, ctx.ActorID, ctx.Role)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []PublicTransmissionBlock
	for rows.Next() {
		var block TransmissionBlock
		var publicID, subjectRef, displayName string
		if err := rows.Scan(&block.ID, &block.OwnerScope, &block.OwnerOrbitID,
			&block.OwnerActorID, &block.BlockedKind, &block.BlockedActorID,
			&block.BlockedOrbitID, &block.CreatedByActorID, &block.CreatedAt,
			&block.RevokedAt, &block.Revision, &publicID, &subjectRef, &displayName); err != nil {
			return nil, err
		}
		result = append(result, publicBlockFrom(block, publicID, subjectRef, displayName))
	}
	return result, rows.Err()
}

func (s *Store) AuthorizedDeleteTransmissionBlock(expectedActorID int64, bearer, publicID string, now int64) (PublicTransmissionBlock, bool, error) {
	return s.AuthorizedDeleteTransmissionBlockForIdentity(expectedActorID,
		Identity{Kind: IdentityBearer, Token: bearer}, publicID, now)
}

func (s *Store) AuthorizedDeleteTransmissionBlockForIdentity(expectedActorID int64, identity Identity, publicID string, now int64) (PublicTransmissionBlock, bool, error) {
	if expectedActorID <= 0 || now <= 0 || len(publicID) != 29 || publicID[:3] != "bl_" {
		return PublicTransmissionBlock{}, false, ErrTransmissionNotFound
	}
	tx, err := s.db.Begin()
	if err != nil {
		return PublicTransmissionBlock{}, false, err
	}
	defer tx.Rollback()
	ctx, err := resolveActorContext(tx, identity)
	if err != nil || ctx.ActorID != expectedActorID {
		if err == nil {
			err = ErrUnauthorized
		}
		return PublicTransmissionBlock{}, false, err
	}
	block, err := scanTransmissionBlock(tx.QueryRow(`SELECT `+transmissionBlockColumns+`
FROM blocks b JOIN transmission_block_public_ids p ON p.block_id = b.id
WHERE p.public_id = ? AND b.owner_orbit_id = ?
  AND ((b.owner_scope = 'actor' AND b.owner_actor_id = ?)
    OR (b.owner_scope = 'orbit' AND ? = 'primary'))`, publicID, ctx.OrbitID, ctx.ActorID, ctx.Role))
	if errors.Is(err, sql.ErrNoRows) {
		return PublicTransmissionBlock{}, false, ErrTransmissionNotFound
	}
	if err != nil {
		return PublicTransmissionBlock{}, false, err
	}
	changed := false
	if block.RevokedAt == 0 {
		if err := authorizeBlockOwnerTx(tx, block.OwnerScope, block.OwnerOrbitID,
			block.OwnerActorID, ctx.ActorID); err != nil {
			return PublicTransmissionBlock{}, false, ErrTransmissionNotFound
		}
		result, updateErr := tx.Exec(`UPDATE blocks SET revoked_at = ?, revision = revision + 1
WHERE id = ? AND revoked_at = 0 AND revision = ?`, now, block.ID, block.Revision)
		if updateErr != nil {
			return PublicTransmissionBlock{}, false, updateErr
		}
		if count, countErr := result.RowsAffected(); countErr != nil || count != 1 {
			if countErr != nil {
				return PublicTransmissionBlock{}, false, countErr
			}
			return PublicTransmissionBlock{}, false, ErrTransmissionStateConflict
		}
		block, err = scanTransmissionBlock(tx.QueryRow(`SELECT `+transmissionBlockColumns+`
FROM blocks WHERE id = ?`, block.ID))
		if err != nil {
			return PublicTransmissionBlock{}, false, err
		}
		changed = true
	}
	if err := tx.Commit(); err != nil {
		return PublicTransmissionBlock{}, false, err
	}
	return publicBlockFrom(block, publicID, "", ""), changed, nil
}
