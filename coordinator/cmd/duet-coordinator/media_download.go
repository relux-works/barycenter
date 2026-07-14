package main

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"relux.works/duet/coordinator/internal/store"
)

func (api *onboardingAPI) mediaItem(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		api.downloadMediaItem(w, r)
	case http.MethodDelete:
		actor := r.Context().Value(actorRequestKey{}).(actorRequest)
		if !actor.Context.Capabilities.Has(store.CapabilityControl) {
			apiError(w, http.StatusForbidden, errorInsufficientCapability, 0)
			return
		}
		api.deleteMediaItem(w, r)
	default:
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
	}
}

func (api *onboardingAPI) downloadMediaItem(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	mediaID := strings.TrimPrefix(r.URL.Path, "/v1/media/")
	if !mediaItemIDPattern.MatchString(mediaID) || strings.Contains(mediaID, "/") {
		apiError(w, http.StatusNotFound, errorMediaNotFound, 0)
		return
	}
	if api.mediaDownloadInitErr != nil || api.mediaDownload == nil {
		api.log.Error("initialize media download failed")
		apiError(w, http.StatusInternalServerError, errorInternal, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	download, err := api.mediaDownload.OpenAuthorized(
		r.Context(), actor.Context, actor.Bearer, mediaID,
	)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrMediaNotFound), errors.Is(err, store.ErrMediaStateConflict):
			apiError(w, http.StatusNotFound, errorMediaNotFound, 0)
		case errors.Is(err, store.ErrUnauthorized):
			apiError(w, http.StatusUnauthorized, errorUnauthorized, 0)
		case errors.Is(err, store.ErrInsufficientCapability),
			errors.Is(err, store.ErrOrbitDisabled),
			errors.Is(err, store.ErrSelfServiceOnboardingDisabled):
			apiError(w, http.StatusForbidden, errorInsufficientCapability, 0)
		default:
			// Tokens, media IDs, titles, target identities and local paths are
			// deliberately absent from the operational log.
			api.log.Error("open authorized media download failed")
			apiError(w, http.StatusInternalServerError, errorInternal, 0)
		}
		return
	}
	defer download.File.Close()
	w.Header().Set("Content-Type", download.Item.MIME)
	w.Header().Set("X-Content-Type-Options", "nosniff")
	if download.Item.SHA256 != "" {
		w.Header().Set("ETag", `"`+download.Item.SHA256+`"`)
	}
	http.ServeContent(
		w, r, "", time.UnixMilli(download.Item.PublishedAt), download.File,
	)
}
