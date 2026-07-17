package winprobe

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
	"unsafe"
)

func TestCaptureFormatABISize(t *testing.T) {
	t.Parallel()
	if got := uint32(unsafe.Sizeof(CaptureFormat{})); got != CaptureFormatStructSize {
		t.Fatalf("CaptureFormat size = %d, want %d", got, CaptureFormatStructSize)
	}
}

func TestCaptureQualityNativeLayoutIsVersionedSeparately(t *testing.T) {
	t.Parallel()
	if got := uint32(unsafe.Sizeof(CaptureQualityNative{})); got != CaptureQualityNativeStructSize {
		t.Fatalf("CaptureQualityNative size = %d, want %d", got, CaptureQualityNativeStructSize)
	}
	value := NewCaptureQualityNative()
	if value.Version != CaptureQualityNativeVersion || value.StructSize != CaptureQualityNativeStructSize {
		t.Fatalf("capture quality header = %+v", value)
	}
}

func TestProbeDiagnosticsV1WireSizeIsIndependent(t *testing.T) {
	t.Parallel()
	if got := uint32(unsafe.Sizeof(probeCaptureDiagnosticsV1{})); got != ProbeCaptureDiagnosticsV1StructSize {
		t.Fatalf("private probe diagnostics size = %d, want %d", got, ProbeCaptureDiagnosticsV1StructSize)
	}
	if HelperABIVersion != 1 || ProbeDiagnosticsExtensionVersion != 1 {
		t.Fatalf("core/extension versions = %d/%d, want independently frozen 1/1", HelperABIVersion, ProbeDiagnosticsExtensionVersion)
	}
}

func TestHResultFromUintptrTruncatesToLow32Bits(t *testing.T) {
	t.Parallel()

	got := HResultFromUintptr(0x0000000180070005)
	want := HResult(-2147024891)
	if got != want {
		t.Fatalf("HResultFromUintptr() = %#x, want %#x", int32(got), int32(want))
	}
	if !got.Failed() {
		t.Fatalf("Failed() = false, want true")
	}
	if got.Hex() != "0x80070005" {
		t.Fatalf("Hex() = %q, want 0x80070005", got.Hex())
	}
}

func TestHigherPriorityReason(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		cur  CaptureReason
		next CaptureReason
		want CaptureReason
	}{
		{name: "overflow beats user stop", cur: ReasonUserStop, next: ReasonOverflow, want: ReasonOverflow},
		{name: "permission beats shutdown", cur: ReasonShutdown, next: ReasonPermissionRevoke, want: ReasonPermissionRevoke},
		{name: "equal rank keeps first", cur: ReasonWasapiError, next: ReasonFormatError, want: ReasonWasapiError},
		{name: "lower priority ignored", cur: ReasonDiscontinuity, next: ReasonUserStop, want: ReasonDiscontinuity},
	}
	for _, tc := range tests {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := HigherPriorityReason(tc.cur, tc.next); got != tc.want {
				t.Fatalf("HigherPriorityReason(%v, %v) = %v, want %v", tc.cur, tc.next, got, tc.want)
			}
		})
	}
}

func TestPublicCaptureState(t *testing.T) {
	t.Parallel()

	if got := PublicCaptureState(ReasonCancel); got != CaptureStateCancelled {
		t.Fatalf("cancel -> %v, want cancelled", got)
	}
	if got := PublicCaptureState(ReasonUserStop); got != CaptureStateStopped {
		t.Fatalf("user stop -> %v, want stopped", got)
	}
	if got := PublicCaptureState(ReasonOverflow); got != CaptureStateFailed {
		t.Fatalf("overflow -> %v, want failed", got)
	}
}

func TestJSONLoggerIncludesStructuredFields(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := NewJSONLogger(&buf)
	visible := false
	if err := logger.Log(LogEvent{
		Scenario:         ScenarioPicker,
		Result:           ResultBlocked,
		Action:           "picker_open",
		SelectedAPIPath:  "FileOpenPicker+IStorageItemHandleAccess",
		PermissionStatus: PermissionAllowed.String(),
		HResult:          "0x80004005",
		FailureCause:     "Create handle failed",
		WindowVisible:    &visible,
		Fields: map[string]any{
			"ownerPath": "visible_window",
		},
	}); err != nil {
		t.Fatalf("Log() error = %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal() error = %v", err)
	}
	if got["scenario"] != string(ScenarioPicker) {
		t.Fatalf("scenario = %v, want %q", got["scenario"], ScenarioPicker)
	}
	if got["selectedApiPath"] != "FileOpenPicker+IStorageItemHandleAccess" {
		t.Fatalf("selectedApiPath = %v", got["selectedApiPath"])
	}
	if got["permissionStatus"] != PermissionAllowed.String() {
		t.Fatalf("permissionStatus = %v", got["permissionStatus"])
	}
	if got["result"] != string(ResultBlocked) {
		t.Fatalf("result = %v", got["result"])
	}
}

func TestProbeManifestKeepsReviewedSandbox(t *testing.T) {
	t.Parallel()

	manifestPath := filepath.Join("..", "..", "probe-msix", "AppxManifest.xml.in")
	got, err := InspectManifest(manifestPath)
	if err != nil {
		t.Fatalf("InspectManifest() error = %v", err)
	}
	if err := got.Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestPackagePayloadRequiresActualHelper(t *testing.T) {
	t.Parallel()
	valid := []string{"stage/AppxManifest.xml", "stage/pulsar-win-probe-amd64.exe", "stage/pulsar-capture.dll"}
	if err := ValidatePackagePayload(valid); err != nil {
		t.Fatalf("ValidatePackagePayload() error = %v", err)
	}
	if err := ValidatePackagePayload(valid[:2]); err == nil {
		t.Fatal("ValidatePackagePayload() accepted payload without helper DLL")
	}
}
