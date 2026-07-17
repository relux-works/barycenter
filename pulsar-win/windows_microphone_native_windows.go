//go:build windows

package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"sync"
	"time"

	"golang.org/x/sys/windows"
	"relux.works/duet/pulsar-win/internal/winprobe"
)

const windowsCaptureNativePoll = 50 * time.Millisecond

type nativeWindowsMicrophoneBackend struct {
	helper          *winprobe.Helper
	permissionEvent windows.Handle
	closeOnce       sync.Once
}

func (b *nativeWindowsMicrophoneBackend) Close() {
	if b == nil {
		return
	}
	b.closeOnce.Do(func() {
		_ = b.helper.PermissionUnsubscribe()
		if b.permissionEvent != 0 {
			_ = windows.CloseHandle(b.permissionEvent)
			b.permissionEvent = 0
		}
	})
}

// Inputs promotes the already reviewed AppCapability enumeration ABI from the
// hardware probe into production composition. It returns stable ids only after
// the async operation reaches a successful terminal state.
func (b *nativeWindowsMicrophoneBackend) Inputs(ctx context.Context) ([]WindowsCaptureInput, error) {
	event, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return nil, err
	}
	defer windows.CloseHandle(event)
	id, hr := b.helper.EnumerateDevices(event)
	if hr.Failed() || id == 0 {
		return nil, hr
	}
	defer b.helper.EnumerateDevicesRelease(id)
	for {
		state, count, outcome, callHR := b.helper.EnumerateDevicesResult(id)
		if callHR.Failed() {
			_ = b.helper.EnumerateDevicesCancel(id)
			return nil, callHR
		}
		if state == 1 {
			if outcome.Failed() || count < 0 || count > 256 {
				return nil, outcome
			}
			inputs := make([]WindowsCaptureInput, 0, count)
			for index := int32(0); index < count; index++ {
				device, infoHR := b.helper.DeviceInfo(id, index)
				if infoHR.Failed() || device.ID == "" {
					continue
				}
				inputs = append(inputs, WindowsCaptureInput{ID: device.ID, Name: device.Name})
			}
			return inputs, nil
		}
		if state == 3 {
			return nil, outcome
		}
		if err := waitWindowsCaptureEvent(ctx, event); err != nil {
			_ = b.helper.EnumerateDevicesCancel(id)
			return nil, err
		}
	}
}

// PickAudio returns a broker-owned handle, never a filesystem path. The
// intake layer takes ownership on its first Open call and closes it after the
// bounded review/import pass.
func (b *nativeWindowsMicrophoneBackend) PickAudio(ctx context.Context, owner windows.Handle) (WindowsBrokeredAudioFile, error) {
	event, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return WindowsBrokeredAudioFile{}, err
	}
	defer windows.CloseHandle(event)
	id, hr := b.helper.PickerOpen(owner, event)
	if hr.Failed() || id == 0 {
		return WindowsBrokeredAudioFile{}, hr
	}
	defer b.helper.PickerRelease(id)
	for {
		result, required, callHR := b.helper.PickerResult(id, false, 0)
		if callHR.Failed() {
			_ = b.helper.PickerCancel(id)
			return WindowsBrokeredAudioFile{}, callHR
		}
		if result.State == 1 {
			if result.Outcome.Failed() {
				return WindowsBrokeredAudioFile{}, result.Outcome
			}
			result, _, callHR = b.helper.PickerResult(id, true, required)
			if callHR.Failed() || result.Outcome.Failed() || !result.HandleTaken || result.Handle == windows.InvalidHandle {
				return WindowsBrokeredAudioFile{}, ErrWindowsBrokeredAccess
			}
			var handleMu sync.Mutex
			handle := result.Handle
			return WindowsBrokeredAudioFile{
				DisplayName: result.Name, SizeBytes: result.FileSize,
				Open: func() (io.ReadCloser, error) {
					handleMu.Lock()
					defer handleMu.Unlock()
					if handle == windows.InvalidHandle {
						return nil, ErrWindowsBrokeredAccess
					}
					opened := os.NewFile(uintptr(handle), result.Name)
					handle = windows.InvalidHandle
					if opened == nil {
						return nil, ErrWindowsBrokeredAccess
					}
					return opened, nil
				},
				Release: func() {
					handleMu.Lock()
					defer handleMu.Unlock()
					if handle != windows.InvalidHandle {
						_ = windows.CloseHandle(handle)
						handle = windows.InvalidHandle
					}
				},
			}, nil
		}
		if result.State == 3 {
			return WindowsBrokeredAudioFile{}, result.Outcome
		}
		if err := waitWindowsCaptureEvent(ctx, event); err != nil {
			_ = b.helper.PickerCancel(id)
			return WindowsBrokeredAudioFile{}, err
		}
	}
}

