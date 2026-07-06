// Minimal go-librespot HTTP API client (daemon 0.7.x OpenAPI) — mirror of
// LibrespotClient.swift's command set: playback_ready, status, two-step load
// (play paused + seek), resume/pause/stop, add_to_queue. Tolerant decoding:
// unknown fields ignored, pre-login /status returns an empty body.
//
// The /events WebSocket (metadata/seek/volume/takeover detection) is not part
// of the skeleton yet — see the TODO list in player.go.
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type DaemonTrack struct {
	URI      string `json:"uri"`
	Name     string `json:"name"`
	Position int64  `json:"position"`
	Duration int64  `json:"duration"`
}

// DaemonStatus uses pointers where the Swift client used optionals: absent
// and false are different answers during the paused-load confirmation poll.
type DaemonStatus struct {
	Username  string       `json:"username"`
	Stopped   *bool        `json:"stopped"`
	Paused    *bool        `json:"paused"`
	Buffering *bool        `json:"buffering"`
	Volume    *int         `json:"volume"`
	Track     *DaemonTrack `json:"track"`
}

type DaemonClient struct {
	base string
	http *http.Client
}

// NewDaemonClient talks to the local daemon on 127.0.0.1:apiPort.
// Context loads can exceed 5 s on a cold cache (R0 prod finding) — 30 s cap.
func NewDaemonClient(apiPort int) *DaemonClient {
	return newDaemonClientBase(fmt.Sprintf("http://127.0.0.1:%d", apiPort))
}

func newDaemonClientBase(base string) *DaemonClient {
	return &DaemonClient{
		base: strings.TrimSuffix(base, "/"),
		http: &http.Client{Timeout: 30 * time.Second},
	}
}

// PlaybackReady is GET / -> {"playback_ready": bool}; any error reads false
// (the daemon needs seconds after (re)start to authenticate).
func (d *DaemonClient) PlaybackReady(ctx context.Context) bool {
	raw, err := d.get(ctx, "/")
	if err != nil {
		return false
	}
	var root struct {
		PlaybackReady *bool `json:"playback_ready"`
	}
	if err := json.Unmarshal(raw, &root); err != nil || root.PlaybackReady == nil {
		return false
	}
	return *root.PlaybackReady
}

func (d *DaemonClient) Status(ctx context.Context) (DaemonStatus, error) {
	raw, err := d.get(ctx, "/status")
	if err != nil {
		return DaemonStatus{}, err
	}
	if len(bytes.TrimSpace(raw)) == 0 {
		// Pre-login /status returns an empty body (spike, spec 6.2).
		return DaemonStatus{}, nil
	}
	var st DaemonStatus
	if err := json.Unmarshal(raw, &st); err != nil {
		return DaemonStatus{}, fmt.Errorf("decode /status: %w", err)
	}
	return st, nil
}

// PlayPaused is the two-step load, part 1 (spec 6.3): play the uri already
// paused; the seek follows while paused.
func (d *DaemonClient) PlayPaused(ctx context.Context, uri string) error {
	return d.post(ctx, "/player/play", struct {
		URI    string `json:"uri"`
		Paused bool   `json:"paused"`
	}{uri, true})
}

func (d *DaemonClient) Seek(ctx context.Context, positionMS int64) error {
	return d.post(ctx, "/player/seek", struct {
		Position int64 `json:"position"`
		Relative bool  `json:"relative"`
	}{positionMS, false})
}

func (d *DaemonClient) Resume(ctx context.Context) error {
	return d.post(ctx, "/player/resume", struct{}{})
}

func (d *DaemonClient) Pause(ctx context.Context) error {
	return d.post(ctx, "/player/pause", struct{}{})
}

func (d *DaemonClient) Stop(ctx context.Context) error {
	return d.post(ctx, "/player/stop", struct{}{})
}

func (d *DaemonClient) AddToQueue(ctx context.Context, uri string) error {
	return d.post(ctx, "/player/add_to_queue", struct {
		URI string `json:"uri"`
	}{uri})
}

func (d *DaemonClient) get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, d.base+path, nil)
	if err != nil {
		return nil, err
	}
	resp, err := d.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	if err := checkDaemonHTTP(resp.StatusCode, path); err != nil {
		return nil, err
	}
	return raw, nil
}

func (d *DaemonClient) post(ctx context.Context, path string, body any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, d.base+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := d.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<20))
	return checkDaemonHTTP(resp.StatusCode, path)
}

func checkDaemonHTTP(status int, path string) error {
	if status >= 200 && status < 300 {
		return nil
	}
	// HTTP 500 on play = unavailable uri (spike finding: treat as load_failed,
	// distrust /status until the next play).
	return fmt.Errorf("librespot %s -> HTTP %d", path, status)
}
