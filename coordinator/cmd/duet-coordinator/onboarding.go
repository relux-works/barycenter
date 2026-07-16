package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"relux.works/duet/coordinator/internal/config"
	"relux.works/duet/coordinator/internal/media"
	"relux.works/duet/coordinator/internal/moderation"
	"relux.works/duet/coordinator/internal/store"
)

const (
	errorInvalidRequest          = "invalid_request"
	errorUnauthorized            = "unauthorized"
	errorUnauthenticated         = "unauthenticated"
	errorInsufficientCapability  = "insufficient_capability"
	errorCredentialInvalid       = "credential_invalid"
	errorTooManyAttempts         = "too_many_attempts"
	errorUploadCredential        = "upload_credential_invalid"
	errorUploadTooLarge          = "upload_too_large"
	errorUploadOffsetConflict    = "upload_offset_conflict"
	errorUploadLengthMismatch    = "upload_length_mismatch"
	errorUploadStateConflict     = "upload_state_conflict"
	errorUploadQuota             = "upload_quota_exceeded"
	errorMediaProcessing         = "media_processing_failed"
	errorMediaNotFound           = "media_not_found"
	errorModerationNotFound      = "moderation_not_found"
	errorModerationForbidden     = "moderation_forbidden"
	errorModerationConflict      = "moderation_conflict"
	errorEvidenceExpired         = "moderation_evidence_expired"
	errorDNDRevisionConflict     = "dnd_revision_conflict"
	errorPolicyIdempotency       = "policy_idempotency_conflict"
	errorBlockSubjectNotFound    = "block_subject_not_found"
	errorBlockNotFound           = "block_not_found"
	errorHistoryNotFound         = "history_not_found"
	errorCursorExpired           = "cursor_expired"
	errorInboxNotFound           = "inbox_not_found"
	errorReplayDepthExceeded     = "replay_depth_exceeded"
	errorContentPolicyAcceptance = "content_policy_acceptance_required"
	errorAirNotFound             = "air_not_found"
	errorAirMembershipNotFound   = "membership_not_found"
	errorAirInviteUnavailable    = "invite_unavailable"
	errorAirIdempotency          = "idempotency_conflict"
	errorAirRevision             = "revision_conflict"
	errorAirActiveChanged        = "active_air_changed"
	errorAirDissolved            = "air_dissolved"
	errorAirAlreadyMember        = "already_member"
	errorAirConfirmationRequired = "membership_confirmation_required"
	errorAirCapacity             = "air_barycenter_capacity_reached"
	errorAirOnlineCapacity       = "air_online_pulsar_capacity_reached"
	errorAirParked               = "air_parked"
	errorAirPolicyDenied         = "policy_denied"
	errorAirOwnerTransfer        = "owner_transfer_required"
	errorForbidden               = "forbidden"
	errorServiceUnavailable      = "service_unavailable"
	errorInternal                = "internal_error"
)

var (
	lowerHexBearer = regexp.MustCompile(`^[0-9a-f]{64}$`)
	recoveryHandle = regexp.MustCompile(`^rec_[0-9a-f]{32}$`)
	humanCode      = regexp.MustCompile(`^[ABCDEFGHJKMNPQRSTVWXYZ2-9]{27}$`)
	attemptID      = regexp.MustCompile(`^[A-Za-z0-9_-]{16,128}$`)
)

type apiErrorBody struct {
	Error struct {
		Code              string `json:"code"`
		Message           string `json:"message"`
		RetryAfterSeconds *int64 `json:"retry_after_seconds"`
	} `json:"error"`
}

type optionalJSONString struct {
	Value string
	Set   bool
}

func (value *optionalJSONString) UnmarshalJSON(raw []byte) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return errors.New("explicit null string")
	}
	var decoded string
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return err
	}
	value.Value = decoded
	value.Set = true
	return nil
}

