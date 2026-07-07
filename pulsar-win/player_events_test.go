// Player <- /events wiring: U9 takeover reports, anchors, fades, external
// volume, ended-after-drain; plus the voice/wait/offset_test commands and
// the heartbeat state polish (provider, speakers).
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	protocol "relux.works/duet/pulsar-win/wire"
)

func strPtr(s string) *string { return &s }
func i64Ptr(v int64) *int64   { return &v }
func intPtr(v int) *int       { return &v }

func expectNothing(t *testing.T, sent chan sentMsg, within time.Duration) {
	t.Helper()
	select {
	case m := <-sent:
		t.Fatalf("unexpected message %q (%+v) within %v", m.Type, m.Payload, within)
	case <-time.After(within):
	}
}

func loadReady(t *testing.T, p *Player, sent chan sentMsg, el, uri string) {
	t.Helper()
	p.Handle(loadEnvelope(el, uri, 0))
	expectSent(t, sent, protocol.TypeReady)
}

// --- U9: foreign daemon playback in shared mode ---

func TestForeignPlaybackReportsWithDebounce(t *testing.T) {
	daemon := newFakeDaemon()
	p, sent, _ := newTestPlayer(t, daemon, fixedClock{ok: true})
	p.externalDebounce = 40 * time.Millisecond
	loadReady(t, p, sent, "el_1", "spotify:track:ours")

	// Our own uri never reports.
	p.HandleLibrespotEvent(LibrespotEvent{Type: "playing", URI: strPtr("spotify:track:ours")})
	expectNothing(t, sent, 20*time.Millisecond)

	// A foreign uri reports external_playback.
	p.HandleLibrespotEvent(LibrespotEvent{Type: "playing", URI: strPtr("spotify:track:theirs")})
	m := expectSent(t, sent, protocol.TypeExternalPlayback)
	if m.Payload.(*protocol.ExternalPlaybackPayload).URI != "spotify:track:theirs" {
		t.Fatalf("external payload %+v", m.Payload)
	}

	// Debounced: an immediate repeat stays silent...
	p.HandleLibrespotEvent(LibrespotEvent{Type: "metadata", URI: strPtr("spotify:track:theirs")})
	expectNothing(t, sent, 20*time.Millisecond)

	// ...but after the window it reports again (metadata path this time).
	time.Sleep(40 * time.Millisecond)
	p.HandleLibrespotEvent(LibrespotEvent{Type: "metadata", URI: strPtr("spotify:track:theirs")})
	expectSent(t, sent, protocol.TypeExternalPlayback)
}

func TestForeignPlaybackIgnoredOutsideShared(t *testing.T) {
	daemon := newFakeDaemon()
	p, sent, _ := newTestPlayer(t, daemon, fixedClock{ok: true})
	p.Handle(protocol.Envelope{Type: protocol.TypeSetMode}, &protocol.SetModePayload{Mode: "solo"})

	p.HandleLibrespotEvent(LibrespotEvent{Type: "playing", URI: strPtr("spotify:track:anything")})
	expectNothing(t, sent, 30*time.Millisecond)

	// Solo also adopts the daemon-driven uri from metadata (queue advances).
	p.HandleLibrespotEvent(LibrespotEvent{Type: "metadata", URI: strPtr("spotify:track:next")})
	if st := p.StatePayload(0); st.URI == nil || *st.URI != "spotify:track:next" {
		t.Fatalf("solo metadata must adopt the uri, state %+v", st)
	}
}

// --- anchors and fades from events ---

func TestMetadataAndSeekEventsMoveAnchor(t *testing.T) {
	daemon := newFakeDaemon()
	p, sent, _ := newTestPlayer(t, daemon, fixedClock{ok: true})
	loadReady(t, p, sent, "el_1", "spotify:track:x")

	p.HandleLibrespotEvent(LibrespotEvent{Type: "metadata",
		URI: strPtr("spotify:track:x"), Position: i64Ptr(30_000)})
	if got := p.AudiblePositionMS(); got != 30_000 {
		t.Fatalf("position after metadata anchor %d, want 30000", got)
	}

	p.HandleLibrespotEvent(LibrespotEvent{Type: "seek", Position: i64Ptr(90_000)})
	if got := p.AudiblePositionMS(); got != 90_000 {
		t.Fatalf("position after seek anchor %d, want 90000", got)
	}
}

