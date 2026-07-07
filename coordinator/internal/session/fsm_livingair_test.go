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