func apiError(w http.ResponseWriter, status int, code string, retry time.Duration) {
	messages := map[string]string{
		errorInvalidRequest:          "The request is malformed or contains invalid parameters.",
		errorUnauthorized:            "Authentication is required.",
		errorUnauthenticated:         "Authentication is required.",
		errorInsufficientCapability:  "This token does not have the required capability.",
		errorCredentialInvalid:       "The provided credential is not valid.",
		errorTooManyAttempts:         "Too many attempts. Please wait before retrying.",
		errorUploadCredential:        "The upload credential is not valid.",
		errorUploadTooLarge:          "The upload exceeds the allowed size.",
		errorUploadOffsetConflict:    "The upload offset does not match the stored offset.",
		errorUploadLengthMismatch:    "The upload body length does not match the request.",
		errorUploadStateConflict:     "The upload cannot be changed in its current state.",
		errorUploadQuota:             "The media upload quota has been reached.",
		errorMediaProcessing:         "The media could not be validated or prepared.",
		errorMediaNotFound:           "The media item was not found.",
		errorModerationNotFound:      "The moderation item was not found.",
		errorModerationForbidden:     "This operator credential lacks the required capability.",
		errorModerationConflict:      "A different moderation decision already exists.",
		errorEvidenceExpired:         "The moderation evidence window has expired.",
		errorDNDRevisionConflict:     "The DND layer changed; retry with its current revision.",
		errorPolicyIdempotency:       "The idempotency key was already used for different input.",
		errorBlockSubjectNotFound:    "The blocking subject is unavailable.",
		errorBlockNotFound:           "The block was not found.",
		errorHistoryNotFound:         "The history item was not found.",
		errorCursorExpired:           "The pagination cursor is invalid or expired.",
		errorInboxNotFound:           "The inbox item was not found.",
		errorReplayDepthExceeded:     "The replay depth limit has been reached.",
		errorContentPolicyAcceptance: "Accept the current content policy before this action.",
		errorAirNotFound:             "The Air was not found.",
		errorAirMembershipNotFound:   "The Air membership was not found.",
		errorAirInviteUnavailable:    "The Air invite is unavailable.",
		errorAirIdempotency:          "The idempotency key was already used for different input.",
		errorAirRevision:             "The Air resource changed; retry with its current revision.",
		errorAirActiveChanged:        "The active Air changed; refresh before retrying.",
		errorAirDissolved:            "The Air has been dissolved.",
		errorAirAlreadyMember:        "This barycenter already has a live membership in the Air.",
		errorAirConfirmationRequired: "The joining barycenter primary must confirm this membership.",
		errorAirCapacity:             "The Air has reached its barycenter capacity.",
		errorAirOnlineCapacity:       "The Air has reached its online Pulsar capacity.",
		errorAirParked:               "The Air is parked and cannot accept playback work.",
		errorAirPolicyDenied:         "The Air policy denies this operation.",
		errorAirOwnerTransfer:        "Transfer Air ownership before leaving.",
		errorForbidden:               "This actor is not permitted to perform the operation.",
		errorServiceUnavailable:      "The service is temporarily unavailable.",
		errorInternal:                "An internal error occurred.",
	}
	var body apiErrorBody
	body.Error.Code = code
	body.Error.Message = messages[code]
	if code == errorTooManyAttempts {
		seconds := int64(retry / time.Second)
		if retry%time.Second != 0 {
			seconds++
		}
		if seconds < 1 {
			seconds = 1
		}
		body.Error.RetryAfterSeconds = &seconds
		w.Header().Set("Retry-After", strconv.FormatInt(seconds, 10))
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

type attemptEntry struct {
	timestamps []int64
	lastUsed   uint64
}

// attemptLimiter reserves before returning and caps arbitrary-key state. It
// retains the newest limit+1 timestamps per key, which is sufficient to
// decide admission while ensuring every rejected attempt advances the rolling
// window without unbounded per-key memory.
type attemptLimiter struct {
	mu      sync.Mutex
	entries map[string]attemptEntry
	limit   int
	window  int64
	cap     int
	clock   uint64
	now     func() time.Time
}

func newAttemptLimiter(limit int, window time.Duration, cap int) *attemptLimiter {
	return &attemptLimiter{
		entries: make(map[string]attemptEntry), limit: limit,
		window: window.Milliseconds(), cap: cap, now: time.Now,
	}
}

func (l *attemptLimiter) reserve(key string) (bool, time.Duration) {
	now := l.now().UnixMilli()
	l.mu.Lock()
	defer l.mu.Unlock()
	l.clock++
	entry, exists := l.entries[key]
	cut := now - l.window
	kept := entry.timestamps[:0]
	for _, timestamp := range entry.timestamps {
		if timestamp > cut {
			kept = append(kept, timestamp)
		}
	}
	if !exists && l.cap > 0 && len(l.entries) >= l.cap {
		var oldestKey string
		oldest := ^uint64(0)
		for candidate, current := range l.entries {
			if current.lastUsed < oldest {
				oldestKey, oldest = candidate, current.lastUsed
			}
		}
		delete(l.entries, oldestKey)
	}
	kept = append(kept, now)
	if len(kept) > l.limit+1 {
		kept = kept[len(kept)-(l.limit+1):]
	}
	entry.timestamps = kept
	entry.lastUsed = l.clock
	l.entries[key] = entry
	if len(kept) <= l.limit {
		return true, 0
	}
	// The next request also counts. Admission therefore requires the current
	// population to fall to limit-1 before that append. For m retained events,
	// m-limit+1 events must expire, so the last required expiry is at index
	// m-limit (zero based). With the bounded limit+1 representation this is the
	// second-oldest timestamp.
	retryMS := kept[len(kept)-l.limit] + l.window - now
	if retryMS < 1 {
		retryMS = 1
	}
	return false, time.Duration(retryMS) * time.Millisecond
}

type actorRequest struct {
	Context store.ActorContext
	Bearer  string
}

type actorRequestKey struct{}

type moderationOperatorRequest struct {
	Context store.ModerationOperatorContext
	Bearer  string
}

type moderationOperatorRequestKey struct{}

type onboardingAPI struct {
	store                  *store.Store
	config                 *config.Config
	log                    *slog.Logger
	botUsername            string
	createIP               *attemptLimiter
	createAttempt          *attemptLimiter
	inviteConsumeIP        *attemptLimiter
	recoveryIP             *attemptLimiter
	recoveryID             *attemptLimiter
	rotateActor            *attemptLimiter
	linkActor              *attemptLimiter
	contentPolicyNow       func() time.Time
	mediaUploadDir         string
	mediaUploadQuota       store.MediaUploadQuota
	mediaUploadNow         func() time.Time
	mediaUploadLocks       [64]sync.Mutex
	mediaUploadMaintenance sync.Mutex
	mediaUploadInitErr     error
	mediaSubmitter         mediaUploadSubmitter
	mediaSubmitterInitErr  error
	mediaLifecycle         *media.LifecycleService
	mediaLifecycleInitErr  error
	mediaDownload          *media.DownloadService
	mediaDownloadInitErr   error
	moderationService      *moderation.Service
	transmissionNow        func() time.Time
	transmissionPresence   transmissionPresenceSnapshotter
	transmissionToken      func() (string, error)
	transmissionActor      *attemptLimiter
	transmissionOrbit      *attemptLimiter
	transmissionAccepted   func(string)
	transmissionCancelled  func(store.CancelTransmissionResult)
	airInviteConsumeActor  *attemptLimiter
	airInviteConsumeIP     *attemptLimiter
	airNow                 func() time.Time
	airRuntimeChanged      func() error
	// testAfterAuth is nil in production. Tests use it to pause between
	// middleware authentication and the immediate writer transaction.
	testAfterAuth   func(store.ActorContext)
	testBeforeStore func(string)
}

func newOnboardingAPIBase(st *store.Store, cfg *config.Config, log *slog.Logger, botUsername string) *onboardingAPI {
	api := &onboardingAPI{
		store: st, config: cfg, log: log, botUsername: strings.TrimPrefix(botUsername, "@"),
		createIP:         newAttemptLimiter(5, time.Hour, 10_000),
		createAttempt:    newAttemptLimiter(1, time.Hour, 10_000),
		inviteConsumeIP:  newAttemptLimiter(20, 15*time.Minute, 10_000),
		recoveryIP:       newAttemptLimiter(30, 15*time.Minute, 10_000),
		recoveryID:       newAttemptLimiter(10, 15*time.Minute, 10_000),
		rotateActor:      newAttemptLimiter(10, time.Hour, 0),
		linkActor:        newAttemptLimiter(10, time.Hour, 0),
		contentPolicyNow: time.Now,
		mediaUploadQuota: store.DefaultMediaUploadQuota(),
		mediaUploadNow:   time.Now,
		transmissionNow:  time.Now,
		transmissionPresence: func() map[transmissionPresenceKey]transmissionPresenceState {
			return map[transmissionPresenceKey]transmissionPresenceState{}
		},
		transmissionToken:     newTransmissionConfirmationToken,
		transmissionActor:     newAttemptLimiter(120, time.Minute, 10_000),
		transmissionOrbit:     newAttemptLimiter(600, time.Minute, 10_000),
		transmissionAccepted:  func(string) {},
		transmissionCancelled: func(store.CancelTransmissionResult) {},
		airInviteConsumeActor: newAttemptLimiter(5, time.Minute, 10_000),
		airInviteConsumeIP:    newAttemptLimiter(5, time.Minute, 10_000),
		airNow:                time.Now,
		airRuntimeChanged:     func() error { return nil },
	}
	api.mediaUploadInitErr = api.initializeMediaUploadStorage()
	api.mediaLifecycle, api.mediaLifecycleInitErr = media.NewLifecycleService(st, cfg.MediaDir)
	api.mediaDownload, api.mediaDownloadInitErr = media.NewDownloadService(st, cfg.MediaDir)
	if api.mediaDownloadInitErr == nil {
		api.mediaDownload.SetTargetSnapshotReader(st)
	}
	return api
}

func newOnboardingAPI(st *store.Store, cfg *config.Config, log *slog.Logger, botUsername string) *onboardingAPI {
	api := newOnboardingAPIBase(st, cfg, log, botUsername)
	preset := media.Preset(cfg.Media.Preset)
	if preset == "" {
		preset = media.PresetDefault
	}
	api.mediaSubmitter, api.mediaSubmitterInitErr = media.NewSubmitService(st, cfg.MediaDir, preset)
	return api
}

func newOnboardingAPIWithMediaSubmitter(
	st *store.Store,
	cfg *config.Config,
	log *slog.Logger,
	botUsername string,
	submitter mediaUploadSubmitter,
	submitterInitErr error,
) *onboardingAPI {
	api := newOnboardingAPIBase(st, cfg, log, botUsername)
	api.mediaSubmitter = submitter
	api.mediaSubmitterInitErr = submitterInitErr
	return api
}

func registerOnboardingRoutes(mux *http.ServeMux, st *store.Store, cfg *config.Config, log *slog.Logger, botUsername string) *onboardingAPI {
	if !cfg.SelfServiceOnboarding {
		return nil
	}
	api := newOnboardingAPI(st, cfg, log, botUsername)
	api.register(mux)
	return api
}

func registerOnboardingRoutesWithMediaSubmitter(
	mux *http.ServeMux,
	st *store.Store,
	cfg *config.Config,
	log *slog.Logger,
	botUsername string,
	submitter mediaUploadSubmitter,
	submitterInitErr error,
) *onboardingAPI {
	if !cfg.SelfServiceOnboarding {
		return nil
	}
	api := newOnboardingAPIWithMediaSubmitter(
		st, cfg, log, botUsername, submitter, submitterInitErr,
	)
	api.register(mux)
	return api
}

func (api *onboardingAPI) register(mux *http.ServeMux) {
	mux.HandleFunc("/v1/onboarding/orbits", api.secure(api.createOrbit))
	mux.HandleFunc("/v1/device-invites", api.secure(api.withControl(api.deviceInvites)))
	mux.HandleFunc("/v1/device-invites/consume", api.secure(api.consumeDeviceInvite))
	mux.HandleFunc("/v1/recovery/consume", api.secure(api.consumeRecovery))
	mux.HandleFunc("/v1/recovery/rotate", api.secure(api.withControl(api.rotateRecovery)))
	mux.HandleFunc("/v1/actor/context", api.secure(api.withActor(api.actorContext)))
	mux.HandleFunc("/v1/telegram-links", api.secure(api.withControl(api.telegramLinks)))
	mux.HandleFunc("/v1/content-policy", api.secure(api.withActor(api.contentPolicy)))
	mux.HandleFunc("/v1/content-policy/acceptance", api.secure(api.withActor(api.contentPolicyAcceptance)))
	mux.HandleFunc("/v1/media/uploads", api.secure(api.withControl(api.createMediaUpload)))
	mux.HandleFunc("/v1/media/uploads/", api.secure(api.writeMediaUpload))
	mux.HandleFunc("/v1/media/", api.secure(api.withActor(api.mediaItem)))
	mux.HandleFunc("/v1/transmissions", api.secure(api.withControl(api.createTransmission)))
	mux.HandleFunc("/v1/transmissions/", api.secure(api.withActor(api.transmissionItem)))
	mux.HandleFunc("/v1/transmission-targets", api.secure(api.withControl(api.transmissionTargetReferences)))
	mux.HandleFunc("/v1/presence", api.secure(api.withActor(api.presence)))
	mux.HandleFunc("/v1/presence/dnd/local", api.secure(api.withControl(api.localDND)))
	mux.HandleFunc("/v1/presence/dnd/orbit", api.secure(api.withControl(api.orbitDND)))
	mux.HandleFunc("/v1/blocks", api.secure(api.withControl(api.blocks)))
	mux.HandleFunc("/v1/blocks/", api.secure(api.withControl(api.blockItem)))
	mux.HandleFunc("/v1/history", api.secure(api.withActor(api.history)))
	mux.HandleFunc("/v1/history/", api.secure(api.withActor(api.historyItem)))
	mux.HandleFunc("/v1/inbox", api.secure(api.withControl(api.inbox)))
	mux.HandleFunc("/v1/inbox/", api.secure(api.withControl(api.inboxItem)))
	mux.HandleFunc("/v1/reports", api.secure(api.withControl(api.moderationReports)))
	mux.HandleFunc("/v1/reports/", api.secure(api.withControl(api.moderationReportItem)))
	mux.HandleFunc("/v1/moderation/reports", api.secure(api.withModerationOperator(api.moderationQueue)))
	mux.HandleFunc("/v1/moderation/reports/", api.secure(api.withModerationOperator(api.moderationQueueItem)))
	mux.HandleFunc("/v1/airs", api.secure(api.withAirControl(api.airsCollection)))
	mux.HandleFunc("/v1/airs/", api.secure(api.withAirControl(api.airItem)))
	mux.HandleFunc("/v1/air-invites/consume", api.secure(api.withAirControl(api.consumeAirInvite)))
}

func (api *onboardingAPI) secure(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		if !secureRequest(r, api.config.TrustedProxy) {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		next(w, r)
	}
}

func secureRequest(r *http.Request, trustedProxy bool) bool {
	if r.TLS != nil {
		return true
	}
	peer := directPeerIP(r)
	if peer == nil || !peer.IsLoopback() {
		// Forwarding headers cannot authenticate a non-loopback direct peer.
		return false
	}
	if !trustedProxy || !hasForwardingMarker(r) {
		// Direct loopback is the explicit local/test exception in the contract.
		return true
	}
	// A configured loopback TLS terminator represents an external origin only
	// when it asserts the canonical secure scheme and its proxy-appended final
	// XFF hop is a valid IP. Missing, duplicated, comma-valued, or plaintext
	// scheme markers fail closed.
	protoValues := r.Header.Values("X-Forwarded-Proto")
	if len(protoValues) != 1 || protoValues[0] != "https" {
		return false
	}
	_, ok := forwardedClientIP(r)
	return ok
}

func hasForwardingMarker(r *http.Request) bool {
	return len(r.Header.Values("X-Forwarded-Proto")) != 0 ||
		len(r.Header.Values("X-Forwarded-For")) != 0 ||
		len(r.Header.Values("X-Real-Ip")) != 0 ||
		len(r.Header.Values("Forwarded")) != 0
}

func directPeerIP(r *http.Request) net.IP {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	return net.ParseIP(strings.TrimSpace(host))
}

func onboardingClientIP(r *http.Request, trustedProxy bool) string {
	if r.TLS != nil {
		return clientIP(r, false)
	}
	peer := directPeerIP(r)
	if trustedProxy && peer != nil && peer.IsLoopback() {
		if forwarded, ok := forwardedClientIP(r); ok {
			return forwarded
		}
		return peer.String()
	}
	return clientIP(r, false)
}

func forwardedClientIP(r *http.Request) (string, bool) {
	values := r.Header.Values("X-Forwarded-For")
	if len(values) != 1 {
		return "", false
	}
	parts := strings.Split(values[0], ",")
	candidate := strings.TrimSpace(parts[len(parts)-1])
	parsed := net.ParseIP(candidate)
	if parsed == nil {
		return "", false
	}
	return parsed.String(), true
}

func bearerToken(r *http.Request) (string, bool) {
	values := r.Header.Values("Authorization")
	if len(values) != 1 {
		return "", false
	}
	header := values[0]
	if !strings.HasPrefix(header, "Bearer ") || strings.Count(header, " ") != 1 {
		return "", false
	}
	token := strings.TrimPrefix(header, "Bearer ")
	return token, lowerHexBearer.MatchString(token)
}

func moderationBearerToken(r *http.Request) (string, bool) {
	values := r.Header.Values("Authorization")
	if len(values) != 1 {
		return "", false
	}
	header := values[0]
	if !strings.HasPrefix(header, "Bearer mod_") || strings.Count(header, " ") != 1 {
		return "", false
	}
	token := strings.TrimPrefix(header, "Bearer ")
	if len(token) != 68 || !lowerHexBearer.MatchString(strings.TrimPrefix(token, "mod_")) {
		return "", false
	}
	return token, true
}

func (api *onboardingAPI) withModerationOperator(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := moderationBearerToken(r)
		if !ok {
			apiError(w, http.StatusUnauthorized, errorUnauthorized, 0)
			return
		}
		operator, err := api.store.ResolveModerationOperator(token)
		if errors.Is(err, store.ErrUnauthorized) {
			apiError(w, http.StatusUnauthorized, errorUnauthorized, 0)
			return
		}
		if err != nil {
			api.internalError(w, "resolve moderation operator", err)
			return
		}
		request := moderationOperatorRequest{Context: operator, Bearer: token}
		next(w, r.WithContext(context.WithValue(
			r.Context(), moderationOperatorRequestKey{}, request,
		)))
	}
}

func (api *onboardingAPI) withActor(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			apiError(w, http.StatusUnauthorized, errorUnauthorized, 0)
			return
		}
		ctx, err := api.store.ResolveTokenActorContext(token)
		if err != nil {
			switch {
			case errors.Is(err, store.ErrInsufficientCapability), errors.Is(err, store.ErrOrbitDisabled):
				apiError(w, http.StatusForbidden, errorInsufficientCapability, 0)
			case errors.Is(err, store.ErrUnauthorized):
				apiError(w, http.StatusUnauthorized, errorUnauthorized, 0)
			default:
				api.internalError(w, "resolve actor context", err)
			}
			return
		}
		req := actorRequest{Context: ctx, Bearer: token}
		next(w, r.WithContext(context.WithValue(r.Context(), actorRequestKey{}, req)))
	}
}

