package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHumanSecretNormalizationIsASCIIOnly(t *testing.T) {
	canonical := "ABCDEFGHJKMNPQRSTVWXYZ23456"
	for _, input := range []string{
		"abcdefgh-jkmnp-qrstv-wxyz2-3456",
		"ABCD EFGH\tJKMNP\nQRSTV-WXYZ2-3456",
		canonical,
	} {
		got, ok := normalizeHumanSecret(input)
		if !ok || got != canonical {
			t.Fatalf("normalize %q = %q, %t", input, got, ok)
		}
	}
	for _, canary := range []string{
		"KBCDEFGHJKMNPQRSTVWXYZ23456",
		"ſBCDEFGHJKMNPQRSTVWXYZ23456",
		"ıBCDEFGHJKMNPQRSTVWXYZ23456",
		"İBCDEFGHJKMNPQRSTVWXYZ23456",
		"ＡBCDEFGHJKMNPQRSTVWXYZ23456",
		"A\u030aBCDEFGHJKMNPQRSTVWXYZ23456",
	} {
		if got, ok := normalizeHumanSecret(canary); ok {
			t.Fatalf("non-ASCII canary normalized to %q", got)
		}
		if _, err := NewRecoveryInput(9, "rec_0123456789abcdef0123456789abcdef", canary); err == nil {
			t.Fatalf("non-ASCII recovery canary accepted: %q", canary)
		}
	}
}

func TestHumanSecretRawInputIsBoundedBeforeTransport(t *testing.T) {
	var calls atomic.Int32
	client := newHTTPTestClient(t, func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return nil, errors.New("must not send")
	})
	oversized := strings.Repeat("A", 1<<20)
	if _, err := client.JoinOrbit(context.Background(), oversized); err == nil {
		t.Fatal("oversized invite accepted")
	}
	if _, err := NewRecoveryInput(9, "rec_0123456789abcdef0123456789abcdef", oversized); err == nil {
		t.Fatal("oversized recovery secret accepted")
	}
	if calls.Load() != 0 {
		t.Fatalf("oversized input reached transport %d times", calls.Load())
	}
}

