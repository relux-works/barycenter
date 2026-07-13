package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
)

// R8: a handcrafted pre-feature database spans multiple tenants, every role,
// live and revoked slots, and real legacy token hashes. Migration may project
// identity, but must not rewrite any legacy authority row.
func TestR8MultiOrbitLegacyBackfillPreservesRolesSlotsAndTokens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "multi-orbit-legacy.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(orbitSchema); err != nil {
		t.Fatal(err)
	}
	tokens := map[string]string{
		"primary":   randomHex(32),
		"companion": randomHex(32),
		"satellite": randomHex(32),
		"second":    randomHex(32),
		"revoked":   randomHex(32),
	}
	if _, err := db.Exec(`INSERT INTO orbits(id, title, max_pulsars, max_members, created_at)
VALUES(10, 'Legacy one', 5, 10, 1), (20, 'Legacy two', 5, 10, 2);
INSERT INTO members(orbit_id, tg_user_id, role, joined_at, display_name) VALUES
  (10, 101, 'primary', 1, 'Primary'),
  (10, 102, 'companion', 2, 'Companion'),
  (10, 103, 'satellite', 3, 'Satellite'),
  (20, 201, 'primary', 4, 'Second');
INSERT INTO slots(orbit_id, slot, token_hash, paired_by, paired_at, revoked_at) VALUES
  (10, 'a', ?, 101, 10, NULL),
  (10, 'b', ?, 102, 11, NULL),
  (10, 'c', ?, 103, 12, NULL),
  (20, 'a', ?, 201, 13, NULL),
  (20, 'b', ?, 201, 14, 15)`,
		hashToken(tokens["primary"]), hashToken(tokens["companion"]),
		hashToken(tokens["satellite"]), hashToken(tokens["second"]),
		hashToken(tokens["revoked"])); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for _, tc := range []struct {
		name    string
		orbitID int64
		slot    string
		role    string
	}{
		{name: "primary", orbitID: 10, slot: "a", role: "primary"},
		{name: "companion", orbitID: 10, slot: "b", role: "companion"},
		{name: "satellite", orbitID: 10, slot: "c", role: "companion"},
		{name: "second", orbitID: 20, slot: "a", role: "primary"},
	} {
		ctx, err := s.ResolveTokenActorContext(tokens[tc.name])
		if err != nil || ctx.OrbitID != tc.orbitID || ctx.Slot != tc.slot || ctx.Role != tc.role || ctx.Capabilities != CapabilityNode {
			t.Fatalf("%s context=%+v err=%v", tc.name, ctx, err)
		}
		if gotOrbit, gotSlot, ok, err := s.LookupToken(tokens[tc.name]); err != nil || !ok || gotOrbit != tc.orbitID || gotSlot != tc.slot {
			t.Fatalf("%s legacy lookup orbit=%d slot=%s ok=%v err=%v", tc.name, gotOrbit, gotSlot, ok, err)
		}
	}
	if _, _, ok, err := s.LookupToken(tokens["revoked"]); err != nil || ok {
		t.Fatalf("revoked legacy token ok=%v err=%v", ok, err)
	}
	var revokedCredentials int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM installation_credentials WHERE slot_orbit_id = 20 AND slot_name = 'b'`).Scan(&revokedCredentials); err != nil || revokedCredentials != 0 {
		t.Fatalf("revoked slot credential count=%d err=%v", revokedCredentials, err)
	}
	for _, tc := range []struct {
		id   int64
		role string
	}{
		{id: 101, role: "primary"}, {id: 102, role: "companion"},
		{id: 103, role: "satellite"}, {id: 201, role: "primary"},
	} {
		var role string
		if err := s.db.QueryRow(`SELECT role FROM members WHERE tg_user_id = ?`, tc.id).Scan(&role); err != nil || role != tc.role {
			t.Fatalf("legacy member %d role=%q err=%v", tc.id, role, err)
		}
	}
	assertDatabaseHealthy(t, s)
}

// R8: exercise every previous-coordinator mutation against legacy authority,
// then re-enable and verify the additive projection. This deliberately names
// itself emulation; a real archived previous binary remains a separate gate.
func TestR8FullPreviousAuthorityMutationEmulationReconciles(t *testing.T) {
	path := filepath.Join(t.TempDir(), "full-rollback-emulation.db")
	s, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	keep, _ := s.CreateOrbit("Keep", 101)
	if err := s.AddMember(keep.ID, 202, "companion"); err != nil {
		t.Fatal(err)
	}
	_, oldAContext := provisionTestInstallation(t, s, keep.ID, 101)
	_, oldBToken, err := s.PairSlot(keep.ID, 202)
	if err != nil {
		t.Fatal(err)
	}
	oldBContext, err := s.ResolveTokenActorContext(oldBToken)
	if err != nil {
		t.Fatal(err)
	}
	controlA, recoveryIDA, recoverySecretA := newProvisioningMaterial(t)
	if err := s.ProvisionInstallationSecrets(Identity{Kind: IdentityTelegram, TelegramUserID: 101},
		oldAContext.ActorID, controlA, recoveryIDA, recoverySecretA); err != nil {
		t.Fatal(err)
	}
	dissolve, _ := s.CreateOrbit("Dissolve", 404)
	_, dissolveToken, err := s.PairSlot(dissolve.ID, 404)
	if err != nil {
		t.Fatal(err)
	}
	dissolveContext, err := s.ResolveTokenActorContext(dissolveToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	legacy, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	newBToken := randomHex(32)
	newCToken := randomHex(32)
	statements := []struct {
		query string
		args  []any
	}{
		{`INSERT INTO members(orbit_id, tg_user_id, role, joined_at, display_name) VALUES(?, 303, 'companion', 30, '')`, []any{keep.ID}},
		{`UPDATE members SET display_name = 'Legacy Renamed' WHERE orbit_id = ? AND tg_user_id = 303`, []any{keep.ID}},
		{`UPDATE members SET role = CASE tg_user_id WHEN 101 THEN 'companion' WHEN 303 THEN 'primary' ELSE role END WHERE orbit_id = ?`, []any{keep.ID}},
		{`DELETE FROM members WHERE orbit_id = ? AND tg_user_id = 202`, []any{keep.ID}},
		{`UPDATE slots SET revoked_at = 31 WHERE orbit_id = ? AND paired_by = 202 AND revoked_at IS NULL`, []any{keep.ID}},
		{`UPDATE slots SET revoked_at = 32 WHERE orbit_id = ? AND slot = 'a' AND revoked_at IS NULL`, []any{keep.ID}},
		{`INSERT OR REPLACE INTO slots(orbit_id, slot, token_hash, paired_by, provider, paired_at, revoked_at) VALUES(?, 'b', ?, 303, 'spotify', 33, NULL)`, []any{keep.ID, hashToken(newBToken)}},
		{`INSERT INTO slots(orbit_id, slot, token_hash, paired_by, provider, paired_at, revoked_at) VALUES(?, 'c', ?, 303, 'spotify', 34, NULL)`, []any{keep.ID, hashToken(newCToken)}},
	}
	for _, statement := range statements {
		if _, err := legacy.Exec(statement.query, statement.args...); err != nil {
			legacy.Close()
			t.Fatalf("legacy mutation failed: %v", err)
		}
	}
	for _, statement := range []struct {
		query string
		args  []any
	}{
		{`DELETE FROM members WHERE orbit_id = ?`, []any{dissolve.ID}},
		{`DELETE FROM slots WHERE orbit_id = ?`, []any{dissolve.ID}},
		{`DELETE FROM invites WHERE orbit_id = ?`, []any{dissolve.ID}},
		{`DELETE FROM availability WHERE orbit_id = ?`, []any{dissolve.ID}},
		{`DELETE FROM links WHERE orbit_a = ? OR orbit_b = ?`, []any{dissolve.ID, dissolve.ID}},
		{`DELETE FROM orbits WHERE id = ?`, []any{dissolve.ID}},
	} {
		if _, err := legacy.Exec(statement.query, statement.args...); err != nil {
			legacy.Close()
			t.Fatal(err)
		}
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	newB, err := s.ResolveTokenActorContext(newBToken)
	if err != nil || newB.Role != "primary" || newB.ActorID == oldBContext.ActorID {
		t.Fatalf("rebound B context=%+v old=%d err=%v", newB, oldBContext.ActorID, err)
	}
	newC, err := s.ResolveTokenActorContext(newCToken)
	if err != nil || newC.Role != "primary" || newC.Slot != "c" {
		t.Fatalf("new C context=%+v err=%v", newC, err)
	}
	if _, err := s.ResolveTokenActorContext(controlA); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("control credential for revoked A error=%v", err)
	}
	for _, oldToken := range []string{oldBToken, dissolveToken} {
		if _, _, ok, err := s.LookupToken(oldToken); err != nil || ok {
			t.Fatalf("retired legacy token ok=%v err=%v", ok, err)
		}
	}
	member303, err := s.ResolveTelegramActorContext(303)
	if err != nil || member303.Role != "primary" {
		t.Fatalf("added/promoted member context=%+v err=%v", member303, err)
	}
	var name string
	if err := s.db.QueryRow(`SELECT display_name FROM actors WHERE id = ?`, member303.ActorID).Scan(&name); err != nil || name != "Legacy Renamed" {
		t.Fatalf("reconciled name=%q err=%v", name, err)
	}
	member202 := telegramActorID(t, s, 202)
	assertMembership(t, s, member202, keep.ID, "companion", true)
	var dissolvedRows int
	if err := s.db.QueryRow(`SELECT
  (SELECT COUNT(*) FROM memberships WHERE orbit_id = ?) +
  (SELECT COUNT(*) FROM installation_credentials WHERE slot_orbit_id = ?)`, dissolve.ID, dissolve.ID).Scan(&dissolvedRows); err != nil || dissolvedRows != 0 {
		t.Fatalf("dissolved additive rows=%d err=%v", dissolvedRows, err)
	}
	var dissolvedRevoked sql.NullInt64
	if err := s.db.QueryRow(`SELECT revoked_at FROM actors WHERE id = ?`, dissolveContext.ActorID).Scan(&dissolvedRevoked); err != nil || !dissolvedRevoked.Valid {
		t.Fatalf("dissolved actor revoked_at=%+v err=%v", dissolvedRevoked, err)
	}
	assertDatabaseHealthy(t, s)
}

// R8: two projection generations retain each generation's current quotas,
// including a user change between cycles, and projected slots remain revoked.
func TestR8TwoRollbackProjectionGenerationsPreserveQuotaChanges(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projection-generations.db")
	s, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	orbit, _ := s.CreateOrbit("Projection generations", 101)
	if err := s.AddMember(orbit.ID, 202, "companion"); err != nil {
		t.Fatal(err)
	}
	_, firstToken, _ := s.PairSlot(orbit.ID, 101)
	if _, err := s.db.Exec(`UPDATE orbits SET status = 'disabled' WHERE id = ?`, orbit.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ProjectIdentityForLegacyRollback(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	off, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok, err := off.LookupToken(firstToken); err != nil || ok {
		t.Fatalf("cycle one projected token ok=%v err=%v", ok, err)
	}
	if _, _, err := off.LeaveOrbit(orbit.ID, 202); err != nil {
		t.Fatal(err)
	}
	if err := off.AddMember(orbit.ID, 303, "companion"); !errors.Is(err, ErrLimit) {
		t.Fatalf("cycle one blocked add error=%v", err)
	}
	if err := off.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE orbits SET status = 'active', max_pulsars = 3, max_members = 7 WHERE id = ?`, orbit.ID); err != nil {
		t.Fatal(err)
	}
	_, secondToken, err := s.PairSlot(orbit.ID, 101)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE orbits SET status = 'disabled' WHERE id = ?`, orbit.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ProjectIdentityForLegacyRollback(); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	off, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok, err := off.LookupToken(secondToken); err != nil || ok {
		t.Fatalf("cycle two projected token ok=%v err=%v", ok, err)
	}
	if err := off.Close(); err != nil {
		t.Fatal(err)
	}
	s, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var maxPulsars, maxMembers, originalPulsars, originalMembers int
	var restored sql.NullInt64
	if err := s.db.QueryRow(`SELECT max_pulsars, max_members FROM orbits WHERE id = ?`, orbit.ID).Scan(&maxPulsars, &maxMembers); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT original_max_pulsars, original_max_members, restored_at
FROM rollback_projections WHERE orbit_id = ?`, orbit.ID).Scan(&originalPulsars, &originalMembers, &restored); err != nil {
		t.Fatal(err)
	}
	if maxPulsars != 3 || maxMembers != 7 || originalPulsars != 3 || originalMembers != 7 || !restored.Valid {
		t.Fatalf("cycle two quotas current=%d/%d journal=%d/%d restored=%v", maxPulsars, maxMembers, originalPulsars, originalMembers, restored.Valid)
	}
	if _, _, ok, err := s.LookupToken(secondToken); err != nil || ok {
		t.Fatalf("cycle two projected slot was revived: ok=%v err=%v", ok, err)
	}
	assertDatabaseHealthy(t, s)
}

