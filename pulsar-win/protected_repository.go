package main

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	protectedCredentialsFileName = "credentials.v1.dpapi"
	protectedEnvelopeVersion     = 1
	protectedEnvelopeHeaderSize  = 9
	maximumProtectedPayloadBytes = 16 << 10
	maximumCiphertextBytes       = 1 << 20
	maximumLegacyCredentialBytes = 64 << 10

	fileGenericRead      uint32 = 0x80000000
	fileGenericWrite     uint32 = 0x40000000
	fileShareNone        uint32 = 0
	fileCreateNew        uint32 = 1
	fileOpenExisting     uint32 = 3
	fileOpenAlways       uint32 = 4
	fileAttributeNormal  uint32 = 0x80
	fileFlagWriteThrough uint32 = 0x80000000
	moveReplaceExisting  uint32 = 0x1
	moveWriteThrough     uint32 = 0x8
)

var protectedEnvelopeMagic = [4]byte{'B', 'C', 'D', 'P'}

type secureFileHandle uintptr

type secureOpenSpec struct {
	Access      uint32
	Share       uint32
	Disposition uint32
	Flags       uint32
}

type secureFileOps interface {
	EnsureDir(string) error
	Exists(string) (bool, error)
	Open(string, secureOpenSpec) (secureFileHandle, error)
	Write(secureFileHandle, []byte) (int, error)
	Read(secureFileHandle, []byte) (int, error)
	Size(secureFileHandle) (int64, error)
	Flush(secureFileHandle) error
	Close(secureFileHandle) error
	Move(string, string, uint32) error
	Delete(string) error
	List(string) ([]string, error)
	AcquireLock(string) (secureFileHandle, error)
}

type RepositoryClock interface{ Now() time.Time }

type systemRepositoryClock struct{}

func (systemRepositoryClock) Now() time.Time { return time.Now() }

type CredentialRepositoryOptions struct {
	Directory string
	Protector dataProtector
	Files     secureFileOps
	Random    io.Reader
	Clock     RepositoryClock
}

// ProtectedCredentialRepository contains all portable migration, envelope,
// crash-safety, and recovery-record logic. Native DPAPI/file primitives are
// injected behind the interfaces above.
type ProtectedCredentialRepository struct {
	dir       string
	protector dataProtector
	files     secureFileOps
	random    io.Reader
	randomMu  sync.Mutex
	clock     RepositoryClock
}

func (r *ProtectedCredentialRepository) String() string {
	return "ProtectedCredentialRepository{<redacted>}"
}

func (r *ProtectedCredentialRepository) GoString() string { return r.String() }

func NewProtectedCredentialRepository(options CredentialRepositoryOptions) (*ProtectedCredentialRepository, error) {
	if options.Directory == "" || options.Protector == nil || options.Files == nil {
		return nil, errCredentialStorageUnavailable
	}
	if options.Random == nil {
		options.Random = rand.Reader
	}
	if options.Clock == nil {
		options.Clock = systemRepositoryClock{}
	}
	return &ProtectedCredentialRepository{dir: options.Directory, protector: options.Protector, files: options.Files, random: options.Random, clock: options.Clock}, nil
}

type PendingRecoveryRecord struct {
	CanonicalCoordinatorOrigin string `json:"canonical_coordinator_origin"`
	ActorID                    int64  `json:"actor_id"`
	RecoveryID                 string `json:"recovery_id"`
	PendingControlToken        string `json:"pending_control_token"`
	EverSent                   bool   `json:"ever_sent"`
}

func (r PendingRecoveryRecord) String() string {
	return fmt.Sprintf("PendingRecoveryRecord{actor:%d sent:%t origin:<redacted> recovery:<redacted> token:<redacted>}", r.ActorID, r.EverSent)
}

func (r PendingRecoveryRecord) GoString() string { return r.String() }

func (r PendingRecoveryRecord) validate() error {
	origin, err := CanonicalCoordinatorOrigin(r.CanonicalCoordinatorOrigin)
	if err != nil || origin.String() != r.CanonicalCoordinatorOrigin || r.ActorID <= 0 || !recoveryIDPattern.MatchString(r.RecoveryID) || !lowerHexTokenPattern.MatchString(r.PendingControlToken) {
		return errCredentialCorrupt
	}
	return nil
}