func NewNativeWindowsMicrophoneBackend() (WindowsMicrophoneBackend, error) {
	helper, err := winprobe.LoadHelper()
	if err != nil {
		return nil, err
	}
	version, size, hr := helper.Version()
	if hr.Failed() || version != winprobe.HelperABIVersion || size != winprobe.CaptureFormatStructSize {
		return nil, fmt.Errorf("pulsar-capture ABI mismatch: %s version=%d format=%d", hr.Hex(), version, size)
	}
	if hr := helper.Init(); hr.Failed() {
		return nil, fmt.Errorf("CapInit: %s", hr.Hex())
	}
	permissionEvent, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return nil, err
	}
	if hr := helper.PermissionSubscribe(permissionEvent); hr.Failed() {
		windows.CloseHandle(permissionEvent)
		return nil, fmt.Errorf("CapPermissionSubscribe: %s", hr.Hex())
	}
	return &nativeWindowsMicrophoneBackend{helper: helper, permissionEvent: permissionEvent}, nil
}

func (b *nativeWindowsMicrophoneBackend) Permission(ctx context.Context, prompt bool) (WindowsCapturePermission, error) {
	status, hr := b.helper.PermissionCheck()
	if hr.Failed() {
		return WindowsCapturePermissionUnavailable, hr
	}
	if status != winprobe.PermissionPromptRequired || !prompt {
		return productionPermission(status), nil
	}
	event, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return WindowsCapturePermissionUnavailable, err
	}
	defer windows.CloseHandle(event)
	id, requestHR := b.helper.PermissionRequest(event)
	if requestHR.Failed() || id == 0 {
		return WindowsCapturePermissionUnavailable, requestHR
	}
	defer finishNativePermissionRequest(b.helper, id, event)
	for {
		state, result, outcome, callHR := b.helper.PermissionRequestResult(id)
		if callHR.Failed() {
			_ = b.helper.PermissionRequestCancel(id)
			return WindowsCapturePermissionUnavailable, callHR
		}
		if state == 1 {
			if outcome.Failed() {
				return productionPermission(result), outcome
			}
			return productionPermission(result), nil
		}
		if state == 3 {
			return productionPermission(result), outcome
		}
		if err := waitWindowsCaptureEvent(ctx, event); err != nil {
			return WindowsCapturePermissionUnavailable, err
		}
	}
}

func productionPermission(status winprobe.PermissionStatus) WindowsCapturePermission {
	switch status {
	case winprobe.PermissionAllowed:
		return WindowsCapturePermissionAllowed
	case winprobe.PermissionPromptRequired:
		return WindowsCapturePermissionPromptRequired
	case winprobe.PermissionDeniedByUser, winprobe.PermissionDeniedBySystem, winprobe.PermissionNotDeclared:
		return WindowsCapturePermissionDenied
	default:
		return WindowsCapturePermissionUnavailable
	}
}

func (b *nativeWindowsMicrophoneBackend) ResolveInput(ctx context.Context, requested string) (string, error) {
	if requested != "" {
		return requested, nil
	}
	event, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(event)
	id, hr := b.helper.DefaultDevice(0, event)
	if hr.Failed() || id == 0 {
		return "", hr
	}
	defer finishNativeDefaultDeviceRequest(b.helper, id, event)
	for {
		state, deviceID, outcome, callHR := b.helper.DefaultDeviceResult(id)
		if callHR.Failed() {
			return "", callHR
		}
		if state == 1 {
			if outcome.Failed() || deviceID == "" {
				return "", outcome
			}
			return deviceID, nil
		}
		if state == 3 {
			return "", outcome
		}
		if err := waitWindowsCaptureEvent(ctx, event); err != nil {
			return "", err
		}
	}
}

func (b *nativeWindowsMicrophoneBackend) Open(ctx context.Context, deviceID string) (WindowsMicrophoneStream, error) {
	return b.open(ctx, deviceID, WindowsCaptureQualityLegacy)
}

func (b *nativeWindowsMicrophoneBackend) OpenQuality(
	ctx context.Context,
	deviceID string,
	request WindowsCaptureQualityRequest,
) (WindowsMicrophoneStream, error) {
	return b.open(ctx, deviceID, request)
}

