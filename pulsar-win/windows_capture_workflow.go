package main

import (
	"context"
	"sync"
)

// WindowsCaptureInput is the stable identity shown by the main window. IDs
// are passed back to the AppCapability/WASAPI backend; names are presentation
// only and never used to reopen a device.
type WindowsCaptureInput struct {
	ID   string
	Name string
}

// WindowsCaptureWorkflowSnapshot is the local-only projection shared by the
// main window and tray. Draft paths deliberately stay out of this type.
type WindowsCaptureWorkflowSnapshot struct {
	Available               bool
	SelfTestPhase           WindowsLocalSelfTestPhase
	Meter                   float32
	Failure                 string
	DraftAvailable          bool
	DraftName               string
	RecordingDraftAvailable bool
	Review                  *WindowsShortAudioReview
	Inputs                  []WindowsCaptureInput
	SelectedInput           int
}

type WindowsCaptureWorkflowController struct {
	recording *WindowsRecordingController
	selfTest  *WindowsLocalSelfTestService
	picker    func(context.Context, uintptr) (WindowsBrokeredAudioFile, error)

	mu                sync.RWMutex
	snapshot          WindowsCaptureWorkflowSnapshot
	ctx               context.Context
	cancel            context.CancelFunc
	pending           sync.WaitGroup
	platformClose     func()
	platformCloseOnce sync.Once
}

func (c *WindowsCaptureWorkflowController) SetPicker(picker func(context.Context, uintptr) (WindowsBrokeredAudioFile, error)) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.picker = picker
	c.mu.Unlock()
}

func NewWindowsCaptureWorkflowController(recording *WindowsRecordingController, selfTest *WindowsLocalSelfTestService, inputs []WindowsCaptureInput) *WindowsCaptureWorkflowController {
	ctx, cancel := context.WithCancel(context.Background())
	c := &WindowsCaptureWorkflowController{recording: recording, selfTest: selfTest, ctx: ctx, cancel: cancel}
	c.snapshot = WindowsCaptureWorkflowSnapshot{
		Available:     recording != nil && selfTest != nil,
		SelfTestPhase: WindowsLocalSelfTestIdle,
		Inputs:        append([]WindowsCaptureInput(nil), inputs...),
	}
	if selfTest != nil {
		selfTest.SetEventHandler(c.handleSelfTestEvent)
	}
	if recording != nil {
		recording.ConfigureRequest(c.recordingRequest, c.handleRecordingOutcome)
	}
	return c
}

func (c *WindowsCaptureWorkflowController) SetPlatformClose(close func()) {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.platformClose = close
	c.mu.Unlock()
}

func (c *WindowsCaptureWorkflowController) ClosePlatform() {
	if c == nil {
		return
	}
	c.platformCloseOnce.Do(func() {
		c.mu.RLock()
		close := c.platformClose
		c.mu.RUnlock()
		if close != nil {
			close()
		}
	})
}

func (c *WindowsCaptureWorkflowController) Snapshot() (ShellRecording, bool) {
	if c == nil || c.recording == nil {
		return ShellRecordingUnavailable, false
	}
	return c.recording.Snapshot()
}

func (c *WindowsCaptureWorkflowController) LocalSnapshot() WindowsCaptureWorkflowSnapshot {
	if c == nil {
		return WindowsCaptureWorkflowSnapshot{}
	}
	c.mu.RLock()
	defer c.mu.RUnlock()
	result := c.snapshot
	result.Inputs = append([]WindowsCaptureInput(nil), c.snapshot.Inputs...)
	if c.snapshot.Review != nil {
		review := *c.snapshot.Review
		result.Review = &review
	}
	return result
}

func (c *WindowsCaptureWorkflowController) Toggle() {
	if c == nil || c.recording == nil || c.selfTestBusy() {
		return
	}
	c.recording.Toggle()
}

func (c *WindowsCaptureWorkflowController) TryLocally() {
	if c == nil || c.selfTest == nil || c.recordingBusy() {
		return
	}
	if !c.beginOperation() {
		return
	}
	go func() { defer c.pending.Done(); _ = c.selfTest.RecordFiveSeconds(c.ctx, c.selectedInputID()) }()
}

func (c *WindowsCaptureWorkflowController) PlayBuiltinCue() {
	if c == nil || c.selfTest == nil || c.recordingBusy() {
		return
	}
	if !c.beginOperation() {
		return
	}
	go func() { defer c.pending.Done(); _ = c.selfTest.PlayBuiltinCue(c.ctx) }()
}

func (c *WindowsCaptureWorkflowController) AcceptBrokeredFile(file WindowsBrokeredAudioFile) {
	if c == nil || c.selfTest == nil || c.recordingBusy() {
		if file.Release != nil {
			file.Release()
		}
		return
	}
	if !c.beginOperation() {
		if file.Release != nil {
			file.Release()
		}
		return
	}
	go func() { defer c.pending.Done(); _, _ = c.selfTest.AcceptFile(file) }()
}

func (c *WindowsCaptureWorkflowController) ChooseFile(owner uintptr) {
	if c == nil || c.recordingBusy() || c.selfTestBusy() {
		return
	}
	c.mu.RLock()
	picker := c.picker
	c.mu.RUnlock()
	if picker == nil {
		return
	}
	if !c.beginOperation() {
		return
	}
	go func() {
		defer c.pending.Done()
		file, err := picker(c.ctx, owner)
		if err != nil {
			if c.ctx.Err() == nil {
				c.mu.Lock()
				c.snapshot.Failure = "file_picker_failed"
				c.mu.Unlock()
			}
			return
		}
		if c.ctx.Err() != nil {
			if file.Release != nil {
				file.Release()
			}
			return
		}
		_, _ = c.selfTest.AcceptFile(file)
	}()
}

