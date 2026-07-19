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
	"math"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

const (
	windowsE2EEEnvelopeVersion       = 1
	windowsE2EEEnvelopeHeaderBytes   = 9
	windowsE2EEMaxPlaintextBytes     = 3 << 20
	windowsE2EEMaxCiphertextBytes    = 4 << 20
	WindowsE2EEMaxPrivateKeyBytes    = 4096
	WindowsE2EEMaxOpaqueStateBytes   = 1 << 20
	WindowsE2EEMaxGrantBytes         = 64 << 10
	WindowsE2EEMaxCachedContentKeys  = 32
	WindowsE2EEMaxCachedKeyBytes     = 64 << 10
	windowsE2EEMaxIndividualKeyBytes = 4096
)

var windowsE2EEEnvelopeMagic = [4]byte{'B', 'E', 'K', 'S'}

var (
	ErrWindowsE2EEConflict        = errors.New("protected E2EE key state conflicts with the requested update")
	ErrWindowsE2EECorrupt         = errors.New("protected E2EE key state is invalid")
	ErrWindowsE2EEExpired         = errors.New("protected E2EE key state is expired")
	ErrWindowsE2EEInvalid         = errors.New("protected E2EE key state request is invalid")
	ErrWindowsE2EENotFound        = errors.New("protected E2EE key state was not found")
	ErrWindowsE2EEReplay          = errors.New("protected E2EE key state replay was rejected")
	ErrWindowsE2EERollbackOrClone = errors.New("protected E2EE key state rollback or clone was detected")
	ErrWindowsE2EEStaleEpoch      = errors.New("protected E2EE key state epoch is stale")
	ErrWindowsE2EEUnavailable     = errors.New("protected E2EE key state is unavailable")
	ErrWindowsE2EEBusy            = errors.New("protected E2EE key state is owned by another process")
)

type WindowsE2EEKeyStateOptions struct {
	Directory string
	Protector dataProtector
	Files     secureFileOps
	Random    io.Reader
}

type WindowsE2EEKeyStateRepository struct {
	dir       string
	protector dataProtector
	files     secureFileOps
	random    io.Reader
	randomMu  sync.Mutex
}

func NewWindowsE2EEKeyStateRepository(options WindowsE2EEKeyStateOptions) (*WindowsE2EEKeyStateRepository, error) {
	if options.Directory == "" || options.Protector == nil || options.Files == nil {
		return nil, ErrWindowsE2EEUnavailable
	}
	if options.Random == nil {
		options.Random = rand.Reader
	}
	return &WindowsE2EEKeyStateRepository{
		dir: filepath.Join(options.Directory, "e2ee-key-state-v1"), protector: options.Protector,
		files: options.Files, random: options.Random,
	}, nil
}

func (r *WindowsE2EEKeyStateRepository) String() string {
	return "WindowsE2EEKeyStateRepository{<redacted>}"
}

func (r *WindowsE2EEKeyStateRepository) GoString() string { return r.String() }

type WindowsE2EESecretLease struct {
	mu        sync.Mutex
	bytes     []byte
	destroyed bool
}

func newWindowsE2EESecretLease(value []byte) *WindowsE2EESecretLease {
	lease := &WindowsE2EESecretLease{bytes: append([]byte(nil), value...)}
	runtime.SetFinalizer(lease, func(value *WindowsE2EESecretLease) { value.Destroy() })
	return lease
}

func (l *WindowsE2EESecretLease) String() string   { return "WindowsE2EESecretLease{<redacted>}" }
func (l *WindowsE2EESecretLease) GoString() string { return l.String() }

