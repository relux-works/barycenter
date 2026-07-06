package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPairSuccess(t *testing.T) {
	var gotMethod, gotPath, gotBody, gotContentType string
	token := strings.Repeat("cd", 32)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		json.NewEncoder(w).Encode(map[string]any{
			"orbit_id": 3, "slot": "b", "token": token, "ws_url": "wss://coord/ws",
		})
	}))
	defer srv.Close()

	creds, err := Pair(srv.URL+"/", "ABCD1234") // trailing slash must not break the path
	if err != nil {
		t.Fatal(err)
	}
	if gotMethod != http.MethodPost || gotPath != "/pair" {
		t.Fatalf("%s %s, want POST /pair", gotMethod, gotPath)
	}
	if gotContentType != "application/json" {
		t.Fatalf("content-type %q", gotContentType)
	}
	var body map[string]string
	json.Unmarshal([]byte(gotBody), &body)
	if body["code"] != "ABCD1234" {
		t.Fatalf("body %q, want {\"code\":\"ABCD1234\"}", gotBody)
	}
	want := Credentials{OrbitID: 3, Slot: "b", Token: token, WSURL: "wss://coord/ws"}
	if creds != want {
		t.Fatalf("creds %+v, want %+v", creds, want)
	}
}

func TestPairErrorMessages(t *testing.T) {
	cases := []struct {
		status  int
		wantSub string
	}{
		{http.StatusForbidden, "код не подошёл"},
		{http.StatusConflict, "нет свободных мест"},
		{http.StatusInternalServerError, "сервер ответил 500"},
	}
	for _, tc := range cases {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "nope", tc.status)
		}))
		_, err := Pair(srv.URL, "X")
		srv.Close()
		if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
			t.Fatalf("status %d: err %v, want substring %q", tc.status, err, tc.wantSub)
		}
	}
}

func TestPairRejectsGarbageResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.WriteString(w, "not json")
	}))
	defer srv.Close()
	if _, err := Pair(srv.URL, "X"); err == nil || !strings.Contains(err.Error(), "непонятный ответ") {
		t.Fatalf("err %v, want decode failure", err)
	}
}

func TestPairRejectsInvalidCredentials(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"orbit_id": 1, "slot": "zzz", "token": "short", "ws_url": ":bad:",
		})
	}))
	defer srv.Close()
	if _, err := Pair(srv.URL, "X"); err == nil {
		t.Fatal("invalid credentials shape must be rejected")
	}
}
