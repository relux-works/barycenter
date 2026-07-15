package main

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"io"
	"math"
	"os"
	"path"
	"strings"
	"sync"
	"time"

	protocol "relux.works/duet/pulsar-win/wire"
)

const WindowsLocalSelfTestRecordingDuration = 5 * time.Second

var (
	ErrWindowsLocalOutputBusy       = errors.New("windows_local_output_busy")
	ErrWindowsLocalOutputDecode     = errors.New("windows_local_output_decode_failed")
	ErrWindowsLocalOutputPlayback   = errors.New("windows_local_output_playback_failed")
	ErrWindowsLocalSelfTestSetup    = errors.New("windows_local_self_test_setup")
	ErrWindowsBrokeredAccess        = errors.New("windows_brokered_file_access")
	ErrWindowsShortAudioStorage     = errors.New("windows_short_audio_storage")
	ErrWindowsShortAudioUnsupported = errors.New("windows_short_audio_unsupported")
)

// WindowsLocalClipPlaying is the local-only seam used by cues and self-test
// playback. The production implementation below talks directly to the same
// mixer as network clips; it has no coordinator, upload or receipt dependency.
type WindowsLocalClipPlaying interface {
	Play(context.Context, string, func(error))
	Cancel()
}

type windowsLocalClipMixer interface {
	Prepare(string, string) (*PreparedMediaClip, error)
	Arm(*PreparedMediaClip, MediaClipPlayPlan, func(int64), func(int64), func(error)) error
	Cancel(*PreparedMediaClip, protocol.CancelMediaPayload, func(bool, error))
	Dispose(*PreparedMediaClip)
}

type windowsLocalOutputActive struct {
	clip       *PreparedMediaClip
	generation int64
	finished   chan struct{}
	cancelling bool
}

// WindowsProductionLocalClipOutput creates a synthetic in-process schedule
// and deliberately bypasses MediaClipClient, so no fetch or protocol telemetry
// can be produced by Try locally.
type WindowsProductionLocalClipOutput struct {
	mixer windowsLocalClipMixer
	now   func() time.Time

	mu       sync.Mutex
	next     int64
	reserved int64
	active   *windowsLocalOutputActive
}

func NewWindowsProductionLocalClipOutput(mixer *WindowsOverlayMediaClipMixer) *WindowsProductionLocalClipOutput {
	return newWindowsProductionLocalClipOutput(mixer, time.Now)
}

func newWindowsProductionLocalClipOutput(mixer windowsLocalClipMixer, now func() time.Time) *WindowsProductionLocalClipOutput {
	if now == nil {
		now = time.Now
	}
	return &WindowsProductionLocalClipOutput{mixer: mixer, now: now}
}

func (o *WindowsProductionLocalClipOutput) Play(ctx context.Context, localPath string, done func(error)) {
	if done == nil {
		done = func(error) {}
	}
	if o == nil || o.mixer == nil || localPath == "" {
		done(ErrWindowsLocalOutputPlayback)
		return
	}
	o.mu.Lock()
	if o.active != nil || o.reserved != 0 {
		o.mu.Unlock()
		done(ErrWindowsLocalOutputBusy)
		return
	}
	o.next++
	if o.next <= 0 {
		o.next = 1
	}
	generation := o.next
	o.reserved = generation
	o.mu.Unlock()

	clip, err := o.mixer.Prepare(localPath, "overlay")
	if err != nil {
		o.clearReservation(generation)
		done(ErrWindowsLocalOutputDecode)
		return
	}
	o.mu.Lock()
	if o.reserved != generation || o.active != nil {
		o.mu.Unlock()
		o.mixer.Dispose(clip)
		done(ErrWindowsLocalOutputPlayback)
		return
	}
	o.reserved = 0
	finished := make(chan struct{})
	o.active = &windowsLocalOutputActive{clip: clip, generation: generation, finished: finished}
	o.mu.Unlock()

	start := o.now().Add(30 * time.Millisecond)
	deadline := start.Add(100 * time.Millisecond)
	transmissionID := "local-self-test-" + formatLocalGeneration(generation)
	plan := MediaClipPlayPlan{
		Payload: protocol.PlayMediaAtPayload{
			TransmissionID: transmissionID, Generation: generation,
			TCoordMS: start.UnixMilli(), StartDeadlineCoordMS: deadline.UnixMilli(),
			Delivery: "overlay",
		},
		LocalStartMS: start.UnixMilli(), LocalStartDeadlineMS: deadline.UnixMilli(),
		Control: MixerControlParameters{
			TransmissionID: transmissionID, Generation: generation, Delivery: "overlay",
			DuckDB: 0, AttackMS: 0, ReleaseMS: 0, LimiterCeilingDB: -1,
			ReportStarted: false, ReportEnded: false,
		},
	}
	finish := func(result error) {
		if o.clearActive(clip, generation) {
			o.mixer.Dispose(clip)
			done(result)
		}
	}
	if err := o.mixer.Arm(
		clip,
		plan,
		func(int64) {},
		func(int64) { finish(nil) },
		func(error) { finish(ErrWindowsLocalOutputPlayback) },
	); err != nil {
		finish(ErrWindowsLocalOutputPlayback)
		return
	}
	go func() {
		select {
		case <-ctx.Done():
			o.cancelGeneration(clip, generation, "context_cancelled")
		case <-finished:
		}
	}()
}

