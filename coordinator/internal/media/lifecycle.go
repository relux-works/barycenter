package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"relux.works/duet/coordinator/internal/store"
)

const (
	mediaLifecycleBatchLimit = 1000
	mediaAuditRetention      = 90 * 24 * time.Hour
)

// DeliveryCancellationSink is the forward-only ingest-to-transmission seam.
// Implementations must be idempotent by MediaID and MediaRevision: a crash can
// happen after CancelMedia applies its target changes but before the durable
// outbox acknowledgement commits.
type DeliveryCancellationSink interface {
	CancelMedia(context.Context, store.MediaDeliveryCancellation) error
}

type MediaLifecycleMetrics struct {
	Healthy               bool   `json:"healthy"`
	LastSweepUnixMS       int64  `json:"last_sweep_unix_ms"`
	SweepsTotal           uint64 `json:"sweeps_total"`
	SweepFailuresTotal    uint64 `json:"sweep_failures_total"`
	DeleteRequestsTotal   uint64 `json:"delete_requests_total"`
	MediaExpiredTotal     uint64 `json:"media_expired_total"`
	StorageCleanupsTotal  uint64 `json:"storage_cleanups_total"`
	StorageBytesTotal     uint64 `json:"storage_bytes_total"`
	StorageFailuresTotal  uint64 `json:"storage_failures_total"`
	StorageRefusalsTotal  uint64 `json:"storage_refusals_total"`
	LegacyCleanupsTotal   uint64 `json:"legacy_cleanups_total"`
	LegacyCleanupFailures uint64 `json:"legacy_cleanup_failures_total"`
	CancellationsTotal    uint64 `json:"cancellations_total"`
	CancellationFailures  uint64 `json:"cancellation_failures_total"`
	AuditEventsPruned     uint64 `json:"audit_events_pruned_total"`
	ExpirableMedia        int64  `json:"expirable_media"`
	PendingStorageCleanup int64  `json:"pending_storage_cleanup"`
	PendingCancellation   int64  `json:"pending_cancellation"`
	PendingTempCleanup    int64  `json:"pending_temp_cleanup"`
	PendingLegacyCleanup  int64  `json:"pending_legacy_cleanup"`
}

type lifecycleCounters struct {
	healthy               atomic.Bool
	lastSweepUnixMS       atomic.Int64
	sweepsTotal           atomic.Uint64
	sweepFailures         atomic.Uint64
	deleteRequests        atomic.Uint64
	mediaExpired          atomic.Uint64
	storageCleanups       atomic.Uint64
	storageBytes          atomic.Uint64
	storageFailures       atomic.Uint64
	storageRefusals       atomic.Uint64
	legacyCleanups        atomic.Uint64
	legacyCleanupFailures atomic.Uint64
	cancellations         atomic.Uint64
	cancellationFailures  atomic.Uint64
	auditEventsPruned     atomic.Uint64
	expirableMedia        atomic.Int64
	pendingStorageCleanup atomic.Int64
	pendingCancellation   atomic.Int64
	pendingTempCleanup    atomic.Int64
	pendingLegacyCleanup  atomic.Int64
}

type LifecycleService struct {
	store                 *store.Store
	canonicalDir          string
	canonicalDirReal      string
	telegramSourceDir     string
	telegramSourceDirReal string
	now                   func() time.Time
	wake                  chan struct{}

	sinkMu sync.RWMutex
	sink   DeliveryCancellationSink

	metrics lifecycleCounters

	// Tests inject a process interruption after unlink+directory fsync and
	// before the durable cleanup receipt. A retry must converge from ENOENT.
	testAfterStorageRemove func() error
	testAfterLegacyRemove  func() error
}

func NewLifecycleService(st *store.Store, mediaDir string) (*LifecycleService, error) {
	if st == nil || mediaDir == "" {
		return nil, errors.New("invalid media lifecycle configuration")
	}
	mediaDir, err := filepath.Abs(mediaDir)
	if err != nil {
		return nil, errors.New("resolve media lifecycle storage")
	}
	canonicalDir := filepath.Join(mediaDir, "canonical")
	telegramSourceDir := filepath.Join(mediaDir, ".telegram")
	for _, directory := range []string{canonicalDir, telegramSourceDir} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return nil, errors.New("initialize media lifecycle storage")
		}
		info, err := os.Lstat(directory)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("inspect media lifecycle storage")
		}
		if err := os.Chmod(directory, 0o700); err != nil {
			return nil, errors.New("secure media lifecycle storage")
		}
	}
	canonicalDirReal, err := filepath.EvalSymlinks(canonicalDir)
	if err != nil {
		return nil, errors.New("resolve real canonical lifecycle storage")
	}
	telegramSourceDirReal, err := filepath.EvalSymlinks(telegramSourceDir)
	if err != nil {
		return nil, errors.New("resolve real Telegram lifecycle storage")
	}
	service := &LifecycleService{
		store: st, canonicalDir: canonicalDir, canonicalDirReal: canonicalDirReal,
		telegramSourceDir: telegramSourceDir, telegramSourceDirReal: telegramSourceDirReal,
		now:  time.Now,
		wake: make(chan struct{}, 1),
	}
	service.metrics.healthy.Store(true)
	return service, nil
}

