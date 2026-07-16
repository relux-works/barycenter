package main

import (
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	StreamTrackMaximumFileBytes  int64 = 524288000
	StreamTrackMaximumDurationMS int64 = 7200000
	StreamTrackMaximumTargets          = 64
)

type StreamTrackDraftPhase string

const (
	StreamTrackDraftRetained   StreamTrackDraftPhase = "retained"
	StreamTrackDraftUploading  StreamTrackDraftPhase = "uploading"
	StreamTrackDraftUploaded   StreamTrackDraftPhase = "uploaded"
	StreamTrackDraftProcessing StreamTrackDraftPhase = "processing"
	StreamTrackDraftReady      StreamTrackDraftPhase = "ready"
	StreamTrackDraftFailed     StreamTrackDraftPhase = "failed"
)

type StreamTrackPlaybackPhase string

const (
	StreamTrackPlaybackIdle        StreamTrackPlaybackPhase = "idle"
	StreamTrackPlaybackQueued      StreamTrackPlaybackPhase = "queued"
	StreamTrackPlaybackLoading     StreamTrackPlaybackPhase = "loading"
	StreamTrackPlaybackReady       StreamTrackPlaybackPhase = "ready"
	StreamTrackPlaybackPlaying     StreamTrackPlaybackPhase = "playing"
	StreamTrackPlaybackPaused      StreamTrackPlaybackPhase = "paused"
	StreamTrackPlaybackSeeking     StreamTrackPlaybackPhase = "seeking"
	StreamTrackPlaybackRebuffering StreamTrackPlaybackPhase = "rebuffering"
	StreamTrackPlaybackEnded       StreamTrackPlaybackPhase = "ended"
	StreamTrackPlaybackFailed      StreamTrackPlaybackPhase = "failed"
)

type StreamTrackAudience string

const (
	StreamTrackCurrentAir StreamTrackAudience = "current_air"
	StreamTrackExplicit   StreamTrackAudience = "explicit"
)

type StreamTrackInsertion string

const (
	StreamTrackQueue   StreamTrackInsertion = "queue"
	StreamTrackReplace StreamTrackInsertion = "replace"
)

type StreamTrackFailure string

const (
	StreamTrackOffline            StreamTrackFailure = "offline"
	StreamTrackQuotaExceeded      StreamTrackFailure = "quota_exceeded"
	StreamTrackUnsupportedTargets StreamTrackFailure = "unsupported_targets"
	StreamTrackPolicyRequired     StreamTrackFailure = "policy_required"
	StreamTrackProcessingFailed   StreamTrackFailure = "processing_failed"
	StreamTrackVariantUnavailable StreamTrackFailure = "variant_unavailable"
	StreamTrackStaleGeneration    StreamTrackFailure = "stale_generation"
	StreamTrackServiceUnavailable StreamTrackFailure = "service_unavailable"
)

type StreamTrackDraft struct {
	LocalID                 string
	LocalByteCount          int64
	RetainedLocalBytes      bool
	Title                   string
	ClientMIME              string
	DurationMS              int64
	HasDuration             bool
	Phase                   StreamTrackDraftPhase
	PhaseLabel              TargetsInboxLocalizedLabel
	MediaID                 string
	VariantManifest         string
	ServerMetadataConfirmed bool
	UploadOffset            int64
	ProcessingPercent       int
	Failure                 StreamTrackFailure
	FailureLabel            TargetsInboxLocalizedLabel
}

func (value StreamTrackDraft) String() string   { return "StreamTrackDraft{<opaque>}" }
func (value StreamTrackDraft) GoString() string { return value.String() }

type StreamTrackPlayback struct {
	Phase              StreamTrackPlaybackPhase
	PhaseLabel         TargetsInboxLocalizedLabel
	StreamID           string
	DurationMS         int64
	AudiblePositionMS  int64
	PlaybackGeneration uint64
	SeekGeneration     uint64
	Failure            StreamTrackFailure
	FailureLabel       TargetsInboxLocalizedLabel
}

