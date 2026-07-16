package session

import (
	"errors"
	"fmt"
	"sort"
	"sync"

	"relux.works/duet/coordinator/internal/protocol"
)

var (
	ErrLivePTTBusy         = errors.New("live PTT playback domain is busy")
	ErrLivePTTStale        = errors.New("live PTT session is stale")
	ErrLivePTTUnauthorized = errors.New("live PTT sender or target is unauthorized")
	ErrLivePTTNotReady     = errors.New("live PTT session has no accepted targets")
)

const (
	livePTTMaxSessions      = 256
	livePTTRateBurstFrames  = 8
	livePTTRateFramesPerSec = protocol.LivePTTMaxFramesPerSecond
	livePTTMaxTargetEvents  = 32
)

type LivePTTNode struct {
	OrbitID int64
	Slot    protocol.NodeID
}

type LivePTTTarget struct {
	Node    LivePTTNode
	ActorID int64
}

type LivePTTStart struct {
	Sender        LivePTTNode
	SenderActorID int64
	DomainKind    string
	DomainID      int64
	Payload       protocol.LivePTTStartPayload
	Targets       []LivePTTTarget
	NowMS         int64
}

type LivePTTEffectKind string

const (
	LivePTTSendSignal LivePTTEffectKind = "send_signal"
	LivePTTSendBinary LivePTTEffectKind = "send_binary"
	LivePTTDuckStart  LivePTTEffectKind = "duck_start"
	LivePTTDuckEnd    LivePTTEffectKind = "duck_end"
)

type LivePTTEffect struct {
	Kind      LivePTTEffectKind
	To        LivePTTNode
	Type      string
	Payload   any
	Binary    []byte
	SessionID string
}

type livePTTTargetState struct {
	target       LivePTTTarget
	state        string
	lastEventSeq int64
}

type livePTTSession struct {
	sender       LivePTTNode
	senderActor  int64
	domainKey    string
	payload      protocol.LivePTTStartPayload
	guard        protocol.LivePTTFrameGuard
	targets      map[LivePTTNode]*livePTTTargetState
	startedAtMS  int64
	deadlineMS   int64
	duckStarted  bool
	lastEventSeq int64
	rateMilli    int64
	rateAtMS     int64
}

type LivePTTMetrics struct {
	StartsTotal             uint64
	RejectedStartsTotal     uint64
	FramesRelayedTotal      uint64
	DuplicateFramesTotal    uint64
	StaleFramesTotal        uint64
	InvalidFramesTotal      uint64
	TargetBackpressureTotal uint64
	TargetPolicyDropsTotal  uint64
	SessionsEndedTotal      uint64
	WatchdogCancellations   uint64
	ActiveSessions          int
	RetainedAudioBytes      int
	PersistedAudioBytes     int
}

type LivePTTBinding struct {
	SessionID  string
	DomainKind string
	DomainID   int64
	Sender     LivePTTNode
	Targets    []LivePTTNode
}

// LivePTTRuntime owns only ephemeral metadata. It never retains an accepted
// audio payload after RelayFrame returns and has no storage dependency.
type LivePTTRuntime struct {
	mu             sync.Mutex
	byDomain       map[string]*livePTTSession
	bySession      map[string]*livePTTSession
	lastGeneration map[string]int64
	metrics        LivePTTMetrics
}

func NewLivePTTRuntime() *LivePTTRuntime {
	return &LivePTTRuntime{
		byDomain: map[string]*livePTTSession{}, bySession: map[string]*livePTTSession{},
		lastGeneration: map[string]int64{},
	}
}

func livePTTDomainKey(kind string, id int64) string { return fmt.Sprintf("%s/%d", kind, id) }

