package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"relux.works/duet/coordinator/internal/presentation"
	"relux.works/duet/coordinator/internal/store"
)

const targetsInboxContract = "p2-targets-inbox-parity.v1"

type inboxMediaJSON struct {
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	DurationMS int64  `json:"duration_ms"`
}

type inboxSenderJSON struct {
	DisplayName string `json:"display_name"`
	OrbitLabel  string `json:"orbit_label"`
}

type inboxReceiptJSON struct {
	Status     string `json:"status"`
	ReasonCode string `json:"reason_code"`
	OccurredAt string `json:"occurred_at"`
}

type inboxItemJSON struct {
	ID                string                             `json:"id"`
	HistoryItemID     string                             `json:"history_item_id"`
	Revision          int64                              `json:"revision"`
	Media             inboxMediaJSON                     `json:"media"`
	Sender            inboxSenderJSON                    `json:"sender"`
	RequestedDelivery string                             `json:"requested_delivery"`
	EffectiveDelivery string                             `json:"effective_delivery"`
	Availability      string                             `json:"availability"`
	Receipt           inboxReceiptJSON                   `json:"receipt"`
	ReplayDepth       int                                `json:"replay_depth"`
	CreatedAt         string                             `json:"created_at"`
	ExpiresAt         string                             `json:"expires_at"`
	Actions           []string                           `json:"actions"`
	Presentation      presentation.InboxItemPresentation `json:"presentation"`
}

func inboxItemResponse(item store.AuthorizedTransmissionInboxItem) inboxItemJSON {
	actions := make([]string, 0, 6)
	if item.CanReplay {
		actions = append(actions, "replay")
	}
	if item.CanDismiss {
		actions = append(actions, "dismiss")
	}
	if item.CanReport {
		actions = append(actions, "report")
	}
	if item.CanBlockActor {
		actions = append(actions, "block_actor")
	}
	if item.CanBlockOrbit {
		actions = append(actions, "block_orbit")
	}
	if item.CanUnblock {
		actions = append(actions, "unblock")
	}
	result := inboxItemJSON{
		ID: item.Item.ID, HistoryItemID: item.HistoryItemID,
		Revision: item.Item.Revision,
		Media: inboxMediaJSON{Kind: string(item.Item.MediaKind), Title: item.MediaTitle,
			DurationMS: item.DurationMS},
		Sender:            inboxSenderJSON{DisplayName: item.SourceName, OrbitLabel: item.SourceOrbitName},
		RequestedDelivery: string(item.Item.RequestedDelivery),
		EffectiveDelivery: string(item.Item.EffectiveDelivery),
		Availability:      string(item.Item.Availability), ReplayDepth: item.Item.ReplayDepth,
		Receipt: inboxReceiptJSON{Status: string(item.Item.MissedStatus),
			ReasonCode: string(item.Item.MissedReason), OccurredAt: coordTime(item.Item.CreatedAt)},
		CreatedAt: coordTime(item.Item.CreatedAt), ExpiresAt: coordTime(item.Item.ExpiresAt),
		Actions: actions,
	}
	result.Presentation = presentation.PresentInboxItem(
		item.SourceName, item.SourceOrbitName, item.Item.RequestedDelivery,
		item.Item.EffectiveDelivery, string(item.Item.Availability),
		string(item.Item.MissedStatus), item.Item.MissedReason, actions,
	)
	return result
}

func parseBoundedPageQuery(r *http.Request, defaultLimit int, allowed map[string]bool) (int, string, string, bool) {
	query := r.URL.Query()
	for key, values := range query {
		if !allowed[key] || len(values) != 1 || values[0] == "" {
			return 0, "", "", false
		}
	}
	limit := defaultLimit
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 || parsed > 100 || strconv.Itoa(parsed) != raw {
			return 0, "", "", false
		}
		limit = parsed
	}
	return limit, query.Get("cursor"), query.Get("view"), true
}

func (api *onboardingAPI) inbox(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.URL.Path != "/v1/inbox" ||
		r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		w.Header().Set("Allow", http.MethodGet)
		apiError(w, http.StatusMethodNotAllowed, errorInvalidRequest, 0)
		return
	}
	limit, cursor, view, ok := parseBoundedPageQuery(r, 20, map[string]bool{
		"limit": true, "cursor": true, "view": true,
	})
	if !ok {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	if view == "" {
		view = "all"
	}
	if view != "all" && view != "available" && view != "dismissed" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	page, err := api.store.QueryAuthorizedTransmissionInbox(
		actor.Context.ActorID, store.Identity{Kind: store.IdentityBearer, Token: actor.Bearer},
		view, limit, cursor, api.transmissionNow().UTC().UnixMilli(),
	)
	if api.inboxError(w, "list inbox", err) {
		return
	}
	items := make([]inboxItemJSON, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, inboxItemResponse(item))
	}
	response := map[string]any{"contract": targetsInboxContract, "items": items}
	if page.NextCursor != "" {
		response["next_cursor"] = page.NextCursor
	}
	writeJSON(w, http.StatusOK, response)
}

