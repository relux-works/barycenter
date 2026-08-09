package main

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"relux.works/duet/coordinator/internal/store"
)

// buildShadowAirStoreFile builds a production-like coordinator database: two
// orbits joined by an active legacy link, reopened so initAirSchema backfills
// the link and advances links_authoritative -> airs_shadow. That is the exact
// pre-rollout production state (health: air_rooms_enabled=false). It returns the
// closed database path and the owner orbit id.
func buildShadowAirStoreFile(t *testing.T) (string, int64) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "air-authority.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	owner, err := st.CreateOrbit("Owner", 5001)
	if err != nil {
		t.Fatal(err)
	}
	peer, err := st.CreateOrbit("Peer", 5002)
	if err != nil {
		t.Fatal(err)
	}
	code, err := st.ProposeLink(owner.ID, 5001)
	if err != nil {
		t.Fatal(err)
	}
	linkID, gotA, err := st.AcceptByCode(code, peer.ID)
	if err != nil || gotA != owner.ID || linkID == 0 {
		t.Fatalf("accept link id=%d a=%d err=%v", linkID, gotA, err)
	}
	if err := st.ActivateLink(linkID); err != nil {
		t.Fatal(err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	st, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	authority, err := st.AirAuthority()
	if err != nil || authority.Mode != "airs_shadow" {
		t.Fatalf("expected airs_shadow pre-state, got %+v err=%v", authority, err)
	}
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	return path, owner.ID
}

func reopenAirAuthorityStore(t *testing.T, path string) *store.Store {
	t.Helper()
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func parseAuthorityReceipt(t *testing.T, output string) map[string]string {
	t.Helper()
	fields := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(output), "\n") {
		if line == "" {
			continue
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			t.Fatalf("unparseable receipt line %q", line)
		}
		fields[key] = value
	}
	return fields
}

func TestAirAuthorityCutoverEnablesHealthThenCleanRollback(t *testing.T) {
	path, _ := buildShadowAirStoreFile(t)
	st := reopenAirAuthorityStore(t, path)

	var cut bytes.Buffer
	if err := applyAirAuthorityTransition(st, true, false, 1000, &cut); err != nil {
		t.Fatalf("cutover: %v", err)
	}
	receipt := parseAuthorityReceipt(t, cut.String())
	if receipt["air_authority_command"] != "cutover" ||
		receipt["before_mode"] != "airs_shadow" || receipt["before_air_rooms_enabled"] != "false" {
		t.Fatalf("cutover before-receipt=%v", receipt)
	}
	if receipt["after_mode"] != "airs_authoritative" ||
		receipt["after_air_rooms_enabled"] != "true" || receipt["result"] != "ok" {
		t.Fatalf("cutover after-receipt=%v", receipt)
	}

	// The deployed health surface must now report authoritative ownership.
	health, err := st.Phase2HealthSnapshot(1000)
	if err != nil {
		t.Fatal(err)
	}
	if !health.Features.AirRooms.Enabled || health.Features.AirRooms.State != "airs_authoritative" {
		t.Fatalf("health after cutover=%+v", health.Features.AirRooms)
	}

	// No Air-native writes yet (divergence == 0): a clean rollback reverts to
	// links_authoritative and disables the Air feature again.
	var back bytes.Buffer
	if err := applyAirAuthorityTransition(st, false, true, 2000, &back); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	rb := parseAuthorityReceipt(t, back.String())
	if rb["after_mode"] != "links_authoritative" ||
		rb["after_air_rooms_enabled"] != "false" || rb["result"] != "ok" {
		t.Fatalf("rollback receipt=%v", rb)
	}
	health, err = st.Phase2HealthSnapshot(2000)
	if err != nil {
		t.Fatal(err)
	}
	if health.Features.AirRooms.Enabled {
		t.Fatalf("air rooms still enabled after rollback: %+v", health.Features.AirRooms)
	}
}

func TestAirAuthorityCutoverAllowsCreatePlusInviteProbeThenRollbackHolds(t *testing.T) {
	path, owner := buildShadowAirStoreFile(t)
	st := reopenAirAuthorityStore(t, path)

	// In shadow mode the create is rejected (the production symptom before the
	// rollout); it must not commit a partial Air.
	if _, err := st.CreateAir(store.CreateAirParams{Title: "Family", OwnerOrbitID: owner, CreatedAt: 500}); err == nil {
		t.Fatal("expected Air create to be rejected in shadow mode")
	}

	if err := applyAirAuthorityTransition(st, true, false, 1000, io.Discard); err != nil {
		t.Fatalf("cutover: %v", err)
	}

	// Targeted probe: create an Air plus an initial single-use invite, without a
	// revision conflict or partial commit.
	air, err := st.CreateAir(store.CreateAirParams{Title: "Family", OwnerOrbitID: owner, CreatedAt: 1100})
	if err != nil {
		t.Fatalf("probe Air create: %v", err)
	}
	policy, err := st.AirPolicy(air.ID)
	if err != nil {
		t.Fatal(err)
	}
	invite, err := st.CreateAirInvite(store.AirInvite{
		AirID:           air.ID,
		CodeHash:        strings.Repeat("ab", 32), // 64-char lowercase hex probe hash
		IntendedRole:    "member",
		IssuedByActorID: owner,
		IssuedByOrbitID: owner,
		PolicyRevision:  policy.Revision,
		ExpiresAt:       2000,
		CreatedAt:       1200,
	})
	if err != nil {
		t.Fatalf("probe Air invite: %v", err)
	}
	if invite.Status != "open" || invite.AirID != air.ID {
		t.Fatalf("probe invite=%+v", invite)
	}

	// Airs have now diverged from the legacy links; a clean rollback must refuse
	// and hold rather than silently drop Air-native state. This proves the
	// safety gate: the operator is told to restore from backup instead.
	var back bytes.Buffer
	err = applyAirAuthorityTransition(st, false, true, 2100, &back)
	if err == nil || !errors.Is(err, store.ErrAirRollbackUnsafe) {
		t.Fatalf("expected rollback_hold error, got %v", err)
	}
	rb := parseAuthorityReceipt(t, back.String())
	if rb["result"] != "rollback_hold" || rb["after_mode"] != "rollback_hold" {
		t.Fatalf("rollback-hold receipt=%v", rb)
	}
	authority, err := st.AirAuthority()
	if err != nil || authority.Mode != "rollback_hold" {
		t.Fatalf("authority after held rollback=%+v err=%v", authority, err)
	}
}

func TestAirAuthorityTransitionGuards(t *testing.T) {
	path, _ := buildShadowAirStoreFile(t)
	st := reopenAirAuthorityStore(t, path)

	if err := applyAirAuthorityTransition(st, false, false, 1, io.Discard); err == nil {
		t.Fatal("expected error when neither flag is set")
	}
	if err := applyAirAuthorityTransition(st, true, true, 1, io.Discard); err == nil {
		t.Fatal("expected error when both flags are set")
	}

	// Rollback is refused from shadow (nothing authoritative to revert).
	var refusedRollback bytes.Buffer
	if err := applyAirAuthorityTransition(st, false, true, 1, &refusedRollback); err == nil {
		t.Fatal("expected rollback refusal from shadow")
	}
	if !strings.Contains(refusedRollback.String(), "result=refused") {
		t.Fatalf("rollback-from-shadow output=%q", refusedRollback.String())
	}

	// Cut over, then a second cutover is refused (already authoritative).
	if err := applyAirAuthorityTransition(st, true, false, 2, io.Discard); err != nil {
		t.Fatalf("cutover: %v", err)
	}
	var refusedCutover bytes.Buffer
	if err := applyAirAuthorityTransition(st, true, false, 3, &refusedCutover); err == nil {
		t.Fatal("expected second cutover to be refused")
	}
	if !strings.Contains(refusedCutover.String(), "result=refused") {
		t.Fatalf("double-cutover output=%q", refusedCutover.String())
	}
}

func TestRunAirAuthorityCommandCutsOverStoreFromConfig(t *testing.T) {
	path, _ := buildShadowAirStoreFile(t)
	dir := t.TempDir()
	configPath := filepath.Join(dir, "coordinator.yml")
	configYAML := "listen: \"127.0.0.1:18090\"\n" +
		"db_path: " + path + "\n" +
		"media_dir: " + filepath.Join(dir, "media") + "\n"
	if err := os.WriteFile(configPath, []byte(configYAML), 0o600); err != nil {
		t.Fatal(err)
	}

	output, err := captureAirAuthorityCommandOutput(t, func() error {
		return runAirAuthorityCommand(configPath, true, false)
	})
	if err != nil {
		t.Fatalf("runAirAuthorityCommand: %v", err)
	}
	receipt := parseAuthorityReceipt(t, output)
	if receipt["after_mode"] != "airs_authoritative" || receipt["result"] != "ok" {
		t.Fatalf("command receipt=%v", receipt)
	}

	st := reopenAirAuthorityStore(t, path)
	authority, err := st.AirAuthority()
	if err != nil || authority.Mode != "airs_authoritative" {
		t.Fatalf("store not authoritative after command: %+v err=%v", authority, err)
	}
}

// captureAirAuthorityCommandOutput captures os.Stdout for the config-loading
// wrapper (which writes directly to stdout) and returns both the output and the
// command error, so error paths such as rollback_hold can be asserted.
func captureAirAuthorityCommandOutput(t *testing.T, run func() error) (string, error) {
	t.Helper()
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	previous := os.Stdout
	os.Stdout = writer
	runErr := run()
	_ = writer.Close()
	os.Stdout = previous
	output, readErr := io.ReadAll(reader)
	_ = reader.Close()
	if readErr != nil {
		t.Fatal(readErr)
	}
	return string(output), runErr
}