func (api *onboardingAPI) withControl(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token, ok := bearerToken(r)
		if !ok {
			apiError(w, http.StatusUnauthorized, errorUnauthorized, 0)
			return
		}
		ctx, err := api.store.ResolveTokenActorContext(token)
		if errors.Is(err, store.ErrUnauthorized) {
			apiError(w, http.StatusUnauthorized, errorUnauthorized, 0)
			return
		}
		if err != nil && !errors.Is(err, store.ErrInsufficientCapability) {
			api.internalError(w, "resolve control context", err)
			return
		}
		if !ctx.Capabilities.Has(store.CapabilityControl) {
			apiError(w, http.StatusForbidden, errorInsufficientCapability, 0)
			return
		}
		if api.testAfterAuth != nil {
			api.testAfterAuth(ctx)
		}
		req := actorRequest{Context: ctx, Bearer: token}
		next(w, r.WithContext(context.WithValue(r.Context(), actorRequestKey{}, req)))
	}
}

func decodeBoundedJSON(w http.ResponseWriter, r *http.Request, max int64, target any) bool {
	bounded := json.NewDecoder(http.MaxBytesReader(w, r.Body, max))
	var raw json.RawMessage
	if err := bounded.Decode(&raw); err != nil {
		return false
	}
	var extra any
	if !errors.Is(bounded.Decode(&extra), io.EOF) {
		return false
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) < 2 || raw[0] != '{' {
		return false
	}
	strict := json.NewDecoder(bytes.NewReader(raw))
	strict.DisallowUnknownFields()
	if err := strict.Decode(target); err != nil {
		return false
	}
	return errors.Is(strict.Decode(&extra), io.EOF)
}

