package store

import (
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	automationcontract "relux.works/duet/coordinator/internal/automation"
	"relux.works/duet/coordinator/internal/ulid"
)

const (
	AutomationSecretHashVersion  = "sha256-domain-v1"
	AutomationExecutionRetention = 90 * 24 * time.Hour
	AutomationMaxLease           = time.Minute
)

var (
	ErrAutomationInvalid              = errors.New("automation lineage input is invalid")
	ErrAutomationNotFound             = errors.New("automation lineage was not found")
	ErrAutomationStateConflict        = errors.New("automation lineage state changed")
	ErrAutomationInvalidCredential    = errors.New("invalid automation credential")
	ErrAutomationIdempotencyConflict  = errors.New("automation idempotency conflict")
	ErrAutomationOccurrenceNotCurrent = errors.New("automation occurrence is not the current canonical minute")
	ErrAutomationOccurrenceLaterFold  = errors.New("automation occurrence is the repeated DST fold minute")
	ErrAutomationDisabled             = errors.New("automation is disabled")
	automationScheduleIDPattern       = regexp.MustCompile(`^sch_[0-9A-HJKMNP-TV-Z]{26}$`)
	automationPrincipalIDPattern      = regexp.MustCompile(`^ap_[0-9A-HJKMNP-TV-Z]{26}$`)
	automationExecutionIDPattern      = regexp.MustCompile(`^ax_[0-9A-HJKMNP-TV-Z]{26}$`)
)

type AutomationFeatureState struct {
	OwnerOrbitID        int64
	SoundboardEnabled   bool
	AutomationEnabled   bool
	EmergencyDisabled   bool
	Timezone            string
	QuietHoursJSON      string
	QuietHoursHash      string
	PolicyVersion       string
	Revision            int64
	UpdatedByActorID    int64
	UpdatedAt           int64
	EmergencyDisabledAt int64
}

type SetAutomationFeatureStateParams struct {
	ExpectedActorID   int64
	Bearer            string
	SoundboardEnabled bool
	AutomationEnabled bool
	EmergencyDisabled bool
	Timezone          string
	QuietHoursJSON    string
	ExpectedRevision  int64
	OccurredAt        int64
}

func boolInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func compactAutomationPolicy(raw string) (string, string, error) {
	if raw == "" {
		return "", "", nil
	}
	var value any
	if err := json.Unmarshal([]byte(raw), &value); err != nil {
		return "", "", ErrAutomationInvalid
	}
	canonical, err := json.Marshal(value)
	if err != nil || len(canonical) > 16384 {
		return "", "", ErrAutomationInvalid
	}
	digest := sha256.Sum256(canonical)
	return string(canonical), hex.EncodeToString(digest[:]), nil
}

func scanAutomationFeatureState(row interface{ Scan(...any) error }) (AutomationFeatureState, error) {
	var state AutomationFeatureState
	var soundboard, automationEnabled, emergency int
	err := row.Scan(&state.OwnerOrbitID, &soundboard, &automationEnabled, &emergency,
		&state.Timezone, &state.QuietHoursJSON, &state.QuietHoursHash,
		&state.PolicyVersion, &state.Revision, &state.UpdatedByActorID,
		&state.UpdatedAt, &state.EmergencyDisabledAt)
	state.SoundboardEnabled = soundboard != 0
	state.AutomationEnabled = automationEnabled != 0
	state.EmergencyDisabled = emergency != 0
	return state, err
}

const automationFeatureColumns = `owner_orbit_id, soundboard_enabled,
automation_enabled, emergency_disabled, timezone, quiet_hours_json,
quiet_hours_hash, policy_version, revision, updated_by_actor_id, updated_at,
emergency_disabled_at`

func (s *Store) SetAutomationFeatureState(params SetAutomationFeatureStateParams) (AutomationFeatureState, error) {
	if params.OccurredAt <= 0 || params.ExpectedRevision < 0 {
		return AutomationFeatureState{}, ErrAutomationInvalid
	}
	if params.Timezone != "" {
		if _, err := time.LoadLocation(params.Timezone); err != nil {
			return AutomationFeatureState{}, ErrAutomationInvalid
		}
	}
	quietJSON, quietHash, err := compactAutomationPolicy(params.QuietHoursJSON)
	if err != nil {
		return AutomationFeatureState{}, err
	}
	if params.AutomationEnabled && (params.Timezone == "" || quietJSON == "") {
		return AutomationFeatureState{}, ErrAutomationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AutomationFeatureState{}, err
	}
	defer tx.Rollback()
	ctx, err := authorizeSavedCueMutationTx(tx, params.ExpectedActorID, params.Bearer)
	if err != nil {
		return AutomationFeatureState{}, err
	}
	if params.AutomationEnabled {
		if err := requireCurrentContentPolicyTx(tx, ctx); err != nil {
			return AutomationFeatureState{}, err
		}
	}
	var currentRevision int64
	err = tx.QueryRow(`SELECT revision FROM automation_feature_state
WHERE owner_orbit_id = ?`, ctx.OrbitID).Scan(&currentRevision)
	if errors.Is(err, sql.ErrNoRows) {
		if params.ExpectedRevision != 0 {
			return AutomationFeatureState{}, ErrAutomationStateConflict
		}
		currentRevision = 0
	} else if err != nil {
		return AutomationFeatureState{}, err
	} else if currentRevision != params.ExpectedRevision {
		return AutomationFeatureState{}, ErrAutomationStateConflict
	}
	emergencyAt := int64(0)
	if params.EmergencyDisabled {
		emergencyAt = params.OccurredAt
	}
	if currentRevision == 0 {
		_, err = tx.Exec(`INSERT INTO automation_feature_state(
  owner_orbit_id, soundboard_enabled, automation_enabled, emergency_disabled,
  timezone, quiet_hours_json, quiet_hours_hash, policy_version, revision,
  updated_by_actor_id, updated_at, emergency_disabled_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`, ctx.OrbitID,
			boolInt(params.SoundboardEnabled), boolInt(params.AutomationEnabled),
			boolInt(params.EmergencyDisabled), params.Timezone, quietJSON, quietHash,
			automationcontract.ContractVersion, ctx.ActorID, params.OccurredAt, emergencyAt)
	} else {
		_, err = tx.Exec(`UPDATE automation_feature_state SET
  soundboard_enabled = ?, automation_enabled = ?, emergency_disabled = ?,
  timezone = ?, quiet_hours_json = ?, quiet_hours_hash = ?, revision = revision + 1,
  updated_by_actor_id = ?, updated_at = ?, emergency_disabled_at = ?
WHERE owner_orbit_id = ? AND revision = ?`, boolInt(params.SoundboardEnabled),
			boolInt(params.AutomationEnabled), boolInt(params.EmergencyDisabled),
			params.Timezone, quietJSON, quietHash, ctx.ActorID, params.OccurredAt,
			emergencyAt, ctx.OrbitID, params.ExpectedRevision)
	}
	if err != nil {
		return AutomationFeatureState{}, err
	}
	state, err := scanAutomationFeatureState(tx.QueryRow(`SELECT `+automationFeatureColumns+`
FROM automation_feature_state WHERE owner_orbit_id = ?`, ctx.OrbitID))
	if err != nil {
		return AutomationFeatureState{}, err
	}
	if err := s.checkpoint("automation_feature_state_before_commit"); err != nil {
		return AutomationFeatureState{}, err
	}
	if err := tx.Commit(); err != nil {
		return AutomationFeatureState{}, err
	}
	return state, nil
}

