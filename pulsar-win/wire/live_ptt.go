// Code mirrored from coordinator/internal/protocol — keep in sync via golden tests.
// Do not edit below this header: golden_test.go verifies both the wire contract
// (round-trip of every golden file) and byte-equality with the coordinator source.
package protocol

import (
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
)

const (
	LivePTTCapability                    = "live_ptt_v1"
	LivePTTCodecProfile                  = "opus-1.6.1-48k-mono-20ms-24k-cvbr-c5-fec2"
	LivePTTFrameHeaderBytes              = 40
	LivePTTMaxPayloadBytes               = 400
	LivePTTMaxMessageBytes               = LivePTTFrameHeaderBytes + LivePTTMaxPayloadBytes
	LivePTTFrameMS                       = 20
	LivePTTMaxFramesPerSecond            = 50
	LivePTTJitterBufferMS                = 60
	LivePTTMaxGapFrames           uint32 = 8
	LivePTTMaxDurationMS                 = 300000
	LivePTTAcceptTimeoutMS               = 1500
	LivePTTDrainTimeoutMS                = 600
	LivePTTMaxTargets                    = 64
	LivePTTMixedVersionRequireAll        = "require_all"
	LivePTTMixedVersionReceipts          = "supported_only_with_receipts"
	LivePTTLateJoinPolicy                = "frozen_targets_no_late_join"
	LivePTTCaptureAuthority              = "local_user_input_only"
)

const (
	LivePTTFlagStart byte = 1 << iota
	LivePTTFlagEnd
	LivePTTFlagFEC
	livePTTAllowedFlags = LivePTTFlagStart | LivePTTFlagEnd | LivePTTFlagFEC
)

type LivePTTStartPayload struct {
	SessionID             string `json:"session_id"`
	Generation            int64  `json:"generation"`
	SenderActorID         int64  `json:"sender_actor_id"`
	SenderOrbitID         int64  `json:"sender_orbit_id"`
	SenderNodeID          string `json:"sender_node_id"`
	TargetSnapshot        string `json:"target_snapshot"`
	TargetSHA256          string `json:"target_sha256"`
	TargetCount           int    `json:"target_count"`
	PlaybackDomain        string `json:"playback_domain"`
	PlaybackDomainID      int64  `json:"playback_domain_id"`
	CodecProfile          string `json:"codec_profile"`
	FrameMS               int    `json:"frame_ms"`
	MaxPayloadBytes       int    `json:"max_payload_bytes"`
	JitterBufferMS        int    `json:"jitter_buffer_ms"`
	StartedAtCoordMS      int64  `json:"started_at_coord_ms"`
	AcceptDeadlineCoordMS int64  `json:"accept_deadline_coord_ms"`
	MaxDurationMS         int64  `json:"max_duration_ms"`
	MixedVersionPolicy    string `json:"mixed_version_policy"`
	LateJoinPolicy        string `json:"late_join_policy"`
	CaptureAuthority      string `json:"capture_authority"`
}

type LivePTTAcceptPayload struct {
	SessionID         string `json:"session_id"`
	Generation        int64  `json:"generation"`
	EventSequence     int64  `json:"event_sequence"`
	AcceptedAtCoordMS int64  `json:"accepted_at_coord_ms"`
	LiveEdgeSequence  uint32 `json:"live_edge_sequence"`
	BufferFrames      int    `json:"buffer_frames"`
}

type LivePTTRejectPayload struct {
	SessionID         string `json:"session_id"`
	Generation        int64  `json:"generation"`
	EventSequence     int64  `json:"event_sequence"`
	Code              string `json:"code"`
	RejectedAtCoordMS int64  `json:"rejected_at_coord_ms"`
}

type LivePTTEndPayload struct {
	SessionID            string `json:"session_id"`
	Generation           int64  `json:"generation"`
	CommandSequence      int64  `json:"command_sequence"`
	LastSequence         uint32 `json:"last_sequence"`
	EndedAtCoordMS       int64  `json:"ended_at_coord_ms"`
	DrainDeadlineCoordMS int64  `json:"drain_deadline_coord_ms"`
	Reason               string `json:"reason"`
}

