package storelisting

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	_ "image/png"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

const (
	productID       = "9P26FDCWV1GC"
	packageIdentity = "ReluxWorksLLC.PulsarBarycenter"
	maxImageBytes   = 50 << 20
)

var (
	commitPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)
	digestPattern = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

type Package struct {
	SchemaVersion      int               `json:"schema_version"`
	TaskID             string            `json:"task_id"`
	State              string            `json:"state"`
	Product            Product           `json:"product"`
	ApprovedLinksPath  string            `json:"approved_links_path"`
	ListingFiles       map[string]string `json:"listing_files"`
	ScreenshotManifest string            `json:"screenshot_manifest"`
	IARCProfile        string            `json:"iarc_profile"`
	CertificationNotes string            `json:"certification_notes"`
	WACK               WACKGate          `json:"wack"`
	Submission         SubmissionGate    `json:"submission"`
	Sources            []Source          `json:"sources"`
}

type Product struct {
	ID                string `json:"id"`
	Name              string `json:"name"`
	PackageIdentity   string `json:"package_identity"`
	Category          string `json:"category"`
	Price             string `json:"price"`
	MinimumAge        int    `json:"minimum_age"`
	CoordinatorOrigin string `json:"coordinator_origin"`
}

type WACKGate struct {
	State        string `json:"state"`
	Runner       string `json:"runner"`
	ManifestPath string `json:"manifest_path"`
}

type SubmissionGate struct {
	State     string `json:"state"`
	Authority string `json:"authority"`
}

type Source struct {
	ID          string `json:"id"`
	URL         string `json:"url"`
	RetrievedAt string `json:"retrieved_at"`
}

type Listing struct {
	SchemaVersion        int      `json:"schema_version"`
	Locale               string   `json:"locale"`
	ProductName          string   `json:"product_name"`
	Category             string   `json:"category"`
	Price                string   `json:"price"`
	Description          string   `json:"description"`
	ShortDescription     string   `json:"short_description"`
	WhatsNew             string   `json:"whats_new"`
	Features             []string `json:"features"`
	Keywords             []string `json:"keywords"`
	PrivacyURL           string   `json:"privacy_url"`
	SupportURL           string   `json:"support_url"`
	TermsURL             string   `json:"terms_url"`
	ContentGuidelinesURL string   `json:"content_guidelines_url"`
	OptionalIntegrations []string `json:"optional_integrations"`
	Limitations          []string `json:"limitations"`
	ClaimEvidence        []string `json:"claim_evidence"`
}

type ScreenshotManifest struct {
	SchemaVersion int               `json:"schema_version"`
	State         string            `json:"state"`
	Rules         ScreenshotRules   `json:"rules"`
	Screenshots   []ScreenshotEntry `json:"screenshots"`
}

type ScreenshotRules struct {
	Format           string `json:"format"`
	MinimumWidth     int    `json:"minimum_width"`
	MinimumHeight    int    `json:"minimum_height"`
	MaximumBytes     int64  `json:"maximum_bytes"`
	MaximumPerLocale int    `json:"maximum_per_locale"`
}

type ScreenshotEntry struct {
	Locale  string `json:"locale"`
	Order   int    `json:"order"`
	Scene   string `json:"scene"`
	Path    string `json:"path"`
	Caption string `json:"caption"`
	Status  string `json:"status"`
	SHA256  string `json:"sha256"`
}

type IARCProfile struct {
	SchemaVersion int               `json:"schema_version"`
	State         string            `json:"state"`
	MinimumAge    int               `json:"minimum_age"`
	AudienceNote  string            `json:"audience_note"`
	Facts         []IARCFact        `json:"facts"`
	Questionnaire IARCQuestionnaire `json:"questionnaire"`
}

type IARCFact struct {
	ID        string   `json:"id"`
	Answer    string   `json:"answer"`
	Rationale string   `json:"rationale"`
	Evidence  []string `json:"evidence"`
}

