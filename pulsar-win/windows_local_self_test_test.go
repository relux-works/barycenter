package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	protocol "relux.works/duet/pulsar-win/wire"
)

type fakeWindowsLocalMixer struct {
	mu          sync.Mutex
	prepared    []*PreparedMediaClip
	plans       []MediaClipPlayPlan
	ended       func(int64)
	cancelled   []protocol.CancelMediaPayload
	cancelDone  func(bool, error)
	deferCancel bool
	disposed    int
	prepareErr  error
	armErr      error
}

func (m *fakeWindowsLocalMixer) Prepare(localPath, delivery string) (*PreparedMediaClip, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.prepareErr != nil {
		return nil, m.prepareErr
	}
	clip := &PreparedMediaClip{LocalPath: localPath, DecodedDurationMS: 100, Decoder: delivery}
	m.prepared = append(m.prepared, clip)
	return clip, nil
}

func (m *fakeWindowsLocalMixer) Arm(_ *PreparedMediaClip, plan MediaClipPlayPlan, _ func(int64), ended func(int64)) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.plans = append(m.plans, plan)
	m.ended = ended
	return m.armErr
}

func (m *fakeWindowsLocalMixer) Cancel(_ *PreparedMediaClip, command protocol.CancelMediaPayload, done func(bool, error)) {
	m.mu.Lock()
	m.cancelled = append(m.cancelled, command)
	if m.deferCancel {
		m.cancelDone = done
		m.mu.Unlock()
		return
	}
	m.mu.Unlock()
	done(false, nil)
}

func (m *fakeWindowsLocalMixer) Dispose(_ *PreparedMediaClip) {
	m.mu.Lock()
	m.disposed++
	m.mu.Unlock()
}

func (m *fakeWindowsLocalMixer) finish() {
	m.mu.Lock()
	ended := m.ended
	m.mu.Unlock()
	if ended != nil {
		ended(1234)
	}
}

func TestWindowsProductionLocalOutputUsesMixerWithoutTelemetry(t *testing.T) {
	mixer := &fakeWindowsLocalMixer{}
	now := time.Unix(1_700_000_000, 0)
	output := newWindowsProductionLocalClipOutput(mixer, func() time.Time { return now })
	done := make(chan error, 1)
	output.Play(context.Background(), "C:/private/cue.wav", func(err error) { done <- err })

	mixer.mu.Lock()
	if len(mixer.plans) != 1 {
		mixer.mu.Unlock()
		t.Fatalf("plans=%d", len(mixer.plans))
	}
	plan := mixer.plans[0]
	mixer.mu.Unlock()
	if plan.Payload.TransmissionID != "local-self-test-1" || plan.Payload.Delivery != "overlay" ||
		plan.LocalStartMS != now.Add(30*time.Millisecond).UnixMilli() ||
		plan.LocalStartDeadlineMS != now.Add(130*time.Millisecond).UnixMilli() {
		t.Fatalf("unexpected local plan: %+v", plan)
	}
	if plan.Control.ReportStarted || plan.Control.ReportEnded || plan.Control.DuckDB != 0 {
		t.Fatalf("local plan enabled telemetry or duck: %+v", plan.Control)
	}

	second := make(chan error, 1)
	output.Play(context.Background(), "C:/private/second.wav", func(err error) { second <- err })
	if err := <-second; !errors.Is(err, ErrWindowsLocalOutputBusy) {
		t.Fatalf("concurrent playback err=%v", err)
	}
	mixer.finish()
	if err := <-done; err != nil {
		t.Fatalf("playback err=%v", err)
	}
	mixer.mu.Lock()
	defer mixer.mu.Unlock()
	if mixer.disposed != 1 || len(mixer.cancelled) != 0 {
		t.Fatalf("disposed=%d cancelled=%v", mixer.disposed, mixer.cancelled)
	}
}

func TestWindowsProductionLocalOutputCancelDisposesOnce(t *testing.T) {
	mixer := &fakeWindowsLocalMixer{deferCancel: true}
	output := newWindowsProductionLocalClipOutput(mixer, time.Now)
	output.Play(context.Background(), "cue.wav", func(error) {})
	output.Cancel()
	output.Cancel()
	mixer.mu.Lock()
	if len(mixer.cancelled) != 1 || mixer.cancelled[0].Reason != "local_close" || mixer.disposed != 0 {
		t.Fatalf("cancelled=%+v disposed=%d", mixer.cancelled, mixer.disposed)
	}
	done := mixer.cancelDone
	mixer.mu.Unlock()
	done(false, nil)
	mixer.mu.Lock()
	defer mixer.mu.Unlock()
	if mixer.disposed != 1 {
		t.Fatalf("deferred cancel disposed=%d", mixer.disposed)
	}
}