func (r *LivePTTRuntime) Start(request LivePTTStart) ([]LivePTTEffect, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if err := protocol.ValidateLivePTTStartPayload(request.Payload); err != nil ||
		request.NowMS <= 0 || request.Sender.OrbitID <= 0 || request.Sender.Slot == "" ||
		request.SenderActorID <= 0 || request.Payload.SenderActorID != request.SenderActorID ||
		request.Payload.SenderOrbitID != request.Sender.OrbitID ||
		request.Payload.SenderNodeID != string(request.Sender.Slot) ||
		request.Payload.PlaybackDomain != request.DomainKind ||
		request.Payload.PlaybackDomainID != request.DomainID || len(request.Targets) == 0 ||
		len(request.Targets) > protocol.LivePTTMaxTargets {
		r.metrics.RejectedStartsTotal++
		return nil, ErrLivePTTUnauthorized
	}
	if len(r.bySession) >= livePTTMaxSessions {
		r.metrics.RejectedStartsTotal++
		return nil, ErrLivePTTBusy
	}
	key := livePTTDomainKey(request.DomainKind, request.DomainID)
	if r.byDomain[key] != nil {
		r.metrics.RejectedStartsTotal++
		return nil, ErrLivePTTBusy
	}
	if request.Payload.Generation <= r.lastGeneration[key] || r.bySession[request.Payload.SessionID] != nil {
		r.metrics.RejectedStartsTotal++
		return nil, ErrLivePTTStale
	}
	sessionID, err := protocol.ParseLivePTTSessionID(request.Payload.SessionID)
	if err != nil {
		r.metrics.RejectedStartsTotal++
		return nil, ErrLivePTTStale
	}
	targets := make(map[LivePTTNode]*livePTTTargetState, len(request.Targets))
	for _, target := range request.Targets {
		if target.Node.OrbitID <= 0 || target.Node.Slot == "" || target.ActorID <= 0 || target.Node == request.Sender {
			r.metrics.RejectedStartsTotal++
			return nil, ErrLivePTTUnauthorized
		}
		if _, duplicate := targets[target.Node]; duplicate {
			r.metrics.RejectedStartsTotal++
			return nil, ErrLivePTTUnauthorized
		}
		targets[target.Node] = &livePTTTargetState{target: target, state: "pending"}
	}
	s := &livePTTSession{sender: request.Sender, senderActor: request.SenderActorID,
		domainKey: key, payload: request.Payload,
		guard:   protocol.NewLivePTTFrameGuard(sessionID, request.Payload.Generation),
		targets: targets, startedAtMS: request.NowMS,
		deadlineMS: request.Payload.AcceptDeadlineCoordMS, lastEventSeq: 1}
	s.rateMilli = livePTTRateBurstFrames * 1000
	s.rateAtMS = request.NowMS
	r.byDomain[key], r.bySession[request.Payload.SessionID] = s, s
	r.lastGeneration[key] = request.Payload.Generation
	r.metrics.StartsTotal++
	effects := make([]LivePTTEffect, 0, len(targets)+1)
	for _, node := range sortedLivePTTNodes(targets) {
		effects = append(effects, LivePTTEffect{Kind: LivePTTSendSignal, To: node,
			Type: protocol.TypeLivePTTStart, Payload: &s.payload, SessionID: s.payload.SessionID})
	}
	state := protocol.LivePTTStatePayload{Revision: 1, Phase: "accepting",
		ActiveSessionID: s.payload.SessionID, Generation: s.payload.Generation,
		SpeakerActorID: request.SenderActorID, GeneratedAtCoordMS: request.NowMS}
	effects = append(effects, LivePTTEffect{Kind: LivePTTSendSignal, To: s.sender,
		Type: protocol.TypeLivePTTState, Payload: &state, SessionID: s.payload.SessionID})
	return effects, nil
}

func sortedLivePTTNodes(targets map[LivePTTNode]*livePTTTargetState) []LivePTTNode {
	nodes := make([]LivePTTNode, 0, len(targets))
	for node := range targets {
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].OrbitID == nodes[j].OrbitID {
			return nodes[i].Slot < nodes[j].Slot
		}
		return nodes[i].OrbitID < nodes[j].OrbitID
	})
	return nodes
}

