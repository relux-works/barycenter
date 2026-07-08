// Provider-layer wiring (spec-providers §2-§4, EPIC A6): glue between the
// loop, the resolve cascade (internal/resolver) and the store's canonical
// track cache. Everything in this file is inert unless DUET_PROVIDERS=1.
package main

import (
	"fmt"
	"log/slog"
	"os"
	"sort"
	"strings"
	"time"

	"relux.works/duet/coordinator/internal/protocol"
	"relux.works/duet/coordinator/internal/resolver"
	"relux.works/duet/coordinator/internal/session"
	"relux.works/duet/coordinator/internal/spotify"
	"relux.works/duet/coordinator/internal/store"
	"relux.works/duet/coordinator/internal/ulid"
)

// refEntry aliases the per-provider ref shape of store.UpsertTrack.
type refEntry = struct {
	Ref        string
	DurationMS int64
}

// resolvedTrack is one cascade run: canonical metadata for the tracks row
// plus a ref for every target provider that resolved.
type resolvedTrack struct {
	Title      string
	Artists    []string
	DurationMS int64
	ISRC       string
	Method     string // resolution.method (spec-providers §2)
	Score      float64
	Refs       map[string]refEntry // target provider -> resolved ref
}

// resolveTrackFn maps an origin ref onto the given target providers.
// Production implementation: newResolveTrackFn; tests stub it on the loop.
type resolveTrackFn func(originProvider, originRef, sourceURL string, targets []string) resolvedTrack

// spotifyProvider adapts internal/spotify to resolver.ProviderClient.
type spotifyProvider struct {
	c *spotify.Client
}

func (p spotifyProvider) TrackByRef(ref string) (resolver.Candidate, string, error) {
	t, err := p.c.TrackByRef(ref)
	if err != nil {
		return resolver.Candidate{}, "", err
	}
	return resolver.Candidate{Ref: t.URI, Title: t.Title, Artists: t.Artists, DurationMS: t.DurationMS}, t.ISRC, nil
}

func (p spotifyProvider) SearchISRC(isrc string) (string, error) {
	return p.c.SearchISRC(isrc)
}

// Search (metadata cascade step toward Spotify) is a spike-2 follow-up; an
// empty result makes the cascade fall through to unresolved.
func (p spotifyProvider) Search(string) ([]resolver.Candidate, error) { return nil, nil }

// newResolveTrackFn builds the production cascade runner. Secrets come from
// the environment (spec-providers §0, decision (b)): DUET_YANDEX_TOKEN may
// be empty — the Yandex client then only errors and the cascade falls
// through; DUET_ODESLI_KEY is optional (rate limits apply without it).
func newResolveTrackFn(log *slog.Logger, sp *spotify.Client) resolveTrackFn {
	clients := map[string]resolver.ProviderClient{
		"yandex": resolver.NewYandex(os.Getenv("DUET_YANDEX_TOKEN")),
	}
	if sp != nil {
		clients["spotify"] = spotifyProvider{c: sp}
	}
	odesli := resolver.NewOdesli(os.Getenv("DUET_ODESLI_KEY"))

	return func(originProvider, originRef, sourceURL string, targets []string) resolvedTrack {
		out := resolvedTrack{Method: "same", Score: 1, Refs: map[string]refEntry{}}
		origin := resolver.Candidate{Ref: originRef}
		isrc := ""
		if c, ok := clients[originProvider]; ok {
			if cand, i, err := c.TrackByRef(originRef); err == nil {
				cand.Ref = originRef
				origin, isrc = cand, i
				out.Title, out.Artists, out.DurationMS, out.ISRC = cand.Title, cand.Artists, cand.DurationMS, i
			} else {
				log.Warn("origin metadata fetch failed", "provider", originProvider, "ref", originRef, "err", err)
			}
		}
		for _, target := range targets {
			if target == originProvider {
				continue // same-provider needs no cascade
			}
			res := resolver.Resolve(origin, originProvider, isrc, sourceURL, target, clients, odesli)
			if res.Method == "unresolved" || res.Ref == "" {
				log.Info("resolve unresolved", "target", target, "origin", originRef)
				continue
			}
			out.Refs[target] = refEntry{Ref: res.Ref}
			out.Method, out.Score = res.Method, res.Score
		}
		return out
	}
}

