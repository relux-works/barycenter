package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"relux.works/duet/coordinator/internal/bot"
	"relux.works/duet/coordinator/internal/store"
)

func telegramAirAuth(ctx store.ActorContext, telegramUserID int64, key string, request any, now int64) store.AirMutationAuth {
	raw, _ := json.Marshal(request)
	return store.AirMutationAuth{
		ExpectedActorID:    ctx.ActorID,
		Identity:           store.Identity{Kind: store.IdentityTelegram, TelegramUserID: telegramUserID},
		IdempotencyKeyHash: hashAirHTTP("telegram-air:" + key),
		RequestHash:        hashAirHTTP(string(raw)), Now: now,
	}
}

func telegramAirExpected(current string) string {
	if current == "" {
		return "none"
	}
	return current
}

func telegramAirPolicyText(policy store.AirPolicyView) string {
	label := func(value string) string {
		switch value {
		case "owner_primary":
			return "owner primary / primary владельца"
		case "air_admin_primary":
			return "Air admins / администраторы Air"
		case "all_member_primaries":
			return "all Barycenter primaries / все primary Барицентров"
		case "primary_companion":
			return "primaries + companions / primary и companion"
		case "disabled":
			return "off / выключено"
		default:
			return "unknown / неизвестно"
		}
	}
	return fmt.Sprintf("invites / приглашения: %s\noverlay: %s · queue / очередь: %s\nreplace / замена: %s",
		label(policy.Invite), label(policy.Overlay), label(policy.Queue), label(policy.Replace))
}

func telegramAirNextPolicy(policy store.AirPolicyView) store.AirPolicyView {
	next := store.AirPolicyView{Revision: policy.Revision}
	switch {
	case policy.Invite == "owner_primary" && policy.Overlay == "disabled":
		next.Invite, next.Overlay, next.Queue, next.Replace =
			"air_admin_primary", "air_admin_primary", "air_admin_primary", "air_admin_primary"
	case policy.Invite == "air_admin_primary" && policy.Overlay == "air_admin_primary":
		next.Invite, next.Overlay, next.Queue, next.Replace =
			"all_member_primaries", "all_member_primaries", "all_member_primaries", "all_member_primaries"
	case policy.Invite == "all_member_primaries":
		next.Invite, next.Overlay, next.Queue, next.Replace =
			"air_admin_primary", "primary_companion", "primary_companion", "air_admin_primary"
	default:
		next.Invite, next.Overlay, next.Queue, next.Replace =
			"owner_primary", "disabled", "disabled", "owner_primary"
	}
	return next
}

func telegramAirText(air store.AirProjection) string {
	current := "saved / сохранён"
	if air.IsCurrent {
		current = "current / текущий"
	}
	status := "joined / подключён"
	if air.MembershipStatus == "pending_confirmation" {
		status = "confirmation required / нужно подтверждение"
	}
	return fmt.Sprintf("<b>Air «%s»</b>\n%s · %s · role %s\nBarycenters / Барицентры: %d/%d · online Pulsars: %d/%d\n%s",
		esc(air.Title), current, status, esc(air.AirRole), air.MemberCount,
		air.Capacity.Barycenters, air.OnlinePulsarCount, air.Capacity.OnlinePulsars,
		esc(telegramAirPolicyText(air.Policy)))
}

