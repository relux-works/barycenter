package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"relux.works/duet/coordinator/internal/bot"
	"relux.works/duet/coordinator/internal/store"
)

// Four common-route buttons plus two buttons for each explicit target stay at
// Telegram's 100-button keyboard ceiling. The current Air capacity cannot
// produce more than 48 Barycenter/Pulsar target options.
const telegramAutomationMaxExplicitTargets = 48

func telegramButtonLabel(value string, maxRunes int) string {
	value = strings.TrimSpace(strings.Join(strings.Fields(value), " "))
	if utf8.RuneCountInString(value) <= maxRunes {
		return value
	}
	runes := []rune(value)
	return string(runes[:maxRunes-1]) + "…"
}

func telegramScheduleNextRun(schedule store.AutomationSchedule, now time.Time) (time.Time, bool) {
	if !schedule.Enabled {
		return time.Time{}, false
	}
	location, err := time.LoadLocation(schedule.Timezone)
	if err != nil {
		return time.Time{}, false
	}
	// Enumerating UTC minutes makes DST gaps disappear and chooses the first
	// occurrence of a fall-back fold, matching the runtime's canonical rule.
	candidate := now.UTC().Truncate(time.Minute).Add(time.Minute)
	limit := candidate.Add(8*24*time.Hour + 3*time.Hour)
	for !candidate.After(limit) {
		local := candidate.In(location)
		if schedule.WeekdaysMask&(1<<int(local.Weekday())) != 0 &&
			local.Hour()*60+local.Minute() == schedule.LocalMinute {
			return candidate, true
		}
		candidate = candidate.Add(time.Minute)
	}
	return time.Time{}, false
}

func (l *loop) telegramAutomationContext(ev bot.Event) (store.ActorContext, store.Identity, bool) {
	if ev.ChatType != "private" {
		ev.Reply("Open this control in a private chat / Открой управление в личном чате.")
		return store.ActorContext{}, store.Identity{}, false
	}
	ctx, err := l.st.ResolveTelegramActorContext(ev.FromUserID)
	if err != nil || ctx.Role != "primary" || (!ctx.Capabilities.Has(store.CapabilityControl) &&
		!ctx.Capabilities.Has(store.CapabilityTelegram)) {
		ev.Reply("Control unavailable / Управление недоступно")
		return store.ActorContext{}, store.Identity{}, false
	}
	return ctx, store.Identity{Kind: store.IdentityTelegram, TelegramUserID: ev.FromUserID}, true
}

func (l *loop) sendTelegramSoundboard(ev bot.Event) {
	ctx, identity, ok := l.telegramAutomationContext(ev)
	if !ok {
		return
	}
	feature, err := l.st.AuthorizedAutomationFeatureStateForIdentity(ctx.ActorID, identity)
	if err != nil || !feature.SoundboardEnabled {
		ev.Reply("Soundboard is disabled / Звуковая панель выключена")
		return
	}
	list, err := l.st.AuthorizedSavedCueControlListForIdentity(ctx.ActorID, identity)
	if err != nil {
		l.log.Error("list Telegram soundboard", "err", err)
		ev.Reply("Soundboard unavailable / Звуковая панель недоступна")
		return
	}
	if len(list.Items) == 0 {
		ev.Reply("No saved cues / Нет сохранённых звуков")
		return
	}
	if l.telegramInlinePrompt == nil {
		ev.Reply("Soundboard is available in Pulsar / Звуковая панель доступна в Pulsar")
		return
	}
	l.telegramInlinePrompt(ev.ChatID,
		"<b>Soundboard / Звуковая панель</b>\nChoose a cue, then an explicit target and delivery policy / Выбери звук, затем адресата и способ доставки.",
		func(messageID int64) (bot.InlineKeyboard, error) {
			keyboard := make(bot.InlineKeyboard, 0, len(list.Items))
			for _, item := range list.Items {
				token, err := l.st.MintTelegramAutomationCallback(
					store.MintTelegramAutomationCallbackParams{
						TelegramUserID: ev.FromUserID, ChatID: ev.ChatID, MessageID: messageID,
						Binding: store.TelegramAutomationBinding{
							Action: store.TelegramAutomationCueSelect, CueID: item.Cue.ID,
						},
						Now: time.Now().UnixMilli(),
					})
				if err != nil {
					return nil, err
				}
				keyboard = append(keyboard, []bot.InlineButton{{
					Text: telegramButtonLabel(item.Cue.Title, 56), Data: token,
				}})
			}
			return keyboard, nil
		})
}

