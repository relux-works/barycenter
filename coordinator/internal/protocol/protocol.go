// Package protocol implements the coordinator<->node wire protocol v1 (spec ch. 8).
// Golden files in protocol/golden/ are the contract; see docs/protocol.md.
package protocol

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const Version = 1

const (
	StreamMinimumBufferedMS  int64 = 2000
	StreamLoadReadyTimeoutMS int64 = 5000
	StreamSeekReadyTimeoutMS int64 = 3000
	StreamStartDeadlineMS    int64 = 5000
)

const (
	StreamMixedVersionRequireAll                = "require_all"
	StreamMixedVersionSupportedOnlyWithReceipts = "supported_only_with_receipts"
)

const (
	CapabilityInterruptResume  = "interrupt_resume_v1"
	CapabilityLivePTT          = LivePTTCapability
	CapabilityMediaClip        = "media_clip_v1"
	CapabilityOverlayMix       = "overlay_mix_v1"
	CapabilitySeamlessAdoption = "seamless_adoption_v1"
	CapabilityStreamTrack      = "stream_track_v1"
)

// CapabilitySet is an immutable, validated register capability snapshot.
// Values remain available for diagnostics even when this build does not know
// them; Supports is the only input to feature decisions.
type CapabilitySet struct {
	values []string
}

// ParseCapabilitySet requires the register wire invariant: non-empty printable
// ASCII names in strictly increasing byte order. Strict ordering rejects both
// duplicates and unstable encodings while retaining unknown additive names.
func ParseCapabilitySet(values []string) (CapabilitySet, error) {
	copyValues := append([]string(nil), values...)
	for i, value := range copyValues {
		if value == "" {
			return CapabilitySet{}, fmt.Errorf("capability %d is empty", i)
		}
		for _, b := range []byte(value) {
			if b < 0x21 || b > 0x7e {
				return CapabilitySet{}, fmt.Errorf("capability %d is not printable ASCII", i)
			}
		}
		if i > 0 && copyValues[i-1] >= value {
			return CapabilitySet{}, fmt.Errorf("capabilities are not unique and ASCII-sorted")
		}
	}
	return CapabilitySet{values: copyValues}, nil
}

// Supports reports whether the exact capability was advertised.
func (s CapabilitySet) Supports(capability string) bool {
	i := sort.SearchStrings(s.values, capability)
	return i < len(s.values) && s.values[i] == capability
}

// Values returns a defensive copy in canonical wire order.
func (s CapabilitySet) Values() []string {
	return append([]string(nil), s.values...)
}

type NodeID string

const (
	NodeA NodeID = "a"
	NodeB NodeID = "b"
)

// Coordinator -> node message types.
const (
	TypeWelcome    = "welcome"
	TypeLoad       = "load"
	TypeResumeAt   = "resume_at"
	TypePause      = "pause"
	TypeSeek       = "seek"
	TypePlayVoice  = "play_voice"
	TypeWait       = "wait"
	TypeSetVolume  = "set_volume"
	TypeSetMode    = "set_mode"
	TypeStop       = "stop"
	TypeSoloInject = "solo_inject"
	TypeSoloVoice  = "solo_voice"
	TypePong       = "pong"
	// v1 additions beyond the spec ch. 8 catalog (docs/protocol.md):
	TypeSetOffset      = "set_offset"
	TypeOffsetTest     = "offset_test"
	TypePrepareMedia   = "prepare_media"
	TypePlayMediaAt    = "play_media_at"
	TypeCancelMedia    = "cancel_media"
	TypePresenceUpdate = "presence_update"
	TypeStreamLoad     = "stream_load"
	TypeStreamResumeAt = "stream_resume_at"
	TypeStreamSeek     = "stream_seek"
	TypeStreamPause    = "stream_pause"
	TypeStreamCancel   = "stream_cancel"
	// live_ptt_v1 signalling is bidirectional; direction is constrained by the
	// session state machine rather than by distinct envelope names.
	TypeLivePTTStart   = "live_ptt_start"
	TypeLivePTTAccept  = "live_ptt_accept"
	TypeLivePTTReject  = "live_ptt_reject"
	TypeLivePTTEnd     = "live_ptt_end"
	TypeLivePTTCancel  = "live_ptt_cancel"
	TypeLivePTTFailed  = "live_ptt_failed"
	TypeLivePTTReceipt = "live_ptt_receipt"
	TypeLivePTTState   = "live_ptt_state"
)

