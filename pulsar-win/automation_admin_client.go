package main

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"
)

type AutomationAudience string

const (
	AutomationOwnBarycenter AutomationAudience = "own_barycenter"
	AutomationCurrentAir    AutomationAudience = "current_air"
	AutomationExplicit      AutomationAudience = "explicit"
)

type AutomationQuietWindow struct {
	Weekday     int `json:"weekday"`
	StartMinute int `json:"start_minute"`
	EndMinute   int `json:"end_minute"`
}

type AutomationFeature struct {
	SoundboardEnabled bool
	AutomationEnabled bool
	EmergencyDisabled bool
	Timezone          string
	QuietHours        []AutomationQuietWindow
	PolicyVersion     string
	Revision          int64
	PolicyValid       bool
	UpdatedAt         time.Time
}

type AutomationSchedule struct {
	ID                   string
	CueID                string
	DisplayName          string
	Timezone             string
	Weekdays             []int
	LocalTime            string
	Audience             AutomationAudience
	TargetDigests        []string
	AirID                string
	AdditionalQuietHours []AutomationQuietWindow
	PolicyVersion        string
	PolicyRevision       int64
	Enabled              bool
	Revision             int64
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type AutomationScheduleDraft struct {
	CueID                string
	DisplayName          string
	Timezone             string
	Weekdays             []int
	LocalTime            string
	Audience             AutomationAudience
	TargetReferences     []string
	AirID                string
	AdditionalQuietHours []AutomationQuietWindow
	PolicyRevision       int64
}

type AutomationPrincipal struct {
	ID               string
	DisplayName      string
	Permission       string
	AirID            string
	MaxTargetCount   int
	AllowedCueIDs    []string
	AllowedAudiences []AutomationAudience
	TargetDigests    []string
	IssuedAt         time.Time
	ExpiresAt        time.Time
	DisabledAt       time.Time
	RevokedAt        time.Time
	Revision         int64
}

type AutomationPrincipalDraft struct {
	DisplayName      string
	AllowedCueIDs    []string
	AllowedAudiences []AutomationAudience
	TargetReferences []string
	AirID            string
	MaxTargetCount   int
	ExpiresAt        time.Time
}

type AutomationPrincipalIssue struct {
	Principal       AutomationPrincipal
	Secret          string
	SecretAvailable bool
}

func (i AutomationPrincipalIssue) String() string {
	return fmt.Sprintf("AutomationPrincipalIssue{principal:%s secret_available:%t secret:<redacted>}", i.Principal.DisplayName, i.SecretAvailable)
}

func (i AutomationPrincipalIssue) GoString() string { return i.String() }

type AutomationAdminService interface {
	SoundboardCues(context.Context) (SoundboardCueList, error)
	AutomationFeature(context.Context) (AutomationFeature, error)
	ReplaceAutomationFeature(context.Context, AutomationFeature, string) (AutomationFeature, error)
	AutomationSchedules(context.Context) ([]AutomationSchedule, error)
	CreateAutomationSchedule(context.Context, AutomationScheduleDraft, string) (AutomationSchedule, error)
	ReplaceAutomationSchedule(context.Context, AutomationSchedule, AutomationScheduleDraft, string) (AutomationSchedule, error)
	SetAutomationScheduleEnabled(context.Context, AutomationSchedule, bool, string) (AutomationSchedule, error)
	DeleteAutomationSchedule(context.Context, AutomationSchedule, string) (AutomationSchedule, error)
	AutomationPrincipals(context.Context) ([]AutomationPrincipal, error)
	IssueAutomationPrincipal(context.Context, AutomationPrincipalDraft, string) (AutomationPrincipalIssue, error)
	RevokeAutomationPrincipal(context.Context, AutomationPrincipal, string) (AutomationPrincipal, error)
	CancelAutomationHistory(context.Context, string) error
}

type automationFeatureWire struct {
	SoundboardEnabled bool                    `json:"soundboard_enabled"`
	AutomationEnabled bool                    `json:"automation_enabled"`
	EmergencyDisabled bool                    `json:"emergency_disabled"`
	Timezone          string                  `json:"timezone"`
	QuietHours        []AutomationQuietWindow `json:"quiet_hours"`
	PolicyVersion     string                  `json:"policy_version"`
	Revision          int64                   `json:"revision"`
	PolicyValid       bool                    `json:"policy_valid"`
	UpdatedAt         string                  `json:"updated_at"`
}

type automationAudienceWire struct {
	Kind          AutomationAudience `json:"kind"`
	TargetDigests []string           `json:"target_digests"`
	AirID         string             `json:"air_id"`
}

type automationScheduleWire struct {
	ScheduleID           string                  `json:"schedule_id"`
	CueID                string                  `json:"cue_id"`
	DisplayName          string                  `json:"display_name"`
	Timezone             string                  `json:"timezone"`
	Weekdays             []int                   `json:"weekdays"`
	LocalTime            string                  `json:"local_time"`
	Audience             automationAudienceWire  `json:"audience"`
	Delivery             string                  `json:"delivery"`
	AdditionalQuietHours []AutomationQuietWindow `json:"additional_quiet_hours"`
	PolicyVersion        string                  `json:"policy_version"`
	PolicyRevision       int64                   `json:"policy_revision"`
	Enabled              bool                    `json:"enabled"`
	Revision             int64                   `json:"revision"`
	CreatedAt            string                  `json:"created_at"`
	UpdatedAt            string                  `json:"updated_at"`
}

type automationPrincipalWire struct {
	PrincipalID          string               `json:"principal_id"`
	DisplayName          string               `json:"display_name"`
	Permission           string               `json:"permission"`
	BoundAirID           string               `json:"bound_air_id"`
	MaxTargetCount       int                  `json:"max_target_count"`
	AllowedCueIDs        []string             `json:"allowed_cue_ids"`
	AllowedAudienceKinds []AutomationAudience `json:"allowed_audience_kinds"`
	AllowedTargetDigests []string             `json:"allowed_target_digests"`
	IssuedAt             string               `json:"issued_at"`
	ExpiresAt            string               `json:"expires_at"`
	DisabledAt           string               `json:"disabled_at"`
	RevokedAt            string               `json:"revoked_at"`
	Revision             int64                `json:"revision"`
}

func validAutomationAudience(value AutomationAudience) bool {
	return value == AutomationOwnBarycenter || value == AutomationCurrentAir || value == AutomationExplicit
}

func validAutomationDigest(value string) bool {
	return len(value) == 64 && strings.Trim(value, "0123456789abcdef") == ""
}

func validAutomationTimezone(value string) bool {
	if value == "" || value == "Local" || len(value) > 128 || strings.TrimSpace(value) != value {
		return false
	}
	_, err := time.LoadLocation(value)
	return err == nil
}

func validAutomationLocalTime(value string) bool {
	if len(value) != 5 || value[2] != ':' {
		return false
	}
	_, err := time.Parse("15:04", value)
	return err == nil
}

func validAutomationQuietWindows(values []AutomationQuietWindow) bool {
	if len(values) > 128 {
		return false
	}
	occupied := make([]bool, 7*24*60)
	for _, value := range values {
		if value.Weekday < 0 || value.Weekday > 6 || value.StartMinute < 0 || value.StartMinute > 1439 ||
			value.EndMinute < 0 || value.EndMinute > 1439 || value.StartMinute == value.EndMinute {
			return false
		}
		minute := value.StartMinute
		for minute != value.EndMinute {
			index := value.Weekday*1440 + minute
			if minute < value.StartMinute {
				index = ((value.Weekday+1)%7)*1440 + minute
			}
			if occupied[index] {
				return false
			}
			occupied[index] = true
			minute = (minute + 1) % 1440
		}
	}
	return true
}

func validAutomationWeekdays(values []int) bool {
	if len(values) == 0 || len(values) > 7 {
		return false
	}
	seen := map[int]bool{}
	for _, value := range values {
		if value < 0 || value > 6 || seen[value] {
			return false
		}
		seen[value] = true
	}
	return true
}

func decodeAutomationTime(value string, optional bool) (time.Time, bool) {
	if value == "" && optional {
		return time.Time{}, true
	}
	parsed, err := time.Parse(time.RFC3339, value)
	return parsed, err == nil
}

func decodeAutomationFeature(raw []byte) (AutomationFeature, error) {
	var wire automationFeatureWire
	if decodePhaseOneJSON(raw, &wire) != nil || !validAutomationTimezone(wire.Timezone) ||
		!validAutomationQuietWindows(wire.QuietHours) || !wire.PolicyValid || wire.Revision < 0 ||
		!validPhaseOneDisplayText(wire.PolicyVersion, 64, false) {
		return AutomationFeature{}, phaseOneError(PhaseOneInvalidResponse)
	}
	updatedAt, ok := decodeAutomationTime(wire.UpdatedAt, false)
	if !ok {
		return AutomationFeature{}, phaseOneError(PhaseOneInvalidResponse)
	}
	return AutomationFeature{SoundboardEnabled: wire.SoundboardEnabled, AutomationEnabled: wire.AutomationEnabled,
		EmergencyDisabled: wire.EmergencyDisabled, Timezone: wire.Timezone,
		QuietHours: append([]AutomationQuietWindow(nil), wire.QuietHours...), PolicyVersion: wire.PolicyVersion,
		Revision: wire.Revision, PolicyValid: true, UpdatedAt: updatedAt}, nil
}

func (c *PhaseOneAppClient) AutomationFeature(ctx context.Context) (AutomationFeature, error) {
	raw, _, err := c.request(ctx, http.MethodGet, "/v1/automation/status", c.token, nil, nil, true, http.StatusOK)
	if err != nil {
		return AutomationFeature{}, err
	}
	return decodeAutomationFeature(raw)
}

func automationKeyHeaders(key string) (map[string]string, error) {
	if !validPhaseOneIdempotencyKey(key) || len(key) < 8 {
		return nil, phaseOneError(PhaseOneInvalidRequest)
	}
	return map[string]string{"Idempotency-Key": key}, nil
}

func (c *PhaseOneAppClient) ReplaceAutomationFeature(ctx context.Context, feature AutomationFeature, key string) (AutomationFeature, error) {
	headers, err := automationKeyHeaders(key)
	if err != nil || !validAutomationTimezone(feature.Timezone) || !validAutomationQuietWindows(feature.QuietHours) || feature.Revision < 0 {
		return AutomationFeature{}, phaseOneError(PhaseOneInvalidRequest)
	}
	body := map[string]any{"soundboard_enabled": feature.SoundboardEnabled, "automation_enabled": feature.AutomationEnabled,
		"emergency_disabled": feature.EmergencyDisabled, "timezone": feature.Timezone,
		"quiet_hours": feature.QuietHours, "expected_revision": feature.Revision}
	raw, _, err := c.requestJSON(ctx, http.MethodPut, "/v1/automation/status", c.token, headers, body, http.StatusOK)
	if err != nil {
		return AutomationFeature{}, err
	}
	return decodeAutomationFeature(raw)
}

func decodeAutomationSchedule(wire automationScheduleWire) (AutomationSchedule, error) {
	if !validPhaseOnePublicID(wire.ScheduleID, "sch_") || !validPhaseOnePublicID(wire.CueID, "cq_") ||
		!validPhaseOneDisplayText(wire.DisplayName, 128, false) || !validAutomationTimezone(wire.Timezone) ||
		!validAutomationWeekdays(wire.Weekdays) || !validAutomationLocalTime(wire.LocalTime) ||
		!validAutomationAudience(wire.Audience.Kind) || wire.Delivery != "overlay" ||
		!validAutomationQuietWindows(wire.AdditionalQuietHours) || wire.PolicyRevision <= 0 || wire.Revision <= 0 ||
		!validPhaseOneDisplayText(wire.PolicyVersion, 64, false) {
		return AutomationSchedule{}, phaseOneError(PhaseOneInvalidResponse)
	}
	if wire.Audience.Kind != AutomationExplicit && len(wire.Audience.TargetDigests) != 0 ||
		wire.Audience.Kind == AutomationExplicit && len(wire.Audience.TargetDigests) == 0 {
		return AutomationSchedule{}, phaseOneError(PhaseOneInvalidResponse)
	}
	for _, digest := range wire.Audience.TargetDigests {
		if !validAutomationDigest(digest) {
			return AutomationSchedule{}, phaseOneError(PhaseOneInvalidResponse)
		}
	}
	createdAt, createdOK := decodeAutomationTime(wire.CreatedAt, false)
	updatedAt, updatedOK := decodeAutomationTime(wire.UpdatedAt, false)
	if !createdOK || !updatedOK {
		return AutomationSchedule{}, phaseOneError(PhaseOneInvalidResponse)
	}
	result := AutomationSchedule{ID: wire.ScheduleID, CueID: wire.CueID, DisplayName: wire.DisplayName,
		Timezone: wire.Timezone, Weekdays: append([]int(nil), wire.Weekdays...), LocalTime: wire.LocalTime,
		Audience: wire.Audience.Kind, TargetDigests: append([]string(nil), wire.Audience.TargetDigests...),
		AirID: wire.Audience.AirID, AdditionalQuietHours: append([]AutomationQuietWindow(nil), wire.AdditionalQuietHours...),
		PolicyVersion: wire.PolicyVersion, PolicyRevision: wire.PolicyRevision, Enabled: wire.Enabled,
		Revision: wire.Revision, CreatedAt: createdAt, UpdatedAt: updatedAt}
	return result, nil
}

func (c *PhaseOneAppClient) AutomationSchedules(ctx context.Context) ([]AutomationSchedule, error) {
	raw, _, err := c.request(ctx, http.MethodGet, "/v1/automation/schedules", c.token, nil, nil, true, http.StatusOK)
	if err != nil {
		return nil, err
	}
	var response struct {
		Schedules []automationScheduleWire `json:"schedules"`
	}
	if decodePhaseOneJSON(raw, &response) != nil || len(response.Schedules) > 128 {
		return nil, phaseOneError(PhaseOneInvalidResponse)
	}
	result := make([]AutomationSchedule, 0, len(response.Schedules))
	seen := map[string]bool{}
	for _, wire := range response.Schedules {
		item, decodeErr := decodeAutomationSchedule(wire)
		if decodeErr != nil || seen[item.ID] {
			return nil, phaseOneError(PhaseOneInvalidResponse)
		}
		seen[item.ID] = true
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].DisplayName < result[j].DisplayName })
	return result, nil
}

