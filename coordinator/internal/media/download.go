package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

// MediaTargetSnapshotReader is the only node-read authorization seam for
// generic media. Its production implementation must query accepted, persisted
// transmission targets and must never infer access from current approach, Air,
// orbit membership, presence, history visibility, or knowledge of a URL.
type MediaTargetSnapshotReader interface {
	AllowsMediaDownload(context.Context, store.MediaTargetIdentity) (bool, error)
}

// MediaTargetAuthorizationReader is required for every non-Store target
// source. The implementation must keep the decision represented by allowed
// valid until authorized returns; the callback acquires the descriptor before
// returning. A Boolean-only reader is intentionally rejected by the setter.
type MediaTargetAuthorizationReader interface {
	MediaTargetSnapshotReader
	WithMediaDownloadAuthorization(
		context.Context,
		store.MediaTargetIdentity,
		func() error,
	) (allowed bool, err error)
}

type MediaDownload struct {
	Item store.MediaItem
	File *os.File
}

type StreamVariantDownload struct {
	Variant store.StreamVariant
	File    *os.File
}

type ModerationEvidenceDownload struct {
	Evidence store.ModerationEvidence
	File     *os.File
}

type DownloadService struct {
	store        *store.Store
	canonicalDir string
	streamDir    string
	now          func() time.Time

	targetsMu sync.RWMutex
	targets   MediaTargetSnapshotReader
	lease     MediaTargetAuthorizationReader
	// persistedTargets is true only when the reader is this service's Store.
	// That path rechecks the target/block decision inside both authorization
	// transactions instead of trusting the preflight Boolean through open(2).
	persistedTargets bool

	// Tests pause after the first authorization and before the canonical-key
	// lock to prove that the second live-state check closes delete races.
	testAfterAuthorization func()
	// Tests pause inside the second immediate authorization transaction and
	// before opening bytes to prove that revocation cannot commit in that gap.
	testBeforeOpen func()
}

func NewDownloadService(st *store.Store, mediaDir string) (*DownloadService, error) {
	if st == nil || mediaDir == "" {
		return nil, errors.New("invalid media download configuration")
	}
	canonicalDir, streamDir := filepath.Join(mediaDir, "canonical"), filepath.Join(mediaDir, "stream")
	for _, directory := range []string{canonicalDir, streamDir} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return nil, errors.New("initialize media download storage")
		}
		if err := os.Chmod(directory, 0o700); err != nil {
			return nil, errors.New("secure media download storage")
		}
	}
	return &DownloadService{
		store: st, canonicalDir: canonicalDir, streamDir: streamDir, now: time.Now,
	}, nil
}

// SetTargetSnapshotReader is the forward-only handoff to transmission
// persistence. The exact Store backing this service uses its immediate
// transaction path. A non-Store reader is accepted only when it implements the
// authorization-lease contract above; a Boolean-only reader is fail-closed.
func (service *DownloadService) SetTargetSnapshotReader(reader MediaTargetSnapshotReader) bool {
	service.targetsMu.Lock()
	persistedStore, ok := reader.(*store.Store)
	persisted := ok && persistedStore == service.store
	lease, leased := reader.(MediaTargetAuthorizationReader)
	trusted := persisted || leased
	if persisted {
		service.targets = persistedStore
		service.lease = nil
	} else if leased {
		service.targets = reader
		service.lease = lease
	} else {
		service.targets = nil
		service.lease = nil
	}
	service.persistedTargets = persisted
	service.targetsMu.Unlock()
	return trusted
}

