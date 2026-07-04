// Package links parses Spotify track references from chat messages (spec 9.1).
// Accepted: open.spotify.com/track/<id> (intl paths and query params dropped)
// and spotify:track:<id>. Playlists/albums are recognized but unsupported in MVP.
package links

import (
	"errors"
	"net/url"
	"regexp"
	"strings"
)

var (
	// ErrNoLink: the text contains no Spotify reference at all.
	ErrNoLink = errors.New("no spotify link found")
	// ErrUnsupportedKind: a Spotify link of a non-track kind (playlist, album, ...).
	ErrUnsupportedKind = errors.New("only track links are supported")
)

var base62ID = regexp.MustCompile(`^[0-9A-Za-z]{22}$`)

var uriRe = regexp.MustCompile(`spotify:([a-z]+):([0-9A-Za-z]{22})`)

// Ref is any recognized Spotify reference (U10: playlists/albums join tracks).
type Ref struct {
	Kind string // "track" | "playlist" | "album"
	ID   string
	URI  string // canonical spotify:<kind>:<id>
}

// ParseRef extracts the first track/playlist/album reference from free text.
// Returns ErrNoLink when nothing Spotify-shaped is present and
// ErrUnsupportedKind for other Spotify kinds (episode, show, artist...).
func ParseRef(text string) (Ref, error) {
	found := false
	consider := func(kind, id string) (Ref, bool) {
		switch kind {
		case "track", "playlist", "album":
			return Ref{Kind: kind, ID: id, URI: "spotify:" + kind + ":" + id}, true
		}
		return Ref{}, false
	}

	if m := uriRe.FindStringSubmatch(text); m != nil {
		found = true
		if ref, ok := consider(m[1], m[2]); ok {
			return ref, nil
		}
	}
	for _, field := range strings.Fields(text) {
		if !strings.Contains(field, "open.spotify.com/") {
			continue
		}
		raw := field
		if !strings.Contains(raw, "://") {
			raw = "https://" + raw
		}
		u, err := url.Parse(raw)
		if err != nil || !strings.EqualFold(u.Hostname(), "open.spotify.com") {
			continue
		}
		segs := strings.Split(strings.Trim(u.Path, "/"), "/")
		if len(segs) > 0 && strings.HasPrefix(segs[0], "intl-") {
			segs = segs[1:]
		}
		if len(segs) < 2 || !base62ID.MatchString(segs[1]) {
			continue
		}
		found = true
		if ref, ok := consider(segs[0], segs[1]); ok {
			return ref, nil
		}
	}
	if found {
		return Ref{}, ErrUnsupportedKind
	}
	return Ref{}, ErrNoLink
}

// ParseTrack extracts the first Spotify track reference from free text and
// returns its canonical URI "spotify:track:<id>".
func ParseTrack(text string) (string, error) {
	found := false

	if m := uriRe.FindStringSubmatch(text); m != nil {
		found = true
		if m[1] == "track" {
			return "spotify:track:" + m[2], nil
		}
	}

	for _, field := range strings.Fields(text) {
		if !strings.Contains(field, "open.spotify.com/") {
			continue
		}
		raw := field
		if !strings.Contains(raw, "://") {
			raw = "https://" + raw
		}
		u, err := url.Parse(raw)
		if err != nil || !strings.EqualFold(u.Hostname(), "open.spotify.com") {
			continue
		}
		segs := strings.Split(strings.Trim(u.Path, "/"), "/")
		// Drop intl-xx prefix: open.spotify.com/intl-ru/track/<id>
		if len(segs) > 0 && strings.HasPrefix(segs[0], "intl-") {
			segs = segs[1:]
		}
		if len(segs) < 2 {
			continue
		}
		kind, id := segs[0], segs[1]
		if !base62ID.MatchString(id) {
			continue
		}
		found = true
		if kind == "track" {
			return "spotify:track:" + id, nil
		}
	}

	if found {
		return "", ErrUnsupportedKind
	}
	return "", ErrNoLink
}
