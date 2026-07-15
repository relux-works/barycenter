package main

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestWindowsShellCatalogAndInformationArchitecture(t *testing.T) {
	for _, locale := range []ShellLocale{ShellEnglish, ShellRussian} {
		if missing := catalogMissing(locale); len(missing) != 0 {
			t.Fatalf("%s catalog missing %v", locale, missing)
		}
		copy := NewShellCopy(locale)
		for _, section := range shellSections {
			if strings.TrimSpace(copy.Section(section)) == "" {
				t.Fatalf("%s section %s has no label", locale, section)
			}
		}
	}
	want := []ShellSection{ShellHome, ShellCreate, ShellJoin, ShellTryLocally, ShellHistory, ShellSettings}
	if !reflect.DeepEqual(shellSections, want) {
		t.Fatalf("sections=%v want %v", shellSections, want)
	}
	if NewShellCopy(ShellEnglish).Text(txtOnline) == NewShellCopy(ShellRussian).Text(txtOnline) {
		t.Fatal("EN and RU catalogs unexpectedly alias")
	}
}

func TestWindowsNativeShellBlindBuildContracts(t *testing.T) {
	native, err := os.ReadFile("main_window_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	tray, err := os.ReadFile("ui_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	output, err := os.ReadFile("windows_output_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile("winres/pulsar.exe.manifest")
	if err != nil {
		t.Fatal(err)
	}
	nativeText, trayText := string(native), string(tray)
	for _, seam := range []string{
		"wmDPIChanged", "pGetDpiForWindow", "pIsDialogMessageW",
		"pTranslateAcceleratorW", "wmGetMinMax", "wsExControlParent",
		`mk(0, "BUTTON"`, `mk(0, "STATIC"`, "wmDropFiles", "pDragAcceptFiles", "AcceptDroppedFile",
		"windowText(ctx.identityInput)", "chooseWindowsRecoveryDestination", "idShellSend", "SendSelectedDraft",
		`mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idShellReportReason)`,
		`mk(0, "STATIC", "", wsChild|wsVisible|ssLeft, 0)`,
		`mk(wsExClientEdge, "EDIT", "", wsChild|wsVisible|wsTabStop|0x0080, idShellReportDetails)`,
		"pSendMessageW.Call(uintptr(ctx.reportDetails), emSetLimitText, 2000, 0)",
		"ReportSelectedHistoryItem(windowText(ctx.reportDetails))",
		`setText(ctx.reportLabel, "Details (optional)")`, `setText(ctx.reportLabel, "Детали (необязательно)")`,
		"showControl(ctx.historyDelete, historyPage && hasHistory && selectedHistory.CanDelete)",
		"showControl(ctx.historyReport, historyPage && hasHistory && selectedHistory.CanReport)",
	} {
		if !strings.Contains(nativeText, seam) {
			t.Errorf("native shell missing %q", seam)
		}
	}
	for _, seam := range []string{"EnumAudioEndpoints", "GetDefaultAudioEndpoint", "renderLoop", "SelectNext"} {
		if !strings.Contains(string(output), seam) {
			t.Errorf("output selection missing %q", seam)
		}
	}
	for _, command := range []string{
		"menuOpen", "menuCreate", "menuJoin", "menuTry", "menuRecord", "menuDND", "menuQuitCmd",
	} {
		if !strings.Contains(trayText, command) {
			t.Errorf("tray missing %q", command)
		}
	}
	if strings.Contains(trayText, "tpmLeftAlign|tpmRightBtn|tpmReturnCmd") {
		t.Error("tray suppresses WM_COMMAND by ignoring TPM_RETURNCMD result")
	}
	if !strings.Contains(string(manifest), "PerMonitorV2") {
		t.Error("packaged executable is not PerMonitorV2-aware")
	}
}

func TestWindowsModerationCopyCoversFrozenReasonsAndPrivacySafeOutcomes(t *testing.T) {
	for _, locale := range []ShellLocale{ShellEnglish, ShellRussian} {
		copy := NewShellCopy(locale)
		for _, reason := range phaseOneModerationReasons {
			if label := copy.ModerationReason(reason); strings.TrimSpace(label) == "" || strings.Contains(label, string(reason)+"_") {
				t.Errorf("%s reason %s has non-user-facing label %q", locale, reason, label)
			}
		}
		for _, outcome := range []string{
			"media_deleted", "report_received", "report_already_received", "sender_blocked",
			"sender_already_blocked", "action_not_allowed", "coordinator_unavailable", "unauthorized", "insufficient_capability",
		} {
			message := copy.PhaseOneActionMessage(outcome)
			if strings.TrimSpace(message) == "" || strings.Contains(message, "hi_") || strings.Contains(message, "rp_") || strings.Contains(message, "bl_") {
				t.Errorf("%s outcome %s unsafe/missing message %q", locale, outcome, message)
			}
		}
	}
	snapshot := ShellSnapshot{
		PhaseOneHistory:      []ShellPhaseOneHistoryItem{{Title: "Foreign clip", SenderName: "Sender", Status: "played", CanReport: true, CanBlock: true}},
		SelectedReportReason: PhaseOneReportHarassment, PhaseOneActionOutcome: "report_received",
	}
	if body := NewShellCopy(ShellEnglish).Body(ShellHistory, snapshot); !strings.Contains(body, "Report received") || strings.Contains(body, "report_received") {
		t.Fatalf("history outcome not localized: %q", body)
	}
}

func TestWindowsShellPhaseOneLabelsAreCanonicalAndHideOpaqueIDs(t *testing.T) {
	snapshot := ShellSnapshot{
		SelectedPhaseOneRoute: PhaseOneOwnBarycenter, SelectedPhaseOneDelivery: PhaseOneOverlay,
		PhaseOneDrafts: []ShellPhaseOneDraft{{
			Title: "Daily note", State: PhaseOneDraftRetryableFailure,
			RequestedDelivery: PhaseOneOverlay, EffectiveDelivery: PhaseOneAfterCurrent,
			DowngradeReason: "mandatory_target_missing_overlay_capability", FailureCode: "coordinator_unavailable",
		}},
		PhaseOneHistory: []ShellPhaseOneHistoryItem{{
			Title: "Team update", SenderName: "Ivan", Status: "played",
			RequestedDelivery: "overlay", EffectiveDelivery: "after_current", PlayedCount: 1,
		}},
	}
	for _, locale := range []ShellLocale{ShellEnglish, ShellRussian} {
		copy := NewShellCopy(locale)
		projection := copy.Draft(snapshot) + "\n" + copy.Body(ShellHistory, snapshot)
		deliveryLabel := "After current"
		if locale == ShellRussian {
			deliveryLabel = "После текущего"
		}
		for _, required := range []string{"Daily note", "Team update", deliveryLabel} {
			if !strings.Contains(projection, required) {
				t.Errorf("%s projection missing %q: %q", locale, required, projection)
			}
		}
		for _, forbidden := range []string{"m_", "tr_", "hi_"} {
			if strings.Contains(projection, forbidden) {
				t.Errorf("%s projection leaked opaque ID marker %q: %q", locale, forbidden, projection)
			}
		}
	}
}

func TestWindowsShellStatesNeverDependOnColorAlone(t *testing.T) {
	copy := NewShellCopy(ShellEnglish)
	for _, state := range []ShellConnection{ShellUnpaired, ShellReconnecting, ShellOnline, ShellDegraded} {
		label := copy.Connection(ShellSnapshot{Connection: state})
		if !strings.HasPrefix(label, "[") || !strings.Contains(label, "] ") {
			t.Fatalf("connection %s lacks textual indicator: %q", state, label)
		}
	}
	for _, state := range []ShellRecording{
		ShellRecordingUnavailable, ShellRecordingIdle, ShellRecordingActive,
		ShellRecordingProcessing, ShellRecordingFailed,
	} {
		if label := copy.Recording(ShellSnapshot{Recording: state}); !strings.HasPrefix(label, "[") {
			t.Fatalf("recording %s lacks textual indicator: %q", state, label)
		}
	}
}

func TestWindowsShellHonestActionAvailability(t *testing.T) {
	unpaired := ShellSnapshot{Connection: ShellUnpaired, Recording: ShellRecordingUnavailable}
	for _, section := range shellSections {
		if !shellActionEnabled(unpaired, section) {
			t.Fatalf("unpaired shell lost navigation to %s", section)
		}
	}
	if shellDNDEnabled(unpaired) || shellRecordingEnabled(unpaired) {
		t.Fatal("unpaired shell enabled unavailable DND or recording")
	}
	active := ShellSnapshot{Connection: ShellReconnecting, Recording: ShellRecordingActive}
	if !shellDNDEnabled(active) || !shellRecordingEnabled(active) {
		t.Fatal("degraded/active shell hid a safe DND or Stop path")
	}
	selfTest := ShellSnapshot{Recording: ShellRecordingIdle, RecordingAvailable: true, SelfTestPhase: WindowsLocalSelfTestRecording}
	if shellRecordingEnabled(selfTest) || !shellLocalCaptureBusy(selfTest) {
		t.Fatal("normal recording remained enabled during the five-second self-test")
	}
}

func TestWindowsShellProjectsLocalInputMeterAndDraft(t *testing.T) {
	snapshot := ShellSnapshot{
		SelfTestAvailable: true, SelfTestPhase: WindowsLocalSelfTestRecording, SelfTestMeter: .42,
		LocalDraftAvailable: true, LocalDraftName: "voice.wav",
		CaptureInputs: []WindowsCaptureInput{{ID: "id", Name: "Studio microphone"}},
		AudioOutputs:  []WindowsAudioOutput{{ID: "out", Name: "Studio speakers"}},
	}
	for _, locale := range []ShellLocale{ShellEnglish, ShellRussian} {
		body := NewShellCopy(locale).Body(ShellTryLocally, snapshot)
		for _, required := range []string{"Studio microphone", "Studio speakers", "42%", "voice.wav"} {
			if !strings.Contains(body, required) {
				t.Errorf("%s local projection missing %q: %q", locale, required, body)
			}
		}
	}
}

func TestWindowsShellNormalizesUntrustedRuntimeProjection(t *testing.T) {
	shell := NewWindowsShell(ShellEnglish, func() ShellSnapshot {
		return ShellSnapshot{Connection: "unknown", DND: "unknown", Recording: "unknown", Volume: 240, PresenceOnline: 4, PresenceTotal: 2}
	}, ShellActions{})
	got := shell.Snapshot()
	if got.Volume != 100 || got.PresenceTotal != 4 || got.Connection != ShellUnpaired ||
		got.DND != ShellDNDAllowAll || got.Recording != ShellRecordingUnavailable {
		t.Fatalf("normalization failed: %+v", got)
	}
	shell.Select(ShellSettings)
	shell.Select(ShellSection("unknown"))
	if shell.Section() != ShellSettings {
		t.Fatal("unknown section changed navigation")
	}
}

func TestWindowsShellDPIContracts(t *testing.T) {
	for _, dpi := range []int{96, 120, 144, 192} {
		layout := layoutWindowsShell(dip(780, dpi), dip(500, dpi), dpi)
		if layout.Sidebar.Width < dip(180, dpi) || layout.Content.Width < dip(380, dpi) {
			t.Fatalf("dpi %d loses navigation/content: %+v", dpi, layout)
		}
		regions := append([]ShellRect{layout.Header, layout.Banner, layout.Body}, layout.Cards[:]...)
		for _, region := range regions {
			if region.X < layout.Content.X || region.Right() > layout.Content.Right() ||
				region.Y < layout.Content.Y || region.Bottom() > layout.Client.Bottom() ||
				region.Width <= 0 || region.Height <= 0 {
				t.Fatalf("dpi %d region outside usable content: %+v in %+v", dpi, region, layout)
			}
		}
		if layout.Cards[0].Right() > layout.Cards[1].X || layout.Cards[1].Right() > layout.Cards[2].X {
			t.Fatalf("dpi %d cards overlap: %+v", dpi, layout.Cards)
		}
	}
}

func TestWindowsShellKeyboardContractIsUnique(t *testing.T) {
	seen := map[string]bool{}
	commands := map[string]bool{}
	for _, shortcut := range shellShortcuts {
		key := shortcut.Key
		if shortcut.Control {
			key = "Ctrl+" + key
		}
		if shortcut.Shift {
			key = "Shift+" + key
		}
		if seen[key] {
			t.Fatalf("duplicate shortcut %s", key)
		}
		seen[key] = true
		commands[shortcut.Command] = true
	}
	for _, required := range []string{"open", "section", "record", "dnd"} {
		if !commands[required] {
			t.Fatalf("missing %s shortcut", required)
		}
	}
}