// Node -> coordinator message types.
const (
	TypeRegister     = "register"
	TypeState        = "state"
	TypeReady        = "ready"
	TypeStarted      = "started"
	TypeEnded        = "ended"
	TypeVoiceStarted = "voice_started"
	TypeVoiceEnded   = "voice_ended"
	TypeWaitEnded    = "wait_ended"
	TypeError        = "error"
	TypePing         = "ping"
	// U9: node saw daemon playback not belonging to the broadcast (phone).
	TypeExternalPlayback = "external_playback"
	// v1.1 (spec-providers §7): switch a node's active provider.
	TypeSetProvider = "set_provider"
	// Personal pause (2026-07-10): the user paused/resumed THIS Pulsar via
	// the Spotify app. Pause detaches only this home from the shared air
	// (the broadcast keeps playing for the others); resume catches it back
	// up at the live position.
	TypeUserPause       = "user_pause"
	TypeUserResume      = "user_resume"
	TypeMediaReady      = "media_ready"
	TypeMediaStarted    = "media_started"
	TypeMediaEnded      = "media_ended"
	TypeMediaFailed     = "media_failed"
	TypeMediaCancelled  = "media_cancelled"
	TypeSetDND          = "set_dnd"
	TypeStreamReady     = "stream_ready"
	TypeStreamStarted   = "stream_started"
	TypeStreamProgress  = "stream_progress"
	TypeStreamRebuffer  = "stream_rebuffer"
	TypeStreamFailed    = "stream_failed"
	TypeStreamEnded     = "stream_ended"
	TypeStreamCancelled = "stream_cancelled"
)

type Envelope struct {
	V       int             `json:"v"`
	ID      string          `json:"id"`
	TS      int64           `json:"ts"`
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload"`
}

// --- Coordinator -> node payloads ---

type SessionCurrent struct {
	ElementID  string `json:"element_id"`
	Kind       string `json:"kind"`
	URI        string `json:"uri,omitempty"`
	PositionMS int64  `json:"position_ms"`
}

type SessionSnapshot struct {
	Mode    string          `json:"mode"`
	State   string          `json:"state"`
	Current *SessionCurrent `json:"current"`
	Volume  int             `json:"volume"`
}

type WelcomePayload struct {
	SessionSnapshot SessionSnapshot `json:"session_snapshot"`
}

type LoadPayload struct {
	ElementID  string `json:"element_id"`
	URI        string `json:"uri"` // stays = spotify ref for pre-v1.1 nodes
	PositionMS int64  `json:"position_ms"`
	// v1.1 additive (spec-providers §7); absent provider = "spotify".
	Provider   string  `json:"provider,omitempty"`
	Ref        string  `json:"ref,omitempty"`
	DurationMS int64   `json:"duration_ms,omitempty"`
	GainDB     float64 `json:"gain_db,omitempty"` // loudness fallback (S5 option c)
	// AdoptPlaying relabels playback that the user already started on this
	// Pulsar. The node must not pause, clear or reload its daemon.
	AdoptPlaying bool `json:"adopt_playing,omitempty"`
}

type ResumeAtPayload struct {
	ElementID string `json:"element_id"`
	TCoordMS  int64  `json:"t_coord_ms"`
	// PositionMS is present for a catch-up start. The node seeks while paused,
	// then resumes at T; ordinary synchronized starts omit it.
	PositionMS *int64 `json:"position_ms,omitempty"`
}

type PausePayload struct {
	ElementID string `json:"element_id"`
	FadeMS    int64  `json:"fade_ms"`
}

type SeekPayload struct {
	ElementID  string `json:"element_id"`
	PositionMS int64  `json:"position_ms"`
}

type PlayVoicePayload struct {
	ElementID string `json:"element_id"`
	FileURL   string `json:"file_url"`
	TCoordMS  *int64 `json:"t_coord_ms,omitempty"`
}

type WaitPayload struct {
	ElementID  string `json:"element_id"`
	DurationMS int64  `json:"duration_ms"`
}

