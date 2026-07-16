package store

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	telegramCallbackPrefix = "tg1_"
	telegramCallbackTTL    = 15 * time.Minute
	telegramQueryTTL       = 24 * time.Hour
)

var telegramCallbackPattern = regexp.MustCompile(`^tg1_[A-Za-z0-9_-]{32}$`)

type TelegramInlineAction string

const (
	TelegramChooseOverlay      TelegramInlineAction = "choose_overlay"
	TelegramChooseInterrupt    TelegramInlineAction = "choose_interrupt"
	TelegramChooseAfterCurrent TelegramInlineAction = "choose_after_current"
	TelegramChooseOwn          TelegramInlineAction = "choose_own_barycenter"
	TelegramChooseCurrentAir   TelegramInlineAction = "choose_current_air"
	TelegramConfirmOverlay     TelegramInlineAction = "confirm_overlay"
	TelegramConfirmAfter       TelegramInlineAction = "confirm_after_current"
	TelegramDismiss            TelegramInlineAction = "dismiss"
)

type TelegramCallbackOutcome string

const (
	TelegramCallbackApplied              TelegramCallbackOutcome = "applied"
	TelegramCallbackAlreadyApplied       TelegramCallbackOutcome = "already_applied"
	TelegramCallbackRequiresConfirmation TelegramCallbackOutcome = "requires_confirmation"
	TelegramCallbackTooLate              TelegramCallbackOutcome = "too_late"
	TelegramCallbackExpired              TelegramCallbackOutcome = "expired"
	TelegramCallbackForbidden            TelegramCallbackOutcome = "forbidden"
	TelegramCallbackUnsupported          TelegramCallbackOutcome = "unsupported"
	TelegramCallbackFailed               TelegramCallbackOutcome = "failed"
)

type TelegramInlineRoute struct {
	MediaID                string
	MediaGeneration        int64
	SourceActorID          int64
	SourceOrbitID          int64
	OriginalUpdateID       int64
	AttachmentKind         string
	DefaultTransmissionID  string
	SelectedTransmissionID string
	State                  string
	Revision               int64
	CreatedAt              int64
	UpdatedAt              int64
}

type RegisterTelegramInlineRouteParams struct {
	TelegramUserID   int64
	MediaID          string
	OriginalUpdateID int64
	AttachmentKind   string
	AcceptedAt       int64
	AudienceKind     TransmissionAudienceKind
	Selectors        []TransmissionAudienceSelector
	IncludeOrigin    bool
	Availability     []TransmissionTargetAvailability
}

type RegisterTelegramInlineRouteResult struct {
	Route    TelegramInlineRoute
	Creation *TransmissionCreation
}

type MintTelegramInlineCallbackParams struct {
	TelegramUserID  int64
	MediaID         string
	MediaGeneration int64
	ChatID          int64
	MessageID       int64
	Action          TelegramInlineAction
	Delivery        TransmissionDelivery
	Audience        TransmissionAudienceKind
	// RouteV2 binds Phase 2 values in the additive fail-closed companion.
	// TargetReference is an opaque capability issued by the common target
	// service; numeric target identities are never accepted here.
	RouteV2               bool
	TargetReference       string
	IncludeOrigin         bool
	ConfirmationDelivery  TransmissionDelivery
	ConfirmationTokenHash string
	Now                   int64
}

type ApplyTelegramInlineCallbackParams struct {
	TelegramUserID int64
	QueryID        string
	Token          string
	ChatID         int64
	MessageID      int64
	Now            int64
	Availability   []TransmissionTargetAvailability
}

type ApplyTelegramInlineCallbackResult struct {
	Outcome               TelegramCallbackOutcome
	ClearKeyboard         bool
	Replay                bool
	Creation              *TransmissionCreation
	Cancellation          *CancelTransmissionResult
	Challenge             *TransmissionChallenge
	ConfirmationTokenHash string
	MediaID               string
	MediaGeneration       int64
	Audience              TransmissionAudienceKind
	RouteV2               bool
	TargetReference       string
	IncludeOrigin         bool
}

type telegramInlineBinding struct {
	MediaID               string
	MediaGeneration       int64
	ActorID               int64
	OrbitID               int64
	Authorization         string
	ChatID                int64
	MessageID             int64
	OriginalUpdateID      int64
	Action                TelegramInlineAction
	Delivery              TransmissionDelivery
	Audience              TransmissionAudienceKind
	ConfirmationTokenHash string
	RouteV2               bool
	TargetReference       string
	IncludeOrigin         bool
	ConfirmationDelivery  TransmissionDelivery
	ExpiresAt             int64
	ConsumedAt            int64
	Outcome               TelegramCallbackOutcome
}

