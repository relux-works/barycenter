package policypublication

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"relux.works/duet/coordinator/internal/legalops"
	"relux.works/duet/coordinator/internal/policypack"
)

func repositoryBundle(t *testing.T) (string, []File, DeploymentManifest) {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	packPath := filepath.Join(repoRoot, "docs", "compliance", "policy-pack-2026-07-14.json")
	pack, err := policypack.Load(packPath)
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := legalops.Load(filepath.Join(repoRoot, "docs", "compliance", "legal-ops-inputs.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := pack.Validate(repoRoot, inputs, false); err != nil {
		t.Fatal(err)
	}
	files, manifest, err := Build(repoRoot, packPath, "0123456789abcdef", pack)
	if err != nil {
		t.Fatal(err)
	}
	return repoRoot, files, manifest
}

func TestRepositorySurfacesUseTheStableLocaleContract(t *testing.T) {
	repoRoot, _, _ := repositoryBundle(t)
	russianURLs := []string{
		"https://barycenter.live/legal/privacy/ru",
		"https://barycenter.live/legal/terms/ru",
		"https://barycenter.live/legal/content-guidelines/ru",
		"https://barycenter.live/legal/upload-rights/ru",
		"https://barycenter.live/legal/support/ru",
	}
	for _, relative := range []string{
		"node-app/Sources/NodeCore/PublicPolicyLinks.swift",
		"pulsar-win/ui_common.go",
		"coordinator/internal/bot/commands.go",
	} {
		content, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(relative)))
		if err != nil {
			t.Fatal(err)
		}
		for _, publicURL := range russianURLs {
			if !bytes.Contains(content, []byte(publicURL)) {
				t.Fatalf("%s lacks %s", relative, publicURL)
			}
		}
	}
	storeBytes, err := os.ReadFile(filepath.Join(repoRoot, "docs", "compliance", "store-public-links.json"))
	if err != nil {
		t.Fatal(err)
	}
	var store map[string]any
	if err := json.Unmarshal(storeBytes, &store); err != nil {
		t.Fatal(err)
	}
	for _, publicURL := range russianURLs[:3] {
		if !bytes.Contains(storeBytes, []byte(publicURL)) {
			t.Fatalf("Store metadata lacks %s", publicURL)
		}
	}
	for _, publicURL := range []string{
		"https://barycenter.live/legal/privacy",
		"https://barycenter.live/legal/terms",
		"https://barycenter.live/legal/content-guidelines",
		"https://barycenter.live/legal/support",
	} {
		if !bytes.Contains(storeBytes, []byte(publicURL)) {
			t.Fatalf("Store metadata lacks %s", publicURL)
		}
	}
}

func TestRepositoryBundleIsDeterministicAndHashBound(t *testing.T) {
	_, files, manifest := repositoryBundle(t)
	if len(files) != 33 {
		t.Fatalf("file count=%d, want 33", len(files))
	}
	if manifest.PublicationDecision != "hold" || manifest.DeploymentState != "staged" || len(manifest.Routes) != 10 {
		t.Fatalf("manifest=%+v", manifest)
	}
	for _, route := range manifest.Routes {
		if len(route.SourceSHA256) != 64 || len(route.StableHTMLSHA256) != 64 || len(route.VersionedHTMLSHA256) != 64 {
			t.Fatalf("route lacks hashes: %+v", route)
		}
	}
	var privacy []byte
	for _, file := range files {
		if file.Path == "legal/privacy/index.html" {
			privacy = file.Data
			break
		}
	}
	for _, marker := range []string{`id="P-01"`, `hreflang="ru"`, `pulsar-policy-source-sha256`, `not end-to-end encrypted`, `mailto:support@barycenter.live`} {
		if !bytes.Contains(privacy, []byte(marker)) {
			t.Fatalf("privacy output lacks %q", marker)
		}
	}
}

func TestWriteCheckAndTamperFailure(t *testing.T) {
	_, files, _ := repositoryBundle(t)
	root := t.TempDir()
	if err := Write(root, files); err != nil {
		t.Fatal(err)
	}
	if err := Check(root, files); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "legal", "privacy", "index.html")
	if err := os.WriteFile(path, []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := Check(root, files); err == nil || !strings.Contains(err.Error(), "differs") {
		t.Fatalf("tamper error=%v", err)
	}
}

func TestMarkdownRendererEscapesAndPinsSections(t *testing.T) {
	title, body, sections, err := renderMarkdown([]byte("# Test\n\n## S-01. Contact <now>\n\nUse **care** and support@example.com.\n"))
	if err != nil {
		t.Fatal(err)
	}
	if title != "Test" || !slicesEqual(sections, []string{"S-01"}) || !strings.Contains(body, "Contact &lt;now&gt;") || !strings.Contains(body, "mailto:support@example.com") {
		t.Fatalf("title=%q sections=%v body=%s", title, sections, body)
	}
}

func slicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
