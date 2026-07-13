package main

import (
	"encoding"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

func testRecoveryMaterial(t testing.TB) *RecoveryMaterial {
	t.Helper()
	material, err := newRecoveryMaterial(9, "rec_0123456789abcdef0123456789abcdef", "ABCDEFGHJKMNPQRSTVWXYZ23456")
	if err != nil {
		t.Fatal(err)
	}
	return material
}

type fakeExportFile struct {
	data       []byte
	writeChunk int
	failWrite  bool
	failSync   bool
	failClose  bool
	closed     bool
}

func (f *fakeExportFile) Write(value []byte) (int, error) {
	if f.failWrite {
		return 0, errors.New("canary write")
	}
	n := len(value)
	if f.writeChunk > 0 && n > f.writeChunk {
		n = f.writeChunk
	}
	f.data = append(f.data, value[:n]...)
	return n, nil
}

func (f *fakeExportFile) Sync() error {
	if f.failSync {
		return errors.New("canary sync")
	}
	return nil
}

func (f *fakeExportFile) Close() error {
	f.closed = true
	if f.failClose {
		return errors.New("canary close")
	}
	return nil
}

type fakeExportFS struct {
	file       *fakeExportFile
	created    string
	deleted    []string
	failCreate bool
}

func (f *fakeExportFS) CreateExclusive(path string) (directExportFile, error) {
	if f.failCreate {
		return nil, errors.New("canary create")
	}
	f.created = path
	return f.file, nil
}

func (f *fakeExportFS) Delete(path string) error {
	f.deleted = append(f.deleted, path)
	return nil
}

func TestRecoveryExportIsExplicitExactAndNotBackupAcknowledgement(t *testing.T) {
	material := testRecoveryMaterial(t)
	file := &fakeExportFile{writeChunk: 3}
	fs := &fakeExportFS{file: file}
	exporter := newRecoveryExporterForTesting(fs)
	if fs.created != "" {
		t.Fatal("export occurred without explicit save")
	}
	if err := exporter.SaveSelectedDestination("selected-by-user.json", material); err != nil {
		t.Fatal(err)
	}
	if fs.created != "selected-by-user.json" || len(fs.deleted) != 0 || !file.closed {
		t.Fatalf("direct export lifecycle created=%q deleted=%v closed=%t", fs.created, fs.deleted, file.closed)
	}
	var object map[string]any
	if err := json.Unmarshal(file.data, &object); err != nil {
		t.Fatal(err)
	}
	if len(object) != 3 || object["actor_id"] != float64(9) || object["recovery_id"] == nil || object["recovery_secret"] == nil {
		t.Fatalf("export shape %#v", object)
	}
	_, _, secret, ok := material.RevealForDisplay()
	if !ok || secret == "" {
		t.Fatal("save silently discarded or acknowledged material")
	}
}

func TestRecoveryExportFailureRemovesOnlySelectedPartial(t *testing.T) {
	for _, configure := range []func(*fakeExportFile){
		func(file *fakeExportFile) { file.failWrite = true },
		func(file *fakeExportFile) { file.failSync = true },
		func(file *fakeExportFile) { file.failClose = true },
	} {
		file := &fakeExportFile{}
		configure(file)
		fs := &fakeExportFS{file: file}
		exporter := newRecoveryExporterForTesting(fs)
		if err := exporter.SaveSelectedDestination("chosen.json", testRecoveryMaterial(t)); err == nil {
			t.Fatal("failure claimed success")
		}
		if len(fs.deleted) != 1 || fs.deleted[0] != "chosen.json" {
			t.Fatalf("partial selected file not cleaned: %v", fs.deleted)
		}
	}
}

func TestRecoveryDismissReturnsExactLossWarning(t *testing.T) {
	material := testRecoveryMaterial(t)
	notice := material.DismissWithoutBackup()
	if notice.English != RecoveryLossWarningEN || notice.Russian != RecoveryLossWarningRU {
		t.Fatalf("warning %#v", notice)
	}
	if _, _, _, ok := material.RevealForDisplay(); ok {
		t.Fatal("dismissed one-time material remained available")
	}
}

func TestOneTimeMaterialHasNoImplicitMarshalOrFormattingDisclosure(t *testing.T) {
	material := testRecoveryMaterial(t)
	codeCanary := "ABCDEFGHJKMNPQRSTVWXYZ23456"
	code := newOneTimeCode(codeCanary)
	for name, value := range map[string]any{"recovery": material, "code": code} {
		if _, ok := value.(json.Marshaler); ok {
			t.Fatalf("%s unexpectedly implements json.Marshaler", name)
		}
		if _, ok := value.(encoding.TextMarshaler); ok {
			t.Fatalf("%s unexpectedly implements encoding.TextMarshaler", name)
		}
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		for _, rendered := range []string{string(raw), fmt.Sprint(value), fmt.Sprintf("%+v", value), fmt.Sprintf("%#v", value)} {
			if strings.Contains(rendered, codeCanary) || strings.Contains(rendered, "rec_0123456789abcdef0123456789abcdef") {
				t.Fatalf("%s leaked through implicit representation: %q", name, rendered)
			}
		}
	}
}

type directTestDispatcher struct{ calls int }

func (d *directTestDispatcher) Invoke(callback func() error) error {
	d.calls++
	return callback()
}

type fakeClipboardBackend struct {
	mu                sync.Mutex
	sequence          uint32
	payload           string
	markers           map[string]uint32
	failBeforeChange  bool
	failCopy          bool
	failAfterExposure bool
	ambiguousExposure bool
	failClear         bool
	clearFailures     int
	clearSentinels    []string
	clearEntered      chan struct{}
	clearProceed      chan struct{}
}

func (b *fakeClipboardBackend) Publish(_ uintptr, payload string) (clipboardPublication, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.failBeforeChange {
		return clipboardPublication{}, errors.New("canary copy before change")
	}
	b.sequence++
	b.markers = map[string]uint32{}
	for _, name := range windowsClipboardExclusionFormats {
		b.markers[name] = windowsClipboardExclusionDWORD
	}
	if b.failCopy {
		b.payload = ""
		return clipboardPublication{Sequence: b.sequence, Changed: true}, errors.New("canary copy")
	}
	b.payload = payload
	if b.failAfterExposure {
		sequence := b.sequence
		if b.ambiguousExposure {
			sequence = 0
		}
		return clipboardPublication{Sequence: sequence, Changed: true, Exposed: true}, errors.New("canary post-exposure sequence")
	}
	return clipboardPublication{Sequence: b.sequence, Changed: true, Exposed: true}, nil
}

func (b *fakeClipboardBackend) ClearIfUnchanged(_ uintptr, sequence uint32, payload string) (bool, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.clearEntered != nil {
		close(b.clearEntered)
		<-b.clearProceed
		b.clearEntered = nil
	}
	if len(b.clearSentinels) != 0 {
		sentinel := b.clearSentinels[0]
		b.clearSentinels = b.clearSentinels[1:]
		return false, errors.New("canary " + sentinel)
	}
	if b.failClear || b.clearFailures > 0 {
		if b.clearFailures > 0 {
			b.clearFailures--
		}
		return false, errors.New("canary clear")
	}
	if (sequence != 0 && b.sequence != sequence) || b.payload != payload {
		return false, nil
	}
	b.sequence++
	b.payload = ""
	return true, nil
}

func (b *fakeClipboardBackend) externalWrite(value string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sequence++
	b.payload = value
}

type fakeClipboardTimer struct {
	callback func()
	stopped  bool
}

func (t *fakeClipboardTimer) Stop() bool {
	was := !t.stopped
	t.stopped = true
	return was
}

type fakeClipboardScheduledTimer struct {
	delay time.Duration
	timer *fakeClipboardTimer
}

type fakeClipboardScheduler struct{ timers []fakeClipboardScheduledTimer }

func (s *fakeClipboardScheduler) AfterFunc(delay time.Duration, callback func()) clipboardTimer {
	timer := &fakeClipboardTimer{callback: callback}
	s.timers = append(s.timers, fakeClipboardScheduledTimer{delay: delay, timer: timer})
	return timer
}

func (s *fakeClipboardScheduler) fire(index int) {
	timer := s.timers[index].timer
	if !timer.stopped {
		timer.callback()
	}
}

func TestRecoveryClipboardMarkersTTLAndNewerContentSurvival(t *testing.T) {
	backend := &fakeClipboardBackend{}
	scheduler := &fakeClipboardScheduler{}
	dispatcher := &directTestDispatcher{}
	clipboard, err := newRecoveryClipboard(123, dispatcher, backend, scheduler)
	if err != nil {
		t.Fatal(err)
	}
	lease1, err := clipboard.Copy(testRecoveryMaterial(t), 30*time.Second)
	if err != nil || lease1 == 0 {
		t.Fatalf("copy lease=%d err=%v", lease1, err)
	}
	if len(backend.markers) != 3 {
		t.Fatalf("exclusion markers %#v", backend.markers)
	}
	for _, name := range windowsClipboardExclusionFormats {
		if value, ok := backend.markers[name]; !ok || value != 0 {
			t.Fatalf("marker %s=%d present=%t", name, value, ok)
		}
	}
	lease2, err := clipboard.Copy(testRecoveryMaterial(t), maximumRecoveryClipboardTTL)
	if err != nil || lease2 == lease1 {
		t.Fatalf("second lease=%d err=%v", lease2, err)
	}
	// Old timer closures carry only the lease id; they cannot clear a new lease.
	scheduler.timers[0].timer.callback()
	if backend.payload == "" {
		t.Fatal("old timer cleared new lease")
	}
	backend.externalWrite("newer user content")
	scheduler.fire(1)
	if backend.payload != "newer user content" {
		t.Fatal("expiry erased newer clipboard data")
	}
	if err := clipboard.Clear(); err != nil {
		t.Fatal(err)
	}
}

func TestRecoveryClipboardExplicitClearIsAtomicAndIdempotent(t *testing.T) {
	backend := &fakeClipboardBackend{}
	scheduler := &fakeClipboardScheduler{}
	clipboard, _ := newRecoveryClipboard(123, &directTestDispatcher{}, backend, scheduler)
	if _, err := clipboard.Copy(testRecoveryMaterial(t), time.Second); err != nil {
		t.Fatal(err)
	}
	if err := clipboard.Clear(); err != nil {
		t.Fatal(err)
	}
	if backend.payload != "" {
		t.Fatal("owned clipboard was not cleared")
	}
	if err := clipboard.Clear(); err != nil {
		t.Fatal("idempotent clear failed")
	}
}

func TestRecoveryClipboardCopyContendersAndTTLBound(t *testing.T) {
	backend := &fakeClipboardBackend{}
	scheduler := &fakeClipboardScheduler{}
	clipboard, _ := newRecoveryClipboard(123, &directTestDispatcher{}, backend, scheduler)
	if _, err := clipboard.Copy(testRecoveryMaterial(t), maximumRecoveryClipboardTTL+time.Second); err == nil {
		t.Fatal("TTL over 300 seconds accepted")
	}
	var wg sync.WaitGroup
	materials := []*RecoveryMaterial{testRecoveryMaterial(t), testRecoveryMaterial(t)}
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			_, _ = clipboard.Copy(materials[index], time.Second)
		}(i)
	}
	wg.Wait()
	if clipboard.current == nil || clipboard.current.id != 2 || backend.sequence != 2 {
		t.Fatalf("contenders current=%#v sequence=%d", clipboard.current, backend.sequence)
	}
}

