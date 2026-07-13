//go:build previoushead

package store

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"
)

const previousCoordinatorRevision = "e8bd240664a40b9cc78b974f3c34ad30712e2aa5"

type previousHeadIntegrationResult struct {
	CreatedOrbitID int64  `json:"created_orbit_id"`
	ReboundSlot    string `json:"rebound_slot"`
	ReboundToken   string `json:"rebound_token"`
	NewSlot        string `json:"new_slot"`
	NewToken       string `json:"new_token"`
}

type exactPreviousGenerationFixture struct {
	generation            int
	keepOrbitID           int64
	deleteOrbitID         int64
	dissolveOrbitID       int64
	disabledOrbitID       int64
	keepOwner             int64
	keepLeavingMember     int64
	keepAddedMember       int64
	createdOwner          int64
	dissolveOwner         int64
	disabledOwner         int64
	disabledBlockedMember int64
	renamedMember         string
	oldANode              string
	oldAControl           string
	oldBNode              string
	deleteNode            string
	dissolveNode          string
	disabledNode          string
	disabledInvite        string
	oldAActorID           int64
	oldBActorID           int64
	deleteActorID         int64
	dissolveActorID       int64
	disabledActorID       int64
	wantMaxPulsars        int
	wantMaxMembers        int
}

// TestR8ExactPreviousHEADAuthorityRoundTrip extracts the exact predecessor
// revision, injects a test that calls its real Store API, runs that revision,
// and then reopens the resulting database through the new reconciliation path.
// It is tagged because it starts a nested Go toolchain and is an explicit
// rollback-compatibility gate rather than a unit test.
func TestR8ExactPreviousHEADAuthorityRoundTrip(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate current test source")
	}
	storeDir := filepath.Dir(currentFile)
	repoRoot := filepath.Clean(filepath.Join(storeDir, "..", "..", ".."))
	assertExactRevisionExists(t, repoRoot)

	path := filepath.Join(t.TempDir(), "previous-head-roundtrip.db")
	s, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	keep, err := s.CreateOrbit("Keep through previous HEAD", 101)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddMember(keep.ID, 202, "companion"); err != nil {
		t.Fatal(err)
	}
	oldANode, oldAContext := provisionTestInstallation(t, s, keep.ID, 101)
	controlA, recoveryIDA, recoverySecretA := newProvisioningMaterial(t)
	if err := s.ProvisionInstallationSecrets(
		Identity{Kind: IdentityTelegram, TelegramUserID: 101},
		oldAContext.ActorID, controlA, recoveryIDA, recoverySecretA,
	); err != nil {
		t.Fatal(err)
	}
	_, oldBNode, err := s.PairSlot(keep.ID, 202)
	if err != nil {
		t.Fatal(err)
	}
	oldBContext, err := s.ResolveTokenActorContext(oldBNode)
	if err != nil {
		t.Fatal(err)
	}
	dissolve, err := s.CreateOrbit("Dissolve through previous HEAD", 404)
	if err != nil {
		t.Fatal(err)
	}
	_, dissolveNode, err := s.PairSlot(dissolve.ID, 404)
	if err != nil {
		t.Fatal(err)
	}
	dissolveContext, err := s.ResolveTokenActorContext(dissolveNode)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	result := runExactPreviousHeadStoreTest(t, repoRoot, storeDir, path, keep.ID, dissolve.ID)
	if result.ReboundSlot != "b" || result.NewSlot != "c" || result.CreatedOrbitID == 0 || result.ReboundToken == "" || result.NewToken == "" {
		t.Fatal("previous HEAD result contains incomplete authority coordinates")
	}

	s, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	member303, err := s.ResolveTelegramActorContext(303)
	if err != nil || member303.OrbitID != keep.ID || member303.Role != "primary" {
		t.Fatalf("previous HEAD promoted member context=%+v err=%v", member303, err)
	}
	var displayName string
	if err := s.db.QueryRow(`SELECT display_name FROM actors WHERE id = ?`, member303.ActorID).Scan(&displayName); err != nil || displayName != "Legacy Renamed" {
		t.Fatalf("previous HEAD member name=%q err=%v", displayName, err)
	}
	member202 := telegramActorID(t, s, 202)
	assertMembership(t, s, member202, keep.ID, "companion", true)

	for name, token := range map[string]string{
		"revoked A node": oldANode,
		"rebound B node": oldBNode,
		"dissolved node": dissolveNode,
	} {
		if _, _, found, err := s.LookupToken(token); err != nil || found {
			t.Fatalf("%s retained legacy authority: found=%v err=%v", name, found, err)
		}
	}
	if _, err := s.ResolveTokenActorContext(controlA); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked A control authority error=%v", err)
	}
	assertRevokedInstallationActor(t, s, oldAContext.ActorID)
	assertRevokedInstallationActor(t, s, oldBContext.ActorID)

	for _, credential := range []struct {
		slot  string
		token string
	}{
		{slot: result.ReboundSlot, token: result.ReboundToken},
		{slot: result.NewSlot, token: result.NewToken},
	} {
		slot := credential.slot
		orbitID, playbackSlot, found, err := s.LookupPlaybackToken(credential.token)
		if err != nil || !found || orbitID != keep.ID || playbackSlot != slot {
			t.Fatalf("old-minted token for slot %s playback orbit=%d slot=%q found=%v err=%v", slot, orbitID, playbackSlot, found, err)
		}
		resolved, err := s.ResolveTokenActorContext(credential.token)
		if err != nil || resolved.OrbitID != keep.ID || resolved.Slot != slot || resolved.Role != "primary" || resolved.Capabilities != CapabilityNode {
			t.Fatalf("old-minted token for slot %s context=%+v err=%v", slot, resolved, err)
		}
		var actorID, pairedBy int64
		var role string
		if err := s.db.QueryRow(`SELECT ic.actor_id, sl.paired_by, m.role
FROM installation_credentials ic
JOIN slots sl ON sl.orbit_id = ic.slot_orbit_id AND sl.slot = ic.slot_name
JOIN memberships m ON m.actor_id = ic.actor_id AND m.orbit_id = ic.slot_orbit_id AND m.left_at IS NULL
WHERE ic.slot_orbit_id = ? AND ic.slot_name = ?`, keep.ID, slot).Scan(&actorID, &pairedBy, &role); err != nil {
			t.Fatalf("reconciled slot %s: %v", slot, err)
		}
		if actorID != resolved.ActorID || actorID == oldBContext.ActorID || pairedBy != 303 || role != "primary" {
			t.Fatalf("reconciled slot %s actor=%d old=%d paired_by=%d role=%q", slot, actorID, oldBContext.ActorID, pairedBy, role)
		}
	}

	var dissolvedRows int
	if err := s.db.QueryRow(`SELECT
  (SELECT COUNT(*) FROM memberships WHERE orbit_id = ?) +
  (SELECT COUNT(*) FROM installation_credentials WHERE slot_orbit_id = ?)`,
		dissolve.ID, dissolve.ID).Scan(&dissolvedRows); err != nil || dissolvedRows != 0 {
		t.Fatalf("dissolved additive rows=%d err=%v", dissolvedRows, err)
	}
	assertRevokedInstallationActor(t, s, dissolveContext.ActorID)
	created, err := s.ResolveTelegramActorContext(901)
	if err != nil || created.OrbitID != result.CreatedOrbitID || created.Role != "primary" {
		t.Fatalf("previous HEAD CreateOrbit projection=%+v created_orbit_id=%d err=%v", created, result.CreatedOrbitID, err)
	}
	assertDatabaseHealthy(t, s)
}

