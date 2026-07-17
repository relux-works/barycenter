package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAutomationAdminClientStrictControlAndOneTimeSecretContracts(t *testing.T) {
	cueID := "cq_" + strings.Repeat("A", 26)
	scheduleID := "sch_" + strings.Repeat("B", 26)
	principalID := "ap_" + strings.Repeat("C", 26)
	secret := strings.Repeat("de", 32)
	doer := &phaseOneScriptedDoer{handle: func(request *http.Request, index int) (*http.Response, error) {
		if request.Header.Get("Authorization") != "Bearer "+phaseOneTestBundle().Control.ControlToken || request.Header.Get("Origin") != "" {
			t.Fatalf("request %d authentication/origin=%v", index, request.Header)
		}
		switch index {
		case 0:
			return phaseOneJSONResponse(request, 200, `{"soundboard_enabled":true,"automation_enabled":true,"emergency_disabled":false,"timezone":"America/New_York","quiet_hours":[{"weekday":0,"start_minute":60,"end_minute":120}],"policy_version":"automation-safety-v1","revision":3,"policy_valid":true,"updated_at":"2026-07-17T00:00:00Z"}`), nil
		case 1:
			return phaseOneJSONResponse(request, 200, fmt.Sprintf(`{"schedules":[{"schedule_id":%q,"cue_id":%q,"display_name":"Morning","timezone":"America/New_York","weekdays":[0],"local_time":"01:30","audience":{"kind":"own_barycenter","target_digests":[],"air_id":""},"delivery":"overlay","additional_quiet_hours":[],"policy_version":"automation-safety-v1","policy_revision":3,"enabled":true,"revision":2,"created_at":"2026-07-16T00:00:00Z","updated_at":"2026-07-17T00:00:00Z"}]}`, scheduleID, cueID)), nil
		case 2:
			// A malicious/forward-compatible secret member is ignored rather than
			// entering the public principal model.
			return phaseOneJSONResponse(request, 200, fmt.Sprintf(`{"principals":[{"principal_id":%q,"display_name":"Kitchen","permission":"trigger","bound_air_id":"","max_target_count":1,"allowed_cue_ids":[%q],"allowed_audience_kinds":["own_barycenter"],"allowed_target_digests":[],"issued_at":"2026-07-16T00:00:00Z","expires_at":"2026-08-16T00:00:00Z","disabled_at":"","revoked_at":"","revision":1,"secret":%q}]}`, principalID, cueID, secret)), nil
		case 3:
			body, _ := io.ReadAll(request.Body)
			if request.Method != http.MethodPut || request.URL.Path != "/v1/automation/status" || request.Header.Get("Idempotency-Key") != "windows-automation-feature-test" || !strings.Contains(string(body), `"expected_revision":3`) {
				t.Fatalf("feature mutation=%s %s %s headers=%v", request.Method, request.URL, body, request.Header)
			}
			return phaseOneJSONResponse(request, 200, `{"soundboard_enabled":true,"automation_enabled":false,"emergency_disabled":false,"timezone":"America/New_York","quiet_hours":[],"policy_version":"automation-safety-v1","revision":4,"policy_valid":true,"updated_at":"2026-07-17T00:01:00Z","replayed":false}`), nil
		case 4:
			body, _ := io.ReadAll(request.Body)
			if request.Method != http.MethodPost || request.URL.Path != "/v1/automation/principals" || strings.Contains(string(body), "secret") || request.Header.Get("Idempotency-Key") == "" {
				t.Fatalf("principal issue=%s %s %s headers=%v", request.Method, request.URL, body, request.Header)
			}
			return phaseOneJSONResponse(request, 201, fmt.Sprintf(`{"principal":{"principal_id":%q,"display_name":"Kitchen two","permission":"trigger","bound_air_id":"","max_target_count":1,"allowed_cue_ids":[%q],"allowed_audience_kinds":["own_barycenter"],"allowed_target_digests":[],"issued_at":"2026-07-17T00:00:00Z","expires_at":"2026-08-17T00:00:00Z","disabled_at":"","revoked_at":"","revision":1},"secret_available":true,"secret":%q,"replayed":false}`, principalID, cueID, secret)), nil
		case 5:
			return phaseOneJSONResponse(request, 201, fmt.Sprintf(`{"principal":{"principal_id":%q,"display_name":"Kitchen two","permission":"trigger","bound_air_id":"","max_target_count":1,"allowed_cue_ids":[%q],"allowed_audience_kinds":["own_barycenter"],"allowed_target_digests":[],"issued_at":"2026-07-17T00:00:00Z","expires_at":"2026-08-17T00:00:00Z","disabled_at":"","revoked_at":"","revision":1},"secret_available":false,"replayed":true}`, principalID, cueID)), nil
		default:
			t.Fatalf("unexpected request %d", index)
			return nil, nil
		}
	}}
	client, err := NewPhaseOneAppClient(phaseOneTestBundle(), doer)
	if err != nil {
		t.Fatal(err)
	}
	feature, err := client.AutomationFeature(context.Background())
	if err != nil || feature.Revision != 3 || len(feature.QuietHours) != 1 {
		t.Fatalf("feature=%+v err=%v", feature, err)
	}
	schedules, err := client.AutomationSchedules(context.Background())
	if err != nil || len(schedules) != 1 || schedules[0].ID != scheduleID {
		t.Fatalf("schedules=%+v err=%v", schedules, err)
	}
	principals, err := client.AutomationPrincipals(context.Background())
	if err != nil || len(principals) != 1 || strings.Contains(fmt.Sprintf("%+v", principals), secret) {
		t.Fatalf("principals=%+v err=%v", principals, err)
	}
	feature.AutomationEnabled, feature.QuietHours = false, []AutomationQuietWindow{}
	if updated, err := client.ReplaceAutomationFeature(context.Background(), feature, "windows-automation-feature-test"); err != nil || updated.Revision != 4 || updated.AutomationEnabled {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	draft := AutomationPrincipalDraft{DisplayName: "Kitchen two", AllowedCueIDs: []string{cueID}, AllowedAudiences: []AutomationAudience{AutomationOwnBarycenter}, TargetReferences: []string{}, MaxTargetCount: 1, ExpiresAt: time.Date(2026, 8, 17, 0, 0, 0, 0, time.UTC)}
	issue, err := client.IssueAutomationPrincipal(context.Background(), draft, "windows-automation-principal-issue")
	if err != nil || !issue.SecretAvailable || issue.Secret != secret || strings.Contains(fmt.Sprintf("%+v", issue), secret) {
		t.Fatalf("issue=%+v err=%v", issue, err)
	}
	replay, err := client.IssueAutomationPrincipal(context.Background(), draft, "windows-automation-principal-replay")
	if err != nil || replay.SecretAvailable || replay.Secret != "" {
		t.Fatalf("replay=%+v err=%v", replay, err)
	}
}

func TestAutomationAdminClientRejectsInvalidTimezoneQuietHoursAndSecretShape(t *testing.T) {
	if validAutomationTimezone("Local") || validAutomationQuietWindows([]AutomationQuietWindow{{Weekday: 1, StartMinute: 30, EndMinute: 30}}) {
		t.Fatal("invalid timezone/quiet window accepted")
	}
	if _, err := parseAutomationQuietHours("Mon 22:00-06:00; Mon 23:00-01:00"); err == nil {
		t.Fatal("overlapping cross-midnight quiet hours accepted")
	}
	weekdays, err := parseAutomationWeekdays("Mon,Wed,Fri")
	if err != nil || formatAutomationWeekdays(weekdays) != "Mon,Wed,Fri" {
		t.Fatalf("weekday round trip=%v err=%v", weekdays, err)
	}
	quiet, err := parseAutomationQuietHours("Mon 22:00-06:00; Tue 12:00-13:00")
	if err != nil || formatAutomationQuietHours(quiet) != "Mon 22:00-06:00; Tue 12:00-13:00" {
		t.Fatalf("quiet round trip=%v err=%v", quiet, err)
	}
	client, _ := NewPhaseOneAppClient(phaseOneTestBundle(), &phaseOneScriptedDoer{handle: func(request *http.Request, index int) (*http.Response, error) {
		return phaseOneJSONResponse(request, 200, `{"soundboard_enabled":true,"automation_enabled":true,"emergency_disabled":false,"timezone":"Invalid/Zone","quiet_hours":[],"policy_version":"automation-safety-v1","revision":1,"policy_valid":true,"updated_at":"2026-07-17T00:00:00Z"}`), nil
	}})
	if _, err := client.AutomationFeature(context.Background()); err == nil {
		t.Fatal("invalid coordinator timezone accepted")
	}
}
