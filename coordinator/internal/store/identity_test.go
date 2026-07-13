package store

import (
	"database/sql"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func openIdentityTemp(t *testing.T) *Store {
	t.Helper()
	s, err := OpenWithOptions(filepath.Join(t.TempDir(), "identity.db"), Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

type legacyFixture struct {
	path        string
	primaryNode string
	partnerNode string
	orphanNode  string
}

func createLegacyFixture(t *testing.T) legacyFixture {
	t.Helper()
	fixture := legacyFixture{
		path:        filepath.Join(t.TempDir(), "legacy.db"),
		primaryNode: randomHex(32),
		partnerNode: randomHex(32),
		orphanNode:  randomHex(32),
	}
	db, err := sql.Open("sqlite", fixture.path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(orbitSchema); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO orbits
  (id, title, takeover_policy, voice_default, max_pulsars, max_members, created_at)
VALUES(7, 'Legacy', 'user', 'personal', 5, 10, 1000)`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO members(orbit_id, tg_user_id, role, joined_at, display_name)
VALUES(7, 101, 'primary', 1001, 'Primary'),
      (7, 202, 'companion', 1002, 'Partner')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO slots
  (orbit_id, slot, token_hash, paired_by, provider, paired_at, revoked_at)
VALUES(7, 'a', ?, 101, 'spotify', 1101, NULL),
      (7, 'b', ?, 202, 'spotify', NULL, NULL),
      (7, 'c', ?, 999, 'spotify', 1103, NULL)`,
		hashToken(fixture.primaryNode), hashToken(fixture.partnerNode), hashToken(fixture.orphanNode)); err != nil {
		t.Fatal(err)
	}
	return fixture
}

func TestIdentityMigrationBackfillsRepresentativeLegacyDatabase(t *testing.T) {
	fixture := createLegacyFixture(t)
	s, err := OpenWithOptions(fixture.path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()

	wantRoles := map[int64]string{101: "primary", 202: "companion"}
	rows, err := s.db.Query(`SELECT tg_user_id, role FROM members WHERE orbit_id = 7 ORDER BY tg_user_id`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var id int64
		var role string
		if err := rows.Scan(&id, &role); err != nil {
			t.Fatal(err)
		}
		if role != wantRoles[id] {
			t.Fatalf("legacy role changed for member %d: %q", id, role)
		}
	}
	rows.Close()

	tests := []struct {
		token string
		slot  string
		role  string
	}{
		{fixture.primaryNode, "a", "primary"},
		{fixture.partnerNode, "b", "companion"},
		{fixture.orphanNode, "c", "satellite"},
	}
	for _, tc := range tests {
		ctx, err := s.ResolveTokenActorContext(tc.token)
		if err != nil {
			t.Fatalf("resolve slot %s: %v", tc.slot, err)
		}
		if ctx.OrbitID != 7 || ctx.Slot != tc.slot || ctx.Role != tc.role || ctx.ActorID == 0 {
			t.Fatalf("slot %s context = %+v", tc.slot, ctx)
		}
		if ctx.Capabilities != CapabilityNode {
			t.Fatalf("slot %s capability = %v, want node-only", tc.slot, ctx.Capabilities)
		}
		if orbitID, slot, ok, err := s.LookupToken(tc.token); err != nil || !ok || orbitID != 7 || slot != tc.slot {
			t.Fatalf("legacy token validity changed for slot %s: orbit=%d slot=%s ok=%v err=%v", tc.slot, orbitID, slot, ok, err)
		}
	}

	telegram, err := s.ResolveTelegramActorContext(202)
	if err != nil || telegram.OrbitID != 7 || telegram.Role != "companion" || !telegram.Capabilities.Has(CapabilityTelegram) {
		t.Fatalf("telegram context = %+v err=%v", telegram, err)
	}
	var owner int64
	if err := s.db.QueryRow(`SELECT paired_by FROM slots WHERE orbit_id = 7 AND slot = 'b'`).Scan(&owner); err != nil || owner != 202 {
		t.Fatalf("slot ownership changed: owner=%d err=%v", owner, err)
	}
	var unprovisioned int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM installation_credentials
WHERE control_token_hash IS NULL AND recovery_id IS NULL AND recovery_secret_hash IS NULL`).Scan(&unprovisioned); err != nil || unprovisioned != 3 {
		t.Fatalf("backfilled credentials must be unprovisioned: count=%d err=%v", unprovisioned, err)
	}
	var pairedAt int64
	if err := s.db.QueryRow(`SELECT slot_paired_at FROM installation_credentials WHERE slot_name = 'b'`).Scan(&pairedAt); err != nil || pairedAt != 0 {
		t.Fatalf("nullable paired_at sentinel = %d, err=%v", pairedAt, err)
	}

	before := identityRowCounts(t, s)
	if err := s.ReconcileIdentity(); err != nil {
		t.Fatal(err)
	}
	if after := identityRowCounts(t, s); after != before {
		t.Fatalf("second reconciliation changed row counts: before=%v after=%v", before, after)
	}
	assertDatabaseHealthy(t, s)
}

type identityCounts struct {
	actors      int
	memberships int
	credentials int
	audits      int
}

func identityRowCounts(t *testing.T, s *Store) identityCounts {
	t.Helper()
	var counts identityCounts
	for _, item := range []struct {
		table string
		dst   *int
	}{
		{"actors", &counts.actors},
		{"memberships", &counts.memberships},
		{"installation_credentials", &counts.credentials},
		{"audit_events", &counts.audits},
	} {
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM ` + item.table).Scan(item.dst); err != nil {
			t.Fatal(err)
		}
	}
	return counts
}

func assertDatabaseHealthy(t *testing.T, s *Store) {
	t.Helper()
	if err := assertForeignKeys(s.db); err != nil {
		t.Fatal(err)
	}
	if err := foreignKeyCheck(s.db); err != nil {
		t.Fatal(err)
	}
	var integrity string
	if err := s.db.QueryRow(`PRAGMA integrity_check`).Scan(&integrity); err != nil || integrity != "ok" {
		t.Fatalf("integrity_check = %q err=%v", integrity, err)
	}
}

func TestHashOnlyCredentialPersistenceAndActorLookup(t *testing.T) {
	s := openIdentityTemp(t)
	orbit, err := s.CreateOrbit("Hash only", 101)
	if err != nil {
		t.Fatal(err)
	}
	_, nodeToken, err := s.PairSlot(orbit.ID, 101)
	if err != nil {
		t.Fatal(err)
	}
	nodeContext, err := s.ResolveTokenActorContext(nodeToken)
	if err != nil {
		t.Fatal(err)
	}
	if nodeContext.Capabilities.Has(CapabilityControl) {
		t.Fatal("node token unexpectedly has control capability")
	}

	controlToken := randomHex(32)
	recoveryID := "rec_" + randomHex(16)
	recoverySecret, err := generateSecret(27)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ProvisionInstallationSecrets(Identity{Kind: IdentityBearer, Token: nodeToken}, nodeContext.ActorID, controlToken, recoveryID, recoverySecret); !errors.Is(err, ErrInsufficientCapability) {
		t.Fatalf("node-token provisioning error = %v", err)
	}
	var stillUnprovisioned int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM installation_credentials
WHERE actor_id = ? AND control_token_hash IS NULL`, nodeContext.ActorID).Scan(&stillUnprovisioned); err != nil || stillUnprovisioned != 1 {
		t.Fatalf("node-token provisioning changed credentials: count=%d err=%v", stillUnprovisioned, err)
	}
	authority, err := s.ResolveTelegramActorContext(101)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.ProvisionInstallationSecrets(Identity{Kind: IdentityTelegram, TelegramUserID: 101}, nodeContext.ActorID, controlToken, recoveryID, recoverySecret); err != nil {
		t.Fatal(err)
	}

	var storedControl, storedRecovery string
	if err := s.db.QueryRow(`SELECT control_token_hash, recovery_secret_hash
FROM installation_credentials WHERE actor_id = ?`, nodeContext.ActorID).Scan(&storedControl, &storedRecovery); err != nil {
		t.Fatal(err)
	}
	if storedControl != hashToken(controlToken) || storedRecovery != hashToken(recoverySecret) {
		t.Fatal("credential hashes do not match the canonical string-byte hash contract")
	}
	if storedControl == controlToken || storedRecovery == recoverySecret {
		t.Fatal("plaintext credential material was persisted")
	}
	deviceCode, _ := generateSecret(27)
	linkCode, _ := generateSecret(27)
	now := time.Now().UnixMilli()
	if _, err := s.db.Exec(`INSERT INTO device_invites
  (code_hash, orbit_id, issued_by_actor_id, intended_role, expires_at, created_at)
VALUES(?, ?, ?, 'companion', ?, ?)`, hashToken(deviceCode), orbit.ID, authority.ActorID, now+60_000, now); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO telegram_link_codes
  (code_hash, issuer_actor_id, orbit_id, desired_role, expires_at, created_at)
VALUES(?, ?, ?, 'satellite', ?, ?)`, hashToken(linkCode), authority.ActorID, orbit.ID, now+60_000, now); err != nil {
		t.Fatal(err)
	}
	var storedDeviceCode, storedLinkCode string
	if err := s.db.QueryRow(`SELECT code_hash FROM device_invites`).Scan(&storedDeviceCode); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT code_hash FROM telegram_link_codes`).Scan(&storedLinkCode); err != nil {
		t.Fatal(err)
	}
	if storedDeviceCode != hashToken(deviceCode) || storedLinkCode != hashToken(linkCode) ||
		storedDeviceCode == deviceCode || storedLinkCode == linkCode {
		t.Fatal("new invite/link material was not persisted hash-only")
	}
	assertNoPlaintextSecretColumns(t, s)

	controlContext, err := s.ResolveTokenActorContext(controlToken)
	if err != nil {
		t.Fatal(err)
	}
	if controlContext.ActorID != nodeContext.ActorID || controlContext.OrbitID != orbit.ID || controlContext.Role != "primary" {
		t.Fatalf("control context mismatch: %+v", controlContext)
	}
	if !controlContext.Capabilities.Has(CapabilityNode | CapabilityControl) {
		t.Fatalf("control capabilities = %v", controlContext.Capabilities)
	}
	if _, err := s.ResolveTokenActorContext(randomHex(32)); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("unknown credential error = %v", err)
	}

	if got := hashToken(strings.Repeat("0", 64)); got != "60e05bd1b195af2f94112fa7197a5c88289058840ce7c6df9693756bc6250f55" {
		t.Fatalf("token hash vector mismatch: %s", got)
	}
	humanVector := secretAlphabet[:27]
	if got := hashToken(humanVector); got != "e45d6091f70eeb484d8b9fe2e4a9067d0159b336298c9a5f30804f592c3e824d" {
		t.Fatalf("human-secret hash vector mismatch: %s", got)
	}
	if !constantTimeHashEqual(humanVector, hashToken(humanVector)) {
		t.Fatal("constant-time digest comparison rejected matching values")
	}
}

func assertNoPlaintextSecretColumns(t *testing.T, s *Store) {
	t.Helper()
	for _, table := range []string{"installation_credentials", "device_invites", "telegram_link_codes"} {
		rows, err := s.db.Query(`PRAGMA table_info(` + quoteIdent(table) + `)`)
		if err != nil {
			t.Fatal(err)
		}
		for rows.Next() {
			var cid, notNull, pk int
			var name, typ string
			var defaultValue any
			if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
				rows.Close()
				t.Fatal(err)
			}
			switch name {
			case "control_token", "node_token", "recovery_secret", "code", "link_code":
				rows.Close()
				t.Fatalf("%s contains plaintext secret column %q", table, name)
			}
		}
		if err := rows.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestActorResolverFeatureGateAndLegacyCompatibility(t *testing.T) {
	path := filepath.Join(t.TempDir(), "feature-off.db")
	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	orbit, err := s.CreateOrbit("Legacy", 101)
	if err != nil {
		t.Fatal(err)
	}
	_, token, err := s.PairSlot(orbit.ID, 101)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetSlotProvider(orbit.ID, "a", "yandex"); err != nil {
		t.Fatal(err)
	}
	if found, err := s.RevokeSlot(orbit.ID, "a"); err != nil || !found {
		t.Fatalf("feature-off revoke: found=%v err=%v", found, err)
	}
	if slot, reboundToken, err := s.PairSlot(orbit.ID, 101); err != nil || slot != "a" {
		t.Fatalf("feature-off rebind: slot=%q err=%v", slot, err)
	} else {
		token = reboundToken
	}
	var provider string
	if err := s.db.QueryRow(`SELECT provider FROM slots WHERE orbit_id = ? AND slot = 'a'`, orbit.ID).Scan(&provider); err != nil || provider != "spotify" {
		t.Fatalf("feature-off rebind provider=%q err=%v", provider, err)
	}
	if _, err := s.ResolveTokenActorContext(token); !errors.Is(err, ErrSelfServiceOnboardingDisabled) {
		t.Fatalf("feature-off resolver error = %v", err)
	}
	if gotOrbit, gotSlot, ok, err := s.LookupPlaybackToken(token); err != nil || !ok || gotOrbit != orbit.ID || gotSlot != "a" {
		t.Fatalf("feature-off playback compatibility: orbit=%d slot=%s ok=%v err=%v", gotOrbit, gotSlot, ok, err)
	}
	if counts := identityRowCounts(t, s); counts.actors != 0 || counts.memberships != 0 || counts.credentials != 0 {
		t.Fatalf("feature-off legacy mutation unexpectedly backfilled identity: %+v", counts)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	ctx, err := s.ResolveTokenActorContext(token)
	if err != nil || ctx.OrbitID != orbit.ID || ctx.Role != "primary" || ctx.ActorID == 0 {
		t.Fatalf("feature-on backfill context = %+v err=%v", ctx, err)
	}
}

func TestFeatureOnInviteConsumeRejectsDisabledOrbitWithoutBurningCode(t *testing.T) {
	s := openIdentityTemp(t)
	orbit, err := s.CreateOrbit("Invite gate", 101)
	if err != nil {
		t.Fatal(err)
	}
	code, err := s.NewInvite(orbit.ID, 101)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE orbits SET status = 'disabled' WHERE id = ?`, orbit.ID); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.ConsumeInvite(code, "member"); !errors.Is(err, ErrOrbitDisabled) {
		t.Fatalf("disabled invite consume error = %v", err)
	}
	var usedAt sql.NullInt64
	if err := s.db.QueryRow(`SELECT used_at FROM invites WHERE code = ?`, code).Scan(&usedAt); err != nil || usedAt.Valid {
		t.Fatalf("disabled invite was burned: used_at=%+v err=%v", usedAt, err)
	}
	if _, err := s.db.Exec(`UPDATE orbits SET status = 'active' WHERE id = ?`, orbit.ID); err != nil {
		t.Fatal(err)
	}
	gotOrbit, issuedBy, err := s.ConsumeInvite(code, "member")
	if err != nil || gotOrbit != orbit.ID || issuedBy != 101 {
		t.Fatalf("active invite consume: orbit=%d issued_by=%d err=%v", gotOrbit, issuedBy, err)
	}
}