func telegramSoundboardRouteLabel(audience store.TransmissionAudienceKind,
	delivery store.TransmissionDelivery, target string) string {
	deliveryLabel := "Overlay / Поверх"
	if delivery == store.TransmissionDeliveryAfterCurrent {
		deliveryLabel = "After current / После текущего"
	}
	audienceLabel := target
	if audienceLabel == "" {
		switch audience {
		case store.TransmissionAudienceOwnBarycenter:
			audienceLabel = "own Barycenter / свой Барицентр"
		case store.TransmissionAudienceCurrentAir:
			audienceLabel = "current Air / текущий Air"
		}
	}
	return telegramButtonLabel(deliveryLabel+" · "+audienceLabel, 60)
}

func (l *loop) sendTelegramCueRoutes(ev bot.Event, cueID string) error {
	ctx, identity, ok := l.telegramAutomationContext(ev)
	if !ok {
		return store.ErrUnauthorized
	}
	list, err := l.st.AuthorizedSavedCueControlListForIdentity(ctx.ActorID, identity)
	if err != nil {
		return err
	}
	title := ""
	for _, item := range list.Items {
		if item.Cue.ID == cueID {
			title = item.Cue.Title
			break
		}
	}
	if title == "" {
		return store.ErrSavedCueNotFound
	}
	targets, err := l.st.ListTransmissionTargetReferencesForIdentity(ctx.ActorID, identity,
		time.Now().UnixMilli())
	if err != nil {
		return err
	}
	if l.telegramInlinePrompt == nil {
		return store.ErrAutomationDisabled
	}
	l.telegramInlinePrompt(ev.ChatID,
		fmt.Sprintf("<b>%s</b>\nChoose target and delivery / Выбери адресата и способ доставки.", esc(title)),
		func(messageID int64) (bot.InlineKeyboard, error) {
			var keyboard bot.InlineKeyboard
			add := func(audience store.TransmissionAudienceKind, reference string,
				delivery store.TransmissionDelivery, label string) error {
				token, err := l.st.MintTelegramAutomationCallback(
					store.MintTelegramAutomationCallbackParams{
						TelegramUserID: ev.FromUserID, ChatID: ev.ChatID, MessageID: messageID,
						Binding: store.TelegramAutomationBinding{
							Action: store.TelegramAutomationTrigger, CueID: cueID,
							Audience: audience, TargetReference: reference,
							Delivery: delivery, IncludeOrigin: true,
						}, Now: time.Now().UnixMilli(),
					})
				if err != nil {
					return err
				}
				keyboard = append(keyboard, []bot.InlineButton{{Text: label, Data: token}})
				return nil
			}
			for _, audience := range []store.TransmissionAudienceKind{
				store.TransmissionAudienceOwnBarycenter,
				store.TransmissionAudienceCurrentAir,
			} {
				for _, delivery := range []store.TransmissionDelivery{
					store.TransmissionDeliveryOverlay,
					store.TransmissionDeliveryAfterCurrent,
				} {
					if err := add(audience, "", delivery,
						telegramSoundboardRouteLabel(audience, delivery, "")); err != nil {
						return nil, err
					}
				}
			}
			for index, target := range targets {
				if index >= telegramAutomationMaxExplicitTargets {
					break
				}
				for _, delivery := range []store.TransmissionDelivery{
					store.TransmissionDeliveryOverlay,
					store.TransmissionDeliveryAfterCurrent,
				} {
					if err := add(store.TransmissionAudienceExplicit, target.Reference, delivery,
						telegramSoundboardRouteLabel(store.TransmissionAudienceExplicit,
							delivery, target.Label)); err != nil {
						return nil, err
					}
				}
			}
			return keyboard, nil
		})
	return nil
}

