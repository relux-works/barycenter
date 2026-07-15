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
	"relux.works/duet/coordinator/internal/media"
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

	// Composite homes render with human Barycenter/Pulsar names in /status.
	l.handleBot(cmdEvent(t, "a", "/status", r))
	if !strings.Contains(r.last(t), "⇄") || !strings.Contains(r.last(t), "«Дальний»") ||
		!strings.Contains(r.last(t), "Пульсар A") {
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

func TestLinkedVoiceProcessingCannotReorderBarycenters(t *testing.T) {
	l, _, o2 := twoOrbitLoop(t)
	r := &replies{}
	proposeAndAwait(t, l, r)
	l.handleBot(cmdEvent(t, "a", "/accept", r))
	g := l.stateFor(1)
	if g != l.stateFor(o2) || !g.group() {
		t.Fatal("approach did not create one shared air")
	}

	orderAir := g.id
	l.voiceNext[orderAir] = 1
	l.voiceAccepted[orderAir] = 2
	done := func(sourceOrbit, sequence, acceptedAt int64, mediaID, from string) mediaDone {
		return mediaDone{
			orbit: sourceOrbit, orderAir: orderAir, mediaID: mediaID,
			fromName: from, acceptedAt: acceptedAt, sequence: sequence,
			result: media.Result{DurationMS: 1_000, WAVPath: "/tmp/" + mediaID + ".wav", LoudnormJSON: "{}"},
			reply:  func(string) {},
		}
	}

	// The later message belongs to the other Barycenter and finishes first.
	// It still cannot escape the shared-air reorder buffer.
	l.handleMediaDone(done(o2, 2, 2_000, "m_timur", "Timur"))
	if g.sess.Current != nil {
		t.Fatalf("later linked voice escaped the reorder buffer: %+v", g.sess.Current)
	}
	l.handleMediaDone(done(1, 1, 1_000, "m_ivan", "Ivan"))
	if g.sess.Current == nil || g.sess.Current.MediaID != "m_ivan" {
		t.Fatalf("first linked voice = %+v, want Ivan", g.sess.Current)
	}
	if g.sess.QueueLen() != 1 || g.sess.Queue[0].MediaID != "m_timur" {
		t.Fatalf("second linked voice queue = %+v, want Timur", g.sess.Queue)
	}
}

func TestProviderResolveReturnsToLinkedAir(t *testing.T) {
	l, _, o2 := twoOrbitLoop(t)
	l.cfg.Providers = true
	l.resolveTrack = func(_, _ string, _ string, _ []string) resolvedTrack {
		return resolvedTrack{Title: "Linked Song", Artists: []string{"Artist"}, Method: "same", Score: 1}
	}
	r := &replies{}
	proposeAndAwait(t, l, r)
	l.handleBot(cmdEvent(t, "a", "/accept", r))
	g := l.stateFor(1)

	l.handleBot(cmdEvent(t, "o2", link, r))
	l.pumpResolve(t)
	if g != l.stateFor(o2) || g.sess.Current == nil ||
		g.sess.Current.Title != "Artist — Linked Song" {
		t.Fatalf("resolved link escaped shared air: group=%v current=%+v", g == l.stateFor(o2), g.sess.Current)
	}
}

func cutoverApproachLoop(t *testing.T) (*loop, *fakeSender, int64) {
	t.Helper()
	legacy, fake, peerOrbit := twoOrbitLoop(t)
	path := legacy.cfg.DBPath
	if err := legacy.st.Close(); err != nil {
		t.Fatal(err)
	}
	legacy.cfg.SelfServiceOnboarding = true
	st, err := store.OpenWithOptions(path, store.Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.CutoverLinksToAirs(1, 100); err != nil {
		t.Fatal(err)
	}
	l := newLoop(slog.Default(), legacy.cfg, fake, st, nil, nil)
	l.warmup()
	l.handleNode(hub.EvOnline{Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeA}})
	l.handleNode(hub.EvOnline{Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeB}})
	l.handleNode(hub.EvOnline{Key: hub.NodeKey{Orbit: peerOrbit, Slot: protocol.NodeA}})
	fake.drain()
	return l, fake, peerOrbit
}

