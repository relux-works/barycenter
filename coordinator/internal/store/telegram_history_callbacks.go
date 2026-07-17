package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"time"
)

type TelegramHistoryAction string

const (
	TelegramHistoryReplay     TelegramHistoryAction = "replay"
	TelegramHistoryDelete     TelegramHistoryAction = "delete"
	TelegramHistoryReport     TelegramHistoryAction = "report"
	TelegramHistoryBlockActor TelegramHistoryAction = "block_actor"
)

type MintTelegramHistoryCallbackParams struct {
	TelegramUserID int64
	HistoryItemID  string
	ChatID         int64
	MessageID      int64
	Action         TelegramHistoryAction
	Reason         ModerationReason
	Now            int64
}

type ClaimTelegramHistoryCallbackParams struct {
	TelegramUserID int64
	QueryID        string
	Token          string
	ChatID         int64
	MessageID      int64
	Now            int64
}

type TelegramHistoryCallbackClaim struct {
	HistoryItemID string
	ActorID       int64
	OrbitID       int64
	Role          string
	Action        TelegramHistoryAction
	Reason        ModerationReason
}

type TelegramHistoryCallbackResult struct {
	Found         bool
	Replay        bool
	ClearKeyboard bool
	Outcome       TelegramCallbackOutcome
	ActionOutcome string
	Claim         *TelegramHistoryCallbackClaim
}

type FinalizeTelegramHistoryCallbackParams struct {
	TelegramUserID int64
	QueryID        string
	Token          string
	ChatID         int64
	MessageID      int64
	Claim          TelegramHistoryCallbackClaim
	Outcome        TelegramCallbackOutcome
	ActionOutcome  string
	Consume        bool
	ClearKeyboard  bool
	Now            int64
}

func validTelegramHistoryAction(action TelegramHistoryAction, reason ModerationReason) bool {
	switch action {
	case TelegramHistoryReplay, TelegramHistoryDelete, TelegramHistoryBlockActor:
		return reason == ""
	case TelegramHistoryReport:
		return validModerationReason(reason)
	default:
		return false
	}
}

func telegramHistoryActionAllowed(item HistoryQueryItem, action TelegramHistoryAction) bool {
	switch action {
	case TelegramHistoryReplay:
		return item.CanReplay
	case TelegramHistoryDelete:
		return item.CanDelete
	case TelegramHistoryReport:
		return item.CanReport
	case TelegramHistoryBlockActor:
		return item.CanBlockActor
	default:
		return false
	}
}

func validTelegramHistoryOutcome(outcome TelegramCallbackOutcome) bool {
	switch outcome {
	case TelegramCallbackApplied, TelegramCallbackAlreadyApplied,
		TelegramCallbackTooLate, TelegramCallbackExpired, TelegramCallbackForbidden,
		TelegramCallbackUnsupported, TelegramCallbackFailed:
		return true
	default:
		return false
	}
}

func validTelegramHistoryActionOutcome(outcome string) bool {
	switch outcome {
	case "", "media_deleted", "report_received", "report_already_received",
		"sender_blocked", "sender_already_blocked", "replay_accepted",
		"replay_already_accepted", "history_action_unavailable":
		return true
	default:
		return false
	}
}

func validTelegramQueryID(value string) bool {
	return value != "" && len(value) <= 256 && !strings.ContainsAny(value, "\x00\r\n")
}