func (l *loop) telegramAirKeyboard(
	telegramUserID, chatID, messageID int64,
	view store.AirListView, air store.AirProjection,
) (bot.InlineKeyboard, error) {
	now := time.Now().UnixMilli()
	var keyboard bot.InlineKeyboard
	add := func(text string, binding store.TelegramAirBinding) error {
		token, err := l.st.MintTelegramAirCallback(store.MintTelegramAirCallbackParams{
			TelegramUserID: telegramUserID, ChatID: chatID, MessageID: messageID,
			Binding: binding, Now: now,
		})
		if err != nil {
			return err
		}
		keyboard = append(keyboard, []bot.InlineButton{{Text: text, Data: token}})
		return nil
	}
	base := store.TelegramAirBinding{
		AirID: air.AirID, MembershipID: air.MembershipID,
		AirRevision: air.Revision, MembershipRevision: air.MembershipRevision,
		ExpectedActiveAirID: telegramAirExpected(view.CurrentAirID), Policy: air.Policy,
	}
	if air.MembershipStatus == "pending_confirmation" {
		confirm := base
		confirm.Action = store.TelegramAirConfirmJoin
		if err := add("Join saved / Подтвердить", confirm); err != nil {
			return nil, err
		}
		confirmActive := base
		confirmActive.Action = store.TelegramAirConfirmJoinActivate
		if err := add("Join & switch / Подтвердить и включить", confirmActive); err != nil {
			return nil, err
		}
		decline := base
		decline.Action = store.TelegramAirDeclineJoin
		if err := add("Decline / Отказаться", decline); err != nil {
			return nil, err
		}
		return keyboard, nil
	}
	if air.IsCurrent {
		deactivate := base
		deactivate.Action = store.TelegramAirDeactivate
		if err := add("Park Air / Остановить Air", deactivate); err != nil {
			return nil, err
		}
	} else {
		activate := base
		activate.Action = store.TelegramAirActivate
		label := "Activate / Включить"
		if view.CurrentAirID != "" {
			label = "Confirm switch here / Переключить сюда"
		}
		if err := add(label, activate); err != nil {
			return nil, err
		}
	}
	if air.AirRole == "owner" || air.AirRole == "admin" {
		member := base
		member.Action = store.TelegramAirIssueMember
		if err := add("Invite member / Позвать участника", member); err != nil {
			return nil, err
		}
	}
	if air.AirRole == "owner" {
		admin := base
		admin.Action = store.TelegramAirIssueAdmin
		if err := add("Invite admin / Позвать администратора", admin); err != nil {
			return nil, err
		}
		policy := base
		policy.Action, policy.Policy = store.TelegramAirPolicyNext, telegramAirNextPolicy(air.Policy)
		if err := add("Next policy preset / Следующая политика", policy); err != nil {
			return nil, err
		}
		dissolve := base
		dissolve.Action = store.TelegramAirDissolve
		if err := add("Dissolve forever / Распустить навсегда", dissolve); err != nil {
			return nil, err
		}
	} else {
		leave := base
		leave.Action = store.TelegramAirLeave
		if err := add("Leave Air / Покинуть Air", leave); err != nil {
			return nil, err
		}
	}
	return keyboard, nil
}

func (l *loop) sendTelegramAirs(ev bot.Event) {
	ctx, err := l.st.ResolveTelegramActorContext(ev.FromUserID)
	if err != nil {
		ev.Reply("Air unavailable / Air недоступны")
		return
	}
	identity := store.Identity{Kind: store.IdentityTelegram, TelegramUserID: ev.FromUserID}
	view, err := l.st.AuthorizedAirListForIdentity(ctx.ActorID, identity)
	if err != nil {
		l.log.Error("list Telegram Airs", "err", err)
		ev.Reply("Could not load Airs / Не удалось загрузить Air")
		return
	}
	if len(view.Saved) == 0 {
		ev.Reply("No saved Airs / Нет сохранённых Air. Создать: /air create Название; вступить: /air join КОД")
		return
	}
	for _, air := range view.Saved {
		air := air
		text := telegramAirText(air)
		if l.telegramInlinePrompt == nil {
			ev.Reply(text)
			continue
		}
		l.telegramInlinePrompt(ev.ChatID, text, func(messageID int64) (bot.InlineKeyboard, error) {
			return l.telegramAirKeyboard(ev.FromUserID, ev.ChatID, messageID, view, air)
		})
	}
}

func (l *loop) handleTelegramAirCommand(ev bot.Event, target string) {
	if ev.ChatType != "private" {
		ev.Reply("Open /air in a private chat / Открой /air в личном чате с ботом.")
		return
	}
	ctx, err := l.st.ResolveTelegramActorContext(ev.FromUserID)
	if err != nil {
		ev.Reply("Air unavailable / Air недоступны")
		return
	}
	now := time.Now().UnixMilli()
	parts := strings.SplitN(strings.TrimSpace(target), " ", 2)
	if len(parts) == 1 && parts[0] == "" {
		l.sendTelegramAirs(ev)
		return
	}
	switch strings.ToLower(parts[0]) {
	case "create":
		if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
			ev.Reply("Usage / Формат: /air create Название")
			return
		}
		title := strings.TrimSpace(parts[1])
		req := struct {
			Title string `json:"title"`
		}{title}
		_, err := l.st.CreateAuthorizedAir(telegramAirAuth(ctx, ev.FromUserID,
			fmt.Sprintf("create:%d:%d", ev.ChatID, ev.MessageID), req, now), title)
		if err != nil {
			l.replyTelegramAirError(ev, err)
			return
		}
		ev.Reply("Air created / Air создан. /air — открыть управление.")
	case "join":
		if len(parts) != 2 || strings.TrimSpace(parts[1]) == "" {
			ev.Reply("Usage / Формат: /air join КОД")
			return
		}
		code := strings.TrimSpace(parts[1])
		req := struct {
			Code string `json:"code"`
		}{code}
		preview, err := l.st.ConsumeAuthorizedAirInvite(telegramAirAuth(ctx, ev.FromUserID,
			fmt.Sprintf("join:%d:%d", ev.ChatID, ev.MessageID), req, now), code)
		if err != nil {
			l.replyTelegramAirError(ev, err)
			return
		}
		if ev.DeleteSource != nil {
			ev.DeleteSource()
		}
		l.sendTelegramAirJoinConfirmation(ev, preview)
	default:
		ev.Reply("Commands / Команды: /air · /air create Название · /air join КОД")
	}
}

