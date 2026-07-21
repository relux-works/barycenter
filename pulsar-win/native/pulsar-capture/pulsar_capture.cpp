#ifndef NOMINMAX
#define NOMINMAX
#endif
#ifndef WIN32_LEAN_AND_MEAN
#define WIN32_LEAN_AND_MEAN
#endif

#include <initguid.h>
#include "pulsar_capture_internal.h"
#include "pulsar_probe_diagnostics.h"

#include <roapi.h>
#include <shobjidl_core.h>
#include <windowsstoragecom.h>
#include <process.h>
#include <winrt/Windows.Devices.Enumeration.h>
#include <winrt/Windows.Foundation.h>
#include <winrt/Windows.Foundation.Collections.h>
#include <winrt/Windows.Media.Devices.h>
#include <winrt/Windows.Security.Authorization.AppCapabilityAccess.h>
#include <winrt/Windows.Storage.h>
#include <winrt/Windows.Storage.Pickers.h>
#include <winrt/base.h>

#include <algorithm>
#include <atomic>
#include <cerrno>
#include <cmath>
#include <cstdio>
#include <cstring>
#include <limits>
#include <map>
#include <memory>
#include <mutex>
#include <new>
#include <string>
#include <utility>
#include <vector>

#ifndef E_ILLEGAL_METHOD_CALL
#define E_ILLEGAL_METHOD_CALL _HRESULT_TYPEDEF_(0x8000000EL)
#endif
#ifndef E_NOT_VALID_STATE
#define E_NOT_VALID_STATE HRESULT_FROM_WIN32(ERROR_INVALID_STATE)
#endif

using winrt::Windows::Foundation::AsyncStatus;
using winrt::Windows::Security::Authorization::AppCapabilityAccess::AppCapability;
using winrt::Windows::Security::Authorization::AppCapabilityAccess::AppCapabilityAccessStatus;

namespace pulsar_capture {

namespace {

constexpr HRESULT kCancelledHr = HRESULT_FROM_WIN32(ERROR_CANCELLED);
constexpr HRESULT kOverflowHr = HRESULT_FROM_WIN32(ERROR_BUFFER_OVERFLOW);
constexpr HRESULT kDiscontinuityHr = HRESULT_FROM_WIN32(ERROR_INVALID_DATA);
constexpr HRESULT kDeviceInvalidated = static_cast<HRESULT>(0x88890004u);
constexpr uint32_t kMaxBufferFrames = 65536;
constexpr REFERENCE_TIME kBufferDuration100ms = 1000000;

enum class OperationKind {
    Permission,
    Enumeration,
    DefaultDevice,
    Picker,
    Capture,
};

struct CallbackScope {
    CallbackScope();
    ~CallbackScope();
};

struct ScopedMTA {
    ScopedMTA() {
        result = RoInitialize(RO_INIT_MULTITHREADED);
        initialized = result == S_OK || result == S_FALSE;
    }
    ~ScopedMTA() { if (initialized) RoUninitialize(); }
    HRESULT result = E_FAIL;
    bool initialized = false;
};

struct Operation {
    explicit Operation(OperationKind value) : kind(value) {}
    virtual ~Operation() {
        const uintptr_t value = notify.exchange(0, std::memory_order_acq_rel);
        if (value != 0) {
            CloseHandle(reinterpret_cast<HANDLE>(value));
        }
    }

    HANDLE take_notify() {
        return reinterpret_cast<HANDLE>(notify.exchange(0, std::memory_order_acq_rel));
    }

    OperationKind kind;
    uint32_t id = 0;
    std::atomic<int32_t> state{0};
    std::atomic<uintptr_t> notify{0};
    std::mutex mutex;
    HRESULT outcome = S_OK;
};

struct PermissionOperation final : Operation {
    PermissionOperation() : Operation(OperationKind::Permission) {}
    int32_t status = CAP_PERMISSION_UNKNOWN;
    winrt::Windows::Foundation::IAsyncOperation<AppCapabilityAccessStatus> async{nullptr};
};

struct DeviceInfoResult {
    std::wstring id;
    std::wstring name;
};

struct EnumerationOperation final : Operation {
    EnumerationOperation() : Operation(OperationKind::Enumeration) {}
    std::vector<DeviceInfoResult> devices;
    winrt::Windows::Foundation::IAsyncOperation<winrt::Windows::Devices::Enumeration::DeviceInformationCollection> async{nullptr};
};

struct DefaultDeviceOperation final : Operation {
    DefaultDeviceOperation() : Operation(OperationKind::DefaultDevice) {}
    std::wstring device_id;
};

struct PickerOperation final : Operation {
    PickerOperation() : Operation(OperationKind::Picker) {}
    ~PickerOperation() override { CloseUntakenPickerHandle(&result); }
    PickerResultCore result;
    winrt::Windows::Foundation::IAsyncOperation<winrt::Windows::Storage::StorageFile> async{nullptr};
};

struct CaptureSession final : Operation, CaptureControl {
    CaptureSession() : Operation(OperationKind::Capture) {
        format = {};
        format.structSize = sizeof(CaptureFormat);
        format.version = kCaptureFormatVersion;
    }
    ~CaptureSession() override {
        if (capture_thread_wake != nullptr) CloseHandle(capture_thread_wake);
        if (capture_data_event != nullptr) CloseHandle(capture_data_event);
        if (stop_event != nullptr) CloseHandle(stop_event);
        const uintptr_t dangling = capture_notify.exchange(0, std::memory_order_acq_rel);
        if (dangling != 0) CloseHandle(reinterpret_cast<HANDLE>(dangling));
    }

	std::atomic<uint32_t> mta_ready{0};
    std::atomic<uint32_t> thread_done{0};
    // 0=creator still preparing publication, 1=published, 2=launch aborted.
    std::atomic<uint32_t> creator_fence{0};
    std::atomic<uintptr_t> capture_notify{0};
    HANDLE original_notify = nullptr;
	HANDLE capture_data_event = nullptr;

	IAudioClient* handoff_client = nullptr;
	bool activation_launched = false;
	bool callback_completed = false;
    winrt::com_ptr<IActivateAudioInterfaceAsyncOperation> activation_op;
    winrt::com_ptr<IActivateAudioInterfaceCompletionHandler> activation_handler;

    std::mutex result_mutex;
    CaptureFormat format{};
    FrameRing ring;
	HRESULT terminal_hr = S_OK;
	int32_t terminal_reason = CAP_REASON_USER_STOP;
	CaptureCleanupDiagnostics cleanup_diagnostics{};
    std::atomic<uint32_t> timestamp_error_count{0};
    std::atomic<HRESULT> cleanup_release_buffer_hr{S_OK};
    std::atomic<HRESULT> cleanup_stop_hr{S_OK};
    std::atomic<uint32_t> quality_requested{0};
    std::atomic<uint32_t> communications_category_active{0};
    // The public category request is not proof that the endpoint enabled AEC
    // or NS. This remains zero until a later exact-build effects query and
    // physical evidence path can establish it honestly.
    std::atomic<uint32_t> native_effects_verified{0};
};

std::atomic<uint32_t> g_subscription_states{0};

struct PermissionSubscription : PermissionNotificationState {
    PermissionSubscription() {
        countedSubscription = true;
        g_subscription_states.fetch_add(1, std::memory_order_acq_rel);
    }
    AppCapability capability{nullptr};
    winrt::event_token token{};
#if defined(PULSAR_CAPTURE_STATIC)
    bool test_registration = false;
#endif
};

std::mutex g_mutex;
std::map<uint32_t, std::shared_ptr<Operation>> g_operations;
std::shared_ptr<PermissionSubscription> g_subscription;
std::atomic<uint32_t> g_callback_refs{0};
std::atomic<uint32_t> g_capture_threads{0};
uint32_t g_next_id = 1;
bool g_initialized = false;
DWORD g_init_thread = 0;
bool g_ro_initialized = false;
std::unique_ptr<uint8_t> g_runtime_state;
AppCapability g_microphone_capability{nullptr};
HRESULT g_capability_hr = E_FAIL;

#if defined(PULSAR_CAPTURE_STATIC)
std::atomic<HRESULT> g_test_post_ro_init_failure{S_OK};
std::atomic<HRESULT> g_test_duplicate_failure{S_OK};
std::atomic<uint32_t> g_test_thread_launch_failure{0};
std::atomic<HRESULT> g_test_activation_launch_failure{S_OK};
std::atomic<HRESULT> g_test_capture_coinit_result{S_OK};
std::atomic<uintptr_t> g_test_capture_coinit_entered{0};
std::atomic<uintptr_t> g_test_capture_coinit_proceed{0};
std::atomic<uintptr_t> g_test_permission_handler_entered{0};
std::atomic<uintptr_t> g_test_permission_handler_proceed{0};
std::atomic<uint32_t> g_test_ro_uninitialize_count{0};
std::atomic<uint32_t> g_test_permission_registration{0};
std::function<void()> g_test_permission_dispatch;
std::atomic<uint32_t> g_test_permission_revoke_count{0};
std::atomic<uint32_t> g_test_defer_activation_callback{0};
std::atomic<uintptr_t> g_test_activation_callback_entered{0};
std::atomic<uintptr_t> g_test_activation_callback_proceed{0};
std::atomic<uintptr_t> g_test_capture_after_wake_entered{0};
std::atomic<uintptr_t> g_test_capture_after_wake_proceed{0};
std::atomic<uint32_t> g_test_capture_notify_signals{0};
std::atomic<uint32_t> g_test_capture_notify_closes{0};
std::atomic<uint32_t> g_test_callback_notify_signals{0};
std::atomic<uint32_t> g_test_callback_notify_closes{0};
#endif

void balanced_ro_uninitialize() {
    // Projected objects are released before this boundary. Drop cached WinRT
    // activation factories as well so a later CapInit cannot reuse factories
    // retained across the apartment teardown.
    winrt::clear_factory_cache();
    RoUninitialize();
#if defined(PULSAR_CAPTURE_STATIC)
    g_test_ro_uninitialize_count.fetch_add(1, std::memory_order_acq_rel);
#endif
}

CallbackScope::CallbackScope() { g_callback_refs.fetch_add(1, std::memory_order_acq_rel); }
CallbackScope::~CallbackScope() { g_callback_refs.fetch_sub(1, std::memory_order_acq_rel); }

HRESULT exception_hr() noexcept {
    try {
        throw;
    } catch (winrt::hresult_error const& error) {
        return error.code();
    } catch (std::bad_alloc const&) {
        return E_OUTOFMEMORY;
    } catch (...) {
        return E_FAIL;
    }
}

HRESULT validate_event(HANDLE event_handle) {
    if (event_handle == nullptr || event_handle == INVALID_HANDLE_VALUE) return E_HANDLE;
    DWORD flags = 0;
    if (!GetHandleInformation(event_handle, &flags)) return HRESULT_FROM_WIN32(GetLastError());
    return S_OK;
}

uint32_t allocate_id_locked() {
    std::vector<uint32_t> occupied;
    occupied.reserve(g_operations.size());
    for (auto const& item : g_operations) occupied.push_back(item.first);
    const uint32_t candidate = FindAvailableOperationId(g_next_id, occupied, g_operations.size());
    if (candidate != 0) g_next_id = candidate == UINT32_MAX ? 1 : candidate + 1;
    return candidate;
}

template <typename T>
std::shared_ptr<T> lookup_operation(uint32_t id, OperationKind kind) {
    std::lock_guard<std::mutex> lock(g_mutex);
    auto it = g_operations.find(id);
    if (it == g_operations.end() || it->second->kind != kind) return {};
    return std::dynamic_pointer_cast<T>(it->second);
}

bool has_kind_locked(OperationKind kind) {
    for (auto const& item : g_operations) {
        if (item.second->kind == kind) return true;
    }
    return false;
}

HRESULT require_initialized(bool require_ui_thread = false) {
    std::lock_guard<std::mutex> lock(g_mutex);
    if (!g_initialized) return E_NOT_VALID_STATE;
    if (require_ui_thread && GetCurrentThreadId() != g_init_thread) return RPC_E_WRONG_THREAD;
    return S_OK;
}

int32_t map_permission(AppCapabilityAccessStatus value) {
    switch (value) {
    case AppCapabilityAccessStatus::DeniedByUser: return CAP_PERMISSION_DENIED_BY_USER;
    case AppCapabilityAccessStatus::Allowed: return CAP_PERMISSION_ALLOWED;
    case AppCapabilityAccessStatus::UserPromptRequired: return CAP_PERMISSION_PROMPT_REQUIRED;
    case AppCapabilityAccessStatus::DeniedBySystem: return CAP_PERMISSION_DENIED_BY_SYSTEM;
    case AppCapabilityAccessStatus::NotDeclaredByApp: return CAP_PERMISSION_NOT_DECLARED;
    default: return CAP_PERMISSION_UNKNOWN;
    }
}

void complete_and_signal(std::shared_ptr<Operation> const& op, int32_t state, HRESULT outcome) {
    {
        std::lock_guard<std::mutex> lock(op->mutex);
        op->outcome = outcome;
    }
    op->state.store(state, std::memory_order_release);
    HANDLE notify = op->take_notify();
    if (notify != nullptr) {
        SetEvent(notify);
        CloseHandle(notify);
    }
}

void dispatch_permission_access_changed(const std::shared_ptr<PermissionSubscription>& sub) {
    if (!sub) return;
    try {
        if (sub->capability) (void)sub->capability.CheckAccess();
    } catch (...) {}
    SignalPermissionNotification(sub);
}

HRESULT register_permission_access_changed(const std::shared_ptr<PermissionSubscription>& sub) {
    if (!sub) return E_POINTER;
#if defined(PULSAR_CAPTURE_STATIC)
    if (g_test_permission_registration.load(std::memory_order_acquire) != 0) {
        sub->test_registration = true;
        sub->token.value = 0x50434150; // deterministic synthetic token at the OS seam
        g_test_permission_dispatch = [sub]() { dispatch_permission_access_changed(sub); };
        return S_OK;
    }
#endif
    if (!sub->capability) return FAILED(g_capability_hr) ? g_capability_hr : E_FAIL;
    try {
        sub->token = sub->capability.AccessChanged([sub](auto const&, auto const&) {
            dispatch_permission_access_changed(sub);
        });
        return S_OK;
    } catch (...) {
        return exception_hr();
    }
}

HRESULT revoke_permission_access_changed(const std::shared_ptr<PermissionSubscription>& sub) {
    if (!sub) return S_OK;
#if defined(PULSAR_CAPTURE_STATIC)
    if (sub->test_registration) {
        {
            std::lock_guard<std::mutex> lock(g_mutex);
            g_test_permission_dispatch = {};
        }
        sub->token.value = 0;
        g_test_permission_revoke_count.fetch_add(1, std::memory_order_acq_rel);
        return S_OK;
    }
#endif
    try {
        sub->capability.AccessChanged(sub->token);
        return S_OK;
    } catch (...) {
        return exception_hr();
    }
}

HRESULT copy_string(std::wstring const& value, wchar_t* buffer, int32_t capacity, int32_t* required, bool strict) {
    if (required != nullptr) {
        const size_t chars = value.size() + 1;
        *required = chars > static_cast<size_t>(INT32_MAX) ? INT32_MAX : static_cast<int32_t>(chars);
    }
    if (buffer == nullptr || capacity <= 0) {
        return strict && !value.empty() ? HRESULT_FROM_WIN32(ERROR_INSUFFICIENT_BUFFER) : S_OK;
    }
    const size_t writable = static_cast<size_t>(capacity - 1);
    const size_t copied = std::min(writable, value.size());
    if (copied != 0) std::memcpy(buffer, value.data(), copied * sizeof(wchar_t));
    buffer[copied] = L'\0';
    if (strict && copied != value.size()) return HRESULT_FROM_WIN32(ERROR_INSUFFICIENT_BUFFER);
    return S_OK;
}

template <typename Async>
HRESULT cancel_async(Async const& async) {
    if (!async) return E_NOT_VALID_STATE;
    ScopedMTA apartment;
    if (FAILED(apartment.result) && apartment.result != RPC_E_CHANGED_MODE) return apartment.result;
    try {
        async.Cancel();
        return S_OK;
    } catch (winrt::hresult_error const& error) {
        return error.code();
    } catch (...) {
        return E_FAIL;
    }
}

HRESULT exact_hresult_for_reason(int32_t reason, HRESULT fallback) {
    return HResultForReason(reason, fallback);
}

uint8_t public_for_private(uint8_t state) {
    if (state <= kPrivateStatePrepared) return CAP_STATE_PREPARING;
    if (state == kPrivateStateActivating) return CAP_STATE_ACTIVATING;
    return CAP_STATE_CAPTURING;
}

bool is_terminal_private(uint8_t state) { return state == kPrivateStateTerminal; }

bool is_stopping_private(uint8_t state) { return state >= kPrivateStateStopping; }

bool install_reason(CaptureSession* session, int32_t reason, HRESULT hr, bool internal) {
    return InstallCaptureReason(session, reason, hr, internal);
}

int32_t seal_reason(CaptureSession* session) {
    return SealCaptureReason(session);
}

HRESULT pending_hresult_for(CaptureSession* session, int32_t reason) {
    return SealedCaptureHResult(session, reason);
}

void publish_terminal(CaptureSession* session, int32_t reason, HRESULT hr) {
    session->terminal_reason = reason;
    session->terminal_hr = exact_hresult_for_reason(reason, hr);
    uint64_t current = session->packed.load(std::memory_order_acquire);
    const uint64_t terminal = PackState(PackedLastPublicState(current), kPrivateStateTerminal, true, static_cast<uint16_t>(reason));
    std::atomic_thread_fence(std::memory_order_seq_cst);
    session->packed.store(terminal, std::memory_order_release);
}

int32_t classify_audio_error(HRESULT hr, bool format_stage = false) {
    if (hr == E_ACCESSDENIED) return CAP_REASON_PERMISSION_REVOKE;
    if (hr == kDeviceInvalidated) return CAP_REASON_DEVICE_LOST;
    if (format_stage) return CAP_REASON_FORMAT_ERROR;
    return CAP_REASON_WASAPI_ERROR;
}

class WASAPIPacketSource final : public PacketSource {
public:
    explicit WASAPIPacketSource(IAudioCaptureClient* value) : value_(value) {}
    HRESULT GetNextPacketSize(uint32_t* frames) override { return value_->GetNextPacketSize(frames); }
    HRESULT GetBuffer(BYTE** data, uint32_t* frames, DWORD* flags) override {
        return value_->GetBuffer(data, frames, flags, nullptr, nullptr);
    }
    HRESULT ReleaseBuffer(uint32_t frames) override { return value_->ReleaseBuffer(frames); }
private:
    IAudioCaptureClient* value_;
};

bool capture_stop_check(void* context) {
    auto* session = static_cast<CaptureSession*>(context);
    return WaitForSingleObject(session->stop_event, 0) == WAIT_OBJECT_0;
}

unsigned __stdcall capture_thread_main(void* context);

HRESULT begin_thread_error() {
    const int crt_errno = errno;
    unsigned long dos_errno = 0;
    (void)_get_doserrno(&dos_errno);
    if (dos_errno != 0) return HRESULT_FROM_WIN32(dos_errno);
    switch (crt_errno) {
    case EINVAL: return E_INVALIDARG;
    case EACCES: return E_ACCESSDENIED;
    case EAGAIN:
    case ENOMEM: return E_OUTOFMEMORY;
    default: return E_FAIL;
    }
}

uintptr_t launch_capture_thread(void* context, unsigned* thread_id) {
#if defined(PULSAR_CAPTURE_STATIC)
    if (g_test_thread_launch_failure.exchange(0, std::memory_order_acq_rel) != 0) {
        errno = EAGAIN;
        (void)_set_doserrno(0);
        return 0;
    }
#endif
    return _beginthreadex(nullptr, 0, capture_thread_main, context, 0, thread_id);
}

struct CaptureResources final : CaptureCleanupOps {
    IAudioClient* client = nullptr;
    IAudioCaptureClient* capture = nullptr;
    WAVEFORMATEX* mix = nullptr;
    CaptureCleanupState ownership{};

