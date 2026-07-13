#ifndef PULSAR_CAPTURE_INTERNAL_H_
#define PULSAR_CAPTURE_INTERNAL_H_

#include "pulsar_capture.h"

#include <audioclient.h>
#include <ksmedia.h>
#include <mmdeviceapi.h>
#include <stdint.h>
#include <atomic>
#include <functional>
#include <mutex>
#include <memory>
#include <string>
#include <vector>

namespace pulsar_capture {

static_assert(sizeof(CaptureFormat) == 13u * sizeof(uint32_t), "CaptureFormat ABI size changed");

constexpr uint32_t kHelperAbiVersion = 1;
constexpr uint32_t kCaptureFormatVersion = 2;
constexpr uint8_t kPrivateStatePreparing = 0;
constexpr uint8_t kPrivateStatePrepared = 1;
constexpr uint8_t kPrivateStateActivating = 2;
constexpr uint8_t kPrivateStateCapturing = 3;
constexpr uint8_t kPrivateStateStopping = 4;
constexpr uint8_t kPrivateStateSealed = 5;
constexpr uint8_t kPrivateStateTerminal = 6;
constexpr uint32_t kMaxChannels = 8;
constexpr uint32_t kMaxSampleRate = 384000;
constexpr uint32_t kMaxDevices = 256;
constexpr uint32_t kMaxDeviceStringChars = 512;

uint64_t PackState(uint8_t last_public_state, uint8_t state, bool sealed, uint16_t reason);
uint8_t PackedLastPublicState(uint64_t packed);
uint8_t PackedState(uint64_t packed);
bool PackedSealed(uint64_t packed);
uint16_t PackedReason(uint64_t packed);
uint8_t CollapsedPublicStateForPrivate(uint8_t private_state);
int ReasonPriority(int32_t reason);
int32_t HigherPriorityReason(int32_t current, int32_t next);
CapCaptureState PublicStateFromReason(int32_t reason);
HRESULT HResultForReason(int32_t reason, HRESULT fallback);
uint32_t FindAvailableOperationId(uint32_t start, const std::vector<uint32_t>& occupied, uint64_t occupiedCount);
HRESULT DuplicateSignalHandle(HANDLE source, HANDLE* duplicate);
HRESULT ValidateAndFillCaptureFormat(const WAVEFORMATEX* src, CaptureFormat* dst, std::wstring* diagnostic);
HRESULT ConvertFramesToFloat32(const WAVEFORMATEX* src, const CaptureFormat& fmt, const BYTE* input, uint32_t frames, float* output);
HRESULT ComputeCaptureAllocation(uint32_t sample_rate, uint32_t channels, uint32_t buffer_frames,
                                 uint32_t* ring_frames, size_t* scratch_samples);

// CaptureControl is the exact packed-state/wake control used by the production
// CaptureSession. Keeping the activation handoff mutex in the same object lets
// deterministic tests prove that stop publication never acquires it.
struct CaptureControl {
    CaptureControl()
        : packed(PackState(CAP_STATE_PREPARING, kPrivateStatePreparing, false, CAP_REASON_USER_STOP)) {}

