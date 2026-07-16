package session

import (
	"errors"
	"fmt"
	"sort"

	"relux.works/duet/coordinator/internal/protocol"
)

var (
	ErrStreamFlowInvalid  = errors.New("stream main-program input is invalid")
	ErrStreamFlowConflict = errors.New("stream main-program state changed")
)

type StreamFlowState string

const (
	StreamFlowIdle        StreamFlowState = "idle"
	StreamFlowLoading     StreamFlowState = "loading"
	StreamFlowArmed       StreamFlowState = "armed"
	StreamFlowPlaying     StreamFlowState = "playing"
	StreamFlowPaused      StreamFlowState = "paused"
	StreamFlowRebuffering StreamFlowState = "rebuffering"
)

// MainProgramSource is the provider-neutral base program parked beneath a
// streamed-track insert. The stream FSM never interprets provider references.
type MainProgramSource struct {
	Kind string `json:"kind"`
	Ref  string `json:"ref"`
}

type StreamProgramItem struct {
	StreamID, MediaID, VariantManifest, VariantURL string
	VariantETag, VariantSHA256                     string
	VariantSizeBytes, DurationMS                   int64
	MixedVersionPolicy                             string
}

type StreamFlowTarget struct {
	Node           protocol.NodeID
	RTTMS          int64
	SupportsStream bool
}

type StreamFlowSnapshot struct {
	State               StreamFlowState
	Current             *StreamProgramItem
	Base                MainProgramSource
	PlaybackGeneration  int64
	SeekGeneration      int64
	AudiblePositionMS   int64
	PausedAfterReady    bool
	SupportedTargetNode []protocol.NodeID
}

type StreamFlowEffect any

type (
	EffStreamLoad struct {
		To      protocol.NodeID
		Payload protocol.StreamLoadPayload
	}
	EffStreamResumeAt struct {
		To      protocol.NodeID
		Payload protocol.StreamResumeAtPayload
	}
	EffStreamSeek struct {
		To      protocol.NodeID
		Payload protocol.StreamSeekPayload
	}
	EffStreamPause struct {
		To      protocol.NodeID
		Payload protocol.StreamPausePayload
	}
	EffStreamCancel struct {
		To      protocol.NodeID
		Payload protocol.StreamCancelPayload
	}
	EffStreamReceipt struct {
		To     protocol.NodeID
		Status string
		Reason string
	}
	EffStreamPersist   struct{ Snapshot StreamFlowSnapshot }
	EffStreamCompleted struct {
		Item   StreamProgramItem
		Status string
	}
	EffRestoreMainProgram struct{ Source MainProgramSource }
)

type streamFlowTargetState struct {
	Target   StreamFlowTarget
	Guard    protocol.StreamGenerationGuard
	Ready    bool
	Started  bool
	Ended    bool
	Joining  bool
	Position int64
	Buffered int64
}

// StreamMainProgram is the candidate-neutral coordinator FSM for one current
// uploaded track. Queue durability and activation are owned by the store; this
// type owns only the exact-generation node barrier and audible state.
type StreamMainProgram struct {
	State              StreamFlowState
	Current            *StreamProgramItem
	Base               MainProgramSource
	PlaybackGeneration int64
	SeekGeneration     int64
	AudiblePositionMS  int64
	PausedAfterReady   bool

	targets map[protocol.NodeID]*streamFlowTargetState
}

func NewStreamMainProgram(base MainProgramSource) *StreamMainProgram {
	return &StreamMainProgram{State: StreamFlowIdle, Base: base, targets: make(map[protocol.NodeID]*streamFlowTargetState)}
}

func validMainProgramSource(source MainProgramSource) bool {
	return source.Kind == "" && source.Ref == "" ||
		(source.Kind == "legacy_session" || source.Kind == "spotify") && source.Ref != ""
}

func validStreamProgramItem(item StreamProgramItem) bool {
	load := protocol.StreamLoadPayload{
		StreamID: item.StreamID, PlaybackGeneration: 1, CommandSequence: 1,
		MediaID: item.MediaID, VariantManifest: item.VariantManifest,
		VariantURL: item.VariantURL, VariantETag: item.VariantETag,
		VariantSHA256: item.VariantSHA256, VariantSizeBytes: item.VariantSizeBytes,
		MinimumBufferedMS: protocol.StreamMinimumBufferedMS, ReadyDeadlineCoordMS: 1,
		MixedVersionPolicy: item.MixedVersionPolicy,
	}
	return item.DurationMS > 0 && protocol.ValidateStreamLoadPayload(load) == nil
}

func (flow *StreamMainProgram) Snapshot() StreamFlowSnapshot {
	snapshot := StreamFlowSnapshot{
		State: flow.State, Base: flow.Base, PlaybackGeneration: flow.PlaybackGeneration,
		SeekGeneration: flow.SeekGeneration, AudiblePositionMS: flow.AudiblePositionMS,
		PausedAfterReady: flow.PausedAfterReady,
	}
	if flow.Current != nil {
		item := *flow.Current
		snapshot.Current = &item
	}
	for node := range flow.targets {
		snapshot.SupportedTargetNode = append(snapshot.SupportedTargetNode, node)
	}
	sort.Slice(snapshot.SupportedTargetNode, func(i, j int) bool {
		return snapshot.SupportedTargetNode[i] < snapshot.SupportedTargetNode[j]
	})
	return snapshot
}