func TestOnboardingHTTPZerosOwnedRequestBodyAfterTransport(t *testing.T) {
	var captured *http.Request
	client := newHTTPTestClient(t, func(request *http.Request) (*http.Response, error) {
		captured = request
		return nil, errors.New("transport stopped before consuming body")
	})
	_, _ = client.JoinOrbit(context.Background(), "ABCDEFGHJKMNPQRSTVWXYZ23456")
	if captured == nil || captured.Body == nil {
		t.Fatal("request was not captured")
	}
	remaining, err := io.ReadAll(captured.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(remaining) == 0 {
		t.Fatal("captured request body was unexpectedly empty")
	}
	for _, value := range remaining {
		if value != 0 {
			t.Fatalf("mutable request body survived transport: %q", remaining)
		}
	}
}

func TestOnboardingHTTPClearsBearerFromOwnedRequestAfterTransport(t *testing.T) {
	var captured *http.Request
	client := newHTTPTestClient(t, func(request *http.Request) (*http.Response, error) {
		captured = request
		if request.Header.Get("Authorization") == "" {
			t.Fatal("transport did not receive bearer")
		}
		return nil, errors.New("stop after capture")
	})
	_, _ = client.ActorContext(context.Background(), activeControlCapability(t))
	if captured == nil {
		t.Fatal("request was not captured")
	}
	if authorization := captured.Header.Get("Authorization"); authorization != "" {
		t.Fatalf("owned request retained bearer after transport: %q", authorization)
	}

	var finalRequest *http.Request
	client = newHTTPTestClient(t, func(request *http.Request) (*http.Response, error) {
		finalRequest = request.Clone(request.Context())
		finalRequest.Header["authorization"] = []string{request.Header.Get("Authorization")}
		response := jsonResponse(http.StatusOK, []byte(`{"orbit_id":7,"actor_id":9,"role":"primary"}`))
		response.Request = finalRequest
		return response, nil
	})
	if _, err := client.ActorContext(context.Background(), activeControlCapability(t)); err != nil {
		t.Fatal(err)
	}
	for key, values := range finalRequest.Header {
		if strings.EqualFold(key, "Authorization") {
			t.Fatalf("response final request retained bearer after transport: %q=%q", key, values)
		}
	}
}

type httpDoerFunc func(*http.Request) (*http.Response, error)

type alienActorCapability struct {
	origin CoordinatorOrigin
	token  string
}

func (a alienActorCapability) actorBearer() (CoordinatorOrigin, string) { return a.origin, a.token }

func (f httpDoerFunc) Do(request *http.Request) (*http.Response, error) {
	response, err := f(request)
	if response != nil && response.Request == nil {
		response.Request = request
	}
	return response, err
}

func jsonResponse(status int, body []byte) *http.Response {
	header := make(http.Header)
	header.Set("Content-Type", "application/json; charset=utf-8")
	header.Set("Cache-Control", "no-store")
	return &http.Response{StatusCode: status, Header: header, Body: io.NopCloser(bytes.NewReader(body))}
}

func newHTTPTestClient(t *testing.T, handler func(*http.Request) (*http.Response, error)) *OnboardingClient {
	t.Helper()
	client, err := NewOnboardingClient("https://coord.example", httpDoerFunc(handler))
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func testCreateResponse() []byte {
	value := map[string]any{
		"orbit_id": 7, "title": "Home", "actor_id": 9, "role": "primary", "slot": "b",
		"node_token": strings.Repeat("ab", 32), "control_token": strings.Repeat("cd", 32),
		"recovery_id":     "rec_0123456789abcdef0123456789abcdef",
		"recovery_secret": "ABCDEFGHJKMNPQRSTVWXYZ23456", "shown_once": true,
	}
	result, _ := json.Marshal(value)
	return result
}

func testJoinResponse() []byte {
	value := map[string]any{
		"orbit_id": 7, "title": "Home", "actor_id": 9, "role": "companion", "slot": "b",
		"node_token": strings.Repeat("ab", 32), "control_token": strings.Repeat("cd", 32),
	}
	result, _ := json.Marshal(value)
	return result
}

func activeControlCapability(t *testing.T) ControlCapability {
	t.Helper()
	origin, err := CanonicalCoordinatorOrigin("https://coord.example")
	if err != nil {
		t.Fatal(err)
	}
	return ControlCapability{origin: origin, value: ControlCredential{ActorID: 9, OrbitID: 7, Role: "primary", ControlToken: strings.Repeat("cd", 32), Context: ControlContextActive}}
}

func TestOnboardingHTTPExactRoutesAndSchemas(t *testing.T) {
	t.Run("create", func(t *testing.T) {
		client := newHTTPTestClient(t, func(request *http.Request) (*http.Response, error) {
			assertRequest(t, request, http.MethodPost, "/v1/onboarding/orbits", "")
			assertExactRequestObject(t, request, map[string]any{"title": "Home", "installation_attempt_id": "attempt_0123456789"})
			return jsonResponse(http.StatusCreated, testCreateResponse()), nil
		})
		result, err := client.CreateOrbit(context.Background(), "Home", "attempt_0123456789")
		if err != nil {
			t.Fatal(err)
		}
		if result.Bundle.Node.WSURL != "wss://coord.example/ws" || result.Bundle.Control.ControlToken != strings.Repeat("cd", 32) {
			t.Fatalf("unexpected create result %#v", result)
		}
		_, recoveryID, secret, ok := result.Recovery.RevealForDisplay()
		if !ok || recoveryID == "" || secret != "ABCDEFGHJKMNPQRSTVWXYZ23456" {
			t.Fatal("one-time recovery material missing")
		}
	})

	t.Run("invite issue", func(t *testing.T) {
		client := newHTTPTestClient(t, func(request *http.Request) (*http.Response, error) {
			assertRequest(t, request, http.MethodPost, "/v1/device-invites", strings.Repeat("cd", 32))
			assertExactRequestObject(t, request, map[string]any{"intended_role": "satellite"})
			return jsonResponse(http.StatusCreated, []byte(`{"invite_code":"ABCDEFGHJKMNPQRSTVWXYZ23456","intended_role":"satellite","expires_at":"2026-07-13T12:00:00Z"}`)), nil
		})
		invite, err := client.IssueDeviceInvite(context.Background(), activeControlCapability(t), "satellite")
		if err != nil || invite.IntendedRole != "satellite" {
			t.Fatalf("invite %#v err=%v", invite, err)
		}
	})

	t.Run("join", func(t *testing.T) {
		client := newHTTPTestClient(t, func(request *http.Request) (*http.Response, error) {
			assertRequest(t, request, http.MethodPost, "/v1/device-invites/consume", "")
			assertExactRequestObject(t, request, map[string]any{"invite_code": "ABCDEFGHJKMNPQRSTVWXYZ23456"})
			return jsonResponse(http.StatusOK, testJoinResponse()), nil
		})
		result, err := client.JoinOrbit(context.Background(), "ABCD-EFGH-JKMN-PQRS-TVWX-YZ23456")
		if err != nil || result.Bundle.Control == nil || result.Bundle.RecoveryID != "" {
			t.Fatalf("join %#v err=%v", result, err)
		}
	})

	t.Run("context", func(t *testing.T) {
		client := newHTTPTestClient(t, func(request *http.Request) (*http.Response, error) {
			assertRequest(t, request, http.MethodGet, "/v1/actor/context", strings.Repeat("cd", 32))
			if request.Body != nil {
				raw, _ := io.ReadAll(request.Body)
				if len(raw) != 0 {
					t.Fatalf("GET body %q", raw)
				}
			}
			return jsonResponse(http.StatusOK, []byte(`{"orbit_id":7,"actor_id":9,"role":"primary"}`)), nil
		})
		contextResult, err := client.ActorContext(context.Background(), activeControlCapability(t))
		if err != nil || contextResult.ActorID != 9 {
			t.Fatalf("context %#v err=%v", contextResult, err)
		}
	})

	t.Run("rotate", func(t *testing.T) {
		client := newHTTPTestClient(t, func(request *http.Request) (*http.Response, error) {
			assertRequest(t, request, http.MethodPost, "/v1/recovery/rotate", strings.Repeat("cd", 32))
			raw, _ := io.ReadAll(request.Body)
			if string(raw) != "{}" {
				t.Fatalf("rotate body %q", raw)
			}
			return jsonResponse(http.StatusOK, []byte(`{"actor_id":9,"recovery_id":"rec_0123456789abcdef0123456789abcdef","recovery_secret":"ABCDEFGHJKMNPQRSTVWXYZ23456","shown_once":true}`)), nil
		})
		material, err := client.RotateRecovery(context.Background(), activeControlCapability(t))
		if err != nil || material == nil {
			t.Fatalf("rotate material=%v err=%v", material, err)
		}
	})

	t.Run("telegram", func(t *testing.T) {
		client := newHTTPTestClient(t, func(request *http.Request) (*http.Response, error) {
			assertRequest(t, request, http.MethodPost, "/v1/telegram-links", strings.Repeat("cd", 32))
			assertExactRequestObject(t, request, map[string]any{"desired_role": "companion"})
			return jsonResponse(http.StatusCreated, []byte(`{"link_code":"ABCDEFGHJKMNPQRSTVWXYZ23456","desired_role":"companion","expires_at":"2026-07-13T12:00:00Z","bot_username":"barycenter_bot"}`)), nil
		})
		link, err := client.IssueTelegramLink(context.Background(), activeControlCapability(t), "companion")
		if err != nil || link.BotUsername != "barycenter_bot" {
			t.Fatalf("link %#v err=%v", link, err)
		}
		if strings.Contains(fmt.Sprint(link), "ABCDEFGH") || strings.Contains(fmt.Sprintf("%#v", link), "ABCDEFGH") {
			t.Fatal("link formatting exposed code")
		}
	})
}

func TestOnboardingHTTPRecoveryRequestIsBodyOnly(t *testing.T) {
	client := newHTTPTestClient(t, func(request *http.Request) (*http.Response, error) {
		assertRequest(t, request, http.MethodPost, "/v1/recovery/consume", "")
		if request.URL.RawQuery != "" || request.URL.Fragment != "" || strings.Contains(request.URL.Path, "ABCDEFGH") {
			t.Fatalf("secret escaped into URL: %#v", request.URL)
		}
		assertExactRequestObject(t, request, map[string]any{
			"recovery_id":               "rec_0123456789abcdef0123456789abcdef",
			"recovery_secret":           "ABCDEFGHJKMNPQRSTVWXYZ23456",
			"replacement_control_token": strings.Repeat("ef", 32),
		})
		return jsonResponse(http.StatusOK, []byte(`{"orbit_id":7,"actor_id":9,"role":"companion"}`)), nil
	})
	result, err := client.consumeRecovery(context.Background(), "rec_0123456789abcdef0123456789abcdef", []byte("ABCDEFGHJKMNPQRSTVWXYZ23456"), strings.Repeat("ef", 32))
	if err != nil || result.ActorID != 9 {
		t.Fatalf("recovery %#v err=%v", result, err)
	}
}

func TestOnboardingHTTPStrictErrorsAndRedaction(t *testing.T) {
	canaries := []string{"SECRET-BODY-CANARY", "SECRET-TOKEN-CANARY", "SECRET-URL-CANARY"}
	errorBody := []byte(`{"error":{"code":"too_many_attempts","message":"Too many attempts. Please wait before retrying.","retry_after_seconds":17}}`)
	client := newHTTPTestClient(t, func(*http.Request) (*http.Response, error) {
		response := jsonResponse(http.StatusTooManyRequests, errorBody)
		response.Header.Set("Retry-After", "17")
		response.Header.Set("Location", "https://coord.example/?code=SECRET-URL-CANARY")
		return response, nil
	})
	_, err := client.CreateOrbit(context.Background(), "Home", "attempt_0123456789")
	var clientErr *OnboardingClientError
	if !errors.As(err, &clientErr) || clientErr.Kind != ClientErrorRateLimited || clientErr.RetryAfterSeconds != 17 {
		t.Fatalf("unexpected error %#v", err)
	}
	for _, rendered := range []string{err.Error(), fmt.Sprintf("%#v", err)} {
		for _, canary := range canaries {
			if strings.Contains(rendered, canary) {
				t.Fatalf("error leaked %q: %q", canary, rendered)
			}
		}
	}

	transport := newHTTPTestClient(t, func(*http.Request) (*http.Response, error) {
		return nil, errors.New("SECRET-TOKEN-CANARY https://coord.example/private")
	})
	_, err = transport.CreateOrbit(context.Background(), "Home", "attempt_0123456789")
	if err == nil || strings.Contains(err.Error(), "CANARY") || strings.Contains(fmt.Sprintf("%#v", err), "coord.example") {
		t.Fatalf("transport error not redacted: %#v", err)
	}
}

func TestOnboardingHTTPRejectsNoncanonicalRetryAfterHeaders(t *testing.T) {
	errorBody := []byte(`{"error":{"code":"too_many_attempts","message":"Too many attempts. Please wait before retrying.","retry_after_seconds":17}}`)
	tests := []struct {
		name   string
		values []string
	}{
		{"leading plus", []string{"+17"}},
		{"leading zero", []string{"017"}},
		{"surrounding whitespace", []string{" 17 "}},
		{"duplicate", []string{"17", "17"}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newHTTPTestClient(t, func(*http.Request) (*http.Response, error) {
				response := jsonResponse(http.StatusTooManyRequests, errorBody)
				response.Header["Retry-After"] = append([]string(nil), test.values...)
				return response, nil
			})
			_, err := client.CreateOrbit(context.Background(), "Home", "attempt_0123456789")
			var clientErr *OnboardingClientError
			if !errors.As(err, &clientErr) || clientErr.Kind != ClientErrorInvalidResponse {
				t.Fatalf("noncanonical Retry-After accepted: %#v", err)
			}
		})
	}

	t.Run("present on non-rate-limit response", func(t *testing.T) {
		client := newHTTPTestClient(t, func(*http.Request) (*http.Response, error) {
			response := apiErrorResponse(http.StatusBadRequest, "invalid_request", "The request is malformed or contains invalid parameters.", 0)
			response.Header["Retry-After"] = []string{""}
			return response, nil
		})
		_, err := client.CreateOrbit(context.Background(), "Home", "attempt_0123456789")
		var clientErr *OnboardingClientError
		if !errors.As(err, &clientErr) || clientErr.Kind != ClientErrorInvalidResponse {
			t.Fatalf("non-rate-limit Retry-After accepted: %#v", err)
		}
	})
}

func TestOnboardingHTTPRejectsInvalidRetryAfterBodies(t *testing.T) {
	for _, value := range []string{
		"null", `"17"`, "true", `{}`, `[]`, "0", "-1", "17.0", "1.7e1", "9223372036854775808",
	} {
		t.Run(value, func(t *testing.T) {
			body := []byte(`{"error":{"code":"too_many_attempts","message":"Too many attempts. Please wait before retrying.","retry_after_seconds":` + value + `}}`)
			client := newHTTPTestClient(t, func(*http.Request) (*http.Response, error) {
				response := jsonResponse(http.StatusTooManyRequests, body)
				response.Header.Set("Retry-After", "17")
				return response, nil
			})
			_, err := client.CreateOrbit(context.Background(), "Home", "attempt_0123456789")
			var clientErr *OnboardingClientError
			if !errors.As(err, &clientErr) || clientErr.Kind != ClientErrorInvalidResponse {
				t.Fatalf("invalid retry_after_seconds=%s accepted: %#v", value, err)
			}
		})
	}
}

func TestOnboardingHTTPRejectsRedirectOversizeAndMalformedJSON(t *testing.T) {
	tests := []struct {
		name     string
		response func() *http.Response
	}{
		{"redirect", func() *http.Response {
			response := jsonResponse(http.StatusTemporaryRedirect, []byte(`{}`))
			response.Header.Set("Location", "https://coord.example/?secret=canary")
			return response
		}},
		{"oversize", func() *http.Response {
			return jsonResponse(http.StatusCreated, bytes.Repeat([]byte{'x'}, maximumHTTPResponseBytes+1))
		}},
		{"duplicate", func() *http.Response { return jsonResponse(http.StatusCreated, []byte(`{"orbit_id":7,"orbit_id":7}`)) }},
		{"trailing", func() *http.Response {
			return jsonResponse(http.StatusCreated, append(testCreateResponse(), []byte(` {}`)...))
		}},
		{"unknown", func() *http.Response { return jsonResponse(http.StatusCreated, []byte(`{"unknown":true}`)) }},
		{"wrong scalar", func() *http.Response { return jsonResponse(http.StatusCreated, []byte(`{"orbit_id":"7"}`)) }},
		{"duplicate content type", func() *http.Response {
			response := jsonResponse(http.StatusCreated, testCreateResponse())
			response.Header["Content-Type"] = []string{"application/json", "application/json"}
			return response
		}},
		{"wrong json charset", func() *http.Response {
			response := jsonResponse(http.StatusCreated, testCreateResponse())
			response.Header.Set("Content-Type", "application/json; charset=iso-8859-1")
			return response
		}},
		{"unknown json media parameter", func() *http.Response {
			response := jsonResponse(http.StatusCreated, testCreateResponse())
			response.Header.Set("Content-Type", "application/json; profile=canary")
			return response
		}},
		{"missing no-store", func() *http.Response {
			response := jsonResponse(http.StatusCreated, testCreateResponse())
			response.Header.Del("Cache-Control")
			return response
		}},
		{"deep", func() *http.Response {
			return jsonResponse(http.StatusCreated, []byte(strings.Repeat(`{"x":`, maximumJSONDepth+2)+`null`+strings.Repeat(`}`, maximumJSONDepth+2)))
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newHTTPTestClient(t, func(*http.Request) (*http.Response, error) { return test.response(), nil })
			if _, err := client.CreateOrbit(context.Background(), "Home", "attempt_0123456789"); err == nil {
				t.Fatal("invalid response accepted")
			}
		})
	}
}

func TestOnboardingHTTPRejectsInvalidJSONUnicode(t *testing.T) {
	validSuffix := []byte(`","actor_id":9,"role":"companion","slot":"b","node_token":"` + strings.Repeat("ab", 32) + `","control_token":"` + strings.Repeat("cd", 32) + `"}`)
	validPair := append([]byte(`{"orbit_id":7,"title":"\ud83d\ude80`), validSuffix...)
	client := newHTTPTestClient(t, func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, validPair), nil
	})
	result, err := client.JoinOrbit(context.Background(), "ABCDEFGHJKMNPQRSTVWXYZ23456")
	if err != nil || result.Title != "🚀" {
		t.Fatalf("valid surrogate pair rejected: title=%q err=%v", result.Title, err)
	}

	invalidUTF8 := append([]byte(`{"orbit_id":7,"title":"`), 0xff)
	invalidUTF8 = append(invalidUTF8, validSuffix...)
	tests := map[string][]byte{
		"invalid utf8":         invalidUTF8,
		"unpaired high escape": append([]byte(`{"orbit_id":7,"title":"\ud800`), validSuffix...),
		"unpaired low escape":  append([]byte(`{"orbit_id":7,"title":"\udc00`), validSuffix...),
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			client := newHTTPTestClient(t, func(*http.Request) (*http.Response, error) {
				return jsonResponse(http.StatusOK, body), nil
			})
			if _, err := client.JoinOrbit(context.Background(), "ABCDEFGHJKMNPQRSTVWXYZ23456"); err == nil {
				t.Fatal("invalid JSON Unicode accepted")
			}
		})
	}
}

