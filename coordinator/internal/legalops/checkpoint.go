package legalops

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/mail"
	"net/url"
	"os"
	"regexp"
	"sort"
	"strings"
	"time"
)

const (
	SchemaVersion = 1
	TaskID        = "TASK-260712-16zfvu"
)

var RequiredInputIDs = []string{
	"legal_identity_and_controller",
	"contacts_and_public_urls",
	"hosting_and_data_locations",
	"markets_age_and_disputes",
	"moderation_ownership_and_response",
	"partner_center_and_submission",
	"policy_review_and_configuration",
}

var placeholderPattern = regexp.MustCompile(`(?i)(^|[^a-z])(todo|tbd|placeholder|changeme|change[ _-]me|example\.com|insert[ _-]here|not[ _-]set|unknown|pending)([^a-z]|$)`)

type Checkpoint struct {
	SchemaVersion   int             `json:"schema_version"`
	TaskID          string          `json:"task_id"`
	ObservedAt      string          `json:"observed_at"`
	PublicationGate PublicationGate `json:"publication_gate"`
	Inputs          []Input         `json:"inputs"`
}

type PublicationGate struct {
	State      string  `json:"state"`
	ApprovedBy *string `json:"approved_by"`
	ApprovedAt *string `json:"approved_at"`
}

type Input struct {
	ID         string          `json:"id"`
	Status     string          `json:"status"`
	Value      json.RawMessage `json:"value"`
	Candidate  json.RawMessage `json:"candidate"`
	Owner      *string         `json:"owner"`
	ApprovedBy *string         `json:"approved_by"`
	ApprovedAt *string         `json:"approved_at"`
	Evidence   []string        `json:"evidence"`
}

func Load(path string) (Checkpoint, error) {
	f, err := os.Open(path)
	if err != nil {
		return Checkpoint{}, fmt.Errorf("open legal/operations checkpoint: %w", err)
	}
	defer f.Close()
	return Decode(f)
}

func Decode(reader io.Reader) (Checkpoint, error) {
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	var checkpoint Checkpoint
	if err := decoder.Decode(&checkpoint); err != nil {
		return Checkpoint{}, fmt.Errorf("decode legal/operations checkpoint: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		if err == nil {
			err = errors.New("trailing JSON value")
		}
		return Checkpoint{}, fmt.Errorf("decode legal/operations checkpoint: %w", err)
	}
	return checkpoint, nil
}

func (checkpoint Checkpoint) Validate(requireApproved bool) error {
	var problems []error
	if checkpoint.SchemaVersion != SchemaVersion {
		problems = append(problems, fmt.Errorf("schema_version=%d, want %d", checkpoint.SchemaVersion, SchemaVersion))
	}
	if checkpoint.TaskID != TaskID {
		problems = append(problems, fmt.Errorf("task_id=%q, want %q", checkpoint.TaskID, TaskID))
	}
	if _, err := time.Parse("2006-01-02", checkpoint.ObservedAt); err != nil {
		problems = append(problems, fmt.Errorf("observed_at must be YYYY-MM-DD: %w", err))
	}

	seen := make(map[string]bool, len(checkpoint.Inputs))
	approvedCount := 0
	for index, input := range checkpoint.Inputs {
		if seen[input.ID] {
			problems = append(problems, fmt.Errorf("duplicate input id %q", input.ID))
		}
		seen[input.ID] = true
		if index >= len(RequiredInputIDs) || input.ID != RequiredInputIDs[index] {
			problems = append(problems, fmt.Errorf("input %d id=%q, want stable order %q", index, input.ID, requiredID(index)))
		}
		if len(input.Evidence) == 0 {
			problems = append(problems, fmt.Errorf("input %q has no evidence", input.ID))
		}
		switch input.Status {
		case "observed_unapproved":
			if isNull(input.Candidate) {
				problems = append(problems, fmt.Errorf("input %q is observed without a candidate", input.ID))
			}
			problems = append(problems, validateUnapproved(input)...)
		case "missing":
			problems = append(problems, validateUnapproved(input)...)
		case "approved":
			approvedCount++
			problems = append(problems, validateApproved(input)...)
		default:
			problems = append(problems, fmt.Errorf("input %q has invalid status %q", input.ID, input.Status))
		}
	}
	for _, id := range RequiredInputIDs {
		if !seen[id] {
			problems = append(problems, fmt.Errorf("required input %q is absent", id))
		}
	}

	allApproved := approvedCount == len(RequiredInputIDs) && len(checkpoint.Inputs) == len(RequiredInputIDs)
	if allApproved {
		if checkpoint.PublicationGate.State != "approved" {
			problems = append(problems, errors.New("all inputs are approved but publication gate is not approved"))
		}
		problems = append(problems, validateApproval(
			"publication_gate", checkpoint.PublicationGate.ApprovedBy, checkpoint.PublicationGate.ApprovedAt,
		)...)
	} else {
		if checkpoint.PublicationGate.State != "blocked" {
			problems = append(problems, errors.New("publication gate must remain blocked while any input is unapproved"))
		}
		if checkpoint.PublicationGate.ApprovedBy != nil || checkpoint.PublicationGate.ApprovedAt != nil {
			problems = append(problems, errors.New("blocked publication gate cannot carry approval metadata"))
		}
	}
	if requireApproved && !allApproved {
		problems = append(problems, fmt.Errorf("external publication approval is required; unresolved: %s", strings.Join(checkpoint.Unresolved(), ", ")))
	}
	return errors.Join(problems...)
}

