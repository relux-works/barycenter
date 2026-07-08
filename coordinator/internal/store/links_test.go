package store

import (
	"errors"
	"testing"
	"time"
)

// Full approach lifecycle: propose -> awaiting -> active -> break
// (design §12 L1).
func TestLinkLifecycle(t *testing.T) {
	s := openTemp(t)
	a, _ := s.CreateOrbit("A", 111)
	b, _ := s.CreateOrbit("B", 222)

	code, err := s.ProposeLink(a.ID, 111)
	if err != nil || len(code) != 8 {
		t.Fatalf("propose: %q %v", code, err)
	}

	// Unknown code claims nothing.
	if id, _, err := s.AcceptByCode("NOPE1234", b.ID); err != nil || id != 0 {
		t.Fatalf("bogus code: %d %v", id, err)
	}
	// Self-approach is rejected.
	if _, _, err := s.AcceptByCode(code, a.ID); !errors.Is(err, ErrLinkSelf) {
		t.Fatalf("self approach: %v", err)
	}

	linkID, orbitA, err := s.AcceptByCode(code, b.ID)
	if err != nil || linkID == 0 || orbitA != a.ID {
		t.Fatalf("accept: %d %d %v", linkID, orbitA, err)
	}
	// The code burned with the claim.
	if id, _, _ := s.AcceptByCode(code, b.ID); id != 0 {
		t.Fatal("code must be one-time")
	}

	// Awaiting is visible on the initiator side only.
	if id, other, ok, _ := s.AwaitingLink(a.ID); !ok || id != linkID || other != b.ID {
		t.Fatalf("awaiting on A: %d %d %v", id, other, ok)
	}
	if _, _, ok, _ := s.AwaitingLink(b.ID); ok {
		t.Fatal("acceptor side must not see an awaiting confirmation")
	}
	// Not active yet.
	if _, _, ok, _ := s.ActiveLink(a.ID); ok {
		t.Fatal("awaiting link must not be active")
	}

	if err := s.ActivateLink(linkID); err != nil {
		t.Fatal(err)
	}
	if err := s.ActivateLink(linkID); err == nil {
		t.Fatal("double activation must fail")
	}
	for _, o := range []struct{ mine, other int64 }{{a.ID, b.ID}, {b.ID, a.ID}} {
		id, other, ok, _ := s.ActiveLink(o.mine)
		if !ok || id != linkID || other != o.other {
			t.Fatalf("active link of %d: %d %d %v", o.mine, id, other, ok)
		}
	}
	links, _ := s.ActiveLinks()
	if len(links) != 1 || links[0].OrbitA != a.ID || links[0].OrbitB != b.ID {
		t.Fatalf("active links: %+v", links)
	}
	if lk, _ := s.GetLink(linkID); lk == nil || lk.State != "active" || lk.Code != "" {
		t.Fatalf("get link: %+v", lk)
	}

	if err := s.BreakLink(linkID); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, _ := s.ActiveLink(a.ID); ok {
		t.Fatal("broken link still active")
	}
	if lk, _ := s.GetLink(linkID); lk != nil {
		t.Fatalf("broken link row survives: %+v", lk)
	}
}

