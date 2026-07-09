package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"
)

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
