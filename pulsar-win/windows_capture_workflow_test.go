package main

import (
	"context"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"
)

func TestWindowsCaptureWorkflowSerializesRecordingAndSelfTest(t *testing.T) {
	root := t.TempDir()
	store := NewCaptureMediaStore(filepath.Join(root, "media"))
	selfCapture := &fakeWindowsSelfTestCapture{store: store, events: make(chan string, 8)}
	selfTest := newWindowsSelfTestServiceForTest(t, selfCapture, &fakeWindowsLocalOutput{}, store, time.Hour)
	recordingCapture := newFakeWindowsRecordingCapture()
	recording := NewWindowsRecordingController(recordingCapture)
	workflow := NewWindowsCaptureWorkflowController(recording, selfTest, []WindowsCaptureInput{{ID: "one", Name: "One"}, {ID: "two", Name: "Two"}})

	workflow.SelectNextInput()
	workflow.Toggle()
	waitSignal(t, recordingCapture.start)
	recordingSession := newFakeWindowsRecordingSession()
	recordingCapture.result <- fakeWindowsRecordingStart{session: recordingSession}
	waitRecordingState(t, recording, ShellRecordingActive)
	workflow.TryLocally()
	select {
	case <-selfCapture.events:
		t.Fatal("self-test crossed an active normal recording")
	case <-time.After(10 * time.Millisecond):
	}
	recordingCapture.mu.Lock()
	request := recordingCapture.requests[0]
	recordingCapture.mu.Unlock()
	if request.DeviceID != "two" || request.Meter == nil || request.MediaClass != CaptureUserRecording {
		t.Fatalf("normal recording request=%+v", request)
	}
	recordingSession.done <- WindowsCaptureOutcome{Reason: WindowsCaptureUserStop, Draft: &CaptureMediaHandle{Class: CaptureUserRecording, State: CaptureDurableUnsent}}
	close(recordingSession.done)
	waitRecordingState(t, recording, ShellRecordingIdle)
	if !workflow.LocalSnapshot().RecordingDraftAvailable {
		t.Fatal("normal durable draft was not projected")
	}

	workflow.TryLocally()
	select {
	case <-selfCapture.events:
	case <-time.After(time.Second):
		t.Fatal("self-test did not start")
	}
	if selfCapture.request.DeviceID != "two" || selfCapture.request.MediaClass != CaptureSelfTest {
		t.Fatalf("self-test request=%+v", selfCapture.request)
	}
	workflow.Toggle()
	if state, _ := recording.Snapshot(); state != ShellRecordingIdle {
		t.Fatalf("normal recording entered during self-test: %s", state)
	}
	workflow.Cancel()
	if selfTest.Phase() != WindowsLocalSelfTestIdle {
		t.Fatalf("cancel left self-test phase=%s", selfTest.Phase())
	}
}

func TestWindowsCaptureWorkflowShutdownCancelsPickerDrainsAndClosesPlatformOnce(t *testing.T) {
	store := NewCaptureMediaStore(t.TempDir())
	selfTest := newWindowsSelfTestServiceForTest(t, failingWindowsSelfTestCapture{err: ErrWindowsCapturePermission}, &fakeWindowsLocalOutput{}, store, time.Hour)
	workflow := NewWindowsCaptureWorkflowController(NewWindowsRecordingController(nil), selfTest, nil)
	started := make(chan struct{})
	workflow.SetPicker(func(ctx context.Context, _ uintptr) (WindowsBrokeredAudioFile, error) {
		close(started)
		<-ctx.Done()
		return WindowsBrokeredAudioFile{}, ctx.Err()
	})
	var closes atomic.Int32
	workflow.SetPlatformClose(func() { closes.Add(1) })
	workflow.ChooseFile(1)
	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("picker did not start")
	}
	workflow.Shutdown()
	drain, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := workflow.Wait(drain); err != nil {
		t.Fatalf("shutdown did not drain: %v", err)
	}
	workflow.ClosePlatform()
	workflow.ClosePlatform()
	if closes.Load() != 1 {
		t.Fatalf("platform closes=%d", closes.Load())
	}
	if snapshot := workflow.LocalSnapshot(); snapshot.Available {
		t.Fatal("shutdown projection remained available")
	}
}

func TestWindowsCaptureWorkflowBrokeredPickerAndHonestProjection(t *testing.T) {
	store := NewCaptureMediaStore(t.TempDir())
	selfTest := newWindowsSelfTestServiceForTest(t, failingWindowsSelfTestCapture{err: ErrWindowsCapturePermission}, &fakeWindowsLocalOutput{}, store, time.Hour)
	workflow := NewWindowsCaptureWorkflowController(NewWindowsRecordingController(nil), selfTest, nil)
	workflow.SetPicker(func(context.Context, uintptr) (WindowsBrokeredAudioFile, error) {
		return brokeredBytes(`C:\Users\Ivan\voice.wav`, makeWAV16(1, 48_000, make([]int16, 4_800))), nil
	})
	workflow.ChooseFile(42)
	deadline := time.Now().Add(time.Second)
	var snapshot WindowsCaptureWorkflowSnapshot
	for time.Now().Before(deadline) {
		snapshot = workflow.LocalSnapshot()
		if snapshot.DraftAvailable {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if !snapshot.DraftAvailable || snapshot.DraftName != "voice.wav" || snapshot.Review == nil || !snapshot.Review.Eligible() {
		t.Fatalf("brokered picker projection=%+v", snapshot)
	}
	workflow.DeleteLocalDraft()
	if snapshot = workflow.LocalSnapshot(); snapshot.DraftAvailable || snapshot.DraftName != "" {
		t.Fatalf("deleted draft projection=%+v", snapshot)
	}
}
