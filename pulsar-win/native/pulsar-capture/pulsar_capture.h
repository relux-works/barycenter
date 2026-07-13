#ifndef PULSAR_CAPTURE_H_
#define PULSAR_CAPTURE_H_

#include <windows.h>
#include <stdint.h>

#if defined(PULSAR_CAPTURE_STATIC)
#define PULSAR_CAPTURE_API
#elif defined(PULSAR_CAPTURE_EXPORTS)
#define PULSAR_CAPTURE_API __declspec(dllexport)
#else
#define PULSAR_CAPTURE_API __declspec(dllimport)
#endif

enum CapPermissionStatus {
    CAP_PERMISSION_UNAVAILABLE = -1,
    CAP_PERMISSION_DENIED_BY_USER = 0,
    CAP_PERMISSION_ALLOWED = 1,
    CAP_PERMISSION_PROMPT_REQUIRED = 2,
    CAP_PERMISSION_DENIED_BY_SYSTEM = 3,
    CAP_PERMISSION_NOT_DECLARED = 4,
    CAP_PERMISSION_UNKNOWN = 5,
};

enum CapCaptureReason {
    CAP_REASON_USER_STOP = 0,
    CAP_REASON_PERMISSION_REVOKE = 1,
    CAP_REASON_DEVICE_LOST = 2,
    CAP_REASON_SHUTDOWN = 3,
    CAP_REASON_SUSPEND = 4,
    CAP_REASON_LOCK = 5,
    CAP_REASON_CANCEL = 6,
    CAP_REASON_OVERFLOW = 7,
    CAP_REASON_WASAPI_ERROR = 8,
    CAP_REASON_FORMAT_ERROR = 9,
    CAP_REASON_DISCONTINUITY = 10,
};

enum CapCaptureState {
    CAP_STATE_PREPARING = 0,
    CAP_STATE_ACTIVATING = 1,
    CAP_STATE_CAPTURING = 2,
    CAP_STATE_STOPPED = 3,
    CAP_STATE_FAILED = 4,
    CAP_STATE_CANCELLED = 5,
};

typedef struct CaptureFormat {
    uint32_t structSize;
    uint32_t version;
    uint32_t ready;
    uint32_t valid;
    uint32_t sampleRate;
    uint32_t channels;
    uint32_t bitsPerSample;
    uint32_t validBits;
    uint32_t channelMask;
    uint32_t nativeSubtype;
    uint32_t nativeBits;
    uint32_t nativeValidBits;
    uint32_t nBlockAlign;
} CaptureFormat;

extern "C" {

PULSAR_CAPTURE_API HRESULT __stdcall CapGetVersion(uint32_t* version, uint32_t* structHeaderSize);

PULSAR_CAPTURE_API HRESULT __stdcall CapPermissionCheck(int32_t* status);
PULSAR_CAPTURE_API HRESULT __stdcall CapPermissionRequest(HANDLE notifyEvent, uint32_t* opId);
PULSAR_CAPTURE_API HRESULT __stdcall CapPermissionRequestResult(uint32_t opId, int32_t* state, int32_t* status, HRESULT* hresult);
PULSAR_CAPTURE_API HRESULT __stdcall CapPermissionRequestCancel(uint32_t opId);
PULSAR_CAPTURE_API HRESULT __stdcall CapPermissionRequestRelease(uint32_t opId);
PULSAR_CAPTURE_API HRESULT __stdcall CapPermissionSubscribe(HANDLE notifyEvent);
PULSAR_CAPTURE_API HRESULT __stdcall CapPermissionUnsubscribe(void);

PULSAR_CAPTURE_API HRESULT __stdcall CapEnumerateDevices(HANDLE notifyEvent, uint32_t* opId);
PULSAR_CAPTURE_API HRESULT __stdcall CapEnumerateDevicesResult(uint32_t opId, int32_t* state, int32_t* count, HRESULT* hresult);
PULSAR_CAPTURE_API HRESULT __stdcall CapGetDeviceInfo(uint32_t opId, int32_t index, wchar_t* idBuf, int32_t idBufLen, wchar_t* nameBuf, int32_t nameBufLen);
PULSAR_CAPTURE_API HRESULT __stdcall CapEnumerateDevicesCancel(uint32_t opId);
PULSAR_CAPTURE_API HRESULT __stdcall CapEnumerateDevicesRelease(uint32_t opId);

PULSAR_CAPTURE_API HRESULT __stdcall CapGetDefaultDevice(int32_t role, HANDLE notifyEvent, uint32_t* opId);
PULSAR_CAPTURE_API HRESULT __stdcall CapGetDefaultDeviceResult(uint32_t opId, int32_t* state, wchar_t* buf, int32_t bufLen, int32_t* written, HRESULT* hresult);
PULSAR_CAPTURE_API HRESULT __stdcall CapGetDefaultDeviceRelease(uint32_t opId);

PULSAR_CAPTURE_API HRESULT __stdcall CapturePrepare(HANDLE notifyEvent, uint32_t* opId);
PULSAR_CAPTURE_API HRESULT __stdcall CaptureActivate(uint32_t opId, const wchar_t* deviceId);
PULSAR_CAPTURE_API HRESULT __stdcall CaptureGetResult(uint32_t opId, int32_t* state, CaptureFormat* format, uint32_t* framesAvailable, HRESULT* hresult, int32_t* terminalReason);
PULSAR_CAPTURE_API HRESULT __stdcall CaptureRead(uint32_t opId, float* buf, uint32_t maxFrames, uint32_t* framesRead);
PULSAR_CAPTURE_API HRESULT __stdcall CaptureRequestStop(uint32_t opId, int32_t reason);
PULSAR_CAPTURE_API HRESULT __stdcall CaptureRelease(uint32_t opId);

PULSAR_CAPTURE_API HRESULT __stdcall PickerOpenFile(HWND hwnd, const wchar_t* filterDesc, const wchar_t* filterPattern, HANDLE notifyEvent, uint32_t* opId);
PULSAR_CAPTURE_API HRESULT __stdcall PickerGetResult(uint32_t opId, int32_t takeHandle, int32_t* state, HANDLE* fileHandle, int32_t* handleTaken, int64_t* fileSize, wchar_t* nameBuf, int32_t nameBufLen, int32_t* requiredNameChars, HRESULT* hresult);
PULSAR_CAPTURE_API HRESULT __stdcall PickerCancel(uint32_t opId);
PULSAR_CAPTURE_API HRESULT __stdcall PickerRelease(uint32_t opId);

PULSAR_CAPTURE_API HRESULT __stdcall CapInit(void);
PULSAR_CAPTURE_API HRESULT __stdcall CapIsQuiescent(void);
PULSAR_CAPTURE_API HRESULT __stdcall CapDestroy(void);

}

#endif