func telegramRoutingDigest(parts ...string) string {
	digest := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(digest[:])
}

func telegramCallbackKeyTx(tx *sql.Tx) ([]byte, error) {
	var encoded string
	err := tx.QueryRow(`SELECT value FROM settings WHERE key = 'telegram_inline_callback_hmac_v1'`).Scan(&encoded)
	if err == nil {
		key, decodeErr := hex.DecodeString(encoded)
		if decodeErr != nil || len(key) != sha256.Size {
			return nil, errors.New("invalid Telegram callback HMAC key")
		}
		return key, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	key := make([]byte, sha256.Size)
	if _, err := rand.Read(key); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`INSERT INTO settings(key, value) VALUES(?, ?)`,
		"telegram_inline_callback_hmac_v1", hex.EncodeToString(key)); err != nil {
		return nil, err
	}
	return key, nil
}

func telegramKeyedDigest(key []byte, domain, value string) string {
	mac := hmac.New(sha256.New, key)
	mac.Write([]byte(domain))
	mac.Write([]byte{0})
	mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))
}

func scanTelegramInlineRoute(row sqlScanner) (TelegramInlineRoute, error) {
	var route TelegramInlineRoute
	err := row.Scan(&route.MediaID, &route.MediaGeneration, &route.SourceActorID,
		&route.SourceOrbitID, &route.OriginalUpdateID, &route.AttachmentKind,
		&route.DefaultTransmissionID, &route.SelectedTransmissionID, &route.State,
		&route.Revision, &route.CreatedAt, &route.UpdatedAt)
	return route, err
}

const telegramInlineRouteColumns = `media_id, media_generation, source_actor_id,
source_orbit_id, original_update_id, attachment_kind, default_transmission_id,
selected_transmission_id, state, revision, created_at, updated_at`

func loadTelegramInlineRouteTx(tx *sql.Tx, mediaID string) (TelegramInlineRoute, error) {
	return scanTelegramInlineRoute(tx.QueryRow(`SELECT `+telegramInlineRouteColumns+`
FROM telegram_inline_routes WHERE media_id = ?`, mediaID))
}

