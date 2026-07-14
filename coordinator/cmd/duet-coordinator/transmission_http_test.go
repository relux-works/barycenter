package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/store"
)

func transmissionAPIRequest(
	handler http.Handler,
	method, path, body, bearer, idempotencyKey string,
) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.RemoteAddr = "127.0.0.1:34567"
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func transmissionBody(
	mediaID, audienceKind, delivery, originKind string,
) string {
	return fmt.Sprintf(
		`{"media_id":%q,"audience":{"kind":%q},"delivery":%q,"origin_kind":%q}`,
		mediaID, audienceKind, delivery, originKind,
	)
}

func transmissionPresenceFor(
	credentials store.OnboardingCredentials,
	now int64,
) transmissionPresenceState {
	return transmissionPresenceState{
		Connected: true, LastSeenAt: now,
		CredentialTokenHash: transmissionDigest(credentials.NodeToken),
		MediaClipCapable:    true, OverlayCapable: true,
		InterruptCapable: true, MainActive: true,
		InterruptResumeReady: true,
	}
}

func installTransmissionCompanion(
	t *testing.T,
	harness onboardingHarness,
	owner store.OnboardingCredentials,
) store.OnboardingCredentials {
	t.Helper()
	invite, err := harness.store.IssueDeviceInvite(owner.ActorID, owner.ControlToken, "companion")
	if err != nil {
		t.Fatal(err)
	}
	companion, err := harness.store.ConsumeDeviceInvite(invite.Code)
	if err != nil {
		t.Fatal(err)
	}
	return companion
}

func readyTransmissionHTTPMedia(
	t *testing.T,
	harness onboardingHarness,
	credentials store.OnboardingCredentials,
	now, durationMS int64,
) store.MediaItem {
	return readyTransmissionHTTPMediaKind(
		t, harness, credentials, now, durationMS, store.MediaKindAudioClip,
	)
}

