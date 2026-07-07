// Approaches (design §12 L1): linking two personal barycenters into one
// shared broadcast — the full bot flow driven through the real loop + FSM +
// SQLite store with the fake transport.
package main

import (
	"log/slog"
	"regexp"
	"strings"
	"testing"

	"relux.works/duet/coordinator/internal/hub"
	"relux.works/duet/coordinator/internal/protocol"
	"relux.works/duet/coordinator/internal/session"
	"relux.works/duet/coordinator/internal/store"
)

var approachCodeRe = regexp.MustCompile(`<code>([A-Z0-9]+)</code>`)

// twoOrbitLoop: orbit 1 = legacy bootstrap (homes a+b, users 111/222),
// second orbit = "Дальний" (user 333, one paired home "a").
func twoOrbitLoop(t *testing.T) (*loop, *fakeSender, int64) {
	t.Helper()
	l, fake := newTestLoop(t)
	// Legacy bootstrap picks a primary by map order — pin it to Ivan (111).
	if err := l.st.TransferPrimary(1, 111); err != nil {
		t.Fatal(err)
	}
	o2, err := l.st.CreateOrbit("Дальний", 333)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := l.st.PairSlot(o2.ID, 333); err != nil {
		t.Fatal(err)
	}
	l.orbit(o2.ID) // warm like the real startup path
	l.handleNode(hub.EvOnline{Key: hub.NodeKey{Orbit: o2.ID, Slot: protocol.NodeA}})
	fake.drain()
	return l, fake, o2.ID
}

// proposeAndAwait drives /approach + /approach CODE, returning the link in
// the awaiting state.
func proposeAndAwait(t *testing.T, l *loop, r *replies) string {
	t.Helper()
	l.handleBot(cmdEvent(t, "a", "/approach", r))
	m := approachCodeRe.FindStringSubmatch(r.last(t))
	if m == nil {
		t.Fatalf("no approach code in reply: %q", r.last(t))
	}
	l.handleBot(cmdEvent(t, "o2", "/approach "+m[1], r))
	if !strings.Contains(r.last(t), "ждём подтверждения") {
		t.Fatalf("claim reply: %q", r.last(t))
	}
	return m[1]
}

