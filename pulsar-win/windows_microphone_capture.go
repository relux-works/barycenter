package main

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"os"
	"sync"
)

const (
	WindowsCaptureSampleRate     = 48_000
	WindowsCaptureChannels       = 1
	WindowsCaptureBitsPerSample  = 16
	WindowsCaptureMaximumSeconds = 180
	WindowsCaptureMaximumBytes   = 50 << 20
	windowsCaptureWAVHeaderBytes = 44
	windowsCaptureDuckGain       = 0.25
)

var (
	ErrWindowsCaptureExplicitAction = errors.New("windows_capture_explicit_record_required")
	ErrWindowsCaptureBusy           = errors.New("windows_capture_busy")
	ErrWindowsCapturePermission     = errors.New("windows_capture_permission_denied")
	ErrWindowsCaptureDevice         = errors.New("windows_capture_device_unavailable")
	ErrWindowsCaptureFormat         = errors.New("windows_capture_format_unsupported")
	ErrWindowsCaptureBackendFailure = errors.New("windows_capture_backend_failure")
)

type WindowsCapturePermission string

const (
	WindowsCapturePermissionAllowed        WindowsCapturePermission = "allowed"
	WindowsCapturePermissionPromptRequired WindowsCapturePermission = "prompt_required"
	WindowsCapturePermissionDenied         WindowsCapturePermission = "denied"
	WindowsCapturePermissionUnavailable    WindowsCapturePermission = "unavailable"
)

type WindowsCaptureStopReason string

const (
	WindowsCaptureUserStop         WindowsCaptureStopReason = "user_stop"
	WindowsCaptureDurationLimit    WindowsCaptureStopReason = "duration_limit"
	WindowsCaptureSizeLimit        WindowsCaptureStopReason = "size_limit"
	WindowsCaptureCancel           WindowsCaptureStopReason = "cancel"
	WindowsCaptureQuit             WindowsCaptureStopReason = "quit"
	WindowsCaptureSessionLock      WindowsCaptureStopReason = "session_lock"
	WindowsCaptureSuspend          WindowsCaptureStopReason = "suspend"
	WindowsCaptureDeviceLost       WindowsCaptureStopReason = "device_lost"
	WindowsCapturePermissionRevoke WindowsCaptureStopReason = "permission_revoke"
	WindowsCaptureOverflow         WindowsCaptureStopReason = "overflow"
	WindowsCaptureBackendFailure   WindowsCaptureStopReason = "backend_failure"
)

type WindowsCaptureFormat struct {
	SampleRate uint32
	Channels   uint32
}

type WindowsCaptureTerminalError struct {
	Reason WindowsCaptureStopReason
	Err    error
}

func (e *WindowsCaptureTerminalError) Error() string {
	if e.Err == nil {
		return string(e.Reason)
	}
	return fmt.Sprintf("%s: %v", e.Reason, e.Err)
}

func (e *WindowsCaptureTerminalError) Unwrap() error { return e.Err }

// WindowsMicrophoneBackend is deliberately UI- and network-free. The Windows
// implementation owns AppCapability, device resolution and the WASAPI helper;
// portable tests replace it with a deterministic fake.
type WindowsMicrophoneBackend interface {
	Permission(context.Context, bool) (WindowsCapturePermission, error)
	ResolveInput(context.Context, string) (string, error)
	Open(context.Context, string) (WindowsMicrophoneStream, error)
}

type WindowsMicrophoneStream interface {
	Format() WindowsCaptureFormat
	Read(context.Context, []float32) (uint32, error)
	Stop(WindowsCaptureStopReason) error
	Close() error
}

type WindowsCaptureCuePlayer interface {
	PlayRecordingCue(context.Context, RecordingCuePhase) error
}

type WindowsCaptureDucker interface {
	SetMusicGain(float32, int)
}

type WindowsCaptureRequest struct {
	ExplicitUserAction bool
	DeviceID           string
	MediaClass         CaptureMediaClass
	Meter              func(float32)
}

type WindowsCaptureOutcome struct {
	Reason WindowsCaptureStopReason
	Draft  *CaptureMediaHandle
	Frames uint64
	Bytes  uint64
	Err    error
}

type WindowsMicrophoneCaptureService struct {
	backend WindowsMicrophoneBackend
	store   *CaptureMediaStore
	cues    WindowsCaptureCuePlayer
	ducker  WindowsCaptureDucker

	mu     sync.Mutex
	active *WindowsCaptureSession
}

