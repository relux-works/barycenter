package protocol

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

type captureQualityVectorDocument struct {
	SchemaVersion    int                 `json:"schemaVersion"`
	Contract         string              `json:"contract"`
	ValidState       CaptureQualityState `json:"validState"`
	InvalidMutations []struct {
		Name  string          `json:"name"`
		Field string          `json:"field"`
		Value json.RawMessage `json:"value"`
	} `json:"invalidMutations"`
	GenerationSequence []struct {
		Generation int64                          `json:"generation"`
		UpdatedMS  int64                          `json:"updated_monotonic_ms"`
		Expected   CaptureQualityGenerationResult `json:"expected"`
	} `json:"generationSequence"`
}

func captureQualityVectors(t *testing.T) captureQualityVectorDocument {
	t.Helper()
	path := filepath.Join(repoRoot(t), "protocol", "capture-quality-v1-vectors.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var document captureQualityVectorDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	if document.SchemaVersion != 1 || document.Contract != "capture-quality.v1-vectors" {
		t.Fatalf("unexpected vector document: %+v", document)
	}
	return document
}

func mutateCaptureQualityState(t *testing.T, base CaptureQualityState, field string, value json.RawMessage) CaptureQualityState {
	t.Helper()
	raw, err := json.Marshal(base)
	if err != nil {
		t.Fatal(err)
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	var replacement any
	if err := json.Unmarshal(value, &replacement); err != nil {
		t.Fatal(err)
	}
	object[field] = replacement
	raw, err = json.Marshal(object)
	if err != nil {
		t.Fatal(err)
	}
	var state CaptureQualityState
	if err := json.Unmarshal(raw, &state); err != nil {
		t.Fatal(err)
	}
	return state
}

func TestCaptureQualitySharedVectorsRejectMalformedAndStaleState(t *testing.T) {
	document := captureQualityVectors(t)
	if err := ValidateCaptureQualityState(&document.ValidState); err != nil {
		t.Fatal(err)
	}
	for _, vector := range document.InvalidMutations {
		t.Run(vector.Name, func(t *testing.T) {
			state := mutateCaptureQualityState(t, document.ValidState, vector.Field, vector.Value)
			if err := ValidateCaptureQualityState(&state); err == nil {
				t.Fatalf("accepted malformed capture quality state: %+v", state)
			}
		})
	}
	var guard CaptureQualityGenerationGuard
	for _, vector := range document.GenerationSequence {
		if got := guard.Accept(vector.Generation, vector.UpdatedMS); got != vector.Expected {
			t.Fatalf("guard generation=%d updated=%d got=%s want=%s", vector.Generation, vector.UpdatedMS, got, vector.Expected)
		}
	}
}

func TestCaptureQualityStateCodecAndMixedVersionGuidance(t *testing.T) {
	document := captureQualityVectors(t)
	payload := StatePayload{Playback: "stopped", Speakers: []Speaker{}, CaptureQuality: &document.ValidState}
	envelope, err := NewEnvelope("msg_x", 1, TypeState, &payload)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := DecodePayloadStrict(envelope)
	if err != nil {
		t.Fatal(err)
	}
	state := decoded.(*StatePayload)
	if !reflect.DeepEqual(state.CaptureQuality, &document.ValidState) {
		t.Fatalf("capture quality round trip mismatch: %+v", state.CaptureQuality)
	}

	legacy, err := ParseCapabilitySet([]string{CapabilityMediaClip})
	if err != nil {
		t.Fatal(err)
	}
	if got := PresentCaptureQuality(legacy, nil); got.Key != "capture_quality.mixed_version" || got.Quality != CaptureQualityUnsupported {
		t.Fatalf("legacy guidance=%+v", got)
	}
	capable, err := ParseCapabilitySet([]string{CapabilityCaptureQuality})
	if err != nil {
		t.Fatal(err)
	}
	if got := PresentCaptureQuality(capable, &document.ValidState); got.Key != "capture_quality.accepted.none" {
		t.Fatalf("accepted guidance=%+v", got)
	} else if !got.Available || got.RequestedMode != CaptureRouteAuto ||
		got.ResolvedMode != CaptureRouteSpeaker || got.AEC != CaptureEffectActive ||
		got.NS != CaptureEffectActive || got.AGC != CaptureEffectActive ||
		got.InputHealth != CaptureHealthOK || got.InputCeilingDBFS != -3 ||
		got.OutputCeilingDBFS != -1 {
		t.Fatalf("incomplete shell projection=%+v", got)
	}
}

func TestCaptureQualityDiagnosticProjectionIsContentFree(t *testing.T) {
	state := captureQualityVectors(t).ValidState
	raw, err := json.Marshal(CaptureQualityDiagnosticFields(&state))
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(raw))
	for _, forbidden := range []string{"audio", "sample", "device", "path", "file", "transcript", "reference_age", "input_ceiling"} {
		if strings.Contains(text, forbidden) {
			t.Fatalf("diagnostic projection leaked %q: %s", forbidden, text)
		}
	}
}
