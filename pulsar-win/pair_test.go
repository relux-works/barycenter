package main

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestPairSuccess(t *testing.T) {
	var gotMethod, gotPath, gotBody, gotContentType string
	token := strings.Repeat("cd", 32)
	doer := httpDoerFunc(func(r *http.Request) (*http.Response, error) {
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotContentType = r.Header.Get("Content-Type")
		raw, _ := io.ReadAll(r.Body)
		gotBody = string(raw)
		body, _ := json.Marshal(map[string]any{
			"orbit_id": 3, "slot": "b", "token": token, "ws_url": "wss://coord/ws",
		})
		return jsonResponse(http.StatusOK, body), nil
	})

	creds, err := pairWithDoer("https://coord.example/", "ABCD1234", doer)
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
		doer := httpDoerFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: tc.status, Header: make(http.Header), Body: io.NopCloser(strings.NewReader("canary secret body"))}, nil
		})
		_, err := pairWithDoer("https://coord.example", "X", doer)
		if err == nil || !strings.Contains(err.Error(), tc.wantSub) {
			t.Fatalf("status %d: err %v, want substring %q", tc.status, err, tc.wantSub)
		}
	}
}

func TestPairRejectsGarbageResponse(t *testing.T) {
	doer := httpDoerFunc(func(*http.Request) (*http.Response, error) {
		return jsonResponse(http.StatusOK, []byte("not json")), nil
	})
	if _, err := pairWithDoer("https://coord.example", "X", doer); err == nil || !strings.Contains(err.Error(), "непонятный ответ") {
		t.Fatalf("err %v, want decode failure", err)
	}
}

func TestPairRejectsInvalidCredentials(t *testing.T) {
	doer := httpDoerFunc(func(*http.Request) (*http.Response, error) {
		body, _ := json.Marshal(map[string]any{
			"orbit_id": 1, "slot": "zzz", "token": "short", "ws_url": ":bad:",
		})
		return jsonResponse(http.StatusOK, body), nil
	})
	if _, err := pairWithDoer("https://coord.example", "X", doer); err == nil {
		t.Fatal("invalid credentials shape must be rejected")
	}
}
