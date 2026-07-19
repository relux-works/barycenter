package main

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/media"
	"relux.works/duet/coordinator/internal/store"
)

type httpTargetSnapshotReader struct {
	mu     sync.Mutex
	grants map[store.MediaTargetIdentity]bool
}

func (reader *httpTargetSnapshotReader) AllowsMediaDownload(
	_ context.Context,
	target store.MediaTargetIdentity,
) (bool, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	return reader.grants[target], nil
}

func (reader *httpTargetSnapshotReader) WithMediaDownloadAuthorization(
	_ context.Context,
	target store.MediaTargetIdentity,
	authorized func() error,
) (bool, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	if !reader.grants[target] {
		return false, nil
	}
	return true, authorized()
}

func (reader *httpTargetSnapshotReader) grant(target store.MediaTargetIdentity) {
	reader.mu.Lock()
	reader.grants[target] = true
	reader.mu.Unlock()
}

func installHTTPTestTargetLease(
	t *testing.T,
	harness onboardingHarness,
	mediaID string,
	target store.OnboardingCredentials,
) (*httpTargetSnapshotReader, store.ActorContext) {
	t.Helper()
	ctx, err := harness.store.ResolveTokenActorContext(target.NodeToken)
	if err != nil {
		t.Fatal(err)
	}
	reader := &httpTargetSnapshotReader{grants: map[store.MediaTargetIdentity]bool{{
		MediaID: mediaID, OrbitID: ctx.OrbitID, ActorID: ctx.ActorID, Slot: ctx.Slot,
	}: true}}
	if !harness.api.mediaDownload.SetTargetSnapshotReader(reader) {
		t.Fatal("test authorization-lease reader was rejected")
	}
	return reader, ctx
}

