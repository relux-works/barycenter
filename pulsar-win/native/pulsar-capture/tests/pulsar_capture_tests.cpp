#include "../pulsar_capture_internal.h"
#include "../pulsar_probe_diagnostics.h"

#include <cmath>
#include <chrono>
#include <cstdint>
#include <cstring>
#include <functional>
#include <future>
#include <iostream>
#include <memory>
#include <string>
#include <thread>
#include <vector>

using namespace pulsar_capture;

namespace {

int failures = 0;

void check(bool condition, const char* expression, const char* file, int line) {
    if (condition) return;
    ++failures;
    std::cerr << file << ':' << line << ": CHECK failed: " << expression << '\n';
}

#define CHECK(expr) check((expr), #expr, __FILE__, __LINE__)

CaptureFormat make_format(uint32_t subtype, uint32_t container_bits, uint32_t valid_bits) {
    CaptureFormat format{};
    format.structSize = sizeof(format);
    format.version = kCaptureFormatVersion;
    format.ready = 1;
    format.valid = 1;
    format.sampleRate = 48000;
    format.channels = 1;
    format.bitsPerSample = 32;
    format.validBits = valid_bits;
    format.nativeSubtype = subtype;
    format.nativeBits = container_bits;
    format.nativeValidBits = valid_bits;
    format.nBlockAlign = container_bits / 8;
    return format;
}

void check_close(float got, float want, float tolerance = 0.0f) {
    CHECK(std::fabs(got - want) <= tolerance);
}

void test_version_and_null_contract() {
    uint32_t version = 0;
    uint32_t size = 0;
    CHECK(CapGetVersion(&version, &size) == S_OK);
    CHECK(version == 1);
    CHECK(size == sizeof(CaptureFormat));
    CHECK(CapGetVersion(nullptr, &size) == E_POINTER);
    CHECK(CapGetVersion(&version, nullptr) == E_POINTER);
    CHECK(sizeof(CaptureFormat) == 13u * sizeof(uint32_t));
    uint32_t diagnostics_version = 0;
    uint32_t diagnostics_size = 0;
    CHECK(PulsarProbeDiagnosticsGetVersion(&diagnostics_version, &diagnostics_size) == S_OK);
    CHECK(diagnostics_version == PULSAR_PROBE_DIAGNOSTICS_EXTENSION_V1);
    CHECK(diagnostics_size == sizeof(PulsarProbeCaptureDiagnosticsV1));
    CHECK(PulsarProbeDiagnosticsGetVersion(nullptr, &diagnostics_size) == E_POINTER);
    CHECK(PulsarProbeDiagnosticsGetVersion(&diagnostics_version, nullptr) == E_POINTER);
    CHECK(version == 1); // Core CapGetVersion did not negotiate the private extension.
    CHECK(CapturePrepare(nullptr, nullptr) == E_POINTER);
    CHECK(PickerGetResult(0, 0, nullptr, nullptr, nullptr, nullptr, nullptr, 0, nullptr, nullptr) == E_HANDLE);
    PulsarProbeCaptureDiagnosticsV1 diagnostics{};
    diagnostics.structSize = sizeof(diagnostics);
    diagnostics.version = PULSAR_PROBE_DIAGNOSTICS_EXTENSION_V1;
    CHECK(PulsarProbeCaptureGetDiagnosticsV1(0, &diagnostics) == E_HANDLE);
    CHECK(PulsarProbeCaptureGetDiagnosticsV1(0, nullptr) == E_POINTER);
    diagnostics.structSize--;
    CHECK(PulsarProbeCaptureGetDiagnosticsV1(0, &diagnostics) == E_INVALIDARG);
    diagnostics.structSize = sizeof(diagnostics);
    diagnostics.version++;
    CHECK(PulsarProbeCaptureGetDiagnosticsV1(0, &diagnostics) == E_INVALIDARG);
}

void test_packed_fsm_and_priority() {
    const uint64_t packed = PackState(CAP_STATE_CAPTURING, kPrivateStateStopping, false, CAP_REASON_USER_STOP);
    CHECK(PackedLastPublicState(packed) == CAP_STATE_CAPTURING);
    CHECK(PackedState(packed) == kPrivateStateStopping);
    CHECK(!PackedSealed(packed));
    CHECK(PackedReason(packed) == CAP_REASON_USER_STOP);
    const uint64_t sealed = PackState(CAP_STATE_CAPTURING, kPrivateStateSealed, true, CAP_REASON_OVERFLOW);
    CHECK(PackedSealed(sealed));
    CHECK(PackedReason(sealed) == CAP_REASON_OVERFLOW);
    CHECK(HigherPriorityReason(CAP_REASON_USER_STOP, CAP_REASON_PERMISSION_REVOKE) == CAP_REASON_PERMISSION_REVOKE);
    CHECK(HigherPriorityReason(CAP_REASON_PERMISSION_REVOKE, CAP_REASON_OVERFLOW) == CAP_REASON_OVERFLOW);
    CHECK(HigherPriorityReason(CAP_REASON_WASAPI_ERROR, CAP_REASON_FORMAT_ERROR) == CAP_REASON_WASAPI_ERROR);
    CHECK(PublicStateFromReason(CAP_REASON_CANCEL) == CAP_STATE_CANCELLED);
    CHECK(PublicStateFromReason(CAP_REASON_DEVICE_LOST) == CAP_STATE_STOPPED);
    CHECK(PublicStateFromReason(CAP_REASON_DISCONTINUITY) == CAP_STATE_FAILED);
    CHECK(HResultForReason(CAP_REASON_CANCEL, S_OK) == HRESULT_FROM_WIN32(ERROR_CANCELLED));
    CHECK(HResultForReason(CAP_REASON_OVERFLOW, S_OK) == HRESULT_FROM_WIN32(ERROR_BUFFER_OVERFLOW));
    CHECK(FindAvailableOperationId(UINT32_MAX, {UINT32_MAX}, 1) == 1);
    CHECK(FindAvailableOperationId(UINT32_MAX, {UINT32_MAX, 1}, 2) == 2);
    CHECK(FindAvailableOperationId(0, {}, 0) == 1);
    CHECK(FindAvailableOperationId(1, {}, UINT32_MAX) == 0);
}

void test_stop_is_nonblocking_while_handoff_mutex_is_stalled() {
    CaptureControl control;
    control.capture_thread_wake = CreateEventW(nullptr, FALSE, FALSE, nullptr);
    control.stop_event = CreateEventW(nullptr, TRUE, FALSE, nullptr);
    CHECK(control.capture_thread_wake != nullptr && control.stop_event != nullptr);
    control.packed.store(PackState(CAP_STATE_PREPARING, kPrivateStatePrepared, false,
                                   CAP_REASON_USER_STOP), std::memory_order_release);

    std::unique_lock<std::mutex> stalled_handoff(control.handoff_mutex);
    auto request = std::async(std::launch::async, [&]() {
        return InstallCaptureReason(&control, CAP_REASON_CANCEL,
                                    HRESULT_FROM_WIN32(ERROR_CANCELLED), false);
    });
    CHECK(request.wait_for(std::chrono::milliseconds(100)) == std::future_status::ready);
    CHECK(request.get());
    CHECK(PackedState(control.packed.load(std::memory_order_acquire)) == kPrivateStateStopping);
    CHECK(PackedReason(control.packed.load(std::memory_order_acquire)) == CAP_REASON_CANCEL);
    CHECK(WaitForSingleObject(control.capture_thread_wake, 0) == WAIT_OBJECT_0);
    stalled_handoff.unlock();
    CloseHandle(control.capture_thread_wake);
    CloseHandle(control.stop_event);
}

void test_priority_and_seal_races_on_production_control() {
    for (int iteration = 0; iteration < 50; ++iteration) {
        CaptureControl control;
        control.packed.store(PackState(CAP_STATE_CAPTURING, kPrivateStateCapturing, false,
                                       CAP_REASON_USER_STOP), std::memory_order_release);
        std::promise<void> ready;
        auto gate = ready.get_future().share();
        std::thread user([&]() { gate.wait(); InstallCaptureReason(&control, CAP_REASON_USER_STOP, S_OK, true); });
        std::thread revoke([&]() { gate.wait(); InstallCaptureReason(&control, CAP_REASON_PERMISSION_REVOKE, E_ACCESSDENIED, true); });
        ready.set_value();
        user.join();
        revoke.join();
        CHECK(SealCaptureReason(&control) == CAP_REASON_PERMISSION_REVOKE);
        CHECK(SealedCaptureHResult(&control, CAP_REASON_PERMISSION_REVOKE) == E_ACCESSDENIED);
        CHECK(!InstallCaptureReason(&control, CAP_REASON_OVERFLOW,
                                    HRESULT_FROM_WIN32(ERROR_BUFFER_OVERFLOW), true));
        CHECK(PackedReason(control.packed.load(std::memory_order_acquire)) == CAP_REASON_PERMISSION_REVOKE);
    }
}

void test_format_validation() {
    WAVEFORMATEX pcm16{};
    pcm16.wFormatTag = WAVE_FORMAT_PCM;
    pcm16.nChannels = 2;
    pcm16.nSamplesPerSec = 48000;
    pcm16.wBitsPerSample = 16;
    pcm16.nBlockAlign = 4;
    CaptureFormat out{};
    std::wstring diagnostic;
    CHECK(ValidateAndFillCaptureFormat(&pcm16, &out, &diagnostic) == S_OK);
    CHECK(out.nativeSubtype == 1);
    CHECK(out.nativeBits == 16);
    CHECK(out.channels == 2);

    WAVEFORMATEXTENSIBLE pcm24in32{};
    pcm24in32.Format.wFormatTag = WAVE_FORMAT_EXTENSIBLE;
    pcm24in32.Format.cbSize = 22;
    pcm24in32.Format.nChannels = 1;
    pcm24in32.Format.nSamplesPerSec = 48000;
    pcm24in32.Format.wBitsPerSample = 32;
    pcm24in32.Format.nBlockAlign = 4;
    pcm24in32.Samples.wValidBitsPerSample = 24;
    pcm24in32.SubFormat = KSDATAFORMAT_SUBTYPE_PCM;
    CHECK(ValidateAndFillCaptureFormat(&pcm24in32.Format, &out, &diagnostic) == S_OK);
    CHECK(out.nativeBits == 32);
    CHECK(out.nativeValidBits == 24);

    pcm24in32.Samples.wValidBitsPerSample = 33;
    CHECK(ValidateAndFillCaptureFormat(&pcm24in32.Format, &out, &diagnostic) == E_INVALIDARG);
    pcm24in32.Samples.wValidBitsPerSample = 20;
    CHECK(ValidateAndFillCaptureFormat(&pcm24in32.Format, &out, &diagnostic) == E_INVALIDARG);
    pcm24in32.Samples.wValidBitsPerSample = 24;
    pcm24in32.Format.nChannels = 9;
    CHECK(ValidateAndFillCaptureFormat(&pcm24in32.Format, &out, &diagnostic) == E_INVALIDARG);

    WAVEFORMATEX unsupported{};
    unsupported.wFormatTag = WAVE_FORMAT_IEEE_FLOAT;
    unsupported.nChannels = 1;
    unsupported.nSamplesPerSec = 48000;
    unsupported.wBitsPerSample = 64;
    unsupported.nBlockAlign = 8;
    CHECK(ValidateAndFillCaptureFormat(&unsupported, &out, &diagnostic) == E_INVALIDARG);
    unsupported.wFormatTag = WAVE_FORMAT_PCM;
    unsupported.wBitsPerSample = 8;
    unsupported.nBlockAlign = 1;
    CHECK(ValidateAndFillCaptureFormat(&unsupported, &out, &diagnostic) == E_INVALIDARG);
}

void test_checked_capture_allocations() {
    uint32_t ring_frames = 0;
    size_t scratch_samples = 0;
    CHECK(ComputeCaptureAllocation(8000, 1, 65536, &ring_frames, &scratch_samples) == S_OK);
    CHECK(ring_frames == 65536 && scratch_samples == 65536);
    CHECK(ComputeCaptureAllocation(384000, 8, 65536, &ring_frames, &scratch_samples) == S_OK);
    CHECK(ring_frames == 768000 && scratch_samples == 524288);
    CHECK(ComputeCaptureAllocation(384001, 1, 1, &ring_frames, &scratch_samples) == E_INVALIDARG);
    CHECK(ComputeCaptureAllocation(48000, 9, 1, &ring_frames, &scratch_samples) == E_INVALIDARG);
    CHECK(ComputeCaptureAllocation(48000, 1, 65537, &ring_frames, &scratch_samples) == E_INVALIDARG);
    CHECK(ComputeCaptureAllocation(48000, 1, 1, nullptr, &scratch_samples) == E_POINTER);
}

void test_float_and_pcm_vectors_unaligned() {
    alignas(4) uint8_t storage[96]{};
    uint8_t* data = storage + 1; // deliberately unaligned
    float output[8]{};

    const uint32_t float_bits[] = {0x00000000u, 0x3f800000u, 0xbf800000u, 0x3f000000u};
    std::memcpy(data, float_bits, sizeof(float_bits));
    CHECK(ConvertFramesToFloat32(nullptr, make_format(3, 32, 32), data, 4, output) == S_OK);
    check_close(output[0], 0.0f);
    check_close(output[1], 1.0f);
    check_close(output[2], -1.0f);
    check_close(output[3], 0.5f);

    const int16_t pcm16[] = {0, 32767, INT16_MIN, 1, -1, 16384};
    std::memcpy(data, pcm16, sizeof(pcm16));
    CHECK(ConvertFramesToFloat32(nullptr, make_format(1, 16, 16), data, 6, output) == S_OK);
    check_close(output[0], 0.0f);
    check_close(output[1], 32767.0f / 32768.0f);
    check_close(output[2], -1.0f);
    check_close(output[3], 1.0f / 32768.0f);
    check_close(output[4], -1.0f / 32768.0f);
    check_close(output[5], 0.5f);

    const uint8_t pcm24[] = {
        0x00, 0x00, 0x00, 0xff, 0xff, 0x7f, 0x00, 0x00, 0x80,
        0x01, 0x00, 0x00, 0xff, 0xff, 0xff, 0x00, 0x00, 0x40,
    };
    std::memcpy(data, pcm24, sizeof(pcm24));
    CHECK(ConvertFramesToFloat32(nullptr, make_format(1, 24, 24), data, 6, output) == S_OK);
    check_close(output[0], 0.0f);
    check_close(output[1], 8388607.0f / 8388608.0f);
    check_close(output[2], -1.0f);
    check_close(output[3], 1.0f / 8388608.0f);
    check_close(output[4], -1.0f / 8388608.0f);
    check_close(output[5], 0.5f);

    const uint32_t pcm24in32[] = {0x00000000u, 0x7fffff00u, 0x80000000u, 0x00000100u, 0xffffff00u, 0x40000000u};
    std::memcpy(data, pcm24in32, sizeof(pcm24in32));
    CHECK(ConvertFramesToFloat32(nullptr, make_format(1, 32, 24), data, 6, output) == S_OK);
    check_close(output[0], 0.0f);
    check_close(output[1], 8388607.0f / 8388608.0f);
    check_close(output[2], -1.0f);
    check_close(output[3], 1.0f / 8388608.0f);
    check_close(output[4], -1.0f / 8388608.0f);
    check_close(output[5], 0.5f);

    const int32_t pcm32[] = {0, INT32_MAX, INT32_MIN, 1, -1, 1073741824};
    std::memcpy(data, pcm32, sizeof(pcm32));
    CHECK(ConvertFramesToFloat32(nullptr, make_format(1, 32, 32), data, 6, output) == S_OK);
    check_close(output[0], 0.0f);
    check_close(output[1], 1.0f);
    check_close(output[2], -1.0f);
    check_close(output[3], 1.0f / 2147483648.0f);
    check_close(output[4], -1.0f / 2147483648.0f);
    check_close(output[5], 0.5f);
}

void test_ring_exact_fit_overflow_and_drain() {
    FrameRing ring;
    CHECK(ring.Reset(2, 3) == S_OK);
    const float frames[] = {1, 2, 3, 4, 5, 6};
    CHECK(ring.HasSpaceFor(3));
    CHECK(ring.Write(frames, 3) == S_OK);
    CHECK(!ring.HasSpaceFor(1));
    CHECK(ring.Write(frames, 1) == HRESULT_FROM_WIN32(ERROR_BUFFER_OVERFLOW));
    float read[6]{};
    CHECK(ring.Read(read, 2) == 2);
    CHECK(read[0] == 1 && read[1] == 2 && read[2] == 3 && read[3] == 4);
    CHECK(ring.Write(frames, 2) == S_OK);
    CHECK(ring.Read(read, 3) == 3);
    CHECK(ring.Available() == 0);
}

void test_notification_duplicate_and_callback_release_fence() {
    HANDLE original = CreateEventW(nullptr, FALSE, FALSE, nullptr);
    CHECK(original != nullptr);
    HANDLE duplicate = nullptr;
    CHECK(DuplicateSignalHandle(original, &duplicate) == S_OK);
    CHECK(duplicate != nullptr);
    CHECK(CloseHandle(original) != FALSE); // unsubscribe/release can close original immediately
    CHECK(SetEvent(duplicate) != FALSE);
    CHECK(CloseHandle(duplicate) != FALSE);

    struct FenceState {
        explicit FenceState(bool* value) : destroyed(value) {}
        bool* destroyed;
        ~FenceState() { *destroyed = true; }
    };
    bool destroyed = false;
    auto registry_ref = std::make_shared<FenceState>(&destroyed);
    auto callback_ref = registry_ref;
    registry_ref.reset(); // release export drops only registry ownership
    CHECK(!destroyed);
    callback_ref.reset(); // callback epilogue drops the completion fence
    CHECK(destroyed);
}

void test_unsubscribe_with_inflight_production_handler() {
    HANDLE original = CreateEventW(nullptr, TRUE, FALSE, nullptr);
    CHECK(original != nullptr);
    HANDLE observer_duplicate = nullptr;
    CHECK(DuplicateSignalHandle(original, &observer_duplicate) == S_OK);

    HANDLE entered = CreateEventW(nullptr, TRUE, FALSE, nullptr);
    HANDLE proceed = CreateEventW(nullptr, TRUE, FALSE, nullptr);
    CHECK(entered != nullptr && proceed != nullptr);
    TestEnablePermissionRegistration(true);
    TestSetPermissionHandlerBarrier(entered, proceed);
    CHECK(CapPermissionSubscribe(original) == S_OK);
    CHECK(TestPermissionTokenValue() != 0);
    CHECK(CapIsQuiescent() == S_FALSE); // the registered subscription owns its duplicate
    auto handler = std::async(std::launch::async, []() {
        return TestDispatchPermissionAccessChanged();
    });
    CHECK(WaitForSingleObject(entered, 1000) == WAIT_OBJECT_0);
    CHECK(CapPermissionUnsubscribe() == S_OK); // real token/registry export path
    CHECK(TestPermissionTokenValue() == 0);
    CHECK(TestPermissionRevokeCount() == 1);
    CHECK(CloseHandle(original) != FALSE); // Go may close its original immediately
    CHECK(CapIsQuiescent() == S_FALSE);
    CHECK(SetEvent(proceed) != FALSE);
    CHECK(handler.get() == S_OK);
    CHECK(WaitForSingleObject(observer_duplicate, 1000) == WAIT_OBJECT_0);
    CHECK(CapIsQuiescent() == S_OK);
    TestSetPermissionHandlerBarrier(nullptr, nullptr);
    TestEnablePermissionRegistration(false);
    CloseHandle(observer_duplicate);
    CloseHandle(entered);
    CloseHandle(proceed);
}

void test_picker_truth_table_and_handle_ownership() {
    CHECK(QueryPickerResult(nullptr, 0, nullptr, nullptr, nullptr, nullptr,
                            nullptr, 0, nullptr, nullptr) == E_POINTER);
    PickerResultCore pending;
    int32_t state = -1;
    int32_t taken = -1;
    HRESULT outcome = E_FAIL;
    HANDLE handle = INVALID_HANDLE_VALUE;
    CHECK(QueryPickerResult(&pending, 0, &state, &handle, &taken, nullptr, nullptr, 0, nullptr, &outcome) == S_FALSE);
    CHECK(state == 0);
    taken = 77;
    outcome = E_ABORT;
    handle = reinterpret_cast<HANDLE>(static_cast<uintptr_t>(7));
    CHECK(QueryPickerResult(&pending, 0, &state, &handle, &taken, nullptr, nullptr, 0, nullptr, &outcome) == S_FALSE);
    CHECK(state == 0 && taken == 77 && outcome == E_ABORT && handle == reinterpret_cast<HANDLE>(static_cast<uintptr_t>(7)));

    PickerResultCore picked;
    picked.state = 1;
    picked.outcome = S_OK;
    picked.fileHandle = CreateEventW(nullptr, FALSE, FALSE, nullptr);
    picked.fileSize = 7;
    picked.displayName = L"a-very-long-name.wav";
    CHECK(picked.fileHandle != nullptr);
    HANDLE helper_owned = picked.fileHandle;

    int64_t size = -1;
    int32_t required = 0;
    wchar_t short_name[5] = {};
    CHECK(QueryPickerResult(&picked, 0, &state, &handle, &taken, &size,
                            short_name, 5, &required, &outcome) == S_OK);
    CHECK(state == 1 && outcome == S_OK && taken == 0);
    CHECK(handle == INVALID_HANDLE_VALUE);
    CHECK(size == 7);
    CHECK(required == static_cast<int32_t>(picked.displayName.size() + 1));
    CHECK(short_name[4] == L'\0');
    DWORD flags = 0;
    CHECK(GetHandleInformation(helper_owned, &flags) != FALSE);

    CHECK(QueryPickerResult(&picked, 1, nullptr, &handle, &taken, &size,
                            nullptr, -1, nullptr, &outcome) == E_POINTER);
    CHECK(QueryPickerResult(&picked, 1, &state, &handle, nullptr, &size,
                            nullptr, -1, nullptr, &outcome) == E_POINTER);
    CHECK(QueryPickerResult(&picked, 1, &state, &handle, &taken, &size,
                            nullptr, -1, nullptr, nullptr) == E_POINTER);
    CHECK(QueryPickerResult(&picked, 1, &state, nullptr, &taken, &size,
                            nullptr, -1, nullptr, &outcome) == E_POINTER);
    CHECK(GetHandleInformation(helper_owned, &flags) != FALSE); // no transfer/close on validation error
    CHECK(QueryPickerResult(&picked, 2, &state, &handle, &taken, &size,
                            nullptr, 0, nullptr, &outcome) == E_INVALIDARG);
    CHECK(QueryPickerResult(&picked, 0, &state, nullptr, &taken, nullptr,
                            nullptr, -1, nullptr, &outcome) == S_OK);
    CHECK(state == 1 && taken == 0 && outcome == S_OK);

    CHECK(QueryPickerResult(&picked, 1, &state, &handle, &taken, &size,
                            nullptr, -1, nullptr, &outcome) == S_OK);
    CHECK(taken == 1 && handle == helper_owned && outcome == S_OK);
    HANDLE second = nullptr;
    CHECK(QueryPickerResult(&picked, 1, &state, &second, &taken, &size,
                            nullptr, 0, nullptr, &outcome) == S_OK);
    CHECK(taken == 0 && second == INVALID_HANDLE_VALUE && outcome == S_OK);
    CloseUntakenPickerHandle(&picked);
    CHECK(GetHandleInformation(helper_owned, &flags) != FALSE); // Go still owns it
    CHECK(CloseHandle(helper_owned) != FALSE);

    PickerResultCore release_before_take;
    release_before_take.state = 1;
    release_before_take.fileHandle = CreateEventW(nullptr, FALSE, FALSE, nullptr);
    HANDLE untaken = release_before_take.fileHandle;
    CloseUntakenPickerHandle(&release_before_take);
    CHECK(GetHandleInformation(untaken, &flags) == FALSE);
    CloseUntakenPickerHandle(&release_before_take); // idempotent repeat release

    PickerResultCore cancelled;
    cancelled.state = 2;
    cancelled.outcome = S_OK;
    handle = nullptr;
    CHECK(QueryPickerResult(&cancelled, 0, &state, &handle, &taken, nullptr,
                            nullptr, 0, nullptr, &outcome) == S_OK);
    CHECK(state == 2 && handle == INVALID_HANDLE_VALUE && taken == 0 && outcome == S_OK);

    PickerResultCore failed;
    failed.state = 3;
    failed.outcome = E_ACCESSDENIED;
    handle = nullptr;
    CHECK(QueryPickerResult(&failed, 0, &state, &handle, &taken, &size,
                            nullptr, -1, &required, &outcome) == S_OK);
    CHECK(state == 3 && handle == INVALID_HANDLE_VALUE && taken == 0 && outcome == E_ACCESSDENIED);
}

struct MockPacket {
    std::vector<uint8_t> data;
    uint32_t frames = 0;
    DWORD flags = 0;
    HRESULT getBufferHR = S_OK;
    HRESULT releaseHR = S_OK;
};

class MockPacketSource final : public PacketSource {
public:
    HRESULT GetNextPacketSize(uint32_t* frames) override {
        ++nextCalls;
        if (FAILED(nextHR)) return nextHR;
        *frames = index < packets.size() ? packets[index].frames : 0;
        return S_OK;
    }
    HRESULT GetBuffer(BYTE** data, uint32_t* frames, DWORD* flags) override {
        ++bufferCalls;
        if (index >= packets.size()) return E_FAIL;
        auto& packet = packets[index];
        if (FAILED(packet.getBufferHR)) return packet.getBufferHR;
        *data = packet.data.data();
        *frames = packet.frames;
        *flags = packet.flags;
        return S_OK;
    }
    HRESULT ReleaseBuffer(uint32_t frames) override {
        ++releaseCalls;
        releasedFrames.push_back(frames);
        if (index >= packets.size()) return E_FAIL;
        HRESULT hr = packets[index].releaseHR;
        ++index;
        return hr;
    }
    std::vector<MockPacket> packets;
    size_t index = 0;
    HRESULT nextHR = S_OK;
    int nextCalls = 0;
    int bufferCalls = 0;
    int releaseCalls = 0;
    std::vector<uint32_t> releasedFrames;
};

class MockCleanupOps final : public CaptureCleanupOps {
public:
    HRESULT Stop() override { ++stopCalls; return E_FAIL; }
    void ReleaseService() override { ++serviceReleaseCalls; }
    void FreeMixFormat() override { ++mixFreeCalls; }
    void ReleaseClient() override { ++clientReleaseCalls; }