func (o *WindowsProductionLocalClipOutput) Cancel() {
	if o == nil {
		return
	}
	o.mu.Lock()
	o.reserved = 0
	active := o.active
	o.mu.Unlock()
	if active != nil {
		o.cancelGeneration(active.clip, active.generation, "local_close")
	}
}

func (o *WindowsProductionLocalClipOutput) cancelGeneration(clip *PreparedMediaClip, generation int64, reason string) {
	o.mu.Lock()
	current := o.active != nil && o.active.clip == clip && o.active.generation == generation && !o.active.cancelling
	if current {
		o.active.cancelling = true
	}
	o.mu.Unlock()
	if !current {
		return
	}
	o.mixer.Cancel(clip, protocol.CancelMediaPayload{
		TransmissionID: "local-self-test-" + formatLocalGeneration(generation),
		Generation:     generation, Reason: reason, Action: "cancel", ResumeMain: false, FadeMS: 40,
	}, func(bool, error) {
		if o.clearActive(clip, generation) {
			o.mixer.Dispose(clip)
		}
	})
}

func (o *WindowsProductionLocalClipOutput) clearReservation(generation int64) {
	o.mu.Lock()
	if o.reserved == generation {
		o.reserved = 0
	}
	o.mu.Unlock()
}

func (o *WindowsProductionLocalClipOutput) clearActive(clip *PreparedMediaClip, generation int64) bool {
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.active == nil || o.active.clip != clip || o.active.generation != generation {
		return false
	}
	close(o.active.finished)
	o.active = nil
	return true
}

func formatLocalGeneration(value int64) string {
	if value == 0 {
		return "0"
	}
	var buffer [20]byte
	index := len(buffer)
	for value > 0 {
		index--
		buffer[index] = byte('0' + value%10)
		value /= 10
	}
	return string(buffer[index:])
}

// WindowsLocalRecordingCuePlayer lets the capture service sequence the exact
// reviewed cue through the production local output before opening capture and
// after closing it.
type WindowsLocalRecordingCuePlayer struct {
	Output  WindowsLocalClipPlaying
	CuePath string
}

func (p WindowsLocalRecordingCuePlayer) PlayRecordingCue(ctx context.Context, phase RecordingCuePhase) error {
	if phase != CuePlayingStart && phase != CuePlayingStop || p.Output == nil || p.CuePath == "" {
		return ErrRecordingCueUnavailable
	}
	result := make(chan error, 1)
	p.Output.Play(ctx, p.CuePath, func(err error) { result <- err })
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		p.Output.Cancel()
		return ctx.Err()
	}
}

type WindowsShortAudioFormat string

const (
	WindowsShortAudioWAV  WindowsShortAudioFormat = "wav"
	WindowsShortAudioMP3  WindowsShortAudioFormat = "mp3"
	WindowsShortAudioM4A  WindowsShortAudioFormat = "m4a"
	WindowsShortAudioAAC  WindowsShortAudioFormat = "aac"
	WindowsShortAudioOGG  WindowsShortAudioFormat = "ogg"
	WindowsShortAudioFLAC WindowsShortAudioFormat = "flac"
)

type WindowsShortAudioRejection string

