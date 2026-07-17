package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sort"
	"strings"
	"time"

	automationcontract "relux.works/duet/coordinator/internal/automation"
)

var (
	ErrAutomationControlIdempotencyConflict = errors.New("automation control idempotency conflict")
	ErrAutomationAudienceNotAllowed         = errors.New("automation audience is not allowed")
)

// AutomationControlAuth binds a mutation to an exact current primary and to a
// durable actor-scoped idempotency key. Only digests enter SQLite.
type AutomationControlAuth struct {
	ExpectedActorID    int64
	Bearer             string
	IdempotencyKeyHash string
	RequestHash        string
	Now                int64
}

type automationControlMutationState struct {
	tx       *sql.Tx
	ctx      ActorContext
	replayed bool
	raw      []byte
}

func normalizeAutomationControlAuth(auth AutomationControlAuth) (AutomationControlAuth, error) {
	if auth.Now <= 0 {
		auth.Now = time.Now().UnixMilli()
	}
	if auth.ExpectedActorID <= 0 || auth.Bearer == "" ||
		!lowerHexTokenPattern.MatchString(auth.IdempotencyKeyHash) ||
		!lowerHexTokenPattern.MatchString(auth.RequestHash) {
		return AutomationControlAuth{}, ErrAutomationInvalid
	}
	return auth, nil
}

func (s *Store) beginAutomationControlMutation(auth AutomationControlAuth, operation string) (automationControlMutationState, AutomationControlAuth, error) {
	auth, err := normalizeAutomationControlAuth(auth)
	if err != nil || operation == "" || len(operation) > 128 {
		return automationControlMutationState{}, auth, ErrAutomationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return automationControlMutationState{}, auth, err
	}
	ctx, err := authorizeSavedCueMutationTx(tx, auth.ExpectedActorID, auth.Bearer)
	if err != nil {
		tx.Rollback()
		return automationControlMutationState{}, auth, err
	}
	var storedOperation, storedRequest, raw string
	err = tx.QueryRow(`SELECT operation, request_hash, response_json
FROM automation_control_mutation_results
WHERE actor_id = ? AND idempotency_key_hash = ?`, ctx.ActorID,
		auth.IdempotencyKeyHash).Scan(&storedOperation, &storedRequest, &raw)
	if err == nil {
		if storedOperation != operation || storedRequest != auth.RequestHash {
			tx.Rollback()
			return automationControlMutationState{}, auth, ErrAutomationControlIdempotencyConflict
		}
		return automationControlMutationState{
			tx: tx, ctx: ctx, replayed: true, raw: []byte(raw),
		}, auth, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		tx.Rollback()
		return automationControlMutationState{}, auth, err
	}
	return automationControlMutationState{tx: tx, ctx: ctx}, auth, nil
}

func finishAutomationControlMutation(state automationControlMutationState, auth AutomationControlAuth, operation, resourceID string, stored any) error {
	raw, err := json.Marshal(stored)
	if err != nil || len(raw) > 65536 || len(resourceID) > 128 {
		if err == nil {
			err = ErrAutomationInvalid
		}
		return err
	}
	_, err = state.tx.Exec(`INSERT INTO automation_control_mutation_results(
  actor_id, idempotency_key_hash, operation, request_hash, response_json,
  resource_id, created_at
) VALUES(?, ?, ?, ?, ?, ?, ?)`, state.ctx.ActorID, auth.IdempotencyKeyHash,
		operation, auth.RequestHash, string(raw), resourceID, auth.Now)
	if err != nil {
		return err
	}
	if _, err := state.tx.Exec(`INSERT INTO automation_audit_events(
  event_kind, operation, owner_orbit_id, actor_id, outcome, reason_code,
  terminal_at, created_at, principal_id, schedule_id
) VALUES('control', ?, ?, ?, 'accepted', '', ?, ?,
  CASE WHEN substr(?, 1, 3) = 'ap_' THEN ? ELSE '' END,
  CASE WHEN substr(?, 1, 4) = 'sch_' THEN ? ELSE '' END)`, operation,
		state.ctx.OrbitID, state.ctx.ActorID, auth.Now, auth.Now,
		resourceID, resourceID, resourceID, resourceID); err != nil {
		return err
	}
	return state.tx.Commit()
}

