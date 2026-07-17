package main

import (
	"os"
	"strings"
	"testing"

	protocol "relux.works/duet/pulsar-win/wire"
)

func TestWindowsCaptureQualityPresentationFailsClosedAndCyclesModes(t *testing.T) {
	unsupported := presentWindowsCaptureQuality(ShellSnapshot{})
	if unsupported.Quality != protocol.CaptureQualityUnsupported || unsupported.Reason != "mixed_version" || unsupported.BackendAvailable {
		t.Fatalf("mixed-version projection=%+v", unsupported)
	}
	preflight := presentWindowsCaptureQuality(ShellSnapshot{CaptureQualityBackendAvailable: true})
	if preflight.Quality != protocol.CaptureQualityDegraded || preflight.Reason != "aec_unavailable" || !preflight.RequiresDegradedConsent {
		t.Fatalf("unproven Windows preflight=%+v", preflight)
	}
	mode := WindowsCaptureQualityAuto
	for _, want := range []WindowsCaptureQualityMode{WindowsCaptureQualitySpeaker, WindowsCaptureQualityHeadphone, WindowsCaptureQualityAuto} {
		mode = nextWindowsCaptureQualityMode(mode)
		if mode != want {
			t.Fatalf("next mode=%s want %s", mode, want)
		}
	}
}

func TestWindowsCaptureQualityPresentationShowsExactRouteEffectsAndSeparateCeilings(t *testing.T) {
	state := acceptedCaptureQualityState()
	snapshot := ShellSnapshot{
		CaptureQualityMode: WindowsCaptureQualityAuto, CaptureQualityBackendAvailable: true,
		CaptureQualityState: state,
	}
	presentation := presentWindowsCaptureQuality(snapshot)
	if presentation.Quality != protocol.CaptureQualityAccepted || presentation.ResolvedMode != protocol.CaptureRouteSpeaker ||
		presentation.AEC != protocol.CaptureEffectActive || presentation.NS != protocol.CaptureEffectActive ||
		presentation.AGC != protocol.CaptureEffectActive || presentation.InputCeilingDBFS != -3 ||
		presentation.ReceiverOutputCeilingDBFS != -1 || !presentation.Active || !presentation.CanStop {
		t.Fatalf("accepted projection=%+v", presentation)
	}
	for _, locale := range []ShellLocale{ShellEnglish, ShellRussian} {
		projection := NewShellCopy(locale).CaptureQualityProjection(snapshot)
		for _, exact := range []string{"AEC:", "NS:", "AGC:", "-3 dBFS", "-1 dBFS"} {
			if !strings.Contains(projection, exact) {
				t.Fatalf("%s projection %q missing %q", locale, projection, exact)
			}
		}
	}
}

func TestWindowsCaptureQualityPresentationKeepsFailureAndHealthDistinct(t *testing.T) {
	state := acceptedCaptureQualityState()
	state.Quality = protocol.CaptureQualityDegraded
	state.Lifecycle = protocol.CaptureLifecycleFailed
	state.AEC = protocol.CaptureEffectFaulted
	state.InputHealth = protocol.CaptureHealthProcessorOverrun
	state.Reason = "processor_overrun"
	presentation := presentWindowsCaptureQuality(ShellSnapshot{CaptureQualityBackendAvailable: true, CaptureQualityState: state})
	if presentation.Active || presentation.CanStop || presentation.Reason != protocol.CaptureHealthProcessorOverrun || presentation.AEC != protocol.CaptureEffectFaulted {
		t.Fatalf("failed projection=%+v", presentation)
	}
	copy := NewShellCopy(ShellEnglish)
	if copy.CaptureEffect(protocol.CaptureEffectFaulted) == copy.CaptureEffect(protocol.CaptureEffectUnavailable) {
		t.Fatal("faulted and unavailable effects collapsed")
	}

	reconfiguring := acceptedCaptureQualityState()
	reconfiguring.Quality = protocol.CaptureQualityDegraded
	reconfiguring.Lifecycle = protocol.CaptureLifecycleReconfiguring
	reconfiguring.Reason = "reference_stale"
	reconfiguring.InputHealth = protocol.CaptureHealthReferenceStale
	if got := presentWindowsCaptureQuality(ShellSnapshot{CaptureQualityBackendAvailable: true, CaptureQualityState: reconfiguring}); !got.Active || !got.CanStop {
		t.Fatalf("route reconfiguration lost local Stop: %+v", got)
	}
}