type IARCQuestionnaire struct {
	State         string `json:"state"`
	AnswersExport string `json:"answers_export"`
	ExportSHA256  string `json:"export_sha256"`
	RatingID      string `json:"rating_id"`
	GeneratedAt   string `json:"generated_at"`
	Reviewer      string `json:"reviewer"`
}

type CertificationNotes struct {
	SchemaVersion     int                `json:"schema_version"`
	State             string             `json:"state"`
	ProductID         string             `json:"product_id"`
	PackageIdentity   string             `json:"package_identity"`
	CoordinatorOrigin string             `json:"coordinator_origin"`
	Build             CertificationBuild `json:"build"`
	AvailabilityOwner string             `json:"availability_owner"`
	Notes             string             `json:"notes"`
	FindingResponses  map[string]string  `json:"finding_responses"`
}

type CertificationBuild struct {
	GitCommit     string `json:"git_commit"`
	PackageSHA256 string `json:"package_sha256"`
	Version       string `json:"version"`
}

type publicLinks struct {
	SchemaVersion       int         `json:"schema_version"`
	TaskID              string      `json:"task_id"`
	ControllingLanguage string      `json:"controlling_language"`
	FallbackBehavior    string      `json:"fallback_behavior"`
	English             localeLinks `json:"english"`
	Russian             localeLinks `json:"russian"`
	PublicationGate     string      `json:"publication_gate"`
}

type localeLinks struct {
	Privacy           string `json:"privacy"`
	Terms             string `json:"terms"`
	ContentGuidelines string `json:"content_guidelines"`
	Support           string `json:"support"`
}

func Load(path string) (Package, error) {
	var pack Package
	if err := decodeJSON(path, &pack); err != nil {
		return Package{}, err
	}
	return pack, nil
}

func (p Package) Validate(repoRoot string, requireReady bool) error {
	if p.SchemaVersion != 1 || p.TaskID != "TASK-260712-2s4e9p" {
		return fmt.Errorf("invalid package identity")
	}
	if p.Product.ID != productID || p.Product.Name != "Pulsar" ||
		p.Product.PackageIdentity != packageIdentity || p.Product.Category == "" ||
		p.Product.Price == "" || p.Product.MinimumAge != 13 {
		return fmt.Errorf("invalid frozen product metadata")
	}
	if err := secureHTTPS(p.Product.CoordinatorOrigin); err != nil {
		return fmt.Errorf("coordinator origin: %w", err)
	}
	if p.State != "engineering-ready-manual-hold" && p.State != "submission-ready" {
		return fmt.Errorf("invalid package state %q", p.State)
	}
	if len(p.ListingFiles) != 2 {
		return fmt.Errorf("exactly en-US and ru-RU listings are required")
	}

	var links publicLinks
	if err := decodeRepoJSON(repoRoot, p.ApprovedLinksPath, &links); err != nil {
		return err
	}
	expectedLinks := map[string]localeLinks{"en-US": links.English, "ru-RU": links.Russian}
	listings := map[string]Listing{}
	for _, locale := range []string{"en-US", "ru-RU"} {
		path, ok := p.ListingFiles[locale]
		if !ok {
			return fmt.Errorf("missing %s listing", locale)
		}
		var listing Listing
		if err := decodeRepoJSON(repoRoot, path, &listing); err != nil {
			return err
		}
		if err := validateListing(listing, locale, p.Product, expectedLinks[locale], repoRoot); err != nil {
			return err
		}
		listings[locale] = listing
	}
	if len(listings["en-US"].Features) != len(listings["ru-RU"].Features) ||
		len(listings["en-US"].Limitations) != len(listings["ru-RU"].Limitations) {
		return fmt.Errorf("EN/RU listing structures are not semantically aligned")
	}

	var screenshots ScreenshotManifest
	if err := decodeRepoJSON(repoRoot, p.ScreenshotManifest, &screenshots); err != nil {
		return err
	}
	if err := screenshots.Validate(repoRoot, requireReady); err != nil {
		return err
	}
	var iarc IARCProfile
	if err := decodeRepoJSON(repoRoot, p.IARCProfile, &iarc); err != nil {
		return err
	}
	if err := iarc.Validate(repoRoot, requireReady); err != nil {
		return err
	}
	var notes CertificationNotes
	if err := decodeRepoJSON(repoRoot, p.CertificationNotes, &notes); err != nil {
		return err
	}
	if err := notes.Validate(p.Product, requireReady); err != nil {
		return err
	}
	if err := validateWACK(repoRoot, p.WACK, requireReady); err != nil {
		return err
	}
	if p.Submission.Authority != "Ivan Oparin" {
		return fmt.Errorf("unexpected submission authority")
	}
	if p.Submission.State != "hold" && p.Submission.State != "proceed" {
		return fmt.Errorf("invalid submission state %q", p.Submission.State)
	}
	if !requireReady && (p.State == "submission-ready" || p.Submission.State == "proceed") {
		return fmt.Errorf("submission-ready packages must be checked with --require-ready")
	}
	if requireReady && (p.State != "submission-ready" || p.Submission.State != "proceed") {
		return fmt.Errorf("submission remains on hold")
	}
	if len(p.Sources) < 5 {
		return fmt.Errorf("official source registry is incomplete")
	}
	seenSources := map[string]bool{}
	for _, source := range p.Sources {
		if source.ID == "" || seenSources[source.ID] || source.RetrievedAt == "" || secureHTTPS(source.URL) != nil {
			return fmt.Errorf("invalid official source record %q", source.ID)
		}
		seenSources[source.ID] = true
	}
	return nil
}

