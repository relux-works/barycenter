package policypack

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"relux.works/duet/coordinator/internal/legalops"
)

func repositoryFixture(t *testing.T) (string, Pack, legalops.Checkpoint) {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", "..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	pack, err := Load(filepath.Join(repoRoot, "docs", "compliance", "policy-pack-2026-07-14.json"))
	if err != nil {
		t.Fatal(err)
	}
	inputs, err := legalops.Load(filepath.Join(repoRoot, "docs", "compliance", "legal-ops-inputs.json"))
	if err != nil {
		t.Fatal(err)
	}
	return repoRoot, pack, inputs
}

func TestRepositoryPolicyPackHasExactOwnerApproval(t *testing.T) {
	repoRoot, pack, inputs := repositoryFixture(t)
	if err := pack.Validate(repoRoot, inputs, true); err != nil {
		t.Fatal(err)
	}
	if pack.Review.ApprovedBy == nil || *pack.Review.ApprovedBy != "Ivan Oparin" {
		t.Fatalf("approved_by=%v", pack.Review.ApprovedBy)
	}
}

func TestHeldCopyFailsTheProceedGate(t *testing.T) {
	repoRoot, pack, inputs := repositoryFixture(t)
	pack.Review.ExactContentReviewState = "pending_owner_approval"
	pack.Review.PublicationDecision = "hold"
	pack.Review.ApprovedBy = nil
	pack.Review.ApprovedAt = nil
	pack.Review.Reason = "Test-only held copy."
	if err := pack.Validate(repoRoot, inputs, true); err == nil ||
		!strings.Contains(err.Error(), "want proceed") {
		t.Fatalf("require-proceed error=%v", err)
	}
}

func TestDocumentHashAndTraceabilityTamperingFailClosed(t *testing.T) {
	repoRoot, pack, inputs := repositoryFixture(t)
	pack.Artifacts[0].ENSHA256 = strings.Repeat("0", 64)
	if err := pack.Validate(repoRoot, inputs, false); err == nil ||
		!strings.Contains(err.Error(), "sha256") {
		t.Fatalf("hash tamper error=%v", err)
	}

	_, pack, inputs = repositoryFixture(t)
	pack.Traceability[0].Sections = pack.Traceability[0].Sections[1:]
	if err := pack.Validate(repoRoot, inputs, false); err == nil ||
		!strings.Contains(err.Error(), "has no factual traceability") {
		t.Fatalf("trace tamper error=%v", err)
	}
}

func TestLanguageParityAndSourceAuthorityFailClosed(t *testing.T) {
	repoRoot, pack, inputs := repositoryFixture(t)
	pack.Artifacts[1].Sections = pack.Artifacts[1].Sections[:len(pack.Artifacts[1].Sections)-1]
	if err := pack.Validate(repoRoot, inputs, false); err == nil ||
		!strings.Contains(err.Error(), "EN/RU section IDs") {
		t.Fatalf("section parity error=%v", err)
	}

	_, pack, inputs = repositoryFixture(t)
	pack.Sources[5].URL = "https://example.invalid/store-policy"
	if err := pack.Validate(repoRoot, inputs, false); err == nil ||
		!strings.Contains(err.Error(), "unsupported host") {
		t.Fatalf("source authority error=%v", err)
	}
}

func TestLoadRejectsUnknownManifestFields(t *testing.T) {
	path := filepath.Join(t.TempDir(), "pack.json")
	if err := os.WriteFile(path, []byte(`{"schema_version":1,"unexpected":true}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(path); err == nil || !strings.Contains(err.Error(), "unknown field") {
		t.Fatalf("strict decode error=%v", err)
	}
}
