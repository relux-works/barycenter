// Bot-UX redesign (bot-ux.md): /home renders members by name, /make_primary
// takes a name, and /leave + /dissolve exist. Driven through the real loop +
// FSM + SQLite store with the fake transport.
package main

import (
	"strings"
	"testing"

	"relux.works/duet/coordinator/internal/protocol"
)

// /home lists members BY NAME (not raw tg id), with each home's liveness.
func TestHomeRendersMembersByName(t *testing.T) {
	l, _ := newTestLoop(t)
	if err := l.st.TransferPrimary(1, 111); err != nil { // pin Ivan as primary
		t.Fatal(err)
	}
	// A command from each member records their display name (SetMemberName).
	l.handleBot(cmdEvent(t, "a", "/now", &replies{}))
	l.handleBot(cmdEvent(t, "b", "/now", &replies{}))

	r := &replies{}
	l.handleBot(cmdEvent(t, "b", "/home", r))
	got := r.last(t)
	for _, want := range []string{"user-a", "user-b", "дом a", "дом b", "в сети", "главная звезда"} {
		if !strings.Contains(got, want) {
			t.Fatalf("/home missing %q in: %q", want, got)
		}
	}
	if strings.Contains(got, "· 111") || strings.Contains(got, "участник 111") {
		t.Fatalf("/home leaked a raw tg id: %q", got)
	}
}

// /make_primary accepts a display name (and @name), not a raw id (bot-ux #5).
func TestMakePrimaryByName(t *testing.T) {
	l, _ := newTestLoop(t)
	if err := l.st.TransferPrimary(1, 111); err != nil {
		t.Fatal(err)
	}
	l.handleBot(cmdEvent(t, "b", "/now", &replies{})) // record Katya's name

	r := &replies{}
	l.handleBot(cmdEvent(t, "a", "/make_primary user-b", r))
	if m, _ := l.st.MemberOf(222); m == nil || m.Role != "primary" {
		t.Fatalf("222 must be primary after transfer: %+v", m)
	}
	if m, _ := l.st.MemberOf(111); m == nil || m.Role != "companion" {
		t.Fatalf("111 must step down to companion: %+v", m)
	}

	// An unknown name is rejected, not silently applied.
	l.handleBot(cmdEvent(t, "b", "/make_primary никого-нет", r))
	if !strings.Contains(r.last(t), "не нашёл") {
		t.Fatalf("unknown name reply: %q", r.last(t))
	}
}

// /leave removes a member and stops their home; the orbit survives.
func TestLeaveMember(t *testing.T) {
	l, fake := newTestLoop(t)
	if err := l.st.TransferPrimary(1, 111); err != nil { // 222 is a companion
		t.Fatal(err)
	}
	fake.drain()

	r := &replies{}
	l.handleBot(cmdEvent(t, "b", "/leave", r))
	if !strings.Contains(r.last(t), "вышел") {
		t.Fatalf("leave reply: %q", r.last(t))
	}
	if m, _ := l.st.MemberOf(222); m != nil {
		t.Fatal("leaver is still a member")
	}
	if slot, _ := l.st.SlotOf(1, 222); slot != "" {
		t.Fatalf("leaver's home not revoked: %q", slot)
	}
	stopped := false
	for _, m := range fake.ofType(protocol.TypeStop) {
		if m.node == protocol.NodeB {
			stopped = true
		}
	}
	if !stopped {
		t.Fatalf("leaver's node must be stopped: %+v", fake.sent)
	}
	if got, _ := l.st.GetOrbit(1); got == nil {
		t.Fatal("orbit must survive a companion leaving")
	}
}

// /dissolve (primary only) deletes the whole barycenter and stops every home.
func TestDissolveOrbit(t *testing.T) {
	l, fake := newTestLoop(t)
	if err := l.st.TransferPrimary(1, 111); err != nil {
		t.Fatal(err)
	}
	fake.drain()

	// A companion cannot dissolve.
	r := &replies{}
	l.handleBot(cmdEvent(t, "b", "/dissolve", r))
	if !strings.Contains(r.last(t), "primary") {
		t.Fatalf("companion dissolve gate: %q", r.last(t))
	}
	if got, _ := l.st.GetOrbit(1); got == nil {
		t.Fatal("orbit dissolved by a non-primary")
	}
	fake.drain()

	// The primary dissolves it: rows gone, homes stopped, state dropped.
	l.handleBot(cmdEvent(t, "a", "/dissolve", r))
	if !strings.Contains(r.last(t), "распущен") {
		t.Fatalf("dissolve reply: %q", r.last(t))
	}
	if got, _ := l.st.GetOrbit(1); got != nil {
		t.Fatal("orbit row survived dissolve")
	}
	if _, ok := l.states[1]; ok {
		t.Fatal("in-memory orbit state survived dissolve")
	}
	if len(fake.ofType(protocol.TypeStop)) < 2 {
		t.Fatalf("dissolve must stop every home: %+v", fake.sent)
	}
	// A former member now falls through to the stranger onboarding path.
	l.handleBot(cmdEvent(t, "a", "/now", r))
	if !strings.Contains(r.last(t), "приватная система") {
		t.Fatalf("former member should be a stranger now: %q", r.last(t))
	}
}

// /solo and /together are plain-language aliases of the mode switch (bot-ux).
func TestTogetherSoloAliases(t *testing.T) {
	l, fake := newTestLoop(t)
	r := &replies{}
	l.handleBot(cmdEvent(t, "a", "/solo", r))
	if len(fake.ofType(protocol.TypeSetMode)) != 2 {
		t.Fatalf("/solo must switch mode on both homes: %+v", fake.sent)
	}
	fake.drain()
	l.handleBot(cmdEvent(t, "a", "/together", r))
	if len(fake.ofType(protocol.TypeSetMode)) != 2 {
		t.Fatalf("/together must switch mode back: %+v", fake.sent)
	}
}
