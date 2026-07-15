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

func TestPresencePlaybackStateIsClosedAndSanitized(t *testing.T) {
	cases := map[string]string{
		"stopped": "idle", "paused": "idle", "wait": "idle",
		"loading": "main", "playing": "main", "voice": "interrupt",
		"microphone_capture_process": "unknown", "": "unknown",
	}
	for input, expected := range cases {
		if got := presencePlaybackState(input); got != expected {
			t.Fatalf("playback %q=%q want %q", input, got, expected)
		}
	}
}

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

func TestRegisterRejectsUnsupportedProtocolVersionBeforeAuthentication(t *testing.T) {
	h := New(slog.Default(), func(string) (int64, string, bool) {
		t.Fatal("mixed-version register reached credential lookup")
		return 0, "", false
	}, time.Second)
	server := httptest.NewServer(http.HandlerFunc(h.HandleWS))
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial(websocketTestURL(server.URL), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	frame := registerWire([]any{})
	frame["v"] = protocol.Version + 1
	if err := conn.WriteJSON(frame); err != nil {
		t.Fatal(err)
	}
	_, _, err = conn.ReadMessage()
	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) || closeErr.Code != closeInvalidAuth {
		t.Fatalf("mixed-version register close=%v", err)
	}
}

func TestEstablishedConnectionClosesOnProtocolVersionMismatch(t *testing.T) {
	h := New(slog.Default(), func(token string) (int64, string, bool) {
		return 42, "a", token == "valid"
	}, time.Second)
	server := httptest.NewServer(http.HandlerFunc(h.HandleWS))
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial(websocketTestURL(server.URL), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.WriteJSON(registerWire([]any{})); err != nil {
		t.Fatal(err)
	}
	select {
	case <-h.Events:
	case <-time.After(time.Second):
		t.Fatal("valid registration did not complete")
	}
	if err := conn.WriteJSON(map[string]any{
		"v": protocol.Version + 1, "id": "msg_test", "ts": int64(2),
		"type": protocol.TypePing, "payload": map[string]any{"t1": int64(1)},
	}); err != nil {
		t.Fatal(err)
	}
	conn.SetReadDeadline(time.Now().Add(time.Second))
	if _, _, err := conn.ReadMessage(); err == nil {
		t.Fatal("mixed-version frame left the authenticated socket open")
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

func TestDisconnectClosesRevokedLiveGenerationImmediately(t *testing.T) {
	h := New(slog.Default(), func(token string) (int64, string, bool) {
		return 42, "a", token == "valid"
	}, time.Second)
	server := httptest.NewServer(http.HandlerFunc(h.HandleWS))
	defer server.Close()

	conn, _, err := websocket.DefaultDialer.Dial(websocketTestURL(server.URL), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.WriteJSON(registerWire([]any{})); err != nil {
		t.Fatal(err)
	}
	key := NodeKey{Orbit: 42, Slot: "a"}
	deadline := time.Now().Add(time.Second)
	for !h.NodeSnapshots()[key].Connected && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !h.Disconnect(key) {
		t.Fatal("live node was not disconnected")
	}
	_, _, err = conn.ReadMessage()
	var closeErr *websocket.CloseError
	if !errors.As(err, &closeErr) || closeErr.Code != closeRevokedAuth {
		t.Fatalf("revocation close=%v", err)
	}
	if h.Disconnect(key) {
		t.Fatal("repeat disconnect reported a live generation")
	}
}

func TestRTTSampleBelongsOnlyToCurrentAuthenticatedSocket(t *testing.T) {
	h := New(slog.Default(), func(token string) (int64, string, bool) {
		return 7, "a", token == "valid"
	}, time.Second)
	server := httptest.NewServer(http.HandlerFunc(h.HandleWS))
	defer server.Close()

	connect := func() *websocket.Conn {
		t.Helper()
		conn, _, err := websocket.DefaultDialer.Dial(websocketTestURL(server.URL), nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := conn.WriteJSON(registerWire([]any{protocol.CapabilityMediaClip})); err != nil {
			conn.Close()
			t.Fatal(err)
		}
		return conn
	}
	waitEvent := func(match func(Event) bool) {
		t.Helper()
		deadline := time.After(2 * time.Second)
		for {
			select {
			case event := <-h.Events:
				if match(event) {
					return
				}
			case <-deadline:
				t.Fatal("timed out waiting for hub event")
			}
		}
	}

	first := connect()
	defer first.Close()
	waitEvent(func(event Event) bool {
		_, ok := event.(EvRegistered)
		return ok
	})
	state := map[string]any{
		"v": 1, "id": "msg_state", "ts": time.Now().UnixMilli(),
		"type": protocol.TypeState,
		"payload": map[string]any{
			"playback": "playing", "position_ms": 10, "volume": 80,
			"degraded": false, "underruns": 0, "rtt_ms": 87,
			"speakers": []any{},
		},
	}
	if err := first.WriteJSON(state); err != nil {
		t.Fatal(err)
	}
	var received EvMessage
	waitEvent(func(event Event) bool {
		message, ok := event.(EvMessage)
		if ok && message.Env.Type == protocol.TypeState {
			received = message
			return true
		}
		return false
	})
	key := NodeKey{Orbit: 7, Slot: "a"}
	before := h.NodeSnapshots()[key]
	if before.RTTMS != 87 || before.RTTSampledAt <= 0 || !before.Connected ||
		before.PlaybackState != "main" || before.OutputDegraded ||
		received.CredentialTokenHash == "" ||
		received.CredentialTokenHash != before.CredentialTokenHash {
		t.Fatalf("current RTT snapshot=%+v", before)
	}

	second := connect()
	defer second.Close()
	waitEvent(func(event Event) bool {
		_, ok := event.(EvRegistered)
		return ok
	})
	after := h.NodeSnapshots()[key]
	if !after.Connected || after.RTTMS != 0 || after.RTTSampledAt != 0 ||
		after.PlaybackState != "" || after.OutputDegraded ||
		after.CredentialTokenHash == "" {
		t.Fatalf("reconnect retained predecessor RTT=%+v", after)
	}
}
