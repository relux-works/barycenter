package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	windowsStreamTrackChunkBytes = int64(4 << 20)
	streamTrackDraftFileName     = "draft.v1.json"
)

var (
	ErrWindowsStreamTrackInvalid     = errors.New("windows_stream_track_invalid")
	ErrWindowsStreamTrackPersistence = errors.New("windows_stream_track_persistence")
)

type windowsStreamTrackPersistedDraft struct {
	LocalID        string `json:"local_id"`
	DisplayName    string `json:"display_name"`
	LocalByteCount int64  `json:"local_byte_count"`
	ClientMIME     string `json:"client_mime"`
	UploadOffset   int64  `json:"upload_offset"`
	MediaID        string `json:"media_id,omitempty"`
}

// WindowsStreamTrackDraftStore owns one app-private long-track draft. Intake
// copies a broker-authorized stream with a fixed buffer and atomically swaps
// metadata only after the bytes are durable. The UI never retains the source
// path or reads the whole track into memory.
type WindowsStreamTrackDraftStore struct {
	mu   sync.Mutex
	dir  string
	copy func(io.Writer, io.Reader) (int64, error)
}

func NewWindowsStreamTrackDraftStore(configDir string) (*WindowsStreamTrackDraftStore, error) {
	if strings.TrimSpace(configDir) == "" {
		return nil, ErrWindowsStreamTrackPersistence
	}
	dir := filepath.Join(configDir, "stream-track-draft")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, fmt.Errorf("%w: create draft directory", ErrWindowsStreamTrackPersistence)
	}
	return &WindowsStreamTrackDraftStore{
		dir: dir,
		copy: func(dst io.Writer, src io.Reader) (int64, error) {
			return io.CopyBuffer(dst, src, make([]byte, 64<<10))
		},
	}, nil
}

func (s *WindowsStreamTrackDraftStore) dataPath(localID string) string {
	return filepath.Join(s.dir, localID+".bin")
}

func (s *WindowsStreamTrackDraftStore) metadataPath() string {
	return filepath.Join(s.dir, streamTrackDraftFileName)
}

func (s *WindowsStreamTrackDraftStore) Import(file WindowsBrokeredAudioFile) (windowsStreamTrackPersistedDraft, error) {
	if s == nil || file.Open == nil || file.SizeBytes <= 0 || file.SizeBytes > StreamTrackMaximumFileBytes {
		if file.Release != nil {
			file.Release()
		}
		return windowsStreamTrackPersistedDraft{}, ErrWindowsStreamTrackInvalid
	}
	defer func() {
		if file.Release != nil {
			file.Release()
		}
	}()
	title := streamTrackDisplayName(file.DisplayName)
	if !validStreamTrackTitle(title) || !eligibleWindowsStreamTrackExtension(title) {
		return windowsStreamTrackPersistedDraft{}, ErrWindowsStreamTrackInvalid
	}
	source, err := file.Open()
	if err != nil || source == nil {
		return windowsStreamTrackPersistedDraft{}, ErrWindowsStreamTrackInvalid
	}
	defer source.Close()

	s.mu.Lock()
	defer s.mu.Unlock()
	localID, err := newStreamTrackLocalID()
	if err != nil {
		return windowsStreamTrackPersistedDraft{}, ErrWindowsStreamTrackPersistence
	}
	previous, _ := s.loadMetadataLocked()
	temporary, err := os.CreateTemp(s.dir, ".track-*")
	if err != nil {
		return windowsStreamTrackPersistedDraft{}, ErrWindowsStreamTrackPersistence
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		temporary.Close()
		return windowsStreamTrackPersistedDraft{}, ErrWindowsStreamTrackPersistence
	}
	written, copyErr := s.copy(temporary, io.LimitReader(source, file.SizeBytes+1))
	syncErr := temporary.Sync()
	closeErr := temporary.Close()
	if copyErr != nil || syncErr != nil || closeErr != nil || written != file.SizeBytes {
		return windowsStreamTrackPersistedDraft{}, ErrWindowsStreamTrackPersistence
	}
	if err := os.Rename(temporaryPath, s.dataPath(localID)); err != nil {
		return windowsStreamTrackPersistedDraft{}, ErrWindowsStreamTrackPersistence
	}
	draft := windowsStreamTrackPersistedDraft{
		LocalID: localID, DisplayName: title, LocalByteCount: written,
		ClientMIME: streamTrackMIME(title),
	}
	if err := s.saveLocked(draft); err != nil {
		_ = os.Remove(s.dataPath(localID))
		return windowsStreamTrackPersistedDraft{}, err
	}
	if previous.LocalID != "" && previous.LocalID != localID {
		_ = os.Remove(s.dataPath(previous.LocalID))
	}
	return draft, nil
}

