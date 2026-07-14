package policypublication

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"

	"relux.works/duet/coordinator/internal/policypack"
)

const (
	SchemaVersion          = 1
	TaskID                 = "TASK-260712-1x0lot"
	ProductionOrigin       = "https://barycenter.live"
	StableCacheControl     = "public, max-age=300, must-revalidate"
	VersionedCacheControl  = "public, max-age=31536000, immutable"
	DeploymentManifestPath = "legal/deployment-manifest.json"
)

var (
	sectionHeadingPattern = regexp.MustCompile(`^## ([PCTUS]-[0-9]{2})\. (.+)$`)
	boldPattern           = regexp.MustCompile(`\*\*([^*]+)\*\*`)
	emailPattern          = regexp.MustCompile(`[A-Za-z0-9._%+\-]+@[A-Za-z0-9.\-]+\.[A-Za-z]{2,}`)
	urlPattern            = regexp.MustCompile(`https://[^\s<]+`)
)

type File struct {
	Path string
	Data []byte
}

type DeploymentManifest struct {
	SchemaVersion         int               `json:"schema_version"`
	TaskID                string            `json:"task_id"`
	UpstreamRepository    string            `json:"upstream_repository"`
	UpstreamCommit        string            `json:"upstream_commit"`
	SourcePackSHA256      string            `json:"source_pack_sha256"`
	SourcePackVersion     string            `json:"source_pack_version"`
	EffectiveDate         string            `json:"effective_date"`
	ControllingLanguage   string            `json:"controlling_language"`
	PublicationDecision   string            `json:"publication_decision"`
	DeploymentState       string            `json:"deployment_state"`
	StableCacheControl    string            `json:"stable_cache_control"`
	VersionedCacheControl string            `json:"versioned_cache_control"`
	Routes                []DeploymentRoute `json:"routes"`
}

type DeploymentRoute struct {
	Kind                string `json:"kind"`
	Slug                string `json:"slug"`
	Locale              string `json:"locale"`
	CanonicalURL        string `json:"canonical_url"`
	StableURL           string `json:"stable_url"`
	VersionedURL        string `json:"versioned_url"`
	SourcePath          string `json:"source_path"`
	SourceSHA256        string `json:"source_sha256"`
	StableHTMLSHA256    string `json:"stable_html_sha256"`
	VersionedHTMLSHA256 string `json:"versioned_html_sha256"`
}

type artifactView struct {
	Artifact policypack.Artifact
	Slug     string
	NavEN    string
	NavRU    string
}

