package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type WindowsAutomationScheduleProjection struct {
	Schedule         AutomationSchedule
	NextRun          time.Time
	NextRunAvailable bool
	QuietHoursSkip   bool
}

type WindowsAutomationSnapshot struct {
	Available         bool
	Feature           AutomationFeature
	Cues              []SoundboardCue
	Schedules         []WindowsAutomationScheduleProjection
	Principals        []AutomationPrincipal
	History           []PhaseOneHistoryItem
	SelectedSchedule  int
	SelectedPrincipal int
	SelectedHistory   int
	SecretAvailable   bool
	Busy              bool
	ConfirmAction     string
	Outcome           string
	Failure           string
}

type automationHistoryService interface {
	History(context.Context, int, string) (PhaseOneHistoryPage, error)
}

// WindowsAutomationAdmin owns only control state. Principal credentials are
// held in the private secret field until explicit copy/hide and never enter a
// snapshot, preference file, log, accessible label, error or String method.
type WindowsAutomationAdmin struct {
	mu      sync.RWMutex
	service AutomationAdminService
	history automationHistoryService
	ctx     context.Context
	cancel  context.CancelFunc
	wake    chan struct{}
	now     func() time.Time
	copy    func(string) bool
	secret  string
	epoch   uint64
	state   WindowsAutomationSnapshot
}

func NewWindowsAutomationAdmin(service AutomationAdminService, history automationHistoryService) (*WindowsAutomationAdmin, error) {
	return newWindowsAutomationAdmin(service, history, time.Now, copyAutomationSecretToClipboard)
}

func newWindowsAutomationAdmin(service AutomationAdminService, history automationHistoryService, now func() time.Time, copySecret func(string) bool) (*WindowsAutomationAdmin, error) {
	if service == nil || history == nil {
		return nil, phaseOneError(PhaseOneInvalidConfiguration)
	}
	if now == nil {
		now = time.Now
	}
	ctx, cancel := context.WithCancel(context.Background())
	result := &WindowsAutomationAdmin{service: service, history: history, ctx: ctx, cancel: cancel,
		wake: make(chan struct{}, 1), now: now, copy: copySecret,
		state: WindowsAutomationSnapshot{Failure: "refresh_pending"}}
	go result.run()
	result.Refresh()
	return result, nil
}

func newProductionWindowsAutomationAdmin(dir string) (*WindowsAutomationAdmin, error) {
	repository, err := newDefaultCredentialRepository(dir)
	if err != nil {
		return nil, err
	}
	bundle, err := repository.LoadBundle()
	if err != nil || bundle == nil {
		return nil, phaseOneError(PhaseOneInvalidConfiguration)
	}
	client, err := NewPhaseOneAppClient(*bundle, nil)
	if err != nil {
		return nil, err
	}
	return NewWindowsAutomationAdmin(client, client)
}

func (c *WindowsAutomationAdmin) String() string   { return "WindowsAutomationAdmin{<redacted>}" }
func (c *WindowsAutomationAdmin) GoString() string { return c.String() }

func (c *WindowsAutomationAdmin) Snapshot() WindowsAutomationSnapshot {
	if c == nil {
		return WindowsAutomationSnapshot{Failure: "automation_unavailable"}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := c.state
	result.Cues = append([]SoundboardCue(nil), c.state.Cues...)
	result.Schedules = append([]WindowsAutomationScheduleProjection(nil), c.state.Schedules...)
	result.Principals = append([]AutomationPrincipal(nil), c.state.Principals...)
	result.History = append([]PhaseOneHistoryItem(nil), c.state.History...)
	result.Feature.QuietHours = append([]AutomationQuietWindow(nil), c.state.Feature.QuietHours...)
	return result
}

func (c *WindowsAutomationAdmin) ApplyShellSnapshot(shell *ShellSnapshot) {
	if c == nil || shell == nil {
		return
	}
	state := c.Snapshot()
	shell.Automation = state
}

func (c *WindowsAutomationAdmin) Refresh() {
	if c == nil {
		return
	}
	select {
	case c.wake <- struct{}{}:
	default:
	}
}

func (c *WindowsAutomationAdmin) run() {
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.wake:
			c.refresh()
		case <-ticker.C:
			c.refresh()
		}
	}
}