// TestR8ExactPreviousHEADTwoGenerationProjectionComposition proves two full
// new-on -> projection -> exact-old mutation -> re-enable generations against
// one database. It closes the composition gap left by the separate exact-old
// and current-feature-off rollback tests.
func TestR8ExactPreviousHEADTwoGenerationProjectionComposition(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate current test source")
	}
	storeDir := filepath.Dir(currentFile)
	repoRoot := filepath.Clean(filepath.Join(storeDir, "..", "..", ".."))
	assertExactRevisionExists(t, repoRoot)
	previousCoordinatorDir := prepareExactPreviousHeadStoreTree(t, repoRoot, storeDir)
	path := filepath.Join(t.TempDir(), "exact-old-two-generations.db")

	var disabledOrbitID, disabledOwner int64
	for generation := 1; generation <= 2; generation++ {
		s, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
		if err != nil {
			t.Fatal(err)
		}
		fixture := prepareExactPreviousGeneration(t, s, generation, disabledOrbitID, disabledOwner)
		disabledOrbitID, disabledOwner = fixture.disabledOrbitID, fixture.disabledOwner
		assertPendingProjectionGeneration(t, s, fixture)
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}

		result := runExactPreviousHeadProjectionGeneration(t, previousCoordinatorDir, path, fixture)
		s, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
		if err != nil {
			t.Fatal(err)
		}
		verifyExactPreviousGeneration(t, s, fixture, result)
		proveProjectedSlotRequiresExplicitRepair(t, s, fixture)
		assertDatabaseHealthy(t, s)
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