func validAutomationScheduleDraft(draft AutomationScheduleDraft) bool {
	if !validPhaseOnePublicID(draft.CueID, "cq_") || !validPhaseOneDisplayText(draft.DisplayName, 128, false) ||
		!validAutomationTimezone(draft.Timezone) || !validAutomationWeekdays(draft.Weekdays) ||
		!validAutomationLocalTime(draft.LocalTime) || !validAutomationAudience(draft.Audience) ||
		!validAutomationQuietWindows(draft.AdditionalQuietHours) || draft.PolicyRevision <= 0 {
		return false
	}
	if draft.Audience == AutomationExplicit {
		if len(draft.TargetReferences) == 0 || len(draft.TargetReferences) > 64 {
			return false
		}
	} else if len(draft.TargetReferences) != 0 {
		return false
	}
	seenTargets := map[string]bool{}
	for _, reference := range draft.TargetReferences {
		if !targetReferencePattern.MatchString(reference) || seenTargets[reference] {
			return false
		}
		seenTargets[reference] = true
	}
	return true
}

func automationScheduleBody(draft AutomationScheduleDraft, revision int64) map[string]any {
	audience := map[string]any{"kind": string(draft.Audience)}
	if len(draft.TargetReferences) > 0 {
		audience["target_references"] = draft.TargetReferences
	}
	if draft.AirID != "" {
		audience["air_id"] = draft.AirID
	}
	body := map[string]any{"cue_id": draft.CueID, "display_name": draft.DisplayName, "timezone": draft.Timezone,
		"weekdays": draft.Weekdays, "local_time": draft.LocalTime, "audience": audience, "delivery": "overlay",
		"additional_quiet_hours": draft.AdditionalQuietHours, "policy_revision": draft.PolicyRevision}
	if revision > 0 {
		body["expected_revision"] = revision
	}
	return body
}

