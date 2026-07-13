# 2026-07-12 Windows AppContainer capture and picker bridge decision

Task: `TASK-260712-6kba80`  
Scope: select the legal capture, picker, hotkey, and lifecycle API surface for the signed Windows Store probe under `packagedClassicApp` + `appContainer`, without `runFullTrust`, broad filesystem access, undocumented APIs, or developer-mode-only behavior.

**Revision 9** — amended per root review round 8 blocking findings (2026-07-12).
Changes: **Executable CaptureStart branch table** — replaced contradictory prose
with a single branch table covering every `CaptureStart` failure path: bad input,
ID exhaustion, allocation failure, thread-creation failure, capture-thread
`CoInitializeEx` failure, readiness timeout, synchronous activation-launch
failure, async activation failure, cancel before callback, cancel after handoff,
normal stop, and capture-loop failure; each row specifies function HRESULT,
whether `*opId` is written, registry membership, terminal publisher, callback
expectation, wake/event signals, and cleanup owner; the rule is: once an
operation ID is returned, `CaptureStart` returns `S_OK` and all subsequent
outcomes (including timeout, `CoInitializeEx` failure, synchronous activation
failure) are delivered via the operation's terminal state through `CaptureGetResult`;
failures before the operation ID is published (bad input, ID exhaustion, allocation
failure, thread-creation failure) return an error HRESULT directly with no
operation created; a timeout or synchronous launch failure sets a pending
stop/failure cause and lets the capture thread publish terminal only after its
apartment cleanup; a cancelled activation is not terminal while the un-cancellable
callback may still acquire an `IAudioClient`; terminal waits until both the
capture thread and late callback have reached their defined cleanup fence (R8-1).
**Non-self-joining capture thread ownership** — the R8 race where the capture
thread holds the last `shared_ptr`, releases it, triggers the destructor that
waits for the thread's own exit event, and deadlocks, is eliminated; the capture
thread no longer holds a reference that can trigger the destructor; instead, the
thread completes COM cleanup, publishes terminal state, performs its final session
access (incrementing an atomic `threadDone` flag), then exits; the destructor
uses the `threadDone` flag (not a `WaitForSingleObject` join) so it never waits
on the current thread; `CapDestroy` checks `threadDone` to confirm the thread
has exited; a dedicated barrier test pauses the capture thread after publishing
terminal but before setting `threadDone`, verifies Go can observe terminal and
call `CaptureRelease`, and confirms neither deadlocks nor crashes (R8-2).
**Coherent permission ABI with named `CAP_PERMISSION_*` enum** — replaced the
ambiguous "mapping" between raw `AppCapabilityAccessStatus` and the ABI with an
explicitly named `CAP_PERMISSION_*` enum; an exhaustive switch in the helper maps
every raw WinRT value: raw `DeniedBySystem`(0)→`CAP_PERMISSION_DENIED_BY_SYSTEM`(3),
raw `NotDeclaredByApp`(1)→`CAP_PERMISSION_NOT_DECLARED`(4), raw `DeniedByUser`(2)
→`CAP_PERMISSION_DENIED_BY_USER`(0), raw `UserPromptRequired`(3)
→`CAP_PERMISSION_PROMPT_REQUIRED`(2), raw `Allowed`(4)→`CAP_PERMISSION_ALLOWED`(1);
unknown/future raw values map to `CAP_PERMISSION_UNKNOWN`(5) (fail-closed);
a direct cast of the raw integer NEVER reaches Go — the switch guarantees no
`NotDeclaredByApp`(1) → `Allowed`(1) misinterpretation; the fallback-unavailable
contradiction is resolved: `CAP_PERMISSION_UNAVAILABLE`(-1) is a **no-go for
promotion** — the pre-promotion guard rejects it identically to denied states,
and a separately gated `activation-consent + proven-revoke-monitor` mode with
an explicit promotion rule is defined for the fallback; the `AccessChanged`
ownership is frozen as a strong `shared_ptr<SubscriptionState>` containing the
`AppCapability` object (not a raw pointer or `weak_ref`); tests cover every raw
enum value including the security-critical raw-1-vs-raw-4 case (R8-3).
**Truthful WASAPI terminal reasons** — `AUDCLNT_E_SERVICE_NOT_RUNNING` and
`AUDCLNT_E_RESOURCES_INVALIDATED` are reclassified from `CAP_REASON_DEVICE_LOST`
to `CAP_REASON_WASAPI_ERROR` (non-promotable); Microsoft documents
`RESOURCES_INVALIDATED` as covering suspended/quiesced streams, and
`SERVICE_NOT_RUNNING` as audio service stopped, neither of which is a removed
device; only `AUDCLNT_E_DEVICE_INVALIDATED` maps to `CAP_REASON_DEVICE_LOST`;
the HRESULT→reason mapping applies to errors from `GetNextPacketSize`,
`GetBuffer`, `ReleaseBuffer`, `Start`, and `Stop`, not only the first two;
in AppCapability-unavailable fallback mode, no WASAPI failure other than a
hardware-proven privacy mapping may be assumed safe for promotion; the stop-reason
linearization is tightened: the capture thread reloads/commits the final
priority-CAS value immediately before publishing terminal (R8-4).
**UI-thread event/message integration via `MsgWaitForMultipleObjectsEx`** — the
live `pulsar-win` main thread blocks in `GetMessageW`; a second independent
`WaitForMultipleObjects` loop cannot coexist on the same thread; replaced with a
dedicated waiter goroutine/OS thread (`runtime.LockOSThread`) that waits for
helper notification events via `WaitForMultipleObjects`, drains thread-safe result
APIs (`CaptureRead`, `CaptureGetResult`, `CapPermissionCheck`, `PickerGetResult`),
and uses `PostMessageW` to forward UI-affecting actions to the pinned UI thread;
`CapInit`, `CapDestroy`, `CaptureStart`, permission request, and picker initiation
remain on the exact initialized UI thread; event handle lifetime and waiter
shutdown are defined; a coalesced-signal test while picker/UI messages and
`WM_ENDSESSION` are queued proves messages continue to dispatch and every
operation is drained (R8-5). **Ring atomicity with scratch buffer and capacity
bound** — conversion writes into scratch storage first; the producer index is
published only after the entire packet converts successfully; on conversion or
`ReleaseBuffer` failure, zero frames become visible to the consumer; the claim
that a 2-second ring is always larger than the maximum WASAPI packet is proven:
after `GetBufferSize`, the ring is allocated as
`max(2 * sampleRate, bufferFrames) * channels * sizeof(float32)` — if
`bufferFrames > 2 * sampleRate`, the ring grows to fit; an allocation failure
before `Start` prevents capture with `E_OUTOFMEMORY`; a low-rate/large-buffer
fixture test verifies the dynamic sizing (R8-6). **Global init/loader
completeness** — `CapInit` stores the initializing thread ID; a second `CapInit`
before `CapDestroy` returns `E_NOT_VALID_STATE`; `CapDestroy` from a wrong thread
returns `RPC_E_WRONG_THREAD`; a failed `CapInit` leaves no state; idempotent
`CapDestroy` after success is a no-op; re-`CapInit` after `CapDestroy` works;
`S_OK`, `S_FALSE`, `RPC_E_CHANGED_MODE`, repeated init, wrong-thread destroy,
double destroy, and re-init are all tested; the loader uses
`windows.NewLazySystemDLL("kernel32.dll")` (not `NewLazyDLL`) as required by
R7 — the x/sys kernel32 special-case exists but the contract should not depend
on a hidden exception; the unit test uses an injectable function wrapper seam
(a `var loadPackagedLibraryFn func(...)` that tests replace), not `.Call` method
replacement on `*windows.LazyProc`; tests cover `APPMODEL_ERROR_NO_PACKAGE`
fallback, all other errors, absolute path construction, flags, and
process-lifetime no-unload (R8-7).
All earlier rev-1/2/3/4/5/6/7/8 changes preserved.

## Highlights

- Keep the package posture exactly in the current lane: `uap10:RuntimeBehavior="packagedClassicApp"` plus `uap10:TrustLevel="appContainer"`. Do not add `runFullTrust`; Microsoft documents that as the medium-IL/full-trust route, not the current partial-trust lane. `[MS-1]`
- Add one manifest capability only: `<DeviceCapability Name="microphone" />`. The picker path does not need `broadFileSystemAccess` or library capabilities because the standard file picker already grants access to the picked file. `[MS-2] [MS-3]`
- Select a hybrid bridge:
  - WinRT for explicit microphone permission, access-change monitoring, default/selected-device enumeration, and brokered file picking.
  - `ActivateAudioInterfaceAsync` for the actual AppContainer-safe WASAPI activation of `IAudioClient`, followed by `IAudioCaptureClient` inside a native helper.
  - Existing Go shell keeps tray/UI state, logging, PCM handling, and MSIX posture.
- The current tray loop cannot stay message-only for P1.0. Microsoft documents that message-only windows do not receive broadcast messages, while P1.0 needs shutdown, suspend, lock/unlock, and lifecycle signals. The lifecycle owner must be a hidden top-level window — but the **picker owner must be a visible top-level window** (see §Picker owner HWND). `[MS-4]`
- `AppCapability.Create` is documented as callable only by SUA (single-user) apps. `[MS-6]` If it is unavailable at runtime, `ActivateAudioInterfaceAsync` itself shows the consent prompt on first microphone use and the HRESULT in the completion handler reports denial — this is the documented fallback. `[MS-5]`
- Media Foundation and full `MediaCapture` are both legal candidates, but neither is the best P1.0 implementation target. Media Foundation adds a second media stack without improving the permission story; `MediaCapture` adds a heavier WinRT recording abstraction when the app ultimately wants raw PCM and deterministic start/stop control.
- **The entire ABI is asynchronous.** No native export blocks the UI thread with `.get()` on a WinRT async operation. Every operation that touches WinRT async or WASAPI activation uses an initiate → event/message → query/take-result contract. `[MS-39]`
- **The helper DLL is loaded via `LoadPackagedLibrary`.** This is the only safe loading path for a DLL inside a signed MSIX package; it searches only the package dependency graph and eliminates ambient DLL search. `[MS-40]`
- **`GetMixFormat` runs before `Initialize`.** The format returned by `GetMixFormat` is the format passed to shared-mode `Initialize`. The previous rev had them reversed. `[MS-38]`
- **The probe writes short disposable native-format evidence WAVs** at the device's native capture rate and channel count as IEEE float32. These are **not** user drafts and no production bounds (180 s / 50 MiB) apply to them. The production recording task (future, outside this bridge scope) must implement a streaming mono downmixer to a frozen canonical mono format and enforce product bounds against upload-ready mono bytes. `toEngineFormat` in `voice.go` is not used for recording — it is a batch converter for voice-insert playback.
- **Recording ring overflow is terminal failure** with `HRESULT_FROM_WIN32(ERROR_BUFFER_OVERFLOW)` = `0x8007006F`. The `CAP_REASON_OVERFLOW` terminal-reason enum disambiguates overflow from other causes. A separate lossy meter ring handles UI VU display.
- **PCM valid bits are left-aligned** in their container per `[MS-44]`. 24-in-32 positive full-scale is `0x7FFFFF00`, not `0x007FFFFF`. Scaling divisor is `2^(validBits-1)`. Both packed 24-bit and 24-in-32 extraction use unsigned assembly with explicit sign extension (no signed-left-shift UB, no implementation-defined arithmetic right shift). Boundary test vectors cover min, max, ±1 LSB, and silence (R4-1, R5-2).
- **WASAPI packet draining is exact.** Auto-reset events are readiness hints only — `SetEvent` calls can coalesce; the capture thread loops `GetNextPacketSize`/`GetBuffer`/`ReleaseBuffer` until packet size is zero. Acquired packets are always released before stop/error. Go drains `CaptureRead` until `S_FALSE` and queries all operations (including `CapPermissionCheck` for `AccessChanged` subscription) per wake (R4-3, R5-4).
- **`CapDestroy` always requires zero active operations and zero active subscriptions.** No forced-destroy mode exists. `CapPermissionUnsubscribe` must be called explicitly before `CapDestroy` — there is no auto-unsubscribe. On `WM_ENDSESSION`, Go requests stop and returns from the wndproc; the OS reclaims process resources (R4-2, R5-4).
- **Picker uses a two-step size-discovery/take API.** `PickerGetResult(takeHandle=0)` probes `requiredNameChars`/`fileSize` without transferring the handle. `takeHandle=1` transfers exactly once. `PickerRelease` closes untaken handles (R4-4).
- **Lifecycle evidence uses a frozen outcome matrix.** Valid user media requires finalized `.wav` or proven-recoverable `.partial`. Permission revoke/cancel/too-short is evidenced deliberate discard. Queued `CaptureRequestStop` alone is never a pass. AppCapability fallback is conditional on proven WASAPI revoke detection (R4-7).
- **`IStorageItemHandleAccess::Create` under AppContainer is a probe hypothesis.** If it fails, the picker scenario is blocked.
- **`uintptr` → `int32` truncation** before any HRESULT sign test. `uintptr < 0` is never valid (unsigned).
- **Checked allocation bounds**: channels ≤ 8 (field is `uint16` max 65535, but >8 rejected); sample rate ≤ 384 kHz; all arithmetic in wide types with overflow check (R4-6).
- **Every async callback holds a strong operation reference** until its final instruction/return. `CaptureRelease`/`PickerRelease`/destroy drop only the registry reference; the operation destructs only when the last reference (registry or callback) is released. The helper DLL is loaded once and **never unloaded** during the process lifetime (`FreeLibrary` is never called); `CapDestroy` tears down application state only; the module is reclaimed at process exit (R6-1).
- **AccessChanged unsubscribe is handle-safe.** At `CapPermissionSubscribe` time, the helper duplicates Go's `notifyEvent` via `DuplicateHandle`; handlers signal only the duplicate; Go can safely close its original handle immediately after `CapPermissionUnsubscribe` returns (R6-2).
- **Stop-reason arbitration uses atomic priority.** Overflow, discontinuity, and permission_revoke dominate all finalizable reasons regardless of arrival order. Go rechecks final terminal reason AND permission status (must be exactly `Allowed`) before `.partial` → `.wav` promotion; unknown WASAPI errors map to `CAP_REASON_WASAPI_ERROR` (non-promotable), not fake `CAP_REASON_PERMISSION_REVOKE` (R7-2). `AUDCLNT_E_NOT_ALLOWED` removed from the HRESULT table — `0x88890003` is `AUDCLNT_E_WRONG_ENDPOINT_TYPE` (R7-2).
- **PCM sample reads use `memcpy`** (or byte assembly for packed 24-bit), not pointer casts, eliminating unaligned-access and strict-aliasing UB. Signed 24-bit conversion uses safe signed arithmetic on representable values: `int32_t val = (int32_t)u; if (u >= 0x800000u) val -= 0x1000000;` — the first cast is safe because `u <= 0xFFFFFF`; the subtraction is signed arithmetic on `int32_t` values, not the implementation-defined out-of-range unsigned-to-signed cast that R7 correctly identified in the prior `(int32_t)(u - 0x1000000u)` form (R7-1). WAV header is the **selected initial build-time contract**, confirmed or rejected by independent decoder gate before signed hardware scenarios — no artifact promoted using pre-gate assumptions (R7-7).
- **Whole-packet ring preflight** before conversion/copy. If the recording ring lacks room for the entire WASAPI packet, zero frames are written, `ReleaseBuffer` is called, and the session transitions to overflow failure. The prior "copy then check" design could overrun or leave a partial packet (R7-3).
- **WASAPI discontinuity handling.** First packet `DATA_DISCONTINUITY` is accepted (stream start); subsequent discontinuity is terminal `CAP_REASON_DISCONTINUITY` (non-promotable integrity failure). `TIMESTAMP_ERROR` logged but accepted (R7-3).
- **Helper loaded via `kernel32.NewProc("LoadPackagedLibrary")`**, not the non-existent `windows.LoadPackagedLibrary` in `x/sys v0.46.0` (R7-5).
- **`CapInit` initializes the UI-thread WinRT apartment** via `RoInitialize(RO_INIT_SINGLETHREADED)`, balanced by `RoUninitialize` in `CapDestroy` (R7-5).
- **Coherent CaptureStart state machine.** `readyEvent` has a 5-second finite timeout; all async failures (including `CoInitializeEx`) travel through the operation, not `CaptureStart` return HRESULT; terminal state published only after `CoUninitialize`; capture thread holds a strong session ref; C++/WinRT async cycle explicitly broken at callback point (R7-4).
- **Complete picker pointer/error truth table.** Every parameter classified as mandatory or optional; validation order frozen; null mandatory pointers return `E_POINTER` without transfer/close; negative `nameBufLen` treated as zero capacity; table-driven ABI tests for every combination (R7-6).

## Repo snapshot

### Current seams inspected

- `pulsar-win/msix/AppxManifest.xml.in`
  - already declares `uap10:TrustLevel="appContainer"` and `uap10:RuntimeBehavior="packagedClassicApp"`;
  - currently declares only networking capabilities.
- `pulsar-win/audio_windows.go`
  - output-only WASAPI render path using `go-wca` and `go-ole`;
  - no microphone capture path.
- `pulsar-win/ui_windows.go`
  - main goroutine is pinned to the OS thread (`runtime.LockOSThread()` in `init()`), which is good and necessary for UI-thread-only APIs;
  - tray loop currently creates a message-only window (`HWND_MESSAGE`);
  - onboarding window is a visible top-level window on the same pinned UI thread;
  - no `RegisterHotKey`, shutdown/suspend/session handling, or picker owner path yet.
- `pulsar-win/config.go`
  - already records the AppContainer named-pipe rule via `\\.\pipe\LOCAL\...`.
- `.github/workflows/release.yml`
  - builds `pulsar-win-amd64.exe` and `go-librespot.exe`;
  - current Windows/MSIX packaging is x64-only.

### Dependency posture

- Current Windows Go dependencies are:
  - `github.com/Microsoft/go-winio` `v0.6.2` (MIT)
  - `github.com/go-ole/go-ole` `v1.2.6` (MIT)
  - `github.com/moutend/go-wca` `v0.3.0` (MIT)
- That license posture is already clean for Store redistribution.

## Selected decision

### Decision

Use a small native Windows helper bridge for WinRT-sensitive operations, but keep the capture engine on WASAPI:

1. Permission and permission-revoke monitoring:
   - `Windows.Security.Authorization.AppCapabilityAccess.AppCapability`
   - `CheckAccess()`
   - `RequestAccessAsync()`
   - `AccessChanged`
   - **Caveat**: `AppCapability.Create` is SUA-only `[MS-6]`; the probe must test this under the signed package and fall back to `ActivateAudioInterfaceAsync` consent if it fails (see §AppCapability fallback).
2. Default and selected input enumeration:
   - `Windows.Media.Devices.MediaDevice.GetDefaultAudioCaptureId`
   - `Windows.Devices.Enumeration.DeviceInformation.FindAllAsync(DeviceClass.AudioCapture)`
   - `DeviceInformation.CreateWatcher(DeviceClass.AudioCapture)` for live updates
3. Actual microphone activation:
   - `ActivateAudioInterfaceAsync(..., IID_IAudioClient, ...)`
   - then `IAudioClient::GetService(IID_IAudioCaptureClient)`
4. File picker:
   - `Windows.Storage.Pickers.FileOpenPicker`
   - initialized with `IInitializeWithWindow` using a **visible** Pulsar top-level window
   - picker returns a **read handle** via `IStorageItemHandleAccess::Create`, not a path `[MS-41]`
5. Hotkey and lifecycle owner:
   - hidden top-level Win32 window on the existing main UI thread
   - `RegisterHotKey`
   - `WM_QUERYENDSESSION`
   - `WM_ENDSESSION`
   - `WM_POWERBROADCAST`
   - `WTSRegisterSessionNotification` + `WM_WTSSESSION_CHANGE`

### Why this is the best fit

- It is the clearest documented AppContainer path for microphone capture in a Store-style package:
  - `ActivateAudioInterfaceAsync` explicitly says it enables Windows Store apps to activate WASAPI COM interfaces after WinRT device selection, and it documents the first-microphone consent prompt and UI-thread requirement. `[MS-5]`
- It gives an explicit permission surface before opening the mic:
  - `AppCapability.CheckAccess()` can return `UserPromptRequired`;
  - `RequestAccessAsync()` can prompt on the UI thread;
  - `AccessChanged` gives a documented access-status change signal while the app is not suspended. `[MS-6] [MS-7] [MS-8]`
  - If `AppCapability` is unavailable (SUA-only constraint), `ActivateAudioInterfaceAsync` consent is the fallback. `[MS-5]`
- It keeps raw PCM ownership and existing audio architecture alignment:
  - the app already uses a Go-side render engine and ring buffer;
  - a WASAPI capture client is the least awkward way to feed the same style of PCM pipeline.
- It avoids broad file access:
  - the picker grants access to exactly the file the user picked;
  - durable reuse can be done with `FutureAccessList` only if a later task truly needs restart persistence for a picked file. `[MS-3] [MS-9]`

## Exact manifest decision

No runtime-behavior change is needed. Keep:

```xml
<Application Id="Pulsar"
  Executable="pulsar-win-amd64.exe"
  uap10:TrustLevel="appContainer"
  uap10:RuntimeBehavior="packagedClassicApp">
```

The exact capability delta is:

```xml
<Capabilities>
  <Capability Name="internetClient" />
  <Capability Name="internetClientServer" />
  <Capability Name="privateNetworkClientServer" />
  <DeviceCapability Name="microphone" />
</Capabilities>
```

Explicitly rejected in this task:

- `runFullTrust`
- `broadFileSystemAccess`
- `documentsLibrary`
- `musicLibrary`
- any restricted capability for capture or picker convenience

Reason:

- `packagedClassicApp` + `appContainer` is already a supported manifest combination on Windows 10 version 2004 / build 19041 and later, which matches the current `TargetDeviceFamily` minimum. `[MS-1]`
- Microsoft says microphone is a device capability and that apps must handle user disablement. `[MS-2]`
- Microsoft also says extra file-system reach should come either from declared capabilities or from the file picker, and recommends using the picker when programmatic broad access is not required. `[MS-2] [MS-9]`

---

## COM ownership and thread handoff contract

*Addresses root review R1 finding 1. MTA proof tightened per R2 finding 8.*

### Documented threading facts

1. `ActivateAudioInterfaceAsync` must be called on the main UI thread so the consent prompt can be shown. `[MS-5]`
2. The completion handler (`IActivateAudioInterfaceCompletionHandler::ActivateCompleted`) fires on an MTA worker thread. `[MS-5]`
3. The completion handler implementation must be agile — it must aggregate a free-threaded marshaler. `[MS-32]`
4. Windows holds a COM reference to the handler until the operation completes and the async operation object is released. Applications must not free the handler until the callback has fired (documented as **Important**). `[MS-5]`
5. `GetActivateResult` called before completion returns `E_ILLEGAL_METHOD_CALL`. `[MS-33]`
6. There is no cancellation API for in-flight `ActivateAudioInterfaceAsync` operations. `[MS-5] [MS-33]`
7. `IAudioClient::GetService` documentation states: "The client must release a service from the same thread that releases the `IAudioClient` object." `[MS-34]`
8. `IAudioCaptureClient` documentation states: "When releasing an `IAudioCaptureClient` interface instance, the client must call the `Release` method of the instance from the same thread as the call to `IAudioClient::GetService` that created the object." `[MS-17]`
9. The documentation does not explicitly state whether the returned `IAudioClient` is agile/free-threaded. It also does not state that it is apartment-bound. `[MS-5] [MS-22]`
10. Windows 8 had a documented STA requirement for first use of `IAudioClient`; Windows 10+ does not carry this restriction. `[MS-22]`

### MTA proof

*Addresses R2 finding 8.*

Microsoft's multithreaded-apartment documentation explicitly states: "all the threads in the process that have been initialized as free-threaded reside in a single apartment. Therefore, there is no need to marshal between threads." and "interface pointers are passed directly from thread to thread within a multithreaded apartment, so interface pointers are not marshaled between its threads." `[MS-42]`

The frozen requirement:

1. The helper's capture thread **must** call `CoInitializeEx(nullptr, COINIT_MULTITHREADED)` successfully before accessing any COM pointer handed off from the activation callback. If this call fails, the capture session fails with the returned HRESULT.
2. The system's `ActivateCompleted` callback fires on an MTA worker thread (documented `[MS-5]`).
3. Both the callback thread and the capture thread are in the MTA (both initialized with `COINIT_MULTITHREADED` — the system callback implicitly, the capture thread explicitly). Per `[MS-42]`, COM interface pointers pass directly between MTA threads without marshaling.
4. The `IAudioClient*` pointer stored in the mutex-protected handoff slot is a raw COM pointer. The mutex provides memory ordering for the Go memory model. After handoff, the capture thread has **exclusive ownership** — no other thread touches the pointer.
5. If either endpoint is not demonstrably in the MTA (e.g., `CoInitializeEx` on the capture thread returns `RPC_E_CHANGED_MODE`), the capture session **must** fail. COM marshaling via `CoMarshalInterThreadInterfaceInStream`/`CoGetInterfaceAndReleaseStream` is not used because both threads are in the MTA; if they were not, the design would be broken and must be reported as a probe failure, not silently papered over.

### Frozen handoff sequence

*Capture thread started before activation per R6 finding 4.*

```
UI thread (STA, pinned main goroutine)
  │
  ├── Go calls helper: CaptureStart(deviceId, notifyEvent) → opId
  │     helper stores deviceId, creates internal state, sets cancelled=false
  │     returns operation ID immediately (not format/HRESULT — those
  │     come from the completion event)
  │
  ├── Helper creates the capture thread BEFORE launching activation
  │
  ╔══════════════════════════════════════════════════════════════════╗
  ║  Capture thread (helper-owned) — started first                 ║
  ║  Thread does NOT hold a ref-counted session ref (R8-2).        ║
  ║  Thread accesses session state under mutex or atomics only;    ║
  ║  sets atomic threadDone=1 as its final instruction before      ║
  ║  returning from the thread function.                           ║
  ║                                                                ║
  ║  CoInitializeEx(nullptr, COINIT_MULTITHREADED) — must succeed  ║
  ║  If CoInitializeEx fails:                                      ║
  ║    → set terminal FAILED with returned HRESULT                 ║
  ║    → signal readyEvent (UI thread unblocks, sees failure)      ║
  ║    → SetEvent(notifyEvent) — Go sees terminal via GetResult    ║
  ║    → set threadDone=1 (atomic, final instruction)              ║
  ║    → thread exits (no CoUninitialize needed — init failed)     ║
  ║  Signal readyEvent — capture thread is MTA-ready               ║
  ║  WaitForSingleObject(captureThreadWakeEvent) — wait for        ║
  ║  activation handoff or cancellation                            ║
  ╚══════════════════════════════════════════════════════════════════╝
  │
  ├── UI thread waits for readyEvent (finite timeout: 5 seconds)
  │     If timeout expires (R7-4):
  │       → signal captureThreadWakeEvent (thread wakes & exits)
  │       → set terminal FAILED with ERROR_TIMEOUT
  │       → SetEvent(notifyEvent) — Go sees terminal
  │       → CaptureStart returns S_OK with opId (failure is async)
  │     If capture thread reported CoInitializeEx failure:
  │       → CaptureStart returns S_OK with opId (failure is async)
  │
  ├── Helper calls ActivateAudioInterfaceAsync(deviceId, IID_IAudioClient, ...)
  │     passes its agile completion-handler COM object
  │     If ActivateAudioInterfaceAsync returns error HRESULT:
  │       → no callback will fire; session transitions to FAILED
  │       → signal captureThreadWakeEvent (thread sees no handoff)
  │       → SetEvent(notifyEvent) — Go sees immediate failure
  │       → CaptureStart returns the operation ID (failure is
  │         async from Go's perspective)
  │     returns S_OK
  │
  │   ... consent prompt may appear on this UI thread ...
  │
  ╔══════════════════════════════════════════════════════════════════╗
  ║  MTA worker thread (system-owned)                              ║
  ║                                                                ║
  ║  ActivateCompleted(asyncOp) fires:                             ║
  ║    1. Lock helper mutex                                        ║
  ║    2. If cancelled flag is set:                                 ║
  ║         GetActivateResult → release returned interface → done  ║
  ║    3. GetActivateResult → IAudioClient*                        ║
  ║    4. Store IAudioClient* in handoff slot                       ║
  ║       *** LINEARIZATION POINT: before this write, the callback ║
  ║       owns the IAudioClient*. After this write (under mutex),  ║
  ║       the capture thread owns it exclusively. ***               ║
  ║    5. Store activation HRESULT in result slot                   ║
  ║    6. Unlock mutex                                              ║
  ║    7. SetEvent(captureThreadWakeEvent) — capture thread wakes   ║
  ║    8. Callback releases its strong operation ref (see §Callback ║
  ║       strong-reference lifetime)                                ║
  ║    9. Callback returns to the OS                                ║
  ╚══════════════════════════════════════════════════════════════════╝
       │
       ▼
  ╔══════════════════════════════════════════════════════════════════╗
  ║  Capture thread (already running, MTA proven)                  ║
  ║                                                                ║
  ║  WaitForSingleObject(captureThreadWakeEvent) returns:          ║
  ║    1. Lock mutex, take IAudioClient* from handoff slot, unlock  ║
  ║       (exclusive ownership confirmed — linearization point     ║
  ║       already passed)                                          ║
  ║    2. If handoff slot is null (launch failure or cancel):       ║
  ║       → set terminal state, signal notifyEvent, exit thread    ║
  ║    3. IAudioClient::GetMixFormat() → device mix format         ║
  ║       Validate subtype (PCM int16/24/32 or IEEE float32);      ║
  ║       if unsupported → release IAudioClient, fail with         ║
  ║       E_INVALIDARG                                             ║
  ║    4. IAudioClient::Initialize(SHARED, EVENT_CALLBACK,         ║
  ║         bufferDuration, 0, mixFormat, nullptr)                 ║
  ║       The format from step 3 is passed as the Initialize       ║
  ║       format. [MS-38] requires this order.                     ║
  ║    5. Store validated format in session result slot             ║
  ║    6. IAudioClient::SetEventHandle(captureDataEvent)           ║
  ║    7. IAudioClient::GetService(IID_IAudioCaptureClient)        ║
  ║    8. IAudioClient::Start()                                    ║
  ║    9. SetEvent(notifyEvent) — Go queries format via            ║
  ║       CaptureGetResult                                         ║
  ║   10. Capture loop (exact packet drain — R4-3, R7-3):           ║
  ║         WaitForMultipleObjects({captureDataEvent, stopEvent})  ║
  ║         On data event (auto-reset — treat as readiness hint):  ║
  ║           loop {                                               ║
  ║             GetNextPacketSize(&packetSize)                     ║
  ║             if packetSize == 0: break (all packets drained)    ║
  ║             GetBuffer(&data, &frames, &flags, ...)             ║
  ║             if DATA_DISCONTINUITY && !isFirstPacket:           ║
  ║               ReleaseBuffer → terminal DISCONTINUITY           ║
  ║             if TIMESTAMP_ERROR: log for evidence, accept       ║
  ║             isFirstPacket = false                               ║
  ║             if ring.availableForWrite() < frames*channels:     ║
  ║               ReleaseBuffer → terminal OVERFLOW (R7-3)         ║
  ║             if AUDCLNT_BUFFERFLAGS_SILENT: write zeros         ║
  ║             else: convert PCM → float32, copy to ring          ║
  ║             ReleaseBuffer(frames)                              ║
  ║           }                                                    ║
  ║         SetEvent(notifyEvent) — Go reads via CaptureRead       ║
  ║   11. On stop signal:                                          ║
  ║         Reload/commit the final priority-CAS stop reason       ║
  ║           immediately before terminal publication (R8-4).      ║
  ║         IAudioClient::Stop()                                   ║
  ║         IAudioCaptureClient::Release()  ← same thread          ║
  ║         IAudioClient::Release()         ← same thread          ║
  ║         CoUninitialize() — BEFORE terminal publication (R7-4)  ║
  ║         Set terminal state in session (atomic)                 ║
  ║         SetEvent(notifyEvent) — Go sees terminal               ║
  ║         Set threadDone=1 (atomic) — final instruction (R8-2)   ║
  ║         Thread exits                                            ║
  ║         Note: terminal state means all COM objects are released ║
  ║         and CoUninitialize is complete. After threadDone=1 the ║
  ║         thread performs no further session-state access.        ║
  ║         CaptureRelease drops only the registry ref. The        ║
  ║         operation destructor checks threadDone (never joins     ║
  ║         the thread — R8-2). The DLL is process-lifetime loaded ║
  ║         so no module-unload join is needed.                     ║
  ╚══════════════════════════════════════════════════════════════════╝
```

