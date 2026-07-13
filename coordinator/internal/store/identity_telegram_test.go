package store

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type telegramLinkFixture struct {
	orbit       *Orbit
	issuer      ActorContext
	issuerOwner int64
	code        string
	now         int64
}

type fakeTelegramLinkClock struct {
	now int64
}

func (c *fakeTelegramLinkClock) advance(d time.Duration) {
	c.now += d.Milliseconds()
}

func expectedTelegramLinkRateLimitDigest(subject string) string {
	sum := sha256.Sum256([]byte("barycenter/rate-limit-subject/v1:" +
		string(RateLimitTelegramLinkConsumeTelegram) + ":" + subject))
	return hex.EncodeToString(sum[:])
}

func newTelegramLinkFixture(t *testing.T, s *Store, owner int64, role string) telegramLinkFixture {
	t.Helper()
	orbit, err := s.CreateOrbit("Telegram link fixture", owner)
	if err != nil {
		t.Fatal(err)
	}
	_, issuer := provisionTestInstallation(t, s, orbit.ID, owner)
	now := time.Now().UnixMilli()
	return telegramLinkFixture{
		orbit:       orbit,
		issuer:      issuer,
		issuerOwner: owner,
		code:        insertTelegramLinkCode(t, s, issuer.ActorID, orbit.ID, role, now+time.Hour.Milliseconds(), now),
		now:         now,
	}
}

func insertTelegramLinkCode(t *testing.T, s *Store, issuerActorID, orbitID int64, role string, expiresAt, createdAt int64) string {
	t.Helper()
	code, err := generateSecret(27)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`INSERT INTO telegram_link_codes
  (code_hash, issuer_actor_id, orbit_id, desired_role, expires_at, created_at)
VALUES(?, ?, ?, ?, ?, ?)`, hashToken(code), issuerActorID, orbitID, role, expiresAt, createdAt); err != nil {
		t.Fatal(err)
	}
	return code
}

func linkedTelegramActorID(t *testing.T, s *Store, userID int64) int64 {
	t.Helper()
	var actorID int64
	if err := s.db.QueryRow(`SELECT id FROM actors
WHERE kind = 'telegram_user' AND external_ref = ?`, userID).Scan(&actorID); err != nil {
		t.Fatal(err)
	}
	return actorID
}

func assertTelegramCodeConsumed(t *testing.T, s *Store, code string, want bool) {
	t.Helper()
	var consumed sql.NullInt64
	if err := s.db.QueryRow(`SELECT consumed_at FROM telegram_link_codes WHERE code_hash = ?`, hashToken(code)).Scan(&consumed); err != nil {
		t.Fatal(err)
	}
	if consumed.Valid != want {
		t.Fatalf("code consumed=%v, want %v", consumed.Valid, want)
	}
}

func TestTelegramMigrationPreservesMemberRolesSlotsAndActorContexts(t *testing.T) {
	fixture := createLegacyFixture(t)
	satelliteNode := randomHex(32)
	db, err := sql.Open("sqlite", fixture.path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO members(orbit_id, tg_user_id, role, joined_at, display_name)
VALUES(7, 303, 'satellite', 1003, 'Satellite')`); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`INSERT INTO slots
  (orbit_id, slot, token_hash, paired_by, provider, paired_at, revoked_at)
VALUES(7, 'd', ?, 303, 'spotify', 1104, NULL)`, hashToken(satelliteNode)); err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	s, err := OpenWithOptions(fixture.path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	for userID, wantRole := range map[int64]string{101: "primary", 202: "companion", 303: "satellite"} {
		ctx, err := s.ResolveTelegramActorContext(userID)
		if err != nil || ctx.OrbitID != 7 || ctx.Role != wantRole || ctx.Capabilities != CapabilityTelegram {
			t.Fatalf("telegram %d context=%+v err=%v", userID, ctx, err)
		}
		legacy, err := s.MemberOf(userID)
		if err != nil || legacy == nil || legacy.Role != wantRole {
			t.Fatalf("legacy member %d=%+v err=%v", userID, legacy, err)
		}
	}
	if orbitID, slot, ok, err := s.LookupToken(satelliteNode); err != nil || !ok || orbitID != 7 || slot != "d" {
		t.Fatalf("satellite-owned slot changed: orbit=%d slot=%q ok=%v err=%v", orbitID, slot, ok, err)
	}
	var pairedBy int64
	if err := s.db.QueryRow(`SELECT paired_by FROM slots WHERE orbit_id = 7 AND slot = 'd'`).Scan(&pairedBy); err != nil || pairedBy != 303 {
		t.Fatalf("slot owner=%d err=%v", pairedBy, err)
	}
}

func TestConsumeTelegramLinkJoinsWithoutTransferringInstallationOwnership(t *testing.T) {
	s := openIdentityTemp(t)
	fixture := newTelegramLinkFixture(t, s, 101, "companion")

	var credentialOwner int64
	if err := s.db.QueryRow(`SELECT actor_id FROM installation_credentials
WHERE slot_orbit_id = ? AND slot_name = ?`, fixture.orbit.ID, fixture.issuer.Slot).Scan(&credentialOwner); err != nil {
		t.Fatal(err)
	}
	formattedCode := strings.ToLower(fixture.code[:9] + "-" + fixture.code[9:18] + " " + fixture.code[18:])
	result, err := s.consumeTelegramLinkAt(303, "Linked User", "private", formattedCode, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	if result.OrbitID != fixture.orbit.ID || result.ActorID == 0 || result.Role != "companion" {
		t.Fatalf("consume result=%+v", result)
	}
	ctx, err := s.ResolveTelegramActorContext(303)
	if err != nil || ctx != (ActorContext{OrbitID: fixture.orbit.ID, ActorID: result.ActorID, Role: "companion", Capabilities: CapabilityTelegram}) {
		t.Fatalf("linked context=%+v err=%v", ctx, err)
	}
	legacy, err := s.MemberOf(303)
	if err != nil || legacy == nil || legacy.OrbitID != fixture.orbit.ID || legacy.Role != "companion" || legacy.DisplayName != "Linked User" {
		t.Fatalf("legacy dual-write=%+v err=%v", legacy, err)
	}
	var credentialOwnerAfter int64
	if err := s.db.QueryRow(`SELECT actor_id FROM installation_credentials
WHERE slot_orbit_id = ? AND slot_name = ?`, fixture.orbit.ID, fixture.issuer.Slot).Scan(&credentialOwnerAfter); err != nil {
		t.Fatal(err)
	}
	if credentialOwnerAfter != credentialOwner || credentialOwnerAfter == result.ActorID {
		t.Fatalf("installation ownership changed: before=%d after=%d telegram=%d", credentialOwner, credentialOwnerAfter, result.ActorID)
	}
	var storedHash string
	if err := s.db.QueryRow(`SELECT code_hash FROM telegram_link_codes WHERE consuming_actor_id = ?`, result.ActorID).Scan(&storedHash); err != nil {
		t.Fatal(err)
	}
	if storedHash != hashToken(fixture.code) || storedHash == fixture.code {
		t.Fatal("link code was not stored under the canonical hash contract")
	}
	var audits int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM audit_events
WHERE actor_id = ? AND type = 'telegram_link.consumed'`, result.ActorID).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("consume audits=%d err=%v", audits, err)
	}
	assertTelegramCodeConsumed(t, s, fixture.code, true)
	if _, err := s.consumeTelegramLinkAt(303, "Linked User", "private", fixture.code, fixture.now+1); !errors.Is(err, ErrTelegramLinkInvalid) || strings.Contains(err.Error(), fixture.code) {
		t.Fatalf("replay error=%v", err)
	}
}