func decodeAutomationScheduleEnvelope(raw []byte) (AutomationSchedule, error) {
	var response struct {
		Schedule automationScheduleWire `json:"schedule"`
	}
	if decodePhaseOneJSON(raw, &response) != nil {
		return AutomationSchedule{}, phaseOneError(PhaseOneInvalidResponse)
	}
	return decodeAutomationSchedule(response.Schedule)
}

func (c *PhaseOneAppClient) CreateAutomationSchedule(ctx context.Context, draft AutomationScheduleDraft, key string) (AutomationSchedule, error) {
	headers, err := automationKeyHeaders(key)
	if err != nil || !validAutomationScheduleDraft(draft) {
		return AutomationSchedule{}, phaseOneError(PhaseOneInvalidRequest)
	}
	raw, _, err := c.requestJSON(ctx, http.MethodPost, "/v1/automation/schedules", c.token, headers, automationScheduleBody(draft, 0), http.StatusCreated)
	if err != nil {
		return AutomationSchedule{}, err
	}
	return decodeAutomationScheduleEnvelope(raw)
}

func (c *PhaseOneAppClient) ReplaceAutomationSchedule(ctx context.Context, current AutomationSchedule, draft AutomationScheduleDraft, key string) (AutomationSchedule, error) {
	headers, err := automationKeyHeaders(key)
	if err != nil || !validPhaseOnePublicID(current.ID, "sch_") || current.Revision <= 0 || !validAutomationScheduleDraft(draft) {
		return AutomationSchedule{}, phaseOneError(PhaseOneInvalidRequest)
	}
	raw, _, err := c.requestJSON(ctx, http.MethodPut, "/v1/automation/schedules/"+current.ID, c.token, headers, automationScheduleBody(draft, current.Revision), http.StatusOK)
	if err != nil {
		return AutomationSchedule{}, err
	}
	return decodeAutomationScheduleEnvelope(raw)
}

