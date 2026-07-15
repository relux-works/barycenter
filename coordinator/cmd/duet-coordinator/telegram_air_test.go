package main

import (
	"log/slog"
	"strings"
	"testing"

	"relux.works/duet/coordinator/internal/bot"
	"relux.works/duet/coordinator/internal/config"
	"relux.works/duet/coordinator/internal/store"
)

func telegramAirButton(t *testing.T, prompts []capturedTelegramHistoryPrompt, label string) (string, int64) {
	t.Helper()
	for i := len(prompts) - 1; i >= 0; i-- {
		for _, row := range prompts[i].keyboard {
			for _, button := range row {
				if button.Text == label {
					return button.Data, prompts[i].messageID
				}
			}
		}
	}
	t.Fatalf("Telegram Air button %q not found", label)
	return "", 0
}

func telegramAirCallbackEvent(t *testing.T, userID, messageID int64, queryID, token string, replies *replies,
	answer *bot.CallbackAnswerCode, cleared *bool,
) bot.Event {
	t.Helper()
	ev := telegramBotEvent(t, userID, "private", "", replies)
	ev.MessageID = messageID
	ev.Callback = &bot.CallbackEvent{QueryID: queryID, Data: token,
		Answer:        func(code bot.CallbackAnswerCode) { *answer = code },
		ClearKeyboard: func() { *cleared = true }}
	return ev
}