func TestWindowsLocalOutputDecodeFailureAndRecordingCueAdapter(t *testing.T) {
	mixer := &fakeWindowsLocalMixer{prepareErr: errors.New("decode")}
	output := newWindowsProductionLocalClipOutput(mixer, nil)
	done := make(chan error, 1)
	output.Play(context.Background(), "broken.wav", func(err error) { done <- err })
	if err := <-done; !errors.Is(err, ErrWindowsLocalOutputDecode) {
		t.Fatalf("decode err=%v", err)
	}
	mixer.mu.Lock()
	mixer.prepareErr = nil
	mixer.mu.Unlock()
	output.Play(context.Background(), "valid.wav", func(err error) { done <- err })
	mixer.finish()
	if err := <-done; err != nil {
		t.Fatalf("reservation did not clear after decode failure: %v", err)
	}

	local := &fakeWindowsLocalOutput{}
	cues := WindowsLocalRecordingCuePlayer{Output: local, CuePath: "cue.wav"}
	if err := cues.PlayRecordingCue(context.Background(), CuePlayingStart); err != nil {
		t.Fatalf("start cue: %v", err)
	}
	if err := cues.PlayRecordingCue(context.Background(), CueCapturing); !errors.Is(err, ErrRecordingCueUnavailable) {
		t.Fatalf("invalid cue phase err=%v", err)
	}
}

type fakeWindowsLocalOutput struct {
	mu      sync.Mutex
	paths   []string
	cancels int
	err     error
}

func (o *fakeWindowsLocalOutput) Play(_ context.Context, file string, done func(error)) {
	o.mu.Lock()
	o.paths = append(o.paths, file)
	err := o.err
	o.mu.Unlock()
	done(err)
}

func (o *fakeWindowsLocalOutput) Cancel() {
	o.mu.Lock()
	o.cancels++
	o.mu.Unlock()
}

type fakeWindowsSelfTestCapture struct {
	mu      sync.Mutex
	store   *CaptureMediaStore
	events  chan string
	request WindowsCaptureRequest
	session *fakeWindowsSelfTestSession
}

func (c *fakeWindowsSelfTestCapture) Start(_ context.Context, request WindowsCaptureRequest) (WindowsSelfTestCaptureSession, error) {
	session := &fakeWindowsSelfTestSession{store: c.store, events: c.events, done: make(chan WindowsCaptureOutcome, 1)}
	c.mu.Lock()
	c.request = request
	c.session = session
	c.mu.Unlock()
	c.events <- "start-cue"
	if request.Meter != nil {
		request.Meter(0.25)
	}
	return session, nil
}

func (c *fakeWindowsSelfTestCapture) Cancel() {
	c.mu.Lock()
	session := c.session
	c.mu.Unlock()
	if session != nil {
		session.Stop(WindowsCaptureCancel)
	}
}

func (c *fakeWindowsSelfTestCapture) snapshot() (WindowsCaptureRequest, *fakeWindowsSelfTestSession) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.request, c.session
}

type fakeWindowsSelfTestSession struct {
	store  *CaptureMediaStore
	events chan string
	done   chan WindowsCaptureOutcome
	once   sync.Once
	stopAt time.Time
}

func (s *fakeWindowsSelfTestSession) Stop(reason WindowsCaptureStopReason) {
	s.once.Do(func() {
		s.stopAt = time.Now()
		if reason != WindowsCaptureUserStop {
			s.done <- WindowsCaptureOutcome{Reason: reason}
			close(s.done)
			return
		}
		s.events <- "stop-cue"
		draft, err := createWindowsTestDraft(s.store, CaptureSelfTest, makeWAV16(1, 48_000, make([]int16, 4_800)))
		outcome := WindowsCaptureOutcome{Reason: reason, Err: err}
		if err == nil {
			outcome.Draft = &draft
		}
		s.done <- outcome
		close(s.done)
	})
}

func (s *fakeWindowsSelfTestSession) Done() <-chan WindowsCaptureOutcome { return s.done }

