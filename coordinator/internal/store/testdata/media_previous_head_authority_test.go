package store

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"
)

type mediaPreviousHeadResult struct {
	LegacyStatus      string `json:"legacy_status"`
	SessionPositionMS int64  `json:"session_position_ms"`
	InsertedLegacyID  string `json:"inserted_legacy_id"`
	DissolvedOrbitID  int64  `json:"dissolved_orbit_id"`
}

func requiredMediaPreviousEnv(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s is required", name)
	}
	return value
}

func requiredMediaPreviousInt64(t *testing.T, name string) int64 {
	t.Helper()
	value := requiredMediaPreviousEnv(t, name)
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		t.Fatalf("%s is invalid: %q (%v)", name, value, err)
	}
	return parsed
}

// This source is copied into the exact pre-media-ingest coordinator. Every
// read and mutation therefore uses the predecessor's real Store API rather
// than a SQL emulation of old behavior.
func TestMediaPreviousHeadAuthority(t *testing.T) {
	path := requiredMediaPreviousEnv(t, "BARYCENTER_MEDIA_PREVIOUS_DB")
	resultPath := requiredMediaPreviousEnv(t, "BARYCENTER_MEDIA_PREVIOUS_RESULT")
	keepOrbitID := requiredMediaPreviousInt64(t, "BARYCENTER_MEDIA_KEEP_ORBIT")
	dissolveOrbitID := requiredMediaPreviousInt64(t, "BARYCENTER_MEDIA_DISSOLVE_ORBIT")
	nodeToken := requiredMediaPreviousEnv(t, "BARYCENTER_MEDIA_NODE_TOKEN")
	legacyID := requiredMediaPreviousEnv(t, "BARYCENTER_MEDIA_LEGACY_ID")

	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	legacy, err := store.GetMedia(legacyID)
	if err != nil || legacy == nil {
		t.Fatalf("previous media read=%+v err=%v", legacy, err)
	}
	if orbitID, slot, ok, err := store.LookupToken(nodeToken); err != nil || !ok || orbitID != keepOrbitID || slot != "a" {
		t.Fatalf("previous token lookup orbit=%d slot=%q ok=%v err=%v", orbitID, slot, ok, err)
	}
	snapshot, err := store.LoadSession(keepOrbitID)
	if err != nil || snapshot == nil {
		t.Fatalf("previous session=%+v err=%v", snapshot, err)
	}

	legacy.Status = "processing"
	legacy.DurationMS = 1777
	if err := store.UpdateMedia(*legacy); err != nil {
		t.Fatal(err)
	}
	snapshot.SavedPositionMS = 909
	if err := store.SaveSession(keepOrbitID, *snapshot); err != nil {
		t.Fatal(err)
	}
	insertedLegacyID := "m_previous_head_added"
	if err := store.InsertMedia(MediaRecord{
		ID: insertedLegacyID, TGFileID: "tg-added-by-previous", DurationMS: 333,
		PathWAV: "/srv/previous-added.wav", LoudnormJSON: "{}",
		CreatedAt: legacy.CreatedAt + 1, ExpiresAt: legacy.ExpiresAt,
		Status: "ready", OrbitID: keepOrbitID,
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.DeleteOrbit(dissolveOrbitID); err != nil {
		t.Fatal(err)
	}
	if _, _, ok, err := store.LookupToken(nodeToken); err != nil || !ok {
		t.Fatalf("unrelated orbit dissolution changed keep token: ok=%v err=%v", ok, err)
	}

	encoded, err := json.Marshal(mediaPreviousHeadResult{
		LegacyStatus: "processing", SessionPositionMS: 909,
		InsertedLegacyID: insertedLegacyID, DissolvedOrbitID: dissolveOrbitID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}