type SetVolumePayload struct {
	Volume int `json:"volume"`
}

type SetModePayload struct {
	Mode string `json:"mode"`
}

type StopPayload struct{}

type SoloInjectPayload struct {
	URI string `json:"uri"`
	// v1.1 additive:
	Provider string `json:"provider,omitempty"`
	Ref      string `json:"ref,omitempty"`
	CTID     string `json:"ctid,omitempty"`
}

type SoloVoicePayload struct {
	ElementID string `json:"element_id"`
	FileURL   string `json:"file_url"`
}

type PongPayload struct {
	T1 int64 `json:"t1"`
	T2 int64 `json:"t2"`
	T3 int64 `json:"t3"`
}

type SetOffsetPayload struct {
	OffsetMS int64 `json:"offset_ms"`
}

type OffsetTestPayload struct {
	TCoordMS   int64 `json:"t_coord_ms"`
	Clicks     int   `json:"clicks"`
	IntervalMS int64 `json:"interval_ms"`
}

type PrepareMediaPayload struct {
	TransmissionID         string `json:"transmission_id"`
	Generation             int64  `json:"generation"`
	MediaID                string `json:"media_id"`
	Kind                   string `json:"kind"`
	Delivery               string `json:"delivery"`
	FileURL                string `json:"file_url"`
	SHA256                 string `json:"sha256"`
	SizeBytes              int64  `json:"size_bytes"`
	DurationMS             int64  `json:"duration_ms"`
	MediaExpiresAtCoordMS  int64  `json:"media_expires_at_coord_ms"`
	PrepareDeadlineCoordMS int64  `json:"prepare_deadline_coord_ms"`
}

type PlayMediaAtPayload struct {
	TransmissionID       string   `json:"transmission_id"`
	Generation           int64    `json:"generation"`
	TCoordMS             int64    `json:"t_coord_ms"`
	StartDeadlineCoordMS int64    `json:"start_deadline_coord_ms"`
	Delivery             string   `json:"delivery"`
	DuckDB               *float64 `json:"duck_db,omitempty"`
	AttackMS             *int64   `json:"attack_ms,omitempty"`
	ReleaseMS            *int64   `json:"release_ms,omitempty"`
	FadeOutMS            *int64   `json:"fade_out_ms,omitempty"`
	FadeInMS             *int64   `json:"fade_in_ms,omitempty"`
}

type CancelMediaPayload struct {
	TransmissionID string `json:"transmission_id"`
	Generation     int64  `json:"generation"`
	Reason         string `json:"reason"`
	Action         string `json:"action"`
	ResumeMain     bool   `json:"resume_main"`
	FadeMS         int64  `json:"fade_ms"`
}

type PresenceNode struct {
	OrbitID              int64    `json:"orbit_id"`
	Slot                 string   `json:"slot"`
	Online               bool     `json:"online"`
	LastSeenAtCoordMS    int64    `json:"last_seen_at_coord_ms"`
	OutputState          string   `json:"output_state"`
	PlaybackState        string   `json:"playback_state"`
	DNDMode              string   `json:"dnd_mode"`
	DNDRevision          int64    `json:"dnd_revision"`
	DNDUntilCoordMS      *int64   `json:"dnd_until_coord_ms,omitempty"`
	Capabilities         []string `json:"capabilities"`
	InterruptResumeReady bool     `json:"interrupt_resume_ready"`
}

type PresenceUpdatePayload struct {
	Revision           int64          `json:"revision"`
	GeneratedAtCoordMS int64          `json:"generated_at_coord_ms"`
	Nodes              []PresenceNode `json:"nodes"`
}

// Streamed-track commands are additive protocol-v1 messages. VariantManifest
// is an opaque server-issued value: clients do not negotiate codecs, derive
// storage keys or place credentials in VariantURL. Every command is ordered by
// CommandSequence inside an exact playback/seek generation.
type StreamLoadPayload struct {
	StreamID             string `json:"stream_id"`
	PlaybackGeneration   int64  `json:"playback_generation"`
	SeekGeneration       int64  `json:"seek_generation"`
	CommandSequence      int64  `json:"command_sequence"`
	MediaID              string `json:"media_id"`
	VariantManifest      string `json:"variant_manifest"`
	VariantURL           string `json:"variant_url"`
	VariantETag          string `json:"variant_etag"`
	VariantSHA256        string `json:"variant_sha256"`
	VariantSizeBytes     int64  `json:"variant_size_bytes"`
	StartPositionMS      int64  `json:"start_position_ms"`
	MinimumBufferedMS    int64  `json:"minimum_buffered_ms"`
	ReadyDeadlineCoordMS int64  `json:"ready_deadline_coord_ms"`
	MixedVersionPolicy   string `json:"mixed_version_policy"`
}