func (l *loop) sendTelegramAutomation(ev bot.Event) {
	ctx, identity, ok := l.telegramAutomationContext(ev)
	if !ok {
		return
	}
	feature, err := l.st.AuthorizedAutomationFeatureStateForIdentity(ctx.ActorID, identity)
	if err != nil {
		ev.Reply("Automation unavailable / Автоматизация недоступна")
		return
	}
	schedules, err := l.st.AuthorizedAutomationSchedulesForIdentity(ctx.ActorID, identity)
	if err != nil {
		l.log.Error("list Telegram automation", "err", err)
		ev.Reply("Automation unavailable / Автоматизация недоступна")
		return
	}
	state := "off / выключена"
	if feature.AutomationEnabled && !feature.EmergencyDisabled {
		state = "on / включена"
	} else if feature.EmergencyDisabled {
		state = "EMERGENCY STOP / АВАРИЙНО ОТКЛЮЧЕНА"
	}
	var text strings.Builder
	fmt.Fprintf(&text, "<b>Automation / Автоматизация</b>\nState / Состояние: %s", state)
	now := time.Now()
	for _, control := range schedules {
		schedule := control.Schedule
		line := "\n• " + telegramButtonLabel(schedule.DisplayName, 44)
		if schedule.Enabled {
			line += " — on"
			if next, ok := telegramScheduleNextRun(schedule, now); ok {
				location, _ := time.LoadLocation(schedule.Timezone)
				line += " · " + next.In(location).Format("2006-01-02 15:04 MST")
			}
		} else {
			line += " — off"
		}
		if text.Len()+len(line) <= 3500 {
			text.WriteString(line)
		}
	}
	if len(schedules) == 0 {
		text.WriteString("\nNo schedules / Нет расписаний")
	}
	if l.telegramInlinePrompt == nil {
		ev.Reply(text.String())
		return
	}
	l.telegramInlinePrompt(ev.ChatID, text.String(), func(messageID int64) (bot.InlineKeyboard, error) {
		keyboard := make(bot.InlineKeyboard, 0, len(schedules)+1)
		for _, control := range schedules {
			schedule := control.Schedule
			action, verb := store.TelegramAutomationScheduleEnable, "Enable / Включить"
			if schedule.Enabled {
				action, verb = store.TelegramAutomationScheduleDisable, "Disable / Выключить"
			}
			token, err := l.st.MintTelegramAutomationCallback(
				store.MintTelegramAutomationCallbackParams{
					TelegramUserID: ev.FromUserID, ChatID: ev.ChatID, MessageID: messageID,
					Binding: store.TelegramAutomationBinding{Action: action, ScheduleID: schedule.ID},
					Now:     time.Now().UnixMilli(),
				})
			if err != nil {
				return nil, err
			}
			keyboard = append(keyboard, []bot.InlineButton{{
				Text: telegramButtonLabel(verb+" · "+schedule.DisplayName, 60), Data: token,
			}})
		}
		token, err := l.st.MintTelegramAutomationCallback(
			store.MintTelegramAutomationCallbackParams{
				TelegramUserID: ev.FromUserID, ChatID: ev.ChatID, MessageID: messageID,
				Binding: store.TelegramAutomationBinding{Action: store.TelegramAutomationEmergencyDisable},
				Now:     time.Now().UnixMilli(),
			})
		if err != nil {
			return nil, err
		}
		keyboard = append(keyboard, []bot.InlineButton{{
			Text: "Emergency stop / Аварийно выключить", Data: token,
		}})
		return keyboard, nil
	})
}

