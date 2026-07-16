package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	automationcontract "relux.works/duet/coordinator/internal/automation"
	"relux.works/duet/coordinator/internal/store"
)

func automationAPIRequest(handler http.Handler, method, path, body, bearer, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:34567"
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func automationResponseObject(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	return decodeObject(t, response)
}

func TestAutomationControlMutationAuthBindsMethodAndPath(t *testing.T) {
	actor := actorRequest{Context: store.ActorContext{ActorID: 7}, Bearer: strings.Repeat("a", 64)}
	request := expectedRevisionHTTPRequest{ExpectedRevision: 1}
	firstRequest := httptest.NewRequest(http.MethodPost, "/v1/automation/schedules/sch_01J00000000000000000000000/enable", nil)
	firstRequest.Header.Set("Idempotency-Key", "automation-route-bound-0001")
	secondRequest := httptest.NewRequest(http.MethodPost, "/v1/automation/schedules/sch_01J00000000000000000000001/enable", nil)
	secondRequest.Header.Set("Idempotency-Key", "automation-route-bound-0001")
	first, firstOK := automationControlMutationAuth(firstRequest, actor, request, time.Now())
	second, secondOK := automationControlMutationAuth(secondRequest, actor, request, time.Now())
	if !firstOK || !secondOK || first.IdempotencyKeyHash != second.IdempotencyKeyHash ||
		first.RequestHash == second.RequestHash {
		t.Fatalf("route-bound auth first=%+v second=%+v", first, second)
	}
}

func TestAutomationControlHTTPCueSchedulePrincipalLifecycleAndSecretRedaction(t *testing.T) {
	harness := newOnboardingHarness(t)
	created := createViaAPI(t, harness)
	control := created["control_token"].(string)
	now := time.Now().UTC().Truncate(time.Second)
	harness.api.automationNow = func() time.Time { return now }

	status := automationAPIRequest(harness.mux, http.MethodGet,
		"/v1/automation/status", "", control, "")
	if status.Code != http.StatusOK || automationResponseObject(t, status)["revision"].(float64) != 0 {
		t.Fatalf("initial status=%d body=%q", status.Code, status.Body.String())
	}
	featureBody := `{"soundboard_enabled":true,"automation_enabled":true,"emergency_disabled":false,"timezone":"UTC","quiet_hours":[],"expected_revision":0}`
	feature := automationAPIRequest(harness.mux, http.MethodPut,
		"/v1/automation/status", featureBody, control, "feature1")
	if feature.Code != http.StatusOK || automationResponseObject(t, feature)["revision"].(float64) != 1 {
		t.Fatalf("feature status=%d body=%q", feature.Code, feature.Body.String())
	}

	cueBody := `{"title":"Recording cue","source":{"kind":"builtin","asset_id":"pulsar.recording-cue.v1","sha256":"479b1a9d605ac12454e3449e129991b7ce8599251506ca54a93be0b6144730fd"}}`
	cueResponse := automationAPIRequest(harness.mux, http.MethodPost,
		"/v1/soundboard/cues", cueBody, control, "automation-cue-create-0001")
	if cueResponse.Code != http.StatusCreated {
		t.Fatalf("cue status=%d body=%q", cueResponse.Code, cueResponse.Body.String())
	}
	cueObject := automationResponseObject(t, cueResponse)
	cueID := cueObject["cue"].(map[string]any)["cue_id"].(string)
	if !savedCueHTTPIDPattern.MatchString(cueID) {
		t.Fatalf("cue id=%q", cueID)
	}
	cueReplay := automationAPIRequest(harness.mux, http.MethodPost,
		"/v1/soundboard/cues", cueBody, control, "automation-cue-create-0001")
	if cueReplay.Code != http.StatusCreated ||
		automationResponseObject(t, cueReplay)["replayed"] != true {
		t.Fatalf("cue replay status=%d body=%q", cueReplay.Code, cueReplay.Body.String())
	}
	cueConflict := automationAPIRequest(harness.mux, http.MethodPost,
		"/v1/soundboard/cues", strings.Replace(cueBody, "Recording cue", "Other", 1),
		control, "automation-cue-create-0001")
	assertAPIError(t, cueConflict, http.StatusConflict, errorAirIdempotency, nil)

	rename := automationAPIRequest(harness.mux, http.MethodPatch,
		"/v1/soundboard/cues/"+cueID,
		`{"title":"Door bell","expected_revision":1}`, control,
		"automation-cue-rename-0001")
	if rename.Code != http.StatusOK {
		t.Fatalf("rename status=%d body=%q", rename.Code, rename.Body.String())
	}
	reorder := automationAPIRequest(harness.mux, http.MethodPut,
		"/v1/soundboard/cues/order",
		`{"expected_order_revision":1,"cue_ids":["`+cueID+`"]}`, control,
		"automation-cue-order-0001")
	if reorder.Code != http.StatusOK ||
		automationResponseObject(t, reorder)["order_revision"].(float64) != 2 {
		t.Fatalf("reorder status=%d body=%q", reorder.Code, reorder.Body.String())
	}

	scheduleBody := `{"cue_id":"` + cueID + `","display_name":"Morning","timezone":"UTC","weekdays":[1,3,5],"local_time":"08:30","audience":{"kind":"own_barycenter"},"delivery":"overlay","additional_quiet_hours":[],"policy_revision":1}`
	scheduleResponse := automationAPIRequest(harness.mux, http.MethodPost,
		"/v1/automation/schedules", scheduleBody, control,
		"automation-schedule-create-0001")
	if scheduleResponse.Code != http.StatusCreated {
		t.Fatalf("schedule status=%d body=%q", scheduleResponse.Code, scheduleResponse.Body.String())
	}
	schedule := automationResponseObject(t, scheduleResponse)["schedule"].(map[string]any)
	if schedule["enabled"] != false {
		t.Fatalf("new schedule unexpectedly armed: %v", schedule)
	}
	scheduleID := schedule["schedule_id"].(string)
	updatedScheduleBody := `{"cue_id":"` + cueID + `","display_name":"Weekday morning","timezone":"UTC","weekdays":[1,2,3,4,5],"local_time":"08:45","audience":{"kind":"own_barycenter"},"delivery":"overlay","additional_quiet_hours":[{"weekday":0,"start_minute":0,"end_minute":60}],"policy_revision":1,"expected_revision":1}`
	updatedSchedule := automationAPIRequest(harness.mux, http.MethodPut,
		"/v1/automation/schedules/"+scheduleID, updatedScheduleBody, control,
		"automation-schedule-update-0001")
	if updatedSchedule.Code != http.StatusOK ||
		automationResponseObject(t, updatedSchedule)["schedule"].(map[string]any)["revision"].(float64) != 2 {
		t.Fatalf("update schedule status=%d body=%q", updatedSchedule.Code, updatedSchedule.Body.String())
	}
	enable := automationAPIRequest(harness.mux, http.MethodPost,
		"/v1/automation/schedules/"+scheduleID+"/enable",
		`{"expected_revision":2}`, control, "automation-schedule-enable-0001")
	if enable.Code != http.StatusOK {
		t.Fatalf("enable status=%d body=%q", enable.Code, enable.Body.String())
	}
	disable := automationAPIRequest(harness.mux, http.MethodPost,
		"/v1/automation/schedules/"+scheduleID+"/disable",
		`{"expected_revision":3}`, control, "automation-schedule-disable-0001")
	if disable.Code != http.StatusOK {
		t.Fatalf("disable status=%d body=%q", disable.Code, disable.Body.String())
	}
	reenable := automationAPIRequest(harness.mux, http.MethodPost,
		"/v1/automation/schedules/"+scheduleID+"/enable",
		`{"expected_revision":4}`, control, "automation-schedule-reenable-0001")
	if reenable.Code != http.StatusOK {
		t.Fatalf("re-enable status=%d body=%q", reenable.Code, reenable.Body.String())
	}

	expiresAt := now.Add(24 * time.Hour).Format(time.RFC3339)
	principalBody := `{"display_name":"Home automation","allowed_cue_ids":["` + cueID + `"],"allowed_audience_kinds":["own_barycenter"],"allowed_target_references":[],"max_target_count":1,"expires_at":"` + expiresAt + `"}`
	principalResponse := automationAPIRequest(harness.mux, http.MethodPost,
		"/v1/automation/principals", principalBody, control,
		"automation-principal-issue-0001")
	if principalResponse.Code != http.StatusCreated {
		t.Fatalf("principal status=%d body=%q", principalResponse.Code, principalResponse.Body.String())
	}
	principalObject := automationResponseObject(t, principalResponse)
	secret := principalObject["secret"].(string)
	principalID := principalObject["principal"].(map[string]any)["principal_id"].(string)
	if len(secret) != 64 || principalObject["secret_available"] != true {
		t.Fatalf("principal response=%v", principalObject)
	}
	if strings.Contains(harness.logs.String(), secret) {
		t.Fatal("one-time automation secret was written to logs")
	}
	principalReplay := automationAPIRequest(harness.mux, http.MethodPost,
		"/v1/automation/principals", principalBody, control,
		"automation-principal-issue-0001")
	if principalReplay.Code != http.StatusCreated {
		t.Fatalf("principal replay status=%d body=%q", principalReplay.Code, principalReplay.Body.String())
	}
	replayObject := automationResponseObject(t, principalReplay)
	if replayObject["replayed"] != true || replayObject["secret_available"] != false {
		t.Fatalf("principal replay=%v", replayObject)
	}
	if _, exists := replayObject["secret"]; exists || strings.Contains(principalReplay.Body.String(), secret) {
		t.Fatalf("principal replay recovered secret body=%q", principalReplay.Body.String())
	}
	principalList := automationAPIRequest(harness.mux, http.MethodGet,
		"/v1/automation/principals", "", control, "")
	if principalList.Code != http.StatusOK || strings.Contains(principalList.Body.String(), secret) ||
		strings.Contains(principalList.Body.String(), "secret_hash") {
		t.Fatalf("principal list status=%d body=%q", principalList.Code, principalList.Body.String())
	}
	revoke := automationAPIRequest(harness.mux, http.MethodPost,
		"/v1/automation/principals/"+principalID+"/revoke",
		`{"expected_revision":1}`, control, "automation-principal-revoke-0001")
	if revoke.Code != http.StatusOK {
		t.Fatalf("revoke status=%d body=%q", revoke.Code, revoke.Body.String())
	}
	if _, err := harness.store.ResolveAutomationPrincipalSecret(secret, now.Add(time.Minute).UnixMilli()); !errors.Is(err, store.ErrAutomationInvalidCredential) {
		t.Fatalf("revoked secret resolution=%v", err)
	}

	deleteSchedule := automationAPIRequest(harness.mux, http.MethodDelete,
		"/v1/automation/schedules/"+scheduleID,
		`{"expected_revision":5}`, control, "automation-schedule-delete-0001")
	if deleteSchedule.Code != http.StatusOK {
		t.Fatalf("delete schedule status=%d body=%q", deleteSchedule.Code, deleteSchedule.Body.String())
	}
	deleteCue := automationAPIRequest(harness.mux, http.MethodDelete,
		"/v1/soundboard/cues/"+cueID,
		`{"expected_revision":2}`, control, "automation-cue-delete-0001")
	if deleteCue.Code != http.StatusOK {
		t.Fatalf("delete cue status=%d body=%q", deleteCue.Code, deleteCue.Body.String())
	}
}

func TestAutomationControlHTTPRequiresExplicitEmptyPolicyAndScopeLists(t *testing.T) {
	harness := newOnboardingHarness(t)
	created := createViaAPI(t, harness)
	control := created["control_token"].(string)
	expiresAt := time.Now().UTC().Add(24 * time.Hour).Format(time.RFC3339)

	tests := []struct {
		name   string
		method string
		path   string
		body   string
	}{
		{
			name: "feature quiet hours", method: http.MethodPut, path: "/v1/automation/status",
			body: `{"soundboard_enabled":true,"automation_enabled":false,"emergency_disabled":false,"timezone":"UTC","expected_revision":0}`,
		},
		{
			name: "feature expected revision", method: http.MethodPut, path: "/v1/automation/status",
			body: `{"soundboard_enabled":true,"automation_enabled":false,"emergency_disabled":false,"timezone":"UTC","quiet_hours":[]}`,
		},
		{
			name: "feature boolean", method: http.MethodPut, path: "/v1/automation/status",
			body: `{"soundboard_enabled":true,"emergency_disabled":false,"timezone":"UTC","quiet_hours":[],"expected_revision":0}`,
		},
		{
			name: "feature timezone", method: http.MethodPut, path: "/v1/automation/status",
			body: `{"soundboard_enabled":true,"automation_enabled":false,"emergency_disabled":false,"quiet_hours":[],"expected_revision":0}`,
		},
		{
			name: "cue order", method: http.MethodPut, path: "/v1/soundboard/cues/order",
			body: `{"expected_order_revision":1}`,
		},
		{
			name: "schedule additional quiet hours", method: http.MethodPost, path: "/v1/automation/schedules",
			body: `{"cue_id":"cq_01J00000000000000000000000","display_name":"Morning","timezone":"UTC","weekdays":[1],"local_time":"08:30","audience":{"kind":"own_barycenter"},"delivery":"overlay","policy_revision":1}`,
		},
		{
			name: "principal target references", method: http.MethodPost, path: "/v1/automation/principals",
			body: `{"display_name":"Home","allowed_cue_ids":["cq_01J00000000000000000000000"],"allowed_audience_kinds":["own_barycenter"],"max_target_count":1,"expires_at":"` + expiresAt + `"}`,
		},
	}
	for index, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			response := automationAPIRequest(harness.mux, test.method, test.path,
				test.body, control, "automation-required-field-000"+strconv.Itoa(index+1))
			assertAPIError(t, response, http.StatusBadRequest, errorInvalidRequest, nil)
		})
	}
}