func TestTelegramAirLifecycleParityAndRedaction(t *testing.T) {
	cfg := &config.Config{SelfServiceOnboarding: true, DBPath: t.TempDir() + "/telegram-air.db", MediaDir: t.TempDir()}
	st, err := store.OpenWithOptions(cfg.DBPath, store.Options{SelfServiceOnboarding: true})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	ownerOrbit, err := st.CreateOrbit("Owner Barycenter", 101)
	if err != nil {
		t.Fatal(err)
	}
	peerOrbit, err := st.CreateOrbit("Peer Barycenter", 202)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CutoverLinksToAirs(1, 100); err != nil {
		t.Fatal(err)
	}
	l := newLoop(slog.Default(), cfg, &fakeSender{}, st, nil, nil)
	l.warmup()
	var prompts []capturedTelegramHistoryPrompt
	messageID := int64(900)
	l.telegramInlinePrompt = func(chatID int64, text string, builder bot.InlineKeyboardBuilder) {
		messageID++
		keyboard, buildErr := builder(messageID)
		if buildErr != nil {
			t.Fatalf("build Telegram Air keyboard: %v", buildErr)
		}
		prompts = append(prompts, capturedTelegramHistoryPrompt{chatID: chatID, messageID: messageID, text: text, keyboard: keyboard})
	}

	groupReplies := &replies{}
	l.handleBot(telegramBotEvent(t, 101, "group", "/air", groupReplies))
	if !strings.Contains(groupReplies.last(t), "личном чате") {
		t.Fatalf("group reply=%q", groupReplies.last(t))
	}

	createReplies := &replies{}
	l.handleBot(telegramBotEvent(t, 101, "private", "/air create Friends", createReplies))
	if !strings.Contains(createReplies.last(t), "Air создан") {
		t.Fatalf("create=%v", createReplies.texts)
	}
	listReplies := &replies{}
	l.handleBot(telegramBotEvent(t, 101, "private", "/air", listReplies))
	if len(prompts) != 1 || !strings.Contains(prompts[0].text, "Friends") {
		t.Fatalf("list prompts=%+v", prompts)
	}
	view, err := st.AuthorizedAirListForIdentity(mustTelegramActor(t, st, 101).ActorID,
		store.Identity{Kind: store.IdentityTelegram, TelegramUserID: 101})
	if err != nil || len(view.Saved) != 1 {
		t.Fatalf("view=%+v err=%v", view, err)
	}
	created := view.Saved[0]
	if strings.Contains(prompts[0].text, created.AirID) || strings.Contains(prompts[0].text, created.MembershipID) {
		t.Fatalf("Air ids leaked into text: %q", prompts[0].text)
	}
	for _, row := range prompts[0].keyboard {
		for _, button := range row {
			if !strings.HasPrefix(button.Data, "tg1_") || strings.Contains(button.Data, created.AirID) {
				t.Fatalf("non-opaque callback: %+v", button)
			}
		}
	}

	issueToken, issueMessage := telegramAirButton(t, prompts, "Invite member / Позвать участника")
	issueReplies := &replies{}
	var issueAnswer bot.CallbackAnswerCode
	issueCleared := false
	l.handleBot(telegramAirCallbackEvent(t, 101, issueMessage, "issue", issueToken, issueReplies, &issueAnswer, &issueCleared))
	if issueAnswer != bot.CallbackApplied || !issueCleared || len(prompts) != 2 {
		t.Fatalf("issue answer=%s cleared=%v prompts=%d", issueAnswer, issueCleared, len(prompts))
	}
	inviteText := issueReplies.last(t)
	if strings.Contains(prompts[1].text, "<code>") || strings.Contains(prompts[1].text, "join ") {
		t.Fatalf("invite secret reached callback prompt: %q", prompts[1].text)
	}
	const prefix, suffix = "<code>/air join ", "</code>"
	start := strings.Index(inviteText, prefix)
	end := strings.Index(inviteText, suffix)
	if start < 0 || end <= start {
		t.Fatalf("invite text=%q", inviteText)
	}
	code := inviteText[start+len(prefix) : end]
	if len(code) != 43 || strings.Contains(issueToken, code) {
		t.Fatalf("invite/callback redaction failed")
	}

	joinReplies := &replies{}
	joinEvent := telegramBotEvent(t, 202, "private", "/air join "+code, joinReplies)
	deleted := false
	joinEvent.DeleteSource = func() { deleted = true }
	l.handleBot(joinEvent)
	if !deleted || len(prompts) != 3 || !strings.Contains(prompts[2].text, "Friends") {
		t.Fatalf("join deleted=%v prompts=%+v replies=%v", deleted, prompts, joinReplies.texts)
	}
	confirmToken, confirmMessage := telegramAirButton(t, prompts, "Join & switch / Подтвердить и включить")

	var foreignAnswer bot.CallbackAnswerCode
	foreignCleared := false
	l.handleBot(telegramAirCallbackEvent(t, 101, confirmMessage, "foreign", confirmToken, &replies{}, &foreignAnswer, &foreignCleared))
	if foreignAnswer != bot.CallbackForbidden {
		t.Fatalf("foreign answer=%s", foreignAnswer)
	}

	var confirmAnswer bot.CallbackAnswerCode
	confirmCleared := false
	l.handleBot(telegramAirCallbackEvent(t, 202, confirmMessage, "confirm", confirmToken, &replies{}, &confirmAnswer, &confirmCleared))
	if confirmAnswer != bot.CallbackApplied || !confirmCleared {
		t.Fatalf("confirm=%s cleared=%v", confirmAnswer, confirmCleared)
	}
	active, _, ok, err := st.ActiveAirForOrbit(peerOrbit.ID)
	if err != nil || !ok || active != created.AirID {
		t.Fatalf("active=%q ok=%v err=%v", active, ok, err)
	}
	peerAfterConfirm, err := st.AuthorizedAirForIdentity(mustTelegramActor(t, st, 202).ActorID,
		store.Identity{Kind: store.IdentityTelegram, TelegramUserID: 202}, created.AirID)
	if err != nil || peerAfterConfirm.MembershipRevision != 2 {
		t.Fatalf("peer projection=%+v err=%v", peerAfterConfirm, err)
	}
	var replayAnswer bot.CallbackAnswerCode
	replayCleared := false
	l.handleBot(telegramAirCallbackEvent(t, 202, confirmMessage, "confirm-repeated", confirmToken, &replies{}, &replayAnswer, &replayCleared))
	if replayAnswer != bot.CallbackAlreadyApplied || !replayCleared {
		t.Fatalf("replay=%s cleared=%v", replayAnswer, replayCleared)
	}
	peerAfterReplay, err := st.AuthorizedAirForIdentity(mustTelegramActor(t, st, 202).ActorID,
		store.Identity{Kind: store.IdentityTelegram, TelegramUserID: 202}, created.AirID)
	if err != nil || peerAfterReplay.MembershipRevision != peerAfterConfirm.MembershipRevision {
		t.Fatalf("replay changed membership before=%d after=%d err=%v", peerAfterConfirm.MembershipRevision, peerAfterReplay.MembershipRevision, err)
	}

	// The owner policy button calls the same canonical replacement service.
	prompts = nil
	l.handleBot(telegramBotEvent(t, 101, "private", "/air", &replies{}))
	policyToken, policyMessage := telegramAirButton(t, prompts, "Next policy preset / Следующая политика")
	var policyAnswer bot.CallbackAnswerCode
	policyCleared := false
	l.handleBot(telegramAirCallbackEvent(t, 101, policyMessage, "policy", policyToken, &replies{}, &policyAnswer, &policyCleared))
	if policyAnswer != bot.CallbackApplied || !policyCleared {
		t.Fatalf("policy=%s cleared=%v", policyAnswer, policyCleared)
	}
	afterPolicy, err := st.AuthorizedAirForIdentity(mustTelegramActor(t, st, 101).ActorID,
		store.Identity{Kind: store.IdentityTelegram, TelegramUserID: 101}, created.AirID)
	if err != nil || afterPolicy.Policy.Revision != created.Policy.Revision+1 {
		t.Fatalf("policy=%+v err=%v", afterPolicy.Policy, err)
	}
	_ = ownerOrbit
}

func mustTelegramActor(t *testing.T, st *store.Store, userID int64) store.ActorContext {
	t.Helper()
	ctx, err := st.ResolveTelegramActorContext(userID)
	if err != nil {
		t.Fatal(err)
	}
	return ctx
}