func (value StreamTrackPlayback) String() string   { return "StreamTrackPlayback{<opaque>}" }
func (value StreamTrackPlayback) GoString() string { return value.String() }

type StreamTrackSnapshot struct {
	State                   TargetsInboxSurfaceState
	StateLabel              TargetsInboxLocalizedLabel
	Draft                   *StreamTrackDraft
	Playback                StreamTrackPlayback
	Targets                 []TargetsInboxTargetChoice
	SelectedReferences      []string
	SelectedAudience        StreamTrackAudience
	SelectedInsertion       StreamTrackInsertion
	ActiveAirAvailable      bool
	ContentPolicyState      string
	Actions                 []TargetsInboxActionCapability
	Failure                 StreamTrackFailure
	FailureLabel            TargetsInboxLocalizedLabel
	ConfirmedDeletedLocalID string
}

func (value StreamTrackSnapshot) String() string   { return "StreamTrackSnapshot{<opaque>}" }
func (value StreamTrackSnapshot) GoString() string { return value.String() }

type StreamTrackCommandKind string

const (
	StreamTrackAcceptPolicy   StreamTrackCommandKind = "accept_policy"
	StreamTrackUpload         StreamTrackCommandKind = "upload"
	StreamTrackRetry          StreamTrackCommandKind = "retry"
	StreamTrackDelete         StreamTrackCommandKind = "delete"
	StreamTrackQueueCommand   StreamTrackCommandKind = "queue"
	StreamTrackReplaceCommand StreamTrackCommandKind = "replace"
	StreamTrackPause          StreamTrackCommandKind = "pause"
	StreamTrackSeek           StreamTrackCommandKind = "seek"
	StreamTrackResume         StreamTrackCommandKind = "resume"
	StreamTrackReport         StreamTrackCommandKind = "report"
)

type StreamTrackCommand struct {
	Kind               StreamTrackCommandKind
	LocalID            string
	MediaID            string
	StreamID           string
	Audience           StreamTrackAudience
	Targets            []string
	Confirmed          bool
	PositionMS         int64
	PlaybackGeneration uint64
	SeekGeneration     uint64
	Details            string
}

func (value StreamTrackCommand) String() string {
	return "StreamTrackCommand{" + string(value.Kind) + ",<opaque>}"
}
func (value StreamTrackCommand) GoString() string { return value.String() }

type StreamTrackModel struct {
	mu       sync.RWMutex
	snapshot StreamTrackSnapshot
}

var streamTrackLocalIDPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)

func NewStreamTrackModel() *StreamTrackModel {
	return &StreamTrackModel{snapshot: StreamTrackSnapshot{
		State:              TargetsInboxLoading,
		Playback:           StreamTrackPlayback{Phase: StreamTrackPlaybackIdle},
		ContentPolicyState: "required",
	}}
}

func (model *StreamTrackModel) Replace(replacement StreamTrackSnapshot, now time.Time) {
	if model == nil {
		return
	}
	model.mu.Lock()
	defer model.mu.Unlock()
	current := cloneStreamTrackSnapshot(model.snapshot)
	normalized := normalizeStreamTrackSnapshot(replacement, current, now)
	model.snapshot = normalized
}

func (model *StreamTrackModel) Snapshot() StreamTrackSnapshot {
	if model == nil {
		return StreamTrackSnapshot{State: TargetsInboxCoordinatorError}
	}
	model.mu.RLock()
	defer model.mu.RUnlock()
	return cloneStreamTrackSnapshot(model.snapshot)
}

func (model *StreamTrackModel) SelectAudience(audience StreamTrackAudience) bool {
	if model == nil {
		return false
	}
	model.mu.Lock()
	defer model.mu.Unlock()
	if model.snapshot.State != TargetsInboxReady {
		return false
	}
	if audience == StreamTrackCurrentAir {
		if !model.snapshot.ActiveAirAvailable {
			return false
		}
		model.snapshot.SelectedReferences = nil
	} else if audience != StreamTrackExplicit || len(model.snapshot.SelectedReferences) == 0 {
		return false
	}
	model.snapshot.SelectedAudience = audience
	return true
}

