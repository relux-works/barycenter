package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/media"
	"relux.works/duet/coordinator/internal/moderation"
	"relux.works/duet/coordinator/internal/store"
)

type moderationHTTPSink struct{}

func (moderationHTTPSink) CancelMedia(context.Context, store.MediaDeliveryCancellation) error {
	return nil
}

type moderationHTTPFixture struct {
	harness  onboardingHarness
	source   store.OnboardingCredentials
	reporter store.OnboardingCredentials
	media    store.MediaItem
	operator store.ModerationOperatorCredential
}

func newModerationHTTPFixture(t *testing.T) moderationHTTPFixture {
	t.Helper()
	harness := newOnboardingHarness(t)
	source, err := harness.store.CreateSelfServiceOrbit("HTTP moderation source")
	if err != nil {
		t.Fatal(err)
	}
	reporter, err := harness.store.CreateSelfServiceOrbit("HTTP moderation reporter")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	item, err := harness.store.CreateMediaItem(store.CreateMediaItemParams{
		OwnerOrbitID: source.OrbitID, ActorID: source.ActorID,
		Kind: store.MediaKindAudioClip, Source: store.MediaSourceApp,
		Title: "HTTP evidence", CreatedAt: now,
		ExpiresAt: now + int64((45*24*time.Hour)/time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := harness.store.StageMediaPublication(item.ID, item.Revision, now+1)
	if err != nil {
		t.Fatal(err)
	}
	content := []byte("http-moderation-evidence")
	digest := sha256.Sum256(content)
	canonical := filepath.Join(harness.api.config.MediaDir, "canonical")
	if err := os.MkdirAll(canonical, 0o700); err != nil {
		t.Fatal(err)
	}
	path, ok := media.CanonicalPath(canonical, operation.StorageKey)
	if !ok {
		t.Fatal("invalid canonical test path")
	}
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	ready, err := harness.store.CompleteMediaPublication(
		operation.ID, operation.Revision,
		store.MediaPublication{
			MIME: "audio/wav", Codec: "pcm_s16le", DurationMS: 1000,
			SizeBytes: int64(len(content)), SHA256: hex.EncodeToString(digest[:]),
			LoudnessJSON: `{"input_i":"-20.0","input_tp":"-3.0","output_i":"-14.0","output_tp":"-1.5"}`,
		}, now+2,
	)
	if err != nil {
		t.Fatal(err)
	}
	_, err = harness.store.CreateTransmission(store.CreateTransmissionParams{
		MediaID: ready.ID, SourceOrbitID: source.OrbitID,
		SourceActorID: source.ActorID, SourceSlot: source.Slot,
		PlaybackDomainKind: store.PlaybackDomainOrbit,
		PlaybackDomainID:   source.OrbitID,
		AudienceKind:       store.TransmissionAudienceExplicit,
		OriginKind:         store.TransmissionOriginFile, IncludeOrigin: true,
		RequestedDelivery: store.TransmissionDeliveryOverlay,
		EffectiveDelivery: store.TransmissionDeliveryOverlay,
		AcceptedAt:        now + 3,
		Targets: []store.CreateTransmissionTarget{{
			OrbitID: reporter.OrbitID, ActorID: reporter.ActorID,
			Slot: reporter.Slot, OnlineAtAcceptance: true,
			MediaClipCapable: true, OverlayCapable: true,
			InterruptCapable: true, InterruptResumeReady: true,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	operator, err := harness.store.ProvisionModerationOperator(
		"HTTP moderator", store.ModerationOperatorCapabilities{
			List: true, Evidence: true, Decide: true,
		}, now+4,
	)
	if err != nil {
		t.Fatal(err)
	}
	harness.api.mediaLifecycle.SetDeliveryCancellationSink(moderationHTTPSink{})
	service, err := moderation.NewService(
		harness.store, harness.api.mediaLifecycle, harness.api.mediaDownload,
		nil, nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	harness.api.moderationService = service
	return moderationHTTPFixture{
		harness: harness, source: source, reporter: reporter,
		media: ready, operator: operator,
	}
}

func TestModerationHTTPAuthPrivacyEvidenceAndDecision(t *testing.T) {
	fixture := newModerationHTTPFixture(t)
	created := apiRequest(
		fixture.harness.mux, http.MethodPost, "/v1/reports",
		`{"media_id":"`+fixture.media.ID+`","reason":"harassment","details":"private reporter text"}`,
		fixture.reporter.ControlToken,
	)
	if created.Code != http.StatusCreated {
		t.Fatalf("create status=%d body=%s", created.Code, created.Body.String())
	}
	report := decodeObject(t, created)
	reportID := report["id"].(string)
	for _, forbidden := range []string{"actor_id", "orbit_id", "details", "reported_subject"} {
		if strings.Contains(created.Body.String(), forbidden) {
			t.Fatalf("reporter response disclosed %q: %s", forbidden, created.Body.String())
		}
	}
	if response := apiRequest(
		fixture.harness.mux, http.MethodPost, "/v1/reports",
		`{"media_id":"`+fixture.media.ID+`","reason":"spam","details":""}`,
		fixture.reporter.NodeToken,
	); response.Code != http.StatusForbidden {
		t.Fatalf("node report status=%d body=%s", response.Code, response.Body.String())
	}
	if response := apiRequest(
		fixture.harness.mux, http.MethodGet, "/v1/actor/context", "",
		fixture.operator.Token,
	); response.Code != http.StatusUnauthorized {
		t.Fatalf("operator-as-user status=%d", response.Code)
	}
	if response := apiRequest(
		fixture.harness.mux, http.MethodGet, "/v1/moderation/reports", "",
		fixture.reporter.ControlToken,
	); response.Code != http.StatusUnauthorized {
		t.Fatalf("user-as-operator status=%d", response.Code)
	}
	queue := apiRequest(
		fixture.harness.mux, http.MethodGet, "/v1/moderation/reports?status=open&limit=10", "",
		fixture.operator.Token,
	)
	if queue.Code != http.StatusOK || !strings.Contains(queue.Body.String(), "private reporter text") {
		t.Fatalf("queue status=%d body=%s", queue.Code, queue.Body.String())
	}
	evidence := apiRequest(
		fixture.harness.mux, http.MethodGet,
		"/v1/moderation/reports/"+reportID+"/evidence", "",
		fixture.operator.Token,
	)
	if evidence.Code != http.StatusOK || evidence.Body.String() != "http-moderation-evidence" {
		t.Fatalf("evidence status=%d body=%q", evidence.Code, evidence.Body.String())
	}
	audit := apiRequest(
		fixture.harness.mux, http.MethodGet,
		"/v1/moderation/reports/"+reportID+"/audit?limit=20", "",
		fixture.operator.Token,
	)
	if audit.Code != http.StatusOK ||
		!strings.Contains(audit.Body.String(), `"event_type":"report.created"`) ||
		!strings.Contains(audit.Body.String(), `"event_type":"evidence.read"`) ||
		strings.Contains(audit.Body.String(), "private reporter text") ||
		strings.Contains(audit.Body.String(), fixture.media.StorageKey) {
		t.Fatalf("audit status=%d body=%s", audit.Code, audit.Body.String())
	}
	decision := apiRequest(
		fixture.harness.mux, http.MethodPost,
		"/v1/moderation/reports/"+reportID+"/decision",
		`{"action":"no_action"}`, fixture.operator.Token,
	)
	if decision.Code != http.StatusOK || !strings.Contains(decision.Body.String(), `"state":"applied"`) {
		t.Fatalf("decision status=%d body=%s", decision.Code, decision.Body.String())
	}
	status := apiRequest(
		fixture.harness.mux, http.MethodGet, "/v1/reports/"+reportID, "",
		fixture.reporter.ControlToken,
	)
	if status.Code != http.StatusOK || !strings.Contains(status.Body.String(), `"status":"reviewed"`) {
		t.Fatalf("report status=%d body=%s", status.Code, status.Body.String())
	}
	if strings.Contains(fixture.harness.logs.String(), fixture.operator.Token) ||
		strings.Contains(fixture.harness.logs.String(), fixture.reporter.ControlToken) ||
		strings.Contains(fixture.harness.logs.String(), fixture.harness.api.config.MediaDir) {
		t.Fatalf("secret or local path leaked to logs: %s", fixture.harness.logs.String())
	}
}

func TestModerationHTTPRevokedAndLeastPrivilegeOperatorsFailClosed(t *testing.T) {
	fixture := newModerationHTTPFixture(t)
	listOnly, err := fixture.harness.store.ProvisionModerationOperator(
		"List only", store.ModerationOperatorCapabilities{List: true},
		time.Now().UnixMilli(),
	)
	if err != nil {
		t.Fatal(err)
	}
	created := apiRequest(
		fixture.harness.mux, http.MethodPost, "/v1/reports",
		`{"media_id":"`+fixture.media.ID+`","reason":"spam","details":""}`,
		fixture.reporter.ControlToken,
	)
	reportID := decodeObject(t, created)["id"].(string)
	response := apiRequest(
		fixture.harness.mux, http.MethodGet,
		"/v1/moderation/reports/"+reportID+"/evidence", "", listOnly.Token,
	)
	if response.Code != http.StatusForbidden {
		t.Fatalf("list-only evidence status=%d body=%s", response.Code, response.Body.String())
	}
	audit := apiRequest(
		fixture.harness.mux, http.MethodGet,
		"/v1/moderation/reports/"+reportID+"/audit", "", listOnly.Token,
	)
	if audit.Code != http.StatusOK ||
		!strings.Contains(audit.Body.String(), `"event_type":"report.created"`) {
		t.Fatalf("list-only audit status=%d body=%s", audit.Code, audit.Body.String())
	}
	if _, err := fixture.harness.store.RevokeModerationOperator(
		fixture.operator.Operator.ID, time.Now().UnixMilli(),
	); err != nil {
		t.Fatal(err)
	}
	response = apiRequest(
		fixture.harness.mux, http.MethodGet, "/v1/moderation/reports", "",
		fixture.operator.Token,
	)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("revoked operator status=%d body=%s", response.Code, response.Body.String())
	}
}
