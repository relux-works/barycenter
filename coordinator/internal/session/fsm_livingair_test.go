package session

import (
	"testing"

	"relux.works/duet/coordinator/internal/protocol"
)

// side: a=b belong to side "L", c belongs to side "R" (a linked pair where
// one side has two homes). Living air starts when each side has one home up.
func livingSession() *Session {
	s := New()
	s.SetPeers([]string{"a", "b", "c"})
	s.GateMode = GateEachSide
	s.SideOf = func(n protocol.NodeID) string {
		if n == "c" {
			return "R"
		}
		return "L"
	}
	return s
}

// One home per side online (a on L, c on R), b offline -> air starts; b is
// not on the barrier; a+c ready arms the pair without b.
func TestLivingAirStartsWithOneHomePerSide(t *testing.T) {
	s := livingSession()
	s.online["a"] = true
	s.online["c"] = true // b stays offline
	s.OnHeartbeat("a", 0, 40)
	s.OnHeartbeat("c", 0, 60)

	effs := s.EnqueueTrack(trackEl("el1", "spotify:track:X"))
	loads := of[EffLoad](t, effs)
	if len(loads) != 2 {
		t.Fatalf("load only to online homes a,c: %#v", effs)
	}
	// "стартуем без b — догонят" announced once.
	if len(of[EffNotify](t, effs)) != 1 {
		t.Fatalf("expected catch-up notice, got %#v", effs)
	}
	if s.OnReady(1000, "a", "el1") != nil {
		t.Fatal("a alone must not arm")
	}
	effs = s.OnReady(1000, "c", "el1")
	if len(of[EffResumeAt](t, effs)) != 2 || s.State != StateArmed {
		t.Fatalf("a+c arm the air without b: %#v state=%s", effs, s.State)
	}
	s.OnStarted("a", "el1", 1620)
	effs = s.OnStarted("c", "el1", 1650)
	if s.State != StatePlaying {
		t.Fatalf("state=%s", s.State)
	}
	if d := one[EffLogDesync](t, effs); d.DeltaMS != 30 {
		t.Fatalf("desync over participants a,c = %d", d.DeltaMS)
	}
}

// A dark SIDE still parks (living air needs each side present).
func TestLivingAirParksWhenSideDark(t *testing.T) {
	s := livingSession()
	s.online["a"] = true
	s.online["b"] = true // side L up, side R (c) dark
	effs := s.EnqueueTrack(trackEl("el1", "spotify:track:X"))
	if s.State != StateDegraded {
		t.Fatalf("dark side must park, state=%s", s.State)
	}
	if len(of[EffLoad](t, effs)) != 0 {
		t.Fatal("nothing loads while a side is dark")
	}
}

// b wakes mid-flight -> individual load at live position + solo resume_at,
// without disturbing the a+c pair; b graduates on started and counts for eof.
func TestCatchUpJoin(t *testing.T) {
	s := livingSession()
	s.online["a"] = true
	s.online["c"] = true
	s.OnHeartbeat("a", 0, 40)
	s.OnHeartbeat("c", 0, 60)
	s.EnqueueTrack(trackEl("el1", "spotify:track:X"))
	s.OnReady(1000, "a", "el1")
	s.OnReady(1000, "c", "el1")
	s.OnStarted("a", "el1", 1620)
	s.OnStarted("c", "el1", 1650)
	s.OnHeartbeat("a", 30000, 40)
	s.OnHeartbeat("c", 30000, 60)

	// b comes online and catches up.
	s.online["b"] = true
	effs := s.JoinInProgress("b")
	loads := of[EffLoad](t, effs)
	if len(loads) != 1 || loads[0].To != "b" || loads[0].PositionMS != 30000 {
		t.Fatalf("b loads solo at live position: %#v", effs)
	}
	if s.State != StatePlaying {
		t.Fatalf("join must not disturb the playing pair, state=%s", s.State)
	}
	effs = s.OnReady(1100, "b", "el1")
	res := of[EffResumeAt](t, effs)
	if len(res) != 1 || res[0].To != "b" {
		t.Fatalf("b arms alone: %#v", effs)
	}
	s.OnStarted("b", "el1", 1700) // b graduates
	// now b counts for ended: a+c ended but b still playing -> not done.
	s.OnEnded("a", "el1", "eof")
	if effs := s.OnEnded("c", "el1", "eof"); effs != nil {
		t.Fatalf("b now participates, must wait for it: %#v", effs)
	}
	effs = s.OnEnded("b", "el1", "eof")
	one[EffElementDone](t, effs)
}

