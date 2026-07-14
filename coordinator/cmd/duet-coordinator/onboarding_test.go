package main

import (
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/config"
	"relux.works/duet/coordinator/internal/store"
)

type onboardingHarness struct {
	store *store.Store
	api   *onboardingAPI
	mux   *http.ServeMux
	logs  *bytes.Buffer
	path  string
}

func newOnboardingHarness(t *testing.T) onboardingHarness {
	t.Helper()
	path := filepath.Join(t.TempDir(), "api.db")
	st, err := store.OpenWithOptions(path, store.Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	logs := new(bytes.Buffer)
	log := slog.New(slog.NewJSONHandler(logs, nil))
	cfg := &config.Config{SelfServiceOnboarding: true, MediaDir: t.TempDir()}
	api := newOnboardingAPI(st, cfg, log, "@barycenter_bot")
	// Upload-session tests predating SubmitMedia assert the durable finalizing
	// boundary with arbitrary bytes. SubmitMedia tests install an explicit
	// fake or real processor instead.
	api.mediaSubmitter = nil
	api.mediaSubmitterInitErr = nil
	mux := http.NewServeMux()
	api.register(mux)
	return onboardingHarness{store: st, api: api, mux: mux, logs: logs, path: path}
}

func runtimeHTTPTestToken(t *testing.T) string {
	t.Helper()
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(raw)
}

func runtimeHTTPTestHumanCode(t *testing.T) string {
	t.Helper()
	const alphabet = "ABCDEFGHJKMNPQRSTVWXYZ23456789"
	result := make([]byte, 0, 27)
	raw := make([]byte, 32)
	for len(result) < 27 {
		if _, err := rand.Read(raw); err != nil {
			t.Fatal(err)
		}
		for _, value := range raw {
			if value >= 240 {
				continue
			}
			result = append(result, alphabet[int(value)/8])
			if len(result) == 27 {
				break
			}
		}
	}
	return string(result)
}

func apiRequest(handler http.Handler, method, path, body, bearer string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:34567"
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func decodeObject(t *testing.T, recorder *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(recorder.Body.Bytes(), &value); err != nil {
		t.Fatalf("decode response status=%d bytes=%d: %v", recorder.Code, recorder.Body.Len(), err)
	}
	return value
}

func createViaAPI(t *testing.T, harness onboardingHarness) map[string]any {
	t.Helper()
	recorder := apiRequest(harness.mux, http.MethodPost, "/v1/onboarding/orbits",
		`{"title":"Home","installation_attempt_id":"install_attempt_0001"}`, "")
	if recorder.Code != http.StatusCreated {
		t.Fatalf("create status=%d response_bytes=%d", recorder.Code, recorder.Body.Len())
	}
	return decodeObject(t, recorder)
}

func TestOnboardingHTTPCreateContextAndSecretRedaction(t *testing.T) {
	harness := newOnboardingHarness(t)
	created := createViaAPI(t, harness)
	for _, field := range []string{"orbit_id", "title", "actor_id", "role", "slot", "node_token", "control_token", "recovery_id", "recovery_secret", "shown_once"} {
		if _, exists := created[field]; !exists {
			t.Fatalf("create response missing %q", field)
		}
	}
	if created["node_token"] == created["control_token"] || created["shown_once"] != true || created["role"] != "primary" {
		t.Fatal("create response violated credential separation or non-secret metadata")
	}
	node := created["node_token"].(string)
	control := created["control_token"].(string)
	recovery := created["recovery_secret"].(string)
	for name, token := range map[string]string{"node": node, "control": control} {
		recorder := apiRequest(harness.mux, http.MethodGet, "/v1/actor/context", "", token)
		if recorder.Code != http.StatusOK || recorder.Header().Get("Cache-Control") != "no-store" {
			t.Fatalf("%s context status=%d cache_control=%q", name, recorder.Code, recorder.Header().Get("Cache-Control"))
		}
		context := decodeObject(t, recorder)
		if context["actor_id"] != created["actor_id"] || context["role"] != "primary" {
			t.Fatalf("%s context=%v", name, context)
		}
	}
	if strings.Contains(harness.logs.String(), node) || strings.Contains(harness.logs.String(), control) || strings.Contains(harness.logs.String(), recovery) {
		t.Fatalf("plaintext secret entered logs (log_bytes=%d)", harness.logs.Len())
	}
}

func TestConcurrentInstallationAttemptCreatesOneOrbit(t *testing.T) {
	harness := newOnboardingHarness(t)
	const body = `{"title":"Single winner","installation_attempt_id":"shared_attempt_0001"}`
	start := make(chan struct{})
	results := make(chan *httptest.ResponseRecorder, 2)
	var ready sync.WaitGroup
	ready.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			ready.Done()
			<-start
			results <- apiRequest(harness.mux, http.MethodPost, "/v1/onboarding/orbits", body, "")
		}()
	}
	ready.Wait()
	close(start)
	created, limited := 0, 0
	for i := 0; i < 2; i++ {
		recorder := <-results
		switch recorder.Code {
		case http.StatusCreated:
			created++
		case http.StatusTooManyRequests:
			limited++
		default:
			t.Fatalf("concurrent create status=%d response_bytes=%d", recorder.Code, recorder.Body.Len())
		}
	}
	if created != 1 || limited != 1 {
		t.Fatalf("create winners=%d rate_limited=%d", created, limited)
	}
	orbits, err := harness.store.OrbitIDs()
	if err != nil || len(orbits) != 1 {
		t.Fatalf("persisted orbit count=%d err=%v", len(orbits), err)
	}
}

