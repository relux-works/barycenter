package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"runtime"
	"strings"
	"testing"
)

var goldenIdentifierPatterns = map[string]*regexp.Regexp{
	"msg_": regexp.MustCompile(`^msg_[0-7][0-9A-HJKMNP-TV-Z]{25}$`),
	"el_":  regexp.MustCompile(`^el_[0-7][0-9A-HJKMNP-TV-Z]{25}$`),
	"m_":   regexp.MustCompile(`^m_[0-7][0-9A-HJKMNP-TV-Z]{25}$`),
	"tr_":  regexp.MustCompile(`^tr_[0-7][0-9A-HJKMNP-TV-Z]{25}$`),
}

func validateGoldenIdentifiers(t *testing.T, value any) {
	t.Helper()
	switch value := value.(type) {
	case map[string]any:
		for _, child := range value {
			validateGoldenIdentifiers(t, child)
		}
	case []any:
		for _, child := range value {
			validateGoldenIdentifiers(t, child)
		}
	case string:
		for prefix, pattern := range goldenIdentifierPatterns {
			if strings.HasPrefix(value, prefix) && !pattern.MatchString(value) {
				t.Errorf("malformed %s golden identifier %q", prefix, value)
			}
		}
	}
}

func goldenDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// coordinator/internal/protocol -> repo root -> protocol/golden
	dir := filepath.Join(filepath.Dir(thisFile), "..", "..", "..", "protocol", "golden")
	abs, err := filepath.Abs(dir)
	if err != nil {
		t.Fatal(err)
	}
	return abs
}

// Contract test (spec 8.7): every golden file must decode strictly into a
// typed payload and re-encode to a semantically identical JSON document.
func TestGoldenRoundTrip(t *testing.T) {
	dir := goldenDir(t)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read golden dir: %v", err)
	}
	var files []string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".json") {
			files = append(files, e.Name())
		}
	}
	if len(files) != len(payloadFactory) {
		t.Fatalf("golden files (%d) and known types (%d) out of sync", len(files), len(payloadFactory))
	}

	seenTypes := map[string]bool{}
	for _, name := range files {
		name := name
		t.Run(name, func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(dir, name))
			if err != nil {
				t.Fatal(err)
			}
			var env Envelope
			if err := json.Unmarshal(raw, &env); err != nil {
				t.Fatalf("envelope decode: %v", err)
			}
			if env.V != Version {
				t.Fatalf("envelope v = %d, want %d", env.V, Version)
			}
			if want := strings.TrimSuffix(name, ".json"); env.Type != want {
				t.Fatalf("file %s carries type %q", name, env.Type)
			}
			seenTypes[env.Type] = true

			payload, err := DecodePayloadStrict(env)
			if err != nil {
				t.Fatalf("strict payload decode: %v", err)
			}

			out, err := NewEnvelope(env.ID, env.TS, env.Type, payload)
			if err != nil {
				t.Fatalf("re-encode: %v", err)
			}
			outRaw, err := json.Marshal(out)
			if err != nil {
				t.Fatal(err)
			}

			var wantAny, gotAny any
			if err := json.Unmarshal(raw, &wantAny); err != nil {
				t.Fatal(err)
			}
			validateGoldenIdentifiers(t, wantAny)
			if err := json.Unmarshal(outRaw, &gotAny); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(wantAny, gotAny) {
				t.Fatalf("round-trip mismatch\ngolden: %s\ngot:    %s", raw, outRaw)
			}
		})
	}
	for typ := range payloadFactory {
		if !seenTypes[typ] {
			t.Errorf("no golden file for message type %q", typ)
		}
	}
}

