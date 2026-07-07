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

// resolveElement runs the resolve cascade for a fresh track element toward
// every provider active in the orbit, persists the canonical track (forever
// cache: tracks/track_refs) and applies the P1 strict-air gate — a home
// whose provider holds no ref for the track rejects the whole enqueue
// (spec-providers §4.2, decision P1). Returns false when rejected (the
// reply has been sent). Only called when cfg.Providers is on.
func (l *loop) resolveElement(o *orbitState, el *session.Element, reply func(string)) bool {
	originProv, originRef, sourceURL := originFromURI(el.URI)
	if originProv == "" {
		return true // not provider-shaped (playlist elements etc.): legacy path
	}
	slotProviders, err := l.sessionSlotProviders(o)
	if err != nil {
		l.log.Error("slot providers lookup failed", "orbit", o.id, "err", err)
		return true // fail open: behave as pre-provider
	}

	// track_refs is a forever-cache — reuse the ctid and every known ref.
	ctid, err := l.st.CTIDByRef(originProv, originRef)
	if err != nil {
		l.log.Error("ctid lookup failed", "origin", originRef, "err", err)
	}
	resolved := map[string]bool{originProv: true}
	var missing []string // providers that still need the cascade
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

	if len(missing) > 0 && l.resolveTrack != nil {
		res := l.resolveTrack(originProv, originRef, sourceURL, missing)
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
	} else if ctid == "" {
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
				trackLabel(*el), l.peerName(o, protocol.NodeID(slot)), providerName(prov)))
			return false
		}
	}
	return true
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
