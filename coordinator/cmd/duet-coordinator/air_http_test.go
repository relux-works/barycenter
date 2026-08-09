package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/store"
)

func airAPIRequest(handler http.Handler, method, path, body, bearer, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:34567"
	req.Header.Set("Authorization", "Bearer "+bearer)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func decodeAirProjection(t *testing.T, recorder *httptest.ResponseRecorder) store.AirProjection {
	t.Helper()
	var projection store.AirProjection
	if err := json.Unmarshal(recorder.Body.Bytes(), &projection); err != nil {
		t.Fatalf("decode Air projection status=%d body=%q: %v", recorder.Code, recorder.Body.String(), err)
	}
	return projection
}

func TestAirHTTPFrozenLifecycleRoutesAndStableErrors(t *testing.T) {
	harness := newOnboardingHarness(t)
	owner, err := harness.store.CreateSelfServiceOrbit("HTTP owner")
	if err != nil {
		t.Fatal(err)
	}
	peer, err := harness.store.CreateSelfServiceOrbit("HTTP peer")
	if err != nil {
		t.Fatal(err)
	}
	outsider, err := harness.store.CreateSelfServiceOrbit("HTTP outsider")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := harness.store.CutoverLinksToAirs(1, 100); err != nil {
		t.Fatal(err)
	}
	unauthenticated := airAPIRequest(harness.mux, http.MethodGet, "/v1/airs", "", "", "")
	if unauthenticated.Code != http.StatusUnauthorized ||
		decodeObject(t, unauthenticated)["error"].(map[string]any)["code"] != errorUnauthenticated {
		t.Fatalf("unauthenticated status=%d body=%q", unauthenticated.Code, unauthenticated.Body.String())
	}
	now := time.UnixMilli(1_800_000_000_000)
	harness.api.airNow = func() time.Time { return now }
	runtimeSignals := 0
	harness.api.airRuntimeChanged = func() error { runtimeSignals++; return nil }

	createdResponse := airAPIRequest(harness.mux, http.MethodPost, "/v1/airs",
		`{"title":"HTTP Air"}`, owner.ControlToken, "http-air-create-0001")
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%q", createdResponse.Code, createdResponse.Body.String())
	}
	created := decodeAirProjection(t, createdResponse)
	if !airIDPattern.MatchString(created.AirID) || created.AirRole != "owner" {
		t.Fatalf("created=%+v", created)
	}

	replay := airAPIRequest(harness.mux, http.MethodPost, "/v1/airs",
		`{"title":"HTTP Air"}`, owner.ControlToken, "http-air-create-0001")
	if replay.Code != http.StatusCreated || decodeAirProjection(t, replay).AirID != created.AirID {
		t.Fatalf("create replay status=%d body=%q", replay.Code, replay.Body.String())
	}
	conflict := airAPIRequest(harness.mux, http.MethodPost, "/v1/airs",
		`{"title":"Different"}`, owner.ControlToken, "http-air-create-0001")
	if conflict.Code != http.StatusConflict || decodeObject(t, conflict)["error"].(map[string]any)["code"] != errorAirIdempotency {
		t.Fatalf("idempotency conflict status=%d body=%q", conflict.Code, conflict.Body.String())
	}

	foreign := airAPIRequest(harness.mux, http.MethodGet, "/v1/airs/"+created.AirID,
		"", outsider.ControlToken, "")
	if foreign.Code != http.StatusNotFound || decodeObject(t, foreign)["error"].(map[string]any)["code"] != errorAirNotFound {
		t.Fatalf("foreign read status=%d body=%q", foreign.Code, foreign.Body.String())
	}

	issueResponse := airAPIRequest(harness.mux, http.MethodPost, "/v1/airs/"+created.AirID+"/invites",
		`{"air_role":"member"}`, owner.ControlToken, "http-air-issue-0001")
	if issueResponse.Code != http.StatusCreated {
		t.Fatalf("issue status=%d body=%q", issueResponse.Code, issueResponse.Body.String())
	}
	var issued airInviteHTTPResponse
	if err := json.Unmarshal(issueResponse.Body.Bytes(), &issued); err != nil || len(issued.Code) != 43 {
		t.Fatalf("issued=%+v err=%v", issued, err)
	}

	consumeResponse := airAPIRequest(harness.mux, http.MethodPost, "/v1/air-invites/consume",
		`{"code":"`+issued.Code+`"}`, peer.ControlToken, "http-air-consume-0001")
	if consumeResponse.Code != http.StatusAccepted {
		t.Fatalf("consume status=%d body=%q", consumeResponse.Code, consumeResponse.Body.String())
	}
	var preview store.AirJoinPreview
	if err := json.Unmarshal(consumeResponse.Body.Bytes(), &preview); err != nil || preview.MembershipRevision != 1 {
		t.Fatalf("preview=%+v err=%v", preview, err)
	}

	confirmResponse := airAPIRequest(harness.mux, http.MethodPost,
		"/v1/airs/"+created.AirID+"/join/confirm",
		`{"membership_revision":1,"activate":true,"expected_active_air_id":"none"}`,
		peer.ControlToken, "http-air-confirm-0001")
	if confirmResponse.Code != http.StatusOK || !decodeAirProjection(t, confirmResponse).IsCurrent {
		t.Fatalf("confirm status=%d body=%q", confirmResponse.Code, confirmResponse.Body.String())
	}
	activateResponse := airAPIRequest(harness.mux, http.MethodPost,
		"/v1/airs/"+created.AirID+"/activate",
		`{"membership_revision":1,"expected_active_air_id":"none"}`,
		owner.ControlToken, "http-air-activate-0001")
	if activateResponse.Code != http.StatusOK || decodeAirProjection(t, activateResponse).Status != "active" {
		t.Fatalf("activate status=%d body=%q", activateResponse.Code, activateResponse.Body.String())
	}
	if runtimeSignals != 2 {
		t.Fatalf("runtime signals=%d want=2", runtimeSignals)
	}
	unauthorizedPolicy := airAPIRequest(harness.mux, http.MethodPut,
		"/v1/airs/"+created.AirID+"/policy",
		`{"policy_revision":1,"invite":"air_admin_primary","overlay":"primary_companion","queue":"primary_companion","replace":"air_admin_primary"}`,
		peer.ControlToken, "http-air-policy-peer-0001")
	if unauthorizedPolicy.Code != http.StatusForbidden ||
		decodeObject(t, unauthorizedPolicy)["error"].(map[string]any)["code"] != errorForbidden {
		t.Fatalf("unauthorized policy status=%d body=%q", unauthorizedPolicy.Code, unauthorizedPolicy.Body.String())
	}

	unauthorizedDissolve := airAPIRequest(harness.mux, http.MethodPost,
		"/v1/airs/"+created.AirID+"/dissolve", `{"air_revision":4}`,
		peer.ControlToken, "http-air-dissolve-peer-0001")
	if unauthorizedDissolve.Code != http.StatusForbidden ||
		decodeObject(t, unauthorizedDissolve)["error"].(map[string]any)["code"] != errorForbidden {
		t.Fatalf("unauthorized dissolve status=%d body=%q", unauthorizedDissolve.Code, unauthorizedDissolve.Body.String())
	}

	listResponse := airAPIRequest(harness.mux, http.MethodGet, "/v1/airs", "", peer.ControlToken, "")
	if listResponse.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%q", listResponse.Code, listResponse.Body.String())
	}
	var list store.AirListView
	if err := json.Unmarshal(listResponse.Body.Bytes(), &list); err != nil || list.CurrentAirID != created.AirID || len(list.Saved) != 1 {
		t.Fatalf("list=%+v err=%v", list, err)
	}
}

