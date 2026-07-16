package store

import (
	"database/sql"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"time"
)

var (
	ErrTransmissionMediaNotFound           = errors.New("transmission media was not found")
	ErrTransmissionMediaNotReady           = errors.New("transmission media is not ready")
	ErrTransmissionAudienceNotFound        = errors.New("transmission audience was not found")
	ErrTransmissionAudienceEmpty           = errors.New("transmission audience is empty")
	ErrTransmissionIdempotencyConflict     = errors.New("transmission idempotency key conflicts")
	ErrTransmissionConfirmationInvalid     = errors.New("transmission fallback confirmation is invalid")
	ErrTransmissionDeliveryKindMismatch    = errors.New("transmission delivery and media kind mismatch")
	ErrTransmissionOverlayDurationExceeded = errors.New("transmission overlay duration exceeded")
	ErrTransmissionUnsupportedTargets      = errors.New("transmission targets do not support the requested delivery")
	ErrTransmissionReplayDepthExceeded     = errors.New("transmission replay depth exceeded")
)

const (
	TransmissionDowngradeMissingOverlay        = "mandatory_target_missing_overlay_capability"
	TransmissionDowngradeConfirmedOverlay      = "sender_confirmed_overlay_fallback"
	TransmissionDowngradeConfirmedAfterCurrent = "sender_confirmed_after_current_fallback"

	transmissionPresenceFreshFor = 12 * time.Second
	transmissionConfirmationTTL  = 5 * time.Minute

	TransmissionCapabilityMediaClip    = "media_clip_v1"
	TransmissionCapabilityOverlayMix   = "overlay_mix_v1"
	TransmissionCapabilityInterrupt    = "interrupt_resume_v1"
	TransmissionCapabilityAudioTrack   = "audio_track_v1"
	TransmissionCapabilityQueueReplace = "queue_replace_v1"
	TransmissionCapabilityStream       = "stream_variant_v1"
)

var transmissionDigestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)

type TransmissionAudienceSelectorKind string

const (
	TransmissionSelectorBarycenter TransmissionAudienceSelectorKind = "barycenter"
	TransmissionSelectorPulsar     TransmissionAudienceSelectorKind = "pulsar"
)

// TransmissionAudienceSelector carries an opaque caller capability. Numeric
// fields are populated only after server-side resolution and never accepted
// from an application request.
type TransmissionAudienceSelector struct {
	Reference string
	Kind      TransmissionAudienceSelectorKind
	OrbitID   int64
	Slot      string
}

// TransmissionTargetAvailability is a point-in-time projection from the
// authenticated WS hub. It carries no authority: the repository resolves and
// revalidates every actor/binding tuple before persisting it.
type TransmissionTargetAvailability struct {
	OrbitID              int64
	Slot                 string
	Connected            bool
	LastSeenAt           int64
	CredentialTokenHash  string
	MediaClipCapable     bool
	OverlayCapable       bool
	InterruptCapable     bool
	MainActive           bool
	InterruptResumeReady bool
	// Capabilities retains the exact canonical register set for additive
	// policies such as streamed tracks. Phase 1 booleans remain the rollback
	// compatibility projection used by the existing clip scheduler.
	Capabilities []string
}

type ConfirmTransmissionFallback struct {
	TokenHash string
	Delivery  TransmissionDelivery
}

type CreateResolvedTransmissionParams struct {
	ExpectedActorID int64
	Bearer          string
	// Identity is used by verified non-HTTP transports such as Telegram.
	// Bearer remains the application API compatibility field; callers must
	// provide exactly one proof kind.
	Identity           Identity
	IdempotencyKeyHash string
	RequestHash        string
	MediaID            string
	AudienceKind       TransmissionAudienceKind
	Selectors          []TransmissionAudienceSelector
	OriginKind         TransmissionOriginKind
	IncludeOrigin      bool
	RequestedDelivery  TransmissionDelivery
	AcceptedAt         int64
	// PolicyAt defaults to AcceptedAt. Telegram defaults retain the trusted
	// inbound FIFO timestamp while readiness policy is resolved after ingest.
	PolicyAt           int64
	Availability       []TransmissionTargetAvailability
	Confirmation       *ConfirmTransmissionFallback
	ChallengeTokenHash string
	// ReplayInboxID is set only by the authorized inbox replay service. It
	// permits the exact current target to reuse sender-owned media and seals
	// lineage in the same transaction as the new transmission acceptance.
	ReplayInboxID string
}

type TransmissionAlternative struct {
	Delivery  TransmissionDelivery
	Available bool
	Reason    string
}

type TransmissionChallenge struct {
	ExpiresAt    int64
	Alternatives []TransmissionAlternative
}

type ResolvedTransmissionCreation struct {
	Creation  TransmissionCreation
	Reused    bool
	Challenge *TransmissionChallenge
}

type TransmissionOverlayDurationError struct {
	Alternatives []TransmissionAlternative
}

type UnsupportedTransmissionTarget struct {
	Reference           string
	MissingCapabilities []string
}

type TransmissionUnsupportedTargetsError struct {
	Targets []UnsupportedTransmissionTarget
}

func (e *TransmissionUnsupportedTargetsError) Error() string {
	return ErrTransmissionUnsupportedTargets.Error()
}

func (e *TransmissionUnsupportedTargetsError) Unwrap() error {
	return ErrTransmissionUnsupportedTargets
}