// RegisterTelegramInlineRoute creates the voice default and its durable route
// in one transaction. Audio/document clips deliberately register only the
// explicit-action route and never acquire a hidden autoplay grace window.
func (s *Store) RegisterTelegramInlineRoute(
	params RegisterTelegramInlineRouteParams,
) (RegisterTelegramInlineRouteResult, error) {
	if params.TelegramUserID <= 0 || !mediaItemIDPattern.MatchString(params.MediaID) ||
		params.OriginalUpdateID <= 0 || params.AcceptedAt <= 0 ||
		(params.AttachmentKind != "voice" && params.AttachmentKind != "audio" &&
			params.AttachmentKind != "document") {
		return RegisterTelegramInlineRouteResult{}, ErrTransmissionInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return RegisterTelegramInlineRouteResult{}, err
	}
	defer tx.Rollback()
	ctx, err := resolveActorContext(tx, Identity{Kind: IdentityTelegram,
		TelegramUserID: params.TelegramUserID})
	if err != nil {
		return RegisterTelegramInlineRouteResult{}, err
	}
	media, err := scanMediaItem(tx.QueryRow(
		`SELECT `+mediaItemColumns+` FROM media_items WHERE id = ?`, params.MediaID))
	if err != nil {
		return RegisterTelegramInlineRouteResult{}, err
	}
	if media.Source != MediaSourceTelegram || media.ActorID != ctx.ActorID ||
		media.OwnerOrbitID != ctx.OrbitID || media.Status != MediaStatusReady {
		return RegisterTelegramInlineRouteResult{}, ErrTransmissionMediaInvalid
	}
	if existing, err := loadTelegramInlineRouteTx(tx, params.MediaID); err == nil {
		if existing.MediaGeneration != media.Revision ||
			existing.OriginalUpdateID != params.OriginalUpdateID {
			return RegisterTelegramInlineRouteResult{}, ErrTransmissionStateConflict
		}
		var creation *TransmissionCreation
		if existing.DefaultTransmissionID != "" {
			loaded, loadErr := loadTransmissionCreationTx(tx, existing.DefaultTransmissionID)
			if loadErr != nil {
				return RegisterTelegramInlineRouteResult{}, loadErr
			}
			creation = &loaded
		}
		if err := tx.Commit(); err != nil {
			return RegisterTelegramInlineRouteResult{}, err
		}
		return RegisterTelegramInlineRouteResult{Route: existing, Creation: creation}, nil
	} else if !errors.Is(err, sql.ErrNoRows) {
		return RegisterTelegramInlineRouteResult{}, err
	}

	defaultTransmissionID := ""
	var creation *TransmissionCreation
	if params.AttachmentKind == "voice" {
		identity := Identity{Kind: IdentityTelegram, TelegramUserID: params.TelegramUserID}
		idempotency := telegramRoutingDigest("default", params.MediaID,
			strconv.FormatInt(media.Revision, 10))
		requestHash := telegramRoutingDigest("default-request", params.MediaID,
			string(params.AudienceKind), fmt.Sprint(params.Selectors),
			strconv.FormatBool(params.IncludeOrigin))
		resolved := CreateResolvedTransmissionParams{
			ExpectedActorID: ctx.ActorID, Identity: identity,
			IdempotencyKeyHash: idempotency, RequestHash: requestHash,
			MediaID: params.MediaID, AudienceKind: params.AudienceKind,
			Selectors: params.Selectors, OriginKind: TransmissionOriginTelegram,
			IncludeOrigin:     params.IncludeOrigin,
			RequestedDelivery: TransmissionDeliveryAfterCurrent,
			AcceptedAt:        params.AcceptedAt, PolicyAt: media.PublishedAt,
			Availability: params.Availability,
		}
		if !validResolvedTransmissionParams(resolved) {
			return RegisterTelegramInlineRouteResult{}, ErrTransmissionInvalid
		}
		created, err := s.createResolvedTransmissionTx(tx, ctx, resolved)
		if err != nil || created.Challenge != nil {
			if err == nil {
				err = ErrTransmissionStateConflict
			}
			return RegisterTelegramInlineRouteResult{}, err
		}
		defaultTransmissionID = created.Creation.Transmission.ID
		copy := created.Creation
		creation = &copy
	}
	if _, err := tx.Exec(`INSERT INTO telegram_inline_routes(
  media_id, media_generation, source_actor_id, source_orbit_id,
  original_update_id, attachment_kind, default_transmission_id,
  created_at, updated_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`, params.MediaID, media.Revision,
		ctx.ActorID, ctx.OrbitID, params.OriginalUpdateID, params.AttachmentKind,
		defaultTransmissionID, params.AcceptedAt, params.AcceptedAt); err != nil {
		return RegisterTelegramInlineRouteResult{}, err
	}
	route, err := loadTelegramInlineRouteTx(tx, params.MediaID)
	if err != nil {
		return RegisterTelegramInlineRouteResult{}, err
	}
	if err := s.checkpoint("telegram_inline_route_before_commit"); err != nil {
		return RegisterTelegramInlineRouteResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return RegisterTelegramInlineRouteResult{}, err
	}
	return RegisterTelegramInlineRouteResult{Route: route, Creation: creation}, nil
}

func validTelegramCallbackChoice(params MintTelegramInlineCallbackParams) bool {
	if params.RouteV2 {
		if params.Audience != TransmissionAudienceOwnBarycenter &&
			params.Audience != TransmissionAudienceCurrentAir &&
			params.Audience != TransmissionAudienceExplicit {
			return false
		}
		if params.Audience == TransmissionAudienceExplicit {
			if !transmissionTargetReferencePattern.MatchString(params.TargetReference) {
				return false
			}
		} else if params.TargetReference != "" {
			return false
		}
		switch params.Delivery {
		case TransmissionDeliveryOverlay, TransmissionDeliveryInterrupt,
			TransmissionDeliveryAfterCurrent, TransmissionDelivery("queue"),
			TransmissionDelivery("replace"):
		default:
			return false
		}
		return (params.ConfirmationTokenHash == "" && params.ConfirmationDelivery == "") ||
			(params.Delivery == TransmissionDeliveryInterrupt &&
				(params.ConfirmationDelivery == TransmissionDeliveryOverlay ||
					params.ConfirmationDelivery == TransmissionDeliveryAfterCurrent) &&
				transmissionDigestPattern.MatchString(params.ConfirmationTokenHash))
	}
	switch params.Action {
	case TelegramChooseOverlay:
		return params.Delivery == TransmissionDeliveryOverlay
	case TelegramChooseInterrupt:
		return params.Delivery == TransmissionDeliveryInterrupt
	case TelegramChooseAfterCurrent:
		return params.Delivery == TransmissionDeliveryAfterCurrent
	case TelegramConfirmOverlay:
		return params.Delivery == TransmissionDeliveryOverlay &&
			transmissionDigestPattern.MatchString(params.ConfirmationTokenHash)
	case TelegramConfirmAfter:
		return params.Delivery == TransmissionDeliveryAfterCurrent &&
			transmissionDigestPattern.MatchString(params.ConfirmationTokenHash)
	case TelegramDismiss:
		return params.Delivery == "" && params.Audience == ""
	default:
		return false
	}
}

