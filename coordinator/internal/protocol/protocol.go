// Package protocol implements the coordinator<->node wire protocol v1 (spec ch. 8).
// Golden files in protocol/golden/ are the contract; see docs/protocol.md.
package protocol

import "encoding/json"

const Version = 1

const CapabilitySeamlessAdoption = "seamless_adoption_v1"

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
	TypeSetOffset  = "set_offset"
	TypeOffsetTest = "offset_test"
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
	TypeUserPause  = "user_pause"
	TypeUserResume = "user_resume"
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

// --- Node -> coordinator payloads ---

type RegisterPayload struct {
	NodeID           string   `json:"node_id"`
	Token            string   `json:"token"`
	AppVersion       string   `json:"app_version"`
	LibrespotVersion string   `json:"librespot_version"`
	Capabilities     []string `json:"capabilities,omitempty"`
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
