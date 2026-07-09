package session

import "testing"

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
