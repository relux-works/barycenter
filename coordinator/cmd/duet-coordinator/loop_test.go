// Integration tests: every phase-1 bot command (spec ch. 9, goal DoD-4)
// driven through the real loop + FSM + SQLite store with a fake transport.
package main

import (
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/bot"
	"relux.works/duet/coordinator/internal/config"
	"relux.works/duet/coordinator/internal/hub"
	"relux.works/duet/coordinator/internal/media"
	"relux.works/duet/coordinator/internal/protocol"
	"relux.works/duet/coordinator/internal/session"
	"relux.works/duet/coordinator/internal/store"
)

type sentMsg struct {
	node    protocol.NodeID // slot within the test orbit
	key     hub.NodeKey     // full wire address (approach tests assert orbit routing)
	msgType string
	payload any
}

type fakeSender struct {
	sent []sentMsg
}

func (f *fakeSender) Send(key hub.NodeKey, msgType string, payload any) bool {
	f.sent = append(f.sent, sentMsg{key.Slot, key, msgType, payload})
	return true
}

func (f *fakeSender) Online(orbitID int64) map[protocol.NodeID]bool {
	return map[protocol.NodeID]bool{protocol.NodeA: true, protocol.NodeB: true}
}

func (f *fakeSender) drain() []sentMsg {
	out := f.sent
	f.sent = nil
	return out
}

func (f *fakeSender) ofType(t string) []sentMsg {
	var out []sentMsg
	for _, m := range f.sent {
		if m.msgType == t {
			out = append(out, m)
		}
	}
	return out
}

func testConfig(t *testing.T) *config.Config {
	return &config.Config{
		Listen:   "127.0.0.1:0",
		DBPath:   filepath.Join(t.TempDir(), "duet.db"),
		MediaDir: t.TempDir(),
		Nodes: map[string]config.Node{
			"a": {Token: strings.Repeat("a", 64)},
			"b": {Token: strings.Repeat("b", 64)},
		},
		Telegram: config.Telegram{Users: map[int64]string{111: "a", 222: "b"}},
		Timings:  config.Timings{ReadyTimeoutS: 8, StartMarginMS: 500, OfflineAfterS: 12, NearEndMS: 400},
		Media:    config.Media{MaxVoiceS: 180, RetentionDays: 30, Preset: "default"},
	}
}

func newTestLoop(t *testing.T) (*loop, *fakeSender) {
	t.Helper()
	cfg := testConfig(t)
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	fake := &fakeSender{}
	if _, err := st.BootstrapLegacyOrbit(
		map[string]string{"a": cfg.Nodes["a"].Token, "b": cfg.Nodes["b"].Token},
		cfg.Telegram.Users); err != nil {
		t.Fatal(err)
	}
	l := newLoop(slog.Default(), cfg, fake, st, nil, nil)
	l.warmup()
	// Both homes online — the real path is hub EvOnline on registration
	// (the offline gate parks broadcasts otherwise).
	l.handleNode(hub.EvOnline{Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeA}})
	l.handleNode(hub.EvOnline{Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeB}})
	return l, fake
}

type replies struct{ texts []string }

func (r *replies) fn(text string) { r.texts = append(r.texts, text) }

func (r *replies) last(t *testing.T) string {
	t.Helper()
	if len(r.texts) == 0 {
		t.Fatal("no reply arrived")
	}
	return r.texts[len(r.texts)-1]
}

// homes: "a" is user 111 (Ivan), "b" is user 222 (Katya) — see testConfig.
// "o2" is user 333, primary of the second orbit in approach tests.
var testUsers = map[string]int64{"a": 111, "b": 222, "o2": 333}

func cmdEvent(t *testing.T, from, text string, r *replies) bot.Event {
	t.Helper()
	cmd, err := bot.Parse(text)
	if err != nil {
		t.Fatalf("parse %q: %v", text, err)
	}
	return bot.Event{FromUserID: testUsers[from], FromName: "user-" + from, Command: cmd, Reply: r.fn}
}

