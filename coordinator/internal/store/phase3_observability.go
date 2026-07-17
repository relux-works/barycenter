package store

import "time"

const Phase3ObservabilityWindow = 24 * time.Hour

type Phase3AutomationFeatureMetrics struct {
	ObservedScopes              int64 `json:"observed_scopes"`
	SoundboardEnabled           int64 `json:"soundboard_enabled_scopes"`
	AutomationEnabled           int64 `json:"automation_enabled_scopes"`
	EmergencyDisabled           int64 `json:"emergency_disabled_scopes"`
	AutomationEmergencyDisabled int64 `json:"automation_emergency_disabled_scopes"`
	InvalidCombinationScopes    int64 `json:"invalid_automation_without_soundboard_scopes"`
	MaximumRevision             int64 `json:"maximum_revision"`
	MostRecentUpdateAtMS        int64 `json:"most_recent_update_at_ms"`
}

type Phase3AutomationAttemptMetrics struct {
	Total24h               int64 `json:"total_24h"`
	Accepted24h            int64 `json:"accepted_24h"`
	Denied24h              int64 `json:"denied_24h"`
	ReservedCurrent        int64 `json:"reserved_current"`
	RateLimited24h         int64 `json:"rate_limited_24h"`
	ExecutionInProgress24h int64 `json:"execution_in_progress_24h"`
	QuietHours24h          int64 `json:"quiet_hours_24h"`
	PolicyDenied24h        int64 `json:"policy_denied_24h"`
}

type Phase3AutomationResourceMetrics struct {
	SchedulesTotal    int64 `json:"schedules_total"`
	SchedulesEnabled  int64 `json:"schedules_enabled"`
	PrincipalsActive  int64 `json:"principals_active"`
	PrincipalsRevoked int64 `json:"principals_revoked"`
	AuditEvents24h    int64 `json:"audit_events_24h"`
	ControlEvents24h  int64 `json:"control_events_24h"`
}

type Phase3AutomationObservabilitySnapshot struct {
	Feature   Phase3AutomationFeatureMetrics  `json:"feature"`
	Attempts  Phase3AutomationAttemptMetrics  `json:"attempts"`
	Resources Phase3AutomationResourceMetrics `json:"resources"`
}

func phase3AutomationObservabilitySnapshot(
	q phase2ObservabilityQuerier, now int64,
) (Phase3AutomationObservabilitySnapshot, error) {
	if now <= 0 {
		return Phase3AutomationObservabilitySnapshot{}, ErrStreamAccountingInvalid
	}
	cutoff := now - Phase3ObservabilityWindow.Milliseconds()
	var view Phase3AutomationObservabilitySnapshot
	if err := q.QueryRow(`SELECT COUNT(*),
COALESCE(SUM(soundboard_enabled), 0), COALESCE(SUM(automation_enabled), 0),
COALESCE(SUM(emergency_disabled), 0), COALESCE(MAX(revision), 0),
COALESCE(SUM(CASE WHEN automation_enabled = 1 AND emergency_disabled = 1 THEN 1 ELSE 0 END), 0),
COALESCE(SUM(CASE WHEN automation_enabled = 1 AND soundboard_enabled = 0 THEN 1 ELSE 0 END), 0),
COALESCE(MAX(updated_at), 0) FROM automation_feature_state`).Scan(
		&view.Feature.ObservedScopes, &view.Feature.SoundboardEnabled,
		&view.Feature.AutomationEnabled, &view.Feature.EmergencyDisabled,
		&view.Feature.MaximumRevision, &view.Feature.AutomationEmergencyDisabled,
		&view.Feature.InvalidCombinationScopes, &view.Feature.MostRecentUpdateAtMS,
	); err != nil {
		return Phase3AutomationObservabilitySnapshot{}, err
	}
	if err := q.QueryRow(`SELECT
COUNT(*),
COALESCE(SUM(CASE WHEN outcome = 'accepted' THEN 1 ELSE 0 END), 0),
COALESCE(SUM(CASE WHEN outcome = 'denied' THEN 1 ELSE 0 END), 0),
(SELECT COUNT(*) FROM automation_runtime_attempts WHERE outcome = 'reserved'),
COALESCE(SUM(CASE WHEN reason_code = 'too_many_attempts' THEN 1 ELSE 0 END), 0),
COALESCE(SUM(CASE WHEN reason_code = 'execution_in_progress' THEN 1 ELSE 0 END), 0),
COALESCE(SUM(CASE WHEN reason_code = 'quiet_hours' THEN 1 ELSE 0 END), 0),
COALESCE(SUM(CASE WHEN reason_code IN ('audience_not_allowed','air_policy_denied',
  'automation_capability_missing','delivery_capability_missing') THEN 1 ELSE 0 END), 0)
FROM automation_runtime_attempts WHERE attempted_at >= ? AND attempted_at <= ?`, cutoff, now).Scan(
		&view.Attempts.Total24h, &view.Attempts.Accepted24h, &view.Attempts.Denied24h,
		&view.Attempts.ReservedCurrent, &view.Attempts.RateLimited24h,
		&view.Attempts.ExecutionInProgress24h, &view.Attempts.QuietHours24h,
		&view.Attempts.PolicyDenied24h,
	); err != nil {
		return Phase3AutomationObservabilitySnapshot{}, err
	}
	if err := q.QueryRow(`SELECT
(SELECT COUNT(*) FROM automation_schedules),
(SELECT COUNT(*) FROM automation_schedules WHERE enabled = 1),
(SELECT COUNT(*) FROM automation_principals
  WHERE revoked_at = 0 AND disabled_at = 0 AND expires_at > ?),
(SELECT COUNT(*) FROM automation_principals WHERE revoked_at > 0),
(SELECT COUNT(*) FROM automation_audit_events WHERE created_at >= ? AND created_at <= ?),
(SELECT COUNT(*) FROM automation_audit_events
  WHERE event_kind = 'control' AND created_at >= ? AND created_at <= ?)`,
		now, cutoff, now, cutoff, now).Scan(
		&view.Resources.SchedulesTotal, &view.Resources.SchedulesEnabled,
		&view.Resources.PrincipalsActive, &view.Resources.PrincipalsRevoked,
		&view.Resources.AuditEvents24h, &view.Resources.ControlEvents24h,
	); err != nil {
		return Phase3AutomationObservabilitySnapshot{}, err
	}
	return view, nil
}

func (s *Store) Phase3AutomationObservabilitySnapshot(
	now int64,
) (Phase3AutomationObservabilitySnapshot, error) {
	return phase3AutomationObservabilitySnapshot(s.db, now)
}

func (s *Store) GetAuthorizedPhase3AutomationObservability(
	operatorID, bearer string, now int64,
) (Phase3AutomationObservabilitySnapshot, error) {
	if operatorID == "" || now <= 0 {
		return Phase3AutomationObservabilitySnapshot{}, ErrStreamAccountingInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return Phase3AutomationObservabilitySnapshot{}, err
	}
	defer tx.Rollback()
	operator, err := resolveModerationOperator(tx, bearer)
	if err != nil {
		return Phase3AutomationObservabilitySnapshot{}, err
	}
	if operator.ID != operatorID || !operator.Capabilities.List {
		return Phase3AutomationObservabilitySnapshot{}, ErrModerationForbidden
	}
	view, err := phase3AutomationObservabilitySnapshot(tx, now)
	if err != nil {
		return Phase3AutomationObservabilitySnapshot{}, err
	}
	if err := tx.Commit(); err != nil {
		return Phase3AutomationObservabilitySnapshot{}, err
	}
	return view, nil
}
