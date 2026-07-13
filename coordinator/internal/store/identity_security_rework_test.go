package store

import (
	"database/sql"
	"errors"
	"path/filepath"
	"sync"
	"testing"
)

func provisionTestInstallation(t *testing.T, s *Store, orbitID, ownerID int64) (string, ActorContext) {
	t.Helper()
	_, nodeToken, err := s.PairSlot(orbitID, ownerID)
	if err != nil {
		t.Fatal(err)
	}
	ctx, err := s.ResolveTokenActorContext(nodeToken)
	if err != nil {
		t.Fatal(err)
	}
	return nodeToken, ctx
}

func newProvisioningMaterial(t *testing.T) (controlToken, recoveryID, recoverySecret string) {
	t.Helper()
	secret, err := generateSecret(27)
	if err != nil {
		t.Fatal(err)
	}
	return randomHex(32), "rec_" + randomHex(16), secret
}

// R1: deliberate same-value and cross-orbit capability-domain aliases must be
// rejected through the public provisioning API, then fail closed if an older
// binary nevertheless leaves an ambiguous database behind.
func TestR1CredentialDomainsRejectSameValueAndAmbiguousResolution(t *testing.T) {
	s := openIdentityTemp(t)
	orbitA, err := s.CreateOrbit("Domain A", 101)
	if err != nil {
		t.Fatal(err)
	}
	nodeA, installationA := provisionTestInstallation(t, s, orbitA.ID, 101)
	orbitB, err := s.CreateOrbit("Domain B", 202)
	if err != nil {
		t.Fatal(err)
	}
	_, installationB := provisionTestInstallation(t, s, orbitB.ID, 202)

	for name, target := range map[string]ActorContext{
		"same orbit":  installationA,
		"cross orbit": installationB,
	} {
		t.Run(name, func(t *testing.T) {
			_, recoveryID, recoverySecret := newProvisioningMaterial(t)
			authority := int64(101)
			if target.OrbitID == orbitB.ID {
				authority = 202
			}
			err := s.ProvisionInstallationSecrets(
				Identity{Kind: IdentityTelegram, TelegramUserID: authority},
				target.ActorID, nodeA, recoveryID, recoverySecret)
			if !errors.Is(err, ErrCredentialDomainConflict) {
				t.Fatalf("same-digest provisioning error = %v", err)
			}
		})
	}

	controlB, recoveryID, recoverySecret := newProvisioningMaterial(t)
	if err := s.ProvisionInstallationSecrets(
		Identity{Kind: IdentityTelegram, TelegramUserID: 202},
		installationB.ActorID, controlB, recoveryID, recoverySecret); err != nil {
		t.Fatal(err)
	}

	// Emulate a previous binary that knows only slots and happens to write the
	// already-live control digest into another orbit's node domain.
	if _, err := s.db.Exec(`UPDATE slots SET token_hash = ? WHERE orbit_id = ? AND slot = 'a'`,
		hashToken(controlB), orbitA.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ResolveTokenActorContext(controlB); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("ambiguous cross-domain resolver error = %v", err)
	}
	if _, _, ok, err := s.LookupPlaybackToken(controlB); err != nil || ok {
		t.Fatalf("ambiguous playback authorization ok=%v err=%v", ok, err)
	}
}

// R1: feature-off stays on the legacy node-only path, but re-enabling the new
// resolver on an old-binary cross-domain database fails at the serving gate.
func TestR1OldDatabaseCrossDomainCollisionFailsFeatureOn(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old-domain.db")
	s, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	orbitA, _ := s.CreateOrbit("Old A", 101)
	_, _ = provisionTestInstallation(t, s, orbitA.ID, 101)
	orbitB, _ := s.CreateOrbit("Old B", 202)
	_, installationB := provisionTestInstallation(t, s, orbitB.ID, 202)
	controlB, recoveryID, recoverySecret := newProvisioningMaterial(t)
	if err := s.ProvisionInstallationSecrets(
		Identity{Kind: IdentityTelegram, TelegramUserID: 202},
		installationB.ActorID, controlB, recoveryID, recoverySecret); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	legacy, err := sql.Open("sqlite", path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.Exec(`UPDATE slots SET token_hash = ? WHERE orbit_id = ? AND slot = 'a'`,
		hashToken(controlB), orbitA.ID); err != nil {
		legacy.Close()
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	off, err := Open(path)
	if err != nil {
		t.Fatalf("feature-off open must tolerate additive collision: %v", err)
	}
	if _, err := off.ResolveTokenActorContext(controlB); !errors.Is(err, ErrSelfServiceOnboardingDisabled) {
		t.Fatalf("feature-off resolver error = %v", err)
	}
	if _, _, ok, err := off.LookupPlaybackToken(controlB); err != nil || !ok {
		t.Fatalf("feature-off legacy lookup ok=%v err=%v", ok, err)
	}
	if err := off.Close(); err != nil {
		t.Fatal(err)
	}

	if reopened, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true}); err == nil ||
		!errors.Is(err, ErrCredentialDomainConflict) {
		if reopened != nil {
			reopened.Close()
		}
		t.Fatalf("feature-on collision open error = %v", err)
	}
}

