package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"strings"
	"time"
)

var (
	// ErrCredentialInvalid is deliberately non-oracular. It covers every
	// secret-facing lookup, lifecycle, expiry, replay, and race-loser failure.
	ErrCredentialInvalid   = errors.New("credential is not valid")
	errRecoveryIDCollision = errors.New("generated recovery id collision")
	errDeviceCodeCollision = errors.New("generated device invite code collision")
	errLinkCodeCollision   = errors.New("generated link code collision")
	dummyCredentialHash    = hashToken(strings.Repeat("\x00", 64))
)

const onboardingSecretLength = 27

type OnboardingCredentials struct {
	OrbitID        int64
	OrbitTitle     string
	ActorID        int64
	Role           string
	Slot           string
	NodeToken      string
	ControlToken   string
	RecoveryID     string
	RecoverySecret string
}

type DeviceInviteResult struct {
	Code         string
	IntendedRole string
	ExpiresAt    time.Time
}

type RecoveryConsumeResult struct {
	OrbitID int64
	ActorID int64
	Role    string
}

type RecoveryRotationResult struct {
	ActorID        int64
	RecoveryID     string
	RecoverySecret string
}

type TelegramLinkResult struct {
	Code        string
	DesiredRole string
	ExpiresAt   time.Time
}

func randomHexSecret(bytes int) (string, error) {
	b := make([]byte, bytes)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func newRecoveryMaterial() (id, secret string, err error) {
	rawID, err := randomHexSecret(16)
	if err != nil {
		return "", "", err
	}
	secret, err = generateSecret(onboardingSecretLength)
	if err != nil {
		return "", "", err
	}
	return "rec_" + rawID, secret, nil
}

func newInstallationTokens() (node, control string, err error) {
	node, err = randomHexSecret(32)
	if err != nil {
		return "", "", err
	}
	for {
		control, err = randomHexSecret(32)
		if err != nil {
			return "", "", err
		}
		if control != node {
			return node, control, nil
		}
	}
}

// CreateSelfServiceOrbit creates the orbit, first app actor, primary
// membership, slot, both capability credentials, recovery generation, and
// audit event in one immediate writer transaction. Plaintext material exists
// only in this return value.
func (s *Store) CreateSelfServiceOrbit(title string) (OnboardingCredentials, error) {
	if !s.selfServiceOnboarding {
		return OnboardingCredentials{}, ErrSelfServiceOnboardingDisabled
	}
	title = strings.TrimSpace(title)
	if title == "" {
		return OnboardingCredentials{}, errors.New("orbit title is required")
	}
	for {
		node, control, err := newInstallationTokens()
		if err != nil {
			return OnboardingCredentials{}, err
		}
		recoveryID, recoverySecret, err := newRecoveryMaterial()
		if err != nil {
			return OnboardingCredentials{}, err
		}
		result, err := s.createSelfServiceOrbitWithMaterial(title, node, control, recoveryID, recoverySecret)
		if errors.Is(err, ErrCredentialDomainConflict) || errors.Is(err, errRecoveryIDCollision) {
			continue
		}
		return result, err
	}
}

func (s *Store) createSelfServiceOrbitWithMaterial(title, node, control, recoveryID, recoverySecret string) (OnboardingCredentials, error) {
	now := time.Now().UnixMilli()
	tx, err := s.db.Begin()
	if err != nil {
		return OnboardingCredentials{}, err
	}
	defer tx.Rollback()

	if err := ensureCredentialDigestsUnusedTx(tx, hashToken(node), hashToken(control)); err != nil {
		return OnboardingCredentials{}, err
	}
	var recoveryIDMatches int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM installation_credentials WHERE recovery_id = ?`, recoveryID).Scan(&recoveryIDMatches); err != nil {
		return OnboardingCredentials{}, err
	}
	if recoveryIDMatches != 0 {
		return OnboardingCredentials{}, errRecoveryIDCollision
	}
	res, err := tx.Exec(`INSERT INTO orbits(title, takeover_policy, voice_default, max_pulsars, max_members, created_at, status)
VALUES(?, 'user', 'personal', 5, 10, ?, 'active')`, title, now)
	if err != nil {
		return OnboardingCredentials{}, err
	}
	orbitID, err := res.LastInsertId()
	if err != nil {
		return OnboardingCredentials{}, err
	}
	const slot = "a"
	if _, err := tx.Exec(`INSERT INTO slots(orbit_id, slot, token_hash, paired_by, provider, paired_at, revoked_at)
VALUES(?, ?, ?, 0, 'spotify', ?, NULL)`, orbitID, slot, hashToken(node), now); err != nil {
		return OnboardingCredentials{}, err
	}
	externalRef, err := installationExternalRef(orbitID, slot, hashToken(node))
	if err != nil {
		return OnboardingCredentials{}, err
	}
	res, err = tx.Exec(`INSERT INTO actors(kind, display_name, external_ref, created_at, revoked_at)
VALUES('app_installation', ?, ?, ?, NULL)`, slot, externalRef, now)
	if err != nil {
		return OnboardingCredentials{}, err
	}
	actorID, err := res.LastInsertId()
	if err != nil {
		return OnboardingCredentials{}, err
	}
	if _, err := tx.Exec(`INSERT INTO memberships(orbit_id, actor_id, role, joined_at, left_at)
VALUES(?, ?, 'primary', ?, NULL)`, orbitID, actorID, now); err != nil {
		return OnboardingCredentials{}, err
	}
	if _, err := tx.Exec(`INSERT INTO installation_credentials
  (actor_id, slot_orbit_id, slot_name, slot_paired_at, binding_token_hash,
   control_token_hash, recovery_id, recovery_secret_hash, consumed_at, created_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, NULL, ?)`, actorID, orbitID, slot, now,
		hashToken(node), hashToken(control), recoveryID, hashToken(recoverySecret), now); err != nil {
		return OnboardingCredentials{}, err
	}
	if _, err := tx.Exec(`INSERT INTO audit_events(orbit_id, actor_id, type, created_at)
VALUES(?, ?, 'onboarding.orbit_created', ?)`, orbitID, actorID, now); err != nil {
		return OnboardingCredentials{}, err
	}
	if err := s.checkpoint("onboarding_create_before_commit"); err != nil {
		return OnboardingCredentials{}, err
	}
	if err := tx.Commit(); err != nil {
		return OnboardingCredentials{}, err
	}
	return OnboardingCredentials{
		OrbitID: orbitID, OrbitTitle: title, ActorID: actorID, Role: "primary", Slot: slot,
		NodeToken: node, ControlToken: control, RecoveryID: recoveryID, RecoverySecret: recoverySecret,
	}, nil
}

func ensureCredentialDigestsUnusedTx(tx *sql.Tx, digests ...string) error {
	for _, digest := range digests {
		var matches int
		if err := tx.QueryRow(`SELECT
  (SELECT COUNT(*) FROM slots WHERE token_hash = ?) +
  (SELECT COUNT(*) FROM installation_credentials WHERE control_token_hash = ?)`, digest, digest).Scan(&matches); err != nil {
			return err
		}
		if matches != 0 {
			return ErrCredentialDomainConflict
		}
	}
	return nil
}

// mutationActorContextTx re-resolves the digest of the presented bearer token
// inside the writer transaction. The entrypoint computes this digest before
// material generation and writer acquisition; only the fixed digest crosses
// the transaction boundary.
func mutationActorContextTx(tx *sql.Tx, expectedActorID int64, presentedHash string) (ActorContext, error) {
	var orbitID int64
	var slot, controlHash, bindingHash string
	var revokedAt sql.NullInt64
	err := tx.QueryRow(`SELECT ic.slot_orbit_id, ic.slot_name, ic.control_token_hash,
       ic.binding_token_hash, a.revoked_at
FROM installation_credentials ic
JOIN actors a ON a.id = ic.actor_id
JOIN slots sl ON sl.orbit_id = ic.slot_orbit_id
  AND sl.slot = ic.slot_name
  AND sl.revoked_at IS NULL
  AND sl.token_hash = ic.binding_token_hash
  AND COALESCE(sl.paired_at, 0) = ic.slot_paired_at
WHERE ic.actor_id = ? AND ic.control_token_hash IS NOT NULL`, expectedActorID).Scan(
		&orbitID, &slot, &controlHash, &bindingHash, &revokedAt)
	if errors.Is(err, sql.ErrNoRows) || revokedAt.Valid {
		return ActorContext{}, ErrUnauthorized
	}
	if err != nil {
		return ActorContext{}, err
	}
	controlMatches := constantTimeDigestEqual(presentedHash, controlHash)
	nodeMatches := constantTimeDigestEqual(presentedHash, bindingHash)
	if !controlMatches {
		if nodeMatches {
			return ActorContext{OrbitID: orbitID, ActorID: expectedActorID, Slot: slot, Capabilities: CapabilityNode}, ErrInsufficientCapability
		}
		return ActorContext{}, ErrUnauthorized
	}
	ctx, err := resolveActiveMembership(tx, expectedActorID, orbitID, slot, CapabilityNode|CapabilityControl)
	if err != nil {
		return ctx, err
	}
	if ctx.Role == "satellite" {
		ctx.Role = ""
		return ctx, ErrInsufficientCapability
	}
	return ctx, nil
}

func (s *Store) IssueDeviceInvite(actorID int64, bearer, intendedRole string) (DeviceInviteResult, error) {
	if !s.selfServiceOnboarding {
		return DeviceInviteResult{}, ErrSelfServiceOnboardingDisabled
	}
	if intendedRole != "companion" && intendedRole != "satellite" {
		return DeviceInviteResult{}, errors.New("invalid intended role")
	}
	presentedHash := hashToken(bearer)
	if err := s.checkpoint("device_invite_bearer_prepared"); err != nil {
		return DeviceInviteResult{}, err
	}
	for {
		code, err := generateSecret(onboardingSecretLength)
		if err != nil {
			return DeviceInviteResult{}, err
		}
		codeHash := hashToken(code)
		if err := s.checkpoint("device_invite_material_prepared"); err != nil {
			return DeviceInviteResult{}, err
		}
		result, err := s.issueDeviceInviteWithHash(actorID, presentedHash, intendedRole, codeHash)
		if errors.Is(err, errDeviceCodeCollision) {
			continue
		}
		if err == nil {
			result.Code = code
		}
		return result, err
	}
}

func (s *Store) issueDeviceInviteWithHash(actorID int64, presentedHash, intendedRole, codeHash string) (DeviceInviteResult, error) {
	now := time.Now()
	expires := now.Add(15 * time.Minute)
	if err := s.checkpoint("device_invite_issue_before_begin"); err != nil {
		return DeviceInviteResult{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return DeviceInviteResult{}, err
	}
	defer tx.Rollback()
	if err := s.checkpoint("device_invite_transaction_started"); err != nil {
		return DeviceInviteResult{}, err
	}
	ctx, err := mutationActorContextTx(tx, actorID, presentedHash)
	if err != nil {
		return DeviceInviteResult{}, err
	}
	var codeMatches int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM device_invites WHERE code_hash = ?`, codeHash).Scan(&codeMatches); err != nil {
		return DeviceInviteResult{}, err
	}
	if codeMatches != 0 {
		return DeviceInviteResult{}, errDeviceCodeCollision
	}
	if _, err := tx.Exec(`INSERT INTO device_invites
  (code_hash, orbit_id, issued_by_actor_id, intended_role, expires_at, consumed_at, created_at)
VALUES(?, ?, ?, ?, ?, NULL, ?)`, codeHash, ctx.OrbitID, ctx.ActorID,
		intendedRole, expires.UnixMilli(), now.UnixMilli()); err != nil {
		return DeviceInviteResult{}, err
	}
	if err := s.checkpoint("device_invite_issue_before_audit"); err != nil {
		return DeviceInviteResult{}, err
	}
	if _, err := tx.Exec(`INSERT INTO audit_events(orbit_id, actor_id, type, created_at)
VALUES(?, ?, 'device_invite.issued', ?)`, ctx.OrbitID, ctx.ActorID, now.UnixMilli()); err != nil {
		return DeviceInviteResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return DeviceInviteResult{}, err
	}
	return DeviceInviteResult{IntendedRole: intendedRole, ExpiresAt: expires}, nil
}

