package links

import (
	"errors"
	"testing"
)

const id = "4cOdK2wGLETKBW3PvgPWqT"
const want = "spotify:track:" + id

func TestParseTrack(t *testing.T) {
	cases := []struct {
		name string
		in   string
		out  string
		err  error
	}{
		{"plain https", "https://open.spotify.com/track/" + id, want, nil},
		{"with query", "https://open.spotify.com/track/" + id + "?si=abc123&utm_source=copy", want, nil},
		{"intl path", "https://open.spotify.com/intl-ru/track/" + id + "?si=x", want, nil},
		{"no scheme", "open.spotify.com/track/" + id, want, nil},
		{"uri form", "spotify:track:" + id, want, nil},
		{"uri inside text", "зацени spotify:track:" + id + " огонь", want, nil},
		{"link inside text", "вот https://open.spotify.com/track/" + id + " слушай", want, nil},
		{"playlist link", "https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M", "", ErrUnsupportedKind},
		{"album uri", "spotify:album:" + id, "", ErrUnsupportedKind},
		{"episode link", "https://open.spotify.com/episode/" + id, "", ErrUnsupportedKind},
		{"no link", "просто текст без ссылок", "", ErrNoLink},
		{"garbage id", "https://open.spotify.com/track/short", "", ErrNoLink},
		{"empty", "", "", ErrNoLink},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := ParseTrack(c.in)
			if !errors.Is(err, c.err) {
				t.Fatalf("err = %v, want %v", err, c.err)
			}
			if got != c.out {
				t.Fatalf("uri = %q, want %q", got, c.out)
			}
		})
	}
}