func (b *nativeWindowsMicrophoneBackend) open(
	ctx context.Context,
	deviceID string,
	request WindowsCaptureQualityRequest,
) (WindowsMicrophoneStream, error) {
	event, err := windows.CreateEvent(nil, 0, 0, nil)
	if err != nil {
		return nil, err
	}
	id, hr := b.helper.CapturePrepare(event)
	if hr.Failed() || id == 0 {
		windows.CloseHandle(event)
		return nil, hr
	}
	failed := true
	defer func() {
		if failed {
			_ = stopAndReleaseNativeCapture(b.helper, id, event)
			windows.CloseHandle(event)
		}
	}()
	if configureHR := b.helper.CaptureConfigureQuality(id, request.ProcessingRequested); configureHR.Failed() {
		return nil, configureHR
	}
	activated := false
	for {
		result, callHR := b.helper.CaptureResult(id)
		if callHR.Failed() {
			return nil, callHR
		}
		if !activated && result.State == winprobe.CaptureStatePreparing && result.Format.Ready == 1 {
			if hr := b.helper.CaptureActivate(id, deviceID); hr.Failed() {
				return nil, hr
			}
			activated = true
		}
		if result.State == winprobe.CaptureStateCapturing && result.Format.Valid == 1 {
			quality, qualityHR := b.helper.CaptureQualityResult(id)
			if qualityHR.Failed() {
				return nil, qualityHR
			}
			failed = false
			return &nativeWindowsMicrophoneStream{
				helper: b.helper, event: event, permissionEvent: b.permissionEvent, operationID: id,
				format: WindowsCaptureFormat{
					SampleRate: result.Format.SampleRate, Channels: result.Format.Channels,
					CommunicationsCategoryActive: quality.CommunicationsCategoryActive == 1,
					NativeEffectsVerified:        quality.NativeEffectsVerified == 1,
				},
			}, nil
		}
		if result.State == winprobe.CaptureStateStopped || result.State == winprobe.CaptureStateFailed || result.State == winprobe.CaptureStateCancelled {
			return nil, &WindowsCaptureTerminalError{Reason: productionCaptureReason(result.Reason), Err: result.Outcome}
		}
		if err := waitWindowsCaptureEvent(ctx, event); err != nil {
			return nil, err
		}
	}
}

type nativeWindowsMicrophoneStream struct {
	helper          *winprobe.Helper
	event           windows.Handle
	permissionEvent windows.Handle
	operationID     uint32
	format          WindowsCaptureFormat

	mu        sync.Mutex
	requested WindowsCaptureStopReason
	terminal  bool
	closeOnce sync.Once
	closeErr  error
}

func (s *nativeWindowsMicrophoneStream) Format() WindowsCaptureFormat { return s.format }

func (s *nativeWindowsMicrophoneStream) Read(ctx context.Context, buffer []float32) (uint32, error) {
	for {
		result, hr := s.helper.CaptureResult(s.operationID)
		if hr.Failed() {
			return 0, hr
		}
		if result.FramesAvailable > 0 {
			maxFrames := uint32(len(buffer)) / s.format.Channels
			if result.FramesAvailable < maxFrames {
				maxFrames = result.FramesAvailable
			}
			frames, readHR := s.helper.CaptureRead(s.operationID, buffer, maxFrames)
			if readHR.Failed() {
				return 0, readHR
			}
			return frames, nil
		}
		if result.State == winprobe.CaptureStateStopped || result.State == winprobe.CaptureStateFailed || result.State == winprobe.CaptureStateCancelled {
			reason := productionCaptureReason(result.Reason)
			s.mu.Lock()
			if result.Reason == winprobe.ReasonUserStop && s.requested != "" {
				reason = s.requested
			}
			s.terminal = true
			s.mu.Unlock()
			var outcome error
			if result.Outcome.Failed() {
				outcome = result.Outcome
			}
			return 0, &WindowsCaptureTerminalError{Reason: reason, Err: outcome}
		}
		permissionChanged, err := waitWindowsCaptureEvents(ctx, s.event, s.permissionEvent)
		if err != nil {
			return 0, err
		}
		if permissionChanged {
			status, hr := s.helper.PermissionCheck()
			if hr.Failed() || status != winprobe.PermissionAllowed {
				_ = s.Stop(WindowsCapturePermissionRevoke)
			}
		}
	}
}

func (s *nativeWindowsMicrophoneStream) Stop(reason WindowsCaptureStopReason) error {
	s.mu.Lock()
	if s.requested == "" {
		s.requested = reason
	}
	s.mu.Unlock()
	hr := s.helper.CaptureStop(s.operationID, nativeCaptureReason(reason))
	if hr.Failed() {
		return hr
	}
	return nil
}

func (s *nativeWindowsMicrophoneStream) Close() error {
	s.closeOnce.Do(func() {
		s.mu.Lock()
		terminal := s.terminal
		s.mu.Unlock()
		if !terminal {
			s.closeErr = stopAndReleaseNativeCapture(s.helper, s.operationID, s.event)
		} else {
			hr := s.helper.CaptureRelease(s.operationID)
			if hr.Failed() {
				s.closeErr = hr
			}
		}
		if err := windows.CloseHandle(s.event); s.closeErr == nil {
			s.closeErr = err
		}
	})
	return s.closeErr
}

