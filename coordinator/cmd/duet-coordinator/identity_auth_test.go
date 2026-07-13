package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/store"
)

func randomIdentityHex(t *testing.T, bytes int) string {
	t.Helper()
	buf := make([]byte, bytes)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	return hex.EncodeToString(buf)
}

func randomIdentityHumanSecret(t *testing.T) string {
	t.Helper()
	const alphabet = "ABCDEFGHJKMNPQRSTVWXYZ23456789"
	buf := make([]byte, 27)
	if _, err := rand.Read(buf); err != nil {
		t.Fatal(err)
	}
	for i := range buf {
		buf[i] = alphabet[int(buf[i])%len(alphabet)]
	}
	return string(buf)
}

// R1 HTTP regression: a node credential cannot be installed as another
// orbit's control credential and therefore cannot fetch that tenant's media.
func TestNodeTokenCannotEscalateAcrossOrbitMediaAuthorization(t *testing.T) {
	st, err := store.OpenWithOptions(filepath.Join(t.TempDir(), "tenant-auth.db"), store.Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	orbitA, err := st.CreateOrbit("Tenant A", 101)
	if err != nil {
		t.Fatal(err)
	}
	_, nodeA, err := st.PairSlot(orbitA.ID, 101)
	if err != nil {
		t.Fatal(err)
	}
	orbitB, err := st.CreateOrbit("Tenant B", 202)
	if err != nil {
		t.Fatal(err)
	}
	_, nodeB, err := st.PairSlot(orbitB.ID, 202)
	if err != nil {
		t.Fatal(err)
	}
	targetB, err := st.ResolveTokenActorContext(nodeB)
	if err != nil {
		t.Fatal(err)
	}
	err = st.ProvisionInstallationSecrets(
		store.Identity{Kind: store.IdentityTelegram, TelegramUserID: 202},
		targetB.ActorID,
		nodeA,
		"rec_"+randomIdentityHex(t, 16),
		randomIdentityHumanSecret(t),
	)
	if !errors.Is(err, store.ErrCredentialDomainConflict) {
		t.Fatalf("cross-orbit provisioning error = %v", err)
	}

	mediaPath := filepath.Join(t.TempDir(), "tenant-b.wav")
	if err := os.WriteFile(mediaPath, []byte("tenant-b-media"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := st.InsertMedia(store.MediaRecord{
		ID:        "tenant-b-only",
		PathWAV:   mediaPath,
		CreatedAt: time.Now().UnixMilli(),
		ExpiresAt: time.Now().Add(time.Hour).UnixMilli(),
		Status:    "ready",
		OrbitID:   orbitB.ID,
	}); err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/media/tenant-b-only.wav", nil)
	req.Header.Set("Authorization", "Bearer "+nodeA)
	response := httptest.NewRecorder()
	mediaHandler(st).ServeHTTP(response, req)
	if response.Code != http.StatusNotFound {
		t.Fatalf("tenant A node fetched tenant B media: status=%d body=%q", response.Code, response.Body.String())
	}
}
