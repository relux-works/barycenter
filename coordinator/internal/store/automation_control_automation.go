package store

import (
	"database/sql"
	"errors"
	"strings"
	"time"

	automationcontract "relux.works/duet/coordinator/internal/automation"
	"relux.works/duet/coordinator/internal/ulid"
)

type AutomationFeatureControlParams struct {
	SoundboardEnabled bool
	AutomationEnabled bool
	EmergencyDisabled bool
	Timezone          string
	QuietHours        []AutomationQuietWindow
	ExpectedRevision  int64
}

type AutomationFeatureControlMutation struct {
	State    AutomationFeatureState `json:"state"`
	Replayed bool                   `json:"replayed"`
}

func (s *Store) ReplaceAuthorizedAutomationFeatureState(auth AutomationControlAuth, params AutomationFeatureControlParams) (AutomationFeatureControlMutation, error) {
	const operation = "automation.feature.replace.v1"
	state, auth, err := s.beginAutomationControlMutation(auth, operation)
	if err != nil {
		return AutomationFeatureControlMutation{}, err
	}
	defer state.tx.Rollback()
	if state.replayed {
		result, err := replayAutomationControlMutation[AutomationFeatureControlMutation](state)
		result.Replayed = true
		return result, err
	}
	if params.ExpectedRevision < 0 {
		return AutomationFeatureControlMutation{}, ErrAutomationInvalid
	}
	if params.Timezone != "" {
		if _, err := time.LoadLocation(params.Timezone); err != nil {
			return AutomationFeatureControlMutation{}, ErrAutomationInvalid
		}
	}
	_, quietJSON, quietHash, err := NormalizeAutomationQuietHours(params.QuietHours)
	if err != nil {
		return AutomationFeatureControlMutation{}, err
	}
	if params.AutomationEnabled && params.Timezone == "" {
		return AutomationFeatureControlMutation{}, ErrAutomationInvalid
	}
	if params.AutomationEnabled {
		if err := requireCurrentContentPolicyTx(state.tx, state.ctx); err != nil {
			return AutomationFeatureControlMutation{}, err
		}
	}
	var currentRevision int64
	err = state.tx.QueryRow(`SELECT revision FROM automation_feature_state
WHERE owner_orbit_id = ?`, state.ctx.OrbitID).Scan(&currentRevision)
	if errors.Is(err, sql.ErrNoRows) {
		currentRevision = 0
	} else if err != nil {
		return AutomationFeatureControlMutation{}, err
	}
	if currentRevision != params.ExpectedRevision {
		return AutomationFeatureControlMutation{}, ErrAutomationStateConflict
	}
	emergencyAt := int64(0)
	if params.EmergencyDisabled {
		emergencyAt = auth.Now
	}
	if currentRevision == 0 {
		_, err = state.tx.Exec(`INSERT INTO automation_feature_state(
  owner_orbit_id, soundboard_enabled, automation_enabled, emergency_disabled,
  timezone, quiet_hours_json, quiet_hours_hash, policy_version, revision,
  updated_by_actor_id, updated_at, emergency_disabled_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?)`, state.ctx.OrbitID,
			boolInt(params.SoundboardEnabled), boolInt(params.AutomationEnabled),
			boolInt(params.EmergencyDisabled), params.Timezone, quietJSON, quietHash,
			automationcontract.ContractVersion, state.ctx.ActorID, auth.Now, emergencyAt)
	} else {
		_, err = state.tx.Exec(`UPDATE automation_feature_state SET
  soundboard_enabled = ?, automation_enabled = ?, emergency_disabled = ?,
  timezone = ?, quiet_hours_json = ?, quiet_hours_hash = ?, revision = revision + 1,
  updated_by_actor_id = ?, updated_at = ?, emergency_disabled_at = ?
WHERE owner_orbit_id = ? AND revision = ?`, boolInt(params.SoundboardEnabled),
			boolInt(params.AutomationEnabled), boolInt(params.EmergencyDisabled),
			params.Timezone, quietJSON, quietHash, state.ctx.ActorID, auth.Now,
			emergencyAt, state.ctx.OrbitID, params.ExpectedRevision)
	}
	if err != nil {
		return AutomationFeatureControlMutation{}, err
	}
	// Any feature-policy revision invalidates armed schedules. They must be
	// explicitly reviewed/replaced against the new revision before re-enable,
	// so a policy edit cannot silently weaken already configured automation.
	if currentRevision > 0 {
		if _, err := state.tx.Exec(`UPDATE automation_schedules SET enabled = 0,
revision = revision + 1, updated_at = ?, disabled_at = ?
WHERE owner_orbit_id = ? AND enabled = 1`, auth.Now, auth.Now,
			state.ctx.OrbitID); err != nil {
			return AutomationFeatureControlMutation{}, err
		}
	}
	feature, err := scanAutomationFeatureState(state.tx.QueryRow(`SELECT `+automationFeatureColumns+`
FROM automation_feature_state WHERE owner_orbit_id = ?`, state.ctx.OrbitID))
	if err != nil {
		return AutomationFeatureControlMutation{}, err
	}
	result := AutomationFeatureControlMutation{State: feature}
	if err := s.checkpoint("automation_control_feature_before_commit"); err != nil {
		return AutomationFeatureControlMutation{}, err
	}
	if err := finishAutomationControlMutation(state, auth, operation, "", result); err != nil {
		return AutomationFeatureControlMutation{}, err
	}
	return result, nil
}