func readyTransmissionHTTPMediaKind(
	t *testing.T,
	harness onboardingHarness,
	credentials store.OnboardingCredentials,
	now, durationMS int64,
	kind store.MediaKind,
) store.MediaItem {
	t.Helper()
	item, err := harness.store.CreateMediaItem(store.CreateMediaItemParams{
		OwnerOrbitID: credentials.OrbitID, ActorID: credentials.ActorID,
		Kind: kind, Source: store.MediaSourceApp,
		Title: "transmission-http-fixture", CreatedAt: now,
		ExpiresAt: now + int64((7*24*time.Hour)/time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := harness.store.StageMediaPublication(item.ID, item.Revision, now+1)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := harness.store.CompleteMediaPublication(
		operation.ID, operation.Revision,
		store.MediaPublication{
			MIME: "audio/wav", Codec: "pcm_s16le", DurationMS: durationMS,
			SizeBytes: 176444, SHA256: strings.Repeat("e", 64),
			LoudnessJSON: `{"input_i":"-20.0","input_tp":"-3.0","output_i":"-14.0","output_tp":"-1.5"}`,
		},
		now+2,
	)
	if err != nil {
		t.Fatal(err)
	}
	return ready
}

func assertTransmissionError(
	t *testing.T,
	recorder *httptest.ResponseRecorder,
	status int,
	code string,
) map[string]any {
	t.Helper()
	if recorder.Code != status {
		t.Fatalf("status=%d want=%d body=%s", recorder.Code, status, recorder.Body.String())
	}
	body := decodeObject(t, recorder)
	errorObject, ok := body["error"].(map[string]any)
	if !ok || errorObject["code"] != code {
		t.Fatalf("error body=%v want code=%q", body, code)
	}
	return errorObject
}

func TestTransmissionHTTPCreateReplayStatusAndCancel(t *testing.T) {
	harness := newOnboardingHarness(t)
	owner, err := harness.store.CreateSelfServiceOrbit("Transmission HTTP owner")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyTransmissionHTTPMedia(t, harness, owner, now, 1200)
	current := now + 3
	harness.api.transmissionNow = func() time.Time { return time.UnixMilli(current) }
	tokenCalls := 0
	harness.api.transmissionToken = func() (string, error) {
		tokenCalls++
		return "fc_" + strings.Repeat("f", 64), nil
	}
	harness.api.transmissionPresence = func() map[transmissionPresenceKey]transmissionPresenceState {
		return map[transmissionPresenceKey]transmissionPresenceState{
			{OrbitID: owner.OrbitID, Slot: owner.Slot}: transmissionPresenceFor(owner, current),
		}
	}
	body := transmissionBody(media.ID, "own_barycenter", "overlay", "file")
	key := "transmission-create-idempotency-0001"
	created := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions", body, owner.ControlToken, key,
	)
	if created.Code != http.StatusCreated ||
		created.Header().Get("Location") == "" ||
		created.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("create status=%d headers=%v body=%s", created.Code, created.Header(), created.Body.String())
	}
	if tokenCalls != 0 {
		t.Fatalf("non-interrupt create minted %d confirmation token(s)", tokenCalls)
	}
	createdBody := decodeObject(t, created)
	transmissionID, ok := createdBody["transmission_id"].(string)
	if !ok || !transmissionHTTPID.MatchString(transmissionID) ||
		createdBody["requested_delivery"] != "overlay" ||
		createdBody["effective_delivery"] != "overlay" ||
		createdBody["include_origin"] != true || createdBody["reused"] != false {
		t.Fatalf("created body=%v", createdBody)
	}
	counts := createdBody["target_counts"].(map[string]any)
	for _, name := range []string{
		"accepted", "preparing", "ready", "scheduled", "playing", "cancelling",
		"played", "missed_offline", "missed_dnd", "missed_not_ready", "blocked",
		"failed", "cancelled", "expired",
	} {
		if _, exists := counts[name]; !exists {
			t.Fatalf("target_counts missing %q: %v", name, counts)
		}
	}
	acceptedAt := createdBody["accepted_at"]
	current += 1000
	replayed := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions", body, owner.ControlToken, key,
	)
	if replayed.Code != http.StatusOK {
		t.Fatalf("replay status=%d body=%s", replayed.Code, replayed.Body.String())
	}
	replayedBody := decodeObject(t, replayed)
	if replayedBody["transmission_id"] != transmissionID || replayedBody["reused"] != true ||
		replayedBody["accepted_at"] != acceptedAt {
		t.Fatalf("replayed body=%v", replayedBody)
	}
	conflict := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions",
		transmissionBody(media.ID, "own_barycenter", "after_current", "file"),
		owner.ControlToken, key,
	)
	assertTransmissionError(t, conflict, http.StatusConflict, errorTransmissionIdempotency)

	status := transmissionAPIRequest(
		harness.mux, http.MethodGet, "/v1/transmissions/"+transmissionID,
		"", owner.NodeToken, "",
	)
	if status.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", status.Code, status.Body.String())
	}
	statusBody := decodeObject(t, status)
	targets, ok := statusBody["targets"].([]any)
	if !ok || len(targets) != 1 || statusBody["reused"] != nil ||
		statusBody["can_cancel"] != false {
		t.Fatalf("status body=%v", statusBody)
	}
	serializedTarget := fmt.Sprint(targets[0])
	for _, forbidden := range []string{"actor_id", "binding", "capabil", "token", "url", "path"} {
		if strings.Contains(serializedTarget, forbidden) {
			t.Fatalf("target leaked %q: %v", forbidden, targets[0])
		}
	}

	nodeCancel := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions/"+transmissionID+"/cancel",
		`{}`, owner.NodeToken, "",
	)
	assertTransmissionError(t, nodeCancel, http.StatusForbidden, errorInsufficientCapability)
	nodeMalformedCancel := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions/"+transmissionID+"/cancel",
		`{"probe":true}`, owner.NodeToken, "",
	)
	assertTransmissionError(t, nodeMalformedCancel, http.StatusForbidden, errorInsufficientCapability)
	cancelled := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions/"+transmissionID+"/cancel",
		`{}`, owner.ControlToken, "",
	)
	if cancelled.Code != http.StatusOK {
		t.Fatalf("cancel status=%d body=%s", cancelled.Code, cancelled.Body.String())
	}
	cancelledBody := decodeObject(t, cancelled)
	if cancelledBody["status"] != "cancelled" || cancelledBody["changed"] != true ||
		cancelledBody["reason_code"] != "sender_cancelled" {
		t.Fatalf("cancel body=%v", cancelledBody)
	}
	repeatedCancel := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions/"+transmissionID+"/cancel",
		`{}`, owner.ControlToken, "",
	)
	if repeatedCancel.Code != http.StatusOK || decodeObject(t, repeatedCancel)["changed"] != false {
		t.Fatalf("repeat cancel status=%d body=%s", repeatedCancel.Code, repeatedCancel.Body.String())
	}
}

