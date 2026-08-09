package store

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"relux.works/duet/coordinator/internal/session"
)

func createLegacyOrbit(t *testing.T, s *Store, title string, userID int64) int64 {
	t.Helper()
	orbit, err := s.CreateOrbit(title, userID)
	if err != nil {
		t.Fatal(err)
	}
	return orbit.ID
}

func createActiveLegacyLink(t *testing.T, s *Store, a, b, userA int64) int64 {
	t.Helper()
	code, err := s.ProposeLink(a, userA)
	if err != nil {
		t.Fatal(err)
	}
	linkID, gotA, err := s.AcceptByCode(code, b)
	if err != nil || gotA != a || linkID == 0 {
		t.Fatalf("accept link id=%d a=%d err=%v", linkID, gotA, err)
	}
	if err := s.ActivateLink(linkID); err != nil {
		t.Fatal(err)
	}
	return linkID
}

func reopenAirStore(t *testing.T, path string) *Store {
	t.Helper()
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestAirSchemaFreshRepositoryLifecycleAndOneActiveInvariant(t *testing.T) {
	path := filepath.Join(t.TempDir(), "air-fresh.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	owner := createLegacyOrbit(t, s, "Owner", 1001)
	peer := createLegacyOrbit(t, s, "Peer", 1002)
	third := createLegacyOrbit(t, s, "Third", 1003)
	authority, err := s.AirAuthority()
	if err != nil || authority.Mode != "links_authoritative" || authority.Generation != 1 {
		t.Fatalf("fresh authority=%+v err=%v", authority, err)
	}
	authority, err = s.CutoverLinksToAirs(authority.Generation, 100)
	if err != nil || authority.Mode != "airs_authoritative" || authority.Generation != 2 {
		t.Fatalf("cutover authority=%+v err=%v", authority, err)
	}

	air, err := s.CreateAir(CreateAirParams{Title: "  Family  ", OwnerOrbitID: owner, CreatedAt: 200})
	if err != nil || air.Title != "Family" || air.Status != "parked" || air.OwnerOrbitID != owner {
		t.Fatalf("created Air=%+v err=%v", air, err)
	}
	policy, err := s.AirPolicy(air.ID)
	if err != nil || policy.Invite != "air_admin_primary" || policy.Overlay != "primary_companion" ||
		policy.Queue != "primary_companion" || policy.Replace != "air_admin_primary" {
		t.Fatalf("default policy=%+v err=%v", policy, err)
	}
	if err := s.ActivateAir(owner, air.ID, "none", 210); err != nil {
		t.Fatal(err)
	}
	if current, _, ok, err := s.ActiveAirForOrbit(owner); err != nil || !ok || current != air.ID {
		t.Fatalf("owner current=%q ok=%v err=%v", current, ok, err)
	}
	if air, _ = s.AirByID(air.ID); air.Status != "parked" {
		t.Fatalf("one-pointer Air status=%q want parked", air.Status)
	}

	pending, err := s.AddPendingAirMember(air.ID, peer, "member", 220)
	if err != nil || pending.Status != "pending_confirmation" || pending.Revision != 1 {
		t.Fatalf("pending=%+v err=%v", pending, err)
	}
	if err := s.ConfirmAirMember(pending.ID, pending.Revision, true, "none", 230); err != nil {
		t.Fatal(err)
	}
	if air, _ = s.AirByID(air.ID); air.Status != "active" {
		t.Fatalf("two-pointer Air status=%q want active", air.Status)
	}

	second, err := s.CreateAir(CreateAirParams{Title: "Saved second", OwnerOrbitID: third, CreatedAt: 240})
	if err != nil {
		t.Fatal(err)
	}
	secondPending, err := s.AddPendingAirMember(second.ID, peer, "member", 250)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ConfirmAirMember(secondPending.ID, 1, true, air.ID, 260); err != nil {
		t.Fatal(err)
	}
	if current, revision, ok, err := s.ActiveAirForOrbit(peer); err != nil || !ok || current != second.ID || revision != 2 {
		t.Fatalf("peer switched current=%q revision=%d ok=%v err=%v", current, revision, ok, err)
	}
	if _, err := s.db.Exec(`INSERT INTO air_active_pointers(orbit_id, air_id, revision, activated_at)
VALUES(?, ?, 3, 261)`, peer, air.ID); err == nil {
		t.Fatal("database accepted a second active Air for one orbit")
	}
	if _, err := s.db.Exec(`INSERT INTO air_members(
  public_id, air_id, orbit_id, air_role, status, revision, joined_at, created_at
) VALUES(?, ?, ?, 'member', 'joined', 1, 261, 261)`,
		"aim_"+strings.Repeat("0", 26), second.ID, peer); err == nil {
		t.Fatal("database accepted a second live membership for one Air/orbit")
	}
	if air, _ = s.AirByID(air.ID); air.Status != "parked" {
		t.Fatalf("old Air after switch status=%q want parked", air.Status)
	}
	if err := s.ActivateAir(peer, air.ID, "none", 261); !errors.Is(err, ErrAirActiveChanged) {
		t.Fatalf("stale activation error=%v", err)
	}

	members, err := s.AirMembers(second.ID)
	if err != nil || len(members) != 2 {
		t.Fatalf("second members=%+v err=%v", members, err)
	}
	var ownerMember, peerMember AirMember
	for _, member := range members {
		if member.Role == "owner" {
			ownerMember = member
		} else if member.OrbitID == peer {
			peerMember = member
		}
	}
	if err := s.LeaveAirMember(ownerMember.ID, ownerMember.Revision, 270); !errors.Is(err, ErrAirOwnerLeave) {
		t.Fatalf("owner leave error=%v", err)
	}
	if err := s.LeaveAirMember(peerMember.ID, 2, 271); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, err := s.ActiveAirForOrbit(peer); err != nil || ok {
		t.Fatalf("left peer pointer ok=%v err=%v", ok, err)
	}

	policy.Invite = "owner_primary"
	if err := s.ReplaceAirPolicy(*policy, policy.Revision, 280); err != nil {
		t.Fatal(err)
	}
	if err := s.ReplaceAirPolicy(*policy, policy.Revision, 281); !errors.Is(err, ErrAirRevision) {
		t.Fatalf("stale policy error=%v", err)
	}
	if err := s.InsertAirInvite("", air.ID, strings.Repeat("a", 64), "member", 10, owner, 2, 1180, 290); err != nil {
		t.Fatal(err)
	}
	if invite, err := s.AirInviteByCodeHash(strings.Repeat("a", 64)); err != nil ||
		invite.Status != "open" || invite.PolicyRevision != 2 || invite.IssuedByOrbitID != owner {
		t.Fatalf("stored invite=%+v err=%v", invite, err)
	}
	var audits int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM air_audit_events`).Scan(&audits); err != nil || audits < 9 {
		t.Fatalf("audit rows=%d err=%v", audits, err)
	}
	assertDatabaseHealthy(t, s)
}

func TestActiveLinkBackfillIsStableExactlyOnceAndReversible(t *testing.T) {
	path := filepath.Join(t.TempDir(), "air-link-backfill.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a := createLegacyOrbit(t, s, "A", 2001)
	b := createLegacyOrbit(t, s, "B", 2002)
	linkID := createActiveLegacyLink(t, s, a, b, 2001)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s = reopenAirStore(t, path)
	authority, err := s.AirAuthority()
	if err != nil || authority.Mode != "airs_shadow" || authority.Generation != 1 {
		t.Fatalf("shadow authority=%+v err=%v", authority, err)
	}
	if _, err := s.CreateAir(CreateAirParams{
		Title: "Unavailable before cutover", OwnerOrbitID: a, CreatedAt: 250,
	}); !errors.Is(err, ErrAirRoomsDisabled) {
		t.Fatalf("shadow Air creation error=%v", err)
	}
	var airID string
	if err := s.db.QueryRow(`SELECT air_id FROM air_legacy_link_mappings WHERE link_id = ?`, linkID).Scan(&airID); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(airID, "air_") || len(airID) != 30 {
		t.Fatalf("mapped Air id=%q", airID)
	}
	members, err := s.AirMembers(airID)
	if err != nil || len(members) != 2 {
		t.Fatalf("backfilled members=%+v err=%v", members, err)
	}
	roles := map[int64]string{}
	for _, member := range members {
		roles[member.OrbitID] = member.Role
	}
	if roles[a] != "owner" || roles[b] != "member" {
		t.Fatalf("backfilled member roles=%v", roles)
	}
	if current, _, ok, err := s.ActiveAirForOrbit(a); err != nil || ok || current != "" {
		t.Fatalf("shadow must not own runtime current=%q ok=%v err=%v", current, ok, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s = reopenAirStore(t, path)
	var airIDAgain string
	var airRows, memberRows, mappingRows int
	if err := s.db.QueryRow(`SELECT
  (SELECT air_id FROM air_legacy_link_mappings WHERE link_id = ?),
  (SELECT COUNT(*) FROM airs),
  (SELECT COUNT(*) FROM air_members),
  (SELECT COUNT(*) FROM air_legacy_link_mappings)`, linkID).Scan(&airIDAgain, &airRows, &memberRows, &mappingRows); err != nil {
		t.Fatal(err)
	}
	if airIDAgain != airID || airRows != 1 || memberRows != 2 || mappingRows != 1 {
		t.Fatalf("restart backfill id=%q rows=%d/%d/%d", airIDAgain, airRows, memberRows, mappingRows)
	}
	authority, err = s.CutoverLinksToAirs(1, 300)
	if err != nil || authority.Mode != "airs_authoritative" || authority.Generation != 2 {
		t.Fatalf("cutover=%+v err=%v", authority, err)
	}
	for _, orbitID := range []int64{a, b} {
		if current, _, ok, err := s.ActiveAirForOrbit(orbitID); err != nil || !ok || current != airID {
			t.Fatalf("cutover pointer orbit=%d current=%q ok=%v err=%v", orbitID, current, ok, err)
		}
	}
	authority, err = s.RollbackAirsToLinks(2, 400)
	if err != nil || authority.Mode != "links_authoritative" || authority.Generation != 3 {
		t.Fatalf("rollback=%+v err=%v", authority, err)
	}
	var pointers int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM air_active_pointers`).Scan(&pointers); err != nil || pointers != 0 {
		t.Fatalf("rollback pointers=%d err=%v", pointers, err)
	}
	if air, err := s.AirByID(airID); err != nil || air.Status != "parked" {
		t.Fatalf("preserved Air=%+v err=%v", air, err)
	}
	if _, _, ok, err := s.ActiveLink(a); err != nil || !ok {
		t.Fatalf("legacy link after rollback ok=%v err=%v", ok, err)
	}
}

func TestConcurrentAirLifecycleChangesHaveOneTransactionalWinner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "air-concurrent-lifecycle.db")
	first, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	target := createLegacyOrbit(t, first, "Target", 2501)
	other := createLegacyOrbit(t, first, "Other", 2502)
	if _, err := first.CutoverLinksToAirs(1, 100); err != nil {
		t.Fatal(err)
	}
	current, err := first.CreateAir(CreateAirParams{Title: "Current", OwnerOrbitID: target, CreatedAt: 110})
	if err != nil {
		t.Fatal(err)
	}
	next, err := first.CreateAir(CreateAirParams{Title: "Next", OwnerOrbitID: other, CreatedAt: 120})
	if err != nil {
		t.Fatal(err)
	}
	pending, err := first.AddPendingAirMember(next.ID, target, "member", 130)
	if err != nil {
		t.Fatal(err)
	}
	if err := first.ConfirmAirMember(pending.ID, pending.Revision, false, "none", 140); err != nil {
		t.Fatal(err)
	}
	if err := first.ActivateAir(target, current.ID, "none", 150); err != nil {
		t.Fatal(err)
	}
	second, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	defer second.Close()

	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		results <- first.ActivateAir(target, next.ID, current.ID, 160)
	}()
	go func() {
		defer wg.Done()
		<-start
		results <- second.DeactivateAir(target, current.ID, 161)
	}()
	close(start)
	wg.Wait()
	close(results)
	successes, conflicts := 0, 0
	for result := range results {
		switch {
		case result == nil:
			successes++
		case errors.Is(result, ErrAirActiveChanged):
			conflicts++
		default:
			t.Fatalf("concurrent lifecycle result=%v", result)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent lifecycle successes=%d conflicts=%d", successes, conflicts)
	}
	var pointers int
	if err := first.db.QueryRow(`SELECT COUNT(*) FROM air_active_pointers WHERE orbit_id = ?`, target).Scan(&pointers); err != nil || pointers > 1 {
		t.Fatalf("concurrent lifecycle pointers=%d err=%v", pointers, err)
	}
	assertDatabaseHealthy(t, first)
}