// The whole L1 lifecycle: propose -> awaiting -> accept -> one shared
// session over three homes in two orbits -> /apart -> independent orbits.
func TestApproachLifecycleEndToEnd(t *testing.T) {
	l, fake, o2 := twoOrbitLoop(t)
	r := &replies{}

	// Companions cannot propose.
	l.handleBot(cmdEvent(t, "b", "/approach", r))
	if !strings.Contains(r.last(t), "primary") {
		t.Fatalf("companion gate: %q", r.last(t))
	}

	proposeAndAwait(t, l, r)
	// Awaiting is not linked yet: no group, orbits still route to themselves.
	if len(l.groups) != 0 || l.linkOf[1] != 0 {
		t.Fatalf("group before /accept: %v %v", l.groups, l.linkOf)
	}

	// /accept boots the shared session over the union of homes.
	l.handleBot(cmdEvent(t, "a", "/accept", r))
	if l.linkOf[1] == 0 || l.linkOf[1] != l.linkOf[o2] {
		t.Fatalf("linkOf after accept: %v", l.linkOf)
	}
	g := l.stateFor(1)
	if !g.group() || g != l.stateFor(o2) {
		t.Fatal("both orbits must share one group state")
	}
	if len(g.sess.Peers) != 3 {
		t.Fatalf("group peers: %v", g.sess.Peers)
	}
	fake.drain() // stops from parking the personal sessions

	// Heartbeats from all three homes feed the scheduler.
	for _, k := range []hub.NodeKey{{Orbit: 1, Slot: "a"}, {Orbit: 1, Slot: "b"}, {Orbit: o2, Slot: "a"}} {
		l.handleNodeMessage(hub.EvMessage{Key: k, Payload: &protocol.StatePayload{PositionMS: 0, RTTMS: 40, Volume: 80}})
	}

	// A track from orbit-2's primary loads EVERY home of BOTH orbits.
	l.handleBot(cmdEvent(t, "o2", link, r))
	loads := fake.ofType(protocol.TypeLoad)
	if len(loads) != 3 {
		t.Fatalf("load fan-out: %+v", loads)
	}
	seen := map[hub.NodeKey]bool{}
	for _, m := range loads {
		seen[m.key] = true
	}
	for _, k := range []hub.NodeKey{{Orbit: 1, Slot: "a"}, {Orbit: 1, Slot: "b"}, {Orbit: o2, Slot: "a"}} {
		if !seen[k] {
			t.Fatalf("no load to %+v, got %v", k, seen)
		}
	}
	elID := g.sess.Current.ID
	fake.drain()

	// Ready from every home arms one synchronized start across orbits.
	for _, k := range []hub.NodeKey{{Orbit: 1, Slot: "a"}, {Orbit: 1, Slot: "b"}, {Orbit: o2, Slot: "a"}} {
		l.handleNodeMessage(hub.EvMessage{Key: k, Payload: &protocol.ReadyPayload{ElementID: elID}})
	}
	if got := fake.ofType(protocol.TypeResumeAt); len(got) != 3 {
		t.Fatalf("resume_at fan-out: %+v", fake.sent)
	}
	if g.sess.State != session.StateArmed {
		t.Fatalf("state = %s", g.sess.State)
	}

	// Composite homes render as "slot@title" in /status.
	l.handleBot(cmdEvent(t, "a", "/status", r))
	if !strings.Contains(r.last(t), "⇄") || !strings.Contains(r.last(t), "a@Дальний") {
		t.Fatalf("status rendering: %q", r.last(t))
	}
	fake.drain()

	// /apart (either primary) kills the group: stop to every home, both
	// orbits routed to their own sessions again.
	l.handleBot(cmdEvent(t, "o2", "/apart", r))
	stops := fake.ofType(protocol.TypeStop)
	if len(stops) < 3 {
		t.Fatalf("apart must stop every home: %+v", fake.sent)
	}
	if len(l.groups) != 0 || l.linkOf[1] != 0 || l.linkOf[o2] != 0 {
		t.Fatalf("group survived apart: %v %v", l.groups, l.linkOf)
	}
	if _, _, ok, _ := l.st.ActiveLink(1); ok {
		t.Fatal("link row survived apart")
	}
	fake.drain()

	// Orbit 1 broadcasts to its own two homes only…
	l.handleBot(cmdEvent(t, "a", link2, r))
	loads = fake.ofType(protocol.TypeLoad)
	if len(loads) != 2 {
		t.Fatalf("post-apart loads: %+v", loads)
	}
	for _, m := range loads {
		if m.key.Orbit != 1 {
			t.Fatalf("post-apart leak to orbit %d", m.key.Orbit)
		}
	}
	fake.drain()

	// …and orbit 2 to its single home, independently.
	l.handleBot(cmdEvent(t, "o2", link, r))
	loads = fake.ofType(protocol.TypeLoad)
	if len(loads) != 1 || loads[0].key.Orbit != o2 {
		t.Fatalf("orbit-2 loads: %+v", loads)
	}
}

// /decline burns the awaiting link and leaves everyone free to try again.
func TestApproachDecline(t *testing.T) {
	l, _, _ := twoOrbitLoop(t)
	r := &replies{}

	proposeAndAwait(t, l, r)
	l.handleBot(cmdEvent(t, "a", "/decline", r))
	if !strings.Contains(r.last(t), "отклонил") {
		t.Fatalf("decline reply: %q", r.last(t))
	}
	if _, _, ok, _ := l.st.AwaitingLink(1); ok {
		t.Fatal("awaiting link survived decline")
	}
	if len(l.groups) != 0 {
		t.Fatal("decline must not create a group")
	}
	// Both sides can start over.
	l.handleBot(cmdEvent(t, "o2", "/approach", r))
	if approachCodeRe.FindStringSubmatch(r.last(t)) == nil {
		t.Fatalf("re-propose after decline: %q", r.last(t))
	}
}

// L1 at the bot level: an orbit in an active approach cannot start another.
func TestApproachOnePerOrbit(t *testing.T) {
	l, _, _ := twoOrbitLoop(t)
	r := &replies{}

	proposeAndAwait(t, l, r)
	l.handleBot(cmdEvent(t, "a", "/accept", r))

	l.handleBot(cmdEvent(t, "a", "/approach", r))
	if !strings.Contains(r.last(t), "/apart") {
		t.Fatalf("busy propose reply: %q", r.last(t))
	}
	// Nothing pending: /accept and /apart answer honestly.
	l.handleBot(cmdEvent(t, "a", "/accept", r))
	if !strings.Contains(r.last(t), "нечего") {
		t.Fatalf("stray accept reply: %q", r.last(t))
	}
	l.handleBot(cmdEvent(t, "a", "/apart", r))
	l.handleBot(cmdEvent(t, "a", "/apart", r))
	if !strings.Contains(r.last(t), "активного сближения нет") {
		t.Fatalf("double apart reply: %q", r.last(t))
	}
}

