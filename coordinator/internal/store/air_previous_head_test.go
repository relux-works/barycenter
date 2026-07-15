//go:build previoushead

package store

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

const airPreviousCoordinatorRevision = "d4964098765ef9e53aa4de2be54d69a25c953cd1"

type previousAirServiceResult struct {
	OrbitA int64 `json:"orbit_a"`
	OrbitB int64 `json:"orbit_b"`
	LinkID int64 `json:"link_id"`
}

// TestAirExactPreviousCoordinatorLegacyServicePreservesPhase2Rows runs the
// exact coordinator predecessor against a links-authoritative database that
// already contains the additive Air shadow. It proves the old binary can
// service legacy links without knowing about, deleting or corrupting Phase 2
// rows, after which the current binary deterministically resumes migration.
func TestAirExactPreviousCoordinatorLegacyServicePreservesPhase2Rows(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate current test source")
	}
	storeDir := filepath.Dir(currentFile)
	repoRoot := filepath.Clean(filepath.Join(storeDir, "..", "..", ".."))
	assertAirPreviousRevision(t, repoRoot)

	path := filepath.Join(t.TempDir(), "air-previous-coordinator.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	firstA := createLegacyOrbit(t, s, "First A", 8101)
	firstB := createLegacyOrbit(t, s, "First B", 8102)
	firstLink := createActiveLegacyLink(t, s, firstA, firstB, 8101)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var firstAirID string
	if err := s.db.QueryRow(`SELECT air_id FROM air_legacy_link_mappings WHERE link_id = ?`, firstLink).Scan(&firstAirID); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	previousDir := prepareAirPreviousCoordinatorTree(t, repoRoot, storeDir)
	resultPath := filepath.Join(t.TempDir(), "previous-air-service.json")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, "go", "test", "-count=1", "./internal/store", "-run", "^TestPreviousCoordinatorLegacyAirRollbackService$")
	cmd.Dir = previousDir
	cmd.Env = append(os.Environ(),
		"BARYCENTER_PREVIOUS_AIR_DB="+path,
		"BARYCENTER_PREVIOUS_AIR_RESULT="+resultPath,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("exact previous coordinator Air service: %v\n%s", err, output)
	}
	encoded, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var result previousAirServiceResult
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}

	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var airs, members, mappings, activeLinks int
	if err := s.db.QueryRow(`SELECT
  (SELECT COUNT(*) FROM airs),
  (SELECT COUNT(*) FROM air_members),
  (SELECT COUNT(*) FROM air_legacy_link_mappings),
  (SELECT COUNT(*) FROM links WHERE state = 'active')`).Scan(
		&airs, &members, &mappings, &activeLinks,
	); err != nil || airs != 2 || members != 4 || mappings != 2 || activeLinks != 1 {
		t.Fatalf("resumed rows airs=%d members=%d mappings=%d active_links=%d err=%v", airs, members, mappings, activeLinks, err)
	}
	if preserved, err := s.AirByID(firstAirID); err != nil || preserved.Status != "parked" {
		t.Fatalf("predecessor preserved first Air=%+v err=%v", preserved, err)
	}
	if _, _, ok, err := s.ActiveLink(firstA); err != nil || ok {
		t.Fatalf("predecessor legacy break ok=%v err=%v", ok, err)
	}
	if linkID, other, ok, err := s.ActiveLink(result.OrbitA); err != nil || !ok || linkID != result.LinkID || other != result.OrbitB {
		t.Fatalf("predecessor legacy create link=%d other=%d ok=%v err=%v", linkID, other, ok, err)
	}
	authority, err := s.AirAuthority()
	if err != nil || authority.Mode != "airs_shadow" || authority.Generation != 1 {
		t.Fatalf("resumed authority=%+v err=%v", authority, err)
	}
	authority, err = s.CutoverLinksToAirs(authority.Generation, 900)
	if err != nil || authority.Mode != "airs_authoritative" {
		t.Fatalf("post-predecessor cutover=%+v err=%v", authority, err)
	}
	if current, _, ok, err := s.ActiveAirForOrbit(result.OrbitA); err != nil || !ok || current == firstAirID {
		t.Fatalf("post-predecessor active Air=%q ok=%v err=%v", current, ok, err)
	}
	if authority, err = s.RollbackAirsToLinks(authority.Generation, 901); err != nil || authority.Mode != "links_authoritative" {
		t.Fatalf("post-predecessor rollback=%+v err=%v", authority, err)
	}
	assertDatabaseHealthy(t, s)
}

func assertAirPreviousRevision(t *testing.T, repoRoot string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", airPreviousCoordinatorRevision+"^{commit}")
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("resolve exact Air predecessor: %v: %s", err, output)
	}
	if strings.TrimSpace(string(output)) != airPreviousCoordinatorRevision {
		t.Fatalf("resolved Air predecessor=%q", strings.TrimSpace(string(output)))
	}
}

func prepareAirPreviousCoordinatorTree(t *testing.T, repoRoot, storeDir string) string {
	t.Helper()
	extractRoot := filepath.Join(t.TempDir(), "air-previous-coordinator")
	if err := os.MkdirAll(extractRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "archive", "--format=tar.gz", airPreviousCoordinatorRevision, "coordinator")
	cmd.Dir = repoRoot
	compressed, err := cmd.Output()
	if err != nil {
		t.Fatalf("archive exact Air predecessor: %v", err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := extractTar(tar.NewReader(reader), extractRoot); err != nil {
		t.Fatal(err)
	}
	driver, err := os.ReadFile(filepath.Join(storeDir, "testdata", "air_previous_coordinator_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	previousStoreDir := filepath.Join(extractRoot, "coordinator", "internal", "store")
	if err := os.WriteFile(filepath.Join(previousStoreDir, "air_previous_coordinator_test.go"), driver, 0o600); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(extractRoot, "coordinator")
}