func replayAutomationControlMutation[T any](state automationControlMutationState) (T, error) {
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

func beginAutomationControlRead(s *Store, expectedActorID int64, bearer string) (*sql.Tx, ActorContext, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, ActorContext{}, err
	}
	ctx, err := authorizeSavedCueMutationTx(tx, expectedActorID, bearer)
	if err != nil {
		tx.Rollback()
		return nil, ActorContext{}, err
	}
	return tx, ctx, nil
}

// AutomationQuietWindow is a weekly half-open local-time denial interval. A
// cross-midnight interval belongs to Weekday and continues into the next day.
type AutomationQuietWindow struct {
	Weekday     int `json:"weekday"`
	StartMinute int `json:"start_minute"`
	EndMinute   int `json:"end_minute"`
}

func NormalizeAutomationQuietHours(windows []AutomationQuietWindow) ([]AutomationQuietWindow, string, string, error) {
	if len(windows) > 128 {
		return nil, "", "", ErrAutomationInvalid
	}
	canonical := make([]AutomationQuietWindow, len(windows))
	copy(canonical, windows)
	sort.Slice(canonical, func(i, j int) bool {
		if canonical[i].Weekday != canonical[j].Weekday {
			return canonical[i].Weekday < canonical[j].Weekday
		}
		if canonical[i].StartMinute != canonical[j].StartMinute {
			return canonical[i].StartMinute < canonical[j].StartMinute
		}
		return canonical[i].EndMinute < canonical[j].EndMinute
	})
	occupied := make([]bool, 7*24*60)
	for _, window := range canonical {
		if window.Weekday < 0 || window.Weekday > 6 ||
			window.StartMinute < 0 || window.StartMinute > 1439 ||
			window.EndMinute < 0 || window.EndMinute > 1439 ||
			window.StartMinute == window.EndMinute {
			return nil, "", "", ErrAutomationInvalid
		}
		start := window.Weekday*1440 + window.StartMinute
		length := window.EndMinute - window.StartMinute
		if length <= 0 {
			length += 1440
		}
		for offset := 0; offset < length; offset++ {
			minute := (start + offset) % len(occupied)
			if occupied[minute] {
				return nil, "", "", ErrAutomationInvalid
			}
			occupied[minute] = true
		}
	}
	raw, err := json.Marshal(canonical)
	if err != nil || len(raw) > 16384 {
		return nil, "", "", ErrAutomationInvalid
	}
	digest := sha256.Sum256(raw)
	return canonical, string(raw), hex.EncodeToString(digest[:]), nil
}

func AutomationQuietPolicyValid(timezone, raw, expectedHash string) bool {
	if timezone == "" || raw == "" || expectedHash == "" {
		return false
	}
	if _, err := time.LoadLocation(timezone); err != nil {
		return false
	}
	var windows []AutomationQuietWindow
	decoder := json.NewDecoder(strings.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&windows); err != nil || windows == nil {
		return false
	}
	var extra any
	if !errors.Is(decoder.Decode(&extra), io.EOF) {
		return false
	}
	_, canonical, digest, err := NormalizeAutomationQuietHours(windows)
	return err == nil && canonical == raw && digest == expectedHash
}

type AutomationTargetScope struct {
	Digest          string                           `json:"digest"`
	Kind            TransmissionAudienceSelectorKind `json:"kind"`
	OrbitID         int64                            `json:"orbit_id"`
	ActorID         int64                            `json:"actor_id,omitempty"`
	Slot            string                           `json:"slot,omitempty"`
	BindingPairedAt int64                            `json:"binding_paired_at,omitempty"`
}

func automationTargetScopeDigest(scope AutomationTargetScope) string {
	return hashToken(fmt.Sprintf("barycenter/automation-target-scope/v1:%s:%d:%d:%s:%d",
		scope.Kind, scope.OrbitID, scope.ActorID, scope.Slot, scope.BindingPairedAt))
}

