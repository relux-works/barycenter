package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestTargetsInboxClientProjectsCapabilitiesAndUsesOnlyExplicitActions(t *testing.T) {
	ref := "trf_" + strings.Repeat("A", 43)
	inboxID := "ib_01J00000000000000000000000"
	historyID := "hi_01J00000000000000000000000"
	label := `{"key":"action.replay","en":"Replay","ru":"Повторить"}`
	requested := `{"key":"delivery.overlay","en":"Overlay","ru":"Поверх"}`
	doer := &phaseOneScriptedDoer{}
	doer.handle = func(request *http.Request, _ int) (*http.Response, error) {
		if request.Header.Get("Authorization") != "Bearer "+phaseOneTestBundle().Control.ControlToken || request.Header.Get("Accept") != "application/json" {
			t.Fatalf("missing authenticated JSON boundary: %v", request.Header)
		}
		switch request.Method + " " + request.URL.Path {
		case "GET /v1/transmission-targets":
			return phaseOneJSONResponse(request, 200, fmt.Sprintf(`{"contract":"%s","targets":[{"reference":%q,"kind":"barycenter","capability_state":"known","capabilities":["media_clip_v1","overlay_mix_v1"],"expires_at":"2027-07-16T00:00:00Z","presentation":{"label":{"key":"target.home","en":"Home","ru":"Дом"},"capability_state":{"key":"capability.known","en":"Ready","ru":"Готов"},"capabilities":["media_clip_v1","overlay_mix_v1"]}}]}`, targetsInboxContract, ref)), nil
		case "GET /v1/inbox":
			if request.URL.Query().Get("limit") != "20" {
				t.Fatalf("inbox query=%s", request.URL.RawQuery)
			}
			return phaseOneJSONResponse(request, 200, fmt.Sprintf(`{"contract":"%s","items":[{"id":%q,"history_item_id":%q,"media":{"title":"Voice"},"availability":"available","expires_at":"2027-07-16T00:00:00Z","actions":["replay","dismiss","report","block_actor"],"presentation":{"sender":{"key":"sender.one","en":"Ivan","ru":"Иван"},"source":{"key":"source.air","en":"Air","ru":"Эфир"},"requested_delivery":%s,"effective_delivery":%s,"receipt":{"key":"receipt.missed","en":"Missed","ru":"Пропущено"},"actions":[{"action":"replay","label":%s},{"action":"dismiss","label":%s},{"action":"report","label":%s},{"action":"block_actor","label":%s}]}}],"next_cursor":null}`, targetsInboxContract, inboxID, historyID, requested, requested, label, label, label, label)), nil
		case "GET /v1/history":
			return phaseOneJSONResponse(request, 200, fmt.Sprintf(`{"contract":"p1-history-presence-telegram-v1","items":[{"history_item_id":%q,"media":{"title":"Voice"},"target_counts":{"played":1,"other":2},"actions":["delete","report","block_actor"],"presentation":{"status":{"key":"status.partial","en":"Partial","ru":"Частично"},"actions":[{"action":"delete","label":%s},{"action":"report","label":%s},{"action":"block_actor","label":%s}]}}],"next_cursor":null}`, historyID, label, label, label)), nil
		case "GET /v1/content-policy/acceptance":
			return phaseOneJSONResponse(request, 200, `{"contract":"p2-content-policy-consent.v1","current":true,"terms_accepted":true}`), nil
		case "GET /v1/history/" + historyID + "/receipts":
			return phaseOneJSONResponse(request, 200, fmt.Sprintf(`{"contract":"%s","history_item_id":%q,"items":[{"target_label":"Living room","presentation":{"status":{"key":"receipt.played","en":"Played","ru":"Проиграно"}}}],"next_cursor":null}`, targetsInboxContract, historyID)), nil
		case "POST /v1/inbox/" + inboxID + "/replays":
			if request.Header.Get("Idempotency-Key") != "windows-inbox-replay-test" {
				t.Fatal("replay key missing")
			}
			return phaseOneJSONResponse(request, 201, fmt.Sprintf(`{"contract":"%s","history_item_id":%q,"requested_delivery":"overlay","effective_delivery":"overlay","reused":false}`, targetsInboxContract, historyID)), nil
		case "DELETE /v1/inbox/" + inboxID:
			return phaseOneJSONResponse(request, 200, fmt.Sprintf(`{"contract":"%s","item":{"id":%q,"availability":"dismissed"}}`, targetsInboxContract, inboxID)), nil
		case "POST /v1/history/" + historyID + "/actions/delete":
			return phaseOneJSONResponse(request, 200, fmt.Sprintf(`{"history_item_id":%q,"deleted":true}`, historyID)), nil
		case "POST /v1/history/" + historyID + "/actions/report":
			return phaseOneJSONResponse(request, 201, fmt.Sprintf(`{"history_item_id":%q,"reused":false}`, historyID)), nil
		case "POST /v1/history/" + historyID + "/actions/block_actor":
			return phaseOneJSONResponse(request, 201, `{"block_id":"bl_01J00000000000000000000000","reused":false}`), nil
		default:
			t.Fatalf("unexpected request %s %s", request.Method, request.URL)
			return nil, nil
		}
	}
	client, err := NewTargetsInboxAppClient(phaseOneTestBundle(), doer)
	if err != nil {
		t.Fatal(err)
	}
	projection, err := client.Projection(context.Background())
	if err != nil || len(projection.Targets) != 1 || projection.Targets[0].Reference != ref || len(projection.Inbox.Items) != 1 || len(projection.History.Items) != 1 || projection.ContentPolicyState != "current" {
		t.Fatalf("projection=%+v err=%v", projection, err)
	}
	receipts, err := client.Receipts(context.Background(), historyID, "")
	if err != nil || len(receipts.Items) != 1 || receipts.Items[0].TargetLabel != "Living room" {
		t.Fatalf("receipts=%+v err=%v", receipts, err)
	}
	if outcome, err := client.ReplayInbox(context.Background(), inboxID, PhaseOneOverlay, "windows-inbox-replay-test"); err != nil || outcome != "replay_accepted" {
		t.Fatalf("replay=%s err=%v", outcome, err)
	}
	if outcome, err := client.DismissInbox(context.Background(), inboxID); err != nil || outcome != "inbox_dismissed" {
		t.Fatalf("dismiss=%s err=%v", outcome, err)
	}
	if outcome, err := client.DeleteTargetsHistory(context.Background(), historyID); err != nil || outcome != "media_deleted" {
		t.Fatalf("delete=%s err=%v", outcome, err)
	}
	if outcome, err := client.ReportTargetsHistory(context.Background(), historyID, PhaseOneReportSpam, ""); err != nil || outcome != "report_received" {
		t.Fatalf("report=%s err=%v", outcome, err)
	}
	if outcome, err := client.MuteTargetsHistorySender(context.Background(), historyID, "windows-mute-sender-test"); err != nil || outcome != "sender_blocked" {
		t.Fatalf("mute=%s err=%v", outcome, err)
	}
	doer.mu.Lock()
	defer doer.mu.Unlock()
	for _, request := range doer.requests {
		if strings.Contains(request.URL.Path, "/media/") || strings.Contains(request.URL.Path, "download") || strings.Contains(request.URL.RawQuery, "token") {
			t.Fatalf("projection caused playback/secret read: %s", request.URL)
		}
	}
}