// R2: provisioning is a one-time generation transition. Two independent
// Store handles can race, but only one candidate may ever commit.
func TestR2ConcurrentInitialProvisioningHasSingleWinner(t *testing.T) {
	path := filepath.Join(t.TempDir(), "double-provision.db")
	s1, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s1.Close()
	orbit, _ := s1.CreateOrbit("Single winner", 101)
	_, target := provisionTestInstallation(t, s1, orbit.ID, 101)
	s2, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s2.Close()

	type candidate struct {
		token, recoveryID, recoverySecret string
		err                               error
	}
	candidates := make([]candidate, 2)
	for i := range candidates {
		candidates[i].token, candidates[i].recoveryID, candidates[i].recoverySecret = newProvisioningMaterial(t)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i, st := range []*Store{s1, s2} {
		wg.Add(1)
		go func(i int, st *Store) {
			defer wg.Done()
			<-start
			candidates[i].err = st.ProvisionInstallationSecrets(
				Identity{Kind: IdentityTelegram, TelegramUserID: 101}, target.ActorID,
				candidates[i].token, candidates[i].recoveryID, candidates[i].recoverySecret)
		}(i, st)
	}
	close(start)
	wg.Wait()

	winners := 0
	for i := range candidates {
		if candidates[i].err == nil {
			winners++
			ctx, err := s1.ResolveTokenActorContext(candidates[i].token)
			if err != nil || ctx.ActorID != target.ActorID {
				t.Fatalf("winner %d did not resolve: ctx=%+v err=%v", i, ctx, err)
			}
			continue
		}
		if !errors.Is(candidates[i].err, ErrCredentialAlreadyProvisioned) {
			t.Fatalf("loser %d error = %v", i, candidates[i].err)
		}
		if _, err := s1.ResolveTokenActorContext(candidates[i].token); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("loser %d token resolution error = %v", i, err)
		}
	}
	if winners != 1 {
		t.Fatalf("provisioning winners = %d, want 1; errors=%v / %v", winners, candidates[0].err, candidates[1].err)
	}
}

