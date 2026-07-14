package store

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"
)

type mediaLifecyclePreviousHeadResult struct {
	CreatedUpload string `json:"created_upload"`
}

func TestMediaLifecyclePreviousHeadAuthority(t *testing.T) {
	path := os.Getenv("BARYCENTER_LIFECYCLE_PREVIOUS_DB")
	resultPath := os.Getenv("BARYCENTER_LIFECYCLE_PREVIOUS_RESULT")
	mediaID := os.Getenv("BARYCENTER_LIFECYCLE_PREVIOUS_MEDIA")
	orbitID, err := strconv.ParseInt(os.Getenv("BARYCENTER_LIFECYCLE_PREVIOUS_ORBIT"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	actorID, err := strconv.ParseInt(os.Getenv("BARYCENTER_LIFECYCLE_PREVIOUS_ACTOR"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	now, err := strconv.ParseInt(os.Getenv("BARYCENTER_LIFECYCLE_PREVIOUS_NOW"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	st, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	deleted, err := st.GetMediaItem(mediaID)
	if err != nil || deleted == nil || deleted.Status != MediaStatusDeleted || deleted.StorageKey != "" {
		t.Fatalf("new deleted media through predecessor=%+v err=%v", deleted, err)
	}
	upload, err := st.CreateMediaUpload(CreateMediaUploadParams{
		Media: CreateMediaItemParams{
			OwnerOrbitID: orbitID,
			ActorID:      actorID,
			Kind:         MediaKindVoiceClip,
			Source:       MediaSourceApp,
			Title:        "created-by-lifecycle-predecessor",
			CreatedAt:    now,
			ExpiresAt:    now + int64((7*24*time.Hour)/time.Millisecond),
		},
		DeclaredSizeBytes: 8,
		SessionExpiresAt:  now + int64(time.Hour/time.Millisecond),
		IdempotencyKey:    "lifecycle-previous-create-0001",
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(mediaLifecyclePreviousHeadResult{CreatedUpload: upload.Session.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}
