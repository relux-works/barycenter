// Provider layer (spec-providers, EPIC A6): flag inertness, /provider
// switching, cascade wiring on enqueue and the P1 strict-air gate — driven
// through the real loop + FSM + SQLite with a stubbed cascade.
package main

import (
	"strings"
	"testing"

	"relux.works/duet/coordinator/internal/protocol"
)

const yandexLink = "https://music.yandex.ru/album/1193829/track/10994777"
const linkURI = "spotify:track:4cOdK2wGLETKBW3PvgPWqT"

// Flag off: /provider and /resolve answer "not enabled", yandex links get
// the unsupported-kind reply, spotify enqueue carries no ctid and the
// cascade never runs.
func TestProvidersFlagOffInert(t *testing.T) {
	l, fake := newTestLoop(t)
	r := &replies{}
	calls := 0
	l.resolveTrack = func(_, _, _ string, _ []string) resolvedTrack {
		calls++
		return resolvedTrack{}
	}

	l.handleBot(cmdEvent(t, "a", "/provider b yandex", r))
	if r.last(t) != "провайдерский слой ещё не включён" {
		t.Fatalf("/provider reply: %q", r.last(t))
	}
	if got := fake.ofType(protocol.TypeSetProvider); len(got) != 0 {
		t.Fatalf("set_provider sent with the flag off: %+v", got)
	}

	l.handleBot(cmdEvent(t, "a", yandexLink, r))
	if !strings.Contains(r.last(t), "такие ссылки не поддерживаю") {
		t.Fatalf("yandex link reply: %q", r.last(t))
	}
	if l.orbit(1).sess.Current != nil {
		t.Fatal("yandex link must not enqueue with the flag off")
	}

	// Spotify enqueue works exactly as before the provider layer.
	l.handleBot(cmdEvent(t, "a", link, r))
	cur := l.orbit(1).sess.Current
	if cur == nil || cur.CTID != "" {
		t.Fatalf("flag-off element must carry no ctid: %+v", cur)
	}
	if calls != 0 {
		t.Fatalf("cascade ran %d times with the flag off", calls)
	}
	if ctid, _ := l.st.CTIDByRef("spotify", linkURI); ctid != "" {
		t.Fatalf("track cached with the flag off: %q", ctid)
	}
}

// Flag on: /provider validates the slot, persists slots.provider and pushes
// set_provider to that node.
func TestProviderSwitchPersistsAndPushes(t *testing.T) {
	l, fake := newTestLoop(t)
	l.cfg.Providers = true
	r := &replies{}

	l.handleBot(cmdEvent(t, "a", "/provider b yandex", r))
	if !strings.Contains(r.last(t), "Яндекс") {
		t.Fatalf("confirmation: %q", r.last(t))
	}
	provs, err := l.st.SlotProviders(1)
	if err != nil || provs["b"] != "yandex" || provs["a"] != "spotify" {
		t.Fatalf("slot providers after switch: %v %v", provs, err)
	}
	sets := fake.ofType(protocol.TypeSetProvider)
	if len(sets) != 1 || sets[0].node != protocol.NodeB ||
		sets[0].payload.(*protocol.SetProviderPayload).Provider != "yandex" {
		t.Fatalf("set_provider push: %+v", sets)
	}
	fake.drain()

	// Unknown slot is rejected before any write or push.
	l.handleBot(cmdEvent(t, "a", "/provider z yandex", r))
	if !strings.Contains(r.last(t), "нет в орбите") {
		t.Fatalf("unknown slot reply: %q", r.last(t))
	}
	if got := fake.ofType(protocol.TypeSetProvider); len(got) != 0 {
		t.Fatalf("set_provider sent for unknown slot: %+v", got)
	}
}