// TestR8ExactPreviousHEADConfigBootstrapContract pins the deployment-side
// rollback constraint that is easy to miss in database-only rehearsals. The
// predecessor uses yaml.KnownFields(true), so a current-only YAML key prevents
// that binary from booting. Rollout therefore enables self-service onboarding
// through the environment while the predecessor remains a rollback target and
// keeps the YAML itself predecessor-neutral.
func TestR8ExactPreviousHEADConfigBootstrapContract(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate current test source")
	}
	storeDir := filepath.Dir(currentFile)
	repoRoot := filepath.Clean(filepath.Join(storeDir, "..", "..", ".."))
	assertExactRevisionExists(t, repoRoot)
	previousCoordinatorDir := prepareExactPreviousHeadStoreTree(t, repoRoot, storeDir)

	driver, err := os.ReadFile(filepath.Join(storeDir, "testdata", "previous_head_config_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	previousConfigDir := filepath.Join(previousCoordinatorDir, "internal", "config")
	if err := os.WriteFile(filepath.Join(previousConfigDir, "previous_head_config_test.go"), driver, 0o600); err != nil {
		t.Fatal(err)
	}

	testCtx, cancelTest := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelTest()
	cmd := exec.CommandContext(testCtx, "go", "test", "-count=1", "./internal/config", "-run", "^TestPreviousHeadRollbackConfigBootstrapContract$")
	cmd.Dir = previousCoordinatorDir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("exact previous HEAD config bootstrap test: %v\n%s", err, output)
	}
}