type AutomationSchedule struct {
	ID               string
	OwnerOrbitID     int64
	CreatedByActorID int64
	CueID            string
	DisplayName      string
	Timezone         string
	WeekdaysMask     int
	LocalMinute      int
	AudienceKind     automationcontract.AudienceKind
	SelectorDigest   string
	BoundAirID       string
	Delivery         automationcontract.Delivery
	PolicyVersion    string
	PolicyRevision   int64
	Enabled          bool
	Revision         int64
	CreatedAt        int64
	UpdatedAt        int64
	DisabledAt       int64
}

type CreateAutomationScheduleParams struct {
	ExpectedActorID int64
	Bearer          string
	CueID           string
	DisplayName     string
	Timezone        string
	WeekdaysMask    int
	LocalMinute     int
	AudienceKind    automationcontract.AudienceKind
	SelectorDigest  string
	BoundAirID      string
	PolicyRevision  int64
	CreatedAt       int64
}

const automationScheduleColumns = `id, owner_orbit_id, created_by_actor_id,
cue_id, display_name, timezone, weekdays_mask, local_minute, audience_kind,
selector_digest, bound_air_id, delivery, policy_version, policy_revision,
enabled, revision, created_at, updated_at, disabled_at`

func scanAutomationSchedule(row interface{ Scan(...any) error }) (AutomationSchedule, error) {
	var schedule AutomationSchedule
	var enabled int
	err := row.Scan(&schedule.ID, &schedule.OwnerOrbitID, &schedule.CreatedByActorID,
		&schedule.CueID, &schedule.DisplayName, &schedule.Timezone,
		&schedule.WeekdaysMask, &schedule.LocalMinute, &schedule.AudienceKind,
		&schedule.SelectorDigest, &schedule.BoundAirID, &schedule.Delivery,
		&schedule.PolicyVersion, &schedule.PolicyRevision, &enabled,
		&schedule.Revision, &schedule.CreatedAt, &schedule.UpdatedAt,
		&schedule.DisabledAt)
	schedule.Enabled = enabled != 0
	return schedule, err
}

func validAutomationAudience(kind automationcontract.AudienceKind, selector, airID string) bool {
	switch kind {
	case automationcontract.AudienceOwnBarycenter:
		return selector == "" && airID == ""
	case automationcontract.AudienceCurrentAir:
		return selector == "" && airPolicyAirIDPattern.MatchString(airID)
	case automationcontract.AudienceExplicit:
		return lowerHexTokenPattern.MatchString(selector) && airID == ""
	default:
		return false
	}
}

func automationBoundAirActiveTx(tx *sql.Tx, airID string, orbitID int64) (bool, error) {
	var active int
	err := tx.QueryRow(`SELECT EXISTS(
  SELECT 1 FROM airs a JOIN air_members m ON m.air_id = a.public_id
  WHERE a.public_id = ? AND a.status IN ('parked', 'active')
    AND m.orbit_id = ? AND m.status = 'joined'
)`, airID, orbitID).Scan(&active)
	return active != 0, err
}

func (s *Store) CreateAutomationSchedule(params CreateAutomationScheduleParams) (AutomationSchedule, error) {
	name := strings.TrimSpace(params.DisplayName)
	if name == "" || len(name) > 128 || params.WeekdaysMask < 1 ||
		params.WeekdaysMask > 127 || params.LocalMinute < 0 || params.LocalMinute > 1439 ||
		params.PolicyRevision <= 0 || params.CreatedAt <= 0 ||
		!savedCueIDPattern.MatchString(params.CueID) ||
		!validAutomationAudience(params.AudienceKind, params.SelectorDigest, params.BoundAirID) {
		return AutomationSchedule{}, ErrAutomationInvalid
	}
	if _, err := time.LoadLocation(params.Timezone); err != nil {
		return AutomationSchedule{}, ErrAutomationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AutomationSchedule{}, err
	}
	defer tx.Rollback()
	ctx, err := authorizeSavedCueMutationTx(tx, params.ExpectedActorID, params.Bearer)
	if err != nil {
		return AutomationSchedule{}, err
	}
	if err := requireCurrentContentPolicyTx(tx, ctx); err != nil {
		return AutomationSchedule{}, err
	}
	if _, err := savedCueOwnedActiveTx(tx, params.CueID, ctx.OrbitID); err != nil {
		return AutomationSchedule{}, err
	}
	if params.AudienceKind == automationcontract.AudienceCurrentAir {
		active, err := automationBoundAirActiveTx(tx, params.BoundAirID, ctx.OrbitID)
		if err != nil {
			return AutomationSchedule{}, err
		}
		if !active {
			return AutomationSchedule{}, ErrInsufficientCapability
		}
	}
	var featureRevision int64
	if err := tx.QueryRow(`SELECT revision FROM automation_feature_state
WHERE owner_orbit_id = ?`, ctx.OrbitID).Scan(&featureRevision); errors.Is(err, sql.ErrNoRows) {
		return AutomationSchedule{}, ErrAutomationDisabled
	} else if err != nil {
		return AutomationSchedule{}, err
	}
	if featureRevision != params.PolicyRevision {
		return AutomationSchedule{}, ErrAutomationStateConflict
	}
	id := ulid.NewAutomationScheduleID(time.UnixMilli(params.CreatedAt))
	_, err = tx.Exec(`INSERT INTO automation_schedules(
  id, owner_orbit_id, created_by_actor_id, cue_id, display_name, timezone,
  weekdays_mask, local_minute, audience_kind, selector_digest, bound_air_id,
  delivery, policy_version, policy_revision, enabled, revision, created_at, updated_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'overlay', ?, ?, 1, 1, ?, ?)`,
		id, ctx.OrbitID, ctx.ActorID, params.CueID, name, params.Timezone,
		params.WeekdaysMask, params.LocalMinute, params.AudienceKind,
		params.SelectorDigest, params.BoundAirID, automationcontract.ContractVersion,
		params.PolicyRevision, params.CreatedAt, params.CreatedAt)
	if err != nil {
		return AutomationSchedule{}, err
	}
	schedule, err := scanAutomationSchedule(tx.QueryRow(`SELECT `+automationScheduleColumns+`
FROM automation_schedules WHERE id = ?`, id))
	if err != nil {
		return AutomationSchedule{}, err
	}
	if err := s.checkpoint("automation_schedule_create_before_commit"); err != nil {
		return AutomationSchedule{}, err
	}
	if err := tx.Commit(); err != nil {
		return AutomationSchedule{}, err
	}
	return schedule, nil
}