// R8 projection crash barrier: an interruption after journal creation but
// before quota projection rolls the entire SQLite transaction back. A retry
// creates one pending generation and applies every legacy safety projection.
func TestR8ProjectionInterruptionAfterJournalIsAtomicAndRetryable(t *testing.T) {
	s := openIdentityTemp(t)
	orbit, err := s.CreateOrbit("Interrupted projection", 101)
	if err != nil {
		t.Fatal(err)
	}
	_, nodeToken, err := s.PairSlot(orbit.ID, 101)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE orbits SET status = 'disabled' WHERE id = ?`, orbit.ID); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("injected projection interruption")
	s.testCheckpoint = func(name string) error {
		if name == "rollback_projection_after_journal" {
			return wantErr
		}
		return nil
	}
	if err := s.ProjectIdentityForLegacyRollback(); !errors.Is(err, wantErr) {
		t.Fatalf("projection interruption error=%v", err)
	}
	var journalRows, maxPulsars, maxMembers int
	var revoked sql.NullInt64
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM rollback_projections WHERE orbit_id = ?`, orbit.ID).Scan(&journalRows); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT max_pulsars, max_members FROM orbits WHERE id = ?`, orbit.ID).Scan(&maxPulsars, &maxMembers); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT revoked_at FROM slots WHERE orbit_id = ? AND slot = 'a'`, orbit.ID).Scan(&revoked); err != nil {
		t.Fatal(err)
	}
	if journalRows != 0 || maxPulsars != 5 || maxMembers != 10 || revoked.Valid {
		t.Fatalf("partial projection escaped transaction: journal=%d quota=%d/%d revoked=%v", journalRows, maxPulsars, maxMembers, revoked.Valid)
	}
	if _, _, ok, err := s.LookupToken(nodeToken); err != nil || !ok {
		t.Fatalf("rolled-back projection changed node token: ok=%v err=%v", ok, err)
	}

	s.testCheckpoint = nil
	if err := s.ProjectIdentityForLegacyRollback(); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM rollback_projections WHERE orbit_id = ? AND restored_at IS NULL`, orbit.ID).Scan(&journalRows); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT max_pulsars, max_members FROM orbits WHERE id = ?`, orbit.ID).Scan(&maxPulsars, &maxMembers); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT revoked_at FROM slots WHERE orbit_id = ? AND slot = 'a'`, orbit.ID).Scan(&revoked); err != nil {
		t.Fatal(err)
	}
	if journalRows != 1 || maxPulsars != 0 || maxMembers != 0 || !revoked.Valid {
		t.Fatalf("retry projection journal=%d quota=%d/%d revoked=%v", journalRows, maxPulsars, maxMembers, revoked.Valid)
	}
}

// R8 restoration crash barrier: quota restoration and restored_at marking are
// one transaction. An interruption between them leaves the pending projection
// intact, and the next reconciliation safely retries both operations.
func TestR8RestorationInterruptionAfterQuotaUpdateIsAtomicAndRetryable(t *testing.T) {
	s := openIdentityTemp(t)
	orbit, err := s.CreateOrbit("Interrupted restoration", 101)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE orbits SET status = 'disabled' WHERE id = ?`, orbit.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ProjectIdentityForLegacyRollback(); err != nil {
		t.Fatal(err)
	}
	wantErr := errors.New("injected restoration interruption")
	s.testCheckpoint = func(name string) error {
		if name == "rollback_restoration_after_quota_update" {
			return wantErr
		}
		return nil
	}
	if err := s.ReconcileIdentity(); !errors.Is(err, wantErr) {
		t.Fatalf("restoration interruption error=%v", err)
	}
	var maxPulsars, maxMembers int
	var restored sql.NullInt64
	if err := s.db.QueryRow(`SELECT max_pulsars, max_members FROM orbits WHERE id = ?`, orbit.ID).Scan(&maxPulsars, &maxMembers); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT restored_at FROM rollback_projections WHERE orbit_id = ?`, orbit.ID).Scan(&restored); err != nil {
		t.Fatal(err)
	}
	if maxPulsars != 0 || maxMembers != 0 || restored.Valid {
		t.Fatalf("partial restoration escaped transaction: quota=%d/%d restored=%v", maxPulsars, maxMembers, restored.Valid)
	}

	s.testCheckpoint = nil
	if err := s.ReconcileIdentity(); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT max_pulsars, max_members FROM orbits WHERE id = ?`, orbit.ID).Scan(&maxPulsars, &maxMembers); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT restored_at FROM rollback_projections WHERE orbit_id = ?`, orbit.ID).Scan(&restored); err != nil {
		t.Fatal(err)
	}
	if maxPulsars != 5 || maxMembers != 10 || !restored.Valid {
		t.Fatalf("restoration retry quota=%d/%d restored=%v", maxPulsars, maxMembers, restored.Valid)
	}
	assertDatabaseHealthy(t, s)
}

func TestR8ProjectedSlotReenableBranches(t *testing.T) {
	t.Run("unchanged_projected_slot_stays_revoked", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "unchanged.db")
		s, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
		if err != nil {
			t.Fatal(err)
		}
		orbit, err := s.CreateOrbit("Unchanged projection", 101)
		if err != nil {
			t.Fatal(err)
		}
		_, oldNode, err := s.PairSlot(orbit.ID, 101)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`UPDATE orbits SET status = 'disabled' WHERE id = ?`, orbit.ID); err != nil {
			t.Fatal(err)
		}
		if err := s.ProjectIdentityForLegacyRollback(); err != nil {
			t.Fatal(err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}

		s, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		if _, err := s.db.Exec(`UPDATE orbits SET status = 'active' WHERE id = ?`, orbit.ID); err != nil {
			t.Fatal(err)
		}
		if err := s.ReconcileIdentity(); err != nil {
			t.Fatal(err)
		}
		if _, _, ok, err := s.LookupToken(oldNode); err != nil || ok {
			t.Fatalf("unchanged projected node revived: ok=%v err=%v", ok, err)
		}
		var credentials int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM installation_credentials WHERE slot_orbit_id = ?`, orbit.ID).Scan(&credentials); err != nil || credentials != 0 {
			t.Fatalf("unchanged projected credentials=%d err=%v", credentials, err)
		}
		assertDatabaseHealthy(t, s)
	})

	t.Run("old_binary_rebind_becomes_new_generation_only_after_reenable", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "rebound.db")
		s, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
		if err != nil {
			t.Fatal(err)
		}
		orbit, err := s.CreateOrbit("Rebound projection", 101)
		if err != nil {
			t.Fatal(err)
		}
		_, oldNode, err := s.PairSlot(orbit.ID, 101)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`UPDATE orbits SET status = 'disabled' WHERE id = ?`, orbit.ID); err != nil {
			t.Fatal(err)
		}
		if err := s.ProjectIdentityForLegacyRollback(); err != nil {
			t.Fatal(err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}

		// New code first restores the recorded quotas while keeping the orbit
		// disabled. The previous coordinator then reuses the revoked slot.
		s, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
		if err != nil {
			t.Fatal(err)
		}
		if err := s.Close(); err != nil {
			t.Fatal(err)
		}
		legacy, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		slot, reboundNode, err := legacy.PairSlot(orbit.ID, 101)
		if err != nil || slot != "a" {
			legacy.Close()
			t.Fatalf("old binary rebound slot=%q err=%v", slot, err)
		}
		if err := legacy.Close(); err != nil {
			t.Fatal(err)
		}

		s, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		if _, err := s.ResolveTokenActorContext(reboundNode); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("disabled rebound resolver error=%v", err)
		}
		if _, err := s.db.Exec(`UPDATE orbits SET status = 'active' WHERE id = ?`, orbit.ID); err != nil {
			t.Fatal(err)
		}
		if err := s.ReconcileIdentity(); err != nil {
			t.Fatal(err)
		}
		ctx, err := s.ResolveTokenActorContext(reboundNode)
		if err != nil || ctx.OrbitID != orbit.ID || ctx.Slot != "a" || ctx.ActorID == 0 {
			t.Fatalf("rebound generation context=%+v err=%v", ctx, err)
		}
		if _, _, ok, err := s.LookupToken(oldNode); err != nil || ok {
			t.Fatalf("old projected generation revived: ok=%v err=%v", ok, err)
		}
		assertDatabaseHealthy(t, s)
	})
}

