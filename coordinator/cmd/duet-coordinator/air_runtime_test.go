package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"relux.works/duet/coordinator/internal/hub"
	"relux.works/duet/coordinator/internal/media"
	"relux.works/duet/coordinator/internal/protocol"
	"relux.works/duet/coordinator/internal/session"
	"relux.works/duet/coordinator/internal/store"
)

func createActiveTestAir(t *testing.T, l *loop) (string, int64, int64) {
	t.Helper()
	peer, err := l.st.CreateOrbit("Air peer", 7301)
	if err != nil {
		t.Fatal(err)
	}
	third, err := l.st.CreateOrbit("Air third", 7302)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ orbit, user int64 }{{peer.ID, 7301}, {third.ID, 7302}} {
		if _, _, err := l.st.PairSlot(item.orbit, item.user); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := l.st.CutoverLinksToAirs(1, 100); err != nil {
		t.Fatal(err)
	}
	air, err := l.st.CreateAir(store.CreateAirParams{Title: "Stable Air", OwnerOrbitID: 1, CreatedAt: 110})
	if err != nil {
		t.Fatal(err)
	}
	if err := l.st.ActivateAir(1, air.ID, "none", 120); err != nil {
		t.Fatal(err)
	}
	for index, orbitID := range []int64{peer.ID, third.ID} {
		member, err := l.st.AddPendingAirMember(air.ID, orbitID, "member", int64(130+index))
		if err != nil {
			t.Fatal(err)
		}
		if err := l.st.ConfirmAirMember(member.ID, 1, true, "none", int64(140+index)); err != nil {
			t.Fatal(err)
		}
	}
	return air.ID, peer.ID, third.ID
}

func TestAirRuntimeOwnsExactCurrentUnionAndWarmupIsLazy(t *testing.T) {
	l, fake := newTestLoop(t)
	airID, peer, third := createActiveTestAir(t, l)

	// A separate saved membership must not create a transitive runtime union.
	parkedOwner, err := l.st.CreateOrbit("Parked owner", 7303)
	if err != nil {
		t.Fatal(err)
	}
	parked, err := l.st.CreateAir(store.CreateAirParams{Title: "Saved elsewhere", OwnerOrbitID: parkedOwner.ID, CreatedAt: 160})
	if err != nil {
		t.Fatal(err)
	}
	saved, err := l.st.AddPendingAirMember(parked.ID, peer, "member", 170)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.st.ConfirmAirMember(saved.ID, 1, false, "none", 180); err != nil {
		t.Fatal(err)
	}

	runtime := l.stateFor(1)
	if runtime.airID != airID || runtime.id != 0 || runtime != l.stateFor(peer) || runtime != l.stateFor(third) {
		t.Fatalf("stable Air ownership id=%q legacy_id=%d", runtime.airID, runtime.id)
	}
	if len(runtime.orbits) != 3 || len(runtime.sess.Peers) != 4 {
		t.Fatalf("runtime orbits=%v peers=%v", runtime.orbits, runtime.sess.Peers)
	}
	if _, err := l.st.ProposeLink(1, 111); !errors.Is(err, store.ErrAirRevision) {
		t.Fatalf("Air authority accepted legacy link write: %v", err)
	}
	for _, node := range runtime.sess.Peers {
		if orbitID, _, _ := splitComposite(node); orbitID == parkedOwner.ID {
			t.Fatalf("saved Air leaked transitively into peers: %v", runtime.sess.Peers)
		}
	}
	fake.drain()
	replies := &replies{}
	l.handleBot(cmdEvent(t, "a", link, replies))
	loads := fake.ofType(protocol.TypeLoad)
	unique := map[hub.NodeKey]bool{}
	for _, load := range loads {
		unique[load.key] = true
	}
	if len(loads) != 4 || len(unique) != 4 {
		t.Fatalf("Air main-track fanout loads=%+v unique=%v", loads, unique)
	}
	l.cancelReadyTimer(runtime)
	runtime.sess.Current = &session.Element{ID: "living-main", Kind: session.KindTrack, URI: "spotify:track:living"}
	runtime.sess.State = session.StatePlaying
	runtime.sess.Queue = []session.Element{{ID: "next-main", Kind: session.KindTrack}}
	l.persist(runtime)

	// A restart instantiates the one active Air, restores it paused under the
	// stable public ID, and leaves the saved/parked Air completely lazy.
	restarted := newLoop(l.log, l.cfg, fake, l.st, nil, nil)
	restarted.warmup()
	if len(restarted.airs) != 1 || restarted.airs[parked.ID] != nil {
		t.Fatalf("warm Air cache=%v", restarted.airs)
	}
	restored := restarted.stateFor(peer)
	if restored.airID != airID || restored.sess.State != session.StatePaused ||
		restored.sess.Current == nil || restored.sess.Current.ID != "living-main" || restored.sess.QueueLen() != 1 {
		t.Fatalf("restored runtime air=%q state=%s current=%+v queue=%d", restored.airID,
			restored.sess.State, restored.sess.Current, restored.sess.QueueLen())
	}
	if len(restarted.linkOf) != 0 || len(restarted.groups) != 0 {
		t.Fatalf("Air authority warmed legacy runtime links=%v groups=%v", restarted.linkOf, restarted.groups)
	}
}