func (model *StreamTrackModel) SelectTargets(references []string) bool {
	if model == nil {
		return false
	}
	model.mu.Lock()
	defer model.mu.Unlock()
	if model.snapshot.State != TargetsInboxReady || len(references) > StreamTrackMaximumTargets {
		return false
	}
	available := make(map[string]bool, len(model.snapshot.Targets))
	for _, target := range model.snapshot.Targets {
		available[target.Reference] = true
	}
	seen := map[string]bool{}
	for _, reference := range references {
		if !available[reference] || seen[reference] {
			return false
		}
		seen[reference] = true
	}
	model.snapshot.SelectedReferences = append([]string(nil), references...)
	if len(references) == 0 && model.snapshot.SelectedAudience == StreamTrackExplicit {
		model.snapshot.SelectedAudience = ""
	}
	return true
}

func (model *StreamTrackModel) SelectInsertion(insertion StreamTrackInsertion) bool {
	if model == nil || insertion != StreamTrackQueue && insertion != StreamTrackReplace {
		return false
	}
	model.mu.Lock()
	defer model.mu.Unlock()
	if model.snapshot.State != TargetsInboxReady {
		return false
	}
	model.snapshot.SelectedInsertion = insertion
	return true
}

func normalizeStreamTrackSnapshot(value, current StreamTrackSnapshot, now time.Time) StreamTrackSnapshot {
	value = cloneStreamTrackSnapshot(value)
	switch value.State {
	case TargetsInboxLoading, TargetsInboxReady, TargetsInboxStale, TargetsInboxOffline, TargetsInboxCoordinatorError:
	default:
		value.State = TargetsInboxCoordinatorError
	}
	if !validStreamTrackLabel(value.StateLabel, "surface."+string(value.State)) {
		value.StateLabel = TargetsInboxLocalizedLabel{}
	}
	value.Failure = canonicalStreamTrackFailure(value.Failure)
	if value.Failure == "" || !validStreamTrackLabel(value.FailureLabel, "stream_track.failure."+string(value.Failure)) {
		value.FailureLabel = TargetsInboxLocalizedLabel{}
	}
	if value.ContentPolicyState != "current" && value.ContentPolicyState != "required" && value.ContentPolicyState != "stale" {
		value.ContentPolicyState = "required"
	}
	if value.SelectedInsertion != "" && value.SelectedInsertion != StreamTrackQueue && value.SelectedInsertion != StreamTrackReplace {
		value.SelectedInsertion = ""
	}
	if value.ConfirmedDeletedLocalID != "" && !streamTrackLocalIDPattern.MatchString(value.ConfirmedDeletedLocalID) {
		value.ConfirmedDeletedLocalID = ""
	}
	value.Actions = canonicalStreamTrackActions(value.Actions)
	if value.State != TargetsInboxReady {
		value.Actions = nil
	}
	value.Targets = canonicalStreamTrackTargets(value.Targets, now)
	available := make(map[string]bool, len(value.Targets))
	for _, target := range value.Targets {
		available[target.Reference] = true
	}
	selected := make([]string, 0, min(len(value.SelectedReferences), StreamTrackMaximumTargets))
	seen := map[string]bool{}
	for _, reference := range value.SelectedReferences {
		if available[reference] && !seen[reference] && len(selected) < StreamTrackMaximumTargets {
			seen[reference] = true
			selected = append(selected, reference)
		}
	}
	value.SelectedReferences = selected
	if value.SelectedAudience == StreamTrackCurrentAir && !value.ActiveAirAvailable ||
		value.SelectedAudience == StreamTrackExplicit && len(selected) == 0 ||
		value.SelectedAudience != "" && value.SelectedAudience != StreamTrackCurrentAir && value.SelectedAudience != StreamTrackExplicit {
		value.SelectedAudience = ""
	}
	if value.Draft != nil && !normalizeStreamTrackDraft(value.Draft) {
		value.Draft = nil
	}
	if value.Draft == nil && current.Draft != nil && current.Draft.RetainedLocalBytes &&
		value.ConfirmedDeletedLocalID != current.Draft.LocalID {
		retained := *current.Draft
		value.Draft = &retained
	}
	value.Playback = normalizeStreamTrackPlayback(value.Playback, current.Playback)
	return value
}