func (r *LivePTTRuntime) Accept(from LivePTTNode, payload protocol.LivePTTAcceptPayload) ([]LivePTTEffect, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.bySession[payload.SessionID]
	if s == nil || s.payload.Generation != payload.Generation {
		return nil, ErrLivePTTStale
	}
	target := s.targets[from]
	if target == nil {
		return nil, ErrLivePTTUnauthorized
	}
	if target.state == "accepted" {
		return nil, nil
	}
	if target.state != "pending" || protocol.ValidateLivePTTAcceptPayload(payload) != nil {
		return nil, ErrLivePTTStale
	}
	if payload.AcceptedAtCoordMS < s.startedAtMS || payload.AcceptedAtCoordMS > s.deadlineMS {
		return nil, ErrLivePTTStale
	}
	target.state = "accepted"
	target.lastEventSeq = payload.EventSequence
	effects := []LivePTTEffect{{Kind: LivePTTSendSignal, To: s.sender, Type: protocol.TypeLivePTTAccept, Payload: &payload, SessionID: payload.SessionID}}
	if !s.duckStarted {
		s.duckStarted = true
		effects = append(effects, LivePTTEffect{Kind: LivePTTDuckStart, SessionID: payload.SessionID})
	}
	return effects, nil
}

func (r *LivePTTRuntime) Reject(from LivePTTNode, payload protocol.LivePTTRejectPayload) ([]LivePTTEffect, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.bySession[payload.SessionID]
	if s == nil || s.payload.Generation != payload.Generation {
		return nil, ErrLivePTTStale
	}
	target := s.targets[from]
	if target == nil {
		return nil, ErrLivePTTUnauthorized
	}
	if target.state != "pending" {
		return nil, nil
	}
	target.state = "rejected"
	target.lastEventSeq = payload.EventSequence
	effects := []LivePTTEffect{{Kind: LivePTTSendSignal, To: s.sender, Type: protocol.TypeLivePTTReject, Payload: &payload, SessionID: payload.SessionID}}
	if !hasLivePTTTargets(s, "pending", "accepted") {
		effects = append(effects, r.terminateLocked(s, "no_targets", payload.RejectedAtCoordMS, false)...)
	}
	return effects, nil
}

func (r *LivePTTRuntime) Failed(from LivePTTNode, payload protocol.LivePTTFailedPayload) ([]LivePTTEffect, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.bySession[payload.SessionID]
	if s == nil || s.payload.Generation != payload.Generation || s.targets[from] == nil ||
		protocol.ValidateLivePTTFailedPayload(payload) != nil {
		return nil, ErrLivePTTStale
	}
	target := s.targets[from]
	if target.state == "terminal" || payload.EventSequence == target.lastEventSeq {
		return nil, nil
	}
	if payload.EventSequence < target.lastEventSeq || payload.EventSequence > livePTTMaxTargetEvents {
		return nil, ErrLivePTTStale
	}
	target.lastEventSeq = payload.EventSequence
	target.state = "terminal"
	r.metrics.TargetPolicyDropsTotal++
	effects := []LivePTTEffect{{Kind: LivePTTSendSignal, To: s.sender,
		Type: protocol.TypeLivePTTFailed, Payload: &payload, SessionID: payload.SessionID}}
	if !hasLivePTTTargets(s, "pending", "accepted") {
		effects = append(effects, r.terminateLocked(s, payload.Code, payload.FailedAtCoordMS, false)...)
	}
	return effects, nil
}

