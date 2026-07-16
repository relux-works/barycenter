package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

const transmissionTargetReferenceTTL = 24 * time.Hour

var transmissionTargetReferencePattern = regexp.MustCompile(`^trf_[A-Za-z0-9_-]{43}$`)

// IssueTransmissionTargetReferenceParams identifies a trusted target selected
// by an adapter. The returned capability contains none of these identities and
// is accepted only for the same current ActorContext and credential scope.
type IssueTransmissionTargetReferenceParams struct {
	ExpectedActorID int64
	Bearer          string
	Identity        Identity
	Kind            TransmissionAudienceSelectorKind
	OrbitID         int64
	Slot            string
	IssuedAt        int64
}

type IssuePersonalTransmissionTargetsParams struct {
	ExpectedActorID int64
	Identity        Identity
	SourceOrbitID   int64
	IssuedAt        int64
}

// TransmissionTargetReferenceOption is safe to present to an authorized
// target picker. Label is presentation-only; Reference is the sole value a
// create request may return to the coordinator.
type TransmissionTargetReferenceOption struct {
	Reference   string
	Kind        TransmissionAudienceSelectorKind
	Label       string
	OrbitID     int64
	OrbitTitle  string
	Slot        string
	TargetSlots []string
	ExpiresAt   int64
}

type storedTransmissionTargetReference struct {
	Kind            TransmissionAudienceSelectorKind
	OrbitID         int64
	ActorID         int64
	Slot            string
	BindingPairedAt int64
}

func transmissionProofIdentity(bearer string, identity Identity) (Identity, bool) {
	if bearer != "" && identity == (Identity{}) {
		return Identity{Kind: IdentityBearer, Token: bearer}, true
	}
	if bearer == "" && identity.Kind == IdentityTelegram && identity.TelegramUserID > 0 {
		return identity, true
	}
	return Identity{}, false
}

func authorizeTransmissionProofTx(
	tx *sql.Tx,
	expectedActorID int64,
	bearer string,
	identity Identity,
) (ActorContext, Identity, error) {
	proof, valid := transmissionProofIdentity(bearer, identity)
	if expectedActorID <= 0 || !valid {
		return ActorContext{}, Identity{}, ErrUnauthorized
	}
	ctx, err := resolveActorContext(tx, proof)
	if errors.Is(err, ErrUnauthorized) || ctx.ActorID != expectedActorID {
		return ActorContext{}, Identity{}, ErrUnauthorized
	}
	if err != nil && !errors.Is(err, ErrInsufficientCapability) {
		return ActorContext{}, Identity{}, err
	}
	if (!ctx.Capabilities.Has(CapabilityControl) &&
		!ctx.Capabilities.Has(CapabilityTelegram)) ||
		(ctx.Role == "satellite" && !ctx.Capabilities.Has(CapabilityTelegram)) ||
		ctx.OrbitID <= 0 || ctx.ActorID <= 0 {
		return ActorContext{}, Identity{}, ErrInsufficientCapability
	}
	return ctx, proof, nil
}

func newTransmissionTargetReference() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	reference := "trf_" + base64.RawURLEncoding.EncodeToString(raw)
	if !transmissionTargetReferencePattern.MatchString(reference) {
		return "", errors.New("invalid generated transmission target reference")
	}
	return reference, nil
}

func targetReferenceDomainTx(
	tx *sql.Tx,
	ctx ActorContext,
) (map[int64]struct{}, error) {
	policyContext, err := activeAirPolicyContextTx(tx, ctx.OrbitID)
	if err != nil {
		return nil, err
	}
	_, _, allowed, err := transmissionDomainTx(tx, ctx.OrbitID, policyContext)
	return allowed, err
}

func validateTargetReferenceSubjectTx(
	tx *sql.Tx,
	allowed map[int64]struct{},
	kind TransmissionAudienceSelectorKind,
	orbitID int64,
	slot string,
) (storedTransmissionTargetReference, error) {
	if _, ok := allowed[orbitID]; !ok {
		return storedTransmissionTargetReference{}, ErrTransmissionAudienceNotFound
	}
	targets, err := liveTransmissionTargetsTx(tx, orbitID, slot)
	if err != nil {
		return storedTransmissionTargetReference{}, err
	}
	switch kind {
	case TransmissionSelectorBarycenter:
		if slot != "" || len(targets) == 0 {
			return storedTransmissionTargetReference{}, ErrTransmissionAudienceNotFound
		}
		return storedTransmissionTargetReference{Kind: kind, OrbitID: orbitID}, nil
	case TransmissionSelectorPulsar:
		if !transmissionSlotPattern.MatchString(slot) || len(targets) != 1 {
			return storedTransmissionTargetReference{}, ErrTransmissionAudienceNotFound
		}
		target := targets[0]
		return storedTransmissionTargetReference{
			Kind: kind, OrbitID: orbitID, ActorID: target.ActorID, Slot: target.Slot,
			BindingPairedAt: target.BindingPairedAt,
		}, nil
	default:
		return storedTransmissionTargetReference{}, ErrTransmissionAudienceNotFound
	}
}