const (
	WindowsShortAudioUnsupportedFormat WindowsShortAudioRejection = "unsupported_format"
	WindowsShortAudioEmpty             WindowsShortAudioRejection = "empty"
	WindowsShortAudioSizeLimit         WindowsShortAudioRejection = "size_limit"
	WindowsShortAudioDurationLimit     WindowsShortAudioRejection = "duration_limit"
	WindowsShortAudioUnreadable        WindowsShortAudioRejection = "unreadable"
)

type WindowsShortAudioReview struct {
	Filename                 string
	Format                   WindowsShortAudioFormat
	DurationMS               int64
	SizeBytes                int64
	Audience                 []string
	DeliveryModes            []string
	RightsReminder           string
	PhaseTwoGuidance         string
	ServerValidationRequired bool
	Rejection                WindowsShortAudioRejection
}

func (r WindowsShortAudioReview) Eligible() bool { return r.Rejection == "" }

type WindowsShortAudioLimits struct {
	MaximumBytes             int64
	MaximumDurationMS        int64
	MaximumOverlayDurationMS int64
}

func DefaultWindowsShortAudioLimits() WindowsShortAudioLimits {
	return WindowsShortAudioLimits{
		MaximumBytes: 50 << 20, MaximumDurationMS: 180_000, MaximumOverlayDurationMS: 60_000,
	}
}

// WindowsBrokeredAudioFile represents bytes already authorized by a
// FileOpenPicker or drop data-object. No broad filesystem path or capability is
// required; every review/accept pass receives a fresh broker-owned stream.
type WindowsBrokeredAudioFile struct {
	DisplayName string
	SizeBytes   int64
	Open        func() (io.ReadCloser, error)
	Release     func()
}

type WindowsShortAudioInspector struct{ limits WindowsShortAudioLimits }

func NewWindowsShortAudioInspector(limits WindowsShortAudioLimits) *WindowsShortAudioInspector {
	if limits.MaximumBytes <= 0 || limits.MaximumDurationMS <= 0 || limits.MaximumOverlayDurationMS <= 0 {
		limits = DefaultWindowsShortAudioLimits()
	}
	return &WindowsShortAudioInspector{limits: limits}
}

func (i *WindowsShortAudioInspector) Inspect(file WindowsBrokeredAudioFile) WindowsShortAudioReview {
	review, _ := i.inspect(file)
	return review
}

func (i *WindowsShortAudioInspector) inspect(file WindowsBrokeredAudioFile) (WindowsShortAudioReview, []byte) {
	filename := brokeredDisplayName(file.DisplayName)
	if i == nil {
		i = NewWindowsShortAudioInspector(DefaultWindowsShortAudioLimits())
	}
	if file.SizeBytes > i.limits.MaximumBytes {
		return i.review(filename, "", 0, file.SizeBytes, WindowsShortAudioSizeLimit), nil
	}
	if file.Open == nil {
		return i.review(filename, "", 0, max(file.SizeBytes, 0), WindowsShortAudioUnreadable), nil
	}
	stream, err := file.Open()
	if err != nil || stream == nil {
		return i.review(filename, "", 0, max(file.SizeBytes, 0), WindowsShortAudioUnreadable), nil
	}
	raw, readErr := io.ReadAll(io.LimitReader(stream, i.limits.MaximumBytes+1))
	closeErr := stream.Close()
	if readErr != nil || closeErr != nil {
		return i.review(filename, "", 0, int64(len(raw)), WindowsShortAudioUnreadable), nil
	}
	actualBytes := int64(len(raw))
	format := detectWindowsShortAudioFormat(raw)
	if actualBytes == 0 {
		return i.review(filename, format, 0, 0, WindowsShortAudioEmpty), nil
	}
	if actualBytes > i.limits.MaximumBytes {
		return i.review(filename, format, 0, actualBytes, WindowsShortAudioSizeLimit), nil
	}
	if format != WindowsShortAudioWAV {
		return i.review(filename, format, 0, actualBytes, WindowsShortAudioUnsupportedFormat), nil
	}
	if !validateWindowsWAVContainer(raw) {
		return i.review(filename, format, 0, actualBytes, WindowsShortAudioUnreadable), nil
	}
	decoded, err := parseWAV(raw)
	if err != nil || decoded.sampleRate <= 0 || decoded.channels <= 0 || len(decoded.samples) == 0 {
		return i.review(filename, format, 0, actualBytes, WindowsShortAudioUnreadable), nil
	}
	frames := int64(len(decoded.samples) / decoded.channels)
	durationMS := (frames*1000 + int64(decoded.sampleRate) - 1) / int64(decoded.sampleRate)
	if durationMS <= 0 {
		return i.review(filename, format, 0, actualBytes, WindowsShortAudioUnreadable), nil
	}
	if durationMS > i.limits.MaximumDurationMS {
		return i.review(filename, format, durationMS, actualBytes, WindowsShortAudioDurationLimit), nil
	}
	canonical := canonicalWindowsShortWAV(decoded)
	if len(canonical) <= windowsCaptureWAVHeaderBytes || int64(len(canonical)) > maximumCanonicalClipBytes {
		return i.review(filename, format, durationMS, actualBytes, WindowsShortAudioUnreadable), nil
	}
	return i.review(filename, format, durationMS, actualBytes, ""), canonical
}

