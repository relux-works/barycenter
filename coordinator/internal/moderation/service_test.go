package moderation

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
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
