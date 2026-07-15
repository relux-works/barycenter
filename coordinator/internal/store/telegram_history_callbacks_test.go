package store

import (
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type telegramHistoryFixture struct {
	store          *Store
	source         OnboardingCredentials
	recipient      OnboardingCredentials
	telegramUserID int64
	historyItemID  string
	now            int64
}

func newTelegramHistoryFixture(t *testing.T) telegramHistoryFixture {
	t.Helper()
	path := filepath.Join(t.TempDir(), "telegram-history.db")
	st, err := OpenWithOptions(path, Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	source, err := st.CreateSelfServiceOrbit("History callback source")
	if err != nil {
		t.Fatal(err)
	}
	recipient, err := st.CreateSelfServiceOrbit("History callback recipient")
	if err != nil {
		t.Fatal(err)
	}
	activateTransmissionApproach(t, st, source, recipient)
	const telegramUserID = int64(8_800_712)
	link, err := st.IssueTelegramLink(recipient.ActorID, recipient.ControlToken, "companion")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.ConsumeTelegramLink(
		telegramUserID, "History callback primary", "private", link.Code,
	); err != nil {
		t.Fatal(err)
	}
	if err := st.TransferPrimary(recipient.OrbitID, telegramUserID); err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 7, 15, 8, 0, 0, 0, time.UTC).UnixMilli()
	media := readyLifecycleMedia(t, st, source, now-1000, now+7*24*time.Hour.Milliseconds())
	creation := createHistoryTransmission(t, st, source, media, now-900,
		fullTransmissionAvailability(source, now-900),
		fullTransmissionAvailability(recipient, now-900))
	historyItemID := "hi_" + strings.TrimPrefix(creation.Transmission.ID, "tr_")
	ctx, err := st.ResolveTelegramActorContext(telegramUserID)
	if err != nil {
		t.Fatal(err)
	}
	item, err := st.GetAuthorizedHistoryItem(ctx.ActorID,
		Identity{Kind: IdentityTelegram, TelegramUserID: telegramUserID}, historyItemID, now)
	if err != nil || !item.CanReport || !item.CanBlockActor || item.CanDelete {
		t.Fatalf("Telegram foreign history item=%+v err=%v", item, err)
	}
	return telegramHistoryFixture{st, source, recipient, telegramUserID, historyItemID, now}
}

func (fixture telegramHistoryFixture) mint(
	t *testing.T,
	action TelegramHistoryAction,
	reason ModerationReason,
	messageID int64,
) string {
	t.Helper()
	token, err := fixture.store.MintTelegramHistoryCallback(MintTelegramHistoryCallbackParams{
		TelegramUserID: fixture.telegramUserID, HistoryItemID: fixture.historyItemID,
		ChatID: fixture.telegramUserID, MessageID: messageID,
		Action: action, Reason: reason, Now: fixture.now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 36 || !strings.HasPrefix(token, "tg1_") ||
		strings.Contains(token, fixture.historyItemID) {
		t.Fatalf("opaque callback token=%q", token)
	}
	return token
}

func TestTelegramHistoryCallbackBindsActorChatMessageAndExpiry(t *testing.T) {
	fixture := newTelegramHistoryFixture(t)
	token := fixture.mint(t, TelegramHistoryReport, ModerationReasonHarassment, 71)

	forged, err := fixture.store.ClaimTelegramHistoryCallback(ClaimTelegramHistoryCallbackParams{
		TelegramUserID: fixture.telegramUserID, QueryID: "forged-query",
		Token: "tg1_" + strings.Repeat("A", 32), ChatID: fixture.telegramUserID,
		MessageID: 71, Now: fixture.now + 1,
	})
	if err != nil || forged.Found {
		t.Fatalf("forged callback=%+v err=%v", forged, err)
	}

	wrongMessage, err := fixture.store.ClaimTelegramHistoryCallback(ClaimTelegramHistoryCallbackParams{
		TelegramUserID: fixture.telegramUserID, QueryID: "wrong-message-query",
		Token: token, ChatID: fixture.telegramUserID, MessageID: 72, Now: fixture.now + 1,
	})
	if err != nil || !wrongMessage.Found || wrongMessage.Outcome != TelegramCallbackForbidden ||
		wrongMessage.Claim != nil {
		t.Fatalf("wrong message callback=%+v err=%v", wrongMessage, err)
	}

	const otherTelegramUserID = int64(8_800_713)
	if err := fixture.store.AddMember(fixture.recipient.OrbitID, otherTelegramUserID, "companion"); err != nil {
		t.Fatal(err)
	}
	crossUser, err := fixture.store.ClaimTelegramHistoryCallback(ClaimTelegramHistoryCallbackParams{
		TelegramUserID: otherTelegramUserID, QueryID: "cross-user-query", Token: token,
		ChatID: fixture.telegramUserID, MessageID: 71, Now: fixture.now + 1,
	})
	if err != nil || !crossUser.Found || crossUser.Outcome != TelegramCallbackForbidden ||
		crossUser.Claim != nil {
		t.Fatalf("cross-user callback=%+v err=%v", crossUser, err)
	}

	expiredToken := fixture.mint(t, TelegramHistoryBlockActor, "", 73)
	expired, err := fixture.store.ClaimTelegramHistoryCallback(ClaimTelegramHistoryCallbackParams{
		TelegramUserID: fixture.telegramUserID, QueryID: "expired-query", Token: expiredToken,
		ChatID: fixture.telegramUserID, MessageID: 73,
		Now: fixture.now + telegramCallbackTTL.Milliseconds(),
	})
	if err != nil || !expired.Found || expired.Outcome != TelegramCallbackExpired ||
		!expired.ClearKeyboard || expired.Claim != nil {
		t.Fatalf("expired callback=%+v err=%v", expired, err)
	}
}

func TestTelegramHistoryCallbackFinalizationIsReplaySafe(t *testing.T) {
	fixture := newTelegramHistoryFixture(t)
	token := fixture.mint(t, TelegramHistoryReport, ModerationReasonSpam, 81)
	claimParams := ClaimTelegramHistoryCallbackParams{
		TelegramUserID: fixture.telegramUserID, QueryID: "report-query", Token: token,
		ChatID: fixture.telegramUserID, MessageID: 81, Now: fixture.now + 1,
	}
	claimed, err := fixture.store.ClaimTelegramHistoryCallback(claimParams)
	if err != nil || !claimed.Found || claimed.Claim == nil ||
		claimed.Claim.HistoryItemID != fixture.historyItemID ||
		claimed.Claim.Action != TelegramHistoryReport ||
		claimed.Claim.Reason != ModerationReasonSpam {
		t.Fatalf("claimed callback=%+v err=%v", claimed, err)
	}
	final, err := fixture.store.FinalizeTelegramHistoryCallback(
		FinalizeTelegramHistoryCallbackParams{
			TelegramUserID: fixture.telegramUserID, QueryID: claimParams.QueryID,
			Token: token, ChatID: claimParams.ChatID, MessageID: claimParams.MessageID,
			Claim: *claimed.Claim, Outcome: TelegramCallbackApplied,
			ActionOutcome: "report_received", Consume: true, ClearKeyboard: true,
			Now: fixture.now + 2,
		})
	if err != nil || final.Outcome != TelegramCallbackApplied ||
		final.ActionOutcome != "report_received" || !final.ClearKeyboard {
		t.Fatalf("final callback=%+v err=%v", final, err)
	}
	replayedQuery, err := fixture.store.ClaimTelegramHistoryCallback(claimParams)
	if err != nil || !replayedQuery.Replay || replayedQuery.Claim != nil ||
		replayedQuery.Outcome != TelegramCallbackApplied ||
		replayedQuery.ActionOutcome != "report_received" {
		t.Fatalf("query replay=%+v err=%v", replayedQuery, err)
	}
	repeated, err := fixture.store.ClaimTelegramHistoryCallback(ClaimTelegramHistoryCallbackParams{
		TelegramUserID: fixture.telegramUserID, QueryID: "second-report-query", Token: token,
		ChatID: fixture.telegramUserID, MessageID: 81, Now: fixture.now + 3,
	})
	if err != nil || repeated.Outcome != TelegramCallbackAlreadyApplied ||
		repeated.ActionOutcome != "report_received" || !repeated.ClearKeyboard {
		t.Fatalf("consumed callback=%+v err=%v", repeated, err)
	}
}

func TestTelegramHistoryCallbackMintRejectsUnavailableAndInvalidActions(t *testing.T) {
	fixture := newTelegramHistoryFixture(t)
	for _, params := range []MintTelegramHistoryCallbackParams{
		{TelegramUserID: fixture.telegramUserID, HistoryItemID: fixture.historyItemID,
			ChatID: fixture.telegramUserID, MessageID: 91, Action: TelegramHistoryDelete,
			Now: fixture.now},
		{TelegramUserID: fixture.telegramUserID, HistoryItemID: fixture.historyItemID,
			ChatID: fixture.telegramUserID, MessageID: 91, Action: TelegramHistoryReport,
			Reason: "invalid", Now: fixture.now},
	} {
		if token, err := fixture.store.MintTelegramHistoryCallback(params); err == nil || token != "" {
			t.Fatalf("invalid callback minted token=%q err=%v", token, err)
		}
	}
}