// Flag on, mixed orbit: enqueue runs the cascade toward the missing
// provider, persists the canonical track, stamps Element.CTID (which
// survives the session snapshot) and never resolves twice (forever cache).
func TestEnqueueResolvesAndCaches(t *testing.T) {
	l, fake := newTestLoop(t)
	l.cfg.Providers = true
	if err := l.st.SetSlotProvider(1, "b", "yandex"); err != nil {
		t.Fatal(err)
	}
	calls := 0
	l.resolveTrack = func(originProv, originRef, sourceURL string, targets []string) resolvedTrack {
		calls++
		if originProv != "spotify" || originRef != linkURI {
			t.Errorf("origin: %s %s", originProv, originRef)
		}
		if sourceURL != "https://open.spotify.com/track/4cOdK2wGLETKBW3PvgPWqT" {
			t.Errorf("source url: %s", sourceURL)
		}
		if len(targets) != 1 || targets[0] != "yandex" {
			t.Errorf("targets: %v", targets)
		}
		return resolvedTrack{
			Title: "Test Song", Artists: []string{"Tester"}, DurationMS: 214000,
			ISRC: "GBAYE0601498", Method: "odesli", Score: 0.99,
			Refs: map[string]refEntry{"yandex": {Ref: "10994777:1193829", DurationMS: 213800}},
		}
	}
	r := &replies{}

	l.handleBot(cmdEvent(t, "a", link, r))
	l.pumpResolve(t) // the cascade runs off the loop, then enqueues
	if calls != 1 {
		t.Fatalf("cascade calls = %d", calls)
	}
	cur := l.orbit(1).sess.Current
	if cur == nil || !strings.HasPrefix(cur.CTID, "ct_") {
		t.Fatalf("current element ctid: %+v", cur)
	}
	if cur.Title != "Tester — Test Song" || cur.DurationMS != 214000 {
		t.Fatalf("metadata on element: %+v", cur)
	}
	if got := fake.ofType(protocol.TypeLoad); len(got) != 2 {
		t.Fatalf("enqueue still loads both homes, sent: %+v", fake.sent)
	}
	if !strings.Contains(r.last(t), "ставлю сразу") || !strings.Contains(r.last(t), "Test Song") {
		t.Fatalf("reply: %q", r.last(t))
	}

	// Canonical track persisted: reverse lookup + both refs.
	ctid, _ := l.st.CTIDByRef("spotify", linkURI)
	if ctid != cur.CTID {
		t.Fatalf("ctid cache: %q vs element %q", ctid, cur.CTID)
	}
	if ref, dur, _ := l.st.TrackRef(ctid, "yandex"); ref != "10994777:1193829" || dur != 213800 {
		t.Fatalf("yandex ref: %q %d", ref, dur)
	}
	if ref, _, _ := l.st.TrackRef(ctid, "spotify"); ref != linkURI {
		t.Fatalf("spotify ref: %q", ref)
	}

	// A4: ctid rides the session snapshot.
	snap, err := l.st.LoadSession(1)
	if err != nil || snap == nil || snap.Current == nil || snap.Current.CTID != ctid {
		t.Fatalf("snapshot ctid: %+v %v", snap, err)
	}

	// Same link again: forever cache, no second cascade, same ctid.
	l.handleBot(cmdEvent(t, "b", link, r))
	if calls != 1 {
		t.Fatalf("cache miss: cascade ran %d times", calls)
	}
	q := l.orbit(1).sess.Queue
	if len(q) != 1 || q[0].CTID != ctid {
		t.Fatalf("queued element ctid: %+v", q)
	}
}