type LivePTTCancelPayload struct {
	SessionID          string `json:"session_id"`
	Generation         int64  `json:"generation"`
	CommandSequence    int64  `json:"command_sequence"`
	CancelledAtCoordMS int64  `json:"cancelled_at_coord_ms"`
	Reason             string `json:"reason"`
	DiscardBuffered    bool   `json:"discard_buffered"`
}

type LivePTTFailedPayload struct {
	SessionID       string `json:"session_id"`
	Generation      int64  `json:"generation"`
	EventSequence   int64  `json:"event_sequence"`
	Stage           string `json:"stage"`
	Code            string `json:"code"`
	FailedAtCoordMS int64  `json:"failed_at_coord_ms"`
}

type LivePTTReceiptPayload struct {
	SessionID         string `json:"session_id"`
	Generation        int64  `json:"generation"`
	EventSequence     int64  `json:"event_sequence"`
	State             string `json:"state"`
	LastSequence      uint32 `json:"last_sequence,omitempty"`
	ObservedAtCoordMS int64  `json:"observed_at_coord_ms"`
}

type LivePTTStatePayload struct {
	Revision           int64  `json:"revision"`
	Phase              string `json:"phase"`
	ActiveSessionID    string `json:"active_session_id,omitempty"`
	Generation         int64  `json:"generation,omitempty"`
	SpeakerActorID     int64  `json:"speaker_actor_id,omitempty"`
	LastSequence       uint32 `json:"last_sequence,omitempty"`
	GeneratedAtCoordMS int64  `json:"generated_at_coord_ms"`
}

func validLivePTTSessionID(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(value) == 32 && len(decoded) == 16 && value == strings.ToLower(value) &&
		value != "00000000000000000000000000000000"
}

func validLivePTTCommon(sessionID string, generation int64) bool {
	return validLivePTTSessionID(sessionID) && generation > 0
}

func validLivePTTToken(value, prefix string, maximum int) bool {
	return value != "" && strings.HasPrefix(value, prefix) && len(value) <= maximum &&
		strings.Trim(value, "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789._-") == ""
}

func validLivePTTCode(value string) bool {
	return value != "" && len(value) <= 64 && strings.Trim(value, "abcdefghijklmnopqrstuvwxyz0123456789_") == ""
}

func ValidateLivePTTStartPayload(payload LivePTTStartPayload) error {
	if !validLivePTTCommon(payload.SessionID, payload.Generation) || payload.SenderActorID <= 0 ||
		payload.SenderOrbitID <= 0 || !validLivePTTToken(payload.SenderNodeID, "", 64) ||
		!validLivePTTToken(payload.TargetSnapshot, "lts1.", 128) ||
		len(payload.TargetSHA256) != 64 || strings.Trim(payload.TargetSHA256, "0123456789abcdef") != "" ||
		payload.TargetCount < 1 || payload.TargetCount > LivePTTMaxTargets || payload.PlaybackDomainID <= 0 {
		return fmt.Errorf("invalid live_ptt_start identity or target snapshot")
	}
	if payload.PlaybackDomain != "personal" && payload.PlaybackDomain != "barycenter" && payload.PlaybackDomain != "air" {
		return fmt.Errorf("invalid live_ptt_start playback domain")
	}
	if payload.CodecProfile != LivePTTCodecProfile || payload.FrameMS != LivePTTFrameMS ||
		payload.MaxPayloadBytes != LivePTTMaxPayloadBytes || payload.JitterBufferMS != LivePTTJitterBufferMS ||
		payload.MaxDurationMS != LivePTTMaxDurationMS || payload.LateJoinPolicy != LivePTTLateJoinPolicy ||
		payload.CaptureAuthority != LivePTTCaptureAuthority {
		return fmt.Errorf("invalid live_ptt_start codec, bound or authority")
	}
	if payload.StartedAtCoordMS <= 0 || payload.AcceptDeadlineCoordMS <= payload.StartedAtCoordMS ||
		payload.AcceptDeadlineCoordMS-payload.StartedAtCoordMS > LivePTTAcceptTimeoutMS {
		return fmt.Errorf("invalid live_ptt_start deadline")
	}
	if payload.MixedVersionPolicy != LivePTTMixedVersionRequireAll &&
		payload.MixedVersionPolicy != LivePTTMixedVersionReceipts {
		return fmt.Errorf("invalid live_ptt_start mixed-version policy")
	}
	return nil
}

