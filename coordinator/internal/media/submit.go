package media

import (
	"context"
	"crypto/sha256"
	"errors"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"relux.works/duet/coordinator/internal/store"
)

var (
	uploadSessionIDPattern = regexp.MustCompile(`^up_[0-9A-HJKMNP-TV-Z]{26}$`)
	mediaItemIDPattern     = regexp.MustCompile(`^m_[0-9A-HJKMNP-TV-Z]{26}$`)
	mediaStorageKeyPattern = regexp.MustCompile(`^media/v1/[0-9a-f]{64}$`)
)

type Submission struct {
	MediaID         string
	SourcePath      string
	ExpectedSize    int64
	UploadSessionID string
}

type SubmitService struct {
	store            *store.Store
	processor        *Processor
	preset           Preset
	uploadDir        string
	processingDir    string
	canonicalDir     string
	now              func() time.Time
	locks            [64]sync.Mutex
	workerSlots      chan struct{}
	testAfterPublish func() error
}

func NewSubmitService(st *store.Store, mediaDir string, preset Preset) (*SubmitService, error) {
	processor, err := NewProcessor()
	if err != nil {
		return nil, err
	}
	return newSubmitService(st, mediaDir, preset, processor)
}

func newSubmitService(st *store.Store, mediaDir string, preset Preset, processor *Processor) (*SubmitService, error) {
	if st == nil || mediaDir == "" || processor == nil ||
		processor.limits.WorkerConcurrency <= 0 || processor.limits.WorkerQueueTimeout <= 0 ||
		(preset != PresetDefault && preset != PresetRadio) {
		return nil, errors.New("invalid SubmitMedia configuration")
	}
	service := &SubmitService{
		store: st, processor: processor, preset: preset,
		uploadDir:     filepath.Join(mediaDir, ".uploads"),
		processingDir: filepath.Join(mediaDir, ".processing"),
		canonicalDir:  filepath.Join(mediaDir, "canonical"),
		now:           time.Now,
		workerSlots:   make(chan struct{}, processor.limits.WorkerConcurrency),
	}
	for _, directory := range []string{service.uploadDir, service.processingDir, service.canonicalDir} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return nil, errors.New("initialize SubmitMedia storage")
		}
		if err := os.Chmod(directory, 0o700); err != nil {
			return nil, errors.New("secure SubmitMedia storage")
		}
	}
	if err := service.removeRestartArtifacts(); err != nil {
		return nil, err
	}
	return service, nil
}

// A single coordinator owns mediaDir. Files with these private names are
// never published in the repository, so finding them during construction
// proves a prior worker stopped before its atomic link/CAS boundary.
func (service *SubmitService) removeRestartArtifacts() error {
	processing, err := os.ReadDir(service.processingDir)
	if err != nil {
		return errors.New("inspect SubmitMedia restart artifacts")
	}
	for _, entry := range processing {
		if err := os.RemoveAll(filepath.Join(service.processingDir, entry.Name())); err != nil {
			return errors.New("remove SubmitMedia restart artifacts")
		}
	}
	canonical, err := os.ReadDir(service.canonicalDir)
	if err != nil {
		return errors.New("inspect canonical restart artifacts")
	}
	for _, entry := range canonical {
		if !strings.HasPrefix(entry.Name(), ".canonical-") {
			continue
		}
		if err := os.RemoveAll(filepath.Join(service.canonicalDir, entry.Name())); err != nil {
			return errors.New("remove canonical restart artifacts")
		}
	}
	return nil
}