    HRESULT Stop() override { return client->Stop(); }
    void ReleaseService() override { capture->Release(); capture = nullptr; }
    void FreeMixFormat() override { CoTaskMemFree(mix); mix = nullptr; }
    void ReleaseClient() override { client->Release(); client = nullptr; }
};

void cleanup_capture(CaptureResources& resources, CaptureCleanupDiagnostics* diagnostics) {
    ExecuteCaptureCleanup(&resources.ownership, &resources, diagnostics);
}

void emit_cleanup_diagnostics(const CaptureCleanupDiagnostics& diagnostics) {
    if (SUCCEEDED(diagnostics.releaseBufferHResult) && SUCCEEDED(diagnostics.stopHResult)) return;
    char message[256]{};
    (void)std::snprintf(message, sizeof(message),
                       "{\"component\":\"pulsar-capture\",\"event\":\"capture_cleanup\","
                       "\"releaseBufferHResult\":\"0x%08x\",\"stopHResult\":\"0x%08x\","
                       "\"cleanupSteps\":%u}\n",
                       static_cast<uint32_t>(diagnostics.releaseBufferHResult),
                       static_cast<uint32_t>(diagnostics.stopHResult), diagnostics.stepCount);
    OutputDebugStringA(message);
}

void store_first_failure(std::atomic<HRESULT>* destination, HRESULT value) {
    if (destination == nullptr || SUCCEEDED(value)) return;
    HRESULT empty = S_OK;
    (void)destination->compare_exchange_strong(
        empty, value, std::memory_order_release, std::memory_order_relaxed);
}

void consume_capture_packet_evidence(CaptureSession* session, const PacketDrainResult& packet) {
    if (session == nullptr) return;
    ConsumePacketCleanupDiagnostic(packet, &session->cleanup_diagnostics);
    if (packet.timestampErrorCount != 0) {
        session->timestamp_error_count.fetch_add(packet.timestampErrorCount, std::memory_order_acq_rel);
    }
    store_first_failure(&session->cleanup_release_buffer_hr, packet.cleanupReleaseHResult);
}

void consume_capture_cleanup_evidence(CaptureSession* session,
                                      const CaptureCleanupDiagnostics& cleanup) {
    if (session == nullptr) return;
    store_first_failure(&session->cleanup_stop_hr, cleanup.stopHResult);
}

void signal_and_close_capture_notify(HANDLE notify, bool signal) {
    if (notify == nullptr) return;
    if (signal) {
        SetEvent(notify);
#if defined(PULSAR_CAPTURE_STATIC)
        g_test_capture_notify_signals.fetch_add(1, std::memory_order_acq_rel);
#endif
    }
    CloseHandle(notify);
#if defined(PULSAR_CAPTURE_STATIC)
    g_test_capture_notify_closes.fetch_add(1, std::memory_order_acq_rel);
#endif
}

void finish_capture_thread(CaptureSession* session, CaptureResources& resources,
                           bool com_initialized, HANDLE local_notify, int32_t reason, HRESULT hr) {
    seal_reason(session);
    const int32_t sealed_reason = static_cast<int32_t>(PackedReason(session->packed.load(std::memory_order_acquire)));
    cleanup_capture(resources, &session->cleanup_diagnostics);
    consume_capture_cleanup_evidence(session, session->cleanup_diagnostics);
    emit_cleanup_diagnostics(session->cleanup_diagnostics);
    if (com_initialized) CoUninitialize();
    const HRESULT sealed_hr = sealed_reason == reason ? hr : pending_hresult_for(session, sealed_reason);
    session->thread_done.store(1, std::memory_order_release);
    std::atomic_thread_fence(std::memory_order_seq_cst);
    publish_terminal(session, sealed_reason, sealed_hr);
    signal_and_close_capture_notify(local_notify, true);
}

unsigned __stdcall capture_thread_main(void* context) {
    auto* raw = static_cast<CaptureSession*>(context);
    uint32_t creator_state = 0;
    while ((creator_state = raw->creator_fence.load(std::memory_order_acquire)) == 0) {
        SwitchToThread();
    }
    if (creator_state == 2) return 0;
    CaptureSession* session = nullptr;
    {
        std::lock_guard<std::mutex> lock(g_mutex);
        auto it = g_operations.find(raw->id);
        if (it != g_operations.end() && it->second.get() == raw && it->second->kind == OperationKind::Capture) {
            session = raw;
        }
    }
    if (!session) return 0;

    g_capture_threads.fetch_add(1, std::memory_order_acq_rel);
    HANDLE local_notify = reinterpret_cast<HANDLE>(session->capture_notify.exchange(0, std::memory_order_acq_rel));
    CaptureResources resources;
    bool com_initialized = false;
    HRESULT hr = S_OK;
#if defined(PULSAR_CAPTURE_STATIC)
    HANDLE coinit_entered = reinterpret_cast<HANDLE>(g_test_capture_coinit_entered.load(std::memory_order_acquire));
    HANDLE coinit_proceed = reinterpret_cast<HANDLE>(g_test_capture_coinit_proceed.load(std::memory_order_acquire));
    if (coinit_entered != nullptr) SetEvent(coinit_entered);
    if (coinit_proceed != nullptr) (void)WaitForSingleObject(coinit_proceed, INFINITE);
    const HRESULT injected_coinit = g_test_capture_coinit_result.load(std::memory_order_acquire);
    if (FAILED(injected_coinit)) hr = injected_coinit;
    else hr = CoInitializeEx(nullptr, COINIT_MULTITHREADED);
#else
    hr = CoInitializeEx(nullptr, COINIT_MULTITHREADED);
#endif
    if (hr != S_OK && hr != S_FALSE) {
        install_reason(session, CAP_REASON_WASAPI_ERROR, hr, true);
        finish_capture_thread(session, resources, false, local_notify, CAP_REASON_WASAPI_ERROR, hr);
        g_capture_threads.fetch_sub(1, std::memory_order_acq_rel);
        return 0;
    }
    com_initialized = true;

    uint64_t expected = session->packed.load(std::memory_order_acquire);
    if (PackedState(expected) == kPrivateStatePreparing) {
        const uint64_t prepared = PackState(CAP_STATE_PREPARING, kPrivateStatePrepared, false, PackedReason(expected));
        if (session->packed.compare_exchange_strong(expected, prepared, std::memory_order_acq_rel, std::memory_order_acquire)) {
            session->mta_ready.store(1, std::memory_order_release);
            if (local_notify != nullptr) SetEvent(local_notify);
        }
    }
    if (PackedState(session->packed.load(std::memory_order_acquire)) >= kPrivateStateStopping) {
        const int32_t reason = seal_reason(session);
        finish_capture_thread(session, resources, com_initialized, local_notify, reason, pending_hresult_for(session, reason));
        g_capture_threads.fetch_sub(1, std::memory_order_acq_rel);
        return 0;
    }

    WaitForSingleObject(session->capture_thread_wake, INFINITE);
#if defined(PULSAR_CAPTURE_STATIC)
    HANDLE after_wake_entered = reinterpret_cast<HANDLE>(
        g_test_capture_after_wake_entered.load(std::memory_order_acquire));
    HANDLE after_wake_proceed = reinterpret_cast<HANDLE>(
        g_test_capture_after_wake_proceed.load(std::memory_order_acquire));
    if (after_wake_entered != nullptr) SetEvent(after_wake_entered);
    if (after_wake_proceed != nullptr) (void)WaitForSingleObject(after_wake_proceed, INFINITE);
#endif

    bool callback_completed = false;
    bool activation_launched = false;
    {
        std::lock_guard<std::mutex> lock(session->handoff_mutex);
        resources.client = session->handoff_client;
        session->handoff_client = nullptr;
        callback_completed = session->callback_completed;
        activation_launched = session->activation_launched;
    }

    const uint8_t after_wake = PackedState(session->packed.load(std::memory_order_acquire));
    if (after_wake >= kPrivateStateStopping && activation_launched && !callback_completed && resources.client == nullptr) {
        // Diagram A: the late callback is the sole terminal publisher.
        seal_reason(session);
        CoUninitialize();
        session->thread_done.store(1, std::memory_order_release);
        std::atomic_thread_fence(std::memory_order_seq_cst);
        signal_and_close_capture_notify(local_notify, false);
        g_capture_threads.fetch_sub(1, std::memory_order_acq_rel);
        return 0;
    }

    if (resources.client == nullptr) {
        if (PackedState(session->packed.load(std::memory_order_acquire)) < kPrivateStateStopping) {
            install_reason(session, CAP_REASON_WASAPI_ERROR, E_FAIL, true);
        }
        const int32_t reason = seal_reason(session);
        finish_capture_thread(session, resources, com_initialized, local_notify, reason,
                              pending_hresult_for(session, reason));
        g_capture_threads.fetch_sub(1, std::memory_order_acq_rel);
        return 0;
    }
    resources.ownership.audioClientOwned = true;

    auto stopping = [&]() { return is_stopping_private(PackedState(session->packed.load(std::memory_order_acquire))); };
    int32_t failure_reason = CAP_REASON_WASAPI_ERROR;
    auto fail = [&](HRESULT value, bool format_stage = false) {
        const int32_t reason = classify_audio_error(value, format_stage);
        install_reason(session, reason, value, true);
        hr = value;
        failure_reason = reason;
    };

    if (!stopping() && session->quality_requested.load(std::memory_order_acquire) != 0) {
        IAudioClient2* communications = nullptr;
        const HRESULT query = resources.client->QueryInterface(
            __uuidof(IAudioClient2), reinterpret_cast<void**>(&communications));
        if (SUCCEEDED(query) && communications != nullptr) {
            AudioClientProperties properties{};
            properties.cbSize = sizeof(properties);
            properties.bIsOffload = FALSE;
            properties.eCategory = AudioCategory_Communications;
            properties.Options = AUDCLNT_STREAMOPTIONS_NONE;
            const HRESULT category = communications->SetClientProperties(&properties);
            if (SUCCEEDED(category)) {
                session->communications_category_active.store(1, std::memory_order_release);
            }
            communications->Release();
        }
    }

    if (!stopping()) {
        hr = resources.client->GetMixFormat(&resources.mix);
        // Treat any non-null out pointer as acquired even when the COM method
        // reports failure; cleanup must not trust a failing provider to have
        // preserved the conventional null-on-failure behavior.
        resources.ownership.mixFormatOwned = resources.mix != nullptr;
        if (FAILED(hr)) fail(hr);
    }
    CaptureFormat validated{};
    validated.structSize = sizeof(CaptureFormat);
    validated.version = kCaptureFormatVersion;
    std::wstring diagnostic;
    if (!stopping() && SUCCEEDED(hr)) {
        hr = ValidateAndFillCaptureFormat(resources.mix, &validated, &diagnostic);
        if (FAILED(hr)) fail(hr, true);
        else {
            validated.ready = 1;
            std::lock_guard<std::mutex> lock(session->result_mutex);
            session->format = validated;
        }
    }
    if (!stopping() && SUCCEEDED(hr)) {
        hr = resources.client->Initialize(AUDCLNT_SHAREMODE_SHARED, AUDCLNT_STREAMFLAGS_EVENTCALLBACK,
                                          kBufferDuration100ms, 0, resources.mix, nullptr);
        if (FAILED(hr)) fail(hr);
        else {
            CoTaskMemFree(resources.mix);
            resources.mix = nullptr;
            resources.ownership.mixFormatOwned = false;
        }
    }
    UINT32 buffer_frames = 0;
    if (!stopping() && SUCCEEDED(hr)) {
        hr = resources.client->GetBufferSize(&buffer_frames);
        if (FAILED(hr)) fail(hr);
        else if (buffer_frames == 0 || buffer_frames > kMaxBufferFrames) fail(E_INVALIDARG);
    }
    std::vector<float> scratch;
    if (!stopping() && SUCCEEDED(hr)) {
        uint32_t ring_frames = 0;
        size_t scratch_samples = 0;
        hr = ComputeCaptureAllocation(validated.sampleRate, validated.channels, buffer_frames,
                                      &ring_frames, &scratch_samples);
        if (SUCCEEDED(hr)) {
            hr = session->ring.Reset(validated.channels, ring_frames);
            if (SUCCEEDED(hr)) {
                try { scratch.resize(scratch_samples); }
                catch (...) { hr = E_OUTOFMEMORY; }
            }
        }
        if (FAILED(hr)) fail(hr);
    }
    if (!stopping() && SUCCEEDED(hr)) {
        hr = resources.client->SetEventHandle(session->capture_data_event);
        if (FAILED(hr)) fail(hr);
    }
    if (!stopping() && SUCCEEDED(hr)) {
        hr = resources.client->GetService(__uuidof(IAudioCaptureClient), reinterpret_cast<void**>(&resources.capture));
        resources.ownership.serviceAcquired = resources.capture != nullptr;
        if (FAILED(hr)) fail(hr);
    }
    if (!stopping() && SUCCEEDED(hr)) {
        hr = resources.client->Start();
        if (FAILED(hr)) fail(hr);
        else resources.ownership.started = true;
    }

    if (!stopping() && SUCCEEDED(hr)) {
        uint64_t current = session->packed.load(std::memory_order_acquire);
        const uint64_t capturing = PackState(CAP_STATE_CAPTURING, kPrivateStateCapturing, false, PackedReason(current));
        if (!session->packed.compare_exchange_strong(current, capturing, std::memory_order_acq_rel, std::memory_order_acquire)) {
            // A stop won while Start was executing; started=true makes cleanup call Stop.
        } else if (local_notify != nullptr) {
            SetEvent(local_notify);
        }
    }

    bool first_packet = true;
    bool done = stopping() || FAILED(hr);
    HANDLE waits[2] = {session->capture_data_event, session->stop_event};
    WASAPIPacketSource packet_source(resources.capture);
    while (!done) {
        const DWORD wait = WaitForMultipleObjects(2, waits, FALSE, INFINITE);
        if (wait == WAIT_OBJECT_0 + 1) break;
        if (wait != WAIT_OBJECT_0) {
            fail(HRESULT_FROM_WIN32(GetLastError()));
            break;
        }
        PacketDrainResult drain;
        hr = DrainCapturePackets(&packet_source, validated, &session->ring, &scratch,
                                 &first_packet, capture_stop_check, session, &drain);
        consume_capture_packet_evidence(session, drain);
        if (drain.packetsCommitted != 0 && local_notify != nullptr) SetEvent(local_notify);
        if (drain.terminalReason >= 0) {
            install_reason(session, drain.terminalReason, drain.terminalHResult, true);
            failure_reason = drain.terminalReason;
            hr = drain.terminalHResult;
            done = true;
        } else if (drain.stopObserved || FAILED(hr)) {
            done = true;
        }
    }

    if (PackedState(session->packed.load(std::memory_order_acquire)) < kPrivateStateStopping) {
        if (FAILED(hr)) install_reason(session, failure_reason, hr, true);
        else install_reason(session, CAP_REASON_USER_STOP, S_OK, true);
    }
    const int32_t reason = seal_reason(session);
    finish_capture_thread(session, resources, com_initialized, local_notify, reason,
                          reason == failure_reason ? hr : pending_hresult_for(session, reason));
    g_capture_threads.fetch_sub(1, std::memory_order_acq_rel);
    return 0;
}

struct ActivationHandler : winrt::implements<ActivationHandler, IActivateAudioInterfaceCompletionHandler> {
    ActivationHandler(std::shared_ptr<CaptureSession> value, HANDLE notify)
        : session_(std::move(value)), notify_(reinterpret_cast<uintptr_t>(notify)) {}