func validateListing(listing Listing, locale string, product Product, links localeLinks, repoRoot string) error {
	if listing.SchemaVersion != 1 || listing.Locale != locale || listing.ProductName != product.Name ||
		listing.Category != product.Category || listing.Price != product.Price {
		return fmt.Errorf("%s listing metadata does not match package", locale)
	}
	if strings.TrimSpace(listing.Description) == "" || len([]rune(listing.Description)) > 10000 {
		return fmt.Errorf("%s description is empty or exceeds 10000 characters", locale)
	}
	if len([]rune(listing.ShortDescription)) == 0 || len([]rune(listing.ShortDescription)) > 270 {
		return fmt.Errorf("%s short description must fit the recommended 270 visible characters", locale)
	}
	if len([]rune(listing.WhatsNew)) > 1500 {
		return fmt.Errorf("%s whats_new exceeds 1500 characters", locale)
	}
	if len(listing.Features) == 0 || len(listing.Features) > 20 {
		return fmt.Errorf("%s features must contain 1..20 entries", locale)
	}
	for _, feature := range listing.Features {
		if strings.TrimSpace(feature) == "" || len([]rune(feature)) > 200 || strings.HasPrefix(strings.TrimSpace(feature), "-") {
			return fmt.Errorf("%s has invalid feature text", locale)
		}
	}
	if len(listing.Keywords) == 0 || len(listing.Keywords) > 7 {
		return fmt.Errorf("%s keywords must contain 1..7 entries", locale)
	}
	wordCount := 0
	for _, keyword := range listing.Keywords {
		if strings.TrimSpace(keyword) == "" || len([]rune(keyword)) > 40 {
			return fmt.Errorf("%s has invalid keyword", locale)
		}
		wordCount += len(strings.Fields(keyword))
	}
	if wordCount > 21 {
		return fmt.Errorf("%s keywords exceed 21 words", locale)
	}
	if listing.PrivacyURL != links.Privacy || listing.SupportURL != links.Support ||
		listing.TermsURL != links.Terms || listing.ContentGuidelinesURL != links.ContentGuidelines {
		return fmt.Errorf("%s public links differ from the approved locale map", locale)
	}
	if len(listing.OptionalIntegrations) != 2 ||
		!containsFold(listing.OptionalIntegrations, "Spotify") || !containsFold(listing.OptionalIntegrations, "Telegram") {
		return fmt.Errorf("%s must label Spotify and Telegram as optional integrations", locale)
	}
	requiredCopy := map[string][]string{
		"en-US": {"Spotify and Telegram integrations are optional", "not end-to-end encrypted"},
		"ru-RU": {"Интеграции Spotify и Telegram необязательны", "сквозного шифрования нет"},
	}
	for _, phrase := range requiredCopy[locale] {
		if !strings.Contains(listing.Description, phrase) {
			return fmt.Errorf("%s description omits required limitation %q", locale, phrase)
		}
	}
	if strings.Contains(strings.ToLower(listing.Description), "spotify premium") {
		return fmt.Errorf("%s resurrects the retired Spotify-first prerequisite", locale)
	}
	if len(listing.Limitations) < 3 || len(listing.ClaimEvidence) == 0 {
		return fmt.Errorf("%s limitations or claim evidence are incomplete", locale)
	}
	for _, path := range listing.ClaimEvidence {
		resolved, err := repoPath(repoRoot, path)
		if err != nil {
			return err
		}
		if info, err := os.Stat(resolved); err != nil || info.IsDir() {
			return fmt.Errorf("%s claim evidence missing: %s", locale, path)
		}
	}
	return nil
}

