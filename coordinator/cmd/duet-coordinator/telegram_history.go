package main

import (
	"errors"
	"fmt"
	"time"

	"relux.works/duet/coordinator/internal/bot"
	"relux.works/duet/coordinator/internal/historyactions"
	"relux.works/duet/coordinator/internal/presentation"
	"relux.works/duet/coordinator/internal/store"
)

var telegramHistoryReasons = []store.ModerationReason{
	store.ModerationReasonSpam,
	store.ModerationReasonHarassment,
	store.ModerationReasonIllegal,
	store.ModerationReasonSexualContent,
	store.ModerationReasonViolence,
	store.ModerationReasonOther,
}

func telegramHistoryItemText(item store.HistoryQueryItem) string {
	status, reason := historyMediaState(item.Media)
	if item.Transmission != nil {
		status, reason = historyTransmissionState(item)
	}
	label := presentation.ReceiptLabel(status, store.TransmissionReason(reason))
	text := fmt.Sprintf("<b>%s</b>\n%s · %s", esc(item.Media.Title),
		esc(presentation.HistoryDirectionLabel(item.Direction).Text(presentation.Russian)),
		esc(label.Text(presentation.Russian)))
	if item.Direction == store.HistoryReceived || item.Direction == store.HistorySentAndReceived {
		text += "\n" + esc(presentation.SenderLabel(item.SourceActorName).Text(presentation.Russian))
	}
	return text
}

func telegramHistoryHasActions(item store.HistoryQueryItem) bool {
	return item.CanReplay || item.CanDelete || item.CanReport || item.CanBlockActor
}

func (l *loop) telegramHistoryKeyboard(
	telegramUserID, chatID, messageID int64,
	item store.HistoryQueryItem,
) (bot.InlineKeyboard, error) {
	now := time.Now().UnixMilli()
	var keyboard bot.InlineKeyboard
	add := func(action store.TelegramHistoryAction, reason store.ModerationReason) error {
		token, err := l.st.MintTelegramHistoryCallback(store.MintTelegramHistoryCallbackParams{
			TelegramUserID: telegramUserID, HistoryItemID: item.HistoryItemID,
			ChatID: chatID, MessageID: messageID, Action: action, Reason: reason, Now: now,
		})
		if err != nil {
			return err
		}
		keyboard = append(keyboard, []bot.InlineButton{{
			Text: presentation.HistoryActionLabel(string(action), reason).Text(presentation.Russian),
			Data: token,
		}})
		return nil
	}
	if item.CanReplay {
		if err := add(store.TelegramHistoryReplay, ""); err != nil {
			return nil, err
		}
	}
	if item.CanDelete {
		if err := add(store.TelegramHistoryDelete, ""); err != nil {
			return nil, err
		}
	}
	if item.CanReport {
		for _, reason := range telegramHistoryReasons {
			if err := add(store.TelegramHistoryReport, reason); err != nil {
				return nil, err
			}
		}
	}
	if item.CanBlockActor {
		if err := add(store.TelegramHistoryBlockActor, ""); err != nil {
			return nil, err
		}
	}
	return keyboard, nil
}

func (l *loop) sendTelegramHistory(ev bot.Event) {
	if ev.ChatType != "private" {
		ev.Reply("Открой /history в личном чате с ботом — групповые кнопки модерации отключены.")
		return
	}
	ctx, err := l.st.ResolveTelegramActorContext(ev.FromUserID)
	if err != nil {
		ev.Reply(presentation.HistoryActionOutcomeLabel("failed").Text(presentation.Russian))
		return
	}
	now := time.Now().UnixMilli()
	page, err := l.st.QueryAuthorizedHistory(ctx.ActorID,
		store.Identity{Kind: store.IdentityTelegram, TelegramUserID: ev.FromUserID},
		"all", 5, "", now)
	if err != nil {
		l.log.Error("query Telegram history", "err", err)
		ev.Reply(presentation.HistoryActionOutcomeLabel("failed").Text(presentation.Russian))
		return
	}
	if len(page.Items) == 0 {
		ev.Reply(presentation.HistoryEmptyLabel().Text(presentation.Russian))
		return
	}
	for _, item := range page.Items {
		item := item
		text := telegramHistoryItemText(item)
		if !telegramHistoryHasActions(item) || l.telegramInlinePrompt == nil {
			ev.Reply(text)
			continue
		}
		l.telegramInlinePrompt(ev.ChatID, text, func(messageID int64) (bot.InlineKeyboard, error) {
			return l.telegramHistoryKeyboard(ev.FromUserID, ev.ChatID, messageID, item)
		})
	}
}

func telegramHistoryCallbackFailure(err error) (store.TelegramCallbackOutcome, string, bool) {
	switch {
	case errors.Is(err, historyactions.ErrActionUnavailable),
		errors.Is(err, store.ErrMediaNotFound),
		errors.Is(err, store.ErrMediaStateConflict),
		errors.Is(err, store.ErrTransmissionStateConflict),
		errors.Is(err, store.ErrTransmissionNotFound):
		return store.TelegramCallbackTooLate, "history_action_unavailable", true
	case errors.Is(err, store.ErrUnauthorized),
		errors.Is(err, store.ErrInsufficientCapability),
		errors.Is(err, store.ErrTransmissionPolicyForbidden),
		errors.Is(err, store.ErrSelfServiceOnboardingDisabled):
		return store.TelegramCallbackForbidden, "", false
	default:
		return store.TelegramCallbackFailed, "", false
	}
}

