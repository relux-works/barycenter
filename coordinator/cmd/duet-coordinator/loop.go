package main

import (
	"errors"
	"fmt"
	"html"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"relux.works/duet/coordinator/internal/bot"
	"relux.works/duet/coordinator/internal/config"
	"relux.works/duet/coordinator/internal/hub"
	"relux.works/duet/coordinator/internal/media"
	"relux.works/duet/coordinator/internal/protocol"
	"relux.works/duet/coordinator/internal/session"
	"relux.works/duet/coordinator/internal/spotify"
	"relux.works/duet/coordinator/internal/store"
	"relux.works/duet/coordinator/internal/ulid"
)

// nodeSender abstracts the ws-hub for loop tests.
type nodeSender interface {
	Send(key hub.NodeKey, msgType string, payload any) bool
	Online(orbitID int64) map[protocol.NodeID]bool
}

// orbitState is everything the loop tracks per orbit (v2.1 multi-tenant):
// one FSM session plus its knobs, timers and last-seen node telemetry.
// Group sessions (design §12 approaches) reuse the same shape with id =
// -linkID and peer ids in composite "orbit:slot" form.
type orbitState struct {
	id    int64
	title string
	sess  *session.Session

	// orbits: for group states, the linked orbit ids; nil for plain orbits.
	orbits []int64

	takeoverPolicy string // user | coordinator (per-orbit, orbits table)
	voiceDefault   string // personal | broadcast

	volumes  map[protocol.NodeID]int
	offsets  map[protocol.NodeID]int64
	lastSeen map[protocol.NodeID]*protocol.StatePayload
	versions map[protocol.NodeID]string
	// restoredPaused: after coordinator restart, resume position must follow
	// live heartbeats until the user resumes (spec 7.2).
	restoredPaused bool
	lastDesyncMS   int64

	readyTimer   *time.Timer
	timerElement string
}

// group reports whether this state is a link (approach) session.
func (o *orbitState) group() bool { return o.id < 0 }

// loop serializes every session-affecting event across all orbits
// (spec 7.2: the FSM is single-threaded; one goroutine, many orbits).
type loop struct {
	log *slog.Logger
	cfg *config.Config
	hub nodeSender
	st  *store.Store
	bot *bot.Bot        // nil when telegram is disabled (dev mode)
	sp  *spotify.Client // nil when spotify app creds are not configured (U10)

	// resolveTrack runs the provider cascade for one track (providers.go).
	// nil while the provider layer is off; tests stub it directly.
	resolveTrack resolveTrackFn

	states map[int64]*orbitState

	// Approaches (design §12 L1): linkOf maps an orbit to its active link id
	// (absent when solo), groups holds the shared per-link sessions.
	linkOf map[int64]int64
	groups map[int64]*orbitState

	timeouts   chan orbitTimeout
	mediaCh    chan mediaDone
	playlistCh chan playlistDone
}

type orbitTimeout struct {
	orbit     int64
	elementID string
}

type playlistDone struct {
	orbit  int64
	uri    string
	title  string
	tracks []string
	err    error
	reply  func(string)
}

type mediaDone struct {
	orbit    int64
	mediaID  string
	from     int64 // tg user id of the sender
	fromName string
	personal bool
	result   media.Result
	err      error
	reply    func(string)
}

func newLoop(log *slog.Logger, cfg *config.Config, h nodeSender, st *store.Store, b *bot.Bot, sp *spotify.Client) *loop {
	return &loop{
		log:        log,
		cfg:        cfg,
		hub:        h,
		st:         st,
		bot:        b,
		sp:         sp,
		states:     map[int64]*orbitState{},
		linkOf:     map[int64]int64{},
		groups:     map[int64]*orbitState{},
		timeouts:   make(chan orbitTimeout, 8),
		mediaCh:    make(chan mediaDone, 8),
		playlistCh: make(chan playlistDone, 4),
	}
}

// orbit returns the live state for an orbit, restoring it from the store on
// first touch (session snapshot, knobs, per-orbit settings).
func (l *loop) orbit(id int64) *orbitState {
	if o, ok := l.states[id]; ok {
		return o
	}
	o := &orbitState{
		id:             id,
		title:          "Барицентр",
		sess:           session.New(),
		takeoverPolicy: "user",
		voiceDefault:   "personal",
		volumes:        map[protocol.NodeID]int{},
		offsets:        map[protocol.NodeID]int64{},
		lastSeen:       map[protocol.NodeID]*protocol.StatePayload{},
		versions:       map[protocol.NodeID]string{},
	}
	o.sess.StartMarginMS = int64(l.cfg.Timings.StartMarginMS)
	if rec, err := l.st.GetOrbit(id); err == nil && rec != nil {
		o.title = rec.Title
		o.takeoverPolicy = rec.TakeoverPolicy
		o.voiceDefault = rec.VoiceDefault
	}
	slots, _ := l.st.ActiveSlots(id)
	// M2: the broadcast machinery runs over the orbit's real slot set.
	o.sess.SetPeers(slots)
	for _, sl := range slots {
		n := protocol.NodeID(sl)
		o.volumes[n] = 80
		if v, _ := l.st.GetSetting(fmt.Sprintf("volume_%d_%s", id, sl)); v != "" {
			if i, err := strconv.Atoi(v); err == nil {
				o.volumes[n] = i
			}
		}
		if v, _ := l.st.GetSetting(fmt.Sprintf("offset_%d_%s", id, sl)); v != "" {
			if i, err := strconv.ParseInt(v, 10, 64); err == nil {
				o.offsets[n] = i
			}
		}
	}
	l.restoreSnapshot(o)
	l.states[id] = o
	return o
}

// restoreSnapshot loads the persisted FSM state into o. o.id doubles as the
// storage key: positive for orbits, -linkID for group sessions.
func (l *loop) restoreSnapshot(o *orbitState) {
	snap, err := l.st.LoadSession(o.id)
	if err != nil || snap == nil {
		return
	}
	o.sess.Mode = snap.Mode
	o.sess.State = snap.State
	o.sess.Current = snap.Current
	o.sess.SavedPositionMS = snap.SavedPositionMS
	o.sess.Queue = snap.Queue
	o.sess.Playlist = snap.Playlist
	if snap.State == session.StatePaused && snap.Current != nil {
		o.restoredPaused = true
		// A linked orbit's own session is dormant behind the approach — the
		// group session speaks for the air, no restart noise from it.
		if o.group() || l.linkOf[o.id] == 0 {
			l.notify(o, "координатор перезапустился: эфир на паузе, /resume чтобы продолжить")
		}
	}
	l.log.Info("session restored", "orbit", o.id, "state", snap.State, "queue_len", len(snap.Queue))
}

// --- Approaches (design §12 L1): composite peers & group sessions ---

// compositeID addresses a pulsar inside a group session: "orbit:slot"
// (protocol.NodeID is a string, the FSM is agnostic to the shape).
func compositeID(orbit int64, slot protocol.NodeID) protocol.NodeID {
	return protocol.NodeID(strconv.FormatInt(orbit, 10) + ":" + string(slot))
}

// splitComposite parses "orbit:slot"; ok=false for bare slot ids.
func splitComposite(id protocol.NodeID) (int64, protocol.NodeID, bool) {
	orbit, slot, found := strings.Cut(string(id), ":")
	if !found {
		return 0, "", false
	}
	n, err := strconv.ParseInt(orbit, 10, 64)
	if err != nil {
		return 0, "", false
	}
	return n, protocol.NodeID(slot), true
}

// peerFor maps a hub node to its id inside o's session: bare slot for plain
// orbits, composite for groups.
func (l *loop) peerFor(o *orbitState, key hub.NodeKey) protocol.NodeID {
	if o.group() {
		return compositeID(key.Orbit, key.Slot)
	}
	return key.Slot
}

