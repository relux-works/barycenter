package media

import (
	"context"
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"

	"relux.works/duet/coordinator/internal/store"
)

const maxTelegramVoiceBytes = int64(20 << 20)

// MediaSubmitter is the transport-neutral processing boundary shared by app
// uploads and Telegram intake.
type MediaSubmitter interface {
	SubmitMedia(context.Context, Submission) (store.MediaItem, error)
}

// TelegramVoiceDownloader is implemented by the bot transport. The adapter
// owns the destination path and all persistence/processing decisions.
type TelegramVoiceDownloader interface {
	DownloadVoiceBounded(fileID, destination string, maxBytes int64) (int64, error)
}

type TelegramVoice struct {
	OwnerOrbitID   int64
	TelegramUserID int64
	TelegramFileID string
	Title          string
	AcceptedAt     int64
	ExpiresAt      int64
}

type TelegramAcceptance struct {
	MediaID        string
	OwnerOrbitID   int64
	ActorID        int64
	TelegramFileID string
	AcceptedAt     int64
}

// TelegramAdapter is intentionally thin: acceptance is persisted by Store,
// source bytes are downloaded into private server-owned storage, and every
// supported file enters the same SubmitMedia service as an app upload.
type TelegramAdapter struct {
	store        *store.Store
	downloader   TelegramVoiceDownloader
	submitter    MediaSubmitter
	sourceDir    string
	canonicalDir string
	now          func() time.Time
	locks        [64]sync.Mutex
}

func NewTelegramAdapter(
	st *store.Store,
	mediaDir string,
	downloader TelegramVoiceDownloader,
	submitter MediaSubmitter,
) (*TelegramAdapter, error) {
	if st == nil || mediaDir == "" || downloader == nil || submitter == nil {
		return nil, errors.New("invalid Telegram media adapter configuration")
	}
	adapter := &TelegramAdapter{
		store:        st,
		downloader:   downloader,
		submitter:    submitter,
		sourceDir:    filepath.Join(mediaDir, ".telegram"),
		canonicalDir: filepath.Join(mediaDir, "canonical"),
		now:          time.Now,
	}
	if err := os.MkdirAll(adapter.sourceDir, 0o700); err != nil {
		return nil, errors.New("initialize Telegram media storage")
	}
	if err := os.Chmod(adapter.sourceDir, 0o700); err != nil {
		return nil, errors.New("secure Telegram media storage")
	}
	return adapter, nil
}

func (adapter *TelegramAdapter) Accept(voice TelegramVoice) (TelegramAcceptance, error) {
	created, err := adapter.store.CreateTelegramMedia(store.CreateTelegramMediaParams{
		OwnerOrbitID:   voice.OwnerOrbitID,
		TelegramUserID: voice.TelegramUserID,
		TelegramFileID: voice.TelegramFileID,
		Title:          voice.Title,
		CreatedAt:      voice.AcceptedAt,
		ExpiresAt:      voice.ExpiresAt,
	})
	if err != nil {
		return TelegramAcceptance{}, err
	}
	return TelegramAcceptance{
		MediaID:        created.Media.ID,
		OwnerOrbitID:   created.Media.OwnerOrbitID,
		ActorID:        created.Media.ActorID,
		TelegramFileID: voice.TelegramFileID,
		AcceptedAt:     created.Media.CreatedAt,
	}, nil
}

