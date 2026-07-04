// Package spotify is a minimal Web API client (client-credentials flow) used
// only to expand playlist/album links into track lists for the shared
// broadcast (U10). Playback never touches this API — the daemons do that.
package spotify

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Client struct {
	clientID     string
	clientSecret string
	http         *http.Client

	mu      sync.Mutex
	token   string
	expires time.Time
}

// New returns nil when credentials are not configured — callers treat nil as
// "playlist expansion unavailable".
func New(clientID, clientSecret string) *Client {
	if clientID == "" || clientSecret == "" {
		return nil
	}
	return &Client{
		clientID:     clientID,
		clientSecret: clientSecret,
		http:         &http.Client{Timeout: 15 * time.Second},
	}
}

func (c *Client) accessToken() (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.token != "" && time.Now().Before(c.expires.Add(-30*time.Second)) {
		return c.token, nil
	}
	req, _ := http.NewRequest("POST", "https://accounts.spotify.com/api/token",
		strings.NewReader(url.Values{"grant_type": {"client_credentials"}}.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Authorization", "Basic "+
		base64.StdEncoding.EncodeToString([]byte(c.clientID+":"+c.clientSecret)))
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("spotify token: %w", err)
	}
	defer resp.Body.Close()
	var body struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
		Error       string `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return "", fmt.Errorf("spotify token decode: %w", err)
	}
	if body.AccessToken == "" {
		return "", fmt.Errorf("spotify token: %s (http %d) — check spotify.client_id/client_secret", body.Error, resp.StatusCode)
	}
	c.token = body.AccessToken
	c.expires = time.Now().Add(time.Duration(body.ExpiresIn) * time.Second)
	return c.token, nil
}

func (c *Client) getJSON(rawURL string, out any) error {
	tok, err := c.accessToken()
	if err != nil {
		return err
	}
	req, _ := http.NewRequest("GET", rawURL, nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return fmt.Errorf("not found (private playlist? only public/link-shared ones work with app credentials)")
	}
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("spotify api http %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// Expansion holds an expanded playlist/album.
type Expansion struct {
	Title  string
	Tracks []string // spotify:track:... in order
}

// ExpandPlaylist fetches the playlist name and full track list (paged).
func (c *Client) ExpandPlaylist(id string) (*Expansion, error) {
	var meta struct {
		Name string `json:"name"`
	}
	if err := c.getJSON("https://api.spotify.com/v1/playlists/"+id+"?fields=name", &meta); err != nil {
		return nil, err
	}
	exp := &Expansion{Title: meta.Name}
	next := "https://api.spotify.com/v1/playlists/" + id + "/tracks?limit=100&fields=next,items(track(uri,is_local))"
	for next != "" {
		var page struct {
			Next  string `json:"next"`
			Items []struct {
				Track struct {
					URI     string `json:"uri"`
					IsLocal bool   `json:"is_local"`
				} `json:"track"`
			} `json:"items"`
		}
		if err := c.getJSON(next, &page); err != nil {
			return nil, err
		}
		for _, it := range page.Items {
			if !it.Track.IsLocal && strings.HasPrefix(it.Track.URI, "spotify:track:") {
				exp.Tracks = append(exp.Tracks, it.Track.URI)
			}
		}
		next = page.Next
	}
	if len(exp.Tracks) == 0 {
		return nil, fmt.Errorf("playlist has no playable tracks")
	}
	return exp, nil
}

// ExpandAlbum fetches the album name and its track list (paged).
func (c *Client) ExpandAlbum(id string) (*Expansion, error) {
	var meta struct {
		Name string `json:"name"`
	}
	if err := c.getJSON("https://api.spotify.com/v1/albums/"+id+"?fields=name", &meta); err != nil {
		return nil, err
	}
	exp := &Expansion{Title: meta.Name}
	next := "https://api.spotify.com/v1/albums/" + id + "/tracks?limit=50"
	for next != "" {
		var page struct {
			Next  string `json:"next"`
			Items []struct {
				URI string `json:"uri"`
			} `json:"items"`
		}
		if err := c.getJSON(next, &page); err != nil {
			return nil, err
		}
		for _, it := range page.Items {
			if strings.HasPrefix(it.URI, "spotify:track:") {
				exp.Tracks = append(exp.Tracks, it.URI)
			}
		}
		next = page.Next
	}
	if len(exp.Tracks) == 0 {
		return nil, fmt.Errorf("album has no tracks")
	}
	return exp, nil
}