func (r PendingRecoveryRecord) sameCandidate(other PendingRecoveryRecord) bool {
	return r.CanonicalCoordinatorOrigin == other.CanonicalCoordinatorOrigin && r.ActorID == other.ActorID && r.RecoveryID == other.RecoveryID && r.PendingControlToken == other.PendingControlToken
}

func (r *ProtectedCredentialRepository) LoadBundle() (*CredentialBundle, error) {
	path := r.activePath()
	release := globalRepositoryLocks.acquire(path)
	defer release()
	return r.loadBundleLocked()
}

func (r *ProtectedCredentialRepository) SaveBundle(bundle CredentialBundle) error {
	if err := bundle.validate(); err != nil {
		return errCredentialCorrupt
	}
	path := r.activePath()
	release := globalRepositoryLocks.acquire(path)
	defer release()
	if _, err := r.loadBundleLocked(); err != nil {
		return err
	}
	return r.writeProtected(path, bundle, decodeCredentialBundle)
}

func (r *ProtectedCredentialRepository) UpdateRecoveryMetadata(capability ControlCapability, recoveryID string) error {
	origin, controlToken := capability.actorBearer()
	actorID := capability.value.ActorID
	if origin.String() == "" || actorID <= 0 || !lowerHexTokenPattern.MatchString(controlToken) || !recoveryIDPattern.MatchString(recoveryID) {
		return errCredentialCorrupt
	}
	path := r.activePath()
	release := globalRepositoryLocks.acquire(path)
	defer release()
	bundle, err := r.loadBundleLocked()
	if err != nil {
		return err
	}
	if bundle == nil || bundle.Control == nil || bundle.Control.ActorID != actorID ||
		bundle.Control.ControlToken != controlToken || bundle.CoordinatorOrigin != origin.String() {
		return errCredentialStorageConflict
	}
	bundle.RecoveryID = recoveryID
	bundle.RecoveryConsumed = false
	bundle.RecoveryBackupAcknowledged = false
	if err := bundle.validate(); err != nil {
		return errCredentialStorageConflict
	}
	return r.writeProtected(path, *bundle, decodeCredentialBundle)
}

func (r *ProtectedCredentialRepository) AcknowledgeRecoveryBackup(origin CoordinatorOrigin, actorID int64, recoveryID string) error {
	if origin.String() == "" || actorID <= 0 || !recoveryIDPattern.MatchString(recoveryID) {
		return errCredentialCorrupt
	}
	path := r.activePath()
	release := globalRepositoryLocks.acquire(path)
	defer release()
	bundle, err := r.loadBundleLocked()
	if err != nil {
		return err
	}
	if bundle == nil || bundle.Control == nil || bundle.Control.ActorID != actorID || bundle.CoordinatorOrigin != origin.String() || bundle.RecoveryID != recoveryID {
		return errCredentialStorageConflict
	}
	bundle.RecoveryBackupAcknowledged = true
	if err := bundle.validate(); err != nil {
		return errCredentialStorageConflict
	}
	return r.writeProtected(path, *bundle, decodeCredentialBundle)
}

func (r *ProtectedCredentialRepository) UpdateNode(node NodeCredential) error {
	if err := validateNodeCredential(node); err != nil {
		return errCredentialCorrupt
	}
	path := r.activePath()
	release := globalRepositoryLocks.acquire(path)
	defer release()
	bundle, err := r.loadBundleLocked()
	if err != nil {
		return err
	}
	if bundle == nil {
		bundle = &CredentialBundle{Version: credentialBundleVersion}
	}
	bundle.Node = &node
	if bundle.CoordinatorOrigin == "" {
		origin, err := canonicalCoordinatorOriginFromWSURL(node.WSURL)
		if err != nil {
			return errCredentialCorrupt
		}
		bundle.CoordinatorOrigin = origin.String()
	}
	if err := bundle.validate(); err != nil {
		return errCredentialStorageConflict
	}
	return r.writeProtected(path, *bundle, decodeCredentialBundle)
}