    HANDLE abandon_notify() noexcept {
        return reinterpret_cast<HANDLE>(notify_.exchange(0, std::memory_order_acq_rel));
    }

    HRESULT __stdcall ActivateCompleted(IActivateAudioInterfaceAsyncOperation* operation) noexcept override {
        CallbackScope callback_scope;
        auto session = session_;
        if (!session) return E_HANDLE;
        HRESULT activate_hr = E_FAIL;
        IUnknown* unknown = nullptr;
        HRESULT call_hr = operation->GetActivateResult(&activate_hr, &unknown);
        if (FAILED(call_hr)) activate_hr = call_hr;
        IAudioClient* client = nullptr;
        if (SUCCEEDED(activate_hr) && unknown != nullptr) {
            activate_hr = unknown->QueryInterface(__uuidof(IAudioClient), reinterpret_cast<void**>(&client));
        }
        if (unknown != nullptr) unknown->Release();

        bool callback_publishes = false;
        int32_t failure_reason = -1;
        {
            std::lock_guard<std::mutex> lock(session->handoff_mutex);
            const uint8_t state = PackedState(session->packed.load(std::memory_order_acquire));
            session->activation_op = nullptr;
            session->activation_handler = nullptr;
            session->callback_completed = true;
            if (state >= kPrivateStateStopping) {
                if (client != nullptr) { client->Release(); client = nullptr; }
                callback_publishes = PlanActivationCancellation(
                    session->thread_done.load(std::memory_order_acquire)).callbackPublishes;
            } else if (FAILED(activate_hr) || client == nullptr) {
                failure_reason = classify_audio_error(activate_hr);
            } else {
                session->handoff_client = client;
                client = nullptr;
            }
        }
		if (failure_reason >= 0) {
			install_reason(session.get(), failure_reason, activate_hr, true);
		}

        HANDLE local_notify = abandon_notify();
        if (callback_publishes) {
            const int32_t reason = seal_reason(session.get());
            publish_terminal(session.get(), reason, pending_hresult_for(session.get(), reason));
            if (local_notify != nullptr) {
                SetEvent(local_notify);
#if defined(PULSAR_CAPTURE_STATIC)
                g_test_callback_notify_signals.fetch_add(1, std::memory_order_acq_rel);
#endif
            }
        } else {
            SetEvent(session->capture_thread_wake);
        }
#if defined(PULSAR_CAPTURE_STATIC)
        HANDLE callback_entered = reinterpret_cast<HANDLE>(
            g_test_activation_callback_entered.load(std::memory_order_acquire));
        HANDLE callback_proceed = reinterpret_cast<HANDLE>(
            g_test_activation_callback_proceed.load(std::memory_order_acquire));
        if (callback_entered != nullptr) SetEvent(callback_entered);
        if (callback_proceed != nullptr) (void)WaitForSingleObject(callback_proceed, INFINITE);
#endif
        if (local_notify != nullptr) {
            CloseHandle(local_notify);
#if defined(PULSAR_CAPTURE_STATIC)
            g_test_callback_notify_closes.fetch_add(1, std::memory_order_acq_rel);
#endif
        }
        session_.reset();
        return S_OK;
    }

private:
    std::shared_ptr<CaptureSession> session_;
    std::atomic<uintptr_t> notify_{0};
};

#if defined(PULSAR_CAPTURE_STATIC)
struct TestActivationOperation
    : winrt::implements<TestActivationOperation, IActivateAudioInterfaceAsyncOperation> {
    explicit TestActivationOperation(HRESULT result) : result_(result) {}

    HRESULT __stdcall GetActivateResult(HRESULT* activateResult,
                                        IUnknown** activatedInterface) noexcept override {
        if (activateResult == nullptr || activatedInterface == nullptr) return E_POINTER;
        *activateResult = result_;
        *activatedInterface = nullptr;
        return S_OK;
    }

private:
    HRESULT result_;
};
#endif

}  // namespace

uint64_t PackState(uint8_t last_public_state, uint8_t state, bool sealed, uint16_t reason) {
    return (static_cast<uint64_t>(last_public_state) << 56) |
           (static_cast<uint64_t>(state) << 24) |
           (sealed ? (uint64_t{1} << 23) : 0) |
           static_cast<uint64_t>(reason);
}

uint8_t PackedLastPublicState(uint64_t packed) { return static_cast<uint8_t>((packed >> 56) & 0xff); }
uint8_t PackedState(uint64_t packed) { return static_cast<uint8_t>((packed >> 24) & 0xff); }
bool PackedSealed(uint64_t packed) { return ((packed >> 23) & 1) != 0; }
uint16_t PackedReason(uint64_t packed) { return static_cast<uint16_t>(packed & 0xffff); }

uint8_t CollapsedPublicStateForPrivate(uint8_t private_state) { return public_for_private(private_state); }

int ReasonPriority(int32_t reason) {
    switch (reason) {
    case CAP_REASON_OVERFLOW: return 1;
    case CAP_REASON_DISCONTINUITY: return 2;
    case CAP_REASON_PERMISSION_REVOKE: return 3;
    case CAP_REASON_WASAPI_ERROR:
    case CAP_REASON_FORMAT_ERROR: return 4;
    case CAP_REASON_DEVICE_LOST: return 5;
    case CAP_REASON_SHUTDOWN: return 6;
    case CAP_REASON_SUSPEND: return 7;
    case CAP_REASON_LOCK: return 8;
    case CAP_REASON_CANCEL: return 9;
    case CAP_REASON_USER_STOP: return 10;
    default: return 100;
    }
}

int32_t HigherPriorityReason(int32_t current, int32_t next) {
    return ReasonPriority(next) < ReasonPriority(current) ? next : current;
}

CapCaptureState PublicStateFromReason(int32_t reason) {
    if (reason == CAP_REASON_CANCEL) return CAP_STATE_CANCELLED;
    switch (reason) {
    case CAP_REASON_USER_STOP:
    case CAP_REASON_DEVICE_LOST:
    case CAP_REASON_SHUTDOWN:
    case CAP_REASON_SUSPEND:
    case CAP_REASON_LOCK: return CAP_STATE_STOPPED;
    default: return CAP_STATE_FAILED;
    }
}

HRESULT HResultForReason(int32_t reason, HRESULT fallback) {
    switch (reason) {
    case CAP_REASON_USER_STOP:
    case CAP_REASON_SHUTDOWN:
    case CAP_REASON_SUSPEND:
    case CAP_REASON_LOCK: return S_OK;
    case CAP_REASON_PERMISSION_REVOKE: return E_ACCESSDENIED;
    case CAP_REASON_DEVICE_LOST: return kDeviceInvalidated;
    case CAP_REASON_CANCEL: return kCancelledHr;
    case CAP_REASON_OVERFLOW: return kOverflowHr;
    case CAP_REASON_FORMAT_ERROR: return E_INVALIDARG;
    case CAP_REASON_DISCONTINUITY: return kDiscontinuityHr;
    case CAP_REASON_WASAPI_ERROR: return FAILED(fallback) ? fallback : E_FAIL;
    default: return E_INVALIDARG;
    }
}

bool InstallCaptureReason(CaptureControl* control, int32_t reason, HRESULT hresult, bool internal) {
    if (control == nullptr) return false;
    if (reason == CAP_REASON_WASAPI_ERROR) {
        const HRESULT candidate = FAILED(hresult) ? hresult : E_FAIL;
        HRESULT unset = S_OK;
        (void)control->wasapi_hresult.compare_exchange_strong(
            unset, candidate, std::memory_order_release, std::memory_order_relaxed);
    }
    for (;;) {
        uint64_t current = control->packed.load(std::memory_order_acquire);
        const uint8_t state = PackedState(current);
        if (state == kPrivateStateTerminal || state == kPrivateStateSealed || PackedSealed(current)) return false;
        const int32_t old_reason = static_cast<int32_t>(PackedReason(current));
        uint64_t desired = current;
        if (state < kPrivateStateStopping) {
            desired = PackState(CollapsedPublicStateForPrivate(state), kPrivateStateStopping, false,
                                static_cast<uint16_t>(reason));
        } else if (state == kPrivateStateStopping) {
            if (ReasonPriority(reason) >= ReasonPriority(old_reason)) return false;
            desired = PackState(PackedLastPublicState(current), kPrivateStateStopping, false,
                                static_cast<uint16_t>(reason));
        } else {
            return false;
        }
        if (control->packed.compare_exchange_weak(current, desired, std::memory_order_acq_rel,
                                                  std::memory_order_acquire)) {
            if (!internal) {
                if (state == kPrivateStateCapturing) {
                    if (control->stop_event != nullptr) SetEvent(control->stop_event);
                } else if (control->capture_thread_wake != nullptr) {
                    SetEvent(control->capture_thread_wake);
                }
            }
            return true;
        }
    }
}

int32_t SealCaptureReason(CaptureControl* control) {
    if (control == nullptr) return CAP_REASON_WASAPI_ERROR;
    for (;;) {
        uint64_t current = control->packed.load(std::memory_order_acquire);
        if (PackedState(current) == kPrivateStateSealed && PackedSealed(current)) {
            return static_cast<int32_t>(PackedReason(current));
        }
        if (PackedState(current) != kPrivateStateStopping) {
            (void)InstallCaptureReason(control, CAP_REASON_WASAPI_ERROR, E_FAIL, true);
            continue;
        }
        const uint64_t desired = PackState(PackedLastPublicState(current), kPrivateStateSealed,
                                           true, PackedReason(current));
        if (control->packed.compare_exchange_weak(current, desired, std::memory_order_acq_rel,
                                                  std::memory_order_acquire)) {
            return static_cast<int32_t>(PackedReason(desired));
        }
    }
}

HRESULT SealedCaptureHResult(CaptureControl* control, int32_t reason) {
    if (reason == CAP_REASON_WASAPI_ERROR && control != nullptr) {
        const HRESULT stored = control->wasapi_hresult.load(std::memory_order_acquire);
        return FAILED(stored) ? stored : E_FAIL;
    }
    return HResultForReason(reason, E_FAIL);
}

ActivationCancelPlan PlanActivationCancellation(uint32_t threadDone) {
    ActivationCancelPlan plan;
    plan.callbackPublishes = threadDone == 1;
    plan.callbackSignals = plan.callbackPublishes;
    plan.captureThreadSignals = !plan.callbackPublishes;
    return plan;
}

PermissionNotificationState::~PermissionNotificationState() {
    const uintptr_t value = notify.exchange(0, std::memory_order_acq_rel);
    if (value != 0) CloseHandle(reinterpret_cast<HANDLE>(value));
	if (countedSubscription) g_subscription_states.fetch_sub(1, std::memory_order_acq_rel);
}

void SignalPermissionNotification(const std::shared_ptr<PermissionNotificationState>& state) {
    CallbackScope scope;
    if (!state) return;
#if defined(PULSAR_CAPTURE_STATIC)
    HANDLE entered = reinterpret_cast<HANDLE>(g_test_permission_handler_entered.load(std::memory_order_acquire));
    HANDLE proceed = reinterpret_cast<HANDLE>(g_test_permission_handler_proceed.load(std::memory_order_acquire));
    if (entered != nullptr) SetEvent(entered);
    if (proceed != nullptr) (void)WaitForSingleObject(proceed, INFINITE);
#endif
    HANDLE event = reinterpret_cast<HANDLE>(state->notify.load(std::memory_order_acquire));
    if (event != nullptr) SetEvent(event);
}

void ExecuteCaptureCleanup(CaptureCleanupState* state, CaptureCleanupOps* ops,
                           CaptureCleanupDiagnostics* diagnostics) {
    if (state == nullptr || ops == nullptr || diagnostics == nullptr) return;
    auto record = [&](uint32_t step) {
        if (diagnostics->stepCount < 4) diagnostics->steps[diagnostics->stepCount++] = step;
    };
    if (state->started) {
        diagnostics->stopHResult = ops->Stop();
        state->started = false;
        record(CAP_CLEANUP_STOP);
    }
    if (state->serviceAcquired) {
        ops->ReleaseService();
        state->serviceAcquired = false;
        record(CAP_CLEANUP_RELEASE_SERVICE);
    }
    if (state->mixFormatOwned) {
        ops->FreeMixFormat();
        state->mixFormatOwned = false;
        record(CAP_CLEANUP_FREE_MIX_FORMAT);
    }
    if (state->audioClientOwned) {
        ops->ReleaseClient();
        state->audioClientOwned = false;
        record(CAP_CLEANUP_RELEASE_CLIENT);
    }
}

void ConsumePacketCleanupDiagnostic(const PacketDrainResult& result,
                                    CaptureCleanupDiagnostics* diagnostics) {
    if (diagnostics != nullptr && FAILED(result.cleanupReleaseHResult)) {
        diagnostics->releaseBufferHResult = result.cleanupReleaseHResult;
    }
}

#if defined(PULSAR_CAPTURE_STATIC)
void TestSetPostRoInitFailure(HRESULT hresult) {
    g_test_post_ro_init_failure.store(hresult, std::memory_order_release);
}

void TestFailNextDuplicate(HRESULT hresult) {
    g_test_duplicate_failure.store(hresult, std::memory_order_release);
}

void TestFailNextThreadLaunch() {
    g_test_thread_launch_failure.store(1, std::memory_order_release);
}

void TestFailNextActivationLaunch(HRESULT hresult) {
    g_test_activation_launch_failure.store(hresult, std::memory_order_release);
}

void TestSetCaptureCoInitialize(HRESULT hresult, HANDLE entered, HANDLE proceed) {
    g_test_capture_coinit_result.store(hresult, std::memory_order_release);
    g_test_capture_coinit_entered.store(reinterpret_cast<uintptr_t>(entered), std::memory_order_release);
    g_test_capture_coinit_proceed.store(reinterpret_cast<uintptr_t>(proceed), std::memory_order_release);
}

void TestSetPermissionHandlerBarrier(HANDLE entered, HANDLE proceed) {
    g_test_permission_handler_entered.store(reinterpret_cast<uintptr_t>(entered), std::memory_order_release);
    g_test_permission_handler_proceed.store(reinterpret_cast<uintptr_t>(proceed), std::memory_order_release);
}

void TestEnablePermissionRegistration(bool enabled) {
    g_test_permission_registration.store(enabled ? 1u : 0u, std::memory_order_release);
}

HRESULT TestDispatchPermissionAccessChanged() {
    std::function<void()> dispatch;
    {
        std::lock_guard<std::mutex> lock(g_mutex);
        dispatch = g_test_permission_dispatch;
    }
    if (!dispatch) return E_NOT_VALID_STATE;
    dispatch();
    return S_OK;
}

int64_t TestPermissionTokenValue() {
    std::lock_guard<std::mutex> lock(g_mutex);
    return g_subscription ? g_subscription->token.value : 0;
}

uint32_t TestPermissionRevokeCount() {
    return g_test_permission_revoke_count.load(std::memory_order_acquire);
}

void TestDeferNextActivationCallback() {
    g_test_defer_activation_callback.store(1, std::memory_order_release);
}

HRESULT TestCompleteDeferredActivation(uint32_t opId, HRESULT activateHResult) {
    auto session = lookup_operation<CaptureSession>(opId, OperationKind::Capture);
    if (!session) return E_HANDLE;
    winrt::com_ptr<IActivateAudioInterfaceCompletionHandler> handler;
    {
        std::lock_guard<std::mutex> lock(session->handoff_mutex);
        handler = session->activation_handler;
    }
    if (!handler) return E_NOT_VALID_STATE;
    auto operation = winrt::make_self<TestActivationOperation>(activateHResult);
    auto operation_interface = operation.as<IActivateAudioInterfaceAsyncOperation>();
    return handler->ActivateCompleted(operation_interface.get());
}

void TestSetActivationCallbackBarrier(HANDLE entered, HANDLE proceed) {
    g_test_activation_callback_entered.store(reinterpret_cast<uintptr_t>(entered), std::memory_order_release);
    g_test_activation_callback_proceed.store(reinterpret_cast<uintptr_t>(proceed), std::memory_order_release);
}

void TestSetCaptureAfterWakeBarrier(HANDLE entered, HANDLE proceed) {
    g_test_capture_after_wake_entered.store(reinterpret_cast<uintptr_t>(entered), std::memory_order_release);
    g_test_capture_after_wake_proceed.store(reinterpret_cast<uintptr_t>(proceed), std::memory_order_release);
}

uint32_t TestCaptureThreadDone(uint32_t opId) {
    auto session = lookup_operation<CaptureSession>(opId, OperationKind::Capture);
    return session ? session->thread_done.load(std::memory_order_acquire) : 0;
}

void TestGetNotificationCounts(uint32_t* captureSignals, uint32_t* captureCloses,
                               uint32_t* callbackSignals, uint32_t* callbackCloses) {
    if (captureSignals != nullptr) *captureSignals = g_test_capture_notify_signals.load(std::memory_order_acquire);
    if (captureCloses != nullptr) *captureCloses = g_test_capture_notify_closes.load(std::memory_order_acquire);
    if (callbackSignals != nullptr) *callbackSignals = g_test_callback_notify_signals.load(std::memory_order_acquire);
    if (callbackCloses != nullptr) *callbackCloses = g_test_callback_notify_closes.load(std::memory_order_acquire);
}

HRESULT TestRouteCaptureDiagnostics(uint32_t opId, const PacketDrainResult* packet,
                                    HRESULT stopHResult) {
    auto session = lookup_operation<CaptureSession>(opId, OperationKind::Capture);
    if (!session) return E_HANDLE;
    if (packet == nullptr) return E_POINTER;
    consume_capture_packet_evidence(session.get(), *packet);
    CaptureCleanupDiagnostics cleanup;
    cleanup.stopHResult = stopHResult;
    consume_capture_cleanup_evidence(session.get(), cleanup);
    return S_OK;
}

HRESULT TestHoldCaptureHandoff(uint32_t opId, HANDLE entered, HANDLE proceed) {
    auto session = lookup_operation<CaptureSession>(opId, OperationKind::Capture);
    if (!session) return E_HANDLE;
    std::lock_guard<std::mutex> lock(session->handoff_mutex);
    if (entered != nullptr && !SetEvent(entered)) return HRESULT_FROM_WIN32(GetLastError());
    if (proceed != nullptr) {
        const DWORD wait = WaitForSingleObject(proceed, INFINITE);
        if (wait != WAIT_OBJECT_0) return HRESULT_FROM_WIN32(GetLastError());
    }
    return S_OK;
}

uint32_t TestRoUninitializeCount() {
    return g_test_ro_uninitialize_count.load(std::memory_order_acquire);
}

void TestResetNativeHooks() {
    g_test_post_ro_init_failure.store(S_OK, std::memory_order_release);
    g_test_duplicate_failure.store(S_OK, std::memory_order_release);
    g_test_thread_launch_failure.store(0, std::memory_order_release);
    g_test_activation_launch_failure.store(S_OK, std::memory_order_release);
    g_test_capture_coinit_result.store(S_OK, std::memory_order_release);
    g_test_capture_coinit_entered.store(0, std::memory_order_release);
    g_test_capture_coinit_proceed.store(0, std::memory_order_release);
    g_test_permission_handler_entered.store(0, std::memory_order_release);
    g_test_permission_handler_proceed.store(0, std::memory_order_release);
    g_test_ro_uninitialize_count.store(0, std::memory_order_release);
    g_test_permission_registration.store(0, std::memory_order_release);
    g_test_permission_revoke_count.store(0, std::memory_order_release);
    {
        std::lock_guard<std::mutex> lock(g_mutex);
        g_test_permission_dispatch = {};
    }
    g_test_defer_activation_callback.store(0, std::memory_order_release);
    g_test_activation_callback_entered.store(0, std::memory_order_release);
    g_test_activation_callback_proceed.store(0, std::memory_order_release);
    g_test_capture_after_wake_entered.store(0, std::memory_order_release);
    g_test_capture_after_wake_proceed.store(0, std::memory_order_release);
    g_test_capture_notify_signals.store(0, std::memory_order_release);
    g_test_capture_notify_closes.store(0, std::memory_order_release);
    g_test_callback_notify_signals.store(0, std::memory_order_release);
    g_test_callback_notify_closes.store(0, std::memory_order_release);
}
#endif

uint32_t FindAvailableOperationId(uint32_t start, const std::vector<uint32_t>& occupied, uint64_t occupiedCount) {
    if (occupiedCount >= UINT32_MAX) return 0;
    uint32_t candidate = start == 0 ? 1 : start;
    const uint32_t first = candidate;
    do {
        if (std::find(occupied.begin(), occupied.end(), candidate) == occupied.end()) return candidate;
        candidate = candidate == UINT32_MAX ? 1 : candidate + 1;
    } while (candidate != first);
    return 0;
}

HRESULT DuplicateSignalHandle(HANDLE source, HANDLE* duplicate) {
    if (duplicate == nullptr) return E_POINTER;
    *duplicate = nullptr;
#if defined(PULSAR_CAPTURE_STATIC)
    const HRESULT injected = g_test_duplicate_failure.exchange(S_OK, std::memory_order_acq_rel);
    if (FAILED(injected)) return injected;
#endif
    HRESULT valid = validate_event(source);
    if (FAILED(valid)) return valid;
    if (!DuplicateHandle(GetCurrentProcess(), source, GetCurrentProcess(), duplicate, 0, FALSE, DUPLICATE_SAME_ACCESS)) {
        return HRESULT_FROM_WIN32(GetLastError());
    }
    return S_OK;
}

HRESULT ValidateAndFillCaptureFormat(const WAVEFORMATEX* src, CaptureFormat* dst, std::wstring* diagnostic) {
    if (src == nullptr || dst == nullptr) return E_POINTER;
    if (src->nChannels == 0 || src->nChannels > kMaxChannels ||
        src->nSamplesPerSec == 0 || src->nSamplesPerSec > kMaxSampleRate) return E_INVALIDARG;
    if (src->wBitsPerSample == 0 || (src->wBitsPerSample % 8) != 0) return E_INVALIDARG;
    const uint32_t bytes = src->wBitsPerSample / 8;
    if (src->nBlockAlign != static_cast<uint32_t>(src->nChannels) * bytes) return E_INVALIDARG;

    uint32_t subtype = 0;
    uint32_t valid_bits = src->wBitsPerSample;
    uint32_t channel_mask = 0;
    if (src->wFormatTag == WAVE_FORMAT_EXTENSIBLE) {
        if (src->cbSize < 22) return E_INVALIDARG;
        auto const* ext = reinterpret_cast<WAVEFORMATEXTENSIBLE const*>(src);
        valid_bits = ext->Samples.wValidBitsPerSample == 0 ? src->wBitsPerSample : ext->Samples.wValidBitsPerSample;
        channel_mask = ext->dwChannelMask;
        if (IsEqualGUID(ext->SubFormat, KSDATAFORMAT_SUBTYPE_PCM)) subtype = 1;
        else if (IsEqualGUID(ext->SubFormat, KSDATAFORMAT_SUBTYPE_IEEE_FLOAT)) subtype = 3;
    } else if (src->wFormatTag == WAVE_FORMAT_PCM) {
        subtype = 1;
    } else if (src->wFormatTag == WAVE_FORMAT_IEEE_FLOAT) {
        subtype = 3;
    }
    if (valid_bits == 0 || valid_bits > src->wBitsPerSample) return E_INVALIDARG;
    bool supported = false;
    if (subtype == 3) supported = src->wBitsPerSample == 32 && valid_bits == 32;
    if (subtype == 1) {
        supported = (src->wBitsPerSample == 16 && valid_bits == 16) ||
                    (src->wBitsPerSample == 24 && valid_bits == 24) ||
                    (src->wBitsPerSample == 32 && (valid_bits == 24 || valid_bits == 32));
    }
    if (!supported) {
        if (diagnostic != nullptr) *diagnostic = L"unsupported capture subtype/container/valid-bits combination";
        return E_INVALIDARG;
    }
    *dst = {};
    dst->structSize = sizeof(CaptureFormat);
    dst->version = kCaptureFormatVersion;
    dst->valid = 1;
    dst->sampleRate = src->nSamplesPerSec;
    dst->channels = src->nChannels;
    dst->bitsPerSample = 32;
    dst->validBits = valid_bits;
    dst->channelMask = channel_mask;
    dst->nativeSubtype = subtype;
    dst->nativeBits = src->wBitsPerSample;
    dst->nativeValidBits = valid_bits;
    dst->nBlockAlign = src->nBlockAlign;
    return S_OK;
}

HRESULT ConvertFramesToFloat32(const WAVEFORMATEX*, const CaptureFormat& fmt, const BYTE* input,
                               uint32_t frames, float* output) {
    if ((input == nullptr && frames != 0) || (output == nullptr && frames != 0)) return E_POINTER;
    if (fmt.channels == 0 || fmt.channels > kMaxChannels) return E_INVALIDARG;
    const uint64_t samples = static_cast<uint64_t>(frames) * fmt.channels;
    if (samples > SIZE_MAX) return E_OUTOFMEMORY;
    for (uint64_t i = 0; i < samples; ++i) {
        const BYTE* ptr = input + i * (fmt.nativeBits / 8);
        if (fmt.nativeSubtype == 3 && fmt.nativeBits == 32) {
            std::memcpy(&output[i], ptr, 4);
        } else if (fmt.nativeSubtype == 1 && fmt.nativeBits == 16 && fmt.nativeValidBits == 16) {
            int16_t value;
            std::memcpy(&value, ptr, 2);
            output[i] = static_cast<float>(value) / 32768.0f;
        } else if (fmt.nativeSubtype == 1 && fmt.nativeBits == 24 && fmt.nativeValidBits == 24) {
            const uint32_t u = static_cast<uint32_t>(ptr[2]) << 16 |
                               static_cast<uint32_t>(ptr[1]) << 8 |
                               static_cast<uint32_t>(ptr[0]);
            int32_t value = static_cast<int32_t>(u);
            if (u >= 0x800000u) value -= 0x1000000;
            output[i] = static_cast<float>(value) / 8388608.0f;
        } else if (fmt.nativeSubtype == 1 && fmt.nativeBits == 32 && fmt.nativeValidBits == 24) {
            uint32_t raw;
            std::memcpy(&raw, ptr, 4);
            const uint32_t u = raw >> 8;
            int32_t value = static_cast<int32_t>(u);
            if (u >= 0x800000u) value -= 0x1000000;
            output[i] = static_cast<float>(value) / 8388608.0f;
        } else if (fmt.nativeSubtype == 1 && fmt.nativeBits == 32 && fmt.nativeValidBits == 32) {
            int32_t value;
            std::memcpy(&value, ptr, 4);
            output[i] = static_cast<float>(value) / 2147483648.0f;
        } else {
            return E_INVALIDARG;
        }
    }
    return S_OK;
}

HRESULT ComputeCaptureAllocation(uint32_t sample_rate, uint32_t channels, uint32_t buffer_frames,
                                 uint32_t* ring_frames, size_t* scratch_samples) {
    if (ring_frames == nullptr || scratch_samples == nullptr) return E_POINTER;
    if (sample_rate == 0 || sample_rate > kMaxSampleRate || channels == 0 || channels > kMaxChannels ||
        buffer_frames == 0 || buffer_frames > kMaxBufferFrames) return E_INVALIDARG;
    const uint64_t candidate_frames = std::max<uint64_t>(static_cast<uint64_t>(sample_rate) * 2u, buffer_frames);
    const uint64_t ring_samples = candidate_frames * channels;
    const uint64_t packet_samples = static_cast<uint64_t>(buffer_frames) * channels;
    if (candidate_frames > UINT32_MAX || ring_samples > SIZE_MAX / sizeof(float) ||
        packet_samples > SIZE_MAX / sizeof(float)) return E_OUTOFMEMORY;
    *ring_frames = static_cast<uint32_t>(candidate_frames);
    *scratch_samples = static_cast<size_t>(packet_samples);
    return S_OK;
}

HRESULT DrainCapturePackets(PacketSource* source, const CaptureFormat& format, FrameRing* ring,
                            std::vector<float>* scratch, bool* firstPacket, StopCheck stopCheck,
                            void* stopContext, PacketDrainResult* result) {
    if (source == nullptr || ring == nullptr || scratch == nullptr || firstPacket == nullptr || result == nullptr) {
        return E_POINTER;
    }
    *result = {};
    for (;;) {
        uint32_t packet_size = 0;
        HRESULT hr = source->GetNextPacketSize(&packet_size);
        if (FAILED(hr)) {
            result->terminalReason = classify_audio_error(hr);
            result->terminalHResult = hr;
            return hr;
        }
        if (packet_size == 0) return S_OK;

        BYTE* data = nullptr;
        uint32_t frames = 0;
        DWORD flags = 0;
        hr = source->GetBuffer(&data, &frames, &flags);
        if (FAILED(hr)) {
            result->terminalReason = classify_audio_error(hr);
            result->terminalHResult = hr;
            return hr;
        }
        auto release_for_original = [&](int32_t reason, HRESULT original) {
            result->cleanupReleaseHResult = source->ReleaseBuffer(frames);
            result->terminalReason = reason;
            result->terminalHResult = original;
            return original;
        };
        if (stopCheck != nullptr && stopCheck(stopContext)) {
            result->cleanupReleaseHResult = source->ReleaseBuffer(frames);
            result->stopObserved = true;
            return S_OK;
        }
        if ((flags & AUDCLNT_BUFFERFLAGS_DATA_DISCONTINUITY) != 0 && !*firstPacket) {
            return release_for_original(CAP_REASON_DISCONTINUITY, kDiscontinuityHr);
        }
        if ((flags & AUDCLNT_BUFFERFLAGS_TIMESTAMP_ERROR) != 0) {
            ++result->timestampErrorCount;
        }
        *firstPacket = false;
        if (!ring->HasSpaceFor(frames)) {
            return release_for_original(CAP_REASON_OVERFLOW, kOverflowHr);
        }
        const uint64_t required = static_cast<uint64_t>(frames) * format.channels;
        if (required > scratch->size()) {
            return release_for_original(CAP_REASON_FORMAT_ERROR, E_INVALIDARG);
        }
        if ((flags & AUDCLNT_BUFFERFLAGS_SILENT) != 0) {
            std::fill_n(scratch->data(), static_cast<size_t>(required), 0.0f);
        } else {
            hr = ConvertFramesToFloat32(nullptr, format, data, frames, scratch->data());
            if (FAILED(hr)) return release_for_original(CAP_REASON_FORMAT_ERROR, E_INVALIDARG);
        }
        if (stopCheck != nullptr && stopCheck(stopContext)) {
            result->cleanupReleaseHResult = source->ReleaseBuffer(frames);
            result->stopObserved = true;
            return S_OK;
        }
        hr = source->ReleaseBuffer(frames);
        if (FAILED(hr)) {
            result->terminalReason = classify_audio_error(hr);
            result->terminalHResult = hr;
            return hr;
        }
        hr = ring->Write(scratch->data(), frames);
        if (FAILED(hr)) {
            result->terminalReason = CAP_REASON_OVERFLOW;
            result->terminalHResult = kOverflowHr;
            return kOverflowHr;
        }
        ++result->packetsCommitted;
    }
}

HRESULT QueryPickerResult(PickerResultCore* result, int32_t takeHandle, int32_t* state,
                          HANDLE* fileHandle, int32_t* handleTaken, int64_t* fileSize,
                          wchar_t* nameBuf, int32_t nameBufLen, int32_t* requiredNameChars,
                          HRESULT* hresult) {
    if (result == nullptr) return E_POINTER;
    if (takeHandle != 0 && takeHandle != 1) return E_INVALIDARG;
    if (state == nullptr || handleTaken == nullptr || hresult == nullptr ||
        (takeHandle == 1 && fileHandle == nullptr)) return E_POINTER;
    if (result->state == 0) {
        *state = 0;
        return S_FALSE;
    }
    *state = result->state;
    *hresult = result->outcome;
    *handleTaken = 0;
    if (fileHandle != nullptr) *fileHandle = INVALID_HANDLE_VALUE;
    if (result->state != 1) return S_OK;
    if (fileSize != nullptr) *fileSize = result->fileSize;
    (void)copy_string(result->displayName, nameBuf, nameBufLen, requiredNameChars, false);
    if (takeHandle == 1 && !result->handleTaken && result->fileHandle != INVALID_HANDLE_VALUE) {
        *fileHandle = result->fileHandle;
        result->fileHandle = INVALID_HANDLE_VALUE;
        result->handleTaken = true;
        *handleTaken = 1;
    }
    return S_OK;
}

void CloseUntakenPickerHandle(PickerResultCore* result) {
    if (result != nullptr && result->fileHandle != INVALID_HANDLE_VALUE) {
        CloseHandle(result->fileHandle);
        result->fileHandle = INVALID_HANDLE_VALUE;
    }
}

FrameRing::FrameRing() : channels_(0), capacity_frames_(0), read_frame_(0), write_frame_(0) {}

HRESULT FrameRing::Reset(uint32_t channels, uint32_t capacity_frames) {
    if (channels == 0 || channels > kMaxChannels || capacity_frames == 0) return E_INVALIDARG;
    const uint64_t samples = static_cast<uint64_t>(channels) * capacity_frames;
    if (samples > SIZE_MAX / sizeof(float)) return E_OUTOFMEMORY;
    try { data_.assign(static_cast<size_t>(samples), 0.0f); }
    catch (...) { return E_OUTOFMEMORY; }
    channels_ = channels;
    capacity_frames_ = capacity_frames;
    read_frame_.store(0, std::memory_order_relaxed);
    write_frame_.store(0, std::memory_order_relaxed);
    return S_OK;
}

bool FrameRing::HasSpaceFor(uint32_t frames) const {
    const uint64_t write = write_frame_.load(std::memory_order_relaxed);
    const uint64_t read = read_frame_.load(std::memory_order_acquire);
    return write - read <= capacity_frames_ && frames <= capacity_frames_ - (write - read);
}

HRESULT FrameRing::Write(const float* src, uint32_t frames) {
    if ((src == nullptr && frames != 0) || !HasSpaceFor(frames)) return kOverflowHr;
    const uint64_t write = write_frame_.load(std::memory_order_relaxed);
    for (uint32_t frame = 0; frame < frames; ++frame) {
        const size_t dest = static_cast<size_t>((write + frame) % capacity_frames_) * channels_;
        std::memcpy(&data_[dest], src + static_cast<size_t>(frame) * channels_, channels_ * sizeof(float));
    }
    write_frame_.store(write + frames, std::memory_order_release);
    return S_OK;
}

uint32_t FrameRing::Read(float* dst, uint32_t max_frames) {
    if (dst == nullptr || max_frames == 0) return 0;
    const uint64_t read = read_frame_.load(std::memory_order_relaxed);
    const uint64_t write = write_frame_.load(std::memory_order_acquire);
    const uint32_t frames = static_cast<uint32_t>(std::min<uint64_t>(max_frames, write - read));
    for (uint32_t frame = 0; frame < frames; ++frame) {
        const size_t src = static_cast<size_t>((read + frame) % capacity_frames_) * channels_;
        std::memcpy(dst + static_cast<size_t>(frame) * channels_, &data_[src], channels_ * sizeof(float));
    }
    read_frame_.store(read + frames, std::memory_order_release);
    return frames;
}

uint32_t FrameRing::Available() const {
    return static_cast<uint32_t>(write_frame_.load(std::memory_order_acquire) - read_frame_.load(std::memory_order_acquire));
}
uint32_t FrameRing::Capacity() const { return capacity_frames_; }
uint32_t FrameRing::Channels() const { return channels_; }

}  // namespace pulsar_capture

