package winprobe

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestNativeSourcePreservesReviewedStaticContracts(t *testing.T) {
	t.Parallel()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	cpp := mustReadTestFile(t, filepath.Join(root, "native", "pulsar-capture", "pulsar_capture.cpp"))
	header := mustReadTestFile(t, filepath.Join(root, "native", "pulsar-capture", "pulsar_capture.h"))
	diagnosticsHeader := mustReadTestFile(t, filepath.Join(root, "native", "pulsar-capture", "pulsar_probe_diagnostics.h"))
	manifest := mustReadTestFile(t, filepath.Join(root, "probe-msix", "AppxManifest.xml.in"))
	windowsShell := mustReadTestFile(t, filepath.Join(root, "cmd", "pulsar-win-probe", "main_windows.go"))

	for _, forbidden := range []string{
		"CreateThread(",
		"FreeLibrary(",
		"SetEvent(session->original_notify)",
		"CloseHandle(session->original_notify)",
		"*(uint32_t*)",
		"AUDCLNT_E_NOT_ALLOWED",
	} {
		if strings.Contains(cpp, forbidden) {
			t.Errorf("native source contains forbidden pattern %q", forbidden)
		}
	}
	for _, required := range []string{
		"_beginthreadex",
		"creator_fence",
		"decltype(g_operations)::node_type registry_node",
		"CaptureSession* session = nullptr",
		"GetMixFormat",
		"Initialize(AUDCLNT_SHAREMODE_SHARED, AUDCLNT_STREAMFLAGS_EVENTCALLBACK",
		"SetEventHandle",
		"GetService(__uuidof(IAudioCaptureClient)",
		"GetNextPacketSize",
		"ReleaseBuffer",
		"std::atomic_thread_fence(std::memory_order_seq_cst)",
		"IStorageItemHandleAccess",
		"AudioCategory_Communications",
		"SetClientProperties(&properties)",
		"native_effects_verified{0}",
	} {
		if !strings.Contains(cpp, required) {
			t.Errorf("native source missing required contract marker %q", required)
		}
	}
	if strings.Contains(cpp, "native_effects_verified.store(1") {
		t.Error("native helper promotes capture effects without independent verification")
	}

	for _, export := range []string{
		"CapGetVersion", "CapInit", "CapDestroy", "CapIsQuiescent",
		"CapPermissionCheck", "CapPermissionRequest", "CapPermissionRequestCancel",
		"CapEnumerateDevices", "CapEnumerateDevicesCancel", "CapGetDefaultDevice",
		"CapturePrepare", "CapQualityGetVersion", "CaptureConfigureQuality", "CaptureGetQualityResult",
		"CaptureActivate", "CaptureGetResult", "CaptureRead", "CaptureRequestStop", "CaptureRelease",
		"PickerOpenFile", "PickerGetResult", "PickerCancel", "PickerRelease",
	} {
		if !strings.Contains(header, export+"(") {
			t.Errorf("native ABI header missing %s", export)
		}
	}
	for _, marker := range []string{
		"PULSAR_PROBE_DIAGNOSTICS_EXTENSION_V1 = 1",
		"PulsarProbeDiagnosticsGetVersion(",
		"PulsarProbeCaptureGetDiagnosticsV1(",
		"PulsarProbeCaptureDiagnosticsV1",
	} {
		if !strings.Contains(diagnosticsHeader, marker) {
			t.Errorf("private diagnostics contract missing %q", marker)
		}
	}
	for _, privateMarker := range []string{"PulsarProbeDiagnostics", "PulsarProbeCaptureDiagnostics", "CaptureGetDiagnostics"} {
		if strings.Contains(header, privateMarker) {
			t.Errorf("frozen public ABI header contains private extension marker %q", privateMarker)
		}
	}
	if !strings.Contains(cpp, "PulsarProbeDiagnosticsGetVersion(") || !strings.Contains(cpp, "PulsarProbeCaptureGetDiagnosticsV1(") {
		t.Error("native implementation does not expose the independently negotiated private diagnostics extension")
	}
	if strings.Contains(windowsShell, "framesDurablyScheduled") {
		t.Error("Windows shell labels appended frames as durably scheduled")
	}
	if !strings.Contains(windowsShell, `"framesWritten": writer.Frames()`) {
		t.Error("Windows shell does not expose the honest framesWritten metric on periodic-sync failure")
	}

	for _, forbidden := range []string{"runFullTrust", "broadFileSystemAccess", "documentsLibrary", "musicLibrary", "picturesLibrary", "videosLibrary", "removableStorage"} {
		if strings.Contains(manifest, forbidden) {
			t.Errorf("probe manifest contains prohibited capability %q", forbidden)
		}
	}
}

func mustReadTestFile(t *testing.T, path string) string {
	t.Helper()
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(raw)
}
