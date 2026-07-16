package store

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"relux.works/duet/coordinator/internal/ulid"
)

var (
	ErrAirInvalid              = errors.New("invalid Air request")
	ErrAirForbidden            = errors.New("Air operation forbidden")
	ErrAirDissolved            = errors.New("Air dissolved")
	ErrAirAlreadyMember        = errors.New("Air membership already exists")
	ErrAirMembershipNotFound   = errors.New("Air membership not found")
	ErrAirInviteUnavailable    = errors.New("Air invite unavailable")
	ErrAirIdempotencyConflict  = errors.New("Air idempotency conflict")
	ErrAirConfirmationRequired = errors.New("Air membership confirmation required")
	ErrAirPolicyDenied         = errors.New("Air policy denied")
	ErrAirRateLimited          = errors.New("Air operation rate limited")
)

const (
	AirBarycenterCapacity = 8
	AirPulsarCapacity     = 20
	AirInviteTTL          = 15 * time.Minute
)

// AirMutationAuth binds a mutation to the actor resolved by the HTTP
// middleware. The same credential is re-resolved after the SQLite writer lock
// is acquired, closing the revoke/role-change race.
type AirMutationAuth struct {
	ExpectedActorID int64
	Bearer          string
	// Identity lets non-HTTP adapters use the same transactional Air service.
	// Bearer remains for source compatibility and is normalized to a bearer
	// identity when Identity is empty.
	Identity           Identity
	IdempotencyKeyHash string
	RequestHash        string
	Now                int64
}

type AirCapacityView struct {
	Barycenters   int `json:"barycenters"`
	OnlinePulsars int `json:"online_pulsars"`
}

type AirPolicyView struct {
	Revision int64  `json:"revision"`
	Invite   string `json:"invite"`
	Overlay  string `json:"overlay"`
	Queue    string `json:"queue"`
	Replace  string `json:"replace"`
}

type AirProjection struct {
	AirID              string          `json:"air_id"`
	Title              string          `json:"title"`
	Status             string          `json:"status"`
	Revision           int64           `json:"revision"`
	MembershipID       string          `json:"membership_id"`
	MembershipStatus   string          `json:"membership_status"`
	MembershipRevision int64           `json:"membership_revision"`
	AirRole            string          `json:"air_role"`
	MemberCount        int             `json:"member_count"`
	ActiveMemberCount  int             `json:"active_member_count"`
	OnlinePulsarCount  int             `json:"online_pulsar_count"`
	Capacity           AirCapacityView `json:"capacity"`
	Policy             AirPolicyView   `json:"policy"`
	IsCurrent          bool            `json:"is_current"`
}

type AirListView struct {
	CurrentAirID          string          `json:"current_air_id,omitempty"`
	ActivePointerRevision int64           `json:"active_pointer_revision"`
	Saved                 []AirProjection `json:"saved"`
}

type AirInviteIssueResult struct {
	InviteID  string `json:"invite_id"`
	Revision  int64  `json:"revision"`
	ExpiresAt int64  `json:"expires_at"`
	Code      string `json:"code,omitempty"`
}

type AirJoinPreview struct {
	AirID                 string          `json:"air_id"`
	Title                 string          `json:"title"`
	OwnerDisplayName      string          `json:"owner_display_name"`
	IntendedRole          string          `json:"air_role"`
	MembershipID          string          `json:"membership_id"`
	MembershipRevision    int64           `json:"membership_revision"`
	Policy                AirPolicyView   `json:"policy"`
	MemberCount           int             `json:"member_count"`
	Capacity              AirCapacityView `json:"capacity"`
	ActivationWouldSwitch bool            `json:"activation_would_switch"`
}

type AirLifecycleResult struct {
	AirID         string         `json:"air_id"`
	Status        string         `json:"status"`
	Projection    *AirProjection `json:"air,omitempty"`
	PreviousAirID string         `json:"previous_air_id,omitempty"`
}

type airMutationState struct {
	tx       *sql.Tx
	ctx      ActorContext
	replayed bool
	raw      []byte
}

func normalizeAirMutationAuth(auth AirMutationAuth) (AirMutationAuth, error) {
	if auth.Now <= 0 {
		auth.Now = time.Now().UnixMilli()
	}
	if auth.Identity.Kind == "" && auth.Bearer != "" {
		auth.Identity = Identity{Kind: IdentityBearer, Token: auth.Bearer}
	}
	if auth.ExpectedActorID <= 0 ||
		(auth.Identity.Kind != IdentityBearer && auth.Identity.Kind != IdentityTelegram) ||
		len(auth.IdempotencyKeyHash) != 64 ||
		len(auth.RequestHash) != 64 || !lowerHexTokenPattern.MatchString(auth.IdempotencyKeyHash) ||
		!lowerHexTokenPattern.MatchString(auth.RequestHash) {
		return AirMutationAuth{}, ErrAirInvalid
	}
	return auth, nil
}

func (s *Store) beginAirMutation(auth AirMutationAuth, operation string) (airMutationState, AirMutationAuth, error) {
	auth, err := normalizeAirMutationAuth(auth)
	if err != nil {
		return airMutationState{}, auth, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return airMutationState{}, auth, err
	}
	if _, err := requireAirsAuthoritativeTx(tx); err != nil {
		tx.Rollback()
		return airMutationState{}, auth, err
	}
	ctx, err := resolveActorContext(tx, auth.Identity)
	if err != nil {
		tx.Rollback()
		return airMutationState{}, auth, err
	}
	if ctx.ActorID != auth.ExpectedActorID {
		tx.Rollback()
		return airMutationState{}, auth, ErrUnauthorized
	}
	var storedOperation, storedRequest string
	var raw string
	err = tx.QueryRow(`SELECT operation, request_hash, response_json
FROM air_mutation_results WHERE actor_id = ? AND idempotency_key_hash = ?`,
		ctx.ActorID, auth.IdempotencyKeyHash).Scan(&storedOperation, &storedRequest, &raw)
	if err == nil {
		if storedOperation != operation || storedRequest != auth.RequestHash {
			tx.Rollback()
			return airMutationState{}, auth, ErrAirIdempotencyConflict
		}
		return airMutationState{tx: tx, ctx: ctx, replayed: true, raw: []byte(raw)}, auth, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		tx.Rollback()
		return airMutationState{}, auth, err
	}
	return airMutationState{tx: tx, ctx: ctx}, auth, nil
}

func finishAirMutation(state airMutationState, auth AirMutationAuth, operation string, stored any) error {
	raw, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	if _, err := state.tx.Exec(`INSERT INTO air_mutation_results(
  actor_id, idempotency_key_hash, operation, request_hash, response_json, created_at
) VALUES(?, ?, ?, ?, ?, ?)`, state.ctx.ActorID, auth.IdempotencyKeyHash,
		operation, auth.RequestHash, string(raw), auth.Now); err != nil {
		return err
	}
	return state.tx.Commit()
}

