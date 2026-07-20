package main

// This file is a production-dark Windows protected-media send boundary. It is
// deliberately not referenced by main, HTTP clients, capability advertisement,
// capture, soundboard, or track composition while the E2EE provider gates are
// open. The injected provider owns codec/container/cryptographic details; this
// layer only validates, persists, resumes, and routes ciphertext.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

const (
	WindowsProtectedMediaMaximumPlaintextBytes  int64 = 64 << 20
	WindowsProtectedMediaMaximumCiphertextBytes int64 = 64 << 20
	WindowsProtectedMediaMaximumChunkBytes            = 1 << 20
	WindowsProtectedMediaMaximumChunks                = 1024
	WindowsProtectedMediaMaximumDraftLifetimeMS int64 = 24 * 60 * 60 * 1000
	WindowsProtectedMediaRecoveryLimit                = 100
)

var (
	ErrWindowsProtectedMediaBusy               = errors.New("protected media draft is busy")
	ErrWindowsProtectedMediaCancelled          = errors.New("protected media send was cancelled")
	ErrWindowsProtectedMediaInvalidArtifact    = errors.New("protected media artifact is invalid")
	ErrWindowsProtectedMediaInvalidRequest     = errors.New("protected media request is invalid")
	ErrWindowsProtectedMediaLocalCleanup       = errors.New("protected media local cleanup failed")
	ErrWindowsProtectedMediaPersistence        = errors.New("protected media ciphertext persistence failed")
	ErrWindowsProtectedMediaProductionDisabled = errors.New("protected media is disabled")
	ErrWindowsProtectedMediaQuotaExceeded      = errors.New("protected media quota exceeded")
	ErrWindowsProtectedMediaSourceUnavailable  = errors.New("protected media source is unavailable")
	ErrWindowsProtectedMediaStaleKeyState      = errors.New("protected media key state is stale")
	ErrWindowsProtectedMediaTargetChanged      = errors.New("protected media target changed")
	ErrWindowsProtectedMediaTransport          = errors.New("protected media transport failed")
	ErrWindowsProtectedMediaUnsupportedTarget  = errors.New("protected media target does not support E2EE")
)

type WindowsProtectedMediaKind string

const (
	WindowsProtectedMediaClip     WindowsProtectedMediaKind = "clip"
	WindowsProtectedMediaSavedCue WindowsProtectedMediaKind = "saved_cue"
	WindowsProtectedMediaTrack    WindowsProtectedMediaKind = "track"
)

type WindowsProtectedMediaPlaintextPolicy string

const (
	WindowsProtectedMediaUserOwnedRetain            WindowsProtectedMediaPlaintextPolicy = "user_owned_retain"
	WindowsProtectedMediaAppPrivateDeleteOnTerminal WindowsProtectedMediaPlaintextPolicy = "app_private_delete_on_terminal"
)

type WindowsProtectedMediaRecipient struct {
	DeviceID               string `json:"device_id"`
	Verified               bool   `json:"verified"`
	CurrentMember          bool   `json:"current_member"`
	SupportsProtectedMedia bool   `json:"supports_protected_media"`
}

type WindowsProtectedMediaSendRequest struct {
	DraftID                      string
	SourceObjectID               string
	SourcePath                   string
	PlaintextPolicy              WindowsProtectedMediaPlaintextPolicy
	Kind                         WindowsProtectedMediaKind
	AuthorDeviceID               string
	GroupID                      string
	ExpectedGroupRevision        uint64
	ExpectedTargetSnapshotDigest string
	Recipients                   []WindowsProtectedMediaRecipient
	DeclaredDurationMS           int64
	RightsConfirmed              bool
	TargetConfirmed              bool
	ExpiresAtMS                  int64
}

type WindowsProtectedMediaSealContext struct {
	DraftID              string                    `json:"draft_id"`
	SourceObjectID       string                    `json:"source_object_id"`
	Kind                 WindowsProtectedMediaKind `json:"kind"`
	GroupID              string                    `json:"group_id"`
	Epoch                uint64                    `json:"epoch"`
	Generation           uint64                    `json:"generation"`
	TargetSnapshotDigest string                    `json:"target_snapshot_digest"`
	RecipientDeviceIDs   []string                  `json:"recipient_device_ids"`
	DeclaredDurationMS   int64                     `json:"declared_duration_ms"`
	ExpiresAtMS          int64                     `json:"expires_at_ms"`
}

func (c WindowsProtectedMediaSealContext) equal(other WindowsProtectedMediaSealContext) bool {
	return c.DraftID == other.DraftID && c.SourceObjectID == other.SourceObjectID && c.Kind == other.Kind &&
		c.GroupID == other.GroupID && c.Epoch == other.Epoch && c.Generation == other.Generation &&
		c.TargetSnapshotDigest == other.TargetSnapshotDigest && c.DeclaredDurationMS == other.DeclaredDurationMS &&
		c.ExpiresAtMS == other.ExpiresAtMS && slicesEqual(c.RecipientDeviceIDs, other.RecipientDeviceIDs)
}

type WindowsProtectedMediaCiphertextChunk struct {
	Nonce      string
	Ciphertext []byte
}

type WindowsProtectedMediaSealedArtifact struct {
	Contract              string
	Capability            string
	Suite                 string
	Container             string
	Context               WindowsProtectedMediaSealContext
	EncryptedManifest     []byte
	OpaqueKeyEnvelopes    []byte
	AuthenticatedManifest []byte
	Signature             []byte
	Chunks                []WindowsProtectedMediaCiphertextChunk
}

type WindowsProtectedMediaSealer interface {
	ProductionApproved() bool
	Seal(context.Context, []byte, WindowsProtectedMediaSealContext, *WindowsE2EEDeviceIdentityLease, *WindowsE2EEGroupStateLease) (WindowsProtectedMediaSealedArtifact, error)
	Verify(context.Context, WindowsProtectedMediaSealedArtifact) bool
}

type WindowsProtectedMediaStageRequest struct {
	IdempotencyKey        string
	SourceObjectID        string
	Kind                  WindowsProtectedMediaKind
	AuthorDeviceID        string
	GroupID               string
	Epoch                 uint64
	Generation            uint64
	TargetSnapshotDigest  string
	ManifestDigest        string
	CiphertextDigest      string
	CiphertextSize        int64
	ChunkCount            int
	DeclaredDurationMS    int64
	EncryptedManifest     []byte
	OpaqueKeyEnvelopes    []byte
	AuthenticatedManifest []byte
	Signature             []byte
}

type WindowsProtectedMediaRemoteObject struct {
	ObjectID string `json:"object_id"`
	Revision uint64 `json:"revision"`
}