func parseInboxItemPath(path string) (inboxID, action string, ok bool) {
	parts := strings.Split(strings.TrimPrefix(path, "/v1/inbox/"), "/")
	if len(parts) != 1 && (len(parts) != 2 || parts[1] != "replays") {
		return "", "", false
	}
	if len(parts[0]) != 29 || !strings.HasPrefix(parts[0], "ib_") {
		return "", "", false
	}
	if len(parts) == 2 {
		return parts[0], "replays", true
	}
	return parts[0], "", true
}

func (api *onboardingAPI) inboxItem(w http.ResponseWriter, r *http.Request) {
	inboxID, action, ok := parseInboxItemPath(r.URL.Path)
	if !ok || r.URL.RawQuery != "" {
		apiError(w, http.StatusNotFound, errorInboxNotFound, 0)
		return
	}
	if action == "replays" {
		api.replayInboxItem(w, r, inboxID)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	identity := store.Identity{Kind: store.IdentityBearer, Token: actor.Bearer}
	now := api.transmissionNow().UTC().UnixMilli()
	switch r.Method {
	case http.MethodGet:
		if r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		item, err := api.store.GetAuthorizedTransmissionInboxItem(
			actor.Context.ActorID, identity, inboxID, now,
		)
		if api.inboxError(w, "get inbox item", err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"contract": targetsInboxContract, "item": inboxItemResponse(item),
		})
	case http.MethodDelete:
		if r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		item, err := api.store.DismissAuthorizedTransmissionInboxItem(
			actor.Context.ActorID, identity, inboxID, now,
		)
		if api.inboxError(w, "dismiss inbox item", err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"contract": targetsInboxContract, "item": inboxItemResponse(item),
		})
	default:
		w.Header().Set("Allow", http.MethodGet+", "+http.MethodDelete)
		apiError(w, http.StatusMethodNotAllowed, errorInvalidRequest, 0)
	}
}

type inboxReplayRequest struct {
	Delivery             string                       `json:"delivery"`
	FallbackConfirmation *transmissionFallbackRequest `json:"fallback_confirmation,omitempty"`
}

func (api *onboardingAPI) replayInboxItem(w http.ResponseWriter, r *http.Request, inboxID string) {
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		apiError(w, http.StatusMethodNotAllowed, errorInvalidRequest, 0)
		return
	}
	idempotencyKey, ok := singleRequestHeader(r, "Idempotency-Key")
	if !ok || !transmissionIdempotencyKey.MatchString(idempotencyKey) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	var request inboxReplayRequest
	if !decodeStrictTransmissionJSON(w, r, &request) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	delivery := store.TransmissionDelivery(request.Delivery)
	if delivery != store.TransmissionDeliveryOverlay &&
		delivery != store.TransmissionDeliveryInterrupt &&
		delivery != store.TransmissionDeliveryAfterCurrent {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	if !api.reserveTransmission(w, actor.Context) {
		return
	}
	now := api.transmissionNow().UTC().UnixMilli()
	canonical, err := json.Marshal(map[string]string{
		"action": "inbox_replay", "inbox_id": inboxID, "delivery": request.Delivery,
	})
	if err != nil {
		api.internalError(w, "hash inbox replay", err)
		return
	}
	params := store.CreateAuthorizedInboxReplayParams{
		ExpectedActorID: actor.Context.ActorID,
		Identity:        store.Identity{Kind: store.IdentityBearer, Token: actor.Bearer},
		InboxID:         inboxID, IdempotencyKeyHash: transmissionDigest(idempotencyKey),
		RequestHash: transmissionDigest(string(canonical)), RequestedDelivery: delivery,
		AcceptedAt: now, Availability: api.transmissionAvailability(),
	}
	challengeToken := ""
	if delivery == store.TransmissionDeliveryInterrupt {
		challengeToken, err = api.transmissionToken()
		if err != nil || !transmissionConfirmationToken.MatchString(challengeToken) {
			api.internalError(w, "mint inbox replay confirmation", err)
			return
		}
		params.ChallengeTokenHash = transmissionDigest(challengeToken)
	}
	if request.FallbackConfirmation != nil {
		params.Confirmation = &store.ConfirmTransmissionFallback{
			TokenHash: transmissionDigest(request.FallbackConfirmation.Token),
			Delivery:  store.TransmissionDelivery(request.FallbackConfirmation.Delivery),
		}
	}
	result, err := api.store.CreateAuthorizedInboxReplay(params)
	if api.inboxReplayError(w, err) {
		return
	}
	if result.Challenge != nil {
		writeTransmissionChallenge(w, *result.Challenge, challengeToken)
		return
	}
	if !result.Reused {
		api.transmissionAccepted(result.Creation.Transmission.ID)
	}
	historyItemID := "hi_" + strings.TrimPrefix(result.Creation.Transmission.ID, "tr_")
	replayRequestID := "ir_" + strings.TrimPrefix(result.Creation.Transmission.ID, "tr_")
	status := http.StatusCreated
	if result.Reused {
		status = http.StatusOK
	}
	w.Header().Set("Location", "/v1/history/"+historyItemID)
	writeJSON(w, status, map[string]any{
		"contract": targetsInboxContract, "replay_request_id": replayRequestID,
		"history_item_id":    historyItemID,
		"requested_delivery": string(result.Creation.Transmission.RequestedDelivery),
		"effective_delivery": string(result.Creation.Transmission.EffectiveDelivery),
		"status":             string(result.Creation.Transmission.Status), "reused": result.Reused,
	})
}

