package main

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"relux.works/duet/coordinator/internal/store"
)

var streamVariantIDPattern = regexp.MustCompile(`^sv_[0-9A-HJKMNP-TV-Z]{26}$`)

const maxStreamRangeBytes = int64(1 << 20)

func (api *onboardingAPI) mediaItem(w http.ResponseWriter, r *http.Request) {
	if strings.Contains(strings.TrimPrefix(r.URL.Path, "/v1/media/"), "/variants/") {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
			return
		}
		api.downloadStreamVariant(w, r)
		return
	}
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

func parseStreamVariantPath(path string) (string, string, bool) {
	relative := strings.TrimPrefix(path, "/v1/media/")
	parts := strings.Split(relative, "/")
	if len(parts) != 3 || parts[1] != "variants" ||
		!mediaItemIDPattern.MatchString(parts[0]) ||
		!streamVariantIDPattern.MatchString(parts[2]) {
		return "", "", false
	}
	return parts[0], parts[2], true
}

func streamETagMatches(value, etag string) bool {
	for _, candidate := range strings.Split(value, ",") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "*" || candidate == etag || strings.TrimPrefix(candidate, "W/") == etag {
			return true
		}
	}
	return false
}

func parseStreamByteRange(value string, size int64) (start, end int64, partial bool, ok bool) {
	if value == "" {
		return 0, size - 1, false, size > 0
	}
	if !strings.HasPrefix(value, "bytes=") || strings.Contains(value, ",") || size <= 0 {
		return 0, 0, false, false
	}
	spec := strings.TrimPrefix(value, "bytes=")
	left, right, found := strings.Cut(spec, "-")
	if !found || (left == "" && right == "") {
		return 0, 0, false, false
	}
	if left == "" {
		suffix, err := strconv.ParseInt(right, 10, 64)
		if err != nil || suffix <= 0 {
			return 0, 0, false, false
		}
		if suffix > size {
			suffix = size
		}
		return size - suffix, size - 1, true, true
	}
	start, err := strconv.ParseInt(left, 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false, false
	}
	if right == "" {
		return start, size - 1, true, true
	}
	end, err = strconv.ParseInt(right, 10, 64)
	if err != nil || end < start {
		return 0, 0, false, false
	}
	if end >= size {
		end = size - 1
	}
	return start, end, true, true
}

func streamHTTPRequestKey() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return "stream-http-range-v1-" + hex.EncodeToString(raw), nil
}

func setStreamVariantHeaders(w http.ResponseWriter, variant store.StreamVariant) {
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Type", variant.MIME)
	w.Header().Set("ETag", variant.ETag)
	w.Header().Set("Vary", "Authorization, X-Codec-Spike-Target")
	w.Header().Set("X-Content-SHA256", variant.SHA256)
	w.Header().Set("X-Content-Type-Options", "nosniff")
}