type WindowsProtectedMediaUploader interface {
	// Every operation is idempotent for its key and exact bytes/revision.
	// Reusing a key with different input must fail closed.
	Stage(context.Context, WindowsProtectedMediaStageRequest) (WindowsProtectedMediaRemoteObject, error)
	PutChunk(context.Context, string, string, int, int64, string, []byte) error
	Finalize(context.Context, string, string, uint64) (WindowsProtectedMediaRemoteObject, error)
	Delete(context.Context, string, string, uint64) error
}

type WindowsProtectedMediaProgressPhase string

const (
	WindowsProtectedMediaPreparing  WindowsProtectedMediaProgressPhase = "preparing"
	WindowsProtectedMediaStaging    WindowsProtectedMediaProgressPhase = "staging"
	WindowsProtectedMediaUploading  WindowsProtectedMediaProgressPhase = "uploading"
	WindowsProtectedMediaFinalizing WindowsProtectedMediaProgressPhase = "finalizing"
	WindowsProtectedMediaPublished  WindowsProtectedMediaProgressPhase = "published"
)

type WindowsProtectedMediaSendProgress struct {
	Phase          WindowsProtectedMediaProgressPhase
	CompletedBytes int64
	TotalBytes     int64
}

type WindowsProtectedMediaPublication struct {
	DraftID          string
	ObjectID         string
	Revision         uint64
	Epoch            uint64
	Generation       uint64
	ManifestDigest   string
	CiphertextDigest string
}

type WindowsProtectedMediaSendOptions struct {
	KeyState           *WindowsE2EEKeyStateRepository
	Sealer             WindowsProtectedMediaSealer
	Uploader           WindowsProtectedMediaUploader
	CiphertextRoot     string
	PlaintextDraftRoot string
}

type windowsProtectedDraftPhase string

const (
	windowsProtectedPrepared   windowsProtectedDraftPhase = "prepared"
	windowsProtectedStaged     windowsProtectedDraftPhase = "staged"
	windowsProtectedUploading  windowsProtectedDraftPhase = "uploading"
	windowsProtectedFinalizing windowsProtectedDraftPhase = "finalizing"
	windowsProtectedPublished  windowsProtectedDraftPhase = "published"
)

type windowsProtectedStoredChunk struct {
	Index      int    `json:"index"`
	ByteOffset int64  `json:"byte_offset"`
	Size       int    `json:"size"`
	Digest     string `json:"digest"`
	Nonce      string `json:"nonce"`
}

type windowsProtectedStoredDraft struct {
	Version               int                                  `json:"version"`
	DraftID               string                               `json:"draft_id"`
	SourceObjectID        string                               `json:"source_object_id"`
	SourcePath            string                               `json:"source_path"`
	SourceFingerprint     string                               `json:"source_fingerprint"`
	PlaintextPolicy       WindowsProtectedMediaPlaintextPolicy `json:"plaintext_policy"`
	Kind                  WindowsProtectedMediaKind            `json:"kind"`
	AuthorDeviceID        string                               `json:"author_device_id"`
	InitialGroupRevision  uint64                               `json:"initial_group_revision"`
	CommitDigest          string                               `json:"commit_digest"`
	Context               WindowsProtectedMediaSealContext     `json:"context"`
	Contract              string                               `json:"contract"`
	Capability            string                               `json:"capability"`
	Suite                 string                               `json:"suite"`
	Container             string                               `json:"container"`
	EncryptedManifest     []byte                               `json:"encrypted_manifest"`
	OpaqueKeyEnvelopes    []byte                               `json:"opaque_key_envelopes"`
	AuthenticatedManifest []byte                               `json:"authenticated_manifest"`
	Signature             []byte                               `json:"signature"`
	ManifestDigest        string                               `json:"manifest_digest"`
	CiphertextDigest      string                               `json:"ciphertext_digest"`
	CiphertextSize        int64                                `json:"ciphertext_size"`
	Chunks                []windowsProtectedStoredChunk        `json:"chunks"`
	CreatedAtMS           int64                                `json:"created_at_ms"`
	ExpiresAtMS           int64                                `json:"expires_at_ms"`
	Phase                 windowsProtectedDraftPhase           `json:"phase"`
	NextChunkIndex        int                                  `json:"next_chunk_index"`
	Remote                *WindowsProtectedMediaRemoteObject   `json:"remote,omitempty"`
}

type WindowsProtectedMediaSendService struct {
	keyState           *WindowsE2EEKeyStateRepository
	sealer             WindowsProtectedMediaSealer
	uploader           WindowsProtectedMediaUploader
	ciphertextRoot     string
	plaintextDraftRoot string
	fixtureMode        bool
	mu                 sync.Mutex
	activeDrafts       map[string]struct{}
}

func NewWindowsProtectedMediaSendService(options WindowsProtectedMediaSendOptions) (*WindowsProtectedMediaSendService, error) {
	return newWindowsProtectedMediaSendService(options, false)
}

// Repository-only fixture constructor. Production composition must never call
// this; acceptance validation checks the complete runtime tree.
func newWindowsProtectedMediaSendServiceForAudit(options WindowsProtectedMediaSendOptions) (*WindowsProtectedMediaSendService, error) {
	return newWindowsProtectedMediaSendService(options, true)
}

func newWindowsProtectedMediaSendService(options WindowsProtectedMediaSendOptions, fixture bool) (*WindowsProtectedMediaSendService, error) {
	if options.KeyState == nil || options.Sealer == nil || options.Uploader == nil || options.CiphertextRoot == "" || options.PlaintextDraftRoot == "" {
		return nil, ErrWindowsProtectedMediaPersistence
	}
	ciphertextRoot, err := canonicalDirectory(options.CiphertextRoot, true)
	if err != nil {
		return nil, ErrWindowsProtectedMediaPersistence
	}
	plaintextRoot, err := canonicalDirectory(options.PlaintextDraftRoot, false)
	if err != nil {
		return nil, ErrWindowsProtectedMediaPersistence
	}
	return &WindowsProtectedMediaSendService{
		keyState: options.KeyState, sealer: options.Sealer, uploader: options.Uploader,
		ciphertextRoot: ciphertextRoot, plaintextDraftRoot: plaintextRoot,
		fixtureMode: fixture, activeDrafts: map[string]struct{}{},
	}, nil
}

