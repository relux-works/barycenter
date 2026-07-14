package main

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
)

type CaptureMediaClass string

const (
	CaptureSelfTest      CaptureMediaClass = "self_test"
	CaptureUserRecording CaptureMediaClass = "user_recording"
)

type CaptureMediaState string

const (
	CaptureAbsent            CaptureMediaState = "absent"
	CapturePartial           CaptureMediaState = "capturing_partial"
	CaptureFinalizing        CaptureMediaState = "finalizing"
	CaptureSelfTestLocal     CaptureMediaState = "self_test_local"
	CaptureDurableUnsent     CaptureMediaState = "durable_unsent"
	CaptureUploading         CaptureMediaState = "uploading"
	CaptureUploadedConfirmed CaptureMediaState = "uploaded_confirmed"
	CaptureDeleted           CaptureMediaState = "deleted"
)

type CaptureMediaAction string

const (
	CaptureBegin                     CaptureMediaAction = "begin"
	CaptureStop                      CaptureMediaAction = "stop"
	CaptureFinalizeSucceeded         CaptureMediaAction = "finalize_succeeded"
	CaptureFinalizeFailed            CaptureMediaAction = "finalize_failed"
	CaptureCancel                    CaptureMediaAction = "cancel"
	CaptureClose                     CaptureMediaAction = "close"
	CaptureExplicitDelete            CaptureMediaAction = "explicit_delete"
	CaptureBeginUpload               CaptureMediaAction = "begin_upload"
	CaptureUploadFailedOrInterrupted CaptureMediaAction = "upload_failed_or_interrupted"
	CaptureUploadConfirmed           CaptureMediaAction = "upload_confirmed"
	CaptureCleanup                   CaptureMediaAction = "cleanup"
)

func NextCaptureMediaState(class CaptureMediaClass, state CaptureMediaState, action CaptureMediaAction) (CaptureMediaState, bool) {
	switch {
	case state == CaptureAbsent && action == CaptureBegin:
		return CapturePartial, true
	case state == CapturePartial && action == CaptureStop:
		return CaptureFinalizing, true
	case class == CaptureSelfTest && state == CaptureFinalizing && action == CaptureFinalizeSucceeded:
		return CaptureSelfTestLocal, true
	case class == CaptureUserRecording && state == CaptureFinalizing && action == CaptureFinalizeSucceeded:
		return CaptureDurableUnsent, true
	case state == CapturePartial && action == CaptureCancel,
		state == CaptureFinalizing && action == CaptureFinalizeFailed:
		return CaptureDeleted, true
	case class == CaptureSelfTest && state == CaptureSelfTestLocal &&
		(action == CaptureClose || action == CaptureExplicitDelete):
		return CaptureDeleted, true
	case class == CaptureUserRecording && state == CaptureDurableUnsent && action == CaptureBeginUpload:
		return CaptureUploading, true
	case class == CaptureUserRecording && state == CaptureUploading && action == CaptureUploadFailedOrInterrupted:
		return CaptureDurableUnsent, true
	case class == CaptureUserRecording && state == CaptureUploading && action == CaptureUploadConfirmed:
		return CaptureUploadedConfirmed, true
	case class == CaptureUserRecording && state == CaptureUploadedConfirmed && action == CaptureCleanup,
		class == CaptureUserRecording && state == CaptureDurableUnsent && action == CaptureExplicitDelete:
		return CaptureDeleted, true
	default:
		return "", false
	}
}

type RecordingCuePhase string

const (
	CueIdle             RecordingCuePhase = "idle"
	CuePlayingStart     RecordingCuePhase = "playing_start_cue"
	CueCapturing        RecordingCuePhase = "capturing"
	CueClosingCapture   RecordingCuePhase = "closing_capture"
	CuePlayingStop      RecordingCuePhase = "playing_stop_cue"
	CueComplete         RecordingCuePhase = "complete"
	CueCancelled        RecordingCuePhase = "cancelled"
	CommandPlayStartCue                   = "play_start_cue"
	CommandEnableCommit                   = "enable_microphone_commit"
	CommandCloseCapture                   = "disable_microphone_commit_and_close_capture"
	CommandPlayStopCue                    = "play_stop_cue"
	CommandComplete                       = "complete"
)

type RecordingCueSequencer struct{ Phase RecordingCuePhase }

func NewRecordingCueSequencer() RecordingCueSequencer {
	return RecordingCueSequencer{Phase: CueIdle}
}

func (s RecordingCueSequencer) MayCommitMicrophoneSamples() bool { return s.Phase == CueCapturing }

func (s *RecordingCueSequencer) Begin() (string, bool) {
	if s.Phase != CueIdle {
		return "", false
	}
	s.Phase = CuePlayingStart
	return CommandPlayStartCue, true
}

