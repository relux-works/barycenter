package media

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
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

type MediaDownload struct {
	Item store.MediaItem
	File *os.File
}

type ModerationEvidenceDownload struct {
	Evidence store.ModerationEvidence
	File     *os.File
}

type DownloadService struct {
	store        *store.Store
	canonicalDir string
	now          func() time.Time

	targetsMu sync.RWMutex
	targets   MediaTargetSnapshotReader
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
	canonicalDir := filepath.Join(mediaDir, "canonical")
	if err := os.MkdirAll(canonicalDir, 0o700); err != nil {
		return nil, errors.New("initialize media download storage")
	}
	if err := os.Chmod(canonicalDir, 0o700); err != nil {
		return nil, errors.New("secure media download storage")
	}
	return &DownloadService{store: st, canonicalDir: canonicalDir, now: time.Now}, nil
}

// SetTargetSnapshotReader is the forward-only handoff to transmission
// persistence. Nil is fail-closed: until that downstream store exists, generic
// node reads remain unavailable while owning control reads are still useful.
func (service *DownloadService) SetTargetSnapshotReader(reader MediaTargetSnapshotReader) {
	service.targetsMu.Lock()
	service.targets = reader
	persistedStore, ok := reader.(*store.Store)
	service.persistedTargets = ok && persistedStore == service.store
	service.targetsMu.Unlock()
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
	if principal.Capabilities.Has(store.CapabilityNode) &&
		!principal.Capabilities.Has(store.CapabilityControl) {
		service.targetsMu.RLock()
		reader := service.targets
		persistedTargets = service.persistedTargets
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
