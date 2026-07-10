// Package session implements the coordinator session state machine
// (spec 7.2) as a pure core: events in, effects out, no I/O, no clocks.
// The caller (main loop) owns timers, transport and persistence.
package session

import (
	"fmt"
	"strings"

	"relux.works/duet/coordinator/internal/protocol"
)

type Mode string

const (
	ModeShared Mode = "shared"
	ModeSolo   Mode = "solo"
)

type State string

const (
	StateIdle     State = "idle"
	StateLoading  State = "loading"
	StateArmed    State = "armed"
	StatePlaying  State = "playing"
	StateVoice    State = "voice"
	StatePaused   State = "paused"
	StateDegraded State = "degraded"
)

type Kind string

const (
	KindTrack Kind = "track"
	KindVoice Kind = "voice"
)

// GateMode selects the offline gate for starting material (design §12 L1.5
// "living air"). Strict is the historic rule: every peer must be online.
// EachSide is for linked (group) sessions: the broadcast may start as soon
// as every SIDE (linked orbit) has at least one home online — offline homes
// do not block and catch up individually when they wake (JoinInProgress).
type GateMode string

const (
	GateStrict   GateMode = "strict"
	GateEachSide GateMode = "eachSide"
)

// Element is the unit of airtime (spec 5.1).
type Element struct {
	ID   string
	Kind Kind
	URI  string // kind=track
	// CTID references the canonical track (spec-providers §2). Set only when
	// the provider layer is on; URI stays the denormalized origin ref.
	CTID        string `json:"ctid,omitempty"`
	MediaID     string // kind=voice
	Title       string
	DurationMS  int64
	RequestedBy protocol.NodeID
	Target      string // "both" | "a" | "b"
	CreatedAt   int64
}

// defaultPeers keeps two-home behavior for sessions created before the
// orbit's slot list is applied (M2: peers are set from the slots table).
var defaultPeers = []protocol.NodeID{protocol.NodeA, protocol.NodeB}

// --- Effects: what the caller must do after an event ---

type Effect any

type (
	EffLoad struct {
		To         protocol.NodeID
		ElementID  string
		URI        string
		PositionMS int64
		// AdoptPlaying means To is the Pulsar where the user already started
		// this URI. It only relabels the live stream; no pause/reload is allowed.
		AdoptPlaying bool
	}
	EffResumeAt struct {
		To        protocol.NodeID
		ElementID string
		TCoordMS  int64
		// PositionMS is set for followers joining an already-playing leader.
		// They seek while paused and resume at T at the leader's future position.
		PositionMS *int64
	}
	EffPause struct {
		To        protocol.NodeID
		ElementID string
		FadeMS    int64
	}
	EffPlayVoice struct {
		To        protocol.NodeID
		ElementID string
		MediaID   string
	}
	EffWait struct {
		To         protocol.NodeID
		ElementID  string
		DurationMS int64
	}
	EffStop    struct{ To protocol.NodeID }
	EffSetMode struct {
		To   protocol.NodeID
		Mode Mode
	}
	// EffNotify is a chat message (spec 9.2: no spam — only meaningful events).
	EffNotify struct{ Text string }
	// EffArmReadyTimer: caller starts the ready timeout for the element.
	EffArmReadyTimer    struct{ ElementID string }
	EffCancelReadyTimer struct{}
	// EffLogDesync: record the measured start skew (spec 7.2 ARMED->PLAYING).
	EffLogDesync struct{ DeltaMS int64 }
	// EffElementDone: journal entry for the elements table (spec 5.3).
	EffElementDone struct {
		Element Element
		Status  string // "eof" | "skipped" | "error"
	}
	// EffPersist: session state changed in a way worth persisting.
	EffPersist struct{}
)

// Playlist is the base layer of the shared broadcast (U10): an expanded
// Spotify playlist/album the air returns to whenever the insert queue is
// empty. Cursor points at the NEXT track to play.
type Playlist struct {
	URI    string   `json:"uri"`
	Title  string   `json:"title"`
	Tracks []string `json:"tracks"` // spotify:track:... uris
	Cursor int      `json:"cursor"`
}

// Session is the FSM. Not safe for concurrent use; the owning loop serializes.
type Session struct {
	Mode  Mode
	State State

	// Queue holds inserts (single tracks, voices) that cut ahead of the
	// playlist layer (U10). Playlist is the base flow underneath.
	Queue    []Element
	Playlist *Playlist
	Current  *Element
	// SavedPositionMS: where to resume the current element after pause/degraded.
	SavedPositionMS int64

	ready     map[protocol.NodeID]bool
	started   map[protocol.NodeID]int64
	ended     map[protocol.NodeID]bool
	voiceDone map[protocol.NodeID]bool
	retried   bool

	online  map[protocol.NodeID]bool
	nodePos map[protocol.NodeID]int64 // audible position from heartbeats
	// nodePosAt is the coordinator timestamp of nodePos. It lets a catch-up
	// start aim at where a live leader will be at T instead of a stale 5 s
	// heartbeat position.
	nodePosAt map[protocol.NodeID]int64
	nodeRTT   map[protocol.NodeID]int64

	// adoptionLeader is non-empty while the current element originated from
	// Spotify on that Pulsar. The leader stays audible during the ready barrier;
	// every other participant catches up to it.
	adoptionLeader protocol.NodeID

	// participants: peers the CURRENT element was dealt to (sealed at
	// loadCurrent/startVoice). nil means "everyone" — strict sessions and
	// restored snapshots keep the historic all-peers barriers. Living air
	// (eachSide) seals the online subset so absent homes never block
	// ready/started/ended accounting.
	participants map[protocol.NodeID]bool
	// joining: homes catching up with an element already in flight
	// (JoinInProgress). Their ready arms a solo resume_at, never the main
	// barrier; they graduate into participants once started arrives.
	joining map[protocol.NodeID]bool
	// lastAbsent dedups the "стартуем без ..." notify: repeat it only when
	// the set of absent homes changes, not on every element (spec 9.2).
	lastAbsent map[protocol.NodeID]bool
	// pausedLocally: homes whose USER paused this Pulsar in the Spotify app
	// (personal pause, 2026-07-10). Excluded from barriers and from sealing
	// of subsequent elements until OnUserResume. Runtime-only: a coordinator
	// restart or the node going offline clears it — catch-up then resumes
	// the home, the safer default when state is lost.
	pausedLocally map[protocol.NodeID]bool

	// Peers: the orbit's pulsars (slots). The broadcast machinery runs over
	// this set (M2: N homes, not two).
	Peers []protocol.NodeID

	// GateMode: which offline gate advance() consults (living air, §12 L1.5).
	GateMode GateMode
	// SideOf maps a peer to its side for the eachSide gate. The loop sets it
	// for group sessions (composite "orbit:slot" prefix = orbit id). nil =
	// every peer is its own side, which makes eachSide degenerate to strict.
	SideOf func(protocol.NodeID) string

	// StartMarginMS: extra margin in resume_at scheduling (spec 7.1 scheduler).
	StartMarginMS int64
}