func TestRecoveryClipboardFailedReplacementPreservesOnlyOwnedLease(t *testing.T) {
	backend := &fakeClipboardBackend{}
	scheduler := &fakeClipboardScheduler{}
	clipboard, _ := newRecoveryClipboard(123, &directTestDispatcher{}, backend, scheduler)
	lease, err := clipboard.Copy(testRecoveryMaterial(t), time.Second)
	if err != nil {
		t.Fatal(err)
	}
	originalPayload := backend.payload
	backend.failBeforeChange = true
	if _, err := clipboard.Copy(testRecoveryMaterial(t), time.Second); !errors.Is(err, errClipboardCopyFailed) {
		t.Fatalf("pre-change failure=%v", err)
	}
	if clipboard.current == nil || clipboard.current.id != lease || backend.payload != originalPayload || scheduler.timers[0].timer.stopped {
		t.Fatal("pre-change failure abandoned the still-owned clipboard lease")
	}

	backend.failBeforeChange = false
	backend.failCopy = true
	if _, err := clipboard.Copy(testRecoveryMaterial(t), time.Second); !errors.Is(err, errClipboardCopyFailed) {
		t.Fatalf("marker failure=%v", err)
	}
	if clipboard.current != nil || backend.payload != "" || !scheduler.timers[0].timer.stopped {
		t.Fatal("changed clipboard retained an obsolete secret lease")
	}
}