func TestCapabilityMiddlewareRejectsNodeAcrossAdministrationSurfaces(t *testing.T) {
	harness := newOnboardingHarness(t)
	created := createViaAPI(t, harness)
	node := created["node_token"].(string)
	for _, request := range []struct {
		path string
		body string
	}{
		{"/v1/device-invites", `{}`},
		{"/v1/recovery/rotate", `{}`},
		{"/v1/telegram-links", `{}`},
	} {
		recorder := apiRequest(harness.mux, http.MethodPost, request.path, request.body, node)
		assertAPIError(t, recorder, http.StatusForbidden, errorInsufficientCapability, nil)
	}

	// Upload administration is outside this task's route set, but it consumes
	// the same production capability middleware. Prove that an attached admin
	// surface cannot accidentally accept playback-only node authority.
	uploadCalled := false
	uploadAdmin := harness.api.secure(harness.api.withControl(func(w http.ResponseWriter, _ *http.Request) {
		uploadCalled = true
		writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
	}))
	recorder := apiRequest(http.HandlerFunc(uploadAdmin), http.MethodPost, "/test-upload-admin", `{}`, node)
	assertAPIError(t, recorder, http.StatusForbidden, errorInsufficientCapability, nil)
	if uploadCalled {
		t.Fatal("node token reached upload administration handler")
	}

	recorder = apiRequest(harness.mux, http.MethodPost, "/v1/onboarding/orbits",
		`{"title":"Escalation","installation_attempt_id":"install_attempt_0002"}`, node)
	assertAPIError(t, recorder, http.StatusForbidden, errorInsufficientCapability, nil)
}

func TestCapabilityRoleMatrixForOnboardingMutations(t *testing.T) {
	harness := newOnboardingHarness(t)
	primary, err := harness.store.CreateSelfServiceOrbit("Matrix")
	if err != nil {
		t.Fatal(err)
	}
	join := func(role string) store.OnboardingCredentials {
		t.Helper()
		invite, err := harness.store.IssueDeviceInvite(primary.ActorID, primary.ControlToken, role)
		if err != nil {
			t.Fatal(err)
		}
		joined, err := harness.store.ConsumeDeviceInvite(invite.Code)
		if err != nil {
			t.Fatal(err)
		}
		return joined
	}
	companion := join("companion")
	satellite := join("satellite")
	tests := []struct {
		name    string
		control string
		node    string
		allowed bool
	}{
		{"primary", primary.ControlToken, primary.NodeToken, true},
		{"companion", companion.ControlToken, companion.NodeToken, true},
		{"satellite", satellite.ControlToken, satellite.NodeToken, false},
	}
	endpoints := []struct {
		path    string
		body    string
		success int
	}{
		{"/v1/device-invites", `{"intended_role":"satellite"}`, http.StatusCreated},
		{"/v1/recovery/rotate", `{}`, http.StatusOK},
		{"/v1/telegram-links", `{"desired_role":"satellite"}`, http.StatusCreated},
	}
	for _, identity := range tests {
		for _, endpoint := range endpoints {
			t.Run(identity.name+endpoint.path, func(t *testing.T) {
				recorder := apiRequest(harness.mux, http.MethodPost, endpoint.path, endpoint.body, identity.control)
				if identity.allowed {
					if recorder.Code != endpoint.success {
						t.Fatalf("control role=%s path=%s status=%d response_bytes=%d", identity.name, endpoint.path, recorder.Code, recorder.Body.Len())
					}
				} else {
					assertAPIError(t, recorder, http.StatusForbidden, errorInsufficientCapability, nil)
				}
				recorder = apiRequest(harness.mux, http.MethodPost, endpoint.path, endpoint.body, identity.node)
				assertAPIError(t, recorder, http.StatusForbidden, errorInsufficientCapability, nil)
			})
		}
		for _, token := range []string{identity.node, identity.control} {
			recorder := apiRequest(harness.mux, http.MethodGet, "/v1/actor/context", "", token)
			if recorder.Code != http.StatusOK {
				t.Fatalf("context role=%s status=%d response_bytes=%d", identity.name, recorder.Code, recorder.Body.Len())
			}
		}
	}
}

func TestActorContextAndMutationLifecycleErrors(t *testing.T) {
	tests := []struct {
		name       string
		mutation   string
		args       func(store.OnboardingCredentials) []any
		wantStatus int
		wantCode   string
	}{
		{"revoked_actor", `UPDATE actors SET revoked_at = 1 WHERE id = ?`, func(value store.OnboardingCredentials) []any { return []any{value.ActorID} }, http.StatusUnauthorized, errorUnauthorized},
		{"revoked_slot", `UPDATE slots SET revoked_at = 1 WHERE orbit_id = ? AND slot = ?`, func(value store.OnboardingCredentials) []any { return []any{value.OrbitID, value.Slot} }, http.StatusUnauthorized, errorUnauthorized},
		{"stale_binding", `UPDATE slots SET token_hash = ? WHERE orbit_id = ? AND slot = ?`, func(value store.OnboardingCredentials) []any {
			return []any{strings.Repeat("a", 64), value.OrbitID, value.Slot}
		}, http.StatusUnauthorized, errorUnauthorized},
		{"left_membership", `UPDATE memberships SET left_at = 1 WHERE actor_id = ?`, func(value store.OnboardingCredentials) []any { return []any{value.ActorID} }, http.StatusForbidden, errorInsufficientCapability},
		{"disabled_orbit", `UPDATE orbits SET status = 'disabled' WHERE id = ?`, func(value store.OnboardingCredentials) []any { return []any{value.OrbitID} }, http.StatusForbidden, errorInsufficientCapability},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			harness := newOnboardingHarness(t)
			created, err := harness.store.CreateSelfServiceOrbit("Lifecycle")
			if err != nil {
				t.Fatal(err)
			}
			inspect, err := sql.Open("sqlite", harness.path)
			if err != nil {
				t.Fatal(err)
			}
			defer inspect.Close()
			if _, err := inspect.Exec(test.mutation, test.args(created)...); err != nil {
				t.Fatal(err)
			}
			assertAPIError(t, apiRequest(harness.mux, http.MethodGet, "/v1/actor/context", "", created.ControlToken),
				test.wantStatus, test.wantCode, nil)
			if test.wantStatus == http.StatusForbidden {
				assertAPIError(t, apiRequest(harness.mux, http.MethodPost, "/v1/recovery/rotate", `{"unexpected":true}`, created.ControlToken),
					http.StatusBadRequest, errorInvalidRequest, nil)
				if count := limiterAttemptCount(harness.api.rotateActor, fmt.Sprint(created.ActorID)); count != 0 {
					t.Fatalf("malformed lifecycle request reserved attempts=%d", count)
				}
			}
			assertAPIError(t, apiRequest(harness.mux, http.MethodPost, "/v1/recovery/rotate", `{}`, created.ControlToken),
				test.wantStatus, test.wantCode, nil)
			if test.wantStatus == http.StatusForbidden {
				if count := limiterAttemptCount(harness.api.rotateActor, fmt.Sprint(created.ActorID)); count != 1 {
					t.Fatalf("valid lifecycle request reserved attempts=%d", count)
				}
			}
		})
	}
}

