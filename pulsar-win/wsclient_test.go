// WS client against a real gorilla upgrader (the coordinator's library):
// register handshake, command dispatch, heartbeat, clock sync, reconnect.
package main

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"

	protocol "relux.works/duet/pulsar-win/wire"
)

var testUpgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(testWriter{}, &slog.HandlerOptions{Level: slog.LevelError}))
}

type testWriter struct{}

func (testWriter) Write(p []byte) (int, error) { return len(p), nil }

func readEnvelope(t *testing.T, ws *websocket.Conn) protocol.Envelope {
	t.Helper()
	ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	_, raw, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("server read: %v", err)
	}
	var env protocol.Envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		t.Fatalf("server decode: %v", err)
	}
	return env
}

func sendEnvelope(t *testing.T, ws *websocket.Conn, msgType string, payload any) {
	t.Helper()
	env, err := protocol.NewEnvelope(newMessageID(time.Now()), nowMS(), msgType, payload)
	if err != nil {
		t.Fatal(err)
	}
	if err := ws.WriteJSON(env); err != nil {
		t.Fatalf("server write: %v", err)
	}
}

func wsURL(srv *httptest.Server) string {
	return "ws" + strings.TrimPrefix(srv.URL, "http")
}

func newTestClient(url string) *WSClient {
	c := NewWSClient(url, Identity{
		NodeID: "a", Token: strings.Repeat("aa", 32), AppVersion: "0.0.0-test",
		LibrespotVersion: func() string { return "9.9.9" },
		Capabilities: []string{
			protocol.CapabilitySeamlessAdoption,
			protocol.CapabilityMediaClip,
			protocol.CapabilityMediaClip,
		},
	}, testLogger())
	c.PingInterval = 30 * time.Millisecond
	c.HeartbeatInterval = 25 * time.Millisecond
	c.MinBackoff = 10 * time.Millisecond
	c.MaxBackoff = 50 * time.Millisecond
	return c
}

func newLiveTestClient(url string) *WSClient {
	c := NewWSClient(url, Identity{
		NodeID: "a", Token: strings.Repeat("aa", 32), AppVersion: "0.0.0-test",
		LibrespotVersion: func() string { return "9.9.9" },
		Capabilities: []string{
			protocol.CapabilityLivePTT,
			protocol.CapabilitySeamlessAdoption,
		},
	}, testLogger())
	c.PingInterval = time.Hour
	c.HeartbeatInterval = time.Hour
	c.MinBackoff = 10 * time.Millisecond
	c.MaxBackoff = 50 * time.Millisecond
	return c
}

func wsLiveFrame(sequence uint32) protocol.LivePTTBinaryFrame {
	return protocol.LivePTTBinaryFrame{
		Flags:     protocol.LivePTTFlagFEC | protocol.LivePTTFlagStart,
		SessionID: [16]byte{0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15},
		Sequence:  sequence, CaptureMonotonicUS: 2_000_000,
		Payload: []byte{0xf8, 0xff, byte(sequence)},
	}
}

func wsLiveStart() protocol.LivePTTStartPayload {
	return protocol.LivePTTStartPayload{
		SessionID: "00112233445566778899aabbccddeeff", Generation: 7,
		SenderActorID: 1, SenderOrbitID: 2, SenderNodeID: "win-a",
		TargetSnapshot: "lts1.ws", TargetSHA256: strings.Repeat("a", 64), TargetCount: 1,
		PlaybackDomain: "personal", PlaybackDomainID: 1,
		CodecProfile: protocol.LivePTTCodecProfile, FrameMS: protocol.LivePTTFrameMS,
		MaxPayloadBytes: protocol.LivePTTMaxPayloadBytes, JitterBufferMS: protocol.LivePTTJitterBufferMS,
		StartedAtCoordMS: 1_000, AcceptDeadlineCoordMS: 2_500,
		MaxDurationMS: protocol.LivePTTMaxDurationMS, MixedVersionPolicy: protocol.LivePTTMixedVersionRequireAll,
		LateJoinPolicy: protocol.LivePTTLateJoinPolicy, CaptureAuthority: protocol.LivePTTCaptureAuthority,
	}
}