func TestAirRuntimeLeaveStopsOnlyCallerThenParksBelowTwoMembers(t *testing.T) {
	l, fake := newTestLoop(t)
	airID, peer, third := createActiveTestAir(t, l)
	runtime := l.stateFor(1)
	if runtime.airID != airID {
		t.Fatal("active Air missing")
	}
	fake.drain()

	if err := l.st.DeactivateAir(third, airID, 200); err != nil {
		t.Fatal(err)
	}
	if personal := l.stateFor(third); personal.group() || personal.id != third {
		t.Fatalf("leaver did not restore personal state: %+v", personal)
	}
	for _, message := range fake.drain() {
		if message.key.Orbit != third {
			t.Fatalf("leaver disturbed remaining node: %+v", message)
		}
	}
	remaining := l.stateFor(1)
	if remaining.airID != airID || remaining != l.stateFor(peer) || len(remaining.orbits) != 2 {
		t.Fatalf("remaining Air=%q orbits=%v", remaining.airID, remaining.orbits)
	}

	if err := l.st.DeactivateAir(peer, airID, 210); err != nil {
		t.Fatal(err)
	}
	fake.drain()
	_ = l.stateFor(peer)
	if l.airs[airID] != nil || l.stateFor(1).group() {
		t.Fatalf("parked Air retained live runtime: airs=%v owner=%+v", l.airs, l.stateFor(1))
	}
	for _, message := range fake.drain() {
		if message.msgType != protocol.TypeStop {
			continue
		}
		if message.key.Orbit != 1 && message.key.Orbit != peer {
			t.Fatalf("unexpected parked stop: %+v", message)
		}
	}
}

func TestAirRuntimeJoinCatchesCurrentTrackButNeverOldVoiceOverlay(t *testing.T) {
	l, fake := newTestLoop(t)
	airID, _, third := createActiveTestAir(t, l)
	runtime := l.stateFor(1)
	if err := l.st.DeactivateAir(third, airID, 200); err != nil {
		t.Fatal(err)
	}
	_ = l.stateFor(third)
	runtime = l.stateFor(1)
	runtime.sess.Current = &session.Element{ID: "main-track", Kind: session.KindTrack, URI: "spotify:track:main"}
	runtime.sess.State = session.StatePlaying
	fake.drain()
	if err := l.st.ActivateAir(third, airID, "none", 210); err != nil {
		t.Fatal(err)
	}
	_ = l.stateFor(third)
	loads := fake.ofType(protocol.TypeLoad)
	if len(loads) != 1 || loads[0].key.Orbit != third {
		t.Fatalf("joining main-track catch-up=%+v", fake.sent)
	}

	if err := l.st.DeactivateAir(third, airID, 220); err != nil {
		t.Fatal(err)
	}
	_ = l.stateFor(third)
	runtime = l.stateFor(1)
	runtime.sess.Current = &session.Element{ID: "old-overlay", Kind: session.KindVoice, MediaID: "clip"}
	runtime.sess.State = session.StateVoice
	fake.drain()
	if err := l.st.ActivateAir(third, airID, "none", 230); err != nil {
		t.Fatal(err)
	}
	_ = l.stateFor(third)
	for _, message := range fake.drain() {
		if message.key.Orbit == third &&
			(message.msgType == protocol.TypeLoad || message.msgType == protocol.TypePlayVoice) {
			t.Fatalf("joining member heard stale overlay: %+v", message)
		}
	}
}

