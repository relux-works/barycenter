//go:build windows

package winprobe

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32System          = windows.NewLazySystemDLL("kernel32.dll")
	procLoadPackagedLibrary = kernel32System.NewProc("LoadPackagedLibrary")
	loadPackagedLibraryFn   = callLoadPackagedLibrary
	loadLibraryExFn         = windows.LoadLibraryEx
	executablePathFn        = os.Executable
)

type Helper struct {
	dll                         *windows.DLL
	LoadedVia                   LoaderChoice
	DiagnosticsExtensionVersion uint32
	CaptureQualityVersion       uint32
	procs                       map[string]*windows.Proc
}

func LoadHelper() (*Helper, error) {
	dll, choice, err := loadHelperModule()
	if err != nil {
		return nil, err
	}
	helper := &Helper{dll: dll, LoadedVia: choice, procs: make(map[string]*windows.Proc)}
	for _, name := range coreHelperProcNames {
		proc, err := dll.FindProc(name)
		if err != nil {
			return nil, fmt.Errorf("resolve core ABI symbol %s: %w", name, err)
		}
		helper.procs[name] = proc
	}
	symbols := ProbeDiagnosticsSymbols{}
	versionProc, err := dll.FindProc(ProbeDiagnosticsVersionSymbol)
	if err == nil {
		symbols.Version = true
		helper.procs[ProbeDiagnosticsVersionSymbol] = versionProc
	}
	queryProc, queryErr := dll.FindProc(ProbeDiagnosticsQuerySymbol)
	if queryErr == nil {
		symbols.Query = true
		helper.procs[ProbeDiagnosticsQuerySymbol] = queryProc
	}
	if !symbols.Version || !symbols.Query {
		return nil, ValidateProbeDiagnosticsExtension(symbols, 0, 0)
	}
	var extensionVersion, extensionStructSize uint32
	hr := helper.call(ProbeDiagnosticsVersionSymbol,
		uintptr(unsafe.Pointer(&extensionVersion)),
		uintptr(unsafe.Pointer(&extensionStructSize)))
	if hr.Failed() {
		return nil, fmt.Errorf("negotiate private probe diagnostics extension: %s", hr.Hex())
	}
	if err := ValidateProbeDiagnosticsExtension(symbols, extensionVersion, extensionStructSize); err != nil {
		return nil, err
	}
	helper.DiagnosticsExtensionVersion = extensionVersion
	for _, name := range []string{
		"CapQualityGetVersion", "CaptureConfigureQuality", "CaptureGetQualityResult",
	} {
		proc, err := dll.FindProc(name)
		if err != nil {
			return nil, fmt.Errorf("resolve capture-quality ABI symbol %s: %w", name, err)
		}
		helper.procs[name] = proc
	}
	var qualityVersion, qualitySize uint32
	qualityHR := helper.call("CapQualityGetVersion",
		uintptr(unsafe.Pointer(&qualityVersion)), uintptr(unsafe.Pointer(&qualitySize)))
	if qualityHR.Failed() || qualityVersion != CaptureQualityNativeVersion || qualitySize != CaptureQualityNativeStructSize {
		return nil, fmt.Errorf(
			"negotiate capture-quality extension: %s version=%d size=%d",
			qualityHR.Hex(), qualityVersion, qualitySize)
	}
	helper.CaptureQualityVersion = qualityVersion
	// Deliberately no FreeLibrary method: the accepted callback/COM lifetime
	// contract keeps this module mapped for the process lifetime.
	return helper, nil
}