type StreamResumeAtPayload struct {
	StreamID             string `json:"stream_id"`
	PlaybackGeneration   int64  `json:"playback_generation"`
	SeekGeneration       int64  `json:"seek_generation"`
	CommandSequence      int64  `json:"command_sequence"`
	TCoordMS             int64  `json:"t_coord_ms"`
	StartDeadlineCoordMS int64  `json:"start_deadline_coord_ms"`
}

type StreamSeekPayload struct {
	StreamID             string `json:"stream_id"`
	PlaybackGeneration   int64  `json:"playback_generation"`
	SeekGeneration       int64  `json:"seek_generation"`
	CommandSequence      int64  `json:"command_sequence"`
	PositionMS           int64  `json:"position_ms"`
	MinimumBufferedMS    int64  `json:"minimum_buffered_ms"`
	ReadyDeadlineCoordMS int64  `json:"ready_deadline_coord_ms"`
}

type StreamPausePayload struct {
	StreamID           string `json:"stream_id"`
	PlaybackGeneration int64  `json:"playback_generation"`
	SeekGeneration     int64  `json:"seek_generation"`
	CommandSequence    int64  `json:"command_sequence"`
	FadeMS             int64  `json:"fade_ms"`
}

type StreamCancelPayload struct {
	StreamID           string `json:"stream_id"`
	PlaybackGeneration int64  `json:"playback_generation"`
	SeekGeneration     int64  `json:"seek_generation"`
	CommandSequence    int64  `json:"command_sequence"`
	Reason             string `json:"reason"`
}

