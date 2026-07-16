package main

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"relux.works/duet/coordinator/internal/store"
)

type streamAccountingHealthView struct {
	Ready            bool  `json:"ready"`
	Saturated        bool  `json:"saturated"`
	LastReconciledAt int64 `json:"last_reconciled_at"`
}

type phase2HealthView struct {
	Ready                 bool   `json:"ready"`
	StreamedTracksEnabled bool   `json:"streamed_tracks_enabled"`
	AirRoomsEnabled       bool   `json:"air_rooms_enabled"`
	AirAuthorityState     string `json:"air_authority_state"`
}

func applyPhase2RuntimeReadiness(
	readiness *store.Phase2Readiness,
	mediaProcessorReady, streamStorageReady bool,
) {
	if readiness.MediaProcessorRequired {
		readiness.MediaProcessorReady = mediaProcessorReady
	} else {
		readiness.MediaProcessorReady = true
	}
	if readiness.StreamStorageRequired {
		readiness.StreamStorageReady = streamStorageReady
	} else {
		readiness.StreamStorageReady = true
	}
	readiness.Ready =
		(!readiness.StreamAccountingRequired || readiness.StreamAccountingReady) &&
			readiness.MediaProcessorReady && readiness.StreamStorageReady &&
			readiness.AirRuntimeReady
}

// addStreamAccountingHealth deliberately exposes only readiness and a coarse
// saturation bit on the public health route. Exact storage, egress and scope
// usage remain behind the authenticated operator surface.
func addStreamAccountingHealth(body map[string]any, st *store.Store, now int64, runtimeReady ...bool) {
	if st == nil {
		body["status"] = "degraded"
		body["stream_accounting"] = map[string]string{"status": "unavailable"}
		body["phase2"] = map[string]string{"status": "unavailable"}
		return
	}
	view, err := st.Phase2HealthSnapshot(now)
	if err != nil {
		body["status"] = "degraded"
		body["stream_accounting"] = map[string]string{"status": "unavailable"}
		body["phase2"] = map[string]string{"status": "unavailable"}
		return
	}
	processorReady, storageReady := false, false
	if len(runtimeReady) > 0 {
		processorReady = runtimeReady[0]
	}
	if len(runtimeReady) > 1 {
		storageReady = runtimeReady[1]
	}
	applyPhase2RuntimeReadiness(&view.Readiness, processorReady, storageReady)
	body["stream_accounting"] = streamAccountingHealthView{
		Ready: view.Accounting.Ready, Saturated: view.Accounting.Saturated,
		LastReconciledAt: view.Accounting.LastReconciledAt,
	}
	body["phase2"] = phase2HealthView{
		Ready:                 view.Readiness.Ready,
		StreamedTracksEnabled: view.Features.StreamedTracks.Enabled,
		AirRoomsEnabled:       view.Features.AirRooms.Enabled,
		AirAuthorityState:     view.Features.AirRooms.State,
	}
	if !view.Readiness.Ready {
		body["status"] = "degraded"
	}
}

func requireStreamAccountingCapability(w http.ResponseWriter, r *http.Request, decide bool) (moderationOperatorRequest, bool) {
	operator := r.Context().Value(moderationOperatorRequestKey{}).(moderationOperatorRequest)
	allowed := operator.Context.Capabilities.List
	if decide {
		allowed = operator.Context.Capabilities.Decide
	}
	if !allowed {
		apiError(w, http.StatusForbidden, errorModerationForbidden, 0)
		return moderationOperatorRequest{}, false
	}
	return operator, true
}

func (api *onboardingAPI) streamAccountingOperatorView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	operator, ok := requireStreamAccountingCapability(w, r, false)
	if !ok {
		return
	}
	query := r.URL.Query()
	for key, values := range query {
		if (key != "scope_kind" && key != "scope_id") || len(values) != 1 {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
	}
	scopeKind, rawScopeID := query.Get("scope_kind"), query.Get("scope_id")
	var scopeID int64
	if scopeKind != "" || rawScopeID != "" {
		var err error
		scopeID, err = strconv.ParseInt(rawScopeID, 10, 64)
		if (scopeKind != "actor" && scopeKind != "orbit") || err != nil || scopeID <= 0 {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
	}
	view, err := api.store.GetAuthorizedStreamAccounting(
		operator.Context.ID, operator.Bearer, scopeKind, scopeID, time.Now().UnixMilli(),
	)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrUnauthorized):
			apiError(w, http.StatusUnauthorized, errorUnauthorized, 0)
		case errors.Is(err, store.ErrModerationForbidden):
			apiError(w, http.StatusForbidden, errorModerationForbidden, 0)
		case errors.Is(err, store.ErrStreamAccountingInvalid), errors.Is(err, store.ErrStreamAccountingNotFound):
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		default:
			api.internalError(w, "read stream accounting view", err)
		}
		return
	}
	writeJSON(w, http.StatusOK, view)
}