type closeTrackingBody struct {
	reader   io.Reader
	closed   bool
	closeErr error
}

type observedResponseBody struct {
	data     []byte
	offset   int
	observed []byte
}

func (b *observedResponseBody) Read(destination []byte) (int, error) {
	if b.offset == len(b.data) {
		return 0, io.EOF
	}
	n := copy(destination, b.data[b.offset:])
	b.observed = destination[:n]
	b.offset += n
	return n, nil
}

func (*observedResponseBody) Close() error { return nil }

type failingResponseBody struct{ err error }

func (b *failingResponseBody) Read([]byte) (int, error) { return 0, b.err }
func (*failingResponseBody) Close() error               { return nil }

func (b *closeTrackingBody) Read(value []byte) (int, error) { return b.reader.Read(value) }
func (b *closeTrackingBody) Close() error {
	b.closed = true
	return b.closeErr
}

func TestOnboardingHTTPClosesBodiesOnEveryPath(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		body   []byte
	}{
		{"success", http.StatusCreated, testCreateResponse()},
		{"error", http.StatusForbidden, []byte(`{"error":{"code":"insufficient_capability","message":"This token does not have the required capability.","retry_after_seconds":null}}`)},
		{"redirect", http.StatusTemporaryRedirect, []byte(`{}`)},
		{"oversize", http.StatusCreated, bytes.Repeat([]byte{'x'}, maximumHTTPResponseBytes+1)},
	} {
		t.Run(test.name, func(t *testing.T) {
			body := &closeTrackingBody{reader: bytes.NewReader(test.body)}
			client := newHTTPTestClient(t, func(*http.Request) (*http.Response, error) {
				response := jsonResponse(test.status, nil)
				response.Body = body
				return response, nil
			})
			_, _ = client.CreateOrbit(context.Background(), "Home", "attempt_0123456789")
			if !body.closed {
				t.Fatal("response body was not closed")
			}
		})
	}
	responseBody := &closeTrackingBody{reader: strings.NewReader("unused")}
	client := newHTTPTestClient(t, func(*http.Request) (*http.Response, error) {
		return &http.Response{Body: responseBody}, errors.New("transport canary")
	})
	_, _ = client.CreateOrbit(context.Background(), "Home", "attempt_0123456789")
	if !responseBody.closed {
		t.Fatal("response accompanying transport error was not closed")
	}
}