func (r *LivePTTRuntime) Receipt(from LivePTTNode, payload protocol.LivePTTReceiptPayload) ([]LivePTTEffect, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.bySession[payload.SessionID]
	if s == nil || s.payload.Generation != payload.Generation || s.targets[from] == nil ||
		protocol.ValidateLivePTTReceiptPayload(payload) != nil {
		return nil, ErrLivePTTStale
	}
	target := s.targets[from]
	if target.state == "terminal" || payload.EventSequence == target.lastEventSeq {
		return nil, nil
	}
	if target.state != "accepted" || payload.EventSequence < target.lastEventSeq ||
		payload.EventSequence > livePTTMaxTargetEvents {
		return nil, ErrLivePTTStale
	}
	target.lastEventSeq = payload.EventSequence
	return []LivePTTEffect{{Kind: LivePTTSendSignal, To: s.sender,
		Type: protocol.TypeLivePTTReceipt, Payload: &payload, SessionID: payload.SessionID}}, nil
}

func hasLivePTTTargets(s *livePTTSession, states ...string) bool {
	allowed := map[string]bool{}
	for _, state := range states {
		allowed[state] = true
	}
	for _, target := range s.targets {
		if allowed[target.state] {
			return true
		}
	}
	return false
}

func (r *LivePTTRuntime) RelayFrame(from LivePTTNode, frame protocol.LivePTTBinaryFrame, raw []byte, nowMS int64) ([]LivePTTEffect, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.bySession[fmt.Sprintf("%x", frame.SessionID)]
	if s == nil || s.sender != from || nowMS <= 0 {
		r.metrics.StaleFramesTotal++
		return nil, ErrLivePTTStale
	}
	if !hasLivePTTTargets(s, "accepted") {
		r.metrics.InvalidFramesTotal++
		return nil, ErrLivePTTNotReady
	}
	nextGuard := s.guard
	switch decision := nextGuard.Accept(frame); decision {
	case protocol.LivePTTFrameDuplicate:
		r.metrics.DuplicateFramesTotal++
		return nil, nil
	case protocol.LivePTTFrameStale:
		r.metrics.StaleFramesTotal++
		return nil, ErrLivePTTStale
	case protocol.LivePTTFrameInvalid:
		r.metrics.InvalidFramesTotal++
		return nil, fmt.Errorf("invalid live PTT frame")
	}
	if nowMS < s.rateAtMS {
		nowMS = s.rateAtMS
	}
	s.rateMilli += (nowMS - s.rateAtMS) * livePTTRateFramesPerSec
	if s.rateMilli > livePTTRateBurstFrames*1000 {
		s.rateMilli = livePTTRateBurstFrames * 1000
	}
	s.rateAtMS = nowMS
	if s.rateMilli < 1000 {
		r.metrics.InvalidFramesTotal++
		return nil, fmt.Errorf("live PTT frame rate exceeded")
	}
	s.rateMilli -= 1000
	s.guard = nextGuard
	effects := make([]LivePTTEffect, 0, len(s.targets))
	for _, node := range sortedLivePTTNodes(s.targets) {
		if s.targets[node].state == "accepted" {
			effects = append(effects, LivePTTEffect{Kind: LivePTTSendBinary, To: node,
				Binary: append([]byte(nil), raw...), SessionID: s.payload.SessionID})
		}
	}
	r.metrics.FramesRelayedTotal++
	return effects, nil
}

func (r *LivePTTRuntime) End(from LivePTTNode, payload protocol.LivePTTEndPayload) ([]LivePTTEffect, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.bySession[payload.SessionID]
	if s == nil || s.sender != from || s.payload.Generation != payload.Generation || protocol.ValidateLivePTTEndPayload(payload) != nil {
		return nil, ErrLivePTTStale
	}
	effects := r.signalTargetsLocked(s, protocol.TypeLivePTTEnd, &payload)
	effects = append(effects, r.terminateLocked(s, payload.Reason, payload.EndedAtCoordMS, false)...)
	return effects, nil
}

func (r *LivePTTRuntime) Cancel(from LivePTTNode, payload protocol.LivePTTCancelPayload) ([]LivePTTEffect, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.bySession[payload.SessionID]
	if s == nil || s.sender != from || s.payload.Generation != payload.Generation || protocol.ValidateLivePTTCancelPayload(payload) != nil {
		return nil, ErrLivePTTStale
	}
	effects := r.signalTargetsLocked(s, protocol.TypeLivePTTCancel, &payload)
	effects = append(effects, r.terminateLocked(s, payload.Reason, payload.CancelledAtCoordMS, false)...)
	return effects, nil
}

