package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/store"
)

func createHistoryHTTPMedia(t *testing.T, harness onboardingHarness, owner store.OnboardingCredentials, createdAt int64) store.MediaItem {
	t.Helper()
	item, err := harness.store.CreateMediaItem(store.CreateMediaItemParams{
		OwnerOrbitID: owner.OrbitID,
		ActorID:      owner.ActorID,
		Kind:         store.MediaKindAudioClip,
		Source:       store.MediaSourceApp,
		Title:        "HTTP history fixture",
		CreatedAt:    createdAt,
		ExpiresAt:    createdAt + int64((30*24*time.Hour)/time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func historyActionRequest(handler http.Handler, path, body, bearer, key string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:34567"
	req.Header.Set("Authorization", "Bearer "+bearer)
	if key != "" {
		req.Header.Set("Idempotency-Key", key)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func TestHistoryHTTPMediaPaginationValidationAndRedaction(t *testing.T) {
	harness := newOnboardingHarness(t)
	owner, err := harness.store.CreateSelfServiceOrbit("HTTP history owner")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 14, 15, 0, 0, 0, time.UTC)
	harness.api.transmissionNow = func() time.Time { return now }
	_ = createHistoryHTTPMedia(t, harness, owner, now.Add(-2*time.Second).UnixMilli())
	_ = createHistoryHTTPMedia(t, harness, owner, now.Add(-time.Second).UnixMilli())

	page := apiRequest(harness.mux, http.MethodGet, "/v1/history?limit=1", "", owner.ControlToken)
	if page.Code != http.StatusOK || page.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("history=%d headers=%v body=%s", page.Code, page.Header(), page.Body.String())
	}
	body := decodeObject(t, page)
	if body["contract"] != presencePolicyContract {
		t.Fatalf("contract=%v", body["contract"])
	}
	items := body["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items=%v", items)
	}
	item := items[0].(map[string]any)
	media := item["media"].(map[string]any)
	if item["item_kind"] != "media" || item["direction"] != "sent" ||
		item["status"] != "processing" || item["history_item_id"] == "" {
		t.Fatalf("history item=%v", item)
	}
	if _, exists := media["media_id"]; exists {
		t.Fatalf("history exposed raw media id: %v", media)
	}
	if _, exists := media["duration_ms"]; exists {
		t.Fatalf("processing media exposed duration: %v", media)
	}
	if _, exists := media["content_available"]; exists {
		t.Fatalf("processing media exposed content availability: %v", media)
	}
	actions := item["actions"].([]any)
	if len(actions) != 1 || actions[0] != "delete" {
		t.Fatalf("processing actions=%v", actions)
	}
	cursor, ok := body["next_cursor"].(string)
	if !ok || !strings.HasPrefix(cursor, "hc_") {
		t.Fatalf("next_cursor=%v", body["next_cursor"])
	}
	for _, forbidden := range []string{
		"actor_id", "control_token", "node_token", "credential", "binding_paired_at",
		"media_id", "transmission_id", "orbit_id", "slot", "media_url",
		"storage_key", "hostname", "process_name", "microphone",
	} {
		if strings.Contains(page.Body.String(), forbidden) {
			t.Fatalf("history leaked %q: %s", forbidden, page.Body.String())
		}
	}

	invalidCursor := apiRequest(harness.mux, http.MethodGet,
		"/v1/history?view=sent&limit=1&cursor="+cursor, "", owner.ControlToken)
	assertTransmissionError(t, invalidCursor, http.StatusGone, errorCursorExpired)
	for _, path := range []string{
		"/v1/history?unknown=1",
		"/v1/history?limit=1&limit=1",
		"/v1/history?limit=01",
		"/v1/history?view=queue",
		"/v1/history?view=",
		"/v1/history?limit=",
		"/v1/history?cursor=",
	} {
		response := apiRequest(harness.mux, http.MethodGet, path, "", owner.ControlToken)
		assertTransmissionError(t, response, http.StatusBadRequest, errorInvalidRequest)
	}
	notFound := apiRequest(harness.mux, http.MethodGet, "/v1/history/hi_bad", "", owner.ControlToken)
	assertTransmissionError(t, notFound, http.StatusNotFound, errorHistoryNotFound)
}

func TestHistoryHTTPTransmissionListAndDetail(t *testing.T) {
	harness := newOnboardingHarness(t)
	owner, err := harness.store.CreateSelfServiceOrbit("HTTP transmission history")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 14, 16, 0, 0, 0, time.UTC)
	current := now.UnixMilli()
	harness.api.transmissionNow = func() time.Time { return time.UnixMilli(current) }
	media := readyTransmissionHTTPMedia(t, harness, owner, current-10, 1400)
	harness.api.transmissionPresence = func() map[transmissionPresenceKey]transmissionPresenceState {
		return map[transmissionPresenceKey]transmissionPresenceState{
			{OrbitID: owner.OrbitID, Slot: owner.Slot}: transmissionPresenceFor(owner, current),
		}
	}
	created := transmissionAPIRequest(harness.mux, http.MethodPost, "/v1/transmissions",
		transmissionBody(media.ID, "own_barycenter", "overlay", "file"), owner.ControlToken,
		"history-http-transmission-key-001")
	if created.Code != http.StatusCreated {
		t.Fatalf("create=%d %s", created.Code, created.Body.String())
	}
	listed := apiRequest(harness.mux, http.MethodGet, "/v1/history?view=sent", "", owner.ControlToken)
	if listed.Code != http.StatusOK {
		t.Fatalf("list=%d %s", listed.Code, listed.Body.String())
	}
	items := decodeObject(t, listed)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("items=%v", items)
	}
	item := items[0].(map[string]any)
	if item["item_kind"] != "transmission" || item["status"] != "ready" ||
		item["requested_delivery"] != "overlay" || item["effective_delivery"] != "overlay" {
		t.Fatalf("transmission history=%v", item)
	}
	sender := item["sender"].(map[string]any)
	if !strings.HasPrefix(sender["actor_ref"].(string), "ar_") ||
		!strings.HasPrefix(sender["source_orbit_ref"].(string), "or_") {
		t.Fatalf("sender=%v", sender)
	}
	if _, exists := sender["source_orbit_id"]; exists {
		t.Fatalf("sender exposed raw orbit id: %v", sender)
	}
	counts := item["target_counts"].(map[string]any)
	if counts["played"] != float64(0) || counts["other"] != float64(1) {
		t.Fatalf("compact counts=%v", counts)
	}
	wantActions := []string{"cancel", "delete", "replay"}
	gotActions := item["actions"].([]any)
	if len(gotActions) != len(wantActions) {
		t.Fatalf("actions=%v", gotActions)
	}
	for index, want := range wantActions {
		if gotActions[index] != want {
			t.Fatalf("actions=%v", gotActions)
		}
	}

	historyID := item["history_item_id"].(string)
	detail := apiRequest(harness.mux, http.MethodGet, "/v1/history/"+historyID, "", owner.ControlToken)
	if detail.Code != http.StatusOK {
		t.Fatalf("detail=%d %s", detail.Code, detail.Body.String())
	}
	detailBody := decodeObject(t, detail)
	if detailBody["accepted_at"] == "" || detailBody["expires_at"] == "" {
		t.Fatalf("detail=%v", detailBody)
	}
	if _, exists := detailBody["transmission_id"]; exists {
		t.Fatalf("detail exposed raw transmission id: %v", detailBody)
	}
	if _, exists := detailBody["targets"]; exists {
		t.Fatalf("detail exposed raw target rows instead of receipt page: %v", detailBody)
	}
	detailSender := detailBody["sender"].(map[string]any)
	if detailSender["actor_ref"] != sender["actor_ref"] || detailSender["source_orbit_ref"] != sender["source_orbit_ref"] {
		t.Fatalf("viewer-scoped refs were not reused: list=%v detail=%v", sender, detailSender)
	}
	fullCounts := detailBody["target_counts"].(map[string]any)
	for _, name := range []string{
		"accepted", "preparing", "ready", "scheduled", "playing", "cancelling", "played",
		"missed_offline", "missed_dnd", "missed_not_ready", "blocked", "failed", "cancelled", "expired",
	} {
		if _, exists := fullCounts[name]; !exists {
			t.Fatalf("detail counts missing %q: %v", name, fullCounts)
		}
	}
	queryOnDetail := apiRequest(harness.mux, http.MethodGet, "/v1/history/"+historyID+"?x=1", "", owner.ControlToken)
	assertTransmissionError(t, queryOnDetail, http.StatusBadRequest, errorInvalidRequest)
	for _, forbidden := range []string{"actor_id", "orbit_id", "slot", "media_id", "transmission_id", "binding_paired_at", "generation", "connection", "media_url", "storage_key"} {
		if strings.Contains(detail.Body.String(), forbidden) {
			t.Fatalf("detail leaked %q: %s", forbidden, detail.Body.String())
		}
	}

	if _, err := harness.store.DeleteMediaItem(media.ID, media.Revision, current+1); err != nil {
		t.Fatal(err)
	}
	current++
	contentGone := apiRequest(harness.mux, http.MethodGet, "/v1/history/"+historyID, "", owner.ControlToken)
	if contentGone.Code != http.StatusOK {
		t.Fatalf("deleted-content detail=%d %s", contentGone.Code, contentGone.Body.String())
	}
	deletedBody := decodeObject(t, contentGone)
	if deletedBody["media"].(map[string]any)["content_available"] != false {
		t.Fatalf("deleted content availability=%v", deletedBody["media"])
	}
	for _, action := range deletedBody["actions"].([]any) {
		if action == "delete" || action == "replay" {
			t.Fatalf("deleted content retained action %q: %v", action, deletedBody["actions"])
		}
	}
}

func TestHistoryHTTPReplayDeleteUsesFreshResolutionAndStrictIdempotency(t *testing.T) {
	harness := newOnboardingHarness(t)
	owner, err := harness.store.CreateSelfServiceOrbit("HTTP history actions")
	if err != nil {
		t.Fatal(err)
	}
	current := time.Date(2026, 7, 14, 18, 0, 0, 0, time.UTC).UnixMilli()
	harness.api.transmissionNow = func() time.Time { return time.UnixMilli(current) }
	media := readyTransmissionHTTPMedia(t, harness, owner, current-10, 1400)
	harness.api.transmissionPresence = func() map[transmissionPresenceKey]transmissionPresenceState {
		return map[transmissionPresenceKey]transmissionPresenceState{
			{OrbitID: owner.OrbitID, Slot: owner.Slot}: transmissionPresenceFor(owner, current),
		}
	}
	created := transmissionAPIRequest(harness.mux, http.MethodPost, "/v1/transmissions",
		transmissionBody(media.ID, "own_barycenter", "after_current", "file"), owner.ControlToken,
		"history-action-origin-key-001")
	if created.Code != http.StatusCreated {
		t.Fatalf("create=%d %s", created.Code, created.Body.String())
	}
	original := decodeObject(t, created)
	listed := apiRequest(harness.mux, http.MethodGet, "/v1/history?view=sent", "", owner.ControlToken)
	historyID := decodeObject(t, listed)["items"].([]any)[0].(map[string]any)["history_item_id"].(string)

	invite, err := harness.store.IssueDeviceInvite(owner.ActorID, owner.ControlToken, "companion")
	if err != nil {
		t.Fatal(err)
	}
	companion, err := harness.store.ConsumeDeviceInvite(invite.Code)
	if err != nil {
		t.Fatal(err)
	}
	current++
	harness.api.transmissionPresence = func() map[transmissionPresenceKey]transmissionPresenceState {
		return map[transmissionPresenceKey]transmissionPresenceState{
			{OrbitID: owner.OrbitID, Slot: owner.Slot}:         transmissionPresenceFor(owner, current),
			{OrbitID: companion.OrbitID, Slot: companion.Slot}: transmissionPresenceFor(companion, current),
		}
	}
	path := "/v1/history/" + historyID + "/actions/replay"
	body := `{"audience":{"kind":"own_barycenter"},"delivery":"after_current"}`
	replay := historyActionRequest(harness.mux, path, body, owner.ControlToken,
		"history-action-replay-key-001")
	if replay.Code != http.StatusCreated {
		t.Fatalf("replay=%d %s", replay.Code, replay.Body.String())
	}
	replayed := decodeObject(t, replay)
	if replayed["transmission_id"] == original["transmission_id"] ||
		replayed["accepted_at"] == original["accepted_at"] ||
		replayed["audience"].(map[string]any)["target_count"] != float64(2) {
		t.Fatalf("replay did not resolve fresh state: original=%v replay=%v", original, replayed)
	}
	repeated := historyActionRequest(harness.mux, path, body, owner.ControlToken,
		"history-action-replay-key-001")
	if repeated.Code != http.StatusOK || decodeObject(t, repeated)["transmission_id"] != replayed["transmission_id"] {
		t.Fatalf("repeated=%d %s", repeated.Code, repeated.Body.String())
	}
	conflict := historyActionRequest(harness.mux, path,
		`{"audience":{"kind":"this_pulsar"},"delivery":"after_current"}`,
		owner.ControlToken, "history-action-replay-key-001")
	assertTransmissionError(t, conflict, http.StatusConflict, errorTransmissionIdempotency)

	deleteResponse := historyActionRequest(harness.mux,
		"/v1/history/"+historyID+"/actions/delete", `{}`, owner.ControlToken, "")
	if deleteResponse.Code != http.StatusOK || decodeObject(t, deleteResponse)["deleted"] != true {
		t.Fatalf("delete=%d %s", deleteResponse.Code, deleteResponse.Body.String())
	}
	repeatedDelete := historyActionRequest(harness.mux,
		"/v1/history/"+historyID+"/actions/delete", `{}`, owner.ControlToken, "")
	if repeatedDelete.Code != http.StatusOK || decodeObject(t, repeatedDelete)["deleted"] != true {
		t.Fatalf("repeated delete=%d %s", repeatedDelete.Code, repeatedDelete.Body.String())
	}
	current++
	replayRetryAfterDelete := historyActionRequest(harness.mux, path, body, owner.ControlToken,
		"history-action-replay-key-001")
	if replayRetryAfterDelete.Code != http.StatusOK ||
		decodeObject(t, replayRetryAfterDelete)["transmission_id"] != replayed["transmission_id"] {
		t.Fatalf("replay retry after delete=%d %s", replayRetryAfterDelete.Code, replayRetryAfterDelete.Body.String())
	}
	afterDelete := historyActionRequest(harness.mux, path, body, owner.ControlToken,
		"history-action-replay-key-002")
	assertTransmissionError(t, afterDelete, http.StatusConflict, errorHistoryActionUnavailable)

	unknown := historyActionRequest(harness.mux,
		"/v1/history/"+historyID+"/actions/archive", `{}`, owner.ControlToken, "")
	assertTransmissionError(t, unknown, http.StatusNotFound, errorHistoryNotFound)
	badBody := historyActionRequest(harness.mux,
		"/v1/history/"+historyID+"/actions/delete", `{"media_id":"`+media.ID+`"}`,
		owner.ControlToken, "")
	assertTransmissionError(t, badBody, http.StatusBadRequest, errorInvalidRequest)
}

func TestHistoryHTTPReportAndBlockShareCanonicalOwnerServices(t *testing.T) {
	fixture := newModerationHTTPFixture(t)
	listed := apiRequest(fixture.harness.mux, http.MethodGet,
		"/v1/history?view=received", "", fixture.reporter.ControlToken)
	if listed.Code != http.StatusOK {
		t.Fatalf("history=%d %s", listed.Code, listed.Body.String())
	}
	items := decodeObject(t, listed)["items"].([]any)
	if len(items) != 1 {
		t.Fatalf("history items=%v", items)
	}
	item := items[0].(map[string]any)
	historyID := item["history_item_id"].(string)
	actions := item["actions"].([]any)
	if !containsJSONValue(actions, "report") || !containsJSONValue(actions, "block_actor") {
		t.Fatalf("actions=%v", actions)
	}

	reportPath := "/v1/history/" + historyID + "/actions/report"
	report := historyActionRequest(fixture.harness.mux, reportPath,
		`{"reason":"harassment","details":"history-visible evidence"}`,
		fixture.reporter.ControlToken, "")
	if report.Code != http.StatusCreated {
		t.Fatalf("report=%d %s", report.Code, report.Body.String())
	}
	reportBody := decodeObject(t, report)
	if reportBody["history_item_id"] != historyID || reportBody["reused"] != false {
		t.Fatalf("report body=%v", reportBody)
	}
	if _, exists := reportBody["media_id"]; exists {
		t.Fatalf("history report exposed raw media id: %v", reportBody)
	}
	repeatedReport := historyActionRequest(fixture.harness.mux, reportPath,
		`{"reason":"spam","details":"ignored on exact duplicate"}`,
		fixture.reporter.ControlToken, "")
	if repeatedReport.Code != http.StatusOK || decodeObject(t, repeatedReport)["id"] != reportBody["id"] {
		t.Fatalf("repeated report=%d %s", repeatedReport.Code, repeatedReport.Body.String())
	}

	blockPath := "/v1/history/" + historyID + "/actions/block_actor"
	block := historyActionRequest(fixture.harness.mux, blockPath, `{}`,
		fixture.reporter.ControlToken, "history-action-block-key-001")
	if block.Code != http.StatusCreated {
		t.Fatalf("block=%d %s", block.Code, block.Body.String())
	}
	blockBody := decodeObject(t, block)
	if blockBody["scope"] != "actor" || blockBody["reused"] != false {
		t.Fatalf("block body=%v", blockBody)
	}
	repeatedBlock := historyActionRequest(fixture.harness.mux, blockPath, `{}`,
		fixture.reporter.ControlToken, "history-action-block-key-001")
	if repeatedBlock.Code != http.StatusOK || decodeObject(t, repeatedBlock)["block_id"] != blockBody["block_id"] {
		t.Fatalf("repeated block=%d %s", repeatedBlock.Code, repeatedBlock.Body.String())
	}
	detail := apiRequest(fixture.harness.mux, http.MethodGet,
		"/v1/history/"+historyID, "", fixture.reporter.ControlToken)
	detailActions := decodeObject(t, detail)["actions"].([]any)
	if containsJSONValue(detailActions, "block_actor") || !containsJSONValue(detailActions, "unblock") {
		t.Fatalf("post-block actions=%v", detailActions)
	}
}

func containsJSONValue(values []any, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func TestHistoryClientStateMappingIsClosedAndDeterministic(t *testing.T) {
	cases := []struct {
		name   string
		media  store.MediaItemStatus
		counts map[store.TransmissionTargetStatus]int
		total  int
		reason store.TransmissionReason
		status string
		code   string
	}{
		{"processing", store.MediaStatusProcessing, map[store.TransmissionTargetStatus]int{store.TransmissionTargetAccepted: 1}, 1, "", "processing", ""},
		{"ready", store.MediaStatusReady, map[store.TransmissionTargetStatus]int{store.TransmissionTargetReady: 1}, 1, "", "ready", ""},
		{"playing", store.MediaStatusReady, map[store.TransmissionTargetStatus]int{store.TransmissionTargetPlaying: 1}, 1, "", "playing", ""},
		{"cancelling stays playing", store.MediaStatusReady, map[store.TransmissionTargetStatus]int{store.TransmissionTargetCancelling: 1}, 1, "", "playing", ""},
		{"played", store.MediaStatusReady, map[store.TransmissionTargetStatus]int{store.TransmissionTargetPlayed: 2}, 2, "", "played", "completed"},
		{"partial", store.MediaStatusReady, map[store.TransmissionTargetStatus]int{store.TransmissionTargetPlayed: 1, store.TransmissionTargetFailed: 1}, 2, store.TransmissionReasonPartialDelivery, "partial", "partial_delivery"},
		{"expired", store.MediaStatusReady, map[store.TransmissionTargetStatus]int{store.TransmissionTargetExpired: 2}, 2, store.TransmissionReasonDeliveryExpired, "expired", "expired"},
		{"error", store.MediaStatusReady, map[store.TransmissionTargetStatus]int{store.TransmissionTargetFailed: 2}, 2, store.TransmissionReasonAllTargetsFailed, "error", "all_targets_failed"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			transmission := store.Transmission{ReasonCode: tc.reason}
			item := store.HistoryQueryItem{Media: store.MediaItem{Status: tc.media}, Transmission: &transmission,
				TargetStatusCounts: tc.counts, TargetCount: tc.total}
			status, code := historyTransmissionState(item)
			if status != tc.status || code != tc.code {
				t.Fatalf("state=(%q,%q), want=(%q,%q)", status, code, tc.status, tc.code)
			}
		})
	}
	mediaCases := []struct {
		item   store.MediaItem
		status string
		code   string
	}{
		{store.MediaItem{Status: store.MediaStatusProcessing}, "processing", ""},
		{store.MediaItem{Status: store.MediaStatusReady}, "ready", ""},
		{store.MediaItem{Status: store.MediaStatusFailed, FailureCode: "decode_failed"}, "error", "decode_failed"},
	}
	for _, tc := range mediaCases {
		status, code := historyMediaState(tc.item)
		if status != tc.status || code != tc.code {
			t.Fatalf("media state=(%q,%q), want=(%q,%q)", status, code, tc.status, tc.code)
		}
	}
}

func TestHistoryBlockedReceiptReasonRequiresBlockOwnership(t *testing.T) {
	item := store.HistoryQueryItem{Targets: []store.TransmissionTarget{{
		OrbitID: 7, Slot: "a", Status: store.TransmissionTargetBlocked,
		ReasonCode: store.TransmissionReasonActorBlocked,
	}}}
	redacted := historyTargetResponses(item)
	if len(redacted) != 1 || redacted[0].Status != "blocked" || redacted[0].ReasonCode != "" {
		t.Fatalf("redacted receipt=%+v", redacted)
	}
	item.RevealBlockedReason = true
	owned := historyTargetResponses(item)
	if len(owned) != 1 || owned[0].ReasonCode != "actor_blocked" {
		t.Fatalf("owned receipt=%+v", owned)
	}
}