func prepareExactPreviousGeneration(t *testing.T, s *Store, generation int, disabledOrbitID, disabledOwner int64) exactPreviousGenerationFixture {
	t.Helper()
	base := int64(generation * 10000)
	fixture := exactPreviousGenerationFixture{
		generation:            generation,
		keepOwner:             base + 101,
		keepLeavingMember:     base + 202,
		keepAddedMember:       base + 303,
		createdOwner:          base + 901,
		dissolveOwner:         base + 505,
		disabledBlockedMember: base + 707,
		renamedMember:         "Legacy Renamed Generation " + strconv.Itoa(generation),
		wantMaxPulsars:        5,
		wantMaxMembers:        10,
	}
	keep, err := s.CreateOrbit("Exact old keep generation "+strconv.Itoa(generation), fixture.keepOwner)
	if err != nil {
		t.Fatal(err)
	}
	fixture.keepOrbitID = keep.ID
	if err := s.AddMember(keep.ID, fixture.keepLeavingMember, "companion"); err != nil {
		t.Fatal(err)
	}
	var oldAContext ActorContext
	fixture.oldANode, oldAContext = provisionTestInstallation(t, s, keep.ID, fixture.keepOwner)
	fixture.oldAActorID = oldAContext.ActorID
	var recoveryID, recoverySecret string
	fixture.oldAControl, recoveryID, recoverySecret = newProvisioningMaterial(t)
	if err := s.ProvisionInstallationSecrets(
		Identity{Kind: IdentityTelegram, TelegramUserID: fixture.keepOwner},
		fixture.oldAActorID, fixture.oldAControl, recoveryID, recoverySecret,
	); err != nil {
		t.Fatal(err)
	}
	_, fixture.oldBNode, err = s.PairSlot(keep.ID, fixture.keepLeavingMember)
	if err != nil {
		t.Fatal(err)
	}
	oldBContext, err := s.ResolveTokenActorContext(fixture.oldBNode)
	if err != nil {
		t.Fatal(err)
	}
	fixture.oldBActorID = oldBContext.ActorID

	deleteOrbit, err := s.CreateOrbit("Exact old delete generation "+strconv.Itoa(generation), base+404)
	if err != nil {
		t.Fatal(err)
	}
	fixture.deleteOrbitID = deleteOrbit.ID
	var oldDeleteContext ActorContext
	fixture.deleteNode, oldDeleteContext = provisionTestInstallation(t, s, deleteOrbit.ID, base+404)
	fixture.deleteActorID = oldDeleteContext.ActorID

	dissolveOrbit, err := s.CreateOrbit("Exact old dissolve generation "+strconv.Itoa(generation), fixture.dissolveOwner)
	if err != nil {
		t.Fatal(err)
	}
	fixture.dissolveOrbitID = dissolveOrbit.ID
	var oldDissolveContext ActorContext
	fixture.dissolveNode, oldDissolveContext = provisionTestInstallation(t, s, dissolveOrbit.ID, fixture.dissolveOwner)
	fixture.dissolveActorID = oldDissolveContext.ActorID

	if generation == 1 {
		fixture.disabledOwner = base + 606
		disabled, err := s.CreateOrbit("Exact old projected orbit", fixture.disabledOwner)
		if err != nil {
			t.Fatal(err)
		}
		fixture.disabledOrbitID = disabled.ID
	} else {
		fixture.disabledOrbitID = disabledOrbitID
		fixture.disabledOwner = disabledOwner
		fixture.wantMaxPulsars = 3
		fixture.wantMaxMembers = 7
		if _, err := s.db.Exec(`UPDATE orbits SET max_pulsars = ?, max_members = ? WHERE id = ? AND status = 'active'`,
			fixture.wantMaxPulsars, fixture.wantMaxMembers, fixture.disabledOrbitID); err != nil {
			t.Fatal(err)
		}
	}
	var disabledContext ActorContext
	fixture.disabledNode, disabledContext = provisionTestInstallation(t, s, fixture.disabledOrbitID, fixture.disabledOwner)
	fixture.disabledActorID = disabledContext.ActorID
	fixture.disabledInvite, err = s.NewInvite(fixture.disabledOrbitID, fixture.disabledOwner)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE orbits SET status = 'disabled' WHERE id = ?`, fixture.disabledOrbitID); err != nil {
		t.Fatal(err)
	}
	if err := s.ProjectIdentityForLegacyRollback(); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func assertPendingProjectionGeneration(t *testing.T, s *Store, fixture exactPreviousGenerationFixture) {
	t.Helper()
	var maxPulsars, maxMembers, originalPulsars, originalMembers int
	var restored sql.NullInt64
	if err := s.db.QueryRow(`SELECT o.max_pulsars, o.max_members,
       rp.original_max_pulsars, rp.original_max_members, rp.restored_at
FROM orbits o JOIN rollback_projections rp ON rp.orbit_id = o.id
WHERE o.id = ?`, fixture.disabledOrbitID).Scan(
		&maxPulsars, &maxMembers, &originalPulsars, &originalMembers, &restored,
	); err != nil {
		t.Fatal(err)
	}
	if maxPulsars != 0 || maxMembers != 0 || originalPulsars != fixture.wantMaxPulsars || originalMembers != fixture.wantMaxMembers || restored.Valid {
		t.Fatalf("generation %d pending projection current=%d/%d journal=%d/%d restored=%v",
			fixture.generation, maxPulsars, maxMembers, originalPulsars, originalMembers, restored.Valid)
	}
	var revoked, inviteUsed sql.NullInt64
	if err := s.db.QueryRow(`SELECT revoked_at FROM slots WHERE orbit_id = ? AND token_hash = ?`,
		fixture.disabledOrbitID, hashToken(fixture.disabledNode)).Scan(&revoked); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT used_at FROM invites WHERE code = ?`, fixture.disabledInvite).Scan(&inviteUsed); err != nil {
		t.Fatal(err)
	}
	if !revoked.Valid || !inviteUsed.Valid {
		t.Fatalf("generation %d projection revoked=%v invite_used=%v", fixture.generation, revoked.Valid, inviteUsed.Valid)
	}
}

