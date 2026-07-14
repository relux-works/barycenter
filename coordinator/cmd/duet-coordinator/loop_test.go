// Integration tests: every phase-1 bot command (spec ch. 9, goal DoD-4)
// driven through the real loop + FSM + SQLite store with a fake transport.
package main

import (
	"context"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
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
	sent      []sentMsg
	snapshots map[hub.NodeKey]hub.NodeSnapshot
}

func TestRegistrationReplacesExactCapabilitySnapshot(t *testing.T) {
	l, _ := newTestLoop(t)
	initial, err := protocol.ParseCapabilitySet([]string{
		protocol.CapabilityInterruptResume,
		protocol.CapabilityMediaClip,
		protocol.CapabilityOverlayMix,
		"unknown_future_v2",
	})
	if err != nil {
		t.Fatal(err)
	}
	key := hub.NodeKey{Orbit: 1, Slot: protocol.NodeA}
	l.handleNode(hub.EvRegistered{Key: key, Capabilities: initial})
	o := l.orbit(1)
	if !o.capabilities[protocol.NodeA].Supports(protocol.CapabilityInterruptResume) ||
		!o.capabilities[protocol.NodeA].Supports("unknown_future_v2") {
		t.Fatalf("registration lost exact capabilities: %v", o.capabilities[protocol.NodeA].Values())
	}

	replacement, err := protocol.ParseCapabilitySet([]string{protocol.CapabilityMediaClip})
	if err != nil {
		t.Fatal(err)
	}
	l.handleNode(hub.EvRegistered{Key: key, Capabilities: replacement})
	got := o.capabilities[protocol.NodeA]
	if !got.Supports(protocol.CapabilityMediaClip) ||
		got.Supports(protocol.CapabilityInterruptResume) ||
		got.Supports("unknown_future_v2") || len(got.Values()) != 1 {
		t.Fatalf("reconnect unioned instead of replacing capabilities: %v", got.Values())
	}
}

func (f *fakeSender) Send(key hub.NodeKey, msgType string, payload any) bool {
	f.sent = append(f.sent, sentMsg{key.Slot, key, msgType, payload})
	return true
}

func (f *fakeSender) Online(orbitID int64) map[protocol.NodeID]bool {
	return map[protocol.NodeID]bool{protocol.NodeA: true, protocol.NodeB: true}
}

