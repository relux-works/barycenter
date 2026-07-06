// Package resolver maps a track reference of one provider to all providers
// active in an orbit (spec-providers §3): same-provider -> ISRC (toward
// Spotify only) -> Odesli -> metadata scoring -> unresolved. Results are
// cached forever in the store; /resolve overrides with method=manual.
package resolver

import (
	"regexp"
	"strings"
)

// Candidate is a track offered by a provider client for scoring.
type Candidate struct {
	Ref        string
	Title      string
	Artists    []string
	DurationMS int64
}

// ProviderClient is the minimal surface the cascade needs per provider.
type ProviderClient interface {
	// TrackByRef returns canonical metadata for a ref (title/artists/duration/isrc).
	TrackByRef(ref string) (Candidate, string, error) // candidate, isrc ("" if none)
	// SearchISRC returns a ref by ISRC ("" if unsupported/not found).
	SearchISRC(isrc string) (string, error)
	// Search returns scored candidates for "artist title".
	Search(query string) ([]Candidate, error)
}

// OdesliClient resolves a source URL into per-platform refs.
type OdesliClient interface {
	Links(sourceURL string) (map[string]string, error) // provider -> ref
}

const durationToleranceMS = 2000

var junkRe = regexp.MustCompile(`(?i)\s*[\(\[-][^)\]]*(feat\.|remaster|deluxe|bonus|edition)[^)\]]*[\)\]]?`)
var penaltyRe = regexp.MustCompile(`(?i)\b(cover|karaoke|tribute|live)\b`)

// Normalize prepares a title for comparison (spec-providers §3 rules).
func Normalize(title string) string {
	t := strings.ToLower(strings.TrimSpace(title))
	t = junkRe.ReplaceAllString(t, "")
	return strings.Join(strings.Fields(t), " ")
}

// Similarity is a trigram Dice coefficient over normalized titles.
func Similarity(a, b string) float64 {
	a, b = Normalize(a), Normalize(b)
	if a == b {
		return 1
	}
	ta, tb := trigrams(a), trigrams(b)
	if len(ta) == 0 || len(tb) == 0 {
		return 0
	}
	inter := 0
	for g := range ta {
		if tb[g] {
			inter++
		}
	}
	return 2 * float64(inter) / float64(len(ta)+len(tb))
}

func trigrams(s string) map[string]bool {
	out := map[string]bool{}
	r := []rune("  " + s + "  ")
	for i := 0; i+3 <= len(r); i++ {
		out[string(r[i:i+3])] = true
	}
	return out
}

func artistsIntersect(a, b []string) bool {
	set := map[string]bool{}
	for _, x := range a {
		set[strings.ToLower(strings.TrimSpace(x))] = true
	}
	for _, y := range b {
		if set[strings.ToLower(strings.TrimSpace(y))] {
			return true
		}
	}
	return false
}

// Score rates a candidate against the origin (0 = reject).
func Score(origin, cand Candidate) float64 {
	if !artistsIntersect(origin.Artists, cand.Artists) {
		return 0
	}
	if abs64(origin.DurationMS-cand.DurationMS) > durationToleranceMS {
		return 0
	}
	if penaltyRe.MatchString(cand.Title) && !penaltyRe.MatchString(origin.Title) {
		return 0
	}
	sim := Similarity(origin.Title, cand.Title)
	if sim < 0.9 {
		return 0
	}
	return sim
}

func abs64(v int64) int64 {
	if v < 0 {
		return -v
	}
	return v
}

// Result of a cascade run for one target provider.
type Result struct {
	Ref    string
	Method string // same | isrc | odesli | metadata | unresolved
	Score  float64
}

// Resolve maps origin (provider, ref, url) onto targetProvider.
func Resolve(origin Candidate, originProvider, originISRC, sourceURL, targetProvider string,
	clients map[string]ProviderClient, odesli OdesliClient) Result {

	if targetProvider == originProvider {
		return Result{Ref: origin.Ref, Method: "same", Score: 1}
	}

	// ISRC works only toward Spotify (Yandex neither returns nor searches it).
	if targetProvider == "spotify" && originISRC != "" {
		if c, ok := clients["spotify"]; ok {
			if ref, err := c.SearchISRC(originISRC); err == nil && ref != "" {
				return Result{Ref: ref, Method: "isrc", Score: 1}
			}
		}
	}

	if odesli != nil && sourceURL != "" {
		if links, err := odesli.Links(sourceURL); err == nil {
			if ref := links[targetProvider]; ref != "" {
				// Duration guard applies to every method (§3).
				if c, ok := clients[targetProvider]; ok {
					if cand, _, err := c.TrackByRef(ref); err == nil &&
						abs64(cand.DurationMS-origin.DurationMS) > durationToleranceMS {
						return Result{Method: "unresolved"}
					}
				}
				return Result{Ref: ref, Method: "odesli", Score: 0.99}
			}
		}
	}

	if c, ok := clients[targetProvider]; ok {
		query := strings.Join(origin.Artists, " ") + " " + Normalize(origin.Title)
		if cands, err := c.Search(query); err == nil {
			best, bestScore := Candidate{}, 0.0
			for _, cand := range cands {
				if s := Score(origin, cand); s > bestScore {
					best, bestScore = cand, s
				}
			}
			if bestScore > 0 {
				return Result{Ref: best.Ref, Method: "metadata", Score: bestScore}
			}
		}
	}
	return Result{Method: "unresolved"}
}