func normalizeCodeForSyntax(value string) (string, bool) {
	var b strings.Builder
	for _, r := range value {
		switch r {
		case '-', ' ', '\t', '\r', '\n', '\v', '\f':
		default:
			b.WriteRune(r)
		}
	}
	canonical := strings.ToUpper(b.String())
	return canonical, humanCode.MatchString(canonical)
}

func rateLimitActorScope(ctx store.ActorContext) store.RateLimitAuditScope {
	orbitID, actorID := ctx.OrbitID, ctx.ActorID
	return store.RateLimitAuditScope{OrbitID: &orbitID, ActorID: &actorID}
}

func (api *onboardingAPI) reserve(w http.ResponseWriter, limiter *attemptLimiter, class store.RateLimitAuditClass, subject string, scope store.RateLimitAuditScope) bool {
	allowed, retry := limiter.reserve(subject)
	if allowed {
		return true
	}
	if err := api.store.RecordRateLimitAudit(class, subject, scope); err != nil {
		api.internalError(w, "audit rate-limit rejection", err)
		return false
	}
	apiError(w, http.StatusTooManyRequests, errorTooManyAttempts, retry)
	return false
}

func (api *onboardingAPI) createOrbit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	// Bootstrap never accepts ambient bearer authority. In particular a node
	// token cannot use this unauthenticated route to mint control authority.
	if len(r.Header.Values("Authorization")) != 0 {
		token, ok := bearerToken(r)
		if !ok {
			apiError(w, http.StatusUnauthorized, errorUnauthorized, 0)
			return
		}
		ctx, err := api.store.ResolveTokenActorContext(token)
		switch {
		case err == nil, errors.Is(err, store.ErrInsufficientCapability):
			if ctx.Capabilities.Has(store.CapabilityNode) {
				apiError(w, http.StatusForbidden, errorInsufficientCapability, 0)
				return
			}
			apiError(w, http.StatusUnauthorized, errorUnauthorized, 0)
		case errors.Is(err, store.ErrUnauthorized):
			apiError(w, http.StatusUnauthorized, errorUnauthorized, 0)
		default:
			api.internalError(w, "resolve bootstrap bearer", err)
		}
		return
	}
	var req struct {
		Title                 string `json:"title"`
		InstallationAttemptID string `json:"installation_attempt_id"`
	}
	if !decodeBoundedJSON(w, r, 512, &req) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	if len(req.Title) > 120 {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if req.Title == "" || !utf8.ValidString(req.Title) || !attemptID.MatchString(req.InstallationAttemptID) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	if !api.reserve(w, api.createIP, store.RateLimitCreateSourceIP,
		onboardingClientIP(r, api.config.TrustedProxy), store.RateLimitAuditScope{}) ||
		!api.reserve(w, api.createAttempt, store.RateLimitCreateInstallationAttempt,
			req.InstallationAttemptID, store.RateLimitAuditScope{}) {
		return
	}
	result, err := api.store.CreateSelfServiceOrbit(req.Title)
	if err != nil {
		api.internalError(w, "create orbit", err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"orbit_id": result.OrbitID, "title": result.OrbitTitle,
		"actor_id": result.ActorID, "role": result.Role, "slot": result.Slot,
		"node_token": result.NodeToken, "control_token": result.ControlToken,
		"recovery_id": result.RecoveryID, "recovery_secret": result.RecoverySecret,
		"shown_once": true,
	})
}

