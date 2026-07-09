package session

import (
	"strings"
	"testing"

	"relux.works/duet/coordinator/internal/protocol"
)

func trackEl(id, uri string) Element {
	return Element{ID: id, Kind: KindTrack, URI: uri, Target: "both", DurationMS: 200_000}
}

func voiceEl(id, target string, durMS int64) Element {
	return Element{ID: id, Kind: KindVoice, MediaID: "m_" + id, Target: target, DurationMS: durMS}
}

func of[T any](t *testing.T, effs []Effect) []T {
	t.Helper()
	var out []T
	for _, e := range effs {
		if v, ok := e.(T); ok {
			out = append(out, v)
		}
	}
	return out
}

func one[T any](t *testing.T, effs []Effect) T {
	t.Helper()
	found := of[T](t, effs)
	if len(found) != 1 {
		t.Fatalf("want exactly 1 effect of %T, got %d in %#v", *new(T), len(found), effs)
	}
	return found[0]
}

func goOnline(s *Session) {
	s.online[protocol.NodeA] = true
	s.online[protocol.NodeB] = true
}

// drive a fresh session to PLAYING with element el1
func playingSession(t *testing.T) *Session {
	t.Helper()
	s := New()
	goOnline(s)
	s.OnHeartbeat(protocol.NodeA, 0, 40)
	s.OnHeartbeat(protocol.NodeB, 0, 60)
	effs := s.EnqueueTrack(trackEl("el1", "spotify:track:X"))
	if len(of[EffLoad](t, effs)) != 2 {
		t.Fatalf("enqueue on idle must load both, got %#v", effs)
	}
	if s.State != StateLoading {
		t.Fatalf("state = %s", s.State)
	}
	if effs := s.OnReady(1000, protocol.NodeA, "el1"); effs != nil {
		t.Fatalf("first ready must produce nothing, got %#v", effs)
	}
	effs = s.OnReady(1000, protocol.NodeB, "el1")
	resumes := of[EffResumeAt](t, effs)
	if len(resumes) != 2 {
		t.Fatalf("both nodes must get resume_at, got %#v", effs)
	}
	// T = now + 2*max(rtt 40,60) + margin 500 = 1000 + 120 + 500
	if resumes[0].TCoordMS != 1620 {
		t.Fatalf("T = %d, want 1620", resumes[0].TCoordMS)
	}
	if s.State != StateArmed {
		t.Fatalf("state = %s", s.State)
	}
	s.OnStarted(protocol.NodeA, "el1", 1620)
	effs = s.OnStarted(protocol.NodeB, "el1", 1650)
	if d := one[EffLogDesync](t, effs); d.DeltaMS != 30 {
		t.Fatalf("desync = %d, want 30", d.DeltaMS)
	}
	if s.State != StatePlaying {
		t.Fatalf("state = %s", s.State)
	}
	return s
}

func TestTrackHappyPath(t *testing.T) {
	s := playingSession(t)
	if effs := s.OnEnded(protocol.NodeA, "el1", "eof"); effs != nil {
		t.Fatalf("single ended must wait for the other node, got %#v", effs)
	}
	effs := s.OnEnded(protocol.NodeB, "el1", "eof")
	if done := one[EffElementDone](t, effs); done.Status != "eof" || done.Element.ID != "el1" {
		t.Fatalf("done = %#v", done)
	}
	if s.State != StateIdle || s.Current != nil {
		t.Fatalf("after last element: state=%s current=%v", s.State, s.Current)
	}
}

func TestEndedOneNodeOtherNearEnd(t *testing.T) {
	s := playingSession(t)
	s.OnHeartbeat(protocol.NodeB, 199_200, 60) // duration 200000, within 1s of end
	effs := s.OnEnded(protocol.NodeA, "el1", "eof")
	one[EffElementDone](t, effs)
	if s.State != StateIdle {
		t.Fatalf("state = %s", s.State)
	}
}

func TestEndedCompletesViaHeartbeat(t *testing.T) {
	s := playingSession(t)
	if effs := s.OnEnded(protocol.NodeA, "el1", "eof"); effs != nil {
		t.Fatalf("premature advance: %#v", effs)
	}
	effs := s.OnHeartbeat(protocol.NodeB, 199_200, 60)
	one[EffElementDone](t, effs)
}

