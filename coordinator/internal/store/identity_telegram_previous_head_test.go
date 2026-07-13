//go:build previoushead

package store

import (
	"database/sql"
	"path/filepath"
	"runtime"
	"testing"
)

// TestTelegramLinkExactPreviousHEADReconciliation proves that a membership
// created by the trusted Telegram consume path remains usable by the exact
// predecessor Store and reconciles its old role/leave/slot mutations when the
// identity feature is enabled again.
func TestTelegramLinkExactPreviousHEADReconciliation(t *testing.T) {
	_, currentFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("locate current test source")
	}
	storeDir := filepath.Dir(currentFile)
	repoRoot := filepath.Clean(filepath.Join(storeDir, "..", "..", ".."))
	assertExactRevisionExists(t, repoRoot)

	path := filepath.Join(t.TempDir(), "telegram-previous-head.db")
	s, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	keep, err := s.CreateOrbit("Telegram exact-old keep", 101)
	if err != nil {
		t.Fatal(err)
	}
	_, issuerContext := provisionTestInstallation(t, s, keep.ID, 101)
	controlToken, recoveryID, recoverySecret := newProvisioningMaterial(t)
	if err := s.ProvisionInstallationSecrets(
		Identity{Kind: IdentityTelegram, TelegramUserID: 101},
		issuerContext.ActorID,
		controlToken,
		recoveryID,
		recoverySecret,
	); err != nil {
		t.Fatal(err)
	}
	issued, err := s.IssueTelegramLink(issuerContext.ActorID, controlToken, "companion")
	if err != nil {
		t.Fatal(err)
	}
	linked, err := s.ConsumeTelegramLink(202, "Linked before exact old", "private", issued.Code)
	if err != nil {
		t.Fatal(err)
	}
	if linked.OrbitID != keep.ID || linked.Role != "companion" || linked.ActorID == issuerContext.ActorID {
		t.Fatalf("linked context=%+v issuer_actor=%d", linked, issuerContext.ActorID)
	}
	if _, _, err := s.PairSlot(keep.ID, 202); err != nil {
		t.Fatal(err)
	}
	dissolve, err := s.CreateOrbit("Telegram exact-old dissolve", 404)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.PairSlot(dissolve.ID, 404); err != nil {
		t.Fatal(err)
	}
	if err := s.Close(); err != nil {
		t.Fatal(err)
	}

	result := runExactPreviousHeadStoreTest(t, repoRoot, storeDir, path, keep.ID, dissolve.ID)
	if result.CreatedOrbitID == 0 || result.ReboundSlot != "b" || result.NewSlot != "c" {
		t.Fatal("exact previous Store did not exercise the expected authority coordinates")
	}

	s, err = OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	assertMembership(t, s, linked.ActorID, keep.ID, "companion", true)
	if member, err := s.MemberOf(202); err != nil || member != nil {
		t.Fatalf("exact-old leave did not project to legacy authority: member=%+v err=%v", member, err)
	}
	promoted, err := s.ResolveTelegramActorContext(303)
	if err != nil || promoted.OrbitID != keep.ID || promoted.Role != "primary" {
		t.Fatalf("exact-old primary transfer context=%+v err=%v", promoted, err)
	}
	var consumedAt sql.NullInt64
	var consumingActorID sql.NullInt64
	if err := s.db.QueryRow(`SELECT consumed_at, consuming_actor_id
FROM telegram_link_codes WHERE code_hash = ?`, hashToken(issued.Code)).Scan(&consumedAt, &consumingActorID); err != nil {
		t.Fatal(err)
	}
	if !consumedAt.Valid || !consumingActorID.Valid || consumingActorID.Int64 != linked.ActorID {
		t.Fatalf("consumed link state changed: consumed=%v actor=%v", consumedAt.Valid, consumingActorID)
	}
	assertDatabaseHealthy(t, s)
}
