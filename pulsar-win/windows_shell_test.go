package main

import (
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
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
	want := []ShellSection{ShellHome, ShellCreate, ShellJoin, ShellTryLocally, ShellSoundboard, ShellHistory, ShellInbox, ShellAirs, ShellAutomation, ShellSettings}
	if !reflect.DeepEqual(shellSections, want) {
		t.Fatalf("sections=%v want %v", shellSections, want)
	}
	if NewShellCopy(ShellEnglish).Text(txtOnline) == NewShellCopy(ShellRussian).Text(txtOnline) {
		t.Fatal("EN and RU catalogs unexpectedly alias")
	}
}

func TestWindowsSoundboardProjectionShowsHonestShortcutAndRouting(t *testing.T) {
	snapshot := ShellSnapshot{SoundboardCues: []ShellSoundboardCue{{Title: "Bell", SourceKind: "builtin",
		ShortcutLabel: "Ctrl+Alt+F1", ShortcutStatus: WindowsShortcutConflict}}, SoundboardRoute: PhaseOneOwnBarycenter,
		SoundboardDelivery: PhaseOneOverlay, SoundboardIncludeOrigin: true, SoundboardHistoryCount: 2}
	for _, locale := range []ShellLocale{ShellEnglish, ShellRussian} {
		body := NewShellCopy(locale).Body(ShellSoundboard, snapshot)
		if !strings.Contains(body, "Bell") || !strings.Contains(body, "Ctrl+Alt+F1") || !strings.Contains(body, "conflict") || !strings.Contains(body, "2") {
			t.Fatalf("%s projection=%q", locale, body)
		}
	}
}