func (c *PhaseOneAppClient) SetAutomationScheduleEnabled(ctx context.Context, current AutomationSchedule, enabled bool, key string) (AutomationSchedule, error) {
	headers, err := automationKeyHeaders(key)
	if err != nil || !validPhaseOnePublicID(current.ID, "sch_") || current.Revision <= 0 {
		return AutomationSchedule{}, phaseOneError(PhaseOneInvalidRequest)
	}
	action := "disable"
	if enabled {
		action = "enable"
	}
	raw, _, err := c.requestJSON(ctx, http.MethodPost, "/v1/automation/schedules/"+current.ID+"/"+action, c.token, headers, map[string]any{"expected_revision": current.Revision}, http.StatusOK)
	if err != nil {
		return AutomationSchedule{}, err
	}
	return decodeAutomationScheduleEnvelope(raw)
}

func (c *PhaseOneAppClient) DeleteAutomationSchedule(ctx context.Context, current AutomationSchedule, key string) (AutomationSchedule, error) {
	headers, err := automationKeyHeaders(key)
	if err != nil || !validPhaseOnePublicID(current.ID, "sch_") || current.Revision <= 0 {
		return AutomationSchedule{}, phaseOneError(PhaseOneInvalidRequest)
	}
	raw, _, err := c.requestJSON(ctx, http.MethodDelete, "/v1/automation/schedules/"+current.ID, c.token, headers, map[string]any{"expected_revision": current.Revision}, http.StatusOK)
	if err != nil {
		return AutomationSchedule{}, err
	}
	return decodeAutomationScheduleEnvelope(raw)
}

