package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
)

type TelegramAirAction string

const (
	TelegramAirActivate            TelegramAirAction = "activate"
	TelegramAirDeactivate          TelegramAirAction = "deactivate"
	TelegramAirConfirmJoin         TelegramAirAction = "confirm_join"
	TelegramAirConfirmJoinActivate TelegramAirAction = "confirm_join_activate"
	TelegramAirDeclineJoin         TelegramAirAction = "decline_join"
	TelegramAirLeave               TelegramAirAction = "leave"
	TelegramAirDissolve            TelegramAirAction = "dissolve"
	TelegramAirIssueMember         TelegramAirAction = "issue_member"
	TelegramAirIssueAdmin          TelegramAirAction = "issue_admin"
	TelegramAirWithdrawInvite      TelegramAirAction = "withdraw_invite"
	TelegramAirPolicyNext          TelegramAirAction = "policy_next"
)

type TelegramAirBinding struct {
	ActorID, OrbitID                int64
	Role                            string
	Action                          TelegramAirAction
	AirID, MembershipID, InviteID   string
	AirRevision, MembershipRevision int64
	InviteRevision                  int64
	ExpectedActiveAirID             string
	Policy                          AirPolicyView
}

type MintTelegramAirCallbackParams struct {
	TelegramUserID, ChatID, MessageID int64
	Binding                           TelegramAirBinding
	Now                               int64
}

type ClaimTelegramAirCallbackParams struct {
	TelegramUserID, ChatID, MessageID int64
	QueryID, Token                    string
	Now                               int64
}

type TelegramAirCallbackResult struct {
	Found, Replay, ClearKeyboard bool
	Outcome                      TelegramCallbackOutcome
	Binding                      *TelegramAirBinding
}

func validTelegramAirAction(action TelegramAirAction) bool {
	switch action {
	case TelegramAirActivate, TelegramAirDeactivate, TelegramAirConfirmJoin,
		TelegramAirConfirmJoinActivate, TelegramAirDeclineJoin, TelegramAirLeave,
		TelegramAirDissolve, TelegramAirIssueMember, TelegramAirIssueAdmin,
		TelegramAirWithdrawInvite, TelegramAirPolicyNext:
		return true
	default:
		return false
	}
}