func savedCueOwnedActiveTx(tx *sql.Tx, cueID string, orbitID int64) (SavedCue, error) {
	cue, err := savedCueOwnedTx(tx, cueID, orbitID)
	if err != nil || cue.State != SavedCueActive {
		if err != nil {
			return SavedCue{}, err
		}
		return SavedCue{}, ErrSavedCueNotFound
	}
	return cue, nil
}

func automationActorActiveTx(tx *sql.Tx, actorID, orbitID int64) (bool, error) {
	var active int
	err := tx.QueryRow(`SELECT EXISTS(
  SELECT 1 FROM actors a JOIN memberships m ON m.actor_id = a.id
  JOIN orbits o ON o.id = m.orbit_id
  WHERE a.id = ? AND a.revoked_at IS NULL AND m.orbit_id = ?
    AND m.left_at IS NULL AND o.status = 'active'
)`, actorID, orbitID).Scan(&active)
	return active != 0, err
}

func (s *Store) DisableAutomationSchedule(expectedActorID int64, bearer, scheduleID string, expectedRevision, now int64) (AutomationSchedule, error) {
	if !automationScheduleIDPattern.MatchString(scheduleID) || expectedRevision <= 0 || now <= 0 {
		return AutomationSchedule{}, ErrAutomationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AutomationSchedule{}, err
	}
	defer tx.Rollback()
	ctx, err := authorizeSavedCueMutationTx(tx, expectedActorID, bearer)
	if err != nil {
		return AutomationSchedule{}, err
	}
	schedule, err := scanAutomationSchedule(tx.QueryRow(`SELECT `+automationScheduleColumns+`
FROM automation_schedules WHERE id = ? AND owner_orbit_id = ?`, scheduleID, ctx.OrbitID))
	if errors.Is(err, sql.ErrNoRows) {
		return AutomationSchedule{}, ErrAutomationNotFound
	}
	if err != nil {
		return AutomationSchedule{}, err
	}
	if !schedule.Enabled {
		return schedule, tx.Commit()
	}
	if schedule.Revision != expectedRevision {
		return AutomationSchedule{}, ErrAutomationStateConflict
	}
	if _, err := tx.Exec(`UPDATE automation_schedules SET enabled = 0,
revision = revision + 1, updated_at = ?, disabled_at = ? WHERE id = ? AND revision = ?`,
		now, now, scheduleID, expectedRevision); err != nil {
		return AutomationSchedule{}, err
	}
	schedule, err = scanAutomationSchedule(tx.QueryRow(`SELECT `+automationScheduleColumns+`
FROM automation_schedules WHERE id = ?`, scheduleID))
	if err != nil {
		return AutomationSchedule{}, err
	}
	if err := tx.Commit(); err != nil {
		return AutomationSchedule{}, err
	}
	return schedule, nil
}

type AutomationPrincipal struct {
	ID                string
	OwnerOrbitID      int64
	IssuedByActorID   int64
	DisplayName       string
	Permission        string
	BoundAirID        string
	MaxTargetCount    int
	IssuedAt          int64
	ExpiresAt         int64
	DisabledAt        int64
	DisabledByActorID int64
	RevokedAt         int64
	RevokedByActorID  int64
	Revision          int64
	AllowedCueIDs     []string
	AllowedAudiences  []automationcontract.AudienceKind
	TargetRefDigests  []string
}

type AutomationPrincipalIssue struct {
	Principal AutomationPrincipal
	Secret    string
}

type IssueAutomationPrincipalParams struct {
	ExpectedActorID  int64
	Bearer           string
	DisplayName      string
	AllowedCueIDs    []string
	AllowedAudiences []automationcontract.AudienceKind
	TargetRefDigests []string
	BoundAirID       string
	MaxTargetCount   int
	IssuedAt         int64
	ExpiresAt        int64
}

const automationPrincipalColumns = `id, owner_orbit_id, issued_by_actor_id,
display_name, permission, bound_air_id, max_target_count, issued_at, expires_at,
disabled_at, disabled_by_actor_id, revoked_at, revoked_by_actor_id, revision`

func scanAutomationPrincipal(row interface{ Scan(...any) error }) (AutomationPrincipal, error) {
	var principal AutomationPrincipal
	err := row.Scan(&principal.ID, &principal.OwnerOrbitID, &principal.IssuedByActorID,
		&principal.DisplayName, &principal.Permission, &principal.BoundAirID,
		&principal.MaxTargetCount, &principal.IssuedAt, &principal.ExpiresAt,
		&principal.DisabledAt, &principal.DisabledByActorID, &principal.RevokedAt,
		&principal.RevokedByActorID, &principal.Revision)
	return principal, err
}

func uniqueSortedStrings(values []string) ([]string, bool) {
	if len(values) == 0 {
		return nil, false
	}
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	sort.Strings(result)
	return result, true
}

func normalizeAutomationAudiences(values []automationcontract.AudienceKind) ([]automationcontract.AudienceKind, bool) {
	seen := make(map[automationcontract.AudienceKind]struct{}, len(values))
	result := make([]automationcontract.AudienceKind, 0, len(values))
	for _, value := range values {
		if value != automationcontract.AudienceOwnBarycenter &&
			value != automationcontract.AudienceCurrentAir &&
			value != automationcontract.AudienceExplicit {
			return nil, false
		}
		if _, exists := seen[value]; !exists {
			seen[value] = struct{}{}
			result = append(result, value)
		}
	}
	sort.Slice(result, func(i, j int) bool { return result[i] < result[j] })
	return result, len(result) > 0
}

func newAutomationSecret() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return hex.EncodeToString(raw), nil
}