func TestConsumeTelegramLinkAcceptsCompanionIssuerAndSatelliteRole(t *testing.T) {
	s := openIdentityTemp(t)
	orbit, err := s.CreateOrbit("Companion-issued Telegram link", 1301)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.AddMember(orbit.ID, 1302, "companion"); err != nil {
		t.Fatal(err)
	}
	_, companionInstallation := provisionTestInstallation(t, s, orbit.ID, 1302)
	control, recoveryID, recoverySecret := newProvisioningMaterial(t)
	if err := s.ProvisionInstallationSecrets(
		Identity{Kind: IdentityTelegram, TelegramUserID: 1302},
		companionInstallation.ActorID,
		control,
		recoveryID,
		recoverySecret,
	); err != nil {
		t.Fatal(err)
	}
	issued, err := s.IssueTelegramLink(companionInstallation.ActorID, control, "satellite")
	if err != nil {
		t.Fatal(err)
	}
	result, err := s.ConsumeTelegramLink(1303, "Satellite", "private", issued.Code)
	if err != nil || result.OrbitID != orbit.ID || result.Role != "satellite" {
		t.Fatalf("companion-issued consume=%+v err=%v", result, err)
	}
	ctx, err := s.ResolveTelegramActorContext(1303)
	if err != nil || ctx.Role != "satellite" || ctx.Capabilities != CapabilityTelegram {
		t.Fatalf("satellite context=%+v err=%v", ctx, err)
	}
}