func (l *WindowsE2EESecretLease) WithBytes(body func([]byte) error) error {
	if body == nil {
		return ErrWindowsE2EEInvalid
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.destroyed || len(l.bytes) == 0 {
		return ErrWindowsE2EEUnavailable
	}
	value := append([]byte(nil), l.bytes...)
	defer zeroBytes(value)
	return body(value)
}

func (l *WindowsE2EESecretLease) Destroy() {
	l.mu.Lock()
	if !l.destroyed {
		zeroBytes(l.bytes)
		l.bytes = nil
		l.destroyed = true
	}
	l.mu.Unlock()
	runtime.SetFinalizer(l, nil)
}

type WindowsE2EEDeviceIdentityMetadata struct {
	DeviceID       string
	InstallationID string
	KeyFormat      string
	Revision       uint64
	CreatedAtMS    int64
}

type WindowsE2EEDeviceIdentityLease struct {
	Metadata  WindowsE2EEDeviceIdentityMetadata
	signing   *WindowsE2EESecretLease
	agreement *WindowsE2EESecretLease
}

func (l *WindowsE2EEDeviceIdentityLease) String() string {
	return "WindowsE2EEDeviceIdentityLease{device:<redacted> keys:<redacted>}"
}

func (l *WindowsE2EEDeviceIdentityLease) GoString() string { return l.String() }

func (l *WindowsE2EEDeviceIdentityLease) WithSigningPrivateKey(body func([]byte) error) error {
	return l.signing.WithBytes(body)
}

func (l *WindowsE2EEDeviceIdentityLease) WithKeyAgreementPrivateKey(body func([]byte) error) error {
	return l.agreement.WithBytes(body)
}

func (l *WindowsE2EEDeviceIdentityLease) Destroy() {
	l.signing.Destroy()
	l.agreement.Destroy()
}

type WindowsE2EEGroupStateMetadata struct {
	GroupID              string
	InstallationID       string
	Epoch                uint64
	SendGeneration       uint64
	CommitDigest         string
	TargetSnapshotDigest string
	Revision             uint64
	UpdatedAtMS          int64
}

type WindowsE2EEGroupStateLease struct {
	Metadata WindowsE2EEGroupStateMetadata
	state    *WindowsE2EESecretLease
}

func (l *WindowsE2EEGroupStateLease) String() string {
	return fmt.Sprintf("WindowsE2EEGroupStateLease{group:<redacted> epoch:%d state:<redacted>}", l.Metadata.Epoch)
}

func (l *WindowsE2EEGroupStateLease) GoString() string { return l.String() }
func (l *WindowsE2EEGroupStateLease) WithOpaqueState(body func([]byte) error) error {
	return l.state.WithBytes(body)
}
func (l *WindowsE2EEGroupStateLease) Destroy() { l.state.Destroy() }

type WindowsE2EESendReservation struct {
	GroupID    string
	Epoch      uint64
	Generation uint64
	Domain     string
	Revision   uint64
}

type WindowsE2EEGrantMetadata struct {
	GrantID     string
	GroupID     string
	FirstEpoch  uint64
	LastEpoch   uint64
	ExpiresAtMS int64
	Revision    uint64
}

type WindowsE2EEContentKeyMetadata struct {
	ObjectID    string
	GroupID     string
	Epoch       uint64
	ExpiresAtMS int64
}

type WindowsE2EETargetDeviceDecision string

const (
	WindowsE2EETargetRoute             WindowsE2EETargetDeviceDecision = "route"
	WindowsE2EETargetRemovedEndpoint   WindowsE2EETargetDeviceDecision = "removed_endpoint"
	WindowsE2EETargetUnsupportedTarget WindowsE2EETargetDeviceDecision = "unsupported_target"
)

func DecideWindowsE2EETargetDevice(active bool, registered, verified, supported int) WindowsE2EETargetDeviceDecision {
	if !active || registered < 0 || verified < 0 || supported < 0 || supported > verified || verified > registered {
		return WindowsE2EETargetRemovedEndpoint
	}
	if verified == 0 {
		return WindowsE2EETargetRemovedEndpoint
	}
	if supported == 0 {
		return WindowsE2EETargetUnsupportedTarget
	}
	return WindowsE2EETargetRoute
}

type windowsE2EEKind string

const (
	windowsE2EEDeviceMetadata  windowsE2EEKind = "device_metadata"
	windowsE2EEDeviceSigning   windowsE2EEKind = "device_signing"
	windowsE2EEDeviceAgreement windowsE2EEKind = "device_agreement"
	windowsE2EEGroup           windowsE2EEKind = "group"
	windowsE2EEGrant           windowsE2EEKind = "grant"
	windowsE2EEContentCache    windowsE2EEKind = "content_cache"
)

type windowsE2EERecord struct {
	Version        int             `json:"version"`
	Kind           windowsE2EEKind `json:"kind"`
	InstallationID string          `json:"installation_id"`
	Scope          string          `json:"scope"`
	Revision       uint64          `json:"revision"`
	PayloadDigest  string          `json:"payload_digest"`
	Payload        json.RawMessage `json:"payload"`
	CreatedAtMS    int64           `json:"created_at_ms"`
	UpdatedAtMS    int64           `json:"updated_at_ms"`
}

type windowsE2EEWitness struct {
	Version        int             `json:"version"`
	Kind           windowsE2EEKind `json:"kind"`
	InstallationID string          `json:"installation_id"`
	Scope          string          `json:"scope"`
	Revision       uint64          `json:"revision"`
	RecordDigest   string          `json:"record_digest"`
}

type windowsE2EEDevicePayload struct {
	DeviceID       string `json:"device_id"`
	InstallationID string `json:"installation_id"`
	KeyFormat      string `json:"key_format"`
	CreatedAtMS    int64  `json:"created_at_ms"`
}

type windowsE2EEDeviceSecretPayload struct {
	DeviceID       string `json:"device_id"`
	InstallationID string `json:"installation_id"`
	KeyFormat      string `json:"key_format"`
	Role           string `json:"role"`
	PrivateKey     []byte `json:"private_key"`
	CreatedAtMS    int64  `json:"created_at_ms"`
}

type windowsE2EEGroupPayload struct {
	GroupID              string `json:"group_id"`
	Epoch                uint64 `json:"epoch"`
	SendGeneration       uint64 `json:"send_generation"`
	CommitDigest         string `json:"commit_digest"`
	TargetSnapshotDigest string `json:"target_snapshot_digest"`
	OpaqueState          []byte `json:"opaque_state"`
	UpdatedAtMS          int64  `json:"updated_at_ms"`
}

type windowsE2EEGrantPayload struct {
	GrantID     string `json:"grant_id"`
	GroupID     string `json:"group_id"`
	FirstEpoch  uint64 `json:"first_epoch"`
	LastEpoch   uint64 `json:"last_epoch"`
	ExpiresAtMS int64  `json:"expires_at_ms"`
	OpaqueGrant []byte `json:"opaque_grant"`
}

type windowsE2EEContentKeyEntry struct {
	ObjectID    string `json:"object_id"`
	GroupID     string `json:"group_id"`
	Epoch       uint64 `json:"epoch"`
	ExpiresAtMS int64  `json:"expires_at_ms"`
	Key         []byte `json:"key"`
	CachedAtMS  int64  `json:"cached_at_ms"`
}

type windowsE2EEContentCachePayload struct {
	Entries []windowsE2EEContentKeyEntry `json:"entries"`
}

func (r *WindowsE2EEKeyStateRepository) InstallDeviceIdentity(deviceID, keyFormat string, signing, agreement []byte, createdAtMS int64) (metadata WindowsE2EEDeviceIdentityMetadata, err error) {
	err = r.withExclusiveLock(func() error {
		if !validWindowsE2EEIdentifier(deviceID, 8, 128) || !validWindowsE2EELabel(keyFormat, 64) ||
			!validWindowsE2EESecret(signing, WindowsE2EEMaxPrivateKeyBytes) ||
			!validWindowsE2EESecret(agreement, WindowsE2EEMaxPrivateKeyBytes) || createdAtMS <= 0 {
			return ErrWindowsE2EEInvalid
		}
		currentMetadata, err := r.loadRecord(windowsE2EEDeviceMetadata, "device-metadata", "")
		if err != nil {
			return err
		}
		currentSigning, err := r.loadRecord(windowsE2EEDeviceSigning, "device-signing", "")
		if err != nil {
			return err
		}
		currentAgreement, err := r.loadRecord(windowsE2EEDeviceAgreement, "device-agreement", "")
		if err != nil {
			return err
		}
		if currentMetadata != nil || currentSigning != nil || currentAgreement != nil {
			if currentMetadata == nil || currentSigning == nil || currentAgreement == nil {
				return ErrWindowsE2EERollbackOrClone
			}
			var mp windowsE2EEDevicePayload
			var sp, ap windowsE2EEDeviceSecretPayload
			defer func() { zeroBytes(sp.PrivateKey) }()
			defer func() { zeroBytes(ap.PrivateKey) }()
			if err := decodeWindowsE2EEPayload(currentMetadata, &mp); err != nil {
				return err
			}
			if err := decodeWindowsE2EEPayload(currentSigning, &sp); err != nil {
				return err
			}
			if err := decodeWindowsE2EEPayload(currentAgreement, &ap); err != nil {
				return err
			}
			if err := validateWindowsE2EEDevicePayloads(mp, sp, ap); err != nil {
				return err
			}
			if mp.DeviceID != deviceID || mp.KeyFormat != keyFormat || !bytes.Equal(sp.PrivateKey, signing) || !bytes.Equal(ap.PrivateKey, agreement) {
				return ErrWindowsE2EEConflict
			}
			metadata = makeWindowsE2EEDeviceMetadata(mp, currentMetadata.Revision)
			return nil
		}
		installationBytes := make([]byte, 32)
		if err := r.readRandom(installationBytes); err != nil {
			return err
		}
		installationID := hex.EncodeToString(installationBytes)
		zeroBytes(installationBytes)
		mp := windowsE2EEDevicePayload{DeviceID: deviceID, InstallationID: installationID, KeyFormat: keyFormat, CreatedAtMS: createdAtMS}
		sp := windowsE2EEDeviceSecretPayload{DeviceID: deviceID, InstallationID: installationID, KeyFormat: keyFormat, Role: "signing", PrivateKey: append([]byte(nil), signing...), CreatedAtMS: createdAtMS}
		ap := windowsE2EEDeviceSecretPayload{DeviceID: deviceID, InstallationID: installationID, KeyFormat: keyFormat, Role: "agreement", PrivateKey: append([]byte(nil), agreement...), CreatedAtMS: createdAtMS}
		defer zeroBytes(sp.PrivateKey)
		defer zeroBytes(ap.PrivateKey)
		if _, err := r.persistSlot(windowsE2EEDeviceMetadata, "device-metadata", installationID, mp, 0, createdAtMS); err != nil {
			return err
		}
		if _, err := r.persistSlot(windowsE2EEDeviceSigning, "device-signing", installationID, sp, 0, createdAtMS); err != nil {
			return err
		}
		if _, err := r.persistSlot(windowsE2EEDeviceAgreement, "device-agreement", installationID, ap, 0, createdAtMS); err != nil {
			return err
		}
		metadata = makeWindowsE2EEDeviceMetadata(mp, 1)
		return nil
	})
	return metadata, err
}

func (r *WindowsE2EEKeyStateRepository) LoadDeviceIdentity(deviceID string) (lease *WindowsE2EEDeviceIdentityLease, err error) {
	err = r.withExclusiveLock(func() error {
		if !validWindowsE2EEIdentifier(deviceID, 8, 128) {
			return ErrWindowsE2EEInvalid
		}
		mr, err := r.loadRecord(windowsE2EEDeviceMetadata, "device-metadata", "")
		if err != nil {
			return err
		}
		sr, err := r.loadRecord(windowsE2EEDeviceSigning, "device-signing", "")
		if err != nil {
			return err
		}
		ar, err := r.loadRecord(windowsE2EEDeviceAgreement, "device-agreement", "")
		if err != nil {
			return err
		}
		if mr == nil && sr == nil && ar == nil {
			return ErrWindowsE2EENotFound
		}
		if mr == nil || sr == nil || ar == nil {
			return ErrWindowsE2EERollbackOrClone
		}
		var mp windowsE2EEDevicePayload
		var sp, ap windowsE2EEDeviceSecretPayload
		defer func() { zeroBytes(sp.PrivateKey) }()
		defer func() { zeroBytes(ap.PrivateKey) }()
		if err := decodeWindowsE2EEPayload(mr, &mp); err != nil {
			return err
		}
		if err := decodeWindowsE2EEPayload(sr, &sp); err != nil {
			return err
		}
		if err := decodeWindowsE2EEPayload(ar, &ap); err != nil {
			return err
		}
		if err := validateWindowsE2EEDevicePayloads(mp, sp, ap); err != nil {
			return err
		}
		if mp.DeviceID != deviceID {
			return ErrWindowsE2EECorrupt
		}
		lease = &WindowsE2EEDeviceIdentityLease{Metadata: makeWindowsE2EEDeviceMetadata(mp, mr.Revision), signing: newWindowsE2EESecretLease(sp.PrivateKey), agreement: newWindowsE2EESecretLease(ap.PrivateKey)}
		return nil
	})
	return lease, err
}

func (r *WindowsE2EEKeyStateRepository) PersistGroupState(installationID, groupID string, epoch uint64, previousCommitDigest, commitDigest, targetDigest string, opaqueState []byte, expectedRevision uint64, nowMS int64) (metadata WindowsE2EEGroupStateMetadata, err error) {
	err = r.withExclusiveLock(func() error {
		if err := r.requireInstallation(installationID); err != nil {
			return err
		}
		if !validWindowsE2EEIdentifier(groupID, 8, 128) || epoch == 0 || !validWindowsE2EEDigest(commitDigest) ||
			!validWindowsE2EEDigest(targetDigest) || !validWindowsE2EESecret(opaqueState, WindowsE2EEMaxOpaqueStateBytes) || nowMS <= 0 {
			return ErrWindowsE2EEInvalid
		}
		scope := "group/" + groupID
		current, err := r.loadRecord(windowsE2EEGroup, scope, installationID)
		if err != nil {
			return err
		}
		if current != nil {
			var previous windowsE2EEGroupPayload
			defer func() { zeroBytes(previous.OpaqueState) }()
			if err := decodeWindowsE2EEPayload(current, &previous); err != nil {
				return err
			}
			if err := validateWindowsE2EEGroupPayload(previous, groupID); err != nil {
				return err
			}
			if current.Revision != expectedRevision {
				return ErrWindowsE2EEConflict
			}
			if epoch <= previous.Epoch {
				return ErrWindowsE2EEStaleEpoch
			}
			if !validWindowsE2EEDigest(previousCommitDigest) || previousCommitDigest != previous.CommitDigest || previous.Epoch == math.MaxUint64 || epoch != previous.Epoch+1 {
				return ErrWindowsE2EERollbackOrClone
			}
		} else if expectedRevision != 0 {
			return ErrWindowsE2EEConflict
		} else if previousCommitDigest != "" {
			return ErrWindowsE2EERollbackOrClone
		}
		payload := windowsE2EEGroupPayload{GroupID: groupID, Epoch: epoch, CommitDigest: commitDigest, TargetSnapshotDigest: targetDigest, OpaqueState: append([]byte(nil), opaqueState...), UpdatedAtMS: nowMS}
		defer zeroBytes(payload.OpaqueState)
		revision, err := r.persistSlot(windowsE2EEGroup, scope, installationID, payload, expectedRevision, nowMS)
		if err != nil {
			return err
		}
		metadata = windowsE2EEGroupMetadata(payload, installationID, revision)
		return nil
	})
	return metadata, err
}

func (r *WindowsE2EEKeyStateRepository) LoadGroupState(installationID, groupID string) (lease *WindowsE2EEGroupStateLease, err error) {
	err = r.withExclusiveLock(func() error {
		if err := r.requireInstallation(installationID); err != nil {
			return err
		}
		if !validWindowsE2EEIdentifier(groupID, 8, 128) {
			return ErrWindowsE2EEInvalid
		}
		record, err := r.loadRecord(windowsE2EEGroup, "group/"+groupID, installationID)
		if err != nil {
			return err
		}
		if record == nil {
			return ErrWindowsE2EENotFound
		}
		var payload windowsE2EEGroupPayload
		defer func() { zeroBytes(payload.OpaqueState) }()
		if err := decodeWindowsE2EEPayload(record, &payload); err != nil {
			return err
		}
		if err := validateWindowsE2EEGroupPayload(payload, groupID); err != nil {
			return err
		}
		lease = &WindowsE2EEGroupStateLease{Metadata: windowsE2EEGroupMetadata(payload, installationID, record.Revision), state: newWindowsE2EESecretLease(payload.OpaqueState)}
		return nil
	})
	return lease, err
}

func (r *WindowsE2EEKeyStateRepository) ReserveSendGeneration(installationID, groupID, domain string, expectedRevision uint64, nowMS int64) (reservation WindowsE2EESendReservation, err error) {
	err = r.withExclusiveLock(func() error {
		if err := r.requireInstallation(installationID); err != nil {
			return err
		}
		if !validWindowsE2EEIdentifier(groupID, 8, 128) || !validWindowsE2EELabel(domain, 32) || nowMS <= 0 {
			return ErrWindowsE2EEInvalid
		}
		scope := "group/" + groupID
		record, err := r.loadRecord(windowsE2EEGroup, scope, installationID)
		if err != nil {
			return err
		}
		if record == nil {
			return ErrWindowsE2EENotFound
		}
		if record.Revision != expectedRevision {
			return ErrWindowsE2EEConflict
		}
		var payload windowsE2EEGroupPayload
		defer func() { zeroBytes(payload.OpaqueState) }()
		if err := decodeWindowsE2EEPayload(record, &payload); err != nil {
			return err
		}
		if err := validateWindowsE2EEGroupPayload(payload, groupID); err != nil {
			return err
		}
		if payload.SendGeneration == math.MaxUint64 {
			return ErrWindowsE2EEReplay
		}
		payload.SendGeneration++
		payload.UpdatedAtMS = nowMS
		revision, err := r.persistSlot(windowsE2EEGroup, scope, installationID, payload, expectedRevision, nowMS)
		if err != nil {
			return err
		}
		reservation = WindowsE2EESendReservation{GroupID: groupID, Epoch: payload.Epoch, Generation: payload.SendGeneration, Domain: domain, Revision: revision}
		return nil
	})
	return reservation, err
}

func (r *WindowsE2EEKeyStateRepository) DeleteGroupState(installationID, groupID string) error {
	return r.withExclusiveLock(func() error {
		if err := r.requireInstallation(installationID); err != nil {
			return err
		}
		if !validWindowsE2EEIdentifier(groupID, 8, 128) {
			return ErrWindowsE2EEInvalid
		}
		return r.deleteSlot(windowsE2EEGroup, "group/"+groupID)
	})
}

func (r *WindowsE2EEKeyStateRepository) StoreGrant(installationID, grantID, groupID string, firstEpoch, lastEpoch uint64, expiresAtMS int64, opaqueGrant []byte, expectedRevision uint64, nowMS int64) (metadata WindowsE2EEGrantMetadata, err error) {
	err = r.withExclusiveLock(func() error {
		if err := r.requireInstallation(installationID); err != nil {
			return err
		}
		if !validWindowsE2EEIdentifier(grantID, 8, 128) || !validWindowsE2EEIdentifier(groupID, 8, 128) || firstEpoch == 0 || lastEpoch < firstEpoch || expiresAtMS <= nowMS || nowMS <= 0 || !validWindowsE2EESecret(opaqueGrant, WindowsE2EEMaxGrantBytes) {
			return ErrWindowsE2EEInvalid
		}
		scope := "grant/" + grantID
		current, err := r.loadRecord(windowsE2EEGrant, scope, installationID)
		if err != nil {
			return err
		}
		if current != nil {
			if current.Revision != expectedRevision {
				return ErrWindowsE2EEConflict
			}
			var previous windowsE2EEGrantPayload
			defer func() { zeroBytes(previous.OpaqueGrant) }()
			if err := decodeWindowsE2EEPayload(current, &previous); err != nil {
				return err
			}
			if err := validateWindowsE2EEGrantPayload(previous, grantID); err != nil {
				return err
			}
			if previous.GroupID != groupID || previous.FirstEpoch != firstEpoch || lastEpoch < previous.LastEpoch || expiresAtMS < previous.ExpiresAtMS {
				return ErrWindowsE2EEReplay
			}
		} else if expectedRevision != 0 {
			return ErrWindowsE2EEConflict
		}
		payload := windowsE2EEGrantPayload{GrantID: grantID, GroupID: groupID, FirstEpoch: firstEpoch, LastEpoch: lastEpoch, ExpiresAtMS: expiresAtMS, OpaqueGrant: append([]byte(nil), opaqueGrant...)}
		defer zeroBytes(payload.OpaqueGrant)
		revision, err := r.persistSlot(windowsE2EEGrant, scope, installationID, payload, expectedRevision, nowMS)
		if err != nil {
			return err
		}
		metadata = windowsE2EEGrantMetadata(payload, revision)
		return nil
	})
	return metadata, err
}

func (r *WindowsE2EEKeyStateRepository) LoadGrant(installationID, grantID string, nowMS int64) (metadata WindowsE2EEGrantMetadata, lease *WindowsE2EESecretLease, err error) {
	err = r.withExclusiveLock(func() error {
		if err := r.requireInstallation(installationID); err != nil {
			return err
		}
		if !validWindowsE2EEIdentifier(grantID, 8, 128) || nowMS <= 0 {
			return ErrWindowsE2EEInvalid
		}
		record, err := r.loadRecord(windowsE2EEGrant, "grant/"+grantID, installationID)
		if err != nil {
			return err
		}
		if record == nil {
			return ErrWindowsE2EENotFound
		}
		var payload windowsE2EEGrantPayload
		defer func() { zeroBytes(payload.OpaqueGrant) }()
		if err := decodeWindowsE2EEPayload(record, &payload); err != nil {
			return err
		}
		if err := validateWindowsE2EEGrantPayload(payload, grantID); err != nil {
			return err
		}
		if payload.ExpiresAtMS <= nowMS {
			return ErrWindowsE2EEExpired
		}
		metadata, lease = windowsE2EEGrantMetadata(payload, record.Revision), newWindowsE2EESecretLease(payload.OpaqueGrant)
		return nil
	})
	return metadata, lease, err
}

func (r *WindowsE2EEKeyStateRepository) RevokeGrant(installationID, grantID string) error {
	return r.withExclusiveLock(func() error {
		if err := r.requireInstallation(installationID); err != nil {
			return err
		}
		if !validWindowsE2EEIdentifier(grantID, 8, 128) {
			return ErrWindowsE2EEInvalid
		}
		return r.deleteSlot(windowsE2EEGrant, "grant/"+grantID)
	})
}

func (r *WindowsE2EEKeyStateRepository) CacheContentKey(installationID, objectID, groupID string, epoch uint64, expiresAtMS int64, key []byte, expectedRevision uint64, nowMS int64) (revision uint64, err error) {
	err = r.withExclusiveLock(func() error {
		if err := r.requireInstallation(installationID); err != nil {
			return err
		}
		if !validWindowsE2EEIdentifier(objectID, 8, 128) || !validWindowsE2EEIdentifier(groupID, 8, 128) || epoch == 0 || expiresAtMS <= nowMS || nowMS <= 0 || !validWindowsE2EESecret(key, windowsE2EEMaxIndividualKeyBytes) {
			return ErrWindowsE2EEInvalid
		}
		current, err := r.loadRecord(windowsE2EEContentCache, "content-cache", installationID)
		if err != nil {
			return err
		}
		var payload windowsE2EEContentCachePayload
		defer func() { zeroWindowsE2EEContentEntries(payload.Entries) }()
		if current != nil {
			if current.Revision != expectedRevision {
				return ErrWindowsE2EEConflict
			}
			if err := decodeWindowsE2EEPayload(current, &payload); err != nil {
				return err
			}
			if err := validateWindowsE2EEContentEntries(payload.Entries); err != nil {
				return err
			}
		} else if expectedRevision != 0 {
			return ErrWindowsE2EEConflict
		}
		kept := payload.Entries[:0]
		for _, entry := range payload.Entries {
			if entry.ExpiresAtMS <= nowMS || entry.ObjectID == objectID {
				zeroBytes(entry.Key)
				continue
			}
			kept = append(kept, entry)
		}
		payload.Entries = append(kept, windowsE2EEContentKeyEntry{ObjectID: objectID, GroupID: groupID, Epoch: epoch, ExpiresAtMS: expiresAtMS, Key: append([]byte(nil), key...), CachedAtMS: nowMS})
		sort.Slice(payload.Entries, func(i, j int) bool {
			if payload.Entries[i].CachedAtMS != payload.Entries[j].CachedAtMS {
				return payload.Entries[i].CachedAtMS < payload.Entries[j].CachedAtMS
			}
			return payload.Entries[i].ObjectID < payload.Entries[j].ObjectID
		})
		for len(payload.Entries) > WindowsE2EEMaxCachedContentKeys || windowsE2EEContentKeyBytes(payload.Entries) > WindowsE2EEMaxCachedKeyBytes {
			zeroBytes(payload.Entries[0].Key)
			payload.Entries = payload.Entries[1:]
		}
		revision, err = r.persistSlot(windowsE2EEContentCache, "content-cache", installationID, payload, expectedRevision, nowMS)
		if err != nil {
			return err
		}
		return nil
	})
	return revision, err
}

func (r *WindowsE2EEKeyStateRepository) LoadContentKey(installationID, objectID string, nowMS int64) (metadata WindowsE2EEContentKeyMetadata, lease *WindowsE2EESecretLease, err error) {
	err = r.withExclusiveLock(func() error {
		if err := r.requireInstallation(installationID); err != nil {
			return err
		}
		if !validWindowsE2EEIdentifier(objectID, 8, 128) || nowMS <= 0 {
			return ErrWindowsE2EEInvalid
		}
		record, err := r.loadRecord(windowsE2EEContentCache, "content-cache", installationID)
		if err != nil {
			return err
		}
		if record == nil {
			return ErrWindowsE2EENotFound
		}
		var payload windowsE2EEContentCachePayload
		defer func() { zeroWindowsE2EEContentEntries(payload.Entries) }()
		if err := decodeWindowsE2EEPayload(record, &payload); err != nil {
			return err
		}
		if err := validateWindowsE2EEContentEntries(payload.Entries); err != nil {
			return err
		}
		for _, entry := range payload.Entries {
			if entry.ObjectID != objectID {
				continue
			}
			if entry.ExpiresAtMS <= nowMS {
				return ErrWindowsE2EEExpired
			}
			metadata = WindowsE2EEContentKeyMetadata{ObjectID: entry.ObjectID, GroupID: entry.GroupID, Epoch: entry.Epoch, ExpiresAtMS: entry.ExpiresAtMS}
			lease = newWindowsE2EESecretLease(entry.Key)
			return nil
		}
		return ErrWindowsE2EENotFound
	})
	return metadata, lease, err
}

func (r *WindowsE2EEKeyStateRepository) ClearContentKeyCache(installationID string) error {
	return r.withExclusiveLock(func() error {
		if err := r.requireInstallation(installationID); err != nil {
			return err
		}
		return r.deleteSlot(windowsE2EEContentCache, "content-cache")
	})
}

func (r *WindowsE2EEKeyStateRepository) withExclusiveLock(body func() error) error {
	lockPath := filepath.Join(r.dir, "repository.lock")
	processRelease := globalWindowsE2EEKeyStateLocks.acquire(lockPath)
	defer processRelease()
	if err := r.files.EnsureDir(r.dir); err != nil {
		return ErrWindowsE2EEUnavailable
	}
	handle, err := r.files.AcquireLock(lockPath)
	if err != nil {
		return ErrWindowsE2EEBusy
	}
	bodyErr := body()
	closeErr := r.files.Close(handle)
	if bodyErr != nil {
		return bodyErr
	}
	if closeErr != nil {
		return ErrWindowsE2EEUnavailable
	}
	return nil
}

func (r *WindowsE2EEKeyStateRepository) requireInstallation(installationID string) error {
	if !validWindowsE2EEDigest(installationID) {
		return ErrWindowsE2EEInvalid
	}
	mr, err := r.loadRecord(windowsE2EEDeviceMetadata, "device-metadata", installationID)
	if err != nil {
		return err
	}
	sr, err := r.loadRecord(windowsE2EEDeviceSigning, "device-signing", installationID)
	if err != nil {
		return err
	}
	ar, err := r.loadRecord(windowsE2EEDeviceAgreement, "device-agreement", installationID)
	if err != nil {
		return err
	}
	if mr == nil || sr == nil || ar == nil {
		return ErrWindowsE2EERollbackOrClone
	}
	var mp windowsE2EEDevicePayload
	var sp, ap windowsE2EEDeviceSecretPayload
	defer func() { zeroBytes(sp.PrivateKey) }()
	defer func() { zeroBytes(ap.PrivateKey) }()
	if err := decodeWindowsE2EEPayload(mr, &mp); err != nil {
		return err
	}
	if err := decodeWindowsE2EEPayload(sr, &sp); err != nil {
		return err
	}
	if err := decodeWindowsE2EEPayload(ar, &ap); err != nil {
		return err
	}
	if err := validateWindowsE2EEDevicePayloads(mp, sp, ap); err != nil {
		return err
	}
	if mp.InstallationID != installationID {
		return ErrWindowsE2EERollbackOrClone
	}
	return nil
}

func (r *WindowsE2EEKeyStateRepository) persistSlot(kind windowsE2EEKind, scope, installationID string, payload any, expectedRevision uint64, nowMS int64) (uint64, error) {
	current, err := r.loadRecord(kind, scope, installationID)
	if err != nil {
		return 0, err
	}
	if current != nil {
		defer zeroBytes(current.Payload)
		if current.Revision != expectedRevision {
			return 0, ErrWindowsE2EEConflict
		}
	} else if expectedRevision != 0 {
		return 0, ErrWindowsE2EEConflict
	}
	payloadBytes, err := marshalWindowsE2EECanonical(payload)
	if err != nil {
		return 0, err
	}
	defer zeroBytes(payloadBytes)
	record := windowsE2EERecord{Version: 1, Kind: kind, InstallationID: installationID, Scope: scope, Revision: expectedRevision + 1, PayloadDigest: windowsE2EEDigest(payloadBytes), Payload: append(json.RawMessage(nil), payloadBytes...), CreatedAtMS: nowMS, UpdatedAtMS: nowMS}
	if current != nil {
		record.CreatedAtMS = current.CreatedAtMS
	}
	recordBytes, err := marshalWindowsE2EECanonical(record)
	if err != nil {
		zeroBytes(record.Payload)
		return 0, err
	}
	defer zeroBytes(record.Payload)
	defer zeroBytes(recordBytes)
	statePath, witnessPath, err := r.slotPaths(kind, scope)
	if err != nil {
		return 0, err
	}
	if err := r.writeProtectedBytes(statePath, recordBytes); err != nil {
		return 0, err
	}
	witness := windowsE2EEWitness{Version: 1, Kind: kind, InstallationID: installationID, Scope: scope, Revision: record.Revision, RecordDigest: windowsE2EEDigest(recordBytes)}
	witnessBytes, err := marshalWindowsE2EECanonical(witness)
	if err != nil {
		return 0, err
	}
	defer zeroBytes(witnessBytes)
	if err := r.writeProtectedBytes(witnessPath, witnessBytes); err != nil {
		return 0, err
	}
	verified, err := r.loadRecord(kind, scope, installationID)
	if err != nil {
		return 0, err
	}
	if verified == nil {
		return 0, ErrWindowsE2EEUnavailable
	}
	defer zeroBytes(verified.Payload)
	if !windowsE2EERecordsEqual(verified, &record) {
		return 0, ErrWindowsE2EEUnavailable
	}
	return record.Revision, nil
}

func (r *WindowsE2EEKeyStateRepository) loadRecord(kind windowsE2EEKind, scope, installationID string) (*windowsE2EERecord, error) {
	statePath, witnessPath, err := r.slotPaths(kind, scope)
	if err != nil {
		return nil, err
	}
	stateExists, err := r.files.Exists(statePath)
	if err != nil {
		return nil, ErrWindowsE2EEUnavailable
	}
	witnessExists, err := r.files.Exists(witnessPath)
	if err != nil {
		return nil, ErrWindowsE2EEUnavailable
	}
	if !stateExists && !witnessExists {
		return nil, nil
	}
	if stateExists != witnessExists {
		return nil, ErrWindowsE2EERollbackOrClone
	}
	recordBytes, err := r.readProtectedBytes(statePath)
	if err != nil {
		return nil, err
	}
	defer zeroBytes(recordBytes)
	witnessBytes, err := r.readProtectedBytes(witnessPath)
	if err != nil {
		return nil, err
	}
	defer zeroBytes(witnessBytes)
	var record windowsE2EERecord
	var witness windowsE2EEWitness
	if err := decodeWindowsE2EECanonical(recordBytes, &record); err != nil {
		zeroBytes(record.Payload)
		return nil, err
	}
	if err := decodeWindowsE2EECanonical(witnessBytes, &witness); err != nil {
		zeroBytes(record.Payload)
		return nil, err
	}
	if record.Version != 1 || witness.Version != 1 || record.Kind != kind || witness.Kind != kind || record.Scope != scope || witness.Scope != scope || record.InstallationID != witness.InstallationID || (installationID != "" && record.InstallationID != installationID) || record.Revision == 0 || record.Revision != witness.Revision || record.PayloadDigest != windowsE2EEDigest(record.Payload) || witness.RecordDigest != windowsE2EEDigest(recordBytes) || record.CreatedAtMS <= 0 || record.UpdatedAtMS < record.CreatedAtMS {
		zeroBytes(record.Payload)
		return nil, ErrWindowsE2EERollbackOrClone
	}
	return &record, nil
}

func (r *WindowsE2EEKeyStateRepository) deleteSlot(kind windowsE2EEKind, scope string) error {
	statePath, witnessPath, err := r.slotPaths(kind, scope)
	if err != nil {
		return err
	}
	if err := r.files.Delete(statePath); err != nil {
		return ErrWindowsE2EEUnavailable
	}
	if err := r.files.Delete(witnessPath); err != nil {
		return ErrWindowsE2EEUnavailable
	}
	return nil
}

func (r *WindowsE2EEKeyStateRepository) slotPaths(kind windowsE2EEKind, scope string) (string, string, error) {
	if !validWindowsE2EEIdentifier(scope, 3, 256) {
		return "", "", ErrWindowsE2EEInvalid
	}
	token := windowsE2EEDigest([]byte(scope))
	return filepath.Join(r.dir, "state-"+string(kind)+"-"+token+".dpapi"), filepath.Join(r.dir, "witness-"+string(kind)+"-"+token+".dpapi"), nil
}

func (r *WindowsE2EEKeyStateRepository) writeProtectedBytes(path string, payload []byte) error {
	if len(payload) == 0 || len(payload) > windowsE2EEMaxPlaintextBytes {
		return ErrWindowsE2EECorrupt
	}
	if err := r.files.EnsureDir(filepath.Dir(path)); err != nil {
		return ErrWindowsE2EEUnavailable
	}
	r.cleanupTemps(path)
	plaintext, err := encodeWindowsE2EEEnvelope(payload)
	if err != nil {
		return err
	}
	defer zeroBytes(plaintext)
	suffix := make([]byte, 8)
	if err := r.readRandom(suffix); err != nil {
		return err
	}
	tempPath := path + ".tmp." + hex.EncodeToString(suffix)
	zeroBytes(suffix)
	handle, err := r.files.Open(tempPath, secureOpenSpec{Access: fileGenericWrite, Share: fileShareNone, Disposition: fileCreateNew, Flags: fileAttributeNormal | fileFlagWriteThrough})
	if err != nil {
		return ErrWindowsE2EEUnavailable
	}
	ciphertext, protectErr := r.protector.Protect(plaintext)
	if protectErr != nil || len(ciphertext) == 0 || len(ciphertext) > windowsE2EEMaxCiphertextBytes {
		_ = r.files.Close(handle)
		_ = r.files.Delete(tempPath)
		zeroBytes(ciphertext)
		return ErrWindowsE2EEUnavailable
	}
	written := 0
	for written < len(ciphertext) {
		n, writeErr := r.files.Write(handle, ciphertext[written:])
		if writeErr != nil || n <= 0 || n > len(ciphertext)-written {
			_ = r.files.Close(handle)
			_ = r.files.Delete(tempPath)
			zeroBytes(ciphertext)
			return ErrWindowsE2EEUnavailable
		}
		written += n
	}
	zeroBytes(ciphertext)
	if err := r.files.Flush(handle); err != nil {
		_ = r.files.Close(handle)
		_ = r.files.Delete(tempPath)
		return ErrWindowsE2EEUnavailable
	}
	if err := r.files.Close(handle); err != nil {
		_ = r.files.Delete(tempPath)
		return ErrWindowsE2EEUnavailable
	}
	if err := r.files.Move(tempPath, path, moveReplaceExisting|moveWriteThrough); err != nil {
		_ = r.files.Delete(tempPath)
		return ErrWindowsE2EEUnavailable
	}
	actual, err := r.readProtectedBytes(path)
	if err != nil {
		return err
	}
	defer zeroBytes(actual)
	if !bytes.Equal(actual, payload) {
		return ErrWindowsE2EEUnavailable
	}
	return nil
}

func (r *WindowsE2EEKeyStateRepository) readProtectedBytes(path string) ([]byte, error) {
	handle, err := r.files.Open(path, secureOpenSpec{Access: fileGenericRead, Share: fileShareNone, Disposition: fileOpenExisting, Flags: fileAttributeNormal})
	if err != nil {
		return nil, ErrWindowsE2EEUnavailable
	}
	size, sizeErr := r.files.Size(handle)
	if sizeErr != nil || size <= 0 || size > windowsE2EEMaxCiphertextBytes {
		_ = r.files.Close(handle)
		return nil, ErrWindowsE2EEUnavailable
	}
	ciphertext := make([]byte, int(size))
	read := 0
	for read < len(ciphertext) {
		n, readErr := r.files.Read(handle, ciphertext[read:])
		if readErr != nil || n <= 0 || n > len(ciphertext)-read {
			_ = r.files.Close(handle)
			zeroBytes(ciphertext)
			return nil, ErrWindowsE2EEUnavailable
		}
		read += n
	}
	if err := r.files.Close(handle); err != nil {
		zeroBytes(ciphertext)
		return nil, ErrWindowsE2EEUnavailable
	}
	plaintext, err := r.protector.Unprotect(ciphertext)
	zeroBytes(ciphertext)
	if err != nil {
		zeroBytes(plaintext)
		return nil, ErrWindowsE2EEUnavailable
	}
	payload, err := decodeWindowsE2EEEnvelope(plaintext)
	zeroBytes(plaintext)
	if err != nil {
		return nil, err
	}
	return payload, nil
}

func (r *WindowsE2EEKeyStateRepository) readRandom(target []byte) error {
	r.randomMu.Lock()
	_, err := io.ReadFull(r.random, target)
	r.randomMu.Unlock()
	if err != nil {
		zeroBytes(target)
		return ErrWindowsE2EEUnavailable
	}
	return nil
}

func (r *WindowsE2EEKeyStateRepository) cleanupTemps(path string) {
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
		if _, err := hex.DecodeString(suffix); err == nil {
			_ = r.files.Delete(filepath.Join(filepath.Dir(path), name))
		}
	}
}