func (c *WindowsAutomationAdmin) refresh() {
	c.mu.RLock()
	epoch := c.epoch
	c.mu.RUnlock()
	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()
	feature, featureErr := c.service.AutomationFeature(ctx)
	cues, cueErr := c.service.SoundboardCues(ctx)
	schedules, scheduleErr := c.service.AutomationSchedules(ctx)
	principals, principalErr := c.service.AutomationPrincipals(ctx)
	history, historyErr := c.history.History(ctx, 30, "")
	if featureErr != nil || cueErr != nil || scheduleErr != nil || principalErr != nil || historyErr != nil {
		c.mu.Lock()
		if c.epoch == epoch {
			c.secret = ""
			c.state = WindowsAutomationSnapshot{Failure: automationFailure(firstAutomationError(featureErr, cueErr, scheduleErr, principalErr, historyErr))}
		}
		c.mu.Unlock()
		return
	}
	now := c.now()
	projected := make([]WindowsAutomationScheduleProjection, 0, len(schedules))
	for _, schedule := range schedules {
		next, ok := windowsAutomationNextRun(schedule, now)
		projected = append(projected, WindowsAutomationScheduleProjection{Schedule: schedule, NextRun: next,
			NextRunAvailable: ok, QuietHoursSkip: ok && automationQuietAt(next, schedule.Timezone, append(append([]AutomationQuietWindow(nil), feature.QuietHours...), schedule.AdditionalQuietHours...))})
	}
	automationHistory := make([]PhaseOneHistoryItem, 0, len(history.Items))
	for _, item := range history.Items {
		if item.Automation != nil || phaseOneActionAllowed(item.Actions, "cancel") {
			automationHistory = append(automationHistory, item)
		}
	}
	c.mu.Lock()
	if c.epoch != epoch {
		c.mu.Unlock()
		return
	}
	c.state.Available, c.state.Feature = true, feature
	c.state.Cues, c.state.Schedules, c.state.Principals, c.state.History = cues.Cues, projected, principals, automationHistory
	if c.state.SelectedSchedule >= len(projected) {
		c.state.SelectedSchedule = 0
	}
	if c.state.SelectedPrincipal >= len(principals) {
		c.state.SelectedPrincipal = 0
	}
	if c.state.SelectedHistory >= len(automationHistory) {
		c.state.SelectedHistory = 0
	}
	c.state.Busy, c.state.Failure = false, ""
	c.mu.Unlock()
}

func firstAutomationError(values ...error) error {
	for _, value := range values {
		if value != nil {
			return value
		}
	}
	return nil
}

func windowsAutomationNextRun(schedule AutomationSchedule, now time.Time) (time.Time, bool) {
	if !schedule.Enabled || !validAutomationTimezone(schedule.Timezone) || !validAutomationLocalTime(schedule.LocalTime) || !validAutomationWeekdays(schedule.Weekdays) {
		return time.Time{}, false
	}
	location, _ := time.LoadLocation(schedule.Timezone)
	allowed := map[time.Weekday]bool{}
	for _, day := range schedule.Weekdays {
		allowed[time.Weekday(day)] = true
	}
	parsed, _ := time.Parse("15:04", schedule.LocalTime)
	minute := parsed.Hour()*60 + parsed.Minute()
	// UTC enumeration mirrors the runtime: spring gaps have no UTC mapping and
	// the earliest instant in a fall-back fold wins.
	candidate := now.UTC().Truncate(time.Minute).Add(time.Minute)
	limit := candidate.Add(8*24*time.Hour + 3*time.Hour)
	for !candidate.After(limit) {
		local := candidate.In(location)
		if allowed[local.Weekday()] && local.Hour()*60+local.Minute() == minute {
			return candidate, true
		}
		candidate = candidate.Add(time.Minute)
	}
	return time.Time{}, false
}

func automationQuietAt(instant time.Time, timezone string, windows []AutomationQuietWindow) bool {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return true
	}
	local := instant.In(location)
	weekday, minute := int(local.Weekday()), local.Hour()*60+local.Minute()
	for _, window := range windows {
		if window.EndMinute > window.StartMinute {
			if weekday == window.Weekday && minute >= window.StartMinute && minute < window.EndMinute {
				return true
			}
		} else if weekday == window.Weekday && minute >= window.StartMinute || weekday == (window.Weekday+1)%7 && minute < window.EndMinute {
			return true
		}
	}
	return false
}