func TestOnboardingHTTPZerosOwnedResponseBufferAfterDecode(t *testing.T) {
	body := &observedResponseBody{data: testCreateResponse()}
	client := newHTTPTestClient(t, func(*http.Request) (*http.Response, error) {
		response := jsonResponse(http.StatusCreated, nil)
		response.Body = body
		return response, nil
	})
	if _, err := client.CreateOrbit(context.Background(), "Home", "attempt_0123456789"); err != nil {
		t.Fatal(err)
	}
	if len(body.observed) == 0 {
		t.Fatal("response reader did not observe the owned decode buffer")
	}
	for _, value := range body.observed {
		if value != 0 {
			t.Fatalf("mutable response buffer survived strict decode: %q", body.observed)
		}
	}
}

func TestOnboardingHTTPRejectsEndpointInvalidStatusCodePair(t *testing.T) {
	client := newHTTPTestClient(t, func(*http.Request) (*http.Response, error) {
		return apiErrorResponse(403, "credential_invalid", "The provided credential is not valid.", 0), nil
	})
	_, err := client.ActorContext(context.Background(), activeControlCapability(t))
	var clientErr *OnboardingClientError
	if !errors.As(err, &clientErr) || clientErr.Kind != ClientErrorInvalidResponse {
		t.Fatalf("authenticated endpoint accepted consume-only error: %#v", err)
	}
}