// RequiredTransmissionCapabilities is the one transport-neutral capability
// policy for explicit clip and future streamed-track targets.
func RequiredTransmissionCapabilities(
	mediaKind MediaKind,
	delivery TransmissionDelivery,
) ([]string, error) {
	var required []string
	switch mediaKind {
	case MediaKindVoiceClip, MediaKindAudioClip, MediaKindBuiltinCue:
		required = []string{TransmissionCapabilityMediaClip}
		switch delivery {
		case TransmissionDeliveryOverlay:
			required = append(required, TransmissionCapabilityOverlayMix)
		case TransmissionDeliveryInterrupt:
			required = append(required, TransmissionCapabilityInterrupt)
		case TransmissionDeliveryAfterCurrent:
		default:
			return nil, ErrTransmissionDeliveryKindMismatch
		}
	case MediaKindAudioTrack:
		if delivery != TransmissionDelivery("queue") && delivery != TransmissionDelivery("replace") {
			return nil, ErrTransmissionDeliveryKindMismatch
		}
		required = []string{
			TransmissionCapabilityAudioTrack,
			TransmissionCapabilityQueueReplace,
			TransmissionCapabilityStream,
		}
	default:
		return nil, ErrTransmissionDeliveryKindMismatch
	}
	sort.Strings(required)
	return required, nil
}

func missingTransmissionCapabilities(
	availability TransmissionTargetAvailability,
	mediaKind MediaKind,
	delivery TransmissionDelivery,
) ([]string, error) {
	required, err := RequiredTransmissionCapabilities(mediaKind, delivery)
	if err != nil {
		return nil, err
	}
	available := make(map[string]bool, len(availability.Capabilities)+3)
	for _, capability := range availability.Capabilities {
		available[capability] = true
	}
	available[TransmissionCapabilityMediaClip] = availability.MediaClipCapable ||
		available[TransmissionCapabilityMediaClip]
	available[TransmissionCapabilityOverlayMix] = availability.OverlayCapable ||
		available[TransmissionCapabilityOverlayMix]
	available[TransmissionCapabilityInterrupt] =
		(availability.InterruptCapable &&
			(!availability.MainActive || availability.InterruptResumeReady)) ||
			available[TransmissionCapabilityInterrupt]
	var missing []string
	for _, capability := range required {
		if !available[capability] {
			missing = append(missing, capability)
		}
	}
	return missing, nil
}

func (e *TransmissionOverlayDurationError) Error() string {
	return ErrTransmissionOverlayDurationExceeded.Error()
}

func (e *TransmissionOverlayDurationError) Unwrap() error {
	return ErrTransmissionOverlayDurationExceeded
}

type resolvedTransmissionTarget struct {
	OrbitID          int64
	ActorID          int64
	Slot             string
	BindingPairedAt  int64
	Reference        string
	NodeTokenHash    string
	ControlTokenHash string
}

type storedTransmissionConfirmation struct {
	ActorID               int64
	IdempotencyKeyHash    string
	RequestHash           string
	OverlayAvailable      bool
	AfterCurrentAvailable bool
	CreatedAt             int64
	ExpiresAt             int64
	ConsumedAt            int64
}

func validResolvedTransmissionParams(params CreateResolvedTransmissionParams) bool {
	identityValid := (params.Bearer != "" && params.Identity == (Identity{})) ||
		(params.Bearer == "" && params.Identity.Kind == IdentityTelegram &&
			params.Identity.TelegramUserID > 0)
	if params.ExpectedActorID <= 0 || !identityValid ||
		!transmissionDigestPattern.MatchString(params.IdempotencyKeyHash) ||
		!transmissionDigestPattern.MatchString(params.RequestHash) ||
		!mediaItemIDPattern.MatchString(params.MediaID) || params.AcceptedAt <= 0 ||
		!validTransmissionDelivery(params.RequestedDelivery) {
		return false
	}
	if params.ReplayInboxID != "" &&
		(!transmissionInboxIDPattern.MatchString(params.ReplayInboxID) ||
			params.AudienceKind != TransmissionAudienceThisPulsar ||
			params.OriginKind != TransmissionOriginFile || !params.IncludeOrigin) {
		return false
	}
	if params.PolicyAt != 0 && params.PolicyAt < params.AcceptedAt {
		return false
	}
	switch params.AudienceKind {
	case TransmissionAudienceThisPulsar, TransmissionAudienceOwnBarycenter,
		TransmissionAudienceCurrentAir:
		if len(params.Selectors) != 0 {
			return false
		}
	case TransmissionAudienceExplicit:
		if len(params.Selectors) == 0 || len(params.Selectors) > 64 {
			return false
		}
	default:
		return false
	}
	switch params.OriginKind {
	case TransmissionOriginMicrophone, TransmissionOriginFile,
		TransmissionOriginTelegram, TransmissionOriginBuiltin:
	default:
		return false
	}
	for _, selector := range params.Selectors {
		if !transmissionTargetReferencePattern.MatchString(selector.Reference) ||
			selector.Kind != "" || selector.OrbitID != 0 || selector.Slot != "" {
			return false
		}
	}
	seenAvailability := make(map[string]struct{}, len(params.Availability))
	for _, availability := range params.Availability {
		if availability.OrbitID <= 0 ||
			!transmissionSlotPattern.MatchString(availability.Slot) ||
			availability.LastSeenAt < 0 ||
			(availability.CredentialTokenHash != "" &&
				!transmissionDigestPattern.MatchString(availability.CredentialTokenHash)) {
			return false
		}
		key := transmissionTargetKey(availability.OrbitID, availability.Slot)
		if _, exists := seenAvailability[key]; exists {
			return false
		}
		for index, capability := range availability.Capabilities {
			if capability == "" || (index > 0 &&
				availability.Capabilities[index-1] >= capability) {
				return false
			}
			for _, b := range []byte(capability) {
				if b < 0x21 || b > 0x7e {
					return false
				}
			}
		}
		seenAvailability[key] = struct{}{}
	}
	if params.Confirmation != nil {
		if params.RequestedDelivery != TransmissionDeliveryInterrupt ||
			!transmissionDigestPattern.MatchString(params.Confirmation.TokenHash) ||
			(params.Confirmation.Delivery != TransmissionDeliveryOverlay &&
				params.Confirmation.Delivery != TransmissionDeliveryAfterCurrent) {
			return false
		}
	}
	if params.RequestedDelivery == TransmissionDeliveryInterrupt &&
		!transmissionDigestPattern.MatchString(params.ChallengeTokenHash) {
		return false
	}
	return true
}