func (s *WindowsProtectedMediaSendService) Send(ctx context.Context, request WindowsProtectedMediaSendRequest, nowMS int64, progress func(WindowsProtectedMediaSendProgress)) (WindowsProtectedMediaPublication, error) {
	if ctx == nil {
		return WindowsProtectedMediaPublication{}, ErrWindowsProtectedMediaInvalidRequest
	}
	if !s.sealer.ProductionApproved() && !s.fixtureMode {
		return WindowsProtectedMediaPublication{}, ErrWindowsProtectedMediaProductionDisabled
	}
	if err := s.validateRequest(request, nowMS); err != nil {
		return WindowsProtectedMediaPublication{}, err
	}
	if !s.acquireDraft(request.DraftID) {
		return WindowsProtectedMediaPublication{}, ErrWindowsProtectedMediaBusy
	}
	defer s.releaseDraft(request.DraftID)

	var draft windowsProtectedStoredDraft
	if exists, err := regularFileExists(s.statePath(request.DraftID)); err != nil {
		return WindowsProtectedMediaPublication{}, ErrWindowsProtectedMediaPersistence
	} else if exists {
		draft, err = s.loadDraft(request.DraftID)
		if err != nil {
			return WindowsProtectedMediaPublication{}, err
		}
		if err := s.validateResume(ctx, draft, request, nowMS); err != nil {
			return WindowsProtectedMediaPublication{}, err
		}
	} else {
		emitWindowsProtectedProgress(progress, WindowsProtectedMediaPreparing, 0, 0)
		var err error
		draft, err = s.prepare(ctx, request, nowMS)
		if err != nil {
			return WindowsProtectedMediaPublication{}, err
		}
	}
	return s.publish(ctx, draft, progress)
}

func (s *WindowsProtectedMediaSendService) Cancel(ctx context.Context, draftID string) error {
	if ctx == nil || !validWindowsProtectedToken(draftID) {
		return ErrWindowsProtectedMediaInvalidRequest
	}
	if !s.acquireDraft(draftID) {
		return ErrWindowsProtectedMediaBusy
	}
	defer s.releaseDraft(draftID)
	exists, err := regularFileExists(s.statePath(draftID))
	if err != nil {
		return ErrWindowsProtectedMediaPersistence
	}
	if !exists {
		return nil
	}
	draft, err := s.loadDraft(draftID)
	if err != nil {
		return err
	}
	if draft.Remote != nil {
		if err := s.uploader.Delete(ctx, draft.Remote.ObjectID, "windows-protected-delete-"+draftID, draft.Remote.Revision); err != nil {
			return mapWindowsProtectedTransportError(ctx, err)
		}
	}
	return s.cleanup(draft, draft.PlaintextPolicy == WindowsProtectedMediaAppPrivateDeleteOnTerminal)
}

