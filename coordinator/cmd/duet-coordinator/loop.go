package main

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"relux.works/duet/coordinator/internal/bot"
	"relux.works/duet/coordinator/internal/config"
	"relux.works/duet/coordinator/internal/hub"
	"relux.works/duet/coordinator/internal/media"
	"relux.works/duet/coordinator/internal/moderation"
	"relux.works/duet/coordinator/internal/presentation"
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
	NodeSnapshots() map[hub.NodeKey]hub.NodeSnapshot
}

type telegramMediaAdapter interface {
	Accept(media.TelegramVoice) (media.TelegramAcceptance, error)
	Submit(context.Context, media.TelegramAcceptance) (media.Result, error)
}

// orbitState is everything the loop tracks per orbit (v2.1 multi-tenant):
// one FSM session plus its knobs, timers and last-seen node telemetry.
// Group sessions (design §12 approaches) reuse the same shape with id =
// -linkID and peer ids in composite "orbit:slot" form.
type orbitState struct {
	id    int64
	airID string
	// authorityGeneration fences delayed effects from a prior Air ownership
	// generation. Personal and legacy-link states keep zero here.
	authorityGeneration int64
	airRevision         int64
	title               string
	sess                *session.Session

	// orbits: for group states, the linked orbit ids; nil for plain orbits.
	orbits []int64

	takeoverPolicy string // user | coordinator (per-orbit, orbits table)
	voiceDefault   string // personal | broadcast

	volumes  map[protocol.NodeID]int
	offsets  map[protocol.NodeID]int64
	lastSeen map[protocol.NodeID]*protocol.StatePayload
	versions map[protocol.NodeID]string
	// capabilities is the exact validated set from the current authenticated
	// connection. Registration replaces it wholesale; unknown names remain for
	// diagnostics while feature decisions query exact known flags.
	capabilities map[protocol.NodeID]protocol.CapabilitySet
	seamless     map[protocol.NodeID]bool
	// restoredPaused: after coordinator restart, resume position must follow
	// live heartbeats until the user resumes (spec 7.2).
	restoredPaused bool
	lastDesyncMS   int64

	readyTimer   *time.Timer
	timerElement string
}

// group reports whether this state is a link (approach) session.
func (o *orbitState) group() bool { return o.airID != "" || o.id < 0 }

// loop serializes every session-affecting event across all orbits
// (spec 7.2: the FSM is single-threaded; one goroutine, many orbits).
type loop struct {
	log *slog.Logger
	cfg *config.Config
	hub nodeSender
	st  *store.Store
	bot *bot.Bot        // nil when telegram is disabled (dev mode)
	sp  *spotify.Client // nil when spotify app creds are not configured (U10)

	telegramMedia        telegramMediaAdapter
	telegramMediaInitErr error
	telegramInlinePrompt func(int64, string, bot.InlineKeyboardBuilder)
	mediaLifecycle       *media.LifecycleService
	moderationService    *moderation.Service
	livePTT              *session.LivePTTRuntime
	livePTTGeneration    int64

	// resolveTrack runs the provider cascade for one track (providers.go).
	// nil while the provider layer is off; tests stub it directly.
	resolveTrack resolveTrackFn
	// fetchTrackMetadata gives the legacy Spotify-only path the same human
	// Artist — Track labels without enabling the multi-provider gate.
	fetchTrackMetadata func(string) (trackMetadata, error)

	states map[int64]*orbitState

	// Approaches (design §12 L1): linkOf maps an orbit to its active link id
	// (absent when solo), groups holds the shared per-link sessions.
	linkOf map[int64]int64
	groups map[int64]*orbitState
	// Phase 2 runtime ownership is keyed only by stable public Air ID. airOf is
	// a resolver cache, never authority; every stateFor call revalidates it
	// against the persisted current pointer.
	airOf map[int64]string
	airs  map[string]*orbitState

	timeouts             chan orbitTimeout
	mediaCh              chan mediaDone
	mediaCancel          chan mediaCancellationCall
	playlistCh           chan playlistDone
	resolveCh            chan resolveDone
	trackMetaCh          chan trackMetadataDone
	transmissionCh       chan transmissionSignal
	airControlCh         chan airControlCall
	transmissionTimer    *time.Timer
	transmissionTimerC   <-chan time.Time
	transmissionTimerDue int64
	transmissionNow      func() int64
	// Voice processing is concurrent, but airtime order is the serial Telegram
	// acceptance order per air. Completed jobs wait here for older jobs.
	voiceAccepted    map[int64]int64
	voiceNext        map[int64]int64
	voicePending     map[int64]map[int64]mediaDone
	airVoiceAccepted map[string]int64
	airVoiceNext     map[string]int64
	airVoicePending  map[string]map[int64]mediaDone
	stopped          chan struct{}
}

type orbitTimeout struct {
	orbit      int64
	airID      string
	generation int64
	revision   int64
	elementID  string
}

type runtimeFence struct {
	airID      string
	generation int64
	revision   int64
	legacyID   int64
}

type playlistDone struct {
	orbit  int64
	fence  runtimeFence
	uri    string
	title  string
	tracks []string
	err    error
	reply  func(string)
}

type trackMetadata struct {
	title      string
	artists    []string
	durationMS int64
}

type trackMetadataDone struct {
	orbit   int64
	fence   runtimeFence
	el      session.Element
	playNow bool
	meta    trackMetadata
	err     error
	reply   func(string)
}

type mediaDone struct {
	orbit int64
	// orderAir is the session that owned the air when Telegram accepted the
	// message. Two linked barycenters have different source orbit ids but one
	// shared orderAir, so ffmpeg completion cannot reorder their voices.
	orderAir           int64
	orderAirID         string
	orderAirGeneration int64
	orderAirRevision   int64
	mediaID            string
	from               int64 // tg user id of the sender
	fromName           string
	acceptedAt         int64
	sequence           int64
	personal           bool
	attachmentKind     string
	chatID             int64
	originalUpdateID   int64
	result             media.Result
	err                error
	reply              func(string)
}

type mediaCancellationCall struct {
	request store.MediaDeliveryCancellation
	result  chan error
}

func newLoop(log *slog.Logger, cfg *config.Config, h nodeSender, st *store.Store, b *bot.Bot, sp *spotify.Client) *loop {
	l := &loop{
		log:            log,
		cfg:            cfg,
		hub:            h,
		st:             st,
		bot:            b,
		sp:             sp,
		states:         map[int64]*orbitState{},
		linkOf:         map[int64]int64{},
		groups:         map[int64]*orbitState{},
		airOf:          map[int64]string{},
		airs:           map[string]*orbitState{},
		timeouts:       make(chan orbitTimeout, 8),
		mediaCh:        make(chan mediaDone, 8),
		mediaCancel:    make(chan mediaCancellationCall, 8),
		playlistCh:     make(chan playlistDone, 4),
		resolveCh:      make(chan resolveDone, 8),
		trackMetaCh:    make(chan trackMetadataDone, 8),
		transmissionCh: make(chan transmissionSignal, 256),
		airControlCh:   make(chan airControlCall, 8),
		transmissionNow: func() int64 {
			return time.Now().UnixMilli()
		},
		voiceAccepted:    map[int64]int64{},
		voiceNext:        map[int64]int64{},
		voicePending:     map[int64]map[int64]mediaDone{},
		airVoiceAccepted: map[string]int64{},
		airVoiceNext:     map[string]int64{},
		airVoicePending:  map[string]map[int64]mediaDone{},
		stopped:          make(chan struct{}),
		livePTT:          session.NewLivePTTRuntime(),
	}
	if b != nil {
		l.telegramInlinePrompt = b.SendInlinePrompt
	}
	if sp != nil {
		l.fetchTrackMetadata = func(ref string) (trackMetadata, error) {
			t, err := sp.TrackByRef(ref)
			if err != nil {
				return trackMetadata{}, err
			}
			return trackMetadata{title: t.Title, artists: t.Artists, durationMS: t.DurationMS}, nil
		}
	}
	return l
}

// orbitGone reports a dissolved orbit (L3): async completions (media,
// playlist expansion, provider resolve) can land after /dissolve deleted the
// orbit — stateFor would then rebuild a ghost state and persist() would
// re-create its session snapshot in settings.
func (l *loop) orbitGone(id int64) bool {
	rec, err := l.st.GetOrbit(id)
	return err == nil && rec == nil
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
		capabilities:   map[protocol.NodeID]protocol.CapabilitySet{},
		seamless:       map[protocol.NodeID]bool{},
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
	var snap *store.SessionSnapshot
	var err error
	if o.airID != "" {
		snap, err = l.st.LoadAirSession(o.airID)
	} else {
		snap, err = l.st.LoadSession(o.id)
	}
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
	if runtime, err := l.st.ActiveAirRuntimeForOrbit(orbitID); err == nil && runtime != nil {
		if prior := l.airOf[orbitID]; prior != "" && prior != runtime.AirID {
			l.detachOrbitFromAir(orbitID, prior)
		}
		l.airOf[orbitID] = runtime.AirID
		return l.airState(*runtime)
	} else if err != nil {
		l.log.Error("Air runtime resolution failed", "orbit", orbitID, "err", err)
	}
	if prior := l.airOf[orbitID]; prior != "" {
		l.detachOrbitFromAir(orbitID, prior)
	}
	authority, authorityErr := l.st.AirAuthority()
	if authorityErr != nil {
		l.log.Error("shared runtime authority unavailable", "orbit", orbitID, "err", authorityErr)
		return l.orbit(orbitID)
	}
	if authority.Mode == "airs_authoritative" || authority.Mode == "rollback_hold" {
		return l.orbit(orbitID)
	}
	if linkID := l.linkOf[orbitID]; linkID != 0 {
		if g := l.group(linkID); g != nil {
			return g
		}
		delete(l.linkOf, orbitID) // stale link: fall back to the personal orbit
	}
	return l.orbit(orbitID)
}

func fenceFor(o *orbitState) runtimeFence {
	return runtimeFence{
		airID: o.airID, generation: o.authorityGeneration,
		revision: o.airRevision, legacyID: o.id,
	}
}

func (l *loop) stateForFence(orbitID int64, fence runtimeFence) (*orbitState, bool) {
	o := l.stateFor(orbitID)
	if fence.airID != "" {
		return o, o.airID == fence.airID && o.authorityGeneration == fence.generation &&
			o.airRevision == fence.revision
	}
	return o, o.airID == "" && o.id == fence.legacyID
}

