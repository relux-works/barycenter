package main

import (
	"context"
	"errors"
	"sync"
)

type WindowsRecordingSession interface {
	Stop(WindowsCaptureStopReason)
	Done() <-chan WindowsCaptureOutcome
}

type WindowsRecordingCapture interface {
	Start(context.Context, WindowsCaptureRequest) (WindowsRecordingSession, error)
	Stop(WindowsCaptureStopReason)
}

type windowsMicrophoneRecordingCapture struct {
	service *WindowsMicrophoneCaptureService
}

func (c windowsMicrophoneRecordingCapture) Start(ctx context.Context, request WindowsCaptureRequest) (WindowsRecordingSession, error) {
	return c.service.Start(ctx, request)
}

func (c windowsMicrophoneRecordingCapture) Stop(reason WindowsCaptureStopReason) {
	if c.service != nil {
		c.service.stopActive(reason)
	}
}

// WindowsRecordingController is the single state owner shared by window,
// tray and hotkey actions. Operations that can enter AppCapability, WASAPI or
// cue playback always run away from the Win32 message thread.
type WindowsRecordingController struct {
	capture WindowsRecordingCapture

	mu            sync.RWMutex
	state         ShellRecording
	available     bool
	generation    uint64
	session       WindowsRecordingSession
	startCancel   context.CancelFunc
	requestedStop WindowsCaptureStopReason
	pending       sync.WaitGroup
}

func NewWindowsRecordingController(capture WindowsRecordingCapture) *WindowsRecordingController {
	return &WindowsRecordingController{
		capture: capture, available: capture != nil,
		state: func() ShellRecording {
			if capture == nil {
				return ShellRecordingUnavailable
			}
			return ShellRecordingIdle
		}(),
	}
}

func (c *WindowsRecordingController) Snapshot() (ShellRecording, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.state, c.available
}

func (c *WindowsRecordingController) Toggle() {
	c.mu.Lock()
	if !c.available || c.capture == nil {
		c.mu.Unlock()
		return
	}
	switch c.state {
	case ShellRecordingIdle, ShellRecordingFailed:
		c.generation++
		generation := c.generation
		ctx, cancel := context.WithCancel(context.Background())
		c.startCancel = cancel
		c.requestedStop = ""
		c.state = ShellRecordingProcessing
		c.pending.Add(1)
		c.mu.Unlock()
		go c.start(generation, ctx)
		return
	case ShellRecordingActive, ShellRecordingProcessing:
		c.requestedStop = WindowsCaptureUserStop
		cancel := c.startCancel
		capture := c.capture
		c.state = ShellRecordingProcessing
		c.mu.Unlock()
		go func() {
			capture.Stop(WindowsCaptureUserStop)
			if cancel != nil {
				cancel()
			}
		}()
		return
	default:
		c.mu.Unlock()
	}
}

func (c *WindowsRecordingController) Cancel()            { c.stop(WindowsCaptureCancel) }
func (c *WindowsRecordingController) HandleSessionLock() { c.stop(WindowsCaptureSessionLock) }
func (c *WindowsRecordingController) HandleSuspend()     { c.stop(WindowsCaptureSuspend) }
func (c *WindowsRecordingController) Shutdown() {
	c.mu.Lock()
	c.available = false
	if c.state != ShellRecordingActive && c.state != ShellRecordingProcessing {
		c.state = ShellRecordingUnavailable
	}
	c.mu.Unlock()
	c.stop(WindowsCaptureQuit)
}

func (c *WindowsRecordingController) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		c.pending.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (c *WindowsRecordingController) stop(reason WindowsCaptureStopReason) {
	c.mu.Lock()
	if c.capture == nil || (c.state != ShellRecordingActive && c.state != ShellRecordingProcessing) {
		c.mu.Unlock()
		return
	}
	c.requestedStop = reason
	c.state = ShellRecordingProcessing
	cancel := c.startCancel
	capture := c.capture
	c.mu.Unlock()
	go func() {
		capture.Stop(reason)
		if cancel != nil {
			cancel()
		}
	}()
}

func (c *WindowsRecordingController) start(generation uint64, ctx context.Context) {
	session, err := c.capture.Start(ctx, WindowsCaptureRequest{
		ExplicitUserAction: true,
		MediaClass:         CaptureUserRecording,
	})
	c.mu.Lock()
	if generation != c.generation || !c.available {
		if !c.available {
			c.state = ShellRecordingUnavailable
		}
		c.mu.Unlock()
		if session != nil {
			session.Stop(WindowsCaptureCancel)
		}
		c.pending.Done()
		return
	}
	c.startCancel = nil
	if err != nil || session == nil {
		if c.requestedStop != "" || errors.Is(err, context.Canceled) {
			c.state = ShellRecordingIdle
		} else {
			c.state = ShellRecordingFailed
		}
		c.requestedStop = ""
		c.mu.Unlock()
		c.pending.Done()
		return
	}
	c.session = session
	c.state = ShellRecordingActive
	requestedStop := c.requestedStop
	c.mu.Unlock()
	if requestedStop != "" {
		go session.Stop(requestedStop)
	}
	go c.finish(generation, session)
}

func (c *WindowsRecordingController) finish(generation uint64, session WindowsRecordingSession) {
	defer c.pending.Done()
	outcome, ok := <-session.Done()
	c.mu.Lock()
	defer c.mu.Unlock()
	if generation != c.generation || c.session != session {
		return
	}
	c.session = nil
	c.requestedStop = ""
	if !c.available {
		c.state = ShellRecordingUnavailable
	} else if !ok || outcome.Err != nil || outcome.Reason == WindowsCaptureBackendFailure {
		c.state = ShellRecordingFailed
	} else {
		c.state = ShellRecordingIdle
	}
}