func resolveAutomationTargetScopesTx(tx *sql.Tx, ctx ActorContext, bearer string, references []string, now int64) ([]AutomationTargetScope, error) {
	if len(references) == 0 || len(references) > automationcontract.MaxExplicitSelectors {
		return nil, ErrAutomationInvalid
	}
	proof := Identity{Kind: IdentityBearer, Token: bearer}
	allowed, err := targetReferenceDomainTx(tx, ctx)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(references))
	result := make([]AutomationTargetScope, 0, len(references))
	for _, reference := range references {
		selector, err := resolveTransmissionTargetReferenceTx(tx, ctx, proof, reference, now, allowed)
		if err != nil {
			return nil, ErrAutomationAudienceNotAllowed
		}
		scope := AutomationTargetScope{Kind: selector.Kind, OrbitID: selector.OrbitID, Slot: selector.Slot}
		if selector.Kind == TransmissionSelectorPulsar {
			targets, err := liveTransmissionTargetsTx(tx, selector.OrbitID, selector.Slot)
			if err != nil || len(targets) != 1 {
				return nil, ErrAutomationAudienceNotAllowed
			}
			scope.ActorID = targets[0].ActorID
			scope.BindingPairedAt = targets[0].BindingPairedAt
		}
		scope.Digest = automationTargetScopeDigest(scope)
		if _, exists := seen[scope.Digest]; exists {
			continue
		}
		seen[scope.Digest] = struct{}{}
		result = append(result, scope)
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Digest < result[j].Digest })
	return result, nil
}

type OrderedSavedCue struct {
	Cue      SavedCue `json:"cue"`
	Position int      `json:"position"`
}

type SavedCueControlList struct {
	OrderRevision int64             `json:"order_revision"`
	Items         []OrderedSavedCue `json:"items"`
}

func savedCueControlListTx(tx *sql.Tx, orbitID int64) (SavedCueControlList, error) {
	var result SavedCueControlList
	err := tx.QueryRow(`SELECT revision FROM saved_cue_order_state
WHERE owner_orbit_id = ?`, orbitID).Scan(&result.OrderRevision)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return result, err
	}
	rows, err := tx.Query(`SELECT c.id, c.owner_orbit_id, c.created_by_actor_id,
c.title, c.source_kind, COALESCE(c.media_id, ''), c.media_revision,
c.builtin_asset_id, c.builtin_sha256, c.source_sha256, c.source_bytes,
c.source_duration_ms, c.state, c.revoke_reason, c.revision,
c.source_generation, c.created_at, c.updated_at, c.deleted_at,
COALESCE(o.position, -1)
FROM saved_cues c LEFT JOIN saved_cue_order_items o
  ON o.owner_orbit_id = c.owner_orbit_id AND o.cue_id = c.id
WHERE c.owner_orbit_id = ? AND c.state = 'active'
ORDER BY CASE WHEN o.position IS NULL THEN 1 ELSE 0 END, o.position,
  c.created_at, c.id`, orbitID)
	if err != nil {
		return result, err
	}
	defer rows.Close()
	for rows.Next() {
		var item OrderedSavedCue
		var cue SavedCue
		err := rows.Scan(&cue.ID, &cue.OwnerOrbitID, &cue.CreatedByActorID, &cue.Title,
			&cue.SourceKind, &cue.MediaID, &cue.MediaRevision, &cue.BuiltinAssetID,
			&cue.BuiltinSHA256, &cue.SourceSHA256, &cue.SourceBytes,
			&cue.SourceDurationMS, &cue.State, &cue.RevokeReason, &cue.Revision,
			&cue.SourceGeneration, &cue.CreatedAt, &cue.UpdatedAt, &cue.DeletedAt,
			&item.Position)
		if err != nil {
			return result, err
		}
		item.Cue = cue
		if item.Position < 0 {
			item.Position = len(result.Items)
		}
		result.Items = append(result.Items, item)
	}
	return result, rows.Err()
}

func (s *Store) AuthorizedSavedCueControlList(expectedActorID int64, bearer string) (SavedCueControlList, error) {
	tx, ctx, err := beginAutomationControlRead(s, expectedActorID, bearer)
	if err != nil {
		return SavedCueControlList{}, err
	}
	defer tx.Rollback()
	result, err := savedCueControlListTx(tx, ctx.OrbitID)
	if err != nil {
		return SavedCueControlList{}, err
	}
	return result, tx.Commit()
}

