package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeWindowsMicrophoneBackend struct {
	permission WindowsCapturePermission
	stream     *fakeWindowsMicrophoneStream
	device     string
	prompts    []bool
	resolved   []string
	events     *[]string
}

type blockingWindowsPermissionBackend struct{ started chan struct{} }

func (b *blockingWindowsPermissionBackend) Permission(ctx context.Context, _ bool) (WindowsCapturePermission, error) {
	close(b.started)
	<-ctx.Done()
	return WindowsCapturePermissionUnavailable, ctx.Err()
}

func (b *blockingWindowsPermissionBackend) ResolveInput(context.Context, string) (string, error) {
	panic("ResolveInput must not follow a cancelled permission request")
}

func (b *blockingWindowsPermissionBackend) Open(context.Context, string) (WindowsMicrophoneStream, error) {
	panic("Open must not follow a cancelled permission request")
}

func (b *fakeWindowsMicrophoneBackend) Permission(_ context.Context, prompt bool) (WindowsCapturePermission, error) {
	b.prompts = append(b.prompts, prompt)
	if b.events != nil {
		*b.events = append(*b.events, "permission")
	}
	return b.permission, nil
}

func (b *fakeWindowsMicrophoneBackend) ResolveInput(_ context.Context, requested string) (string, error) {
	b.resolved = append(b.resolved, requested)
	if requested != "" {
		return requested, nil
	}
	if b.device == "" {
		return "default-input", nil
	}
	return b.device, nil
}

func (b *fakeWindowsMicrophoneBackend) Open(_ context.Context, device string) (WindowsMicrophoneStream, error) {
	if b.events != nil {
		*b.events = append(*b.events, "open:"+device)
	}
	return b.stream, nil
}

type fakeWindowsMicrophoneStream struct {
	format     WindowsCaptureFormat
	chunks     [][]float32
	repeat     []float32
	index      int
	stop       chan WindowsCaptureStopReason
	stopOnce   sync.Once
	closeCount int
	mu         sync.Mutex
}

func newFakeWindowsMicrophoneStream(format WindowsCaptureFormat, chunks ...[]float32) *fakeWindowsMicrophoneStream {
	return &fakeWindowsMicrophoneStream{format: format, chunks: chunks, stop: make(chan WindowsCaptureStopReason, 1)}
}

func (s *fakeWindowsMicrophoneStream) Format() WindowsCaptureFormat { return s.format }

func (s *fakeWindowsMicrophoneStream) Read(ctx context.Context, buffer []float32) (uint32, error) {
	s.mu.Lock()
	if s.index < len(s.chunks) {
		chunk := s.chunks[s.index]
		s.index++
		copy(buffer, chunk)
		s.mu.Unlock()
		return uint32(len(chunk)) / s.format.Channels, nil
	}
	if len(s.repeat) != 0 {
		copy(buffer, s.repeat)
		frames := uint32(len(s.repeat)) / s.format.Channels
		s.mu.Unlock()
		select {
		case reason := <-s.stop:
			return 0, &WindowsCaptureTerminalError{Reason: reason}
		default:
			return frames, nil
		}
	}
	s.mu.Unlock()
	select {
	case reason := <-s.stop:
		return 0, &WindowsCaptureTerminalError{Reason: reason}
	case <-ctx.Done():
		return 0, ctx.Err()
	}
}

func (s *fakeWindowsMicrophoneStream) Stop(reason WindowsCaptureStopReason) error {
	s.stopOnce.Do(func() { s.stop <- reason })
	return nil
}

func (s *fakeWindowsMicrophoneStream) Close() error {
	s.mu.Lock()
	s.closeCount++
	s.mu.Unlock()
	return nil
}

type fakeWindowsCaptureCues struct {
	phases []RecordingCuePhase
	events *[]string
}

func (c *fakeWindowsCaptureCues) PlayRecordingCue(_ context.Context, phase RecordingCuePhase) error {
	c.phases = append(c.phases, phase)
	if c.events != nil {
		*c.events = append(*c.events, string(phase))
	}
	return nil
}

type fakeWindowsCaptureDucker struct{ calls []float32 }

func (d *fakeWindowsCaptureDucker) SetMusicGain(gain float32, _ int) { d.calls = append(d.calls, gain) }

func TestWindowsMicrophoneCaptureRequiresExplicitRecordBeforePermission(t *testing.T) {
	backend := &fakeWindowsMicrophoneBackend{permission: WindowsCapturePermissionAllowed}
	service := NewWindowsMicrophoneCaptureService(backend, NewCaptureMediaStore(t.TempDir()), nil, nil)
	if _, err := service.Start(context.Background(), WindowsCaptureRequest{}); !errors.Is(err, ErrWindowsCaptureExplicitAction) {
		t.Fatalf("Start error = %v", err)
	}
	if len(backend.prompts) != 0 {
		t.Fatalf("permission touched without explicit Record: %v", backend.prompts)
	}
}