func (service *DownloadService) OpenAuthorized(
	ctx context.Context,
	principal store.ActorContext,
	bearer string,
	mediaID string,
) (MediaDownload, error) {
	if ctx == nil {
		return MediaDownload{}, errors.New("nil media download context")
	}
	targetAuthorized := false
	var targetIdentity store.MediaTargetIdentity
	persistedTargets := false
	var lease MediaTargetAuthorizationReader
	if principal.Capabilities.Has(store.CapabilityNode) &&
		!principal.Capabilities.Has(store.CapabilityControl) {
		service.targetsMu.RLock()
		reader := service.targets
		persistedTargets = service.persistedTargets
		lease = service.lease
		service.targetsMu.RUnlock()
		if reader != nil {
			var err error
			targetIdentity = store.MediaTargetIdentity{
				MediaID: mediaID,
				OrbitID: principal.OrbitID,
				ActorID: principal.ActorID,
				Slot:    principal.Slot,
			}
			targetAuthorized, err = reader.AllowsMediaDownload(ctx, targetIdentity)
			if err != nil {
				return MediaDownload{}, errors.New("query media target snapshot")
			}
		}
	}
	var item store.MediaItem
	var err error
	if persistedTargets && targetAuthorized {
		item, err = service.store.AuthorizePersistedMediaDownload(
			principal, bearer, targetIdentity, service.now().UnixMilli(),
		)
	} else {
		item, err = service.store.AuthorizeMediaDownload(
			principal, bearer, mediaID, targetAuthorized, service.now().UnixMilli(),
		)
	}
	if err != nil {
		return MediaDownload{}, err
	}
	if service.testAfterAuthorization != nil {
		service.testAfterAuthorization()
	}

	lock := canonicalStorageLock(item.StorageKey)
	lock.Lock()
	defer lock.Unlock()

	// Deletion can commit after the first authorization and while this request
	// waits behind publication or cleanup. Recheck live credentials and media
	// state under the canonical-key lock, and keep the immediate store
	// transaction open until the descriptor has been acquired. This makes a
	// concurrent delete, target block/receipt, or actor revocation linearize on
	// one side of open(2).
	var file *os.File
	authorizedOpen := func(current store.MediaItem) error {
		if current.ID != item.ID || current.Revision != item.Revision ||
			current.StorageKey != item.StorageKey || current.SHA256 != item.SHA256 ||
			current.SizeBytes != item.SizeBytes {
			return store.ErrMediaNotFound
		}
		if service.testBeforeOpen != nil {
			service.testBeforeOpen()
		}
		path, ok := CanonicalPath(service.canonicalDir, current.StorageKey)
		if !ok {
			return errors.New("invalid media download storage identity")
		}
		before, err := os.Lstat(path)
		if err != nil || !before.Mode().IsRegular() {
			return errors.New("inspect media download storage")
		}
		opened, err := os.Open(path)
		if err != nil {
			return errors.New("open media download storage")
		}
		after, err := opened.Stat()
		if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) ||
			after.Size() != current.SizeBytes {
			opened.Close()
			return errors.New("verify media download storage")
		}
		file = opened
		return nil
	}
	var confirmed store.MediaItem
	if persistedTargets && targetAuthorized {
		confirmed, err = service.store.WithAuthorizedPersistedMediaDownload(
			principal, bearer, targetIdentity, service.now().UnixMilli(), authorizedOpen,
		)
	} else if targetAuthorized && lease != nil {
		allowed := false
		allowed, err = lease.WithMediaDownloadAuthorization(ctx, targetIdentity, func() error {
			var callbackErr error
			confirmed, callbackErr = service.store.WithAuthorizedMediaDownload(
				principal, bearer, mediaID, true, service.now().UnixMilli(), authorizedOpen,
			)
			return callbackErr
		})
		if err == nil && !allowed {
			err = store.ErrMediaNotFound
		}
	} else {
		confirmed, err = service.store.WithAuthorizedMediaDownload(
			principal, bearer, mediaID, targetAuthorized,
			service.now().UnixMilli(), authorizedOpen,
		)
	}
	if err != nil {
		if file != nil {
			file.Close()
		}
		return MediaDownload{}, err
	}
	if file == nil {
		return MediaDownload{}, errors.New("media download descriptor not acquired")
	}
	return MediaDownload{Item: confirmed, File: file}, nil
}