func (s *Store) AuthorizedAutomationFeatureState(expectedActorID int64, bearer string) (AutomationFeatureState, error) {
	tx, ctx, err := beginAutomationControlRead(s, expectedActorID, bearer)
	if err != nil {
		return AutomationFeatureState{}, err
	}
	defer tx.Rollback()
	state, err := scanAutomationFeatureState(tx.QueryRow(`SELECT `+automationFeatureColumns+`
FROM automation_feature_state WHERE owner_orbit_id = ?`, ctx.OrbitID))
	if errors.Is(err, sql.ErrNoRows) {
		state = AutomationFeatureState{
			OwnerOrbitID: ctx.OrbitID, PolicyVersion: automationcontract.ContractVersion,
		}
	} else if err != nil {
		return AutomationFeatureState{}, err
	}
	return state, tx.Commit()
}

type AutomationScheduleControl struct {
	Schedule             AutomationSchedule      `json:"schedule"`
	AdditionalQuietHours []AutomationQuietWindow `json:"additional_quiet_hours"`
	Targets              []AutomationTargetScope `json:"targets"`
}

func loadAutomationScheduleControlTx(tx *sql.Tx, schedule AutomationSchedule) (AutomationScheduleControl, error) {
	result := AutomationScheduleControl{Schedule: schedule}
	var quietJSON string
	err := tx.QueryRow(`SELECT additional_quiet_hours_json
FROM automation_schedule_controls WHERE schedule_id = ? AND deleted_at = 0`,
		schedule.ID).Scan(&quietJSON)
	if errors.Is(err, sql.ErrNoRows) {
		quietJSON = "[]"
	} else if err != nil {
		return result, err
	}
	if err := json.Unmarshal([]byte(quietJSON), &result.AdditionalQuietHours); err != nil {
		return result, err
	}
	rows, err := tx.Query(`SELECT target_digest, target_kind, target_orbit_id,
target_actor_id, target_slot, target_binding_paired_at
FROM automation_schedule_targets WHERE schedule_id = ? ORDER BY target_digest`, schedule.ID)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var target AutomationTargetScope
		if err := rows.Scan(&target.Digest, &target.Kind, &target.OrbitID,
			&target.ActorID, &target.Slot, &target.BindingPairedAt); err != nil {
			rows.Close()
			return result, err
		}
		result.Targets = append(result.Targets, target)
	}
	return result, rows.Close()
}

func (s *Store) AuthorizedAutomationSchedules(expectedActorID int64, bearer string) ([]AutomationScheduleControl, error) {
	tx, ctx, err := beginAutomationControlRead(s, expectedActorID, bearer)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT `+automationScheduleColumns+`
FROM automation_schedules s
WHERE s.owner_orbit_id = ? AND NOT EXISTS(
  SELECT 1 FROM automation_schedule_controls c
  WHERE c.schedule_id = s.id AND c.deleted_at > 0
)
ORDER BY s.created_at, s.id`, ctx.OrbitID)
	if err != nil {
		return nil, err
	}
	var schedules []AutomationSchedule
	for rows.Next() {
		schedule, err := scanAutomationSchedule(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		schedules = append(schedules, schedule)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	result := make([]AutomationScheduleControl, 0, len(schedules))
	for _, schedule := range schedules {
		control, err := loadAutomationScheduleControlTx(tx, schedule)
		if err != nil {
			return nil, err
		}
		result = append(result, control)
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return result, nil
}

func (s *Store) AuthorizedAutomationPrincipals(expectedActorID int64, bearer string) ([]AutomationPrincipal, error) {
	tx, ctx, err := beginAutomationControlRead(s, expectedActorID, bearer)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT `+automationPrincipalColumns+`
FROM automation_principals WHERE owner_orbit_id = ? ORDER BY issued_at, id`, ctx.OrbitID)
	if err != nil {
		return nil, err
	}
	var principals []AutomationPrincipal
	for rows.Next() {
		principal, err := scanAutomationPrincipal(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		principals = append(principals, principal)
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for index := range principals {
		if err := loadAutomationPrincipalScopesTx(tx, &principals[index]); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return principals, nil
}