func TestWindowsCaptureQualityGenerationAndLocalStopSurviveReconnectProjection(t *testing.T) {
	state := acceptedCaptureQualityState()
	for _, connection := range []ShellConnection{ShellOnline, ShellReconnecting, ShellDegraded} {
		snapshot := ShellSnapshot{
			Connection: connection, CaptureQualityBackendAvailable: true, CaptureQualityState: state,
		}
		normalized := snapshot.normalized()
		presentation := presentWindowsCaptureQuality(normalized)
		if normalized.CaptureQualityState == nil || normalized.CaptureQualityState.Generation != state.Generation ||
			!presentation.Active || !presentation.CanStop {
			t.Fatalf("connection %s erased local generation or Stop: snapshot=%+v presentation=%+v", connection, normalized, presentation)
		}
	}
}

func TestWindowsCaptureQualityCopyCoversTypedFailuresAndSettings(t *testing.T) {
	reasons := []string{
		"none", "mixed_version", "permission_denied", "no_device", "reference_unavailable", "reference_stale",
		"route_unknown", "route_excluded", "aec_unavailable", "ns_unavailable", "agc_unavailable", "silent",
		"too_quiet", "clipping", "clock_unstable", "processor_overrun", "device_lost",
		"user_selected_unprocessed", "rearm_timeout",
	}
	snapshot := ShellSnapshot{CaptureQualityBackendAvailable: true}
	for _, locale := range []ShellLocale{ShellEnglish, ShellRussian} {
		copy := NewShellCopy(locale)
		for _, reason := range reasons {
			if text := copy.CaptureQualityReason(reason); strings.TrimSpace(text) == "" || strings.Contains(text, "unknown") || strings.Contains(text, "неизвестно") {
				t.Errorf("%s reason %s=%q", locale, reason, text)
			}
		}
		for _, section := range []ShellSection{ShellTryLocally, ShellSettings} {
			body := copy.Body(section, snapshot)
			if !strings.Contains(body, copy.Text(txtCaptureQuality)) || !strings.Contains(body, "-3 dBFS") || !strings.Contains(body, "-1 dBFS") {
				t.Errorf("%s section %s lacks capture quality: %q", locale, section, body)
			}
		}
	}
}

func TestWindowsCaptureQualityNativeWindowAndTrayStayLocalAndAccessible(t *testing.T) {
	windowBytes, err := os.ReadFile("main_window_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	trayBytes, err := os.ReadFile("ui_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	mainBytes, err := os.ReadFile("main.go")
	if err != nil {
		t.Fatal(err)
	}
	window, tray, production := string(windowBytes), string(trayBytes), string(mainBytes)
	for _, required := range []string{
		`mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idCaptureQualityMode)`,
		`mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idCaptureQualityConsent)`,
		`mk(0, "BUTTON", "", buttonStyle|bsPushButton|bsMultiline, idCaptureQualityStop)`,
		"vkPeriod", "idCaptureQualityStop", "idShellCancel", "actions.StopActiveCapture", "wmDPIChanged", "pGetDpiForWindow",
	} {
		if !strings.Contains(window, required) {
			t.Errorf("native capture-quality UI missing %q", required)
		}
	}
	for _, required := range []string{"menuCaptureStop", "presentWindowsCaptureQuality", "actions.StopActiveCapture", "CaptureQualityLabel"} {
		if !strings.Contains(tray, required) {
			t.Errorf("tray capture-quality UI missing %q", required)
		}
	}
	for _, required := range []string{"SetCaptureQuality:  workflow.SetCaptureQuality", "StopActiveCapture:  workflow.Cancel"} {
		if !strings.Contains(production, required) {
			t.Errorf("production local workflow seam missing %q", required)
		}
	}
	for _, forbidden := range []string{"CapabilityLivePTT", "NewWindowsLivePTTNode("} {
		if strings.Contains(production, forbidden) {
			t.Errorf("capture quality UI remotely enabled production-dark live PTT through %q", forbidden)
		}
	}
}