func NewWindowsMicrophoneCaptureService(backend WindowsMicrophoneBackend, store *CaptureMediaStore, cues WindowsCaptureCuePlayer, ducker WindowsCaptureDucker) *WindowsMicrophoneCaptureService {
	return &WindowsMicrophoneCaptureService{backend: backend, store: store, cues: cues, ducker: ducker}
}

type WindowsCaptureSession struct {
	service *WindowsMicrophoneCaptureService
	stream  WindowsMicrophoneStream
	cancel  context.CancelFunc
	done    chan WindowsCaptureOutcome

	stopOnce sync.Once
	mu       sync.Mutex
	reason   WindowsCaptureStopReason
}

func (s *WindowsMicrophoneCaptureService) Start(ctx context.Context, request WindowsCaptureRequest) (*WindowsCaptureSession, error) {
	if !request.ExplicitUserAction {
		return nil, ErrWindowsCaptureExplicitAction
	}
	if s.backend == nil || s.store == nil {
		return nil, ErrWindowsCaptureBackendFailure
	}
	mediaClass := request.MediaClass
	if mediaClass == "" {
		mediaClass = CaptureUserRecording
	}
	if mediaClass != CaptureUserRecording && mediaClass != CaptureSelfTest {
		return nil, ErrCaptureInvalidState
	}
	s.mu.Lock()
	if s.active != nil {
		s.mu.Unlock()
		return nil, ErrWindowsCaptureBusy
	}
	// Reserve the service before the permission prompt. A second Record cannot
	// start a competing prompt or native operation.
	startCtx, cancel := context.WithCancel(ctx)
	placeholder := &WindowsCaptureSession{service: s, cancel: cancel, done: make(chan WindowsCaptureOutcome, 1)}
	s.active = placeholder
	s.mu.Unlock()

	releaseReservation := func() {
		cancel()
		s.mu.Lock()
		if s.active == placeholder {
			s.active = nil
		}
		s.mu.Unlock()
	}
	permission, err := s.backend.Permission(startCtx, true)
	if err != nil || permission != WindowsCapturePermissionAllowed {
		releaseReservation()
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrWindowsCapturePermission, err)
		}
		return nil, fmt.Errorf("%w: %s", ErrWindowsCapturePermission, permission)
	}
	deviceID, err := s.backend.ResolveInput(startCtx, request.DeviceID)
	if err != nil || deviceID == "" {
		releaseReservation()
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrWindowsCaptureDevice, err)
		}
		return nil, ErrWindowsCaptureDevice
	}
	if s.cues != nil {
		if err := s.cues.PlayRecordingCue(startCtx, CuePlayingStart); err != nil {
			releaseReservation()
			return nil, err
		}
	}
	stream, err := s.backend.Open(startCtx, deviceID)
	if err != nil || stream == nil {
		releaseReservation()
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrWindowsCaptureDevice, err)
		}
		return nil, ErrWindowsCaptureDevice
	}
	format := stream.Format()
	if format.SampleRate == 0 || format.Channels == 0 || format.Channels > 32 {
		_ = stream.Stop(WindowsCaptureBackendFailure)
		_ = stream.Close()
		releaseReservation()
		return nil, ErrWindowsCaptureFormat
	}
	partial, err := s.store.Begin(mediaClass)
	if err != nil {
		_ = stream.Stop(WindowsCaptureCancel)
		_ = stream.Close()
		releaseReservation()
		return nil, err
	}
	file, err := os.OpenFile(partial.Path, os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil || writeWindowsCaptureWAVHeader(file, 0) != nil {
		if file != nil {
			_ = file.Close()
		}
		_ = s.store.Cancel(partial)
		_ = stream.Stop(WindowsCaptureCancel)
		_ = stream.Close()
		releaseReservation()
		return nil, ErrCaptureStorage
	}

	session := placeholder
	session.mu.Lock()
	session.stream = stream
	pendingReason := session.reason
	session.mu.Unlock()
	if pendingReason != "" {
		_ = stream.Stop(pendingReason)
	}
	if s.ducker != nil {
		s.ducker.SetMusicGain(windowsCaptureDuckGain, 80)
	}
	go s.capture(startCtx, session, request, partial, file)
	return session, nil
}