func New() *Session {
	return &Session{
		Mode:          ModeShared,
		State:         StateIdle,
		ready:         map[protocol.NodeID]bool{},
		started:       map[protocol.NodeID]int64{},
		ended:         map[protocol.NodeID]bool{},
		voiceDone:     map[protocol.NodeID]bool{},
		online:        map[protocol.NodeID]bool{},
		nodePos:       map[protocol.NodeID]int64{},
		nodePosAt:     map[protocol.NodeID]int64{},
		nodeRTT:       map[protocol.NodeID]int64{},
		joining:       map[protocol.NodeID]bool{},
		pausedLocally: map[protocol.NodeID]bool{},
		Peers:         append([]protocol.NodeID{}, defaultPeers...),
		GateMode:      GateStrict,
		StartMarginMS: 500,
	}
}

// SetPeers replaces the peer set (orbit slots at restore/creation time).
func (s *Session) SetPeers(slots []string) {
	s.Peers = s.Peers[:0]
	for _, sl := range slots {
		s.Peers = append(s.Peers, protocol.NodeID(sl))
	}
}

// EnsurePeer adds a newly paired slot to the set (offline until it talks).
func (s *Session) EnsurePeer(n protocol.NodeID) {
	for _, p := range s.Peers {
		if p == n {
			return
		}
	}
	s.Peers = append(s.Peers, n)
}

// SeedOnline replaces liveness knowledge wholesale, without effects. Link
// boundaries (design §12) move nodes between sessions: the session that
// starts consuming a node's events must not trust a stale map, and the hub
// only emits EvOnline on real transitions.
func (s *Session) SeedOnline(online map[protocol.NodeID]bool) {
	s.online = map[protocol.NodeID]bool{}
	for n, on := range online {
		s.online[n] = on
	}
}

// RemovePeer drops a revoked slot and re-evaluates everything the missing
// peer might have been blocking (gate, ready, ended, voice completion).
func (s *Session) RemovePeer(nowMS int64, n protocol.NodeID) []Effect {
	kept := s.Peers[:0]
	for _, p := range s.Peers {
		if p != n {
			kept = append(kept, p)
		}
	}
	s.Peers = kept
	delete(s.online, n)
	delete(s.ready, n)
	delete(s.started, n)
	delete(s.ended, n)
	delete(s.voiceDone, n)
	delete(s.nodePos, n)
	delete(s.nodePosAt, n)
	delete(s.nodeRTT, n)
	if s.participants != nil {
		delete(s.participants, n)
	}
	delete(s.joining, n)
	delete(s.pausedLocally, n)

	switch s.State {
	case StateDegraded:
		if s.gateSatisfied() {
			if s.Current == nil && s.hasUpcoming() {
				return append([]Effect{EffNotify{Text: "все дома в сети — поехали"}}, s.advance()...)
			}
			if s.Current == nil {
				s.State = StateIdle
				return []Effect{EffPersist{}}
			}
			s.State = StatePaused
			return []Effect{EffNotify{Text: "оставшиеся дома в сети. /resume чтобы продолжить"}, EffPersist{}}
		}
	case StateLoading:
		if s.Current != nil {
			return s.checkAllReady(nowMS, s.Current.ID)
		}
	case StateArmed:
		// The revoked home may have been the only one not yet started — the
		// same stall as the offline drop (H1), via /revoke or /leave.
		if effs := s.checkAllStarted(); effs != nil {
			return effs
		}
	case StatePlaying:
		if s.Current != nil && len(s.ended) > 0 && s.endedConditionMet() {
			return s.finishCurrent("eof")
		}
	case StateVoice:
		if s.Current != nil && s.voiceCompleteAcrossPeers() {
			return s.finishCurrent("eof")
		}
	}
	return []Effect{EffPersist{}}
}

func (s *Session) peersExcept(n protocol.NodeID) []protocol.NodeID {
	var out []protocol.NodeID
	for _, p := range s.Peers {
		if p != n {
			out = append(out, p)
		}
	}
	return out
}

func (s *Session) targetNodes(el *Element) []protocol.NodeID {
	if el.Target != "" && el.Target != "both" {
		return []protocol.NodeID{protocol.NodeID(el.Target)}
	}
	return s.Peers
}

func (s *Session) resetElementTracking() {
	s.ready = map[protocol.NodeID]bool{}
	s.started = map[protocol.NodeID]int64{}
	s.ended = map[protocol.NodeID]bool{}
	s.voiceDone = map[protocol.NodeID]bool{}
	s.joining = map[protocol.NodeID]bool{}
	s.participants = nil // resealed by loadCurrent/startVoice
	s.retried = false
	// Positions are per-element: a stale heartbeat from the previous track
	// (StatePayload carries no element id) must not satisfy the next track's
	// near-end condition or aim a joiner's load past the new track's length.
	// Missing data is read as "not near end" / "position 0", both safe.
	// nodeRTT survives — round-trips are element-independent.
	s.nodePos = map[protocol.NodeID]int64{}
	s.nodePosAt = map[protocol.NodeID]int64{}
	s.adoptionLeader = ""
}

// --- Queue operations (spec 7.3) ---

// EnqueueTrack appends to the tail. Starts playback if idle.
func (s *Session) EnqueueTrack(el Element) []Effect {
	s.Queue = append(s.Queue, el)
	return s.maybeAdvanceFromIdle()
}

// EnqueueVoice inserts right after the current element: before all queued
// tracks but after voices queued earlier (arrival order preserved).
func (s *Session) EnqueueVoice(el Element) []Effect {
	idx := 0
	for idx < len(s.Queue) && s.Queue[idx].Kind == KindVoice &&
		(s.Queue[idx].CreatedAt < el.CreatedAt ||
			(s.Queue[idx].CreatedAt == el.CreatedAt && s.Queue[idx].ID < el.ID)) {
		idx++
	}
	s.Queue = append(s.Queue[:idx], append([]Element{el}, s.Queue[idx:]...)...)
	return s.maybeAdvanceFromIdle()
}

// EnqueueTrackAfterVoices preserves a voice block already promised to users,
// then puts a freshly selected Spotify track ahead of older music inserts.
func (s *Session) EnqueueTrackAfterVoices(el Element) []Effect {
	idx := 0
	for idx < len(s.Queue) && s.Queue[idx].Kind == KindVoice {
		idx++
	}
	s.Queue = append(s.Queue[:idx], append([]Element{el}, s.Queue[idx:]...)...)
	return s.maybeAdvanceFromIdle()
}

