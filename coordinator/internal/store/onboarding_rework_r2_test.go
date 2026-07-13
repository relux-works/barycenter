package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type recoveryAuditState struct {
	recoveryID     sql.NullString
	recoveryHash   sql.NullString
	consumedAt     sql.NullInt64
	controlHash    sql.NullString
	nodeHash       string
	rotationAudits int
	detailAudits   int
}

func loadRecoveryAuditState(t *testing.T, s *Store, actorID int64) recoveryAuditState {
	t.Helper()
	var state recoveryAuditState
	if err := s.db.QueryRow(`SELECT ic.recovery_id, ic.recovery_secret_hash,
       ic.consumed_at, ic.control_token_hash, sl.token_hash
FROM installation_credentials ic
JOIN slots sl ON sl.orbit_id = ic.slot_orbit_id AND sl.slot = ic.slot_name
WHERE ic.actor_id = ?`, actorID).Scan(&state.recoveryID, &state.recoveryHash,
		&state.consumedAt, &state.controlHash, &state.nodeHash); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM audit_events
WHERE actor_id = ? AND type = 'recovery.rotated'`, actorID).Scan(&state.rotationAudits); err != nil {
		t.Fatal(err)
	}
	if err := s.db.QueryRow(`SELECT COUNT(*)
FROM recovery_rotation_audit_details d
JOIN audit_events a ON a.id = d.audit_event_id
WHERE a.actor_id = ?`, actorID).Scan(&state.detailAudits); err != nil {
		t.Fatal(err)
	}
	return state
}

func TestR2RecoveryRotationAuditRetainsExactTransition(t *testing.T) {
	s := openOnboardingStore(t)
	owner, err := s.CreateSelfServiceOrbit("Rotation audit")
	if err != nil {
		t.Fatal(err)
	}
	oldID := owner.RecoveryID
	rotated, err := s.RotateRecovery(owner.ActorID, owner.ControlToken)
	if err != nil {
		t.Fatal(err)
	}
	var eventType string
	var oldDetail sql.NullString
	var newDetail string
	if err := s.db.QueryRow(`SELECT a.type, d.old_recovery_id, d.new_recovery_id
FROM audit_events a
JOIN recovery_rotation_audit_details d ON d.audit_event_id = a.id
WHERE a.actor_id = ? AND a.type = 'recovery.rotated'`, owner.ActorID).Scan(
		&eventType, &oldDetail, &newDetail); err != nil {
		t.Fatal(err)
	}
	if eventType != "recovery.rotated" || !oldDetail.Valid || oldDetail.String != oldID || newDetail != rotated.RecoveryID {
		t.Fatalf("rotation detail type=%q old_valid=%v old_matches=%v new_matches=%v",
			eventType, oldDetail.Valid, oldDetail.String == oldID, newDetail == rotated.RecoveryID)
	}

	credentialMaterial := []string{
		owner.NodeToken, owner.ControlToken, owner.RecoverySecret, rotated.RecoverySecret,
		hashToken(owner.NodeToken), hashToken(owner.ControlToken), hashToken(owner.RecoverySecret), hashToken(rotated.RecoverySecret),
	}
	for _, material := range credentialMaterial {
		var count int
		if err := s.db.QueryRow(`SELECT
  (SELECT COUNT(*) FROM audit_events
   WHERE type = ? OR CAST(orbit_id AS TEXT) = ? OR CAST(actor_id AS TEXT) = ?) +
  (SELECT COUNT(*) FROM recovery_rotation_audit_details
   WHERE old_recovery_id = ? OR new_recovery_id = ?)`,
			material, material, material, material, material).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatal("credential material entered a recovery audit table")
		}
	}
}

func TestR2FirstRecoveryRotationRecordsNullPriorGeneration(t *testing.T) {
	s := openOnboardingStore(t)
	owner, err := s.CreateSelfServiceOrbit("First rotation")
	if err != nil {
		t.Fatal(err)
	}
	invite, err := s.IssueDeviceInvite(owner.ActorID, owner.ControlToken, "companion")
	if err != nil {
		t.Fatal(err)
	}
	joined, err := s.ConsumeDeviceInvite(invite.Code)
	if err != nil {
		t.Fatal(err)
	}
	before := loadRecoveryAuditState(t, s, joined.ActorID)
	if before.recoveryID.Valid || before.recoveryHash.Valid {
		t.Fatal("invite-joined installation unexpectedly had recovery material")
	}
	rotated, err := s.RotateRecovery(joined.ActorID, joined.ControlToken)
	if err != nil {
		t.Fatal(err)
	}
	var oldDetail sql.NullString
	var newDetail string
	if err := s.db.QueryRow(`SELECT d.old_recovery_id, d.new_recovery_id
FROM recovery_rotation_audit_details d
JOIN audit_events a ON a.id = d.audit_event_id
WHERE a.actor_id = ? AND a.type = 'recovery.rotated'`, joined.ActorID).Scan(&oldDetail, &newDetail); err != nil {
		t.Fatal(err)
	}
	if oldDetail.Valid || newDetail != rotated.RecoveryID {
		t.Fatalf("first rotation old_valid=%v new_matches=%v", oldDetail.Valid, newDetail == rotated.RecoveryID)
	}
}

func TestR2RecoveryRotationAuditFailuresRollbackCredential(t *testing.T) {
	for _, test := range []struct {
		name    string
		trigger string
	}{
		{
			name: "base_audit",
			trigger: `CREATE TRIGGER fail_rotation_base_audit
BEFORE INSERT ON audit_events
WHEN NEW.type = 'recovery.rotated'
BEGIN SELECT RAISE(ABORT, 'rotation base audit unavailable'); END`,
		},
		{
			name: "detail_audit",
			trigger: `CREATE TRIGGER fail_rotation_detail_audit
BEFORE INSERT ON recovery_rotation_audit_details
BEGIN SELECT RAISE(ABORT, 'rotation detail audit unavailable'); END`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := openOnboardingStore(t)
			owner, err := s.CreateSelfServiceOrbit("Rotation rollback")
			if err != nil {
				t.Fatal(err)
			}
			before := loadRecoveryAuditState(t, s, owner.ActorID)
			if _, err := s.db.Exec(test.trigger); err != nil {
				t.Fatal(err)
			}
			result, err := s.RotateRecovery(owner.ActorID, owner.ControlToken)
			if err == nil {
				t.Fatal("rotation unexpectedly succeeded with unavailable audit persistence")
			}
			if result != (RecoveryRotationResult{}) {
				t.Fatal("failed rotation returned recovery material")
			}
			after := loadRecoveryAuditState(t, s, owner.ActorID)
			if after != before {
				t.Fatalf("rotation audit failure changed credential/audit state: before=%+v after=%+v", before, after)
			}
			for _, material := range []string{owner.NodeToken, owner.ControlToken, owner.RecoverySecret,
				hashToken(owner.NodeToken), hashToken(owner.ControlToken), hashToken(owner.RecoverySecret)} {
				if strings.Contains(err.Error(), material) {
					t.Fatal("failed rotation error leaked credential material")
				}
			}
		})
	}
}

func TestR2RecoveryRotationCollisionRetriesWithoutExtraAudit(t *testing.T) {
	s := openOnboardingStore(t)
	owner, err := s.CreateSelfServiceOrbit("Collision owner")
	if err != nil {
		t.Fatal(err)
	}
	other, err := s.CreateSelfServiceOrbit("Collision source")
	if err != nil {
		t.Fatal(err)
	}
	uniqueID, uniqueSecret, err := newRecoveryMaterial()
	if err != nil {
		t.Fatal(err)
	}
	collisionSecret := runtimeTestHumanSecret(t)
	calls := 0
	rotated, err := s.rotateRecoveryWithGenerator(owner.ActorID, owner.ControlToken, func() (string, string, error) {
		calls++
		if calls == 1 {
			return other.RecoveryID, collisionSecret, nil
		}
		return uniqueID, uniqueSecret, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls != 2 || rotated.RecoveryID != uniqueID || rotated.RecoverySecret != uniqueSecret {
		t.Fatalf("collision retry calls=%d id_matches=%v secret_matches=%v", calls,
			rotated.RecoveryID == uniqueID, rotated.RecoverySecret == uniqueSecret)
	}
	state := loadRecoveryAuditState(t, s, owner.ActorID)
	if state.rotationAudits != 1 || state.detailAudits != 1 {
		t.Fatalf("collision retry audit counts base=%d detail=%d", state.rotationAudits, state.detailAudits)
	}
}

func TestR2RateLimitAuditRepositoryHashesSubjectsAndPreservesNullableScope(t *testing.T) {
	s := openOnboardingStore(t)
	owner, err := s.CreateSelfServiceOrbit("Rate audit")
	if err != nil {
		t.Fatal(err)
	}
	sourceSubject := "198.51.100.27"
	if err := s.RecordRateLimitAudit(RateLimitCreateSourceIP, sourceSubject, RateLimitAuditScope{}); err != nil {
		t.Fatal(err)
	}
	orbitID, actorID := owner.OrbitID, owner.ActorID
	actorSubject := "actor-rate-subject"
	if err := s.RecordRateLimitAudit(RateLimitRecoveryRotateActor, actorSubject,
		RateLimitAuditScope{OrbitID: &orbitID, ActorID: &actorID}); err != nil {
		t.Fatal(err)
	}
	rows, err := s.db.Query(`SELECT event_type, limiter_class, subject_digest, orbit_id, actor_id, created_at
FROM rate_limit_audit_events ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	type auditRow struct {
		eventType string
		class     string
		digest    string
		orbit     sql.NullInt64
		actor     sql.NullInt64
		createdAt int64
	}
	var got []auditRow
	for rows.Next() {
		var row auditRow
		if err := rows.Scan(&row.eventType, &row.class, &row.digest, &row.orbit, &row.actor, &row.createdAt); err != nil {
			t.Fatal(err)
		}
		got = append(got, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("rate-limit audit rows=%d", len(got))
	}
	latestSaneTimestamp := time.Now().Add(time.Minute).UnixMilli()
	if got[0].eventType != "security.rate_limited" || got[0].createdAt <= 0 || got[0].createdAt > latestSaneTimestamp ||
		got[0].class != string(RateLimitCreateSourceIP) ||
		got[0].digest != rateLimitSubjectDigest(RateLimitCreateSourceIP, sourceSubject) ||
		got[0].orbit.Valid || got[0].actor.Valid {
		t.Fatalf("unscoped audit row=%+v", got[0])
	}
	if got[1].eventType != "security.rate_limited" || got[1].createdAt <= 0 || got[1].createdAt > latestSaneTimestamp ||
		got[1].class != string(RateLimitRecoveryRotateActor) ||
		got[1].digest != rateLimitSubjectDigest(RateLimitRecoveryRotateActor, actorSubject) ||
		got[1].orbit.Int64 != owner.OrbitID || got[1].actor.Int64 != owner.ActorID {
		t.Fatalf("scoped audit row=%+v", got[1])
	}
	for _, subject := range []string{sourceSubject, actorSubject} {
		var count int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM rate_limit_audit_events
WHERE limiter_class = ? OR subject_digest = ?`, subject, subject).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatal("raw limiter subject entered durable audit persistence")
		}
	}

	other, err := s.CreateSelfServiceOrbit("Different rate scope")
	if err != nil {
		t.Fatal(err)
	}
	fabricatedOrbit, fabricatedActor := int64(987654321), int64(987654322)
	invalid := []struct {
		name  string
		class RateLimitAuditClass
		scope RateLimitAuditScope
	}{
		{name: "partial_orbit", class: RateLimitRecoveryRotateActor, scope: RateLimitAuditScope{OrbitID: &orbitID}},
		{name: "partial_actor", class: RateLimitRecoveryRotateActor, scope: RateLimitAuditScope{ActorID: &actorID}},
		{name: "scoped_pre_identity_class", class: RateLimitCreateSourceIP, scope: RateLimitAuditScope{OrbitID: &orbitID, ActorID: &actorID}},
		{name: "unscoped_actor_class", class: RateLimitRecoveryRotateActor, scope: RateLimitAuditScope{}},
		{name: "fabricated_coordinates", class: RateLimitRecoveryRotateActor, scope: RateLimitAuditScope{OrbitID: &fabricatedOrbit, ActorID: &fabricatedActor}},
		{name: "mismatched_real_coordinates", class: RateLimitRecoveryRotateActor, scope: RateLimitAuditScope{OrbitID: &other.OrbitID, ActorID: &actorID}},
	}
	for _, test := range invalid {
		t.Run(test.name, func(t *testing.T) {
			if err := s.RecordRateLimitAudit(test.class, "invalid-scope-subject", test.scope); err == nil {
				t.Fatal("invalid rate-limit audit scope unexpectedly persisted")
			}
		})
	}
	directInvalid := []struct {
		name    string
		class   RateLimitAuditClass
		orbitID any
		actorID any
	}{
		{name: "partial_pre_identity_scope", class: RateLimitCreateSourceIP, orbitID: orbitID},
		{name: "scoped_pre_identity_class", class: RateLimitCreateSourceIP, orbitID: orbitID, actorID: actorID},
		{name: "unscoped_actor_class", class: RateLimitRecoveryRotateActor},
	}
	for _, test := range directInvalid {
		t.Run("schema_"+test.name, func(t *testing.T) {
			if _, err := s.db.Exec(`INSERT INTO rate_limit_audit_events
  (event_type, limiter_class, subject_digest, orbit_id, actor_id, created_at)
VALUES('security.rate_limited', ?, ?, ?, ?, ?)`, string(test.class),
				rateLimitSubjectDigest(test.class, "schema-invalid-"+test.name), test.orbitID, test.actorID,
				time.Now().UnixMilli()); err == nil {
				t.Fatal("schema accepted an invalid class/scope shape")
			}
			var count int
			if err := s.db.QueryRow(`SELECT COUNT(*) FROM rate_limit_audit_events`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count != 2 {
				t.Fatalf("rejected direct insert changed durable row count to %d", count)
			}
		})
	}
	var finalCount int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM rate_limit_audit_events`).Scan(&finalCount); err != nil {
		t.Fatal(err)
	}
	if finalCount != 2 {
		t.Fatalf("invalid rate-limit scopes changed durable row count to %d", finalCount)
	}
}

func TestR2AppFirstAlignmentViolationQuarantinesAndRefusesServing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "alignment.db")
	s, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := s.CreateSelfServiceOrbit("Aligned source")
	if err != nil {
		t.Fatal(err)
	}
	legacyOrbit, err := s.CreateOrbit("Rollback marker", 7_001_337)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.SetMemberName(legacyOrbit.ID, 7_001_337, "source-name"); err != nil {
		t.Fatal(err)
	}
	var legacyActorID int64
	if err := s.db.QueryRow(`SELECT id FROM actors
WHERE kind = 'telegram_user' AND external_ref = ?`, "7001337").Scan(&legacyActorID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE actors SET display_name = 'stale-name' WHERE id = ?`, legacyActorID); err != nil {
		t.Fatal(err)
	}
	before := loadRecoveryAuditState(t, s, owner.ActorID)
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	inspect, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(ON)&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	var foreignOrbit int64
	res, err := inspect.Exec(`INSERT INTO orbits(title, created_at, status)
VALUES('Foreign membership', ?, 'active')`, time.Now().UnixMilli())
	if err != nil {
		inspect.Close()
		t.Fatal(err)
	}
	foreignOrbit, err = res.LastInsertId()
	if err != nil {
		inspect.Close()
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	if _, err := inspect.Exec(`UPDATE memberships SET left_at = ?
WHERE actor_id = ? AND orbit_id = ?`, now, owner.ActorID, owner.OrbitID); err != nil {
		inspect.Close()
		t.Fatal(err)
	}
	if _, err := inspect.Exec(`INSERT INTO memberships(orbit_id, actor_id, role, joined_at, left_at)
VALUES(?, ?, 'companion', ?, NULL)`, foreignOrbit, owner.ActorID, now); err != nil {
		inspect.Close()
		t.Fatal(err)
	}
	tx, err := inspect.Begin()
	if err != nil {
		inspect.Close()
		t.Fatal(err)
	}
	if err := assertIdentityServingGate(tx); !errors.Is(err, ErrIdentityAlignmentViolation) {
		_ = tx.Rollback()
		inspect.Close()
		t.Fatalf("independent serving gate error=%v", err)
	}
	_ = tx.Rollback()
	if err := inspect.Close(); err != nil {
		t.Fatal(err)
	}

	opened, openErr := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if opened != nil {
		_ = opened.Close()
		t.Fatal("misaligned startup returned a serving store")
	}
	if !errors.Is(openErr, ErrIdentityAlignmentViolation) {
		t.Fatalf("misaligned startup error=%v", openErr)
	}
	for _, material := range []string{owner.NodeToken, owner.ControlToken, owner.RecoverySecret,
		hashToken(owner.NodeToken), hashToken(owner.ControlToken), hashToken(owner.RecoverySecret)} {
		if strings.Contains(openErr.Error(), material) {
			t.Fatal("alignment startup error leaked credential material")
		}
	}

	inspect, err = sql.Open("sqlite", path+"?_pragma=foreign_keys(ON)&_txlock=immediate")
	if err != nil {
		t.Fatal(err)
	}
	defer inspect.Close()
	var revokedAt sql.NullInt64
	if err := inspect.QueryRow(`SELECT revoked_at FROM actors WHERE id = ?`, owner.ActorID).Scan(&revokedAt); err != nil || !revokedAt.Valid {
		t.Fatalf("quarantined actor revoked_at=%v err=%v", revokedAt, err)
	}
	var auditCount int
	if err := inspect.QueryRow(`SELECT COUNT(*) FROM audit_events
WHERE orbit_id = ? AND actor_id = ? AND type = 'identity.alignment_quarantined'`,
		owner.OrbitID, owner.ActorID).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("alignment quarantine audit count=%d err=%v", auditCount, err)
	}
	var staleDisplay string
	if err := inspect.QueryRow(`SELECT display_name FROM actors WHERE id = ?`, legacyActorID).Scan(&staleDisplay); err != nil || staleDisplay != "stale-name" {
		t.Fatalf("ordinary reconciliation did not roll back display=%q err=%v", staleDisplay, err)
	}
	var after recoveryAuditState
	if err := inspect.QueryRow(`SELECT ic.recovery_id, ic.recovery_secret_hash,
       ic.consumed_at, ic.control_token_hash, sl.token_hash
FROM installation_credentials ic
JOIN slots sl ON sl.orbit_id = ic.slot_orbit_id AND sl.slot = ic.slot_name
WHERE ic.actor_id = ?`, owner.ActorID).Scan(&after.recoveryID, &after.recoveryHash,
		&after.consumedAt, &after.controlHash, &after.nodeHash); err != nil {
		t.Fatal(err)
	}
	if after.recoveryID != before.recoveryID || after.recoveryHash != before.recoveryHash ||
		after.consumedAt != before.consumedAt || after.controlHash != before.controlHash || after.nodeHash != before.nodeHash {
		t.Fatal("alignment quarantine changed credential material")
	}
	if err := foreignKeyCheck(inspect); err != nil {
		t.Fatal(err)
	}

	second, secondErr := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if second != nil {
		_ = second.Close()
		t.Fatal("second misaligned startup returned a serving store")
	}
	if secondErr == nil {
		t.Fatal("second startup succeeded before explicit repair")
	}

	repairAt := time.Now().UnixMilli()
	if _, err := inspect.Exec(`UPDATE memberships SET left_at = ?
WHERE actor_id = ? AND orbit_id = ?`, repairAt, owner.ActorID, foreignOrbit); err != nil {
		t.Fatal(err)
	}
	if _, err := inspect.Exec(`UPDATE memberships SET left_at = NULL
WHERE actor_id = ? AND orbit_id = ?`, owner.ActorID, owner.OrbitID); err != nil {
		t.Fatal(err)
	}
	if _, err := inspect.Exec(`UPDATE actors SET revoked_at = NULL WHERE id = ?`, owner.ActorID); err != nil {
		t.Fatal(err)
	}
	if err := inspect.Close(); err != nil {
		t.Fatal(err)
	}

	repaired, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer repaired.Close()
	ctx, err := repaired.ResolveTokenActorContext(owner.ControlToken)
	if err != nil || ctx.ActorID != owner.ActorID || ctx.OrbitID != owner.OrbitID || ctx.Role != "primary" {
		t.Fatalf("repaired app-first context actor=%d orbit=%d role=%q err=%v", ctx.ActorID, ctx.OrbitID, ctx.Role, err)
	}
}

func TestR2AppFirstMissingMembershipIsQuarantined(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing-membership.db")
	s, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := s.CreateSelfServiceOrbit("Missing membership")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE memberships SET left_at = ?
WHERE actor_id = ? AND orbit_id = ?`, time.Now().UnixMilli(), owner.ActorID, owner.OrbitID); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	opened, openErr := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if opened != nil {
		_ = opened.Close()
		t.Fatal("missing-membership startup returned a serving store")
	}
	if !errors.Is(openErr, ErrIdentityAlignmentViolation) {
		t.Fatalf("missing-membership startup error=%v", openErr)
	}
	inspect, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	defer inspect.Close()
	var revokedAt sql.NullInt64
	var audits int
	if err := inspect.QueryRow(`SELECT revoked_at FROM actors WHERE id = ?`, owner.ActorID).Scan(&revokedAt); err != nil || !revokedAt.Valid {
		t.Fatalf("missing-membership actor revoked_at=%v err=%v", revokedAt, err)
	}
	if err := inspect.QueryRow(`SELECT COUNT(*) FROM audit_events
WHERE actor_id = ? AND type = 'identity.alignment_quarantined'`, owner.ActorID).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("missing-membership quarantine audits=%d err=%v", audits, err)
	}
}

