package main

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"

	"relux.works/duet/coordinator/internal/historyactions"
	"relux.works/duet/coordinator/internal/presentation"
	"relux.works/duet/coordinator/internal/store"
)

const errorHistoryActionUnavailable = "history_action_unavailable"

type historyMediaJSON struct {
	Kind             string `json:"kind"`
	Title            string `json:"title"`
	DurationMS       *int64 `json:"duration_ms,omitempty"`
	ContentAvailable *bool  `json:"content_available,omitempty"`
}

type historySenderJSON struct {
	ActorRef       string `json:"actor_ref"`
	DisplayName    string `json:"display_name"`
	SourceOrbitRef string `json:"source_orbit_ref"`
}

type historyCountsJSON struct {
	Played int `json:"played"`
	Other  int `json:"other"`
}

type historyAutomationJSON struct {
	TriggerKind         string `json:"trigger_kind"`
	PrincipalRef        string `json:"principal_ref,omitempty"`
	PrincipalLabel      string `json:"principal_label,omitempty"`
	ScheduleID          string `json:"schedule_id,omitempty"`
	ScheduleLabel       string `json:"schedule_label,omitempty"`
	ScheduleRevision    int64  `json:"schedule_revision,omitempty"`
	ExecutionID         string `json:"execution_id,omitempty"`
	CueID               string `json:"cue_id"`
	CueLabel            string `json:"cue_label,omitempty"`
	CueRevision         int64  `json:"cue_revision,omitempty"`
	AudienceKind        string `json:"audience_kind,omitempty"`
	ResolvedTargetCount int    `json:"resolved_target_count"`
	Outcome             string `json:"outcome"`
	ReasonCode          string `json:"reason_code,omitempty"`
	RetryAfterMS        int64  `json:"retry_after_ms,omitempty"`
	ScheduledAt         string `json:"scheduled_at,omitempty"`
	AcceptedAt          string `json:"accepted_at,omitempty"`
	TerminalAt          string `json:"terminal_at,omitempty"`
}

type historyListItemJSON struct {
	HistoryItemID     string                               `json:"history_item_id"`
	ItemKind          string                               `json:"item_kind"`
	Direction         store.HistoryDirection               `json:"direction"`
	OccurredAt        string                               `json:"occurred_at"`
	Media             historyMediaJSON                     `json:"media"`
	Sender            *historySenderJSON                   `json:"sender,omitempty"`
	Audience          *transmissionAudienceResponse        `json:"audience,omitempty"`
	RequestedDelivery string                               `json:"requested_delivery,omitempty"`
	EffectiveDelivery string                               `json:"effective_delivery,omitempty"`
	DowngradeReason   string                               `json:"downgrade_reason,omitempty"`
	Status            string                               `json:"status"`
	ReasonCode        string                               `json:"reason_code,omitempty"`
	TargetCounts      *historyCountsJSON                   `json:"target_counts,omitempty"`
	Automation        *historyAutomationJSON               `json:"automation,omitempty"`
	Actions           []string                             `json:"actions"`
	Presentation      presentation.HistoryItemPresentation `json:"presentation"`
}

func historyMedia(item store.HistoryQueryItem, now int64) historyMediaJSON {
	media := historyMediaJSON{Kind: string(item.Media.Kind), Title: item.Media.Title}
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
	result := make([]string, 0, 10)
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
	if item.CanDisableSchedule {
		result = append(result, "disable_schedule")
	}
	if item.CanRevokePrincipal {
		result = append(result, "revoke_principal")
	}
	if item.CanEmergencyDisable {
		result = append(result, "emergency_disable_automation")
	}
	return result
}

func historyAutomation(value *store.AutomationHistory) *historyAutomationJSON {
	if value == nil {
		return nil
	}
	result := &historyAutomationJSON{
		TriggerKind: value.TriggerKind, PrincipalRef: value.PrincipalRef,
		PrincipalLabel: value.PrincipalLabel, ScheduleID: value.ScheduleID,
		ScheduleLabel: value.ScheduleLabel, ScheduleRevision: value.ScheduleRevision,
		ExecutionID: value.ExecutionID, CueID: value.CueID, CueLabel: value.CueLabel,
		CueRevision: value.CueRevision, AudienceKind: value.AudienceKind,
		ResolvedTargetCount: value.ResolvedTargetCount, Outcome: value.Outcome,
		ReasonCode: value.ReasonCode, RetryAfterMS: value.RetryAfterMS,
	}
	if value.ScheduledAt > 0 {
		result.ScheduledAt = coordTime(value.ScheduledAt)
	}
	if value.AcceptedAt > 0 {
		result.AcceptedAt = coordTime(value.AcceptedAt)
	}
	if value.TerminalAt > 0 {
		result.TerminalAt = coordTime(value.TerminalAt)
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
		SourceOrbitRef: orbitRef.PublicID}, nil
}