// Cancel removes queue position n (1-based, as shown by /queue).
func (s *Session) Cancel(n int) ([]Effect, error) {
	if n < 1 || n > len(s.Queue) {
		return nil, fmt.Errorf("queue has %d items, no item %d", len(s.Queue), n)
	}
	s.Queue = append(s.Queue[:n-1], s.Queue[n:]...)
	return []Effect{EffPersist{}}, nil
}

func (s *Session) maybeAdvanceFromIdle() []Effect {
	if s.Mode != ModeShared || s.State != StateIdle {
		return []Effect{EffPersist{}}
	}
	return s.advance()
}

// allOnline: every peer online. An orbit with no paired pulsars is never
// "all online" — queued material waits for the first home to pair.
func (s *Session) allOnline() bool {
	if len(s.Peers) == 0 {
		return false
	}
	for _, n := range s.Peers {
		if !s.online[n] {
			return false
		}
	}
	return true
}

func (s *Session) sideOf(n protocol.NodeID) string {
	if s.SideOf != nil {
		return s.SideOf(n)
	}
	return string(n)
}

// gateSatisfied: may material start? Strict = every home online (historic
// rule). EachSide (living air, §12 L1.5) = every side has at least one home
// online; the rest catch up when they wake.
func (s *Session) gateSatisfied() bool {
	if s.GateMode != GateEachSide {
		return s.allOnline()
	}
	if len(s.Peers) == 0 {
		return false
	}
	sideAlive := map[string]bool{}
	for _, n := range s.Peers {
		side := s.sideOf(n)
		sideAlive[side] = sideAlive[side] || s.online[n]
	}
	for _, alive := range sideAlive {
		if !alive {
			return false
		}
	}
	return true
}

// counts reports whether a peer is on the current element's barriers
// (ready/started/ended/voice). nil participants = everyone (strict sessions
// and restored snapshots keep the historic behavior).
func (s *Session) counts(n protocol.NodeID) bool {
	return s.participants == nil || s.participants[n]
}

func (s *Session) hasPeer(n protocol.NodeID) bool {
	for _, p := range s.Peers {
		if p == n {
			return true
		}
	}
	return false
}

// HasPeer reports whether n is one of the session's peers (loop-side checks).
func (s *Session) HasPeer(n protocol.NodeID) bool { return s.hasPeer(n) }

// IsOnline reports the session's liveness belief for a peer. The loop compares
// it against the fact of an arriving message to heal the hub's EvOnline /
// stale-EvOffline reorder race — the hub map ends "online", so no further
// EvOnline ever comes while the FSM still believes the home is dark (M1).
func (s *Session) IsOnline(n protocol.NodeID) bool { return s.online[n] }

func (s *Session) hasUpcoming() bool {
	if len(s.Queue) > 0 {
		return true
	}
	p := s.Playlist
	return p != nil && p.Cursor < len(p.Tracks)
}

// advance takes the next element and drives it (spec 4.1): inserts first,
// then the playlist layer (U10). Current must be finished or intentionally
// abandoned by the caller before advancing.
//
// Spec 7.2 (strict gate): the broadcast never starts with a home missing —
// with material queued but a node offline we park in DEGRADED and resume
// automatically when both homes are back. Living air (eachSide, §12 L1.5)
// relaxes this for linked sessions: one home per side is enough, the rest
// join in flight.
func (s *Session) advance() []Effect { return s.advanceAt(0) }

// advanceAt is advance with a starting position for the next queued track.
// Spotify selections use it to turn the initiating Pulsar's audible position
// into the common load point; every other transition starts at zero.
func (s *Session) advanceAt(positionMS int64) []Effect {
	if positionMS < 0 {
		positionMS = 0
	}
	s.resetElementTracking()
	s.SavedPositionMS = positionMS
	// Every home on personal pause: idle with the queue intact instead of
	// dealing an element nobody would play. Reachable only via membership
	// changes — the last ACTIVE home cannot personally pause (that degrades
	// to a global pause in OnUserPause); OnUserResume re-advances from here.
	if s.hasUpcoming() && s.allPausedLocally() {
		s.Current = nil
		s.State = StateIdle
		return []Effect{EffPersist{}}
	}
	if s.hasUpcoming() && !s.gateSatisfied() {
		s.Current = nil
		s.State = StateDegraded
		return []Effect{s.parkedNotify(), EffPersist{}}
	}
	if len(s.Queue) > 0 {
		el := s.Queue[0]
		s.Queue = s.Queue[1:]
		s.Current = &el
		if el.Kind == KindTrack {
			return s.loadCurrent(positionMS)
		}
		return s.startVoice()
	}
	// Insert queue drained: return to the playlist at the next track after
	// the interruption point.
	if p := s.Playlist; p != nil && p.Cursor < len(p.Tracks) {
		el := Element{
			ID:        "el_pl_" + fmt.Sprintf("%s_%d", shortPlaylistID(p.URI), p.Cursor),
			Kind:      KindTrack,
			URI:       p.Tracks[p.Cursor],
			Title:     fmt.Sprintf("%s · %d/%d", p.Title, p.Cursor+1, len(p.Tracks)),
			Target:    "both",
			CreatedAt: 0,
		}
		p.Cursor++
		s.Current = &el
		return s.loadCurrent(0)
	}
	s.Current = nil
	s.State = StateIdle
	effs := []Effect{EffPersist{}}
	if p := s.Playlist; p != nil && p.Cursor >= len(p.Tracks) {
		effs = append(effs, EffNotify{Text: fmt.Sprintf("плейлист «%s» доиграл до конца — кидайте ссылки", p.Title)})
	} else if s.Playlist == nil {
		effs = append(effs, EffNotify{Text: "очередь кончилась — кидайте ссылки"})
	}
	return effs
}

// htmlEscape guards user/provider-supplied text inside EffNotify (M5): the
// bot sends parse_mode=HTML, and a title containing '<' (e.g. "<3") makes
// Telegram reject the whole message — the outbox logs and DROPS it, so the
// user never learns the track was skipped. Only the Sprintf sites that embed
// Element.Title need this; peer ids and our own literals are tag-free.
var htmlEscaper = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func htmlEscape(s string) string { return htmlEscaper.Replace(s) }

func shortPlaylistID(uri string) string {
	if i := len(uri) - 8; i > 0 {
		return uri[i:]
	}
	return uri
}

// SetPlaylist installs/replaces the shared playlist layer (U10). Playback of
// the base flow starts when the insert queue drains (or immediately if idle).
func (s *Session) SetPlaylist(uri, title string, tracks []string) []Effect {
	s.Playlist = &Playlist{URI: uri, Title: title, Tracks: tracks, Cursor: 0}
	effs := []Effect{EffNotify{Text: fmt.Sprintf("общий плейлист: «%s», %d треков", title, len(tracks))}}
	return append(effs, s.maybeAdvanceFromIdle()...)
}