func automationSecretHash(secret string) string {
	return hashToken("barycenter/automation-principal/sha256-domain-v1:" + secret)
}

func loadAutomationPrincipalScopesTx(tx *sql.Tx, principal *AutomationPrincipal) error {
	rows, err := tx.Query(`SELECT cue_id FROM automation_principal_cues
WHERE principal_id = ? ORDER BY cue_id`, principal.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			rows.Close()
			return err
		}
		principal.AllowedCueIDs = append(principal.AllowedCueIDs, value)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	rows, err = tx.Query(`SELECT audience_kind FROM automation_principal_audiences
WHERE principal_id = ? ORDER BY audience_kind`, principal.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var value automationcontract.AudienceKind
		if err := rows.Scan(&value); err != nil {
			rows.Close()
			return err
		}
		principal.AllowedAudiences = append(principal.AllowedAudiences, value)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	rows, err = tx.Query(`SELECT target_ref_digest FROM automation_principal_target_refs
WHERE principal_id = ? ORDER BY target_ref_digest`, principal.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			rows.Close()
			return err
		}
		principal.TargetRefDigests = append(principal.TargetRefDigests, value)
	}
	return rows.Close()
}

func (s *Store) IssueAutomationPrincipal(params IssueAutomationPrincipalParams) (AutomationPrincipalIssue, error) {
	name := strings.TrimSpace(params.DisplayName)
	cues, cueOK := uniqueSortedStrings(params.AllowedCueIDs)
	audiences, audienceOK := normalizeAutomationAudiences(params.AllowedAudiences)
	targets, targetOK := uniqueSortedStrings(params.TargetRefDigests)
	if name == "" || len(name) > 128 || !cueOK || !audienceOK ||
		params.MaxTargetCount < 1 || params.MaxTargetCount > automationcontract.MaxExplicitSelectors ||
		params.IssuedAt <= 0 || params.ExpiresAt <= params.IssuedAt ||
		params.ExpiresAt-params.IssuedAt > int64((90*24*time.Hour)/time.Millisecond) {
		return AutomationPrincipalIssue{}, ErrAutomationInvalid
	}
	hasAir, hasExplicit := false, false
	for _, audience := range audiences {
		hasAir = hasAir || audience == automationcontract.AudienceCurrentAir
		hasExplicit = hasExplicit || audience == automationcontract.AudienceExplicit
	}
	if hasAir != (params.BoundAirID != "") || hasExplicit != targetOK {
		return AutomationPrincipalIssue{}, ErrAutomationInvalid
	}
	if hasAir && !airPolicyAirIDPattern.MatchString(params.BoundAirID) {
		return AutomationPrincipalIssue{}, ErrAutomationInvalid
	}
	for _, cueID := range cues {
		if !savedCueIDPattern.MatchString(cueID) {
			return AutomationPrincipalIssue{}, ErrAutomationInvalid
		}
	}
	for _, digest := range targets {
		if !lowerHexTokenPattern.MatchString(digest) {
			return AutomationPrincipalIssue{}, ErrAutomationInvalid
		}
	}
	secret, err := newAutomationSecret()
	if err != nil {
		return AutomationPrincipalIssue{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AutomationPrincipalIssue{}, err
	}
	defer tx.Rollback()
	ctx, err := authorizeSavedCueMutationTx(tx, params.ExpectedActorID, params.Bearer)
	if err != nil {
		return AutomationPrincipalIssue{}, err
	}
	if err := requireCurrentContentPolicyTx(tx, ctx); err != nil {
		return AutomationPrincipalIssue{}, err
	}
	for _, cueID := range cues {
		if _, err := savedCueOwnedActiveTx(tx, cueID, ctx.OrbitID); err != nil {
			return AutomationPrincipalIssue{}, err
		}
	}
	if hasAir {
		active, err := automationBoundAirActiveTx(tx, params.BoundAirID, ctx.OrbitID)
		if err != nil {
			return AutomationPrincipalIssue{}, err
		}
		if !active {
			return AutomationPrincipalIssue{}, ErrInsufficientCapability
		}
	}
	id := ulid.NewAutomationPrincipalID(time.UnixMilli(params.IssuedAt))
	_, err = tx.Exec(`INSERT INTO automation_principals(
  id, owner_orbit_id, issued_by_actor_id, display_name, secret_hash,
  secret_hash_version, permission, bound_air_id, max_target_count,
  issued_at, expires_at
) VALUES(?, ?, ?, ?, ?, ?, 'automation:trigger', ?, ?, ?, ?)`, id,
		ctx.OrbitID, ctx.ActorID, name, automationSecretHash(secret),
		AutomationSecretHashVersion, params.BoundAirID, params.MaxTargetCount,
		params.IssuedAt, params.ExpiresAt)
	if err != nil {
		return AutomationPrincipalIssue{}, err
	}
	for _, cueID := range cues {
		if _, err := tx.Exec(`INSERT INTO automation_principal_cues(principal_id, cue_id)
VALUES(?, ?)`, id, cueID); err != nil {
			return AutomationPrincipalIssue{}, err
		}
	}
	for _, audience := range audiences {
		if _, err := tx.Exec(`INSERT INTO automation_principal_audiences(principal_id, audience_kind)
VALUES(?, ?)`, id, audience); err != nil {
			return AutomationPrincipalIssue{}, err
		}
	}
	for _, digest := range targets {
		if _, err := tx.Exec(`INSERT INTO automation_principal_target_refs(principal_id, target_ref_digest)
VALUES(?, ?)`, id, digest); err != nil {
			return AutomationPrincipalIssue{}, err
		}
	}
	principal, err := scanAutomationPrincipal(tx.QueryRow(`SELECT `+automationPrincipalColumns+`
FROM automation_principals WHERE id = ?`, id))
	if err != nil {
		return AutomationPrincipalIssue{}, err
	}
	if err := loadAutomationPrincipalScopesTx(tx, &principal); err != nil {
		return AutomationPrincipalIssue{}, err
	}
	if err := s.checkpoint("automation_principal_issue_before_commit"); err != nil {
		return AutomationPrincipalIssue{}, err
	}
	if err := tx.Commit(); err != nil {
		return AutomationPrincipalIssue{}, err
	}
	return AutomationPrincipalIssue{Principal: principal, Secret: secret}, nil
}

func resolveAutomationPrincipalSecretTx(tx *sql.Tx, secret string, now int64) (AutomationPrincipal, error) {
	if len(secret) != 64 || now <= 0 {
		return AutomationPrincipal{}, ErrAutomationInvalidCredential
	}
	principal, err := scanAutomationPrincipal(tx.QueryRow(`SELECT `+automationPrincipalColumns+`
FROM automation_principals WHERE secret_hash = ? AND secret_hash_version = ?`,
		automationSecretHash(secret), AutomationSecretHashVersion))
	if errors.Is(err, sql.ErrNoRows) {
		return AutomationPrincipal{}, ErrAutomationInvalidCredential
	}
	if err != nil {
		return AutomationPrincipal{}, err
	}
	if principal.IssuedAt > now || principal.RevokedAt != 0 ||
		principal.DisabledAt != 0 || principal.ExpiresAt <= now {
		return AutomationPrincipal{}, ErrAutomationInvalidCredential
	}
	active, err := automationActorActiveTx(tx, principal.IssuedByActorID, principal.OwnerOrbitID)
	if err != nil {
		return AutomationPrincipal{}, err
	}
	if !active {
		return AutomationPrincipal{}, ErrAutomationInvalidCredential
	}
	if err := loadAutomationPrincipalScopesTx(tx, &principal); err != nil {
		return AutomationPrincipal{}, err
	}
	return principal, nil
}

func (s *Store) ResolveAutomationPrincipalSecret(secret string, now int64) (AutomationPrincipal, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return AutomationPrincipal{}, err
	}
	defer tx.Rollback()
	principal, err := resolveAutomationPrincipalSecretTx(tx, secret, now)
	if err != nil {
		return AutomationPrincipal{}, err
	}
	return principal, tx.Commit()
}

