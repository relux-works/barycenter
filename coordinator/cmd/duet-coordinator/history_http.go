package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"relux.works/duet/coordinator/internal/historyactions"
	"relux.works/duet/coordinator/internal/store"
)

const errorHistoryActionUnavailable = "history_action_unavailable"

type historyMediaJSON struct {
	MediaID          string `json:"media_id"`
	Kind             string `json:"kind"`
	Title            string `json:"title"`
	DurationMS       *int64 `json:"duration_ms,omitempty"`
	ContentAvailable *bool  `json:"content_available,omitempty"`
}

type historySenderJSON struct {
	ActorRef       string `json:"actor_ref"`
	DisplayName    string `json:"display_name"`
	SourceOrbitID  int64  `json:"source_orbit_id"`
	SourceOrbitRef string `json:"source_orbit_ref"`
}

type historyCountsJSON struct {
	Played int `json:"played"`
	Other  int `json:"other"`
}

type historyListItemJSON struct {
	HistoryItemID     string                        `json:"history_item_id"`
	ItemKind          string                        `json:"item_kind"`
	Direction         store.HistoryDirection        `json:"direction"`
	OccurredAt        string                        `json:"occurred_at"`
	Media             historyMediaJSON              `json:"media"`
	Sender            *historySenderJSON            `json:"sender,omitempty"`
	Audience          *transmissionAudienceResponse `json:"audience,omitempty"`
	RequestedDelivery string                        `json:"requested_delivery,omitempty"`
	EffectiveDelivery string                        `json:"effective_delivery,omitempty"`
	DowngradeReason   string                        `json:"downgrade_reason,omitempty"`
	Status            string                        `json:"status"`
	ReasonCode        string                        `json:"reason_code,omitempty"`
	TargetCounts      *historyCountsJSON            `json:"target_counts,omitempty"`
	Actions           []string                      `json:"actions"`
}

func historyMedia(item store.HistoryQueryItem, now int64) historyMediaJSON {
	media := historyMediaJSON{MediaID: item.Media.ID, Kind: string(item.Media.Kind), Title: item.Media.Title}
	if item.Media.Status != store.MediaStatusProcessing {
		duration := item.Media.DurationMS
		media.DurationMS = &duration
		available := item.Media.Status == store.MediaStatusReady && item.Media.DeletedAt == 0 && item.Media.ExpiresAt > now
		media.ContentAvailable = &available
	}
	return media
}

func historyTransmissionState(item store.HistoryQueryItem) (string, string) {
	c := item.TargetStatusCounts
	if c[store.TransmissionTargetPlaying]+c[store.TransmissionTargetCancelling] > 0 {
		return "playing", ""
	}
	terminal := c[store.TransmissionTargetPlayed] + c[store.TransmissionTargetMissedOffline] + c[store.TransmissionTargetMissedDND] +
		c[store.TransmissionTargetMissedNotReady] + c[store.TransmissionTargetBlocked] + c[store.TransmissionTargetFailed] +
		c[store.TransmissionTargetCancelled] + c[store.TransmissionTargetExpired]
	if terminal == item.TargetCount && item.TargetCount > 0 {
		if c[store.TransmissionTargetPlayed] == item.TargetCount {
			return "played", "completed"
		}
		if c[store.TransmissionTargetPlayed] > 0 {
			return "partial", "partial_delivery"
		}
		if c[store.TransmissionTargetExpired] == item.TargetCount {
			return "expired", "expired"
		}
		return "error", string(item.Transmission.ReasonCode)
	}
	if item.Media.Status == store.MediaStatusProcessing {
		return "processing", ""
	}
	return "ready", ""
}

func historyMediaState(media store.MediaItem) (string, string) {
	switch media.Status {
	case store.MediaStatusProcessing:
		return "processing", ""
	case store.MediaStatusReady:
		return "ready", ""
	default:
		return "error", media.FailureCode
	}
}

func historyActions(item store.HistoryQueryItem) []string {
	result := make([]string, 0, 7)
	if item.CanCancel {
		result = append(result, "cancel")
	}
	if item.CanDelete {
		result = append(result, "delete")
	}
	if item.CanReplay {
		result = append(result, "replay")
	}
	if item.CanReport {
		result = append(result, "report")
	}
	if item.CanBlockActor {
		result = append(result, "block_actor")
	}
	if item.CanBlockOrbit {
		result = append(result, "block_orbit")
	}
	if item.CanUnblock {
		result = append(result, "unblock")
	}
	return result
}