func TestR2AlignmentQuarantineFailureStillRefusesServing(t *testing.T) {
	path := filepath.Join(t.TempDir(), "quarantine-failure.db")
	s, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	owner, err := s.CreateSelfServiceOrbit("Quarantine failure")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE memberships SET left_at = ?
WHERE actor_id = ? AND orbit_id = ?`, time.Now().UnixMilli(), owner.ActorID, owner.OrbitID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`CREATE TRIGGER fail_alignment_quarantine_audit
BEFORE INSERT ON audit_events
WHEN NEW.type = 'identity.alignment_quarantined'
BEGIN SELECT RAISE(ABORT, 'alignment audit unavailable'); END`); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	opened, openErr := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if opened != nil {
		_ = opened.Close()
		t.Fatal("quarantine audit failure returned a serving store")
	}
	if !errors.Is(openErr, ErrIdentityAlignmentViolation) ||
		!strings.Contains(openErr.Error(), "alignment audit unavailable") {
		t.Fatalf("quarantine audit failure did not preserve both errors: %v", openErr)
	}
	for _, material := range []string{owner.NodeToken, owner.ControlToken, owner.RecoverySecret,
		hashToken(owner.NodeToken), hashToken(owner.ControlToken), hashToken(owner.RecoverySecret)} {
		if strings.Contains(openErr.Error(), material) {
			t.Fatal("quarantine failure error leaked credential material")
		}
	}

	inspect, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(ON)")
	if err != nil {
		t.Fatal(err)
	}
	defer inspect.Close()
	var revokedAt sql.NullInt64
	if err := inspect.QueryRow(`SELECT revoked_at FROM actors WHERE id = ?`, owner.ActorID).Scan(&revokedAt); err != nil {
		t.Fatal(err)
	}
	if revokedAt.Valid {
		t.Fatal("failed quarantine committed actor revocation without its audit row")
	}
	var audits int
	if err := inspect.QueryRow(`SELECT COUNT(*) FROM audit_events
WHERE actor_id = ? AND type = 'identity.alignment_quarantined'`, owner.ActorID).Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if audits != 0 {
		t.Fatalf("failed quarantine audit rows=%d", audits)
	}
}
