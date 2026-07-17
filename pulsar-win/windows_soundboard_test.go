package main

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestWindowsSoundboardPreferencesAreBoundedAndSecretFree(t *testing.T) {
	path := filepath.Join(t.TempDir(), "soundboard", "preferences.v1.json")
	store := WindowsSoundboardPreferenceStore{Path: path}
	cueID := "cq_" + strings.Repeat("A", 26)
	prefs := DefaultWindowsSoundboardPreferences()
	prefs.SelectedCueID = cueID
	prefs.Shortcuts = []WindowsSoundboardShortcutBinding{{CueID: cueID,
		Shortcut: WindowsRecordingShortcut{VirtualKey: 0x70, Modifiers: WindowsShortcutModControl | WindowsShortcutModAlt}}}
	if err := store.Save(prefs); err != nil {
		t.Fatal(err)
	}
	if loaded := store.Load(); loaded.SelectedCueID != cueID || len(loaded.Shortcuts) != 1 {
		t.Fatalf("loaded=%+v", loaded)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"bearer", "token", "microphone", "media_id", "local_path"} {
		if strings.Contains(strings.ToLower(string(data)), forbidden) {
			t.Fatalf("preference persisted forbidden field %q: %s", forbidden, data)
		}
	}
	var decoded map[string]any
	if json.Unmarshal(data, &decoded) != nil || len(decoded) != 6 {
		t.Fatalf("unexpected persisted schema: %s", data)
	}
}

func TestWindowsSoundboardHotkeysReportConflictsAndKeepButtonFallback(t *testing.T) {
	registrar := newFakeWindowsShortcutRegistrar()
	recording := DefaultWindowsRecordingShortcut()
	cueA := "cq_" + strings.Repeat("A", 26)
	cueB := "cq_" + strings.Repeat("B", 26)
	hotkey := WindowsRecordingShortcut{VirtualKey: 0x70, Modifiers: WindowsShortcutModControl | WindowsShortcutModAlt}
	triggered := []string{}
	controller := NewWindowsSoundboardShortcutController(registrar, recording,
		[]WindowsSoundboardShortcutBinding{{CueID: cueA, Shortcut: recording}, {CueID: cueB, Shortcut: hotkey}},
		func(cueID string) { triggered = append(triggered, cueID) })
	controller.Start()
	states := controller.States()
	if len(states) != 2 || states[0].Status != WindowsShortcutConflict || states[1].Status != WindowsShortcutRegistered || len(registrar.live) != 1 {
		t.Fatalf("states=%+v live=%v", states, registrar.live)
	}
	if !controller.HandleHotKey(registrar.next) || len(triggered) != 1 || triggered[0] != cueB {
		t.Fatalf("hotkey trigger=%v", triggered)
	}
	// The visible button calls the same canonical trigger callback directly;
	// a RegisterHotKey conflict never disables that path.
	triggered = append(triggered, cueA)
	controller.Suspend(WindowsShortcutSessionLocked)
	if controller.HandleHotKey(registrar.next) || len(registrar.live) != 0 {
		t.Fatal("locked session retained a live cue shortcut")
	}
	controller.Resume(WindowsShortcutSessionLocked)
	controller.Stop()
	if len(triggered) != 2 || len(registrar.live) != 0 {
		t.Fatalf("fallback/lifecycle=%v live=%v", triggered, registrar.live)
	}
}

type fakeWindowsSoundboardService struct {
	mu           sync.Mutex
	list         SoundboardCueList
	history      PhaseOneHistoryPage
	triggers     []SoundboardTriggerIntent
	triggerIDs   []string
	renamed      string
	deleted      bool
	reordered    []string
	uploaded     string
	createdMedia string
	deletedMedia []string
	keys         []string
	challenge    bool
}

func (s *fakeWindowsSoundboardService) SoundboardCues(context.Context) (SoundboardCueList, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.list, nil
}
func (s *fakeWindowsSoundboardService) History(context.Context, int, string) (PhaseOneHistoryPage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.history, nil
}

func (s *fakeWindowsSoundboardService) CreateSoundboardMediaCue(_ context.Context, _ string, mediaID string, _ string) (SoundboardCueList, error) {
	s.mu.Lock()
	s.createdMedia = mediaID
	s.mu.Unlock()
	return s.list, nil
}

