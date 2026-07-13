# Root review round 7 — Windows AppContainer bridge

Date: 2026-07-12  
Task: `TASK-260712-6kba80`  
Verdict: rejected. I read all 2,283 lines of Rev 7, verified the authoritative
and outcome copies are byte-identical
(`da300431beed4f6bc477acf5f8fed8bd151ccd4d53b81a1ee7648accc8a8c3b6`),
checked the proposed loader against the repository's actual
`golang.org/x/sys/windows v0.46.0`, and cross-checked WASAPI flags/HRESULTs and
WinRT apartment rules against Microsoft documentation. The process-lifetime
module and duplicated-event decisions are sound, but the revision still
contains a broken signed conversion, a false HRESULT, and contradictory launch,
thread-exit, and ABI contracts.

## Blocking corrections

1. **Fix the still-wrong “range-safe” 24-bit conversion.**
   In `uint32_t u`, the expression `u - 0x1000000u` is unsigned arithmetic. For
   `u >= 0x800000` it wraps to `0xFF800000..0xFFFFFFFF`; the subsequent cast to
   `int32_t` is exactly the implementation-defined out-of-range cast the change
   claims to remove. The prose saying the subtraction produces a small positive
   range mapped to negatives is false. Use only representable signed operands,
   for example:
   `int32_t val = int32_t(u); if (u >= 0x800000u) val -= 0x1000000;`
   (`u <= 0xFFFFFF`, so the first cast is safe), or equivalent checked
   arithmetic. Apply it to packed-24 and 24-in-32. Make every listed vector,
   including all negative values and deliberately unaligned buffers, bit-exact
   against a scalar reference; sanitizers do not detect implementation-defined
   numeric mis-conversion by themselves.

2. **Replace the invented/misidentified WASAPI HRESULT table with verified
   constants and truthful terminal reasons.**
   `AUDCLNT_E_NOT_ALLOWED` is not a documented WASAPI constant here, and
   `0x88890003` is `AUDCLNT_E_WRONG_ENDPOINT_TYPE`, not a privacy-revocation
   signal. Misclassifying it as permission loss destroys evidence for the wrong
   reason. Keep only SDK/officially documented names and values. A hardware
   probe may discover the actual revoke HRESULT; until then, unknown audio
   failures should use non-promotable `CAP_REASON_WASAPI_ERROR`, not be relabeled
   as permission revoke. Define `AUDCLNT_E_RESOURCES_INVALIDATED`, service-stop,
   format/init errors, and the existing `CAP_REASON_WASAPI_ERROR`/
   `CAP_REASON_FORMAT_ERROR` in the priority/promotion matrix. The pre-promotion
   permission check must promote only on `Allowed`; explicitly handle
   `UserPromptRequired`, `NotDeclaredByApp`, unavailable, and check failure.

3. **Handle WASAPI discontinuity and preflight an entire packet before touching
   the lossless ring.**
   The loop handles only `SILENT`. Microsoft documents
   `AUDCLNT_BUFFERFLAGS_DATA_DISCONTINUITY` as a stream transition/timing glitch;
   accepting it while treating an app-ring drop as fatal is inconsistent. Freeze
   first-packet semantics and make any subsequent discontinuity a distinct
   non-promotable integrity failure (or reconstruct a proven gap from device
   positions; the minimal probe should fail). Record `TIMESTAMP_ERROR` policy as
   well. Add a terminal reason/evidence row and synthetic flag tests.

   Before conversion/copy, atomically verify the recording ring has room for
   **the whole packet**. “Copy, then check whether the ring was full” can overrun
   or leave a partial packet. If capacity is insufficient, write zero frames,
   release the acquired WASAPI packet, then transition to overflow failure. Test
   exact-fit, one-frame-short, concurrent consumer, silent packet, discontinuity,
   conversion error, and stop while acquired.