// R2/R8: revoked, left, stale-generation, satellite, and already-provisioned
// targets all fail without changing credential state.
func TestR2InitialProvisioningTargetLifecycleMatrix(t *testing.T) {
	tests := []struct {
		name    string
		mutate  func(t *testing.T, s *Store, orbitID int64, target ActorContext)
		wantErr error
	}{
		{
			name: "revoked actor",
			mutate: func(t *testing.T, s *Store, _ int64, target ActorContext) {
				_, err := s.db.Exec(`UPDATE actors SET revoked_at = 1 WHERE id = ?`, target.ActorID)
				if err != nil {
					t.Fatal(err)
				}
			},
			wantErr: ErrUnauthorized,
		},
		{
			name: "left membership",
			mutate: func(t *testing.T, s *Store, orbitID int64, target ActorContext) {
				_, err := s.db.Exec(`UPDATE memberships SET left_at = 1 WHERE orbit_id = ? AND actor_id = ?`, orbitID, target.ActorID)
				if err != nil {
					t.Fatal(err)
				}
			},
			wantErr: ErrUnauthorized,
		},
		{
			name: "stale binding",
			mutate: func(t *testing.T, s *Store, orbitID int64, _ ActorContext) {
				_, err := s.db.Exec(`UPDATE slots SET token_hash = ? WHERE orbit_id = ? AND slot = 'a'`, hashToken(randomHex(32)), orbitID)
				if err != nil {
					t.Fatal(err)
				}
			},
			wantErr: ErrUnauthorized,
		},
		{
			name: "satellite target",
			mutate: func(t *testing.T, s *Store, orbitID int64, target ActorContext) {
				_, err := s.db.Exec(`UPDATE memberships SET role = 'satellite' WHERE orbit_id = ? AND actor_id = ?`, orbitID, target.ActorID)
				if err != nil {
					t.Fatal(err)
				}
			},
			wantErr: ErrInsufficientCapability,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s := openIdentityTemp(t)
			orbit, _ := s.CreateOrbit("Lifecycle target", 101)
			_, target := provisionTestInstallation(t, s, orbit.ID, 101)
			tc.mutate(t, s, orbit.ID, target)
			control, recoveryID, recoverySecret := newProvisioningMaterial(t)
			err := s.ProvisionInstallationSecrets(
				Identity{Kind: IdentityTelegram, TelegramUserID: 101}, target.ActorID,
				control, recoveryID, recoverySecret)
			if !errors.Is(err, tc.wantErr) {
				t.Fatalf("provisioning error = %v, want %v", err, tc.wantErr)
			}
			var count int
			if err := s.db.QueryRow(`SELECT COUNT(*) FROM installation_credentials
WHERE actor_id = ? AND control_token_hash IS NULL AND recovery_id IS NULL
  AND recovery_secret_hash IS NULL`, target.ActorID).Scan(&count); err != nil || count != 1 {
				t.Fatalf("rejected target was mutated: count=%d err=%v", count, err)
			}
		})
	}

	t.Run("already provisioned", func(t *testing.T) {
		s := openIdentityTemp(t)
		orbit, _ := s.CreateOrbit("Already", 101)
		_, target := provisionTestInstallation(t, s, orbit.ID, 101)
		firstControl, firstRecoveryID, firstRecoverySecret := newProvisioningMaterial(t)
		if err := s.ProvisionInstallationSecrets(
			Identity{Kind: IdentityTelegram, TelegramUserID: 101}, target.ActorID,
			firstControl, firstRecoveryID, firstRecoverySecret); err != nil {
			t.Fatal(err)
		}
		secondControl, secondRecoveryID, secondRecoverySecret := newProvisioningMaterial(t)
		if err := s.ProvisionInstallationSecrets(
			Identity{Kind: IdentityTelegram, TelegramUserID: 101}, target.ActorID,
			secondControl, secondRecoveryID, secondRecoverySecret); !errors.Is(err, ErrCredentialAlreadyProvisioned) {
			t.Fatalf("second provisioning error = %v", err)
		}
		var storedControl, storedRecoveryID string
		if err := s.db.QueryRow(`SELECT control_token_hash, recovery_id FROM installation_credentials WHERE actor_id = ?`,
			target.ActorID).Scan(&storedControl, &storedRecoveryID); err != nil {
			t.Fatal(err)
		}
		if storedControl != hashToken(firstControl) || storedRecoveryID != firstRecoveryID {
			t.Fatal("second provisioning overwrote the initial generation")
		}
	})
}