### Why the capture thread starts before activation

*Addresses R6 finding 4. Coherent state machine per R7 finding 4.*

If the capture thread were created after activation completes (as in revisions 1–6), a `CoInitializeEx` failure on the capture thread would leave an `IAudioClient*` in the handoff slot with no thread to release it. By starting the capture thread first and proving `CoInitializeEx` succeeds before launching `ActivateAudioInterfaceAsync`, the handoff slot is guaranteed to have a ready consumer. If `CoInitializeEx` fails, `CaptureStart` publishes the failure through the operation (via `readyEvent` + terminal state + `notifyEvent`), not as a `CaptureStart` return HRESULT.

**`readyEvent` timeout** (R7-4): the UI thread waits for `readyEvent` with a finite timeout of **5 seconds** (not "usually microseconds"). If the timeout expires, `CaptureStart` signals the capture thread to exit, sets the operation to terminal `FAILED` with `HRESULT_FROM_WIN32(ERROR_TIMEOUT)`, signals `notifyEvent`, and returns `S_OK` with the `opId` written (the failure is async — Go queries it via `CaptureGetResult`). In practice, `CoInitializeEx` completes in microseconds; the timeout is a safety net against pathological system states. This does not violate the "no blocking async" rule because it is a bounded synchronous wait for a thread-local system call, not a WinRT `.get()`.

**Linearization point**: the mutex-protected write to the handoff slot (callback step 4) is the linearization point for `IAudioClient*` ownership transfer. Before this write, the callback owns the pointer (via `GetActivateResult`). After this write, the capture thread owns it exclusively. The mutex provides the memory-ordering guarantee.

### `CaptureStart` executable branch table

*Addresses R8 finding 1. Replaces contradictory prose with one authoritative table.*

The rule: once an operation ID is written to `*opId` and `CaptureStart` returns `S_OK`, **all** subsequent outcomes — including timeout, `CoInitializeEx` failure, synchronous activation failure — are delivered through the operation's terminal state via `CaptureGetResult`. Failures before the operation ID is published return an error HRESULT directly with no operation created and `*opId` not written.

| Failure point | Function HRESULT | `*opId` written? | Registry membership | Terminal publisher | Callback expected? | Wakes/signals | Cleanup owner |
|---|---|---|---|---|---|---|---|
| Null `deviceId` or `notifyEvent` or `opId` | `E_POINTER` / `E_HANDLE` | No | None | N/A | No | None | Caller |
| ID exhaustion (all IDs occupied) | `E_OUTOFMEMORY` | No | None | N/A | No | None | Caller |
| Internal allocation failure (session state) | `E_OUTOFMEMORY` | No | None | N/A | No | None | Caller |
| Thread-creation failure (`CreateThread`) | `E_FAIL` | No | None | N/A | No | None | UI thread frees session state |
| Capture thread `CoInitializeEx` failure | `S_OK` | **Yes** | Active → terminal | Capture thread (via `readyEvent` + `notifyEvent`) | No (activation never launched) | `readyEvent` (failure), `notifyEvent` | Capture thread: no `CoUninitialize` needed (init failed); sets `threadDone`, exits |
| `readyEvent` timeout (5 seconds) | `S_OK` | **Yes** | Active → terminal | UI thread sets terminal `FAILED` (`ERROR_TIMEOUT`), signals capture thread to exit | No (activation never launched) | `captureThreadWakeEvent`, `notifyEvent` | Capture thread: `CoUninitialize`, sets `threadDone`, exits |
| Synchronous `ActivateAudioInterfaceAsync` failure | `S_OK` | **Yes** | Active → terminal | UI thread sets terminal `FAILED` with returned HRESULT | No (launch failed) | `captureThreadWakeEvent` (thread exits), `notifyEvent` | UI thread publishes terminal; capture thread: `CoUninitialize`, sets `threadDone`, exits |
| Async activation failure (callback `GetActivateResult` fails) | `S_OK` | **Yes** | Active → terminal | MTA callback | Yes (fires with failure) | `captureThreadWakeEvent` (null handoff), `notifyEvent` | Callback: release returned interface (if any). Capture thread: sees null handoff, `CoUninitialize`, sets `threadDone`, exits |
| Cancel before callback | `S_OK` | **Yes** | Active → terminal | MTA callback (eventual) | Yes (un-cancellable — OS guarantees callback) | `captureThreadWakeEvent` (cancel wake), `notifyEvent` | Capture thread: sees cancel + null handoff, `CoUninitialize`, sets `threadDone`, exits. Callback: `GetActivateResult`, release interface, sees terminal already set, releases strong ref |
| Cancel after handoff (capture running) | `S_OK` | **Yes** | Active → terminal | Capture thread | N/A (callback completed) | `stopEvent`, `notifyEvent` | Capture thread: `Stop`, `Release` services, `Release` `IAudioClient`, `CoUninitialize`, sets `threadDone`, exits |
| Normal stop (user/lifecycle) | `S_OK` | **Yes** | Active → terminal | Capture thread | N/A | `stopEvent`, `notifyEvent` | Same as cancel after handoff |
| Capture-loop failure (WASAPI error, overflow, discontinuity) | `S_OK` | **Yes** | Active → terminal | Capture thread | N/A | `notifyEvent` | Capture thread: `ReleaseBuffer` (if acquired), `Stop`, `Release`, `CoUninitialize`, sets `threadDone`, exits |

**Cancelled activation is not immediately terminal.** When `CaptureRequestStop(cancel)` is called while activation is in flight, the `cancelled` flag is set and the capture thread is woken to exit, but the operation is **not** terminal until the un-cancellable MTA callback fires, retrieves/releases the `IAudioClient*`, and transitions to terminal state. The capture thread may exit before the callback fires (it sees cancel + null handoff); the callback's strong reference keeps the operation state alive. Terminal state requires that **both** the capture thread's `threadDone` flag is set **and** the callback has completed (released its strong ref or the late-callback has published terminal).

**All initiating exports follow the same rule**: `CaptureStart`, `PickerOpenFile`, `CapPermissionRequest`, `CapEnumerateDevices`, `CapGetDefaultDevice`. Validation/launch failures before the operation is created return an error HRESULT directly. Once `*opId` is written and `S_OK` returned, all outcomes are via query.

### Why this handoff is legal

- Both the MTA callback thread and the helper-owned capture thread are in the MTA. Microsoft explicitly documents that "all the threads in the process that have been initialized as free-threaded reside in a single apartment" and that "interface pointers are passed directly from thread to thread within a multithreaded apartment." `[MS-42]` The `IAudioClient*` pointer stored in the handoff slot is protected by a mutex (memory ordering), and used exclusively by the capture thread after handoff.
- `GetMixFormat`, `Initialize`, `GetService`, `Start`, `Stop`, and `Release` are all called on the same capture thread. `GetMixFormat` runs before `Initialize` because its returned format is the format passed to shared-mode `Initialize` `[MS-38]`. The documented same-thread release rule for `IAudioCaptureClient` and `IAudioClient` is satisfied. `[MS-17] [MS-34]`
- The completion handler COM object is reference-counted. Windows holds a reference until the operation completes. The helper holds its own reference and releases it only after the callback has fired and the handoff is complete. `[MS-5]`
- The capture thread calls `CoInitializeEx(nullptr, COINIT_MULTITHREADED)` before touching any COM pointer. If this fails (returns anything other than `S_OK` or `S_FALSE`), the session fails immediately.

### Cancellation before activation completes

There is no `Cancel` API on `IActivateAudioInterfaceAsyncOperation`. The frozen cancellation path:

