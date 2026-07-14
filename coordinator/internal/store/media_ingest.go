package store

import (
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"time"

	"relux.works/duet/coordinator/internal/ulid"
)

var (
	ErrMediaNotFound            = errors.New("media item was not found")
	ErrMediaStateConflict       = errors.New("media state changed")
	ErrMediaOwnerInvalid        = errors.New("media owner is not an active orbit member")
	ErrMediaIdempotencyMismatch = errors.New("media idempotency key was reused for a different request")
	ErrMediaInvalid             = errors.New("media input is invalid")
	ErrMediaUploadTooLarge      = errors.New("media upload exceeds the item byte limit")
	ErrMediaUploadRateLimited   = errors.New("media upload start rate limit reached")
	ErrMediaUploadConcurrent    = errors.New("media concurrent processing limit reached")
	ErrMediaUploadDailyBytes    = errors.New("media daily byte limit reached")
)

const (
	DefaultMediaUploadMaxStarts     = 10
	DefaultMediaUploadMaxConcurrent = 3
	DefaultMediaUploadMaxDailyBytes = int64(1 << 30)
	DefaultMediaUploadMaxItemBytes  = int64(50 << 20)
)

// MediaUploadQuota is evaluated in the same SQLite writer transaction that
// creates the processing media row. Reserved open/finalizing bytes count at
// their declared size so concurrent requests cannot oversubscribe the orbit.
type MediaUploadQuota struct {
	MaxStarts     int
	StartWindow   time.Duration
	MaxConcurrent int
	MaxDailyBytes int64
	DailyWindow   time.Duration
	MaxItemBytes  int64
}

func DefaultMediaUploadQuota() MediaUploadQuota {
	return MediaUploadQuota{
		MaxStarts:     DefaultMediaUploadMaxStarts,
		StartWindow:   time.Minute,
		MaxConcurrent: DefaultMediaUploadMaxConcurrent,
		MaxDailyBytes: DefaultMediaUploadMaxDailyBytes,
		DailyWindow:   24 * time.Hour,
		MaxItemBytes:  DefaultMediaUploadMaxItemBytes,
	}
}

// MediaUploadRateLimitError carries only the duration needed for Retry-After;
// no actor, orbit, request, or credential material crosses the repository.
type MediaUploadRateLimitError struct {
	RetryAfter time.Duration
}

func (e *MediaUploadRateLimitError) Error() string { return ErrMediaUploadRateLimited.Error() }
func (e *MediaUploadRateLimitError) Unwrap() error { return ErrMediaUploadRateLimited }

type MediaKind string

const (
	MediaKindVoiceClip  MediaKind = "voice_clip"
	MediaKindAudioClip  MediaKind = "audio_clip"
	MediaKindAudioTrack MediaKind = "audio_track"
	MediaKindBuiltinCue MediaKind = "builtin_cue"
)

type MediaSource string

const (
	MediaSourceApp      MediaSource = "app"
	MediaSourceTelegram MediaSource = "telegram"
	MediaSourceSystem   MediaSource = "system"
)

type MediaItemStatus string

const (
	MediaStatusProcessing MediaItemStatus = "processing"
	MediaStatusReady      MediaItemStatus = "ready"
	MediaStatusFailed     MediaItemStatus = "failed"
	MediaStatusDeleted    MediaItemStatus = "deleted"
	MediaStatusExpired    MediaItemStatus = "expired"
)

type MediaItem struct {
	ID           string
	OwnerOrbitID int64
	ActorID      int64
	Kind         MediaKind
	Source       MediaSource
	Title        string
	MIME         string
	Codec        string
	DurationMS   int64
	SizeBytes    int64
	SHA256       string
	StorageKey   string
	LoudnessJSON string
	Status       MediaItemStatus
	FailureCode  string
	Revision     int64
	CreatedAt    int64
	UpdatedAt    int64
	ExpiresAt    int64
	PublishedAt  int64
	DeletedAt    int64
}

type CreateMediaItemParams struct {
	OwnerOrbitID int64
	ActorID      int64
	Kind         MediaKind
	Source       MediaSource
	Title        string
	CreatedAt    int64
	ExpiresAt    int64
}

type UploadSessionStatus string

const (
	UploadStatusOpen       UploadSessionStatus = "open"
	UploadStatusFinalizing UploadSessionStatus = "finalizing"
	UploadStatusCompleted  UploadSessionStatus = "completed"
	UploadStatusFailed     UploadSessionStatus = "failed"
	UploadStatusExpired    UploadSessionStatus = "expired"
)

type MediaUploadSession struct {
	ID                string
	MediaID           string
	OwnerOrbitID      int64
	ActorID           int64
	DeclaredSizeBytes int64
	ReceivedSizeBytes int64
	Status            UploadSessionStatus
	Revision          int64
	CreatedAt         int64
	UpdatedAt         int64
	ExpiresAt         int64
	CompletedAt       int64
	TempCleanedAt     int64
}

type CreateMediaUploadParams struct {
	Media             CreateMediaItemParams
	DeclaredSizeBytes int64
	SessionExpiresAt  int64
	IdempotencyKey    string
}

// MediaUploadCreation never reflects persisted plaintext. CreateMediaUpload
// returns a random capability only for a new row; CreateAuthorizedMediaUpload
// derives the capability again after control authorization and can therefore
// return it on an idempotent replay without storing it.
type MediaUploadCreation struct {
	Media   MediaItem
	Session MediaUploadSession
	Token   string
	Reused  bool
}

type StorageOperationKind string

const (
	StorageOperationPublish StorageOperationKind = "publish"
	StorageOperationCleanup StorageOperationKind = "cleanup"
)

type StorageOperationState string

const (
	StorageOperationPending   StorageOperationState = "pending"
	StorageOperationDone      StorageOperationState = "done"
	StorageOperationCancelled StorageOperationState = "cancelled"
)

type MediaStorageOperation struct {
	ID            string
	MediaID       string
	Kind          StorageOperationKind
	StorageKey    string
	MediaRevision int64
	State         StorageOperationState
	Revision      int64
	CreatedAt     int64
	UpdatedAt     int64
	CompletedAt   int64
}

type MediaPublication struct {
	MIME         string
	Codec        string
	DurationMS   int64
	SizeBytes    int64
	SHA256       string
	LoudnessJSON string
}

type sqlScanner interface {
	Scan(dest ...any) error
}

var (
	mediaFailureCodePattern = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,63}$`)
	mediaMIMEPattern        = regexp.MustCompile(`^[a-z0-9][a-z0-9!#$&^_.+-]{0,63}/[a-z0-9][a-z0-9!#$&^_.+-]{0,63}$`)
	mediaCodecPattern       = regexp.MustCompile(`^[a-z0-9][a-z0-9_.-]{0,63}$`)
)

const mediaItemColumns = `id, owner_orbit_id, actor_id, kind, source, title,
mime, codec, duration_ms, size_bytes, sha256, storage_key, loudness_json,
status, failure_code, revision, created_at, updated_at, expires_at,
published_at, deleted_at`

const qualifiedMediaItemColumns = `i.id, i.owner_orbit_id, i.actor_id, i.kind,
i.source, i.title, i.mime, i.codec, i.duration_ms, i.size_bytes, i.sha256,
i.storage_key, i.loudness_json, i.status, i.failure_code, i.revision,
i.created_at, i.updated_at, i.expires_at, i.published_at, i.deleted_at`

const mediaUploadColumns = `id, media_id, owner_orbit_id, actor_id,
declared_size_bytes, received_size_bytes, status, revision, created_at,
updated_at, expires_at, completed_at, temp_cleaned_at`

const mediaStorageOperationColumns = `id, media_id, kind, storage_key,
media_revision, state, revision, created_at, updated_at, completed_at`

