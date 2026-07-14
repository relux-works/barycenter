package storepolicy

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"slices"
	"strings"
	"time"
)

const SchemaVersion = 1

var RequiredPolicySections = []string{"10.1", "10.3", "10.5", "10.6", "10.7", "11.11", "11.12"}

type Baseline struct {
	SchemaVersion         int                    `json:"schema_version"`
	SnapshotDate          string                 `json:"snapshot_date"`
	Policy                Policy                 `json:"policy"`
	Sources               []Source               `json:"sources"`
	Requirements          []Requirement          `json:"requirements"`
	CertificationFindings []CertificationFinding `json:"certification_findings"`
	PreSubmitDeltaGate    DeltaGate              `json:"pre_submit_delta_gate"`
}

type Policy struct {
	DocumentVersion string `json:"document_version"`
	PublishDate     string `json:"publish_date"`
	EffectiveDate   string `json:"effective_date"`
}

type Source struct {
	ID            string `json:"id"`
	Authority     string `json:"authority"`
	Title         string `json:"title"`
	URL           string `json:"url"`
	RetrievedDate string `json:"retrieved_date"`
}

type Requirement struct {
	ID             string   `json:"id"`
	Classification string   `json:"classification"`
	Summary        string   `json:"summary"`
	SourceIDs      []string `json:"source_ids"`
	OwnerTasks     []string `json:"owner_tasks"`
	EvidenceTasks  []string `json:"evidence_tasks"`
}

type CertificationFinding struct {
	Code             string   `json:"code"`
	Title            string   `json:"title"`
	Provenance       string   `json:"provenance"`
	RawReportPresent bool     `json:"raw_report_present"`
	PolicyAnchor     string   `json:"policy_anchor"`
	OwnerTasks       []string `json:"owner_tasks"`
	EvidenceTasks    []string `json:"evidence_tasks"`
}

type DeltaGate struct {
	Required       bool     `json:"required"`
	OwnerTasks     []string `json:"owner_tasks"`
	RequiredFields []string `json:"required_fields"`
}

type PreSubmitRecord struct {
	SchemaVersion            int               `json:"schema_version"`
	BaselineSnapshotDate     string            `json:"baseline_snapshot_date"`
	VerificationDate         string            `json:"verification_date"`
	VerifiedAt               string            `json:"verified_at"`
	Reviewer                 string            `json:"reviewer"`
	PolicyVersion            string            `json:"policy_version"`
	EffectiveDate            string            `json:"effective_date"`
	SourceURLs               []string          `json:"source_urls"`
	SourceContentHashes      map[string]string `json:"source_content_hashes"`
	ChangedRequirements      []string          `json:"changed_requirements"`
	CreatedOrReopenedTaskIDs []string          `json:"created_or_reopened_task_ids"`
	Decision                 string            `json:"decision"`
	SubmissionTag            string            `json:"submission_tag"`
	Notes                    string            `json:"notes"`
}

func LoadBaseline(path string) (Baseline, error) {
	var value Baseline
	if err := decodeStrictFile(path, &value); err != nil {
		return Baseline{}, fmt.Errorf("load Store policy baseline: %w", err)
	}
	if err := value.Validate(); err != nil {
		return Baseline{}, fmt.Errorf("validate Store policy baseline: %w", err)
	}
	return value, nil
}

func LoadPreSubmitRecord(path string) (PreSubmitRecord, error) {
	var value PreSubmitRecord
	if err := decodeStrictFile(path, &value); err != nil {
		return PreSubmitRecord{}, fmt.Errorf("load Store policy pre-submit record: %w", err)
	}
	return value, nil
}