func TestRegisterHandshakeAndCommandDispatch(t *testing.T) {
	conns := make(chan *websocket.Conn, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conns <- ws
	}))
	defer srv.Close()

	client := newTestClient(wsURL(srv))
	received := make(chan any, 16)
	client.OnMessage = func(env protocol.Envelope, payload any) { received <- payload }
	client.Start()
	defer client.Stop()

	ws := <-conns
	defer ws.Close()

	// First message must be register with the node identity (spec 8.2).
	env := readEnvelope(t, ws)
	if env.Type != protocol.TypeRegister {
		t.Fatalf("first message %q, want register", env.Type)
	}
	if env.V != protocol.Version || !strings.HasPrefix(env.ID, "msg_") || len(env.ID) != 30 {
		t.Fatalf("envelope head off: v=%d id=%q", env.V, env.ID)
	}
	var reg protocol.RegisterPayload
	if err := json.Unmarshal(env.Payload, &reg); err != nil {
		t.Fatal(err)
	}
	if reg.NodeID != "a" || reg.Token != strings.Repeat("aa", 32) ||
		reg.AppVersion != "0.0.0-test" || reg.LibrespotVersion != "9.9.9" {
		t.Fatalf("register payload %+v", reg)
	}
	if !reflect.DeepEqual(reg.Capabilities, []string{
		protocol.CapabilityMediaClip, protocol.CapabilitySeamlessAdoption,
	}) {
		t.Fatalf("register capabilities are not exact and canonical: %v", reg.Capabilities)
	}
	if _, err := protocol.ParseCapabilitySet(reg.Capabilities); err != nil {
		t.Fatalf("client emitted non-canonical capabilities: %v", err)
	}

	// A command envelope must reach the handler as its typed payload.
	sendEnvelope(t, ws, protocol.TypeLoad, &protocol.LoadPayload{
		ElementID: "el_1", URI: "spotify:track:x", PositionMS: 500,
	})
	select {
	case got := <-received:
		load, ok := got.(*protocol.LoadPayload)
		if !ok {
			t.Fatalf("payload type %T", got)
		}
		if load.URI != "spotify:track:x" || load.PositionMS != 500 {
			t.Fatalf("load payload %+v", load)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("command never dispatched")
	}
}

func TestHeartbeatPingAndClockSync(t *testing.T) {
	conns := make(chan *websocket.Conn, 4)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conns <- ws
	}))
	defer srv.Close()

	client := newTestClient(wsURL(srv))
	var stateCalls atomic.Int64
	client.StateProvider = func() protocol.StatePayload {
		stateCalls.Add(1)
		return protocol.StatePayload{Playback: "stopped", Volume: 80, Speakers: []protocol.Speaker{}}
	}
	client.Start()
	defer client.Stop()

	ws := <-conns
	defer ws.Close()
	if env := readEnvelope(t, ws); env.Type != protocol.TypeRegister {
		t.Fatalf("first message %q", env.Type)
	}

	sawState := false
	sawPing := false
	deadline := time.Now().Add(5 * time.Second)
	for (!sawState || !sawPing) && time.Now().Before(deadline) {
		env := readEnvelope(t, ws)
		switch env.Type {
		case protocol.TypeState:
			var st protocol.StatePayload
			json.Unmarshal(env.Payload, &st)
			if st.Playback != "stopped" || st.Volume != 80 {
				t.Fatalf("state payload %+v", st)
			}
			sawState = true
		case protocol.TypePing:
			var ping protocol.PingPayload
			json.Unmarshal(env.Payload, &ping)
			// Answer like the hub does (inline t2/t3): node 250 ms ahead,
			// one-way ~0 -> offset must land near 250.
			sendEnvelope(t, ws, protocol.TypePong, &protocol.PongPayload{
				T1: ping.T1, T2: ping.T1 - 250, T3: ping.T1 - 250,
			})
			sawPing = true
		}
	}
	if !sawState || !sawPing {
		t.Fatalf("heartbeat traffic incomplete: state=%v ping=%v", sawState, sawPing)
	}
	if stateCalls.Load() == 0 {
		t.Fatal("StateProvider never consulted")
	}

	// The pong must have fed the clock (offset ~250, rtt tiny).
	waitFor(t, 5*time.Second, func() bool {
		off, ok := client.Clock().OffsetMS()
		return ok && off > 200 && off < 300
	}, "clock offset never converged near 250")
}

