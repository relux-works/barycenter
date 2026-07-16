package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"relux.works/duet/coordinator/internal/hub"
	"relux.works/duet/coordinator/internal/protocol"
	"relux.works/duet/coordinator/internal/store"
)

const transmissionRequestMaxBytes = int64(16 << 10)

var (
	transmissionHTTPID            = regexp.MustCompile(`^tr_[0-9A-HJKMNP-TV-Z]{26}$`)
	transmissionMediaID           = regexp.MustCompile(`^m_[0-9A-HJKMNP-TV-Z]{26}$`)
	transmissionIdempotencyKey    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{15,127}$`)
	transmissionConfirmationToken = regexp.MustCompile(`^fc_[0-9a-f]{64}$`)
	transmissionTargetReference   = regexp.MustCompile(`^trf_[A-Za-z0-9_-]{43}$`)
)

const (
	errorAudienceNotFound        = "audience_not_found"
	errorAudienceEmpty           = "audience_empty"
	errorTransmissionNotFound    = "transmission_not_found"
	errorMediaNotReady           = "media_not_ready"
	errorTransmissionIdempotency = "transmission_idempotency_conflict"
	errorRequiresConfirmation    = "requires_confirmation"
	errorConfirmationInvalid     = "confirmation_invalid"
	errorTransmissionState       = "transmission_state_conflict"
	errorDeliveryKindMismatch    = "delivery_kind_mismatch"
	errorDeliveryNotSupported    = "delivery_not_supported"
	errorUnsupportedTargets      = "unsupported_targets"
	errorOverlayDuration         = "overlay_duration_exceeded"
)

type transmissionPresenceKey struct {
	OrbitID int64
	Slot    string
}

type transmissionPresenceState struct {
	Connected            bool
	LastSeenAt           int64
	CredentialTokenHash  string
	MediaClipCapable     bool
	OverlayCapable       bool
	InterruptCapable     bool
	MainActive           bool
	PlaybackState        string
	OutputDegraded       bool
	InterruptResumeReady bool
	Capabilities         []string
}

type transmissionPresenceSnapshotter func() map[transmissionPresenceKey]transmissionPresenceState

func transmissionPresenceSnapshotterForHub(h *hub.Hub) transmissionPresenceSnapshotter {
	return func() map[transmissionPresenceKey]transmissionPresenceState {
		snapshots := h.NodeSnapshots()
		result := make(map[transmissionPresenceKey]transmissionPresenceState, len(snapshots))
		for key, snapshot := range snapshots {
			mediaClip := snapshot.Capabilities.Supports(protocol.CapabilityMediaClip)
			overlay := snapshot.Capabilities.Supports(protocol.CapabilityOverlayMix)
			interrupt := snapshot.Capabilities.Supports(protocol.CapabilityInterruptResume)
			result[transmissionPresenceKey{OrbitID: key.Orbit, Slot: string(key.Slot)}] =
				transmissionPresenceState{
					Connected: snapshot.Connected, LastSeenAt: snapshot.LastSeenAt,
					CredentialTokenHash: snapshot.CredentialTokenHash,
					MediaClipCapable:    mediaClip, OverlayCapable: overlay,
					InterruptCapable: interrupt,
					// Until the later presence/client-hook tasks expose a finer
					// runtime signal, an authenticated exact-resume capability is
					// the conservative readiness boundary.
					MainActive: true, PlaybackState: snapshot.PlaybackState,
					OutputDegraded:       snapshot.OutputDegraded,
					InterruptResumeReady: interrupt,
					Capabilities:         snapshot.Capabilities.Values(),
				}
		}
		return result
	}
}

func newTransmissionConfirmationToken() (string, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "fc_" + hex.EncodeToString(raw), nil
}

func transmissionDigest(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

type transmissionAudienceTargetRequest struct {
	Reference string `json:"reference"`
}

type transmissionAudienceRequest struct {
	Kind    string                              `json:"kind"`
	Targets []transmissionAudienceTargetRequest `json:"targets,omitempty"`
}

type transmissionFallbackRequest struct {
	Token    string `json:"token"`
	Delivery string `json:"delivery"`
}

type createTransmissionRequest struct {
	MediaID              string                       `json:"media_id"`
	Audience             transmissionAudienceRequest  `json:"audience"`
	Delivery             string                       `json:"delivery"`
	OriginKind           string                       `json:"origin_kind"`
	IncludeOrigin        *bool                        `json:"include_origin,omitempty"`
	FallbackConfirmation *transmissionFallbackRequest `json:"fallback_confirmation,omitempty"`
}

type canonicalTransmissionRequest struct {
	MediaID       string                      `json:"media_id"`
	Audience      transmissionAudienceRequest `json:"audience"`
	Delivery      string                      `json:"delivery"`
	OriginKind    string                      `json:"origin_kind"`
	IncludeOrigin bool                        `json:"include_origin"`
}

func scanStrictJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token == nil {
		return errors.New("explicit null")
	}
	delimiter, isDelimiter := token.(json.Delim)
	if !isDelimiter {
		return nil
	}
	switch delimiter {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("non-string object key")
			}
			if _, duplicate := seen[key]; duplicate {
				return errors.New("duplicate object key")
			}
			seen[key] = struct{}{}
			if err := scanStrictJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim('}') {
			return errors.New("unterminated object")
		}
	case '[':
		for decoder.More() {
			if err := scanStrictJSONValue(decoder); err != nil {
				return err
			}
		}
		end, err := decoder.Token()
		if err != nil || end != json.Delim(']') {
			return errors.New("unterminated array")
		}
	default:
		return errors.New("unexpected delimiter")
	}
	return nil
}

func decodeStrictTransmissionJSON(
	w http.ResponseWriter,
	r *http.Request,
	target any,
) bool {
	return decodeStrictJSON(w, r, transmissionRequestMaxBytes, target)
}

func decodeStrictJSON(w http.ResponseWriter, r *http.Request, maxBytes int64, target any) bool {
	raw, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxBytes))
	if err != nil || !utf8.Valid(raw) {
		return false
	}
	raw = bytes.TrimSpace(raw)
	if len(raw) < 2 || raw[0] != '{' {
		return false
	}
	shape := json.NewDecoder(bytes.NewReader(raw))
	shape.UseNumber()
	if err := scanStrictJSONValue(shape); err != nil {
		return false
	}
	if _, err := shape.Token(); !errors.Is(err, io.EOF) {
		return false
	}
	strict := json.NewDecoder(bytes.NewReader(raw))
	strict.DisallowUnknownFields()
	if err := strict.Decode(target); err != nil {
		return false
	}
	var extra any
	return errors.Is(strict.Decode(&extra), io.EOF)
}

func defaultIncludeOrigin(origin store.TransmissionOriginKind) bool {
	return origin != store.TransmissionOriginMicrophone
}

func validateTransmissionCreateRequest(
	request createTransmissionRequest,
) (bool, []store.TransmissionAudienceSelector) {
	if !transmissionMediaID.MatchString(request.MediaID) {
		return false, nil
	}
	switch store.TransmissionOriginKind(request.OriginKind) {
	case store.TransmissionOriginMicrophone, store.TransmissionOriginFile,
		store.TransmissionOriginTelegram, store.TransmissionOriginBuiltin:
	default:
		return false, nil
	}
	switch request.Delivery {
	case "overlay", "interrupt", "after_current":
	case "queue", "replace":
	default:
		return false, nil
	}
	audienceKind := store.TransmissionAudienceKind(request.Audience.Kind)
	selectors := make([]store.TransmissionAudienceSelector, 0, len(request.Audience.Targets))
	switch audienceKind {
	case store.TransmissionAudienceThisPulsar,
		store.TransmissionAudienceOwnBarycenter,
		store.TransmissionAudienceCurrentAir:
		if request.Audience.Targets != nil {
			return false, nil
		}
	case store.TransmissionAudienceExplicit:
		if len(request.Audience.Targets) == 0 || len(request.Audience.Targets) > 64 {
			return false, nil
		}
		for _, target := range request.Audience.Targets {
			if !transmissionTargetReference.MatchString(target.Reference) {
				return false, nil
			}
			selectors = append(selectors, store.TransmissionAudienceSelector{
				Reference: target.Reference,
			})
		}
	default:
		return false, nil
	}
	includeOrigin := defaultIncludeOrigin(store.TransmissionOriginKind(request.OriginKind))
	if request.IncludeOrigin != nil {
		includeOrigin = *request.IncludeOrigin
	}
	if audienceKind == store.TransmissionAudienceThisPulsar && !includeOrigin {
		return false, nil
	}
	if request.FallbackConfirmation != nil {
		if request.Delivery != "interrupt" ||
			!transmissionConfirmationToken.MatchString(request.FallbackConfirmation.Token) ||
			(request.FallbackConfirmation.Delivery != "overlay" &&
				request.FallbackConfirmation.Delivery != "after_current") {
			return false, nil
		}
	}
	return true, selectors
}

type transmissionAPIErrorBody struct {
	Error struct {
		Code    string `json:"code"`
		Message string `json:"message"`
		Details any    `json:"details,omitempty"`
	} `json:"error"`
}

func writeTransmissionAPIError(
	w http.ResponseWriter,
	status int,
	code string,
	details any,
) {
	messages := map[string]string{
		errorInvalidRequest:          "The request is malformed or contains invalid parameters.",
		errorUnauthorized:            "Authentication is required.",
		errorInsufficientCapability:  "This token does not have the required capability.",
		errorAirPolicyDenied:         "The current Air policy does not allow this operation.",
		errorMediaNotFound:           "The media item was not found.",
		errorMediaNotReady:           "The media item is not ready.",
		errorAudienceNotFound:        "The selected audience was not found.",
		errorAudienceEmpty:           "The selected audience contains no installation.",
		errorTransmissionNotFound:    "The transmission was not found.",
		errorTransmissionIdempotency: "The idempotency key was already used for different input.",
		errorRequiresConfirmation:    "Interrupt cannot be honored for all selected targets; choose an explicit fallback.",
		errorConfirmationInvalid:     "The fallback confirmation is invalid or expired.",
		errorTransmissionState:       "The transmission cannot be changed in its current state.",
		errorDeliveryKindMismatch:    "The media kind and delivery mode cannot be combined.",
		errorDeliveryNotSupported:    "The requested delivery mode is not supported in this phase.",
		errorUnsupportedTargets:      "One or more selected targets do not support the requested delivery.",
		errorOverlayDuration:         "Overlay is limited to 60 seconds.",
		errorContentPolicyAcceptance: "Accept the current content policy before this action.",
		errorInternal:                "An internal error occurred.",
	}
	var body transmissionAPIErrorBody
	body.Error.Code = code
	body.Error.Message = messages[code]
	body.Error.Details = details
	writeJSON(w, status, body)
}

type transmissionAlternativeResponse struct {
	Delivery  string `json:"delivery"`
	Available bool   `json:"available"`
	Reason    string `json:"reason,omitempty"`
}

type transmissionChallengeDetails struct {
	ConfirmationToken string                            `json:"confirmation_token"`
	ExpiresAt         string                            `json:"expires_at"`
	Alternatives      []transmissionAlternativeResponse `json:"alternatives"`
}

type transmissionAlternativesDetails struct {
	Alternatives []transmissionAlternativeResponse `json:"alternatives"`
}

type transmissionUnsupportedTargetResponse struct {
	Reference           string   `json:"reference"`
	MissingCapabilities []string `json:"missing_capabilities"`
}

type transmissionUnsupportedTargetsDetails struct {
	Targets []transmissionUnsupportedTargetResponse `json:"targets"`
}

func transmissionAlternatives(
	alternatives []store.TransmissionAlternative,
) []transmissionAlternativeResponse {
	result := make([]transmissionAlternativeResponse, 0, len(alternatives))
	for _, alternative := range alternatives {
		result = append(result, transmissionAlternativeResponse{
			Delivery: string(alternative.Delivery), Available: alternative.Available,
			Reason: alternative.Reason,
		})
	}
	return result
}

func writeTransmissionChallenge(
	w http.ResponseWriter,
	challenge store.TransmissionChallenge,
	token string,
) {
	writeTransmissionAPIError(w, http.StatusConflict, errorRequiresConfirmation,
		transmissionChallengeDetails{
			ConfirmationToken: token,
			ExpiresAt:         formatTransmissionTime(challenge.ExpiresAt),
			Alternatives:      transmissionAlternatives(challenge.Alternatives),
		},
	)
}

func (api *onboardingAPI) transmissionStoreError(w http.ResponseWriter, operation string, err error) bool {
	if err == nil {
		return false
	}
	var duration *store.TransmissionOverlayDurationError
	var unsupported *store.TransmissionUnsupportedTargetsError
	switch {
	case errors.Is(err, store.ErrUnauthorized):
		writeTransmissionAPIError(w, http.StatusUnauthorized, errorUnauthorized, nil)
	case errors.Is(err, store.ErrInsufficientCapability), errors.Is(err, store.ErrOrbitDisabled):
		writeTransmissionAPIError(w, http.StatusForbidden, errorInsufficientCapability, nil)
	case errors.Is(err, store.ErrAirPolicyDenied):
		writeTransmissionAPIError(w, http.StatusForbidden, errorAirPolicyDenied, nil)
	case errors.Is(err, store.ErrTransmissionMediaNotFound),
		errors.Is(err, store.ErrTransmissionMediaInvalid):
		writeTransmissionAPIError(w, http.StatusNotFound, errorMediaNotFound, nil)
	case errors.Is(err, store.ErrTransmissionMediaNotReady):
		writeTransmissionAPIError(w, http.StatusConflict, errorMediaNotReady, nil)
	case errors.Is(err, store.ErrContentPolicyAcceptanceRequired):
		writeTransmissionAPIError(w, http.StatusPreconditionRequired,
			errorContentPolicyAcceptance, nil)
	case errors.Is(err, store.ErrTransmissionAudienceNotFound):
		writeTransmissionAPIError(w, http.StatusNotFound, errorAudienceNotFound, nil)
	case errors.Is(err, store.ErrTransmissionAudienceEmpty):
		writeTransmissionAPIError(w, http.StatusUnprocessableEntity, errorAudienceEmpty, nil)
	case errors.Is(err, store.ErrTransmissionIdempotencyConflict):
		writeTransmissionAPIError(w, http.StatusConflict, errorTransmissionIdempotency, nil)
	case errors.Is(err, store.ErrTransmissionConfirmationInvalid):
		writeTransmissionAPIError(w, http.StatusConflict, errorConfirmationInvalid, nil)
	case errors.Is(err, store.ErrTransmissionDeliveryKindMismatch):
		writeTransmissionAPIError(w, http.StatusUnprocessableEntity, errorDeliveryKindMismatch, nil)
	case errors.As(err, &unsupported):
		details := transmissionUnsupportedTargetsDetails{
			Targets: make([]transmissionUnsupportedTargetResponse, 0, len(unsupported.Targets)),
		}
		for _, target := range unsupported.Targets {
			details.Targets = append(details.Targets, transmissionUnsupportedTargetResponse{
				Reference: target.Reference, MissingCapabilities: target.MissingCapabilities,
			})
		}
		writeTransmissionAPIError(w, http.StatusUnprocessableEntity, errorUnsupportedTargets, details)
	case errors.As(err, &duration):
		writeTransmissionAPIError(w, http.StatusUnprocessableEntity, errorOverlayDuration,
			transmissionAlternativesDetails{Alternatives: transmissionAlternatives(duration.Alternatives)})
	case errors.Is(err, store.ErrTransmissionNotFound):
		writeTransmissionAPIError(w, http.StatusNotFound, errorTransmissionNotFound, nil)
	case errors.Is(err, store.ErrTransmissionStateConflict):
		writeTransmissionAPIError(w, http.StatusConflict, errorTransmissionState, nil)
	case errors.Is(err, store.ErrTransmissionInvalid):
		writeTransmissionAPIError(w, http.StatusBadRequest, errorInvalidRequest, nil)
	default:
		api.log.Error(operation, "err", err)
		writeTransmissionAPIError(w, http.StatusInternalServerError, errorInternal, nil)
	}
	return true
}

func canonicalTransmissionHash(
	request createTransmissionRequest,
	includeOrigin bool,
) (string, error) {
	canonical, err := json.Marshal(canonicalTransmissionRequest{
		MediaID: request.MediaID, Audience: request.Audience,
		Delivery: request.Delivery, OriginKind: request.OriginKind,
		IncludeOrigin: includeOrigin,
	})
	if err != nil {
		return "", err
	}
	return transmissionDigest(string(canonical)), nil
}

func (api *onboardingAPI) transmissionAvailability() []store.TransmissionTargetAvailability {
	snapshot := api.transmissionPresence()
	result := make([]store.TransmissionTargetAvailability, 0, len(snapshot))
	for key, state := range snapshot {
		result = append(result, store.TransmissionTargetAvailability{
			OrbitID: key.OrbitID, Slot: key.Slot,
			Connected: state.Connected, LastSeenAt: state.LastSeenAt,
			CredentialTokenHash:  state.CredentialTokenHash,
			MediaClipCapable:     state.MediaClipCapable,
			OverlayCapable:       state.OverlayCapable,
			InterruptCapable:     state.InterruptCapable,
			MainActive:           state.MainActive,
			InterruptResumeReady: state.InterruptResumeReady,
			Capabilities:         append([]string(nil), state.Capabilities...),
		})
	}
	return result
}

type transmissionTargetReferenceJSON struct {
	Reference string `json:"reference"`
	Kind      string `json:"kind"`
	Label     string `json:"label"`
}

type transmissionTargetReferencesJSON struct {
	Contract string                            `json:"contract"`
	Targets  []transmissionTargetReferenceJSON `json:"targets"`
}

func (api *onboardingAPI) transmissionTargetReferences(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.URL.Path != "/v1/transmission-targets" ||
		r.URL.RawQuery != "" || r.ContentLength > 0 {
		writeTransmissionAPIError(w, http.StatusBadRequest, errorInvalidRequest, nil)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	options, err := api.store.ListTransmissionTargetReferences(
		actor.Context.ActorID, actor.Bearer, api.transmissionNow().UTC().UnixMilli(),
	)
	if api.transmissionStoreError(w, "list transmission target references", err) {
		return
	}
	response := transmissionTargetReferencesJSON{
		Contract: "p2-targets-inbox-parity.v1",
		Targets:  make([]transmissionTargetReferenceJSON, 0, len(options)),
	}
	for _, option := range options {
		response.Targets = append(response.Targets, transmissionTargetReferenceJSON{
			Reference: option.Reference, Kind: string(option.Kind), Label: option.Label,
		})
	}
	writeJSON(w, http.StatusOK, response)
}

func (api *onboardingAPI) reserveTransmission(w http.ResponseWriter, actor store.ActorContext) bool {
	if allowed, retry := api.transmissionActor.reserve(strconv.FormatInt(actor.ActorID, 10)); !allowed {
		apiError(w, http.StatusTooManyRequests, errorTooManyAttempts, retry)
		return false
	}
	if allowed, retry := api.transmissionOrbit.reserve(strconv.FormatInt(actor.OrbitID, 10)); !allowed {
		apiError(w, http.StatusTooManyRequests, errorTooManyAttempts, retry)
		return false
	}
	return true
}

func (api *onboardingAPI) createTransmission(w http.ResponseWriter, r *http.Request) {
	ingress := api.transmissionNow().UTC()
	if r.Method != http.MethodPost || r.URL.Path != "/v1/transmissions" ||
		r.URL.RawQuery != "" {
		writeTransmissionAPIError(w, http.StatusBadRequest, errorInvalidRequest, nil)
		return
	}
	idempotencyKey, ok := singleRequestHeader(r, "Idempotency-Key")
	if !ok || !transmissionIdempotencyKey.MatchString(idempotencyKey) {
		writeTransmissionAPIError(w, http.StatusBadRequest, errorInvalidRequest, nil)
		return
	}
	var request createTransmissionRequest
	if !decodeStrictTransmissionJSON(w, r, &request) {
		writeTransmissionAPIError(w, http.StatusBadRequest, errorInvalidRequest, nil)
		return
	}
	valid, selectors := validateTransmissionCreateRequest(request)
	if !valid {
		writeTransmissionAPIError(w, http.StatusBadRequest, errorInvalidRequest, nil)
		return
	}
	if request.Delivery == "queue" || request.Delivery == "replace" {
		writeTransmissionAPIError(w, http.StatusUnprocessableEntity, errorDeliveryNotSupported, nil)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	if !api.reserveTransmission(w, actor.Context) {
		return
	}
	includeOrigin := defaultIncludeOrigin(store.TransmissionOriginKind(request.OriginKind))
	if request.IncludeOrigin != nil {
		includeOrigin = *request.IncludeOrigin
	}
	requestHash, err := canonicalTransmissionHash(request, includeOrigin)
	if err != nil {
		api.log.Error("hash transmission request", "err", err)
		writeTransmissionAPIError(w, http.StatusInternalServerError, errorInternal, nil)
		return
	}
	challengeToken := ""
	if request.Delivery == "interrupt" {
		challengeToken, err = api.transmissionToken()
		if err != nil || !transmissionConfirmationToken.MatchString(challengeToken) {
			api.log.Error("mint transmission confirmation", "err", err)
			writeTransmissionAPIError(w, http.StatusInternalServerError, errorInternal, nil)
			return
		}
	}
	params := store.CreateResolvedTransmissionParams{
		ExpectedActorID: actor.Context.ActorID, Bearer: actor.Bearer,
		IdempotencyKeyHash: transmissionDigest(idempotencyKey), RequestHash: requestHash,
		MediaID:      request.MediaID,
		AudienceKind: store.TransmissionAudienceKind(request.Audience.Kind),
		Selectors:    selectors, OriginKind: store.TransmissionOriginKind(request.OriginKind),
		IncludeOrigin:     includeOrigin,
		RequestedDelivery: store.TransmissionDelivery(request.Delivery),
		AcceptedAt:        ingress.UnixMilli(), Availability: api.transmissionAvailability(),
	}
	if challengeToken != "" {
		params.ChallengeTokenHash = transmissionDigest(challengeToken)
	}
	if request.FallbackConfirmation != nil {
		params.Confirmation = &store.ConfirmTransmissionFallback{
			TokenHash: transmissionDigest(request.FallbackConfirmation.Token),
			Delivery:  store.TransmissionDelivery(request.FallbackConfirmation.Delivery),
		}
	}
	result, err := api.store.CreateResolvedTransmission(params)
	if api.transmissionStoreError(w, "create transmission", err) {
		return
	}
	if result.Challenge != nil {
		writeTransmissionChallenge(w, *result.Challenge, challengeToken)
		return
	}
	status := http.StatusCreated
	if result.Reused {
		status = http.StatusOK
	}
	api.transmissionAccepted(result.Creation.Transmission.ID)
	w.Header().Set("Location", "/v1/transmissions/"+result.Creation.Transmission.ID)
	reused := result.Reused
	writeJSON(w, status, transmissionResponseForCreation(result.Creation, &reused))
}

type transmissionAudienceResponse struct {
	Kind        string `json:"kind"`
	TargetCount int    `json:"target_count"`
}

type transmissionTargetCountsResponse struct {
	Accepted       int `json:"accepted"`
	Preparing      int `json:"preparing"`
	Ready          int `json:"ready"`
	Scheduled      int `json:"scheduled"`
	Playing        int `json:"playing"`
	Cancelling     int `json:"cancelling"`
	Played         int `json:"played"`
	MissedOffline  int `json:"missed_offline"`
	MissedDND      int `json:"missed_dnd"`
	MissedNotReady int `json:"missed_not_ready"`
	Blocked        int `json:"blocked"`
	Failed         int `json:"failed"`
	Cancelled      int `json:"cancelled"`
	Expired        int `json:"expired"`
}

type transmissionTargetResponse struct {
	OrbitID     int64  `json:"orbit_id"`
	Slot        string `json:"slot"`
	Status      string `json:"status"`
	ReasonCode  string `json:"reason_code,omitempty"`
	ReadyAt     string `json:"ready_at,omitempty"`
	ScheduledAt string `json:"scheduled_at,omitempty"`
	StartedAt   string `json:"started_at,omitempty"`
	EndedAt     string `json:"ended_at,omitempty"`
}

type transmissionHTTPResponse struct {
	TransmissionID    string                           `json:"transmission_id"`
	MediaID           string                           `json:"media_id"`
	Audience          transmissionAudienceResponse     `json:"audience"`
	OriginKind        string                           `json:"origin_kind"`
	IncludeOrigin     bool                             `json:"include_origin"`
	RequestedDelivery string                           `json:"requested_delivery"`
	EffectiveDelivery string                           `json:"effective_delivery"`
	DowngradeReason   string                           `json:"downgrade_reason,omitempty"`
	AcceptedAt        string                           `json:"accepted_at"`
	ExpiresAt         string                           `json:"expires_at"`
	Status            string                           `json:"status"`
	ReasonCode        string                           `json:"reason_code,omitempty"`
	TargetCounts      transmissionTargetCountsResponse `json:"target_counts"`
	CanCancel         bool                             `json:"can_cancel"`
	Reused            *bool                            `json:"reused,omitempty"`
	Targets           *[]transmissionTargetResponse    `json:"targets,omitempty"`
}

func formatTransmissionTime(milliseconds int64) string {
	return time.UnixMilli(milliseconds).UTC().Format("2006-01-02T15:04:05.000Z")
}

func targetCountsFromTargets(targets []store.TransmissionTarget) map[store.TransmissionTargetStatus]int {
	counts := make(map[store.TransmissionTargetStatus]int)
	for _, target := range targets {
		counts[target.Status]++
	}
	return counts
}

func targetCountsResponse(
	counts map[store.TransmissionTargetStatus]int,
) transmissionTargetCountsResponse {
	return transmissionTargetCountsResponse{
		Accepted:       counts[store.TransmissionTargetAccepted],
		Preparing:      counts[store.TransmissionTargetPreparing],
		Ready:          counts[store.TransmissionTargetReady],
		Scheduled:      counts[store.TransmissionTargetScheduled],
		Playing:        counts[store.TransmissionTargetPlaying],
		Cancelling:     counts[store.TransmissionTargetCancelling],
		Played:         counts[store.TransmissionTargetPlayed],
		MissedOffline:  counts[store.TransmissionTargetMissedOffline],
		MissedDND:      counts[store.TransmissionTargetMissedDND],
		MissedNotReady: counts[store.TransmissionTargetMissedNotReady],
		Blocked:        counts[store.TransmissionTargetBlocked],
		Failed:         counts[store.TransmissionTargetFailed],
		Cancelled:      counts[store.TransmissionTargetCancelled],
		Expired:        counts[store.TransmissionTargetExpired],
	}
}

func targetResponses(targets []store.TransmissionTarget) []transmissionTargetResponse {
	result := make([]transmissionTargetResponse, 0, len(targets))
	for _, target := range targets {
		response := transmissionTargetResponse{
			OrbitID: target.OrbitID, Slot: target.Slot,
			Status: string(target.Status), ReasonCode: string(target.ReasonCode),
		}
		if target.Status == store.TransmissionTargetBlocked ||
			target.ReasonCode == store.TransmissionReasonReported {
			// Reveal only that delivery was blocked, never which actor- or
			// orbit-scoped rule produced the decision. A local report-driven
			// cancellation likewise never identifies its reporter to a sender.
			response.ReasonCode = ""
		}
		if target.ReadyAt != 0 {
			response.ReadyAt = formatTransmissionTime(target.ReadyAt)
		}
		if target.ScheduledAt != 0 {
			response.ScheduledAt = formatTransmissionTime(target.ScheduledAt)
		}
		if target.StartedAt != 0 {
			response.StartedAt = formatTransmissionTime(target.StartedAt)
		}
		if target.EndedAt != 0 {
			response.EndedAt = formatTransmissionTime(target.EndedAt)
		}
		result = append(result, response)
	}
	return result
}

func canCancelCreation(creation store.TransmissionCreation) bool {
	if creation.Transmission.CancellationCause != "" ||
		creation.Transmission.CompletedAt != 0 {
		return false
	}
	for _, target := range creation.Targets {
		if target.Status == store.TransmissionTargetPlaying ||
			target.Status == store.TransmissionTargetPlayed ||
			target.Status == store.TransmissionTargetCancelling {
			return false
		}
	}
	return true
}

func transmissionResponse(
	transmission store.Transmission,
	targetCount int,
	counts map[store.TransmissionTargetStatus]int,
	canCancel bool,
) transmissionHTTPResponse {
	return transmissionHTTPResponse{
		TransmissionID: transmission.ID, MediaID: transmission.MediaID,
		Audience: transmissionAudienceResponse{
			Kind: string(transmission.AudienceKind), TargetCount: targetCount,
		},
		OriginKind: string(transmission.OriginKind), IncludeOrigin: transmission.IncludeOrigin,
		RequestedDelivery: string(transmission.RequestedDelivery),
		EffectiveDelivery: string(transmission.EffectiveDelivery),
		DowngradeReason:   transmission.DowngradeReason,
		AcceptedAt:        formatTransmissionTime(transmission.AcceptedAt),
		ExpiresAt:         formatTransmissionTime(transmission.ExpiresAt),
		Status:            string(transmission.Status), ReasonCode: string(transmission.ReasonCode),
		TargetCounts: targetCountsResponse(counts), CanCancel: canCancel,
	}
}

func transmissionResponseForCreation(
	creation store.TransmissionCreation,
	reused *bool,
) transmissionHTTPResponse {
	response := transmissionResponse(
		creation.Transmission, len(creation.Targets),
		targetCountsFromTargets(creation.Targets), canCancelCreation(creation),
	)
	response.Reused = reused
	return response
}

func parseTransmissionItemPath(path string) (string, bool, bool) {
	suffix := strings.TrimPrefix(path, "/v1/transmissions/")
	parts := strings.Split(suffix, "/")
	if len(parts) == 1 && transmissionHTTPID.MatchString(parts[0]) {
		return parts[0], false, true
	}
	if len(parts) == 2 && transmissionHTTPID.MatchString(parts[0]) && parts[1] == "cancel" {
		return parts[0], true, true
	}
	return "", false, false
}

func (api *onboardingAPI) transmissionItem(w http.ResponseWriter, r *http.Request) {
	transmissionID, cancel, ok := parseTransmissionItemPath(r.URL.Path)
	if !ok || r.URL.RawQuery != "" {
		writeTransmissionAPIError(w, http.StatusBadRequest, errorInvalidRequest, nil)
		return
	}
	if cancel {
		api.cancelTransmission(w, r, transmissionID)
		return
	}
	if r.Method != http.MethodGet {
		writeTransmissionAPIError(w, http.StatusBadRequest, errorInvalidRequest, nil)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	view, err := api.store.GetAuthorizedTransmission(
		actor.Context.ActorID, actor.Bearer, transmissionID,
	)
	if api.transmissionStoreError(w, "get transmission", err) {
		return
	}
	response := transmissionResponse(
		view.Transmission, view.TargetCount, view.TargetStatusCounts, view.CanCancel,
	)
	targets := targetResponses(view.Targets)
	response.Targets = &targets
	writeJSON(w, http.StatusOK, response)
}

type cancelTransmissionResponse struct {
	TransmissionID string `json:"transmission_id"`
	Status         string `json:"status"`
	Changed        bool   `json:"changed"`
	ReasonCode     string `json:"reason_code"`
}

func (api *onboardingAPI) cancelTransmission(
	w http.ResponseWriter,
	r *http.Request,
	transmissionID string,
) {
	if r.Method != http.MethodPost {
		writeTransmissionAPIError(w, http.StatusBadRequest, errorInvalidRequest, nil)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	if !actor.Context.Capabilities.Has(store.CapabilityControl) {
		writeTransmissionAPIError(w, http.StatusForbidden, errorInsufficientCapability, nil)
		return
	}
	var empty struct{}
	if !decodeStrictTransmissionJSON(w, r, &empty) {
		writeTransmissionAPIError(w, http.StatusBadRequest, errorInvalidRequest, nil)
		return
	}
	result, err := api.store.CancelAuthorizedTransmission(
		actor.Context.ActorID, actor.Bearer, transmissionID,
		api.transmissionNow().UTC().UnixMilli(),
	)
	if api.transmissionStoreError(w, "cancel transmission", err) {
		return
	}
	api.transmissionCancelled(result)
	writeJSON(w, http.StatusOK, cancelTransmissionResponse{
		TransmissionID: result.Transmission.ID,
		Status:         string(result.Transmission.Status), Changed: result.Changed,
		ReasonCode: string(store.TransmissionReasonSenderCancelled),
	})
}
