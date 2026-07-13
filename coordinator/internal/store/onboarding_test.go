package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type forbiddenQueryRow struct{}

func (forbiddenQueryRow) QueryRow(string, ...any) *sql.Row {
	panic("invalid Telegram principal reached identity storage")
}

func openOnboardingStore(t *testing.T) *Store {
	t.Helper()
	s, err := OpenWithOptions(filepath.Join(t.TempDir(), "onboarding.db"), Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func runtimeTestToken(t *testing.T) string {
	t.Helper()
	token, err := randomHexSecret(32)
	if err != nil {
		t.Fatal(err)
	}
	return token
}

func runtimeTestHumanSecret(t *testing.T) string {
	t.Helper()
	secret, err := generateSecret(onboardingSecretLength)
	if err != nil {
		t.Fatal(err)
	}
	return secret
}

func TestCreateSelfServiceOrbitMintsSeparatedHashOnlyCredentials(t *testing.T) {
	s := openOnboardingStore(t)
	created, err := s.CreateSelfServiceOrbit("  Home  ")
	if err != nil {
		t.Fatal(err)
	}
	if created.OrbitTitle != "Home" || created.Role != "primary" || created.Slot != "a" {
		t.Fatal("create returned incorrect non-secret orbit metadata")
	}
	if !lowerHexTokenPattern.MatchString(created.NodeToken) || !lowerHexTokenPattern.MatchString(created.ControlToken) || created.NodeToken == created.ControlToken {
		t.Fatal("node/control credentials are not separate 256-bit lowercase hex tokens")
	}
	if !recoveryIDPattern.MatchString(created.RecoveryID) || !humanSecretPattern.MatchString(created.RecoverySecret) {
		t.Fatal("recovery material does not satisfy frozen format")
	}

	var nodeHash, controlHash, recoveryHash, recoveryID, role string
	if err := s.db.QueryRow(`SELECT sl.token_hash, ic.control_token_hash, ic.recovery_secret_hash,
       ic.recovery_id, m.role
FROM installation_credentials ic
JOIN slots sl ON sl.orbit_id = ic.slot_orbit_id AND sl.slot = ic.slot_name
JOIN memberships m ON m.actor_id = ic.actor_id AND m.orbit_id = ic.slot_orbit_id
WHERE ic.actor_id = ?`, created.ActorID).Scan(&nodeHash, &controlHash, &recoveryHash, &recoveryID, &role); err != nil {
		t.Fatal(err)
	}
	if nodeHash != hashToken(created.NodeToken) || controlHash != hashToken(created.ControlToken) ||
		recoveryHash != hashToken(created.RecoverySecret) || recoveryID != created.RecoveryID || role != "primary" {
		t.Fatal("stored hashes or identity projection do not match returned material")
	}
	for name, plaintext := range map[string]string{
		"node": created.NodeToken, "control": created.ControlToken, "recovery": created.RecoverySecret,
	} {
		var count int
		if err := s.db.QueryRow(`SELECT
  (SELECT COUNT(*) FROM slots WHERE token_hash = ?) +
  (SELECT COUNT(*) FROM installation_credentials
   WHERE control_token_hash = ? OR recovery_secret_hash = ?) +
  (SELECT COUNT(*) FROM audit_events WHERE type = ?)`, plaintext, plaintext, plaintext, plaintext).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("plaintext %s credential entered persistent storage", name)
		}
	}
	var audits int
	if err := s.db.QueryRow(`SELECT COUNT(*) FROM audit_events
WHERE orbit_id = ? AND actor_id = ? AND type = 'onboarding.orbit_created'`, created.OrbitID, created.ActorID).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("create audit count=%d err=%v", audits, err)
	}

	nodeCtx, err := s.ResolveTokenActorContext(created.NodeToken)
	if err != nil || nodeCtx.Capabilities != CapabilityNode {
		t.Fatalf("node context mismatch: actor=%d role=%q capability=%d err=%v", nodeCtx.ActorID, nodeCtx.Role, nodeCtx.Capabilities, err)
	}
	controlCtx, err := s.ResolveTokenActorContext(created.ControlToken)
	if err != nil || !controlCtx.Capabilities.Has(CapabilityControl) {
		t.Fatalf("control context mismatch: actor=%d role=%q capability=%d err=%v", controlCtx.ActorID, controlCtx.Role, controlCtx.Capabilities, err)
	}
	if err := s.ReconcileIdentity(); err != nil {
		t.Fatal(err)
	}
	if err := s.ReconcileIdentity(); err != nil {
		t.Fatal(err)
	}
	controlCtx, err = s.ResolveTokenActorContext(created.ControlToken)
	if err != nil || controlCtx.Role != "primary" {
		t.Fatalf("app-first primary was not preserved: actor=%d role=%q err=%v", controlCtx.ActorID, controlCtx.Role, err)
	}
}

func TestCreateSelfServiceOrbitRollbackIncludesAuditAndSecrets(t *testing.T) {
	s := openOnboardingStore(t)
	s.testCheckpoint = func(name string) error {
		if name == "onboarding_create_before_commit" {
			return errors.New("injected create failure")
		}
		return nil
	}
	if _, err := s.CreateSelfServiceOrbit("Rollback"); err == nil {
		t.Fatal("expected injected failure")
	}
	for _, table := range []string{"orbits", "actors", "memberships", "slots", "installation_credentials", "audit_events"} {
		var count int
		if err := s.db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("%s has %d rows after rollback", table, count)
		}
	}
}