func TestOnboardingHTTPOriginBoundCapabilities(t *testing.T) {
	called := false
	client := newHTTPTestClient(t, func(*http.Request) (*http.Response, error) {
		called = true
		return nil, errors.New("must not call")
	})
	other, _ := CanonicalCoordinatorOrigin("https://other.example")
	capability := ControlCapability{origin: other, value: activeControlCapability(t).value}
	if _, err := client.IssueTelegramLink(context.Background(), capability, "companion"); err == nil || called {
		t.Fatalf("cross-origin bearer accepted err=%v called=%t", err, called)
	}
}

func TestOnboardingHTTPActorContextIdentityAndCapabilityRobustness(t *testing.T) {
	var calls atomic.Int32
	client := newHTTPTestClient(t, func(*http.Request) (*http.Response, error) {
		calls.Add(1)
		return jsonResponse(http.StatusOK, []byte(`{"orbit_id":7,"actor_id":9,"role":"companion"}`)), nil
	})
	var nilNode *NodeCapability
	if _, err := client.ActorContext(context.Background(), nilNode); err == nil {
		t.Fatal("typed-nil node capability accepted")
	}
	alien := alienActorCapability{origin: client.Origin(), token: strings.Repeat("ab", 32)}
	if _, err := client.ActorContext(context.Background(), alien); err == nil {
		t.Fatal("alien sealed capability accepted")
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid capabilities reached transport %d times", calls.Load())
	}
	control := ControlCapability{origin: client.Origin(), value: ControlCredential{ActorID: 9, OrbitID: 7, Role: "primary", ControlToken: strings.Repeat("cd", 32), Context: ControlContextActive}}
	contextResult, err := client.ActorContext(context.Background(), control)
	if err != nil || contextResult.Role != "companion" {
		t.Fatalf("legitimate role refresh rejected: %#v err=%v", contextResult, err)
	}

	controlMismatch := newHTTPTestClient(t, func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, []byte(`{"orbit_id":7,"actor_id":10,"role":"primary"}`)), nil
	})
	if _, err := controlMismatch.ActorContext(context.Background(), activeControlCapability(t)); err == nil {
		t.Fatal("cross-actor control context accepted")
	}
	nodeMismatch := newHTTPTestClient(t, func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, []byte(`{"orbit_id":8,"actor_id":9,"role":"primary"}`)), nil
	})
	node := NodeCapability{origin: nodeMismatch.Origin(), value: NodeCredential{OrbitID: 7, Slot: "b", NodeToken: strings.Repeat("ab", 32), WSURL: "wss://coord.example/ws"}}
	if _, err := nodeMismatch.ActorContext(context.Background(), node); err == nil {
		t.Fatal("cross-orbit node context accepted")
	}
}