func (r *ProtectedCredentialRepository) loadBundleLocked() (*CredentialBundle, error) {
	if err := r.files.EnsureDir(r.dir); err != nil {
		return nil, storageError("directory")
	}
	legacyPath := filepath.Join(r.dir, credentialsFileName)
	activePath := r.activePath()
	legacyExists, err := r.files.Exists(legacyPath)
	if err != nil {
		return nil, storageError("inspect")
	}
	activeExists, err := r.files.Exists(activePath)
	if err != nil {
		return nil, storageError("inspect")
	}
	if !legacyExists {
		if !activeExists {
			return nil, nil
		}
		bundle, err := r.readBundle(activePath)
		if err != nil {
			return nil, err
		}
		return &bundle, nil
	}
	legacy, err := r.readLegacyCredentials(legacyPath)
	if err != nil {
		return nil, err
	}
	node := nodeFromCredentials(legacy)
	nodeOrigin, err := canonicalCoordinatorOriginFromWSURL(node.WSURL)
	if err != nil {
		return nil, errCredentialCorrupt
	}
	if activeExists {
		bundle, err := r.readBundle(activePath)
		if err != nil {
			return nil, err
		}
		if bundle.Node == nil {
			bundle.Node = &node
			if err := bundle.validate(); err != nil {
				return nil, errCredentialMigrationConflict
			}
			if err := r.writeProtected(activePath, bundle, decodeCredentialBundle); err != nil {
				return nil, err
			}
		} else if *bundle.Node != node {
			return nil, errCredentialMigrationConflict
		}
		if err := r.files.Delete(legacyPath); err != nil {
			return nil, storageError("remove legacy")
		}
		return &bundle, nil
	}
	bundle := CredentialBundle{Version: credentialBundleVersion, Node: &node, CoordinatorOrigin: nodeOrigin.String()}
	if err := r.writeProtected(activePath, bundle, decodeCredentialBundle); err != nil {
		return nil, err
	}
	verified, err := r.readBundle(activePath)
	if err != nil || !reflect.DeepEqual(verified, bundle) {
		return nil, storageError("verify migration")
	}
	if err := r.files.Delete(legacyPath); err != nil {
		return nil, storageError("remove legacy")
	}
	return &bundle, nil
}

func (r *ProtectedCredentialRepository) LoadPending(origin CoordinatorOrigin, actorID int64) (*PendingRecoveryRecord, error) {
	path, err := r.pendingPath(origin, actorID)
	if err != nil {
		return nil, err
	}
	release := globalRepositoryLocks.acquire(path)
	defer release()
	return r.loadPendingLocked(path, origin, actorID)
}

func (r *ProtectedCredentialRepository) CreatePendingUnsent(record PendingRecoveryRecord) error {
	if record.EverSent || record.validate() != nil {
		return errCredentialCorrupt
	}
	origin, _ := CanonicalCoordinatorOrigin(record.CanonicalCoordinatorOrigin)
	path, _ := r.pendingPath(origin, record.ActorID)
	release := globalRepositoryLocks.acquire(path)
	defer release()
	existing, err := r.loadPendingLocked(path, origin, record.ActorID)
	if err != nil {
		return err
	}
	if existing != nil {
		return errCredentialStorageConflict
	}
	return r.writeProtected(path, record, decodePendingRecovery)
}

func (r *ProtectedCredentialRepository) ReplacePendingUnsent(record PendingRecoveryRecord) error {
	if record.EverSent || record.validate() != nil {
		return errCredentialCorrupt
	}
	origin, _ := CanonicalCoordinatorOrigin(record.CanonicalCoordinatorOrigin)
	path, _ := r.pendingPath(origin, record.ActorID)
	release := globalRepositoryLocks.acquire(path)
	defer release()
	existing, err := r.loadPendingLocked(path, origin, record.ActorID)
	if err != nil {
		return err
	}
	if existing != nil && existing.EverSent {
		return errCredentialStorageConflict
	}
	return r.writeProtected(path, record, decodePendingRecovery)
}