func (s *RecordingCueSequencer) StartCueCompleted() (string, bool) {
	if s.Phase != CuePlayingStart {
		return "", false
	}
	s.Phase = CueCapturing
	return CommandEnableCommit, true
}

func (s *RecordingCueSequencer) StopRequested() (string, bool) {
	if s.Phase != CueCapturing {
		return "", false
	}
	s.Phase = CueClosingCapture
	return CommandCloseCapture, true
}

func (s *RecordingCueSequencer) CaptureClosed() (string, bool) {
	if s.Phase != CueClosingCapture {
		return "", false
	}
	s.Phase = CuePlayingStop
	return CommandPlayStopCue, true
}

func (s *RecordingCueSequencer) StopCueCompleted() (string, bool) {
	if s.Phase != CuePlayingStop {
		return "", false
	}
	s.Phase = CueComplete
	return CommandComplete, true
}

func (s *RecordingCueSequencer) Cancel() { s.Phase = CueCancelled }

const (
	BuiltinRecordingCueID         = "pulsar.recording-cue.v1"
	BuiltinRecordingCueFilename   = "pulsar-recording-cue.wav"
	BuiltinRecordingCueSHA256     = "479b1a9d605ac12454e3449e129991b7ce8599251506ca54a93be0b6144730fd"
	BuiltinRecordingCueByteCount  = 15_404
	BuiltinRecordingCueSampleRate = 48_000
	BuiltinRecordingCueFrames     = 7_680
)

func ValidateBuiltinRecordingCue(data []byte) bool {
	if len(data) != BuiltinRecordingCueByteCount || string(data[0:4]) != "RIFF" ||
		string(data[8:12]) != "WAVE" || binary.LittleEndian.Uint16(data[20:22]) != 1 ||
		binary.LittleEndian.Uint16(data[22:24]) != 1 ||
		binary.LittleEndian.Uint32(data[24:28]) != BuiltinRecordingCueSampleRate ||
		binary.LittleEndian.Uint16(data[34:36]) != 16 || string(data[36:40]) != "data" ||
		int(binary.LittleEndian.Uint32(data[40:44])) != len(data)-44 {
		return false
	}
	digest := sha256.Sum256(data)
	return fmt.Sprintf("%x", digest) == BuiltinRecordingCueSHA256
}

func LoadBuiltinRecordingCueFromPackageRoot(packageRoot string) ([]byte, error) {
	data, err := os.ReadFile(filepath.Join(packageRoot, "Assets", "Audio", BuiltinRecordingCueFilename))
	if err != nil || !ValidateBuiltinRecordingCue(data) {
		return nil, ErrRecordingCueUnavailable
	}
	return data, nil
}

var (
	ErrRecordingCueUnavailable  = errors.New("recording_cue_unavailable")
	ErrCaptureInvalidIdentifier = errors.New("capture_media_invalid_identifier")
	ErrCaptureInvalidState      = errors.New("capture_media_invalid_state")
	ErrCaptureInvalidWAV        = errors.New("capture_media_invalid_wav")
	ErrCaptureStorage           = errors.New("capture_media_storage")
)

type CaptureMediaHandle struct {
	ID    string
	Class CaptureMediaClass
	State CaptureMediaState
	Path  string
}

type CaptureMediaRecovery struct {
	RetainedDrafts           []CaptureMediaHandle
	DeletedPartialCount      int
	DeletedSelfTestCount     int
	DeletedInvalidDraftCount int
}

type CaptureMediaStore struct {
	root  string
	mu    sync.Mutex
	newID func() (string, error)
}

func NewCaptureMediaStore(root string) *CaptureMediaStore {
	return &CaptureMediaStore{root: filepath.Clean(root), newID: randomCaptureMediaID}
}

func (s *CaptureMediaStore) Begin(class CaptureMediaClass) (CaptureMediaHandle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if class != CaptureSelfTest && class != CaptureUserRecording {
		return CaptureMediaHandle{}, ErrCaptureInvalidState
	}
	if err := s.prepareDirectories(); err != nil {
		return CaptureMediaHandle{}, err
	}
	id, err := s.newID()
	if err != nil || !isCaptureMediaID(id) {
		return CaptureMediaHandle{}, ErrCaptureInvalidIdentifier
	}
	path := filepath.Join(s.partialDirectory(), id+".partial.wav")
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return CaptureMediaHandle{}, ErrCaptureStorage
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(path)
		return CaptureMediaHandle{}, ErrCaptureStorage
	}
	return CaptureMediaHandle{ID: id, Class: class, State: CapturePartial, Path: path}, nil
}

func (s *CaptureMediaStore) Stop(handle CaptureMediaHandle) (CaptureMediaHandle, error) {
	if handle.State != CapturePartial || !isCaptureMediaID(handle.ID) ||
		filepath.Clean(handle.Path) != s.expectedPath(handle) {
		return CaptureMediaHandle{}, ErrCaptureInvalidState
	}
	handle.State = CaptureFinalizing
	return handle, nil
}