// R1/R8: the resolver's negative matrix rejects multiple node matches,
// multiple control matches, and orbit-misaligned lifecycle state. A satellite
// control credential can authenticate for context, but cannot provision.
func TestR8ActorResolverAmbiguityAndAlignmentNegativeMatrix(t *testing.T) {
	t.Run("multiple node matches", func(t *testing.T) {
		s := openIdentityTemp(t)
		orbitA, _ := s.CreateOrbit("Node match A", 101)
		nodeA, _ := provisionTestInstallation(t, s, orbitA.ID, 101)
		orbitB, _ := s.CreateOrbit("Node match B", 202)
		_, installationB := provisionTestInstallation(t, s, orbitB.ID, 202)
		if _, err := s.db.Exec(`DROP INDEX slots_token`); err != nil {
			t.Fatal(err)
		}
		digest := hashToken(nodeA)
		if _, err := s.db.Exec(`UPDATE slots SET token_hash = ? WHERE orbit_id = ? AND slot = 'a'`, digest, orbitB.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`UPDATE installation_credentials SET binding_token_hash = ? WHERE actor_id = ?`, digest, installationB.ActorID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ResolveTokenActorContext(nodeA); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("multiple-node resolver error = %v", err)
		}
	})

	t.Run("multiple control matches", func(t *testing.T) {
		s := openIdentityTemp(t)
		orbitA, _ := s.CreateOrbit("Control match A", 101)
		_, installationA := provisionTestInstallation(t, s, orbitA.ID, 101)
		controlA, recoveryIDA, recoverySecretA := newProvisioningMaterial(t)
		if err := s.ProvisionInstallationSecrets(Identity{Kind: IdentityTelegram, TelegramUserID: 101},
			installationA.ActorID, controlA, recoveryIDA, recoverySecretA); err != nil {
			t.Fatal(err)
		}
		orbitB, _ := s.CreateOrbit("Control match B", 202)
		_, installationB := provisionTestInstallation(t, s, orbitB.ID, 202)
		controlB, recoveryIDB, recoverySecretB := newProvisioningMaterial(t)
		if err := s.ProvisionInstallationSecrets(Identity{Kind: IdentityTelegram, TelegramUserID: 202},
			installationB.ActorID, controlB, recoveryIDB, recoverySecretB); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`DROP INDEX installation_credentials_control`); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`UPDATE installation_credentials SET control_token_hash = ? WHERE actor_id = ?`,
			hashToken(controlA), installationB.ActorID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ResolveTokenActorContext(controlA); !errors.Is(err, ErrUnauthorized) {
			t.Fatalf("multiple-control resolver error = %v", err)
		}
	})

	t.Run("misaligned membership", func(t *testing.T) {
		s := openIdentityTemp(t)
		orbitA, _ := s.CreateOrbit("Aligned A", 101)
		_, installationA := provisionTestInstallation(t, s, orbitA.ID, 101)
		controlA, recoveryID, recoverySecret := newProvisioningMaterial(t)
		if err := s.ProvisionInstallationSecrets(Identity{Kind: IdentityTelegram, TelegramUserID: 101},
			installationA.ActorID, controlA, recoveryID, recoverySecret); err != nil {
			t.Fatal(err)
		}
		orbitB, _ := s.CreateOrbit("Aligned B", 202)
		if _, err := s.db.Exec(`UPDATE memberships SET left_at = 1 WHERE actor_id = ? AND orbit_id = ?`, installationA.ActorID, orbitA.ID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`INSERT INTO memberships(orbit_id, actor_id, role, joined_at) VALUES(?, ?, 'primary', 2)`,
			orbitB.ID, installationA.ActorID); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ResolveTokenActorContext(controlA); !errors.Is(err, ErrInsufficientCapability) {
			t.Fatalf("misaligned resolver error = %v", err)
		}
	})

	t.Run("satellite control cannot provision", func(t *testing.T) {
		s := openIdentityTemp(t)
		orbit, _ := s.CreateOrbit("Satellite control", 101)
		_, installationA := provisionTestInstallation(t, s, orbit.ID, 101)
		_, installationB := provisionTestInstallation(t, s, orbit.ID, 101)
		controlA, recoveryIDA, recoverySecretA := newProvisioningMaterial(t)
		if err := s.ProvisionInstallationSecrets(Identity{Kind: IdentityTelegram, TelegramUserID: 101},
			installationA.ActorID, controlA, recoveryIDA, recoverySecretA); err != nil {
			t.Fatal(err)
		}
		if _, err := s.db.Exec(`UPDATE memberships SET role = 'satellite' WHERE actor_id = ? AND orbit_id = ?`,
			installationA.ActorID, orbit.ID); err != nil {
			t.Fatal(err)
		}
		ctx, err := s.ResolveTokenActorContext(controlA)
		if err != nil || ctx.Role != "satellite" || !ctx.Capabilities.Has(CapabilityControl) {
			t.Fatalf("satellite context=%+v err=%v", ctx, err)
		}
		controlB, recoveryIDB, recoverySecretB := newProvisioningMaterial(t)
		if err := s.ProvisionInstallationSecrets(Identity{Kind: IdentityBearer, Token: controlA},
			installationB.ActorID, controlB, recoveryIDB, recoverySecretB); !errors.Is(err, ErrInsufficientCapability) {
			t.Fatalf("satellite provisioning error = %v", err)
		}
	})

	t.Run("active membership uniqueness", func(t *testing.T) {
		s := openIdentityTemp(t)
		orbitA, _ := s.CreateOrbit("Unique A", 101)
		orbitB, _ := s.CreateOrbit("Unique B", 202)
		actorID := telegramActorID(t, s, 101)
		if _, err := s.db.Exec(`INSERT INTO memberships(orbit_id, actor_id, role, joined_at) VALUES(?, ?, 'companion', 2)`,
			orbitB.ID, actorID); err == nil {
			t.Fatal("memberships_one_active accepted a real second active membership")
		}
		assertMembership(t, s, actorID, orbitA.ID, "primary", false)
	})
}