func TestAirRuntimeVoiceOrderIsScopedByStableAirID(t *testing.T) {
	l, _ := newTestLoop(t)
	airID, peer, _ := createActiveTestAir(t, l)
	runtime := l.stateFor(1)
	l.airVoiceNext[airID] = 1
	l.airVoiceAccepted[airID] = 2
	done := func(orbitID, sequence int64, mediaID string) mediaDone {
		return mediaDone{
			orbit: orbitID, orderAirID: airID,
			orderAirGeneration: runtime.authorityGeneration,
			orderAirRevision:   runtime.airRevision,
			sequence:           sequence, mediaID: mediaID, fromName: mediaID,
			acceptedAt: sequence, result: media.Result{DurationMS: 1000,
				WAVPath: "/tmp/" + mediaID + ".wav", LoudnormJSON: "{}"},
			reply: func(string) {},
		}
	}
	l.handleMediaDone(done(peer, 2, "air-second"))
	if runtime.sess.Current != nil {
		t.Fatalf("later Air voice escaped ordering: %+v", runtime.sess.Current)
	}
	l.handleMediaDone(done(1, 1, "air-first"))
	if runtime.sess.Current == nil || runtime.sess.Current.MediaID != "air-first" ||
		runtime.sess.QueueLen() != 1 || runtime.sess.Queue[0].MediaID != "air-second" {
		t.Fatalf("Air voice order current=%+v queue=%+v", runtime.sess.Current, runtime.sess.Queue)
	}
}

func TestAirRuntimeSwitchDetachesOldOwnerBeforeJoiningNewAir(t *testing.T) {
	l, fake := newTestLoop(t)
	oldAirID, _, switching := createActiveTestAir(t, l)
	oldRuntime := l.stateFor(1)
	owner, err := l.st.CreateOrbit("Second owner", 7350)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := l.st.PairSlot(owner.ID, 7350); err != nil {
		t.Fatal(err)
	}
	newAir, err := l.st.CreateAir(store.CreateAirParams{Title: "Second Air", OwnerOrbitID: owner.ID, CreatedAt: 300})
	if err != nil {
		t.Fatal(err)
	}
	if err := l.st.ActivateAir(owner.ID, newAir.ID, "none", 310); err != nil {
		t.Fatal(err)
	}
	member, err := l.st.AddPendingAirMember(newAir.ID, switching, "member", 320)
	if err != nil {
		t.Fatal(err)
	}
	fake.drain()
	if err := l.st.ConfirmAirMember(member.ID, 1, true, oldAirID, 330); err != nil {
		t.Fatal(err)
	}
	newRuntime := l.stateFor(switching)
	if newRuntime.airID != newAir.ID || newRuntime != l.stateFor(owner.ID) {
		t.Fatalf("new runtime=%q owner runtime=%q", newRuntime.airID, l.stateFor(owner.ID).airID)
	}
	if oldRuntime != l.stateFor(1) || len(oldRuntime.orbits) != 2 {
		t.Fatalf("old Air disturbed orbits=%v", oldRuntime.orbits)
	}
	for _, peer := range oldRuntime.sess.Peers {
		if orbitID, _, _ := splitComposite(peer); orbitID == switching {
			t.Fatalf("switching orbit remained in old Air peers=%v", oldRuntime.sess.Peers)
		}
	}
	stops := fake.ofType(protocol.TypeStop)
	for _, stop := range stops {
		if stop.key.Orbit != switching {
			t.Fatalf("Air switch stopped remaining old member: %+v", stop)
		}
	}
}

func TestAirRuntimeRejectsAsyncCompletionAfterOwnershipSwitch(t *testing.T) {
	l, _ := newTestLoop(t)
	oldAirID, _, switching := createActiveTestAir(t, l)
	oldRuntime := l.stateFor(switching)
	staleFence := fenceFor(oldRuntime)
	owner, err := l.st.CreateOrbit("Async owner", 7360)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := l.st.PairSlot(owner.ID, 7360); err != nil {
		t.Fatal(err)
	}
	newAir, err := l.st.CreateAir(store.CreateAirParams{Title: "Async Air", OwnerOrbitID: owner.ID, CreatedAt: 400})
	if err != nil {
		t.Fatal(err)
	}
	if err := l.st.ActivateAir(owner.ID, newAir.ID, "none", 410); err != nil {
		t.Fatal(err)
	}
	member, err := l.st.AddPendingAirMember(newAir.ID, switching, "member", 420)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.st.ConfirmAirMember(member.ID, 1, true, oldAirID, 430); err != nil {
		t.Fatal(err)
	}
	current := l.stateFor(switching)
	if current.airID != newAir.ID {
		t.Fatalf("current Air=%q", current.airID)
	}
	replies := &replies{}
	element := session.Element{ID: "stale-track", Kind: session.KindTrack, URI: "spotify:track:stale"}
	l.handleTrackMetadataDone(trackMetadataDone{
		orbit: switching, fence: staleFence, el: element, reply: replies.fn,
	})
	l.handlePlaylistDone(playlistDone{
		orbit: switching, fence: staleFence, uri: "spotify:playlist:stale",
		title: "Stale", tracks: []string{"spotify:track:stale"}, reply: replies.fn,
	})
	l.onResolveDone(resolveDone{
		orbit: switching, fence: staleFence, el: element,
		originProv: "spotify", originRef: "stale", reply: replies.fn,
	})
	if current.sess.Current != nil || current.sess.QueueLen() != 0 || current.sess.Playlist != nil {
		t.Fatalf("stale completion entered replacement Air current=%+v queue=%+v playlist=%+v",
			current.sess.Current, current.sess.Queue, current.sess.Playlist)
	}
	if len(replies.texts) != 3 {
		t.Fatalf("stale completion replies=%v", replies.texts)
	}
}