func TestConsumeTelegramLinkConflictMatrixPreservesRolesAndCodes(t *testing.T) {
	roles := []string{"primary", "companion", "satellite"}
	for i, role := range roles {
		t.Run("legacy_"+role, func(t *testing.T) {
			s := openIdentityTemp(t)
			fixture := newTelegramLinkFixture(t, s, 101, "companion")
			userID := int64(101 + i)
			if role != "primary" {
				if err := s.AddMember(fixture.orbit.ID, userID, role); err != nil {
					t.Fatal(err)
				}
			}
			code := insertTelegramLinkCode(t, s, fixture.issuer.ActorID, fixture.orbit.ID, "companion", fixture.now+time.Hour.Milliseconds(), fixture.now+1)
			if _, err := s.consumeTelegramLinkAt(userID, "Existing", "private", code, fixture.now+2); !errors.Is(err, ErrTelegramAlreadyLinkedSameOrbit) {
				t.Fatalf("same-orbit error=%v", err)
			}
			legacy, err := s.MemberOf(userID)
			if err != nil || legacy == nil || legacy.Role != role {
				t.Fatalf("legacy role=%+v err=%v", legacy, err)
			}
			assertTelegramCodeConsumed(t, s, code, false)
		})
	}

	t.Run("additive_only_same_orbit", func(t *testing.T) {
		s := openIdentityTemp(t)
		fixture := newTelegramLinkFixture(t, s, 201, "satellite")
		res, err := s.db.Exec(`INSERT INTO actors(kind, display_name, external_ref, created_at)
VALUES('telegram_user', 'Additive', '404', ?)`, fixture.now)
		if err != nil {
			t.Fatal(err)
		}
		actorID, _ := res.LastInsertId()
		if _, err := s.db.Exec(`INSERT INTO memberships(orbit_id, actor_id, role, joined_at)
VALUES(?, ?, 'companion', ?)`, fixture.orbit.ID, actorID, fixture.now); err != nil {
			t.Fatal(err)
		}
		if _, err := s.consumeTelegramLinkAt(404, "Additive", "private", fixture.code, fixture.now+1); !errors.Is(err, ErrTelegramAlreadyLinkedSameOrbit) {
			t.Fatalf("additive-only error=%v", err)
		}
		assertTelegramCodeConsumed(t, s, fixture.code, false)
	})

	t.Run("foreign_orbit", func(t *testing.T) {
		s := openIdentityTemp(t)
		target := newTelegramLinkFixture(t, s, 301, "companion")
		if _, err := s.CreateOrbit("Foreign", 909); err != nil {
			t.Fatal(err)
		}
		if _, err := s.consumeTelegramLinkAt(909, "Foreign", "private", target.code, target.now+1); !errors.Is(err, ErrTelegramMemberOfOtherOrbit) {
			t.Fatalf("foreign-orbit error=%v", err)
		}
		assertTelegramCodeConsumed(t, s, target.code, false)
	})

	t.Run("legacy_additive_role_divergence", func(t *testing.T) {
		s := openIdentityTemp(t)
		fixture := newTelegramLinkFixture(t, s, 501, "companion")
		actorID := linkedTelegramActorID(t, s, 501)
		if _, err := s.db.Exec(`UPDATE memberships SET role = 'satellite' WHERE actor_id = ?`, actorID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.consumeTelegramLinkAt(501, "Divergent", "private", fixture.code, fixture.now+1); !errors.Is(err, ErrTelegramAlreadyLinkedSameOrbit) {
			t.Fatalf("divergent error=%v", err)
		}
		legacy, _ := s.MemberOf(501)
		if legacy == nil || legacy.Role != "primary" {
			t.Fatalf("legacy role changed=%+v", legacy)
		}
		var additiveRole string
		if err := s.db.QueryRow(`SELECT role FROM memberships WHERE actor_id = ?`, actorID).Scan(&additiveRole); err != nil || additiveRole != "satellite" {
			t.Fatalf("additive role=%q err=%v", additiveRole, err)
		}
		assertTelegramCodeConsumed(t, s, fixture.code, false)
	})
}

func TestConsumeTelegramLinkLifecycleExpiryInvalidationAndReplay(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, s *Store, f telegramLinkFixture)
	}{
		{name: "expired", mutate: func(t *testing.T, s *Store, f telegramLinkFixture) {
			_, err := s.db.Exec(`UPDATE telegram_link_codes SET expires_at = ? WHERE code_hash = ?`, f.now, hashToken(f.code))
			if err != nil {
				t.Fatal(err)
			}
		}},
		{name: "invalidated", mutate: func(t *testing.T, s *Store, f telegramLinkFixture) {
			_, err := s.db.Exec(`UPDATE telegram_link_codes SET invalidated_at = ? WHERE code_hash = ?`, f.now, hashToken(f.code))
			if err != nil {
				t.Fatal(err)
			}
		}},
		{name: "issuer_revoked", mutate: func(t *testing.T, s *Store, f telegramLinkFixture) {
			_, err := s.db.Exec(`UPDATE actors SET revoked_at = ? WHERE id = ?`, f.now, f.issuer.ActorID)
			if err != nil {
				t.Fatal(err)
			}
		}},
		{name: "issuer_left", mutate: func(t *testing.T, s *Store, f telegramLinkFixture) {
			_, err := s.db.Exec(`UPDATE memberships SET left_at = ? WHERE actor_id = ?`, f.now, f.issuer.ActorID)
			if err != nil {
				t.Fatal(err)
			}
		}},
		{name: "issuer_satellite", mutate: func(t *testing.T, s *Store, f telegramLinkFixture) {
			_, err := s.db.Exec(`UPDATE memberships SET role = 'satellite' WHERE actor_id = ?`, f.issuer.ActorID)
			if err != nil {
				t.Fatal(err)
			}
		}},
		{name: "orbit_disabled", mutate: func(t *testing.T, s *Store, f telegramLinkFixture) {
			_, err := s.db.Exec(`UPDATE orbits SET status = 'disabled' WHERE id = ?`, f.orbit.ID)
			if err != nil {
				t.Fatal(err)
			}
		}},
		{name: "consumer_revoked", mutate: func(t *testing.T, s *Store, f telegramLinkFixture) {
			_, err := s.db.Exec(`INSERT INTO actors(kind, display_name, external_ref, created_at, revoked_at)
VALUES('telegram_user', 'Revoked', '777', ?, ?)`, f.now, f.now)
			if err != nil {
				t.Fatal(err)
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := openIdentityTemp(t)
			fixture := newTelegramLinkFixture(t, s, 601, "companion")
			tc.mutate(t, s, fixture)
			userID := int64(778)
			if tc.name == "consumer_revoked" {
				userID = 777
			}
			if _, err := s.consumeTelegramLinkAt(userID, "Consumer", "private", fixture.code, fixture.now); !errors.Is(err, ErrTelegramLinkInvalid) {
				t.Fatalf("lifecycle error=%v", err)
			}
			assertTelegramCodeConsumed(t, s, fixture.code, false)
		})
	}

	t.Run("post_success_replay", func(t *testing.T) {
		s := openIdentityTemp(t)
		fixture := newTelegramLinkFixture(t, s, 701, "companion")
		if _, err := s.consumeTelegramLinkAt(778, "Replay", "private", fixture.code, fixture.now); err != nil {
			t.Fatal(err)
		}
		if _, err := s.consumeTelegramLinkAt(778, "Replay", "private", fixture.code, fixture.now+1); !errors.Is(err, ErrTelegramLinkInvalid) {
			t.Fatalf("replay error=%v", err)
		}
		assertTelegramCodeConsumed(t, s, fixture.code, true)
	})
}