func TestWindowsLocalSelfTestExactRecordCuePlaybackAndCleanup(t *testing.T) {
	if WindowsLocalSelfTestRecordingDuration != 5*time.Second {
		t.Fatalf("production self-test duration=%s", WindowsLocalSelfTestRecordingDuration)
	}
	root := t.TempDir()
	store := NewCaptureMediaStore(filepath.Join(root, "media"))
	output := &fakeWindowsLocalOutput{}
	capture := &fakeWindowsSelfTestCapture{store: store, events: make(chan string, 8)}
	service := newWindowsSelfTestServiceForTest(t, capture, output, store, 25*time.Millisecond)
	phases := make(chan WindowsLocalSelfTestPhase, 16)
	service.SetEventHandler(func(event WindowsLocalSelfTestEvent) {
		if event.Phase != "" {
			phases <- event.Phase
		}
	})

	started := time.Now()
	if err := service.RecordFiveSeconds(context.Background(), "selected-input"); err != nil {
		t.Fatalf("record: %v", err)
	}
	waitForWindowsSelfTestPhase(t, phases, WindowsLocalSelfTestReviewingDraft)
	request, session := capture.snapshot()
	if request.MediaClass != CaptureSelfTest || !request.ExplicitUserAction || request.DeviceID != "selected-input" {
		t.Fatalf("capture request=%+v", request)
	}
	if session.stopAt.Sub(started) < 20*time.Millisecond {
		t.Fatalf("recording stopped early after %s", session.stopAt.Sub(started))
	}
	if got := []string{<-capture.events, <-capture.events}; !reflect.DeepEqual(got, []string{"start-cue", "stop-cue"}) {
		t.Fatalf("cue sequence=%v", got)
	}
	output.mu.Lock()
	paths := append([]string(nil), output.paths...)
	output.mu.Unlock()
	if len(paths) != 1 || !strings.HasSuffix(paths[0], ".selftest.wav") {
		t.Fatalf("recording playback paths=%v", paths)
	}
	selfTestPath := paths[0]
	service.Close()
	if _, err := os.Stat(selfTestPath); !os.IsNotExist(err) {
		t.Fatalf("self-test survived close: %v", err)
	}
}

func TestWindowsLocalSelfTestBuiltinCueAndCloseInvalidateLateCapture(t *testing.T) {
	root := t.TempDir()
	store := NewCaptureMediaStore(filepath.Join(root, "media"))
	output := &fakeWindowsLocalOutput{}
	capture := &fakeWindowsSelfTestCapture{store: store, events: make(chan string, 8)}
	service := newWindowsSelfTestServiceForTest(t, capture, output, store, time.Hour)
	if err := service.PlayBuiltinCue(context.Background()); err != nil {
		t.Fatalf("cue: %v", err)
	}
	if service.Phase() != WindowsLocalSelfTestIdle {
		t.Fatalf("phase=%s", service.Phase())
	}
	if err := service.RecordFiveSeconds(context.Background(), ""); err != nil {
		t.Fatalf("record: %v", err)
	}
	service.Close()
	if service.Phase() != WindowsLocalSelfTestIdle {
		t.Fatalf("phase after close=%s", service.Phase())
	}
	output.mu.Lock()
	defer output.mu.Unlock()
	if len(output.paths) != 1 || output.cancels == 0 {
		t.Fatalf("cue paths=%v cancels=%d", output.paths, output.cancels)
	}
}

type failingWindowsSelfTestCapture struct{ err error }

func (c failingWindowsSelfTestCapture) Start(context.Context, WindowsCaptureRequest) (WindowsSelfTestCaptureSession, error) {
	return nil, c.err
}

func (failingWindowsSelfTestCapture) Cancel() {}