// SetDeliveryCancellationSink is called by the later transmission integration
// before its worker starts. Nil deliberately leaves durable requests pending;
// ingest never pretends a scheduler accepted work that does not exist yet.
func (service *LifecycleService) SetDeliveryCancellationSink(sink DeliveryCancellationSink) {
	service.sinkMu.Lock()
	service.sink = sink
	service.sinkMu.Unlock()
	service.signal()
}

func (service *LifecycleService) Wake() <-chan struct{} { return service.wake }

func (service *LifecycleService) signal() {
	select {
	case service.wake <- struct{}{}:
	default:
	}
}

func (service *LifecycleService) DeleteAuthorized(
	expectedActorID int64,
	bearer string,
	mediaID string,
) (store.MediaItem, error) {
	deleted, err := service.store.DeleteAuthorizedMedia(
		expectedActorID, bearer, mediaID, service.now().UnixMilli(),
	)
	if err != nil {
		return store.MediaItem{}, err
	}
	service.metrics.deleteRequests.Add(1)
	service.signal()
	return deleted, nil
}

func (service *LifecycleService) Metrics() MediaLifecycleMetrics {
	return MediaLifecycleMetrics{
		Healthy:               service.metrics.healthy.Load(),
		LastSweepUnixMS:       service.metrics.lastSweepUnixMS.Load(),
		SweepsTotal:           service.metrics.sweepsTotal.Load(),
		SweepFailuresTotal:    service.metrics.sweepFailures.Load(),
		DeleteRequestsTotal:   service.metrics.deleteRequests.Load(),
		MediaExpiredTotal:     service.metrics.mediaExpired.Load(),
		StorageCleanupsTotal:  service.metrics.storageCleanups.Load(),
		StorageBytesTotal:     service.metrics.storageBytes.Load(),
		StorageFailuresTotal:  service.metrics.storageFailures.Load(),
		StorageRefusalsTotal:  service.metrics.storageRefusals.Load(),
		LegacyCleanupsTotal:   service.metrics.legacyCleanups.Load(),
		LegacyCleanupFailures: service.metrics.legacyCleanupFailures.Load(),
		CancellationsTotal:    service.metrics.cancellations.Load(),
		CancellationFailures:  service.metrics.cancellationFailures.Load(),
		AuditEventsPruned:     service.metrics.auditEventsPruned.Load(),
		ExpirableMedia:        service.metrics.expirableMedia.Load(),
		PendingStorageCleanup: service.metrics.pendingStorageCleanup.Load(),
		PendingCancellation:   service.metrics.pendingCancellation.Load(),
		PendingTempCleanup:    service.metrics.pendingTempCleanup.Load(),
		PendingLegacyCleanup:  service.metrics.pendingLegacyCleanup.Load(),
	}
}

func (service *LifecycleService) Sweep(ctx context.Context) error {
	if ctx == nil {
		return errors.New("nil media lifecycle context")
	}
	service.metrics.sweepsTotal.Add(1)
	now := service.now().UnixMilli()
	var sweepErr error

	if err := service.expireMedia(ctx, now); err != nil {
		sweepErr = errors.Join(sweepErr, errors.New("expire media lifecycle rows"))
	}
	if err := service.cleanupStorage(ctx, now); err != nil {
		sweepErr = errors.Join(sweepErr, errors.New("clean media lifecycle storage"))
	}
	if err := service.cleanupLegacyMedia(ctx, now); err != nil {
		sweepErr = errors.Join(sweepErr, errors.New("clean legacy media compatibility storage"))
	}
	if err := service.deliverCancellations(ctx, now); err != nil {
		sweepErr = errors.Join(sweepErr, errors.New("deliver media lifecycle cancellations"))
	}
	pruned, err := service.store.PruneMediaIngestAudit(
		now-mediaAuditRetention.Milliseconds(), mediaLifecycleBatchLimit,
	)
	if err != nil {
		sweepErr = errors.Join(sweepErr, errors.New("prune media lifecycle audit"))
	} else if pruned > 0 {
		service.metrics.auditEventsPruned.Add(uint64(pruned))
	}
	backlog, err := service.store.MediaLifecycleBacklog(now)
	if err != nil {
		sweepErr = errors.Join(sweepErr, errors.New("measure media lifecycle backlog"))
	} else {
		service.metrics.expirableMedia.Store(backlog.ExpirableMedia)
		service.metrics.pendingStorageCleanup.Store(backlog.PendingStorageCleanup)
		service.metrics.pendingCancellation.Store(backlog.PendingCancellation)
		service.metrics.pendingTempCleanup.Store(backlog.PendingTempCleanup)
		service.metrics.pendingLegacyCleanup.Store(backlog.PendingLegacyCleanup)
	}
	service.metrics.lastSweepUnixMS.Store(now)
	if sweepErr != nil {
		service.metrics.sweepFailures.Add(1)
		service.metrics.healthy.Store(false)
	} else {
		service.metrics.healthy.Store(true)
	}
	return sweepErr
}