func verifyExactPreviousGeneration(t *testing.T, s *Store, fixture exactPreviousGenerationFixture, result previousHeadIntegrationResult) {
	t.Helper()
	if result.ReboundSlot != "b" || result.NewSlot != "c" || result.CreatedOrbitID == 0 || result.ReboundToken == "" || result.NewToken == "" {
		t.Fatalf("generation %d previous HEAD returned incomplete authority coordinates", fixture.generation)
	}

	added, err := s.ResolveTelegramActorContext(fixture.keepAddedMember)
	if err != nil || added.OrbitID != fixture.keepOrbitID || added.Role != "primary" {
		t.Fatalf("generation %d added/promoted context=%+v err=%v", fixture.generation, added, err)
	}
	owner, err := s.ResolveTelegramActorContext(fixture.keepOwner)
	if err != nil || owner.OrbitID != fixture.keepOrbitID || owner.Role != "companion" {
		t.Fatalf("generation %d original owner context=%+v err=%v", fixture.generation, owner, err)
	}
	var displayName string
	if err := s.db.QueryRow(`SELECT display_name FROM actors WHERE id = ?`, added.ActorID).Scan(&displayName); err != nil || displayName != fixture.renamedMember {
		t.Fatalf("generation %d renamed member=%q err=%v", fixture.generation, displayName, err)
	}
	leavingActor := telegramActorID(t, s, fixture.keepLeavingMember)
	assertMembership(t, s, leavingActor, fixture.keepOrbitID, "companion", true)

	for name, token := range map[string]string{
		"revoked original a":    fixture.oldANode,
		"rebound original b":    fixture.oldBNode,
		"explicitly deleted":    fixture.deleteNode,
		"last-member dissolved": fixture.dissolveNode,
		"projected disabled":    fixture.disabledNode,
	} {
		if _, _, found, err := s.LookupToken(token); err != nil || found {
			t.Fatalf("generation %d %s token found=%v err=%v", fixture.generation, name, found, err)
		}
	}
	if _, err := s.ResolveTokenActorContext(fixture.oldAControl); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("generation %d revoked control error=%v", fixture.generation, err)
	}
	for _, actorID := range []int64{
		fixture.oldAActorID,
		fixture.oldBActorID,
		fixture.deleteActorID,
		fixture.dissolveActorID,
		fixture.disabledActorID,
	} {
		assertRevokedInstallationActor(t, s, actorID)
	}

	for _, credential := range []struct {
		slot  string
		token string
	}{
		{slot: result.ReboundSlot, token: result.ReboundToken},
		{slot: result.NewSlot, token: result.NewToken},
	} {
		orbitID, slot, found, err := s.LookupPlaybackToken(credential.token)
		if err != nil || !found || orbitID != fixture.keepOrbitID || slot != credential.slot {
			t.Fatalf("generation %d old-minted slot %s playback orbit=%d slot=%q found=%v err=%v",
				fixture.generation, credential.slot, orbitID, slot, found, err)
		}
		ctx, err := s.ResolveTokenActorContext(credential.token)
		if err != nil || ctx.OrbitID != fixture.keepOrbitID || ctx.Slot != credential.slot || ctx.Role != "primary" || ctx.Capabilities != CapabilityNode {
			t.Fatalf("generation %d old-minted slot %s context=%+v err=%v", fixture.generation, credential.slot, ctx, err)
		}
		var pairedBy int64
		var role string
		if err := s.db.QueryRow(`SELECT sl.paired_by, m.role
FROM installation_credentials ic
JOIN slots sl ON sl.orbit_id = ic.slot_orbit_id AND sl.slot = ic.slot_name
JOIN memberships m ON m.actor_id = ic.actor_id AND m.orbit_id = ic.slot_orbit_id AND m.left_at IS NULL
WHERE ic.slot_orbit_id = ? AND ic.slot_name = ?`, fixture.keepOrbitID, credential.slot).Scan(&pairedBy, &role); err != nil {
			t.Fatal(err)
		}
		if pairedBy != fixture.keepAddedMember || role != "primary" {
			t.Fatalf("generation %d old-minted slot %s paired_by=%d role=%q", fixture.generation, credential.slot, pairedBy, role)
		}
	}

	created, err := s.ResolveTelegramActorContext(fixture.createdOwner)
	if err != nil || created.OrbitID != result.CreatedOrbitID || created.Role != "primary" {
		t.Fatalf("generation %d previous CreateOrbit context=%+v result=%d err=%v", fixture.generation, created, result.CreatedOrbitID, err)
	}
	for _, orbitID := range []int64{fixture.deleteOrbitID, fixture.dissolveOrbitID} {
		orbit, err := s.GetOrbit(orbitID)
		if err != nil || orbit != nil {
			t.Fatalf("generation %d removed orbit %d value=%+v err=%v", fixture.generation, orbitID, orbit, err)
		}
		var rows int
		if err := s.db.QueryRow(`SELECT
  (SELECT COUNT(*) FROM memberships WHERE orbit_id = ?) +
  (SELECT COUNT(*) FROM installation_credentials WHERE slot_orbit_id = ?)`, orbitID, orbitID).Scan(&rows); err != nil || rows != 0 {
			t.Fatalf("generation %d removed orbit %d additive rows=%d err=%v", fixture.generation, orbitID, rows, err)
		}
	}

	var status string
	var maxPulsars, maxMembers, originalPulsars, originalMembers int
	var restored sql.NullInt64
	if err := s.db.QueryRow(`SELECT o.status, o.max_pulsars, o.max_members,
       rp.original_max_pulsars, rp.original_max_members, rp.restored_at
FROM orbits o JOIN rollback_projections rp ON rp.orbit_id = o.id
WHERE o.id = ?`, fixture.disabledOrbitID).Scan(
		&status, &maxPulsars, &maxMembers, &originalPulsars, &originalMembers, &restored,
	); err != nil {
		t.Fatal(err)
	}
	if status != "disabled" || maxPulsars != fixture.wantMaxPulsars || maxMembers != fixture.wantMaxMembers ||
		originalPulsars != fixture.wantMaxPulsars || originalMembers != fixture.wantMaxMembers || !restored.Valid {
		t.Fatalf("generation %d restored projection status=%q current=%d/%d journal=%d/%d restored=%v",
			fixture.generation, status, maxPulsars, maxMembers, originalPulsars, originalMembers, restored.Valid)
	}
	if member, err := s.MemberOf(fixture.disabledBlockedMember); err != nil || member != nil {
		t.Fatalf("generation %d blocked member=%+v err=%v", fixture.generation, member, err)
	}
}