func limiterAttemptCount(limiter *attemptLimiter, key string) int {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()
	return len(limiter.entries[key].timestamps)
}

func TestAuthenticatedMutationRechecksBearerAndRoleInsideTransaction(t *testing.T) {
	t.Run("stale_control_after_middleware", func(t *testing.T) {
		harness := newOnboardingHarness(t)
		created, err := harness.store.CreateSelfServiceOrbit("Stale bearer")
		if err != nil {
			t.Fatal(err)
		}
		entered := make(chan struct{})
		release := make(chan struct{})
		harness.api.testAfterAuth = func(store.ActorContext) {
			close(entered)
			<-release
		}
		response := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			response <- apiRequest(harness.mux, http.MethodPost, "/v1/recovery/rotate", `{}`, created.ControlToken)
		}()
		<-entered
		replacement := runtimeHTTPTestToken(t)
		if _, err := harness.store.ConsumeRecovery(created.RecoveryID, created.RecoverySecret, replacement); err != nil {
			t.Fatal(err)
		}
		close(release)
		assertAPIError(t, <-response, http.StatusUnauthorized, errorUnauthorized, nil)
		if recorder := apiRequest(harness.mux, http.MethodGet, "/v1/actor/context", "", replacement); recorder.Code != http.StatusOK {
			t.Fatalf("replacement control probe status=%d response_bytes=%d", recorder.Code, recorder.Body.Len())
		}
	})
	t.Run("role_changed_after_middleware", func(t *testing.T) {
		harness := newOnboardingHarness(t)
		created, err := harness.store.CreateSelfServiceOrbit("Stale role")
		if err != nil {
			t.Fatal(err)
		}
		entered := make(chan struct{})
		release := make(chan struct{})
		harness.api.testAfterAuth = func(store.ActorContext) {
			close(entered)
			<-release
		}
		response := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			response <- apiRequest(harness.mux, http.MethodPost, "/v1/telegram-links", `{}`, created.ControlToken)
		}()
		<-entered
		inspect, err := sql.Open("sqlite", harness.path)
		if err != nil {
			t.Fatal(err)
		}
		defer inspect.Close()
		if _, err := inspect.Exec(`UPDATE memberships SET role = 'satellite' WHERE actor_id = ?`, created.ActorID); err != nil {
			t.Fatal(err)
		}
		close(release)
		assertAPIError(t, <-response, http.StatusForbidden, errorInsufficientCapability, nil)
	})
}

func TestControlMutationOrderingForSatelliteAndNode(t *testing.T) {
	t.Run("satellite", func(t *testing.T) {
		harness := newOnboardingHarness(t)
		created, err := harness.store.CreateSelfServiceOrbit("Satellite ordering")
		if err != nil {
			t.Fatal(err)
		}
		inspect, err := sql.Open("sqlite", harness.path)
		if err != nil {
			t.Fatal(err)
		}
		defer inspect.Close()
		if _, err := inspect.Exec(`UPDATE memberships SET role = 'satellite' WHERE actor_id = ?`, created.ActorID); err != nil {
			t.Fatal(err)
		}
		key := fmt.Sprint(created.ActorID)
		assertAPIError(t, apiRequest(harness.mux, http.MethodPost, "/v1/recovery/rotate", `{"bad":true}`, created.ControlToken),
			http.StatusBadRequest, errorInvalidRequest, nil)
		if count := limiterAttemptCount(harness.api.rotateActor, key); count != 0 {
			t.Fatalf("malformed satellite request reserved attempts=%d", count)
		}
		assertAPIError(t, apiRequest(harness.mux, http.MethodPost, "/v1/recovery/rotate", `{}`, created.ControlToken),
			http.StatusForbidden, errorInsufficientCapability, nil)
		if count := limiterAttemptCount(harness.api.rotateActor, key); count != 1 {
			t.Fatalf("valid satellite request reserved attempts=%d", count)
		}
	})
	t.Run("node", func(t *testing.T) {
		harness := newOnboardingHarness(t)
		created, err := harness.store.CreateSelfServiceOrbit("Node ordering")
		if err != nil {
			t.Fatal(err)
		}
		assertAPIError(t, apiRequest(harness.mux, http.MethodPost, "/v1/recovery/rotate", `{}`, created.NodeToken),
			http.StatusForbidden, errorInsufficientCapability, nil)
		if count := limiterAttemptCount(harness.api.rotateActor, fmt.Sprint(created.ActorID)); count != 0 {
			t.Fatalf("node request reached control limiter attempts=%d", count)
		}
	})
}

