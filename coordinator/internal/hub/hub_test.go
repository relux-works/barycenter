package hub

import (
	"log/slog"
	"testing"
	"time"
)

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