func (m ScreenshotManifest) Validate(repoRoot string, requireReady bool) error {
	if m.SchemaVersion != 1 || m.Rules.Format != "png" || m.Rules.MinimumWidth != 1366 ||
		m.Rules.MinimumHeight != 768 || m.Rules.MaximumBytes != maxImageBytes || m.Rules.MaximumPerLocale != 10 {
		return fmt.Errorf("screenshot rules do not match the frozen desktop limits")
	}
	if m.State != "manual-required" && m.State != "captured-reviewed" {
		return fmt.Errorf("invalid screenshot manifest state %q", m.State)
	}
	requiredScenes := map[string]bool{"main-window": false, "try-locally": false, "active-recording": false,
		"routing": false, "played-history": false, "settings-moderation": false}
	counts := map[string]int{}
	seen := map[string]bool{}
	seenScenes := map[string]bool{}
	seenPaths := map[string]bool{}
	readyScreenshots := requireReady || m.State == "captured-reviewed"
	for _, shot := range m.Screenshots {
		if shot.Locale != "en-US" && shot.Locale != "ru-RU" {
			return fmt.Errorf("invalid screenshot locale %q", shot.Locale)
		}
		if _, ok := requiredScenes[shot.Scene]; !ok {
			return fmt.Errorf("invalid screenshot scene %q", shot.Scene)
		}
		key := fmt.Sprintf("%s:%d", shot.Locale, shot.Order)
		sceneKey := shot.Locale + ":" + shot.Scene
		if seen[key] || seenScenes[sceneKey] || seenPaths[shot.Path] || shot.Order < 1 || shot.Order > 6 ||
			len([]rune(shot.Caption)) == 0 || len([]rune(shot.Caption)) > 200 {
			return fmt.Errorf("invalid screenshot slot %s", key)
		}
		seen[key] = true
		seenScenes[sceneKey] = true
		seenPaths[shot.Path] = true
		counts[shot.Locale]++
		resolved, err := repoPath(repoRoot, shot.Path)
		if err != nil {
			return err
		}
		info, statErr := os.Stat(resolved)
		if statErr != nil {
			if !os.IsNotExist(statErr) || shot.Status != "manual-required" || readyScreenshots {
				return fmt.Errorf("screenshot missing: %s", shot.Path)
			}
			continue
		}
		if info.IsDir() || info.Size() > m.Rules.MaximumBytes {
			return fmt.Errorf("invalid screenshot file: %s", shot.Path)
		}
		file, err := os.Open(resolved)
		if err != nil {
			return err
		}
		config, format, decodeErr := image.DecodeConfig(file)
		file.Close()
		if decodeErr != nil || format != "png" || config.Width < m.Rules.MinimumWidth || config.Height < m.Rules.MinimumHeight {
			return fmt.Errorf("screenshot dimensions/format invalid: %s", shot.Path)
		}
		actual, err := fileSHA256(resolved)
		if err != nil || !digestPattern.MatchString(shot.SHA256) || shot.SHA256 != actual {
			return fmt.Errorf("screenshot digest invalid: %s", shot.Path)
		}
		if shot.Status != "captured" {
			return fmt.Errorf("present screenshot is not marked captured: %s", shot.Path)
		}
	}
	for _, locale := range []string{"en-US", "ru-RU"} {
		if counts[locale] != 6 {
			return fmt.Errorf("%s requires exactly six corrective screenshots", locale)
		}
	}
	if requireReady && m.State != "captured-reviewed" {
		return fmt.Errorf("screenshot set remains manual-required")
	}
	return nil
}