func TestRecoveryClipboardClearRetriesUntilSafeResolution(t *testing.T) {
	backend := &fakeClipboardBackend{clearFailures: 2}
	scheduler := &fakeClipboardScheduler{}
	clipboard, _ := newRecoveryClipboard(123, &directTestDispatcher{}, backend, scheduler)
	if _, err := clipboard.Copy(testRecoveryMaterial(t), time.Second); err != nil {
		t.Fatal(err)
	}
	scheduler.fire(0)
	if clipboard.current == nil || len(scheduler.timers) != 2 || scheduler.timers[1].delay != recoveryClipboardRetryDelay {
		t.Fatal("first transient clear failure lost retry obligation")
	}
	scheduler.fire(1)
	if clipboard.current == nil || len(scheduler.timers) != 3 {
		t.Fatal("repeated clear failure lost retry obligation")
	}
	scheduler.fire(2)
	if clipboard.current != nil || backend.payload != "" {
		t.Fatal("successful retry did not clear owned payload")
	}

	backend = &fakeClipboardBackend{clearFailures: 1}
	scheduler = &fakeClipboardScheduler{}
	clipboard, _ = newRecoveryClipboard(123, &directTestDispatcher{}, backend, scheduler)
	if _, err := clipboard.Copy(testRecoveryMaterial(t), time.Second); err != nil {
		t.Fatal(err)
	}
	scheduler.fire(0)
	backend.externalWrite("newer user content")
	scheduler.fire(1)
	if clipboard.current != nil || backend.payload != "newer user content" {
		t.Fatal("retry erased externally replaced clipboard")
	}

	backend = &fakeClipboardBackend{clearFailures: 1}
	scheduler = &fakeClipboardScheduler{}
	clipboard, _ = newRecoveryClipboard(123, &directTestDispatcher{}, backend, scheduler)
	if _, err := clipboard.Copy(testRecoveryMaterial(t), time.Second); err != nil {
		t.Fatal(err)
	}
	scheduler.fire(0)
	if _, err := clipboard.Copy(testRecoveryMaterial(t), time.Second); err != nil {
		t.Fatal(err)
	}
	scheduler.timers[1].timer.callback()
	if clipboard.current == nil || backend.payload == "" {
		t.Fatal("old retry timer cleared a newer lease")
	}

	backend = &fakeClipboardBackend{clearFailures: 1}
	scheduler = &fakeClipboardScheduler{}
	clipboard, _ = newRecoveryClipboard(123, &directTestDispatcher{}, backend, scheduler)
	if _, err := clipboard.Copy(testRecoveryMaterial(t), time.Second); err != nil {
		t.Fatal(err)
	}
	if err := clipboard.Clear(); !errors.Is(err, errClipboardClearFailed) || clipboard.current == nil || len(scheduler.timers) != 2 {
		t.Fatalf("explicit clear lost retry obligation: current=%#v timers=%d err=%v", clipboard.current, len(scheduler.timers), err)
	}
	scheduler.fire(1)
	if clipboard.current != nil || backend.payload != "" {
		t.Fatal("explicit-clear retry did not clear owned payload")
	}
}

