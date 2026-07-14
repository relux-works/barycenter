package main

import (
	"errors"
	"net/http"
	"regexp"
	"strings"

	"relux.works/duet/coordinator/internal/store"
)

var mediaDeleteItemIDPattern = regexp.MustCompile(`^m_[0-9A-HJKMNP-TV-Z]{26}$`)

func (api *onboardingAPI) deleteMediaItem(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodDelete || r.URL.RawQuery != "" ||
		r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	mediaID := strings.TrimPrefix(r.URL.Path, "/v1/media/")
	if !mediaDeleteItemIDPattern.MatchString(mediaID) || strings.Contains(mediaID, "/") {
		apiError(w, http.StatusNotFound, errorMediaNotFound, 0)
		return
	}
	if api.mediaLifecycleInitErr != nil || api.mediaLifecycle == nil {
		api.log.Error("initialize media lifecycle failed")
		apiError(w, http.StatusInternalServerError, errorInternal, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	_, err := api.mediaLifecycle.DeleteAuthorized(
		actor.Context.ActorID, actor.Bearer, mediaID,
	)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusNoContent)
	case errors.Is(err, store.ErrMediaNotFound), errors.Is(err, store.ErrMediaStateConflict):
		apiError(w, http.StatusNotFound, errorMediaNotFound, 0)
	case errors.Is(err, store.ErrUnauthorized):
		apiError(w, http.StatusUnauthorized, errorUnauthorized, 0)
	case errors.Is(err, store.ErrInsufficientCapability),
		errors.Is(err, store.ErrOrbitDisabled),
		errors.Is(err, store.ErrSelfServiceOnboardingDisabled):
		apiError(w, http.StatusForbidden, errorInsufficientCapability, 0)
	default:
		// Repository errors and filesystem paths are not reflected or logged.
		api.log.Error("delete media lifecycle item failed")
		apiError(w, http.StatusInternalServerError, errorInternal, 0)
	}
}