func historyTargetResponses(item store.HistoryQueryItem) []transmissionTargetResponse {
	result := targetResponses(item.Targets)
	if !item.RevealBlockedReason {
		return result
	}
	for index := range result {
		if item.Targets[index].Status == store.TransmissionTargetBlocked {
			result[index].ReasonCode = string(item.Targets[index].ReasonCode)
		}
	}
	return result
}

func (api *onboardingAPI) historySender(actor actorRequest, item store.HistoryQueryItem, now int64) (*historySenderJSON, error) {
	actorRef, err := api.store.MintTransmissionSubjectReference(actor.Context.ActorID, actor.Bearer,
		store.BlockedSubjectActor, item.SourceActorID, now)
	if err != nil {
		return nil, err
	}
	orbitRef, err := api.store.MintTransmissionSubjectReference(actor.Context.ActorID, actor.Bearer,
		store.BlockedSubjectOrbit, item.SourceOrbitID, now)
	if err != nil {
		return nil, err
	}
	return &historySenderJSON{ActorRef: actorRef.PublicID, DisplayName: item.SourceActorName,
		SourceOrbitID: item.SourceOrbitID, SourceOrbitRef: orbitRef.PublicID}, nil
}

func (api *onboardingAPI) historyListResponse(actor actorRequest, item store.HistoryQueryItem, now int64) (historyListItemJSON, error) {
	result := historyListItemJSON{HistoryItemID: item.HistoryItemID, ItemKind: item.ItemKind,
		Direction: item.Direction, OccurredAt: coordTime(item.OccurredAt), Media: historyMedia(item, now),
		Actions: historyActions(item)}
	if item.Transmission == nil {
		result.Status, result.ReasonCode = historyMediaState(item.Media)
		return result, nil
	}
	sender, err := api.historySender(actor, item, now)
	if err != nil {
		return historyListItemJSON{}, err
	}
	result.Sender = sender
	result.Audience = &transmissionAudienceResponse{Kind: string(item.Transmission.AudienceKind), TargetCount: item.TargetCount}
	result.RequestedDelivery = string(item.Transmission.RequestedDelivery)
	result.EffectiveDelivery = string(item.Transmission.EffectiveDelivery)
	result.DowngradeReason = item.Transmission.DowngradeReason
	result.Status, result.ReasonCode = historyTransmissionState(item)
	played := item.TargetStatusCounts[store.TransmissionTargetPlayed]
	result.TargetCounts = &historyCountsJSON{Played: played, Other: item.TargetCount - played}
	return result, nil
}

func parseHistoryQuery(r *http.Request) (string, int, string, bool) {
	query := r.URL.Query()
	for key, values := range query {
		if (key != "view" && key != "limit" && key != "cursor") || len(values) != 1 || values[0] == "" {
			return "", 0, "", false
		}
	}
	view := "all"
	if values, exists := query["view"]; exists {
		view = values[0]
	}
	if view != "all" && view != "sent" && view != "received" {
		return "", 0, "", false
	}
	limit := 30
	if values, exists := query["limit"]; exists {
		raw := values[0]
		value, err := strconv.Atoi(raw)
		if err != nil || value < 1 || value > 100 || strconv.Itoa(value) != raw {
			return "", 0, "", false
		}
		limit = value
	}
	return view, limit, query.Get("cursor"), true
}

func (api *onboardingAPI) history(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		apiError(w, http.StatusMethodNotAllowed, errorInvalidRequest, 0)
		return
	}
	view, limit, cursor, ok := parseHistoryQuery(r)
	if !ok {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	now := api.transmissionNow().UnixMilli()
	page, err := api.store.QueryAuthorizedHistory(actor.Context.ActorID,
		store.Identity{Kind: store.IdentityBearer, Token: actor.Bearer}, view, limit, cursor, now)
	if err != nil {
		api.historyError(w, "query history", err)
		return
	}
	items := make([]historyListItemJSON, 0, len(page.Items))
	for _, item := range page.Items {
		value, err := api.historyListResponse(actor, item, now)
		if err != nil {
			api.internalError(w, "project history sender", err)
			return
		}
		items = append(items, value)
	}
	response := map[string]any{"contract": presencePolicyContract, "items": items}
	if page.NextCursor != "" {
		response["next_cursor"] = page.NextCursor
	}
	writeJSON(w, http.StatusOK, response)
}