func ValidateLivePTTAcceptPayload(payload LivePTTAcceptPayload) error {
	if !validLivePTTCommon(payload.SessionID, payload.Generation) || payload.EventSequence != 1 ||
		payload.AcceptedAtCoordMS <= 0 || payload.LiveEdgeSequence != 1 || payload.BufferFrames != 3 {
		return fmt.Errorf("invalid live_ptt_accept")
	}
	return nil
}

func ValidateLivePTTRejectPayload(payload LivePTTRejectPayload) error {
	allowed := map[string]bool{"blocked": true, "busy": true, "dnd": true, "expired": true,
		"policy": true, "unauthorized": true, "unsupported": true}
	if !validLivePTTCommon(payload.SessionID, payload.Generation) || payload.EventSequence != 1 ||
		!allowed[payload.Code] || payload.RejectedAtCoordMS <= 0 {
		return fmt.Errorf("invalid live_ptt_reject")
	}
	return nil
}

func ValidateLivePTTEndPayload(payload LivePTTEndPayload) error {
	allowed := map[string]bool{"release": true, "lost_release": true, "lock": true, "sleep": true,
		"permission_revoked": true, "device_lost": true, "disconnect": true, "quit": true}
	if !validLivePTTCommon(payload.SessionID, payload.Generation) || payload.CommandSequence <= 0 ||
		payload.LastSequence == 0 || payload.EndedAtCoordMS <= 0 ||
		payload.DrainDeadlineCoordMS-payload.EndedAtCoordMS != LivePTTDrainTimeoutMS || !allowed[payload.Reason] {
		return fmt.Errorf("invalid live_ptt_end")
	}
	return nil
}

func ValidateLivePTTCancelPayload(payload LivePTTCancelPayload) error {
	allowed := map[string]bool{"backpressure": true, "coordinator_restart": true, "generation_replaced": true,
		"lost_release": true, "policy_changed": true, "sender_disconnect": true, "target_revoked": true,
		"timeout": true, "user_cancel": true}
	if !validLivePTTCommon(payload.SessionID, payload.Generation) || payload.CommandSequence <= 0 ||
		payload.CancelledAtCoordMS <= 0 || !allowed[payload.Reason] || !payload.DiscardBuffered {
		return fmt.Errorf("invalid live_ptt_cancel")
	}
	return nil
}

func ValidateLivePTTFailedPayload(payload LivePTTFailedPayload) error {
	allowedStages := map[string]bool{"capture": true, "decode": true, "frame": true, "jitter": true,
		"policy": true, "relay": true, "render": true, "transport": true}
	if !validLivePTTCommon(payload.SessionID, payload.Generation) || payload.EventSequence <= 0 ||
		!allowedStages[payload.Stage] || !validLivePTTCode(payload.Code) || payload.FailedAtCoordMS <= 0 {
		return fmt.Errorf("invalid live_ptt_failed")
	}
	return nil
}

func ValidateLivePTTReceiptPayload(payload LivePTTReceiptPayload) error {
	allowed := map[string]bool{"accepted": true, "audible_started": true, "cancelled": true, "ended": true,
		"failed": true, "rejected": true, "unsupported": true}
	if !validLivePTTCommon(payload.SessionID, payload.Generation) || payload.EventSequence <= 0 ||
		!allowed[payload.State] || payload.ObservedAtCoordMS <= 0 {
		return fmt.Errorf("invalid live_ptt_receipt")
	}
	return nil
}