func normalizeStreamTrackDraft(value *StreamTrackDraft) bool {
	if value == nil || !streamTrackLocalIDPattern.MatchString(value.LocalID) || value.LocalByteCount <= 0 ||
		value.LocalByteCount > StreamTrackMaximumFileBytes || !validStreamTrackTitle(value.Title) {
		return false
	}
	switch value.Phase {
	case StreamTrackDraftRetained, StreamTrackDraftUploading, StreamTrackDraftUploaded,
		StreamTrackDraftProcessing, StreamTrackDraftReady, StreamTrackDraftFailed:
	default:
		value.Phase = StreamTrackDraftFailed
		value.Failure = StreamTrackServiceUnavailable
	}
	if value.UploadOffset < 0 {
		value.UploadOffset = 0
	}
	if value.UploadOffset > value.LocalByteCount {
		value.UploadOffset = value.LocalByteCount
	}
	if value.ProcessingPercent < 0 {
		value.ProcessingPercent = 0
	}
	if value.ProcessingPercent > 100 {
		value.ProcessingPercent = 100
	}
	if value.HasDuration {
		value.DurationMS = clampStreamTrackDuration(value.DurationMS)
	}
	if value.Phase == StreamTrackDraftReady && (!value.ServerMetadataConfirmed || value.MediaID == "" || value.VariantManifest == "") {
		value.Phase = StreamTrackDraftProcessing
		value.Failure = ""
	}
	value.Failure = canonicalStreamTrackFailure(value.Failure)
	if !validStreamTrackLabel(value.PhaseLabel, "stream_track.draft."+string(value.Phase)) {
		value.PhaseLabel = TargetsInboxLocalizedLabel{}
	}
	if value.Failure == "" || !validStreamTrackLabel(value.FailureLabel, "stream_track.failure."+string(value.Failure)) {
		value.FailureLabel = TargetsInboxLocalizedLabel{}
	}
	return true
}

func normalizeStreamTrackPlayback(value, current StreamTrackPlayback) StreamTrackPlayback {
	if value.PlaybackGeneration < current.PlaybackGeneration ||
		value.PlaybackGeneration == current.PlaybackGeneration && value.SeekGeneration < current.SeekGeneration {
		return current
	}
	switch value.Phase {
	case StreamTrackPlaybackIdle, StreamTrackPlaybackQueued, StreamTrackPlaybackLoading,
		StreamTrackPlaybackReady, StreamTrackPlaybackPlaying, StreamTrackPlaybackPaused,
		StreamTrackPlaybackSeeking, StreamTrackPlaybackRebuffering, StreamTrackPlaybackEnded,
		StreamTrackPlaybackFailed:
	default:
		value.Phase = StreamTrackPlaybackFailed
		value.Failure = StreamTrackServiceUnavailable
	}
	value.DurationMS = clampStreamTrackDuration(value.DurationMS)
	if value.AudiblePositionMS < 0 {
		value.AudiblePositionMS = 0
	}
	if value.AudiblePositionMS > value.DurationMS {
		value.AudiblePositionMS = value.DurationMS
	}
	if value.PlaybackGeneration == current.PlaybackGeneration && value.SeekGeneration == current.SeekGeneration &&
		value.AudiblePositionMS < current.AudiblePositionMS {
		value.AudiblePositionMS = current.AudiblePositionMS
	}
	if value.Phase != StreamTrackPlaybackIdle && value.StreamID == "" {
		value.Phase = StreamTrackPlaybackFailed
		value.Failure = StreamTrackVariantUnavailable
	}
	value.Failure = canonicalStreamTrackFailure(value.Failure)
	if !validStreamTrackLabel(value.PhaseLabel, "stream_track.playback."+string(value.Phase)) {
		value.PhaseLabel = TargetsInboxLocalizedLabel{}
	}
	if value.Failure == "" || !validStreamTrackLabel(value.FailureLabel, "stream_track.failure."+string(value.Failure)) {
		value.FailureLabel = TargetsInboxLocalizedLabel{}
	}
	return value
}