func proveProjectedSlotRequiresExplicitRepair(t *testing.T, s *Store, fixture exactPreviousGenerationFixture) {
	t.Helper()
	if _, err := s.db.Exec(`UPDATE orbits SET status = 'active' WHERE id = ?`, fixture.disabledOrbitID); err != nil {
		t.Fatal(err)
	}
	if err := s.ReconcileIdentity(); err != nil {
		t.Fatal(err)
	}
	if _, _, found, err := s.LookupPlaybackToken(fixture.disabledNode); err != nil || found {
		t.Fatalf("generation %d projected token revived by status change: found=%v err=%v", fixture.generation, found, err)
	}
	var credentials int
	var revoked sql.NullInt64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM installation_credentials WHERE slot_orbit_id = ? AND slot_name = 'a'`, fixture.disabledOrbitID).Scan(&credentials); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT revoked_at FROM slots WHERE orbit_id = ? AND slot = 'a'`, fixture.disabledOrbitID).Scan(&revoked); err != nil {
		t.Fatal(err)
	}
	if credentials != 0 || !revoked.Valid {
		t.Fatalf("generation %d projected slot credentials=%d revoked=%v", fixture.generation, credentials, revoked.Valid)
	}
}

func assertExactRevisionExists(t *testing.T, repoRoot string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, "git", "rev-parse", previousCoordinatorRevision+"^{commit}")
	cmd.Dir = repoRoot
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("resolve exact previous coordinator revision: %v: %s", err, output)
	}
	if strings.TrimSpace(string(output)) != previousCoordinatorRevision {
		t.Fatalf("resolved previous revision=%q, want %q", strings.TrimSpace(string(output)), previousCoordinatorRevision)
	}
}