func (l *loop) sendTelegramAirJoinConfirmation(ev bot.Event, preview store.AirJoinPreview) {
	text := fmt.Sprintf("Join Air «%s»? / Вступить в Air «%s»?\nOwner / Владелец: %s · role %s\nBarycenters / Барицентры: %d/%d\n%s",
		esc(preview.Title), esc(preview.Title), esc(preview.OwnerDisplayName), esc(preview.IntendedRole),
		preview.MemberCount, preview.Capacity.Barycenters, esc(telegramAirPolicyText(preview.Policy)))
	if l.telegramInlinePrompt == nil {
		ev.Reply(text)
		return
	}
	l.telegramInlinePrompt(ev.ChatID, text, func(messageID int64) (bot.InlineKeyboard, error) {
		// Mint validates the fresh actor and pending membership projection.
		base := store.TelegramAirBinding{AirID: preview.AirID, MembershipID: preview.MembershipID,
			MembershipRevision: preview.MembershipRevision, Policy: preview.Policy,
			ExpectedActiveAirID: "none"}
		ctx, resolveErr := l.st.ResolveTelegramActorContext(ev.FromUserID)
		if resolveErr != nil {
			return nil, resolveErr
		}
		current, listErr := l.st.AuthorizedAirListForIdentity(ctx.ActorID,
			store.Identity{Kind: store.IdentityTelegram, TelegramUserID: ev.FromUserID})
		if listErr != nil {
			return nil, listErr
		}
		base.ExpectedActiveAirID = telegramAirExpected(current.CurrentAirID)
		var keyboard bot.InlineKeyboard
		for _, choice := range []struct {
			text   string
			action store.TelegramAirAction
		}{
			{"Join saved / Подтвердить", store.TelegramAirConfirmJoin},
			{"Join & switch / Подтвердить и включить", store.TelegramAirConfirmJoinActivate},
			{"Decline / Отказаться", store.TelegramAirDeclineJoin},
		} {
			binding := base
			binding.Action = choice.action
			token, mintErr := l.st.MintTelegramAirCallback(store.MintTelegramAirCallbackParams{
				TelegramUserID: ev.FromUserID, ChatID: ev.ChatID, MessageID: messageID,
				Binding: binding, Now: time.Now().UnixMilli(),
			})
			if mintErr != nil {
				return nil, mintErr
			}
			keyboard = append(keyboard, []bot.InlineButton{{Text: choice.text, Data: token}})
		}
		return keyboard, nil
	})
}

func (l *loop) replyTelegramAirError(ev bot.Event, err error) {
	switch {
	case errors.Is(err, store.ErrAirForbidden), errors.Is(err, store.ErrUnauthorized),
		errors.Is(err, store.ErrInsufficientCapability), errors.Is(err, store.ErrAirPolicyDenied):
		ev.Reply("Forbidden / Недостаточно прав")
	case errors.Is(err, store.ErrAirInviteUnavailable):
		ev.Reply("Invite expired or used / Приглашение истекло или уже использовано")
	case errors.Is(err, store.ErrAirRevision), errors.Is(err, store.ErrAirActiveChanged),
		errors.Is(err, store.ErrAirConfirmationRequired), errors.Is(err, store.ErrAirMembershipNotFound):
		ev.Reply("Air changed; reopen / Air изменился — открой /air заново")
	case errors.Is(err, store.ErrAirAlreadyMember):
		ev.Reply("Already joined / Этот Барицентр уже участвует")
	case errors.Is(err, store.ErrAirCapacity):
		ev.Reply("Air is full / Air заполнен")
	case errors.Is(err, store.ErrAirRateLimited):
		ev.Reply("Too many attempts / Слишком много попыток")
	default:
		l.log.Error("Telegram Air action failed", "err", err)
		ev.Reply("Air action failed / Действие Air не выполнено")
	}
}