// ImportUserDraft copies picker-approved bytes into an opaque app-private
// partial. The UI owns and closes any external access token before calling it.
func (s *CaptureMediaStore) ImportUserDraft(source io.Reader) (CaptureMediaHandle, error) {
	partial, err := s.Begin(CaptureUserRecording)
	if err != nil {
		return CaptureMediaHandle{}, err
	}
	file, err := os.OpenFile(partial.Path, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err == nil {
		_, err = io.Copy(file, source)
	}
	if closeErr := func() error {
		if file == nil {
			return nil
		}
		return file.Close()
	}(); err == nil {
		err = closeErr
	}
	if err != nil {
		_ = s.Cancel(partial)
		return CaptureMediaHandle{}, ErrCaptureStorage
	}
	finalizing, err := s.Stop(partial)
	if err != nil {
		_ = s.Cancel(partial)
		return CaptureMediaHandle{}, err
	}
	return s.Finalize(finalizing)
}

func (s *CaptureMediaStore) Finalize(handle CaptureMediaHandle) (CaptureMediaHandle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if handle.State != CaptureFinalizing || !isCaptureMediaID(handle.ID) ||
		filepath.Clean(handle.Path) != filepath.Join(s.partialDirectory(), handle.ID+".partial.wav") {
		return CaptureMediaHandle{}, ErrCaptureInvalidState
	}
	data, err := os.ReadFile(handle.Path)
	if err != nil {
		_ = os.Remove(handle.Path)
		return CaptureMediaHandle{}, ErrCaptureStorage
	}
	if !isStructurallyCompleteWAV(data) {
		_ = os.Remove(handle.Path)
		return CaptureMediaHandle{}, ErrCaptureInvalidWAV
	}
	file, err := os.OpenFile(handle.Path, os.O_RDWR, 0)
	if err != nil {
		_ = os.Remove(handle.Path)
		return CaptureMediaHandle{}, ErrCaptureStorage
	}
	if err = file.Sync(); err == nil {
		err = file.Close()
	} else {
		_ = file.Close()
	}
	if err != nil {
		_ = os.Remove(handle.Path)
		return CaptureMediaHandle{}, ErrCaptureStorage
	}
	var destination string
	switch handle.Class {
	case CaptureSelfTest:
		handle.State = CaptureSelfTestLocal
		destination = filepath.Join(s.selfTestDirectory(), handle.ID+".selftest.wav")
	case CaptureUserRecording:
		handle.State = CaptureDurableUnsent
		destination = filepath.Join(s.draftDirectory(), handle.ID+".draft.wav")
	default:
		_ = os.Remove(handle.Path)
		return CaptureMediaHandle{}, ErrCaptureInvalidState
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		_ = os.Remove(handle.Path)
		return CaptureMediaHandle{}, ErrCaptureStorage
	}
	if err := os.Rename(handle.Path, destination); err != nil {
		_ = os.Remove(handle.Path)
		return CaptureMediaHandle{}, ErrCaptureStorage
	}
	if err := os.Chmod(destination, 0o600); err != nil {
		_ = os.Remove(destination)
		return CaptureMediaHandle{}, ErrCaptureStorage
	}
	if err := syncDirectory(filepath.Dir(destination)); err != nil {
		_ = os.Remove(destination)
		return CaptureMediaHandle{}, ErrCaptureStorage
	}
	handle.Path = destination
	return handle, nil
}

func (s *CaptureMediaStore) Cancel(handle CaptureMediaHandle) error {
	return s.deleteOwned(handle, CapturePartial, CaptureFinalizing)
}

func (s *CaptureMediaStore) CloseSelfTest(handle CaptureMediaHandle) error {
	if handle.Class != CaptureSelfTest {
		return ErrCaptureInvalidState
	}
	return s.deleteOwned(handle, CaptureSelfTestLocal)
}

func (s *CaptureMediaStore) ConfirmUploadAndDelete(handle CaptureMediaHandle) error {
	if handle.Class != CaptureUserRecording {
		return ErrCaptureInvalidState
	}
	return s.deleteOwned(handle, CaptureDurableUnsent, CaptureUploadedConfirmed)
}

func (s *CaptureMediaStore) ExplicitlyDelete(handle CaptureMediaHandle) error {
	return s.deleteOwned(handle, CaptureSelfTestLocal, CaptureDurableUnsent)
}

func (s *CaptureMediaStore) Recover() (CaptureMediaRecovery, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.prepareDirectories(); err != nil {
		return CaptureMediaRecovery{}, err
	}
	partials, err := removeAllOwned(s.partialDirectory())
	if err != nil {
		return CaptureMediaRecovery{}, err
	}
	selfTests, err := removeAllOwned(s.selfTestDirectory())
	if err != nil {
		return CaptureMediaRecovery{}, err
	}
	entries, err := os.ReadDir(s.draftDirectory())
	if err != nil {
		return CaptureMediaRecovery{}, ErrCaptureStorage
	}
	recovery := CaptureMediaRecovery{DeletedPartialCount: partials, DeletedSelfTestCount: selfTests}
	for _, entry := range entries {
		path := filepath.Join(s.draftDirectory(), entry.Name())
		id := strings.TrimSuffix(entry.Name(), ".draft.wav")
		data, readErr := os.ReadFile(path)
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".draft.wav") ||
			!isCaptureMediaID(id) || readErr != nil || !isStructurallyCompleteWAV(data) {
			if err := removeAndVerify(path); err != nil {
				return CaptureMediaRecovery{}, err
			}
			recovery.DeletedInvalidDraftCount++
			continue
		}
		recovery.RetainedDrafts = append(recovery.RetainedDrafts, CaptureMediaHandle{
			ID: id, Class: CaptureUserRecording, State: CaptureDurableUnsent, Path: path,
		})
	}
	sort.Slice(recovery.RetainedDrafts, func(i, j int) bool {
		return recovery.RetainedDrafts[i].ID < recovery.RetainedDrafts[j].ID
	})
	return recovery, nil
}