4. **Make `CaptureStart`, cancellation, terminal publication, and COM cleanup one
   coherent state machine.**
   The revision simultaneously says initiate exports return launch errors,
   `CaptureStart` returns an error on `CoInitializeEx` failure, and synchronous
   `ActivateAudioInterfaceAsync` failure is returned later through a valid op ID.
   It also publishes terminal state before `CoUninitialize`/thread exit while
   elsewhere asserting terminal means the capture thread has exited. Freeze one
   exact contract for thread-create, MTA-ready, activation-launch, callback,
   cancel, and stop failures: function HRESULT, whether `opId` is written,
   registry ownership, notification behavior, and required release.

   The UI wait for `readyEvent` needs a real finite timeout and cleanup; calling
   `CoInitializeEx` “usually takes microseconds” is not a bound. Cancellation
   before activation completion must also wake and terminate the already-created
   capture thread. The capture thread must hold an explicit strong session ref
   until after `CoUninitialize` and its exit fence; either publish terminal only
   after all thread access is finished or make release/query semantics observe
   that fence. Correct the still-stale operation-ID sentence that says
   exhaustion fails `CapInit`.

   Finally, enumerate actual release/cycle breaking: synchronous activation
   failure still owns the helper-created handler/session/thread; a C++/WinRT
   async operation → delegate → state → async-operation cycle is not broken by a
   destructor that cannot run because of that cycle. Clear/release the held
   operation/delegate at a specified callback point. Add deterministic barriers
   for every launch/cancel/terminal ordering and exact live-object/thread counts.

5. **Use a loader that exists in the checked-in Go dependency and freeze the UI
   WinRT apartment.**
   `windows.LoadPackagedLibrary` does not exist in the repository's
   `x/sys/windows v0.46.0`; the shown code will not compile. Define the real
   package loader, for example a small typed wrapper around
   `LoadPackagedLibrary` resolved from `windows.NewLazySystemDLL("kernel32.dll")`,
   converting its returned `HMODULE` into `windows.DLL` for `FindProc`, with
   exact zero-handle/last-error handling and `APPMODEL_ERROR_NO_PACKAGE` fallback.
   Keep the accepted process-lifetime no-`FreeLibrary` policy. Unit-test loader
   selection with injected calls and cross-compile the actual wrapper.

   `CapInit` also needs an explicit UI-thread WinRT apartment contract. All
   WinRT-using threads must be initialized. Freeze `RoInitialize(RO_INIT_SINGLETHREADED)`
   (or exact C++/WinRT equivalent), accept `S_OK`/`S_FALSE`, reject
   `RPC_E_CHANGED_MODE`, balance every successful call—including `S_FALSE`—with
   same-thread `RoUninitialize`, and define repeated `CapInit`/`CapDestroy`.

6. **Complete the picker pointer/error truth table instead of calling a partial
   table complete.**
   The table covers null `fileHandle` only for `takeHandle=1`, but not null
   `state`, `hresult`, `handleTaken`, `fileSize`, `fileHandle` during probe,
   inconsistent `nameBuf`/`nameBufLen`, negative lengths, or null `opId` on
   initiation. Mark every pointer mandatory or optional and state validation
   order, function HRESULT, outputs written, and whether the take occurred. A
   caller error must never transfer/close the picked handle. Add table-driven
   ABI tests for every null/negative combination and repeat call.

7. **Keep probe evidence claims aligned with gates.**
   The note still says a 44-byte float WAV is valid/widely interoperable before
   calling that fact a hypothesis. Make the pre-gate wording conditional. If the
   gate selects `fact`/extensible headers, the writer, finalizer, startup scanner,
   offsets, parser tests, and process-kill recovery must all switch together;
   no artifact may be promoted using the old 44-byte assumptions. Record one
   selected header shape as the packaged probe's build-time contract before the
   signed hardware scenarios run.

## Resubmission

- Amend and reattach one byte-identical authoritative outcome; product source
  remains untouched.
- Preserve AppContainer posture, process-lifetime module loading, duplicated
  subscription handle, visible picker owner, exact WASAPI packet-drain order,
  standard overflow HRESULT, probe-vs-production boundary, and hardware no-go
  gates.
- Add the corrected signed arithmetic, verified HRESULT/flag policy, whole-
  packet ring preflight, coherent launch/thread-exit state machine, real Go
  loader, UI apartment balance, complete picker ABI negatives, and named tests.
- Return to `to-review`, never `done`.
