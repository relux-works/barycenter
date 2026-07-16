package presentation

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStreamTrackPresentationMatchesPortableContract(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "..", "..", "protocol", "pulsar-stream-track-ui-model-v1.json"))
	if err != nil {
		t.Fatal(err)
	}
	var contract struct {
		ContractID     string   `json:"contract_id"`
		DraftPhases    []string `json:"draft_phases"`
		PlaybackPhases []string `json:"playback_phases"`
		Actions        []string `json:"actions"`
		FailureCodes   []string `json:"failure_codes"`
		Localization   struct {
			Labels map[string]struct {
				EN string `json:"en"`
				RU string `json:"ru"`
			} `json:"labels"`
		} `json:"localization"`
	}
	if err := json.Unmarshal(raw, &contract); err != nil {
		t.Fatal(err)
	}
	if contract.ContractID != "pulsar.stream-track-ui-model.v1" {
		t.Fatalf("contract_id=%q", contract.ContractID)
	}
	check := func(prefix string, values []string, present func(string) Label) {
		t.Helper()
		for _, value := range values {
			want, ok := contract.Localization.Labels[prefix+value]
			if !ok {
				t.Errorf("contract missing label %s%s", prefix, value)
				continue
			}
			got := present(value)
			if got.Key != prefix+value || got.EN != want.EN || got.RU != want.RU {
				t.Errorf("%s: got=%+v want=%+v", value, got, want)
			}
		}
	}
	check("stream_track.draft.", contract.DraftPhases, StreamTrackDraftPhaseLabel)
	check("stream_track.playback.", contract.PlaybackPhases, StreamTrackPlaybackPhaseLabel)
	check("stream_track.action.", contract.Actions, StreamTrackActionLabel)
	check("stream_track.failure.", contract.FailureCodes, StreamTrackFailureLabel)
}

func TestStreamTrackPresentationFailsClosed(t *testing.T) {
	if got := StreamTrackDraftPhaseLabel("raw secret"); got.Key != "stream_track.draft.failed" {
		t.Fatalf("draft fallback=%+v", got)
	}
	if got := StreamTrackPlaybackPhaseLabel("raw secret"); got.Key != "stream_track.playback.failed" {
		t.Fatalf("playback fallback=%+v", got)
	}
	if got := StreamTrackFailureLabel("raw secret"); got.Key != "stream_track.failure.service_unavailable" || strings.Contains(got.EN, "secret") {
		t.Fatalf("failure fallback=%+v", got)
	}
	if got := StreamTrackActionLabel("raw secret"); got != (Label{}) {
		t.Fatalf("unknown action became capability: %+v", got)
	}
}
