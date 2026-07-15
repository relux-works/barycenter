package main

import (
	"fmt"
	"time"

	"relux.works/duet/coordinator/internal/bot"
	"relux.works/duet/coordinator/internal/hub"
	"relux.works/duet/coordinator/internal/presentation"
	"relux.works/duet/coordinator/internal/protocol"
	"relux.works/duet/coordinator/internal/session"
	"relux.works/duet/coordinator/internal/store"
	"relux.works/duet/coordinator/internal/ulid"
)

// processLegacyTelegramMediaDone keeps old direct/internal callers and
// rollback-era rows source compatible. Production SubmitMedia items always
// take the durable generic-transmission path above.
func (l *loop) processLegacyTelegramMediaDone(d mediaDone) {
	o := l.stateFor(d.orbit)
	target := "both"
	if d.personal {
		mine := protocol.NodeID("")
		if slot, _ := l.st.SlotOf(d.orbit, d.from); slot != "" {
			mine = l.peerFor(o, hub.NodeKey{Orbit: d.orbit, Slot: protocol.NodeID(slot)})
		}
		var others []protocol.NodeID
		for _, peer := range o.sess.Peers {
			if mine != "" && peer == mine {
				continue
			}
			others = append(others, peer)
		}
		if len(others) == 1 {
			target = string(others[0])
		} else if len(others) == 0 {
			d.reply("в орбите пока только твой дом — отправлю всем, когда появятся другие")
			return
		}
	}
	element := session.Element{
		ID: ulid.NewElementID(time.Now()), Kind: session.KindVoice,
		MediaID: d.mediaID, DurationMS: d.result.DurationMS,
		RequestedBy: protocol.NodeID(d.fromName), Target: target, CreatedAt: d.acceptedAt,
	}
	_ = l.st.InsertElement(element)
	if o.sess.Mode == session.ModeShared {
		l.apply(o, o.sess.EnqueueVoice(element))
		mediaLabel := "голосовое"
		if d.attachmentKind == "audio" || d.attachmentKind == "document" {
			mediaLabel = "аудиоклип"
		}
		if target != "both" {
			d.reply(fmt.Sprintf("личное %s от %s готово: после текущего трека, только адресату", mediaLabel, esc(d.fromName)))
		} else {
			d.reply(fmt.Sprintf("%s от %s готово: после текущего трека, для всех", mediaLabel, esc(d.fromName)))
		}
		return
	}
	payload := &protocol.SoloVoicePayload{ElementID: element.ID, FileURL: l.mediaURL(d.mediaID)}
	targets := append([]protocol.NodeID{}, o.sess.Peers...)
	if target != "both" {
		targets = []protocol.NodeID{protocol.NodeID(target)}
	}
	for _, peer := range targets {
		l.hub.Send(l.nodeKey(o, peer), protocol.TypeSoloVoice, payload)
	}
	d.reply("вставка уйдёт на ближайшей границе трека")
}

func (l *loop) telegramTransmissionAvailability() []store.TransmissionTargetAvailability {
	snapshot := l.hub.NodeSnapshots()
	result := make([]store.TransmissionTargetAvailability, 0, len(snapshot))
	for key, state := range snapshot {
		result = append(result, store.TransmissionTargetAvailability{
			OrbitID: key.Orbit, Slot: string(key.Slot), Connected: state.Connected,
			LastSeenAt: state.LastSeenAt, CredentialTokenHash: state.CredentialTokenHash,
			MediaClipCapable: state.Capabilities.Supports(protocol.CapabilityMediaClip),
			OverlayCapable:   state.Capabilities.Supports(protocol.CapabilityOverlayMix),
			InterruptCapable: state.Capabilities.Supports(protocol.CapabilityInterruptResume),
			MainActive:       true,
			InterruptResumeReady: state.Capabilities.Supports(
				protocol.CapabilityInterruptResume,
			),
		})
	}
	return result
}

func (l *loop) telegramDefaultAudience(
	d mediaDone,
) (store.TransmissionAudienceKind, []store.TransmissionAudienceSelector, bool, bool) {
	if d.attachmentKind != "voice" || !d.personal {
		return store.TransmissionAudienceCurrentAir, nil, true, true
	}
	if _, other, active, err := l.st.ActiveLink(d.orbit); err == nil && active {
		return store.TransmissionAudienceExplicit,
			[]store.TransmissionAudienceSelector{{
				Kind: store.TransmissionSelectorBarycenter, OrbitID: other,
			}}, true, true
	}
	// A solo barycenter may have multiple installations. Preserve the old
	// personal rule by selecting every Pulsar except the sender's own slot.
	mine, _ := l.st.SlotOf(d.orbit, d.from)
	var selectors []store.TransmissionAudienceSelector
	for _, peer := range l.orbit(d.orbit).sess.Peers {
		slot := string(peer)
		if slot == mine {
			continue
		}
		selectors = append(selectors, store.TransmissionAudienceSelector{
			Kind: store.TransmissionSelectorPulsar, OrbitID: d.orbit, Slot: slot,
		})
	}
	if len(selectors) == 0 {
		return "", nil, false, false
	}
	return store.TransmissionAudienceExplicit, selectors, true, true
}

