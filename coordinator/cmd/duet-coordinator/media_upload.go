package main

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"relux.works/duet/coordinator/internal/media"
	"relux.works/duet/coordinator/internal/store"
)

const (
	mediaUploadSessionLifetime = time.Hour
	mediaClipRetention         = 7 * 24 * time.Hour
	mediaUploadSweepLimit      = 1000
)

var (
	mediaUploadIDPattern      = regexp.MustCompile(`^up_[0-9A-HJKMNP-TV-Z]{26}$`)
	mediaUploadIdempotencyKey = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:-]{15,127}$`)
	errMediaUploadTempBehind  = errors.New("media upload temp bytes are behind persisted offset")
)

type mediaUploadResponse struct {
	UploadID    string `json:"upload_id"`
	MediaID     string `json:"media_id"`
	UploadToken string `json:"upload_token,omitempty"`
	Offset      int64  `json:"upload_offset"`
	Length      int64  `json:"upload_length"`
	ExpiresAt   string `json:"expires_at"`
	Status      string `json:"status"`
	Reused      bool   `json:"reused,omitempty"`
}

type mediaUploadSubmitter interface {
	SubmitUpload(context.Context, string) (store.MediaItem, error)
}

func (api *onboardingAPI) initializeMediaUploadStorage() error {
	if api.config.MediaDir == "" {
		return errors.New("media upload storage is not configured")
	}
	api.mediaUploadDir = filepath.Join(api.config.MediaDir, ".uploads")
	if err := os.MkdirAll(api.mediaUploadDir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(api.mediaUploadDir, 0o700); err != nil {
		return err
	}
	if err := api.removeAbandonedMediaUploadChunks(api.mediaUploadNow(), true); err != nil {
		return err
	}
	return api.maintainMediaUploadStorage(api.mediaUploadNow())
}

func (api *onboardingAPI) mediaUploadStorageReady(w http.ResponseWriter, now time.Time) bool {
	api.mediaUploadMaintenance.Lock()
	defer api.mediaUploadMaintenance.Unlock()
	var err error
	if api.mediaUploadInitErr != nil {
		err = api.initializeMediaUploadStorage()
	} else {
		err = api.maintainMediaUploadStorage(now)
	}
	api.mediaUploadInitErr = err
	if err != nil {
		api.mediaUploadInternalError(w, "maintain media upload storage")
		return false
	}
	return true
}

func (api *onboardingAPI) maintainMediaUploadStorage(now time.Time) error {
	if err := api.removeAbandonedMediaUploadChunks(now, false); err != nil {
		return err
	}
	nowMS := now.UnixMilli()
	expired, err := api.store.ExpiredMediaUploadSessions(nowMS, mediaUploadSweepLimit)
	if err != nil {
		return err
	}
	for _, candidate := range expired {
		lock := api.mediaUploadLock(candidate.ID)
		lock.Lock()
		_, err := api.store.ExpireMediaUploadSession(candidate.ID, candidate.Revision, nowMS)
		lock.Unlock()
		if err != nil && !errors.Is(err, store.ErrMediaStateConflict) &&
			!errors.Is(err, store.ErrMediaNotFound) {
			return err
		}
	}
	cleanups, err := api.store.MediaUploadSessionsForTempCleanup(mediaUploadSweepLimit)
	if err != nil {
		return err
	}
	for _, candidate := range cleanups {
		path, ok := api.mediaUploadPath(candidate.ID)
		if !ok {
			return errors.New("invalid persisted media upload id")
		}
		lock := api.mediaUploadLock(candidate.ID)
		lock.Lock()
		err := os.Remove(path)
		if errors.Is(err, os.ErrNotExist) {
			err = nil
		}
		if err == nil {
			_, err = api.store.MarkMediaUploadTempCleaned(
				candidate.ID, candidate.Revision, nowMS,
			)
		}
		lock.Unlock()
		if err != nil && !errors.Is(err, store.ErrMediaStateConflict) &&
			!errors.Is(err, store.ErrMediaNotFound) {
			return err
		}
	}
	return nil
}

func (api *onboardingAPI) removeAbandonedMediaUploadChunks(now time.Time, all bool) error {
	entries, err := os.ReadDir(api.mediaUploadDir)
	if err != nil {
		return err
	}
	cutoff := now.Add(-mediaUploadSessionLifetime)
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".chunk-") {
			continue
		}
		if !all {
			info, err := entry.Info()
			if err != nil {
				return err
			}
			if info.ModTime().After(cutoff) {
				continue
			}
		}
		if err := os.Remove(filepath.Join(api.mediaUploadDir, entry.Name())); err != nil &&
			!errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	return nil
}

func (api *onboardingAPI) mediaUploadLock(sessionID string) *sync.Mutex {
	digest := sha256.Sum256([]byte(sessionID))
	return &api.mediaUploadLocks[int(digest[0])%len(api.mediaUploadLocks)]
}

func (api *onboardingAPI) mediaUploadPath(sessionID string) (string, bool) {
	if !mediaUploadIDPattern.MatchString(sessionID) || api.mediaUploadDir == "" {
		return "", false
	}
	return filepath.Join(api.mediaUploadDir, sessionID+".part"), true
}

func singleRequestHeader(r *http.Request, name string) (string, bool) {
	values := r.Header.Values(name)
	if len(values) != 1 || values[0] == "" {
		return "", false
	}
	return values[0], true
}

func setMediaUploadHeaders(w http.ResponseWriter, session store.MediaUploadSession) {
	w.Header().Set("Upload-Offset", strconv.FormatInt(session.ReceivedSizeBytes, 10))
	w.Header().Set("Upload-Length", strconv.FormatInt(session.DeclaredSizeBytes, 10))
	w.Header().Set("Upload-Expires", time.UnixMilli(session.ExpiresAt).UTC().Format(time.RFC3339))
}

func mediaUploadBody(session store.MediaUploadSession, token string, reused bool) mediaUploadResponse {
	return mediaUploadResponse{
		UploadID: session.ID, MediaID: session.MediaID, UploadToken: token,
		Offset: session.ReceivedSizeBytes, Length: session.DeclaredSizeBytes,
		ExpiresAt: time.UnixMilli(session.ExpiresAt).UTC().Format(time.RFC3339),
		Status:    string(session.Status), Reused: reused,
	}
}

func writeMediaUploadState(w http.ResponseWriter, status int, session store.MediaUploadSession) {
	setMediaUploadHeaders(w, session)
	writeJSON(w, status, mediaUploadBody(session, "", false))
}

func (api *onboardingAPI) createMediaUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost || r.URL.Path != "/v1/media/uploads" || r.URL.RawQuery != "" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	idempotencyKey, ok := singleRequestHeader(r, "Idempotency-Key")
	if !ok || !mediaUploadIdempotencyKey.MatchString(idempotencyKey) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	var req struct {
		Kind               string `json:"kind"`
		Title              string `json:"title"`
		SizeBytes          int64  `json:"size_bytes"`
		RightsAcknowledged bool   `json:"rights_acknowledged"`
	}
	if !decodeBoundedJSON(w, r, 1024, &req) ||
		(req.Kind != string(store.MediaKindVoiceClip) && req.Kind != string(store.MediaKindAudioClip) &&
			req.Kind != string(store.MediaKindAudioTrack)) ||
		req.SizeBytes <= 0 || len(req.Title) > 512 || !utf8.ValidString(req.Title) {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if !req.RightsAcknowledged {
		apiError(w, http.StatusPreconditionRequired, errorContentPolicyAcceptance, 0)
		return
	}
	now := api.mediaUploadNow()
	if !api.mediaUploadStorageReady(w, now) {
		return
	}
	actor := r.Context().Value(actorRequestKey{}).(actorRequest)
	uploadQuota := api.mediaUploadQuota
	if req.Kind == string(store.MediaKindAudioTrack) {
		uploadQuota.MaxItemBytes = media.MaxTrackBytes
	}
	creation, err := api.store.CreateAuthorizedMediaUpload(
		actor.Context.ActorID, actor.Bearer,
		store.CreateMediaUploadParams{
			Media: store.CreateMediaItemParams{
				OwnerOrbitID: actor.Context.OrbitID,
				ActorID:      actor.Context.ActorID,
				Kind:         store.MediaKind(req.Kind), Source: store.MediaSourceApp,
				Title: req.Title, CreatedAt: now.UnixMilli(),
				ExpiresAt: now.Add(mediaClipRetention).UnixMilli(),
			},
			DeclaredSizeBytes: req.SizeBytes,
			SessionExpiresAt:  now.Add(mediaUploadSessionLifetime).UnixMilli(),
			IdempotencyKey:    idempotencyKey,
		},
		uploadQuota,
	)
	if api.mediaUploadCreationError(w, err) {
		return
	}
	w.Header().Set("Location", "/v1/media/uploads/"+creation.Session.ID)
	setMediaUploadHeaders(w, creation.Session)
	status := http.StatusCreated
	if creation.Reused {
		status = http.StatusOK
	}
	writeJSON(w, status, mediaUploadBody(
		creation.Session, creation.Token, creation.Reused,
	))
}

func (api *onboardingAPI) mediaUploadCreationError(w http.ResponseWriter, err error) bool {
	if err == nil {
		return false
	}
	var rate *store.MediaUploadRateLimitError
	switch {
	case errors.Is(err, store.ErrUnauthorized):
		apiError(w, http.StatusUnauthorized, errorUnauthorized, 0)
	case errors.Is(err, store.ErrInsufficientCapability), errors.Is(err, store.ErrOrbitDisabled):
		apiError(w, http.StatusForbidden, errorInsufficientCapability, 0)
	case errors.Is(err, store.ErrMediaUploadTooLarge):
		apiError(w, http.StatusRequestEntityTooLarge, errorUploadTooLarge, 0)
	case errors.As(err, &rate):
		apiError(w, http.StatusTooManyRequests, errorTooManyAttempts, rate.RetryAfter)
	case errors.Is(err, store.ErrMediaUploadConcurrent), errors.Is(err, store.ErrMediaUploadDailyBytes):
		apiError(w, http.StatusTooManyRequests, errorUploadQuota, 0)
	case errors.Is(err, store.ErrStreamQuotaExceeded):
		apiError(w, http.StatusTooManyRequests, errorUploadQuota, 0)
	case errors.Is(err, store.ErrMediaIdempotencyMismatch):
		apiError(w, http.StatusConflict, errorUploadStateConflict, 0)
	case errors.Is(err, store.ErrContentPolicyAcceptanceRequired):
		apiError(w, http.StatusPreconditionRequired, errorContentPolicyAcceptance, 0)
	case errors.Is(err, store.ErrMediaInvalid), errors.Is(err, store.ErrMediaOwnerInvalid):
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
	default:
		api.mediaUploadInternalError(w, "create media upload")
	}
	return true
}

func (api *onboardingAPI) writeMediaUpload(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut || r.URL.RawQuery != "" {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	defer r.Body.Close()
	sessionID := strings.TrimPrefix(r.URL.Path, "/v1/media/uploads/")
	if !mediaUploadIDPattern.MatchString(sessionID) || strings.Contains(sessionID, "/") {
		apiError(w, http.StatusUnauthorized, errorUploadCredential, 0)
		return
	}
	token, ok := bearerToken(r)
	if !ok {
		apiError(w, http.StatusUnauthorized, errorUploadCredential, 0)
		return
	}
	now := api.mediaUploadNow()
	session, err := api.store.AuthorizeMediaUploadSession(sessionID, token, now.UnixMilli())
	if err != nil {
		api.mediaUploadInternalError(w, "authorize media upload")
		return
	}
	if session == nil {
		apiError(w, http.StatusUnauthorized, errorUploadCredential, 0)
		return
	}
	if session.Status != store.UploadStatusOpen {
		lock := api.mediaUploadLock(sessionID)
		lock.Lock()
		defer lock.Unlock()
		session, err = api.store.AuthorizeMediaUploadSession(sessionID, token, now.UnixMilli())
		if err != nil {
			api.mediaUploadInternalError(w, "reauthorize finalized media upload")
			return
		}
		if session == nil {
			apiError(w, http.StatusUnauthorized, errorUploadCredential, 0)
			return
		}
		processed, ok := api.processFinalizingMediaUpload(w, r, session)
		if ok {
			writeMediaUploadState(w, http.StatusOK, *processed)
		}
		return
	}
	if !api.mediaUploadStorageReady(w, now) {
		return
	}
	offsetValue, ok := singleRequestHeader(r, "Upload-Offset")
	if !ok {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	offset, err := strconv.ParseInt(offsetValue, 10, 64)
	if err != nil || offset < 0 || strconv.FormatInt(offset, 10) != offsetValue {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	mediaType, _, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil || mediaType != "application/octet-stream" ||
		r.ContentLength < 0 || len(r.TransferEncoding) != 0 {
		apiError(w, http.StatusBadRequest, errorInvalidRequest, 0)
		return
	}
	setMediaUploadHeaders(w, *session)
	if offset != session.ReceivedSizeBytes {
		apiError(w, http.StatusConflict, errorUploadOffsetConflict, 0)
		return
	}
	remaining := session.DeclaredSizeBytes - session.ReceivedSizeBytes
	if r.ContentLength > remaining {
		apiError(w, http.StatusRequestEntityTooLarge, errorUploadTooLarge, 0)
		return
	}
	if r.ContentLength == 0 && remaining != 0 {
		apiError(w, http.StatusBadRequest, errorUploadLengthMismatch, 0)
		return
	}

	var staged *os.File
	if r.ContentLength > 0 {
		staged, err = os.CreateTemp(api.mediaUploadDir, ".chunk-*")
		if err != nil {
			api.mediaUploadInternalError(w, "stage media upload")
			return
		}
		stagedName := staged.Name()
		defer os.Remove(stagedName)
		defer staged.Close()
		if err := staged.Chmod(0o600); err != nil {
			api.mediaUploadInternalError(w, "secure staged media upload")
			return
		}
		written, copyErr := io.Copy(staged, io.LimitReader(r.Body, r.ContentLength+1))
		if written != r.ContentLength {
			apiError(w, http.StatusBadRequest, errorUploadLengthMismatch, 0)
			return
		}
		if copyErr != nil || staged.Sync() != nil {
			api.mediaUploadInternalError(w, "write staged media upload")
			return
		}
	}

	lock := api.mediaUploadLock(sessionID)
	lock.Lock()
	defer lock.Unlock()
	session, err = api.store.AuthorizeMediaUploadSession(sessionID, token, now.UnixMilli())
	if err != nil {
		api.mediaUploadInternalError(w, "reauthorize media upload")
		return
	}
	if session == nil {
		apiError(w, http.StatusUnauthorized, errorUploadCredential, 0)
		return
	}
	if session.Status != store.UploadStatusOpen {
		setMediaUploadHeaders(w, *session)
		apiError(w, http.StatusConflict, errorUploadOffsetConflict, 0)
		return
	}
	setMediaUploadHeaders(w, *session)
	if offset != session.ReceivedSizeBytes {
		apiError(w, http.StatusConflict, errorUploadOffsetConflict, 0)
		return
	}
	target, err := api.reconcileMediaUploadFile(*session)
	if errors.Is(err, errMediaUploadTempBehind) {
		_, _ = api.store.FailMediaUploadSession(
			session.ID, session.Revision, "upload_temp_missing", now.UnixMilli(),
		)
		api.mediaUploadInternalError(w, "reconcile media upload")
		return
	}
	if err != nil {
		api.mediaUploadInternalError(w, "open media upload")
		return
	}
	defer target.Close()

	if r.ContentLength > 0 {
		if _, err := staged.Seek(0, io.SeekStart); err != nil {
			api.mediaUploadInternalError(w, "rewind staged media upload")
			return
		}
		if _, err := target.Seek(offset, io.SeekStart); err != nil {
			api.mediaUploadInternalError(w, "position media upload")
			return
		}
		written, copyErr := io.CopyN(target, staged, r.ContentLength)
		if copyErr != nil || written != r.ContentLength || target.Sync() != nil {
			_ = target.Truncate(offset)
			_ = target.Sync()
			api.mediaUploadInternalError(w, "append media upload")
			return
		}
		advanced, err := api.store.AdvanceMediaUpload(
			session.ID, offset, r.ContentLength, now.UnixMilli(),
		)
		if err != nil {
			_ = target.Truncate(offset)
			_ = target.Sync()
			if errors.Is(err, store.ErrMediaStateConflict) {
				if current, lookupErr := api.store.GetMediaUploadSession(session.ID); lookupErr == nil && current != nil {
					setMediaUploadHeaders(w, *current)
					apiError(w, http.StatusConflict, errorUploadOffsetConflict, 0)
					return
				}
			}
			api.mediaUploadInternalError(w, "advance media upload")
			return
		}
		session = &advanced
	}
	if session.ReceivedSizeBytes == session.DeclaredSizeBytes {
		if err := target.Close(); err != nil {
			api.mediaUploadInternalError(w, "close media upload before processing")
			return
		}
		finalizing, err := api.store.BeginMediaUploadFinalization(
			session.ID, session.Revision, now.UnixMilli(),
		)
		if err != nil {
			if errors.Is(err, store.ErrMediaStateConflict) {
				if current, lookupErr := api.store.GetMediaUploadSession(session.ID); lookupErr == nil && current != nil &&
					(current.Status == store.UploadStatusFinalizing || current.Status == store.UploadStatusCompleted) {
					writeMediaUploadState(w, http.StatusOK, *current)
					return
				}
			}
			api.mediaUploadInternalError(w, "finalize media upload")
			return
		}
		session = &finalizing
	}
	processed, ok := api.processFinalizingMediaUpload(w, r, session)
	if !ok {
		return
	}
	writeMediaUploadState(w, http.StatusOK, *processed)
}

func (api *onboardingAPI) processFinalizingMediaUpload(
	w http.ResponseWriter,
	r *http.Request,
	session *store.MediaUploadSession,
) (*store.MediaUploadSession, bool) {
	if session.Status != store.UploadStatusFinalizing {
		return session, true
	}
	if api.mediaSubmitterInitErr != nil {
		api.mediaUploadInternalError(w, "initialize media processor")
		return nil, false
	}
	if api.mediaSubmitter != nil {
		_, err := api.mediaSubmitter.SubmitUpload(context.WithoutCancel(r.Context()), session.ID)
		if err != nil {
			if _, processing := media.FailureCode(err); processing {
				apiError(w, http.StatusUnprocessableEntity, errorMediaProcessing, 0)
				return nil, false
			}
			api.mediaUploadInternalError(w, "process media upload")
			return nil, false
		}
		completed, err := api.store.GetMediaUploadSession(session.ID)
		if err != nil || completed == nil {
			api.mediaUploadInternalError(w, "load completed media upload")
			return nil, false
		}
		if completed.Status != store.UploadStatusCompleted {
			api.mediaUploadInternalError(w, "verify completed media upload")
			return nil, false
		}
		session = completed
	}
	return session, true
}

func (api *onboardingAPI) reconcileMediaUploadFile(session store.MediaUploadSession) (*os.File, error) {
	path, ok := api.mediaUploadPath(session.ID)
	if !ok {
		return nil, errors.New("invalid media upload path identity")
	}
	target, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, err
	}
	if err := target.Chmod(0o600); err != nil {
		target.Close()
		return nil, err
	}
	info, err := target.Stat()
	if err != nil {
		target.Close()
		return nil, err
	}
	if info.Size() < session.ReceivedSizeBytes {
		target.Close()
		return nil, errMediaUploadTempBehind
	}
	if info.Size() > session.ReceivedSizeBytes {
		if err := target.Truncate(session.ReceivedSizeBytes); err != nil {
			target.Close()
			return nil, err
		}
		if err := target.Sync(); err != nil {
			target.Close()
			return nil, err
		}
	}
	return target, nil
}

func (api *onboardingAPI) mediaUploadInternalError(w http.ResponseWriter, operation string) {
	// File-system errors can contain absolute local paths. Upload failures log
	// only a fixed operation class; request headers, credentials, filenames,
	// titles, and idempotency material are deliberately excluded.
	api.log.Error(operation + " failed")
	apiError(w, http.StatusInternalServerError, errorInternal, 0)
}

func (api *onboardingAPI) runMediaUploadMaintenance(stop <-chan struct{}) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	var lifecycleWake <-chan struct{}
	if api.mediaLifecycle != nil && api.mediaLifecycleInitErr == nil {
		lifecycleWake = api.mediaLifecycle.Wake()
		api.runMediaLifecycleSweep()
	}
	for {
		select {
		case <-stop:
			return
		case <-lifecycleWake:
			api.runMediaLifecycleSweep()
		case now := <-ticker.C:
			api.mediaUploadMaintenance.Lock()
			var err error
			if api.mediaUploadInitErr != nil {
				err = api.initializeMediaUploadStorage()
			} else {
				err = api.maintainMediaUploadStorage(now)
			}
			api.mediaUploadInitErr = err
			api.mediaUploadMaintenance.Unlock()
			if err != nil {
				// File-system errors may carry absolute local paths.
				api.log.Error("scheduled media upload maintenance failed")
			}
			api.runMediaLifecycleSweep()
		}
	}
}

func (api *onboardingAPI) runMediaLifecycleSweep() {
	if api.mediaLifecycleInitErr != nil || api.mediaLifecycle == nil {
		return
	}
	if err := api.mediaLifecycle.Sweep(context.Background()); err != nil {
		// Lifecycle errors are intentionally sanitized at the service boundary.
		api.log.Error("scheduled media lifecycle sweep failed")
	}
}