func TestAirHTTPReportsDisabledRolloutInsteadOfRevisionConflict(t *testing.T) {
	harness := newOnboardingHarness(t)
	owner, err := harness.store.CreateSelfServiceOrbit("Shadow owner")
	if err != nil {
		t.Fatal(err)
	}
	response := airAPIRequest(
		harness.mux, http.MethodPost, "/v1/airs",
		`{"title":"Unavailable Air"}`, owner.ControlToken, "http-air-shadow-0001")
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("shadow create status=%d body=%q", response.Code, response.Body.String())
	}
	if code := decodeObject(t, response)["error"].(map[string]any)["code"]; code != errorAirRoomsDisabled {
		t.Fatalf("shadow create code=%v", code)
	}
}

func TestAirHTTPRejectsLooseShapesAndRateLimitsUnavailableInvites(t *testing.T) {
	harness := newOnboardingHarness(t)
	actor, err := harness.store.CreateSelfServiceOrbit("Rate limited actor")
	if err != nil {
		t.Fatal(err)
	}
	owner, err := harness.store.CreateSelfServiceOrbit("Invite owner")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := harness.store.CutoverLinksToAirs(1, 100); err != nil {
		t.Fatal(err)
	}
	createdResponse := airAPIRequest(harness.mux, http.MethodPost, "/v1/airs",
		`{"title":"Rate limit Air"}`, owner.ControlToken, "http-air-rate-create-0001")
	if createdResponse.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%q", createdResponse.Code, createdResponse.Body.String())
	}
	created := decodeAirProjection(t, createdResponse)
	issueResponse := airAPIRequest(harness.mux, http.MethodPost,
		"/v1/airs/"+created.AirID+"/invites", `{"air_role":"member"}`,
		owner.ControlToken, "http-air-rate-issue-0001")
	if issueResponse.Code != http.StatusCreated {
		t.Fatalf("issue status=%d body=%q", issueResponse.Code, issueResponse.Body.String())
	}
	var issued airInviteHTTPResponse
	if err := json.Unmarshal(issueResponse.Body.Bytes(), &issued); err != nil || len(issued.Code) != 43 {
		t.Fatalf("issued=%+v err=%v", issued, err)
	}
	bad := airAPIRequest(harness.mux, http.MethodPost, "/v1/airs?loose=1",
		`{"title":"No"}`, actor.ControlToken, "http-air-invalid-0001")
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("query status=%d body=%q", bad.Code, bad.Body.String())
	}
	unknown := `{"code":"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"}`
	for i := 0; i < 5; i++ {
		response := airAPIRequest(harness.mux, http.MethodPost, "/v1/air-invites/consume",
			unknown, actor.ControlToken, "http-air-missing-000"+string(rune('1'+i)))
		if response.Code != http.StatusNotFound {
			t.Fatalf("unavailable attempt=%d status=%d body=%q", i, response.Code, response.Body.String())
		}
	}
	valid := `{"code":"` + issued.Code + `"}`
	limited := airAPIRequest(harness.mux, http.MethodPost, "/v1/air-invites/consume",
		valid, actor.ControlToken, "http-air-valid-0006")
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("limited status=%d body=%q", limited.Code, limited.Body.String())
	}
	// The admission check must happen before the store mutation: exhausting
	// the failure budget cannot be bypassed by making the next guess valid.
	harness.api.airInviteConsumeActor = newAttemptLimiter(5, time.Minute, 10_000)
	harness.api.airInviteConsumeIP = newAttemptLimiter(5, time.Minute, 10_000)
	accepted := airAPIRequest(harness.mux, http.MethodPost, "/v1/air-invites/consume",
		valid, actor.ControlToken, "http-air-valid-0006")
	if accepted.Code != http.StatusAccepted {
		t.Fatalf("valid invite was consumed before limiter admission status=%d body=%q",
			accepted.Code, accepted.Body.String())
	}
}