func TestVoiceBothAfterCurrentTrack(t *testing.T) {
	s := playingSession(t)
	s.EnqueueTrack(trackEl("el2", "spotify:track:Y"))
	s.EnqueueVoice(voiceEl("v1", "both", 8000))
	if s.Queue[0].ID != "v1" || s.Queue[1].ID != "el2" {
		t.Fatalf("voice must cut the line: %#v", s.Queue)
	}
	s.EnqueueVoice(voiceEl("v2", "both", 5000))
	if s.Queue[0].ID != "v1" || s.Queue[1].ID != "v2" || s.Queue[2].ID != "el2" {
		t.Fatalf("second voice keeps arrival order: %#v", s.Queue)
	}

	s.OnEnded(protocol.NodeA, "el1", "eof")
	effs := s.OnEnded(protocol.NodeB, "el1", "eof")
	plays := of[EffPlayVoice](t, effs)
	if len(plays) != 2 || s.State != StateVoice {
		t.Fatalf("voice to both expected, got %#v state=%s", effs, s.State)
	}
	if len(of[EffWait](t, effs)) != 0 {
		t.Fatalf("no wait for target=both")
	}
	if effs := s.OnVoiceEnded(protocol.NodeA, "v1"); effs != nil {
		t.Fatalf("waiting second node, got %#v", effs)
	}
	effs = s.OnVoiceEnded(protocol.NodeB, "v1")
	if s.State != StateVoice || s.Current.ID != "v2" {
		t.Fatalf("v2 must follow v1: state=%s current=%#v", s.State, s.Current)
	}
	_ = effs
}

func TestVoicePersonal(t *testing.T) {
	s := playingSession(t)
	s.EnqueueVoice(voiceEl("v1", "b", 12_400)) // "лично" for B, sender A waits
	s.OnEnded(protocol.NodeA, "el1", "eof")
	effs := s.OnEnded(protocol.NodeB, "el1", "eof")
	play := one[EffPlayVoice](t, effs)
	if play.To != protocol.NodeB {
		t.Fatalf("voice must go to b, got %v", play.To)
	}
	wait := one[EffWait](t, effs)
	if wait.To != protocol.NodeA || wait.DurationMS != 12_400 {
		t.Fatalf("wait = %#v", wait)
	}
	s.OnVoiceEnded(protocol.NodeB, "v1")
	effs = s.OnWaitEnded(protocol.NodeA, "v1")
	if s.State != StateIdle {
		t.Fatalf("voice element must finish after voice_ended + wait_ended, state=%s", s.State)
	}
	_ = effs
}

func TestReadyTimeoutRetryThenSkip(t *testing.T) {
	s := New()
	goOnline(s)
	s.EnqueueTrack(trackEl("el1", "spotify:track:X"))
	s.EnqueueTrack(trackEl("el2", "spotify:track:Y"))
	s.OnReady(1000, protocol.NodeA, "el1")

	effs := s.OnReadyTimeout("el1")
	loads := of[EffLoad](t, effs)
	if len(loads) != 1 || loads[0].To != protocol.NodeB {
		t.Fatalf("retry must reload only the missing node, got %#v", effs)
	}
	one[EffArmReadyTimer](t, effs)

	effs = s.OnReadyTimeout("el1")
	one[EffNotify](t, effs)
	if done := one[EffElementDone](t, effs); done.Status != "error" {
		t.Fatalf("done = %#v", done)
	}
	if s.Current == nil || s.Current.ID != "el2" || s.State != StateLoading {
		t.Fatalf("must move to el2, current=%#v state=%s", s.Current, s.State)
	}
}

func TestVoicesAreFIFOByAcceptanceTimeNotProcessingCompletion(t *testing.T) {
	s := playingSession(t)
	later := voiceEl("v_later", "both", 2_000)
	later.CreatedAt = 2_000
	earlier := voiceEl("v_earlier", "both", 2_000)
	earlier.CreatedAt = 1_000

	// Simulate ffmpeg finishing the later message first.
	s.EnqueueVoice(later)
	s.EnqueueVoice(earlier)
	if len(s.Queue) < 2 || s.Queue[0].ID != "v_earlier" || s.Queue[1].ID != "v_later" {
		t.Fatalf("voice FIFO follows Telegram acceptance time: %#v", s.Queue)
	}
}