func (s *fakeWindowsSoundboardService) Upload(_ context.Context, path, _ string, _ string, rights bool) (PhaseOneUploadConfirmation, error) {
	if !rights {
		return PhaseOneUploadConfirmation{}, phaseOneError(PhaseOneInvalidRequest)
	}
	s.mu.Lock()
	s.uploaded = path
	s.mu.Unlock()
	return PhaseOneUploadConfirmation{MediaID: "m_" + strings.Repeat("9", 26)}, nil
}

func (s *fakeWindowsSoundboardService) DeleteMedia(_ context.Context, mediaID string) error {
	s.mu.Lock()
	s.deletedMedia = append(s.deletedMedia, mediaID)
	s.mu.Unlock()
	return nil
}
func (s *fakeWindowsSoundboardService) RenameSoundboardCue(_ context.Context, _ string, title string, _ int64, _ string) (SoundboardCueList, error) {
	s.mu.Lock()
	s.renamed = title
	s.mu.Unlock()
	return s.list, nil
}
func (s *fakeWindowsSoundboardService) DeleteSoundboardCue(context.Context, string, int64, string) (SoundboardCueList, error) {
	s.mu.Lock()
	s.deleted = true
	s.mu.Unlock()
	return s.list, nil
}
func (s *fakeWindowsSoundboardService) ReorderSoundboardCues(_ context.Context, ids []string, _ int64, _ string) (SoundboardCueList, error) {
	s.mu.Lock()
	s.reordered = append([]string(nil), ids...)
	s.mu.Unlock()
	return s.list, nil
}
func (s *fakeWindowsSoundboardService) TriggerSoundboardCue(_ context.Context, cueID string, intent SoundboardTriggerIntent, key string) (SoundboardTriggerReceipt, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.triggerIDs = append(s.triggerIDs, cueID)
	s.triggers = append(s.triggers, intent)
	s.keys = append(s.keys, key)
	if s.challenge && intent.Fallback == nil {
		return SoundboardTriggerReceipt{}, &PhaseOneClientError{Kind: PhaseOneRejected, Code: "requires_confirmation",
			ConfirmationToken: "fc_" + strings.Repeat("a", 64), Alternatives: []PhaseOneFallbackAlternative{{Delivery: PhaseOneAfterCurrent, Available: true}}}
	}
	return SoundboardTriggerReceipt{ExecutionID: "mx_" + strings.Repeat("C", 26), PhaseOneTransmissionReceipt: PhaseOneTransmissionReceipt{TransmissionID: "tr_" + strings.Repeat("D", 26)}}, nil
}

func TestWindowsSoundboardCompositionTriggersWithoutCaptureAndProjectsAutomationHistory(t *testing.T) {
	cueA, cueB := "cq_"+strings.Repeat("A", 26), "cq_"+strings.Repeat("B", 26)
	service := &fakeWindowsSoundboardService{list: SoundboardCueList{OrderRevision: 7, Cues: []SoundboardCue{
		{ID: cueA, Title: "Bell", State: "active", Revision: 2, Position: 0},
		{ID: cueB, Title: "Alert", State: "active", Revision: 3, Position: 1},
	}}, history: PhaseOneHistoryPage{Items: []PhaseOneHistoryItem{
		{ID: "hi_" + strings.Repeat("E", 26), Title: "Bell", Automation: &PhaseOneAutomationHistory{TriggerKind: "manual_soundboard", CueID: cueA, Outcome: "played"}},
		{ID: "hi_" + strings.Repeat("F", 26), Title: "ordinary"},
	}}}
	composition, err := NewWindowsSoundboardComposition(service, service,
		WindowsSoundboardPreferenceStore{Path: filepath.Join(t.TempDir(), "preferences.json")})
	if err != nil {
		t.Fatal(err)
	}
	defer composition.Close()
	deadline := time.Now().Add(time.Second)
	for len(composition.Snapshot().Cues) != 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	composition.SelectNextCue()
	composition.CycleSelectedShortcut()
	composition.SelectNextDelivery()
	composition.TriggerSelected()
	for composition.Snapshot().Busy && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	state := composition.Snapshot()
	if state.Outcome != "accepted" || len(state.History) != 1 || state.History[0].Automation.TriggerKind != "manual_soundboard" {
		t.Fatalf("state=%+v", state)
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if len(service.triggerIDs) != 1 || service.triggerIDs[0] != cueB || service.triggers[0].Delivery != PhaseOneInterrupt {
		t.Fatalf("trigger ids=%v intents=%+v", service.triggerIDs, service.triggers)
	}
	if prefs := composition.Preferences(); len(prefs.Shortcuts) != 1 || prefs.Shortcuts[0].CueID != cueB || prefs.Shortcuts[0].Shortcut.Label() != "Ctrl+Alt+F2" {
		t.Fatalf("shortcut prefs=%+v", prefs)
	}
}

func TestWindowsSoundboardWindowsSourceUsesRegisterHotKeyAndNoCaptureHook(t *testing.T) {
	files := []string{"windows_soundboard_shortcuts.go", "windows_recording_hotkey_windows.go", "ui_windows.go", "main_window_windows.go", "main.go"}
	var source string
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		source += string(data)
	}
	if !strings.Contains(source, "RegisterHotKey") || !strings.Contains(source, "button path") {
		t.Fatal("soundboard shortcut posture is not explicit")
	}
	for _, required := range []string{"menuSoundboardTrigger", "ShellSoundboard", "TriggerSelectedSoundboardCue", "currentWindowsSoundboardShortcutStates", "CycleSelectedSoundboardShortcut"} {
		if !strings.Contains(source, required) {
			t.Errorf("Windows soundboard wiring missing %q", required)
		}
	}
	for _, forbidden := range []string{"SetWindowsHookEx", "WH_KEYBOARD", "StartMicrophone", "CaptureAudio"} {
		if strings.Contains(source, forbidden) {
			t.Fatalf("soundboard shortcut source contains %q", forbidden)
		}
	}
}