func (flow *StreamMainProgram) persist() StreamFlowEffect {
	return EffStreamPersist{Snapshot: flow.Snapshot()}
}

func (flow *StreamMainProgram) orderedTargets() []*streamFlowTargetState {
	targets := make([]*streamFlowTargetState, 0, len(flow.targets))
	for _, target := range flow.targets {
		targets = append(targets, target)
	}
	sort.Slice(targets, func(i, j int) bool {
		return targets[i].Target.Node < targets[j].Target.Node
	})
	return targets
}

func (flow *StreamMainProgram) Start(
	item StreamProgramItem,
	playbackGeneration, startPositionMS, nowMS int64,
	targets []StreamFlowTarget,
) ([]StreamFlowEffect, error) {
	if flow.State != StreamFlowIdle || flow.Current != nil ||
		!validMainProgramSource(flow.Base) || !validStreamProgramItem(item) ||
		playbackGeneration <= flow.PlaybackGeneration || startPositionMS < 0 ||
		startPositionMS > item.DurationMS || nowMS <= 0 || len(targets) == 0 {
		return nil, ErrStreamFlowInvalid
	}
	seen := make(map[protocol.NodeID]bool, len(targets))
	unsupported := make([]StreamFlowTarget, 0)
	supported := make([]StreamFlowTarget, 0, len(targets))
	for _, target := range targets {
		if target.Node == "" || target.RTTMS < 0 || seen[target.Node] {
			return nil, ErrStreamFlowInvalid
		}
		seen[target.Node] = true
		if target.SupportsStream {
			supported = append(supported, target)
		} else {
			unsupported = append(unsupported, target)
		}
	}
	if item.MixedVersionPolicy == protocol.StreamMixedVersionRequireAll && len(unsupported) > 0 {
		return nil, ErrStreamFlowConflict
	}
	if len(supported) == 0 {
		return nil, ErrStreamFlowConflict
	}
	sort.Slice(unsupported, func(i, j int) bool { return unsupported[i].Node < unsupported[j].Node })
	sort.Slice(supported, func(i, j int) bool { return supported[i].Node < supported[j].Node })
	flow.Current = &item
	flow.PlaybackGeneration = playbackGeneration
	flow.SeekGeneration = 0
	flow.AudiblePositionMS = startPositionMS
	flow.PausedAfterReady = false
	flow.State = StreamFlowLoading
	flow.targets = make(map[protocol.NodeID]*streamFlowTargetState, len(supported))
	effects := make([]StreamFlowEffect, 0, len(targets)+1)
	for _, target := range unsupported {
		effects = append(effects, EffStreamReceipt{To: target.Node, Status: "unsupported", Reason: "capability_missing"})
	}
	for _, target := range supported {
		state := &streamFlowTargetState{Target: target, Position: startPositionMS}
		if state.Guard.AcceptLoad(playbackGeneration, 0, 1) != protocol.StreamGenerationApply {
			return nil, ErrStreamFlowInvalid
		}
		flow.targets[target.Node] = state
		effects = append(effects, EffStreamLoad{To: target.Node, Payload: flow.loadPayload(state, startPositionMS, nowMS)})
	}
	effects = append(effects, flow.persist())
	return effects, nil
}

// Replace cancels the current streamed insert and starts the replacement
// without briefly restoring the parked base source. The durable queue owner
// supplies the next strictly greater playback generation.
func (flow *StreamMainProgram) Replace(
	item StreamProgramItem,
	playbackGeneration, startPositionMS, nowMS int64,
	targets []StreamFlowTarget,
) ([]StreamFlowEffect, error) {
	if flow.Current == nil {
		return flow.Start(item, playbackGeneration, startPositionMS, nowMS, targets)
	}
	if playbackGeneration <= flow.PlaybackGeneration {
		return nil, ErrStreamFlowInvalid
	}
	preflight := NewStreamMainProgram(flow.Base)
	preflight.PlaybackGeneration = flow.PlaybackGeneration
	if _, err := preflight.Start(item, playbackGeneration, startPositionMS, nowMS, targets); err != nil {
		return nil, err
	}
	old := *flow.Current
	effects := make([]StreamFlowEffect, 0, len(flow.targets)+len(targets)+2)
	for _, target := range flow.orderedTargets() {
		sequence := target.Guard.CommandSequence + 1
		apply, err := streamDecision(target.Guard.AcceptCommand(
			flow.PlaybackGeneration, target.Guard.SeekGeneration, sequence, "cancel",
		))
		if err != nil || !apply {
			return nil, err
		}
		effects = append(effects, EffStreamCancel{To: target.Target.Node, Payload: protocol.StreamCancelPayload{
			StreamID: old.StreamID, PlaybackGeneration: flow.PlaybackGeneration,
			SeekGeneration: target.Guard.SeekGeneration, CommandSequence: sequence,
			Reason: "replaced",
		}})
	}
	flow.State = StreamFlowIdle
	flow.Current = nil
	flow.SeekGeneration = 0
	flow.AudiblePositionMS = 0
	flow.PausedAfterReady = false
	flow.targets = make(map[protocol.NodeID]*streamFlowTargetState)
	effects = append(effects, EffStreamCompleted{Item: old, Status: "replaced"})
	started, err := flow.Start(item, playbackGeneration, startPositionMS, nowMS, targets)
	if err != nil {
		return nil, err
	}
	return append(effects, started...), nil
}

