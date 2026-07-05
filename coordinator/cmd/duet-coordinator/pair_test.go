// Pairing flow end-to-end (design §4): bot-issued code -> POST /pair ->
// credentials -> hub token lookup. This is Katya's two-tap onboarding.
package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"relux.works/duet/coordinator/internal/config"
	"relux.works/duet/coordinator/internal/store"
)

func TestPairingFlow(t *testing.T) {
	cfg := testConfig(t)
	cfg.PublicURL = "https://barycenter.relux.works"
	st, err := store.Open(filepath.Join(t.TempDir(), "pair.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()

	orbit, err := st.CreateOrbit("Тестовый", 111)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewServer(pairHandler(slog.Default(), st, cfg))
	defer srv.Close()

	pair := func(code string) (*http.Response, map[string]any) {
		t.Helper()
		resp, err := http.Post(srv.URL, "application/json", strings.NewReader(`{"code":"`+code+`"}`))
		if err != nil {
			t.Fatal(err)
		}
		var body map[string]any
		json.NewDecoder(resp.Body).Decode(&body)
		resp.Body.Close()
		return resp, body
	}

	// Happy path: code -> slot a credentials, wss URL from PublicURL.
	code, err := st.NewPairCode(orbit.ID, 111)
	if err != nil {
		t.Fatal(err)
	}
	resp, body := pair(code)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("pair: http %d", resp.StatusCode)
	}
	if body["slot"] != "a" || body["ws_url"] != "wss://barycenter.relux.works/ws" {
		t.Fatalf("body = %v", body)
	}
	token, _ := body["token"].(string)
	if len(token) != 64 {
		t.Fatalf("token %q", token)
	}

	// The minted token authenticates as (orbit, a) — what the hub will do.
	orbitID, slot, ok, _ := st.LookupToken(token)
	if !ok || orbitID != orbit.ID || slot != "a" {
		t.Fatalf("lookup: %v %v %v", orbitID, slot, ok)
	}

	// Codes are one-time.
	if resp, _ := pair(code); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("reuse must 403, got %d", resp.StatusCode)
	}

	// Second code -> slot b for the partner's home.
	code2, _ := st.NewPairCode(orbit.ID, 111)
	if _, body := pair(code2); body["slot"] != "b" {
		t.Fatalf("second slot = %v", body["slot"])
	}

	// Garbage in -> 403, no side effects.
	if resp, _ := pair("WRONGCOD"); resp.StatusCode != http.StatusForbidden {
		t.Fatalf("bad code must 403, got %d", resp.StatusCode)
	}
}

func TestPairRejectsNonPost(t *testing.T) {
	st, _ := store.Open(filepath.Join(t.TempDir(), "p.db"))
	defer st.Close()
	srv := httptest.NewServer(pairHandler(slog.Default(), st, &config.Config{Listen: "127.0.0.1:0"}))
	defer srv.Close()
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET must 405, got %d", resp.StatusCode)
	}
}

// Coordinator policy defends only a BUSY broadcast; an empty air always
// yields to the phone (customer R0 finding).
func TestCoordinatorPolicyYieldsWhenIdle(t *testing.T) {
	l, fake := newTestLoop(t)
	o := l.orbit(1)
	o.takeoverPolicy = "coordinator"
	l.handleExternalPlayback(o, "a", "spotify:track:X")
	for _, m := range fake.ofType("stop") {
		if m.node == "a" {
			t.Fatalf("empty air must not stop the phone's node: %+v", fake.sent)
		}
	}
	if o.sess.Mode != "solo" {
		t.Fatalf("empty air yields to apoastron, mode=%s", o.sess.Mode)
	}
}