func TestTargetsInboxClientFailsClosedOnDuplicateOpaqueReferences(t *testing.T) {
	ref := "trf_" + strings.Repeat("Z", 43)
	doer := &phaseOneScriptedDoer{handle: func(request *http.Request, _ int) (*http.Response, error) {
		body := fmt.Sprintf(`{"contract":"%s","targets":[{"reference":%q,"kind":"pulsar","capability_state":"unknown","capabilities":[],"expires_at":"2027-07-16T00:00:00Z","presentation":{"label":{"key":"target.one","en":"One","ru":"Один"},"capabilities":[]}},{"reference":%q,"kind":"pulsar","capability_state":"unknown","capabilities":[],"expires_at":"2027-07-16T00:00:00Z","presentation":{"label":{"key":"target.two","en":"Two","ru":"Два"},"capabilities":[]}}]}`, targetsInboxContract, ref, ref)
		return phaseOneJSONResponse(request, 200, body), nil
	}}
	client, err := NewTargetsInboxAppClient(phaseOneTestBundle(), doer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.targets(context.Background()); err == nil {
		t.Fatal("duplicate target capability accepted")
	}
}

func TestPhaseOneExplicitTransmissionSortsOpaqueReferences(t *testing.T) {
	refA, refB := "trf_"+strings.Repeat("A", 43), "trf_"+strings.Repeat("B", 43)
	mediaID := "m_" + strings.Repeat("M", 26)
	doer := &phaseOneScriptedDoer{handle: func(request *http.Request, _ int) (*http.Response, error) {
		body, _ := io.ReadAll(request.Body)
		text := string(body)
		if request.URL.Path != "/v1/transmissions" || !strings.Contains(text, `"kind":"explicit"`) || strings.Index(text, refA) > strings.Index(text, refB) || !strings.Contains(text, `"include_origin":true`) {
			t.Fatalf("explicit request=%s body=%s", request.URL, text)
		}
		return phaseOneJSONResponse(request, 201, fmt.Sprintf(`{"transmission_id":"tr_%s","media_id":%q,"requested_delivery":"overlay","effective_delivery":"overlay","status":"accepted","reused":false}`, strings.Repeat("T", 26), mediaID)), nil
	}}
	client, err := NewPhaseOneAppClient(phaseOneTestBundle(), doer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.TransmitExplicit(context.Background(), mediaID, []string{refB, refA}, true, PhaseOneOverlay, PhaseOneFile, "windows-explicit-transmission-test"); err != nil {
		t.Fatal(err)
	}
}
