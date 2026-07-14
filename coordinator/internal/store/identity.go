package store

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

var (
	ErrSelfServiceOnboardingDisabled = errors.New("self_service_onboarding is disabled")
	ErrUnauthorized                  = errors.New("actor credential is not authorized")
	ErrInsufficientCapability        = errors.New("actor has insufficient capability")
	ErrOrbitDisabled                 = errors.New("orbit is disabled")
	ErrCredentialDomainConflict      = errors.New("credential digest crosses capability domains")
	ErrCredentialAlreadyProvisioned  = errors.New("installation credential is already provisioned")
	ErrIdentityAlignmentViolation    = errors.New("installation identity orbit alignment violation")
)

// Capability is a transport-neutral authorization capability. A control
// token includes node capability; Telegram is a verified transport principal
// whose role policy is evaluated by application services.
type Capability uint8

const (
	CapabilityNode Capability = 1 << iota
	CapabilityControl
	CapabilityTelegram
)

func (c Capability) Has(required Capability) bool { return c&required == required }

// ActorContext is the common identity returned to node, control-token, and
// verified Telegram callers. Slot is populated for app installations.
type ActorContext struct {
	OrbitID      int64
	ActorID      int64
	Role         string
	Slot         string
	Capabilities Capability
}

// IdentityKind identifies the transport proof supplied to ResolveActorContext.
type IdentityKind string

const (
	IdentityBearer   IdentityKind = "bearer"
	IdentityTelegram IdentityKind = "telegram"
)

type Identity struct {
	Kind           IdentityKind
	Token          string
	TelegramUserID int64
}

type rowQuerier interface {
	QueryRow(query string, args ...any) *sql.Row
}

// ResolveActorContext is the shared, feature-gated identity resolver.
func (s *Store) ResolveActorContext(identity Identity) (ActorContext, error) {
	if !s.selfServiceOnboarding {
		return ActorContext{}, ErrSelfServiceOnboardingDisabled
	}
	return resolveActorContext(s.db, identity)
}

func resolveActorContext(q rowQuerier, identity Identity) (ActorContext, error) {
	switch identity.Kind {
	case IdentityBearer:
		return resolveTokenActorContext(q, identity.Token)
	case IdentityTelegram:
		return resolveTelegramActorContext(q, identity.TelegramUserID)
	default:
		return ActorContext{}, ErrUnauthorized
	}
}

func (s *Store) ResolveTokenActorContext(token string) (ActorContext, error) {
	return s.ResolveActorContext(Identity{Kind: IdentityBearer, Token: token})
}

func (s *Store) ResolveTelegramActorContext(telegramUserID int64) (ActorContext, error) {
	return s.ResolveActorContext(Identity{Kind: IdentityTelegram, TelegramUserID: telegramUserID})
}