func rejectStaleRuntime(reply func(string)) {
	if reply != nil {
		reply("эфир изменился, пока шла обработка — старый результат отменён")
	}
}

func (l *loop) detachOrbitFromAir(orbitID int64, airID string) {
	if runtime, err := l.st.ActiveAirRuntimeByID(airID); err == nil && runtime != nil {
		if existing := l.airs[airID]; existing != nil {
			l.syncAirState(existing, *runtime)
		}
	} else {
		l.parkAirState(airID)
	}
	if l.airOf[orbitID] == airID {
		delete(l.airOf, orbitID)
	}
}

func (l *loop) airState(runtime store.AirRuntime) *orbitState {
	if existing := l.airs[runtime.AirID]; existing != nil {
		l.syncAirState(existing, runtime)
		return existing
	}
	title := runtime.AirID
	if record, err := l.st.AirByID(runtime.AirID); err == nil {
		title = record.Title
	}
	o := &orbitState{
		airID: runtime.AirID, authorityGeneration: runtime.Generation,
		airRevision: runtime.Revision,
		orbits:      append([]int64(nil), runtime.OrbitIDs...), title: title,
		sess: session.New(), takeoverPolicy: "user", voiceDefault: "personal",
		volumes: map[protocol.NodeID]int{}, offsets: map[protocol.NodeID]int64{},
		lastSeen: map[protocol.NodeID]*protocol.StatePayload{}, versions: map[protocol.NodeID]string{},
		capabilities: map[protocol.NodeID]protocol.CapabilitySet{}, seamless: map[protocol.NodeID]bool{},
	}
	o.sess.StartMarginMS = int64(l.cfg.Timings.StartMarginMS)
	l.syncAirState(o, runtime)
	l.restoreSnapshot(o)
	l.airs[runtime.AirID] = o
	return o
}

func (l *loop) syncAirState(o *orbitState, runtime store.AirRuntime) {
	previousRevision := o.airRevision
	oldPeers := map[protocol.NodeID]bool{}
	for _, peer := range o.sess.Peers {
		oldPeers[peer] = true
	}
	current := map[int64]bool{}
	for _, orbitID := range runtime.OrbitIDs {
		current[orbitID] = true
	}
	for _, oldOrbitID := range o.orbits {
		if current[oldOrbitID] {
			continue
		}
		delete(l.airOf, oldOrbitID)
		for _, peer := range append([]protocol.NodeID(nil), o.sess.Peers...) {
			peerOrbit, _, ok := splitComposite(peer)
			if ok && peerOrbit == oldOrbitID {
				l.hub.Send(l.nodeKey(o, peer), protocol.TypeStop, &protocol.StopPayload{})
				l.apply(o, o.sess.RemovePeer(time.Now().UnixMilli(), peer))
			}
		}
	}
	o.authorityGeneration = runtime.Generation
	o.airRevision = runtime.Revision
	o.orbits = append(o.orbits[:0], runtime.OrbitIDs...)
	peers := make([]string, 0)
	online := map[protocol.NodeID]bool{}
	for _, orbitID := range runtime.OrbitIDs {
		l.airOf[orbitID] = runtime.AirID
		slots, _ := l.st.ActiveSlots(orbitID)
		for _, slot := range slots {
			peer := compositeID(orbitID, protocol.NodeID(slot))
			peers = append(peers, string(peer))
			if _, ok := o.volumes[peer]; !ok {
				o.volumes[peer] = 80
			}
		}
		for slot, isOnline := range l.hub.Online(orbitID) {
			online[compositeID(orbitID, slot)] = isOnline
		}
	}
	o.sess.SetPeers(peers)
	o.sess.GateMode = session.GateEachSide
	o.sess.SideOf = func(peer protocol.NodeID) string {
		orbitID, _, ok := splitComposite(peer)
		if !ok {
			return string(peer)
		}
		return strconv.FormatInt(orbitID, 10)
	}
	var joining []protocol.NodeID
	for _, peerText := range peers {
		peer := protocol.NodeID(peerText)
		if !oldPeers[peer] {
			joining = append(joining, peer)
		}
	}
	o.sess.PrepareLivingAirJoiners(joining)
	for _, peerText := range peers {
		peer := protocol.NodeID(peerText)
		if !oldPeers[peer] && online[peer] {
			// The FSM deliberately catches up only a current main track;
			// voice/overlay work is never replayed for a joining member.
			if effects := o.sess.JoinInProgressAt(time.Now().UnixMilli(), peer); effects != nil {
				l.apply(o, effects)
			}
		}
	}
	o.sess.SeedOnline(online)
	if previousRevision != 0 && previousRevision != runtime.Revision && o.timerElement != "" {
		l.armReadyTimer(o, o.timerElement)
	}
}

