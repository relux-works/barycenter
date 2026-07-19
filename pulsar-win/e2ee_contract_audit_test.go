package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"testing"
)

func TestWindowsE2EEAuditModelConsumesSharedMalformedVectors(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "protocol", "e2ee-media-audit-v1-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Status       string `json:"status"`
		FixtureSuite string `json:"fixtureSuite"`
		Baseline     struct {
			GroupID, AirID, TargetSnapshotDigest string
			Epoch                                uint64
			CommitDigest                         string
		} `json:"baseline"`
		ValidContent json.RawMessage `json:"validContent"`
		ValidCommit  json.RawMessage `json:"validCommit"`
		Malformed    []struct {
			Name, Mutation, Value, Expected string
		} `json:"malformedVectors"`
		MultiFault []struct {
			Name      string
			Mutations map[string]string
			Expected  string
		} `json:"multiFaultVectors"`
		ReplayState []struct {
			Name               string
			RememberGeneration uint64
			RememberSequence   uint64
			Generation         uint64
			Sequence           uint64
			Expected           string
		} `json:"replayStateVectors"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	if fixture.Status != "audit-only-production-disabled" {
		t.Fatal("draft status drifted")
	}
	verify := e2eeAuditVerifier(func(_ string, signature string) bool { return signature == "fixture-valid" })
	suites := map[string]bool{fixture.FixtureSuite: true}
	base, code := decodeE2EEAuditMetadata(fixture.ValidContent)
	if code != "" {
		t.Fatal(code)
	}
	state := newE2EEAuditState(fixture.Baseline.GroupID, fixture.Baseline.AirID, fixture.Baseline.TargetSnapshotDigest, fixture.Baseline.Epoch, fixture.Baseline.CommitDigest)
	if code := state.accept(base, base.ManifestDigest, 1000, suites, verify); code != "" {
		t.Fatal(code)
	}
	if code := newE2EEAuditState(fixture.Baseline.GroupID, fixture.Baseline.AirID, fixture.Baseline.TargetSnapshotDigest, fixture.Baseline.Epoch, fixture.Baseline.CommitDigest).accept(base, base.ManifestDigest, 1000, map[string]bool{}, nil); code != "unknown_suite" {
		t.Fatalf("production no-go = %s", code)
	}

	for _, vector := range fixture.Malformed {
		t.Run(vector.Name, func(t *testing.T) {
			var object map[string]any
			if err := json.Unmarshal(fixture.ValidContent, &object); err != nil {
				t.Fatal(err)
			}
			if vector.Mutation == "epoch" || vector.Mutation == "expires_at_ms" {
				number, _ := strconv.ParseInt(vector.Value, 10, 64)
				object[vector.Mutation] = number
			} else {
				object[vector.Mutation] = vector.Value
			}
			candidateRaw, _ := json.Marshal(object)
			candidate, code := decodeE2EEAuditMetadata(candidateRaw)
			if code != "" {
				t.Fatal(code)
			}
			state := newE2EEAuditState(fixture.Baseline.GroupID, fixture.Baseline.AirID, fixture.Baseline.TargetSnapshotDigest, fixture.Baseline.Epoch, fixture.Baseline.CommitDigest)
			if vector.Mutation == "nonce" {
				state.seenNonces[vector.Value] = true
			}
			if vector.Mutation == "event_id" {
				state.seenEvents[vector.Value] = true
			}
			if got := state.accept(candidate, base.ManifestDigest, 1000, suites, verify); got != vector.Expected {
				t.Fatalf("got %q, want %q", got, vector.Expected)
			}
		})
	}
	for _, vector := range fixture.MultiFault {
		t.Run(vector.Name, func(t *testing.T) {
			var object map[string]any
			if err := json.Unmarshal(fixture.ValidContent, &object); err != nil {
				t.Fatal(err)
			}
			for key, value := range vector.Mutations {
				object[key] = value
			}
			candidateRaw, _ := json.Marshal(object)
			candidate, code := decodeE2EEAuditMetadata(candidateRaw)
			if code != "" {
				t.Fatal(code)
			}
			state := newE2EEAuditState(fixture.Baseline.GroupID, fixture.Baseline.AirID,
				fixture.Baseline.TargetSnapshotDigest, fixture.Baseline.Epoch, fixture.Baseline.CommitDigest)
			if got := state.accept(candidate, base.ManifestDigest, 1000, suites, verify); got != vector.Expected {
				t.Fatalf("got %q, want %q", got, vector.Expected)
			}
		})
	}
	for _, vector := range fixture.ReplayState {
		t.Run(vector.Name, func(t *testing.T) {
			candidate := base
			candidate.EventID = "state-" + vector.Name
			candidate.Nonce = "nonce-" + vector.Name
			candidate.Generation = vector.Generation
			candidate.Sequence = vector.Sequence
			state := newE2EEAuditState(fixture.Baseline.GroupID, fixture.Baseline.AirID,
				fixture.Baseline.TargetSnapshotDigest, fixture.Baseline.Epoch, fixture.Baseline.CommitDigest)
			key := candidate.DeviceID + "/" + candidate.ObjectID + "/" + strconv.FormatUint(vector.RememberGeneration, 10)
			state.lastSequences[key] = vector.RememberSequence
			if got := state.accept(candidate, base.ManifestDigest, 1000, suites, verify); got != vector.Expected {
				t.Fatalf("got %q, want %q", got, vector.Expected)
			}
		})
	}
}

func TestWindowsE2EEAuditCommitOrderingAndForks(t *testing.T) {
	raw, err := os.ReadFile(filepath.Join("..", "protocol", "e2ee-media-audit-v1-vectors.json"))
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		FixtureSuite string `json:"fixtureSuite"`
		Baseline     struct {
			GroupID, AirID, TargetSnapshotDigest, CommitDigest string
			Epoch                                              uint64
		} `json:"baseline"`
		ValidCommit e2eeAuditCommit `json:"validCommit"`
	}
	if err := json.Unmarshal(raw, &fixture); err != nil {
		t.Fatal(err)
	}
	verify := e2eeAuditVerifier(func(_ string, signature string) bool { return signature == "fixture-valid" })
	suites := map[string]bool{fixture.FixtureSuite: true}
	newState := func() *e2eeAuditState {
		return newE2EEAuditState(fixture.Baseline.GroupID, fixture.Baseline.AirID, fixture.Baseline.TargetSnapshotDigest, fixture.Baseline.Epoch, fixture.Baseline.CommitDigest)
	}
	state := newState()
	if code := state.applyCommit(fixture.ValidCommit, suites, verify); code != "" || state.epoch != 8 {
		t.Fatalf("valid commit: %s epoch=%d", code, state.epoch)
	}
	if code := state.applyCommit(fixture.ValidCommit, suites, verify); code != "replay" {
		t.Fatalf("replay = %s", code)
	}
	fork := fixture.ValidCommit
	fork.EventID = "fork"
	fork.PreviousCommitDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if code := newState().applyCommit(fork, suites, verify); code != "forked_epoch" {
		t.Fatalf("fork = %s", code)
	}
}

func TestWindowsE2EEAuditModelRejectsCoordinatorSecrets(t *testing.T) {
	raw := []byte(`{"content_key":"secret"}`)
	if _, code := decodeE2EEAuditMetadata(raw); code != "malformed" {
		t.Fatalf("secret-bearing metadata accepted: %s", code)
	}
}