// originFromURI splits a canonical element URI into (provider, providerRef,
// sourceURL). Ref formats per spec-providers §2: spotify keeps the full uri,
// yandex uses "<track_id>:<album_id>". sourceURL feeds the Odesli step.
func originFromURI(uri string) (provider, ref, sourceURL string) {
	if id, ok := strings.CutPrefix(uri, "spotify:track:"); ok && id != "" {
		return "spotify", uri, "https://open.spotify.com/track/" + id
	}
	if rest, ok := strings.CutPrefix(uri, "yandex:track:"); ok && rest != "" {
		track, album, _ := strings.Cut(rest, ":")
		if album != "" {
			return "yandex", rest, fmt.Sprintf("https://music.yandex.ru/album/%s/track/%s", album, track)
		}
		return "yandex", rest, "https://music.yandex.ru/track/" + track
	}
	return "", "", ""
}

// providerName renders a provider id for chat texts (spec-providers §4.2:
// availability answers always name the home and the provider).
func providerName(p string) string {
	switch p {
	case "spotify":
		return "Spotify"
	case "yandex":
		return "Яндекс"
	}
	return p
}

// distinctProviders returns the sorted provider set active in the orbit.
func distinctProviders(slotProviders map[string]string) []string {
	seen := map[string]bool{}
	var out []string
	for _, p := range slotProviders {
		if !seen[p] {
			seen[p] = true
			out = append(out, p)
		}
	}
	sort.Strings(out)
	return out
}

