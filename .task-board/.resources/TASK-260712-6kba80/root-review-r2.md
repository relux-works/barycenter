# Root review round 2 — Windows AppContainer bridge

Date: 2026-07-12  
Task: `TASK-260712-6kba80`  
Verdict: changes still required. The platform choice is credible, but the rev-2
ABI cannot be implemented safely as written.

## Accepted from rev 2

- Manifest posture and microphone-only capability delta.
- Visible top-level picker owner; hidden top-level lifecycle/hotkey window.
- Shared, event-driven WASAPI capture with `AudioDeviceRole::Default`.
- Documented-vs-probe distinction for AppContainer behavior.
- Native x64 C++/WinRT helper, `/MT`, package signing, no COM pointer crossing
  into Go.
- A raw `IAudioClient*` may move between the system callback MTA thread and the
  helper MTA capture thread only because all free-threaded threads in a process
  share one MTA. Add the direct Microsoft MTA citation and keep exclusive
  ownership plus exact AddRef/Release transfer.

## Blocking corrections

1. **Redesign the ABI as asynchronous.**
   `RequestAccessAsync`, `FindAllAsync`, `FileOpenPicker`, and audio activation
   cannot be wrapped as synchronous UI-thread exports that fill result buffers.
   C++/WinRT explicitly warns not to block with `.get()` on a UI thread. Freeze
   an initiate → event/message → query/take-result contract with request IDs and
   operation states. No native callback may jump into Go. `CaptureStart` may
   return an operation/session handle immediately, but negotiated format and
   activation HRESULT become available only in its completion event/query.
   - https://learn.microsoft.com/en-us/windows/apps/develop/cpp-winrt/concurrency

2. **Make session lifetime internally consistent.**
   Rev 2 says `CaptureDestroy` is nonblocking, releases the COM objects/thread,
   invalidates the handle on return, and is safe while an un-cancellable
   activation callback still owns state. Those guarantees conflict. Freeze a
   two-phase lifetime: idempotent nonblocking `RequestStop(reason)`, async
   terminal `stopped/failed` notification, then `Release` only after terminal
   state (or an explicitly ref-counted self-owned tombstone until late callback
   completion). Define who owns every reference in start/cancel/late-callback,
   and ensure UI/lifecycle handlers never join. C++ exceptions must be caught at
   every export and converted to HRESULT; none cross the ABI.

3. **Freeze the actual sample representation.**
   `GetMixFormat()` may return PCM integer or IEEE float via
   `WAVEFORMATEXTENSIBLE`; `CaptureRead(float*)` cannot simply copy arbitrary
   WASAPI bytes. Choose one contract. Recommended: helper converts supported
   native formats to interleaved float32 at the native rate/channel count and
   reports a versioned format struct after activation; Go then performs
   channel/rate conversion. Include valid bits, channel mask, silent-buffer
   handling, unsupported subtype error, buffer-overflow policy, and evidence.
   Do not return `bitsPerSample` synchronously from `CaptureStart`.

4. **Prove readable picker content, not a path.**
   A brokered/provider-backed `StorageFile` is not safely represented by
   `StorageFile.Path` alone and may be virtual. Use either WinRT stream reads in
   the helper or `IStorageItemHandleAccess::Create`, then hand Go a same-process
   read handle with exact close ownership. Cover cancel, zero-byte/read error,
   cloud hydration/provider failure, and enforce file limits while reading. The
   scenario passes only after reading and hashing bytes without broad
   capability.
   - https://learn.microsoft.com/en-us/windows/win32/api/windowsstoragecom/nf-windowsstoragecom-istorageitemhandleaccess-create

5. **Use one safe packaged DLL loading path.**
   Bare `windows.NewLazyDLL("pulsar-capture.dll")` is still ambient name search
   and contradicts the review requirement. In the packaged probe use
   `LoadPackagedLibrary(L"pulsar-capture.dll", 0)` (or a single equivalently
   package-bound documented API). If unpackaged unit probes need a fallback,
   use a separately explicit absolute executable-directory path. Never present
   unsafe and safe loaders as interchangeable choices.
   - https://learn.microsoft.com/en-us/windows/win32/api/winbase/nf-winbase-loadpackagedlibrary

6. **Do not cast HRESULT to `syscall.Errno`.**
   Preserve the signed 32-bit HRESULT/hex value, decode `HRESULT_FROM_WIN32`
   only where applicable, and use a dedicated Go error type/FormatMessage
   strategy. `syscall.Errno` is a Win32 error-code namespace, not a generic
   HRESULT namespace. Evidence logs must retain both HRESULT and any separately
   captured `GetLastError`.

7. **Version and disambiguate the ABI.**
   Add an ABI version query and `size/version` fields to structs. Distinguish OS
   kernel `HANDLE`s (events, picked file) from opaque helper session pointers or
   IDs. Specify x64 calling convention (where `__stdcall` is accepted but not a
   distinct convention), null/size validation, thread safety, maximum device
   counts/string sizes, request cancellation, and one-vs-many operation limits.

8. **Tighten the MTA proof.**
   Cite Microsoft’s multithreaded-apartment rule directly, require successful
   `CoInitializeEx(..., COINIT_MULTITHREADED)` on the capture thread, and make
   the callback's returned reference transfer explicit. If either endpoint is
   not demonstrably in the same MTA, use COM marshaling instead of a mutex-only
   pointer handoff.
   - https://learn.microsoft.com/en-us/windows/win32/com/multithreaded-apartments

9. **Required P1.0 behavior cannot silently degrade to a non-equivalent UI.**
   If `RegisterHotKey`, session-lock notification, suspend notification, or
   deterministic permission-revoke detection fails under signed AppContainer,
   tray-only/manual fallback does not satisfy spec §19.2. Record the probe as
   blocked/no-go and select another legal mechanism in a separate decision.
   `WM_QUERYENDSESSION` is not a substitute for suspend, and
   `SM_REMOTESESSION` is not a substitute for lock notification.

10. **Make interrupted-draft handling crash-safe.**
    A window procedure can only signal stop and return; it cannot assume an
    asynchronous WAV finalization finishes before shutdown/suspend kills the
    process. Freeze app-private `.partial` streaming, atomic promotion to a
    durable draft only on confirmed finalization, and startup cleanup/recovery.
    User Stop may finalize; permission revoke/cancel discards; interrupted
    shutdown/suspend/lock never reports a pass merely because stop was queued.

11. **Leave one byte-identical authoritative outcome.**
    Amend the same research note, reattach it as the single outcome, keep source
    code untouched, and return to `to-review`, never `done`.