func TestTransmissionHTTPStrictJSONAndStableErrors(t *testing.T) {
	harness := newOnboardingHarness(t)
	owner, err := harness.store.CreateSelfServiceOrbit("Transmission strict owner")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyTransmissionHTTPMedia(t, harness, owner, now, 1200)
	current := now + 3
	harness.api.transmissionNow = func() time.Time { return time.UnixMilli(current) }
	harness.api.transmissionPresence = func() map[transmissionPresenceKey]transmissionPresenceState {
		return map[transmissionPresenceKey]transmissionPresenceState{
			{OrbitID: owner.OrbitID, Slot: owner.Slot}: transmissionPresenceFor(owner, current),
		}
	}
	validAudience := `"audience":{"kind":"own_barycenter"},"delivery":"overlay","origin_kind":"file"`
	nodeCreate := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions",
		transmissionBody(media.ID, "own_barycenter", "overlay", "file"),
		owner.NodeToken, "strict-node-create-key-01",
	)
	assertTransmissionError(t, nodeCreate, http.StatusForbidden, errorInsufficientCapability)
	missingAuth := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions",
		transmissionBody(media.ID, "own_barycenter", "overlay", "file"),
		"", "strict-missing-auth-key-1",
	)
	assertTransmissionError(t, missingAuth, http.StatusUnauthorized, errorUnauthorized)
	malformed := []string{
		fmt.Sprintf(`{"media_id":%q,"media_id":%q,%s}`, media.ID, media.ID, validAudience),
		fmt.Sprintf(`{"media_id":%q,"audience":{"kind":"own_barycenter","kind":"current_air"},"delivery":"overlay","origin_kind":"file"}`, media.ID),
		fmt.Sprintf(`{"media_id":%q,%s,"include_origin":null}`, media.ID, validAudience),
		fmt.Sprintf(`{"media_id":%q,%s,"unknown":true}`, media.ID, validAudience),
		fmt.Sprintf(`{"media_id":%q,%s}{}`, media.ID, validAudience),
		fmt.Sprintf(`{"media_id":%q,"audience":{"kind":"own_barycenter","targets":null},"delivery":"overlay","origin_kind":"file"}`, media.ID),
		fmt.Sprintf(`{"media_id":%q,"audience":{"kind":"explicit","targets":[{"kind":"barycenter","orbit_id":%d,"slot":""}]},"delivery":"overlay","origin_kind":"file"}`, media.ID, owner.OrbitID),
		fmt.Sprintf(`{"media_id":%q,"audience":{"kind":"explicit","targets":[{"kind":"pulsar","orbit_id":%d}]},"delivery":"overlay","origin_kind":"file"}`, media.ID, owner.OrbitID),
	}
	for index, body := range malformed {
		recorder := transmissionAPIRequest(
			harness.mux, http.MethodPost, "/v1/transmissions", body,
			owner.ControlToken, fmt.Sprintf("strict-invalid-key-%04d", index),
		)
		assertTransmissionError(t, recorder, http.StatusBadRequest, errorInvalidRequest)
	}
	queue := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions",
		transmissionBody(media.ID, "own_barycenter", "queue", "file"),
		owner.ControlToken, "strict-unsupported-key-0001",
	)
	assertTransmissionError(t, queue, http.StatusUnprocessableEntity, errorDeliveryNotSupported)
	query := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions?force=1",
		transmissionBody(media.ID, "own_barycenter", "overlay", "file"),
		owner.ControlToken, "strict-query-key-0000001",
	)
	assertTransmissionError(t, query, http.StatusBadRequest, errorInvalidRequest)
	request := httptest.NewRequest(http.MethodPost, "/v1/transmissions",
		strings.NewReader(transmissionBody(media.ID, "own_barycenter", "overlay", "file")))
	request.RemoteAddr = "127.0.0.1:34567"
	request.Header.Set("Authorization", "Bearer "+owner.ControlToken)
	request.Header.Add("Idempotency-Key", "strict-duplicate-header-1")
	request.Header.Add("Idempotency-Key", "strict-duplicate-header-2")
	duplicateHeader := httptest.NewRecorder()
	harness.mux.ServeHTTP(duplicateHeader, request)
	assertTransmissionError(t, duplicateHeader, http.StatusBadRequest, errorInvalidRequest)

	longMedia := readyTransmissionHTTPMedia(t, harness, owner, now+10, 60001)
	current = now + 13
	duration := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions",
		transmissionBody(longMedia.ID, "own_barycenter", "overlay", "file"),
		owner.ControlToken, "strict-duration-key-00001",
	)
	errorObject := assertTransmissionError(
		t, duration, http.StatusUnprocessableEntity, errorOverlayDuration,
	)
	details := errorObject["details"].(map[string]any)
	alternatives := details["alternatives"].([]any)
	if len(alternatives) != 2 || alternatives[0].(map[string]any)["delivery"] != "interrupt" ||
		alternatives[1].(map[string]any)["delivery"] != "after_current" {
		t.Fatalf("duration alternatives=%v", alternatives)
	}

	processing, err := harness.store.CreateMediaItem(store.CreateMediaItemParams{
		OwnerOrbitID: owner.OrbitID, ActorID: owner.ActorID,
		Kind: store.MediaKindVoiceClip, Source: store.MediaSourceApp,
		Title: "not-ready", CreatedAt: now + 20,
		ExpiresAt: now + int64((7*24*time.Hour)/time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	current = now + 21
	notReady := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions",
		transmissionBody(processing.ID, "own_barycenter", "overlay", "file"),
		owner.ControlToken, "strict-media-not-ready-001",
	)
	assertTransmissionError(t, notReady, http.StatusConflict, errorMediaNotReady)

	foreign, err := harness.store.CreateSelfServiceOrbit("Foreign transmission media")
	if err != nil {
		t.Fatal(err)
	}
	foreignMedia := readyTransmissionHTTPMedia(t, harness, foreign, now+30, 1000)
	current = now + 33
	foreignRequest := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions",
		transmissionBody(foreignMedia.ID, "own_barycenter", "overlay", "file"),
		owner.ControlToken, "strict-foreign-media-key-1",
	)
	assertTransmissionError(t, foreignRequest, http.StatusNotFound, errorMediaNotFound)
	explicitOutsideBody := fmt.Sprintf(
		`{"media_id":%q,"audience":{"kind":"explicit","targets":[{"kind":"barycenter","orbit_id":%d}]},"delivery":"overlay","origin_kind":"file"}`,
		media.ID, foreign.OrbitID,
	)
	explicitOutside := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions", explicitOutsideBody,
		owner.ControlToken, "strict-outside-audience-01",
	)
	assertTransmissionError(t, explicitOutside, http.StatusNotFound, errorAudienceNotFound)

	track := readyTransmissionHTTPMediaKind(
		t, harness, owner, now+40, 1000, store.MediaKindAudioTrack,
	)
	current = now + 43
	trackRequest := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions",
		transmissionBody(track.ID, "own_barycenter", "overlay", "file"),
		owner.ControlToken, "strict-track-delivery-key1",
	)
	assertTransmissionError(t, trackRequest, http.StatusUnprocessableEntity, errorDeliveryKindMismatch)

	current = now + 44
	microphoneDefault := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions",
		transmissionBody(media.ID, "own_barycenter", "overlay", "microphone"),
		owner.ControlToken, "strict-microphone-default1",
	)
	assertTransmissionError(t, microphoneDefault, http.StatusUnprocessableEntity, errorAudienceEmpty)
	thisPulsarDefault := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions",
		transmissionBody(media.ID, "this_pulsar", "overlay", "microphone"),
		owner.ControlToken, "strict-this-pulsar-default",
	)
	assertTransmissionError(t, thisPulsarDefault, http.StatusBadRequest, errorInvalidRequest)
	playHereBody := fmt.Sprintf(
		`{"media_id":%q,"audience":{"kind":"this_pulsar"},"delivery":"overlay","origin_kind":"microphone","include_origin":true}`,
		media.ID,
	)
	playHere := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions", playHereBody,
		owner.ControlToken, "strict-this-pulsar-play-here",
	)
	if playHere.Code != http.StatusCreated || decodeObject(t, playHere)["include_origin"] != true {
		t.Fatalf("play-here status=%d body=%s", playHere.Code, playHere.Body.String())
	}
}

