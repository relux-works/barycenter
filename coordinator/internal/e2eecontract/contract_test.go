package e2eecontract

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
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
	MultiFaultVectors []struct {
		Name      string            `json:"name"`
		Mutations map[string]string `json:"mutations"`
		Expected  string            `json:"expected"`
	} `json:"multiFaultVectors"`
	ReplayStateVectors []struct {
		Name               string `json:"name"`
		RememberGeneration uint64 `json:"rememberGeneration"`
		RememberSequence   uint64 `json:"rememberSequence"`
		Generation         uint64 `json:"generation"`
		Sequence           uint64 `json:"sequence"`
		Expected           string `json:"expected"`
	} `json:"replayStateVectors"`
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
	for _, vector := range fixture.MultiFaultVectors {
		t.Run(vector.Name, func(t *testing.T) {
			var object map[string]any
			if err := json.Unmarshal(fixture.ValidContent, &object); err != nil {
				t.Fatal(err)
			}
			for key, value := range vector.Mutations {
				object[key] = value
			}
			raw, _ := json.Marshal(object)
			candidate, err := DecodeCoordinatorMetadata(raw)
			if err != nil {
				t.Fatal(err)
			}
			if got := string(Code(stateFromVectors(fixture).AcceptContent(
				fixtureConfig(), candidate, base.ManifestDigest, 1000,
			))); got != vector.Expected {
				t.Fatalf("got %q, want %q", got, vector.Expected)
			}
		})
	}
	for _, vector := range fixture.ReplayStateVectors {
		t.Run(vector.Name, func(t *testing.T) {
			candidate := base
			candidate.EventID = "state-" + vector.Name
			candidate.Nonce = "nonce-" + vector.Name
			candidate.Generation = vector.Generation
			candidate.Sequence = vector.Sequence
			state := stateFromVectors(fixture)
			state.RememberSequence(candidate.DeviceID, candidate.ObjectID,
				vector.RememberGeneration, vector.RememberSequence)
			if got := string(Code(state.AcceptContent(
				fixtureConfig(), candidate, base.ManifestDigest, 1000,
			))); got != vector.Expected {
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

func TestEveryCoordinatorPublicEnvelopeRejectsSecretsAndUnknownFields(t *testing.T) {
	fixture := loadVectors(t)
	var commit Commit
	if err := json.Unmarshal(fixture.ValidCommit, &commit); err != nil {
		t.Fatal(err)
	}
	envelopes := []struct {
		name   string
		value  any
		decode func([]byte) error
	}{
		{"commit", commit, func(raw []byte) error { _, err := DecodeCoordinatorCommit(raw); return err }},
		{"proposal", Proposal{Contract: Contract, Capability: Capability, Suite: fixtureSuite,
			EventID: "proposal-event", GroupID: commit.GroupID, ActorID: commit.ActorID,
			DeviceID: commit.DeviceID, AirID: commit.AirID, PreviousEpoch: 7, Epoch: 8,
			TargetSnapshotDigest: commit.TargetSnapshotDigest,
			ProposalDigest:       strings.Repeat("a", 64), AuthenticatedDataDigest: commit.AuthenticatedDataDigest,
			Signature: "fixture-valid"}, func(raw []byte) error { _, err := DecodeCoordinatorProposal(raw); return err }},
		{"welcome", Welcome{Contract: Contract, Capability: Capability, Suite: fixtureSuite,
			EventID: "welcome-event", GroupID: commit.GroupID, ActorID: commit.ActorID,
			DeviceID: commit.DeviceID, RecipientDeviceID: "recipient-device", AirID: commit.AirID,
			Epoch: 8, TargetSnapshotDigest: commit.TargetSnapshotDigest,
			WelcomeDigest: strings.Repeat("b", 64), ExpiresAtMS: 2000,
			CiphertextURL: "/welcome", AuthenticatedDataDigest: commit.AuthenticatedDataDigest,
			Signature: "fixture-valid"}, func(raw []byte) error { _, err := DecodeCoordinatorWelcome(raw); return err }},
		{"key-package", KeyPackage{Contract: Contract, Capability: Capability, Suite: fixtureSuite,
			EventID: "key-package-event", ActorID: commit.ActorID, DeviceID: commit.DeviceID,
			KeyPackageDigest: strings.Repeat("c", 64), PublicPackageURL: "/key-package",
			ExpiresAtMS: 2000, AuthenticatedDataDigest: commit.AuthenticatedDataDigest,
			Signature: "fixture-valid"}, func(raw []byte) error { _, err := DecodeCoordinatorKeyPackage(raw); return err }},
		{"history-grant", HistoryGrant{Contract: Contract, Capability: Capability, Suite: fixtureSuite,
			EventID: "history-grant-event", GroupID: commit.GroupID, ActorID: commit.ActorID,
			DeviceID: commit.DeviceID, RecipientDeviceID: "recipient-device", AirID: commit.AirID,
			ObjectID: "object-id", FirstEpoch: 1, LastEpoch: 7,
			TargetSnapshotDigest: commit.TargetSnapshotDigest, GrantDigest: strings.Repeat("d", 64),
			ExpiresAtMS: 2000, CiphertextURL: "/history-grant",
			AuthenticatedDataDigest: commit.AuthenticatedDataDigest, Signature: "fixture-valid"},
			func(raw []byte) error { _, err := DecodeCoordinatorHistoryGrant(raw); return err }},
	}
	for _, envelope := range envelopes {
		t.Run(envelope.name, func(t *testing.T) {
			raw, err := json.Marshal(envelope.value)
			if err != nil {
				t.Fatal(err)
			}
			if err := envelope.decode(raw); err != nil {
				t.Fatalf("valid envelope rejected: %v", err)
			}
			var object map[string]any
			if err := json.Unmarshal(raw, &object); err != nil {
				t.Fatal(err)
			}
			for _, key := range append(coordinatorForbiddenFields, "future_secretish_field") {
				object[key] = "must-never-route"
				mutated, _ := json.Marshal(object)
				if err := envelope.decode(mutated); Code(err) != ErrMalformed {
					t.Fatalf("accepted forbidden/unknown field %q", key)
				}
				delete(object, key)
			}
		})
	}
}

func TestProposalValidationUsesCanonicalFailurePrecedence(t *testing.T) {
	fixture := loadVectors(t)
	var commit Commit
	if err := json.Unmarshal(fixture.ValidCommit, &commit); err != nil {
		t.Fatal(err)
	}
	valid := Proposal{Contract: Contract, Capability: Capability, Suite: fixtureSuite,
		EventID: "proposal-precedence-event", GroupID: commit.GroupID,
		ActorID: commit.ActorID, DeviceID: commit.DeviceID, AirID: commit.AirID,
		PreviousEpoch: fixture.Baseline.Epoch, Epoch: fixture.Baseline.Epoch + 1,
		TargetSnapshotDigest:    fixture.Baseline.TargetSnapshotDigest,
		ProposalDigest:          strings.Repeat("a", 64),
		AuthenticatedDataDigest: commit.AuthenticatedDataDigest,
		Signature:               "fixture-valid"}
	if err := stateFromVectors(fixture).ValidateProposal(fixtureConfig(), valid); err != nil {
		t.Fatalf("valid proposal rejected: %v", err)
	}
	if got := Code(stateFromVectors(fixture).ValidateProposal(ProductionConfig(), valid)); got != ErrUnknownSuite {
		t.Fatalf("production proposal = %s", got)
	}
	malformed := valid
	malformed.ActorID = ""
	malformed.Signature = "invalid"
	if got := Code(stateFromVectors(fixture).ValidateProposal(fixtureConfig(), malformed)); got != ErrMalformed {
		t.Fatalf("malformed/signature precedence = %s", got)
	}
	invalidSignature := valid
	invalidSignature.Signature = "invalid"
	invalidSignature.GroupID = "foreign"
	if got := Code(stateFromVectors(fixture).ValidateProposal(fixtureConfig(), invalidSignature)); got != ErrInvalidSignature {
		t.Fatalf("signature/target precedence = %s", got)
	}
	foreign := valid
	foreign.GroupID = "foreign"
	foreign.PreviousEpoch = fixture.Baseline.Epoch - 1
	foreign.Epoch = fixture.Baseline.Epoch
	if got := Code(stateFromVectors(fixture).ValidateProposal(fixtureConfig(), foreign)); got != ErrForeignTarget {
		t.Fatalf("target/stale precedence = %s", got)
	}
	staleState := stateFromVectors(fixture)
	staleState.RememberEvent(valid.EventID)
	stale := valid
	stale.PreviousEpoch = fixture.Baseline.Epoch - 1
	stale.Epoch = fixture.Baseline.Epoch
	if got := Code(staleState.ValidateProposal(fixtureConfig(), stale)); got != ErrStaleEpoch {
		t.Fatalf("stale/replay precedence = %s", got)
	}
	forkState := stateFromVectors(fixture)
	forkState.RememberEvent(valid.EventID)
	fork := valid
	fork.Epoch = fixture.Baseline.Epoch + 2
	if got := Code(forkState.ValidateProposal(fixtureConfig(), fork)); got != ErrForkedEpoch {
		t.Fatalf("fork/replay precedence = %s", got)
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