func (r *ProtectedCredentialRepository) MarkPendingSent(expected PendingRecoveryRecord) (PendingRecoveryRecord, error) {
	if expected.EverSent || expected.validate() != nil {
		return PendingRecoveryRecord{}, errCredentialCorrupt
	}
	origin, _ := CanonicalCoordinatorOrigin(expected.CanonicalCoordinatorOrigin)
	path, _ := r.pendingPath(origin, expected.ActorID)
	release := globalRepositoryLocks.acquire(path)
	defer release()
	existing, err := r.loadPendingLocked(path, origin, expected.ActorID)
	if err != nil {
		return PendingRecoveryRecord{}, err
	}
	if existing == nil || !existing.sameCandidate(expected) {
		return PendingRecoveryRecord{}, errCredentialStorageConflict
	}
	if existing.EverSent {
		return *existing, nil
	}
	sent := expected
	sent.EverSent = true
	if err := r.writeProtected(path, sent, decodePendingRecovery); err != nil {
		return PendingRecoveryRecord{}, err
	}
	return sent, nil
}

func (r *ProtectedCredentialRepository) DeletePendingExact(expected PendingRecoveryRecord) error {
	origin, err := CanonicalCoordinatorOrigin(expected.CanonicalCoordinatorOrigin)
	if err != nil || expected.validate() != nil {
		return errCredentialCorrupt
	}
	path, _ := r.pendingPath(origin, expected.ActorID)
	release := globalRepositoryLocks.acquire(path)
	defer release()
	existing, err := r.loadPendingLocked(path, origin, expected.ActorID)
	if err != nil {
		return err
	}
	if existing == nil {
		return nil
	}
	if *existing != expected && !(expected.EverSent && existing.EverSent && existing.sameCandidate(expected)) {
		return errCredentialStorageConflict
	}
	if err := r.files.Delete(path); err != nil {
		return storageError("delete pending")
	}
	return nil
}

func (r *ProtectedCredentialRepository) PromotePending(record PendingRecoveryRecord, context *ActorContext) (CredentialBundle, error) {
	if !record.EverSent || record.validate() != nil {
		return CredentialBundle{}, errCredentialCorrupt
	}
	activePath := r.activePath()
	release := globalRepositoryLocks.acquire(activePath)
	defer release()
	bundle, err := r.loadBundleLocked()
	if err != nil {
		return CredentialBundle{}, err
	}
	if bundle == nil {
		bundle = &CredentialBundle{Version: credentialBundleVersion}
	}
	if bundle.Control != nil && bundle.Control.ControlToken == record.PendingControlToken {
		if bundle.Control.ActorID != record.ActorID || bundle.CoordinatorOrigin != record.CanonicalCoordinatorOrigin {
			return CredentialBundle{}, errCredentialStorageConflict
		}
		if bundle.RecoveryID != record.RecoveryID {
			// Rotation is the only supported transition that changes recovery
			// generation without changing the already-promoted control token. It
			// resets consumed=false; preserve that newer generation (and its exact
			// acknowledgement) while the stale pending record is retired.
			if bundle.RecoveryID == "" || bundle.RecoveryConsumed {
				return CredentialBundle{}, errCredentialStorageConflict
			}
			return *bundle, nil
		}
		if !bundle.RecoveryConsumed {
			return CredentialBundle{}, errCredentialStorageConflict
		}
		if bundle.RecoveryBackupAcknowledged {
			bundle.RecoveryBackupAcknowledged = false
			if err := r.writeProtected(activePath, *bundle, decodeCredentialBundle); err != nil {
				return CredentialBundle{}, err
			}
		}
		return *bundle, nil
	}
	control := ControlCredential{ActorID: record.ActorID, ControlToken: record.PendingControlToken, Context: ControlContextLimited}
	if context != nil {
		if context.ActorID != record.ActorID || context.OrbitID <= 0 || !validRole(context.Role) {
			return CredentialBundle{}, errCredentialCorrupt
		}
		control.OrbitID, control.Role, control.Context = context.OrbitID, context.Role, ControlContextActive
	} else if bundle.Control != nil && bundle.Control.ActorID == record.ActorID && bundle.Control.Context == ControlContextActive {
		control.LastKnownOrbitID, control.LastKnownRole = bundle.Control.OrbitID, bundle.Control.Role
	}
	bundle.Control = &control
	bundle.RecoveryID = record.RecoveryID
	bundle.CoordinatorOrigin = record.CanonicalCoordinatorOrigin
	bundle.RecoveryConsumed = true
	bundle.RecoveryBackupAcknowledged = false
	if err := bundle.validate(); err != nil {
		return CredentialBundle{}, err
	}
	if err := r.writeProtected(activePath, *bundle, decodeCredentialBundle); err != nil {
		return CredentialBundle{}, err
	}
	return *bundle, nil
}

