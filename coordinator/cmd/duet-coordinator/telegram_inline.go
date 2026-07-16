package main

import (
	"errors"
	"fmt"
	"strings"
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
	if d.orderAirID != "" && (o.airID != d.orderAirID ||
		o.authorityGeneration != d.orderAirGeneration ||
		o.airRevision != d.orderAirRevision) {
		d.reply("аудиоклип готов, но исходный Air уже изменился — доставка отменена")
		return
	}
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
		} else {
			// Rollback-era session elements cannot encode an exact N-target set.
			// Refuse the legacy path instead of silently turning personal delivery
			// into a broadcast. Production media uses the common transmission
			// service and supports every resolved recipient independently.
			d.reply("личная доставка нескольким адресатам требует общего сервиса маршрутизации")
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
			Capabilities: state.Capabilities.Values(),
		})
	}
	return result
}

func (l *loop) telegramDefaultAudience(
	d mediaDone,
) (store.TransmissionAudienceKind, []store.TransmissionAudienceSelector, bool, bool, error) {
	if d.attachmentKind != "voice" || !d.personal {
		return store.TransmissionAudienceCurrentAir, nil, true, true, nil
	}
	actor, err := l.st.ResolveTelegramActorContext(d.from)
	if err != nil {
		return "", nil, false, false, err
	}
	selectors, err := l.st.IssuePersonalTransmissionTargets(
		store.IssuePersonalTransmissionTargetsParams{
			ExpectedActorID: actor.ActorID,
			Identity: store.Identity{
				Kind: store.IdentityTelegram, TelegramUserID: d.from,
			},
			SourceOrbitID: d.orbit, IssuedAt: d.acceptedAt,
		},
	)
	if errors.Is(err, store.ErrTransmissionAudienceEmpty) {
		return "", nil, false, false, nil
	}
	if err != nil {
		return "", nil, false, false, err
	}
	return store.TransmissionAudienceExplicit, selectors, true, true, nil
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
	text            string
	action          store.TelegramInlineAction
	delivery        store.TransmissionDelivery
	audience        store.TransmissionAudienceKind
	routeV2         bool
	targetReference string
	includeOrigin   bool
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

func (l *loop) telegramExplicitRoutingChoices(
	telegramUserID int64,
	route store.TelegramInlineRoute,
	now int64,
) ([]telegramPromptChoice, error) {
	actor, err := l.st.ResolveTelegramActorContext(telegramUserID)
	if err != nil {
		return nil, err
	}
	options, err := l.st.ListTransmissionTargetReferencesForIdentity(
		actor.ActorID,
		store.Identity{Kind: store.IdentityTelegram, TelegramUserID: telegramUserID},
		now,
	)
	if err != nil {
		return nil, err
	}
	media, err := l.st.GetMediaItem(route.MediaID)
	if err != nil || media == nil {
		return nil, store.ErrTransmissionMediaNotFound
	}
	deliveries := []store.TransmissionDelivery{
		store.TransmissionDeliveryOverlay,
		store.TransmissionDeliveryInterrupt,
		store.TransmissionDeliveryAfterCurrent,
	}
	if media.Kind == store.MediaKindAudioTrack {
		deliveries = []store.TransmissionDelivery{"queue", "replace"}
	}
	choices := make([]telegramPromptChoice, 0, len(options)*len(deliveries)*2+8)
	if media.Kind == store.MediaKindAudioTrack {
		for _, audience := range []store.TransmissionAudienceKind{
			store.TransmissionAudienceOwnBarycenter,
			store.TransmissionAudienceCurrentAir,
		} {
			audienceLabel := presentation.AudienceLabel(
				audience, l.telegramPeerTitle(route.SourceOrbitID),
			).Text(presentation.Russian)
			for _, includeOrigin := range []bool{false, true} {
				for _, delivery := range deliveries {
					choices = append(choices, telegramPromptChoice{
						text: presentation.DeliveryLabel(delivery).Text(presentation.Russian) +
							" · " + audienceLabel + " · " +
							presentation.IncludeOriginLabel(includeOrigin).Text(presentation.Russian),
						action: store.TelegramChooseOwn, delivery: delivery,
						audience: audience, routeV2: true, includeOrigin: includeOrigin,
					})
				}
			}
		}
	}
	for _, option := range options {
		targetLabel := presentation.TargetLabel(presentation.TargetMetadata{
			OrbitTitle: option.OrbitTitle, Slot: option.Slot,
			MultipleSlots: len(option.TargetSlots) > 1,
		}).Text(presentation.Russian)
		for _, includeOrigin := range []bool{false, true} {
			originSuffix := " · без исходного Pulsar"
			if includeOrigin {
				originSuffix = " · + исходный Pulsar"
			}
			for _, delivery := range deliveries {
				choices = append(choices, telegramPromptChoice{
					text: presentation.DeliveryLabel(delivery).Text(presentation.Russian) +
						" · " + targetLabel + originSuffix,
					action: store.TelegramChooseOwn, delivery: delivery,
					audience: store.TransmissionAudienceExplicit, routeV2: true,
					targetReference: option.Reference, includeOrigin: includeOrigin,
				})
			}
		}
	}
	return choices, nil
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
	if media, mediaErr := l.st.GetMediaItem(route.MediaID); mediaErr == nil &&
		media != nil && media.Kind == store.MediaKindAudioTrack {
		choices = nil
	}
	explicit, err := l.telegramExplicitRoutingChoices(
		telegramUserID, route, time.Now().UnixMilli(),
	)
	if err != nil {
		l.log.Error("list Telegram explicit targets", "media", route.MediaID, "err", err)
	} else {
		choices = append(choices, explicit...)
	}
	l.telegramInlinePrompt(chatID, text, func(messageID int64) (bot.InlineKeyboard, error) {
		keyboard := make(bot.InlineKeyboard, 0, len(choices)+1)
		for _, choice := range choices {
			token, err := l.st.MintTelegramInlineCallback(store.MintTelegramInlineCallbackParams{
				TelegramUserID: telegramUserID, MediaID: route.MediaID,
				MediaGeneration: route.MediaGeneration, ChatID: chatID, MessageID: messageID,
				Action: choice.action, Delivery: choice.delivery, Audience: choice.audience,
				RouteV2: choice.routeV2, TargetReference: choice.targetReference,
				IncludeOrigin: choice.includeOrigin,
				Now:           time.Now().UnixMilli(),
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
				params := store.MintTelegramInlineCallbackParams{
					TelegramUserID: telegramUserID, MediaID: result.MediaID,
					MediaGeneration: result.MediaGeneration, ChatID: chatID, MessageID: messageID,
					Action: action, Delivery: alternative.Delivery, Audience: result.Audience,
					ConfirmationTokenHash: result.ConfirmationTokenHash,
					Now:                   time.Now().UnixMilli(),
				}
				if result.RouteV2 {
					params.RouteV2 = true
					params.Delivery = store.TransmissionDeliveryInterrupt
					params.ConfirmationDelivery = alternative.Delivery
					params.TargetReference = result.TargetReference
					params.IncludeOrigin = result.IncludeOrigin
				}
				token, err := l.st.MintTelegramInlineCallback(params)
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

func telegramUnsupportedTargetsText(err error) string {
	if errors.Is(err, store.ErrTransmissionDeliveryKindMismatch) {
		return "доставка пользовательских треков пока недоступна: production-профиль потокового воспроизведения не принят"
	}
	var unsupported *store.TransmissionUnsupportedTargetsError
	if !errors.As(err, &unsupported) {
		return "выбранные получатели больше недоступны"
	}
	missing := map[string]bool{}
	for _, target := range unsupported.Targets {
		for _, capability := range target.MissingCapabilities {
			missing[capability] = true
		}
	}
	labels := make([]string, 0, len(missing))
	for _, capability := range []struct{ value, label string }{
		{store.TransmissionCapabilityMediaClip, "аудиоклипы"},
		{store.TransmissionCapabilityOverlayMix, "микширование поверх эфира"},
		{store.TransmissionCapabilityInterrupt, "прерывание с продолжением"},
		{store.TransmissionCapabilityAudioTrack, "пользовательские треки"},
		{store.TransmissionCapabilityQueueReplace, "очередь и замена трека"},
		{store.TransmissionCapabilityStream, "потоковое воспроизведение"},
	} {
		if missing[capability.value] {
			labels = append(labels, capability.label)
		}
	}
	if len(labels) == 0 {
		return "выбранные получатели не поддерживают этот способ доставки"
	}
	return "выбранные получатели пока не поддерживают: " + strings.Join(labels, ", ")
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
	airResult, err := l.st.ClaimTelegramAirCallback(store.ClaimTelegramAirCallbackParams{
		TelegramUserID: ev.FromUserID, QueryID: callback.QueryID,
		Token: callback.Data, ChatID: ev.ChatID, MessageID: ev.MessageID,
		Now: time.Now().UnixMilli(),
	})
	if err != nil {
		l.log.Error("claim Telegram Air callback", "err", err)
		if callback.Answer != nil {
			callback.Answer(bot.CallbackFailed)
		}
		return
	}
	if airResult.Found {
		l.handleTelegramAirCallback(ev, airResult)
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
		if errors.Is(err, store.ErrAirPolicyDenied) {
			result.Outcome = store.TelegramCallbackForbidden
			result.ClearKeyboard = true
			ev.Reply("политика текущего Air запрещает этот способ доставки")
		} else if errors.Is(err, store.ErrTransmissionUnsupportedTargets) ||
			errors.Is(err, store.ErrTransmissionDeliveryKindMismatch) {
			result.Outcome = store.TelegramCallbackUnsupported
			result.ClearKeyboard = true
			ev.Reply(telegramUnsupportedTargetsText(err))
		} else if errors.Is(err, store.ErrTransmissionAudienceNotFound) ||
			errors.Is(err, store.ErrTransmissionAudienceEmpty) {
			result.Outcome = store.TelegramCallbackForbidden
			result.ClearKeyboard = true
			ev.Reply("выбранные получатели больше недоступны")
		} else {
			l.log.Error("apply Telegram inline callback", "err", err)
			result.Outcome = store.TelegramCallbackFailed
		}
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
