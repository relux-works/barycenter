package main

import (
	"log/slog"
	"strings"
	"testing"

	"relux.works/duet/coordinator/internal/bot"
	"relux.works/duet/coordinator/internal/presentation"
	"relux.works/duet/coordinator/internal/store"
)

type capturedTelegramHistoryPrompt struct {
	chatID    int64
	messageID int64
	text      string
	keyboard  bot.InlineKeyboard
}

func telegramHistoryButton(t *testing.T, prompts []capturedTelegramHistoryPrompt, label string) string {
	t.Helper()
	for _, prompt := range prompts {
		for _, row := range prompt.keyboard {
			for _, button := range row {
				if button.Text == label {
					return button.Data
				}
			}
		}
	}
	t.Fatalf("Telegram history button %q not found in %+v", label, prompts)
	return ""
}

func TestTelegramHistoryReportAndBlockUseCanonicalServices(t *testing.T) {
	fixture := newModerationHTTPFixture(t)
	const telegramUserID = int64(8_800_714)
	link, err := fixture.harness.store.IssueTelegramLink(
		fixture.reporter.ActorID, fixture.reporter.ControlToken, "companion",
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fixture.harness.store.ConsumeTelegramLink(
		telegramUserID, "Telegram moderation owner", "private", link.Code,
	); err != nil {
		t.Fatal(err)
	}
	if err := fixture.harness.store.TransferPrimary(fixture.reporter.OrbitID, telegramUserID); err != nil {
		t.Fatal(err)
	}

	fake := &fakeSender{}
	l := newLoop(slog.Default(), fixture.harness.api.config, fake, fixture.harness.store, nil, nil)
	l.mediaLifecycle = fixture.harness.api.mediaLifecycle
	l.moderationService = fixture.harness.api.moderationService
	l.warmup()

	groupReplies := &replies{}
	l.handleBot(telegramBotEvent(t, telegramUserID, "group", "/history", groupReplies))
	if got := groupReplies.last(t); !strings.Contains(got, "личном чате") {
		t.Fatalf("group history reply=%q", got)
	}

	var prompts []capturedTelegramHistoryPrompt
	nextMessageID := int64(700)
	l.telegramInlinePrompt = func(chatID int64, text string, builder bot.InlineKeyboardBuilder) {
		nextMessageID++
		keyboard, buildErr := builder(nextMessageID)
		if buildErr != nil {
			t.Fatalf("build Telegram history keyboard: %v", buildErr)
		}
		prompts = append(prompts, capturedTelegramHistoryPrompt{
			chatID: chatID, messageID: nextMessageID, text: text, keyboard: keyboard,
		})
	}

	historyReplies := &replies{}
	l.handleBot(telegramBotEvent(t, telegramUserID, "private", "/history", historyReplies))
	if len(prompts) != 1 || prompts[0].chatID != telegramUserID ||
		strings.Contains(prompts[0].text, fixture.media.ID) ||
		!strings.Contains(prompts[0].text, fixture.media.Title) {
		t.Fatalf("history prompt=%+v replies=%v", prompts, historyReplies.texts)
	}

	reportToken := telegramHistoryButton(t, prompts,
		presentation.HistoryActionLabel("report", store.ModerationReasonHarassment).
			Text(presentation.Russian))
	groupReportToken := telegramHistoryButton(t, prompts,
		presentation.HistoryActionLabel("report", store.ModerationReasonSpam).
			Text(presentation.Russian))
	var reportAnswer bot.CallbackAnswerCode
	reportCleared := false
	reportReplies := &replies{}
	reportEvent := telegramBotEvent(t, telegramUserID, "private", "", reportReplies)
	reportEvent.MessageID = prompts[0].messageID
	reportEvent.Callback = &bot.CallbackEvent{
		QueryID: "telegram-history-report-query", Data: reportToken,
		Answer:        func(code bot.CallbackAnswerCode) { reportAnswer = code },
		ClearKeyboard: func() { reportCleared = true },
	}
	l.handleBot(reportEvent)
	if reportAnswer != bot.CallbackApplied || !reportCleared ||
		reportReplies.last(t) != presentation.HistoryActionOutcomeLabel("report_received").Text(presentation.Russian) {
		t.Fatalf("report answer=%q cleared=%v replies=%v", reportAnswer, reportCleared, reportReplies.texts)
	}
	reports, err := fixture.harness.store.ListModerationReports(
		fixture.operator.Operator.ID, fixture.operator.Token, "open", 10,
	)
	if err != nil || len(reports) != 1 || reports[0].MediaID != fixture.media.ID ||
		reports[0].Reason != store.ModerationReasonHarassment || reports[0].Details != "" ||
		reports[0].TargetActorID != fixture.reporter.ActorID {
		t.Fatalf("canonical Telegram report=%+v err=%v", reports, err)
	}

	// Telegram can retry the same query after a transport timeout. The durable
	// callback result is replayed without creating a second report.
	reportAnswer = ""
	reportCleared = false
	l.handleBot(reportEvent)
	if reportAnswer != bot.CallbackApplied || !reportCleared {
		t.Fatalf("report query replay answer=%q cleared=%v", reportAnswer, reportCleared)
	}
	reports, err = fixture.harness.store.ListModerationReports(
		fixture.operator.Operator.ID, fixture.operator.Token, "open", 10,
	)
	if err != nil || len(reports) != 1 {
		t.Fatalf("report query replay created duplicate: %+v err=%v", reports, err)
	}

	var groupCallbackAnswer bot.CallbackAnswerCode
	groupCallbackCleared := false
	groupCallback := telegramBotEvent(t, telegramUserID, "group", "", &replies{})
	groupCallback.MessageID = prompts[0].messageID
	groupCallback.Callback = &bot.CallbackEvent{
		QueryID: "telegram-history-group-report-query", Data: groupReportToken,
		Answer:        func(code bot.CallbackAnswerCode) { groupCallbackAnswer = code },
		ClearKeyboard: func() { groupCallbackCleared = true },
	}
	l.handleBot(groupCallback)
	if groupCallbackAnswer != bot.CallbackForbidden || groupCallbackCleared {
		t.Fatalf("group callback answer=%q cleared=%v", groupCallbackAnswer, groupCallbackCleared)
	}

	// A group attempt is non-terminal: the same actor can still use the exact
	// capability in its bound private chat, where the canonical report service
	// returns the existing report without changing its first reason.
	var alreadyAnswer bot.CallbackAnswerCode
	alreadyCleared := false
	alreadyReplies := &replies{}
	alreadyEvent := telegramBotEvent(t, telegramUserID, "private", "", alreadyReplies)
	alreadyEvent.MessageID = prompts[0].messageID
	alreadyEvent.Callback = &bot.CallbackEvent{
		QueryID: "telegram-history-private-report-retry", Data: groupReportToken,
		Answer:        func(code bot.CallbackAnswerCode) { alreadyAnswer = code },
		ClearKeyboard: func() { alreadyCleared = true },
	}
	l.handleBot(alreadyEvent)
	if alreadyAnswer != bot.CallbackAlreadyApplied || !alreadyCleared ||
		alreadyReplies.last(t) != presentation.HistoryActionOutcomeLabel("report_already_received").Text(presentation.Russian) {
		t.Fatalf("private retry answer=%q cleared=%v replies=%v", alreadyAnswer, alreadyCleared, alreadyReplies.texts)
	}

	prompts = nil
	l.handleBot(telegramBotEvent(t, telegramUserID, "private", "/history", &replies{}))
	blockToken := telegramHistoryButton(t, prompts,
		presentation.HistoryActionLabel("block_actor", "").Text(presentation.Russian))
	var blockAnswer bot.CallbackAnswerCode
	blockCleared := false
	blockReplies := &replies{}
	blockEvent := telegramBotEvent(t, telegramUserID, "private", "", blockReplies)
	blockEvent.MessageID = prompts[0].messageID
	blockEvent.Callback = &bot.CallbackEvent{
		QueryID: "telegram-history-block-query", Data: blockToken,
		Answer:        func(code bot.CallbackAnswerCode) { blockAnswer = code },
		ClearKeyboard: func() { blockCleared = true },
	}
	l.handleBot(blockEvent)
	if blockAnswer != bot.CallbackApplied || !blockCleared ||
		blockReplies.last(t) != presentation.HistoryActionOutcomeLabel("sender_blocked").Text(presentation.Russian) {
		t.Fatalf("block answer=%q cleared=%v replies=%v", blockAnswer, blockCleared, blockReplies.texts)
	}
	ctx, err := fixture.harness.store.ResolveTelegramActorContext(telegramUserID)
	if err != nil {
		t.Fatal(err)
	}
	blocks, err := fixture.harness.store.AuthorizedListTransmissionBlocksForIdentity(
		ctx.ActorID, store.Identity{Kind: store.IdentityTelegram, TelegramUserID: telegramUserID},
	)
	if err != nil || len(blocks) != 1 || blocks[0].OwnerScope != store.BlockOwnerOrbit ||
		blocks[0].SubjectKind != store.BlockedSubjectActor ||
		blocks[0].Internal.BlockedActorID != fixture.source.ActorID {
		t.Fatalf("canonical Telegram block=%+v err=%v", blocks, err)
	}
}