func TestApproachAliasesUseOnlyAirAuthorityAfterCutover(t *testing.T) {
	l, fake, peerOrbit := cutoverApproachLoop(t)
	r := &replies{}

	l.handleBot(cmdEvent(t, "a", "/approach", r))
	match := approachCodeRe.FindStringSubmatch(r.last(t))
	if match == nil {
		t.Fatalf("Air alias code reply=%q", r.last(t))
	}
	code := match[1]
	// Retrying creation is stable: no second Air or invite is created.
	l.handleBot(cmdEvent(t, "a", "/approach", r))
	second := approachCodeRe.FindStringSubmatch(r.last(t))
	if second == nil || second[1] != code {
		t.Fatalf("Air alias retry code=%q first=%q reply=%q", second, code, r.last(t))
	}

	l.handleBot(cmdEvent(t, "o2", "/approach "+code, r))
	if !strings.Contains(r.last(t), "подтверди /accept") {
		t.Fatalf("Air claim reply=%q", r.last(t))
	}
	// The legacy inviter no longer performs final confirmation after cutover.
	l.handleBot(cmdEvent(t, "a", "/accept", r))
	if runtime, err := l.st.ActiveAirRuntimeForOrbit(1); err != nil || runtime != nil {
		t.Fatalf("issuer activated claimed Air runtime=%+v err=%v", runtime, err)
	}

	l.handleBot(cmdEvent(t, "o2", "/accept", r))
	runtime := l.stateFor(1)
	if runtime.airID == "" || runtime != l.stateFor(peerOrbit) || len(runtime.orbits) != 2 {
		t.Fatalf("Air alias runtime=%+v peer=%+v", runtime, l.stateFor(peerOrbit))
	}
	if len(l.linkOf) != 0 || len(l.groups) != 0 {
		t.Fatalf("legacy runtime mutated after cutover links=%v groups=%v", l.linkOf, l.groups)
	}
	if _, _, ok, _ := l.st.ActiveLink(1); ok {
		t.Fatal("Air alias created an active legacy link")
	}
	l.handleBot(cmdEvent(t, "a", "/orbit", r))
	if !strings.Contains(r.last(t), "сближение с «Дальний»") || strings.Contains(r.last(t), "air_") {
		t.Fatalf("Air /home copy leaked authority identifiers: %q", r.last(t))
	}

	beforeDuplicate := len(r.texts)
	fake.drain()
	l.handleBot(cmdEvent(t, "o2", "/accept", r))
	if len(r.texts) != beforeDuplicate+1 || !strings.Contains(r.last(t), "уже активно") {
		t.Fatalf("duplicate accept reply=%q", r.last(t))
	}
	if len(fake.drain()) != 0 {
		t.Fatal("duplicate accept emitted a second broadcast")
	}

	// Warmup reconstructs the same runtime strictly by stable Air ID.
	restarted := newLoop(l.log, l.cfg, fake, l.st, nil, nil)
	restarted.warmup()
	restored := restarted.stateFor(peerOrbit)
	if restored.airID != runtime.airID || restored != restarted.stateFor(1) || len(restarted.linkOf) != 0 {
		t.Fatalf("restart alias runtime=%+v links=%v", restored, restarted.linkOf)
	}
	fake.drain()
	restarted.handleBot(cmdEvent(t, "o2", "/apart", r))
	if restarted.stateFor(peerOrbit).group() || restarted.stateFor(1).group() {
		t.Fatal("apart did not restore personal orbit controllers")
	}
	if _, _, ok, err := restarted.st.ActiveAirForOrbit(peerOrbit); err != nil || ok {
		t.Fatalf("caller pointer survived apart ok=%v err=%v", ok, err)
	}
	ownerAir, _, ok, err := restarted.st.ActiveAirForOrbit(1)
	if err != nil || !ok || ownerAir != runtime.airID {
		t.Fatalf("apart removed other member pointer=%q ok=%v err=%v", ownerAir, ok, err)
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

// C1 regression: the LAST member of a linked orbit /leave's. LeaveOrbit ->
// DeleteOrbit erases the links row before dissolveOrbit could read it, so the
// break must run from in-memory link state — before the fix the partner orbit
// was stranded behind a phantom group session (air bricked, /apart refused)
// until a coordinator restart.
func TestLastMemberLeaveWhileLinkedFreesPartner(t *testing.T) {
	l, fake, o2 := twoOrbitLoop(t)
	r := &replies{}
	proposeAndAwait(t, l, r)
	l.handleBot(cmdEvent(t, "a", "/accept", r))
	linkID := l.linkOf[1]
	if linkID == 0 || l.groups[linkID] == nil {
		t.Fatal("precondition: active link with a live group session")
	}
	fake.drain()

	// User 333 is the only member of orbit 2 -> /leave dissolves it mid-link.
	l.handleBot(cmdEvent(t, "o2", "/leave", r))

	if _, ok := l.groups[linkID]; ok {
		t.Fatal("group session must die with the linked orbit")
	}
	if l.linkOf[1] != 0 || l.linkOf[o2] != 0 {
		t.Fatalf("linkOf must be cleared, got %v", l.linkOf)
	}
	if o := l.stateFor(1); o.group() {
		t.Fatalf("stateFor(1) still routes to a group (id=%d)", o.id)
	}
	// The survivor's air works immediately: a track loads to its own homes.
	fake.drain()
	l.handleNodeMessage(hub.EvMessage{Key: hub.NodeKey{Orbit: 1, Slot: "a"}, Payload: &protocol.StatePayload{PositionMS: 0, RTTMS: 40, Volume: 80}})
	l.handleNodeMessage(hub.EvMessage{Key: hub.NodeKey{Orbit: 1, Slot: "b"}, Payload: &protocol.StatePayload{PositionMS: 0, RTTMS: 60, Volume: 80}})
	l.handleBot(cmdEvent(t, "a", link, r))
	if st := l.stateFor(1).sess.State; st != session.StateLoading {
		t.Fatalf("survivor air state = %s, want loading", st)
	}
	if got := fake.ofType(protocol.TypeLoad); len(got) != 2 {
		t.Fatalf("post-leave loads to orbit 1's own homes: %+v", fake.sent)
	}
}

// H3 regression (leave-during-link): a member with a paired home /leave's
// while an approach is active; the slot revocation lands on the GROUP session
// only. breakGroup must re-read the slot set — before the fix the personal
// session still listed the revoked home after /apart and the strict gate
// parked every new track forever.
func TestMemberLeaveDuringLinkHealsPersonalPeers(t *testing.T) {
	l, fake, _ := twoOrbitLoop(t)
	r := &replies{}
	proposeAndAwait(t, l, r)
	l.handleBot(cmdEvent(t, "a", "/accept", r))
	fake.drain()

	// Katya (user 222, home "b" of orbit 1) leaves while linked.
	l.handleBot(cmdEvent(t, "b", "/leave", r))
	l.handleBot(cmdEvent(t, "a", "/apart", r))

	o := l.orbit(1)
	for _, p := range o.sess.Peers {
		if p == protocol.NodeID("b") {
			t.Fatal("personal session still lists the revoked slot 'b' after /apart")
		}
	}
	// The strict gate passes with the remaining home online: a track plays.
	fake.drain()
	l.handleNodeMessage(hub.EvMessage{Key: hub.NodeKey{Orbit: 1, Slot: "a"}, Payload: &protocol.StatePayload{PositionMS: 0, RTTMS: 40, Volume: 80}})
	l.handleBot(cmdEvent(t, "a", link, r))
	if st := o.sess.State; st != session.StateLoading {
		t.Fatalf("state = %s, want loading (gate must not wait for revoked 'b')", st)
	}
}

// H3 regression (pair-during-link): a home paired while the approach is active
// registers into the group session only. After /apart the personal session
// must know it — before the fix the new home never got a load and stayed
// silent until a coordinator restart.
func TestSlotPairedDuringLinkJoinsPersonalAfterApart(t *testing.T) {
	l, _, o2 := twoOrbitLoop(t)
	r := &replies{}
	proposeAndAwait(t, l, r)
	l.handleBot(cmdEvent(t, "a", "/accept", r))

	slot, _, err := l.st.PairSlot(o2, 333)
	if err != nil {
		t.Fatal(err)
	}
	l.handleNode(hub.EvRegistered{Key: hub.NodeKey{Orbit: o2, Slot: protocol.NodeID(slot)}})
	l.handleNode(hub.EvOnline{Key: hub.NodeKey{Orbit: o2, Slot: protocol.NodeID(slot)}})

	l.handleBot(cmdEvent(t, "a", "/apart", r))

	found := false
	for _, p := range l.orbit(o2).sess.Peers {
		if p == protocol.NodeID(slot) {
			found = true
		}
	}
	if !found {
		t.Fatalf("slot %q paired during the link is missing from orbit %d peers after /apart: %v",
			slot, o2, l.orbit(o2).sess.Peers)
	}
}

// M4 regression: the CLAIMANT of an approach code can /decline the awaiting
// link too. Before, only the initiator could — an initiator gone dark left
// the claimant link-locked ("сначала /apart" while /apart needs an ACTIVE
// link) until the 48 h TTL.
func TestClaimantCanDeclineAwaiting(t *testing.T) {
	l, _, o2 := twoOrbitLoop(t)
	r := &replies{}
	proposeAndAwait(t, l, r)

	// Orbit 2 (the claimant, user 333) withdraws.
	l.handleBot(cmdEvent(t, "o2", "/decline", r))
	if !strings.Contains(r.last(t), "отозвал") {
		t.Fatalf("claimant decline reply: %q", r.last(t))
	}
	if _, _, _, ok, _ := l.st.AwaitingLinkAnySide(o2); ok {
		t.Fatal("awaiting link survived the claimant's decline")
	}
	// Both sides are free to start over.
	l.handleBot(cmdEvent(t, "o2", "/approach", r))
	if approachCodeRe.FindStringSubmatch(r.last(t)) == nil {
		t.Fatalf("re-propose after claimant decline: %q", r.last(t))
	}
}
