package storelisting

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckedInPackagePassesEngineeringShapeAndFailsReadyClosed(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")
	pack, err := Load(filepath.Join(repoRoot, "docs", "store", "phase1", "partner-center-package.json"))
	if err != nil {
		t.Fatal(err)
	}
	if err := pack.Validate(repoRoot, false); err != nil {
		t.Fatalf("engineering package invalid: %v", err)
	}
	if err := pack.Validate(repoRoot, true); err == nil || !strings.Contains(err.Error(), "screenshot missing") {
		t.Fatalf("ready gate did not fail on absent real screenshots: %v", err)
	}
}

func TestListingLimitsAndApprovedLinkBinding(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")
	var listing Listing
	if err := decodeRepoJSON(repoRoot, "docs/store/phase1/listing-en-US.json", &listing); err != nil {
		t.Fatal(err)
	}
	var links publicLinks
	if err := decodeRepoJSON(repoRoot, "docs/compliance/store-public-links.json", &links); err != nil {
		t.Fatal(err)
	}
	product := Product{Name: "Pulsar", Category: "Music", Price: "Free"}
	tests := []struct {
		name   string
		mutate func(*Listing)
		want   string
	}{
		{"description limit", func(v *Listing) { v.Description = strings.Repeat("x", 10001) }, "10000"},
		{"feature limit", func(v *Listing) { v.Features[0] = strings.Repeat("x", 201) }, "feature"},
		{"keyword count", func(v *Listing) { v.Keywords = append(v.Keywords, "eighth") }, "keywords"},
		{"locale link", func(v *Listing) { v.PrivacyURL = "https://attacker.example/privacy" }, "approved locale map"},
		{"optional integration", func(v *Listing) { v.OptionalIntegrations = []string{"Spotify"} }, "optional integrations"},
		{"evidence traversal", func(v *Listing) { v.ClaimEvidence = []string{"../secret"} }, "escapes repository"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := listing
			candidate.Features = append([]string(nil), listing.Features...)
			candidate.Keywords = append([]string(nil), listing.Keywords...)
			candidate.OptionalIntegrations = append([]string(nil), listing.OptionalIntegrations...)
			candidate.ClaimEvidence = append([]string(nil), listing.ClaimEvidence...)
			test.mutate(&candidate)
			err := validateListing(candidate, "en-US", product, links.English, repoRoot)
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("error=%v want substring %q", err, test.want)
			}
		})
	}
}

func TestScreenshotManifestRejectsDuplicateSceneAndPathTraversal(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")
	var manifest ScreenshotManifest
	if err := decodeRepoJSON(repoRoot, "docs/store/phase1/screenshots.json", &manifest); err != nil {
		t.Fatal(err)
	}
	duplicate := manifest
	duplicate.Screenshots = append([]ScreenshotEntry(nil), manifest.Screenshots...)
	duplicate.Screenshots[1].Scene = duplicate.Screenshots[0].Scene
	if err := duplicate.Validate(repoRoot, false); err == nil || !strings.Contains(err.Error(), "invalid screenshot slot") {
		t.Fatalf("duplicate scene accepted: %v", err)
	}

	traversal := manifest
	traversal.Screenshots = append([]ScreenshotEntry(nil), manifest.Screenshots...)
	traversal.Screenshots[0].Path = "../outside.png"
	if err := traversal.Validate(repoRoot, false); err == nil || !strings.Contains(err.Error(), "escapes repository") {
		t.Fatalf("path traversal accepted: %v", err)
	}
}

func TestScreenshotManifestValidatesRealPNGDimensionsAndDigests(t *testing.T) {
	root := t.TempDir()
	makePNG := func(path string, width, height int) string {
		t.Helper()
		var encoded bytes.Buffer
		if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, width, height))); err != nil {
			t.Fatal(err)
		}
		absolute := filepath.Join(root, path)
		if err := os.MkdirAll(filepath.Dir(absolute), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(absolute, encoded.Bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		digest, err := fileSHA256(absolute)
		if err != nil {
			t.Fatal(err)
		}
		return digest
	}
	scenes := []string{"main-window", "try-locally", "active-recording", "routing", "played-history", "settings-moderation"}
	manifest := ScreenshotManifest{
		SchemaVersion: 1, State: "captured-reviewed",
		Rules: ScreenshotRules{Format: "png", MinimumWidth: 1366, MinimumHeight: 768, MaximumBytes: maxImageBytes, MaximumPerLocale: 10},
	}
	for _, locale := range []string{"en-US", "ru-RU"} {
		for index, scene := range scenes {
			path := filepath.Join("shots", locale, fmt.Sprintf("%02d.png", index+1))
			manifest.Screenshots = append(manifest.Screenshots, ScreenshotEntry{
				Locale: locale, Order: index + 1, Scene: scene, Path: path,
				Caption: "real exact-build scene", Status: "captured", SHA256: makePNG(path, 1366, 768),
			})
		}
	}
	if err := manifest.Validate(root, true); err != nil {
		t.Fatalf("valid real screenshot set rejected: %v", err)
	}
	bad := manifest
	bad.Screenshots = append([]ScreenshotEntry(nil), manifest.Screenshots...)
	bad.Screenshots[0].SHA256 = makePNG(bad.Screenshots[0].Path, 100, 100)
	if err := bad.Validate(root, true); err == nil || !strings.Contains(err.Error(), "dimensions/format") {
		t.Fatalf("undersized screenshot accepted: %v", err)
	}
}

func TestCertificationNotesRequireCorrectiveStatementsAndExactBuild(t *testing.T) {
	repoRoot := filepath.Join("..", "..", "..")
	var notes CertificationNotes
	if err := decodeRepoJSON(repoRoot, "docs/store/phase1/certification-notes.json", &notes); err != nil {
		t.Fatal(err)
	}
	product := Product{ID: productID, PackageIdentity: packageIdentity, CoordinatorOrigin: "https://barycenter.relux.works"}
	if err := notes.Validate(product, false); err != nil {
		t.Fatal(err)
	}
	if err := notes.Validate(product, true); err == nil || !strings.Contains(err.Error(), "exact build") {
		t.Fatalf("unfrozen notes passed ready gate: %v", err)
	}
	broken := notes
	broken.Notes = strings.ReplaceAll(broken.Notes, "demo credentials", "review access")
	if err := broken.Validate(product, false); err == nil || !strings.Contains(err.Error(), "demo credentials") {
		t.Fatalf("missing corrective statement accepted: %v", err)
	}
}
