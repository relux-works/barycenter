package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

type automationAdminFake struct {
	mu         sync.Mutex
	feature    AutomationFeature
	cues       SoundboardCueList
	schedules  []AutomationSchedule
	principals []AutomationPrincipal
	history    PhaseOneHistoryPage
	secret     string
	cancelled  bool
	featureErr error
}

func (f *automationAdminFake) AutomationFeature(context.Context) (AutomationFeature, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.feature, f.featureErr
}
func (f *automationAdminFake) SoundboardCues(context.Context) (SoundboardCueList, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.cues, nil
}
func (f *automationAdminFake) AutomationSchedules(context.Context) ([]AutomationSchedule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]AutomationSchedule(nil), f.schedules...), nil
}
func (f *automationAdminFake) AutomationPrincipals(context.Context) ([]AutomationPrincipal, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]AutomationPrincipal(nil), f.principals...), nil
}
func (f *automationAdminFake) History(context.Context, int, string) (PhaseOneHistoryPage, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.history, nil
}
func (f *automationAdminFake) ReplaceAutomationFeature(_ context.Context, v AutomationFeature, _ string) (AutomationFeature, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v.Revision++
	f.feature = v
	return v, nil
}
func (f *automationAdminFake) CreateAutomationSchedule(_ context.Context, d AutomationScheduleDraft, _ string) (AutomationSchedule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v := AutomationSchedule{ID: "sch_" + strings.Repeat("D", 26), CueID: d.CueID, DisplayName: d.DisplayName, Timezone: d.Timezone, Weekdays: d.Weekdays, LocalTime: d.LocalTime, Audience: d.Audience, PolicyVersion: "automation-safety-v1", PolicyRevision: d.PolicyRevision, Revision: 1}
	f.schedules = append(f.schedules, v)
	return v, nil
}
func (f *automationAdminFake) ReplaceAutomationSchedule(_ context.Context, v AutomationSchedule, d AutomationScheduleDraft, _ string) (AutomationSchedule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v.DisplayName, v.Timezone, v.LocalTime = d.DisplayName, d.Timezone, d.LocalTime
	v.Weekdays, v.AdditionalQuietHours = d.Weekdays, d.AdditionalQuietHours
	v.Revision++
	f.schedules[0] = v
	return v, nil
}
func (f *automationAdminFake) SetAutomationScheduleEnabled(_ context.Context, v AutomationSchedule, enabled bool, _ string) (AutomationSchedule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v.Enabled = enabled
	v.Revision++
	f.schedules[0] = v
	return v, nil
}
func (f *automationAdminFake) DeleteAutomationSchedule(_ context.Context, v AutomationSchedule, _ string) (AutomationSchedule, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.schedules = nil
	v.Enabled = false
	v.Revision++
	return v, nil
}
func (f *automationAdminFake) IssueAutomationPrincipal(_ context.Context, d AutomationPrincipalDraft, _ string) (AutomationPrincipalIssue, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v := AutomationPrincipal{ID: "ap_" + strings.Repeat("E", 26), DisplayName: d.DisplayName, Permission: "trigger", MaxTargetCount: d.MaxTargetCount, AllowedCueIDs: d.AllowedCueIDs, AllowedAudiences: d.AllowedAudiences, IssuedAt: time.Date(2026, 7, 17, 0, 0, 0, 0, time.UTC), ExpiresAt: d.ExpiresAt, Revision: 1}
	f.principals = append(f.principals, v)
	return AutomationPrincipalIssue{Principal: v, Secret: f.secret, SecretAvailable: true}, nil
}
func (f *automationAdminFake) RevokeAutomationPrincipal(_ context.Context, v AutomationPrincipal, _ string) (AutomationPrincipal, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v.RevokedAt = time.Date(2026, 7, 17, 1, 0, 0, 0, time.UTC)
	v.Revision++
	f.principals[0] = v
	return v, nil
}
func (f *automationAdminFake) CancelAutomationHistory(context.Context, string) error {
	f.mu.Lock()
	f.cancelled = true
	f.mu.Unlock()
	return nil
}