func (s *CaptureMediaStore) deleteOwned(handle CaptureMediaHandle, states ...CaptureMediaState) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	allowed := false
	for _, state := range states {
		allowed = allowed || handle.State == state
	}
	if !allowed || !isCaptureMediaID(handle.ID) || filepath.Clean(handle.Path) != s.expectedPath(handle) {
		return ErrCaptureInvalidState
	}
	return removeAndVerify(handle.Path)
}

func (s *CaptureMediaStore) expectedPath(handle CaptureMediaHandle) string {
	switch handle.State {
	case CapturePartial, CaptureFinalizing:
		return filepath.Join(s.partialDirectory(), handle.ID+".partial.wav")
	case CaptureSelfTestLocal:
		return filepath.Join(s.selfTestDirectory(), handle.ID+".selftest.wav")
	case CaptureDurableUnsent, CaptureUploading, CaptureUploadedConfirmed:
		return filepath.Join(s.draftDirectory(), handle.ID+".draft.wav")
	default:
		return filepath.Join(s.root, "invalid")
	}
}

func (s *CaptureMediaStore) prepareDirectories() error {
	for _, directory := range []string{s.root, s.partialDirectory(), s.selfTestDirectory(), s.draftDirectory()} {
		if err := os.MkdirAll(directory, 0o700); err != nil || os.Chmod(directory, 0o700) != nil {
			return ErrCaptureStorage
		}
	}
	return nil
}

func (s *CaptureMediaStore) partialDirectory() string  { return filepath.Join(s.root, "partials") }
func (s *CaptureMediaStore) selfTestDirectory() string { return filepath.Join(s.root, "self-tests") }
func (s *CaptureMediaStore) draftDirectory() string    { return filepath.Join(s.root, "drafts") }

func randomCaptureMediaID() (string, error) {
	raw := make([]byte, 16)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", raw), nil
}

func isCaptureMediaID(value string) bool {
	if len(value) != 32 {
		return false
	}
	for _, char := range value {
		if (char < '0' || char > '9') && (char < 'a' || char > 'f') {
			return false
		}
	}
	return true
}

func isStructurallyCompleteWAV(data []byte) bool {
	return len(data) >= 44 && string(data[0:4]) == "RIFF" && string(data[8:12]) == "WAVE" &&
		string(data[36:40]) == "data" && int(binary.LittleEndian.Uint32(data[4:8])) == len(data)-8 &&
		int(binary.LittleEndian.Uint32(data[40:44])) == len(data)-44
}

func removeAllOwned(directory string) (int, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return 0, ErrCaptureStorage
	}
	for _, entry := range entries {
		if err := removeAndVerify(filepath.Join(directory, entry.Name())); err != nil {
			return 0, err
		}
	}
	return len(entries), nil
}

func removeAndVerify(path string) error {
	if err := os.RemoveAll(path); err != nil {
		return ErrCaptureStorage
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		return ErrCaptureStorage
	}
	return nil
}

func syncDirectory(directory string) error {
	if runtime.GOOS == "windows" {
		// MoveFileEx semantics on the reviewed Windows path provide the atomic
		// name transition; directory handles cannot be fsynced through os.File.
		return nil
	}
	file, err := os.Open(directory)
	if err != nil {
		return ErrCaptureStorage
	}
	defer file.Close()
	if err := file.Sync(); err != nil {
		return ErrCaptureStorage
	}
	return nil
}