    int stopCalls = 0;
    int serviceReleaseCalls = 0;
    int mixFreeCalls = 0;
    int clientReleaseCalls = 0;
};

void test_cleanup_diagnostics_preserve_cause_and_release_order() {
    CaptureCleanupState state;
    state.started = true;
    state.serviceAcquired = true;
    state.mixFormatOwned = true;
    state.audioClientOwned = true;
    CaptureCleanupDiagnostics diagnostics;
    MockCleanupOps ops;
    ExecuteCaptureCleanup(&state, &ops, &diagnostics);
    CHECK(diagnostics.stopHResult == E_FAIL);
    CHECK(diagnostics.stepCount == 4);
    CHECK(diagnostics.steps[0] == CAP_CLEANUP_STOP);
    CHECK(diagnostics.steps[1] == CAP_CLEANUP_RELEASE_SERVICE);
    CHECK(diagnostics.steps[2] == CAP_CLEANUP_FREE_MIX_FORMAT);
    CHECK(diagnostics.steps[3] == CAP_CLEANUP_RELEASE_CLIENT);
    CHECK(ops.stopCalls == 1 && ops.serviceReleaseCalls == 1 &&
          ops.mixFreeCalls == 1 && ops.clientReleaseCalls == 1);
    CHECK(!state.started && !state.serviceAcquired && !state.mixFormatOwned && !state.audioClientOwned);

    PacketDrainResult packet;
    packet.terminalReason = CAP_REASON_OVERFLOW;
    packet.terminalHResult = HRESULT_FROM_WIN32(ERROR_BUFFER_OVERFLOW);
    packet.cleanupReleaseHResult = E_ACCESSDENIED;
    ConsumePacketCleanupDiagnostic(packet, &diagnostics);
    CHECK(diagnostics.releaseBufferHResult == E_ACCESSDENIED);
    CHECK(packet.terminalReason == CAP_REASON_OVERFLOW);
    CHECK(packet.terminalHResult == HRESULT_FROM_WIN32(ERROR_BUFFER_OVERFLOW));
}

bool stop_true(void*) { return true; }
bool stop_false(void*) { return false; }
bool stop_on_second_check(void* context) {
    auto* checks = static_cast<int*>(context);
    return ++(*checks) >= 2;
}

MockPacket float_packet(std::initializer_list<float> values, DWORD flags = 0) {
    MockPacket packet;
    packet.frames = static_cast<uint32_t>(values.size());
    packet.flags = flags;
    packet.data.resize(values.size() * sizeof(float));
    std::memcpy(packet.data.data(), values.begin(), packet.data.size());
    return packet;
}

void test_packet_drain_coalescing_cleanup_and_overflow() {
    CaptureFormat format = make_format(3, 32, 32);
    FrameRing ring;
    CHECK(ring.Reset(1, 8) == S_OK);
    std::vector<float> scratch(8);
    bool first = true;
    MockPacketSource source;
    source.packets = {float_packet({0.25f}), float_packet({-0.5f})};
    PacketDrainResult result;
    CHECK(DrainCapturePackets(&source, format, &ring, &scratch, &first,
                              stop_false, nullptr, &result) == S_OK);
    CHECK(result.packetsCommitted == 2);
    CHECK(source.nextCalls == 3 && source.bufferCalls == 2 && source.releaseCalls == 2);
    float read[2]{};
    CHECK(ring.Read(read, 2) == 2);
    CHECK(read[0] == 0.25f && read[1] == -0.5f);

    MockPacketSource stopped;
    stopped.packets = {float_packet({1.0f})};
    first = true;
    CHECK(DrainCapturePackets(&stopped, format, &ring, &scratch, &first,
                              stop_true, nullptr, &result) == S_OK);
    CHECK(result.stopObserved && result.packetsCommitted == 0 && stopped.releaseCalls == 1);
    CHECK(ring.Available() == 0);

    MockPacketSource stopped_during_packet;
    stopped_during_packet.packets = {float_packet({1.0f})};
    int stop_checks = 0;
    first = true;
    CHECK(DrainCapturePackets(&stopped_during_packet, format, &ring, &scratch, &first,
                              stop_on_second_check, &stop_checks, &result) == S_OK);
    CHECK(result.stopObserved && result.packetsCommitted == 0 && stopped_during_packet.releaseCalls == 1);
    CHECK(ring.Available() == 0);

    MockPacketSource stopped_release_failure;
    auto stopped_packet = float_packet({1.0f});
    stopped_packet.releaseHR = E_FAIL;
    stopped_release_failure.packets = {stopped_packet};
    first = true;
    CHECK(DrainCapturePackets(&stopped_release_failure, format, &ring, &scratch, &first,
                              stop_true, nullptr, &result) == S_OK);
    CHECK(result.stopObserved && result.terminalReason == -1 && result.cleanupReleaseHResult == E_FAIL);

    FrameRing small_ring;
    CHECK(small_ring.Reset(1, 1) == S_OK);
    MockPacketSource overflow;
    overflow.packets = {float_packet({1.0f, 2.0f})};
    first = true;
    CHECK(DrainCapturePackets(&overflow, format, &small_ring, &scratch, &first,
                              stop_false, nullptr, &result) == HRESULT_FROM_WIN32(ERROR_BUFFER_OVERFLOW));
    CHECK(result.terminalReason == CAP_REASON_OVERFLOW);
    CHECK(result.packetsCommitted == 0 && overflow.releaseCalls == 1 && small_ring.Available() == 0);

    MockPacketSource overflow_release_failure;
    auto overflow_packet = float_packet({1.0f, 2.0f});
    overflow_packet.releaseHR = E_ACCESSDENIED;
    overflow_release_failure.packets = {overflow_packet};
    first = true;
    CHECK(DrainCapturePackets(&overflow_release_failure, format, &small_ring, &scratch, &first,
                              stop_false, nullptr, &result) == HRESULT_FROM_WIN32(ERROR_BUFFER_OVERFLOW));
    CHECK(result.terminalReason == CAP_REASON_OVERFLOW);
    CHECK(result.terminalHResult == HRESULT_FROM_WIN32(ERROR_BUFFER_OVERFLOW));
    CHECK(result.cleanupReleaseHResult == E_ACCESSDENIED);

    FrameRing format_ring;
    CHECK(format_ring.Reset(1, 4) == S_OK);
    std::vector<float> too_small(1);
    MockPacketSource format_failure;
    auto format_packet = float_packet({1.0f, 2.0f});
    format_packet.releaseHR = E_FAIL;
    format_failure.packets = {format_packet};
    first = true;
    CHECK(DrainCapturePackets(&format_failure, format, &format_ring, &too_small, &first,
                              stop_false, nullptr, &result) == E_INVALIDARG);
    CHECK(result.terminalReason == CAP_REASON_FORMAT_ERROR && result.terminalHResult == E_INVALIDARG);
    CHECK(result.cleanupReleaseHResult == E_FAIL && format_ring.Available() == 0);
}

void test_packet_flags_release_before_commit_and_device_errors() {
    CaptureFormat format = make_format(3, 32, 32);
    std::vector<float> scratch(4);
    FrameRing ring;
    CHECK(ring.Reset(1, 4) == S_OK);
    bool first = true;
    PacketDrainResult result;

    MockPacketSource flags;
    flags.packets = {
        float_packet({7.0f}, AUDCLNT_BUFFERFLAGS_DATA_DISCONTINUITY),
        float_packet({8.0f}, AUDCLNT_BUFFERFLAGS_DATA_DISCONTINUITY | AUDCLNT_BUFFERFLAGS_TIMESTAMP_ERROR),
    };
    CHECK(DrainCapturePackets(&flags, format, &ring, &scratch, &first,
                              stop_false, nullptr, &result) == HRESULT_FROM_WIN32(ERROR_INVALID_DATA));
    CHECK(result.terminalReason == CAP_REASON_DISCONTINUITY);
    CHECK(flags.releaseCalls == 2);
    CHECK(ring.Available() == 1); // first discontinuity accepted, second never committed

    FrameRing discontinuity_ring;
    CHECK(discontinuity_ring.Reset(1, 4) == S_OK);
    MockPacketSource discontinuity_release_failure;
    auto first_ok = float_packet({1.0f});
    auto second_bad = float_packet({2.0f}, AUDCLNT_BUFFERFLAGS_DATA_DISCONTINUITY);
    second_bad.releaseHR = E_FAIL;
    discontinuity_release_failure.packets = {first_ok, second_bad};
    first = true;
    CHECK(DrainCapturePackets(&discontinuity_release_failure, format, &discontinuity_ring,
                              &scratch, &first, stop_false, nullptr, &result) ==
          HRESULT_FROM_WIN32(ERROR_INVALID_DATA));
    CHECK(result.terminalReason == CAP_REASON_DISCONTINUITY);
    CHECK(result.cleanupReleaseHResult == E_FAIL);

    FrameRing release_ring;
    CHECK(release_ring.Reset(1, 4) == S_OK);
    MockPacketSource release_failure;
    auto packet = float_packet({1.0f});
    packet.releaseHR = static_cast<HRESULT>(0x88890004u);
    release_failure.packets = {packet};
    first = true;
    CHECK(DrainCapturePackets(&release_failure, format, &release_ring, &scratch, &first,
                              stop_false, nullptr, &result) == static_cast<HRESULT>(0x88890004u));
    CHECK(result.terminalReason == CAP_REASON_DEVICE_LOST);
    CHECK(result.packetsCommitted == 0 && release_ring.Available() == 0); // release-before-commit

    MockPacketSource silent;
    silent.packets = {float_packet({42.0f}, AUDCLNT_BUFFERFLAGS_SILENT | AUDCLNT_BUFFERFLAGS_TIMESTAMP_ERROR)};
    FrameRing silent_ring;
    CHECK(silent_ring.Reset(1, 2) == S_OK);
    first = true;
    CHECK(DrainCapturePackets(&silent, format, &silent_ring, &scratch, &first,
                              stop_false, nullptr, &result) == S_OK);
    CHECK(result.timestampErrorCount == 1);
    float zero = 1.0f;
    CHECK(silent_ring.Read(&zero, 1) == 1 && zero == 0.0f);

    MockPacketSource next_failure;
    next_failure.nextHR = static_cast<HRESULT>(0x88890004u);
    first = true;
    CHECK(DrainCapturePackets(&next_failure, format, &silent_ring, &scratch, &first,
                              stop_false, nullptr, &result) == static_cast<HRESULT>(0x88890004u));
    CHECK(result.terminalReason == CAP_REASON_DEVICE_LOST && next_failure.releaseCalls == 0);

    MockPacketSource acquire_failure;
    auto bad_packet = float_packet({1.0f});
    bad_packet.getBufferHR = E_ACCESSDENIED;
    acquire_failure.packets = {bad_packet};
    first = true;
    CHECK(DrainCapturePackets(&acquire_failure, format, &silent_ring, &scratch, &first,
                              stop_false, nullptr, &result) == E_ACCESSDENIED);
    CHECK(result.terminalReason == CAP_REASON_PERMISSION_REVOKE && acquire_failure.releaseCalls == 0);
}

bool wait_for_capture(uint32_t id, HANDLE event, bool want_ready, CapCaptureState* terminal_state,
                      HRESULT* terminal_hr, int32_t* terminal_reason) {
    const ULONGLONG deadline = GetTickCount64() + 5000;
    while (GetTickCount64() < deadline) {
        (void)WaitForSingleObject(event, 50);
        CaptureFormat format{};
        format.structSize = sizeof(format);
        format.version = kCaptureFormatVersion;
        int32_t state = -1;
        uint32_t available = 0;
        HRESULT outcome = E_FAIL;
        int32_t reason = -1;
        if (CaptureGetResult(id, &state, &format, &available, &outcome, &reason) != S_OK) return false;
        if (want_ready && state == CAP_STATE_PREPARING && format.ready == 1) return true;
        if (!want_ready && state >= CAP_STATE_STOPPED) {
            if (terminal_state != nullptr) *terminal_state = static_cast<CapCaptureState>(state);
            if (terminal_hr != nullptr) *terminal_hr = outcome;
            if (terminal_reason != nullptr) *terminal_reason = reason;
            return true;
        }
    }
    return false;
}

void test_production_activation_cancel_diagrams_and_diagnostics(HANDLE capture_event) {
    // Diagram A: the capture thread closes its duplicate and exits before the
    // real registered ActivationHandler enters. The callback publishes and
    // signals terminal, while an immediate CaptureRelease drops only the
    // registry ref until the callback epilogue passes its barrier.
    TestResetNativeHooks();
    TestDeferNextActivationCallback();
    uint32_t diagram_a = 0;
    CHECK(CapturePrepare(capture_event, &diagram_a) == S_OK);
    CHECK(wait_for_capture(diagram_a, capture_event, true, nullptr, nullptr, nullptr));
    CHECK(CaptureActivate(diagram_a, L"test-deferred-diagram-a") == S_OK);
    CHECK(CaptureRequestStop(diagram_a, CAP_REASON_CANCEL) == S_OK);
    for (int attempt = 0; attempt < 1000 && TestCaptureThreadDone(diagram_a) == 0; ++attempt) Sleep(1);
    CHECK(TestCaptureThreadDone(diagram_a) == 1);
    for (int attempt = 0; attempt < 1000 && CapIsQuiescent() != S_OK; ++attempt) Sleep(1);
    CHECK(CapIsQuiescent() == S_OK);

    PacketDrainResult routed_packet;
    routed_packet.timestampErrorCount = 2;
    routed_packet.cleanupReleaseHResult = E_ACCESSDENIED;
    CHECK(TestRouteCaptureDiagnostics(diagram_a, &routed_packet, E_FAIL) == S_OK);
    PulsarProbeCaptureDiagnosticsV1 diagnostics{};
    diagnostics.structSize = sizeof(diagnostics);
    diagnostics.version = PULSAR_PROBE_DIAGNOSTICS_EXTENSION_V1;
    CHECK(PulsarProbeCaptureGetDiagnosticsV1(diagram_a, &diagnostics) == S_OK);
    CHECK(diagnostics.timestampErrorCount == 2 &&
          diagnostics.cleanupReleaseBufferHResult == E_ACCESSDENIED &&
          diagnostics.cleanupStopHResult == E_FAIL);

    HANDLE callback_entered = CreateEventW(nullptr, TRUE, FALSE, nullptr);
    HANDLE callback_proceed = CreateEventW(nullptr, TRUE, FALSE, nullptr);
    CHECK(callback_entered != nullptr && callback_proceed != nullptr);
    TestSetActivationCallbackBarrier(callback_entered, callback_proceed);
    auto late_callback = std::async(std::launch::async, [&]() {
        return TestCompleteDeferredActivation(diagram_a, S_OK);
    });
    CHECK(WaitForSingleObject(callback_entered, 1000) == WAIT_OBJECT_0);

    CaptureFormat format{};
    format.structSize = sizeof(format);
    format.version = kCaptureFormatVersion;
    int32_t state = -1;
    uint32_t available = 0;
    HRESULT terminal_hr = E_FAIL;
    int32_t terminal_reason = -1;
    CHECK(CaptureGetResult(diagram_a, &state, &format, &available,
                           &terminal_hr, &terminal_reason) == S_OK);
    CHECK(state == CAP_STATE_CANCELLED);
    CHECK(terminal_hr == HRESULT_FROM_WIN32(ERROR_CANCELLED));
    CHECK(terminal_reason == CAP_REASON_CANCEL); // diagnostics never replace the primary cause
    CHECK(CaptureRelease(diagram_a) == S_OK);
    CHECK(CapIsQuiescent() == S_FALSE);
    uint32_t capture_signals = 0;
    uint32_t capture_closes = 0;
    uint32_t callback_signals = 0;
    uint32_t callback_closes = 0;
    TestGetNotificationCounts(&capture_signals, &capture_closes,
                              &callback_signals, &callback_closes);
    CHECK(capture_signals == 0 && capture_closes == 1);
    CHECK(callback_signals == 1 && callback_closes == 0);
    CHECK(SetEvent(callback_proceed) != FALSE);
    CHECK(late_callback.get() == S_OK);
    CHECK(CapIsQuiescent() == S_OK);
    TestGetNotificationCounts(&capture_signals, &capture_closes,
                              &callback_signals, &callback_closes);
    CHECK(callback_signals == 1 && callback_closes == 1);
    TestSetActivationCallbackBarrier(nullptr, nullptr);
    CloseHandle(callback_entered);
    CloseHandle(callback_proceed);

    // Diagram B: hold the real capture thread immediately after its wake so
    // the registered callback deterministically runs first. It closes without
    // signaling, leaves the registry nonterminal, and the capture thread later
    // publishes/signals after the barrier is released.
    TestResetNativeHooks();
    HANDLE wake_entered = CreateEventW(nullptr, TRUE, FALSE, nullptr);
    HANDLE wake_proceed = CreateEventW(nullptr, TRUE, FALSE, nullptr);
    CHECK(wake_entered != nullptr && wake_proceed != nullptr);
    TestSetCaptureAfterWakeBarrier(wake_entered, wake_proceed);
    TestDeferNextActivationCallback();
    uint32_t diagram_b = 0;
    CHECK(CapturePrepare(capture_event, &diagram_b) == S_OK);
    CHECK(wait_for_capture(diagram_b, capture_event, true, nullptr, nullptr, nullptr));
    CHECK(CaptureActivate(diagram_b, L"test-deferred-diagram-b") == S_OK);
    CHECK(CaptureRequestStop(diagram_b, CAP_REASON_CANCEL) == S_OK);
    CHECK(WaitForSingleObject(wake_entered, 1000) == WAIT_OBJECT_0);
    CHECK(TestCompleteDeferredActivation(diagram_b, S_OK) == S_OK);

    format = {};
    format.structSize = sizeof(format);
    format.version = kCaptureFormatVersion;
    state = -1;
    CHECK(CaptureGetResult(diagram_b, &state, &format, &available,
                           &terminal_hr, &terminal_reason) == S_OK);
    CHECK(state == CAP_STATE_ACTIVATING); // callback did not publish early
    TestGetNotificationCounts(&capture_signals, &capture_closes,
                              &callback_signals, &callback_closes);
    CHECK(capture_signals == 0 && capture_closes == 0);
    CHECK(callback_signals == 0 && callback_closes == 1);
    CHECK(SetEvent(wake_proceed) != FALSE);
    CapCaptureState diagram_b_terminal = CAP_STATE_PREPARING;
    CHECK(wait_for_capture(diagram_b, capture_event, false, &diagram_b_terminal,
                           &terminal_hr, &terminal_reason));
    CHECK(diagram_b_terminal == CAP_STATE_CANCELLED);
    CHECK(terminal_hr == HRESULT_FROM_WIN32(ERROR_CANCELLED));
    CHECK(terminal_reason == CAP_REASON_CANCEL);
    CHECK(CaptureRelease(diagram_b) == S_OK);
    for (int attempt = 0; attempt < 100 && CapIsQuiescent() != S_OK; ++attempt) Sleep(1);
    CHECK(CapIsQuiescent() == S_OK);
    TestGetNotificationCounts(&capture_signals, &capture_closes,
                              &callback_signals, &callback_closes);
    CHECK(capture_signals == 1 && capture_closes == 1);
    CHECK(callback_signals == 0 && callback_closes == 1);
    TestSetCaptureAfterWakeBarrier(nullptr, nullptr);
    CloseHandle(wake_entered);
    CloseHandle(wake_proceed);
    TestResetNativeHooks();
}

void test_global_abi_validation_and_teardown() {
    std::cerr << "[   STEP   ] initialize_and_check_permission" << std::endl;
    int32_t permission = 99;
    CHECK(CapPermissionCheck(&permission) == E_NOT_VALID_STATE);
    CHECK(CapInit() == S_OK);
    CHECK(CapInit() == E_NOT_VALID_STATE);
    CHECK(CapIsQuiescent() == S_OK);
    CHECK(CapPermissionCheck(&permission) == S_OK);
    CHECK(permission >= CAP_PERMISSION_UNAVAILABLE && permission <= CAP_PERMISSION_UNKNOWN);

    std::cerr << "[   STEP   ] permission_subscription_fence" << std::endl;
    test_unsubscribe_with_inflight_production_handler();

    std::cerr << "[   STEP   ] wrong_thread_destroy" << std::endl;
    auto wrong_thread_destroy = std::async(std::launch::async, []() { return CapDestroy(); });
    CHECK(wrong_thread_destroy.get() == RPC_E_WRONG_THREAD);

    std::cerr << "[   STEP   ] rejected_capture_prepare" << std::endl;
    HANDLE capture_event = CreateEventW(nullptr, FALSE, FALSE, nullptr);
    CHECK(capture_event != nullptr);
    uint32_t rejected_id = 0xfeedbeefu;
    TestFailNextDuplicate(E_ACCESSDENIED);
    CHECK(CapturePrepare(capture_event, &rejected_id) == E_ACCESSDENIED);
    CHECK(rejected_id == 0xfeedbeefu);
    TestFailNextThreadLaunch();
    CHECK(CapturePrepare(capture_event, &rejected_id) == E_OUTOFMEMORY);
    CHECK(rejected_id == 0xfeedbeefu);

    std::cerr << "[   STEP   ] stopped_capture_prepare" << std::endl;
    HANDLE coinit_entered = CreateEventW(nullptr, TRUE, FALSE, nullptr);
    HANDLE coinit_proceed = CreateEventW(nullptr, TRUE, FALSE, nullptr);
    CHECK(coinit_entered != nullptr && coinit_proceed != nullptr);
    TestSetCaptureCoInitialize(S_OK, coinit_entered, coinit_proceed);
    uint32_t preparing_id = 0;
    CHECK(CapturePrepare(capture_event, &preparing_id) == S_OK);
    CHECK(WaitForSingleObject(coinit_entered, 1000) == WAIT_OBJECT_0);
    CHECK(CaptureRequestStop(preparing_id, CAP_REASON_CANCEL) == S_OK);
    CaptureFormat preparing_format{};
    preparing_format.structSize = sizeof(preparing_format);
    preparing_format.version = kCaptureFormatVersion;
    int32_t preparing_state = -1;
    uint32_t preparing_available = 0;
    HRESULT preparing_hr = E_FAIL;
    int32_t preparing_reason = -1;
    CHECK(CaptureGetResult(preparing_id, &preparing_state, &preparing_format,
                           &preparing_available, &preparing_hr, &preparing_reason) == S_OK);
    CHECK(preparing_state == CAP_STATE_PREPARING && preparing_format.ready == 0);
    CHECK(SetEvent(coinit_proceed) != FALSE);
    CapCaptureState preparing_terminal = CAP_STATE_PREPARING;
    CHECK(wait_for_capture(preparing_id, capture_event, false, &preparing_terminal,
                           &preparing_hr, &preparing_reason));
    CHECK(preparing_terminal == CAP_STATE_CANCELLED && preparing_reason == CAP_REASON_CANCEL);
    CHECK(CaptureRelease(preparing_id) == S_OK);
    TestResetNativeHooks();
    CloseHandle(coinit_entered);
    CloseHandle(coinit_proceed);

    std::cerr << "[   STEP   ] failed_capture_coinitialize" << std::endl;
    TestSetCaptureCoInitialize(RPC_E_CHANGED_MODE, nullptr, nullptr);
    uint32_t coinit_failure_id = 0;
    CHECK(CapturePrepare(capture_event, &coinit_failure_id) == S_OK);
    CapCaptureState coinit_failure_state = CAP_STATE_PREPARING;
    HRESULT coinit_failure_hr = S_OK;
    int32_t coinit_failure_reason = -1;
    CHECK(wait_for_capture(coinit_failure_id, capture_event, false, &coinit_failure_state,
                           &coinit_failure_hr, &coinit_failure_reason));
    CHECK(coinit_failure_state == CAP_STATE_FAILED);
    CHECK(coinit_failure_hr == RPC_E_CHANGED_MODE);
    CHECK(coinit_failure_reason == CAP_REASON_WASAPI_ERROR);
    CHECK(CaptureRelease(coinit_failure_id) == S_OK);
    TestResetNativeHooks();

    std::cerr << "[   STEP   ] failed_capture_activation" << std::endl;
    uint32_t activation_failure_id = 0;
    CHECK(CapturePrepare(capture_event, &activation_failure_id) == S_OK);
    CHECK(wait_for_capture(activation_failure_id, capture_event, true, nullptr, nullptr, nullptr));
    TestFailNextActivationLaunch(E_ACCESSDENIED);
    CHECK(CaptureActivate(activation_failure_id, L"test-device") == S_OK);
    CapCaptureState activation_failure_state = CAP_STATE_PREPARING;
    HRESULT activation_failure_hr = S_OK;
    int32_t activation_failure_reason = -1;
    CHECK(wait_for_capture(activation_failure_id, capture_event, false, &activation_failure_state,
                           &activation_failure_hr, &activation_failure_reason));
    CHECK(activation_failure_state == CAP_STATE_FAILED);
    CHECK(activation_failure_hr == E_ACCESSDENIED);
    CHECK(activation_failure_reason == CAP_REASON_PERMISSION_REVOKE);
    CHECK(CaptureRelease(activation_failure_id) == S_OK);
    TestResetNativeHooks();

    std::cerr << "[   STEP   ] activation_cancel_diagrams" << std::endl;
    test_production_activation_cancel_diagrams_and_diagnostics(capture_event);

    std::cerr << "[   STEP   ] stalled_handoff_stop" << std::endl;
    uint32_t capture_id = 0;
    CHECK(CapturePrepare(capture_event, &capture_id) == S_OK && capture_id != 0);
    CHECK(wait_for_capture(capture_id, capture_event, true, nullptr, nullptr, nullptr));
    HANDLE handoff_entered = CreateEventW(nullptr, TRUE, FALSE, nullptr);
    HANDLE handoff_proceed = CreateEventW(nullptr, TRUE, FALSE, nullptr);
    CHECK(handoff_entered != nullptr && handoff_proceed != nullptr);
    auto stalled_handoff = std::async(std::launch::async, [&]() {
        return TestHoldCaptureHandoff(capture_id, handoff_entered, handoff_proceed);
    });
    CHECK(WaitForSingleObject(handoff_entered, 1000) == WAIT_OBJECT_0);
    auto exported_stop = std::async(std::launch::async, [&]() {
        return CaptureRequestStop(capture_id, CAP_REASON_CANCEL);
    });
    CHECK(exported_stop.wait_for(std::chrono::milliseconds(100)) == std::future_status::ready);
    CHECK(exported_stop.get() == S_OK);
    CHECK(SetEvent(handoff_proceed) != FALSE);
    CHECK(stalled_handoff.get() == S_OK);
    CloseHandle(handoff_entered);
    CloseHandle(handoff_proceed);
    CapCaptureState terminal_state = CAP_STATE_PREPARING;
    HRESULT terminal_hr = E_FAIL;
    int32_t terminal_reason = -1;
    CHECK(wait_for_capture(capture_id, capture_event, false, &terminal_state, &terminal_hr, &terminal_reason));
    CHECK(terminal_state == CAP_STATE_CANCELLED);
    CHECK(terminal_hr == HRESULT_FROM_WIN32(ERROR_CANCELLED));
    CHECK(terminal_reason == CAP_REASON_CANCEL);
    // Release at zero delay after terminal notification. The real registry,
    // capture thread, and terminal publication fence are exercised here.
    CHECK(CaptureRelease(capture_id) == S_OK);
    for (int attempt = 0; attempt < 100 && CapIsQuiescent() != S_OK; ++attempt) Sleep(1);
    CHECK(CapIsQuiescent() == S_OK);
    CHECK(CloseHandle(capture_event) != FALSE);

    std::cerr << "[   STEP   ] invalid_handles_and_teardown" << std::endl;
    uint32_t id = 0xfeedbeefu;
    std::cerr << "[   STEP   ] invalid_permission_request" << std::endl;
    CHECK(CapPermissionRequest(nullptr, &id) == E_HANDLE && id == 0xfeedbeefu);
    std::cerr << "[   STEP   ] invalid_device_enumeration" << std::endl;
    CHECK(CapEnumerateDevices(nullptr, &id) == E_HANDLE && id == 0xfeedbeefu);
    std::cerr << "[   STEP   ] invalid_capture_prepare" << std::endl;
    CHECK(CapturePrepare(nullptr, &id) == E_HANDLE && id == 0xfeedbeefu);
    std::cerr << "[   STEP   ] invalid_picker_window" << std::endl;
    CHECK(PickerOpenFile(nullptr, L"audio", L".wav", nullptr, &id) == E_INVALIDARG && id == 0xfeedbeefu);
    std::cerr << "[   STEP   ] invalid_picker_event" << std::endl;
    CHECK(PickerOpenFile(reinterpret_cast<HWND>(static_cast<uintptr_t>(1)), L"audio", L".wav", nullptr, &id) == E_HANDLE && id == 0xfeedbeefu);
    std::cerr << "[   STEP   ] invalid_picker_filter" << std::endl;
    CHECK(PickerOpenFile(reinterpret_cast<HWND>(static_cast<uintptr_t>(1)), nullptr, L".wav", nullptr, &id) == E_HANDLE && id == 0xfeedbeefu);
    std::cerr << "[   STEP   ] first_destroy" << std::endl;
    CHECK(CapDestroy() == S_OK);
    std::cerr << "[   STEP   ] idempotent_destroy" << std::endl;
    CHECK(CapDestroy() == S_OK);
    std::cerr << "[   STEP   ] reinitialize" << std::endl;
    CHECK(CapInit() == S_OK);
    std::cerr << "[   STEP   ] destroy_reinitialized" << std::endl;
    CHECK(CapDestroy() == S_OK);

    std::cerr << "[   STEP   ] initialization_rollback" << std::endl;
    TestResetNativeHooks();
    TestSetPostRoInitFailure(E_OUTOFMEMORY);
    CHECK(CapInit() == E_OUTOFMEMORY);
    CHECK(TestRoUninitializeCount() == 1);
    CHECK(CapDestroy() == S_OK); // rollback left no initialized state
    TestSetPostRoInitFailure(S_OK);
    CHECK(CapInit() == S_OK);
    CHECK(CapDestroy() == S_OK);
    TestResetNativeHooks();
}

}  // namespace