func TestWindowsLocalSelfTestFailureReviewAndPublicAdapters(t *testing.T) {
	store := NewCaptureMediaStore(t.TempDir())
	output := &fakeWindowsLocalOutput{}
	captureErr := errors.New("permission denied")
	capture := failingWindowsSelfTestCapture{err: captureErr}
	cuePath := filepath.Join("..", "assets", "audio", BuiltinRecordingCueFilename)
	intake := NewWindowsShortAudioIntake(NewWindowsShortAudioInspector(DefaultWindowsShortAudioLimits()), store)
	service, err := NewWindowsLocalSelfTestService(capture, output, store, intake, cuePath)
	if err != nil {
		t.Fatalf("public constructor: %v", err)
	}
	events := make(chan WindowsLocalSelfTestEvent, 16)
	service.SetEventHandler(func(event WindowsLocalSelfTestEvent) { events <- event })
	if err := service.RecordFiveSeconds(context.Background(), ""); !errors.Is(err, captureErr) {
		t.Fatalf("capture error=%v", err)
	}
	if service.Phase() != WindowsLocalSelfTestFailed {
		t.Fatalf("failed phase=%s", service.Phase())
	}
	if err := service.PlayBuiltinCue(context.Background()); err != nil || service.Phase() != WindowsLocalSelfTestIdle {
		t.Fatalf("builtin cue unavailable after capture denial: err=%v phase=%s", err, service.Phase())
	}
	review := service.ReviewFile(brokeredBytes("review.wav", makeWAV16(1, 48_000, make([]int16, 480))))
	if !review.Eligible() {
		t.Fatalf("review=%+v", review)
	}
	rejected, err := service.AcceptFile(brokeredBytes("spoof.wav", []byte("not audio")))
	if !errors.Is(err, ErrWindowsShortAudioUnsupported) || rejected.Rejection != WindowsShortAudioUnsupportedFormat || service.Phase() != WindowsLocalSelfTestIdle {
		t.Fatalf("rejected=%+v err=%v phase=%s", rejected, err, service.Phase())
	}

	adapter := WindowsMicrophoneSelfTestCapture{}
	if _, err := adapter.Start(context.Background(), WindowsCaptureRequest{}); !errors.Is(err, ErrWindowsLocalSelfTestSetup) {
		t.Fatalf("nil adapter err=%v", err)
	}
	adapter.Cancel()
}