// MintTelegramHistoryCallback creates an opaque, initiator-only capability for
// one currently visible history action. The mutation owner reauthorizes it
// again when clicked; this projection check only prevents rendering a control
// that is already unavailable.
func (s *Store) MintTelegramHistoryCallback(params MintTelegramHistoryCallbackParams) (string, error) {
	if params.TelegramUserID <= 0 || params.ChatID == 0 || params.MessageID <= 0 ||
		params.Now <= 0 || !validTelegramHistoryAction(params.Action, params.Reason) {
		return "", ErrTransmissionInvalid
	}
	ctx, err := s.ResolveTelegramActorContext(params.TelegramUserID)
	if err != nil {
		return "", err
	}
	identity := Identity{Kind: IdentityTelegram, TelegramUserID: params.TelegramUserID}
	item, err := s.GetAuthorizedHistoryItem(ctx.ActorID, identity, params.HistoryItemID, params.Now)
	if err != nil {
		return "", err
	}
	if !telegramHistoryActionAllowed(item, params.Action) {
		return "", ErrTransmissionStateConflict
	}

	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	current, err := resolveActorContext(tx, identity)
	if err != nil || current.ActorID != ctx.ActorID || current.OrbitID != ctx.OrbitID ||
		current.Role != ctx.Role {
		if err == nil {
			err = ErrUnauthorized
		}
		return "", err
	}
	key, err := telegramCallbackKeyTx(tx)
	if err != nil {
		return "", err
	}
	for attempts := 0; attempts < 4; attempts++ {
		raw := make([]byte, 24)
		if _, err := rand.Read(raw); err != nil {
			return "", err
		}
		token := telegramCallbackPrefix + base64.RawURLEncoding.EncodeToString(raw)
		if !telegramCallbackPattern.MatchString(token) {
			continue
		}
		hash := telegramKeyedDigest(key, "token", token)
		var conflicts int
		if err := tx.QueryRow(`SELECT
  (SELECT COUNT(*) FROM telegram_inline_callbacks WHERE token_hash = ?) +
  (SELECT COUNT(*) FROM telegram_history_callbacks WHERE token_hash = ?) +
  (SELECT COUNT(*) FROM telegram_air_callbacks WHERE token_hash = ?) +
  (SELECT COUNT(*) FROM telegram_automation_callbacks WHERE token_hash = ?)`,
			hash, hash, hash, hash).Scan(&conflicts); err != nil {
			return "", err
		}
		if conflicts != 0 {
			continue
		}
		_, err = tx.Exec(`INSERT INTO telegram_history_callbacks(
  token_hash, history_item_id, actor_id, orbit_id, role, chat_id, message_id,
  action, reason, created_at, expires_at) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			hash, params.HistoryItemID, current.ActorID, current.OrbitID, current.Role,
			params.ChatID, params.MessageID, params.Action, params.Reason, params.Now,
			params.Now+int64(telegramCallbackTTL/time.Millisecond))
		if err != nil {
			return "", err
		}
		if err := tx.Commit(); err != nil {
			return "", err
		}
		return token, nil
	}
	return "", errors.New("Telegram callback token collision")
}

func cacheTelegramHistoryQueryTx(
	tx *sql.Tx,
	key []byte,
	params ClaimTelegramHistoryCallbackParams,
	ctx ActorContext,
	result TelegramHistoryCallbackResult,
) error {
	if !validTelegramQueryID(params.QueryID) {
		return nil
	}
	_, err := tx.Exec(`INSERT OR REPLACE INTO telegram_history_callback_queries(
  query_hash, actor_id, orbit_id, role, chat_id, message_id, callback_outcome,
  action_outcome, clear_keyboard, created_at, expires_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		telegramKeyedDigest(key, "query", params.QueryID), ctx.ActorID, ctx.OrbitID,
		ctx.Role, params.ChatID, params.MessageID, result.Outcome, result.ActionOutcome,
		result.ClearKeyboard, params.Now, params.Now+int64(telegramQueryTTL/time.Millisecond))
	return err
}

func loadTelegramHistoryQueryTx(
	tx *sql.Tx,
	key []byte,
	params ClaimTelegramHistoryCallbackParams,
	ctx ActorContext,
) (TelegramHistoryCallbackResult, bool, error) {
	if !validTelegramQueryID(params.QueryID) {
		return TelegramHistoryCallbackResult{}, false, nil
	}
	var actorID, orbitID, chatID, messageID, expiresAt int64
	var role, actionOutcome string
	var outcome TelegramCallbackOutcome
	var clear bool
	err := tx.QueryRow(`SELECT actor_id, orbit_id, role, chat_id, message_id,
       callback_outcome, action_outcome, clear_keyboard, expires_at
FROM telegram_history_callback_queries WHERE query_hash = ?`,
		telegramKeyedDigest(key, "query", params.QueryID)).Scan(
		&actorID, &orbitID, &role, &chatID, &messageID, &outcome, &actionOutcome,
		&clear, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		return TelegramHistoryCallbackResult{}, false, nil
	}
	if err != nil {
		return TelegramHistoryCallbackResult{}, false, err
	}
	if expiresAt <= params.Now {
		return TelegramHistoryCallbackResult{}, false, nil
	}
	if actorID != ctx.ActorID || orbitID != ctx.OrbitID || role != ctx.Role ||
		chatID != params.ChatID || messageID != params.MessageID {
		return TelegramHistoryCallbackResult{
			Found: true, Outcome: TelegramCallbackForbidden,
		}, true, nil
	}
	return TelegramHistoryCallbackResult{
		Found: true, Replay: true, ClearKeyboard: clear,
		Outcome: outcome, ActionOutcome: actionOutcome,
	}, true, nil
}

// ClaimTelegramHistoryCallback authenticates the opaque transport capability.
// Unknown tokens intentionally return Found=false so the caller can try the
// delivery callback namespace; a wholly forged token eventually collapses to
// the shared expired result without revealing which namespace was consulted.
func (s *Store) ClaimTelegramHistoryCallback(params ClaimTelegramHistoryCallbackParams) (TelegramHistoryCallbackResult, error) {
	if params.TelegramUserID <= 0 || params.ChatID == 0 || params.MessageID <= 0 || params.Now <= 0 {
		return TelegramHistoryCallbackResult{}, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return TelegramHistoryCallbackResult{}, err
	}
	defer tx.Rollback()
	identity := Identity{Kind: IdentityTelegram, TelegramUserID: params.TelegramUserID}
	ctx, err := resolveActorContext(tx, identity)
	if err != nil {
		return TelegramHistoryCallbackResult{}, err
	}
	key, err := telegramCallbackKeyTx(tx)
	if err != nil {
		return TelegramHistoryCallbackResult{}, err
	}
	if cached, ok, err := loadTelegramHistoryQueryTx(tx, key, params, ctx); err != nil || ok {
		return cached, err
	}
	if !telegramCallbackPattern.MatchString(params.Token) {
		return TelegramHistoryCallbackResult{}, nil
	}
	var claim TelegramHistoryCallbackClaim
	var actorID, orbitID, chatID, messageID, expiresAt, consumedAt int64
	var role, actionOutcome string
	var storedOutcome TelegramCallbackOutcome
	err = tx.QueryRow(`SELECT history_item_id, actor_id, orbit_id, role, chat_id,
       message_id, action, reason, expires_at, consumed_at, callback_outcome,
       action_outcome FROM telegram_history_callbacks WHERE token_hash = ?`,
		telegramKeyedDigest(key, "token", params.Token)).Scan(
		&claim.HistoryItemID, &actorID, &orbitID, &role, &chatID, &messageID,
		&claim.Action, &claim.Reason, &expiresAt, &consumedAt, &storedOutcome,
		&actionOutcome)
	if errors.Is(err, sql.ErrNoRows) {
		return TelegramHistoryCallbackResult{}, nil
	}
	if err != nil {
		return TelegramHistoryCallbackResult{}, err
	}
	result := TelegramHistoryCallbackResult{Found: true}
	switch {
	case actorID != ctx.ActorID || orbitID != ctx.OrbitID || role != ctx.Role ||
		chatID != params.ChatID || messageID != params.MessageID:
		result.Outcome = TelegramCallbackForbidden
	case expiresAt <= params.Now:
		result.Outcome, result.ClearKeyboard = TelegramCallbackExpired, true
	case consumedAt != 0:
		result.Outcome, result.ActionOutcome = TelegramCallbackAlreadyApplied, actionOutcome
		result.ClearKeyboard = true
	default:
		claim.ActorID, claim.OrbitID, claim.Role = actorID, orbitID, role
		result.Claim = &claim
		return result, tx.Commit()
	}
	if err := cacheTelegramHistoryQueryTx(tx, key, params, ctx, result); err != nil {
		return TelegramHistoryCallbackResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return TelegramHistoryCallbackResult{}, err
	}
	return result, nil
}

func (s *Store) FinalizeTelegramHistoryCallback(params FinalizeTelegramHistoryCallbackParams) (TelegramHistoryCallbackResult, error) {
	if params.Now <= 0 || !validTelegramQueryID(params.QueryID) ||
		!validTelegramHistoryOutcome(params.Outcome) ||
		!validTelegramHistoryActionOutcome(params.ActionOutcome) {
		return TelegramHistoryCallbackResult{}, ErrTransmissionInvalid
	}
	claimParams := ClaimTelegramHistoryCallbackParams{
		TelegramUserID: params.TelegramUserID, QueryID: params.QueryID,
		Token: params.Token, ChatID: params.ChatID, MessageID: params.MessageID, Now: params.Now,
	}
	tx, err := s.db.Begin()
	if err != nil {
		return TelegramHistoryCallbackResult{}, err
	}
	defer tx.Rollback()
	identity := Identity{Kind: IdentityTelegram, TelegramUserID: params.TelegramUserID}
	ctx, err := resolveActorContext(tx, identity)
	if err != nil {
		return TelegramHistoryCallbackResult{}, err
	}
	key, err := telegramCallbackKeyTx(tx)
	if err != nil {
		return TelegramHistoryCallbackResult{}, err
	}
	if cached, ok, err := loadTelegramHistoryQueryTx(tx, key, claimParams, ctx); err != nil || ok {
		return cached, err
	}
	if !telegramCallbackPattern.MatchString(params.Token) {
		return TelegramHistoryCallbackResult{}, ErrTransmissionInvalid
	}
	hash := telegramKeyedDigest(key, "token", params.Token)
	var historyID, role, actionOutcome string
	var action TelegramHistoryAction
	var reason ModerationReason
	var actorID, orbitID, chatID, messageID, expiresAt, consumedAt int64
	err = tx.QueryRow(`SELECT history_item_id, actor_id, orbit_id, role, chat_id,
       message_id, action, reason, expires_at, consumed_at, action_outcome
FROM telegram_history_callbacks WHERE token_hash = ?`, hash).Scan(
		&historyID, &actorID, &orbitID, &role, &chatID, &messageID, &action,
		&reason, &expiresAt, &consumedAt, &actionOutcome)
	if err != nil {
		return TelegramHistoryCallbackResult{}, err
	}
	result := TelegramHistoryCallbackResult{Found: true}
	if actorID != ctx.ActorID || orbitID != ctx.OrbitID || role != ctx.Role ||
		chatID != params.ChatID || messageID != params.MessageID ||
		historyID != params.Claim.HistoryItemID || action != params.Claim.Action ||
		reason != params.Claim.Reason {
		result.Outcome = TelegramCallbackForbidden
	} else if expiresAt <= params.Now {
		result.Outcome, result.ClearKeyboard = TelegramCallbackExpired, true
	} else if consumedAt != 0 {
		result.Outcome, result.ActionOutcome = TelegramCallbackAlreadyApplied, actionOutcome
		result.ClearKeyboard = true
	} else {
		result.Outcome, result.ActionOutcome = params.Outcome, params.ActionOutcome
		result.ClearKeyboard = params.ClearKeyboard
		if params.Consume {
			result.ClearKeyboard = true
			_, err = tx.Exec(`UPDATE telegram_history_callbacks SET consumed_at = ?,
  callback_outcome = ?, action_outcome = ? WHERE token_hash = ? AND consumed_at = 0`,
				params.Now, params.Outcome, params.ActionOutcome, hash)
			if err != nil {
				return TelegramHistoryCallbackResult{}, err
			}
		}
	}
	if err := cacheTelegramHistoryQueryTx(tx, key, claimParams, ctx, result); err != nil {
		return TelegramHistoryCallbackResult{}, err
	}
	if _, err := tx.Exec(`DELETE FROM telegram_history_callback_queries WHERE expires_at <= ?`, params.Now); err != nil {
		return TelegramHistoryCallbackResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return TelegramHistoryCallbackResult{}, err
	}
	return result, nil
}
