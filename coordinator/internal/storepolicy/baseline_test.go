package storepolicy

import (
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

const (
	wantSnapshotDate  = "2026-07-14"
	wantPolicyVersion = "7.19"
)

type baseline struct {
	SchemaVersion         int                    `json:"schema_version"`
	SnapshotDate          string                 `json:"snapshot_date"`
	Policy                policy                 `json:"policy"`
	Sources               []source               `json:"sources"`
	Requirements          []requirement          `json:"requirements"`
	CertificationFindings []certificationFinding `json:"certification_findings"`
	PreSubmitDeltaGate    deltaGate              `json:"pre_submit_delta_gate"`
}

type policy struct {
	DocumentVersion string `json:"document_version"`
	PublishDate     string `json:"publish_date"`
	EffectiveDate   string `json:"effective_date"`
}

type source struct {
	ID            string `json:"id"`
	Authority     string `json:"authority"`
	Title         string `json:"title"`
	URL           string `json:"url"`
	RetrievedDate string `json:"retrieved_date"`
}

type requirement struct {
	ID             string   `json:"id"`
	Classification string   `json:"classification"`
	Summary        string   `json:"summary"`
	SourceIDs      []string `json:"source_ids"`
	OwnerTasks     []string `json:"owner_tasks"`
	EvidenceTasks  []string `json:"evidence_tasks"`
}

type certificationFinding struct {
	Code             string   `json:"code"`
	Title            string   `json:"title"`
	Provenance       string   `json:"provenance"`
	RawReportPresent bool     `json:"raw_report_present"`
	PolicyAnchor     string   `json:"policy_anchor"`
	OwnerTasks       []string `json:"owner_tasks"`
	EvidenceTasks    []string `json:"evidence_tasks"`
}

type deltaGate struct {
	Required       bool     `json:"required"`
	OwnerTasks     []string `json:"owner_tasks"`
	RequiredFields []string `json:"required_fields"`
}

func TestCurrentStorePolicyBaselineIsStrictAndOwned(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	path := filepath.Join(root, "docs", "compliance", "store-policy-baseline-2026-07-14.json")
	validatedBaseline, err := LoadBaseline(path)
	if err != nil {
		t.Fatalf("production baseline validation: %v", err)
	}
	f, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()

	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	var got baseline
	if err := decoder.Decode(&got); err != nil {
		t.Fatalf("strictly decode policy baseline: %v", err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		t.Fatalf("policy baseline has trailing JSON: %v", err)
	}

	if got.SchemaVersion != 1 || got.SnapshotDate != wantSnapshotDate {
		t.Fatalf("baseline identity = schema %d date %q", got.SchemaVersion, got.SnapshotDate)
	}
	if got.Policy != (policy{
		DocumentVersion: wantPolicyVersion,
		PublishDate:     "2025-09-10",
		EffectiveDate:   "2025-10-14",
	}) {
		t.Fatalf("unexpected Store policy version record: %+v", got.Policy)
	}

	sourceIDs := make(map[string]struct{}, len(got.Sources))
	for _, source := range got.Sources {
		if source.ID == "" || source.Title == "" || source.RetrievedDate != wantSnapshotDate {
			t.Errorf("incomplete or stale source: %+v", source)
		}
		if _, duplicate := sourceIDs[source.ID]; duplicate {
			t.Errorf("duplicate source id %q", source.ID)
		}
		sourceIDs[source.ID] = struct{}{}

		parsed, err := url.Parse(source.URL)
		if err != nil || parsed.Scheme != "https" {
			t.Errorf("source %q is not an HTTPS URL: %q (%v)", source.ID, source.URL, err)
			continue
		}
		switch source.Authority {
		case "microsoft":
			if parsed.Hostname() != "learn.microsoft.com" {
				t.Errorf("Microsoft source %q uses host %q", source.ID, parsed.Hostname())
			}
		case "iarc":
			if parsed.Hostname() != "globalratings.com" {
				t.Errorf("IARC source %q uses host %q", source.ID, parsed.Hostname())
			}
		default:
			t.Errorf("source %q has unsupported authority %q", source.ID, source.Authority)
		}
	}

	wantRequirements := []string{"10.1", "10.3", "10.5", "10.6", "10.7", "11.11", "11.12"}
	seenRequirements := make(map[string]struct{}, len(got.Requirements))
	allTaskIDs := make(map[string]struct{})
	for _, requirement := range got.Requirements {
		if requirement.Classification != "mandatory" || requirement.Summary == "" {
			t.Errorf("requirement %q is not a complete mandatory mapping", requirement.ID)
		}
		if len(requirement.SourceIDs) == 0 || len(requirement.OwnerTasks) == 0 || len(requirement.EvidenceTasks) == 0 {
			t.Errorf("requirement %q lacks sources, owners, or evidence", requirement.ID)
		}
		if _, duplicate := seenRequirements[requirement.ID]; duplicate {
			t.Errorf("duplicate requirement %q", requirement.ID)
		}
		seenRequirements[requirement.ID] = struct{}{}
		for _, sourceID := range requirement.SourceIDs {
			if _, ok := sourceIDs[sourceID]; !ok {
				t.Errorf("requirement %q references missing source %q", requirement.ID, sourceID)
			}
		}
		collectTaskIDs(t, allTaskIDs, requirement.OwnerTasks)
		collectTaskIDs(t, allTaskIDs, requirement.EvidenceTasks)
	}
	for _, id := range wantRequirements {
		if _, ok := seenRequirements[id]; !ok {
			t.Errorf("missing required Store policy mapping %q", id)
		}
	}
	if len(seenRequirements) != len(wantRequirements) {
		t.Fatalf("unexpected requirement set: %+v", seenRequirements)
	}

	wantFindings := map[string]string{"10.3.1": "10.3", "10.1.1.3": "10.1"}
	for _, finding := range got.CertificationFindings {
		anchor, ok := wantFindings[finding.Code]
		if !ok || finding.PolicyAnchor != anchor {
			t.Errorf("unexpected finding mapping: %+v", finding)
		}
		if finding.RawReportPresent || !strings.Contains(finding.Provenance, "planning summary") {
			t.Errorf("finding %q overclaims raw Partner Center evidence", finding.Code)
		}
		if finding.Title == "" || len(finding.OwnerTasks) == 0 || len(finding.EvidenceTasks) == 0 {
			t.Errorf("finding %q lacks closure ownership", finding.Code)
		}
		collectTaskIDs(t, allTaskIDs, finding.OwnerTasks)
		collectTaskIDs(t, allTaskIDs, finding.EvidenceTasks)
		delete(wantFindings, finding.Code)
	}
	if len(wantFindings) != 0 {
		t.Fatalf("missing certification findings: %+v", wantFindings)
	}

	if !got.PreSubmitDeltaGate.Required {
		t.Fatal("pre-submit policy delta gate is not mandatory")
	}
	for _, field := range []string{
		"verification_date", "reviewer", "policy_version", "effective_date",
		"source_urls", "source_content_hashes", "changed_requirements",
		"created_or_reopened_task_ids", "decision",
	} {
		if !slices.Contains(got.PreSubmitDeltaGate.RequiredFields, field) {
			t.Errorf("delta gate does not require %q", field)
		}
	}
	collectTaskIDs(t, allTaskIDs, got.PreSubmitDeltaGate.OwnerTasks)

	documentPath := filepath.Join(root, "docs", "analysis", "store-policy-baseline-2026-07-14.md")
	document, err := os.ReadFile(documentPath)
	if err != nil {
		t.Fatal(err)
	}
	text := string(document)
	for taskID := range allTaskIDs {
		if !strings.Contains(text, taskID) {
			t.Errorf("matrix JSON task %q is absent from the human-readable matrix", taskID)
		}
	}
	for _, guard := range []string{
		"recommendations", "no raw Partner Center report artifact",
		"does not select the IARC result", "Mandatory pre-submit delta gate",
		"`10.1.1.3` is the certification finding label",
	} {
		if !strings.Contains(text, guard) {
			t.Errorf("human-readable matrix lacks overclaim guard %q", guard)
		}
	}

	recordPath := filepath.Join(root, "docs", "compliance", "store-policy-pre-submit.json")
	record, err := LoadPreSubmitRecord(recordPath)
	if err != nil {
		t.Fatalf("load checked-in pre-submit record: %v", err)
	}
	verifiedAt, err := time.Parse(time.RFC3339, record.VerifiedAt)
	if err != nil {
		t.Fatal(err)
	}
	if err := record.Validate(validatedBaseline, verifiedAt, 24*time.Hour, "", false); err != nil {
		t.Fatalf("checked-in hold record is not structurally valid: %v", err)
	}
	if err := record.Validate(validatedBaseline, verifiedAt, 24*time.Hour, "v-test", true); err == nil || !strings.Contains(err.Error(), "want proceed") {
		t.Fatalf("hold record proceed validation error = %v", err)
	}

	workflow, err := os.ReadFile(filepath.Join(root, ".github", "workflows", "store-submit.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, gate := range []string{"store-policy-check", "--require-proceed", "--max-age 24h", "--tag \"${{ inputs.tag }}\""} {
		if !strings.Contains(string(workflow), gate) {
			t.Errorf("Store submission workflow can bypass policy gate %q", gate)
		}
	}
}

func TestPreSubmitProceedRecordIsTagBoundFreshAndDeltaOwned(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	baseline, err := LoadBaseline(filepath.Join(root, "docs", "compliance", "store-policy-baseline-2026-07-14.json"))
	if err != nil {
		t.Fatal(err)
	}
	record, err := LoadPreSubmitRecord(filepath.Join(root, "docs", "compliance", "store-policy-pre-submit.json"))
	if err != nil {
		t.Fatal(err)
	}
	verifiedAt, err := time.Parse(time.RFC3339, record.VerifiedAt)
	if err != nil {
		t.Fatal(err)
	}
	record.Decision = "proceed"
	record.SubmissionTag = "v1.2.3"
	if err := record.Validate(baseline, verifiedAt.Add(23*time.Hour), 24*time.Hour, "v1.2.3", true); err != nil {
		t.Fatalf("complete fresh proceed record: %v", err)
	}
	if err := record.Validate(baseline, verifiedAt.Add(25*time.Hour), 24*time.Hour, "v1.2.3", true); err == nil || !strings.Contains(err.Error(), "outside the allowed window") {
		t.Fatalf("stale record error = %v", err)
	}
	if err := record.Validate(baseline, verifiedAt, 24*time.Hour, "v9.9.9", true); err == nil || !strings.Contains(err.Error(), "submission_tag") {
		t.Fatalf("wrong-tag record error = %v", err)
	}
	record.ChangedRequirements = []string{"10.3 changed"}
	if err := record.Validate(baseline, verifiedAt, 24*time.Hour, "v1.2.3", true); err == nil || !strings.Contains(err.Error(), "created or reopened task IDs") {
		t.Fatalf("unowned delta error = %v", err)
	}
	record.CreatedOrReopenedTaskIDs = []string{"TASK-260712-example"}
	if err := record.Validate(baseline, verifiedAt, 24*time.Hour, "v1.2.3", true); err != nil {
		t.Fatalf("owned delta record: %v", err)
	}
}

func collectTaskIDs(t *testing.T, seen map[string]struct{}, taskIDs []string) {
	t.Helper()
	for _, taskID := range taskIDs {
		if !strings.HasPrefix(taskID, "TASK-") {
			t.Errorf("invalid owner or evidence task id %q", taskID)
		}
		seen[taskID] = struct{}{}
	}
}
