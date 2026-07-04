package store

import (
	"strings"
	"testing"
)

func TestOrbitLifecycle(t *testing.T) {
	s := openTemp(t)

	o, err := s.CreateOrbit("Наш барицентр", 111)
	if err != nil {
		t.Fatal(err)
	}
	if o.TakeoverPolicy != "user" || o.VoiceDefault != "personal" {
		t.Fatalf("defaults: %+v", o)
	}

	m, err := s.MemberOf(111)
	if err != nil || m == nil || m.Role != "primary" || m.OrbitID != o.ID {
		t.Fatalf("creator must be primary: %+v %v", m, err)
	}
	if m, _ := s.MemberOf(999); m != nil {
		t.Fatalf("stranger resolved: %+v", m)
	}

	if err := s.AddMember(o.ID, 222, "companion"); err != nil {
		t.Fatal(err)
	}
	if err := s.TransferPrimary(o.ID, 222); err != nil {
		t.Fatal(err)
	}
	m1, _ := s.MemberOf(111)
	m2, _ := s.MemberOf(222)
	if m1.Role != "companion" || m2.Role != "primary" {
		t.Fatalf("transfer: %v %v", m1.Role, m2.Role)
	}
}

func TestInvitesAndPairing(t *testing.T) {
	s := openTemp(t)
	o, _ := s.CreateOrbit("O", 111)

	inv, err := s.NewInvite(o.ID, 111)
	if err != nil || !strings.HasPrefix(inv, "inv") {
		t.Fatalf("invite: %q %v", inv, err)
	}
	if orbit, by, _ := s.ConsumeInvite(inv, "member"); orbit != o.ID || by != 111 {
		t.Fatalf("consume failed: %d %d", orbit, by)
	}
	if orbit, _, _ := s.ConsumeInvite(inv, "member"); orbit != 0 {
		t.Fatal("invite must be one-time")
	}

	code, err := s.NewPairCode(o.ID, 111)
	if err != nil || len(code) != 8 {
		t.Fatalf("pair code: %q %v", code, err)
	}
	if orbit, _, _ := s.ConsumeInvite(code, "member"); orbit != 0 {
		t.Fatal("kind mismatch must not consume")
	}
	if orbit, _, _ := s.ConsumeInvite(code, "pair"); orbit != o.ID {
		t.Fatal("pair consume failed")
	}
}

func TestSlotsTokensRevoke(t *testing.T) {
	s := openTemp(t)
	o, _ := s.CreateOrbit("O", 111)

	slotA, tokenA, err := s.PairSlot(o.ID, 111)
	if err != nil || slotA != "a" || len(tokenA) != 64 {
		t.Fatalf("first slot: %q %v", slotA, err)
	}
	slotB, tokenB, _ := s.PairSlot(o.ID, 111)
	if slotB != "b" || tokenB == tokenA {
		t.Fatalf("second slot: %q", slotB)
	}

	orbitID, slot, ok, err := s.LookupToken(tokenA)
	if err != nil || !ok || orbitID != o.ID || slot != "a" {
		t.Fatalf("lookup: %v %v %v", orbitID, slot, ok)
	}
	if _, _, ok, _ := s.LookupToken("deadbeef"); ok {
		t.Fatal("unknown token resolved")
	}

	if err := s.RevokeSlot(o.ID, "a"); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, _ := s.LookupToken(tokenA); ok {
		t.Fatal("revoked token still valid")
	}
	// Revoked letter becomes reusable.
	slotA2, tokenA2, _ := s.PairSlot(o.ID, 111)
	if slotA2 != "a" || tokenA2 == tokenA {
		t.Fatalf("slot reuse: %q", slotA2)
	}

	slots, _ := s.ActiveSlots(o.ID)
	if len(slots) != 2 {
		t.Fatalf("active slots: %v", slots)
	}
	if slot, _ := s.SlotOf(o.ID, 111); slot == "" {
		t.Fatal("SlotOf must find the member's node")
	}
	if slot, _ := s.SlotOf(o.ID, 999); slot != "" {
		t.Fatalf("stranger has a slot: %q", slot)
	}
}

func TestLimitsSoftConfigured(t *testing.T) {
	s := openTemp(t)
	o, _ := s.CreateOrbit("O", 111)
	// Default caps: 5 pulsars, 10 members.
	for i := 0; i < 5; i++ {
		if _, _, err := s.PairSlot(o.ID, 111); err != nil {
			t.Fatalf("slot %d: %v", i, err)
		}
	}
	if _, _, err := s.PairSlot(o.ID, 111); err != ErrLimit {
		t.Fatalf("6th slot must hit ErrLimit, got %v", err)
	}
}

func TestLegacyBootstrap(t *testing.T) {
	s := openTemp(t)
	tokens := map[string]string{"a": strings.Repeat("1", 64), "b": strings.Repeat("2", 64)}
	users := map[int64]string{816078: "a", 1: "b"}
	o, err := s.BootstrapLegacyOrbit(tokens, users)
	if err != nil || o == nil {
		t.Fatalf("bootstrap: %v %v", o, err)
	}
	if orbitID, slot, ok, _ := s.LookupToken(tokens["a"]); !ok || orbitID != o.ID || slot != "a" {
		t.Fatal("legacy token must resolve")
	}
	// Second boot: no-op.
	if o2, _ := s.BootstrapLegacyOrbit(tokens, users); o2 != nil {
		t.Fatal("bootstrap must be idempotent")
	}
}