func (flow *StreamMainProgram) loadPayload(
	target *streamFlowTargetState,
	positionMS, nowMS int64,
) protocol.StreamLoadPayload {
	item := flow.Current
	return protocol.StreamLoadPayload{
		StreamID: item.StreamID, PlaybackGeneration: flow.PlaybackGeneration,
		SeekGeneration: 0, CommandSequence: target.Guard.CommandSequence,
		MediaID: item.MediaID, VariantManifest: item.VariantManifest,
		VariantURL: item.VariantURL, VariantETag: item.VariantETag,
		VariantSHA256: item.VariantSHA256, VariantSizeBytes: item.VariantSizeBytes,
		StartPositionMS: positionMS, MinimumBufferedMS: protocol.StreamMinimumBufferedMS,
		ReadyDeadlineCoordMS: nowMS + protocol.StreamLoadReadyTimeoutMS,
		MixedVersionPolicy:   item.MixedVersionPolicy,
	}
}

func streamDecision(decision protocol.StreamGenerationDecision) (bool, error) {
	switch decision {
	case protocol.StreamGenerationApply:
		return true, nil
	case protocol.StreamGenerationDuplicate, protocol.StreamGenerationStale:
		return false, nil
	default:
		return false, ErrStreamFlowConflict
	}
}

func (flow *StreamMainProgram) targetForEvent(
	node protocol.NodeID,
	streamID string,
) *streamFlowTargetState {
	if flow.Current == nil || streamID != flow.Current.StreamID {
		return nil
	}
	target := flow.targets[node]
	if target == nil {
		return nil
	}
	return target
}

func (flow *StreamMainProgram) OnReady(
	nowMS int64,
	node protocol.NodeID,
	payload protocol.StreamReadyPayload,
) ([]StreamFlowEffect, error) {
	if nowMS <= 0 || protocol.ValidateStreamReadyPayload(payload) != nil {
		return nil, ErrStreamFlowInvalid
	}
	target := flow.targetForEvent(node, payload.StreamID)
	if target == nil {
		return nil, nil
	}
	if payload.AudiblePositionMS > flow.Current.DurationMS {
		return nil, ErrStreamFlowInvalid
	}
	apply, err := streamDecision(target.Guard.AcceptReady(
		payload.PlaybackGeneration, payload.SeekGeneration, payload.EventSequence,
		payload.BufferedDurationMS, protocol.StreamMinimumBufferedMS,
	))
	if err != nil || !apply {
		return nil, err
	}
	target.Ready = true
	target.Position = payload.AudiblePositionMS
	target.Buffered = payload.BufferedDurationMS
	if target.Joining && (flow.State == StreamFlowPlaying || flow.State == StreamFlowArmed) {
		return flow.resumeTargets(nowMS, []*streamFlowTargetState{target})
	}
	if flow.State != StreamFlowLoading && flow.State != StreamFlowRebuffering {
		return []StreamFlowEffect{flow.persist()}, nil
	}
	for _, state := range flow.targets {
		if !state.Ready {
			return []StreamFlowEffect{flow.persist()}, nil
		}
	}
	if flow.PausedAfterReady {
		flow.State = StreamFlowPaused
		return []StreamFlowEffect{flow.persist()}, nil
	}
	return flow.resumeTargets(nowMS, flow.orderedTargets())
}

func (flow *StreamMainProgram) resumeTargets(
	nowMS int64,
	targets []*streamFlowTargetState,
) ([]StreamFlowEffect, error) {
	sort.Slice(targets, func(i, j int) bool {
		return targets[i].Target.Node < targets[j].Target.Node
	})
	var maxRTT int64
	for _, target := range targets {
		if target.Target.RTTMS > maxRTT {
			maxRTT = target.Target.RTTMS
		}
	}
	startAt := nowMS + 2*maxRTT + 500
	effects := make([]StreamFlowEffect, 0, len(targets)+1)
	for _, target := range targets {
		sequence := target.Guard.CommandSequence + 1
		apply, err := streamDecision(target.Guard.AcceptCommand(
			flow.PlaybackGeneration, target.Guard.SeekGeneration, sequence, "resume",
		))
		if err != nil || !apply {
			return nil, err
		}
		effects = append(effects, EffStreamResumeAt{To: target.Target.Node, Payload: protocol.StreamResumeAtPayload{
			StreamID: flow.Current.StreamID, PlaybackGeneration: flow.PlaybackGeneration,
			SeekGeneration: target.Guard.SeekGeneration, CommandSequence: sequence,
			TCoordMS: startAt, StartDeadlineCoordMS: startAt + protocol.StreamStartDeadlineMS,
		}})
	}
	if len(targets) == len(flow.targets) {
		flow.State = StreamFlowArmed
	}
	effects = append(effects, flow.persist())
	return effects, nil
}