func TestFeatureOnLegacyBootstrapBackfillsBeforeReturn(t *testing.T) {
	s := openIdentityTemp(t)
	nodeToken := randomHex(32)
	orbit, err := s.BootstrapLegacyOrbit(map[string]string{"a": nodeToken}, map[int64]string{101: "a"})
	if err != nil || orbit == nil {
		t.Fatalf("bootstrap: orbit=%v err=%v", orbit, err)
	}
	ctx, err := s.ResolveTokenActorContext(nodeToken)
	if err != nil || ctx.OrbitID != orbit.ID || ctx.Role != "primary" || ctx.ActorID == 0 {
		t.Fatalf("bootstrap context = %+v err=%v", ctx, err)
	}
	telegram, err := s.ResolveTelegramActorContext(101)
	if err != nil || telegram.OrbitID != orbit.ID || telegram.Role != "primary" {
		t.Fatalf("bootstrap Telegram context = %+v err=%v", telegram, err)
	}
}

func TestActorResolverDistinguishesLifecycleFromCredentialFailure(t *testing.T) {
	s := openIdentityTemp(t)
	orbit, _ := s.CreateOrbit("Lifecycle", 101)
	_, nodeToken, _ := s.PairSlot(orbit.ID, 101)
	nodeContext, _ := s.ResolveTokenActorContext(nodeToken)
	controlToken := randomHex(32)
	recoverySecret, _ := generateSecret(27)
	if err := s.ProvisionInstallationSecrets(Identity{Kind: IdentityTelegram, TelegramUserID: 101}, nodeContext.ActorID, controlToken, "rec_"+randomHex(16), recoverySecret); err != nil {
		t.Fatal(err)
	}

	if _, err := s.db.Exec(`UPDATE orbits SET status = 'disabled' WHERE id = ?`, orbit.ID); err != nil {
		t.Fatal(err)
	}
	for _, token := range []string{nodeToken, controlToken} {
		if _, err := s.ResolveTokenActorContext(token); !errors.Is(err, ErrInsufficientCapability) {
			t.Fatalf("disabled-orbit resolver error = %v", err)
		}
		if _, _, ok, err := s.LookupPlaybackToken(token); err != nil || ok {
			t.Fatalf("disabled-orbit playback ok=%v err=%v", ok, err)
		}
	}
	if _, _, ok, err := s.LookupToken(nodeToken); err != nil || !ok {
		t.Fatalf("legacy raw token path changed while orbit disabled: ok=%v err=%v", ok, err)
	}
	var revoked sql.NullInt64
	if err := s.db.QueryRow(`SELECT revoked_at FROM slots WHERE orbit_id = ? AND slot = 'a'`, orbit.ID).Scan(&revoked); err != nil || revoked.Valid {
		t.Fatalf("orbit disable mutated slot revocation: %+v err=%v", revoked, err)
	}
	if _, err := s.db.Exec(`UPDATE orbits SET status = 'active' WHERE id = ?`, orbit.ID); err != nil {
		t.Fatal(err)
	}
	if ctx, err := s.ResolveTokenActorContext(controlToken); err != nil || ctx.ActorID != nodeContext.ActorID {
		t.Fatalf("re-enable did not restore original credential: %+v err=%v", ctx, err)
	}
	if _, err := s.db.Exec(`UPDATE actors SET revoked_at = ? WHERE id = ?`, time.Now().UnixMilli(), nodeContext.ActorID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveTokenActorContext(controlToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked actor error = %v", err)
	}
}

func TestRollbackRebindReconcilesWithoutChangingLegacyAuthority(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollback.db")
	s, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	orbit, _ := s.CreateOrbit("Rollback", 101)
	if err := s.AddMember(orbit.ID, 202, "companion"); err != nil {
		t.Fatal(err)
	}
	_, oldToken, _ := s.PairSlot(orbit.ID, 101)
	oldContext, _ := s.ResolveTokenActorContext(oldToken)
	var originalPairedAt int64
	if err := s.db.QueryRow(`SELECT paired_at FROM slots WHERE orbit_id = ? AND slot = 'a'`, orbit.ID).Scan(&originalPairedAt); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// Emulate the previous coordinator: no FK pragma, unknown additive rows
	// ignored, legacy members/slots remain the only authority.
	legacyDB, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	newToken := randomHex(32)
	if _, err := legacyDB.Exec(`UPDATE members SET role = CASE tg_user_id
WHEN 101 THEN 'companion' WHEN 202 THEN 'primary' END,
display_name = CASE tg_user_id WHEN 202 THEN 'Renamed' ELSE display_name END
WHERE orbit_id = ?`, orbit.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := legacyDB.Exec(`INSERT OR REPLACE INTO slots
  (orbit_id, slot, token_hash, paired_by, provider, paired_at, revoked_at)
VALUES(?, 'a', ?, 202, 'spotify', ?, NULL)`, orbit.ID, hashToken(newToken), originalPairedAt); err != nil {
		t.Fatal(err)
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	newContext, err := s.ResolveTokenActorContext(newToken)
	if err != nil {
		t.Fatal(err)
	}
	if newContext.ActorID == oldContext.ActorID || newContext.Role != "primary" || newContext.Slot != "a" {
		t.Fatalf("rebound context = %+v old_actor=%d", newContext, oldContext.ActorID)
	}
	if _, _, ok, err := s.LookupToken(oldToken); err != nil || ok {
		t.Fatalf("old pair token survived legacy rebind: ok=%v err=%v", ok, err)
	}
	var pairedBy int64
	if err := s.db.QueryRow(`SELECT paired_by FROM slots WHERE orbit_id = ? AND slot = 'a'`, orbit.ID).Scan(&pairedBy); err != nil || pairedBy != 202 {
		t.Fatalf("legacy slot ownership changed: paired_by=%d err=%v", pairedBy, err)
	}
	var partnerRole, partnerName string
	if err := s.db.QueryRow(`SELECT role, display_name FROM members WHERE orbit_id = ? AND tg_user_id = 202`, orbit.ID).Scan(&partnerRole, &partnerName); err != nil {
		t.Fatal(err)
	}
	if partnerRole != "primary" || partnerName != "Renamed" {
		t.Fatalf("legacy member mutation changed: role=%q name=%q", partnerRole, partnerName)
	}
	var oldRevoked sql.NullInt64
	if err := s.db.QueryRow(`SELECT revoked_at FROM actors WHERE id = ?`, oldContext.ActorID).Scan(&oldRevoked); err != nil || !oldRevoked.Valid {
		t.Fatalf("old actor was not revoked: %+v err=%v", oldRevoked, err)
	}
	var oldCredentials int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM installation_credentials WHERE actor_id = ?`, oldContext.ActorID).Scan(&oldCredentials); err != nil || oldCredentials != 0 {
		t.Fatalf("old credential row survived rebind: count=%d err=%v", oldCredentials, err)
	}
	assertDatabaseHealthy(t, s)
}

func TestRollbackDissolutionRevokesInstallationActorsBeforeCleanup(t *testing.T) {
	path := filepath.Join(t.TempDir(), "rollback-dissolve.db")
	s, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	orbit, err := s.CreateOrbit("Dissolve", 101)
	if err != nil {
		t.Fatal(err)
	}
	_, nodeToken, err := s.PairSlot(orbit.ID, 101)
	if err != nil {
		t.Fatal(err)
	}
	installation, err := s.ResolveTokenActorContext(nodeToken)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	legacyDB, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	for _, stmt := range []string{
		`DELETE FROM members WHERE orbit_id = ?`,
		`DELETE FROM slots WHERE orbit_id = ?`,
		`DELETE FROM invites WHERE orbit_id = ?`,
		`DELETE FROM availability WHERE orbit_id = ?`,
		`DELETE FROM links WHERE orbit_a = ? OR orbit_b = ?`,
		`DELETE FROM orbits WHERE id = ?`,
	} {
		args := []any{orbit.ID}
		if strings.Contains(stmt, "orbit_a") {
			args = []any{orbit.ID, orbit.ID}
		}
		if _, err := legacyDB.Exec(stmt, args...); err != nil {
			legacyDB.Close()
			t.Fatal(err)
		}
	}
	if err := legacyDB.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	var revokedAt sql.NullInt64
	if err := s.db.QueryRow(`SELECT revoked_at FROM actors WHERE id = ?`, installation.ActorID).Scan(&revokedAt); err != nil || !revokedAt.Valid {
		t.Fatalf("dissolved installation actor not revoked: revoked_at=%+v err=%v", revokedAt, err)
	}
	for _, table := range []string{"installation_credentials", "memberships"} {
		var count int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM `+table+` WHERE actor_id = ?`, installation.ActorID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s retained dissolved actor rows: count=%d err=%v", table, count, err)
		}
	}
	assertDatabaseHealthy(t, s)
}

func TestReconciliationFailsClosedOnBindingFingerprintCollision(t *testing.T) {
	s := openIdentityTemp(t)
	orbit, _ := s.CreateOrbit("Collision", 101)
	_, oldToken, _ := s.PairSlot(orbit.ID, 101)
	oldContext, _ := s.ResolveTokenActorContext(oldToken)
	newToken := randomHex(32)
	newHash := hashToken(newToken)
	newExternalRef, err := installationExternalRef(orbit.ID, "a", newHash)
	if err != nil {
		t.Fatal(err)
	}
	// Inject the otherwise-astronomical conflict: actor identity claims the new
	// fingerprint while its independent binding proof remains the old hash.
	if _, err := s.db.Exec(`UPDATE actors SET external_ref = ? WHERE id = ?`, newExternalRef, oldContext.ActorID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE slots SET token_hash = ? WHERE orbit_id = ? AND slot = 'a'`, newHash, orbit.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ReconcileIdentity(); err == nil || !strings.Contains(err.Error(), "binding collision") {
		t.Fatalf("binding collision must fail closed, got %v", err)
	}
	var revokedAt sql.NullInt64
	if err := s.db.QueryRow(`SELECT revoked_at FROM actors WHERE id = ?`, oldContext.ActorID).Scan(&revokedAt); err != nil || revokedAt.Valid {
		t.Fatalf("failed reconciliation committed actor mutation: %+v err=%v", revokedAt, err)
	}
	var credentials int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM installation_credentials WHERE actor_id = ?`, oldContext.ActorID).Scan(&credentials); err != nil || credentials != 1 {
		t.Fatalf("failed reconciliation committed credential mutation: count=%d err=%v", credentials, err)
	}
}

func TestReconciliationUsesFullBindingFingerprint(t *testing.T) {
	const (
		firstTokenHash  = "00000000000000000000000000000000000000000000000000000000000010d9"
		secondTokenHash = "0000000000000000000000000000000000000000000000000000000000015799"
	)
	firstRef, err := installationExternalRef(1, "a", firstTokenHash)
	if err != nil {
		t.Fatal(err)
	}
	secondRef, err := installationExternalRef(1, "a", secondTokenHash)
	if err != nil {
		t.Fatal(err)
	}
	firstFingerprint := strings.TrimPrefix(firstRef, "1:a:")
	secondFingerprint := strings.TrimPrefix(secondRef, "1:a:")
	if firstFingerprint[:8] != secondFingerprint[:8] || firstFingerprint == secondFingerprint {
		t.Fatal("fixture does not isolate truncated fingerprint behavior")
	}

	s := openIdentityTemp(t)
	orbit, err := s.CreateOrbit("Full fingerprint", 101)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO slots(orbit_id, slot, token_hash, paired_by, paired_at)
VALUES(?, 'a', ?, 101, 1234)`, orbit.ID, firstTokenHash); err != nil {
		t.Fatal(err)
	}
	if err := s.ReconcileIdentity(); err != nil {
		t.Fatal(err)
	}
	var firstActor int64
	if err := s.db.QueryRow(`SELECT actor_id FROM installation_credentials
WHERE slot_orbit_id = ? AND slot_name = 'a'`, orbit.ID).Scan(&firstActor); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE slots SET token_hash = ? WHERE orbit_id = ? AND slot = 'a'`, secondTokenHash, orbit.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ReconcileIdentity(); err != nil {
		t.Fatal(err)
	}
	var secondActor int64
	if err := s.db.QueryRow(`SELECT actor_id FROM installation_credentials
WHERE slot_orbit_id = ? AND slot_name = 'a'`, orbit.ID).Scan(&secondActor); err != nil {
		t.Fatal(err)
	}
	if firstActor == secondActor {
		t.Fatal("same-millisecond rebind reused the actor after a first-eight-hex fingerprint collision")
	}
	var revokedAt sql.NullInt64
	if err := s.db.QueryRow(`SELECT revoked_at FROM actors WHERE id = ?`, firstActor).Scan(&revokedAt); err != nil || !revokedAt.Valid {
		t.Fatalf("old collision fixture actor not revoked: revoked_at=%+v err=%v", revokedAt, err)
	}
}

func TestFeatureOnLegacyMutationsDualWriteIdentity(t *testing.T) {
	s := openIdentityTemp(t)
	orbit, err := s.CreateOrbit("Dual write", 101)
	if err != nil {
		t.Fatal(err)
	}
	primaryActor := telegramActorID(t, s, 101)
	assertMembership(t, s, primaryActor, orbit.ID, "primary", false)

	if err := s.AddMember(orbit.ID, 202, "companion"); err != nil {
		t.Fatal(err)
	}
	partnerActor := telegramActorID(t, s, 202)
	assertMembership(t, s, partnerActor, orbit.ID, "companion", false)
	if err := s.SetMemberName(orbit.ID, 202, "Partner"); err != nil {
		t.Fatal(err)
	}
	var displayName string
	if err := s.db.QueryRow(`SELECT display_name FROM actors WHERE id = ?`, partnerActor).Scan(&displayName); err != nil || displayName != "Partner" {
		t.Fatalf("actor display name = %q err=%v", displayName, err)
	}

	slotA, tokenA, err := s.PairSlot(orbit.ID, 101)
	if err != nil {
		t.Fatal(err)
	}
	installationA, err := s.ResolveTokenActorContext(tokenA)
	if err != nil || installationA.Slot != slotA || installationA.Role != "primary" {
		t.Fatalf("first installation = %+v err=%v", installationA, err)
	}
	if err := s.TransferPrimary(orbit.ID, 202); err != nil {
		t.Fatal(err)
	}
	assertMembership(t, s, primaryActor, orbit.ID, "companion", false)
	assertMembership(t, s, partnerActor, orbit.ID, "primary", false)
	assertMembership(t, s, installationA.ActorID, orbit.ID, "companion", false)

	slotB, tokenB, err := s.PairSlot(orbit.ID, 202)
	if err != nil {
		t.Fatal(err)
	}
	installationB, err := s.ResolveTokenActorContext(tokenB)
	if err != nil || installationB.Slot != slotB || installationB.Role != "primary" {
		t.Fatalf("second installation = %+v err=%v", installationB, err)
	}
	if found, err := s.RevokeSlot(orbit.ID, slotA); err != nil || !found {
		t.Fatalf("revoke: found=%v err=%v", found, err)
	}
	assertMembership(t, s, installationA.ActorID, orbit.ID, "companion", true)
	var credentialA int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM installation_credentials WHERE actor_id = ?`, installationA.ActorID).Scan(&credentialA); err != nil || credentialA != 0 {
		t.Fatalf("revoked installation credential count=%d err=%v", credentialA, err)
	}

	dissolved, promoted, err := s.LeaveOrbit(orbit.ID, 202)
	if err != nil || dissolved || promoted != 101 {
		t.Fatalf("leave: dissolved=%v promoted=%d err=%v", dissolved, promoted, err)
	}
	assertMembership(t, s, partnerActor, orbit.ID, "primary", true)
	assertMembership(t, s, primaryActor, orbit.ID, "primary", false)
	assertMembership(t, s, installationB.ActorID, orbit.ID, "primary", true)

	if _, err := s.db.Exec(`UPDATE orbits SET status = 'disabled' WHERE id = ?`, orbit.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.AddMember(orbit.ID, 303, "companion"); !errors.Is(err, ErrOrbitDisabled) {
		t.Fatalf("disabled AddMember error = %v", err)
	}
	if _, _, err := s.PairSlot(orbit.ID, 101); !errors.Is(err, ErrOrbitDisabled) {
		t.Fatalf("disabled PairSlot error = %v", err)
	}
	if _, err := s.db.Exec(`UPDATE orbits SET status = 'active' WHERE id = ?`, orbit.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteOrbit(orbit.ID); err != nil {
		t.Fatal(err)
	}
	for _, tableAndColumn := range [][2]string{
		{"members", "orbit_id"},
		{"slots", "orbit_id"},
		{"memberships", "orbit_id"},
		{"installation_credentials", "slot_orbit_id"},
		{"device_invites", "orbit_id"},
		{"telegram_link_codes", "orbit_id"},
	} {
		var count int
		query := fmt.Sprintf(`SELECT COUNT(*) FROM %s WHERE %s = ?`, tableAndColumn[0], tableAndColumn[1])
		if err := s.db.QueryRow(query, orbit.ID).Scan(&count); err != nil || count != 0 {
			t.Fatalf("%s rows after DeleteOrbit = %d err=%v", tableAndColumn[0], count, err)
		}
	}
	assertDatabaseHealthy(t, s)
}

func telegramActorID(t *testing.T, s *Store, telegramUserID int64) int64 {
	t.Helper()
	var actorID int64
	if err := s.db.QueryRow(`SELECT id FROM actors WHERE kind = 'telegram_user' AND external_ref = ?`,
		fmt.Sprintf("%d", telegramUserID)).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	return actorID
}

func assertMembership(t *testing.T, s *Store, actorID, orbitID int64, wantRole string, wantLeft bool) {
	t.Helper()
	var role string
	var leftAt sql.NullInt64
	if err := s.db.QueryRow(`SELECT role, left_at FROM memberships WHERE actor_id = ? AND orbit_id = ?`, actorID, orbitID).Scan(&role, &leftAt); err != nil {
		t.Fatal(err)
	}
	if role != wantRole || leftAt.Valid != wantLeft {
		t.Fatalf("membership actor=%d role=%q left=%v, want role=%q left=%v", actorID, role, leftAt.Valid, wantRole, wantLeft)
	}
}

func TestOrbitStatusMigrationRepairsUnconstrainedColumnAndPreservesObjects(t *testing.T) {
	path := filepath.Join(t.TempDir(), "unconstrained.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	dDL := []string{
		`CREATE TABLE orbits (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  takeover_policy TEXT NOT NULL DEFAULT 'user',
  voice_default TEXT NOT NULL DEFAULT 'personal',
  max_pulsars INTEGER NOT NULL DEFAULT 5,
  max_members INTEGER NOT NULL DEFAULT 10,
  created_at INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'active'
)`,
		`CREATE TABLE aux(value TEXT)`,
		`INSERT INTO orbits(id, title, created_at, status) VALUES(1, 'One', 1, 'active')`,
		`CREATE INDEX orbits_title_idx ON orbits(title)`,
		`CREATE TRIGGER orbits_audit AFTER UPDATE ON orbits BEGIN INSERT INTO aux(value) VALUES(new.title); END`,
		`CREATE VIEW orbit_titles AS SELECT id, title FROM orbits`,
		`CREATE VIEW aux_literal AS SELECT 'orbits' AS value FROM aux`,
		`CREATE TRIGGER aux_reads_orbits AFTER INSERT ON aux BEGIN SELECT COUNT(*) FROM orbits; END`,
		`CREATE TRIGGER aux_noop AFTER UPDATE ON aux BEGIN SELECT 1; END`,
	}
	for _, stmt := range dDL {
		if _, err := db.Exec(stmt); err != nil {
			t.Fatalf("fixture DDL failed: %v", err)
		}
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.db.Exec(`UPDATE orbits SET status = 'bogus' WHERE id = 1`); !isCheckConstraintError(err) {
		t.Fatalf("rebuilt status accepted bogus value: %v", err)
	}
	for _, name := range []string{"orbits_title_idx", "orbits_audit", "orbit_titles", "aux_literal", "aux_reads_orbits", "aux_noop"} {
		var count int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE name = ?`, name).Scan(&count); err != nil || count != 1 {
			t.Fatalf("schema object %q not preserved: count=%d err=%v", name, count, err)
		}
	}
	if _, err := s.db.Exec(`UPDATE orbits SET title = 'Changed' WHERE id = 1`); err != nil {
		t.Fatal(err)
	}
	var auditValue string
	if err := s.db.QueryRow(`SELECT value FROM aux WHERE value = 'Changed'`).Scan(&auditValue); err != nil {
		t.Fatalf("recreated trigger did not run: %v", err)
	}
	if next, err := s.CreateOrbit("Next", 101); err != nil || next.ID != 2 {
		t.Fatalf("AUTOINCREMENT state after rebuild: orbit=%+v err=%v", next, err)
	}
	assertDatabaseHealthy(t, s)
}