func TestPauseResume(t *testing.T) {
	s := playingSession(t)
	s.OnHeartbeat(protocol.NodeA, 63_012, 40)
	s.OnHeartbeat(protocol.NodeB, 62_998, 60)
	effs := s.CmdPause()
	if len(of[EffPause](t, effs)) != 2 || s.State != StatePaused {
		t.Fatalf("pause both expected: %#v state=%s", effs, s.State)
	}
	if s.SavedPositionMS != 62_998 {
		t.Fatalf("saved position must be the safe minimum, got %d", s.SavedPositionMS)
	}
	effs = s.CmdResume()
	loads := of[EffLoad](t, effs)
	if len(loads) != 2 || loads[0].PositionMS != 62_998 {
		t.Fatalf("resume loads from saved position, got %#v", effs)
	}
	if s.State != StateLoading {
		t.Fatalf("state = %s", s.State)
	}
}

func TestSkip(t *testing.T) {
	s := playingSession(t)
	s.EnqueueTrack(trackEl("el2", "spotify:track:Y"))
	effs := s.CmdSkip()
	if done := one[EffElementDone](t, effs); done.Status != "skipped" {
		t.Fatalf("done = %#v", done)
	}
	pauses := of[EffPause](t, effs)
	if len(pauses) != 2 || pauses[0].FadeMS != 300 {
		t.Fatalf("skip pauses with fade 300: %#v", effs)
	}
	if s.Current == nil || s.Current.ID != "el2" {
		t.Fatalf("current = %#v", s.Current)
	}
}

func TestPlayNow(t *testing.T) {
	s := playingSession(t)
	s.EnqueueTrack(trackEl("el2", "spotify:track:Y"))
	effs := s.CmdPlayNow(trackEl("elX", "spotify:track:Z"))
	loads := of[EffLoad](t, effs)
	if len(loads) != 2 || loads[0].ElementID != "elX" {
		t.Fatalf("playnow must load the new element, got %#v", effs)
	}
	if s.Queue[0].ID != "el2" {
		t.Fatalf("old queue preserved after the injected head: %#v", s.Queue)
	}
}

func TestOfflineDegradedBackResume(t *testing.T) {
	s := playingSession(t)
	s.online[protocol.NodeA] = true
	s.online[protocol.NodeB] = true
	s.OnHeartbeat(protocol.NodeA, 63_012, 40)
	s.OnHeartbeat(protocol.NodeB, 62_998, 60)

	effs := s.OnNodeOffline(63_100, protocol.NodeB)
	p := one[EffPause](t, effs)
	if p.To != protocol.NodeA {
		t.Fatalf("alive node must be paused, got %#v", p)
	}
	one[EffNotify](t, effs)
	if s.State != StateDegraded {
		t.Fatalf("state = %s", s.State)
	}
	if s.SavedPositionMS != 63_012 {
		t.Fatalf("saved position from the alive node, got %d", s.SavedPositionMS)
	}

	effs = s.OnNodeBack(protocol.NodeB)
	one[EffNotify](t, effs)
	if s.State != StatePaused {
		t.Fatalf("state = %s", s.State)
	}

	effs = s.CmdResume()
	loads := of[EffLoad](t, effs)
	if len(loads) != 2 || loads[0].PositionMS != 63_012 {
		t.Fatalf("resume from saved position: %#v", effs)
	}
}

func TestTrackUnavailableSkips(t *testing.T) {
	s := New()
	goOnline(s)
	s.EnqueueTrack(trackEl("el1", "spotify:track:X"))
	s.EnqueueTrack(trackEl("el2", "spotify:track:Y"))
	effs := s.OnNodeError(protocol.NodeB, "track_unavailable", "el1")
	one[EffNotify](t, effs)
	if s.Current == nil || s.Current.ID != "el2" {
		t.Fatalf("must skip to el2, current=%#v", s.Current)
	}
}