func TestSameProviderTrackFetchesAndCachesHumanTitle(t *testing.T) {
	l, _ := newTestLoop(t)
	l.cfg.Providers = true
	calls := 0
	l.resolveTrack = func(originProv, originRef, _ string, targets []string) resolvedTrack {
		calls++
		if originProv != "spotify" || originRef != linkURI || len(targets) != 0 {
			t.Fatalf("metadata resolve: provider=%s ref=%s targets=%v", originProv, originRef, targets)
		}
		return resolvedTrack{Title: "Human Song", Artists: []string{"Human Artist"}, DurationMS: 123_000, Method: "same", Score: 1}
	}
	r := &replies{}

	l.handleBot(cmdEvent(t, "a", link, r))
	l.pumpResolve(t)
	cur := l.orbit(1).sess.Current
	if cur == nil || cur.Title != "Human Artist — Human Song" {
		t.Fatalf("first title = %+v", cur)
	}

	// The second link is a synchronous forever-cache hit and keeps the title.
	l.handleBot(cmdEvent(t, "b", link, r))
	if calls != 1 {
		t.Fatalf("metadata fetched %d times", calls)
	}
	q := l.orbit(1).sess.Queue
	if len(q) != 1 || q[0].Title != "Human Artist — Human Song" {
		t.Fatalf("cached title = %+v", q)
	}
}

// P1 strict air (spec-providers §4.2): a home whose provider did not
// resolve rejects the enqueue with a reply naming home and provider.
func TestPeriastronP1RejectsUnresolved(t *testing.T) {
	l, fake := newTestLoop(t)
	l.cfg.Providers = true
	if err := l.st.SetSlotProvider(1, "b", "yandex"); err != nil {
		t.Fatal(err)
	}
	l.resolveTrack = func(_, _, _ string, _ []string) resolvedTrack {
		return resolvedTrack{Title: "Test Song", Artists: []string{"Tester"},
			Method: "unresolved", Refs: map[string]refEntry{}}
	}
	r := &replies{}

	l.handleBot(cmdEvent(t, "a", link, r))
	l.pumpResolve(t) // cascade returns unresolved, then the P1 gate rejects
	want := "«Tester — Test Song»: нет у дома «Барицентр», Пульсар B (Яндекс), не ставлю"
	if r.last(t) != want {
		t.Fatalf("reply %q, want %q", r.last(t), want)
	}
	if l.orbit(1).sess.Current != nil || l.orbit(1).sess.QueueLen() != 0 {
		t.Fatalf("rejected track leaked into the session: %+v", l.orbit(1).sess.Current)
	}
	if got := fake.ofType(protocol.TypeLoad); len(got) != 0 {
		t.Fatalf("load sent for a rejected track: %+v", got)
	}
	// The origin ref is still cached for future retries (partial resolve).
	if ctid, _ := l.st.CTIDByRef("spotify", linkURI); ctid == "" {
		t.Fatal("origin ref must be cached even when a target is unresolved")
	}
}

// If metadata lookup is unavailable, the same-provider path still fails open:
// the element gets a ctid and the identity mapping is cached.
func TestSameProviderOrbitCachesIdentityWithoutMetadataResolver(t *testing.T) {
	l, _ := newTestLoop(t)
	l.cfg.Providers = true
	l.resolveTrack = nil
	r := &replies{}

	l.handleBot(cmdEvent(t, "a", link, r))
	cur := l.orbit(1).sess.Current
	if cur == nil || !strings.HasPrefix(cur.CTID, "ct_") {
		t.Fatalf("element ctid: %+v", cur)
	}
	if ref, _, _ := l.st.TrackRef(cur.CTID, "spotify"); ref != linkURI {
		t.Fatalf("identity ref: %q", ref)
	}
}

func TestOriginFromURI(t *testing.T) {
	cases := []struct {
		uri, prov, ref, src string
	}{
		{linkURI, "spotify", linkURI, "https://open.spotify.com/track/4cOdK2wGLETKBW3PvgPWqT"},
		{"yandex:track:10994777:1193829", "yandex", "10994777:1193829", "https://music.yandex.ru/album/1193829/track/10994777"},
		{"yandex:track:10994777:", "yandex", "10994777:", "https://music.yandex.ru/track/10994777"},
		{"m_voice", "", "", ""},
		{"el_pl_x", "", "", ""},
	}
	for _, c := range cases {
		prov, ref, src := originFromURI(c.uri)
		if prov != c.prov || ref != c.ref || src != c.src {
			t.Errorf("originFromURI(%q) = %q %q %q", c.uri, prov, ref, src)
		}
	}
}