// pumpResolve drives one provider-cascade round-trip: the loop offloads the
// external resolve to a goroutine that delivers back over resolveCh (bugs #4).
// Tests that call handleBot directly (no l.run) drain that channel here.
func (l *loop) pumpResolve(t *testing.T) {
	t.Helper()
	select {
	case d := <-l.resolveCh:
		l.onResolveDone(d)
	case <-time.After(2 * time.Second):
		t.Fatal("resolve cascade did not complete")
	}
}

const link = "https://open.spotify.com/track/4cOdK2wGLETKBW3PvgPWqT"
const link2 = "https://open.spotify.com/track/1301WleyT98MSxVHPZCA6M"

// The full phase-1 command set against the live FSM (goal DoD-4).
func TestPhase1BotCommandsEndToEnd(t *testing.T) {
	l, fake := newTestLoop(t)
	r := &replies{}

	// Heartbeats give the scheduler RTTs and positions.
	l.handleNodeMessage(hub.EvMessage{Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeA}, Payload: &protocol.StatePayload{PositionMS: 0, RTTMS: 40, Volume: 80}})
	l.handleNodeMessage(hub.EvMessage{Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeB}, Payload: &protocol.StatePayload{PositionMS: 0, RTTMS: 60, Volume: 80}})
	fake.drain()

	// Link on idle -> load both immediately.
	l.handleBot(cmdEvent(t, "a", link, r))
	if got := fake.ofType(protocol.TypeLoad); len(got) != 2 {
		t.Fatalf("load to both expected, sent: %+v", fake.sent)
	}
	if !strings.Contains(r.last(t), "ставлю сразу") {
		t.Fatalf("reply: %q", r.last(t))
	}
	elID := l.orbit(1).sess.Current.ID
	fake.drain()

	// Second link queues up.
	l.handleBot(cmdEvent(t, "b", link2, r))
	if !strings.Contains(r.last(t), "номером 1") {
		t.Fatalf("reply: %q", r.last(t))
	}

	// /queue shows it.
	l.handleBot(cmdEvent(t, "a", "/queue", r))
	if !strings.Contains(r.last(t), "очередь") || !strings.Contains(r.last(t), "1301WleyT98MSxVHPZCA6M") {
		t.Fatalf("queue text: %q", r.last(t))
	}

	// ready/started full cycle.
	l.handleNodeMessage(hub.EvMessage{Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeA}, Payload: &protocol.ReadyPayload{ElementID: elID}})
	l.handleNodeMessage(hub.EvMessage{Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeB}, Payload: &protocol.ReadyPayload{ElementID: elID}})
	if got := fake.ofType(protocol.TypeResumeAt); len(got) != 2 {
		t.Fatalf("resume_at to both, sent: %+v", fake.sent)
	}
	l.handleNodeMessage(hub.EvMessage{Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeA}, Payload: &protocol.StartedPayload{ElementID: elID, TFirstSampleCoordMS: 1000}})
	l.handleNodeMessage(hub.EvMessage{Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeB}, Payload: &protocol.StartedPayload{ElementID: elID, TFirstSampleCoordMS: 1034}})
	if l.orbit(1).lastDesyncMS != 34 {
		t.Fatalf("desync = %d", l.orbit(1).lastDesyncMS)
	}
	if l.orbit(1).sess.State != session.StatePlaying {
		t.Fatalf("state = %s", l.orbit(1).sess.State)
	}
	fake.drain()

	// /pause -> pause both; /resume -> reload from saved position.
	l.handleNodeMessage(hub.EvMessage{Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeA}, Payload: &protocol.StatePayload{PositionMS: 63000, RTTMS: 40, Volume: 80}})
	l.handleNodeMessage(hub.EvMessage{Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeB}, Payload: &protocol.StatePayload{PositionMS: 62950, RTTMS: 60, Volume: 80}})
	l.handleBot(cmdEvent(t, "a", "/pause", r))
	if got := fake.ofType(protocol.TypePause); len(got) != 2 {
		t.Fatalf("pause both, sent: %+v", fake.sent)
	}
	fake.drain()
	l.handleBot(cmdEvent(t, "b", "/resume", r))
	loads := fake.ofType(protocol.TypeLoad)
	if len(loads) != 2 || loads[0].payload.(*protocol.LoadPayload).PositionMS != 62950 {
		t.Fatalf("resume reloads from min position, sent: %+v", fake.sent)
	}
	fake.drain()

	// /vol without target = sender's home; with target = that node.
	l.handleBot(cmdEvent(t, "a", "/vol 55", r))
	if got := fake.ofType(protocol.TypeSetVolume); len(got) != 1 || got[0].node != protocol.NodeA {
		t.Fatalf("vol to own node, sent: %+v", fake.sent)
	}
	if v, _ := l.st.GetSetting("volume_1_a"); v != "55" {
		t.Fatalf("volume_a persisted = %q", v)
	}
	l.handleBot(cmdEvent(t, "a", "/vol 40 b", r))
	vols := fake.ofType(protocol.TypeSetVolume)
	if vols[len(vols)-1].node != protocol.NodeB {
		t.Fatalf("vol 40 b went to %v", vols[len(vols)-1].node)
	}
	fake.drain()

	// /offset persists and pushes.
	l.handleBot(cmdEvent(t, "b", "/offset b 250", r))
	offs := fake.ofType(protocol.TypeSetOffset)
	if len(offs) != 1 || offs[0].node != protocol.NodeB || offs[0].payload.(*protocol.SetOffsetPayload).OffsetMS != 250 {
		t.Fatalf("set_offset, sent: %+v", fake.sent)
	}
	if v, _ := l.st.GetSetting("offset_1_b"); v != "250" {
		t.Fatalf("offset_b persisted = %q", v)
	}
	fake.drain()

	// /offset_test hits both nodes.
	l.handleBot(cmdEvent(t, "a", "/offset_test", r))
	if got := fake.ofType(protocol.TypeOffsetTest); len(got) != 2 {
		t.Fatalf("offset_test both, sent: %+v", fake.sent)
	}
	fake.drain()

	// /playnow replaces current; /skip moves on; /cancel validates.
	l.handleBot(cmdEvent(t, "a", "/playnow "+link, r))
	if got := fake.ofType(protocol.TypeLoad); len(got) != 2 {
		t.Fatalf("playnow loads, sent: %+v", fake.sent)
	}
	fake.drain()
	l.handleBot(cmdEvent(t, "a", "/skip", r))
	if !strings.Contains(r.last(t), "пропустил") {
		t.Fatalf("skip reply: %q", r.last(t))
	}
	l.handleBot(cmdEvent(t, "a", "/cancel 99", r))
	if !strings.Contains(r.last(t), "нет") {
		t.Fatalf("cancel reply: %q", r.last(t))
	}
	fake.drain()

	// /sync during play restarts current.
	elID2 := l.orbit(1).sess.Current.ID
	l.handleNodeMessage(hub.EvMessage{Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeA}, Payload: &protocol.ReadyPayload{ElementID: elID2}})
	l.handleNodeMessage(hub.EvMessage{Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeB}, Payload: &protocol.ReadyPayload{ElementID: elID2}})
	l.handleNodeMessage(hub.EvMessage{Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeA}, Payload: &protocol.StartedPayload{ElementID: elID2, TFirstSampleCoordMS: 2000}})
	l.handleNodeMessage(hub.EvMessage{Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeB}, Payload: &protocol.StartedPayload{ElementID: elID2, TFirstSampleCoordMS: 2010}})
	fake.drain()
	l.handleBot(cmdEvent(t, "b", "/sync", r))
	if got := fake.ofType(protocol.TypeLoad); len(got) != 2 {
		t.Fatalf("sync reloads current, sent: %+v", fake.sent)
	}
	fake.drain()

	// Mode switch + /inject in solo goes to the partner by default.
	l.handleBot(cmdEvent(t, "a", "/mode solo", r))
	if len(fake.ofType(protocol.TypeStop)) != 2 || len(fake.ofType(protocol.TypeSetMode)) != 2 {
		t.Fatalf("solo switch, sent: %+v", fake.sent)
	}
	fake.drain()
	l.handleBot(cmdEvent(t, "a", "/inject "+link, r))
	inj := fake.ofType(protocol.TypeSoloInject)
	if len(inj) != 1 || inj[0].node != protocol.NodeB {
		t.Fatalf("inject defaults to partner, sent: %+v", fake.sent)
	}
	fake.drain()

	// /now and /status always answer.
	l.handleBot(cmdEvent(t, "a", "/now", r))
	if !strings.Contains(r.last(t), "апоастрон") {
		t.Fatalf("now text: %q", r.last(t))
	}
	l.handleBot(cmdEvent(t, "b", "/status", r))
	if !strings.Contains(r.last(t), "режим solo") || !strings.Contains(r.last(t), "дом a") {
		t.Fatalf("status text: %q", r.last(t))
	}

	// Back to shared resumes the interrupted element.
	l.handleBot(cmdEvent(t, "a", "/mode shared", r))
	if got := fake.ofType(protocol.TypeLoad); len(got) != 2 {
		t.Fatalf("shared resume, sent: %+v", fake.sent)
	}
}