func readyDownloadHTTPMedia(
	t *testing.T,
	harness onboardingHarness,
	credentials store.OnboardingCredentials,
	createdAt int64,
	expiresAt int64,
	payload []byte,
) store.MediaItem {
	t.Helper()
	item, err := harness.store.CreateMediaItem(store.CreateMediaItemParams{
		OwnerOrbitID: credentials.OrbitID,
		ActorID:      credentials.ActorID,
		Kind:         store.MediaKindVoiceClip,
		Source:       store.MediaSourceApp,
		Title:        "private-download-title",
		CreatedAt:    createdAt,
		ExpiresAt:    expiresAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := harness.store.StageMediaPublication(item.ID, item.Revision, createdAt+1)
	if err != nil {
		t.Fatal(err)
	}
	path, ok := media.CanonicalPath(
		filepath.Join(harness.api.config.MediaDir, "canonical"), operation.StorageKey,
	)
	if !ok {
		t.Fatal("invalid HTTP download storage key")
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(payload)
	ready, err := harness.store.CompleteMediaPublication(
		operation.ID,
		operation.Revision,
		store.MediaPublication{
			MIME: "audio/wav", Codec: "pcm_s16le", DurationMS: 1000,
			SizeBytes: int64(len(payload)), SHA256: fmt.Sprintf("%x", digest),
			LoudnessJSON: `{"input_i":"-20.0","input_tp":"-3.0","output_i":"-14.0","output_tp":"-1.5"}`,
		},
		createdAt+2,
	)
	if err != nil {
		t.Fatal(err)
	}
	return ready
}

func persistHTTPMediaTarget(
	t *testing.T,
	harness onboardingHarness,
	mediaItem store.MediaItem,
	source, target store.OnboardingCredentials,
	acceptedAt int64,
) {
	t.Helper()
	created, err := harness.store.CreateTransmission(store.CreateTransmissionParams{
		MediaID: mediaItem.ID, SourceOrbitID: source.OrbitID,
		SourceActorID: source.ActorID, SourceSlot: source.Slot,
		PlaybackDomainKind: store.PlaybackDomainOrbit,
		PlaybackDomainID:   source.OrbitID,
		AudienceKind:       store.TransmissionAudienceExplicit,
		OriginKind:         store.TransmissionOriginFile,
		IncludeOrigin:      false,
		RequestedDelivery:  store.TransmissionDeliveryOverlay,
		EffectiveDelivery:  store.TransmissionDeliveryOverlay,
		AcceptedAt:         acceptedAt,
		Targets: []store.CreateTransmissionTarget{{
			OrbitID: target.OrbitID, ActorID: target.ActorID, Slot: target.Slot,
			OnlineAtAcceptance: true, MediaClipCapable: true, OverlayCapable: true,
			InterruptCapable: true, InterruptResumeReady: true,
		}},
	})
	if err != nil || created.Transmission.MediaID != mediaItem.ID {
		t.Fatalf("create persisted media target=%+v err=%v", created, err)
	}
}

func readyHTTPStreamVariant(
	t *testing.T,
	harness onboardingHarness,
	credentials store.OnboardingCredentials,
	payload []byte,
) (store.MediaItem, store.StreamVariant) {
	t.Helper()
	now := time.Now().UnixMilli()
	item, err := harness.store.CreateMediaItem(store.CreateMediaItemParams{
		OwnerOrbitID: credentials.OrbitID, ActorID: credentials.ActorID,
		Kind: store.MediaKindAudioTrack, Source: store.MediaSourceApp,
		Title: "private-range-track.mp3", CreatedAt: now,
		ExpiresAt: now + int64((7*24*time.Hour)/time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	digest := fmt.Sprintf("%x", sha256.Sum256(payload))
	chunkSize := int64(len(payload))
	if chunkSize > maxStreamRangeBytes {
		chunkSize = maxStreamRangeBytes
	}
	chunks := make([]store.StreamChunk, 0, (int64(len(payload))+chunkSize-1)/chunkSize)
	for start := int64(0); start < int64(len(payload)); start += chunkSize {
		end := start + chunkSize
		if end > int64(len(payload)) {
			end = int64(len(payload))
		}
		chunkDigest := fmt.Sprintf("%x", sha256.Sum256(payload[start:end]))
		chunks = append(chunks, store.StreamChunk{
			Index: len(chunks), Start: start, End: end - 1,
			Bytes: end - start, SHA256: chunkDigest,
		})
	}
	if _, err := harness.store.CreateStreamTrackMetadata(store.CreateStreamTrackMetadataParams{
		MediaID: item.ID, OriginalFilename: "private-range-track.mp3",
		OriginalMIME: "audio/mpeg", OriginalContainer: "mp3", OriginalCodec: "mp3",
		OriginalSizeBytes: int64(len(payload)), OriginalSHA256: digest,
		OriginalDurationMS: 1000, OriginalSampleRateHz: 48000, OriginalChannels: 2,
		CreatedAt: now + 1,
	}); err != nil {
		t.Fatal(err)
	}
	publication, err := harness.store.StageMediaPublication(item.ID, item.Revision, now+2)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := harness.store.CompleteMediaPublication(
		publication.ID, publication.Revision,
		store.MediaPublication{
			MIME: "audio/mpeg", Codec: "mp3", DurationMS: 1000,
			SizeBytes: int64(len(payload)), SHA256: digest,
			LoudnessJSON: `{"input_i":"-14","input_tp":"-1","output_i":"-14","output_tp":"-1"}`,
		}, now+3,
	)
	if err != nil {
		t.Fatal(err)
	}
	variant, err := harness.store.CreateStagedStreamVariant(store.CreateStreamVariantParams{
		MediaID: ready.ID, Purpose: "canonical", Profile: "http-range-test-v1",
		Codec: "mp3", Container: "mp3", MIME: "audio/mpeg", RateMode: "cbr",
		BitrateBPS: 128000, SampleRateHz: 48000, Channels: 2, DurationMS: 1000,
		SizeBytes: int64(len(payload)), SHA256: digest, ETag: store.CreateStrongStreamETag(digest),
		StorageKey: "stream/v1/" + digest, ChunkSizeBytes: chunkSize,
		Chunks:  chunks,
		SeekMap: []store.StreamSeekPoint{{TimeMS: 0, Offset: 0}}, CreatedAt: now + 4,
	})
	if err != nil {
		t.Fatal(err)
	}
	variant, err = harness.store.PublishStreamVariant(variant.ID, variant.Revision, now+5)
	if err != nil {
		t.Fatal(err)
	}
	path, ok := media.StreamVariantPath(
		filepath.Join(harness.api.config.MediaDir, "stream"), variant.StorageKey,
	)
	if !ok {
		t.Fatal("invalid HTTP stream path")
	}
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	return ready, variant
}

func streamVariantRequest(
	handler http.Handler,
	method, path, bearer string,
	headers map[string]string,
) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, nil)
	req.RemoteAddr = "127.0.0.1:34567"
	req.Header.Set("Authorization", "Bearer "+bearer)
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func TestStreamVariantHTTPRangesConditionalsEgressAndRevocation(t *testing.T) {
	harness := newOnboardingHarness(t)
	source, err := harness.store.CreateSelfServiceOrbit("HTTP stream source")
	if err != nil {
		t.Fatal(err)
	}
	target, err := harness.store.CreateSelfServiceOrbit("HTTP stream target")
	if err != nil {
		t.Fatal(err)
	}
	foreign, err := harness.store.CreateSelfServiceOrbit("HTTP stream foreign")
	if err != nil {
		t.Fatal(err)
	}
	payload := []byte("0123456789-private-stream-range-payload")
	ready, variant := readyHTTPStreamVariant(t, harness, source, payload)
	reader, targetContext := installHTTPTestTargetLease(t, harness, ready.ID, target)
	path := "/v1/media/" + ready.ID + "/variants/" + variant.ID

	full := streamVariantRequest(harness.mux, http.MethodGet, path, target.NodeToken, nil)
	if full.Code != http.StatusOK || full.Body.String() != string(payload) {
		t.Fatalf("full status=%d body=%q", full.Code, full.Body.String())
	}
	for name, want := range map[string]string{
		"Accept-Ranges": "bytes", "Cache-Control": "private, no-store",
		"Content-Type": "audio/mpeg", "Content-Length": fmt.Sprint(len(payload)),
		"ETag": variant.ETag, "X-Content-SHA256": variant.SHA256,
		"X-Content-Type-Options": "nosniff",
		"Vary":                   "Authorization, X-Codec-Spike-Target",
	} {
		if got := full.Header().Get(name); got != want {
			t.Fatalf("full header %s=%q want=%q", name, got, want)
		}
	}
	partial := streamVariantRequest(harness.mux, http.MethodGet, path, target.NodeToken,
		map[string]string{"Range": "bytes=5-10"})
	if partial.Code != http.StatusPartialContent || partial.Body.String() != string(payload[5:11]) ||
		partial.Header().Get("Content-Range") != fmt.Sprintf("bytes 5-10/%d", len(payload)) {
		t.Fatalf("partial status=%d headers=%v body=%q", partial.Code, partial.Header(), partial.Body.String())
	}
	ifRangeMatched := streamVariantRequest(harness.mux, http.MethodGet, path, target.NodeToken,
		map[string]string{"Range": "bytes=5-10", "If-Range": variant.ETag})
	if ifRangeMatched.Code != http.StatusPartialContent ||
		ifRangeMatched.Body.String() != string(payload[5:11]) {
		t.Fatalf("matching if-range status=%d body=%q", ifRangeMatched.Code, ifRangeMatched.Body.String())
	}
	ifRange := streamVariantRequest(harness.mux, http.MethodGet, path, target.NodeToken,
		map[string]string{"Range": "bytes=5-10", "If-Range": `"sha256-stale"`})
	if ifRange.Code != http.StatusOK || ifRange.Body.String() != string(payload) ||
		ifRange.Header().Get("Content-Range") != "" {
		t.Fatalf("if-range status=%d headers=%v body=%q", ifRange.Code, ifRange.Header(), ifRange.Body.String())
	}
	head := streamVariantRequest(harness.mux, http.MethodHead, path, target.NodeToken,
		map[string]string{"Range": "bytes=-7"})
	if head.Code != http.StatusPartialContent || head.Body.Len() != 0 ||
		head.Header().Get("Content-Length") != "7" {
		t.Fatalf("head status=%d headers=%v body=%q", head.Code, head.Header(), head.Body.String())
	}
	openEnded := streamVariantRequest(harness.mux, http.MethodGet, path, target.NodeToken,
		map[string]string{"Range": fmt.Sprintf("bytes=%d-", len(payload)-4)})
	if openEnded.Code != http.StatusPartialContent ||
		openEnded.Body.String() != string(payload[len(payload)-4:]) {
		t.Fatalf("open-ended status=%d body=%q", openEnded.Code, openEnded.Body.String())
	}
	notModified := streamVariantRequest(harness.mux, http.MethodGet, path, target.NodeToken,
		map[string]string{"If-None-Match": variant.ETag})
	if notModified.Code != http.StatusNotModified || notModified.Body.Len() != 0 {
		t.Fatalf("not-modified status=%d body=%q", notModified.Code, notModified.Body.String())
	}
	repeatedConditionalRequest := httptest.NewRequest(http.MethodGet, path, nil)
	repeatedConditionalRequest.RemoteAddr = "127.0.0.1:34567"
	repeatedConditionalRequest.Header.Set("Authorization", "Bearer "+target.NodeToken)
	repeatedConditionalRequest.Header.Add("If-None-Match", `"sha256-stale"`)
	repeatedConditionalRequest.Header.Add("If-None-Match", variant.ETag)
	repeatedConditional := httptest.NewRecorder()
	harness.mux.ServeHTTP(repeatedConditional, repeatedConditionalRequest)
	if repeatedConditional.Code != http.StatusNotModified || repeatedConditional.Body.Len() != 0 {
		t.Fatalf("repeated conditional status=%d body=%q", repeatedConditional.Code, repeatedConditional.Body.String())
	}
	unsatisfied := streamVariantRequest(harness.mux, http.MethodGet, path, target.NodeToken,
		map[string]string{"Range": "bytes=999-"})
	if unsatisfied.Code != http.StatusRequestedRangeNotSatisfiable ||
		unsatisfied.Header().Get("Content-Range") != fmt.Sprintf("bytes */%d", len(payload)) ||
		unsatisfied.Body.Len() != 0 {
		t.Fatalf("416 status=%d headers=%v body=%q", unsatisfied.Code, unsatisfied.Header(), unsatisfied.Body.String())
	}
	multiple := streamVariantRequest(harness.mux, http.MethodGet, path, target.NodeToken,
		map[string]string{"Range": "bytes=0-1,4-5"})
	if multiple.Code != http.StatusRequestedRangeNotSatisfiable || multiple.Body.Len() != 0 {
		t.Fatalf("multiple range status=%d body=%q", multiple.Code, multiple.Body.String())
	}
	repeatedRangeRequest := httptest.NewRequest(http.MethodGet, path, nil)
	repeatedRangeRequest.RemoteAddr = "127.0.0.1:34567"
	repeatedRangeRequest.Header.Set("Authorization", "Bearer "+target.NodeToken)
	repeatedRangeRequest.Header.Add("Range", "bytes=0-1")
	repeatedRangeRequest.Header.Add("Range", "bytes=4-5")
	repeatedRange := httptest.NewRecorder()
	harness.mux.ServeHTTP(repeatedRange, repeatedRangeRequest)
	if repeatedRange.Code != http.StatusRequestedRangeNotSatisfiable || repeatedRange.Body.Len() != 0 {
		t.Fatalf("repeated range status=%d body=%q", repeatedRange.Code, repeatedRange.Body.String())
	}
	largePayload := make([]byte, maxStreamRangeBytes+1)
	largeReady, largeVariant := readyHTTPStreamVariant(t, harness, source, largePayload)
	reader.grant(store.MediaTargetIdentity{
		MediaID: largeReady.ID, OrbitID: targetContext.OrbitID,
		ActorID: targetContext.ActorID, Slot: targetContext.Slot,
	})
	largeRange := streamVariantRequest(
		harness.mux, http.MethodGet,
		"/v1/media/"+largeReady.ID+"/variants/"+largeVariant.ID,
		target.NodeToken,
		map[string]string{"Range": fmt.Sprintf("bytes=0-%d", maxStreamRangeBytes)},
	)
	if largeRange.Code != http.StatusRequestedRangeNotSatisfiable || largeRange.Body.Len() != 0 {
		t.Fatalf("oversized range status=%d body_bytes=%d", largeRange.Code, largeRange.Body.Len())
	}
	unknown := streamVariantRequest(harness.mux, http.MethodGet,
		"/v1/media/"+ready.ID+"/variants/sv_00000000000000000000000000", target.NodeToken, nil)
	for name, token := range map[string]string{
		"foreign node": foreign.NodeToken, "owner control": source.ControlToken,
		"target control": target.ControlToken,
	} {
		denied := streamVariantRequest(harness.mux, http.MethodGet, path, token, nil)
		if denied.Code != http.StatusNotFound || denied.Body.String() != unknown.Body.String() {
			t.Fatalf("%s status=%d body=%q unknown=%q", name, denied.Code, denied.Body.String(), unknown.Body.String())
		}
	}
	usage, err := harness.store.GetStreamAccountingUsage("actor", source.ActorID, time.Now().UnixMilli())
	wantActual := int64(len(payload) + 6 + 6 + len(payload) + 4)
	if err != nil || usage.ActualEgressBytes24h != wantActual || usage.RangeRequests24h != 5 ||
		usage.ActiveEgressReservedBytes != 0 {
		t.Fatalf("stream HTTP usage=%+v want_actual=%d err=%v", usage, wantActual, err)
	}
	if _, err := harness.store.RevokeStreamVariant(variant.ID, variant.Revision, time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	revoked := streamVariantRequest(harness.mux, http.MethodGet, path, target.NodeToken, nil)
	if revoked.Code != http.StatusNotFound || revoked.Body.String() != unknown.Body.String() {
		t.Fatalf("revoked status=%d body=%q", revoked.Code, revoked.Body.String())
	}
	deletable, deleteVariant := readyHTTPStreamVariant(
		t, harness, source, []byte("separate-delete-revocation-track"),
	)
	reader.grant(store.MediaTargetIdentity{
		MediaID: deletable.ID, OrbitID: targetContext.OrbitID,
		ActorID: targetContext.ActorID, Slot: targetContext.Slot,
	})
	deletePath := "/v1/media/" + deletable.ID + "/variants/" + deleteVariant.ID
	if _, err := harness.store.DeleteMediaItem(deletable.ID, deletable.Revision, time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	deleted := streamVariantRequest(harness.mux, http.MethodGet, deletePath, target.NodeToken, nil)
	if deleted.Code != http.StatusNotFound || deleted.Body.String() != unknown.Body.String() {
		t.Fatalf("deleted status=%d body=%q", deleted.Code, deleted.Body.String())
	}
	moderated, moderatedVariant := readyHTTPStreamVariant(
		t, harness, source, []byte("separate-moderation-delete-track"),
	)
	reader.grant(store.MediaTargetIdentity{
		MediaID: moderated.ID, OrbitID: targetContext.OrbitID,
		ActorID: targetContext.ActorID, Slot: targetContext.Slot,
	})
	moderatedPath := "/v1/media/" + moderated.ID + "/variants/" + moderatedVariant.ID
	if _, err := harness.store.DeleteMediaForModeration(moderated.ID, time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	moderatorDeleted := streamVariantRequest(
		harness.mux, http.MethodGet, moderatedPath, target.NodeToken, nil,
	)
	if moderatorDeleted.Code != http.StatusNotFound ||
		moderatorDeleted.Body.String() != unknown.Body.String() {
		t.Fatalf("moderator deleted status=%d body=%q", moderatorDeleted.Code, moderatorDeleted.Body.String())
	}
	disabled, disabledVariant := readyHTTPStreamVariant(
		t, harness, source, []byte("separate-owner-disable-track"),
	)
	reader.grant(store.MediaTargetIdentity{
		MediaID: disabled.ID, OrbitID: targetContext.OrbitID,
		ActorID: targetContext.ActorID, Slot: targetContext.Slot,
	})
	disabledPath := "/v1/media/" + disabled.ID + "/variants/" + disabledVariant.ID
	if _, err := harness.store.DisableActorForModeration(source.ActorID, time.Now().UnixMilli()); err != nil {
		t.Fatal(err)
	}
	disabledOwner := streamVariantRequest(
		harness.mux, http.MethodGet, disabledPath, target.NodeToken, nil,
	)
	if disabledOwner.Code != http.StatusNotFound || disabledOwner.Body.String() != unknown.Body.String() {
		t.Fatalf("disabled owner status=%d body=%q", disabledOwner.Code, disabledOwner.Body.String())
	}
}

func TestStreamVariantHTTPQuotaFailsBeforeBytes(t *testing.T) {
	harness := newOnboardingHarness(t)
	source, err := harness.store.CreateSelfServiceOrbit("HTTP stream quota source")
	if err != nil {
		t.Fatal(err)
	}
	target, err := harness.store.CreateSelfServiceOrbit("HTTP stream quota target")
	if err != nil {
		t.Fatal(err)
	}
	ready, variant := readyHTTPStreamVariant(t, harness, source, []byte("quota-track-payload"))
	installHTTPTestTargetLease(t, harness, ready.ID, target)
	now := time.Now().UnixMilli()
	operator, err := harness.store.ProvisionModerationOperator(
		"Stream quota test", store.ModerationOperatorCapabilities{Decide: true}, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	usage, err := harness.store.GetStreamAccountingUsage("actor", source.ActorID, now+1)
	if err != nil {
		t.Fatal(err)
	}
	policy := usage.Policy
	policy.ScopeID = source.ActorID
	policy.MaxEgressBytes24h = 1
	if _, err := harness.store.SetStreamQuotaPolicy(
		operator.Operator.ID, operator.Token, policy, 0, "http_range_quota_test", now+2,
	); err != nil {
		t.Fatal(err)
	}
	response := streamVariantRequest(
		harness.mux, http.MethodGet,
		"/v1/media/"+ready.ID+"/variants/"+variant.ID,
		target.NodeToken, map[string]string{"Range": "bytes=0-3"},
	)
	if response.Code != http.StatusTooManyRequests ||
		!strings.Contains(response.Body.String(), errorUploadQuota) {
		t.Fatalf("quota response status=%d body=%q", response.Code, response.Body.String())
	}
	usage, err = harness.store.GetStreamAccountingUsage("actor", source.ActorID, now+3)
	if err != nil || usage.ActualEgressBytes24h != 0 || usage.RangeRequests24h != 0 {
		t.Fatalf("quota usage=%+v err=%v", usage, err)
	}
}

func TestStreamVariantHTTPTinyRangesConsumeRequestFloor(t *testing.T) {
	harness := newOnboardingHarness(t)
	source, err := harness.store.CreateSelfServiceOrbit("HTTP tiny range source")
	if err != nil {
		t.Fatal(err)
	}
	target, err := harness.store.CreateSelfServiceOrbit("HTTP tiny range target")
	if err != nil {
		t.Fatal(err)
	}
	ready, variant := readyHTTPStreamVariant(t, harness, source, []byte("tiny-range-floor-payload"))
	installHTTPTestTargetLease(t, harness, ready.ID, target)
	now := time.Now().UnixMilli()
	operator, err := harness.store.ProvisionModerationOperator(
		"Tiny range quota test", store.ModerationOperatorCapabilities{Decide: true}, now,
	)
	if err != nil {
		t.Fatal(err)
	}
	usage, err := harness.store.GetStreamAccountingUsage("actor", source.ActorID, now+1)
	if err != nil {
		t.Fatal(err)
	}
	policy := usage.Policy
	policy.ScopeID = source.ActorID
	policy.MaxEgressBytes24h = 2 * store.StreamRangeRequestChargeBytes
	if _, err := harness.store.SetStreamQuotaPolicy(
		operator.Operator.ID, operator.Token, policy, 0, "http_tiny_range_floor", now+2,
	); err != nil {
		t.Fatal(err)
	}
	path := "/v1/media/" + ready.ID + "/variants/" + variant.ID
	for index := 0; index < 2; index++ {
		response := streamVariantRequest(
			harness.mux, http.MethodGet, path, target.NodeToken,
			map[string]string{"Range": "bytes=0-0"},
		)
		if response.Code != http.StatusPartialContent || response.Body.String() != "t" {
			t.Fatalf("tiny range %d status=%d body=%q", index, response.Code, response.Body.String())
		}
	}
	rejected := streamVariantRequest(
		harness.mux, http.MethodGet, path, target.NodeToken,
		map[string]string{"Range": "bytes=0-0"},
	)
	if rejected.Code != http.StatusTooManyRequests ||
		!strings.Contains(rejected.Body.String(), errorUploadQuota) {
		t.Fatalf("tiny range rejection status=%d body=%q", rejected.Code, rejected.Body.String())
	}
	usage, err = harness.store.GetStreamAccountingUsage("actor", source.ActorID, now+3)
	if err != nil || usage.ActualEgressBytes24h != 2 || usage.RangeRequests24h != 2 ||
		usage.ActiveEgressReservedBytes != 0 {
		t.Fatalf("tiny range usage=%+v err=%v", usage, err)
	}
}

func TestMediaDownloadHTTPEnforcesOwnerAndExactTargetACL(t *testing.T) {
	harness := newOnboardingHarness(t)
	owner, err := harness.store.CreateSelfServiceOrbit("HTTP download owner")
	if err != nil {
		t.Fatal(err)
	}
	target, err := harness.store.CreateSelfServiceOrbit("HTTP download target")
	if err != nil {
		t.Fatal(err)
	}
	nontarget, err := harness.store.CreateSelfServiceOrbit("HTTP download non-target")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	payload := []byte("canonical-private-wav-bytes")
	ready := readyDownloadHTTPMedia(
		t, harness, owner, now, now+int64((7*24*time.Hour)/time.Millisecond), payload,
	)
	persistHTTPMediaTarget(t, harness, ready, owner, target, now+3)

	ownerResponse := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+ready.ID, "", owner.ControlToken,
	)
	if ownerResponse.Code != http.StatusOK || ownerResponse.Body.String() != string(payload) {
		t.Fatalf("owner download status=%d body=%q", ownerResponse.Code, ownerResponse.Body.String())
	}
	if ownerResponse.Header().Get("Cache-Control") != "no-store" ||
		ownerResponse.Header().Get("Content-Type") != "audio/wav" ||
		ownerResponse.Header().Get("X-Content-Type-Options") != "nosniff" ||
		ownerResponse.Header().Get("ETag") != `"`+ready.SHA256+`"` {
		t.Fatalf("owner download headers=%v", ownerResponse.Header())
	}
	targetResponse := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+ready.ID, "", target.NodeToken,
	)
	if targetResponse.Code != http.StatusOK || targetResponse.Body.String() != string(payload) {
		t.Fatalf("target download status=%d body=%q", targetResponse.Code, targetResponse.Body.String())
	}
	ownerNode := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+ready.ID, "", owner.NodeToken,
	)
	nontargetResponse := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+ready.ID, "", nontarget.NodeToken,
	)
	foreignControl := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+ready.ID, "", target.ControlToken,
	)
	unknown := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/m_00000000000000000000000000", "", target.NodeToken,
	)
	malformed := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/copied-url", "", target.NodeToken,
	)
	for name, response := range map[string]*httptest.ResponseRecorder{
		"owner_node_without_snapshot": ownerNode,
		"nontarget":                   nontargetResponse,
		"foreign_control":             foreignControl,
		"unknown":                     unknown,
		"malformed":                   malformed,
	} {
		if response.Code != http.StatusNotFound || response.Body.String() != unknown.Body.String() {
			t.Fatalf("%s response=(%d,%q) unknown=(%d,%q)",
				name, response.Code, response.Body.String(), unknown.Code, unknown.Body.String())
		}
	}
	if response := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+ready.ID+"?token=forbidden", "", target.NodeToken,
	); response.Code != http.StatusBadRequest {
		t.Fatalf("query credential status=%d body=%q", response.Code, response.Body.String())
	}
	if response := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+ready.ID, "body", target.NodeToken,
	); response.Code != http.StatusBadRequest {
		t.Fatalf("GET body status=%d body=%q", response.Code, response.Body.String())
	}
	if response := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+ready.ID, "", "",
	); response.Code != http.StatusUnauthorized {
		t.Fatalf("missing bearer status=%d body=%q", response.Code, response.Body.String())
	}
	if response := apiRequest(
		harness.mux, http.MethodDelete, "/v1/media/"+ready.ID, "", target.NodeToken,
	); response.Code != http.StatusForbidden {
		t.Fatalf("node DELETE status=%d body=%q", response.Code, response.Body.String())
	}
	deleted := apiRequest(
		harness.mux, http.MethodDelete, "/v1/media/"+ready.ID, "", owner.ControlToken,
	)
	if deleted.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d body=%q", deleted.Code, deleted.Body.String())
	}
	deletedRead := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+ready.ID, "", target.NodeToken,
	)
	if deletedRead.Code != http.StatusNotFound || deletedRead.Body.String() != unknown.Body.String() {
		t.Fatalf("deleted read status=%d body=%q", deletedRead.Code, deletedRead.Body.String())
	}

	createdAt := time.Now().Add(-10 * time.Second).UnixMilli()
	expired := readyDownloadHTTPMedia(
		t, harness, owner, createdAt, createdAt+int64((5*time.Second)/time.Millisecond), []byte("expired"),
	)
	persistHTTPMediaTarget(t, harness, expired, owner, target, createdAt+3)
	expiredRead := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+expired.ID, "", target.NodeToken,
	)
	if expiredRead.Code != http.StatusNotFound || expiredRead.Body.String() != unknown.Body.String() {
		t.Fatalf("expired read status=%d body=%q", expiredRead.Code, expiredRead.Body.String())
	}

	for _, secret := range []string{
		owner.ControlToken, target.NodeToken, nontarget.NodeToken,
		ready.ID, expired.ID, "private-download-title", harness.api.config.MediaDir,
	} {
		if strings.Contains(harness.logs.String(), secret) {
			t.Fatalf("media download logs contain request identity")
		}
	}
}