func (i *WindowsShortAudioInspector) review(filename string, format WindowsShortAudioFormat, durationMS, sizeBytes int64, rejection WindowsShortAudioRejection) WindowsShortAudioReview {
	modes := []string{"interrupt", "after_current"}
	if durationMS > 0 && durationMS <= i.limits.MaximumOverlayDurationMS {
		modes = append([]string{"overlay"}, modes...)
	}
	if rejection != "" {
		modes = nil
	}
	guidance := ""
	switch rejection {
	case WindowsShortAudioSizeLimit, WindowsShortAudioDurationLimit:
		guidance = "Long audio remains unavailable until Phase 2 streamed tracks."
	case WindowsShortAudioUnsupportedFormat:
		guidance = "This Windows build currently prepares content-validated PCM16 or float32 WAV files only."
	case WindowsShortAudioEmpty, WindowsShortAudioUnreadable:
		guidance = "Choose a non-empty readable audio file; the server will validate accepted media again."
	}
	return WindowsShortAudioReview{
		Filename: filename, Format: format, DurationMS: durationMS, SizeBytes: sizeBytes,
		Audience:                 []string{"this_pulsar", "own_barycenter", "current_approach"},
		DeliveryModes:            modes,
		RightsReminder:           "Upload only audio you recorded or have the right to share.",
		PhaseTwoGuidance:         guidance,
		ServerValidationRequired: rejection == "", Rejection: rejection,
	}
}

func detectWindowsShortAudioFormat(raw []byte) WindowsShortAudioFormat {
	if len(raw) >= 12 && string(raw[:4]) == "RIFF" && string(raw[8:12]) == "WAVE" {
		return WindowsShortAudioWAV
	}
	if len(raw) >= 4 && string(raw[:4]) == "fLaC" {
		return WindowsShortAudioFLAC
	}
	if len(raw) >= 4 && string(raw[:4]) == "OggS" {
		return WindowsShortAudioOGG
	}
	if len(raw) >= 3 && string(raw[:3]) == "ID3" {
		return WindowsShortAudioMP3
	}
	if len(raw) >= 8 && string(raw[4:8]) == "ftyp" {
		return WindowsShortAudioM4A
	}
	if len(raw) >= 2 && raw[0] == 0xff {
		if raw[1]&0xf6 == 0xf0 {
			return WindowsShortAudioAAC
		}
		if raw[1]&0xe0 == 0xe0 && raw[1]&0x06 != 0 {
			return WindowsShortAudioMP3
		}
	}
	return ""
}

func validateWindowsWAVContainer(raw []byte) bool {
	if len(raw) < 12 || string(raw[:4]) != "RIFF" || string(raw[8:12]) != "WAVE" ||
		int64(binary.LittleEndian.Uint32(raw[4:8])) != int64(len(raw)-8) {
		return false
	}
	offset := 12
	haveFormat, haveData := false, false
	for offset < len(raw) {
		if offset+8 > len(raw) {
			return false
		}
		chunkID := string(raw[offset : offset+4])
		size := int64(binary.LittleEndian.Uint32(raw[offset+4 : offset+8]))
		body := int64(offset + 8)
		end := body + size
		if size < 0 || end < body || end > int64(len(raw)) {
			return false
		}
		if chunkID == "fmt " {
			haveFormat = size >= 16
		}
		if chunkID == "data" {
			haveData = size > 0
		}
		offset = int(end)
		if size%2 == 1 {
			offset++
			if offset > len(raw) {
				return false
			}
		}
	}
	return offset == len(raw) && haveFormat && haveData
}

