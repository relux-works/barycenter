package policypack

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"time"

	"relux.works/duet/coordinator/internal/legalops"
)

const (
	SchemaVersion = 1
	TaskID        = "TASK-260712-1epb3a"
)

var (
	sectionPattern     = regexp.MustCompile(`(?m)^## ([PCTU]-[0-9]{2})\.`)
	taskIDPattern      = regexp.MustCompile(`^TASK-[0-9]{6}-[a-z0-9]+$`)
	placeholderPattern = regexp.MustCompile(`(?i)\b(todo|tbd|changeme|insert here|example\.com)\b`)
	unsupportedClaims  = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\bpulsar (is|uses) (fully )?(anonymous|end[- ]to[- ]end encrypted|e2ee)\b`),
		regexp.MustCompile(`(?i)\bguarantees? (delivery|availability|deletion|recovery)\b`),
		regexp.MustCompile(`(?i)\binstant(ly)? (erases?|deletes?).{0,60}\bbackup`),
	}
)

var artifactContract = []struct {
	Kind       string
	URLKey     string
	ENPath     string
	RUPath     string
	Prefix     string
	SectionMax int
}{
	{"privacy", "privacy", "docs/legal/en/privacy.md", "docs/legal/ru/privacy.md", "P", 14},
	{"terms", "terms", "docs/legal/en/terms.md", "docs/legal/ru/terms.md", "T", 15},
	{"content_guidelines", "content_guidelines", "docs/legal/en/content-guidelines.md", "docs/legal/ru/content-guidelines.md", "C", 11},
	{"upload_rights_notice", "terms", "docs/legal/en/upload-rights-notice.md", "docs/legal/ru/upload-rights-notice.md", "U", 4},
}

var requiredSourceIDs = []string{
	"approved-inputs", "spec-privacy-ugc", "media-retention", "moderation-control",
	"backup-contract", "microsoft-store-7.19", "microsoft-age-ratings",
	"telegram-privacy", "telegram-bot-terms", "spotify-policy", "spotify-terms",
	"ftc-coppa", "eu-privacy-notice", "california-ccpa",
}

type Pack struct {
	SchemaVersion       int               `json:"schema_version"`
	TaskID              string            `json:"task_id"`
	Version             string            `json:"version"`
	EffectiveDate       string            `json:"effective_date"`
	RetrievedAt         string            `json:"retrieved_at"`
	ControllingLanguage string            `json:"controlling_language"`
	ApprovedInputsPath  string            `json:"approved_inputs_path"`
	Review              Review            `json:"review"`
	CanonicalURLs       map[string]string `json:"canonical_urls"`
	Artifacts           []Artifact        `json:"artifacts"`
	Sources             []Source          `json:"sources"`
	Traceability        []Trace           `json:"traceability"`
	SurfaceConsumers    []SurfaceConsumer `json:"surface_consumers"`
	DeltaTriggers       []string          `json:"delta_triggers"`
}

type Review struct {
	CounselReviewRequired   bool    `json:"counsel_review_required"`
	EnglishReviewer         string  `json:"english_reviewer"`
	RussianReviewer         string  `json:"russian_reviewer"`
	ExactContentReviewState string  `json:"exact_content_review_state"`
	PublicationDecision     string  `json:"publication_decision"`
	ApprovedBy              *string `json:"approved_by"`
	ApprovedAt              *string `json:"approved_at"`
	Reason                  string  `json:"reason"`
}

type Artifact struct {
	Kind         string   `json:"kind"`
	CanonicalURL string   `json:"canonical_url"`
	ENPath       string   `json:"en_path"`
	RUPath       string   `json:"ru_path"`
	ENSHA256     string   `json:"en_sha256"`
	RUSHA256     string   `json:"ru_sha256"`
	Sections     []string `json:"sections"`
}

type Source struct {
	ID          string `json:"id"`
	Authority   string `json:"authority"`
	URL         string `json:"url"`
	RetrievedAt string `json:"retrieved_at"`
}

type Trace struct {
	Sections []string `json:"sections"`
	Evidence []string `json:"evidence"`
}

type SurfaceConsumer struct {
	Surface   string   `json:"surface"`
	Tasks     []string `json:"tasks"`
	Artifacts []string `json:"artifacts"`
}

func Load(path string) (Pack, error) {
	file, err := os.Open(path)
	if err != nil {
		return Pack{}, fmt.Errorf("open policy pack: %w", err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	var pack Pack
	if err := decoder.Decode(&pack); err != nil {
		return Pack{}, fmt.Errorf("decode policy pack: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Pack{}, fmt.Errorf("decode policy pack: trailing JSON: %v", err)
	}
	return pack, nil
}

func (p Pack) Validate(repoRoot string, inputs legalops.Checkpoint, requireProceed bool) error {
	var problems []error
	if p.SchemaVersion != SchemaVersion {
		problems = append(problems, fmt.Errorf("schema_version=%d, want %d", p.SchemaVersion, SchemaVersion))
	}
	if p.TaskID != TaskID {
		problems = append(problems, fmt.Errorf("task_id=%q, want %q", p.TaskID, TaskID))
	}
	if p.Version == "" || p.ControllingLanguage != "en" {
		problems = append(problems, errors.New("version is required and controlling_language must be en"))
	}
	for name, value := range map[string]string{"effective_date": p.EffectiveDate, "retrieved_at": p.RetrievedAt} {
		if _, err := time.Parse(time.DateOnly, value); err != nil {
			problems = append(problems, fmt.Errorf("%s must be YYYY-MM-DD: %w", name, err))
		}
	}
	if p.ApprovedInputsPath != "docs/compliance/legal-ops-inputs.json" {
		problems = append(problems, fmt.Errorf("approved_inputs_path=%q", p.ApprovedInputsPath))
	}
	if err := inputs.Validate(true); err != nil {
		problems = append(problems, fmt.Errorf("approved legal/operations inputs: %w", err))
	}

	approved := approvedValues(inputs, &problems)
	problems = append(problems, p.validateReview(approved, requireProceed)...)
	problems = append(problems, p.validateURLs(approved)...)

	sectionOwner := make(map[string]string)
	artifactKinds := make(map[string]struct{})
	corpus := strings.Builder{}
	if len(p.Artifacts) != len(artifactContract) {
		problems = append(problems, fmt.Errorf("artifact count=%d, want %d", len(p.Artifacts), len(artifactContract)))
	}
	for index, contract := range artifactContract {
		if index >= len(p.Artifacts) {
			break
		}
		artifact := p.Artifacts[index]
		if artifact.Kind != contract.Kind || artifact.ENPath != contract.ENPath || artifact.RUPath != contract.RUPath {
			problems = append(problems, fmt.Errorf("artifact %d does not match stable %s path contract", index, contract.Kind))
		}
		if artifact.CanonicalURL != p.CanonicalURLs[contract.URLKey] {
			problems = append(problems, fmt.Errorf("artifact %q canonical URL does not match %s", artifact.Kind, contract.URLKey))
		}
		if _, duplicate := artifactKinds[artifact.Kind]; duplicate {
			problems = append(problems, fmt.Errorf("duplicate artifact kind %q", artifact.Kind))
		}
		artifactKinds[artifact.Kind] = struct{}{}

		wantSections := numberedSections(contract.Prefix, contract.SectionMax)
		if !slices.Equal(artifact.Sections, wantSections) {
			problems = append(problems, fmt.Errorf("artifact %q sections=%v, want %v", artifact.Kind, artifact.Sections, wantSections))
		}
		enBytes, enSections, err := validateDocument(repoRoot, artifact.ENPath, artifact.ENSHA256)
		if err != nil {
			problems = append(problems, fmt.Errorf("artifact %q English: %w", artifact.Kind, err))
		}
		ruBytes, ruSections, err := validateDocument(repoRoot, artifact.RUPath, artifact.RUSHA256)
		if err != nil {
			problems = append(problems, fmt.Errorf("artifact %q Russian: %w", artifact.Kind, err))
		}
		if !slices.Equal(enSections, artifact.Sections) || !slices.Equal(ruSections, artifact.Sections) {
			problems = append(problems, fmt.Errorf("artifact %q EN/RU section IDs do not match manifest", artifact.Kind))
		}
		for _, section := range artifact.Sections {
			if owner, duplicate := sectionOwner[section]; duplicate {
				problems = append(problems, fmt.Errorf("section %q belongs to both %s and %s", section, owner, artifact.Kind))
			}
			sectionOwner[section] = artifact.Kind
		}
		corpus.Write(enBytes)
		corpus.WriteByte('\n')
		corpus.Write(ruBytes)
		corpus.WriteByte('\n')
	}
	problems = append(problems, validatePublicFacts(corpus.String(), approved)...)

	sourceIDs := make(map[string]struct{}, len(p.Sources))
	for _, source := range p.Sources {
		if source.ID == "" || source.Authority == "" || source.RetrievedAt != p.RetrievedAt {
			problems = append(problems, fmt.Errorf("source %q is incomplete or stale", source.ID))
			continue
		}
		if _, duplicate := sourceIDs[source.ID]; duplicate {
			problems = append(problems, fmt.Errorf("duplicate source %q", source.ID))
		}
		sourceIDs[source.ID] = struct{}{}
		if err := validateSource(repoRoot, source); err != nil {
			problems = append(problems, err)
		}
	}
	if len(sourceIDs) != len(requiredSourceIDs) {
		problems = append(problems, fmt.Errorf("source count=%d, want %d", len(sourceIDs), len(requiredSourceIDs)))
	}
	for _, sourceID := range requiredSourceIDs {
		if _, ok := sourceIDs[sourceID]; !ok {
			problems = append(problems, fmt.Errorf("required source %q is absent", sourceID))
		}
	}

	traced := make(map[string]struct{}, len(sectionOwner))
	for _, trace := range p.Traceability {
		if len(trace.Sections) == 0 || len(trace.Evidence) == 0 {
			problems = append(problems, errors.New("traceability row must have sections and evidence"))
		}
		for _, section := range trace.Sections {
			if _, ok := sectionOwner[section]; !ok {
				problems = append(problems, fmt.Errorf("traceability references unknown section %q", section))
			}
			if _, duplicate := traced[section]; duplicate {
				problems = append(problems, fmt.Errorf("section %q has duplicate traceability", section))
			}
			traced[section] = struct{}{}
		}
		for _, evidence := range trace.Evidence {
			if _, ok := sourceIDs[evidence]; !ok {
				problems = append(problems, fmt.Errorf("traceability references unknown evidence %q", evidence))
			}
		}
	}
	for section := range sectionOwner {
		if _, ok := traced[section]; !ok {
			problems = append(problems, fmt.Errorf("section %q has no factual traceability", section))
		}
	}

	wantSurfaces := []string{"website", "desktop_app", "telegram", "microsoft_store", "moderation_operations"}
	gotSurfaces := make([]string, 0, len(p.SurfaceConsumers))
	for _, consumer := range p.SurfaceConsumers {
		gotSurfaces = append(gotSurfaces, consumer.Surface)
		if err := validateTaskIDs(consumer.Tasks); err != nil {
			problems = append(problems, fmt.Errorf("surface %q tasks: %w", consumer.Surface, err))
		}
		for _, kind := range consumer.Artifacts {
			if _, ok := artifactKinds[kind]; !ok {
				problems = append(problems, fmt.Errorf("surface %q references unknown artifact %q", consumer.Surface, kind))
			}
		}
	}
	if !slices.Equal(gotSurfaces, wantSurfaces) {
		problems = append(problems, fmt.Errorf("surface order=%v, want %v", gotSurfaces, wantSurfaces))
	}
	if len(p.DeltaTriggers) < 10 || !slices.Contains(p.DeltaTriggers, "official_rule") || !slices.Contains(p.DeltaTriggers, "e2ee") {
		problems = append(problems, errors.New("policy delta triggers are incomplete"))
	}
	return errors.Join(problems...)
}

func (p Pack) validateReview(approved map[string]map[string]any, requireProceed bool) []error {
	var problems []error
	configuration := approved["policy_review_and_configuration"]
	if p.Review.CounselReviewRequired != boolValue(configuration, "counsel_review_required") ||
		p.Review.EnglishReviewer != stringValue(configuration, "english_legal_reviewer") ||
		p.Review.RussianReviewer != stringValue(configuration, "russian_legal_reviewer") {
		problems = append(problems, errors.New("review configuration differs from approved inputs"))
	}
	if strings.TrimSpace(p.Review.Reason) == "" {
		problems = append(problems, errors.New("review reason is required"))
	}
	switch p.Review.PublicationDecision {
	case "hold":
		if p.Review.ExactContentReviewState != "pending_owner_approval" || p.Review.ApprovedBy != nil || p.Review.ApprovedAt != nil {
			problems = append(problems, errors.New("hold requires pending_owner_approval and no approval metadata"))
		}
	case "proceed":
		if p.Review.ExactContentReviewState != "approved" || p.Review.ApprovedBy == nil || *p.Review.ApprovedBy != p.Review.EnglishReviewer || p.Review.ApprovedAt == nil {
			problems = append(problems, errors.New("proceed requires exact-content approval by the configured reviewer"))
		} else if _, err := time.Parse(time.RFC3339, *p.Review.ApprovedAt); err != nil {
			problems = append(problems, fmt.Errorf("approved_at must be RFC3339: %w", err))
		}
	default:
		problems = append(problems, fmt.Errorf("invalid publication_decision %q", p.Review.PublicationDecision))
	}
	if requireProceed && p.Review.PublicationDecision != "proceed" {
		problems = append(problems, fmt.Errorf("publication_decision=%q, want proceed", p.Review.PublicationDecision))
	}
	return problems
}

func (p Pack) validateURLs(approved map[string]map[string]any) []error {
	contacts := approved["contacts_and_public_urls"]
	want := map[string]string{
		"privacy":            stringValue(contacts, "product_privacy_url"),
		"terms":              stringValue(contacts, "product_terms_url"),
		"content_guidelines": stringValue(contacts, "content_guidelines_url"),
		"support":            stringValue(contacts, "support_url"),
	}
	var problems []error
	if len(p.CanonicalURLs) != len(want) {
		problems = append(problems, fmt.Errorf("canonical URL count=%d, want %d", len(p.CanonicalURLs), len(want)))
	}
	for key, value := range want {
		if p.CanonicalURLs[key] != value {
			problems = append(problems, fmt.Errorf("canonical URL %s=%q, want %q", key, p.CanonicalURLs[key], value))
		}
	}
	return problems
}

func approvedValues(checkpoint legalops.Checkpoint, problems *[]error) map[string]map[string]any {
	values := make(map[string]map[string]any, len(checkpoint.Inputs))
	for _, input := range checkpoint.Inputs {
		var value map[string]any
		if err := json.Unmarshal(input.Value, &value); err != nil {
			*problems = append(*problems, fmt.Errorf("decode approved input %q: %w", input.ID, err))
			continue
		}
		values[input.ID] = value
	}
	return values
}

func validateDocument(repoRoot, relativePath, wantHash string) ([]byte, []string, error) {
	if filepath.IsAbs(relativePath) || strings.Contains(filepath.ToSlash(relativePath), "../") {
		return nil, nil, errors.New("document path must remain inside the repository")
	}
	content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(relativePath)))
	if err != nil {
		return nil, nil, err
	}
	digest := sha256.Sum256(content)
	gotHash := hex.EncodeToString(digest[:])
	if len(wantHash) != 64 || strings.Trim(wantHash, "0123456789abcdef") != "" || gotHash != wantHash {
		return content, nil, fmt.Errorf("sha256=%s, manifest=%s", gotHash, wantHash)
	}
	if placeholderPattern.Match(content) {
		return content, nil, errors.New("document contains placeholder language")
	}
	for _, pattern := range unsupportedClaims {
		if pattern.Match(content) {
			return content, nil, fmt.Errorf("document contains unsupported claim matching %q", pattern.String())
		}
	}
	matches := sectionPattern.FindAllSubmatch(content, -1)
	sections := make([]string, 0, len(matches))
	seen := make(map[string]struct{}, len(matches))
	for _, match := range matches {
		section := string(match[1])
		if _, duplicate := seen[section]; duplicate {
			return content, nil, fmt.Errorf("duplicate section %q", section)
		}
		seen[section] = struct{}{}
		sections = append(sections, section)
	}
	return content, sections, nil
}

func validatePublicFacts(corpus string, approved map[string]map[string]any) []error {
	contacts := approved["contacts_and_public_urls"]
	identity := approved["legal_identity_and_controller"]
	hosting := approved["hosting_and_data_locations"]
	moderation := approved["moderation_ownership_and_response"]
	markets := approved["markets_age_and_disputes"]
	normalizedCorpus := strings.Join(strings.Fields(corpus), " ")
	required := []string{
		stringValue(identity, "legal_name"), stringValue(identity, "registration_number"),
		stringValue(identity, "tax_id"), stringValue(identity, "registered_address"),
		stringValue(contacts, "privacy_email"), stringValue(contacts, "legal_email"),
		stringValue(contacts, "support_email"), stringValue(contacts, "moderation_email"),
		stringValue(moderation, "urgent_removal_email"),
		stringValue(moderation, "coverage_hours_and_timezone"),
		stringValue(moderation, "normal_report_response_target"),
		stringValue(moderation, "urgent_removal_response_target"),
		stringValue(hosting, "primary_data_country_or_region"),
		stringValue(hosting, "subprocessor_disclosure"),
		stringValue(markets, "governing_law"), stringValue(markets, "age_positioning"),
		"13", "not end-to-end encrypted", "seven days", "30 days", "90 days", "one hour",
	}
	var problems []error
	for _, value := range required {
		if value == "" || !strings.Contains(normalizedCorpus, value) {
			problems = append(problems, fmt.Errorf("public corpus does not contain approved/required fact %q", value))
		}
	}
	return problems
}

func validateSource(repoRoot string, source Source) error {
	if strings.HasPrefix(source.URL, "repo:") {
		relative := strings.SplitN(strings.TrimPrefix(source.URL, "repo:"), "#", 2)[0]
		if relative == "" || filepath.IsAbs(relative) || strings.Contains(filepath.ToSlash(relative), "../") {
			return fmt.Errorf("source %q has invalid repository path", source.ID)
		}
		if _, err := os.Stat(filepath.Join(repoRoot, filepath.FromSlash(relative))); err != nil {
			return fmt.Errorf("source %q repository path: %w", source.ID, err)
		}
		return nil
	}
	parsed, err := url.Parse(source.URL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
		return fmt.Errorf("source %q must use an absolute credential-free HTTPS URL", source.ID)
	}
	allowedHosts := []string{
		"learn.microsoft.com", "telegram.org", "www.telegram.org", "developer.spotify.com",
		"www.ftc.gov", "commission.europa.eu", "oag.ca.gov", "www.oag.ca.gov",
	}
	if !slices.Contains(allowedHosts, parsed.Hostname()) {
		return fmt.Errorf("source %q uses unsupported host %q", source.ID, parsed.Hostname())
	}
	return nil
}

func numberedSections(prefix string, count int) []string {
	sections := make([]string, count)
	for index := range sections {
		sections[index] = fmt.Sprintf("%s-%02d", prefix, index+1)
	}
	return sections
}

func validateTaskIDs(taskIDs []string) error {
	if len(taskIDs) == 0 {
		return errors.New("at least one task ID is required")
	}
	seen := make(map[string]struct{}, len(taskIDs))
	for _, taskID := range taskIDs {
		if !taskIDPattern.MatchString(taskID) {
			return fmt.Errorf("invalid task ID %q", taskID)
		}
		if _, duplicate := seen[taskID]; duplicate {
			return fmt.Errorf("duplicate task ID %q", taskID)
		}
		seen[taskID] = struct{}{}
	}
	return nil
}

func stringValue(value map[string]any, key string) string {
	result, _ := value[key].(string)
	return result
}

func boolValue(value map[string]any, key string) bool {
	result, _ := value[key].(bool)
	return result
}