func TestOnboardingMutationsRollbackWhenAuditBoundaryFails(t *testing.T) {
	t.Run("device_invite_issue", func(t *testing.T) {
		s := openOnboardingStore(t)
		owner, err := s.CreateSelfServiceOrbit("Issue rollback")
		if err != nil {
			t.Fatal(err)
		}
		s.testCheckpoint = func(name string) error {
			if name == "device_invite_issue_before_audit" {
				return errors.New("injected audit failure")
			}
			return nil
		}
		if _, err := s.IssueDeviceInvite(owner.ActorID, owner.ControlToken, "companion"); err == nil {
			t.Fatal("expected issue failure")
		}
		var invites int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM device_invites`).Scan(&invites); err != nil || invites != 0 {
			t.Fatalf("invite rollback count=%d err=%v", invites, err)
		}
	})
	t.Run("device_invite_consume", func(t *testing.T) {
		s := openOnboardingStore(t)
		owner, err := s.CreateSelfServiceOrbit("Consume rollback")
		if err != nil {
			t.Fatal(err)
		}
		invite, err := s.IssueDeviceInvite(owner.ActorID, owner.ControlToken, "companion")
		if err != nil {
			t.Fatal(err)
		}
		before := onboardingRowCounts(t, s)
		s.testCheckpoint = func(name string) error {
			if name == "device_invite_consume_before_audit" {
				return errors.New("injected audit failure")
			}
			return nil
		}
		if _, err := s.ConsumeDeviceInvite(invite.Code); err == nil {
			t.Fatal("expected consume failure")
		}
		if after := onboardingRowCounts(t, s); after != before {
			t.Fatalf("consume rollback changed row counts: before=%v after=%v", before, after)
		}
		var consumedAt sql.NullInt64
		if err := s.db.QueryRow(`SELECT consumed_at FROM device_invites WHERE code_hash = ?`,
			hashToken(invite.Code)).Scan(&consumedAt); err != nil {
			t.Fatal(err)
		}
		if consumedAt.Valid {
			t.Fatal("consume rollback burned invite")
		}
	})
	t.Run("recovery_consume", func(t *testing.T) {
		s := openOnboardingStore(t)
		owner, err := s.CreateSelfServiceOrbit("Recovery rollback")
		if err != nil {
			t.Fatal(err)
		}
		s.testCheckpoint = func(name string) error {
			if name == "recovery_after_consume" {
				return errors.New("injected audit failure")
			}
			return nil
		}
		replacement := runtimeTestToken(t)
		if _, err := s.ConsumeRecovery(owner.RecoveryID, owner.RecoverySecret, replacement); err == nil {
			t.Fatal("expected recovery failure")
		}
		if _, err := s.ResolveTokenActorContext(owner.ControlToken); err != nil {
			t.Fatalf("old control did not survive rollback: %v", err)
		}
		if _, err := s.ResolveTokenActorContext(replacement); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("replacement survived rollback: %v", err)
		}
		var audits int
		if err := s.db.QueryRow(`SELECT COUNT(*) FROM audit_events WHERE type = 'recovery.consumed'`).Scan(&audits); err != nil || audits != 0 {
			t.Fatalf("recovery rollback audit count=%d err=%v", audits, err)
		}
	})
	t.Run("recovery_rotate", func(t *testing.T) {
		s := openOnboardingStore(t)
		owner, err := s.CreateSelfServiceOrbit("Rotate rollback")
		if err != nil {
			t.Fatal(err)
		}
		s.testCheckpoint = func(name string) error {
			if name == "recovery_rotate_before_audit" {
				return errors.New("injected audit failure")
			}
			return nil
		}
		if _, err := s.RotateRecovery(owner.ActorID, owner.ControlToken); err == nil {
			t.Fatal("expected rotation failure")
		}
		var recoveryID string
		if err := s.db.QueryRow(`SELECT recovery_id FROM installation_credentials WHERE actor_id = ?`, owner.ActorID).Scan(&recoveryID); err != nil {
			t.Fatal(err)
		}
		if recoveryID != owner.RecoveryID {
			t.Fatal("rotation generation survived rollback")
		}
	})
	t.Run("telegram_link_issue", func(t *testing.T) {
		s := openOnboardingStore(t)
		owner, err := s.CreateSelfServiceOrbit("Link rollback")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.IssueTelegramLink(owner.ActorID, owner.ControlToken, "companion"); err != nil {
			t.Fatal(err)
		}
		s.testCheckpoint = func(name string) error {
			if name == "telegram_link_issue_before_audit" {
				return errors.New("injected audit failure")
			}
			return nil
		}
		if _, err := s.IssueTelegramLink(owner.ActorID, owner.ControlToken, "companion"); err == nil {
			t.Fatal("expected link issue failure")
		}
		var links, invalidated int
		if err := s.db.QueryRow(`SELECT COUNT(*), COUNT(invalidated_at) FROM telegram_link_codes`).Scan(&links, &invalidated); err != nil || links != 1 || invalidated != 0 {
			t.Fatalf("link rollback count=%d err=%v", links, err)
		}
	})
}

func TestAuthenticatedMutationsPrepareDigestsBeforeWriterTransaction(t *testing.T) {
	for _, test := range []struct {
		name             string
		expectedSequence []string
		run              func(*Store, OnboardingCredentials) error
		wantInsufficient bool
	}{
		{
			name: "node_invite_negative",
			expectedSequence: []string{
				"device_invite_bearer_prepared", "device_invite_material_prepared",
				"device_invite_issue_before_begin", "device_invite_transaction_started",
			},
			run: func(s *Store, owner OnboardingCredentials) error {
				_, err := s.IssueDeviceInvite(owner.ActorID, owner.NodeToken, "companion")
				return err
			},
			wantInsufficient: true,
		},
		{
			name: "recovery_rotation",
			expectedSequence: []string{
				"recovery_rotate_bearer_prepared", "recovery_rotate_material_prepared",
				"recovery_rotate_before_begin", "recovery_rotate_transaction_started",
			},
			run: func(s *Store, owner OnboardingCredentials) error {
				_, err := s.RotateRecovery(owner.ActorID, owner.ControlToken)
				return err
			},
		},
		{
			name: "telegram_link_issue",
			expectedSequence: []string{
				"telegram_link_bearer_prepared", "telegram_link_material_prepared",
				"telegram_link_issue_before_begin", "telegram_link_transaction_started",
			},
			run: func(s *Store, owner OnboardingCredentials) error {
				_, err := s.IssueTelegramLink(owner.ActorID, owner.ControlToken, "companion")
				return err
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := openOnboardingStore(t)
			owner, err := s.CreateSelfServiceOrbit("Digest ordering")
			if err != nil {
				t.Fatal(err)
			}
			wanted := make(map[string]bool, len(test.expectedSequence))
			for _, checkpoint := range test.expectedSequence {
				wanted[checkpoint] = true
			}
			var sequence []string
			s.testCheckpoint = func(checkpoint string) error {
				if wanted[checkpoint] {
					sequence = append(sequence, checkpoint)
				}
				return nil
			}
			err = test.run(s, owner)
			if test.wantInsufficient {
				if !errors.Is(err, ErrInsufficientCapability) {
					t.Fatalf("node mutation error=%v", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if len(sequence) != len(test.expectedSequence) {
				t.Fatalf("checkpoint count=%d want=%d", len(sequence), len(test.expectedSequence))
			}
			for i := range sequence {
				if sequence[i] != test.expectedSequence[i] {
					t.Fatalf("checkpoint[%d]=%q want=%q", i, sequence[i], test.expectedSequence[i])
				}
			}
			if test.wantInsufficient {
				var invites int
				if err := s.db.QueryRow(`SELECT COUNT(*) FROM device_invites`).Scan(&invites); err != nil || invites != 0 {
					t.Fatalf("node mutation invite rows=%d err=%v", invites, err)
				}
			}
		})
	}
}

func TestDeviceInviteCapabilityLifecycleAndOneTimeConsume(t *testing.T) {
	s := openOnboardingStore(t)
	owner, err := s.CreateSelfServiceOrbit("Invite")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.IssueDeviceInvite(owner.ActorID, owner.NodeToken, "companion"); !errors.Is(err, ErrInsufficientCapability) {
		t.Fatalf("node token issue error=%v", err)
	}
	issued, err := s.IssueDeviceInvite(owner.ActorID, owner.ControlToken, "companion")
	if err != nil {
		t.Fatal(err)
	}
	var persistedCode, role string
	if err := s.db.QueryRow(`SELECT code_hash, intended_role FROM device_invites`).Scan(&persistedCode, &role); err != nil {
		t.Fatal(err)
	}
	if persistedCode != hashToken(issued.Code) || persistedCode == issued.Code || role != "companion" {
		t.Fatal("device invite was not persisted hash-only")
	}
	joined, err := s.ConsumeDeviceInvite(issued.Code)
	if err != nil {
		t.Fatal(err)
	}
	if joined.Role != "companion" || joined.Slot != "b" || joined.NodeToken == joined.ControlToken ||
		joined.RecoveryID != "" || joined.RecoverySecret != "" {
		t.Fatal("join returned incorrect metadata or credential separation")
	}
	var recoveryID, recoveryHash *string
	if err := s.db.QueryRow(`SELECT recovery_id, recovery_secret_hash
FROM installation_credentials WHERE actor_id = ?`, joined.ActorID).Scan(&recoveryID, &recoveryHash); err != nil {
		t.Fatal(err)
	}
	if recoveryID != nil || recoveryHash != nil {
		t.Fatal("join returned or persisted recovery material without deliberate rotation")
	}
	if err := s.ReconcileIdentity(); err != nil {
		t.Fatal(err)
	}
	if ctx, err := s.ResolveTokenActorContext(joined.ControlToken); err != nil || ctx.Role != "companion" {
		t.Fatalf("app-first joined role was not preserved: actor=%d role=%q err=%v", ctx.ActorID, ctx.Role, err)
	}
	if _, err := s.ConsumeDeviceInvite(issued.Code); !errors.Is(err, ErrCredentialInvalid) {
		t.Fatalf("replay error=%v", err)
	}
	if _, err := s.IssueDeviceInvite(joined.ActorID, joined.ControlToken, "satellite"); err != nil {
		t.Fatalf("companion control should issue invite: %v", err)
	}
	if _, err := s.db.Exec(`UPDATE memberships SET role = 'satellite' WHERE actor_id = ?`, joined.ActorID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.IssueDeviceInvite(joined.ActorID, joined.ControlToken, "companion"); !errors.Is(err, ErrInsufficientCapability) {
		t.Fatalf("satellite issue error=%v", err)
	}
}

func TestDeviceInviteExpiredAndIssuerInvalidatedAreUniform(t *testing.T) {
	t.Run("expired", func(t *testing.T) {
		s := openOnboardingStore(t)
		owner, err := s.CreateSelfServiceOrbit("Expired")
		if err != nil {
			t.Fatal(err)
		}
		invite, err := s.IssueDeviceInvite(owner.ActorID, owner.ControlToken, "companion")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.consumeDeviceInviteAt(invite.Code, invite.ExpiresAt.Add(time.Millisecond).UnixMilli()); !errors.Is(err, ErrCredentialInvalid) {
			t.Fatalf("expired invite error=%v", err)
		}
	})
	t.Run("issuer_lost_authority", func(t *testing.T) {
		s := openOnboardingStore(t)
		owner, err := s.CreateSelfServiceOrbit("Invalidated")
		if err != nil {
			t.Fatal(err)
		}
		invite, err := s.IssueDeviceInvite(owner.ActorID, owner.ControlToken, "companion")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`UPDATE memberships SET left_at = 1 WHERE actor_id = ?`, owner.ActorID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ConsumeDeviceInvite(invite.Code); !errors.Is(err, ErrCredentialInvalid) {
			t.Fatalf("invalidated issuer invite error=%v", err)
		}
		var consumed any
		if err := s.db.QueryRow(`SELECT consumed_at FROM device_invites WHERE code_hash = ?`, hashToken(invite.Code)).Scan(&consumed); err != nil {
			t.Fatal(err)
		}
		if consumed != nil {
			t.Fatal("issuer-invalidated invite was burned on failed consume")
		}
	})
	t.Run("issuer_stale_binding", func(t *testing.T) {
		s := openOnboardingStore(t)
		owner, err := s.CreateSelfServiceOrbit("Stale issuer")
		if err != nil {
			t.Fatal(err)
		}
		invite, err := s.IssueDeviceInvite(owner.ActorID, owner.ControlToken, "companion")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`UPDATE slots SET token_hash = ? WHERE orbit_id = ? AND slot = ?`, strings.Repeat("d", 64), owner.OrbitID, owner.Slot); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ConsumeDeviceInvite(invite.Code); !errors.Is(err, ErrCredentialInvalid) {
			t.Fatalf("stale issuer invite error=%v", err)
		}
	})
}

func TestDeviceInviteInvalidIssuerMatrixHasFixedValidationAndNoSideEffects(t *testing.T) {
	type mutation struct {
		name string
		sql  string
		args func(OnboardingCredentials) []any
	}
	tests := []mutation{
		{"revoked_actor", `UPDATE actors SET revoked_at = 1 WHERE id = ?`, func(owner OnboardingCredentials) []any { return []any{owner.ActorID} }},
		{"left_membership", `UPDATE memberships SET left_at = 1 WHERE actor_id = ?`, func(owner OnboardingCredentials) []any { return []any{owner.ActorID} }},
		{"downgraded_role", `UPDATE memberships SET role = 'satellite' WHERE actor_id = ?`, func(owner OnboardingCredentials) []any { return []any{owner.ActorID} }},
		{"disabled_orbit", `UPDATE orbits SET status = 'disabled' WHERE id = ?`, func(owner OnboardingCredentials) []any { return []any{owner.OrbitID} }},
		{"revoked_slot", `UPDATE slots SET revoked_at = 1 WHERE orbit_id = ? AND slot = ?`, func(owner OnboardingCredentials) []any { return []any{owner.OrbitID, owner.Slot} }},
		{"stale_token_hash", `UPDATE slots SET token_hash = ? WHERE orbit_id = ? AND slot = ?`, func(owner OnboardingCredentials) []any {
			return []any{strings.Repeat("e", 64), owner.OrbitID, owner.Slot}
		}},
		{"stale_paired_generation", `UPDATE slots SET paired_at = paired_at + 1 WHERE orbit_id = ? AND slot = ?`, func(owner OnboardingCredentials) []any { return []any{owner.OrbitID, owner.Slot} }},
		{"missing_credential", `DELETE FROM installation_credentials WHERE actor_id = ?`, func(owner OnboardingCredentials) []any { return []any{owner.ActorID} }},
		{"missing_control_lifecycle", `UPDATE installation_credentials SET control_token_hash = NULL WHERE actor_id = ?`, func(owner OnboardingCredentials) []any { return []any{owner.ActorID} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := openOnboardingStore(t)
			owner, err := s.CreateSelfServiceOrbit("Invalid issuer")
			if err != nil {
				t.Fatal(err)
			}
			invite, err := s.IssueDeviceInvite(owner.ActorID, owner.ControlToken, "companion")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.db.Exec(test.sql, test.args(owner)...); err != nil {
				t.Fatal(err)
			}
			before := onboardingRowCounts(t, s)
			queryChecks, hashChecks := 0, 0
			s.testCheckpoint = func(name string) error {
				switch name {
				case "device_invite_validation_query":
					queryChecks++
				case "device_invite_validation_hash":
					hashChecks++
				}
				return nil
			}
			if _, err := s.ConsumeDeviceInvite(invite.Code); !errors.Is(err, ErrCredentialInvalid) {
				t.Fatalf("invalid issuer consume error=%v", err)
			}
			if queryChecks != 1 || hashChecks != 1 {
				t.Fatalf("validation operations query=%d hash=%d", queryChecks, hashChecks)
			}
			if after := onboardingRowCounts(t, s); after != before {
				t.Fatalf("failed consume changed row counts: before=%v after=%v", before, after)
			}
			var consumed any
			if err := s.db.QueryRow(`SELECT consumed_at FROM device_invites WHERE code_hash = ?`, hashToken(invite.Code)).Scan(&consumed); err != nil {
				t.Fatal(err)
			}
			if consumed != nil {
				t.Fatal("invalid issuer consume burned invite")
			}
		})
	}
}

func TestDeviceInviteUniformFailuresUseOneValidationReadAndHash(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*testing.T, *Store) (string, int64)
	}{
		{"unknown", func(t *testing.T, _ *Store) (string, int64) {
			return runtimeTestHumanSecret(t), time.Now().UnixMilli()
		}},
		{"expired", func(t *testing.T, s *Store) (string, int64) {
			owner, err := s.CreateSelfServiceOrbit("Expired structural")
			if err != nil {
				t.Fatal(err)
			}
			invite, err := s.IssueDeviceInvite(owner.ActorID, owner.ControlToken, "companion")
			if err != nil {
				t.Fatal(err)
			}
			return invite.Code, invite.ExpiresAt.Add(time.Millisecond).UnixMilli()
		}},
		{"consumed", func(t *testing.T, s *Store) (string, int64) {
			owner, err := s.CreateSelfServiceOrbit("Consumed structural")
			if err != nil {
				t.Fatal(err)
			}
			invite, err := s.IssueDeviceInvite(owner.ActorID, owner.ControlToken, "companion")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.ConsumeDeviceInvite(invite.Code); err != nil {
				t.Fatal(err)
			}
			return invite.Code, time.Now().UnixMilli()
		}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			s := openOnboardingStore(t)
			code, now := test.setup(t, s)
			queryChecks, hashChecks := 0, 0
			s.testCheckpoint = func(name string) error {
				if name == "device_invite_validation_query" {
					queryChecks++
				}
				if name == "device_invite_validation_hash" {
					hashChecks++
				}
				return nil
			}
			if _, err := s.consumeDeviceInviteAt(code, now); !errors.Is(err, ErrCredentialInvalid) {
				t.Fatalf("uniform failure error=%v", err)
			}
			if queryChecks != 1 || hashChecks != 1 {
				t.Fatalf("validation operations query=%d hash=%d", queryChecks, hashChecks)
			}
		})
	}
}

func TestDeviceInviteNullableIssuerGenerationAndFullOrbit(t *testing.T) {
	t.Run("nullable_zero_generation", func(t *testing.T) {
		s := openOnboardingStore(t)
		owner, err := s.CreateSelfServiceOrbit("Nullable issuer")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`UPDATE slots SET paired_at = NULL WHERE orbit_id = ? AND slot = ?`, owner.OrbitID, owner.Slot); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`UPDATE installation_credentials SET slot_paired_at = 0 WHERE actor_id = ?`, owner.ActorID); err != nil {
			t.Fatal(err)
		}
		invite, err := s.IssueDeviceInvite(owner.ActorID, owner.ControlToken, "companion")
		if err != nil {
			t.Fatal(err)
		}
		joined, err := s.ConsumeDeviceInvite(invite.Code)
		if err != nil || joined.Role != "companion" || joined.RecoveryID != "" || joined.RecoverySecret != "" {
			t.Fatalf("nullable issuer join metadata mismatch: role=%q recovery_returned=%t err=%v",
				joined.Role, joined.RecoveryID != "" || joined.RecoverySecret != "", err)
		}
	})
	t.Run("full_orbit", func(t *testing.T) {
		s := openOnboardingStore(t)
		owner, err := s.CreateSelfServiceOrbit("Full")
		if err != nil {
			t.Fatal(err)
		}
		invite, err := s.IssueDeviceInvite(owner.ActorID, owner.ControlToken, "companion")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`UPDATE orbits SET max_pulsars = 1 WHERE id = ?`, owner.OrbitID); err != nil {
			t.Fatal(err)
		}
		before := onboardingRowCounts(t, s)
		queryChecks, hashChecks := 0, 0
		s.testCheckpoint = func(name string) error {
			if name == "device_invite_validation_query" {
				queryChecks++
			}
			if name == "device_invite_validation_hash" {
				hashChecks++
			}
			return nil
		}
		if _, err := s.ConsumeDeviceInvite(invite.Code); !errors.Is(err, ErrCredentialInvalid) {
			t.Fatalf("full-orbit consume error=%v", err)
		}
		if queryChecks != 1 || hashChecks != 1 || onboardingRowCounts(t, s) != before {
			t.Fatalf("full-orbit path query=%d hash=%d changed_rows=%t", queryChecks, hashChecks, onboardingRowCounts(t, s) != before)
		}
		var consumed any
		if err := s.db.QueryRow(`SELECT consumed_at FROM device_invites WHERE code_hash = ?`, hashToken(invite.Code)).Scan(&consumed); err != nil {
			t.Fatal(err)
		}
		if consumed != nil {
			t.Fatal("full-orbit credential was consumed")
		}
	})
}

func TestDeviceInviteReusesRevokedSlotAndRetiresStaleCredential(t *testing.T) {
	for _, featureOffRevoke := range []bool{false, true} {
		name := "feature_on_revoke"
		if featureOffRevoke {
			name = "stale_feature_off_credential"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "slot-reuse.db")
			s, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
			if err != nil {
				t.Fatal(err)
			}
			defer s.Close()
			owner, err := s.CreateSelfServiceOrbit("Slot reuse")
			if err != nil {
				t.Fatal(err)
			}
			firstInvite, err := s.IssueDeviceInvite(owner.ActorID, owner.ControlToken, "companion")
			if err != nil {
				t.Fatal(err)
			}
			oldInstallation, err := s.ConsumeDeviceInvite(firstInvite.Code)
			if err != nil || oldInstallation.Slot != "b" {
				t.Fatalf("initial join slot=%q err=%v", oldInstallation.Slot, err)
			}
			if featureOffRevoke {
				legacy, err := Open(path)
				if err != nil {
					t.Fatal(err)
				}
				found, revokeErr := legacy.RevokeSlot(owner.OrbitID, oldInstallation.Slot)
				if closeErr := legacy.Close(); closeErr != nil {
					t.Fatal(closeErr)
				}
				if revokeErr != nil || !found {
					t.Fatalf("feature-off revoke found=%t err=%v", found, revokeErr)
				}
				var staleCredentials int
				if err := s.db.QueryRow(`SELECT COUNT(*) FROM installation_credentials WHERE actor_id = ?`,
					oldInstallation.ActorID).Scan(&staleCredentials); err != nil || staleCredentials != 1 {
					t.Fatalf("stale credential rows=%d err=%v", staleCredentials, err)
				}
			} else {
				found, err := s.RevokeSlot(owner.OrbitID, oldInstallation.Slot)
				if err != nil || !found {
					t.Fatalf("feature-on revoke found=%t err=%v", found, err)
				}
			}

			secondInvite, err := s.IssueDeviceInvite(owner.ActorID, owner.ControlToken, "satellite")
			if err != nil {
				t.Fatal(err)
			}
			joined, err := s.ConsumeDeviceInvite(secondInvite.Code)
			if err != nil || joined.Slot != oldInstallation.Slot || joined.ActorID == oldInstallation.ActorID || joined.Role != "satellite" {
				t.Fatalf("reused join slot=%q actor_reused=%t role=%q err=%v",
					joined.Slot, joined.ActorID == oldInstallation.ActorID, joined.Role, err)
			}
			if _, err := s.ResolveTokenActorContext(oldInstallation.NodeToken); !errors.Is(err, ErrUnauthorized) {
				t.Fatalf("old reused-slot node credential error=%v", err)
			}
			if _, err := s.ResolveTokenActorContext(oldInstallation.ControlToken); !errors.Is(err, ErrUnauthorized) {
				t.Fatalf("old reused-slot control credential error=%v", err)
			}
			if ctx, err := s.ResolveTokenActorContext(joined.NodeToken); err != nil || ctx.ActorID != joined.ActorID || ctx.Role != "satellite" {
				t.Fatalf("new reused-slot context actor=%d role=%q err=%v", ctx.ActorID, ctx.Role, err)
			}
			var revokedAt, leftAt sql.NullInt64
			var oldCredentials int
			if err := s.db.QueryRow(`SELECT a.revoked_at, m.left_at,
  (SELECT COUNT(*) FROM installation_credentials WHERE actor_id = a.id)
FROM actors a JOIN memberships m ON m.actor_id = a.id AND m.orbit_id = ?
WHERE a.id = ?`, owner.OrbitID, oldInstallation.ActorID).Scan(&revokedAt, &leftAt, &oldCredentials); err != nil {
				t.Fatal(err)
			}
			if !revokedAt.Valid || !leftAt.Valid || oldCredentials != 0 {
				t.Fatalf("stale owner retirement revoked=%t left=%t credential_rows=%d",
					revokedAt.Valid, leftAt.Valid, oldCredentials)
			}
			var activeSlots, pairedBy int
			var slotRevoked sql.NullInt64
			if err := s.db.QueryRow(`SELECT
  (SELECT COUNT(*) FROM slots WHERE orbit_id = ? AND revoked_at IS NULL),
  paired_by, revoked_at
FROM slots WHERE orbit_id = ? AND slot = ?`, owner.OrbitID, owner.OrbitID, joined.Slot).Scan(
				&activeSlots, &pairedBy, &slotRevoked); err != nil {
				t.Fatal(err)
			}
			if activeSlots != 2 || pairedBy != 0 || slotRevoked.Valid {
				t.Fatalf("reused slot state active=%d paired_by=%d revoked=%t", activeSlots, pairedBy, slotRevoked.Valid)
			}
		})
	}
}

func onboardingRowCounts(t *testing.T, s *Store) [4]int {
	t.Helper()
	var counts [4]int
	if err := s.db.QueryRow(`SELECT
  (SELECT COUNT(*) FROM actors),
  (SELECT COUNT(*) FROM slots),
  (SELECT COUNT(*) FROM memberships),
  (SELECT COUNT(*) FROM audit_events)`).Scan(&counts[0], &counts[1], &counts[2], &counts[3]); err != nil {
		t.Fatal(err)
	}
	return counts
}

func TestConcurrentDeviceInviteConsumeHasOneWinner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "invite-race.db")
	s1, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s1.Close()
	owner, err := s1.CreateSelfServiceOrbit("Race")
	if err != nil {
		t.Fatal(err)
	}
	invite, err := s1.IssueDeviceInvite(owner.ActorID, owner.ControlToken, "companion")
	if err != nil {
		t.Fatal(err)
	}
	s2, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	entered := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	s1.testCheckpoint = func(name string) error {
		if name == "device_invite_after_reserve" {
			once.Do(func() { close(entered) })
			<-release
		}
		return nil
	}
	type outcome struct {
		value OnboardingCredentials
		err   error
	}
	results := make(chan outcome, 2)
	go func() { value, err := s1.ConsumeDeviceInvite(invite.Code); results <- outcome{value, err} }()
	<-entered
	secondAttempted := make(chan struct{})
	var secondAttemptOnce sync.Once
	s2.testCheckpoint = func(name string) error {
		if name == "device_invite_consume_before_begin" {
			secondAttemptOnce.Do(func() { close(secondAttempted) })
		}
		return nil
	}
	go func() {
		value, err := s2.ConsumeDeviceInvite(invite.Code)
		results <- outcome{value, err}
	}()
	<-secondAttempted
	select {
	case <-results:
		t.Fatal("second writer passed BEGIN IMMEDIATE while first consume was paused")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	winners, losers := 0, 0
	for i := 0; i < 2; i++ {
		result := <-results
		if result.err == nil {
			winners++
		} else if errors.Is(result.err, ErrCredentialInvalid) {
			losers++
		} else {
			t.Fatalf("unexpected race error: %v", result.err)
		}
	}
	if winners != 1 || losers != 1 {
		t.Fatalf("winners=%d losers=%d", winners, losers)
	}
}

func TestRecoveryConsumeReplayRotateAndNodePreservation(t *testing.T) {
	s := openOnboardingStore(t)
	created, err := s.CreateSelfServiceOrbit("Recover")
	if err != nil {
		t.Fatal(err)
	}
	replacement := runtimeTestToken(t)
	result, err := s.ConsumeRecovery(created.RecoveryID, created.RecoverySecret, replacement)
	if err != nil || result.ActorID != created.ActorID || result.OrbitID != created.OrbitID || result.Role != "primary" {
		t.Fatalf("recovery returned incorrect non-secret context: err=%v", err)
	}
	if _, err := s.ResolveTokenActorContext(created.ControlToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("old control token survived: %v", err)
	}
	if ctx, err := s.ResolveTokenActorContext(replacement); err != nil || ctx.ActorID != created.ActorID {
		t.Fatalf("replacement control context mismatch: actor=%d role=%q err=%v", ctx.ActorID, ctx.Role, err)
	}
	if ctx, err := s.ResolveTokenActorContext(created.NodeToken); err != nil || ctx.ActorID != created.ActorID {
		t.Fatalf("node token changed during recovery: actor=%d role=%q err=%v", ctx.ActorID, ctx.Role, err)
	}
	if _, err := s.ConsumeRecovery(created.RecoveryID, created.RecoverySecret, replacement); err != nil {
		t.Fatalf("same tuple must be idempotent: %v", err)
	}
	if _, err := s.ConsumeRecovery(created.RecoveryID, created.RecoverySecret, runtimeTestToken(t)); !errors.Is(err, ErrCredentialInvalid) {
		t.Fatalf("different tuple replay error=%v", err)
	}
	rotated, err := s.RotateRecovery(created.ActorID, replacement)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.ActorID != created.ActorID || rotated.RecoveryID == created.RecoveryID || !humanSecretPattern.MatchString(rotated.RecoverySecret) {
		t.Fatal("rotation returned incorrect actor, generation, or secret format")
	}
	if _, err := s.ConsumeRecovery(created.RecoveryID, created.RecoverySecret, replacement); !errors.Is(err, ErrCredentialInvalid) {
		t.Fatalf("old generation replay after rotate error=%v", err)
	}
	if ctx, err := s.ResolveTokenActorContext(replacement); err != nil || ctx.ActorID != created.ActorID {
		t.Fatalf("rotation changed control token: actor=%d role=%q err=%v", ctx.ActorID, ctx.Role, err)
	}
}

func TestConcurrentRecoverySameTupleIsIdempotentWithOneAudit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "recovery-race.db")
	s1, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s1.Close()
	created, err := s1.CreateSelfServiceOrbit("Race")
	if err != nil {
		t.Fatal(err)
	}
	s2, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()
	entered := make(chan struct{})
	release := make(chan struct{})
	s1.testCheckpoint = func(name string) error {
		if name == "recovery_after_consume" {
			close(entered)
			<-release
		}
		return nil
	}
	replacement := runtimeTestToken(t)
	errs := make(chan error, 2)
	go func() {
		_, err := s1.ConsumeRecovery(created.RecoveryID, created.RecoverySecret, replacement)
		errs <- err
	}()
	<-entered
	secondAttempted := make(chan struct{})
	var secondAttemptOnce sync.Once
	s2.testCheckpoint = func(name string) error {
		if name == "recovery_consume_before_begin" {
			secondAttemptOnce.Do(func() { close(secondAttempted) })
		}
		return nil
	}
	go func() {
		_, err := s2.ConsumeRecovery(created.RecoveryID, created.RecoverySecret, replacement)
		errs <- err
	}()
	<-secondAttempted
	select {
	case <-errs:
		t.Fatal("second recovery passed writer lock before first commit")
	case <-time.After(50 * time.Millisecond):
	}
	close(release)
	for i := 0; i < 2; i++ {
		if err := <-errs; err != nil {
			t.Fatalf("same-tuple concurrent recovery: %v", err)
		}
	}
	var audits int
	if err := s1.db.QueryRow(`SELECT COUNT(*) FROM audit_events
WHERE actor_id = ? AND type = 'recovery.consumed'`, created.ActorID).Scan(&audits); err != nil || audits != 1 {
		t.Fatalf("recovery audit count=%d err=%v", audits, err)
	}
}

func TestRecoveryRotationConsumeSerialization(t *testing.T) {
	t.Run("rotation_commits_first", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "rotate-first.db")
		s1, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
		if err != nil {
			t.Fatal(err)
		}
		defer s1.Close()
		created, err := s1.CreateSelfServiceOrbit("Rotate first")
		if err != nil {
			t.Fatal(err)
		}
		s2, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
		if err != nil {
			t.Fatal(err)
		}
		defer s2.Close()
		entered := make(chan struct{})
		release := make(chan struct{})
		s1.testCheckpoint = func(name string) error {
			if name == "recovery_rotate_after_auth" {
				close(entered)
				<-release
			}
			return nil
		}
		rotateErr := make(chan error, 1)
		consumeErr := make(chan error, 1)
		replacement := runtimeTestToken(t)
		go func() { _, err := s1.RotateRecovery(created.ActorID, created.ControlToken); rotateErr <- err }()
		<-entered
		consumeAttempted := make(chan struct{})
		var consumeAttemptOnce sync.Once
		s2.testCheckpoint = func(name string) error {
			if name == "recovery_consume_before_begin" {
				consumeAttemptOnce.Do(func() { close(consumeAttempted) })
			}
			return nil
		}
		go func() {
			_, err := s2.ConsumeRecovery(created.RecoveryID, created.RecoverySecret, replacement)
			consumeErr <- err
		}()
		<-consumeAttempted
		select {
		case <-consumeErr:
			t.Fatal("recovery consume passed rotation writer lock")
		case <-time.After(50 * time.Millisecond):
		}
		close(release)
		if err := <-rotateErr; err != nil {
			t.Fatalf("rotation failed: %v", err)
		}
		if err := <-consumeErr; !errors.Is(err, ErrCredentialInvalid) {
			t.Fatalf("old recovery generation after rotation error=%v", err)
		}
	})
	t.Run("consume_commits_first", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "consume-first.db")
		s1, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
		if err != nil {
			t.Fatal(err)
		}
		defer s1.Close()
		created, err := s1.CreateSelfServiceOrbit("Consume first")
		if err != nil {
			t.Fatal(err)
		}
		s2, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
		if err != nil {
			t.Fatal(err)
		}
		defer s2.Close()
		entered := make(chan struct{})
		release := make(chan struct{})
		s1.testCheckpoint = func(name string) error {
			if name == "recovery_after_consume" {
				close(entered)
				<-release
			}
			return nil
		}
		replacement := runtimeTestToken(t)
		consumeErr := make(chan error, 1)
		rotateErr := make(chan error, 1)
		go func() {
			_, err := s1.ConsumeRecovery(created.RecoveryID, created.RecoverySecret, replacement)
			consumeErr <- err
		}()
		<-entered
		rotateAttempted := make(chan struct{})
		var rotateAttemptOnce sync.Once
		s2.testCheckpoint = func(name string) error {
			if name == "recovery_rotate_before_begin" {
				rotateAttemptOnce.Do(func() { close(rotateAttempted) })
			}
			return nil
		}
		go func() {
			_, err := s2.RotateRecovery(created.ActorID, replacement)
			rotateErr <- err
		}()
		<-rotateAttempted
		select {
		case <-rotateErr:
			t.Fatal("recovery rotation passed consume writer lock")
		case <-time.After(50 * time.Millisecond):
		}
		close(release)
		if err := <-consumeErr; err != nil {
			t.Fatalf("consume failed: %v", err)
		}
		if err := <-rotateErr; err != nil {
			t.Fatalf("post-consume rotation failed: %v", err)
		}
	})
}

func TestRecoveryAndAuthenticatedMutationsRejectInvalidLifecycle(t *testing.T) {
	for _, test := range []struct {
		name   string
		mutate string
	}{
		{"revoked", `UPDATE actors SET revoked_at = 1 WHERE id = ?`},
		{"left", `UPDATE memberships SET left_at = 1 WHERE actor_id = ?`},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := openOnboardingStore(t)
			created, err := s.CreateSelfServiceOrbit(test.name)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.db.Exec(test.mutate, created.ActorID); err != nil {
				t.Fatal(err)
			}
			if _, err := s.ConsumeRecovery(created.RecoveryID, created.RecoverySecret, runtimeTestToken(t)); !errors.Is(err, ErrCredentialInvalid) {
				t.Fatalf("recovery lifecycle error=%v", err)
			}
			if _, err := s.RotateRecovery(created.ActorID, created.ControlToken); test.name == "revoked" {
				if !errors.Is(err, ErrUnauthorized) {
					t.Fatalf("revoked rotate error=%v", err)
				}
			} else if !errors.Is(err, ErrInsufficientCapability) {
				t.Fatalf("limited rotate error=%v", err)
			}
		})
	}
	t.Run("disabled", func(t *testing.T) {
		s := openOnboardingStore(t)
		created, err := s.CreateSelfServiceOrbit("disabled")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`UPDATE orbits SET status = 'disabled' WHERE id = ?`, created.OrbitID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ConsumeRecovery(created.RecoveryID, created.RecoverySecret, runtimeTestToken(t)); !errors.Is(err, ErrCredentialInvalid) {
			t.Fatalf("disabled recovery error=%v", err)
		}
		if _, err := s.RotateRecovery(created.ActorID, created.ControlToken); !errors.Is(err, ErrInsufficientCapability) {
			t.Fatalf("disabled rotate error=%v", err)
		}
	})
	t.Run("stale_binding", func(t *testing.T) {
		s := openOnboardingStore(t)
		created, err := s.CreateSelfServiceOrbit("stale")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`UPDATE slots SET token_hash = ? WHERE orbit_id = ? AND slot = ?`, strings.Repeat("b", 64), created.OrbitID, created.Slot); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ConsumeRecovery(created.RecoveryID, created.RecoverySecret, runtimeTestToken(t)); !errors.Is(err, ErrCredentialInvalid) {
			t.Fatalf("stale-binding recovery error=%v", err)
		}
		if _, err := s.RotateRecovery(created.ActorID, created.ControlToken); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("stale-binding rotate error=%v", err)
		}
	})
}

func TestSatelliteRecoveryRestoresControlWithoutAuthorityEscalation(t *testing.T) {
	s := openOnboardingStore(t)
	created, err := s.CreateSelfServiceOrbit("Satellite recovery")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.db.Exec(`UPDATE memberships SET role = 'satellite' WHERE actor_id = ?`, created.ActorID); err != nil {
		t.Fatal(err)
	}
	replacement := runtimeTestToken(t)
	result, err := s.ConsumeRecovery(created.RecoveryID, created.RecoverySecret, replacement)
	if err != nil || result.ActorID != created.ActorID || result.OrbitID != created.OrbitID || result.Role != "satellite" {
		t.Fatalf("satellite recovery context actor=%d orbit=%d role=%q err=%v", result.ActorID, result.OrbitID, result.Role, err)
	}
	if _, err := s.ResolveTokenActorContext(created.ControlToken); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("old satellite control credential survived: %v", err)
	}
	if ctx, err := s.ResolveTokenActorContext(created.NodeToken); err != nil || ctx.ActorID != created.ActorID ||
		ctx.Role != "satellite" || ctx.Capabilities != CapabilityNode {
		t.Fatalf("satellite node context actor=%d role=%q capability=%d err=%v", ctx.ActorID, ctx.Role, ctx.Capabilities, err)
	}
	if ctx, err := s.ResolveTokenActorContext(replacement); err != nil || ctx.ActorID != created.ActorID ||
		ctx.Role != "satellite" || ctx.Capabilities != CapabilityNode|CapabilityControl {
		t.Fatalf("satellite replacement context actor=%d role=%q capability=%d err=%v", ctx.ActorID, ctx.Role, ctx.Capabilities, err)
	}
	if _, err := s.RotateRecovery(created.ActorID, replacement); !errors.Is(err, ErrInsufficientCapability) {
		t.Fatalf("satellite recovery rotation error=%v", err)
	}
	if _, err := s.IssueDeviceInvite(created.ActorID, replacement, "companion"); !errors.Is(err, ErrInsufficientCapability) {
		t.Fatalf("satellite invite issue error=%v", err)
	}
	if _, err := s.IssueTelegramLink(created.ActorID, replacement, "companion"); !errors.Is(err, ErrInsufficientCapability) {
		t.Fatalf("satellite Telegram-link issue error=%v", err)
	}
	var role string
	if err := s.db.QueryRow(`SELECT role FROM memberships WHERE actor_id = ? AND orbit_id = ?`,
		created.ActorID, created.OrbitID).Scan(&role); err != nil {
		t.Fatal(err)
	}
	if role != "satellite" {
		t.Fatalf("recovery changed membership role=%q", role)
	}
}

func TestRecoveryValidationUsesOneReadAndOneSubmittedSecretHash(t *testing.T) {
	for _, name := range []string{
		"unknown", "wrong_secret", "revoked", "left", "disabled", "stale_binding", "stale_generation", "valid", "consumed_same_tuple",
	} {
		t.Run(name, func(t *testing.T) {
			s := openOnboardingStore(t)
			recoveryID := "rec_" + runtimeTestToken(t)[:32]
			recoverySecret := runtimeTestHumanSecret(t)
			replacement := runtimeTestToken(t)
			wantRealTarget := false
			wantSuccess := false
			if name != "unknown" {
				created, err := s.CreateSelfServiceOrbit("Recovery validation")
				if err != nil {
					t.Fatal(err)
				}
				recoveryID, recoverySecret = created.RecoveryID, created.RecoverySecret
				switch name {
				case "wrong_secret":
					recoverySecret = runtimeTestHumanSecret(t)
					wantRealTarget = true
				case "revoked":
					_, err = s.db.Exec(`UPDATE actors SET revoked_at = 1 WHERE id = ?`, created.ActorID)
				case "left":
					_, err = s.db.Exec(`UPDATE memberships SET left_at = 1 WHERE actor_id = ?`, created.ActorID)
				case "disabled":
					_, err = s.db.Exec(`UPDATE orbits SET status = 'disabled' WHERE id = ?`, created.OrbitID)
				case "stale_binding":
					_, err = s.db.Exec(`UPDATE slots SET token_hash = ? WHERE orbit_id = ? AND slot = ?`,
						strings.Repeat("7", 64), created.OrbitID, created.Slot)
				case "stale_generation":
					_, err = s.db.Exec(`UPDATE slots SET paired_at = paired_at + 1 WHERE orbit_id = ? AND slot = ?`,
						created.OrbitID, created.Slot)
				case "valid":
					wantRealTarget, wantSuccess = true, true
				case "consumed_same_tuple":
					if _, err = s.ConsumeRecovery(recoveryID, recoverySecret, replacement); err == nil {
						wantRealTarget, wantSuccess = true, true
					}
				}
				if err != nil {
					t.Fatal(err)
				}
			}

			queryChecks, hashChecks, realTargets, dummyTargets := 0, 0, 0, 0
			s.testCheckpoint = func(checkpoint string) error {
				switch checkpoint {
				case "recovery_validation_query":
					queryChecks++
				case "recovery_validation_hash":
					hashChecks++
				case "recovery_validation_real_target":
					realTargets++
				case "recovery_validation_dummy_target":
					dummyTargets++
				}
				return nil
			}
			_, err := s.ConsumeRecovery(recoveryID, recoverySecret, replacement)
			if wantSuccess {
				if err != nil {
					t.Fatalf("valid recovery path error=%v", err)
				}
			} else if !errors.Is(err, ErrCredentialInvalid) {
				t.Fatalf("uniform recovery failure error=%v", err)
			}
			wantReal, wantDummy := 0, 1
			if wantRealTarget {
				wantReal, wantDummy = 1, 0
			}
			if queryChecks != 1 || hashChecks != 1 || realTargets != wantReal || dummyTargets != wantDummy {
				t.Fatalf("validation shape query=%d hash=%d real=%d dummy=%d", queryChecks, hashChecks, realTargets, dummyTargets)
			}
		})
	}
}

func TestTelegramLinkIssueInvalidatesPriorAndPersistsHashOnly(t *testing.T) {
	s := openOnboardingStore(t)
	created, err := s.CreateSelfServiceOrbit("Link")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.IssueTelegramLink(created.ActorID, created.NodeToken, "companion"); !errors.Is(err, ErrInsufficientCapability) {
		t.Fatalf("node link issue error=%v", err)
	}
	first, err := s.IssueTelegramLink(created.ActorID, created.ControlToken, "companion")
	if err != nil {
		t.Fatal(err)
	}
	second, err := s.IssueTelegramLink(created.ActorID, created.ControlToken, "satellite")
	if err != nil {
		t.Fatal(err)
	}
	if first.Code == second.Code || time.Until(second.ExpiresAt) < 14*time.Minute {
		t.Fatal("link issuance did not rotate the code or apply the expected expiry")
	}
	rows, err := s.db.Query(`SELECT code_hash, invalidated_at, desired_role
FROM telegram_link_codes ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var hashes []string
	var invalidated []*int64
	var roles []string
	for rows.Next() {
		var hash, role string
		var invalid *int64
		if err := rows.Scan(&hash, &invalid, &role); err != nil {
			t.Fatal(err)
		}
		hashes, invalidated, roles = append(hashes, hash), append(invalidated, invalid), append(roles, role)
	}
	if len(hashes) != 2 || hashes[0] != hashToken(first.Code) || hashes[1] != hashToken(second.Code) ||
		hashes[0] == first.Code || invalidated[0] == nil || invalidated[1] != nil || roles[1] != "satellite" {
		t.Fatalf("link hash-only persistence mismatch: rows=%d prior_invalidated=%t current_invalidated=%t",
			len(hashes), len(invalidated) > 0 && invalidated[0] != nil, len(invalidated) > 1 && invalidated[1] != nil)
	}
}

func TestOnboardingStoreFeatureOff(t *testing.T) {
	s, err := Open(filepath.Join(t.TempDir(), "off.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if _, err := s.CreateSelfServiceOrbit("off"); !errors.Is(err, ErrSelfServiceOnboardingDisabled) {
		t.Fatalf("create feature-off error=%v", err)
	}
	if _, err := s.ConsumeRecovery("rec_"+strings.Repeat("1", 32), runtimeTestHumanSecret(t), runtimeTestToken(t)); !errors.Is(err, ErrSelfServiceOnboardingDisabled) {
		t.Fatalf("recovery feature-off error=%v", err)
	}
}

func TestActorResolverPartialContextIsMinimalAndInvalidCredentialsAreZero(t *testing.T) {
	t.Run("unknown", func(t *testing.T) {
		s := openOnboardingStore(t)
		ctx, err := s.ResolveTokenActorContext(runtimeTestToken(t))
		if !errors.Is(err, ErrUnauthorized) || ctx != (ActorContext{}) {
			t.Fatalf("unknown credential context actor=%d orbit=%d capability=%d err=%v", ctx.ActorID, ctx.OrbitID, ctx.Capabilities, err)
		}
	})
	for _, test := range []struct {
		name     string
		mutation string
		args     func(OnboardingCredentials) []any
	}{
		{"revoked_actor", `UPDATE actors SET revoked_at = 1 WHERE id = ?`, func(owner OnboardingCredentials) []any { return []any{owner.ActorID} }},
		{"revoked_slot", `UPDATE slots SET revoked_at = 1 WHERE orbit_id = ? AND slot = ?`, func(owner OnboardingCredentials) []any { return []any{owner.OrbitID, owner.Slot} }},
		{"stale_binding", `UPDATE slots SET token_hash = ? WHERE orbit_id = ? AND slot = ?`, func(owner OnboardingCredentials) []any {
			return []any{strings.Repeat("f", 64), owner.OrbitID, owner.Slot}
		}},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := openOnboardingStore(t)
			owner, err := s.CreateSelfServiceOrbit("Invalid context")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.db.Exec(test.mutation, test.args(owner)...); err != nil {
				t.Fatal(err)
			}
			ctx, err := s.ResolveTokenActorContext(owner.ControlToken)
			if !errors.Is(err, ErrUnauthorized) || ctx != (ActorContext{}) {
				t.Fatalf("invalid context actor=%d orbit=%d capability=%d err=%v", ctx.ActorID, ctx.OrbitID, ctx.Capabilities, err)
			}
		})
	}
	for _, test := range []struct {
		name     string
		mutation string
		args     func(OnboardingCredentials) []any
	}{
		{"left", `UPDATE memberships SET left_at = 1 WHERE actor_id = ?`, func(owner OnboardingCredentials) []any { return []any{owner.ActorID} }},
		{"disabled", `UPDATE orbits SET status = 'disabled' WHERE id = ?`, func(owner OnboardingCredentials) []any { return []any{owner.OrbitID} }},
	} {
		t.Run(test.name, func(t *testing.T) {
			s := openOnboardingStore(t)
			owner, err := s.CreateSelfServiceOrbit("Limited context")
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.db.Exec(test.mutation, test.args(owner)...); err != nil {
				t.Fatal(err)
			}
			for _, credential := range []struct {
				token      string
				capability Capability
			}{{owner.NodeToken, CapabilityNode}, {owner.ControlToken, CapabilityNode | CapabilityControl}} {
				ctx, err := s.ResolveTokenActorContext(credential.token)
				if !errors.Is(err, ErrInsufficientCapability) || ctx.ActorID != owner.ActorID || ctx.OrbitID != owner.OrbitID ||
					ctx.Slot != owner.Slot || ctx.Capabilities != credential.capability || ctx.Role != "" {
					t.Fatalf("limited context mismatch actor=%d orbit=%d role=%q capability=%d err=%v",
						ctx.ActorID, ctx.OrbitID, ctx.Role, ctx.Capabilities, err)
				}
			}
		})
	}
	t.Run("satellite_mutation", func(t *testing.T) {
		s := openOnboardingStore(t)
		owner, err := s.CreateSelfServiceOrbit("Satellite context")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`UPDATE memberships SET role = 'satellite' WHERE actor_id = ?`, owner.ActorID); err != nil {
			t.Fatal(err)
		}
		probe, err := s.ResolveTokenActorContext(owner.ControlToken)
		if err != nil || probe.Role != "satellite" {
			t.Fatalf("satellite probe role=%q err=%v", probe.Role, err)
		}
		tx, err := s.db.Begin()
		if err != nil {
			t.Fatal(err)
		}
		defer tx.Rollback()
		ctx, err := mutationActorContextTx(tx, owner.ActorID, hashToken(owner.ControlToken))
		if !errors.Is(err, ErrInsufficientCapability) || ctx.ActorID != owner.ActorID || ctx.OrbitID != owner.OrbitID ||
			ctx.Slot != owner.Slot || ctx.Capabilities != CapabilityNode|CapabilityControl || ctx.Role != "" {
			t.Fatalf("satellite mutation context actor=%d orbit=%d role=%q capability=%d err=%v",
				ctx.ActorID, ctx.OrbitID, ctx.Role, ctx.Capabilities, err)
		}
	})
}