func (l *loop) handleTelegramAirCallback(ev bot.Event, result store.TelegramAirCallbackResult) {
	callback := ev.Callback
	if result.Binding == nil {
		if callback.Answer != nil {
			callback.Answer(telegramCallbackAnswer(result.Outcome))
		}
		if result.ClearKeyboard && callback.ClearKeyboard != nil {
			callback.ClearKeyboard()
		}
		return
	}
	b := *result.Binding
	now := time.Now().UnixMilli()
	auth := telegramAirAuth(store.ActorContext{ActorID: b.ActorID}, ev.FromUserID,
		"callback:"+callback.Data, b, now)
	var err error
	var invite *store.AirInviteIssueResult
	switch b.Action {
	case store.TelegramAirActivate:
		_, err = l.st.ActivateAuthorizedAir(auth, b.AirID, b.MembershipRevision, b.ExpectedActiveAirID)
	case store.TelegramAirDeactivate:
		_, err = l.st.DeactivateAuthorizedAir(auth, b.AirID, b.MembershipRevision, b.ExpectedActiveAirID)
	case store.TelegramAirConfirmJoin, store.TelegramAirConfirmJoinActivate:
		_, err = l.st.ConfirmAuthorizedAirJoin(auth, b.AirID, b.MembershipRevision,
			b.Action == store.TelegramAirConfirmJoinActivate, b.ExpectedActiveAirID)
	case store.TelegramAirDeclineJoin:
		_, err = l.st.DeclineAuthorizedAirJoin(auth, b.AirID, b.MembershipRevision)
	case store.TelegramAirLeave:
		_, err = l.st.LeaveAuthorizedAir(auth, b.AirID, b.MembershipRevision, b.ExpectedActiveAirID)
	case store.TelegramAirDissolve:
		_, err = l.st.DissolveAuthorizedAir(auth, b.AirID, b.AirRevision)
	case store.TelegramAirIssueMember, store.TelegramAirIssueAdmin:
		role := "member"
		if b.Action == store.TelegramAirIssueAdmin {
			role = "admin"
		}
		issued, issueErr := l.st.IssueAuthorizedAirInvite(auth, b.AirID, role)
		err, invite = issueErr, &issued
	case store.TelegramAirWithdrawInvite:
		_, err = l.st.WithdrawAuthorizedAirInvite(auth, b.AirID, b.InviteID, b.InviteRevision)
	case store.TelegramAirPolicyNext:
		_, err = l.st.ReplaceAuthorizedAirPolicy(auth, b.AirID, b.Policy)
	default:
		err = store.ErrAirInvalid
	}
	outcome := store.TelegramCallbackApplied
	if err != nil {
		switch {
		case errors.Is(err, store.ErrAirForbidden), errors.Is(err, store.ErrUnauthorized),
			errors.Is(err, store.ErrInsufficientCapability), errors.Is(err, store.ErrAirPolicyDenied):
			outcome = store.TelegramCallbackForbidden
		case errors.Is(err, store.ErrAirRevision), errors.Is(err, store.ErrAirActiveChanged),
			errors.Is(err, store.ErrAirInviteUnavailable), errors.Is(err, store.ErrAirMembershipNotFound):
			outcome = store.TelegramCallbackTooLate
		default:
			outcome = store.TelegramCallbackFailed
		}
		l.replyTelegramAirError(ev, err)
	} else if invite != nil {
		l.sendTelegramIssuedAirInvite(ev, b.AirID, *invite)
	} else {
		ev.Reply("Air updated / Air обновлён. /air — актуальное состояние.")
		if reconcileErr := l.reconcileAirControlRuntime(); reconcileErr != nil {
			l.log.Error("reconcile Telegram Air runtime", "err", reconcileErr)
		}
	}
	_ = l.st.FinalizeTelegramAirCallback(callback.Data, callback.QueryID, outcome, now)
	if callback.Answer != nil {
		callback.Answer(telegramCallbackAnswer(outcome))
	}
	if callback.ClearKeyboard != nil {
		callback.ClearKeyboard()
	}
}

func (l *loop) sendTelegramIssuedAirInvite(ev bot.Event, airID string, invite store.AirInviteIssueResult) {
	// The secret is a plain, private response. It is never included in inline
	// prompt text, callback_data, durable mutation results or logs.
	ev.Reply(fmt.Sprintf("One-time Air invite (15 min) / Одноразовое приглашение (15 мин):\n<code>/air join %s</code>", esc(invite.Code)))
	if l.telegramInlinePrompt == nil {
		return
	}
	l.telegramInlinePrompt(ev.ChatID, "Invite is active for 15 minutes / Приглашение активно 15 минут.", func(messageID int64) (bot.InlineKeyboard, error) {
		binding := store.TelegramAirBinding{Action: store.TelegramAirWithdrawInvite,
			AirID: airID, InviteID: invite.InviteID, InviteRevision: invite.Revision}
		token, err := l.st.MintTelegramAirCallback(store.MintTelegramAirCallbackParams{
			TelegramUserID: ev.FromUserID, ChatID: ev.ChatID, MessageID: messageID,
			Binding: binding, Now: time.Now().UnixMilli(),
		})
		if err != nil {
			return nil, err
		}
		return bot.InlineKeyboard{{{Text: "Withdraw / Отозвать", Data: token}}}, nil
	})
}