func TestRecoveryClipboardAmbiguousPostExposureRetainsClearableLease(t *testing.T) {
	backend := &fakeClipboardBackend{failAfterExposure: true, ambiguousExposure: true}
	scheduler := &fakeClipboardScheduler{}
	clipboard, _ := newRecoveryClipboard(123, &directTestDispatcher{}, backend, scheduler)
	lease, err := clipboard.Copy(testRecoveryMaterial(t), time.Second)
	if !errors.Is(err, errClipboardCopyFailed) || lease == 0 || clipboard.current == nil || backend.payload == "" {
		t.Fatalf("ambiguous exposure lease=%d current=%#v err=%v", lease, clipboard.current, err)
	}
	backend.failAfterExposure = false
	scheduler.fire(0)
	if clipboard.current != nil || backend.payload != "" {
		t.Fatal("ambiguous exposed payload was not cleared by its retained lease")
	}
}

func TestRecoveryClipboardFailureSentinelsRetainAndRetryExactLease(t *testing.T) {
	for _, sentinel := range []string{"sequence-zero", "data-null", "global-size-zero"} {
		t.Run(sentinel, func(t *testing.T) {
			backend := &fakeClipboardBackend{clearSentinels: []string{sentinel}}
			scheduler := &fakeClipboardScheduler{}
			clipboard, _ := newRecoveryClipboard(123, &directTestDispatcher{}, backend, scheduler)
			if _, err := clipboard.Copy(testRecoveryMaterial(t), time.Second); err != nil {
				t.Fatal(err)
			}
			scheduler.fire(0)
			if clipboard.current == nil || backend.payload == "" || len(scheduler.timers) != 2 {
				t.Fatal("failure sentinel was misclassified as replacement")
			}
			scheduler.fire(1)
			if clipboard.current != nil || backend.payload != "" {
				t.Fatal("transient sentinel recovery did not clear exact lease")
			}
		})
	}
}