func TestTransmissionHTTPWholeDowngradeAndExplicitInterruptConfirmation(t *testing.T) {
	harness := newOnboardingHarness(t)
	owner, err := harness.store.CreateSelfServiceOrbit("Transmission capability owner")
	if err != nil {
		t.Fatal(err)
	}
	companion := installTransmissionCompanion(t, harness, owner)
	now := time.Now().UnixMilli()
	media := readyTransmissionHTTPMedia(t, harness, owner, now, 1500)
	current := now + 3
	harness.api.transmissionNow = func() time.Time { return time.UnixMilli(current) }
	harness.api.transmissionPresence = func() map[transmissionPresenceKey]transmissionPresenceState {
		ownerState := transmissionPresenceFor(owner, current)
		companionState := transmissionPresenceFor(companion, current)
		companionState.OverlayCapable = false
		companionState.InterruptCapable = false
		companionState.InterruptResumeReady = false
		return map[transmissionPresenceKey]transmissionPresenceState{
			{OrbitID: owner.OrbitID, Slot: owner.Slot}:         ownerState,
			{OrbitID: companion.OrbitID, Slot: companion.Slot}: companionState,
		}
	}
	overlay := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions",
		transmissionBody(media.ID, "own_barycenter", "overlay", "file"),
		owner.ControlToken, "capability-overlay-key-001",
	)
	if overlay.Code != http.StatusCreated {
		t.Fatalf("overlay status=%d body=%s", overlay.Code, overlay.Body.String())
	}
	overlayBody := decodeObject(t, overlay)
	if overlayBody["effective_delivery"] != "after_current" ||
		overlayBody["downgrade_reason"] != store.TransmissionDowngradeMissingOverlay {
		t.Fatalf("overlay body=%v", overlayBody)
	}

	interruptBody := transmissionBody(media.ID, "own_barycenter", "interrupt", "file")
	interruptKey := "capability-interrupt-key-01"
	challenge := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions",
		interruptBody, owner.ControlToken, interruptKey,
	)
	errorObject := assertTransmissionError(
		t, challenge, http.StatusConflict, errorRequiresConfirmation,
	)
	details := errorObject["details"].(map[string]any)
	token, ok := details["confirmation_token"].(string)
	if !ok || !transmissionConfirmationToken.MatchString(token) {
		t.Fatalf("challenge details=%v", details)
	}
	alternatives := details["alternatives"].([]any)
	if len(alternatives) != 2 || alternatives[0].(map[string]any)["delivery"] != "overlay" ||
		alternatives[0].(map[string]any)["available"] != false ||
		alternatives[1].(map[string]any)["delivery"] != "after_current" ||
		alternatives[1].(map[string]any)["available"] != true {
		t.Fatalf("challenge alternatives=%v", alternatives)
	}
	current += 1000
	confirmedBody := fmt.Sprintf(
		`{"media_id":%q,"audience":{"kind":"own_barycenter"},"delivery":"interrupt","origin_kind":"file","fallback_confirmation":{"token":%q,"delivery":"after_current"}}`,
		media.ID, token,
	)
	confirmed := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions",
		confirmedBody, owner.ControlToken, interruptKey,
	)
	if confirmed.Code != http.StatusCreated {
		t.Fatalf("confirmed status=%d body=%s", confirmed.Code, confirmed.Body.String())
	}
	confirmedObject := decodeObject(t, confirmed)
	if confirmedObject["requested_delivery"] != "interrupt" ||
		confirmedObject["effective_delivery"] != "after_current" ||
		confirmedObject["downgrade_reason"] != store.TransmissionDowngradeConfirmedAfterCurrent {
		t.Fatalf("confirmed body=%v", confirmedObject)
	}
	retry := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions",
		confirmedBody, owner.ControlToken, interruptKey,
	)
	if retry.Code != http.StatusOK || decodeObject(t, retry)["reused"] != true {
		t.Fatalf("confirmed retry status=%d body=%s", retry.Code, retry.Body.String())
	}
	wrongToken := strings.Repeat("0", 64)
	wrongBody := fmt.Sprintf(
		`{"media_id":%q,"audience":{"kind":"own_barycenter"},"delivery":"interrupt","origin_kind":"file","fallback_confirmation":{"token":"fc_%s","delivery":"after_current"}}`,
		media.ID, wrongToken,
	)
	wrong := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions",
		wrongBody, owner.ControlToken, "capability-wrong-token-001",
	)
	assertTransmissionError(t, wrong, http.StatusConflict, errorConfirmationInvalid)
}