func TestDeviceInviteHTTPJoinAndUniformReplay(t *testing.T) {
	harness := newOnboardingHarness(t)
	created := createViaAPI(t, harness)
	control := created["control_token"].(string)
	recorder := apiRequest(harness.mux, http.MethodPost, "/v1/device-invites", `{"intended_role":"satellite"}`, control)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("issue status=%d response_bytes=%d", recorder.Code, recorder.Body.Len())
	}
	issued := decodeObject(t, recorder)
	code := issued["invite_code"].(string)
	if issued["intended_role"] != "satellite" || issued["expires_at"] == "" {
		t.Fatal("invite issue returned incorrect non-secret metadata")
	}
	recorder = apiRequest(harness.mux, http.MethodPost, "/v1/device-invites/consume",
		fmt.Sprintf(`{"invite_code":%q}`, code), "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("consume status=%d response_bytes=%d", recorder.Code, recorder.Body.Len())
	}
	joined := decodeObject(t, recorder)
	if joined["role"] != "satellite" || joined["node_token"] == joined["control_token"] {
		t.Fatal("join response violated role or credential separation")
	}
	if _, exists := joined["recovery_secret"]; exists {
		t.Fatal("join response returned recovery secret outside create/rotate")
	}
	recorder = apiRequest(harness.mux, http.MethodPost, "/v1/device-invites/consume",
		fmt.Sprintf(`{"invite_code":%q}`, code), "")
	assertAPIError(t, recorder, http.StatusForbidden, errorCredentialInvalid, nil)
	recorder = apiRequest(harness.mux, http.MethodPost, "/v1/device-invites/consume", `{"invite_code":"bad"}`, "")
	assertAPIError(t, recorder, http.StatusBadRequest, errorInvalidRequest, nil)
}

func TestRecoveryHTTPExactContractAndRotation(t *testing.T) {
	harness := newOnboardingHarness(t)
	created := createViaAPI(t, harness)
	oldControl := created["control_token"].(string)
	node := created["node_token"].(string)
	replacement := runtimeHTTPTestToken(t)
	body := fmt.Sprintf(`{"recovery_id":%q,"recovery_secret":%q,"replacement_control_token":%q}`,
		created["recovery_id"], created["recovery_secret"], replacement)
	recorder := apiRequest(harness.mux, http.MethodPost, "/v1/recovery/consume", body, "")
	if recorder.Code != http.StatusOK {
		t.Fatalf("recover status=%d response_bytes=%d", recorder.Code, recorder.Body.Len())
	}
	result := decodeObject(t, recorder)
	if len(result) != 3 || result["actor_id"] != created["actor_id"] || result["orbit_id"] != created["orbit_id"] || result["role"] != "primary" {
		t.Fatal("recovery response violated the flat non-secret context contract")
	}
	assertAPIError(t, apiRequest(harness.mux, http.MethodGet, "/v1/actor/context", "", oldControl),
		http.StatusUnauthorized, errorUnauthorized, nil)
	if recorder := apiRequest(harness.mux, http.MethodGet, "/v1/actor/context", "", node); recorder.Code != http.StatusOK {
		t.Fatalf("node credential changed during recovery: status=%d response_bytes=%d", recorder.Code, recorder.Body.Len())
	}

	recorder = apiRequest(harness.mux, http.MethodPost, "/v1/recovery/rotate", `{}`, replacement)
	if recorder.Code != http.StatusOK {
		t.Fatalf("rotate status=%d response_bytes=%d", recorder.Code, recorder.Body.Len())
	}
	rotated := decodeObject(t, recorder)
	if len(rotated) != 4 || rotated["actor_id"] != created["actor_id"] || rotated["shown_once"] != true ||
		rotated["recovery_id"] == created["recovery_id"] {
		t.Fatal("rotation response violated actor, generation, or shown-once contract")
	}
	if strings.Contains(harness.logs.String(), replacement) || strings.Contains(harness.logs.String(), created["recovery_secret"].(string)) ||
		strings.Contains(harness.logs.String(), rotated["recovery_secret"].(string)) {
		t.Fatalf("recovery plaintext entered logs (log_bytes=%d)", harness.logs.Len())
	}
}

func TestTelegramLinkHTTPContractHasNoSecretURL(t *testing.T) {
	harness := newOnboardingHarness(t)
	created := createViaAPI(t, harness)
	recorder := apiRequest(harness.mux, http.MethodPost, "/v1/telegram-links", `{"desired_role":"companion"}`,
		created["control_token"].(string))
	if recorder.Code != http.StatusCreated {
		t.Fatalf("link status=%d response_bytes=%d", recorder.Code, recorder.Body.Len())
	}
	response := decodeObject(t, recorder)
	if len(response) != 4 || response["desired_role"] != "companion" || response["bot_username"] != "barycenter_bot" || response["expires_at"] == "" {
		t.Fatal("link response violated the exact non-secret metadata contract")
	}
	code := response["link_code"].(string)
	if strings.Contains(recorder.Body.String(), "http") || strings.Contains(recorder.Body.String(), "?start=") || strings.Contains(harness.logs.String(), code) {
		t.Fatalf("link code leaked through URL/log (response_bytes=%d log_bytes=%d)", recorder.Body.Len(), harness.logs.Len())
	}
	recorder = apiRequest(harness.mux, http.MethodPost, "/v1/telegram-links", `{"desired_role":"primary"}`,
		created["control_token"].(string))
	assertAPIError(t, recorder, http.StatusBadRequest, errorInvalidRequest, nil)
}