type fakeAutomationTrigger struct {
	mu     sync.Mutex
	inputs []automationTriggerInput
	result automationTriggerOutput
	err    error
}

func (fake *fakeAutomationTrigger) TriggerAutomation(input automationTriggerInput) (automationTriggerOutput, error) {
	fake.mu.Lock()
	defer fake.mu.Unlock()
	fake.inputs = append(fake.inputs, input)
	return fake.result, fake.err
}

func TestAutomationTriggerBoundaryAbsentOriginStrictJSONAndCredentialCollapse(t *testing.T) {
	harness := newOnboardingHarness(t)
	harness.api.automationTrigger = nil
	dark := httptest.NewRequest(http.MethodPost, automationcontract.TriggerPath+"?probe=1",
		strings.NewReader(`{"cue_id":null}`))
	dark.RemoteAddr = "127.0.0.1:34567"
	dark.Header.Set("Origin", "https://attacker.example")
	darkResponse := httptest.NewRecorder()
	harness.mux.ServeHTTP(darkResponse, dark)
	if darkResponse.Code != http.StatusNotFound || darkResponse.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("dark status=%d headers=%v", darkResponse.Code, darkResponse.Header())
	}

	now := time.Now().UTC()
	secret := strings.Repeat("a", 64)
	executionID := "ax_01J00000000000000000000000"
	fake := &fakeAutomationTrigger{result: automationTriggerOutput{
		Execution: store.AutomationExecution{ID: executionID, Status: "claimed"},
	}}
	harness.api.automationTrigger = fake
	harness.api.automationNow = func() time.Time { return now }
	body := `{"cue_id":"cq_01J00000000000000000000000","audience":{"kind":"own_barycenter"},"delivery":"overlay"}`
	success := automationAPIRequest(harness.mux, http.MethodPost,
		automationcontract.TriggerPath, body, secret, "automation-trigger-key-0001")
	if success.Code != http.StatusAccepted ||
		automationResponseObject(t, success)["execution_id"] != executionID {
		t.Fatalf("trigger status=%d body=%q", success.Code, success.Body.String())
	}
	fake.mu.Lock()
	if len(fake.inputs) != 1 || fake.inputs[0].Secret != secret ||
		fake.inputs[0].AudienceKind != automationcontract.AudienceOwnBarycenter {
		t.Fatalf("inputs=%+v", fake.inputs)
	}
	fake.mu.Unlock()

	invalidBodies := []string{
		`{"cue_id":"cq_01J00000000000000000000000","cue_id":"cq_01J00000000000000000000000","audience":{"kind":"own_barycenter"},"delivery":"overlay"}`,
		`{"cue_id":null,"audience":{"kind":"own_barycenter"},"delivery":"overlay"}`,
		`{"cue_id":"cq_01J00000000000000000000000","audience":{"kind":"own_barycenter"},"delivery":"overlay","force":true}`,
	}
	for index, invalid := range invalidBodies {
		response := automationAPIRequest(harness.mux, http.MethodPost,
			automationcontract.TriggerPath, invalid, secret,
			"automation-trigger-invalid-"+string(rune('a'+index))+"001")
		assertAPIError(t, response, http.StatusBadRequest, errorInvalidRequest, nil)
	}
	shortKey := automationAPIRequest(harness.mux, http.MethodPost,
		automationcontract.TriggerPath, body, secret, "short")
	assertAPIError(t, shortKey, http.StatusBadRequest, errorInvalidRequest, nil)
	cookieRequest := httptest.NewRequest(http.MethodPost, automationcontract.TriggerPath, strings.NewReader(body))
	cookieRequest.RemoteAddr = "127.0.0.1:34567"
	cookieRequest.Header.Set("Authorization", "Bearer "+secret)
	cookieRequest.Header.Set("Idempotency-Key", "automation-trigger-cookie-0001")
	cookieRequest.Header.Set("Content-Type", "application/json")
	cookieRequest.Header.Set("Cookie", "session=ambient")
	cookieResponse := httptest.NewRecorder()
	harness.mux.ServeHTTP(cookieResponse, cookieRequest)
	assertAPIError(t, cookieResponse, http.StatusBadRequest, errorInvalidRequest, nil)
	originRequest := httptest.NewRequest(http.MethodPost, automationcontract.TriggerPath, strings.NewReader(body))
	originRequest.RemoteAddr = "127.0.0.1:34567"
	originRequest.Header.Set("Authorization", "Bearer "+secret)
	originRequest.Header.Set("Idempotency-Key", "automation-trigger-origin-0001")
	originRequest.Header.Set("Content-Type", "application/json")
	originRequest.Header.Set("Origin", "https://attacker.example")
	originResponse := httptest.NewRecorder()
	harness.mux.ServeHTTP(originResponse, originRequest)
	assertAPIError(t, originResponse, http.StatusBadRequest, errorInvalidRequest, nil)
	if originResponse.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("unexpected CORS header=%q", originResponse.Header().Get("Access-Control-Allow-Origin"))
	}
	fake.err = store.ErrAutomationDisabled
	disabled := automationAPIRequest(harness.mux, http.MethodPost,
		automationcontract.TriggerPath, body, secret, "automation-trigger-disabled-0001")
	if disabled.Code != http.StatusNotFound || strings.Contains(disabled.Body.String(), errorAutomationDisabled) {
		t.Fatalf("disabled route status=%d body=%q", disabled.Code, disabled.Body.String())
	}

	fake.err = store.ErrAutomationInvalidCredential
	for _, candidate := range []string{strings.Repeat("b", 64), strings.Repeat("c", 64)} {
		response := automationAPIRequest(harness.mux, http.MethodPost,
			automationcontract.TriggerPath, body, candidate, "automation-trigger-credential-0001")
		assertAPIError(t, response, http.StatusUnauthorized, errorAutomationCredential, nil)
	}
}