// A participant dropping mid-play under living air must NOT freeze the air:
// the survivors keep playing and the offline home catches up on return.
func TestLivingAirSurvivesMidPlayDrop(t *testing.T) {
	s := livingSession() // a,b on side L; c on side R
	s.online["a"] = true
	s.online["b"] = true
	s.online["c"] = true
	s.OnHeartbeat("a", 0, 40)
	s.OnHeartbeat("b", 0, 40)
	s.OnHeartbeat("c", 0, 60)
	s.EnqueueTrack(trackEl("el1", "spotify:track:X"))
	s.OnReady(1000, "a", "el1")
	s.OnReady(1000, "b", "el1")
	s.OnReady(1000, "c", "el1")
	s.OnStarted("a", "el1", 1600)
	s.OnStarted("b", "el1", 1600)
	s.OnStarted("c", "el1", 1650)
	if s.State != StatePlaying {
		t.Fatalf("want playing, got %s", s.State)
	}
	effs := s.OnNodeOffline(2000, "b")
	if s.State != StatePlaying {
		t.Fatalf("mid-play drop must NOT degrade under living air, got %s", s.State)
	}
	if len(of[EffPause](t, effs)) != 0 {
		t.Fatalf("survivors must not be paused: %#v", effs)
	}
	s.OnEnded("a", "el1", "eof")
	if e := s.OnEnded("c", "el1", "eof"); len(of[EffElementDone](t, e)) == 0 {
		t.Fatalf("track must end without the offline home: %#v", e)
	}
}

func TestLivingAirLibrespotRestartOnlyReloadsFailedHome(t *testing.T) {
	s := livingSession()
	s.online["a"] = true
	s.online["c"] = true
	s.OnHeartbeatAt(1_000, "a", 30_000, 40)
	s.OnHeartbeatAt(1_000, "c", 30_000, 60)
	s.EnqueueTrack(trackEl("el1", "spotify:track:X"))
	s.OnReady(1_100, "a", "el1")
	s.OnReady(1_100, "c", "el1")
	s.OnStarted("a", "el1", 1_720)
	s.OnStarted("c", "el1", 1_730)
	s.OnHeartbeatAt(10_000, "a", 39_000, 40)
	s.OnHeartbeatAt(10_000, "c", 39_000, 60)

	effs := s.OnNodeErrorAt(10_100, "c", "librespot_restart", "")
	if len(of[EffPause](t, effs)) != 0 {
		t.Fatalf("healthy leader must continue: %#v", effs)
	}
	load := one[EffLoad](t, effs)
	if load.To != "c" || load.PositionMS < 39_000 {
		t.Fatalf("only failed home catches up: %#v", effs)
	}
	if s.State != StatePlaying {
		t.Fatalf("surviving air state = %s", s.State)
	}

	ready := s.OnReady(11_000, "c", "el1")
	resume := one[EffResumeAt](t, ready)
	if resume.To != "c" || resume.PositionMS == nil {
		t.Fatalf("recovered home must seek to the live air: %#v", ready)
	}
}