func TestRecoveryLimiterOrderingBoundaryAndExactEnvelope(t *testing.T) {
	harness := newOnboardingHarness(t)
	fakeID := "rec_" + strings.Repeat("7", 32)
	secret := runtimeHTTPTestHumanCode(t)
	replacement := runtimeHTTPTestToken(t)
	validBody := fmt.Sprintf(`{"recovery_id":%q,"recovery_secret":%q,"replacement_control_token":%q}`, fakeID, secret, replacement)

	for i := 0; i < 3; i++ {
		recorder := apiRequest(harness.mux, http.MethodPost, "/v1/recovery/consume", `{}`, "")
		assertAPIError(t, recorder, http.StatusBadRequest, errorInvalidRequest, nil)
	}
	for i := 1; i <= 10; i++ {
		recorder := apiRequest(harness.mux, http.MethodPost, "/v1/recovery/consume", validBody, "")
		assertAPIError(t, recorder, http.StatusForbidden, errorCredentialInvalid, nil)
	}
	recorder := apiRequest(harness.mux, http.MethodPost, "/v1/recovery/consume", validBody, "")
	assertAPIError(t, recorder, http.StatusTooManyRequests, errorTooManyAttempts, func(value *int64) bool { return value != nil && *value > 0 })
	if recorder.Header().Get("Retry-After") == "" {
		t.Fatal("429 omitted Retry-After")
	}
}

func TestRequestObjectsAndPreNormalizationBoundsRejectBeforeWork(t *testing.T) {
	harness := newOnboardingHarness(t)
	owner, err := harness.store.CreateSelfServiceOrbit("Bounds")
	if err != nil {
		t.Fatal(err)
	}
	inspect, err := sql.Open("sqlite", harness.path)
	if err != nil {
		t.Fatal(err)
	}
	defer inspect.Close()
	invite, err := harness.store.IssueDeviceInvite(owner.ActorID, owner.ControlToken, "companion")
	if err != nil {
		t.Fatal(err)
	}
	storeCalls := 0
	harness.api.testBeforeStore = func(string) { storeCalls++ }
	overlongInvite := strings.Repeat("-", 14) + invite.Code
	assertAPIError(t, apiRequest(harness.mux, http.MethodPost, "/v1/device-invites/consume",
		fmt.Sprintf(`{"invite_code":%q}`, overlongInvite), ""), http.StatusBadRequest, errorInvalidRequest, nil)
	if storeCalls != 0 || limiterAttemptCount(harness.api.inviteConsumeIP, "127.0.0.1") != 0 {
		t.Fatalf("overlong invite reached work: store_calls=%d attempts=%d", storeCalls,
			limiterAttemptCount(harness.api.inviteConsumeIP, "127.0.0.1"))
	}
	overlongRecovery := strings.Repeat("-", 14) + owner.RecoverySecret
	assertAPIError(t, apiRequest(harness.mux, http.MethodPost, "/v1/recovery/consume",
		fmt.Sprintf(`{"recovery_id":%q,"recovery_secret":%q,"replacement_control_token":%q}`,
			owner.RecoveryID, overlongRecovery, runtimeHTTPTestToken(t)), ""), http.StatusBadRequest, errorInvalidRequest, nil)
	if limiterAttemptCount(harness.api.recoveryIP, "127.0.0.1") != 0 ||
		limiterAttemptCount(harness.api.recoveryID, owner.RecoveryID) != 0 || storeCalls != 0 {
		t.Fatal("overlong recovery reserved an attempt")
	}

	requests := []struct {
		path   string
		body   string
		bearer string
	}{
		{"/v1/onboarding/orbits", "null", ""},
		{"/v1/device-invites", "null", owner.ControlToken},
		{"/v1/device-invites/consume", "null", ""},
		{"/v1/recovery/consume", "null", ""},
		{"/v1/recovery/rotate", "null", owner.ControlToken},
		{"/v1/telegram-links", "null", owner.ControlToken},
		{"/v1/recovery/rotate", `[]`, owner.ControlToken},
		{"/v1/recovery/rotate", `"scalar"`, owner.ControlToken},
		{"/v1/recovery/rotate", `{}` + ` {}`, owner.ControlToken},
		{"/v1/recovery/rotate", `{"unknown":true}`, owner.ControlToken},
	}
	for _, request := range requests {
		assertAPIError(t, apiRequest(harness.mux, http.MethodPost, request.path, request.body, request.bearer),
			http.StatusBadRequest, errorInvalidRequest, nil)
	}
	if count := limiterAttemptCount(harness.api.rotateActor, fmt.Sprint(owner.ActorID)); count != 0 {
		t.Fatalf("non-object/invalid rotation bodies reserved attempts=%d", count)
	}
	if count := limiterAttemptCount(harness.api.linkActor, fmt.Sprint(owner.ActorID)); count != 0 {
		t.Fatalf("null link body reserved attempts=%d", count)
	}
	if limiterAttemptCount(harness.api.createIP, "127.0.0.1") != 0 ||
		limiterAttemptCount(harness.api.inviteConsumeIP, "127.0.0.1") != 0 ||
		limiterAttemptCount(harness.api.recoveryIP, "127.0.0.1") != 0 {
		t.Fatal("non-object unauthenticated body touched a source limiter")
	}

	var invitesBefore, linksBefore int
	if err := inspect.QueryRow(`SELECT COUNT(*) FROM device_invites`).Scan(&invitesBefore); err != nil {
		t.Fatal(err)
	}
	if err := inspect.QueryRow(`SELECT COUNT(*) FROM telegram_link_codes`).Scan(&linksBefore); err != nil {
		t.Fatal(err)
	}
	for _, request := range []struct {
		path string
		body string
	}{
		{"/v1/device-invites", `{"intended_role":null}`},
		{"/v1/device-invites", `{"intended_role":1}`},
		{"/v1/device-invites", `{"intended_role":"companion","intended_role":null}`},
		{"/v1/device-invites", `{"intended_role":null,"intended_role":"companion"}`},
		{"/v1/telegram-links", `{"desired_role":null}`},
		{"/v1/telegram-links", `{"desired_role":1}`},
		{"/v1/telegram-links", `{"desired_role":"companion","desired_role":null}`},
		{"/v1/telegram-links", `{"desired_role":null,"desired_role":"companion"}`},
	} {
		assertAPIError(t, apiRequest(harness.mux, http.MethodPost, request.path, request.body, owner.ControlToken),
			http.StatusBadRequest, errorInvalidRequest, nil)
	}
	var invitesAfter, linksAfter int
	if err := inspect.QueryRow(`SELECT COUNT(*) FROM device_invites`).Scan(&invitesAfter); err != nil {
		t.Fatal(err)
	}
	if err := inspect.QueryRow(`SELECT COUNT(*) FROM telegram_link_codes`).Scan(&linksAfter); err != nil {
		t.Fatal(err)
	}
	if invitesAfter != invitesBefore || linksAfter != linksBefore ||
		limiterAttemptCount(harness.api.linkActor, fmt.Sprint(owner.ActorID)) != 0 {
		t.Fatalf("invalid optional role reached mutation: invites_delta=%d links_delta=%d link_attempts=%d",
			invitesAfter-invitesBefore, linksAfter-linksBefore,
			limiterAttemptCount(harness.api.linkActor, fmt.Sprint(owner.ActorID)))
	}
}