func TestOrbitStatusMigrationUsesBehaviorProbeVariants(t *testing.T) {
	tests := []struct {
		name           string
		statusClause   string
		insertRow      bool
		wantRebuiltSQL string
	}{
		{
			name:           "equivalent constraint",
			statusClause:   `CHECK(status = 'active' OR status = 'disabled')`,
			insertRow:      true,
			wantRebuiltSQL: `status = 'active' OR status = 'disabled'`,
		},
		{
			name:           "alternate whitespace constraint",
			statusClause:   "CHECK ( status = 'active'\n OR status = 'disabled' )",
			insertRow:      true,
			wantRebuiltSQL: `status = 'active'`,
		},
		{
			name:         "misleading comment is unconstrained",
			statusClause: "-- CHECK(status IN ('active', 'disabled'))",
			insertRow:    true,
		},
		{
			name:      "empty unconstrained table",
			insertRow: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "probe.db")
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			ddl := fmt.Sprintf(`CREATE TABLE orbits (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  takeover_policy TEXT NOT NULL DEFAULT 'user',
  voice_default TEXT NOT NULL DEFAULT 'personal',
  max_pulsars INTEGER NOT NULL DEFAULT 5,
  max_members INTEGER NOT NULL DEFAULT 10,
  created_at INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'active' %s
)`, tc.statusClause)
			if _, err := db.Exec(ddl); err != nil {
				t.Fatal(err)
			}
			if tc.insertRow {
				if _, err := db.Exec(`INSERT INTO orbits(id, title, created_at) VALUES(1, 'Probe', 1)`); err != nil {
					t.Fatal(err)
				}
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}

			s, err := Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			var writeErr error
			if tc.insertRow {
				_, writeErr = s.db.Exec(`UPDATE orbits SET status = 'bogus' WHERE id = 1`)
			} else {
				_, writeErr = s.db.Exec(`INSERT INTO orbits(title, created_at, status) VALUES('Bad', 1, 'bogus')`)
			}
			if !isCheckConstraintError(writeErr) {
				t.Fatalf("behavior probe variant accepted bogus status: %v", writeErr)
			}
			if tc.wantRebuiltSQL != "" {
				var schemaSQL string
				if err := s.db.QueryRow(`SELECT sql FROM sqlite_master WHERE type = 'table' AND name = 'orbits'`).Scan(&schemaSQL); err != nil {
					t.Fatal(err)
				}
				if !strings.Contains(schemaSQL, tc.wantRebuiltSQL) {
					t.Fatalf("equivalent constrained table was unexpectedly rebuilt: %s", schemaSQL)
				}
			}
		})
	}
}