func ValidateStreamLoadPayload(payload StreamLoadPayload) error {
	if payload.StreamID == "" || payload.MediaID == "" || payload.PlaybackGeneration <= 0 ||
		payload.SeekGeneration != 0 || payload.CommandSequence != 1 ||
		!strings.HasPrefix(payload.VariantManifest, "svm1.") || len(payload.VariantManifest) > 512 ||
		strings.Trim(payload.VariantManifest, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._-") != "" ||
		payload.VariantSizeBytes <= 0 || payload.StartPositionMS < 0 ||
		payload.MinimumBufferedMS != StreamMinimumBufferedMS || payload.ReadyDeadlineCoordMS <= 0 {
		return fmt.Errorf("invalid stream_load identity, generation, size or timing")
	}
	if len(payload.VariantSHA256) != 64 || strings.Trim(payload.VariantSHA256, "0123456789abcdef") != "" ||
		payload.VariantETag != `"sha256-`+payload.VariantSHA256+`"` {
		return fmt.Errorf("invalid stream_load integrity")
	}
	if !strings.HasPrefix(payload.VariantURL, "/v1/media/"+payload.MediaID+"/variants/") ||
		strings.ContainsAny(payload.VariantURL, "?#@") || strings.Contains(payload.VariantURL, "://") {
		return fmt.Errorf("stream_load variant_url must be credential-free and coordinator-relative")
	}
	if payload.MixedVersionPolicy != StreamMixedVersionRequireAll &&
		payload.MixedVersionPolicy != StreamMixedVersionSupportedOnlyWithReceipts {
		return fmt.Errorf("invalid stream_load mixed-version policy")
	}
	return nil
}

func ValidateStreamReadyPayload(payload StreamReadyPayload) error {
	if payload.StreamID == "" || payload.PlaybackGeneration <= 0 || payload.SeekGeneration < 0 ||
		payload.EventSequence <= 0 || payload.AudiblePositionMS < 0 ||
		payload.BufferedDurationMS < StreamMinimumBufferedMS {
		return fmt.Errorf("stream_ready is below the frozen buffer barrier")
	}
	return nil
}

// --- Node -> coordinator payloads ---

type RegisterPayload struct {
	NodeID           string   `json:"node_id"`
	Token            string   `json:"token"`
	AppVersion       string   `json:"app_version"`
	LibrespotVersion string   `json:"librespot_version"`
	Capabilities     []string `json:"capabilities,omitempty"`
}

type MediaReadyPayload struct {
	TransmissionID    string `json:"transmission_id"`
	Generation        int64  `json:"generation"`
	DecodedDurationMS int64  `json:"decoded_duration_ms"`
}

type MediaStartedPayload struct {
	TransmissionID      string `json:"transmission_id"`
	Generation          int64  `json:"generation"`
	TFirstSampleCoordMS int64  `json:"t_first_sample_coord_ms"`
}

type MediaEndedPayload struct {
	TransmissionID     string `json:"transmission_id"`
	Generation         int64  `json:"generation"`
	TLastSampleCoordMS int64  `json:"t_last_sample_coord_ms"`
	Reason             string `json:"reason"`
}

type MediaFailedPayload struct {
	TransmissionID string `json:"transmission_id"`
	Generation     int64  `json:"generation"`
	Stage          string `json:"stage"`
	Code           string `json:"code"`
}

type MediaCancelledPayload struct {
	TransmissionID string `json:"transmission_id"`
	Generation     int64  `json:"generation"`
	Reason         string `json:"reason"`
	Action         string `json:"action"`
	MainResumed    bool   `json:"main_resumed"`
}

type SetDNDPayload struct {
	Revision          int64  `json:"revision"`
	Mode              string `json:"mode"`
	MutedUntilCoordMS *int64 `json:"muted_until_coord_ms,omitempty"`
}

type StreamReadyPayload struct {
	StreamID           string `json:"stream_id"`
	PlaybackGeneration int64  `json:"playback_generation"`
	SeekGeneration     int64  `json:"seek_generation"`
	EventSequence      int64  `json:"event_sequence"`
	AudiblePositionMS  int64  `json:"audible_position_ms"`
	BufferedDurationMS int64  `json:"buffered_duration_ms"`
}

type StreamStartedPayload struct {
	StreamID            string `json:"stream_id"`
	PlaybackGeneration  int64  `json:"playback_generation"`
	SeekGeneration      int64  `json:"seek_generation"`
	EventSequence       int64  `json:"event_sequence"`
	AudiblePositionMS   int64  `json:"audible_position_ms"`
	TFirstSampleCoordMS int64  `json:"t_first_sample_coord_ms"`
}

type StreamProgressPayload struct {
	StreamID           string `json:"stream_id"`
	PlaybackGeneration int64  `json:"playback_generation"`
	SeekGeneration     int64  `json:"seek_generation"`
	EventSequence      int64  `json:"event_sequence"`
	AudiblePositionMS  int64  `json:"audible_position_ms"`
	BufferedDurationMS int64  `json:"buffered_duration_ms"`
}

type StreamRebufferPayload = StreamProgressPayload

type StreamFailedPayload struct {
	StreamID           string `json:"stream_id"`
	PlaybackGeneration int64  `json:"playback_generation"`
	SeekGeneration     int64  `json:"seek_generation"`
	EventSequence      int64  `json:"event_sequence"`
	Stage              string `json:"stage"`
	Code               string `json:"code"`
}

type StreamEndedPayload struct {
	StreamID           string `json:"stream_id"`
	PlaybackGeneration int64  `json:"playback_generation"`
	SeekGeneration     int64  `json:"seek_generation"`
	EventSequence      int64  `json:"event_sequence"`
	AudiblePositionMS  int64  `json:"audible_position_ms"`
	TLastSampleCoordMS int64  `json:"t_last_sample_coord_ms"`
	Reason             string `json:"reason"`
}

type StreamCancelledPayload struct {
	StreamID           string `json:"stream_id"`
	PlaybackGeneration int64  `json:"playback_generation"`
	SeekGeneration     int64  `json:"seek_generation"`
	EventSequence      int64  `json:"event_sequence"`
	AudiblePositionMS  int64  `json:"audible_position_ms"`
	Reason             string `json:"reason"`
}

type Speaker struct {
	Name      string `json:"name"`
	Connected bool   `json:"connected"`
}

type StatePayload struct {
	Playback   string    `json:"playback"`
	URI        *string   `json:"uri"`
	PositionMS int64     `json:"position_ms"`
	Volume     int       `json:"volume"`
	Degraded   bool      `json:"degraded"`
	Underruns  int64     `json:"underruns"`
	RTTMS      int64     `json:"rtt_ms"`
	Speakers   []Speaker `json:"speakers"`
	// v1.1: the node's active provider ("" = spotify).
	Provider string `json:"provider,omitempty"`
}

type ReadyPayload struct {
	ElementID string `json:"element_id"`
}

type StartedPayload struct {
	ElementID           string `json:"element_id"`
	TFirstSampleCoordMS int64  `json:"t_first_sample_coord_ms"`
}

type EndedPayload struct {
	ElementID string `json:"element_id"`
	Reason    string `json:"reason"`
}

type VoiceStartedPayload struct {
	ElementID string `json:"element_id"`
}

type VoiceEndedPayload struct {
	ElementID string `json:"element_id"`
}

type WaitEndedPayload struct {
	ElementID string `json:"element_id"`
}

type ErrorPayload struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	ElementID string `json:"element_id,omitempty"`
}