// MintTelegramAirCallback snapshots the visible lifecycle revisions but never
// serializes them into callback_data. The canonical Air mutation rechecks all
// authority and revisions again when the button is clicked.
func (s *Store) MintTelegramAirCallback(params MintTelegramAirCallbackParams) (string, error) {
	if params.TelegramUserID <= 0 || params.ChatID == 0 || params.MessageID <= 0 ||
		params.Now <= 0 || !validTelegramAirAction(params.Binding.Action) ||
		!airPolicyAirIDPattern.MatchString(params.Binding.AirID) {
		return "", ErrAirInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	ctx, err := resolveActorContext(tx, Identity{Kind: IdentityTelegram, TelegramUserID: params.TelegramUserID})
	if err != nil {
		return "", err
	}
	projection, err := airProjectionTx(tx, params.Binding.AirID, ctx.OrbitID)
	if err != nil {
		return "", err
	}
	if params.Binding.MembershipID != "" && params.Binding.MembershipID != projection.MembershipID {
		return "", ErrAirForbidden
	}
	params.Binding.ActorID, params.Binding.OrbitID, params.Binding.Role = ctx.ActorID, ctx.OrbitID, ctx.Role
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
		b := params.Binding
		_, err = tx.Exec(`INSERT INTO telegram_air_callbacks(
  token_hash, actor_id, orbit_id, role, chat_id, message_id, action, air_id,
  membership_id, invite_id, air_revision, membership_revision, invite_revision,
  expected_active_air_id, policy_revision, invite_policy, overlay_policy,
  queue_policy, replace_policy, created_at, expires_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			hash, b.ActorID, b.OrbitID, b.Role, params.ChatID, params.MessageID, b.Action,
			b.AirID, b.MembershipID, b.InviteID, b.AirRevision, b.MembershipRevision,
			b.InviteRevision, b.ExpectedActiveAirID, b.Policy.Revision, b.Policy.Invite,
			b.Policy.Overlay, b.Policy.Queue, b.Policy.Replace, params.Now,
			params.Now+telegramCallbackTTL.Milliseconds())
		if err != nil {
			return "", err
		}
		if err := tx.Commit(); err != nil {
			return "", err
		}
		return token, nil
	}
	return "", errors.New("Telegram Air callback token collision")
}

// ClaimTelegramAirCallback atomically consumes one opaque capability. Query-id
// fencing prevents one Telegram click from being replayed with another token;
// stale independent buttons are rejected by the canonical Air revisions.
func (s *Store) ClaimTelegramAirCallback(params ClaimTelegramAirCallbackParams) (TelegramAirCallbackResult, error) {
	if params.TelegramUserID <= 0 || params.ChatID == 0 || params.MessageID <= 0 || params.Now <= 0 {
		return TelegramAirCallbackResult{}, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return TelegramAirCallbackResult{}, err
	}
	defer tx.Rollback()
	ctx, err := resolveActorContext(tx, Identity{Kind: IdentityTelegram, TelegramUserID: params.TelegramUserID})
	if err != nil {
		return TelegramAirCallbackResult{}, err
	}
	key, err := telegramCallbackKeyTx(tx)
	if err != nil {
		return TelegramAirCallbackResult{}, err
	}
	queryHash := ""
	if validTelegramQueryID(params.QueryID) {
		queryHash = telegramKeyedDigest(key, "query", params.QueryID)
		var actorID, orbitID, chatID, messageID, expiresAt int64
		var role string
		var outcome TelegramCallbackOutcome
		err = tx.QueryRow(`SELECT actor_id, orbit_id, role, chat_id, message_id, outcome, expires_at
FROM telegram_air_callback_queries WHERE query_hash = ?`, queryHash).Scan(
			&actorID, &orbitID, &role, &chatID, &messageID, &outcome, &expiresAt)
		if err == nil && expiresAt > params.Now {
			if actorID != ctx.ActorID || orbitID != ctx.OrbitID || role != ctx.Role ||
				chatID != params.ChatID || messageID != params.MessageID {
				outcome = TelegramCallbackForbidden
			}
			return TelegramAirCallbackResult{Found: true, Replay: true, ClearKeyboard: true, Outcome: outcome}, tx.Commit()
		}
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return TelegramAirCallbackResult{}, err
		}
	}
	if !telegramCallbackPattern.MatchString(params.Token) {
		return TelegramAirCallbackResult{}, nil
	}
	hash := telegramKeyedDigest(key, "token", params.Token)
	var b TelegramAirBinding
	var actorID, orbitID, chatID, messageID, expiresAt, consumedAt int64
	var storedOutcome TelegramCallbackOutcome
	err = tx.QueryRow(`SELECT actor_id, orbit_id, role, chat_id, message_id, action,
  air_id, membership_id, invite_id, air_revision, membership_revision,
  invite_revision, expected_active_air_id, policy_revision, invite_policy,
  overlay_policy, queue_policy, replace_policy, expires_at, consumed_at, outcome
FROM telegram_air_callbacks WHERE token_hash = ?`, hash).Scan(
		&actorID, &orbitID, &b.Role, &chatID, &messageID, &b.Action, &b.AirID,
		&b.MembershipID, &b.InviteID, &b.AirRevision, &b.MembershipRevision,
		&b.InviteRevision, &b.ExpectedActiveAirID, &b.Policy.Revision,
		&b.Policy.Invite, &b.Policy.Overlay, &b.Policy.Queue, &b.Policy.Replace,
		&expiresAt, &consumedAt, &storedOutcome)
	if errors.Is(err, sql.ErrNoRows) {
		return TelegramAirCallbackResult{}, nil
	}
	if err != nil {
		return TelegramAirCallbackResult{}, err
	}
	result := TelegramAirCallbackResult{Found: true, ClearKeyboard: true}
	switch {
	case actorID != ctx.ActorID || orbitID != ctx.OrbitID || b.Role != ctx.Role ||
		chatID != params.ChatID || messageID != params.MessageID:
		result.Outcome = TelegramCallbackForbidden
	case expiresAt <= params.Now:
		result.Outcome = TelegramCallbackExpired
	case consumedAt != 0:
		result.Outcome = TelegramCallbackAlreadyApplied
	default:
		updated, err := tx.Exec(`UPDATE telegram_air_callbacks
SET consumed_at = ?, outcome = 'claimed' WHERE token_hash = ? AND consumed_at = 0`, params.Now, hash)
		if err != nil {
			return TelegramAirCallbackResult{}, err
		}
		changed, err := updated.RowsAffected()
		if err != nil {
			return TelegramAirCallbackResult{}, err
		}
		if changed == 1 {
			b.ActorID, b.OrbitID = actorID, orbitID
			result.Outcome, result.Binding = TelegramCallbackApplied, &b
		} else {
			result.Outcome = TelegramCallbackAlreadyApplied
		}
	}
	if queryHash != "" {
		_, err = tx.Exec(`INSERT OR IGNORE INTO telegram_air_callback_queries(
  query_hash, actor_id, orbit_id, role, chat_id, message_id, outcome, created_at, expires_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`, queryHash, ctx.ActorID, ctx.OrbitID, ctx.Role,
			params.ChatID, params.MessageID, result.Outcome, params.Now,
			params.Now+telegramQueryTTL.Milliseconds())
		if err != nil {
			return TelegramAirCallbackResult{}, err
		}
	}
	if err := tx.Commit(); err != nil {
		return TelegramAirCallbackResult{}, err
	}
	return result, nil
}

func (s *Store) FinalizeTelegramAirCallback(token, queryID string, outcome TelegramCallbackOutcome, now int64) error {
	if !telegramCallbackPattern.MatchString(token) || now <= 0 {
		return ErrAirInvalid
	}
	key, err := telegramCallbackKeyTxMustCommit(s)
	if err != nil {
		return err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec(`UPDATE telegram_air_callbacks SET outcome = ?
WHERE token_hash = ? AND consumed_at <> 0`, outcome, telegramKeyedDigest(key, "token", token)); err != nil {
		return err
	}
	if validTelegramQueryID(queryID) {
		if _, err = tx.Exec(`UPDATE telegram_air_callback_queries SET outcome = ?
WHERE query_hash = ?`, outcome, telegramKeyedDigest(key, "query", queryID)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// The callback key normally lives inside a caller transaction. Finalization is
// deliberately a separate best-effort status update after the Air transaction.
func telegramCallbackKeyTxMustCommit(s *Store) ([]byte, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	key, err := telegramCallbackKeyTx(tx)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return key, nil
}