func scanMediaItem(row sqlScanner) (MediaItem, error) {
	var item MediaItem
	err := row.Scan(
		&item.ID, &item.OwnerOrbitID, &item.ActorID, &item.Kind, &item.Source,
		&item.Title, &item.MIME, &item.Codec, &item.DurationMS, &item.SizeBytes,
		&item.SHA256, &item.StorageKey, &item.LoudnessJSON, &item.Status,
		&item.FailureCode, &item.Revision, &item.CreatedAt, &item.UpdatedAt,
		&item.ExpiresAt, &item.PublishedAt, &item.DeletedAt,
	)
	return item, err
}

func scanMediaUpload(row sqlScanner) (MediaUploadSession, error) {
	var session MediaUploadSession
	err := row.Scan(
		&session.ID, &session.MediaID, &session.OwnerOrbitID, &session.ActorID,
		&session.DeclaredSizeBytes, &session.ReceivedSizeBytes, &session.Status,
		&session.Revision, &session.CreatedAt, &session.UpdatedAt,
		&session.ExpiresAt, &session.CompletedAt, &session.TempCleanedAt,
	)
	return session, err
}

func scanMediaStorageOperation(row sqlScanner) (MediaStorageOperation, error) {
	var operation MediaStorageOperation
	err := row.Scan(
		&operation.ID, &operation.MediaID, &operation.Kind, &operation.StorageKey,
		&operation.MediaRevision, &operation.State, &operation.Revision,
		&operation.CreatedAt, &operation.UpdatedAt, &operation.CompletedAt,
	)
	return operation, err
}

func validateCreateMediaItem(params CreateMediaItemParams) error {
	if params.OwnerOrbitID <= 0 || params.ActorID <= 0 {
		return fmt.Errorf("%w: positive owner and actor are required", ErrMediaInvalid)
	}
	switch params.Kind {
	case MediaKindVoiceClip, MediaKindAudioClip, MediaKindAudioTrack, MediaKindBuiltinCue:
	default:
		return fmt.Errorf("%w: unsupported media kind", ErrMediaInvalid)
	}
	switch params.Source {
	case MediaSourceApp, MediaSourceTelegram, MediaSourceSystem:
	default:
		return fmt.Errorf("%w: unsupported media source", ErrMediaInvalid)
	}
	if len(params.Title) > 512 {
		return fmt.Errorf("%w: title exceeds 512 bytes", ErrMediaInvalid)
	}
	if params.CreatedAt <= 0 || params.ExpiresAt <= params.CreatedAt {
		return fmt.Errorf("%w: invalid media retention window", ErrMediaInvalid)
	}
	return nil
}

func validateMediaOwnerTx(tx *sql.Tx, orbitID, actorID int64) error {
	var matches int
	err := tx.QueryRow(`SELECT COUNT(*)
FROM orbits o
JOIN memberships m ON m.orbit_id = o.id
JOIN actors a ON a.id = m.actor_id
WHERE o.id = ? AND o.status = 'active'
  AND m.actor_id = ? AND m.left_at IS NULL AND a.revoked_at IS NULL`,
		orbitID, actorID).Scan(&matches)
	if err != nil {
		return err
	}
	if matches != 1 {
		return ErrMediaOwnerInvalid
	}
	return nil
}

func insertMediaItemTx(tx *sql.Tx, params CreateMediaItemParams, id string) (MediaItem, error) {
	if _, err := tx.Exec(`INSERT INTO media_items(
  id, owner_orbit_id, actor_id, kind, source, title, status, revision,
  created_at, updated_at, expires_at
) VALUES(?, ?, ?, ?, ?, ?, 'processing', 1, ?, ?, ?)`,
		id, params.OwnerOrbitID, params.ActorID, params.Kind, params.Source,
		params.Title, params.CreatedAt, params.CreatedAt, params.ExpiresAt); err != nil {
		return MediaItem{}, err
	}
	if err := insertMediaAuditTx(tx, MediaItem{
		ID: id, OwnerOrbitID: params.OwnerOrbitID, ActorID: params.ActorID,
	}, "media.created", "", MediaStatusProcessing, params.CreatedAt); err != nil {
		return MediaItem{}, err
	}
	return scanMediaItem(tx.QueryRow(`SELECT `+mediaItemColumns+` FROM media_items WHERE id = ?`, id))
}

func insertMediaAuditTx(tx *sql.Tx, item MediaItem, eventType string, from, to MediaItemStatus, now int64) error {
	_, err := tx.Exec(`INSERT INTO media_ingest_audit_events(
  media_id, owner_orbit_id, actor_id, event_type, from_status, to_status, created_at
) VALUES(?, ?, ?, ?, ?, ?, ?)`,
		item.ID, item.OwnerOrbitID, item.ActorID, eventType, from, to, now)
	return err
}