func (l *loop) parkAirState(airID string) {
	o := l.airs[airID]
	if o == nil {
		return
	}
	l.cancelReadyTimer(o)
	for _, peer := range o.sess.Peers {
		l.hub.Send(l.nodeKey(o, peer), protocol.TypeStop, &protocol.StopPayload{})
	}
	l.persist(o)
	delete(l.airs, airID)
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
		capabilities:   map[protocol.NodeID]protocol.CapabilitySet{},
		seamless:       map[protocol.NodeID]bool{},
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
			homeNode := protocol.NodeID(sl)
			g.versions[n] = l.orbit(orbitID).versions[homeNode]
			g.capabilities[n] = l.orbit(orbitID).capabilities[homeNode]
			g.seamless[n] = l.orbit(orbitID).seamless[homeNode]
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
	links := []store.Link{}
	runtimes := []store.AirRuntime{}
	authority, authorityErr := l.st.AirAuthority()
	airAuthoritative := authorityErr == nil && authority.Mode == "airs_authoritative"
	legacyAuthoritative := authorityErr == nil &&
		(authority.Mode == "links_authoritative" || authority.Mode == "airs_shadow")
	if airAuthoritative {
		var runtimeErr error
		runtimes, runtimeErr = l.st.ActiveAirRuntimes()
		if runtimeErr != nil {
			l.log.Error("Air runtime warmup failed", "err", runtimeErr)
		}
	} else if legacyAuthoritative {
		var err error
		links, err = l.st.ActiveLinks()
		if err != nil {
			l.log.Error("link warmup failed", "err", err)
		}
		for _, lk := range links {
			l.linkOf[lk.OrbitA] = lk.ID
			l.linkOf[lk.OrbitB] = lk.ID
		}
	} else {
		l.log.Error("shared runtime warmup disabled by authority state", "mode", authority.Mode, "err", authorityErr)
	}
	if airAuthoritative {
		for _, runtime := range runtimes {
			for _, orbitID := range runtime.OrbitIDs {
				l.airOf[orbitID] = runtime.AirID
			}
		}
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
	for _, runtime := range runtimes {
		l.airState(runtime)
	}
	restartResults, err := l.st.ReconcileTransmissionSchedulerRestart(time.Now().UnixMilli())
	if err != nil {
		l.log.Error("transmission restart reconciliation failed", "err", err)
	} else {
		l.deliverTransmissionCancellations(restartResults)
	}
	l.log.Info("orbits warmed up", "count", len(ids), "links", len(links), "active_airs", len(runtimes))
	l.signalTransmission(transmissionSignal{})
}

func (l *loop) run(stop <-chan struct{}, nodeEvents <-chan hub.Event) {
	defer close(l.stopped)
	liveTicker := time.NewTicker(100 * time.Millisecond)
	defer liveTicker.Stop()
	livePolicyTicker := time.NewTicker(time.Second)
	defer livePolicyTicker.Stop()
	var botEvents chan bot.Event
	if l.bot != nil {
		botEvents = l.bot.Events
	}
	for {
		select {
		case <-stop:
			l.livePTT.ResetForRestart()
			return
		case ev := <-nodeEvents:
			l.handleNode(ev)
		case to := <-l.timeouts:
			// A group timer may outlive its link; drop it then.
			var o *orbitState
			if to.airID != "" {
				o = l.airs[to.airID]
				if o != nil && o.authorityGeneration != to.generation {
					o = nil
				}
				if o != nil && o.airRevision != to.revision {
					o = nil
				}
			} else {
				o = l.stateByID(to.orbit)
			}
			if o != nil {
				l.apply(o, o.sess.OnReadyTimeoutAt(time.Now().UnixMilli(), to.elementID))
			}
		case ev := <-botEvents:
			l.handleBot(ev)
		case done := <-l.mediaCh:
			l.handleMediaDone(done)
		case call := <-l.mediaCancel:
			call.result <- l.applyMediaCancellation(call.request)
		case d := <-l.playlistCh:
			l.handlePlaylistDone(d)
		case d := <-l.resolveCh:
			l.onResolveDone(d)
		case d := <-l.trackMetaCh:
			l.handleTrackMetadataDone(d)
		case signal := <-l.transmissionCh:
			l.handleTransmissionSignal(signal)
		case call := <-l.airControlCh:
			call.done <- l.reconcileAirControlRuntime()
		case <-l.transmissionTimerC:
			due := l.transmissionTimerDue
			l.transmissionTimerC = nil
			l.transmissionTimerDue = 0
			now := time.Now().UnixMilli()
			if due > now {
				// The timer itself is monotonic. If wall time moved backwards,
				// persisted coordinator time must not extend the barrier/T guard.
				now = due
			}
			l.runTransmissionScheduler(now)
		case now := <-liveTicker.C:
			l.applyLivePTTEffects(l.livePTT.Sweep(now.UnixMilli()))
		case now := <-livePolicyTicker.C:
			l.auditLivePTTPolicy(now.UnixMilli())
		}
	}
}

// CancelMedia adapts the frozen generic lifecycle outbox to the current
// single-threaded legacy session runtime. The later transmission scheduler can
// replace this sink without changing delete or retention persistence.
func (l *loop) CancelMedia(ctx context.Context, request store.MediaDeliveryCancellation) error {
	if ctx == nil {
		return errors.New("nil media cancellation context")
	}
	if request.MediaID == "" || request.PolicyVersion != store.MediaLifecyclePolicyV1 ||
		request.NotStartedAction != store.MediaNotStartedActionCancel ||
		request.ActiveAction != store.MediaActiveActionFadeStop ||
		request.InterruptedMainAction != store.MediaInterruptedMainActionResumeOnce {
		return errors.New("unsupported media cancellation policy")
	}
	call := mediaCancellationCall{request: request, result: make(chan error, 1)}
	select {
	case l.mediaCancel <- call:
	case <-ctx.Done():
		return ctx.Err()
	case <-l.stopped:
		return errors.New("media cancellation runtime stopped")
	}
	select {
	case err := <-call.result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-l.stopped:
		return errors.New("media cancellation runtime stopped")
	}
}

func (l *loop) applyMediaCancellation(request store.MediaDeliveryCancellation) error {
	var persistErr error
	for _, state := range l.states {
		l.apply(state, state.sess.CancelMedia(request.MediaID))
		if err := l.saveSession(state); err != nil {
			persistErr = errors.Join(persistErr, err)
		}
	}
	for _, state := range l.groups {
		l.apply(state, state.sess.CancelMedia(request.MediaID))
		if err := l.saveSession(state); err != nil {
			persistErr = errors.Join(persistErr, err)
		}
	}
	for _, state := range l.airs {
		l.apply(state, state.sess.CancelMedia(request.MediaID))
		if err := l.saveSession(state); err != nil {
			persistErr = errors.Join(persistErr, err)
		}
	}
	if persistErr != nil {
		return fmt.Errorf("persist media cancellation: %w", persistErr)
	}
	reason := store.TransmissionReasonMediaDeleted
	if request.Reason == store.MediaCancellationExpired {
		reason = store.TransmissionReasonMediaExpired
	}
	results, err := l.st.CancelTransmissionsForMedia(
		request.MediaID, reason, time.Now().UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("cancel media transmissions: %w", err)
	}
	l.deliverTransmissionCancellations(results)
	l.runTransmissionScheduler(time.Now().UnixMilli())
	return nil
}

func (l *loop) enrichSpotifyTrack(
	sourceOrbit int64, o *orbitState, el session.Element, playNow bool, reply func(string),
) {
	_, isSpotifyTrack := strings.CutPrefix(el.URI, "spotify:track:")
	if l.fetchTrackMetadata == nil || !isSpotifyTrack {
		l.finishTrackAction(o, el, playNow, reply)
		return
	}
	fence := fenceFor(o)
	go func() {
		meta, err := l.fetchTrackMetadata(el.URI)
		l.trackMetaCh <- trackMetadataDone{
			orbit: sourceOrbit, fence: fence, el: el, playNow: playNow,
			meta: meta, err: err, reply: reply,
		}
	}()
}

func (l *loop) handleTrackMetadataDone(d trackMetadataDone) {
	if l.orbitGone(d.orbit) {
		return
	}
	o, current := l.stateForFence(d.orbit, d.fence)
	if !current {
		rejectStaleRuntime(d.reply)
		return
	}
	if o.sess.Mode != session.ModeShared {
		d.reply("сейчас режим solo: /inject подкинет трек партнёру, /together вернёт общий эфир")
		return
	}
	if d.err != nil {
		// Metadata is presentation only: a temporary Web API problem must not
		// make an otherwise playable link disappear from the air.
		l.log.Warn("spotify track metadata failed", "uri", d.el.URI, "err", d.err)
	} else {
		stampTrackMetadata(&d.el, d.meta.title, d.meta.artists, d.meta.durationMS)
	}
	l.finishTrackAction(o, d.el, d.playNow, d.reply)
}

func (l *loop) finishTrackAction(o *orbitState, el session.Element, playNow bool, reply func(string)) {
	if !playNow {
		l.finishEnqueue(o, el, reply)
		return
	}
	l.st.InsertElement(el)
	l.apply(o, o.sess.CmdPlayNow(el))
	reply("врубаю немедленно: " + trackLabel(el))
}

func (l *loop) handlePlaylistDone(d playlistDone) {
	if d.err != nil {
		l.log.Warn("playlist expansion failed", "uri", d.uri, "err", d.err)
		d.reply(fmt.Sprintf("не смог раскрыть плейлист: %v", d.err))
		return
	}
	if l.orbitGone(d.orbit) { // L3: /dissolve raced the expansion goroutine
		return
	}
	o, current := l.stateForFence(d.orbit, d.fence)
	if !current {
		rejectStaleRuntime(d.reply)
		return
	}
	l.apply(o, o.sess.SetPlaylist(d.uri, esc(d.title), d.tracks))
}

// notify DMs every member of the session's orbit — both linked orbits for a
// group session (group chat binding comes in M4).
var compositePeerRe = regexp.MustCompile(`\b(\d+):([a-z])\b`)

// humanizePeers rewrites raw composite peer ids ("1:a") from core-produced
// texts into a human orbit name — the FSM stays pure, rendering lives here.
func (l *loop) humanizePeers(text string) string {
	return compositePeerRe.ReplaceAllStringFunc(text, func(m string) string {
		parts := compositePeerRe.FindStringSubmatch(m)
		var orbitID int64
		fmt.Sscanf(parts[1], "%d", &orbitID)
		return l.namedGroupPeer(orbitID, parts[2])
	})
}

func (l *loop) namedGroupPeer(orbitID int64, slot string) string {
	title := ""
	if rec, err := l.st.GetOrbit(orbitID); err == nil && rec != nil {
		title = rec.Title
	}
	slots, _ := l.st.ActiveSlots(orbitID)
	return esc(presentation.TargetLabel(presentation.TargetMetadata{
		OrbitTitle: title, Slot: slot, MultipleSlots: len(slots) > 1,
	}).Text(presentation.Russian))
}

func (l *loop) notify(o *orbitState, text string) {
	// L8: composite "N:x" ids only ever appear in GROUP session texts — do
	// not run the rewrite over personal-orbit texts, where the same pattern
	// inside a track title ("Part 1:a remix") got mangled with a DB lookup.
	if o.group() {
		text = l.humanizePeers(text)
	}
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

	startedText := func(otherTitle string) string {
		text := fmt.Sprintf("сближение с «%s» началось — эфир общий", esc(otherTitle))
		if donor == 0 {
			text += ". Выбери Пульсар в Spotify и включи трек"
		}
		return text
	}
	l.notifyOrbit(orbitA, startedText(l.orbit(orbitB).title))
	l.notifyOrbit(orbitB, startedText(l.orbit(orbitA).title))
	l.log.Info("approach started", "link", linkID, "orbit_a", orbitA, "orbit_b", orbitB, "donor", donor)

	if donor != 0 && (cur != nil || len(queue) > 0 || playlist != nil) {
		l.apply(g, g.sess.Transplant(cur, pos, queue, playlist))
	}
}

// startAirApproach preserves the pairwise broadcast handoff while the
// durable authority is an Air. The store mutation has already committed;
// this method only parks the two personal controllers and reconciles the
// serialized runtime keyed by the stable Air ID.
func (l *loop) startAirApproach(result store.AirApproachAliasResult) error {
	donor, cur, pos, queue, playlist := l.captureDonor(result.OwnerOrbitID, result.CallerOrbitID)
	l.parkOrbitSession(l.orbit(result.OwnerOrbitID))
	l.parkOrbitSession(l.orbit(result.CallerOrbitID))
	if donor != 0 {
		l.emptyParkedSnapshot(donor)
	}
	if err := l.reconcileAirControlRuntime(); err != nil {
		return err
	}
	runtime := l.airs[result.AirID]
	if runtime == nil {
		return errors.New("confirmed Air runtime is unavailable")
	}
	startedText := func(otherTitle string) string {
		text := fmt.Sprintf("сближение с «%s» началось — эфир общий", esc(otherTitle))
		if donor == 0 {
			text += ". Выбери Пульсар в Spotify и включи трек"
		}
		return text
	}
	l.notifyOrbit(result.OwnerOrbitID, startedText(result.CallerTitle))
	l.notifyOrbit(result.CallerOrbitID, startedText(result.OwnerTitle))
	l.log.Info("Air approach started", "air", result.AirID,
		"owner_orbit", result.OwnerOrbitID, "joining_orbit", result.CallerOrbitID, "donor", donor)
	if donor != 0 && (cur != nil || len(queue) > 0 || playlist != nil) {
		l.apply(runtime, runtime.sess.Transplant(cur, pos, queue, playlist))
	}
	return nil
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
	results, err := l.st.CancelTransmissionPlaybackDomain(
		store.PlaybackDomainApproach, linkID,
		store.TransmissionReasonApproachApart, time.Now().UnixMilli(),
	)
	if err != nil {
		l.log.Error("cancel approach transmissions", "link", linkID, "err", err)
	} else {
		l.deliverTransmissionCancellations(results)
	}
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
		// to the group, its liveness map is stale — reseed from the hub. The
		// MEMBERSHIP is stale too (H3): /leave, /revoke, /rebind and fresh
		// pairings were applied to the group session only, so re-read the slot
		// set — or the strict gate waits forever for a revoked home, and a
		// home paired during the link never gets a load.
		o := l.orbit(id)
		slots, _ := l.st.ActiveSlots(id)
		o.sess.SetPeers(slots)
		o.sess.SeedOnline(l.hub.Online(id))
		l.notifyOrbit(id, "сближение завершено — каждый у себя")
	}
	l.log.Info("approach ended", "link", linkID)
}

// dissolveOrbit tears the barycenter down (design §2 delete-orbit): any active
// approach breaks, every home is stopped, and both the session snapshot and the
// orbit's rows are wiped. In-memory state is dropped so future commands from
// former members fall through to the stranger path.
func (l *loop) dissolveOrbit(home *orbitState) {
	if linkID := l.linkOf[home.id]; linkID != 0 {
		// Derive the partner orbit from IN-MEMORY link state, not the store: on
		// the /leave-last-member path LeaveOrbit->DeleteOrbit has already erased
		// the links row, and an ActiveLink lookup here came back empty — so
		// breakGroup never ran and the partner orbit was stranded behind a
		// phantom group session until a coordinator restart (C1).
		others := []int64{home.id}
		if g, ok := l.groups[linkID]; ok {
			for _, id := range g.orbits {
				if id != home.id {
					others = append(others, id)
				}
			}
		} else if _, other, ok, _ := l.st.ActiveLink(home.id); ok {
			others = append(others, other)
		}
		l.breakGroup(linkID, others...)
	}
	results, err := l.st.CancelTransmissionPlaybackDomain(
		store.PlaybackDomainOrbit, home.id,
		store.TransmissionReasonApproachLeft, time.Now().UnixMilli(),
	)
	if err != nil {
		l.log.Error("cancel dissolved-orbit transmissions", "orbit", home.id, "err", err)
	} else {
		l.deliverTransmissionCancellations(results)
	}
	o := l.stateFor(home.id) // the personal orbit again after any breakGroup
	l.cancelReadyTimer(o)
	for _, p := range l.sessionPeers(o) {
		l.hub.Send(l.nodeKey(o, p), protocol.TypeStop, &protocol.StopPayload{})
	}
	l.st.ClearSession(home.id)
	if err := l.st.DeleteOrbit(home.id); err != nil {
		l.log.Error("delete orbit failed", "orbit", home.id, "err", err)
	}
	delete(l.states, home.id)
	l.log.Info("orbit dissolved", "orbit", home.id)
}