func canonicalStreamTrackTargets(values []TargetsInboxTargetChoice, now time.Time) []TargetsInboxTargetChoice {
	seen := map[string]bool{}
	result := make([]TargetsInboxTargetChoice, 0, len(values))
	for _, target := range values {
		if !targetReferencePattern.MatchString(target.Reference) || !target.ExpiresAt.After(now) || seen[target.Reference] {
			continue
		}
		seen[target.Reference] = true
		target.Capabilities = canonicalTargetsInboxEnums(target.Capabilities)
		result = append(result, target)
	}
	return result
}

func canonicalStreamTrackActions(values []TargetsInboxActionCapability) []TargetsInboxActionCapability {
	allowed := map[string]bool{
		"accept_policy": true, "upload": true, "retry": true, "delete": true,
		"queue": true, "replace": true, "pause": true, "seek": true, "resume": true, "report": true,
	}
	seen := map[string]bool{}
	result := make([]TargetsInboxActionCapability, 0, len(values))
	for _, value := range values {
		if allowed[value.Action] && !seen[value.Action] &&
			value.Label.Key == "stream_track.action."+value.Action && value.Label.EN != "" && value.Label.RU != "" {
			seen[value.Action] = true
			result = append(result, value)
		}
	}
	return result
}

func (model *StreamTrackModel) BuildCommand(request StreamTrackCommand) (StreamTrackCommand, bool) {
	snapshot := model.Snapshot()
	return buildStreamTrackCommand(snapshot, request)
}

func buildStreamTrackCommand(snapshot StreamTrackSnapshot, request StreamTrackCommand) (StreamTrackCommand, bool) {
	ready := snapshot.State == TargetsInboxReady
	hasAction := func(action string) bool { return targetsInboxHasAction(snapshot.Actions, action) }
	switch request.Kind {
	case StreamTrackAcceptPolicy:
		return request, ready && hasAction("accept_policy") &&
			(snapshot.ContentPolicyState == "required" || snapshot.ContentPolicyState == "stale")
	case StreamTrackUpload:
		return request, ready && hasAction("upload") && snapshot.ContentPolicyState == "current" &&
			snapshot.Draft != nil && snapshot.Draft.LocalID == request.LocalID &&
			snapshot.Draft.Phase == StreamTrackDraftRetained && snapshot.Draft.RetainedLocalBytes
	case StreamTrackRetry:
		return request, ready && hasAction("retry") && snapshot.Draft != nil && snapshot.Draft.LocalID == request.LocalID &&
			(snapshot.Draft.Phase == StreamTrackDraftFailed || snapshot.Playback.Phase == StreamTrackPlaybackFailed)
	case StreamTrackDelete:
		return request, ready && request.Confirmed && hasAction("delete") && snapshot.Draft != nil && snapshot.Draft.LocalID == request.LocalID
	case StreamTrackQueueCommand, StreamTrackReplaceCommand:
		if request.Kind == StreamTrackReplaceCommand && snapshot.Playback.PlaybackGeneration == ^uint64(0) {
			return StreamTrackCommand{}, false
		}
		return buildStreamTrackDeliveryCommand(snapshot, request)
	case StreamTrackPause:
		return request, hasAction("pause") && snapshot.Playback.Phase == StreamTrackPlaybackPlaying && exactStreamTrackPlayback(snapshot, request)
	case StreamTrackSeek:
		phaseOK := snapshot.Playback.Phase == StreamTrackPlaybackReady || snapshot.Playback.Phase == StreamTrackPlaybackPlaying ||
			snapshot.Playback.Phase == StreamTrackPlaybackPaused || snapshot.Playback.Phase == StreamTrackPlaybackRebuffering
		return request, hasAction("seek") && phaseOK && exactStreamTrackPlayback(snapshot, request) &&
			request.SeekGeneration == snapshot.Playback.SeekGeneration && request.SeekGeneration < ^uint64(0) &&
			request.PositionMS >= 0 && request.PositionMS <= snapshot.Playback.DurationMS
	case StreamTrackResume:
		phaseOK := snapshot.Playback.Phase == StreamTrackPlaybackReady || snapshot.Playback.Phase == StreamTrackPlaybackPaused ||
			snapshot.Playback.Phase == StreamTrackPlaybackRebuffering
		return request, hasAction("resume") && phaseOK && exactStreamTrackPlayback(snapshot, request)
	case StreamTrackReport:
		return request, ready && hasAction("report") && snapshot.Draft != nil && snapshot.Draft.MediaID == request.MediaID &&
			validStreamTrackDetails(request.Details)
	default:
		return StreamTrackCommand{}, false
	}
}