func resolveTokenActorContext(q rowQuerier, token string) (ActorContext, error) {
	digest := hashToken(token)
	var controlMatches, nodeMatches int
	if err := q.QueryRow(`SELECT
  (SELECT COUNT(*) FROM installation_credentials WHERE control_token_hash = ?),
  (SELECT COUNT(*) FROM slots WHERE token_hash = ?)`, digest, digest).Scan(&controlMatches, &nodeMatches); err != nil {
		return ActorContext{}, err
	}
	// Capability domains are intentionally resolved before lifecycle checks.
	// A digest must identify exactly one credential in exactly one domain;
	// ambiguous, duplicate, and cross-domain matches all fail closed.
	if controlMatches+nodeMatches != 1 {
		return ActorContext{}, ErrUnauthorized
	}

	// Stage 1: control credential + live slot binding. Lifecycle/role checks
	// intentionally happen in stage 2 so invalid binding maps to unauthorized.
	var actorID, orbitID int64
	var slot string
	var revokedAt sql.NullInt64
	if controlMatches == 1 {
		err := q.QueryRow(`SELECT ic.actor_id, ic.slot_orbit_id, ic.slot_name, a.revoked_at
FROM installation_credentials ic
JOIN actors a ON a.id = ic.actor_id
JOIN slots sl ON sl.orbit_id = ic.slot_orbit_id
  AND sl.slot = ic.slot_name
  AND sl.revoked_at IS NULL
  AND sl.token_hash = ic.binding_token_hash
  AND COALESCE(sl.paired_at, 0) = ic.slot_paired_at
WHERE ic.control_token_hash = ?`, digest).Scan(&actorID, &orbitID, &slot, &revokedAt)
		if errors.Is(err, sql.ErrNoRows) {
			return ActorContext{}, ErrUnauthorized
		}
		if err != nil {
			return ActorContext{}, err
		}
		if revokedAt.Valid {
			return ActorContext{}, ErrUnauthorized
		}
		return resolveActiveMembership(q, actorID, orbitID, slot, CapabilityNode|CapabilityControl)
	}

	// Node-token lookup starts from the authoritative legacy slots.token_hash,
	// then requires a current additive binding produced by reconciliation.
	var role sql.NullString
	var status string
	err := q.QueryRow(`SELECT sl.orbit_id, sl.slot, ic.actor_id, a.revoked_at,
       m.role, o.status
FROM slots sl
JOIN installation_credentials ic
  ON ic.slot_orbit_id = sl.orbit_id AND ic.slot_name = sl.slot
  AND ic.binding_token_hash = sl.token_hash
  AND ic.slot_paired_at = COALESCE(sl.paired_at, 0)
JOIN actors a ON a.id = ic.actor_id
JOIN orbits o ON o.id = sl.orbit_id
LEFT JOIN memberships m ON m.actor_id = ic.actor_id
  AND m.orbit_id = sl.orbit_id AND m.left_at IS NULL
WHERE sl.token_hash = ? AND sl.revoked_at IS NULL`, digest).Scan(
		&orbitID, &slot, &actorID, &revokedAt, &role, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return ActorContext{}, ErrUnauthorized
	}
	if err != nil {
		return ActorContext{}, err
	}
	if revokedAt.Valid {
		return ActorContext{}, ErrUnauthorized
	}
	if !role.Valid || status != "active" {
		return ActorContext{
			OrbitID: orbitID, ActorID: actorID, Slot: slot, Capabilities: CapabilityNode,
		}, ErrInsufficientCapability
	}
	return ActorContext{
		OrbitID:      orbitID,
		ActorID:      actorID,
		Role:         role.String,
		Slot:         slot,
		Capabilities: CapabilityNode,
	}, nil
}

func resolveTelegramActorContext(q rowQuerier, telegramUserID int64) (ActorContext, error) {
	if telegramUserID <= 0 {
		return ActorContext{}, ErrUnauthorized
	}
	var actorID int64
	var revokedAt sql.NullInt64
	err := q.QueryRow(`SELECT id, revoked_at FROM actors
WHERE kind = 'telegram_user' AND external_ref = ?`, strconv.FormatInt(telegramUserID, 10)).Scan(&actorID, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ActorContext{}, ErrUnauthorized
	}
	if err != nil {
		return ActorContext{}, err
	}
	known := ActorContext{ActorID: actorID}
	if revokedAt.Valid {
		return known, ErrUnauthorized
	}
	var orbitID int64
	var role, status string
	var leftAt sql.NullInt64
	err = q.QueryRow(`SELECT m.orbit_id, m.role, o.status, m.left_at
FROM memberships m JOIN orbits o ON o.id = m.orbit_id
WHERE m.actor_id = ?
ORDER BY (m.left_at IS NULL) DESC, m.joined_at DESC
LIMIT 1`, actorID).Scan(&orbitID, &role, &status, &leftAt)
	if errors.Is(err, sql.ErrNoRows) {
		return known, ErrInsufficientCapability
	}
	if err != nil {
		return ActorContext{}, err
	}
	if leftAt.Valid {
		return known, ErrInsufficientCapability
	}
	if status != "active" {
		return ActorContext{OrbitID: orbitID, ActorID: actorID, Role: role}, ErrInsufficientCapability
	}
	return ActorContext{
		OrbitID:      orbitID,
		ActorID:      actorID,
		Role:         role,
		Capabilities: CapabilityTelegram,
	}, nil
}