func TestOrbitStatusMigrationRestoresForeignKeysAfterValidationRollback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "fk-rollback.db")
	fixture, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.Exec(`CREATE TABLE orbits (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  takeover_policy TEXT NOT NULL DEFAULT 'user',
  voice_default TEXT NOT NULL DEFAULT 'personal',
  max_pulsars INTEGER NOT NULL DEFAULT 5,
  max_members INTEGER NOT NULL DEFAULT 10,
  created_at INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'active'
);
CREATE TABLE actors(id INTEGER PRIMARY KEY);
CREATE TABLE memberships(
  orbit_id INTEGER NOT NULL REFERENCES orbits(id),
  actor_id INTEGER NOT NULL REFERENCES actors(id)
);
INSERT INTO actors(id) VALUES(1);
INSERT INTO memberships(orbit_id, actor_id) VALUES(999, 1)`); err != nil {
		t.Fatal(err)
	}
	if err := fixture.Close(); err != nil {
		t.Fatal(err)
	}

	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(ON)&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	defer db.Close()
	s := &Store{db: db}
	if err := s.ensureOrbitStatusConstraint(); err == nil || !strings.Contains(err.Error(), "foreign key violation") {
		t.Fatalf("migration validation error = %v", err)
	}
	if err := assertForeignKeys(db); err != nil {
		t.Fatalf("foreign key enforcement was not restored: %v", err)
	}
	constrained, err := orbitStatusConstrained(db)
	if err != nil {
		t.Fatal(err)
	}
	if constrained {
		t.Fatal("failed rebuild committed the replacement table")
	}
}

