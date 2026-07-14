package bot

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"sync"
	"time"
)

const (
	callbackTokenPrefix = "tg1_"
	callbackTokenBytes  = 24
	callbackTokenTTL    = 15 * time.Minute
	callbackQueryTTL    = 24 * time.Hour
)

type CallbackAnswerCode string

const (
	CallbackApplied              CallbackAnswerCode = "applied"
	CallbackAlreadyApplied       CallbackAnswerCode = "already_applied"
	CallbackRequiresConfirmation CallbackAnswerCode = "requires_confirmation"
	CallbackTooLate              CallbackAnswerCode = "too_late"
	CallbackExpired              CallbackAnswerCode = "expired"
	CallbackForbidden            CallbackAnswerCode = "forbidden"
	CallbackUnsupported          CallbackAnswerCode = "unsupported"
	CallbackFailed               CallbackAnswerCode = "failed"
)

// CallbackAnswerText is deliberately finite and contains no callback, actor,
// orbit, media or Telegram identifiers.
func CallbackAnswerText(code CallbackAnswerCode) string {
	switch code {
	case CallbackApplied:
		return "Готово"
	case CallbackAlreadyApplied:
		return "Уже применено"
	case CallbackRequiresConfirmation:
		return "Нужно подтверждение"
	case CallbackTooLate:
		return "Уже поздно менять"
	case CallbackExpired:
		return "Кнопка устарела"
	case CallbackForbidden:
		return "Недостаточно прав"
	case CallbackUnsupported:
		return "Действие пока недоступно"
	default:
		return "Не удалось выполнить"
	}
}

type CallbackAction string

const (
	CallbackChooseOverlay      CallbackAction = "choose_overlay"
	CallbackChooseInterrupt    CallbackAction = "choose_interrupt"
	CallbackChooseAfterCurrent CallbackAction = "choose_after_current"
	CallbackChooseOwn          CallbackAction = "choose_own_barycenter"
	CallbackChooseCurrentAir   CallbackAction = "choose_current_air"
	CallbackConfirmOverlay     CallbackAction = "confirm_overlay"
	CallbackConfirmAfter       CallbackAction = "confirm_after_current"
	CallbackDismiss            CallbackAction = "dismiss"
)

type CallbackAuthorization string

const (
	CallbackInitiatorOnly CallbackAuthorization = "initiator_only"
	CallbackSourcePrimary CallbackAuthorization = "source_primary"
)

type CallbackActor struct {
	ActorID int64
	OrbitID int64
	Role    string
}

// CallbackBinding is stored server-side. None of these values is serialized
// into callback_data.
type CallbackBinding struct {
	Initiator        CallbackActor
	Authorization    CallbackAuthorization
	ChatID           int64
	MessageID        int64
	OriginalUpdateID int64
	MediaID          string
	MediaGeneration  int64
	Action           CallbackAction
	Delivery         string
	Audience         string
}

type CallbackRequest struct {
	QueryID   string
	Data      string
	Actor     CallbackActor
	ChatID    int64
	MessageID int64
	Now       time.Time
}

type CallbackDecision struct {
	Code          CallbackAnswerCode
	Consume       bool
	ClearKeyboard bool
}

type CallbackResult struct {
	Code          CallbackAnswerCode
	Binding       CallbackBinding
	Replay        bool
	ClearKeyboard bool
}

type callbackRecord struct {
	tokenDigest   [sha256.Size]byte
	binding       CallbackBinding
	expiresAt     time.Time
	consumed      bool
	consumedActor int64
}

type callbackQueryRecord struct {
	result    CallbackResult
	expiresAt time.Time
	actorID   int64
	orbitID   int64
	role      string
	chatID    int64
	messageID int64
}

// CallbackRegistry owns only opaque transport references. Durable action and
// transmission state belongs to the downstream inline router/store.
type CallbackRegistry struct {
	mu      sync.Mutex
	key     []byte
	random  io.Reader
	now     func() time.Time
	tokens  map[[sha256.Size]byte]*callbackRecord
	queries map[[sha256.Size]byte]callbackQueryRecord
}

