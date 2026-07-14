// Package protocol implements the coordinator<->node wire protocol v1 (spec ch. 8).
// Golden files in protocol/golden/ are the contract; see docs/protocol.md.
package protocol

import (
	"encoding/json"
	"fmt"
	"sort"
)

const Version = 1

const (
	CapabilityInterruptResume  = "interrupt_resume_v1"
	CapabilityMediaClip        = "media_clip_v1"
	CapabilityOverlayMix       = "overlay_mix_v1"
	CapabilitySeamlessAdoption = "seamless_adoption_v1"
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
	TypeUserPause      = "user_pause"
	TypeUserResume     = "user_resume"
	TypeMediaReady     = "media_ready"
	TypeMediaStarted   = "media_started"
	TypeMediaEnded     = "media_ended"
	TypeMediaFailed    = "media_failed"
	TypeMediaCancelled = "media_cancelled"
	TypeSetDND         = "set_dnd"
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