using namespace pulsar_capture;

extern "C" {

HRESULT __stdcall CapGetVersion(uint32_t* version, uint32_t* structHeaderSize) {
    if (version == nullptr || structHeaderSize == nullptr) return E_POINTER;
    *version = kHelperAbiVersion;
    *structHeaderSize = sizeof(CaptureFormat);
    return S_OK;
}

HRESULT __stdcall CapInit(void) {
    try {
        std::lock_guard<std::mutex> lock(g_mutex);
        if (g_initialized) return E_NOT_VALID_STATE;
        HRESULT hr = RoInitialize(RO_INIT_SINGLETHREADED);
        if (hr != S_OK && hr != S_FALSE) return hr;
        g_ro_initialized = true;
#if defined(PULSAR_CAPTURE_STATIC)
        const HRESULT injected = g_test_post_ro_init_failure.load(std::memory_order_acquire);
        if (FAILED(injected)) {
            balanced_ro_uninitialize();
            g_ro_initialized = false;
            return injected;
        }
#endif
        g_runtime_state = std::make_unique<uint8_t>();
        try {
            g_microphone_capability = AppCapability::Create(L"microphone");
            g_capability_hr = g_microphone_capability ? S_OK : E_FAIL;
        } catch (...) {
            g_microphone_capability = nullptr;
            g_capability_hr = exception_hr();
        }
        g_init_thread = GetCurrentThreadId();
        g_initialized = true;
        return S_OK;
    } catch (...) {
        if (g_ro_initialized) {
            balanced_ro_uninitialize();
            g_ro_initialized = false;
        }
		g_runtime_state.reset();
        g_init_thread = 0;
        g_initialized = false;
        return exception_hr();
    }
}

HRESULT __stdcall CapPermissionCheck(int32_t* status) {
    if (status == nullptr) return E_POINTER;
    HRESULT initialized = require_initialized();
    if (FAILED(initialized)) return initialized;
    try {
        DWORD init_thread = 0;
        {
            std::lock_guard<std::mutex> lock(g_mutex);
            init_thread = g_init_thread;
        }
        std::unique_ptr<ScopedMTA> apartment;
        if (GetCurrentThreadId() != init_thread) {
            apartment = std::make_unique<ScopedMTA>();
            if (FAILED(apartment->result) && apartment->result != RPC_E_CHANGED_MODE) return apartment->result;
        }
        // AppCapability is apartment-bound on Windows 10. The instance cached
        // by CapInit belongs to the UI STA and must never cross into the
        // locked waiter thread. Create and consume a short-lived capability in
        // the calling apartment instead; subscription keeps its UI-owned
        // instance and RequestAccessAsync is still initiated on the UI thread.
        AppCapability capability = AppCapability::Create(L"microphone");
        if (!capability) {
            *status = CAP_PERMISSION_UNAVAILABLE;
            return S_OK;
        }
        *status = map_permission(capability.CheckAccess());
        return S_OK;
    } catch (...) {
        *status = CAP_PERMISSION_UNKNOWN;
        return exception_hr();
    }
}

HRESULT __stdcall CapPermissionRequest(HANDLE notifyEvent, uint32_t* opId) {
    if (opId == nullptr) return E_POINTER;
    HRESULT initialized = require_initialized(true);
    if (FAILED(initialized)) return initialized;
    HRESULT valid = validate_event(notifyEvent);
    if (FAILED(valid)) return valid;
    try {
        auto op = std::make_shared<PermissionOperation>();
        HANDLE duplicate = nullptr;
        HRESULT hr = DuplicateSignalHandle(notifyEvent, &duplicate);
        if (FAILED(hr)) return hr;
        op->notify.store(reinterpret_cast<uintptr_t>(duplicate), std::memory_order_release);
        AppCapability capability{nullptr};
        {
            std::lock_guard<std::mutex> lock(g_mutex);
            if (has_kind_locked(OperationKind::Permission)) return E_NOT_VALID_STATE;
            const uint32_t id = allocate_id_locked();
            if (id == 0) return E_OUTOFMEMORY;
            op->id = id;
            g_operations.emplace(id, op);
            capability = g_microphone_capability;
            *opId = id;
        }
        if (!capability) {
            op->status = CAP_PERMISSION_UNAVAILABLE;
            complete_and_signal(op, 2, FAILED(g_capability_hr) ? g_capability_hr : E_FAIL);
            return S_OK;
        }
        try {
            op->async = capability.RequestAccessAsync();
            op->async.Completed([op](auto const& operation, AsyncStatus async_status) {
                CallbackScope scope;
                try {
                    std::lock_guard<std::mutex> lock(op->mutex);
                    if (async_status == AsyncStatus::Canceled) {
                        op->status = CAP_PERMISSION_UNKNOWN;
                        op->outcome = kCancelledHr;
                        op->async = nullptr;
                        op->state.store(3, std::memory_order_release);
                    } else if (async_status == AsyncStatus::Completed) {
                        op->status = map_permission(operation.GetResults());
                        op->outcome = S_OK;
                        op->async = nullptr;
                        op->state.store(1, std::memory_order_release);
                    } else {
                        op->outcome = operation.ErrorCode();
                        op->async = nullptr;
                        op->state.store(2, std::memory_order_release);
                    }
                } catch (...) {
                    op->outcome = exception_hr();
                    op->async = nullptr;
                    op->state.store(2, std::memory_order_release);
                }
                HANDLE event = op->take_notify();
                if (event != nullptr) { SetEvent(event); CloseHandle(event); }
            });
        } catch (...) {
            complete_and_signal(op, 2, exception_hr());
        }
        return S_OK;
    } catch (...) { return exception_hr(); }
}

HRESULT __stdcall CapPermissionRequestResult(uint32_t opId, int32_t* state, int32_t* status, HRESULT* hresult) {
    if (state == nullptr || status == nullptr || hresult == nullptr) return E_POINTER;
    auto op = lookup_operation<PermissionOperation>(opId, OperationKind::Permission);
    if (!op) return E_HANDLE;
    *state = op->state.load(std::memory_order_acquire);
    if (*state == 0) return S_FALSE;
    std::lock_guard<std::mutex> lock(op->mutex);
    *status = op->status;
    *hresult = op->outcome;
    return S_OK;
}

HRESULT __stdcall CapPermissionRequestCancel(uint32_t opId) {
    auto op = lookup_operation<PermissionOperation>(opId, OperationKind::Permission);
    if (!op) return E_HANDLE;
    if (op->state.load(std::memory_order_acquire) != 0) return E_NOT_VALID_STATE;
    winrt::Windows::Foundation::IAsyncOperation<AppCapabilityAccessStatus> async{nullptr};
    { std::lock_guard<std::mutex> lock(op->mutex); async = op->async; }
    return cancel_async(async);
}

HRESULT __stdcall CapPermissionRequestRelease(uint32_t opId) {
    std::lock_guard<std::mutex> lock(g_mutex);
    auto it = g_operations.find(opId);
    if (it == g_operations.end()) return S_OK;
    if (it->second->kind != OperationKind::Permission) return E_HANDLE;
    if (it->second->state.load(std::memory_order_acquire) == 0) return E_ILLEGAL_METHOD_CALL;
    g_operations.erase(it);
    return S_OK;
}

HRESULT __stdcall CapPermissionSubscribe(HANDLE notifyEvent) {
    HRESULT initialized = require_initialized(true);
    if (FAILED(initialized)) return initialized;
    HRESULT valid = validate_event(notifyEvent);
    if (FAILED(valid)) return valid;
    try {
        std::lock_guard<std::mutex> lock(g_mutex);
        if (g_subscription) return E_NOT_VALID_STATE;
        bool test_registration = false;
#if defined(PULSAR_CAPTURE_STATIC)
        test_registration = g_test_permission_registration.load(std::memory_order_acquire) != 0;
        if (!test_registration && !g_microphone_capability) {
            return FAILED(g_capability_hr) ? g_capability_hr : E_FAIL;
        }
#else
        if (!g_microphone_capability) return FAILED(g_capability_hr) ? g_capability_hr : E_FAIL;
#endif
        HANDLE duplicate = nullptr;
        HRESULT hr = DuplicateSignalHandle(notifyEvent, &duplicate);
        if (FAILED(hr)) return hr;
        auto sub = std::make_shared<PermissionSubscription>();
        sub->notify.store(reinterpret_cast<uintptr_t>(duplicate), std::memory_order_release);
        if (!test_registration) sub->capability = g_microphone_capability;
        hr = register_permission_access_changed(sub);
        if (FAILED(hr)) return hr;
        g_subscription = std::move(sub);
        return S_OK;
    } catch (...) { return exception_hr(); }
}

HRESULT __stdcall CapPermissionUnsubscribe(void) {
    std::shared_ptr<PermissionSubscription> sub;
    {
        std::lock_guard<std::mutex> lock(g_mutex);
        sub = g_subscription;
    }
    if (!sub) return S_OK;
    HRESULT revoked = revoke_permission_access_changed(sub);
    if (FAILED(revoked)) return revoked;
    {
        std::lock_guard<std::mutex> lock(g_mutex);
        if (g_subscription == sub) g_subscription.reset();
    }
    return S_OK;
}

HRESULT __stdcall CapEnumerateDevices(HANDLE notifyEvent, uint32_t* opId) {
    if (opId == nullptr) return E_POINTER;
    HRESULT initialized = require_initialized(true);
    if (FAILED(initialized)) return initialized;
    HRESULT valid = validate_event(notifyEvent);
    if (FAILED(valid)) return valid;
    try {
        auto op = std::make_shared<EnumerationOperation>();
        HANDLE duplicate = nullptr;
        HRESULT hr = DuplicateSignalHandle(notifyEvent, &duplicate);
        if (FAILED(hr)) return hr;
        op->notify.store(reinterpret_cast<uintptr_t>(duplicate), std::memory_order_release);
        {
            std::lock_guard<std::mutex> lock(g_mutex);
            if (has_kind_locked(OperationKind::Enumeration)) return E_NOT_VALID_STATE;
            const uint32_t id = allocate_id_locked();
            if (id == 0) return E_OUTOFMEMORY;
            op->id = id;
            g_operations.emplace(id, op);
            *opId = id;
        }
        try {
            op->async = winrt::Windows::Devices::Enumeration::DeviceInformation::FindAllAsync(
                winrt::Windows::Devices::Enumeration::DeviceClass::AudioCapture);
            op->async.Completed([op](auto const& operation, AsyncStatus async_status) {
                CallbackScope scope;
                try {
                    std::lock_guard<std::mutex> lock(op->mutex);
                    if (async_status == AsyncStatus::Canceled) {
                        op->outcome = kCancelledHr;
                        op->state.store(3, std::memory_order_release);
                    } else if (async_status == AsyncStatus::Completed) {
                        auto results = operation.GetResults();
                        const uint32_t count = std::min<uint32_t>(results.Size(), kMaxDevices);
                        op->devices.reserve(count);
                        for (uint32_t i = 0; i < count; ++i) {
                            auto info = results.GetAt(i);
                            std::wstring id(info.Id().c_str());
                            std::wstring name(info.Name().c_str());
                            if (id.size() >= kMaxDeviceStringChars) id.resize(kMaxDeviceStringChars - 1);
                            if (name.size() >= kMaxDeviceStringChars) name.resize(kMaxDeviceStringChars - 1);
                            op->devices.push_back({std::move(id), std::move(name)});
                        }
                        op->outcome = S_OK;
                        op->state.store(1, std::memory_order_release);
                    } else {
                        op->outcome = operation.ErrorCode();
                        op->state.store(2, std::memory_order_release);
                    }
                    op->async = nullptr;
                } catch (...) {
                    op->outcome = exception_hr();
                    op->async = nullptr;
                    op->state.store(2, std::memory_order_release);
                }
                HANDLE event = op->take_notify();
                if (event != nullptr) { SetEvent(event); CloseHandle(event); }
            });
        } catch (...) { complete_and_signal(op, 2, exception_hr()); }
        return S_OK;
    } catch (...) { return exception_hr(); }
}

HRESULT __stdcall CapEnumerateDevicesResult(uint32_t opId, int32_t* state, int32_t* count, HRESULT* hresult) {
    if (state == nullptr || count == nullptr || hresult == nullptr) return E_POINTER;
    auto op = lookup_operation<EnumerationOperation>(opId, OperationKind::Enumeration);
    if (!op) return E_HANDLE;
    *state = op->state.load(std::memory_order_acquire);
    if (*state == 0) return S_FALSE;
    std::lock_guard<std::mutex> lock(op->mutex);
    *count = static_cast<int32_t>(op->devices.size());
    *hresult = op->outcome;
    return S_OK;
}

HRESULT __stdcall CapGetDeviceInfo(uint32_t opId, int32_t index, wchar_t* idBuf, int32_t idBufLen,
                                   wchar_t* nameBuf, int32_t nameBufLen) {
    if (idBuf == nullptr || nameBuf == nullptr) return E_POINTER;
    auto op = lookup_operation<EnumerationOperation>(opId, OperationKind::Enumeration);
    if (!op) return E_HANDLE;
    if (op->state.load(std::memory_order_acquire) != 1) return E_ILLEGAL_METHOD_CALL;
    std::lock_guard<std::mutex> lock(op->mutex);
    if (index < 0 || static_cast<size_t>(index) >= op->devices.size()) return E_INVALIDARG;
    HRESULT id_hr = copy_string(op->devices[index].id, idBuf, idBufLen, nullptr, true);
    HRESULT name_hr = copy_string(op->devices[index].name, nameBuf, nameBufLen, nullptr, true);
    return FAILED(id_hr) ? id_hr : name_hr;
}

HRESULT __stdcall CapEnumerateDevicesCancel(uint32_t opId) {
    auto op = lookup_operation<EnumerationOperation>(opId, OperationKind::Enumeration);
    if (!op) return E_HANDLE;
    if (op->state.load(std::memory_order_acquire) != 0) return E_NOT_VALID_STATE;
    winrt::Windows::Foundation::IAsyncOperation<winrt::Windows::Devices::Enumeration::DeviceInformationCollection> async{nullptr};
    { std::lock_guard<std::mutex> lock(op->mutex); async = op->async; }
    return cancel_async(async);
}

HRESULT __stdcall CapEnumerateDevicesRelease(uint32_t opId) {
    std::lock_guard<std::mutex> lock(g_mutex);
    auto it = g_operations.find(opId);
    if (it == g_operations.end()) return S_OK;
    if (it->second->kind != OperationKind::Enumeration) return E_HANDLE;
    if (it->second->state.load(std::memory_order_acquire) == 0) return E_ILLEGAL_METHOD_CALL;
    g_operations.erase(it);
    return S_OK;
}

HRESULT __stdcall CapGetDefaultDevice(int32_t role, HANDLE notifyEvent, uint32_t* opId) {
    if (opId == nullptr) return E_POINTER;
    if (role != 0 && role != 1) return E_INVALIDARG;
    HRESULT initialized = require_initialized(true);
    if (FAILED(initialized)) return initialized;
    HRESULT valid = validate_event(notifyEvent);
    if (FAILED(valid)) return valid;
    try {
        auto op = std::make_shared<DefaultDeviceOperation>();
        HANDLE duplicate = nullptr;
        HRESULT hr = DuplicateSignalHandle(notifyEvent, &duplicate);
        if (FAILED(hr)) return hr;
        op->notify.store(reinterpret_cast<uintptr_t>(duplicate), std::memory_order_release);
        {
            std::lock_guard<std::mutex> lock(g_mutex);
            if (has_kind_locked(OperationKind::DefaultDevice)) return E_NOT_VALID_STATE;
            const uint32_t id = allocate_id_locked();
            if (id == 0) return E_OUTOFMEMORY;
            op->id = id;
            g_operations.emplace(id, op);
            *opId = id;
        }
        try {
            auto device_role = role == 0 ? winrt::Windows::Media::Devices::AudioDeviceRole::Default
                                         : winrt::Windows::Media::Devices::AudioDeviceRole::Communications;
            auto device_id = winrt::Windows::Media::Devices::MediaDevice::GetDefaultAudioCaptureId(device_role);
            op->device_id = device_id.c_str();
            complete_and_signal(op, 1, S_OK);
        } catch (...) { complete_and_signal(op, 2, exception_hr()); }
        return S_OK;
    } catch (...) { return exception_hr(); }
}

HRESULT __stdcall CapGetDefaultDeviceResult(uint32_t opId, int32_t* state, wchar_t* buf, int32_t bufLen,
                                            int32_t* written, HRESULT* hresult) {
    if (state == nullptr || written == nullptr || hresult == nullptr) return E_POINTER;
    auto op = lookup_operation<DefaultDeviceOperation>(opId, OperationKind::DefaultDevice);
    if (!op) return E_HANDLE;
    *state = op->state.load(std::memory_order_acquire);
    if (*state == 0) return S_FALSE;
    std::lock_guard<std::mutex> lock(op->mutex);
    *hresult = op->outcome;
    if (*state != 1) { *written = 0; return S_OK; }
    return copy_string(op->device_id, buf, bufLen, written, true);
}

HRESULT __stdcall CapGetDefaultDeviceRelease(uint32_t opId) {
    std::lock_guard<std::mutex> lock(g_mutex);
    auto it = g_operations.find(opId);
    if (it == g_operations.end()) return S_OK;
    if (it->second->kind != OperationKind::DefaultDevice) return E_HANDLE;
    if (it->second->state.load(std::memory_order_acquire) == 0) return E_ILLEGAL_METHOD_CALL;
    g_operations.erase(it);
    return S_OK;
}

HRESULT __stdcall CapturePrepare(HANDLE notifyEvent, uint32_t* opId) {
    if (opId == nullptr) return E_POINTER;
    HRESULT initialized = require_initialized(true);
    if (FAILED(initialized)) return initialized;
    HRESULT valid = validate_event(notifyEvent);
    if (FAILED(valid)) return valid;
    try {
        auto session = std::make_shared<CaptureSession>();
        uint32_t id = 0;
        decltype(g_operations)::node_type registry_node;
        {
            std::lock_guard<std::mutex> lock(g_mutex);
            if (has_kind_locked(OperationKind::Capture)) return E_NOT_VALID_STATE;
            id = allocate_id_locked();
            if (id == 0) return E_OUTOFMEMORY;

            // Preallocate the registry node before duplicating a handle or
            // starting a thread. Insertion after launch then cannot allocate.
            decltype(g_operations) reservation;
            reservation.emplace(id, std::static_pointer_cast<Operation>(session));
            registry_node = reservation.extract(id);
        }
        session->id = id;
        session->original_notify = notifyEvent;
        session->capture_thread_wake = CreateEventW(nullptr, FALSE, FALSE, nullptr);
        session->capture_data_event = CreateEventW(nullptr, FALSE, FALSE, nullptr);
        session->stop_event = CreateEventW(nullptr, TRUE, FALSE, nullptr);
        if (!session->capture_thread_wake || !session->capture_data_event || !session->stop_event) {
            return HRESULT_FROM_WIN32(GetLastError());
        }
        HANDLE duplicate = nullptr;
        HRESULT hr = DuplicateSignalHandle(notifyEvent, &duplicate);
        if (FAILED(hr)) return hr;
        session->capture_notify.store(reinterpret_cast<uintptr_t>(duplicate), std::memory_order_release);

        unsigned thread_id = 0;
        errno = 0;
        (void)_set_doserrno(0);
        const uintptr_t thread = launch_capture_thread(session.get(), &thread_id);
        if (thread == 0) {
            return begin_thread_error();
        }
        {
            std::lock_guard<std::mutex> lock(g_mutex);
            auto insertion = g_operations.insert(std::move(registry_node));
            if (!insertion.inserted) {
                registry_node = std::move(insertion.node);
                session->creator_fence.store(2, std::memory_order_release);
                WaitForSingleObject(reinterpret_cast<HANDLE>(thread), INFINITE);
                CloseHandle(reinterpret_cast<HANDLE>(thread));
                return E_NOT_VALID_STATE;
            }
            // Keep the creator hold through both no-fail publication and the
            // output write. Only then may the raw-pointer worker proceed.
            *opId = id;
            session->creator_fence.store(1, std::memory_order_release);
        }
        CloseHandle(reinterpret_cast<HANDLE>(thread));
        return S_OK;
    } catch (...) { return exception_hr(); }
}

HRESULT __stdcall CaptureActivate(uint32_t opId, const wchar_t* deviceId) {
    if (deviceId == nullptr) return E_POINTER;
    HRESULT initialized = require_initialized(true);
    if (FAILED(initialized)) return initialized;
    auto session = lookup_operation<CaptureSession>(opId, OperationKind::Capture);
    if (!session) return E_HANDLE;
    HANDLE callback_notify = nullptr;
    HRESULT hr = DuplicateSignalHandle(session->original_notify, &callback_notify);
    if (FAILED(hr)) return hr;
    uint64_t current = session->packed.load(std::memory_order_acquire);
    if (PackedState(current) != kPrivateStatePrepared) {
        CloseHandle(callback_notify);
        return E_NOT_VALID_STATE;
    }
    const uint64_t activating = PackState(CAP_STATE_ACTIVATING, kPrivateStateActivating, false, PackedReason(current));
    if (!session->packed.compare_exchange_strong(current, activating, std::memory_order_acq_rel, std::memory_order_acquire)) {
        CloseHandle(callback_notify);
        return E_NOT_VALID_STATE;
    }
    winrt::com_ptr<ActivationHandler> handler;
    try {
        handler = winrt::make_self<ActivationHandler>(session, callback_notify);
        auto handler_interface = handler.as<IActivateAudioInterfaceCompletionHandler>();
        winrt::com_ptr<IActivateAudioInterfaceAsyncOperation> operation;
        {
            std::lock_guard<std::mutex> lock(session->handoff_mutex);
#if defined(PULSAR_CAPTURE_STATIC)
            const bool defer_callback =
                g_test_defer_activation_callback.exchange(0, std::memory_order_acq_rel) != 0;
            const HRESULT injected_activation =
                g_test_activation_launch_failure.exchange(S_OK, std::memory_order_acq_rel);
            if (defer_callback) hr = S_OK;
            else if (FAILED(injected_activation)) hr = injected_activation;
            else hr = ActivateAudioInterfaceAsync(deviceId, __uuidof(IAudioClient), nullptr,
                                                   handler_interface.get(), operation.put());
#else
            hr = ActivateAudioInterfaceAsync(deviceId, __uuidof(IAudioClient), nullptr, handler_interface.get(), operation.put());
#endif
            if (SUCCEEDED(hr)) {
                session->activation_launched = true;
                session->activation_op = operation;
                session->activation_handler = handler_interface;
            } else {
                session->activation_launched = false;
            }
        }
        if (FAILED(hr)) {
			install_reason(session.get(), classify_audio_error(hr), hr, true);
            HANDLE abandoned = handler->abandon_notify();
            if (abandoned != nullptr) CloseHandle(abandoned);
            SetEvent(session->capture_thread_wake);
        }
        return S_OK;
    } catch (...) {
		const HRESULT failure = exception_hr();
        HANDLE abandoned = handler ? handler->abandon_notify() : callback_notify;
        if (abandoned != nullptr) CloseHandle(abandoned);
        {
            std::lock_guard<std::mutex> lock(session->handoff_mutex);
            session->activation_launched = false;
        }
		install_reason(session.get(), CAP_REASON_WASAPI_ERROR, failure, true);
        SetEvent(session->capture_thread_wake);
        return S_OK;
    }
}

HRESULT __stdcall CapQualityGetVersion(uint32_t* version, uint32_t* structSize) {
    if (version == nullptr || structSize == nullptr) return E_POINTER;
    *version = kCaptureQualityNativeVersion;
    *structSize = sizeof(CaptureQualityNative);
    return S_OK;
}

HRESULT __stdcall CaptureConfigureQuality(uint32_t opId, int32_t requested) {
    if (requested != 0 && requested != 1) return E_INVALIDARG;
    auto session = lookup_operation<CaptureSession>(opId, OperationKind::Capture);
    if (!session) return E_HANDLE;
    const uint8_t state = PackedState(session->packed.load(std::memory_order_acquire));
    if (state >= kPrivateStateActivating) return E_NOT_VALID_STATE;
    session->quality_requested.store(static_cast<uint32_t>(requested), std::memory_order_release);
    return S_OK;
}

HRESULT __stdcall CaptureGetQualityResult(uint32_t opId, CaptureQualityNative* quality) {
    if (quality == nullptr) return E_POINTER;
    if (quality->structSize < sizeof(CaptureQualityNative) ||
        quality->version != kCaptureQualityNativeVersion) return E_INVALIDARG;
    auto session = lookup_operation<CaptureSession>(opId, OperationKind::Capture);
    if (!session) return E_HANDLE;
    quality->requested = session->quality_requested.load(std::memory_order_acquire);
    quality->communicationsCategoryActive =
        session->communications_category_active.load(std::memory_order_acquire);
    quality->nativeEffectsVerified =
        session->native_effects_verified.load(std::memory_order_acquire);
    return S_OK;
}

HRESULT __stdcall CaptureGetResult(uint32_t opId, int32_t* state, CaptureFormat* format,
                                   uint32_t* framesAvailable, HRESULT* hresult, int32_t* terminalReason) {
    if (state == nullptr || format == nullptr || framesAvailable == nullptr || hresult == nullptr || terminalReason == nullptr) return E_POINTER;
    if (format->structSize < sizeof(CaptureFormat) || format->version != kCaptureFormatVersion) return E_INVALIDARG;
    auto session = lookup_operation<CaptureSession>(opId, OperationKind::Capture);
    if (!session) return E_HANDLE;
    const uint64_t packed = session->packed.load(std::memory_order_acquire);
    const uint8_t private_state = PackedState(packed);
    if (private_state == kPrivateStateTerminal) {
        *state = PublicStateFromReason(PackedReason(packed));
        *hresult = session->terminal_hr;
        *terminalReason = session->terminal_reason;
    } else if (private_state <= kPrivateStatePrepared) {
        *state = CAP_STATE_PREPARING;
        *hresult = S_OK;
        *terminalReason = CAP_REASON_USER_STOP;
    } else if (private_state == kPrivateStateActivating) {
        *state = CAP_STATE_ACTIVATING;
        *hresult = S_OK;
        *terminalReason = CAP_REASON_USER_STOP;
    } else if (private_state == kPrivateStateCapturing) {
        *state = CAP_STATE_CAPTURING;
        *hresult = S_OK;
        *terminalReason = CAP_REASON_USER_STOP;
    } else {
        *state = PackedLastPublicState(packed);
        *hresult = S_OK;
        *terminalReason = CAP_REASON_USER_STOP;
    }
    {
        std::lock_guard<std::mutex> lock(session->result_mutex);
        *format = session->format;
    }
    format->ready = session->mta_ready.load(std::memory_order_acquire);
    *framesAvailable = session->ring.Available();
    return S_OK;
}

HRESULT __stdcall CaptureRead(uint32_t opId, float* buf, uint32_t maxFrames, uint32_t* framesRead) {
    if (framesRead == nullptr) return E_POINTER;
    if (buf == nullptr && maxFrames != 0) return E_POINTER;
    auto session = lookup_operation<CaptureSession>(opId, OperationKind::Capture);
    if (!session) return E_HANDLE;
    maxFrames = std::min<uint32_t>(maxFrames, kMaxBufferFrames);
    *framesRead = session->ring.Read(buf, maxFrames);
    return *framesRead == 0 ? S_FALSE : S_OK;
}

HRESULT __stdcall CaptureRequestStop(uint32_t opId, int32_t reason) {
    auto session = lookup_operation<CaptureSession>(opId, OperationKind::Capture);
    if (!session) return S_OK;
    if (reason < CAP_REASON_USER_STOP || reason > CAP_REASON_CANCEL) return E_INVALIDARG;
    install_reason(session.get(), reason, exact_hresult_for_reason(reason, S_OK), false);
    return S_OK;
}

HRESULT __stdcall PulsarProbeDiagnosticsGetVersion(uint32_t* version, uint32_t* structSize) {
    if (version == nullptr || structSize == nullptr) return E_POINTER;
    *version = PULSAR_PROBE_DIAGNOSTICS_EXTENSION_V1;
    *structSize = sizeof(PulsarProbeCaptureDiagnosticsV1);
    return S_OK;
}

HRESULT __stdcall PulsarProbeCaptureGetDiagnosticsV1(
    uint32_t opId,
    PulsarProbeCaptureDiagnosticsV1* diagnostics) {
    if (diagnostics == nullptr) return E_POINTER;
    if (diagnostics->structSize != sizeof(PulsarProbeCaptureDiagnosticsV1) ||
        diagnostics->version != PULSAR_PROBE_DIAGNOSTICS_EXTENSION_V1) return E_INVALIDARG;
    auto session = lookup_operation<CaptureSession>(opId, OperationKind::Capture);
    if (!session) return E_HANDLE;
    diagnostics->timestampErrorCount = session->timestamp_error_count.load(std::memory_order_acquire);
    diagnostics->cleanupReleaseBufferHResult =
        session->cleanup_release_buffer_hr.load(std::memory_order_acquire);
    diagnostics->cleanupStopHResult = session->cleanup_stop_hr.load(std::memory_order_acquire);
    return S_OK;
}

HRESULT __stdcall CaptureRelease(uint32_t opId) {
    std::lock_guard<std::mutex> lock(g_mutex);
    auto it = g_operations.find(opId);
    if (it == g_operations.end()) return S_OK;
    if (it->second->kind != OperationKind::Capture) return E_HANDLE;
    auto session = std::dynamic_pointer_cast<CaptureSession>(it->second);
    if (!session || !is_terminal_private(PackedState(session->packed.load(std::memory_order_acquire)))) return E_ILLEGAL_METHOD_CALL;
    g_operations.erase(it);
    return S_OK;
}

HRESULT __stdcall PickerOpenFile(HWND hwnd, const wchar_t* filterDesc, const wchar_t* filterPattern,
                                 HANDLE notifyEvent, uint32_t* opId) {
    if (opId == nullptr) return E_POINTER;
    if (hwnd == nullptr) return E_INVALIDARG;
    HRESULT valid = validate_event(notifyEvent);
    if (FAILED(valid)) return valid;
    if (filterDesc == nullptr || filterPattern == nullptr) return E_POINTER;
    (void)filterDesc; // FileOpenPicker exposes extension filters, not descriptions.
    HRESULT initialized = require_initialized(true);
    if (FAILED(initialized)) return initialized;
    try {
        auto op = std::make_shared<PickerOperation>();
        HANDLE duplicate = nullptr;
        HRESULT hr = DuplicateSignalHandle(notifyEvent, &duplicate);
        if (FAILED(hr)) return hr;
        op->notify.store(reinterpret_cast<uintptr_t>(duplicate), std::memory_order_release);
        {
            std::lock_guard<std::mutex> lock(g_mutex);
            if (has_kind_locked(OperationKind::Picker)) return E_NOT_VALID_STATE;
            const uint32_t id = allocate_id_locked();
            if (id == 0) return E_OUTOFMEMORY;
            op->id = id;
            g_operations.emplace(id, op);
            *opId = id;
        }
        try {
            winrt::Windows::Storage::Pickers::FileOpenPicker picker;
            auto initialize = picker.as<IInitializeWithWindow>();
            winrt::check_hresult(initialize->Initialize(hwnd));
            std::wstring pattern(filterPattern);
            if (pattern == L"*.*" || pattern.empty()) pattern = L"*";
            if (pattern.size() > 2 && pattern[0] == L'*' && pattern[1] == L'.') pattern.erase(0, 1);
            picker.FileTypeFilter().Append(winrt::hstring(pattern));
            op->async = picker.PickSingleFileAsync();
            op->async.Completed([op](auto const& operation, AsyncStatus async_status) {
                CallbackScope scope;
                try {
                    std::lock_guard<std::mutex> lock(op->mutex);
                    if (async_status == AsyncStatus::Canceled) {
                        op->outcome = S_OK;
                        op->state.store(2, std::memory_order_release);
                    } else if (async_status != AsyncStatus::Completed) {
                        op->outcome = operation.ErrorCode();
                        op->state.store(3, std::memory_order_release);
                    } else {
                        auto file = operation.GetResults();
                        if (!file) {
                            op->outcome = S_OK;
                            op->state.store(2, std::memory_order_release);
                        } else {
                            winrt::com_ptr<IStorageItemHandleAccess> access;
                            winrt::check_hresult(reinterpret_cast<IUnknown*>(winrt::get_abi(file))->QueryInterface(
                                __uuidof(IStorageItemHandleAccess), access.put_void()));
                            HANDLE handle = INVALID_HANDLE_VALUE;
                            winrt::check_hresult(access->Create(HAO_READ, HSO_SHARE_READ, HO_NONE, nullptr, &handle));
                            if (handle == nullptr || handle == INVALID_HANDLE_VALUE) {
                                winrt::throw_hresult(E_HANDLE);
                            }
                            winrt::handle owned_handle(handle);
                            LARGE_INTEGER size{};
                            op->result.fileSize = GetFileSizeEx(handle, &size) && size.QuadPart >= 0 ? size.QuadPart : -1;
                            op->result.displayName = file.Name().c_str();
                            if (op->result.displayName.size() > 259) op->result.displayName.resize(259);
                            op->result.fileHandle = owned_handle.detach();
                            op->result.outcome = S_OK;
                            op->result.state = 1;
                            op->outcome = S_OK;
                            op->state.store(1, std::memory_order_release);
                        }
                    }
                    op->async = nullptr;
                } catch (...) {
                    op->outcome = exception_hr();
                    op->async = nullptr;
                    op->state.store(3, std::memory_order_release);
                }
                HANDLE event = op->take_notify();
                if (event != nullptr) { SetEvent(event); CloseHandle(event); }
            });
        } catch (...) { complete_and_signal(op, 3, exception_hr()); }
        return S_OK;
    } catch (...) { return exception_hr(); }
}

HRESULT __stdcall PickerGetResult(uint32_t opId, int32_t takeHandle, int32_t* state, HANDLE* fileHandle,
                                  int32_t* handleTaken, int64_t* fileSize, wchar_t* nameBuf, int32_t nameBufLen,
                                  int32_t* requiredNameChars, HRESULT* hresult) {
    auto op = lookup_operation<PickerOperation>(opId, OperationKind::Picker);
    if (!op) return E_HANDLE;
    if (takeHandle != 0 && takeHandle != 1) return E_INVALIDARG;
    if (state == nullptr || handleTaken == nullptr || hresult == nullptr || (takeHandle == 1 && fileHandle == nullptr)) return E_POINTER;
    std::lock_guard<std::mutex> lock(op->mutex);
    op->result.state = op->state.load(std::memory_order_acquire);
    op->result.outcome = op->outcome;
    return QueryPickerResult(&op->result, takeHandle, state, fileHandle, handleTaken,
                             fileSize, nameBuf, nameBufLen, requiredNameChars, hresult);
}

HRESULT __stdcall PickerCancel(uint32_t opId) {
    auto op = lookup_operation<PickerOperation>(opId, OperationKind::Picker);
    if (!op) return E_HANDLE;
    if (op->state.load(std::memory_order_acquire) != 0) return E_NOT_VALID_STATE;
    winrt::Windows::Foundation::IAsyncOperation<winrt::Windows::Storage::StorageFile> async{nullptr};
    { std::lock_guard<std::mutex> lock(op->mutex); async = op->async; }
    return cancel_async(async);
}

HRESULT __stdcall PickerRelease(uint32_t opId) {
    std::lock_guard<std::mutex> lock(g_mutex);
    auto it = g_operations.find(opId);
    if (it == g_operations.end()) return S_OK;
    if (it->second->kind != OperationKind::Picker) return E_HANDLE;
    if (it->second->state.load(std::memory_order_acquire) == 0) return E_ILLEGAL_METHOD_CALL;
    g_operations.erase(it);
    return S_OK;
}

HRESULT __stdcall CapIsQuiescent(void) {
    return g_callback_refs.load(std::memory_order_acquire) == 0 &&
           g_capture_threads.load(std::memory_order_acquire) == 0 &&
           g_subscription_states.load(std::memory_order_acquire) == 0 ? S_OK : S_FALSE;
}

HRESULT __stdcall CapDestroy(void) {
    std::lock_guard<std::mutex> lock(g_mutex);
    if (!g_initialized) return S_OK;
    if (GetCurrentThreadId() != g_init_thread) return RPC_E_WRONG_THREAD;
    if (!g_operations.empty() || g_subscription || g_callback_refs.load(std::memory_order_acquire) != 0 ||
        g_capture_threads.load(std::memory_order_acquire) != 0 ||
        g_subscription_states.load(std::memory_order_acquire) != 0) return E_ILLEGAL_METHOD_CALL;
    g_microphone_capability = nullptr;
    g_capability_hr = E_FAIL;
	g_runtime_state.reset();
    g_initialized = false;
    g_init_thread = 0;
    if (g_ro_initialized) {
        balanced_ro_uninitialize();
        g_ro_initialized = false;
    }
    return S_OK;
}

}  // extern "C"