type PingPayload struct {
	T1 int64 `json:"t1"`
}

type ExternalPlaybackPayload struct {
	URI        string `json:"uri"`
	PositionMS *int64 `json:"position_ms,omitempty"`
	Title      string `json:"title,omitempty"`
}

// UserPausePayload / UserResumePayload: element_id is what the node believes
// is current — informational; the coordinator acts on its own current element
// (the node may resume days later, long past that element).
type UserPausePayload struct {
	ElementID string `json:"element_id"`
}

type UserResumePayload struct {
	ElementID string `json:"element_id"`
}

type SetProviderPayload struct {
	Provider string `json:"provider"`
}

type StreamGenerationDecision string

const (
	StreamGenerationApply     StreamGenerationDecision = "apply"
	StreamGenerationDuplicate StreamGenerationDecision = "duplicate"
	StreamGenerationStale     StreamGenerationDecision = "stale"
	StreamGenerationInvalid   StreamGenerationDecision = "invalid"
)

type StreamEventKind string

const (
	StreamEventReady     StreamEventKind = "ready"
	StreamEventStarted   StreamEventKind = "started"
	StreamEventProgress  StreamEventKind = "progress"
	StreamEventRebuffer  StreamEventKind = "rebuffer"
	StreamEventFailed    StreamEventKind = "failed"
	StreamEventEnded     StreamEventKind = "ended"
	StreamEventCancelled StreamEventKind = "cancelled"
)

// StreamGenerationGuard is the shared deterministic ordering rule used by the
// coordinator and candidate test clients. WebSocket delivery is ordered, so a
// command/event sequence gap is invalid rather than guessed through. A seek
// advances seek generation and resets event ordering. Terminal events close
// only their exact generation; late output can never close a newer one.
type StreamGenerationGuard struct {
	PlaybackGeneration int64
	SeekGeneration     int64
	CommandSequence    int64
	EventSequence      int64
	CommandKind        string
	EventKind          StreamEventKind
	Phase              string
}

func (g *StreamGenerationGuard) AcceptLoad(playbackGeneration, seekGeneration, commandSequence int64) StreamGenerationDecision {
	if playbackGeneration <= 0 || seekGeneration != 0 || commandSequence != 1 {
		return StreamGenerationInvalid
	}
	if playbackGeneration < g.PlaybackGeneration {
		return StreamGenerationStale
	}
	if playbackGeneration == g.PlaybackGeneration {
		if seekGeneration == g.SeekGeneration && commandSequence == g.CommandSequence && g.CommandKind == "load" {
			return StreamGenerationDuplicate
		}
		return StreamGenerationStale
	}
	g.PlaybackGeneration = playbackGeneration
	g.SeekGeneration = 0
	g.CommandSequence = commandSequence
	g.EventSequence = 0
	g.CommandKind = "load"
	g.EventKind = ""
	g.Phase = "loading"
	return StreamGenerationApply
}