func TestOnboardingHTTPCancellationIsStable(t *testing.T) {
	client := newHTTPTestClient(t, func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := client.CreateOrbit(ctx, "Home", "attempt_0123456789")
	var clientErr *OnboardingClientError
	if !errors.As(err, &clientErr) || clientErr.Kind != ClientErrorCancelled {
		t.Fatalf("cancellation %#v", err)
	}

	for _, readErr := range []error{context.Canceled, context.DeadlineExceeded} {
		client = newHTTPTestClient(t, func(*http.Request) (*http.Response, error) {
			response := jsonResponse(http.StatusCreated, nil)
			response.Body = &failingResponseBody{err: readErr}
			return response, nil
		})
		_, err = client.CreateOrbit(context.Background(), "Home", "attempt_0123456789")
		clientErr = nil
		if !errors.As(err, &clientErr) || clientErr.Kind != ClientErrorCancelled {
			t.Fatalf("body-read cancellation %v classified as %#v", readErr, err)
		}
	}
}

func TestOnboardingHTTPRejectsNoncanonicalSuccessValues(t *testing.T) {
	createMutations := []func(map[string]any){
		func(value map[string]any) { value["role"] = "companion" },
		func(value map[string]any) { value["title"] = "   " },
		func(value map[string]any) { value["title"] = "Other" },
		func(value map[string]any) { value["title"] = " Home " },
		func(value map[string]any) { value["title"] = strings.Repeat("x", 121) },
		func(value map[string]any) { value["slot"] = "B" },
		func(value map[string]any) { value["node_token"] = strings.Repeat("AB", 32) },
	}
	for index, mutate := range createMutations {
		var value map[string]any
		if err := json.Unmarshal(testCreateResponse(), &value); err != nil {
			t.Fatal(err)
		}
		mutate(value)
		body, _ := json.Marshal(value)
		client := newHTTPTestClient(t, func(*http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusCreated, body), nil
		})
		if _, err := client.CreateOrbit(context.Background(), "Home", "attempt_0123456789"); err == nil {
			t.Fatalf("create mutation %d accepted", index)
		}
	}

	var join map[string]any
	_ = json.Unmarshal(testJoinResponse(), &join)
	join["role"] = "primary"
	joinBody, _ := json.Marshal(join)
	joinClient := newHTTPTestClient(t, func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, joinBody), nil
	})
	if _, err := joinClient.JoinOrbit(context.Background(), "ABCDEFGHJKMNPQRSTVWXYZ23456"); err == nil {
		t.Fatal("join accepted primary role")
	}

	for _, username := range []string{"@barycenter_bot", "bot", strings.Repeat("a", 33), "bary-center_bot", "бот_bot"} {
		body, _ := json.Marshal(map[string]any{
			"link_code": "ABCDEFGHJKMNPQRSTVWXYZ23456", "desired_role": "companion",
			"expires_at": "2026-07-13T12:00:00Z", "bot_username": username,
		})
		client := newHTTPTestClient(t, func(*http.Request) (*http.Response, error) {
			return jsonResponse(http.StatusCreated, body), nil
		})
		if _, err := client.IssueTelegramLink(context.Background(), activeControlCapability(t), "companion"); err == nil {
			t.Fatalf("bot username %q accepted", username)
		}
	}
}