func TestLibrespotRestartReloadsCurrent(t *testing.T) {
	s := playingSession(t)
	s.OnHeartbeat(protocol.NodeA, 30_000, 40)
	s.OnHeartbeat(protocol.NodeB, 30_100, 60)
	effs := s.OnNodeError(protocol.NodeA, "librespot_restart", "el1")
	loads := of[EffLoad](t, effs)
	if len(loads) != 2 || loads[0].PositionMS != 30_000 {
		t.Fatalf("restart must reload current from min position: %#v", effs)
	}
	if s.State != StateLoading {
		t.Fatalf("state = %s", s.State)
	}
}

func TestIdempotencyStaleElementIgnored(t *testing.T) {
	s := playingSession(t)
	if effs := s.OnReady(1, protocol.NodeA, "el_stale"); effs != nil {
		t.Fatalf("stale ready must be ignored, got %#v", effs)
	}
	if effs := s.OnEnded(protocol.NodeA, "el_stale", "eof"); effs != nil {
		t.Fatalf("stale ended must be ignored, got %#v", effs)
	}
}

func TestModeSwitchRoundTrip(t *testing.T) {
	s := playingSession(t)
	s.OnHeartbeat(protocol.NodeA, 30_000, 40)
	s.OnHeartbeat(protocol.NodeB, 30_100, 60)
	effs := s.SetModeSolo()
	if len(of[EffStop](t, effs)) != 2 || len(of[EffSetMode](t, effs)) != 2 {
		t.Fatalf("solo must stop both and set mode: %#v", effs)
	}
	if s.Mode != ModeSolo || s.SavedPositionMS != 30_000 {
		t.Fatalf("mode=%s saved=%d", s.Mode, s.SavedPositionMS)
	}
	effs = s.SetModeShared()
	loads := of[EffLoad](t, effs)
	if len(loads) != 2 || loads[0].PositionMS != 30_000 {
		t.Fatalf("shared must resume the interrupted element: %#v", effs)
	}
}

func TestSyncRestartsFromLivePosition(t *testing.T) {
	s := playingSession(t)
	s.OnHeartbeat(protocol.NodeA, 45_000, 40)
	s.OnHeartbeat(protocol.NodeB, 45_150, 60)
	effs := s.CmdSync()
	loads := of[EffLoad](t, effs)
	if len(loads) != 2 || loads[0].PositionMS != 45_000 || loads[0].ElementID != "el1" {
		t.Fatalf("sync must reload current from min position: %#v", effs)
	}
	if s.State != StateLoading {
		t.Fatalf("state = %s", s.State)
	}
}

// U10: playlist base layer with inserts cutting in and the air returning to
// the next playlist track after the interruption.
func TestPlaylistWithInsertsAndReturn(t *testing.T) {
	s := New()
	goOnline(s)
	s.OnHeartbeat(protocol.NodeA, 0, 40)
	s.OnHeartbeat(protocol.NodeB, 0, 60)

	effs := s.SetPlaylist("spotify:playlist:37i9dQZF1DXcBW", "Наш вечер", []string{
		"spotify:track:T1", "spotify:track:T2", "spotify:track:T3",
	})
	one[EffNotify](t, effs)
	loads := of[EffLoad](t, effs)
	if len(loads) != 2 || loads[0].URI != "spotify:track:T1" {
		t.Fatalf("playlist must start with T1: %#v", effs)
	}
	firstID := s.Current.ID

	run := func(elID string) { // ready+started both -> playing
		s.OnReady(1000, protocol.NodeA, elID)
		s.OnReady(1000, protocol.NodeB, elID)
		s.OnStarted(protocol.NodeA, elID, 1500)
		s.OnStarted(protocol.NodeB, elID, 1510)
	}
	end := func(elID string) []Effect {
		s.OnEnded(protocol.NodeA, elID, "eof")
		return s.OnEnded(protocol.NodeB, elID, "eof")
	}
	run(firstID)

	// Mid-track: a voice and a single track arrive (insert layer).
	s.EnqueueVoice(voiceEl("v1", "both", 8000))
	s.EnqueueTrack(trackEl("single1", "spotify:track:SINGLE"))
	if s.Playlist.Cursor != 1 {
		t.Fatalf("cursor must already point past T1, got %d", s.Playlist.Cursor)
	}

	// T1 over -> voice first (queue discipline), playlist paused underneath.
	effs = end(firstID)
	if len(of[EffPlayVoice](t, effs)) != 2 {
		t.Fatalf("voice must cut in after the current track: %#v", effs)
	}
	s.OnVoiceEnded(protocol.NodeA, "v1")
	effs = s.OnVoiceEnded(protocol.NodeB, "v1")
	loads = of[EffLoad](t, effs)
	if len(loads) != 2 || loads[0].URI != "spotify:track:SINGLE" {
		t.Fatalf("inserted single plays after the voice: %#v", effs)
	}
	singleID := s.Current.ID
	run(singleID)

	// Single over -> back into the playlist at T2 (next after the cut).
	effs = end(singleID)
	loads = of[EffLoad](t, effs)
	if len(loads) != 2 || loads[0].URI != "spotify:track:T2" {
		t.Fatalf("air must return to the playlist at T2: %#v", effs)
	}
	t2ID := s.Current.ID
	run(t2ID)
	effs = end(t2ID)
	loads = of[EffLoad](t, effs)
	if len(loads) != 2 || loads[0].URI != "spotify:track:T3" {
		t.Fatalf("then T3: %#v", effs)
	}
	t3ID := s.Current.ID
	run(t3ID)

	// Playlist exhausted -> idle + human notice (no silent dead air).
	effs = end(t3ID)
	if s.State != StateIdle {
		t.Fatalf("state = %s", s.State)
	}
	n := one[EffNotify](t, effs)
	if !strings.Contains(n.Text, "доиграл до конца") {
		t.Fatalf("notice = %q", n.Text)
	}
}