func waitAutomationState(t *testing.T, admin *WindowsAutomationAdmin, predicate func(WindowsAutomationSnapshot) bool) WindowsAutomationSnapshot {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		state := admin.Snapshot()
		if predicate(state) {
			return state
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out: %+v", admin.Snapshot())
	return WindowsAutomationSnapshot{}
}

func TestWindowsAutomationAdminDSTQuietHoursSecretsAndManualSoundboardSeparation(t *testing.T) {
	cueID := "cq_" + strings.Repeat("A", 26)
	scheduleID := "sch_" + strings.Repeat("B", 26)
	historyID := "hi_" + strings.Repeat("C", 26)
	secret := strings.Repeat("ab", 32)
	fake := &automationAdminFake{feature: AutomationFeature{SoundboardEnabled: true, AutomationEnabled: true, Timezone: "America/New_York", QuietHours: []AutomationQuietWindow{{Weekday: 0, StartMinute: 60, EndMinute: 120}}, PolicyVersion: "automation-safety-v1", Revision: 3, PolicyValid: true}, cues: SoundboardCueList{Cues: []SoundboardCue{{ID: cueID, Title: "Bell"}}}, schedules: []AutomationSchedule{{ID: scheduleID, CueID: cueID, DisplayName: "Fold", Timezone: "America/New_York", Weekdays: []int{0}, LocalTime: "01:30", Audience: AutomationOwnBarycenter, PolicyVersion: "automation-safety-v1", PolicyRevision: 3, Enabled: true, Revision: 1}}, history: PhaseOneHistoryPage{Items: []PhaseOneHistoryItem{{ID: historyID, Title: "Bell", Status: "pending", Actions: []string{"cancel"}}}}, secret: secret}
	admin, err := newWindowsAutomationAdmin(fake, fake, func() time.Time { return time.Date(2026, 11, 1, 4, 59, 0, 0, time.UTC) }, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	state := waitAutomationState(t, admin, func(s WindowsAutomationSnapshot) bool { return s.Available && len(s.Schedules) == 1 })
	if !state.Schedules[0].NextRun.Equal(time.Date(2026, 11, 1, 5, 30, 0, 0, time.UTC)) || !state.Schedules[0].QuietHoursSkip {
		t.Fatalf("DST projection=%+v", state.Schedules[0])
	}
	admin.SaveSchedule("Fold", "America/New_York", "Sun", "01:45", "Sun 00:30-01:00")
	state = waitAutomationState(t, admin, func(s WindowsAutomationSnapshot) bool {
		return !s.Busy && len(s.Schedules) == 1 && s.Schedules[0].Schedule.LocalTime == "01:45"
	})
	if state.Schedules[0].Schedule.Revision != 2 || formatAutomationWeekdays(state.Schedules[0].Schedule.Weekdays) != "Sun" || len(state.Schedules[0].Schedule.AdditionalQuietHours) != 1 {
		t.Fatalf("schedule edit=%+v", state.Schedules[0].Schedule)
	}
	admin.SaveSchedule("Second", "UTC", "Mon,Wed,Fri", "09:15", "")
	state = waitAutomationState(t, admin, func(s WindowsAutomationSnapshot) bool { return !s.Busy && len(s.Schedules) == 2 })
	body := NewShellCopy(ShellEnglish).AutomationProjection(ShellSnapshot{Automation: state})
	for _, required := range []string{"first UTC mapping", "skipped by quiet hours", "Manual Soundboard"} {
		if !strings.Contains(body, required) {
			t.Fatalf("body=%q missing %q", body, required)
		}
	}
	admin.Request("automation_toggle")
	if admin.Snapshot().ConfirmAction != "automation_toggle" {
		t.Fatal("toggle lacked explicit confirmation")
	}
	admin.Confirm("")
	state = waitAutomationState(t, admin, func(s WindowsAutomationSnapshot) bool { return !s.Busy && !s.Feature.AutomationEnabled })
	manual := NewShellCopy(ShellEnglish).Body(ShellSoundboard, ShellSnapshot{SoundboardCues: []ShellSoundboardCue{{Title: "Bell", SourceKind: "builtin"}}, SoundboardRoute: PhaseOneOwnBarycenter, SoundboardDelivery: PhaseOneOverlay, Automation: state})
	if !strings.Contains(manual, "Bell") {
		t.Fatalf("manual soundboard disappeared: %q", manual)
	}
	admin.Request("principal_issue")
	admin.Confirm("Kitchen")
	state = waitAutomationState(t, admin, func(s WindowsAutomationSnapshot) bool { return s.SecretAvailable })
	if strings.Contains(fmt.Sprintf("%+v", state), secret) || strings.Contains(admin.String(), secret) {
		t.Fatal("secret leaked through snapshot/string")
	}
	var copied string
	admin.mu.Lock()
	admin.copy = func(value string) bool { copied = value; return true }
	admin.mu.Unlock()
	admin.CopySecret()
	if copied != secret {
		t.Fatalf("copied=%q", copied)
	}
	admin.HideSecret()
	if admin.Snapshot().SecretAvailable || admin.secret != "" {
		t.Fatal("secret survived hide")
	}
	admin.Request("history_cancel")
	admin.Confirm("")
	waitAutomationState(t, admin, func(s WindowsAutomationSnapshot) bool { return !s.Busy && s.Outcome == "pending_cancelled" })
	fake.mu.Lock()
	cancelled := fake.cancelled
	fake.mu.Unlock()
	if !cancelled {
		t.Fatal("pending history not cancelled")
	}
}

func TestWindowsAutomationAdminUnauthorizedProjectionRevealsNothing(t *testing.T) {
	fake := &automationAdminFake{featureErr: &PhaseOneClientError{Kind: PhaseOneRejected, Code: "forbidden"}}
	admin, err := newWindowsAutomationAdmin(fake, fake, time.Now, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	state := waitAutomationState(t, admin, func(s WindowsAutomationSnapshot) bool { return s.Failure == "forbidden" })
	if state.Available || len(state.Schedules) != 0 || len(state.Principals) != 0 || state.SecretAvailable {
		t.Fatalf("denied state retained configuration: %+v", state)
	}
	body := NewShellCopy(ShellEnglish).AutomationProjection(ShellSnapshot{Automation: state})
	if !strings.Contains(body, "not disclosed") || strings.Contains(body, "schedule") || strings.Contains(body, "principal") {
		t.Fatalf("unauthorized projection inferred config: %q", body)
	}
}
