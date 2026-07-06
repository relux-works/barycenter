// HTTP clients for the cascade: Odesli and a minimal unofficial Yandex Music
// client (track/search only — playback stays on the node). Spotify ISRC
// search lives in internal/spotify and is adapted in the coordinator wiring.
package resolver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// --- Odesli (song.link) ---

type Odesli struct {
	Base   string // default https://api.song.link
	APIKey string // optional (developers@song.link)
	HTTP   *http.Client
}

func NewOdesli(apiKey string) *Odesli {
	return &Odesli{Base: "https://api.song.link", APIKey: apiKey,
		HTTP: &http.Client{Timeout: 15 * time.Second}}
}

// Links implements OdesliClient: provider -> our internal ref format.
func (o *Odesli) Links(sourceURL string) (map[string]string, error) {
	q := url.Values{"url": {sourceURL}}
	if o.APIKey != "" {
		q.Set("key", o.APIKey)
	}
	resp, err := o.HTTP.Get(o.Base + "/v1-alpha.1/links?" + q.Encode())
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("odesli http %d", resp.StatusCode)
	}
	var body struct {
		LinksByPlatform map[string]struct {
			URL string `json:"url"`
		} `json:"linksByPlatform"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return nil, err
	}
	out := map[string]string{}
	if l, ok := body.LinksByPlatform["spotify"]; ok {
		if id := lastPathSegment(l.URL, "/track/"); id != "" {
			out["spotify"] = "spotify:track:" + id
		}
	}
	if l, ok := body.LinksByPlatform["yandex"]; ok {
		if tr, al := yandexIDs(l.URL); tr != "" {
			out["yandex"] = tr + ":" + al
		}
	}
	return out, nil
}

func lastPathSegment(u, marker string) string {
	i := strings.Index(u, marker)
	if i < 0 {
		return ""
	}
	rest := u[i+len(marker):]
	if j := strings.IndexAny(rest, "?&/"); j >= 0 {
		rest = rest[:j]
	}
	return rest
}

func yandexIDs(u string) (track, album string) {
	album = lastPathSegment(u, "/album/")
	track = lastPathSegment(u, "/track/")
	return
}

// --- Yandex Music (unofficial, resolve/availability only) ---

type Yandex struct {
	Base  string // default https://api.music.yandex.net
	Token string // OAuth of the querying account (see spec-providers §0)
	HTTP  *http.Client
}

func NewYandex(token string) *Yandex {
	return &Yandex{Base: "https://api.music.yandex.net", Token: token,
		HTTP: &http.Client{Timeout: 15 * time.Second}}
}

func (y *Yandex) get(path string, out any) error {
	req, _ := http.NewRequest("GET", y.Base+path, nil)
	if y.Token != "" {
		req.Header.Set("Authorization", "OAuth "+y.Token)
	}
	resp, err := y.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("yandex http %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

type yaTrack struct {
	ID         json.Number `json:"id"`
	Title      string      `json:"title"`
	DurationMS int64       `json:"durationMs"`
	Available  bool        `json:"available"`
	Albums     []struct {
		ID json.Number `json:"id"`
	} `json:"albums"`
	Artists []struct {
		Name string `json:"name"`
	} `json:"artists"`
}

func (t yaTrack) candidate() Candidate {
	ref := t.ID.String() + ":"
	if len(t.Albums) > 0 {
		ref += t.Albums[0].ID.String()
	}
	var artists []string
	for _, a := range t.Artists {
		artists = append(artists, a.Name)
	}
	return Candidate{Ref: ref, Title: t.Title, Artists: artists, DurationMS: t.DurationMS}
}

// TrackByRef: ref "<track_id>:<album_id>" (album part optional).
func (y *Yandex) TrackByRef(ref string) (Candidate, string, error) {
	trackID := ref
	if i := strings.IndexByte(ref, ':'); i >= 0 {
		trackID = ref[:i]
	}
	if _, err := strconv.ParseInt(trackID, 10, 64); err != nil {
		return Candidate{}, "", fmt.Errorf("bad yandex ref %q", ref)
	}
	var body struct {
		Result []yaTrack `json:"result"`
	}
	if err := y.get("/tracks/"+trackID, &body); err != nil {
		return Candidate{}, "", err
	}
	if len(body.Result) == 0 {
		return Candidate{}, "", fmt.Errorf("yandex track %s not found", trackID)
	}
	// Yandex exposes no ISRC (spec-providers §2).
	return body.Result[0].candidate(), "", nil
}

func (y *Yandex) SearchISRC(string) (string, error) { return "", nil } // unsupported

func (y *Yandex) Search(query string) ([]Candidate, error) {
	var body struct {
		Result struct {
			Tracks struct {
				Results []yaTrack `json:"results"`
			} `json:"tracks"`
		} `json:"result"`
	}
	q := url.Values{"text": {query}, "type": {"track"}, "page": {"0"}}
	if err := y.get("/search?"+q.Encode(), &body); err != nil {
		return nil, err
	}
	var out []Candidate
	for _, t := range body.Result.Tracks.Results {
		out = append(out, t.candidate())
	}
	return out, nil
}