func (r *ProtectedCredentialRepository) AcquireRecoveryScope(origin CoordinatorOrigin, actorID int64) (func() error, error) {
	path, err := r.pendingPath(origin, actorID)
	if err != nil {
		return nil, err
	}
	processRelease := globalRecoveryLocks.acquire(path)
	if err := r.files.EnsureDir(r.dir); err != nil {
		processRelease()
		return nil, storageError("directory")
	}
	lockPath := filepath.Join(r.dir, "recovery-lock-v1-"+scopeDigest(origin, actorID)+".lock")
	handle, err := r.files.AcquireLock(lockPath)
	if err != nil {
		processRelease()
		return nil, errCredentialStorageBusy
	}
	var once sync.Once
	return func() error {
		var closeErr error
		once.Do(func() {
			if err := r.files.Close(handle); err != nil {
				closeErr = storageError("release recovery lock")
			}
			processRelease()
		})
		return closeErr
	}, nil
}

func (r *ProtectedCredentialRepository) loadPendingLocked(path string, origin CoordinatorOrigin, actorID int64) (*PendingRecoveryRecord, error) {
	exists, err := r.files.Exists(path)
	if err != nil {
		return nil, storageError("inspect pending")
	}
	if !exists {
		return nil, nil
	}
	record, err := r.readPending(path)
	if err != nil {
		return nil, err
	}
	if record.CanonicalCoordinatorOrigin != origin.String() || record.ActorID != actorID {
		return nil, errCredentialCorrupt
	}
	return &record, nil
}

func (r *ProtectedCredentialRepository) activePath() string {
	return filepath.Join(r.dir, protectedCredentialsFileName)
}

func (r *ProtectedCredentialRepository) pendingPath(origin CoordinatorOrigin, actorID int64) (string, error) {
	if origin.String() == "" || actorID <= 0 {
		return "", errCredentialCorrupt
	}
	return filepath.Join(r.dir, "recovery-pending-v1-"+scopeDigest(origin, actorID)+".dpapi"), nil
}

func scopeDigest(origin CoordinatorOrigin, actorID int64) string {
	source := origin.String() + "\x00" + strconv.FormatInt(actorID, 10)
	digest := sha256.Sum256([]byte(source))
	return hex.EncodeToString(digest[:])
}

func (r *ProtectedCredentialRepository) readBundle(path string) (CredentialBundle, error) {
	value, err := r.readProtected(path, decodeCredentialBundle)
	if err != nil {
		return CredentialBundle{}, err
	}
	return value.(CredentialBundle), nil
}

func (r *ProtectedCredentialRepository) readPending(path string) (PendingRecoveryRecord, error) {
	value, err := r.readProtected(path, decodePendingRecovery)
	if err != nil {
		return PendingRecoveryRecord{}, err
	}
	return value.(PendingRecoveryRecord), nil
}

type protectedDecoder func([]byte) (any, error)

func (r *ProtectedCredentialRepository) readProtected(path string, decoder protectedDecoder) (any, error) {
	handle, err := r.files.Open(path, secureOpenSpec{Access: fileGenericRead, Share: fileShareNone, Disposition: fileOpenExisting, Flags: fileAttributeNormal})
	if err != nil {
		return nil, storageError("open protected")
	}
	value, readErr := r.readProtectedHandle(handle, decoder)
	closeErr := r.files.Close(handle)
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, storageError("close protected")
	}
	return value, nil
}

func (r *ProtectedCredentialRepository) readProtectedHandle(handle secureFileHandle, decoder protectedDecoder) (any, error) {
	size, err := r.files.Size(handle)
	if err != nil || size <= 0 || size > maximumCiphertextBytes {
		return nil, storageError("size protected")
	}
	ciphertext := make([]byte, int(size))
	read := 0
	for read < len(ciphertext) {
		n, err := r.files.Read(handle, ciphertext[read:])
		if err != nil || n <= 0 || n > len(ciphertext)-read {
			zeroBytes(ciphertext)
			return nil, storageError("read protected")
		}
		read += n
	}
	plaintext, err := r.protector.Unprotect(ciphertext)
	zeroBytes(ciphertext)
	if err != nil {
		zeroBytes(plaintext)
		return nil, storageError("decrypt protected")
	}
	payload, err := decodeProtectedEnvelope(plaintext)
	zeroBytes(plaintext)
	if err != nil {
		return nil, errCredentialCorrupt
	}
	value, err := decoder(payload)
	zeroBytes(payload)
	if err != nil {
		return nil, errCredentialCorrupt
	}
	return value, nil
}