func (s *Store) ConsumeDeviceInvite(code string) (OnboardingCredentials, error) {
	return s.consumeDeviceInviteAt(code, time.Now().UnixMilli())
}

func (s *Store) consumeDeviceInviteAt(code string, now int64) (OnboardingCredentials, error) {
	if !s.selfServiceOnboarding {
		return OnboardingCredentials{}, ErrSelfServiceOnboardingDisabled
	}
	if len(code) > 40 {
		return OnboardingCredentials{}, ErrCredentialInvalid
	}
	canonical, err := normalizeHumanSecret(code)
	if err != nil {
		return OnboardingCredentials{}, ErrCredentialInvalid
	}
	codeHash := hashToken(canonical)
	for {
		node, control, err := newInstallationTokens()
		if err != nil {
			return OnboardingCredentials{}, err
		}
		nodeHash, controlHash := hashToken(node), hashToken(control)
		result, err := s.consumeDeviceInviteWithDigestsAt(codeHash, nodeHash, controlHash, now)
		if errors.Is(err, ErrCredentialDomainConflict) {
			continue
		}
		if err == nil {
			result.NodeToken = node
			result.ControlToken = control
		}
		return result, err
	}
}

func (s *Store) consumeDeviceInviteWithDigestsAt(codeHash, nodeHash, newControlHash string, now int64) (OnboardingCredentials, error) {
	if err := s.checkpoint("device_invite_consume_before_begin"); err != nil {
		return OnboardingCredentials{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return OnboardingCredentials{}, err
	}
	defer tx.Rollback()
	var exists int
	var storedHash string
	var orbitID, issuerID, expiresAt, consumedAt sql.NullInt64
	var intendedRole, title, orbitStatus, actorKind sql.NullString
	var maxPulsars sql.NullInt64
	var occupiedSlots int64
	var actorRevoked, membershipLeft sql.NullInt64
	var membershipRole sql.NullString
	var credentialActorID, credentialPairedAt sql.NullInt64
	var controlHash, bindingHash, slotHash, slotName sql.NullString
	var slotPairedAt int64
	var slotRevoked sql.NullInt64
	err = tx.QueryRow(`SELECT
  di.code_hash IS NOT NULL,
  COALESCE(di.code_hash, ?),
  di.orbit_id, di.issued_by_actor_id, di.intended_role,
  di.expires_at, di.consumed_at,
  o.title, o.status, o.max_pulsars,
  (SELECT COUNT(*) FROM slots occupied
   WHERE occupied.orbit_id = di.orbit_id AND occupied.revoked_at IS NULL),
  a.kind, a.revoked_at,
  m.role, m.left_at,
  ic.actor_id, ic.control_token_hash, ic.binding_token_hash, ic.slot_paired_at,
  sl.token_hash, COALESCE(sl.paired_at, 0), sl.revoked_at, sl.slot
FROM (SELECT 1) anchor
LEFT JOIN device_invites di ON di.code_hash = ?
LEFT JOIN orbits o ON o.id = di.orbit_id
LEFT JOIN actors a ON a.id = di.issued_by_actor_id
LEFT JOIN memberships m ON m.actor_id = di.issued_by_actor_id AND m.orbit_id = di.orbit_id
LEFT JOIN installation_credentials ic ON ic.actor_id = di.issued_by_actor_id AND ic.slot_orbit_id = di.orbit_id
LEFT JOIN slots sl ON sl.orbit_id = ic.slot_orbit_id AND sl.slot = ic.slot_name`,
		dummyCredentialHash, codeHash).Scan(
		&exists, &storedHash, &orbitID, &issuerID, &intendedRole,
		&expiresAt, &consumedAt, &title, &orbitStatus, &maxPulsars, &occupiedSlots, &actorKind, &actorRevoked,
		&membershipRole, &membershipLeft, &credentialActorID, &controlHash,
		&bindingHash, &credentialPairedAt, &slotHash, &slotPairedAt, &slotRevoked, &slotName)
	if err != nil {
		return OnboardingCredentials{}, err
	}
	if err := s.checkpoint("device_invite_validation_query"); err != nil {
		return OnboardingCredentials{}, err
	}
	hashValid := constantTimeDigestEqual(codeHash, storedHash)
	if err := s.checkpoint("device_invite_validation_hash"); err != nil {
		return OnboardingCredentials{}, err
	}
	issuerRoleValid := membershipRole.Valid &&
		(membershipRole.String == "primary" || membershipRole.String == "companion")
	capacity := int64(0)
	if maxPulsars.Valid {
		capacity = maxPulsars.Int64
		if capacity > 26 {
			capacity = 26
		}
	}
	valid := exists == 1 && hashValid && orbitID.Valid && issuerID.Valid &&
		intendedRole.Valid && (intendedRole.String == "companion" || intendedRole.String == "satellite") &&
		expiresAt.Valid && expiresAt.Int64 > now && !consumedAt.Valid &&
		title.Valid && orbitStatus.Valid && orbitStatus.String == "active" && capacity > 0 && occupiedSlots < capacity &&
		actorKind.Valid && actorKind.String == "app_installation" && !actorRevoked.Valid &&
		issuerRoleValid && !membershipLeft.Valid &&
		credentialActorID.Valid && credentialActorID.Int64 == issuerID.Int64 && controlHash.Valid &&
		bindingHash.Valid && slotHash.Valid && bindingHash.String == slotHash.String &&
		credentialPairedAt.Valid && credentialPairedAt.Int64 == slotPairedAt &&
		slotName.Valid && !slotRevoked.Valid
	if !valid {
		return OnboardingCredentials{}, ErrCredentialInvalid
	}
	if err := ensureCredentialDigestsUnusedTx(tx, nodeHash, newControlHash); err != nil {
		return OnboardingCredentials{}, err
	}
	res, err := tx.Exec(`UPDATE device_invites SET consumed_at = ?
WHERE code_hash = ? AND consumed_at IS NULL AND expires_at > ?`, now, codeHash, now)
	if err != nil {
		return OnboardingCredentials{}, err
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		if err != nil {
			return OnboardingCredentials{}, err
		}
		return OnboardingCredentials{}, ErrCredentialInvalid
	}
	if err := s.checkpoint("device_invite_after_reserve"); err != nil {
		return OnboardingCredentials{}, err
	}
	slot, err := nextInstallationSlotTx(tx, orbitID.Int64)
	if err != nil {
		if errors.Is(err, ErrLimit) {
			return OnboardingCredentials{}, ErrCredentialInvalid
		}
		return OnboardingCredentials{}, err
	}
	var staleActorID int64
	err = tx.QueryRow(`SELECT actor_id FROM installation_credentials
WHERE slot_orbit_id = ? AND slot_name = ?`, orbitID.Int64, slot).Scan(&staleActorID)
	if err == nil {
		if err := retireInstallationTx(tx, staleActorID, now); err != nil {
			return OnboardingCredentials{}, err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return OnboardingCredentials{}, err
	}
	if _, err := tx.Exec(`INSERT INTO slots(orbit_id, slot, token_hash, paired_by, provider, paired_at, revoked_at)
VALUES(?, ?, ?, 0, 'spotify', ?, NULL)
ON CONFLICT(orbit_id, slot) DO UPDATE SET
  token_hash = excluded.token_hash,
  paired_by = 0,
  provider = excluded.provider,
  paired_at = excluded.paired_at,
  revoked_at = NULL`, orbitID.Int64, slot, nodeHash, now); err != nil {
		return OnboardingCredentials{}, err
	}
	externalRef, err := installationExternalRef(orbitID.Int64, slot, nodeHash)
	if err != nil {
		return OnboardingCredentials{}, err
	}
	res, err = tx.Exec(`INSERT INTO actors(kind, display_name, external_ref, created_at, revoked_at)
VALUES('app_installation', ?, ?, ?, NULL)`, slot, externalRef, now)
	if err != nil {
		return OnboardingCredentials{}, err
	}
	actorID, err := res.LastInsertId()
	if err != nil {
		return OnboardingCredentials{}, err
	}
	if _, err := tx.Exec(`INSERT INTO memberships(orbit_id, actor_id, role, joined_at, left_at)
VALUES(?, ?, ?, ?, NULL)`, orbitID.Int64, actorID, intendedRole.String, now); err != nil {
		return OnboardingCredentials{}, err
	}
	if _, err := tx.Exec(`INSERT INTO installation_credentials
  (actor_id, slot_orbit_id, slot_name, slot_paired_at, binding_token_hash,
   control_token_hash, recovery_id, recovery_secret_hash, consumed_at, created_at)
VALUES(?, ?, ?, ?, ?, ?, NULL, NULL, NULL, ?)`, actorID, orbitID.Int64, slot, now,
		nodeHash, newControlHash, now); err != nil {
		return OnboardingCredentials{}, err
	}
	if err := s.checkpoint("device_invite_consume_before_audit"); err != nil {
		return OnboardingCredentials{}, err
	}
	if _, err := tx.Exec(`INSERT INTO audit_events(orbit_id, actor_id, type, created_at)
VALUES(?, ?, 'device_invite.consumed', ?)`, orbitID.Int64, actorID, now); err != nil {
		return OnboardingCredentials{}, err
	}
	if err := tx.Commit(); err != nil {
		return OnboardingCredentials{}, err
	}
	return OnboardingCredentials{OrbitID: orbitID.Int64, OrbitTitle: title.String, ActorID: actorID,
		Role: intendedRole.String, Slot: slot}, nil
}

func nextInstallationSlotTx(tx *sql.Tx, orbitID int64) (string, error) {
	var max int
	if err := tx.QueryRow(`SELECT max_pulsars FROM orbits WHERE id = ? AND status = 'active'`, orbitID).Scan(&max); err != nil {
		return "", err
	}
	if max > 26 {
		max = 26
	}
	for i := 0; i < max; i++ {
		slot := string(rune('a' + i))
		var exists int
		if err := tx.QueryRow(`SELECT COUNT(*) FROM slots
WHERE orbit_id = ? AND slot = ? AND revoked_at IS NULL`, orbitID, slot).Scan(&exists); err != nil {
			return "", err
		}
		if exists == 0 {
			return slot, nil
		}
	}
	return "", ErrLimit
}

func (s *Store) ConsumeRecovery(recoveryID, recoverySecret, replacementControl string) (RecoveryConsumeResult, error) {
	return s.consumeRecoveryAt(recoveryID, recoverySecret, replacementControl, time.Now().UnixMilli())
}

func (s *Store) consumeRecoveryAt(recoveryID, recoverySecret, replacementControl string, now int64) (RecoveryConsumeResult, error) {
	if !s.selfServiceOnboarding {
		return RecoveryConsumeResult{}, ErrSelfServiceOnboardingDisabled
	}
	if len(recoverySecret) > 40 {
		return RecoveryConsumeResult{}, ErrCredentialInvalid
	}
	canonical, err := normalizeHumanSecret(recoverySecret)
	if err != nil || !recoveryIDPattern.MatchString(recoveryID) || !lowerHexTokenPattern.MatchString(replacementControl) {
		return RecoveryConsumeResult{}, ErrCredentialInvalid
	}
	if err := s.checkpoint("recovery_consume_before_begin"); err != nil {
		return RecoveryConsumeResult{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return RecoveryConsumeResult{}, err
	}
	defer tx.Rollback()
	var exists int
	var actorID, orbitID sql.NullInt64
	var role, status, actorKind, storedSecretHash, currentControl sql.NullString
	var bindingHash, slotHash, slotName sql.NullString
	var consumedAt, actorRevoked, membershipLeft, slotRevoked sql.NullInt64
	var credentialPairedAt sql.NullInt64
	var slotPairedAt int64
	err = tx.QueryRow(`SELECT
  ic.recovery_id IS NOT NULL,
  ic.actor_id, ic.slot_orbit_id, m.role, o.status, a.kind,
  ic.recovery_secret_hash, ic.control_token_hash, ic.consumed_at,
  a.revoked_at, m.left_at, sl.revoked_at, ic.binding_token_hash,
  sl.token_hash, sl.slot, ic.slot_paired_at, COALESCE(sl.paired_at, 0)
FROM (SELECT 1) anchor
LEFT JOIN installation_credentials ic ON ic.recovery_id = ?
LEFT JOIN actors a ON a.id = ic.actor_id
LEFT JOIN orbits o ON o.id = ic.slot_orbit_id
LEFT JOIN memberships m ON m.actor_id = ic.actor_id AND m.orbit_id = ic.slot_orbit_id
LEFT JOIN slots sl ON sl.orbit_id = ic.slot_orbit_id AND sl.slot = ic.slot_name
`, recoveryID).Scan(&exists, &actorID, &orbitID, &role, &status, &actorKind,
		&storedSecretHash, &currentControl, &consumedAt, &actorRevoked, &membershipLeft,
		&slotRevoked, &bindingHash, &slotHash, &slotName, &credentialPairedAt, &slotPairedAt)
	if err != nil {
		return RecoveryConsumeResult{}, err
	}
	if err := s.checkpoint("recovery_validation_query"); err != nil {
		return RecoveryConsumeResult{}, err
	}
	roleValid := role.Valid && (role.String == "primary" || role.String == "companion" || role.String == "satellite")
	validLifecycle := exists == 1 && actorID.Valid && orbitID.Valid && roleValid &&
		status.Valid && status.String == "active" && actorKind.Valid && actorKind.String == "app_installation" &&
		storedSecretHash.Valid && currentControl.Valid && !actorRevoked.Valid && !membershipLeft.Valid && !slotRevoked.Valid &&
		bindingHash.Valid && slotHash.Valid && bindingHash.String == slotHash.String && slotName.Valid &&
		credentialPairedAt.Valid && credentialPairedAt.Int64 == slotPairedAt
	comparisonTarget := dummyCredentialHash
	if validLifecycle {
		comparisonTarget = storedSecretHash.String
		if err := s.checkpoint("recovery_validation_real_target"); err != nil {
			return RecoveryConsumeResult{}, err
		}
	} else if err := s.checkpoint("recovery_validation_dummy_target"); err != nil {
		return RecoveryConsumeResult{}, err
	}
	validSecret := constantTimeHashEqual(canonical, comparisonTarget)
	if err := s.checkpoint("recovery_validation_hash"); err != nil {
		return RecoveryConsumeResult{}, err
	}
	if !validSecret || !validLifecycle {
		return RecoveryConsumeResult{}, ErrCredentialInvalid
	}
	replacementHash := hashToken(replacementControl)
	if consumedAt.Valid {
		if !constantTimeDigestEqual(replacementHash, currentControl.String) {
			return RecoveryConsumeResult{}, ErrCredentialInvalid
		}
		if err := tx.Commit(); err != nil {
			return RecoveryConsumeResult{}, err
		}
		return RecoveryConsumeResult{OrbitID: orbitID.Int64, ActorID: actorID.Int64, Role: role.String}, nil
	}
	if constantTimeHashEqual(replacementControl, currentControl.String) {
		return RecoveryConsumeResult{}, ErrCredentialInvalid
	}
	if err := ensureCredentialDigestsUnusedTx(tx, replacementHash); err != nil {
		if errors.Is(err, ErrCredentialDomainConflict) {
			return RecoveryConsumeResult{}, ErrCredentialInvalid
		}
		return RecoveryConsumeResult{}, err
	}
	res, err := tx.Exec(`UPDATE installation_credentials
SET control_token_hash = ?, consumed_at = ?
WHERE actor_id = ? AND recovery_id = ? AND recovery_secret_hash = ?
  AND consumed_at IS NULL AND control_token_hash = ?
	  AND EXISTS (SELECT 1 FROM slots sl WHERE sl.orbit_id = installation_credentials.slot_orbit_id
    AND sl.slot = installation_credentials.slot_name AND sl.revoked_at IS NULL
    AND sl.token_hash = installation_credentials.binding_token_hash
    AND COALESCE(sl.paired_at, 0) = installation_credentials.slot_paired_at)`, replacementHash, now,
		actorID.Int64, recoveryID, storedSecretHash.String, currentControl.String)
	if err != nil {
		return RecoveryConsumeResult{}, err
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		if err != nil {
			return RecoveryConsumeResult{}, err
		}
		return RecoveryConsumeResult{}, ErrCredentialInvalid
	}
	if err := s.checkpoint("recovery_after_consume"); err != nil {
		return RecoveryConsumeResult{}, err
	}
	if _, err := tx.Exec(`INSERT INTO audit_events(orbit_id, actor_id, type, created_at)
VALUES(?, ?, 'recovery.consumed', ?)`, orbitID.Int64, actorID.Int64, now); err != nil {
		return RecoveryConsumeResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return RecoveryConsumeResult{}, err
	}
	return RecoveryConsumeResult{OrbitID: orbitID.Int64, ActorID: actorID.Int64, Role: role.String}, nil
}

func (s *Store) RotateRecovery(actorID int64, bearer string) (RecoveryRotationResult, error) {
	return s.rotateRecoveryWithGenerator(actorID, bearer, newRecoveryMaterial)
}

func (s *Store) rotateRecoveryWithGenerator(actorID int64, bearer string, generate func() (string, string, error)) (RecoveryRotationResult, error) {
	if !s.selfServiceOnboarding {
		return RecoveryRotationResult{}, ErrSelfServiceOnboardingDisabled
	}
	presentedHash := hashToken(bearer)
	if err := s.checkpoint("recovery_rotate_bearer_prepared"); err != nil {
		return RecoveryRotationResult{}, err
	}
	for {
		recoveryID, recoverySecret, err := generate()
		if err != nil {
			return RecoveryRotationResult{}, err
		}
		recoverySecretHash := hashToken(recoverySecret)
		if err := s.checkpoint("recovery_rotate_material_prepared"); err != nil {
			return RecoveryRotationResult{}, err
		}
		result, err := s.rotateRecoveryWithHashes(actorID, presentedHash, recoveryID, recoverySecretHash)
		if errors.Is(err, errRecoveryIDCollision) {
			continue
		}
		if err == nil {
			result.RecoverySecret = recoverySecret
		}
		return result, err
	}
}

func (s *Store) rotateRecoveryWithHashes(actorID int64, presentedHash, recoveryID, recoverySecretHash string) (RecoveryRotationResult, error) {
	now := time.Now().UnixMilli()
	if err := s.checkpoint("recovery_rotate_before_begin"); err != nil {
		return RecoveryRotationResult{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return RecoveryRotationResult{}, err
	}
	defer tx.Rollback()
	if err := s.checkpoint("recovery_rotate_transaction_started"); err != nil {
		return RecoveryRotationResult{}, err
	}
	ctx, err := mutationActorContextTx(tx, actorID, presentedHash)
	if err != nil {
		return RecoveryRotationResult{}, err
	}
	if err := s.checkpoint("recovery_rotate_after_auth"); err != nil {
		return RecoveryRotationResult{}, err
	}
	var oldRecoveryID sql.NullString
	if err := tx.QueryRow(`SELECT recovery_id FROM installation_credentials WHERE actor_id = ?`, ctx.ActorID).Scan(&oldRecoveryID); err != nil {
		return RecoveryRotationResult{}, err
	}
	var recoveryIDMatches int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM installation_credentials WHERE recovery_id = ?`, recoveryID).Scan(&recoveryIDMatches); err != nil {
		return RecoveryRotationResult{}, err
	}
	if recoveryIDMatches != 0 {
		return RecoveryRotationResult{}, errRecoveryIDCollision
	}
	res, err := tx.Exec(`UPDATE installation_credentials
SET recovery_id = ?, recovery_secret_hash = ?, consumed_at = NULL
WHERE actor_id = ?`, recoveryID, recoverySecretHash, ctx.ActorID)
	if err != nil {
		return RecoveryRotationResult{}, err
	}
	if n, err := res.RowsAffected(); err != nil || n != 1 {
		if err != nil {
			return RecoveryRotationResult{}, err
		}
		return RecoveryRotationResult{}, ErrUnauthorized
	}
	if err := s.checkpoint("recovery_rotate_before_audit"); err != nil {
		return RecoveryRotationResult{}, err
	}
	if err := insertRecoveryRotationAuditTx(tx, ctx.OrbitID, ctx.ActorID, oldRecoveryID, recoveryID, now); err != nil {
		return RecoveryRotationResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return RecoveryRotationResult{}, err
	}
	return RecoveryRotationResult{ActorID: ctx.ActorID, RecoveryID: recoveryID}, nil
}

func (s *Store) IssueTelegramLink(actorID int64, bearer, desiredRole string) (TelegramLinkResult, error) {
	if !s.selfServiceOnboarding {
		return TelegramLinkResult{}, ErrSelfServiceOnboardingDisabled
	}
	if desiredRole == "" {
		desiredRole = "companion"
	}
	if desiredRole != "companion" && desiredRole != "satellite" {
		return TelegramLinkResult{}, errors.New("invalid desired role")
	}
	presentedHash := hashToken(bearer)
	if err := s.checkpoint("telegram_link_bearer_prepared"); err != nil {
		return TelegramLinkResult{}, err
	}
	for {
		code, err := generateSecret(onboardingSecretLength)
		if err != nil {
			return TelegramLinkResult{}, err
		}
		codeHash := hashToken(code)
		if err := s.checkpoint("telegram_link_material_prepared"); err != nil {
			return TelegramLinkResult{}, err
		}
		result, err := s.issueTelegramLinkWithHash(actorID, presentedHash, desiredRole, codeHash)
		if errors.Is(err, errLinkCodeCollision) {
			continue
		}
		if err == nil {
			result.Code = code
		}
		return result, err
	}
}

func (s *Store) issueTelegramLinkWithHash(actorID int64, presentedHash, desiredRole, codeHash string) (TelegramLinkResult, error) {
	now := time.Now()
	expires := now.Add(15 * time.Minute)
	if err := s.checkpoint("telegram_link_issue_before_begin"); err != nil {
		return TelegramLinkResult{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return TelegramLinkResult{}, err
	}
	defer tx.Rollback()
	if err := s.checkpoint("telegram_link_transaction_started"); err != nil {
		return TelegramLinkResult{}, err
	}
	ctx, err := mutationActorContextTx(tx, actorID, presentedHash)
	if err != nil {
		return TelegramLinkResult{}, err
	}
	var codeMatches int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM telegram_link_codes WHERE code_hash = ?`, codeHash).Scan(&codeMatches); err != nil {
		return TelegramLinkResult{}, err
	}
	if codeMatches != 0 {
		return TelegramLinkResult{}, errLinkCodeCollision
	}
	if _, err := tx.Exec(`UPDATE telegram_link_codes SET invalidated_at = ?
WHERE issuer_actor_id = ? AND consumed_at IS NULL AND invalidated_at IS NULL`, now.UnixMilli(), ctx.ActorID); err != nil {
		return TelegramLinkResult{}, err
	}
	if _, err := tx.Exec(`INSERT INTO telegram_link_codes
  (code_hash, issuer_actor_id, orbit_id, desired_role, expires_at, invalidated_at, consumed_at, consuming_actor_id, created_at)
VALUES(?, ?, ?, ?, ?, NULL, NULL, NULL, ?)`, codeHash, ctx.ActorID, ctx.OrbitID,
		desiredRole, expires.UnixMilli(), now.UnixMilli()); err != nil {
		return TelegramLinkResult{}, err
	}
	if err := s.checkpoint("telegram_link_issue_before_audit"); err != nil {
		return TelegramLinkResult{}, err
	}
	if _, err := tx.Exec(`INSERT INTO audit_events(orbit_id, actor_id, type, created_at)
VALUES(?, ?, 'telegram_link.issued', ?)`, ctx.OrbitID, ctx.ActorID, now.UnixMilli()); err != nil {
		return TelegramLinkResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return TelegramLinkResult{}, err
	}
	return TelegramLinkResult{DesiredRole: desiredRole, ExpiresAt: expires}, nil
}