func parseAutomationQuietHours(value string) ([]AutomationQuietWindow, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return []AutomationQuietWindow{}, nil
	}
	days := map[string]int{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}
	var result []AutomationQuietWindow
	for _, raw := range strings.Split(value, ";") {
		fields := strings.Fields(strings.TrimSpace(raw))
		if len(fields) != 2 {
			return nil, fmt.Errorf("quiet_hours_format")
		}
		day, ok := days[strings.ToLower(fields[0])]
		if !ok {
			return nil, fmt.Errorf("quiet_hours_weekday")
		}
		ends := strings.Split(fields[1], "-")
		if len(ends) != 2 || !validAutomationLocalTime(ends[0]) || !validAutomationLocalTime(ends[1]) {
			return nil, fmt.Errorf("quiet_hours_time")
		}
		toMinute := func(v string) int { parsed, _ := time.Parse("15:04", v); return parsed.Hour()*60 + parsed.Minute() }
		result = append(result, AutomationQuietWindow{Weekday: day, StartMinute: toMinute(ends[0]), EndMinute: toMinute(ends[1])})
	}
	if !validAutomationQuietWindows(result) {
		return nil, fmt.Errorf("quiet_hours_overlap")
	}
	return result, nil
}

func parseAutomationWeekdays(value string) ([]int, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return []int{0, 1, 2, 3, 4, 5, 6}, nil
	}
	days := map[string]int{"sun": 0, "mon": 1, "tue": 2, "wed": 3, "thu": 4, "fri": 5, "sat": 6}
	var result []int
	for _, field := range strings.Split(value, ",") {
		day, ok := days[strings.ToLower(strings.TrimSpace(field))]
		if !ok {
			return nil, fmt.Errorf("weekdays_format")
		}
		result = append(result, day)
	}
	if !validAutomationWeekdays(result) {
		return nil, fmt.Errorf("weekdays_duplicate")
	}
	return result, nil
}

func formatAutomationWeekdays(values []int) string {
	names := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value >= 0 && value < len(names) {
			result = append(result, names[value])
		}
	}
	return strings.Join(result, ",")
}

func formatAutomationQuietHours(values []AutomationQuietWindow) string {
	names := []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value.Weekday < 0 || value.Weekday >= len(names) {
			continue
		}
		result = append(result, fmt.Sprintf("%s %02d:%02d-%02d:%02d", names[value.Weekday], value.StartMinute/60, value.StartMinute%60, value.EndMinute/60, value.EndMinute%60))
	}
	return strings.Join(result, "; ")
}

func (c *WindowsAutomationAdmin) SelectNextSchedule() {
	c.mu.Lock()
	if len(c.state.Schedules) > 0 {
		c.state.SelectedSchedule = (c.state.SelectedSchedule + 1) % len(c.state.Schedules)
	}
	c.mu.Unlock()
}
func (c *WindowsAutomationAdmin) SelectNextPrincipal() {
	c.mu.Lock()
	if len(c.state.Principals) > 0 {
		c.state.SelectedPrincipal = (c.state.SelectedPrincipal + 1) % len(c.state.Principals)
	}
	c.mu.Unlock()
}
func (c *WindowsAutomationAdmin) SelectNextHistory() {
	c.mu.Lock()
	if len(c.state.History) > 0 {
		c.state.SelectedHistory = (c.state.SelectedHistory + 1) % len(c.state.History)
	}
	c.mu.Unlock()
}

func (c *WindowsAutomationAdmin) SaveFeature(timezone, quiet string) {
	windows, err := parseAutomationQuietHours(quiet)
	if err != nil || !validAutomationTimezone(timezone) {
		c.setFailure("invalid_quiet_hours")
		return
	}
	c.mu.RLock()
	feature := c.state.Feature
	c.mu.RUnlock()
	feature.Timezone, feature.QuietHours = timezone, windows
	c.mutate("feature", func(key string) (string, error) {
		_, err := c.service.ReplaceAutomationFeature(c.ctx, feature, key)
		return "feature_saved", err
	})
}