func (s *WindowsProtectedMediaSendService) RecoverExpiredDrafts(ctx context.Context, nowMS int64, limit int) (int, error) {
	if ctx == nil || nowMS <= 0 || limit < 1 || limit > WindowsProtectedMediaRecoveryLimit {
		return 0, ErrWindowsProtectedMediaInvalidRequest
	}
	entries, err := os.ReadDir(s.ciphertextRoot)
	if err != nil {
		return 0, ErrWindowsProtectedMediaPersistence
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	removed := 0
	for _, entry := range entries {
		if removed >= limit {
			break
		}
		if !entry.IsDir() || !validWindowsProtectedToken(entry.Name()) || s.isActive(entry.Name()) {
			continue
		}
		exists, inspectErr := regularFileExists(s.statePath(entry.Name()))
		if inspectErr != nil {
			return removed, ErrWindowsProtectedMediaPersistence
		}
		if !exists {
			continue
		}
		if !s.acquireDraft(entry.Name()) {
			continue
		}
		draft, loadErr := s.loadDraft(entry.Name())
		if loadErr == nil && draft.ExpiresAtMS <= nowMS && draft.Remote != nil {
			loadErr = s.uploader.Delete(ctx, draft.Remote.ObjectID, "windows-protected-delete-"+draft.DraftID, draft.Remote.Revision)
			if loadErr != nil {
				loadErr = mapWindowsProtectedTransportError(ctx, loadErr)
			}
		}
		if loadErr == nil && draft.ExpiresAtMS <= nowMS {
			loadErr = s.cleanup(draft, draft.PlaintextPolicy == WindowsProtectedMediaAppPrivateDeleteOnTerminal)
			if loadErr == nil {
				removed++
			}
		}
		s.releaseDraft(entry.Name())
		if loadErr != nil {
			return removed, loadErr
		}
	}
	return removed, nil
}

func (s *WindowsProtectedMediaSendService) prepare(ctx context.Context, request WindowsProtectedMediaSendRequest, nowMS int64) (windowsProtectedStoredDraft, error) {
	canonicalSource, plaintext, fingerprint, err := readWindowsProtectedSource(request.SourcePath)
	if err != nil {
		return windowsProtectedStoredDraft{}, err
	}
	defer zeroBytes(plaintext)

	identity, err := s.keyState.LoadDeviceIdentity(request.AuthorDeviceID)
	if err != nil {
		return windowsProtectedStoredDraft{}, ErrWindowsProtectedMediaStaleKeyState
	}
	installationID := identity.Metadata.InstallationID
	identity.Destroy()
	group, err := s.keyState.LoadGroupState(installationID, request.GroupID)
	if err != nil {
		return windowsProtectedStoredDraft{}, ErrWindowsProtectedMediaStaleKeyState
	}
	initialMetadata := group.Metadata
	group.Destroy()
	if initialMetadata.Revision != request.ExpectedGroupRevision || initialMetadata.TargetSnapshotDigest != request.ExpectedTargetSnapshotDigest {
		return windowsProtectedStoredDraft{}, ErrWindowsProtectedMediaTargetChanged
	}
	reservation, err := s.keyState.ReserveSendGeneration(installationID, request.GroupID, "media", initialMetadata.Revision, nowMS)
	if err != nil {
		return windowsProtectedStoredDraft{}, ErrWindowsProtectedMediaStaleKeyState
	}
	currentGroup, err := s.keyState.LoadGroupState(installationID, request.GroupID)
	if err != nil {
		return windowsProtectedStoredDraft{}, ErrWindowsProtectedMediaStaleKeyState
	}
	defer currentGroup.Destroy()
	identity, err = s.keyState.LoadDeviceIdentity(request.AuthorDeviceID)
	if err != nil {
		return windowsProtectedStoredDraft{}, ErrWindowsProtectedMediaStaleKeyState
	}
	defer identity.Destroy()
	if currentGroup.Metadata.Revision != reservation.Revision || currentGroup.Metadata.Epoch != reservation.Epoch ||
		currentGroup.Metadata.TargetSnapshotDigest != request.ExpectedTargetSnapshotDigest || currentGroup.Metadata.CommitDigest != initialMetadata.CommitDigest {
		return windowsProtectedStoredDraft{}, ErrWindowsProtectedMediaStaleKeyState
	}
	contextValue := WindowsProtectedMediaSealContext{
		DraftID: request.DraftID, SourceObjectID: request.SourceObjectID, Kind: request.Kind,
		GroupID: request.GroupID, Epoch: reservation.Epoch, Generation: reservation.Generation,
		TargetSnapshotDigest: request.ExpectedTargetSnapshotDigest,
		RecipientDeviceIDs:   sortedWindowsProtectedRecipients(request.Recipients),
		DeclaredDurationMS:   request.DeclaredDurationMS, ExpiresAtMS: request.ExpiresAtMS,
	}
	artifact, err := s.sealer.Seal(ctx, plaintext, contextValue, identity, currentGroup)
	zeroBytes(plaintext)
	if err != nil {
		return windowsProtectedStoredDraft{}, mapWindowsProtectedProviderError(ctx, err)
	}
	if !s.validateArtifact(ctx, artifact, contextValue) {
		zeroWindowsProtectedArtifact(&artifact)
		return windowsProtectedStoredDraft{}, ErrWindowsProtectedMediaInvalidArtifact
	}
	draft, err := s.persistPrepared(artifact, request, canonicalSource, fingerprint, identity.Metadata.DeviceID, initialMetadata.CommitDigest, nowMS)
	zeroWindowsProtectedArtifact(&artifact)
	return draft, err
}

func (s *WindowsProtectedMediaSendService) publish(ctx context.Context, draft windowsProtectedStoredDraft, progress func(WindowsProtectedMediaSendProgress)) (WindowsProtectedMediaPublication, error) {
	stage := WindowsProtectedMediaStageRequest{
		IdempotencyKey: "windows-protected-stage-" + draft.DraftID,
		SourceObjectID: draft.SourceObjectID, Kind: draft.Kind, AuthorDeviceID: draft.AuthorDeviceID,
		GroupID: draft.Context.GroupID, Epoch: draft.Context.Epoch, Generation: draft.Context.Generation,
		TargetSnapshotDigest: draft.Context.TargetSnapshotDigest, ManifestDigest: draft.ManifestDigest,
		CiphertextDigest: draft.CiphertextDigest, CiphertextSize: draft.CiphertextSize,
		ChunkCount: len(draft.Chunks), DeclaredDurationMS: draft.Context.DeclaredDurationMS,
		EncryptedManifest: cloneBytes(draft.EncryptedManifest), OpaqueKeyEnvelopes: cloneBytes(draft.OpaqueKeyEnvelopes),
		AuthenticatedManifest: cloneBytes(draft.AuthenticatedManifest), Signature: cloneBytes(draft.Signature),
	}
	defer zeroWindowsProtectedStage(&stage)
	if draft.Phase == windowsProtectedPublished {
		if draft.Remote == nil {
			return WindowsProtectedMediaPublication{}, ErrWindowsProtectedMediaPersistence
		}
		published := *draft.Remote
		if err := s.cleanup(draft, draft.PlaintextPolicy == WindowsProtectedMediaAppPrivateDeleteOnTerminal); err != nil {
			return WindowsProtectedMediaPublication{}, err
		}
		emitWindowsProtectedProgress(progress, WindowsProtectedMediaPublished, draft.CiphertextSize, draft.CiphertextSize)
		return WindowsProtectedMediaPublication{
			DraftID: draft.DraftID, ObjectID: published.ObjectID, Revision: published.Revision,
			Epoch: draft.Context.Epoch, Generation: draft.Context.Generation,
			ManifestDigest: draft.ManifestDigest, CiphertextDigest: draft.CiphertextDigest,
		}, nil
	}
	if draft.Remote == nil {
		emitWindowsProtectedProgress(progress, WindowsProtectedMediaStaging, 0, draft.CiphertextSize)
		remote, err := s.uploader.Stage(ctx, stage)
		if err != nil {
			return WindowsProtectedMediaPublication{}, mapWindowsProtectedTransportError(ctx, err)
		}
		if !validWindowsProtectedIdentifier(remote.ObjectID) || remote.Revision == 0 {
			return WindowsProtectedMediaPublication{}, ErrWindowsProtectedMediaTransport
		}
		draft.Remote = &remote
		draft.Phase = windowsProtectedStaged
		if err := s.saveDraft(draft); err != nil {
			return WindowsProtectedMediaPublication{}, err
		}
	}
	remote := *draft.Remote
	draft.Phase = windowsProtectedUploading
	if err := s.saveDraft(draft); err != nil {
		return WindowsProtectedMediaPublication{}, err
	}
	var completed int64
	for _, chunk := range draft.Chunks[:draft.NextChunkIndex] {
		completed += int64(chunk.Size)
	}
	for _, chunk := range draft.Chunks[draft.NextChunkIndex:] {
		if err := ctx.Err(); err != nil {
			return WindowsProtectedMediaPublication{}, ErrWindowsProtectedMediaCancelled
		}
		ciphertext, err := s.loadChunk(draft.DraftID, chunk)
		if err != nil {
			return WindowsProtectedMediaPublication{}, err
		}
		uploadErr := s.uploader.PutChunk(ctx, remote.ObjectID,
			fmt.Sprintf("windows-protected-chunk-%s-%d", draft.DraftID, chunk.Index),
			chunk.Index, chunk.ByteOffset, chunk.Digest, ciphertext)
		zeroBytes(ciphertext)
		if uploadErr != nil {
			return WindowsProtectedMediaPublication{}, mapWindowsProtectedTransportError(ctx, uploadErr)
		}
		completed += int64(chunk.Size)
		draft.NextChunkIndex = chunk.Index + 1
		if err := s.saveDraft(draft); err != nil {
			return WindowsProtectedMediaPublication{}, err
		}
		emitWindowsProtectedProgress(progress, WindowsProtectedMediaUploading, completed, draft.CiphertextSize)
	}
	draft.Phase = windowsProtectedFinalizing
	if err := s.saveDraft(draft); err != nil {
		return WindowsProtectedMediaPublication{}, err
	}
	emitWindowsProtectedProgress(progress, WindowsProtectedMediaFinalizing, draft.CiphertextSize, draft.CiphertextSize)
	published, err := s.uploader.Finalize(ctx, remote.ObjectID, "windows-protected-finalize-"+draft.DraftID, remote.Revision)
	if err != nil {
		return WindowsProtectedMediaPublication{}, mapWindowsProtectedTransportError(ctx, err)
	}
	if published.ObjectID != remote.ObjectID || published.Revision <= remote.Revision {
		return WindowsProtectedMediaPublication{}, ErrWindowsProtectedMediaTransport
	}
	draft.Remote = &published
	draft.Phase = windowsProtectedPublished
	if err := s.saveDraft(draft); err != nil {
		return WindowsProtectedMediaPublication{}, err
	}
	if err := s.cleanup(draft, draft.PlaintextPolicy == WindowsProtectedMediaAppPrivateDeleteOnTerminal); err != nil {
		return WindowsProtectedMediaPublication{}, err
	}
	emitWindowsProtectedProgress(progress, WindowsProtectedMediaPublished, draft.CiphertextSize, draft.CiphertextSize)
	return WindowsProtectedMediaPublication{
		DraftID: draft.DraftID, ObjectID: published.ObjectID, Revision: published.Revision,
		Epoch: draft.Context.Epoch, Generation: draft.Context.Generation,
		ManifestDigest: draft.ManifestDigest, CiphertextDigest: draft.CiphertextDigest,
	}, nil
}

func (s *WindowsProtectedMediaSendService) validateRequest(request WindowsProtectedMediaSendRequest, nowMS int64) error {
	if !validWindowsProtectedToken(request.DraftID) || !validWindowsProtectedIdentifier(request.SourceObjectID) ||
		!validWindowsProtectedIdentifier(request.AuthorDeviceID) || !validWindowsProtectedIdentifier(request.GroupID) ||
		request.ExpectedGroupRevision == 0 || !validWindowsProtectedDigest(request.ExpectedTargetSnapshotDigest) ||
		request.DeclaredDurationMS < 0 || nowMS <= 0 || request.ExpiresAtMS <= nowMS ||
		request.ExpiresAtMS-nowMS > WindowsProtectedMediaMaximumDraftLifetimeMS || !request.RightsConfirmed ||
		!request.TargetConfirmed || len(request.Recipients) == 0 || len(request.Recipients) > 64 ||
		!validWindowsProtectedKind(request.Kind) || !validWindowsProtectedPolicy(request.PlaintextPolicy) {
		return ErrWindowsProtectedMediaInvalidRequest
	}
	seen := map[string]bool{}
	for _, recipient := range request.Recipients {
		if !validWindowsProtectedIdentifier(recipient.DeviceID) || seen[recipient.DeviceID] {
			return ErrWindowsProtectedMediaInvalidRequest
		}
		seen[recipient.DeviceID] = true
		if !recipient.CurrentMember || !recipient.Verified {
			return ErrWindowsProtectedMediaTargetChanged
		}
		if !recipient.SupportsProtectedMedia {
			return ErrWindowsProtectedMediaUnsupportedTarget
		}
	}
	if request.SourcePath == "" {
		return ErrWindowsProtectedMediaInvalidRequest
	}
	if request.PlaintextPolicy == WindowsProtectedMediaAppPrivateDeleteOnTerminal {
		canonical, err := canonicalRegularPath(request.SourcePath)
		if err != nil || !isWindowsProtectedOwned(canonical, s.plaintextDraftRoot) {
			return ErrWindowsProtectedMediaInvalidRequest
		}
	}
	return nil
}

func (s *WindowsProtectedMediaSendService) validateArtifact(ctx context.Context, artifact WindowsProtectedMediaSealedArtifact, expected WindowsProtectedMediaSealContext) bool {
	if artifact.Contract != "e2ee-media-audit.v1" || artifact.Capability != "e2ee_media_v1" || !artifact.Context.equal(expected) ||
		len(artifact.Suite) == 0 || len(artifact.Suite) > 128 || len(artifact.Container) == 0 || len(artifact.Container) > 128 ||
		len(artifact.EncryptedManifest) == 0 || len(artifact.EncryptedManifest) > 1<<20 ||
		len(artifact.OpaqueKeyEnvelopes) == 0 || len(artifact.OpaqueKeyEnvelopes) > 1<<20 ||
		len(artifact.AuthenticatedManifest) == 0 || len(artifact.AuthenticatedManifest) > 1<<20 ||
		len(artifact.Signature) == 0 || len(artifact.Signature) > 1<<16 || len(artifact.Chunks) == 0 ||
		len(artifact.Chunks) > WindowsProtectedMediaMaximumChunks {
		return false
	}
	nonces := map[string]bool{}
	var total int64
	for _, chunk := range artifact.Chunks {
		if len(chunk.Nonce) == 0 || len(chunk.Nonce) > 256 || nonces[chunk.Nonce] || len(chunk.Ciphertext) == 0 || len(chunk.Ciphertext) > WindowsProtectedMediaMaximumChunkBytes {
			return false
		}
		nonces[chunk.Nonce] = true
		total += int64(len(chunk.Ciphertext))
		if total > WindowsProtectedMediaMaximumCiphertextBytes {
			return false
		}
	}
	return s.sealer.Verify(ctx, artifact)
}

func (s *WindowsProtectedMediaSendService) persistPrepared(artifact WindowsProtectedMediaSealedArtifact, request WindowsProtectedMediaSendRequest, canonicalSource, sourceFingerprint, authorDeviceID, commitDigest string, nowMS int64) (windowsProtectedStoredDraft, error) {
	directory := s.draftDirectory(request.DraftID)
	if err := os.Mkdir(directory, 0o700); err != nil {
		return windowsProtectedStoredDraft{}, ErrWindowsProtectedMediaPersistence
	}
	failed := true
	defer func() {
		if failed {
			_ = os.RemoveAll(directory)
		}
	}()
	chunks := make([]windowsProtectedStoredChunk, 0, len(artifact.Chunks))
	whole := sha256.New()
	var offset int64
	for index, value := range artifact.Chunks {
		digest := windowsProtectedDigest(value.Ciphertext)
		metadata := windowsProtectedStoredChunk{Index: index, ByteOffset: offset, Size: len(value.Ciphertext), Digest: digest, Nonce: value.Nonce}
		if err := writeWindowsProtectedNewFile(s.chunkPath(request.DraftID, index), value.Ciphertext); err != nil {
			return windowsProtectedStoredDraft{}, ErrWindowsProtectedMediaPersistence
		}
		_, _ = whole.Write(value.Ciphertext)
		chunks = append(chunks, metadata)
		offset += int64(len(value.Ciphertext))
	}
	draft := windowsProtectedStoredDraft{
		Version: 1, DraftID: request.DraftID, SourceObjectID: request.SourceObjectID,
		SourcePath: canonicalSource, SourceFingerprint: sourceFingerprint,
		PlaintextPolicy: request.PlaintextPolicy, Kind: request.Kind, AuthorDeviceID: authorDeviceID,
		InitialGroupRevision: request.ExpectedGroupRevision, CommitDigest: commitDigest,
		Context: artifact.Context, Contract: artifact.Contract, Capability: artifact.Capability,
		Suite: artifact.Suite, Container: artifact.Container,
		EncryptedManifest: cloneBytes(artifact.EncryptedManifest), OpaqueKeyEnvelopes: cloneBytes(artifact.OpaqueKeyEnvelopes),
		AuthenticatedManifest: cloneBytes(artifact.AuthenticatedManifest), Signature: cloneBytes(artifact.Signature),
		ManifestDigest: windowsProtectedDigest(artifact.EncryptedManifest), CiphertextDigest: hex.EncodeToString(whole.Sum(nil)),
		CiphertextSize: offset, Chunks: chunks, CreatedAtMS: nowMS, ExpiresAtMS: request.ExpiresAtMS,
		Phase: windowsProtectedPrepared, NextChunkIndex: 0,
	}
	if err := s.saveDraft(draft); err != nil {
		zeroWindowsProtectedDraft(&draft)
		return windowsProtectedStoredDraft{}, err
	}
	failed = false
	return draft, nil
}

func (s *WindowsProtectedMediaSendService) validateResume(ctx context.Context, draft windowsProtectedStoredDraft, request WindowsProtectedMediaSendRequest, nowMS int64) error {
	canonicalSource, plaintext, fingerprint, err := readWindowsProtectedSource(request.SourcePath)
	if err != nil {
		return err
	}
	zeroBytes(plaintext)
	if draft.Version != 1 || draft.DraftID != request.DraftID || draft.SourceObjectID != request.SourceObjectID ||
		draft.SourcePath != canonicalSource || draft.SourceFingerprint != fingerprint || draft.PlaintextPolicy != request.PlaintextPolicy ||
		draft.Kind != request.Kind || draft.AuthorDeviceID != request.AuthorDeviceID || draft.InitialGroupRevision != request.ExpectedGroupRevision ||
		draft.Context.GroupID != request.GroupID || draft.Context.TargetSnapshotDigest != request.ExpectedTargetSnapshotDigest ||
		!slicesEqual(draft.Context.RecipientDeviceIDs, sortedWindowsProtectedRecipients(request.Recipients)) ||
		draft.Context.DeclaredDurationMS != request.DeclaredDurationMS || draft.ExpiresAtMS != request.ExpiresAtMS || draft.ExpiresAtMS <= nowMS {
		if draft.Context.TargetSnapshotDigest != request.ExpectedTargetSnapshotDigest {
			return ErrWindowsProtectedMediaTargetChanged
		}
		return ErrWindowsProtectedMediaInvalidRequest
	}
	identity, err := s.keyState.LoadDeviceIdentity(request.AuthorDeviceID)
	if err != nil {
		return ErrWindowsProtectedMediaStaleKeyState
	}
	defer identity.Destroy()
	group, err := s.keyState.LoadGroupState(identity.Metadata.InstallationID, request.GroupID)
	if err != nil {
		return ErrWindowsProtectedMediaStaleKeyState
	}
	defer group.Destroy()
	if group.Metadata.Epoch != draft.Context.Epoch || group.Metadata.CommitDigest != draft.CommitDigest || group.Metadata.SendGeneration < draft.Context.Generation {
		return ErrWindowsProtectedMediaStaleKeyState
	}
	if group.Metadata.TargetSnapshotDigest != draft.Context.TargetSnapshotDigest {
		return ErrWindowsProtectedMediaTargetChanged
	}
	artifact, err := s.artifactFromDraft(draft)
	if err != nil {
		return err
	}
	defer zeroWindowsProtectedArtifact(&artifact)
	if !s.sealer.Verify(ctx, artifact) {
		return ErrWindowsProtectedMediaInvalidArtifact
	}
	return nil
}

func (s *WindowsProtectedMediaSendService) artifactFromDraft(draft windowsProtectedStoredDraft) (WindowsProtectedMediaSealedArtifact, error) {
	chunks := make([]WindowsProtectedMediaCiphertextChunk, 0, len(draft.Chunks))
	for _, metadata := range draft.Chunks {
		value, err := s.loadChunk(draft.DraftID, metadata)
		if err != nil {
			for index := range chunks {
				zeroBytes(chunks[index].Ciphertext)
			}
			return WindowsProtectedMediaSealedArtifact{}, err
		}
		chunks = append(chunks, WindowsProtectedMediaCiphertextChunk{Nonce: metadata.Nonce, Ciphertext: value})
	}
	return WindowsProtectedMediaSealedArtifact{
		Contract: draft.Contract, Capability: draft.Capability, Suite: draft.Suite, Container: draft.Container,
		Context: draft.Context, EncryptedManifest: cloneBytes(draft.EncryptedManifest),
		OpaqueKeyEnvelopes: cloneBytes(draft.OpaqueKeyEnvelopes), AuthenticatedManifest: cloneBytes(draft.AuthenticatedManifest),
		Signature: cloneBytes(draft.Signature), Chunks: chunks,
	}, nil
}

func (s *WindowsProtectedMediaSendService) validateStoredDraft(draft windowsProtectedStoredDraft) error {
	if draft.Version != 1 || !validWindowsProtectedToken(draft.DraftID) || !validWindowsProtectedIdentifier(draft.SourceObjectID) ||
		!validWindowsProtectedIdentifier(draft.AuthorDeviceID) || draft.InitialGroupRevision == 0 || !validWindowsProtectedDigest(draft.CommitDigest) ||
		draft.Contract != "e2ee-media-audit.v1" || draft.Capability != "e2ee_media_v1" || len(draft.Suite) == 0 || len(draft.Suite) > 128 ||
		len(draft.Container) == 0 || len(draft.Container) > 128 || !validWindowsProtectedDigest(draft.SourceFingerprint) ||
		!validWindowsProtectedDigest(draft.ManifestDigest) || windowsProtectedDigest(draft.EncryptedManifest) != draft.ManifestDigest ||
		!validWindowsProtectedDigest(draft.CiphertextDigest) || len(draft.EncryptedManifest) > 1<<20 ||
		len(draft.OpaqueKeyEnvelopes) == 0 || len(draft.OpaqueKeyEnvelopes) > 1<<20 ||
		len(draft.AuthenticatedManifest) == 0 || len(draft.AuthenticatedManifest) > 1<<20 ||
		len(draft.Signature) == 0 || len(draft.Signature) > 1<<16 ||
		len(draft.Chunks) == 0 || len(draft.Chunks) > WindowsProtectedMediaMaximumChunks || draft.NextChunkIndex < 0 ||
		draft.NextChunkIndex > len(draft.Chunks) || draft.CreatedAtMS <= 0 || draft.ExpiresAtMS <= draft.CreatedAtMS ||
		draft.ExpiresAtMS-draft.CreatedAtMS > WindowsProtectedMediaMaximumDraftLifetimeMS || draft.ExpiresAtMS != draft.Context.ExpiresAtMS ||
		!validWindowsProtectedPhase(draft.Phase) || draft.Context.DraftID != draft.DraftID ||
		draft.Context.SourceObjectID != draft.SourceObjectID || draft.Context.Kind != draft.Kind ||
		!validWindowsProtectedIdentifier(draft.Context.GroupID) || draft.Context.Epoch == 0 || draft.Context.Generation == 0 ||
		!validWindowsProtectedDigest(draft.Context.TargetSnapshotDigest) || draft.Context.DeclaredDurationMS < 0 ||
		len(draft.Context.RecipientDeviceIDs) == 0 || len(draft.Context.RecipientDeviceIDs) > 64 ||
		!sort.StringsAreSorted(draft.Context.RecipientDeviceIDs) || !validWindowsProtectedStoredSourcePath(draft.SourcePath) {
		return ErrWindowsProtectedMediaPersistence
	}
	for index, deviceID := range draft.Context.RecipientDeviceIDs {
		if !validWindowsProtectedIdentifier(deviceID) || (index > 0 && draft.Context.RecipientDeviceIDs[index-1] == deviceID) {
			return ErrWindowsProtectedMediaPersistence
		}
	}
	if (draft.Phase == windowsProtectedPrepared) != (draft.Remote == nil) {
		return ErrWindowsProtectedMediaPersistence
	}
	nonces := map[string]bool{}
	whole := sha256.New()
	var offset int64
	for index, metadata := range draft.Chunks {
		if metadata.Index != index || metadata.ByteOffset != offset || metadata.Size <= 0 || metadata.Size > WindowsProtectedMediaMaximumChunkBytes ||
			len(metadata.Nonce) == 0 || len(metadata.Nonce) > 256 || nonces[metadata.Nonce] || !validWindowsProtectedDigest(metadata.Digest) {
			return ErrWindowsProtectedMediaPersistence
		}
		nonces[metadata.Nonce] = true
		value, err := s.loadChunk(draft.DraftID, metadata)
		if err != nil {
			return err
		}
		_, _ = whole.Write(value)
		zeroBytes(value)
		offset += int64(metadata.Size)
	}
	if offset != draft.CiphertextSize || offset > WindowsProtectedMediaMaximumCiphertextBytes || hex.EncodeToString(whole.Sum(nil)) != draft.CiphertextDigest {
		return ErrWindowsProtectedMediaPersistence
	}
	if draft.Remote != nil && (!validWindowsProtectedIdentifier(draft.Remote.ObjectID) || draft.Remote.Revision == 0) {
		return ErrWindowsProtectedMediaPersistence
	}
	return nil
}

func (s *WindowsProtectedMediaSendService) loadChunk(draftID string, metadata windowsProtectedStoredChunk) ([]byte, error) {
	file, err := os.Open(s.chunkPath(draftID, metadata.Index))
	if err != nil {
		return nil, ErrWindowsProtectedMediaPersistence
	}
	defer file.Close()
	value, err := io.ReadAll(io.LimitReader(file, int64(WindowsProtectedMediaMaximumChunkBytes+1)))
	if err != nil || len(value) != metadata.Size || windowsProtectedDigest(value) != metadata.Digest {
		zeroBytes(value)
		return nil, ErrWindowsProtectedMediaPersistence
	}
	return value, nil
}

func (s *WindowsProtectedMediaSendService) saveDraft(draft windowsProtectedStoredDraft) error {
	raw, err := json.Marshal(draft)
	if err != nil || len(raw) > 8<<20 {
		zeroBytes(raw)
		return ErrWindowsProtectedMediaPersistence
	}
	raw = append(raw, '\n')
	defer zeroBytes(raw)
	directory := s.draftDirectory(draft.DraftID)
	temporary, err := os.CreateTemp(directory, ".state-*.tmp")
	if err != nil {
		return ErrWindowsProtectedMediaPersistence
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return ErrWindowsProtectedMediaPersistence
	}
	if _, err := temporary.Write(raw); err != nil || temporary.Sync() != nil || temporary.Close() != nil {
		return ErrWindowsProtectedMediaPersistence
	}
	if err := replaceStateFile(temporaryPath, s.statePath(draft.DraftID)); err != nil {
		return ErrWindowsProtectedMediaPersistence
	}
	return nil
}

func (s *WindowsProtectedMediaSendService) loadDraft(draftID string) (windowsProtectedStoredDraft, error) {
	file, err := os.Open(s.statePath(draftID))
	if err != nil {
		return windowsProtectedStoredDraft{}, ErrWindowsProtectedMediaPersistence
	}
	defer file.Close()
	raw, err := io.ReadAll(io.LimitReader(file, (8<<20)+1))
	if err != nil || len(raw) == 0 || len(raw) > 8<<20 {
		zeroBytes(raw)
		return windowsProtectedStoredDraft{}, ErrWindowsProtectedMediaPersistence
	}
	defer zeroBytes(raw)
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var draft windowsProtectedStoredDraft
	if decoder.Decode(&draft) != nil || decoder.Decode(&struct{}{}) != io.EOF {
		zeroWindowsProtectedDraft(&draft)
		return windowsProtectedStoredDraft{}, ErrWindowsProtectedMediaPersistence
	}
	if err := s.validateStoredDraft(draft); err != nil {
		zeroWindowsProtectedDraft(&draft)
		return windowsProtectedStoredDraft{}, err
	}
	return draft, nil
}

func (s *WindowsProtectedMediaSendService) cleanup(draft windowsProtectedStoredDraft, deletePlaintext bool) error {
	if deletePlaintext {
		canonical, err := canonicalRegularPath(draft.SourcePath)
		if err != nil || !isWindowsProtectedOwned(canonical, s.plaintextDraftRoot) {
			return ErrWindowsProtectedMediaLocalCleanup
		}
		if err := os.Remove(canonical); err != nil && !errors.Is(err, os.ErrNotExist) {
			return ErrWindowsProtectedMediaLocalCleanup
		}
	}
	if err := os.RemoveAll(s.draftDirectory(draft.DraftID)); err != nil {
		return ErrWindowsProtectedMediaLocalCleanup
	}
	return nil
}

func (s *WindowsProtectedMediaSendService) draftDirectory(draftID string) string {
	return filepath.Join(s.ciphertextRoot, draftID)
}

func (s *WindowsProtectedMediaSendService) statePath(draftID string) string {
	return filepath.Join(s.draftDirectory(draftID), "state.json")
}

func (s *WindowsProtectedMediaSendService) chunkPath(draftID string, index int) string {
	return filepath.Join(s.draftDirectory(draftID), fmt.Sprintf("chunk-%04d.bin", index))
}

func (s *WindowsProtectedMediaSendService) acquireDraft(draftID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, exists := s.activeDrafts[draftID]; exists {
		return false
	}
	s.activeDrafts[draftID] = struct{}{}
	return true
}

func (s *WindowsProtectedMediaSendService) releaseDraft(draftID string) {
	s.mu.Lock()
	delete(s.activeDrafts, draftID)
	s.mu.Unlock()
}

func (s *WindowsProtectedMediaSendService) isActive(draftID string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, active := s.activeDrafts[draftID]
	return active
}

func emitWindowsProtectedProgress(progress func(WindowsProtectedMediaSendProgress), phase WindowsProtectedMediaProgressPhase, completed, total int64) {
	if progress != nil {
		progress(WindowsProtectedMediaSendProgress{Phase: phase, CompletedBytes: completed, TotalBytes: total})
	}
}

func mapWindowsProtectedProviderError(ctx context.Context, err error) error {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ErrWindowsProtectedMediaCancelled
	}
	return ErrWindowsProtectedMediaInvalidArtifact
}