func (s *Store) RevokeAutomationPrincipal(expectedActorID int64, bearer, principalID string, expectedRevision, now int64) (AutomationPrincipal, error) {
	if !automationPrincipalIDPattern.MatchString(principalID) || expectedRevision <= 0 || now <= 0 {
		return AutomationPrincipal{}, ErrAutomationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AutomationPrincipal{}, err
	}
	defer tx.Rollback()
	ctx, err := authorizeSavedCueMutationTx(tx, expectedActorID, bearer)
	if err != nil {
		return AutomationPrincipal{}, err
	}
	principal, err := scanAutomationPrincipal(tx.QueryRow(`SELECT `+automationPrincipalColumns+`
FROM automation_principals WHERE id = ? AND owner_orbit_id = ?`, principalID, ctx.OrbitID))
	if errors.Is(err, sql.ErrNoRows) {
		return AutomationPrincipal{}, ErrAutomationNotFound
	}
	if err != nil {
		return AutomationPrincipal{}, err
	}
	if principal.RevokedAt != 0 {
		return principal, tx.Commit()
	}
	if principal.Revision != expectedRevision {
		return AutomationPrincipal{}, ErrAutomationStateConflict
	}
	if _, err := tx.Exec(`UPDATE automation_principals SET revoked_at = ?,
revoked_by_actor_id = ?, revision = revision + 1 WHERE id = ? AND revision = ?`,
		now, ctx.ActorID, principalID, expectedRevision); err != nil {
		return AutomationPrincipal{}, err
	}
	principal, err = scanAutomationPrincipal(tx.QueryRow(`SELECT `+automationPrincipalColumns+`
FROM automation_principals WHERE id = ?`, principalID))
	if err != nil {
		return AutomationPrincipal{}, err
	}
	if err := loadAutomationPrincipalScopesTx(tx, &principal); err != nil {
		return AutomationPrincipal{}, err
	}
	if err := s.checkpoint("automation_principal_revoke_before_commit"); err != nil {
		return AutomationPrincipal{}, err
	}
	if err := tx.Commit(); err != nil {
		return AutomationPrincipal{}, err
	}
	return principal, nil
}

type AutomationExecution struct {
	ID                   string
	TriggerKind          automationcontract.TriggerKind
	OwnerOrbitID         int64
	PrincipalID          string
	ScheduleID           string
	ScheduleRevision     int64
	IssuedByActorID      int64
	CueID                string
	CueRevision          int64
	CueSourceGeneration  int64
	MediaIdentity        string
	AudienceKind         automationcontract.AudienceKind
	SelectorDigest       string
	BoundAirID           string
	TargetSnapshotDigest string
	ResolvedTargetCount  int
	Delivery             automationcontract.Delivery
	IdempotencyDigest    string
	RequestDigest        string
	OccurrenceKey        string
	ScheduledLocalDate   string
	ScheduledLocalMinute int
	ScheduledUTC         int64
	FeatureRevision      int64
	PolicyRevision       int64
	ClaimedAt            int64
	TransmissionID       string
	Status               string
	Outcome              string
	ReasonCode           string
	RetryGeneration      int64
	LeaseOwnerHash       string
	LeaseGeneration      int64
	LeaseExpiresAt       int64
	CompletedAt          int64
	RetentionExpiresAt   int64
	Revision             int64
}

const automationExecutionColumns = `id, trigger_kind, owner_orbit_id,
COALESCE(principal_id, ''), COALESCE(schedule_id, ''), schedule_revision,
issued_by_actor_id, cue_id, cue_revision, cue_source_generation, media_identity,
audience_kind, selector_digest, target_snapshot_digest, resolved_target_count,
bound_air_id,
delivery, idempotency_digest, request_digest, occurrence_key,
scheduled_local_date, scheduled_local_minute, scheduled_utc, feature_revision,
policy_revision, claimed_at, COALESCE(transmission_id, ''), status, outcome,
reason_code, retry_generation, lease_owner_hash, lease_generation,
lease_expires_at, completed_at, retention_expires_at, revision`

func scanAutomationExecution(row interface{ Scan(...any) error }) (AutomationExecution, error) {
	var execution AutomationExecution
	err := row.Scan(&execution.ID, &execution.TriggerKind, &execution.OwnerOrbitID,
		&execution.PrincipalID, &execution.ScheduleID, &execution.ScheduleRevision,
		&execution.IssuedByActorID, &execution.CueID, &execution.CueRevision,
		&execution.CueSourceGeneration, &execution.MediaIdentity,
		&execution.AudienceKind, &execution.SelectorDigest,
		&execution.TargetSnapshotDigest, &execution.ResolvedTargetCount,
		&execution.BoundAirID,
		&execution.Delivery, &execution.IdempotencyDigest, &execution.RequestDigest,
		&execution.OccurrenceKey, &execution.ScheduledLocalDate,
		&execution.ScheduledLocalMinute, &execution.ScheduledUTC,
		&execution.FeatureRevision, &execution.PolicyRevision, &execution.ClaimedAt,
		&execution.TransmissionID, &execution.Status, &execution.Outcome,
		&execution.ReasonCode, &execution.RetryGeneration, &execution.LeaseOwnerHash,
		&execution.LeaseGeneration, &execution.LeaseExpiresAt, &execution.CompletedAt,
		&execution.RetentionExpiresAt, &execution.Revision)
	return execution, err
}