// Activation parks a busy personal broadcast: stop to its homes, session
// frozen paused, nothing leaks into the fresh group air — and /apart hands
// the parked material back to /resume.
// Approach-to-stream (§12 L1.5): the code issuer's playing track continues
// onto all homes of the group, and the issuer's own snapshot is emptied so
// nothing double-plays after /apart.
func TestApproachToStreamContinuesBusy(t *testing.T) {
	l, fake, _ := twoOrbitLoop(t)
	r := &replies{}
	// Orbit 1 (the issuer) is mid-broadcast when the approach activates.
	for _, k := range []hub.NodeKey{{Orbit: 1, Slot: "a"}, {Orbit: 1, Slot: "b"}} {
		l.handleNodeMessage(hub.EvMessage{Key: k, Payload: &protocol.StatePayload{PositionMS: 5000, RTTMS: 40, Volume: 80}})
	}
	l.handleBot(cmdEvent(t, "a", link, r))
	trackURI := l.orbit(1).sess.Current.URI
	fake.drain()

	proposeAndAwait(t, l, r)
	l.handleBot(cmdEvent(t, "a", "/accept", r))

	// The group now carries orbit-1's track and loads it to the online homes.
	g := l.stateFor(1)
	if g.sess.Current == nil || g.sess.Current.URI != trackURI {
		t.Fatalf("group must continue the issuer's track, got %v", g.sess.Current)
	}
	if g.sess.State != session.StateLoading {
		t.Fatalf("group should be loading the continued track, state=%s", g.sess.State)
	}
	if len(fake.ofType(protocol.TypeLoad)) == 0 {
		t.Fatalf("continued track must load to homes: %+v", fake.sent)
	}
	// The donor's own snapshot is emptied — no double play lurking.
	if l.orbit(1).sess.Current != nil || len(l.orbit(1).sess.Queue) != 0 {
		t.Fatalf("issuer snapshot must be emptied, got %v", l.orbit(1).sess.Current)
	}
}

// An active approach and its shared session survive a coordinator restart:
// warmup rebuilds linkOf + the group state from the links table and the
// persisted -linkID snapshot (queue intact, paused).
func TestApproachSurvivesRestart(t *testing.T) {
	cfg := testConfig(t)
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.BootstrapLegacyOrbit(
		map[string]string{"a": cfg.Nodes["a"].Token, "b": cfg.Nodes["b"].Token},
		cfg.Telegram.Users); err != nil {
		t.Fatal(err)
	}
	if err := st.TransferPrimary(1, 111); err != nil {
		t.Fatal(err)
	}
	o2, err := st.CreateOrbit("Дальний", 333)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := st.PairSlot(o2.ID, 333); err != nil {
		t.Fatal(err)
	}
	fake := &fakeSender{}
	l := newLoop(slog.Default(), cfg, fake, st, nil, nil)
	l.warmup()
	for _, k := range []hub.NodeKey{{Orbit: 1, Slot: "a"}, {Orbit: 1, Slot: "b"}, {Orbit: o2.ID, Slot: "a"}} {
		l.handleNode(hub.EvOnline{Key: k})
	}
	r := &replies{}
	proposeAndAwait(t, l, r)
	l.handleBot(cmdEvent(t, "a", "/accept", r))
	l.handleBot(cmdEvent(t, "o2", link, r)) // current
	l.handleBot(cmdEvent(t, "a", link2, r)) // queued
	linkID := l.linkOf[1]
	if linkID == 0 || l.stateFor(1).sess.Current == nil {
		t.Fatalf("pre-restart group: link %d, current %v", linkID, l.stateFor(1).sess.Current)
	}
	st.Close()

	st2, err := store.Open(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	fake2 := &fakeSender{}
	l2 := newLoop(slog.Default(), cfg, fake2, st2, nil, nil)
	l2.warmup()

	if l2.linkOf[1] != linkID || l2.linkOf[o2.ID] != linkID {
		t.Fatalf("linkOf after restart: %v", l2.linkOf)
	}
	g := l2.stateFor(1)
	if !g.group() || len(g.sess.Peers) != 3 {
		t.Fatalf("group after restart: %v", g.sess.Peers)
	}
	if g.sess.State != session.StatePaused || g.sess.Current == nil || g.sess.QueueLen() != 1 {
		t.Fatalf("restored group session: state=%s current=%v queue=%d",
			g.sess.State, g.sess.Current, g.sess.QueueLen())
	}
}
