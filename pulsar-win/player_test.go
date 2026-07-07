// Player state machine against a fake daemon and a fixed clock: the command
// semantics ported from the macOS PlayerCore must hold.
package main

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	protocol "relux.works/duet/pulsar-win/wire"
)

// --- fakes ---

type fakeDaemon struct {
	mu          sync.Mutex
	calls       chan string
	readyAfter  int  // PlaybackReady returns false this many times
	playFails   int  // PlayPaused errors this many times
	statusEmpty bool // Status stays pre-login empty
	lastURI     string
}

func newFakeDaemon() *fakeDaemon {
	return &fakeDaemon{calls: make(chan string, 128)}
}

func (f *fakeDaemon) record(s string) { f.calls <- s }

func (f *fakeDaemon) PlaybackReady(context.Context) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("ready?")
	if f.readyAfter > 0 {
		f.readyAfter--
		return false
	}
	return true
}

func (f *fakeDaemon) Status(context.Context) (DaemonStatus, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("status")
	if f.statusEmpty || f.lastURI == "" {
		return DaemonStatus{}, nil
	}
	paused := true
	return DaemonStatus{Paused: &paused, Track: &DaemonTrack{URI: f.lastURI}}, nil
}

func (f *fakeDaemon) PlayPaused(_ context.Context, uri string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.record("play " + uri)
	if f.playFails > 0 {
		f.playFails--
		return fmt.Errorf("transient daemon stall")
	}
	f.lastURI = uri
	return nil
}

func (f *fakeDaemon) Seek(_ context.Context, pos int64) error {
	f.record(fmt.Sprintf("seek %d", pos))
	return nil
}
func (f *fakeDaemon) Resume(context.Context) error { f.record("resume"); return nil }
func (f *fakeDaemon) Pause(context.Context) error  { f.record("pause"); return nil }
func (f *fakeDaemon) Stop(context.Context) error   { f.record("stop"); return nil }
func (f *fakeDaemon) AddToQueue(_ context.Context, uri string) error {
	f.record("queue " + uri)
	return nil
}

// expectCall waits for the next daemon call matching prefix, draining
// everything else (polling noise and calls already asserted by an earlier
// phase of the same test).
func expectCall(t *testing.T, f *fakeDaemon, prefix string) string {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		select {
		case got := <-f.calls:
			if strings.HasPrefix(got, prefix) {
				return got
			}
		case <-deadline:
			t.Fatalf("daemon never received a %q call", prefix)
		}
	}
}

func neverCall(t *testing.T, f *fakeDaemon, prefix string, within time.Duration) {
	t.Helper()
	deadline := time.After(within)
	for {
		select {
		case got := <-f.calls:
			if strings.HasPrefix(got, prefix) {
				t.Fatalf("unexpected daemon call %q within %v", got, within)
			}
		case <-deadline:
			return
		}
	}
}

type sentMsg struct {
	Type    string
	Payload any
}

type fixedClock struct {
	offset float64
	ok     bool
}

func (c fixedClock) LocalDeadline(tCoordMS int64, latency int) (int64, bool) {
	if !c.ok {
		return 0, false
	}
	return tCoordMS + int64(c.offset) - int64(latency), true
}
func (c fixedClock) OffsetMS() (float64, bool) { return c.offset, c.ok }

func newTestPlayer(t *testing.T, daemon daemonAPI, clock deadlineClock) (*Player, chan sentMsg, *Ring) {
	t.Helper()
	sent := make(chan sentMsg, 64)
	ring := NewRing(sampleRate * channels) // 1 s
	gain := NewGain()
	engine := NewEngine(ring, gain)
	p := NewPlayer(daemon, ring, engine, nil, clock,
		func(msgType string, payload any) { sent <- sentMsg{msgType, payload} },
		0, testLogger())
	// Shrink the poll knobs: defaults mirror production (20x500ms etc).
	p.readyPollInterval = 2 * time.Millisecond
	p.confirmPollInterval = 2 * time.Millisecond
	p.drainInterval = 2 * time.Millisecond
	p.Start() // watchers read the knobs, so start after shrinking them
	t.Cleanup(func() {
		p.Close()
		gain.Close()
	})
	return p, sent, ring
}

func expectSent(t *testing.T, sent chan sentMsg, msgType string) sentMsg {
	t.Helper()
	select {
	case m := <-sent:
		if m.Type != msgType {
			t.Fatalf("sent %q (%+v), want %q", m.Type, m.Payload, msgType)
		}
		return m
	case <-time.After(5 * time.Second):
		t.Fatalf("nothing sent, want %q", msgType)
		return sentMsg{}
	}
}

func loadEnvelope(el, uri string, pos int64) (protocol.Envelope, *protocol.LoadPayload) {
	p := &protocol.LoadPayload{ElementID: el, URI: uri, PositionMS: pos}
	env, _ := protocol.NewEnvelope("msg_x", 1, protocol.TypeLoad, p)
	return env, p
}