func Build(repoRoot, packPath, upstreamCommit string, pack policypack.Pack) ([]File, DeploymentManifest, error) {
	if strings.TrimSpace(upstreamCommit) == "" {
		return nil, DeploymentManifest{}, errors.New("upstream commit is required")
	}
	packBytes, err := os.ReadFile(packPath)
	if err != nil {
		return nil, DeploymentManifest{}, fmt.Errorf("read policy pack: %w", err)
	}
	packDigest := sha256.Sum256(packBytes)
	state := "staged"
	if pack.Review.PublicationDecision == "proceed" {
		state = "ready"
	}
	manifest := DeploymentManifest{
		SchemaVersion:         SchemaVersion,
		TaskID:                TaskID,
		UpstreamRepository:    "relux-works/barycenter",
		UpstreamCommit:        upstreamCommit,
		SourcePackSHA256:      hex.EncodeToString(packDigest[:]),
		SourcePackVersion:     pack.Version,
		EffectiveDate:         pack.EffectiveDate,
		ControllingLanguage:   pack.ControllingLanguage,
		PublicationDecision:   pack.Review.PublicationDecision,
		DeploymentState:       state,
		StableCacheControl:    StableCacheControl,
		VersionedCacheControl: VersionedCacheControl,
	}

	views, err := artifactViews(pack.Artifacts)
	if err != nil {
		return nil, DeploymentManifest{}, err
	}
	files := []File{{Path: "legal/source/policy-pack.json", Data: packBytes}}
	for _, view := range views {
		for _, locale := range []string{"en", "ru"} {
			sourcePath, sourceHash := localizedSource(view.Artifact, locale)
			source, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(sourcePath)))
			if err != nil {
				return nil, DeploymentManifest{}, fmt.Errorf("read %s source %s: %w", locale, sourcePath, err)
			}
			if got := digest(source); got != sourceHash {
				return nil, DeploymentManifest{}, fmt.Errorf("%s source sha256=%s, manifest=%s", sourcePath, got, sourceHash)
			}
			title, body, sections, err := renderMarkdown(source)
			if err != nil {
				return nil, DeploymentManifest{}, fmt.Errorf("render %s: %w", sourcePath, err)
			}
			if !slices.Equal(sections, view.Artifact.Sections) {
				return nil, DeploymentManifest{}, fmt.Errorf("%s rendered sections=%v, want %v", sourcePath, sections, view.Artifact.Sections)
			}

			stableURL, versionedURL := routeURLs(view.Slug, locale, pack.Version)
			stablePath, versionedPath := routePaths(view.Slug, locale, pack.Version)
			stableHTML := renderPage(pageInput{
				Title: title, Body: body, Locale: locale, View: view, Pack: pack,
				SourceHash: sourceHash, StableURL: stableURL, VersionedURL: versionedURL,
			})
			versionedHTML := renderPage(pageInput{
				Title: title, Body: body, Locale: locale, View: view, Pack: pack,
				SourceHash: sourceHash, StableURL: stableURL, VersionedURL: versionedURL, Versioned: true,
			})
			files = append(files,
				File{Path: "legal/source/" + sourcePath, Data: source},
				File{Path: stablePath, Data: stableHTML},
				File{Path: versionedPath, Data: versionedHTML},
			)
			manifest.Routes = append(manifest.Routes, DeploymentRoute{
				Kind: view.Artifact.Kind, Slug: view.Slug, Locale: locale,
				CanonicalURL: view.Artifact.CanonicalURL, StableURL: stableURL,
				VersionedURL: versionedURL, SourcePath: sourcePath, SourceSHA256: sourceHash,
				StableHTMLSHA256: digest(stableHTML), VersionedHTMLSHA256: digest(versionedHTML),
			})
		}
	}
	manifestBytes, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, DeploymentManifest{}, err
	}
	manifestBytes = append(manifestBytes, '\n')
	files = append(files,
		File{Path: DeploymentManifestPath, Data: manifestBytes},
		File{Path: "_headers", Data: []byte(headersFile())},
	)
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, manifest, nil
}

func Write(outputRoot string, files []File) error {
	for _, file := range files {
		path := filepath.Join(outputRoot, filepath.FromSlash(file.Path))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, file.Data, 0o644); err != nil {
			return err
		}
	}
	return nil
}

func Check(outputRoot string, files []File) error {
	var problems []error
	for _, file := range files {
		path := filepath.Join(outputRoot, filepath.FromSlash(file.Path))
		got, err := os.ReadFile(path)
		if err != nil {
			problems = append(problems, fmt.Errorf("%s: %w", file.Path, err))
			continue
		}
		if !bytes.Equal(got, file.Data) {
			problems = append(problems, fmt.Errorf("%s differs from deterministic output", file.Path))
		}
	}
	return errors.Join(problems...)
}

func LoadDeploymentManifest(content []byte) (DeploymentManifest, error) {
	decoder := json.NewDecoder(bytes.NewReader(content))
	decoder.DisallowUnknownFields()
	var manifest DeploymentManifest
	if err := decoder.Decode(&manifest); err != nil {
		return DeploymentManifest{}, err
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return DeploymentManifest{}, fmt.Errorf("trailing JSON: %v", err)
	}
	return manifest, nil
}