func (l *loop) telegramPeerTitle(orbitID int64) string {
	_, other, active, err := l.st.ActiveLink(orbitID)
	if err != nil || !active {
		return ""
	}
	orbit, err := l.st.GetOrbit(other)
	if err != nil || orbit == nil {
		return ""
	}
	return orbit.Title
}

type telegramPromptChoice struct {
	text     string
	action   store.TelegramInlineAction
	delivery store.TransmissionDelivery
	audience store.TransmissionAudienceKind
}

func (l *loop) telegramRoutingChoices(orbitID int64) []telegramPromptChoice {
	peerTitle := l.telegramPeerTitle(orbitID)
	choices := make([]telegramPromptChoice, 0, 6)
	for _, audience := range []store.TransmissionAudienceKind{
		store.TransmissionAudienceOwnBarycenter,
		store.TransmissionAudienceCurrentAir,
	} {
		audienceText := presentation.AudienceLabel(audience, peerTitle).Text(presentation.Russian)
		for _, delivery := range []struct {
			value  store.TransmissionDelivery
			action store.TelegramInlineAction
		}{
			{store.TransmissionDeliveryOverlay, store.TelegramChooseOverlay},
			{store.TransmissionDeliveryInterrupt, store.TelegramChooseInterrupt},
			{store.TransmissionDeliveryAfterCurrent, store.TelegramChooseAfterCurrent},
		} {
			choices = append(choices, telegramPromptChoice{
				text: presentation.DeliveryLabel(delivery.value).Text(presentation.Russian) +
					" · " + audienceText,
				action: delivery.action, delivery: delivery.value, audience: audience,
			})
		}
	}
	return choices
}

func (l *loop) sendTelegramRoutingPrompt(
	chatID, telegramUserID int64,
	route store.TelegramInlineRoute,
	text string,
) {
	if l.telegramInlinePrompt == nil || chatID == 0 {
		return
	}
	choices := l.telegramRoutingChoices(route.SourceOrbitID)
	l.telegramInlinePrompt(chatID, text, func(messageID int64) (bot.InlineKeyboard, error) {
		keyboard := make(bot.InlineKeyboard, 0, len(choices)+1)
		for _, choice := range choices {
			token, err := l.st.MintTelegramInlineCallback(store.MintTelegramInlineCallbackParams{
				TelegramUserID: telegramUserID, MediaID: route.MediaID,
				MediaGeneration: route.MediaGeneration, ChatID: chatID, MessageID: messageID,
				Action: choice.action, Delivery: choice.delivery, Audience: choice.audience,
				Now: time.Now().UnixMilli(),
			})
			if err != nil {
				return nil, err
			}
			keyboard = append(keyboard, []bot.InlineButton{{Text: choice.text, Data: token}})
		}
		dismiss, err := l.st.MintTelegramInlineCallback(store.MintTelegramInlineCallbackParams{
			TelegramUserID: telegramUserID, MediaID: route.MediaID,
			MediaGeneration: route.MediaGeneration, ChatID: chatID, MessageID: messageID,
			Action: store.TelegramDismiss, Now: time.Now().UnixMilli(),
		})
		if err != nil {
			return nil, err
		}
		return append(keyboard, []bot.InlineButton{{Text: "Оставить как есть", Data: dismiss}}), nil
	})
}

