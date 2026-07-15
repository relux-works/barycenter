package store

import (
	"encoding/json"
	"os"
	"testing"
)

type previousAirServiceResult struct {
	OrbitA int64 `json:"orbit_a"`
	OrbitB int64 `json:"orbit_b"`
	LinkID int64 `json:"link_id"`
}

func TestPreviousCoordinatorLegacyAirRollbackService(t *testing.T) {
	path := os.Getenv("BARYCENTER_PREVIOUS_AIR_DB")
	resultPath := os.Getenv("BARYCENTER_PREVIOUS_AIR_RESULT")
	if path == "" || resultPath == "" {
		t.Fatal("previous Air service environment is incomplete")
	}
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var firstLink int64
	var airs, members, mappings int
	var authority string
	if err := s.db.QueryRow(`SELECT
  (SELECT MIN(link_id) FROM air_legacy_link_mappings),
  (SELECT COUNT(*) FROM airs),
  (SELECT COUNT(*) FROM air_members),
  (SELECT COUNT(*) FROM air_legacy_link_mappings),
  (SELECT mode FROM air_authority WHERE singleton = 1)`).Scan(
		&firstLink, &airs, &members, &mappings, &authority,
	); err != nil || airs != 1 || members != 2 || mappings != 1 || authority != "airs_shadow" {
		t.Fatalf("unknown Phase 2 rows before service airs=%d members=%d mappings=%d authority=%q err=%v", airs, members, mappings, authority, err)
	}
	if err := s.BreakLink(firstLink); err != nil {
		t.Fatal(err)
	}
	a, err := s.CreateOrbit("Previous coordinator A", 8201)
	if err != nil {
		t.Fatal(err)
	}
	b, err := s.CreateOrbit("Previous coordinator B", 8202)
	if err != nil {
		t.Fatal(err)
	}
	code, err := s.ProposeLink(a.ID, 8201)
	if err != nil {
		t.Fatal(err)
	}
	linkID, gotA, err := s.AcceptByCode(code, b.ID)
	if err != nil || gotA != a.ID || linkID == 0 {
		t.Fatalf("accept predecessor link=%d a=%d err=%v", linkID, gotA, err)
	}
	if err := s.ActivateLink(linkID); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT
  (SELECT COUNT(*) FROM airs),
  (SELECT COUNT(*) FROM air_members),
  (SELECT COUNT(*) FROM air_legacy_link_mappings),
  (SELECT mode FROM air_authority WHERE singleton = 1)`).Scan(
		&airs, &members, &mappings, &authority,
	); err != nil || airs != 1 || members != 2 || mappings != 1 || authority != "airs_shadow" {
		t.Fatalf("unknown Phase 2 rows after service airs=%d members=%d mappings=%d authority=%q err=%v", airs, members, mappings, authority, err)
	}
	encoded, err := json.Marshal(previousAirServiceResult{OrbitA: a.ID, OrbitB: b.ID, LinkID: linkID})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}