func (r *ProtectedCredentialRepository) writeProtected(path string, expected any, decoder protectedDecoder) error {
	if err := r.files.EnsureDir(filepath.Dir(path)); err != nil {
		return storageError("directory")
	}
	r.cleanupStaleTemps(path)
	payload, err := json.Marshal(expected)
	if err != nil || len(payload) > maximumProtectedPayloadBytes {
		zeroBytes(payload)
		return errCredentialCorrupt
	}
	plaintext, err := encodeProtectedEnvelope(payload)
	zeroBytes(payload)
	if err != nil {
		return err
	}
	suffix := make([]byte, 8)
	r.randomMu.Lock()
	_, randomErr := io.ReadFull(r.random, suffix)
	r.randomMu.Unlock()
	if randomErr != nil {
		zeroBytes(plaintext)
		return storageError("random")
	}
	tempPath := path + ".tmp." + hex.EncodeToString(suffix)
	zeroBytes(suffix)
	handle, err := r.files.Open(tempPath, secureOpenSpec{Access: fileGenericWrite, Share: fileShareNone, Disposition: fileCreateNew, Flags: fileAttributeNormal | fileFlagWriteThrough})
	if err != nil {
		zeroBytes(plaintext)
		return storageError("create temporary")
	}
	ciphertext, protectErr := r.protector.Protect(plaintext)
	zeroBytes(plaintext)
	if protectErr != nil || len(ciphertext) == 0 || len(ciphertext) > maximumCiphertextBytes {
		_ = r.files.Close(handle)
		_ = r.files.Delete(tempPath)
		zeroBytes(ciphertext)
		return storageError("encrypt protected")
	}
	written := 0
	for written < len(ciphertext) {
		n, writeErr := r.files.Write(handle, ciphertext[written:])
		if writeErr != nil || n <= 0 || n > len(ciphertext)-written {
			_ = r.files.Close(handle)
			_ = r.files.Delete(tempPath)
			zeroBytes(ciphertext)
			return storageError("write protected")
		}
		written += n
	}
	zeroBytes(ciphertext)
	if err := r.files.Flush(handle); err != nil {
		_ = r.files.Close(handle)
		_ = r.files.Delete(tempPath)
		return storageError("flush protected")
	}
	if err := r.files.Close(handle); err != nil {
		_ = r.files.Delete(tempPath)
		return storageError("close temporary")
	}
	if err := r.files.Move(tempPath, path, moveReplaceExisting|moveWriteThrough); err != nil {
		_ = r.files.Delete(tempPath)
		return storageError("move protected")
	}
	actual, err := r.readProtected(path, decoder)
	if err != nil {
		return err
	}
	if !reflect.DeepEqual(actual, expected) {
		return storageError("verify protected")
	}
	return nil
}

func (r *ProtectedCredentialRepository) cleanupStaleTemps(path string) {
	names, err := r.files.List(filepath.Dir(path))
	if err != nil {
		return
	}
	prefix := filepath.Base(path) + ".tmp."
	for _, name := range names {
		if !strings.HasPrefix(name, prefix) {
			continue
		}
		suffix := strings.TrimPrefix(name, prefix)
		if len(suffix) != 16 {
			continue
		}
		if _, err := hex.DecodeString(suffix); err != nil {
			continue
		}
		_ = r.files.Delete(filepath.Join(filepath.Dir(path), name))
	}
}

func encodeProtectedEnvelope(payload []byte) ([]byte, error) {
	if len(payload) > maximumProtectedPayloadBytes {
		return nil, errCredentialCorrupt
	}
	result := make([]byte, protectedEnvelopeHeaderSize+len(payload))
	copy(result[:4], protectedEnvelopeMagic[:])
	result[4] = protectedEnvelopeVersion
	binary.LittleEndian.PutUint32(result[5:9], uint32(len(payload)))
	copy(result[9:], payload)
	return result, nil
}