func mapWindowsProtectedTransportError(ctx context.Context, err error) error {
	if ctx.Err() != nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ErrWindowsProtectedMediaCancelled
	}
	return ErrWindowsProtectedMediaTransport
}

func canonicalDirectory(path string, create bool) (string, error) {
	cleaned, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	if create {
		if err := os.MkdirAll(cleaned, 0o700); err != nil {
			return "", err
		}
	}
	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.IsDir() {
		return "", errors.New("not a directory")
	}
	return filepath.Clean(resolved), nil
}

func canonicalRegularPath(path string) (string, error) {
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(resolved)
	if err != nil || !info.Mode().IsRegular() {
		return "", errors.New("not a regular file")
	}
	return filepath.Clean(resolved), nil
}

func readWindowsProtectedSource(path string) (string, []byte, string, error) {
	canonical, err := canonicalRegularPath(path)
	if err != nil {
		return "", nil, "", ErrWindowsProtectedMediaSourceUnavailable
	}
	file, err := os.Open(canonical)
	if err != nil {
		return "", nil, "", ErrWindowsProtectedMediaSourceUnavailable
	}
	defer file.Close()
	value, err := io.ReadAll(io.LimitReader(file, WindowsProtectedMediaMaximumPlaintextBytes+1))
	if err != nil || len(value) == 0 || int64(len(value)) > WindowsProtectedMediaMaximumPlaintextBytes {
		zeroBytes(value)
		if int64(len(value)) > WindowsProtectedMediaMaximumPlaintextBytes {
			return "", nil, "", ErrWindowsProtectedMediaQuotaExceeded
		}
		return "", nil, "", ErrWindowsProtectedMediaSourceUnavailable
	}
	return canonical, value, windowsProtectedDigest(value), nil
}