func (l *loop) enforceTelegramHistoryBlock(
	claim store.TelegramHistoryCallbackClaim,
	telegramUserID int64,
	block store.PublicTransmissionBlock,
	now int64,
) {
	domain, err := l.st.AuthorizedPresenceDomainForIdentity(claim.ActorID,
		store.Identity{Kind: store.IdentityTelegram, TelegramUserID: telegramUserID})
	if err != nil {
		return
	}
	for _, target := range domain.Targets {
		if block.OwnerScope == store.BlockOwnerActor && target.ActorID != claim.ActorID {
			continue
		}
		results, err := l.st.CancelTransmissionsFromSourceActorToNode(
			block.Internal.BlockedActorID, target.OrbitID, target.ActorID, target.Slot,
			store.TransmissionReasonActorBlocked, now)
		if err == nil {
			l.deliverTransmissionCancellations(results)
		}
	}
}

func (l *loop) applyTelegramHistoryAction(
	ev bot.Event,
	claim store.TelegramHistoryCallbackClaim,
) (store.TelegramCallbackOutcome, string, bool) {
	service, err := historyactions.NewService(l.st, l.mediaLifecycle, l.moderationService)
	if err != nil {
		return store.TelegramCallbackFailed, "", false
	}
	now := time.Now().UnixMilli()
	actor := historyactions.Actor{ExpectedActorID: claim.ActorID, Identity: store.Identity{
		Kind: store.IdentityTelegram, TelegramUserID: ev.FromUserID,
	}}
	switch claim.Action {
	case store.TelegramHistoryDelete:
		_, err = service.Delete(actor, claim.HistoryItemID, now)
		if err == nil {
			return store.TelegramCallbackApplied, "media_deleted", true
		}
	case store.TelegramHistoryReport:
		var report store.ModerationReportCreation
		report, err = service.Report(actor, claim.HistoryItemID, store.CreateModerationReportParams{
			Reason: claim.Reason,
		}, now)
		if err == nil {
			if report.Reused {
				return store.TelegramCallbackAlreadyApplied, "report_already_received", true
			}
			return store.TelegramCallbackApplied, "report_received", true
		}
	case store.TelegramHistoryBlockActor:
		keyHash := transmissionDigest("telegram-history-block:" + ev.Callback.Data)
		var block store.PublicTransmissionBlock
		block, err = service.Block(historyactions.BlockParams{
			Actor: actor, HistoryItemID: claim.HistoryItemID, Kind: historyactions.BlockActor,
			IdempotencyKeyHash: keyHash,
			RequestHash:        transmissionDigest("telegram-history-block:" + claim.HistoryItemID),
			CreatedAt:          now,
		})
		if err == nil {
			if !block.Reused {
				l.enforceTelegramHistoryBlock(claim, ev.FromUserID, block, now)
				return store.TelegramCallbackApplied, "sender_blocked", true
			}
			return store.TelegramCallbackAlreadyApplied, "sender_already_blocked", true
		}
	case store.TelegramHistoryReplay:
		keyHash := transmissionDigest("telegram-history-replay:" + ev.Callback.Data)
		var replay store.ResolvedTransmissionCreation
		replay, err = service.Replay(historyactions.ReplayParams{
			Actor: actor, HistoryItemID: claim.HistoryItemID,
			IdempotencyKeyHash: keyHash,
			RequestHash:        transmissionDigest("telegram-history-replay:" + claim.HistoryItemID),
			AudienceKind:       store.TransmissionAudienceCurrentAir,
			OriginKind:         store.TransmissionOriginTelegram, IncludeOrigin: true,
			RequestedDelivery: store.TransmissionDeliveryAfterCurrent, AcceptedAt: now,
			Availability: l.telegramTransmissionAvailability(),
		})
		if err == nil {
			l.handleTransmissionSignal(transmissionSignal{
				transmissionID: replay.Creation.Transmission.ID,
			})
			if replay.Reused {
				return store.TelegramCallbackAlreadyApplied, "replay_already_accepted", true
			}
			return store.TelegramCallbackApplied, "replay_accepted", true
		}
	default:
		return store.TelegramCallbackUnsupported, "", true
	}
	l.log.Error("apply Telegram history action", "action", claim.Action, "err", err)
	return telegramHistoryCallbackFailure(err)
}

func (l *loop) handleTelegramHistoryCallback(
	ev bot.Event,
	result store.TelegramHistoryCallbackResult,
) {
	callback := ev.Callback
	if result.Claim != nil {
		outcome, actionOutcome, consume := store.TelegramCallbackForbidden, "", false
		if ev.ChatType == "private" {
			outcome, actionOutcome, consume = l.applyTelegramHistoryAction(ev, *result.Claim)
		}
		final, err := l.st.FinalizeTelegramHistoryCallback(
			store.FinalizeTelegramHistoryCallbackParams{
				TelegramUserID: ev.FromUserID, QueryID: callback.QueryID,
				Token: callback.Data, ChatID: ev.ChatID, MessageID: ev.MessageID,
				Claim: *result.Claim, Outcome: outcome, ActionOutcome: actionOutcome,
				Consume: consume, ClearKeyboard: consume, Now: time.Now().UnixMilli(),
			})
		if err != nil {
			l.log.Error("finalize Telegram history callback", "err", err)
			result = store.TelegramHistoryCallbackResult{
				Found: true, Outcome: store.TelegramCallbackFailed,
			}
		} else {
			result = final
		}
	}
	if callback.Answer != nil {
		callback.Answer(telegramCallbackAnswer(result.Outcome))
	}
	if result.ActionOutcome != "" {
		ev.Reply(presentation.HistoryActionOutcomeLabel(result.ActionOutcome).Text(presentation.Russian))
	}
	if result.ClearKeyboard && callback.ClearKeyboard != nil {
		callback.ClearKeyboard()
	}
}
