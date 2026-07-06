// Code mirrored from coordinator/internal/protocol — keep in sync via golden tests.
// Do not edit below this header: golden_test.go verifies both the wire contract
// (round-trip of every golden file) and byte-equality with the coordinator source.
package protocol

import (
	"bytes"
	"encoding/json"
	"fmt"
)

var payloadFactory = map[string]func() any{
	TypeWelcome:          func() any { return &WelcomePayload{} },
	TypeLoad:             func() any { return &LoadPayload{} },
	TypeResumeAt:         func() any { return &ResumeAtPayload{} },
	TypePause:            func() any { return &PausePayload{} },
	TypeSeek:             func() any { return &SeekPayload{} },
	TypePlayVoice:        func() any { return &PlayVoicePayload{} },
	TypeWait:             func() any { return &WaitPayload{} },
	TypeSetVolume:        func() any { return &SetVolumePayload{} },
	TypeSetMode:          func() any { return &SetModePayload{} },
	TypeStop:             func() any { return &StopPayload{} },
	TypeSoloInject:       func() any { return &SoloInjectPayload{} },
	TypeSoloVoice:        func() any { return &SoloVoicePayload{} },
	TypePong:             func() any { return &PongPayload{} },
	TypeSetOffset:        func() any { return &SetOffsetPayload{} },
	TypeOffsetTest:       func() any { return &OffsetTestPayload{} },
	TypeRegister:         func() any { return &RegisterPayload{} },
	TypeState:            func() any { return &StatePayload{} },
	TypeReady:            func() any { return &ReadyPayload{} },
	TypeStarted:          func() any { return &StartedPayload{} },
	TypeEnded:            func() any { return &EndedPayload{} },
	TypeVoiceStarted:     func() any { return &VoiceStartedPayload{} },
	TypeVoiceEnded:       func() any { return &VoiceEndedPayload{} },
	TypeWaitEnded:        func() any { return &WaitEndedPayload{} },
	TypeError:            func() any { return &ErrorPayload{} },
	TypePing:             func() any { return &PingPayload{} },
	TypeExternalPlayback: func() any { return &ExternalPlaybackPayload{} },
	TypeSetProvider:      func() any { return &SetProviderPayload{} },
}

// KnownType reports whether t is a protocol v1 message type.
func KnownType(t string) bool {
	_, ok := payloadFactory[t]
	return ok
}

// DecodePayload decodes the payload of env into its typed struct,
// tolerating unknown fields (forward compatibility, spec 8.6).
func DecodePayload(env Envelope) (any, error) {
	return decode(env, false)
}

// DecodePayloadStrict rejects unknown fields; used by contract tests
// so schema drift against golden files fails loudly.
func DecodePayloadStrict(env Envelope) (any, error) {
	return decode(env, true)
}

func decode(env Envelope, strict bool) (any, error) {
	factory, ok := payloadFactory[env.Type]
	if !ok {
		return nil, fmt.Errorf("unknown message type %q", env.Type)
	}
	target := factory()
	dec := json.NewDecoder(bytes.NewReader(env.Payload))
	if strict {
		dec.DisallowUnknownFields()
	}
	if err := dec.Decode(target); err != nil {
		return nil, fmt.Errorf("decode %s payload: %w", env.Type, err)
	}
	return target, nil
}

// NewEnvelope wraps a typed payload into a wire envelope.
func NewEnvelope(id string, ts int64, msgType string, payload any) (Envelope, error) {
	if !KnownType(msgType) {
		return Envelope{}, fmt.Errorf("unknown message type %q", msgType)
	}
	raw, err := json.Marshal(payload)
	if err != nil {
		return Envelope{}, fmt.Errorf("encode %s payload: %w", msgType, err)
	}
	return Envelope{V: Version, ID: id, TS: ts, Type: msgType, Payload: raw}, nil
}