func TestActorResolverRejectsPairedGenerationMismatchForBothDomains(t *testing.T) {
	t.Run("non_null_generation", func(t *testing.T) {
		s := openOnboardingStore(t)
		owner, err := s.CreateSelfServiceOrbit("Generation")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`UPDATE slots SET paired_at = paired_at + 1 WHERE orbit_id = ? AND slot = ?`, owner.OrbitID, owner.Slot); err != nil {
			t.Fatal(err)
		}
		for _, token := range []string{owner.NodeToken, owner.ControlToken} {
			ctx, err := s.ResolveTokenActorContext(token)
			if !errors.Is(err, ErrUnauthorized) || ctx != (ActorContext{}) {
				t.Fatalf("stale generation context actor=%d orbit=%d capability=%d err=%v", ctx.ActorID, ctx.OrbitID, ctx.Capabilities, err)
			}
		}
	})
	t.Run("nullable_zero_sentinel", func(t *testing.T) {
		s := openOnboardingStore(t)
		owner, err := s.CreateSelfServiceOrbit("Nullable generation")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`UPDATE slots SET paired_at = NULL WHERE orbit_id = ? AND slot = ?`, owner.OrbitID, owner.Slot); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`UPDATE installation_credentials SET slot_paired_at = 0 WHERE actor_id = ?`, owner.ActorID); err != nil {
			t.Fatal(err)
		}
		for _, token := range []string{owner.NodeToken, owner.ControlToken} {
			ctx, err := s.ResolveTokenActorContext(token)
			if err != nil || ctx.ActorID != owner.ActorID {
				t.Fatalf("nullable generation context actor=%d capability=%d err=%v", ctx.ActorID, ctx.Capabilities, err)
			}
		}
		if _, err := s.db.Exec(`UPDATE slots SET paired_at = 1 WHERE orbit_id = ? AND slot = ?`, owner.OrbitID, owner.Slot); err != nil {
			t.Fatal(err)
		}
		for _, token := range []string{owner.NodeToken, owner.ControlToken} {
			ctx, err := s.ResolveTokenActorContext(token)
			if !errors.Is(err, ErrUnauthorized) || ctx != (ActorContext{}) {
				t.Fatalf("nullable stale generation actor=%d capability=%d err=%v", ctx.ActorID, ctx.Capabilities, err)
			}
		}
	})
}