type historyDetailJSON struct {
	Contract          string                            `json:"contract"`
	HistoryItemID     string                            `json:"history_item_id"`
	ItemKind          string                            `json:"item_kind"`
	Direction         store.HistoryDirection            `json:"direction"`
	TransmissionID    string                            `json:"transmission_id,omitempty"`
	OccurredAt        string                            `json:"occurred_at"`
	AcceptedAt        string                            `json:"accepted_at,omitempty"`
	ExpiresAt         string                            `json:"expires_at,omitempty"`
	Media             historyMediaJSON                  `json:"media"`
	Sender            *historySenderJSON                `json:"sender,omitempty"`
	Audience          *transmissionAudienceResponse     `json:"audience,omitempty"`
	RequestedDelivery string                            `json:"requested_delivery,omitempty"`
	EffectiveDelivery string                            `json:"effective_delivery,omitempty"`
	DowngradeReason   string                            `json:"downgrade_reason,omitempty"`
	Status            string                            `json:"status"`
	ReasonCode        string                            `json:"reason_code,omitempty"`
	TargetCounts      *transmissionTargetCountsResponse `json:"target_counts,omitempty"`
	Targets           *[]transmissionTargetResponse     `json:"targets,omitempty"`
	Actions           []string                          `json:"actions"`
}

func (api *onboardingAPI) historyItem(w http.ResponseWriter, r *http.Request) {
	id, action, validPath := parseHistoryItemPath(r.URL.Path)
	if !validPath {
		apiError(w, http.StatusNotFound, errorHistoryNotFound, 0)
		return
	}
	if action != "" {
		api.historyAction(w, r, id, action)
		return
	}
	if r.Method != http.MethodGet {
		w.Header().Set("Allow", http.MethodGet)
		apiError(w, http.StatusMethodNotAllowed, errorInvalidRequest, 0)
		return
	}
	if r.URL.RawQuery != "" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	now := api.transmissionNow().UnixMilli()
	item, err := api.store.GetAuthorizedHistoryItem(actor.Context.ActorID,
		store.Identity{Kind: store.IdentityBearer, Token: actor.Bearer}, id, now)
	if err != nil {
		api.historyError(w, "get history", err)
		return
	}
	base, err := api.historyListResponse(actor, item, now)
	if err != nil {
		api.internalError(w, "project history detail", err)
		return
	}
	result := historyDetailJSON{Contract: presencePolicyContract, HistoryItemID: id, ItemKind: item.ItemKind,
		Direction: item.Direction, OccurredAt: base.OccurredAt, Media: base.Media, Sender: base.Sender,
		Audience: base.Audience, RequestedDelivery: base.RequestedDelivery, EffectiveDelivery: base.EffectiveDelivery,
		DowngradeReason: base.DowngradeReason, Status: base.Status, ReasonCode: base.ReasonCode, Actions: base.Actions}
	if item.Transmission != nil {
		result.TransmissionID = item.Transmission.ID
		result.AcceptedAt = coordTime(item.Transmission.AcceptedAt)
		result.ExpiresAt = coordTime(item.Transmission.ExpiresAt)
		counts := targetCountsResponse(item.TargetStatusCounts)
		result.TargetCounts = &counts
		targets := historyTargetResponses(item)
		result.Targets = &targets
	}
	writeJSON(w, http.StatusOK, result)
}

func parseHistoryItemPath(path string) (historyItemID, action string, ok bool) {
	parts := strings.Split(strings.TrimPrefix(path, "/v1/history/"), "/")
	if len(parts) != 1 && (len(parts) != 3 || parts[1] != "actions") {
		return "", "", false
	}
	if len(parts[0]) != 29 || !strings.HasPrefix(parts[0], "hi_") {
		return "", "", false
	}
	if len(parts) == 3 {
		return parts[0], parts[2], parts[2] != ""
	}
	return parts[0], "", true
}

type historyReplayRequest struct {
	Audience             transmissionAudienceRequest  `json:"audience"`
	Delivery             string                       `json:"delivery"`
	IncludeOrigin        *bool                        `json:"include_origin,omitempty"`
	FallbackConfirmation *transmissionFallbackRequest `json:"fallback_confirmation,omitempty"`
}

type canonicalHistoryReplayRequest struct {
	HistoryItemID string                      `json:"history_item_id"`
	Action        string                      `json:"action"`
	Audience      transmissionAudienceRequest `json:"audience"`
	Delivery      string                      `json:"delivery"`
	IncludeOrigin bool                        `json:"include_origin"`
}