func (service *SubmitService) SubmitUpload(ctx context.Context, uploadSessionID string) (store.MediaItem, error) {
	if !uploadSessionIDPattern.MatchString(uploadSessionID) {
		return store.MediaItem{}, processingError("media_request_invalid", nil)
	}
	session, err := service.store.GetMediaUploadSession(uploadSessionID)
	if err != nil {
		return store.MediaItem{}, errors.New("load upload session")
	}
	if session == nil {
		return store.MediaItem{}, processingError("media_upload_unavailable", nil)
	}
	item, err := service.store.GetMediaItem(session.MediaID)
	if err != nil {
		return store.MediaItem{}, errors.New("load upload media")
	}
	if item == nil {
		return store.MediaItem{}, processingError("media_upload_unavailable", nil)
	}
	if session.Status == store.UploadStatusCompleted && item.Status == store.MediaStatusReady {
		service.cleanupSource(Submission{
			MediaID: item.ID, SourcePath: filepath.Join(service.uploadDir, uploadSessionID+".part"),
			ExpectedSize: session.DeclaredSizeBytes, UploadSessionID: uploadSessionID,
		})
		return *item, nil
	}
	if session.Status != store.UploadStatusFinalizing || item.Status != store.MediaStatusProcessing ||
		session.ReceivedSizeBytes != session.DeclaredSizeBytes {
		return store.MediaItem{}, processingError("media_upload_state_invalid", nil)
	}
	return service.SubmitMedia(ctx, Submission{
		MediaID:         item.ID,
		SourcePath:      filepath.Join(service.uploadDir, uploadSessionID+".part"),
		ExpectedSize:    session.DeclaredSizeBytes,
		UploadSessionID: uploadSessionID,
	})
}

// SubmitMedia is transport-neutral. The app upload wrapper above and the
// later Telegram adapter both enter here after persisting a processing item
// and placing source bytes in server-owned private storage.
func (service *SubmitService) SubmitMedia(ctx context.Context, submission Submission) (store.MediaItem, error) {
	if ctx == nil || !mediaItemIDPattern.MatchString(submission.MediaID) ||
		submission.SourcePath == "" || submission.ExpectedSize <= 0 ||
		(submission.UploadSessionID != "" && !uploadSessionIDPattern.MatchString(submission.UploadSessionID)) {
		return store.MediaItem{}, processingError("media_request_invalid", nil)
	}
	lock := service.mediaLock(submission.MediaID)
	lock.Lock()
	defer lock.Unlock()

	item, err := service.store.GetMediaItem(submission.MediaID)
	if err != nil {
		return store.MediaItem{}, errors.New("load media for submission")
	}
	if item == nil {
		return store.MediaItem{}, processingError("media_unavailable", nil)
	}
	if submission.UploadSessionID != "" {
		expectedPath := filepath.Join(service.uploadDir, submission.UploadSessionID+".part")
		session, sessionErr := service.store.GetMediaUploadSession(submission.UploadSessionID)
		if sessionErr != nil {
			return store.MediaItem{}, errors.New("load submission upload session")
		}
		if session == nil || session.MediaID != item.ID || session.DeclaredSizeBytes != submission.ExpectedSize ||
			filepath.Clean(submission.SourcePath) != filepath.Clean(expectedPath) ||
			(session.Status != store.UploadStatusFinalizing && session.Status != store.UploadStatusCompleted) {
			return store.MediaItem{}, processingError("media_upload_state_invalid", nil)
		}
	}
	if item.Status == store.MediaStatusReady {
		service.cleanupSource(submission)
		return *item, nil
	}
	if item.Status != store.MediaStatusProcessing ||
		(item.Kind != store.MediaKindVoiceClip && item.Kind != store.MediaKindAudioClip) {
		return store.MediaItem{}, processingError("media_state_invalid", nil)
	}

	workDir, err := os.MkdirTemp(service.processingDir, item.ID+"-")
	if err != nil {
		return store.MediaItem{}, errors.New("create SubmitMedia work directory")
	}
	defer os.RemoveAll(workDir)
	if err := os.Chmod(workDir, 0o700); err != nil {
		return store.MediaItem{}, errors.New("secure SubmitMedia work directory")
	}
	privateInput := filepath.Join(workDir, "input.bin")
	if err := copySubmissionInput(submission.SourcePath, privateInput, submission.ExpectedSize, service.processor.limits.MaxInputBytes); err != nil {
		return service.failProcessing(*item, err)
	}
	output, err := os.CreateTemp(service.canonicalDir, ".canonical-*.wav")
	if err != nil {
		return store.MediaItem{}, errors.New("create canonical staging file")
	}
	outputPath := output.Name()
	if err := output.Chmod(0o600); err != nil {
		output.Close()
		os.Remove(outputPath)
		return store.MediaItem{}, errors.New("secure canonical staging file")
	}
	if err := output.Close(); err != nil {
		os.Remove(outputPath)
		return store.MediaItem{}, errors.New("close canonical staging file")
	}
	defer os.Remove(outputPath)

	if err := service.acquireWorker(ctx); err != nil {
		return store.MediaItem{}, err
	}
	processed, err := func() (Result, error) {
		defer service.releaseWorker()
		return service.processor.Process(ctx, privateInput, outputPath, service.preset)
	}()
	if err != nil {
		return service.failProcessing(*item, err)
	}
	if err := os.Chmod(processed.WAVPath, 0o600); err != nil {
		return store.MediaItem{}, errors.New("secure canonical staging bytes")
	}
	if err := syncFile(processed.WAVPath); err != nil {
		return store.MediaItem{}, errors.New("sync canonical staging bytes")
	}
	operation, err := service.store.PendingMediaPublicationForMedia(item.ID)
	if err != nil {
		return store.MediaItem{}, errors.New("load pending media publication")
	}
	if operation == nil {
		operationValue, stageErr := service.store.StageMediaPublication(
			item.ID, item.Revision, service.now().UnixMilli(),
		)
		if stageErr != nil {
			if !errors.Is(stageErr, store.ErrMediaStateConflict) {
				return store.MediaItem{}, errors.New("stage media publication")
			}
			operation, err = service.store.PendingMediaPublicationForMedia(item.ID)
			if err != nil || operation == nil {
				return store.MediaItem{}, errors.New("recover concurrent media publication")
			}
		} else {
			operation = &operationValue
		}
	}
	if err := service.publishCanonical(*item, *operation, processed); err != nil {
		return store.MediaItem{}, err
	}
	if service.testAfterPublish != nil {
		if err := service.testAfterPublish(); err != nil {
			return store.MediaItem{}, err
		}
	}
	ready, err := service.store.CompleteMediaPublication(
		operation.ID, operation.Revision,
		store.MediaPublication{
			MIME: "audio/wav", Codec: "pcm_s16le",
			DurationMS: processed.DurationMS, SizeBytes: processed.SizeBytes,
			SHA256: processed.SHA256, LoudnessJSON: processed.LoudnormJSON,
		},
		service.now().UnixMilli(),
	)
	if err != nil {
		if errors.Is(err, store.ErrMediaStateConflict) {
			current, lookupErr := service.store.GetMediaItem(item.ID)
			if lookupErr == nil && current != nil && current.Status == store.MediaStatusReady {
				service.cleanupSource(submission)
				return *current, nil
			}
		}
		return store.MediaItem{}, errors.New("complete media publication")
	}
	service.cleanupSource(submission)
	return ready, nil
}