func resolveActiveMembership(q rowQuerier, actorID, orbitID int64, slot string, capabilities Capability) (ActorContext, error) {
	ctx := ActorContext{
		OrbitID: orbitID, ActorID: actorID, Slot: slot, Capabilities: capabilities,
	}
	var role, status string
	err := q.QueryRow(`SELECT m.role, o.status
FROM memberships m JOIN orbits o ON o.id = m.orbit_id
	WHERE m.actor_id = ? AND m.orbit_id = ? AND m.left_at IS NULL`, actorID, orbitID).Scan(&role, &status)
	if errors.Is(err, sql.ErrNoRows) {
		return ctx, ErrInsufficientCapability
	}
	if err != nil {
		return ActorContext{}, err
	}
	if status != "active" {
		return ctx, ErrInsufficientCapability
	}
	ctx.Role = role
	return ctx, nil
}

// LookupPlaybackToken preserves the previous LookupToken behavior when the
// feature is off. When enabled it applies actor binding, role, revocation, and
// orbit-status checks and accepts either node or control capability.
func (s *Store) LookupPlaybackToken(token string) (orbitID int64, slot string, ok bool, err error) {
	if !s.selfServiceOnboarding {
		return s.LookupToken(token)
	}
	ctx, err := s.ResolveTokenActorContext(token)
	if errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrInsufficientCapability) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, err
	}
	if !ctx.Capabilities.Has(CapabilityNode) {
		return 0, "", false, nil
	}
	return ctx.OrbitID, ctx.Slot, true, nil
}

// LookupLegacyMediaNodeToken narrows the legacy WAV endpoint to an actual node
// credential when additive identity is enabled. Control credentials retain
// their generic owner-read capability at /v1/media/{id}, but are not silently
// accepted as old playback-node credentials.
func (s *Store) LookupLegacyMediaNodeToken(token string) (orbitID int64, slot string, ok bool, err error) {
	if !s.selfServiceOnboarding {
		return s.LookupToken(token)
	}
	ctx, err := s.ResolveTokenActorContext(token)
	if errors.Is(err, ErrUnauthorized) || errors.Is(err, ErrInsufficientCapability) ||
		errors.Is(err, ErrOrbitDisabled) {
		return 0, "", false, nil
	}
	if err != nil {
		return 0, "", false, err
	}
	if !ctx.Capabilities.Has(CapabilityNode) || ctx.Capabilities.Has(CapabilityControl) {
		return 0, "", false, nil
	}
	return ctx.OrbitID, ctx.Slot, true, nil
}

const secretAlphabet = "ABCDEFGHJKMNPQRSTVWXYZ23456789"

var (
	lowerHexTokenPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
	recoveryIDPattern    = regexp.MustCompile(`^rec_[0-9a-f]{32}$`)
	humanSecretPattern   = regexp.MustCompile(`^[ABCDEFGHJKMNPQRSTVWXYZ2-9]{27}$`)
)

// generateSecret uses rejection sampling: each of the 30 symbols owns exactly
// eight byte values below 240, avoiding the legacy randomCode modulo bias.
func generateSecret(length int) (string, error) {
	if length <= 0 {
		return "", errors.New("secret length must be positive")
	}
	out := make([]byte, 0, length)
	buf := make([]byte, 32)
	for len(out) < length {
		if _, err := rand.Read(buf); err != nil {
			return "", err
		}
		for _, b := range buf {
			if b >= 240 {
				continue
			}
			out = append(out, secretAlphabet[int(b)/8])
			if len(out) == length {
				break
			}
		}
	}
	return string(out), nil
}