func TestMalformedOrMultipleAuthorizationStopsBeforeAuthDispatch(t *testing.T) {
	harness := newOnboardingHarness(t)
	owner, err := harness.store.CreateSelfServiceOrbit("Authorization framing")
	if err != nil {
		t.Fatal(err)
	}
	inspect, err := sql.Open("sqlite", harness.path)
	if err != nil {
		t.Fatal(err)
	}
	defer inspect.Close()
	var recoveryIDBefore string
	if err := inspect.QueryRow(`SELECT recovery_id FROM installation_credentials WHERE actor_id = ?`,
		owner.ActorID).Scan(&recoveryIDBefore); err != nil {
		t.Fatal(err)
	}
	authDispatches := 0
	harness.api.testAfterAuth = func(store.ActorContext) { authDispatches++ }
	for _, headers := range [][]string{
		{"Bearer " + owner.ControlToken, "Bearer " + owner.ControlToken},
		{"Bearer " + owner.NodeToken, "Bearer " + owner.ControlToken},
		{"Bearer " + owner.ControlToken + ", Bearer " + owner.ControlToken},
		{"bearer " + owner.ControlToken},
		{"Bearer " + strings.ToUpper(owner.ControlToken)},
		{"Bearer  " + owner.ControlToken},
	} {
		req := httptest.NewRequest(http.MethodPost, "/v1/recovery/rotate", strings.NewReader(`{}`))
		req.RemoteAddr = "127.0.0.1:34567"
		for _, header := range headers {
			req.Header.Add("Authorization", header)
		}
		recorder := httptest.NewRecorder()
		harness.mux.ServeHTTP(recorder, req)
		assertAPIError(t, recorder, http.StatusUnauthorized, errorUnauthorized, nil)
	}
	create := httptest.NewRequest(http.MethodPost, "/v1/onboarding/orbits",
		strings.NewReader(`{"title":"Rejected","installation_attempt_id":"authorization_attempt_1"}`))
	create.RemoteAddr = "127.0.0.1:34567"
	create.Header.Add("Authorization", "")
	create.Header.Add("Authorization", "Bearer "+owner.NodeToken)
	createResponse := httptest.NewRecorder()
	harness.mux.ServeHTTP(createResponse, create)
	assertAPIError(t, createResponse, http.StatusUnauthorized, errorUnauthorized, nil)
	var recoveryIDAfter string
	if err := inspect.QueryRow(`SELECT recovery_id FROM installation_credentials WHERE actor_id = ?`,
		owner.ActorID).Scan(&recoveryIDAfter); err != nil {
		t.Fatal(err)
	}
	orbits, err := harness.store.OrbitIDs()
	if err != nil {
		t.Fatal(err)
	}
	if authDispatches != 0 || limiterAttemptCount(harness.api.rotateActor, fmt.Sprint(owner.ActorID)) != 0 ||
		limiterAttemptCount(harness.api.createIP, "127.0.0.1") != 0 || recoveryIDAfter != recoveryIDBefore || len(orbits) != 1 {
		t.Fatalf("invalid authorization reached work: auth_dispatches=%d rotate_attempts=%d create_attempts=%d recovery_changed=%t orbit_count=%d",
			authDispatches, limiterAttemptCount(harness.api.rotateActor, fmt.Sprint(owner.ActorID)),
			limiterAttemptCount(harness.api.createIP, "127.0.0.1"), recoveryIDAfter != recoveryIDBefore, len(orbits))
	}
}