func TestConsumeTelegramLinkReactivatesLeftActorIdempotently(t *testing.T) {
	s := openIdentityTemp(t)
	fixture := newTelegramLinkFixture(t, s, 801, "satellite")
	res, err := s.db.Exec(`INSERT INTO actors(kind, display_name, external_ref, created_at)
VALUES('telegram_user', 'Former', '880', ?)`, fixture.now-10)
	if err != nil {
		t.Fatal(err)
	}
	actorID, _ := res.LastInsertId()
	if _, err := s.db.Exec(`INSERT INTO memberships(orbit_id, actor_id, role, joined_at, left_at)
VALUES(?, ?, 'companion', ?, ?)`, fixture.orbit.ID, actorID, fixture.now-10, fixture.now-5); err != nil {
		t.Fatal(err)
	}
	result, err := s.consumeTelegramLinkAt(880, "Returned", "private", fixture.code, fixture.now)
	if err != nil {
		t.Fatal(err)
	}
	if result.ActorID != actorID || result.Role != "satellite" {
		t.Fatalf("reactivation result=%+v old actor=%d", result, actorID)
	}
	var actorCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM actors WHERE kind = 'telegram_user' AND external_ref = '880'`).Scan(&actorCount); err != nil || actorCount != 1 {
		t.Fatalf("actor count=%d err=%v", actorCount, err)
	}
	secondCode := insertTelegramLinkCode(t, s, fixture.issuer.ActorID, fixture.orbit.ID, "companion", fixture.now+time.Hour.Milliseconds(), fixture.now+1)
	if _, err := s.consumeTelegramLinkAt(880, "Returned", "private", secondCode, fixture.now+2); !errors.Is(err, ErrTelegramAlreadyLinkedSameOrbit) {
		t.Fatalf("idempotent same-orbit error=%v", err)
	}
	assertTelegramCodeConsumed(t, s, secondCode, false)
}

func TestLinkedTelegramPreservesPairTransferLeaveAndRevokeBehavior(t *testing.T) {
	s := openIdentityTemp(t)
	fixture := newTelegramLinkFixture(t, s, 901, "companion")
	if _, err := s.consumeTelegramLinkAt(902, "Linked", "private", fixture.code, fixture.now); err != nil {
		t.Fatal(err)
	}
	slot, nodeToken, err := s.PairSlot(fixture.orbit.ID, 902)
	if err != nil {
		t.Fatal(err)
	}
	node, err := s.ResolveTokenActorContext(nodeToken)
	if err != nil || node.Role != "companion" || node.Slot != slot {
		t.Fatalf("paired context=%+v err=%v", node, err)
	}
	if err := s.TransferPrimary(fixture.orbit.ID, 902); err != nil {
		t.Fatal(err)
	}
	telegram, err := s.ResolveTelegramActorContext(902)
	if err != nil || telegram.Role != "primary" {
		t.Fatalf("transferred telegram=%+v err=%v", telegram, err)
	}
	node, err = s.ResolveTokenActorContext(nodeToken)
	if err != nil || node.Role != "primary" {
		t.Fatalf("transferred slot=%+v err=%v", node, err)
	}
	if found, err := s.RevokeSlot(fixture.orbit.ID, slot); err != nil || !found {
		t.Fatalf("revoke found=%v err=%v", found, err)
	}
	if _, err := s.ResolveTokenActorContext(nodeToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked node error=%v", err)
	}
	dissolved, promoted, err := s.LeaveOrbit(fixture.orbit.ID, 902)
	if err != nil || dissolved || promoted != 901 {
		t.Fatalf("leave dissolved=%v promoted=%d err=%v", dissolved, promoted, err)
	}
	if _, err := s.ResolveTelegramActorContext(902); !errors.Is(err, ErrInsufficientCapability) {
		t.Fatalf("left telegram error=%v", err)
	}
	owner, err := s.ResolveTelegramActorContext(901)
	if err != nil || owner.Role != "primary" {
		t.Fatalf("promoted owner=%+v err=%v", owner, err)
	}
	var credentialOwner int64
	if err := s.db.QueryRow(`SELECT actor_id FROM installation_credentials
WHERE slot_orbit_id = ? AND slot_name = ?`, fixture.orbit.ID, fixture.issuer.Slot).Scan(&credentialOwner); err != nil || credentialOwner != fixture.issuer.ActorID {
		t.Fatalf("issuer installation owner=%d want=%d err=%v", credentialOwner, fixture.issuer.ActorID, err)
	}
}

func TestConsumeTelegramLinkRollsBackLegacyAndAuditFailures(t *testing.T) {
	for _, tc := range []struct {
		name    string
		trigger string
	}{
		{
			name: "legacy_dual_write",
			trigger: `CREATE TRIGGER telegram_link_fail_legacy BEFORE INSERT ON members
WHEN NEW.tg_user_id = 990 BEGIN SELECT RAISE(ABORT, 'injected legacy write failure'); END`,
		},
		{
			name: "audit",
			trigger: `CREATE TRIGGER telegram_link_fail_audit BEFORE INSERT ON audit_events
WHEN NEW.type = 'telegram_link.consumed' BEGIN SELECT RAISE(ABORT, 'injected audit failure'); END`,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := openIdentityTemp(t)
			fixture := newTelegramLinkFixture(t, s, 989, "companion")
			if _, err := s.db.Exec(tc.trigger); err != nil {
				t.Fatal(err)
			}
			if _, err := s.consumeTelegramLinkAt(990, "Rollback", "private", fixture.code, fixture.now); err == nil {
				t.Fatal("consume unexpectedly succeeded")
			}
			assertTelegramCodeConsumed(t, s, fixture.code, false)
			var actors, members int
			if err := s.db.QueryRow(`SELECT COUNT(*) FROM actors WHERE kind = 'telegram_user' AND external_ref = '990'`).Scan(&actors); err != nil {
				t.Fatal(err)
			}
			if err := s.db.QueryRow(`SELECT COUNT(*) FROM members WHERE tg_user_id = 990`).Scan(&members); err != nil {
				t.Fatal(err)
			}
			if actors != 0 || members != 0 {
				t.Fatalf("rollback leaked actors=%d members=%d", actors, members)
			}
		})
	}
}

func TestConsumeTelegramLinkConcurrentSameCodeTwoUsersHasOneWinner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "telegram-concurrent.db")
	s1, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s1.Close()
	fixture := newTelegramLinkFixture(t, s1, 1001, "companion")
	s2, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	s1.testCheckpoint = func(name string) error {
		if name == "telegram_link_after_lookup" {
			once.Do(func() { close(entered) })
			<-release
		}
		return nil
	}
	type consumeOutcome struct {
		result ConsumeTelegramLinkResult
		err    error
	}
	first := make(chan consumeOutcome, 1)
	second := make(chan consumeOutcome, 1)
	secondAttempting := make(chan struct{})
	var secondAttemptOnce sync.Once
	var firstReleased atomic.Bool
	var releaseOnce sync.Once
	releaseFirstWriter := func() {
		firstReleased.Store(true)
		releaseOnce.Do(func() { close(release) })
	}
	defer releaseFirstWriter()
	s2.testCheckpoint = func(name string) error {
		switch name {
		case "telegram_link_transaction_attempting":
			secondAttemptOnce.Do(func() { close(secondAttempting) })
		case "telegram_link_preflight_read":
			if !firstReleased.Load() {
				return errors.New("second Telegram writer read the credential before acquiring the immediate transaction")
			}
		}
		return nil
	}
	go func() {
		result, err := s1.consumeTelegramLinkAt(1002, "First", "private", fixture.code, fixture.now)
		first <- consumeOutcome{result: result, err: err}
	}()
	<-entered
	go func() {
		result, err := s2.consumeTelegramLinkAt(1003, "Second", "private", fixture.code, fixture.now)
		second <- consumeOutcome{result: result, err: err}
	}()
	select {
	case <-secondAttempting:
	case outcome := <-second:
		t.Fatalf("second writer completed before attempting BEGIN IMMEDIATE: %+v", outcome)
	case <-time.After(5 * time.Second):
		t.Fatal("second writer did not attempt BEGIN IMMEDIATE")
	}
	select {
	case outcome := <-second:
		t.Fatalf("second writer escaped BEGIN IMMEDIATE barrier: %+v", outcome)
	case <-time.After(100 * time.Millisecond):
	}
	releaseFirstWriter()
	firstOutcome := <-first
	secondOutcome := <-second
	if firstOutcome.err != nil || firstOutcome.result.ActorID == 0 {
		t.Fatalf("winner=%+v", firstOutcome)
	}
	if !errors.Is(secondOutcome.err, ErrTelegramLinkInvalid) {
		t.Fatalf("loser=%+v", secondOutcome)
	}
	assertTelegramCodeConsumed(t, s2, fixture.code, true)
	var members int
	if err := s2.db.QueryRow(`SELECT COUNT(*) FROM members WHERE tg_user_id IN (1002, 1003)`).Scan(&members); err != nil || members != 1 {
		t.Fatalf("winner member count=%d err=%v", members, err)
	}
}

func TestConsumeTelegramLinkConcurrentTwoCodesSameUserLeavesLoserCode(t *testing.T) {
	path := filepath.Join(t.TempDir(), "telegram-two-codes.db")
	s1, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s1.Close()
	fixture := newTelegramLinkFixture(t, s1, 1101, "companion")
	secondCode := insertTelegramLinkCode(t, s1, fixture.issuer.ActorID, fixture.orbit.ID, "satellite", fixture.now+time.Hour.Milliseconds(), fixture.now+1)
	s2, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	s1.testCheckpoint = func(name string) error {
		if name == "telegram_link_after_lookup" {
			once.Do(func() { close(entered) })
			<-release
		}
		return nil
	}
	first := make(chan error, 1)
	second := make(chan error, 1)
	secondAttempting := make(chan struct{})
	var secondAttemptOnce sync.Once
	var firstReleased atomic.Bool
	var releaseOnce sync.Once
	releaseFirstWriter := func() {
		firstReleased.Store(true)
		releaseOnce.Do(func() { close(release) })
	}
	defer releaseFirstWriter()
	s2.testCheckpoint = func(name string) error {
		switch name {
		case "telegram_link_transaction_attempting":
			secondAttemptOnce.Do(func() { close(secondAttempting) })
		case "telegram_link_preflight_read":
			if !firstReleased.Load() {
				return errors.New("second Telegram writer read the credential before acquiring the immediate transaction")
			}
		}
		return nil
	}
	go func() {
		_, err := s1.consumeTelegramLinkAt(1102, "Same", "private", fixture.code, fixture.now+2)
		first <- err
	}()
	<-entered
	go func() {
		_, err := s2.consumeTelegramLinkAt(1102, "Same", "private", secondCode, fixture.now+2)
		second <- err
	}()
	select {
	case <-secondAttempting:
	case err := <-second:
		t.Fatalf("second code completed before attempting BEGIN IMMEDIATE: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("second code did not attempt BEGIN IMMEDIATE")
	}
	select {
	case err := <-second:
		t.Fatalf("second code escaped BEGIN IMMEDIATE barrier: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	releaseFirstWriter()
	if err := <-first; err != nil {
		t.Fatalf("first code error=%v", err)
	}
	if err := <-second; !errors.Is(err, ErrTelegramAlreadyLinkedSameOrbit) {
		t.Fatalf("second code error=%v", err)
	}
	assertTelegramCodeConsumed(t, s2, fixture.code, true)
	assertTelegramCodeConsumed(t, s2, secondCode, false)
}

func TestConsumeTelegramLinkFeatureOffPrivateChatAndRateLimit(t *testing.T) {
	t.Run("feature_off", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "feature-off.db")
		s, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}
		defer s.Close()
		code, err := generateSecret(27)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.ConsumeTelegramLink(1201, "Feature Off", "private", code); !errors.Is(err, ErrSelfServiceOnboardingDisabled) {
			t.Fatalf("feature-off error=%v", err)
		}
	})

	t.Run("private_chat", func(t *testing.T) {
		s := openIdentityTemp(t)
		fixture := newTelegramLinkFixture(t, s, 1202, "companion")
		if _, err := s.consumeTelegramLinkAt(1203, "Group", "group", fixture.code, fixture.now); !errors.Is(err, ErrTelegramLinkInvalid) {
			t.Fatalf("group error=%v", err)
		}
		assertTelegramCodeConsumed(t, s, fixture.code, false)
	})

	t.Run("atomic_attempt_boundary", func(t *testing.T) {
		s := openIdentityTemp(t)
		now := time.Date(2026, 7, 13, 15, 0, 0, 0, time.UTC).UnixMilli()
		const userID int64 = 8_123_456_789
		subject := strconv.FormatInt(userID, 10)
		beforeAudit := time.Now().Add(-time.Second).UnixMilli()
		for i := 1; i <= telegramLinkAttemptLimit; i++ {
			code, err := generateSecret(27)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.consumeTelegramLinkAt(userID, "Limited", "private", code, now); !errors.Is(err, ErrTelegramLinkInvalid) {
				t.Fatalf("attempt %d error=%v", i, err)
			}
		}
		code, err := generateSecret(27)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.consumeTelegramLinkAt(userID, "Limited", "private", code, now); !errors.Is(err, ErrTelegramLinkRateLimited) {
			t.Fatalf("attempt %d error=%v", telegramLinkAttemptLimit+1, err)
		}
		var eventType, class, digest string
		var orbitID, actorID sql.NullInt64
		var createdAt int64
		if err := s.db.QueryRow(`SELECT event_type, limiter_class, subject_digest,
  orbit_id, actor_id, created_at
FROM rate_limit_audit_events`).Scan(
			&eventType, &class, &digest, &orbitID, &actorID, &createdAt,
		); err != nil {
			t.Fatal(err)
		}
		if eventType != "security.rate_limited" || class != string(RateLimitTelegramLinkConsumeTelegram) ||
			digest != expectedTelegramLinkRateLimitDigest(subject) || orbitID.Valid || actorID.Valid ||
			createdAt < beforeAudit || createdAt > time.Now().Add(time.Minute).UnixMilli() {
			t.Fatalf("durable rate-limit audit event=%q class=%q digest_matches=%t orbit=%v actor=%v created_at=%d",
				eventType, class, digest == expectedTelegramLinkRateLimitDigest(subject), orbitID, actorID, createdAt)
		}
		persisted := strings.Join([]string{
			eventType,
			class,
			digest,
			strconv.FormatInt(createdAt, 10),
		}, "|")
		if strings.Contains(persisted, subject) {
			t.Fatal("raw Telegram user ID entered durable rate-limit audit")
		}
		var durableAudits, legacyAudits int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM rate_limit_audit_events`).Scan(&durableAudits); err != nil {
			t.Fatal(err)
		}
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM events
WHERE type = 'telegram_link.rate_limited'`).Scan(&legacyAudits); err != nil {
			t.Fatal(err)
		}
		if durableAudits != 1 || legacyAudits != 0 {
			t.Fatalf("durable audits=%d legacy audits=%d", durableAudits, legacyAudits)
		}
	})

	t.Run("durable_audit_failure_consumes_attempt_and_fails_structurally", func(t *testing.T) {
		s := openIdentityTemp(t)
		now := time.Date(2026, 7, 13, 15, 30, 0, 0, time.UTC).UnixMilli()
		const userID int64 = 8_223_456_789
		subject := strconv.FormatInt(userID, 10)
		code, err := generateSecret(27)
		if err != nil {
			t.Fatal(err)
		}
		for attempt := 1; attempt <= telegramLinkAttemptLimit; attempt++ {
			if _, err := s.consumeTelegramLinkAt(userID, "Limited", "private", code, now); !errors.Is(err, ErrTelegramLinkInvalid) {
				t.Fatalf("priming attempt %d error=%v", attempt, err)
			}
		}
		if _, err := s.db.Exec(`CREATE TRIGGER telegram_link_rate_audit_failure