func normalizeHumanSecret(secret string) (string, error) {
	var b strings.Builder
	for _, r := range secret {
		switch r {
		case '-', ' ', '\t', '\r', '\n', '\v', '\f':
			continue
		default:
			b.WriteRune(r)
		}
	}
	canonical := strings.ToUpper(b.String())
	if !humanSecretPattern.MatchString(canonical) {
		return "", errors.New("secret does not match the canonical alphabet and length")
	}
	return canonical, nil
}

func constantTimeHashEqual(canonical, storedHex string) bool {
	return constantTimeDigestEqual(hashToken(canonical), storedHex)
}

func constantTimeDigestEqual(computedHex, storedHex string) bool {
	computed, err := hex.DecodeString(computedHex)
	if err != nil {
		return false
	}
	stored, err := hex.DecodeString(storedHex)
	if err != nil || len(computed) != sha256.Size || len(stored) != sha256.Size {
		return false
	}
	return subtle.ConstantTimeCompare(computed, stored) == 1
}

func installationExternalRef(orbitID int64, slot, tokenHash string) (string, error) {
	if orbitID <= 0 || len(slot) != 1 || slot[0] < 'a' || slot[0] > 'z' || !lowerHexTokenPattern.MatchString(tokenHash) {
		return "", errors.New("invalid slot binding identity")
	}
	fingerprintInput := "barycenter/slot-binding/v1:" + tokenHash
	fingerprint := hashToken(fingerprintInput)
	ref := fmt.Sprintf("%d:%s:%s", orbitID, slot, fingerprint)
	if !appExternalRefPattern.MatchString(ref) {
		return "", errors.New("invalid generated installation external_ref")
	}
	return ref, nil
}