func (l *loop) persist(o *orbitState) {
	if err := l.saveSession(o); err != nil {
		l.log.Error("persist failed", "orbit", o.id, "err", err)
	}
}

func (l *loop) saveSession(o *orbitState) error {
	snapshot := store.SessionSnapshot{
		Mode:            o.sess.Mode,
		State:           o.sess.State,
		Current:         o.sess.Current,
		SavedPositionMS: o.sess.SavedPositionMS,
		Queue:           o.sess.Queue,
		Playlist:        o.sess.Playlist,
	}
	if o.airID != "" {
		return l.st.SaveAirSession(o.airID, snapshot)
	}
	return l.st.SaveSession(o.id, snapshot)
}

// --- Node events ---

func (l *loop) handleNode(ev hub.Event) {
	switch e := ev.(type) {
	case hub.EvRegistered:
		// A last-write-wins socket is a new generation and cannot resume live
		// media from the predecessor connection.
		l.applyLivePTTEffects(l.livePTT.Disconnect(livePTTNode(e.Key), time.Now().UnixMilli()))
		o := l.stateFor(e.Key.Orbit)
		n := l.peerFor(o, e.Key)
		o.sess.EnsurePeer(n) // a slot paired after orbit/group warm-up
		l.log.Info("node registered", "orbit", e.Key.Orbit, "slot", e.Key.Slot, "app", e.AppVersion, "librespot", e.LibrespotVersion)
		o.versions[n] = e.AppVersion + "/librespot " + e.LibrespotVersion
		o.capabilities[n] = e.Capabilities
		supportsSeamless := e.Capabilities.Supports(protocol.CapabilitySeamlessAdoption)
		o.seamless[n] = supportsSeamless
		// Keep the personal state warm too while its air is owned by a group;
		// a later /apart -> /approach must not forget this capability until the
		// node reconnects.
		if o.group() {
			home := l.orbit(e.Key.Orbit)
			home.versions[e.Key.Slot] = o.versions[n]
			home.capabilities[e.Key.Slot] = e.Capabilities
			home.seamless[e.Key.Slot] = supportsSeamless
		}
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
		l.reconcileTransmissionNode(e.Key, time.Now().UnixMilli())
		l.signalTransmission(transmissionSignal{})
	case hub.EvOnline:
		o := l.stateFor(e.Key.Orbit)
		peer := l.peerFor(o, e.Key)
		// Living air (§12 L1.5): a home returning to a playing group catches
		// up individually instead of the strict pause/resume dance.
		if join := o.sess.JoinInProgressAt(time.Now().UnixMilli(), peer); join != nil {
			l.apply(o, join)
		} else {
			l.apply(o, o.sess.OnNodeBack(peer))
		}
		l.signalTransmission(transmissionSignal{})
	case hub.EvOffline:
		l.applyLivePTTEffects(l.livePTT.Disconnect(livePTTNode(e.Key), time.Now().UnixMilli()))
		o := l.stateFor(e.Key.Orbit)
		l.log.Warn("node offline", "orbit", e.Key.Orbit, "slot", e.Key.Slot)
		l.st.LogEvent(string(e.Key.Slot), "offline", nil)
		l.apply(o, o.sess.OnNodeOffline(time.Now().UnixMilli(), l.peerFor(o, e.Key)))
		l.signalTransmission(transmissionSignal{})
	case hub.EvMessage:
		l.handleNodeMessage(e)
	case hub.EvBinary:
		l.handleLivePTTBinary(e)
	}
}

func (l *loop) handleNodeMessage(m hub.EvMessage) {
	o := l.stateFor(m.Key.Orbit)
	slot := l.peerFor(o, m.Key)
	now := time.Now().UnixMilli()
	// Proof-of-life guard (M1): the hub's sweep can emit a stale EvOffline just
	// after the reader emitted EvOnline for the same node (flip-then-emit races
	// on both sides). The hub map then says "online", so no further EvOnline
	// will ever come — while the FSM believes the home is dark and a strict
	// orbit stays parked although the node heartbeats normally. Any message
	// from a known peer is proof of life: replay the online edge first.
	if !o.sess.IsOnline(slot) && o.sess.HasPeer(slot) {
		if join := o.sess.JoinInProgressAt(now, slot); join != nil {
			l.apply(o, join)
		} else {
			l.apply(o, o.sess.OnNodeBack(slot))
		}
	}
	switch p := m.Payload.(type) {
	case *protocol.StatePayload:
		o.lastSeen[slot] = p
		o.volumes[slot] = p.Volume
		l.apply(o, o.sess.OnHeartbeatAt(now, slot, p.PositionMS, p.RTTMS))
		if o.restoredPaused {
			o.sess.RefreshSavedPosition()
		}
		l.signalTransmission(transmissionSignal{})
	case *protocol.ReadyPayload:
		l.apply(o, o.sess.OnReady(now, slot, p.ElementID))
	case *protocol.StartedPayload:
		l.apply(o, o.sess.OnStarted(slot, p.ElementID, p.TFirstSampleCoordMS))
	case *protocol.EndedPayload:
		l.apply(o, o.sess.OnEnded(slot, p.ElementID, p.Reason))
	case *protocol.VoiceStartedPayload:
		l.log.Info("voice started", "orbit", o.id, "slot", slot, "element", p.ElementID)
		l.handleLegacyTransmissionStarted(m.Key, m.CredentialTokenHash, p.ElementID)
	case *protocol.VoiceEndedPayload:
		l.handleLegacyTransmissionEnded(m.Key, m.CredentialTokenHash, p.ElementID)
		l.apply(o, o.sess.OnVoiceEnded(slot, p.ElementID))
	case *protocol.WaitEndedPayload:
		l.apply(o, o.sess.OnWaitEnded(slot, p.ElementID))
	case *protocol.ErrorPayload:
		l.log.Warn("node error", "orbit", o.id, "slot", slot, "code", p.Code, "msg", p.Message, "element", p.ElementID)
		l.st.LogEvent(string(slot), "node_error", p)
		l.apply(o, o.sess.OnNodeErrorAt(now, slot, p.Code, p.ElementID))
	case *protocol.UserPausePayload:
		// Personal pause (2026-07-10): detach just this home from the air.
		l.apply(o, o.sess.OnUserPause(now, slot))
	case *protocol.UserResumePayload:
		l.apply(o, o.sess.OnUserResume(slot))
	case *protocol.MediaReadyPayload:
		l.handleMediaReady(m.Key, m.CredentialTokenHash, p)
	case *protocol.MediaStartedPayload:
		l.handleMediaStarted(m.Key, m.CredentialTokenHash, p)
	case *protocol.MediaEndedPayload:
		l.handleMediaEnded(m.Key, m.CredentialTokenHash, p)
	case *protocol.MediaFailedPayload:
		l.handleMediaFailed(m.Key, m.CredentialTokenHash, p)
	case *protocol.MediaCancelledPayload:
		l.handleMediaCancelled(m.Key, m.CredentialTokenHash, p)
	case *protocol.SetDNDPayload:
		l.handleNodeDND(m.Key, m.CredentialTokenHash, p)
		l.applyLivePTTEffects(l.livePTT.Disconnect(livePTTNode(m.Key), now))
	case *protocol.LivePTTStartPayload:
		l.handleLivePTTStart(m.Key, m.CredentialTokenHash, *p, now)
	case *protocol.LivePTTAcceptPayload:
		l.auditLivePTTPolicy(now)
		effects, _ := l.livePTT.Accept(livePTTNode(m.Key), *p)
		l.applyLivePTTEffects(effects)
	case *protocol.LivePTTRejectPayload:
		effects, _ := l.livePTT.Reject(livePTTNode(m.Key), *p)
		l.applyLivePTTEffects(effects)
	case *protocol.LivePTTEndPayload:
		effects, _ := l.livePTT.End(livePTTNode(m.Key), *p)
		l.applyLivePTTEffects(effects)
	case *protocol.LivePTTCancelPayload:
		effects, _ := l.livePTT.Cancel(livePTTNode(m.Key), *p)
		l.applyLivePTTEffects(effects)
	case *protocol.LivePTTFailedPayload:
		effects, _ := l.livePTT.Failed(livePTTNode(m.Key), *p)
		l.applyLivePTTEffects(effects)
	case *protocol.LivePTTReceiptPayload:
		effects, _ := l.livePTT.Receipt(livePTTNode(m.Key), *p)
		l.applyLivePTTEffects(effects)
	case *protocol.ExternalPlaybackPayload:
		positionMS := int64(0)
		if p.PositionMS != nil {
			positionMS = *p.PositionMS
		}
		l.handleExternalPlayback(o, m.Key, p.URI, positionMS, p.Title)
	default:
		l.log.Debug("unhandled message", "slot", slot, "type", m.Env.Type)
	}
}

// --- Bot events: onboarding, roles, commands (spec ch. 9 + v2.1 M1) ---

const strangerHello = `Привет! Я <b>Барицентр</b> — общий музыкальный эфир на несколько домов: включи трек на любом Пульсаре, и он синхронно заиграет у всех; здесь остаются подключение, очередь и голосовые между песнями.

<b>Как начать</b>
/create — создать свой барицентр
…или открой инвайт-ссылку от того, кто уже в системе.`

var errTelegramActorLifecycleDenied = errors.New("telegram actor lifecycle denies onboarding")