func (p IARCProfile) Validate(repoRoot string, requireReady bool) error {
	if p.SchemaVersion != 1 || p.MinimumAge != 13 || strings.TrimSpace(p.AudienceNote) == "" || len(p.Facts) < 10 {
		return fmt.Errorf("IARC truth profile is incomplete")
	}
	if p.State != "answer-source-ready-questionnaire-manual" && p.State != "generated-reviewed" {
		return fmt.Errorf("invalid IARC state %q", p.State)
	}
	seen := map[string]bool{}
	for _, fact := range p.Facts {
		if fact.ID == "" || seen[fact.ID] || fact.Answer == "" || fact.Rationale == "" || len(fact.Evidence) == 0 {
			return fmt.Errorf("invalid IARC fact %q", fact.ID)
		}
		seen[fact.ID] = true
		for _, path := range fact.Evidence {
			resolved, err := repoPath(repoRoot, path)
			if err != nil {
				return err
			}
			if info, err := os.Stat(resolved); err != nil || info.IsDir() {
				return fmt.Errorf("IARC evidence missing: %s", path)
			}
		}
	}
	q := p.Questionnaire
	if p.State == "answer-source-ready-questionnaire-manual" {
		if q.State != "manual-required-in-partner-center" || q.AnswersExport != "" || q.ExportSHA256 != "" ||
			q.RatingID != "" || q.GeneratedAt != "" || q.Reviewer != "" {
			return fmt.Errorf("manual IARC questionnaire contains partial or invented result data")
		}
	}
	if p.State == "generated-reviewed" || requireReady {
		if q.State != "generated-reviewed" || q.RatingID == "" || q.Reviewer != "Ivan Oparin" ||
			q.GeneratedAt == "" || !digestPattern.MatchString(q.ExportSHA256) {
			return fmt.Errorf("IARC questionnaire/rating remains manual-required")
		}
		resolved, err := repoPath(repoRoot, q.AnswersExport)
		if err != nil {
			return err
		}
		actual, err := fileSHA256(resolved)
		if err != nil || actual != q.ExportSHA256 {
			return fmt.Errorf("IARC answer export is missing or does not match its digest")
		}
	}
	return nil
}