func VerifyLive(ctx context.Context, client *http.Client, origin string, packBytes []byte, pack policypack.Pack) error {
	parsedOrigin, err := url.Parse(origin)
	if err != nil || parsedOrigin.Scheme != "https" || parsedOrigin.Host == "" {
		return errors.New("live origin must be an absolute HTTPS URL")
	}
	guardedClient := *client
	previousRedirectCheck := client.CheckRedirect
	guardedClient.CheckRedirect = func(request *http.Request, via []*http.Request) error {
		if len(via) >= 5 {
			return errors.New("too many redirects")
		}
		if request.URL.Scheme != "https" || request.URL.Host != parsedOrigin.Host {
			return errors.New("redirect left the approved HTTPS origin")
		}
		if previousRedirectCheck != nil {
			return previousRedirectCheck(request, via)
		}
		return nil
	}
	client = &guardedClient
	manifestURL := strings.TrimRight(origin, "/") + "/" + DeploymentManifestPath
	manifestBody, _, err := fetch(ctx, client, manifestURL)
	if err != nil {
		return fmt.Errorf("deployment manifest: %w", err)
	}
	manifest, err := LoadDeploymentManifest(manifestBody)
	if err != nil {
		return fmt.Errorf("decode deployment manifest: %w", err)
	}
	var problems []error
	packDigest := sha256.Sum256(packBytes)
	if manifest.SchemaVersion != SchemaVersion || manifest.TaskID != TaskID ||
		manifest.SourcePackSHA256 != hex.EncodeToString(packDigest[:]) ||
		manifest.SourcePackVersion != pack.Version || manifest.PublicationDecision != "proceed" ||
		manifest.DeploymentState != "ready" || manifest.StableCacheControl != StableCacheControl ||
		manifest.VersionedCacheControl != VersionedCacheControl {
		problems = append(problems, errors.New("live deployment manifest does not match the approved ready source pack"))
	}
	if len(manifest.Routes) != len(pack.Artifacts)*2 {
		problems = append(problems, fmt.Errorf("live route count=%d, want %d", len(manifest.Routes), len(pack.Artifacts)*2))
	}
	for _, route := range manifest.Routes {
		for _, target := range []struct {
			name, rawURL, wantHash, wantCache string
		}{
			{"stable", route.StableURL, route.StableHTMLSHA256, StableCacheControl},
			{"versioned", route.VersionedURL, route.VersionedHTMLSHA256, VersionedCacheControl},
		} {
			targetURL, parseErr := url.Parse(target.rawURL)
			if parseErr != nil || targetURL.Scheme != "https" || targetURL.Host != parsedOrigin.Host {
				problems = append(problems, fmt.Errorf("%s %s/%s leaves the approved HTTPS origin", target.name, route.Kind, route.Locale))
				continue
			}
			body, headers, err := fetch(ctx, client, target.rawURL)
			if err != nil {
				problems = append(problems, fmt.Errorf("%s %s/%s: %w", target.name, route.Kind, route.Locale, err))
				continue
			}
			if digest(body) != target.wantHash {
				problems = append(problems, fmt.Errorf("%s %s/%s body hash mismatch", target.name, route.Kind, route.Locale))
			}
			if headers.Get("Cache-Control") != target.wantCache {
				problems = append(problems, fmt.Errorf("%s %s/%s cache-control=%q, want %q", target.name, route.Kind, route.Locale, headers.Get("Cache-Control"), target.wantCache))
			}
			if !strings.Contains(headers.Get("Content-Type"), "text/html") ||
				!bytes.Contains(body, []byte(`name="pulsar-policy-source-sha256" content="`+route.SourceSHA256+`"`)) {
				problems = append(problems, fmt.Errorf("%s %s/%s lacks HTML/source-hash evidence", target.name, route.Kind, route.Locale))
			}
		}
	}
	return errors.Join(problems...)
}