func TestMediaDownloadHTTPUsesPersistedTransmissionTargetsInProductionWiring(t *testing.T) {
	harness := newOnboardingHarness(t)
	source, err := harness.store.CreateSelfServiceOrbit("Persisted ACL source")
	if err != nil {
		t.Fatal(err)
	}
	target, err := harness.store.CreateSelfServiceOrbit("Persisted ACL target")
	if err != nil {
		t.Fatal(err)
	}
	nontarget, err := harness.store.CreateSelfServiceOrbit("Persisted ACL nontarget")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	payload := []byte("persisted-transmission-target-bytes")
	ready := readyDownloadHTTPMedia(
		t, harness, source, now,
		now+int64((7*24*time.Hour)/time.Millisecond), payload,
	)
	code, err := harness.store.ProposeLink(source.OrbitID, source.ActorID)
	if err != nil {
		t.Fatal(err)
	}
	linkID, _, err := harness.store.AcceptByCode(code, target.OrbitID)
	if err != nil {
		t.Fatal(err)
	}
	if err := harness.store.ActivateLink(linkID); err != nil {
		t.Fatal(err)
	}
	created, err := harness.store.CreateTransmission(store.CreateTransmissionParams{
		MediaID:            ready.ID,
		SourceOrbitID:      source.OrbitID,
		SourceActorID:      source.ActorID,
		SourceSlot:         source.Slot,
		PlaybackDomainKind: store.PlaybackDomainApproach,
		PlaybackDomainID:   linkID,
		AudienceKind:       store.TransmissionAudienceCurrentAir,
		OriginKind:         store.TransmissionOriginFile,
		IncludeOrigin:      false,
		RequestedDelivery:  store.TransmissionDeliveryOverlay,
		EffectiveDelivery:  store.TransmissionDeliveryOverlay,
		AcceptedAt:         now + 3,
		Targets: []store.CreateTransmissionTarget{{
			OrbitID: target.OrbitID, ActorID: target.ActorID, Slot: target.Slot,
			OnlineAtAcceptance: true, MediaClipCapable: true, OverlayCapable: true,
			InterruptCapable: true, InterruptResumeReady: true,
		}},
	})
	if err != nil || created.Transmission.MediaID != ready.ID {
		t.Fatalf("create persisted ACL transmission=%+v err=%v", created, err)
	}
	response := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+ready.ID, "", target.NodeToken,
	)
	if response.Code != http.StatusOK || response.Body.String() != string(payload) {
		t.Fatalf("persisted target status=%d body=%q", response.Code, response.Body.String())
	}
	unknown := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/m_00000000000000000000000000", "", target.NodeToken,
	)
	for name, token := range map[string]string{
		"source without include-origin snapshot": source.NodeToken,
		"copied ID nontarget":                    nontarget.NodeToken,
	} {
		denied := apiRequest(
			harness.mux, http.MethodGet, "/v1/media/"+ready.ID, "", token,
		)
		if denied.Code != http.StatusNotFound || denied.Body.String() != unknown.Body.String() {
			t.Fatalf("%s status=%d body=%q", name, denied.Code, denied.Body.String())
		}
	}
	if err := harness.store.BreakLink(linkID); err != nil {
		t.Fatal(err)
	}
	afterSplit := apiRequest(
		harness.mux, http.MethodGet, "/v1/media/"+ready.ID, "", target.NodeToken,
	)
	if afterSplit.Code != http.StatusOK || afterSplit.Body.String() != string(payload) {
		t.Fatalf("immutable target after split status=%d body=%q",
			afterSplit.Code, afterSplit.Body.String())
	}
}