// --- tests ---

func TestLoadHappyPathSendsReady(t *testing.T) {
	daemon := newFakeDaemon()
	daemon.readyAfter = 2 // load must wait for daemon auth, not fail (R0)
	p, sent, _ := newTestPlayer(t, daemon, fixedClock{ok: true})

	env, payload := loadEnvelope("el_1", "spotify:track:x", 63000)
	p.Handle(env, payload)

	if got := expectCall(t, daemon, "play "); got != "play spotify:track:x" {
		t.Fatalf("play call %q", got)
	}
	if got := expectCall(t, daemon, "seek "); got != "seek 63000" {
		t.Fatalf("seek call %q", got)
	}
	m := expectSent(t, sent, protocol.TypeReady)
	if m.Payload.(*protocol.ReadyPayload).ElementID != "el_1" {
		t.Fatalf("ready payload %+v", m.Payload)
	}
	if pb := p.StatePayload(0).Playback; pb != "paused" {
		t.Fatalf("playback %q after load, want paused", pb)
	}
}

func TestLoadSkipsSeekAtZero(t *testing.T) {
	daemon := newFakeDaemon()
	p, sent, _ := newTestPlayer(t, daemon, fixedClock{ok: true})
	p.Handle(loadEnvelope("el_1", "spotify:track:x", 0))
	expectSent(t, sent, protocol.TypeReady)
	close(daemon.calls)
	for got := range daemon.calls {
		if strings.HasPrefix(got, "seek") {
			t.Fatal("seek must be skipped at position 0")
		}
	}
}

func TestLoadRetriesOnceThenSucceeds(t *testing.T) {
	daemon := newFakeDaemon()
	daemon.playFails = 1
	p, sent, _ := newTestPlayer(t, daemon, fixedClock{ok: true})
	p.Handle(loadEnvelope("el_1", "spotify:track:x", 0))

	expectCall(t, daemon, "play ")
	expectCall(t, daemon, "play ") // the one local retry
	expectSent(t, sent, protocol.TypeReady)
}

func TestLoadFailureSendsLoadFailed(t *testing.T) {
	daemon := newFakeDaemon()
	daemon.playFails = 2 // both attempts fail
	p, sent, _ := newTestPlayer(t, daemon, fixedClock{ok: true})
	p.Handle(loadEnvelope("el_1", "spotify:track:gone", 0))

	m := expectSent(t, sent, protocol.TypeError)
	errPayload := m.Payload.(*protocol.ErrorPayload)
	if errPayload.Code != "load_failed" || errPayload.ElementID != "el_1" {
		t.Fatalf("error payload %+v", errPayload)
	}
	if pb := p.StatePayload(0).Playback; pb != "stopped" {
		t.Fatalf("playback %q after failed load, want stopped", pb)
	}
}

func TestResumeAtSchedulesOnDeadline(t *testing.T) {
	daemon := newFakeDaemon()
	p, sent, _ := newTestPlayer(t, daemon, fixedClock{ok: true}) // offset 0, latency 0
	p.Handle(loadEnvelope("el_1", "spotify:track:x", 0))
	expectSent(t, sent, protocol.TypeReady)

	env, _ := protocol.NewEnvelope("msg_r", 1, protocol.TypeResumeAt, &protocol.ResumeAtPayload{
		ElementID: "el_1", TCoordMS: nowMS() + 150,
	})
	p.Handle(env, &protocol.ResumeAtPayload{ElementID: "el_1", TCoordMS: nowMS() + 150})

	neverCall(t, daemon, "resume", 60*time.Millisecond) // not before the deadline
	expectCall(t, daemon, "resume")                     // fires at T_local

	if pb := p.StatePayload(0).Playback; pb != "playing" {
		t.Fatalf("playback %q, want playing", pb)
	}

	// First rendered samples -> started with a coordinator timestamp.
	before := nowMS()
	p.NoteRendered(512)
	m := expectSent(t, sent, protocol.TypeStarted)
	started := m.Payload.(*protocol.StartedPayload)
	if started.ElementID != "el_1" {
		t.Fatalf("started element %q", started.ElementID)
	}
	if started.TFirstSampleCoordMS < before-100 || started.TFirstSampleCoordMS > nowMS()+100 {
		t.Fatalf("t_first_sample_coord_ms %d implausible", started.TFirstSampleCoordMS)
	}
	// Only the FIRST pull reports started.
	p.NoteRendered(512)
	select {
	case m := <-sent:
		t.Fatalf("second NoteRendered sent %+v", m)
	case <-time.After(30 * time.Millisecond):
	}
}

