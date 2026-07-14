package store

import (
	"encoding/json"
	"os"
	"strconv"
	"testing"
	"time"
)

type mediaIntegrationPreviousResult struct {
	CreatedMediaID string `json:"created_media_id"`
}

func requiredMediaIntegrationPreviousEnv(t *testing.T, name string) string {
	t.Helper()
	value := os.Getenv(name)
	if value == "" {
		t.Fatalf("%s is required", name)
	}
	return value
}

func TestMediaIntegrationPreviousHeadAuthority(t *testing.T) {
	path := requiredMediaIntegrationPreviousEnv(t, "BARYCENTER_MEDIA_INTEGRATION_PREVIOUS_DB")
	resultPath := requiredMediaIntegrationPreviousEnv(t, "BARYCENTER_MEDIA_INTEGRATION_PREVIOUS_RESULT")
	existingID := requiredMediaIntegrationPreviousEnv(t, "BARYCENTER_MEDIA_INTEGRATION_PREVIOUS_EXISTING")
	orbitID, err := strconv.ParseInt(
		requiredMediaIntegrationPreviousEnv(t, "BARYCENTER_MEDIA_INTEGRATION_PREVIOUS_ORBIT"),
		10, 64,
	)
	if err != nil || orbitID <= 0 {
		t.Fatalf("invalid rollback orbit: %d (%v)", orbitID, err)
	}
	now, err := strconv.ParseInt(
		requiredMediaIntegrationPreviousEnv(t, "BARYCENTER_MEDIA_INTEGRATION_PREVIOUS_NOW"),
		10, 64,
	)
	if err != nil || now <= 0 {
		t.Fatalf("invalid rollback time: %d (%v)", now, err)
	}
	st, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	legacy, err := st.GetMedia(existingID)
	if err != nil || legacy == nil {
		t.Fatalf("previous legacy read=%+v err=%v", legacy, err)
	}
	legacy.Status = "ready"
	legacy.PathWAV = "/srv/previous-revived.wav"
	if err := st.UpdateMedia(*legacy); err != nil {
		t.Fatal(err)
	}
	created, err := st.CreateTelegramMedia(CreateTelegramMediaParams{
		OwnerOrbitID: orbitID, TelegramUserID: 7001,
		TelegramFileID: "tg-created-by-previous", CreatedAt: now,
		ExpiresAt: now + int64((30*24*time.Hour)/time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(mediaIntegrationPreviousResult{CreatedMediaID: created.Media.ID})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(resultPath, encoded, 0o600); err != nil {
		t.Fatal(err)
	}
}