func (c *WindowsCaptureWorkflowController) DeleteLocalDraft() {
	if c != nil && c.selfTest != nil && !c.recordingBusy() {
		c.selfTest.DeleteDraft()
	}
}

func (c *WindowsCaptureWorkflowController) SelectNextInput() {
	if c == nil || c.recordingBusy() || c.selfTestBusy() {
		return
	}
	c.mu.Lock()
	if len(c.snapshot.Inputs) > 0 {
		c.snapshot.SelectedInput = (c.snapshot.SelectedInput + 1) % len(c.snapshot.Inputs)
	}
	c.mu.Unlock()
}

func (c *WindowsCaptureWorkflowController) Cancel() {
	if c == nil {
		return
	}
	if c.recordingBusy() {
		c.recording.Cancel()
		return
	}
	if c.selfTest != nil && c.selfTestBusy() {
		c.selfTest.Close()
	}
}

func (c *WindowsCaptureWorkflowController) HandleSessionLock() {
	if c == nil {
		return
	}
	if c.recording != nil {
		c.recording.HandleSessionLock()
	}
	if c.selfTest != nil {
		c.selfTest.Close()
	}
}

func (c *WindowsCaptureWorkflowController) HandleSuspend() {
	if c == nil {
		return
	}
	if c.recording != nil {
		c.recording.HandleSuspend()
	}
	if c.selfTest != nil {
		c.selfTest.Close()
	}
}

func (c *WindowsCaptureWorkflowController) Shutdown() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.snapshot.Available = false
	if c.cancel != nil {
		c.cancel()
	}
	c.mu.Unlock()
	if c.recording != nil {
		c.recording.Shutdown()
	}
	if c.selfTest != nil {
		c.selfTest.Close()
	}
}

func (c *WindowsCaptureWorkflowController) Wait(ctx context.Context) error {
	if c == nil {
		return nil
	}
	done := make(chan struct{})
	go func() {
		if c.recording != nil {
			_ = c.recording.Wait(context.Background())
		}
		if c.selfTest != nil {
			_ = c.selfTest.Wait(context.Background())
		}
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

func (c *WindowsCaptureWorkflowController) beginOperation() bool {
	if c == nil {
		return false
	}
	c.mu.RLock()
	available := c.snapshot.Available && c.ctx != nil && c.ctx.Err() == nil
	c.mu.RUnlock()
	if !available {
		return false
	}
	c.pending.Add(1)
	return true
}

func (c *WindowsCaptureWorkflowController) recordingRequest() WindowsCaptureRequest {
	return WindowsCaptureRequest{DeviceID: c.selectedInputID(), Meter: func(value float32) {
		c.mu.Lock()
		c.snapshot.Meter = value
		c.mu.Unlock()
	}}
}

func (c *WindowsCaptureWorkflowController) selectedInputID() string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	if c.snapshot.SelectedInput < 0 || c.snapshot.SelectedInput >= len(c.snapshot.Inputs) {
		return ""
	}
	return c.snapshot.Inputs[c.snapshot.SelectedInput].ID
}

func (c *WindowsCaptureWorkflowController) recordingBusy() bool {
	if c == nil || c.recording == nil {
		return false
	}
	state, _ := c.recording.Snapshot()
	return state == ShellRecordingActive || state == ShellRecordingProcessing
}

func (c *WindowsCaptureWorkflowController) selfTestBusy() bool {
	if c == nil || c.selfTest == nil {
		return false
	}
	phase := c.selfTest.Phase()
	return !windowsLocalSelfTestCanStart(phase)
}

func (c *WindowsCaptureWorkflowController) handleRecordingOutcome(outcome WindowsCaptureOutcome) {
	c.mu.Lock()
	c.snapshot.Meter = 0
	c.snapshot.Failure = ""
	if outcome.Err != nil || outcome.Reason == WindowsCaptureBackendFailure {
		c.snapshot.Failure = "recording_failed"
	}
	if outcome.Draft != nil {
		c.snapshot.RecordingDraftAvailable = true
	}
	c.mu.Unlock()
}

func (c *WindowsCaptureWorkflowController) handleSelfTestEvent(event WindowsLocalSelfTestEvent) {
	c.mu.Lock()
	if event.Phase != "" {
		c.snapshot.SelfTestPhase = event.Phase
		if event.Phase != WindowsLocalSelfTestRecording {
			c.snapshot.Meter = 0
		}
	}
	if event.Meter >= 0 {
		c.snapshot.Meter = event.Meter
	}
	if event.Review != nil {
		review := *event.Review
		c.snapshot.Review = &review
		c.snapshot.DraftName = review.Filename
	}
	if event.Draft != nil {
		c.snapshot.DraftAvailable = true
	}
	if event.Failure != "" {
		c.snapshot.Failure = event.Failure
	}
	if event.Phase == WindowsLocalSelfTestIdle {
		c.snapshot.DraftAvailable = false
		c.snapshot.DraftName = ""
		c.snapshot.Review = nil
		c.snapshot.Failure = ""
	}
	c.mu.Unlock()
}