func authorizeTransmissionControlTx(
	tx *sql.Tx,
	expectedActorID int64,
	params CreateResolvedTransmissionParams,
) (ActorContext, error) {
	ctx, _, err := authorizeTransmissionProofTx(
		tx, expectedActorID, params.Bearer, params.Identity,
	)
	return ctx, err
}

func loadTransmissionCreationTx(tx *sql.Tx, id string) (TransmissionCreation, error) {
	transmission, err := scanTransmission(tx.QueryRow(
		`SELECT `+transmissionColumns+` FROM transmissions WHERE id = ?`, id,
	))
	if errors.Is(err, sql.ErrNoRows) {
		return TransmissionCreation{}, ErrTransmissionNotFound
	}
	if err != nil {
		return TransmissionCreation{}, err
	}
	rows, err := tx.Query(`SELECT `+transmissionTargetColumns+`
FROM transmission_targets WHERE transmission_id = ? ORDER BY orbit_id, slot`, id)
	if err != nil {
		return TransmissionCreation{}, err
	}
	defer rows.Close()
	var targets []TransmissionTarget
	for rows.Next() {
		target, err := scanTransmissionTarget(rows)
		if err != nil {
			return TransmissionCreation{}, err
		}
		targets = append(targets, target)
	}
	if err := rows.Err(); err != nil {
		return TransmissionCreation{}, err
	}
	return TransmissionCreation{Transmission: transmission, Targets: targets}, nil
}

