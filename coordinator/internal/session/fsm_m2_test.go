package session

import (
	"strings"
	"testing"

	"relux.works/duet/coordinator/internal/protocol"
)

func trioSession() *Session {
	s := New()
	s.SetPeers([]string{"a", "b", "c"})
	for _, n := range s.Peers {
		s.online[n] = true
	}
	return s
}

// M2 acceptance: three pulsars run the full synchronized start cycle;
// T uses the worst RTT of the orbit, desync is max-min across all starts.
func TestTrioStartCycleAndDesync(t *testing.T) {
	s := trioSession()
	s.OnHeartbeat("a", 0, 40)
	s.OnHeartbeat("b", 0, 60)
	s.OnHeartbeat("c", 0, 90)

	effs := s.EnqueueTrack(trackEl("el1", "spotify:track:X"))
	if len(of[EffLoad](t, effs)) != 3 {
		t.Fatalf("load to all three, got %#v", effs)
	}
	if s.OnReady(1000, "a", "el1") != nil || s.OnReady(1000, "b", "el1") != nil {
		t.Fatal("partial ready must produce nothing")
	}
	effs = s.OnReady(1000, "c", "el1")
	resumes := of[EffResumeAt](t, effs)
	if len(resumes) != 3 {
		t.Fatalf("resume_at to all three, got %#v", effs)
	}
	// T = 1000 + 2*max(40,60,90) + 500
	if resumes[0].TCoordMS != 1000+180+500 {
		t.Fatalf("T = %d", resumes[0].TCoordMS)
	}
	s.OnStarted("a", "el1", 1690)
	s.OnStarted("b", "el1", 1710)
	effs = s.OnStarted("c", "el1", 1702)
	if d := one[EffLogDesync](t, effs); d.DeltaMS != 20 { // 1710-1690
		t.Fatalf("desync = %d, want 20", d.DeltaMS)
	}
	if s.State != StatePlaying {
		t.Fatalf("state = %s", s.State)
	}

	// Ended: two report eof, the third hangs far from the end -> not done.
	s.OnEnded("a", "el1", "eof")
	if effs := s.OnEnded("b", "el1", "eof"); effs != nil {
		t.Fatalf("c is nowhere near the end: %#v", effs)
	}
	// The laggard reaches near-end via heartbeat -> element completes.
	effs = s.OnHeartbeat("c", 199_300, 90)
	one[EffElementDone](t, effs)
}

// Personal voice in a trio: play to the addressee, wait to every other peer.
func TestTrioPersonalVoiceWaits(t *testing.T) {
	s := trioSession()
	adv := s.EnqueueVoice(voiceEl("v1", "b", 9000))
	if s.State != StateVoice {
		t.Fatalf("state = %s", s.State)
	}
	plays := of[EffPlayVoice](t, adv)
	waits := of[EffWait](t, adv)
	if len(plays) != 1 || plays[0].To != protocol.NodeID("b") {
		t.Fatalf("play to b only: %#v", adv)
	}
	if len(waits) != 2 {
		t.Fatalf("waits to a and c: %#v", adv)
	}
	s.OnVoiceEnded("b", "v1")
	s.OnWaitEnded("a", "v1")
	if s.State != StateVoice {
		t.Fatal("c has not finished waiting")
	}
	s.OnWaitEnded("c", "v1")
	if s.State != StateIdle {
		t.Fatalf("state = %s", s.State)
	}
}

// One of three offline -> the whole orbit parks; survivors get the pause.
func TestTrioOfflineParksAll(t *testing.T) {
	s := trioSession()
	s.OnHeartbeat("a", 0, 40)
	s.OnHeartbeat("b", 0, 60)
	s.OnHeartbeat("c", 0, 90)
	s.EnqueueTrack(trackEl("el1", "spotify:track:X"))
	for _, n := range []string{"a", "b", "c"} {
		s.OnReady(1000, protocol.NodeID(n), "el1")
	}
	for _, n := range []string{"a", "b", "c"} {
		s.OnStarted(protocol.NodeID(n), "el1", 1700)
	}
	s.OnHeartbeat("a", 63_000, 40)
	s.OnHeartbeat("b", 62_950, 60)
	s.OnHeartbeat("c", 63_010, 90)

	effs := s.OnNodeOffline(63_100, "b")
	if s.State != StateDegraded {
		t.Fatalf("state = %s", s.State)
	}
	pauses := of[EffPause](t, effs)
	if len(pauses) != 2 {
		t.Fatalf("survivors a and c must pause: %#v", effs)
	}
	if s.SavedPositionMS != 63_000 {
		t.Fatalf("frozen at min survivor position, got %d", s.SavedPositionMS)
	}
}

// Revoking the dead slot releases the orbit: remaining homes resume.
func TestRemovePeerUnparks(t *testing.T) {
	s := trioSession()
	s.online["c"] = false
	effs := s.EnqueueTrack(trackEl("el1", "spotify:track:X"))
	n := one[EffNotify](t, effs)
	if !strings.Contains(n.Text, "подождёт") || s.State != StateDegraded {
		t.Fatalf("offline gate: %q %s", n.Text, s.State)
	}
	effs = s.RemovePeer(2000, "c")
	loads := of[EffLoad](t, effs)
	if len(loads) != 2 || s.State != StateLoading {
		t.Fatalf("remaining pair must start: %#v state=%s", effs, s.State)
	}
}

// A removed peer no longer blocks the ready barrier mid-load.
func TestRemovePeerUnblocksReady(t *testing.T) {
	s := trioSession()
	s.OnHeartbeat("a", 0, 40)
	s.OnHeartbeat("b", 0, 60)
	s.EnqueueTrack(trackEl("el1", "spotify:track:X"))
	s.OnReady(1000, "a", "el1")
	s.OnReady(1000, "b", "el1")
	if s.State != StateLoading {
		t.Fatalf("still waiting for c: %s", s.State)
	}
	effs := s.RemovePeer(1500, "c")
	if len(of[EffResumeAt](t, effs)) != 2 || s.State != StateArmed {
		t.Fatalf("pair must arm after c is gone: %#v %s", effs, s.State)
	}
}