func decodeAutomationPrincipal(wire automationPrincipalWire) (AutomationPrincipal, error) {
	if !validPhaseOnePublicID(wire.PrincipalID, "ap_") || !validPhaseOneDisplayText(wire.DisplayName, 128, false) ||
		wire.Permission != "trigger" || wire.MaxTargetCount < 1 || wire.MaxTargetCount > 64 || wire.Revision <= 0 ||
		len(wire.AllowedCueIDs) == 0 || len(wire.AllowedAudienceKinds) == 0 {
		return AutomationPrincipal{}, phaseOneError(PhaseOneInvalidResponse)
	}
	for _, id := range wire.AllowedCueIDs {
		if !validPhaseOnePublicID(id, "cq_") {
			return AutomationPrincipal{}, phaseOneError(PhaseOneInvalidResponse)
		}
	}
	for _, audience := range wire.AllowedAudienceKinds {
		if !validAutomationAudience(audience) {
			return AutomationPrincipal{}, phaseOneError(PhaseOneInvalidResponse)
		}
	}
	hasExplicit := false
	for _, audience := range wire.AllowedAudienceKinds {
		hasExplicit = hasExplicit || audience == AutomationExplicit
	}
	if hasExplicit != (len(wire.AllowedTargetDigests) > 0) {
		return AutomationPrincipal{}, phaseOneError(PhaseOneInvalidResponse)
	}
	for _, digest := range wire.AllowedTargetDigests {
		if !validAutomationDigest(digest) {
			return AutomationPrincipal{}, phaseOneError(PhaseOneInvalidResponse)
		}
	}
	issuedAt, issuedOK := decodeAutomationTime(wire.IssuedAt, false)
	expiresAt, expiresOK := decodeAutomationTime(wire.ExpiresAt, false)
	disabledAt, disabledOK := decodeAutomationTime(wire.DisabledAt, true)
	revokedAt, revokedOK := decodeAutomationTime(wire.RevokedAt, true)
	if !issuedOK || !expiresOK || !disabledOK || !revokedOK || !expiresAt.After(issuedAt) {
		return AutomationPrincipal{}, phaseOneError(PhaseOneInvalidResponse)
	}
	return AutomationPrincipal{ID: wire.PrincipalID, DisplayName: wire.DisplayName, Permission: wire.Permission,
		AirID: wire.BoundAirID, MaxTargetCount: wire.MaxTargetCount, AllowedCueIDs: append([]string(nil), wire.AllowedCueIDs...),
		AllowedAudiences: append([]AutomationAudience(nil), wire.AllowedAudienceKinds...), TargetDigests: append([]string(nil), wire.AllowedTargetDigests...),
		IssuedAt: issuedAt, ExpiresAt: expiresAt, DisabledAt: disabledAt, RevokedAt: revokedAt, Revision: wire.Revision}, nil
}