BEFORE INSERT ON rate_limit_audit_events
BEGIN SELECT RAISE(ABORT, 'injected durable audit failure'); END`); err != nil {
			t.Fatal(err)
		}
		_, err = s.consumeTelegramLinkAt(userID, "Limited", "private", code, now)
		if err == nil || !strings.HasPrefix(err.Error(), "record telegram link rate-limit audit: ") {
			t.Fatalf("audit persistence error=%v", err)
		}
		for _, sentinel := range []error{
			ErrTelegramLinkRateLimited,
			ErrTelegramLinkInvalid,
			ErrTelegramAlreadyLinkedSameOrbit,
			ErrTelegramMemberOfOtherOrbit,
		} {
			if errors.Is(err, sentinel) {
				t.Fatalf("audit persistence error mapped to %v", sentinel)
			}
		}
		if strings.Contains(err.Error(), subject) || strings.Contains(err.Error(), code) {
			t.Fatal("audit persistence error exposed Telegram identity or link code")
		}
		var audits int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM rate_limit_audit_events`).Scan(&audits); err != nil || audits != 0 {
			t.Fatalf("failed durable audits=%d err=%v", audits, err)
		}
		if _, err := s.db.Exec(`DROP TRIGGER telegram_link_rate_audit_failure`); err != nil {
			t.Fatal(err)
		}
		if _, err := s.consumeTelegramLinkAt(userID, "Limited", "private", code, now); !errors.Is(err, ErrTelegramLinkRateLimited) {
			t.Fatalf("post-failure reserved attempt error=%v", err)
		}
		var class, digest string
		if err := s.db.QueryRow(`SELECT limiter_class, subject_digest
FROM rate_limit_audit_events`).Scan(&class, &digest); err != nil {
			t.Fatal(err)
		}
		if class != string(RateLimitTelegramLinkConsumeTelegram) ||
			digest != expectedTelegramLinkRateLimitDigest(subject) {
			t.Fatalf("post-failure audit class=%q digest_matches=%t",
				class, digest == expectedTelegramLinkRateLimitDigest(subject))
		}
	})
}