// nodeKey resolves a session peer id back to its wire address (the effect
// boundary: composite ids never leave the loop).
func (l *loop) nodeKey(o *orbitState, to protocol.NodeID) hub.NodeKey {
	if o.group() {
		if orbit, slot, ok := splitComposite(to); ok {
			return hub.NodeKey{Orbit: orbit, Slot: slot}
		}
	}
	return hub.NodeKey{Orbit: o.id, Slot: to}
}

// stateFor returns the session that owns an orbit's air: the group state
// while an approach is active, the personal orbit state otherwise.
func (l *loop) stateFor(orbitID int64) *orbitState {
	if linkID := l.linkOf[orbitID]; linkID != 0 {
		if g := l.group(linkID); g != nil {
			return g
		}
		delete(l.linkOf, orbitID) // stale link: fall back to the personal orbit
	}
	return l.orbit(orbitID)
}

// stateByID resolves a state id after a timer/channel roundtrip: negative
// ids are group sessions (-linkID) and may be gone when the event lands.
func (l *loop) stateByID(id int64) *orbitState {
	if id < 0 {
		return l.groups[-id]
	}
	return l.stateFor(id)
}

// group returns the live state of an active approach, building it on first
// touch: peers = union of both orbits' slots as composite ids, liveness
// seeded from current hub knowledge (the hub only emits EvOnline on real
// transitions — a session born mid-flight must not wait for one), snapshot
// restored like a regular orbit. Returns nil when the link row is gone.
func (l *loop) group(linkID int64) *orbitState {
	if g, ok := l.groups[linkID]; ok {
		return g
	}
	lk, err := l.st.GetLink(linkID)
	if err != nil || lk == nil || lk.State != "active" {
		return nil
	}
	g := &orbitState{
		id:             -linkID,
		orbits:         []int64{lk.OrbitA, lk.OrbitB},
		title:          l.orbit(lk.OrbitA).title + " ⇄ " + l.orbit(lk.OrbitB).title,
		sess:           session.New(),
		takeoverPolicy: "user",     // groups arbitrate per interfering orbit
		voiceDefault:   "personal", // groups read the sender's orbit setting
		volumes:        map[protocol.NodeID]int{},
		offsets:        map[protocol.NodeID]int64{},
		lastSeen:       map[protocol.NodeID]*protocol.StatePayload{},
		versions:       map[protocol.NodeID]string{},
	}
	g.sess.StartMarginMS = int64(l.cfg.Timings.StartMarginMS)
	var peers []string
	online := map[protocol.NodeID]bool{}
	for _, orbitID := range g.orbits {
		slots, _ := l.st.ActiveSlots(orbitID)
		for _, sl := range slots {
			n := compositeID(orbitID, protocol.NodeID(sl))
			peers = append(peers, string(n))
			g.volumes[n] = 80
			if v, _ := l.st.GetSetting(fmt.Sprintf("volume_%d_%s", orbitID, sl)); v != "" {
				if i, err := strconv.Atoi(v); err == nil {
					g.volumes[n] = i
				}
			}
			if v, _ := l.st.GetSetting(fmt.Sprintf("offset_%d_%s", orbitID, sl)); v != "" {
				if i, err := strconv.ParseInt(v, 10, 64); err == nil {
					g.offsets[n] = i
				}
			}
		}
		for sl, on := range l.hub.Online(orbitID) {
			online[compositeID(orbitID, sl)] = on
		}
	}
	g.sess.SetPeers(peers)
	// Living air (§12 L1.5): a linked broadcast starts when each SIDE (orbit)
	// has one home online; the composite peer's orbit prefix is its side.
	g.sess.GateMode = session.GateEachSide
	g.sess.SideOf = func(n protocol.NodeID) string {
		if orbitID, _, ok := splitComposite(n); ok {
			return strconv.FormatInt(orbitID, 10)
		}
		return string(n)
	}
	g.sess.SeedOnline(online)
	l.restoreSnapshot(g)
	l.groups[linkID] = g
	return g
}

// warmup restores every known orbit and active approach at startup.
func (l *loop) warmup() {
	links, err := l.st.ActiveLinks()
	if err != nil {
		l.log.Error("link warmup failed", "err", err)
	}
	for _, lk := range links {
		l.linkOf[lk.OrbitA] = lk.ID
		l.linkOf[lk.OrbitB] = lk.ID
	}
	ids, err := l.st.OrbitIDs()
	if err != nil {
		l.log.Error("orbit warmup failed", "err", err)
		return
	}
	for _, id := range ids {
		l.orbit(id)
	}
	// Group sessions after orbits: titles and parked sessions are warm.
	for _, lk := range links {
		l.group(lk.ID)
	}
	l.log.Info("orbits warmed up", "count", len(ids), "links", len(links))
}

func (l *loop) run(stop <-chan struct{}, nodeEvents <-chan hub.Event) {
	var botEvents chan bot.Event
	if l.bot != nil {
		botEvents = l.bot.Events
	}
	for {
		select {
		case <-stop:
			return
		case ev := <-nodeEvents:
			l.handleNode(ev)
		case to := <-l.timeouts:
			// A group timer may outlive its link; drop it then.
			if o := l.stateByID(to.orbit); o != nil {
				l.apply(o, o.sess.OnReadyTimeout(to.elementID))
			}
		case ev := <-botEvents:
			l.handleBot(ev)
		case done := <-l.mediaCh:
			l.handleMediaDone(done)
		case d := <-l.playlistCh:
			l.handlePlaylistDone(d)
		}
	}
}

func (l *loop) handlePlaylistDone(d playlistDone) {
	if d.err != nil {
		l.log.Warn("playlist expansion failed", "uri", d.uri, "err", d.err)
		d.reply(fmt.Sprintf("не смог раскрыть плейлист: %v", d.err))
		return
	}
	o := l.stateFor(d.orbit)
	l.apply(o, o.sess.SetPlaylist(d.uri, esc(d.title), d.tracks))
}

// notify DMs every member of the session's orbit — both linked orbits for a
// group session (group chat binding comes in M4).
var compositePeerRe = regexp.MustCompile(`\b(\d+):([a-z])\b`)

// humanizePeers rewrites raw composite peer ids ("1:a") from core-produced
// texts into "a@«Title»" — the FSM stays pure, rendering lives here.
func (l *loop) humanizePeers(text string) string {
	return compositePeerRe.ReplaceAllStringFunc(text, func(m string) string {
		parts := compositePeerRe.FindStringSubmatch(m)
		var orbitID int64
		fmt.Sscanf(parts[1], "%d", &orbitID)
		title := "?"
		if rec, err := l.st.GetOrbit(orbitID); err == nil && rec != nil {
			title = rec.Title
		}
		return parts[2] + "@«" + esc(title) + "»"
	})
}

func (l *loop) notify(o *orbitState, text string) {
	text = l.humanizePeers(text)
	if o.group() {
		for _, id := range o.orbits {
			l.notifyOrbit(id, text)
		}
		return
	}
	l.notifyOrbit(o.id, text)
}

func (l *loop) notifyOrbit(orbitID int64, text string) {
	l.log.Info("notify", "orbit", orbitID, "text", text)
	if l.bot == nil {
		return
	}
	members, err := l.st.Members(orbitID)
	if err != nil {
		l.log.Error("members lookup failed", "orbit", orbitID, "err", err)
		return
	}
	for _, m := range members {
		l.bot.SendTo(m.TGUserID, text)
	}
}

// --- Link lifecycle: activation and dissolution (design §12 L1) ---