func (g *StreamGenerationGuard) AcceptCommand(playbackGeneration, seekGeneration, commandSequence int64, command string) StreamGenerationDecision {
	if playbackGeneration != g.PlaybackGeneration || seekGeneration != g.SeekGeneration {
		return StreamGenerationStale
	}
	if commandSequence <= g.CommandSequence {
		if commandSequence == g.CommandSequence && command == g.CommandKind {
			return StreamGenerationDuplicate
		}
		if commandSequence == g.CommandSequence {
			return StreamGenerationInvalid
		}
		return StreamGenerationStale
	}
	if commandSequence != g.CommandSequence+1 || g.Phase == "terminal" {
		return StreamGenerationInvalid
	}
	switch command {
	case "resume":
		if g.Phase != "ready" && g.Phase != "paused_ready" {
			return StreamGenerationInvalid
		}
		g.Phase = "ready"
	case "pause":
		if g.Phase == "started" {
			g.Phase = "paused_ready"
		} else if g.Phase == "rebuffering" {
			g.Phase = "paused_loading"
		} else {
			return StreamGenerationInvalid
		}
	case "cancel":
	default:
		return StreamGenerationInvalid
	}
	g.CommandSequence = commandSequence
	g.CommandKind = command
	return StreamGenerationApply
}

func (g *StreamGenerationGuard) AcceptSeek(playbackGeneration, seekGeneration, commandSequence int64) StreamGenerationDecision {
	if playbackGeneration != g.PlaybackGeneration {
		return StreamGenerationStale
	}
	if seekGeneration <= g.SeekGeneration {
		if seekGeneration == g.SeekGeneration && commandSequence == g.CommandSequence && g.CommandKind == "seek" {
			return StreamGenerationDuplicate
		}
		return StreamGenerationStale
	}
	if seekGeneration != g.SeekGeneration+1 || commandSequence != g.CommandSequence+1 || g.Phase == "terminal" {
		return StreamGenerationInvalid
	}
	g.SeekGeneration = seekGeneration
	g.CommandSequence = commandSequence
	g.EventSequence = 0
	g.CommandKind = "seek"
	g.EventKind = ""
	g.Phase = "loading"
	return StreamGenerationApply
}

func (g *StreamGenerationGuard) AcceptEvent(playbackGeneration, seekGeneration, eventSequence int64, kind StreamEventKind) StreamGenerationDecision {
	if playbackGeneration != g.PlaybackGeneration || seekGeneration != g.SeekGeneration {
		return StreamGenerationStale
	}
	if eventSequence <= g.EventSequence {
		if eventSequence == g.EventSequence && kind == g.EventKind {
			return StreamGenerationDuplicate
		}
		if eventSequence == g.EventSequence {
			return StreamGenerationInvalid
		}
		return StreamGenerationStale
	}
	if eventSequence != g.EventSequence+1 || g.Phase == "terminal" {
		return StreamGenerationInvalid
	}
	switch kind {
	case StreamEventReady:
		if g.Phase != "loading" && g.Phase != "rebuffering" && g.Phase != "paused_loading" {
			return StreamGenerationInvalid
		}
		if g.Phase == "paused_loading" {
			g.Phase = "paused_ready"
		} else {
			g.Phase = "ready"
		}
	case StreamEventStarted:
		if g.Phase != "ready" {
			return StreamGenerationInvalid
		}
		g.Phase = "started"
	case StreamEventProgress:
		if g.Phase != "started" {
			return StreamGenerationInvalid
		}
	case StreamEventRebuffer:
		if g.Phase != "started" {
			return StreamGenerationInvalid
		}
		g.Phase = "rebuffering"
	case StreamEventFailed, StreamEventEnded, StreamEventCancelled:
		g.Phase = "terminal"
	default:
		return StreamGenerationInvalid
	}
	g.EventSequence = eventSequence
	g.EventKind = kind
	return StreamGenerationApply
}

func (g *StreamGenerationGuard) AcceptReady(playbackGeneration, seekGeneration, eventSequence, bufferedDurationMS, minimumBufferedMS int64) StreamGenerationDecision {
	if minimumBufferedMS != StreamMinimumBufferedMS || bufferedDurationMS < minimumBufferedMS {
		return StreamGenerationInvalid
	}
	return g.AcceptEvent(playbackGeneration, seekGeneration, eventSequence, StreamEventReady)
}