func TestRecoveryClipboardCheckAndClearHasNoExternalWriteGap(t *testing.T) {
	backend := &fakeClipboardBackend{clearEntered: make(chan struct{}), clearProceed: make(chan struct{})}
	clipboard, _ := newRecoveryClipboard(123, &directTestDispatcher{}, backend, &fakeClipboardScheduler{})
	if _, err := clipboard.Copy(testRecoveryMaterial(t), time.Second); err != nil {
		t.Fatal(err)
	}
	clearResult := make(chan error, 1)
	go func() { clearResult <- clipboard.Clear() }()
	<-backend.clearEntered
	externalDone := make(chan struct{})
	go func() {
		backend.externalWrite("newer user content")
		close(externalDone)
	}()
	close(backend.clearProceed)
	if err := <-clearResult; err != nil {
		t.Fatal(err)
	}
	<-externalDone
	if backend.payload != "newer user content" {
		t.Fatal("external copy was erased between validation and clear")
	}
}

func TestRecoveryClipboardRequiresRealOwner(t *testing.T) {
	if _, err := newRecoveryClipboard(0, &directTestDispatcher{}, &fakeClipboardBackend{}, &fakeClipboardScheduler{}); !errors.Is(err, errClipboardOwnerRequired) {
		t.Fatalf("null owner accepted: %v", err)
	}
}