func TestOnboardingHTTPRejectsAlienEndpointErrors(t *testing.T) {
	tests := []struct {
		name    string
		status  int
		code    string
		message string
		call    func(*OnboardingClient) error
	}{
		{"telegram credential", 403, "credential_invalid", "The provided credential is not valid.", func(client *OnboardingClient) error {
			_, err := client.IssueTelegramLink(context.Background(), activeControlCapability(t), "companion")
			return err
		}},
		{"context credential", 403, "credential_invalid", "The provided credential is not valid.", func(client *OnboardingClient) error {
			_, err := client.ActorContext(context.Background(), activeControlCapability(t))
			return err
		}},
		{"recovery capability", 403, "insufficient_capability", "This token does not have the required capability.", func(client *OnboardingClient) error {
			_, err := client.consumeRecovery(context.Background(), "rec_0123456789abcdef0123456789abcdef", []byte("ABCDEFGHJKMNPQRSTVWXYZ23456"), strings.Repeat("ef", 32))
			return err
		}},
		{"create capability", 403, "insufficient_capability", "This token does not have the required capability.", func(client *OnboardingClient) error {
			_, err := client.CreateOrbit(context.Background(), "Home", "attempt_0123456789")
			return err
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			client := newHTTPTestClient(t, func(*http.Request) (*http.Response, error) {
				return apiErrorResponse(test.status, test.code, test.message, 0), nil
			})
			err := test.call(client)
			var clientErr *OnboardingClientError
			if !errors.As(err, &clientErr) || clientErr.Kind != ClientErrorInvalidResponse {
				t.Fatalf("alien pair returned %T %v", err, err)
			}
		})
	}
}