func (service *SubmitService) acquireWorker(ctx context.Context) error {
	if ctx.Err() != nil {
		return errors.New("media worker capacity wait cancelled")
	}
	timer := time.NewTimer(service.processor.limits.WorkerQueueTimeout)
	defer timer.Stop()
	select {
	case service.workerSlots <- struct{}{}:
		return nil
	case <-ctx.Done():
		return errors.New("media worker capacity wait cancelled")
	case <-timer.C:
		return errors.New("media worker capacity unavailable")
	}
}

func (service *SubmitService) releaseWorker() {
	<-service.workerSlots
}

func (service *SubmitService) failProcessing(item store.MediaItem, processingErr error) (store.MediaItem, error) {
	code, ok := FailureCode(processingErr)
	if !ok {
		return store.MediaItem{}, processingErr
	}
	failed, err := service.store.MarkMediaItemFailed(
		item.ID, item.Revision, code, service.now().UnixMilli(),
	)
	if err != nil {
		if errors.Is(err, store.ErrMediaStateConflict) {
			current, lookupErr := service.store.GetMediaItem(item.ID)
			if lookupErr == nil && current != nil && current.Status == store.MediaStatusFailed {
				return *current, processingErr
			}
		}
		return store.MediaItem{}, errors.New("persist media processing failure")
	}
	return failed, processingErr
}