func fetch(ctx context.Context, client *http.Client, rawURL string) ([]byte, http.Header, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return nil, nil, errors.New("route is not absolute HTTPS")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, nil, err
	}
	response, err := client.Do(req)
	if err != nil {
		return nil, nil, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, response.Header, fmt.Errorf("status=%d", response.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
	if err != nil {
		return nil, response.Header, err
	}
	return body, response.Header, nil
}

func artifactViews(artifacts []policypack.Artifact) ([]artifactView, error) {
	contract := map[string]artifactView{
		"privacy":              {Slug: "privacy", NavEN: "Privacy", NavRU: "Конфиденциальность"},
		"terms":                {Slug: "terms", NavEN: "Terms", NavRU: "Условия"},
		"content_guidelines":   {Slug: "content-guidelines", NavEN: "Guidelines", NavRU: "Правила"},
		"upload_rights_notice": {Slug: "upload-rights", NavEN: "Upload rights", NavRU: "Права на загрузку"},
		"support":              {Slug: "support", NavEN: "Support", NavRU: "Поддержка"},
	}
	views := make([]artifactView, 0, len(artifacts))
	for _, artifact := range artifacts {
		view, ok := contract[artifact.Kind]
		if !ok {
			return nil, fmt.Errorf("unsupported public artifact %q", artifact.Kind)
		}
		view.Artifact = artifact
		views = append(views, view)
	}
	return views, nil
}

func localizedSource(artifact policypack.Artifact, locale string) (string, string) {
	if locale == "ru" {
		return artifact.RUPath, artifact.RUSHA256
	}
	return artifact.ENPath, artifact.ENSHA256
}

func routeURLs(slug, locale, version string) (string, string) {
	stable := ProductionOrigin + "/legal/" + slug
	if locale == "ru" {
		stable += "/ru"
	}
	versioned := ProductionOrigin + "/legal/versions/" + version + "/" + locale + "/" + slug
	return stable, versioned
}

func routePaths(slug, locale, version string) (string, string) {
	stable := "legal/" + slug
	if locale == "ru" {
		stable += "/ru"
	}
	return stable + "/index.html", "legal/versions/" + version + "/" + locale + "/" + slug + "/index.html"
}

type pageInput struct {
	Title, Body, Locale, SourceHash, StableURL, VersionedURL string
	View                                                     artifactView
	Pack                                                     policypack.Pack
	Versioned                                                bool
}

func renderPage(input pageInput) []byte {
	otherLocale := "ru"
	otherLabel := "Русский"
	if input.Locale == "ru" {
		otherLocale = "en"
		otherLabel = "English"
	}
	otherStable, otherVersioned := routeURLs(input.View.Slug, otherLocale, input.Pack.Version)
	languageTarget := otherStable
	archiveNote := ""
	robots := ""
	if input.Versioned {
		languageTarget = otherVersioned
		robots = `<meta name="robots" content="noindex">` + "\n"
		if input.Locale == "ru" {
			archiveNote = `<p class="archive">Неизменяемая архивная копия. <a href="` + input.StableURL + `">Открыть текущую версию</a>.</p>`
		} else {
			archiveNote = `<p class="archive">Immutable archived copy. <a href="` + input.StableURL + `">Open the current version</a>.</p>`
		}
	}
	homeLabel, versionLabel, sourceLabel := "Pulsar home", "Version", "Source SHA-256"
	if input.Locale == "ru" {
		homeLabel, versionLabel, sourceLabel = "Главная Pulsar", "Версия", "SHA-256 источника"
	}
	var nav strings.Builder
	views, _ := artifactViews(input.Pack.Artifacts)
	for _, view := range views {
		stable, versioned := routeURLs(view.Slug, input.Locale, input.Pack.Version)
		href := stable
		if input.Versioned {
			href = versioned
		}
		label := view.NavEN
		if input.Locale == "ru" {
			label = view.NavRU
		}
		fmt.Fprintf(&nav, `<a href="%s">%s</a>`, href, html.EscapeString(label))
	}
	englishStable, _ := routeURLs(input.View.Slug, "en", input.Pack.Version)
	russianStable, _ := routeURLs(input.View.Slug, "ru", input.Pack.Version)
	page := fmt.Sprintf(`<!doctype html>
<html lang="%s">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width,initial-scale=1">
%s<title>%s · Pulsar</title>
<link rel="canonical" href="%s">
<link rel="alternate" hreflang="en" href="%s">
<link rel="alternate" hreflang="ru" href="%s">
<link rel="alternate" hreflang="x-default" href="%s">
<meta name="pulsar-policy-version" content="%s">
<meta name="pulsar-policy-source-sha256" content="%s">
<meta name="pulsar-policy-artifact" content="%s">
<style>
:root{color-scheme:dark;--bg:#0a0926;--panel:#12103a;--text:#eceafd;--muted:#b8b2dc;--link:#b9adff;--line:#34305f;--accent:#f0b35c}*{box-sizing:border-box}html{scroll-behavior:smooth}body{margin:0;background:linear-gradient(180deg,var(--panel),var(--bg) 38rem);color:var(--text);font:17px/1.65 -apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}.wrap{max-width:920px;margin:auto;padding:28px 22px 72px}.top,.meta{display:flex;gap:14px;align-items:center;justify-content:space-between;flex-wrap:wrap}.top a,.docs a{color:var(--link)}.docs{display:flex;gap:8px 18px;flex-wrap:wrap;padding:20px 0;border-bottom:1px solid var(--line)}.docs a{font-size:14px}h1{font-size:clamp(2rem,6vw,3.4rem);line-height:1.08;margin:64px 0 20px;letter-spacing:-.035em}h2{font-size:1.45rem;line-height:1.3;margin:54px 0 14px;scroll-margin-top:20px}h2 a{color:inherit;text-decoration:none}p,li{max-width:78ch}a{color:var(--link);text-underline-offset:3px}strong{color:#fff}table{width:100%%;border-collapse:collapse;margin:24px 0;font-size:15px}th,td{padding:12px;border:1px solid var(--line);text-align:left;vertical-align:top}th{background:rgba(255,255,255,.06)}.meta,.archive{padding:14px 18px;border:1px solid var(--line);background:rgba(255,255,255,.045);border-radius:14px;color:var(--muted);font-size:14px}.archive{margin:24px 0}.hash{overflow-wrap:anywhere}footer{margin-top:64px;padding-top:20px;border-top:1px solid var(--line);color:var(--muted);font-size:13px}@media(max-width:600px){.wrap{padding-inline:16px}table{display:block;overflow-x:auto}}
</style>
</head>
<body><div class="wrap">
<header><div class="top"><a href="/">%s</a><a href="%s" hreflang="%s">%s</a></div><nav class="docs">%s</nav></header>
<main data-policy-kind="%s" data-source-sha256="%s">
<h1>%s</h1>
<div class="meta"><span>%s %s · %s</span><span class="hash">%s: <code>%s</code></span></div>
%s%s
</main>
<footer><a href="%s">%s</a> · <a href="%s">%s</a></footer>
</div></body></html>
`, input.Locale, robots, html.EscapeString(input.Title), input.View.Artifact.CanonicalURL,
		englishStable, russianStable, englishStable, input.Pack.Version, input.SourceHash,
		input.View.Artifact.Kind, homeLabel, languageTarget, otherLocale, otherLabel, nav.String(),
		input.View.Artifact.Kind, input.SourceHash, html.EscapeString(input.Title), versionLabel,
		html.EscapeString(input.Pack.Version), html.EscapeString(input.Pack.EffectiveDate), sourceLabel,
		input.SourceHash, archiveNote, input.Body, input.StableURL, html.EscapeString(input.StableURL),
		input.VersionedURL, html.EscapeString(input.VersionedURL))
	return []byte(page)
}

func renderMarkdown(source []byte) (string, string, []string, error) {
	lines := strings.Split(strings.ReplaceAll(string(source), "\r\n", "\n"), "\n")
	if len(lines) == 0 || !strings.HasPrefix(lines[0], "# ") {
		return "", "", nil, errors.New("document must start with one level-one title")
	}
	title := strings.TrimPrefix(lines[0], "# ")
	blocks := splitBlocks(lines[1:])
	var output strings.Builder
	var sections []string
	for _, block := range blocks {
		if match := sectionHeadingPattern.FindStringSubmatch(block[0]); match != nil && len(block) == 1 {
			sections = append(sections, match[1])
			fmt.Fprintf(&output, `<h2 id="%s"><a href="#%s">%s. %s</a></h2>`+"\n", match[1], match[1], match[1], inline(match[2]))
			continue
		}
		if strings.HasPrefix(block[0], "| ") {
			table, err := renderTable(block)
			if err != nil {
				return "", "", nil, err
			}
			output.WriteString(table)
			continue
		}
		if strings.HasPrefix(block[0], "- ") {
			output.WriteString(renderList(block))
			continue
		}
		fmt.Fprintf(&output, "<p>%s</p>\n", inline(strings.Join(block, " ")))
	}
	return title, output.String(), sections, nil
}

func splitBlocks(lines []string) [][]string {
	var blocks [][]string
	var current []string
	flush := func() {
		if len(current) > 0 {
			blocks = append(blocks, current)
			current = nil
		}
	}
	for _, line := range lines {
		if strings.TrimSpace(line) == "" {
			flush()
			continue
		}
		current = append(current, line)
	}
	flush()
	return blocks
}

func renderList(lines []string) string {
	var output strings.Builder
	output.WriteString("<ul>\n")
	var item []string
	flush := func() {
		if len(item) > 0 {
			fmt.Fprintf(&output, "<li>%s</li>\n", inline(strings.Join(item, " ")))
			item = nil
		}
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "- ") {
			flush()
			item = append(item, strings.TrimPrefix(line, "- "))
		} else {
			item = append(item, line)
		}
	}
	flush()
	output.WriteString("</ul>\n")
	return output.String()
}

