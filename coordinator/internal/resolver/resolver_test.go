package resolver

import "testing"

type fakeClient struct {
	isrcRef string
	cands   []Candidate
	byRef   map[string]Candidate
}

func (f *fakeClient) TrackByRef(ref string) (Candidate, string, error) {
	return f.byRef[ref], "", nil
}
func (f *fakeClient) SearchISRC(isrc string) (string, error) { return f.isrcRef, nil }
func (f *fakeClient) Search(q string) ([]Candidate, error)   { return f.cands, nil }

type fakeOdesli struct{ links map[string]string }

func (f *fakeOdesli) Links(u string) (map[string]string, error) { return f.links, nil }

var origin = Candidate{Ref: "spotify:track:X", Title: "Astral Entrance", Artists: []string{"Beast In Black"}, DurationMS: 214000}

func TestSameProviderShortCircuits(t *testing.T) {
	r := Resolve(origin, "spotify", "ISRC1", "url", "spotify", nil, nil)
	if r.Method != "same" || r.Ref != origin.Ref {
		t.Fatalf("%+v", r)
	}
}

func TestISRCTowardSpotifyOnly(t *testing.T) {
	sp := &fakeClient{isrcRef: "spotify:track:BYISRC"}
	r := Resolve(Candidate{Ref: "1:2", Title: "T", Artists: []string{"A"}, DurationMS: 200000},
		"yandex", "ISRC1", "", "spotify", map[string]ProviderClient{"spotify": sp}, nil)
	if r.Method != "isrc" || r.Ref != "spotify:track:BYISRC" {
		t.Fatalf("%+v", r)
	}
	// toward yandex ISRC must be skipped (no client support) -> unresolved
	r = Resolve(origin, "spotify", "ISRC1", "", "yandex", map[string]ProviderClient{}, nil)
	if r.Method != "unresolved" {
		t.Fatalf("%+v", r)
	}
}

func TestOdesliWithDurationGuard(t *testing.T) {
	ya := &fakeClient{byRef: map[string]Candidate{"111:222": {Ref: "111:222", DurationMS: 213800}}}
	od := &fakeOdesli{links: map[string]string{"yandex": "111:222"}}
	r := Resolve(origin, "spotify", "", "https://open.spotify.com/track/X", "yandex",
		map[string]ProviderClient{"yandex": ya}, od)
	if r.Method != "odesli" || r.Ref != "111:222" {
		t.Fatalf("%+v", r)
	}
	// radio edit (>2s off) must be rejected even via odesli
	ya.byRef["111:222"] = Candidate{Ref: "111:222", DurationMS: 190000}
	r = Resolve(origin, "spotify", "", "url", "yandex", map[string]ProviderClient{"yandex": ya}, od)
	if r.Method != "unresolved" {
		t.Fatalf("radio edit slipped: %+v", r)
	}
}

func TestMetadataScoring(t *testing.T) {
	ya := &fakeClient{cands: []Candidate{
		{Ref: "bad:1", Title: "Astral Entrance (Karaoke Version)", Artists: []string{"Beast In Black"}, DurationMS: 214100},
		{Ref: "good:1", Title: "Astral Entrance (Remastered 2020)", Artists: []string{"Beast In Black"}, DurationMS: 213500},
		{Ref: "far:1", Title: "Astral Entrance", Artists: []string{"Beast In Black"}, DurationMS: 250000},
	}}
	r := Resolve(origin, "spotify", "", "", "yandex", map[string]ProviderClient{"yandex": ya}, nil)
	if r.Method != "metadata" || r.Ref != "good:1" {
		t.Fatalf("%+v", r)
	}
}

func TestUnresolvedBeatsAlmost(t *testing.T) {
	ya := &fakeClient{cands: []Candidate{
		{Ref: "other:1", Title: "Completely Different Song", Artists: []string{"Beast In Black"}, DurationMS: 214000},
	}}
	r := Resolve(origin, "spotify", "", "", "yandex", map[string]ProviderClient{"yandex": ya}, nil)
	if r.Method != "unresolved" {
		t.Fatalf("almost-match slipped: %+v", r)
	}
}