func savedCueMediaIdentity(cue SavedCue) string {
	if cue.SourceKind == SavedCueSourceBuiltin {
		return cue.BuiltinSHA256
	}
	return cue.MediaID
}

func weekdayAllowed(mask int, weekday time.Weekday) bool {
	return mask&(1<<int(weekday)) != 0
}

func canonicalScheduledMinute(schedule AutomationSchedule, scheduledUTC, now int64) (time.Time, error) {
	if scheduledUTC <= 0 || scheduledUTC%int64(time.Minute/time.Millisecond) != 0 ||
		now < scheduledUTC || now >= scheduledUTC+int64(time.Minute/time.Millisecond) {
		return time.Time{}, ErrAutomationOccurrenceNotCurrent
	}
	location, err := time.LoadLocation(schedule.Timezone)
	if err != nil {
		return time.Time{}, ErrAutomationInvalid
	}
	candidate := time.UnixMilli(scheduledUTC).UTC()
	local := candidate.In(location)
	minute := local.Hour()*60 + local.Minute()
	if minute != schedule.LocalMinute || !weekdayAllowed(schedule.WeekdaysMask, local.Weekday()) {
		return time.Time{}, ErrAutomationOccurrenceNotCurrent
	}
	// Enumerating the bounded timezone-offset window avoids relying on
	// time.Date's implementation-defined choice during a fall-back fold. Only
	// the earliest UTC instant mapping to this local minute is canonical.
	for probe := candidate.Add(-3 * time.Hour); probe.Before(candidate); probe = probe.Add(time.Minute) {
		mapped := probe.In(location)
		if mapped.Year() == local.Year() && mapped.Month() == local.Month() &&
			mapped.Day() == local.Day() && mapped.Hour() == local.Hour() &&
			mapped.Minute() == local.Minute() {
			return time.Time{}, ErrAutomationOccurrenceLaterFold
		}
	}
	return local, nil
}