func NewCallbackRegistry(key []byte) (*CallbackRegistry, error) {
	if len(key) < sha256.Size {
		return nil, errors.New("callback registry key must contain at least 32 bytes")
	}
	keyCopy := append([]byte(nil), key...)
	return &CallbackRegistry{
		key: keyCopy, random: rand.Reader, now: time.Now,
		tokens:  make(map[[sha256.Size]byte]*callbackRecord),
		queries: make(map[[sha256.Size]byte]callbackQueryRecord),
	}, nil
}

func (r *CallbackRegistry) Mint(binding CallbackBinding) (string, error) {
	if !validCallbackBinding(binding) {
		return "", errors.New("invalid callback binding")
	}
	raw := make([]byte, callbackTokenBytes)
	if _, err := io.ReadFull(r.random, raw); err != nil {
		return "", errors.New("generate callback token")
	}
	token := callbackTokenPrefix + base64.RawURLEncoding.EncodeToString(raw)
	if len(token) != 36 {
		return "", errors.New("invalid callback token length")
	}
	now := r.now().UTC()
	digest := r.digest("token", token)
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.tokens[digest]; exists {
		return "", errors.New("callback token collision")
	}
	r.tokens[digest] = &callbackRecord{
		tokenDigest: digest, binding: binding, expiresAt: now.Add(callbackTokenTTL),
	}
	return token, nil
}