// R8: if an emergency rollback skips projection, the old node-only path has
// the acknowledged gap; re-enabling new code rejects the gap token until the
// disabled orbit is explicitly re-enabled and reconciled.
func TestR8EmergencyRollbackGapIsContainedOnReenable(t *testing.T) {
	path := filepath.Join(t.TempDir(), "emergency-gap.db")
	s, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	orbit, _ := s.CreateOrbit("Emergency gap", 101)
	if _, err := s.db.Exec(`UPDATE orbits SET status = 'disabled' WHERE id = ?`, orbit.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	off, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, gapToken, err := off.PairSlot(orbit.ID, 101)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok, err := off.LookupToken(gapToken); err != nil || !ok {
		t.Fatalf("emergency legacy gap not reproduced: ok=%v err=%v", ok, err)
	}
	if err := off.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, _, ok, err := s.LookupPlaybackToken(gapToken); err != nil || ok {
		t.Fatalf("new-code status-aware gap lookup ok=%v err=%v", ok, err)
	}
	var credentials int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM installation_credentials WHERE slot_orbit_id = ?`, orbit.ID).Scan(&credentials); err != nil || credentials != 0 {
		t.Fatalf("disabled gap credentials=%d err=%v", credentials, err)
	}
	if _, err := s.db.Exec(`UPDATE orbits SET status = 'active' WHERE id = ?`, orbit.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ReconcileIdentity(); err != nil {
		t.Fatal(err)
	}
	ctx, err := s.ResolveTokenActorContext(gapToken)
	if err != nil || ctx.OrbitID != orbit.ID {
		t.Fatalf("re-enabled gap context=%+v err=%v", ctx, err)
	}
	assertDatabaseHealthy(t, s)
}

func TestR8ConcurrentReconciliationAndBackfillIsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "concurrent-reconcile.db")
	s1, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s1.Close()
	orbit, _ := s1.CreateOrbit("Concurrent reconcile", 101)
	s2, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	// Emulate one old-binary interval while both new stores remain open. The
	// next reconciliation calls must serialize and converge on one actor/binding.
	legacy, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	nodeToken := randomHex(32)
	if _, err := legacy.Exec(`INSERT INTO members(orbit_id, tg_user_id, role, joined_at) VALUES(?, 202, 'companion', 2)`, orbit.ID); err != nil {
		legacy.Close()
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`INSERT INTO slots(orbit_id, slot, token_hash, paired_by, paired_at) VALUES(?, 'a', ?, 202, 2)`,
		orbit.ID, hashToken(nodeToken)); err != nil {
		legacy.Close()
		t.Fatal(err)
	}
	legacy.Close()
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, st := range []*Store{s1, s2} {
		wg.Add(1)
		go func(st *Store) {
			defer wg.Done()
			<-start
			errs <- st.ReconcileIdentity()
		}(st)
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	ctx, err := s1.ResolveTokenActorContext(nodeToken)
	if err != nil || ctx.Role != "companion" {
		t.Fatalf("concurrent backfill context=%+v err=%v", ctx, err)
	}
	var credentials, actors int
	if err := s1.db.QueryRow(`SELECT COUNT(*) FROM installation_credentials WHERE slot_orbit_id = ? AND slot_name = 'a'`, orbit.ID).Scan(&credentials); err != nil {
		t.Fatal(err)
	}
	if err := s1.db.QueryRow(`SELECT COUNT(*) FROM actors WHERE kind = 'app_installation' AND external_ref LIKE ?`,
		"%:a:%").Scan(&actors); err != nil {
		t.Fatal(err)
	}
	if credentials != 1 || actors != 1 {
		t.Fatalf("concurrent backfill credentials=%d actors=%d", credentials, actors)
	}
	assertDatabaseHealthy(t, s1)
}