func stopAndReleaseNativeCapture(helper *winprobe.Helper, operationID uint32, event windows.Handle) error {
	_ = helper.CaptureStop(operationID, winprobe.ReasonCancel)
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		result, hr := helper.CaptureResult(operationID)
		if hr.Failed() || result.State == winprobe.CaptureStateStopped || result.State == winprobe.CaptureStateFailed || result.State == winprobe.CaptureStateCancelled {
			break
		}
		_, _ = windows.WaitForSingleObject(event, uint32(windowsCaptureNativePoll/time.Millisecond))
	}
	hr := helper.CaptureRelease(operationID)
	if hr.Failed() {
		return hr
	}
	return nil
}

func waitWindowsCaptureEvent(ctx context.Context, event windows.Handle) error {
	for {
		result, err := windows.WaitForSingleObject(event, uint32(windowsCaptureNativePoll/time.Millisecond))
		if err != nil {
			return err
		}
		if result == windows.WAIT_OBJECT_0 {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}
	}
}

func waitWindowsCaptureEvents(ctx context.Context, captureEvent, permissionEvent windows.Handle) (bool, error) {
	for {
		result, err := windows.WaitForMultipleObjects([]windows.Handle{captureEvent, permissionEvent}, false, uint32(windowsCaptureNativePoll/time.Millisecond))
		if err != nil {
			return false, err
		}
		if result == windows.WAIT_OBJECT_0 {
			return false, nil
		}
		if result == windows.WAIT_OBJECT_0+1 {
			return true, nil
		}
		select {
		case <-ctx.Done():
			return false, ctx.Err()
		default:
		}
	}
}

func finishNativePermissionRequest(helper *winprobe.Helper, operationID uint32, event windows.Handle) {
	state, _, _, hr := helper.PermissionRequestResult(operationID)
	if hr.Succeeded() && state == 0 {
		_ = helper.PermissionRequestCancel(operationID)
		deadline := time.Now().Add(2 * time.Second)
		for state == 0 && time.Now().Before(deadline) {
			_, _ = windows.WaitForSingleObject(event, uint32(windowsCaptureNativePoll/time.Millisecond))
			state, _, _, _ = helper.PermissionRequestResult(operationID)
		}
	}
	_ = helper.PermissionRequestRelease(operationID)
}

func finishNativeDefaultDeviceRequest(helper *winprobe.Helper, operationID uint32, event windows.Handle) {
	state, _, _, hr := helper.DefaultDeviceResult(operationID)
	deadline := time.Now().Add(2 * time.Second)
	for hr.Succeeded() && state == 0 && time.Now().Before(deadline) {
		_, _ = windows.WaitForSingleObject(event, uint32(windowsCaptureNativePoll/time.Millisecond))
		state, _, _, hr = helper.DefaultDeviceResult(operationID)
	}
	_ = helper.DefaultDeviceRelease(operationID)
}

func nativeCaptureReason(reason WindowsCaptureStopReason) winprobe.CaptureReason {
	switch reason {
	case WindowsCapturePermissionRevoke:
		return winprobe.ReasonPermissionRevoke
	case WindowsCaptureDeviceLost:
		return winprobe.ReasonDeviceLost
	case WindowsCaptureQuit:
		return winprobe.ReasonShutdown
	case WindowsCaptureSuspend:
		return winprobe.ReasonSuspend
	case WindowsCaptureSessionLock:
		return winprobe.ReasonLock
	case WindowsCaptureCancel:
		return winprobe.ReasonCancel
	default:
		return winprobe.ReasonUserStop
	}
}

func productionCaptureReason(reason winprobe.CaptureReason) WindowsCaptureStopReason {
	switch reason {
	case winprobe.ReasonPermissionRevoke:
		return WindowsCapturePermissionRevoke
	case winprobe.ReasonDeviceLost:
		return WindowsCaptureDeviceLost
	case winprobe.ReasonShutdown:
		return WindowsCaptureQuit
	case winprobe.ReasonSuspend:
		return WindowsCaptureSuspend
	case winprobe.ReasonLock:
		return WindowsCaptureSessionLock
	case winprobe.ReasonCancel:
		return WindowsCaptureCancel
	case winprobe.ReasonOverflow:
		return WindowsCaptureOverflow
	case winprobe.ReasonWasapiError, winprobe.ReasonFormatError, winprobe.ReasonDiscontinuity:
		return WindowsCaptureBackendFailure
	default:
		return WindowsCaptureUserStop
	}
}
