package main

// This file is a production-dark Windows protected-media playback boundary.
// It deliberately has no runtime, HTTP, capability, codec, container, or
// provider registration. The injected provider authenticates the manifest,
// key envelope, sender context, and every ciphertext record before any bytes
// can reach the existing bounded candidate decoder.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

const (
	WindowsProtectedMediaPlaybackMaximumManifestBytes  = 1 << 20
	WindowsProtectedMediaPlaybackMaximumEnvelopeBytes  = 1 << 20
	WindowsProtectedMediaPlaybackMaximumSignatureBytes = 1 << 16
)

var (
	ErrWindowsProtectedMediaPlaybackBlocked               = errors.New("protected media playback blocked")
	ErrWindowsProtectedMediaPlaybackCorruptCiphertext     = errors.New("protected media playback corrupt ciphertext")
	ErrWindowsProtectedMediaPlaybackDowngradeForbidden    = errors.New("protected media playback downgrade forbidden")
	ErrWindowsProtectedMediaPlaybackExpired               = errors.New("protected media playback expired")
	ErrWindowsProtectedMediaPlaybackForkedEpoch           = errors.New("protected media playback forked epoch")
	ErrWindowsProtectedMediaPlaybackInvalidAuthentication = errors.New("protected media playback invalid authentication")
	ErrWindowsProtectedMediaPlaybackInvalidManifest       = errors.New("protected media playback invalid manifest")
	ErrWindowsProtectedMediaPlaybackInvalidRequest        = errors.New("protected media playback invalid request")
	ErrWindowsProtectedMediaPlaybackMissingGrant          = errors.New("protected media playback missing grant")
	ErrWindowsProtectedMediaPlaybackProductionDisabled    = errors.New("protected media playback production disabled")
	ErrWindowsProtectedMediaPlaybackRevoked               = errors.New("protected media playback revoked")
	ErrWindowsProtectedMediaPlaybackTargetChanged         = errors.New("protected media playback target changed")
	ErrWindowsProtectedMediaPlaybackTransport             = errors.New("protected media playback transport failed")
)

type WindowsProtectedMediaPlaybackRequest struct {
	ObjectID                     string
	RecipientDeviceID            string
	GroupID                      string
	ExpectedGroupRevision        uint64
	ExpectedEpoch                uint64
	ExpectedGeneration           uint64
	ExpectedTargetSnapshotDigest string
	HistoryGrantID               string
	PolicyAllowed                bool
	DNDAllowed                   bool
	SenderBlocked                bool
}

type WindowsProtectedMediaPlaybackRoute struct {
	Contract              string
	Capability            string
	Suite                 string
	Container             string
	ObjectID              string
	SourceObjectID        string
	Kind                  WindowsProtectedMediaKind
	AuthorDeviceID        string
	RecipientDeviceID     string
	GroupID               string
	Epoch                 uint64
	Generation            uint64
	TargetSnapshotDigest  string
	ExpiresAtMS           int64
	ManifestDigest        string
	EncryptedManifest     []byte
	OpaqueKeyEnvelope     []byte
	AuthenticatedManifest []byte
	Signature             []byte
	StreamManifest        WindowsStreamManifest
}

type WindowsProtectedMediaPlaybackRangeRequest struct {
	ObjectID             string
	RecipientDeviceID    string
	GroupID              string
	Epoch                uint64
	Generation           uint64
	TargetSnapshotDigest string
	ManifestDigest       string
	ETag                 string
	Start                int64
	End                  int64
}

type WindowsProtectedMediaPlaybackTransport interface {
	FetchManifest(context.Context, string, string, int64) (WindowsProtectedMediaPlaybackRoute, error)
	FetchRange(context.Context, WindowsProtectedMediaPlaybackRangeRequest) ([]byte, string, error)
}

type WindowsProtectedMediaOpenLease struct {
	mu        sync.Mutex
	opaque    []byte
	destroyed bool
}

func NewWindowsProtectedMediaOpenLease(opaque []byte) *WindowsProtectedMediaOpenLease {
	lease := &WindowsProtectedMediaOpenLease{opaque: append([]byte(nil), opaque...)}
	runtime.SetFinalizer(lease, func(value *WindowsProtectedMediaOpenLease) { value.Destroy() })
	return lease
}