func copySubmissionInput(sourcePath, targetPath string, expectedSize, limit int64) error {
	info, err := os.Lstat(sourcePath)
	if err != nil || !info.Mode().IsRegular() {
		return processingError("media_input_unavailable", err)
	}
	if info.Size() != expectedSize {
		return processingError("media_input_length_mismatch", nil)
	}
	if info.Size() <= 0 || info.Size() > limit {
		return processingError("media_input_oversized", nil)
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return processingError("media_input_unavailable", err)
	}
	defer source.Close()
	target, err := os.OpenFile(targetPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return errors.New("create private media input")
	}
	written, copyErr := io.Copy(target, io.LimitReader(source, limit+1))
	syncErr := target.Sync()
	closeErr := target.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil {
		return errors.New("copy private media input")
	}
	if written != expectedSize || written > limit {
		return processingError("media_input_length_mismatch", nil)
	}
	return nil
}

func (service *SubmitService) publishCanonical(item store.MediaItem, operation store.MediaStorageOperation, processed Result) error {
	finalPath, ok := CanonicalPath(service.canonicalDir, operation.StorageKey)
	if !ok {
		return errors.New("invalid canonical storage identity")
	}
	if existingHash, existingSize, err := hashFile(finalPath, service.processor.limits.MaxOutputBytes); err == nil {
		if existingHash != processed.SHA256 || existingSize != processed.SizeBytes {
			return errors.New("canonical storage collision")
		}
		if err := os.Chmod(finalPath, 0o600); err != nil {
			return errors.New("secure recovered canonical storage")
		}
		if err := syncFile(finalPath); err != nil {
			return errors.New("sync recovered canonical storage")
		}
		if err := syncDirectory(service.canonicalDir); err != nil {
			return errors.New("sync recovered canonical directory")
		}
		return nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return errors.New("inspect canonical storage")
	}
	sourcePath := processed.WAVPath
	dedupe, err := service.store.FindReadyMediaByCanonicalHash(
		item.OwnerOrbitID, processed.SHA256, item.ID,
	)
	if err != nil {
		return errors.New("query tenant media dedupe")
	}
	if dedupe != nil {
		if candidate, valid := CanonicalPath(service.canonicalDir, dedupe.StorageKey); valid {
			if candidateHash, candidateSize, hashErr := hashFile(candidate, service.processor.limits.MaxOutputBytes); hashErr == nil && candidateHash == processed.SHA256 && candidateSize == processed.SizeBytes {
				sourcePath = candidate
			}
		}
	}
	if err := os.Link(sourcePath, finalPath); err != nil {
		if existingHash, _, hashErr := hashFile(finalPath, service.processor.limits.MaxOutputBytes); hashErr != nil || existingHash != processed.SHA256 {
			return errors.New("publish canonical storage")
		}
	}
	if err := os.Chmod(finalPath, 0o600); err != nil {
		return errors.New("secure canonical storage")
	}
	if err := syncDirectory(service.canonicalDir); err != nil {
		return errors.New("sync canonical storage")
	}
	return nil
}

func (service *SubmitService) cleanupSource(submission Submission) {
	if err := os.Remove(submission.SourcePath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return
	}
	if submission.UploadSessionID == "" {
		return
	}
	session, err := service.store.GetMediaUploadSession(submission.UploadSessionID)
	if err != nil || session == nil || session.Status != store.UploadStatusCompleted || session.TempCleanedAt != 0 {
		return
	}
	_, _ = service.store.MarkMediaUploadTempCleaned(
		session.ID, session.Revision, service.now().UnixMilli(),
	)
}

func (service *SubmitService) mediaLock(mediaID string) *sync.Mutex {
	digest := sha256.Sum256([]byte(mediaID))
	return &service.locks[int(digest[0])%len(service.locks)]
}

func CanonicalPath(canonicalDir, storageKey string) (string, bool) {
	if canonicalDir == "" || !mediaStorageKeyPattern.MatchString(storageKey) {
		return "", false
	}
	name := strings.TrimPrefix(storageKey, "media/v1/") + ".wav"
	return filepath.Join(canonicalDir, name), true
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer directory.Close()
	return directory.Sync()
}

func syncFile(path string) error {
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	return file.Sync()
}