type AutomationScheduleControlParams struct {
	CueID                string
	DisplayName          string
	Timezone             string
	WeekdaysMask         int
	LocalMinute          int
	AudienceKind         automationcontract.AudienceKind
	TargetReferences     []string
	BoundAirID           string
	AdditionalQuietHours []AutomationQuietWindow
	PolicyRevision       int64
}

type AutomationScheduleControlMutation struct {
	Control  AutomationScheduleControl `json:"control"`
	Replayed bool                      `json:"replayed"`
}

func automationTargetSetDigest(scopes []AutomationTargetScope) string {
	values := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		values = append(values, scope.Digest)
	}
	return hashToken("barycenter/automation-target-set/v1:" + strings.Join(values, ":"))
}

func validateAutomationScheduleControlTx(tx *sql.Tx, ctx ActorContext, bearer string, params AutomationScheduleControlParams, now int64) (string, string, []AutomationQuietWindow, []AutomationTargetScope, string, error) {
	name := strings.TrimSpace(params.DisplayName)
	if name == "" || len(name) > 128 || params.WeekdaysMask < 1 ||
		params.WeekdaysMask > 127 || params.LocalMinute < 0 || params.LocalMinute > 1439 ||
		params.PolicyRevision <= 0 || !savedCueIDPattern.MatchString(params.CueID) {
		return "", "", nil, nil, "", ErrAutomationInvalid
	}
	if _, err := time.LoadLocation(params.Timezone); err != nil {
		return "", "", nil, nil, "", ErrAutomationInvalid
	}
	quiet, quietJSON, quietHash, err := NormalizeAutomationQuietHours(params.AdditionalQuietHours)
	if err != nil {
		return "", "", nil, nil, "", err
	}
	if _, err := savedCueOwnedActiveTx(tx, params.CueID, ctx.OrbitID); err != nil {
		return "", "", nil, nil, "", err
	}
	feature, err := scanAutomationFeatureState(tx.QueryRow(`SELECT `+automationFeatureColumns+`
FROM automation_feature_state WHERE owner_orbit_id = ?`, ctx.OrbitID))
	if errors.Is(err, sql.ErrNoRows) {
		return "", "", nil, nil, "", ErrAutomationDisabled
	} else if err != nil {
		return "", "", nil, nil, "", err
	}
	if !AutomationQuietPolicyValid(feature.Timezone, feature.QuietHoursJSON, feature.QuietHoursHash) {
		return "", "", nil, nil, "", ErrAutomationDisabled
	}
	if feature.Revision != params.PolicyRevision {
		return "", "", nil, nil, "", ErrAutomationStateConflict
	}
	var targets []AutomationTargetScope
	switch params.AudienceKind {
	case automationcontract.AudienceOwnBarycenter:
		if len(params.TargetReferences) != 0 || params.BoundAirID != "" {
			return "", "", nil, nil, "", ErrAutomationInvalid
		}
	case automationcontract.AudienceCurrentAir:
		if len(params.TargetReferences) != 0 || !airPolicyAirIDPattern.MatchString(params.BoundAirID) {
			return "", "", nil, nil, "", ErrAutomationInvalid
		}
		active, err := automationBoundAirActiveTx(tx, params.BoundAirID, ctx.OrbitID)
		if err != nil {
			return "", "", nil, nil, "", err
		}
		if !active {
			return "", "", nil, nil, "", ErrAutomationAudienceNotAllowed
		}
	case automationcontract.AudienceExplicit:
		if params.BoundAirID != "" {
			return "", "", nil, nil, "", ErrAutomationInvalid
		}
		targets, err = resolveAutomationTargetScopesTx(tx, ctx, bearer, params.TargetReferences, now)
		if err != nil {
			return "", "", nil, nil, "", err
		}
	default:
		return "", "", nil, nil, "", ErrAutomationInvalid
	}
	return name, quietJSON, quiet, targets, quietHash, nil
}