// parkOrbitSession freezes a personal orbit's own broadcast for the duration
// of an approach: nodes get stop, the session persists paused at the live
// position and waits, shadowed by the group session, until /apart.
func (l *loop) parkOrbitSession(o *orbitState) {
	if effs := o.sess.CmdPause(); effs != nil {
		l.apply(o, effs)
	} else if o.sess.State == session.StateVoice && o.sess.Current != nil {
		// CmdPause skips VOICE; a paused voice restarts from scratch on
		// resume anyway (CmdResume rule), freeze it as paused directly.
		o.sess.State = session.StatePaused
	}
	l.cancelReadyTimer(o)
	for _, p := range o.sess.Peers {
		l.hub.Send(hub.NodeKey{Orbit: o.id, Slot: p}, protocol.TypeStop, &protocol.StopPayload{})
	}
	l.persist(o)
}

// startGroup activates an approach: both orbits' own broadcasts park, one
// shared session takes over the union of their homes.
func (l *loop) startGroup(linkID, orbitA, orbitB int64) {
	// Approach-to-stream (§12 L1.5): the code issuer's stream continues onto
	// all homes; if the issuer is silent, the acceptor's does; both silent ->
	// blank group. Capture BEFORE parking (live positions are still fresh).
	donor, cur, pos, queue, playlist := l.captureDonor(orbitA, orbitB)

	l.parkOrbitSession(l.orbit(orbitA))
	l.parkOrbitSession(l.orbit(orbitB))
	if donor != 0 {
		l.emptyParkedSnapshot(donor) // transplanted content must not double
	}
	l.linkOf[orbitA] = linkID
	l.linkOf[orbitB] = linkID
	g := l.group(linkID)

	l.notifyOrbit(orbitA, fmt.Sprintf("сближение с «%s» началось — эфир общий", esc(l.orbit(orbitB).title)))
	l.notifyOrbit(orbitB, fmt.Sprintf("сближение с «%s» началось — эфир общий", esc(l.orbit(orbitA).title)))
	l.log.Info("approach started", "link", linkID, "orbit_a", orbitA, "orbit_b", orbitB, "donor", donor)

	if donor != 0 && (cur != nil || len(queue) > 0 || playlist != nil) {
		l.apply(g, g.sess.Transplant(cur, pos, queue, playlist))
	}
}

// captureDonor picks the side whose stream continues and snapshots its
// content at the live position. Issuer (orbitA) wins if playing/queued;
// else the acceptor (orbitB); else donor=0 (blank group).
func (l *loop) captureDonor(orbitA, orbitB int64) (donor int64, cur *session.Element, pos int64, queue []session.Element, playlist *session.Playlist) {
	pick := func(id int64) bool {
		o := l.orbit(id)
		return o.sess.Current != nil || len(o.sess.Queue) > 0 ||
			(o.sess.Playlist != nil && o.sess.Playlist.Cursor < len(o.sess.Playlist.Tracks))
	}
	switch {
	case pick(orbitA):
		donor = orbitA
	case pick(orbitB):
		donor = orbitB
	default:
		return 0, nil, 0, nil, nil
	}
	o := l.orbit(donor)
	if o.sess.Current != nil {
		c := *o.sess.Current
		cur = &c
		pos = o.sess.LivePositionForTransplant()
	}
	queue = append([]session.Element{}, o.sess.Queue...)
	playlist = o.sess.Playlist
	return
}

// emptyParkedSnapshot clears the donor's own session so its parked snapshot
// does not replay content already transplanted into the group.
func (l *loop) emptyParkedSnapshot(donor int64) {
	o := l.orbit(donor)
	o.sess.Current = nil
	o.sess.Queue = nil
	o.sess.Playlist = nil
	o.sess.State = session.StateIdle
	l.persist(o)
}

// breakGroup dissolves an approach: the shared session dies, each orbit
// returns to its own solo session (design §12: breaking up is painless).
func (l *loop) breakGroup(linkID int64, orbits ...int64) {
	if g := l.group(linkID); g != nil {
		l.cancelReadyTimer(g)
		for _, p := range g.sess.Peers {
			l.hub.Send(l.nodeKey(g, p), protocol.TypeStop, &protocol.StopPayload{})
		}
	}
	delete(l.groups, linkID)
	l.st.BreakLink(linkID)
	l.st.ClearSession(-linkID)
	for _, id := range orbits {
		delete(l.linkOf, id)
		// The orbit session slept through the link: EvOnline/EvOffline went
		// to the group, its liveness map is stale — reseed from the hub.
		o := l.orbit(id)
		o.sess.SeedOnline(l.hub.Online(id))
		l.notifyOrbit(id, "сближение завершено — каждый у себя")
	}
	l.log.Info("approach ended", "link", linkID)
}

func (l *loop) persist(o *orbitState) {
	err := l.st.SaveSession(o.id, store.SessionSnapshot{
		Mode:            o.sess.Mode,
		State:           o.sess.State,
		Current:         o.sess.Current,
		SavedPositionMS: o.sess.SavedPositionMS,
		Queue:           o.sess.Queue,
		Playlist:        o.sess.Playlist,
	})
	if err != nil {
		l.log.Error("persist failed", "orbit", o.id, "err", err)
	}
}

// --- Node events ---

func (l *loop) handleNode(ev hub.Event) {
	switch e := ev.(type) {
	case hub.EvRegistered:
		o := l.stateFor(e.Key.Orbit)
		n := l.peerFor(o, e.Key)
		o.sess.EnsurePeer(n) // a slot paired after orbit/group warm-up
		l.log.Info("node registered", "orbit", e.Key.Orbit, "slot", e.Key.Slot, "app", e.AppVersion, "librespot", e.LibrespotVersion)
		o.versions[n] = e.AppVersion + "/librespot " + e.LibrespotVersion
		vol, ok := o.volumes[n]
		if !ok {
			vol = 80
			o.volumes[n] = vol
		}
		snap := o.sess.Snapshot(vol)
		l.hub.Send(e.Key, protocol.TypeWelcome, &protocol.WelcomePayload{SessionSnapshot: snap})
		if off, ok := o.offsets[n]; ok {
			l.hub.Send(e.Key, protocol.TypeSetOffset, &protocol.SetOffsetPayload{OffsetMS: off})
		}
	case hub.EvOnline:
		o := l.stateFor(e.Key.Orbit)
		peer := l.peerFor(o, e.Key)
		// Living air (§12 L1.5): a home returning to a playing group catches
		// up individually instead of the strict pause/resume dance.
		if join := o.sess.JoinInProgress(peer); join != nil {
			l.apply(o, join)
		} else {
			l.apply(o, o.sess.OnNodeBack(peer))
		}
	case hub.EvOffline:
		o := l.stateFor(e.Key.Orbit)
		l.log.Warn("node offline", "orbit", e.Key.Orbit, "slot", e.Key.Slot)
		l.st.LogEvent(string(e.Key.Slot), "offline", nil)
		l.apply(o, o.sess.OnNodeOffline(l.peerFor(o, e.Key)))
	case hub.EvMessage:
		l.handleNodeMessage(e)
	}
}

