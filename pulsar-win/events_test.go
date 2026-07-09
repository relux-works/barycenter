// Daemon /events client: parsing, reconnect-on-close and the ping watchdog
// that catches half-dead sockets.
package main

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gorilla/websocket"
)

func TestParseLibrespotEvent(t *testing.T) {
	ev, ok := parseLibrespotEvent([]byte(`{"type":"metadata","data":{"uri":"spotify:track:x","name":"Song","artist_names":["Artist"],"position":1500,"duration":180000}}`))
	if !ok || ev.Type != "metadata" {
		t.Fatalf("metadata parse failed: %+v ok=%v", ev, ok)
	}
	if *ev.URI != "spotify:track:x" || *ev.Name != "Song" || len(ev.ArtistNames) != 1 || *ev.Position != 1500 || *ev.Duration != 180000 {
		t.Fatalf("metadata fields wrong: %+v", ev)
	}

	ev, ok = parseLibrespotEvent([]byte(`{"type":"playing","data":{"uri":"spotify:track:y","play_origin":"playlist","context_uri":"spotify:album:z","resume":false}}`))
	if !ok || ev.Type != "playing" || *ev.URI != "spotify:track:y" || ev.Position != nil ||
		ev.PlayOrigin == nil || *ev.PlayOrigin != "playlist" || ev.ContextURI == nil || ev.Resume == nil {
		t.Fatalf("playing parse wrong: %+v", ev)
	}

	// Absent data object: all fields nil (paused/stopped arrive bare).
	ev, ok = parseLibrespotEvent([]byte(`{"type":"stopped"}`))
	if !ok || ev.Type != "stopped" || ev.URI != nil {
		t.Fatalf("bare event parse wrong: %+v", ev)
	}

	ev, ok = parseLibrespotEvent([]byte(`{"type":"volume","data":{"value":32768,"max":65536}}`))
	if !ok || *ev.Value != 32768 || *ev.Max != 65536 {
		t.Fatalf("volume parse wrong: %+v", ev)
	}

	ev, ok = parseLibrespotEvent([]byte(`{"type":"seek","data":{"position":42000,"duration":100000}}`))
	if !ok || *ev.Position != 42000 {
		t.Fatalf("seek parse wrong: %+v", ev)
	}

	// Unknown types still parse (the player ignores them) — forward compat.
	if ev, ok = parseLibrespotEvent([]byte(`{"type":"新しい","data":{}}`)); !ok || ev.Type != "新しい" {
		t.Fatalf("unknown type must still parse: %+v ok=%v", ev, ok)
	}

	// Not events: invalid JSON, missing type, extra fields tolerated.
	if _, ok = parseLibrespotEvent([]byte(`{nope`)); ok {
		t.Fatal("invalid JSON accepted")
	}
	if _, ok = parseLibrespotEvent([]byte(`{"data":{"uri":"x"}}`)); ok {
		t.Fatal("frame without type accepted")
	}
	if _, ok = parseLibrespotEvent([]byte(`{"type":"paused","data":{"uri":"x","unknown_field":1},"extra":2}`)); !ok {
		t.Fatal("unknown fields must be tolerated")
	}
}

// eventsURL points the events client at a test server ("/events" path; the
// shared testUpgrader/wsURL helpers live in wsclient_test.go).
func eventsURL(ts *httptest.Server) string {
	return wsURL(ts) + "/events"
}

func TestEventsClientDeliversAndReconnectsOnClose(t *testing.T) {
	var conns atomic.Int64
	events := make(chan LibrespotEvent, 16)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		n := conns.Add(1)
		conn.WriteMessage(websocket.TextMessage,
			[]byte(`{"type":"metadata","data":{"uri":"spotify:track:conn`+string(rune('0'+n))+`"}}`))
		if n == 1 {
			conn.Close() // die right after the first event: client must reconnect
			return
		}
		// Later connections stay up and echo pings (ReadMessage pumps the
		// control handlers, sending pongs automatically).
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				conn.Close()
				return
			}
		}
	}))
	defer ts.Close()

	c := newEventsClientURL(eventsURL(ts), func(ev LibrespotEvent) { events <- ev }, testLogger())
	c.ReconnectDelay = 10 * time.Millisecond
	c.PingInterval = 50 * time.Millisecond
	c.PongWait = 50 * time.Millisecond
	c.Start()
	defer c.Stop()

	first := expectEvent(t, events)
	if *first.URI != "spotify:track:conn1" {
		t.Fatalf("first event %+v", first)
	}
	second := expectEvent(t, events)
	if *second.URI != "spotify:track:conn2" {
		t.Fatalf("event after reconnect %+v", second)
	}
	if conns.Load() < 2 {
		t.Fatalf("connections %d, want >= 2 (reconnect after server close)", conns.Load())
	}
}

// TestEventsClientWatchdogRecoversHalfDeadSocket is the port of the mac R0
// fix: the server accepts and then goes silent WITHOUT a close frame and
// WITHOUT reading (so pings are never answered). A receive-only client would
// hang forever; the watchdog's read deadline must kill the socket and
// reconnect.
func TestEventsClientWatchdogRecoversHalfDeadSocket(t *testing.T) {
	var conns atomic.Int64
	hold := make(chan struct{})
	defer close(hold) // let handlers exit before ts.Close waits on them

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conns.Add(1)
		<-hold // half-dead: no reads (=> no pongs), no writes, no close frame
		conn.Close()
	}))
	defer ts.Close()

	c := newEventsClientURL(eventsURL(ts), nil, testLogger())
	c.ReconnectDelay = 10 * time.Millisecond
	c.PingInterval = 40 * time.Millisecond
	c.PongWait = 40 * time.Millisecond
	c.Start()
	defer c.Stop()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if conns.Load() >= 3 {
			return // the watchdog keeps forcing reconnects — exactly the fix
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("connections %d, want >= 3 (watchdog must reconnect half-dead sockets)", conns.Load())
}

func TestEventsClientStopEndsLoop(t *testing.T) {
	var conns atomic.Int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := testUpgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		conns.Add(1)
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				conn.Close()
				return
			}
		}
	}))
	defer ts.Close()

	c := newEventsClientURL(eventsURL(ts), nil, testLogger())
	c.ReconnectDelay = 5 * time.Millisecond
	c.Start()

	deadline := time.Now().Add(2 * time.Second)
	for conns.Load() == 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if conns.Load() == 0 {
		t.Fatal("client never connected")
	}

	c.Stop()
	base := conns.Load()
	time.Sleep(60 * time.Millisecond)
	if conns.Load() != base {
		t.Fatal("client reconnected after Stop")
	}
}

func expectEvent(t *testing.T, ch chan LibrespotEvent) LibrespotEvent {
	t.Helper()
	select {
	case ev := <-ch:
		return ev
	case <-time.After(5 * time.Second):
		t.Fatal("no event received")
		return LibrespotEvent{}
	}
}