func (service *LifecycleService) expireMedia(ctx context.Context, now int64) error {
	items, err := service.store.ExpiredMediaItems(now, mediaLifecycleBatchLimit)
	if err != nil {
		return err
	}
	var result error
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return errors.Join(result, err)
		}
		if _, err := service.store.ExpireMediaItem(item.ID, item.Revision, now); err != nil {
			if !errors.Is(err, store.ErrMediaStateConflict) && !errors.Is(err, store.ErrMediaNotFound) {
				result = errors.Join(result, err)
			}
			continue
		}
		service.metrics.mediaExpired.Add(1)
	}
	return result
}

func (service *LifecycleService) cleanupStorage(ctx context.Context, now int64) error {
	operations, err := service.store.PendingMediaStorageOperations(
		store.StorageOperationCleanup, mediaLifecycleBatchLimit,
	)
	if err != nil {
		return err
	}
	var result error
	for _, operation := range operations {
		if err := ctx.Err(); err != nil {
			return errors.Join(result, err)
		}
		removedBytes, err := service.cleanupStorageOperation(operation, now)
		if err != nil {
			if errors.Is(err, store.ErrMediaStateConflict) {
				service.metrics.storageRefusals.Add(1)
			} else if !errors.Is(err, store.ErrMediaNotFound) {
				service.metrics.storageFailures.Add(1)
				result = errors.Join(result, err)
			}
			continue
		}
		service.metrics.storageCleanups.Add(1)
		if removedBytes > 0 {
			service.metrics.storageBytes.Add(uint64(removedBytes))
		}
	}
	return result
}

func (service *LifecycleService) cleanupStorageOperation(
	operation store.MediaStorageOperation,
	now int64,
) (int64, error) {
	lock := canonicalStorageLock(operation.StorageKey)
	lock.Lock()
	defer lock.Unlock()

	current, err := service.store.MediaStorageCleanupCandidate(operation.ID, operation.Revision)
	if err != nil {
		return 0, err
	}
	if err := service.validateCanonicalDirectory(); err != nil {
		return 0, err
	}
	path, ok := CanonicalPath(service.canonicalDir, current.StorageKey)
	if !ok {
		return 0, errors.New("invalid canonical cleanup identity")
	}
	var removedBytes int64
	info, err := os.Lstat(path)
	switch {
	case errors.Is(err, os.ErrNotExist):
	case err != nil:
		return 0, errors.New("inspect canonical cleanup target")
	case !info.Mode().IsRegular():
		return 0, errors.New("refuse non-regular canonical cleanup target")
	default:
		removedBytes = info.Size()
		if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
			return 0, errors.New("remove canonical cleanup target")
		}
	}
	// Sync even when Lstat already reports ENOENT. A previous process may have
	// crashed after unlink but before its directory fsync; the retry must make
	// that absence durable before acknowledging the cleanup outbox.
	if err := syncDirectory(service.canonicalDir); err != nil {
		return 0, errors.New("sync canonical cleanup directory")
	}
	if service.testAfterStorageRemove != nil {
		if err := service.testAfterStorageRemove(); err != nil {
			return 0, err
		}
	}
	if _, err := service.store.CompleteMediaStorageCleanup(
		current.ID, current.Revision, now,
	); err != nil {
		return 0, err
	}
	return removedBytes, nil
}

func validateManagedDirectory(directory, expectedReal string) error {
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errors.New("refuse replaced media cleanup directory")
	}
	real, err := filepath.EvalSymlinks(directory)
	if err != nil || real != expectedReal {
		return errors.New("refuse redirected media cleanup directory")
	}
	return nil
}

func (service *LifecycleService) validateCanonicalDirectory() error {
	return validateManagedDirectory(service.canonicalDir, service.canonicalDirReal)
}