func loadHelperModule() (*windows.DLL, LoaderChoice, error) {
	name, err := windows.UTF16PtrFromString(HelperDLLName)
	if err != nil {
		return nil, "", err
	}
	handle, lastError := loadPackagedLibraryFn(name, 0)
	choice, selectErr := SelectLoader(lastError)
	if selectErr != nil {
		return nil, "", selectErr
	}
	dllName := HelperDLLName
	if choice == LoaderExecutableDir {
		executable, err := executablePathFn()
		if err != nil {
			return nil, "", fmt.Errorf("resolve executable path: %w", err)
		}
		absolute, err := filepath.Abs(filepath.Join(filepath.Dir(executable), HelperDLLName))
		if err != nil {
			return nil, "", err
		}
		const flags = windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR | windows.LOAD_LIBRARY_SEARCH_SYSTEM32
		handle, err = loadLibraryExFn(absolute, 0, flags)
		if err != nil {
			return nil, "", fmt.Errorf("LoadLibraryExW(%s): %w", absolute, err)
		}
		dllName = absolute
	}
	dll := &windows.DLL{Name: dllName, Handle: handle}
	return dll, choice, nil
}

func callLoadPackagedLibrary(name *uint16, reserved uint32) (windows.Handle, uint32) {
	r, _, callErr := procLoadPackagedLibrary.Call(uintptr(unsafe.Pointer(name)), uintptr(reserved))
	if r != 0 {
		return windows.Handle(r), 0
	}
	if errno, ok := callErr.(syscall.Errno); ok {
		return 0, uint32(errno)
	}
	return 0, uint32(windows.ERROR_GEN_FAILURE)
}

var coreHelperProcNames = []string{
	"CapGetVersion", "CapInit", "CapDestroy", "CapIsQuiescent",
	"CapPermissionCheck", "CapPermissionRequest", "CapPermissionRequestResult",
	"CapPermissionRequestCancel", "CapPermissionRequestRelease", "CapPermissionSubscribe", "CapPermissionUnsubscribe",
	"CapEnumerateDevices", "CapEnumerateDevicesResult", "CapGetDeviceInfo", "CapEnumerateDevicesCancel", "CapEnumerateDevicesRelease",
	"CapGetDefaultDevice", "CapGetDefaultDeviceResult", "CapGetDefaultDeviceRelease",
	"CapturePrepare", "CaptureActivate", "CaptureGetResult", "CaptureRead", "CaptureRequestStop", "CaptureRelease",
	"PickerOpenFile", "PickerGetResult", "PickerCancel", "PickerRelease",
}

func (h *Helper) call(name string, args ...uintptr) HResult {
	r, _, _ := h.procs[name].Call(args...)
	return HResultFromUintptr(r)
}

func (h *Helper) Version() (uint32, uint32, HResult) {
	var version, size uint32
	hr := h.call("CapGetVersion", uintptr(unsafe.Pointer(&version)), uintptr(unsafe.Pointer(&size)))
	return version, size, hr
}

func (h *Helper) Init() HResult        { return h.call("CapInit") }
func (h *Helper) Destroy() HResult     { return h.call("CapDestroy") }
func (h *Helper) IsQuiescent() HResult { return h.call("CapIsQuiescent") }

func (h *Helper) PermissionCheck() (PermissionStatus, HResult) {
	var status int32
	hr := h.call("CapPermissionCheck", uintptr(unsafe.Pointer(&status)))
	return PermissionStatus(status), hr
}

func (h *Helper) PermissionRequest(event windows.Handle) (uint32, HResult) {
	var id uint32
	hr := h.call("CapPermissionRequest", uintptr(event), uintptr(unsafe.Pointer(&id)))
	return id, hr
}

func (h *Helper) PermissionRequestResult(id uint32) (int32, PermissionStatus, HResult, HResult) {
	var state, status int32
	var outcome HResult
	callHR := h.call("CapPermissionRequestResult", uintptr(id), uintptr(unsafe.Pointer(&state)), uintptr(unsafe.Pointer(&status)), uintptr(unsafe.Pointer(&outcome)))
	return state, PermissionStatus(status), outcome, callHR
}