func buildStreamTrackDeliveryCommand(snapshot StreamTrackSnapshot, request StreamTrackCommand) (StreamTrackCommand, bool) {
	action := string(request.Kind)
	if snapshot.State != TargetsInboxReady || snapshot.ContentPolicyState != "current" ||
		!targetsInboxHasAction(snapshot.Actions, action) || snapshot.Draft == nil ||
		snapshot.Draft.Phase != StreamTrackDraftReady || snapshot.Draft.MediaID != request.MediaID ||
		!snapshot.Draft.ServerMetadataConfirmed || snapshot.Draft.VariantManifest == "" ||
		snapshot.SelectedAudience != request.Audience || string(snapshot.SelectedInsertion) != action {
		return StreamTrackCommand{}, false
	}
	if request.Audience == StreamTrackCurrentAir {
		return request, snapshot.ActiveAirAvailable && len(request.Targets) == 0
	}
	if request.Audience != StreamTrackExplicit || len(request.Targets) == 0 || len(request.Targets) > StreamTrackMaximumTargets ||
		!equalStringSlices(request.Targets, snapshot.SelectedReferences) {
		return StreamTrackCommand{}, false
	}
	byReference := make(map[string]TargetsInboxTargetChoice, len(snapshot.Targets))
	for _, target := range snapshot.Targets {
		byReference[target.Reference] = target
	}
	seen := map[string]bool{}
	for _, reference := range request.Targets {
		target, ok := byReference[reference]
		if !ok || seen[reference] || !containsString(target.Capabilities, "stream_track") {
			return StreamTrackCommand{}, false
		}
		seen[reference] = true
	}
	request.Targets = append([]string(nil), request.Targets...)
	return request, true
}