func encodeWindowsE2EEEnvelope(payload []byte) ([]byte, error) {
	if len(payload) == 0 || len(payload) > windowsE2EEMaxPlaintextBytes {
		return nil, ErrWindowsE2EECorrupt
	}
	result := make([]byte, windowsE2EEEnvelopeHeaderBytes+len(payload))
	copy(result[:4], windowsE2EEEnvelopeMagic[:])
	result[4] = windowsE2EEEnvelopeVersion
	binary.LittleEndian.PutUint32(result[5:9], uint32(len(payload)))
	copy(result[9:], payload)
	return result, nil
}

func decodeWindowsE2EEEnvelope(value []byte) ([]byte, error) {
	if len(value) < windowsE2EEEnvelopeHeaderBytes || !bytes.Equal(value[:4], windowsE2EEEnvelopeMagic[:]) || value[4] != windowsE2EEEnvelopeVersion {
		return nil, ErrWindowsE2EECorrupt
	}
	length := binary.LittleEndian.Uint32(value[5:9])
	if length == 0 || length > windowsE2EEMaxPlaintextBytes || int(length) != len(value)-windowsE2EEEnvelopeHeaderBytes {
		return nil, ErrWindowsE2EECorrupt
	}
	return append([]byte(nil), value[windowsE2EEEnvelopeHeaderBytes:]...), nil
}