func (n CertificationNotes) Validate(product Product, requireReady bool) error {
	if n.SchemaVersion != 1 || n.ProductID != product.ID || n.PackageIdentity != product.PackageIdentity ||
		n.CoordinatorOrigin != product.CoordinatorOrigin || n.AvailabilityOwner != "Ivan Oparin" {
		return fmt.Errorf("certification note identity/owner mismatch")
	}
	if len([]rune(n.Notes)) == 0 || len([]rune(n.Notes)) > 2000 {
		return fmt.Errorf("certification notes must fit 2000 characters")
	}
	for _, required := range []string{"9P26FDCWV1GC", "Try locally", "Create a Barycenter", "This Pulsar", "Spotify", "Telegram", "demo credentials", "10.3.1", "10.1.1.3"} {
		if !strings.Contains(n.Notes, required) {
			return fmt.Errorf("certification notes omit %q", required)
		}
	}
	if n.FindingResponses["10.3.1"] == "" || n.FindingResponses["10.1.1.3"] == "" {
		return fmt.Errorf("certification finding responses are incomplete")
	}
	if n.State != "requires-exact-build-freeze" && n.State != "frozen" {
		return fmt.Errorf("invalid certification note state %q", n.State)
	}
	if n.State == "requires-exact-build-freeze" && (n.Build.GitCommit != "" || n.Build.PackageSHA256 != "" ||
		n.Build.Version != "" || !strings.Contains(n.Notes, "{{")) {
		return fmt.Errorf("unfrozen certification notes contain partial build data")
	}
	if n.State == "frozen" || requireReady {
		if n.State != "frozen" || !commitPattern.MatchString(n.Build.GitCommit) ||
			!digestPattern.MatchString(n.Build.PackageSHA256) || n.Build.Version == "" || strings.Contains(n.Notes, "{{") {
			return fmt.Errorf("certification notes are not frozen to an exact build")
		}
	}
	return nil
}

func validateWACK(repoRoot string, gate WACKGate, requireReady bool) error {
	runner, err := repoPath(repoRoot, gate.Runner)
	if err != nil {
		return err
	}
	if info, err := os.Stat(runner); err != nil || info.IsDir() {
		return fmt.Errorf("WACK runner missing")
	}
	if gate.State != "manual-required" && gate.State != "completed-reviewed" {
		return fmt.Errorf("invalid WACK state")
	}
	if requireReady && gate.State != "completed-reviewed" {
		return fmt.Errorf("WACK evidence remains manual-required")
	}
	if gate.State == "manual-required" {
		if gate.ManifestPath != "" {
			return fmt.Errorf("manual WACK gate contains partial manifest data")
		}
		return nil
	}
	if gate.ManifestPath == "" {
		return fmt.Errorf("completed WACK gate has no manifest")
	}
	manifest, err := repoPath(repoRoot, gate.ManifestPath)
	if err != nil {
		return err
	}
	if info, err := os.Stat(manifest); err != nil || info.IsDir() {
		return fmt.Errorf("WACK manifest missing")
	}
	return nil
}

func decodeRepoJSON(repoRoot, relative string, target any) error {
	path, err := repoPath(repoRoot, relative)
	if err != nil {
		return err
	}
	return decodeJSON(path, target)
}

func decodeJSON(path string, target any) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()
	decoder := json.NewDecoder(file)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("decode %s: %w", path, err)
	}
	if err := decoder.Decode(&struct{}{}); err != io.EOF {
		return fmt.Errorf("decode %s: trailing JSON", path)
	}
	return nil
}

func repoPath(repoRoot, relative string) (string, error) {
	if relative == "" || filepath.IsAbs(relative) {
		return "", fmt.Errorf("invalid repository-relative path %q", relative)
	}
	clean := filepath.Clean(relative)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes repository: %q", relative)
	}
	root, err := filepath.Abs(repoRoot)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.Abs(filepath.Join(root, clean))
	if err != nil {
		return "", err
	}
	if resolved != root && !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
		return "", fmt.Errorf("path escapes repository: %q", relative)
	}
	return resolved, nil
}

func secureHTTPS(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("not a canonical HTTPS URL")
	}
	return nil
}

func containsFold(values []string, wanted string) bool {
	for _, value := range values {
		if strings.EqualFold(value, wanted) {
			return true
		}
	}
	return false
}

func fileSHA256(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	hash := sha256.New()
	if _, err := io.Copy(hash, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}
