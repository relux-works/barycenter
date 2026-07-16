package main

import (
	"net/http"
	"strings"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/store"
)

func createInboxHTTPMiss(
	t *testing.T,
	harness onboardingHarness,
	media store.MediaItem,
	source store.OnboardingCredentials,
	target store.OnboardingCredentials,
	acceptedAt int64,
) store.TransmissionCreation {
	t.Helper()
	created, err := harness.store.CreateTransmission(store.CreateTransmissionParams{
		MediaID: media.ID, SourceOrbitID: source.OrbitID,
		SourceActorID: source.ActorID, SourceSlot: source.Slot,
		PlaybackDomainKind: store.PlaybackDomainOrbit, PlaybackDomainID: source.OrbitID,
		AudienceKind: store.TransmissionAudienceExplicit,
		OriginKind:   store.TransmissionOriginFile, IncludeOrigin: false,
		RequestedDelivery: store.TransmissionDeliveryAfterCurrent,
		EffectiveDelivery: store.TransmissionDeliveryAfterCurrent,
		AcceptedAt:        acceptedAt,
		Targets: []store.CreateTransmissionTarget{{
			OrbitID: target.OrbitID, ActorID: target.ActorID, Slot: target.Slot,
			OnlineAtAcceptance: false, MediaClipCapable: true, OverlayCapable: true,
			InterruptCapable: true, InterruptResumeReady: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	targetRow := created.Targets[0]
	if _, err := harness.store.TransitionTransmissionTarget(
		store.TransitionTransmissionTargetParams{
			TransmissionID: created.Transmission.ID, OrbitID: targetRow.OrbitID,
			ActorID: targetRow.ActorID, Slot: targetRow.Slot,
			ExpectedRevision: targetRow.Revision, Generation: targetRow.Generation,
			Status:     store.TransmissionTargetMissedOffline,
			ReasonCode: store.TransmissionReasonOfflineBeforeStart,
			OccurredAt: acceptedAt + 1,
		}); err != nil {
		t.Fatal(err)
	}
	return created
}

func TestInboxHTTPPaginationRedactionDismissReplayAndReceipts(t *testing.T) {
	harness := newOnboardingHarness(t)
	source, err := harness.store.CreateSelfServiceOrbit("Inbox HTTP source")
	if err != nil {
		t.Fatal(err)
	}
	target, err := harness.store.CreateSelfServiceOrbit("Inbox HTTP target")
	if err != nil {
		t.Fatal(err)
	}
	outsider, err := harness.store.CreateSelfServiceOrbit("Inbox HTTP outsider")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 16, 4, 0, 0, 0, time.UTC)
	current := now.UnixMilli()
	harness.api.transmissionNow = func() time.Time { return time.UnixMilli(current) }
	media := readyTransmissionHTTPMedia(t, harness, source, current-100, 1200)
	firstTransmission := createInboxHTTPMiss(t, harness, media, source, target, current-80)
	_ = createInboxHTTPMiss(t, harness, media, source, target, current-60)
	accepted := []string{}
	harness.api.transmissionAccepted = func(id string) { accepted = append(accepted, id) }

	page := apiRequest(harness.mux, http.MethodGet, "/v1/inbox?limit=1", "", target.ControlToken)
	if page.Code != http.StatusOK || page.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("inbox status=%d headers=%v body=%s", page.Code, page.Header(), page.Body.String())
	}
	body := decodeObject(t, page)
	items := body["items"].([]any)
	if body["contract"] != targetsInboxContract || len(items) != 1 || body["next_cursor"] == "" {
		t.Fatalf("inbox body=%v", body)
	}
	item := items[0].(map[string]any)
	inboxID := item["id"].(string)
	historyItemID := item["history_item_id"].(string)
	if !strings.HasPrefix(inboxID, "ib_") || !strings.HasPrefix(historyItemID, "hi_") ||
		len(accepted) != 0 {
		t.Fatalf("safe handles inbox=%q history=%q accepted=%v", inboxID, historyItemID, accepted)
	}
	for _, forbidden := range []string{
		`"media_id"`, `"transmission_id"`, `"actor_id"`, `"orbit_id"`,
		`"slot"`, "binding_paired_at", source.ControlToken, target.NodeToken,
	} {
		if strings.Contains(page.Body.String(), forbidden) {
			t.Fatalf("inbox leaked %q: %s", forbidden, page.Body.String())
		}
	}

	missing := apiRequest(harness.mux, http.MethodGet, "/v1/inbox/"+inboxID, "", outsider.ControlToken)
	unknown := apiRequest(harness.mux, http.MethodGet,
		"/v1/inbox/ib_00000000000000000000000000", "", outsider.ControlToken)
	if missing.Code != http.StatusNotFound || missing.Body.String() != unknown.Body.String() {
		t.Fatalf("non-target inference missing=%d %q unknown=%d %q",
			missing.Code, missing.Body.String(), unknown.Code, unknown.Body.String())
	}

	dismissed := apiRequest(harness.mux, http.MethodDelete, "/v1/inbox/"+inboxID, "", target.ControlToken)
	if dismissed.Code != http.StatusOK ||
		decodeObject(t, dismissed)["item"].(map[string]any)["availability"] != "dismissed" {
		t.Fatalf("dismiss status=%d body=%s", dismissed.Code, dismissed.Body.String())
	}
	dismissRetry := apiRequest(harness.mux, http.MethodDelete, "/v1/inbox/"+inboxID, "", target.ControlToken)
	if dismissRetry.Code != http.StatusOK {
		t.Fatalf("dismiss retry=%d body=%s", dismissRetry.Code, dismissRetry.Body.String())
	}

	secondPage := apiRequest(harness.mux, http.MethodGet,
		"/v1/inbox?limit=1&cursor="+body["next_cursor"].(string), "", target.ControlToken)
	if secondPage.Code != http.StatusOK {
		t.Fatalf("second page=%d body=%s", secondPage.Code, secondPage.Body.String())
	}
	replayItem := decodeObject(t, secondPage)["items"].([]any)[0].(map[string]any)
	replayInboxID := replayItem["id"].(string)
	replayPath := "/v1/inbox/" + replayInboxID + "/replays"
	policyRequired := transmissionAPIRequest(harness.mux, http.MethodPost, replayPath,
		`{"delivery":"after_current"}`, target.ControlToken, "inbox-replay-key-policy")
	assertTransmissionError(t, policyRequired, http.StatusPreconditionRequired,
		errorContentPolicyAcceptance)
	if len(accepted) != 0 {
		t.Fatalf("policy failure scheduled playback: %v", accepted)
	}
	acceptContentPolicyStore(t, harness, target, current-20)
	replay := transmissionAPIRequest(harness.mux, http.MethodPost, replayPath,
		`{"delivery":"after_current"}`, target.ControlToken, "inbox-replay-key-0001")
	if replay.Code != http.StatusCreated {
		t.Fatalf("replay=%d body=%s", replay.Code, replay.Body.String())
	}
	replayBody := decodeObject(t, replay)
	if !strings.HasPrefix(replayBody["replay_request_id"].(string), "ir_") ||
		!strings.HasPrefix(replayBody["history_item_id"].(string), "hi_") || len(accepted) != 1 {
		t.Fatalf("replay body=%v accepted=%v", replayBody, accepted)
	}
	for _, forbidden := range []string{`"media_id"`, `"transmission_id"`, `"orbit_id"`, `"slot"`} {
		if strings.Contains(replay.Body.String(), forbidden) {
			t.Fatalf("replay leaked %q: %s", forbidden, replay.Body.String())
		}
	}
	reused := transmissionAPIRequest(harness.mux, http.MethodPost, replayPath,
		`{"delivery":"after_current"}`, target.ControlToken, "inbox-replay-key-0001")
	if reused.Code != http.StatusOK || decodeObject(t, reused)["reused"] != true {
		t.Fatalf("reused=%d body=%s", reused.Code, reused.Body.String())
	}
	if len(accepted) != 1 {
		t.Fatalf("idempotent retry rescheduled playback: %v", accepted)
	}

	receipts := apiRequest(harness.mux, http.MethodGet,
		"/v1/history/"+historyIDForTransmission(firstTransmission.Transmission.ID)+"/receipts?limit=1",
		"", source.ControlToken)
	if receipts.Code != http.StatusOK {
		t.Fatalf("receipts=%d body=%s", receipts.Code, receipts.Body.String())
	}
	for _, forbidden := range []string{`"actor_id"`, `"orbit_id"`, `"slot"`, `"transmission_id"`} {
		if strings.Contains(receipts.Body.String(), forbidden) {
			t.Fatalf("receipt leaked %q: %s", forbidden, receipts.Body.String())
		}
	}
	receiptItems := decodeObject(t, receipts)["items"].([]any)
	if len(receiptItems) != 1 || receiptItems[0].(map[string]any)["target_label"] == "" {
		t.Fatalf("receipt items=%v", receiptItems)
	}
}

func historyIDForTransmission(transmissionID string) string {
	return "hi_" + strings.TrimPrefix(transmissionID, "tr_")
}