func ValidateLivePTTStatePayload(payload LivePTTStatePayload) error {
	allowed := map[string]bool{"accepting": true, "cancelled": true, "ended": true, "idle": true,
		"receiving": true, "relaying": true, "starting": true, "terminal": true}
	if payload.Revision <= 0 || !allowed[payload.Phase] || payload.GeneratedAtCoordMS <= 0 {
		return fmt.Errorf("invalid live_ptt_state")
	}
	if payload.Phase == "idle" {
		if payload.ActiveSessionID != "" || payload.Generation != 0 || payload.SpeakerActorID != 0 || payload.LastSequence != 0 {
			return fmt.Errorf("idle live_ptt_state carries active identity")
		}
		return nil
	}
	if !validLivePTTCommon(payload.ActiveSessionID, payload.Generation) || payload.SpeakerActorID <= 0 {
		return fmt.Errorf("active live_ptt_state lacks identity")
	}
	return nil
}

func validateLivePTTPayload(payload any) error {
	switch value := payload.(type) {
	case *LivePTTStartPayload:
		return ValidateLivePTTStartPayload(*value)
	case *LivePTTAcceptPayload:
		return ValidateLivePTTAcceptPayload(*value)
	case *LivePTTRejectPayload:
		return ValidateLivePTTRejectPayload(*value)
	case *LivePTTEndPayload:
		return ValidateLivePTTEndPayload(*value)
	case *LivePTTCancelPayload:
		return ValidateLivePTTCancelPayload(*value)
	case *LivePTTFailedPayload:
		return ValidateLivePTTFailedPayload(*value)
	case *LivePTTReceiptPayload:
		return ValidateLivePTTReceiptPayload(*value)
	case *LivePTTStatePayload:
		return ValidateLivePTTStatePayload(*value)
	default:
		return nil
	}
}

type LivePTTBinaryFrame struct {
	Flags              byte
	SessionID          [16]byte
	Sequence           uint32
	CaptureMonotonicUS uint64
	Payload            []byte
}

func ParseLivePTTSessionID(value string) ([16]byte, error) {
	var session [16]byte
	if !validLivePTTSessionID(value) {
		return session, fmt.Errorf("invalid live PTT session id")
	}
	decoded, _ := hex.DecodeString(value)
	copy(session[:], decoded)
	return session, nil
}

func EncodeLivePTTBinaryFrame(frame LivePTTBinaryFrame) ([]byte, error) {
	if err := validateLivePTTBinaryFrame(frame); err != nil {
		return nil, err
	}
	result := make([]byte, LivePTTFrameHeaderBytes+len(frame.Payload))
	copy(result[0:2], []byte("BP"))
	result[2] = 1
	result[3] = frame.Flags
	copy(result[4:20], frame.SessionID[:])
	binary.BigEndian.PutUint32(result[20:24], frame.Sequence)
	binary.BigEndian.PutUint64(result[24:32], frame.CaptureMonotonicUS)
	binary.BigEndian.PutUint16(result[32:34], uint16(len(frame.Payload)))
	result[34], result[35], result[36], result[37] = LivePTTFrameMS, 1, 1, 1
	binary.BigEndian.PutUint16(result[38:40], 0)
	copy(result[40:], frame.Payload)
	return result, nil
}