// Optional fields must be omitted, not null (docs/protocol.md).
func TestOptionalFieldOmission(t *testing.T) {
	env, err := NewEnvelope("msg_x", 1, TypePlayVoice, &PlayVoicePayload{
		ElementID: "el_x", FileURL: "http://coord:8080/media/m.wav",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(env.Payload), "t_coord_ms") {
		t.Fatalf("t_coord_ms must be omitted when nil, got %s", env.Payload)
	}
	env2, err := NewEnvelope("msg_x", 1, TypeError, &ErrorPayload{Code: "load_failed", Message: "m"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(env2.Payload), "element_id") {
		t.Fatalf("error.element_id must be omitted when empty, got %s", env2.Payload)
	}

	interruptFadeOut, interruptFadeIn := int64(250), int64(120)
	interrupt, err := NewEnvelope("msg_x", 1, TypePlayMediaAt, &PlayMediaAtPayload{
		TransmissionID: "tr_x", Generation: 1, TCoordMS: 2,
		StartDeadlineCoordMS: 102, Delivery: "interrupt",
		FadeOutMS: &interruptFadeOut, FadeInMS: &interruptFadeIn,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"duck_db", "attack_ms", "release_ms"} {
		if strings.Contains(string(interrupt.Payload), forbidden) {
			t.Fatalf("interrupt %s must be omitted, got %s", forbidden, interrupt.Payload)
		}
	}

	dnd, err := NewEnvelope("msg_x", 1, TypeSetDND, &SetDNDPayload{Revision: 1, Mode: "allow_all"})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(dnd.Payload), "muted_until_coord_ms") {
		t.Fatalf("set_dnd muted_until_coord_ms must be omitted outside muted_until, got %s", dnd.Payload)
	}

	presence, err := NewEnvelope("msg_x", 1, TypePresenceUpdate, &PresenceUpdatePayload{
		Revision: 1, GeneratedAtCoordMS: 1,
		Nodes: []PresenceNode{{OrbitID: 1, Slot: "a", DNDMode: "messages_only", Capabilities: []string{}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(presence.Payload), "dnd_until_coord_ms") {
		t.Fatalf("presence dnd_until_coord_ms must be omitted outside muted_until, got %s", presence.Payload)
	}
}

func TestCapabilitySetContract(t *testing.T) {
	valid := []string{
		CapabilityInterruptResume,
		CapabilityMediaClip,
		CapabilityOverlayMix,
		CapabilitySeamlessAdoption,
		"unknown_future_v2",
	}
	set, err := ParseCapabilitySet(valid)
	if err != nil {
		t.Fatal(err)
	}
	if !set.Supports(CapabilityMediaClip) || !set.Supports("unknown_future_v2") || set.Supports("absent") {
		t.Fatalf("capability lookup mismatch: %v", set.Values())
	}
	copyValues := set.Values()
	copyValues[0] = "mutated"
	if set.Values()[0] != CapabilityInterruptResume {
		t.Fatal("capability set leaked mutable storage")
	}

	for _, tc := range []struct {
		name   string
		values []string
	}{
		{"duplicate", []string{"media_clip_v1", "media_clip_v1"}},
		{"unsorted", []string{"overlay_mix_v1", "media_clip_v1"}},
		{"empty", []string{""}},
		{"space", []string{"media clip"}},
		{"non-ascii", []string{"média_clip_v1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ParseCapabilitySet(tc.values); err == nil {
				t.Fatalf("accepted invalid capabilities %q", tc.values)
			}
		})
	}
}

func TestLegacyVoiceMessagesRemainCompatible(t *testing.T) {
	for _, tc := range []struct {
		msgType string
		payload any
	}{
		{TypePlayVoice, &PlayVoicePayload{ElementID: "el_x", FileURL: "https://coord/media/x"}},
		{TypeSoloVoice, &SoloVoicePayload{ElementID: "el_x", FileURL: "https://coord/media/x"}},
	} {
		env, err := NewEnvelope("msg_x", 1, tc.msgType, tc.payload)
		if err != nil {
			t.Fatal(err)
		}
		if !KnownType(tc.msgType) {
			t.Fatalf("legacy type %q is no longer known", tc.msgType)
		}
		if _, err := DecodePayloadStrict(env); err != nil {
			t.Fatalf("legacy type %q no longer decodes: %v", tc.msgType, err)
		}
	}
}

// v1.1 additive fields (spec-providers §7) are optional: a pre-v1.1-style
// message with none of them set must still encode byte-identically to v1, so
// old nodes keep round-tripping. This is the omitempty half of the guard that
// the populated goldens (load/solo_inject/state) exercise on the present side.
func TestV11AdditiveFieldsOmittedWhenEmpty(t *testing.T) {
	cases := []struct {
		name    string
		msgType string
		payload any
		fields  []string
	}{
		{"load", TypeLoad, &LoadPayload{ElementID: "el_x", URI: "spotify:track:x"},
			[]string{`"provider"`, `"ref"`, `"duration_ms"`, `"gain_db"`}},
		{"solo_inject", TypeSoloInject, &SoloInjectPayload{URI: "spotify:track:x"},
			[]string{`"provider"`, `"ref"`, `"ctid"`}},
		{"state", TypeState, &StatePayload{Playback: "playing", Speakers: []Speaker{}},
			[]string{`"provider"`}},
		{"external_playback", TypeExternalPlayback, &ExternalPlaybackPayload{URI: "spotify:track:x"},
			[]string{`"position_ms"`}},
	}
	for _, tc := range cases {
		env, err := NewEnvelope("msg_x", 1, tc.msgType, tc.payload)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range tc.fields {
			if strings.Contains(string(env.Payload), f) {
				t.Fatalf("%s: %s must be omitted when empty, got %s", tc.name, f, env.Payload)
			}
		}
	}
}

// Lenient decode must tolerate unknown payload fields (spec 8.6 forward compat).
func TestLenientDecodeToleratesUnknownFields(t *testing.T) {
	env := Envelope{V: 1, ID: "msg_x", TS: 1, Type: TypeReady,
		Payload: json.RawMessage(`{"element_id":"el_x","future_field":42}`)}
	if _, err := DecodePayload(env); err != nil {
		t.Fatalf("lenient decode: %v", err)
	}
	if _, err := DecodePayloadStrict(env); err == nil {
		t.Fatal("strict decode should reject unknown fields")
	}
}

func TestEveryDecoderRejectsMixedMajorVersionBeforePayloadDispatch(t *testing.T) {
	for _, strict := range []bool{false, true} {
		env := Envelope{V: Version + 1, ID: "msg_x", TS: 1, Type: TypePing,
			Payload: json.RawMessage(`{"t1":1}`)}
		var err error
		if strict {
			_, err = DecodePayloadStrict(env)
		} else {
			_, err = DecodePayload(env)
		}
		if err == nil || !strings.Contains(err.Error(), "unsupported protocol version 2, want 1") {
			t.Fatalf("strict=%v mixed-version error=%v", strict, err)
		}
	}
}
