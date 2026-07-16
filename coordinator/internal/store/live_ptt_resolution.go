package store

import (
	"database/sql"
	"errors"
)

type LivePTTAvailability struct {
	OrbitID             int64
	Slot                string
	Connected           bool
	LastSeenAt          int64
	CredentialTokenHash string
	SupportsLivePTT     bool
}

type LivePTTResolvedTarget struct {
	OrbitID int64
	ActorID int64
	Slot    string
	Reason  string
}

type LivePTTResolution struct {
	SourceActorID  int64
	DomainKind     string
	DomainID       int64
	Targets        []LivePTTResolvedTarget
	Excluded       []LivePTTResolvedTarget
	PolicyRevision int64
}

func (s *Store) HasActiveTransmissionRuntime(domainKind PlaybackDomainKind, domainID int64) (bool, error) {
	if (domainKind != PlaybackDomainOrbit && domainKind != PlaybackDomainApproach) || domainID <= 0 {
		return false, ErrTransmissionInvalid
	}
	var active bool
	err := s.db.QueryRow(`SELECT EXISTS(
  SELECT 1 FROM transmissions
  WHERE playback_domain_kind = ? AND playback_domain_id = ?
    AND effective_delivery IN ('overlay', 'interrupt') AND completed_at = 0
)`, domainKind, domainID).Scan(&active)
	return active, err
}

// ResolveLivePTTTargets atomically proves the current sender socket, Air
// overlay authority, membership, block/DND state and current target binding.
// It performs no write and creates no media or transmission rows.
func (s *Store) ResolveLivePTTTargets(
	orbitID int64,
	slot string,
	credentialTokenHash string,
	availability []LivePTTAvailability,
	nowMS int64,
) (LivePTTResolution, error) {
	if orbitID <= 0 || !transmissionSlotPattern.MatchString(slot) ||
		!transmissionDigestPattern.MatchString(credentialTokenHash) || nowMS <= 0 {
		return LivePTTResolution{}, ErrTransmissionTargetInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return LivePTTResolution{}, err
	}
	defer tx.Rollback()
	var ctx ActorContext
	err = tx.QueryRow(`SELECT ic.slot_orbit_id, ic.actor_id, ic.slot_name, m.role
FROM installation_credentials ic
JOIN actors a ON a.id = ic.actor_id AND a.revoked_at IS NULL
JOIN memberships m ON m.actor_id = ic.actor_id
  AND m.orbit_id = ic.slot_orbit_id AND m.left_at IS NULL
JOIN orbits o ON o.id = m.orbit_id AND o.status = 'active'
JOIN slots sl ON sl.orbit_id = ic.slot_orbit_id AND sl.slot = ic.slot_name
  AND sl.revoked_at IS NULL AND sl.token_hash = ic.binding_token_hash
  AND COALESCE(sl.paired_at, 0) = ic.slot_paired_at
WHERE ic.slot_orbit_id = ? AND ic.slot_name = ? AND ic.binding_token_hash = ?`,
		orbitID, slot, credentialTokenHash).Scan(&ctx.OrbitID, &ctx.ActorID, &ctx.Slot, &ctx.Role)
	if errors.Is(err, sql.ErrNoRows) {
		return LivePTTResolution{}, ErrUnauthorized
	}
	if err != nil {
		return LivePTTResolution{}, err
	}
	ctx.Capabilities = CapabilityNode
	params := CreateResolvedTransmissionParams{AudienceKind: TransmissionAudienceCurrentAir,
		OriginKind: TransmissionOriginMicrophone, RequestedDelivery: TransmissionDeliveryOverlay,
		AcceptedAt: nowMS, PolicyAt: nowMS, IncludeOrigin: false}
	resolved, domainKind, domainID, policy, err := resolveTransmissionAudienceTx(tx, ctx, params)
	if err != nil {
		return LivePTTResolution{}, err
	}
	authorization, err := authorizeAirPolicyTx(ctx, policy, AirPolicyOverlay)
	if err != nil {
		return LivePTTResolution{}, err
	}
	result := LivePTTResolution{SourceActorID: ctx.ActorID, DomainKind: "barycenter", DomainID: domainID,
		PolicyRevision: authorization.PolicyRevision}
	if policy != nil && policy.AirStatus == "active" {
		result.DomainKind = "air"
	}
	if domainKind == PlaybackDomainOrbit {
		result.DomainID = ctx.OrbitID
	}
	current := make(map[string]LivePTTAvailability, len(availability))
	for _, item := range availability {
		current[transmissionTargetKey(item.OrbitID, item.Slot)] = item
	}
	for _, target := range resolved {
		item := current[transmissionTargetKey(target.OrbitID, target.Slot)]
		out := LivePTTResolvedTarget{OrbitID: target.OrbitID, ActorID: target.ActorID, Slot: target.Slot}
		bindingMatches := item.CredentialTokenHash != "" && (item.CredentialTokenHash == target.NodeTokenHash || item.CredentialTokenHash == target.ControlTokenHash)
		online := bindingMatches && item.Connected && item.LastSeenAt > 0 &&
			nowMS-item.LastSeenAt <= transmissionPresenceFreshFor.Milliseconds() &&
			item.LastSeenAt-nowMS <= transmissionPresenceFreshFor.Milliseconds()
		block, blockErr := transmissionBlockDecisionTx(tx, target.OrbitID, target.ActorID, ctx.OrbitID, ctx.ActorID)
		if blockErr != nil {
			return LivePTTResolution{}, blockErr
		}
		dnd, dndErr := effectiveDNDTx(tx, target, nowMS)
		if dndErr != nil {
			return LivePTTResolution{}, dndErr
		}
		switch {
		case block.Blocked:
			out.Reason = "blocked"
		case dnd.Mode != DNDAllowAll:
			out.Reason = "dnd"
		case !online:
			out.Reason = "offline"
		case !item.SupportsLivePTT:
			out.Reason = "unsupported"
		default:
			result.Targets = append(result.Targets, out)
			continue
		}
		result.Excluded = append(result.Excluded, out)
	}
	return result, tx.Commit()
}