func (r *LivePTTRuntime) signalTargetsLocked(s *livePTTSession, messageType string, payload any) []LivePTTEffect {
	var effects []LivePTTEffect
	for _, node := range sortedLivePTTNodes(s.targets) {
		if s.targets[node].state == "accepted" || s.targets[node].state == "pending" {
			effects = append(effects, LivePTTEffect{Kind: LivePTTSendSignal, To: node, Type: messageType, Payload: payload, SessionID: s.payload.SessionID})
		}
	}
	return effects
}

func (r *LivePTTRuntime) TargetUnavailable(node LivePTTNode, sessionID, reason string, nowMS int64) []LivePTTEffect {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.bySession[sessionID]
	if s == nil {
		return nil
	}
	target := s.targets[node]
	if target == nil || (target.state != "pending" && target.state != "accepted") {
		return nil
	}
	target.state = "terminal"
	if reason == "backpressure" {
		r.metrics.TargetBackpressureTotal++
	} else {
		r.metrics.TargetPolicyDropsTotal++
	}
	s.lastEventSeq++
	failed := protocol.LivePTTFailedPayload{SessionID: sessionID, Generation: s.payload.Generation,
		EventSequence: s.lastEventSeq, Stage: "relay", Code: reason, FailedAtCoordMS: nowMS}
	effects := []LivePTTEffect{{Kind: LivePTTSendSignal, To: s.sender, Type: protocol.TypeLivePTTFailed, Payload: &failed, SessionID: sessionID}}
	if !hasLivePTTTargets(s, "pending", "accepted") {
		effects = append(effects, r.terminateLocked(s, reason, nowMS, false)...)
	}
	return effects
}

func (r *LivePTTRuntime) Disconnect(node LivePTTNode, nowMS int64) []LivePTTEffect {
	r.mu.Lock()
	defer r.mu.Unlock()
	var effects []LivePTTEffect
	for _, s := range r.bySession {
		if s.sender == node {
			effects = append(effects, r.signalTargetsLocked(s, protocol.TypeLivePTTCancel,
				&protocol.LivePTTCancelPayload{SessionID: s.payload.SessionID, Generation: s.payload.Generation, CommandSequence: 2, CancelledAtCoordMS: nowMS, Reason: "sender_disconnect", DiscardBuffered: true})...)
			effects = append(effects, r.terminateLocked(s, "sender_disconnect", nowMS, false)...)
		} else if s.targets[node] != nil {
			effects = append(effects, r.targetUnavailableLocked(s, node, "target_revoked", nowMS)...)
		}
	}
	return effects
}

func (r *LivePTTRuntime) DeliveryUnavailable(node LivePTTNode, sessionID string, nowMS int64) []LivePTTEffect {
	r.mu.Lock()
	defer r.mu.Unlock()
	s := r.bySession[sessionID]
	if s == nil {
		return nil
	}
	if s.sender == node {
		cancel := protocol.LivePTTCancelPayload{SessionID: s.payload.SessionID,
			Generation: s.payload.Generation, CommandSequence: 2, CancelledAtCoordMS: nowMS,
			Reason: "sender_disconnect", DiscardBuffered: true}
		effects := r.signalTargetsLocked(s, protocol.TypeLivePTTCancel, &cancel)
		return append(effects, r.terminateLocked(s, "sender_disconnect", nowMS, false)...)
	}
	return r.targetUnavailableLocked(s, node, "target_revoked", nowMS)
}

