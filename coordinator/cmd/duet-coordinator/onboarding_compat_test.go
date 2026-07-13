package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"relux.works/duet/coordinator/internal/config"
	"relux.works/duet/coordinator/internal/hub"
	"relux.works/duet/coordinator/internal/protocol"
	"relux.works/duet/coordinator/internal/store"
)

func TestSelfServiceFlagPreservesLegacyPairAndWebSocketRegistration(t *testing.T) {
	st, err := store.OpenWithOptions(filepath.Join(t.TempDir(), "compat.db"), store.Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	orbit, err := st.CreateOrbit("Legacy", 111)
	if err != nil {
		t.Fatal(err)
	}
	code, err := st.NewPairCode(orbit.ID, 111)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/pair", strings.NewReader(`{"code":`+strconvQuote(code)+`}`))
	recorder := httptest.NewRecorder()
	pairHandler(slog.Default(), st, &config.Config{PublicURL: "https://coord.example"})(recorder, request)
	if recorder.Code != http.StatusOK {
		t.Fatalf("legacy pair status=%d response_bytes=%d", recorder.Code, recorder.Body.Len())
	}
	var paired struct {
		OrbitID int64  `json:"orbit_id"`
		Slot    string `json:"slot"`
		Token   string `json:"token"`
	}
	if err := json.Unmarshal(recorder.Body.Bytes(), &paired); err != nil {
		t.Fatal(err)
	}
	if paired.OrbitID != orbit.ID || paired.Slot != "a" || len(paired.Token) != 64 {
		t.Fatalf("legacy pair metadata mismatch: orbit=%d slot=%q token_length=%d", paired.OrbitID, paired.Slot, len(paired.Token))
	}

	lookup := func(token string) (int64, string, bool) {
		orbitID, slot, ok, err := st.LookupPlaybackToken(token)
		if err != nil {
			t.Errorf("lookup: %v", err)
			return 0, "", false
		}
		return orbitID, slot, ok
	}
	h := hub.New(slog.Default(), lookup, time.Minute)
	server := httptest.NewServer(http.HandlerFunc(h.HandleWS))
	defer server.Close()
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	connection, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer connection.Close()
	register := map[string]any{
		"v": 1, "id": "01HLEGACYREGISTRATION0000000", "ts": time.Now().UnixMilli(), "type": protocol.TypeRegister,
		"payload": map[string]any{"node_id": paired.Slot, "token": paired.Token, "app_version": "test", "librespot_version": "test"},
	}
	if err := connection.WriteJSON(register); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-h.Events:
		registered, ok := event.(hub.EvRegistered)
		if !ok || registered.Key.Orbit != orbit.ID || string(registered.Key.Slot) != paired.Slot {
			t.Fatalf("registration event=%#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("legacy-paired node did not register over websocket")
	}
}

func strconvQuote(value string) string {
	raw, _ := json.Marshal(value)
	return string(raw)
}