func TestWindowsBrokeredShortAudioReviewAndPrivateDraft(t *testing.T) {
	store := NewCaptureMediaStore(t.TempDir())
	inspector := NewWindowsShortAudioInspector(DefaultWindowsShortAudioLimits())
	intake := NewWindowsShortAudioIntake(inspector, store)
	raw := makeWAV16(1, 48_000, make([]int16, 4_800))
	opens := 0
	file := WindowsBrokeredAudioFile{
		DisplayName: `C:\Users\Ivan\family-voice.wav`, SizeBytes: int64(len(raw)),
		Open: func() (io.ReadCloser, error) {
			opens++
			return io.NopCloser(bytes.NewReader(raw)), nil
		},
	}
	review := intake.Review(file)
	if !review.Eligible() || review.Filename != "family-voice.wav" || review.Format != WindowsShortAudioWAV ||
		review.DurationMS != 100 || review.SizeBytes != int64(len(raw)) || !review.ServerValidationRequired {
		t.Fatalf("review=%+v", review)
	}
	if !reflect.DeepEqual(review.Audience, []string{"this_pulsar", "own_barycenter", "current_approach"}) ||
		!reflect.DeepEqual(review.DeliveryModes, []string{"overlay", "interrupt", "after_current"}) ||
		review.RightsReminder == "" || review.PhaseTwoGuidance != "" {
		t.Fatalf("review guidance=%+v", review)
	}
	accepted, draft, err := intake.Accept(file)
	if err != nil || !accepted.Eligible() || draft.Class != CaptureUserRecording || draft.State != CaptureDurableUnsent {
		t.Fatalf("accept review=%+v draft=%+v err=%v", accepted, draft, err)
	}
	if opens != 2 || strings.Contains(draft.Path, "family-voice") {
		t.Fatalf("opens=%d private path=%q", opens, draft.Path)
	}
	got, err := os.ReadFile(draft.Path)
	decoded, decodeErr := parseWAV(got)
	if err != nil || decodeErr != nil || decoded.sampleRate != sampleRate || decoded.channels != channels {
		t.Fatalf("stored canonical draft bytes=%d read=%v decode=%v format=%d/%d", len(got), err, decodeErr, decoded.sampleRate, decoded.channels)
	}
	if err := store.ExplicitlyDelete(draft); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestWindowsLocalSelfTestImportedDraftReplacementDeleteAndClose(t *testing.T) {
	root := t.TempDir()
	store := NewCaptureMediaStore(filepath.Join(root, "media"))
	output := &fakeWindowsLocalOutput{}
	capture := &fakeWindowsSelfTestCapture{store: store, events: make(chan string, 8)}
	service := newWindowsSelfTestServiceForTest(t, capture, output, store, time.Hour)
	raw := makeWAV16(1, 48_000, make([]int16, 4_800))

	if _, err := service.AcceptFile(brokeredBytes("first.wav", raw)); err != nil {
		t.Fatalf("accept first: %v", err)
	}
	first := service.draft.Path
	if _, err := service.AcceptFile(brokeredBytes("second.wav", raw)); err != nil {
		t.Fatalf("accept second: %v", err)
	}
	second := service.draft.Path
	if first == second {
		t.Fatal("replacement reused an owned draft path")
	}
	if _, err := os.Stat(first); !os.IsNotExist(err) {
		t.Fatalf("replaced draft survived: %v", err)
	}
	service.DeleteDraft()
	if _, err := os.Stat(second); !os.IsNotExist(err) {
		t.Fatalf("explicitly deleted draft survived: %v", err)
	}

	if _, err := service.AcceptFile(brokeredBytes("third.wav", raw)); err != nil {
		t.Fatalf("accept third: %v", err)
	}
	third := service.draft.Path
	service.Close()
	if _, err := os.Stat(third); !os.IsNotExist(err) {
		t.Fatalf("closed draft survived: %v", err)
	}
}

func TestWindowsBrokeredShortAudioRejectsSpoofedUnsupportedAndLimits(t *testing.T) {
	limits := WindowsShortAudioLimits{MaximumBytes: 400, MaximumDurationMS: 100, MaximumOverlayDurationMS: 60}
	inspector := NewWindowsShortAudioInspector(limits)
	cases := []struct {
		name string
		file WindowsBrokeredAudioFile
		want WindowsShortAudioRejection
	}{
		{
			name: "extension spoof",
			file: brokeredBytes("voice.wav", []byte("not audio")),
			want: WindowsShortAudioUnsupportedFormat,
		},
		{
			name: "recognized but decoder unavailable",
			file: brokeredBytes("voice.wav", append([]byte("fLaC"), make([]byte, 20)...)),
			want: WindowsShortAudioUnsupportedFormat,
		},
		{
			name: "duration",
			file: brokeredBytes("long.wav", makeWAV16(1, 1_000, make([]int16, 101))),
			want: WindowsShortAudioDurationLimit,
		},
		{
			name: "actual size",
			file: brokeredBytes("large.wav", make([]byte, 401)),
			want: WindowsShortAudioSizeLimit,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			review := inspector.Inspect(tc.file)
			if review.Rejection != tc.want || review.Eligible() || len(review.DeliveryModes) != 0 || review.ServerValidationRequired {
				t.Fatalf("review=%+v", review)
			}
		})
	}
	opened := false
	review := inspector.Inspect(WindowsBrokeredAudioFile{
		DisplayName: "declared-large.wav", SizeBytes: 401,
		Open: func() (io.ReadCloser, error) { opened = true; return nil, errors.New("must not open") },
	})
	if review.Rejection != WindowsShortAudioSizeLimit || opened {
		t.Fatalf("declared limit review=%+v opened=%v", review, opened)
	}
}

func TestWindowsShortAudioContentSignaturesAndAccessFailures(t *testing.T) {
	formats := []struct {
		raw  []byte
		want WindowsShortAudioFormat
	}{
		{append([]byte("fLaC"), make([]byte, 8)...), WindowsShortAudioFLAC},
		{append([]byte("OggS"), make([]byte, 8)...), WindowsShortAudioOGG},
		{append([]byte("ID3"), make([]byte, 9)...), WindowsShortAudioMP3},
		{[]byte{0, 0, 0, 0, 'f', 't', 'y', 'p'}, WindowsShortAudioM4A},
		{[]byte{0xff, 0xf0}, WindowsShortAudioAAC},
		{[]byte{0xff, 0xe2}, WindowsShortAudioMP3},
	}
	for _, tc := range formats {
		if got := detectWindowsShortAudioFormat(tc.raw); got != tc.want {
			t.Fatalf("signature %x format=%s want=%s", tc.raw, got, tc.want)
		}
	}
	inspector := NewWindowsShortAudioInspector(DefaultWindowsShortAudioLimits())
	for _, file := range []WindowsBrokeredAudioFile{
		{DisplayName: "missing.wav"},
		{DisplayName: "denied.wav", Open: func() (io.ReadCloser, error) { return nil, ErrWindowsBrokeredAccess }},
		brokeredBytes("empty.wav", nil),
		brokeredBytes("truncated.wav", []byte("RIFF\x20\x00\x00\x00WAVEfmt ")),
	} {
		review := inspector.Inspect(file)
		if review.Eligible() {
			t.Fatalf("access/corrupt file accepted: %+v", review)
		}
	}
}