// Personal voice in shared: play_voice to the partner, wait to the sender
// (spec 4.1 item 5, goal DoD-6 routing part).
func TestPersonalVoiceRouting(t *testing.T) {
	l, fake := newTestLoop(t)
	r := &replies{}
	l.handleMediaDone(mediaDone{
		orbit:    1,
		mediaID:  "m_test1",
		from:     111, // Ivan owns slot a
		fromName: "user-a",
		personal: true,
		result:   media.Result{DurationMS: 12_400, WAVPath: "/tmp/x.wav", LoudnormJSON: "{}"},
		reply:    r.fn,
	})
	plays := fake.ofType(protocol.TypePlayVoice)
	waits := fake.ofType(protocol.TypeWait)
	if len(plays) != 1 || plays[0].node != protocol.NodeB {
		t.Fatalf("play_voice to partner, sent: %+v", fake.sent)
	}
	if len(waits) != 1 || waits[0].node != protocol.NodeA ||
		waits[0].payload.(*protocol.WaitPayload).DurationMS != 12_400 {
		t.Fatalf("wait to sender with same duration, sent: %+v", fake.sent)
	}
	if !strings.Contains(r.last(t), "личная") {
		t.Fatalf("reply: %q", r.last(t))
	}
}

// Coordinator restart: state comes back PAUSED with the queue intact and the
// resume position follows fresh heartbeats (spec 7.2, goal DoD around 18.10).
func TestRestartRestoresPausedSession(t *testing.T) {
	cfg := testConfig(t)
	st, err := store.Open(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	fake := &fakeSender{}
	if _, err := st.BootstrapLegacyOrbit(
		map[string]string{"a": cfg.Nodes["a"].Token, "b": cfg.Nodes["b"].Token},
		cfg.Telegram.Users); err != nil {
		t.Fatal(err)
	}
	l := newLoop(slog.Default(), cfg, fake, st, nil, nil)
	l.warmup()
	l.handleNode(hub.EvOnline{Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeA}})
	l.handleNode(hub.EvOnline{Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeB}})
	r := &replies{}
	l.handleBot(cmdEvent(t, "a", link, r))
	l.handleBot(cmdEvent(t, "b", link2, r))
	st.Close()

	st2, err := store.Open(cfg.DBPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	fake2 := &fakeSender{}
	l2 := newLoop(slog.Default(), cfg, fake2, st2, nil, nil)
	l2.warmup()

	if l2.orbit(1).sess.State != session.StatePaused || l2.orbit(1).sess.Current == nil {
		t.Fatalf("restored state=%s current=%v", l2.orbit(1).sess.State, l2.orbit(1).sess.Current)
	}
	if l2.orbit(1).sess.QueueLen() != 1 {
		t.Fatalf("queue len = %d", l2.orbit(1).sess.QueueLen())
	}
	// Fresh heartbeats refresh the resume position (restoredPaused path).
	l2.handleNodeMessage(hub.EvMessage{Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeA}, Payload: &protocol.StatePayload{PositionMS: 42_000, RTTMS: 40}})
	l2.handleNodeMessage(hub.EvMessage{Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeB}, Payload: &protocol.StatePayload{PositionMS: 41_900, RTTMS: 60}})
	l2.handleBot(cmdEvent(t, "a", "/resume", r))
	loads := fake2.ofType(protocol.TypeLoad)
	if len(loads) != 2 || loads[0].payload.(*protocol.LoadPayload).PositionMS != 41_900 {
		t.Fatalf("resume position must follow heartbeats, sent: %+v", fake2.sent)
	}
}