// Handle validates the opaque reference and actor binding, deduplicates the
// Telegram query ID, then invokes apply at most once for that query. apply must
// commit the associated mutation before returning Consume=true.
func (r *CallbackRegistry) Handle(
	request CallbackRequest,
	apply func(CallbackBinding) CallbackDecision,
) CallbackResult {
	now := request.Now.UTC()
	if request.Now.IsZero() {
		now = r.now().UTC()
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.expireQueries(now)
	r.expireTokens(now)

	queryDigest := r.digest("query", request.QueryID)
	if request.QueryID != "" {
		if cached, ok := r.queries[queryDigest]; ok {
			if cached.actorID != request.Actor.ActorID || cached.orbitID != request.Actor.OrbitID ||
				cached.role != request.Actor.Role ||
				cached.chatID != request.ChatID || cached.messageID != request.MessageID {
				return CallbackResult{Code: CallbackForbidden}
			}
			result := cached.result
			result.Replay = true
			return result
		}
	}

	result := CallbackResult{Code: CallbackExpired}
	if !validCallbackToken(request.Data) {
		return r.cacheQuery(queryDigest, request, result, now)
	}
	presentedDigest := r.digest("token", request.Data)
	record, ok := r.tokens[presentedDigest]
	if !ok || !hmac.Equal(presentedDigest[:], record.tokenDigest[:]) {
		return r.cacheQuery(queryDigest, request, result, now)
	}
	if request.ChatID != record.binding.ChatID || request.MessageID != record.binding.MessageID {
		return r.cacheQuery(queryDigest, request, result, now)
	}
	if !callbackAuthorized(record.binding, request.Actor) {
		result.Code = CallbackForbidden
		return r.cacheQuery(queryDigest, request, result, now)
	}
	if !now.Before(record.expiresAt) {
		result.ClearKeyboard = true
		return r.cacheQuery(queryDigest, request, result, now)
	}
	if record.consumed {
		if record.consumedActor == request.Actor.ActorID {
			result.Code = CallbackAlreadyApplied
			result.Binding = record.binding
			result.ClearKeyboard = true
		} else {
			result.Code = CallbackForbidden
		}
		return r.cacheQuery(queryDigest, request, result, now)
	}
	if apply == nil {
		result.Code = CallbackFailed
		return r.cacheQuery(queryDigest, request, result, now)
	}
	decision := apply(record.binding)
	if !validCallbackAnswer(decision.Code) {
		decision = CallbackDecision{Code: CallbackFailed}
	}
	result = CallbackResult{
		Code: decision.Code, Binding: record.binding,
		ClearKeyboard: decision.ClearKeyboard,
	}
	if decision.Consume && decision.Code != CallbackFailed {
		record.consumed = true
		record.consumedActor = request.Actor.ActorID
	}
	return r.cacheQuery(queryDigest, request, result, now)
}

func (r *CallbackRegistry) cacheQuery(
	digest [sha256.Size]byte,
	request CallbackRequest,
	result CallbackResult,
	now time.Time,
) CallbackResult {
	if request.QueryID != "" {
		r.queries[digest] = callbackQueryRecord{
			result: result, expiresAt: now.Add(callbackQueryTTL),
			actorID: request.Actor.ActorID, orbitID: request.Actor.OrbitID,
			role:   request.Actor.Role,
			chatID: request.ChatID, messageID: request.MessageID,
		}
	}
	return result
}

func (r *CallbackRegistry) expireTokens(now time.Time) {
	for digest, record := range r.tokens {
		if !now.Before(record.expiresAt.Add(callbackQueryTTL)) {
			delete(r.tokens, digest)
		}
	}
}

func (r *CallbackRegistry) expireQueries(now time.Time) {
	for digest, record := range r.queries {
		if !now.Before(record.expiresAt) {
			delete(r.queries, digest)
		}
	}
}

func (r *CallbackRegistry) digest(domain, value string) [sha256.Size]byte {
	mac := hmac.New(sha256.New, r.key)
	_, _ = mac.Write([]byte(domain))
	_, _ = mac.Write([]byte{0})
	_, _ = mac.Write([]byte(value))
	var digest [sha256.Size]byte
	copy(digest[:], mac.Sum(nil))
	return digest
}

func validCallbackToken(token string) bool {
	if len(token) != 36 || len(token) > 64 || token[:len(callbackTokenPrefix)] != callbackTokenPrefix {
		return false
	}
	raw, err := base64.RawURLEncoding.DecodeString(token[len(callbackTokenPrefix):])
	return err == nil && len(raw) == callbackTokenBytes
}

func validCallbackBinding(binding CallbackBinding) bool {
	if binding.Initiator.ActorID <= 0 || binding.Initiator.OrbitID <= 0 ||
		!validCallbackRole(binding.Initiator.Role) ||
		binding.ChatID == 0 || binding.MessageID <= 0 || binding.OriginalUpdateID <= 0 ||
		binding.MediaID == "" || len(binding.MediaID) > 128 || binding.MediaGeneration <= 0 ||
		!validCallbackAction(binding.Action) || !validCallbackDelivery(binding.Delivery) ||
		!validCallbackAudience(binding.Audience) {
		return false
	}
	return binding.Authorization == CallbackInitiatorOnly ||
		binding.Authorization == CallbackSourcePrimary
}

func validCallbackRole(role string) bool {
	return role == "primary" || role == "companion" || role == "satellite"
}

func validCallbackDelivery(delivery string) bool {
	return delivery == "" || delivery == "overlay" || delivery == "interrupt" ||
		delivery == "after_current"
}

func validCallbackAudience(audience string) bool {
	return audience == "" || audience == "own_barycenter" || audience == "current_air"
}

func validCallbackAction(action CallbackAction) bool {
	switch action {
	case CallbackChooseOverlay, CallbackChooseInterrupt, CallbackChooseAfterCurrent,
		CallbackChooseOwn, CallbackChooseCurrentAir, CallbackConfirmOverlay,
		CallbackConfirmAfter, CallbackDismiss:
		return true
	default:
		return false
	}
}

func validCallbackAnswer(code CallbackAnswerCode) bool {
	switch code {
	case CallbackApplied, CallbackAlreadyApplied, CallbackRequiresConfirmation,
		CallbackTooLate, CallbackExpired, CallbackForbidden, CallbackUnsupported,
		CallbackFailed:
		return true
	default:
		return false
	}
}

func callbackAuthorized(binding CallbackBinding, actor CallbackActor) bool {
	if actor.ActorID <= 0 || actor.OrbitID != binding.Initiator.OrbitID {
		return false
	}
	if actor.ActorID == binding.Initiator.ActorID {
		return actor.Role == binding.Initiator.Role
	}
	return binding.Authorization == CallbackSourcePrimary && actor.Role == "primary"
}