func (l *loop) handleBot(ev bot.Event) {
	if ev.Command.Kind == bot.KindTelegramLink {
		l.handleTelegramLink(ev)
		return
	}
	member, err := l.telegramCommandMember(ev.FromUserID)
	if errors.Is(err, errTelegramActorLifecycleDenied) {
		if ev.Callback != nil && ev.Callback.Answer != nil {
			ev.Callback.Answer(bot.CallbackForbidden)
		}
		return
	}
	if err != nil {
		l.log.Error("membership lookup failed", "err", err)
		if ev.Callback != nil && ev.Callback.Answer != nil {
			ev.Callback.Answer(bot.CallbackFailed)
		}
		return
	}
	if member == nil {
		l.handleStranger(ev)
		return
	}
	// Keep the member's display name fresh so /home and /make_primary can use
	// names instead of raw tg ids (bot-ux #4/#5). Cheap single-row update.
	l.st.SetMemberName(member.OrbitID, ev.FromUserID, ev.FromName)

	// home is the member's personal barycenter (admin & settings); o is the
	// air — the shared group session while an approach is active (§12 L1).
	home := l.orbit(member.OrbitID)
	o := l.stateFor(member.OrbitID)
	if ev.Callback != nil {
		l.handleTelegramInlineCallback(ev)
		return
	}

	if ev.Voice != nil || ev.Attachment != nil {
		if member.Role == "satellite" && false { // satellites may voice: allowed by design
			return
		}
		if ev.Voice != nil {
			l.handleVoice(home, ev)
		} else {
			l.handleTelegramAttachment(home, ev)
		}
		return
	}

	cmd := ev.Command
	authority, authorityErr := l.st.AirAuthority()
	airAliases := authorityErr == nil && authority.Mode == "airs_authoritative"
	var policyOperation store.AirPolicyOperation
	switch cmd.Kind {
	case bot.KindLink:
		policyOperation = store.AirPolicyQueue
	case bot.KindPlayNow, bot.KindPlaylist:
		policyOperation = store.AirPolicyReplace
	}
	if policyOperation != "" && l.cfg.SelfServiceOnboarding {
		_, err := l.st.AuthorizeAirActionForIdentity(store.Identity{
			Kind: store.IdentityTelegram, TelegramUserID: ev.FromUserID,
		}, policyOperation)
		if errors.Is(err, store.ErrAirPolicyDenied) {
			ev.Reply("политика текущего Air запрещает эту команду")
			return
		}
		if err != nil {
			l.log.Error("authorize Telegram Air action", "operation", policyOperation, "err", err)
			ev.Reply("не удалось проверить политику текущего Air")
			return
		}
	}

	// Role gate: satellites contribute (tracks, voices, info) but do not
	// steer the air (design §2).
	if member.Role == "satellite" {
		switch cmd.Kind {
		case bot.KindLink, bot.KindQueue, bot.KindNow, bot.KindStatus,
			bot.KindHistory, bot.KindAir, bot.KindContentPolicy, bot.KindAcceptContentPolicy,
			bot.KindStart, bot.KindShare, bot.KindOrbit, bot.KindPairCode, bot.KindLeave:
		default:
			ev.Reply("это управление эфиром — оно у companion'ов. Твоё оружие: треки и голосовые")
			return
		}
	}

	switch cmd.Kind {

	case bot.KindStart:
		ev.Reply(fmt.Sprintf("ты уже в барицентре <b>«%s»</b>.\n/help — команды · /home — кто на связи", esc(home.title)))

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

	case bot.KindRebind:
		// F3: re-pair this member's home. The old slot token dies now (so a
		// lost/leaked one can't linger); the new code re-pairs in the app via
		// "Подключить заново…". A brief tokenless gap is fine — you're re-pairing.
		if old, _ := l.st.SlotOf(home.id, ev.FromUserID); old != "" {
			identity, _ := l.st.CurrentInstallationTarget(home.id, old)
			_, _ = l.st.RevokeSlot(home.id, old) // slot came from SlotOf — always live
			l.cancelTransmissionInstallation(
				identity, store.TransmissionReasonTargetRevoked, time.Now().UnixMilli(),
			)
			o := l.stateFor(home.id)
			peer := protocol.NodeID(old)
			if o.group() {
				peer = compositeID(home.id, protocol.NodeID(old))
			}
			l.apply(o, o.sess.RemovePeer(time.Now().UnixMilli(), peer))
		}
		code, err := l.st.NewPairCode(home.id, ev.FromUserID)
		if err != nil {
			ev.Reply("не смог создать код")
			return
		}
		ev.Reply(fmt.Sprintf("старый доступ отозван. Новый код — живёт 5 минут:\n\n<code>%s</code>\n\nВ приложении Pulsar: меню → «Подключить заново…», введи код.", code))

	case bot.KindOrbit:
		ev.Reply(l.orbitText(home))

	case bot.KindMakePrimary:
		if member.Role != "primary" {
			ev.Reply("передать главную звезду может только primary")
			return
		}
		target := strings.TrimSpace(cmd.Target)
		if target == "" {
			ev.Reply(l.orbitText(home) + "\n\n/make_primary &lt;имя из /home&gt; передаст титул")
			return
		}
		newPrimary := l.resolveMember(home.id, target)
		if newPrimary == 0 {
			ev.Reply("не нашёл такого участника (/home покажет, кто в барицентре)")
			return
		}
		if err := l.st.TransferPrimary(home.id, newPrimary); err != nil {
			ev.Reply("не получилось передать титул")
			return
		}
		l.notify(home, "главная звезда барицентра теперь у "+esc(target))

	case bot.KindRevoke:
		if member.Role != "primary" {
			ev.Reply("отзывать дома может только primary")
			return
		}
		identity, _ := l.st.CurrentInstallationTarget(home.id, cmd.Target)
		found, err := l.st.RevokeSlot(home.id, cmd.Target)
		if err != nil {
			ev.Reply("не получилось")
			return
		}
		if !found {
			// L11: an UPDATE matching zero rows used to read as success.
			ev.Reply(fmt.Sprintf("дома %s нет в орбите (/orbit покажет слоты)", cmd.Target))
			return
		}
		l.cancelTransmissionInstallation(
			identity, store.TransmissionReasonTargetRevoked, time.Now().UnixMilli(),
		)
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
			ev.Reply("сейчас режим solo: /inject подкинет трек партнёру, /together вернёт общий эфир")
			return
		}
		el := l.newTrackElement(cmd.URI, ev.FromName)
		if l.cfg.Providers {
			// The provider cascade may do external HTTP: resolveAndEnqueue runs
			// it off the loop and either enqueues or rejects (P1) from the
			// resolveCh handler (bugs #4). Behaviour is unchanged with the flag
			// off — this branch is simply never taken.
			l.resolveAndEnqueue(member.OrbitID, o, el, ev.Reply)
			return
		}
		l.enrichSpotifyTrack(member.OrbitID, o, el, false, ev.Reply)

	case bot.KindPlayNow:
		if o.sess.Mode != session.ModeShared {
			ev.Reply("/playnow работает в shared. Сейчас solo")
			return
		}
		el := l.newTrackElement(cmd.URI, ev.FromName)
		l.enrichSpotifyTrack(member.OrbitID, o, el, true, ev.Reply)

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
		orbitID := member.OrbitID // re-resolved, then checked against the dispatch-time owner
		fence := fenceFor(o)
		go func() {
			var exp *spotify.Expansion
			var err error
			if kind == "album" {
				exp, err = l.sp.ExpandAlbum(id)
			} else {
				exp, err = l.sp.ExpandPlaylist(id)
			}
			d := playlistDone{orbit: orbitID, fence: fence, uri: uri, err: err, reply: reply}
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
			ev.Reply("политика: телефон главнее — выбранный на Пульсаре трек становится общим эфиром")
		} else {
			ev.Reply("политика: эфир главнее — вмешательство с телефона откатывается (с уведомлением)")
		}

	case bot.KindQueue:
		ev.Reply(l.queueText(o))

	case bot.KindHistory:
		l.sendTelegramHistory(ev)

	case bot.KindAir:
		l.handleTelegramAirCommand(ev, cmd.Target)

	case bot.KindSoundboard:
		l.sendTelegramSoundboard(ev)

	case bot.KindAutomation:
		l.sendTelegramAutomation(ev)

	case bot.KindContentPolicy:
		l.handleTelegramContentPolicy(ev, false)

	case bot.KindAcceptContentPolicy:
		l.handleTelegramContentPolicy(ev, true)

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
		} else if !home.sess.HasPeer(target) {
			// M8: any slot letter parses; the orbit decides which exist.
			ev.Reply(fmt.Sprintf("дома %s нет в орбите (/orbit покажет слоты)", target))
			return
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
		if !home.sess.HasPeer(target) {
			// M8: any slot letter parses; the orbit decides which exist.
			ev.Reply(fmt.Sprintf("дома %s нет в орбите (/orbit покажет слоты)", target))
			return
		}
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
		// M8: an explicit slot must exist in the sender's orbit — otherwise the
		// send just fails and the reply blamed an "offline" node that isn't one.
		if cmd.Target != "" && cmd.Target != "both" && !home.sess.HasPeer(protocol.NodeID(cmd.Target)) {
			ev.Reply(fmt.Sprintf("дома %s нет в орбите (/orbit покажет слоты)", cmd.Target))
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

	case bot.KindLeave:
		// Capture the leaver's home before the store forgets them, so its node
		// leaves the live air.
		slot, _ := l.st.SlotOf(home.id, ev.FromUserID)
		var leavingIdentity *store.MediaTargetIdentity
		if slot != "" {
			leavingIdentity, _ = l.st.CurrentInstallationTarget(home.id, slot)
		}
		dissolved, promoted, err := l.st.LeaveOrbit(home.id, ev.FromUserID)
		if err != nil {
			ev.Reply("не смог выйти из барицентра")
			return
		}
		if slot != "" {
			key := hub.NodeKey{Orbit: home.id, Slot: protocol.NodeID(slot)}
			l.apply(o, o.sess.RemovePeer(time.Now().UnixMilli(), l.peerFor(o, key)))
			l.hub.Send(key, protocol.TypeStop, &protocol.StopPayload{})
		}
		if dissolved {
			l.dissolveOrbit(home)
			ev.Reply("ты вышел — в барицентре больше никого, он распущен")
			return
		}
		l.cancelTransmissionInstallation(
			leavingIdentity, store.TransmissionReasonApproachLeft,
			time.Now().UnixMilli(),
		)
		l.notify(home, fmt.Sprintf("%s покинул барицентр", esc(ev.FromName)))
		if promoted != 0 {
			l.notify(home, "главная звезда перешла следующему участнику")
		}
		ev.Reply("ты вышел из барицентра")

	case bot.KindDissolve:
		if member.Role != "primary" {
			ev.Reply("распустить барицентр может только главная звезда (primary)")
			return
		}
		l.notify(home, fmt.Sprintf("барицентр «%s» распущен — все дома отвязаны", esc(home.title)))
		l.dissolveOrbit(home)
		ev.Reply("барицентр распущен")

	// --- Approaches (design §12 L1): two barycenters, one shared air ---

	case bot.KindApproach:
		if member.Role != "primary" {
			ev.Reply("сближение предлагает primary барицентра")
			return
		}
		if airAliases {
			if cmd.Target == "" {
				result, err := l.st.CreateAirApproachAlias(ev.FromUserID, time.Now().UnixMilli())
				switch {
				case errors.Is(err, store.ErrAirApproachBusy):
					ev.Reply("вы уже в сближении — сначала /apart")
				case errors.Is(err, store.ErrAirForbidden):
					ev.Reply("сближение предлагает primary барицентра")
				case err != nil:
					l.log.Error("create Air approach alias failed", "orbit", home.id, "err", err)
					ev.Reply("не смог создать код сближения")
				default:
					ev.Reply(fmt.Sprintf("код сближения — одноразовый, живёт 15 минут:\n\n<code>%s</code>\n\nПередай его primary другого барицентра: он отправит /approach %s, затем сам подтвердит /accept — и эфир станет общим на оба дома.", result.Code, result.Code))
				}
				return
			}
			result, err := l.st.ConsumeAirApproachAlias(ev.FromUserID, cmd.Target, time.Now().UnixMilli())
			switch {
			case errors.Is(err, store.ErrAirInvalid):
				ev.Reply("это код твоего же барицентра — сближаться с собой не нужно")
			case errors.Is(err, store.ErrAirInviteUnavailable):
				ev.Reply("код не подошёл — истёк или уже использован, попроси новый /approach")
			case errors.Is(err, store.ErrAirAlreadyMember), errors.Is(err, store.ErrAirApproachBusy):
				ev.Reply("одна из сторон уже в сближении — сначала /apart")
			case err != nil:
				l.log.Error("consume Air approach alias failed", "orbit", home.id, "err", err)
				ev.Reply("не получилось принять код")
			default:
				if !result.Replayed {
					l.notifyOrbit(result.OwnerOrbitID, fmt.Sprintf("барицентр «%s» хочет сближения: общий эфир на все дома.\nИх primary подтвердит /accept или откажется /decline", esc(home.title)))
				}
				ev.Reply(fmt.Sprintf("предложение барицентра «%s» принято — подтверди /accept или откажись /decline", esc(result.OwnerTitle)))
			}
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
		if airAliases {
			result, err := l.st.ConfirmAirApproachAlias(ev.FromUserID, time.Now().UnixMilli())
			switch {
			case errors.Is(err, store.ErrAirApproachSwitchConfirmation):
				ev.Reply("сначала заверши текущее сближение через /apart — переключение без подтверждения запрещено")
			case errors.Is(err, store.ErrAirApproachAmbiguous):
				ev.Reply("есть несколько одинаково новых предложений — выбери нужное в приложении")
			case errors.Is(err, store.ErrAirApproachNothingPending):
				ev.Reply("подтверждать нечего — сближение не предлагали")
			case err != nil:
				l.log.Error("confirm Air approach alias failed", "orbit", home.id, "err", err)
				ev.Reply("не получилось активировать сближение")
			case result.Replayed:
				ev.Reply("сближение уже активно")
			default:
				if err := l.startAirApproach(result); err != nil {
					l.log.Error("reconcile confirmed Air approach", "air", result.AirID, "err", err)
					ev.Reply("сближение принято; общий эфир восстановится автоматически")
				}
			}
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
		if airAliases {
			result, err := l.st.DeclineAirApproachAlias(ev.FromUserID, time.Now().UnixMilli())
			switch {
			case errors.Is(err, store.ErrAirApproachAmbiguous):
				ev.Reply("есть несколько одинаково новых предложений — выбери нужное в приложении")
			case errors.Is(err, store.ErrAirApproachNothingPending):
				ev.Reply("отклонять нечего")
			case err != nil:
				l.log.Error("decline Air approach alias failed", "orbit", home.id, "err", err)
				ev.Reply("не получилось отклонить сближение")
			case result.Outcome == "declined":
				l.notifyOrbit(result.OwnerOrbitID, fmt.Sprintf("барицентр «%s» отклонил сближение", esc(home.title)))
				ev.Reply("отклонил — остаёмся каждый у себя")
			case result.Outcome == "cancelled":
				l.notifyOrbit(result.CallerOrbitID, fmt.Sprintf("барицентр «%s» отозвал предложение сближения", esc(home.title)))
				ev.Reply("отозвал предложение — остаёмся каждый у себя")
			default:
				ev.Reply("отозвал предложение — остаёмся каждый у себя")
			}
			return
		}
		// Either side may kill an awaiting claim (M4): the claimant used to be
		// link-locked until the initiator answered — /approach said "сначала
		// /apart" while /apart needs an ACTIVE link.
		linkID, other, initiator, ok, err := l.st.AwaitingLinkAnySide(home.id)
		if err != nil || !ok {
			ev.Reply("отклонять нечего")
			return
		}
		l.st.BreakLink(linkID)
		if initiator {
			l.notifyOrbit(other, fmt.Sprintf("барицентр «%s» отклонил сближение", esc(home.title)))
			ev.Reply("отклонил — остаёмся каждый у себя")
		} else {
			l.notifyOrbit(other, fmt.Sprintf("барицентр «%s» отозвал предложение сближения", esc(home.title)))
			ev.Reply("отозвал предложение — остаёмся каждый у себя")
		}

	case bot.KindApart:
		if member.Role != "primary" {
			ev.Reply("разорвать сближение может только primary")
			return
		}
		if airAliases {
			result, err := l.st.LeaveCurrentAirAlias(ev.FromUserID, time.Now().UnixMilli())
			if errors.Is(err, store.ErrAirApproachNothingPending) {
				ev.Reply("активного сближения нет")
				return
			}
			if err != nil {
				l.log.Error("leave Air approach alias failed", "orbit", home.id, "err", err)
				ev.Reply("не получилось завершить сближение")
				return
			}
			if err := l.reconcileAirControlRuntime(); err != nil {
				l.log.Error("reconcile Air apart", "air", result.AirID, "err", err)
			}
			l.notifyOrbit(result.CallerOrbitID, "сближение завершено — каждый у себя")
			if result.OtherOrbitID != 0 {
				l.notifyOrbit(result.OtherOrbitID, "сближение завершено — каждый у себя")
			}
			l.log.Info("Air approach caller left", "air", result.AirID, "orbit", result.CallerOrbitID)
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

func (l *loop) telegramCommandMember(telegramUserID int64) (*store.Member, error) {
	if !l.cfg.SelfServiceOnboarding {
		return l.st.MemberOf(telegramUserID)
	}
	ctx, err := l.st.ResolveTelegramActorContext(telegramUserID)
	if errors.Is(err, store.ErrUnauthorized) {
		if ctx.ActorID == 0 {
			return nil, nil // genuinely unknown: eligible for onboarding
		}
		return nil, errTelegramActorLifecycleDenied // known and revoked
	}
	if errors.Is(err, store.ErrInsufficientCapability) {
		if ctx.ActorID != 0 && ctx.OrbitID == 0 {
			return nil, nil // deliberately left: eligible for re-onboarding
		}
		return nil, errTelegramActorLifecycleDenied // disabled or otherwise ineligible
	}
	if err != nil {
		return nil, err
	}
	return &store.Member{
		OrbitID:  ctx.OrbitID,
		TGUserID: telegramUserID,
		Role:     ctx.Role,
	}, nil
}

func (l *loop) handleTelegramLink(ev bot.Event) {
	if !l.cfg.SelfServiceOnboarding {
		return // exact feature-off behavior: a bare code is ordinary chatter
	}
	if ev.ChatType != "private" {
		ev.Reply("The provided credential is not valid.")
		return
	}
	result, err := l.st.ConsumeTelegramLink(
		ev.FromUserID,
		ev.FromName,
		ev.ChatType,
		ev.Command.Target,
	)
	switch {
	case err == nil:
		if ev.DeleteSource != nil {
			ev.DeleteSource()
		}
		home := l.orbit(result.OrbitID)
		ev.Reply(fmt.Sprintf("Telegram account linked to <b>«%s»</b> as %s.", esc(home.title), result.Role))
	case errors.Is(err, store.ErrTelegramAlreadyLinkedSameOrbit):
		ev.Reply("This Telegram account is already linked to this orbit.")
	case errors.Is(err, store.ErrTelegramMemberOfOtherOrbit):
		ev.Reply("This Telegram account belongs to a different orbit.")
	case errors.Is(err, store.ErrTelegramLinkRateLimited):
		ev.Reply("Too many attempts. Please wait before retrying.")
	case errors.Is(err, store.ErrTelegramLinkInvalid):
		ev.Reply("The provided credential is not valid.")
	default:
		l.log.Error("telegram link consume failed", "err", err)
		ev.Reply("The provided credential is not valid.")
	}
}

// handleStranger is the zero-context onboarding path (design §4).
func (l *loop) handleStranger(ev bot.Event) {
	if ev.Callback != nil {
		if ev.Callback.Answer != nil {
			ev.Callback.Answer(bot.CallbackForbidden)
		}
		return
	}
	if ev.Voice != nil || ev.Attachment != nil || ev.Callback != nil {
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
				ev.Reply("не смог добавить в барицентр (возможно, он полон)")
				return
			}
			l.st.SetMemberName(orbitID, ev.FromUserID, ev.FromName)
			o := l.orbit(orbitID)
			l.notify(o, fmt.Sprintf("%s теперь в барицентре", esc(ev.FromName)))
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
			ev.Reply("не смог создать барицентр")
			return
		}
		l.st.SetMemberName(o.ID, ev.FromUserID, ev.FromName)
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
	l.handleTelegramMedia(o, ev, "voice", v.TGFileID, ev.FromName, v.Personal, v.Broadcast)
}

func (l *loop) handleTelegramAttachment(o *orbitState, ev bot.Event) {
	if l.cfg.SelfServiceOnboarding && !l.requireTelegramContentPolicy(ev) {
		return
	}
	attachment := ev.Attachment
	if !attachment.RightsAcknowledged {
		ev.Reply("Перед загрузкой подтвердите права на этот файл: отправьте его снова с подписью `rights` или `права`. Можно добавить `лично` либо `всем`/`all` в той же подписи. Подтверждение не доказывает владение и не заменяет необходимые права и согласия.")
		return
	}
	if attachment.MediaGroupID != "" {
		ev.Reply(bot.AttachmentFailureText(bot.AttachmentGroupUnsupported))
		return
	}
	title := attachment.FileName
	if title == "" {
		title = ev.FromName
	}
	l.handleTelegramMedia(
		o, ev, string(attachment.Kind), attachment.TGFileID, title,
		attachment.Personal, attachment.Broadcast,
	)
}

func telegramContentPolicyLocale(target string) store.ContentPolicyLocale {
	if target == "en" {
		return store.ContentPolicyLocaleEN
	}
	return store.ContentPolicyLocaleRU
}

func telegramContentPolicyText(manifest store.ContentPolicyManifest) string {
	return fmt.Sprintf("<b>%s</b>\n\n%s\n\n%s\n\nTerms: %s\nContent Guidelines: %s\n\nVersion: <code>%s</code>\nPolicy hash: <code>%s</code>",
		esc(manifest.Title), esc(manifest.RightsText), esc(manifest.ConsentText),
		manifest.TermsURL, manifest.ContentGuidelinesURL, manifest.Version, manifest.Hash)
}

func (l *loop) handleTelegramContentPolicy(ev bot.Event, accept bool) {
	locale := telegramContentPolicyLocale(ev.Command.Target)
	manifest, err := store.CurrentContentPolicy(locale)
	if err != nil {
		ev.Reply("locale: en или ru")
		return
	}
	if !accept {
		ev.Reply(telegramContentPolicyText(manifest) +
			fmt.Sprintf("\n\nAccept / Принять: /accept_content_policy %s", locale))
		return
	}
	ctx, err := l.st.ResolveTelegramActorContext(ev.FromUserID)
	if err != nil {
		l.log.Error("resolve Telegram content policy actor", "err", err)
		ev.Reply("Не удалось проверить аккаунт / Could not verify the account.")
		return
	}
	grant, err := l.st.AcceptContentPolicy(store.AcceptContentPolicyParams{
		ExpectedActorID: ctx.ActorID,
		Identity:        store.Identity{Kind: store.IdentityTelegram, TelegramUserID: ev.FromUserID},
		Version:         manifest.Version, PolicyHash: manifest.Hash, Locale: locale,
		AcceptedAt: time.Now().UnixMilli(),
	})
	if err != nil {
		var limited *store.ContentPolicyRateLimitError
		if errors.As(err, &limited) {
			ev.Reply("Слишком много изменений; попробуйте позже / Too many changes; try later.")
			return
		}
		l.log.Error("accept Telegram content policy", "err", err)
		ev.Reply("Не удалось принять правила / Could not accept the policy.")
		return
	}
	ev.Reply(telegramContentPolicyText(manifest) + fmt.Sprintf(
		"\n\nПринято / Accepted at %s (revision %d). Это не подтверждает владение контентом / This does not prove content ownership.",
		time.UnixMilli(grant.AcceptedAt).UTC().Format(time.RFC3339), grant.Revision))
}

func (l *loop) requireTelegramContentPolicy(ev bot.Event) bool {
	ctx, err := l.st.ResolveTelegramActorContext(ev.FromUserID)
	if err == nil {
		_, err = l.st.RequireCurrentContentPolicy(ctx.ActorID, store.Identity{
			Kind: store.IdentityTelegram, TelegramUserID: ev.FromUserID,
		})
	}
	if err == nil {
		return true
	}
	if errors.Is(err, store.ErrContentPolicyAcceptanceRequired) {
		ev.Reply("Перед отправкой audio/document откройте /content_policy и примите /accept_content_policy. Голосовые сообщения работают по прежнему правилу.\n\nBefore sending audio/document, open /content_policy en and accept with /accept_content_policy en. Voice messages keep the existing policy.")
		return false
	}
	l.log.Error("authorize Telegram content policy", "err", err)
	ev.Reply("Не удалось проверить правила загрузки / Could not verify upload policy.")
	return false
}

func (l *loop) handleTelegramMedia(
	o *orbitState,
	ev bot.Event,
	attachmentKind string,
	telegramFileID string,
	title string,
	personalHint bool,
	broadcastHint bool,
) {
	if l.telegramMedia == nil || l.telegramMediaInitErr != nil {
		l.log.Error("Telegram media adapter unavailable", "err", l.telegramMediaInitErr)
		ev.Reply("внутренняя ошибка, голосовое не принято")
		return
	}
	acceptedAt := time.Now()
	accepted, err := l.telegramMedia.Accept(media.TelegramVoice{
		OwnerOrbitID:   o.id,
		TelegramUserID: ev.FromUserID,
		TelegramFileID: telegramFileID,
		AttachmentKind: attachmentKind,
		Title:          title,
		AcceptedAt:     acceptedAt.UnixMilli(),
		ExpiresAt:      acceptedAt.AddDate(0, 0, l.cfg.Media.RetentionDays).UnixMilli(),
	})
	if err != nil {
		l.log.Error("Telegram media acceptance failed", "err", err)
		ev.Reply("внутренняя ошибка, голосовое не принято")
		return
	}
	// Personal is the orbit default (design §5): an explicit «лично» caption
	// forces it; «всем» forces broadcast.
	personal := personalHint || (o.voiceDefault == "personal" && !broadcastHint)
	if broadcastHint {
		personal = false
	}
	reply := ev.Reply
	if attachmentKind == "voice" {
		ev.Reply("голосовое принято — поставлю по времени отправки, даже если другое обработается быстрее")
	} else {
		ev.Reply("аудиовложение принято как неподтверждённый файл — тип, размер и длительность проверит общий ingest")
	}
	orbitID := o.id
	owner := l.stateFor(orbitID)
	orderAir := owner.id
	orderAirID := owner.airID
	orderGeneration := owner.authorityGeneration
	orderRevision := owner.airRevision
	sequence := int64(0)
	if orderAirID != "" {
		l.airVoiceAccepted[orderAirID]++
		sequence = l.airVoiceAccepted[orderAirID]
		if l.airVoiceNext[orderAirID] == 0 {
			l.airVoiceNext[orderAirID] = 1
		}
	} else {
		l.voiceAccepted[orderAir]++
		sequence = l.voiceAccepted[orderAir]
		if l.voiceNext[orderAir] == 0 {
			l.voiceNext[orderAir] = 1
		}
	}
	from := ev.FromUserID
	fromName := ev.FromName
	originalUpdateID := int64(0)
	if ev.Voice != nil {
		originalUpdateID = ev.Voice.OriginalUpdateID
	} else if ev.Attachment != nil {
		originalUpdateID = ev.Attachment.OriginalUpdateID
	}
	if originalUpdateID <= 0 {
		// Direct/internal callers predating transport update ids still receive a
		// stable positive binding; production Bot events always use update_id.
		originalUpdateID = ev.MessageID
	}
	go func() {
		res, err := l.telegramMedia.Submit(context.Background(), accepted)
		l.mediaCh <- mediaDone{
			orbit: orbitID, orderAir: orderAir, orderAirID: orderAirID,
			orderAirGeneration: orderGeneration,
			orderAirRevision:   orderRevision,
			mediaID:            accepted.MediaID, from: from, fromName: fromName,
			acceptedAt: accepted.AcceptedAt, sequence: sequence, personal: personal,
			attachmentKind: attachmentKind,
			chatID:         ev.ChatID, originalUpdateID: originalUpdateID,
			result: res, err: err, reply: reply,
		}
	}()
}

func (l *loop) handleMediaDone(d mediaDone) {
	// Tests and internal direct callers use sequence 0: process immediately.
	if d.sequence == 0 {
		l.processMediaDone(d)
		return
	}
	if d.orderAirID != "" {
		pending := l.airVoicePending[d.orderAirID]
		if pending == nil {
			pending = map[int64]mediaDone{}
			l.airVoicePending[d.orderAirID] = pending
		}
		pending[d.sequence] = d
		for {
			next := l.airVoiceNext[d.orderAirID]
			ready, ok := pending[next]
			if !ok {
				return
			}
			delete(pending, next)
			l.airVoiceNext[d.orderAirID] = next + 1
			l.processMediaDone(ready)
		}
	}
	orderAir := d.orderAir
	if orderAir == 0 { // backward-compatible direct test/internal callers
		orderAir = d.orbit
	}
	pending := l.voicePending[orderAir]
	if pending == nil {
		pending = map[int64]mediaDone{}
		l.voicePending[orderAir] = pending
	}
	pending[d.sequence] = d
	for {
		next := l.voiceNext[orderAir]
		ready, ok := pending[next]
		if !ok {
			return
		}
		delete(pending, next)
		l.voiceNext[orderAir] = next + 1
		l.processMediaDone(ready)
	}
}

func (l *loop) processMediaDone(d mediaDone) {
	if d.err != nil {
		l.log.Error("voice processing failed", "media", d.mediaID, "err", d.err)
		if err := l.st.UpdateMedia(store.MediaRecord{ID: d.mediaID, Status: "failed"}); err != nil {
			l.log.Error("legacy Telegram media failure mapping failed", "media", d.mediaID, "err", err)
		}
		if d.attachmentKind == "audio" || d.attachmentKind == "document" {
			code, ok := media.FailureCode(d.err)
			if !ok {
				code = "media_input_unavailable"
			}
			d.reply(bot.AttachmentFailureText(bot.AttachmentFailureFromIngest(bot.AttachmentKind(d.attachmentKind), code)))
		} else {
			d.reply("не смог обработать голосовое, оставил исходник для разбора")
		}
		return
	}
	if err := l.st.UpdateMedia(store.MediaRecord{
		ID: d.mediaID, DurationMS: d.result.DurationMS,
		PathWAV: d.result.WAVPath, LoudnormJSON: d.result.LoudnormJSON, Status: "ready",
	}); err != nil {
		l.log.Error("legacy Telegram media ready mapping failed", "media", d.mediaID, "err", err)
		if d.attachmentKind == "audio" || d.attachmentKind == "document" {
			d.reply(bot.AttachmentFailureText(bot.AttachmentDecodeFailed))
		} else {
			d.reply("не смог обработать голосовое, оставил исходник для разбора")
		}
		return
	}
	if l.orbitGone(d.orbit) { // L3: /dissolve raced the ffmpeg goroutine
		return
	}
	if d.originalUpdateID <= 0 {
		// Only rollback-era rows and direct internal callers lack a transport
		// update binding. They remain readable, but production Telegram events
		// always enter the common opaque-target service below.
		l.processLegacyTelegramMediaDone(d)
		return
	}
	audience, selectors, includeOrigin, ok, audienceErr := l.telegramDefaultAudience(d)
	if audienceErr != nil {
		l.log.Error("resolve Telegram default audience", "media", d.mediaID, "err", audienceErr)
		d.reply("аудиоклип готов, но адресатов доставки определить не удалось")
		return
	}
	if d.attachmentKind == "voice" && !ok {
		d.reply("в орбите пока только твой дом — личное голосовое некому отправить")
		return
	}
	originalUpdateID := d.originalUpdateID
	if originalUpdateID <= 0 {
		originalUpdateID = d.acceptedAt
	}
	registered, err := l.st.RegisterTelegramInlineRoute(store.RegisterTelegramInlineRouteParams{
		TelegramUserID: d.from, MediaID: d.mediaID,
		OriginalUpdateID: originalUpdateID, AttachmentKind: d.attachmentKind,
		AcceptedAt: d.acceptedAt, AudienceKind: audience, Selectors: selectors,
		IncludeOrigin: includeOrigin, Availability: l.telegramTransmissionAvailability(),
	})
	if err != nil {
		if errors.Is(err, store.ErrAirPolicyDenied) {
			d.reply("политика текущего Air запрещает этот способ доставки")
			return
		}
		l.log.Error("register Telegram inline route", "media", d.mediaID, "err", err)
		d.reply("аудиоклип готов, но маршрут доставки создать не удалось")
		return
	}
	if registered.Creation != nil {
		// The default is durable before the keyboard enters Telegram's outbox.
		l.handleTransmissionSignal(transmissionSignal{
			transmissionID: registered.Creation.Transmission.ID,
		})
	}
	if d.attachmentKind == "voice" {
		if d.personal {
			d.reply(fmt.Sprintf("личное голосовое от %s готово: после текущего трека, только адресату", esc(d.fromName)))
		} else {
			d.reply(fmt.Sprintf("голосовое от %s готово: после текущего трека, для всех", esc(d.fromName)))
		}
	} else {
		d.reply("аудиоклип готов — автозапуск для файлов выключен, выбери способ доставки")
	}
	l.sendTelegramRoutingPrompt(d.chatID, d.from, registered.Route, "Выбери способ и аудиторию:")
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
			l.hub.Send(key(e.To), protocol.TypeLoad, &protocol.LoadPayload{
				ElementID: e.ElementID, URI: e.URI, PositionMS: e.PositionMS,
				AdoptPlaying: e.AdoptPlaying,
			})
		case session.EffResumeAt:
			l.hub.Send(key(e.To), protocol.TypeResumeAt, &protocol.ResumeAtPayload{
				ElementID: e.ElementID, TCoordMS: e.TCoordMS, PositionMS: e.PositionMS,
			})
		case session.EffPause:
			l.hub.Send(key(e.To), protocol.TypePause, &protocol.PausePayload{ElementID: e.ElementID, FadeMS: e.FadeMS})
		case session.EffPlayVoice:
			fileURL := l.mediaURL(e.MediaID)
			if strings.HasPrefix(e.ElementID, "tr_") {
				fileURL = l.genericMediaURL(e.MediaID)
			}
			l.hub.Send(key(e.To), protocol.TypePlayVoice, &protocol.PlayVoicePayload{
				ElementID: e.ElementID,
				FileURL:   fileURL,
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
	airID := o.airID
	generation := o.authorityGeneration
	revision := o.airRevision
	o.readyTimer = time.AfterFunc(d, func() {
		l.timeouts <- orbitTimeout{orbit: orbitID, airID: airID, generation: generation, revision: revision, elementID: elementID}
	})
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

func modeLabel(mode session.Mode) string {
	if mode == session.ModeSolo {
		return "каждый слушает своё"
	}
	return "слушаем вместе"
}

func stateLabel(state session.State) string {
	switch state {
	case session.StateIdle:
		return "тишина"
	case session.StateLoading:
		return "загружаю трек"
	case session.StateArmed:
		return "готовимся начать"
	case session.StatePlaying:
		return "играет"
	case session.StateVoice:
		return "играет голосовое"
	case session.StatePaused:
		return "пауза"
	case session.StateDegraded:
		return "жду недоступный дом"
	default:
		return string(state)
	}
}

// peerName renders a session peer for chat texts. Group composites are shown
// as the Barycenter name, never as the internal "slot@orbit" identifier.
func (l *loop) peerName(o *orbitState, id protocol.NodeID) string {
	if !o.group() {
		slots, _ := l.st.ActiveSlots(o.id)
		return esc(presentation.TargetLabel(presentation.TargetMetadata{
			OrbitTitle: o.title, Slot: string(id), MultipleSlots: len(slots) > 1,
		}).Text(presentation.Russian))
	}
	orbit, slot, ok := splitComposite(id)
	if !ok {
		return string(id)
	}
	return l.namedGroupPeer(orbit, string(slot))
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

// orbitText is the /home view: who is in the barycenter (BY NAME, not raw tg
// id — bot-ux #4), each member's home and its liveness, plus any active
// approach. Vocabulary is «Барицентр»/«дом» only (bot-ux #9).
func (l *loop) orbitText(o *orbitState) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<b>«%s»</b>\n", esc(o.title))
	members, _ := l.st.Members(o.id)
	online := l.hub.Online(o.id)
	slots, _ := l.st.ActiveSlots(o.id)
	homed := 0
	for _, m := range members {
		name := esc(presentation.MemberLabel(m.DisplayName).Text(presentation.Russian))
		home := "без своего дома"
		if slot, _ := l.st.SlotOf(o.id, m.TGUserID); slot != "" {
			homed++
			mark := "офлайн"
			if online[protocol.NodeID(slot)] {
				mark = "в сети"
			}
			target := presentation.TargetLabel(presentation.TargetMetadata{
				OrbitTitle: o.title, Slot: slot, MultipleSlots: len(slots) > 1,
			}).Text(presentation.Russian)
			home = fmt.Sprintf("%s — %s", esc(target), mark)
		}
		fmt.Fprintf(&b, "· %s — %s, %s\n", name, memberRole(m.Role), home)
	}
	if homed == 0 {
		b.WriteString("домов пока нет — /pair выдаст код для приложения Pulsar\n")
	}
	// An active approach is part of the barycenter's identity (design §12).
	if linkID := l.linkOf[o.id]; linkID != 0 {
		if g := l.group(linkID); g != nil {
			for _, other := range g.orbits {
				if other != o.id {
					fmt.Fprintf(&b, "сближение с «%s» — эфир общий (/apart завершит)\n", esc(l.orbit(other).title))
				}
			}
		}
	} else if runtime, err := l.st.ActiveAirRuntimeForOrbit(o.id); err == nil && runtime != nil {
		for _, other := range runtime.OrbitIDs {
			if other == o.id {
				continue
			}
			if otherOrbit := l.orbit(other); otherOrbit != nil {
				fmt.Fprintf(&b, "сближение с «%s» — эфир общий (/apart завершит только для вас)\n",
					esc(otherOrbit.title))
			}
		}
	}
	return strings.TrimRight(b.String(), "\n")
}

// memberRole renders a star-system role in plain Russian for /home.
func memberRole(role string) string {
	switch role {
	case "primary":
		return "⭐ главная звезда"
	case "companion":
		return "участник"
	case "satellite":
		return "слушатель"
	}
	return role
}

// resolveMember maps a /make_primary argument (a display name as shown by
// /home, or — for back-compat — a raw tg id) to a member's tg id; 0 when not
// found. L12: the bot never stores Telegram @usernames (only first_name), so
// an "@ник" only ever matches by accident — the help texts say "имя из
// /home"; the @-strip below stays as a courtesy for users who type one.
func (l *loop) resolveMember(orbitID int64, target string) int64 {
	target = strings.TrimSpace(target)
	if target == "" {
		return 0
	}
	if id, err := l.st.MemberByName(orbitID, target); err == nil && id != 0 {
		return id
	}
	if n, err := strconv.ParseInt(strings.TrimPrefix(target, "@"), 10, 64); err == nil {
		if m, _ := l.st.MemberOf(n); m != nil && m.OrbitID == orbitID {
			return n
		}
	}
	return 0
}

func (l *loop) queueText(o *orbitState) string {
	var b strings.Builder
	if cur := o.sess.Current; cur != nil {
		fmt.Fprintf(&b, "сейчас: %s\n", l.elementLabel(o, *cur))
	}
	if o.sess.QueueLen() == 0 {
		b.WriteString("очередь пуста")
	} else {
		b.WriteString("очередь:\n")
		for i, el := range o.sess.Queue {
			requester := presentation.SenderLabel(string(el.RequestedBy)).Text(presentation.Russian)
			fmt.Fprintf(&b, "%d. %s <i>(от %s)</i>\n", i+1, l.elementLabel(o, el), esc(requester))
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
		from := presentation.SenderLabel(string(el.RequestedBy)).Text(presentation.Russian)
		return fmt.Sprintf("голосовое от %s · %s · %s", esc(from), fmtMS(el.DurationMS), who)
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
		return fmt.Sprintf("сейчас: %s · %s · %s", l.elementLabel(o, *cur), fmtMS(l.livePosition(o)), stateLabel(o.sess.State))
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
	fmt.Fprintf(&b, "<b>«%s»</b> — %s, %s\n", esc(o.title), modeLabel(o.sess.Mode), stateLabel(o.sess.State))
	now := time.Now().UnixMilli()
	snapshots := l.hub.NodeSnapshots()
	for _, n := range l.sessionPeers(o) {
		name := l.peerName(o, n)
		key := l.nodeKey(o, n)
		snapshot, exists := snapshots[key]
		online := exists && snapshot.Connected &&
			snapshot.LastSeenAt > now-int64((12*time.Second)/time.Millisecond)
		if !online {
			fmt.Fprintf(&b, "Пульсар %s: офлайн, выход unavailable, воспроизведение unknown\n", name)
			continue
		}
		output := "degraded"
		if snapshot.Capabilities.Supports(protocol.CapabilityMediaClip) && !snapshot.OutputDegraded {
			output = "ready"
		}
		playback := snapshot.PlaybackState
		if playback == "" {
			playback = "unknown"
		}
		dnd := string(store.DNDAllowAll)
		if target, err := l.st.CurrentInstallationTargetForSocket(
			key.Orbit, string(key.Slot), snapshot.CredentialTokenHash,
		); err == nil && target != nil {
			if effective, err := l.st.EffectiveDND(context.Background(), *target, now); err == nil {
				dnd = string(effective.Mode)
			}
		}
		capabilities := snapshot.Capabilities.Values()
		fmt.Fprintf(&b, "Пульсар %s: онлайн, выход %s, воспроизведение %s, DND %s, возможности %s\n",
			name, output, playback, dnd, strings.Join(capabilities, ","))
	}
	fmt.Fprintf(&b, "координатор %s", version)
	return b.String()
}
