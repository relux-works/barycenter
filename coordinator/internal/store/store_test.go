package store

import (
	"path/filepath"
	"testing"

	"relux.works/duet/coordinator/internal/session"
)

func openTemp(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "duet.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestSessionRoundTripAndPausedRule(t *testing.T) {
	s := openTemp(t)

	if snap, err := s.LoadSession(1); err != nil || snap != nil {
		t.Fatalf("fresh db: snap=%v err=%v", snap, err)
	}

	el := session.Element{ID: "el1", Kind: session.KindTrack, URI: "spotify:track:X", Target: "both", DurationMS: 200000}
	err := s.SaveSession(1, SessionSnapshot{
		Mode:            session.ModeShared,
		State:           session.StatePlaying, // must come back as paused
		Current:         &el,
		SavedPositionMS: 63000,
		Queue:           []session.Element{{ID: "el2", Kind: session.KindTrack, URI: "spotify:track:Y", Target: "both"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	snap, err := s.LoadSession(1)
	if err != nil {
		t.Fatal(err)
	}
	if snap.State != session.StatePaused {
		t.Fatalf("restart rule: state = %s, want paused (spec 7.2)", snap.State)
	}
	if snap.Current == nil || snap.Current.ID != "el1" || snap.SavedPositionMS != 63000 {
		t.Fatalf("current lost: %+v", snap)
	}
	if len(snap.Queue) != 1 || snap.Queue[0].ID != "el2" {
		t.Fatalf("queue lost: %+v", snap.Queue)
	}
}

func TestSettings(t *testing.T) {
	s := openTemp(t)
	if v, err := s.GetSetting("offset_a"); err != nil || v != "" {
		t.Fatalf("missing key: %q %v", v, err)
	}
	if err := s.SetSetting("offset_a", "250"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetSetting("offset_a", "300"); err != nil {
		t.Fatal(err) // upsert
	}
	if v, _ := s.GetSetting("offset_a"); v != "300" {
		t.Fatalf("v = %q", v)
	}
}

func TestElementsJournal(t *testing.T) {
	s := openTemp(t)
	el := session.Element{ID: "el1", Kind: session.KindTrack, URI: "u", Target: "both", CreatedAt: 111}
	if err := s.InsertElement(el); err != nil {
		t.Fatal(err)
	}
	if err := s.MarkElementDone("el1", "eof", 222); err != nil {
		t.Fatal(err)
	}
}

func TestMediaLifecycle(t *testing.T) {
	s := openTemp(t)
	m := MediaRecord{ID: "m1", TGFileID: "tg1", CreatedAt: 100, ExpiresAt: 200, Status: "processing"}
	if err := s.InsertMedia(m); err != nil {
		t.Fatal(err)
	}
	m.Status = "ready"
	m.DurationMS = 12400
	m.PathWAV = "/tmp/m1.wav"
	m.LoudnormJSON = `{"input_i":"-23.0"}`
	if err := s.UpdateMedia(m); err != nil {
		t.Fatal(err)
	}
	got, err := s.GetMedia("m1")
	if err != nil || got == nil {
		t.Fatalf("get: %v %v", got, err)
	}
	if got.DurationMS != 12400 || got.Status != "ready" {
		t.Fatalf("got %+v", got)
	}

	expired, err := s.ExpiredMedia(300)
	if err != nil || len(expired) != 1 {
		t.Fatalf("expired: %v %v", expired, err)
	}
	if err := s.MarkMediaDeleted("m1"); err != nil {
		t.Fatal(err)
	}
	expired, _ = s.ExpiredMedia(300)
	if len(expired) != 0 {
		t.Fatalf("deleted media must leave the sweep, got %v", expired)
	}

	if missing, err := s.GetMedia("nope"); err != nil || missing != nil {
		t.Fatalf("missing media: %v %v", missing, err)
	}
}