func replayAirMutation[T any](state airMutationState) (T, error) {
	var result T
	if err := json.Unmarshal(state.raw, &result); err != nil {
		state.tx.Rollback()
		return result, err
	}
	if err := state.tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}

func airControlKeyTx(tx *sql.Tx) ([]byte, error) {
	const setting = "air_invite_hmac_v1"
	var encoded string
	err := tx.QueryRow(`SELECT value FROM settings WHERE key = ?`, setting).Scan(&encoded)
	if err == nil {
		key, decodeErr := hex.DecodeString(encoded)
		if decodeErr != nil || len(key) != 32 {
			return nil, errors.New("invalid Air invite HMAC key")
		}
		return key, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	key := make([]byte, 32)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`INSERT INTO settings(key, value) VALUES(?, ?)`, setting, hex.EncodeToString(key)); err != nil {
		return nil, err
	}
	return key, nil
}

func deriveAirInviteCode(key []byte, actorID int64, idempotencyHash, requestHash string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = fmt.Fprintf(mac, "pulsar-air-invite-secret/v1:%d:%s:%s", actorID, idempotencyHash, requestHash)
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}

func hashAirInviteCode(key []byte, code string) string {
	mac := hmac.New(sha256.New, key)
	_, _ = mac.Write([]byte("pulsar-air-invite-code/v1:" + code))
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *Store) AuthorizedAirList(actorID int64, bearer string) (AirListView, error) {
	return s.AuthorizedAirListForIdentity(actorID, Identity{Kind: IdentityBearer, Token: bearer})
}

func (s *Store) AuthorizedAirListForIdentity(actorID int64, identity Identity) (AirListView, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return AirListView{}, err
	}
	defer tx.Rollback()
	if _, err := requireAirsAuthoritativeTx(tx); err != nil {
		return AirListView{}, err
	}
	ctx, err := resolveActorContext(tx, identity)
	if err != nil {
		return AirListView{}, err
	}
	if ctx.ActorID != actorID {
		return AirListView{}, ErrUnauthorized
	}
	view, err := airListViewTx(tx, ctx.OrbitID)
	if err != nil {
		return AirListView{}, err
	}
	return view, tx.Commit()
}

func (s *Store) AuthorizedAir(actorID int64, bearer, airID string) (AirProjection, error) {
	return s.AuthorizedAirForIdentity(actorID, Identity{Kind: IdentityBearer, Token: bearer}, airID)
}

func (s *Store) AuthorizedAirForIdentity(actorID int64, identity Identity, airID string) (AirProjection, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return AirProjection{}, err
	}
	defer tx.Rollback()
	if _, err := requireAirsAuthoritativeTx(tx); err != nil {
		return AirProjection{}, err
	}
	ctx, err := resolveActorContext(tx, identity)
	if err != nil {
		return AirProjection{}, err
	}
	if ctx.ActorID != actorID {
		return AirProjection{}, ErrUnauthorized
	}
	projection, err := airProjectionTx(tx, airID, ctx.OrbitID)
	if err != nil {
		return AirProjection{}, err
	}
	return projection, tx.Commit()
}

func airListViewTx(tx *sql.Tx, orbitID int64) (AirListView, error) {
	view := AirListView{Saved: []AirProjection{}}
	err := tx.QueryRow(`SELECT air_id, revision FROM air_active_pointers WHERE orbit_id = ?`, orbitID).
		Scan(&view.CurrentAirID, &view.ActivePointerRevision)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return view, err
	}
	rows, err := tx.Query(`SELECT a.public_id
FROM airs a JOIN air_members m ON m.air_id = a.public_id
WHERE m.orbit_id = ? AND m.status IN ('pending_confirmation', 'joined')
  AND a.status <> 'dissolved'
ORDER BY lower(a.title), a.public_id`, orbitID)
	if err != nil {
		return view, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return view, err
		}
		ids = append(ids, id)
	}
	if err := rows.Close(); err != nil {
		return view, err
	}
	for _, id := range ids {
		projection, err := airProjectionTx(tx, id, orbitID)
		if err != nil {
			return view, err
		}
		view.Saved = append(view.Saved, projection)
	}
	return view, nil
}

func airProjectionTx(tx *sql.Tx, airID string, orbitID int64) (AirProjection, error) {
	projection := AirProjection{Capacity: AirCapacityView{Barycenters: AirBarycenterCapacity, OnlinePulsars: AirPulsarCapacity}}
	var current int
	err := tx.QueryRow(`SELECT a.public_id, a.title, a.status, a.revision,
  m.public_id, m.status, m.revision, m.air_role,
  (SELECT COUNT(*) FROM air_members x WHERE x.air_id = a.public_id AND x.status = 'joined'),
  (SELECT COUNT(*) FROM air_active_pointers p JOIN air_members x
     ON x.air_id = p.air_id AND x.orbit_id = p.orbit_id
     WHERE p.air_id = a.public_id AND x.status = 'joined'),
  p.revision, p.invite_policy, p.overlay_policy, p.queue_policy, p.replace_policy,
  EXISTS(SELECT 1 FROM air_active_pointers ap WHERE ap.orbit_id = ? AND ap.air_id = a.public_id)
FROM airs a JOIN air_members m ON m.air_id = a.public_id
JOIN air_policies p ON p.air_id = a.public_id
WHERE a.public_id = ? AND a.status <> 'dissolved' AND m.orbit_id = ?
  AND m.status IN ('pending_confirmation', 'joined')`, orbitID, airID, orbitID).Scan(
		&projection.AirID, &projection.Title, &projection.Status, &projection.Revision,
		&projection.MembershipID, &projection.MembershipStatus, &projection.MembershipRevision,
		&projection.AirRole, &projection.MemberCount, &projection.ActiveMemberCount,
		&projection.Policy.Revision, &projection.Policy.Invite, &projection.Policy.Overlay,
		&projection.Policy.Queue, &projection.Policy.Replace, &current)
	if errors.Is(err, sql.ErrNoRows) {
		return AirProjection{}, ErrAirNotFound
	}
	projection.IsCurrent = current != 0
	return projection, err
}

func currentPrimary(ctx ActorContext) bool { return ctx.Role == "primary" }