// parkedNotify names why the gate parked the air. Strict text is the
// historic one; eachSide names the homes of the side that is fully dark.
func (s *Session) parkedNotify() Effect {
	if s.GateMode != GateEachSide {
		missing := ""
		for _, n := range s.Peers {
			if !s.online[n] {
				missing += " " + string(n)
			}
		}
		if missing == "" {
			missing = " (ни одного пульсара не подключено)"
		}
		return EffNotify{Text: fmt.Sprintf("эфир подождёт: дом%s не в сети — продолжу, как только все дома подключатся", missing)}
	}
	sideAlive := map[string]bool{}
	for _, n := range s.Peers {
		side := s.sideOf(n)
		sideAlive[side] = sideAlive[side] || s.online[n]
	}
	var dark []string
	for _, n := range s.Peers {
		if !sideAlive[s.sideOf(n)] {
			dark = append(dark, string(n))
		}
	}
	if len(dark) == 0 {
		return EffNotify{Text: "эфир подождёт: ни одного пульсара не подключено — продолжу, как только дома появятся"}
	}
	return EffNotify{Text: fmt.Sprintf("эфир подождёт: с одной стороны все дома офлайн (%s) — продолжу, как только там кто-нибудь проснётся", strings.Join(dark, ", "))}
}

// sealParticipants pins the peer set the element is dealt to. Strict keeps
// nil (= everyone, historic barriers). Living air seals the online subset
// and, when the absent set changed, announces who will catch up later.
func (s *Session) sealParticipants(effs []Effect) []Effect {
	if s.GateMode != GateEachSide {
		// Strict sessions historically run with nil participants (= everyone).
		// Personal pause needs the set materialized so the paused homes can
		// be excluded; without any, keep the historic nil.
		if len(s.pausedLocally) == 0 {
			s.participants = nil
			return effs
		}
		s.participants = map[protocol.NodeID]bool{}
		for _, n := range s.Peers {
			if !s.pausedLocally[n] {
				s.participants[n] = true
			}
		}
		return effs
	}
	s.participants = map[protocol.NodeID]bool{}
	absentSet := map[protocol.NodeID]bool{}
	var absent []string
	for _, n := range s.Peers {
		if s.pausedLocally[n] {
			continue // personal pause: not dealt to, not announced as absent
		}
		if s.online[n] {
			s.participants[n] = true
		} else {
			absentSet[n] = true
			absent = append(absent, string(n))
		}
	}
	changed := len(absentSet) != len(s.lastAbsent)
	for n := range absentSet {
		if !s.lastAbsent[n] {
			changed = true
		}
	}
	s.lastAbsent = absentSet
	if len(absent) > 0 && changed {
		effs = append(effs, EffNotify{Text: fmt.Sprintf("стартуем без %s — догонят, как проснутся", strings.Join(absent, ", "))})
	}
	return effs
}

// Transplant seeds a fresh (idle) session with a donor's in-flight content
// and starts it through the gate (approach-to-stream, §12 L1.5): the home
// whose code opened the approach keeps playing onto all homes. cur may be
// nil (only a queue/playlist moved); positionMS is the donor's live position.
func (s *Session) Transplant(cur *Element, positionMS int64, queue []Element, playlist *Playlist) []Effect {
	s.Queue = queue
	s.Playlist = playlist
	if cur == nil {
		return s.maybeAdvanceFromIdle()
	}
	s.Current = cur
	s.resetElementTracking()
	// Gate BEFORE the voice branch (L10): a voice current used to start on
	// the alive side even with the other side fully dark — the same park
	// rule must cover both kinds, exactly as advance() does.
	if !s.gateSatisfied() {
		s.State = StateDegraded
		return []Effect{s.parkedNotify(), EffPersist{}}
	}
	if cur.Kind == KindVoice {
		return s.startVoice()
	}
	return s.loadCurrent(positionMS)
}

func (s *Session) loadCurrent(positionMS int64) []Effect {
	s.State = StateLoading
	s.SavedPositionMS = positionMS
	effs := s.sealParticipants([]Effect{EffPersist{}})
	for _, n := range s.Peers {
		if !s.counts(n) {
			continue
		}
		effs = append(effs, EffLoad{To: n, ElementID: s.Current.ID, URI: s.Current.URI, PositionMS: positionMS})
	}
	effs = append(effs, EffArmReadyTimer{ElementID: s.Current.ID})
	return effs
}

func (s *Session) startVoice() []Effect {
	s.State = StateVoice
	el := s.Current
	effs := s.sealParticipants([]Effect{EffPersist{}})
	targets := s.targetNodes(el)
	targetSet := map[protocol.NodeID]bool{}
	for _, n := range targets {
		if !s.counts(n) {
			continue
		}
		targetSet[n] = true
		effs = append(effs, EffPlayVoice{To: n, ElementID: el.ID, MediaID: el.MediaID})
	}
	for _, n := range s.Peers {
		if !targetSet[n] && s.counts(n) {
			effs = append(effs, EffWait{To: n, ElementID: el.ID, DurationMS: el.DurationMS})
		}
	}
	return effs
}

// --- Node events ---

func (s *Session) isCurrent(elementID string) bool {
	return s.Current != nil && s.Current.ID == elementID
}

// OnReady handles a node's ready. When all nodes are ready, arms the start:
// T = now + 2*max(rtt) + margin (spec 7.1 scheduler).
func (s *Session) OnReady(nowMS int64, node protocol.NodeID, elementID string) []Effect {
	// A home catching up under living air (§12 L1.5) arms alone: its ready
	// gives it a solo resume_at without touching the barrier of the homes
	// already playing. It starts a touch behind the leaders (load latency);
	// /sync realigns if it matters.
	if s.joining[node] && s.isCurrent(elementID) &&
		(s.State == StatePlaying || s.State == StateArmed) {
		s.ready[node] = true
		t := nowMS + 2*s.nodeRTT[node] + s.StartMarginMS
		position := s.livePositionAt(t)
		return []Effect{EffResumeAt{
			To: node, ElementID: elementID, TCoordMS: t, PositionMS: &position,
		}}
	}
	if s.State != StateLoading || !s.isCurrent(elementID) {
		return nil // idempotency (spec 7.2)
	}
	s.ready[node] = true
	return s.checkAllReady(nowMS, elementID)
}

// JoinInProgress deals the currently-playing track to a home that just came
// online under living air (§12 L1.5): an individual load at the live
// position, then a solo resume_at on its ready. Returns nil when there is
// nothing to join (no current track, not playing, unknown or already a
// participant home).
func (s *Session) JoinInProgress(node protocol.NodeID) []Effect {
	return s.JoinInProgressAt(0, node)
}

