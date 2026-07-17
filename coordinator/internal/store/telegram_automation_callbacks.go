package store

import (
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"fmt"
)

type TelegramAutomationAction string

const (
	TelegramAutomationCueSelect        TelegramAutomationAction = "cue_select"
	TelegramAutomationTrigger          TelegramAutomationAction = "trigger"
	TelegramAutomationScheduleEnable   TelegramAutomationAction = "schedule_enable"
	TelegramAutomationScheduleDisable  TelegramAutomationAction = "schedule_disable"
	TelegramAutomationEmergencyDisable TelegramAutomationAction = "emergency_disable"
)

type TelegramAutomationBinding struct {
	ActorID, OrbitID    int64
	Role                string
	Action              TelegramAutomationAction
	CueID               string
	CueRevision         int64
	CueSourceGeneration int64
	Audience            TransmissionAudienceKind
	TargetReference     string
	IncludeOrigin       bool
	Delivery            TransmissionDelivery
	ScheduleID          string
	ScheduleRevision    int64
	FeatureRevision     int64
}

type MintTelegramAutomationCallbackParams struct {
	TelegramUserID, ChatID, MessageID int64
	Binding                           TelegramAutomationBinding
	Now                               int64
}

type ClaimTelegramAutomationCallbackParams struct {
	TelegramUserID, ChatID, MessageID int64
	QueryID, Token                    string
	Now                               int64
}

type TelegramAutomationCallbackResult struct {
	Found, Replay, ClearKeyboard bool
	Outcome                      TelegramCallbackOutcome
	Binding                      *TelegramAutomationBinding
}

func validTelegramAutomationBinding(binding TelegramAutomationBinding) bool {
	switch binding.Action {
	case TelegramAutomationCueSelect:
		return savedCueIDPattern.MatchString(binding.CueID) && binding.Audience == "" &&
			binding.Delivery == "" && binding.TargetReference == ""
	case TelegramAutomationTrigger:
		if !savedCueIDPattern.MatchString(binding.CueID) ||
			(binding.Delivery != TransmissionDeliveryOverlay &&
				binding.Delivery != TransmissionDeliveryAfterCurrent) {
			return false
		}
		switch binding.Audience {
		case TransmissionAudienceThisPulsar, TransmissionAudienceOwnBarycenter,
			TransmissionAudienceCurrentAir:
			return binding.TargetReference == ""
		case TransmissionAudienceExplicit:
			return transmissionTargetReferencePattern.MatchString(binding.TargetReference)
		default:
			return false
		}
	case TelegramAutomationScheduleEnable, TelegramAutomationScheduleDisable:
		return automationScheduleIDPattern.MatchString(binding.ScheduleID)
	case TelegramAutomationEmergencyDisable:
		return binding.CueID == "" && binding.ScheduleID == ""
	default:
		return false
	}
}

func authorizeTelegramAutomationTx(tx *sql.Tx, telegramUserID int64) (ActorContext, Identity, error) {
	identity := Identity{Kind: IdentityTelegram, TelegramUserID: telegramUserID}
	ctx, err := resolveActorContext(tx, identity)
	if err != nil {
		return ActorContext{}, Identity{}, err
	}
	if ctx.Role != "primary" || (!ctx.Capabilities.Has(CapabilityControl) &&
		!ctx.Capabilities.Has(CapabilityTelegram)) {
		return ActorContext{}, Identity{}, ErrInsufficientCapability
	}
	return ctx, identity, nil
}

