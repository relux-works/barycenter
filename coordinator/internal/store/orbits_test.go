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

// A container config with empty token placeholders must not create a ghost
// orbit (prod regression 2026-07-04).
func TestLegacyBootstrapIgnoresEmptyTokens(t *testing.T) {
	s := openTemp(t)
	o, err := s.BootstrapLegacyOrbit(map[string]string{"a": "", "b": ""}, nil)
	if err != nil || o != nil {
		t.Fatalf("ghost orbit: %v %v", o, err)
	}
	ids, _ := s.OrbitIDs()
	if len(ids) != 0 {
		t.Fatalf("orbits created: %v", ids)
	}
}

func TestProviderLayerStore(t *testing.T) {
	s := openTemp(t)
	o, _ := s.CreateOrbit("O", 111)
	slot, _, _ := s.PairSlot(o.ID, 111)

	// slots.provider default + switch
	provs, _ := s.SlotProviders(o.ID)
	if provs[slot] != "spotify" {
		t.Fatalf("default provider: %v", provs)
	}
	s.SetSlotProvider(o.ID, slot, "yandex")
	provs, _ = s.SlotProviders(o.ID)
	if provs[slot] != "yandex" {
		t.Fatalf("switched provider: %v", provs)
	}

	// canonical track + refs
	err := s.UpsertTrack(Track{CTID: "ct_1", Title: "T", Artists: []string{"A"},
		DurationMS: 214000, ISRC: "X", OriginProv: "spotify", OriginRef: "spotify:track:s1",
		ResolveMethod: "odesli", ResolveScore: 0.97},
		map[string]struct {
			Ref        string
			DurationMS int64
		}{
			"spotify": {"spotify:track:s1", 214000},
			"yandex":  {"111:222", 213800},
		})
	if err != nil {
		t.Fatal(err)
	}
	if ctid, _ := s.CTIDByRef("yandex", "111:222"); ctid != "ct_1" {
		t.Fatalf("reverse lookup: %q", ctid)
	}
	if ref, dur, _ := s.TrackRef("ct_1", "yandex"); ref != "111:222" || dur != 213800 {
		t.Fatalf("ref lookup: %q %d", ref, dur)
	}
	if ref, _, _ := s.TrackRef("ct_1", "tidal"); ref != "" {
		t.Fatalf("unresolved provider must be empty, got %q", ref)
	}

	// availability cache with TTL
	s.SetAvailability(o.ID, slot, "yandex", "111:222", true)
	if ok, known, _ := s.Availability(o.ID, slot, "yandex", "111:222", 60_000); !ok || !known {
		t.Fatal("fresh availability must be known+ok")
	}
	if _, known, _ := s.Availability(o.ID, slot, "yandex", "111:222", -1); known {
		t.Fatal("expired availability must be unknown")
	}
}