func TestTelegramLinkAttemptLimiterUsesExactRollingWindow(t *testing.T) {
	limiter := newTelegramLinkAttemptLimiter()
	clock := &fakeTelegramLinkClock{now: time.Date(2026, 7, 13, 12, 0, 0, 0, time.UTC).UnixMilli()}
	const userID int64 = 3101

	for attempt := 1; attempt <= telegramLinkAttemptLimit; attempt++ {
		if !limiter.reserve(userID, clock.now) {
			t.Fatalf("attempt %d was rejected", attempt)
		}
	}
	if limiter.reserve(userID, clock.now) {
		t.Fatalf("attempt %d was admitted", telegramLinkAttemptLimit+1)
	}

	// A later rejected attempt is itself part of the rolling history. Once the
	// original burst expires, it consumes one of the ten available positions.
	clock.advance(14 * time.Minute)
	if limiter.reserve(userID, clock.now) {
		t.Fatal("later over-limit attempt was admitted")
	}
	clock.advance(time.Minute)
	for attempt := 1; attempt < telegramLinkAttemptLimit; attempt++ {
		if !limiter.reserve(userID, clock.now) {
			t.Fatalf("post-expiry attempt %d was rejected", attempt)
		}
	}
	if limiter.reserve(userID, clock.now) {
		t.Fatal("rejected attempt did not advance the rolling boundary")
	}
	if got := len(limiter.entries[userID].attempts); got != telegramLinkAttemptLimit {
		t.Fatalf("retained history=%d, want exactly %d", got, telegramLinkAttemptLimit)
	}
}