func writeWindowsProtectedNewFile(path string, value []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	if _, err := file.Write(value); err != nil {
		file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		file.Close()
		return err
	}
	return file.Close()
}

func regularFileExists(path string) (bool, error) {
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return info.Mode().IsRegular(), nil
}

func isWindowsProtectedOwned(child, root string) bool {
	relative, err := filepath.Rel(root, child)
	return err == nil && relative != "." && relative != "" && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}

func sortedWindowsProtectedRecipients(values []WindowsProtectedMediaRecipient) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, value.DeviceID)
	}
	sort.Strings(result)
	return result
}

func validWindowsProtectedKind(value WindowsProtectedMediaKind) bool {
	return value == WindowsProtectedMediaClip || value == WindowsProtectedMediaTrack || value == WindowsProtectedMediaSavedCue
}

func validWindowsProtectedPolicy(value WindowsProtectedMediaPlaintextPolicy) bool {
	return value == WindowsProtectedMediaUserOwnedRetain || value == WindowsProtectedMediaAppPrivateDeleteOnTerminal
}

func validWindowsProtectedPhase(value windowsProtectedDraftPhase) bool {
	return value == windowsProtectedPrepared || value == windowsProtectedStaged || value == windowsProtectedUploading || value == windowsProtectedFinalizing || value == windowsProtectedPublished
}

