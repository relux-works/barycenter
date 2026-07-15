package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

type phaseOneScriptedDoer struct {
	mu       sync.Mutex
	requests []*http.Request
	handle   func(*http.Request, int) (*http.Response, error)
}

func (d *phaseOneScriptedDoer) Do(request *http.Request) (*http.Response, error) {
	d.mu.Lock()
	index := len(d.requests)
	d.requests = append(d.requests, request)
	d.mu.Unlock()
	return d.handle(request, index)
}

func phaseOneJSONResponse(request *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(body)), Request: request,
	}
}

func phaseOneTestBundle() CredentialBundle {
	return CredentialBundle{
		Version:           credentialBundleVersion,
		Node:              &NodeCredential{OrbitID: 1, Slot: "a", NodeToken: strings.Repeat("ab", 32), WSURL: "wss://coord.example/ws"},
		Control:           &ControlCredential{ActorID: 2, OrbitID: 1, Role: "primary", ControlToken: strings.Repeat("cd", 32), Context: ControlContextActive},
		CoordinatorOrigin: "https://coord.example",
	}
}

func TestPhaseOneClientCanonicalAuthenticatedContracts(t *testing.T) {
	filePath := filepath.Join("..", "assets", "audio", "pulsar-recording-cue.wav")
	info, err := os.Stat(filePath)
	if err != nil {
		t.Fatal(err)
	}
	uploadID := "up_" + strings.Repeat("A", 26)
	mediaID := "m_" + strings.Repeat("B", 26)
	transmissionID := "tr_" + strings.Repeat("C", 26)
	historyID := "hi_" + strings.Repeat("D", 26)
	doer := &phaseOneScriptedDoer{}
	doer.handle = func(request *http.Request, index int) (*http.Response, error) {
		if got := request.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ") || strings.Contains(request.URL.RawQuery, "token") {
			t.Fatalf("request %d authentication boundary=%q url=%s", index, got, request.URL)
		}
		switch index {
		case 0:
			if request.Method != http.MethodPost || request.URL.Path != "/v1/media/uploads" || request.Header.Get("Idempotency-Key") != "windows-upload-00000000000000000000000000000001" {
				t.Fatalf("upload create=%s %s headers=%v", request.Method, request.URL, request.Header)
			}
			return phaseOneJSONResponse(request, http.StatusCreated, fmt.Sprintf(`{"upload_id":%q,"media_id":%q,"upload_token":%q,"upload_offset":0,"upload_length":%d,"expires_at":"2026-07-15T00:00:00Z","status":"open"}`, uploadID, mediaID, strings.Repeat("ef", 32), info.Size())), nil
		case 1:
			if request.Method != http.MethodPut || request.URL.Path != "/v1/media/uploads/"+uploadID || request.Header.Get("Upload-Offset") != "0" {
				t.Fatalf("upload put=%s %s headers=%v", request.Method, request.URL, request.Header)
			}
			body, _ := io.ReadAll(request.Body)
			if int64(len(body)) != info.Size() {
				t.Fatalf("uploaded bytes=%d want=%d", len(body), info.Size())
			}
			return phaseOneJSONResponse(request, http.StatusOK, fmt.Sprintf(`{"upload_id":%q,"media_id":%q,"upload_offset":%d,"upload_length":%d,"expires_at":"2026-07-15T00:00:00Z","status":"completed"}`, uploadID, mediaID, info.Size(), info.Size())), nil
		case 2:
			if request.URL.Path != "/v1/transmissions" || request.Header.Get("Idempotency-Key") != "windows-transmission-00000000000000000000000000000001" {
				t.Fatalf("transmission request=%s headers=%v", request.URL, request.Header)
			}
			return phaseOneJSONResponse(request, http.StatusCreated, fmt.Sprintf(`{"transmission_id":%q,"media_id":%q,"requested_delivery":"overlay","effective_delivery":"after_current","downgrade_reason":"mandatory_target_missing_overlay_capability","status":"accepted","reused":false}`, transmissionID, mediaID)), nil
		case 3:
			return phaseOneJSONResponse(request, http.StatusOK, `{"contract":"p1-history-presence-telegram-v1","nodes":[{"slot":"a","online":true,"output_state":"ready","playback_state":"playing","effective_dnd":{"mode":"allow_all"}}]}`), nil
		case 4:
			if request.URL.Query().Get("limit") != "20" {
				t.Fatalf("history query=%s", request.URL.RawQuery)
			}
			return phaseOneJSONResponse(request, http.StatusOK, fmt.Sprintf(`{"contract":"p1-history-presence-telegram-v1","items":[{"history_item_id":%q,"direction":"sent","occurred_at":"2026-07-15T00:00:00.000Z","media":{"title":"Pulsar recording"},"requested_delivery":"overlay","effective_delivery":"after_current","downgrade_reason":"mandatory_target_missing_overlay_capability","status":"partial","reason_code":"partial_delivery","target_counts":{"played":1,"other":1},"actions":["delete","replay"]}],"next_cursor":"opaque"}`, historyID)), nil
		case 5:
			return phaseOneJSONResponse(request, http.StatusOK, fmt.Sprintf(`{"history_item_id":%q,"deleted":true}`, historyID)), nil
		case 6:
			return &http.Response{StatusCode: http.StatusNoContent, Header: http.Header{}, Body: io.NopCloser(strings.NewReader("")), Request: request}, nil
		default:
			t.Fatalf("unexpected request %d", index)
			return nil, nil
		}
	}
	client, err := NewPhaseOneAppClient(phaseOneTestBundle(), doer)
	if err != nil {
		t.Fatal(err)
	}
	upload, err := client.Upload(context.Background(), filePath, "Pulsar recording", "windows-upload-00000000000000000000000000000001")
	if err != nil || upload.MediaID != mediaID {
		t.Fatalf("upload=%+v err=%v", upload, err)
	}
	receipt, err := client.Transmit(context.Background(), mediaID, PhaseOneOwnBarycenter, PhaseOneOverlay, PhaseOneMicrophone, "windows-transmission-00000000000000000000000000000001", nil)
	if err != nil || receipt.EffectiveDelivery != PhaseOneAfterCurrent || receipt.DowngradeReason == "" {
		t.Fatalf("receipt=%+v err=%v", receipt, err)
	}
	presence, err := client.Presence(context.Background())
	if err != nil || len(presence) != 1 || !presence[0].Online {
		t.Fatalf("presence=%+v err=%v", presence, err)
	}
	history, err := client.History(context.Background(), 20, "")
	if err != nil || len(history.Items) != 1 || history.Items[0].PlayedCount != 1 || history.NextCursor != "opaque" {
		t.Fatalf("history=%+v err=%v", history, err)
	}
	if err := client.DeleteHistoryItem(context.Background(), historyID); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteMedia(context.Background(), mediaID); err != nil {
		t.Fatal(err)
	}
}