func TestLegacyMediaHTTPKeepsOnlyNodeApproachCompatibility(t *testing.T) {
	harness := newOnboardingHarness(t)
	owner, err := harness.store.CreateSelfServiceOrbit("Legacy media owner")
	if err != nil {
		t.Fatal(err)
	}
	peer, err := harness.store.CreateSelfServiceOrbit("Legacy media peer")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "legacy-compatible.wav")
	payload := []byte("legacy-compatible-bytes")
	if err := os.WriteFile(path, payload, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := harness.store.InsertMedia(store.MediaRecord{
		ID: "legacy-compat", PathWAV: path, Status: "ready", OrbitID: owner.OrbitID,
		CreatedAt: time.Now().UnixMilli(), ExpiresAt: time.Now().Add(time.Hour).UnixMilli(),
	}); err != nil {
		t.Fatal(err)
	}
	code, err := harness.store.ProposeLink(owner.OrbitID, owner.ActorID)
	if err != nil {
		t.Fatal(err)
	}
	linkID, _, err := harness.store.AcceptByCode(code, peer.OrbitID)
	if err != nil || linkID == 0 {
		t.Fatalf("accept legacy approach=%d err=%v", linkID, err)
	}
	if err := harness.store.ActivateLink(linkID); err != nil {
		t.Fatal(err)
	}
	handler := mediaHandler(harness.store)
	request := func(token string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/media/legacy-compat.wav", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, req)
		return response
	}
	for name, token := range map[string]string{
		"owner_node":  owner.NodeToken,
		"linked_node": peer.NodeToken,
	} {
		response := request(token)
		if response.Code != http.StatusOK || response.Body.String() != string(payload) {
			t.Fatalf("%s status=%d body=%q", name, response.Code, response.Body.String())
		}
	}
	if response := request(owner.ControlToken); response.Code != http.StatusUnauthorized {
		t.Fatalf("legacy control status=%d body=%q", response.Code, response.Body.String())
	}
	if err := harness.store.BreakLink(linkID); err != nil {
		t.Fatal(err)
	}
	if response := request(peer.NodeToken); response.Code != http.StatusNotFound {
		t.Fatalf("legacy unlinked peer status=%d body=%q", response.Code, response.Body.String())
	}
}
