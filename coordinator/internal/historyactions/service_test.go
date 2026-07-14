package historyactions

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"relux.works/duet/coordinator/internal/media"
	"relux.works/duet/coordinator/internal/moderation"
	"relux.works/duet/coordinator/internal/store"
)

type actionFixture struct {
	store   *store.Store
	service *Service
	source  store.OnboardingCredentials
	target  store.OnboardingCredentials
	media   store.MediaItem
	initial store.TransmissionCreation
	now     int64
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func availability(credentials store.OnboardingCredentials, now int64) store.TransmissionTargetAvailability {
	return store.TransmissionTargetAvailability{
		OrbitID: credentials.OrbitID, Slot: credentials.Slot, Connected: true,
		LastSeenAt: now, CredentialTokenHash: digest(credentials.NodeToken),
		MediaClipCapable: true, OverlayCapable: true, InterruptCapable: true,
		MainActive: true, InterruptResumeReady: true,
	}
}

func readyMedia(t *testing.T, st *store.Store, owner store.OnboardingCredentials, now int64) store.MediaItem {
	t.Helper()
	item, err := st.CreateMediaItem(store.CreateMediaItemParams{
		OwnerOrbitID: owner.OrbitID, ActorID: owner.ActorID,
		Kind: store.MediaKindAudioClip, Source: store.MediaSourceApp,
		Title: "history-action.wav", CreatedAt: now,
		ExpiresAt: now + int64((7*24*time.Hour)/time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := st.StageMediaPublication(item.ID, item.Revision, now+1)
	if err != nil {
		t.Fatal(err)
	}
	item, err = st.CompleteMediaPublication(operation.ID, operation.Revision, store.MediaPublication{
		MIME: "audio/wav", Codec: "pcm_s16le", DurationMS: 1000,
		SizeBytes: 176444, SHA256: strings.Repeat("a", 64),
		LoudnessJSON: `{"input_i":"-20.0","output_i":"-14.0"}`,
	}, now+2)
	if err != nil {
		t.Fatal(err)
	}
	return item
}

func newActionFixture(t *testing.T) actionFixture {
	t.Helper()
	root := t.TempDir()
	st, err := store.OpenWithOptions(filepath.Join(root, "history-actions.db"),
		store.Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	source, err := st.CreateSelfServiceOrbit("History action source")
	if err != nil {
		t.Fatal(err)
	}
	target, err := st.CreateSelfServiceOrbit("History action target")
	if err != nil {
		t.Fatal(err)
	}
	code, err := st.ProposeLink(source.OrbitID, source.ActorID)
	if err != nil {
		t.Fatal(err)
	}
	linkID, _, err := st.AcceptByCode(code, target.OrbitID)
	if err != nil {
		t.Fatal(err)
	}
	if err := st.ActivateLink(linkID); err != nil {
		t.Fatal(err)
	}
	lifecycle, err := media.NewLifecycleService(st, root)
	if err != nil {
		t.Fatal(err)
	}
	download, err := media.NewDownloadService(st, root)
	if err != nil {
		t.Fatal(err)
	}
	moderationService, err := moderation.NewService(st, lifecycle, download, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	service, err := NewService(st, lifecycle, moderationService)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	item := readyMedia(t, st, source, now)
	created, err := st.CreateResolvedTransmission(store.CreateResolvedTransmissionParams{
		ExpectedActorID: source.ActorID, Bearer: source.ControlToken,
		IdempotencyKeyHash: digest("initial-key"), RequestHash: digest("initial-request"),
		MediaID: item.ID, AudienceKind: store.TransmissionAudienceCurrentAir,
		OriginKind: store.TransmissionOriginFile, IncludeOrigin: true,
		RequestedDelivery: store.TransmissionDeliveryAfterCurrent, AcceptedAt: now + 3,
		Availability: []store.TransmissionTargetAvailability{
			availability(source, now+3), availability(target, now+3),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return actionFixture{store: st, service: service, source: source, target: target,
		media: item, initial: created.Creation, now: now + 10}
}

func actor(credentials store.OnboardingCredentials) Actor {
	return Actor{ExpectedActorID: credentials.ActorID,
		Identity: store.Identity{Kind: store.IdentityBearer, Token: credentials.ControlToken}}
}

func (fixture actionFixture) historyID() string {
	return "hi_" + strings.TrimPrefix(fixture.initial.Transmission.ID, "tr_")
}

func (fixture actionFixture) replayParams(key, request string, now int64) ReplayParams {
	return ReplayParams{
		Actor: actor(fixture.source), HistoryItemID: fixture.historyID(),
		IdempotencyKeyHash: digest(key), RequestHash: digest(request),
		AudienceKind: store.TransmissionAudienceThisPulsar,
		OriginKind:   store.TransmissionOriginFile, IncludeOrigin: true,
		RequestedDelivery: store.TransmissionDeliveryAfterCurrent, AcceptedAt: now,
		Availability: []store.TransmissionTargetAvailability{availability(fixture.source, now)},
	}
}

func TestReplayCreatesFreshAcceptanceTargetsAndIdempotentResult(t *testing.T) {
	fixture := newActionFixture(t)
	params := fixture.replayParams("replay-key", "replay-request", fixture.now)
	created, err := fixture.service.Replay(params)
	if err != nil {
		t.Fatal(err)
	}
	if created.Reused || created.Creation.Transmission.ID == fixture.initial.Transmission.ID ||
		created.Creation.Transmission.AcceptedAt != fixture.now ||
		created.Creation.Transmission.MediaID != fixture.media.ID || len(created.Creation.Targets) != 1 ||
		created.Creation.Targets[0].OrbitID != fixture.source.OrbitID {
		t.Fatalf("fresh replay=%+v", created)
	}
	repeated, err := fixture.service.Replay(params)
	if err != nil || !repeated.Reused || repeated.Creation.Transmission.ID != created.Creation.Transmission.ID {
		t.Fatalf("repeated replay=%+v err=%v", repeated, err)
	}
	conflict := params
	conflict.RequestHash = digest("changed-request")
	if _, err := fixture.service.Replay(conflict); !errors.Is(err, store.ErrTransmissionIdempotencyConflict) {
		t.Fatalf("idempotency conflict=%v", err)
	}
}

func TestDeleteReplayRaceNeverRevivesMedia(t *testing.T) {
	fixture := newActionFixture(t)
	start := make(chan struct{})
	var wait sync.WaitGroup
	wait.Add(2)
	var replayErr, deleteErr error
	go func() {
		defer wait.Done()
		<-start
		_, replayErr = fixture.service.Replay(
			fixture.replayParams("race-replay", "race-request", fixture.now))
	}()
	go func() {
		defer wait.Done()
		<-start
		_, deleteErr = fixture.service.Delete(actor(fixture.source), fixture.historyID(), fixture.now)
	}()
	close(start)
	wait.Wait()
	if deleteErr != nil {
		t.Fatalf("delete lost race: %v", deleteErr)
	}
	if replayErr != nil && !errors.Is(replayErr, ErrActionUnavailable) &&
		!errors.Is(replayErr, store.ErrTransmissionMediaNotFound) {
		t.Fatalf("unexpected replay race error: %v", replayErr)
	}
	item, err := fixture.store.GetMediaItem(fixture.media.ID)
	if err != nil || item.Status != store.MediaStatusDeleted {
		t.Fatalf("media after race=%+v err=%v", item, err)
	}
	cancellations, err := fixture.store.PendingMediaDeliveryCancellations(10)
	if err != nil || len(cancellations) != 1 || cancellations[0].MediaID != fixture.media.ID ||
		cancellations[0].Reason != store.MediaCancellationDeleted {
		t.Fatalf("delete cancellation outbox=%+v err=%v", cancellations, err)
	}
	if _, err := fixture.service.Replay(
		fixture.replayParams("after-delete", "after-delete-request", fixture.now+1)); !errors.Is(err, store.ErrTransmissionMediaNotFound) {
		t.Fatalf("deleted media replay=%v", err)
	}
	if _, err := fixture.service.Delete(actor(fixture.source), fixture.historyID(), fixture.now+1); err != nil {
		t.Fatalf("repeated delete=%v", err)
	}
}

func TestExpiredMediaCannotBeReplayedOrDeleted(t *testing.T) {
	fixture := newActionFixture(t)
	afterExpiry := fixture.media.ExpiresAt + 1
	params := fixture.replayParams("expired-replay", "expired-request", afterExpiry)
	if _, err := fixture.service.Replay(params); !errors.Is(err, store.ErrTransmissionMediaNotFound) {
		t.Fatalf("expired replay=%v", err)
	}
	if _, err := fixture.service.Delete(actor(fixture.source), fixture.historyID(), afterExpiry); !errors.Is(err, ErrActionUnavailable) {
		t.Fatalf("expired delete=%v", err)
	}
}

func TestReportAndBlockDelegateWithCurrentAuthorizationAndIdempotency(t *testing.T) {
	fixture := newActionFixture(t)
	if _, err := fixture.service.Report(actor(fixture.source), fixture.historyID(),
		store.CreateModerationReportParams{Reason: store.ModerationReasonOther}, fixture.now); !errors.Is(err, ErrActionUnavailable) {
		t.Fatalf("source received self-report action: %v", err)
	}
	target := actor(fixture.target)
	report, err := fixture.service.Report(target, fixture.historyID(), store.CreateModerationReportParams{
		Reason: store.ModerationReasonHarassment, Details: "history action evidence",
	}, fixture.now)
	if err != nil || report.Reused || report.Report.MediaID != fixture.media.ID ||
		report.Report.TransmissionID != fixture.initial.Transmission.ID {
		t.Fatalf("report=%+v err=%v", report, err)
	}
	if count, err := fixture.store.ModerationAuditCount(report.Report.ID, "report.created"); err != nil || count != 1 {
		t.Fatalf("report audit count=%d err=%v", count, err)
	}
	repeated, err := fixture.service.Report(target, fixture.historyID(), store.CreateModerationReportParams{
		Reason: store.ModerationReasonSpam,
	}, fixture.now+1)
	if err != nil || !repeated.Reused || repeated.Report.ID != report.Report.ID {
		t.Fatalf("repeated report=%+v err=%v", repeated, err)
	}
	blockParams := BlockParams{
		Actor: target, HistoryItemID: fixture.historyID(), Kind: BlockActor,
		IdempotencyKeyHash: digest("block-key"), RequestHash: digest("block-request"),
		CreatedAt: fixture.now + 2,
	}
	block, err := fixture.service.Block(blockParams)
	if err != nil || block.Reused || block.SubjectKind != store.BlockedSubjectActor ||
		block.Internal.BlockedActorID != fixture.source.ActorID {
		t.Fatalf("block=%+v err=%v", block, err)
	}
	reused, err := fixture.service.Block(blockParams)
	if err != nil || !reused.Reused || reused.ID != block.ID {
		t.Fatalf("reused block=%+v err=%v", reused, err)
	}
	if _, err := fixture.service.Block(BlockParams{
		Actor: target, HistoryItemID: fixture.historyID(), Kind: BlockOrbit,
		IdempotencyKeyHash: digest("orbit-block-key"), RequestHash: digest("orbit-block-request"),
		CreatedAt: fixture.now + 3,
	}); err != nil {
		t.Fatalf("orbit block: %v", err)
	}
}

func TestRevokedAndTelegramActorsCannotEscalateHistoryActions(t *testing.T) {
	fixture := newActionFixture(t)
	link, err := fixture.store.IssueTelegramLink(
		fixture.source.ActorID, fixture.source.ControlToken, "companion")
	if err != nil {
		t.Fatal(err)
	}
	telegram, err := fixture.store.ConsumeTelegramLink(991_007, "History Telegram", "private", link.Code)
	if err != nil {
		t.Fatal(err)
	}
	telegramActor := Actor{ExpectedActorID: telegram.ActorID,
		Identity: store.Identity{Kind: store.IdentityTelegram, TelegramUserID: 991_007}}
	if _, err := fixture.service.Delete(telegramActor, fixture.historyID(), fixture.now); !errors.Is(err, ErrActionUnavailable) {
		t.Fatalf("foreign Telegram delete=%v", err)
	}
	if _, err := fixture.store.DisableActorForModeration(fixture.source.ActorID, fixture.now); err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.service.Replay(
		fixture.replayParams("revoked-key", "revoked-request", fixture.now+1)); !errors.Is(err, store.ErrUnauthorized) {
		t.Fatalf("revoked replay=%v", err)
	}
}

func TestVerifiedTelegramOwnerUsesSameReplayAndDeleteCommands(t *testing.T) {
	fixture := newActionFixture(t)
	link, err := fixture.store.IssueTelegramLink(
		fixture.source.ActorID, fixture.source.ControlToken, "companion")
	if err != nil {
		t.Fatal(err)
	}
	const telegramUserID = int64(991_008)
	telegram, err := fixture.store.ConsumeTelegramLink(
		telegramUserID, "History Telegram owner", "private", link.Code)
	if err != nil {
		t.Fatal(err)
	}
	createdAt := fixture.now - 100
	item, err := fixture.store.CreateMediaItem(store.CreateMediaItemParams{
		OwnerOrbitID: fixture.source.OrbitID, ActorID: telegram.ActorID,
		Kind: store.MediaKindVoiceClip, Source: store.MediaSourceTelegram,
		Title: "telegram-history.ogg", CreatedAt: createdAt,
		ExpiresAt: fixture.now + int64((7*24*time.Hour)/time.Millisecond),
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := fixture.store.StageMediaPublication(item.ID, item.Revision, createdAt+1)
	if err != nil {
		t.Fatal(err)
	}
	item, err = fixture.store.CompleteMediaPublication(operation.ID, operation.Revision,
		store.MediaPublication{MIME: "audio/ogg", Codec: "opus", DurationMS: 900,
			SizeBytes: 4096, SHA256: strings.Repeat("b", 64),
			LoudnessJSON: `{"input_i":"-19.0","output_i":"-14.0"}`}, createdAt+2)
	if err != nil {
		t.Fatal(err)
	}
	identity := store.Identity{Kind: store.IdentityTelegram, TelegramUserID: telegramUserID}
	accepted := createdAt + 3
	initial, err := fixture.store.CreateResolvedTransmission(store.CreateResolvedTransmissionParams{
		ExpectedActorID: telegram.ActorID, Identity: identity,
		IdempotencyKeyHash: digest("telegram-initial-key"),
		RequestHash:        digest("telegram-initial-request"),
		MediaID:            item.ID, AudienceKind: store.TransmissionAudienceOwnBarycenter,
		OriginKind: store.TransmissionOriginTelegram, IncludeOrigin: true,
		RequestedDelivery: store.TransmissionDeliveryAfterCurrent, AcceptedAt: accepted,
		Availability: []store.TransmissionTargetAvailability{
			availability(fixture.source, accepted),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	historyID := "hi_" + strings.TrimPrefix(initial.Creation.Transmission.ID, "tr_")
	telegramActor := Actor{ExpectedActorID: telegram.ActorID, Identity: identity}
	projected, err := fixture.store.GetAuthorizedHistoryItem(
		telegram.ActorID, identity, historyID, accepted+1)
	if err != nil || !projected.CanReplay || !projected.CanDelete {
		t.Fatalf("Telegram action hints=%+v err=%v", projected, err)
	}
	replayed, err := fixture.service.Replay(ReplayParams{
		Actor: telegramActor, HistoryItemID: historyID,
		IdempotencyKeyHash: digest("telegram-replay-key"),
		RequestHash:        digest("telegram-replay-request"),
		AudienceKind:       store.TransmissionAudienceOwnBarycenter,
		OriginKind:         store.TransmissionOriginTelegram, IncludeOrigin: true,
		RequestedDelivery: store.TransmissionDeliveryAfterCurrent, AcceptedAt: accepted + 1,
		Availability: []store.TransmissionTargetAvailability{
			availability(fixture.source, accepted+1),
		},
	})
	if err != nil || replayed.Creation.Transmission.ID == initial.Creation.Transmission.ID ||
		replayed.Creation.Transmission.SourceActorID != telegram.ActorID {
		t.Fatalf("Telegram replay=%+v err=%v", replayed, err)
	}
	deleted, err := fixture.service.Delete(telegramActor, historyID, accepted+2)
	if err != nil || deleted.Status != store.MediaStatusDeleted {
		t.Fatalf("Telegram delete=%+v err=%v", deleted, err)
	}
}