func canonicalWindowsShortWAV(decoded wavData) []byte {
	engineSamples := toEngineFormat(decoded)
	if len(engineSamples) < channels {
		return nil
	}
	dataBytes := len(engineSamples) * 2
	if dataBytes < 0 || int64(dataBytes)+windowsCaptureWAVHeaderBytes > maximumCanonicalClipBytes {
		return nil
	}
	raw := make([]byte, windowsCaptureWAVHeaderBytes+dataBytes)
	copy(raw[0:4], "RIFF")
	binary.LittleEndian.PutUint32(raw[4:8], uint32(len(raw)-8))
	copy(raw[8:12], "WAVE")
	copy(raw[12:16], "fmt ")
	binary.LittleEndian.PutUint32(raw[16:20], 16)
	binary.LittleEndian.PutUint16(raw[20:22], 1)
	binary.LittleEndian.PutUint16(raw[22:24], uint16(channels))
	binary.LittleEndian.PutUint32(raw[24:28], uint32(sampleRate))
	binary.LittleEndian.PutUint32(raw[28:32], uint32(sampleRate*channels*2))
	binary.LittleEndian.PutUint16(raw[32:34], uint16(channels*2))
	binary.LittleEndian.PutUint16(raw[34:36], 16)
	copy(raw[36:40], "data")
	binary.LittleEndian.PutUint32(raw[40:44], uint32(dataBytes))
	for index, sample := range engineSamples {
		clamped := min(float64(1), max(float64(-1), float64(sample)))
		value := int16(math.Round(clamped * 32767))
		binary.LittleEndian.PutUint16(raw[44+index*2:], uint16(value))
	}
	return raw
}

func brokeredDisplayName(value string) string {
	value = strings.ReplaceAll(value, "\\", "/")
	value = path.Base(value)
	if value == "." || value == "/" || value == "" {
		return "audio"
	}
	return value
}

type WindowsShortAudioIntake struct {
	inspector *WindowsShortAudioInspector
	store     *CaptureMediaStore
}

func NewWindowsShortAudioIntake(inspector *WindowsShortAudioInspector, store *CaptureMediaStore) *WindowsShortAudioIntake {
	return &WindowsShortAudioIntake{inspector: inspector, store: store}
}

func (i *WindowsShortAudioIntake) Review(file WindowsBrokeredAudioFile) WindowsShortAudioReview {
	if i == nil || i.inspector == nil {
		return NewWindowsShortAudioInspector(DefaultWindowsShortAudioLimits()).Inspect(file)
	}
	return i.inspector.Inspect(file)
}

func (i *WindowsShortAudioIntake) Accept(file WindowsBrokeredAudioFile) (WindowsShortAudioReview, CaptureMediaHandle, error) {
	if i == nil || i.inspector == nil || i.store == nil {
		if file.Release != nil {
			file.Release()
		}
		return WindowsShortAudioReview{}, CaptureMediaHandle{}, ErrWindowsShortAudioStorage
	}
	if file.Release != nil {
		defer file.Release()
	}
	review, raw := i.inspector.inspect(file)
	if !review.Eligible() {
		return review, CaptureMediaHandle{}, ErrWindowsShortAudioUnsupported
	}
	draft, err := i.store.ImportUserDraft(bytes.NewReader(raw))
	if err != nil {
		return review, CaptureMediaHandle{}, ErrWindowsShortAudioStorage
	}
	return review, draft, nil
}

type WindowsSelfTestCaptureSession interface {
	Stop(WindowsCaptureStopReason)
	Done() <-chan WindowsCaptureOutcome
}

type WindowsSelfTestCapture interface {
	Start(context.Context, WindowsCaptureRequest) (WindowsSelfTestCaptureSession, error)
	Cancel()
}

type WindowsMicrophoneSelfTestCapture struct {
	Service *WindowsMicrophoneCaptureService
}

func (c WindowsMicrophoneSelfTestCapture) Start(ctx context.Context, request WindowsCaptureRequest) (WindowsSelfTestCaptureSession, error) {
	if c.Service == nil {
		return nil, ErrWindowsLocalSelfTestSetup
	}
	return c.Service.Start(ctx, request)
}

