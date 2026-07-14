package legalops

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryLegalOpsCheckpointIsStrictAndBlocked(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..", "..")
	checkpointPath := filepath.Join(repositoryRoot, "docs", "compliance", "legal-ops-inputs.json")
	checkpoint, err := Load(checkpointPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := checkpoint.Validate(false); err != nil {
		t.Fatalf("partially approved checkpoint must remain structurally valid: %v", err)
	}
	expectedUnresolved := []string{
		"hosting_and_data_locations",
		"markets_age_and_disputes",
		"moderation_ownership_and_response",
		"policy_review_and_configuration",
	}
	unresolved := checkpoint.Unresolved()
	if strings.Join(unresolved, ",") != strings.Join(expectedUnresolved, ",") {
		t.Fatalf("unresolved inputs=%v, want %v", unresolved, expectedUnresolved)
	}
	if err := checkpoint.Validate(true); err == nil {
		t.Fatal("partially approved checkpoint passed the external publication gate")
	} else {
		for _, id := range expectedUnresolved {
			if !strings.Contains(err.Error(), id) {
				t.Errorf("approval failure does not name unresolved input %q: %v", id, err)
			}
		}
	}

	checklist, err := os.ReadFile(filepath.Join(repositoryRoot, "docs", "analysis", "p1-legal-ops-input-checkpoint.md"))
	if err != nil {
		t.Fatalf("read concise legal/operations checklist: %v", err)
	}
	for _, input := range checkpoint.Inputs {
		marker := "- [x] `"
		if input.Status != "approved" {
			marker = "- [ ] `"
		}
		if !strings.Contains(string(checklist), marker+input.ID+"`") {
			t.Errorf("concise checklist does not expose status %q for input %q", input.Status, input.ID)
		}
	}

	workflow, err := os.ReadFile(filepath.Join(repositoryRoot, ".github", "workflows", "store-submit.yml"))
	if err != nil {
		t.Fatalf("read Store submission workflow: %v", err)
	}
	if !strings.Contains(string(workflow), "legal-ops-check --require-approved") {
		t.Fatal("Store submission workflow can bypass the legal/operations approval gate")
	}
}

func TestLegalOpsCheckpointRejectsUnknownFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "checkpoint.json")
	if err := os.WriteFile(path, []byte(`{
  "schema_version":1,
  "task_id":"TASK-260712-16zfvu",
  "observed_at":"2026-07-14",
  "publication_gate":{"state":"blocked","approved_by":null,"approved_at":null},
  "inputs":[],
  "invented_authority":"nobody"
}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("strict decode error=%v, want unknown field", err)
	}
}

func TestLegalOpsCheckpointRejectsApprovedPlaceholders(t *testing.T) {
	checkpoint := fullyApprovedCheckpoint(t)
	checkpoint.Inputs[0].Value = json.RawMessage(`{"legal_name":"TODO company"}`)
	if err := checkpoint.Validate(true); err == nil || !strings.Contains(err.Error(), "placeholder") {
		t.Fatalf("placeholder validation error=%v", err)
	}
}

func TestLegalOpsCheckpointAcceptsCompleteApproval(t *testing.T) {
	checkpoint := fullyApprovedCheckpoint(t)
	if err := checkpoint.Validate(true); err != nil {
		t.Fatalf("complete approved checkpoint: %v", err)
	}
}

func fullyApprovedCheckpoint(t *testing.T) Checkpoint {
	t.Helper()
	approver := "Ivan Oparin"
	approvedAt := "2026-07-14T13:30:00+04:00"
	owner := "Relux Works release owner"
	inputs := make([]Input, 0, len(RequiredInputIDs))
	for _, id := range RequiredInputIDs {
		inputs = append(inputs, Input{
			ID: id, Status: "approved",
			Value:      json.RawMessage(`{"approved_fact":"real value"}`),
			Candidate:  json.RawMessage("null"),
			Owner:      &owner,
			ApprovedBy: &approver,
			ApprovedAt: &approvedAt,
			Evidence:   []string{"approval record"},
		})
	}
	return Checkpoint{
		SchemaVersion: SchemaVersion,
		TaskID:        TaskID,
		ObservedAt:    "2026-07-14",
		PublicationGate: PublicationGate{
			State: "approved", ApprovedBy: &approver, ApprovedAt: &approvedAt,
		},
		Inputs: inputs,
	}
}