func (l *loop) prepareTelegramSoundboardBuiltin(ctx store.ActorContext, identity store.Identity,
	cueID string, now int64) error {
	list, err := l.st.AuthorizedSavedCueControlListForIdentity(ctx.ActorID, identity)
	if err != nil {
		return err
	}
	builtin := false
	for _, item := range list.Items {
		if item.Cue.ID == cueID {
			builtin = item.Cue.SourceKind == store.SavedCueSourceBuiltin
			break
		}
	}
	if !builtin {
		return nil
	}
	if err := materializeAutomationBuiltinFile(l.cfg.MediaDir, ctx.OrbitID); err != nil {
		return err
	}
	_, err = l.st.EnsureAuthorizedAutomationBuiltinMediaForIdentity(ctx.ActorID,
		identity, cueID, now)
	return err
}

func telegramAutomationFailure(err error) store.TelegramCallbackOutcome {
	switch {
	case errors.Is(err, store.ErrUnauthorized), errors.Is(err, store.ErrInsufficientCapability),
		errors.Is(err, store.ErrAirPolicyDenied), errors.Is(err, store.ErrAutomationAudienceNotAllowed),
		errors.Is(err, store.ErrContentPolicyAcceptanceRequired):
		return store.TelegramCallbackForbidden
	case errors.Is(err, store.ErrSavedCueNotFound), errors.Is(err, store.ErrSavedCueStateConflict),
		errors.Is(err, store.ErrAutomationNotFound), errors.Is(err, store.ErrAutomationStateConflict),
		errors.Is(err, store.ErrAutomationDisabled), errors.Is(err, store.ErrAutomationCueNotReady),
		errors.Is(err, store.ErrTransmissionAudienceNotFound),
		errors.Is(err, store.ErrTransmissionAudienceEmpty),
		errors.Is(err, store.ErrTransmissionMediaInvalid),
		errors.Is(err, store.ErrTransmissionStateConflict),
		errors.Is(err, store.ErrTransmissionTargetInvalid):
		return store.TelegramCallbackTooLate
	case errors.Is(err, store.ErrTransmissionUnsupportedTargets),
		errors.Is(err, store.ErrTransmissionDeliveryKindMismatch),
		errors.Is(err, store.ErrAutomationCapabilityMissing):
		return store.TelegramCallbackUnsupported
	default:
		return store.TelegramCallbackFailed
	}
}