func (b Baseline) Validate() error {
	if b.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schema_version = %d, want %d", b.SchemaVersion, SchemaVersion)
	}
	if _, err := time.Parse(time.DateOnly, b.SnapshotDate); err != nil {
		return fmt.Errorf("snapshot_date: %w", err)
	}
	if b.Policy.DocumentVersion == "" {
		return errors.New("policy document_version is required")
	}
	for name, value := range map[string]string{
		"policy publish_date":   b.Policy.PublishDate,
		"policy effective_date": b.Policy.EffectiveDate,
	} {
		if _, err := time.Parse(time.DateOnly, value); err != nil {
			return fmt.Errorf("%s: %w", name, err)
		}
	}

	sources := make(map[string]struct{}, len(b.Sources))
	for index, source := range b.Sources {
		if source.ID == "" || source.Title == "" || source.RetrievedDate != b.SnapshotDate {
			return fmt.Errorf("source %d is incomplete or not retrieved on the snapshot date", index)
		}
		if _, duplicate := sources[source.ID]; duplicate {
			return fmt.Errorf("duplicate source id %q", source.ID)
		}
		sources[source.ID] = struct{}{}
		parsed, err := url.Parse(source.URL)
		if err != nil || parsed.Scheme != "https" {
			return fmt.Errorf("source %q is not an HTTPS URL", source.ID)
		}
		switch source.Authority {
		case "microsoft":
			if parsed.Hostname() != "learn.microsoft.com" {
				return fmt.Errorf("Microsoft source %q uses host %q", source.ID, parsed.Hostname())
			}
		case "iarc":
			if parsed.Hostname() != "globalratings.com" {
				return fmt.Errorf("IARC source %q uses host %q", source.ID, parsed.Hostname())
			}
		default:
			return fmt.Errorf("source %q has unsupported authority %q", source.ID, source.Authority)
		}
	}

	requirements := make(map[string]struct{}, len(b.Requirements))
	for _, requirement := range b.Requirements {
		if requirement.Classification != "mandatory" || requirement.Summary == "" {
			return fmt.Errorf("requirement %q is not a complete mandatory mapping", requirement.ID)
		}
		if len(requirement.SourceIDs) == 0 || len(requirement.OwnerTasks) == 0 || len(requirement.EvidenceTasks) == 0 {
			return fmt.Errorf("requirement %q lacks sources, owners, or evidence", requirement.ID)
		}
		if _, duplicate := requirements[requirement.ID]; duplicate {
			return fmt.Errorf("duplicate requirement %q", requirement.ID)
		}
		requirements[requirement.ID] = struct{}{}
		for _, sourceID := range requirement.SourceIDs {
			if _, ok := sources[sourceID]; !ok {
				return fmt.Errorf("requirement %q references missing source %q", requirement.ID, sourceID)
			}
		}
		if err := validateTaskIDs(requirement.OwnerTasks); err != nil {
			return fmt.Errorf("requirement %q owners: %w", requirement.ID, err)
		}
		if err := validateTaskIDs(requirement.EvidenceTasks); err != nil {
			return fmt.Errorf("requirement %q evidence: %w", requirement.ID, err)
		}
	}
	for _, id := range RequiredPolicySections {
		if _, ok := requirements[id]; !ok {
			return fmt.Errorf("missing required policy mapping %q", id)
		}
	}
	if len(requirements) != len(RequiredPolicySections) {
		return fmt.Errorf("unexpected policy requirement count %d", len(requirements))
	}

	wantFindings := map[string]string{"10.3.1": "10.3", "10.1.1.3": "10.1"}
	for _, finding := range b.CertificationFindings {
		anchor, ok := wantFindings[finding.Code]
		if !ok || finding.PolicyAnchor != anchor {
			return fmt.Errorf("unexpected certification finding %q -> %q", finding.Code, finding.PolicyAnchor)
		}
		if finding.RawReportPresent || !strings.Contains(finding.Provenance, "planning summary") {
			return fmt.Errorf("finding %q overclaims raw Partner Center evidence", finding.Code)
		}
		if finding.Title == "" || len(finding.OwnerTasks) == 0 || len(finding.EvidenceTasks) == 0 {
			return fmt.Errorf("finding %q lacks closure ownership", finding.Code)
		}
		if err := validateTaskIDs(append(slices.Clone(finding.OwnerTasks), finding.EvidenceTasks...)); err != nil {
			return fmt.Errorf("finding %q: %w", finding.Code, err)
		}
		delete(wantFindings, finding.Code)
	}
	if len(wantFindings) != 0 {
		return fmt.Errorf("missing certification findings: %v", wantFindings)
	}

	if !b.PreSubmitDeltaGate.Required {
		return errors.New("pre-submit policy delta gate must be required")
	}
	if err := validateTaskIDs(b.PreSubmitDeltaGate.OwnerTasks); err != nil {
		return fmt.Errorf("pre-submit gate owners: %w", err)
	}
	for _, field := range []string{
		"verification_date", "reviewer", "policy_version", "effective_date",
		"source_urls", "source_content_hashes", "changed_requirements",
		"created_or_reopened_task_ids", "decision",
	} {
		if !slices.Contains(b.PreSubmitDeltaGate.RequiredFields, field) {
			return fmt.Errorf("pre-submit gate does not require %q", field)
		}
	}
	return nil
}

