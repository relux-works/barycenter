package main

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

type fakeWindowsRecordingSession struct {
	done  chan WindowsCaptureOutcome
	mu    sync.Mutex
	stops []WindowsCaptureStopReason
}

func newFakeWindowsRecordingSession() *fakeWindowsRecordingSession {
	return &fakeWindowsRecordingSession{done: make(chan WindowsCaptureOutcome, 1)}
}

func (s *fakeWindowsRecordingSession) Stop(reason WindowsCaptureStopReason) {
	s.mu.Lock()
	s.stops = append(s.stops, reason)
	s.mu.Unlock()
}
func (s *fakeWindowsRecordingSession) Done() <-chan WindowsCaptureOutcome { return s.done }

type fakeWindowsRecordingCapture struct {
	start    chan struct{}
	result   chan fakeWindowsRecordingStart
	mu       sync.Mutex
	stops    []WindowsCaptureStopReason
	requests []WindowsCaptureRequest
}
type fakeWindowsRecordingStart struct {
	session *fakeWindowsRecordingSession
	err     error
}

func newFakeWindowsRecordingCapture() *fakeWindowsRecordingCapture {
	return &fakeWindowsRecordingCapture{start: make(chan struct{}, 8), result: make(chan fakeWindowsRecordingStart, 8)}
}
func (c *fakeWindowsRecordingCapture) Start(ctx context.Context, request WindowsCaptureRequest) (WindowsRecordingSession, error) {
	c.mu.Lock()
	c.requests = append(c.requests, request)
	c.mu.Unlock()
	c.start <- struct{}{}
	select {
	case result := <-c.result:
		return result.session, result.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
func (c *fakeWindowsRecordingCapture) Stop(reason WindowsCaptureStopReason) {
	c.mu.Lock()
	c.stops = append(c.stops, reason)
	c.mu.Unlock()
}

func TestWindowsRecordingControllerToggleStopAndStateProjection(t *testing.T) {
	capture := newFakeWindowsRecordingCapture()
	controller := NewWindowsRecordingController(capture)
	if state, available := controller.Snapshot(); state != ShellRecordingIdle || !available {
		t.Fatalf("initial state = %s available=%v", state, available)
	}
	controller.Toggle()
	waitSignal(t, capture.start)
	if state, _ := controller.Snapshot(); state != ShellRecordingProcessing {
		t.Fatalf("state while start is pending = %s", state)
	}
	session := newFakeWindowsRecordingSession()
	capture.result <- fakeWindowsRecordingStart{session: session}
	waitRecordingState(t, controller, ShellRecordingActive)
	controller.Toggle()
	waitCaptureStop(t, capture, WindowsCaptureUserStop)
	session.done <- WindowsCaptureOutcome{Reason: WindowsCaptureUserStop, Draft: &CaptureMediaHandle{}}
	close(session.done)
	waitRecordingState(t, controller, ShellRecordingIdle)
	capture.mu.Lock()
	request := capture.requests[0]
	capture.mu.Unlock()
	if !request.ExplicitUserAction || request.MediaClass != CaptureUserRecording {
		t.Fatalf("capture request = %+v", request)
	}
}

func TestWindowsRecordingControllerCancelPendingStartAndLifecycleReasons(t *testing.T) {
	for _, test := range []struct {
		name   string
		invoke func(*WindowsRecordingController)
		want   WindowsCaptureStopReason
	}{
		{"cancel", (*WindowsRecordingController).Cancel, WindowsCaptureCancel},
		{"lock", (*WindowsRecordingController).HandleSessionLock, WindowsCaptureSessionLock},
		{"suspend", (*WindowsRecordingController).HandleSuspend, WindowsCaptureSuspend},
	} {
		t.Run(test.name, func(t *testing.T) {
			capture := newFakeWindowsRecordingCapture()
			controller := NewWindowsRecordingController(capture)
			controller.Toggle()
			waitSignal(t, capture.start)
			test.invoke(controller)
			waitCaptureStop(t, capture, test.want)
			waitRecordingState(t, controller, ShellRecordingIdle)
		})
	}
}

func TestWindowsRecordingControllerFailureAndShutdown(t *testing.T) {
	capture := newFakeWindowsRecordingCapture()
	controller := NewWindowsRecordingController(capture)
	controller.Toggle()
	waitSignal(t, capture.start)
	capture.result <- fakeWindowsRecordingStart{err: errors.New("permission denied")}
	waitRecordingState(t, controller, ShellRecordingFailed)
	controller.Shutdown()
	if state, available := controller.Snapshot(); state != ShellRecordingUnavailable || available {
		t.Fatalf("shutdown state = %s available=%v", state, available)
	}
}

func TestWindowsRecordingControllerShutdownDrainsActiveCapture(t *testing.T) {
	capture := newFakeWindowsRecordingCapture()
	controller := NewWindowsRecordingController(capture)
	controller.Toggle()
	waitSignal(t, capture.start)
	session := newFakeWindowsRecordingSession()
	capture.result <- fakeWindowsRecordingStart{session: session}
	waitRecordingState(t, controller, ShellRecordingActive)
	controller.Shutdown()
	waitCaptureStop(t, capture, WindowsCaptureQuit)
	shortContext, cancel := context.WithTimeout(context.Background(), 5*time.Millisecond)
	defer cancel()
	if err := controller.Wait(shortContext); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("active capture drain returned early: %v", err)
	}
	session.done <- WindowsCaptureOutcome{Reason: WindowsCaptureQuit}
	close(session.done)
	drainContext, drainCancel := context.WithTimeout(context.Background(), time.Second)
	defer drainCancel()
	if err := controller.Wait(drainContext); err != nil {
		t.Fatalf("capture did not drain: %v", err)
	}
	if state, available := controller.Snapshot(); state != ShellRecordingUnavailable || available {
		t.Fatalf("drained shutdown state=%s available=%v", state, available)
	}
}

func waitSignal(t *testing.T, signal <-chan struct{}) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for start")
	}
}

func waitRecordingState(t *testing.T, controller *WindowsRecordingController, want ShellRecording) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		if state, _ := controller.Snapshot(); state == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	state, _ := controller.Snapshot()
	t.Fatalf("recording state = %s, want %s", state, want)
}

func waitCaptureStop(t *testing.T, capture *fakeWindowsRecordingCapture, want WindowsCaptureStopReason) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		capture.mu.Lock()
		for _, reason := range capture.stops {
			if reason == want {
				capture.mu.Unlock()
				return
			}
		}
		capture.mu.Unlock()
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("capture stop %s not observed", want)
}