func (api *onboardingAPI) inboxReplayError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, store.ErrTransmissionInboxNotFound),
		errors.Is(err, store.ErrTransmissionMediaNotFound):
		apiError(w, http.StatusNotFound, errorInboxNotFound, 0)
		return true
	case errors.Is(err, store.ErrTransmissionReplayDepthExceeded):
		apiError(w, http.StatusConflict, errorReplayDepthExceeded, 0)
		return true
	default:
		return api.transmissionStoreError(w, "replay inbox item", err)
	}
}

func (api *onboardingAPI) inboxError(w http.ResponseWriter, operation string, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, store.ErrInboxCursorExpired), errors.Is(err, store.ErrReceiptCursorExpired):
		apiError(w, http.StatusGone, errorCursorExpired, 0)
	case errors.Is(err, store.ErrTransmissionInboxNotFound),
		errors.Is(err, store.ErrTransmissionNotFound):
		apiError(w, http.StatusNotFound, errorInboxNotFound, 0)
	case errors.Is(err, store.ErrTransmissionInboxConflict):
		apiError(w, http.StatusConflict, errorHistoryActionUnavailable, 0)
	case errors.Is(err, store.ErrUnauthorized):
		apiError(w, http.StatusUnauthorized, errorUnauthorized, 0)
	case errors.Is(err, store.ErrInsufficientCapability), errors.Is(err, store.ErrOrbitDisabled):
		apiError(w, http.StatusForbidden, errorInsufficientCapability, 0)
	case errors.Is(err, store.ErrTransmissionInvalid):
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
	default:
		api.internalError(w, operation, err)
	}
	return true
}

type historyReceiptJSON struct {
	TargetLabel  string                                  `json:"target_label"`
	Status       string                                  `json:"status"`
	ReasonCode   string                                  `json:"reason_code,omitempty"`
	ReadyAt      string                                  `json:"ready_at,omitempty"`
	ScheduledAt  string                                  `json:"scheduled_at,omitempty"`
	StartedAt    string                                  `json:"started_at,omitempty"`
	EndedAt      string                                  `json:"ended_at,omitempty"`
	Presentation presentation.HistoryReceiptPresentation `json:"presentation"`
}

func historyReceiptResponse(item store.AuthorizedHistoryReceipt) historyReceiptJSON {
	value := historyReceiptJSON{TargetLabel: item.DisplayLabel, Status: string(item.Target.Status)}
	reason := store.TransmissionReason("")
	if item.RevealReason {
		value.ReasonCode = string(item.Target.ReasonCode)
		reason = item.Target.ReasonCode
	}
	value.Presentation = presentation.PresentHistoryReceipt(string(item.Target.Status), reason)
	if item.Target.ReadyAt > 0 {
		value.ReadyAt = coordTime(item.Target.ReadyAt)
	}
	if item.Target.ScheduledAt > 0 {
		value.ScheduledAt = coordTime(item.Target.ScheduledAt)
	}
	if item.Target.StartedAt > 0 {
		value.StartedAt = coordTime(item.Target.StartedAt)
	}
	if item.Target.EndedAt > 0 {
		value.EndedAt = coordTime(item.Target.EndedAt)
	}
	return value
}

func (api *onboardingAPI) historyReceipts(w http.ResponseWriter, r *http.Request, historyItemID string) {
	if r.Method != http.MethodGet || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		w.Header().Set("Allow", http.MethodGet)
		apiError(w, http.StatusMethodNotAllowed, errorInvalidRequest, 0)
		return
	}
	limit, cursor, _, ok := parseBoundedPageQuery(r, 20, map[string]bool{
		"limit": true, "cursor": true,
	})
	if !ok {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	page, err := api.store.QueryAuthorizedHistoryReceipts(
		actor.Context.ActorID, store.Identity{Kind: store.IdentityBearer, Token: actor.Bearer},
		historyItemID, limit, cursor, api.transmissionNow().UTC().UnixMilli(),
	)
	if err != nil {
		if errors.Is(err, store.ErrReceiptCursorExpired) {
			apiError(w, http.StatusGone, errorCursorExpired, 0)
		} else if errors.Is(err, store.ErrTransmissionNotFound) {
			apiError(w, http.StatusNotFound, errorHistoryNotFound, 0)
		} else {
			api.historyError(w, "list history receipts", err)
		}
		return
	}
	items := make([]historyReceiptJSON, 0, len(page.Items))
	for _, item := range page.Items {
		items = append(items, historyReceiptResponse(item))
	}
	response := map[string]any{
		"contract": targetsInboxContract, "history_item_id": historyItemID, "items": items,
	}
	if page.NextCursor != "" {
		response["next_cursor"] = page.NextCursor
	}
	writeJSON(w, http.StatusOK, response)
}