func (api *onboardingAPI) phase2ObservabilityOperatorView(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.ContentLength != 0 || len(r.TransferEncoding) != 0 ||
		r.URL.RawQuery != "" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	operator, ok := requireStreamAccountingCapability(w, r, false)
	if !ok {
		return
	}
	view, err := api.store.GetAuthorizedPhase2Observability(
		operator.Context.ID, operator.Bearer, time.Now().UnixMilli(),
	)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrUnauthorized):
			apiError(w, http.StatusUnauthorized, errorUnauthorized, 0)
		case errors.Is(err, store.ErrModerationForbidden):
			apiError(w, http.StatusForbidden, errorModerationForbidden, 0)
		case errors.Is(err, store.ErrStreamAccountingInvalid):
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		default:
			api.internalError(w, "read Phase 2 observability view", err)
		}
		return
	}
	applyPhase2RuntimeReadiness(
		&view.Readiness,
		api.mediaSubmitter != nil && api.mediaSubmitterInitErr == nil,
		api.mediaUploadInitErr == nil && api.mediaLifecycleInitErr == nil &&
			api.mediaDownloadInitErr == nil,
	)
	writeJSON(w, http.StatusOK, view)
}

func (api *onboardingAPI) streamAccountingPolicies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.RawQuery != "" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	operator, ok := requireStreamAccountingCapability(w, r, true)
	if !ok {
		return
	}
	var request struct {
		Policy           store.StreamQuotaPolicy `json:"policy"`
		ExpectedRevision int64                   `json:"expected_revision"`
		Reason           string                  `json:"reason"`
	}
	if !decodeBoundedJSON(w, r, 8192, &request) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	updated, err := api.store.SetStreamQuotaPolicy(
		operator.Context.ID, operator.Bearer, request.Policy, request.ExpectedRevision,
		request.Reason, time.Now().UnixMilli(),
	)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrUnauthorized):
			apiError(w, http.StatusUnauthorized, errorUnauthorized, 0)
		case errors.Is(err, store.ErrModerationForbidden):
			apiError(w, http.StatusForbidden, errorModerationForbidden, 0)
		case errors.Is(err, store.ErrStreamAccountingInvalid):
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		case errors.Is(err, store.ErrStreamAccountingConflict):
			apiError(w, http.StatusConflict, errorModerationConflict, 0)
		default:
			api.internalError(w, "update stream accounting policy", err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"policy": updated})
}

func (api *onboardingAPI) streamAccountingPolicyAudit(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	operator, ok := requireStreamAccountingCapability(w, r, false)
	if !ok {
		return
	}
	query := r.URL.Query()
	for key, values := range query {
		if (key != "scope_kind" && key != "scope_id" && key != "limit") || len(values) != 1 {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
	}
	scopeKind := query.Get("scope_kind")
	scopeID, err := strconv.ParseInt(query.Get("scope_id"), 10, 64)
	limit := 100
	if raw := query.Get("limit"); raw != "" {
		limit, err = strconv.Atoi(raw)
	}
	if (scopeKind != "actor" && scopeKind != "orbit") || err != nil ||
		scopeID < 0 || limit < 1 || limit > 500 {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	events, err := api.store.ListAuthorizedStreamQuotaPolicyAudit(
		operator.Context.ID, operator.Bearer, scopeKind, scopeID, limit,
	)
	if err != nil {
		if errors.Is(err, store.ErrUnauthorized) {
			apiError(w, http.StatusUnauthorized, errorUnauthorized, 0)
		} else if errors.Is(err, store.ErrModerationForbidden) {
			apiError(w, http.StatusForbidden, errorModerationForbidden, 0)
		} else {
			api.internalError(w, "read stream accounting policy audit", err)
		}
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": events})
}