func TestPhaseOneClientRejectsRedirectAndInactiveControl(t *testing.T) {
	doer := &phaseOneScriptedDoer{handle: func(request *http.Request, _ int) (*http.Response, error) {
		redirected := request.Clone(request.Context())
		redirected.URL, _ = request.URL.Parse("https://attacker.example/v1/presence")
		return phaseOneJSONResponse(redirected, http.StatusOK, `{}`), nil
	}}
	client, err := NewPhaseOneAppClient(phaseOneTestBundle(), doer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Presence(context.Background()); err == nil || !strings.Contains(err.Error(), "redirect") {
		t.Fatalf("redirect err=%v", err)
	}
	bundle := phaseOneTestBundle()
	bundle.Control.Context = ControlContextLimited
	bundle.Control.OrbitID = 0
	bundle.Control.Role = ""
	if _, err := NewPhaseOneAppClient(bundle, nil); err == nil {
		t.Fatal("inactive control capability created a Phase 1 client")
	}
}

func TestPhaseOneClientCarriesExplicitInterruptFallbackConfirmation(t *testing.T) {
	mediaID := "m_" + strings.Repeat("A", 26)
	transmissionID := "tr_" + strings.Repeat("B", 26)
	token := "fc_" + strings.Repeat("c", 64)
	doer := &phaseOneScriptedDoer{}
	doer.handle = func(request *http.Request, index int) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		if index == 0 {
			if strings.Contains(string(body), "fallback_confirmation") {
				t.Fatalf("initial interrupt silently confirmed fallback: %s", body)
			}
			return phaseOneJSONResponse(request, http.StatusConflict, fmt.Sprintf(`{"error":{"code":"requires_confirmation","message":"Confirm fallback","details":{"confirmation_token":%q,"expires_at":"2026-07-15T00:01:00Z","alternatives":[{"delivery":"after_current","available":true}]}}}`, token)), nil
		}
		if !strings.Contains(string(body), token) || !strings.Contains(string(body), `"delivery":"after_current"`) {
			t.Fatalf("confirmation body=%s", body)
		}
		return phaseOneJSONResponse(request, http.StatusCreated, fmt.Sprintf(`{"transmission_id":%q,"media_id":%q,"requested_delivery":"interrupt","effective_delivery":"after_current","downgrade_reason":"fallback_confirmed","status":"accepted"}`, transmissionID, mediaID)), nil
	}
	client, err := NewPhaseOneAppClient(phaseOneTestBundle(), doer)
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Transmit(context.Background(), mediaID, PhaseOneOwnBarycenter, PhaseOneInterrupt, PhaseOneMicrophone, "windows-transmission-confirmation-test", nil)
	var challenged *PhaseOneClientError
	if !errors.As(err, &challenged) || challenged.Code != "requires_confirmation" || challenged.ConfirmationToken != token || len(challenged.Alternatives) != 1 {
		t.Fatalf("challenge=%#v err=%v", challenged, err)
	}
	if strings.Contains(fmt.Sprint(challenged), token) || strings.Contains(fmt.Sprintf("%#v", challenged), token) {
		t.Fatal("fallback token escaped through public error formatting")
	}
	receipt, err := client.Transmit(context.Background(), mediaID, PhaseOneOwnBarycenter, PhaseOneInterrupt, PhaseOneMicrophone, "windows-transmission-confirmation-test", &PhaseOneFallbackConfirmation{Token: token, Delivery: PhaseOneAfterCurrent})
	if err != nil || receipt.RequestedDelivery != PhaseOneInterrupt || receipt.EffectiveDelivery != PhaseOneAfterCurrent {
		t.Fatalf("receipt=%+v err=%v", receipt, err)
	}
}