func renderTable(lines []string) (string, error) {
	if len(lines) < 2 || !strings.Contains(lines[1], "---") {
		return "", errors.New("table is missing a separator row")
	}
	rows := make([][]string, 0, len(lines)-1)
	for index, line := range lines {
		if index == 1 {
			continue
		}
		trimmed := strings.Trim(strings.TrimSpace(line), "|")
		parts := strings.Split(trimmed, "|")
		for i := range parts {
			parts[i] = strings.TrimSpace(parts[i])
		}
		rows = append(rows, parts)
	}
	columns := len(rows[0])
	for _, row := range rows {
		if len(row) != columns {
			return "", errors.New("table rows have different column counts")
		}
	}
	var output strings.Builder
	output.WriteString("<table><thead><tr>")
	for _, cell := range rows[0] {
		fmt.Fprintf(&output, "<th>%s</th>", inline(cell))
	}
	output.WriteString("</tr></thead><tbody>\n")
	for _, row := range rows[1:] {
		output.WriteString("<tr>")
		for _, cell := range row {
			fmt.Fprintf(&output, "<td>%s</td>", inline(cell))
		}
		output.WriteString("</tr>\n")
	}
	output.WriteString("</tbody></table>\n")
	return output.String(), nil
}

func inline(value string) string {
	escaped := html.EscapeString(value)
	escaped = boldPattern.ReplaceAllString(escaped, `<strong>$1</strong>`)
	escaped = emailPattern.ReplaceAllStringFunc(escaped, func(match string) string {
		return `<a href="mailto:` + match + `">` + match + `</a>`
	})
	escaped = urlPattern.ReplaceAllStringFunc(escaped, func(match string) string {
		trimmed := strings.TrimRight(match, ".,;:)")
		suffix := strings.TrimPrefix(match, trimmed)
		return `<a href="` + trimmed + `">` + trimmed + `</a>` + suffix
	})
	return escaped
}