func (s *Store) CreateMediaItem(params CreateMediaItemParams) (MediaItem, error) {
	if err := validateCreateMediaItem(params); err != nil {
		return MediaItem{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return MediaItem{}, err
	}
	defer tx.Rollback()
	if err := validateMediaOwnerTx(tx, params.OwnerOrbitID, params.ActorID); err != nil {
		return MediaItem{}, err
	}
	id := ulid.NewMediaID(time.UnixMilli(params.CreatedAt))
	item, err := insertMediaItemTx(tx, params, id)
	if err != nil {
		return MediaItem{}, err
	}
	if err := s.checkpoint("media_item_create_before_commit"); err != nil {
		return MediaItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaItem{}, err
	}
	return item, nil
}

func mediaUploadRequestFingerprint(params CreateMediaUploadParams) (string, error) {
	canonical := struct {
		OwnerOrbitID      int64       `json:"owner_orbit_id"`
		ActorID           int64       `json:"actor_id"`
		Kind              MediaKind   `json:"kind"`
		Source            MediaSource `json:"source"`
		Title             string      `json:"title"`
		DeclaredSizeBytes int64       `json:"declared_size_bytes"`
	}{
		params.Media.OwnerOrbitID, params.Media.ActorID, params.Media.Kind,
		params.Media.Source, params.Media.Title, params.DeclaredSizeBytes,
	}
	raw, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	return hashToken("barycenter/media-upload-request/v1:" + string(raw)), nil
}

func validateCreateMediaUpload(params CreateMediaUploadParams) error {
	if err := validateCreateMediaItem(params.Media); err != nil {
		return err
	}
	if params.DeclaredSizeBytes <= 0 || params.SessionExpiresAt <= params.Media.CreatedAt ||
		params.SessionExpiresAt > params.Media.ExpiresAt {
		return fmt.Errorf("%w: invalid upload size or expiry", ErrMediaInvalid)
	}
	if len(params.IdempotencyKey) < 8 || len(params.IdempotencyKey) > 512 {
		return fmt.Errorf("%w: invalid idempotency key length", ErrMediaInvalid)
	}
	return nil
}

func validateMediaUploadQuota(quota MediaUploadQuota) error {
	if quota.MaxStarts <= 0 || quota.StartWindow <= 0 ||
		quota.MaxConcurrent <= 0 || quota.MaxDailyBytes <= 0 ||
		quota.DailyWindow <= 0 || quota.MaxItemBytes <= 0 {
		return fmt.Errorf("%w: invalid media upload quota", ErrMediaInvalid)
	}
	return nil
}

func mediaUploadIdentity(params CreateMediaUploadParams) (string, string, error) {
	idempotencyHash := hashToken("barycenter/media-upload-idempotency/v1:" + params.IdempotencyKey)
	fingerprint, err := mediaUploadRequestFingerprint(params)
	if err != nil {
		return "", "", err
	}
	return idempotencyHash, fingerprint, nil
}

func mediaUploadCapability(controlToken, idempotencyHash string, orbitID, actorID int64) string {
	mac := hmac.New(sha256.New, []byte(controlToken))
	_, _ = fmt.Fprintf(mac, "barycenter/media-upload-capability/v1\n%d\n%d\n%s",
		orbitID, actorID, idempotencyHash)
	return hex.EncodeToString(mac.Sum(nil))
}

func findMediaUploadTx(tx *sql.Tx, params CreateMediaUploadParams, idempotencyHash, fingerprint string) (MediaUploadCreation, bool, error) {
	var existingID, existingFingerprint string
	err := tx.QueryRow(`SELECT id, request_fingerprint
FROM media_upload_sessions
WHERE owner_orbit_id = ? AND actor_id = ? AND idempotency_key_hash = ?`,
		params.Media.OwnerOrbitID, params.Media.ActorID, idempotencyHash).Scan(
		&existingID, &existingFingerprint)
	if errors.Is(err, sql.ErrNoRows) {
		return MediaUploadCreation{}, false, nil
	}
	if err != nil {
		return MediaUploadCreation{}, false, err
	}
	if existingFingerprint != fingerprint {
		return MediaUploadCreation{}, true, ErrMediaIdempotencyMismatch
	}
	session, err := scanMediaUpload(tx.QueryRow(
		`SELECT `+mediaUploadColumns+` FROM media_upload_sessions WHERE id = ?`, existingID))
	if err != nil {
		return MediaUploadCreation{}, true, err
	}
	item, err := scanMediaItem(tx.QueryRow(
		`SELECT `+mediaItemColumns+` FROM media_items WHERE id = ?`, session.MediaID))
	if err != nil {
		return MediaUploadCreation{}, true, err
	}
	return MediaUploadCreation{Media: item, Session: session, Reused: true}, true, nil
}

func insertMediaUploadTx(tx *sql.Tx, params CreateMediaUploadParams, idempotencyHash, fingerprint, token string) (MediaUploadCreation, error) {
	mediaID := ulid.NewMediaID(time.UnixMilli(params.Media.CreatedAt))
	item, err := insertMediaItemTx(tx, params.Media, mediaID)
	if err != nil {
		return MediaUploadCreation{}, err
	}
	sessionID := ulid.NewUploadSessionID(time.UnixMilli(params.Media.CreatedAt))
	if _, err := tx.Exec(`INSERT INTO media_upload_sessions(
  id, media_id, owner_orbit_id, actor_id, token_hash, idempotency_key_hash,
  request_fingerprint, declared_size_bytes, received_size_bytes, status,
  revision, created_at, updated_at, expires_at
) VALUES(?, ?, ?, ?, ?, ?, ?, ?, 0, 'open', 1, ?, ?, ?)`,
		sessionID, mediaID, params.Media.OwnerOrbitID, params.Media.ActorID,
		hashToken(token), idempotencyHash, fingerprint, params.DeclaredSizeBytes,
		params.Media.CreatedAt, params.Media.CreatedAt, params.SessionExpiresAt); err != nil {
		return MediaUploadCreation{}, err
	}
	if err := insertMediaAuditTx(tx, item, "media.upload_session_created", "", "", params.Media.CreatedAt); err != nil {
		return MediaUploadCreation{}, err
	}
	session, err := scanMediaUpload(tx.QueryRow(
		`SELECT `+mediaUploadColumns+` FROM media_upload_sessions WHERE id = ?`, sessionID))
	if err != nil {
		return MediaUploadCreation{}, err
	}
	return MediaUploadCreation{Media: item, Session: session, Token: token}, nil
}

func (s *Store) CreateMediaUpload(params CreateMediaUploadParams) (MediaUploadCreation, error) {
	if err := validateCreateMediaUpload(params); err != nil {
		return MediaUploadCreation{}, err
	}
	idempotencyHash, fingerprint, err := mediaUploadIdentity(params)
	if err != nil {
		return MediaUploadCreation{}, err
	}

	tx, err := s.db.Begin()
	if err != nil {
		return MediaUploadCreation{}, err
	}
	defer tx.Rollback()
	if err := validateMediaOwnerTx(tx, params.Media.OwnerOrbitID, params.Media.ActorID); err != nil {
		return MediaUploadCreation{}, err
	}
	if existing, found, err := findMediaUploadTx(tx, params, idempotencyHash, fingerprint); err != nil {
		return MediaUploadCreation{}, err
	} else if found {
		if err := tx.Commit(); err != nil {
			return MediaUploadCreation{}, err
		}
		return existing, nil
	}

	token, err := randomHexSecret(32)
	if err != nil {
		return MediaUploadCreation{}, err
	}
	created, err := insertMediaUploadTx(tx, params, idempotencyHash, fingerprint, token)
	if err != nil {
		return MediaUploadCreation{}, err
	}
	if err := s.checkpoint("media_upload_create_before_commit"); err != nil {
		return MediaUploadCreation{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaUploadCreation{}, err
	}
	return created, nil
}

func mediaUploadStartRateTx(tx *sql.Tx, orbitID, actorID, now int64, quota MediaUploadQuota) error {
	windowMS := quota.StartWindow.Milliseconds()
	cutoff := now - windowMS
	var actorCount, orbitCount int
	var actorOldest, orbitOldest sql.NullInt64
	if err := tx.QueryRow(`SELECT
  COALESCE(SUM(CASE WHEN actor_id = ? THEN 1 ELSE 0 END), 0),
  COUNT(*),
  MIN(CASE WHEN actor_id = ? THEN created_at END),
  MIN(created_at)
FROM media_upload_sessions
WHERE owner_orbit_id = ? AND created_at > ?`,
		actorID, actorID, orbitID, cutoff).Scan(
		&actorCount, &orbitCount, &actorOldest, &orbitOldest); err != nil {
		return err
	}
	if actorCount < quota.MaxStarts && orbitCount < quota.MaxStarts {
		return nil
	}
	oldest := orbitOldest.Int64
	if actorCount >= quota.MaxStarts && actorOldest.Valid &&
		(!orbitOldest.Valid || actorOldest.Int64 > oldest) {
		oldest = actorOldest.Int64
	}
	retryMS := oldest + windowMS - now
	if retryMS < 1 {
		retryMS = 1
	}
	return &MediaUploadRateLimitError{RetryAfter: time.Duration(retryMS) * time.Millisecond}
}

func mediaUploadCapacityTx(tx *sql.Tx, params CreateMediaUploadParams, quota MediaUploadQuota) error {
	if params.DeclaredSizeBytes > quota.MaxItemBytes {
		return ErrMediaUploadTooLarge
	}
	if err := mediaUploadStartRateTx(
		tx, params.Media.OwnerOrbitID, params.Media.ActorID, params.Media.CreatedAt, quota,
	); err != nil {
		return err
	}
	var actorConcurrent, orbitConcurrent int
	if err := tx.QueryRow(`SELECT
  COALESCE(SUM(CASE WHEN actor_id = ? THEN 1 ELSE 0 END), 0), COUNT(*)
FROM media_items
WHERE owner_orbit_id = ? AND status = 'processing'`,
		params.Media.ActorID, params.Media.OwnerOrbitID).Scan(
		&actorConcurrent, &orbitConcurrent); err != nil {
		return err
	}
	if actorConcurrent >= quota.MaxConcurrent || orbitConcurrent >= quota.MaxConcurrent {
		return ErrMediaUploadConcurrent
	}
	cutoff := params.Media.CreatedAt - quota.DailyWindow.Milliseconds()
	var reserved int64
	if err := tx.QueryRow(`SELECT COALESCE(SUM(
  CASE WHEN status IN ('open', 'finalizing')
    THEN declared_size_bytes ELSE received_size_bytes END
), 0)
FROM media_upload_sessions
WHERE owner_orbit_id = ? AND created_at > ?`,
		params.Media.OwnerOrbitID, cutoff).Scan(&reserved); err != nil {
		return err
	}
	if params.DeclaredSizeBytes > quota.MaxDailyBytes-reserved {
		return ErrMediaUploadDailyBytes
	}
	return nil
}

// CreateAuthorizedMediaUpload is the public-app creation boundary. It
// rechecks the presented control bearer inside the same writer transaction as
// idempotency, quota reservation, and row creation. The scoped capability is
// deterministically derived from that high-entropy bearer and the request's
// idempotency identity, allowing safe replay without storing plaintext.
func (s *Store) CreateAuthorizedMediaUpload(expectedActorID int64, bearer string, params CreateMediaUploadParams, quota MediaUploadQuota) (MediaUploadCreation, error) {
	if !s.selfServiceOnboarding {
		return MediaUploadCreation{}, ErrSelfServiceOnboardingDisabled
	}
	if expectedActorID <= 0 || !lowerHexTokenPattern.MatchString(bearer) {
		return MediaUploadCreation{}, ErrUnauthorized
	}
	if err := validateCreateMediaUpload(params); err != nil {
		return MediaUploadCreation{}, err
	}
	if err := validateMediaUploadQuota(quota); err != nil {
		return MediaUploadCreation{}, err
	}
	idempotencyHash, fingerprint, err := mediaUploadIdentity(params)
	if err != nil {
		return MediaUploadCreation{}, err
	}
	token := mediaUploadCapability(
		bearer, idempotencyHash, params.Media.OwnerOrbitID, params.Media.ActorID,
	)
	presentedHash := hashToken(bearer)

	tx, err := s.db.Begin()
	if err != nil {
		return MediaUploadCreation{}, err
	}
	defer tx.Rollback()
	ctx, err := mutationActorContextTx(tx, expectedActorID, presentedHash)
	if err != nil {
		return MediaUploadCreation{}, err
	}
	if ctx.ActorID != params.Media.ActorID || ctx.OrbitID != params.Media.OwnerOrbitID ||
		params.Media.Source != MediaSourceApp {
		return MediaUploadCreation{}, ErrUnauthorized
	}
	if existing, found, err := findMediaUploadTx(tx, params, idempotencyHash, fingerprint); err != nil {
		return MediaUploadCreation{}, err
	} else if found {
		if (existing.Session.Status == UploadStatusOpen || existing.Session.Status == UploadStatusFinalizing) &&
			existing.Session.ExpiresAt > params.Media.CreatedAt {
			result, err := tx.Exec(`UPDATE media_upload_sessions
SET token_hash = ?, revision = revision + 1, updated_at = ?
WHERE id = ? AND token_hash <> ?`, hashToken(token), params.Media.CreatedAt,
				existing.Session.ID, hashToken(token))
			if err != nil {
				return MediaUploadCreation{}, err
			}
			if changed, err := result.RowsAffected(); err != nil {
				return MediaUploadCreation{}, err
			} else if changed == 1 {
				existing.Session, err = scanMediaUpload(tx.QueryRow(
					`SELECT `+mediaUploadColumns+` FROM media_upload_sessions WHERE id = ?`, existing.Session.ID))
				if err != nil {
					return MediaUploadCreation{}, err
				}
			}
			existing.Token = token
		}
		if err := tx.Commit(); err != nil {
			return MediaUploadCreation{}, err
		}
		return existing, nil
	}
	if err := mediaUploadCapacityTx(tx, params, quota); err != nil {
		return MediaUploadCreation{}, err
	}
	created, err := insertMediaUploadTx(tx, params, idempotencyHash, fingerprint, token)
	if err != nil {
		return MediaUploadCreation{}, err
	}
	if err := s.checkpoint("media_upload_authorized_create_before_commit"); err != nil {
		return MediaUploadCreation{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaUploadCreation{}, err
	}
	return created, nil
}

func (s *Store) GetMediaItem(id string) (*MediaItem, error) {
	item, err := scanMediaItem(s.db.QueryRow(
		`SELECT `+mediaItemColumns+` FROM media_items WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}

func (s *Store) GetMediaUploadSession(id string) (*MediaUploadSession, error) {
	session, err := scanMediaUpload(s.db.QueryRow(
		`SELECT `+mediaUploadColumns+` FROM media_upload_sessions WHERE id = ?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (s *Store) GetMediaUploadSessionByToken(token string) (*MediaUploadSession, error) {
	if !lowerHexTokenPattern.MatchString(token) {
		return nil, nil
	}
	session, err := scanMediaUpload(s.db.QueryRow(
		`SELECT `+mediaUploadColumns+` FROM media_upload_sessions WHERE token_hash = ?`,
		hashToken(token)))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

// AuthorizeMediaUploadSession intentionally collapses unknown IDs, wrong
// capabilities, terminal states, and expired capabilities to a nil result so
// the HTTP layer has one non-disclosing credential response.
func (s *Store) AuthorizeMediaUploadSession(sessionID, token string, now int64) (*MediaUploadSession, error) {
	if sessionID == "" || !lowerHexTokenPattern.MatchString(token) || now <= 0 {
		return nil, nil
	}
	session, err := scanMediaUpload(s.db.QueryRow(
		`SELECT `+mediaUploadColumns+` FROM media_upload_sessions
WHERE id = ? AND token_hash = ? AND expires_at > ?
  AND status IN ('open', 'finalizing', 'completed')`,
		sessionID, hashToken(token), now))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &session, nil
}

func (s *Store) AdvanceMediaUpload(sessionID string, expectedOffset, bytesAdded, now int64) (MediaUploadSession, error) {
	if sessionID == "" || expectedOffset < 0 || bytesAdded <= 0 || now <= 0 {
		return MediaUploadSession{}, fmt.Errorf("%w: invalid upload advance", ErrMediaInvalid)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return MediaUploadSession{}, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE media_upload_sessions
SET received_size_bytes = received_size_bytes + ?, revision = revision + 1,
    updated_at = ?
WHERE id = ? AND status = 'open' AND expires_at > ?
  AND received_size_bytes = ?
  AND received_size_bytes + ? <= declared_size_bytes`,
		bytesAdded, now, sessionID, now, expectedOffset, bytesAdded)
	if err != nil {
		return MediaUploadSession{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return MediaUploadSession{}, err
		}
		return MediaUploadSession{}, ErrMediaStateConflict
	}
	session, err := scanMediaUpload(tx.QueryRow(
		`SELECT `+mediaUploadColumns+` FROM media_upload_sessions WHERE id = ?`, sessionID))
	if err != nil {
		return MediaUploadSession{}, err
	}
	if err := s.checkpoint("media_upload_advance_before_commit"); err != nil {
		return MediaUploadSession{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaUploadSession{}, err
	}
	return session, nil
}

func (s *Store) BeginMediaUploadFinalization(sessionID string, expectedRevision, now int64) (MediaUploadSession, error) {
	if sessionID == "" || expectedRevision <= 0 || now <= 0 {
		return MediaUploadSession{}, fmt.Errorf("%w: invalid finalize request", ErrMediaInvalid)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return MediaUploadSession{}, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE media_upload_sessions
SET status = 'finalizing', revision = revision + 1, updated_at = ?
WHERE id = ? AND status = 'open' AND revision = ? AND expires_at > ?
  AND received_size_bytes = declared_size_bytes`, now, sessionID, expectedRevision, now)
	if err != nil {
		return MediaUploadSession{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return MediaUploadSession{}, err
		}
		return MediaUploadSession{}, ErrMediaStateConflict
	}
	session, err := scanMediaUpload(tx.QueryRow(
		`SELECT `+mediaUploadColumns+` FROM media_upload_sessions WHERE id = ?`, sessionID))
	if err != nil {
		return MediaUploadSession{}, err
	}
	item, err := scanMediaItem(tx.QueryRow(
		`SELECT `+mediaItemColumns+` FROM media_items WHERE id = ?`, session.MediaID))
	if err != nil {
		return MediaUploadSession{}, err
	}
	if err := insertMediaAuditTx(tx, item, "media.upload_finalizing", "", "", now); err != nil {
		return MediaUploadSession{}, err
	}
	if err := s.checkpoint("media_upload_finalize_before_commit"); err != nil {
		return MediaUploadSession{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaUploadSession{}, err
	}
	return session, nil
}

func (s *Store) ExpiredMediaUploadSessions(now int64, limit int) ([]MediaUploadSession, error) {
	if now <= 0 || limit <= 0 || limit > 1000 {
		return nil, fmt.Errorf("%w: invalid expired upload query", ErrMediaInvalid)
	}
	rows, err := s.db.Query(`SELECT `+mediaUploadColumns+`
FROM media_upload_sessions
WHERE status IN ('open', 'finalizing') AND expires_at <= ?
ORDER BY expires_at, id
LIMIT ?`, now, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sessions := make([]MediaUploadSession, 0)
	for rows.Next() {
		session, err := scanMediaUpload(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func (s *Store) MediaUploadSessionsForTempCleanup(limit int) ([]MediaUploadSession, error) {
	if limit <= 0 || limit > 1000 {
		return nil, fmt.Errorf("%w: invalid upload cleanup query", ErrMediaInvalid)
	}
	rows, err := s.db.Query(`SELECT `+mediaUploadColumns+`
FROM media_upload_sessions
WHERE status IN ('failed', 'expired', 'completed') AND temp_cleaned_at = 0
ORDER BY updated_at, id
LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sessions := make([]MediaUploadSession, 0)
	for rows.Next() {
		session, err := scanMediaUpload(rows)
		if err != nil {
			return nil, err
		}
		sessions = append(sessions, session)
	}
	return sessions, rows.Err()
}

func (s *Store) MarkMediaUploadTempCleaned(sessionID string, expectedRevision, now int64) (MediaUploadSession, error) {
	if sessionID == "" || expectedRevision <= 0 || now <= 0 {
		return MediaUploadSession{}, fmt.Errorf("%w: invalid temp cleanup acknowledgement", ErrMediaInvalid)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return MediaUploadSession{}, err
	}
	defer tx.Rollback()
	result, err := tx.Exec(`UPDATE media_upload_sessions
SET temp_cleaned_at = ?, revision = revision + 1, updated_at = ?
WHERE id = ? AND revision = ? AND temp_cleaned_at = 0
  AND status IN ('failed', 'expired', 'completed')`,
		now, now, sessionID, expectedRevision)
	if err != nil {
		return MediaUploadSession{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return MediaUploadSession{}, err
		}
		return MediaUploadSession{}, ErrMediaStateConflict
	}
	cleaned, err := scanMediaUpload(tx.QueryRow(
		`SELECT `+mediaUploadColumns+` FROM media_upload_sessions WHERE id = ?`, sessionID))
	if err != nil {
		return MediaUploadSession{}, err
	}
	if err := s.checkpoint("media_upload_temp_cleaned_before_commit"); err != nil {
		return MediaUploadSession{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaUploadSession{}, err
	}
	return cleaned, nil
}

func (s *Store) ExpireMediaUploadSession(sessionID string, expectedRevision, now int64) (MediaUploadSession, error) {
	if sessionID == "" || expectedRevision <= 0 || now <= 0 {
		return MediaUploadSession{}, fmt.Errorf("%w: invalid upload expiry", ErrMediaInvalid)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return MediaUploadSession{}, err
	}
	defer tx.Rollback()
	session, err := scanMediaUpload(tx.QueryRow(
		`SELECT `+mediaUploadColumns+` FROM media_upload_sessions WHERE id = ?`, sessionID))
	if errors.Is(err, sql.ErrNoRows) {
		return MediaUploadSession{}, ErrMediaNotFound
	}
	if err != nil {
		return MediaUploadSession{}, err
	}
	if session.Revision != expectedRevision || session.ExpiresAt > now ||
		(session.Status != UploadStatusOpen && session.Status != UploadStatusFinalizing) {
		return MediaUploadSession{}, ErrMediaStateConflict
	}
	item, err := scanMediaItem(tx.QueryRow(
		`SELECT `+mediaItemColumns+` FROM media_items WHERE id = ?`, session.MediaID))
	if err != nil {
		return MediaUploadSession{}, err
	}
	if _, err := transitionMediaTerminalTx(
		tx, item.ID, item.Revision, MediaStatusFailed, "upload_expired", now, false,
	); err != nil {
		return MediaUploadSession{}, err
	}
	result, err := tx.Exec(`UPDATE media_upload_sessions
SET status = 'expired', revision = revision + 1, updated_at = ?
WHERE id = ? AND status = 'failed' AND revision = ?`,
		now, session.ID, expectedRevision+1)
	if err != nil {
		return MediaUploadSession{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return MediaUploadSession{}, err
		}
		return MediaUploadSession{}, ErrMediaStateConflict
	}
	expired, err := scanMediaUpload(tx.QueryRow(
		`SELECT `+mediaUploadColumns+` FROM media_upload_sessions WHERE id = ?`, session.ID))
	if err != nil {
		return MediaUploadSession{}, err
	}
	if err := s.checkpoint("media_upload_expire_before_commit"); err != nil {
		return MediaUploadSession{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaUploadSession{}, err
	}
	return expired, nil
}

func (s *Store) FailMediaUploadSession(sessionID string, expectedRevision int64, failureCode string, now int64) (MediaUploadSession, error) {
	if sessionID == "" || expectedRevision <= 0 || now <= 0 ||
		!mediaFailureCodePattern.MatchString(failureCode) {
		return MediaUploadSession{}, fmt.Errorf("%w: invalid upload failure", ErrMediaInvalid)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return MediaUploadSession{}, err
	}
	defer tx.Rollback()
	session, err := scanMediaUpload(tx.QueryRow(
		`SELECT `+mediaUploadColumns+` FROM media_upload_sessions WHERE id = ?`, sessionID))
	if errors.Is(err, sql.ErrNoRows) {
		return MediaUploadSession{}, ErrMediaNotFound
	}
	if err != nil {
		return MediaUploadSession{}, err
	}
	if session.Revision != expectedRevision || session.Status != UploadStatusOpen {
		return MediaUploadSession{}, ErrMediaStateConflict
	}
	item, err := scanMediaItem(tx.QueryRow(
		`SELECT `+mediaItemColumns+` FROM media_items WHERE id = ?`, session.MediaID))
	if err != nil {
		return MediaUploadSession{}, err
	}
	if _, err := transitionMediaTerminalTx(
		tx, item.ID, item.Revision, MediaStatusFailed, failureCode, now, false,
	); err != nil {
		return MediaUploadSession{}, err
	}
	failed, err := scanMediaUpload(tx.QueryRow(
		`SELECT `+mediaUploadColumns+` FROM media_upload_sessions WHERE id = ?`, session.ID))
	if err != nil {
		return MediaUploadSession{}, err
	}
	if err := s.checkpoint("media_upload_fail_before_commit"); err != nil {
		return MediaUploadSession{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaUploadSession{}, err
	}
	return failed, nil
}

func newStorageKey() (string, error) {
	random, err := randomHexSecret(32)
	if err != nil {
		return "", err
	}
	return "media/v1/" + random, nil
}

func (s *Store) StageMediaPublication(mediaID string, expectedRevision, now int64) (MediaStorageOperation, error) {
	if mediaID == "" || expectedRevision <= 0 || now <= 0 {
		return MediaStorageOperation{}, fmt.Errorf("%w: invalid publication request", ErrMediaInvalid)
	}
	storageKey, err := newStorageKey()
	if err != nil {
		return MediaStorageOperation{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return MediaStorageOperation{}, err
	}
	defer tx.Rollback()
	var sessionStatus UploadSessionStatus
	err = tx.QueryRow(`SELECT status FROM media_upload_sessions WHERE media_id = ?`, mediaID).Scan(&sessionStatus)
	if err == nil && sessionStatus != UploadStatusFinalizing {
		return MediaStorageOperation{}, ErrMediaStateConflict
	}
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return MediaStorageOperation{}, err
	}
	result, err := tx.Exec(`UPDATE media_items
SET revision = revision + 1, updated_at = ?
WHERE id = ? AND status = 'processing' AND revision = ? AND storage_key = ''`,
		now, mediaID, expectedRevision)
	if err != nil {
		return MediaStorageOperation{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return MediaStorageOperation{}, err
		}
		return MediaStorageOperation{}, ErrMediaStateConflict
	}
	operationID := ulid.NewStorageOperationID(time.UnixMilli(now))
	mediaRevision := expectedRevision + 1
	if _, err := tx.Exec(`INSERT INTO media_storage_operations(
  id, media_id, kind, storage_key, media_revision, state, revision,
  created_at, updated_at
) VALUES(?, ?, 'publish', ?, ?, 'pending', 1, ?, ?)`,
		operationID, mediaID, storageKey, mediaRevision, now, now); err != nil {
		return MediaStorageOperation{}, err
	}
	item, err := scanMediaItem(tx.QueryRow(
		`SELECT `+mediaItemColumns+` FROM media_items WHERE id = ?`, mediaID))
	if err != nil {
		return MediaStorageOperation{}, err
	}
	if err := insertMediaAuditTx(tx, item, "media.publication_staged", "", "", now); err != nil {
		return MediaStorageOperation{}, err
	}
	operation, err := scanMediaStorageOperation(tx.QueryRow(
		`SELECT `+mediaStorageOperationColumns+` FROM media_storage_operations WHERE id = ?`, operationID))
	if err != nil {
		return MediaStorageOperation{}, err
	}
	if err := s.checkpoint("media_publication_stage_before_commit"); err != nil {
		return MediaStorageOperation{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaStorageOperation{}, err
	}
	return operation, nil
}

func validateMediaPublication(publication MediaPublication) error {
	var loudness map[string]json.RawMessage
	if !mediaMIMEPattern.MatchString(publication.MIME) ||
		!mediaCodecPattern.MatchString(publication.Codec) ||
		publication.DurationMS < 0 || publication.SizeBytes <= 0 ||
		!lowerHexTokenPattern.MatchString(publication.SHA256) ||
		len(publication.LoudnessJSON) == 0 || len(publication.LoudnessJSON) > 16384 ||
		json.Unmarshal([]byte(publication.LoudnessJSON), &loudness) != nil || loudness == nil {
		return fmt.Errorf("%w: invalid canonical publication metadata", ErrMediaInvalid)
	}
	return nil
}

func (s *Store) CompleteMediaPublication(operationID string, expectedOperationRevision int64, publication MediaPublication, now int64) (MediaItem, error) {
	if operationID == "" || expectedOperationRevision <= 0 || now <= 0 {
		return MediaItem{}, fmt.Errorf("%w: invalid publication completion", ErrMediaInvalid)
	}
	if err := validateMediaPublication(publication); err != nil {
		return MediaItem{}, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return MediaItem{}, err
	}
	defer tx.Rollback()
	operation, err := scanMediaStorageOperation(tx.QueryRow(
		`SELECT `+mediaStorageOperationColumns+` FROM media_storage_operations WHERE id = ?`, operationID))
	if errors.Is(err, sql.ErrNoRows) {
		return MediaItem{}, ErrMediaNotFound
	}
	if err != nil {
		return MediaItem{}, err
	}
	if operation.Kind != StorageOperationPublish || operation.State != StorageOperationPending ||
		operation.Revision != expectedOperationRevision {
		return MediaItem{}, ErrMediaStateConflict
	}
	result, err := tx.Exec(`UPDATE media_items
SET mime = ?, codec = ?, duration_ms = ?, size_bytes = ?, sha256 = ?,
    storage_key = ?, loudness_json = ?, status = 'ready', failure_code = '',
    revision = revision + 1, updated_at = ?, published_at = ?
WHERE id = ? AND status = 'processing' AND revision = ? AND storage_key = ''`,
		publication.MIME, publication.Codec, publication.DurationMS,
		publication.SizeBytes, publication.SHA256, operation.StorageKey,
		publication.LoudnessJSON, now, now, operation.MediaID, operation.MediaRevision)
	if err != nil {
		return MediaItem{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return MediaItem{}, err
		}
		return MediaItem{}, ErrMediaStateConflict
	}
	var uploadStatus UploadSessionStatus
	err = tx.QueryRow(`SELECT status FROM media_upload_sessions WHERE media_id = ?`, operation.MediaID).Scan(&uploadStatus)
	if err == nil {
		if uploadStatus != UploadStatusFinalizing {
			return MediaItem{}, ErrMediaStateConflict
		}
		result, err = tx.Exec(`UPDATE media_upload_sessions
SET status = 'completed', revision = revision + 1, updated_at = ?, completed_at = ?
WHERE media_id = ? AND status = 'finalizing'`, now, now, operation.MediaID)
		if err != nil {
			return MediaItem{}, err
		}
		if changed, err := result.RowsAffected(); err != nil || changed != 1 {
			if err != nil {
				return MediaItem{}, err
			}
			return MediaItem{}, ErrMediaStateConflict
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return MediaItem{}, err
	}
	result, err = tx.Exec(`UPDATE media_storage_operations
SET state = 'done', revision = revision + 1, updated_at = ?, completed_at = ?
WHERE id = ? AND state = 'pending' AND revision = ?`,
		now, now, operation.ID, expectedOperationRevision)
	if err != nil {
		return MediaItem{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return MediaItem{}, err
		}
		return MediaItem{}, ErrMediaStateConflict
	}
	item, err := scanMediaItem(tx.QueryRow(
		`SELECT `+mediaItemColumns+` FROM media_items WHERE id = ?`, operation.MediaID))
	if err != nil {
		return MediaItem{}, err
	}
	if err := insertMediaAuditTx(tx, item, "media.ready", MediaStatusProcessing, MediaStatusReady, now); err != nil {
		return MediaItem{}, err
	}
	if err := s.checkpoint("media_publication_complete_before_commit"); err != nil {
		return MediaItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaItem{}, err
	}
	return item, nil
}

func storageKeysForTerminalTransitionTx(tx *sql.Tx, item MediaItem) ([]string, error) {
	seen := make(map[string]struct{})
	if item.StorageKey != "" {
		seen[item.StorageKey] = struct{}{}
	}
	rows, err := tx.Query(`SELECT storage_key FROM media_storage_operations
WHERE media_id = ? AND kind = 'publish' AND state = 'pending'`, item.ID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var key string
		if err := rows.Scan(&key); err != nil {
			rows.Close()
			return nil, err
		}
		seen[key] = struct{}{}
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(seen))
	for key := range seen {
		keys = append(keys, key)
	}
	return keys, nil
}

func scheduleStorageCleanupTx(tx *sql.Tx, mediaID, storageKey string, mediaRevision, now int64) error {
	operationID := ulid.NewStorageOperationID(time.UnixMilli(now))
	_, err := tx.Exec(`INSERT INTO media_storage_operations(
  id, media_id, kind, storage_key, media_revision, state, revision,
  created_at, updated_at
) VALUES(?, ?, 'cleanup', ?, ?, 'pending', 1, ?, ?)
ON CONFLICT(media_id, kind, storage_key) DO NOTHING`,
		operationID, mediaID, storageKey, mediaRevision, now, now)
	return err
}

func transitionMediaTerminalTx(tx *sql.Tx, mediaID string, expectedRevision int64, target MediaItemStatus, failureCode string, now int64, requireExpired bool) (MediaItem, error) {
	item, err := scanMediaItem(tx.QueryRow(
		`SELECT `+mediaItemColumns+` FROM media_items WHERE id = ?`, mediaID))
	if errors.Is(err, sql.ErrNoRows) {
		return MediaItem{}, ErrMediaNotFound
	}
	if err != nil {
		return MediaItem{}, err
	}
	if item.Revision != expectedRevision {
		return MediaItem{}, ErrMediaStateConflict
	}
	switch target {
	case MediaStatusFailed:
		if item.Status != MediaStatusProcessing || !mediaFailureCodePattern.MatchString(failureCode) {
			return MediaItem{}, ErrMediaStateConflict
		}
	case MediaStatusDeleted, MediaStatusExpired:
		if item.Status != MediaStatusProcessing && item.Status != MediaStatusReady && item.Status != MediaStatusFailed {
			return MediaItem{}, ErrMediaStateConflict
		}
		if requireExpired && item.ExpiresAt > now {
			return MediaItem{}, ErrMediaStateConflict
		}
		failureCode = ""
	default:
		return MediaItem{}, fmt.Errorf("%w: unsupported terminal status", ErrMediaInvalid)
	}
	keys, err := storageKeysForTerminalTransitionTx(tx, item)
	if err != nil {
		return MediaItem{}, err
	}
	deletedAt := int64(0)
	if target == MediaStatusDeleted || target == MediaStatusExpired {
		deletedAt = now
	}
	result, err := tx.Exec(`UPDATE media_items
SET status = ?, failure_code = ?, storage_key = '', revision = revision + 1,
    updated_at = ?, deleted_at = ?
WHERE id = ? AND revision = ?`,
		target, failureCode, now, deletedAt, mediaID, expectedRevision)
	if err != nil {
		return MediaItem{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return MediaItem{}, err
		}
		return MediaItem{}, ErrMediaStateConflict
	}
	newRevision := expectedRevision + 1
	if _, err := tx.Exec(`UPDATE media_storage_operations
SET state = 'cancelled', revision = revision + 1, updated_at = ?, completed_at = ?
WHERE media_id = ? AND kind = 'publish' AND state = 'pending'`, now, now, mediaID); err != nil {
		return MediaItem{}, err
	}
	for _, key := range keys {
		if err := scheduleStorageCleanupTx(tx, mediaID, key, newRevision, now); err != nil {
			return MediaItem{}, err
		}
	}
	uploadTarget := UploadStatusFailed
	if target == MediaStatusExpired {
		uploadTarget = UploadStatusExpired
	}
	if _, err := tx.Exec(`UPDATE media_upload_sessions
SET status = ?, revision = revision + 1, updated_at = ?
WHERE media_id = ? AND status IN ('open', 'finalizing')`, uploadTarget, now, mediaID); err != nil {
		return MediaItem{}, err
	}
	updated, err := scanMediaItem(tx.QueryRow(
		`SELECT `+mediaItemColumns+` FROM media_items WHERE id = ?`, mediaID))
	if err != nil {
		return MediaItem{}, err
	}
	eventType := "media." + string(target)
	if err := insertMediaAuditTx(tx, updated, eventType, item.Status, target, now); err != nil {
		return MediaItem{}, err
	}
	return updated, nil
}

// reconcileOrphanedMediaItems handles the one mutation an older coordinator
// can legitimately make without knowing the additive media model: dissolving
// an owning orbit. External orbit/actor foreign keys are intentionally absent
// so rollback stays operable; on roll-forward, active orphaned media is
// revoked and any canonical key is placed on the durable cleanup outbox.
func (s *Store) reconcileOrphanedMediaItems() error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	rows, err := tx.Query(`SELECT ` + qualifiedMediaItemColumns + `
FROM media_items i
WHERE i.status IN ('processing', 'ready', 'failed')
  AND NOT EXISTS (SELECT 1 FROM orbits o WHERE o.id = i.owner_orbit_id)
ORDER BY i.created_at, i.id`)
	if err != nil {
		return err
	}
	orphans := make([]MediaItem, 0)
	for rows.Next() {
		item, err := scanMediaItem(rows)
		if err != nil {
			rows.Close()
			return err
		}
		orphans = append(orphans, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	for _, orphan := range orphans {
		if _, err := transitionMediaTerminalTx(
			tx, orphan.ID, orphan.Revision, MediaStatusDeleted, "", now, false,
		); err != nil {
			return err
		}
	}
	if err := foreignKeyCheck(tx); err != nil {
		return err
	}
	if err := s.checkpoint("media_orphan_reconcile_before_commit"); err != nil {
		return err
	}
	return tx.Commit()
}

// revokeOrbitMediaTx is part of the current orbit-dissolution transaction.
// The separate roll-forward reconciler above covers the same mutation when it
// was performed by a predecessor that could not call this hook.
func revokeOrbitMediaTx(tx *sql.Tx, orbitID, now int64) error {
	rows, err := tx.Query(`SELECT `+qualifiedMediaItemColumns+`
FROM media_items i
WHERE i.owner_orbit_id = ? AND i.status IN ('processing', 'ready', 'failed')
ORDER BY i.created_at, i.id`, orbitID)
	if err != nil {
		return err
	}
	items := make([]MediaItem, 0)
	for rows.Next() {
		item, err := scanMediaItem(rows)
		if err != nil {
			rows.Close()
			return err
		}
		items = append(items, item)
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if err := rows.Err(); err != nil {
		return err
	}
	for _, item := range items {
		if _, err := transitionMediaTerminalTx(
			tx, item.ID, item.Revision, MediaStatusDeleted, "", now, false,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) MarkMediaItemFailed(mediaID string, expectedRevision int64, failureCode string, now int64) (MediaItem, error) {
	if mediaID == "" || expectedRevision <= 0 || now <= 0 || !mediaFailureCodePattern.MatchString(failureCode) {
		return MediaItem{}, fmt.Errorf("%w: invalid failure transition", ErrMediaInvalid)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return MediaItem{}, err
	}
	defer tx.Rollback()
	item, err := transitionMediaTerminalTx(tx, mediaID, expectedRevision, MediaStatusFailed, failureCode, now, false)
	if err != nil {
		return MediaItem{}, err
	}
	if err := s.checkpoint("media_failed_before_commit"); err != nil {
		return MediaItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaItem{}, err
	}
	return item, nil
}

func (s *Store) DeleteMediaItem(mediaID string, expectedRevision, now int64) (MediaItem, error) {
	if mediaID == "" || expectedRevision <= 0 || now <= 0 {
		return MediaItem{}, fmt.Errorf("%w: invalid delete transition", ErrMediaInvalid)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return MediaItem{}, err
	}
	defer tx.Rollback()
	item, err := transitionMediaTerminalTx(tx, mediaID, expectedRevision, MediaStatusDeleted, "", now, false)
	if err != nil {
		return MediaItem{}, err
	}
	if err := s.checkpoint("media_deleted_before_commit"); err != nil {
		return MediaItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaItem{}, err
	}
	return item, nil
}

func (s *Store) ExpireMediaItem(mediaID string, expectedRevision, now int64) (MediaItem, error) {
	if mediaID == "" || expectedRevision <= 0 || now <= 0 {
		return MediaItem{}, fmt.Errorf("%w: invalid expiry transition", ErrMediaInvalid)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return MediaItem{}, err
	}
	defer tx.Rollback()
	item, err := transitionMediaTerminalTx(tx, mediaID, expectedRevision, MediaStatusExpired, "", now, true)
	if err != nil {
		return MediaItem{}, err
	}
	if err := s.checkpoint("media_expired_before_commit"); err != nil {
		return MediaItem{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaItem{}, err
	}
	return item, nil
}

func (s *Store) PendingMediaStorageOperations(kind StorageOperationKind, limit int) ([]MediaStorageOperation, error) {
	if (kind != StorageOperationPublish && kind != StorageOperationCleanup) || limit <= 0 || limit > 1000 {
		return nil, fmt.Errorf("%w: invalid storage operation query", ErrMediaInvalid)
	}
	rows, err := s.db.Query(`SELECT `+mediaStorageOperationColumns+`
FROM media_storage_operations
WHERE kind = ? AND state = 'pending'
ORDER BY created_at, id
LIMIT ?`, kind, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	operations := make([]MediaStorageOperation, 0)
	for rows.Next() {
		operation, err := scanMediaStorageOperation(rows)
		if err != nil {
			return nil, err
		}
		operations = append(operations, operation)
	}
	return operations, rows.Err()
}

func (s *Store) CompleteMediaStorageCleanup(operationID string, expectedRevision, now int64) (MediaStorageOperation, error) {
	if operationID == "" || expectedRevision <= 0 || now <= 0 {
		return MediaStorageOperation{}, fmt.Errorf("%w: invalid cleanup completion", ErrMediaInvalid)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return MediaStorageOperation{}, err
	}
	defer tx.Rollback()
	operation, err := scanMediaStorageOperation(tx.QueryRow(
		`SELECT `+mediaStorageOperationColumns+` FROM media_storage_operations WHERE id = ?`, operationID))
	if errors.Is(err, sql.ErrNoRows) {
		return MediaStorageOperation{}, ErrMediaNotFound
	}
	if err != nil {
		return MediaStorageOperation{}, err
	}
	if operation.Kind != StorageOperationCleanup || operation.State != StorageOperationPending ||
		operation.Revision != expectedRevision {
		return MediaStorageOperation{}, ErrMediaStateConflict
	}
	result, err := tx.Exec(`UPDATE media_storage_operations
SET state = 'done', revision = revision + 1, updated_at = ?, completed_at = ?
WHERE id = ? AND kind = 'cleanup' AND state = 'pending' AND revision = ?`,
		now, now, operationID, expectedRevision)
	if err != nil {
		return MediaStorageOperation{}, err
	}
	if changed, err := result.RowsAffected(); err != nil || changed != 1 {
		if err != nil {
			return MediaStorageOperation{}, err
		}
		return MediaStorageOperation{}, ErrMediaStateConflict
	}
	completed, err := scanMediaStorageOperation(tx.QueryRow(
		`SELECT `+mediaStorageOperationColumns+` FROM media_storage_operations WHERE id = ?`, operationID))
	if err != nil {
		return MediaStorageOperation{}, err
	}
	item, err := scanMediaItem(tx.QueryRow(
		`SELECT `+mediaItemColumns+` FROM media_items WHERE id = ?`, operation.MediaID))
	if err != nil {
		return MediaStorageOperation{}, err
	}
	if err := insertMediaAuditTx(tx, item, "media.cleanup_completed", "", "", now); err != nil {
		return MediaStorageOperation{}, err
	}
	if err := s.checkpoint("media_cleanup_complete_before_commit"); err != nil {
		return MediaStorageOperation{}, err
	}
	if err := tx.Commit(); err != nil {
		return MediaStorageOperation{}, err
	}
	return completed, nil
}

func (s *Store) LinkLegacyWAV(mediaID string, expectedRevision int64, legacyMediaID string, now int64) error {
	if mediaID == "" || expectedRevision <= 0 || legacyMediaID == "" || now <= 0 {
		return fmt.Errorf("%w: invalid legacy media link", ErrMediaInvalid)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	item, err := scanMediaItem(tx.QueryRow(
		`SELECT `+mediaItemColumns+` FROM media_items WHERE id = ?`, mediaID))
	if errors.Is(err, sql.ErrNoRows) {
		return ErrMediaNotFound
	}
	if err != nil {
		return err
	}
	if item.Revision != expectedRevision ||
		(item.Status != MediaStatusProcessing && item.Status != MediaStatusReady) {
		return ErrMediaStateConflict
	}
	var legacyOrbit int64
	var legacyStatus string
	if err := tx.QueryRow(`SELECT orbit_id, status FROM media WHERE id = ?`, legacyMediaID).Scan(&legacyOrbit, &legacyStatus); errors.Is(err, sql.ErrNoRows) {
		return ErrMediaNotFound
	} else if err != nil {
		return err
	}
	if legacyStatus == "deleted" || (legacyOrbit != 0 && legacyOrbit != item.OwnerOrbitID) {
		return ErrMediaStateConflict
	}
	if legacyOrbit == 0 {
		if _, err := tx.Exec(`UPDATE media SET orbit_id = ? WHERE id = ? AND orbit_id = 0`, item.OwnerOrbitID, legacyMediaID); err != nil {
			return err
		}
	}
	var linkedMediaID string
	err = tx.QueryRow(`SELECT media_id FROM media_legacy_wav_links WHERE legacy_media_id = ?`, legacyMediaID).Scan(&linkedMediaID)
	if err == nil {
		if linkedMediaID != mediaID {
			return ErrMediaStateConflict
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	var linkedLegacyID string
	err = tx.QueryRow(`SELECT legacy_media_id FROM media_legacy_wav_links WHERE media_id = ?`, mediaID).Scan(&linkedLegacyID)
	if err == nil {
		if linkedLegacyID != legacyMediaID {
			return ErrMediaStateConflict
		}
		return tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	if _, err := tx.Exec(`INSERT INTO media_legacy_wav_links(media_id, legacy_media_id, linked_at)
VALUES(?, ?, ?)`, mediaID, legacyMediaID, now); err != nil {
		return err
	}
	if err := insertMediaAuditTx(tx, item, "media.legacy_wav_linked", "", "", now); err != nil {
		return err
	}
	if err := s.checkpoint("media_legacy_link_before_commit"); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Store) LegacyWAVForMediaItem(mediaID string) (*MediaRecord, error) {
	var media MediaRecord
	err := s.db.QueryRow(`SELECT m.id, m.tg_file_id, m.duration_ms, m.path_wav,
       m.loudnorm_json, m.created_at, m.expires_at, m.status, m.orbit_id
FROM media_legacy_wav_links l
JOIN media m ON m.id = l.legacy_media_id
WHERE l.media_id = ?`, mediaID).Scan(
		&media.ID, &media.TGFileID, &media.DurationMS, &media.PathWAV,
		&media.LoudnormJSON, &media.CreatedAt, &media.ExpiresAt, &media.Status,
		&media.OrbitID,
	)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &media, nil
}

func (s *Store) MediaItemForLegacyWAV(legacyMediaID string) (*MediaItem, error) {
	item, err := scanMediaItem(s.db.QueryRow(`SELECT `+qualifiedMediaItemColumns+`
FROM media_items i
JOIN media_legacy_wav_links l ON l.media_id = i.id
WHERE l.legacy_media_id = ?`, legacyMediaID))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &item, nil
}