func (flow *StreamMainProgram) OnStarted(
	node protocol.NodeID,
	payload protocol.StreamStartedPayload,
) ([]StreamFlowEffect, error) {
	target := flow.targetForEvent(node, payload.StreamID)
	if target == nil {
		return nil, nil
	}
	if payload.AudiblePositionMS < 0 || payload.AudiblePositionMS > flow.Current.DurationMS ||
		payload.TFirstSampleCoordMS <= 0 {
		return nil, ErrStreamFlowInvalid
	}
	apply, err := streamDecision(target.Guard.AcceptEvent(
		payload.PlaybackGeneration, payload.SeekGeneration, payload.EventSequence,
		protocol.StreamEventStarted,
	))
	if err != nil || !apply {
		return nil, err
	}
	target.Started = true
	target.Joining = false
	target.Position = payload.AudiblePositionMS
	for _, state := range flow.targets {
		if !state.Started && !state.Joining {
			return []StreamFlowEffect{flow.persist()}, nil
		}
	}
	flow.State = StreamFlowPlaying
	flow.PausedAfterReady = false
	flow.updateAudiblePosition()
	return []StreamFlowEffect{flow.persist()}, nil
}

func (flow *StreamMainProgram) OnProgress(
	node protocol.NodeID,
	payload protocol.StreamProgressPayload,
) ([]StreamFlowEffect, error) {
	target := flow.targetForEvent(node, payload.StreamID)
	if target == nil {
		return nil, nil
	}
	if payload.AudiblePositionMS < target.Position || payload.BufferedDurationMS < 0 ||
		payload.AudiblePositionMS > flow.Current.DurationMS {
		return nil, ErrStreamFlowInvalid
	}
	apply, err := streamDecision(target.Guard.AcceptEvent(
		payload.PlaybackGeneration, payload.SeekGeneration, payload.EventSequence,
		protocol.StreamEventProgress,
	))
	if err != nil || !apply {
		return nil, err
	}
	target.Position = payload.AudiblePositionMS
	target.Buffered = payload.BufferedDurationMS
	flow.updateAudiblePosition()
	return []StreamFlowEffect{flow.persist()}, nil
}

func (flow *StreamMainProgram) updateAudiblePosition() {
	first := true
	var minimum int64
	for _, target := range flow.targets {
		if !target.Started || target.Joining {
			continue
		}
		if first || target.Position < minimum {
			minimum = target.Position
			first = false
		}
	}
	if !first && minimum >= flow.AudiblePositionMS {
		flow.AudiblePositionMS = minimum
	}
}

func (flow *StreamMainProgram) Pause(fadeMS int64) ([]StreamFlowEffect, error) {
	if flow.Current == nil || flow.State != StreamFlowPlaying || fadeMS < 0 || fadeMS > 1000 {
		return nil, ErrStreamFlowConflict
	}
	effects := make([]StreamFlowEffect, 0, len(flow.targets)+1)
	for _, target := range flow.orderedTargets() {
		node := target.Target.Node
		sequence := target.Guard.CommandSequence + 1
		command := "pause"
		if target.Joining {
			command = "cancel"
		}
		apply, err := streamDecision(target.Guard.AcceptCommand(
			flow.PlaybackGeneration, target.Guard.SeekGeneration, sequence, command,
		))
		if err != nil || !apply {
			return nil, err
		}
		if target.Joining {
			effects = append(effects, EffStreamCancel{To: target.Target.Node, Payload: protocol.StreamCancelPayload{
				StreamID: flow.Current.StreamID, PlaybackGeneration: flow.PlaybackGeneration,
				SeekGeneration: target.Guard.SeekGeneration, CommandSequence: sequence,
				Reason: "paused_during_join",
			}})
			delete(flow.targets, node)
			continue
		}
		effects = append(effects, EffStreamPause{To: target.Target.Node, Payload: protocol.StreamPausePayload{
			StreamID: flow.Current.StreamID, PlaybackGeneration: flow.PlaybackGeneration,
			SeekGeneration: target.Guard.SeekGeneration, CommandSequence: sequence, FadeMS: fadeMS,
		}})
	}
	flow.State = StreamFlowPaused
	flow.PausedAfterReady = true
	flow.updateAudiblePosition()
	effects = append(effects, flow.persist())
	return effects, nil
}

func (flow *StreamMainProgram) Resume(nowMS int64) ([]StreamFlowEffect, error) {
	if flow.Current == nil || flow.State != StreamFlowPaused || nowMS <= 0 || len(flow.targets) == 0 {
		return nil, ErrStreamFlowConflict
	}
	for _, target := range flow.targets {
		if !target.Ready {
			flow.State = StreamFlowLoading
			flow.PausedAfterReady = false
			return []StreamFlowEffect{flow.persist()}, nil
		}
	}
	flow.PausedAfterReady = false
	return flow.resumeTargets(nowMS, flow.orderedTargets())
}

