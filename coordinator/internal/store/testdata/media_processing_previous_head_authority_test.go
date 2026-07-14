package store

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"
)

type mediaProcessingPreviousHeadResult struct {
	ReadyMediaID   string `json:"ready_media_id"`
	ReadySHA256    string `json:"ready_sha256"`
	CreatedUpload  string `json:"created_upload"`
	InsertedLegacy string `json:"inserted_legacy"`
}

func TestMediaProcessingPreviousHeadAuthority(t *testing.T) {
	path := os.Getenv("BARYCENTER_PROCESSING_PREVIOUS_DB")
	resultPath := os.Getenv("BARYCENTER_PROCESSING_PREVIOUS_RESULT")
	mediaID := os.Getenv("BARYCENTER_PROCESSING_PREVIOUS_MEDIA")
	wantSHA := os.Getenv("BARYCENTER_PROCESSING_PREVIOUS_SHA")
	orbitID, err := strconv.ParseInt(os.Getenv("BARYCENTER_PROCESSING_PREVIOUS_ORBIT"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	actorID, err := strconv.ParseInt(os.Getenv("BARYCENTER_PROCESSING_PREVIOUS_ACTOR"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	now, err := strconv.ParseInt(os.Getenv("BARYCENTER_PROCESSING_PREVIOUS_NOW"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	st, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ready, err := st.GetMediaItem(mediaID)
	if err != nil || ready == nil || ready.Status != MediaStatusReady || ready.SHA256 != wantSHA ||
		ready.MIME != "audio/wav" || ready.Codec != "pcm_s16le" || ready.StorageKey == "" {
		t.Fatalf("current ready media through predecessor=%+v err=%v", ready, err)
	}
	upload, err := st.CreateMediaUpload(CreateMediaUploadParams{
		Media: CreateMediaItemParams{
			OwnerOrbitID: orbitID,
			ActorID:      actorID,
			Kind:         MediaKindVoiceClip,
			Source:       MediaSourceApp,
			Title:        "created-by-immediate-predecessor",
			CreatedAt:    now,
			ExpiresAt:    now + int64((7*24*time.Hour)/time.Millisecond),
		},
		DeclaredSizeBytes: 8,
		SessionExpiresAt:  now + int64(time.Hour/time.Millisecond),
		IdempotencyKey:    "processing-previous-create-0001",
	})
	if err != nil {
		t.Fatal(err)
	}
	legacyID := "m_processing_previous_legacy"
	if err := st.InsertMedia(MediaRecord{
		ID: legacyID, TGFileID: "tg-processing-previous", DurationMS: 1000,
		PathWAV: "/srv/processing-previous.wav", LoudnormJSON: "{}",
		CreatedAt: now, ExpiresAt: now + int64((7*24*time.Hour)/time.Millisecond),
		Status: "ready", OrbitID: orbitID,
	}); err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(mediaProcessingPreviousHeadResult{
		ReadyMediaID: ready.ID, ReadySHA256: ready.SHA256,
		CreatedUpload: upload.Session.ID, InsertedLegacy: legacyID,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}