func validWindowsProtectedToken(value string) bool {
	if len(value) < 16 || len(value) > 64 {
		return false
	}
	for _, r := range value {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '_' && r != '-' {
			return false
		}
	}
	return true
}

func validWindowsProtectedIdentifier(value string) bool {
	return len(value) >= 8 && len(value) <= 128 && !strings.ContainsAny(value, "/\\\x00")
}

func validWindowsProtectedStoredSourcePath(value string) bool {
	return value != "" && filepath.IsAbs(value) && filepath.Clean(value) == value
}

func validWindowsProtectedDigest(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if !((r >= '0' && r <= '9') || (r >= 'a' && r <= 'f')) {
			return false
		}
	}
	return true
}

func windowsProtectedDigest(value []byte) string {
	digest := sha256.Sum256(value)
	return hex.EncodeToString(digest[:])
}

func slicesEqual(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func cloneBytes(value []byte) []byte { return append([]byte(nil), value...) }

func zeroWindowsProtectedArtifact(value *WindowsProtectedMediaSealedArtifact) {
	zeroBytes(value.EncryptedManifest)
	zeroBytes(value.OpaqueKeyEnvelopes)
	zeroBytes(value.AuthenticatedManifest)
	zeroBytes(value.Signature)
	for index := range value.Chunks {
		zeroBytes(value.Chunks[index].Ciphertext)
	}
}

func zeroWindowsProtectedStage(value *WindowsProtectedMediaStageRequest) {
	zeroBytes(value.EncryptedManifest)
	zeroBytes(value.OpaqueKeyEnvelopes)
	zeroBytes(value.AuthenticatedManifest)
	zeroBytes(value.Signature)
}

func zeroWindowsProtectedDraft(value *windowsProtectedStoredDraft) {
	zeroBytes(value.EncryptedManifest)
	zeroBytes(value.OpaqueKeyEnvelopes)
	zeroBytes(value.AuthenticatedManifest)
	zeroBytes(value.Signature)
}