func TestTelegramResolverClassifiesKnownLifecycleFailuresWithoutCapabilities(t *testing.T) {
	const telegramID int64 = 7001
	for _, invalidID := range []int64{0, -1, -7001} {
		ctx, err := resolveTelegramActorContext(forbiddenQueryRow{}, invalidID)
		if !errors.Is(err, ErrUnauthorized) || ctx != (ActorContext{}) {
			t.Fatalf("non-positive Telegram context actor=%d orbit=%d capability=%d err=%v",
				ctx.ActorID, ctx.OrbitID, ctx.Capabilities, err)
		}
	}
	t.Run("unknown", func(t *testing.T) {
		s := openOnboardingStore(t)
		ctx, err := s.ResolveTelegramActorContext(telegramID)
		if !errors.Is(err, ErrUnauthorized) || ctx != (ActorContext{}) {
			t.Fatalf("unknown Telegram context actor=%d orbit=%d capability=%d err=%v", ctx.ActorID, ctx.OrbitID, ctx.Capabilities, err)
		}
	})
	setup := func(t *testing.T) (*Store, *Orbit, ActorContext) {
		t.Helper()
		s := openOnboardingStore(t)
		orbit, err := s.CreateOrbit("Telegram lifecycle", telegramID)
		if err != nil {
			t.Fatal(err)
		}
		ctx, err := s.ResolveTelegramActorContext(telegramID)
		if err != nil || ctx.ActorID == 0 || ctx.OrbitID != orbit.ID || ctx.Role != "primary" || ctx.Capabilities != CapabilityTelegram {
			t.Fatalf("active Telegram context actor=%d orbit=%d role=%q capability=%d err=%v",
				ctx.ActorID, ctx.OrbitID, ctx.Role, ctx.Capabilities, err)
		}
		return s, orbit, ctx
	}
	t.Run("revoked", func(t *testing.T) {
		s, _, active := setup(t)
		if _, err := s.db.Exec(`UPDATE actors SET revoked_at = 1 WHERE id = ?`, active.ActorID); err != nil {
			t.Fatal(err)
		}
		ctx, err := s.ResolveTelegramActorContext(telegramID)
		if !errors.Is(err, ErrUnauthorized) || ctx.ActorID != active.ActorID || ctx.OrbitID != 0 || ctx.Role != "" || ctx.Capabilities != 0 {
			t.Fatalf("revoked Telegram context actor=%d orbit=%d role=%q capability=%d err=%v",
				ctx.ActorID, ctx.OrbitID, ctx.Role, ctx.Capabilities, err)
		}
	})
	t.Run("left", func(t *testing.T) {
		s, _, active := setup(t)
		if _, err := s.db.Exec(`UPDATE memberships SET left_at = 1 WHERE actor_id = ?`, active.ActorID); err != nil {
			t.Fatal(err)
		}
		ctx, err := s.ResolveTelegramActorContext(telegramID)
		if !errors.Is(err, ErrInsufficientCapability) || ctx.ActorID != active.ActorID || ctx.OrbitID != 0 || ctx.Role != "" || ctx.Capabilities != 0 {
			t.Fatalf("left Telegram context actor=%d orbit=%d role=%q capability=%d err=%v",
				ctx.ActorID, ctx.OrbitID, ctx.Role, ctx.Capabilities, err)
		}
	})
	t.Run("disabled", func(t *testing.T) {
		s, orbit, active := setup(t)
		if _, err := s.db.Exec(`UPDATE orbits SET status = 'disabled' WHERE id = ?`, orbit.ID); err != nil {
			t.Fatal(err)
		}
		ctx, err := s.ResolveTelegramActorContext(telegramID)
		if !errors.Is(err, ErrInsufficientCapability) || ctx.ActorID != active.ActorID || ctx.OrbitID != orbit.ID ||
			ctx.Role != "primary" || ctx.Capabilities != 0 {
			t.Fatalf("disabled Telegram context actor=%d orbit=%d role=%q capability=%d err=%v",
				ctx.ActorID, ctx.OrbitID, ctx.Role, ctx.Capabilities, err)
		}
	})
}