func (l *loop) applyTelegramAutomationCallback(ev bot.Event,
	binding store.TelegramAutomationBinding) (store.TelegramCallbackOutcome, error) {
	ctx, err := l.st.ResolveTelegramActorContext(ev.FromUserID)
	if err != nil {
		return telegramAutomationFailure(err), err
	}
	identity := store.Identity{Kind: store.IdentityTelegram, TelegramUserID: ev.FromUserID}
	now := time.Now().UnixMilli()
	switch binding.Action {
	case store.TelegramAutomationCueSelect:
		err := l.sendTelegramCueRoutes(ev, binding.CueID)
		if err == nil {
			return store.TelegramCallbackApplied, nil
		}
		return telegramAutomationFailure(err), err
	case store.TelegramAutomationTrigger:
		if err := l.prepareTelegramSoundboardBuiltin(ctx, identity, binding.CueID, now); err != nil {
			return telegramAutomationFailure(err), err
		}
		selectors := []store.TransmissionAudienceSelector(nil)
		if binding.Audience == store.TransmissionAudienceExplicit {
			selectors = []store.TransmissionAudienceSelector{{Reference: binding.TargetReference}}
		}
		requestHash := hashAutomationHTTP(fmt.Sprintf("telegram-soundboard:%s:%s:%s:%s:%t",
			binding.CueID, binding.Audience, binding.TargetReference, binding.Delivery,
			binding.IncludeOrigin))
		result, err := l.st.TriggerManualSoundboard(store.ManualSoundboardTriggerParams{
			CueID: binding.CueID,
			Transmission: store.CreateResolvedTransmissionParams{
				ExpectedActorID: ctx.ActorID, Identity: identity,
				IdempotencyKeyHash: transmissionDigest("telegram-soundboard:" + ev.Callback.Data),
				RequestHash:        requestHash, AudienceKind: binding.Audience, Selectors: selectors,
				IncludeOrigin: binding.IncludeOrigin, RequestedDelivery: binding.Delivery,
				AcceptedAt: now, Availability: l.telegramTransmissionAvailability(),
			},
		})
		if err != nil {
			return telegramAutomationFailure(err), err
		}
		if result.Challenge != nil {
			return store.TelegramCallbackRequiresConfirmation, nil
		}
		if !result.Reused {
			l.transmissionAccepted(result.Creation.Transmission.ID)
		}
		ev.Reply("Cue accepted / Звук принят")
		return store.TelegramCallbackApplied, nil
	case store.TelegramAutomationScheduleEnable, store.TelegramAutomationScheduleDisable:
		enabled := binding.Action == store.TelegramAutomationScheduleEnable
		requestHash := hashAutomationHTTP(fmt.Sprintf("telegram-schedule:%s:%d:%t",
			binding.ScheduleID, binding.ScheduleRevision, enabled))
		_, err := l.st.SetAuthorizedAutomationScheduleEnabled(store.AutomationControlAuth{
			ExpectedActorID: ctx.ActorID, Identity: identity,
			IdempotencyKeyHash: hashAutomationHTTP("telegram-automation:" + ev.Callback.Data),
			RequestHash:        requestHash, Now: now,
		}, binding.ScheduleID, binding.ScheduleRevision, enabled)
		if err != nil {
			return telegramAutomationFailure(err), err
		}
		ev.Reply("Schedule updated / Расписание обновлено")
		return store.TelegramCallbackApplied, nil
	case store.TelegramAutomationEmergencyDisable:
		feature, err := l.st.AuthorizedAutomationFeatureStateForIdentity(ctx.ActorID, identity)
		if err != nil {
			return telegramAutomationFailure(err), err
		}
		var quiet []store.AutomationQuietWindow
		if feature.QuietHoursJSON != "" && json.Unmarshal([]byte(feature.QuietHoursJSON), &quiet) != nil {
			return store.TelegramCallbackFailed, store.ErrAutomationInvalid
		}
		requestHash := hashAutomationHTTP(fmt.Sprintf("telegram-emergency:%d", binding.FeatureRevision))
		_, err = l.st.ReplaceAuthorizedAutomationFeatureState(store.AutomationControlAuth{
			ExpectedActorID: ctx.ActorID, Identity: identity,
			IdempotencyKeyHash: hashAutomationHTTP("telegram-automation:" + ev.Callback.Data),
			RequestHash:        requestHash, Now: now,
		}, store.AutomationFeatureControlParams{
			SoundboardEnabled: feature.SoundboardEnabled,
			AutomationEnabled: feature.AutomationEnabled,
			EmergencyDisabled: true, Timezone: feature.Timezone, QuietHours: quiet,
			ExpectedRevision: binding.FeatureRevision,
		})
		if err != nil {
			return telegramAutomationFailure(err), err
		}
		ev.Reply("Emergency stop applied / Автоматизация аварийно выключена")
		return store.TelegramCallbackApplied, nil
	default:
		return store.TelegramCallbackUnsupported, store.ErrAutomationInvalid
	}
}

func (l *loop) handleTelegramAutomationCallback(ev bot.Event,
	result store.TelegramAutomationCallbackResult) {
	callback := ev.Callback
	outcome := result.Outcome
	if result.Binding != nil {
		applied, err := l.applyTelegramAutomationCallback(ev, *result.Binding)
		outcome = applied
		if err != nil && outcome == store.TelegramCallbackFailed {
			l.log.Error("apply Telegram automation callback", "action", result.Binding.Action,
				"err", err)
		}
		if err := l.st.FinalizeTelegramAutomationCallback(callback.Data,
			callback.QueryID, outcome, time.Now().UnixMilli()); err != nil {
			l.log.Error("finalize Telegram automation callback", "err", err)
			outcome = store.TelegramCallbackFailed
		}
	}
	if callback.Answer != nil {
		callback.Answer(telegramCallbackAnswer(outcome))
	}
	if result.ClearKeyboard && callback.ClearKeyboard != nil {
		callback.ClearKeyboard()
	}
}