func TestAirRuntimeResolutionUsesOnlyCurrentMembersAndStableSnapshotKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "air-runtime-resolution.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	owner := createLegacyOrbit(t, s, "Owner", 2701)
	peer := createLegacyOrbit(t, s, "Peer", 2702)
	savedOnly := createLegacyOrbit(t, s, "Saved only", 2703)
	if _, err := s.CutoverLinksToAirs(1, 100); err != nil {
		t.Fatal(err)
	}
	air, err := s.CreateAir(CreateAirParams{Title: "Living", OwnerOrbitID: owner, CreatedAt: 110})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ActivateAir(owner, air.ID, "none", 120); err != nil {
		t.Fatal(err)
	}
	for index, orbitID := range []int64{peer, savedOnly} {
		member, err := s.AddPendingAirMember(air.ID, orbitID, "member", int64(130+index))
		if err != nil {
			t.Fatal(err)
		}
		if err := s.ConfirmAirMember(member.ID, 1, orbitID == peer, "none", int64(140+index)); err != nil {
			t.Fatal(err)
		}
	}
	runtime, err := s.ActiveAirRuntimeForOrbit(owner)
	if err != nil || runtime == nil || runtime.AirID != air.ID ||
		fmt.Sprint(runtime.OrbitIDs) != fmt.Sprint([]int64{owner, peer}) {
		t.Fatalf("runtime=%+v err=%v", runtime, err)
	}
	if runtime, err := s.ActiveAirRuntimeForOrbit(savedOnly); err != nil || runtime != nil {
		t.Fatalf("saved-only runtime=%+v err=%v", runtime, err)
	}
	runtimes, err := s.ActiveAirRuntimes()
	if err != nil || len(runtimes) != 1 || runtimes[0].AirID != air.ID {
		t.Fatalf("active runtimes=%+v err=%v", runtimes, err)
	}
	snapshot := SessionSnapshot{Mode: session.ModeShared, State: session.StatePlaying,
		Current: &session.Element{ID: "air-track", Kind: session.KindTrack}}
	if err := s.SaveAirSession(air.ID, snapshot); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = reopenAirStore(t, path)
	restored, err := s.LoadAirSession(air.ID)
	if err != nil || restored == nil || restored.State != session.StatePaused || restored.Current.ID != "air-track" {
		t.Fatalf("restored Air snapshot=%+v err=%v", restored, err)
	}
	if err := s.DeactivateAir(peer, air.ID, 200); err != nil {
		t.Fatal(err)
	}
	if runtimes, err := s.ActiveAirRuntimes(); err != nil || len(runtimes) != 0 {
		t.Fatalf("parked runtimes=%+v err=%v", runtimes, err)
	}
}