func (api *onboardingAPI) deviceInvites(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	var req struct {
		IntendedRole optionalJSONString `json:"intended_role"`
	}
	if !decodeBoundedJSON(w, r, 256, &req) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	intendedRole := req.IntendedRole.Value
	if !req.IntendedRole.Set || intendedRole == "" {
		intendedRole = "companion"
	}
	if intendedRole != "companion" && intendedRole != "satellite" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	result, err := api.store.IssueDeviceInvite(actor.Context.ActorID, actor.Bearer, intendedRole)
	if api.mutationError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"invite_code": result.Code, "intended_role": result.IntendedRole,
		"expires_at": result.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

func (api *onboardingAPI) consumeDeviceInvite(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	var req struct {
		InviteCode string `json:"invite_code"`
	}
	if !decodeBoundedJSON(w, r, 256, &req) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	if len(req.InviteCode) > 40 {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	canonical, valid := normalizeCodeForSyntax(req.InviteCode)
	if !valid {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	if !api.reserve(w, api.inviteConsumeIP, store.RateLimitInviteConsumeSourceIP,
		onboardingClientIP(r, api.config.TrustedProxy), store.RateLimitAuditScope{}) {
		return
	}
	if api.testBeforeStore != nil {
		api.testBeforeStore("device_invite.consume")
	}
	result, err := api.store.ConsumeDeviceInvite(canonical)
	if errors.Is(err, store.ErrCredentialInvalid) {
		apiError(w, http.StatusForbidden, errorCredentialInvalid, 0)
		return
	}
	if err != nil {
		api.internalError(w, "consume device invite", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"orbit_id": result.OrbitID, "title": result.OrbitTitle,
		"actor_id": result.ActorID, "role": result.Role, "slot": result.Slot,
		"node_token": result.NodeToken, "control_token": result.ControlToken,
	})
}

func (api *onboardingAPI) consumeRecovery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	var req struct {
		RecoveryID              string `json:"recovery_id"`
		RecoverySecret          string `json:"recovery_secret"`
		ReplacementControlToken string `json:"replacement_control_token"`
	}
	if !decodeBoundedJSON(w, r, 512, &req) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	if len(req.RecoverySecret) > 40 {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	canonical, valid := normalizeCodeForSyntax(req.RecoverySecret)
	if !valid || !recoveryHandle.MatchString(req.RecoveryID) || !lowerHexBearer.MatchString(req.ReplacementControlToken) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	if !api.reserve(w, api.recoveryIP, store.RateLimitRecoveryConsumeSourceIP,
		onboardingClientIP(r, api.config.TrustedProxy), store.RateLimitAuditScope{}) ||
		!api.reserve(w, api.recoveryID, store.RateLimitRecoveryConsumeRecoveryID,
			req.RecoveryID, store.RateLimitAuditScope{}) {
		return
	}
	if api.testBeforeStore != nil {
		api.testBeforeStore("recovery.consume")
	}
	result, err := api.store.ConsumeRecovery(req.RecoveryID, canonical, req.ReplacementControlToken)
	if errors.Is(err, store.ErrCredentialInvalid) {
		apiError(w, http.StatusForbidden, errorCredentialInvalid, 0)
		return
	}
	if err != nil {
		api.internalError(w, "consume recovery", err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"orbit_id": result.OrbitID, "actor_id": result.ActorID, "role": result.Role})
}

func (api *onboardingAPI) rotateRecovery(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	var req struct{}
	if !decodeBoundedJSON(w, r, 64, &req) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	if !api.reserve(w, api.rotateActor, store.RateLimitRecoveryRotateActor,
		strconv.FormatInt(actor.Context.ActorID, 10), rateLimitActorScope(actor.Context)) {
		return
	}
	result, err := api.store.RotateRecovery(actor.Context.ActorID, actor.Bearer)
	if api.mutationError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"actor_id": result.ActorID, "recovery_id": result.RecoveryID,
		"recovery_secret": result.RecoverySecret, "shown_once": true,
	})
}

func (api *onboardingAPI) actorContext(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	writeJSON(w, http.StatusOK, map[string]any{
		"orbit_id": actor.Context.OrbitID, "actor_id": actor.Context.ActorID, "role": actor.Context.Role,
	})
}

func (api *onboardingAPI) telegramLinks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	var req struct {
		DesiredRole optionalJSONString `json:"desired_role"`
	}
	if !decodeBoundedJSON(w, r, 256, &req) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	desiredRole := req.DesiredRole.Value
	if !req.DesiredRole.Set || desiredRole == "" {
		desiredRole = "companion"
	}
	if desiredRole != "companion" && desiredRole != "satellite" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	if !api.reserve(w, api.linkActor, store.RateLimitTelegramLinkIssueActor,
		strconv.FormatInt(actor.Context.ActorID, 10), rateLimitActorScope(actor.Context)) {
		return
	}
	result, err := api.store.IssueTelegramLink(actor.Context.ActorID, actor.Bearer, desiredRole)
	if api.mutationError(w, err) {
		return
	}
	writeJSON(w, http.StatusCreated, map[string]any{
		"link_code": result.Code, "desired_role": result.DesiredRole,
		"expires_at": result.ExpiresAt.UTC().Format(time.RFC3339), "bot_username": api.botUsername,
	})
}

func (api *onboardingAPI) mutationError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, store.ErrUnauthorized):
		apiError(w, http.StatusUnauthorized, errorUnauthorized, 0)
	case errors.Is(err, store.ErrInsufficientCapability), errors.Is(err, store.ErrOrbitDisabled):
		apiError(w, http.StatusForbidden, errorInsufficientCapability, 0)
	default:
		api.internalError(w, "onboarding mutation", err)
	}
	return true
}

func (api *onboardingAPI) internalError(w http.ResponseWriter, operation string, err error) {
	// Store errors carry only structural identifiers. Request bodies and
	// plaintext credentials are deliberately never logged.
	api.log.Error(operation+" failed", "err", err)
	apiError(w, http.StatusInternalServerError, errorInternal, 0)
}