func TestPlaylistReplace(t *testing.T) {
	s := New()
	goOnline(s)
	s.SetPlaylist("spotify:playlist:AAA00000", "Первый", []string{"spotify:track:A1"})
	effs := s.SetPlaylist("spotify:playlist:BBB00000", "Второй", []string{"spotify:track:B1", "spotify:track:B2"})
	one[EffNotify](t, effs)
	if s.Playlist.Title != "Второй" || s.Playlist.Cursor != 0 && s.Playlist.Cursor != 1 {
		t.Fatalf("playlist = %+v", s.Playlist)
	}
}

func TestEmptyQueueNoticeWithoutPlaylist(t *testing.T) {
	s := playingSession(t)
	s.OnEnded(protocol.NodeA, "el1", "eof")
	effs := s.OnEnded(protocol.NodeB, "el1", "eof")
	n := one[EffNotify](t, effs)
	if !strings.Contains(n.Text, "кидайте ссылки") {
		t.Fatalf("notice = %q", n.Text)
	}
}

// Offline gate (spec 7.2): material queued with a home missing parks in
// DEGRADED with a human notice, and starts itself when both homes appear.
func TestOfflineGateThenAutoStart(t *testing.T) {
	s := New() // nobody online yet
	effs := s.EnqueueTrack(trackEl("el1", "spotify:track:X"))
	n := one[EffNotify](t, effs)
	if !strings.Contains(n.Text, "подождёт") {
		t.Fatalf("notice = %q", n.Text)
	}
	if s.State != StateDegraded || s.Current != nil || len(s.Queue) != 1 {
		t.Fatalf("state=%s current=%v queue=%d", s.State, s.Current, len(s.Queue))
	}

	if effs := s.OnNodeBack(protocol.NodeA); effs != nil {
		t.Fatalf("one home is not enough: %#v", effs)
	}
	effs = s.OnNodeBack(protocol.NodeB)
	loads := of[EffLoad](t, effs)
	if len(loads) != 2 || loads[0].ElementID != "el1" {
		t.Fatalf("both homes online must auto-start: %#v", effs)
	}
	if s.State != StateLoading {
		t.Fatalf("state = %s", s.State)
	}
}

func TestCancel(t *testing.T) {
	s := playingSession(t)
	s.EnqueueTrack(trackEl("el2", "spotify:track:Y"))
	s.EnqueueTrack(trackEl("el3", "spotify:track:Z"))
	if _, err := s.Cancel(5); err == nil {
		t.Fatal("out of range cancel must error")
	}
	if _, err := s.Cancel(1); err != nil {
		t.Fatal(err)
	}
	if len(s.Queue) != 1 || s.Queue[0].ID != "el3" {
		t.Fatalf("queue = %#v", s.Queue)
	}
}
