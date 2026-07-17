package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	automationcontract "relux.works/duet/coordinator/internal/automation"
	"relux.works/duet/coordinator/internal/store"
)

const automationRequestMaxBytes = 16 << 10

var (
	savedCueHTTPIDPattern        = regexp.MustCompile(`^cq_[0-9A-HJKMNP-TV-Z]{26}$`)
	automationScheduleHTTPID     = regexp.MustCompile(`^sch_[0-9A-HJKMNP-TV-Z]{26}$`)
	automationPrincipalHTTPID    = regexp.MustCompile(`^ap_[0-9A-HJKMNP-TV-Z]{26}$`)
	automationTargetRefHTTP      = regexp.MustCompile(`^trf_[A-Za-z0-9_-]{43}$`)
	automationLocalMinutePattern = regexp.MustCompile(`^(?:[01][0-9]|2[0-3]):[0-5][0-9]$`)
	automationIdempotencyKey     = regexp.MustCompile(`^[!-~]{8,512}$`)
)

type automationTriggerInput struct {
	Secret           string
	IdempotencyKey   string
	CueID            string
	AudienceKind     automationcontract.AudienceKind
	TargetReferences []string
	Delivery         automationcontract.Delivery
	Now              int64
}

type automationTriggerOutput struct {
	Execution store.AutomationExecution
	Replayed  bool
}

type automationTriggerService interface {
	TriggerAutomation(automationTriggerInput) (automationTriggerOutput, error)
}

func (api *onboardingAPI) automationOriginBoundary(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Production keeps the scoped trigger indistinguishable from an absent
		// route until the later runtime task deliberately installs a service.
		if r.URL.Path == automationcontract.TriggerPath && api.automationTrigger == nil {
			http.NotFound(w, r)
			return
		}
		if len(r.Header.Values("Origin")) != 0 {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		next(w, r)
	}
}

func automationJSONRequest(r *http.Request) bool {
	values := r.Header.Values("Content-Type")
	return len(values) == 1 && values[0] == "application/json"
}

func hashAutomationHTTP(value string) string {
	digest := sha256.Sum256([]byte(value))
	return hex.EncodeToString(digest[:])
}

func automationControlMutationAuth(r *http.Request, actor actorRequest, request any, now time.Time) (store.AutomationControlAuth, bool) {
	key, ok := singleRequestHeader(r, "Idempotency-Key")
	if !ok || !automationIdempotencyKey.MatchString(key) {
		return store.AutomationControlAuth{}, false
	}
	canonical, err := json.Marshal(request)
	if err != nil {
		return store.AutomationControlAuth{}, false
	}
	requestEnvelope := r.Method + "\n" + r.URL.Path + "\n" + string(canonical)
	return store.AutomationControlAuth{
		ExpectedActorID: actor.Context.ActorID, Bearer: actor.Bearer,
		IdempotencyKeyHash: hashAutomationHTTP(key), RequestHash: hashAutomationHTTP(requestEnvelope),
		Now: now.UTC().UnixMilli(),
	}, true
}

func automationEmptyBody(r *http.Request) bool {
	return r.ContentLength <= 0 && len(r.TransferEncoding) == 0
}

func (api *onboardingAPI) automationControlError(w http.ResponseWriter, operation, notFoundCode string, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, store.ErrUnauthorized):
		apiError(w, http.StatusUnauthorized, errorUnauthorized, 0)
	case errors.Is(err, store.ErrInsufficientCapability), errors.Is(err, store.ErrOrbitDisabled):
		apiError(w, http.StatusForbidden, errorForbidden, 0)
	case errors.Is(err, store.ErrContentPolicyAcceptanceRequired):
		apiError(w, http.StatusPreconditionRequired, errorContentPolicyAcceptance, 0)
	case errors.Is(err, store.ErrAutomationControlIdempotencyConflict),
		errors.Is(err, store.ErrAutomationIdempotencyConflict):
		apiError(w, http.StatusConflict, errorAirIdempotency, 0)
	case errors.Is(err, store.ErrAutomationStateConflict),
		errors.Is(err, store.ErrSavedCueStateConflict):
		apiError(w, http.StatusConflict, errorAirRevision, 0)
	case errors.Is(err, store.ErrSavedCueNotFound):
		apiError(w, http.StatusNotFound, errorAutomationCueNotFound, 0)
	case errors.Is(err, store.ErrAutomationNotFound):
		apiError(w, http.StatusNotFound, notFoundCode, 0)
	case errors.Is(err, store.ErrSavedCueQuotaExceeded):
		apiError(w, http.StatusConflict, errorAutomationCueQuota, 0)
	case errors.Is(err, store.ErrSavedCueInvalid), errors.Is(err, store.ErrSavedCueDuplicate):
		apiError(w, http.StatusUnprocessableEntity, errorAutomationCueIneligible, 0)
	case errors.Is(err, store.ErrAutomationDisabled):
		apiError(w, http.StatusConflict, errorAutomationDisabled, 0)
	case errors.Is(err, store.ErrAutomationAudienceNotAllowed):
		apiError(w, http.StatusUnprocessableEntity, errorAutomationAudience, 0)
	case errors.Is(err, store.ErrAutomationInvalid):
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
	default:
		api.internalError(w, operation, err)
	}
	return true
}