// sortedSlots returns the orbit's slot letters in stable order.
func sortedSlots(slotProviders map[string]string) []string {
	out := make([]string, 0, len(slotProviders))
	for s := range slotProviders {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}

// resolveDone carries a completed cascade back to the loop goroutine: the
// external HTTP of resolveTrack must never run on the FSM loop (bugs #4 /
// architecture 1.1), so the cache-miss path resolves in a goroutine and returns
// the result here for the gate + enqueue.
type resolveDone struct {
	orbit      int64
	el         session.Element
	originProv string
	originRef  string
	ctid       string // cached ctid at dispatch time ("" when the track is new)
	res        resolvedTrack
	reply      func(string)
}

// resolveAndEnqueue enqueues a fresh shared-air track under the provider layer
// (spec-providers §4.2, decision P1). The forever-cache (tracks/track_refs) is
// consulted on the loop goroutine; only a genuine cache miss offloads the
// external cascade to a goroutine, whose result returns over resolveCh. Only
// called when cfg.Providers is on.
func (l *loop) resolveAndEnqueue(o *orbitState, el session.Element, reply func(string)) {
	originProv, originRef, sourceURL := originFromURI(el.URI)
	if originProv == "" {
		l.finishEnqueue(o, el, reply) // not provider-shaped: legacy path
		return
	}
	slotProviders, err := l.sessionSlotProviders(o)
	if err != nil {
		l.log.Error("slot providers lookup failed", "orbit", o.id, "err", err)
		l.finishEnqueue(o, el, reply) // fail open: behave as pre-provider
		return
	}
	ctid, err := l.st.CTIDByRef(originProv, originRef)
	if err != nil {
		l.log.Error("ctid lookup failed", "origin", originRef, "err", err)
	}
	var missing []string
	resolved := map[string]bool{originProv: true}
	for _, prov := range distinctProviders(slotProviders) {
		if resolved[prov] {
			continue
		}
		if ctid != "" {
			if ref, _, _ := l.st.TrackRef(ctid, prov); ref != "" {
				resolved[prov] = true
				continue
			}
		}
		missing = append(missing, prov)
	}
	if len(missing) == 0 || l.resolveTrack == nil {
		// No external work needed (all cached, same-provider orbit, or no
		// cascade wired): finalize synchronously on the loop.
		l.finalizeResolvedTrack(o, el, originProv, originRef, ctid, resolvedTrack{}, false, reply)
		return
	}
	// Cache miss: run the cascade off the loop and deliver back over resolveCh.
	orbitID := o.id
	go func() {
		res := l.resolveTrack(originProv, originRef, sourceURL, missing)
		l.resolveCh <- resolveDone{
			orbit: orbitID, el: el, originProv: originProv,
			originRef: originRef, ctid: ctid, res: res, reply: reply,
		}
	}()
}

// onResolveDone re-enters the loop with a completed cascade and finishes the
// enqueue (gate + queue). The air is re-resolved because an approach may have
// engaged/dissolved while the cascade ran.
func (l *loop) onResolveDone(d resolveDone) {
	if l.orbitGone(d.orbit) { // L3: /dissolve raced the resolve goroutine
		return
	}
	o := l.stateFor(d.orbit)
	if o.sess.Mode != session.ModeShared {
		d.reply("сейчас режим solo: /inject подкинет трек партнёру, /together вернёт общий эфир")
		return
	}
	l.finalizeResolvedTrack(o, d.el, d.originProv, d.originRef, d.ctid, d.res, true, d.reply)
}

// finalizeResolvedTrack caches the canonical track, stamps the element and
// applies the P1 strict-air gate before enqueuing. hadCascade distinguishes a
// completed external resolve from the cache-only / same-provider path. Runs on
// the loop goroutine.
func (l *loop) finalizeResolvedTrack(o *orbitState, el session.Element, originProv, originRef, ctid string, res resolvedTrack, hadCascade bool, reply func(string)) {
	slotProviders, err := l.sessionSlotProviders(o)
	if err != nil {
		l.log.Error("slot providers lookup failed", "orbit", o.id, "err", err)
		l.finishEnqueue(o, el, reply) // fail open
		return
	}
	resolved := map[string]bool{originProv: true}
	for _, prov := range distinctProviders(slotProviders) {
		if resolved[prov] || ctid == "" {
			continue
		}
		if ref, _, _ := l.st.TrackRef(ctid, prov); ref != "" {
			resolved[prov] = true
		}
	}
	switch {
	case hadCascade:
		if ctid == "" {
			ctid = ulid.NewCTID(time.Now())
		}
		refs := map[string]refEntry{originProv: {Ref: originRef, DurationMS: res.DurationMS}}
		for prov, r := range res.Refs {
			refs[prov] = r
			resolved[prov] = true
		}
		if err := l.st.UpsertTrack(store.Track{
			CTID: ctid, Title: res.Title, Artists: res.Artists,
			DurationMS: res.DurationMS, ISRC: res.ISRC,
			OriginProv: originProv, OriginRef: originRef,
			ResolveMethod: res.Method, ResolveScore: res.Score,
		}, refs); err != nil {
			l.log.Error("track upsert failed", "ctid", ctid, "err", err)
		}
		if res.Title != "" {
			el.Title = res.Title
			if len(res.Artists) > 0 {
				el.Title = strings.Join(res.Artists, ", ") + " — " + res.Title
			}
		}
		if res.DurationMS > 0 {
			el.DurationMS = res.DurationMS
		}
	case ctid == "":
		// Same-provider orbit (or no cascade wired): mint the ctid and cache
		// the identity mapping alone.
		ctid = ulid.NewCTID(time.Now())
		if err := l.st.UpsertTrack(store.Track{
			CTID: ctid, OriginProv: originProv, OriginRef: originRef,
			ResolveMethod: "same", ResolveScore: 1,
		}, map[string]refEntry{originProv: {Ref: originRef}}); err != nil {
			l.log.Error("track upsert failed", "ctid", ctid, "err", err)
		}
	}
	el.CTID = ctid

	// P1 strict air: every home's provider must hold a ref for the track.
	for _, slot := range sortedSlots(slotProviders) {
		if prov := slotProviders[slot]; !resolved[prov] {
			reply(fmt.Sprintf("«%s»: нет у дома %s (%s), не ставлю",
				trackLabel(el), l.peerName(o, protocol.NodeID(slot)), providerName(prov)))
			return
		}
	}
	l.finishEnqueue(o, el, reply)
}

// finishEnqueue journals the element, enqueues it into the shared air and
// replies. Shared by the provider path (after the gate) and the flag-off path.
func (l *loop) finishEnqueue(o *orbitState, el session.Element, reply func(string)) {
	l.st.InsertElement(el)
	l.apply(o, o.sess.EnqueueTrack(el))
	if o.sess.Current != nil && o.sess.Current.ID == el.ID {
		reply("очередь пуста — ставлю сразу: " + trackLabel(el))
	} else {
		reply(fmt.Sprintf("добавил в очередь под номером %d: %s", o.sess.QueueLen(), trackLabel(el)))
	}
}

// sessionSlotProviders maps the session's homes to their providers; a group
// session unions both orbits' slots under composite ids (design §12 L1) so
// the P1 gate keeps protecting every home in the shared air.
func (l *loop) sessionSlotProviders(o *orbitState) (map[string]string, error) {
	if !o.group() {
		return l.st.SlotProviders(o.id)
	}
	out := map[string]string{}
	for _, orbitID := range o.orbits {
		provs, err := l.st.SlotProviders(orbitID)
		if err != nil {
			return nil, err
		}
		for sl, p := range provs {
			out[string(compositeID(orbitID, protocol.NodeID(sl)))] = p
		}
	}
	return out, nil
}