func TestPausedEventFadesOutPlayingMusic(t *testing.T) {
	daemon := newFakeDaemon()
	p, sent, _ := newTestPlayer(t, daemon, fixedClock{ok: true})
	loadReady(t, p, sent, "el_1", "spotify:track:x")
	p.Handle(protocol.Envelope{Type: protocol.TypeResumeAt},
		&protocol.ResumeAtPayload{ElementID: "el_1", TCoordMS: 0}) // past -> now
	expectCall(t, daemon, "resume")

	p.HandleLibrespotEvent(LibrespotEvent{Type: "paused"})
	g := p.engine.gain
	g.mu.Lock()
	target, total := g.target, g.rampTotal
	g.mu.Unlock()
	if target != 0 || total != sampleRate*250/1000 {
		t.Fatalf("paused event must fade to 0 over 250 ms, got target=%v total=%d", target, total)
	}

	// playing again: 120 ms fade back in (the mac resume fade).
	p.HandleLibrespotEvent(LibrespotEvent{Type: "playing"})
	g.mu.Lock()
	target, total = g.target, g.rampTotal
	g.mu.Unlock()
	if target != 1 || total != sampleRate*120/1000 {
		t.Fatalf("playing event must fade to 1 over 120 ms, got target=%v total=%d", target, total)
	}
}

func TestVolumeEventAppliesExternalVolume(t *testing.T) {
	daemon := newFakeDaemon()
	p, _, _ := newTestPlayer(t, daemon, fixedClock{ok: true})

	p.HandleLibrespotEvent(LibrespotEvent{Type: "volume", Value: intPtr(32768), Max: intPtr(65536)})
	if v := p.StatePayload(0).Volume; v != 50 {
		t.Fatalf("volume %d after event, want 50", v)
	}
	// max=0 (bogus) must not divide by zero or change anything.
	p.HandleLibrespotEvent(LibrespotEvent{Type: "volume", Value: intPtr(10), Max: intPtr(0)})
	if v := p.StatePayload(0).Volume; v != 50 {
		t.Fatalf("volume %d after bogus event, want 50", v)
	}
}

// --- ended after ring drain ---

func TestEndedSentOnlyAfterRingDrains(t *testing.T) {
	daemon := newFakeDaemon()
	p, sent, ring := newTestPlayer(t, daemon, fixedClock{ok: true})
	loadReady(t, p, sent, "el_1", "spotify:track:x")
	p.Handle(protocol.Envelope{Type: protocol.TypeResumeAt},
		&protocol.ResumeAtPayload{ElementID: "el_1", TCoordMS: 0})
	expectCall(t, daemon, "resume")

	ring.Write(make([]float32, sampleRate*channels/10)) // 100 ms of tail

	p.HandleLibrespotEvent(LibrespotEvent{Type: "stopped"})
	// The daemon says the track is over, but the tail is still audible.
	expectNothing(t, sent, 30*time.Millisecond)

	// Drain the tail (the render loop's job).
	buf := make([]float32, 4096)
	for ring.Fill() > 0 {
		ring.Read(buf)
	}

	m := expectSent(t, sent, protocol.TypeEnded)
	ended := m.Payload.(*protocol.EndedPayload)
	if ended.ElementID != "el_1" || ended.Reason != "eof" {
		t.Fatalf("ended payload %+v", ended)
	}
	if pb := p.StatePayload(0).Playback; pb != "stopped" {
		t.Fatalf("playback %q after drain, want stopped", pb)
	}
	// Exactly once.
	expectNothing(t, sent, 30*time.Millisecond)
}

func TestNotPlayingWhilePausedDoesNotEnd(t *testing.T) {
	daemon := newFakeDaemon()
	p, sent, _ := newTestPlayer(t, daemon, fixedClock{ok: true})
	loadReady(t, p, sent, "el_1", "spotify:track:x") // paused, never resumed

	p.HandleLibrespotEvent(LibrespotEvent{Type: "not_playing"})
	expectNothing(t, sent, 30*time.Millisecond) // empty ring, but not playing
}

// --- voice / wait / offset_test commands ---

func TestPlayVoiceDownloadsDecodesAndReports(t *testing.T) {
	wav := makeWAV16(2, 44100, make([]int16, 100)) // 50 frames of silence
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer tok" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Write(wav)
	}))
	defer ts.Close()

	daemon := newFakeDaemon()
	p, sent, _ := newTestPlayer(t, daemon, fixedClock{ok: true})
	cache, err := NewVoiceCache(t.TempDir(), "tok", 0, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	p.cache = cache

	p.Handle(protocol.Envelope{Type: protocol.TypePlayVoice},
		&protocol.PlayVoicePayload{ElementID: "el_v", FileURL: ts.URL + "/media/m_1.wav"})

	m := expectSent(t, sent, protocol.TypeVoiceStarted)
	if m.Payload.(*protocol.VoiceStartedPayload).ElementID != "el_v" {
		t.Fatalf("voice_started payload %+v", m.Payload)
	}
	if pb := p.StatePayload(0).Playback; pb != "voice" {
		t.Fatalf("playback %q during voice, want voice", pb)
	}

	// The engine has the insert queued; render it to its audible end.
	deadline := time.Now().Add(5 * time.Second)
	for !p.engine.VoiceActive() && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if !p.engine.VoiceActive() {
		t.Fatal("voice never reached the engine")
	}
	dst := make([]float32, 1024)
	p.engine.Render(dst)

	m = expectSent(t, sent, protocol.TypeVoiceEnded)
	if m.Payload.(*protocol.VoiceEndedPayload).ElementID != "el_v" {
		t.Fatalf("voice_ended payload %+v", m.Payload)
	}
	if pb := p.StatePayload(0).Playback; pb != "stopped" {
		t.Fatalf("playback %q after voice, want stopped", pb)
	}
}