func (l *loop) handleNodeMessage(m hub.EvMessage) {
	o := l.stateFor(m.Key.Orbit)
	slot := l.peerFor(o, m.Key)
	now := time.Now().UnixMilli()
	switch p := m.Payload.(type) {
	case *protocol.StatePayload:
		o.lastSeen[slot] = p
		o.volumes[slot] = p.Volume
		l.apply(o, o.sess.OnHeartbeat(slot, p.PositionMS, p.RTTMS))
		if o.restoredPaused {
			o.sess.RefreshSavedPosition()
		}
	case *protocol.ReadyPayload:
		l.apply(o, o.sess.OnReady(now, slot, p.ElementID))
	case *protocol.StartedPayload:
		l.apply(o, o.sess.OnStarted(slot, p.ElementID, p.TFirstSampleCoordMS))
	case *protocol.EndedPayload:
		l.apply(o, o.sess.OnEnded(slot, p.ElementID, p.Reason))
	case *protocol.VoiceStartedPayload:
		l.log.Info("voice started", "orbit", o.id, "slot", slot, "element", p.ElementID)
	case *protocol.VoiceEndedPayload:
		l.apply(o, o.sess.OnVoiceEnded(slot, p.ElementID))
	case *protocol.WaitEndedPayload:
		l.apply(o, o.sess.OnWaitEnded(slot, p.ElementID))
	case *protocol.ErrorPayload:
		l.log.Warn("node error", "orbit", o.id, "slot", slot, "code", p.Code, "msg", p.Message, "element", p.ElementID)
		l.st.LogEvent(string(slot), "node_error", p)
		l.apply(o, o.sess.OnNodeError(slot, p.Code, p.ElementID))
	case *protocol.ExternalPlaybackPayload:
		l.handleExternalPlayback(o, m.Key, p.URI)
	default:
		l.log.Debug("unhandled message", "slot", slot, "type", m.Env.Type)
	}
}

// handleExternalPlayback applies the takeover policy (U9). In a group
// session the interfering home's own barycenter policy arbitrates.
func (l *loop) handleExternalPlayback(o *orbitState, key hub.NodeKey, uri string) {
	if o.sess.Mode != session.ModeShared {
		return
	}
	id := l.peerFor(o, key)
	policy := o.takeoverPolicy
	if o.group() {
		policy = l.orbit(key.Orbit).takeoverPolicy
	}
	l.st.LogEvent(string(key.Slot), "external_playback", map[string]string{"uri": uri, "policy": policy})
	// The policy arbitrates a CONFLICT over a busy broadcast. An empty air
	// has nothing to defend: the phone is always welcome, both policies
	// step aside into apoastron (customer finding, R0 prod 2026-07-05).
	busy := o.sess.Current != nil || o.sess.QueueLen() > 0 ||
		(o.sess.Playlist != nil && o.sess.Playlist.Cursor < len(o.sess.Playlist.Tracks))
	if policy == "user" || !busy {
		l.notify(o, fmt.Sprintf("дом %s слушает своё — апоастрон", l.peerName(o, id)))
		l.apply(o, o.sess.SetModeSoloKeeping(id))
		return
	}
	l.notify(o, fmt.Sprintf("дом %s вмешался с телефона — эфир восстановлен", l.peerName(o, id)))
	if o.sess.State == session.StatePlaying {
		l.apply(o, o.sess.CmdSync())
		return
	}
	l.hub.Send(key, protocol.TypeStop, &protocol.StopPayload{})
}

// --- Bot events: onboarding, roles, commands (spec ch. 9 + v2.1 M1) ---

const strangerHello = `Привет! Я <b>Барицентр</b> — общий музыкальный эфир на несколько домов: одна и та же музыка звучит синхронно у всех, очередью управляет этот чат, а голосовые встают между песнями.

<b>Как начать</b>
/create — создать свой барицентр
…или открой инвайт-ссылку от того, кто уже в системе.`

