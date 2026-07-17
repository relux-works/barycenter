package hub

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"relux.works/duet/coordinator/internal/protocol"
)

func TestHubValidatesInboundAndPreservesOrderedBinaryFrames(t *testing.T) {
	var logs lockedLogBuffer
	h := New(slog.New(slog.NewJSONHandler(&logs, nil)), func(token string) (int64, string, bool) { return 42, "a", token == "valid" }, time.Second)
	server := httptest.NewServer(http.HandlerFunc(h.HandleWS))
	defer server.Close()
	conn, _, err := websocket.DefaultDialer.Dial(websocketTestURL(server.URL), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if err := conn.WriteJSON(registerWire([]any{protocol.LivePTTCapability})); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		select {
		case <-h.Events:
		case <-time.After(time.Second):
			t.Fatal("registration event timeout")
		}
	}
	id, _ := protocol.ParseLivePTTSessionID("00112233445566778899aabbccddeeff")
	frame := protocol.LivePTTBinaryFrame{Flags: protocol.LivePTTFlagStart | protocol.LivePTTFlagFEC, SessionID: id, Sequence: 1, CaptureMonotonicUS: 1000000, Payload: []byte{0xf8, 0xff, 0xfe}}
	raw, err := protocol.EncodeLivePTTBinaryFrame(frame)
	if err != nil {
		t.Fatal(err)
	}
	if err := conn.WriteMessage(websocket.BinaryMessage, raw); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-h.Events:
		binary, ok := event.(EvBinary)
		if !ok || binary.Frame.Sequence != 1 || string(binary.Raw) != string(raw) {
			t.Fatalf("binary event=%+v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("binary event timeout")
	}
	key := NodeKey{Orbit: 42, Slot: "a"}
	if !h.TrySendBinary(key, raw) {
		t.Fatal("bounded binary enqueue failed")
	}
	messageType, received, err := conn.ReadMessage()
	if err != nil || messageType != websocket.BinaryMessage || string(received) != string(raw) {
		t.Fatalf("outbound type=%d bytes=%x err=%v", messageType, received, err)
	}
	bad := append([]byte(nil), raw...)
	bad[34] = 10
	if err := conn.WriteMessage(websocket.BinaryMessage, bad); err != nil {
		t.Fatal(err)
	}
	select {
	case event := <-h.Events:
		if _, ok := event.(EvBinary); ok {
			t.Fatal("wrong-profile frame emitted")
		}
	case <-time.After(50 * time.Millisecond):
	}
	logged := logs.String()
	for _, forbidden := range []string{`"orbit"`, `"slot"`, `"err"`, "00112233445566778899aabbccddeeff"} {
		if strings.Contains(logged, forbidden) {
			t.Fatalf("rejected live frame log leaked private or arbitrary data %q: %s", forbidden, logged)
		}
	}
	if !strings.Contains(logged, `"reason":"invalid_frame"`) {
		t.Fatalf("rejected live frame lost bounded reason: %s", logged)
	}
}

func TestTrySendBinaryFailsFastOnPerConnectionBackpressure(t *testing.T) {
	h := testHub(time.Second)
	key := NodeKey{Orbit: 1, Slot: "a"}
	c := &conn{send: make(chan outboundFrame, 1), stop: make(chan struct{})}
	c.send <- outboundFrame{binary: []byte{1}}
	h.conns[key] = c
	id, _ := protocol.ParseLivePTTSessionID("00112233445566778899aabbccddeeff")
	raw, err := protocol.EncodeLivePTTBinaryFrame(protocol.LivePTTBinaryFrame{Flags: protocol.LivePTTFlagStart | protocol.LivePTTFlagFEC, SessionID: id, Sequence: 1, CaptureMonotonicUS: 1, Payload: []byte{1}})
	if err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	if h.TrySendBinary(key, raw) {
		t.Fatal("saturated queue accepted frame")
	}
	if time.Since(started) > 10*time.Millisecond {
		t.Fatal("backpressure path blocked")
	}
}
