package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/store"
)

func TestPresenceProjectionStalenessSanitizationAndDND(t *testing.T) {
	harness := newOnboardingHarness(t)
	owner, err := harness.store.CreateSelfServiceOrbit("Presence home")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 14, 8, 0, 0, 0, time.UTC)
	lastSeen := now.Add(-11 * time.Second).UnixMilli()
	harness.api.transmissionNow = func() time.Time { return now }
	harness.api.transmissionPresence = func() map[transmissionPresenceKey]transmissionPresenceState {
		return map[transmissionPresenceKey]transmissionPresenceState{
			{OrbitID: owner.OrbitID, Slot: owner.Slot}: {
				Connected: true, LastSeenAt: lastSeen,
				CredentialTokenHash: transmissionDigest(owner.NodeToken),
				MediaClipCapable:    true, OverlayCapable: true,
				InterruptCapable: true, PlaybackState: "main", InterruptResumeReady: true,
			},
		}
	}

	response := apiRequest(harness.mux, http.MethodGet, "/v1/presence", "", owner.ControlToken)
	if response.Code != http.StatusOK {
		t.Fatalf("presence=%d %s", response.Code, response.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	nodes := body["nodes"].([]any)
	if len(nodes) != 1 || nodes[0].(map[string]any)["online"] != true ||
		nodes[0].(map[string]any)["playback_state"] != "main" {
		t.Fatalf("presence body=%v", body)
	}
	for _, forbidden := range []string{"actor_id", "token", "hostname", "device", "process", "microphone", "capture", "audio_level", "media_url"} {
		if strings.Contains(response.Body.String(), forbidden) {
			t.Fatalf("presence leaked %q: %s", forbidden, response.Body.String())
		}
	}

	request := `{"expected_revision":0,"mode":"messages_only"}`
	first := transmissionAPIRequest(harness.mux, http.MethodPut, "/v1/presence/dnd/local",
		request, owner.ControlToken, "presence-local-dnd-key-001")
	if first.Code != http.StatusOK {
		t.Fatalf("set DND=%d %s", first.Code, first.Body.String())
	}
	replay := transmissionAPIRequest(harness.mux, http.MethodPut, "/v1/presence/dnd/local",
		request, owner.ControlToken, "presence-local-dnd-key-001")
	if replay.Code != http.StatusOK || replay.Body.String() != first.Body.String() {
		t.Fatalf("DND replay first=%s replay=%d %s", first.Body.String(), replay.Code, replay.Body.String())
	}
	conflict := transmissionAPIRequest(harness.mux, http.MethodPut, "/v1/presence/dnd/local",
		`{"expected_revision":0,"mode":"messages_only"}`, owner.ControlToken, "presence-local-dnd-key-002")
	if conflict.Code != http.StatusConflict || !strings.Contains(conflict.Body.String(), `"current"`) {
		t.Fatalf("DND conflict=%d %s", conflict.Code, conflict.Body.String())
	}

	now = now.Add(2 * time.Second)
	stale := apiRequest(harness.mux, http.MethodGet, "/v1/presence", "", owner.ControlToken)
	staleNode := decodeObject(t, stale)["nodes"].([]any)[0].(map[string]any)
	if staleNode["online"] != false || staleNode["output_state"] != "unavailable" ||
		staleNode["playback_state"] != "unknown" || staleNode["interrupt_resume_ready"] != false {
		t.Fatalf("stale presence=%v", staleNode)
	}
}

func TestBlockSurfaceUsesOpaqueViewerReferencesAndIdempotentDelete(t *testing.T) {
	harness := newOnboardingHarness(t)
	recipient, err := harness.store.CreateSelfServiceOrbit("Recipient")
	if err != nil {
		t.Fatal(err)
	}
	sender, err := harness.store.CreateSelfServiceOrbit("Sender")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 14, 9, 0, 0, 0, time.UTC)
	harness.api.transmissionNow = func() time.Time { return now }
	ref, err := harness.store.MintTransmissionSubjectReference(recipient.ActorID,
		recipient.ControlToken, store.BlockedSubjectActor, sender.ActorID, now.UnixMilli())
	if err != nil {
		t.Fatal(err)
	}

	body := fmt.Sprintf(`{"scope":"actor","subject_ref":%q}`, ref.PublicID)
	created := transmissionAPIRequest(harness.mux, http.MethodPost, "/v1/blocks",
		body, recipient.ControlToken, "presence-block-create-key-001")
	if created.Code != http.StatusCreated {
		t.Fatalf("create block=%d %s", created.Code, created.Body.String())
	}
	createdBody := decodeObject(t, created)
	blockID, _ := createdBody["block_id"].(string)
	if !strings.HasPrefix(blockID, "bl_") || strings.Contains(created.Body.String(), "actor_id") {
		t.Fatalf("public block leaked identity: %s", created.Body.String())
	}
	if createdBody["display_name"] != "a" {
		t.Fatalf("block presentation name=%v", createdBody["display_name"])
	}
	replay := transmissionAPIRequest(harness.mux, http.MethodPost, "/v1/blocks",
		body, recipient.ControlToken, "presence-block-create-key-001")
	if replay.Code != http.StatusOK || decodeObject(t, replay)["reused"] != true {
		t.Fatalf("block replay=%d %s", replay.Code, replay.Body.String())
	}
	listed := apiRequest(harness.mux, http.MethodGet, "/v1/blocks", "", recipient.ControlToken)
	if listed.Code != http.StatusOK || !strings.Contains(listed.Body.String(), blockID) ||
		!strings.Contains(listed.Body.String(), ref.PublicID) {
		t.Fatalf("list blocks=%d %s", listed.Code, listed.Body.String())
	}

	deleted := apiRequest(harness.mux, http.MethodDelete, "/v1/blocks/"+blockID, "", recipient.ControlToken)
	if deleted.Code != http.StatusOK || decodeObject(t, deleted)["changed"] != true {
		t.Fatalf("delete=%d %s", deleted.Code, deleted.Body.String())
	}
	deletedAgain := apiRequest(harness.mux, http.MethodDelete, "/v1/blocks/"+blockID, "", recipient.ControlToken)
	if deletedAgain.Code != http.StatusOK || decodeObject(t, deletedAgain)["changed"] != false {
		t.Fatalf("delete replay=%d %s", deletedAgain.Code, deletedAgain.Body.String())
	}
	guessed := apiRequest(harness.mux, http.MethodDelete,
		"/v1/blocks/bl_01J00000000000000000000000", "", sender.ControlToken)
	if guessed.Code != http.StatusNotFound {
		t.Fatalf("guessed delete=%d %s", guessed.Code, guessed.Body.String())
	}
}