func (l *loop) handleBot(ev bot.Event) {
	member, err := l.st.MemberOf(ev.FromUserID)
	if err != nil {
		l.log.Error("membership lookup failed", "err", err)
		return
	}
	if member == nil {
		l.handleStranger(ev)
		return
	}
	// home is the member's personal barycenter (admin & settings); o is the
	// air — the shared group session while an approach is active (§12 L1).
	home := l.orbit(member.OrbitID)
	o := l.stateFor(member.OrbitID)

	if ev.Voice != nil {
		if member.Role == "satellite" && false { // satellites may voice: allowed by design
			return
		}
		l.handleVoice(home, ev)
		return
	}

	cmd := ev.Command

	// Role gate: satellites contribute (tracks, voices, info) but do not
	// steer the air (design §2).
	if member.Role == "satellite" {
		switch cmd.Kind {
		case bot.KindLink, bot.KindQueue, bot.KindNow, bot.KindStatus,
			bot.KindStart, bot.KindShare, bot.KindOrbit, bot.KindPairCode:
		default:
			ev.Reply("это управление эфиром — оно у companion'ов. Твоё оружие: треки и голосовые")
			return
		}
	}

	switch cmd.Kind {

	case bot.KindStart:
		ev.Reply(fmt.Sprintf("ты уже в орбите <b>«%s»</b>.\n/help — команды · /orbit — участники", esc(home.title)))

	case bot.KindCreate:
		ev.Reply(fmt.Sprintf("у тебя уже есть орбит <b>«%s»</b> — вторая вселенная пока не положена", esc(home.title)))

	case bot.KindShare:
		code, err := l.st.NewInvite(home.id, ev.FromUserID)
		if err != nil {
			ev.Reply("не смог создать приглашение")
			return
		}
		link := fmt.Sprintf("https://t.me/%s?start=%s", l.botUsername(), code)
		ev.Reply(fmt.Sprintf("приглашение в <b>«%s»</b> — одноразовое, живёт 48 часов:\n\n%s", esc(home.title), link))

	case bot.KindPairCode:
		code, err := l.st.NewPairCode(home.id, ev.FromUserID)
		if err != nil {
			ev.Reply("не смог создать код")
			return
		}
		ev.Reply(fmt.Sprintf("код для твоего Пульсара — живёт 5 минут:\n\n<code>%s</code>\n\nВведи его в приложении Pulsar при первом запуске, и твой дом подключится к эфиру.", code))

	case bot.KindOrbit:
		ev.Reply(l.orbitText(home))

	case bot.KindMakePrimary:
		if member.Role != "primary" {
			ev.Reply("передать главную звезду может только primary")
			return
		}
		if cmd.Number == 0 {
			ev.Reply(l.orbitText(home) + "\n\n/make_primary <id> передаст титул")
			return
		}
		if err := l.st.TransferPrimary(home.id, int64(cmd.Number)); err != nil {
			ev.Reply("этот id не из нашего орбита (/orbit покажет список)")
			return
		}
		l.notify(home, "главная звезда орбита теперь "+strconv.Itoa(cmd.Number))

	case bot.KindRevoke:
		if member.Role != "primary" {
			ev.Reply("отзывать дома может только primary")
			return
		}
		if err := l.st.RevokeSlot(home.id, cmd.Target); err != nil {
			ev.Reply("не получилось")
			return
		}
		// The revoked slot leaves the peer set of the LIVE session (the
		// group one during an approach) so it stops blocking ready barriers
		// and offline gates (M2).
		revoked := l.peerFor(o, hub.NodeKey{Orbit: home.id, Slot: protocol.NodeID(cmd.Target)})
		l.apply(o, o.sess.RemovePeer(time.Now().UnixMilli(), revoked))
		ev.Reply(fmt.Sprintf("токен дома %s отозван; /pair выдаст новый код", cmd.Target))

	case bot.KindLink:
		// Provider layer off: non-spotify refs keep the pre-provider answer
		// (flag inertness — the parser recognizes yandex links regardless).
		if !l.cfg.Providers && !strings.HasPrefix(cmd.URI, "spotify:") {
			ev.Reply("такие ссылки не поддерживаю — кидай трек, плейлист или альбом")
			return
		}
		if o.sess.Mode != session.ModeShared {
			ev.Reply("сейчас режим solo: /inject подкинет трек партнёру, /periastron вернёт общий эфир")
			return
		}
		el := l.newTrackElement(cmd.URI, ev.FromName)
		if l.cfg.Providers && !l.resolveElement(o, &el, ev.Reply) {
			return // P1 strict air: rejection already replied (spec-providers §4.2)
		}
		l.st.InsertElement(el)
		l.apply(o, o.sess.EnqueueTrack(el))
		if o.sess.Current != nil && o.sess.Current.ID == el.ID {
			ev.Reply("очередь пуста — ставлю сразу: " + trackLabel(el))
		} else {
			ev.Reply(fmt.Sprintf("добавил в очередь под номером %d: %s", o.sess.QueueLen(), trackLabel(el)))
		}

	case bot.KindPlayNow:
		if o.sess.Mode != session.ModeShared {
			ev.Reply("/playnow работает в shared. Сейчас solo")
			return
		}
		el := l.newTrackElement(cmd.URI, ev.FromName)
		l.st.InsertElement(el)
		l.apply(o, o.sess.CmdPlayNow(el))
		ev.Reply("врубаю немедленно: " + trackLabel(el))

	case bot.KindPlaylist:
		if o.sess.Mode != session.ModeShared {
			ev.Reply("общий плейлист — фича shared-режима. Сейчас solo")
			return
		}
		if l.sp == nil {
			ev.Reply("плейлисты заработают после настройки Spotify-приложения на сервере")
			return
		}
		ev.Reply("раскрываю плейлист…")
		uri := cmd.URI
		kind := cmd.Target
		id := uri[strings.LastIndex(uri, ":")+1:]
		reply := ev.Reply
		orbitID := member.OrbitID // re-resolved on completion: the link may change meanwhile
		go func() {
			var exp *spotify.Expansion
			var err error
			if kind == "album" {
				exp, err = l.sp.ExpandAlbum(id)
			} else {
				exp, err = l.sp.ExpandPlaylist(id)
			}
			d := playlistDone{orbit: orbitID, uri: uri, err: err, reply: reply}
			if exp != nil {
				d.title = exp.Title
				d.tracks = exp.Tracks
			}
			l.playlistCh <- d
		}()

	case bot.KindTakeover:
		home.takeoverPolicy = cmd.Target
		l.st.SetOrbitSetting(home.id, "takeover_policy", cmd.Target)
		if cmd.Target == "user" {
			ev.Reply("политика: телефон главнее — вмешательство переключает орбит в solo (с уведомлением)")
		} else {
			ev.Reply("политика: эфир главнее — вмешательство с телефона откатывается (с уведомлением)")
		}

	case bot.KindQueue:
		ev.Reply(l.queueText(o))

	case bot.KindCancel:
		if _, err := o.sess.Cancel(cmd.Number); err != nil {
			ev.Reply(fmt.Sprintf("в очереди %d элементов, номера %d нет", o.sess.QueueLen(), cmd.Number))
			return
		}
		l.persist(o)
		ev.Reply(fmt.Sprintf("убрал элемент %d, в очереди осталось %d", cmd.Number, o.sess.QueueLen()))

	case bot.KindSkip:
		if effs := o.sess.CmdSkip(); effs != nil {
			l.apply(o, effs)
			ev.Reply("пропустил")
		} else {
			ev.Reply("нечего пропускать")
		}

	case bot.KindPause:
		if effs := o.sess.CmdPause(); effs != nil {
			l.apply(o, effs)
			ev.Reply("пауза")
		} else {
			ev.Reply("и так не играет")
		}

	case bot.KindResume:
		if effs := o.sess.CmdResume(); effs != nil {
			o.restoredPaused = false
			l.apply(o, effs)
			ev.Reply("продолжаю")
		} else {
			ev.Reply("нечего продолжать — пришли ссылку на трек")
		}

	case bot.KindSync:
		if effs := o.sess.CmdSync(); effs != nil {
			l.apply(o, effs)
			ev.Reply("пересинхронизирую с текущей позиции")
		} else {
			ev.Reply("/sync работает во время игры трека")
		}

	case bot.KindVol:
		// Slot letters address the sender's OWN barycenter, linked or not.
		target := protocol.NodeID(cmd.Target)
		if cmd.Target == "" {
			slot, _ := l.st.SlotOf(home.id, ev.FromUserID)
			if slot == "" {
				ev.Reply("у тебя нет своего дома в орбите — укажи слот: /vol " + strconv.Itoa(cmd.Number) + " a")
				return
			}
			target = protocol.NodeID(slot)
		}
		key := hub.NodeKey{Orbit: home.id, Slot: target}
		o.volumes[l.peerFor(o, key)] = cmd.Number
		l.st.SetSetting(fmt.Sprintf("volume_%d_%s", home.id, target), strconv.Itoa(cmd.Number))
		if !l.hub.Send(key, protocol.TypeSetVolume, &protocol.SetVolumePayload{Volume: cmd.Number}) {
			ev.Reply(fmt.Sprintf("дом %s офлайн, громкость применю при подключении", target))
			return
		}
		ev.Reply(fmt.Sprintf("громкость дома %s: %d", target, cmd.Number))

	case bot.KindMode:
		var effs []session.Effect
		if cmd.Target == "solo" {
			effs = o.sess.SetModeSolo()
		} else {
			effs = o.sess.SetModeShared()
		}
		if effs == nil {
			ev.Reply("уже в этом режиме")
			return
		}
		l.apply(o, effs)

	case bot.KindNow:
		ev.Reply(l.nowText(o))

	case bot.KindStatus:
		ev.Reply(l.statusText(o))

	case bot.KindOffset:
		target := protocol.NodeID(cmd.Target)
		key := hub.NodeKey{Orbit: home.id, Slot: target}
		o.offsets[l.peerFor(o, key)] = int64(cmd.Number)
		l.st.SetSetting(fmt.Sprintf("offset_%d_%s", home.id, target), strconv.Itoa(cmd.Number))
		l.hub.Send(key, protocol.TypeSetOffset, &protocol.SetOffsetPayload{OffsetMS: int64(cmd.Number)})
		ev.Reply(fmt.Sprintf("offset дома %s = %d мс, действует со следующего старта", target, cmd.Number))

	case bot.KindOffsetTest:
		t := time.Now().UnixMilli() + 2000
		payload := &protocol.OffsetTestPayload{TCoordMS: t, Clicks: 5, IntervalMS: 1000}
		peers := l.sessionPeers(o)
		sent := 0
		for _, p := range peers {
			if l.hub.Send(l.nodeKey(o, p), protocol.TypeOffsetTest, payload) {
				sent++
			}
		}
		if sent < len(peers) || sent == 0 {
			ev.Reply("все ноды орбита должны быть онлайн для клик-теста (/status покажет)")
			return
		}
		ev.Reply("5 синхронных кликов через 2 секунды — слушайте")

	case bot.KindInject:
		if o.sess.Mode != session.ModeSolo {
			ev.Reply("в shared просто кинь ссылку в чат; /inject — для solo")
			return
		}
		targets := l.injectTargets(o, home.id, ev.FromUserID, cmd.Target)
		sent := 0
		for _, t := range targets {
			if l.hub.Send(t, protocol.TypeSoloInject, &protocol.SoloInjectPayload{URI: cmd.URI}) {
				sent++
			}
		}
		if sent == 0 {
			ev.Reply("целевая нода офлайн")
			return
		}
		ev.Reply("подкинул в очередь")

	case bot.KindProvider:
		if !l.cfg.Providers {
			ev.Reply("провайдерский слой ещё не включён")
			return
		}
		provs, err := l.st.SlotProviders(home.id)
		if err != nil {
			l.log.Error("slot providers lookup failed", "orbit", home.id, "err", err)
			ev.Reply("не смог прочитать дома орбита")
			return
		}
		if provs[cmd.Target] == "" {
			ev.Reply(fmt.Sprintf("дома %s нет в орбите (/orbit покажет слоты)", cmd.Target))
			return
		}
		if err := l.st.SetSlotProvider(home.id, cmd.Target, cmd.Provider); err != nil {
			l.log.Error("set slot provider failed", "orbit", home.id, "slot", cmd.Target, "err", err)
			ev.Reply("не получилось сохранить провайдера")
			return
		}
		// Push to the node (spec-providers §6.3): offline nodes learn the
		// provider from the slots table on their next connect.
		if !l.hub.Send(hub.NodeKey{Orbit: home.id, Slot: protocol.NodeID(cmd.Target)},
			protocol.TypeSetProvider, &protocol.SetProviderPayload{Provider: cmd.Provider}) {
			ev.Reply(fmt.Sprintf("дом %s офлайн — провайдер %s сохранён, нода узнает при подключении", cmd.Target, providerName(cmd.Provider)))
			return
		}
		ev.Reply(fmt.Sprintf("дом %s теперь на %s", cmd.Target, providerName(cmd.Provider)))

	case bot.KindResolve:
		if !l.cfg.Providers {
			ev.Reply("провайдерский слой ещё не включён")
			return
		}
		// Reserved (spec-providers §8): manual repair needs ctid queues.
		ev.Reply("ручная починка маппинга приедет вместе с очередями на ctid")

	// --- Approaches (design §12 L1): two barycenters, one shared air ---

	case bot.KindApproach:
		if member.Role != "primary" {
			ev.Reply("сближение предлагает primary барицентра")
			return
		}
		if cmd.Target == "" {
			code, err := l.st.ProposeLink(home.id, ev.FromUserID)
			if errors.Is(err, store.ErrLinkBusy) {
				ev.Reply("вы уже в сближении — сначала /apart")
				return
			}
			if err != nil {
				ev.Reply("не смог создать код сближения")
				return
			}
			ev.Reply(fmt.Sprintf("код сближения — одноразовый, живёт 15 минут:\n\n<code>%s</code>\n\nПередай его primary другого барицентра: он отправит /approach %s, ты подтвердишь — и эфир станет общим на оба дома.", code, code))
			return
		}
		linkID, orbitA, err := l.st.AcceptByCode(cmd.Target, home.id)
		switch {
		case errors.Is(err, store.ErrLinkSelf):
			ev.Reply("это код твоего же барицентра — сближаться с собой не нужно")
			return
		case errors.Is(err, store.ErrLinkBusy):
			ev.Reply("одна из сторон уже в сближении — сначала /apart")
			return
		case err != nil:
			ev.Reply("не получилось принять код")
			return
		case linkID == 0:
			ev.Reply("код не подошёл — истёк или уже использован, попроси новый /approach")
			return
		}
		l.notifyOrbit(orbitA, fmt.Sprintf("барицентр «%s» хочет сближения: общий эфир на все дома.\n/accept — начать · /decline — отказаться", esc(home.title)))
		ev.Reply(fmt.Sprintf("предложение отправлено барицентру «%s» — ждём подтверждения их primary", esc(l.orbit(orbitA).title)))

	case bot.KindAccept:
		if member.Role != "primary" {
			ev.Reply("подтвердить сближение может только primary")
			return
		}
		linkID, other, ok, err := l.st.AwaitingLink(home.id)
		if err != nil || !ok {
			ev.Reply("подтверждать нечего — сближение не предлагали")
			return
		}
		if err := l.st.ActivateLink(linkID); err != nil {
			ev.Reply("не получилось активировать сближение")
			return
		}
		l.startGroup(linkID, home.id, other)

	case bot.KindDecline:
		if member.Role != "primary" {
			ev.Reply("отклонить сближение может только primary")
			return
		}
		linkID, other, ok, err := l.st.AwaitingLink(home.id)
		if err != nil || !ok {
			ev.Reply("отклонять нечего")
			return
		}
		l.st.BreakLink(linkID)
		l.notifyOrbit(other, fmt.Sprintf("барицентр «%s» отклонил сближение", esc(home.title)))
		ev.Reply("отклонил — остаёмся каждый у себя")

	case bot.KindApart:
		if member.Role != "primary" {
			ev.Reply("разорвать сближение может только primary")
			return
		}
		linkID, other, ok, err := l.st.ActiveLink(home.id)
		if err != nil || !ok {
			ev.Reply("активного сближения нет")
			return
		}
		l.breakGroup(linkID, home.id, other)
	}
}

