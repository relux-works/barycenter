package session

import (
	"testing"

	"relux.works/duet/coordinator/internal/protocol"
)

// strictPair: a plain (non-linked) two-home orbit mid-play — the Katya case.
func strictPair(t *testing.T) *Session {
	t.Helper()
	s := New()
	s.SetPeers([]string{"a", "b"})
	s.online["a"] = true
	s.online["b"] = true
	s.OnHeartbeat("a", 0, 40)
	s.OnHeartbeat("b", 0, 60)
	s.EnqueueTrack(trackEl("el1", "spotify:track:X"))
	s.OnReady(1000, "a", "el1")
	s.OnReady(1000, "b", "el1")
	s.OnStarted("a", "el1", 1600)
	s.OnStarted("b", "el1", 1650)
	if s.State != StatePlaying {
		t.Fatalf("precondition: playing, got %s", s.State)
	}
	return s
}

// A personal pause must NOT stop the air for the other home — that is the
// whole point (Timur's ghost-resume report, 2026-07-10).
func TestPersonalPauseKeepsStrictAirPlaying(t *testing.T) {
	s := strictPair(t)
	effs := s.OnUserPause(5000, "b")
	if s.State != StatePlaying {
		t.Fatalf("air must keep playing, got %s", s.State)
	}
	if len(of[EffPause](t, effs)) != 0 {
		t.Fatalf("nobody gets paused by a personal pause: %#v", effs)
	}
	// The element can finish without the paused home.
	s.OnHeartbeat("a", 200000, 40)
	if e := s.OnEnded("a", "el1", "eof"); len(of[EffElementDone](t, e)) == 0 {
		t.Fatalf("track must end without the paused home: %#v", e)
	}
}

// The paused home is excluded from subsequent elements until it resumes.
func TestPersonalPauseExcludesFromNextElement(t *testing.T) {
	s := strictPair(t)
	s.OnUserPause(5000, "b")
	s.EnqueueTrack(trackEl("el2", "spotify:track:Y"))
	effs := s.OnEnded("a", "el1", "eof") // advance to el2
	loads := of[EffLoad](t, effs)
	if len(loads) != 1 || loads[0].To != "a" {
		t.Fatalf("el2 must load only to the active home: %#v", effs)
	}
	if s.counts("b") {
		t.Fatal("paused home must not be sealed into el2")
	}
}

// Play in Spotify returns the home to the air via the living-air catch-up.
func TestPersonalResumeCatchesUp(t *testing.T) {
	s := strictPair(t)
	s.OnUserPause(5000, "b")
	s.EnqueueTrack(trackEl("el2", "spotify:track:Y"))
	s.OnEnded("a", "el1", "eof")
	s.OnReady(6000, "a", "el2")
	s.OnStarted("a", "el2", 6600)
	s.OnHeartbeat("a", 30000, 40)

	effs := s.OnUserResume("b")
	loads := of[EffLoad](t, effs)
	if len(loads) != 1 || loads[0].To != "b" || loads[0].PositionMS != 30000 {
		t.Fatalf("resume must catch up at the live position: %#v", effs)
	}
	// Solo ready -> solo resume_at, then graduation into the barriers.
	res := of[EffResumeAt](t, s.OnReady(31000, "b", "el2"))
	if len(res) != 1 || res[0].To != "b" {
		t.Fatalf("catch-up arms alone: %#v", res)
	}
	s.OnStarted("b", "el2", 31600)
	if !s.counts("b") {
		t.Fatal("resumed home must graduate into participants")
	}
}

// The LAST active home pausing personally IS the air pausing.
func TestLastActiveHomePauseBecomesGlobal(t *testing.T) {
	s := strictPair(t)
	s.OnUserPause(5000, "b")
	effs := s.OnUserPause(6000, "a")
	if s.State != StatePaused {
		t.Fatalf("last-man pause must pause the air, got %s", s.State)
	}
	if len(of[EffPause](t, effs)) == 0 {
		t.Fatalf("global pause must pause the peers: %#v", effs)
	}
	if s.pausedLocally["a"] {
		t.Fatal("the global fallback must not mark a personal pause")
	}
}

// A pause on the LOADING barrier releases the survivors (H1/H2 class).
func TestPersonalPauseDuringLoadingUnblocksBarrier(t *testing.T) {
	s := New()
	s.SetPeers([]string{"a", "b"})
	s.online["a"] = true
	s.online["b"] = true
	s.OnHeartbeat("a", 0, 40)
	s.OnHeartbeat("b", 0, 60)
	s.EnqueueTrack(trackEl("el1", "spotify:track:X"))
	s.OnReady(1000, "a", "el1")

	effs := s.OnUserPause(1500, "b") // b was the only not-ready home
	if s.State != StateArmed {
		t.Fatalf("survivor is ready — must arm, got %s", s.State)
	}
	if len(of[EffResumeAt](t, effs)) != 1 {
		t.Fatalf("resume_at to the survivor: %#v", effs)
	}
	// The paused home resumes onto the loading barrier of a LATER element.
	s.OnStarted("a", "el1", 2100)
	s.EnqueueTrack(trackEl("el2", "spotify:track:Y"))
	s.OnHeartbeat("a", 200000, 40)
	s.OnEnded("a", "el1", "eof") // advance: el2 loads to a only
	if s.State != StateLoading {
		t.Fatalf("el2 loading, got %s", s.State)
	}
	effs = s.OnUserResume("b")
	loads := of[EffLoad](t, effs)
	if len(loads) != 1 || loads[0].To != "b" {
		t.Fatalf("resume during loading re-deals the element: %#v", effs)
	}
	if !s.counts("b") {
		t.Fatal("resumed home joins the loading barrier")
	}
}

// A liveness edge (hub race replay) must not yank a paused home back in.
func TestLivenessEdgeDoesNotResumePausedHome(t *testing.T) {
	s := strictPair(t)
	s.OnUserPause(5000, "b")
	if effs := s.JoinInProgress("b"); effs != nil {
		t.Fatalf("join must not override a personal pause: %#v", effs)
	}
	// Going OFFLINE ends the pause (local flag dies with the node anyway).
	s.OnNodeOffline(6000, "b")
	if s.pausedLocally["b"] {
		t.Fatal("offline must clear the personal pause")
	}
}

// Living air: pausing the ONLY home of one side keeps the air playing —
// unlike an offline drop, a paused home does not darken its side.
func TestPersonalPauseOfWholeSideKeepsGroupAir(t *testing.T) {
	s := livingSession() // a,b side L; c side R
	s.online["a"] = true
	s.online["b"] = true
	s.online["c"] = true
	s.OnHeartbeat("a", 0, 40)
	s.OnHeartbeat("b", 0, 40)
	s.OnHeartbeat("c", 0, 60)
	s.EnqueueTrack(trackEl("el1", "spotify:track:X"))
	for _, n := range []protocol.NodeID{"a", "b", "c"} {
		s.OnReady(1000, n, "el1")
	}
	for _, n := range []protocol.NodeID{"a", "b", "c"} {
		s.OnStarted(n, "el1", 1600)
	}
	s.OnUserPause(5000, "c")
	if s.State != StatePlaying {
		t.Fatalf("paused side is still ONLINE — air continues, got %s", s.State)
	}
	s.EnqueueTrack(trackEl("el2", "spotify:track:Y"))
	s.OnEnded("a", "el1", "eof")
	effs := s.OnEnded("b", "el1", "eof")
	loads := of[EffLoad](t, effs)
	if len(loads) != 2 {
		t.Fatalf("el2 deals to the two active homes only: %#v", effs)
	}
}
