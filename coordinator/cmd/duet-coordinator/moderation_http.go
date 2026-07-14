package main

import (
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"relux.works/duet/coordinator/internal/store"
)

func (api *onboardingAPI) moderationReady(w http.ResponseWriter) bool {
	if api.moderationService == nil {
		apiError(w, http.StatusServiceUnavailable, errorServiceUnavailable, 0)
		return false
	}
	return true
}

func (api *onboardingAPI) moderationReports(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || !api.moderationReady(w) {
		if r.Method != http.MethodPost {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		}
		return
	}
	if r.URL.RawQuery != "" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	var request struct {
		MediaID string                 `json:"media_id"`
		Reason  store.ModerationReason `json:"reason"`
		Details string                 `json:"details"`
	}
	if !decodeBoundedJSON(w, r, 4096, &request) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	created, err := api.moderationService.CreateReport(
		actor.Context.ActorID, actor.Bearer,
		store.CreateModerationReportParams{
			MediaID: request.MediaID, Reason: request.Reason, Details: request.Details,
		},
	)
	if api.moderationError(w, err) {
		return
	}
	status := http.StatusCreated
	if created.Reused {
		status = http.StatusOK
	}
	writeJSON(w, status, reporterModerationView(created.Report))
}

func reporterModerationView(report store.ModerationReport) map[string]any {
	state := "received"
	if report.Status == "resolved" {
		state = "reviewed"
	}
	return map[string]any{
		"id": report.ID, "media_id": report.MediaID,
		"reason": report.Reason, "status": state,
		"created_at": report.CreatedAt, "updated_at": report.UpdatedAt,
	}
}

func (api *onboardingAPI) moderationReportItem(w http.ResponseWriter, r *http.Request) {
	if !api.moderationReady(w) {
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/reports/")
	if path == "" || strings.Contains(path, "//") {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	if strings.HasSuffix(path, "/block") {
		if r.Method != http.MethodPost {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		var request struct{}
		if !decodeBoundedJSON(w, r, 64, &request) {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		reportID := strings.TrimSuffix(path, "/block")
		created, err := api.moderationService.BlockReportedSender(
			actor.Context.ActorID, actor.Bearer, reportID,
		)
		if api.moderationError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"report_id": reportID, "blocked": true, "reused": created.Reused,
		})
		return
	}
	if r.Method != http.MethodGet {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	if r.URL.RawQuery != "" || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	report, err := api.store.GetAuthorizedModerationReport(
		actor.Context.ActorID, actor.Bearer, path,
	)
	if api.moderationError(w, err) {
		return
	}
	writeJSON(w, http.StatusOK, reporterModerationView(report))
}

func operatorReportView(report store.ModerationReport) map[string]any {
	return map[string]any{
		"id": report.ID, "status": report.Status,
		"reporter": map[string]any{
			"orbit_id": report.ReporterOrbitID, "actor_id": report.ReporterActorID,
		},
		"reported_subject": map[string]any{
			"orbit_id": report.ReportedOrbitID, "actor_id": report.ReportedActorID,
		},
		"media": map[string]any{
			"id": report.MediaID, "kind": report.MediaKind,
			"source": report.MediaSource, "title": report.MediaTitle,
			"duration_ms": report.MediaDurationMS, "mime": report.EvidenceMIME,
			"size_bytes": report.EvidenceSizeBytes,
		},
		"accepted_target": map[string]any{
			"transmission_id": report.TransmissionID,
			"orbit_id":        report.TargetOrbitID, "actor_id": report.TargetActorID,
			"slot": report.TargetSlot, "audience": report.AudienceKind,
			"playback_domain_kind": report.PlaybackDomainKind,
			"playback_domain_id":   report.PlaybackDomainID,
			"accepted_at":          report.AcceptedAt,
		},
		"reason": report.Reason, "details": report.Details,
		"evidence_expires_at": report.EvidenceExpiresAt,
		"created_at":          report.CreatedAt, "updated_at": report.UpdatedAt,
		"resolved_at": report.ResolvedAt,
	}
}

func (api *onboardingAPI) moderationQueue(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet || !api.moderationReady(w) {
		if r.Method != http.MethodGet {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		}
		return
	}
	if r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	operator := r.Context().Value(moderationOperatorRequestKey{}).(moderationOperatorRequest)
	query := r.URL.Query()
	for key, values := range query {
		if (key != "status" && key != "limit") || len(values) != 1 {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
	}
	status := query.Get("status")
	limit := 50
	if raw := query.Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		limit = parsed
	}
	reports, err := api.store.ListModerationReports(
		operator.Context.ID, operator.Bearer, status, limit,
	)
	if api.moderationError(w, err) {
		return
	}
	items := make([]map[string]any, 0, len(reports))
	for _, report := range reports {
		items = append(items, operatorReportView(report))
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (api *onboardingAPI) moderationQueueItem(w http.ResponseWriter, r *http.Request) {
	if !api.moderationReady(w) {
		return
	}
	operator := r.Context().Value(moderationOperatorRequestKey{}).(moderationOperatorRequest)
	path := strings.TrimPrefix(r.URL.Path, "/v1/moderation/reports/")
	switch {
	case strings.HasSuffix(path, "/evidence") && r.Method == http.MethodGet:
		if r.URL.RawQuery != "" || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		reportID := strings.TrimSuffix(path, "/evidence")
		download, err := api.moderationService.OpenEvidence(
			r.Context(), operator.Context.ID, operator.Bearer, reportID,
		)
		if api.moderationError(w, err) {
			return
		}
		defer download.File.Close()
		w.Header().Set("Content-Type", download.Evidence.MIME)
		w.Header().Set("Content-Length", strconv.FormatInt(download.Evidence.SizeBytes, 10))
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Content-Disposition", "attachment; filename=moderation-evidence.bin")
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, download.File)
	case strings.HasSuffix(path, "/decision") && r.Method == http.MethodPost:
		reportID := strings.TrimSuffix(path, "/decision")
		var request struct {
			Action store.ModerationAction `json:"action"`
		}
		if !decodeBoundedJSON(w, r, 256, &request) {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		decision, err := api.moderationService.ApplyDecision(
			r.Context(), operator.Context.ID, operator.Bearer,
			reportID, request.Action,
		)
		if api.moderationError(w, err) {
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"report_id": reportID, "decision_id": decision.ID,
			"action": decision.Action, "state": decision.State,
			"applied_at": decision.AppliedAt,
		})
	default:
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
	}
}

func (api *onboardingAPI) moderationError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, store.ErrUnauthorized):
		apiError(w, http.StatusUnauthorized, errorUnauthorized, 0)
	case errors.Is(err, store.ErrModerationForbidden):
		apiError(w, http.StatusForbidden, errorModerationForbidden, 0)
	case errors.Is(err, store.ErrModerationNotFound), errors.Is(err, store.ErrMediaNotFound):
		apiError(w, http.StatusNotFound, errorModerationNotFound, 0)
	case errors.Is(err, store.ErrModerationRateLimited):
		apiError(w, http.StatusTooManyRequests, errorTooManyAttempts, time.Hour)
	case errors.Is(err, store.ErrModerationDecisionConflict):
		apiError(w, http.StatusConflict, errorModerationConflict, 0)
	case errors.Is(err, store.ErrModerationEvidenceExpired):
		apiError(w, http.StatusGone, errorEvidenceExpired, 0)
	case errors.Is(err, store.ErrModerationInvalid),
		errors.Is(err, store.ErrTransmissionInvalid):
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
	default:
		api.internalError(w, "moderation operation", err)
	}
	return true
}