func (api *onboardingAPI) historyListResponse(actor actorRequest, item store.HistoryQueryItem, now int64) (historyListItemJSON, error) {
	result := historyListItemJSON{HistoryItemID: item.HistoryItemID, ItemKind: item.ItemKind,
		Direction: item.Direction, OccurredAt: coordTime(item.OccurredAt), Media: historyMedia(item, now),
		Actions: historyActions(item), Automation: historyAutomation(item.Automation)}
	if item.Transmission == nil {
		if item.Automation != nil {
			result.Status, result.ReasonCode = item.Automation.Outcome, item.Automation.ReasonCode
			result.Presentation = presentation.PresentHistoryItem(
				item.Direction, "", "", "overlay", "overlay", result.Status,
				store.TransmissionReason(result.ReasonCode), result.Actions,
			)
			return result, nil
		}
		result.Status, result.ReasonCode = historyMediaState(item.Media)
		result.Presentation = presentation.PresentHistoryItem(
			item.Direction, "", "", "", "", result.Status,
			store.TransmissionReason(result.ReasonCode), result.Actions,
		)
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
	result.Presentation = presentation.PresentHistoryItem(
		item.Direction, item.SourceActorName, item.SourceOrbitName,
		result.RequestedDelivery, result.EffectiveDelivery, result.Status,
		store.TransmissionReason(result.ReasonCode), result.Actions,
	)
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
	Contract          string                               `json:"contract"`
	HistoryItemID     string                               `json:"history_item_id"`
	ItemKind          string                               `json:"item_kind"`
	Direction         store.HistoryDirection               `json:"direction"`
	OccurredAt        string                               `json:"occurred_at"`
	AcceptedAt        string                               `json:"accepted_at,omitempty"`
	ExpiresAt         string                               `json:"expires_at,omitempty"`
	Media             historyMediaJSON                     `json:"media"`
	Sender            *historySenderJSON                   `json:"sender,omitempty"`
	Audience          *transmissionAudienceResponse        `json:"audience,omitempty"`
	RequestedDelivery string                               `json:"requested_delivery,omitempty"`
	EffectiveDelivery string                               `json:"effective_delivery,omitempty"`
	DowngradeReason   string                               `json:"downgrade_reason,omitempty"`
	Status            string                               `json:"status"`
	ReasonCode        string                               `json:"reason_code,omitempty"`
	TargetCounts      *transmissionTargetCountsResponse    `json:"target_counts,omitempty"`
	Automation        *historyAutomationJSON               `json:"automation,omitempty"`
	Actions           []string                             `json:"actions"`
	Presentation      presentation.HistoryItemPresentation `json:"presentation"`
}

func (api *onboardingAPI) historyItem(w http.ResponseWriter, r *http.Request) {
	id, action, validPath := parseHistoryItemPath(r.URL.Path)
	if !validPath {
		apiError(w, http.StatusNotFound, errorHistoryNotFound, 0)
		return
	}
	if action == "receipts" {
		api.historyReceipts(w, r, id)
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
		DowngradeReason: base.DowngradeReason, Status: base.Status, ReasonCode: base.ReasonCode,
		Actions: base.Actions, Automation: base.Automation}
	result.Presentation = base.Presentation
	if item.Transmission != nil {
		result.AcceptedAt = coordTime(item.Transmission.AcceptedAt)
		result.ExpiresAt = coordTime(item.Transmission.ExpiresAt)
		counts := targetCountsResponse(item.TargetStatusCounts)
		result.TargetCounts = &counts
	}
	writeJSON(w, http.StatusOK, result)
}

func parseHistoryItemPath(path string) (historyItemID, action string, ok bool) {
	parts := strings.Split(strings.TrimPrefix(path, "/v1/history/"), "/")
	if len(parts) != 1 &&
		(len(parts) != 2 || parts[1] != "receipts") &&
		(len(parts) != 3 || parts[1] != "actions") {
		return "", "", false
	}
	if len(parts[0]) != 29 || !strings.HasPrefix(parts[0], "hi_") {
		return "", "", false
	}
	if len(parts) == 2 {
		return parts[0], "receipts", true
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
	case "cancel":
		var request struct{}
		if !decodeStrictTransmissionJSON(w, r, &request) {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		item, err := api.store.GetAuthorizedHistoryItem(actorRequest.Context.ActorID,
			store.Identity{Kind: store.IdentityBearer, Token: actorRequest.Bearer}, historyItemID, now)
		if err != nil || item.Transmission == nil || !item.CanCancel {
			if err == nil {
				err = store.ErrTransmissionNotFound
			}
			api.historyActionError(w, "cancel history transmission", err)
			return
		}
		cancelled, err := api.store.CancelAuthorizedTransmission(actorRequest.Context.ActorID,
			actorRequest.Bearer, item.Transmission.ID, now)
		if api.historyActionError(w, "cancel history transmission", err) {
			return
		}
		api.transmissionCancelled(cancelled)
		writeJSON(w, http.StatusOK, map[string]any{
			"history_item_id": historyItemID, "cancelled": true,
			"reason_code": string(store.TransmissionReasonSenderCancelled),
		})
	case "replay":
		api.replayHistoryAction(w, r, service, actorRequest, actor, historyItemID, now)
	case "delete":
		var request struct{}
		if !decodeStrictTransmissionJSON(w, r, &request) {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		_, err := service.Delete(actor, historyItemID, now)
		if api.historyActionError(w, "delete history media", err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"history_item_id": historyItemID, "deleted": true,
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
		delete(response, "media_id")
		response["history_item_id"] = historyItemID
		response["reused"] = created.Reused
		writeJSON(w, status, response)
	case "block_actor", "block_orbit":
		api.blockHistoryAction(w, r, service, actorRequest, actor, historyItemID, action, now)
	case "disable_schedule", "revoke_principal", "emergency_disable_automation":
		api.automationHistoryControlAction(w, r, actorRequest, historyItemID, action, now)
	default:
		apiError(w, http.StatusNotFound, errorHistoryNotFound, 0)
	}
}

func (api *onboardingAPI) automationHistoryControlAction(w http.ResponseWriter, r *http.Request,
	actor actorRequest, historyItemID, action string, now int64) {
	var request expectedRevisionHTTPRequest
	if !automationJSONRequest(r) || !decodeStrictJSON(w, r, automationRequestMaxBytes, &request) || request.ExpectedRevision <= 0 {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	item, err := api.store.GetAuthorizedHistoryItem(actor.Context.ActorID,
		store.Identity{Kind: store.IdentityBearer, Token: actor.Bearer}, historyItemID, now)
	if err != nil || item.Automation == nil {
		if err == nil {
			err = store.ErrTransmissionNotFound
		}
		api.historyActionError(w, "authorize automation history action", err)
		return
	}
	auth, ok := automationControlMutationAuth(r, actor, request, api.automationNow())
	if !ok {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	switch action {
	case "disable_schedule":
		if !item.CanDisableSchedule || item.Automation.ScheduleID == "" {
			apiError(w, http.StatusConflict, errorHistoryActionUnavailable, 0)
			return
		}
		result, err := api.store.SetAuthorizedAutomationScheduleEnabled(auth,
			item.Automation.ScheduleID, request.ExpectedRevision, false)
		if api.automationControlError(w, "disable automation schedule from history", errorAutomationSchedule, err) {
			return
		}
		if !result.Replayed {
			if err := api.reconcileAutomationCancellations(auth.Now); err != nil {
				api.internalError(w, "cancel disabled automation schedule", err)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"history_item_id": historyItemID,
			"schedule": automationScheduleHTTP(result.Control), "replayed": result.Replayed})
	case "revoke_principal":
		if !item.CanRevokePrincipal || item.Automation.PrincipalRef == "" {
			apiError(w, http.StatusConflict, errorHistoryActionUnavailable, 0)
			return
		}
		result, err := api.store.RevokeAuthorizedAutomationPrincipal(auth,
			item.Automation.PrincipalID, request.ExpectedRevision)
		if api.automationControlError(w, "revoke automation principal from history", errorAutomationPrincipal, err) {
			return
		}
		if !result.Replayed {
			if err := api.reconcileAutomationCancellations(auth.Now); err != nil {
				api.internalError(w, "cancel revoked automation principal", err)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"history_item_id": historyItemID,
			"principal": automationPrincipalHTTP(result.Principal), "replayed": result.Replayed})
	case "emergency_disable_automation":
		if !item.CanEmergencyDisable {
			apiError(w, http.StatusConflict, errorHistoryActionUnavailable, 0)
			return
		}
		current, err := api.store.AuthorizedAutomationFeatureState(actor.Context.ActorID, actor.Bearer)
		if err != nil || current.Revision != request.ExpectedRevision {
			if err == nil {
				err = store.ErrAutomationStateConflict
			}
			api.automationControlError(w, "read automation emergency state", errorAutomationSchedule, err)
			return
		}
		var quiet []store.AutomationQuietWindow
		if err := json.Unmarshal([]byte(current.QuietHoursJSON), &quiet); err != nil {
			api.internalError(w, "decode automation quiet hours", err)
			return
		}
		result, err := api.store.ReplaceAuthorizedAutomationFeatureState(auth,
			store.AutomationFeatureControlParams{SoundboardEnabled: current.SoundboardEnabled,
				AutomationEnabled: current.AutomationEnabled, EmergencyDisabled: true,
				Timezone: current.Timezone, QuietHours: quiet, ExpectedRevision: current.Revision})
		if api.automationControlError(w, "emergency disable automation from history", errorAutomationSchedule, err) {
			return
		}
		if !result.Replayed {
			if err := api.reconcileAutomationCancellations(auth.Now); err != nil {
				api.internalError(w, "cancel emergency-disabled automation", err)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"history_item_id": historyItemID,
			"automation": automationFeatureHTTP(result.State), "replayed": result.Replayed})
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
		apiError(w, http.StatusGone, errorCursorExpired, 0)
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