// L1: one approach per orbit — engaged orbits reject both proposing and
// claiming codes.
func TestLinkOnePerOrbit(t *testing.T) {
	s := openTemp(t)
	a, _ := s.CreateOrbit("A", 111)
	b, _ := s.CreateOrbit("B", 222)
	c, _ := s.CreateOrbit("C", 333)

	code, _ := s.ProposeLink(a.ID, 111)
	linkID, _, err := s.AcceptByCode(code, b.ID)
	if err != nil {
		t.Fatal(err)
	}

	// Awaiting already engages both sides.
	if _, err := s.ProposeLink(a.ID, 111); !errors.Is(err, ErrLinkBusy) {
		t.Fatalf("propose while awaiting: %v", err)
	}
	codeC, err := s.ProposeLink(c.ID, 333)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.AcceptByCode(codeC, b.ID); !errors.Is(err, ErrLinkBusy) {
		t.Fatalf("engaged orbit claimed a code: %v", err)
	}

	if err := s.ActivateLink(linkID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ProposeLink(b.ID, 222); !errors.Is(err, ErrLinkBusy) {
		t.Fatalf("propose while active: %v", err)
	}
	// A stale pre-link code of an engaged orbit is dead too.
	stale, _ := s.ProposeLink(c.ID, 333)
	if _, _, err := s.AcceptByCode(stale, a.ID); !errors.Is(err, ErrLinkBusy) {
		t.Fatalf("active orbit claimed a code: %v", err)
	}

	// After the break both orbits are free again.
	if err := s.BreakLink(linkID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.ProposeLink(a.ID, 111); err != nil {
		t.Fatalf("propose after break: %v", err)
	}
}

// Approach codes expire after 15 minutes and a fresh code supersedes the
// orbit's earlier one.
func TestLinkCodeExpiryAndSupersede(t *testing.T) {
	s := openTemp(t)
	a, _ := s.CreateOrbit("A", 111)
	b, _ := s.CreateOrbit("B", 222)

	code, _ := s.ProposeLink(a.ID, 111)
	// Age the code past the TTL directly (in-package test).
	old := time.Now().Add(-16 * time.Minute).UnixMilli()
	if _, err := s.db.Exec(`UPDATE links SET created_at = ? WHERE code = ?`, old, code); err != nil {
		t.Fatal(err)
	}
	if id, _, err := s.AcceptByCode(code, b.ID); err != nil || id != 0 {
		t.Fatalf("expired code claimed: %d %v", id, err)
	}

	first, _ := s.ProposeLink(a.ID, 111)
	second, _ := s.ProposeLink(a.ID, 111)
	if id, _, _ := s.AcceptByCode(first, b.ID); id != 0 {
		t.Fatal("superseded code must be dead")
	}
	if id, _, err := s.AcceptByCode(second, b.ID); err != nil || id == 0 {
		t.Fatalf("fresh code: %d %v", id, err)
	}
}

// M4: an ignored awaiting claim must not link-lock both orbits forever — it
// stops engaging after the TTL, hygiene removes it, and /accept can't see it.
func TestAwaitingLinkExpires(t *testing.T) {
	st := openTemp(t)
	oa, _ := st.CreateOrbit("A", 101)
	ob, _ := st.CreateOrbit("B", 202)
	a, b := oa.ID, ob.ID
	code, err := st.ProposeLink(a, 101)
	if err != nil {
		t.Fatal(err)
	}
	linkID, _, err := st.AcceptByCode(code, b)
	if err != nil || linkID == 0 {
		t.Fatalf("claim: %v %d", err, linkID)
	}
	// Both engaged while the claim is fresh.
	if _, err := st.ProposeLink(a, 101); err != ErrLinkBusy {
		t.Fatalf("fresh awaiting must engage, got %v", err)
	}
	// Age the claim past the TTL.
	if _, err := st.db.Exec(`UPDATE links SET created_at = created_at - ? WHERE id = ?`,
		(awaitingTTL + time.Hour).Milliseconds(), linkID); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, _ := st.AwaitingLink(a); ok {
		t.Fatal("expired claim still visible to /accept")
	}
	if _, _, _, ok, _ := st.AwaitingLinkAnySide(b); ok {
		t.Fatal("expired claim still visible to /decline")
	}
	// Unblocked: a fresh code mints (hygiene removed the fossil).
	if _, err := st.ProposeLink(a, 101); err != nil {
		t.Fatalf("propose after expiry: %v", err)
	}
	var n int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM links WHERE id = ?`, linkID).Scan(&n); err != nil || n != 0 {
		t.Fatalf("fossil row survived hygiene: n=%d err=%v", n, err)
	}
}

// M4: the claimant side sees the awaiting link too (for /decline).
func TestAwaitingLinkAnySide(t *testing.T) {
	st := openTemp(t)
	oa, _ := st.CreateOrbit("A", 111)
	ob, _ := st.CreateOrbit("B", 222)
	a, b := oa.ID, ob.ID
	code, _ := st.ProposeLink(a, 111)
	linkID, _, err := st.AcceptByCode(code, b)
	if err != nil || linkID == 0 {
		t.Fatalf("claim: %v %d", err, linkID)
	}
	if id, other, initiator, ok, _ := st.AwaitingLinkAnySide(a); !ok || id != linkID || other != b || !initiator {
		t.Fatalf("initiator view: id=%d other=%d init=%v ok=%v", id, other, initiator, ok)
	}
	if id, other, initiator, ok, _ := st.AwaitingLinkAnySide(b); !ok || id != linkID || other != a || initiator {
		t.Fatalf("claimant view: id=%d other=%d init=%v ok=%v", id, other, initiator, ok)
	}
}