func (checkpoint Checkpoint) Unresolved() []string {
	unresolved := make([]string, 0, len(RequiredInputIDs))
	status := make(map[string]string, len(checkpoint.Inputs))
	for _, input := range checkpoint.Inputs {
		status[input.ID] = input.Status
	}
	for _, id := range RequiredInputIDs {
		if status[id] != "approved" {
			unresolved = append(unresolved, id)
		}
	}
	return unresolved
}

func requiredID(index int) string {
	if index >= len(RequiredInputIDs) {
		return "<no extra input>"
	}
	return RequiredInputIDs[index]
}

func validateUnapproved(input Input) []error {
	var problems []error
	if !isNull(input.Value) {
		problems = append(problems, fmt.Errorf("unapproved input %q cannot carry a publishable value", input.ID))
	}
	if input.Owner != nil || input.ApprovedBy != nil || input.ApprovedAt != nil {
		problems = append(problems, fmt.Errorf("unapproved input %q cannot carry owner or approval metadata", input.ID))
	}
	return problems
}

func validateApproved(input Input) []error {
	var problems []error
	if isNull(input.Value) {
		problems = append(problems, fmt.Errorf("approved input %q has no value", input.ID))
	} else {
		var value any
		if err := json.Unmarshal(input.Value, &value); err != nil {
			problems = append(problems, fmt.Errorf("approved input %q value: %w", input.ID, err))
		} else {
			problems = append(problems, validateValue(input.ID, value)...)
		}
	}
	problems = append(problems, validateApproval(input.ID, input.ApprovedBy, input.ApprovedAt)...)
	if input.Owner == nil || strings.TrimSpace(*input.Owner) == "" {
		problems = append(problems, fmt.Errorf("approved input %q has no accountable owner", input.ID))
	} else if placeholderPattern.MatchString(*input.Owner) {
		problems = append(problems, fmt.Errorf("approved input %q owner contains a placeholder", input.ID))
	}
	return problems
}

func validateApproval(label string, approvedBy, approvedAt *string) []error {
	var problems []error
	if approvedBy == nil || strings.TrimSpace(*approvedBy) == "" || placeholderPattern.MatchString(valueOrEmpty(approvedBy)) {
		problems = append(problems, fmt.Errorf("%s has no real approver", label))
	}
	if approvedAt == nil {
		problems = append(problems, fmt.Errorf("%s has no approval timestamp", label))
	} else if _, err := time.Parse(time.RFC3339, *approvedAt); err != nil {
		problems = append(problems, fmt.Errorf("%s approval timestamp must be RFC3339: %w", label, err))
	}
	return problems
}

func validateValue(path string, value any) []error {
	var problems []error
	switch typed := value.(type) {
	case map[string]any:
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			problems = append(problems, validateValue(path+"."+key, typed[key])...)
		}
	case []any:
		if len(typed) == 0 {
			problems = append(problems, fmt.Errorf("approved value %s is an empty list", path))
		}
		for index, item := range typed {
			problems = append(problems, validateValue(fmt.Sprintf("%s[%d]", path, index), item)...)
		}
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			problems = append(problems, fmt.Errorf("approved value %s is empty", path))
		} else if placeholderPattern.MatchString(trimmed) {
			problems = append(problems, fmt.Errorf("approved value %s contains a placeholder", path))
		}
		lowerPath := strings.ToLower(path)
		if strings.HasSuffix(lowerPath, "_url") {
			parsed, err := url.Parse(trimmed)
			if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
				problems = append(problems, fmt.Errorf("approved value %s must be an absolute credential-free HTTPS URL", path))
			}
		}
		if strings.HasSuffix(lowerPath, "_email") {
			address, err := mail.ParseAddress(trimmed)
			if err != nil || address.Address != trimmed || address.Name != "" {
				problems = append(problems, fmt.Errorf("approved value %s must be one plain email address", path))
			}
		}
	case nil:
		problems = append(problems, fmt.Errorf("approved value %s is null", path))
	}
	return problems
}

func isNull(raw json.RawMessage) bool {
	return len(bytes.TrimSpace(raw)) == 0 || bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