func (s *WindowsMicrophoneCaptureService) capture(ctx context.Context, session *WindowsCaptureSession, request WindowsCaptureRequest, partial CaptureMediaHandle, file *os.File) {
	format := session.stream.Format()
	resampler := windowsMonoResampler{inputRate: uint64(format.SampleRate)}
	input := make([]float32, 2048*int(format.Channels))
	pcm := make([]int16, 0, 2300)
	var frames, dataBytes uint64
	reason := WindowsCaptureBackendFailure
	var captureErr error

	for {
		readFrames, err := session.stream.Read(ctx, input)
		if readFrames > 0 {
			count := int(readFrames * format.Channels)
			if count > len(input) {
				captureErr = ErrWindowsCaptureFormat
				break
			}
			pcm = pcm[:0]
			pcm = resampler.append(pcm, input[:count], int(format.Channels))
			if len(pcm) > 0 {
				remainingFrames := uint64(WindowsCaptureMaximumSeconds*WindowsCaptureSampleRate) - frames
				remainingBytes := uint64(WindowsCaptureMaximumBytes-windowsCaptureWAVHeaderBytes) - dataBytes
				allowed := uint64(len(pcm))
				limitReason := WindowsCaptureStopReason("")
				if allowed > remainingFrames {
					allowed, limitReason = remainingFrames, WindowsCaptureDurationLimit
				}
				if allowed*2 > remainingBytes {
					allowed, limitReason = remainingBytes/2, WindowsCaptureSizeLimit
				}
				if allowed > 0 {
					if writeErr := writeWindowsCapturePCM16(file, pcm[:allowed]); writeErr != nil {
						captureErr = writeErr
						break
					}
					frames += allowed
					dataBytes += allowed * 2
					if request.Meter != nil {
						request.Meter(windowsCaptureRMS(pcm[:allowed]))
					}
				}
				if limitReason != "" || frames >= WindowsCaptureMaximumSeconds*WindowsCaptureSampleRate || dataBytes >= WindowsCaptureMaximumBytes-windowsCaptureWAVHeaderBytes {
					if limitReason == "" {
						limitReason = WindowsCaptureDurationLimit
					}
					session.Stop(limitReason)
				}
			}
		}
		if err != nil {
			var terminal *WindowsCaptureTerminalError
			if errors.As(err, &terminal) {
				reason, captureErr = terminal.Reason, terminal.Err
			} else if errors.Is(err, context.Canceled) {
				reason = session.requestedReason()
			} else {
				reason, captureErr = WindowsCaptureBackendFailure, err
			}
			break
		}
	}
	if requested := session.requestedReason(); requested != "" && (reason == WindowsCaptureUserStop || reason == WindowsCaptureBackendFailure) {
		reason = requested
	}
	_ = session.stream.Close()
	if s.ducker != nil {
		s.ducker.SetMusicGain(1, 120)
	}

	outcome := WindowsCaptureOutcome{Reason: reason, Frames: frames, Bytes: dataBytes + windowsCaptureWAVHeaderBytes, Err: captureErr}
	keep := reason == WindowsCaptureUserStop || reason == WindowsCaptureDurationLimit || reason == WindowsCaptureSizeLimit
	if keep && frames > 0 && captureErr == nil {
		if _, err := file.Seek(0, 0); err == nil {
			err = writeWindowsCaptureWAVHeader(file, uint32(dataBytes))
		}
		if err := file.Sync(); err != nil && captureErr == nil {
			captureErr = err
		}
		if err := file.Close(); err != nil && captureErr == nil {
			captureErr = err
		}
		if captureErr == nil {
			finalizing, err := s.store.Stop(partial)
			if err == nil {
				var draft CaptureMediaHandle
				draft, err = s.store.Finalize(finalizing)
				if err == nil {
					outcome.Draft = &draft
				}
			}
			if err != nil {
				captureErr = err
			}
		}
		if captureErr == nil && s.cues != nil {
			captureErr = s.cues.PlayRecordingCue(context.Background(), CuePlayingStop)
		}
	} else {
		_ = file.Close()
	}
	if captureErr != nil || outcome.Draft == nil {
		if outcome.Draft == nil {
			_ = s.store.Cancel(partial)
		}
		outcome.Err = captureErr
	}

	s.mu.Lock()
	if s.active == session {
		s.active = nil
	}
	s.mu.Unlock()
	session.done <- outcome
	close(session.done)
}