func TestAirRuntimeMediaCancellationUpdatesLiveAndDurableSession(t *testing.T) {
	l, _ := newTestLoop(t)
	airID, _, _ := createActiveTestAir(t, l)
	runtime := l.stateFor(1)
	runtime.sess.Queue = []session.Element{{
		ID: "air-queued-voice", Kind: session.KindVoice, MediaID: "air-media",
	}}
	l.persist(runtime)
	request := store.MediaDeliveryCancellation{
		MediaID: "air-media", Reason: store.MediaCancellationDeleted,
		PolicyVersion:         store.MediaLifecyclePolicyV1,
		NotStartedAction:      store.MediaNotStartedActionCancel,
		ActiveAction:          store.MediaActiveActionFadeStop,
		InterruptedMainAction: store.MediaInterruptedMainActionResumeOnce,
	}
	if err := l.applyMediaCancellation(request); err != nil {
		t.Fatal(err)
	}
	if runtime.sess.QueueLen() != 0 {
		t.Fatalf("live Air queue=%+v", runtime.sess.Queue)
	}
	snapshot, err := l.st.LoadAirSession(airID)
	if err != nil || snapshot == nil || len(snapshot.Queue) != 0 {
		t.Fatalf("durable Air cancellation snapshot=%+v err=%v", snapshot, err)
	}
}

func TestRollbackHoldNeverWarmsLegacyLinkRuntime(t *testing.T) {
	l, fake := newTestLoop(t)
	peer, err := l.st.CreateOrbit("Legacy peer", 7401)
	if err != nil {
		t.Fatal(err)
	}
	code, err := l.st.ProposeLink(1, 111)
	if err != nil {
		t.Fatal(err)
	}
	linkID, _, err := l.st.AcceptByCode(code, peer.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.st.ActivateLink(linkID); err != nil {
		t.Fatal(err)
	}
	if err := l.st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(l.cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if _, err := st.CutoverLinksToAirs(1, 100); err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateAir(store.CreateAirParams{Title: "Divergent", OwnerOrbitID: 1, CreatedAt: 110}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.RollbackAirsToLinks(2, 120); err == nil {
		t.Fatal("divergent rollback unexpectedly succeeded")
	}
	hold := newLoop(l.log, l.cfg, fake, st, nil, nil)
	hold.warmup()
	if len(hold.linkOf) != 0 || len(hold.groups) != 0 || len(hold.airs) != 0 {
		t.Fatalf("rollback hold warmed shared runtime links=%v groups=%v airs=%v", hold.linkOf, hold.groups, hold.airs)
	}
}

func TestAirRuntimeHandoffPinsOwnershipAndLifecycleBoundaries(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	raw, err := os.ReadFile(filepath.Join(root, "docs", "analysis", "p2-air-runtime-session-resolution.md"))
	if err != nil {
		t.Fatal(err)
	}
	document := string(raw)
	for _, required := range []string{
		"stable public `air_id`", "no shared",
		"Saved membership in another Air contributes no peer",
		"session_state_air_<public-id>", "old voice/overlay is never replayed",
		"stops and removes only that orbit's", "Generic transmission schema",
	} {
		if !strings.Contains(document, required) {
			t.Errorf("runtime handoff lost %q", required)
		}
	}
	runbook, err := os.ReadFile(filepath.Join(root, "docs", "runbook.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(runbook), "p2-air-runtime-session-resolution.md") {
		t.Fatal("runbook lost Air runtime handoff")
	}
}
