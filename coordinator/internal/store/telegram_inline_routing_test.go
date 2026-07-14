package store

import (
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

func readyTelegramRoutingMedia(
	t *testing.T, st *Store, orbitID, telegramUserID, createdAt int64,
) (MediaItem, ActorContext) {
	t.Helper()
	ctx, err := st.ResolveTelegramActorContext(telegramUserID)
	if err != nil {
		t.Fatal(err)
	}
	created, err := st.CreateTelegramMedia(CreateTelegramMediaParams{
		OwnerOrbitID: orbitID, TelegramUserID: telegramUserID,
		TelegramFileID: "telegram-file", Title: "Telegram clip",
		CreatedAt: createdAt, ExpiresAt: createdAt + 7*24*time.Hour.Milliseconds(),
	})
	if err != nil {
		t.Fatal(err)
	}
	operation, err := st.StageMediaPublication(created.Media.ID, created.Media.Revision, createdAt+10)
	if err != nil {
		t.Fatal(err)
	}
	ready, err := st.CompleteMediaPublication(operation.ID, operation.Revision,
		MediaPublication{
			MIME: "audio/wav", Codec: "pcm_s16le", DurationMS: 4_000,
			SizeBytes: 4_096, SHA256: strings.Repeat("a", 64), LoudnessJSON: `{}`,
		}, createdAt+11)
	if err != nil {
		t.Fatal(err)
	}
	return ready, ctx
}

func telegramRoutingFixture(t *testing.T) (*Store, OnboardingCredentials, int64) {
	t.Helper()
	st, owner := newMediaIngestTestStore(t)
	const telegramUserID = int64(7600712)
	if err := st.AddMember(owner.OrbitID, telegramUserID, "companion"); err != nil {
		t.Fatal(err)
	}
	return st, owner, telegramUserID
}

func registerTelegramVoiceRoute(
	t *testing.T, st *Store, owner OnboardingCredentials, telegramUserID, now int64,
	interrupt bool,
) (RegisterTelegramInlineRouteResult, TransmissionTargetAvailability) {
	t.Helper()
	media, _ := readyTelegramRoutingMedia(t, st, owner.OrbitID, telegramUserID, now)
	availability := fullTransmissionAvailability(owner, media.PublishedAt)
	availability.InterruptCapable = interrupt
	availability.InterruptResumeReady = interrupt
	registered, err := st.RegisterTelegramInlineRoute(RegisterTelegramInlineRouteParams{
		TelegramUserID: telegramUserID, MediaID: media.ID,
		OriginalUpdateID: now, AttachmentKind: "voice", AcceptedAt: now,
		AudienceKind: TransmissionAudienceOwnBarycenter, IncludeOrigin: true,
		Availability: []TransmissionTargetAvailability{availability},
	})
	if err != nil {
		t.Fatal(err)
	}
	if registered.Creation == nil ||
		registered.Creation.Transmission.AcceptedAt != now ||
		registered.Creation.Transmission.EffectiveDelivery != TransmissionDeliveryAfterCurrent ||
		registered.Route.DefaultTransmissionID != registered.Creation.Transmission.ID {
		t.Fatalf("registered route=%+v creation=%+v", registered.Route, registered.Creation)
	}
	return registered, availability
}

func mintTelegramChoice(
	t *testing.T, st *Store, route TelegramInlineRoute, telegramUserID int64,
	action TelegramInlineAction, delivery TransmissionDelivery,
	audience TransmissionAudienceKind, confirmation string, now int64,
) string {
	t.Helper()
	token, err := st.MintTelegramInlineCallback(MintTelegramInlineCallbackParams{
		TelegramUserID: telegramUserID, MediaID: route.MediaID,
		MediaGeneration: route.MediaGeneration, ChatID: 7001, MessageID: 8001,
		Action: action, Delivery: delivery, Audience: audience,
		ConfirmationTokenHash: confirmation, Now: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 36 || !strings.HasPrefix(token, "tg1_") ||
		strings.Contains(token, route.MediaID) {
		t.Fatalf("callback token leaked binding: %q", token)
	}
	return token
}

func applyTelegramChoice(
	t *testing.T, st *Store, telegramUserID int64, query, token string,
	availability TransmissionTargetAvailability, now int64,
) ApplyTelegramInlineCallbackResult {
	t.Helper()
	result, err := st.ApplyTelegramInlineCallback(ApplyTelegramInlineCallbackParams{
		TelegramUserID: telegramUserID, QueryID: query, Token: token,
		ChatID: 7001, MessageID: 8001, Now: now,
		Availability: []TransmissionTargetAvailability{availability},
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestTelegramInlineChoiceAtomicallyReplacesDefaultAndDeduplicates(t *testing.T) {
	st, owner, telegramUserID := telegramRoutingFixture(t)
	now := time.Now().UnixMilli()
	registered, availability := registerTelegramVoiceRoute(t, st, owner, telegramUserID, now, true)
	token := mintTelegramChoice(t, st, registered.Route, telegramUserID,
		TelegramChooseOverlay, TransmissionDeliveryOverlay,
		TransmissionAudienceOwnBarycenter, "", now+20)
	result := applyTelegramChoice(t, st, telegramUserID, "query-one", token, availability, now+21)
	if result.Outcome != TelegramCallbackApplied || result.Creation == nil ||
		result.Cancellation == nil || !result.ClearKeyboard ||
		result.Creation.Transmission.EffectiveDelivery != TransmissionDeliveryOverlay ||
		result.Cancellation.Transmission.ID != registered.Route.DefaultTransmissionID {
		t.Fatalf("replacement result=%+v", result)
	}
	defaultTx, err := st.GetTransmission(registered.Route.DefaultTransmissionID)
	if err != nil || defaultTx.CancellationCause != TransmissionReasonSenderCancelled {
		t.Fatalf("default transmission=%+v err=%v", defaultTx, err)
	}
	replay := applyTelegramChoice(t, st, telegramUserID, "query-one", token, availability, now+22)
	if replay.Outcome != TelegramCallbackApplied || !replay.Replay {
		t.Fatalf("query replay=%+v", replay)
	}
	second := applyTelegramChoice(t, st, telegramUserID, "query-two", token, availability, now+23)
	if second.Outcome != TelegramCallbackAlreadyApplied || second.Creation != nil {
		t.Fatalf("token replay=%+v", second)
	}
	var plaintext, hash string
	if err := st.db.QueryRow(`SELECT token_hash, token_hash FROM telegram_inline_callbacks LIMIT 1`).Scan(
		&hash, &plaintext); err != nil || len(hash) != 64 || plaintext == token {
		t.Fatalf("stored callback hash=%q plaintext=%q err=%v", hash, plaintext, err)
	}
}

func TestTelegramInlineFileClipHasNoDefaultUntilExplicitChoice(t *testing.T) {
	st, owner, telegramUserID := telegramRoutingFixture(t)
	now := time.Now().UnixMilli()
	media, _ := readyTelegramRoutingMedia(t, st, owner.OrbitID, telegramUserID, now)
	availability := fullTransmissionAvailability(owner, media.PublishedAt)
	registered, err := st.RegisterTelegramInlineRoute(RegisterTelegramInlineRouteParams{
		TelegramUserID: telegramUserID, MediaID: media.ID,
		OriginalUpdateID: now, AttachmentKind: "audio", AcceptedAt: now,
		Availability: []TransmissionTargetAvailability{availability},
	})
	if err != nil || registered.Creation != nil || registered.Route.DefaultTransmissionID != "" {
		t.Fatalf("file route=%+v err=%v", registered, err)
	}
	var before int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM transmissions WHERE media_id = ?`, media.ID).Scan(&before); err != nil || before != 0 {
		t.Fatalf("hidden file defaults=%d err=%v", before, err)
	}
	token := mintTelegramChoice(t, st, registered.Route, telegramUserID,
		TelegramChooseAfterCurrent, TransmissionDeliveryAfterCurrent,
		TransmissionAudienceOwnBarycenter, "", now+20)
	selected := applyTelegramChoice(t, st, telegramUserID,
		"file-explicit", token, availability, now+21)
	if selected.Outcome != TelegramCallbackApplied || selected.Creation == nil ||
		selected.Cancellation != nil {
		t.Fatalf("explicit file choice=%+v", selected)
	}
}

func TestTelegramInlineStartWinsWithoutReplacement(t *testing.T) {
	st, owner, telegramUserID := telegramRoutingFixture(t)
	now := time.Now().UnixMilli()
	registered, availability := registerTelegramVoiceRoute(t, st, owner, telegramUserID, now, true)
	claimed, err := st.ClaimLegacyTransmission(
		registered.Route.DefaultTransmissionID, registered.Route.DefaultTransmissionID, now+20,
	)
	if err != nil || len(claimed.Targets) != 1 {
		t.Fatalf("claim=%+v err=%v", claimed, err)
	}
	target := claimed.Targets[0]
	if _, err := st.TransitionTransmissionTarget(TransitionTransmissionTargetParams{
		TransmissionID: target.TransmissionID, OrbitID: target.OrbitID,
		ActorID: target.ActorID, Slot: target.Slot, ExpectedRevision: target.Revision,
		Generation: target.Generation, Status: TransmissionTargetPlaying,
		OccurredAt: now + 21,
	}); err != nil {
		t.Fatal(err)
	}
	token := mintTelegramChoice(t, st, registered.Route, telegramUserID,
		TelegramChooseOverlay, TransmissionDeliveryOverlay,
		TransmissionAudienceCurrentAir, "", now+22)
	result := applyTelegramChoice(t, st, telegramUserID, "start-wins", token, availability, now+23)
	if result.Outcome != TelegramCallbackTooLate || result.Creation != nil ||
		result.Cancellation != nil || !result.ClearKeyboard {
		t.Fatalf("start race result=%+v", result)
	}
	var replacements int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM transmissions WHERE media_id = ?`,
		registered.Route.MediaID).Scan(&replacements); err != nil || replacements != 1 {
		t.Fatalf("transmissions=%d err=%v", replacements, err)
	}
}

func TestTelegramInlineConcurrentChoicesProduceOneReplacement(t *testing.T) {
	st, owner, telegramUserID := telegramRoutingFixture(t)
	now := time.Now().UnixMilli()
	registered, availability := registerTelegramVoiceRoute(t, st, owner, telegramUserID, now, true)
	tokens := []string{
		mintTelegramChoice(t, st, registered.Route, telegramUserID,
			TelegramChooseOverlay, TransmissionDeliveryOverlay,
			TransmissionAudienceOwnBarycenter, "", now+20),
		mintTelegramChoice(t, st, registered.Route, telegramUserID,
			TelegramChooseAfterCurrent, TransmissionDeliveryAfterCurrent,
			TransmissionAudienceCurrentAir, "", now+20),
	}
	start := make(chan struct{})
	results := make(chan ApplyTelegramInlineCallbackResult, 2)
	errorsCh := make(chan error, 2)
	var wait sync.WaitGroup
	for i, token := range tokens {
		wait.Add(1)
		go func(i int, token string) {
			defer wait.Done()
			<-start
			result, err := st.ApplyTelegramInlineCallback(ApplyTelegramInlineCallbackParams{
				TelegramUserID: telegramUserID, QueryID: "race-" + string(rune('a'+i)),
				Token: token, ChatID: 7001, MessageID: 8001, Now: now + 21,
				Availability: []TransmissionTargetAvailability{availability},
			})
			if err != nil {
				errorsCh <- err
				return
			}
			results <- result
		}(i, token)
	}
	close(start)
	wait.Wait()
	close(results)
	close(errorsCh)
	for err := range errorsCh {
		t.Fatal(err)
	}
	applied, already := 0, 0
	for result := range results {
		switch result.Outcome {
		case TelegramCallbackApplied:
			applied++
		case TelegramCallbackAlreadyApplied:
			already++
		default:
			t.Fatalf("race outcome=%+v", result)
		}
	}
	if applied != 1 || already != 1 {
		t.Fatalf("race applied=%d already=%d", applied, already)
	}
	var transmissions int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM transmissions WHERE media_id = ?`,
		registered.Route.MediaID).Scan(&transmissions); err != nil || transmissions != 2 {
		t.Fatalf("transmissions=%d err=%v", transmissions, err)
	}
}

func TestTelegramInlineFaultsRollBackDefaultAndReplacementTransactions(t *testing.T) {
	st, owner, telegramUserID := telegramRoutingFixture(t)
	now := time.Now().UnixMilli()
	media, _ := readyTelegramRoutingMedia(t, st, owner.OrbitID, telegramUserID, now)
	availability := fullTransmissionAvailability(owner, media.PublishedAt)
	injected := errors.New("injected Telegram route commit failure")
	st.testCheckpoint = func(name string) error {
		if name == "telegram_inline_route_before_commit" {
			return injected
		}
		return nil
	}
	params := RegisterTelegramInlineRouteParams{
		TelegramUserID: telegramUserID, MediaID: media.ID,
		OriginalUpdateID: now, AttachmentKind: "voice", AcceptedAt: now,
		AudienceKind: TransmissionAudienceOwnBarycenter, IncludeOrigin: true,
		Availability: []TransmissionTargetAvailability{availability},
	}
	if _, err := st.RegisterTelegramInlineRoute(params); !errors.Is(err, injected) {
		t.Fatalf("route fault err=%v", err)
	}
	var routes, transmissions int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM telegram_inline_routes WHERE media_id = ?`,
		media.ID).Scan(&routes); err != nil {
		t.Fatal(err)
	}
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM transmissions WHERE media_id = ?`,
		media.ID).Scan(&transmissions); err != nil || routes != 0 || transmissions != 0 {
		t.Fatalf("route rollback routes=%d transmissions=%d err=%v", routes, transmissions, err)
	}
	st.testCheckpoint = nil
	registered, err := st.RegisterTelegramInlineRoute(params)
	if err != nil {
		t.Fatal(err)
	}
	token := mintTelegramChoice(t, st, registered.Route, telegramUserID,
		TelegramChooseOverlay, TransmissionDeliveryOverlay,
		TransmissionAudienceOwnBarycenter, "", now+20)
	st.testCheckpoint = func(name string) error {
		if name == "telegram_inline_replace_before_commit" {
			return injected
		}
		return nil
	}
	_, err = st.ApplyTelegramInlineCallback(ApplyTelegramInlineCallbackParams{
		TelegramUserID: telegramUserID, QueryID: "faulted-replace", Token: token,
		ChatID: 7001, MessageID: 8001, Now: now + 21,
		Availability: []TransmissionTargetAvailability{availability},
	})
	if !errors.Is(err, injected) {
		t.Fatalf("replacement fault err=%v", err)
	}
	defaultTx, err := st.GetTransmission(registered.Route.DefaultTransmissionID)
	if err != nil || defaultTx.CancellationCause != "" {
		t.Fatalf("default changed after rollback: %+v err=%v", defaultTx, err)
	}
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM transmissions WHERE media_id = ?`,
		media.ID).Scan(&transmissions); err != nil || transmissions != 1 {
		t.Fatalf("replacement rollback transmissions=%d err=%v", transmissions, err)
	}
}

func TestTelegramInlineInterruptRequiresExplicitFallback(t *testing.T) {
	st, owner, telegramUserID := telegramRoutingFixture(t)
	now := time.Now().UnixMilli()
	registered, availability := registerTelegramVoiceRoute(t, st, owner, telegramUserID, now, false)
	interrupt := mintTelegramChoice(t, st, registered.Route, telegramUserID,
		TelegramChooseInterrupt, TransmissionDeliveryInterrupt,
		TransmissionAudienceOwnBarycenter, "", now+20)
	challenge := applyTelegramChoice(t, st, telegramUserID,
		"interrupt", interrupt, availability, now+21)
	if challenge.Outcome != TelegramCallbackRequiresConfirmation || challenge.Challenge == nil ||
		challenge.ConfirmationTokenHash == "" || challenge.Creation != nil ||
		challenge.Cancellation != nil {
		t.Fatalf("interrupt challenge=%+v", challenge)
	}
	replayedChallenge := applyTelegramChoice(t, st, telegramUserID,
		"interrupt", interrupt, availability, now+22)
	if !replayedChallenge.Replay || replayedChallenge.Challenge == nil ||
		replayedChallenge.ConfirmationTokenHash != challenge.ConfirmationTokenHash {
		t.Fatalf("durable challenge replay=%+v", replayedChallenge)
	}
	defaultTx, err := st.GetTransmission(registered.Route.DefaultTransmissionID)
	if err != nil || defaultTx.CancellationCause != "" {
		t.Fatalf("default changed before confirmation: %+v err=%v", defaultTx, err)
	}
	confirm := mintTelegramChoice(t, st, registered.Route, telegramUserID,
		TelegramConfirmAfter, TransmissionDeliveryAfterCurrent,
		TransmissionAudienceOwnBarycenter, challenge.ConfirmationTokenHash, now+23)
	confirmed := applyTelegramChoice(t, st, telegramUserID,
		"confirm-after", confirm, availability, now+24)
	if confirmed.Outcome != TelegramCallbackApplied || confirmed.Creation == nil ||
		confirmed.Cancellation == nil ||
		confirmed.Creation.Transmission.RequestedDelivery != TransmissionDeliveryInterrupt ||
		confirmed.Creation.Transmission.EffectiveDelivery != TransmissionDeliveryAfterCurrent ||
		confirmed.Creation.Transmission.DowngradeReason != TransmissionDowngradeConfirmedAfterCurrent {
		t.Fatalf("confirmed fallback=%+v", confirmed)
	}
}

func TestTelegramParityMixedCapabilitiesDowngradeAsOneTargetSet(t *testing.T) {
	st, owner, telegramUserID := telegramRoutingFixture(t)
	companion := addTransmissionInstallation(t, st, owner, "companion")
	now := time.Now().UnixMilli()
	media, _ := readyTelegramRoutingMedia(t, st, owner.OrbitID, telegramUserID, now)
	ownerAvailability := fullTransmissionAvailability(owner, media.PublishedAt)
	companionAvailability := fullTransmissionAvailability(companion, media.PublishedAt)
	companionAvailability.OverlayCapable = false
	availability := []TransmissionTargetAvailability{ownerAvailability, companionAvailability}
	registered, err := st.RegisterTelegramInlineRoute(RegisterTelegramInlineRouteParams{
		TelegramUserID: telegramUserID, MediaID: media.ID,
		OriginalUpdateID: now, AttachmentKind: "voice", AcceptedAt: now,
		AudienceKind: TransmissionAudienceOwnBarycenter, IncludeOrigin: true,
		Availability: availability,
	})
	if err != nil {
		t.Fatal(err)
	}
	token := mintTelegramChoice(t, st, registered.Route, telegramUserID,
		TelegramChooseOverlay, TransmissionDeliveryOverlay,
		TransmissionAudienceOwnBarycenter, "", now+20)
	result, err := st.ApplyTelegramInlineCallback(ApplyTelegramInlineCallbackParams{
		TelegramUserID: telegramUserID, QueryID: "mixed-capabilities", Token: token,
		ChatID: 7001, MessageID: 8001, Now: now + 21, Availability: availability,
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Outcome != TelegramCallbackApplied || result.Creation == nil ||
		result.Creation.Transmission.RequestedDelivery != TransmissionDeliveryOverlay ||
		result.Creation.Transmission.EffectiveDelivery != TransmissionDeliveryAfterCurrent ||
		result.Creation.Transmission.DowngradeReason != TransmissionDowngradeMissingOverlay ||
		len(result.Creation.Targets) != 2 {
		t.Fatalf("mixed capability replacement=%+v", result)
	}
	for _, target := range result.Creation.Targets {
		if target.Status != TransmissionTargetAccepted {
			t.Fatalf("mixed target set did not share one schedulable state: %+v",
				result.Creation.Targets)
		}
	}
}

func TestTelegramParityCrossUserCallbackAndQueryReplayCannotMutate(t *testing.T) {
	st, owner, telegramUserID := telegramRoutingFixture(t)
	const otherTelegramUserID = int64(7600713)
	if err := st.AddMember(owner.OrbitID, otherTelegramUserID, "companion"); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UnixMilli()
	registered, availability := registerTelegramVoiceRoute(t, st, owner, telegramUserID, now, true)
	token := mintTelegramChoice(t, st, registered.Route, telegramUserID,
		TelegramChooseOverlay, TransmissionDeliveryOverlay,
		TransmissionAudienceOwnBarycenter, "", now+20)
	ownerResult := applyTelegramChoice(t, st, telegramUserID,
		"owner-query", token, availability, now+21)
	if ownerResult.Outcome != TelegramCallbackApplied || ownerResult.Creation == nil {
		t.Fatalf("owner callback=%+v", ownerResult)
	}
	hijack, err := st.ApplyTelegramInlineCallback(ApplyTelegramInlineCallbackParams{
		TelegramUserID: otherTelegramUserID, QueryID: "owner-query", Token: token,
		ChatID: 7001, MessageID: 8001, Now: now + 22,
		Availability: []TransmissionTargetAvailability{availability},
	})
	if err != nil {
		t.Fatal(err)
	}
	if hijack.Outcome != TelegramCallbackForbidden || hijack.Creation != nil ||
		hijack.Cancellation != nil {
		t.Fatalf("cross-user query replay=%+v", hijack)
	}
	var transmissions int
	if err := st.db.QueryRow(`SELECT COUNT(*) FROM transmissions WHERE media_id = ?`,
		registered.Route.MediaID).Scan(&transmissions); err != nil || transmissions != 2 {
		t.Fatalf("cross-user replay transmissions=%d err=%v", transmissions, err)
	}
}