func (c WindowsMicrophoneSelfTestCapture) Cancel() {
	if c.Service != nil {
		c.Service.Cancel()
	}
}

type WindowsLocalSelfTestPhase string

const (
	WindowsLocalSelfTestIdle                 WindowsLocalSelfTestPhase = "idle"
	WindowsLocalSelfTestPlayingBuiltinCue    WindowsLocalSelfTestPhase = "playing_builtin_cue"
	WindowsLocalSelfTestRequestingPermission WindowsLocalSelfTestPhase = "requesting_permission"
	WindowsLocalSelfTestRecording            WindowsLocalSelfTestPhase = "recording"
	WindowsLocalSelfTestPlayingStopCue       WindowsLocalSelfTestPhase = "playing_stop_cue"
	WindowsLocalSelfTestPlayingRecording     WindowsLocalSelfTestPhase = "playing_recording"
	WindowsLocalSelfTestProcessingFile       WindowsLocalSelfTestPhase = "processing_file"
	WindowsLocalSelfTestReviewingDraft       WindowsLocalSelfTestPhase = "reviewing_draft"
	WindowsLocalSelfTestFailed               WindowsLocalSelfTestPhase = "failed"
)

type WindowsLocalSelfTestEvent struct {
	Phase   WindowsLocalSelfTestPhase
	Meter   float32
	Review  *WindowsShortAudioReview
	Draft   *CaptureMediaHandle
	Failure string
}

type WindowsLocalSelfTestService struct {
	capture  WindowsSelfTestCapture
	output   WindowsLocalClipPlaying
	store    *CaptureMediaStore
	intake   *WindowsShortAudioIntake
	cuePath  string
	duration time.Duration

	mu         sync.Mutex
	phase      WindowsLocalSelfTestPhase
	generation uint64
	session    WindowsSelfTestCaptureSession
	timer      *time.Timer
	draft      *CaptureMediaHandle
	onEvent    func(WindowsLocalSelfTestEvent)
	pending    sync.WaitGroup
}

func NewWindowsLocalSelfTestService(capture WindowsSelfTestCapture, output WindowsLocalClipPlaying, store *CaptureMediaStore, intake *WindowsShortAudioIntake, cuePath string) (*WindowsLocalSelfTestService, error) {
	return newWindowsLocalSelfTestService(capture, output, store, intake, cuePath, WindowsLocalSelfTestRecordingDuration)
}

func newWindowsLocalSelfTestService(capture WindowsSelfTestCapture, output WindowsLocalClipPlaying, store *CaptureMediaStore, intake *WindowsShortAudioIntake, cuePath string, duration time.Duration) (*WindowsLocalSelfTestService, error) {
	data, err := os.ReadFile(cuePath)
	if capture == nil || output == nil || store == nil || intake == nil || duration <= 0 || err != nil || !ValidateBuiltinRecordingCue(data) {
		return nil, ErrWindowsLocalSelfTestSetup
	}
	return &WindowsLocalSelfTestService{
		capture: capture, output: output, store: store, intake: intake,
		cuePath: cuePath, duration: duration, phase: WindowsLocalSelfTestIdle,
	}, nil
}

func (s *WindowsLocalSelfTestService) SetEventHandler(handler func(WindowsLocalSelfTestEvent)) {
	s.mu.Lock()
	s.onEvent = handler
	s.mu.Unlock()
}

func (s *WindowsLocalSelfTestService) Phase() WindowsLocalSelfTestPhase {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.phase
}

func (s *WindowsLocalSelfTestService) PlayBuiltinCue(ctx context.Context) error {
	s.mu.Lock()
	if !windowsLocalSelfTestCanStart(s.phase) {
		s.mu.Unlock()
		return ErrWindowsLocalOutputBusy
	}
	s.generation++
	generation := s.generation
	s.phase = WindowsLocalSelfTestPlayingBuiltinCue
	s.mu.Unlock()
	s.emit(WindowsLocalSelfTestEvent{Phase: WindowsLocalSelfTestPlayingBuiltinCue})
	s.output.Play(ctx, s.cuePath, func(err error) {
		if err != nil {
			s.failGeneration(generation, "cue_playback_failed")
			return
		}
		s.mu.Lock()
		if s.generation != generation {
			s.mu.Unlock()
			return
		}
		if s.draft == nil {
			s.phase = WindowsLocalSelfTestIdle
		} else {
			s.phase = WindowsLocalSelfTestReviewingDraft
		}
		phase := s.phase
		s.mu.Unlock()
		s.emit(WindowsLocalSelfTestEvent{Phase: phase})
	})
	return nil
}