func mintTransmissionTargetReferenceTx(
	tx *sql.Tx,
	ctx ActorContext,
	proof Identity,
	subject storedTransmissionTargetReference,
	now int64,
) (string, error) {
	reference, err := newTransmissionTargetReference()
	if err != nil {
		return "", err
	}
	expiresAt := now + transmissionTargetReferenceTTL.Milliseconds()
	_, err = tx.Exec(`INSERT INTO transmission_target_references(
  reference_hash, actor_id, authorization_hash, target_kind, target_orbit_id,
  target_actor_id, target_slot, target_binding_paired_at, created_at, expires_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, hashToken(reference), ctx.ActorID,
		historyAuthorizationHash(ctx, proof), subject.Kind, subject.OrbitID,
		subject.ActorID, subject.Slot, subject.BindingPairedAt, now, expiresAt)
	if err != nil {
		return "", err
	}
	return reference, nil
}

// IssueTransmissionTargetReference lets verified non-HTTP adapters enter the
// same opaque-selector service as application clients. Authority and target
// generation are checked before the capability is minted.
func (s *Store) IssueTransmissionTargetReference(
	params IssueTransmissionTargetReferenceParams,
) (string, error) {
	if params.IssuedAt <= 0 || params.OrbitID <= 0 {
		return "", ErrTransmissionInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	ctx, proof, err := authorizeTransmissionProofTx(
		tx, params.ExpectedActorID, params.Bearer, params.Identity,
	)
	if err != nil {
		return "", err
	}
	allowed, err := targetReferenceDomainTx(tx, ctx)
	if err != nil {
		return "", err
	}
	subject, err := validateTargetReferenceSubjectTx(
		tx, allowed, params.Kind, params.OrbitID, params.Slot,
	)
	if err != nil {
		return "", err
	}
	reference, err := mintTransmissionTargetReferenceTx(tx, ctx, proof, subject, params.IssuedAt)
	if err != nil {
		return "", err
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return reference, nil
}

// IssuePersonalTransmissionTargets owns the transport-neutral "everyone in
// my current domain except my originating installation" rule. Telegram and
// future adapters provide identity proof only; they do not inspect Air
// membership or construct node selectors themselves.
func (s *Store) IssuePersonalTransmissionTargets(
	params IssuePersonalTransmissionTargetsParams,
) ([]TransmissionAudienceSelector, error) {
	if params.SourceOrbitID <= 0 || params.IssuedAt <= 0 ||
		params.Identity.Kind != IdentityTelegram || params.Identity.TelegramUserID <= 0 {
		return nil, ErrTransmissionInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	ctx, proof, err := authorizeTransmissionProofTx(
		tx, params.ExpectedActorID, "", params.Identity,
	)
	if err != nil {
		return nil, err
	}
	if ctx.OrbitID != params.SourceOrbitID {
		return nil, ErrUnauthorized
	}
	allowed, err := targetReferenceDomainTx(tx, ctx)
	if err != nil {
		return nil, err
	}
	originSlot := ""
	err = tx.QueryRow(`SELECT slot FROM slots
WHERE orbit_id = ? AND paired_by = ? AND revoked_at IS NULL`,
		params.SourceOrbitID, params.Identity.TelegramUserID).Scan(&originSlot)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	orbits := make([]int64, 0, len(allowed))
	for orbitID := range allowed {
		orbits = append(orbits, orbitID)
	}
	sort.Slice(orbits, func(i, j int) bool { return orbits[i] < orbits[j] })
	var selectors []TransmissionAudienceSelector
	for _, orbitID := range orbits {
		targets, err := liveTransmissionTargetsTx(tx, orbitID, "")
		if err != nil {
			return nil, err
		}
		for _, target := range targets {
			if originSlot != "" && target.OrbitID == params.SourceOrbitID &&
				target.Slot == originSlot {
				continue
			}
			reference, err := mintTransmissionTargetReferenceTx(tx, ctx, proof,
				storedTransmissionTargetReference{
					Kind: TransmissionSelectorPulsar, OrbitID: target.OrbitID,
					ActorID: target.ActorID, Slot: target.Slot,
					BindingPairedAt: target.BindingPairedAt,
				}, params.IssuedAt)
			if err != nil {
				return nil, err
			}
			selectors = append(selectors, TransmissionAudienceSelector{Reference: reference})
		}
	}
	if len(selectors) == 0 {
		return nil, ErrTransmissionAudienceEmpty
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return selectors, nil
}

// ListTransmissionTargetReferences returns only targets inside the caller's
// currently authorized domain. Numeric identities remain server-side.
func (s *Store) ListTransmissionTargetReferences(
	expectedActorID int64,
	bearer string,
	now int64,
) ([]TransmissionTargetReferenceOption, error) {
	if now <= 0 {
		return nil, ErrTransmissionInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	ctx, proof, err := authorizeTransmissionProofTx(tx, expectedActorID, bearer, Identity{})
	if err != nil {
		return nil, err
	}
	allowed, err := targetReferenceDomainTx(tx, ctx)
	if err != nil {
		return nil, err
	}
	type orbitOption struct {
		id    int64
		title string
	}
	orbits := make([]orbitOption, 0, len(allowed))
	for orbitID := range allowed {
		var title string
		if err := tx.QueryRow(`SELECT title FROM orbits WHERE id = ? AND status = 'active'`, orbitID).
			Scan(&title); err != nil {
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			return nil, err
		}
		orbits = append(orbits, orbitOption{id: orbitID, title: title})
	}
	sort.Slice(orbits, func(i, j int) bool {
		if orbits[i].title == orbits[j].title {
			return orbits[i].id < orbits[j].id
		}
		return orbits[i].title < orbits[j].title
	})
	var options []TransmissionTargetReferenceOption
	for _, orbit := range orbits {
		targets, err := liveTransmissionTargetsTx(tx, orbit.id, "")
		if err != nil {
			return nil, err
		}
		if len(targets) == 0 {
			continue
		}
		barycenterRef, err := mintTransmissionTargetReferenceTx(tx, ctx, proof,
			storedTransmissionTargetReference{Kind: TransmissionSelectorBarycenter, OrbitID: orbit.id}, now)
		if err != nil {
			return nil, err
		}
		options = append(options, TransmissionTargetReferenceOption{
			Reference: barycenterRef, Kind: TransmissionSelectorBarycenter,
			Label: "Barycenter: " + orbit.title, OrbitID: orbit.id,
			OrbitTitle: orbit.title, ExpiresAt: now + transmissionTargetReferenceTTL.Milliseconds(),
		})
		for _, target := range targets {
			options[len(options)-1].TargetSlots = append(options[len(options)-1].TargetSlots, target.Slot)
		}
		for _, target := range targets {
			pulsarRef, err := mintTransmissionTargetReferenceTx(tx, ctx, proof,
				storedTransmissionTargetReference{
					Kind: TransmissionSelectorPulsar, OrbitID: target.OrbitID,
					ActorID: target.ActorID, Slot: target.Slot,
					BindingPairedAt: target.BindingPairedAt,
				}, now)
			if err != nil {
				return nil, err
			}
			options = append(options, TransmissionTargetReferenceOption{
				Reference: pulsarRef, Kind: TransmissionSelectorPulsar,
				Label:   fmt.Sprintf("%s · Pulsar %s", orbit.title, strings.ToUpper(target.Slot)),
				OrbitID: orbit.id, OrbitTitle: orbit.title, Slot: target.Slot,
				TargetSlots: []string{target.Slot},
				ExpiresAt:   now + transmissionTargetReferenceTTL.Milliseconds(),
			})
		}
	}
	if _, err := tx.Exec(`DELETE FROM transmission_target_references WHERE expires_at <= ?`, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return options, nil
}

func resolveTransmissionTargetReferenceTx(
	tx *sql.Tx,
	ctx ActorContext,
	proof Identity,
	reference string,
	now int64,
	allowed map[int64]struct{},
) (TransmissionAudienceSelector, error) {
	if !transmissionTargetReferencePattern.MatchString(reference) {
		return TransmissionAudienceSelector{}, ErrTransmissionAudienceNotFound
	}
	var stored storedTransmissionTargetReference
	var actorID, expiresAt int64
	var authorizationHash string
	err := tx.QueryRow(`SELECT actor_id, authorization_hash, target_kind,
       target_orbit_id, target_actor_id, target_slot, target_binding_paired_at,
       expires_at
FROM transmission_target_references WHERE reference_hash = ?`, hashToken(reference)).Scan(
		&actorID, &authorizationHash, &stored.Kind, &stored.OrbitID, &stored.ActorID,
		&stored.Slot, &stored.BindingPairedAt, &expiresAt,
	)
	if err != nil || actorID != ctx.ActorID || expiresAt <= now ||
		authorizationHash != historyAuthorizationHash(ctx, proof) {
		return TransmissionAudienceSelector{}, ErrTransmissionAudienceNotFound
	}
	validated, err := validateTargetReferenceSubjectTx(
		tx, allowed, stored.Kind, stored.OrbitID, stored.Slot,
	)
	if err != nil || (stored.Kind == TransmissionSelectorPulsar &&
		(validated.ActorID != stored.ActorID ||
			validated.BindingPairedAt != stored.BindingPairedAt)) {
		return TransmissionAudienceSelector{}, ErrTransmissionAudienceNotFound
	}
	return TransmissionAudienceSelector{
		Reference: reference, Kind: stored.Kind, OrbitID: stored.OrbitID, Slot: stored.Slot,
	}, nil
}