func TestWindowsMicrophoneCaptureNormalStopCreatesOnePrivateDraftAndExcludesStartCue(t *testing.T) {
	root := t.TempDir()
	events := []string{}
	stream := newFakeWindowsMicrophoneStream(
		WindowsCaptureFormat{SampleRate: 48_000, Channels: 2},
		[]float32{0.5, -0.5, 1, 1, -1, -1},
	)
	backend := &fakeWindowsMicrophoneBackend{permission: WindowsCapturePermissionAllowed, stream: stream, events: &events}
	cues := &fakeWindowsCaptureCues{events: &events}
	ducker := &fakeWindowsCaptureDucker{}
	var meters []float32
	service := NewWindowsMicrophoneCaptureService(backend, NewCaptureMediaStore(root), cues, ducker)
	session, err := service.Start(context.Background(), WindowsCaptureRequest{
		ExplicitUserAction: true,
		DeviceID:           "selected-mic",
		Meter:              func(value float32) { meters = append(meters, value) },
	})
	if err != nil {
		t.Fatal(err)
	}
	session.Stop(WindowsCaptureUserStop)
	outcome := awaitWindowsCaptureOutcome(t, session)
	if outcome.Err != nil || outcome.Draft == nil || outcome.Reason != WindowsCaptureUserStop {
		t.Fatalf("outcome = %+v", outcome)
	}
	if outcome.Frames != 3 || outcome.Bytes != 50 {
		t.Fatalf("frames/bytes = %d/%d", outcome.Frames, outcome.Bytes)
	}
	if got := backend.resolved; len(got) != 1 || got[0] != "selected-mic" {
		t.Fatalf("selected device not forwarded: %v", got)
	}
	if len(events) < 3 || events[0] != "permission" || events[1] != string(CuePlayingStart) || events[2] != "open:selected-mic" {
		t.Fatalf("permission/cue/open order = %v", events)
	}
	if len(cues.phases) != 2 || cues.phases[1] != CuePlayingStop {
		t.Fatalf("cue phases = %v", cues.phases)
	}
	if len(ducker.calls) != 2 || ducker.calls[0] != windowsCaptureDuckGain || ducker.calls[1] != 1 {
		t.Fatalf("duck calls = %v", ducker.calls)
	}
	if len(meters) != 1 || meters[0] <= 0 || meters[0] > 1 {
		t.Fatalf("meters = %v", meters)
	}
	data, err := os.ReadFile(outcome.Draft.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !isStructurallyCompleteWAV(data) || len(data) != 50 {
		t.Fatalf("draft is not the exact complete WAV: bytes=%d", len(data))
	}
	entries, err := os.ReadDir(filepath.Join(root, "drafts"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("draft count = %d, err=%v", len(entries), err)
	}
}

func TestWindowsMicrophoneCapturePermissionDenialIsTypedAndLeavesNoMedia(t *testing.T) {
	root := t.TempDir()
	backend := &fakeWindowsMicrophoneBackend{permission: WindowsCapturePermissionDenied}
	service := NewWindowsMicrophoneCaptureService(backend, NewCaptureMediaStore(root), nil, nil)
	_, err := service.Start(context.Background(), WindowsCaptureRequest{ExplicitUserAction: true})
	if !errors.Is(err, ErrWindowsCapturePermission) {
		t.Fatalf("Start error = %v", err)
	}
	if len(backend.prompts) != 1 || !backend.prompts[0] {
		t.Fatalf("explicit prompt contract = %v", backend.prompts)
	}
	if _, statErr := os.Stat(filepath.Join(root, "partials")); !os.IsNotExist(statErr) {
		t.Fatalf("media directories created after denial: %v", statErr)
	}
}

func TestWindowsMicrophoneCaptureQuitCancelsPendingPermissionRequest(t *testing.T) {
	backend := &blockingWindowsPermissionBackend{started: make(chan struct{})}
	service := NewWindowsMicrophoneCaptureService(backend, NewCaptureMediaStore(t.TempDir()), nil, nil)
	result := make(chan error, 1)
	go func() {
		_, err := service.Start(context.Background(), WindowsCaptureRequest{ExplicitUserAction: true})
		result <- err
	}()
	<-backend.started
	service.Shutdown()
	select {
	case err := <-result:
		if !errors.Is(err, ErrWindowsCapturePermission) || !errors.Is(err, context.Canceled) {
			t.Fatalf("Start error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("pending permission request was not cancelled")
	}
}

func TestWindowsMicrophoneCaptureUnsafeLifecycleDeletesPartialAndSkipsStopCue(t *testing.T) {
	reasons := []WindowsCaptureStopReason{
		WindowsCaptureCancel, WindowsCaptureQuit, WindowsCaptureSessionLock,
		WindowsCaptureSuspend, WindowsCaptureDeviceLost, WindowsCapturePermissionRevoke,
	}
	for _, reason := range reasons {
		t.Run(string(reason), func(t *testing.T) {
			root := t.TempDir()
			stream := newFakeWindowsMicrophoneStream(WindowsCaptureFormat{SampleRate: 48_000, Channels: 1}, []float32{0.4, 0.4})
			cues := &fakeWindowsCaptureCues{}
			service := NewWindowsMicrophoneCaptureService(
				&fakeWindowsMicrophoneBackend{permission: WindowsCapturePermissionAllowed, stream: stream},
				NewCaptureMediaStore(root), cues, nil,
			)
			session, err := service.Start(context.Background(), WindowsCaptureRequest{ExplicitUserAction: true})
			if err != nil {
				t.Fatal(err)
			}
			switch reason {
			case WindowsCaptureCancel:
				service.Cancel()
			case WindowsCaptureQuit:
				service.Shutdown()
			case WindowsCaptureSessionLock:
				service.HandleSessionLock()
			case WindowsCaptureSuspend:
				service.HandleSuspend()
			case WindowsCaptureDeviceLost:
				service.HandleDeviceLoss()
			case WindowsCapturePermissionRevoke:
				service.HandlePermissionRevoke()
			}
			outcome := awaitWindowsCaptureOutcome(t, session)
			if outcome.Reason != reason || outcome.Draft != nil {
				t.Fatalf("outcome = %+v", outcome)
			}
			if len(cues.phases) != 1 || cues.phases[0] != CuePlayingStart {
				t.Fatalf("unsafe lifecycle cues = %v", cues.phases)
			}
			partials, err := os.ReadDir(filepath.Join(root, "partials"))
			if err != nil || len(partials) != 0 {
				t.Fatalf("partials = %d err=%v", len(partials), err)
			}
		})
	}
}

func TestWindowsMicrophoneCaptureDurationLimitFinalizesWithExplicitReason(t *testing.T) {
	root := t.TempDir()
	stream := newFakeWindowsMicrophoneStream(WindowsCaptureFormat{SampleRate: 48_000, Channels: 1})
	stream.repeat = make([]float32, 2048)
	for index := range stream.repeat {
		stream.repeat[index] = 0.1
	}
	service := NewWindowsMicrophoneCaptureService(
		&fakeWindowsMicrophoneBackend{permission: WindowsCapturePermissionAllowed, stream: stream},
		NewCaptureMediaStore(root), nil, nil,
	)
	session, err := service.Start(context.Background(), WindowsCaptureRequest{ExplicitUserAction: true})
	if err != nil {
		t.Fatal(err)
	}
	outcome := awaitWindowsCaptureOutcome(t, session)
	if outcome.Reason != WindowsCaptureDurationLimit || outcome.Draft == nil || outcome.Err != nil {
		t.Fatalf("outcome = %+v", outcome)
	}
	if outcome.Frames != WindowsCaptureMaximumSeconds*WindowsCaptureSampleRate || outcome.Bytes > WindowsCaptureMaximumBytes {
		t.Fatalf("hard limits frames=%d bytes=%d", outcome.Frames, outcome.Bytes)
	}
}

func TestWindowsMonoResamplerDownmixesAndConvertsToCanonicalRate(t *testing.T) {
	resampler := windowsMonoResampler{inputRate: 24_000}
	got := resampler.append(nil, []float32{1, -1, 0.5, 0.5}, 2)
	if len(got) != 4 || got[0] != 0 || got[1] != 0 || got[2] <= 0 || got[3] <= 0 {
		t.Fatalf("resampled = %v", got)
	}
}

func TestWindowsMicrophoneProductionPackageDeclaresAndStagesCaptureBridge(t *testing.T) {
	manifest, err := os.ReadFile(filepath.Join("msix", "AppxManifest.xml.in"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifest), `<DeviceCapability Name="microphone" />`) {
		t.Fatal("production MSIX does not declare microphone capability")
	}
	workflow, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, required := range []string{"native/pulsar-capture", "pulsar-capture.dll", "stage/pulsar-capture.dll"} {
		if !strings.Contains(string(workflow), required) {
			t.Fatalf("release workflow does not stage %q", required)
		}
	}
}

func awaitWindowsCaptureOutcome(t *testing.T, session *WindowsCaptureSession) WindowsCaptureOutcome {
	t.Helper()
	select {
	case outcome := <-session.Done():
		return outcome
	case <-time.After(5 * time.Second):
		t.Fatal("capture outcome timed out")
		return WindowsCaptureOutcome{}
	}
}
