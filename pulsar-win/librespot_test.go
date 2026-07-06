package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

type daemonRequest struct {
	Method string
	Path   string
	Body   string
}

func newFakeDaemonServer(t *testing.T, handler func(w http.ResponseWriter, r *http.Request)) (*DaemonClient, *[]daemonRequest) {
	t.Helper()
	var requests []daemonRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		requests = append(requests, daemonRequest{r.Method, r.URL.Path, string(raw)})
		handler(w, r)
	}))
	t.Cleanup(srv.Close)
	return newDaemonClientBase(srv.URL), &requests
}

func TestDaemonPlayPausedAndSeekBodies(t *testing.T) {
	client, requests := newFakeDaemonServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	ctx := context.Background()
	if err := client.PlayPaused(ctx, "spotify:track:x"); err != nil {
		t.Fatal(err)
	}
	if err := client.Seek(ctx, 63000); err != nil {
		t.Fatal(err)
	}
	if err := client.Resume(ctx); err != nil {
		t.Fatal(err)
	}
	if err := client.AddToQueue(ctx, "spotify:track:y"); err != nil {
		t.Fatal(err)
	}

	reqs := *requests
	if len(reqs) != 4 {
		t.Fatalf("%d requests, want 4", len(reqs))
	}
	// Two-step load part 1: play the uri already paused (spec 6.3).
	if reqs[0].Path != "/player/play" || reqs[0].Body != `{"uri":"spotify:track:x","paused":true}` {
		t.Fatalf("play request: %+v", reqs[0])
	}
	if reqs[1].Path != "/player/seek" || reqs[1].Body != `{"position":63000,"relative":false}` {
		t.Fatalf("seek request: %+v", reqs[1])
	}
	if reqs[2].Path != "/player/resume" || reqs[3].Path != "/player/add_to_queue" {
		t.Fatalf("paths: %+v %+v", reqs[2], reqs[3])
	}
}

func TestDaemonPlaybackReady(t *testing.T) {
	ready := false
	client, _ := newFakeDaemonServer(t, func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"playback_ready": ready})
	})
	if client.PlaybackReady(context.Background()) {
		t.Fatal("not ready yet")
	}
	ready = true
	if !client.PlaybackReady(context.Background()) {
		t.Fatal("ready now")
	}
}

func TestDaemonStatusEmptyBodyPreLogin(t *testing.T) {
	client, _ := newFakeDaemonServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Pre-login /status returns an empty body (spike, spec 6.2).
	})
	st, err := client.Status(context.Background())
	if err != nil {
		t.Fatalf("empty body must not error: %v", err)
	}
	if st.Track != nil || st.Paused != nil {
		t.Fatalf("empty status expected, got %+v", st)
	}
}

func TestDaemonStatusParsesTrack(t *testing.T) {
	client, _ := newFakeDaemonServer(t, func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, `{"username":"u","paused":true,"buffering":false,
			"track":{"uri":"spotify:track:x","name":"N","position":63012,"duration":180000},
			"future_field":42}`)
	})
	st, err := client.Status(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if !boolVal(st.Paused) || boolVal(st.Buffering) {
		t.Fatalf("flags: %+v", st)
	}
	if st.Track == nil || st.Track.URI != "spotify:track:x" || st.Track.Position != 63012 {
		t.Fatalf("track: %+v", st.Track)
	}
}

func TestDaemonHTTPErrorSurfaces(t *testing.T) {
	client, _ := newFakeDaemonServer(t, func(w http.ResponseWriter, r *http.Request) {
		// Unavailable uri = HTTP 500 (spike: treat as load_failed).
		http.Error(w, "boom", http.StatusInternalServerError)
	})
	err := client.PlayPaused(context.Background(), "spotify:track:gone")
	if err == nil {
		t.Fatal("HTTP 500 must error")
	}
}