func (s *Store) ClaimScheduledAutomationOccurrence(scheduleID string, expectedRevision, scheduledUTC, now int64) (AutomationExecution, bool, error) {
	if !automationScheduleIDPattern.MatchString(scheduleID) || expectedRevision <= 0 {
		return AutomationExecution{}, false, ErrAutomationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AutomationExecution{}, false, err
	}
	defer tx.Rollback()
	schedule, err := scanAutomationSchedule(tx.QueryRow(`SELECT `+automationScheduleColumns+`
FROM automation_schedules WHERE id = ?`, scheduleID))
	if errors.Is(err, sql.ErrNoRows) {
		return AutomationExecution{}, false, ErrAutomationNotFound
	}
	if err != nil {
		return AutomationExecution{}, false, err
	}
	if !schedule.Enabled {
		return AutomationExecution{}, false, ErrAutomationDisabled
	}
	if schedule.Revision != expectedRevision {
		return AutomationExecution{}, false, ErrAutomationStateConflict
	}
	if scheduledUTC < schedule.CreatedAt {
		return AutomationExecution{}, false, ErrAutomationOccurrenceNotCurrent
	}
	creatorActive, err := automationActorActiveTx(tx, schedule.CreatedByActorID, schedule.OwnerOrbitID)
	if err != nil {
		return AutomationExecution{}, false, err
	}
	if !creatorActive {
		return AutomationExecution{}, false, ErrAutomationDisabled
	}
	feature, err := scanAutomationFeatureState(tx.QueryRow(`SELECT `+automationFeatureColumns+`
FROM automation_feature_state WHERE owner_orbit_id = ?`, schedule.OwnerOrbitID))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (!feature.AutomationEnabled || feature.EmergencyDisabled)) {
		return AutomationExecution{}, false, ErrAutomationDisabled
	}
	if err != nil {
		return AutomationExecution{}, false, err
	}
	cue, err := savedCueOwnedActiveTx(tx, schedule.CueID, schedule.OwnerOrbitID)
	if err != nil {
		return AutomationExecution{}, false, err
	}
	local, err := canonicalScheduledMinute(schedule, scheduledUTC, now)
	if err != nil {
		return AutomationExecution{}, false, err
	}
	localDate := local.Format("2006-01-02")
	occurrenceKey := fmt.Sprintf("%s/%d/%s/%02d:%02d", schedule.ID,
		schedule.Revision, localDate, local.Hour(), local.Minute())
	if existing, err := scanAutomationExecution(tx.QueryRow(`SELECT `+automationExecutionColumns+`
FROM automation_executions WHERE schedule_id = ? AND schedule_revision = ?
  AND scheduled_local_date = ? AND scheduled_local_minute = ?`, schedule.ID,
		schedule.Revision, localDate, schedule.LocalMinute)); err == nil {
		if err := tx.Commit(); err != nil {
			return AutomationExecution{}, false, err
		}
		return existing, true, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return AutomationExecution{}, false, err
	}
	id := ulid.NewAutomationExecutionID(time.UnixMilli(now))
	retention := now + AutomationExecutionRetention.Milliseconds()
	_, err = tx.Exec(`INSERT INTO automation_executions(
  id, trigger_kind, owner_orbit_id, schedule_id, schedule_revision,
  issued_by_actor_id, cue_id, cue_revision, cue_source_generation,
  media_identity, audience_kind, selector_digest, bound_air_id, delivery, occurrence_key,
  scheduled_local_date, scheduled_local_minute, scheduled_utc, feature_revision,
  policy_revision, claimed_at, retention_expires_at
) VALUES(?, 'schedule', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'overlay', ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, schedule.OwnerOrbitID, schedule.ID, schedule.Revision,
		schedule.CreatedByActorID, cue.ID, cue.Revision, cue.SourceGeneration,
		savedCueMediaIdentity(cue), schedule.AudienceKind, schedule.SelectorDigest,
		schedule.BoundAirID, occurrenceKey, localDate, schedule.LocalMinute, scheduledUTC,
		feature.Revision, schedule.PolicyRevision, now, retention)
	if err != nil {
		return AutomationExecution{}, false, err
	}
	execution, err := scanAutomationExecution(tx.QueryRow(`SELECT `+automationExecutionColumns+`
FROM automation_executions WHERE id = ?`, id))
	if err != nil {
		return AutomationExecution{}, false, err
	}
	if err := s.checkpoint("automation_schedule_occurrence_before_commit"); err != nil {
		return AutomationExecution{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return AutomationExecution{}, false, err
	}
	return execution, false, nil
}

type ClaimAutomationAPIExecutionParams struct {
	Secret                 string
	CueID                  string
	AudienceKind           automationcontract.AudienceKind
	TargetReferenceDigests []string
	IdempotencyKey         string
	RequestDigest          string
	ClaimedAt              int64
}

func validAutomationIdempotencyKey(value string) bool {
	if len(value) < 8 || len(value) > 512 {
		return false
	}
	for _, char := range []byte(value) {
		if char < 0x21 || char > 0x7e {
			return false
		}
	}
	return true
}

func digestTargetReferences(values []string) (string, []string, bool) {
	if len(values) == 0 {
		return "", nil, true
	}
	values, _ = uniqueSortedStrings(values)
	for _, value := range values {
		if !lowerHexTokenPattern.MatchString(value) {
			return "", nil, false
		}
	}
	digest := sha256.Sum256([]byte(strings.Join(values, "\n")))
	return hex.EncodeToString(digest[:]), values, true
}

func containsAudience(values []automationcontract.AudienceKind, wanted automationcontract.AudienceKind) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}

func containsString(values []string, wanted string) bool {
	index := sort.SearchStrings(values, wanted)
	return index < len(values) && values[index] == wanted
}

func (s *Store) ClaimAutomationAPIExecution(params ClaimAutomationAPIExecutionParams) (AutomationExecution, bool, error) {
	selectorDigest, targets, targetsOK := digestTargetReferences(params.TargetReferenceDigests)
	if !savedCueIDPattern.MatchString(params.CueID) || !validAutomationIdempotencyKey(params.IdempotencyKey) ||
		!lowerHexTokenPattern.MatchString(params.RequestDigest) || !targetsOK || params.ClaimedAt <= 0 {
		return AutomationExecution{}, false, ErrAutomationInvalid
	}
	if params.AudienceKind == automationcontract.AudienceExplicit && len(targets) == 0 {
		return AutomationExecution{}, false, ErrAutomationInvalid
	}
	if params.AudienceKind != automationcontract.AudienceExplicit && len(targets) != 0 {
		return AutomationExecution{}, false, ErrAutomationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AutomationExecution{}, false, err
	}
	defer tx.Rollback()
	principal, err := resolveAutomationPrincipalSecretTx(tx, params.Secret, params.ClaimedAt)
	if err != nil {
		return AutomationExecution{}, false, err
	}
	feature, err := scanAutomationFeatureState(tx.QueryRow(`SELECT `+automationFeatureColumns+`
FROM automation_feature_state WHERE owner_orbit_id = ?`, principal.OwnerOrbitID))
	if errors.Is(err, sql.ErrNoRows) || (err == nil && (!feature.AutomationEnabled || feature.EmergencyDisabled)) {
		return AutomationExecution{}, false, ErrAutomationDisabled
	}
	if err != nil {
		return AutomationExecution{}, false, err
	}
	if !containsString(principal.AllowedCueIDs, params.CueID) ||
		!containsAudience(principal.AllowedAudiences, params.AudienceKind) {
		return AutomationExecution{}, false, ErrInsufficientCapability
	}
	if params.AudienceKind == automationcontract.AudienceExplicit {
		if len(targets) > principal.MaxTargetCount {
			return AutomationExecution{}, false, ErrInsufficientCapability
		}
		for _, target := range targets {
			if !containsString(principal.TargetRefDigests, target) {
				return AutomationExecution{}, false, ErrInsufficientCapability
			}
		}
	}
	idempotencyDigest := hashToken("barycenter/automation-idempotency/v1:" + params.IdempotencyKey)
	if existing, err := scanAutomationExecution(tx.QueryRow(`SELECT `+automationExecutionColumns+`
FROM automation_executions WHERE principal_id = ? AND idempotency_digest = ?`,
		principal.ID, idempotencyDigest)); err == nil {
		if existing.RequestDigest != params.RequestDigest {
			return AutomationExecution{}, false, ErrAutomationIdempotencyConflict
		}
		if err := tx.Commit(); err != nil {
			return AutomationExecution{}, false, err
		}
		return existing, true, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return AutomationExecution{}, false, err
	}
	cue, err := savedCueOwnedActiveTx(tx, params.CueID, principal.OwnerOrbitID)
	if err != nil {
		return AutomationExecution{}, false, err
	}
	id := ulid.NewAutomationExecutionID(time.UnixMilli(params.ClaimedAt))
	retention := params.ClaimedAt + AutomationExecutionRetention.Milliseconds()
	_, err = tx.Exec(`INSERT INTO automation_executions(
  id, trigger_kind, owner_orbit_id, principal_id, issued_by_actor_id, cue_id,
  cue_revision, cue_source_generation, media_identity, audience_kind,
  selector_digest, bound_air_id, delivery, idempotency_digest, request_digest,
  feature_revision, policy_revision, claimed_at, retention_expires_at
) VALUES(?, 'scoped_api', ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'overlay', ?, ?, ?, ?, ?, ?)`,
		id, principal.OwnerOrbitID, principal.ID, principal.IssuedByActorID,
		cue.ID, cue.Revision, cue.SourceGeneration, savedCueMediaIdentity(cue),
		params.AudienceKind, selectorDigest, principal.BoundAirID, idempotencyDigest,
		params.RequestDigest, feature.Revision, feature.Revision,
		params.ClaimedAt, retention)
	if err != nil {
		return AutomationExecution{}, false, err
	}
	execution, err := scanAutomationExecution(tx.QueryRow(`SELECT `+automationExecutionColumns+`
FROM automation_executions WHERE id = ?`, id))
	if err != nil {
		return AutomationExecution{}, false, err
	}
	if err := s.checkpoint("automation_api_execution_before_commit"); err != nil {
		return AutomationExecution{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return AutomationExecution{}, false, err
	}
	return execution, false, nil
}

func (s *Store) ClaimAutomationExecutionLease(executionID string, expectedRetryGeneration int64, workerID string, now, leaseExpiresAt int64) (AutomationExecution, error) {
	if !automationExecutionIDPattern.MatchString(executionID) || expectedRetryGeneration < 0 ||
		workerID == "" || now <= 0 || leaseExpiresAt <= now ||
		leaseExpiresAt-now > AutomationMaxLease.Milliseconds() {
		return AutomationExecution{}, ErrAutomationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AutomationExecution{}, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE automation_executions SET status = 'leased',
lease_owner_hash = ?, lease_generation = lease_generation + 1,
lease_expires_at = ?, revision = revision + 1
WHERE id = ? AND status = 'claimed' AND retry_generation = ?`,
		hashToken("barycenter/automation-worker/v1:"+workerID), leaseExpiresAt,
		executionID, expectedRetryGeneration)
	if err != nil {
		return AutomationExecution{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return AutomationExecution{}, err
		}
		return AutomationExecution{}, ErrAutomationStateConflict
	}
	execution, err := scanAutomationExecution(tx.QueryRow(`SELECT `+automationExecutionColumns+`
FROM automation_executions WHERE id = ?`, executionID))
	if err != nil {
		return AutomationExecution{}, err
	}
	if err := s.checkpoint("automation_execution_lease_before_commit"); err != nil {
		return AutomationExecution{}, err
	}
	if err := tx.Commit(); err != nil {
		return AutomationExecution{}, err
	}
	return execution, nil
}