type savedCueSourceHTTPRequest struct {
	Kind    string `json:"kind"`
	MediaID string `json:"media_id,omitempty"`
	AssetID string `json:"asset_id,omitempty"`
	SHA256  string `json:"sha256,omitempty"`
}

type savedCueCreateHTTPRequest struct {
	Title  string                    `json:"title"`
	Source savedCueSourceHTTPRequest `json:"source"`
}

type savedCueRenameHTTPRequest struct {
	Title            string `json:"title"`
	ExpectedRevision int64  `json:"expected_revision"`
}

type expectedRevisionHTTPRequest struct {
	ExpectedRevision int64 `json:"expected_revision"`
}

type manualSoundboardTriggerHTTPRequest struct {
	Audience             transmissionAudienceRequest  `json:"audience"`
	Delivery             string                       `json:"delivery"`
	IncludeOrigin        *bool                        `json:"include_origin,omitempty"`
	FallbackConfirmation *transmissionFallbackRequest `json:"fallback_confirmation,omitempty"`
}

type savedCueOrderHTTPRequest struct {
	ExpectedOrderRevision int64     `json:"expected_order_revision"`
	CueIDs                *[]string `json:"cue_ids"`
}

type savedCueHTTPResponse struct {
	CueID            string `json:"cue_id"`
	Title            string `json:"title"`
	SourceKind       string `json:"source_kind"`
	MediaID          string `json:"media_id,omitempty"`
	BuiltinAssetID   string `json:"builtin_asset_id,omitempty"`
	SourceSHA256     string `json:"source_sha256"`
	SourceBytes      int64  `json:"source_bytes"`
	SourceDurationMS int64  `json:"source_duration_ms"`
	State            string `json:"state"`
	Revision         int64  `json:"revision"`
	SourceGeneration int64  `json:"source_generation"`
	Position         *int   `json:"position,omitempty"`
}

func savedCueHTTP(cue store.SavedCue, position *int) savedCueHTTPResponse {
	return savedCueHTTPResponse{
		CueID: cue.ID, Title: cue.Title, SourceKind: string(cue.SourceKind),
		MediaID: cue.MediaID, BuiltinAssetID: cue.BuiltinAssetID,
		SourceSHA256: cue.SourceSHA256, SourceBytes: cue.SourceBytes,
		SourceDurationMS: cue.SourceDurationMS, State: string(cue.State),
		Revision: cue.Revision, SourceGeneration: cue.SourceGeneration, Position: position,
	}
}