func (flow *StreamMainProgram) SeekTo(positionMS, nowMS int64) ([]StreamFlowEffect, error) {
	if flow.Current == nil || (flow.State != StreamFlowPlaying && flow.State != StreamFlowPaused) ||
		positionMS < 0 || positionMS > flow.Current.DurationMS || nowMS <= 0 {
		return nil, ErrStreamFlowConflict
	}
	paused := flow.State == StreamFlowPaused
	flow.SeekGeneration++
	flow.AudiblePositionMS = positionMS
	flow.State = StreamFlowLoading
	flow.PausedAfterReady = paused
	effects := make([]StreamFlowEffect, 0, len(flow.targets)+1)
	for _, target := range flow.orderedTargets() {
		seekGeneration := target.Guard.SeekGeneration + 1
		sequence := target.Guard.CommandSequence + 1
		apply, err := streamDecision(target.Guard.AcceptSeek(
			flow.PlaybackGeneration, seekGeneration, sequence,
		))
		if err != nil || !apply {
			return nil, err
		}
		target.Ready, target.Started, target.Ended = false, false, false
		target.Position = positionMS
		effects = append(effects, EffStreamSeek{To: target.Target.Node, Payload: protocol.StreamSeekPayload{
			StreamID: flow.Current.StreamID, PlaybackGeneration: flow.PlaybackGeneration,
			SeekGeneration: seekGeneration, CommandSequence: sequence, PositionMS: positionMS,
			MinimumBufferedMS:    protocol.StreamMinimumBufferedMS,
			ReadyDeadlineCoordMS: nowMS + protocol.StreamSeekReadyTimeoutMS,
		}})
	}
	effects = append(effects, flow.persist())
	return effects, nil
}

func (flow *StreamMainProgram) OnRebuffer(
	node protocol.NodeID,
	payload protocol.StreamRebufferPayload,
) ([]StreamFlowEffect, error) {
	target := flow.targetForEvent(node, payload.StreamID)
	if target == nil {
		return nil, nil
	}
	if payload.AudiblePositionMS < target.Position ||
		payload.AudiblePositionMS > flow.Current.DurationMS || payload.BufferedDurationMS < 0 {
		return nil, ErrStreamFlowInvalid
	}
	apply, err := streamDecision(target.Guard.AcceptEvent(
		payload.PlaybackGeneration, payload.SeekGeneration, payload.EventSequence,
		protocol.StreamEventRebuffer,
	))
	if err != nil || !apply {
		return nil, err
	}
	target.Position = payload.AudiblePositionMS
	target.Buffered = payload.BufferedDurationMS
	target.Ready = false
	flow.State = StreamFlowRebuffering
	flow.PausedAfterReady = false
	effects := make([]StreamFlowEffect, 0, len(flow.targets)+1)
	for _, state := range flow.orderedTargets() {
		node := state.Target.Node
		sequence := state.Guard.CommandSequence + 1
		command := "pause"
		if state.Joining {
			command = "cancel"
		}
		apply, err := streamDecision(state.Guard.AcceptCommand(
			flow.PlaybackGeneration, state.Guard.SeekGeneration, sequence, command,
		))
		if err != nil || !apply {
			return nil, err
		}
		if state.Joining {
			effects = append(effects, EffStreamCancel{To: state.Target.Node, Payload: protocol.StreamCancelPayload{
				StreamID: flow.Current.StreamID, PlaybackGeneration: flow.PlaybackGeneration,
				SeekGeneration: state.Guard.SeekGeneration, CommandSequence: sequence,
				Reason: "rebuffer_during_join",
			}})
			delete(flow.targets, node)
			continue
		}
		if state != target {
			state.Ready = true
		}
		state.Started = false
		state.Ended = false
		effects = append(effects, EffStreamPause{To: state.Target.Node, Payload: protocol.StreamPausePayload{
			StreamID: flow.Current.StreamID, PlaybackGeneration: flow.PlaybackGeneration,
			SeekGeneration: state.Guard.SeekGeneration, CommandSequence: sequence, FadeMS: 0,
		}})
	}
	flow.updateAudiblePosition()
	effects = append(effects, flow.persist())
	return effects, nil
}