func visibleAirMemberTx(tx *sql.Tx, airID string, orbitID int64) (AirMember, Air, error) {
	var member AirMember
	var air Air
	err := tx.QueryRow(`SELECT m.public_id, m.air_id, m.orbit_id, m.air_role, m.status,
  m.revision, m.joined_at, m.left_at, m.created_at,
  a.public_id, a.title, a.status, a.owner_orbit_id, a.revision, a.created_at, a.dissolved_at
FROM air_members m JOIN airs a ON a.public_id = m.air_id
WHERE m.air_id = ? AND m.orbit_id = ? AND m.status IN ('pending_confirmation', 'joined')`,
		airID, orbitID).Scan(&member.ID, &member.AirID, &member.OrbitID, &member.Role,
		&member.Status, &member.Revision, &member.JoinedAt, &member.LeftAt, &member.CreatedAt,
		&air.ID, &air.Title, &air.Status, &air.OwnerOrbitID, &air.Revision, &air.CreatedAt, &air.DissolvedAt)
	if errors.Is(err, sql.ErrNoRows) {
		var terminalStatus string
		terminalErr := tx.QueryRow(`SELECT a.status FROM airs a JOIN air_members m ON m.air_id = a.public_id
WHERE a.public_id = ? AND m.orbit_id = ? ORDER BY m.created_at DESC LIMIT 1`, airID, orbitID).
			Scan(&terminalStatus)
		if terminalErr == nil && terminalStatus == "dissolved" {
			return AirMember{}, Air{}, ErrAirDissolved
		}
		if terminalErr != nil && !errors.Is(terminalErr, sql.ErrNoRows) {
			return AirMember{}, Air{}, terminalErr
		}
		return AirMember{}, Air{}, ErrAirNotFound
	}
	if err == nil && air.Status == "dissolved" {
		return AirMember{}, Air{}, ErrAirDissolved
	}
	return member, air, err
}

func (s *Store) CreateAuthorizedAir(auth AirMutationAuth, title string) (AirProjection, error) {
	title = strings.TrimSpace(title)
	if title == "" || len([]rune(title)) > 80 {
		return AirProjection{}, ErrAirInvalid
	}
	state, auth, err := s.beginAirMutation(auth, "air.create")
	if err != nil {
		return AirProjection{}, err
	}
	defer state.tx.Rollback()
	if state.replayed {
		return replayAirMutation[AirProjection](state)
	}
	if !currentPrimary(state.ctx) {
		return AirProjection{}, ErrAirForbidden
	}
	airID := "air_" + ulid.New(time.UnixMilli(auth.Now))
	memberID := "aim_" + ulid.New(time.UnixMilli(auth.Now))
	if _, err := state.tx.Exec(`INSERT INTO airs(public_id, title, status, owner_orbit_id, revision, created_at)
VALUES(?, ?, 'parked', ?, 1, ?)`, airID, title, state.ctx.OrbitID, auth.Now); err != nil {
		return AirProjection{}, err
	}
	if _, err := state.tx.Exec(`INSERT INTO air_members(
  public_id, air_id, orbit_id, air_role, status, revision, joined_at, created_at
) VALUES(?, ?, ?, 'owner', 'joined', 1, ?, ?)`, memberID, airID,
		state.ctx.OrbitID, auth.Now, auth.Now); err != nil {
		return AirProjection{}, err
	}
	if _, err := state.tx.Exec(`INSERT INTO air_policies(
  air_id, revision, invite_policy, overlay_policy, queue_policy, replace_policy, updated_at
) VALUES(?, 1, 'air_admin_primary', 'primary_companion', 'primary_companion', 'air_admin_primary', ?)`,
		airID, auth.Now); err != nil {
		return AirProjection{}, err
	}
	if err := markAirDivergenceTx(state.tx, auth.Now); err != nil {
		return AirProjection{}, err
	}
	if err := appendAirAuditTx(state.tx, airID, memberID, "", state.ctx.ActorID,
		state.ctx.OrbitID, "air.create", "", "parked", "ok", auth.Now); err != nil {
		return AirProjection{}, err
	}
	result, err := airProjectionTx(state.tx, airID, state.ctx.OrbitID)
	if err != nil {
		return AirProjection{}, err
	}
	return result, finishAirMutation(state, auth, "air.create", result)
}

func (s *Store) IssueAuthorizedAirInvite(auth AirMutationAuth, airID, role string) (AirInviteIssueResult, error) {
	if role != "member" && role != "admin" {
		return AirInviteIssueResult{}, ErrAirInvalid
	}
	state, auth, err := s.beginAirMutation(auth, "air.invite.issue")
	if err != nil {
		return AirInviteIssueResult{}, err
	}
	defer state.tx.Rollback()
	key, err := airControlKeyTx(state.tx)
	if err != nil {
		return AirInviteIssueResult{}, err
	}
	code := deriveAirInviteCode(key, state.ctx.ActorID, auth.IdempotencyKeyHash, auth.RequestHash)
	if state.replayed {
		result, err := replayAirMutation[AirInviteIssueResult](state)
		result.Code = code
		return result, err
	}
	member, air, err := visibleAirMemberTx(state.tx, airID, state.ctx.OrbitID)
	if err != nil {
		return AirInviteIssueResult{}, err
	}
	if !currentPrimary(state.ctx) || member.Status != "joined" {
		return AirInviteIssueResult{}, ErrAirForbidden
	}
	if role == "admin" && member.Role != "owner" {
		return AirInviteIssueResult{}, ErrAirForbidden
	}
	var policyRevision int64
	var invitePolicy string
	if err := state.tx.QueryRow(`SELECT revision, invite_policy FROM air_policies WHERE air_id = ?`, airID).
		Scan(&policyRevision, &invitePolicy); err != nil {
		return AirInviteIssueResult{}, err
	}
	if (invitePolicy == "owner_primary" && member.Role != "owner") ||
		(invitePolicy == "air_admin_primary" && member.Role != "owner" && member.Role != "admin") {
		return AirInviteIssueResult{}, ErrAirPolicyDenied
	}
	var issued int
	if err := state.tx.QueryRow(`SELECT COUNT(*) FROM air_invites
WHERE air_id = ? AND issued_by_actor_id = ? AND created_at > ?`, airID, state.ctx.ActorID,
		auth.Now-time.Hour.Milliseconds()).Scan(&issued); err != nil {
		return AirInviteIssueResult{}, err
	}
	if issued >= 10 {
		return AirInviteIssueResult{}, ErrAirRateLimited
	}
	result := AirInviteIssueResult{
		InviteID: "ai_" + ulid.New(time.UnixMilli(auth.Now)), Revision: 1,
		ExpiresAt: auth.Now + AirInviteTTL.Milliseconds(), Code: code,
	}
	if _, err := state.tx.Exec(`INSERT INTO air_invites(
  public_id, air_id, code_hash, status, intended_role, issued_by_actor_id,
  issued_by_orbit_id, policy_revision, revision, expires_at, created_at, updated_at
) VALUES(?, ?, ?, 'open', ?, ?, ?, ?, 1, ?, ?, ?)`, result.InviteID, air.ID,
		hashAirInviteCode(key, code), role, state.ctx.ActorID, state.ctx.OrbitID,
		policyRevision, result.ExpiresAt, auth.Now, auth.Now); err != nil {
		return AirInviteIssueResult{}, err
	}
	if err := markAirDivergenceTx(state.tx, auth.Now); err != nil {
		return AirInviteIssueResult{}, err
	}
	if err := appendAirAuditTx(state.tx, airID, "", result.InviteID, state.ctx.ActorID,
		state.ctx.OrbitID, "air.invite.issue", "", role, "ok", auth.Now); err != nil {
		return AirInviteIssueResult{}, err
	}
	stored := result
	stored.Code = ""
	return result, finishAirMutation(state, auth, "air.invite.issue", stored)
}