func (s *WindowsCaptureSession) Stop(reason WindowsCaptureStopReason) {
	if reason == "" {
		reason = WindowsCaptureUserStop
	}
	s.stopOnce.Do(func() {
		s.setReason(reason)
		s.mu.Lock()
		stream := s.stream
		cancel := s.cancel
		s.mu.Unlock()
		if stream != nil {
			_ = stream.Stop(reason)
		} else if cancel != nil {
			cancel()
		}
	})
}

func (s *WindowsCaptureSession) Done() <-chan WindowsCaptureOutcome { return s.done }

func (s *WindowsCaptureSession) setReason(reason WindowsCaptureStopReason) {
	s.mu.Lock()
	if s.reason == "" {
		s.reason = reason
	}
	s.mu.Unlock()
}

func (s *WindowsCaptureSession) requestedReason() WindowsCaptureStopReason {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reason
}

func (s *WindowsMicrophoneCaptureService) stopActive(reason WindowsCaptureStopReason) {
	s.mu.Lock()
	active := s.active
	s.mu.Unlock()
	if active != nil {
		active.Stop(reason)
	}
}

func (s *WindowsMicrophoneCaptureService) Cancel()   { s.stopActive(WindowsCaptureCancel) }
func (s *WindowsMicrophoneCaptureService) Shutdown() { s.stopActive(WindowsCaptureQuit) }
func (s *WindowsMicrophoneCaptureService) HandleSessionLock() {
	s.stopActive(WindowsCaptureSessionLock)
}
func (s *WindowsMicrophoneCaptureService) HandleSuspend()    { s.stopActive(WindowsCaptureSuspend) }
func (s *WindowsMicrophoneCaptureService) HandleDeviceLoss() { s.stopActive(WindowsCaptureDeviceLost) }
func (s *WindowsMicrophoneCaptureService) HandlePermissionRevoke() {
	s.stopActive(WindowsCapturePermissionRevoke)
}

type windowsMonoResampler struct {
	inputRate uint64
	phase     uint64
}

func (r *windowsMonoResampler) append(dst []int16, input []float32, channels int) []int16 {
	if r.inputRate == 0 || channels <= 0 {
		return dst
	}
	for offset := 0; offset+channels <= len(input); offset += channels {
		var mono float64
		for channel := 0; channel < channels; channel++ {
			mono += float64(input[offset+channel])
		}
		mono /= float64(channels)
		mono = math.Max(-1, math.Min(1, mono))
		r.phase += WindowsCaptureSampleRate
		for r.phase >= r.inputRate {
			r.phase -= r.inputRate
			dst = append(dst, int16(math.Round(mono*32767)))
		}
	}
	return dst
}

func windowsCaptureRMS(samples []int16) float32 {
	if len(samples) == 0 {
		return 0
	}
	var sum float64
	for _, sample := range samples {
		value := float64(sample) / 32768
		sum += value * value
	}
	return float32(math.Sqrt(sum / float64(len(samples))))
}

func writeWindowsCaptureWAVHeader(file *os.File, dataBytes uint32) error {
	if file == nil {
		return ErrCaptureStorage
	}
	header := make([]byte, windowsCaptureWAVHeaderBytes)
	copy(header[0:4], "RIFF")
	binary.LittleEndian.PutUint32(header[4:8], dataBytes+36)
	copy(header[8:12], "WAVE")
	copy(header[12:16], "fmt ")
	binary.LittleEndian.PutUint32(header[16:20], 16)
	binary.LittleEndian.PutUint16(header[20:22], 1)
	binary.LittleEndian.PutUint16(header[22:24], WindowsCaptureChannels)
	binary.LittleEndian.PutUint32(header[24:28], WindowsCaptureSampleRate)
	binary.LittleEndian.PutUint32(header[28:32], WindowsCaptureSampleRate*WindowsCaptureChannels*WindowsCaptureBitsPerSample/8)
	binary.LittleEndian.PutUint16(header[32:34], WindowsCaptureChannels*WindowsCaptureBitsPerSample/8)
	binary.LittleEndian.PutUint16(header[34:36], WindowsCaptureBitsPerSample)
	copy(header[36:40], "data")
	binary.LittleEndian.PutUint32(header[40:44], dataBytes)
	_, err := file.Write(header)
	return err
}

func writeWindowsCapturePCM16(file *os.File, samples []int16) error {
	bytes := make([]byte, len(samples)*2)
	for index, sample := range samples {
		binary.LittleEndian.PutUint16(bytes[index*2:], uint16(sample))
	}
	_, err := file.Write(bytes)
	return err
}