    std::atomic<uint64_t> packed;
    std::atomic<HRESULT> wasapi_hresult{S_OK};
    HANDLE capture_thread_wake = nullptr;
    HANDLE stop_event = nullptr;
    std::mutex handoff_mutex;
};

struct PermissionNotificationState {
    virtual ~PermissionNotificationState();
    std::atomic<uintptr_t> notify{0};
    bool countedSubscription = false;
};

void SignalPermissionNotification(const std::shared_ptr<PermissionNotificationState>& state);

bool InstallCaptureReason(CaptureControl* control, int32_t reason, HRESULT hresult, bool internal);
int32_t SealCaptureReason(CaptureControl* control);
HRESULT SealedCaptureHResult(CaptureControl* control, int32_t reason);

struct ActivationCancelPlan {
    bool callbackPublishes = false;
    bool callbackSignals = false;
    bool callbackCloses = true;
    bool captureThreadSignals = true;
    bool captureThreadCloses = true;
};

ActivationCancelPlan PlanActivationCancellation(uint32_t threadDone);

struct PickerResultCore {
    int32_t state = 0;
    HRESULT outcome = S_OK;
    HANDLE fileHandle = INVALID_HANDLE_VALUE;
    bool handleTaken = false;
    int64_t fileSize = -1;
    std::wstring displayName;
};

HRESULT QueryPickerResult(PickerResultCore* result,
                          int32_t takeHandle,
                          int32_t* state,
                          HANDLE* fileHandle,
                          int32_t* handleTaken,
                          int64_t* fileSize,
                          wchar_t* nameBuf,
                          int32_t nameBufLen,
                          int32_t* requiredNameChars,
                          HRESULT* hresult);
void CloseUntakenPickerHandle(PickerResultCore* result);

class FrameRing;

class PacketSource {
public:
    virtual ~PacketSource() = default;
    virtual HRESULT GetNextPacketSize(uint32_t* frames) = 0;
    virtual HRESULT GetBuffer(BYTE** data, uint32_t* frames, DWORD* flags) = 0;
    virtual HRESULT ReleaseBuffer(uint32_t frames) = 0;
};

using StopCheck = bool (*)(void* context);

struct PacketDrainResult {
    uint32_t packetsCommitted = 0;
    int32_t terminalReason = -1;
    HRESULT terminalHResult = S_OK;
    HRESULT cleanupReleaseHResult = S_OK;
    bool stopObserved = false;
    uint32_t timestampErrorCount = 0;
};

enum CaptureCleanupStep : uint32_t {
    CAP_CLEANUP_STOP = 1,
    CAP_CLEANUP_RELEASE_SERVICE = 2,
    CAP_CLEANUP_FREE_MIX_FORMAT = 3,
    CAP_CLEANUP_RELEASE_CLIENT = 4,
};

struct CaptureCleanupState {
    bool started = false;
    bool serviceAcquired = false;
    bool mixFormatOwned = false;
    bool audioClientOwned = false;
};

struct CaptureCleanupDiagnostics {
    HRESULT releaseBufferHResult = S_OK;
    HRESULT stopHResult = S_OK;
    uint32_t steps[4]{};
    uint32_t stepCount = 0;
};

class CaptureCleanupOps {
public:
    virtual ~CaptureCleanupOps() = default;
    virtual HRESULT Stop() = 0;
    virtual void ReleaseService() = 0;
    virtual void FreeMixFormat() = 0;
    virtual void ReleaseClient() = 0;
};

void ExecuteCaptureCleanup(CaptureCleanupState* state, CaptureCleanupOps* ops,
                           CaptureCleanupDiagnostics* diagnostics);
void ConsumePacketCleanupDiagnostic(const PacketDrainResult& result,
                                    CaptureCleanupDiagnostics* diagnostics);

#if defined(PULSAR_CAPTURE_STATIC)
void TestSetPostRoInitFailure(HRESULT hresult);
void TestFailNextDuplicate(HRESULT hresult);
void TestFailNextThreadLaunch();
void TestFailNextActivationLaunch(HRESULT hresult);
void TestSetCaptureCoInitialize(HRESULT hresult, HANDLE entered, HANDLE proceed);
void TestSetPermissionHandlerBarrier(HANDLE entered, HANDLE proceed);
void TestEnablePermissionRegistration(bool enabled);
HRESULT TestDispatchPermissionAccessChanged();
int64_t TestPermissionTokenValue();
uint32_t TestPermissionRevokeCount();
void TestDeferNextActivationCallback();
HRESULT TestCompleteDeferredActivation(uint32_t opId, HRESULT activateHResult);
void TestSetActivationCallbackBarrier(HANDLE entered, HANDLE proceed);
void TestSetCaptureAfterWakeBarrier(HANDLE entered, HANDLE proceed);
uint32_t TestCaptureThreadDone(uint32_t opId);
void TestGetNotificationCounts(uint32_t* captureSignals, uint32_t* captureCloses,
                               uint32_t* callbackSignals, uint32_t* callbackCloses);
HRESULT TestRouteCaptureDiagnostics(uint32_t opId, const PacketDrainResult* packet,
                                    HRESULT stopHResult);
HRESULT TestHoldCaptureHandoff(uint32_t opId, HANDLE entered, HANDLE proceed);
uint32_t TestRoUninitializeCount();
void TestResetNativeHooks();
#endif

HRESULT DrainCapturePackets(PacketSource* source,
                            const CaptureFormat& format,
                            FrameRing* ring,
                            std::vector<float>* scratch,
                            bool* firstPacket,
                            StopCheck stopCheck,
                            void* stopContext,
                            PacketDrainResult* result);

class FrameRing {
public:
    FrameRing();

    HRESULT Reset(uint32_t channels, uint32_t capacity_frames);
    bool HasSpaceFor(uint32_t frames) const;
    HRESULT Write(const float* src, uint32_t frames);
    uint32_t Read(float* dst, uint32_t max_frames);
    uint32_t Available() const;
    uint32_t Capacity() const;
    uint32_t Channels() const;

private:
    uint32_t channels_;
    uint32_t capacity_frames_;
    std::atomic<uint64_t> read_frame_;
    std::atomic<uint64_t> write_frame_;
    std::vector<float> data_;
};

}  // namespace pulsar_capture

#endif