// JoinInProgressAt is JoinInProgress with the coordinator timestamp used to
// extrapolate the leaders' heartbeat positions.
func (s *Session) JoinInProgressAt(nowMS int64, node protocol.NodeID) []Effect {
	if s.Current == nil || s.Current.Kind != KindTrack {
		return nil
	}
	if s.State != StatePlaying && s.State != StateArmed {
		return nil
	}
	if !s.hasPeer(node) || s.counts(node) {
		return nil
	}
	if s.pausedLocally[node] {
		// The user paused this home on purpose — a liveness edge (e.g. the
		// hub race replay) must not yank it back into the air; only
		// OnUserResume does.
		return nil
	}
	// The join IS this node's online edge (the loop routes EvOnline here
	// instead of OnNodeBack). Record it, or the next element's participant
	// sealing and gate still see the home dark and it goes silent right after
	// the catch-up track — the hub never re-emits EvOnline for a live node.
	s.online[node] = true
	s.joining[node] = true
	return []Effect{EffLoad{
		To: node, ElementID: s.Current.ID, URI: s.Current.URI,
		PositionMS: s.livePositionAt(nowMS),
	}}
}

// LivePositionForTransplant is the safe resume point when a donor's stream
// moves into a group (approach-to-stream): the minimum audible position, so
// the new homes never skip past what the donor is hearing.
func (s *Session) LivePositionForTransplant() int64 {
	return s.currentPositionEstimate()
}

// livePosition estimates where the playing homes are now (max participant
// heartbeat position) so a joiner loads near the leaders.
func (s *Session) livePosition() int64 {
	return s.livePositionAt(0)
}

func (s *Session) livePositionAt(nowMS int64) int64 {
	var best int64
	for _, n := range s.Peers {
		if !s.counts(n) {
			continue
		}
		pos := s.nodePos[n]
		at := s.nodePosAt[n]
		playing := s.State == StatePlaying || s.State == StateArmed || n == s.adoptionLeader
		if playing && nowMS > 0 && at > 0 && nowMS > at {
			pos += nowMS - at
		}
		if pos > best {
			best = pos
		}
	}
	return best
}

// checkAllReady arms the synchronized start once every PARTICIPANT reported
// ready: T = now + 2*max(rtt over participants) + margin (spec 7.1, N-wise
// in M2). Non-participants (offline homes under living air, §12 L1.5) are
// not on the barrier — they catch up via JoinInProgress.
func (s *Session) checkAllReady(nowMS int64, elementID string) []Effect {
	if s.State != StateLoading || !s.isCurrent(elementID) {
		return nil
	}
	var maxRTT int64
	for _, n := range s.Peers {
		if !s.counts(n) {
			continue
		}
		if !s.ready[n] {
			return nil
		}
		if s.nodeRTT[n] > maxRTT {
			maxRTT = s.nodeRTT[n]
		}
	}
	s.State = StateArmed
	t := nowMS + 2*maxRTT + s.StartMarginMS
	effs := []Effect{EffCancelReadyTimer{}, EffPersist{}}
	var catchUpPosition *int64
	if s.adoptionLeader != "" {
		position := s.livePositionAt(t)
		catchUpPosition = &position
		// The leader is already audible. Model its content-aligned start at T
		// so the ordinary started barrier measures follower skew around T,
		// rather than the several seconds spent loading followers.
		s.started[s.adoptionLeader] = t
	}
	followers := 0
	for _, n := range s.Peers {
		if s.counts(n) {
			if n == s.adoptionLeader {
				continue
			}
			followers++
			effs = append(effs, EffResumeAt{
				To: n, ElementID: elementID, TCoordMS: t,
				PositionMS: catchUpPosition,
			})
		}
	}
	if s.adoptionLeader != "" && followers == 0 {
		s.State = StatePlaying
		s.adoptionLeader = ""
	}
	return effs
}

func (s *Session) OnStarted(node protocol.NodeID, elementID string, tFirstSampleMS int64) []Effect {
	// A catching-up home (living air, §12 L1.5) graduates into the
	// participant set once it starts, so it counts for ended from here on.
	// No desync recompute — it joined mid-flight, its skew is expected.
	if s.joining[node] && s.isCurrent(elementID) {
		s.started[node] = tFirstSampleMS
		delete(s.joining, node)
		if s.participants != nil {
			s.participants[node] = true
		}
		// Graduation can complete the main barrier too: if the air was still
		// ARMED, the joiner may have been the only home not yet started.
		if effs := s.checkAllStarted(); effs != nil {
			return effs
		}
		return []Effect{EffPersist{}}
	}
	if s.State != StateArmed || !s.isCurrent(elementID) {
		return nil
	}
	s.started[node] = tFirstSampleMS
	return s.checkAllStarted()
}

// checkAllStarted flips ARMED -> PLAYING once every participant reported
// started. Split out of OnStarted because the barrier must also be re-run
// when the participant set SHRINKS (offline drop, /revoke): the vanished
// last laggard used to stall the air in ARMED forever — OnEnded ignores
// non-PLAYING states and no timer covers ARMED (H1).
func (s *Session) checkAllStarted() []Effect {
	if s.State != StateArmed {
		return nil
	}
	var earliest, latest int64
	first := true
	for _, n := range s.Peers {
		if !s.counts(n) {
			continue // offline home under living air — catches up later
		}
		t, ok := s.started[n]
		if !ok {
			return nil
		}
		if first || t < earliest {
			earliest = t
		}
		if first || t > latest {
			latest = t
		}
		first = false
	}
	if first {
		return nil // no participants left at all — the gate/park path owns this
	}
	s.State = StatePlaying
	s.adoptionLeader = ""
	// Desync = worst pairwise skew across the participating homes (max - min).
	return []Effect{EffLogDesync{DeltaMS: latest - earliest}, EffPersist{}}
}

// OnEnded implements: ended from both, or ended from one while the other's
// position is within 1 s of the end (spec 7.2).
func (s *Session) OnEnded(node protocol.NodeID, elementID string, reason string) []Effect {
	if s.State != StatePlaying || !s.isCurrent(elementID) {
		return nil
	}
	s.ended[node] = true
	if s.endedConditionMet() {
		return s.finishCurrent("eof")
	}
	return nil
}

// endedConditionMet: every peer ended, or at least one ended while every
// laggard's position is within 1 s of the end (spec 7.2, N-wise).
func (s *Session) endedConditionMet() bool {
	dur := s.Current.DurationMS
	anyEnded := false
	for _, n := range s.Peers {
		if !s.counts(n) {
			continue // offline home under living air is not on the barrier
		}
		if s.ended[n] {
			anyEnded = true
			continue
		}
		if dur <= 0 || s.nodePos[n] < dur-1000 {
			return false
		}
	}
	return anyEnded
}

func (s *Session) finishCurrent(status string) []Effect {
	done := EffElementDone{Element: *s.Current, Status: status}
	effs := s.advance()
	return append([]Effect{done}, effs...)
}

// OnHeartbeat updates node telemetry; may complete the ended condition.
func (s *Session) OnHeartbeat(node protocol.NodeID, positionMS int64, rttMS int64) []Effect {
	return s.OnHeartbeatAt(0, node, positionMS, rttMS)
}