func runExactPreviousHeadStoreTest(t *testing.T, repoRoot, storeDir, dbPath string, keepOrbitID, dissolveOrbitID int64) previousHeadIntegrationResult {
	t.Helper()
	previousCoordinatorDir := prepareExactPreviousHeadStoreTree(t, repoRoot, storeDir)
	resultPath := filepath.Join(t.TempDir(), "previous-head-result.json")
	testCtx, cancelTest := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelTest()
	cmd := exec.CommandContext(testCtx, "go", "test", "-count=1", "./internal/store", "-run", "^TestPreviousHeadFullStoreAuthoritySurface$")
	cmd.Dir = previousCoordinatorDir
	cmd.Env = append(os.Environ(),
		"BARYCENTER_PREVIOUS_DB="+dbPath,
		"BARYCENTER_PREVIOUS_RESULT="+resultPath,
		"BARYCENTER_KEEP_ORBIT="+strconv.FormatInt(keepOrbitID, 10),
		"BARYCENTER_DISSOLVE_ORBIT="+strconv.FormatInt(dissolveOrbitID, 10),
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("exact previous HEAD Store API test: %v\n%s", err, output)
	}
	return readPreviousHeadIntegrationResult(t, resultPath)
}

func prepareExactPreviousHeadStoreTree(t *testing.T, repoRoot, storeDir string) string {
	t.Helper()
	extractRoot := filepath.Join(t.TempDir(), "previous-head")
	if err := os.MkdirAll(extractRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	archiveCtx, cancelArchive := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelArchive()
	archive := exec.CommandContext(archiveCtx, "git", "archive", "--format=tar.gz", previousCoordinatorRevision, "coordinator")
	archive.Dir = repoRoot
	compressed, err := archive.Output()
	if err != nil {
		t.Fatalf("archive exact previous coordinator: %v", err)
	}
	reader, err := gzip.NewReader(bytes.NewReader(compressed))
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	if err := extractTar(tar.NewReader(reader), extractRoot); err != nil {
		t.Fatal(err)
	}

	driver, err := os.ReadFile(filepath.Join(storeDir, "testdata", "previous_head_authority_test.go"))
	if err != nil {
		t.Fatal(err)
	}
	previousStoreDir := filepath.Join(extractRoot, "coordinator", "internal", "store")
	if err := os.WriteFile(filepath.Join(previousStoreDir, "previous_head_authority_test.go"), driver, 0o600); err != nil {
		t.Fatal(err)
	}
	return filepath.Join(extractRoot, "coordinator")
}

func runExactPreviousHeadProjectionGeneration(t *testing.T, previousCoordinatorDir, dbPath string, fixture exactPreviousGenerationFixture) previousHeadIntegrationResult {
	t.Helper()
	resultPath := filepath.Join(t.TempDir(), "previous-head-result.json")
	testCtx, cancelTest := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancelTest()
	cmd := exec.CommandContext(testCtx, "go", "test", "-count=1", "./internal/store", "-run", "^TestPreviousHeadProjectedGenerationFullSurface$")
	cmd.Dir = previousCoordinatorDir
	cmd.Env = append(os.Environ(),
		"BARYCENTER_PREVIOUS_DB="+dbPath,
		"BARYCENTER_PREVIOUS_RESULT="+resultPath,
		"BARYCENTER_KEEP_ORBIT="+strconv.FormatInt(fixture.keepOrbitID, 10),
		"BARYCENTER_DELETE_ORBIT="+strconv.FormatInt(fixture.deleteOrbitID, 10),
		"BARYCENTER_DISSOLVE_ORBIT="+strconv.FormatInt(fixture.dissolveOrbitID, 10),
		"BARYCENTER_DISABLED_ORBIT="+strconv.FormatInt(fixture.disabledOrbitID, 10),
		"BARYCENTER_KEEP_ADDED_MEMBER="+strconv.FormatInt(fixture.keepAddedMember, 10),
		"BARYCENTER_KEEP_LEAVING_MEMBER="+strconv.FormatInt(fixture.keepLeavingMember, 10),
		"BARYCENTER_CREATED_OWNER="+strconv.FormatInt(fixture.createdOwner, 10),
		"BARYCENTER_DISSOLVE_OWNER="+strconv.FormatInt(fixture.dissolveOwner, 10),
		"BARYCENTER_DISABLED_OWNER="+strconv.FormatInt(fixture.disabledOwner, 10),
		"BARYCENTER_DISABLED_BLOCKED_MEMBER="+strconv.FormatInt(fixture.disabledBlockedMember, 10),
		"BARYCENTER_DISABLED_TOKEN="+fixture.disabledNode,
		"BARYCENTER_DISABLED_INVITE="+fixture.disabledInvite,
		"BARYCENTER_RENAMED_MEMBER="+fixture.renamedMember,
	)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("exact previous HEAD generation %d Store API test: %v\n%s", fixture.generation, err, output)
	}
	return readPreviousHeadIntegrationResult(t, resultPath)
}