func TestTransmissionHTTPVisibilityAndStartedCancelConflict(t *testing.T) {
	harness := newOnboardingHarness(t)
	owner, err := harness.store.CreateSelfServiceOrbit("Transmission visibility owner")
	if err != nil {
		t.Fatal(err)
	}
	companion := installTransmissionCompanion(t, harness, owner)
	stranger, err := harness.store.CreateSelfServiceOrbit("Transmission visibility stranger")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	media := readyTransmissionHTTPMedia(t, harness, owner, now, 1500)
	current := now + 3
	harness.api.transmissionNow = func() time.Time { return time.UnixMilli(current) }
	harness.api.transmissionPresence = func() map[transmissionPresenceKey]transmissionPresenceState {
		return map[transmissionPresenceKey]transmissionPresenceState{
			{OrbitID: owner.OrbitID, Slot: owner.Slot}:         transmissionPresenceFor(owner, current),
			{OrbitID: companion.OrbitID, Slot: companion.Slot}: transmissionPresenceFor(companion, current),
		}
	}
	if _, err := harness.store.CreateTransmissionBlock(store.CreateTransmissionBlockParams{
		OwnerScope: store.BlockOwnerActor, OwnerOrbitID: companion.OrbitID,
		OwnerActorID: companion.ActorID, BlockedKind: store.BlockedSubjectActor,
		BlockedActorID: owner.ActorID, AuthorizedByActorID: companion.ActorID,
		CreatedAt: current - 1,
	}); err != nil {
		t.Fatal(err)
	}
	created := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions",
		transmissionBody(media.ID, "own_barycenter", "overlay", "file"),
		owner.ControlToken, "visibility-create-key-0001",
	)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	transmissionID := decodeObject(t, created)["transmission_id"].(string)
	companionStatus := transmissionAPIRequest(
		harness.mux, http.MethodGet, "/v1/transmissions/"+transmissionID,
		"", companion.NodeToken, "",
	)
	if companionStatus.Code != http.StatusOK {
		t.Fatalf("companion status=%d body=%s", companionStatus.Code, companionStatus.Body.String())
	}
	companionObject := decodeObject(t, companionStatus)
	if len(companionObject["targets"].([]any)) != 1 || companionObject["can_cancel"] != false {
		t.Fatalf("companion status=%v", companionObject)
	}
	companionTarget := companionObject["targets"].([]any)[0].(map[string]any)
	if companionTarget["status"] != "blocked" || companionTarget["reason_code"] != nil {
		t.Fatalf("blocked target leaked rule detail: %v", companionTarget)
	}
	strangerStatus := transmissionAPIRequest(
		harness.mux, http.MethodGet, "/v1/transmissions/"+transmissionID,
		"", stranger.NodeToken, "",
	)
	assertTransmissionError(t, strangerStatus, http.StatusNotFound, errorTransmissionNotFound)

	targets, err := harness.store.TransmissionTargets(transmissionID)
	if err != nil || len(targets) != 2 {
		t.Fatalf("targets=%+v err=%v", targets, err)
	}
	target := targets[0]
	transition, err := harness.store.TransitionTransmissionTarget(
		store.TransitionTransmissionTargetParams{
			TransmissionID: transmissionID, OrbitID: target.OrbitID,
			ActorID: target.ActorID, Slot: target.Slot,
			ExpectedRevision: target.Revision, Generation: target.Generation,
			Status: store.TransmissionTargetPlaying, OccurredAt: current + 1,
		},
	)
	if err != nil || transition.Transmission.Status != store.TransmissionStatusPlaying {
		t.Fatalf("playing transition=%+v err=%v", transition, err)
	}
	current += 2
	cancel := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions/"+transmissionID+"/cancel",
		`{}`, owner.ControlToken, "",
	)
	assertTransmissionError(t, cancel, http.StatusConflict, errorTransmissionState)
	nonEmpty := transmissionAPIRequest(
		harness.mux, http.MethodPost, "/v1/transmissions/"+transmissionID+"/cancel",
		`{"force":true}`, owner.ControlToken, "",
	)
	assertTransmissionError(t, nonEmpty, http.StatusBadRequest, errorInvalidRequest)
}
