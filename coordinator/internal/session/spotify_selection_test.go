package session

import (
	"testing"

	"relux.works/duet/coordinator/internal/protocol"
)

func TestPlayNowAtLoadsSelectedTrackFromAudiblePosition(t *testing.T) {
	s := playingSession(t)
	s.EnqueueTrack(trackEl("el2", "spotify:track:Y"))

	effs := s.CmdPlayNowAt(trackEl("elX", "spotify:track:Z"), 63_000)
	loads := of[EffLoad](t, effs)
	if len(loads) != 2 {
		t.Fatalf("playnow-at must load every home, got %#v", effs)
	}
	for _, load := range loads {
		if load.ElementID != "elX" || load.PositionMS != 63_000 {
			t.Fatalf("load = %#v, want selected track at 63000", load)
		}
	}
	if s.Queue[0].ID != "el2" {
		t.Fatalf("old queue preserved after the selected track: %#v", s.Queue)
	}
}

func TestAdoptPlayingNeverPausesLeaderAndFollowersCatchUpAtFuturePosition(t *testing.T) {
	s := playingSession(t)
	s.EnqueueTrack(trackEl("el2", "spotify:track:Y"))

	effs := s.CmdAdoptPlaying(10_000, protocol.NodeA,
		trackEl("elX", "spotify:track:Z"), 12_000)
	if pauses := of[EffPause](t, effs); len(pauses) != 0 {
		t.Fatalf("the already-playing leader must never be paused: %#v", effs)
	}
	loads := of[EffLoad](t, effs)
	if len(loads) != 2 || !loads[0].AdoptPlaying || loads[1].AdoptPlaying {
		t.Fatalf("leader adopts, follower loads normally: %#v", effs)
	}

	// Leader is pre-ready. Once b loaded, only b receives resume_at and it
	// seeks to where a will be at T (12s + 2.620s elapsed).
	effs = s.OnReady(12_000, protocol.NodeB, "elX")
	resumes := of[EffResumeAt](t, effs)
	if len(resumes) != 1 || resumes[0].To != protocol.NodeB {
		t.Fatalf("only the follower must resume: %#v", effs)
	}
	if resumes[0].PositionMS == nil || *resumes[0].PositionMS != 14_620 {
		t.Fatalf("catch-up position = %v, want 14620", resumes[0].PositionMS)
	}
	if s.State != StateArmed {
		t.Fatalf("state = %s", s.State)
	}

	effs = s.OnStarted(protocol.NodeB, "elX", 12_630)
	if s.State != StatePlaying {
		t.Fatalf("state = %s", s.State)
	}
	if d := one[EffLogDesync](t, effs); d.DeltaMS != 10 {
		t.Fatalf("follower skew = %d, want 10", d.DeltaMS)
	}
	if s.Queue[0].ID != "el2" {
		t.Fatalf("old queue must survive adoption: %#v", s.Queue)
	}
}

func TestAdoptTimeoutKeepsLeaderPlayingAndLetsFollowerJoinLate(t *testing.T) {
	s := playingSession(t)
	s.CmdAdoptPlaying(10_000, protocol.NodeA,
		trackEl("elX", "spotify:track:Z"), 12_000)

	first := s.OnReadyTimeoutAt(18_000, "elX")
	if retry := one[EffLoad](t, first); retry.To != protocol.NodeB {
		t.Fatalf("retry = %#v", retry)
	}
	second := s.OnReadyTimeoutAt(26_000, "elX")
	if len(of[EffElementDone](t, second)) != 0 || s.State != StatePlaying {
		t.Fatalf("lagging follower must not kill leader: %#v state=%s", second, s.State)
	}
	one[EffNotify](t, second)

	late := s.OnReady(27_000, protocol.NodeB, "elX")
	resume := one[EffResumeAt](t, late)
	if resume.To != protocol.NodeB || resume.PositionMS == nil {
		t.Fatalf("late follower catch-up = %#v", late)
	}
}

func TestAdoptFollowerLoadFailureDoesNotStopLeader(t *testing.T) {
	s := playingSession(t)
	s.CmdAdoptPlaying(10_000, protocol.NodeA,
		trackEl("elX", "spotify:track:Z"), 12_000)

	effs := s.OnNodeErrorAt(11_000, protocol.NodeB, "load_failed", "elX")
	if len(of[EffPause](t, effs)) != 0 || len(of[EffElementDone](t, effs)) != 0 {
		t.Fatalf("follower failure must not stop/skip the leader: %#v", effs)
	}
	retry := one[EffLoad](t, effs)
	if retry.To != protocol.NodeB {
		t.Fatalf("only follower retries: %#v", effs)
	}
	if s.State != StatePlaying {
		t.Fatalf("leader should be playing, state=%s", s.State)
	}
}