func insertAutomationScheduleTargetsTx(tx *sql.Tx, scheduleID string, targets []AutomationTargetScope) error {
	for _, target := range targets {
		if _, err := tx.Exec(`INSERT INTO automation_schedule_targets(
  schedule_id, target_digest, target_kind, target_orbit_id, target_actor_id,
  target_slot, target_binding_paired_at
) VALUES(?, ?, ?, ?, ?, ?, ?)`, scheduleID, target.Digest, target.Kind,
			target.OrbitID, target.ActorID, target.Slot, target.BindingPairedAt); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) CreateAuthorizedAutomationSchedule(auth AutomationControlAuth, params AutomationScheduleControlParams) (AutomationScheduleControlMutation, error) {
	const operation = "automation.schedule.create.v1"
	state, auth, err := s.beginAutomationControlMutation(auth, operation)
	if err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	defer state.tx.Rollback()
	if state.replayed {
		result, err := replayAutomationControlMutation[AutomationScheduleControlMutation](state)
		result.Replayed = true
		return result, err
	}
	if err := requireCurrentContentPolicyTx(state.tx, state.ctx); err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	name, quietJSON, quiet, targets, quietHash, err := validateAutomationScheduleControlTx(
		state.tx, state.ctx, auth.Bearer, params, auth.Now)
	if err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	selectorDigest := ""
	if params.AudienceKind == automationcontract.AudienceExplicit {
		selectorDigest = automationTargetSetDigest(targets)
	}
	id := ulid.NewAutomationScheduleID(time.UnixMilli(auth.Now))
	_, err = state.tx.Exec(`INSERT INTO automation_schedules(
  id, owner_orbit_id, created_by_actor_id, cue_id, display_name, timezone,
  weekdays_mask, local_minute, audience_kind, selector_digest, bound_air_id,
  delivery, policy_version, policy_revision, enabled, revision, created_at,
  updated_at, disabled_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'overlay', ?, ?, 0, 1, ?, ?, ?)`, id,
		state.ctx.OrbitID, state.ctx.ActorID, params.CueID, name, params.Timezone,
		params.WeekdaysMask, params.LocalMinute, params.AudienceKind, selectorDigest,
		params.BoundAirID, automationcontract.ContractVersion, params.PolicyRevision,
		auth.Now, auth.Now, auth.Now)
	if err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	if _, err := state.tx.Exec(`INSERT INTO automation_schedule_controls(
  schedule_id, additional_quiet_hours_json, additional_quiet_hours_hash
) VALUES(?, ?, ?)`, id, quietJSON, quietHash); err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	if err := insertAutomationScheduleTargetsTx(state.tx, id, targets); err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	schedule, err := scanAutomationSchedule(state.tx.QueryRow(`SELECT `+automationScheduleColumns+`
FROM automation_schedules WHERE id = ?`, id))
	if err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	result := AutomationScheduleControlMutation{Control: AutomationScheduleControl{
		Schedule: schedule, AdditionalQuietHours: quiet, Targets: targets,
	}}
	if err := s.checkpoint("automation_control_schedule_create_before_commit"); err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	if err := finishAutomationControlMutation(state, auth, operation, id, result); err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	return result, nil
}

func automationScheduleOwnedControlTx(tx *sql.Tx, scheduleID string, orbitID int64) (AutomationSchedule, error) {
	if !automationScheduleIDPattern.MatchString(scheduleID) {
		return AutomationSchedule{}, ErrAutomationNotFound
	}
	schedule, err := scanAutomationSchedule(tx.QueryRow(`SELECT `+automationScheduleColumns+`
FROM automation_schedules s WHERE s.id = ? AND s.owner_orbit_id = ?
AND NOT EXISTS(SELECT 1 FROM automation_schedule_controls c
  WHERE c.schedule_id = s.id AND c.deleted_at > 0)`, scheduleID, orbitID))
	if errors.Is(err, sql.ErrNoRows) {
		return AutomationSchedule{}, ErrAutomationNotFound
	}
	return schedule, err
}

func (s *Store) ReplaceAuthorizedAutomationSchedule(auth AutomationControlAuth, scheduleID string, expectedRevision int64, params AutomationScheduleControlParams) (AutomationScheduleControlMutation, error) {
	const operation = "automation.schedule.replace.v1"
	state, auth, err := s.beginAutomationControlMutation(auth, operation)
	if err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	defer state.tx.Rollback()
	if state.replayed {
		result, err := replayAutomationControlMutation[AutomationScheduleControlMutation](state)
		result.Replayed = true
		return result, err
	}
	if expectedRevision <= 0 {
		return AutomationScheduleControlMutation{}, ErrAutomationInvalid
	}
	if err := requireCurrentContentPolicyTx(state.tx, state.ctx); err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	current, err := automationScheduleOwnedControlTx(state.tx, scheduleID, state.ctx.OrbitID)
	if err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	if current.Revision != expectedRevision {
		return AutomationScheduleControlMutation{}, ErrAutomationStateConflict
	}
	name, quietJSON, quiet, targets, quietHash, err := validateAutomationScheduleControlTx(
		state.tx, state.ctx, auth.Bearer, params, auth.Now)
	if err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	selectorDigest := ""
	if params.AudienceKind == automationcontract.AudienceExplicit {
		selectorDigest = automationTargetSetDigest(targets)
	}
	_, err = state.tx.Exec(`UPDATE automation_schedules SET cue_id = ?, display_name = ?,
timezone = ?, weekdays_mask = ?, local_minute = ?, audience_kind = ?,
selector_digest = ?, bound_air_id = ?, policy_revision = ?, revision = revision + 1,
updated_at = ? WHERE id = ? AND revision = ?`, params.CueID, name, params.Timezone,
		params.WeekdaysMask, params.LocalMinute, params.AudienceKind, selectorDigest,
		params.BoundAirID, params.PolicyRevision, auth.Now, current.ID, current.Revision)
	if err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	_, err = state.tx.Exec(`INSERT INTO automation_schedule_controls(
  schedule_id, additional_quiet_hours_json, additional_quiet_hours_hash
) VALUES(?, ?, ?) ON CONFLICT(schedule_id) DO UPDATE SET
  additional_quiet_hours_json = excluded.additional_quiet_hours_json,
  additional_quiet_hours_hash = excluded.additional_quiet_hours_hash`,
		current.ID, quietJSON, quietHash)
	if err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	if _, err := state.tx.Exec(`DELETE FROM automation_schedule_targets WHERE schedule_id = ?`, current.ID); err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	if err := insertAutomationScheduleTargetsTx(state.tx, current.ID, targets); err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	updated, err := scanAutomationSchedule(state.tx.QueryRow(`SELECT `+automationScheduleColumns+`
FROM automation_schedules WHERE id = ?`, current.ID))
	if err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	result := AutomationScheduleControlMutation{Control: AutomationScheduleControl{
		Schedule: updated, AdditionalQuietHours: quiet, Targets: targets,
	}}
	if err := finishAutomationControlMutation(state, auth, operation, current.ID, result); err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	return result, nil
}

func (s *Store) SetAuthorizedAutomationScheduleEnabled(auth AutomationControlAuth, scheduleID string, expectedRevision int64, enabled bool) (AutomationScheduleControlMutation, error) {
	operation := "automation.schedule.disable.v1"
	if enabled {
		operation = "automation.schedule.enable.v1"
	}
	state, auth, err := s.beginAutomationControlMutation(auth, operation)
	if err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	defer state.tx.Rollback()
	if state.replayed {
		result, err := replayAutomationControlMutation[AutomationScheduleControlMutation](state)
		result.Replayed = true
		return result, err
	}
	if expectedRevision <= 0 {
		return AutomationScheduleControlMutation{}, ErrAutomationInvalid
	}
	schedule, err := automationScheduleOwnedControlTx(state.tx, scheduleID, state.ctx.OrbitID)
	if err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	if schedule.Revision != expectedRevision {
		return AutomationScheduleControlMutation{}, ErrAutomationStateConflict
	}
	if schedule.Enabled == enabled {
		control, err := loadAutomationScheduleControlTx(state.tx, schedule)
		if err != nil {
			return AutomationScheduleControlMutation{}, err
		}
		result := AutomationScheduleControlMutation{Control: control}
		if err := finishAutomationControlMutation(state, auth, operation, schedule.ID, result); err != nil {
			return AutomationScheduleControlMutation{}, err
		}
		return result, nil
	}
	disabledAt := auth.Now
	if enabled {
		disabledAt = 0
		if _, err := savedCueOwnedActiveTx(state.tx, schedule.CueID, state.ctx.OrbitID); err != nil {
			return AutomationScheduleControlMutation{}, err
		}
		var feature AutomationFeatureState
		feature, err = scanAutomationFeatureState(state.tx.QueryRow(`SELECT `+automationFeatureColumns+`
FROM automation_feature_state WHERE owner_orbit_id = ?`, state.ctx.OrbitID))
		if errors.Is(err, sql.ErrNoRows) || (err == nil && (!feature.AutomationEnabled || feature.EmergencyDisabled)) {
			return AutomationScheduleControlMutation{}, ErrAutomationDisabled
		}
		if err != nil {
			return AutomationScheduleControlMutation{}, err
		}
		if !AutomationQuietPolicyValid(feature.Timezone, feature.QuietHoursJSON, feature.QuietHoursHash) {
			return AutomationScheduleControlMutation{}, ErrAutomationDisabled
		}
		if feature.Revision != schedule.PolicyRevision {
			return AutomationScheduleControlMutation{}, ErrAutomationStateConflict
		}
	}
	_, err = state.tx.Exec(`UPDATE automation_schedules SET enabled = ?,
revision = revision + 1, updated_at = ?, disabled_at = ?
WHERE id = ? AND revision = ?`, boolInt(enabled), auth.Now, disabledAt,
		schedule.ID, schedule.Revision)
	if err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	updated, err := scanAutomationSchedule(state.tx.QueryRow(`SELECT `+automationScheduleColumns+`
FROM automation_schedules WHERE id = ?`, schedule.ID))
	if err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	control, err := loadAutomationScheduleControlTx(state.tx, updated)
	if err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	result := AutomationScheduleControlMutation{Control: control}
	if err := finishAutomationControlMutation(state, auth, operation, schedule.ID, result); err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	return result, nil
}

func (s *Store) DeleteAuthorizedAutomationSchedule(auth AutomationControlAuth, scheduleID string, expectedRevision int64) (AutomationScheduleControlMutation, error) {
	const operation = "automation.schedule.delete.v1"
	state, auth, err := s.beginAutomationControlMutation(auth, operation)
	if err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	defer state.tx.Rollback()
	if state.replayed {
		result, err := replayAutomationControlMutation[AutomationScheduleControlMutation](state)
		result.Replayed = true
		return result, err
	}
	if expectedRevision <= 0 {
		return AutomationScheduleControlMutation{}, ErrAutomationInvalid
	}
	schedule, err := automationScheduleOwnedControlTx(state.tx, scheduleID, state.ctx.OrbitID)
	if err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	if schedule.Revision != expectedRevision {
		return AutomationScheduleControlMutation{}, ErrAutomationStateConflict
	}
	_, err = state.tx.Exec(`UPDATE automation_schedules SET enabled = 0,
revision = revision + 1, updated_at = ?, disabled_at = ?
WHERE id = ? AND revision = ?`, auth.Now, auth.Now, schedule.ID, schedule.Revision)
	if err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	_, err = state.tx.Exec(`INSERT INTO automation_schedule_controls(
  schedule_id, additional_quiet_hours_json, additional_quiet_hours_hash,
  deleted_at, deleted_by_actor_id
) VALUES(?, '[]', ?, ?, ?) ON CONFLICT(schedule_id) DO UPDATE SET
  deleted_at = excluded.deleted_at,
  deleted_by_actor_id = excluded.deleted_by_actor_id`, schedule.ID,
		hashToken("[]"), auth.Now, state.ctx.ActorID)
	if err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	updated, err := scanAutomationSchedule(state.tx.QueryRow(`SELECT `+automationScheduleColumns+`
FROM automation_schedules WHERE id = ?`, schedule.ID))
	if err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	control := AutomationScheduleControl{Schedule: updated}
	result := AutomationScheduleControlMutation{Control: control}
	if err := s.checkpoint("automation_control_schedule_delete_before_commit"); err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	if err := finishAutomationControlMutation(state, auth, operation, schedule.ID, result); err != nil {
		return AutomationScheduleControlMutation{}, err
	}
	return result, nil
}

type AutomationPrincipalControlParams struct {
	DisplayName      string
	AllowedCueIDs    []string
	AllowedAudiences []automationcontract.AudienceKind
	TargetReferences []string
	BoundAirID       string
	MaxTargetCount   int
	ExpiresAt        int64
}

type AutomationPrincipalControlIssue struct {
	Principal       AutomationPrincipal `json:"principal"`
	Secret          string              `json:"secret,omitempty"`
	SecretAvailable bool                `json:"secret_available"`
	Replayed        bool                `json:"replayed"`
}

func (s *Store) IssueAuthorizedAutomationPrincipal(auth AutomationControlAuth, params AutomationPrincipalControlParams) (AutomationPrincipalControlIssue, error) {
	const operation = "automation.principal.issue.v1"
	state, auth, err := s.beginAutomationControlMutation(auth, operation)
	if err != nil {
		return AutomationPrincipalControlIssue{}, err
	}
	defer state.tx.Rollback()
	if state.replayed {
		result, err := replayAutomationControlMutation[AutomationPrincipalControlIssue](state)
		result.Secret = ""
		result.SecretAvailable = false
		result.Replayed = true
		return result, err
	}
	name := strings.TrimSpace(params.DisplayName)
	cues, cueOK := uniqueSortedStrings(params.AllowedCueIDs)
	audiences, audienceOK := normalizeAutomationAudiences(params.AllowedAudiences)
	if name == "" || len(name) > 128 || !cueOK || !audienceOK ||
		params.MaxTargetCount < 1 || params.MaxTargetCount > automationcontract.MaxExplicitSelectors ||
		params.ExpiresAt <= auth.Now || params.ExpiresAt-auth.Now > (90*24*time.Hour).Milliseconds() {
		return AutomationPrincipalControlIssue{}, ErrAutomationInvalid
	}
	hasAir, hasExplicit := false, false
	for _, audience := range audiences {
		hasAir = hasAir || audience == automationcontract.AudienceCurrentAir
		hasExplicit = hasExplicit || audience == automationcontract.AudienceExplicit
	}
	if hasAir != (params.BoundAirID != "") || hasExplicit != (len(params.TargetReferences) != 0) {
		return AutomationPrincipalControlIssue{}, ErrAutomationInvalid
	}
	if err := requireCurrentContentPolicyTx(state.tx, state.ctx); err != nil {
		return AutomationPrincipalControlIssue{}, err
	}
	for _, cueID := range cues {
		if _, err := savedCueOwnedActiveTx(state.tx, cueID, state.ctx.OrbitID); err != nil {
			return AutomationPrincipalControlIssue{}, err
		}
	}
	if hasAir {
		active, err := automationBoundAirActiveTx(state.tx, params.BoundAirID, state.ctx.OrbitID)
		if err != nil {
			return AutomationPrincipalControlIssue{}, err
		}
		if !active {
			return AutomationPrincipalControlIssue{}, ErrAutomationAudienceNotAllowed
		}
	}
	var targets []AutomationTargetScope
	if hasExplicit {
		targets, err = resolveAutomationTargetScopesTx(state.tx, state.ctx, auth.Bearer,
			params.TargetReferences, auth.Now)
		if err != nil {
			return AutomationPrincipalControlIssue{}, err
		}
		if len(targets) > params.MaxTargetCount {
			return AutomationPrincipalControlIssue{}, ErrAutomationInvalid
		}
	}
	secret, err := newAutomationSecret()
	if err != nil {
		return AutomationPrincipalControlIssue{}, err
	}
	id := ulid.NewAutomationPrincipalID(time.UnixMilli(auth.Now))
	_, err = state.tx.Exec(`INSERT INTO automation_principals(
  id, owner_orbit_id, issued_by_actor_id, display_name, secret_hash,
  secret_hash_version, permission, bound_air_id, max_target_count,
  issued_at, expires_at
) VALUES(?, ?, ?, ?, ?, ?, 'automation:trigger', ?, ?, ?, ?)`, id,
		state.ctx.OrbitID, state.ctx.ActorID, name, automationSecretHash(secret),
		AutomationSecretHashVersion, params.BoundAirID, params.MaxTargetCount,
		auth.Now, params.ExpiresAt)
	if err != nil {
		return AutomationPrincipalControlIssue{}, err
	}
	for _, cueID := range cues {
		if _, err := state.tx.Exec(`INSERT INTO automation_principal_cues(principal_id, cue_id)
VALUES(?, ?)`, id, cueID); err != nil {
			return AutomationPrincipalControlIssue{}, err
		}
	}
	for _, audience := range audiences {
		if _, err := state.tx.Exec(`INSERT INTO automation_principal_audiences(principal_id, audience_kind)
VALUES(?, ?)`, id, audience); err != nil {
			return AutomationPrincipalControlIssue{}, err
		}
	}
	for _, target := range targets {
		if _, err := state.tx.Exec(`INSERT INTO automation_principal_target_refs(
  principal_id, target_ref_digest
) VALUES(?, ?)`, id, target.Digest); err != nil {
			return AutomationPrincipalControlIssue{}, err
		}
	}
	principal, err := scanAutomationPrincipal(state.tx.QueryRow(`SELECT `+automationPrincipalColumns+`
FROM automation_principals WHERE id = ?`, id))
	if err != nil {
		return AutomationPrincipalControlIssue{}, err
	}
	if err := loadAutomationPrincipalScopesTx(state.tx, &principal); err != nil {
		return AutomationPrincipalControlIssue{}, err
	}
	result := AutomationPrincipalControlIssue{
		Principal: principal, Secret: secret, SecretAvailable: true,
	}
	stored := result
	stored.Secret = ""
	stored.SecretAvailable = false
	if err := s.checkpoint("automation_control_principal_issue_before_commit"); err != nil {
		return AutomationPrincipalControlIssue{}, err
	}
	if err := finishAutomationControlMutation(state, auth, operation, id, stored); err != nil {
		return AutomationPrincipalControlIssue{}, err
	}
	return result, nil
}

type AutomationPrincipalControlMutation struct {
	Principal AutomationPrincipal `json:"principal"`
	Replayed  bool                `json:"replayed"`
}

func (s *Store) RevokeAuthorizedAutomationPrincipal(auth AutomationControlAuth, principalID string, expectedRevision int64) (AutomationPrincipalControlMutation, error) {
	const operation = "automation.principal.revoke.v1"
	state, auth, err := s.beginAutomationControlMutation(auth, operation)
	if err != nil {
		return AutomationPrincipalControlMutation{}, err
	}
	defer state.tx.Rollback()
	if state.replayed {
		result, err := replayAutomationControlMutation[AutomationPrincipalControlMutation](state)
		result.Replayed = true
		return result, err
	}
	if !automationPrincipalIDPattern.MatchString(principalID) || expectedRevision <= 0 {
		return AutomationPrincipalControlMutation{}, ErrAutomationInvalid
	}
	principal, err := scanAutomationPrincipal(state.tx.QueryRow(`SELECT `+automationPrincipalColumns+`
FROM automation_principals WHERE id = ? AND owner_orbit_id = ?`, principalID,
		state.ctx.OrbitID))
	if errors.Is(err, sql.ErrNoRows) {
		return AutomationPrincipalControlMutation{}, ErrAutomationNotFound
	}
	if err != nil {
		return AutomationPrincipalControlMutation{}, err
	}
	if principal.Revision != expectedRevision {
		return AutomationPrincipalControlMutation{}, ErrAutomationStateConflict
	}
	if principal.RevokedAt == 0 {
		_, err = state.tx.Exec(`UPDATE automation_principals SET revoked_at = ?,
revoked_by_actor_id = ?, revision = revision + 1 WHERE id = ? AND revision = ?`,
			auth.Now, state.ctx.ActorID, principal.ID, principal.Revision)
		if err != nil {
			return AutomationPrincipalControlMutation{}, err
		}
		principal, err = scanAutomationPrincipal(state.tx.QueryRow(`SELECT `+automationPrincipalColumns+`
FROM automation_principals WHERE id = ?`, principal.ID))
		if err != nil {
			return AutomationPrincipalControlMutation{}, err
		}
	}
	if err := loadAutomationPrincipalScopesTx(state.tx, &principal); err != nil {
		return AutomationPrincipalControlMutation{}, err
	}
	result := AutomationPrincipalControlMutation{Principal: principal}
	if err := finishAutomationControlMutation(state, auth, operation, principal.ID, result); err != nil {
		return AutomationPrincipalControlMutation{}, err
	}
	return result, nil
}