// OnHeartbeatAt records when the position was observed so catch-up starts can
// extrapolate through the heartbeat interval.
func (s *Session) OnHeartbeatAt(nowMS int64, node protocol.NodeID, positionMS int64, rttMS int64) []Effect {
	s.nodePos[node] = positionMS
	if nowMS > 0 {
		s.nodePosAt[node] = nowMS
	}
	if rttMS > 0 {
		s.nodeRTT[node] = rttMS
	}
	if s.State == StatePlaying && s.Current != nil && len(s.ended) > 0 && s.endedConditionMet() {
		return s.finishCurrent("eof")
	}
	return nil
}

func (s *Session) OnVoiceEnded(node protocol.NodeID, elementID string) []Effect {
	return s.onVoicePartDone(node, elementID)
}

func (s *Session) OnWaitEnded(node protocol.NodeID, elementID string) []Effect {
	return s.onVoicePartDone(node, elementID)
}

func (s *Session) onVoicePartDone(node protocol.NodeID, elementID string) []Effect {
	if s.State != StateVoice || !s.isCurrent(elementID) {
		return nil
	}
	s.voiceDone[node] = true
	if !s.voiceCompleteAcrossPeers() {
		return nil
	}
	return s.finishCurrent("eof")
}

func (s *Session) voiceCompleteAcrossPeers() bool {
	for _, n := range s.Peers {
		if !s.counts(n) {
			continue // offline home under living air is not on the barrier
		}
		if !s.voiceDone[n] {
			return false
		}
	}
	return true
}

// OnReadyTimeout: one retry, then skip with a chat message (spec 7.2).
func (s *Session) OnReadyTimeout(elementID string) []Effect {
	return s.OnReadyTimeoutAt(0, elementID)
}

func (s *Session) OnReadyTimeoutAt(nowMS int64, elementID string) []Effect {
	if s.State != StateLoading || !s.isCurrent(elementID) {
		return nil
	}
	if !s.retried {
		s.retried = true
		effs := []Effect{EffPersist{}}
		for _, n := range s.Peers {
			// Only participants are on the ready barrier: re-loading an offline
			// non-participant (or a solo-managed joiner) is dead weight and, on
			// the drop-during-loading path, used to burn the retry on a home
			// that could never answer (H2 side-fix).
			if !s.counts(n) || s.ready[n] {
				continue
			}
			effs = append(effs, EffLoad{To: n, ElementID: s.Current.ID, URI: s.Current.URI, PositionMS: s.SavedPositionMS})
		}
		effs = append(effs, EffArmReadyTimer{ElementID: s.Current.ID})
		return effs
	}
	if s.adoptionLeader != "" {
		var missing []string
		for _, n := range s.Peers {
			if !s.counts(n) || s.ready[n] {
				continue
			}
			missing = append(missing, string(n))
			delete(s.participants, n)
			s.joining[n] = true // a late ready catches up without stopping leaders
		}
		effs := []Effect{EffNotify{Text: fmt.Sprintf(
			"дом %s не успел подключиться к треку — эфир продолжается, он догонит позже",
			strings.Join(missing, ", "),
		)}}
		return append(effs, s.checkAllReady(nowMS, elementID)...)
	}
	title := s.Current.Title
	if title == "" {
		title = s.Current.URI
	}
	done := EffElementDone{Element: *s.Current, Status: "error"}
	notify := EffNotify{Text: fmt.Sprintf("не смог загрузить %s за отведённое время, пропускаю", htmlEscape(title))}
	effs := s.advance()
	return append([]Effect{done, notify}, effs...)
}

// OnNodeError handles error messages scoped to the current element (spec 4.4).
func (s *Session) OnNodeError(node protocol.NodeID, code string, elementID string) []Effect {
	return s.OnNodeErrorAt(0, node, code, elementID)
}

func (s *Session) OnNodeErrorAt(nowMS int64, node protocol.NodeID, code string, elementID string) []Effect {
	if !s.isCurrent(elementID) && elementID != "" {
		return nil
	}
	switch code {
	case "load_failed", "track_unavailable":
		if s.State != StateLoading && s.State != StateArmed {
			return nil
		}
		if s.adoptionLeader != "" && node != s.adoptionLeader {
			if s.participants != nil {
				delete(s.participants, node)
			}
			delete(s.ready, node)
			delete(s.started, node)
			s.joining[node] = true
			position := s.livePositionAt(nowMS)
			effs := []Effect{
				EffNotify{Text: fmt.Sprintf(
					"дом %s пока не смог подключиться к выбранному треку — остальные продолжают, он попробует догнать",
					node,
				)},
				EffLoad{To: node, ElementID: s.Current.ID, URI: s.Current.URI, PositionMS: position},
			}
			if s.State == StateLoading {
				return append(effs, s.checkAllReady(nowMS, s.Current.ID)...)
			}
			if more := s.checkAllStarted(); more != nil {
				return append(effs, more...)
			}
			return append(effs, EffPersist{})
		}
		title := s.Current.Title
		if title == "" {
			title = s.Current.URI
		}
		done := EffElementDone{Element: *s.Current, Status: "error"}
		notify := EffNotify{Text: fmt.Sprintf("трек %s недоступен на аккаунте %s, пропускаю", htmlEscape(title), node)}
		pause := s.pauseBoth(0)
		effs := s.advance()
		return append(append([]Effect{done, notify, EffCancelReadyTimer{}}, pause...), effs...)
	case "librespot_restart":
		// A single daemon restart in a linked living air is a catch-up event,
		// not a reason to pause and reload every healthy home. The failed node
		// leaves the current barriers and rejoins after its daemon is ready.
		if s.GateMode == GateEachSide && node != s.adoptionLeader &&
			(s.State == StatePlaying || s.State == StateArmed) &&
			s.Current != nil && s.Current.Kind == KindTrack {
			if s.participants == nil {
				s.participants = map[protocol.NodeID]bool{}
				for _, n := range s.Peers {
					if s.online[n] {
						s.participants[n] = true
					}
				}
			}
			delete(s.participants, node)
			delete(s.ready, node)
			delete(s.started, node)
			delete(s.ended, node)
			s.joining[node] = true
			position := s.livePositionAt(nowMS)
			effs := []Effect{
				EffNotify{Text: fmt.Sprintf("плеер дома %s перезапустился — остальные продолжают, этот дом догонит", node)},
				EffLoad{To: node, ElementID: s.Current.ID, URI: s.Current.URI, PositionMS: position},
				EffPersist{},
			}
			if more := s.checkAllStarted(); more != nil {
				effs = append(effs, more...)
			}
			return effs
		}
		// Spec 6.6: coordinator restarts the current element via the normal cycle.
		if s.State == StatePlaying || s.State == StateArmed {
			pos := s.currentPositionEstimate()
			effs := []Effect{EffNotify{Text: fmt.Sprintf("плеер дома %s перезапустился, рестартую трек", node)}}
			s.resetElementTracking()
			return append(effs, s.loadCurrent(pos)...)
		}
	}
	return nil
}