func validateHistoryReplayRequest(request historyReplayRequest) (bool, []store.TransmissionAudienceSelector) {
	include := request.IncludeOrigin
	probe := createTransmissionRequest{
		MediaID: "m_00000000000000000000000000", Audience: request.Audience,
		Delivery: request.Delivery, OriginKind: string(store.TransmissionOriginFile),
		IncludeOrigin: include, FallbackConfirmation: request.FallbackConfirmation,
	}
	return validateTransmissionCreateRequest(probe)
}

func canonicalHistoryActionHash(value any) (string, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return "", err
	}
	return transmissionDigest(string(raw)), nil
}

func (api *onboardingAPI) historyActionService() (*historyactions.Service, error) {
	return historyactions.NewService(api.store, api.mediaLifecycle, api.moderationService)
}

func (api *onboardingAPI) historyAction(w http.ResponseWriter, r *http.Request, historyItemID, action string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		apiError(w, http.StatusMethodNotAllowed, errorInvalidRequest, 0)
		return
	}
	if r.URL.RawQuery != "" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	service, err := api.historyActionService()
	if err != nil {
		api.internalError(w, "initialize history actions", err)
		return
	}
	actorRequest := r.Context().Value(actorRequestKey{}).(actorRequest)
	actor := historyactions.Actor{ExpectedActorID: actorRequest.Context.ActorID,
		Identity: store.Identity{Kind: store.IdentityBearer, Token: actorRequest.Bearer}}
	now := api.transmissionNow().UTC().UnixMilli()
	switch action {
	case "replay":
		api.replayHistoryAction(w, r, service, actorRequest, actor, historyItemID, now)
	case "delete":
		var request struct{}
		if !decodeStrictTransmissionJSON(w, r, &request) {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		item, err := service.Delete(actor, historyItemID, now)
		if api.historyActionError(w, "delete history media", err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"history_item_id": historyItemID, "media_id": item.ID, "deleted": true,
		})
	case "report":
		var request struct {
			Reason  store.ModerationReason `json:"reason"`
			Details string                 `json:"details"`
		}
		if !decodeStrictJSON(w, r, 4096, &request) {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		created, err := service.Report(actor, historyItemID, store.CreateModerationReportParams{
			Reason: request.Reason, Details: request.Details,
		}, now)
		if api.historyActionError(w, "report history media", err) {
			return
		}
		status := http.StatusCreated
		if created.Reused {
			status = http.StatusOK
		}
		response := reporterModerationView(created.Report)
		response["history_item_id"] = historyItemID
		response["reused"] = created.Reused
		writeJSON(w, status, response)
	case "block_actor", "block_orbit":
		api.blockHistoryAction(w, r, service, actorRequest, actor, historyItemID, action, now)
	default:
		apiError(w, http.StatusNotFound, errorHistoryNotFound, 0)
	}
}