func TestWindowsCaptureCanFinalizeSelfTestClass(t *testing.T) {
	stream := newFakeWindowsMicrophoneStream(WindowsCaptureFormat{SampleRate: 48_000, Channels: 1})
	stream.repeat = []float32{0.25, -0.25, 0.25, -0.25}
	backend := &fakeWindowsMicrophoneBackend{
		permission: WindowsCapturePermissionAllowed, stream: stream,
	}
	service := NewWindowsMicrophoneCaptureService(backend, NewCaptureMediaStore(t.TempDir()), nil, nil)
	meter := make(chan struct{}, 1)
	session, err := service.Start(context.Background(), WindowsCaptureRequest{
		ExplicitUserAction: true, MediaClass: CaptureSelfTest,
		Meter: func(float32) {
			select {
			case meter <- struct{}{}:
			default:
			}
		},
	})
	if err != nil {
		t.Fatalf("start: %v", err)
	}
	select {
	case <-meter:
	case <-time.After(time.Second):
		t.Fatal("capture produced no samples")
	}
	session.Stop(WindowsCaptureUserStop)
	outcome := <-session.Done()
	if outcome.Err != nil || outcome.Draft == nil || outcome.Draft.Class != CaptureSelfTest || outcome.Draft.State != CaptureSelfTestLocal {
		t.Fatalf("outcome=%+v", outcome)
	}
	if err := service.store.CloseSelfTest(*outcome.Draft); err != nil {
		t.Fatalf("close self-test: %v", err)
	}
}

func TestWindowsReleaseMSIXStagesReviewedCue(t *testing.T) {
	workflow, err := os.ReadFile(filepath.Join("..", ".github", "workflows", "release.yml"))
	if err != nil {
		t.Fatalf("read release workflow: %v", err)
	}
	source := string(workflow)
	if !strings.Contains(source, "mkdir -p stage/Assets/Audio") ||
		!strings.Contains(source, "assets/audio/pulsar-recording-cue.wav stage/Assets/Audio/pulsar-recording-cue.wav") {
		t.Fatal("production MSIX does not stage the reviewed builtin cue")
	}
}

func newWindowsSelfTestServiceForTest(t *testing.T, capture WindowsSelfTestCapture, output WindowsLocalClipPlaying, store *CaptureMediaStore, duration time.Duration) *WindowsLocalSelfTestService {
	t.Helper()
	cuePath := filepath.Join("..", "assets", "audio", BuiltinRecordingCueFilename)
	service, err := newWindowsLocalSelfTestService(
		capture, output, store,
		NewWindowsShortAudioIntake(NewWindowsShortAudioInspector(DefaultWindowsShortAudioLimits()), store),
		cuePath, duration,
	)
	if err != nil {
		t.Fatalf("new service: %v", err)
	}
	return service
}

func waitForWindowsSelfTestPhase(t *testing.T, phases <-chan WindowsLocalSelfTestPhase, want WindowsLocalSelfTestPhase) {
	t.Helper()
	timer := time.NewTimer(2 * time.Second)
	defer timer.Stop()
	for {
		select {
		case phase := <-phases:
			if phase == want {
				return
			}
		case <-timer.C:
			t.Fatalf("did not observe phase %s", want)
		}
	}
}

func brokeredBytes(name string, raw []byte) WindowsBrokeredAudioFile {
	return WindowsBrokeredAudioFile{
		DisplayName: name, SizeBytes: int64(len(raw)),
		Open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(raw)), nil },
	}
}

func createWindowsTestDraft(store *CaptureMediaStore, class CaptureMediaClass, raw []byte) (CaptureMediaHandle, error) {
	partial, err := store.Begin(class)
	if err != nil {
		return CaptureMediaHandle{}, err
	}
	if err := os.WriteFile(partial.Path, raw, 0o600); err != nil {
		_ = store.Cancel(partial)
		return CaptureMediaHandle{}, err
	}
	finalizing, err := store.Stop(partial)
	if err != nil {
		_ = store.Cancel(partial)
		return CaptureMediaHandle{}, err
	}
	return store.Finalize(finalizing)
}
