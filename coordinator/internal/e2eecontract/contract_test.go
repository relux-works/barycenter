package e2eecontract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"
)

const fixtureSuite = "AUDIT_FIXTURE_SUITE_NOT_FOR_PRODUCTION"

type vectors struct {
	Status       string `json:"status"`
	FixtureSuite string `json:"fixtureSuite"`
	Baseline     struct {
		GroupID              string `json:"groupId"`
		AirID                string `json:"airId"`
		TargetSnapshotDigest string `json:"targetSnapshotDigest"`
		Epoch                uint64 `json:"epoch"`
		CommitDigest         string `json:"commitDigest"`
	} `json:"baseline"`
	ValidContent     json.RawMessage `json:"validContent"`
	ValidCommit      json.RawMessage `json:"validCommit"`
	MalformedVectors []struct {
		Name     string `json:"name"`
		Mutation string `json:"mutation"`
		Value    string `json:"value"`
		Expected string `json:"expected"`
	} `json:"malformedVectors"`
}

func loadVectors(t *testing.T) vectors {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller")
	}
	path := filepath.Join(filepath.Dir(file), "..", "..", "..", "protocol", "e2ee-media-audit-v1-vectors.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var result vectors
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func fixtureConfig() Config {
	return Config{
		AllowedSuites: map[string]struct{}{fixtureSuite: {}},
		Verifier: VerifierFunc(func(_ string, signature string) bool {
			return signature == "fixture-valid"
		}),
	}
}

func stateFromVectors(value vectors) *State {
	return NewState(value.Baseline.GroupID, value.Baseline.AirID,
		value.Baseline.TargetSnapshotDigest, value.Baseline.Epoch, value.Baseline.CommitDigest)
}

func TestCrossPlatformMalformedVectorsFailClosed(t *testing.T) {
	fixture := loadVectors(t)
	if fixture.Status != "audit-only-production-disabled" || fixture.FixtureSuite != fixtureSuite {
		t.Fatal("fixture status or suite drifted")
	}
	base, err := DecodeCoordinatorMetadata(fixture.ValidContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := stateFromVectors(fixture).AcceptContent(fixtureConfig(), base, base.ManifestDigest, 1000); err != nil {
		t.Fatalf("valid audit fixture rejected: %v", err)
	}
	if got := Code(stateFromVectors(fixture).AcceptContent(ProductionConfig(), base, base.ManifestDigest, 1000)); got != ErrUnknownSuite {
		t.Fatalf("production config did not reject unselected suite: %s", got)
	}

	for _, vector := range fixture.MalformedVectors {
		t.Run(vector.Name, func(t *testing.T) {
			var object map[string]any
			if err := json.Unmarshal(fixture.ValidContent, &object); err != nil {
				t.Fatal(err)
			}
			if vector.Mutation == "epoch" || vector.Mutation == "expires_at_ms" {
				number, err := strconv.ParseInt(vector.Value, 10, 64)
				if err != nil {
					t.Fatal(err)
				}
				object[vector.Mutation] = number
			} else {
				object[vector.Mutation] = vector.Value
			}
			raw, err := json.Marshal(object)
			if err != nil {
				t.Fatal(err)
			}
			candidate, err := DecodeCoordinatorMetadata(raw)
			if err != nil {
				t.Fatalf("vector is not structurally decodable: %v", err)
			}
			state := stateFromVectors(fixture)
			if vector.Mutation == "nonce" {
				state.RememberNonce(vector.Value)
			}
			if vector.Mutation == "event_id" {
				state.RememberEvent(vector.Value)
			}
			if got := string(Code(state.AcceptContent(fixtureConfig(), candidate, base.ManifestDigest, 1000))); got != vector.Expected {
				t.Fatalf("got %q, want %q", got, vector.Expected)
			}
		})
	}
}

func TestCoordinatorMetadataRejectsSecretsAndUnknownFields(t *testing.T) {
	fixture := loadVectors(t)
	var object map[string]any
	if err := json.Unmarshal(fixture.ValidContent, &object); err != nil {
		t.Fatal(err)
	}
	for _, key := range []string{"content_key", "epoch_secret", "plaintext", "private_key"} {
		object[key] = "must-never-route"
		raw, _ := json.Marshal(object)
		if _, err := DecodeCoordinatorMetadata(raw); Code(err) != ErrMalformed {
			t.Fatalf("coordinator accepted %s", key)
		}
		delete(object, key)
	}
	object["future_secretish_field"] = "x"
	raw, _ := json.Marshal(object)
	if _, err := DecodeCoordinatorMetadata(raw); Code(err) != ErrMalformed {
		t.Fatal("unknown coordinator field accepted")
	}
}

func TestCommitOrderingRotationAndForkRules(t *testing.T) {
	fixture := loadVectors(t)
	var base Commit
	if err := json.Unmarshal(fixture.ValidCommit, &base); err != nil {
		t.Fatal(err)
	}
	state := stateFromVectors(fixture)
	if err := state.ApplyCommit(fixtureConfig(), base); err != nil {
		t.Fatal(err)
	}
	if state.Epoch != 8 || state.TargetSnapshotDigest != base.TargetSnapshotDigest {
		t.Fatal("accepted commit did not rotate state")
	}
	if got := Code(state.ApplyCommit(fixtureConfig(), base)); got != ErrReplay {
		t.Fatalf("commit replay = %s", got)
	}

	forkState := stateFromVectors(fixture)
	fork := base
	fork.EventID = "fork"
	fork.PreviousCommitDigest = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	if got := Code(forkState.ApplyCommit(fixtureConfig(), fork)); got != ErrForkedEpoch {
		t.Fatalf("concurrent fork = %s", got)
	}
	stale := base
	stale.EventID = "stale"
	stale.PreviousEpoch = 6
	stale.Epoch = 7
	if got := Code(forkState.ApplyCommit(fixtureConfig(), stale)); got != ErrStaleEpoch {
		t.Fatalf("stale commit = %s", got)
	}
}
