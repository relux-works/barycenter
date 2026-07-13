package store

import (
	"encoding/json"
	"errors"
	"os"
	"strconv"
	"testing"
)

type previousHeadResult struct {
	CreatedOrbitID int64  `json:"created_orbit_id"`
	ReboundSlot    string `json:"rebound_slot"`
	ReboundToken   string `json:"rebound_token"`
	NewSlot        string `json:"new_slot"`
	NewToken       string `json:"new_token"`
}

func previousHeadEnvInt64(t *testing.T, name string) int64 {
	t.Helper()
	value, err := strconv.ParseInt(os.Getenv(name), 10, 64)
	if err != nil || value <= 0 {
		t.Fatalf("%s is invalid: %q (%v)", name, os.Getenv(name), err)
	}
	return value
}

func previousHeadEnvString(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s is required", name)
	}
	return value
}

// This file is copied at runtime into the exact previous HEAD source tree by
// the current tagged integration test. It exercises the old Store API itself;
// it does not emulate mutations with SQL.
func TestPreviousHeadFullStoreAuthoritySurface(t *testing.T) {
	path := os.Getenv("BARYCENTER_PREVIOUS_DB")
	resultPath := os.Getenv("BARYCENTER_PREVIOUS_RESULT")
	keepOrbitID := previousHeadEnvInt64(t, "BARYCENTER_KEEP_ORBIT")
	dissolveOrbitID := previousHeadEnvInt64(t, "BARYCENTER_DISSOLVE_ORBIT")
	if path == "" || resultPath == "" {
		t.Fatal("previous-head database/result paths are required")
	}

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	created, err := s.CreateOrbit("Created by previous HEAD", 901)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddMember(keepOrbitID, 303, "companion"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMemberName(keepOrbitID, 303, "Legacy Renamed"); err != nil {
		t.Fatal(err)
	}
	if err := s.TransferPrimary(keepOrbitID, 303); err != nil {
		t.Fatal(err)
	}
	if found, err := s.RevokeSlot(keepOrbitID, "b"); err != nil || !found {
		t.Fatalf("old RevokeSlot found=%v err=%v", found, err)
	}
	reboundSlot, reboundToken, err := s.PairSlot(keepOrbitID, 303)
	if err != nil || reboundSlot != "b" {
		t.Fatalf("old rebound slot=%q err=%v", reboundSlot, err)
	}
	if orbitID, slot, ok, err := s.LookupToken(reboundToken); err != nil || !ok || orbitID != keepOrbitID || slot != "b" {
		t.Fatalf("old rebound LookupToken orbit=%d slot=%q ok=%v err=%v", orbitID, slot, ok, err)
	}
	newSlot, newToken, err := s.PairSlot(keepOrbitID, 303)
	if err != nil || newSlot != "c" {
		t.Fatalf("old new PairSlot slot=%q err=%v", newSlot, err)
	}
	if orbitID, slot, ok, err := s.LookupToken(newToken); err != nil || !ok || orbitID != keepOrbitID || slot != "c" {
		t.Fatalf("old new LookupToken orbit=%d slot=%q ok=%v err=%v", orbitID, slot, ok, err)
	}
	if found, err := s.RevokeSlot(keepOrbitID, "a"); err != nil || !found {
		t.Fatalf("old second RevokeSlot found=%v err=%v", found, err)
	}
	if dissolved, promoted, err := s.LeaveOrbit(keepOrbitID, 202); err != nil || dissolved || promoted != 0 {
		t.Fatalf("old LeaveOrbit dissolved=%v promoted=%d err=%v", dissolved, promoted, err)
	}
	if err := s.DeleteOrbit(dissolveOrbitID); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, err := s.LookupToken(reboundToken); err != nil || !ok {
		t.Fatalf("old live rebound token changed after unrelated dissolve: ok=%v err=%v", ok, err)
	}

	encoded, err := json.Marshal(previousHeadResult{
		CreatedOrbitID: created.ID,
		ReboundSlot:    reboundSlot,
		ReboundToken:   reboundToken,
		NewSlot:        newSlot,
		NewToken:       newToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}

// TestPreviousHeadProjectedGenerationFullSurface is injected into the exact
// pinned predecessor and called once for each rollback generation. Every
// mutation and disabled-orbit check goes through that predecessor's Store API.
func TestPreviousHeadProjectedGenerationFullSurface(t *testing.T) {
	path := previousHeadEnvString(t, "BARYCENTER_PREVIOUS_DB")
	resultPath := previousHeadEnvString(t, "BARYCENTER_PREVIOUS_RESULT")
	keepOrbitID := previousHeadEnvInt64(t, "BARYCENTER_KEEP_ORBIT")
	deleteOrbitID := previousHeadEnvInt64(t, "BARYCENTER_DELETE_ORBIT")
	dissolveOrbitID := previousHeadEnvInt64(t, "BARYCENTER_DISSOLVE_ORBIT")
	disabledOrbitID := previousHeadEnvInt64(t, "BARYCENTER_DISABLED_ORBIT")
	addedMember := previousHeadEnvInt64(t, "BARYCENTER_KEEP_ADDED_MEMBER")
	leavingMember := previousHeadEnvInt64(t, "BARYCENTER_KEEP_LEAVING_MEMBER")
	createdOwner := previousHeadEnvInt64(t, "BARYCENTER_CREATED_OWNER")
	dissolveOwner := previousHeadEnvInt64(t, "BARYCENTER_DISSOLVE_OWNER")
	disabledOwner := previousHeadEnvInt64(t, "BARYCENTER_DISABLED_OWNER")
	disabledBlockedMember := previousHeadEnvInt64(t, "BARYCENTER_DISABLED_BLOCKED_MEMBER")
	disabledToken := previousHeadEnvString(t, "BARYCENTER_DISABLED_TOKEN")
	disabledInvite := previousHeadEnvString(t, "BARYCENTER_DISABLED_INVITE")
	renamedMember := previousHeadEnvString(t, "BARYCENTER_RENAMED_MEMBER")

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	// The current coordinator projected this orbit before the predecessor was
	// opened. Prove the predecessor enforces every projected legacy barrier.
	if _, _, ok, err := s.LookupToken(disabledToken); err != nil || ok {
		t.Fatalf("projected LookupToken ok=%v err=%v", ok, err)
	}
	if _, _, err := s.PairSlot(disabledOrbitID, disabledOwner); !errors.Is(err, ErrLimit) {
		t.Fatalf("projected PairSlot error=%v", err)
	}
	if err := s.AddMember(disabledOrbitID, disabledBlockedMember, "companion"); !errors.Is(err, ErrLimit) {
		t.Fatalf("projected AddMember error=%v", err)
	}
	if orbitID, issuedBy, err := s.ConsumeInvite(disabledInvite, "member"); err != nil || orbitID != 0 || issuedBy != 0 {
		t.Fatalf("projected ConsumeInvite orbit=%d issued_by=%d err=%v", orbitID, issuedBy, err)
	}

	// Exercise the full legacy authority surface in the same exact-old interval.
	created, err := s.CreateOrbit("Created by projected previous HEAD", createdOwner)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddMember(keepOrbitID, addedMember, "companion"); err != nil {
		t.Fatal(err)
	}
	if err := s.SetMemberName(keepOrbitID, addedMember, renamedMember); err != nil {
		t.Fatal(err)
	}
	if err := s.TransferPrimary(keepOrbitID, addedMember); err != nil {
		t.Fatal(err)
	}
	if found, err := s.RevokeSlot(keepOrbitID, "b"); err != nil || !found {
		t.Fatalf("old RevokeSlot for rebind found=%v err=%v", found, err)
	}
	reboundSlot, reboundToken, err := s.PairSlot(keepOrbitID, addedMember)
	if err != nil || reboundSlot != "b" {
		t.Fatalf("old same-coordinate rebind slot=%q err=%v", reboundSlot, err)
	}
	if orbitID, slot, ok, err := s.LookupToken(reboundToken); err != nil || !ok || orbitID != keepOrbitID || slot != "b" {
		t.Fatalf("old rebound LookupToken orbit=%d slot=%q ok=%v err=%v", orbitID, slot, ok, err)
	}
	newSlot, newToken, err := s.PairSlot(keepOrbitID, addedMember)
	if err != nil || newSlot != "c" {
		t.Fatalf("old new PairSlot slot=%q err=%v", newSlot, err)
	}
	if orbitID, slot, ok, err := s.LookupToken(newToken); err != nil || !ok || orbitID != keepOrbitID || slot != "c" {
		t.Fatalf("old new LookupToken orbit=%d slot=%q ok=%v err=%v", orbitID, slot, ok, err)
	}
	if found, err := s.RevokeSlot(keepOrbitID, "a"); err != nil || !found {
		t.Fatalf("old second RevokeSlot found=%v err=%v", found, err)
	}
	if dissolved, promoted, err := s.LeaveOrbit(keepOrbitID, leavingMember); err != nil || dissolved || promoted != 0 {
		t.Fatalf("old LeaveOrbit dissolved=%v promoted=%d err=%v", dissolved, promoted, err)
	}
	if err := s.DeleteOrbit(deleteOrbitID); err != nil {
		t.Fatal(err)
	}
	if dissolved, promoted, err := s.LeaveOrbit(dissolveOrbitID, dissolveOwner); err != nil || !dissolved || promoted != 0 {
		t.Fatalf("old last-member dissolve=%v promoted=%d err=%v", dissolved, promoted, err)
	}

	encoded, err := json.Marshal(previousHeadResult{
		CreatedOrbitID: created.ID,
		ReboundSlot:    reboundSlot,
		ReboundToken:   reboundToken,
		NewSlot:        newSlot,
		NewToken:       newToken,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}