func headersFile() string {
	return `/legal/privacy
  Cache-Control: ` + StableCacheControl + `
  X-Content-Type-Options: nosniff
  Referrer-Policy: strict-origin-when-cross-origin

/legal/privacy/*
  Cache-Control: ` + StableCacheControl + `
  X-Content-Type-Options: nosniff
  Referrer-Policy: strict-origin-when-cross-origin

/legal/terms
  Cache-Control: ` + StableCacheControl + `
  X-Content-Type-Options: nosniff
  Referrer-Policy: strict-origin-when-cross-origin

/legal/terms/*
  Cache-Control: ` + StableCacheControl + `
  X-Content-Type-Options: nosniff
  Referrer-Policy: strict-origin-when-cross-origin

/legal/content-guidelines
  Cache-Control: ` + StableCacheControl + `
  X-Content-Type-Options: nosniff
  Referrer-Policy: strict-origin-when-cross-origin

/legal/content-guidelines/*
  Cache-Control: ` + StableCacheControl + `
  X-Content-Type-Options: nosniff
  Referrer-Policy: strict-origin-when-cross-origin

/legal/upload-rights
  Cache-Control: ` + StableCacheControl + `
  X-Content-Type-Options: nosniff
  Referrer-Policy: strict-origin-when-cross-origin

/legal/upload-rights/*
  Cache-Control: ` + StableCacheControl + `
  X-Content-Type-Options: nosniff
  Referrer-Policy: strict-origin-when-cross-origin

/legal/support
  Cache-Control: ` + StableCacheControl + `
  X-Content-Type-Options: nosniff
  Referrer-Policy: strict-origin-when-cross-origin

/legal/support/*
  Cache-Control: ` + StableCacheControl + `
  X-Content-Type-Options: nosniff
  Referrer-Policy: strict-origin-when-cross-origin

/legal/versions/*
  Cache-Control: ` + VersionedCacheControl + `
  X-Content-Type-Options: nosniff
  Referrer-Policy: strict-origin-when-cross-origin

/legal/deployment-manifest.json
  Cache-Control: no-cache, no-store, must-revalidate
  X-Content-Type-Options: nosniff
`
}

func digest(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

func DefaultHTTPClient() *http.Client {
	return &http.Client{Timeout: 20 * time.Second}
}