// When a WHOLE side goes dark, living air DOES park and recovers via the gate,
// not allOnline.
func TestLivingAirParksAndRecoversByGate(t *testing.T) {
	s := livingSession()
	s.online["a"] = true
	s.online["c"] = true // b offline from the start
	s.OnHeartbeat("a", 0, 40)
	s.OnHeartbeat("c", 0, 60)
	s.EnqueueTrack(trackEl("el1", "spotify:track:X"))
	s.OnReady(1000, "a", "el1")
	s.OnReady(1000, "c", "el1")
	s.OnStarted("a", "el1", 1600)
	s.OnStarted("c", "el1", 1650)
	s.OnNodeOffline(2000, "c")
	if s.State != StateDegraded {
		t.Fatalf("a dark side must park, got %s", s.State)
	}
	s.OnHeartbeat("c", 5000, 60)
	effs := s.OnNodeBack("c")
	if s.State != StatePaused {
		t.Fatalf("gate-satisfied recovery expected paused, got %s", s.State)
	}
	if len(of[EffNotify](t, effs)) == 0 {
		t.Fatalf("expected a resume-me notice: %#v", effs)
	}
}

// H2 regression: the LAST not-ready participant drops during LOADING while the
// gate still holds. The barrier must re-run — before the fix the state stalled
// in LOADING for ~2x ready-timeout and then skipped a track every survivor
// already had ready.
func TestLivingAirDropLastNotReadyDuringLoadingArms(t *testing.T) {
	s := livingSession()
	s.online["a"] = true
	s.online["b"] = true
	s.online["c"] = true
	s.OnHeartbeat("a", 0, 40)
	s.OnHeartbeat("b", 0, 40)
	s.OnHeartbeat("c", 0, 60)
	s.EnqueueTrack(trackEl("el1", "spotify:track:X"))
	s.OnReady(1000, "a", "el1")
	s.OnReady(1000, "c", "el1")

	effs := s.OnNodeOffline(1500, "b") // b was the only not-ready participant
	if s.State != StateArmed {
		t.Fatalf("survivors a+c are ready — must arm, got %s", s.State)
	}
	res := of[EffResumeAt](t, effs)
	if len(res) != 2 {
		t.Fatalf("resume_at to both survivors: %#v", effs)
	}
}

// H1 regression: the LAST not-started participant drops during ARMED. Nothing
// else re-evaluates the started barrier (OnEnded ignores ARMED, no timer runs)
// — the air used to hang there until a human /skip.
func TestLivingAirDropLastNotStartedDuringArmedPlays(t *testing.T) {
	s := livingSession()
	s.online["a"] = true
	s.online["b"] = true
	s.online["c"] = true
	s.OnHeartbeat("a", 0, 40)
	s.OnHeartbeat("b", 0, 40)
	s.OnHeartbeat("c", 0, 60)
	s.EnqueueTrack(trackEl("el1", "spotify:track:X"))
	s.OnReady(1000, "a", "el1")
	s.OnReady(1000, "b", "el1")
	s.OnReady(1000, "c", "el1")
	s.OnStarted("a", "el1", 1600)
	s.OnStarted("c", "el1", 1650)

	s.OnNodeOffline(2000, "b") // b never started
	if s.State != StatePlaying {
		t.Fatalf("survivors a+c both started — must play, got %s", s.State)
	}
	// The element can now actually finish.
	s.OnEnded("a", "el1", "eof")
	if effs := s.OnEnded("c", "el1", "eof"); len(of[EffElementDone](t, effs)) == 0 {
		t.Fatalf("track must end for the survivors: %#v", effs)
	}
}

// H1 via /revoke: RemovePeer during ARMED re-checks the started barrier too.
func TestLivingAirRevokeLastNotStartedDuringArmedPlays(t *testing.T) {
	s := livingSession()
	s.online["a"] = true
	s.online["b"] = true
	s.online["c"] = true
	s.OnHeartbeat("a", 0, 40)
	s.OnHeartbeat("b", 0, 40)
	s.OnHeartbeat("c", 0, 60)
	s.EnqueueTrack(trackEl("el1", "spotify:track:X"))
	s.OnReady(1000, "a", "el1")
	s.OnReady(1000, "b", "el1")
	s.OnReady(1000, "c", "el1")
	s.OnStarted("a", "el1", 1600)
	s.OnStarted("c", "el1", 1650)

	s.RemovePeer(2000, "b")
	if s.State != StatePlaying {
		t.Fatalf("revoke of the last laggard must complete the start, got %s", s.State)
	}
}