// handleStranger is the zero-context onboarding path (design §4).
func (l *loop) handleStranger(ev bot.Event) {
	if ev.Voice != nil {
		return // strangers' voices are ignored silently
	}
	switch ev.Command.Kind {
	case bot.KindStart:
		payload := ev.Command.Target
		if strings.HasPrefix(payload, "inv") {
			orbitID, _, err := l.st.ConsumeInvite(payload, "member")
			if err != nil || orbitID == 0 {
				ev.Reply("ссылка-приглашение истекла или уже использована — попроси новую (/share у любого участника)")
				return
			}
			if err := l.st.AddMember(orbitID, ev.FromUserID, "companion"); err != nil {
				ev.Reply("не смог добавить в орбит (возможно, он полон)")
				return
			}
			o := l.orbit(orbitID)
			l.notify(o, fmt.Sprintf("%s теперь в орбите", ev.FromName))
			ev.Reply(fmt.Sprintf("добро пожаловать в <b>«%s»</b>! Кидай ссылки на треки прямо сюда.\n\nХочешь, чтобы эфир звучал и у тебя дома — поставь приложение Pulsar и набери /pair, дам код.", esc(o.title)))
			return
		}
		ev.Reply(strangerHello)
	case bot.KindCreate:
		title := strings.TrimSpace(ev.Command.Target)
		if title == "" {
			title = "Барицентр " + ev.FromName
		}
		o, err := l.st.CreateOrbit(title, ev.FromUserID)
		if err != nil {
			ev.Reply("не смог создать орбит")
			return
		}
		code, _ := l.st.NewPairCode(o.ID, ev.FromUserID)
		ev.Reply(fmt.Sprintf("орбит <b>«%s»</b> создан, ты — primary ⭐\n\nКод для твоего Пульсара — живёт 5 минут:\n<code>%s</code>\n\n/share — пригласить партнёра\n/pair — новый код\n/help — всё остальное", esc(o.Title), code))
	default:
		ev.Reply("это приватная система общих эфиров. /start расскажет, /create создаст твою собственную")
	}
}

func (l *loop) botUsername() string {
	if l.bot != nil && l.bot.Username != "" {
		return l.bot.Username
	}
	return "barycenter_bot"
}

// injectTargets: explicit slot (within the sender's own orbit), "both" =
// every home in the air, default = every home except the sender's own.
func (l *loop) injectTargets(o *orbitState, fromOrbit, from int64, target string) []hub.NodeKey {
	switch target {
	case "", "both":
		mine := protocol.NodeID("")
		if target == "" {
			if sl, _ := l.st.SlotOf(fromOrbit, from); sl != "" {
				mine = l.peerFor(o, hub.NodeKey{Orbit: fromOrbit, Slot: protocol.NodeID(sl)})
			}
		}
		var out []hub.NodeKey
		for _, p := range o.sess.Peers {
			if mine != "" && p == mine {
				continue
			}
			out = append(out, l.nodeKey(o, p))
		}
		return out
	default:
		return []hub.NodeKey{{Orbit: fromOrbit, Slot: protocol.NodeID(target)}}
	}
}

// --- Voice flow (spec ch. 10) ---