func (f *fakeSender) NodeSnapshots() map[hub.NodeKey]hub.NodeSnapshot {
	result := make(map[hub.NodeKey]hub.NodeSnapshot, len(f.snapshots))
	for key, snapshot := range f.snapshots {
		result[key] = snapshot
	}
	return result
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
		Media:    config.Media{MaxVoiceS: 180, RetentionDays: 7, Preset: "default"},
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

type controlledTelegramCompletion struct {
	result media.Result
	code   string
}

type controlledTelegramAdapter struct {
	store    *store.Store
	accepted chan media.TelegramAcceptance
	mu       sync.Mutex
	pending  map[string]chan controlledTelegramCompletion
}

func newControlledTelegramAdapter(st *store.Store) *controlledTelegramAdapter {
	return &controlledTelegramAdapter{
		store: st, accepted: make(chan media.TelegramAcceptance, 8),
		pending: map[string]chan controlledTelegramCompletion{},
	}
}

func (adapter *controlledTelegramAdapter) Accept(voice media.TelegramVoice) (media.TelegramAcceptance, error) {
	created, err := adapter.store.CreateTelegramMedia(store.CreateTelegramMediaParams{
		OwnerOrbitID: voice.OwnerOrbitID, TelegramUserID: voice.TelegramUserID,
		TelegramFileID: voice.TelegramFileID, Title: voice.Title,
		CreatedAt: voice.AcceptedAt, ExpiresAt: voice.ExpiresAt,
	})
	if err != nil {
		return media.TelegramAcceptance{}, err
	}
	accepted := media.TelegramAcceptance{
		MediaID: created.Media.ID, OwnerOrbitID: created.Media.OwnerOrbitID,
		ActorID: created.Media.ActorID, TelegramFileID: voice.TelegramFileID,
		AttachmentKind: voice.AttachmentKind, AcceptedAt: created.Media.CreatedAt,
	}
	adapter.mu.Lock()
	adapter.pending[accepted.MediaID] = make(chan controlledTelegramCompletion, 1)
	adapter.mu.Unlock()
	adapter.accepted <- accepted
	return accepted, nil
}

func (adapter *controlledTelegramAdapter) Submit(
	_ context.Context,
	accepted media.TelegramAcceptance,
) (media.Result, error) {
	adapter.mu.Lock()
	pending := adapter.pending[accepted.MediaID]
	adapter.mu.Unlock()
	completion := <-pending
	if completion.code != "" {
		item, err := adapter.store.GetMediaItem(accepted.MediaID)
		if err != nil {
			return media.Result{}, err
		}
		if item != nil && item.Status == store.MediaStatusProcessing {
			if _, err := adapter.store.MarkMediaItemFailed(
				item.ID, item.Revision, completion.code, time.Now().UnixMilli(),
			); err != nil {
				return media.Result{}, err
			}
		}
		return media.Result{}, &media.ProcessingError{Code: completion.code}
	}
	item, err := adapter.store.GetMediaItem(accepted.MediaID)
	if err != nil {
		return media.Result{}, err
	}
	if item == nil {
		return media.Result{}, store.ErrMediaNotFound
	}
	if item.Status == store.MediaStatusProcessing {
		now := time.Now().UnixMilli()
		operation, err := adapter.store.StageMediaPublication(item.ID, item.Revision, now)
		if err != nil {
			return media.Result{}, err
		}
		info, err := os.Stat(completion.result.WAVPath)
		if err != nil {
			return media.Result{}, err
		}
		if _, err := adapter.store.CompleteMediaPublication(
			operation.ID, operation.Revision,
			store.MediaPublication{
				MIME: "audio/wav", Codec: "pcm_s16le",
				DurationMS: completion.result.DurationMS, SizeBytes: info.Size(),
				SHA256: strings.Repeat("c", 64), LoudnessJSON: completion.result.LoudnormJSON,
			},
			now+1,
		); err != nil {
			return media.Result{}, err
		}
	}
	return completion.result, nil
}

func (adapter *controlledTelegramAdapter) complete(
	mediaID string,
	completion controlledTelegramCompletion,
) {
	adapter.mu.Lock()
	pending := adapter.pending[mediaID]
	adapter.mu.Unlock()
	pending <- completion
}

func takeTelegramAcceptance(t *testing.T, adapter *controlledTelegramAdapter) media.TelegramAcceptance {
	t.Helper()
	select {
	case accepted := <-adapter.accepted:
		return accepted
	case <-time.After(2 * time.Second):
		t.Fatal("Telegram acceptance did not reach the adapter")
		return media.TelegramAcceptance{}
	}
}

func takeMediaDone(t *testing.T, l *loop) mediaDone {
	t.Helper()
	select {
	case done := <-l.mediaCh:
		return done
	case <-time.After(2 * time.Second):
		t.Fatal("Telegram SubmitMedia completion did not reach the loop")
		return mediaDone{}
	}
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

func (l *loop) pumpTrackMetadata(t *testing.T) {
	t.Helper()
	select {
	case d := <-l.trackMetaCh:
		l.handleTrackMetadataDone(d)
	case <-time.After(2 * time.Second):
		t.Fatal("track metadata lookup did not complete")
	}
}

const link = "https://open.spotify.com/track/4cOdK2wGLETKBW3PvgPWqT"
const link2 = "https://open.spotify.com/track/1301WleyT98MSxVHPZCA6M"

func TestSpotifyLinkReplyUsesHumanTrackMetadata(t *testing.T) {
	l, _ := newTestLoop(t)
	l.fetchTrackMetadata = func(ref string) (trackMetadata, error) {
		if ref != linkURI {
			t.Fatalf("metadata ref = %q", ref)
		}
		return trackMetadata{title: "Human Song", artists: []string{"Human Artist"}, durationMS: 123_000}, nil
	}
	r := &replies{}

	l.handleBot(cmdEvent(t, "a", link, r))
	l.pumpTrackMetadata(t)
	cur := l.orbit(1).sess.Current
	if cur == nil || cur.Title != "Human Artist — Human Song" || cur.DurationMS != 123_000 {
		t.Fatalf("enriched current = %+v", cur)
	}
	if !strings.Contains(r.last(t), "Human Artist — Human Song") || strings.Contains(r.last(t), "spotify:track:") {
		t.Fatalf("technical link reply: %q", r.last(t))
	}
}

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
	if !strings.Contains(r.last(t), "каждый слушает своё") || !strings.Contains(r.last(t), "Пульсар A") {
		t.Fatalf("status text: %q", r.last(t))
	}
	for _, forbidden := range []string{"колонки:", "громкость", "offset", "rtt", "поз ", "librespot", "microphone", "process"} {
		if strings.Contains(strings.ToLower(r.last(t)), forbidden) {
			t.Fatalf("status leaked private diagnostic %q: %q", forbidden, r.last(t))
		}
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
	if !strings.Contains(r.last(t), "личное голосовое") {
		t.Fatalf("reply: %q", r.last(t))
	}
}

func TestTelegramVoiceSubmitMediaAdapterPreservesFIFORepliesAndLegacyPlayback(t *testing.T) {
	l, fake := newTestLoop(t)
	adapter := newControlledTelegramAdapter(l.st)
	l.telegramMedia = adapter
	firstReplies := &replies{}
	secondReplies := &replies{}
	voiceEvent := func(fileID, fromName string, reply func(string)) bot.Event {
		return bot.Event{
			FromUserID: 111, FromName: fromName, Reply: reply,
			Voice: &bot.VoiceEvent{
				TGFileID: fileID, Duration: 1, SizeBytes: 1024,
				Broadcast: true,
			},
		}
	}

	l.handleBot(voiceEvent("tg-first", "Alice", firstReplies.fn))
	first := takeTelegramAcceptance(t, adapter)
	l.handleBot(voiceEvent("tg-second", "Bob", secondReplies.fn))
	second := takeTelegramAcceptance(t, adapter)
	const acceptedReply = "голосовое принято — поставлю по времени отправки, даже если другое обработается быстрее"
	if len(firstReplies.texts) != 1 || firstReplies.texts[0] != acceptedReply ||
		len(secondReplies.texts) != 1 || secondReplies.texts[0] != acceptedReply {
		t.Fatalf("acceptance replies first=%v second=%v", firstReplies.texts, secondReplies.texts)
	}
	for _, accepted := range []media.TelegramAcceptance{first, second} {
		item, err := l.st.GetMediaItem(accepted.MediaID)
		legacy, legacyErr := l.st.GetMedia(accepted.MediaID)
		if err != nil || legacyErr != nil || item == nil || legacy == nil ||
			item.Source != store.MediaSourceTelegram || item.ID != legacy.ID ||
			legacy.Status != "processing" {
			t.Fatalf("accepted common=%+v legacy=%+v err=%v legacyErr=%v",
				item, legacy, err, legacyErr)
		}
		if got := time.Duration(item.ExpiresAt-item.CreatedAt) * time.Millisecond; got != 7*24*time.Hour {
			t.Fatalf("Telegram clip retention=%s, want 168h", got)
		}
	}

	firstPath := filepath.Join(l.cfg.MediaDir, "first-compat.wav")
	secondPath := filepath.Join(l.cfg.MediaDir, "second-compat.wav")
	if err := os.WriteFile(firstPath, []byte("legacy-first-wav"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(secondPath, []byte("legacy-second-wav"), 0o600); err != nil {
		t.Fatal(err)
	}
	adapter.complete(second.MediaID, controlledTelegramCompletion{result: media.Result{
		WAVPath: secondPath, DurationMS: 2_000, LoudnormJSON: `{"second":true}`,
	}})
	l.handleMediaDone(takeMediaDone(t, l))
	if current := l.orbit(1).sess.Current; current != nil {
		t.Fatalf("later SubmitMedia completion escaped FIFO: %+v", current)
	}
	if legacy, err := l.st.GetMedia(second.MediaID); err != nil || legacy == nil || legacy.Status != "processing" {
		t.Fatalf("later legacy media became visible before FIFO turn: %+v err=%v", legacy, err)
	}

	adapter.complete(first.MediaID, controlledTelegramCompletion{result: media.Result{
		WAVPath: firstPath, DurationMS: 1_000, LoudnormJSON: `{"first":true}`,
	}})
	l.handleMediaDone(takeMediaDone(t, l))
	orbit := l.orbit(1)
	if orbit.sess.Current == nil || orbit.sess.Current.MediaID != first.MediaID ||
		orbit.sess.QueueLen() != 1 || orbit.sess.Queue[0].MediaID != second.MediaID {
		t.Fatalf("Telegram FIFO current=%+v queue=%+v", orbit.sess.Current, orbit.sess.Queue)
	}
	if firstReplies.last(t) != "голосовое от Alice готово: после текущего трека, для всех" ||
		secondReplies.last(t) != "голосовое от Bob готово: после текущего трека, для всех" {
		t.Fatalf("ready replies first=%v second=%v", firstReplies.texts, secondReplies.texts)
	}
	for mediaID, wantPath := range map[string]string{first.MediaID: firstPath, second.MediaID: secondPath} {
		legacy, err := l.st.GetMedia(mediaID)
		if err != nil || legacy == nil || legacy.Status != "ready" || legacy.PathWAV != wantPath {
			t.Fatalf("ready legacy media=%+v err=%v", legacy, err)
		}
	}
	plays := fake.ofType(protocol.TypePlayVoice)
	if len(plays) != 2 {
		t.Fatalf("legacy play_voice messages=%+v", plays)
	}
	for _, play := range plays {
		payload := play.payload.(*protocol.PlayVoicePayload)
		if !strings.Contains(payload.FileURL, "/media/"+first.MediaID+".wav") {
			t.Fatalf("legacy play_voice URL=%q", payload.FileURL)
		}
	}

	request := httptest.NewRequest(http.MethodGet, "/media/"+first.MediaID+".wav", nil)
	request.Header.Set("Authorization", "Bearer "+l.cfg.Nodes["a"].Token)
	response := httptest.NewRecorder()
	mediaHandler(l.st).ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "legacy-first-wav" {
		t.Fatalf("legacy node download status=%d body=%q", response.Code, response.Body.String())
	}
}

func TestTelegramVoiceCommonFailureKeepsBotReplyAndBothStatusesTerminal(t *testing.T) {
	l, _ := newTestLoop(t)
	adapter := newControlledTelegramAdapter(l.st)
	l.telegramMedia = adapter
	replies := &replies{}
	l.handleBot(bot.Event{
		FromUserID: 111, FromName: "Alice", Reply: replies.fn,
		Voice: &bot.VoiceEvent{TGFileID: "tg-bad", Duration: 1, SizeBytes: 512},
	})
	accepted := takeTelegramAcceptance(t, adapter)
	adapter.complete(accepted.MediaID, controlledTelegramCompletion{code: "media_signature_unsupported"})
	l.handleMediaDone(takeMediaDone(t, l))

	want := []string{
		"голосовое принято — поставлю по времени отправки, даже если другое обработается быстрее",
		"не смог обработать голосовое, оставил исходник для разбора",
	}
	if len(replies.texts) != len(want) || replies.texts[0] != want[0] || replies.texts[1] != want[1] {
		t.Fatalf("failure replies=%v", replies.texts)
	}
	item, err := l.st.GetMediaItem(accepted.MediaID)
	legacy, legacyErr := l.st.GetMedia(accepted.MediaID)
	if err != nil || legacyErr != nil || item == nil || legacy == nil ||
		item.Status != store.MediaStatusFailed || item.FailureCode != "media_signature_unsupported" ||
		legacy.Status != "failed" || l.orbit(1).sess.Current != nil {
		t.Fatalf("failure common=%+v legacy=%+v current=%+v err=%v legacyErr=%v",
			item, legacy, l.orbit(1).sess.Current, err, legacyErr)
	}
}

func TestTelegramAudioAndDocumentHintsReachCommonIngestWithoutTrust(t *testing.T) {
	tests := []struct {
		name      string
		kind      bot.AttachmentKind
		failure   string
		wantReply string
	}{
		{name: "audio duration hint does not pre-reject", kind: bot.AttachmentAudio,
			failure: "media_duration_exceeded", wantReply: bot.AttachmentFailureText(bot.AttachmentTrackPhase2)},
		{name: "document MIME hint does not classify", kind: bot.AttachmentDocument,
			failure: "media_signature_unsupported", wantReply: bot.AttachmentFailureText(bot.AttachmentNotAudio)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			l, _ := newTestLoop(t)
			adapter := newControlledTelegramAdapter(l.st)
			l.telegramMedia = adapter
			replies := &replies{}
			l.handleBot(bot.Event{
				FromUserID: 111, FromName: "Alice", Reply: replies.fn,
				Attachment: &bot.AttachmentEvent{
					Kind: test.kind, TGFileID: "opaque-telegram-file",
					FileName: "untrusted.exe", MIMEType: "application/octet-stream",
					Duration: 999, SizeBytes: 99 << 20, Broadcast: true,
				},
			})
			accepted := takeTelegramAcceptance(t, adapter)
			if accepted.AttachmentKind != string(test.kind) {
				t.Fatalf("accepted kind=%q", accepted.AttachmentKind)
			}
			if len(replies.texts) != 1 || !strings.Contains(replies.texts[0], "неподтверждённый файл") {
				t.Fatalf("acceptance replies=%v", replies.texts)
			}
			adapter.complete(accepted.MediaID, controlledTelegramCompletion{code: test.failure})
			l.handleMediaDone(takeMediaDone(t, l))
			if got := replies.last(t); got != test.wantReply {
				t.Fatalf("failure reply=%q want=%q", got, test.wantReply)
			}
		})
	}
}

func TestTelegramMediaGroupIsHonestlyRejectedBeforeIngest(t *testing.T) {
	l, _ := newTestLoop(t)
	adapter := newControlledTelegramAdapter(l.st)
	l.telegramMedia = adapter
	replies := &replies{}
	l.handleBot(bot.Event{
		FromUserID: 111, FromName: "Alice", Reply: replies.fn,
		Attachment: &bot.AttachmentEvent{
			Kind: bot.AttachmentAudio, TGFileID: "opaque-telegram-file", MediaGroupID: "group-hint",
		},
	})
	if got := replies.last(t); got != bot.AttachmentFailureText(bot.AttachmentGroupUnsupported) {
		t.Fatalf("reply=%q", got)
	}
	select {
	case accepted := <-adapter.accepted:
		t.Fatalf("media group reached ingest: %+v", accepted)
	default:
	}
}

func TestCallbackWithoutInlineRouterGetsPromptTerminalOutcome(t *testing.T) {
	l, _ := newTestLoop(t)
	var answer bot.CallbackAnswerCode
	cleared := false
	l.handleBot(bot.Event{
		FromUserID: 111, FromName: "Alice",
		Callback: &bot.CallbackEvent{
			Answer:        func(code bot.CallbackAnswerCode) { answer = code },
			ClearKeyboard: func() { cleared = true },
		},
	})
	if answer != bot.CallbackUnsupported || !cleared {
		t.Fatalf("answer=%s cleared=%v", answer, cleared)
	}
}

func TestLoopMediaCancellationSinkDisarmsQueueAndStopsActiveVoice(t *testing.T) {
	l, fake := newTestLoop(t)
	state := l.orbit(1)
	state.sess.Mode = session.ModeShared
	active := session.Element{
		ID: "active-legacy-voice", Kind: session.KindVoice,
		MediaID: "m_cancelled_voice", Target: "both", DurationMS: 1_000,
	}
	queuedCopy := active
	queuedCopy.ID = "queued-copy"
	state.sess.Current = &active
	state.sess.State = session.StateVoice
	state.sess.Queue = []session.Element{
		queuedCopy,
		{ID: "next-track", Kind: session.KindTrack, URI: "spotify:track:next", Target: "both"},
	}
	fake.drain()
	stop := make(chan struct{})
	go l.run(stop, make(chan hub.Event))
	t.Cleanup(func() {
		close(stop)
		<-l.stopped
	})
	request := store.MediaDeliveryCancellation{
		MediaID: active.MediaID, MediaRevision: 4,
		Reason:                store.MediaCancellationDeleted,
		PolicyVersion:         store.MediaLifecyclePolicyV1,
		NotStartedAction:      store.MediaNotStartedActionCancel,
		ActiveAction:          store.MediaActiveActionFadeStop,
		InterruptedMainAction: store.MediaInterruptedMainActionResumeOnce,
	}
	if err := l.CancelMedia(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if state.sess.Current == nil || state.sess.Current.ID != "next-track" ||
		len(state.sess.Queue) != 0 || state.sess.State != session.StateLoading {
		t.Fatalf("cancelled runtime state=%s current=%+v queue=%+v",
			state.sess.State, state.sess.Current, state.sess.Queue)
	}
	stops := fake.ofType(protocol.TypeStop)
	loads := fake.ofType(protocol.TypeLoad)
	if len(stops) != 2 || len(loads) != 2 {
		t.Fatalf("cancellation effects stops=%+v loads=%+v", stops, loads)
	}
	fake.drain()
	if err := l.CancelMedia(context.Background(), request); err != nil {
		t.Fatal(err)
	}
	if replay := fake.drain(); len(replay) != 0 || state.sess.Current.ID != "next-track" {
		t.Fatalf("replayed cancellation messages=%+v current=%+v", replay, state.sess.Current)
	}
}

func TestVoiceProcessingCompletionCannotReorderSenders(t *testing.T) {
	l, _ := newTestLoop(t)
	l.voiceNext[1] = 1
	l.voiceAccepted[1] = 2
	reply := func(string) {}
	done := func(seq int64, mediaID, from string, acceptedAt int64) mediaDone {
		return mediaDone{
			orbit: 1, mediaID: mediaID, fromName: from,
			acceptedAt: acceptedAt, sequence: seq,
			result: media.Result{DurationMS: 1_000, WAVPath: "/tmp/" + mediaID + ".wav", LoudnormJSON: "{}"},
			reply:  reply,
		}
	}

	// Bob's shorter message finishes ffmpeg first, but must wait for Alice's
	// earlier Telegram update.
	l.handleMediaDone(done(2, "m_bob", "Bob", 2_000))
	if cur := l.orbit(1).sess.Current; cur != nil {
		t.Fatalf("later voice escaped the reorder buffer: %+v", cur)
	}
	l.handleMediaDone(done(1, "m_alice", "Alice", 1_000))
	o := l.orbit(1)
	if o.sess.Current == nil || o.sess.Current.MediaID != "m_alice" {
		t.Fatalf("first voice = %+v, want Alice", o.sess.Current)
	}
	if o.sess.QueueLen() != 1 || o.sess.Queue[0].MediaID != "m_bob" {
		t.Fatalf("second voice queue = %+v, want Bob", o.sess.Queue)
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

// M1 regression: the hub can emit a stale EvOffline right after the reader's
// EvOnline (both flip state under the lock but emit after unlock). The hub map
// ends "online", so no further EvOnline ever comes — the FSM would believe the
// home is dark forever while it heartbeats normally. Any message from a known
// peer must therefore count as proof of life.
func TestMessageFromFSMOfflinePeerRevivesIt(t *testing.T) {
	l, fake := newTestLoop(t)
	r := &replies{}
	l.handleNodeMessage(hub.EvMessage{Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeA}, Payload: &protocol.StatePayload{PositionMS: 0, RTTMS: 40, Volume: 80}})
	l.handleNodeMessage(hub.EvMessage{Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeB}, Payload: &protocol.StatePayload{PositionMS: 0, RTTMS: 60, Volume: 80}})
	l.handleBot(cmdEvent(t, "a", link, r)) // loading, both homes on the barrier
	fake.drain()

	// The stale EvOffline lands: the FSM parks the strict orbit.
	l.handleNode(hub.EvOffline{Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeB}})
	if st := l.orbit(1).sess.State; st != session.StateDegraded {
		t.Fatalf("precondition: parked, got %s", st)
	}

	// b keeps talking (hub still believes it online — no EvOnline will come).
	l.handleNodeMessage(hub.EvMessage{Key: hub.NodeKey{Orbit: 1, Slot: protocol.NodeB}, Payload: &protocol.StatePayload{PositionMS: 1000, RTTMS: 60, Volume: 80}})
	if st := l.orbit(1).sess.State; st == session.StateDegraded {
		t.Fatalf("air still parked though the 'offline' home is talking")
	}
	if !l.orbit(1).sess.IsOnline(protocol.NodeB) {
		t.Fatal("proof-of-life must mark b online in the FSM")
	}
}