func TestResumeAtWrongElementIgnored(t *testing.T) {
	daemon := newFakeDaemon()
	p, sent, _ := newTestPlayer(t, daemon, fixedClock{ok: true})
	p.Handle(loadEnvelope("el_1", "spotify:track:x", 0))
	expectSent(t, sent, protocol.TypeReady)

	p.Handle(protocol.Envelope{Type: protocol.TypeResumeAt},
		&protocol.ResumeAtPayload{ElementID: "el_OTHER", TCoordMS: nowMS()})
	neverCall(t, daemon, "resume", 50*time.Millisecond)
}

func TestResumeAtWithoutClockFiresImmediately(t *testing.T) {
	daemon := newFakeDaemon()
	p, sent, _ := newTestPlayer(t, daemon, fixedClock{ok: false})
	p.Handle(loadEnvelope("el_1", "spotify:track:x", 0))
	expectSent(t, sent, protocol.TypeReady)

	p.Handle(protocol.Envelope{Type: protocol.TypeResumeAt},
		&protocol.ResumeAtPayload{ElementID: "el_1", TCoordMS: nowMS() + 60_000})
	expectCall(t, daemon, "resume") // no clock -> start now, do not sit for a minute
}

func TestPauseStopVolumeAndState(t *testing.T) {
	daemon := newFakeDaemon()
	p, sent, ring := newTestPlayer(t, daemon, fixedClock{ok: true})
	p.Handle(loadEnvelope("el_1", "spotify:track:x", 0))
	expectSent(t, sent, protocol.TypeReady)

	p.Handle(protocol.Envelope{Type: protocol.TypePause},
		&protocol.PausePayload{ElementID: "el_1", FadeMS: 250})
	expectCall(t, daemon, "pause")
	if pb := p.StatePayload(42); pb.Playback != "paused" || pb.RTTMS != 42 {
		t.Fatalf("state %+v", pb)
	}

	p.SetVolume(55)
	if v := p.StatePayload(0).Volume; v != 55 {
		t.Fatalf("volume %d, want 55", v)
	}

	ring.Write(make([]float32, 1024))
	p.Handle(protocol.Envelope{Type: protocol.TypeStop}, &protocol.StopPayload{})
	expectCall(t, daemon, "stop")
	// Stop lands softly (spec 4.3): 250 ms raised-cosine fade, THEN the ring
	// tail is dropped (~300 ms) — poll for the deferred clear.
	deadline := time.Now().Add(5 * time.Second)
	for ring.Fill() != 0 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if ring.Fill() != 0 {
		t.Fatal("stop must clear the ring after the fade window")
	}
	st := p.StatePayload(0)
	if st.Playback != "stopped" || st.URI != nil {
		t.Fatalf("state after stop %+v", st)
	}
}

func TestAudiblePositionSubtractsRingFill(t *testing.T) {
	daemon := newFakeDaemon()
	p, _, ring := newTestPlayer(t, daemon, fixedClock{ok: true})

	// Anchor at 10s paused (no extrapolation), 50 ms sitting in the ring.
	p.mu.Lock()
	p.anchorPosMS = 10_000
	p.anchorAt = time.Now()
	p.extrapolate = false
	p.mu.Unlock()
	ring.Write(make([]float32, sampleRate*channels/20)) // 50 ms

	got := p.AudiblePositionMS()
	if got != 9_950 {
		t.Fatalf("audible position %d, want 9950 (10000 - 50ms ring fill)", got)
	}
}

func TestSoloInjectQueuesURI(t *testing.T) {
	daemon := newFakeDaemon()
	p, _, _ := newTestPlayer(t, daemon, fixedClock{ok: true})
	p.Handle(protocol.Envelope{Type: protocol.TypeSoloInject},
		&protocol.SoloInjectPayload{URI: "spotify:track:y"})
	if got := expectCall(t, daemon, "queue "); got != "queue spotify:track:y" {
		t.Fatalf("queue call %q", got)
	}
}

func TestStarvedCountsOnlyWhilePlaying(t *testing.T) {
	daemon := newFakeDaemon()
	p, sent, _ := newTestPlayer(t, daemon, fixedClock{ok: true})

	p.NoteStarved() // stopped: idle silence is not an underrun (R4)
	if u := p.StatePayload(0).Underruns; u != 0 {
		t.Fatalf("underruns %d while stopped, want 0", u)
	}

	p.Handle(loadEnvelope("el_1", "spotify:track:x", 0))
	expectSent(t, sent, protocol.TypeReady)
	p.Handle(protocol.Envelope{Type: protocol.TypeResumeAt},
		&protocol.ResumeAtPayload{ElementID: "el_1", TCoordMS: 0}) // past -> immediate
	expectCall(t, daemon, "resume")
	p.NoteStarved()
	if u := p.StatePayload(0).Underruns; u != 1 {
		t.Fatalf("underruns %d while playing, want 1", u)
	}
}