func (api *onboardingAPI) downloadStreamVariant(w http.ResponseWriter, r *http.Request) {
	if r.URL.RawQuery != "" || r.ContentLength != 0 || len(r.TransferEncoding) != 0 {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	mediaID, variantID, ok := parseStreamVariantPath(r.URL.Path)
	if !ok {
		apiError(w, http.StatusNotFound, errorMediaNotFound, 0)
		return
	}
	if api.mediaDownloadInitErr != nil || api.mediaDownload == nil {
		api.log.Error("initialize stream variant download failed")
		apiError(w, http.StatusInternalServerError, errorInternal, 0)
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	download, err := api.mediaDownload.OpenAuthorizedStreamVariant(
		r.Context(), actor.Context, actor.Bearer, mediaID, variantID,
	)
	if err != nil {
		switch {
		case errors.Is(err, store.ErrStreamTrackNotFound),
			errors.Is(err, store.ErrMediaNotFound),
			errors.Is(err, store.ErrMediaStateConflict):
			apiError(w, http.StatusNotFound, errorMediaNotFound, 0)
		case errors.Is(err, store.ErrUnauthorized):
			apiError(w, http.StatusUnauthorized, errorUnauthorized, 0)
		default:
			api.log.Error("open authorized stream variant failed")
			apiError(w, http.StatusInternalServerError, errorInternal, 0)
		}
		return
	}
	defer download.File.Close()
	variant := download.Variant
	if streamETagMatches(strings.Join(r.Header.Values("If-None-Match"), ","), variant.ETag) {
		setStreamVariantHeaders(w, variant)
		w.WriteHeader(http.StatusNotModified)
		return
	}
	rangeValues := r.Header.Values("Range")
	rangeValue := ""
	if len(rangeValues) == 1 {
		rangeValue = rangeValues[0]
	} else if len(rangeValues) > 1 {
		rangeValue = "invalid-multiple-range-fields"
	}
	if rangeValue != "" {
		ifRangeValues := r.Header.Values("If-Range")
		if len(ifRangeValues) > 1 ||
			(len(ifRangeValues) == 1 && ifRangeValues[0] != variant.ETag) {
			rangeValue = ""
		}
	}
	start, end, partial, valid := parseStreamByteRange(rangeValue, variant.SizeBytes)
	if valid && partial && end-start+1 > maxStreamRangeBytes {
		valid = false
	}
	if !valid {
		setStreamVariantHeaders(w, variant)
		w.Header().Set("Content-Range", "bytes */"+strconv.FormatInt(variant.SizeBytes, 10))
		w.Header().Set("Content-Length", "0")
		w.WriteHeader(http.StatusRequestedRangeNotSatisfiable)
		return
	}
	length := end - start + 1
	status := http.StatusOK
	if partial {
		status = http.StatusPartialContent
	}
	if r.Method == http.MethodHead {
		setStreamVariantHeaders(w, variant)
		w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
		if partial {
			w.Header().Set("Content-Range", "bytes "+strconv.FormatInt(start, 10)+"-"+
				strconv.FormatInt(end, 10)+"/"+strconv.FormatInt(variant.SizeBytes, 10))
		}
		w.WriteHeader(status)
		return
	}
	requestKey, err := streamHTTPRequestKey()
	if err != nil {
		api.log.Error("create stream egress identity failed")
		apiError(w, http.StatusInternalServerError, errorInternal, 0)
		return
	}
	now := api.mediaUploadNow()
	egress, err := api.store.BeginStreamEgress(store.BeginStreamEgressParams{
		VariantID: variant.ID, IdempotencyKey: requestKey,
		PlaybackGeneration: now.UnixNano(), CreatedAt: now.UnixMilli(),
	})
	if err != nil {
		switch {
		case errors.Is(err, store.ErrStreamQuotaExceeded):
			apiError(w, http.StatusTooManyRequests, errorUploadQuota, 0)
		case errors.Is(err, store.ErrStreamAccountingNotFound):
			apiError(w, http.StatusNotFound, errorMediaNotFound, 0)
		default:
			api.log.Error("reserve stream egress failed")
			apiError(w, http.StatusInternalServerError, errorInternal, 0)
		}
		return
	}
	if _, err := download.File.Seek(start, io.SeekStart); err != nil {
		_, _ = api.store.CompleteStreamEgress(egress.ID, egress.Revision, "cancelled", now.UnixMilli())
		api.log.Error("seek stream variant failed")
		apiError(w, http.StatusInternalServerError, errorInternal, 0)
		return
	}
	setStreamVariantHeaders(w, variant)
	w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
	if partial {
		w.Header().Set("Content-Range", "bytes "+strconv.FormatInt(start, 10)+"-"+
			strconv.FormatInt(end, 10)+"/"+strconv.FormatInt(variant.SizeBytes, 10))
	}
	w.WriteHeader(status)
	written, copyErr := io.CopyN(w, download.File, length)
	outcome := "served"
	if copyErr != nil || written != length {
		outcome = "client_cancelled"
	}
	egress, _, recordErr := api.store.RecordStreamEgress(store.RecordStreamEgressParams{
		SessionID: egress.ID, RequestKey: requestKey + "-write",
		ExpectedRevision: egress.Revision, RangeStart: start, RangeEnd: end,
		ActualBytes: written, Outcome: outcome, CreatedAt: api.mediaUploadNow().UnixMilli(),
	})
	if recordErr == nil {
		terminalState := "completed"
		if outcome != "served" {
			terminalState = "cancelled"
		}
		_, recordErr = api.store.CompleteStreamEgress(
			egress.ID, egress.Revision, terminalState, api.mediaUploadNow().UnixMilli(),
		)
	}
	if recordErr != nil || copyErr != nil || written != length {
		api.log.Error("serve stream variant bytes failed")
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