func (l *WindowsProtectedMediaOpenLease) String() string {
	return "WindowsProtectedMediaOpenLease{<redacted>}"
}

func (l *WindowsProtectedMediaOpenLease) GoString() string { return l.String() }

func (l *WindowsProtectedMediaOpenLease) WithOpaqueState(body func([]byte) error) error {
	if body == nil {
		return ErrWindowsProtectedMediaPlaybackInvalidRequest
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.destroyed || len(l.opaque) == 0 {
		return ErrWindowsProtectedMediaPlaybackRevoked
	}
	copyValue := append([]byte(nil), l.opaque...)
	defer zeroBytes(copyValue)
	return body(copyValue)
}

func (l *WindowsProtectedMediaOpenLease) Destroy() {
	l.mu.Lock()
	if !l.destroyed {
		zeroBytes(l.opaque)
		l.opaque = nil
		l.destroyed = true
	}
	l.mu.Unlock()
	runtime.SetFinalizer(l, nil)
}

// A selected implementation must authenticate the manifest and envelope in
// Open and authenticate every record in AuthenticateAndDecrypt. This package
// neither implements nor selects cryptography.
type WindowsProtectedMediaOpening interface {
	ProductionApproved() bool
	Open(context.Context, WindowsProtectedMediaPlaybackRoute, *WindowsE2EEDeviceIdentityLease, *WindowsE2EEGroupStateLease, *WindowsE2EESecretLease) (*WindowsProtectedMediaOpenLease, error)
	AuthenticateAndDecrypt(context.Context, []byte, WindowsStreamChunk, WindowsProtectedMediaPlaybackRoute, *WindowsProtectedMediaOpenLease) ([]byte, error)
}

type WindowsProtectedMediaPlaybackOptions struct {
	KeyState                *WindowsE2EEKeyStateRepository
	Opener                  WindowsProtectedMediaOpening
	Transport               WindowsProtectedMediaPlaybackTransport
	CiphertextCacheRoot     string
	CacheInstallationSecret []byte
}

type windowsProtectedMediaAuthorization struct {
	mu        sync.Mutex
	available bool
}

func newWindowsProtectedMediaAuthorization() *windowsProtectedMediaAuthorization {
	return &windowsProtectedMediaAuthorization{available: true}
}

func (a *windowsProtectedMediaAuthorization) require() error {
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.available {
		return ErrWindowsProtectedMediaPlaybackRevoked
	}
	return nil
}

func (a *windowsProtectedMediaAuthorization) revoke() {
	a.mu.Lock()
	a.available = false
	a.mu.Unlock()
}

type windowsProtectedMediaRangeAdapter struct {
	route         WindowsProtectedMediaPlaybackRoute
	transport     WindowsProtectedMediaPlaybackTransport
	authorization *windowsProtectedMediaAuthorization
}

func (a *windowsProtectedMediaRangeAdapter) FetchRange(
	ctx context.Context, path, etag string, start, end int64,
) ([]byte, string, error) {
	if err := a.authorization.require(); err != nil {
		return nil, "", windowsStreamFailure("fetch", "revoked")
	}
	if path != a.route.StreamManifest.VariantURL || etag != a.route.StreamManifest.ETag ||
		!windowsProtectedMediaHasRange(a.route.StreamManifest, start, end) {
		return nil, "", windowsStreamFailure("fetch", "invalid_range")
	}
	body, returnedETag, err := a.transport.FetchRange(ctx, WindowsProtectedMediaPlaybackRangeRequest{
		ObjectID: a.route.ObjectID, RecipientDeviceID: a.route.RecipientDeviceID,
		GroupID: a.route.GroupID, Epoch: a.route.Epoch, Generation: a.route.Generation,
		TargetSnapshotDigest: a.route.TargetSnapshotDigest,
		ManifestDigest:       a.route.ManifestDigest, ETag: etag, Start: start, End: end,
	})
	if err != nil {
		if errors.Is(err, ErrWindowsProtectedMediaPlaybackRevoked) ||
			errors.Is(err, ErrWindowsProtectedMediaPlaybackExpired) ||
			errors.Is(err, ErrWindowsProtectedMediaPlaybackBlocked) {
			a.authorization.revoke()
			return nil, "", windowsStreamFailure("fetch", "revoked")
		}
		if errors.Is(err, context.Canceled) {
			return nil, "", err
		}
		return nil, "", windowsStreamFailure("fetch", "network_failed")
	}
	if err := a.authorization.require(); err != nil {
		zeroBytes(body)
		return nil, "", windowsStreamFailure("fetch", "revoked")
	}
	return body, returnedETag, nil
}

type windowsProtectedMediaChunkReader struct {
	route          WindowsProtectedMediaPlaybackRoute
	cache          *WindowsStreamChunkCache
	opener         WindowsProtectedMediaOpening
	lease          *WindowsProtectedMediaOpenLease
	authorization  *windowsProtectedMediaAuthorization
	revalidate     func() error
	revocationPath string
	closeOnce      sync.Once
}

func (r *windowsProtectedMediaChunkReader) Manifest() WindowsStreamManifest {
	return r.route.StreamManifest
}

func (r *windowsProtectedMediaChunkReader) ChunkForTime(positionMS int64) int {
	return r.route.StreamManifest.ChunkForTime(positionMS)
}

func (r *windowsProtectedMediaChunkReader) ReadChunk(ctx context.Context, index int) ([]byte, error) {
	if ctx == nil || index < 0 || index >= len(r.route.StreamManifest.Chunks) {
		return nil, windowsStreamFailure("protected_media", "invalid_manifest")
	}
	if err := r.authorizeAndRevalidate(); err != nil {
		return nil, r.fail(err)
	}
	ciphertext, err := r.cache.Get(ctx, r.route.StreamManifest, index)
	if err != nil {
		stage, code := windowsStreamFailureCode(err)
		if code == "chunk_hash_mismatch" || code == "whole_hash_mismatch" {
			return nil, r.fail(ErrWindowsProtectedMediaPlaybackCorruptCiphertext)
		}
		if code == "revoked" {
			return nil, r.fail(ErrWindowsProtectedMediaPlaybackRevoked)
		}
		return nil, windowsStreamFailure(stage, code)
	}
	if err := r.authorizeAndRevalidate(); err != nil {
		zeroBytes(ciphertext)
		return nil, r.fail(err)
	}
	plaintext, err := r.opener.AuthenticateAndDecrypt(
		ctx, ciphertext, r.route.StreamManifest.Chunks[index],
		cloneWindowsProtectedMediaPlaybackRoute(r.route), r.lease,
	)
	ownedPlaintext := append([]byte(nil), plaintext...)
	zeroBytes(plaintext)
	zeroBytes(ciphertext)
	if err != nil {
		zeroBytes(ownedPlaintext)
		return nil, r.fail(ErrWindowsProtectedMediaPlaybackInvalidAuthentication)
	}
	if err := r.authorizeAndRevalidate(); err != nil {
		zeroBytes(ownedPlaintext)
		return nil, r.fail(err)
	}
	if len(ownedPlaintext) == 0 || len(ownedPlaintext) > int(windowsStreamMaximumChunkBytes) {
		zeroBytes(ownedPlaintext)
		return nil, r.fail(ErrWindowsProtectedMediaPlaybackInvalidAuthentication)
	}
	pins := []int{index}
	if index+1 < len(r.route.StreamManifest.Chunks) {
		pins = append(pins, index+1)
	}
	if err := r.cache.SetPinned(r.route.StreamManifest, pins); err != nil {
		zeroBytes(ownedPlaintext)
		return nil, err
	}
	return ownedPlaintext, nil
}

func (r *windowsProtectedMediaChunkReader) VerifyWhole() error {
	if err := r.authorizeAndRevalidate(); err != nil {
		return r.fail(err)
	}
	if err := r.cache.VerifyWhole(r.route.StreamManifest); err != nil {
		stage, code := windowsStreamFailureCode(err)
		if code == "chunk_hash_mismatch" || code == "whole_hash_mismatch" {
			return r.fail(ErrWindowsProtectedMediaPlaybackCorruptCiphertext)
		}
		return windowsStreamFailure(stage, code)
	}
	if err := r.authorizeAndRevalidate(); err != nil {
		return r.fail(err)
	}
	return nil
}

func (r *windowsProtectedMediaChunkReader) authorizeAndRevalidate() error {
	if err := r.authorization.require(); err != nil {
		return err
	}
	if _, err := os.Lstat(r.revocationPath); err == nil {
		return ErrWindowsProtectedMediaPlaybackRevoked
	} else if !errors.Is(err, os.ErrNotExist) {
		return ErrWindowsProtectedMediaPlaybackRevoked
	}
	return r.revalidate()
}

func (r *windowsProtectedMediaChunkReader) fail(err error) error {
	code := windowsProtectedMediaPlaybackCode(err)
	switch {
	case errors.Is(err, ErrWindowsProtectedMediaPlaybackRevoked),
		errors.Is(err, ErrWindowsProtectedMediaPlaybackBlocked):
		r.authorization.revoke()
		_ = r.markRevoked()
		_ = r.cache.Tombstone(r.route.StreamManifest)
	case errors.Is(err, ErrWindowsProtectedMediaPlaybackExpired):
		r.authorization.revoke()
		_ = r.markRevoked()
		_ = r.cache.Tombstone(r.route.StreamManifest)
	case errors.Is(err, ErrWindowsProtectedMediaPlaybackInvalidAuthentication),
		errors.Is(err, ErrWindowsProtectedMediaPlaybackCorruptCiphertext),
		errors.Is(err, ErrWindowsProtectedMediaPlaybackTargetChanged):
		_ = r.cache.Invalidate(r.route.StreamManifest)
	}
	return windowsStreamFailure("protected_media", code)
}

func (r *windowsProtectedMediaChunkReader) markRevoked() error {
	if err := os.MkdirAll(filepath.Dir(r.revocationPath), 0o700); err != nil {
		return windowsStreamFailure("cache", "cache_unavailable")
	}
	file, err := os.OpenFile(r.revocationPath, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if errors.Is(err, os.ErrExist) {
		return nil
	}
	if err != nil {
		return windowsStreamFailure("cache", "cache_unavailable")
	}
	writeErr := func() error {
		if _, err := file.Write([]byte("revoked-v1\n")); err != nil {
			return err
		}
		return file.Sync()
	}()
	closeErr := file.Close()
	if writeErr != nil || closeErr != nil {
		return windowsStreamFailure("cache", "cache_unavailable")
	}
	return nil
}

func (r *windowsProtectedMediaChunkReader) Revoke() error {
	r.authorization.revoke()
	r.lease.Destroy()
	if err := r.markRevoked(); err != nil {
		return err
	}
	return r.cache.Tombstone(r.route.StreamManifest)
}

func (r *windowsProtectedMediaChunkReader) Close() {
	r.closeOnce.Do(func() {
		r.authorization.revoke()
		r.lease.Destroy()
	})
}

type WindowsProtectedMediaPreparedPlayback struct {
	Route  WindowsProtectedMediaPlaybackRoute
	Cache  *WindowsStreamChunkCache
	Chunks WindowsStreamChunkReader
	reader *windowsProtectedMediaChunkReader
}

func (p *WindowsProtectedMediaPreparedPlayback) MakeCandidatePlayer(
	decoder WindowsStreamCandidateDecoder, clock deadlineClock, send func(string, any),
) (*WindowsStreamCandidatePlayer, error) {
	if p == nil || p.reader == nil {
		return nil, windowsStreamFailure("player", "invalid_configuration")
	}
	return newWindowsStreamCandidatePlayer(p.Cache, decoder, clock, p.reader, send)
}

func (p *WindowsProtectedMediaPreparedPlayback) Revoke() error {
	if p == nil || p.reader == nil {
		return nil
	}
	return p.reader.Revoke()
}

func (p *WindowsProtectedMediaPreparedPlayback) Close() {
	if p != nil && p.reader != nil {
		p.reader.Close()
	}
}

type WindowsProtectedMediaPlaybackService struct {
	keyState      *WindowsE2EEKeyStateRepository
	opener        WindowsProtectedMediaOpening
	transport     WindowsProtectedMediaPlaybackTransport
	cacheRoot     string
	cacheSecret   []byte
	currentTimeMS func() int64
	fixtureMode   bool
}

func NewWindowsProtectedMediaPlaybackService(options WindowsProtectedMediaPlaybackOptions) (*WindowsProtectedMediaPlaybackService, error) {
	return newWindowsProtectedMediaPlaybackService(options, false, nowMS)
}

// Repository-only fixture constructor. Runtime composition is forbidden from
// using it and acceptance scans the non-test tree for call sites.
func newWindowsProtectedMediaPlaybackServiceForAudit(
	options WindowsProtectedMediaPlaybackOptions, currentTimeMS func() int64,
) (*WindowsProtectedMediaPlaybackService, error) {
	return newWindowsProtectedMediaPlaybackService(options, true, currentTimeMS)
}

func newWindowsProtectedMediaPlaybackService(
	options WindowsProtectedMediaPlaybackOptions, fixture bool, currentTimeMS func() int64,
) (*WindowsProtectedMediaPlaybackService, error) {
	if options.KeyState == nil || options.Opener == nil || options.Transport == nil ||
		options.CiphertextCacheRoot == "" || len(options.CacheInstallationSecret) < 16 || currentTimeMS == nil {
		return nil, ErrWindowsProtectedMediaPlaybackInvalidRequest
	}
	root, err := canonicalDirectory(options.CiphertextCacheRoot, true)
	if err != nil {
		return nil, ErrWindowsProtectedMediaPlaybackInvalidRequest
	}
	return &WindowsProtectedMediaPlaybackService{
		keyState: options.KeyState, opener: options.Opener, transport: options.Transport,
		cacheRoot: root, cacheSecret: append([]byte(nil), options.CacheInstallationSecret...),
		currentTimeMS: currentTimeMS, fixtureMode: fixture,
	}, nil
}

func (s *WindowsProtectedMediaPlaybackService) Prepare(
	ctx context.Context, request WindowsProtectedMediaPlaybackRequest, now int64,
) (*WindowsProtectedMediaPreparedPlayback, error) {
	if ctx == nil {
		return nil, ErrWindowsProtectedMediaPlaybackInvalidRequest
	}
	if !s.opener.ProductionApproved() && !s.fixtureMode {
		return nil, ErrWindowsProtectedMediaPlaybackProductionDisabled
	}
	if err := validateWindowsProtectedMediaPlaybackRequest(request, now); err != nil {
		return nil, err
	}
	identity, err := s.keyState.LoadDeviceIdentity(request.RecipientDeviceID)
	if err != nil {
		return nil, ErrWindowsProtectedMediaPlaybackInvalidAuthentication
	}
	defer identity.Destroy()
	group, err := s.keyState.LoadGroupState(identity.Metadata.InstallationID, request.GroupID)
	if err != nil {
		return nil, ErrWindowsProtectedMediaPlaybackInvalidAuthentication
	}
	defer group.Destroy()
	if group.Metadata.Revision != request.ExpectedGroupRevision {
		return nil, ErrWindowsProtectedMediaPlaybackTargetChanged
	}
	route, err := s.transport.FetchManifest(ctx, request.ObjectID, request.RecipientDeviceID, now)
	if err != nil {
		if windowsProtectedMediaPlaybackKnown(err) {
			return nil, err
		}
		return nil, ErrWindowsProtectedMediaPlaybackTransport
	}
	route = cloneWindowsProtectedMediaPlaybackRoute(route)
	if err := validateWindowsProtectedMediaPlaybackRoute(route, request, group.Metadata, now); err != nil {
		_ = s.purge(route, windowsProtectedMediaPlaybackPermanent(err))
		return nil, err
	}
	var grant *WindowsE2EESecretLease
	if route.Epoch < group.Metadata.Epoch {
		if request.HistoryGrantID == "" {
			_ = s.purge(route, false)
			return nil, ErrWindowsProtectedMediaPlaybackMissingGrant
		}
		metadata, loaded, loadErr := s.keyState.LoadGrant(identity.Metadata.InstallationID, request.HistoryGrantID, now)
		if loadErr != nil || metadata.GroupID != route.GroupID || metadata.FirstEpoch > route.Epoch || metadata.LastEpoch < route.Epoch {
			if loaded != nil {
				loaded.Destroy()
			}
			_ = s.purge(route, false)
			return nil, ErrWindowsProtectedMediaPlaybackMissingGrant
		}
		grant = loaded
		defer grant.Destroy()
	}
	lease, err := s.opener.Open(
		ctx, cloneWindowsProtectedMediaPlaybackRoute(route), identity, group, grant,
	)
	if err != nil || lease == nil {
		_ = s.purge(route, false)
		return nil, ErrWindowsProtectedMediaPlaybackInvalidAuthentication
	}
	authorization := newWindowsProtectedMediaAuthorization()
	adapter := &windowsProtectedMediaRangeAdapter{route: route, transport: s.transport, authorization: authorization}
	cache, err := NewWindowsStreamChunkCache(s.cacheRoot, s.cacheSecret, adapter)
	if err != nil {
		lease.Destroy()
		return nil, ErrWindowsProtectedMediaPlaybackInvalidRequest
	}
	installationID := identity.Metadata.InstallationID
	frozenGroup := group.Metadata
	revocationPath := windowsProtectedMediaRevocationPath(s.cacheRoot, cache, route)
	reader := &windowsProtectedMediaChunkReader{
		route: route, cache: cache, opener: s.opener, lease: lease,
		authorization: authorization, revocationPath: revocationPath,
		revalidate: func() error {
			checkedAt := s.currentTimeMS()
			if route.ExpiresAtMS <= checkedAt {
				return ErrWindowsProtectedMediaPlaybackExpired
			}
			current, loadErr := s.keyState.LoadGroupState(installationID, route.GroupID)
			if loadErr != nil {
				return ErrWindowsProtectedMediaPlaybackInvalidAuthentication
			}
			defer current.Destroy()
			if current.Metadata.Revision != request.ExpectedGroupRevision ||
				current.Metadata.Epoch != frozenGroup.Epoch ||
				current.Metadata.CommitDigest != frozenGroup.CommitDigest ||
				current.Metadata.TargetSnapshotDigest != frozenGroup.TargetSnapshotDigest {
				return ErrWindowsProtectedMediaPlaybackTargetChanged
			}
			if route.Epoch < frozenGroup.Epoch {
				metadata, liveGrant, grantErr := s.keyState.LoadGrant(installationID, request.HistoryGrantID, checkedAt)
				if grantErr != nil {
					return ErrWindowsProtectedMediaPlaybackRevoked
				}
				defer liveGrant.Destroy()
				if metadata.GroupID != route.GroupID || metadata.FirstEpoch > route.Epoch || metadata.LastEpoch < route.Epoch {
					return ErrWindowsProtectedMediaPlaybackRevoked
				}
			}
			return nil
		},
	}
	if _, err := os.Lstat(revocationPath); err == nil || (err != nil && !errors.Is(err, os.ErrNotExist)) {
		_ = cache.Tombstone(route.StreamManifest)
	}
	return &WindowsProtectedMediaPreparedPlayback{
		Route: cloneWindowsProtectedMediaPlaybackRoute(route),
		Cache: cache, Chunks: reader, reader: reader,
	}, nil
}

func (s *WindowsProtectedMediaPlaybackService) purge(route WindowsProtectedMediaPlaybackRoute, permanent bool) error {
	if validateWindowsStreamManifestShape(route.StreamManifest) != nil {
		return nil
	}
	authorization := newWindowsProtectedMediaAuthorization()
	authorization.revoke()
	adapter := &windowsProtectedMediaRangeAdapter{route: route, transport: s.transport, authorization: authorization}
	cache, err := NewWindowsStreamChunkCache(s.cacheRoot, s.cacheSecret, adapter)
	if err != nil {
		return err
	}
	reader := &windowsProtectedMediaChunkReader{
		route: route, cache: cache, lease: NewWindowsProtectedMediaOpenLease([]byte{1}),
		authorization:  authorization,
		revocationPath: windowsProtectedMediaRevocationPath(s.cacheRoot, cache, route),
		revalidate:     func() error { return ErrWindowsProtectedMediaPlaybackRevoked },
	}
	defer reader.Close()
	if permanent {
		if err := reader.markRevoked(); err != nil {
			return err
		}
		return cache.Tombstone(route.StreamManifest)
	}
	return cache.Invalidate(route.StreamManifest)
}

func validateWindowsProtectedMediaPlaybackRequest(request WindowsProtectedMediaPlaybackRequest, now int64) error {
	if !validWindowsProtectedIdentifier(request.ObjectID) || !validWindowsProtectedIdentifier(request.RecipientDeviceID) ||
		!validWindowsProtectedIdentifier(request.GroupID) || request.ExpectedGroupRevision == 0 ||
		request.ExpectedEpoch == 0 || request.ExpectedGeneration == 0 ||
		!validWindowsProtectedDigest(request.ExpectedTargetSnapshotDigest) || now <= 0 ||
		(request.HistoryGrantID != "" && !validWindowsProtectedIdentifier(request.HistoryGrantID)) {
		return ErrWindowsProtectedMediaPlaybackInvalidRequest
	}
	if !request.PolicyAllowed || !request.DNDAllowed || request.SenderBlocked {
		return ErrWindowsProtectedMediaPlaybackBlocked
	}
	return nil
}

func validateWindowsProtectedMediaPlaybackRoute(
	route WindowsProtectedMediaPlaybackRoute, request WindowsProtectedMediaPlaybackRequest,
	group WindowsE2EEGroupStateMetadata, now int64,
) error {
	if route.Contract != "e2ee-media-audit.v1" || route.Capability != "e2ee_media_v1" {
		return ErrWindowsProtectedMediaPlaybackDowngradeForbidden
	}
	if route.ExpiresAtMS <= now {
		return ErrWindowsProtectedMediaPlaybackExpired
	}
	if route.TargetSnapshotDigest != request.ExpectedTargetSnapshotDigest {
		return ErrWindowsProtectedMediaPlaybackTargetChanged
	}
	if route.Suite == "" || len(route.Suite) > 128 || route.Container == "" || len(route.Container) > 128 ||
		route.ObjectID != request.ObjectID || route.RecipientDeviceID != request.RecipientDeviceID ||
		route.GroupID != request.GroupID || route.Epoch != request.ExpectedEpoch ||
		route.Generation != request.ExpectedGeneration || !validWindowsProtectedIdentifier(route.SourceObjectID) ||
		!validWindowsProtectedIdentifier(route.AuthorDeviceID) || !validWindowsProtectedKind(route.Kind) ||
		!validWindowsProtectedDigest(route.ManifestDigest) || windowsProtectedPlaybackDigest(route.EncryptedManifest) != route.ManifestDigest ||
		len(route.EncryptedManifest) == 0 || len(route.EncryptedManifest) > WindowsProtectedMediaPlaybackMaximumManifestBytes ||
		len(route.OpaqueKeyEnvelope) == 0 || len(route.OpaqueKeyEnvelope) > WindowsProtectedMediaPlaybackMaximumEnvelopeBytes ||
		len(route.AuthenticatedManifest) == 0 || len(route.AuthenticatedManifest) > WindowsProtectedMediaPlaybackMaximumManifestBytes ||
		len(route.Signature) == 0 || len(route.Signature) > WindowsProtectedMediaPlaybackMaximumSignatureBytes ||
		validateWindowsStreamManifestShape(route.StreamManifest) != nil ||
		!strings.HasPrefix(route.StreamManifest.Identity, "svm1.protected.") ||
		!strings.HasPrefix(route.StreamManifest.VariantURL, "/v1/media/"+route.ObjectID+"/") ||
		route.StreamManifest.SizeBytes > windowsStreamCachePerVariantBytes {
		return ErrWindowsProtectedMediaPlaybackInvalidManifest
	}
	if route.Epoch > group.Epoch {
		return ErrWindowsProtectedMediaPlaybackForkedEpoch
	}
	if route.Epoch == group.Epoch && route.TargetSnapshotDigest != group.TargetSnapshotDigest {
		return ErrWindowsProtectedMediaPlaybackTargetChanged
	}
	if route.Epoch < group.Epoch && request.HistoryGrantID == "" {
		return ErrWindowsProtectedMediaPlaybackMissingGrant
	}
	return nil
}

func windowsProtectedMediaHasRange(manifest WindowsStreamManifest, start, end int64) bool {
	for _, chunk := range manifest.Chunks {
		if chunk.Start == start && chunk.End == end {
			return true
		}
	}
	return false
}

func windowsProtectedPlaybackDigest(value []byte) string {
	hash := sha256.Sum256(value)
	return hex.EncodeToString(hash[:])
}

func cloneWindowsProtectedMediaPlaybackRoute(
	route WindowsProtectedMediaPlaybackRoute,
) WindowsProtectedMediaPlaybackRoute {
	copyRoute := route
	copyRoute.EncryptedManifest = append([]byte(nil), route.EncryptedManifest...)
	copyRoute.OpaqueKeyEnvelope = append([]byte(nil), route.OpaqueKeyEnvelope...)
	copyRoute.AuthenticatedManifest = append([]byte(nil), route.AuthenticatedManifest...)
	copyRoute.Signature = append([]byte(nil), route.Signature...)
	copyRoute.StreamManifest.Chunks = append([]WindowsStreamChunk(nil), route.StreamManifest.Chunks...)
	copyRoute.StreamManifest.SeekMap = append([]WindowsStreamSeekPoint(nil), route.StreamManifest.SeekMap...)
	return copyRoute
}

func windowsProtectedMediaRevocationPath(
	root string, cache *WindowsStreamChunkCache, route WindowsProtectedMediaPlaybackRoute,
) string {
	token := cache.hmac(
		"protected-revocation", route.ObjectID, route.RecipientDeviceID, route.GroupID,
		fmt.Sprintf("%d", route.Epoch), fmt.Sprintf("%d", route.Generation),
		route.TargetSnapshotDigest, route.ManifestDigest, route.StreamManifest.Identity,
		route.StreamManifest.ETag,
	)
	return filepath.Join(root, "protected-revocations-v1", token+".revoked")
}

func windowsProtectedMediaPlaybackKnown(err error) bool {
	for _, candidate := range []error{
		ErrWindowsProtectedMediaPlaybackBlocked, ErrWindowsProtectedMediaPlaybackCorruptCiphertext,
		ErrWindowsProtectedMediaPlaybackDowngradeForbidden, ErrWindowsProtectedMediaPlaybackExpired,
		ErrWindowsProtectedMediaPlaybackForkedEpoch, ErrWindowsProtectedMediaPlaybackInvalidAuthentication,
		ErrWindowsProtectedMediaPlaybackInvalidManifest, ErrWindowsProtectedMediaPlaybackInvalidRequest,
		ErrWindowsProtectedMediaPlaybackMissingGrant, ErrWindowsProtectedMediaPlaybackProductionDisabled,
		ErrWindowsProtectedMediaPlaybackRevoked, ErrWindowsProtectedMediaPlaybackTargetChanged,
		ErrWindowsProtectedMediaPlaybackTransport,
	} {
		if errors.Is(err, candidate) {
			return true
		}
	}
	return false
}

func windowsProtectedMediaPlaybackPermanent(err error) bool {
	return errors.Is(err, ErrWindowsProtectedMediaPlaybackBlocked) ||
		errors.Is(err, ErrWindowsProtectedMediaPlaybackRevoked) ||
		errors.Is(err, ErrWindowsProtectedMediaPlaybackExpired)
}

func windowsProtectedMediaPlaybackCode(err error) string {
	switch {
	case errors.Is(err, ErrWindowsProtectedMediaPlaybackBlocked):
		return "blocked"
	case errors.Is(err, ErrWindowsProtectedMediaPlaybackCorruptCiphertext):
		return "corrupt_ciphertext"
	case errors.Is(err, ErrWindowsProtectedMediaPlaybackDowngradeForbidden):
		return "downgrade_forbidden"
	case errors.Is(err, ErrWindowsProtectedMediaPlaybackExpired):
		return "expired"
	case errors.Is(err, ErrWindowsProtectedMediaPlaybackForkedEpoch):
		return "forked_epoch"
	case errors.Is(err, ErrWindowsProtectedMediaPlaybackInvalidAuthentication):
		return "invalid_authentication"
	case errors.Is(err, ErrWindowsProtectedMediaPlaybackInvalidManifest):
		return "invalid_manifest"
	case errors.Is(err, ErrWindowsProtectedMediaPlaybackInvalidRequest):
		return "invalid_request"
	case errors.Is(err, ErrWindowsProtectedMediaPlaybackMissingGrant):
		return "missing_grant"
	case errors.Is(err, ErrWindowsProtectedMediaPlaybackProductionDisabled):
		return "production_disabled"
	case errors.Is(err, ErrWindowsProtectedMediaPlaybackRevoked):
		return "revoked"
	case errors.Is(err, ErrWindowsProtectedMediaPlaybackTargetChanged):
		return "target_changed"
	case errors.Is(err, ErrWindowsProtectedMediaPlaybackTransport):
		return "transport"
	default:
		return "internal_error"
	}
}

func (route WindowsProtectedMediaPlaybackRoute) String() string {
	return fmt.Sprintf("WindowsProtectedMediaPlaybackRoute{object:<redacted> epoch:%d generation:%d}", route.Epoch, route.Generation)
}