1. Go calls `CaptureRequestStop(opId, reason=cancelled)` while activation is in flight.
2. Helper sets `cancelled = true` under the mutex.
3. Helper signals `captureThreadWakeEvent` — the capture thread wakes, sees no handoff (null slot + cancelled), calls `CoUninitialize`, sets `threadDone=1`, and exits (R8-2).
4. The MTA callback eventually fires (guaranteed by the OS).
5. Callback checks `cancelled` under the mutex.
6. If cancelled: calls `GetActivateResult`, releases the returned `IAudioClient*` immediately (on the callback's MTA thread — legal since `Initialize`/`GetService` were never called, so no service release ordering applies). Sets terminal state to `cancelled` (atomic). Signals `notifyEvent` — Go sees terminal.
7. **After signaling, the callback releases its strong operation reference** (see §Callback strong-reference lifetime). Only after this release can the operation's destructor fire.
8. The operation destructor checks `threadDone` (never joins the thread — R8-2). In the cancel case, the thread has typically already exited (step 3) and `threadDone=1`.

### Shutdown without UI-thread deadlock

`CaptureRequestStop` is non-blocking: it sets the stop flag and reason, signals `stopEvent`. The capture thread (or the callback, if activation is still in flight) handles cleanup on its own thread. The UI thread never waits for the capture thread to join — it posts the stop and continues pumping messages. The capture thread's final state is communicated back to Go via the notification event and `CaptureGetResult`.

---

## Callback strong-reference lifetime

*Addresses R5 finding 1.*

### Problem

The R4 design said "No ref-counted session lifetime is needed" because `CapDestroy` requires all operations to be terminal. But the race is not at `CapDestroy` — it is at `CaptureRelease` / `PickerRelease` / `CapPermissionRequestRelease`. The activation-cancel path sets terminal state and signals Go **before the callback has returned**; Go can immediately call `CaptureRelease` and free the session while callback code (after the `SetEvent` call but before `return`) is still executing on the system MTA thread. The same race exists for picker completion, permission-request completion, enumeration/default-device completions, and `AccessChanged` racing `CapPermissionUnsubscribe`.

### Frozen contract

Every async callback and event handler in the helper holds a **strong reference** (C++ `shared_ptr` or explicit ref-count increment) to its owning operation's state for the entire duration of the callback — from entry to final return/destructor. The operation state is ref-counted with at least two reference holders:

1. **Registry reference**: held by the helper's operation registry (the map keyed by operation ID). Dropped by `CaptureRelease` / `PickerRelease` / `CapPermissionRequestRelease` / `CapEnumerateDevicesRelease` / `CapGetDefaultDeviceRelease`.
2. **Callback reference**: held by each in-flight callback or event handler. Acquired before the callback is registered with the OS/WinRT; released as the callback's final action before returning.

The operation's destructor (which frees memory, closes internal handles, etc.) runs only when the reference count reaches zero — i.e., when **both** the registry has released and **all** callbacks have returned. The destructor never joins threads (R8-2): it checks `threadDone` (an atomic flag set by the capture thread as its final instruction), but never waits on it — if `threadDone` is not set, `CapDestroy` would have rejected the call. Since the DLL is process-lifetime loaded, no module-unload join is needed.

### Exact ordering per callback type

#### Activation (`ActivateCompleted`)

1. Callback acquires strong ref at entry (already held from registration).
2. Callback performs its work: check cancelled, `GetActivateResult`, handoff or cleanup.
3. Callback sets terminal state under mutex.
4. Callback calls `SetEvent(notifyEvent)` — Go wakes.
5. Callback releases its strong ref (its final action).
6. Callback returns to the OS.
7. Go sees terminal state, calls `CaptureRelease`.
8. `CaptureRelease` drops the registry ref.
9. If the callback has already completed step 5: ref-count reaches zero, destructor fires, memory freed.
10. If the callback has NOT completed step 5 (race): registry ref is gone but callback ref keeps state alive. Callback completes step 5, ref-count reaches zero, destructor fires.

#### Picker completion

Same pattern: picker async callback holds a strong ref from registration through return. `PickerRelease` drops the registry ref. The picked `StorageFile`, file handle, and display name remain valid until the destructor fires (after both refs are released).

#### Permission request (`RequestAccessAsync` completion)

Same pattern.

#### Enumeration / default-device completion

Same pattern.

#### `AccessChanged` event handler and unsubscribe fence

*Addresses R6 finding 2.*

The `AccessChanged` handler is a subscription, not a one-shot operation. Microsoft's WinRT documentation explicitly warns that an asynchronous event may reach its recipient after revocation has begun — the system may have already dispatched the delegate before the revocation takes effect.

**Duplicated notification handle.** At `CapPermissionSubscribe` time, the helper calls `DuplicateHandle` on Go's `notifyEvent` to create a subscription-owned copy. All `AccessChanged` handler invocations signal this **duplicate**, never Go's original. This eliminates the handle race: Go can safely close or reuse its original `notifyEvent` immediately after `CapPermissionUnsubscribe` returns, because no handler — in-flight or future — will ever signal the original.

**Subscription state and strong references.** The subscription state is ref-counted (`shared_ptr<SubscriptionState>` — R8-3), containing:
- The duplicated notification `HANDLE`
- A **strong** `AppCapability` object reference (for `CheckAccess` — R8-3; not a raw pointer or `weak_ref`)
- The WinRT event token (for revocation)
- An atomic handler-in-flight count
- A mutex protecting `CheckAccess` calls from racing `CapDestroy` (R8-3)

The C++/WinRT delegate registered with `AccessChanged` captures a `shared_ptr<SubscriptionState>`. Each handler invocation:
1. The captured `shared_ptr` is already a strong ref (acquired at delegate copy/dispatch, before handler entry — this protects the dispatch-to-entry interval).
2. Handler performs work: calls `CheckAccess`, calls `SetEvent(duplicatedHandle)`.
3. Handler returns; the delegate's `shared_ptr` copy is released.

**Cycle breaking.** `AppCapability` holds a reference to the delegate (via the registered event token). The delegate holds a `shared_ptr<SubscriptionState>`. The subscription state holds a **strong** reference to the `AppCapability` (R8-3) for `CheckAccess`. This creates a cycle: `AppCapability` → delegate → `SubscriptionState` → `AppCapability`. The cycle is explicitly broken at unsubscribe time: `CapPermissionUnsubscribe` revokes the event token (which causes `AppCapability` to release its reference to the delegate), then the subscription state's destructor releases the `AppCapability` reference. An in-flight handler's `shared_ptr<SubscriptionState>` keeps both the subscription state and the `AppCapability` alive until the handler returns.

**Unsubscribe sequence:**
1. `CapPermissionUnsubscribe` revokes the WinRT event token (prevents new dispatches from `AppCapability`).
2. Drops the registry reference to the subscription state.
3. Returns immediately.
4. If a handler is currently in-flight (its delegate copy holds a `shared_ptr`), the subscription state survives until that handler returns and its `shared_ptr` is released.
5. The subscription state destructor closes the duplicated handle and releases any remaining resources.

**Go-side safety:** Go calls `CapPermissionUnsubscribe`, then may immediately close its original notification `HANDLE`. The in-flight handler (if any) operates only on the duplicated handle inside the subscription state — Go's handle is never at risk. `CapDestroy` still checks the global callback ref-count and rejects if any handler is in-flight (its `shared_ptr` adds to the count).

**Idempotence:** Calling `CapPermissionUnsubscribe` when no subscription is active returns `S_OK` (no-op). Calling it twice returns `S_OK` (already unsubscribed).

### Module lifetime — process-lifetime loader, no `FreeLibrary`

*Addresses R6 finding 1.*

The helper DLL is loaded once at startup and **never unloaded** during the process lifetime. `FreeLibrary` is never called. `CapDestroy` tears down application state only (operation registry, subscription state, internal threads); the DLL's code and static data remain mapped. The module is reclaimed at process exit (the OS unmaps all process memory).

**Why a process-lifetime loader is necessary.** An operation-state `shared_ptr` is not a module-lifetime fence. The global ref-count can reach zero after `SetEvent` (step 4 in the activation ordering) and immediately before the callback epilogue (step 5–6). Meanwhile, the system can release the activation handler or async-operation COM references after `ActivateCompleted` returns — and those `Release` implementations live in the helper DLL's code. If another thread lets `CapDestroy` succeed and calls `FreeLibrary` while callback or COM `Release` code from the DLL is still executing, the process crashes on an unmapped code page. This race is narrow but unfixable without either (a) the OS guaranteeing that all system-held references to handler COM objects are released synchronously inside `ActivateCompleted` before it returns (not documented), or (b) a proven module-pin design covering every code path from callback entry through the system's final COM Release, which includes `IActivateAudioInterfaceAsyncOperation`, the completion handler, and every C++/WinRT async operation/delegate.

**Process-lifetime loading eliminates the race entirely**: no `FreeLibrary` call exists, so no code path can unmap the DLL while any code — application callback, system COM Release, or DLL epilogue — is executing.

#### COM object ownership and release graph

The following COM objects and C++/WinRT delegates are created by the helper and may be held by the OS after their respective callbacks return:

| Object | Created by | OS holds reference until | Helper releases |
|---|---|---|---|
| `IActivateAudioInterfaceCompletionHandler` impl | `CaptureStart` (at `ActivateAudioInterfaceAsync` call) | Documented: "until the activation operation completes and the `IActivateAudioInterfaceAsyncOperation` interface is released" `[MS-5]` | After callback returns — part of operation state destructor |
| `IActivateAudioInterfaceAsyncOperation` | OS (returned by `ActivateAudioInterfaceAsync`) | Application owns the reference | Release is part of activation cleanup (callback or capture thread) |
| C++/WinRT `FileOpenPicker` completion delegate | `PickerOpenFile` | Until the WinRT async operation completes | Part of picker operation state destructor |
| C++/WinRT `RequestAccessAsync` completion delegate | `CapPermissionRequest` | Until the WinRT async operation completes | Part of permission-request operation state destructor |
| C++/WinRT `FindAllAsync` completion delegate | `CapEnumerateDevices` | Until the WinRT async operation completes | Part of enumeration operation state destructor |
| C++/WinRT `AccessChanged` event delegate | `CapPermissionSubscribe` | Until token revocation + in-flight dispatch completes | Subscription state destructor (see §AccessChanged unsubscribe fence) |

**Synchronous launch failure**: if `ActivateAudioInterfaceAsync` returns an error HRESULT synchronously (before any callback is registered), no COM objects need cleanup — the session transitions to `FAILED` immediately. The same applies to C++/WinRT `PickSingleFileAsync`, `RequestAccessAsync`, etc. — if the launch itself throws, the operation transitions to `FAILED` and no delegate is registered.

**Cycle breaking** (R7-4): C++/WinRT async operations hold a reference to the completion delegate, and the delegate captures a `shared_ptr` to the operation state. This creates a cycle: async operation → delegate → state → (may hold) async operation. The cycle is explicitly broken at the callback point: after the callback publishes terminal state and signals `notifyEvent`, it **clears its reference to the WinRT async operation object** (sets the stored `IAsyncOperation`/`IActivateAudioInterfaceAsyncOperation` to `nullptr`). This breaks the cycle and allows the WinRT runtime to release its reference to the delegate when it finishes its own cleanup. Without this explicit clearing, a destructor that needs the cycle broken cannot run because the cycle prevents it from being invoked. For `AccessChanged`, the `AppCapability` → delegate cycle is broken at token revocation (see §AccessChanged unsubscribe fence).

#### `CapDestroy` and module state

`CapDestroy` checks the global callback ref-count and returns `E_ILLEGAL_METHOD_CALL` if any callback reference is still live. After `CapDestroy` succeeds, all application state is freed, but the DLL module remains loaded. Only `CapInit` can re-initialize. The module is reclaimed at process exit.

### Adversarial tests

The probe must include tests that exercise the callback-release race:

1. **Capture cancel + immediate release**: Start `CaptureStart`, immediately call `CaptureRequestStop(cancel)`. On terminal event, call `CaptureRelease` with zero delay. Verify no crash/use-after-free. Run under ASAN or page-heap if available.
2. **Picker cancel + immediate release**: Open picker, simulate user cancel. On terminal event, call `PickerRelease` with zero delay.
3. **Permission unsubscribe racing `AccessChanged`**: Subscribe to `AccessChanged`. Revoke microphone permission in system settings (to trigger the handler). Call `CapPermissionUnsubscribe` while the handler may be in-flight. Verify no crash. The handler's strong ref must keep state alive. Verify Go's original `notifyEvent` handle can be closed immediately after `CapPermissionUnsubscribe` returns without affecting the in-flight handler (which signals only the duplicated handle).
4. **Rapid start/stop/release cycles**: Repeat capture start → stop → wait terminal → release 100 times in a tight loop. No leaked refs, no crash.
5. **Deterministic callback barrier** (R6-1): inject a test barrier (e.g., a `WaitForSingleObject` on a test event) in the activation callback after the terminal-state publication and `SetEvent(notifyEvent)` call but before the callback epilogue (strong-ref release + return). While the callback is held at the barrier, Go wakes from the notification event, calls `CaptureRelease`, and (if applicable) `CapDestroy`. Verify that `CapDestroy` returns `E_ILLEGAL_METHOD_CALL` (callback ref still live). Release the barrier. Verify that the callback completes, its strong ref is released, and a subsequent `CapDestroy` call succeeds. This is a deterministic barrier test — not a zero-delay stress loop that relies on timing to hit the race.
6. **Unsubscribe fence barrier** (R6-2): inject a test barrier in the `AccessChanged` handler after acquiring the strong ref but before calling `SetEvent`. Call `CapPermissionUnsubscribe` from Go while the handler is held. Verify unsubscribe returns, Go closes its original notification handle, and no crash occurs. Release the barrier. Verify the handler completes, signals the duplicated handle (now closed as part of subscription-state destruction), and the ref-count reaches zero cleanly.
7. **Injected CoInitializeEx failure** (R6-4): mock `CoInitializeEx` on the capture thread to return `RPC_E_CHANGED_MODE`. Verify that `CaptureStart` transitions to `FAILED` immediately, no activation is launched, and no COM pointers leak. Verify exact release counts.
8. **Injected activation launch failure** (R6-4): mock `ActivateAudioInterfaceAsync` to return an error HRESULT synchronously. Verify that the capture session transitions to `FAILED`, no callback is registered, no COM objects leak, and the capture thread exits cleanly.
9. **readyEvent timeout** (R7-4): inject a blocking delay in the capture thread before `CoInitializeEx` so that `readyEvent` is never signaled within 5 seconds. Verify that `CaptureStart` returns `S_OK` with a valid `opId`, the operation transitions to `FAILED` with `ERROR_TIMEOUT`, the capture thread is signaled to exit and eventually does, and `CaptureRelease` + `CapDestroy` succeed after thread exit.
10. **Cancellation wakes capture thread** (R7-4): start `CaptureStart`, wait for `readyEvent` (thread is MTA-ready), call `CaptureRequestStop(cancel)` before activation completes. Verify that `captureThreadWakeEvent` is signaled, the capture thread wakes, sees no handoff, calls `CoUninitialize`, releases its strong ref, and exits. Verify exact thread-exit event, object counts, and that the operation destructor waits on thread exit.
11. **Terminal state after CoUninitialize** (R7-4): inject a barrier in the capture thread after `CoUninitialize` but before terminal-state publication. Verify that `CaptureGetResult` does not return terminal while the barrier is held (COM cleanup is still in progress). Release the barrier. Verify terminal state is published, `notifyEvent` is signaled, and the thread-exit event follows.
12. **Synchronous activation failure + thread cleanup** (R7-4): mock `ActivateAudioInterfaceAsync` to fail synchronously. Verify that the capture thread is signaled to exit (via `captureThreadWakeEvent`), calls `CoUninitialize`, releases its strong ref, and exits. Verify no leaked threads. Verify the handler/session/thread COM objects are released despite the failure path.
13. **C++/WinRT cycle breaking** (R7-4): inject a test where the callback completes but does NOT clear its async-operation reference. Verify that the cycle prevents destruction (leak). Then run normally with the clearing step. Verify clean destruction and zero live objects.
14. **Capture thread last-ref no-deadlock** (R8-2): start `CaptureStart`, wait for activation + capture. While the capture thread is running, call `CaptureRequestStop(user_stop)`. On terminal event, call `CaptureRelease`. Verify that the operation destructor does NOT wait on the capture thread (no `WaitForSingleObject` on self). If the capture thread is the last entity touching the session state, verify it sets `threadDone=1` and exits without triggering a destructor that joins itself. Inject a barrier in the capture thread after terminal publication but before `threadDone=1`; while the barrier is held, have Go observe terminal and call `CaptureRelease`; verify no deadlock, no crash; release the barrier; verify `threadDone` transitions to 1 and a subsequent `CapDestroy` succeeds.

---

## Asynchronous ABI design

*Addresses R2 finding 1.*

### Design principle

C++/WinRT explicitly warns: calling `.get()` on the UI thread "is not concurrent nor asynchronous, so it's not appropriate for a UI thread (and an assertion will fire in unoptimized builds if you attempt to use it on one)." `[MS-39]`

Every native export that wraps a WinRT async operation, WASAPI activation, or picker dialog follows an **initiate → event/message → query/take-result** contract:

1. **Initiate**: Go calls the export on the UI thread. The export validates inputs, allocates an operation with a unique request ID, starts the async work, and returns the request ID immediately. The return HRESULT reports only validation/launch errors, not the operation outcome.
2. **Event**: when the async work completes, the helper signals Go's notification event (`HANDLE`). Go's event loop (`WaitForMultipleObjects` or equivalent) wakes.
3. **Query**: Go calls a result query export (`*GetResult`) with the request ID. This returns the operation's terminal state, HRESULT, and any output data. The query is non-blocking and may be called from any thread.

No native callback jumps into Go. No native export blocks waiting for an async operation to finish.

### Request IDs and operation states

Each initiated operation gets a `uint32_t` request/operation ID, unique within the helper's lifetime (monotonically incrementing, wrapping at 2^32 — no reuse while a prior operation with the same ID is still queryable).

Operation states:

| State | Value | Meaning |
|---|---|---|
| `PENDING` | 0 | Operation initiated, not yet completed |
| `SUCCEEDED` | 1 | Operation completed successfully; results available |
| `FAILED` | 2 | Operation completed with an error; HRESULT available |
| `CANCELLED` | 3 | Operation was cancelled before completion |
| `DENIED` | 4 | Access/permission was denied |

Go queries `*GetResult(opId, ...)` and receives the state plus any output.

### Operation-ID wrap and exhaustion

*Addresses R3 finding 7 (partial).*

Operation IDs are monotonically incrementing `uint32_t` values starting at 1. ID 0 is reserved (invalid). The sequence wraps at `UINT32_MAX` back to 1. Before assigning a new ID, the helper checks that the candidate is not currently occupied by an active (non-released) operation. If every ID in the `uint32_t` space is occupied (theoretically 4,294,967,295 concurrent operations — impossible given the one-at-a-time limits), the **initiating export** (e.g. `CaptureStart`, `PickerOpenFile`) fails with `E_OUTOFMEMORY` — not `CapInit`, which has no involvement in ID allocation (R7-4 correction of stale sentence). In practice, the one-active-per-category limits (§Ownership rules) mean at most ~5 operations exist simultaneously; wrap is a theoretical safety, not a realistic concern.

### Notification event semantics

*Addresses R3 finding 2 (partial). False "wakes once" guarantee removed per R5 finding 4.*

Go creates notification events as **auto-reset** events (`CreateEvent(nullptr, FALSE, FALSE, nullptr)`). An auto-reset event returns to non-signaled after a single `WaitForMultipleObjects`/`WaitForSingleObject` wakes.

**Auto-reset events are readiness hints only.** `SetEvent` calls can coalesce: if the helper signals multiple completions rapidly (e.g., capture data + permission change) while Go has not yet returned from the previous wait, the signals merge into a single wake. Go must therefore **query all pending operations on every wake**, not assume a single operation completed. This is the only correct interpretation — the event says "something may be ready," not "exactly one thing completed."

The drain rule: on every wake from a notification event, Go calls every `*GetResult` export for operations associated with that event. Only `PENDING` results are skipped; all non-`PENDING` results are consumed. For capture specifically, Go calls `CaptureRead` in a loop until it returns `S_FALSE` (no data available), then queries `CaptureGetResult` for state changes (format available, terminal state). For `AccessChanged` subscription, Go calls `CapPermissionCheck` to read the current status. This ensures coalesced signals do not lose data or state transitions.

### Exact WASAPI packet drain loop

*Addresses R4 finding 3. Discontinuity and whole-packet ring preflight per R7 finding 3.*

Auto-reset events do not guarantee "one wake per `SetEvent`": `SetEvent` calls can coalesce if the event is already signaled (e.g., the wait has not yet returned from the previous signal). The capture thread must treat the notification as a **readiness hint** — "there may be data" — and drain all available packets:

```
// State: isFirstPacket = true (set at capture start)
//
// On every captureDataEvent wake:
for (;;) {
    UINT32 packetSize = 0;
    hr = captureClient->GetNextPacketSize(&packetSize);
    if (FAILED(hr)) { → terminal FAILED with hr; break; }
    if (packetSize == 0) break;  // all packets consumed

    BYTE *data; UINT32 frames; DWORD flags;
    hr = captureClient->GetBuffer(&data, &frames, &flags, nullptr, nullptr);
    if (FAILED(hr)) { → terminal FAILED with hr; break; }

    // A WASAPI packet is now acquired — it MUST be released before
    // calling GetBuffer again or before stopping the client.

    // --- Flag handling (R7-3) ---
    if (flags & AUDCLNT_BUFFERFLAGS_DATA_DISCONTINUITY) {
        if (isFirstPacket) {
            // Expected: first packet after Start() commonly carries this flag
            // (stream transition). Accept it — no integrity concern.
        } else {
            // Non-first-packet discontinuity: data integrity compromised.
            // Release the acquired packet, then transition to terminal failure.
            captureClient->ReleaseBuffer(frames);
            → terminal FAILED with CAP_REASON_DISCONTINUITY; break;
        }
    }
    if (flags & AUDCLNT_BUFFERFLAGS_TIMESTAMP_ERROR) {
        // Timestamp is unreliable. Logged for evidence but accepted —
        // the recording does not consume device-position timestamps.
        log timestamp error for evidence;
    }
    isFirstPacket = false;

    // --- Whole-packet ring preflight (R7-3) ---
    // Check ring capacity BEFORE conversion/copy.
    uint32_t requiredFloats = frames * channels;
    if (ring.availableForWrite() < requiredFloats) {
        // Insufficient room for the entire packet. Do NOT write a
        // partial packet — that would leave discontinuous audio.
        // Release the acquired WASAPI packet first.
        captureClient->ReleaseBuffer(frames);
        → terminal FAILED with CAP_REASON_OVERFLOW; break;
    }

    if (flags & AUDCLNT_BUFFERFLAGS_SILENT) {
        write zeros to recording ring (frames * channels floats)
    } else {
        // Convert into scratch buffer first (R8-6). The producer index
        // is published only after the entire packet converts successfully.
        convert data to float32 into scratchBuf per §Frozen sample representation
        if conversion fails:
            ReleaseBuffer(frames);
            → terminal FAILED with CAP_REASON_FORMAT_ERROR; break;
        copy scratchBuf to recording ring (publish producer index)
    }

    hr = captureClient->ReleaseBuffer(frames);
    if (FAILED(hr)) { → terminal FAILED with hr; break; }
}
```

**Whole-packet ring preflight** (R7-3): the previous design copied frames into the ring and then checked whether the ring was full. This could overrun the ring (writing beyond capacity) or leave a partial packet (some frames written, remainder dropped). The corrected design checks ring capacity **before** conversion/copy. If the ring cannot hold the entire packet, zero frames are written, the WASAPI packet is released via `ReleaseBuffer`, and the session transitions to overflow failure. The ring capacity check is a single atomic read of the write-available count.

**Scratch-buffer conversion** (R8-6): a single atomic capacity read does not itself guarantee that a conversion error cannot publish a partial packet to the consumer. The capture thread converts into a pre-allocated scratch buffer (`scratchBuf`, sized to `maxFrames * channels * sizeof(float32)` where `maxFrames = bufferFrames`). The producer index is published (ring write committed) only after the **entire packet** converts successfully into the scratch buffer. On conversion failure or `ReleaseBuffer` failure, zero frames become visible to the consumer — the ring's producer index is not advanced. The consumer racing a conversion failure sees the ring as unchanged. Test: consumer reads while a conversion failure occurs mid-packet; verify zero new frames are visible and the ring is in a consistent state.

**WASAPI buffer flag handling** (R7-3):
- `AUDCLNT_BUFFERFLAGS_DATA_DISCONTINUITY`: the first packet after `IAudioClient::Start()` commonly carries this flag (documented as a stream transition signal). Subsequent packets with this flag indicate a timing glitch where WASAPI lost samples between packets. Accepting this while treating an app-ring overflow as fatal would be inconsistent — both produce corrupted, discontinuous audio. The policy: first packet accepts it; any subsequent discontinuity is terminal `CAP_REASON_DISCONTINUITY` (non-promotable). The exact HRESULT stored is `HRESULT_FROM_WIN32(ERROR_BUFFER_OVERFLOW)` (same as overflow — both are data-integrity failures).
- `AUDCLNT_BUFFERFLAGS_TIMESTAMP_ERROR`: the device position/QPC timestamp is unreliable. Since the recording does not consume device-position timestamps (it writes frames sequentially), this flag is logged for evidence but accepted.
- `AUDCLNT_BUFFERFLAGS_SILENT`: handled as before — write zeros.

**Error cleanup for an acquired packet**: if conversion fails while a packet is acquired (between `GetBuffer` and `ReleaseBuffer`), the capture thread **must** call `ReleaseBuffer(frames)` before transitioning to terminal state. Leaving a buffer acquired prevents `IAudioClient::Stop()` from completing cleanly. The sequence on mid-packet failure is: `ReleaseBuffer` → `Stop` → release services → release `IAudioClient` → `CoUninitialize` → terminal state → strong-ref release → thread exit.

**Required tests** (R7-3):
1. **Exact-fit**: ring has exactly `frames * channels` room; packet writes successfully, ring is now full.
2. **One-frame-short**: ring has `(frames - 1) * channels` room; preflight rejects, `ReleaseBuffer` called, terminal overflow.
3. **Concurrent consumer**: Go drains `CaptureRead` while capture thread writes; verify no data corruption or ring-pointer race.
4. **Silent packet**: `AUDCLNT_BUFFERFLAGS_SILENT` produces zeros in the ring.
5. **First-packet discontinuity**: first packet after `Start()` with `DATA_DISCONTINUITY` flag is accepted.
6. **Subsequent discontinuity**: second packet with `DATA_DISCONTINUITY` → terminal `CAP_REASON_DISCONTINUITY`.
7. **Timestamp error**: `TIMESTAMP_ERROR` flag is logged but packet is accepted.
8. **Conversion error mid-packet**: injected conversion failure (e.g. unsupported subtype discovered mid-stream) → `ReleaseBuffer` called → terminal.
9. **Stop while acquired**: `stopEvent` signaled while a packet is acquired → complete current `ReleaseBuffer` before stop sequence.

**Stop racing a packet**: if `stopEvent` is signaled while the drain loop is running, the loop finishes the current packet (release the acquired buffer), then exits normally into the stop sequence. `WaitForMultipleObjects` with both events returns `WAIT_OBJECT_0 + index`; the stop event is checked before re-entering the packet loop.

### Go-side drain protocol

*Addresses R4 finding 3. `CapPermissionCheck` added to drain list per R5 finding 4.*

On every wake from the notification event, Go executes the following drain sequence:

1. Call `CaptureRead(opId, buf, maxFrames, &framesRead)` in a loop until it returns `S_FALSE` (no data). Write each batch of frames to the evidence WAV file.
2. Call `CaptureGetResult(opId, &state, &format, &framesAvailable, &hresult)` to check for state transitions (activation complete, terminal state).
3. If `AccessChanged` subscription is active: call `CapPermissionCheck(&status)` to read the current permission status. If status changed to denied, initiate `CaptureRequestStop(opId, permission_revoke)`.
4. Call `CapPermissionRequestResult` / `CapEnumerateDevicesResult` / `CapGetDefaultDeviceResult` / `PickerGetResult` for any other pending operations associated with this event.
5. If any result has transitioned to terminal state, process it (release, error handling, etc.).

This protocol ensures that coalesced auto-reset signals do not lose data or state transitions.

### UI-thread event/message integration

*Addresses R8 finding 5.*

The live `pulsar-win` main thread blocks in `GetMessageW` (`pumpMessages()` in `ui_windows.go`). A second independent `WaitForMultipleObjects` blocking loop cannot coexist on the same OS thread — they would race for ownership of the thread's wait state. The two blocking APIs are incompatible on a single thread.

**Frozen integration: dedicated waiter goroutine.**

A new goroutine, pinned to its own OS thread via `runtime.LockOSThread()`, owns the helper event wait loop:

```
Waiter goroutine (separate OS thread, LockOSThread)
  │
  ├── WaitForMultipleObjects({captureNotifyEvent, pickerNotifyEvent,
  │     permissionNotifyEvent, enumerationNotifyEvent, shutdownEvent})
  │
  ├── On wake:
  │     1. Drain CaptureRead until S_FALSE (thread-safe — CaptureRead
  │        may be called from any thread per §Ownership rules).
  │     2. Write drained frames to .partial (Go file I/O, no UI dependency).
  │     3. Query CaptureGetResult, CapPermissionCheck, and all pending
  │        operation results (thread-safe — query exports allow any thread).
  │     4. For UI-affecting actions (permission status change → show prompt,
  │        terminal capture state → update tray, picker result → process file):
  │        PostMessageW(lifecycleHwnd, WM_APP+N, ...) to the UI thread.
  │     5. Loop back to WaitForMultipleObjects.
  │
  ├── On shutdownEvent: exit loop, return
  │
  UI thread (pinned main goroutine, GetMessageW loop)
  │
  ├── Receives WM_APP+N from waiter → executes UI actions:
  │     - CaptureRequestStop (if permission revoked)
  │     - Update tray menu state
  │     - Restore visible window for picker
  │     - Start new CaptureStart / PickerOpenFile (UI-thread-only exports)
  │
  ├── Receives WM_HOTKEY, WM_QUERYENDSESSION, WM_ENDSESSION,
  │     WM_POWERBROADCAST, WM_WTSSESSION_CHANGE from the OS
  │     → calls CaptureRequestStop (non-blocking, UI-thread call is legal)
  │
  ├── Receives tray callback, onboarding messages as before
```

**Why not `MsgWaitForMultipleObjectsEx`.** While `MsgWaitForMultipleObjectsEx` can atomically wait for both handles and messages on one thread, the existing codebase uses `pGetMessageW.Call` (a raw `user32.dll` proc), not the Go standard library. Combining `MsgWaitForMultipleObjectsEx` with the existing `syscall.NewCallback`-based wndproc requires careful re-dispatch logic and risks starving either messages or handle events. The dedicated waiter goroutine is simpler and avoids modifying the existing proven message pump. `CapInit`, `CapDestroy`, `CaptureStart`, `PickerOpenFile`, and `CapPermissionRequest` remain on the exact UI thread where WinRT requires them — the waiter only calls thread-safe query/read exports and uses `PostMessageW` for UI actions.

**Event handle lifetime and waiter shutdown.** The waiter goroutine creates a `shutdownEvent` (manual-reset) and includes it in the wait array. On app shutdown (`OnQuit` or `WM_ENDSESSION`), Go signals `shutdownEvent`; the waiter wakes, performs a final drain of all operations, and exits. Event handles for helper operations are valid for the waiter's lifetime because they are created before the waiter starts and closed only after the waiter exits and all operations are released.

**Required test** (R8-5): a coalesced-signal test where the helper signals capture data, `AccessChanged`, and picker completion in rapid succession (coalesced into one wake) while `WM_ENDSESSION` is also queued on the UI thread. Verify that the waiter drains all three results, the UI thread receives and processes `WM_ENDSESSION`, and no data or state transition is lost.

### Operation release/take semantics

*Addresses R3 finding 2 (partial). Callback strong-ref interaction per R5 finding 1.*

Every initiate export (`CaptureStart`, `CapPermissionRequest`, `CapEnumerateDevices`, `CapGetDefaultDevice`, `PickerOpenFile`) creates an operation that must be released after reaching terminal state:

| Operation | Terminal states | Release export | Multiple calls to result query |
|---|---|---|---|
| Capture | stopped, failed, cancelled | `CaptureRelease` | Allowed (idempotent read) |
| Permission request | completed, failed | `CapPermissionRequestRelease` | Allowed |
| Device enumeration | completed, failed | `CapEnumerateDevicesRelease` | Allowed |
| Default device | completed, failed | `CapGetDefaultDeviceRelease` | Allowed |
| Picker | picked, cancelled, failed | `PickerRelease` | `takeHandle=0` probes size (no transfer, repeatable); `takeHandle=1` transfers HANDLE exactly once (subsequent `takeHandle=1` returns `S_OK` with `*hresult` unchanged (operation outcome) and `*handleTaken=0`); `PickerRelease` closes any untaken handle |

Every release export:

1. Returns `E_ILLEGAL_METHOD_CALL` if the operation is still in `PENDING` state.
2. **Drops the registry reference** to the operation. If the callback's strong reference has already been released (the normal case), the ref-count reaches zero and the operation's memory is freed. If a callback is still executing (race case), the operation remains alive until the callback releases its strong ref (see §Callback strong-reference lifetime).
3. Invalidates the operation ID in the registry.
4. Is idempotent: calling with an already-released or unknown ID returns `S_OK` (no-op).

### `CapDestroy` — requires empty operation registry

*Addresses R3 finding 2 (partial). Forced path removed per R4 finding 2. Explicit unsubscribe required per R5 finding 4. Empty-registry requirement per R6 finding 5.*

`CapDestroy` **always** returns `E_ILLEGAL_METHOD_CALL` if any of the following conditions hold:

1. **The operation registry is not empty.** Every operation (capture, picker, permission request, enumeration, default-device query) must be both terminal AND released (via its `*Release` export). A terminal but unreleased operation still occupies the registry and retains its result/event contract — it is not eligible for `CapDestroy`.
2. **The permission subscription is not fully unwound.** Explicit `CapPermissionUnsubscribe` must have been called AND the subscription state destructor must have completed (the duplicated handle closed, all in-flight handler strong refs released). `CapDestroy` does **not** auto-unsubscribe.
3. **Any callback strong reference is still live** (a callback or handler is still executing DLL code — global callback ref count > 0).
4. **A capture thread has not completed.** The capture thread's `threadDone` atomic flag must be 1 (terminal state reached, COM objects released, `CoUninitialize` called, no further session access). `CapDestroy` checks `threadDone` — it never joins the thread (R8-2).

The caller must: `CaptureRequestStop` → wait for terminal state → `CaptureRelease`; similarly release every other operation; call `CapPermissionUnsubscribe`; wait for all callback references to drain; then call `CapDestroy`.

After `CapDestroy` succeeds, all application state is freed. The DLL module remains loaded (process-lifetime — see §Module lifetime). Only `CapInit` can re-initialize.

**Operation-ID exhaustion** (R6-5 correction): ID allocation fails the **initiating export** (e.g. `CaptureStart` returns `E_OUTOFMEMORY`), not `CapInit`. `CapInit` has no involvement in ID allocation.

**Required tests** (R6-5): terminal-but-unreleased operation blocks `CapDestroy`; callback-held-after-release blocks `CapDestroy`; active subscription blocks `CapDestroy`; unsubscribe-in-flight (handler strong ref still live) blocks `CapDestroy`; repeated `CapDestroy` after success is idempotent; `CapInit` after `CapDestroy` re-initializes cleanly.

#### Imminent process termination (`WM_ENDSESSION`)

On `WM_ENDSESSION` with `wParam == TRUE` (confirmed shutdown), Go calls `CaptureRequestStop(opId, shutdown)` and returns from the wndproc immediately **without** calling `CapDestroy`. The OS reclaims all process resources (memory, handles, COM references) when the process exits. No `CapDestroy` call is attempted — the previous design tried to free state while un-cancellable callbacks were pending, which is a contradiction.

#### Lifetime reference graph

Every holder of a reference to helper-internal state:

| Holder | When acquired | When released | Can outlive release export? |
|---|---|---|---|
| Registry (operation map) | Operation initiation | `*Release` export call | N/A — this IS the release |
| `ActivateCompleted` callback (system MTA thread) | `ActivateAudioInterfaceAsync` initiation | Callback's final instruction before return | **Yes** — strong ref keeps state alive if release races callback |
| Capture thread | Created at `CaptureStart` (before activation — R6-4). Thread does **not** hold a ref-counted session reference (R8-2 — eliminates self-join deadlock). Thread accesses session state under mutex/atomics only. | Thread sets atomic `threadDone=1` as its final instruction before returning from the thread function. | **No** — the thread never holds a reference that can trigger the destructor. The destructor checks `threadDone` but never joins the thread. `CapDestroy` requires `threadDone==1`. |
| `AccessChanged` event handler (per invocation) | Delegate copy/dispatch (before handler entry — R6-2) | Handler return releases the `shared_ptr` copy | **Yes** — handler's strong ref keeps subscription state (including duplicated handle) alive if `CapPermissionUnsubscribe` races handler |
| Picker async completion handler | `PickerOpenFile` initiation | Callback return | **Yes** — strong ref keeps state alive if `PickerRelease` races callback |
| Permission request completion handler | `CapPermissionRequest` initiation | Callback return | **Yes** — same pattern |
| `IActivateAudioInterfaceAsyncOperation` | Returned by `ActivateAudioInterfaceAsync` | Released in activation cleanup (callback or capture thread) | No — released within the session's stop sequence |
| `IActivateAudioInterfaceCompletionHandler` impl | `CaptureStart` (registered with OS) | OS releases after operation completes; helper releases via operation state destructor | **Yes** — OS may hold a reference beyond callback return (documented `[MS-5]`). Process-lifetime DLL loading prevents unload during this window. |

`CapDestroy` requires an **empty operation registry** (all operations released), a fully unwound permission subscription, a zero global callback ref-count, and `threadDone==1` for any capture operation that was started. The module remains loaded (process-lifetime — R6-1). In the common case (no race), all callbacks have returned and all threads have exited before the terminal state is observed. In the race case, the strong-ref mechanism ensures safety without `CapDestroy` needing to know about specific in-flight callbacks — it simply checks the global callback count, confirms `threadDone`, and verifies an empty registry. The destructor never joins threads (R8-2).

---

## Two-phase session lifetime

*Addresses R2 finding 2.*

### Problem

Rev 2 said `CaptureDestroy` is nonblocking, releases COM objects/thread, invalidates the handle on return, and is safe while an un-cancellable activation callback still owns state. Those guarantees conflict — the callback may still be running when `CaptureDestroy` returns, and the COM objects cannot be released from the wrong thread.

### Frozen lifetime contract

Two-phase: **`CaptureRequestStop(opId, reason)`** + **`CaptureRelease(opId)`**.

#### Phase 1: `CaptureRequestStop(opId, reason)`

- Non-blocking. Sets the stop reason, signals `stopEvent`. Returns `S_OK` immediately.
- Idempotent: calling it on an already-stopping or stopped session is a no-op (returns `S_OK`).
- The capture thread (or late callback) sees the stop signal and performs its cleanup:
  1. `IAudioClient::Stop()`
  2. `IAudioCaptureClient::Release()` — same thread
  3. `IAudioClient::Release()` — same thread
  4. `CoUninitialize()`
  5. Transition session to terminal state (`STOPPED`/`FAILED`/`CANCELLED`)
  6. `SetEvent(notifyEvent)` — Go wakes and queries terminal state
- If activation is still in flight when stop is requested: the `cancelled` flag is set. The late callback releases the returned `IAudioClient*` on its own MTA thread (legal — no services were obtained), transitions to `CANCELLED`, signals the event, and then releases its strong operation reference.
- Reason values: `user_stop`, `permission_revoke`, `device_lost`, `shutdown`, `suspend`, `lock`, `cancel`.

#### Phase 2: `CaptureRelease(opId)`

- **Only valid after terminal state.** Calling `CaptureRelease` on a non-terminal session returns `E_ILLEGAL_METHOD_CALL`.
- **Drops the registry reference** to the session. If all callback strong refs have been released (the normal case), the session's memory is freed immediately. If a callback is still in-flight (race case), the session remains alive until the callback releases its strong ref.
- Invalidates the operation ID in the registry.
- Non-blocking: all COM objects were already released in phase 1 by the capture thread.
- Idempotent: calling with an already-released or unknown ID returns `S_OK` (no-op).

#### Reference ownership during each phase

| Phase | Who owns IAudioClient | Who owns IAudioCaptureClient | Who owns session memory |
|---|---|---|---|
| Activation in flight | System (via async op) → callback MTA thread | Not yet created | Helper (registry ref + callback strong ref) |
| Capturing | Capture thread (exclusive) | Capture thread (exclusive) | Helper (registry ref) |
| Stop requested | Capture thread (releasing) | Capture thread (releasing) | Helper (registry ref) |
| Terminal state | Released | Released | Helper (registry ref; callback strong ref may still be live briefly) |
| After Release (no race) | N/A | N/A | Freed (ref-count zero) |
| After Release (callback race) | N/A | N/A | Alive (callback strong ref); freed when callback returns and releases |

#### Late callback completion

If `CaptureRequestStop` is called while `ActivateAudioInterfaceAsync` has not yet completed:

1. The `cancelled` flag is set under the mutex.
2. The callback fires on an MTA thread (guaranteed by the OS — there is no cancellation API).
3. The callback checks `cancelled` under the mutex, calls `GetActivateResult`, releases the returned `IAudioClient*` on its own MTA thread (legal since `Initialize`/`GetService` were never called, so no service release ordering applies), transitions to `CANCELLED`, and signals the notification event.
4. **The callback then releases its strong operation reference** — this is its final action before returning. If Go has already called `CaptureRelease` (dropping the registry ref), this release causes the ref-count to reach zero and the session's destructor fires on the callback's thread.
5. The callback returns to the OS.

If the callback has already completed and capture is running, the capture thread's `WaitForMultipleObjects({captureDataEvent, stopEvent})` loop sees `stopEvent` on its next iteration, performs the normal stop sequence, transitions to terminal state, and signals the notification event.

#### C++ exception safety

All C++ exceptions are caught at every exported function boundary via `try/catch(...)` and converted to `HRESULT` (`E_FAIL` for unknown exceptions, or the exception's own HRESULT if it carries one via `winrt::hresult_error`). No C++ exception crosses the ABI.

---

## Frozen sample representation

*Addresses R2 finding 3. Exact PCM conversion per R3 finding 5.*

### Problem

`GetMixFormat()` may return PCM integer or IEEE float via `WAVEFORMATEXTENSIBLE` (SubFormat `KSDATAFORMAT_SUBTYPE_PCM` or `KSDATAFORMAT_SUBTYPE_IEEE_FLOAT`). `[MS-43]` A `CaptureRead(float*)` export cannot simply copy arbitrary WASAPI bytes into a caller float buffer.

### Frozen contract

The helper converts supported native formats to **interleaved float32** at the **native sample rate and channel count**. Go writes probe evidence WAVs at the native format. The production recording task (future) handles channel mapping, rate conversion, and mono downmix.

#### Format detection: `WAVEFORMATEX` vs `WAVEFORMATEXTENSIBLE`

*Addresses R3 finding 5.*

`GetMixFormat` may return either a plain `WAVEFORMATEX` (`cbSize == 0`, `wFormatTag` identifies the format directly) or a `WAVEFORMATEXTENSIBLE` (`wFormatTag == WAVE_FORMAT_EXTENSIBLE`, `cbSize >= 22`, `SubFormat` GUID identifies the actual data type). `[MS-43]`

The helper's format-detection logic:

1. If `wFormatTag == WAVE_FORMAT_EXTENSIBLE` and `cbSize >= 22`:
   - Cast to `WAVEFORMATEXTENSIBLE`.
   - Use `SubFormat` to identify the data type.
   - Use `Samples.wValidBitsPerSample` for actual sample precision.
   - Use `dwChannelMask` for channel layout.
   - If `wValidBitsPerSample == 0`, treat it as equal to `wBitsPerSample` (some drivers omit it).
2. If `wFormatTag == WAVE_FORMAT_IEEE_FLOAT`:
   - Plain IEEE float. `wBitsPerSample` is the container size. `wValidBitsPerSample` is `wBitsPerSample`.
3. If `wFormatTag == WAVE_FORMAT_PCM`:
   - Plain PCM. `wBitsPerSample` is the container size. `wValidBitsPerSample` is `wBitsPerSample`.
4. Any other `wFormatTag`: fail with `E_INVALIDARG`.

#### Supported native subtypes and exact conversion

| Format source | SubFormat / wFormatTag | Container bits | Valid bits | `nBlockAlign` | Conversion |
|---|---|---|---|---|---|
| `WAVEFORMATEXTENSIBLE` or plain | `IEEE_FLOAT` | 32 | 32 | `channels * 4` | Direct `memcpy` (no conversion) |
| `WAVEFORMATEXTENSIBLE` or plain | `PCM` | 16 | 16 | `channels * 2` | Read `int16_t`, divide by `32768.0f` (i.e. `2^(validBits-1)`) |
| `WAVEFORMATEXTENSIBLE` | `PCM` | 24 (packed) | 24 | `channels * 3` | Read 3 bytes LE, assemble in `uint32_t`, sign-extend explicitly, divide by `8388608.0f` (`2^23`) |
| `WAVEFORMATEXTENSIBLE` | `PCM` | 32 | 24 | `channels * 4` | Read `uint32_t`, extract high 24 bits via unsigned right-shift by 8, sign-extend explicitly (same procedure as packed 24-bit), divide by `8388608.0f` (`2^(validBits-1)`) |
| `WAVEFORMATEXTENSIBLE` or plain | `PCM` | 32 | 32 | `channels * 4` | Read `uint32_t`, reinterpret as `int32_t`, divide by `2147483648.0f` (`2^31`) |
| Any other subtype, bit depth, or `wValidBitsPerSample > wBitsPerSample` | — | — | — | — | Session fails with `E_INVALIDARG` and a diagnostic string naming the unsupported format |

#### Left-alignment rule

*Addresses R4 finding 1.*

Microsoft documents that when `wValidBitsPerSample < wBitsPerSample`, the valid data bits are **left-aligned** (most-significant) within the container, and unused least-significant bits are set to zero. `[MS-44]` This means:

- For **24-in-32**: a positive full-scale 24-bit value occupies the high 24 bits of the 32-bit container. The byte pattern `0x7FFFFF00` represents +max (the low 8 bits are unused zeros). Extraction: read as `uint32_t`, unsigned right-shift by `(32 - 24) = 8`, sign-extend the resulting 24-bit value (same procedure as packed 24-bit).
- For **32-bit full (validBits == 32)**: all bits are valid, no alignment issue.
- The **scaling divisor** is always `2^(validBits-1)`, not `2^validBits` and not `2^(containerBits-1)`. This produces the range `[-1.0, +1.0)` matching IEEE float conventions.

#### Key conversion details

*24-in-32 extraction corrected per R5 finding 2: C++20 arithmetic-right-shift guarantee removed from C++17 build; extraction uses explicit unsigned + sign-extension, not implementation-defined signed shift.*

- **Packed 24-bit** (`nBlockAlign == channels * 3`): samples are stored as 3 consecutive bytes per sample, little-endian. The helper reads bytes at offsets `i*3`, `i*3+1`, `i*3+2` and assembles into an **unsigned** `uint32_t`: `uint32_t u = (uint32_t)buf[2] << 16 | (uint32_t)buf[1] << 8 | (uint32_t)buf[0]`. Sign extension uses **safe signed arithmetic on representable values**: `int32_t val = (int32_t)u; if (u >= 0x800000u) val -= 0x1000000;`. The first cast is safe because `u <= 0xFFFFFF` (always fits in `int32_t`). The subsequent subtraction is signed arithmetic: for `u` in `[0x800000, 0xFFFFFF]`, `(int32_t)u` is in `[8388608, 16777215]`, and subtracting `16777216` produces `[−8388608, −1]`, which is representable in `int32_t`. This avoids both the signed-left-shift UB and the implementation-defined out-of-range `uint32_t`-to-`int32_t` cast — the previous form `(int32_t)(u - 0x1000000u)` performed unsigned subtraction wrapping to `0xFF800000..0xFFFFFFFF` and then cast to `int32_t`, which is the same implementation-defined cast it claimed to eliminate. Result is divided by `8388608.0f` (`2^23`).
- **24-in-32 container** (`nBlockAlign == channels * 4`, `wValidBitsPerSample == 24`): valid bits are **left-aligned** in the 32-bit container (high 24 bits carry data, low 8 bits are zero). `[MS-44]` The helper reads the raw 4 bytes via `memcpy` into a `uint32_t` (not a pointer cast — see §Safe sample reads below), extracts the high 24 bits via **unsigned right-shift** by `(32 - 24) = 8`, then sign-extends using the same safe signed arithmetic: `uint32_t raw; memcpy(&raw, ptr, 4); uint32_t u = raw >> 8; int32_t val = (int32_t)u; if (u >= 0x800000u) val -= 0x1000000;`. Result is divided by `8388608.0f` (`2^23`). This approach uses no implementation-defined behavior; the build targets C++17.
- **`nBlockAlign` validation**: the helper verifies `nBlockAlign == channels * (wBitsPerSample / 8)` before proceeding. A mismatch means the format struct is inconsistent and the session fails.
- **Channel count and mask**: the helper passes `nChannels` and `dwChannelMask` (or 0 for plain `WAVEFORMATEX`) through to `CaptureFormat`. It does not remap channels — the production recording task handles layout conversion.
- **`wValidBitsPerSample` rules**: if `wValidBitsPerSample > wBitsPerSample`, the format is rejected. If `wValidBitsPerSample < wBitsPerSample` for float, the format is rejected (float must use all container bits). If `wValidBitsPerSample < wBitsPerSample` for PCM in a 32-bit container, only `wValidBitsPerSample == 24` is accepted (the 24-in-32 path); other combinations (e.g., 20-in-32) are rejected with `E_INVALIDARG`.

#### Safe sample reads — no pointer casts

*Addresses R6 finding 7.*

All sample reads in the helper use `memcpy` (for 16-bit and 32-bit containers) or byte-by-byte assembly (for packed 24-bit) instead of pointer casts like `*(uint32_t*)ptr`. This eliminates:

1. **Strict-aliasing undefined behavior**: casting a `BYTE*` to `uint32_t*` and dereferencing violates C/C++ strict aliasing rules. `memcpy` is the standard-blessed mechanism for type-punning.
2. **Potential unaligned-access undefined behavior**: WASAPI `GetBuffer` returns `BYTE*` and the documentation does not guarantee alignment beyond `nBlockAlign`. While x64 hardware tolerates unaligned access, the C++ standard does not, and UBSan will flag it.

Concrete read procedures:

```
// PCM int16 (2 bytes):
int16_t s; memcpy(&s, ptr, 2);
float out = (float)s / 32768.0f;

// PCM int32 or 24-in-32 (4 bytes):
uint32_t raw; memcpy(&raw, ptr, 4);
// then extract/shift/sign-extend as described above

// IEEE float32 (4 bytes):
float f; memcpy(&f, ptr, 4);

// Packed 24-bit (3 bytes) — byte-by-byte:
uint32_t u = (uint32_t)ptr[2] << 16 | (uint32_t)ptr[1] << 8 | (uint32_t)ptr[0];
```

**Signed conversion for PCM16 and PCM32**: `int16_t` via `memcpy` is safe because x64 Windows is always little-endian and `int16_t` is two's complement on all target platforms. For `int32_t` reads (PCM32 full-scale), `memcpy` into `int32_t` directly: `int32_t s; memcpy(&s, ptr, 4); float out = (float)s / 2147483648.0f;`. This is safe because C++17 x64 compilers universally use two's complement for `int32_t`, even though the standard only requires it from C++20. For the packed-24 and 24-in-32 paths, the unsigned assembly followed by safe signed arithmetic (`int32_t val = (int32_t)u; if (u >= 0x800000u) val -= 0x1000000;`) avoids any reliance on implementation-defined unsigned-to-signed casts — the initial cast is always in range (`u <= 0xFFFFFF`), and the subtraction is signed arithmetic on representable values.

**Deliberately unaligned test vectors** (R6-7): the probe must include conversion tests where the input buffer is offset by 1 byte from natural alignment (e.g., `alignas(4) uint8_t buf[8]; convert(&buf[1], ...)` for int16, `convert(&buf[1], ...)` for int32/float). These must run under AddressSanitizer (`-fsanitize=address`) and UBSan (`-fsanitize=undefined`) where available. Power-of-two conversion vectors (silence, full-scale, half) use **bit-exact** float32 comparison (`memcmp` of the float32 bit pattern or `==`); non-power-of-two vectors use ±0.5 ULP tolerance at the expected magnitude.

#### Deterministic conversion test vectors

*Corrected per R4 finding 1 — left-aligned 24-in-32 vectors, boundary coverage. Int16 vectors and float32 expectations corrected per R5 finding 2.*

The probe must exercise at least these conversions with known input/output pairs. Vectors include boundary values: minimum (negative full-scale), maximum (positive full-scale), ±1 LSB, and silence.

Tests are **bit-exact** for conversions that produce exactly representable float32 values (silence, full-scale, half). Tests that produce non-exactly-representable values (e.g., int32 full-scale) use a tolerance of ±0.5 ULP at the expected magnitude, with the expected value stated as the nearest float32.

**1. IEEE float32** — pass-through:

| Input | Output | Meaning |
|---|---|---|
| `0x00000000` (0.0f) | `0.0f` | Silence |
| `0x3F800000` (1.0f) | `1.0f` | Positive full-scale |
| `0xBF800000` (-1.0f) | `-1.0f` | Negative full-scale |
| `0x3F000000` (0.5f) | `0.5f` | Mid-positive |

**2. PCM int16** (validBits=16, divisor=32768.0f):

| Input bytes (LE, spaced) | int16 value | Output float32 | Meaning |
|---|---|---|---|
| `00 00` | 0 | `0.0f` (exact) | Silence |
| `FF 7F` | 32767 | `0.999969482421875f` (exact: 32767/32768) | +max |
| `00 80` | −32768 | `−1.0f` (exact) | −max (negative full-scale) |
| `01 00` | 1 | `3.0517578125e−5f` (exact: 1/32768) | +1 LSB |
| `FF FF` | −1 | `−3.0517578125e−5f` (exact: −1/32768) | −1 LSB |
| `00 01` | 256 | `0.0078125f` (exact: 256/32768) | +256 |
| `00 FF` | −256 | `−0.0078125f` (exact: −256/32768) | −256 |
| `00 40` | 16384 | `0.5f` (exact) | Half |

**3. PCM packed int24** (validBits=24, divisor=8388608.0f):

| Input bytes [0,1,2] (LE, spaced) | Assembled uint32 | Sign-extended int32 | Output float32 | Meaning |
|---|---|---|---|---|
| `00 00 00` | `0x000000` | 0 | `0.0f` (exact) | Silence |
| `FF FF 7F` | `0x7FFFFF` | 8388607 | `≈0.999999881f` (nearest float32) | +max |
| `00 00 80` | `0x800000` | −8388608 | `−1.0f` (exact) | −max |
| `01 00 00` | `0x000001` | 1 | `≈1.1920929e−7f` (nearest float32) | +1 LSB |
| `FF FF FF` | `0xFFFFFF` | −1 | `≈−1.1920929e−7f` (nearest float32) | −1 LSB |
| `00 00 40` | `0x400000` | 4194304 | `0.5f` (exact) | Half |

**4. PCM 24-in-32** (containerBits=32, validBits=24, **left-aligned**, divisor=8388608.0f):

Extraction: read `uint32_t`, unsigned right-shift by 8, sign-extend.

| Input uint32 (hex) | After >>8 (unsigned) | Sign-extended int32 | Output float32 | Meaning |
|---|---|---|---|---|
| `0x00000000` | `0x000000` | 0 | `0.0f` (exact) | Silence |
| `0x7FFFFF00` | `0x7FFFFF` | 8388607 | `≈0.999999881f` (nearest float32) | +max (high 24 bits = `0x7FFFFF`, low 8 bits zero) |
| `0x80000000` | `0x800000` | −8388608 | `−1.0f` (exact) | −max (high 24 bits = `0x800000`) |
| `0x00000100` | `0x000001` | 1 | `≈1.1920929e−7f` (nearest float32) | +1 LSB |
| `0xFFFFFF00` | `0xFFFFFF` | −1 | `≈−1.1920929e−7f` (nearest float32) | −1 LSB |
| `0x40000000` | `0x400000` | 4194304 | `0.5f` (exact) | Half |

Note: the unused low 8 bits must be zero per the spec; the helper does not mask them before shifting (the unsigned right shift discards them).

**5. PCM int32** (validBits=32, divisor=2147483648.0f):

| Input int32 | Output float32 | Meaning |
|---|---|---|
| `0` | `0.0f` (exact) | Silence |
| `2147483647` (`INT32_MAX`) | `1.0f` (float32 rounds `2147483647.0f / 2147483648.0f` to `1.0f` — the mathematical result `≈0.9999999995` is not representable in float32; the nearest float32 is `1.0f`) | +max |
| `−2147483648` (`INT32_MIN`) | `−1.0f` (exact) | −max |
| `1` | `≈4.6566129e−10f` (nearest float32) | +1 LSB |
| `−1` | `≈−4.6566129e−10f` (nearest float32) | −1 LSB |
| `1073741824` | `0.5f` (exact) | Half |

#### Versioned format struct

Reported to Go after activation completes, via `CaptureGetResult`:

```c
typedef struct {
    uint32_t structSize;    // sizeof(CaptureFormat), for versioning
    uint32_t version;       // 1
    uint32_t valid;         // 1 if format is populated, 0 if activation failed
    uint32_t sampleRate;    // e.g. 48000 (0 if valid==0)
    uint32_t channels;      // e.g. 2 (0 if valid==0)
    uint32_t bitsPerSample; // always 32 (output is float32) (0 if valid==0)
    uint32_t validBits;     // wValidBitsPerSample from the device
    uint32_t channelMask;   // dwChannelMask from WAVEFORMATEXTENSIBLE (0 for plain WAVEFORMATEX)
    uint32_t nativeSubtype; // 0=unknown, 1=PCM, 3=IEEE_FLOAT (original before conversion)
    uint32_t nativeBits;    // original wBitsPerSample from the device
    uint32_t nativeValidBits; // original wValidBitsPerSample (may differ from nativeBits)
    uint32_t nBlockAlign;   // original nBlockAlign from the device
} CaptureFormat;
```

*Addresses R3 finding 7 (partial): `valid` flag.*

`bitsPerSample` is always 32 in the output because the helper converts everything to float32. `nativeSubtype`, `nativeBits`, `nativeValidBits`, and `nBlockAlign` record exactly what the device provided, for evidence logging and probe diagnostics. The `valid` flag is 0 when the session is in `activating` or `failed` state and the format has not been populated — `CaptureGetResult` never returns uninitialized format fields as if they were real data.

#### Buffer handling — recording ring

- `CaptureRead` copies converted float32 frames from the helper's internal **recording ring** into the caller's buffer.
- The helper calls `IAudioCaptureClient::GetBuffer`, converts the packet to float32, appends to the recording ring, calls `ReleaseBuffer`, and signals Go's notification event.
- **Silent-buffer handling**: WASAPI may report `AUDCLNT_BUFFERFLAGS_SILENT`; the helper writes zeros for that packet.

#### Recording ring overflow is terminal failure

*Addresses R3 finding 4. HRESULT corrected per R5 finding 3.*

Dropping oldest unread samples from the recording ring and then finalizing a draft as successful produces corrupted, discontinuous audio. This is unacceptable for the recording use case.

**Frozen policy**: if Go does not call `CaptureRead` fast enough and the recording ring is full, the capture session transitions to **terminal `FAILED`** state with a dedicated overflow reason.

*Corrected per R5 finding 3: the prior `FACILITY_ITF` code `0x80040200` collides with `VFW_E_INVALIDMEDIATYPE` from DirectShow. `FACILITY_ITF` codes are shared across COM interfaces and cannot claim global uniqueness.*

The overflow HRESULT is the standard `HRESULT_FROM_WIN32(ERROR_BUFFER_OVERFLOW)` = `0x8007006F`. This is a well-known Windows error code with defined semantics (buffer too small / overflow). The `CAP_REASON_OVERFLOW` terminal-reason enum in `CaptureGetResult` disambiguates ring overflow from other WASAPI or system failures:

| Terminal reason | Value | Meaning |
|---|---|---|
| `CAP_REASON_USER_STOP` | 0 | Normal user/app stop |
| `CAP_REASON_PERMISSION_REVOKE` | 1 | Microphone permission revoked |
| `CAP_REASON_DEVICE_LOST` | 2 | Device invalidated / removed |
| `CAP_REASON_SHUTDOWN` | 3 | System shutdown / logoff |
| `CAP_REASON_SUSPEND` | 4 | System suspend |
| `CAP_REASON_LOCK` | 5 | Session lock |
| `CAP_REASON_CANCEL` | 6 | Cancelled before activation |
| `CAP_REASON_OVERFLOW` | 7 | Recording ring overflow |
| `CAP_REASON_WASAPI_ERROR` | 8 | WASAPI call failed (HRESULT in result) |
| `CAP_REASON_FORMAT_ERROR` | 9 | Unsupported capture format |
| `CAP_REASON_DISCONTINUITY` | 10 | Non-first-packet AUDCLNT_BUFFERFLAGS_DATA_DISCONTINUITY — data integrity compromised |

The helper's overflow sequence (corrected per R4-3 for acquired-packet safety):

1. **Release the currently acquired WASAPI packet** via `ReleaseBuffer(frames)` — a buffer must never remain acquired.
2. Call `IAudioClient::Stop()`.
3. Call `IAudioCaptureClient::Release()` — same capture thread.
4. Call `IAudioClient::Release()` — same capture thread.
5. Call `CoUninitialize()`.
6. Set the session to `FAILED` with HRESULT `0x8007006F` and reason `CAP_REASON_OVERFLOW`.
7. Signal the notification event.
8. Go sees the failure, deletes/marks the evidence artifact as invalid, and reports the error. Go never promotes a partial from a session that terminated with overflow.

No recording draft is finalized from data that passed through a full ring with dropped samples.

#### Checked allocation bounds

*Addresses R4 finding 6: `nChannels` is `WORD`/`uint16` (max 65535), not 255.*

| Parameter | Type | Supported maximum | Source |
|---|---|---|---|
| Channel count (`nChannels`) | `uint16` | 8 | Channels > 8 are rejected with `E_INVALIDARG` at format validation (before `Initialize`). While `WAVEFORMATEX.nChannels` is a 16-bit field allowing up to 65535, no real microphone produces >8 channels, and the ring/draft arithmetic below assumes ≤8. |
| Sample rate (`nSamplesPerSec`) | `uint32` | 384000 | Rates > 384 kHz are rejected. Real capture devices are ≤192 kHz. |
| Block alignment (`nBlockAlign`) | `uint16` | `channels * 4` = 32 | Validated as `channels * (bitsPerSample / 8)`. |
| WASAPI buffer frames | `uint32` | 65536 | `GetBufferSize()` return; if larger, capture fails with a diagnostic. |
| Recording ring frames | computed | `max(2 * sampleRate, bufferFrames)` (R8-6) | At max 384 kHz × 8 ch × 4 bytes = 24,576,000 bytes ≈ 23 MiB. Dynamic sizing ensures ring ≥ one full WASAPI buffer. Validated before allocation. |
| Ring bytes | `size_t` | ≤ 24,576,000 (typical) | `ringFrames * channels * sizeof(float32)`. Computed in `uint64_t` before C `malloc`; overflow → `E_OUTOFMEMORY`. |
| Caller `maxFrames` (Go `CaptureRead`) | `uint32` | 65536 | Larger values are clamped. `maxFrames * channels * 4` is validated in `int64` before copy. |

All multiplication and addition for ring allocation, WASAPI buffer sizing, PCM conversion buffer allocation, and Go-side `CaptureRead` buffer sizing are performed in a **wide type** (`uint64_t` in C++, `int64` in Go) and checked for overflow before narrowing to the allocation type. On overflow: the helper returns `E_OUTOFMEMORY` and does not proceed with allocation. Go logs the overflow and reports a capture failure.

**Recording ring capacity** (R8-6): `ringFrames * channels * sizeof(float32)`, with `ringFrames = max(2 * sampleRate, bufferFrames)`. The `bufferFrames` value is read from `IAudioClient::GetBufferSize()` after `Initialize`. If `bufferFrames > 2 * sampleRate` (possible with low sample rates or large negotiated buffers), the ring grows to fit at least one full WASAPI buffer. At the validated maximum (384 kHz, 8 channels, `2 * sampleRate` = 768000 frames): `768000 * 8 * 4 = 24,576,000` bytes ≈ 23 MiB. At a hypothetical low rate (8 kHz, 1 channel, `bufferFrames` = 65536 > `2 * 8000` = 16000): ring uses 65536 frames × 1 channel × 4 = 262,144 bytes. If the ring allocation exceeds the maximum allowed (§Checked allocation bounds), the capture session fails with `E_OUTOFMEMORY` before `IAudioClient::Start`.

**Maximum capture packet size**: WASAPI's `GetBuffer` returns at most `GetBufferSize()` frames per packet (the full WASAPI buffer). The ring is always ≥ `bufferFrames` (R8-6), so a single WASAPI packet never exceeds ring capacity. Overflow during ordinary operation remains terminal — it indicates a genuine consumer stall. A low-rate/large-buffer fixture test verifies this dynamic sizing (R8-6).

#### Separate lossy meter ring (not for recording)

A second, independent ring may be used for UI-level VU metering. This ring **may** drop oldest samples on overflow (lossy) because meter display tolerates discontinuity. It must never be the source for `CaptureRead` or draft writing. The meter ring is not part of the ABI — it is internal to Go.

---

## Picker returns a read handle, not a path

*Addresses R2 finding 4.*

### Problem

A brokered/provider-backed `StorageFile` is not safely represented by `StorageFile.Path` alone. The path may be virtual (cloud-only, provider-backed), `null` (no accessible filesystem path), or point to a location the appContainer process cannot read directly.

### Frozen contract

The helper uses `IStorageItemHandleAccess::Create` `[MS-41]` to obtain a kernel `HANDLE` with read access from the picked `StorageFile`. This handle is returned to Go via the async result query.

**`IStorageItemHandleAccess::Create` under this exact signed AppContainer is a mandatory probe hypothesis**, not a fully proven fact. The documentation `[MS-41]` does not explicitly confirm behavior under `packagedClassicApp` + `appContainer`. The probe must test this with a signed MSIX on real hardware and capture the HRESULT. If it fails, the picker scenario is blocked and requires an alternative (e.g., WinRT stream reads inside the helper).

```c
// Initiate a file-open picker owned by hwnd.
// hwnd MUST be a visible, foreground top-level window.
// Returns S_OK and writes the operation ID to *opId.
// When complete, Go calls PickerGetResult to retrieve the file handle.
HRESULT __stdcall PickerOpenFile(HWND hwnd,
                                 const wchar_t *filterDesc,
                                 const wchar_t *filterPattern,
                                 HANDLE notifyEvent,
                                 uint32_t *opId);

// Query the picker result. Two-step size-discovery/take API (R4-4).
//
// state: 0=pending, 1=picked, 2=cancelled, 3=failed.
//
// If state=PENDING: returns S_FALSE with *state=0. All other outputs
// are not written. This is not an error — the operation is in progress.
//
// takeHandle: 0 = size-discovery call (does NOT transfer the handle),
//             1 = take call (transfers exactly once).
//             Any other value: returns E_INVALIDARG.
//
// On state=picked, takeHandle=0 (size discovery):
//   *fileHandle is set to INVALID_HANDLE_VALUE (not transferred).
//   *fileSize receives the file size, or -1 if unknown.
//   *requiredNameChars receives the required buffer size in wchar_t
//     (including null terminator) for the full display name.
//   nameBuf receives the name truncated to nameBufLen-1 + null if too small,
//     or the full name if nameBufLen >= *requiredNameChars.
//     (Note: picker name truncation returns S_OK, not E_NOT_SUFFICIENT_BUFFER.
//      The general E_NOT_SUFFICIENT_BUFFER rule applies only to device/ID
//      string exports. Picker provides requiredNameChars for Go to allocate
//      a correctly-sized buffer for the take call.)
//   *handleTaken is set to 0.
//   The function returns S_OK. May be called repeatedly.
//
// On state=picked, takeHandle=1 (take — exactly once):
//   FIRST CALL with takeHandle=1:
//     *fileHandle is a valid read-only kernel HANDLE. Go owns it, must
//       CloseHandle. *handleTaken is set to 1.
//     *fileSize, nameBuf, *requiredNameChars populated as above.
//     The function returns S_OK.
//   SUBSEQUENT CALLS with takeHandle=1:
//     *fileHandle is INVALID_HANDLE_VALUE. *handleTaken is set to 0.
//     *hresult remains the operation outcome (S_OK — the pick succeeded;
//       it is NEVER overwritten with transfer-state codes).
//     The function returns S_OK.
//     *handleTaken == 0 alone indicates the handle was already taken.
//     All other outputs (*state, *fileSize, nameBuf, *requiredNameChars)
//       are still populated.
//
// On state=cancelled/failed:
//   *fileHandle is INVALID_HANDLE_VALUE. *handleTaken is set to 0.
//   *hresult receives the error HRESULT on failure (S_OK on cancel).
//
// On null/zero nameBuf or nameBufLen<=0: name is not written; no error.
// On null fileHandle with takeHandle=1: E_POINTER (handle not transferred).
// On null requiredNameChars: not written; no error.
//
HRESULT __stdcall PickerGetResult(uint32_t opId,
                                  int32_t takeHandle,
                                  int32_t *state,
                                  HANDLE *fileHandle,
                                  int32_t *handleTaken,
                                  int64_t *fileSize,
                                  wchar_t *nameBuf, int32_t nameBufLen,
                                  int32_t *requiredNameChars,
                                  HRESULT *hresult);

// Release picker operation resources. Only valid after non-PENDING state.
// If the file handle has NOT been taken (no successful takeHandle=1 call),
// PickerRelease closes the helper-owned handle before freeing resources.
// If the handle WAS taken, PickerRelease has no handle to close (Go owns it).
// Drops the registry reference; if the picker callback's strong ref is still
// live (race), state remains alive until the callback releases.
HRESULT __stdcall PickerRelease(uint32_t opId);
```

#### Two-step take protocol

*Addresses R4 finding 4. Replaces the previous take-once design that conflated
size discovery with handle transfer and leaked handles on insufficient buffers.*

The `takeHandle` parameter separates size discovery from transfer:

| Step | `takeHandle` | Transfers handle? | Purpose |
|---|---|---|---|
| 1. Size discovery | `0` | No | Get `requiredNameChars` and `fileSize`; allocate Go buffers |
| 2. Take | `1` | Yes (first call only) | Transfer the handle to Go; subsequent `takeHandle=1` returns `*handleTaken=0` (R6-3) |
| Invalid | any other value | No | Returns `E_INVALIDARG` immediately |

**State table for the helper-owned handle:**

| Condition | Helper handle state | `PickerRelease` behavior |
|---|---|---|
| `state=picked`, no `takeHandle=1` call yet | Valid (helper-owned) | Closes the handle, then drops registry ref |
| `state=picked`, one `takeHandle=1` call succeeded | Transferred (Go-owned) | No handle to close; drops registry ref |
| `state=picked`, repeated `takeHandle=1` | Already transferred | Returns `S_OK` with `*handleTaken=0`; `*hresult` unchanged (operation outcome — R6-3); no transfer |
| `state=cancelled/failed` | No handle exists | Drops registry ref |
| `PickerRelease` before terminal state | N/A | Returns `E_ILLEGAL_METHOD_CALL` |

**Edge cases:**

- **Null `fileHandle` with `takeHandle=1`**: returns `E_POINTER`, does **not** transfer the handle. Go must fix its call and retry with a valid pointer.
- **Null/zero `nameBuf`**: name is simply not written. No error; the handle is still transferable.
- **Null `requiredNameChars`**: not written. No error.
- **`PickerRelease` with untaken handle**: the helper closes its owned handle before dropping the registry ref. No leak.
- **`PickerRelease` before any `PickerGetResult`**: valid after terminal state; closes the helper-owned handle.
- **`PickerGetResult` in `PENDING` state**: returns `S_FALSE` with `*state=0`. No outputs are written. This is not an error — it indicates the picker is still open.

#### Complete `PickerGetResult` truth table

*Addresses R6 finding 3. Extended with full null/negative pointer coverage per R7 finding 6.*

**Pointer parameter classification** (R7-6):

| Parameter | Classification | Notes |
|---|---|---|
| `opId` | Mandatory (value, not pointer) | Invalid/released → `E_HANDLE` |
| `takeHandle` | Mandatory (value) | Must be 0 or 1 → else `E_INVALIDARG` |
| `state` | **Mandatory** | Null → `E_POINTER`; must be writable |
| `fileHandle` | **Mandatory when `takeHandle=1`**, optional when `takeHandle=0` | Null with `takeHandle=1` → `E_POINTER` (no transfer); null with `takeHandle=0` → no error (output skipped) |
| `handleTaken` | **Mandatory** | Null → `E_POINTER` |
| `fileSize` | Optional | Null → not written; no error |
| `nameBuf` | Optional (paired with `nameBufLen`) | Null or `nameBufLen<=0` → name not written; no error |
| `nameBufLen` | Value; validated only when `nameBuf` is non-null | `nameBufLen<=0` with non-null `nameBuf` → name not written; no error (treated as zero capacity) |
| `requiredNameChars` | Optional | Null → not written; no error |
| `hresult` | **Mandatory** | Null → `E_POINTER` |

**Validation order**: (1) `opId` lookup, (2) `takeHandle` range, (3) mandatory pointer null checks (`state`, `handleTaken`, `hresult`, and `fileHandle` when `takeHandle=1`), (4) operation state check. A validation failure at any step returns the error immediately — no outputs are written, no handle is transferred or closed.

| Condition | Function return | `*state` | `*hresult` | `*handleTaken` | `*fileHandle` | Handle owner | Other outputs |
|---|---|---|---|---|---|---|---|
| PENDING | `S_FALSE` | 0 (pending) | not written | not written | not written | Helper | Not written |
| Picked, `takeHandle=0` | `S_OK` | 1 (picked) | `S_OK` | 0 | `INVALID_HANDLE_VALUE` (if non-null) | Helper | `*fileSize`, `nameBuf`, `*requiredNameChars` populated (if non-null) |
| Picked, `takeHandle=1`, first call | `S_OK` | 1 (picked) | `S_OK` | 1 | valid `HANDLE` | Go (transferred) | `*fileSize`, `nameBuf`, `*requiredNameChars` populated (if non-null) |
| Picked, `takeHandle=1`, subsequent | `S_OK` | 1 (picked) | `S_OK` | 0 | `INVALID_HANDLE_VALUE` | Go (already transferred) | `*fileSize`, `nameBuf`, `*requiredNameChars` populated (if non-null) |
| Cancelled | `S_OK` | 2 (cancelled) | `S_OK` | 0 | `INVALID_HANDLE_VALUE` (if non-null) | N/A | — |
| Failed | `S_OK` | 3 (failed) | error HRESULT | 0 | `INVALID_HANDLE_VALUE` (if non-null) | N/A | — |
| Invalid `takeHandle` (not 0 or 1) | `E_INVALIDARG` | not written | not written | not written | not written | unchanged | — |
| Null `state` | `E_POINTER` | — | not written | not written | not written | unchanged | — |
| Null `hresult` | `E_POINTER` | not written | — | not written | not written | unchanged | — |
| Null `handleTaken` | `E_POINTER` | not written | not written | — | not written | unchanged | — |
| Null `fileHandle` with `takeHandle=1` | `E_POINTER` | not written | not written | not written | — | Helper (not transferred) | — |
| Null `fileHandle` with `takeHandle=0` | `S_OK` | written | written | written | (skipped) | unchanged | Other non-null outputs written |
| Null `fileSize` | `S_OK` | written | written | written | written per above | per above | `*fileSize` skipped; other outputs written |
| Null `requiredNameChars` | `S_OK` | written | written | written | written per above | per above | `*requiredNameChars` skipped |
| Null `nameBuf` or `nameBufLen<=0` | `S_OK` | written | written | written | written per above | per above | Name not written; no error |
| Non-null `nameBuf` with `nameBufLen<=0` | `S_OK` | written | written | written | written per above | per above | Name not written (zero capacity); no error |
| Negative `nameBufLen` (e.g. -1) | `S_OK` | written | written | written | written per above | per above | Name not written (treated as zero capacity); no error |
| Unknown/released `opId` | `E_HANDLE` | not written | not written | not written | not written | N/A | — |
| `PickerRelease` before terminal | `E_ILLEGAL_METHOD_CALL` | — | — | — | — | unchanged | — |
| `PickerRelease`, handle not taken | `S_OK` | — | — | — | — | Helper closes handle | — |
| `PickerRelease`, handle taken | `S_OK` | — | — | — | — | Go owns (no close) | — |

**`PickerOpenFile` pointer validation** (R7-6):

| Parameter | Classification | Null behavior |
|---|---|---|
| `hwnd` | Mandatory (value) | `NULL` → `E_INVALIDARG` |
| `filterDesc` | Mandatory | Null → `E_POINTER` |
| `filterPattern` | Mandatory | Null → `E_POINTER` |
| `notifyEvent` | Mandatory (handle) | `NULL`/`INVALID_HANDLE_VALUE` → `E_HANDLE` |
| `opId` | Mandatory (output pointer) | Null → `E_POINTER`; no operation created |

Validation order: (1) `opId` null check, (2) `hwnd` check, (3) `notifyEvent` check, (4) string pointer checks, (5) operation creation. A validation failure at any step returns the error immediately — no operation is created, no `opId` is written.

Key invariant: `*hresult` always reflects the **picker operation outcome** (was the pick successful, cancelled, or failed?) and is never overwritten with transfer-state or call-level codes. `*handleTaken` alone reports whether the handle was transferred in this call. A caller validation error (null mandatory pointer, invalid `takeHandle`, unknown `opId`) **never** transfers or closes the picked handle — the handle ownership is unchanged.

**Required table-driven ABI tests** (R7-6): for every row in both truth tables (including all null/negative combinations), create a test case that calls the function with the specified inputs and verifies: function HRESULT, every output value, handle ownership, and (for error cases) that no transfer or close occurred. Include repeat calls: `takeHandle=1` twice, `PickerRelease` twice, `PickerGetResult` after `PickerRelease`.

#### `IStorageItemHandleAccess` usage

After `FileOpenPicker.PickSingleFileAsync` completes with a `StorageFile`:

1. QI the `StorageFile` for `IStorageItemHandleAccess`. `[MS-41]`
2. Call `Create(HANDLE_ACCESS_OPTIONS_READ, HANDLE_SHARING_OPTIONS_SHARE_READ, HANDLE_OPTIONS_NONE, nullptr, &handle)`.
3. If successful: call `GetFileSizeEx(handle, &size)` to get the file size.
   - If `GetFileSizeEx` fails or returns an implausible value: set `fileSize = -1` (unknown).
   - If `GetFileSizeEx` returns 0: set `fileSize = 0` (this may be a real zero-byte file or a virtual file — Go distinguishes by attempting to read).
4. Store the handle, size, and display name in the operation's result slot. The handle is transferred to Go on the first `PickerGetResult` call with `takeHandle=1`.
5. If QI or Create fails: close the handle if one was obtained, report `FAILED` with the HRESULT. This covers cloud-hydration failures, provider errors, and unexpected `StorageFile` implementations.

#### Edge cases

- **Cancel**: user closes the picker without selecting → state = `cancelled`, no handle.
- **Zero-byte file vs unknown size**: `fileSize == 0` means `GetFileSizeEx` returned zero — this is either a real empty file or a placeholder. `fileSize == -1` means the size is genuinely unknown (provider-backed, no filesystem representation). Go must attempt to read in both cases and handle the outcome:
  - Real zero-byte: `ReadFile` returns 0 bytes immediately. Go rejects the file (minimum-size policy).
  - Unknown size: Go reads in a loop up to the maximum allowed size, counting actual bytes. If bytes exceed the maximum, Go stops reading, closes the handle, and rejects the file.
- **Maximum file size enforcement during reading**: Go does not trust `fileSize` alone (it is racy for network-backed files and optional for provider files). Go enforces the maximum against **actual bytes read** in its `ReadFile` loop. If cumulative bytes exceed the limit mid-read, Go stops, closes the handle, discards partial data, and reports an error.
- **Read error after handle creation**: Go's `ReadFile` may fail if the file is provider-backed and the provider fails mid-stream. Go handles this as a normal I/O error, closes the handle, and discards partial data.
- **Cloud hydration / provider failure**: if the file needs cloud hydration and the provider fails, `IStorageItemHandleAccess::Create` returns an error HRESULT. The picker result reports `FAILED`.
- **Close-on-error**: if Go encounters any error after receiving the handle (size exceeded, read error, format error), Go calls `CloseHandle` immediately. No leaked handles.

---

## Helper ABI, build, and loading contract

*Addresses R1 finding 2. Async redesign per R2 finding 1. Versioning per R2 finding 7. HRESULT handling per R2 finding 6. Loading path per R2 finding 5.*

### ABI version

```c
// Returns S_OK. *version receives the ABI version (currently 1).
// *structHeaderSize receives the minimum struct size for version negotiation.
HRESULT __stdcall CapGetVersion(uint32_t *version, uint32_t *structHeaderSize);
```

Go calls `CapGetVersion` immediately after loading the DLL. If the version is not recognized or the struct sizes are incompatible, Go refuses to use the helper and logs the mismatch.

### Exported ABI

All exports use `__stdcall` calling convention (x64: `__stdcall` is accepted but has no distinct calling convention; `__cdecl` and `__stdcall` are identical on x64 — the convention is documented for clarity and consistency with `syscall.NewProc`). All types are fixed-width. All functions return `HRESULT`. Every struct has a `structSize` field for forward compatibility.

#### Permission

```c
// Check microphone permission status (non-blocking).
// Returns S_OK and writes status as a named CAP_PERMISSION_* value.
//
// CAP_PERMISSION_* enum (ABI values — NOT raw AppCapabilityAccessStatus):
//   0 = CAP_PERMISSION_DENIED_BY_USER
//   1 = CAP_PERMISSION_ALLOWED
//   2 = CAP_PERMISSION_PROMPT_REQUIRED
//   3 = CAP_PERMISSION_DENIED_BY_SYSTEM
//   4 = CAP_PERMISSION_NOT_DECLARED (microphone capability missing from manifest)
//   5 = CAP_PERMISSION_UNKNOWN (future/unrecognized WinRT value — fail-closed)
//  -1 = CAP_PERMISSION_UNAVAILABLE (AppCapability.Create failed — SUA-only)
//
// The helper contains an exhaustive switch from raw AppCapabilityAccessStatus
// (DeniedBySystem=0, NotDeclaredByApp=1, DeniedByUser=2, UserPromptRequired=3,
// Allowed=4) to these ABI values. A direct cast of the raw integer NEVER
// reaches Go — the switch prevents the security-critical misinterpretation
// where raw NotDeclaredByApp(1) would be read as Allowed(1) (R8-3).
//
// Unknown/future raw values map to CAP_PERMISSION_UNKNOWN(5), which is
// non-promotable (fail-closed).
//
// If AppCapability.Create fails (SUA-only), returns S_OK with status=-1.
HRESULT __stdcall CapPermissionCheck(int32_t *status);

// Request microphone permission (async, UI thread).
// Returns S_OK and writes opId. notifyEvent signaled on completion.
// Go calls CapPermissionRequestResult(opId, *status) to get the outcome.
HRESULT __stdcall CapPermissionRequest(HANDLE notifyEvent, uint32_t *opId);

// Query the result of CapPermissionRequest.
// state: 0=pending, 1=completed, 2=failed.
// On state=completed: *status is the resulting access status.
HRESULT __stdcall CapPermissionRequestResult(uint32_t opId,
                                             int32_t *state,
                                             int32_t *status,
                                             HRESULT *hresult);

// Release permission-request operation resources.
// Only valid after non-PENDING state.
HRESULT __stdcall CapPermissionRequestRelease(uint32_t opId);

// Subscribe to permission-change events. The helper duplicates notifyEvent
// via DuplicateHandle and signals only the duplicate — Go's original handle
// is never touched by the handler (R6-2). Returns S_OK. Must be explicitly
// unsubscribed via CapPermissionUnsubscribe before CapDestroy — CapDestroy
// does NOT auto-unsubscribe. Each AccessChanged invocation holds a strong
// subscription ref (see §AccessChanged unsubscribe fence); the subscription
// state destructor closes the duplicated handle after all in-flight handlers
// have returned.
HRESULT __stdcall CapPermissionSubscribe(HANDLE notifyEvent);

// Unsubscribe from permission-change events. Revokes the WinRT event token
// (prevents new dispatches), drops the registry reference, and returns
// immediately. If a handler is in-flight, its strong ref keeps the
// subscription state (and duplicated handle) alive until it returns. Go can
// safely close or reuse its original notifyEvent immediately after this call.
// Idempotent: calling when not subscribed returns S_OK.
HRESULT __stdcall CapPermissionUnsubscribe(void);
```

#### Device enumeration

```c
// Enumerate capture devices (async).
// Returns S_OK and writes opId. notifyEvent signaled on completion.
HRESULT __stdcall CapEnumerateDevices(HANDLE notifyEvent, uint32_t *opId);

// Query enumeration result. count receives the device count.
// Caller then calls CapGetDeviceInfo(opId, index, ...) for each.
HRESULT __stdcall CapEnumerateDevicesResult(uint32_t opId,
                                            int32_t *state,
                                            int32_t *count,
                                            HRESULT *hresult);

// Get info for device at index from a completed enumeration.
// Writes UTF-16 id and name into caller buffers.
// Maximum device count: 256. Maximum string length: 512 wchar_t.
// Returns E_NOT_SUFFICIENT_BUFFER if id or name buffer is too small.
HRESULT __stdcall CapGetDeviceInfo(uint32_t opId, int32_t index,
                                   wchar_t *idBuf, int32_t idBufLen,
                                   wchar_t *nameBuf, int32_t nameBufLen);

// Release enumeration resources.
HRESULT __stdcall CapEnumerateDevicesRelease(uint32_t opId);

// Get the default capture device ID for the given role (0=Default, 1=Communications).
// Async: returns opId, signals notifyEvent.
HRESULT __stdcall CapGetDefaultDevice(int32_t role,
                                      HANDLE notifyEvent,
                                      uint32_t *opId);

// Query default-device result.
// Returns E_NOT_SUFFICIENT_BUFFER if buf is too small.
HRESULT __stdcall CapGetDefaultDeviceResult(uint32_t opId,
                                            int32_t *state,
                                            wchar_t *buf, int32_t bufLen,
                                            int32_t *written,
                                            HRESULT *hresult);

// Release default-device operation resources.
HRESULT __stdcall CapGetDefaultDeviceRelease(uint32_t opId);
```

#### Capture (async)

```c
// Initiate capture on the given device ID (async, UI thread for consent).
// Returns S_OK and writes opId. notifyEvent signaled on:
//   - activation complete (format available)
//   - PCM data available
//   - terminal state (stopped/failed/cancelled)
// Only one capture session may be active at a time. Starting a second
// returns E_NOT_VALID_STATE without creating a new operation.
HRESULT __stdcall CaptureStart(const wchar_t *deviceId,
                               HANDLE notifyEvent,
                               uint32_t *opId);

// Query the capture session state and results.
// state: 0=activating, 1=capturing, 2=stopped, 3=failed, 4=cancelled.
// On state>=1 AND format->valid==1: format is populated with the
//   negotiated capture format. On state==0 or failed activation:
//   format->valid==0 and all other format fields are zero.
// On state=1: framesAvailable > 0 means CaptureRead will return data.
// On state>=2: hresult contains the terminal HRESULT;
//   *terminalReason contains the CAP_REASON_* enum value.
HRESULT __stdcall CaptureGetResult(uint32_t opId,
                                   int32_t *state,
                                   CaptureFormat *format,
                                   uint32_t *framesAvailable,
                                   HRESULT *hresult,
                                   int32_t *terminalReason);

// Read captured PCM (interleaved float32) into caller-owned buffer.
// maxFrames is the buffer capacity in frames (based on format.channels).
// framesRead receives the actual frames copied. Returns S_OK.
// Returns S_FALSE if no data is available (non-blocking).
HRESULT __stdcall CaptureRead(uint32_t opId,
                              float *buf, uint32_t maxFrames,
                              uint32_t *framesRead);

// Request capture stop. Non-blocking, idempotent.
// reason: 0=user_stop, 1=permission_revoke, 2=device_lost,
//   3=shutdown, 4=suspend, 5=lock, 6=cancel.
HRESULT __stdcall CaptureRequestStop(uint32_t opId, int32_t reason);

// Release capture session resources. Only valid after terminal state.
// Returns E_ILLEGAL_METHOD_CALL if session is not terminal.
// Drops the registry reference; if the activation callback's strong ref
// is still live (race), session state remains alive until the callback
// releases (see §Callback strong-reference lifetime).
HRESULT __stdcall CaptureRelease(uint32_t opId);
```

#### File picker (async)

See §Picker returns a read handle for the full two-step size-discovery/take API (`PickerOpenFile`, `PickerGetResult`, `PickerRelease`) with `takeHandle` parameter, handle ownership state table, and edge cases.

#### Global

```c
// Initialize the helper. Must be called once on the UI thread before
// any other export. Internally calls RoInitialize(RO_INIT_SINGLETHREADED)
// to initialize the UI-thread WinRT apartment (R7-5). Accepts S_OK and
// S_FALSE (already initialized); rejects RPC_E_CHANGED_MODE (0x80010106)
// — this means the thread was initialized as MTA, which is incompatible
// with UI-thread WinRT objects. Every successful RoInitialize (including
// S_FALSE) is balanced by a same-thread RoUninitialize in CapDestroy.
//
// Stores the initializing thread ID (R8-7). A second CapInit before
// CapDestroy returns E_NOT_VALID_STATE. A failed CapInit leaves no
// state (no RoUninitialize needed, no thread ID stored).
//
// Returns S_OK on success.
HRESULT __stdcall CapInit(void);

// Tear down the helper's application state (operation registry,
// subscription state, internal threads) and call RoUninitialize to
// balance the CapInit's RoInitialize (R7-5). The DLL module remains
// loaded (process-lifetime — R6-1; FreeLibrary is never called).
// ALWAYS returns E_ILLEGAL_METHOD_CALL if:
//   - the operation registry is not empty (every operation must be both
//     terminal AND released via its *Release export — R6-5), OR
//   - the permission subscription is not fully unwound (explicit
//     CapPermissionUnsubscribe required, and all in-flight handler
//     strong refs must have drained — R6-2), OR
//   - any callback strong reference is still live (global callback
//     ref count > 0 — R5-1), OR
//   - the capture thread is still running.
// There is no forced-destroy mode (R4-2).
// On WM_ENDSESSION, do NOT call CapDestroy — request stop and return
// from the wndproc; OS reclaims process resources.
//
// Must be called from the same thread that called CapInit (R8-7).
// Returns RPC_E_WRONG_THREAD if called from a different thread.
// Idempotent: calling CapDestroy when not initialized returns S_OK.
// After success, only CapInit can be called again.
// Repeated CapInit/CapDestroy cycles work correctly — each CapInit
// calls RoInitialize and each CapDestroy calls RoUninitialize.
// A second CapDestroy after success is S_OK (no-op).
HRESULT __stdcall CapDestroy(void);
```

### UI-thread WinRT apartment

*Addresses R7 finding 5.*

All WinRT-using threads must be initialized. The UI thread (Go's main goroutine, pinned via `runtime.LockOSThread`) is the thread from which `CapInit`, `CaptureStart`, `PickerOpenFile`, and other UI-thread exports are called. `CapInit` initializes the WinRT apartment on this thread:

1. `CapInit` calls `RoInitialize(RO_INIT_SINGLETHREADED)` (value 0) internally. The C++/WinRT equivalent is `winrt::init_apartment(winrt::apartment_type::single_threaded)`.
2. Accepted return values:
   - `S_OK` (0): first initialization on this thread. `CapDestroy` must call `RoUninitialize`.
   - `S_FALSE` (1): the apartment was already initialized (compatible mode). **Still balanced** — `RoUninitialize` is called in `CapDestroy` even for `S_FALSE`, because the COM apartment model uses a per-thread reference count and every successful `RoInitialize` must be balanced.
3. Rejected return value:
   - `RPC_E_CHANGED_MODE` (`0x80010106`): the thread was already initialized as MTA (`COINIT_MULTITHREADED`). This is incompatible with STA WinRT UI objects (e.g. `FileOpenPicker`). `CapInit` returns this HRESULT to Go, which logs the failure and refuses to use the helper.
4. `CapDestroy` calls `RoUninitialize` on the same UI thread to balance the `RoInitialize`. `CapDestroy` from a different thread returns `RPC_E_WRONG_THREAD` (R8-7).
5. Repeated `CapInit`/`CapDestroy` cycles: each `CapInit` increments the apartment ref count; each `CapDestroy` decrements it. No leaked apartment refs.
6. **State machine** (R8-7): a second `CapInit` before `CapDestroy` returns `E_NOT_VALID_STATE`. A failed `CapInit` (e.g. `RPC_E_CHANGED_MODE`) leaves no state — no `RoUninitialize` is needed, no thread ID is stored, and `CapDestroy` returns `S_OK` (no-op — nothing to destroy). Idempotent `CapDestroy` after success returns `S_OK`.
7. **Required tests** (R8-7): `S_OK` init + destroy cycle; `S_FALSE` init (already initialized) + destroy; `RPC_E_CHANGED_MODE` init → verify no state left, `CapDestroy` is `S_OK`; repeated `CapInit` without destroy → `E_NOT_VALID_STATE`; wrong-thread `CapDestroy` → `RPC_E_WRONG_THREAD`; double `CapDestroy` → second is `S_OK`; re-init after `CapDestroy` → `S_OK`.

### Ownership rules

- **Operation IDs**: `uint32_t` returned by initiate exports. The caller must eventually call the corresponding `*Release` export after terminal state. The ID is invalid after release. Using a released/unknown ID returns `S_OK` (no-op) for release calls, `E_HANDLE` for query/read calls.
- **OS kernel `HANDLE`s** (events, picked file handle): distinct from opaque operation IDs. Events are created by Go (`CreateEvent`) and passed to the helper; Go owns their lifetime and must not close them before calling the corresponding release. Picked file handles are created by the helper and transferred to Go on query; Go must `CloseHandle` them.
- **UTF-16 buffers**: always caller-allocated, with explicit size parameters. For device/ID string exports (`CapGetDeviceInfo`, `CapGetDefaultDeviceResult`): the helper writes up to `bufLen - 1` characters plus a null terminator and returns `E_NOT_SUFFICIENT_BUFFER` if too small. For picker name buffers (`PickerGetResult`): the helper truncates to `nameBufLen - 1` + null and returns `S_OK` with `requiredNameChars` indicating the full size needed — Go uses the size-discovery step (`takeHandle=0`) to allocate correctly before the take step. Maximum string sizes: 512 `wchar_t` for device IDs/names, 260 `wchar_t` for file names.
- **PCM buffers**: caller-allocated `float*` in `CaptureRead`. The helper copies converted float32 from its internal ring into the caller's buffer. The caller owns the buffer after return.
- **Notification events**: Go signals readiness by creating events; the helper signals them. Go waits via `WaitForMultipleObjects`. The helper never closes Go's events.
- **Idempotent stop**: `CaptureRequestStop` is safe to call in any state — before activation completes, during capture, after stop, or on an unknown/released ID (no-op).
- **Thread safety**: `CapPermissionCheck` and `CaptureRead` may be called from any thread. `CaptureStart` and `PickerOpenFile` must be called from the UI thread (for consent prompt / picker display). `CapInit` and `CapDestroy` must be called from the UI thread. Result query and release exports may be called from any thread.
- **Maximum operation counts**: at most 1 active capture session, 1 active picker, 1 active permission request, 1 active enumeration, 1 active default-device query. Exceeding returns `E_NOT_VALID_STATE`.
- **Request cancellation**: `CaptureRequestStop(opId, reason=cancel)` cancels a pending capture activation. There is no cancellation for permission request, enumeration, default-device query, or picker — they complete or fail naturally (all are fast except picker, which is user-driven).

### HRESULT handling in Go

*Addresses R2 finding 6. Truncation rule per R3 finding 7.*

Do not cast `HRESULT` to `syscall.Errno`. `HRESULT` is a signed 32-bit value in its own namespace; `syscall.Errno` is an unsigned Win32 error code namespace. Conflating them loses information and misidentifies errors.

Go uses a dedicated error type:

```go
type HResult int32

func (hr HResult) Error() string {
    // FormatMessage for the HRESULT value, or hex fallback
}

func (hr HResult) Succeeded() bool { return hr >= 0 }
func (hr HResult) Failed() bool    { return hr < 0 }

func HResultFromUintptr(r uintptr) HResult {
    return HResult(int32(r)) // explicit truncation to low 32 bits
}
```

**`uintptr` → `int32` truncation rule**: `syscall.Syscall` returns `uintptr` (64 bits on amd64). The HRESULT occupies only the low 32 bits. Go must explicitly truncate via `int32(r)` before any sign test. Testing `uintptr < 0` is **never valid** — `uintptr` is unsigned, so the test is always false. The `HResultFromUintptr` helper enforces this.

- All helper exports return `HRESULT` as `uintptr` from `syscall.Syscall`. Go converts to `HResult` via `HResultFromUintptr(r)` and then checks `hr.Failed()`.
- `HRESULT_FROM_WIN32` is decoded only where applicable (e.g., `RegisterHotKey` via `GetLastError`).
- Evidence logs record both the raw HRESULT (hex, from the truncated `int32`) and any separately captured `GetLastError` value (also truncated to `uint32`).
- `S_OK` (0x00000000) = success. `S_FALSE` (0x00000001) = non-error special case. Negative (high bit set after truncation) = error.

### Build contract

- **Architecture**: x64 only, matching the current MSIX package `ProcessorArchitecture="x64"`.
- **Toolchain**: MSVC (Visual Studio Build Tools 2022), targeting Windows 10 19041+.
- **C++ standard**: C++17 with C++/WinRT headers.
- **CRT**: statically linked (`/MT`). This eliminates any VCRT redistributable requirement. The UCRT is an OS component on Windows 10+ and is never redistributed — it is always loaded from the system directory even if an app-local copy exists. `[MS-35]`
- **C++/WinRT**: header-only projection, no runtime redistributable. WinRT APIs are OS-provided system DLLs. `[MS-31]`
- **Windows SDK**: build-time only. The import library `WindowsApp.lib` resolves to OS-provided WinRT DLLs at runtime. No SDK runtime files are redistributed.
- **Output**: single `pulsar-capture.dll`, no additional runtime DLLs alongside it.
- **x64 calling convention**: on x64, `__stdcall`, `__cdecl`, and `__fastcall` are all equivalent — the compiler uses the x64 calling convention regardless of annotation. The `__stdcall` annotation is retained for documentation clarity and `syscall.NewProc` compatibility.

### Loading contract

*Addresses R2 finding 5.*

*Real Go loader per R7 finding 5. `windows.LoadPackagedLibrary` does not exist in the repository's `x/sys/windows v0.46.0`.*

The helper DLL is loaded via a typed Go wrapper around the Win32 `LoadPackagedLibrary` function. The loader uses `windows.NewLazySystemDLL("kernel32.dll")` (R8-7 — not `NewLazyDLL`, which relies on a hidden kernel32 special-case in `x/sys`; the contract should not depend on that exception). `LoadPackagedLibrary` is resolved from that handle:

```go
var (
    kernel32Sys            = windows.NewLazySystemDLL("kernel32.dll") // R8-7
    procLoadPackagedLibrary = kernel32Sys.NewProc("LoadPackagedLibrary")
)

func loadPackagedLibrary(name string) (*windows.DLL, error) {
    namePtr, err := windows.UTF16PtrFromString(name)
    if err != nil {
        return nil, err
    }
    r, _, lastErr := procLoadPackagedLibrary.Call(
        uintptr(unsafe.Pointer(namePtr)),
        0, // reserved, must be 0
    )
    if r == 0 {
        // Zero HMODULE means failure. Check GetLastError.
        if lastErr == windows.ERROR_MOD_NOT_FOUND ||
            lastErr == syscall.Errno(15700) { // APPMODEL_ERROR_NO_PACKAGE
            return nil, lastErr
        }
        return nil, lastErr
    }
    // Convert the HMODULE to a *windows.DLL for FindProc.
    // windows.DLL wraps a Handle (which is a uintptr).
    return &windows.DLL{Name: name, Handle: windows.Handle(r)}, nil
}
```

**Fallback for unpackaged execution**: if `loadPackagedLibrary` returns `APPMODEL_ERROR_NO_PACKAGE` (error code 15700, indicating the process is not running inside a package), fall back to `windows.LoadLibraryEx` with an **absolute executable-directory path**:

```go
func loadHelperDLL() (*windows.DLL, error) {
    dll, err := loadPackagedLibrary("pulsar-capture.dll")
    if err == nil {
        return dll, nil
    }
    // Fall back only on APPMODEL_ERROR_NO_PACKAGE (unpackaged dev)
    if err != syscall.Errno(15700) {
        return nil, fmt.Errorf("LoadPackagedLibrary: %w", err)
    }
    exePath, err := os.Executable()
    if err != nil {
        return nil, err
    }
    absPath := filepath.Join(filepath.Dir(exePath), "pulsar-capture.dll")
    const flags = windows.LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR |
        windows.LOAD_LIBRARY_SEARCH_SYSTEM32
    h, err := windows.LoadLibraryEx(absPath, 0, flags)
    if err != nil {
        return nil, fmt.Errorf("LoadLibraryEx(%s): %w", absPath, err)
    }
    return &windows.DLL{Name: absPath, Handle: windows.Handle(h)}, nil
}
```

`LoadPackagedLibrary` `[MS-40]` searches only the package dependency graph (the app's own package and any framework packages). It does **not** use ambient DLL search, `PATH`, or the current directory. It is available since Windows 8 and is the documented API for loading DLLs from within a signed packaged app.

- **Packaged probe (production)**: `loadPackagedLibrary("pulsar-capture.dll")`. The DLL is found in the MSIX package payload.
- **Unpackaged development/test fallback**: `windows.LoadLibraryEx` with an **absolute executable-directory path** + `LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR | LOAD_LIBRARY_SEARCH_SYSTEM32`. Used **only** when `LoadPackagedLibrary` returns `APPMODEL_ERROR_NO_PACKAGE`.
- The two loaders are **not** interchangeable — `LoadPackagedLibrary` is the production path.

`windows.NewLazyDLL("pulsar-capture.dll")` is **not used**. It falls back to ambient DLL name search and contradicts the review requirement.

**Unit-test loader selection** (R8-7): the `*windows.LazyProc` `.Call` method cannot be replaced in tests. Instead, the loader uses an injectable function wrapper seam:

```go
// Production default; tests replace this var.
var loadPackagedLibraryFn = func(name *uint16, reserved uint32) (uintptr, error) {
    r, _, lastErr := procLoadPackagedLibrary.Call(
        uintptr(unsafe.Pointer(name)), uintptr(reserved))
    if r == 0 {
        return 0, lastErr
    }
    return r, nil
}
```

Tests replace `loadPackagedLibraryFn` with a mock that returns zero with `APPMODEL_ERROR_NO_PACKAGE` (15700) and verify the fallback path constructs the correct absolute executable-directory path and calls `LoadLibraryEx` with `LOAD_LIBRARY_SEARCH_DLL_LOAD_DIR | LOAD_LIBRARY_SEARCH_SYSTEM32`. Tests also inject a mock that succeeds and verify the returned `*windows.DLL` has the correct name and handle. Additional test cases (R8-7): all-other-errors (non-15700 failure returns the error, no fallback attempted), absolute path construction from `os.Executable()`, flags verification, and process-lifetime no-unload (verify `FreeLibrary` is never called). Cross-compile on non-Windows is guarded by build tags.

### MSIX signing and DLL integrity

- The MSIX package signature covers **all** payload files. `AppxBlockMap.xml` contains cryptographic hashes for every 64 KB block of every file in the package. `AppxSignature.p7x` signs the block map. Windows verifies payload integrity at install time and at runtime. `[MS-37]`
- A DLL inside a signed MSIX package does **not** need its own separate Authenticode signature. The package signature provides integrity for all contents.
- For non-Store packages, `uap10:PackageIntegrity` can opt into stronger runtime enforcement. `[MS-37]`

### Redistribution summary

| Component | Ships with DLL? | Reason |
|---|---|---|
| C++/WinRT | No | Header-only, no runtime |
| WinRT APIs | No | OS-provided system DLLs |
| UCRT (`ucrtbase.dll`) | No | OS component on Win 10+ |
| VCRT (`vcruntime140.dll`, etc.) | No | Statically linked via `/MT` |
| Windows SDK runtime | No | Build-time only |
| `pulsar-capture.dll` itself | Yes | The only shipped file |

---

## Picker owner HWND

*Addresses R1 finding 3.*

### Frozen contract

The normal picker path uses a **visible Pulsar top-level window** on the UI thread as the `IInitializeWithWindow` owner. The hidden lifecycle window is **not** an accepted picker owner.

### Exact behavior

1. If the main Pulsar window is visible: use its HWND as the picker owner.
2. If the main Pulsar window is hidden (tray-only mode): call `ShowWindow(hwnd, SW_RESTORE)` and `SetForegroundWindow(hwnd)` before calling `PickerOpenFile`. The picker opens on the restored, visible window.
3. After the picker returns (pick or cancel): the window may be re-hidden if the user was in tray-only mode.

### Why

The `IInitializeWithWindow` documentation at `[MS-16]` does not explicitly require a visible HWND, but it also does not establish that a hidden window gives correct modality, foreground placement, or accessibility. Standard Win32 modality relies on the owner window being visible — a modal dialog owned by a hidden window may appear behind other windows, fail to receive focus, or confuse screen readers.

The root review correctly requires that the hidden owner remain **only** as an explicitly failed-or-proved probe branch, not the selected production contract. The probe may test a hidden owner as a secondary branch and record the result, but the production contract is: restore the visible window first.

---

## AppContainer claims: documented facts vs. probe hypotheses

*Addresses R1 finding 4. P1.0 silent degradation rejected per R2 finding 9.*

The following Win32 APIs are documented for general Win32 desktop use. Their documentation does **not** explicitly prove runtime behavior under the exact `packagedClassicApp` + `appContainer` + signed MSIX combination. Each is a **mandatory probe hypothesis** that must be tested on real hardware with HRESULT/GetLastError captured.

### Documented facts (supported by cited docs)

| API | What the docs say | Source |
|---|---|---|
| `ActivateAudioInterfaceAsync` | Explicitly documented for "Windows Store apps" and AppContainer WASAPI activation. Shows consent prompt on UI thread. | `[MS-5]` |
| `AppCapability.Create("microphone")` | Documented for capability access checking. **SUA-only**: "Create is callable only by SUA apps." Min OS: Windows 10 1903. | `[MS-6]` |
| `FileOpenPicker` + `IInitializeWithWindow` | Documented for desktop apps (including packaged). Picker grants brokered access to the picked file. | `[MS-3] [MS-16]` |
| `DeviceInformation.FindAllAsync` | Documented as agile, ThreadingModel.Both. Universal API contract. | `[MS-18]` |
| `MediaDevice.GetDefaultAudioCaptureId` | Documented. Universal API contract. | `[MS-23]` |
| `IStorageItemHandleAccess::Create` | Documented for obtaining kernel HANDLE from StorageFile. Min client: Windows 10. | `[MS-41]` |
| `LoadPackagedLibrary` | Documented for packaged apps. Searches only package dependency graph. Min client: Windows 8. | `[MS-40]` |

### Probe hypotheses (require real signed-package evidence)

| API | Hypothesis | Fallback if fails | Evidence required |
|---|---|---|---|
| `RegisterHotKey` | Works from a top-level HWND in appContainer | **No fallback — probe is blocked/no-go** (see §P1.0 behavior cannot silently degrade) | HRESULT/GetLastError on Win 10 + Win 11 |
| `WTSRegisterSessionNotification` | Receives `WM_WTSSESSION_CHANGE` in appContainer | **No fallback — probe is blocked/no-go** | GetLastError + actual message receipt |
| `WM_POWERBROADCAST` | Received by top-level HWND in appContainer | None needed if `WM_QUERYENDSESSION`/`WM_ENDSESSION` work for shutdown, but suspend detection is a P1.0 requirement — **blocked/no-go if not received** | Actual message receipt after sleep/resume |
| `WM_QUERYENDSESSION` / `WM_ENDSESSION` | Received in appContainer | **Critical — probe is blocked/no-go** | Actual message receipt during logoff/shutdown |
| `AppCapability.Create` | Works inside signed `packagedClassicApp` appContainer (SUA) | Use `ActivateAudioInterfaceAsync` consent prompt alone; detect denial from activation HRESULT (**conditionally acceptable** — requires proven WASAPI revoke detection; see §AppCapability fallback) | Actual return value + HRESULT |
| Message-only → top-level window migration | Hidden top-level window receives broadcast messages | **If not, probe is blocked at lifecycle** | Actual broadcast message receipt |

### No silent degradation of P1.0 behavior

*Addresses R2 finding 9.*

If any of the following APIs fail under the signed AppContainer, the probe is **blocked/no-go**. A tray-only or manual-only fallback does not satisfy spec §19.2 and must not be silently substituted:

1. **`RegisterHotKey`**: if it fails, there is no global hotkey stop. The probe records this as blocked and a separate decision must select another legal mechanism (e.g., a different hotkey registration API, or a design change that removes the global hotkey requirement).
2. **`WTSRegisterSessionNotification` / `WM_WTSSESSION_CHANGE`**: if lock/unlock notification fails, the app cannot stop capture on lock. `SM_REMOTESESSION` / `WTSQuerySessionInformationW` are **not** substitutes for lock notification (they report remote-session state, not lock state). The probe records this as blocked.
3. **`WM_POWERBROADCAST` / `PBT_APMSUSPEND`**: if suspend notification is not received, the app cannot stop capture on sleep. `WM_QUERYENDSESSION` is **not** a substitute for suspend (it fires on logoff/shutdown, not sleep). The probe records this as blocked.
4. **`WM_QUERYENDSESSION` / `WM_ENDSESSION`**: if shutdown notification is not received, the app cannot finalize drafts on quit. Critical — the probe is blocked.
5. **Deterministic permission-revoke detection**: if neither `AppCapability.AccessChanged` nor a deterministic WASAPI capture error is received within a bounded time after system settings revoke microphone permission, the probe records this as blocked. The acceptable path is: `AccessChanged` fires (preferred), OR `GetBuffer` returns an error HRESULT (secondary). The unacceptable path is: neither signal fires and capture silently continues after permission revocation.

A "blocked/no-go" result from any of these is a valid probe outcome. The probe does not invent a workaround — it reports the failure and names what evidence would be needed to unblock.

### AppCapability fallback

*Tightened per R4 finding 7: fallback acceptability is conditional, not unconditional.*

If `AppCapability.Create("microphone")` fails at runtime (returns an error HRESULT or is unavailable because the package is not SUA):

1. Skip the preflight permission check.
2. Call `ActivateAudioInterfaceAsync` directly — it shows its own consent prompt on the UI thread. `[MS-5]`
3. Detect denial from the HRESULT in the `ActivateCompleted` callback (`E_ACCESSDENIED` or similar).
4. Permission-revoke monitoring is lost (no `AccessChanged` event). Actual capture failure (WASAPI returning an error during `GetBuffer`) becomes the revoke signal.

**This fallback is acceptable only if the mandatory real-hardware revoke test proves that WASAPI `GetBuffer` (or `GetNextPacketSize`) returns a deterministic error HRESULT within a bounded time after the system revokes microphone permission.** If neither `AccessChanged` nor a deterministic WASAPI error fires after revocation, the probe is **blocked** — silent continued capture after permission revocation is not acceptable. The fallback is not unconditionally "degraded but acceptable"; it is conditionally acceptable pending the hardware evidence.

**Fallback promotion rule** (R8-3): in `CAP_PERMISSION_UNAVAILABLE` (-1) mode, the pre-promotion guard rejects promotion — `status==-1` is **not** treated as `Allowed`. The separately gated `activation-consent + proven-revoke-monitor` promotion mode requires:
1. `ActivateAudioInterfaceAsync` consent succeeded (activation was not denied).
2. The hardware probe has proven that WASAPI `GetBuffer`/`GetNextPacketSize` returns a deterministic error HRESULT within a bounded time after permission revocation.
3. The terminal reason is a finalizable reason (user_stop, device_lost, shutdown, suspend, lock).
4. No WASAPI failure other than a hardware-proven privacy mapping may be assumed safe for promotion. Specifically, `AUDCLNT_E_SERVICE_NOT_RUNNING` and `AUDCLNT_E_RESOURCES_INVALIDATED` are **non-promotable** (R8-4) — they may overlap with privacy revocation and cannot be distinguished without AppCapability.

Until the hardware probe proves condition 2, **no** recording in unavailable mode can be promoted. The scenario matrix, Go promotion algorithm, and final answer reflect this.

### Validation gates

All probe hypotheses require:
1. `MakeAppx pack` validation (package structure).
2. WACK (Windows App Certification Kit) validation (API usage, manifest correctness).
3. Real signed MSIX installed on Windows 10 (19041+) and Windows 11, with HRESULT/GetLastError captured for every import and runtime path.
4. Failures recorded as `fail/blocked` with the exact error, not silently ignored.

---

## Lifecycle stop state machine

*Addresses R1 finding 5. Crash-safe draft handling per R2 finding 10.*

### States

```
IDLE → ACTIVATING → CAPTURING → STOPPING → STOPPED
  │                      │           │
  └──────────────────────┴───────────┘
         (any state) → STOPPING → STOPPED
```

### Signal-to-action mapping

| Signal | Source | Capture action | Draft action | Network | Window procedure |
|---|---|---|---|---|---|
| User Stop (hotkey, menu, UI) | Go shell | `CaptureRequestStop(opId, user_stop)` | Finalize valid draft (see §Crash-safe interrupted-draft handling) | None | Returns immediately |
| `WM_QUERYENDSESSION` | OS (logoff/shutdown) | `CaptureRequestStop(opId, shutdown)` | Finalize if possible; `.partial` remains if not | None — must not block wndproc | Return `TRUE` (allow shutdown) |
| `WM_ENDSESSION` (wParam=TRUE) | OS (confirmed shutdown) | `CaptureRequestStop(opId, shutdown)` (idempotent — already sent from `WM_QUERYENDSESSION`). Do **not** call `CapDestroy` — OS reclaims process resources (R4-2). | Same as above | None | Return 0 |
| `WM_POWERBROADCAST` / `PBT_APMSUSPEND` | OS (sleep) | `CaptureRequestStop(opId, suspend)` | Finalize if possible; `.partial` remains if not | None | Return `TRUE` |
| `WM_POWERBROADCAST` / `PBT_APMRESUMEAUTOMATIC` | OS (wake from sleep) | No auto-restart | None | None | Return `TRUE` |
| `WM_WTSSESSION_CHANGE` / `WTS_SESSION_LOCK` | OS (lock screen) | `CaptureRequestStop(opId, lock)` | Finalize if possible; `.partial` remains if not | None | Return 0 |
| `WM_WTSSESSION_CHANGE` / `WTS_SESSION_UNLOCK` | OS (unlock) | No auto-restart; recheck device/permission | Startup cleanup recovers or discards `.partial` files | None | Return 0 |
| `AppCapability.AccessChanged` (denied) | WinRT event → notifyEvent | `CaptureRequestStop(opId, permission_revoke)` | Discard (permission lost mid-capture) | None | N/A (event handler, not wndproc) |
| Device invalidation (WASAPI error) | Capture thread `GetBuffer` fails | Capture thread exits its loop, transitions to `FAILED` | Finalize if ≥ min duration; discard otherwise | None | N/A |
| Late MTA callback after cancel | `ActivateCompleted` fires after `CaptureRequestStop` | Callback checks `cancelled`, releases interfaces, releases strong ref | N/A (no capture started) | None | N/A |

### Invariants

1. **No network operation blocks the window procedure.** All capture stop actions are non-blocking flag-sets + event signals. Upload/finalization that requires network is deferred to a non-wndproc context.
2. **No UI-thread deadlock.** `CaptureRequestStop` sets flags and signals events but never joins the capture thread synchronously. The capture thread cleans up independently. On `WM_ENDSESSION`, Go requests stop and returns from the wndproc without calling `CapDestroy` — the OS reclaims process resources (R4-2).
3. **Resume/unlock recheck:** after `WTS_SESSION_UNLOCK` or `PBT_APMRESUMEAUTOMATIC`, the shell rechecks device availability and permission status before allowing a new capture. It does not auto-restart a stopped capture.

### Stop-reason priority arbitration

*Addresses R6 finding 6.*

`CaptureRequestStop(opId, reason)` is currently a no-op once stopping has begun. This creates a race: a user-stop arriving before `AccessChanged` can win the reason and cause Go to finalize media even though permission was revoked before promotion. The fix is an atomic priority policy.

#### Priority order (highest to lowest)

| Priority | Reason | Value | Promotes draft? |
|---|---|---|---|
| 1 (highest) | `CAP_REASON_OVERFLOW` | 7 | Never — data integrity compromised |
| 1 (tie) | `CAP_REASON_DISCONTINUITY` | 10 | Never — data integrity compromised |
| 2 | `CAP_REASON_PERMISSION_REVOKE` | 1 | Never — recording not authorized |
| 3 | `CAP_REASON_WASAPI_ERROR` | 8 | Never — cause may include undetected permission loss |
| 3 (tie) | `CAP_REASON_FORMAT_ERROR` | 9 | Never — unsupported format |
| 4 | `CAP_REASON_DEVICE_LOST` | 2 | Yes, if ≥ min duration |
| 5 | `CAP_REASON_SHUTDOWN` | 3 | Yes, if ≥ min duration |
| 6 | `CAP_REASON_SUSPEND` | 4 | Yes, if ≥ min duration |
| 7 | `CAP_REASON_LOCK` | 5 | Yes, if ≥ min duration |
| 8 | `CAP_REASON_CANCEL` | 6 | Never — explicit cancel |
| 9 (lowest) | `CAP_REASON_USER_STOP` | 0 | Yes, if ≥ min duration |

#### Atomic compare-and-swap

`CaptureRequestStop` uses an atomic compare-and-swap on the stop reason:

1. If the session is not yet stopping: set the reason and begin stop sequence.
2. If the session is already stopping: compare the new reason's priority against the current reason. If the new reason has **higher priority** (lower number in the table), replace the current reason. If equal or lower priority, no-op.
3. If the session is already terminal: no-op (reason is frozen).

This ensures that overflow and permission_revoke always dominate finalizable reasons, regardless of arrival order.

#### Go-side promotion guard

After the capture session reaches terminal state, Go reads the final terminal reason AND (when available) calls `CapPermissionCheck` to read the current permission status **immediately before** deciding whether to promote `.partial` → `.wav`:

1. If `terminalReason` is `CAP_REASON_OVERFLOW`, `CAP_REASON_PERMISSION_REVOKE`, or `CAP_REASON_DISCONTINUITY` → reject promotion, delete `.partial`.
2. If `terminalReason` is `CAP_REASON_CANCEL` → reject promotion, delete `.partial`.
3. If `terminalReason` is `CAP_REASON_WASAPI_ERROR` or `CAP_REASON_FORMAT_ERROR` → reject promotion, delete `.partial`. These are non-promotable — the cause may include undetected permission loss.
4. If `terminalReason` is a finalizable reason (`user_stop`, `device_lost`, `shutdown`, `suspend`, `lock`) AND `CapPermissionCheck` returns exactly `CAP_PERMISSION_ALLOWED` (status value 1) → promote if ≥ min duration.
5. If `terminalReason` is a finalizable reason BUT `CapPermissionCheck` returns any other status (`CAP_PERMISSION_DENIED_BY_USER`(0), `CAP_PERMISSION_PROMPT_REQUIRED`(2), `CAP_PERMISSION_DENIED_BY_SYSTEM`(3), `CAP_PERMISSION_NOT_DECLARED`(4), `CAP_PERMISSION_UNKNOWN`(5)) → reject promotion, delete `.partial`. The revoke arrived after the stop completed but before promotion, or the permission state is ambiguous.
6. If `CapPermissionCheck` returns `CAP_PERMISSION_UNAVAILABLE` (-1) → reject promotion, delete `.partial` — unless the separately gated `activation-consent + proven-revoke-monitor` mode has been established by the hardware probe (see §AppCapability fallback). In that gated mode, promotion is allowed only if the terminal reason is finalizable, the terminal WASAPI HRESULT is not one of the non-promotable codes (`AUDCLNT_E_SERVICE_NOT_RUNNING`, `AUDCLNT_E_RESOURCES_INVALIDATED`, or any unknown error), and no stop-reason priority override applies. The default before the gate is proven: reject (R8-3).
7. If `CapPermissionCheck` itself fails (returns a failure HRESULT) → reject promotion, delete `.partial`. Check failure is not `Allowed`.

This double-check (terminal reason + live permission status) closes the race window.

#### Distinguishing permission loss from device loss in WASAPI HRESULTs

When `AppCapability` is unavailable (SUA-only fallback), WASAPI errors are the only revoke signal. The helper maps known HRESULTs to terminal reasons:

| HRESULT | SDK constant | Meaning | Terminal reason |
|---|---|---|---|
| `0x80070005` | `E_ACCESSDENIED` | Access denied — documented Win32 permission error | `CAP_REASON_PERMISSION_REVOKE` |
| `0x88890004` | `AUDCLNT_E_DEVICE_INVALIDATED` | Device unplugged, disabled, or hardware reconfigured | `CAP_REASON_DEVICE_LOST` |
| `0x88890010` | `AUDCLNT_E_SERVICE_NOT_RUNNING` | Windows Audio service stopped — NOT a removed device (R8-4); may indicate system state change, not hardware removal | `CAP_REASON_WASAPI_ERROR` (**non-promotable**) |
| `0x88890025` | `AUDCLNT_E_RESOURCES_INVALIDATED` | Resources invalidated — Microsoft documents this as covering suspended streams, quiesced packaged apps, and disconnected exclusive/offload streams (R8-4); NOT equivalent to device removal | `CAP_REASON_WASAPI_ERROR` (**non-promotable**) |
| Any other negative HRESULT from `GetNextPacketSize`/`GetBuffer`/`ReleaseBuffer`/`Start`/`Stop` | — | Unknown failure | `CAP_REASON_WASAPI_ERROR` (**non-promotable** — Go never promotes a draft with this reason) |

**HRESULT mapping scope** (R8-4): the mapping applies to errors from **all** WASAPI calls in the capture loop and lifecycle — `GetNextPacketSize`, `GetBuffer`, `ReleaseBuffer`, `IAudioClient::Start`, and `IAudioClient::Stop` — not only the first two. Each call's failure HRESULT is looked up in this table.

**Stop-reason linearization** (R8-4): the capture thread reloads and commits the final priority-CAS stop-reason value **immediately before** publishing terminal state. This ensures that a higher-priority revoke arriving after the initial stop-reason was set (but before terminal publication) is not lost. The sequence is: (1) complete COM cleanup, (2) reload the atomic stop-reason and accept any higher-priority reason that arrived during cleanup, (3) publish terminal with the final reason, (4) signal `notifyEvent`.

**Removed** (R7-2): `AUDCLNT_E_NOT_ALLOWED` — this name does not correspond to a documented WASAPI SDK constant. The value `0x88890003` is `AUDCLNT_E_WRONG_ENDPOINT_TYPE` per the Windows SDK headers; misclassifying it as a privacy-revocation signal would destroy evidence for the wrong reason.

**Unknown-error policy is non-promotable, not fake-permission-revoke** (R7-2). Unknown audio failures map to `CAP_REASON_WASAPI_ERROR`, not `CAP_REASON_PERMISSION_REVOKE`. The actual HRESULT that Windows returns when microphone privacy is revoked mid-capture is a **mandatory probe discovery** — the hardware probe must capture the exact HRESULT on Windows 10 and 11 after toggling the microphone privacy setting in System Settings during active capture. Until that evidence exists, only `E_ACCESSDENIED` is classified as permission loss. All other unknown errors are non-promotable via `CAP_REASON_WASAPI_ERROR`, which achieves the same fail-closed property for promotion without misattributing the cause in evidence logs.

#### Required barrier tests (R6-6)

1. **User-stop racing permission revoke**: start capture, send `CaptureRequestStop(user_stop)` and `CaptureRequestStop(permission_revoke)` from two threads with a barrier so they execute simultaneously. Verify `permission_revoke` wins (higher priority). Verify Go deletes the `.partial`.
2. **Permission revoke racing user-stop** (reverse order): same test, reversed timing. Verify same outcome.
3. **User-stop racing overflow**: trigger ring overflow and `CaptureRequestStop(user_stop)` simultaneously. Verify `overflow` wins. Verify Go deletes the `.partial`.
4. **Device-loss racing permission revoke**: trigger device removal and permission revoke simultaneously. Verify `permission_revoke` wins. Verify Go deletes the `.partial`.
5. **Go promotion guard with stale reason**: simulate a finalizable stop (`user_stop`) that completes, but then change permission to denied before Go reads the reason. Verify Go calls `CapPermissionCheck`, sees denied, and rejects promotion.

### Frozen draft outcome matrix by reason and duration

*Addresses R4 finding 7: single authoritative matrix, same wording across state machine, scenarios, unresolved proofs, and final answer.*

Note: for the probe, "draft" refers to the disposable native-format evidence WAV that proves the capture path. For the production recording task (future), the draft is the canonical mono upload-ready file. The outcome policies (finalize vs discard vs recover) apply to both cases with the same logic.

| Stop reason | Has ≥ min duration of valid PCM? | Draft outcome | Evidence classification |
|---|---|---|---|
| **User stop** (hotkey/menu/UI) | Yes | Finalize: drain `CaptureRead` → rewrite headers → flush → rename `.partial` → `.wav` | **Valid user media** — pass requires `.wav` on disk |
| **User stop** | No (too short) | Delete `.partial` | **Evidenced deliberate discard** — too short to be useful; probe records reason + duration |
| **Quit** (`WM_QUERYENDSESSION` + `WM_ENDSESSION`) | Yes | Finalize if capture thread completes before process exit; `.partial` survives otherwise | **Valid user media** — pass requires `.wav` OR proven-recoverable `.partial` |
| **Quit** | No | Delete `.partial` if time permits; `.partial` may survive | **Evidenced deliberate discard** OR startup recovery deletes too-short |
| **Suspend** (`PBT_APMSUSPEND`) | Yes | Same as quit — finalize if possible; `.partial` survives otherwise | **Valid user media** — same criteria as quit |
| **Suspend** | No | Same as quit | **Evidenced deliberate discard** |
| **Lock** (`WTS_SESSION_LOCK`) | Yes | Same as quit | **Valid user media** — same criteria |
| **Lock** | No | Same as quit | **Evidenced deliberate discard** |
| **Permission revoke** (`AccessChanged` / WASAPI error) | Any | Delete `.partial` — recording was not authorized for its full duration | **Evidenced deliberate discard** — permission lost; probe records revoke event/HRESULT + deletion |
| **Explicit cancel** (cancel before activation or during capture) | Any | Delete `.partial` | **Evidenced deliberate discard** — user-initiated; probe records reason |
| **Device lost** (WASAPI error) | Yes | Finalize with available data | **Valid user media** — partial but usable |
| **Device lost** | No | Delete `.partial` | **Evidenced deliberate discard** |
| **Ring overflow** | Any | Delete `.partial` — data integrity compromised | **Failure** — probe records overflow event; never finalized |
| **WASAPI discontinuity** (non-first-packet `DATA_DISCONTINUITY`) | Any | Delete `.partial` — data integrity compromised | **Failure** — probe records discontinuity flag + packet index; never finalized |
| **WASAPI error** (unknown HRESULT from `GetBuffer`/`GetNextPacketSize`) | Any | Delete `.partial` — cause may include undetected permission loss | **Failure** — probe records exact HRESULT; never finalized |

**Key definitions** (used identically in all sections):

- **Valid user media**: pass requires a finalized `.wav` on disk with correct RIFF/data headers verified by the local `parseWAV` parser and an independent decoder/tool, **or** a `.partial` file that is proven recoverable on next launch (startup recovery successfully produces a valid `.wav`). A queued `CaptureRequestStop` alone is **never** a pass.
- **Evidenced deliberate discard**: the system correctly decided not to produce a draft. The probe records the reason, captured duration, and deletion outcome. This is not a pass (no valid media) and not a failure (correct behavior). It is a valid probe outcome.
- **Failure**: an error condition that prevents valid output. The probe records the error and the draft is never promoted.

---

## Crash-safe interrupted-draft handling

*Addresses R2 finding 10. Draft writer and file format per R3 finding 3. Probe vs production separation per R5 finding 5.*

### Problem

A window procedure can only signal stop and return; it cannot assume an asynchronous WAV finalization finishes before shutdown/suspend kills the process.

### Who writes drafts

*Addresses R3 finding 3: the note said the helper writes `.partial`, but the ABI gives the helper no file access — `CaptureRead` returns PCM to Go.*

**Go is the sole draft writer.** The helper's only output is interleaved float32 frames via `CaptureRead`. Go continuously drains those frames and writes them to the app-private `.partial` file. The helper never touches the filesystem for draft data.

### Probe artifact vs production draft

*Addresses R5 finding 5.*

This bridge decision freezes two distinct file roles:

1. **Probe evidence artifact** (this bridge/probe task): a short, disposable native-format WAV written at the device's native sample rate and channel count as IEEE float32. Purpose: prove the capture path works under the signed AppContainer. No production bounds (180 s / 50 MiB) apply — probe recordings are short by design (seconds, not minutes). The probe artifact is explicitly **not** a user draft; it is never uploaded or retained as user content. The `.partial` → `.wav` streaming and recovery machinery is exercised by the probe to prove correctness, but the resulting file is evidence, not product.

2. **Production recording draft** (future recording task, outside this bridge scope): a canonical mono upload-ready WAV at a frozen format (sample rate and encoding frozen by the recording task). The production task must implement a **new streaming mono downmixer** that reads float32 frames from `CaptureRead`, downmixes multichannel to mono (sum and divide by channel count, or take the first channel for stereo), resamples to the upload format, and writes the final upload-ready `.wav`. Product bounds (180 s / 50 MiB) are enforced against the **upload-ready mono bytes**, not the native multichannel representation. At the spec's mono target (e.g. 48 kHz, 1 channel, float32 = 192,000 bytes/s), 50 MiB ≈ 273 seconds — the 180-second duration limit fires first. The 50 MiB limit is a safety net. Both limits are enforced by the recording task, not this bridge.

The bridge freezes:
- Helper output format: interleaved float32 at native rate/channels via `CaptureRead`.
- Probe artifact format: native-rate/channel IEEE-float WAV with zero-size `.partial` streaming header.
- Production draft format and bounds: **deferred** to the recording task. This note names the requirement but does not implement it.

### Frozen contract (probe artifact)

#### Streaming `.partial` file format

*Addresses R3 finding 3: a RIFF `data` size of `0xFFFFFFFF` is an RF64 marker (`ds64` chunk required), not a valid ordinary WAV placeholder.*

During capture, Go writes PCM data to a `.partial` file in app-private storage:

1. **On capture start**: Go creates `<draft-dir>/<session-id>.partial` and writes a RIFF/WAV header with **both the RIFF chunk size and the `data` chunk size set to zero**. This is a clearly invalid WAV file (not RF64, not accidentally valid) that carries embedded format metadata:
   - A `fmt ` chunk with the capture format: the device's native sample rate, native channel count, bits per sample = 32, format tag = IEEE float (`WAVE_FORMAT_IEEE_FLOAT`, 0x0003). This header is fixed at 44 bytes (standard WAV with a 16-byte `fmt ` chunk). The `fmt ` chunk records the native capture format, not the pipeline format — Go writes what the helper produces.
   - A `data` chunk header with `chunkSize = 0`.
   - Total header size: 44 bytes.
2. **During capture**: Go reads float32 frames from the helper via `CaptureRead` and appends them to the `.partial` file after the header. The frames are interleaved float32 at the native sample rate and native channel count as reported by `CaptureFormat`. Go calls `FlushFileBuffers` periodically (every ~1 second or every `sampleRate` frames, whichever comes first) to ensure data reaches disk.
3. **On normal stop (user stop, quit)**: Go reads all remaining frames from `CaptureRead` in a loop until `S_FALSE` (no data) and the session is in terminal state. Go then:
   a. Seeks to offset 4 and writes the correct RIFF chunk size (`fileSize - 8`).
   b. Seeks to offset 40 and writes the correct `data` chunk size (`fileSize - 44`).
   c. Calls `FlushFileBuffers` and closes the file.
   d. Renames `.partial` → `.wav` atomically. This produces a valid evidence artifact.
4. **On abnormal termination** (process killed during shutdown/suspend/lock, crash): the `.partial` file survives on disk with zero-size headers but valid PCM data after the 44-byte header.

#### Probe time limit

The probe enforces a short time limit to keep evidence recordings manageable (e.g. 10 seconds for the default-input test). If `framesWritten / sampleRate >= probeTimeLimit`, Go calls `CaptureRequestStop(opId, user_stop)` and finalizes normally. The write that crosses the limit is **clipped at the last whole-frame boundary** — Go writes only complete frames and never allows a partial frame to overshoot.

#### Native-format WAV validity and independent decoder gate

*WAV interoperability gate per R6 finding 7.*

The probe selects a **44-byte IEEE-float WAV header** (standard `WAVEFORMATEX` with format tag `WAVE_FORMAT_IEEE_FLOAT`, no `WAVEFORMATEXTENSIBLE`) as the **initial build-time contract** for evidence artifacts. Whether this header shape is valid and interoperable across decoders is a **probe hypothesis** — the independent decoder gate (below) must confirm or reject it before any signed hardware scenario produces evidence. If the gate rejects it, all components (writer, finalizer, startup scanner, offsets, parser tests, and process-kill recovery) switch together to the required header shape; no artifact may be promoted using the pre-gate assumptions (R7-7). For multichannel native input (>2 channels), the `WAVE_FORMAT_IEEE_FLOAT` tag with `nChannels > 2` loses the `dwChannelMask` speaker layout — this is acceptable because:

1. The probe artifact is app-private and never exposed to external tools as a product file.
2. The production recording task's mono downmixer reads the helper's output directly via `CaptureRead` during capture (not from the probe's evidence WAV); channel layout beyond "first N channels" is not meaningful for downmix-to-mono.
3. If a future task requires preserving the full channel layout in the draft, it can upgrade to `WAVEFORMATEXTENSIBLE` (changing the header from 44 to 68 bytes and updating startup recovery). This is backward-compatible — the current parser in `voice.go` already handles `WAVEFORMATEXTENSIBLE`.

**Independent decoder gate** (R6-7, R7-7): the 44-byte IEEE-float header is the probe's **selected initial build-time contract**, not an asserted interoperability fact. The gate must run **before any signed hardware scenario produces evidence** — it is a prerequisite, not a post-hoc validation. The probe generates and verifies explicit synthetic WAV files against an independent decoder/tool (e.g. `ffprobe`, `sox`, `mediainfo`) on Windows:

| Synthetic file | Channels | Rate | Expected result |
|---|---|---|---|
| Mono float32, 1 second silence | 1 | 48000 | Decoder reports: 1 channel, 48000 Hz, float32, correct duration |
| Stereo float32, 1 second 440 Hz tone | 2 | 48000 | Same; channel count = 2 |
| 4-channel float32, 0.5 seconds silence | 4 | 48000 | Decoder reports 4 channels OR reports an error |
| 8-channel float32, 0.5 seconds silence | 8 | 48000 | Decoder reports 8 channels OR reports an error |

If the independent decoder **requires** a `fact` chunk (common for non-PCM WAV) or `WAVEFORMATEXTENSIBLE` for >2 channels, the probe records this and **all components switch together** (R7-7):
- Add a `fact` chunk after `fmt ` (12 bytes: `"fact" + 4-byte size + 4-byte sample count`). Header grows from 44 to 56 bytes. Update all offset constants, the `.partial` streaming header write, the finalization header rewrite, the startup scanner/recovery header validation, and the process-kill recovery test.
- Or switch to `WAVEFORMATEXTENSIBLE` for multichannel (header grows from 44 to 68 bytes with extended `fmt ` chunk). Same: all components switch together.
- The decision is made based on actual decoder evidence. No evidence artifact may be promoted using the pre-gate 44-byte assumption — the gate result selects one header shape, and that shape is the packaged probe's build-time contract for all subsequent signed hardware scenarios.
- The decision is made based on actual decoder evidence, not assumed.

These synthetic files are disposable probe evidence, not production drafts.

#### Checked frame/channel arithmetic

Go validates frame writes with checked arithmetic:

- `bytesPerFrame = int64(channels) * 4` (float32). `channels` is from `CaptureFormat.channels` (`uint32`, populated from `WAVEFORMATEX.nChannels` which is `WORD`/`uint16`, max 65535). Supported maximum: 8 (§Checked allocation bounds). `bytesPerFrame` at 8 channels = 32. No overflow.
- `bytesToWrite = int64(framesRead) * bytesPerFrame`. `framesRead` is at most the recording ring capacity (~96000 frames for 2 seconds at 48 kHz), `bytesPerFrame ≤ 32`, so `bytesToWrite ≤ ~3 MB` per drain batch. No overflow in `int64`.
- `totalFramesWritten` tracks cumulative frames. Checked against `probeTimeLimit * sampleRate` for probe artifacts.

#### Startup cleanup/recovery

On app startup, before any new capture:

1. Scan `<draft-dir>` for `.partial` files.
2. For each `.partial` file:
   - Read the 44-byte WAV header. Validate: RIFF magic, WAVE magic, `fmt ` chunk at offset 12 with format tag IEEE float (0x0003), `bitsPerSample == 32`, `channels >= 1`, `sampleRate > 0`, `nBlockAlign == channels * 4`, `data` chunk at offset 36.
   - Compute actual PCM byte count from `fileSize - 44`. Physically truncate the file to the last complete frame: if `(fileSize - 44) % bytesPerFrame != 0`, truncate to `44 + ((fileSize - 44) / bytesPerFrame) * bytesPerFrame` and `FlushFileBuffers`.
   - Compute duration: `(truncatedSize - 44) / bytesPerFrame / sampleRate`.
   - If the file contains ≥ minimum duration of valid PCM: rewrite the RIFF and `data` chunk sizes with correct values from the actual (post-truncation) file length, call `FlushFileBuffers`, rename to `.wav` (recovered artifact).
   - **Verify the recovered WAV**: after rename, open and parse the `.wav` with the local `parseWAV` function in `voice.go`. This is the Windows playback parser — it is **not** the coordinator's ingest validator (which does not exist yet and is outside this bridge's scope). If parsing fails, delete the file and log the failure. For the probe, additionally verify with an independent decoder/tool (e.g. `ffprobe`, `sox`) to confirm the file is valid beyond the local parser. For the production recording task (future), the coordinator ingest contract validates upload-ready files.
   - If the file is too short, corrupt (invalid header magic, unrecognized format), or empty: delete it.
3. Log each recovery or deletion with the file name, recovered duration, format, and outcome.

#### Atomic promotion

A draft is promoted from `.partial` to `.wav` only on **confirmed finalization**:

- **User stop**: Go reads all remaining PCM via `CaptureRead` (loop until `S_FALSE` + terminal state), writes final frames to `.partial`, rewrites header sizes, flushes, renames to `.wav`. The finalization is complete only when the `.wav` file exists with correct headers and passes the parse verification.
- **Permission revoke / cancel / too-short capture**: delete the `.partial` file. This is an **evidenced deliberate discard** — the probe records the reason, duration captured, and deletion. It is not a pass (no valid media produced) and not a failure (the system correctly discarded unauthorized or insufficient media).
- **Shutdown / suspend / lock**: `CaptureRequestStop` is called from the wndproc (non-blocking). The capture thread may or may not finish before the OS kills the process. If it finishes and Go drains and finalizes: normal promotion. If the process is killed before finalization: `.partial` survives for startup recovery.

#### Invariants

1. A `.wav` file in the draft directory is always a valid, finalized artifact with correct RIFF/data sizes, verified by the local `parseWAV` parser (and an independent decoder/tool for probe evidence).
2. A `.partial` file has a 44-byte IEEE-float WAV header with zero sizes, valid `fmt ` metadata at the native capture rate/channels, and raw float32 PCM data after byte 44. It is always recoverable (by truncating incomplete frames, rewriting sizes from actual file length, and verifying the result) or deletable on startup.
3. No network operation or blocking wait occurs in the window procedure path.
4. A **valid user media pass** requires a finalized `.wav` on disk or a proven-recoverable `.partial`. A queued `CaptureRequestStop` alone is not success.
5. A **deliberate discard** (permission revoke, explicit cancel, too-short capture) is an evidenced outcome, not a pass — the probe records the reason and verifies deletion.
6. Interrupted shutdown/suspend/lock never reports a pass — the draft either survives as `.partial` for recovery, or is cleanly discarded if too short.

---

## Capture probe defaults

*Addresses R1 finding 6.*

### Frozen defaults for the P1.0 probe

| Parameter | Value | Rationale |
|---|---|---|
| Share mode | `AUDCLNT_SHAREMODE_SHARED` | Standard for apps that coexist with other audio consumers. Exclusive mode is not needed and would prevent other apps from using the mic. |
| Buffer mode | Event-driven (`AUDCLNT_STREAMFLAGS_EVENTCALLBACK`) | Documented for capture since Vista SP1 `[MS-38]`. Matches the existing render pattern in `audio_windows.go`. More responsive than timer-based polling. |
| Format negotiation | Accept `GetMixFormat()` result from the activated `IAudioClient`; helper converts to interleaved float32 (see §Frozen sample representation) | Go writes probe evidence WAVs at native rate/channels as IEEE float32. The production recording task (future) handles mono downmix and upload-format conversion. |
| `AudioDeviceRole` | `AudioDeviceRole::Default` (WinRT, value 0) | Maps to the system's default recording device. `Communications` (value 1) is for voice-call scenarios with a potentially different default device `[MS-23]`. |
| Device-ID type | WinRT device interface path from `MediaDevice.GetDefaultAudioCaptureId(AudioDeviceRole::Default)` or `DeviceInformation.Id` from `FindAllAsync(DeviceClass.AudioCapture)` | This is the format `ActivateAudioInterfaceAsync` expects. For the default device specifically, `StringFromIID(DEVINTERFACE_AUDIO_CAPTURE)` is also legal per the docs `[MS-5]`. |
| Buffer duration | 100 ms (`REFERENCE_TIME` = 100 * 10000 = 1,000,000) | Matches the existing render buffer duration. Shared-mode negotiation may adjust this; the actual buffer size is read back via `GetBufferSize`. |

### Default-input vs. selected-input probe paths

Both exercise the same capture pipeline:

1. **Default input**: `MediaDevice.GetDefaultAudioCaptureId(Default)` → device ID → `CaptureStart(deviceId, ...)` → activation → `Initialize(SHARED, EVENT_CALLBACK)` → `GetService` → `Start` → capture loop.
2. **Selected input**: `DeviceInformation.FindAllAsync(AudioCapture)` → user picks from list → `DeviceInformation.Id` → same `CaptureStart` → same pipeline.

The only difference is the device ID source. The activation, initialization, format negotiation, conversion, and capture loop are identical.

---

## Hidden-window and lifecycle decision

### Required shell change

Replace the current tray message-only window with a **hidden top-level window** that:

- owns the tray icon callback;
- receives broadcast lifecycle messages;
- owns hotkey registration.

The picker is owned by the **visible main window** (see §Picker owner HWND), not the hidden lifecycle window.

### Why

- Microsoft documents that packaged classic apps are implicitly `unmanaged` for lifecycle under `RuntimeBehavior="packagedClassicApp"`. `[MS-1]`
- Microsoft documents that message-only windows do not receive broadcast messages. `[MS-4]`
- P1.0 requires:
  - shutdown/logoff detection;
  - suspend/resume detection;
  - lock/unlock detection;
  - global hotkey stop.

### Selected APIs (probe hypotheses — see §AppContainer claims)

- Shutdown/logoff:
  - `WM_QUERYENDSESSION`
  - `WM_ENDSESSION` `[MS-10] [MS-11]`
- Suspend/resume:
  - `WM_POWERBROADCAST` with `PBT_APMSUSPEND`, `PBT_APMRESUMEAUTOMATIC`, `PBT_APMRESUMESUSPEND` `[MS-12]`
- Lock/unlock/session state:
  - `WTSRegisterSessionNotification`
  - `WM_WTSSESSION_CHANGE` `[MS-13] [MS-14]`
- Hotkey:
  - `RegisterHotKey` on the hidden top-level window, then `UnregisterHotKey` during teardown. `[MS-15]`

---

## Interface boundary

### Boundary shape

Do not pass WinRT or COM interface pointers into Go.

Instead, add a narrow native helper DLL (`pulsar-capture.dll`) beside `pulsar-win-amd64.exe`, loaded via `LoadPackagedLibrary` from the package. The helper owns WinRT async/event state and COM interface lifetimes, and exports only fixed-width types through the ABI defined in §Helper ABI.

### Proposed responsibility split

#### Go shell owns

- tray state and menu text
- hidden top-level lifecycle window (Win32, pure syscall, no CGO)
- visible main window for picker ownership
- `RegisterHotKey` policy and user-visible semantics
- evidence logging
- PCM buffering, WAV writing (`.partial` streaming + atomic promotion)
- app-private draft lifecycle and startup recovery
- `HResult` error type and `FormatMessage` decoding
- `LoadPackagedLibrary` / `LoadLibraryExW` loader selection

#### Native helper (`pulsar-capture.dll`) owns

- WinRT apartment setup and teardown
- `AppCapability` objects and access-change subscriptions (with SUA fallback)
- `DeviceInformation` enumeration/watchers
- `MediaDevice.GetDefaultAudioCaptureId`
- `FileOpenPicker` object creation, HWND initialization, and `IStorageItemHandleAccess::Create` for read handle
- `ActivateAudioInterfaceAsync` agile completion-handler COM object
- `IAudioClient` / `IAudioCaptureClient` lifetime on the dedicated capture thread
- All COM Release calls on the correct thread
- Format negotiation (`GetMixFormat`) and conversion to interleaved float32
- Internal capture ring buffer and overflow tracking
- `CaptureFormat` struct population
- C++ exception → HRESULT conversion at every export boundary
- Strong-reference callback lifetime management

### Why a helper DLL instead of direct Go WinRT bindings

- The repo already avoids CGO and uses raw Win32/COM syscalls.
- The problematic pieces here are the WinRT async/event-heavy APIs, not plain Win32 calls.
- A helper DLL keeps the Go EXE architecture unchanged while containing:
  - WinRT apartment setup;
  - agile completion-handler COM objects;
  - event-token revoke logic;
  - capture-thread COM ownership and same-thread release;
  - format negotiation and conversion;
  - `IStorageItemHandleAccess` for picker file handles;
  - ref-counted callback lifetime management.

---

## Option matrix

| Option | Permission story | Device selection | Lifecycle / revoke story | ABI / ownership | Store posture | Verdict |
| --- | --- | --- | --- | --- | --- | --- |
| Pure Go MMDevice/WASAPI only (`IMMDeviceEnumerator`, `IMMDevice::Activate`) | Weak. No separate documented AppContainer permission preflight; docs for the MMDevice enumeration/activate APIs are desktop-only. `[MS-19] [MS-20]` | Possible via `EnumAudioEndpoints` / `GetDefaultAudioEndpoint`, but again via desktop-only MMDevice APIs. `[MS-19] [MS-21] [MS-22]` | Device invalidation is documented, but microphone privacy revoke is not surfaced as cleanly as `AppCapability.AccessChanged`. | Pure Go is attractive, but would push Store-sensitive behavior into ad hoc COM use. | Too much legal ambiguity for this spike. | Rejected as the primary P1.0 path. |
| WinRT permission + WinRT enumeration + `ActivateAudioInterfaceAsync` + WASAPI capture | Strong. Explicit access check/prompt/change APIs plus Store-targeted WASAPI activation. `[MS-5] [MS-6] [MS-7] [MS-8]` | Strong. `MediaDevice.GetDefaultAudioCaptureId` and `DeviceInformation` cover default and selected devices. `[MS-18] [MS-23] [MS-24]` | Strongest documented combination for this task; pairs cleanly with hidden-window lifecycle watchers. Permission fallback via activation HRESULT if AppCapability is unavailable (SUA-only). | Requires a native helper, but keeps Go free of COM lifetime hazards. ABI is fully asynchronous, versioned, and uses `LoadPackagedLibrary`. | Best documented fit for signed AppContainer package. | **Selected.** |
| WinRT `MediaCapture` | Strong. Built-in microphone capability model; `Failed` event and `SoundLevel` docs mention mute/stop behavior. `[MS-25] [MS-26] [MS-27]` | Strong. `AudioDeviceId`, `StreamingCaptureMode`, `SharingMode`. `[MS-26]` | Good. | Heavier native bridge and recording-oriented abstractions; awkward if the app wants raw PCM and exact buffer ownership. | Legal, but not the cleanest long-term fit with the current Go audio pipeline. | Fallback only. |
| Media Foundation capture (`MFEnumDeviceSources` + Source Reader) | Medium. Capture is documented, but the permission / consent / revoke story is less direct than the WinRT path. `[MS-28] [MS-29]` | Good on paper via endpoint IDs and roles. `[MS-28] [MS-30]` | Medium. Less explicit AppContainer privacy guidance. | Adds a second native media stack over the top of WASAPI. | Probably legal, but not the least-risk path for this spike. | Rejected. |

---

## Rejected or constrained details

### `runFullTrust`

Rejected. The manifest docs make clear that `runFullTrust` is the medium-IL/full-trust route, while the current lane is explicitly `packagedClassicApp` + `appContainer`. `[MS-1]`

### Broad filesystem access

Rejected. The standard picker already grants access to the chosen file, and the file-access guidance says additional locations should come from either manifest capabilities or picker-mediated user choice. `[MS-3] [MS-9]`

### Reusing the current message-only tray window

Rejected for lifecycle ownership. Message-only windows do not receive broadcast messages. `[MS-4]`

### Direct Go ownership of COM capture interfaces

Rejected. The same-thread release rule for `IAudioCaptureClient` is explicitly documented and too easy to violate from Go's goroutine scheduler; the helper must contain those objects. `[MS-17] [MS-34]`

### Hidden window as picker owner

Rejected as the production contract. The IInitializeWithWindow documentation does not establish that a hidden window gives correct modality, foreground placement, or accessibility. The visible Pulsar window is the picker owner; it is restored/activated before the picker opens if currently hidden. A hidden owner may remain only as an explicitly failed-or-proved probe branch. `[MS-16]`

### `AUDCLNT_STREAMFLAGS_AUTOCONVERTPCM` for capture

Not selected for the probe baseline. The flag's documentation is direction-agnostic but the official capture sample does not use it, and there is no explicit confirmation of capture support. The safer path is to accept `GetMixFormat()` and convert in the helper (to float32) and in Go (to pipeline format in the production recording task).

### `syscall.Errno` for HRESULT

Rejected. `HRESULT` is a signed 32-bit value in its own namespace; `syscall.Errno` is an unsigned Win32 error code. Conflating them misidentifies errors. A dedicated `HResult` type preserves the full value. `[R2 finding 6]`

### `windows.NewLazyDLL` for helper loading

Rejected. Uses ambient DLL name search. `LoadPackagedLibrary` is the only safe path for the packaged probe. `[R2 finding 5]`

### Synchronous ABI exports wrapping WinRT async

Rejected. C++/WinRT warns that `.get()` on the UI thread is not appropriate and will assert in debug builds. All async operations use the initiate → event → query contract. `[MS-39]`

### `toEngineFormat` for recording

Rejected. `toEngineFormat` in `voice.go` consumes a complete in-memory clip, allocates a whole stereo output, and resets its interpolation state per call. It is a batch converter for voice-insert playback, not a streaming recorder/resampler. It also produces stereo, whereas the spec requires mono capture. The production recording task must implement a new streaming mono downmixer.

### `FACILITY_ITF` private HRESULT for overflow

Rejected. `FACILITY_ITF` codes are shared across COM interfaces and cannot claim global uniqueness — the initial `0x80040200` collides with `VFW_E_INVALIDMEDIATYPE` from DirectShow. Use the standard `HRESULT_FROM_WIN32(ERROR_BUFFER_OVERFLOW)` = `0x8007006F` with the `CAP_REASON_OVERFLOW` terminal-reason enum for disambiguation. `[R5 finding 3]`

---

## P1.0 scenario mapping

### 1. First Record with explicit permission

- Hidden top-level lifecycle window exists on the pinned UI thread.
- Helper calls `AppCapability.Create("microphone")`.
  - If Create fails (SUA-only): skip to step 4 (ActivateAudioInterfaceAsync consent).
- Go asks helper for `CapPermissionCheck()`.
- If `UserPromptRequired`, Go calls `CapPermissionRequest(notifyEvent, &opId)` → waits for event → `CapPermissionRequestResult(opId, ...)`.
- If allowed (or if AppCapability unavailable), Go calls `CaptureStart(deviceId, notifyEvent, &opId)` → waits for event → `CaptureGetResult(opId, ...)` to confirm activation and get format.
- Evidence:
  - AppCapability.Create success/failure HRESULT
  - access status before prompt
  - access status after prompt (or activation HRESULT if using fallback)
  - activation HRESULT
  - actual capture format from `CaptureFormat` struct (native subtype, native bits, converted float32)

### 2. Record default input

- Go calls `CapGetDefaultDevice(Default, notifyEvent, &opId)` → waits → `CapGetDefaultDeviceResult(opId, ...)`.
- Go calls `CaptureStart(deviceId, notifyEvent, &opId)` → waits → `CaptureGetResult(opId, ...)`.
- Capture thread initializes in shared mode with event-driven callback, converts to float32.
- Go reads PCM via `CaptureRead` and writes the probe evidence WAV at native format.
- Evidence:
  - chosen role (Default)
  - returned device ID string
  - friendly label from enumeration set
  - actual negotiated capture format (native + converted)
  - evidence WAV file verified by `parseWAV` + independent decoder

### 3. Record selected input

- Go calls `CapEnumerateDevices(notifyEvent, &opId)` → waits → `CapEnumerateDevicesResult(opId, ...)`.
- UI selects a `DeviceInformation.Id` from the list.
- Go calls `CaptureStart(selectedDeviceId, notifyEvent, &opId)`.
- Evidence:
  - visible device list (ID + name for each)
  - selected ID
  - activation result
  - actual negotiated capture format (same pipeline as default-input)

### 4. Hide window while recording, then stop with hotkey

- Recording continues because the capture thread is independent of any window visibility.
- `RegisterHotKey` is registered on the hidden lifecycle window (probe hypothesis — **blocked/no-go if it fails**).
- `WM_HOTKEY` triggers Go to call `CaptureRequestStop(opId, user_stop)` (non-blocking).
- Capture thread stops, Go reads remaining PCM, finalizes evidence artifact (`.partial` → `.wav`).
- Evidence:
  - `RegisterHotKey` success/failure + GetLastError
  - stop event and file finalization result
  - hotkey unregistration result

### 5. Open short-file picker

- Visible Pulsar top-level HWND is passed to `PickerOpenFile`.
  - If the visible window is hidden (tray-only mode): `ShowWindow(SW_RESTORE)` + `SetForegroundWindow` first.
- Go waits for event → `PickerGetResult(opId, takeHandle=0, ...)` to discover `requiredNameChars` and `fileSize`.
- Go allocates name buffer, then calls `PickerGetResult(opId, takeHandle=1, ...)` to transfer the file handle.
- On pick: Go receives a kernel `HANDLE` with read access. Reads file bytes via `ReadFile`, verifies format and size against actual bytes (max 50 MiB), closes handle via `CloseHandle`.
- No path dependency — file content is read from the handle, not a filesystem path.
- Evidence:
  - picker open success (HRESULT)
  - cancel vs picked
  - `IStorageItemHandleAccess::Create` HRESULT
  - returned file size and display name
  - successful read and hash of file bytes
  - (secondary probe branch: test with hidden HWND owner, record modality/focus result)

### 6. Quit, logoff, suspend, lock, unlock

*Evidence alignment with async reality per R3 finding 8.*

- Per the lifecycle stop state machine (§Lifecycle stop state machine).
- Per crash-safe draft handling (§Crash-safe interrupted-draft handling).
- **Pass criteria** (not merely "stop was queued"):
  - **Quit / logoff (user stop or `WM_QUERYENDSESSION` + `WM_ENDSESSION`)**: scenario passes only when either (a) a finalized `.wav` file with correct RIFF/data headers exists on disk, or (b) for interrupted-before-finalization paths, a `.partial` file survives on disk and is proven recoverable on next launch (startup recovery produces a valid `.wav` from it). A queued `CaptureRequestStop` alone is not success.
  - **Suspend (`PBT_APMSUSPEND`)**: same as quit — pass requires `.wav` or proven-recoverable `.partial`. The probe must simulate suspend during active capture and verify the file state after resume.
  - **Lock (`WTS_SESSION_LOCK`)**: same as quit — pass requires `.wav` or proven-recoverable `.partial`. The probe must simulate lock during active capture and verify the file state after unlock.
  - **Unlock (`WTS_SESSION_UNLOCK`)**: pass requires that Go successfully rechecks device availability and permission status, and that any recovered `.partial` files from the lock transition have been promoted or discarded.
- Evidence:
  - actual receipt of each message type under signed appContainer on Win 10 and Win 11
  - `GetLastError` for `WTSRegisterSessionNotification`
  - for each signal path: whether a `.wav` was finalized, or a `.partial` survived and was recoverable
  - simulated process kill during capture: `.partial` survives, startup recovery produces valid `.wav` or correctly discards too-short file
  - exact timing: how long between `CaptureRequestStop` and terminal state vs. how long before the OS kills the process

### 7. Permission revoke while app is running

*Evidence alignment with async reality per R3 finding 8.*

- Preferred signal: `AppCapability.AccessChanged` → notifyEvent fired → Go calls `CapPermissionCheck` (reads current status via drain protocol) → Go calls `CaptureRequestStop(opId, permission_revoke)`.
- Secondary proof: actual capture stop/failure path from the helper (WASAPI `GetBuffer` error → session transitions to `FAILED`).
- If `AppCapability` is unavailable (SUA-only fallback): permission revoke is detected **only** via WASAPI capture error. This fallback is acceptable **only if** the mandatory real-hardware revoke test proves that `GetBuffer` returns a deterministic error HRESULT within a bounded time after the system revokes microphone permission. If neither `AccessChanged` nor a deterministic WASAPI error fires, the probe is **blocked** — silent continued capture after permission revocation is not acceptable.
- Evidence:
  - old/new access status (if AppCapability available)
  - exact HRESULT from `GetBuffer` after revocation (if AppCapability unavailable)
  - time between system-settings revocation and capture-side detection (either event or error)
  - capture shutdown result: `.partial` deleted (permission revoke → **evidenced deliberate discard** per §Frozen draft outcome matrix)
  - whether restart after re-allow is possible without app relaunch

---

## Minimum OS, architecture, signing, redistribution

- Current package minimum `10.0.19041.0` already satisfies the selected WinRT manifest model. `[MS-1]`
- `AppCapability` requires Windows 10 version 1903 (build 18362), so it fits under the existing 19041 floor. `[MS-6] [MS-8]`
- `DeviceInformation` and `MediaDevice.GetDefaultAudioCaptureId` are available on the Windows 10 universal contract. `[MS-18] [MS-23] [MS-24]`
- `ActivateAudioInterfaceAsync` is available since Windows 8 and is documented for desktop and UWP apps. `[MS-5]`
- `IStorageItemHandleAccess::Create` is available since Windows 10. `[MS-41]`
- `LoadPackagedLibrary` is available since Windows 8. `[MS-40]`
- Current repo packages x64 only; the helper matches exactly for P1.0.
- Signed-package eligibility:
  - nothing here requires developer mode;
  - nothing here requires an unpackaged process;
  - nothing here requires sandbox weakening.
- Build/distribution:
  - the helper DLL is staged beside the EXE inside the MSIX and inherits package signing through the signed MSIX `[MS-37]`;
  - no separate Authenticode signature is required;
  - no new Store capability approval path is introduced if the design stays within microphone + picker;
  - CRT is statically linked (`/MT`); no VCRT or UCRT redistributable needed `[MS-35]`.

---

## Unresolved hardware proofs

These are not design blockers, but they are still mandatory probe evidence before implementation can be called closed:

1. Prove that `AppCapability.Create("microphone")` + `RequestAccessAsync()` behaves as expected inside the signed `packagedClassicApp` + `appContainer` package on real Windows 10 and Windows 11. Record whether it succeeds (SUA requirement met) or fails (fallback to ActivateAudioInterfaceAsync consent). Map the complete `AppCapabilityAccessStatus` enum values observed.
2. Prove that `ActivateAudioInterfaceAsync()` on the selected device ID succeeds in the signed package, not only unpackaged. Record the HRESULT (truncated from `uintptr` to `int32`).
3. Prove that microphone privacy revoke triggers either `AccessChanged`, a deterministic capture failure (WASAPI HRESULT from `GetBuffer`), or both, and record the exact HRESULT / event order and time between system-settings revocation and detection. **If `AppCapability` fallback is used, the revoke test must prove deterministic WASAPI failure** — if neither signal fires, the probe is blocked.
4. Prove that the hidden top-level window receives:
   - `WM_QUERYENDSESSION` / `WM_ENDSESSION`
   - `WM_POWERBROADCAST`
   - `WM_WTSSESSION_CHANGE`
   Record GetLastError for `WTSRegisterSessionNotification` and actual message receipt. **If any of these fail, the probe is blocked/no-go** (not silently degraded). For each lifecycle signal received during active capture, prove that either a finalized `.wav` exists on disk or a `.partial` file survives and is recoverable on next launch. A queued `CaptureRequestStop` alone is not a pass.
5. Prove that `RegisterHotKey` works from the hidden lifecycle window while the app is in tray mode. Record success/failure + GetLastError. **If it fails, the probe is blocked/no-go** — tray-menu-only fallback does not satisfy spec §19.2.
6. Prove that `FileOpenPicker` + `IStorageItemHandleAccess::Create` returns a readable kernel handle for the picked file (not just a path) under this exact signed AppContainer. Record the `IStorageItemHandleAccess::Create` HRESULT. Read and hash actual bytes from the handle. Distinguish `fileSize == 0` (real zero-byte) from `fileSize == -1` (unknown/virtual). As a secondary branch, test with a hidden owner and record modality/focus result. **If `IStorageItemHandleAccess::Create` fails under AppContainer, the probe is blocked** and an alternative (WinRT stream reads) must be selected.
7. Run WACK (Windows App Certification Kit) on the packaged MSIX with the helper DLL and microphone capability. Record pass/fail and any API-usage warnings.
8. Prove that `GetMixFormat()` returns a supported format (PCM 16/24/32 or IEEE float 32) on the activated `IAudioClient` and that the event-driven shared-mode capture loop produces correct float32 PCM after conversion on both Windows 10 and Windows 11. Run the deterministic conversion test vectors (§Frozen sample representation) against real device output and verify round-trip correctness.
9. Prove that `LoadPackagedLibrary("pulsar-capture.dll", 0)` successfully loads the helper from the MSIX package on both Windows 10 and Windows 11. Record the HRESULT (truncated to `int32`) and loaded module path.
10. Prove crash-safe draft recovery: simulate process kill during capture (e.g., `taskkill /F`), verify `.partial` file survives with zero-size WAV headers and valid PCM data after byte 44, verify startup recovery rewrites headers from actual file length and produces a valid `.wav`, or correctly discards a too-short/corrupt file. This test must pass end-to-end — a `.partial` file on disk is not sufficient; the recovered `.wav` must be playable and verified by both `parseWAV` and an independent decoder/tool.
11. Prove callback strong-reference safety: run the adversarial tests from §Callback strong-reference lifetime (cancel+immediate release, picker cancel+release, unsubscribe racing AccessChanged, rapid start/stop/release cycles, **deterministic callback barrier** (R6-1), **unsubscribe fence barrier** (R6-2), **injected CoInitializeEx failure** (R6-4), **injected activation launch failure** (R6-4)). No crash, no use-after-free, no leaked references.
12. Prove stop-reason priority arbitration: run the barrier tests from §Stop-reason priority arbitration (user-stop vs permission revoke both orderings, user-stop vs overflow, device-loss vs revoke, Go promotion guard with stale reason). Verify that higher-priority reasons always win and that Go rejects promotion when permission is denied regardless of terminal reason.
13. Prove WAV interoperability: run the independent decoder gate from §Native-format WAV validity (mono, stereo, 4-channel, 8-channel synthetic IEEE-float WAVs) against an independent decoder/tool on Windows. Record channel/rate/frame metadata. If the decoder requires a `fact` chunk or `WAVEFORMATEXTENSIBLE`, update the writer and record the decision.
14. Prove PCM conversion safety under sanitizers: run the deliberately unaligned conversion test vectors under AddressSanitizer and UBSan. Verify no undefined behavior reports. Verify bit-exact results for power-of-two vectors and tolerance-bounded results for others.

---

## Final answer to the task

Select a narrow native WinRT helper (`pulsar-capture.dll`) plus AppContainer-safe WASAPI activation:

- Permission: `AppCapability` (with SUA-only caveat, complete `AppCapabilityAccessStatus` enum mapping, and `ActivateAudioInterfaceAsync` consent fallback — fallback acceptable only with proven WASAPI revoke detection)
- Enumeration: `MediaDevice` + `DeviceInformation`
- Capture: `ActivateAudioInterfaceAsync` → `IAudioClient` / `IAudioCaptureClient` on a helper-owned MTA capture thread; `GetMixFormat` runs before `Initialize` (the format it returns is passed to shared-mode `Initialize`)
- Format: `GetMixFormat()` from the device; helper converts to interleaved float32 with exact conversion for `WAVEFORMATEX` and `WAVEFORMATEXTENSIBLE` including packed 24-bit, 24-in-32; all sample reads via `memcpy` or byte assembly (no pointer casts — R6-7); signed 24-bit uses safe signed arithmetic on representable values: `int32_t val = (int32_t)u; if (u >= 0x800000u) val -= 0x1000000;` — no implementation-defined unsigned-to-signed casts (R7-1); `wValidBitsPerSample`, `nBlockAlign` validation; conversion vectors are bit-exact where float32 is exactly representable, tolerance-based otherwise; int32 `INT32_MAX` correctly rounds to `1.0f` in float32; deliberately unaligned test vectors run under ASan/UBSan
- Picker: `FileOpenPicker` + `IInitializeWithWindow` with a **visible** Pulsar window; returns a kernel read handle via `IStorageItemHandleAccess::Create` (probe hypothesis under AppContainer — blocked if it fails), not a path; take-once handle transfer; `*hresult` is always the operation outcome (never overwritten with transfer-state codes — R6-3); `*handleTaken` alone reports transfer; complete truth table covers all states (R6-3); max enforced against actual bytes read; picker name buffer truncates and returns `S_OK` with `requiredNameChars` (not `E_NOT_SUFFICIENT_BUFFER`); invalid `takeHandle` values return `E_INVALIDARG`; `PENDING` state returns `S_FALSE`
- Hotkey/lifecycle owner: hidden top-level Win32 window on the existing pinned UI thread (probe hypothesis for appContainer — blocked/no-go if it fails, not silently degraded)
- UI-thread event integration: dedicated waiter goroutine pinned to its own OS thread (`runtime.LockOSThread`) waits on helper events (`WaitForMultipleObjects` — capture readiness, picker completion, permission change, shutdown) and uses `PostMessageW` to forward UI actions to the main thread (R8-5); the existing `pGetMessageW.Call` pump is unchanged; `CapInit`/`CapDestroy`/`CaptureStart`/`PickerOpenFile`/`CapPermissionRequest` remain on the UI thread where WinRT requires them; the waiter calls only thread-safe query/read exports; waiter creates a `shutdownEvent` (manual-reset) included in the wait array; on app shutdown Go signals `shutdownEvent`, the waiter performs a final drain and exits; event handles are valid for the waiter's lifetime (created before start, closed after exit and all operations released — R8-5)
- Helper ABI: fully asynchronous (initiate → event → query), auto-reset notification events are **readiness hints only** (coalescing is expected; Go drains all ready state per wake, including `CapPermissionCheck` for `AccessChanged`), versioned structs with `structSize` and `valid` flags, operation IDs with wrap/exhaustion handling, `__stdcall`, fixed-width types, HRESULT returns (Go truncates `uintptr` → `int32` before sign test), C++ exception safety at every export, `/MT` static CRT, no runtime redistributables
- Helper loading: `LoadPackagedLibrary` via `kernel32.NewProc("LoadPackagedLibrary")` (production — `windows.LoadPackagedLibrary` does not exist in `x/sys v0.46.0`; R7-5), absolute-path `windows.LoadLibraryEx` (unpackaged dev only, on `APPMODEL_ERROR_NO_PACKAGE`); kernel32 handle obtained via `windows.NewLazySystemDLL("kernel32.dll")` (not `NewLazyDLL` — R8-7); injectable function wrapper seam for unit-test loader selection (R8-7)
- UI WinRT apartment: `CapInit` calls `RoInitialize(RO_INIT_SINGLETHREADED)` on the UI thread; accepts `S_OK`/`S_FALSE`, rejects `RPC_E_CHANGED_MODE`; balanced by `RoUninitialize` in `CapDestroy`; repeated `CapInit`/`CapDestroy` cycles work correctly (R7-5); `CapInit` stores the initializing thread ID; a second `CapInit` before `CapDestroy` returns `E_NOT_VALID_STATE`; `CapDestroy` from a wrong thread returns `RPC_E_WRONG_THREAD` without teardown (R8-7); required tests: `S_OK`/`S_FALSE` init+destroy, `RPC_E_CHANGED_MODE` leaves no state, repeated init → `E_NOT_VALID_STATE`, wrong-thread destroy → `RPC_E_WRONG_THREAD`, double destroy → `S_OK`, re-init after destroy → `S_OK` (R8-7)
- COM ownership: capture thread created and `CoInitializeEx MTA` proven **before** `ActivateAudioInterfaceAsync` launch (R6-4); if `CoInitializeEx` fails, failure is published through the operation (not `CaptureStart` return HRESULT — R7-4); MTA callback → mutex-protected handoff (linearization point — R6-4) → capture thread owns GetMixFormat/Initialize/GetService/Release exclusively; `readyEvent` has 5-second finite timeout (R7-4); capture thread does NOT hold a ref-counted session ref (R8-2 — eliminates self-join deadlock); thread sets atomic `threadDone=1` as its final instruction; terminal state published only after `CoUninitialize` (R7-4); C++/WinRT async cycle explicitly broken at callback point (R7-4); executable `CaptureStart` branch table covers every failure point with function HRESULT, `*opId`, registry, and cleanup owner (R8-1); injected `CoInitializeEx`-failure, activation-launch-failure, timeout, and cancel-wakes-thread tests with exact release counts
- HRESULT handling: dedicated `HResult` Go type with explicit `uintptr` → `int32` truncation via `HResultFromUintptr`, no `syscall.Errno` conflation, hex logging; overflow uses standard `HRESULT_FROM_WIN32(ERROR_BUFFER_OVERFLOW)` = `0x8007006F` (not a private `FACILITY_ITF` code — R5-3); WASAPI HRESULT table uses only SDK-verified constants — `AUDCLNT_E_NOT_ALLOWED` removed (R7-2: `0x88890003` is `AUDCLNT_E_WRONG_ENDPOINT_TYPE`); `AUDCLNT_E_SERVICE_NOT_RUNNING` and `AUDCLNT_E_RESOURCES_INVALIDATED` reclassified from `CAP_REASON_DEVICE_LOST` to `CAP_REASON_WASAPI_ERROR` (non-promotable — R8-4); HRESULT mapping scope expanded from `GetBuffer`/`GetNextPacketSize` only to all WASAPI calls including `Initialize`, `GetService`, `Start` (R8-4); stop-reason CAS reloads committed value before terminal publication to prevent stale-reason races (R8-4); unknown audio errors map to `CAP_REASON_WASAPI_ERROR` not `CAP_REASON_PERMISSION_REVOKE` (R7-2); actual privacy-revoke HRESULT is mandatory probe discovery
- Permission ABI: named `CAP_PERMISSION_*` enum with explicit exhaustive switch from raw `AppCapabilityAccessStatus` values (R8-3); raw `NotDeclaredByApp`(1) → `CAP_PERMISSION_NOT_DECLARED`(4), raw `Allowed`(4) → `CAP_PERMISSION_ALLOWED`(1) — a direct cast NEVER reaches Go; unknown/future values → `CAP_PERMISSION_UNKNOWN`(5) (fail-closed); `CAP_PERMISSION_UNAVAILABLE`(-1) is a no-go for promotion unless the separately gated `activation-consent + proven-revoke-monitor` mode is established; `AccessChanged` subscription state holds a **strong** `AppCapability` reference (not raw pointer — R8-3)
- Callback lifetime: every async callback (activation, picker, permission, enumeration, `AccessChanged`) holds a **strong operation reference** until its final return; release exports drop only the registry reference; ref-count reaches zero only when all holders release; helper DLL is loaded once and **never unloaded** (`FreeLibrary` never called — R6-1); `CapDestroy` tears down application state only; module reclaimed at process exit; COM object ownership and release graph frozen (R6-1); operation destructor never joins threads (R8-2); deterministic barrier tests (not only stress loops — R6-1); adversarial race tests required (R5-1)
- Session lifetime: two-phase `CaptureRequestStop` + `CaptureRelease`; `CapDestroy` requires **empty operation registry** (all operations released, not just terminal — R6-5), fully unwound permission subscription (R6-2), `threadDone==1` for any started capture (R8-2), zero live callback refs; no forced shutdown path; `CaptureRequestStop` uses atomic stop-reason priority (overflow > permission_revoke > all others — R6-6) with stop-reason linearization (reload/commit before terminal publication — R8-4); Go rechecks terminal reason AND permission status (must be exactly `CAP_PERMISSION_ALLOWED`(1) — R8-3) before promotion; on `WM_ENDSESSION`, Go requests stop and returns from the wndproc, OS reclaims process resources (R4-2)
- Sample representation: helper converts native format to interleaved float32; all reads via `memcpy`/byte assembly (R6-7); valid bits are **left-aligned** in PCM containers (R4-1); `CaptureFormat` struct with `valid` flag, native metadata including `nativeValidBits` and `nBlockAlign`; conversion by `2^(validBits-1)` divisor; signed 24-bit uses safe signed arithmetic: `int32_t val = (int32_t)u; if (u >= 0x800000u) val -= 0x1000000;` (R7-1); deliberately unaligned vectors under ASan/UBSan; 44-byte IEEE-float WAV is the **selected initial build-time contract** — interoperability confirmed or rejected by independent decoder gate before signed hardware scenarios (R7-7); if gate requires `fact`/extensible, all components switch together
- WASAPI packet drain: capture thread loops `GetNextPacketSize`/`GetBuffer`/`ReleaseBuffer` until packet size is zero on every event wake (auto-reset is a readiness hint, not one-shot); **whole-packet ring preflight before conversion/copy** — if ring lacks room for entire packet, zero frames written, `ReleaseBuffer` called, terminal overflow (R7-3); first packet `DATA_DISCONTINUITY` accepted, subsequent → terminal `CAP_REASON_DISCONTINUITY` (R7-3); `TIMESTAMP_ERROR` logged but accepted (R7-3); acquired packet always released before stop/error; Go drains `CaptureRead` until `S_FALSE`, then queries all operations AND `CapPermissionCheck` per wake (R4-3, R5-4)
- Recording ring overflow: terminal `FAILED` state with `HRESULT_FROM_WIN32(ERROR_BUFFER_OVERFLOW)` = `0x8007006F` plus `CAP_REASON_OVERFLOW` terminal reason; WASAPI packet released via `ReleaseBuffer` before COM teardown; separate lossy meter ring for UI only (R5-3)
- Checked allocation bounds: channel count ≤ 8 (field is `uint16`, max 65535, but >8 rejected); sample rate ≤ 384 kHz; ring capacity = `max(2 × sampleRate, bufferFrames)` frames (R8-6 — guaranteed to hold at least two full WASAPI periods; bounds to 6.1 MiB at 384 kHz 8ch); all arithmetic in wide types with overflow check before allocation; Go uses `int64` for frame/byte counters (R4-6); scratch-buffer conversion: capture thread converts into a pre-allocated scratch buffer (`maxFrames × channels × sizeof(float32)`) and commits to ring only after the entire packet converts successfully — partial-packet writes are impossible (R8-6)
- Picker: two-step size-discovery/take API — `PickerGetResult(takeHandle=0)` probes `requiredNameChars` and `fileSize` without transferring; `takeHandle=1` transfers the file handle exactly once; `PickerRelease` closes untaken handles; every pointer parameter classified as mandatory or optional with validation order; null mandatory pointers (`state`, `hresult`, `handleTaken`, `fileHandle` with `takeHandle=1`) return `E_POINTER` without transfer/close; negative `nameBufLen` treated as zero capacity; complete truth table covers all null/negative combinations (R7-6); table-driven ABI tests for every row (R7-6); every error path specified for repeat take, release-before-take, invalid takeHandle, and PENDING state (R4-4, R5-4)
- Probe artifact vs production draft: the bridge/probe writes **short disposable native-format evidence WAVs** to prove the capture path; they are not user drafts; no production bounds apply; the production recording task (future) implements a streaming mono downmixer, freezes the canonical upload format, and enforces 180 s / 50 MiB against upload-ready mono bytes; `toEngineFormat` is not used for recording (R5-5)
- Draft safety: **Go is the sole draft writer**; `.partial` streaming file at native capture rate and channel count as IEEE float32 with zero-size WAV headers (not RF64 marker); writes clipped at whole-frame boundaries; startup recovery truncates incomplete frames, rewrites headers, and verifies with `parseWAV` (local playback parser) + independent decoder/tool; `parseWAV` is not the ingest validator (R5-6)
- Lifecycle evidence: **valid user media** requires finalized `.wav` or proven-recoverable `.partial`; permission revoke/cancel/too-short is **evidenced deliberate discard** (not a pass, not a failure); queued `CaptureRequestStop` alone is never a pass; `AppCapability` fallback is conditional on proven WASAPI revoke detection, not unconditionally "degraded but acceptable"; same wording in state machine, scenarios, unresolved proofs, and this final answer (R4-7)

Do not select:

- `runFullTrust`
- `broadFileSystemAccess`
- a pure MMDevice-only capture implementation as the primary Store probe path
- Media Foundation as the primary P1.0 capture path
- `AUDCLNT_STREAMFLAGS_AUTOCONVERTPCM` for capture (unconfirmed for capture direction)
- hidden window as picker owner (production contract requires visible window)
- `syscall.Errno` for HRESULT (wrong namespace)
- `uintptr < 0` for HRESULT sign test (unsigned — always false)
- `windows.NewLazyDLL` for helper loading (ambient search)
- synchronous ABI exports wrapping WinRT async (UI-thread blocking)
- `StorageFile.Path` as the picker result (may be virtual/null)
- forced `CapDestroy` with active operations (contradictory — free races un-cancellable callbacks; removed in R4-2)
- tray-only/manual fallback for failed P1.0 lifecycle APIs (blocked/no-go, not degraded)
- lossy recording ring (overflow = terminal failure; lossy meter ring is internal to Go and separate)
- helper as draft writer (Go is the sole writer; ABI provides only `CaptureRead`)
- RIFF `data` chunk size `0xFFFFFFFF` as WAV placeholder (RF64 marker — use zero sizes instead)
- `FACILITY_ITF` private HRESULT for overflow (collides with `VFW_E_INVALIDMEDIATYPE`; use standard `HRESULT_FROM_WIN32(ERROR_BUFFER_OVERFLOW)` — R5-3)
- `toEngineFormat` for recording (batch converter, not streaming; produces stereo, spec requires mono — R5-5, R5-6)
- one-step picker result API that conflates size discovery with handle transfer (leaks handles on insufficient buffers)
- implementation-defined arithmetic right shift for PCM extraction (C++17 build; use explicit unsigned + sign extension — R5-2)
- `parseWAV` as "the same parser used by ingest" (it is the local playback parser; ingest validation is a future task — R5-6)
- auto-unsubscribe of `AccessChanged` on `CapDestroy` (explicit `CapPermissionUnsubscribe` required — R5-4)
- assuming auto-reset events wake exactly once per signal (coalescing is expected; drain all ready state — R5-4)
- callback-free operation lifetime (all async callbacks hold strong refs; release exports drop only registry refs — R5-1)
- `FreeLibrary` during process lifetime (callback/COM Release code may execute after CapDestroy — R6-1)
- `*(uint32_t*)ptr` pointer casts for PCM sample reads (strict-aliasing/alignment UB — R6-7; use `memcpy`)
- out-of-range `uint32_t`→`int32_t` casts for signed PCM conversion (implementation-defined in C++17 — R6-7; use conditional subtraction)
- overloading `*hresult` with transfer-state codes in picker (R6-3; `*hresult` is always the operation outcome)
- creating capture thread after activation launch (CoInitializeEx failure leaves IAudioClient orphaned — R6-4)
- terminal-but-unreleased operations satisfying `CapDestroy` (must be released — R6-5)
- first-wins stop-reason arbitration (a benign user_stop can promote after permission revoke — R6-6; use priority CAS)
- Go's `notifyEvent` handle signaled directly by `AccessChanged` handler (handle race — R6-2; use duplicated handle)
- `(int32_t)(u - 0x1000000u)` for signed 24-bit conversion (unsigned wrapping + implementation-defined cast — R7-1; use `int32_t val = (int32_t)u; if (u >= 0x800000u) val -= 0x1000000;`)
- `AUDCLNT_E_NOT_ALLOWED` as a privacy-revoke HRESULT (`0x88890003` is `AUDCLNT_E_WRONG_ENDPOINT_TYPE` — R7-2)
- unknown WASAPI errors classified as `CAP_REASON_PERMISSION_REVOKE` (misattributes cause — R7-2; use `CAP_REASON_WASAPI_ERROR`)
- accepting non-first-packet `AUDCLNT_BUFFERFLAGS_DATA_DISCONTINUITY` as valid recording data (integrity loss — R7-3)
- "copy then check overflow" ring write (can overrun or leave partial packet — R7-3; preflight whole packet)
- `CaptureStart` returning `CoInitializeEx` failure HRESULT directly (all async failures travel through the operation — R7-4)
- unbounded `readyEvent` wait (5-second finite timeout — R7-4)
- terminal state before `CoUninitialize` (COM cleanup must finish first — R7-4)
- `windows.LoadPackagedLibrary` in Go code (does not exist in `x/sys v0.46.0` — R7-5; use `kernel32.NewProc`)
- missing UI-thread `RoInitialize`/`RoUninitialize` balance (R7-5)
- partial picker truth table (must cover all null/negative pointer combinations — R7-6)
- asserting 44-byte WAV interoperability before the independent decoder gate runs (R7-7)
- direct cast of raw `AppCapabilityAccessStatus` enum to ABI values (raw 1=`NotDeclaredByApp` ≠ ABI 1=`CAP_PERMISSION_ALLOWED` — R8-3)
- raw pointer (not strong ref) for `AccessChanged` subscription's `AppCapability` (dangling if subscription outlives creator — R8-3)
- `CAP_PERMISSION_UNAVAILABLE` as unconditionally acceptable for promotion (requires proven `activation-consent + revoke-monitor` gate — R8-3)
- `AUDCLNT_E_SERVICE_NOT_RUNNING` classified as `CAP_REASON_DEVICE_LOST` (audio service stopped, not device removal — R8-4)
- `AUDCLNT_E_RESOURCES_INVALIDATED` classified as `CAP_REASON_DEVICE_LOST` (covers suspended/quiesced streams, not device removal — R8-4)
- HRESULT mapping scoped only to `GetBuffer`/`GetNextPacketSize` (errors from `Initialize`/`GetService`/`Start` need classification too — R8-4)
- publishing terminal stop-reason from stale CAS (must reload committed value before publication — R8-4)
- `MsgWaitForMultipleObjectsEx` to combine handle waits and message pump on the existing UI thread (risks starving messages or events; complicates the proven `pGetMessageW.Call` loop — R8-5)
- `shared_ptr<Session>` preventing capture thread self-join deadlock (destructor join + thread holding ref = infinite wait — R8-2)
- ring capacity = `2 × sampleRate` without considering `bufferFrames` (WASAPI can request a period larger than 1 second — R8-6)
- writing WASAPI packet directly to ring before conversion completes (partial-packet visibility on conversion failure — R8-6)
- `windows.NewLazyDLL("kernel32.dll")` for loader (relies on hidden `x/sys` exception for kernel32; use `NewLazySystemDLL` — R8-7)
- `CapInit`/`CapDestroy` without thread ID tracking (silent wrong-thread use corrupts state — R8-7)
- non-injectable loader function in tests (cannot test packaged vs unpackaged paths — R8-7)

---

## Sources

- `[MS-1]` Application manifest schema, `uap10:TrustLevel`, `uap10:RuntimeBehavior`, `packagedClassicApp` + `appContainer`, implicit unmanaged lifecycle, 19041 requirement:
  <https://learn.microsoft.com/en-us/uwp/schemas/appxpackage/uapmanifestschema/element-f-application>
- `[MS-2]` App capability declarations, microphone device capability, privacy-sensitive capabilities, restricted capability approval:
  <https://learn.microsoft.com/en-us/windows/uwp/packaging/app-capability-declarations>
- `[MS-3]` Open files and folders with a picker, picked file access, `FutureAccessList`:
  <https://learn.microsoft.com/en-us/windows/uwp/files/quickstart-using-file-and-folder-pickers>
- `[MS-4]` Window Features, message-only windows and broadcast-message limitation:
  <https://learn.microsoft.com/en-us/windows/win32/winmsg/window-features>
- `[MS-5]` `ActivateAudioInterfaceAsync`, Store-app WASAPI activation, UI-thread rule, MTA callback, consent prompt:
  <https://learn.microsoft.com/en-us/windows/win32/api/mmdeviceapi/nf-mmdeviceapi-activateaudiointerfaceasync>
- `[MS-6]` `AppCapability` class, create/check/request/access-changed surface, thread model, minimum OS, **SUA-only Create**:
  <https://learn.microsoft.com/en-us/uwp/api/windows.security.authorization.appcapabilityaccess.appcapability>
- `[MS-7]` `AppCapability.RequestAccessAsync`, UI-thread requirement:
  <https://learn.microsoft.com/en-us/uwp/api/windows.security.authorization.appcapabilityaccess.appcapability.requestaccessasync>
- `[MS-8]` `AppCapabilityAccessStatus` and `AccessChanged`:
  <https://learn.microsoft.com/en-us/uwp/api/windows.security.authorization.appcapabilityaccess.appcapabilityaccessstatus>
  <https://learn.microsoft.com/en-us/uwp/api/windows.security.authorization.appcapabilityaccess.appcapability.accesschanged>
- `[MS-9]` File access permissions guidance:
  <https://learn.microsoft.com/en-us/windows/apps/develop/files/file-access-permissions>
- `[MS-10]` `WM_QUERYENDSESSION`:
  <https://learn.microsoft.com/en-us/windows/win32/shutdown/wm-queryendsession>
- `[MS-11]` `WM_ENDSESSION`:
  <https://learn.microsoft.com/en-us/windows/win32/shutdown/wm-endsession>
- `[MS-12]` `WM_POWERBROADCAST`:
  <https://learn.microsoft.com/en-us/windows/win32/power/wm-powerbroadcast>
- `[MS-13]` `WTSRegisterSessionNotification`:
  <https://learn.microsoft.com/en-us/windows/win32/api/wtsapi32/nf-wtsapi32-wtsregistersessionnotification>
- `[MS-14]` `WM_WTSSESSION_CHANGE`:
  <https://learn.microsoft.com/en-us/windows/win32/termserv/wm-wtssession-change>
- `[MS-15]` `RegisterHotKey`:
  <https://learn.microsoft.com/en-us/windows/win32/api/winuser/nf-winuser-registerhotkey>
- `[MS-16]` Desktop interop for WinRT UI objects and `IInitializeWithWindow`, including `FileOpenPicker`:
  <https://learn.microsoft.com/en-us/windows/apps/develop/ui/display-ui-objects>
- `[MS-17]` `IAudioCaptureClient` same-thread release rule:
  <https://learn.microsoft.com/en-us/windows/win32/api/audioclient/nn-audioclient-iaudiocaptureclient>
- `[MS-18]` `DeviceInformation` thread model and watcher/enumeration surface:
  <https://learn.microsoft.com/en-us/uwp/api/windows.devices.enumeration.deviceinformation>
- `[MS-19]` `IMMDeviceEnumerator` / `EnumAudioEndpoints`:
  <https://learn.microsoft.com/en-us/windows/win32/api/mmdeviceapi/nn-mmdeviceapi-immdeviceenumerator>
  <https://learn.microsoft.com/en-us/windows/win32/api/mmdeviceapi/nf-mmdeviceapi-immdeviceenumerator-enumaudioendpoints>
- `[MS-20]` `IMMDevice::Activate`:
  <https://learn.microsoft.com/en-us/windows/win32/api/mmdeviceapi/nf-mmdeviceapi-immdevice-activate>
- `[MS-21]` `IMMDeviceEnumerator::GetDefaultAudioEndpoint`:
  <https://learn.microsoft.com/en-us/windows/win32/api/mmdeviceapi/nf-mmdeviceapi-immdeviceenumerator-getdefaultaudioendpoint>
- `[MS-22]` `IAudioClient` interface, Windows 8 STA note:
  <https://learn.microsoft.com/en-us/windows/win32/api/audioclient/nn-audioclient-iaudioclient>
- `[MS-23]` `MediaDevice.GetDefaultAudioCaptureId`:
  <https://learn.microsoft.com/en-us/uwp/api/windows.media.devices.mediadevice.getdefaultaudiocaptureid>
- `[MS-24]` `DeviceClass.AudioCapture` and `DeviceInformation.FindAllAsync(DeviceClass)`:
  <https://learn.microsoft.com/en-us/uwp/api/windows.devices.enumeration.deviceclass>
  <https://learn.microsoft.com/en-us/uwp/api/windows.devices.enumeration.deviceinformation.findallasync>
- `[MS-25]` `MediaCapture` class:
  <https://learn.microsoft.com/en-us/uwp/api/windows.media.capture.mediacapture>
- `[MS-26]` `MediaCaptureInitializationSettings`, `AudioDeviceId`, `SharingMode`, `StreamingCaptureMode`:
  <https://learn.microsoft.com/en-us/uwp/api/windows.media.capture.mediacaptureinitializationsettings>
  <https://learn.microsoft.com/en-us/uwp/api/windows.media.capture.mediacapturesharingmode>
  <https://learn.microsoft.com/en-us/uwp/api/windows.media.capture.streamingcapturemode>
- `[MS-27]` `MediaCapture.Failed` and `SystemMediaTransportControls.SoundLevel`:
  <https://learn.microsoft.com/en-us/uwp/api/windows.media.capture.mediacapture.failed>
  <https://learn.microsoft.com/en-us/uwp/api/windows.media.systemmediatransportcontrols.soundlevel>
- `[MS-28]` Media Foundation capture overview:
  <https://learn.microsoft.com/en-us/windows/win32/medfound/audio-video-capture-in-media-foundation>
- `[MS-29]` Source Reader overview and interface:
  <https://learn.microsoft.com/en-us/windows/win32/medfound/source-reader>
  <https://learn.microsoft.com/en-us/windows/win32/api/mfreadwrite/nn-mfreadwrite-imfsourcereader>
- `[MS-30]` `MFEnumDeviceSources`, default-role and endpoint-ID attributes:
  <https://learn.microsoft.com/en-us/windows/win32/api/mfidl/nf-mfidl-mfenumdevicesources>
  <https://learn.microsoft.com/en-us/windows/win32/medfound/mf-devsource-attribute-source-type-audcap-role>
  <https://learn.microsoft.com/en-us/windows/win32/medfound/mf-devsource-attribute-source-type-audcap-endpoint-id>
- `[MS-31]` C++/WinRT overview and SDK inclusion, header-only projection:
  <https://learn.microsoft.com/en-us/windows/apps/develop/cpp-winrt/>
  <https://learn.microsoft.com/en-us/windows/apps/develop/cpp-winrt/intro-to-using-cpp-with-winrt>
- `[MS-32]` `IActivateAudioInterfaceCompletionHandler`, agile requirement:
  <https://learn.microsoft.com/en-us/windows/win32/api/mmdeviceapi/nn-mmdeviceapi-iactivateaudiointerfacecompletionhandler>
- `[MS-33]` `IActivateAudioInterfaceAsyncOperation::GetActivateResult`, pre-completion error:
  <https://learn.microsoft.com/en-us/windows/win32/api/mmdeviceapi/nf-mmdeviceapi-iactivateaudiointerfaceasyncoperation-getactivateresult>
- `[MS-34]` `IAudioClient::GetService`, same-thread release rule for services and IAudioClient:
  <https://learn.microsoft.com/en-us/windows/win32/api/audioclient/nf-audioclient-iaudioclient-getservice>
- `[MS-35]` Universal CRT deployment, OS component on Windows 10+:
  <https://learn.microsoft.com/en-us/cpp/windows/universal-crt-deployment>
- `[MS-36]` Dynamic-link library search order, packaged apps:
  <https://learn.microsoft.com/en-us/windows/win32/dlls/dynamic-link-library-search-order>
- `[MS-37]` MSIX signing overview, AppxBlockMap.xml, payload integrity:
  <https://learn.microsoft.com/en-us/windows/msix/package/signing-package-overview>
  <https://learn.microsoft.com/en-us/windows/msix/overview>
- `[MS-38]` `IAudioClient::Initialize` remarks, event-driven capture since Vista SP1, AUTOCONVERTPCM flag:
  <https://learn.microsoft.com/en-us/windows/win32/api/audioclient/nf-audioclient-iaudioclient-initialize>
  <https://learn.microsoft.com/en-us/windows/win32/coreaudio/capturing-a-stream>
- `[MS-39]` C++/WinRT concurrency, `.get()` UI-thread warning, coroutine patterns:
  <https://learn.microsoft.com/en-us/windows/apps/develop/cpp-winrt/concurrency>
- `[MS-40]` `LoadPackagedLibrary`, packaged-app DLL loading, package dependency graph search:
  <https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-loadpackagedlibrary>
- `[MS-41]` `IStorageItemHandleAccess::Create`, kernel HANDLE from StorageFile:
  <https://learn.microsoft.com/en-us/windows/win32/api/windowsstoragecom/nf-windowsstoragecom-istorageitemhandleaccess-create>
- `[MS-42]` Multithreaded apartments, single-MTA-per-process rule, direct interface pointer passing:
  <https://learn.microsoft.com/en-us/windows/win32/com/multithreaded-apartments>
- `[MS-43]` `WAVEFORMATEXTENSIBLE`, SubFormat GUIDs for PCM and IEEE float:
  <https://learn.microsoft.com/en-us/windows/win32/api/mmreg/ns-mmreg-waveformatextensible>
- `[MS-44]` `WAVEFORMATEXTENSIBLE.Samples.wValidBitsPerSample`, valid-bits left-alignment rule ("the data is left-aligned within the container, and unused least-significant bits are set to zero"):
  <https://learn.microsoft.com/en-us/windows/win32/api/mmreg/ns-mmreg-waveformatextensible>
  (documented in the Remarks section of the same `WAVEFORMATEXTENSIBLE` page)
- `[MS-45]` `RoInitialize`, WinRT apartment initialization, `RO_INIT_SINGLETHREADED` / `RO_INIT_MULTITHREADED`, `S_FALSE` for already-initialized, `RPC_E_CHANGED_MODE` for incompatible mode:
  <https://learn.microsoft.com/en-us/windows/win32/api/roapi/nf-roapi-roinitialize>
- `[MS-46]` `AUDCLNT_BUFFERFLAGS`, `AUDCLNT_BUFFERFLAGS_DATA_DISCONTINUITY`, `AUDCLNT_BUFFERFLAGS_SILENT`, `AUDCLNT_BUFFERFLAGS_TIMESTAMP_ERROR`:
  <https://learn.microsoft.com/en-us/windows/win32/api/audioclient/ne-audioclient-_audclnt_bufferflags>
- `[MS-47]` WASAPI error constants (`AUDCLNT_E_DEVICE_INVALIDATED`, `AUDCLNT_E_SERVICE_NOT_RUNNING`, `AUDCLNT_E_RESOURCES_INVALIDATED`, `AUDCLNT_E_WRONG_ENDPOINT_TYPE`):
  <https://learn.microsoft.com/en-us/windows/win32/coreaudio/audclnt-error-codes>
