package store

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestTelegramAndAppShareRoleScopedDNDAndBlockService(t *testing.T) {
	st, err := OpenWithOptions(filepath.Join(t.TempDir(), "presence-policy.db"),
		Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	owner, err := st.CreateSelfServiceOrbit("Shared policy")
	if err != nil {
		t.Fatal(err)
	}
	link, err := st.IssueTelegramLink(owner.ActorID, owner.ControlToken, "companion")
	if err != nil {
		t.Fatal(err)
	}
	const telegramUserID = int64(880_041)
	linked, err := st.ConsumeTelegramLink(telegramUserID, "Policy operator", "private", link.Code)
	if err != nil {
		t.Fatal(err)
	}
	identity := Identity{Kind: IdentityTelegram, TelegramUserID: telegramUserID}
	now := time.Now().UnixMilli()
	base := AuthorizedDNDMutationParams{
		ExpectedActorID: linked.ActorID, Identity: identity, Layer: "orbit",
		Mode: DNDMessagesOnly, ExpectedRevision: 0,
		IdempotencyKeyHash: strings.Repeat("1", 64), RequestHash: strings.Repeat("2", 64), UpdatedAt: now,
	}
	if _, err := st.AuthorizedSetDND(base); !errors.Is(err, ErrTransmissionPolicyForbidden) {
		t.Fatalf("companion changed orbit DND: %v", err)
	}
	if _, err := st.db.Exec(`UPDATE memberships SET role = 'companion'
WHERE orbit_id = ? AND actor_id = ?`, owner.OrbitID, owner.ActorID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`UPDATE memberships SET role = 'primary'
WHERE orbit_id = ? AND actor_id = ?`, owner.OrbitID, linked.ActorID); err != nil {
		t.Fatal(err)
	}
	base.IdempotencyKeyHash = strings.Repeat("3", 64)
	base.RequestHash = strings.Repeat("4", 64)
	mutation, err := st.AuthorizedSetDND(base)
	if err != nil || mutation.Setting.Mode != DNDMessagesOnly || mutation.Setting.Revision != 1 {
		t.Fatalf("Telegram primary orbit DND=%+v err=%v", mutation, err)
	}
	local := base
	local.Layer = "local"
	local.IdempotencyKeyHash = strings.Repeat("5", 64)
	local.RequestHash = strings.Repeat("6", 64)
	if _, err := st.AuthorizedSetDND(local); !errors.Is(err, ErrTransmissionPolicyForbidden) {
		t.Fatalf("Telegram loosened installation-local DND: %v", err)
	}

	sender, err := st.CreateSelfServiceOrbit("Blocked sender")
	if err != nil {
		t.Fatal(err)
	}
	ref, err := st.MintTransmissionSubjectReferenceForIdentity(linked.ActorID,
		identity, BlockedSubjectOrbit, sender.OrbitID, now+1)
	if err != nil {
		t.Fatal(err)
	}
	block, err := st.AuthorizedCreateTransmissionBlock(AuthorizedCreateBlockParams{
		ExpectedActorID: linked.ActorID, Identity: identity, OwnerScope: BlockOwnerOrbit,
		SubjectRef: ref.PublicID, IdempotencyKeyHash: strings.Repeat("7", 64),
		RequestHash: strings.Repeat("8", 64), CreatedAt: now + 2,
	})
	if err != nil || block.SubjectKind != BlockedSubjectOrbit || block.OwnerScope != BlockOwnerOrbit {
		t.Fatalf("Telegram primary block=%+v err=%v", block, err)
	}
	listed, err := st.AuthorizedListTransmissionBlocksForIdentity(linked.ActorID, identity)
	if err != nil || len(listed) != 1 || listed[0].ID != block.ID {
		t.Fatalf("Telegram block list=%+v err=%v", listed, err)
	}
	deleted, changed, err := st.AuthorizedDeleteTransmissionBlockForIdentity(
		linked.ActorID, identity, block.ID, now+3)
	if err != nil || !changed || !deleted.Revoked {
		t.Fatalf("Telegram block delete=%+v changed=%v err=%v", deleted, changed, err)
	}
	if _, err := st.db.Exec(`UPDATE actors SET revoked_at = ? WHERE id = ?`, now+4, linked.ActorID); err != nil {
		t.Fatal(err)
	}
	if _, err := st.AuthorizedSetDND(base); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("revoked Telegram actor replayed an old policy response: %v", err)
	}
}