func (s *Session) currentPositionEstimate() int64 {
	// Safest: the minimum of known heartbeat positions across peers
	// (a little repeat beats a skip). Peers that never reported are skipped.
	first := true
	var best int64
	for _, n := range s.Peers {
		pos, ok := s.nodePos[n]
		if !ok {
			continue
		}
		if first || pos < best {
			best = pos
			first = false
		}
	}
	return best
}

func (s *Session) pauseBoth(fadeMS int64) []Effect {
	if s.Current == nil {
		return nil
	}
	var effs []Effect
	for _, n := range s.Peers {
		effs = append(effs, EffPause{To: n, ElementID: s.Current.ID, FadeMS: fadeMS})
	}
	return effs
}

// --- User commands (bot) ---

func (s *Session) CmdPause() []Effect {
	// Spec table covers PLAYING; LOADING/ARMED pause is honored too to avoid
	// racing a start the user is trying to stop.
	switch s.State {
	case StatePlaying, StateArmed, StateLoading:
		s.SavedPositionMS = s.currentPositionEstimate()
		s.State = StatePaused
		return append(append([]Effect{EffCancelReadyTimer{}}, s.pauseBoth(0)...), EffPersist{})
	}
	return nil
}

func (s *Session) CmdResume() []Effect {
	if s.State != StatePaused || s.Current == nil {
		if s.State == StateIdle && len(s.Queue) > 0 {
			return s.advance()
		}
		return nil
	}
	if s.Current.Kind == KindVoice {
		// A voice insert interrupted by pause/degradation restarts from scratch
		// (seconds long, position tracking not worth it).
		s.resetElementTracking()
		return s.startVoice()
	}
	s.resetElementTracking()
	return s.loadCurrent(s.SavedPositionMS)
}

func (s *Session) CmdSkip() []Effect {
	if s.State != StatePlaying && s.State != StatePaused && s.State != StateVoice &&
		s.State != StateLoading && s.State != StateArmed {
		return nil
	}
	if s.Current == nil { // guard: a paused/idle snapshot can have no element
		return nil
	}
	done := EffElementDone{Element: *s.Current, Status: "skipped"}
	pause := s.pauseBoth(300)
	effs := s.advance()
	return append(append([]Effect{done, EffCancelReadyTimer{}}, pause...), effs...)
}

// RefreshSavedPosition re-estimates the resume position from live heartbeats.
// Used after coordinator restart: spec 7.2 says the position comes from the
// first node heartbeats, not from the stale persisted value.
func (s *Session) RefreshSavedPosition() {
	if s.State == StatePaused && s.Current != nil && s.Current.Kind == KindTrack {
		if p := s.currentPositionEstimate(); p > 0 {
			s.SavedPositionMS = p
		}
	}
}

// QueueLen exposes queue size for user-facing replies.
func (s *Session) QueueLen() int { return len(s.Queue) }

// CmdSync force-restarts the current track from the estimated live position
// (spec 9.1 /sync: resynchronization repair).
func (s *Session) CmdSync() []Effect {
	if s.State != StatePlaying || s.Current == nil || s.Current.Kind != KindTrack {
		return nil
	}
	pos := s.currentPositionEstimate()
	pause := s.pauseBoth(0)
	s.resetElementTracking()
	return append(pause, s.loadCurrent(pos)...)
}

// allPausedLocally: every peer sits on a personal pause.
func (s *Session) allPausedLocally() bool {
	if len(s.Peers) == 0 {
		return false
	}
	for _, n := range s.Peers {
		if !s.pausedLocally[n] {
			return false
		}
	}
	return true
}

// OnUserPause detaches ONE home from the shared air after its user paused
// this Pulsar in the Spotify app (personal pause, 2026-07-10): the broadcast
// keeps playing for the others; the home is excluded from the current
// element's barriers and from subsequent elements until OnUserResume. A pause
// that would leave no active home degrades to the ordinary global pause —
// the last listener stopping IS the air stopping.
func (s *Session) OnUserPause(nowMS int64, node protocol.NodeID) []Effect {
	if s.Mode != ModeShared || !s.hasPeer(node) || s.pausedLocally[node] {
		return nil
	}
	switch s.State {
	case StateLoading, StateArmed, StatePlaying:
	default:
		// Idle/paused/degraded airs have nothing personal to detach from, and
		// VOICE is unreachable here (the node only reports a user pause while
		// its MUSIC pipeline was playing).
		return nil
	}
	active := 0
	for _, n := range s.Peers {
		if n == node || s.pausedLocally[n] || !s.counts(n) {
			continue
		}
		active++
	}
	if active == 0 {
		effs := []Effect{EffNotify{Text: fmt.Sprintf("дом %s поставил эфир на паузу", node)}}
		return append(effs, s.CmdPause()...)
	}
	s.pausedLocally[node] = true
	if s.participants == nil {
		// Strict sessions run with nil participants (= everyone); materialize
		// so the machinery can exclude just this home.
		s.participants = map[protocol.NodeID]bool{}
		for _, n := range s.Peers {
			s.participants[n] = true
		}
	}
	delete(s.participants, node)
	delete(s.joining, node)
	effs := []Effect{EffNotify{Text: fmt.Sprintf("дом %s на личной паузе — эфир продолжается (play в Spotify вернёт в эфир)", node)}, EffPersist{}}
	// The paused home may have been the LAST one a barrier was waiting for —
	// the same re-checks as an offline drop (the H1/H2 stall class).
	switch s.State {
	case StateLoading:
		if s.Current != nil {
			if more := s.checkAllReady(nowMS, s.Current.ID); more != nil {
				return append(effs, more...)
			}
		}
	case StateArmed:
		if more := s.checkAllStarted(); more != nil {
			return append(effs, more...)
		}
	case StatePlaying:
		if s.Current != nil && len(s.ended) > 0 && s.endedConditionMet() {
			return append(effs, s.finishCurrent("eof")...)
		}
	}
	return effs
}

// OnUserResume returns a personally-paused home to the air: a living-air
// catch-up into the playing element, a solo load onto the loading barrier, a
// fresh deal when the air idled behind the pause — or just clearing the flag.
func (s *Session) OnUserResume(node protocol.NodeID) []Effect {
	if !s.pausedLocally[node] {
		return nil
	}
	delete(s.pausedLocally, node)
	switch s.State {
	case StatePlaying, StateArmed:
		if effs := s.JoinInProgress(node); effs != nil {
			return append(effs, EffPersist{})
		}
	case StateLoading:
		if s.Current != nil && s.Current.Kind == KindTrack {
			if s.participants != nil {
				s.participants[node] = true
			}
			delete(s.ready, node)
			return []Effect{
				EffLoad{To: node, ElementID: s.Current.ID, URI: s.Current.URI, PositionMS: s.SavedPositionMS},
				EffPersist{},
			}
		}
	case StateIdle:
		if s.hasUpcoming() {
			return s.advance()
		}
	}
	return []Effect{EffPersist{}}
}