func (c *WindowsAutomationAdmin) SaveSchedule(name, timezone, weekdays, localTime, quiet string) {
	c.mu.RLock()
	state := c.state
	c.mu.RUnlock()
	parsedWeekdays, weekdayErr := parseAutomationWeekdays(weekdays)
	parsedQuiet, quietErr := parseAutomationQuietHours(quiet)
	if !validPhaseOneDisplayText(name, 128, false) || !validAutomationTimezone(timezone) || !validAutomationLocalTime(localTime) || weekdayErr != nil || quietErr != nil || len(state.Cues) == 0 {
		c.setFailure("invalid_schedule")
		return
	}
	draft := AutomationScheduleDraft{CueID: state.Cues[0].ID, DisplayName: name, Timezone: timezone,
		Weekdays: parsedWeekdays, LocalTime: localTime, Audience: AutomationOwnBarycenter,
		AdditionalQuietHours: parsedQuiet, PolicyRevision: state.Feature.Revision}
	var current *AutomationSchedule
	if len(state.Schedules) > 0 {
		selected := state.Schedules[state.SelectedSchedule].Schedule
		if selected.DisplayName == name {
			current = &selected
			draft.CueID, draft.Audience, draft.AirID = selected.CueID, selected.Audience, selected.AirID
		}
	}
	c.mutate("schedule-save", func(key string) (string, error) {
		if current == nil {
			_, err := c.service.CreateAutomationSchedule(c.ctx, draft, key)
			return "schedule_created_disarmed", err
		}
		_, err := c.service.ReplaceAutomationSchedule(c.ctx, *current, draft, key)
		return "schedule_saved", err
	})
}

func (c *WindowsAutomationAdmin) Request(action string) {
	allowed := map[string]bool{"schedule_toggle": true, "schedule_delete": true, "principal_issue": true, "principal_revoke": true, "emergency_disable": true, "automation_toggle": true, "history_cancel": true}
	if !allowed[action] {
		return
	}
	c.mu.Lock()
	if !c.state.Busy {
		c.state.ConfirmAction, c.state.Outcome, c.state.Failure = action, "", ""
	}
	c.mu.Unlock()
}

func (c *WindowsAutomationAdmin) CancelConfirmation() {
	c.mu.Lock()
	c.state.ConfirmAction = ""
	c.mu.Unlock()
}

func (c *WindowsAutomationAdmin) Confirm(principalName string) {
	c.mu.RLock()
	state := c.state
	action := state.ConfirmAction
	c.mu.RUnlock()
	switch action {
	case "schedule_toggle":
		if len(state.Schedules) == 0 {
			c.setFailure("schedule_unavailable")
			return
		}
		current := state.Schedules[state.SelectedSchedule].Schedule
		c.mutate("schedule-toggle", func(key string) (string, error) {
			_, err := c.service.SetAutomationScheduleEnabled(c.ctx, current, !current.Enabled, key)
			return map[bool]string{true: "schedule_disabled", false: "schedule_enabled"}[current.Enabled], err
		})
	case "schedule_delete":
		if len(state.Schedules) == 0 {
			c.setFailure("schedule_unavailable")
			return
		}
		current := state.Schedules[state.SelectedSchedule].Schedule
		c.mutate("schedule-delete", func(key string) (string, error) {
			_, err := c.service.DeleteAutomationSchedule(c.ctx, current, key)
			return "schedule_deleted", err
		})
	case "principal_issue":
		if len(state.Cues) == 0 || !validPhaseOneDisplayText(principalName, 128, false) {
			c.setFailure("invalid_principal")
			return
		}
		draft := AutomationPrincipalDraft{DisplayName: principalName, AllowedCueIDs: []string{state.Cues[0].ID}, AllowedAudiences: []AutomationAudience{AutomationOwnBarycenter}, TargetReferences: []string{}, MaxTargetCount: 1, ExpiresAt: c.now().Add(30 * 24 * time.Hour)}
		c.mutate("principal-issue", func(key string) (string, error) {
			issue, err := c.service.IssueAutomationPrincipal(c.ctx, draft, key)
			if err == nil {
				c.mu.Lock()
				c.secret = issue.Secret
				c.state.SecretAvailable = issue.SecretAvailable
				c.mu.Unlock()
			}
			return map[bool]string{true: "principal_issued_secret_once", false: "principal_issued_secret_unavailable"}[issue.SecretAvailable], err
		})
	case "principal_revoke":
		if len(state.Principals) == 0 {
			c.setFailure("principal_unavailable")
			return
		}
		current := state.Principals[state.SelectedPrincipal]
		c.mutate("principal-revoke", func(key string) (string, error) {
			_, err := c.service.RevokeAutomationPrincipal(c.ctx, current, key)
			return "principal_revoked", err
		})
	case "emergency_disable":
		feature := state.Feature
		feature.EmergencyDisabled = true
		c.mutate("emergency-disable", func(key string) (string, error) {
			_, err := c.service.ReplaceAutomationFeature(c.ctx, feature, key)
			return "automation_emergency_disabled", err
		})
	case "automation_toggle":
		feature := state.Feature
		feature.AutomationEnabled = !feature.AutomationEnabled
		if feature.AutomationEnabled {
			feature.EmergencyDisabled = false
		}
		c.mutate("automation-toggle", func(key string) (string, error) {
			_, err := c.service.ReplaceAutomationFeature(c.ctx, feature, key)
			return map[bool]string{true: "automation_disabled_manual_soundboard_available", false: "automation_enabled"}[state.Feature.AutomationEnabled], err
		})
	case "history_cancel":
		if len(state.History) == 0 || !phaseOneActionAllowed(state.History[state.SelectedHistory].Actions, "cancel") {
			c.setFailure("history_action_unavailable")
			return
		}
		id := state.History[state.SelectedHistory].ID
		c.mutate("history-cancel", func(string) (string, error) { return "pending_cancelled", c.service.CancelAutomationHistory(c.ctx, id) })
	}
}