func decodeProtectedEnvelope(value []byte) ([]byte, error) {
	if len(value) < protectedEnvelopeHeaderSize || !bytes.Equal(value[:4], protectedEnvelopeMagic[:]) || value[4] != protectedEnvelopeVersion {
		return nil, errCredentialCorrupt
	}
	length := binary.LittleEndian.Uint32(value[5:9])
	if length > maximumProtectedPayloadBytes || int(length) != len(value)-protectedEnvelopeHeaderSize {
		return nil, errCredentialCorrupt
	}
	return append([]byte(nil), value[protectedEnvelopeHeaderSize:]...), nil
}

func decodeCredentialBundle(payload []byte) (any, error) {
	object, err := parseStrictJSONObject(payload)
	if err != nil || !validCredentialBundleJSONObject(object) {
		return nil, errCredentialCorrupt
	}
	var bundle CredentialBundle
	if err := strictDecodeProtectedJSON(payload, &bundle); err != nil || bundle.validate() != nil {
		return nil, errCredentialCorrupt
	}
	return bundle, nil
}

func validCredentialBundleJSONObject(object map[string]any) bool {
	if !objectHasOnlyKeys(object,
		"version", "node", "control", "recovery_id", "coordinator_origin",
		"recovery_consumed", "recovery_backup_acknowledged",
	) {
		return false
	}
	if _, ok := jsonInt64(object, "version"); !ok {
		return false
	}
	if _, ok := jsonString(object, "coordinator_origin"); !ok {
		return false
	}
	if !optionalJSONString(object, "recovery_id") ||
		!optionalJSONBoolean(object, "recovery_consumed") ||
		!optionalJSONBoolean(object, "recovery_backup_acknowledged") {
		return false
	}
	if node, exists := object["node"]; exists {
		nodeObject, ok := node.(map[string]any)
		if !ok || !exactObjectKeys(nodeObject, "orbit_id", "slot", "node_token", "ws_url") ||
			!requiredJSONInteger(nodeObject, "orbit_id") ||
			!requiredJSONString(nodeObject, "slot") ||
			!requiredJSONString(nodeObject, "node_token") ||
			!requiredJSONString(nodeObject, "ws_url") {
			return false
		}
	}
	if control, exists := object["control"]; exists {
		controlObject, ok := control.(map[string]any)
		if !ok || !objectHasOnlyKeys(controlObject,
			"actor_id", "orbit_id", "role", "last_known_orbit_id", "last_known_role",
			"control_token", "context",
		) ||
			!requiredJSONInteger(controlObject, "actor_id") ||
			!optionalJSONInteger(controlObject, "orbit_id") ||
			!optionalJSONString(controlObject, "role") ||
			!optionalJSONInteger(controlObject, "last_known_orbit_id") ||
			!optionalJSONString(controlObject, "last_known_role") ||
			!requiredJSONString(controlObject, "control_token") ||
			!requiredJSONString(controlObject, "context") {
			return false
		}
	}
	return true
}

func decodePendingRecovery(payload []byte) (any, error) {
	object, err := parseStrictJSONObject(payload)
	if err != nil || !exactObjectKeys(object, "canonical_coordinator_origin", "actor_id", "recovery_id", "pending_control_token", "ever_sent") {
		return nil, errCredentialCorrupt
	}
	if _, ok := object["ever_sent"].(bool); !ok {
		return nil, errCredentialCorrupt
	}
	var record PendingRecoveryRecord
	if err := strictDecodeProtectedJSON(payload, &record); err != nil || record.validate() != nil {
		return nil, errCredentialCorrupt
	}
	return record, nil
}

func optionalJSONBoolean(object map[string]any, key string) bool {
	value, exists := object[key]
	if !exists {
		return true
	}
	_, ok := value.(bool)
	return ok
}

func requiredJSONInteger(object map[string]any, key string) bool {
	_, ok := jsonInt64(object, key)
	return ok
}

func optionalJSONInteger(object map[string]any, key string) bool {
	if _, exists := object[key]; !exists {
		return true
	}
	return requiredJSONInteger(object, key)
}

func requiredJSONString(object map[string]any, key string) bool {
	_, ok := jsonString(object, key)
	return ok
}