func (model *StreamTrackModel) ApplyOptimistic(request StreamTrackCommand) bool {
	if model == nil {
		return false
	}
	model.mu.Lock()
	defer model.mu.Unlock()
	command, ok := buildStreamTrackCommand(cloneStreamTrackSnapshot(model.snapshot), request)
	if !ok {
		return false
	}
	switch command.Kind {
	case StreamTrackUpload:
		model.snapshot.Draft.Phase = StreamTrackDraftUploading
		model.snapshot.Draft.PhaseLabel = TargetsInboxLocalizedLabel{}
	case StreamTrackRetry:
		if model.snapshot.Draft.Phase == StreamTrackDraftFailed {
			model.snapshot.Draft.Phase = StreamTrackDraftRetained
			model.snapshot.Draft.PhaseLabel = TargetsInboxLocalizedLabel{}
		}
		if model.snapshot.Playback.Phase == StreamTrackPlaybackFailed {
			model.snapshot.Playback.Phase = StreamTrackPlaybackLoading
			model.snapshot.Playback.PhaseLabel = TargetsInboxLocalizedLabel{}
		}
	case StreamTrackQueueCommand:
		model.snapshot.Playback.Phase = StreamTrackPlaybackQueued
		model.snapshot.Playback.PhaseLabel = TargetsInboxLocalizedLabel{}
	case StreamTrackReplaceCommand:
		model.snapshot.Playback.Phase = StreamTrackPlaybackLoading
		model.snapshot.Playback.PhaseLabel = TargetsInboxLocalizedLabel{}
		model.snapshot.Playback.PlaybackGeneration++
		model.snapshot.Playback.SeekGeneration = 0
		model.snapshot.Playback.AudiblePositionMS = 0
	case StreamTrackPause:
		model.snapshot.Playback.Phase = StreamTrackPlaybackPaused
		model.snapshot.Playback.PhaseLabel = TargetsInboxLocalizedLabel{}
	case StreamTrackSeek:
		model.snapshot.Playback.Phase = StreamTrackPlaybackSeeking
		model.snapshot.Playback.PhaseLabel = TargetsInboxLocalizedLabel{}
		model.snapshot.Playback.SeekGeneration++
		model.snapshot.Playback.AudiblePositionMS = command.PositionMS
	case StreamTrackResume:
		model.snapshot.Playback.Phase = StreamTrackPlaybackPlaying
		model.snapshot.Playback.PhaseLabel = TargetsInboxLocalizedLabel{}
	}
	// Consent, delete and report wait for coordinator confirmation.
	return true
}

func cloneStreamTrackSnapshot(value StreamTrackSnapshot) StreamTrackSnapshot {
	if value.Draft != nil {
		draft := *value.Draft
		value.Draft = &draft
	}
	value.Targets = append([]TargetsInboxTargetChoice(nil), value.Targets...)
	for index := range value.Targets {
		value.Targets[index].Capabilities = append([]string(nil), value.Targets[index].Capabilities...)
	}
	value.SelectedReferences = append([]string(nil), value.SelectedReferences...)
	value.Actions = append([]TargetsInboxActionCapability(nil), value.Actions...)
	return value
}

func exactStreamTrackPlayback(snapshot StreamTrackSnapshot, request StreamTrackCommand) bool {
	return snapshot.State == TargetsInboxReady && request.StreamID != "" && request.StreamID == snapshot.Playback.StreamID &&
		request.PlaybackGeneration == snapshot.Playback.PlaybackGeneration
}

func validStreamTrackTitle(value string) bool {
	return value != "" && value == strings.TrimSpace(value) && utf8.ValidString(value) && len([]byte(value)) <= 512
}

func validStreamTrackDetails(value string) bool {
	return value == strings.TrimSpace(value) && utf8.ValidString(value) && len([]byte(value)) <= 2000
}

func validStreamTrackLabel(value TargetsInboxLocalizedLabel, key string) bool {
	return value.Key == key && value.EN != "" && value.RU != ""
}

func clampStreamTrackDuration(value int64) int64 {
	if value < 0 {
		return 0
	}
	if value > StreamTrackMaximumDurationMS {
		return StreamTrackMaximumDurationMS
	}
	return value
}

func canonicalStreamTrackFailure(value StreamTrackFailure) StreamTrackFailure {
	switch value {
	case "", StreamTrackOffline, StreamTrackQuotaExceeded, StreamTrackUnsupportedTargets,
		StreamTrackPolicyRequired, StreamTrackProcessingFailed, StreamTrackVariantUnavailable,
		StreamTrackStaleGeneration, StreamTrackServiceUnavailable:
		return value
	default:
		return StreamTrackServiceUnavailable
	}
}

func containsString(values []string, wanted string) bool {
	index := sort.SearchStrings(values, wanted)
	return index < len(values) && values[index] == wanted
}

func equalStringSlices(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}