func TestAirRuntimeSwitchAdvancesBothOwnershipRevisions(t *testing.T) {
	s := openTemp(t)
	ownerA := createLegacyOrbit(t, s, "Owner A", 2751)
	ownerB := createLegacyOrbit(t, s, "Owner B", 2752)
	switching := createLegacyOrbit(t, s, "Switching", 2753)
	if _, err := s.CutoverLinksToAirs(1, 100); err != nil {
		t.Fatal(err)
	}
	airA, err := s.CreateAir(CreateAirParams{Title: "Air A", OwnerOrbitID: ownerA, CreatedAt: 110})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ActivateAir(ownerA, airA.ID, "none", 120); err != nil {
		t.Fatal(err)
	}
	memberA, err := s.AddPendingAirMember(airA.ID, switching, "member", 130)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ConfirmAirMember(memberA.ID, 1, true, "none", 140); err != nil {
		t.Fatal(err)
	}
	airB, err := s.CreateAir(CreateAirParams{Title: "Air B", OwnerOrbitID: ownerB, CreatedAt: 150})
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ActivateAir(ownerB, airB.ID, "none", 160); err != nil {
		t.Fatal(err)
	}
	memberB, err := s.AddPendingAirMember(airB.ID, switching, "member", 170)
	if err != nil {
		t.Fatal(err)
	}
	beforeA, err := s.AirByID(airA.ID)
	if err != nil {
		t.Fatal(err)
	}
	beforeB, err := s.AirByID(airB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ConfirmAirMember(memberB.ID, 1, true, airA.ID, 180); err != nil {
		t.Fatal(err)
	}
	afterA, err := s.AirByID(airA.ID)
	if err != nil {
		t.Fatal(err)
	}
	afterB, err := s.AirByID(airB.ID)
	if err != nil {
		t.Fatal(err)
	}
	if afterA.Revision <= beforeA.Revision || afterB.Revision <= beforeB.Revision {
		t.Fatalf("ownership switch revisions A=%d->%d B=%d->%d", beforeA.Revision,
			afterA.Revision, beforeB.Revision, afterB.Revision)
	}
	if runtime, err := s.ActiveAirRuntimeByID(airA.ID); err != nil || runtime != nil {
		t.Fatalf("old Air runtime=%+v err=%v", runtime, err)
	}
	runtime, err := s.ActiveAirRuntimeByID(airB.ID)
	if err != nil || runtime == nil || fmt.Sprint(runtime.OrbitIDs) != fmt.Sprint([]int64{ownerB, switching}) {
		t.Fatalf("new Air runtime=%+v err=%v", runtime, err)
	}
}

func TestAirBackfillFailureRollsBackDDLAndDeterministicallyReruns(t *testing.T) {
	path := filepath.Join(t.TempDir(), "air-backfill-failure.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a := createLegacyOrbit(t, s, "A", 3001)
	b := createLegacyOrbit(t, s, "B", 3002)
	linkID := createActiveLegacyLink(t, s, a, b, 3001)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	dropAirSchemaForLegacyFixture(t, path)

	wantErr := errors.New("backfill fault")
	if failed, err := openWithOptionsAndCheckpoint(path, Options{}, func(name string) error {
		if name == "air_backfill_after_first_link" {
			return wantErr
		}
		return nil
	}); !errors.Is(err, wantErr) || failed != nil {
		t.Fatalf("failed migration store=%v err=%v", failed, err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	var airTables int
	if err := db.QueryRow(`SELECT COUNT(*) FROM sqlite_master
WHERE type = 'table' AND name = 'airs'`).Scan(&airTables); err != nil || airTables != 0 {
		t.Fatalf("failed migration Air tables=%d err=%v", airTables, err)
	}
	_ = db.Close()

	s = reopenAirStore(t, path)
	var mapped string
	if err := s.db.QueryRow(`SELECT air_id FROM air_legacy_link_mappings WHERE link_id = ?`, linkID).Scan(&mapped); err != nil {
		t.Fatal(err)
	}
	link := struct{ id, a, b, createdAt int64 }{id: linkID, a: a, b: b}
	if err := s.db.QueryRow(`SELECT created_at FROM links WHERE id = ?`, linkID).Scan(&link.createdAt); err != nil {
		t.Fatal(err)
	}
	if want := deterministicAirMigrationID("air", link); mapped != want {
		t.Fatalf("deterministic mapping=%q want=%q", mapped, want)
	}
}

func TestAirProductionShapedCutoverFailureAndLegacyRollbackPreserveRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "air-production-shaped.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	var orbits []int64
	for i := 0; i < 8; i++ {
		orbits = append(orbits, createLegacyOrbit(t, s, fmt.Sprintf("Home %d", i+1), int64(4000+i)))
	}
	for i := 0; i < 8; i += 2 {
		createActiveLegacyLink(t, s, orbits[i], orbits[i+1], int64(4000+i))
	}
	if err := s.SetSetting("session_state_legacy_fixture", `{"preserved":true}`); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	wantErr := errors.New("cutover fault")
	if failed, err := openWithOptionsAndCheckpoint(path, Options{}, func(name string) error {
		if name == "air_cutover_before_authority_flip" {
			return wantErr
		}
		return nil
	}); err != nil || failed == nil {
		t.Fatalf("open production fixture store=%v err=%v", failed, err)
	} else {
		_, cutoverErr := failed.CutoverLinksToAirs(1, 500)
		if !errors.Is(cutoverErr, wantErr) {
			t.Fatalf("injected cutover error=%v", cutoverErr)
		}
		var pointers int
		authority, authorityErr := failed.AirAuthority()
		if err := failed.db.QueryRow(`SELECT COUNT(*) FROM air_active_pointers`).Scan(&pointers); err != nil ||
			authorityErr != nil || pointers != 0 || authority.Mode != "airs_shadow" || authority.Generation != 1 {
			t.Fatalf("atomic failed cutover pointers=%d authority=%+v errors=%v/%v", pointers, authority, err, authorityErr)
		}
		_ = failed.Close()
	}

	s = reopenAirStore(t, path)
	authority, err := s.CutoverLinksToAirs(1, 600)
	if err != nil || authority.Generation != 2 {
		t.Fatalf("cutover retry=%+v err=%v", authority, err)
	}
	var airs, members, pointers int
	if err := s.db.QueryRow(`SELECT
  (SELECT COUNT(*) FROM airs),
  (SELECT COUNT(*) FROM air_members),
  (SELECT COUNT(*) FROM air_active_pointers)`).Scan(&airs, &members, &pointers); err != nil ||
		airs != 4 || members != 8 || pointers != 8 {
		t.Fatalf("production rows airs=%d members=%d pointers=%d err=%v", airs, members, pointers, err)
	}
	authority, err = s.RollbackAirsToLinks(2, 700)
	if err != nil || authority.Mode != "links_authoritative" {
		t.Fatalf("production rollback=%+v err=%v", authority, err)
	}
	if setting, err := s.GetSetting("session_state_legacy_fixture"); err != nil || setting != `{"preserved":true}` {
		t.Fatalf("legacy setting=%q err=%v", setting, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// This SQL is intentionally limited to the tables and write shape known by
	// the previous coordinator. Unknown Phase 2 tables remain untouched while
	// links_authoritative is the persisted deployment precondition.
	db, err := sql.Open("sqlite", path+"?_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA foreign_keys = ON`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE links SET code = '' WHERE state = 'active'`); err != nil {
		t.Fatalf("previous coordinator legacy write: %v", err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	s = reopenAirStore(t, path)
	if err := s.db.QueryRow(`SELECT
  (SELECT COUNT(*) FROM airs),
  (SELECT COUNT(*) FROM air_members),
  (SELECT COUNT(*) FROM air_legacy_link_mappings)`).Scan(&airs, &members, &pointers); err != nil ||
		airs != 4 || members != 8 || pointers != 4 {
		t.Fatalf("previous binary preserved rows airs=%d members=%d mappings=%d err=%v", airs, members, pointers, err)
	}
	assertDatabaseHealthy(t, s)
}

func TestUnsafeAirRollbackEntersHoldAndNeverEnablesLinks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "air-unsafe-rollback.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	orbit := createLegacyOrbit(t, s, "Owner", 5001)
	authority, err := s.CutoverLinksToAirs(1, 100)
	if err != nil {
		t.Fatal(err)
	}
	air, err := s.CreateAir(CreateAirParams{Title: "Air-only", OwnerOrbitID: orbit, CreatedAt: 110})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.RollbackAirsToLinks(authority.Generation, 120); !errors.Is(err, ErrAirRollbackUnsafe) {
		t.Fatalf("unsafe rollback error=%v", err)
	}
	authority, err = s.AirAuthority()
	if err != nil || authority.Mode != "rollback_hold" || authority.Generation != 3 || authority.DivergenceCount == 0 {
		t.Fatalf("rollback hold=%+v err=%v", authority, err)
	}
	if preserved, err := s.AirByID(air.ID); err != nil || preserved.Status != "parked" {
		t.Fatalf("Air-only row=%+v err=%v", preserved, err)
	}
	if _, _, ok, err := s.ActiveLink(orbit); err != nil || ok {
		t.Fatalf("unsafe rollback resurrected link ok=%v err=%v", ok, err)
	}
}

func TestAirStartupDetectsLegacyMutationAfterCutoverAndPersistsHold(t *testing.T) {
	path := filepath.Join(t.TempDir(), "air-post-cutover-legacy-write.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a := createLegacyOrbit(t, s, "A", 5501)
	b := createLegacyOrbit(t, s, "B", 5502)
	linkID := createActiveLegacyLink(t, s, a, b, 5501)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	s = reopenAirStore(t, path)
	if _, err := s.CutoverLinksToAirs(1, 200); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", path+"?_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`DELETE FROM links WHERE id = ?`, linkID); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if reopened, err := Open(path); reopened != nil || !errors.Is(err, ErrAirRollbackUnsafe) {
		t.Fatalf("startup after legacy mutation store=%v err=%v", reopened, err)
	}
	db, err = sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var mode string
	var generation, airs, pointers int
	if err := db.QueryRow(`SELECT
  (SELECT mode FROM air_authority WHERE singleton = 1),
  (SELECT generation FROM air_authority WHERE singleton = 1),
  (SELECT COUNT(*) FROM airs),
  (SELECT COUNT(*) FROM air_active_pointers)`).Scan(&mode, &generation, &airs, &pointers); err != nil {
		t.Fatal(err)
	}
	if mode != "rollback_hold" || generation != 3 || airs != 1 || pointers != 2 {
		t.Fatalf("startup hold mode=%s generation=%d airs=%d pointers=%d", mode, generation, airs, pointers)
	}
}

func TestConflictingLegacyLinksAbortBackfillWithoutPartialRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "air-conflicting-links.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	a := createLegacyOrbit(t, s, "A", 6001)
	b := createLegacyOrbit(t, s, "B", 6002)
	c := createLegacyOrbit(t, s, "C", 6003)
	createActiveLegacyLink(t, s, a, b, 6001)
	if _, err := s.db.Exec(`INSERT INTO links(
  orbit_a, orbit_b, state, proposed_by, pending_orbit, code, created_at
) VALUES(?, ?, 'active', ?, 0, '', 999)`, a, c, 6001); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}
	if reopened, err := Open(path); err == nil || reopened != nil || !strings.Contains(err.Error(), "has active links") {
		t.Fatalf("conflicting link open=%v err=%v", reopened, err)
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	var mappings int
	if err := db.QueryRow(`SELECT COUNT(*) FROM air_legacy_link_mappings`).Scan(&mappings); err != nil || mappings != 0 {
		t.Fatalf("partial conflicting mappings=%d err=%v", mappings, err)
	}
}

func TestAirMigrationHandoffKeepsSingleAuthorityAndRollbackWarnings(t *testing.T) {
	repositoryRoot := filepath.Join("..", "..", "..")
	raw, err := os.ReadFile(filepath.Join(repositoryRoot, "docs", "analysis", "p2-air-schema-link-migration.md"))
	if err != nil {
		t.Fatal(err)
	}
	document := string(raw)
	for _, decision := range []string{
		"one-active invariant",
		"SHA-256-derived",
		"Backfill does not create active pointers",
		"zero divergence",
		"commits `rollback_hold`",
		"An Air-unaware coordinator may be started only while",
		"forbidden to deploy the older binary",
		"Never edit pointer, mapping, membership or authority rows by hand",
		"exact predecessor coordinator binary",
	} {
		if !strings.Contains(document, decision) {
			t.Errorf("Air migration handoff lost %q", decision)
		}
	}
	runbook, err := os.ReadFile(filepath.Join(repositoryRoot, "docs", "runbook.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(runbook), "p2-air-schema-link-migration.md") {
		t.Fatal("runbook lost Air migration handoff entry point")
	}
}

func dropAirSchemaForLegacyFixture(t *testing.T, path string) {
	t.Helper()
	db, err := sql.Open("sqlite", path+"?_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{
		"air_audit_events", "air_invites", "air_active_pointers", "air_legacy_runtime_snapshots", "air_legacy_link_mappings",
		"air_policies", "air_members", "airs", "air_authority",
	} {
		if _, err := db.Exec(`DROP TABLE IF EXISTS ` + table); err != nil {
			t.Fatalf("drop %s: %v", table, err)
		}
	}
}