func TestPlayVoiceDownloadFailureSendsError(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	daemon := newFakeDaemon()
	p, sent, _ := newTestPlayer(t, daemon, fixedClock{ok: true})
	cache, _ := NewVoiceCache(t.TempDir(), "tok", 0, testLogger())
	p.cache = cache

	p.Handle(protocol.Envelope{Type: protocol.TypePlayVoice},
		&protocol.PlayVoicePayload{ElementID: "el_v", FileURL: ts.URL + "/media/m_broken.wav"})

	m := expectSent(t, sent, protocol.TypeError)
	e := m.Payload.(*protocol.ErrorPayload)
	if e.Code != "media_download_failed" || e.ElementID != "el_v" {
		t.Fatalf("error payload %+v", e)
	}
}

func TestWaitSendsWaitEnded(t *testing.T) {
	daemon := newFakeDaemon()
	p, sent, _ := newTestPlayer(t, daemon, fixedClock{ok: true})

	p.Handle(protocol.Envelope{Type: protocol.TypeWait},
		&protocol.WaitPayload{ElementID: "el_w", DurationMS: 30})
	if pb := p.StatePayload(0).Playback; pb != "wait" {
		t.Fatalf("playback %q during wait, want wait", pb)
	}

	m := expectSent(t, sent, protocol.TypeWaitEnded)
	if m.Payload.(*protocol.WaitEndedPayload).ElementID != "el_w" {
		t.Fatalf("wait_ended payload %+v", m.Payload)
	}
	if pb := p.StatePayload(0).Playback; pb != "stopped" {
		t.Fatalf("playback %q after wait, want stopped", pb)
	}
}

func TestWaitCancelledByStop(t *testing.T) {
	daemon := newFakeDaemon()
	p, sent, _ := newTestPlayer(t, daemon, fixedClock{ok: true})

	p.Handle(protocol.Envelope{Type: protocol.TypeWait},
		&protocol.WaitPayload{ElementID: "el_w", DurationMS: 60})
	p.Handle(protocol.Envelope{Type: protocol.TypeStop}, &protocol.StopPayload{})
	expectCall(t, daemon, "stop")

	expectNothing(t, sent, 120*time.Millisecond) // no wait_ended after stop
}

func TestOffsetTestSchedulesClicks(t *testing.T) {
	daemon := newFakeDaemon()
	p, _, _ := newTestPlayer(t, daemon, fixedClock{ok: true})

	p.Handle(protocol.Envelope{Type: protocol.TypeOffsetTest},
		&protocol.OffsetTestPayload{TCoordMS: nowMS() + 500, Clicks: 3, IntervalMS: 100})
	p.engine.mu.Lock()
	n := len(p.engine.clicks)
	p.engine.mu.Unlock()
	if n != 3 {
		t.Fatalf("scheduled clicks %d, want 3", n)
	}
}

func TestOffsetTestWithoutClockSkips(t *testing.T) {
	daemon := newFakeDaemon()
	p, _, _ := newTestPlayer(t, daemon, fixedClock{ok: false})

	p.Handle(protocol.Envelope{Type: protocol.TypeOffsetTest},
		&protocol.OffsetTestPayload{TCoordMS: nowMS(), Clicks: 2, IntervalMS: 50})
	p.engine.mu.Lock()
	n := len(p.engine.clicks)
	p.engine.mu.Unlock()
	if n != 0 {
		t.Fatalf("clicks scheduled without clock sync: %d", n)
	}
}

// --- heartbeat polish ---

func TestStateCarriesProviderAndSpeakers(t *testing.T) {
	daemon := newFakeDaemon()
	p, _, _ := newTestPlayer(t, daemon, fixedClock{ok: true})

	st := p.StatePayload(7)
	if st.Provider != "spotify" {
		t.Fatalf("provider %q, want spotify", st.Provider)
	}
	if len(st.Speakers) != 1 || st.Speakers[0].Name != "Default output" || !st.Speakers[0].Connected {
		t.Fatalf("speakers %+v, want the single default output entry", st.Speakers)
	}
	if st.Degraded {
		t.Fatal("degraded must stay false (placeholder)")
	}

	p.SetSpeakerName("Speakers (Realtek High Definition Audio)")
	p.SetSpeakerName("") // empty updates are ignored
	if got := p.StatePayload(0).Speakers[0].Name; got != "Speakers (Realtek High Definition Audio)" {
		t.Fatalf("speaker name %q after SetSpeakerName", got)
	}
}
