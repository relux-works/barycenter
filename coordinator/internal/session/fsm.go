// Package session implements the coordinator session state machine
// (spec 7.2) as a pure core: events in, effects out, no I/O, no clocks.
// The caller (main loop) owns timers, transport and persistence.
package session

import (
	"fmt"

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

// Element is the unit of airtime (spec 5.1).
type Element struct {
	ID          string
	Kind        Kind
	URI         string // kind=track
	MediaID     string // kind=voice
	Title       string
	DurationMS  int64
	RequestedBy protocol.NodeID
	Target      string // "both" | "a" | "b"
	CreatedAt   int64
}

var bothNodes = []protocol.NodeID{protocol.NodeA, protocol.NodeB}

func otherNode(n protocol.NodeID) protocol.NodeID {
	if n == protocol.NodeA {
		return protocol.NodeB
	}
	return protocol.NodeA
}

// --- Effects: what the caller must do after an event ---

type Effect any

type (
	EffLoad struct {
		To         protocol.NodeID
		ElementID  string
		URI        string
		PositionMS int64
	}
	EffResumeAt struct {
		To        protocol.NodeID
		ElementID string
		TCoordMS  int64
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
	nodeRTT map[protocol.NodeID]int64

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
		nodeRTT:       map[protocol.NodeID]int64{},
		StartMarginMS: 500,
	}
}

func (s *Session) targetNodes(el *Element) []protocol.NodeID {
	switch el.Target {
	case "a":
		return []protocol.NodeID{protocol.NodeA}
	case "b":
		return []protocol.NodeID{protocol.NodeB}
	default:
		return bothNodes
	}
}

func (s *Session) resetElementTracking() {
	s.ready = map[protocol.NodeID]bool{}
	s.started = map[protocol.NodeID]int64{}
	s.ended = map[protocol.NodeID]bool{}
	s.voiceDone = map[protocol.NodeID]bool{}
	s.retried = false
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

func (s *Session) bothOnline() bool {
	return s.online[protocol.NodeA] && s.online[protocol.NodeB]
}

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
// Spec 7.2: the broadcast never starts with a home missing — with material
// queued but a node offline we park in DEGRADED and resume automatically
// when both homes are back.
func (s *Session) advance() []Effect {
	s.resetElementTracking()
	s.SavedPositionMS = 0
	if s.hasUpcoming() && !s.bothOnline() {
		s.Current = nil
		s.State = StateDegraded
		missing := ""
		for _, n := range bothNodes {
			if !s.online[n] {
				missing += " " + string(n)
			}
		}
		return []Effect{
			EffNotify{Text: fmt.Sprintf("эфир подождёт: дом%s не в сети — продолжу, как только оба дома подключатся", missing)},
			EffPersist{},
		}
	}
	if len(s.Queue) > 0 {
		el := s.Queue[0]
		s.Queue = s.Queue[1:]
		s.Current = &el
		if el.Kind == KindTrack {
			return s.loadCurrent(0)
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

func (s *Session) loadCurrent(positionMS int64) []Effect {
	s.State = StateLoading
	s.SavedPositionMS = positionMS
	effs := []Effect{EffPersist{}}
	for _, n := range bothNodes {
		effs = append(effs, EffLoad{To: n, ElementID: s.Current.ID, URI: s.Current.URI, PositionMS: positionMS})
	}
	effs = append(effs, EffArmReadyTimer{ElementID: s.Current.ID})
	return effs
}

func (s *Session) startVoice() []Effect {
	s.State = StateVoice
	el := s.Current
	effs := []Effect{EffPersist{}}
	targets := s.targetNodes(el)
	targetSet := map[protocol.NodeID]bool{}
	for _, n := range targets {
		targetSet[n] = true
		effs = append(effs, EffPlayVoice{To: n, ElementID: el.ID, MediaID: el.MediaID})
	}
	for _, n := range bothNodes {
		if !targetSet[n] {
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
	if s.State != StateLoading || !s.isCurrent(elementID) {
		return nil // idempotency (spec 7.2)
	}
	s.ready[node] = true
	for _, n := range bothNodes {
		if !s.ready[n] {
			return nil
		}
	}
	s.State = StateArmed
	maxRTT := max(s.nodeRTT[protocol.NodeA], s.nodeRTT[protocol.NodeB])
	t := nowMS + 2*maxRTT + s.StartMarginMS
	effs := []Effect{EffCancelReadyTimer{}, EffPersist{}}
	for _, n := range bothNodes {
		effs = append(effs, EffResumeAt{To: n, ElementID: elementID, TCoordMS: t})
	}
	return effs
}

func (s *Session) OnStarted(node protocol.NodeID, elementID string, tFirstSampleMS int64) []Effect {
	if s.State != StateArmed || !s.isCurrent(elementID) {
		return nil
	}
	s.started[node] = tFirstSampleMS
	a, okA := s.started[protocol.NodeA]
	b, okB := s.started[protocol.NodeB]
	if !okA || !okB {
		return nil
	}
	s.State = StatePlaying
	delta := a - b
	if delta < 0 {
		delta = -delta
	}
	return []Effect{EffLogDesync{DeltaMS: delta}, EffPersist{}}
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

func (s *Session) endedConditionMet() bool {
	a, b := s.ended[protocol.NodeA], s.ended[protocol.NodeB]
	if a && b {
		return true
	}
	dur := s.Current.DurationMS
	nearEnd := func(n protocol.NodeID) bool {
		return dur > 0 && s.nodePos[n] >= dur-1000
	}
	return (a && nearEnd(protocol.NodeB)) || (b && nearEnd(protocol.NodeA))
}

func (s *Session) finishCurrent(status string) []Effect {
	done := EffElementDone{Element: *s.Current, Status: status}
	effs := s.advance()
	return append([]Effect{done}, effs...)
}

// OnHeartbeat updates node telemetry; may complete the ended condition.
func (s *Session) OnHeartbeat(node protocol.NodeID, positionMS int64, rttMS int64) []Effect {
	s.nodePos[node] = positionMS
	if rttMS > 0 {
		s.nodeRTT[node] = rttMS
	}
	if s.State == StatePlaying && s.Current != nil && (s.ended[protocol.NodeA] || s.ended[protocol.NodeB]) && s.endedConditionMet() {
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
	for _, n := range bothNodes {
		if !s.voiceDone[n] {
			return nil
		}
	}
	return s.finishCurrent("eof")
}

// OnReadyTimeout: one retry, then skip with a chat message (spec 7.2).
func (s *Session) OnReadyTimeout(elementID string) []Effect {
	if s.State != StateLoading || !s.isCurrent(elementID) {
		return nil
	}
	if !s.retried {
		s.retried = true
		effs := []Effect{EffPersist{}}
		for _, n := range bothNodes {
			if !s.ready[n] {
				effs = append(effs, EffLoad{To: n, ElementID: s.Current.ID, URI: s.Current.URI, PositionMS: s.SavedPositionMS})
			}
		}
		effs = append(effs, EffArmReadyTimer{ElementID: s.Current.ID})
		return effs
	}
	title := s.Current.Title
	if title == "" {
		title = s.Current.URI
	}
	done := EffElementDone{Element: *s.Current, Status: "error"}
	notify := EffNotify{Text: fmt.Sprintf("не смог загрузить %s за отведённое время, пропускаю", title)}
	effs := s.advance()
	return append([]Effect{done, notify}, effs...)
}

// OnNodeError handles error messages scoped to the current element (spec 4.4).
func (s *Session) OnNodeError(node protocol.NodeID, code string, elementID string) []Effect {
	if !s.isCurrent(elementID) && elementID != "" {
		return nil
	}
	switch code {
	case "load_failed", "track_unavailable":
		if s.State != StateLoading && s.State != StateArmed {
			return nil
		}
		title := s.Current.Title
		if title == "" {
			title = s.Current.URI
		}
		done := EffElementDone{Element: *s.Current, Status: "error"}
		notify := EffNotify{Text: fmt.Sprintf("трек %s недоступен на аккаунте %s, пропускаю", title, node)}
		pause := s.pauseBoth(0)
		effs := s.advance()
		return append(append([]Effect{done, notify, EffCancelReadyTimer{}}, pause...), effs...)
	case "librespot_restart":
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
	// Safest: the minimum of last heartbeat positions (a little repeat beats a skip).
	a, b := s.nodePos[protocol.NodeA], s.nodePos[protocol.NodeB]
	if a < b {
		return a
	}
	return b
}

func (s *Session) pauseBoth(fadeMS int64) []Effect {
	if s.Current == nil {
		return nil
	}
	var effs []Effect
	for _, n := range bothNodes {
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

func (s *Session) CmdPlayNow(el Element) []Effect {
	if s.Mode != ModeShared {
		return nil
	}
	var pre []Effect
	if s.Current != nil {
		pre = append(pre, EffElementDone{Element: *s.Current, Status: "skipped"})
		pre = append(pre, s.pauseBoth(300)...) // spec 7.3: fade 300 ms
		pre = append(pre, EffCancelReadyTimer{})
	}
	s.Queue = append([]Element{el}, s.Queue...)
	return append(pre, s.advance()...)
}

// --- Degradation (spec 4.4, 7.2) ---

func (s *Session) OnNodeOffline(node protocol.NodeID) []Effect {
	s.online[node] = false
	if s.Mode != ModeShared {
		return []Effect{EffNotify{Text: fmt.Sprintf("дом %s офлайн", node)}}
	}
	if s.State == StateIdle || s.State == StateDegraded {
		return nil
	}
	s.SavedPositionMS = s.nodePos[otherNode(node)]
	s.State = StateDegraded
	effs := []Effect{EffCancelReadyTimer{}}
	if s.Current != nil {
		effs = append(effs, EffPause{To: otherNode(node), ElementID: s.Current.ID, FadeMS: 0})
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
	// Both online again?
	for _, n := range bothNodes {
		if !s.online[n] {
			return nil
		}
	}
	// A broadcast interrupted mid-element waits for a human /resume
	// (spec 7.2); one that never started (offline gate) starts itself.
	if s.Current == nil && s.hasUpcoming() {
		effs := []Effect{EffNotify{Text: "оба дома в сети — поехали"}}
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
	if s.Current != nil && (s.State == StatePlaying || s.State == StateArmed || s.State == StateLoading) {
		s.SavedPositionMS = s.currentPositionEstimate()
	}
	s.State = StateIdle
	effs := []Effect{EffCancelReadyTimer{}}
	for _, n := range bothNodes {
		effs = append(effs, EffStop{To: n}, EffSetMode{To: n, Mode: ModeSolo})
	}
	return append(effs, EffNotify{Text: "апоастрон: орбиты расходятся, каждый слушает своё (/inject подкидывает партнёру)"}, EffPersist{})
}

func (s *Session) SetModeShared() []Effect {
	if s.Mode == ModeShared {
		return nil
	}
	s.Mode = ModeShared
	effs := []Effect{}
	for _, n := range bothNodes {
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
	return append(effs, EffNotify{Text: "периастрон: дома в сближении, эфир общий. Кидай ссылку — начнём"}, EffPersist{})
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
