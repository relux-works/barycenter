package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestSoundboardClientCueListAndManualTrigger(t *testing.T) {
	cueID := "cq_" + strings.Repeat("A", 26)
	executionID := "mx_" + strings.Repeat("B", 26)
	transmissionID := "tr_" + strings.Repeat("C", 26)
	doer := &phaseOneScriptedDoer{}
	doer.handle = func(request *http.Request, index int) (*http.Response, error) {
		if request.Header.Get("Authorization") != "Bearer "+phaseOneTestBundle().Control.ControlToken {
			t.Fatalf("authorization=%q", request.Header.Get("Authorization"))
		}
		switch index {
		case 0:
			if request.Method != http.MethodGet || request.URL.Path != "/v1/soundboard/cues" {
				t.Fatalf("list=%s %s", request.Method, request.URL)
			}
			return phaseOneJSONResponse(request, http.StatusOK, fmt.Sprintf(`{"order_revision":2,"cues":[{"cue_id":%q,"title":"Bell","source_kind":"builtin","builtin_asset_id":"pulsar.recording-cue.v1","source_sha256":"%s","source_bytes":15404,"source_duration_ms":348,"state":"active","revision":3,"source_generation":1,"position":0}]}`,
				cueID, strings.Repeat("a", 64))), nil
		case 1:
			if request.Method != http.MethodPost || request.URL.Path != "/v1/soundboard/cues/"+cueID+"/trigger" ||
				request.Header.Get("Idempotency-Key") != "windows-soundboard-trigger-0001" {
				t.Fatalf("trigger=%s %s headers=%v", request.Method, request.URL, request.Header)
			}
			body, _ := io.ReadAll(request.Body)
			if strings.Contains(string(body), "microphone") || !strings.Contains(string(body), `"delivery":"overlay"`) {
				t.Fatalf("trigger body=%s", body)
			}
			return phaseOneJSONResponse(request, http.StatusCreated, fmt.Sprintf(`{"execution_id":%q,"transmission_id":%q,"media_id":"m_%s","requested_delivery":"overlay","effective_delivery":"overlay","status":"accepted","reused":false}`,
				executionID, transmissionID, strings.Repeat("D", 26))), nil
		default:
			t.Fatalf("unexpected request %d", index)
			return nil, nil
		}
	}
	client, err := NewPhaseOneAppClient(phaseOneTestBundle(), doer)
	if err != nil {
		t.Fatal(err)
	}
	list, err := client.SoundboardCues(context.Background())
	if err != nil || list.OrderRevision != 2 || len(list.Cues) != 1 || list.Cues[0].ID != cueID || list.Cues[0].Position != 0 {
		t.Fatalf("list=%+v err=%v", list, err)
	}
	receipt, err := client.TriggerSoundboardCue(context.Background(), cueID,
		SoundboardTriggerIntent{Route: PhaseOneOwnBarycenter, Delivery: PhaseOneOverlay, IncludeOrigin: true},
		"windows-soundboard-trigger-0001")
	if err != nil || receipt.ExecutionID != executionID || receipt.TransmissionID != transmissionID || receipt.Reused {
		t.Fatalf("receipt=%+v err=%v", receipt, err)
	}
}

func TestSoundboardClientRejectsSecretsAndMalformedCueState(t *testing.T) {
	doer := &phaseOneScriptedDoer{handle: func(request *http.Request, _ int) (*http.Response, error) {
		return phaseOneJSONResponse(request, http.StatusOK,
			`{"order_revision":1,"cues":[{"cue_id":"cq_00000000000000000000000000","title":"Bad","source_kind":"builtin","builtin_asset_id":"pulsar.recording-cue.v1","source_sha256":"secret-token","source_bytes":1,"source_duration_ms":1,"state":"active","revision":1,"source_generation":1,"position":0}]}`), nil
	}}
	client, err := NewPhaseOneAppClient(phaseOneTestBundle(), doer)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.SoundboardCues(context.Background()); err == nil {
		t.Fatal("malformed cue response accepted")
	}
	if _, err := client.TriggerSoundboardCue(context.Background(), "cq_00000000000000000000000000",
		SoundboardTriggerIntent{Route: PhaseOneOwnBarycenter, Delivery: PhaseOneOverlay}, "bad key with spaces"); err == nil {
		t.Fatal("malformed idempotency key accepted")
	}
	if strings.Contains(client.String(), phaseOneTestBundle().Control.ControlToken) {
		t.Fatal("client string exposed control bearer")
	}
}

func TestPhaseOneHistoryAcceptsDisplaySafeAutomationAttempts(t *testing.T) {
	cueID := "cq_" + strings.Repeat("A", 26)
	historyID := "hi_a" + strings.Repeat("0", 24) + "1"
	doer := &phaseOneScriptedDoer{handle: func(request *http.Request, _ int) (*http.Response, error) {
		return phaseOneJSONResponse(request, http.StatusOK, fmt.Sprintf(`{"contract":"p1-history-presence-telegram-v1","items":[{"history_item_id":%q,"item_kind":"automation_attempt","direction":"sent","occurred_at":"2026-07-17T00:00:00Z","media":{"title":"Bell"},"status":"denied","reason_code":"automation_disabled","target_counts":{"played":0,"other":0},"automation":{"trigger_kind":"schedule","schedule_id":"sch_%s","schedule_label":"Morning","schedule_revision":2,"cue_id":%q,"cue_label":"Bell","cue_revision":3,"audience_kind":"own_barycenter","resolved_target_count":0,"outcome":"denied","reason_code":"automation_disabled"},"actions":["disable_schedule","emergency_disable_automation"]}]}`,
			historyID, strings.Repeat("B", 26), cueID)), nil
	}}
	client, err := NewPhaseOneAppClient(phaseOneTestBundle(), doer)
	if err != nil {
		t.Fatal(err)
	}
	page, err := client.History(context.Background(), 20, "")
	if err != nil || len(page.Items) != 1 || page.Items[0].ID != historyID || page.Items[0].Automation == nil ||
		page.Items[0].Automation.CueID != cueID || !phaseOneActionAllowed(page.Items[0].Actions, "disable_schedule") {
		t.Fatalf("page=%+v err=%v", page, err)
	}
}