// --- Degradation (spec 4.4, 7.2) ---

func (s *Session) OnNodeOffline(nowMS int64, node protocol.NodeID) []Effect {
	s.online[node] = false
	if s.Mode != ModeShared {
		return []Effect{EffNotify{Text: fmt.Sprintf("дом %s офлайн", node)}}
	}
	if s.State == StateIdle || s.State == StateDegraded {
		return nil
	}
	// Living air (§12 L1.5): an offline home must NOT freeze the air while the
	// gate still holds (each linked side still has a home online). Drop it from
	// the current element's participant barriers so ready/started/ended/voice
	// don't wait for it; the survivors keep playing and it catches up
	// (JoinInProgress) when it returns. Only a whole dark side parks the air.
	// Going offline ends a personal pause: the node loses its local flag on
	// restart anyway, so the coordinator forgetting too keeps both sides
	// symmetric — the standard catch-up resumes the home when it returns.
	delete(s.pausedLocally, node)
	if s.GateMode == GateEachSide {
		if s.participants != nil {
			delete(s.participants, node)
		}
		delete(s.joining, node) // a dropped catch-up re-joins on its next return
		if s.gateSatisfied() {
			effs := []Effect{EffNotify{Text: fmt.Sprintf("дом %s пропал — эфир продолжается, догонит по возвращении", node)}}
			// The dropped home may have been the LAST one a barrier was waiting
			// for — every state's barrier must be re-checked here, or the air
			// stalls: LOADING waits out 2x ready-timeout then skips a track the
			// survivors had ready (H2), ARMED hangs forever (H1), PLAYING/VOICE
			// never finish.
			switch s.State {
			case StateLoading:
				if s.Current != nil {
					if more := s.checkAllReady(nowMS, s.Current.ID); more != nil {
						return append(effs, more...)
					}
				}
			case StateArmed:
				if more := s.checkAllStarted(); more != nil {
					return append(effs, more...)
				}
			case StatePlaying:
				if s.Current != nil && s.endedConditionMet() {
					return append(effs, s.finishCurrent("eof")...)
				}
			case StateVoice:
				if s.Current != nil && s.voiceCompleteAcrossPeers() {
					return append(effs, s.finishCurrent("eof")...)
				}
			}
			return append(effs, EffPersist{})
		}
		// gate no longer satisfied (a whole side is dark) — fall through to park.
	}
	// Freeze the air at the position of the survivors (minimum across them).
	s.State = StateDegraded
	first := true
	for _, n := range s.peersExcept(node) {
		pos, ok := s.nodePos[n]
		if !ok {
			continue
		}
		if first || pos < s.SavedPositionMS {
			s.SavedPositionMS = pos
			first = false
		}
	}
	effs := []Effect{EffCancelReadyTimer{}}
	if s.Current != nil {
		for _, n := range s.peersExcept(node) {
			effs = append(effs, EffPause{To: n, ElementID: s.Current.ID, FadeMS: 0})
		}
	}
	effs = append(effs, EffNotify{Text: fmt.Sprintf("дом %s пропал из сети, ставлю эфир на паузу", node)}, EffPersist{})
	return effs
}

func (s *Session) OnNodeBack(node protocol.NodeID) []Effect {
	wasOffline := !s.online[node]
	s.online[node] = true
	if s.Mode != ModeShared || s.State != StateDegraded || !wasOffline {
		return nil
	}
	// Recovery is gate-aware: strict needs everyone, living air needs each side
	// to have a home online again (gateSatisfied covers both modes).
	if !s.gateSatisfied() {
		return nil
	}
	// A broadcast interrupted mid-element waits for a human /resume
	// (spec 7.2); one that never started (offline gate) starts itself.
	if s.Current == nil && s.hasUpcoming() {
		effs := []Effect{EffNotify{Text: "все дома в сети — поехали"}}
		return append(effs, s.advance()...)
	}
	if s.Current == nil {
		s.State = StateIdle
		return []Effect{EffPersist{}}
	}
	s.State = StatePaused
	return []Effect{
		EffNotify{Text: fmt.Sprintf("дом %s снова в сети. /resume чтобы продолжить", node)},
		EffPersist{},
	}
}

// --- Mode switching (spec 4.3) ---

func (s *Session) SetModeSolo() []Effect {
	if s.Mode == ModeSolo {
		return nil
	}
	s.Mode = ModeSolo
	s.pausedLocally = map[protocol.NodeID]bool{}
	if s.Current != nil && (s.State == StatePlaying || s.State == StateArmed || s.State == StateLoading) {
		s.SavedPositionMS = s.currentPositionEstimate()
	}
	s.State = StateIdle
	effs := []Effect{EffCancelReadyTimer{}}
	for _, n := range s.Peers {
		effs = append(effs, EffStop{To: n})
		effs = append(effs, EffSetMode{To: n, Mode: ModeSolo})
	}
	return append(effs, EffNotify{Text: "апоастрон: орбиты расходятся, каждый слушает своё (/inject подкидывает партнёру)"}, EffPersist{})
}

func (s *Session) SetModeShared() []Effect {
	if s.Mode == ModeShared {
		return nil
	}
	s.Mode = ModeShared
	s.pausedLocally = map[protocol.NodeID]bool{}
	effs := []Effect{}
	for _, n := range s.Peers {
		effs = append(effs, EffStop{To: n}, EffSetMode{To: n, Mode: ModeShared})
	}
	if s.Current != nil {
		// Continue the interrupted element from its saved position.
		if s.Current.Kind == KindVoice {
			s.resetElementTracking()
			return append(effs, s.startVoice()...)
		}
		s.resetElementTracking()
		return append(effs, s.loadCurrent(s.SavedPositionMS)...)
	}
	if len(s.Queue) > 0 {
		return append(effs, s.advance()...)
	}
	s.State = StateIdle
	return append(effs, EffNotify{Text: "периастрон: эфир общий. Выбери Пульсар в Spotify и включи трек"}, EffPersist{})
}

// Snapshot builds the welcome payload for a (re)connecting node (spec 8.3).
func (s *Session) Snapshot(volume int) protocol.SessionSnapshot {
	snap := protocol.SessionSnapshot{
		Mode:   string(s.Mode),
		State:  string(s.State),
		Volume: volume,
	}
	if s.Current != nil {
		cur := &protocol.SessionCurrent{
			ElementID:  s.Current.ID,
			Kind:       string(s.Current.Kind),
			PositionMS: s.SavedPositionMS,
		}
		if s.Current.Kind == KindTrack {
			cur.URI = s.Current.URI
		}
		snap.Current = cur
	}
	return snap
}
