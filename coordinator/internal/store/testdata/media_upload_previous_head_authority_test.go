package store

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"
)

type mediaUploadPreviousHeadResult struct {
	AdvancedOffset  int64  `json:"advanced_offset"`
	CreatedMediaID  string `json:"created_media_id"`
	CreatedUploadID string `json:"created_upload_id"`
}

func requiredMediaUploadPreviousEnv(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s is required", name)
	}
	return value
}

func requiredMediaUploadPreviousInt64(t *testing.T, name string) int64 {
	t.Helper()
	value := requiredMediaUploadPreviousEnv(t, name)
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		t.Fatalf("%s is invalid: %q (%v)", name, value, err)
	}
	return parsed
}

// This source is copied into the exact predecessor coordinator. All reads and
// mutations therefore exercise its real repository against the additive
// upload-session schema created by the current coordinator.
func TestMediaUploadPreviousHeadAuthority(t *testing.T) {
	path := requiredMediaUploadPreviousEnv(t, "BARYCENTER_UPLOAD_PREVIOUS_DB")
	resultPath := requiredMediaUploadPreviousEnv(t, "BARYCENTER_UPLOAD_PREVIOUS_RESULT")
	orbitID := requiredMediaUploadPreviousInt64(t, "BARYCENTER_UPLOAD_PREVIOUS_ORBIT")
	actorID := requiredMediaUploadPreviousInt64(t, "BARYCENTER_UPLOAD_PREVIOUS_ACTOR")
	uploadID := requiredMediaUploadPreviousEnv(t, "BARYCENTER_UPLOAD_PREVIOUS_ID")
	now := requiredMediaUploadPreviousInt64(t, "BARYCENTER_UPLOAD_PREVIOUS_NOW")

	st, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	existing, err := st.GetMediaUploadSession(uploadID)
	if err != nil || existing == nil || existing.ReceivedSizeBytes != 0 || existing.Status != UploadStatusOpen {
		t.Fatalf("predecessor upload read=%+v err=%v", existing, err)
	}
	advanced, err := st.AdvanceMediaUpload(existing.ID, 0, 3, now)
	if err != nil {
		t.Fatal(err)
	}
	created, err := st.CreateMediaUpload(CreateMediaUploadParams{
		Media: CreateMediaItemParams{
			OwnerOrbitID: orbitID, ActorID: actorID,
			Kind: MediaKindAudioClip, Source: MediaSourceApp,
			Title: "predecessor-created-upload", CreatedAt: now + 1,
			ExpiresAt: now + int64((7*24*time.Hour)/time.Millisecond),
		},
		DeclaredSizeBytes: 5,
		SessionExpiresAt:  now + int64(time.Hour/time.Millisecond),
		IdempotencyKey:    "previous-head-created-upload-0001",
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(mediaUploadPreviousHeadResult{
		AdvancedOffset: advanced.ReceivedSizeBytes,
		CreatedMediaID: created.Media.ID, CreatedUploadID: created.Session.ID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}