func (s *Store) ConsumeAuthorizedAirInvite(auth AirMutationAuth, code string) (AirJoinPreview, error) {
	if len(code) != 43 {
		return AirJoinPreview{}, ErrAirInviteUnavailable
	}
	state, auth, err := s.beginAirMutation(auth, "air.invite.consume")
	if err != nil {
		return AirJoinPreview{}, err
	}
	defer state.tx.Rollback()
	if state.replayed {
		return replayAirMutation[AirJoinPreview](state)
	}
	key, err := airControlKeyTx(state.tx)
	if err != nil {
		return AirJoinPreview{}, err
	}
	var invite AirInvite
	err = state.tx.QueryRow(`SELECT public_id, air_id, status, intended_role,
  issued_by_actor_id, issued_by_orbit_id, policy_revision, revision, expires_at,
  consumed_membership_id, created_at, updated_at
FROM air_invites WHERE code_hash = ?`, hashAirInviteCode(key, code)).Scan(
		&invite.ID, &invite.AirID, &invite.Status, &invite.IntendedRole,
		&invite.IssuedByActorID, &invite.IssuedByOrbitID, &invite.PolicyRevision,
		&invite.Revision, &invite.ExpiresAt, &invite.ConsumedMembershipID,
		&invite.CreatedAt, &invite.UpdatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AirJoinPreview{}, ErrAirInviteUnavailable
	}
	if err != nil {
		return AirJoinPreview{}, err
	}
	if invite.Status != "open" || invite.ExpiresAt <= auth.Now {
		return AirJoinPreview{}, ErrAirInviteUnavailable
	}
	if invite.IssuedByOrbitID == state.ctx.OrbitID {
		return AirJoinPreview{}, ErrAirInvalid
	}
	var existing, occupants int
	if err := state.tx.QueryRow(`SELECT
  (SELECT COUNT(*) FROM air_members WHERE air_id = ? AND orbit_id = ?
    AND status IN ('pending_confirmation', 'joined')),
  (SELECT COUNT(*) FROM air_members WHERE air_id = ?
    AND status IN ('pending_confirmation', 'joined'))`, invite.AirID, state.ctx.OrbitID,
		invite.AirID).Scan(&existing, &occupants); err != nil {
		return AirJoinPreview{}, err
	}
	if existing != 0 {
		return AirJoinPreview{}, ErrAirAlreadyMember
	}
	if occupants >= AirBarycenterCapacity {
		return AirJoinPreview{}, ErrAirCapacity
	}
	var airStatus string
	if err := state.tx.QueryRow(`SELECT status FROM airs WHERE public_id = ?`, invite.AirID).Scan(&airStatus); err != nil {
		return AirJoinPreview{}, ErrAirInviteUnavailable
	}
	if airStatus == "dissolved" {
		return AirJoinPreview{}, ErrAirInviteUnavailable
	}
	membershipID := "aim_" + ulid.New(time.UnixMilli(auth.Now))
	if _, err := state.tx.Exec(`INSERT INTO air_members(
  public_id, air_id, orbit_id, air_role, status, revision, created_at
) VALUES(?, ?, ?, ?, 'pending_confirmation', 1, ?)`, membershipID, invite.AirID,
		state.ctx.OrbitID, invite.IntendedRole, auth.Now); err != nil {
		return AirJoinPreview{}, err
	}
	result, err := state.tx.Exec(`UPDATE air_invites SET status = 'consumed',
  consumed_membership_id = ?, revision = revision + 1, updated_at = ?
WHERE public_id = ? AND status = 'open' AND revision = ? AND expires_at > ?`,
		membershipID, auth.Now, invite.ID, invite.Revision, auth.Now)
	if err != nil {
		return AirJoinPreview{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return AirJoinPreview{}, ErrAirInviteUnavailable
	}
	preview, err := airJoinPreviewTx(state.tx, invite.AirID, state.ctx.OrbitID, membershipID)
	if err != nil {
		return AirJoinPreview{}, err
	}
	if err := markAirDivergenceTx(state.tx, auth.Now); err != nil {
		return AirJoinPreview{}, err
	}
	if err := appendAirAuditTx(state.tx, invite.AirID, membershipID, invite.ID,
		state.ctx.ActorID, state.ctx.OrbitID, "air.invite.consume", "open",
		"pending_confirmation", "ok", auth.Now); err != nil {
		return AirJoinPreview{}, err
	}
	return preview, finishAirMutation(state, auth, "air.invite.consume", preview)
}

func airJoinPreviewTx(tx *sql.Tx, airID string, orbitID int64, membershipID string) (AirJoinPreview, error) {
	preview := AirJoinPreview{
		AirID: airID, MembershipID: membershipID,
		Capacity: AirCapacityView{Barycenters: AirBarycenterCapacity, OnlinePulsars: AirPulsarCapacity},
	}
	var currentAir string
	err := tx.QueryRow(`SELECT a.title, o.title, m.air_role, m.revision,
  p.revision, p.invite_policy, p.overlay_policy, p.queue_policy, p.replace_policy,
  (SELECT COUNT(*) FROM air_members x WHERE x.air_id = a.public_id
    AND x.status IN ('pending_confirmation', 'joined')),
  COALESCE((SELECT air_id FROM air_active_pointers WHERE orbit_id = ?), '')
FROM airs a JOIN orbits o ON o.id = a.owner_orbit_id
JOIN air_members m ON m.public_id = ? AND m.air_id = a.public_id
JOIN air_policies p ON p.air_id = a.public_id WHERE a.public_id = ?`,
		orbitID, membershipID, airID).Scan(&preview.Title, &preview.OwnerDisplayName,
		&preview.IntendedRole, &preview.MembershipRevision, &preview.Policy.Revision,
		&preview.Policy.Invite, &preview.Policy.Overlay, &preview.Policy.Queue,
		&preview.Policy.Replace, &preview.MemberCount, &currentAir)
	preview.ActivationWouldSwitch = currentAir != "" && currentAir != airID
	return preview, err
}

func (s *Store) WithdrawAuthorizedAirInvite(auth AirMutationAuth, airID, inviteID string, revision int64) (AirInviteIssueResult, error) {
	state, auth, err := s.beginAirMutation(auth, "air.invite.withdraw")
	if err != nil {
		return AirInviteIssueResult{}, err
	}
	defer state.tx.Rollback()
	if state.replayed {
		return replayAirMutation[AirInviteIssueResult](state)
	}
	member, _, err := visibleAirMemberTx(state.tx, airID, state.ctx.OrbitID)
	if err != nil {
		return AirInviteIssueResult{}, err
	}
	var invite AirInvite
	err = state.tx.QueryRow(`SELECT public_id, air_id, status, issued_by_actor_id,
  revision, expires_at FROM air_invites WHERE public_id = ? AND air_id = ?`, inviteID, airID).
		Scan(&invite.ID, &invite.AirID, &invite.Status, &invite.IssuedByActorID,
			&invite.Revision, &invite.ExpiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return AirInviteIssueResult{}, ErrAirInviteUnavailable
	}
	if err != nil {
		return AirInviteIssueResult{}, err
	}
	if !currentPrimary(state.ctx) || member.Status != "joined" ||
		(invite.IssuedByActorID != state.ctx.ActorID && member.Role != "owner" && member.Role != "admin") {
		return AirInviteIssueResult{}, ErrAirForbidden
	}
	if invite.Status != "open" || invite.ExpiresAt <= auth.Now {
		return AirInviteIssueResult{}, ErrAirInviteUnavailable
	}
	if invite.Revision != revision {
		return AirInviteIssueResult{}, ErrAirRevision
	}
	if _, err := state.tx.Exec(`UPDATE air_invites SET status = 'withdrawn',
  revision = revision + 1, updated_at = ? WHERE public_id = ?`, auth.Now, inviteID); err != nil {
		return AirInviteIssueResult{}, err
	}
	result := AirInviteIssueResult{InviteID: inviteID, Revision: revision + 1, ExpiresAt: invite.ExpiresAt}
	if err := appendAirAuditTx(state.tx, airID, "", inviteID, state.ctx.ActorID,
		state.ctx.OrbitID, "air.invite.withdraw", "open", "withdrawn", "ok", auth.Now); err != nil {
		return AirInviteIssueResult{}, err
	}
	return result, finishAirMutation(state, auth, "air.invite.withdraw", result)
}

func (s *Store) ConfirmAuthorizedAirJoin(auth AirMutationAuth, airID string, membershipRevision int64,
	activate bool, expectedActiveAirID string,
) (AirLifecycleResult, error) {
	state, auth, err := s.beginAirMutation(auth, "air.join.confirm")
	if err != nil {
		return AirLifecycleResult{}, err
	}
	defer state.tx.Rollback()
	if state.replayed {
		return replayAirMutation[AirLifecycleResult](state)
	}
	if !currentPrimary(state.ctx) {
		return AirLifecycleResult{}, ErrAirForbidden
	}
	member, _, err := visibleAirMemberTx(state.tx, airID, state.ctx.OrbitID)
	if err != nil {
		return AirLifecycleResult{}, err
	}
	if member.Status != "pending_confirmation" {
		return AirLifecycleResult{}, ErrAirConfirmationRequired
	}
	if member.Revision != membershipRevision {
		return AirLifecycleResult{}, ErrAirRevision
	}
	var occupants int
	if err := state.tx.QueryRow(`SELECT COUNT(*) FROM air_members WHERE air_id = ?
  AND status IN ('pending_confirmation', 'joined')`, airID).Scan(&occupants); err != nil {
		return AirLifecycleResult{}, err
	}
	if occupants > AirBarycenterCapacity {
		return AirLifecycleResult{}, ErrAirCapacity
	}
	previous, pointerRevision, hasPointer, err := activeAirForOrbitTx(state.tx, state.ctx.OrbitID)
	if err != nil {
		return AirLifecycleResult{}, err
	}
	if activate && !activeAirExpectationMatches(previous, hasPointer, expectedActiveAirID) {
		return AirLifecycleResult{}, ErrAirActiveChanged
	}
	if _, err := state.tx.Exec(`UPDATE air_members SET status = 'joined', joined_at = ?,
  revision = revision + 1 WHERE public_id = ? AND status = 'pending_confirmation' AND revision = ?`,
		auth.Now, member.ID, membershipRevision); err != nil {
		return AirLifecycleResult{}, err
	}
	if activate {
		if err := switchActiveAirTx(state.tx, state.ctx.OrbitID, airID, previous, pointerRevision, hasPointer, auth.Now); err != nil {
			return AirLifecycleResult{}, err
		}
	}
	if err := bumpAirRevisionTx(state.tx, airID); err != nil {
		return AirLifecycleResult{}, err
	}
	if err := markAirDivergenceTx(state.tx, auth.Now); err != nil {
		return AirLifecycleResult{}, err
	}
	if err := appendAirAuditTx(state.tx, airID, member.ID, "", state.ctx.ActorID,
		state.ctx.OrbitID, "air.join.confirm", "pending_confirmation", "joined", "ok", auth.Now); err != nil {
		return AirLifecycleResult{}, err
	}
	projection, err := airProjectionTx(state.tx, airID, state.ctx.OrbitID)
	if err != nil {
		return AirLifecycleResult{}, err
	}
	result := AirLifecycleResult{AirID: airID, Status: projection.Status, Projection: &projection, PreviousAirID: previous}
	return result, finishAirMutation(state, auth, "air.join.confirm", result)
}

func switchActiveAirTx(tx *sql.Tx, orbitID int64, airID, previous string, pointerRevision int64, hasPointer bool, now int64) error {
	if hasPointer && previous == airID {
		return nil
	}
	if hasPointer {
		result, err := tx.Exec(`DELETE FROM air_active_pointers WHERE orbit_id = ? AND revision = ?`, orbitID, pointerRevision)
		if err != nil {
			return err
		}
		if changed, err := result.RowsAffected(); err != nil || changed != 1 {
			return ErrAirActiveChanged
		}
	}
	if _, err := tx.Exec(`INSERT INTO air_active_pointers(orbit_id, air_id, revision, activated_at)
VALUES(?, ?, ?, ?)`, orbitID, airID, pointerRevision+1, now); err != nil {
		return err
	}
	if hasPointer && previous != airID {
		if err := refreshAirStatusTx(tx, previous); err != nil {
			return err
		}
		if err := bumpAirRevisionTx(tx, previous); err != nil {
			return err
		}
	}
	return refreshAirStatusTx(tx, airID)
}

func (s *Store) DeclineAuthorizedAirJoin(auth AirMutationAuth, airID string, membershipRevision int64) (AirLifecycleResult, error) {
	state, auth, err := s.beginAirMutation(auth, "air.join.decline")
	if err != nil {
		return AirLifecycleResult{}, err
	}
	defer state.tx.Rollback()
	if state.replayed {
		return replayAirMutation[AirLifecycleResult](state)
	}
	if !currentPrimary(state.ctx) {
		return AirLifecycleResult{}, ErrAirForbidden
	}
	member, _, err := visibleAirMemberTx(state.tx, airID, state.ctx.OrbitID)
	if err != nil {
		return AirLifecycleResult{}, err
	}
	if member.Status != "pending_confirmation" {
		return AirLifecycleResult{}, ErrAirMembershipNotFound
	}
	if member.Revision != membershipRevision {
		return AirLifecycleResult{}, ErrAirRevision
	}
	if _, err := state.tx.Exec(`UPDATE air_members SET status = 'left', left_at = ?,
  revision = revision + 1 WHERE public_id = ?`, auth.Now, member.ID); err != nil {
		return AirLifecycleResult{}, err
	}
	if err := bumpAirRevisionTx(state.tx, airID); err != nil {
		return AirLifecycleResult{}, err
	}
	if err := markAirDivergenceTx(state.tx, auth.Now); err != nil {
		return AirLifecycleResult{}, err
	}
	if err := appendAirAuditTx(state.tx, airID, member.ID, "", state.ctx.ActorID,
		state.ctx.OrbitID, "air.join.decline", "pending_confirmation", "left", "ok", auth.Now); err != nil {
		return AirLifecycleResult{}, err
	}
	result := AirLifecycleResult{AirID: airID, Status: "left"}
	return result, finishAirMutation(state, auth, "air.join.decline", result)
}

func (s *Store) ActivateAuthorizedAir(auth AirMutationAuth, airID string, membershipRevision int64,
	expectedActiveAirID string,
) (AirLifecycleResult, error) {
	state, auth, err := s.beginAirMutation(auth, "air.activate")
	if err != nil {
		return AirLifecycleResult{}, err
	}
	defer state.tx.Rollback()
	if state.replayed {
		return replayAirMutation[AirLifecycleResult](state)
	}
	member, _, err := visibleAirMemberTx(state.tx, airID, state.ctx.OrbitID)
	if err != nil {
		return AirLifecycleResult{}, err
	}
	if member.Status != "joined" {
		return AirLifecycleResult{}, ErrAirConfirmationRequired
	}
	if member.Revision != membershipRevision {
		return AirLifecycleResult{}, ErrAirRevision
	}
	previous, revision, ok, err := activeAirForOrbitTx(state.tx, state.ctx.OrbitID)
	if err != nil {
		return AirLifecycleResult{}, err
	}
	if !activeAirExpectationMatches(previous, ok, expectedActiveAirID) {
		return AirLifecycleResult{}, ErrAirActiveChanged
	}
	if err := switchActiveAirTx(state.tx, state.ctx.OrbitID, airID, previous, revision, ok, auth.Now); err != nil {
		return AirLifecycleResult{}, err
	}
	if previous != airID {
		if err := bumpAirRevisionTx(state.tx, airID); err != nil {
			return AirLifecycleResult{}, err
		}
		if err := markAirDivergenceTx(state.tx, auth.Now); err != nil {
			return AirLifecycleResult{}, err
		}
	}
	if err := appendAirAuditTx(state.tx, airID, member.ID, "", state.ctx.ActorID,
		state.ctx.OrbitID, "air.activate", previous, airID, "ok", auth.Now); err != nil {
		return AirLifecycleResult{}, err
	}
	projection, err := airProjectionTx(state.tx, airID, state.ctx.OrbitID)
	if err != nil {
		return AirLifecycleResult{}, err
	}
	result := AirLifecycleResult{AirID: airID, Status: projection.Status, Projection: &projection, PreviousAirID: previous}
	return result, finishAirMutation(state, auth, "air.activate", result)
}

func (s *Store) DeactivateAuthorizedAir(auth AirMutationAuth, airID string, membershipRevision int64,
	expectedActiveAirID string,
) (AirLifecycleResult, error) {
	state, auth, err := s.beginAirMutation(auth, "air.deactivate")
	if err != nil {
		return AirLifecycleResult{}, err
	}
	defer state.tx.Rollback()
	if state.replayed {
		return replayAirMutation[AirLifecycleResult](state)
	}
	member, _, err := visibleAirMemberTx(state.tx, airID, state.ctx.OrbitID)
	if err != nil {
		return AirLifecycleResult{}, err
	}
	if member.Status != "joined" || member.Revision != membershipRevision {
		return AirLifecycleResult{}, ErrAirRevision
	}
	current, revision, ok, err := activeAirForOrbitTx(state.tx, state.ctx.OrbitID)
	if err != nil {
		return AirLifecycleResult{}, err
	}
	if !activeAirExpectationMatches(current, ok, expectedActiveAirID) {
		return AirLifecycleResult{}, ErrAirActiveChanged
	}
	if ok {
		if current != airID {
			return AirLifecycleResult{}, ErrAirActiveChanged
		}
		result, err := state.tx.Exec(`DELETE FROM air_active_pointers WHERE orbit_id = ? AND revision = ?`, state.ctx.OrbitID, revision)
		if err != nil {
			return AirLifecycleResult{}, err
		}
		if changed, err := result.RowsAffected(); err != nil || changed != 1 {
			return AirLifecycleResult{}, ErrAirActiveChanged
		}
		if err := refreshAirStatusTx(state.tx, airID); err != nil {
			return AirLifecycleResult{}, err
		}
		if err := bumpAirRevisionTx(state.tx, airID); err != nil {
			return AirLifecycleResult{}, err
		}
		if err := markAirDivergenceTx(state.tx, auth.Now); err != nil {
			return AirLifecycleResult{}, err
		}
	}
	if err := appendAirAuditTx(state.tx, airID, member.ID, "", state.ctx.ActorID,
		state.ctx.OrbitID, "air.deactivate", current, "", "ok", auth.Now); err != nil {
		return AirLifecycleResult{}, err
	}
	projection, err := airProjectionTx(state.tx, airID, state.ctx.OrbitID)
	if err != nil {
		return AirLifecycleResult{}, err
	}
	result := AirLifecycleResult{AirID: airID, Status: projection.Status, Projection: &projection, PreviousAirID: current}
	return result, finishAirMutation(state, auth, "air.deactivate", result)
}

func (s *Store) LeaveAuthorizedAir(auth AirMutationAuth, airID string, membershipRevision int64,
	expectedActiveAirID string,
) (AirLifecycleResult, error) {
	state, auth, err := s.beginAirMutation(auth, "air.leave")
	if err != nil {
		return AirLifecycleResult{}, err
	}
	defer state.tx.Rollback()
	if state.replayed {
		return replayAirMutation[AirLifecycleResult](state)
	}
	if !currentPrimary(state.ctx) {
		return AirLifecycleResult{}, ErrAirForbidden
	}
	member, _, err := visibleAirMemberTx(state.tx, airID, state.ctx.OrbitID)
	if err != nil {
		return AirLifecycleResult{}, err
	}
	if member.Role == "owner" {
		return AirLifecycleResult{}, ErrAirOwnerLeave
	}
	if member.Status != "joined" || member.Revision != membershipRevision {
		return AirLifecycleResult{}, ErrAirRevision
	}
	current, revision, ok, err := activeAirForOrbitTx(state.tx, state.ctx.OrbitID)
	if err != nil {
		return AirLifecycleResult{}, err
	}
	if !activeAirExpectationMatches(current, ok, expectedActiveAirID) {
		return AirLifecycleResult{}, ErrAirActiveChanged
	}
	if ok && current == airID {
		if _, err := state.tx.Exec(`DELETE FROM air_active_pointers WHERE orbit_id = ? AND revision = ?`,
			state.ctx.OrbitID, revision); err != nil {
			return AirLifecycleResult{}, err
		}
	}
	if _, err := state.tx.Exec(`UPDATE air_members SET status = 'left', left_at = ?,
  revision = revision + 1 WHERE public_id = ?`, auth.Now, member.ID); err != nil {
		return AirLifecycleResult{}, err
	}
	if err := refreshAirStatusTx(state.tx, airID); err != nil {
		return AirLifecycleResult{}, err
	}
	if err := bumpAirRevisionTx(state.tx, airID); err != nil {
		return AirLifecycleResult{}, err
	}
	if err := markAirDivergenceTx(state.tx, auth.Now); err != nil {
		return AirLifecycleResult{}, err
	}
	if err := appendAirAuditTx(state.tx, airID, member.ID, "", state.ctx.ActorID,
		state.ctx.OrbitID, "air.leave", "joined", "left", "ok", auth.Now); err != nil {
		return AirLifecycleResult{}, err
	}
	result := AirLifecycleResult{AirID: airID, Status: "left", PreviousAirID: current}
	return result, finishAirMutation(state, auth, "air.leave", result)
}

func ownerPrimaryTx(tx *sql.Tx, ctx ActorContext, airID string) (AirMember, Air, error) {
	member, air, err := visibleAirMemberTx(tx, airID, ctx.OrbitID)
	if err != nil {
		return member, air, err
	}
	if !currentPrimary(ctx) || member.Status != "joined" || member.Role != "owner" || air.OwnerOrbitID != ctx.OrbitID {
		return AirMember{}, Air{}, ErrAirForbidden
	}
	return member, air, nil
}

func (s *Store) ReplaceAuthorizedAirMemberRole(auth AirMutationAuth, airID, membershipID string,
	airRevision, membershipRevision int64, role string,
) (AirProjection, error) {
	if role != "admin" && role != "member" {
		return AirProjection{}, ErrAirInvalid
	}
	state, auth, err := s.beginAirMutation(auth, "air.member.role")
	if err != nil {
		return AirProjection{}, err
	}
	defer state.tx.Rollback()
	if state.replayed {
		return replayAirMutation[AirProjection](state)
	}
	_, air, err := ownerPrimaryTx(state.tx, state.ctx, airID)
	if err != nil {
		return AirProjection{}, err
	}
	if air.Revision != airRevision {
		return AirProjection{}, ErrAirRevision
	}
	var targetRole, targetStatus string
	var targetRevision int64
	if err := state.tx.QueryRow(`SELECT air_role, status, revision FROM air_members
WHERE public_id = ? AND air_id = ?`, membershipID, airID).Scan(&targetRole, &targetStatus, &targetRevision); err != nil {
		return AirProjection{}, ErrAirMembershipNotFound
	}
	if targetStatus != "joined" || targetRole == "owner" {
		return AirProjection{}, ErrAirMembershipNotFound
	}
	if targetRevision != membershipRevision {
		return AirProjection{}, ErrAirRevision
	}
	if _, err := state.tx.Exec(`UPDATE air_members SET air_role = ?, revision = revision + 1
WHERE public_id = ?`, role, membershipID); err != nil {
		return AirProjection{}, err
	}
	if err := bumpAirRevisionTx(state.tx, airID); err != nil {
		return AirProjection{}, err
	}
	if err := markAirDivergenceTx(state.tx, auth.Now); err != nil {
		return AirProjection{}, err
	}
	if err := appendAirAuditTx(state.tx, airID, membershipID, "", state.ctx.ActorID,
		state.ctx.OrbitID, "air.member.role", targetRole, role, "ok", auth.Now); err != nil {
		return AirProjection{}, err
	}
	projection, err := airProjectionTx(state.tx, airID, state.ctx.OrbitID)
	if err != nil {
		return AirProjection{}, err
	}
	return projection, finishAirMutation(state, auth, "air.member.role", projection)
}

func (s *Store) TransferAuthorizedAirOwnership(auth AirMutationAuth, airID, membershipID string,
	airRevision, membershipRevision int64,
) (AirProjection, error) {
	state, auth, err := s.beginAirMutation(auth, "air.ownership.transfer")
	if err != nil {
		return AirProjection{}, err
	}
	defer state.tx.Rollback()
	if state.replayed {
		return replayAirMutation[AirProjection](state)
	}
	owner, air, err := ownerPrimaryTx(state.tx, state.ctx, airID)
	if err != nil {
		return AirProjection{}, err
	}
	if air.Revision != airRevision {
		return AirProjection{}, ErrAirRevision
	}
	var targetOrbit, targetRevision int64
	var targetRole, targetStatus string
	if err := state.tx.QueryRow(`SELECT orbit_id, revision, air_role, status FROM air_members
WHERE public_id = ? AND air_id = ?`, membershipID, airID).Scan(&targetOrbit, &targetRevision,
		&targetRole, &targetStatus); err != nil {
		return AirProjection{}, ErrAirMembershipNotFound
	}
	if targetStatus != "joined" || targetRole == "owner" {
		return AirProjection{}, ErrAirMembershipNotFound
	}
	if targetRevision != membershipRevision {
		return AirProjection{}, ErrAirRevision
	}
	if _, err := state.tx.Exec(`UPDATE air_members SET air_role = 'admin', revision = revision + 1
WHERE public_id = ?`, owner.ID); err != nil {
		return AirProjection{}, err
	}
	if _, err := state.tx.Exec(`UPDATE air_members SET air_role = 'owner', revision = revision + 1
WHERE public_id = ?`, membershipID); err != nil {
		return AirProjection{}, err
	}
	if _, err := state.tx.Exec(`UPDATE airs SET owner_orbit_id = ?, revision = revision + 1
WHERE public_id = ?`, targetOrbit, airID); err != nil {
		return AirProjection{}, err
	}
	if err := markAirDivergenceTx(state.tx, auth.Now); err != nil {
		return AirProjection{}, err
	}
	if err := appendAirAuditTx(state.tx, airID, membershipID, "", state.ctx.ActorID,
		state.ctx.OrbitID, "air.ownership.transfer", strconv.FormatInt(state.ctx.OrbitID, 10),
		strconv.FormatInt(targetOrbit, 10), "ok", auth.Now); err != nil {
		return AirProjection{}, err
	}
	projection, err := airProjectionTx(state.tx, airID, state.ctx.OrbitID)
	if err != nil {
		return AirProjection{}, err
	}
	return projection, finishAirMutation(state, auth, "air.ownership.transfer", projection)
}

func validAirPolicy(policy AirPolicyView) bool {
	invite := map[string]bool{"owner_primary": true, "air_admin_primary": true, "all_member_primaries": true}
	overlay := map[string]bool{"air_admin_primary": true, "all_member_primaries": true, "primary_companion": true, "disabled": true}
	replace := map[string]bool{"owner_primary": true, "air_admin_primary": true, "all_member_primaries": true, "disabled": true}
	return policy.Revision > 0 && invite[policy.Invite] && overlay[policy.Overlay] &&
		overlay[policy.Queue] && replace[policy.Replace]
}

func airPolicyAuditValue(policy AirPolicyView) string {
	raw, err := json.Marshal(policy)
	if err != nil {
		return "{}"
	}
	return string(raw)
}

func (s *Store) ReplaceAuthorizedAirPolicy(auth AirMutationAuth, airID string, policy AirPolicyView) (AirProjection, error) {
	if !validAirPolicy(policy) {
		return AirProjection{}, ErrAirInvalid
	}
	state, auth, err := s.beginAirMutation(auth, "air.policy.replace")
	if err != nil {
		return AirProjection{}, err
	}
	defer state.tx.Rollback()
	if state.replayed {
		return replayAirMutation[AirProjection](state)
	}
	if _, _, err := ownerPrimaryTx(state.tx, state.ctx, airID); err != nil {
		return AirProjection{}, err
	}
	var oldPolicy AirPolicyView
	if err := state.tx.QueryRow(`SELECT revision, invite_policy, overlay_policy,
  queue_policy, replace_policy FROM air_policies WHERE air_id = ?`, airID).Scan(
		&oldPolicy.Revision, &oldPolicy.Invite, &oldPolicy.Overlay,
		&oldPolicy.Queue, &oldPolicy.Replace,
	); err != nil {
		return AirProjection{}, err
	}
	result, err := state.tx.Exec(`UPDATE air_policies SET revision = revision + 1,
  invite_policy = ?, overlay_policy = ?, queue_policy = ?, replace_policy = ?, updated_at = ?
WHERE air_id = ? AND revision = ?`, policy.Invite, policy.Overlay, policy.Queue,
		policy.Replace, auth.Now, airID, policy.Revision)
	if err != nil {
		return AirProjection{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return AirProjection{}, ErrAirRevision
	}
	if err := markAirDivergenceTx(state.tx, auth.Now); err != nil {
		return AirProjection{}, err
	}
	if err := appendAirAuditTx(state.tx, airID, "", "", state.ctx.ActorID,
		state.ctx.OrbitID, "air.policy.replace", airPolicyAuditValue(oldPolicy),
		airPolicyAuditValue(AirPolicyView{Revision: policy.Revision + 1, Invite: policy.Invite,
			Overlay: policy.Overlay, Queue: policy.Queue, Replace: policy.Replace}),
		"ok", auth.Now); err != nil {
		return AirProjection{}, err
	}
	projection, err := airProjectionTx(state.tx, airID, state.ctx.OrbitID)
	if err != nil {
		return AirProjection{}, err
	}
	return projection, finishAirMutation(state, auth, "air.policy.replace", projection)
}

func (s *Store) DissolveAuthorizedAir(auth AirMutationAuth, airID string, airRevision int64) (AirLifecycleResult, error) {
	state, auth, err := s.beginAirMutation(auth, "air.dissolve")
	if err != nil {
		return AirLifecycleResult{}, err
	}
	defer state.tx.Rollback()
	if state.replayed {
		return replayAirMutation[AirLifecycleResult](state)
	}
	_, air, err := ownerPrimaryTx(state.tx, state.ctx, airID)
	if err != nil {
		return AirLifecycleResult{}, err
	}
	if air.Revision != airRevision {
		return AirLifecycleResult{}, ErrAirRevision
	}
	if _, err := state.tx.Exec(`DELETE FROM air_active_pointers WHERE air_id = ?`, airID); err != nil {
		return AirLifecycleResult{}, err
	}
	if _, err := state.tx.Exec(`UPDATE air_members SET status = 'left', left_at = ?,
  revision = revision + 1 WHERE air_id = ? AND status IN ('pending_confirmation', 'joined')`, auth.Now, airID); err != nil {
		return AirLifecycleResult{}, err
	}
	if _, err := state.tx.Exec(`UPDATE air_invites SET status = 'withdrawn',
  revision = revision + 1, updated_at = ? WHERE air_id = ? AND status = 'open'`, auth.Now, airID); err != nil {
		return AirLifecycleResult{}, err
	}
	result, err := state.tx.Exec(`UPDATE airs SET status = 'dissolved', dissolved_at = ?,
  revision = revision + 1 WHERE public_id = ? AND revision = ? AND status <> 'dissolved'`,
		auth.Now, airID, airRevision)
	if err != nil {
		return AirLifecycleResult{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		return AirLifecycleResult{}, ErrAirRevision
	}
	if err := markAirDivergenceTx(state.tx, auth.Now); err != nil {
		return AirLifecycleResult{}, err
	}
	if err := appendAirAuditTx(state.tx, airID, "", "", state.ctx.ActorID,
		state.ctx.OrbitID, "air.dissolve", air.Status, "dissolved", "ok", auth.Now); err != nil {
		return AirLifecycleResult{}, err
	}
	lifecycle := AirLifecycleResult{AirID: airID, Status: "dissolved"}
	return lifecycle, finishAirMutation(state, auth, "air.dissolve", lifecycle)
}