func (api *onboardingAPI) soundboardCues(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/soundboard/cues" || r.URL.RawQuery != "" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	switch r.Method {
	case http.MethodGet:
		if !automationEmptyBody(r) {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		result, err := api.store.AuthorizedSavedCueControlList(actor.Context.ActorID, actor.Bearer)
		if api.automationControlError(w, "list saved cues", errorAutomationCueNotFound, err) {
			return
		}
		cues := make([]savedCueHTTPResponse, 0, len(result.Items))
		for _, item := range result.Items {
			position := item.Position
			cues = append(cues, savedCueHTTP(item.Cue, &position))
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"order_revision": result.OrderRevision, "cues": cues,
		})
	case http.MethodPost:
		if !automationJSONRequest(r) {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		var request savedCueCreateHTTPRequest
		if !decodeStrictJSON(w, r, automationRequestMaxBytes, &request) {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		params := store.CreateSavedCueControlParams{Title: request.Title}
		switch request.Source.Kind {
		case "media":
			if request.Source.MediaID == "" || request.Source.AssetID != "" || request.Source.SHA256 != "" {
				apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
				return
			}
			params.MediaID = request.Source.MediaID
		case "builtin":
			if request.Source.MediaID != "" || request.Source.AssetID == "" || request.Source.SHA256 == "" {
				apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
				return
			}
			params.BuiltinAssetID, params.BuiltinSHA256 = request.Source.AssetID, request.Source.SHA256
		default:
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		auth, ok := automationControlMutationAuth(r, actor, request, api.automationNow())
		if !ok {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		result, err := api.store.CreateAuthorizedSavedCue(auth, params)
		if api.automationControlError(w, "create saved cue", errorAutomationCueNotFound, err) {
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"cue": savedCueHTTP(result.Cue, nil), "order_revision": result.OrderRevision,
			"replayed": result.Replayed,
		})
	default:
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
	}
}

func (api *onboardingAPI) soundboardCueOrder(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut || r.URL.Path != "/v1/soundboard/cues/order" ||
		r.URL.RawQuery != "" || !automationJSONRequest(r) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	var request savedCueOrderHTTPRequest
	if !decodeStrictJSON(w, r, automationRequestMaxBytes, &request) || request.CueIDs == nil {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	auth, ok := automationControlMutationAuth(r, actor, request, api.automationNow())
	if !ok {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	result, err := api.store.ReorderAuthorizedSavedCues(auth, *request.CueIDs, request.ExpectedOrderRevision)
	if api.automationControlError(w, "reorder saved cues", errorAutomationCueNotFound, err) {
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (api *onboardingAPI) soundboardCueItem(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" || !strings.HasPrefix(r.URL.Path, "/v1/soundboard/cues/") {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	suffix := strings.TrimPrefix(r.URL.Path, "/v1/soundboard/cues/")
	parts := strings.Split(suffix, "/")
	if len(parts) == 2 && savedCueHTTPIDPattern.MatchString(parts[0]) && parts[1] == "trigger" {
		api.manualSoundboardTrigger(w, r, parts[0])
		return
	}
	id := suffix
	if !savedCueHTTPIDPattern.MatchString(id) || !automationJSONRequest(r) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	switch r.Method {
	case http.MethodPatch:
		var request savedCueRenameHTTPRequest
		if !decodeStrictJSON(w, r, automationRequestMaxBytes, &request) {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		auth, ok := automationControlMutationAuth(r, actor, request, api.automationNow())
		if !ok {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		result, err := api.store.RenameAuthorizedSavedCue(auth, id, request.Title, request.ExpectedRevision)
		if api.automationControlError(w, "rename saved cue", errorAutomationCueNotFound, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"cue": savedCueHTTP(result.Cue, nil), "order_revision": result.OrderRevision,
			"replayed": result.Replayed,
		})
	case http.MethodDelete:
		var request expectedRevisionHTTPRequest
		if !decodeStrictJSON(w, r, automationRequestMaxBytes, &request) {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		auth, ok := automationControlMutationAuth(r, actor, request, api.automationNow())
		if !ok {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		result, err := api.store.DeleteAuthorizedSavedCue(auth, id, request.ExpectedRevision)
		if api.automationControlError(w, "delete saved cue", errorAutomationCueNotFound, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"cue": savedCueHTTP(result.Cue, nil), "order_revision": result.OrderRevision,
			"replayed": result.Replayed,
		})
	default:
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
	}
}

func (api *onboardingAPI) manualSoundboardTrigger(w http.ResponseWriter, r *http.Request, cueID string) {
	if r.Method != http.MethodPost || r.URL.RawQuery != "" || !automationJSONRequest(r) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	idempotencyKey, ok := singleRequestHeader(r, "Idempotency-Key")
	if !ok || !transmissionIdempotencyKey.MatchString(idempotencyKey) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	var request manualSoundboardTriggerHTTPRequest
	if !decodeStrictTransmissionJSON(w, r, &request) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	includeOrigin := true
	if request.IncludeOrigin != nil {
		includeOrigin = *request.IncludeOrigin
	}
	probe := createTransmissionRequest{MediaID: "m_00000000000000000000000000",
		Audience: request.Audience, Delivery: request.Delivery, OriginKind: "file",
		IncludeOrigin: &includeOrigin, FallbackConfirmation: request.FallbackConfirmation}
	valid, selectors := validateTransmissionCreateRequest(probe)
	if !valid || request.Delivery == "queue" || request.Delivery == "replace" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	if !api.reserveTransmission(w, actor.Context) {
		return
	}
	now := api.transmissionNow().UTC().UnixMilli()
	if err := api.prepareManualSoundboardBuiltin(actor, cueID, now); err != nil {
		api.automationControlError(w, "prepare manual soundboard cue", errorAutomationCueNotFound, err)
		return
	}
	requestHash, err := canonicalHistoryActionHash(struct {
		CueID         string                      `json:"cue_id"`
		Audience      transmissionAudienceRequest `json:"audience"`
		Delivery      string                      `json:"delivery"`
		IncludeOrigin bool                        `json:"include_origin"`
	}{cueID, request.Audience, request.Delivery, includeOrigin})
	if err != nil {
		api.internalError(w, "hash manual soundboard trigger", err)
		return
	}
	challengeToken := ""
	if request.Delivery == "interrupt" {
		challengeToken, err = api.transmissionToken()
		if err != nil || !transmissionConfirmationToken.MatchString(challengeToken) {
			api.internalError(w, "mint manual soundboard confirmation", err)
			return
		}
	}
	params := store.CreateResolvedTransmissionParams{
		ExpectedActorID: actor.Context.ActorID, Bearer: actor.Bearer,
		IdempotencyKeyHash: transmissionDigest(idempotencyKey), RequestHash: requestHash,
		AudienceKind: store.TransmissionAudienceKind(request.Audience.Kind), Selectors: selectors,
		IncludeOrigin: includeOrigin, RequestedDelivery: store.TransmissionDelivery(request.Delivery),
		AcceptedAt: now, Availability: api.transmissionAvailability(),
	}
	if challengeToken != "" {
		params.ChallengeTokenHash = transmissionDigest(challengeToken)
	}
	if request.FallbackConfirmation != nil {
		params.Confirmation = &store.ConfirmTransmissionFallback{
			TokenHash: transmissionDigest(request.FallbackConfirmation.Token),
			Delivery:  store.TransmissionDelivery(request.FallbackConfirmation.Delivery)}
	}
	result, err := api.store.TriggerManualSoundboard(store.ManualSoundboardTriggerParams{
		CueID: cueID, Transmission: params})
	if errors.Is(err, store.ErrAutomationDisabled) || errors.Is(err, store.ErrSavedCueNotFound) ||
		errors.Is(err, store.ErrSavedCueStateConflict) || errors.Is(err, store.ErrAutomationInvalid) {
		api.automationControlError(w, "trigger manual soundboard cue", errorAutomationCueNotFound, err)
		return
	}
	if api.transmissionStoreError(w, "trigger manual soundboard cue", err) {
		return
	}
	if result.Challenge != nil {
		writeTransmissionChallenge(w, *result.Challenge, challengeToken)
		return
	}
	if !result.Reused {
		api.transmissionAccepted(result.Creation.Transmission.ID)
	}
	status := http.StatusCreated
	if result.Reused {
		status = http.StatusOK
	}
	reused := result.Reused
	response := transmissionResponseForCreation(result.Creation, &reused)
	response.ExecutionID = result.ExecutionID
	writeJSON(w, status, response)
}

type automationFeatureHTTPRequest struct {
	SoundboardEnabled *bool                          `json:"soundboard_enabled"`
	AutomationEnabled *bool                          `json:"automation_enabled"`
	EmergencyDisabled *bool                          `json:"emergency_disabled"`
	Timezone          *string                        `json:"timezone"`
	QuietHours        *[]store.AutomationQuietWindow `json:"quiet_hours"`
	ExpectedRevision  *int64                         `json:"expected_revision"`
}

func automationFeatureHTTP(state store.AutomationFeatureState) map[string]any {
	quiet := []store.AutomationQuietWindow{}
	if state.QuietHoursJSON != "" {
		_ = json.Unmarshal([]byte(state.QuietHoursJSON), &quiet)
	}
	return map[string]any{
		"soundboard_enabled": state.SoundboardEnabled,
		"automation_enabled": state.AutomationEnabled,
		"emergency_disabled": state.EmergencyDisabled,
		"timezone":           state.Timezone, "quiet_hours": quiet,
		"policy_version": state.PolicyVersion, "revision": state.Revision,
		"policy_valid": store.AutomationQuietPolicyValid(
			state.Timezone, state.QuietHoursJSON, state.QuietHoursHash),
		"updated_at": coordTime(state.UpdatedAt),
	}
}

func (api *onboardingAPI) automationStatus(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/automation/status" || r.URL.RawQuery != "" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	switch r.Method {
	case http.MethodGet:
		if !automationEmptyBody(r) {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		state, err := api.store.AuthorizedAutomationFeatureState(actor.Context.ActorID, actor.Bearer)
		if api.automationControlError(w, "read automation feature", errorAutomationSchedule, err) {
			return
		}
		writeJSON(w, http.StatusOK, automationFeatureHTTP(state))
	case http.MethodPut:
		if !automationJSONRequest(r) {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		var request automationFeatureHTTPRequest
		if !decodeStrictJSON(w, r, automationRequestMaxBytes, &request) ||
			request.SoundboardEnabled == nil || request.AutomationEnabled == nil ||
			request.EmergencyDisabled == nil || request.Timezone == nil || request.QuietHours == nil ||
			request.ExpectedRevision == nil {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		auth, ok := automationControlMutationAuth(r, actor, request, api.automationNow())
		if !ok {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		result, err := api.store.ReplaceAuthorizedAutomationFeatureState(auth,
			store.AutomationFeatureControlParams{
				SoundboardEnabled: *request.SoundboardEnabled,
				AutomationEnabled: *request.AutomationEnabled,
				EmergencyDisabled: *request.EmergencyDisabled,
				Timezone:          *request.Timezone, QuietHours: *request.QuietHours,
				ExpectedRevision: *request.ExpectedRevision,
			})
		if api.automationControlError(w, "replace automation feature", errorAutomationSchedule, err) {
			return
		}
		if !result.Replayed {
			if err := api.reconcileAutomationCancellations(auth.Now); err != nil {
				api.internalError(w, "cancel disabled automation", err)
				return
			}
		}
		response := automationFeatureHTTP(result.State)
		response["replayed"] = result.Replayed
		writeJSON(w, http.StatusOK, response)
	default:
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
	}
}

type automationAudienceHTTPRequest struct {
	Kind             automationcontract.AudienceKind `json:"kind"`
	TargetReferences []string                        `json:"target_references,omitempty"`
	AirID            string                          `json:"air_id,omitempty"`
}

type automationScheduleHTTPRequest struct {
	CueID                string                         `json:"cue_id"`
	DisplayName          string                         `json:"display_name"`
	Timezone             string                         `json:"timezone"`
	Weekdays             []int                          `json:"weekdays"`
	LocalTime            string                         `json:"local_time"`
	Audience             automationAudienceHTTPRequest  `json:"audience"`
	Delivery             automationcontract.Delivery    `json:"delivery"`
	AdditionalQuietHours *[]store.AutomationQuietWindow `json:"additional_quiet_hours"`
	PolicyRevision       int64                          `json:"policy_revision"`
	ExpectedRevision     int64                          `json:"expected_revision,omitempty"`
}

func scheduleControlParams(request automationScheduleHTTPRequest) (store.AutomationScheduleControlParams, bool) {
	if request.AdditionalQuietHours == nil ||
		request.Delivery != automationcontract.DeliveryOverlay ||
		!automationLocalMinutePattern.MatchString(request.LocalTime) ||
		len(request.Weekdays) == 0 || len(request.Weekdays) > 7 {
		return store.AutomationScheduleControlParams{}, false
	}
	parts := strings.Split(request.LocalTime, ":")
	hour, _ := strconv.Atoi(parts[0])
	minute, _ := strconv.Atoi(parts[1])
	mask := 0
	for _, weekday := range request.Weekdays {
		if weekday < 0 || weekday > 6 || mask&(1<<weekday) != 0 {
			return store.AutomationScheduleControlParams{}, false
		}
		mask |= 1 << weekday
	}
	for _, reference := range request.Audience.TargetReferences {
		if !automationTargetRefHTTP.MatchString(reference) {
			return store.AutomationScheduleControlParams{}, false
		}
	}
	return store.AutomationScheduleControlParams{
		CueID: request.CueID, DisplayName: request.DisplayName,
		Timezone: request.Timezone, WeekdaysMask: mask, LocalMinute: hour*60 + minute,
		AudienceKind:         request.Audience.Kind,
		TargetReferences:     request.Audience.TargetReferences,
		BoundAirID:           request.Audience.AirID,
		AdditionalQuietHours: *request.AdditionalQuietHours,
		PolicyRevision:       request.PolicyRevision,
	}, true
}

func weekdaysFromMask(mask int) []int {
	var result []int
	for day := 0; day < 7; day++ {
		if mask&(1<<day) != 0 {
			result = append(result, day)
		}
	}
	return result
}

func automationScheduleHTTP(control store.AutomationScheduleControl) map[string]any {
	targets := make([]string, 0, len(control.Targets))
	for _, target := range control.Targets {
		targets = append(targets, target.Digest)
	}
	schedule := control.Schedule
	return map[string]any{
		"schedule_id": schedule.ID, "cue_id": schedule.CueID,
		"display_name": schedule.DisplayName, "timezone": schedule.Timezone,
		"weekdays":   weekdaysFromMask(schedule.WeekdaysMask),
		"local_time": fmt.Sprintf("%02d:%02d", schedule.LocalMinute/60, schedule.LocalMinute%60),
		"audience": map[string]any{
			"kind": schedule.AudienceKind, "target_digests": targets,
			"air_id": schedule.BoundAirID,
		},
		"delivery":               schedule.Delivery,
		"additional_quiet_hours": control.AdditionalQuietHours,
		"policy_version":         schedule.PolicyVersion, "policy_revision": schedule.PolicyRevision,
		"enabled": schedule.Enabled, "revision": schedule.Revision,
		"created_at": coordTime(schedule.CreatedAt), "updated_at": coordTime(schedule.UpdatedAt),
	}
}

func (api *onboardingAPI) automationSchedules(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/automation/schedules" || r.URL.RawQuery != "" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	switch r.Method {
	case http.MethodGet:
		if !automationEmptyBody(r) {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		items, err := api.store.AuthorizedAutomationSchedules(actor.Context.ActorID, actor.Bearer)
		if api.automationControlError(w, "list automation schedules", errorAutomationSchedule, err) {
			return
		}
		result := make([]map[string]any, 0, len(items))
		for _, item := range items {
			result = append(result, automationScheduleHTTP(item))
		}
		writeJSON(w, http.StatusOK, map[string]any{"schedules": result})
	case http.MethodPost:
		if !automationJSONRequest(r) {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		var request automationScheduleHTTPRequest
		if !decodeStrictJSON(w, r, automationRequestMaxBytes, &request) || request.ExpectedRevision != 0 {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		params, ok := scheduleControlParams(request)
		if !ok {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		auth, ok := automationControlMutationAuth(r, actor, request, api.automationNow())
		if !ok {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		result, err := api.store.CreateAuthorizedAutomationSchedule(auth, params)
		if api.automationControlError(w, "create automation schedule", errorAutomationSchedule, err) {
			return
		}
		writeJSON(w, http.StatusCreated, map[string]any{
			"schedule": automationScheduleHTTP(result.Control), "replayed": result.Replayed,
		})
	default:
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
	}
}

func (api *onboardingAPI) automationScheduleItem(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" || !strings.HasPrefix(r.URL.Path, "/v1/automation/schedules/") {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/automation/schedules/"), "/")
	if len(parts) < 1 || !automationScheduleHTTPID.MatchString(parts[0]) || len(parts) > 2 ||
		!automationJSONRequest(r) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	if len(parts) == 1 && r.Method == http.MethodPut {
		var request automationScheduleHTTPRequest
		if !decodeStrictJSON(w, r, automationRequestMaxBytes, &request) || request.ExpectedRevision <= 0 {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		params, ok := scheduleControlParams(request)
		if !ok {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		auth, ok := automationControlMutationAuth(r, actor, request, api.automationNow())
		if !ok {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		result, err := api.store.ReplaceAuthorizedAutomationSchedule(auth, parts[0], request.ExpectedRevision, params)
		if api.automationControlError(w, "replace automation schedule", errorAutomationSchedule, err) {
			return
		}
		if !result.Replayed {
			if err := api.reconcileAutomationCancellations(auth.Now); err != nil {
				api.internalError(w, "cancel replaced automation schedule", err)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"schedule": automationScheduleHTTP(result.Control), "replayed": result.Replayed,
		})
		return
	}
	if len(parts) == 1 && r.Method == http.MethodDelete {
		var request expectedRevisionHTTPRequest
		if !decodeStrictJSON(w, r, automationRequestMaxBytes, &request) {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		auth, ok := automationControlMutationAuth(r, actor, request, api.automationNow())
		if !ok {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		result, err := api.store.DeleteAuthorizedAutomationSchedule(auth, parts[0], request.ExpectedRevision)
		if api.automationControlError(w, "delete automation schedule", errorAutomationSchedule, err) {
			return
		}
		if !result.Replayed {
			if err := api.reconcileAutomationCancellations(auth.Now); err != nil {
				api.internalError(w, "cancel deleted automation schedule", err)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"schedule": automationScheduleHTTP(result.Control), "replayed": result.Replayed,
		})
		return
	}
	if len(parts) == 2 && r.Method == http.MethodPost &&
		(parts[1] == "enable" || parts[1] == "disable") {
		var request expectedRevisionHTTPRequest
		if !decodeStrictJSON(w, r, automationRequestMaxBytes, &request) {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		auth, ok := automationControlMutationAuth(r, actor, request, api.automationNow())
		if !ok {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		result, err := api.store.SetAuthorizedAutomationScheduleEnabled(
			auth, parts[0], request.ExpectedRevision, parts[1] == "enable")
		if api.automationControlError(w, "set automation schedule state", errorAutomationSchedule, err) {
			return
		}
		if !result.Replayed && parts[1] == "disable" {
			if err := api.reconcileAutomationCancellations(auth.Now); err != nil {
				api.internalError(w, "cancel disabled automation schedule", err)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"schedule": automationScheduleHTTP(result.Control), "replayed": result.Replayed,
		})
		return
	}
	apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
}

type automationPrincipalHTTPRequest struct {
	DisplayName      string                            `json:"display_name"`
	AllowedCueIDs    []string                          `json:"allowed_cue_ids"`
	AllowedAudiences []automationcontract.AudienceKind `json:"allowed_audience_kinds"`
	TargetReferences *[]string                         `json:"allowed_target_references"`
	BoundAirID       string                            `json:"bound_air_id,omitempty"`
	MaxTargetCount   int                               `json:"max_target_count"`
	ExpiresAt        string                            `json:"expires_at"`
}

func automationPrincipalHTTP(principal store.AutomationPrincipal) map[string]any {
	return map[string]any{
		"principal_id": principal.ID, "display_name": principal.DisplayName,
		"permission": principal.Permission, "bound_air_id": principal.BoundAirID,
		"max_target_count":       principal.MaxTargetCount,
		"allowed_cue_ids":        principal.AllowedCueIDs,
		"allowed_audience_kinds": principal.AllowedAudiences,
		"allowed_target_digests": principal.TargetRefDigests,
		"issued_at":              coordTime(principal.IssuedAt), "expires_at": coordTime(principal.ExpiresAt),
		"disabled_at": coordTime(principal.DisabledAt), "revoked_at": coordTime(principal.RevokedAt),
		"revision": principal.Revision,
	}
}

func (api *onboardingAPI) automationPrincipals(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/v1/automation/principals" || r.URL.RawQuery != "" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	switch r.Method {
	case http.MethodGet:
		if !automationEmptyBody(r) {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		items, err := api.store.AuthorizedAutomationPrincipals(actor.Context.ActorID, actor.Bearer)
		if api.automationControlError(w, "list automation principals", errorAutomationPrincipal, err) {
			return
		}
		result := make([]map[string]any, 0, len(items))
		for _, item := range items {
			result = append(result, automationPrincipalHTTP(item))
		}
		writeJSON(w, http.StatusOK, map[string]any{"principals": result})
	case http.MethodPost:
		if !automationJSONRequest(r) {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		var request automationPrincipalHTTPRequest
		if !decodeStrictJSON(w, r, automationRequestMaxBytes, &request) || request.TargetReferences == nil {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		expiresAt, err := time.Parse(time.RFC3339, request.ExpiresAt)
		if err != nil {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		for _, reference := range *request.TargetReferences {
			if !automationTargetRefHTTP.MatchString(reference) {
				apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
				return
			}
		}
		auth, ok := automationControlMutationAuth(r, actor, request, api.automationNow())
		if !ok {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		result, err := api.store.IssueAuthorizedAutomationPrincipal(auth,
			store.AutomationPrincipalControlParams{
				DisplayName: request.DisplayName, AllowedCueIDs: request.AllowedCueIDs,
				AllowedAudiences: request.AllowedAudiences,
				TargetReferences: *request.TargetReferences, BoundAirID: request.BoundAirID,
				MaxTargetCount: request.MaxTargetCount, ExpiresAt: expiresAt.UTC().UnixMilli(),
			})
		if api.automationControlError(w, "issue automation principal", errorAutomationPrincipal, err) {
			return
		}
		response := map[string]any{
			"principal":        automationPrincipalHTTP(result.Principal),
			"secret_available": result.SecretAvailable, "replayed": result.Replayed,
		}
		if result.SecretAvailable {
			response["secret"] = result.Secret
		}
		writeJSON(w, http.StatusCreated, response)
	default:
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
	}
}

func (api *onboardingAPI) automationPrincipalItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.RawQuery != "" ||
		!strings.HasPrefix(r.URL.Path, "/v1/automation/principals/") || !automationJSONRequest(r) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/automation/principals/"), "/")
	if len(parts) != 2 || !automationPrincipalHTTPID.MatchString(parts[0]) || parts[1] != "revoke" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	var request expectedRevisionHTTPRequest
	if !decodeStrictJSON(w, r, automationRequestMaxBytes, &request) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	auth, ok := automationControlMutationAuth(r, actor, request, api.automationNow())
	if !ok {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	result, err := api.store.RevokeAuthorizedAutomationPrincipal(auth, parts[0], request.ExpectedRevision)
	if api.automationControlError(w, "revoke automation principal", errorAutomationPrincipal, err) {
		return
	}
	if !result.Replayed {
		if err := api.reconcileAutomationCancellations(auth.Now); err != nil {
			api.internalError(w, "cancel revoked automation principal", err)
			return
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"principal": automationPrincipalHTTP(result.Principal), "replayed": result.Replayed,
	})
}

type automationTriggerHTTPRequest struct {
	CueID    string                        `json:"cue_id"`
	Audience automationAudienceHTTPRequest `json:"audience"`
	Delivery automationcontract.Delivery   `json:"delivery"`
}

func automationBearerToken(r *http.Request) (string, bool) {
	values := r.Header.Values("Authorization")
	if len(values) != 1 || !strings.HasPrefix(values[0], "Bearer ") ||
		strings.Count(values[0], " ") != 1 {
		return "", false
	}
	secret := strings.TrimPrefix(values[0], "Bearer ")
	return secret, lowerHexBearer.MatchString(secret)
}

func (api *onboardingAPI) automationTriggerError(w http.ResponseWriter, err error) {
	var limited *store.AutomationRateLimitError
	switch {
	case errors.Is(err, store.ErrAutomationInvalidCredential):
		apiError(w, http.StatusUnauthorized, errorAutomationCredential, 0)
	case errors.Is(err, store.ErrAutomationIdempotencyConflict):
		apiError(w, http.StatusConflict, errorAirIdempotency, 0)
	case errors.Is(err, store.ErrAutomationDisabled):
		http.Error(w, http.StatusText(http.StatusNotFound), http.StatusNotFound)
	case errors.As(err, &limited):
		apiError(w, http.StatusTooManyRequests, "automation_rate_limited", limited.RetryAfter)
	case errors.Is(err, store.ErrAutomationExecutionInProgress):
		apiError(w, http.StatusConflict, "automation_execution_in_progress", 0)
	case errors.Is(err, store.ErrInsufficientCapability):
		apiError(w, http.StatusForbidden, errorAutomationScope, 0)
	case errors.Is(err, store.ErrAutomationQuietHours):
		apiError(w, http.StatusConflict, "automation_quiet_hours", 0)
	case errors.Is(err, store.ErrAutomationCueNotReady):
		apiError(w, http.StatusUnprocessableEntity, "automation_cue_not_ready", 0)
	case errors.Is(err, store.ErrAutomationCapabilityMissing):
		apiError(w, http.StatusUnprocessableEntity, "automation_capability_missing", 0)
	case errors.Is(err, store.ErrTransmissionAudienceNotFound), errors.Is(err, store.ErrAutomationInvalid):
		apiError(w, http.StatusUnprocessableEntity, errorAutomationAudience, 0)
	default:
		api.internalError(w, "trigger automation", err)
	}
}

func (api *onboardingAPI) automationTriggerBoundary(w http.ResponseWriter, r *http.Request) {
	if api.automationTrigger == nil {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost || r.URL.Path != automationcontract.TriggerPath ||
		r.URL.RawQuery != "" || len(r.Header.Values("Cookie")) != 0 || !automationJSONRequest(r) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	secret, ok := automationBearerToken(r)
	if !ok {
		apiError(w, http.StatusUnauthorized, errorAutomationCredential, 0)
		return
	}
	key, ok := singleRequestHeader(r, "Idempotency-Key")
	if !ok || !automationIdempotencyKey.MatchString(key) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	var request automationTriggerHTTPRequest
	if !decodeStrictJSON(w, r, automationRequestMaxBytes, &request) ||
		!savedCueHTTPIDPattern.MatchString(request.CueID) ||
		request.Delivery != automationcontract.DeliveryOverlay {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	switch request.Audience.Kind {
	case automationcontract.AudienceOwnBarycenter:
		if len(request.Audience.TargetReferences) != 0 || request.Audience.AirID != "" {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
	case automationcontract.AudienceCurrentAir:
		if len(request.Audience.TargetReferences) != 0 || request.Audience.AirID != "" {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
	case automationcontract.AudienceExplicit:
		if len(request.Audience.TargetReferences) == 0 ||
			len(request.Audience.TargetReferences) > automationcontract.MaxExplicitSelectors ||
			request.Audience.AirID != "" {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		for _, reference := range request.Audience.TargetReferences {
			if !automationTargetRefHTTP.MatchString(reference) {
				apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
				return
			}
		}
	default:
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	result, err := api.automationTrigger.TriggerAutomation(automationTriggerInput{
		Secret: secret, IdempotencyKey: key, CueID: request.CueID,
		AudienceKind:     request.Audience.Kind,
		TargetReferences: append([]string(nil), request.Audience.TargetReferences...),
		Delivery:         request.Delivery, Now: api.automationNow().UTC().UnixMilli(),
	})
	if err != nil {
		api.automationTriggerError(w, err)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]any{
		"execution_id": result.Execution.ID, "status": result.Execution.Status,
		"replayed": result.Replayed,
	})
}