func (c *WindowsAutomationAdmin) CopySecret() {
	c.mu.RLock()
	secret, available := c.secret, c.state.SecretAvailable
	copier := c.copy
	c.mu.RUnlock()
	if !available || secret == "" || copier == nil || !copier(secret) {
		c.setFailure("secret_copy_failed")
		return
	}
	c.mu.Lock()
	c.state.Outcome, c.state.Failure = "secret_copied_auto_clear", ""
	c.mu.Unlock()
}

func (c *WindowsAutomationAdmin) HideSecret() {
	c.mu.Lock()
	c.secret = ""
	c.state.SecretAvailable = false
	c.state.Outcome = "secret_hidden_not_recoverable"
	c.state.Failure = ""
	c.mu.Unlock()
	clearAutomationSecretClipboard()
}

func (c *WindowsAutomationAdmin) mutate(operation string, fn func(string) (string, error)) {
	c.mu.Lock()
	if c.state.Busy {
		c.mu.Unlock()
		return
	}
	c.state.Busy = true
	c.epoch++
	c.state.ConfirmAction = ""
	c.state.Outcome = ""
	c.state.Failure = ""
	c.mu.Unlock()
	key := automationMutationKey(operation)
	go func() {
		outcome, err := fn(key)
		c.mu.Lock()
		c.state.Busy = false
		if err != nil {
			c.state.Failure = automationFailure(err)
		} else {
			c.state.Outcome = outcome
		}
		c.mu.Unlock()
		if err == nil {
			c.Refresh()
		}
	}()
}

func (c *WindowsAutomationAdmin) setFailure(code string) {
	c.mu.Lock()
	c.state.Failure = code
	c.state.Outcome = ""
	c.mu.Unlock()
}

func (c *WindowsAutomationAdmin) Close() {
	if c == nil {
		return
	}
	c.cancel()
	c.HideSecret()
}

func automationMutationKey(operation string) string {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err == nil {
		return "windows-automation-" + operation + "-" + hex.EncodeToString(raw)
	}
	return "windows-automation-" + operation + "-fallback-" + strconv.FormatInt(time.Now().UnixNano(), 10) + "-" + strconv.FormatUint(automationFallback.Add(1), 10)
}

var automationFallback atomic.Uint64

func automationFailure(err error) string {
	if api, ok := err.(*PhaseOneClientError); ok {
		if api.Kind == PhaseOneRejected && api.Code != "" {
			return api.Code
		}
		switch api.Kind {
		case PhaseOneInvalidConfiguration:
			return "credential_unavailable"
		case PhaseOneInvalidRequest:
			return "invalid_request"
		case PhaseOneInvalidResponse:
			return "invalid_response"
		case PhaseOneRedirectRejected:
			return "redirect_rejected"
		case PhaseOneResponseTooLarge:
			return "response_too_large"
		}
	}
	return "coordinator_unavailable"
}