func TestAttemptLimiterCountsRejectedAttemptsAndConcurrentBoundary(t *testing.T) {
	base := time.Unix(1_700_000_000, 0)
	now := base
	limiter := newAttemptLimiter(2, 10*time.Second, 10)
	limiter.now = func() time.Time { return now }
	if allowed, _ := limiter.reserve("key"); !allowed {
		t.Fatal("first attempt rejected")
	}
	now = base.Add(time.Second)
	if allowed, _ := limiter.reserve("key"); !allowed {
		t.Fatal("second attempt rejected")
	}
	now = base.Add(2 * time.Second)
	allowed, retry := limiter.reserve("key")
	if allowed {
		t.Fatal("N+1 attempt was allowed")
	}
	firstHorizon := now.Add(retry)
	now = base.Add(7 * time.Second)
	allowed, retry = limiter.reserve("key")
	if allowed {
		t.Fatal("further rejected attempt was allowed")
	}
	if horizon := now.Add(retry); !horizon.After(firstHorizon) {
		t.Fatalf("rejected attempt did not advance rolling horizon: first=%d second=%d", firstHorizon.Unix(), horizon.Unix())
	}
	now = base.Add(10 * time.Second)
	allowed, retry = limiter.reserve("key")
	if allowed {
		t.Fatal("rejected attempts were not counted in the rolling window")
	}
	if retry <= 0 {
		t.Fatal("continuing 429 returned non-positive retry")
	}
	now = now.Add(retry)
	if allowed, _ := limiter.reserve("key"); !allowed {
		t.Fatal("request at exact Retry-After boundary was still rejected")
	}

	concurrent := newAttemptLimiter(5, time.Minute, 10)
	start := make(chan struct{})
	results := make(chan bool, 20)
	for i := 0; i < 20; i++ {
		go func() {
			<-start
			allowed, _ := concurrent.reserve("shared")
			results <- allowed
		}()
	}
	close(start)
	successes := 0
	for i := 0; i < 20; i++ {
		if <-results {
			successes++
		}
	}
	if successes != 5 {
		t.Fatalf("concurrent boundary successes=%d want=5", successes)
	}
	if retained := limiterAttemptCount(concurrent, "shared"); retained != 6 {
		t.Fatalf("bounded limiter retained=%d want=6", retained)
	}
}

func TestForwardedHeadersRequireLoopbackProxyPeer(t *testing.T) {
	harness := newOnboardingHarness(t)
	harness.api.config.TrustedProxy = true
	forged := httptest.NewRequest(http.MethodPost, "/v1/onboarding/orbits",
		strings.NewReader(`{"title":"Forged","installation_attempt_id":"forged_attempt_0001"}`))
	forged.RemoteAddr = "203.0.113.20:1234"
	forged.Header.Set("X-Forwarded-Proto", "https")
	forged.Header.Set("X-Real-Ip", "127.0.0.1")
	recorder := httptest.NewRecorder()
	harness.mux.ServeHTTP(recorder, forged)
	assertAPIError(t, recorder, http.StatusBadRequest, errorInvalidRequest, nil)

	proxyRequest := func(peer, proto, xff, realIP string) *http.Request {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req.RemoteAddr = peer
		if proto != "" {
			req.Header.Set("X-Forwarded-Proto", proto)
		}
		if xff != "" {
			req.Header.Set("X-Forwarded-For", xff)
		}
		if realIP != "" {
			req.Header.Set("X-Real-Ip", realIP)
		}
		return req
	}
	if secureRequest(proxyRequest("127.0.0.1:9000", "", "198.51.100.1", ""), true) {
		t.Fatal("loopback proxy request without X-Forwarded-Proto was accepted")
	}
	if secureRequest(proxyRequest("[::1]:9000", "http", "2001:db8::1", ""), true) {
		t.Fatal("loopback proxy plaintext origin was accepted")
	}
	if !secureRequest(proxyRequest("[::1]:9000", "https", "2001:db8::1", ""), true) {
		t.Fatal("authenticated IPv6 loopback TLS proxy request was rejected")
	}
	if secureRequest(proxyRequest("127.0.0.1:9000", "https", "not-an-ip", ""), true) {
		t.Fatal("loopback proxy request with malformed XFF was accepted")
	}
	if secureRequest(proxyRequest("127.0.0.1:9000", "https", "", "198.51.100.4"), true) {
		t.Fatal("loopback proxy request trusted X-Real-IP without XFF")
	}
	duplicateProto := proxyRequest("127.0.0.1:9000", "https", "198.51.100.4", "")
	duplicateProto.Header.Add("X-Forwarded-Proto", "https")
	if secureRequest(duplicateProto, true) {
		t.Fatal("loopback proxy request with duplicate XFP was accepted")
	}
	commaProto := proxyRequest("127.0.0.1:9000", "https,http", "198.51.100.4", "")
	if secureRequest(commaProto, true) {
		t.Fatal("loopback proxy request with comma-valued XFP was accepted")
	}
	emptyProto := proxyRequest("127.0.0.1:9000", "", "", "")
	emptyProto.Header["X-Forwarded-Proto"] = []string{""}
	if secureRequest(emptyProto, true) {
		t.Fatal("loopback proxy request with empty forwarding marker was accepted")
	}
	if !secureRequest(proxyRequest("127.0.0.1:9000", "", "", ""), true) ||
		!secureRequest(proxyRequest("[::1]:9000", "", "", ""), false) {
		t.Fatal("direct loopback local/test request was rejected")
	}
	if got := onboardingClientIP(proxyRequest("127.0.0.1:9000", "https", "192.0.2.1, 198.51.100.9", "203.0.113.99"), true); got != "198.51.100.9" {
		t.Fatalf("proxy last-hop source=%q", got)
	}
	if got := onboardingClientIP(proxyRequest("127.0.0.1:9000", "https", "not-an-ip", "203.0.113.99"), true); got != "127.0.0.1" {
		t.Fatalf("malformed proxy source fallback=%q", got)
	}
	if got := onboardingClientIP(proxyRequest("[::1]:9000", "https", "", "203.0.113.99"), true); got != "::1" {
		t.Fatalf("IPv6 proxy source fallback=%q", got)
	}

	// Even over real TLS, an untrusted direct peer cannot split one source-IP
	// bucket by forging forwarding headers.
	harness.api.createIP = newAttemptLimiter(1, time.Hour, 10_000)
	request := func(attempt, forgedIP string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/v1/onboarding/orbits",
			strings.NewReader(fmt.Sprintf(`{"title":"TLS","installation_attempt_id":%q}`, attempt)))
		req.RemoteAddr = "203.0.113.20:1234"
		req.TLS = &tls.ConnectionState{}
		req.Header.Set("X-Real-Ip", forgedIP)
		req.Header.Set("X-Forwarded-For", forgedIP)
		response := httptest.NewRecorder()
		harness.mux.ServeHTTP(response, req)
		return response
	}
	if response := request("forged_attempt_0002", "198.51.100.1"); response.Code != http.StatusCreated {
		t.Fatalf("first TLS create status=%d response_bytes=%d", response.Code, response.Body.Len())
	}
	assertAPIError(t, request("forged_attempt_0003", "198.51.100.2"), http.StatusTooManyRequests, errorTooManyAttempts,
		func(value *int64) bool { return value != nil && *value > 0 })
	directTLS := proxyRequest("127.0.0.1:9000", "https", "198.51.100.7", "203.0.113.7")
	directTLS.TLS = &tls.ConnectionState{}
	if got := onboardingClientIP(directTLS, true); got != "127.0.0.1" {
		t.Fatalf("direct TLS source trusted spoofed headers: %q", got)
	}
}