func TestOrbitStatusMigrationRejectsInvalidUnconstrainedRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invalid-status.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`CREATE TABLE orbits (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  takeover_policy TEXT NOT NULL DEFAULT 'user',
  voice_default TEXT NOT NULL DEFAULT 'personal',
  max_pulsars INTEGER NOT NULL DEFAULT 5,
  max_members INTEGER NOT NULL DEFAULT 10,
  created_at INTEGER NOT NULL,
  status TEXT NOT NULL DEFAULT 'active'
);
INSERT INTO orbits(id, title, created_at, status) VALUES(1, 'Bad', 1, 'bogus')`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	if s, err := Open(path); err == nil || !strings.Contains(err.Error(), "invalid row") {
		if s != nil {
			s.Close()
		}
		t.Fatalf("invalid unconstrained status must abort migration, got %v", err)
	}
}

func TestOrbitStatusMigrationInterruptedStateHandling(t *testing.T) {
	t.Run("orbits present restarts", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "recoverable.db")
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`CREATE TABLE orbits (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  title TEXT NOT NULL,
  takeover_policy TEXT NOT NULL DEFAULT 'user',
  voice_default TEXT NOT NULL DEFAULT 'personal',
  max_pulsars INTEGER NOT NULL DEFAULT 5,
  max_members INTEGER NOT NULL DEFAULT 10,
  created_at INTEGER NOT NULL
);
CREATE TABLE orbits_new(id INTEGER PRIMARY KEY)`); err != nil {
			t.Fatal(err)
		}
		db.Close()
		s, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		if exists, err := tableExists(s.db, "orbits_new"); err != nil || exists {
			t.Fatalf("recoverable orbits_new remained: exists=%v err=%v", exists, err)
		}
	})

	t.Run("orbits missing fails closed", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "fatal.db")
		db, err := sql.Open("sqlite", path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(`CREATE TABLE orbits_new(id INTEGER PRIMARY KEY)`); err != nil {
			t.Fatal(err)
		}
		db.Close()
		if s, err := Open(path); err == nil || !strings.Contains(err.Error(), "manual migration repair") {
			if s != nil {
				s.Close()
			}
			t.Fatalf("unrecoverable intermediate state error = %v", err)
		}
	})
}