func DecodeLivePTTBinaryFrame(raw []byte) (LivePTTBinaryFrame, error) {
	if len(raw) < LivePTTFrameHeaderBytes || len(raw) > LivePTTMaxMessageBytes {
		return LivePTTBinaryFrame{}, fmt.Errorf("live PTT frame length out of bounds")
	}
	if string(raw[0:2]) != "BP" || raw[2] != 1 || raw[34] != LivePTTFrameMS ||
		raw[35] != 1 || raw[36] != 1 || raw[37] != 1 || binary.BigEndian.Uint16(raw[38:40]) != 0 {
		return LivePTTBinaryFrame{}, fmt.Errorf("live PTT frame profile mismatch")
	}
	payloadBytes := int(binary.BigEndian.Uint16(raw[32:34]))
	if payloadBytes < 1 || payloadBytes > LivePTTMaxPayloadBytes || len(raw) != LivePTTFrameHeaderBytes+payloadBytes {
		return LivePTTBinaryFrame{}, fmt.Errorf("live PTT payload length mismatch")
	}
	var frame LivePTTBinaryFrame
	frame.Flags = raw[3]
	copy(frame.SessionID[:], raw[4:20])
	frame.Sequence = binary.BigEndian.Uint32(raw[20:24])
	frame.CaptureMonotonicUS = binary.BigEndian.Uint64(raw[24:32])
	frame.Payload = append([]byte(nil), raw[40:]...)
	if err := validateLivePTTBinaryFrame(frame); err != nil {
		return LivePTTBinaryFrame{}, err
	}
	return frame, nil
}

func validateLivePTTBinaryFrame(frame LivePTTBinaryFrame) error {
	if frame.Flags&^livePTTAllowedFlags != 0 || frame.Flags&LivePTTFlagFEC == 0 ||
		frame.Sequence == 0 || frame.CaptureMonotonicUS == 0 || len(frame.Payload) < 1 ||
		len(frame.Payload) > LivePTTMaxPayloadBytes {
		return fmt.Errorf("invalid live PTT flags, sequence, timestamp or payload")
	}
	if frame.SessionID == ([16]byte{}) {
		return fmt.Errorf("live PTT session id is zero")
	}
	if (frame.Sequence == 1) != (frame.Flags&LivePTTFlagStart != 0) {
		return fmt.Errorf("live PTT start flag and sequence disagree")
	}
	return nil
}

type LivePTTFrameDecision string

const (
	LivePTTFrameApply     LivePTTFrameDecision = "apply"
	LivePTTFrameDuplicate LivePTTFrameDecision = "duplicate"
	LivePTTFrameStale     LivePTTFrameDecision = "stale"
	LivePTTFrameInvalid   LivePTTFrameDecision = "invalid"
)

type LivePTTFrameGuard struct {
	SessionID       [16]byte
	Generation      int64
	LastSequence    uint32
	LastCaptureUS   uint64
	LastFrameSHA256 [32]byte
	Terminal        bool
}

func NewLivePTTFrameGuard(sessionID [16]byte, generation int64) LivePTTFrameGuard {
	return LivePTTFrameGuard{SessionID: sessionID, Generation: generation}
}

func (g *LivePTTFrameGuard) Accept(frame LivePTTBinaryFrame) LivePTTFrameDecision {
	if g.Generation <= 0 || frame.SessionID != g.SessionID || g.Terminal {
		return LivePTTFrameStale
	}
	encoded, err := EncodeLivePTTBinaryFrame(frame)
	if err != nil {
		return LivePTTFrameInvalid
	}
	digest := sha256.Sum256(encoded)
	if frame.Sequence <= g.LastSequence {
		if frame.Sequence == g.LastSequence && digest == g.LastFrameSHA256 {
			return LivePTTFrameDuplicate
		}
		return LivePTTFrameStale
	}
	if g.LastSequence == 0 {
		if frame.Sequence != 1 {
			return LivePTTFrameInvalid
		}
	} else {
		gap := frame.Sequence - g.LastSequence
		if gap > LivePTTMaxGapFrames || frame.CaptureMonotonicUS <= g.LastCaptureUS ||
			frame.CaptureMonotonicUS-g.LastCaptureUS != uint64(gap*LivePTTFrameMS*1000) {
			return LivePTTFrameInvalid
		}
	}
	if frame.Sequence > uint32(LivePTTMaxDurationMS/LivePTTFrameMS) {
		return LivePTTFrameInvalid
	}
	g.LastSequence = frame.Sequence
	g.LastCaptureUS = frame.CaptureMonotonicUS
	g.LastFrameSHA256 = digest
	g.Terminal = frame.Flags&LivePTTFlagEnd != 0
	return LivePTTFrameApply
}