func TestWindowsSoundboardBrokeredCueUploadCleansLocalBytesAndNeverCaptures(t *testing.T) {
	service := &fakeWindowsSoundboardService{list: SoundboardCueList{}}
	composition, err := NewWindowsSoundboardComposition(service, service,
		WindowsSoundboardPreferenceStore{Path: filepath.Join(t.TempDir(), "preferences.json")})
	if err != nil {
		t.Fatal(err)
	}
	defer composition.Close()
	store := NewCaptureMediaStore(filepath.Join(t.TempDir(), "media"))
	composition.media, composition.mediaStore = service, store
	var released atomic.Int32
	raw := phaseOneCueBytes(t)
	composition.AcceptBrokeredCue(WindowsBrokeredAudioFile{DisplayName: "Bell.wav", SizeBytes: int64(len(raw)),
		Open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(raw)), nil }, Release: func() { released.Add(1) }})
	deadline := time.Now().Add(time.Second)
	for composition.Snapshot().Busy && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if state := composition.Snapshot(); state.Outcome != "cue_created" || state.Failure != "" {
		t.Fatalf("state=%+v", state)
	}
	service.mu.Lock()
	uploaded, created := service.uploaded, service.createdMedia
	service.mu.Unlock()
	if released.Load() != 1 || uploaded == "" || created != "m_"+strings.Repeat("9", 26) {
		t.Fatalf("released=%d uploaded=%q created=%q", released.Load(), uploaded, created)
	}
	if _, statErr := os.Stat(uploaded); !os.IsNotExist(statErr) {
		t.Fatalf("staged soundboard bytes retained: %v", statErr)
	}
}

func TestWindowsSoundboardInterruptConfirmationReusesIntentAndKey(t *testing.T) {
	cueID := "cq_" + strings.Repeat("A", 26)
	service := &fakeWindowsSoundboardService{challenge: true, list: SoundboardCueList{OrderRevision: 1,
		Cues: []SoundboardCue{{ID: cueID, Title: "Bell", State: "active", Revision: 1}}}}
	composition, err := NewWindowsSoundboardComposition(service, service,
		WindowsSoundboardPreferenceStore{Path: filepath.Join(t.TempDir(), "preferences.json")})
	if err != nil {
		t.Fatal(err)
	}
	defer composition.Close()
	deadline := time.Now().Add(time.Second)
	for len(composition.Snapshot().Cues) == 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	composition.SelectNextDelivery() // overlay -> interrupt
	composition.TriggerSelected()
	for composition.Snapshot().Busy && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if composition.Snapshot().Outcome != "confirmation_required" {
		t.Fatalf("challenge=%+v", composition.Snapshot())
	}
	composition.TriggerSelected()
	for composition.Snapshot().Busy && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	service.mu.Lock()
	defer service.mu.Unlock()
	if len(service.triggers) != 2 || service.triggers[1].Fallback == nil || service.triggers[1].Fallback.Delivery != PhaseOneAfterCurrent ||
		service.keys[0] != service.keys[1] || composition.Snapshot().Outcome != "accepted" {
		t.Fatalf("triggers=%+v keys=%v state=%+v", service.triggers, service.keys, composition.Snapshot())
	}
}