void run_test(const char* name, void (*test)()) {
    std::cerr << "[ RUN      ] " << name << std::endl;
    test();
    std::cerr << "[ COMPLETE ] " << name << std::endl;
}

int wmain() {
    run_test("version_and_null_contract", test_version_and_null_contract);
    run_test("packed_fsm_and_priority", test_packed_fsm_and_priority);
    run_test("stop_is_nonblocking_while_handoff_mutex_is_stalled", test_stop_is_nonblocking_while_handoff_mutex_is_stalled);
    run_test("priority_and_seal_races_on_production_control", test_priority_and_seal_races_on_production_control);
    run_test("format_validation", test_format_validation);
    run_test("checked_capture_allocations", test_checked_capture_allocations);
    run_test("float_and_pcm_vectors_unaligned", test_float_and_pcm_vectors_unaligned);
    run_test("ring_exact_fit_overflow_and_drain", test_ring_exact_fit_overflow_and_drain);
    run_test("notification_duplicate_and_callback_release_fence", test_notification_duplicate_and_callback_release_fence);
    run_test("picker_truth_table_and_handle_ownership", test_picker_truth_table_and_handle_ownership);
    run_test("packet_drain_coalescing_cleanup_and_overflow", test_packet_drain_coalescing_cleanup_and_overflow);
    run_test("packet_flags_release_before_commit_and_device_errors", test_packet_flags_release_before_commit_and_device_errors);
    run_test("cleanup_diagnostics_preserve_cause_and_release_order", test_cleanup_diagnostics_preserve_cause_and_release_order);
    run_test("global_abi_validation_and_teardown", test_global_abi_validation_and_teardown);
    if (failures != 0) {
        std::cerr << failures << " native test(s) failed\n";
        return 1;
    }
    std::cout << "pulsar-capture native tests passed\n";
    return 0;
}