func (service *LifecycleService) cleanupLegacyMedia(ctx context.Context, now int64) error {
	cleanups, err := service.store.PendingLegacyMediaCleanups(mediaLifecycleBatchLimit)
	if err != nil {
		return err
	}
	var result error
	for _, cleanup := range cleanups {
		if err := ctx.Err(); err != nil {
			return errors.Join(result, err)
		}
		if err := service.cleanupLegacyMediaItem(cleanup, now); err != nil {
			if !errors.Is(err, store.ErrMediaStateConflict) &&
				!errors.Is(err, store.ErrMediaNotFound) {
				service.metrics.legacyCleanupFailures.Add(1)
				result = errors.Join(result, err)
			}
			continue
		}
		service.metrics.legacyCleanups.Add(1)
	}
	return result
}

func (service *LifecycleService) cleanupLegacyMediaItem(
	selected store.LegacyMediaCleanup,
	now int64,
) error {
	current, err := service.store.LegacyMediaCleanupCandidate(
		selected.MediaID, selected.MediaRevision,
	)
	if err != nil {
		return err
	}
	paths := make([]string, 0, 2)
	if current.PathWAV != "" {
		path, err := managedLegacyPath(
			current.PathWAV, service.canonicalDir, service.canonicalDirReal,
		)
		if err != nil {
			return err
		}
		paths = append(paths, path)
	}
	if current.Source == store.MediaSourceTelegram {
		path, err := managedLegacyPath(
			filepath.Join(service.telegramSourceDir, current.MediaID+".source"),
			service.telegramSourceDir, service.telegramSourceDirReal,
		)
		if err != nil {
			return err
		}
		paths = append(paths, path)
	}

	directories := make(map[string]struct{})
	for _, path := range paths {
		info, err := os.Lstat(path)
		switch {
		case errors.Is(err, os.ErrNotExist):
		case err != nil:
			return errors.New("inspect legacy media cleanup target")
		case !info.Mode().IsRegular():
			return errors.New("refuse non-regular legacy media cleanup target")
		default:
			if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
				return errors.New("remove legacy media cleanup target")
			}
		}
		directories[filepath.Dir(path)] = struct{}{}
	}
	for directory := range directories {
		if err := syncDirectory(directory); err != nil {
			return errors.New("sync legacy media cleanup directory")
		}
	}
	if service.testAfterLegacyRemove != nil {
		if err := service.testAfterLegacyRemove(); err != nil {
			return err
		}
	}
	if err := service.store.CompleteLegacyMediaCleanup(
		current.MediaID, current.MediaRevision, now,
	); err != nil {
		return err
	}
	return nil
}

func managedLegacyPath(path, directory, expectedReal string) (string, error) {
	if err := validateManagedDirectory(directory, expectedReal); err != nil {
		return "", err
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", errors.New("resolve legacy media cleanup target")
	}
	clean := filepath.Clean(absolute)
	relative, err := filepath.Rel(directory, clean)
	if err != nil || relative == "." || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) || filepath.IsAbs(relative) {
		return "", errors.New("refuse unmanaged legacy media cleanup target")
	}
	realParent, err := filepath.EvalSymlinks(filepath.Dir(clean))
	if err != nil {
		return "", errors.New("resolve legacy media cleanup parent")
	}
	realRelative, err := filepath.Rel(expectedReal, realParent)
	if err != nil || realRelative == ".." ||
		strings.HasPrefix(realRelative, ".."+string(filepath.Separator)) || filepath.IsAbs(realRelative) {
		return "", errors.New("refuse redirected legacy media cleanup target")
	}
	return clean, nil
}

func (service *LifecycleService) deliverCancellations(ctx context.Context, now int64) error {
	service.sinkMu.RLock()
	sink := service.sink
	service.sinkMu.RUnlock()
	if sink == nil {
		return nil
	}
	cancellations, err := service.store.PendingMediaDeliveryCancellations(mediaLifecycleBatchLimit)
	if err != nil {
		return err
	}
	var result error
	for _, cancellation := range cancellations {
		if err := ctx.Err(); err != nil {
			return errors.Join(result, err)
		}
		if err := sink.CancelMedia(ctx, cancellation); err != nil {
			service.metrics.cancellationFailures.Add(1)
			result = errors.Join(result, err)
			continue
		}
		if _, err := service.store.CompleteMediaDeliveryCancellation(
			cancellation.MediaID, cancellation.Revision, now,
		); err != nil {
			if !errors.Is(err, store.ErrMediaStateConflict) {
				service.metrics.cancellationFailures.Add(1)
				result = errors.Join(result, err)
			}
			continue
		}
		service.metrics.cancellations.Add(1)
	}
	return result
}
