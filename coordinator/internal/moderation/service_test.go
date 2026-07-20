package moderation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/media"
	"relux.works/duet/coordinator/internal/store"
)

type cancellationSink struct {
	requests []store.MediaDeliveryCancellation
}

func (sink *cancellationSink) CancelMedia(
	_ context.Context,
	request store.MediaDeliveryCancellation,
) error {
	sink.requests = append(sink.requests, request)
	return nil
}

type serviceFixture struct {
	service  *Service
	store    *store.Store
	source   store.OnboardingCredentials
	reporter store.OnboardingCredentials
	report   store.ModerationReport
	operator store.ModerationOperatorCredential
	media    store.MediaItem
	sink     *cancellationSink
	now      int64
	closed   []store.ModerationNodeIdentity
	notified []store.CancelTransmissionResult
}

func newServiceFixture(t *testing.T) *serviceFixture {
	t.Helper()
	now := time.Now().UnixMilli()
	st, err := store.OpenWithOptions(
		filepath.Join(t.TempDir(), "moderation-service.db"),
		store.Options{SelfServiceOnboarding: true},
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	source, err := st.CreateSelfServiceOrbit("Moderation source")
	if err != nil {
		t.Fatal(err)
	}
	reporter, err := st.CreateSelfServiceOrbit("Moderation reporter")
	if err != nil {
		t.Fatal(err)
	}
	item, err := st.CreateMediaItem(store.CreateMediaItemParams{
		OwnerOrbitID: source.OrbitID, ActorID: source.ActorID,
		Kind: store.MediaKindAudioClip, Source: store.MediaSourceApp,
		Title: "moderation fixture", CreatedAt: now,
		ExpiresAt: now + int64((45*24*time.Hour)/time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := st.StageMediaPublication(item.ID, item.Revision, now+1)
	if err != nil {
		t.Fatal(err)
	}
	mediaDir := t.TempDir()
	canonicalDir := filepath.Join(mediaDir, "canonical")
	if err := os.MkdirAll(canonicalDir, 0o700); err != nil {
		t.Fatal(err)
	}
	path, ok := media.CanonicalPath(canonicalDir, operation.StorageKey)
	if !ok {
		t.Fatal("invalid fixture canonical key")
	}
	content := []byte("moderation-evidence")
	digest := sha256.Sum256(content)
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	ready, err := st.CompleteMediaPublication(
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
	_, err = st.CreateTransmission(store.CreateTransmissionParams{
		MediaID: ready.ID, SourceOrbitID: source.OrbitID,
		SourceActorID: source.ActorID, SourceSlot: source.Slot,
		PlaybackDomainKind: store.PlaybackDomainOrbit,
		PlaybackDomainID:   source.OrbitID,
		AudienceKind:       store.TransmissionAudienceExplicit,
		OriginKind:         store.TransmissionOriginFile, IncludeOrigin: true,
		RequestedDelivery: store.TransmissionDeliveryOverlay,
		EffectiveDelivery: store.TransmissionDeliveryOverlay,
		AcceptedAt:        now + 3,
		Targets: []store.CreateTransmissionTarget{
			{
				OrbitID: reporter.OrbitID, ActorID: reporter.ActorID,
				Slot: reporter.Slot, OnlineAtAcceptance: true,
				MediaClipCapable: true, OverlayCapable: true,
				InterruptCapable: true, InterruptResumeReady: true,
			},
			{
				OrbitID: source.OrbitID, ActorID: source.ActorID,
				Slot: source.Slot, OnlineAtAcceptance: true,
				MediaClipCapable: true, OverlayCapable: true,
				InterruptCapable: true, InterruptResumeReady: true,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	created, err := st.CreateModerationReport(
		reporter.ActorID, reporter.ControlToken,
		store.CreateModerationReportParams{
			MediaID: ready.ID, Reason: store.ModerationReasonHarassment,
			Details: "test report", CreatedAt: now + 4,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	operator, err := st.ProvisionModerationOperator(
		"Service moderator", store.ModerationOperatorCapabilities{
			List: true, Evidence: true, Decide: true,
		}, now+5,
	)
	if err != nil {
		t.Fatal(err)
	}
	lifecycle, err := media.NewLifecycleService(st, mediaDir)
	if err != nil {
		t.Fatal(err)
	}
	sink := new(cancellationSink)
	lifecycle.SetDeliveryCancellationSink(sink)
	download, err := media.NewDownloadService(st, mediaDir)
	if err != nil {
		t.Fatal(err)
	}
	fixture := &serviceFixture{
		store: st, source: source, reporter: reporter,
		report: created.Report, operator: operator, media: ready,
		sink: sink, now: now,
	}
	service, err := NewService(
		st, lifecycle, download,
		func(node store.ModerationNodeIdentity) {
			fixture.closed = append(fixture.closed, node)
		},
		func(result store.CancelTransmissionResult) {
			fixture.notified = append(fixture.notified, result)
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.UnixMilli(now + 10) }
	fixture.service = service
	return fixture
}

func TestServiceEvidenceBlockAndDeleteUseCanonicalServices(t *testing.T) {
	fixture := newServiceFixture(t)
	download, err := fixture.service.OpenEvidence(
		context.Background(), fixture.operator.Operator.ID,
		fixture.operator.Token, fixture.report.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(download.File.Name())
	download.File.Close()
	if err != nil || string(content) != "moderation-evidence" {
		t.Fatalf("evidence=%q err=%v", content, err)
	}
	block, err := fixture.service.BlockReportedSender(
		fixture.reporter.ActorID, fixture.reporter.ControlToken, fixture.report.ID,
	)
	if err != nil || block.Reused {
		t.Fatalf("block=%+v err=%v", block, err)
	}
	if replay, err := fixture.service.BlockReportedSender(
		fixture.reporter.ActorID, fixture.reporter.ControlToken, fixture.report.ID,
	); err != nil || !replay.Reused {
		t.Fatalf("block replay=%+v err=%v", replay, err)
	}
	decision, err := fixture.service.ApplyDecision(
		context.Background(), fixture.operator.Operator.ID,
		fixture.operator.Token, fixture.report.ID,
		store.ModerationActionDeleteMedia,
	)
	if err != nil || decision.State != "applied" {
		t.Fatalf("delete decision=%+v err=%v", decision, err)
	}
	if len(fixture.sink.requests) != 1 {
		t.Fatalf("lifecycle cancellations=%+v", fixture.sink.requests)
	}
	request := fixture.sink.requests[0]
	if request.PolicyVersion != store.MediaLifecyclePolicyV1 ||
		request.ActiveAction != store.MediaActiveActionFadeStop ||
		request.InterruptedMainAction != store.MediaInterruptedMainActionResumeOnce {
		t.Fatalf("delete policy=%+v", request)
	}
	if _, err := fixture.service.ApplyDecision(
		context.Background(), fixture.operator.Operator.ID,
		fixture.operator.Token, fixture.report.ID,
		store.ModerationActionDeleteMedia,
	); err != nil {
		t.Fatalf("delete decision replay=%v", err)
	}
}

func TestServiceE2EEDecisionUsesDormantCanonicalOpaqueDelete(t *testing.T) {
	fixture := newServiceFixture(t)
	authority, err := fixture.store.AirAuthority()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.CutoverLinksToAirs(authority.Generation, fixture.now+20); err != nil {
		t.Fatal(err)
	}
	air, err := fixture.store.CreateAir(store.CreateAirParams{
		Title: "E2EE moderation service", OwnerOrbitID: fixture.source.OrbitID,
		CreatedAt: fixture.now + 21,
	})
	if err != nil {
		t.Fatal(err)
	}
	member, err := fixture.store.AddPendingAirMember(
		air.ID, fixture.reporter.OrbitID, "member", fixture.now+22)
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.store.ConfirmAirMember(member.ID, member.Revision, false, "none", fixture.now+23); err != nil {
		t.Fatal(err)
	}
	register := func(credentials store.OnboardingCredentials, deviceID, protocolActorID string, at int64) {
		payload := []byte("public-package:" + deviceID)
		digest := sha256.Sum256(payload)
		if _, err := fixture.store.RegisterE2EEPublicDevice(store.RegisterE2EEPublicDeviceParams{
			DeviceID: deviceID, ProtocolActorID: protocolActorID,
			ActorID: credentials.ActorID, PublicPackage: payload,
			PublicPackageDigest: hex.EncodeToString(digest[:]), VerificationState: "verified",
			VerificationDigest: strings.Repeat("d", 64), CreatedAt: at,
		}); err != nil {
			t.Fatal(err)
		}
	}
	ownerDevice, peerDevice := "service_e2ee_owner_device_1", "service_e2ee_peer_device_01"
	register(fixture.source, ownerDevice, "service_e2ee_owner_actor_01", fixture.now+24)
	register(fixture.reporter, peerDevice, "service_e2ee_peer_actor_001", fixture.now+25)
	snapshot, err := fixture.store.E2EEAirSnapshot(air.ID)
	if err != nil {
		t.Fatal(err)
	}
	group, err := fixture.store.CreateE2EEGroup(store.CreateE2EEGroupParams{
		AirID: air.ID, AuthorDeviceID: ownerDevice,
		TargetSnapshotDigest: snapshot.Digest, CommitDigest: strings.Repeat("b", 64),
		Epoch: 1, CreatedAt: fixture.now + 26,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.InitializeE2EEGroupRouting(group.ID, ownerDevice, fixture.now+27); err != nil {
		t.Fatal(err)
	}
	chunk := []byte("opaque-service-evidence-ciphertext")
	manifest := []byte("opaque-service-manifest")
	chunkDigest := sha256.Sum256(chunk)
	manifestDigest := sha256.Sum256(manifest)
	object, err := fixture.store.StageE2EEProtectedObject(store.StageE2EEProtectedObjectParams{
		GroupID: group.ID, SourceObjectID: "service_e2ee_source_0001", ObjectKind: "clip",
		AuthorDeviceID: ownerDevice, Epoch: group.CurrentEpoch, Generation: 1,
		TargetSnapshotDigest: group.TargetSnapshotDigest,
		ManifestDigest:       hex.EncodeToString(manifestDigest[:]), EncryptedManifest: manifest,
		OpaqueKeyEnvelopes: []byte("opaque-service-envelopes"),
		CiphertextRef:      "ciphertext/v1/" + hex.EncodeToString(chunkDigest[:]),
		CiphertextDigest:   hex.EncodeToString(chunkDigest[:]), CiphertextSize: int64(len(chunk)),
		ChunkCount: 1, CreatedAt: fixture.now + 28,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.store.PutE2EEProtectedChunk(store.PutE2EEProtectedChunkParams{
		ProtectedObjectID: object.ID, AuthorDeviceID: ownerDevice,
		CiphertextDigest: hex.EncodeToString(chunkDigest[:]), ChunkIndex: 0,
		ByteOffset: 0, Ciphertext: chunk, CreatedAt: fixture.now + 29,
	}); err != nil {
		t.Fatal(err)
	}
	ready, err := fixture.store.FinalizeE2EEProtectedObject(object.ID, object.Revision, fixture.now+30)
	if err != nil {
		t.Fatal(err)
	}
	created, err := fixture.store.CreateE2EEModerationReport(store.CreateE2EEModerationReportParams{
		ProtectedObjectID: ready.ID, ReporterActorID: fixture.reporter.ActorID,
		ReporterDeviceID: peerDevice, Reason: store.ModerationReasonIllegal,
		CreatedAt: fixture.now + 31,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.service.now = func() time.Time { return time.UnixMilli(fixture.now + 40) }
	decision, err := fixture.service.ApplyE2EEDecision(
		context.Background(), fixture.operator.Operator.ID, fixture.operator.Token,
		created.Report.ID, store.ModerationActionDeleteMedia,
	)
	if err != nil || decision.State != "applied" {
		t.Fatalf("E2EE delete decision=%+v err=%v", decision, err)
	}
	deleted, err := fixture.store.GetE2EEProtectedObject(ready.ID)
	if err != nil || deleted.Status != "deleted" {
		t.Fatalf("deleted E2EE object=%+v err=%v", deleted, err)
	}
	if replay, err := fixture.service.ApplyE2EEDecision(
		context.Background(), fixture.operator.Operator.ID, fixture.operator.Token,
		created.Report.ID, store.ModerationActionDeleteMedia,
	); err != nil || replay.ID != decision.ID {
		t.Fatalf("E2EE decision replay=%+v err=%v", replay, err)
	}
}

func TestServiceReportDisarmsOnlyReporterAndRetryIsIdempotent(t *testing.T) {
	fixture := newServiceFixture(t)
	created, err := fixture.service.CreateReport(
		fixture.reporter.ActorID, fixture.reporter.ControlToken,
		store.CreateModerationReportParams{
			MediaID: fixture.media.ID, Reason: store.ModerationReasonHarassment,
		},
	)
	if err != nil || !created.Reused || created.Report.ID != fixture.report.ID {
		t.Fatalf("report retry=%+v err=%v", created, err)
	}
	targets, err := fixture.store.TransmissionTargets(fixture.report.TransmissionID)
	if err != nil {
		t.Fatal(err)
	}
	byActor := make(map[int64]store.TransmissionTarget)
	for _, target := range targets {
		byActor[target.ActorID] = target
	}
	if target := byActor[fixture.reporter.ActorID]; target.Status != store.TransmissionTargetCancelled ||
		target.ReasonCode != store.TransmissionReasonReported {
		t.Fatalf("reporter target=%+v", target)
	}
	if target := byActor[fixture.source.ActorID]; target.Status != store.TransmissionTargetAccepted {
		t.Fatalf("source target was censored=%+v", target)
	}
	if len(fixture.notified) != 1 || !fixture.notified[0].Changed {
		t.Fatalf("report cancellation notifications=%+v", fixture.notified)
	}
	if _, err := fixture.service.CreateReport(
		fixture.reporter.ActorID, fixture.reporter.ControlToken,
		store.CreateModerationReportParams{
			MediaID: fixture.media.ID, Reason: store.ModerationReasonSpam,
		},
	); err != nil {
		t.Fatalf("second report retry=%v", err)
	}
	if len(fixture.notified) != 1 {
		t.Fatalf("idempotent retry emitted new cancellation=%+v", fixture.notified)
	}
	item, err := fixture.store.GetMediaItem(fixture.media.ID)
	if err != nil || item == nil || item.Status != store.MediaStatusReady {
		t.Fatalf("report changed global media=%+v err=%v", item, err)
	}
}

func TestServiceDisableActorRevokesFetchCancelsAndDisconnects(t *testing.T) {
	fixture := newServiceFixture(t)
	decision, err := fixture.service.ApplyDecision(
		context.Background(), fixture.operator.Operator.ID,
		fixture.operator.Token, fixture.report.ID,
		store.ModerationActionDisableActor,
	)
	if err != nil || decision.State != "applied" {
		t.Fatalf("disable decision=%+v err=%v", decision, err)
	}
	if len(fixture.closed) != 1 || fixture.closed[0].ActorID != fixture.source.ActorID {
		t.Fatalf("disconnected=%+v", fixture.closed)
	}
	if len(fixture.notified) == 0 {
		t.Fatal("pending source delivery was not sent to runtime cancellation")
	}
	if _, err := fixture.store.ResolveTokenActorContext(fixture.source.ControlToken); !errors.Is(err, store.ErrUnauthorized) {
		t.Fatalf("disabled actor still authorized: %v", err)
	}
}