func (adapter *TelegramAdapter) Submit(
	ctx context.Context,
	accepted TelegramAcceptance,
) (Result, error) {
	if ctx == nil || !mediaItemIDPattern.MatchString(accepted.MediaID) ||
		accepted.OwnerOrbitID <= 0 || accepted.ActorID <= 0 ||
		accepted.TelegramFileID == "" || len(accepted.TelegramFileID) > 512 {
		return Result{}, processingError("media_request_invalid", nil)
	}
	lock := adapter.mediaLock(accepted.MediaID)
	lock.Lock()
	defer lock.Unlock()

	item, err := adapter.store.GetMediaItem(accepted.MediaID)
	if err != nil {
		return Result{}, errors.New("load Telegram media submission")
	}
	if item == nil || item.OwnerOrbitID != accepted.OwnerOrbitID ||
		item.ActorID != accepted.ActorID || item.Source != store.MediaSourceTelegram {
		return Result{}, processingError("media_unavailable", nil)
	}
	legacy, err := adapter.store.GetMedia(accepted.MediaID)
	if err != nil {
		return Result{}, errors.New("load legacy Telegram media submission")
	}
	if legacy == nil || legacy.OrbitID != accepted.OwnerOrbitID ||
		legacy.TGFileID != accepted.TelegramFileID {
		return Result{}, processingError("media_unavailable", nil)
	}
	switch item.Status {
	case store.MediaStatusReady:
		if legacy.Status != "processing" && legacy.Status != "ready" {
			return Result{}, processingError("media_state_invalid", nil)
		}
		return adapter.compatibilityResult(*item)
	case store.MediaStatusFailed:
		return Result{}, processingError(item.FailureCode, nil)
	case store.MediaStatusProcessing:
		if legacy.Status != "processing" {
			return Result{}, processingError("media_state_invalid", nil)
		}
	default:
		return Result{}, processingError("media_state_invalid", nil)
	}

	sourcePath := filepath.Join(adapter.sourceDir, accepted.MediaID+".source")
	if err := os.Remove(sourcePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return Result{}, errors.New("prepare Telegram media source")
	}
	downloaded, err := adapter.downloader.DownloadVoiceBounded(
		accepted.TelegramFileID, sourcePath, maxTelegramVoiceBytes,
	)
	if err != nil {
		adapter.secureRetainedSource(sourcePath)
		code := "media_input_unavailable"
		if downloaded > maxTelegramVoiceBytes {
			code = "media_input_oversized"
		}
		return Result{}, adapter.failProcessing(accepted.MediaID, code)
	}
	info, err := os.Lstat(sourcePath)
	if err != nil || !info.Mode().IsRegular() {
		_ = os.Remove(sourcePath)
		return Result{}, adapter.failProcessing(accepted.MediaID, "media_input_unavailable")
	}
	if err := os.Chmod(sourcePath, 0o600); err != nil {
		return Result{}, adapter.failProcessing(accepted.MediaID, "media_input_unavailable")
	}
	if downloaded != info.Size() {
		return Result{}, adapter.failProcessing(accepted.MediaID, "media_input_length_mismatch")
	}
	if info.Size() <= 0 || info.Size() > maxTelegramVoiceBytes {
		return Result{}, adapter.failProcessing(accepted.MediaID, "media_input_oversized")
	}

	ready, err := adapter.submitter.SubmitMedia(ctx, Submission{
		MediaID:      accepted.MediaID,
		SourcePath:   sourcePath,
		ExpectedSize: info.Size(),
	})
	if err != nil {
		code, ok := FailureCode(err)
		if !ok {
			code = "media_processing_unavailable"
		}
		return Result{}, adapter.failProcessing(accepted.MediaID, code)
	}
	if ready.ID != accepted.MediaID || ready.OwnerOrbitID != accepted.OwnerOrbitID ||
		ready.ActorID != accepted.ActorID || ready.Source != store.MediaSourceTelegram ||
		ready.Status != store.MediaStatusReady {
		return Result{}, processingError("media_state_invalid", nil)
	}
	return adapter.compatibilityResult(ready)
}

func (adapter *TelegramAdapter) compatibilityResult(item store.MediaItem) (Result, error) {
	path, ok := CanonicalPath(adapter.canonicalDir, item.StorageKey)
	if !ok {
		return Result{}, processingError("media_state_invalid", nil)
	}
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() != item.SizeBytes {
		return Result{}, processingError("media_input_unavailable", nil)
	}
	return Result{
		WAVPath:      path,
		DurationMS:   item.DurationMS,
		LoudnormJSON: item.LoudnessJSON,
		SizeBytes:    item.SizeBytes,
		SHA256:       item.SHA256,
	}, nil
}

// failProcessing makes transport/download/internal failures visible through
// the same generic status surface as ffprobe/ffmpeg failures. If SubmitMedia
// already committed a stable failure, its code remains authoritative.
func (adapter *TelegramAdapter) failProcessing(mediaID, fallbackCode string) error {
	item, err := adapter.store.GetMediaItem(mediaID)
	if err != nil || item == nil {
		return errors.New("load Telegram media failure state")
	}
	if item.Status == store.MediaStatusFailed {
		return processingError(item.FailureCode, nil)
	}
	if item.Status != store.MediaStatusProcessing {
		return processingError("media_state_invalid", nil)
	}
	now := adapter.now().UnixMilli()
	if now < item.UpdatedAt {
		now = item.UpdatedAt
	}
	if _, err := adapter.store.MarkMediaItemFailed(item.ID, item.Revision, fallbackCode, now); err != nil {
		if errors.Is(err, store.ErrMediaStateConflict) {
			current, lookupErr := adapter.store.GetMediaItem(mediaID)
			if lookupErr == nil && current != nil && current.Status == store.MediaStatusFailed {
				return processingError(current.FailureCode, nil)
			}
		}
		return errors.New("persist Telegram media failure")
	}
	return processingError(fallbackCode, nil)
}

func (adapter *TelegramAdapter) secureRetainedSource(path string) {
	info, err := os.Lstat(path)
	if err == nil && info.Mode().IsRegular() {
		_ = os.Chmod(path, 0o600)
	}
}

func (adapter *TelegramAdapter) mediaLock(mediaID string) *sync.Mutex {
	digest := sha256.Sum256([]byte(mediaID))
	return &adapter.locks[int(digest[0])%len(adapter.locks)]
}