func transmissionIdempotentReplayTx(
	tx *sql.Tx,
	actorID int64,
	keyHash string,
	requestHash string,
) (*TransmissionCreation, error) {
	var storedRequestHash, transmissionID string
	err := tx.QueryRow(`SELECT request_hash, transmission_id
FROM transmission_requests WHERE actor_id = ? AND idempotency_key_hash = ?`,
		actorID, keyHash,
	).Scan(&storedRequestHash, &transmissionID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if storedRequestHash != requestHash {
		return nil, ErrTransmissionIdempotencyConflict
	}
	creation, err := loadTransmissionCreationTx(tx, transmissionID)
	if err != nil {
		return nil, err
	}
	return &creation, nil
}

func loadTransmissionConfirmationTx(
	tx *sql.Tx,
	tokenHash string,
) (storedTransmissionConfirmation, error) {
	var confirmation storedTransmissionConfirmation
	var overlay, afterCurrent int
	err := tx.QueryRow(`SELECT actor_id, idempotency_key_hash, request_hash,
       overlay_available, after_current_available, created_at, expires_at, consumed_at
FROM transmission_fallback_confirmations WHERE token_hash = ?`, tokenHash).Scan(
		&confirmation.ActorID, &confirmation.IdempotencyKeyHash,
		&confirmation.RequestHash, &overlay, &afterCurrent,
		&confirmation.CreatedAt, &confirmation.ExpiresAt, &confirmation.ConsumedAt,
	)
	confirmation.OverlayAvailable = overlay != 0
	confirmation.AfterCurrentAvailable = afterCurrent != 0
	if errors.Is(err, sql.ErrNoRows) {
		return storedTransmissionConfirmation{}, ErrTransmissionConfirmationInvalid
	}
	return confirmation, err
}

func confirmationMatches(
	confirmation storedTransmissionConfirmation,
	params CreateResolvedTransmissionParams,
) bool {
	if params.Confirmation == nil || confirmation.ActorID != params.ExpectedActorID ||
		confirmation.IdempotencyKeyHash != params.IdempotencyKeyHash ||
		confirmation.RequestHash != params.RequestHash ||
		confirmation.ConsumedAt != 0 || confirmation.CreatedAt > params.AcceptedAt ||
		confirmation.ExpiresAt <= params.AcceptedAt {
		return false
	}
	switch params.Confirmation.Delivery {
	case TransmissionDeliveryOverlay:
		return confirmation.OverlayAvailable
	case TransmissionDeliveryAfterCurrent:
		return confirmation.AfterCurrentAvailable
	default:
		return false
	}
}

func resolveTransmissionMediaTx(
	tx *sql.Tx,
	ctx ActorContext,
	params CreateResolvedTransmissionParams,
) (MediaItem, error) {
	query := `SELECT ` + mediaItemColumns + ` FROM media_items WHERE id = ? AND owner_orbit_id = ?`
	args := []any{params.MediaID, ctx.OrbitID}
	if params.ReplayInboxID != "" {
		pairedAt, err := currentInboxBindingTx(tx, ctx)
		if err != nil {
			return MediaItem{}, ErrTransmissionMediaNotFound
		}
		inbox, err := loadAuthorizedInboxItemTx(
			tx, ctx, pairedAt, params.ReplayInboxID, params.AcceptedAt,
		)
		if err != nil || inbox.Availability != TransmissionInboxAvailable ||
			inbox.MediaID != params.MediaID {
			return MediaItem{}, ErrTransmissionMediaNotFound
		}
		if inbox.ReplayDepth >= 8 {
			return MediaItem{}, ErrTransmissionReplayDepthExceeded
		}
		query = `SELECT ` + mediaItemColumns + ` FROM media_items WHERE id = ?`
		args = []any{params.MediaID}
	}
	mediaItem, err := scanMediaItem(tx.QueryRow(query, args...))
	if errors.Is(err, sql.ErrNoRows) {
		return MediaItem{}, ErrTransmissionMediaNotFound
	}
	if err != nil {
		return MediaItem{}, err
	}
	policyAt := params.AcceptedAt
	if params.PolicyAt > 0 {
		policyAt = params.PolicyAt
	}
	if mediaItem.Status == MediaStatusDeleted || mediaItem.Status == MediaStatusExpired ||
		mediaItem.ExpiresAt <= policyAt {
		return MediaItem{}, ErrTransmissionMediaNotFound
	}
	if mediaItem.Status != MediaStatusReady || mediaItem.PublishedAt <= 0 ||
		mediaItem.PublishedAt > policyAt {
		return MediaItem{}, ErrTransmissionMediaNotReady
	}
	if mediaItem.Kind == MediaKindAudioTrack {
		return MediaItem{}, ErrTransmissionDeliveryKindMismatch
	}
	switch mediaItem.Kind {
	case MediaKindVoiceClip, MediaKindAudioClip, MediaKindBuiltinCue:
	default:
		return MediaItem{}, ErrTransmissionDeliveryKindMismatch
	}
	return mediaItem, nil
}

func transmissionTargetKey(orbitID int64, slot string) string {
	return fmt.Sprintf("%d/%s", orbitID, slot)
}

func liveTransmissionTargetsTx(
	tx *sql.Tx,
	orbitID int64,
	slot string,
) ([]resolvedTransmissionTarget, error) {
	query := `SELECT ic.slot_orbit_id, ic.actor_id, ic.slot_name,
	       ic.slot_paired_at,
	       ic.binding_token_hash, COALESCE(ic.control_token_hash, '')
FROM installation_credentials ic
JOIN actors a ON a.id = ic.actor_id AND a.revoked_at IS NULL
JOIN memberships m ON m.actor_id = ic.actor_id
  AND m.orbit_id = ic.slot_orbit_id AND m.left_at IS NULL
JOIN orbits o ON o.id = ic.slot_orbit_id AND o.status = 'active'
JOIN slots sl ON sl.orbit_id = ic.slot_orbit_id AND sl.slot = ic.slot_name
  AND sl.revoked_at IS NULL AND sl.token_hash = ic.binding_token_hash
  AND COALESCE(sl.paired_at, 0) = ic.slot_paired_at
WHERE ic.slot_orbit_id = ?`
	args := []any{orbitID}
	if slot != "" {
		query += ` AND ic.slot_name = ?`
		args = append(args, slot)
	}
	query += ` ORDER BY ic.slot_orbit_id, ic.slot_name`
	rows, err := tx.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var targets []resolvedTransmissionTarget
	for rows.Next() {
		var target resolvedTransmissionTarget
		if err := rows.Scan(
			&target.OrbitID, &target.ActorID, &target.Slot,
			&target.BindingPairedAt,
			&target.NodeTokenHash, &target.ControlTokenHash,
		); err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return targets, rows.Err()
}

func transmissionDomainTx(
	tx *sql.Tx,
	sourceOrbitID int64,
	policyContext *airPolicyContext,
) (PlaybackDomainKind, int64, map[int64]struct{}, error) {
	allowed := map[int64]struct{}{sourceOrbitID: {}}
	if policyContext != nil {
		// A parked Air has no shared runtime. Work accepted there remains scoped
		// to the source barycenter and cannot expand when another member later
		// activates. An active Air snapshots only current joined pointers.
		if policyContext.AirStatus != "active" {
			return PlaybackDomainOrbit, sourceOrbitID, allowed, nil
		}
		rows, err := tx.Query(`SELECT m.orbit_id
FROM air_members m JOIN air_active_pointers p
  ON p.air_id = m.air_id AND p.orbit_id = m.orbit_id
WHERE m.air_id = ? AND m.status = 'joined' ORDER BY m.orbit_id`, policyContext.AirID)
		if err != nil {
			return "", 0, nil, err
		}
		for rows.Next() {
			var orbitID int64
			if err := rows.Scan(&orbitID); err != nil {
				rows.Close()
				return "", 0, nil, err
			}
			allowed[orbitID] = struct{}{}
		}
		if err := rows.Close(); err != nil {
			return "", 0, nil, err
		}
		domainID, err := airPlaybackDomainTx(tx, policyContext.AirID)
		if err != nil {
			return "", 0, nil, err
		}
		return PlaybackDomainApproach, domainID, allowed, nil
	}
	rows, err := tx.Query(`SELECT id, orbit_a, orbit_b FROM links
WHERE state = 'active' AND (orbit_a = ? OR orbit_b = ?)
ORDER BY id LIMIT 2`,
		sourceOrbitID, sourceOrbitID,
	)
	if err != nil {
		return "", 0, nil, err
	}
	defer rows.Close()
	var links [][3]int64
	for rows.Next() {
		var link [3]int64
		if err := rows.Scan(&link[0], &link[1], &link[2]); err != nil {
			return "", 0, nil, err
		}
		links = append(links, link)
	}
	if err := rows.Err(); err != nil {
		return "", 0, nil, err
	}
	if len(links) == 0 {
		return PlaybackDomainOrbit, sourceOrbitID, allowed, nil
	}
	if len(links) != 1 {
		return "", 0, nil, fmt.Errorf("source orbit %d has multiple active links", sourceOrbitID)
	}
	linkID, orbitA, orbitB := links[0][0], links[0][1], links[0][2]
	if linkID <= 0 || orbitA <= 0 || orbitB <= 0 || orbitA == orbitB ||
		(orbitA != sourceOrbitID && orbitB != sourceOrbitID) {
		return "", 0, nil, fmt.Errorf("source orbit %d has an invalid active link", sourceOrbitID)
	}
	allowed[orbitA] = struct{}{}
	allowed[orbitB] = struct{}{}
	return PlaybackDomainApproach, linkID, allowed, nil
}

func resolveTransmissionAudienceTx(
	tx *sql.Tx,
	ctx ActorContext,
	params CreateResolvedTransmissionParams,
) ([]resolvedTransmissionTarget, PlaybackDomainKind, int64, *airPolicyContext, error) {
	policyContext, err := activeAirPolicyContextTx(tx, ctx.OrbitID)
	if err != nil {
		return nil, "", 0, nil, err
	}
	domainKind, domainID, allowedOrbits, err := transmissionDomainTx(tx, ctx.OrbitID, policyContext)
	if err != nil {
		return nil, "", 0, nil, err
	}
	resolved := make(map[string]resolvedTransmissionTarget)
	add := func(targets []resolvedTransmissionTarget, reference string) {
		for _, target := range targets {
			key := transmissionTargetKey(target.OrbitID, target.Slot)
			existing, found := resolved[key]
			if found && existing.Reference != "" &&
				(reference == "" || existing.Reference <= reference) {
				continue
			}
			target.Reference = reference
			resolved[key] = target
		}
	}
	switch params.AudienceKind {
	case TransmissionAudienceThisPulsar:
		if ctx.Slot == "" || (!params.IncludeOrigin) {
			return nil, "", 0, nil, ErrTransmissionInvalid
		}
		targets, err := liveTransmissionTargetsTx(tx, ctx.OrbitID, ctx.Slot)
		if err != nil {
			return nil, "", 0, nil, err
		}
		for _, target := range targets {
			if target.ActorID == ctx.ActorID {
				add([]resolvedTransmissionTarget{target}, "")
			}
		}
	case TransmissionAudienceOwnBarycenter:
		targets, err := liveTransmissionTargetsTx(tx, ctx.OrbitID, "")
		if err != nil {
			return nil, "", 0, nil, err
		}
		add(targets, "")
	case TransmissionAudienceCurrentAir:
		orbits := make([]int64, 0, len(allowedOrbits))
		for orbitID := range allowedOrbits {
			orbits = append(orbits, orbitID)
		}
		sort.Slice(orbits, func(i, j int) bool { return orbits[i] < orbits[j] })
		for _, orbitID := range orbits {
			targets, err := liveTransmissionTargetsTx(tx, orbitID, "")
			if err != nil {
				return nil, "", 0, nil, err
			}
			add(targets, "")
		}
	case TransmissionAudienceExplicit:
		proof, valid := transmissionProofIdentity(params.Bearer, params.Identity)
		if !valid {
			return nil, "", 0, nil, ErrUnauthorized
		}
		selectors := append([]TransmissionAudienceSelector(nil), params.Selectors...)
		sort.Slice(selectors, func(i, j int) bool {
			return selectors[i].Reference < selectors[j].Reference
		})
		for _, opaque := range selectors {
			selector, err := resolveTransmissionTargetReferenceTx(
				tx, ctx, proof, opaque.Reference, params.AcceptedAt, allowedOrbits,
			)
			if err != nil {
				return nil, "", 0, nil, err
			}
			targets, err := liveTransmissionTargetsTx(tx, selector.OrbitID, selector.Slot)
			if err != nil {
				return nil, "", 0, nil, err
			}
			if len(targets) == 0 ||
				(selector.Kind == TransmissionSelectorPulsar && len(targets) != 1) {
				return nil, "", 0, nil, ErrTransmissionAudienceNotFound
			}
			add(targets, selector.Reference)
		}
	}
	if len(resolved) == 0 {
		return nil, "", 0, nil, ErrTransmissionAudienceEmpty
	}
	if !params.IncludeOrigin && ctx.Slot != "" {
		delete(resolved, transmissionTargetKey(ctx.OrbitID, ctx.Slot))
	}
	if len(resolved) == 0 {
		return nil, "", 0, nil, ErrTransmissionAudienceEmpty
	}
	targets := make([]resolvedTransmissionTarget, 0, len(resolved))
	for _, target := range resolved {
		targets = append(targets, target)
	}
	sort.Slice(targets, func(i, j int) bool {
		if targets[i].OrbitID == targets[j].OrbitID {
			return targets[i].Slot < targets[j].Slot
		}
		return targets[i].OrbitID < targets[j].OrbitID
	})
	return targets, domainKind, domainID, policyContext, nil
}

func transmissionBlockDecisionTx(
	tx *sql.Tx,
	recipientOrbitID, recipientActorID, sourceOrbitID, sourceActorID int64,
) (TransmissionBlockDecision, error) {
	var actorBlocks, orbitBlocks int
	err := tx.QueryRow(`SELECT
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
		return TransmissionBlockDecision{Blocked: true, Reason: TransmissionReasonActorBlocked}, nil
	}
	if orbitBlocks > 0 {
		return TransmissionBlockDecision{Blocked: true, Reason: TransmissionReasonOrbitBlocked}, nil
	}
	return TransmissionBlockDecision{}, nil
}

// transmissionReportDecisionTx is deliberately recipient-local. A report
// protects only the actor who submitted it from another delivery of the same
// media; it cannot quarantine content or affect another target.
func transmissionReportDecisionTx(
	tx *sql.Tx,
	recipientActorID int64,
	mediaID string,
) (bool, error) {
	var reports int
	err := tx.QueryRow(`SELECT COUNT(*) FROM moderation_reports
WHERE reporter_actor_id = ? AND media_id = ?`, recipientActorID, mediaID).Scan(&reports)
	return reports > 0, err
}

func effectiveDNDTx(
	tx *sql.Tx,
	target resolvedTransmissionTarget,
	now int64,
) (EffectiveDND, error) {
	decision := EffectiveDND{Mode: DNDAllowAll}
	var nodeMode DNDMode
	var nodeUntil, nodeRevision int64
	err := tx.QueryRow(`SELECT d.mode, d.muted_until, d.revision
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
	err = tx.QueryRow(`SELECT mode, muted_until, revision
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

func evaluateTransmissionTargetsTx(
	tx *sql.Tx,
	ctx ActorContext,
	mediaItem MediaItem,
	resolved []resolvedTransmissionTarget,
	params CreateResolvedTransmissionParams,
) ([]CreateTransmissionTarget, bool, bool, []UnsupportedTransmissionTarget, error) {
	policyAt := params.AcceptedAt
	if params.PolicyAt > 0 {
		policyAt = params.PolicyAt
	}
	availability := make(map[string]TransmissionTargetAvailability, len(params.Availability))
	for _, current := range params.Availability {
		availability[transmissionTargetKey(current.OrbitID, current.Slot)] = current
	}
	targets := make([]CreateTransmissionTarget, 0, len(resolved))
	missingOverlay, missingInterrupt := false, false
	unsupportedByReference := make(map[string]map[string]struct{})
	for _, identity := range resolved {
		current := availability[transmissionTargetKey(identity.OrbitID, identity.Slot)]
		bindingMatches := current.CredentialTokenHash != "" &&
			(current.CredentialTokenHash == identity.NodeTokenHash ||
				current.CredentialTokenHash == identity.ControlTokenHash)
		online := bindingMatches && current.Connected && current.LastSeenAt > 0 &&
			policyAt-current.LastSeenAt <= transmissionPresenceFreshFor.Milliseconds() &&
			current.LastSeenAt-policyAt <= transmissionPresenceFreshFor.Milliseconds()
		target := CreateTransmissionTarget{
			OrbitID: identity.OrbitID, ActorID: identity.ActorID, Slot: identity.Slot,
			OnlineAtAcceptance: online,
		}
		if online {
			target.CapabilitySetHash = transmissionTargetCapabilitySetHash(
				current.Capabilities, current.MediaClipCapable, current.OverlayCapable,
				current.InterruptCapable, current.InterruptResumeReady,
			)
			target.MediaClipCapable = current.MediaClipCapable
			target.OverlayCapable = current.OverlayCapable
			target.InterruptCapable = current.InterruptCapable
			target.InterruptResumeReady = current.InterruptResumeReady
		}
		block, err := transmissionBlockDecisionTx(
			tx, identity.OrbitID, identity.ActorID, ctx.OrbitID, ctx.ActorID,
		)
		if err != nil {
			return nil, false, false, nil, err
		}
		reported, err := transmissionReportDecisionTx(tx, identity.ActorID, mediaItem.ID)
		if err != nil {
			return nil, false, false, nil, err
		}
		localThisPulsar := params.AudienceKind == TransmissionAudienceThisPulsar &&
			identity.OrbitID == ctx.OrbitID && identity.ActorID == ctx.ActorID &&
			identity.Slot == ctx.Slot
		dnd, err := effectiveDNDTx(tx, identity, policyAt)
		if err != nil {
			return nil, false, false, nil, err
		}
		dndSuppresses := !localThisPulsar &&
			(dnd.Mode == DNDMutedUntil ||
				(dnd.Mode == DNDMessagesOnly && mediaItem.Kind == MediaKindBuiltinCue))
		switch {
		case reported:
			target.Status = TransmissionTargetBlocked
			target.ReasonCode = TransmissionReasonReported
		case block.Blocked:
			target.Status = TransmissionTargetBlocked
			target.ReasonCode = block.Reason
		case dndSuppresses:
			target.Status = TransmissionTargetMissedDND
			target.ReasonCode = dnd.Reason
		case !online && params.RequestedDelivery != TransmissionDeliveryAfterCurrent:
			target.Status = TransmissionTargetMissedOffline
			target.ReasonCode = TransmissionReasonOfflineAtAcceptance
		default:
			target.Status = TransmissionTargetAccepted
		}
		mandatory := target.Status == TransmissionTargetAccepted && online
		if mandatory {
			missing, err := missingTransmissionCapabilities(
				current, mediaItem.Kind, params.RequestedDelivery,
			)
			if err != nil {
				return nil, false, false, nil, err
			}
			if params.AudienceKind == TransmissionAudienceExplicit && len(missing) > 0 {
				set := unsupportedByReference[identity.Reference]
				if set == nil {
					set = make(map[string]struct{})
					unsupportedByReference[identity.Reference] = set
				}
				for _, capability := range missing {
					set[capability] = struct{}{}
				}
			}
			if !target.MediaClipCapable || !target.OverlayCapable {
				missingOverlay = true
			}
			if !target.MediaClipCapable || !target.InterruptCapable ||
				(current.MainActive && !target.InterruptResumeReady) {
				missingInterrupt = true
			}
		}
		targets = append(targets, target)
	}
	unsupported := make([]UnsupportedTransmissionTarget, 0, len(unsupportedByReference))
	for reference, set := range unsupportedByReference {
		missing := make([]string, 0, len(set))
		for capability := range set {
			missing = append(missing, capability)
		}
		sort.Strings(missing)
		unsupported = append(unsupported, UnsupportedTransmissionTarget{
			Reference: reference, MissingCapabilities: missing,
		})
	}
	sort.Slice(unsupported, func(i, j int) bool {
		return unsupported[i].Reference < unsupported[j].Reference
	})
	return targets, missingOverlay, missingInterrupt, unsupported, nil
}

func interruptChallengeAlternatives(
	mediaItem MediaItem,
	missingOverlay bool,
) []TransmissionAlternative {
	overlayAvailable := mediaItem.DurationMS <= 60000 && !missingOverlay
	overlayReason := "interrupt_resume_unavailable"
	if mediaItem.DurationMS > 60000 {
		overlayReason = "overlay_duration_exceeded"
	} else if missingOverlay {
		overlayReason = TransmissionDowngradeMissingOverlay
	}
	return []TransmissionAlternative{
		{Delivery: TransmissionDeliveryOverlay, Available: overlayAvailable, Reason: overlayReason},
		{Delivery: TransmissionDeliveryAfterCurrent, Available: true, Reason: "interrupt_resume_unavailable"},
	}
}

func insertTransmissionChallengeTx(
	tx *sql.Tx,
	params CreateResolvedTransmissionParams,
	alternatives []TransmissionAlternative,
) (TransmissionChallenge, error) {
	overlayAvailable, afterCurrentAvailable := false, false
	for _, alternative := range alternatives {
		switch alternative.Delivery {
		case TransmissionDeliveryOverlay:
			overlayAvailable = alternative.Available
		case TransmissionDeliveryAfterCurrent:
			afterCurrentAvailable = alternative.Available
		}
	}
	expiresAt := params.AcceptedAt + transmissionConfirmationTTL.Milliseconds()
	if _, err := tx.Exec(`DELETE FROM transmission_fallback_confirmations
WHERE expires_at < ?`, params.AcceptedAt-int64((30*24*time.Hour)/time.Millisecond)); err != nil {
		return TransmissionChallenge{}, err
	}
	if _, err := tx.Exec(`INSERT INTO transmission_fallback_confirmations(
  token_hash, actor_id, idempotency_key_hash, request_hash,
  overlay_available, after_current_available, created_at, expires_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?)`, params.ChallengeTokenHash,
		params.ExpectedActorID, params.IdempotencyKeyHash, params.RequestHash,
		overlayAvailable, afterCurrentAvailable, params.AcceptedAt, expiresAt,
	); err != nil {
		return TransmissionChallenge{}, err
	}
	return TransmissionChallenge{ExpiresAt: expiresAt, Alternatives: alternatives}, nil
}

func consumeTransmissionConfirmationTx(
	tx *sql.Tx,
	tokenHash string,
	now int64,
) error {
	result, err := tx.Exec(`UPDATE transmission_fallback_confirmations
SET consumed_at = ? WHERE token_hash = ? AND consumed_at = 0 AND expires_at > ?`,
		now, tokenHash, now,
	)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrTransmissionConfirmationInvalid
	}
	return nil
}

// CreateResolvedTransmission is the HTTP/application-service acceptance
// boundary. It reauthenticates the control credential, expands selectors,
// applies block/DND/presence/capability policy and writes idempotency plus the
// immutable target snapshot in one immediate SQLite transaction.
func (s *Store) CreateResolvedTransmission(
	params CreateResolvedTransmissionParams,
) (ResolvedTransmissionCreation, error) {
	if !validResolvedTransmissionParams(params) {
		return ResolvedTransmissionCreation{}, ErrTransmissionInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return ResolvedTransmissionCreation{}, err
	}
	defer tx.Rollback()
	ctx, err := authorizeTransmissionControlTx(tx, params.ExpectedActorID, params)
	if err != nil {
		return ResolvedTransmissionCreation{}, err
	}
	result, err := s.createResolvedTransmissionTx(tx, ctx, params)
	if err != nil {
		return ResolvedTransmissionCreation{}, err
	}
	if err := tx.Commit(); err != nil {
		return ResolvedTransmissionCreation{}, err
	}
	return result, nil
}

// createResolvedTransmissionTx applies the common transmission policy inside
// a caller-owned writer transaction. Telegram routing uses this seam to make
// replacement of its not-started default and creation of the selected
// transmission indivisible.
func (s *Store) createResolvedTransmissionTx(
	tx *sql.Tx,
	ctx ActorContext,
	params CreateResolvedTransmissionParams,
) (ResolvedTransmissionCreation, error) {
	replay, err := transmissionIdempotentReplayTx(
		tx, ctx.ActorID, params.IdempotencyKeyHash, params.RequestHash,
	)
	if err != nil {
		return ResolvedTransmissionCreation{}, err
	}
	if replay != nil {
		return ResolvedTransmissionCreation{Creation: *replay, Reused: true}, nil
	}
	var confirmation storedTransmissionConfirmation
	if params.Confirmation != nil {
		confirmation, err = loadTransmissionConfirmationTx(tx, params.Confirmation.TokenHash)
		if err != nil || !confirmationMatches(confirmation, params) {
			return ResolvedTransmissionCreation{}, ErrTransmissionConfirmationInvalid
		}
	}
	mediaItem, err := resolveTransmissionMediaTx(tx, ctx, params)
	if err != nil {
		return ResolvedTransmissionCreation{}, err
	}
	if mediaItem.Source != MediaSourceSystem {
		legacyTelegramVoice := mediaItem.Source == MediaSourceTelegram &&
			mediaItem.Kind == MediaKindVoiceClip &&
			params.OriginKind == TransmissionOriginTelegram
		if !legacyTelegramVoice {
			if err := requireCurrentContentPolicyTx(tx, ctx); err != nil {
				return ResolvedTransmissionCreation{}, err
			}
		}
	}
	resolved, domainKind, domainID, policyContext, err := resolveTransmissionAudienceTx(tx, ctx, params)
	if err != nil {
		return ResolvedTransmissionCreation{}, err
	}
	requestedAuthorization, err := authorizeAirPolicyTx(
		ctx, policyContext, airPolicyOperationForDelivery(params.RequestedDelivery),
	)
	if err != nil {
		return ResolvedTransmissionCreation{}, err
	}
	targets, missingOverlay, missingInterrupt, unsupported, err := evaluateTransmissionTargetsTx(
		tx, ctx, mediaItem, resolved, params,
	)
	if err != nil {
		return ResolvedTransmissionCreation{}, err
	}
	if len(unsupported) > 0 {
		return ResolvedTransmissionCreation{}, &TransmissionUnsupportedTargetsError{
			Targets: unsupported,
		}
	}
	if params.RequestedDelivery == TransmissionDeliveryOverlay && mediaItem.DurationMS > 60000 {
		return ResolvedTransmissionCreation{}, &TransmissionOverlayDurationError{
			Alternatives: []TransmissionAlternative{
				{Delivery: TransmissionDeliveryInterrupt, Available: !missingInterrupt},
				{Delivery: TransmissionDeliveryAfterCurrent, Available: true},
			},
		}
	}

	effectiveDelivery := params.RequestedDelivery
	downgradeReason := ""
	if params.Confirmation == nil {
		switch params.RequestedDelivery {
		case TransmissionDeliveryOverlay:
			if missingOverlay {
				effectiveDelivery = TransmissionDeliveryAfterCurrent
				downgradeReason = TransmissionDowngradeMissingOverlay
			}
		case TransmissionDeliveryInterrupt:
			if missingInterrupt {
				challenge, err := insertTransmissionChallengeTx(
					tx, params, interruptChallengeAlternatives(mediaItem, missingOverlay),
				)
				if err != nil {
					return ResolvedTransmissionCreation{}, err
				}
				if err := s.checkpoint("transmission_confirmation_before_commit"); err != nil {
					return ResolvedTransmissionCreation{}, err
				}
				return ResolvedTransmissionCreation{Challenge: &challenge}, nil
			}
		}
	} else {
		selectedAvailable := params.Confirmation.Delivery == TransmissionDeliveryAfterCurrent ||
			(params.Confirmation.Delivery == TransmissionDeliveryOverlay &&
				mediaItem.DurationMS <= 60000 && !missingOverlay)
		if !selectedAvailable {
			if err := consumeTransmissionConfirmationTx(
				tx, params.Confirmation.TokenHash, params.AcceptedAt,
			); err != nil {
				return ResolvedTransmissionCreation{}, err
			}
			challenge, err := insertTransmissionChallengeTx(
				tx, params, interruptChallengeAlternatives(mediaItem, missingOverlay),
			)
			if err != nil {
				return ResolvedTransmissionCreation{}, err
			}
			return ResolvedTransmissionCreation{Challenge: &challenge}, nil
		}
		if err := consumeTransmissionConfirmationTx(
			tx, params.Confirmation.TokenHash, params.AcceptedAt,
		); err != nil {
			return ResolvedTransmissionCreation{}, err
		}
		effectiveDelivery = params.Confirmation.Delivery
		if effectiveDelivery == TransmissionDeliveryOverlay {
			downgradeReason = TransmissionDowngradeConfirmedOverlay
		} else {
			downgradeReason = TransmissionDowngradeConfirmedAfterCurrent
		}
	}

	createParams := CreateTransmissionParams{
		MediaID: params.MediaID, SourceOrbitID: ctx.OrbitID,
		SourceActorID: ctx.ActorID, SourceSlot: ctx.Slot,
		PlaybackDomainKind: domainKind, PlaybackDomainID: domainID,
		AudienceKind: params.AudienceKind, OriginKind: params.OriginKind,
		IncludeOrigin:     params.IncludeOrigin,
		RequestedDelivery: params.RequestedDelivery,
		EffectiveDelivery: effectiveDelivery, DowngradeReason: downgradeReason,
		AcceptedAt: params.AcceptedAt, Targets: targets,
	}
	acceptedAuthorization := requestedAuthorization
	acceptedOperation := airPolicyOperationForDelivery(effectiveDelivery)
	if acceptedOperation != requestedAuthorization.Operation {
		acceptedAuthorization, err = authorizeAirPolicyTx(ctx, policyContext, acceptedOperation)
		if err != nil {
			return ResolvedTransmissionCreation{}, err
		}
	}
	createParams.AirID = acceptedAuthorization.AirID
	createParams.AirPolicyRevision = acceptedAuthorization.PolicyRevision
	createParams.AirPolicyOperation = acceptedAuthorization.Operation
	createParams.AirPolicyResult = acceptedAuthorization.Result
	if err := validateCreateTransmission(createParams); err != nil {
		return ResolvedTransmissionCreation{}, err
	}
	creation, err := s.createTransmissionTx(tx, createParams, mediaItem)
	if err != nil {
		return ResolvedTransmissionCreation{}, err
	}
	if acceptedAuthorization.AirID != "" {
		if err := appendAirAuditTx(tx, acceptedAuthorization.AirID, policyContext.MemberID, "",
			ctx.ActorID, ctx.OrbitID, "air.policy.authorize."+string(acceptedAuthorization.Operation),
			airPolicyValue(*policyContext, acceptedAuthorization.Operation), creation.Transmission.ID,
			"ok", params.AcceptedAt); err != nil {
			return ResolvedTransmissionCreation{}, err
		}
	}
	if params.ReplayInboxID != "" {
		if err := commitTransmissionReplayLineageTx(
			tx, params.ReplayInboxID, creation.Transmission.ID, params.AcceptedAt,
		); err != nil {
			if errors.Is(err, ErrTransmissionInboxConflict) {
				return ResolvedTransmissionCreation{}, ErrTransmissionMediaNotFound
			}
			return ResolvedTransmissionCreation{}, err
		}
	}
	if _, err := tx.Exec(`INSERT INTO transmission_requests(
  actor_id, idempotency_key_hash, request_hash, transmission_id, created_at
) VALUES(?, ?, ?, ?, ?)`, ctx.ActorID, params.IdempotencyKeyHash,
		params.RequestHash, creation.Transmission.ID, params.AcceptedAt,
	); err != nil {
		return ResolvedTransmissionCreation{}, err
	}
	if err := s.checkpoint("transmission_resolved_create_before_commit"); err != nil {
		return ResolvedTransmissionCreation{}, err
	}
	return ResolvedTransmissionCreation{Creation: creation}, nil
}