func readPreviousHeadIntegrationResult(t *testing.T, resultPath string) previousHeadIntegrationResult {
	t.Helper()
	encoded, err := os.ReadFile(resultPath)
	if err != nil {
		t.Fatal(err)
	}
	var result previousHeadIntegrationResult
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func extractTar(reader *tar.Reader, root string) error {
	for {
		header, err := reader.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		path := filepath.Join(root, filepath.Clean(header.Name))
		if !strings.HasPrefix(path, root+string(os.PathSeparator)) {
			return errors.New("archive path escapes extraction root")
		}
		switch header.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(path, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				return err
			}
			file, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, os.FileMode(header.Mode))
			if err != nil {
				return err
			}
			_, copyErr := io.Copy(file, reader)
			closeErr := file.Close()
			if copyErr != nil {
				return copyErr
			}
			if closeErr != nil {
				return closeErr
			}
		}
	}
}

func assertRevokedInstallationActor(t *testing.T, s *Store, actorID int64) {
	t.Helper()
	var revoked bool
	var credentials int
	if err := s.db.QueryRow(`SELECT revoked_at IS NOT NULL FROM actors WHERE id = ?`, actorID).Scan(&revoked); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM installation_credentials WHERE actor_id = ?`, actorID).Scan(&credentials); err != nil {
		t.Fatal(err)
	}
	if !revoked || credentials != 0 {
		t.Fatalf("installation actor %d revoked=%v credentials=%d", actorID, revoked, credentials)
	}
}