func (service *DownloadService) OpenAuthorizedStreamVariant(
	ctx context.Context,
	principal store.ActorContext,
	bearer, mediaID, variantID string,
) (StreamVariantDownload, error) {
	if ctx == nil {
		return StreamVariantDownload{}, errors.New("nil stream download context")
	}
	if !principal.Capabilities.Has(store.CapabilityNode) ||
		principal.Capabilities.Has(store.CapabilityControl) {
		return StreamVariantDownload{}, store.ErrStreamTrackNotFound
	}
	service.targetsMu.RLock()
	reader, persistedTargets, lease := service.targets, service.persistedTargets, service.lease
	service.targetsMu.RUnlock()
	target := store.MediaTargetIdentity{
		MediaID: mediaID, OrbitID: principal.OrbitID,
		ActorID: principal.ActorID, Slot: principal.Slot,
	}
	targetAuthorized := false
	if reader != nil {
		var err error
		targetAuthorized, err = reader.AllowsMediaDownload(ctx, target)
		if err != nil {
			return StreamVariantDownload{}, errors.New("query stream target snapshot")
		}
	}

	var file *os.File
	authorizedOpen := func(variant store.StreamVariant) error {
		lock := canonicalStorageLock(variant.StorageKey)
		lock.Lock()
		defer lock.Unlock()
		path, ok := StreamVariantPath(service.streamDir, variant.StorageKey)
		if !ok {
			return errors.New("invalid stream variant storage identity")
		}
		before, err := os.Lstat(path)
		if err != nil || !before.Mode().IsRegular() || before.Size() != variant.SizeBytes {
			return errors.New("inspect stream variant storage")
		}
		opened, err := os.Open(path)
		if err != nil {
			return errors.New("open stream variant storage")
		}
		after, err := opened.Stat()
		if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) ||
			after.Size() != variant.SizeBytes {
			opened.Close()
			return errors.New("verify stream variant storage")
		}
		file = opened
		return nil
	}
	var variant store.StreamVariant
	var err error
	if persistedTargets && targetAuthorized {
		variant, err = service.store.WithAuthorizedPersistedStreamVariantDownload(
			principal, bearer, target, variantID,
			service.now().UnixMilli(), authorizedOpen,
		)
	} else if targetAuthorized && lease != nil {
		allowed := false
		allowed, err = lease.WithMediaDownloadAuthorization(ctx, target, func() error {
			var callbackErr error
			variant, callbackErr = service.store.WithAuthorizedStreamVariantDownload(
				principal, bearer, mediaID, variantID, true,
				service.now().UnixMilli(), authorizedOpen,
			)
			return callbackErr
		})
		if err == nil && !allowed {
			err = store.ErrStreamTrackNotFound
		}
	} else {
		variant, err = service.store.WithAuthorizedStreamVariantDownload(
			principal, bearer, mediaID, variantID, targetAuthorized,
			service.now().UnixMilli(), authorizedOpen,
		)
	}
	if err != nil {
		if file != nil {
			file.Close()
		}
		return StreamVariantDownload{}, err
	}
	if file == nil {
		return StreamVariantDownload{}, errors.New("stream variant descriptor not acquired")
	}
	return StreamVariantDownload{Variant: variant, File: file}, nil
}

// OpenModerationEvidence is a separate operator-only read path. Store
// authorization commits the append-only access audit before filesystem bytes
// are opened, and the evidence snapshot expires independently of user ACLs.
func (service *DownloadService) OpenModerationEvidence(
	ctx context.Context,
	operatorID, token, reportID string,
) (ModerationEvidenceDownload, error) {
	if ctx == nil {
		return ModerationEvidenceDownload{}, errors.New("nil moderation evidence context")
	}
	evidence, err := service.store.AuthorizeModerationEvidence(
		operatorID, token, reportID, service.now().UnixMilli(),
	)
	if err != nil {
		return ModerationEvidenceDownload{}, err
	}
	lock := canonicalStorageLock(evidence.StorageKey)
	lock.Lock()
	defer lock.Unlock()
	path, ok := CanonicalPath(service.canonicalDir, evidence.StorageKey)
	if !ok {
		return ModerationEvidenceDownload{}, errors.New("invalid moderation evidence storage identity")
	}
	before, err := os.Lstat(path)
	if err != nil || !before.Mode().IsRegular() || before.Size() != evidence.SizeBytes {
		return ModerationEvidenceDownload{}, errors.New("inspect moderation evidence storage")
	}
	file, err := os.Open(path)
	if err != nil {
		return ModerationEvidenceDownload{}, errors.New("open moderation evidence storage")
	}
	after, err := file.Stat()
	if err != nil || !after.Mode().IsRegular() || !os.SameFile(before, after) ||
		after.Size() != evidence.SizeBytes {
		file.Close()
		return ModerationEvidenceDownload{}, errors.New("verify moderation evidence storage")
	}
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil ||
		hex.EncodeToString(digest.Sum(nil)) != evidence.SHA256 {
		file.Close()
		return ModerationEvidenceDownload{}, errors.New("verify moderation evidence digest")
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		file.Close()
		return ModerationEvidenceDownload{}, errors.New("rewind moderation evidence storage")
	}
	return ModerationEvidenceDownload{Evidence: evidence, File: file}, nil
}

var streamStorageKeyPattern = regexp.MustCompile(`^stream/v1/[0-9a-f]{64}$`)

func StreamVariantPath(streamDir, storageKey string) (string, bool) {
	if streamDir == "" || !streamStorageKeyPattern.MatchString(storageKey) {
		return "", false
	}
	return filepath.Join(streamDir, strings.TrimPrefix(storageKey, "stream/v1/")+".bin"), true
}