func optionalJSONString(object map[string]any, key string) bool {
	if _, exists := object[key]; !exists {
		return true
	}
	return requiredJSONString(object, key)
}

func objectHasOnlyKeys(object map[string]any, keys ...string) bool {
	allowed := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		allowed[key] = struct{}{}
	}
	for key := range object {
		if _, ok := allowed[key]; !ok {
			return false
		}
	}
	return true
}

func strictDecodeProtectedJSON(payload []byte, target any) error {
	if _, err := parseStrictJSONObject(payload); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if !errors.Is(decoder.Decode(&extra), io.EOF) {
		return errCredentialCorrupt
	}
	return nil
}

func (r *ProtectedCredentialRepository) readLegacyCredentials(path string) (Credentials, error) {
	handle, err := r.files.Open(path, secureOpenSpec{Access: fileGenericRead, Share: fileShareNone, Disposition: fileOpenExisting, Flags: fileAttributeNormal})
	if err != nil {
		return Credentials{}, storageError("open legacy")
	}
	size, sizeErr := r.files.Size(handle)
	if sizeErr != nil || size <= 0 || size > maximumLegacyCredentialBytes {
		_ = r.files.Close(handle)
		return Credentials{}, errCredentialCorrupt
	}
	raw := make([]byte, int(size))
	read := 0
	for read < len(raw) {
		n, readErr := r.files.Read(handle, raw[read:])
		if readErr != nil || n <= 0 || n > len(raw)-read {
			_ = r.files.Close(handle)
			zeroBytes(raw)
			return Credentials{}, storageError("read legacy")
		}
		read += n
	}
	if err := r.files.Close(handle); err != nil {
		zeroBytes(raw)
		return Credentials{}, storageError("close legacy")
	}
	object, err := parseStrictJSONObject(raw)
	if err != nil || !exactObjectKeys(object, "orbit_id", "slot", "token", "ws_url") {
		zeroBytes(raw)
		return Credentials{}, errCredentialCorrupt
	}
	orbitID, okOrbit := jsonInt64(object, "orbit_id")
	slot, okSlot := jsonString(object, "slot")
	token, okToken := jsonString(object, "token")
	wsURL, okWS := jsonString(object, "ws_url")
	zeroBytes(raw)
	credentials := Credentials{OrbitID: orbitID, Slot: slot, Token: token, WSURL: wsURL}
	if !okOrbit || !okSlot || !okToken || !okWS || ValidateCredentials(credentials) != nil {
		return Credentials{}, errCredentialCorrupt
	}
	return credentials, nil
}

type keyedLockSet struct {
	mu    sync.Mutex
	locks map[string]*keyedLockEntry
}

type keyedLockEntry struct {
	mu   sync.Mutex
	refs int
}

func (s *keyedLockSet) acquire(key string) func() {
	s.mu.Lock()
	if s.locks == nil {
		s.locks = make(map[string]*keyedLockEntry)
	}
	entry := s.locks[key]
	if entry == nil {
		entry = &keyedLockEntry{}
		s.locks[key] = entry
	}
	entry.refs++
	s.mu.Unlock()
	entry.mu.Lock()
	return func() {
		entry.mu.Unlock()
		s.mu.Lock()
		entry.refs--
		if entry.refs == 0 {
			delete(s.locks, key)
		}
		s.mu.Unlock()
	}
}

var (
	globalRepositoryLocks keyedLockSet
	globalRecoveryLocks   keyedLockSet
)

type CredentialStorageError struct{ operation string }

func (e *CredentialStorageError) Error() string {
	return "credential storage operation failed: " + e.operation
}
func (e *CredentialStorageError) String() string { return e.Error() }
func (e *CredentialStorageError) GoString() string {
	return "CredentialStorageError{<redacted>}"
}

func (e *CredentialStorageError) Operation() string { return e.operation }

func storageError(operation string) error { return &CredentialStorageError{operation: operation} }

var (
	errCredentialStorageUnavailable = errors.New("protected credential storage is unavailable on this platform")
	errCredentialStorageConflict    = errors.New("protected credential state conflicts with the requested update")
	errCredentialMigrationConflict  = errors.New("legacy and protected credentials conflict")
	errCredentialStorageBusy        = errors.New("protected credential recovery scope is busy")
)