func (h *Helper) PermissionRequestCancel(id uint32) HResult {
	return h.call("CapPermissionRequestCancel", uintptr(id))
}
func (h *Helper) PermissionRequestRelease(id uint32) HResult {
	return h.call("CapPermissionRequestRelease", uintptr(id))
}
func (h *Helper) PermissionSubscribe(event windows.Handle) HResult {
	return h.call("CapPermissionSubscribe", uintptr(event))
}
func (h *Helper) PermissionUnsubscribe() HResult { return h.call("CapPermissionUnsubscribe") }

func (h *Helper) EnumerateDevices(event windows.Handle) (uint32, HResult) {
	var id uint32
	hr := h.call("CapEnumerateDevices", uintptr(event), uintptr(unsafe.Pointer(&id)))
	return id, hr
}

func (h *Helper) EnumerateDevicesResult(id uint32) (int32, int32, HResult, HResult) {
	var state, count int32
	var outcome HResult
	callHR := h.call("CapEnumerateDevicesResult", uintptr(id), uintptr(unsafe.Pointer(&state)), uintptr(unsafe.Pointer(&count)), uintptr(unsafe.Pointer(&outcome)))
	return state, count, outcome, callHR
}

func (h *Helper) DeviceInfo(id uint32, index int32) (Device, HResult) {
	idBuf := make([]uint16, 512)
	nameBuf := make([]uint16, 512)
	hr := h.call("CapGetDeviceInfo", uintptr(id), uintptr(index), uintptr(unsafe.Pointer(&idBuf[0])), uintptr(len(idBuf)), uintptr(unsafe.Pointer(&nameBuf[0])), uintptr(len(nameBuf)))
	return Device{ID: windows.UTF16ToString(idBuf), Name: windows.UTF16ToString(nameBuf)}, hr
}
func (h *Helper) EnumerateDevicesCancel(id uint32) HResult {
	return h.call("CapEnumerateDevicesCancel", uintptr(id))
}
func (h *Helper) EnumerateDevicesRelease(id uint32) HResult {
	return h.call("CapEnumerateDevicesRelease", uintptr(id))
}

func (h *Helper) DefaultDevice(role int32, event windows.Handle) (uint32, HResult) {
	var id uint32
	hr := h.call("CapGetDefaultDevice", uintptr(role), uintptr(event), uintptr(unsafe.Pointer(&id)))
	return id, hr
}

func (h *Helper) DefaultDeviceResult(id uint32) (int32, string, HResult, HResult) {
	buf := make([]uint16, 512)
	var state, written int32
	var outcome HResult
	callHR := h.call("CapGetDefaultDeviceResult", uintptr(id), uintptr(unsafe.Pointer(&state)), uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&written)), uintptr(unsafe.Pointer(&outcome)))
	return state, windows.UTF16ToString(buf), outcome, callHR
}
func (h *Helper) DefaultDeviceRelease(id uint32) HResult {
	return h.call("CapGetDefaultDeviceRelease", uintptr(id))
}

func (h *Helper) CapturePrepare(event windows.Handle) (uint32, HResult) {
	var id uint32
	hr := h.call("CapturePrepare", uintptr(event), uintptr(unsafe.Pointer(&id)))
	return id, hr
}
func (h *Helper) CaptureConfigureQuality(id uint32, requested bool) HResult {
	var value uintptr
	if requested {
		value = 1
	}
	return h.call("CaptureConfigureQuality", uintptr(id), value)
}

func (h *Helper) CaptureQualityResult(id uint32) (CaptureQualityNative, HResult) {
	result := NewCaptureQualityNative()
	hr := h.call("CaptureGetQualityResult", uintptr(id), uintptr(unsafe.Pointer(&result)))
	return result, hr
}
func (h *Helper) CaptureActivate(id uint32, deviceID string) HResult {
	value, err := windows.UTF16PtrFromString(deviceID)
	if err != nil {
		return HResultFromUintptr(0x8007000d)
	}
	return h.call("CaptureActivate", uintptr(id), uintptr(unsafe.Pointer(value)))
}