func (s *WindowsLocalSelfTestService) RecordFiveSeconds(ctx context.Context, deviceID string) error {
	s.mu.Lock()
	if !windowsLocalSelfTestCanStart(s.phase) {
		s.mu.Unlock()
		return ErrWindowsCaptureBusy
	}
	s.generation++
	generation := s.generation
	s.phase = WindowsLocalSelfTestRequestingPermission
	s.mu.Unlock()
	s.emit(WindowsLocalSelfTestEvent{Phase: WindowsLocalSelfTestRequestingPermission})

	session, err := s.capture.Start(ctx, WindowsCaptureRequest{
		ExplicitUserAction: true, DeviceID: deviceID, MediaClass: CaptureSelfTest,
		Meter: func(value float32) {
			s.mu.Lock()
			current := s.generation == generation && s.phase == WindowsLocalSelfTestRecording
			s.mu.Unlock()
			if current {
				s.emit(WindowsLocalSelfTestEvent{Meter: value})
			}
		},
	})
	if err != nil {
		s.failGeneration(generation, "capture_start_failed")
		return err
	}

	s.mu.Lock()
	if s.generation != generation {
		s.mu.Unlock()
		session.Stop(WindowsCaptureCancel)
		return context.Canceled
	}
	s.session = session
	s.phase = WindowsLocalSelfTestRecording
	s.timer = time.AfterFunc(s.duration, func() { s.stopExactRecording(generation, session) })
	s.mu.Unlock()
	s.emit(WindowsLocalSelfTestEvent{Phase: WindowsLocalSelfTestRecording})
	s.pending.Add(1)
	go func() {
		defer s.pending.Done()
		s.awaitCapture(generation, session)
	}()
	return nil
}