func (l *loop) handleVoice(o *orbitState, ev bot.Event) {
	v := ev.Voice
	if v.Duration > l.cfg.Media.MaxVoiceS {
		ev.Reply(fmt.Sprintf("голосовое длиннее %d минут, не возьму", l.cfg.Media.MaxVoiceS/60))
		return
	}
	if v.SizeBytes > 20*1024*1024 {
		ev.Reply("файл больше 20 МБ, не возьму")
		return
	}
	mediaID := ulid.NewMediaID(time.Now())
	rec := store.MediaRecord{
		ID:        mediaID,
		TGFileID:  v.TGFileID,
		CreatedAt: time.Now().UnixMilli(),
		ExpiresAt: time.Now().AddDate(0, 0, l.cfg.Media.RetentionDays).UnixMilli(),
		Status:    "processing",
	}
	if err := l.st.InsertMedia(rec); err != nil {
		l.log.Error("media insert failed", "err", err)
		ev.Reply("внутренняя ошибка, голосовое не принято")
		return
	}
	// Personal is the orbit default (design §5): an explicit «лично» caption
	// forces it; «всем» forces broadcast.
	personal := v.Personal || (o.voiceDefault == "personal" && !v.Broadcast)
	if v.Broadcast {
		personal = false
	}
	reply := ev.Reply
	orbitID := o.id
	from := ev.FromUserID
	fromName := ev.FromName
	go func() {
		oga := filepath.Join(l.cfg.MediaDir, mediaID+".oga")
		wav := filepath.Join(l.cfg.MediaDir, mediaID+".wav")
		var res media.Result
		err := l.bot.DownloadVoice(v.TGFileID, oga)
		if err == nil {
			res, err = media.Process(oga, wav, media.Preset(l.cfg.Media.Preset))
		}
		if err == nil {
			os.Remove(oga)
		}
		l.mediaCh <- mediaDone{orbit: orbitID, mediaID: mediaID, from: from, fromName: fromName, personal: personal, result: res, err: err, reply: reply}
	}()
}

func (l *loop) handleMediaDone(d mediaDone) {
	if d.err != nil {
		l.log.Error("voice processing failed", "media", d.mediaID, "err", d.err)
		l.st.UpdateMedia(store.MediaRecord{ID: d.mediaID, Status: "failed"})
		d.reply("не смог обработать голосовое, оставил исходник для разбора")
		return
	}
	o := l.stateFor(d.orbit)
	l.st.UpdateMedia(store.MediaRecord{
		ID: d.mediaID, DurationMS: d.result.DurationMS,
		PathWAV: d.result.WAVPath, LoudnormJSON: d.result.LoudnormJSON, Status: "ready",
	})
	// Personal target: every home in the air except the sender's own.
	// In a two-home orbit that is exactly the partner (design §5).
	target := "both"
	if d.personal {
		mine := protocol.NodeID("")
		if sl, _ := l.st.SlotOf(d.orbit, d.from); sl != "" {
			mine = l.peerFor(o, hub.NodeKey{Orbit: d.orbit, Slot: protocol.NodeID(sl)})
		}
		var others []protocol.NodeID
		for _, p := range o.sess.Peers {
			if mine != "" && p == mine {
				continue
			}
			others = append(others, p)
		}
		if len(others) == 1 {
			target = string(others[0])
		} else if len(others) == 0 {
			d.reply("в орбите пока только твой дом — отправлю всем, когда появятся другие")
			return
		}
		// >1 recipients for a personal voice: M1 keeps it simple — broadcast
		// to others is not expressible per-element yet, ship to all.
	}
	el := session.Element{
		ID:          ulid.NewElementID(time.Now()),
		Kind:        session.KindVoice,
		MediaID:     d.mediaID,
		DurationMS:  d.result.DurationMS,
		RequestedBy: protocol.NodeID(d.fromName),
		Target:      target,
		CreatedAt:   time.Now().UnixMilli(),
	}
	l.st.InsertElement(el)

	if o.sess.Mode == session.ModeShared {
		l.apply(o, o.sess.EnqueueVoice(el))
		if target != "both" {
			d.reply("личная вставка встанет после текущего трека")
		} else {
			d.reply("вставка встанет после текущего трека")
		}
		return
	}
	payload := &protocol.SoloVoicePayload{ElementID: el.ID, FileURL: l.mediaURL(d.mediaID)}
	targets := append([]protocol.NodeID{}, o.sess.Peers...)
	if target != "both" {
		targets = []protocol.NodeID{protocol.NodeID(target)}
	}
	sent := 0
	for _, t := range targets {
		if l.hub.Send(l.nodeKey(o, t), protocol.TypeSoloVoice, payload) {
			sent++
		}
	}
	if sent == 0 {
		d.reply("нода-адресат офлайн, вставка не доставлена")
		return
	}
	d.reply("вставка уйдёт на ближайшей границе трека")
}

func (l *loop) mediaURL(mediaID string) string {
	if l.cfg.PublicURL != "" {
		return fmt.Sprintf("%s/media/%s.wav", strings.TrimRight(l.cfg.PublicURL, "/"), mediaID)
	}
	return fmt.Sprintf("http://%s/media/%s.wav", l.cfg.Listen, mediaID)
}

// --- Effects ---

func (l *loop) apply(o *orbitState, effs []session.Effect) {
	// The effect boundary: composite "orbit:slot" peer ids of group sessions
	// resolve to real wire addresses here (design §12 L1).
	key := func(to protocol.NodeID) hub.NodeKey { return l.nodeKey(o, to) }
	for _, eff := range effs {
		switch e := eff.(type) {
		case session.EffLoad:
			l.hub.Send(key(e.To), protocol.TypeLoad, &protocol.LoadPayload{ElementID: e.ElementID, URI: e.URI, PositionMS: e.PositionMS})
		case session.EffResumeAt:
			l.hub.Send(key(e.To), protocol.TypeResumeAt, &protocol.ResumeAtPayload{ElementID: e.ElementID, TCoordMS: e.TCoordMS})
		case session.EffPause:
			l.hub.Send(key(e.To), protocol.TypePause, &protocol.PausePayload{ElementID: e.ElementID, FadeMS: e.FadeMS})
		case session.EffPlayVoice:
			l.hub.Send(key(e.To), protocol.TypePlayVoice, &protocol.PlayVoicePayload{
				ElementID: e.ElementID,
				FileURL:   l.mediaURL(e.MediaID),
			})
		case session.EffWait:
			l.hub.Send(key(e.To), protocol.TypeWait, &protocol.WaitPayload{ElementID: e.ElementID, DurationMS: e.DurationMS})
		case session.EffStop:
			l.hub.Send(key(e.To), protocol.TypeStop, &protocol.StopPayload{})
		case session.EffSetMode:
			l.hub.Send(key(e.To), protocol.TypeSetMode, &protocol.SetModePayload{Mode: string(e.Mode)})
		case session.EffNotify:
			l.notify(o, e.Text)
		case session.EffArmReadyTimer:
			l.armReadyTimer(o, e.ElementID)
		case session.EffCancelReadyTimer:
			l.cancelReadyTimer(o)
		case session.EffLogDesync:
			o.lastDesyncMS = e.DeltaMS
			l.log.Info("start desync measured", "orbit", o.id, "delta_ms", e.DeltaMS)
			l.st.LogEvent("session", "desync", map[string]int64{"delta_ms": e.DeltaMS, "orbit": o.id})
		case session.EffElementDone:
			l.st.MarkElementDone(e.Element.ID, e.Status, time.Now().UnixMilli())
		case session.EffPersist:
			l.persist(o)
		}
	}
}

func (l *loop) armReadyTimer(o *orbitState, elementID string) {
	l.cancelReadyTimer(o)
	o.timerElement = elementID
	d := time.Duration(l.cfg.Timings.ReadyTimeoutS) * time.Second
	orbitID := o.id
	o.readyTimer = time.AfterFunc(d, func() { l.timeouts <- orbitTimeout{orbit: orbitID, elementID: elementID} })
}

func (l *loop) cancelReadyTimer(o *orbitState) {
	if o.readyTimer != nil {
		o.readyTimer.Stop()
		o.readyTimer = nil
		o.timerElement = ""
	}
}