func (h *Helper) CaptureResult(id uint32) (CaptureResult, HResult) {
	result := CaptureResult{Format: NewCaptureFormat()}
	var state, reason int32
	callHR := h.call("CaptureGetResult", uintptr(id), uintptr(unsafe.Pointer(&state)), uintptr(unsafe.Pointer(&result.Format)), uintptr(unsafe.Pointer(&result.FramesAvailable)), uintptr(unsafe.Pointer(&result.Outcome)), uintptr(unsafe.Pointer(&reason)))
	result.State = CaptureState(state)
	result.Reason = CaptureReason(reason)
	return result, callHR
}

func (h *Helper) CaptureRead(id uint32, buffer []float32, maxFrames uint32) (uint32, HResult) {
	var frames uint32
	var pointer uintptr
	if len(buffer) != 0 {
		pointer = uintptr(unsafe.Pointer(&buffer[0]))
	}
	hr := h.call("CaptureRead", uintptr(id), pointer, uintptr(maxFrames), uintptr(unsafe.Pointer(&frames)))
	return frames, hr
}
func (h *Helper) CaptureStop(id uint32, reason CaptureReason) HResult {
	return h.call("CaptureRequestStop", uintptr(id), uintptr(reason))
}

func (h *Helper) CaptureDiagnostics(id uint32) (CaptureDiagnostics, HResult) {
	wire := probeCaptureDiagnosticsV1{
		StructSize: ProbeCaptureDiagnosticsV1StructSize,
		Version:    ProbeDiagnosticsExtensionVersion,
	}
	hr := h.call(ProbeDiagnosticsQuerySymbol, uintptr(id), uintptr(unsafe.Pointer(&wire)))
	return CaptureDiagnostics{
		TimestampErrorCount:       wire.TimestampErrorCount,
		CleanupReleaseBufferError: wire.CleanupReleaseBufferError,
		CleanupStopError:          wire.CleanupStopError,
	}, hr
}

func (h *Helper) CaptureRelease(id uint32) HResult { return h.call("CaptureRelease", uintptr(id)) }

func (h *Helper) PickerOpen(hwnd windows.Handle, event windows.Handle) (uint32, HResult) {
	desc, _ := windows.UTF16PtrFromString("Probe audio")
	pattern, _ := windows.UTF16PtrFromString(".wav")
	var id uint32
	hr := h.call("PickerOpenFile", uintptr(hwnd), uintptr(unsafe.Pointer(desc)), uintptr(unsafe.Pointer(pattern)), uintptr(event), uintptr(unsafe.Pointer(&id)))
	return id, hr
}

type PickerResult struct {
	State       int32
	Handle      windows.Handle
	HandleTaken bool
	FileSize    int64
	Name        string
	Outcome     HResult
}

func (h *Helper) PickerResult(id uint32, take bool, nameChars int32) (PickerResult, int32, HResult) {
	result := PickerResult{Handle: windows.InvalidHandle, FileSize: -1}
	var taken, required int32
	var buf []uint16
	var bufPtr uintptr
	if nameChars > 0 {
		buf = make([]uint16, nameChars)
		bufPtr = uintptr(unsafe.Pointer(&buf[0]))
	}
	takeValue := int32(0)
	if take {
		takeValue = 1
	}
	callHR := h.call("PickerGetResult", uintptr(id), uintptr(takeValue), uintptr(unsafe.Pointer(&result.State)), uintptr(unsafe.Pointer(&result.Handle)), uintptr(unsafe.Pointer(&taken)), uintptr(unsafe.Pointer(&result.FileSize)), bufPtr, uintptr(nameChars), uintptr(unsafe.Pointer(&required)), uintptr(unsafe.Pointer(&result.Outcome)))
	result.HandleTaken = taken == 1
	if len(buf) != 0 {
		result.Name = windows.UTF16ToString(buf)
	}
	return result, required, callHR
}
func (h *Helper) PickerCancel(id uint32) HResult  { return h.call("PickerCancel", uintptr(id)) }
func (h *Helper) PickerRelease(id uint32) HResult { return h.call("PickerRelease", uintptr(id)) }