func (r PreSubmitRecord) Validate(b Baseline, now time.Time, maxAge time.Duration, expectedTag string, requireProceed bool) error {
	if r.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schema_version = %d, want %d", r.SchemaVersion, SchemaVersion)
	}
	if r.BaselineSnapshotDate != b.SnapshotDate {
		return fmt.Errorf("baseline_snapshot_date = %q, want %q", r.BaselineSnapshotDate, b.SnapshotDate)
	}
	verifiedAt, err := time.Parse(time.RFC3339, r.VerifiedAt)
	if err != nil {
		return fmt.Errorf("verified_at: %w", err)
	}
	if r.VerificationDate != verifiedAt.Format(time.DateOnly) {
		return errors.New("verification_date must match the date in verified_at")
	}
	if r.Reviewer == "" || r.PolicyVersion == "" || r.EffectiveDate == "" || r.Notes == "" {
		return errors.New("reviewer, policy_version, effective_date, and notes are required")
	}
	if _, err := time.Parse(time.DateOnly, r.EffectiveDate); err != nil {
		return fmt.Errorf("effective_date: %w", err)
	}
	if r.Decision != "hold" && r.Decision != "proceed" {
		return fmt.Errorf("decision %q is not hold or proceed", r.Decision)
	}
	if err := validateTaskIDs(r.CreatedOrReopenedTaskIDs); err != nil {
		return fmt.Errorf("created_or_reopened_task_ids: %w", err)
	}

	wantURLs := make(map[string]struct{}, len(b.Sources))
	for _, source := range b.Sources {
		wantURLs[source.URL] = struct{}{}
	}
	gotURLs := make(map[string]struct{}, len(r.SourceURLs))
	for _, sourceURL := range r.SourceURLs {
		if _, duplicate := gotURLs[sourceURL]; duplicate {
			return fmt.Errorf("duplicate source URL %q", sourceURL)
		}
		gotURLs[sourceURL] = struct{}{}
		if _, ok := wantURLs[sourceURL]; !ok {
			return fmt.Errorf("pre-submit record contains unapproved source URL %q", sourceURL)
		}
		hash := r.SourceContentHashes[sourceURL]
		if len(hash) != 64 || strings.Trim(hash, "0123456789abcdef") != "" {
			return fmt.Errorf("source %q lacks a lowercase SHA-256 content hash", sourceURL)
		}
	}
	if len(gotURLs) != len(wantURLs) || len(r.SourceContentHashes) != len(wantURLs) {
		return errors.New("pre-submit record must contain every baseline source and exactly one hash per source")
	}
	for sourceURL := range wantURLs {
		if _, ok := gotURLs[sourceURL]; !ok {
			return fmt.Errorf("pre-submit record is missing source %q", sourceURL)
		}
	}

	if !requireProceed {
		return nil
	}
	if r.Decision != "proceed" {
		return fmt.Errorf("pre-submit decision = %q, want proceed", r.Decision)
	}
	if r.PolicyVersion != b.Policy.DocumentVersion || r.EffectiveDate != b.Policy.EffectiveDate {
		return errors.New("proceed record policy version/effective date differs from the checked-in baseline; update the baseline and track the delta")
	}
	if r.SubmissionTag == "" || r.SubmissionTag != expectedTag {
		return fmt.Errorf("submission_tag = %q, want %q", r.SubmissionTag, expectedTag)
	}
	if maxAge <= 0 {
		return errors.New("positive max age is required for a proceed decision")
	}
	age := now.Sub(verifiedAt)
	if age < -5*time.Minute || age > maxAge {
		return fmt.Errorf("pre-submit verification age %s is outside the allowed window", age.Round(time.Second))
	}
	if len(r.ChangedRequirements) > 0 && len(r.CreatedOrReopenedTaskIDs) == 0 {
		return errors.New("changed requirements require created or reopened task IDs")
	}
	return nil
}

func decodeStrictFile(path string, value any) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	decoder := json.NewDecoder(f)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return err
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("trailing JSON: %v", err)
	}
	return nil
}

func validateTaskIDs(taskIDs []string) error {
	seen := make(map[string]struct{}, len(taskIDs))
	for _, taskID := range taskIDs {
		if !strings.HasPrefix(taskID, "TASK-") {
			return fmt.Errorf("invalid task ID %q", taskID)
		}
		if _, duplicate := seen[taskID]; duplicate {
			return fmt.Errorf("duplicate task ID %q", taskID)
		}
		seen[taskID] = struct{}{}
	}
	return nil
}
