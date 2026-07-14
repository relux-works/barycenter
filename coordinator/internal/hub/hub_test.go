package hub

import (
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	"relux.works/duet/coordinator/internal/protocol"
)

func websocketTestURL(raw string) string {
	return "ws" + strings.TrimPrefix(raw, "http")
}

func registerWire(capabilities any) map[string]any {
	return map[string]any{
		"v": 1, "id": "msg_test", "ts": int64(1), "type": protocol.TypeRegister,
		"payload": map[string]any{
			"node_id": "a", "token": "valid", "app_version": "test",
			"librespot_version": "test", "capabilities": capabilities,
		},
	}
}

func testHub(offlineAfter time.Duration) *Hub {
	return New(slog.Default(), func(string) (int64, string, bool) { return 0, "", false }, offlineAfter)
}

func fillEvents(h *Hub) {
	for i := 0; i < cap(h.Events); i++ {
		h.Events <- EvRegistered{}
	}
}

// A node past its deadline must ALWAYS produce an EvOffline, even when the
// event channel is momentarily saturated. The old lossy emit dropped it here
// and stranded a dead node "online" in the FSM forever (bugs #3).
func TestSweepOfflineEmitsReliably(t *testing.T) {
	h := testHub(10 * time.Millisecond)
	key := NodeKey{Orbit: 1, Slot: "a"}
	h.online[key] = true
	h.lastSeen[key] = time.Now().Add(-time.Second)

	fillEvents(h) // saturate: emit must block, not drop

	stop := make(chan struct{})
	done := make(chan struct{})
	go func() { h.sweepOffline(time.Now(), stop); close(done) }()

	// Drain the backlog until the EvOffline surfaces.
	deadline := time.After(2 * time.Second)
	got := false
	for !got {
		select {
		case ev := <-h.Events:
			if off, ok := ev.(EvOffline); ok && off.Key == key {
				got = true
			}
		case <-deadline:
			t.Fatal("EvOffline was dropped / never delivered")
		}
	}
	<-done

	h.mu.Lock()
	still := h.online[key]
	h.mu.Unlock()
	if still {
		t.Fatal("node still marked online after sweep")
	}
}

// emit must never wedge forever on a full channel: the escape (hub shutdown or
// connection close) releases it.
func TestEmitReleasesOnEscape(t *testing.T) {
	h := testHub(time.Second)
	fillEvents(h)

	escape := make(chan struct{})
	done := make(chan struct{})
	go func() { h.emit(EvOffline{}, escape); close(done) }()
	close(escape)

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("emit did not release on escape")
	}
}

func TestRegisterRejectsNonCanonicalCapabilities(t *testing.T) {
	for _, tc := range []struct {
		name         string
		capabilities any
	}{
		{"duplicate", []any{"media_clip_v1", "media_clip_v1"}},
		{"unsorted", []any{"overlay_mix_v1", "media_clip_v1"}},
		{"non-string", []any{"media_clip_v1", 7}},
		{"non-ascii", []any{"média_clip_v1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := New(slog.Default(), func(string) (int64, string, bool) {
				return 1, "a", true
			}, time.Second)
			server := httptest.NewServer(http.HandlerFunc(h.HandleWS))
			defer server.Close()

			conn, _, err := websocket.DefaultDialer.Dial(websocketTestURL(server.URL), nil)
			if err != nil {
				t.Fatal(err)
			}
			defer conn.Close()
			if err := conn.WriteJSON(registerWire(tc.capabilities)); err != nil {
				t.Fatal(err)
			}
			_, _, err = conn.ReadMessage()
			var closeErr *websocket.CloseError
			if !errors.As(err, &closeErr) || closeErr.Code != closeInvalidAuth {
				t.Fatalf("invalid capabilities close=%v", err)
			}
		})
	}
}

func TestRegisterRetainsUnknownCapabilitiesInCanonicalOrder(t *testing.T) {
	h := New(slog.Default(), func(token string) (int64, string, bool) {
		return 42, "b", token == "valid"
	}, time.Second)
	server := httptest.NewServer(http.HandlerFunc(h.HandleWS))
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial(websocketTestURL(server.URL), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	capabilities := []any{
		protocol.CapabilityInterruptResume,
		protocol.CapabilityMediaClip,
		"unknown_future_v2",
	}
	if err := conn.WriteJSON(registerWire(capabilities)); err != nil {
		t.Fatal(err)
	}

	select {
	case event := <-h.Events:
		registered, ok := event.(EvRegistered)
		if !ok {
			t.Fatalf("first event=%T, want EvRegistered", event)
		}
		if registered.Key != (NodeKey{Orbit: 42, Slot: "b"}) ||
			!registered.Capabilities.Supports(protocol.CapabilityMediaClip) ||
			!registered.Capabilities.Supports("unknown_future_v2") {
			t.Fatalf("registered event=%+v capabilities=%v", registered, registered.Capabilities.Values())
		}
	case <-time.After(time.Second):
		t.Fatal("valid registration did not emit")
	}
	key := NodeKey{Orbit: 42, Slot: "b"}
	snapshot := h.NodeSnapshots()[key]
	if !snapshot.Connected || snapshot.LastSeenAt <= 0 ||
		snapshot.CredentialTokenHash == "" ||
		!snapshot.Capabilities.Supports(protocol.CapabilityMediaClip) ||
		!snapshot.Capabilities.Supports("unknown_future_v2") {
		t.Fatalf("node snapshot=%+v capabilities=%v", snapshot, snapshot.Capabilities.Values())
	}
}