func TestTelegramLinkAttemptLimiterRejectsFixedWindowBoundaryBurst(t *testing.T) {
	limiter := newTelegramLinkAttemptLimiter()
	boundary := time.Date(2026, 7, 13, 12, 15, 0, 0, time.UTC).UnixMilli()
	const userID int64 = 3102
	admitted := 0
	for attempt := 0; attempt < telegramLinkAttemptLimit; attempt++ {
		if limiter.reserve(userID, boundary-time.Second.Milliseconds()) {
			admitted++
		}
	}
	for attempt := 0; attempt < telegramLinkAttemptLimit; attempt++ {
		if limiter.reserve(userID, boundary+time.Second.Milliseconds()) {
			admitted++
		}
	}
	if admitted != telegramLinkAttemptLimit {
		t.Fatalf("boundary burst admissions=%d, want %d (never 20)", admitted, telegramLinkAttemptLimit)
	}
}

func TestTelegramLinkAttemptLimiterConcurrentReservationsAndLRUBound(t *testing.T) {
	t.Run("concurrent_exactly_ten", func(t *testing.T) {
		limiter := newTelegramLinkAttemptLimiter()
		now := time.Date(2026, 7, 13, 13, 0, 0, 0, time.UTC).UnixMilli()
		start := make(chan struct{})
		results := make(chan bool, 64)
		var wg sync.WaitGroup
		for i := 0; i < cap(results); i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				<-start
				results <- limiter.reserve(3103, now)
			}()
		}
		close(start)
		wg.Wait()
		close(results)
		admitted := 0
		for allowed := range results {
			if allowed {
				admitted++
			}
		}
		if admitted != telegramLinkAttemptLimit {
			t.Fatalf("concurrent admissions=%d, want %d", admitted, telegramLinkAttemptLimit)
		}
		if got := len(limiter.entries[3103].attempts); got != telegramLinkAttemptLimit {
			t.Fatalf("concurrent retained history=%d", got)
		}
	})

	t.Run("bounded_lru", func(t *testing.T) {
		limiter := newTelegramLinkAttemptLimiter()
		now := time.Date(2026, 7, 13, 14, 0, 0, 0, time.UTC).UnixMilli()
		for userID := int64(1); userID <= telegramLinkLimiterCap; userID++ {
			if !limiter.reserve(userID, now) {
				t.Fatalf("first attempt for user %d rejected", userID)
			}
		}
		// Refresh user 1 so user 2 becomes the least-recently-used entry.
		if !limiter.reserve(1, now+1) {
			t.Fatal("LRU refresh rejected")
		}
		if !limiter.reserve(telegramLinkLimiterCap+1, now+1) {
			t.Fatal("new LRU entry rejected")
		}
		if got := len(limiter.entries); got != telegramLinkLimiterCap {
			t.Fatalf("LRU entries=%d, want %d", got, telegramLinkLimiterCap)
		}
		if _, ok := limiter.entries[1]; !ok {
			t.Fatal("recently used entry was evicted")
		}
		if _, ok := limiter.entries[2]; ok {
			t.Fatal("least-recently-used entry was not evicted")
		}
	})
}