func marshalWindowsE2EECanonical(value any) ([]byte, error) {
	result, err := json.Marshal(value)
	if err != nil || len(result) == 0 || len(result) > windowsE2EEMaxPlaintextBytes {
		zeroBytes(result)
		return nil, ErrWindowsE2EECorrupt
	}
	return result, nil
}

func decodeWindowsE2EECanonical(value []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(value))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return ErrWindowsE2EECorrupt
	}
	var extra any
	if !errors.Is(decoder.Decode(&extra), io.EOF) {
		return ErrWindowsE2EECorrupt
	}
	canonical, err := marshalWindowsE2EECanonical(target)
	if err != nil {
		return err
	}
	defer zeroBytes(canonical)
	if !bytes.Equal(canonical, value) {
		return ErrWindowsE2EECorrupt
	}
	return nil
}

func decodeWindowsE2EEPayload(record *windowsE2EERecord, target any) error {
	defer zeroBytes(record.Payload)
	return decodeWindowsE2EECanonical(record.Payload, target)
}

func validateWindowsE2EEDevicePayloads(metadata windowsE2EEDevicePayload, signing, agreement windowsE2EEDeviceSecretPayload) error {
	if !validWindowsE2EEIdentifier(metadata.DeviceID, 8, 128) || !validWindowsE2EEDigest(metadata.InstallationID) || !validWindowsE2EELabel(metadata.KeyFormat, 64) || metadata.CreatedAtMS <= 0 ||
		signing.DeviceID != metadata.DeviceID || agreement.DeviceID != metadata.DeviceID || signing.InstallationID != metadata.InstallationID || agreement.InstallationID != metadata.InstallationID || signing.KeyFormat != metadata.KeyFormat || agreement.KeyFormat != metadata.KeyFormat || signing.CreatedAtMS != metadata.CreatedAtMS || agreement.CreatedAtMS != metadata.CreatedAtMS || signing.Role != "signing" || agreement.Role != "agreement" || !validWindowsE2EESecret(signing.PrivateKey, WindowsE2EEMaxPrivateKeyBytes) || !validWindowsE2EESecret(agreement.PrivateKey, WindowsE2EEMaxPrivateKeyBytes) {
		return ErrWindowsE2EERollbackOrClone
	}
	return nil
}