// --- Status texts (spec 9.1 /now /queue /status /orbit) ---

func (l *loop) newTrackElement(uri string, fromName string) session.Element {
	return session.Element{
		ID:          ulid.NewElementID(time.Now()),
		Kind:        session.KindTrack,
		URI:         uri,
		RequestedBy: protocol.NodeID(fromName),
		Target:      "both",
		CreatedAt:   time.Now().UnixMilli(),
	}
}

func trackLabel(el session.Element) string {
	if el.Title != "" {
		return esc(el.Title)
	}
	return esc(el.URI)
}

// esc escapes user-controlled text for Telegram HTML parse mode.
func esc(s string) string {
	return html.EscapeString(s)
}

func fmtMS(ms int64) string {
	s := ms / 1000
	return fmt.Sprintf("%02d:%02d", s/60, s%60)
}

// peerName renders a session peer for chat texts: bare slots as-is, group
// composites as "a@<orbit title>" (design §12 bot rendering).
func (l *loop) peerName(o *orbitState, id protocol.NodeID) string {
	if !o.group() {
		return string(id)
	}
	orbit, slot, ok := splitComposite(id)
	if !ok {
		return string(id)
	}
	return fmt.Sprintf("%s@%s", slot, esc(l.orbit(orbit).title))
}

// sessionPeers lists the homes of o's air in stable order: DB-sourced slots
// for a plain orbit (as before §12), the composite peer set for a group.
func (l *loop) sessionPeers(o *orbitState) []protocol.NodeID {
	if o.group() {
		return o.sess.Peers
	}
	slots, _ := l.st.ActiveSlots(o.id)
	out := make([]protocol.NodeID, 0, len(slots))
	for _, sl := range slots {
		out = append(out, protocol.NodeID(sl))
	}
	return out
}

// onlineMap: hub liveness keyed by session peer id.
func (l *loop) onlineMap(o *orbitState) map[protocol.NodeID]bool {
	if !o.group() {
		return l.hub.Online(o.id)
	}
	out := map[protocol.NodeID]bool{}
	for _, orbitID := range o.orbits {
		for sl, on := range l.hub.Online(orbitID) {
			out[compositeID(orbitID, sl)] = on
		}
	}
	return out
}

func (l *loop) orbitText(o *orbitState) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<b>орбит «%s»</b>\n", esc(o.title))
	members, _ := l.st.Members(o.id)
	for _, m := range members {
		slot, _ := l.st.SlotOf(o.id, m.TGUserID)
		home := "без дома"
		if slot != "" {
			home = "дом " + slot
		}
		fmt.Fprintf(&b, "· %d — %s (%s)\n", m.TGUserID, m.Role, home)
	}
	slots, _ := l.st.ActiveSlots(o.id)
	online := l.hub.Online(o.id)
	var parts []string
	for _, sl := range slots {
		mark := "офлайн"
		if online[protocol.NodeID(sl)] {
			mark = "в сети"
		}
		parts = append(parts, sl+": "+mark)
	}
	if len(parts) > 0 {
		fmt.Fprintf(&b, "пульсары: %s", strings.Join(parts, ", "))
	} else {
		b.WriteString("пульсаров пока нет — /pair выдаст код")
	}
	// An active approach is part of the orbit's identity (design §12).
	if linkID := l.linkOf[o.id]; linkID != 0 {
		if g := l.group(linkID); g != nil {
			for _, other := range g.orbits {
				if other != o.id {
					fmt.Fprintf(&b, "\nсближение с «%s» — эфир общий (/apart завершит)", esc(l.orbit(other).title))
				}
			}
		}
	}
	return b.String()
}

func (l *loop) queueText(o *orbitState) string {
	var b strings.Builder
	if cur := o.sess.Current; cur != nil {
		fmt.Fprintf(&b, "сейчас: %s\n", l.elementLabel(o, *cur))
	}
	if o.sess.QueueLen() == 0 {
		b.WriteString("очередь вставок пуста")
	} else {
		b.WriteString("очередь:\n")
		for i, el := range o.sess.Queue {
			fmt.Fprintf(&b, "%d. %s <i>(от %s)</i>\n", i+1, l.elementLabel(o, el), esc(string(el.RequestedBy)))
		}
	}
	if p := o.sess.Playlist; p != nil {
		if p.Cursor < len(p.Tracks) {
			fmt.Fprintf(&b, "\nплейлист <b>«%s»</b>, дальше трек %d/%d", esc(p.Title), p.Cursor+1, len(p.Tracks))
		} else {
			fmt.Fprintf(&b, "\nплейлист <b>«%s»</b> доигран", esc(p.Title))
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

func (l *loop) elementLabel(o *orbitState, el session.Element) string {
	if el.Kind == session.KindVoice {
		who := "всем"
		if el.Target != "both" {
			who = "лично в дом " + l.peerName(o, protocol.NodeID(el.Target))
		}
		return fmt.Sprintf("голосовое %s (%s)", fmtMS(el.DurationMS), who)
	}
	return trackLabel(el)
}

func (l *loop) nowText(o *orbitState) string {
	if o.sess.Mode == session.ModeSolo {
		var b strings.Builder
		b.WriteString("апоастрон: каждый слушает своё\n")
		for _, p := range l.sessionPeers(o) {
			st := o.lastSeen[p]
			if st == nil || st.URI == nil {
				fmt.Fprintf(&b, "дом %s: тишина\n", l.peerName(o, p))
				continue
			}
			fmt.Fprintf(&b, "дом %s: %s @ %s\n", l.peerName(o, p), esc(*st.URI), fmtMS(st.PositionMS))
		}
		return strings.TrimRight(b.String(), "\n")
	}
	if cur := o.sess.Current; cur != nil {
		return fmt.Sprintf("сейчас: %s @ %s (%s)", l.elementLabel(o, *cur), fmtMS(l.livePosition(o)), o.sess.State)
	}
	return "тишина — пришли ссылку на трек"
}

func (l *loop) livePosition(o *orbitState) int64 {
	var best int64
	for _, st := range o.lastSeen {
		if st != nil && st.PositionMS > best {
			best = st.PositionMS
		}
	}
	return best
}

func (l *loop) statusText(o *orbitState) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<b>«%s»</b> — режим %s, состояние %s\n", esc(o.title), o.sess.Mode, o.sess.State)
	online := l.onlineMap(o)
	for _, n := range l.sessionPeers(o) {
		name := l.peerName(o, n)
		if !online[n] {
			fmt.Fprintf(&b, "дом %s: офлайн\n", name)
			continue
		}
		st := o.lastSeen[n]
		if st == nil {
			fmt.Fprintf(&b, "дом %s: онлайн, ждём heartbeat\n", name)
			continue
		}
		mark := ""
		if st.Degraded {
			mark = " [degraded]"
		}
		var speakers []string
		for _, sp := range st.Speakers {
			c := "✗"
			if sp.Connected {
				c = "✓"
			}
			speakers = append(speakers, sp.Name+c)
		}
		fmt.Fprintf(&b, "дом %s: онлайн%s, поз %s, громкость %d, rtt %d мс, offset %d мс, колонки: %s\n",
			name, mark, fmtMS(st.PositionMS), st.Volume, st.RTTMS, o.offsets[n], strings.Join(speakers, " "))
	}
	if o.lastDesyncMS > 0 {
		fmt.Fprintf(&b, "рассинхрон последнего старта: %d мс\n", o.lastDesyncMS)
	}
	fmt.Fprintf(&b, "координатор %s", version)
	for n, v := range o.versions {
		fmt.Fprintf(&b, ", нода %s %s", l.peerName(o, n), v)
	}
	return b.String()
}