// MintTelegramAutomationCallback snapshots only presentation revisions. The
// opaque token is bound to the current primary, chat and message; execution
// re-runs the canonical soundboard or automation mutation against live state.
func (s *Store) MintTelegramAutomationCallback(params MintTelegramAutomationCallbackParams) (string, error) {
	if params.TelegramUserID <= 0 || params.ChatID == 0 || params.MessageID <= 0 ||
		params.Now <= 0 || !validTelegramAutomationBinding(params.Binding) {
		return "", ErrAutomationInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	ctx, identity, err := authorizeTelegramAutomationTx(tx, params.TelegramUserID)
	if err != nil {
		return "", err
	}
	binding := params.Binding
	binding.ActorID, binding.OrbitID, binding.Role = ctx.ActorID, ctx.OrbitID, ctx.Role
	switch binding.Action {
	case TelegramAutomationCueSelect, TelegramAutomationTrigger:
		cue, err := savedCueOwnedActiveTx(tx, binding.CueID, ctx.OrbitID)
		if err != nil {
			return "", err
		}
		binding.CueRevision, binding.CueSourceGeneration = cue.Revision, cue.SourceGeneration
		var feature AutomationFeatureState
		feature, err = scanAutomationFeatureState(tx.QueryRow(`SELECT `+automationFeatureColumns+`
FROM automation_feature_state WHERE owner_orbit_id = ?`, ctx.OrbitID))
		if errors.Is(err, sql.ErrNoRows) || (err == nil && !feature.SoundboardEnabled) {
			return "", ErrAutomationDisabled
		}
		if err != nil {
			return "", err
		}
		binding.FeatureRevision = feature.Revision
		if binding.Audience == TransmissionAudienceExplicit {
			allowed, err := targetReferenceDomainTx(tx, ctx)
			if err != nil {
				return "", err
			}
			if _, err := resolveTransmissionTargetReferenceTx(tx, ctx, identity,
				binding.TargetReference, params.Now, allowed); err != nil {
				return "", ErrAutomationAudienceNotAllowed
			}
		}
	case TelegramAutomationScheduleEnable, TelegramAutomationScheduleDisable:
		schedule, err := automationScheduleOwnedControlTx(tx, binding.ScheduleID, ctx.OrbitID)
		if err != nil {
			return "", err
		}
		binding.ScheduleRevision = schedule.Revision
	case TelegramAutomationEmergencyDisable:
		feature, err := scanAutomationFeatureState(tx.QueryRow(`SELECT `+automationFeatureColumns+`
FROM automation_feature_state WHERE owner_orbit_id = ?`, ctx.OrbitID))
		if errors.Is(err, sql.ErrNoRows) {
			binding.FeatureRevision = 0
		} else if err != nil {
			return "", err
		} else {
			binding.FeatureRevision = feature.Revision
		}
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
		_, err = tx.Exec(`INSERT INTO telegram_automation_callbacks(
  token_hash, actor_id, orbit_id, role, chat_id, message_id, action, cue_id,
  cue_revision, cue_source_generation, audience_kind, target_reference,
  include_origin, delivery, schedule_id, schedule_revision, feature_revision,
  created_at, expires_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, hash,
			binding.ActorID, binding.OrbitID, binding.Role, params.ChatID, params.MessageID,
			binding.Action, binding.CueID, binding.CueRevision, binding.CueSourceGeneration,
			binding.Audience, binding.TargetReference, boolInt(binding.IncludeOrigin),
			binding.Delivery, binding.ScheduleID, binding.ScheduleRevision,
			binding.FeatureRevision, params.Now, params.Now+telegramCallbackTTL.Milliseconds())
		if err != nil {
			return "", err
		}
		if err := tx.Commit(); err != nil {
			return "", err
		}
		return token, nil
	}
	return "", errors.New("Telegram automation callback token collision")
}

func scanTelegramAutomationBinding(row sqlScanner) (TelegramAutomationBinding, int64, int64, TelegramCallbackOutcome, error) {
	var binding TelegramAutomationBinding
	var includeOrigin int
	var expiresAt, consumedAt int64
	var outcome TelegramCallbackOutcome
	err := row.Scan(&binding.ActorID, &binding.OrbitID, &binding.Role, &binding.Action,
		&binding.CueID, &binding.CueRevision, &binding.CueSourceGeneration,
		&binding.Audience, &binding.TargetReference, &includeOrigin, &binding.Delivery,
		&binding.ScheduleID, &binding.ScheduleRevision, &binding.FeatureRevision,
		&expiresAt, &consumedAt, &outcome)
	binding.IncludeOrigin = includeOrigin != 0
	return binding, expiresAt, consumedAt, outcome, err
}

func (s *Store) ClaimTelegramAutomationCallback(params ClaimTelegramAutomationCallbackParams) (TelegramAutomationCallbackResult, error) {
	if params.TelegramUserID <= 0 || params.ChatID == 0 || params.MessageID <= 0 ||
		params.Now <= 0 || !validTelegramQueryID(params.QueryID) {
		return TelegramAutomationCallbackResult{}, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return TelegramAutomationCallbackResult{}, err
	}
	defer tx.Rollback()
	ctx, err := resolveActorContext(tx, Identity{Kind: IdentityTelegram,
		TelegramUserID: params.TelegramUserID})
	if err != nil {
		return TelegramAutomationCallbackResult{}, err
	}
	key, err := telegramCallbackKeyTx(tx)
	if err != nil {
		return TelegramAutomationCallbackResult{}, err
	}
	queryHash := telegramKeyedDigest(key, "query", params.QueryID)
	var queryActor, queryOrbit, queryChat, queryMessage, queryExpires int64
	var queryRole string
	var queryOutcome TelegramCallbackOutcome
	err = tx.QueryRow(`SELECT actor_id, orbit_id, role, chat_id, message_id,
outcome, expires_at FROM telegram_automation_callback_queries WHERE query_hash = ?`,
		queryHash).Scan(&queryActor, &queryOrbit, &queryRole, &queryChat, &queryMessage,
		&queryOutcome, &queryExpires)
	if err == nil && queryExpires > params.Now {
		if queryActor != ctx.ActorID || queryOrbit != ctx.OrbitID || queryRole != ctx.Role ||
			queryChat != params.ChatID || queryMessage != params.MessageID {
			queryOutcome = TelegramCallbackForbidden
		}
		return TelegramAutomationCallbackResult{Found: true, Replay: true,
			ClearKeyboard: true, Outcome: queryOutcome}, tx.Commit()
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return TelegramAutomationCallbackResult{}, err
	}
	if !telegramCallbackPattern.MatchString(params.Token) {
		return TelegramAutomationCallbackResult{}, nil
	}
	tokenHash := telegramKeyedDigest(key, "token", params.Token)
	var chatID, messageID int64
	binding, expiresAt, consumedAt, storedOutcome, err := scanTelegramAutomationBinding(tx.QueryRow(`SELECT
actor_id, orbit_id, role, action, cue_id, cue_revision, cue_source_generation,
audience_kind, target_reference, include_origin, delivery, schedule_id,
schedule_revision, feature_revision, expires_at, consumed_at, outcome
FROM telegram_automation_callbacks WHERE token_hash = ?`, tokenHash))
	if errors.Is(err, sql.ErrNoRows) {
		return TelegramAutomationCallbackResult{}, nil
	}
	if err != nil {
		return TelegramAutomationCallbackResult{}, err
	}
	if err := tx.QueryRow(`SELECT chat_id, message_id FROM telegram_automation_callbacks
WHERE token_hash = ?`, tokenHash).Scan(&chatID, &messageID); err != nil {
		return TelegramAutomationCallbackResult{}, err
	}
	result := TelegramAutomationCallbackResult{Found: true, ClearKeyboard: true}
	switch {
	case ctx.Role != "primary" || (!ctx.Capabilities.Has(CapabilityControl) &&
		!ctx.Capabilities.Has(CapabilityTelegram)) ||
		binding.ActorID != ctx.ActorID || binding.OrbitID != ctx.OrbitID ||
		binding.Role != ctx.Role || chatID != params.ChatID || messageID != params.MessageID:
		result.Outcome = TelegramCallbackForbidden
	case expiresAt <= params.Now:
		result.Outcome = TelegramCallbackExpired
	case consumedAt != 0:
		result.Outcome = storedOutcome
		if result.Outcome == "" || result.Outcome == TelegramCallbackApplied {
			result.Outcome = TelegramCallbackAlreadyApplied
		}
	default:
		result.Outcome = TelegramCallbackApplied
		result.Binding = &binding
		if _, err := tx.Exec(`UPDATE telegram_automation_callbacks SET
consumed_at = ?, outcome = ? WHERE token_hash = ? AND consumed_at = 0`,
			params.Now, result.Outcome, tokenHash); err != nil {
			return TelegramAutomationCallbackResult{}, err
		}
	}
	if _, err := tx.Exec(`INSERT INTO telegram_automation_callback_queries(
  query_hash, actor_id, orbit_id, role, chat_id, message_id, token_hash,
  outcome, created_at, expires_at)
VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, queryHash, ctx.ActorID, ctx.OrbitID,
		ctx.Role, params.ChatID, params.MessageID, tokenHash, result.Outcome,
		params.Now, params.Now+telegramQueryTTL.Milliseconds()); err != nil {
		return TelegramAutomationCallbackResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return TelegramAutomationCallbackResult{}, err
	}
	return result, nil
}

func (s *Store) FinalizeTelegramAutomationCallback(token, queryID string, outcome TelegramCallbackOutcome, now int64) error {
	if !telegramCallbackPattern.MatchString(token) || !validTelegramQueryID(queryID) ||
		now <= 0 || !validTelegramHistoryOutcome(outcome) {
		return ErrAutomationInvalid
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
	tokenHash := telegramKeyedDigest(key, "token", token)
	result, err := tx.Exec(`UPDATE telegram_automation_callbacks SET outcome = ?
WHERE token_hash = ? AND consumed_at <> 0`, outcome, tokenHash)
	if err != nil {
		return err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return err
		}
		return fmt.Errorf("finalize Telegram automation callback: %w", ErrAutomationInvalid)
	}
	if _, err := tx.Exec(`UPDATE telegram_automation_callback_queries SET outcome = ?
WHERE query_hash = ? AND token_hash = ?`, outcome,
		telegramKeyedDigest(key, "query", queryID), tokenHash); err != nil {
		return err
	}
	return tx.Commit()
}