// ProvisionInstallationSecrets is a persistence primitive for a separately
// authorized onboarding service. It resolves the transport proof again inside
// the write transaction, so a caller cannot manufacture capabilities by
// constructing an ActorContext. Node capability is explicitly insufficient:
// an active primary/companion control or verified Telegram identity must
// authorize the target installation in the same orbit. The method never stores
// supplied control or recovery secret plaintext.
func (s *Store) ProvisionInstallationSecrets(identity Identity, actorID int64, controlToken, recoveryID, recoverySecret string) error {
	if !s.selfServiceOnboarding {
		return ErrSelfServiceOnboardingDisabled
	}
	if !lowerHexTokenPattern.MatchString(controlToken) || !recoveryIDPattern.MatchString(recoveryID) {
		return errors.New("invalid installation credential format")
	}
	canonicalRecovery, err := normalizeHumanSecret(recoverySecret)
	if err != nil {
		return err
	}
	controlHash := hashToken(controlToken)
	recoveryHash := hashToken(canonicalRecovery)
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	authorizedBy, err := resolveActorContext(tx, identity)
	if err != nil {
		return err
	}
	if (!authorizedBy.Capabilities.Has(CapabilityControl) && !authorizedBy.Capabilities.Has(CapabilityTelegram)) ||
		authorizedBy.Role == "satellite" {
		return ErrInsufficientCapability
	}
	var currentRole, status string
	var revokedAt sql.NullInt64
	err = tx.QueryRow(`SELECT m.role, o.status, a.revoked_at
FROM memberships m
JOIN orbits o ON o.id = m.orbit_id
JOIN actors a ON a.id = m.actor_id
WHERE m.actor_id = ? AND m.orbit_id = ? AND m.left_at IS NULL`,
		authorizedBy.ActorID, authorizedBy.OrbitID).Scan(&currentRole, &status, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrInsufficientCapability
	}
	if err != nil {
		return err
	}
	if revokedAt.Valid {
		return ErrUnauthorized
	}
	if status != "active" || currentRole == "satellite" {
		return ErrInsufficientCapability
	}

	// Validate the target generation and lifecycle under the same immediate
	// write lock as the conditional update. Initial provisioning is not a
	// rotation or resurrection primitive.
	var targetKind, targetRole, targetStatus string
	var targetRevoked, existingConsumed sql.NullInt64
	var existingControl, existingRecoveryID, existingRecoveryHash sql.NullString
	err = tx.QueryRow(`SELECT a.kind, a.revoked_at, m.role, o.status,
       ic.control_token_hash, ic.recovery_id, ic.recovery_secret_hash,
       ic.consumed_at
FROM installation_credentials ic
JOIN actors a ON a.id = ic.actor_id
JOIN memberships m ON m.actor_id = ic.actor_id
  AND m.orbit_id = ic.slot_orbit_id AND m.left_at IS NULL
JOIN orbits o ON o.id = ic.slot_orbit_id
JOIN slots sl ON sl.orbit_id = ic.slot_orbit_id
  AND sl.slot = ic.slot_name
  AND sl.revoked_at IS NULL
  AND sl.token_hash = ic.binding_token_hash
  AND COALESCE(sl.paired_at, 0) = ic.slot_paired_at
WHERE ic.actor_id = ? AND ic.slot_orbit_id = ?`, actorID, authorizedBy.OrbitID).Scan(
		&targetKind, &targetRevoked, &targetRole, &targetStatus,
		&existingControl, &existingRecoveryID, &existingRecoveryHash, &existingConsumed)
	if errors.Is(err, sql.ErrNoRows) {
		return ErrUnauthorized
	}
	if err != nil {
		return err
	}
	if targetKind != "app_installation" || targetRevoked.Valid {
		return ErrUnauthorized
	}
	if targetStatus != "active" || targetRole == "satellite" {
		return ErrInsufficientCapability
	}
	if existingControl.Valid || existingRecoveryID.Valid || existingRecoveryHash.Valid || existingConsumed.Valid {
		return ErrCredentialAlreadyProvisioned
	}

	var domainMatches int
	if err := tx.QueryRow(`SELECT
  (SELECT COUNT(*) FROM slots WHERE token_hash = ?) +
  (SELECT COUNT(*) FROM installation_credentials WHERE control_token_hash = ?)`,
		controlHash, controlHash).Scan(&domainMatches); err != nil {
		return err
	}
	if domainMatches != 0 {
		return ErrCredentialDomainConflict
	}
	res, err := tx.Exec(`UPDATE installation_credentials
SET control_token_hash = ?, recovery_id = ?, recovery_secret_hash = ?, consumed_at = NULL
WHERE actor_id = ?
  AND slot_orbit_id = ?
  AND control_token_hash IS NULL
  AND recovery_id IS NULL
  AND recovery_secret_hash IS NULL
  AND consumed_at IS NULL
  AND EXISTS (
    SELECT 1 FROM slots sl
    WHERE sl.orbit_id = installation_credentials.slot_orbit_id
      AND sl.slot = installation_credentials.slot_name
      AND sl.revoked_at IS NULL
      AND sl.token_hash = installation_credentials.binding_token_hash
      AND COALESCE(sl.paired_at, 0) = installation_credentials.slot_paired_at
  )`, controlHash, recoveryID, recoveryHash, actorID, authorizedBy.OrbitID)
	if err != nil {
		return err
	}
	if n, err := res.RowsAffected(); err != nil {
		return err
	} else if n != 1 {
		return ErrCredentialAlreadyProvisioned
	}
	var orbitID int64
	if err := tx.QueryRow(`SELECT slot_orbit_id FROM installation_credentials WHERE actor_id = ?`, actorID).Scan(&orbitID); err != nil {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO audit_events(orbit_id, actor_id, type, created_at)
VALUES(?, ?, 'credential.provisioned', ?)`, orbitID, actorID, time.Now().UnixMilli()); err != nil {
		return err
	}
	return tx.Commit()
}