func (r *LivePTTRuntime) targetUnavailableLocked(s *livePTTSession, node LivePTTNode, reason string, nowMS int64) []LivePTTEffect {
	target := s.targets[node]
	if target == nil || (target.state != "pending" && target.state != "accepted") {
		return nil
	}
	target.state = "terminal"
	r.metrics.TargetPolicyDropsTotal++
	s.lastEventSeq++
	failed := protocol.LivePTTFailedPayload{SessionID: s.payload.SessionID, Generation: s.payload.Generation, EventSequence: s.lastEventSeq, Stage: "policy", Code: reason, FailedAtCoordMS: nowMS}
	effects := []LivePTTEffect{{Kind: LivePTTSendSignal, To: s.sender, Type: protocol.TypeLivePTTFailed, Payload: &failed, SessionID: s.payload.SessionID}}
	if !hasLivePTTTargets(s, "pending", "accepted") {
		effects = append(effects, r.terminateLocked(s, reason, nowMS, false)...)
	}
	return effects
}

func (r *LivePTTRuntime) Sweep(nowMS int64) []LivePTTEffect {
	r.mu.Lock()
	defer r.mu.Unlock()
	var effects []LivePTTEffect
	for _, s := range r.bySession {
		if (nowMS >= s.deadlineMS && !hasLivePTTTargets(s, "accepted")) ||
			nowMS-s.startedAtMS >= protocol.LivePTTMaxDurationMS {
			r.metrics.WatchdogCancellations++
			cancel := protocol.LivePTTCancelPayload{SessionID: s.payload.SessionID, Generation: s.payload.Generation, CommandSequence: 2, CancelledAtCoordMS: nowMS, Reason: "timeout", DiscardBuffered: true}
			effects = append(effects, r.signalTargetsLocked(s, protocol.TypeLivePTTCancel, &cancel)...)
			effects = append(effects, r.terminateLocked(s, "timeout", nowMS, false)...)
		}
	}
	return effects
}

func (r *LivePTTRuntime) terminateLocked(s *livePTTSession, reason string, nowMS int64, notifySender bool) []LivePTTEffect {
	delete(r.byDomain, s.domainKey)
	delete(r.bySession, s.payload.SessionID)
	r.metrics.SessionsEndedTotal++
	var effects []LivePTTEffect
	if s.duckStarted {
		effects = append(effects, LivePTTEffect{Kind: LivePTTDuckEnd, SessionID: s.payload.SessionID})
	}
	if notifySender {
		s.lastEventSeq++
		failed := protocol.LivePTTFailedPayload{SessionID: s.payload.SessionID, Generation: s.payload.Generation, EventSequence: s.lastEventSeq, Stage: "relay", Code: reason, FailedAtCoordMS: nowMS}
		effects = append(effects, LivePTTEffect{Kind: LivePTTSendSignal, To: s.sender, Type: protocol.TypeLivePTTFailed, Payload: &failed, SessionID: s.payload.SessionID})
	}
	return effects
}

func (r *LivePTTRuntime) ResetForRestart() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.byDomain = map[string]*livePTTSession{}
	r.bySession = map[string]*livePTTSession{}
}

func (r *LivePTTRuntime) Metrics() LivePTTMetrics {
	r.mu.Lock()
	defer r.mu.Unlock()
	metrics := r.metrics
	metrics.ActiveSessions = len(r.bySession)
	return metrics
}

func (r *LivePTTRuntime) ActiveBindings() []LivePTTBinding {
	r.mu.Lock()
	defer r.mu.Unlock()
	bindings := make([]LivePTTBinding, 0, len(r.bySession))
	for _, s := range r.bySession {
		binding := LivePTTBinding{SessionID: s.payload.SessionID, DomainKind: s.payload.PlaybackDomain,
			DomainID: s.payload.PlaybackDomainID, Sender: s.sender}
		for _, node := range sortedLivePTTNodes(s.targets) {
			if s.targets[node].state == "pending" || s.targets[node].state == "accepted" {
				binding.Targets = append(binding.Targets, node)
			}
		}
		bindings = append(bindings, binding)
	}
	return bindings
}

func (r *LivePTTRuntime) DomainActive(kind string, id int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.byDomain[livePTTDomainKey(kind, id)] != nil
}