func TestReconnectWithBackoff(t *testing.T) {
	var dials atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		n := dials.Add(1)
		if n == 1 {
			ws.Close() // first connection dies immediately -> client must retry
			return
		}
		// Second connection stays up long enough to prove the reconnect.
		ws.SetReadDeadline(time.Now().Add(5 * time.Second))
		ws.ReadMessage()
		ws.Close()
	}))
	defer srv.Close()

	client := newTestClient(wsURL(srv))
	var disconnects atomic.Int64
	client.OnDisconnected = func() { disconnects.Add(1) }
	client.Start()
	defer client.Stop()

	waitFor(t, 5*time.Second, func() bool { return dials.Load() >= 2 },
		"client never reconnected after a dropped connection")
	waitFor(t, 5*time.Second, func() bool { return disconnects.Load() >= 1 },
		"disconnect lifecycle callback was not delivered")
}

func TestReconnectOnProtocolVersionMismatch(t *testing.T) {
	var dials atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		n := dials.Add(1)
		defer ws.Close()
		ws.SetReadDeadline(time.Now().Add(5 * time.Second))
		_, raw, readErr := ws.ReadMessage()
		if readErr != nil {
			t.Errorf("connection %d register read: %v", n, readErr)
			return
		}
		var register protocol.Envelope
		if decodeErr := json.Unmarshal(raw, &register); decodeErr != nil {
			t.Errorf("connection %d register decode: %v", n, decodeErr)
			return
		}
		if register.Type != protocol.TypeRegister {
			t.Errorf("connection %d first message=%q", n, register.Type)
			return
		}
		if n == 1 {
			env, envelopeErr := protocol.NewEnvelope(
				newMessageID(time.Now()), nowMS(), protocol.TypePong,
				&protocol.PongPayload{T1: 1, T2: 1, T3: 1})
			if envelopeErr != nil {
				t.Error(envelopeErr)
				return
			}
			env.V = protocol.Version + 1
			if writeErr := ws.WriteJSON(env); writeErr != nil {
				t.Error(writeErr)
			}
			ws.SetReadDeadline(time.Now().Add(5 * time.Second))
			_, _, _ = ws.ReadMessage()
			return
		}
		time.Sleep(50 * time.Millisecond)
	}))
	defer srv.Close()

	client := newTestClient(wsURL(srv))
	client.Start()
	defer client.Stop()
	waitFor(t, 5*time.Second, func() bool { return dials.Load() >= 2 },
		"client did not reconnect after a major-version mismatch")
}

