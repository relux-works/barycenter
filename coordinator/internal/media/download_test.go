package media

import (
	"context"
	"errors"
	"io"
	"os"
	"sync"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/store"
)

type recordingTargetSnapshotReader struct {
	mu     sync.Mutex
	grants map[store.MediaTargetIdentity]bool
	calls  []store.MediaTargetIdentity
}

func (reader *recordingTargetSnapshotReader) AllowsMediaDownload(
	_ context.Context,
	target store.MediaTargetIdentity,
) (bool, error) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	reader.calls = append(reader.calls, target)
	return reader.grants[target], nil
}

func (reader *recordingTargetSnapshotReader) grant(target store.MediaTargetIdentity) {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	reader.grants[target] = true
}

func (reader *recordingTargetSnapshotReader) callCount() int {
	reader.mu.Lock()
	defer reader.mu.Unlock()
	return len(reader.calls)
}

func TestDownloadServiceUsesExactTargetSnapshotAndOwnerControl(t *testing.T) {
	harness := newSubmitHarness(t)
	ready := readyLifecycleFixture(t, harness, "download-service-ready-0001")
	target, err := harness.store.CreateSelfServiceOrbit("Download service target")
	if err != nil {
		t.Fatal(err)
	}
	nontarget, err := harness.store.CreateSelfServiceOrbit("Download service non-target")
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewDownloadService(harness.store, harness.mediaDir)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.UnixMilli(harness.nextMS()) }
	ownerControl, err := harness.store.ResolveTokenActorContext(harness.credentials.ControlToken)
	if err != nil {
		t.Fatal(err)
	}
	ownerDownload, err := service.OpenAuthorized(
		context.Background(), ownerControl, harness.credentials.ControlToken, ready.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	if bytes, err := io.ReadAll(ownerDownload.File); err != nil || int64(len(bytes)) != ready.SizeBytes {
		t.Fatalf("owner bytes=%d err=%v", len(bytes), err)
	}
	ownerDownload.File.Close()

	targetContext, err := harness.store.ResolveTokenActorContext(target.NodeToken)
	if err != nil {
		t.Fatal(err)
	}
	targetIdentity := store.MediaTargetIdentity{
		MediaID: ready.ID, OrbitID: targetContext.OrbitID,
		ActorID: targetContext.ActorID, Slot: targetContext.Slot,
	}
	if _, err := service.OpenAuthorized(
		context.Background(), targetContext, target.NodeToken, ready.ID,
	); !errors.Is(err, store.ErrMediaNotFound) {
		t.Fatalf("nil snapshot reader did not fail closed: %v", err)
	}
	reader := &recordingTargetSnapshotReader{grants: make(map[store.MediaTargetIdentity]bool)}
	service.SetTargetSnapshotReader(reader)
	if _, err := service.OpenAuthorized(
		context.Background(), targetContext, target.NodeToken, ready.ID,
	); !errors.Is(err, store.ErrMediaNotFound) {
		t.Fatalf("unsnapshotted node error=%v", err)
	}
	reader.grant(targetIdentity)
	targetDownload, err := service.OpenAuthorized(
		context.Background(), targetContext, target.NodeToken, ready.ID,
	)
	if err != nil {
		t.Fatal(err)
	}
	targetDownload.File.Close()

	nontargetContext, err := harness.store.ResolveTokenActorContext(nontarget.NodeToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.OpenAuthorized(
		context.Background(), nontargetContext, nontarget.NodeToken, ready.ID,
	); !errors.Is(err, store.ErrMediaNotFound) {
		t.Fatalf("copied media ID error=%v", err)
	}
	foreignControl, err := harness.store.ResolveTokenActorContext(target.ControlToken)
	if err != nil {
		t.Fatal(err)
	}
	callsBeforeControl := reader.callCount()
	if _, err := service.OpenAuthorized(
		context.Background(), foreignControl, target.ControlToken, ready.ID,
	); !errors.Is(err, store.ErrMediaNotFound) {
		t.Fatalf("foreign control error=%v", err)
	}
	if reader.callCount() != callsBeforeControl {
		t.Fatal("foreign control credential was confused with a target node")
	}
}

func TestDownloadServiceRechecksDeleteAfterTargetAuthorization(t *testing.T) {
	harness := newSubmitHarness(t)
	ready := readyLifecycleFixture(t, harness, "download-delete-race-0001")
	target, err := harness.store.CreateSelfServiceOrbit("Download race target")
	if err != nil {
		t.Fatal(err)
	}
	targetContext, err := harness.store.ResolveTokenActorContext(target.NodeToken)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewDownloadService(harness.store, harness.mediaDir)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.UnixMilli(harness.nextMS()) }
	reader := &recordingTargetSnapshotReader{grants: make(map[store.MediaTargetIdentity]bool)}
	reader.grant(store.MediaTargetIdentity{
		MediaID: ready.ID, OrbitID: targetContext.OrbitID,
		ActorID: targetContext.ActorID, Slot: targetContext.Slot,
	})
	service.SetTargetSnapshotReader(reader)
	entered := make(chan struct{})
	release := make(chan struct{})
	service.testAfterAuthorization = func() {
		close(entered)
		<-release
	}
	result := make(chan error, 1)
	go func() {
		download, err := service.OpenAuthorized(
			context.Background(), targetContext, target.NodeToken, ready.ID,
		)
		if err == nil {
			download.File.Close()
		}
		result <- err
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("download did not reach post-authorization boundary")
	}
	if _, err := harness.store.DeleteAuthorizedMedia(
		harness.credentials.ActorID, harness.credentials.ControlToken,
		ready.ID, harness.nextMS(),
	); err != nil {
		t.Fatal(err)
	}
	close(release)
	select {
	case err := <-result:
		if !errors.Is(err, store.ErrMediaNotFound) {
			t.Fatalf("download/delete race error=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("download/delete race did not finish")
	}
}

func TestDownloadServiceRechecksPersistedTargetBlockInsideDescriptorTransaction(t *testing.T) {
	harness := newSubmitHarness(t)
	ready := readyLifecycleFixture(t, harness, "download-persisted-block-race-0001")
	target, err := harness.store.CreateSelfServiceOrbit("Persisted block race target")
	if err != nil {
		t.Fatal(err)
	}
	acceptedAt := harness.nextMS()
	created, err := harness.store.CreateTransmission(store.CreateTransmissionParams{
		MediaID:            ready.ID,
		SourceOrbitID:      harness.credentials.OrbitID,
		SourceActorID:      harness.credentials.ActorID,
		SourceSlot:         harness.credentials.Slot,
		PlaybackDomainKind: store.PlaybackDomainOrbit,
		PlaybackDomainID:   harness.credentials.OrbitID,
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
	if err != nil || created.Transmission.MediaID != ready.ID {
		t.Fatalf("create persisted target=%+v err=%v", created, err)
	}
	targetContext, err := harness.store.ResolveTokenActorContext(target.NodeToken)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewDownloadService(harness.store, harness.mediaDir)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.UnixMilli(harness.nextMS()) }
	service.SetTargetSnapshotReader(harness.store)
	authorized := make(chan struct{})
	release := make(chan struct{})
	service.testAfterAuthorization = func() {
		close(authorized)
		<-release
	}
	result := make(chan error, 1)
	go func() {
		download, err := service.OpenAuthorized(
			context.Background(), targetContext, target.NodeToken, ready.ID,
		)
		if err == nil {
			download.File.Close()
		}
		result <- err
	}()
	select {
	case <-authorized:
	case <-time.After(5 * time.Second):
		t.Fatal("persisted download did not reach post-authorization boundary")
	}
	block, err := harness.store.CreateTransmissionBlock(store.CreateTransmissionBlockParams{
		OwnerScope: store.BlockOwnerActor, OwnerOrbitID: target.OrbitID,
		OwnerActorID: target.ActorID, BlockedKind: store.BlockedSubjectActor,
		BlockedActorID:      harness.credentials.ActorID,
		AuthorizedByActorID: target.ActorID, CreatedAt: harness.nextMS(),
	})
	if err != nil {
		t.Fatal(err)
	}
	close(release)
	select {
	case err := <-result:
		if !errors.Is(err, store.ErrMediaNotFound) {
			t.Fatalf("block/open race error=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("persisted block/open race did not finish")
	}
	service.testAfterAuthorization = nil
	if _, err := harness.store.RevokeTransmissionBlock(
		block.Block.ID, target.ActorID, block.Block.Revision, harness.nextMS(),
	); err != nil {
		t.Fatal(err)
	}
	download, err := service.OpenAuthorized(
		context.Background(), targetContext, target.NodeToken, ready.ID,
	)
	if err != nil {
		t.Fatalf("download after block removal: %v", err)
	}
	download.File.Close()
}

func TestDownloadServiceRechecksReporterLocalRevocationBeforeDescriptorOpen(t *testing.T) {
	harness := newSubmitHarness(t)
	ready := readyLifecycleFixture(t, harness, "download-report-race-0001")
	target, err := harness.store.CreateSelfServiceOrbit("Persisted report race target")
	if err != nil {
		t.Fatal(err)
	}
	acceptedAt := harness.nextMS()
	created, err := harness.store.CreateTransmission(store.CreateTransmissionParams{
		MediaID: ready.ID, SourceOrbitID: harness.credentials.OrbitID,
		SourceActorID: harness.credentials.ActorID, SourceSlot: harness.credentials.Slot,
		PlaybackDomainKind: store.PlaybackDomainOrbit,
		PlaybackDomainID:   harness.credentials.OrbitID,
		AudienceKind:       store.TransmissionAudienceExplicit,
		OriginKind:         store.TransmissionOriginFile, IncludeOrigin: false,
		RequestedDelivery: store.TransmissionDeliveryOverlay,
		EffectiveDelivery: store.TransmissionDeliveryOverlay,
		AcceptedAt:        acceptedAt,
		Targets: []store.CreateTransmissionTarget{{
			OrbitID: target.OrbitID, ActorID: target.ActorID, Slot: target.Slot,
			OnlineAtAcceptance: true, MediaClipCapable: true, OverlayCapable: true,
			InterruptCapable: true, InterruptResumeReady: true,
		}},
	})
	if err != nil || created.Transmission.MediaID != ready.ID {
		t.Fatalf("create report target=%+v err=%v", created, err)
	}
	targetContext, err := harness.store.ResolveTokenActorContext(target.NodeToken)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewDownloadService(harness.store, harness.mediaDir)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.UnixMilli(harness.nextMS()) }
	service.SetTargetSnapshotReader(harness.store)
	authorized := make(chan struct{})
	release := make(chan struct{})
	service.testAfterAuthorization = func() {
		close(authorized)
		<-release
	}
	result := make(chan error, 1)
	go func() {
		download, err := service.OpenAuthorized(
			context.Background(), targetContext, target.NodeToken, ready.ID,
		)
		if err == nil {
			download.File.Close()
		}
		result <- err
	}()
	select {
	case <-authorized:
	case <-time.After(5 * time.Second):
		t.Fatal("persisted download did not reach report race boundary")
	}
	if _, err := harness.store.CreateModerationReport(
		target.ActorID, target.ControlToken,
		store.CreateModerationReportParams{
			MediaID: ready.ID, Reason: store.ModerationReasonHarassment,
			CreatedAt: harness.nextMS(),
		},
	); err != nil {
		t.Fatal(err)
	}
	close(release)
	select {
	case err := <-result:
		if !errors.Is(err, store.ErrMediaNotFound) {
			t.Fatalf("report/open race error=%v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("persisted report/open race did not finish")
	}
	service.testAfterAuthorization = nil
	owner, err := harness.store.ResolveTokenActorContext(harness.credentials.ControlToken)
	if err != nil {
		t.Fatal(err)
	}
	ownerDownload, err := service.OpenAuthorized(
		context.Background(), owner, harness.credentials.ControlToken, ready.ID,
	)
	if err != nil {
		t.Fatalf("report globally revoked owner: %v", err)
	}
	ownerDownload.File.Close()
}

func TestDownloadServicePinsAuthorizationUntilDescriptorOpen(t *testing.T) {
	harness := newSubmitHarness(t)
	ready := readyLifecycleFixture(t, harness, "download-open-revocation-0001")
	service, err := NewDownloadService(harness.store, harness.mediaDir)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.UnixMilli(harness.nextMS()) }
	ownerControl, err := harness.store.ResolveTokenActorContext(harness.credentials.ControlToken)
	if err != nil {
		t.Fatal(err)
	}
	beforeOpen := make(chan struct{})
	releaseOpen := make(chan struct{})
	service.testBeforeOpen = func() {
		close(beforeOpen)
		<-releaseOpen
	}

	type downloadResult struct {
		download MediaDownload
		err      error
	}
	downloadDone := make(chan downloadResult, 1)
	go func() {
		download, err := service.OpenAuthorized(
			context.Background(), ownerControl, harness.credentials.ControlToken, ready.ID,
		)
		downloadDone <- downloadResult{download: download, err: err}
	}()
	select {
	case <-beforeOpen:
	case <-time.After(5 * time.Second):
		t.Fatal("download did not reach transactional pre-open boundary")
	}

	deleteStarted := make(chan struct{})
	deleteDone := make(chan error, 1)
	go func() {
		close(deleteStarted)
		_, err := harness.store.DeleteAuthorizedMedia(
			harness.credentials.ActorID, harness.credentials.ControlToken,
			ready.ID, harness.nextMS(),
		)
		deleteDone <- err
	}()
	<-deleteStarted
	select {
	case err := <-deleteDone:
		t.Fatalf("delete crossed the authorization/open boundary: %v", err)
	case <-time.After(150 * time.Millisecond):
	}
	close(releaseOpen)

	var opened MediaDownload
	select {
	case result := <-downloadDone:
		if result.err != nil {
			t.Fatalf("authorized open failed: %v", result.err)
		}
		opened = result.download
	case <-time.After(5 * time.Second):
		t.Fatal("authorized open did not finish")
	}
	defer opened.File.Close()
	select {
	case err := <-deleteDone:
		if err != nil {
			t.Fatalf("delete after descriptor acquisition: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("delete did not finish after descriptor acquisition")
	}
	if _, err := service.OpenAuthorized(
		context.Background(), ownerControl, harness.credentials.ControlToken, ready.ID,
	); !errors.Is(err, store.ErrMediaNotFound) {
		t.Fatalf("post-delete authorization error=%v", err)
	}
}

func TestDownloadServiceRefusesCanonicalSymlink(t *testing.T) {
	harness := newSubmitHarness(t)
	ready := readyLifecycleFixture(t, harness, "download-symlink-refusal-0001")
	canonicalPath, ok := CanonicalPath(harness.service.canonicalDir, ready.StorageKey)
	if !ok {
		t.Fatal("invalid canonical path")
	}
	original, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatal(err)
	}
	outside := harness.mediaDir + "-download-outside"
	if err := os.WriteFile(outside, original, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(canonicalPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, canonicalPath); err != nil {
		t.Fatal(err)
	}
	service, err := NewDownloadService(harness.store, harness.mediaDir)
	if err != nil {
		t.Fatal(err)
	}
	service.now = func() time.Time { return time.UnixMilli(harness.nextMS()) }
	ownerControl, err := harness.store.ResolveTokenActorContext(harness.credentials.ControlToken)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.OpenAuthorized(
		context.Background(), ownerControl, harness.credentials.ControlToken, ready.ID,
	); err == nil || errors.Is(err, store.ErrMediaNotFound) {
		t.Fatalf("canonical symlink error=%v", err)
	}
	if bytes, err := os.ReadFile(outside); err != nil || string(bytes) != string(original) {
		t.Fatalf("outside bytes changed=%d err=%v", len(bytes), err)
	}
}