func (c *PhaseOneAppClient) AutomationPrincipals(ctx context.Context) ([]AutomationPrincipal, error) {
	raw, _, err := c.request(ctx, http.MethodGet, "/v1/automation/principals", c.token, nil, nil, true, http.StatusOK)
	if err != nil {
		return nil, err
	}
	var response struct {
		Principals []automationPrincipalWire `json:"principals"`
	}
	if decodePhaseOneJSON(raw, &response) != nil || len(response.Principals) > 128 {
		return nil, phaseOneError(PhaseOneInvalidResponse)
	}
	result := make([]AutomationPrincipal, 0, len(response.Principals))
	seen := map[string]bool{}
	for _, wire := range response.Principals {
		item, decodeErr := decodeAutomationPrincipal(wire)
		if decodeErr != nil || seen[item.ID] {
			return nil, phaseOneError(PhaseOneInvalidResponse)
		}
		seen[item.ID] = true
		result = append(result, item)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].DisplayName < result[j].DisplayName })
	return result, nil
}

func validAutomationPrincipalDraft(draft AutomationPrincipalDraft) bool {
	if !validPhaseOneDisplayText(draft.DisplayName, 128, false) || len(draft.AllowedCueIDs) == 0 ||
		len(draft.AllowedAudiences) == 0 || draft.MaxTargetCount < 1 || draft.MaxTargetCount > 64 ||
		!draft.ExpiresAt.After(time.Unix(0, 0)) {
		return false
	}
	seenCues := map[string]bool{}
	for _, id := range draft.AllowedCueIDs {
		if !validPhaseOnePublicID(id, "cq_") || seenCues[id] {
			return false
		}
		seenCues[id] = true
	}
	seenAudiences := map[AutomationAudience]bool{}
	hasExplicit := false
	for _, audience := range draft.AllowedAudiences {
		if !validAutomationAudience(audience) || seenAudiences[audience] {
			return false
		}
		seenAudiences[audience] = true
		hasExplicit = hasExplicit || audience == AutomationExplicit
	}
	if hasExplicit != (len(draft.TargetReferences) > 0) {
		return false
	}
	seenTargets := map[string]bool{}
	for _, reference := range draft.TargetReferences {
		if !targetReferencePattern.MatchString(reference) || seenTargets[reference] {
			return false
		}
		seenTargets[reference] = true
	}
	return true
}