type telegramLinkDatabaseCounts struct {
	actors      int
	memberships int
	members     int
	audits      int
}

func readTelegramLinkDatabaseCounts(t *testing.T, s *Store) telegramLinkDatabaseCounts {
	t.Helper()
	var counts telegramLinkDatabaseCounts
	if err := s.db.QueryRow(`SELECT
  (SELECT COUNT(*) FROM actors),
  (SELECT COUNT(*) FROM memberships),
  (SELECT COUNT(*) FROM members),
  (SELECT COUNT(*) FROM audit_events)`).Scan(
		&counts.actors,
		&counts.memberships,
		&counts.members,
		&counts.audits,
	); err != nil {
		t.Fatal(err)
	}
	return counts
}

func TestConsumeTelegramLinkUniformPreMutationCredentialGateShape(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, s *Store, f telegramLinkFixture) string
	}{
		{
			name: "unknown_valid_shape_guess",
			mutate: func(t *testing.T, _ *Store, _ telegramLinkFixture) string {
				code, err := generateSecret(27)
				if err != nil {
					t.Fatal(err)
				}
				return code
			},
		},
		{
			name: "expired",
			mutate: func(t *testing.T, s *Store, f telegramLinkFixture) string {
				if _, err := s.db.Exec(`UPDATE telegram_link_codes SET expires_at = ? WHERE code_hash = ?`, f.now, hashToken(f.code)); err != nil {
					t.Fatal(err)
				}
				return f.code
			},
		},
		{
			name: "invalidated",
			mutate: func(t *testing.T, s *Store, f telegramLinkFixture) string {
				if _, err := s.db.Exec(`UPDATE telegram_link_codes SET invalidated_at = ? WHERE code_hash = ?`, f.now, hashToken(f.code)); err != nil {
					t.Fatal(err)
				}
				return f.code
			},
		},
		{
			name: "consumed",
			mutate: func(t *testing.T, s *Store, f telegramLinkFixture) string {
				if _, err := s.db.Exec(`UPDATE telegram_link_codes
SET consumed_at = ?, consuming_actor_id = ? WHERE code_hash = ?`, f.now, f.issuer.ActorID, hashToken(f.code)); err != nil {
					t.Fatal(err)
				}
				return f.code
			},
		},
		{
			name: "issuer_revoked",
			mutate: func(t *testing.T, s *Store, f telegramLinkFixture) string {
				if _, err := s.db.Exec(`UPDATE actors SET revoked_at = ? WHERE id = ?`, f.now, f.issuer.ActorID); err != nil {
					t.Fatal(err)
				}
				return f.code
			},
		},
		{
			name: "issuer_left",
			mutate: func(t *testing.T, s *Store, f telegramLinkFixture) string {
				if _, err := s.db.Exec(`UPDATE memberships SET left_at = ? WHERE actor_id = ?`, f.now, f.issuer.ActorID); err != nil {
					t.Fatal(err)
				}
				return f.code
			},
		},
		{
			name: "issuer_downgraded",
			mutate: func(t *testing.T, s *Store, f telegramLinkFixture) string {
				if _, err := s.db.Exec(`UPDATE memberships SET role = 'satellite' WHERE actor_id = ?`, f.issuer.ActorID); err != nil {
					t.Fatal(err)
				}
				return f.code
			},
		},
		{
			name: "disabled_orbit",
			mutate: func(t *testing.T, s *Store, f telegramLinkFixture) string {
				if _, err := s.db.Exec(`UPDATE orbits SET status = 'disabled' WHERE id = ?`, f.orbit.ID); err != nil {
					t.Fatal(err)
				}
				return f.code
			},
		},
		{
			name: "tampered_desired_role",
			mutate: func(t *testing.T, s *Store, f telegramLinkFixture) string {
				if _, err := s.db.Exec(`PRAGMA ignore_check_constraints = ON`); err != nil {
					t.Fatal(err)
				}
				if _, err := s.db.Exec(`UPDATE telegram_link_codes SET desired_role = 'primary' WHERE code_hash = ?`, hashToken(f.code)); err != nil {
					t.Fatal(err)
				}
				if _, err := s.db.Exec(`PRAGMA ignore_check_constraints = OFF`); err != nil {
					t.Fatal(err)
				}
				return f.code
			},
		},
	}

	for index, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := openIdentityTemp(t)
			fixture := newTelegramLinkFixture(t, s, int64(4000+index), "companion")
			code := tc.mutate(t, s, fixture)
			before := readTelegramLinkDatabaseCounts(t, s)

			operations := map[string]int{}
			s.testCheckpoint = func(name string) error {
				operations[name]++
				return nil
			}
			_, err := s.consumeTelegramLinkAt(int64(5000+index), "Untrusted Hint", "private", code, fixture.now)
			if !errors.Is(err, ErrTelegramLinkInvalid) || err.Error() != ErrTelegramLinkInvalid.Error() {
				t.Fatalf("uniform error=%v", err)
			}
			if operations["telegram_link_preflight_read"] != 1 || operations["telegram_link_preflight_hash_compare"] != 1 {
				t.Fatalf("preflight operations=%v", operations)
			}
			if operations["telegram_link_after_lookup"] != 0 {
				t.Fatalf("invalid credential escaped preflight: operations=%v", operations)
			}
			after := readTelegramLinkDatabaseCounts(t, s)
			if after != before {
				t.Fatalf("database mutated: before=%+v after=%+v", before, after)
			}
		})
	}
}