func TestAutomationControlHTTPForeignCueAndStaleBearerFailClosed(t *testing.T) {
	harness := newOnboardingHarness(t)
	owner := createViaAPI(t, harness)
	foreign, err := harness.store.CreateSelfServiceOrbit("Foreign automation control")
	if err != nil {
		t.Fatal(err)
	}
	acceptContentPolicyHTTP(t, harness, foreign.ControlToken, "en")
	now := time.Now().UTC()
	harness.api.automationNow = func() time.Time { return now }
	cueBody := `{"title":"Owner cue","source":{"kind":"builtin","asset_id":"pulsar.recording-cue.v1","sha256":"479b1a9d605ac12454e3449e129991b7ce8599251506ca54a93be0b6144730fd"}}`
	created := automationAPIRequest(harness.mux, http.MethodPost, "/v1/soundboard/cues",
		cueBody, owner["control_token"].(string), "automation-isolation-create-0001")
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%q", created.Code, created.Body.String())
	}
	cueID := automationResponseObject(t, created)["cue"].(map[string]any)["cue_id"].(string)
	foreignProbe := automationAPIRequest(harness.mux, http.MethodPatch,
		"/v1/soundboard/cues/"+cueID, `{"title":"Probe","expected_revision":1}`,
		foreign.ControlToken, "automation-isolation-probe-0001")
	unknownProbe := automationAPIRequest(harness.mux, http.MethodPatch,
		"/v1/soundboard/cues/cq_01J00000000000000000000000",
		`{"title":"Probe","expected_revision":1}`, foreign.ControlToken,
		"automation-isolation-unknown-0001")
	assertAPIError(t, foreignProbe, http.StatusNotFound, errorAutomationCueNotFound, nil)
	assertAPIError(t, unknownProbe, http.StatusNotFound, errorAutomationCueNotFound, nil)

	entered := make(chan struct{})
	release := make(chan struct{})
	harness.api.testAfterAuth = func(store.ActorContext) {
		close(entered)
		<-release
	}
	response := make(chan *httptest.ResponseRecorder, 1)
	go func() {
		response <- automationAPIRequest(harness.mux, http.MethodPut,
			"/v1/automation/status",
			`{"soundboard_enabled":true,"automation_enabled":false,"emergency_disabled":false,"timezone":"UTC","quiet_hours":[],"expected_revision":0}`,
			owner["control_token"].(string), "automation-stale-control-0001")
	}()
	<-entered
	replacement := runtimeHTTPTestToken(t)
	if _, err := harness.store.ConsumeRecovery(owner["recovery_id"].(string),
		owner["recovery_secret"].(string), replacement); err != nil {
		t.Fatal(err)
	}
	close(release)
	assertAPIError(t, <-response, http.StatusUnauthorized, errorUnauthorized, nil)
}