func validateWindowsE2EEGroupPayload(payload windowsE2EEGroupPayload, groupID string) error {
	if payload.GroupID != groupID || !validWindowsE2EEIdentifier(payload.GroupID, 8, 128) || payload.Epoch == 0 || !validWindowsE2EEDigest(payload.CommitDigest) || !validWindowsE2EEDigest(payload.TargetSnapshotDigest) || !validWindowsE2EESecret(payload.OpaqueState, WindowsE2EEMaxOpaqueStateBytes) || payload.UpdatedAtMS <= 0 {
		return ErrWindowsE2EERollbackOrClone
	}
	return nil
}

func validateWindowsE2EEGrantPayload(payload windowsE2EEGrantPayload, grantID string) error {
	if payload.GrantID != grantID || !validWindowsE2EEIdentifier(payload.GrantID, 8, 128) || !validWindowsE2EEIdentifier(payload.GroupID, 8, 128) || payload.FirstEpoch == 0 || payload.LastEpoch < payload.FirstEpoch || payload.ExpiresAtMS <= 0 || !validWindowsE2EESecret(payload.OpaqueGrant, WindowsE2EEMaxGrantBytes) {
		return ErrWindowsE2EERollbackOrClone
	}
	return nil
}

func validateWindowsE2EEContentEntries(entries []windowsE2EEContentKeyEntry) error {
	if len(entries) > WindowsE2EEMaxCachedContentKeys || windowsE2EEContentKeyBytes(entries) > WindowsE2EEMaxCachedKeyBytes {
		return ErrWindowsE2EERollbackOrClone
	}
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if seen[entry.ObjectID] || !validWindowsE2EEIdentifier(entry.ObjectID, 8, 128) || !validWindowsE2EEIdentifier(entry.GroupID, 8, 128) || entry.Epoch == 0 || entry.CachedAtMS <= 0 || entry.ExpiresAtMS <= entry.CachedAtMS || !validWindowsE2EESecret(entry.Key, windowsE2EEMaxIndividualKeyBytes) {
			return ErrWindowsE2EERollbackOrClone
		}
		seen[entry.ObjectID] = true
	}
	return nil
}