func TestLivePTTBinaryTransportIsCapabilityGatedBoundedAndValidated(t *testing.T) {
	conns := make(chan *websocket.Conn, 1)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ws, err := testUpgrader.Upgrade(w, r, nil)
		if err == nil {
			conns <- ws
		}
	}))
	defer srv.Close()

	client := newLiveTestClient(wsURL(srv))
	received := make(chan protocol.LivePTTBinaryFrame, 1)
	client.OnLivePTTFrame = func(frame protocol.LivePTTBinaryFrame) { received <- frame }
	client.Start()
	defer client.Stop()
	ws := <-conns
	defer ws.Close()
	if env := readEnvelope(t, ws); env.Type != protocol.TypeRegister {
		t.Fatalf("first message %q", env.Type)
	}
	client.MarkHealthy()

	if !client.TrySendLivePTTControl(protocol.TypeLivePTTStart, wsLiveStart()) {
		t.Fatal("ready live start was not queued")
	}
	outbound := wsLiveFrame(1)
	if !client.TrySendLivePTTFrame(outbound) {
		t.Fatal("ready live frame was not queued")
	}
	end := protocol.LivePTTEndPayload{
		SessionID: wsLiveStart().SessionID, Generation: 7, CommandSequence: 1,
		LastSequence: 1, EndedAtCoordMS: 1_100, DrainDeadlineCoordMS: 1_700,
		Reason: "release",
	}
	if !client.TrySendLivePTTControl(protocol.TypeLivePTTEnd, end) {
		t.Fatal("ready live end was not queued")
	}
	if env := readEnvelope(t, ws); env.Type != protocol.TypeLivePTTStart {
		t.Fatalf("live FIFO first message %q, want start", env.Type)
	}
	ws.SetReadDeadline(time.Now().Add(5 * time.Second))
	kind, raw, err := ws.ReadMessage()
	if err != nil || kind != websocket.BinaryMessage {
		t.Fatalf("binary read kind=%d err=%v", kind, err)
	}
	decoded, err := protocol.DecodeLivePTTBinaryFrame(raw)
	if err != nil || !reflect.DeepEqual(decoded, outbound) {
		t.Fatalf("binary round trip frame=%+v err=%v", decoded, err)
	}
	if env := readEnvelope(t, ws); env.Type != protocol.TypeLivePTTEnd {
		t.Fatalf("live FIFO terminal %q overtook or missed the frame", env.Type)
	}

	malformed := append([]byte(nil), raw...)
	malformed[2] = 9
	if err := ws.WriteMessage(websocket.BinaryMessage, malformed); err != nil {
		t.Fatal(err)
	}
	if err := ws.WriteMessage(websocket.BinaryMessage, raw); err != nil {
		t.Fatal(err)
	}
	select {
	case frame := <-received:
		if !reflect.DeepEqual(frame, outbound) {
			t.Fatalf("received frame %+v", frame)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("validated live binary was not dispatched")
	}
}

func TestLivePTTBinaryQueueFailsClosedAtEightAndWithoutCapability(t *testing.T) {
	client := newLiveTestClient("ws://127.0.0.1:1/ws")
	client.mu.Lock()
	client.conn = &websocket.Conn{}
	client.healthy = true
	client.mu.Unlock()
	for index := 0; index < 8; index++ {
		if !client.TrySendLivePTTFrame(wsLiveFrame(1)) {
			t.Fatalf("bounded slot %d rejected", index)
		}
	}
	if client.TrySendLivePTTFrame(wsLiveFrame(1)) {
		t.Fatal("ninth queued/in-flight live frame was accepted")
	}
	for index := 0; index < 16; index++ {
		if !client.TrySendLivePTTControl(protocol.TypeLivePTTStart, wsLiveStart()) {
			t.Fatalf("bounded control slot %d rejected", index)
		}
	}
	if client.TrySendLivePTTControl(protocol.TypeLivePTTStart, wsLiveStart()) {
		t.Fatal("seventeenth live control was accepted")
	}
	client.mu.Lock()
	client.conn = nil
	client.mu.Unlock()
	client.Stop()

	dark := newTestClient("ws://127.0.0.1:1/ws")
	dark.mu.Lock()
	dark.conn = &websocket.Conn{}
	dark.healthy = true
	dark.mu.Unlock()
	if dark.TrySendLivePTTFrame(wsLiveFrame(1)) {
		t.Fatal("unadvertised production client accepted live binary")
	}
	if dark.TrySendLivePTTControl(protocol.TypeLivePTTStart, wsLiveStart()) {
		t.Fatal("unadvertised production client accepted live control")
	}
	dark.mu.Lock()
	dark.conn = nil
	dark.mu.Unlock()
	dark.Stop()
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal(msg)
}