func (api *onboardingAPI) replayHistoryAction(w http.ResponseWriter, r *http.Request, service *historyactions.Service, actorRequest actorRequest, actor historyactions.Actor, historyItemID string, now int64) {
	idempotencyKey, ok := singleRequestHeader(r, "Idempotency-Key")
	if !ok || !transmissionIdempotencyKey.MatchString(idempotencyKey) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	var request historyReplayRequest
	if !decodeStrictTransmissionJSON(w, r, &request) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	valid, selectors := validateHistoryReplayRequest(request)
	if !valid {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	if request.Delivery == "queue" || request.Delivery == "replace" {
		writeTransmissionAPIError(w, http.StatusUnprocessableEntity, errorDeliveryNotSupported, nil)
		return
	}
	if !api.reserveTransmission(w, actorRequest.Context) {
		return
	}
	includeOrigin := true
	if request.IncludeOrigin != nil {
		includeOrigin = *request.IncludeOrigin
	}
	requestHash, err := canonicalHistoryActionHash(canonicalHistoryReplayRequest{
		HistoryItemID: historyItemID, Action: "replay", Audience: request.Audience,
		Delivery: request.Delivery, IncludeOrigin: includeOrigin,
	})
	if err != nil {
		api.internalError(w, "hash history replay", err)
		return
	}
	challengeToken := ""
	if request.Delivery == "interrupt" {
		challengeToken, err = api.transmissionToken()
		if err != nil || !transmissionConfirmationToken.MatchString(challengeToken) {
			api.internalError(w, "mint history replay confirmation", err)
			return
		}
	}
	params := historyactions.ReplayParams{
		Actor: actor, HistoryItemID: historyItemID,
		IdempotencyKeyHash: transmissionDigest(idempotencyKey), RequestHash: requestHash,
		AudienceKind: store.TransmissionAudienceKind(request.Audience.Kind), Selectors: selectors,
		OriginKind: store.TransmissionOriginFile, IncludeOrigin: includeOrigin,
		RequestedDelivery: store.TransmissionDelivery(request.Delivery), AcceptedAt: now,
		Availability: api.transmissionAvailability(),
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
	result, err := service.Replay(params)
	if api.historyReplayError(w, err) {
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

func (api *onboardingAPI) blockHistoryAction(w http.ResponseWriter, r *http.Request, service *historyactions.Service, actorRequest actorRequest, actor historyactions.Actor, historyItemID, action string, now int64) {
	idempotencyKey, ok := singleRequestHeader(r, "Idempotency-Key")
	if !ok || !transmissionIdempotencyKey.MatchString(idempotencyKey) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	var request struct{}
	if !decodeStrictTransmissionJSON(w, r, &request) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	requestHash, err := canonicalHistoryActionHash(map[string]string{
		"history_item_id": historyItemID, "action": action,
	})
	if err != nil {
		api.internalError(w, "hash history block", err)
		return
	}
	block, err := service.Block(historyactions.BlockParams{
		Actor: actor, HistoryItemID: historyItemID, Kind: historyactions.BlockKind(action),
		IdempotencyKeyHash: transmissionDigest(idempotencyKey), RequestHash: requestHash,
		CreatedAt: now,
	})
	if api.historyActionError(w, "block history sender", err) {
		return
	}
	if !block.Reused {
		api.enforceBlock(actorRequest, block, now)
	}
	status := http.StatusCreated
	if block.Reused {
		status = http.StatusOK
	}
	reused := block.Reused
	response := blockResponse(block)
	response.Reused = &reused
	writeJSON(w, status, response)
}

func (api *onboardingAPI) historyReplayError(w http.ResponseWriter, err error) bool {
	if errors.Is(err, historyactions.ErrActionUnavailable) ||
		errors.Is(err, store.ErrTransmissionMediaNotFound) ||
		errors.Is(err, store.ErrTransmissionMediaNotReady) ||
		errors.Is(err, store.ErrTransmissionMediaInvalid) {
		apiError(w, http.StatusConflict, errorHistoryActionUnavailable, 0)
		return true
	}
	return api.transmissionStoreError(w, "replay history", err)
}

func (api *onboardingAPI) historyActionError(w http.ResponseWriter, operation string, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, historyactions.ErrServiceUnavailable):
		apiError(w, http.StatusServiceUnavailable, errorServiceUnavailable, 0)
	case errors.Is(err, historyactions.ErrActionUnavailable),
		errors.Is(err, store.ErrMediaNotFound), errors.Is(err, store.ErrMediaStateConflict),
		errors.Is(err, store.ErrModerationNotFound):
		apiError(w, http.StatusConflict, errorHistoryActionUnavailable, 0)
	case errors.Is(err, store.ErrTransmissionNotFound):
		apiError(w, http.StatusNotFound, errorHistoryNotFound, 0)
	case errors.Is(err, store.ErrModerationInvalid):
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
	case errors.Is(err, store.ErrModerationRateLimited):
		apiError(w, http.StatusTooManyRequests, errorTooManyAttempts, 0)
	case errors.Is(err, store.ErrUnauthorized):
		apiError(w, http.StatusUnauthorized, errorUnauthorized, 0)
	case errors.Is(err, store.ErrInsufficientCapability),
		errors.Is(err, store.ErrTransmissionPolicyForbidden),
		errors.Is(err, store.ErrOrbitDisabled),
		errors.Is(err, store.ErrSelfServiceOnboardingDisabled):
		apiError(w, http.StatusForbidden, errorInsufficientCapability, 0)
	case errors.Is(err, store.ErrTransmissionPolicyIdempotency):
		apiError(w, http.StatusConflict, errorTransmissionIdempotency, 0)
	default:
		api.internalError(w, operation, err)
	}
	return true
}

func (api *onboardingAPI) historyError(w http.ResponseWriter, operation string, err error) {
	switch {
	case errors.Is(err, store.ErrHistoryCursorInvalid):
		apiError(w, http.StatusBadRequest, errorHistoryCursorInvalid, 0)
	case errors.Is(err, store.ErrTransmissionNotFound):
		apiError(w, http.StatusNotFound, errorHistoryNotFound, 0)
	case errors.Is(err, store.ErrTransmissionInvalid):
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
	case errors.Is(err, store.ErrUnauthorized):
		apiError(w, http.StatusUnauthorized, errorUnauthorized, 0)
	default:
		api.internalError(w, operation, err)
	}
}
