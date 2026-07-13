package winprobe

import (
	"encoding/hex"
	"fmt"
)

const (
	HelperABIVersion                    = 1
	CaptureFormatVersion                = 2
	CaptureFormatStructSize             = 13 * 4
	ProbeDiagnosticsExtensionVersion    = 1
	ProbeCaptureDiagnosticsV1StructSize = 5 * 4
)

type HResult int32

func HResultFromUintptr(v uintptr) HResult {
	return HResult(int32(v))
}

func (hr HResult) Succeeded() bool { return hr >= 0 }

func (hr HResult) Failed() bool { return hr < 0 }

func (hr HResult) Hex() string {
	var raw [4]byte
	u := uint32(hr)
	raw[0] = byte(u >> 24)
	raw[1] = byte(u >> 16)
	raw[2] = byte(u >> 8)
	raw[3] = byte(u)
	return "0x" + hex.EncodeToString(raw[:])
}

func (hr HResult) String() string {
	return hr.Hex()
}

func (hr HResult) Error() string {
	return hr.Hex()
}

type PermissionStatus int32

const (
	PermissionUnavailable    PermissionStatus = -1
	PermissionDeniedByUser   PermissionStatus = 0
	PermissionAllowed        PermissionStatus = 1
	PermissionPromptRequired PermissionStatus = 2
	PermissionDeniedBySystem PermissionStatus = 3
	PermissionNotDeclared    PermissionStatus = 4
	PermissionUnknown        PermissionStatus = 5
)

func (s PermissionStatus) String() string {
	switch s {
	case PermissionUnavailable:
		return "unavailable"
	case PermissionDeniedByUser:
		return "denied_by_user"
	case PermissionAllowed:
		return "allowed"
	case PermissionPromptRequired:
		return "prompt_required"
	case PermissionDeniedBySystem:
		return "denied_by_system"
	case PermissionNotDeclared:
		return "not_declared"
	case PermissionUnknown:
		return "unknown"
	default:
		return fmt.Sprintf("permission_%d", int32(s))
	}
}

type CaptureReason int32

const (
	ReasonUserStop CaptureReason = iota
	ReasonPermissionRevoke
	ReasonDeviceLost
	ReasonShutdown
	ReasonSuspend
	ReasonLock
	ReasonCancel
	ReasonOverflow
	ReasonWasapiError
	ReasonFormatError
	ReasonDiscontinuity
)

func (r CaptureReason) String() string {
	switch r {
	case ReasonUserStop:
		return "user_stop"
	case ReasonPermissionRevoke:
		return "permission_revoke"
	case ReasonDeviceLost:
		return "device_lost"
	case ReasonShutdown:
		return "shutdown"
	case ReasonSuspend:
		return "suspend"
	case ReasonLock:
		return "lock"
	case ReasonCancel:
		return "cancel"
	case ReasonOverflow:
		return "overflow"
	case ReasonWasapiError:
		return "wasapi_error"
	case ReasonFormatError:
		return "format_error"
	case ReasonDiscontinuity:
		return "discontinuity"
	default:
		return fmt.Sprintf("reason_%d", int32(r))
	}
}

func reasonPriority(reason CaptureReason) int {
	switch reason {
	case ReasonOverflow:
		return 1
	case ReasonDiscontinuity:
		return 2
	case ReasonPermissionRevoke:
		return 3
	case ReasonWasapiError, ReasonFormatError:
		return 4
	case ReasonDeviceLost:
		return 5
	case ReasonShutdown:
		return 6
	case ReasonSuspend:
		return 7
	case ReasonLock:
		return 8
	case ReasonCancel:
		return 9
	case ReasonUserStop:
		return 10
	default:
		return 100
	}
}

func HigherPriorityReason(current, next CaptureReason) CaptureReason {
	if reasonPriority(next) < reasonPriority(current) {
		return next
	}
	return current
}

type CaptureState int32

const (
	CaptureStatePreparing CaptureState = iota
	CaptureStateActivating
	CaptureStateCapturing
	CaptureStateStopped
	CaptureStateFailed
	CaptureStateCancelled
)

func PublicCaptureState(reason CaptureReason) CaptureState {
	switch reason {
	case ReasonCancel:
		return CaptureStateCancelled
	case ReasonUserStop, ReasonDeviceLost, ReasonShutdown, ReasonSuspend, ReasonLock:
		return CaptureStateStopped
	default:
		return CaptureStateFailed
	}
}

type CaptureFormat struct {
	StructSize      uint32
	Version         uint32
	Ready           uint32
	Valid           uint32
	SampleRate      uint32
	Channels        uint32
	BitsPerSample   uint32
	ValidBits       uint32
	ChannelMask     uint32
	NativeSubtype   uint32
	NativeBits      uint32
	NativeValidBits uint32
	BlockAlign      uint32
}

type CaptureResult struct {
	State           CaptureState
	Format          CaptureFormat
	FramesAvailable uint32
	Outcome         HResult
	Reason          CaptureReason
}

type CaptureDiagnostics struct {
	TimestampErrorCount       uint32
	CleanupReleaseBufferError HResult
	CleanupStopError          HResult
}

// probeCaptureDiagnosticsV1 is private probe evidence wire data. It is not a
// member of the frozen Rev16 core ABI and has its own version/size handshake.
type probeCaptureDiagnosticsV1 struct {
	StructSize                uint32
	Version                   uint32
	TimestampErrorCount       uint32
	CleanupReleaseBufferError HResult
	CleanupStopError          HResult
}

type Device struct {
	ID   string
	Name string
}

func NewCaptureFormat() CaptureFormat {
	return CaptureFormat{
		StructSize: CaptureFormatStructSize,
		Version:    CaptureFormatVersion,
	}
}

type ProbeScenario string

const (
	ScenarioPermission ProbeScenario = "permission"
	ScenarioCapture    ProbeScenario = "capture"
	ScenarioHotkey     ProbeScenario = "hotkey"
	ScenarioPicker     ProbeScenario = "picker"
	ScenarioWindow     ProbeScenario = "window"
)

type ProbeResult string

const (
	ResultAttempt ProbeResult = "attempt"
	ResultPass    ProbeResult = "pass"
	ResultFail    ProbeResult = "fail"
	ResultBlocked ProbeResult = "blocked"
	ResultDiscard ProbeResult = "discard"
)