func (l *loop) sendTelegramConfirmationPrompt(
	chatID, telegramUserID int64,
	result store.ApplyTelegramInlineCallbackResult,
) {
	if l.telegramInlinePrompt == nil || result.Challenge == nil {
		return
	}
	l.telegramInlinePrompt(chatID,
		presentation.ConfirmationLabel("interrupt_required").Text(presentation.Russian),
		func(messageID int64) (bot.InlineKeyboard, error) {
			var keyboard bot.InlineKeyboard
			for _, alternative := range result.Challenge.Alternatives {
				if !alternative.Available {
					continue
				}
				action := store.TelegramConfirmAfter
				if alternative.Delivery == store.TransmissionDeliveryOverlay {
					action = store.TelegramConfirmOverlay
				}
				token, err := l.st.MintTelegramInlineCallback(store.MintTelegramInlineCallbackParams{
					TelegramUserID: telegramUserID, MediaID: result.MediaID,
					MediaGeneration: result.MediaGeneration, ChatID: chatID, MessageID: messageID,
					Action: action, Delivery: alternative.Delivery, Audience: result.Audience,
					ConfirmationTokenHash: result.ConfirmationTokenHash,
					Now:                   time.Now().UnixMilli(),
				})
				if err != nil {
					return nil, err
				}
				keyboard = append(keyboard, []bot.InlineButton{{
					Text: presentation.DeliveryLabel(alternative.Delivery).Text(presentation.Russian),
					Data: token,
				}})
			}
			return keyboard, nil
		})
}

func telegramCallbackAnswer(outcome store.TelegramCallbackOutcome) bot.CallbackAnswerCode {
	switch outcome {
	case store.TelegramCallbackApplied:
		return bot.CallbackApplied
	case store.TelegramCallbackAlreadyApplied:
		return bot.CallbackAlreadyApplied
	case store.TelegramCallbackRequiresConfirmation:
		return bot.CallbackRequiresConfirmation
	case store.TelegramCallbackTooLate:
		return bot.CallbackTooLate
	case store.TelegramCallbackExpired:
		return bot.CallbackExpired
	case store.TelegramCallbackForbidden:
		return bot.CallbackForbidden
	case store.TelegramCallbackUnsupported:
		return bot.CallbackUnsupported
	default:
		return bot.CallbackFailed
	}
}

func (l *loop) handleTelegramInlineCallback(ev bot.Event) {
	callback := ev.Callback
	if callback.Data == "" || ev.ChatID == 0 || ev.MessageID <= 0 {
		if callback.Answer != nil {
			callback.Answer(bot.CallbackUnsupported)
		}
		if callback.ClearKeyboard != nil {
			callback.ClearKeyboard()
		}
		return
	}
	historyResult, err := l.st.ClaimTelegramHistoryCallback(
		store.ClaimTelegramHistoryCallbackParams{
			TelegramUserID: ev.FromUserID, QueryID: callback.QueryID,
			Token: callback.Data, ChatID: ev.ChatID, MessageID: ev.MessageID,
			Now: time.Now().UnixMilli(),
		})
	if err != nil {
		l.log.Error("claim Telegram history callback", "err", err)
		if callback.Answer != nil {
			callback.Answer(bot.CallbackFailed)
		}
		return
	}
	if historyResult.Found {
		l.handleTelegramHistoryCallback(ev, historyResult)
		return
	}
	result, err := l.st.ApplyTelegramInlineCallback(store.ApplyTelegramInlineCallbackParams{
		TelegramUserID: ev.FromUserID, QueryID: callback.QueryID, Token: callback.Data,
		ChatID: ev.ChatID, MessageID: ev.MessageID, Now: time.Now().UnixMilli(),
		Availability: l.telegramTransmissionAvailability(),
	})
	if err != nil {
		l.log.Error("apply Telegram inline callback", "err", err)
		result.Outcome = store.TelegramCallbackFailed
	}
	if result.Cancellation != nil {
		l.handleTransmissionSignal(transmissionSignal{
			transmissionID: result.Cancellation.Transmission.ID,
			cancellation:   result.Cancellation,
		})
	}
	if result.Creation != nil {
		l.handleTransmissionSignal(transmissionSignal{
			transmissionID: result.Creation.Transmission.ID,
		})
		shown := presentation.PresentDelivery(
			result.Creation.Transmission.RequestedDelivery,
			result.Creation.Transmission.EffectiveDelivery,
			result.Creation.Transmission.DowngradeReason,
		)
		message := fmt.Sprintf("маршрут принят: %s · %s",
			esc(shown.Effective.Text(presentation.Russian)),
			esc(presentation.AudienceLabel(result.Creation.Transmission.AudienceKind,
				l.telegramPeerTitle(result.Creation.Transmission.SourceOrbitID)).Text(presentation.Russian)))
		if shown.Notice != nil {
			message += "\n" + esc(shown.Notice.Text(presentation.Russian))
		}
		ev.Reply(message)
	}
	if result.Challenge != nil {
		l.sendTelegramConfirmationPrompt(ev.ChatID, ev.FromUserID, result)
	}
	if callback.Answer != nil {
		callback.Answer(telegramCallbackAnswer(result.Outcome))
	}
	if result.ClearKeyboard && callback.ClearKeyboard != nil {
		callback.ClearKeyboard()
	}
}