func (flow *StreamMainProgram) OnEnded(
	node protocol.NodeID,
	payload protocol.StreamEndedPayload,
) ([]StreamFlowEffect, error) {
	target := flow.targetForEvent(node, payload.StreamID)
	if target == nil {
		return nil, nil
	}
	if target.Guard.Phase != "started" || payload.AudiblePositionMS < target.Position ||
		payload.AudiblePositionMS > flow.Current.DurationMS ||
		payload.TLastSampleCoordMS <= 0 || payload.Reason != "eof_drained" {
		return nil, ErrStreamFlowInvalid
	}
	apply, err := streamDecision(target.Guard.AcceptEvent(
		payload.PlaybackGeneration, payload.SeekGeneration, payload.EventSequence,
		protocol.StreamEventEnded,
	))
	if err != nil || !apply {
		return nil, err
	}
	target.Position = payload.AudiblePositionMS
	target.Ended = true
	for _, state := range flow.targets {
		if !state.Joining && !state.Ended {
			return []StreamFlowEffect{flow.persist()}, nil
		}
	}
	effects := make([]StreamFlowEffect, 0, len(flow.targets)+3)
	for _, state := range flow.orderedTargets() {
		if !state.Joining {
			continue
		}
		sequence := state.Guard.CommandSequence + 1
		apply, err := streamDecision(state.Guard.AcceptCommand(
			flow.PlaybackGeneration, state.Guard.SeekGeneration, sequence, "cancel",
		))
		if err != nil || !apply {
			return nil, err
		}
		effects = append(effects, EffStreamCancel{To: state.Target.Node, Payload: protocol.StreamCancelPayload{
			StreamID: flow.Current.StreamID, PlaybackGeneration: flow.PlaybackGeneration,
			SeekGeneration: state.Guard.SeekGeneration, CommandSequence: sequence,
			Reason: "item_drained_before_join",
		}})
	}
	return append(effects, flow.complete("eof")...), nil
}

func (flow *StreamMainProgram) failRemaining(reason string) ([]StreamFlowEffect, error) {
	effects := make([]StreamFlowEffect, 0, len(flow.targets)+3)
	for _, target := range flow.orderedTargets() {
		if target.Guard.Phase == "terminal" {
			continue
		}
		sequence := target.Guard.CommandSequence + 1
		apply, err := streamDecision(target.Guard.AcceptCommand(
			flow.PlaybackGeneration, target.Guard.SeekGeneration, sequence, "cancel",
		))
		if err != nil || !apply {
			return nil, err
		}
		effects = append(effects, EffStreamCancel{To: target.Target.Node, Payload: protocol.StreamCancelPayload{
			StreamID: flow.Current.StreamID, PlaybackGeneration: flow.PlaybackGeneration,
			SeekGeneration: target.Guard.SeekGeneration, CommandSequence: sequence, Reason: reason,
		}})
	}
	return append(effects, flow.complete("error")...), nil
}

func (flow *StreamMainProgram) allReady() bool {
	if len(flow.targets) == 0 {
		return false
	}
	for _, target := range flow.targets {
		if !target.Ready {
			return false
		}
	}
	return true
}

func (flow *StreamMainProgram) allStarted() bool {
	if len(flow.targets) == 0 {
		return false
	}
	for _, target := range flow.targets {
		if !target.Started && !target.Joining {
			return false
		}
	}
	return true
}

func (flow *StreamMainProgram) allEnded() bool {
	if len(flow.targets) == 0 {
		return false
	}
	for _, target := range flow.targets {
		if !target.Ended {
			return false
		}
	}
	return true
}

// OnFailed applies the frozen mixed-version decision to an exact-generation
// runtime failure. require_all fails the item; supported_only emits a visible
// terminal receipt and lets the remaining frozen capable targets continue.
func (flow *StreamMainProgram) OnFailed(
	nowMS int64,
	node protocol.NodeID,
	payload protocol.StreamFailedPayload,
) ([]StreamFlowEffect, error) {
	if nowMS <= 0 || !validStreamFailureToken(payload.Stage) ||
		!validStreamFailureToken(payload.Code) {
		return nil, ErrStreamFlowInvalid
	}
	target := flow.targetForEvent(node, payload.StreamID)
	if target == nil {
		return nil, nil
	}
	apply, err := streamDecision(target.Guard.AcceptEvent(
		payload.PlaybackGeneration, payload.SeekGeneration, payload.EventSequence,
		protocol.StreamEventFailed,
	))
	if err != nil || !apply {
		return nil, err
	}
	delete(flow.targets, node)
	receipt := EffStreamReceipt{To: node, Status: "failed", Reason: payload.Code}
	if flow.Current.MixedVersionPolicy == protocol.StreamMixedVersionRequireAll || len(flow.targets) == 0 {
		failed, err := flow.failRemaining("peer_failed")
		return append([]StreamFlowEffect{receipt}, failed...), err
	}
	if flow.allEnded() {
		return append([]StreamFlowEffect{receipt}, flow.complete("error")...), nil
	}
	effects := []StreamFlowEffect{receipt}
	switch flow.State {
	case StreamFlowLoading, StreamFlowRebuffering:
		if flow.allReady() {
			if flow.PausedAfterReady {
				flow.State = StreamFlowPaused
			} else {
				resumed, err := flow.resumeTargets(nowMS, flow.orderedTargets())
				return append(effects, resumed...), err
			}
		}
	case StreamFlowArmed:
		if flow.allStarted() {
			flow.State = StreamFlowPlaying
		}
	}
	return append(effects, flow.persist()), nil
}

func validStreamFailureToken(value string) bool {
	if value == "" || len(value) > 64 {
		return false
	}
	for _, char := range value {
		if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '_' {
			return false
		}
	}
	return true
}