// JoinInProgress IS the joiner's online edge (the loop routes EvOnline here
// instead of OnNodeBack): the session must record it, or the NEXT element's
// participant sealing still sees the home dark and it goes silent right after
// the catch-up track.
func TestJoinInProgressCountsForNextElement(t *testing.T) {
	s := livingSession()
	s.online["a"] = true
	s.online["c"] = true // b offline at seal time
	s.OnHeartbeat("a", 0, 40)
	s.OnHeartbeat("c", 0, 60)
	s.EnqueueTrack(trackEl("el1", "spotify:track:X"))
	s.OnReady(1000, "a", "el1")
	s.OnReady(1000, "c", "el1")
	s.OnStarted("a", "el1", 1600)
	s.OnStarted("c", "el1", 1650)
	s.OnHeartbeat("a", 30000, 40)
	s.OnHeartbeat("c", 30000, 60)
	s.EnqueueTrack(trackEl("el2", "spotify:track:Y"))

	// b returns mid-flight: the loop calls JoinInProgress (not OnNodeBack).
	if effs := s.JoinInProgress("b"); len(of[EffLoad](t, effs)) != 1 {
		t.Fatalf("catch-up load expected: %#v", effs)
	}
	s.OnReady(31000, "b", "el1")
	s.OnStarted("b", "el1", 31500) // graduates

	s.OnEnded("a", "el1", "eof")
	s.OnEnded("b", "el1", "eof")
	effs := s.OnEnded("c", "el1", "eof") // advance to el2
	loads := of[EffLoad](t, effs)
	if len(loads) != 3 {
		t.Fatalf("el2 must load to ALL THREE homes (b is online now): %#v", effs)
	}
	if !s.counts("b") {
		t.Fatal("b must be sealed as a participant of el2")
	}
}

// M2 regression: heartbeat positions are per-element. A stale position from
// the previous (longer) track must not satisfy the next track's near-end
// condition — one early errored 'ended' used to cut the new track seconds in.
func TestStaleHeartbeatDoesNotFinishNextTrack(t *testing.T) {
	s := New()
	s.SetPeers([]string{"a", "b"})
	s.online["a"] = true
	s.online["b"] = true
	el1 := trackEl("el1", "spotify:track:LONG")
	el1.DurationMS = 200000
	s.EnqueueTrack(el1)
	s.OnReady(1000, "a", "el1")
	s.OnReady(1000, "b", "el1")
	s.OnStarted("a", "el1", 1600)
	s.OnStarted("b", "el1", 1600)
	s.OnHeartbeat("a", 199500, 40)
	s.OnHeartbeat("b", 199800, 40)
	s.OnEnded("a", "el1", "eof")
	s.OnEnded("b", "el1", "eof")

	el2 := trackEl("el2", "spotify:track:SHORT")
	el2.DurationMS = 120000
	s.EnqueueTrack(el2)
	s.OnReady(3000, "a", "el2")
	s.OnReady(3000, "b", "el2")
	s.OnStarted("a", "el2", 3600)
	s.OnStarted("b", "el2", 3600)

	// b misfires 2s in; a's first el2 heartbeat has not arrived yet.
	if effs := s.OnEnded("b", "el2", "error"); effs != nil {
		t.Fatalf("el2 finished off a's STALE el1 position: %#v", effs)
	}
	if s.State != StatePlaying {
		t.Fatalf("state = %s", s.State)
	}
	// Fresh near-end data still completes the element as designed.
	if effs := s.OnHeartbeat("a", 119500, 40); len(of[EffElementDone](t, effs)) == 0 {
		t.Fatalf("fresh near-end heartbeat must finish the element: %#v", effs)
	}
}