func TestOnboardingErrorEnvelopeAndTransportAndFlagOff(t *testing.T) {
	harness := newOnboardingHarness(t)
	assertAPIError(t, apiRequest(harness.mux, http.MethodGet, "/v1/actor/context", "", ""),
		http.StatusUnauthorized, errorUnauthorized, nil)

	request := httptest.NewRequest(http.MethodPost, "/v1/onboarding/orbits",
		strings.NewReader(`{"title":"Remote","installation_attempt_id":"install_attempt_0003"}`))
	request.RemoteAddr = "203.0.113.10:1234"
	recorder := httptest.NewRecorder()
	harness.mux.ServeHTTP(recorder, request)
	assertAPIError(t, recorder, http.StatusBadRequest, errorInvalidRequest, nil)

	request = httptest.NewRequest(http.MethodPost, "/v1/onboarding/orbits",
		strings.NewReader(`{"title":"TLS","installation_attempt_id":"install_attempt_0004"}`))
	request.RemoteAddr = "203.0.113.10:1234"
	request.TLS = &tls.ConnectionState{}
	recorder = httptest.NewRecorder()
	harness.mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusCreated {
		t.Fatalf("TLS create status=%d response_bytes=%d", recorder.Code, recorder.Body.Len())
	}

	offStore, err := store.Open(filepath.Join(t.TempDir(), "off.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer offStore.Close()
	offMux := http.NewServeMux()
	registerOnboardingRoutes(offMux, offStore, &config.Config{}, slog.Default(), "bot")
	recorder = apiRequest(offMux, http.MethodPost, "/v1/onboarding/orbits", `{}`, "")
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("feature-off onboarding route status=%d", recorder.Code)
	}
}

func TestResolverInternalFailureIsGeneric500WithoutCredentialLeak(t *testing.T) {
	harness := newOnboardingHarness(t)
	created, err := harness.store.CreateSelfServiceOrbit("Resolver failure")
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.store.Close(); err != nil {
		t.Fatal(err)
	}
	for _, request := range []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/v1/actor/context", ""},
		{http.MethodPost, "/v1/recovery/rotate", `{}`},
		{http.MethodPost, "/v1/onboarding/orbits", `{"title":"Closed","installation_attempt_id":"closed_store_attempt"}`},
	} {
		recorder := apiRequest(harness.mux, request.method, request.path, request.body, created.ControlToken)
		assertAPIError(t, recorder, http.StatusInternalServerError, errorInternal, nil)
		if strings.Contains(recorder.Body.String(), created.ControlToken) {
			t.Fatalf("credential entered generic error response (response_bytes=%d)", recorder.Body.Len())
		}
	}
	if strings.Contains(harness.logs.String(), created.ControlToken) {
		t.Fatalf("credential entered resolver error logs (log_bytes=%d)", harness.logs.Len())
	}
}

func assertAPIError(t *testing.T, recorder *httptest.ResponseRecorder, status int, code string, retryCheck func(*int64) bool) {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status=%d want=%d response_bytes=%d", recorder.Code, status, recorder.Body.Len())
	}
	if recorder.Header().Get("Content-Type") != "application/json; charset=utf-8" {
		t.Fatalf("content-type=%q", recorder.Header().Get("Content-Type"))
	}
	var body apiErrorBody
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error response bytes=%d: %v", recorder.Body.Len(), err)
	}
	if body.Error.Code != code {
		t.Fatalf("error code=%q want=%q", body.Error.Code, code)
	}
	if retryCheck == nil {
		if body.Error.RetryAfterSeconds != nil {
			t.Fatalf("non-429 retry_after_seconds=%v", *body.Error.RetryAfterSeconds)
		}
	} else if !retryCheck(body.Error.RetryAfterSeconds) {
		t.Fatal("invalid retry_after_seconds")
	}
}