// ReadyTimeout enforces both frozen load and post-seek readiness deadlines.
// supported_only drops each missing target with a visible receipt; require_all
// fails atomically.
func (flow *StreamMainProgram) ReadyTimeout(nowMS int64) ([]StreamFlowEffect, error) {
	if flow.Current == nil || (flow.State != StreamFlowLoading && flow.State != StreamFlowRebuffering) || nowMS <= 0 {
		return nil, ErrStreamFlowConflict
	}
	missing := make([]protocol.NodeID, 0)
	for _, target := range flow.orderedTargets() {
		if !target.Ready {
			missing = append(missing, target.Target.Node)
		}
	}
	if len(missing) == 0 {
		return nil, nil
	}
	if flow.Current.MixedVersionPolicy == protocol.StreamMixedVersionRequireAll {
		return flow.failRemaining("ready_timeout")
	}
	effects := make([]StreamFlowEffect, 0, len(missing)*2+len(flow.targets)+1)
	for _, node := range missing {
		target := flow.targets[node]
		sequence := target.Guard.CommandSequence + 1
		apply, err := streamDecision(target.Guard.AcceptCommand(
			flow.PlaybackGeneration, target.Guard.SeekGeneration, sequence, "cancel",
		))
		if err != nil || !apply {
			return nil, err
		}
		effects = append(effects,
			EffStreamCancel{To: node, Payload: protocol.StreamCancelPayload{
				StreamID: flow.Current.StreamID, PlaybackGeneration: flow.PlaybackGeneration,
				SeekGeneration: target.Guard.SeekGeneration, CommandSequence: sequence,
				Reason: "ready_timeout",
			}},
			EffStreamReceipt{To: node, Status: "failed", Reason: "ready_timeout"},
		)
		delete(flow.targets, node)
	}
	if len(flow.targets) == 0 {
		return append(effects, flow.complete("error")...), nil
	}
	if flow.PausedAfterReady {
		flow.State = StreamFlowPaused
		return append(effects, flow.persist()), nil
	}
	resumed, err := flow.resumeTargets(nowMS, flow.orderedTargets())
	return append(effects, resumed...), err
}

// StartTimeout bounds the resume_at barrier. Under supported_only, homes that
// did not report a first audible sample receive terminal receipts while
// already-audible homes continue. require_all fails the item.
func (flow *StreamMainProgram) StartTimeout(nowMS int64) ([]StreamFlowEffect, error) {
	if flow.Current == nil || flow.State != StreamFlowArmed || nowMS <= 0 {
		return nil, ErrStreamFlowConflict
	}
	missing := make([]protocol.NodeID, 0)
	for _, target := range flow.orderedTargets() {
		if !target.Started && !target.Joining {
			missing = append(missing, target.Target.Node)
		}
	}
	if len(missing) == 0 {
		return nil, nil
	}
	if flow.Current.MixedVersionPolicy == protocol.StreamMixedVersionRequireAll {
		return flow.failRemaining("start_timeout")
	}
	effects := make([]StreamFlowEffect, 0, len(missing)*2+1)
	for _, node := range missing {
		target := flow.targets[node]
		sequence := target.Guard.CommandSequence + 1
		apply, err := streamDecision(target.Guard.AcceptCommand(
			flow.PlaybackGeneration, target.Guard.SeekGeneration, sequence, "cancel",
		))
		if err != nil || !apply {
			return nil, err
		}
		effects = append(effects,
			EffStreamCancel{To: node, Payload: protocol.StreamCancelPayload{
				StreamID: flow.Current.StreamID, PlaybackGeneration: flow.PlaybackGeneration,
				SeekGeneration: target.Guard.SeekGeneration, CommandSequence: sequence,
				Reason: "start_timeout",
			}},
			EffStreamReceipt{To: node, Status: "failed", Reason: "start_timeout"},
		)
		delete(flow.targets, node)
	}
	if len(flow.targets) == 0 {
		return append(effects, flow.complete("error")...), nil
	}
	if flow.allStarted() {
		flow.State = StreamFlowPlaying
	}
	return append(effects, flow.persist()), nil
}

func (flow *StreamMainProgram) complete(status string) []StreamFlowEffect {
	item := *flow.Current
	base := flow.Base
	flow.State = StreamFlowIdle
	flow.Current = nil
	flow.SeekGeneration = 0
	flow.AudiblePositionMS = 0
	flow.PausedAfterReady = false
	flow.targets = make(map[protocol.NodeID]*streamFlowTargetState)
	effects := []StreamFlowEffect{EffStreamCompleted{Item: item, Status: status}}
	if base.Kind != "" {
		effects = append(effects, EffRestoreMainProgram{Source: base})
	}
	effects = append(effects, flow.persist())
	return effects
}

func (flow *StreamMainProgram) Cancel(reason string) ([]StreamFlowEffect, error) {
	if flow.Current == nil || reason == "" {
		return nil, ErrStreamFlowConflict
	}
	effects := make([]StreamFlowEffect, 0, len(flow.targets)+3)
	for _, target := range flow.orderedTargets() {
		sequence := target.Guard.CommandSequence + 1
		apply, err := streamDecision(target.Guard.AcceptCommand(
			flow.PlaybackGeneration, target.Guard.SeekGeneration, sequence, "cancel",
		))
		if err != nil || !apply {
			return nil, err
		}
		effects = append(effects, EffStreamCancel{To: target.Target.Node, Payload: protocol.StreamCancelPayload{
			StreamID: flow.Current.StreamID, PlaybackGeneration: flow.PlaybackGeneration,
			SeekGeneration: target.Guard.SeekGeneration, CommandSequence: sequence, Reason: reason,
		}})
	}
	return append(effects, flow.complete("cancelled")...), nil
}