func (s *Store) MintTelegramInlineCallback(
	params MintTelegramInlineCallbackParams,
) (string, error) {
	if params.TelegramUserID <= 0 || !mediaItemIDPattern.MatchString(params.MediaID) ||
		params.MediaGeneration <= 0 || params.MessageID <= 0 || params.Now <= 0 ||
		(!params.RouteV2 && params.Audience != TransmissionAudienceOwnBarycenter &&
			params.Audience != TransmissionAudienceCurrentAir &&
			params.Action != TelegramDismiss) || !validTelegramCallbackChoice(params) {
		return "", ErrTransmissionInvalid
	}
	tx, err := s.db.Begin()
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	ctx, err := resolveActorContext(tx, Identity{Kind: IdentityTelegram,
		TelegramUserID: params.TelegramUserID})
	if err != nil {
		return "", err
	}
	route, err := loadTelegramInlineRouteTx(tx, params.MediaID)
	if err != nil || route.MediaGeneration != params.MediaGeneration ||
		route.SourceActorID != ctx.ActorID || route.SourceOrbitID != ctx.OrbitID ||
		route.State != "pending" {
		if err == nil {
			err = ErrTransmissionStateConflict
		}
		return "", err
	}
	key, err := telegramCallbackKeyTx(tx)
	if err != nil {
		return "", err
	}
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	token := telegramCallbackPrefix + base64.RawURLEncoding.EncodeToString(raw)
	if !telegramCallbackPattern.MatchString(token) {
		return "", errors.New("invalid Telegram callback token")
	}
	tokenHash := telegramKeyedDigest(key, "token", token)
	parentAction, parentDelivery, parentAudience := params.Action, params.Delivery, params.Audience
	if params.RouteV2 {
		// An old binary recognizes the schema value but deliberately has no
		// executable choice for it, so rollback fails closed.
		parentAction, parentDelivery, parentAudience = TelegramChooseOwn, "", TransmissionAudienceOwnBarycenter
	}
	if _, err := tx.Exec(`INSERT INTO telegram_inline_callbacks(
  token_hash, media_id, media_generation, actor_id, orbit_id, authorization,
  chat_id, message_id, original_update_id, action, delivery, audience,
  confirmation_token_hash, created_at, expires_at
) VALUES(?, ?, ?, ?, ?, 'initiator_only', ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		tokenHash, route.MediaID, route.MediaGeneration, route.SourceActorID,
		route.SourceOrbitID, params.ChatID, params.MessageID, route.OriginalUpdateID,
		parentAction, parentDelivery, parentAudience, params.ConfirmationTokenHash,
		params.Now, params.Now+telegramCallbackTTL.Milliseconds()); err != nil {
		return "", err
	}
	if params.RouteV2 {
		if _, err := tx.Exec(`INSERT INTO telegram_inline_callback_routes_v2(
  token_hash, requested_delivery, audience, target_reference, include_origin,
  confirmation_delivery, confirmation_token_hash
) VALUES(?, ?, ?, ?, ?, ?, ?)`, tokenHash, params.Delivery, params.Audience,
			params.TargetReference, params.IncludeOrigin, params.ConfirmationDelivery,
			params.ConfirmationTokenHash); err != nil {
			return "", err
		}
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return token, nil
}

func scanTelegramInlineBinding(row sqlScanner) (telegramInlineBinding, error) {
	var binding telegramInlineBinding
	err := row.Scan(&binding.MediaID, &binding.MediaGeneration, &binding.ActorID,
		&binding.OrbitID, &binding.Authorization, &binding.ChatID, &binding.MessageID,
		&binding.OriginalUpdateID, &binding.Action, &binding.Delivery,
		&binding.Audience, &binding.ConfirmationTokenHash, &binding.ExpiresAt,
		&binding.ConsumedAt, &binding.Outcome)
	return binding, err
}

func loadTelegramInlineBindingTx(tx *sql.Tx, tokenHash string) (telegramInlineBinding, error) {
	binding, err := scanTelegramInlineBinding(tx.QueryRow(`SELECT
media_id, media_generation, actor_id, orbit_id, authorization, chat_id,
message_id, original_update_id, action, delivery, audience,
confirmation_token_hash, expires_at, consumed_at, outcome
FROM telegram_inline_callbacks WHERE token_hash = ?`, tokenHash))
	if err != nil {
		return binding, err
	}
	var includeOrigin bool
	err = tx.QueryRow(`SELECT requested_delivery, audience, target_reference,
include_origin, confirmation_delivery, confirmation_token_hash
FROM telegram_inline_callback_routes_v2 WHERE token_hash = ?`, tokenHash).Scan(
		&binding.Delivery, &binding.Audience, &binding.TargetReference,
		&includeOrigin, &binding.ConfirmationDelivery, &binding.ConfirmationTokenHash,
	)
	if err == nil {
		binding.RouteV2, binding.IncludeOrigin = true, includeOrigin
		return binding, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return binding, err
	}
	return binding, nil
}

func cacheTelegramCallbackQueryTx(
	tx *sql.Tx, key []byte, params ApplyTelegramInlineCallbackParams,
	ctx ActorContext, outcome TelegramCallbackOutcome, clear bool,
) error {
	if params.QueryID == "" {
		return nil
	}
	_, err := tx.Exec(`INSERT OR REPLACE INTO telegram_inline_callback_queries(
  query_hash, actor_id, orbit_id, chat_id, message_id, outcome,
  clear_keyboard, created_at, expires_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?)`, telegramKeyedDigest(key, "query", params.QueryID),
		ctx.ActorID, ctx.OrbitID, params.ChatID, params.MessageID, outcome,
		clear, params.Now, params.Now+telegramQueryTTL.Milliseconds())
	return err
}

func cancelNotStartedTransmissionTx(
	tx *sql.Tx, transmissionID string, now int64,
) (CancelTransmissionResult, error) {
	creation, err := loadTransmissionCreationTx(tx, transmissionID)
	if err != nil {
		return CancelTransmissionResult{}, err
	}
	if now < creation.Transmission.UpdatedAt ||
		!senderCancellationAllowed(creation.Transmission, creation.Targets) {
		return CancelTransmissionResult{}, ErrTransmissionStateConflict
	}
	for _, target := range creation.Targets {
		if now < target.UpdatedAt {
			return CancelTransmissionResult{}, ErrTransmissionStateConflict
		}
	}
	result, err := tx.Exec(`UPDATE transmissions
SET cancellation_cause = ?, revision = revision + 1, updated_at = ?
WHERE id = ? AND revision = ? AND cancellation_cause = '' AND completed_at = 0`,
		TransmissionReasonSenderCancelled, now, transmissionID,
		creation.Transmission.Revision)
	if err != nil {
		return CancelTransmissionResult{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return CancelTransmissionResult{}, err
		}
		return CancelTransmissionResult{}, ErrTransmissionStateConflict
	}
	var disarm []TransmissionTarget
	for _, target := range creation.Targets {
		status := target.Status
		switch status {
		case TransmissionTargetAccepted:
			status = TransmissionTargetCancelled
		case TransmissionTargetPreparing, TransmissionTargetReady,
			TransmissionTargetScheduled:
			status = TransmissionTargetCancelling
		default:
			continue
		}
		endedAt := target.EndedAt
		if status == TransmissionTargetCancelled && endedAt == 0 {
			endedAt = now
		}
		updated, err := tx.Exec(`UPDATE transmission_targets
SET status = ?, reason_code = ?, revision = revision + 1,
    ended_at = ?, updated_at = ?
WHERE transmission_id = ? AND orbit_id = ? AND actor_id = ? AND slot = ?
  AND revision = ? AND generation = ?`, status, TransmissionReasonSenderCancelled,
			endedAt, now, transmissionID, target.OrbitID, target.ActorID, target.Slot,
			target.Revision, target.Generation)
		if err != nil {
			return CancelTransmissionResult{}, err
		}
		if changed, err := updated.RowsAffected(); err != nil || changed != 1 {
			if err != nil {
				return CancelTransmissionResult{}, err
			}
			return CancelTransmissionResult{}, ErrTransmissionStateConflict
		}
		if status == TransmissionTargetCancelling {
			target.Status = status
			target.ReasonCode = TransmissionReasonSenderCancelled
			target.Revision++
			target.UpdatedAt = now
			disarm = append(disarm, target)
		}
	}
	transmission, err := recomputeTransmissionTx(tx, transmissionID, now)
	if err != nil {
		return CancelTransmissionResult{}, err
	}
	return CancelTransmissionResult{Transmission: transmission, Changed: true,
		DisarmTargets: disarm}, nil
}

func callbackChoice(binding telegramInlineBinding) (
	TransmissionDelivery,
	TransmissionAudienceKind,
	[]TransmissionAudienceSelector,
	bool,
	*ConfirmTransmissionFallback,
	bool,
) {
	audience := binding.Audience
	if audience != TransmissionAudienceOwnBarycenter &&
		audience != TransmissionAudienceCurrentAir &&
		audience != TransmissionAudienceExplicit {
		return "", "", nil, false, nil, false
	}
	var selectors []TransmissionAudienceSelector
	if audience == TransmissionAudienceExplicit {
		if !binding.RouteV2 || !transmissionTargetReferencePattern.MatchString(binding.TargetReference) {
			return "", "", nil, false, nil, false
		}
		selectors = []TransmissionAudienceSelector{{Reference: binding.TargetReference}}
	}
	if binding.RouteV2 {
		var confirmation *ConfirmTransmissionFallback
		if binding.ConfirmationTokenHash != "" {
			if binding.Delivery != TransmissionDeliveryInterrupt ||
				(binding.ConfirmationDelivery != TransmissionDeliveryOverlay &&
					binding.ConfirmationDelivery != TransmissionDeliveryAfterCurrent) {
				return "", "", nil, false, nil, false
			}
			confirmation = &ConfirmTransmissionFallback{
				TokenHash: binding.ConfirmationTokenHash,
				Delivery:  binding.ConfirmationDelivery,
			}
		}
		return binding.Delivery, audience, selectors, binding.IncludeOrigin,
			confirmation, true
	}
	switch binding.Action {
	case TelegramChooseOverlay, TelegramChooseInterrupt, TelegramChooseAfterCurrent:
		return binding.Delivery, audience, nil, true, nil, true
	case TelegramConfirmOverlay, TelegramConfirmAfter:
		return TransmissionDeliveryInterrupt, audience, nil, true, &ConfirmTransmissionFallback{
			TokenHash: binding.ConfirmationTokenHash, Delivery: binding.Delivery,
		}, transmissionDigestPattern.MatchString(binding.ConfirmationTokenHash)
	default:
		return "", "", nil, false, nil, false
	}
}

func (s *Store) ApplyTelegramInlineCallback(
	params ApplyTelegramInlineCallbackParams,
) (ApplyTelegramInlineCallbackResult, error) {
	result := ApplyTelegramInlineCallbackResult{Outcome: TelegramCallbackExpired}
	if params.TelegramUserID <= 0 || params.ChatID == 0 || params.MessageID <= 0 ||
		params.Now <= 0 {
		return result, nil
	}
	tx, err := s.db.Begin()
	if err != nil {
		return result, err
	}
	defer tx.Rollback()
	ctx, err := resolveActorContext(tx, Identity{Kind: IdentityTelegram,
		TelegramUserID: params.TelegramUserID})
	if err != nil {
		result.Outcome = TelegramCallbackForbidden
		return result, nil
	}
	key, err := telegramCallbackKeyTx(tx)
	if err != nil {
		return result, err
	}
	if params.QueryID != "" {
		var actorID, orbitID, chatID, messageID int64
		var clear bool
		var outcome TelegramCallbackOutcome
		err := tx.QueryRow(`SELECT actor_id, orbit_id, chat_id, message_id,
outcome, clear_keyboard FROM telegram_inline_callback_queries WHERE query_hash = ?
  AND expires_at > ?`, telegramKeyedDigest(key, "query", params.QueryID), params.Now).Scan(
			&actorID, &orbitID, &chatID, &messageID, &outcome, &clear)
		if err == nil {
			if actorID != ctx.ActorID || orbitID != ctx.OrbitID || chatID != params.ChatID ||
				messageID != params.MessageID {
				result.Outcome = TelegramCallbackForbidden
				return result, nil
			}
			result.Outcome, result.ClearKeyboard, result.Replay = outcome, clear, true
			if outcome == TelegramCallbackRequiresConfirmation &&
				telegramCallbackPattern.MatchString(params.Token) {
				binding, bindingErr := loadTelegramInlineBindingTx(tx,
					telegramKeyedDigest(key, "token", params.Token))
				if bindingErr == nil &&
					transmissionDigestPattern.MatchString(binding.ConfirmationTokenHash) {
					var expiresAt int64
					var overlay, after bool
					if challengeErr := tx.QueryRow(`SELECT expires_at,
overlay_available, after_current_available
FROM transmission_fallback_confirmations WHERE token_hash = ?`,
						binding.ConfirmationTokenHash).Scan(&expiresAt, &overlay, &after); challengeErr == nil {
						result.MediaID, result.MediaGeneration = binding.MediaID, binding.MediaGeneration
						result.Audience = binding.Audience
						result.RouteV2 = binding.RouteV2
						result.TargetReference = binding.TargetReference
						result.IncludeOrigin = binding.IncludeOrigin
						result.ConfirmationTokenHash = binding.ConfirmationTokenHash
						result.Challenge = &TransmissionChallenge{
							ExpiresAt: expiresAt,
							Alternatives: []TransmissionAlternative{
								{Delivery: TransmissionDeliveryOverlay, Available: overlay,
									Reason: "interrupt_resume_unavailable"},
								{Delivery: TransmissionDeliveryAfterCurrent, Available: after,
									Reason: "interrupt_resume_unavailable"},
							},
						}
					}
				}
			}
			return result, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return result, err
		}
	}
	if !telegramCallbackPattern.MatchString(params.Token) {
		result.ClearKeyboard = true
		_ = cacheTelegramCallbackQueryTx(tx, key, params, ctx, result.Outcome, true)
		_ = tx.Commit()
		return result, nil
	}
	binding, err := loadTelegramInlineBindingTx(tx,
		telegramKeyedDigest(key, "token", params.Token))
	if errors.Is(err, sql.ErrNoRows) {
		_ = cacheTelegramCallbackQueryTx(tx, key, params, ctx, result.Outcome, false)
		_ = tx.Commit()
		return result, nil
	}
	if err != nil {
		return result, err
	}
	result.MediaID, result.MediaGeneration = binding.MediaID, binding.MediaGeneration
	authorized := binding.ActorID == ctx.ActorID && binding.OrbitID == ctx.OrbitID
	if binding.Authorization == "source_primary" {
		authorized = binding.OrbitID == ctx.OrbitID && ctx.Role == "primary"
	}
	if !authorized || binding.ChatID != params.ChatID || binding.MessageID != params.MessageID {
		result.Outcome = TelegramCallbackForbidden
		_ = cacheTelegramCallbackQueryTx(tx, key, params, ctx, result.Outcome, false)
		_ = tx.Commit()
		return result, nil
	}
	if params.Now >= binding.ExpiresAt {
		result.ClearKeyboard = true
		_ = cacheTelegramCallbackQueryTx(tx, key, params, ctx, result.Outcome, true)
		_ = tx.Commit()
		return result, nil
	}
	if binding.ConsumedAt > 0 {
		result.Outcome, result.ClearKeyboard = TelegramCallbackAlreadyApplied, true
		_ = cacheTelegramCallbackQueryTx(tx, key, params, ctx, result.Outcome, true)
		_ = tx.Commit()
		return result, nil
	}
	route, err := loadTelegramInlineRouteTx(tx, binding.MediaID)
	if err != nil || route.MediaGeneration != binding.MediaGeneration {
		result.Outcome, result.ClearKeyboard = TelegramCallbackExpired, true
		_ = cacheTelegramCallbackQueryTx(tx, key, params, ctx, result.Outcome, true)
		_ = tx.Commit()
		return result, nil
	}
	if route.State == "selected" || route.State == "dismissed" {
		result.Outcome, result.ClearKeyboard = TelegramCallbackAlreadyApplied, true
		_ = cacheTelegramCallbackQueryTx(tx, key, params, ctx, result.Outcome, true)
		_ = tx.Commit()
		return result, nil
	}
	if binding.Action == TelegramDismiss {
		if _, err := tx.Exec(`UPDATE telegram_inline_routes
SET state = 'dismissed', revision = revision + 1, updated_at = ?
WHERE media_id = ? AND revision = ? AND state = 'pending'`, params.Now,
			route.MediaID, route.Revision); err != nil {
			return result, err
		}
		result.Outcome, result.ClearKeyboard = TelegramCallbackApplied, true
		if _, err := tx.Exec(`UPDATE telegram_inline_callbacks SET consumed_at = ?, outcome = ?
WHERE token_hash = ?`, params.Now, result.Outcome,
			telegramKeyedDigest(key, "token", params.Token)); err != nil {
			return result, err
		}
		if err := cacheTelegramCallbackQueryTx(tx, key, params, ctx, result.Outcome, true); err != nil {
			return result, err
		}
		if err := tx.Commit(); err != nil {
			return result, err
		}
		return result, nil
	}
	delivery, audience, selectors, includeOrigin, confirmation, ok := callbackChoice(binding)
	if !ok {
		result.Outcome, result.ClearKeyboard = TelegramCallbackUnsupported, true
		_ = cacheTelegramCallbackQueryTx(tx, key, params, ctx, result.Outcome, true)
		_ = tx.Commit()
		return result, nil
	}
	result.Audience = audience
	result.RouteV2 = binding.RouteV2
	result.TargetReference = binding.TargetReference
	result.IncludeOrigin = includeOrigin
	if route.DefaultTransmissionID != "" {
		defaultCreation, loadErr := loadTransmissionCreationTx(tx, route.DefaultTransmissionID)
		if loadErr != nil || !senderCancellationAllowed(defaultCreation.Transmission, defaultCreation.Targets) {
			result.Outcome, result.ClearKeyboard = TelegramCallbackTooLate, true
			_ = cacheTelegramCallbackQueryTx(tx, key, params, ctx, result.Outcome, true)
			_ = tx.Commit()
			return result, nil
		}
	}
	idempotency := telegramRoutingDigest("choice", route.MediaID,
		strconv.FormatInt(route.MediaGeneration, 10), string(delivery), string(audience),
		binding.TargetReference, strconv.FormatBool(includeOrigin))
	requestHash := telegramRoutingDigest("choice-request", route.MediaID,
		string(delivery), string(audience), binding.TargetReference,
		strconv.FormatBool(includeOrigin))
	create := CreateResolvedTransmissionParams{
		ExpectedActorID:    ctx.ActorID,
		Identity:           Identity{Kind: IdentityTelegram, TelegramUserID: params.TelegramUserID},
		IdempotencyKeyHash: idempotency, RequestHash: requestHash,
		MediaID: route.MediaID, AudienceKind: audience, Selectors: selectors,
		OriginKind: TransmissionOriginTelegram, IncludeOrigin: includeOrigin,
		RequestedDelivery: delivery, AcceptedAt: params.Now,
		Availability: params.Availability, Confirmation: confirmation,
	}
	if delivery == TransmissionDeliveryInterrupt {
		raw := make([]byte, 32)
		if _, err := rand.Read(raw); err != nil {
			return result, err
		}
		create.ChallengeTokenHash = telegramRoutingDigest("confirmation", hex.EncodeToString(raw))
	}
	created, err := s.createResolvedTransmissionTx(tx, ctx, create)
	if err != nil {
		if errors.Is(err, ErrTransmissionStateConflict) {
			result.Outcome, result.ClearKeyboard = TelegramCallbackTooLate, true
			_ = cacheTelegramCallbackQueryTx(tx, key, params, ctx, result.Outcome, true)
			_ = tx.Commit()
			return result, nil
		}
		return result, err
	}
	if created.Challenge != nil {
		result.Outcome, result.ClearKeyboard = TelegramCallbackRequiresConfirmation, true
		result.Challenge = created.Challenge
		result.ConfirmationTokenHash = create.ChallengeTokenHash
		if _, err := tx.Exec(`UPDATE telegram_inline_callbacks
SET consumed_at = ?, outcome = ?, confirmation_token_hash = ?
WHERE token_hash = ?`, params.Now, result.Outcome, create.ChallengeTokenHash,
			telegramKeyedDigest(key, "token", params.Token)); err != nil {
			return result, err
		}
		if err := cacheTelegramCallbackQueryTx(tx, key, params, ctx, result.Outcome, true); err != nil {
			return result, err
		}
		if err := tx.Commit(); err != nil {
			return result, err
		}
		return result, nil
	}
	if route.DefaultTransmissionID != "" {
		cancelled, err := cancelNotStartedTransmissionTx(tx, route.DefaultTransmissionID, params.Now)
		if err != nil {
			if errors.Is(err, ErrTransmissionStateConflict) {
				result.Outcome, result.ClearKeyboard = TelegramCallbackTooLate, true
				_ = cacheTelegramCallbackQueryTx(tx, key, params, ctx, result.Outcome, true)
				_ = tx.Commit()
				return result, nil
			}
			return result, err
		}
		result.Cancellation = &cancelled
	}
	if _, err := tx.Exec(`UPDATE telegram_inline_routes
SET state = 'selected', selected_transmission_id = ?, revision = revision + 1,
    updated_at = ? WHERE media_id = ? AND revision = ? AND state = 'pending'`,
		created.Creation.Transmission.ID, params.Now, route.MediaID, route.Revision); err != nil {
		return result, err
	}
	result.Outcome, result.ClearKeyboard = TelegramCallbackApplied, true
	copy := created.Creation
	result.Creation = &copy
	if _, err := tx.Exec(`UPDATE telegram_inline_callbacks
SET consumed_at = ?, outcome = ? WHERE media_id = ? AND media_generation = ?
  AND consumed_at = 0`, params.Now, TelegramCallbackAlreadyApplied,
		route.MediaID, route.MediaGeneration); err != nil {
		return result, err
	}
	if _, err := tx.Exec(`UPDATE telegram_inline_callbacks SET outcome = ?
WHERE token_hash = ?`, result.Outcome, telegramKeyedDigest(key, "token", params.Token)); err != nil {
		return result, err
	}
	if err := cacheTelegramCallbackQueryTx(tx, key, params, ctx, result.Outcome, true); err != nil {
		return result, err
	}
	if err := s.checkpoint("telegram_inline_replace_before_commit"); err != nil {
		return result, err
	}
	if err := tx.Commit(); err != nil {
		return result, err
	}
	return result, nil
}