func (s *WindowsStreamTrackDraftStore) Load() (*windowsStreamTrackPersistedDraft, error) {
	if s == nil {
		return nil, ErrWindowsStreamTrackPersistence
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	raw, err := os.ReadFile(s.metadataPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil || len(raw) > 4096 {
		return nil, ErrWindowsStreamTrackPersistence
	}
	var draft windowsStreamTrackPersistedDraft
	if json.Unmarshal(raw, &draft) != nil || !validPersistedStreamTrackDraft(draft) {
		return nil, ErrWindowsStreamTrackPersistence
	}
	info, err := os.Stat(s.dataPath(draft.LocalID))
	if err != nil || !info.Mode().IsRegular() || info.Size() != draft.LocalByteCount {
		return nil, ErrWindowsStreamTrackPersistence
	}
	return &draft, nil
}

func (s *WindowsStreamTrackDraftStore) Update(draft windowsStreamTrackPersistedDraft) error {
	if s == nil || !validPersistedStreamTrackDraft(draft) {
		return ErrWindowsStreamTrackPersistence
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.saveLocked(draft)
}

func (s *WindowsStreamTrackDraftStore) Delete(localID string) error {
	if s == nil || !streamTrackLocalIDPattern.MatchString(localID) {
		return ErrWindowsStreamTrackInvalid
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	current, err := s.loadMetadataLocked()
	if err != nil || current.LocalID != localID {
		return ErrWindowsStreamTrackInvalid
	}
	if err := os.Remove(s.dataPath(localID)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return ErrWindowsStreamTrackPersistence
	}
	if err := os.Remove(s.metadataPath()); err != nil && !errors.Is(err, os.ErrNotExist) {
		return ErrWindowsStreamTrackPersistence
	}
	return nil
}

func (s *WindowsStreamTrackDraftStore) saveLocked(draft windowsStreamTrackPersistedDraft) error {
	if !validPersistedStreamTrackDraft(draft) {
		return ErrWindowsStreamTrackPersistence
	}
	if err := writeJSON(s.metadataPath(), draft, 0o600); err != nil {
		return ErrWindowsStreamTrackPersistence
	}
	return nil
}

func (s *WindowsStreamTrackDraftStore) loadMetadataLocked() (windowsStreamTrackPersistedDraft, error) {
	raw, err := os.ReadFile(s.metadataPath())
	if err != nil || len(raw) > 4096 {
		return windowsStreamTrackPersistedDraft{}, ErrWindowsStreamTrackPersistence
	}
	var draft windowsStreamTrackPersistedDraft
	if json.Unmarshal(raw, &draft) != nil || !validPersistedStreamTrackDraft(draft) {
		return windowsStreamTrackPersistedDraft{}, ErrWindowsStreamTrackPersistence
	}
	return draft, nil
}

func validPersistedStreamTrackDraft(draft windowsStreamTrackPersistedDraft) bool {
	return streamTrackLocalIDPattern.MatchString(draft.LocalID) && validStreamTrackTitle(draft.DisplayName) &&
		eligibleWindowsStreamTrackExtension(draft.DisplayName) && draft.LocalByteCount > 0 &&
		draft.LocalByteCount <= StreamTrackMaximumFileBytes && draft.UploadOffset >= 0 &&
		draft.UploadOffset <= draft.LocalByteCount && (draft.MediaID == "" || validPhaseOnePublicID(draft.MediaID, "m_"))
}

func newStreamTrackLocalID() (string, error) {
	value := make([]byte, 16)
	if _, err := io.ReadFull(rand.Reader, value); err != nil {
		return "", err
	}
	return hex.EncodeToString(value), nil
}

func streamTrackDisplayName(value string) string {
	value = strings.TrimSpace(filepath.Base(value))
	if value == "." || value == "" {
		return ""
	}
	return value
}

func eligibleWindowsStreamTrackExtension(name string) bool {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".aac", ".flac", ".m4a", ".mp3", ".ogg", ".opus", ".wav":
		return true
	default:
		return false
	}
}

func streamTrackMIME(name string) string {
	switch strings.ToLower(filepath.Ext(name)) {
	case ".aac":
		return "audio/aac"
	case ".flac":
		return "audio/flac"
	case ".m4a":
		return "audio/mp4"
	case ".mp3":
		return "audio/mpeg"
	case ".ogg", ".opus":
		return "audio/ogg"
	case ".wav":
		return "audio/wav"
	default:
		return "application/octet-stream"
	}
}

type StreamTrackUploadProgress func(offset, total int64)

type WindowsStreamTrackService interface {
	CurrentPolicy(context.Context, ContentPolicyLocale) (ContentPolicyManifest, ContentPolicyGrant, error)
	AcceptPolicy(context.Context, ContentPolicyManifest) (ContentPolicyGrant, error)
	UploadTrack(context.Context, string, string, string, StreamTrackUploadProgress) (PhaseOneUploadConfirmation, error)
	DeleteMedia(context.Context, string) error
}

// WindowsStreamTrackAppClient reuses the authenticated Phase 1 transport but
// streams bounded chunks from disk. A repeated POST with the same local-ID key
// resumes at the coordinator-owned offset and cannot duplicate the media row.
type WindowsStreamTrackAppClient struct{ base *PhaseOneAppClient }

func NewWindowsStreamTrackAppClient(bundle CredentialBundle, doer HTTPDoer) (*WindowsStreamTrackAppClient, error) {
	base, err := NewPhaseOneAppClient(bundle, doer)
	if err != nil {
		return nil, err
	}
	return &WindowsStreamTrackAppClient{base: base}, nil
}

func (c *WindowsStreamTrackAppClient) CurrentPolicy(ctx context.Context, locale ContentPolicyLocale) (ContentPolicyManifest, ContentPolicyGrant, error) {
	manifest, err := c.base.ContentPolicy(ctx, locale)
	if err != nil {
		return ContentPolicyManifest{}, ContentPolicyGrant{}, err
	}
	grant, err := c.base.CurrentContentPolicyGrant(ctx)
	return manifest, grant, err
}

func (c *WindowsStreamTrackAppClient) AcceptPolicy(ctx context.Context, manifest ContentPolicyManifest) (ContentPolicyGrant, error) {
	return c.base.AcceptContentPolicy(ctx, manifest)
}

func (c *WindowsStreamTrackAppClient) DeleteMedia(ctx context.Context, mediaID string) error {
	return c.base.DeleteMedia(ctx, mediaID)
}

func (c *WindowsStreamTrackAppClient) UploadTrack(ctx context.Context, path, title, idempotencyKey string, progress StreamTrackUploadProgress) (PhaseOneUploadConfirmation, error) {
	if c == nil || c.base == nil || !validStreamTrackTitle(title) || !validPhaseOneIdempotencyKey(idempotencyKey) {
		return PhaseOneUploadConfirmation{}, phaseOneError(PhaseOneInvalidRequest)
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() <= 0 || info.Size() > StreamTrackMaximumFileBytes {
		return PhaseOneUploadConfirmation{}, phaseOneError(PhaseOneInvalidRequest)
	}
	body := struct {
		Kind               string `json:"kind"`
		Title              string `json:"title"`
		SizeBytes          int64  `json:"size_bytes"`
		RightsAcknowledged bool   `json:"rights_acknowledged"`
	}{"audio_track", title, info.Size(), true}
	raw, _, err := c.base.requestJSON(ctx, "POST", "/v1/media/uploads", c.base.token,
		map[string]string{"Idempotency-Key": idempotencyKey}, body, 200, 201)
	if err != nil {
		return PhaseOneUploadConfirmation{}, err
	}
	var session phaseOneUploadSession
	if decodePhaseOneJSON(raw, &session) != nil || !validPhaseOnePublicID(session.UploadID, "up_") ||
		!validPhaseOnePublicID(session.MediaID, "m_") || session.UploadLength != info.Size() ||
		session.UploadOffset < 0 || session.UploadOffset > session.UploadLength {
		return PhaseOneUploadConfirmation{}, phaseOneError(PhaseOneInvalidResponse)
	}
	if progress != nil {
		progress(session.UploadOffset, session.UploadLength)
	}
	if session.Status == "completed" && session.UploadOffset == session.UploadLength {
		return PhaseOneUploadConfirmation{MediaID: session.MediaID, Reused: true}, nil
	}
	if !lowerHexTokenPattern.MatchString(session.UploadToken) || session.Status != "open" {
		return PhaseOneUploadConfirmation{}, phaseOneError(PhaseOneInvalidResponse)
	}
	uploadToken := session.UploadToken
	file, err := os.Open(path)
	if err != nil {
		return PhaseOneUploadConfirmation{}, phaseOneError(PhaseOneInvalidRequest)
	}
	defer file.Close()
	for session.UploadOffset < session.UploadLength {
		length := min(windowsStreamTrackChunkBytes, session.UploadLength-session.UploadOffset)
		chunk := io.NewSectionReader(file, session.UploadOffset, length)
		raw, _, err = c.base.request(ctx, "PUT", "/v1/media/uploads/"+session.UploadID, uploadToken,
			map[string]string{"Upload-Offset": fmt.Sprint(session.UploadOffset), "Content-Type": "application/octet-stream"},
			chunk, true, 200)
		if err != nil {
			return PhaseOneUploadConfirmation{}, err
		}
		var next phaseOneUploadSession
		if decodePhaseOneJSON(raw, &next) != nil || next.UploadID != session.UploadID || next.MediaID != session.MediaID ||
			next.UploadLength != session.UploadLength || next.UploadOffset != session.UploadOffset+length ||
			(next.UploadOffset < next.UploadLength && next.Status != "open") ||
			(next.UploadOffset == next.UploadLength && next.Status != "completed") {
			return PhaseOneUploadConfirmation{}, phaseOneError(PhaseOneInvalidResponse)
		}
		session = next
		if progress != nil {
			progress(session.UploadOffset, session.UploadLength)
		}
	}
	if session.Status != "completed" {
		return PhaseOneUploadConfirmation{}, phaseOneError(PhaseOneInvalidResponse)
	}
	return PhaseOneUploadConfirmation{MediaID: session.MediaID, Reused: session.Reused}, nil
}

type WindowsStreamTrackComposition struct {
	mu       sync.RWMutex
	service  WindowsStreamTrackService
	store    *WindowsStreamTrackDraftStore
	model    *StreamTrackModel
	targets  *WindowsTargetsInboxComposition
	ctx      context.Context
	cancel   context.CancelFunc
	pending  sync.WaitGroup
	manifest ContentPolicyManifest
	selected int
	busy     bool
	outcome  string
}

func NewWindowsStreamTrackComposition(service WindowsStreamTrackService, store *WindowsStreamTrackDraftStore, targets *WindowsTargetsInboxComposition) (*WindowsStreamTrackComposition, error) {
	if service == nil || store == nil || targets == nil {
		return nil, ErrWindowsStreamTrackPersistence
	}
	ctx, cancel := context.WithCancel(context.Background())
	c := &WindowsStreamTrackComposition{service: service, store: store, targets: targets, model: NewStreamTrackModel(), ctx: ctx, cancel: cancel}
	if draft, err := store.Load(); err != nil {
		cancel()
		return nil, err
	} else if draft != nil {
		c.replaceDraft(*draft, StreamTrackDraftRetained, "")
	}
	c.Refresh()
	return c, nil
}

func newProductionWindowsStreamTrackComposition(dir string, targets *WindowsTargetsInboxComposition) (*WindowsStreamTrackComposition, error) {
	repository, err := newDefaultCredentialRepository(dir)
	if err != nil {
		return nil, err
	}
	bundle, err := repository.LoadBundle()
	if err != nil || bundle == nil {
		return nil, ErrWindowsStreamTrackPersistence
	}
	client, err := NewWindowsStreamTrackAppClient(*bundle, &http.Client{Timeout: 15 * time.Minute})
	if err != nil {
		return nil, err
	}
	store, err := NewWindowsStreamTrackDraftStore(dir)
	if err != nil {
		return nil, err
	}
	return NewWindowsStreamTrackComposition(client, store, targets)
}

func (c *WindowsStreamTrackComposition) Close() {
	if c == nil {
		return
	}
	c.cancel()
	c.pending.Wait()
}

func (c *WindowsStreamTrackComposition) Snapshot() StreamTrackSnapshot {
	if c == nil {
		return StreamTrackSnapshot{State: TargetsInboxCoordinatorError, Failure: StreamTrackServiceUnavailable}
	}
	return c.model.Snapshot()
}

func (c *WindowsStreamTrackComposition) ApplyShellSnapshot(shell *ShellSnapshot) {
	if c == nil || shell == nil {
		return
	}
	projection := c.model.Snapshot()
	targetProjection := c.targets.Snapshot().Projection
	projection.Targets = streamCapableTargets(targetProjection.Targets)
	projection.ActiveAirAvailable = false
	for _, air := range shell.Airs {
		projection.ActiveAirAvailable = projection.ActiveAirAvailable || air.Current
	}
	c.model.Replace(projection, time.Now())
	c.mu.RLock()
	shell.StreamTrack = c.model.Snapshot()
	shell.SelectedStreamTrackTarget = c.selected
	shell.StreamTrackBusy = c.busy
	shell.StreamTrackOutcome = c.outcome
	c.mu.RUnlock()
}

func (c *WindowsStreamTrackComposition) AcceptBrokeredFile(file WindowsBrokeredAudioFile) {
	if c == nil || !c.begin() {
		if file.Release != nil {
			file.Release()
		}
		return
	}
	c.pending.Add(1)
	go func() {
		defer c.pending.Done()
		defer c.end()
		draft, err := c.store.Import(file)
		if err != nil {
			c.setOutcome("draft_rejected")
			return
		}
		c.replaceDraft(draft, StreamTrackDraftRetained, "")
		c.setOutcome("draft_retained")
	}()
}

func (c *WindowsStreamTrackComposition) Refresh() {
	if c == nil || !c.begin() {
		return
	}
	c.pending.Add(1)
	go func() {
		defer c.pending.Done()
		defer c.end()
		manifest, grant, err := c.service.CurrentPolicy(c.ctx, ContentPolicyEN)
		projection := c.model.Snapshot()
		if err != nil {
			projection.State = TargetsInboxOffline
			projection.Failure = StreamTrackOffline
			projection.Actions = nil
			c.model.Replace(projection, time.Now())
			c.setOutcome("refresh_failed")
			return
		}
		c.mu.Lock()
		c.manifest = manifest
		c.mu.Unlock()
		projection.State = TargetsInboxReady
		projection.Failure = ""
		projection.ContentPolicyState = "required"
		if grant.Current && grant.TermsAccepted && grant.Version == manifest.Version && grant.PolicyHash == manifest.PolicyHash {
			projection.ContentPolicyState = "current"
		}
		projection.Actions = streamTrackUploadActions()
		if projection.Playback.Phase == "" {
			projection.Playback.Phase = StreamTrackPlaybackIdle
		}
		c.model.Replace(projection, time.Now())
		c.setOutcome("refreshed")
	}()
}

func (c *WindowsStreamTrackComposition) AcceptPolicy() {
	if c == nil || !c.begin() {
		return
	}
	c.mu.RLock()
	manifest := c.manifest
	c.mu.RUnlock()
	command, ok := c.model.BuildCommand(StreamTrackCommand{Kind: StreamTrackAcceptPolicy})
	if !ok || command.Kind == "" || manifest.Version == "" {
		c.end()
		return
	}
	c.pending.Add(1)
	go func() {
		defer c.pending.Done()
		defer c.end()
		grant, err := c.service.AcceptPolicy(c.ctx, manifest)
		projection := c.model.Snapshot()
		if err != nil || !grant.Current || grant.Version != manifest.Version || grant.PolicyHash != manifest.PolicyHash {
			projection.Failure = StreamTrackPolicyRequired
			c.model.Replace(projection, time.Now())
			c.setOutcome("policy_accept_failed")
			return
		}
		projection.ContentPolicyState = "current"
		projection.Failure = ""
		c.model.Replace(projection, time.Now())
		c.setOutcome("policy_accepted")
	}()
}

func (c *WindowsStreamTrackComposition) Upload() {
	if c == nil || !c.begin() {
		return
	}
	snapshot := c.model.Snapshot()
	if snapshot.Draft == nil {
		c.end()
		return
	}
	command, ok := c.model.BuildCommand(StreamTrackCommand{Kind: StreamTrackUpload, LocalID: snapshot.Draft.LocalID})
	if !ok || !c.model.ApplyOptimistic(command) {
		c.end()
		return
	}
	localID, title := snapshot.Draft.LocalID, snapshot.Draft.Title
	c.pending.Add(1)
	go func() {
		defer c.pending.Done()
		defer c.end()
		confirmation, err := c.service.UploadTrack(c.ctx, c.store.dataPath(localID), title, "track:"+localID, func(offset, _ int64) {
			draft, loadErr := c.store.Load()
			if loadErr == nil && draft != nil && draft.LocalID == localID {
				draft.UploadOffset = offset
				_ = c.store.Update(*draft)
				c.replaceDraft(*draft, StreamTrackDraftUploading, "")
			}
		})
		draft, loadErr := c.store.Load()
		if err != nil || loadErr != nil || draft == nil || draft.LocalID != localID {
			if draft != nil {
				c.replaceDraft(*draft, StreamTrackDraftFailed, StreamTrackServiceUnavailable)
			}
			c.setOutcome("upload_failed_draft_retained")
			return
		}
		draft.UploadOffset = draft.LocalByteCount
		draft.MediaID = confirmation.MediaID
		_ = c.store.Update(*draft)
		// Upload is authoritative, but the accepted no-go ADR supplies no
		// production variant manifest. Never manufacture ready or clip fallback.
		c.replaceDraft(*draft, StreamTrackDraftProcessing, StreamTrackVariantUnavailable)
		c.setOutcome("upload_confirmed_variant_unavailable")
	}()
}

func (c *WindowsStreamTrackComposition) Delete(confirmed bool) {
	if c == nil || !confirmed || !c.begin() {
		return
	}
	snapshot := c.model.Snapshot()
	if snapshot.Draft == nil {
		c.end()
		return
	}
	localID, mediaID := snapshot.Draft.LocalID, snapshot.Draft.MediaID
	c.pending.Add(1)
	go func() {
		defer c.pending.Done()
		defer c.end()
		if mediaID != "" {
			if err := c.service.DeleteMedia(c.ctx, mediaID); err != nil {
				c.setOutcome("delete_failed_draft_retained")
				return
			}
		}
		if err := c.store.Delete(localID); err != nil {
			c.setOutcome("delete_failed_draft_retained")
			return
		}
		projection := c.model.Snapshot()
		projection.Draft = nil
		projection.ConfirmedDeletedLocalID = localID
		c.model.Replace(projection, time.Now())
		c.setOutcome("draft_deleted")
	}()
}

func (c *WindowsStreamTrackComposition) SelectNextAudience() {
	snapshot := c.model.Snapshot()
	if snapshot.SelectedAudience == StreamTrackCurrentAir || !snapshot.ActiveAirAvailable {
		if len(snapshot.SelectedReferences) > 0 {
			c.model.SelectAudience(StreamTrackExplicit)
		}
		return
	}
	c.model.SelectAudience(StreamTrackCurrentAir)
}

func (c *WindowsStreamTrackComposition) SelectNextTarget() {
	c.mu.Lock()
	if count := len(c.model.Snapshot().Targets); count > 0 {
		c.selected = (c.selected + 1) % count
	}
	c.mu.Unlock()
}

func (c *WindowsStreamTrackComposition) ToggleSelectedTarget() {
	snapshot := c.model.Snapshot()
	c.mu.RLock()
	index := c.selected
	c.mu.RUnlock()
	if index < 0 || index >= len(snapshot.Targets) {
		return
	}
	reference := snapshot.Targets[index].Reference
	selected := append([]string(nil), snapshot.SelectedReferences...)
	for i, value := range selected {
		if value == reference {
			selected = append(selected[:i], selected[i+1:]...)
			c.model.SelectTargets(selected)
			return
		}
	}
	selected = append(selected, reference)
	if c.model.SelectTargets(selected) {
		c.model.SelectAudience(StreamTrackExplicit)
	}
}

func (c *WindowsStreamTrackComposition) SelectNextInsertion() {
	if c.model.Snapshot().SelectedInsertion == StreamTrackQueue {
		c.model.SelectInsertion(StreamTrackReplace)
	} else {
		c.model.SelectInsertion(StreamTrackQueue)
	}
}

func (c *WindowsStreamTrackComposition) Retry() {
	if c == nil {
		return
	}
	snapshot := c.model.Snapshot()
	if snapshot.Draft == nil {
		return
	}
	command, ok := c.model.BuildCommand(StreamTrackCommand{Kind: StreamTrackRetry, LocalID: snapshot.Draft.LocalID})
	if !ok || !c.model.ApplyOptimistic(command) {
		return
	}
	c.Upload()
}

func (c *WindowsStreamTrackComposition) begin() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.busy || c.ctx.Err() != nil {
		return false
	}
	c.busy = true
	return true
}

func (c *WindowsStreamTrackComposition) end() {
	c.mu.Lock()
	c.busy = false
	c.mu.Unlock()
}

func (c *WindowsStreamTrackComposition) setOutcome(value string) {
	c.mu.Lock()
	c.outcome = value
	c.mu.Unlock()
}

func (c *WindowsStreamTrackComposition) replaceDraft(draft windowsStreamTrackPersistedDraft, phase StreamTrackDraftPhase, failure StreamTrackFailure) {
	projection := c.model.Snapshot()
	projection.Draft = &StreamTrackDraft{
		LocalID: draft.LocalID, LocalByteCount: draft.LocalByteCount, RetainedLocalBytes: true,
		Title: draft.DisplayName, ClientMIME: draft.ClientMIME, Phase: phase,
		MediaID: draft.MediaID, UploadOffset: draft.UploadOffset, Failure: failure,
	}
	if phase == StreamTrackDraftProcessing {
		projection.Draft.ProcessingPercent = 0
	}
	projection.Failure = failure
	c.model.Replace(projection, time.Now())
}

func streamCapableTargets(values []TargetsInboxTargetChoice) []TargetsInboxTargetChoice {
	result := make([]TargetsInboxTargetChoice, 0, len(values))
	for _, target := range values {
		if containsString(target.Capabilities, "stream_track") {
			result = append(result, target)
		}
	}
	return result
}

func streamTrackUploadActions() []TargetsInboxActionCapability {
	label := func(action, en, ru string) TargetsInboxActionCapability {
		return TargetsInboxActionCapability{Action: action, Label: TargetsInboxLocalizedLabel{Key: "stream_track.action." + action, EN: en, RU: ru}}
	}
	return []TargetsInboxActionCapability{
		label("accept_policy", "Review and accept", "Ознакомиться и принять"),
		label("upload", "Upload track", "Загрузить трек"),
		label("retry", "Try again", "Повторить"),
		label("delete", "Delete", "Удалить"),
	}
}