// JoinLivingAir loads only the new supported home at the current audible
// position. It never replays an overlay or restarts existing participants.
func (flow *StreamMainProgram) JoinLivingAir(
	nowMS int64,
	target StreamFlowTarget,
) ([]StreamFlowEffect, error) {
	if flow.Current == nil || (flow.State != StreamFlowPlaying && flow.State != StreamFlowArmed) ||
		nowMS <= 0 || target.Node == "" || target.RTTMS < 0 || flow.targets[target.Node] != nil {
		return nil, ErrStreamFlowConflict
	}
	if !target.SupportsStream {
		return []StreamFlowEffect{EffStreamReceipt{To: target.Node, Status: "unsupported", Reason: "capability_missing"}}, nil
	}
	state := &streamFlowTargetState{
		Target: target, Joining: true, Position: flow.AudiblePositionMS,
	}
	if state.Guard.AcceptLoad(flow.PlaybackGeneration, 0, 1) != protocol.StreamGenerationApply {
		return nil, ErrStreamFlowInvalid
	}
	flow.targets[target.Node] = state
	return []StreamFlowEffect{
		EffStreamLoad{To: target.Node, Payload: flow.loadPayload(state, flow.AudiblePositionMS, nowMS)},
		flow.persist(),
	}, nil
}

// LeaveLivingAir cancels only the leaving home. The shared main program and
// every remaining participant continue with their existing generations.
func (flow *StreamMainProgram) LeaveLivingAir(node protocol.NodeID) ([]StreamFlowEffect, error) {
	target := flow.targets[node]
	if flow.Current == nil || target == nil {
		return nil, nil
	}
	sequence := target.Guard.CommandSequence + 1
	apply, err := streamDecision(target.Guard.AcceptCommand(
		flow.PlaybackGeneration, target.Guard.SeekGeneration, sequence, "cancel",
	))
	if err != nil || !apply {
		return nil, err
	}
	delete(flow.targets, node)
	effects := []StreamFlowEffect{EffStreamCancel{To: node, Payload: protocol.StreamCancelPayload{
		StreamID: flow.Current.StreamID, PlaybackGeneration: flow.PlaybackGeneration,
		SeekGeneration: target.Guard.SeekGeneration, CommandSequence: sequence, Reason: "left_air",
	}}}
	if len(flow.targets) == 0 {
		flow.State = StreamFlowPaused
		flow.PausedAfterReady = true
	}
	effects = append(effects, flow.persist())
	return effects, nil
}

// RestorePaused reconstructs one persisted current source after coordinator
// restart. It intentionally emits no command: restart is fail-paused, and the
// caller must advance the persisted generation before a fresh Start.
func (flow *StreamMainProgram) RestorePaused(snapshot StreamFlowSnapshot) error {
	if flow.Current != nil || flow.State != StreamFlowIdle || snapshot.Current == nil ||
		!validStreamProgramItem(*snapshot.Current) || !validMainProgramSource(snapshot.Base) ||
		snapshot.PlaybackGeneration <= 0 || snapshot.SeekGeneration < 0 ||
		snapshot.AudiblePositionMS < 0 || snapshot.AudiblePositionMS > snapshot.Current.DurationMS {
		return fmt.Errorf("%w: restore snapshot", ErrStreamFlowInvalid)
	}
	item := *snapshot.Current
	flow.Current = &item
	flow.Base = snapshot.Base
	flow.PlaybackGeneration = snapshot.PlaybackGeneration
	flow.SeekGeneration = snapshot.SeekGeneration
	flow.AudiblePositionMS = snapshot.AudiblePositionMS
	flow.PausedAfterReady = true
	flow.State = StreamFlowPaused
	flow.targets = make(map[protocol.NodeID]*streamFlowTargetState)
	return nil
}

// Restart reloads a fail-paused restored/current item at its last audible
// position under a fresh playback generation. Old pre-restart events are then
// stale by construction.
func (flow *StreamMainProgram) Restart(
	playbackGeneration, nowMS int64,
	targets []StreamFlowTarget,
) ([]StreamFlowEffect, error) {
	if flow.State != StreamFlowPaused || flow.Current == nil || len(flow.targets) != 0 ||
		playbackGeneration <= flow.PlaybackGeneration {
		return nil, ErrStreamFlowConflict
	}
	item := *flow.Current
	position := flow.AudiblePositionMS
	preflight := NewStreamMainProgram(flow.Base)
	preflight.PlaybackGeneration = flow.PlaybackGeneration
	if _, err := preflight.Start(item, playbackGeneration, position, nowMS, targets); err != nil {
		return nil, err
	}
	flow.State = StreamFlowIdle
	flow.Current = nil
	flow.SeekGeneration = 0
	flow.PausedAfterReady = false
	return flow.Start(item, playbackGeneration, position, nowMS, targets)
}