func (s *Store) ReleaseAutomationExecutionLease(executionID string, leaseGeneration int64, workerID string, now int64) (AutomationExecution, error) {
	if !automationExecutionIDPattern.MatchString(executionID) || leaseGeneration <= 0 ||
		workerID == "" || now <= 0 {
		return AutomationExecution{}, ErrAutomationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return AutomationExecution{}, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE automation_executions SET status = 'claimed',
lease_owner_hash = '', lease_expires_at = 0, retry_generation = retry_generation + 1,
revision = revision + 1 WHERE id = ? AND status = 'leased'
  AND lease_generation = ? AND lease_owner_hash = ?`, executionID, leaseGeneration,
		hashToken("barycenter/automation-worker/v1:"+workerID))
	if err != nil {
		return AutomationExecution{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return AutomationExecution{}, err
		}
		return AutomationExecution{}, ErrAutomationStateConflict
	}
	execution, err := scanAutomationExecution(tx.QueryRow(`SELECT `+automationExecutionColumns+`
FROM automation_executions WHERE id = ?`, executionID))
	if err != nil {
		return AutomationExecution{}, err
	}
	if err := tx.Commit(); err != nil {
		return AutomationExecution{}, err
	}
	return execution, nil
}

func (s *Store) ReconcileAutomationExecutionLeases(now int64) (int64, error) {
	if now <= 0 {
		return 0, ErrAutomationInvalid
	}
	result, err := s.db.Exec(`UPDATE automation_executions SET status = 'claimed',
lease_owner_hash = '', lease_expires_at = 0, retry_generation = retry_generation + 1,
revision = revision + 1 WHERE status = 'leased' AND lease_expires_at <= ?`, now)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

type AutomationCancellationCandidate struct {
	ExecutionID string
	Reason      string
	Status      string
}

func (s *Store) PendingAutomationCancellationCandidates(ownerOrbitID int64, limit int) ([]AutomationCancellationCandidate, error) {
	if ownerOrbitID <= 0 || limit <= 0 || limit > 1000 {
		return nil, ErrAutomationInvalid
	}
	rows, err := s.db.Query(`SELECT e.id,
CASE
  WHEN p.revoked_at > 0 THEN 'principal_revoked'
  WHEN p.disabled_at > 0 THEN 'principal_disabled'
  WHEN pa.revoked_at IS NOT NULL THEN 'principal_revoked'
  WHEN s.enabled = 0 THEN 'schedule_disabled'
  WHEN sa.revoked_at IS NOT NULL THEN 'schedule_disabled'
  WHEN f.automation_enabled = 0 OR f.emergency_disabled = 1 THEN 'automation_disabled'
  WHEN c.state <> 'active' THEN 'cue_not_ready'
END AS reason,
e.status
FROM automation_executions e
JOIN saved_cues c ON c.id = e.cue_id
JOIN automation_feature_state f ON f.owner_orbit_id = e.owner_orbit_id
LEFT JOIN automation_principals p ON p.id = e.principal_id
LEFT JOIN automation_schedules s ON s.id = e.schedule_id
LEFT JOIN actors pa ON pa.id = p.issued_by_actor_id
LEFT JOIN actors sa ON sa.id = s.created_by_actor_id
WHERE e.owner_orbit_id = ?
  AND e.status IN ('claimed', 'leased', 'accepted', 'cancelling')
  AND (p.revoked_at > 0 OR p.disabled_at > 0 OR pa.revoked_at IS NOT NULL
    OR s.enabled = 0 OR sa.revoked_at IS NOT NULL
    OR f.automation_enabled = 0 OR f.emergency_disabled = 1 OR c.state <> 'active')
ORDER BY e.claimed_at, e.id LIMIT ?`, ownerOrbitID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []AutomationCancellationCandidate
	for rows.Next() {
		var value AutomationCancellationCandidate
		if err := rows.Scan(&value.ExecutionID, &value.Reason, &value.Status); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

type AutomationLineageUsage struct {
	ActiveSchedules   int
	ActivePrincipals  int
	PendingExecutions int
}

func (s *Store) AutomationLineageUsage(ownerOrbitID int64, now int64) (AutomationLineageUsage, error) {
	if ownerOrbitID <= 0 || now <= 0 {
		return AutomationLineageUsage{}, ErrAutomationInvalid
	}
	var usage AutomationLineageUsage
	err := s.db.QueryRow(`SELECT
  (SELECT COUNT(*) FROM automation_schedules WHERE owner_orbit_id = ? AND enabled = 1),
  (SELECT COUNT(*) FROM automation_principals WHERE owner_orbit_id = ?
    AND revoked_at = 0 AND disabled_at = 0 AND expires_at > ?),
  (SELECT COUNT(*) FROM automation_executions WHERE owner_orbit_id = ?
    AND status IN ('claimed', 'leased', 'accepted', 'cancelling'))`, ownerOrbitID,
		ownerOrbitID, now, ownerOrbitID).Scan(&usage.ActiveSchedules,
		&usage.ActivePrincipals, &usage.PendingExecutions)
	return usage, err
}

func (s *Store) PruneAutomationExecutions(cutoff int64, limit int) (int64, error) {
	if cutoff <= 0 || limit <= 0 || limit > 1000 {
		return 0, ErrAutomationInvalid
	}
	result, err := s.db.Exec(`DELETE FROM automation_executions WHERE id IN (
  SELECT id FROM automation_executions
  WHERE status IN ('cancelled', 'completed', 'failed')
    AND retention_expires_at <= ?
  ORDER BY retention_expires_at, id LIMIT ?
)`, cutoff, limit)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}