func validWindowsE2EEIdentifier(value string, minimum, maximum int) bool {
	if len(value) < minimum || len(value) > maximum {
		return false
	}
	for _, c := range []byte(value) {
		if (c >= '0' && c <= '9') || (c >= 'A' && c <= 'Z') || (c >= 'a' && c <= 'z') || c == '-' || c == '.' || c == '/' || c == '_' {
			continue
		}
		return false
	}
	return true
}

func validWindowsE2EELabel(value string, maximum int) bool {
	return validWindowsE2EEIdentifier(value, 1, maximum) && !strings.Contains(value, "/")
}

func validWindowsE2EEDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, c := range []byte(value) {
		if !((c >= '0' && c <= '9') || (c >= 'a' && c <= 'f')) {
			return false
		}
	}
	return true
}

func validWindowsE2EESecret(value []byte, maximum int) bool {
	return len(value) > 0 && len(value) <= maximum
}
func windowsE2EEDigest(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func makeWindowsE2EEDeviceMetadata(payload windowsE2EEDevicePayload, revision uint64) WindowsE2EEDeviceIdentityMetadata {
	return WindowsE2EEDeviceIdentityMetadata{DeviceID: payload.DeviceID, InstallationID: payload.InstallationID, KeyFormat: payload.KeyFormat, Revision: revision, CreatedAtMS: payload.CreatedAtMS}
}

func windowsE2EEGroupMetadata(payload windowsE2EEGroupPayload, installationID string, revision uint64) WindowsE2EEGroupStateMetadata {
	return WindowsE2EEGroupStateMetadata{GroupID: payload.GroupID, InstallationID: installationID, Epoch: payload.Epoch, SendGeneration: payload.SendGeneration, CommitDigest: payload.CommitDigest, TargetSnapshotDigest: payload.TargetSnapshotDigest, Revision: revision, UpdatedAtMS: payload.UpdatedAtMS}
}

func windowsE2EEGrantMetadata(payload windowsE2EEGrantPayload, revision uint64) WindowsE2EEGrantMetadata {
	return WindowsE2EEGrantMetadata{GrantID: payload.GrantID, GroupID: payload.GroupID, FirstEpoch: payload.FirstEpoch, LastEpoch: payload.LastEpoch, ExpiresAtMS: payload.ExpiresAtMS, Revision: revision}
}

func windowsE2EEContentKeyBytes(entries []windowsE2EEContentKeyEntry) int {
	total := 0
	for _, entry := range entries {
		total += len(entry.Key)
	}
	return total
}
func zeroWindowsE2EEContentEntries(entries []windowsE2EEContentKeyEntry) {
	for i := range entries {
		zeroBytes(entries[i].Key)
	}
}

func windowsE2EERecordsEqual(left, right *windowsE2EERecord) bool {
	if left == nil || right == nil {
		return left == right
	}
	return left.Version == right.Version && left.Kind == right.Kind && left.InstallationID == right.InstallationID && left.Scope == right.Scope && left.Revision == right.Revision && left.PayloadDigest == right.PayloadDigest && bytes.Equal(left.Payload, right.Payload) && left.CreatedAtMS == right.CreatedAtMS && left.UpdatedAtMS == right.UpdatedAtMS
}

var globalWindowsE2EEKeyStateLocks keyedLockSet