func (c *PhaseOneAppClient) IssueAutomationPrincipal(ctx context.Context, draft AutomationPrincipalDraft, key string) (AutomationPrincipalIssue, error) {
	headers, err := automationKeyHeaders(key)
	if err != nil || !validAutomationPrincipalDraft(draft) {
		return AutomationPrincipalIssue{}, phaseOneError(PhaseOneInvalidRequest)
	}
	body := map[string]any{"display_name": draft.DisplayName, "allowed_cue_ids": draft.AllowedCueIDs,
		"allowed_audience_kinds": draft.AllowedAudiences, "allowed_target_references": draft.TargetReferences,
		"max_target_count": draft.MaxTargetCount, "expires_at": draft.ExpiresAt.UTC().Format(time.RFC3339)}
	if draft.AirID != "" {
		body["bound_air_id"] = draft.AirID
	}
	raw, _, err := c.requestJSON(ctx, http.MethodPost, "/v1/automation/principals", c.token, headers, body, http.StatusCreated)
	if err != nil {
		return AutomationPrincipalIssue{}, err
	}
	var response struct {
		Principal       automationPrincipalWire `json:"principal"`
		Secret          string                  `json:"secret"`
		SecretAvailable bool                    `json:"secret_available"`
	}
	if decodePhaseOneJSON(raw, &response) != nil {
		return AutomationPrincipalIssue{}, phaseOneError(PhaseOneInvalidResponse)
	}
	principal, decodeErr := decodeAutomationPrincipal(response.Principal)
	validSecret := len(response.Secret) == 64 && strings.Trim(response.Secret, "0123456789abcdef") == ""
	if decodeErr != nil || response.SecretAvailable && !validSecret || !response.SecretAvailable && response.Secret != "" {
		return AutomationPrincipalIssue{}, phaseOneError(PhaseOneInvalidResponse)
	}
	return AutomationPrincipalIssue{Principal: principal, Secret: response.Secret, SecretAvailable: response.SecretAvailable}, nil
}

func (c *PhaseOneAppClient) RevokeAutomationPrincipal(ctx context.Context, current AutomationPrincipal, key string) (AutomationPrincipal, error) {
	headers, err := automationKeyHeaders(key)
	if err != nil || !validPhaseOnePublicID(current.ID, "ap_") || current.Revision <= 0 {
		return AutomationPrincipal{}, phaseOneError(PhaseOneInvalidRequest)
	}
	raw, _, err := c.requestJSON(ctx, http.MethodPost, "/v1/automation/principals/"+current.ID+"/revoke", c.token, headers, map[string]any{"expected_revision": current.Revision}, http.StatusOK)
	if err != nil {
		return AutomationPrincipal{}, err
	}
	var response struct {
		Principal automationPrincipalWire `json:"principal"`
	}
	if decodePhaseOneJSON(raw, &response) != nil {
		return AutomationPrincipal{}, phaseOneError(PhaseOneInvalidResponse)
	}
	return decodeAutomationPrincipal(response.Principal)
}

func (c *PhaseOneAppClient) CancelAutomationHistory(ctx context.Context, historyID string) error {
	if !validPhaseOneHistoryID(historyID) {
		return phaseOneError(PhaseOneInvalidRequest)
	}
	_, _, err := c.requestJSON(ctx, http.MethodPost, "/v1/history/"+historyID+"/actions/cancel", c.token, nil, struct{}{}, http.StatusOK)
	return err
}