func TestWindowsHistoryRendersAutomationAttributionAndAvailableQuickControls(t *testing.T) {
	item := ShellPhaseOneHistoryItem{Title: "Bell", Status: "denied", AutomationTrigger: "schedule",
		AutomationActor: "Kitchen timer", AutomationSchedule: "Morning", AutomationCue: "Bell",
		AutomationReason: "automation_disabled", CanDisableSchedule: true, CanRevokePrincipal: true, CanEmergencyDisable: true}
	line := NewShellCopy(ShellEnglish).HistoryItem(item, 1, 1)
	for _, expected := range []string{"Kitchen timer", "Morning", "automation_disabled", "disable schedule", "revoke principal", "emergency disable"} {
		if !strings.Contains(line, expected) {
			t.Fatalf("history=%q missing %q", line, expected)
		}
	}
	if strings.Contains(line, "token") || strings.Contains(line, "selector") {
		t.Fatalf("history leaked secret vocabulary: %q", line)
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
		"wsVisible|wsOverlapped|wsCaption|wsSysMenu",
		"style &^= wsVisible", "showControl(control, true)", "ctx.renderHome(copy, snapshot)",
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
		"idShellAirs", "idShellAirConfirm", "esPassword", "windowText(ctx.airCode)",
		"showControl(ctx.airConfirm, airPage && confirming)", "pGetDpiForWindow", "ctx.laidOutSection != section",
		"idShellInbox", "idTargetsRefresh", "idTargetsSend", "idInboxReplay", "idTargetsHistoryDelete",
		"layoutWindowsTargetsInboxControls", "confirmWindowsPermanentDelete", "ReportSelectedTargetsHistory(windowText(ctx.targetsDetails))",
		"projection.ContentPolicyState == \"current\"", "TargetsInboxReady", "Ctrl+R",
		"idShellAutomation", "idAutomationEmergency", "idAutomationSecretCopy", "idAutomationSecretHide",
		"RequestAutomationAction(\"principal_issue\")", "ConfirmAutomationAction(windowText(ctx.automationName))",
		"windowText(ctx.automationTimezone)", "windowText(ctx.automationQuiet)",
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

func TestWindowsAirControlsRemainReachableAtSupportedDPI(t *testing.T) {
	for _, dpi := range []int{96, 120, 144, 192} {
		layout := layoutWindowsShell(dip(900, dpi), dip(760, dpi), dpi)
		layout.Body.Height = dip(210, dpi)
		controls := layoutWindowsAirControls(layout.Content, layout.Body.Bottom(), dpi)
		for index, rect := range controls.Rects() {
			if rect.X < layout.Content.X || rect.Right() > layout.Content.Right() || rect.Y < layout.Content.Y ||
				rect.Bottom() > layout.Client.Bottom() || rect.Width < dip(80, dpi) || rect.Height < dip(34, dpi) {
				t.Fatalf("dpi %d Air control %d unreachable: %+v in %+v", dpi, index, rect, layout)
			}
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

func TestWindowsTargetsInboxProjectionIsLocalizedAccessibleAndOpaque(t *testing.T) {
	now := time.Now()
	projection := targetsInboxFixture(now)
	projection.StateLabel = targetsStateLabel(TargetsInboxReady)
	projection.SelectedReferences = []string{projection.Targets[0].Reference}
	snapshot := ShellSnapshot{TargetsInbox: projection, TargetsInboxDelivery: PhaseOneOverlay,
		TargetsInboxReason: PhaseOneReportSpam, TargetsInboxActionOutcome: "replay_accepted"}
	for _, locale := range []ShellLocale{ShellEnglish, ShellRussian} {
		body := NewShellCopy(locale).Body(ShellInbox, snapshot)
		for _, required := range []string{"Voice", "1", "Replay"} {
			if locale == ShellRussian && required == "Replay" {
				required = "Повтор"
			}
			if !strings.Contains(body, required) {
				t.Errorf("%s missing %q: %q", locale, required, body)
			}
		}
		for _, forbidden := range []string{"trf_", "ib_", "hi_", projection.Targets[0].Reference} {
			if strings.Contains(body, forbidden) {
				t.Errorf("%s leaked opaque value %q: %q", locale, forbidden, body)
			}
		}
		if !strings.Contains(body, "[+]") {
			t.Errorf("%s outcome lacks non-color status: %q", locale, body)
		}
	}
}

func TestWindowsTargetsInboxControlsRemainReachableAtSupportedDPI(t *testing.T) {
	for _, dpi := range []int{96, 120, 144, 192} {
		layout := layoutWindowsShell(dip(960, dpi), dip(800, dpi), dpi)
		layout.Body.Height = dip(240, dpi)
		controls := layoutWindowsTargetsInboxControls(layout.Content, layout.Body.Bottom(), dpi)
		for index, rect := range controls.Rects() {
			if rect.X < layout.Content.X || rect.Right() > layout.Content.Right() || rect.Y < layout.Content.Y ||
				rect.Bottom() > layout.Client.Bottom() || rect.Width < dip(80, dpi) || rect.Height < dip(34, dpi) {
				t.Fatalf("dpi %d targets control %d unreachable: %+v in %+v", dpi, index, rect, layout)
			}
		}
	}
}

func TestWindowsAirProjectionIsLocalizedHonestAndOpaque(t *testing.T) {
	airID := "air_" + strings.Repeat("A", 26)
	snapshot := ShellSnapshot{Airs: []ShellAirItem{{
		AirID: airID, Title: "Family room", Status: "active", Role: AirRoleOwner,
		MembershipStatus: AirJoined, MemberCount: 2, ActiveMemberCount: 1, OnlinePulsarCount: 3,
		Capacity: AirCapacity{Barycenters: 8, OnlinePulsars: 16}, Current: true,
		Policy: AirPolicy{Revision: 2, Invite: AirInviteOwnerPrimary, Overlay: AirPlaybackAdminPrimary, Queue: AirPlaybackAdminPrimary, Replace: AirPlaybackAllMemberPrimarys},
	}}, AirInviteAvailable: true, AirConfirmAction: "dissolve"}
	for _, locale := range []ShellLocale{ShellEnglish, ShellRussian} {
		projection := NewShellCopy(locale).Body(ShellAirs, snapshot)
		for _, required := range []string{"Family room", "2", "8", "3", "16"} {
			if !strings.Contains(projection, required) {
				t.Errorf("%s missing %q: %q", locale, required, projection)
			}
		}
		for _, forbidden := range []string{airID, "aim_", "ai_", "secret"} {
			if strings.Contains(projection, forbidden) {
				t.Errorf("%s leaked %q: %q", locale, forbidden, projection)
			}
		}
		if !strings.Contains(projection, "[!]") {
			t.Errorf("%s confirmation lacks textual warning: %q", locale, projection)
		}
	}
	if !airInviteAllowed(snapshot.Airs[0]) {
		t.Fatal("owner lost invite action")
	}
	member := snapshot.Airs[0]
	member.Role = AirRoleMember
	if airInviteAllowed(member) {
		t.Fatal("member bypassed owner-only invite policy")
	}
}

func TestWindowsAirSnapshotNormalizesSelectionAndInviteRole(t *testing.T) {
	shell := NewWindowsShell(ShellEnglish, func() ShellSnapshot {
		return ShellSnapshot{SelectedAir: 99, AirInviteRole: AirRoleOwner, Airs: []ShellAirItem{{Title: "Room"}}}
	}, ShellActions{})
	got := shell.Snapshot()
	if got.SelectedAir != 0 || got.AirInviteRole != AirRoleMember {
		t.Fatalf("normalized=%+v", got)
	}
}

func TestWindowsAirCopyCoversFrozenLifecycleFailures(t *testing.T) {
	codes := []string{"invite_unavailable", "air_barycenter_capacity_reached", "air_online_pulsar_capacity_reached",
		"revision_conflict", "active_air_changed", "air_dissolved", "already_member", "membership_confirmation_required",
		"owner_transfer_required", "policy_denied", "coordinator_unavailable", "unauthenticated"}
	for _, locale := range []ShellLocale{ShellEnglish, ShellRussian} {
		copy := NewShellCopy(locale)
		for _, code := range codes {
			message := copy.AirActionMessage(code)
			if strings.TrimSpace(message) == "" || strings.Contains(message, code) {
				t.Errorf("%s code %s has unsafe/missing copy %q", locale, code, message)
			}
		}
	}
}