func TestOnboardingHTTPDisablesRealClientRedirectReplay(t *testing.T) {
	for _, status := range []int{http.StatusTemporaryRedirect, http.StatusPermanentRedirect} {
		for _, crossOrigin := range []bool{false, true} {
			name := fmt.Sprintf("%d-cross=%t", status, crossOrigin)
			t.Run(name, func(t *testing.T) {
				var destinationCalls atomic.Int32
				destination := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
					destinationCalls.Add(1)
				}))
				defer destination.Close()
				var source *httptest.Server
				source = httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
					if request.URL.Path == "/sink" {
						destinationCalls.Add(1)
						return
					}
					location := "/sink"
					if crossOrigin {
						location = destination.URL + "/sink"
					}
					response.Header().Set("Location", location)
					response.WriteHeader(status)
				}))
				defer source.Close()
				client, err := NewOnboardingClient(source.URL, &http.Client{Timeout: time.Second})
				if err != nil {
					t.Fatal(err)
				}
				if _, err := client.IssueDeviceInvite(context.Background(), ControlCapability{
					origin: client.Origin(),
					value:  ControlCredential{ActorID: 9, OrbitID: 7, Role: "primary", ControlToken: strings.Repeat("cd", 32), Context: ControlContextActive},
				}, "companion"); err == nil {
					t.Fatal("redirect accepted")
				}
				if destinationCalls.Load() != 0 {
					t.Fatal("redirect destination received bearer or request body")
				}
			})
		}
	}
}

func TestOnboardingHTTPRejectsMismatchedFinalEndpoint(t *testing.T) {
	client := newHTTPTestClient(t, func(request *http.Request) (*http.Response, error) {
		response := jsonResponse(http.StatusCreated, testCreateResponse())
		wrong, _ := url.Parse("https://coord.example/v1/telegram-links")
		response.Request = &http.Request{Method: request.Method, URL: wrong}
		return response, nil
	})
	if _, err := client.CreateOrbit(context.Background(), "Home", "attempt_0123456789"); err == nil {
		t.Fatal("mismatched final endpoint accepted")
	}
}

func assertRequest(t *testing.T, request *http.Request, method, path, bearer string) {
	t.Helper()
	if request.Method != method || request.URL.Path != path || request.URL.RawQuery != "" || request.URL.Fragment != "" {
		t.Fatalf("request %s %#v, want %s %s", request.Method, request.URL, method, path)
	}
	if got := request.Header.Get("Authorization"); got != func() string {
		if bearer == "" {
			return ""
		}
		return "Bearer " + bearer
	}() {
		t.Fatalf("authorization %q", got)
	}
	if method != http.MethodGet && request.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("content type %q", request.Header.Get("Content-Type"))
	}
}

func assertExactRequestObject(t *testing.T, request *http.Request, want map[string]any) {
	t.Helper()
	raw, err := io.ReadAll(request.Body)
	if err != nil {
		t.Fatal(err)
	}
	object, err := parseStrictJSONObject(raw)
	if err != nil || len(object) != len(want) {
		t.Fatalf("body %q: %v", raw, err)
	}
	for key, value := range want {
		got := object[key]
		if number, ok := got.(json.Number); ok {
			got = number.String()
		}
		if fmt.Sprint(got) != fmt.Sprint(value) {
			t.Fatalf("body field %s=%v, want %v", key, got, value)
		}
	}
}

var _ = time.Second