func TestLegacyRollbackProjectionIsIdempotentAndRestoresQuotas(t *testing.T) {
	path := filepath.Join(t.TempDir(), "projection.db")
	s, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	orbit, _ := s.CreateOrbit("Projected", 101)
	_, nodeToken, _ := s.PairSlot(orbit.ID, 101)
	invite, _ := s.NewPairCode(orbit.ID, 101)
	if _, err := s.db.Exec(`UPDATE orbits SET status = 'disabled' WHERE id = ?`, orbit.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.ProjectIdentityForLegacyRollback(); err != nil {
		t.Fatal(err)
	}
	if err := s.ProjectIdentityForLegacyRollback(); err != nil {
		t.Fatal(err)
	}
	var maxPulsars, maxMembers int
	if err := s.db.QueryRow(`SELECT max_pulsars, max_members FROM orbits WHERE id = ?`, orbit.ID).Scan(&maxPulsars, &maxMembers); err != nil {
		t.Fatal(err)
	}
	if maxPulsars != 0 || maxMembers != 0 {
		t.Fatalf("projected quotas = %d/%d", maxPulsars, maxMembers)
	}
	var originalPulsars, originalMembers, projectionRows int
	if err := s.db.QueryRow(`SELECT original_max_pulsars, original_max_members, COUNT(*)
FROM rollback_projections WHERE orbit_id = ?`, orbit.ID).Scan(&originalPulsars, &originalMembers, &projectionRows); err != nil {
		t.Fatal(err)
	}
	if originalPulsars != 5 || originalMembers != 10 || projectionRows != 1 {
		t.Fatalf("projection journal = originals %d/%d rows=%d", originalPulsars, originalMembers, projectionRows)
	}
	var slotRevoked, inviteUsed sql.NullInt64
	if err := s.db.QueryRow(`SELECT revoked_at FROM slots WHERE orbit_id = ? AND slot = 'a'`, orbit.ID).Scan(&slotRevoked); err != nil || !slotRevoked.Valid {
		t.Fatalf("slot projection missing: %+v err=%v", slotRevoked, err)
	}
	if err := s.db.QueryRow(`SELECT used_at FROM invites WHERE code = ?`, invite).Scan(&inviteUsed); err != nil || !inviteUsed.Valid {
		t.Fatalf("invite projection missing: %+v err=%v", inviteUsed, err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	// Feature-off behavior matches the previous coordinator's enforcement
	// surface: token lookup fails and both mutation limits are zero.
	s, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, ok, err := s.LookupToken(nodeToken); err != nil || ok {
		t.Fatalf("projected legacy token lookup ok=%v err=%v", ok, err)
	}
	if _, _, err := s.PairSlot(orbit.ID, 101); !errors.Is(err, ErrLimit) {
		t.Fatalf("projected PairSlot error = %v", err)
	}
	if err := s.AddMember(orbit.ID, 202, "companion"); !errors.Is(err, ErrLimit) {
		t.Fatalf("projected AddMember error = %v", err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	s, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.db.QueryRow(`SELECT max_pulsars, max_members FROM orbits WHERE id = ?`, orbit.ID).Scan(&maxPulsars, &maxMembers); err != nil {
		t.Fatal(err)
	}
	if maxPulsars != 5 || maxMembers != 10 {
		t.Fatalf("restored quotas = %d/%d", maxPulsars, maxMembers)
	}
	var restoredAt sql.NullInt64
	if err := s.db.QueryRow(`SELECT restored_at FROM rollback_projections WHERE orbit_id = ?`, orbit.ID).Scan(&restoredAt); err != nil || !restoredAt.Valid {
		t.Fatalf("projection journal not restored: %+v err=%v", restoredAt, err)
	}
	if _, _, ok, err := s.LookupToken(nodeToken); err != nil || ok {
		t.Fatalf("projected slot was unexpectedly un-revoked: ok=%v err=%v", ok, err)
	}
	assertDatabaseHealthy(t, s)
}

func TestIdentitySchemaConstraintsAndImmediateWriterLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "constraints.db")
	s1, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s1.Close()
	if _, err := s1.db.Exec(`INSERT INTO actors(kind, display_name, external_ref, created_at)
VALUES('unknown', '', '', 1)`); err == nil {
		t.Fatal("actors.kind CHECK was not enforced")
	}
	orbit, err := s1.CreateOrbit("Constraints", 101)
	if err != nil {
		t.Fatal(err)
	}
	res, err := s1.db.Exec(`INSERT INTO actors(kind, display_name, external_ref, created_at)
VALUES('telegram_user', '', '101', 1)`)
	if err != nil {
		t.Fatal(err)
	}
	actorID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s1.db.Exec(`INSERT INTO memberships(orbit_id, actor_id, role, joined_at)
VALUES(?, ?, 'admin', 1)`, orbit.ID, actorID); err == nil {
		t.Fatal("memberships.role CHECK accepted admin")
	}
	if _, err := s1.db.Exec(`INSERT INTO memberships(orbit_id, actor_id, role, joined_at)
VALUES(?, ?, 'satellite', 1)`, orbit.ID, actorID); err != nil {
		t.Fatalf("memberships.role CHECK rejected satellite: %v", err)
	}
	if _, err := s1.db.Exec(`DELETE FROM memberships WHERE orbit_id = ? AND actor_id = ?`, orbit.ID, actorID); err != nil {
		t.Fatal(err)
	}
	if _, err := s1.db.Exec(`INSERT INTO memberships(orbit_id, actor_id, role, joined_at)
		VALUES(999, ?, 'satellite', 1)`, actorID); err == nil {
		t.Fatal("additive foreign keys were not enforced")
	}
	_, nodeToken, err := s1.PairSlot(orbit.ID, 101)
	if err != nil {
		t.Fatal(err)
	}
	res, err = s1.db.Exec(`INSERT INTO actors(kind, display_name, external_ref, created_at)
VALUES('app_installation', 'a', '1:a:test-constraints', 1)`)
	if err != nil {
		t.Fatal(err)
	}
	installationActorID, err := res.LastInsertId()
	if err != nil {
		t.Fatal(err)
	}
	insertCredential := func(control any, recoveryID any, recoveryHash any, consumedAt any, slotOrbit any, slotName any, bindingHash string) error {
		_, err := s1.db.Exec(`INSERT INTO installation_credentials
  (actor_id, slot_orbit_id, slot_name, slot_paired_at, binding_token_hash,
   control_token_hash, recovery_id, recovery_secret_hash, consumed_at, created_at)
VALUES(?, ?, ?, 1, ?, ?, ?, ?, ?, 1)`, installationActorID, slotOrbit, slotName,
			bindingHash, control, recoveryID, recoveryHash, consumedAt)
		return err
	}
	bindingHash := hashToken(nodeToken)
	for name, insertErr := range map[string]error{
		"short control hash":   insertCredential(strings.Repeat("a", 63), nil, nil, nil, orbit.ID, "a", bindingHash),
		"uppercase control":    insertCredential(strings.Repeat("A", 64), nil, nil, nil, orbit.ID, "a", bindingHash),
		"partial recovery":     insertCredential(nil, "rec_"+strings.Repeat("b", 32), nil, nil, orbit.ID, "a", bindingHash),
		"orphan consumed_at":   insertCredential(nil, nil, nil, int64(2), orbit.ID, "a", bindingHash),
		"missing slot orbit":   insertCredential(strings.Repeat("a", 64), nil, nil, nil, nil, "a", bindingHash),
		"missing slot name":    insertCredential(strings.Repeat("a", 64), nil, nil, nil, orbit.ID, nil, bindingHash),
		"invalid binding hash": insertCredential(nil, nil, nil, nil, orbit.ID, "a", "not-a-hash"),
	} {
		if insertErr == nil {
			t.Fatalf("installation credential CHECK accepted %s", name)
		}
	}
	if err := insertCredential(strings.Repeat("a", 64), "rec_"+strings.Repeat("b", 32), strings.Repeat("c", 64), nil, orbit.ID, "a", bindingHash); err != nil {
		t.Fatalf("valid installation credential rejected: %v", err)
	}
	if _, err := s1.db.Exec(`INSERT INTO telegram_link_codes
  (code_hash, issuer_actor_id, orbit_id, desired_role, expires_at, consumed_at, consuming_actor_id, created_at)
VALUES(?, ?, ?, 'primary', 2, NULL, NULL, 1)`, strings.Repeat("d", 64), actorID, orbit.ID); err == nil {
		t.Fatal("telegram link desired_role CHECK accepted primary")
	}
	if _, err := s1.db.Exec(`INSERT INTO telegram_link_codes
  (code_hash, issuer_actor_id, orbit_id, desired_role, expires_at, consumed_at, consuming_actor_id, created_at)
VALUES(?, ?, ?, 'companion', 2, 2, NULL, 1)`, strings.Repeat("e", 64), actorID, orbit.ID); err == nil {
		t.Fatal("telegram link consumed-state CHECK accepted a partial state")
	}
	if _, err := s1.db.Exec(`INSERT INTO telegram_link_codes
  (code_hash, issuer_actor_id, orbit_id, desired_role, expires_at, consumed_at, consuming_actor_id, created_at)
VALUES(?, ?, ?, 'companion', 2, 2, ?, 1)`, strings.Repeat("f", 64), actorID, orbit.ID, actorID); err != nil {
		t.Fatalf("valid consumed Telegram link rejected: %v", err)
	}
	if _, err := s1.db.Exec(`INSERT INTO telegram_link_codes
  (code_hash, issuer_actor_id, orbit_id, desired_role, expires_at, consumed_at, consuming_actor_id, created_at)
VALUES(?, ?, ?, 'satellite', 2, NULL, NULL, 1)`, strings.Repeat("0", 64), actorID, orbit.ID); err != nil {
		t.Fatalf("valid unconsumed Telegram link rejected: %v", err)
	}
	if _, err := s1.db.Exec(`INSERT INTO device_invites
  (code_hash, orbit_id, issued_by_actor_id, intended_role, expires_at, created_at)
		VALUES('not-a-hash', ?, ?, 'companion', 2, 1)`, orbit.ID, actorID); err == nil {
		t.Fatal("device invite hash CHECK was not enforced")
	}
	assertNoPlaintextSecretColumns(t, s1)

	s2, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	tx1, err := s1.db.Begin()
	if err != nil {
		t.Fatal(err)
	}
	started := make(chan error, 1)
	go func() {
		tx2, err := s2.db.Begin()
		if err == nil {
			_ = tx2.Rollback()
		}
		started <- err
	}()
	select {
	case err := <-started:
		_ = tx1.Rollback()
		t.Fatalf("second BEGIN IMMEDIATE did not block; err=%v", err)
	case <-time.After(100 * time.Millisecond):
	}
	if err := tx1.Commit(); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-started:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("second writer remained blocked after first commit")
	}
	assertDatabaseHealthy(t, s1)
}

func ExampleActorContext() {
	ctx := ActorContext{OrbitID: 1, ActorID: 2, Role: "companion", Capabilities: CapabilityNode | CapabilityControl}
	fmt.Println(ctx.Capabilities.Has(CapabilityControl))
	// Output: true
}
