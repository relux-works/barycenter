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

func contentPolicyRequest(
	handler http.Handler,
	method, path, body, bearer string,
) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:34567"
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func acceptContentPolicyHTTP(
	t *testing.T,
	harness onboardingHarness,
	bearer string,
	locale string,
) contentPolicyGrantResponse {
	t.Helper()
	body := `{"version":"` + store.CurrentContentPolicyVersion +
		`","policy_hash":"` + store.CurrentContentPolicyHash +
		`","locale":"` + locale + `","terms_accepted":true}`
	recorder := contentPolicyRequest(harness.mux, http.MethodPut,
		"/v1/content-policy/acceptance", body, bearer)
	if recorder.Code != http.StatusOK {
		t.Fatalf("accept content policy status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	var response contentPolicyGrantResponse
	if err := json.Unmarshal(recorder.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func acceptContentPolicyStore(
	t *testing.T,
	harness onboardingHarness,
	credentials store.OnboardingCredentials,
	now int64,
) store.ContentPolicyGrant {
	t.Helper()
	grant, err := harness.store.AcceptContentPolicy(store.AcceptContentPolicyParams{
		ExpectedActorID: credentials.ActorID,
		Identity:        store.Identity{Kind: store.IdentityBearer, Token: credentials.ControlToken},
		Version:         store.CurrentContentPolicyVersion, PolicyHash: store.CurrentContentPolicyHash,
		Locale: store.ContentPolicyLocaleEN, AcceptedAt: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	return grant
}

func TestContentPolicyHTTPDisplayAcceptRevokeAndServerTime(t *testing.T) {
	harness := newOnboardingHarness(t)
	identity := createViaAPIWithoutContentPolicy(t, harness)
	control := identity["control_token"].(string)
	node := identity["node_token"].(string)
	frozen := time.Date(2026, 7, 16, 12, 34, 56, 0, time.UTC)
	harness.api.contentPolicyNow = func() time.Time { return frozen }

	for _, locale := range []string{"en", "ru"} {
		display := contentPolicyRequest(harness.mux, http.MethodGet,
			"/v1/content-policy?locale="+locale, "", node)
		if display.Code != http.StatusOK {
			t.Fatalf("display %s status=%d body=%s", locale, display.Code, display.Body.String())
		}
		var manifest contentPolicyResponse
		if err := json.Unmarshal(display.Body.Bytes(), &manifest); err != nil {
			t.Fatal(err)
		}
		if manifest.Contract != "p2-content-policy-consent.v1" || manifest.Locale != locale ||
			manifest.Version != store.CurrentContentPolicyVersion ||
			manifest.PolicyHash != store.CurrentContentPolicyHash ||
			manifest.TermsURL != store.ContentPolicyTermsURL ||
			manifest.ContentGuidelinesURL != store.ContentPolicyGuidelinesURL ||
			manifest.RightsText == "" || manifest.ConsentText == "" {
			t.Fatalf("manifest=%+v", manifest)
		}
	}
	invalidDisplay := contentPolicyRequest(harness.mux, http.MethodGet,
		"/v1/content-policy?locale=ka", "", control)
	if invalidDisplay.Code != http.StatusBadRequest {
		t.Fatalf("invalid display status=%d", invalidDisplay.Code)
	}
	nodeAccept := contentPolicyRequest(harness.mux, http.MethodPut,
		"/v1/content-policy/acceptance",
		`{"version":"1.0","policy_hash":"`+store.CurrentContentPolicyHash+`","locale":"en","terms_accepted":true}`,
		node)
	if nodeAccept.Code != http.StatusForbidden {
		t.Fatalf("node accept status=%d body=%s", nodeAccept.Code, nodeAccept.Body.String())
	}
	wrongHash := contentPolicyRequest(harness.mux, http.MethodPut,
		"/v1/content-policy/acceptance",
		`{"version":"1.0","policy_hash":"`+strings.Repeat("0", 64)+`","locale":"en","terms_accepted":true}`,
		control)
	if wrongHash.Code != http.StatusBadRequest {
		t.Fatalf("wrong hash status=%d body=%s", wrongHash.Code, wrongHash.Body.String())
	}
	missing := contentPolicyRequest(harness.mux, http.MethodGet,
		"/v1/content-policy/acceptance", "", control)
	if missing.Code != http.StatusPreconditionRequired {
		t.Fatalf("missing acceptance status=%d body=%s", missing.Code, missing.Body.String())
	}
	grant := acceptContentPolicyHTTP(t, harness, control, "en")
	if !grant.Current || !grant.TermsAccepted || grant.Revision != 1 ||
		grant.AcceptedAt != frozen.Format(time.RFC3339) {
		t.Fatalf("grant=%+v", grant)
	}
	current := contentPolicyRequest(harness.mux, http.MethodGet,
		"/v1/content-policy/acceptance", "", control)
	if current.Code != http.StatusOK {
		t.Fatalf("current acceptance status=%d body=%s", current.Code, current.Body.String())
	}

	harness.api.contentPolicyNow = func() time.Time { return frozen.Add(time.Second) }
	revoked := contentPolicyRequest(harness.mux, http.MethodDelete,
		"/v1/content-policy/acceptance?locale=en", "", control)
	if revoked.Code != http.StatusOK {
		t.Fatalf("revoke status=%d body=%s", revoked.Code, revoked.Body.String())
	}
	var revokeGrant contentPolicyGrantResponse
	if err := json.Unmarshal(revoked.Body.Bytes(), &revokeGrant); err != nil {
		t.Fatal(err)
	}
	if revokeGrant.Current || revokeGrant.Revision != 2 ||
		revokeGrant.RevokedAt != frozen.Add(time.Second).Format(time.RFC3339) {
		t.Fatalf("revoked=%+v", revokeGrant)
	}
}

func TestContentPolicyHTTPRequiresGrantAndPerUploadRightsAcknowledgement(t *testing.T) {
	harness := newOnboardingHarness(t)
	identity := createViaAPIWithoutContentPolicy(t, harness)
	control := identity["control_token"].(string)

	missingGrant := createMediaUploadRequest(
		harness.mux, control, "policy-missing-grant-0001", 4,
	)
	if missingGrant.Code != http.StatusPreconditionRequired ||
		!strings.Contains(missingGrant.Body.String(), errorContentPolicyAcceptance) {
		t.Fatalf("missing grant status=%d body=%s", missingGrant.Code, missingGrant.Body.String())
	}
	acceptContentPolicyHTTP(t, harness, control, "ru")
	request := httptest.NewRequest(http.MethodPost, "/v1/media/uploads", strings.NewReader(
		`{"kind":"voice_clip","title":"Morning note","size_bytes":4,"rights_acknowledged":false}`,
	))
	request.RemoteAddr = "127.0.0.1:34567"
	request.Header.Set("Authorization", "Bearer "+control)
	request.Header.Set("Idempotency-Key", "policy-missing-rights-0001")
	recorder := httptest.NewRecorder()
	harness.mux.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusPreconditionRequired ||
		!strings.Contains(recorder.Body.String(), errorContentPolicyAcceptance) {
		t.Fatalf("missing rights status=%d body=%s", recorder.Code, recorder.Body.String())
	}
	created := createMediaUploadRequest(harness.mux, control, "policy-complete-0001", 4)
	if created.Code != http.StatusCreated {
		t.Fatalf("accepted upload status=%d body=%s", created.Code, created.Body.String())
	}
}