func (s *WindowsLocalSelfTestService) Wait(ctx context.Context) error {
	if s == nil {
		return nil
	}
	done := make(chan struct{})
	go func() { s.pending.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *WindowsLocalSelfTestService) stopExactRecording(generation uint64, session WindowsSelfTestCaptureSession) {
	s.mu.Lock()
	if s.generation != generation || s.session != session || s.phase != WindowsLocalSelfTestRecording {
		s.mu.Unlock()
		return
	}
	s.phase = WindowsLocalSelfTestPlayingStopCue
	s.mu.Unlock()
	s.emit(WindowsLocalSelfTestEvent{Phase: WindowsLocalSelfTestPlayingStopCue})
	session.Stop(WindowsCaptureUserStop)
}

func (s *WindowsLocalSelfTestService) awaitCapture(generation uint64, session WindowsSelfTestCaptureSession) {
	outcome, ok := <-session.Done()
	if !ok {
		s.failGeneration(generation, "capture_missing_outcome")
		return
	}
	s.mu.Lock()
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if s.generation != generation || s.session != session {
		s.mu.Unlock()
		if outcome.Draft != nil {
			_ = s.store.ExplicitlyDelete(*outcome.Draft)
		}
		return
	}
	s.session = nil
	if outcome.Err != nil || outcome.Draft == nil || outcome.Draft.Class != CaptureSelfTest || outcome.Draft.State != CaptureSelfTestLocal {
		s.mu.Unlock()
		if outcome.Draft != nil {
			_ = s.store.ExplicitlyDelete(*outcome.Draft)
		}
		s.failGeneration(generation, "capture_failed")
		return
	}
	previous := s.draft
	draft := *outcome.Draft
	s.draft = &draft
	s.phase = WindowsLocalSelfTestPlayingRecording
	s.mu.Unlock()
	if previous != nil {
		_ = s.store.ExplicitlyDelete(*previous)
	}
	s.emit(WindowsLocalSelfTestEvent{Draft: &draft})
	s.emit(WindowsLocalSelfTestEvent{Phase: WindowsLocalSelfTestPlayingRecording})
	s.output.Play(context.Background(), draft.Path, func(err error) {
		if err != nil {
			s.failGeneration(generation, "recording_playback_failed")
			return
		}
		s.mu.Lock()
		if s.generation != generation {
			s.mu.Unlock()
			return
		}
		s.phase = WindowsLocalSelfTestReviewingDraft
		s.mu.Unlock()
		s.emit(WindowsLocalSelfTestEvent{Phase: WindowsLocalSelfTestReviewingDraft})
	})
}

func (s *WindowsLocalSelfTestService) ReviewFile(file WindowsBrokeredAudioFile) WindowsShortAudioReview {
	review := s.intake.Review(file)
	s.emit(WindowsLocalSelfTestEvent{Review: &review})
	return review
}

func (s *WindowsLocalSelfTestService) AcceptFile(file WindowsBrokeredAudioFile) (WindowsShortAudioReview, error) {
	s.mu.Lock()
	allowed := windowsLocalSelfTestCanStart(s.phase)
	if !allowed {
		s.mu.Unlock()
		return WindowsShortAudioReview{}, ErrWindowsCaptureBusy
	}
	previousPhase := s.phase
	s.generation++
	generation := s.generation
	s.phase = WindowsLocalSelfTestProcessingFile
	s.mu.Unlock()
	s.emit(WindowsLocalSelfTestEvent{Phase: WindowsLocalSelfTestProcessingFile})
	review, draft, err := s.intake.Accept(file)
	s.emit(WindowsLocalSelfTestEvent{Review: &review})
	if err != nil {
		s.mu.Lock()
		if s.generation == generation {
			s.phase = previousPhase
		}
		phase := s.phase
		s.mu.Unlock()
		s.emit(WindowsLocalSelfTestEvent{Phase: phase})
		return review, err
	}
	s.mu.Lock()
	if s.generation != generation {
		s.mu.Unlock()
		_ = s.store.ExplicitlyDelete(draft)
		return review, context.Canceled
	}
	previous := s.draft
	s.draft = &draft
	s.phase = WindowsLocalSelfTestReviewingDraft
	s.mu.Unlock()
	if previous != nil {
		_ = s.store.ExplicitlyDelete(*previous)
	}
	s.emit(WindowsLocalSelfTestEvent{Draft: &draft})
	s.emit(WindowsLocalSelfTestEvent{Phase: WindowsLocalSelfTestReviewingDraft})
	return review, nil
}

func (s *WindowsLocalSelfTestService) DeleteDraft() {
	s.mu.Lock()
	if !windowsLocalSelfTestCanStart(s.phase) {
		s.mu.Unlock()
		return
	}
	draft := s.draft
	s.draft = nil
	s.generation++
	s.phase = WindowsLocalSelfTestIdle
	s.mu.Unlock()
	s.output.Cancel()
	if draft != nil {
		_ = s.store.ExplicitlyDelete(*draft)
	}
	s.emit(WindowsLocalSelfTestEvent{Phase: WindowsLocalSelfTestIdle})
}

func (s *WindowsLocalSelfTestService) Close() {
	s.mu.Lock()
	s.generation++
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	s.session = nil
	draft := s.draft
	s.draft = nil
	s.phase = WindowsLocalSelfTestIdle
	s.mu.Unlock()
	s.capture.Cancel()
	s.output.Cancel()
	if draft != nil {
		_ = s.store.ExplicitlyDelete(*draft)
	}
	s.emit(WindowsLocalSelfTestEvent{Phase: WindowsLocalSelfTestIdle})
}

func (s *WindowsLocalSelfTestService) failGeneration(generation uint64, code string) {
	s.mu.Lock()
	if s.generation != generation {
		s.mu.Unlock()
		return
	}
	s.phase = WindowsLocalSelfTestFailed
	s.mu.Unlock()
	s.emit(WindowsLocalSelfTestEvent{Phase: WindowsLocalSelfTestFailed})
	s.emit(WindowsLocalSelfTestEvent{Failure: code})
}

func (s *WindowsLocalSelfTestService) emit(event WindowsLocalSelfTestEvent) {
	s.mu.Lock()
	handler := s.onEvent
	s.mu.Unlock()
	if handler != nil {
		handler(event)
	}
}

func windowsLocalSelfTestCanStart(phase WindowsLocalSelfTestPhase) bool {
	return phase == WindowsLocalSelfTestIdle || phase == WindowsLocalSelfTestReviewingDraft || phase == WindowsLocalSelfTestFailed
}
