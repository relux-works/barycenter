# Root review round 8 — Windows AppContainer capture bridge

Date: 2026-07-12  
Task: `TASK-260712-6kba80`  
Verdict: rejected. I read all 2,520 lines of Rev 8, verified the authoritative
and outcome copies are byte-identical
(`00a0a21d659da68544e0f4235cea44658d492394cc6b1ef85eab7abaa3a84256`),
checked the live Go message pump and `x/sys/windows v0.46.0`, and re-checked the
relevant Microsoft API pages. Product source remains untouched. The revision
correctly fixes 24-bit sign extension, removes the invented HRESULT, handles
WASAPI flags, uses a real loader wrapper, balances `RoInitialize`, and closes
the picker truth table. The capture-operation state machine and permission
fallback are still internally contradictory and can deadlock or discard every
otherwise successful fallback recording.

## Blocking corrections

1. **Replace the contradictory launch/cancel/terminal prose with one executable
   CaptureStart state machine.** The note currently promises all of the
   following at once:
   - the revision header says a `readyEvent` timeout returns `E_FAIL`;
   - the detailed flow says the same timeout returns `S_OK` plus a valid
     `opId` and reports the failure asynchronously;
   - the generic ABI says initiating exports return validation/**launch**
     failures directly;
   - the synchronous `ActivateAudioInterfaceAsync` failure path both publishes
     terminal `FAILED` immediately on the UI thread and wakes the capture
     thread to run `CoUninitialize`, despite the invariant that terminal state
     is published only after `CoUninitialize`;
   - the null-handoff capture-thread branch publishes terminal on cancellation,
     while the later cancellation section says only the eventual activation
     callback may publish terminal after retrieving/releasing the returned
     interface.

   Freeze a branch table with exact function HRESULT, whether `*opId` is
   written (and its failure value), registry membership, terminal publisher,
   callback expectation, wake/event signals, and cleanup owner for: bad input,
   ID exhaustion, allocation failure, thread-creation failure, capture-thread
   `CoInitializeEx` failure, readiness timeout, synchronous activation-launch
   failure, async activation failure, cancel before callback, cancel after
   handoff, normal stop, and capture-loop failure. Once an operation ID is
   returned, a coherent design is `S_OK + opId` and all subsequent outcomes via
   query; failures before ownership is published should return directly with
   no operation. Whichever rule is selected must be used consistently by every
   initiate export.

   A timeout or synchronous launch failure must set a pending stop/failure
   cause and let the capture thread publish terminal only after its apartment
   cleanup. A cancelled activation cannot be terminal while the un-cancellable
   callback may still acquire an `IAudioClient`; terminal must wait until both
   the capture thread and late callback have reached their defined cleanup
   fence. Add deterministic tests that assert terminal is invisible at every
   pre-fence barrier.

2. **Eliminate the capture-thread last-reference/self-join deadlock.** Rev 8
   says the capture thread holds a strong session reference, releases it just
   before exiting, and the operation destructor waits for the thread-exit event
   if the thread has not exited. Go is explicitly allowed to observe terminal
   and drop the registry reference immediately. In that race the capture-thread
   reference is the last reference; releasing it runs the destructor on the
   capture thread, which waits for its own not-yet-signaled exit and deadlocks.
   The document also alternates between “release ref then signal/exit” and
   “thread-exit event then release ref.”

   Freeze a non-self-joining ownership scheme and exact final instruction
   order. For example: the thread completes COM cleanup, publishes terminal,
   performs its final session access, marks a separate live-thread fence as
   drained, then drops its reference; the destructor never joins the current
   thread, and `CapDestroy` uses the independent live-thread/callback fences.
   Because the DLL is process-lifetime loaded, no module-unload join is needed.
   Alternatively, use a separate join owner that can never destruct on the
   target thread. Add the decisive barrier: callback ref gone, terminal
   published, Go releases registry ref, capture thread holds the sole ref and
   then exits — no wait on self, no leaked handle, `CapDestroy` eventually
   succeeds.

3. **Make the permission ABI and the AppCapability-unavailable fallback
   coherent.** The numbers in `CapPermissionCheck` are a custom ABI, not the
   raw WinRT enum. Microsoft defines raw `AppCapabilityAccessStatus` as
   `DeniedBySystem=0`, `NotDeclaredByApp=1`, `DeniedByUser=2`,
   `UserPromptRequired=3`, `Allowed=4`. Rev 8 calls its different list a
   “mapping” but never freezes the translation. A direct cast would interpret
   raw `NotDeclaredByApp` (1) as the contract's `Allowed` (1), a security bug.
   Define a named `CAP_PERMISSION_*` enum and an exhaustive switch from every
   WinRT value; unknown/future values fail closed. Test every raw value,
   especially raw 1 versus raw 4.

   The fallback is also impossible as written. The note says AppCapability
   unavailable (`status=-1`) is conditionally acceptable once deterministic
   WASAPI revoke detection is proven, but the pre-promotion guard requires
   status exactly `Allowed` and explicitly deletes the draft for `-1`.
   Therefore every fallback capture is deleted and no fallback scenario can
   pass. Choose one policy: AppCapability unavailable is a no-go, or define a
   separately gated `activation-consent + proven-revoke-monitor` mode with an
   exact promotion rule and evidence prerequisite. Do not silently treat
   “unavailable” as Allowed. Reconcile the scenario matrix, final answer, and
   Go promotion algorithm with that choice.

   The `AccessChanged` ownership text must also choose one model: it first says
   `SubscriptionState` contains an AppCapability object reference, then says it
   holds only a raw pointer/weak reference. Freeze the strong/weak/global owner,
   weak-resolution failure behavior, and the mutex/ref fences that keep
   `CheckAccess` from racing `CapDestroy`.

4. **Use truthful WASAPI reasons and fail-closed promotion until hardware
   disambiguates privacy revoke.** The corrected constants are real, but the
   mapping still labels `AUDCLNT_E_SERVICE_NOT_RUNNING` and
   `AUDCLNT_E_RESOURCES_INVALIDATED` as `DEVICE_LOST`. Microsoft documents
   `RESOURCES_INVALIDATED` as including a suspended stream; a stopped audio
   service is likewise not a removed device. Either add truthful reasons or
   classify these as generic `WASAPI_ERROR`. Apply the table to errors from
   `GetNextPacketSize`, `GetBuffer`, `ReleaseBuffer`, `Start`, and `Stop`, not
   only the first two calls.

   In AppCapability-unavailable mode, no WASAPI failure other than a
   hardware-proven privacy mapping may be assumed safe for promotion. The note
   correctly says the privacy HRESULT is still a mandatory discovery, but it
   simultaneously promotes `DEVICE_LOST`/service/resource-invalidated drafts.
   Until the signed Win10/Win11 revoke gate proves non-overlap, those exits must
   be non-promotable in fallback mode. Also freeze the stop-reason
   linearization: the capture thread must reload/commit the final priority-CAS
   value immediately before publishing terminal, so a higher-priority revoke
   cannot win the CAS yet leave a stale user-stop reason in the result.

5. **Integrate helper events with the real Win32 message pump without blocking
   the pinned UI thread.** Live `pulsar-win` blocks its pinned main thread in
   `GetMessageW`. Rev 8 repeatedly says Go waits in `WaitForMultipleObjects`
   but never defines how that coexists with window messages, picker modality,
   `WM_HOTKEY`, or lifecycle delivery. Two independent blocking loops cannot
   own the same UI thread. Freeze one legal integration:
   - a dedicated waiter goroutine/OS thread waits for helper events, drains
     thread-safe result APIs, and uses `PostMessage` for UI work; or
   - the UI thread uses `MsgWaitForMultipleObjectsEx` and drains both handles
     and the Win32 queue without starving either.

   Keep `CapInit`, `CapDestroy`, `CaptureStart`, permission request, and picker
   initiation on the exact initialized UI thread where required. Define event
   handle lifetime and shutdown of the waiter. Add a coalesced-signal test while
   picker/UI messages and `WM_ENDSESSION` are queued, proving messages continue
   to dispatch and every operation is drained.

6. **Finish packet/ring atomicity and sizing.** Whole-packet capacity preflight
   is correct, but a single atomic capacity read does not itself guarantee that
   a conversion error cannot publish a partial packet. Convert into scratch
   storage or reserve the ring and publish the producer index only after the
   entire packet converts successfully; on conversion/ReleaseBuffer failure,
   freeze whether zero packet frames become visible. Test the consumer racing a
   conversion failure, not only a successful write.

   The claim that a two-second ring is always larger than the maximum WASAPI
   packet is not established by the frozen bounds: `GetBufferSize` is accepted
   up to 65,536 frames, while `2 * sampleRate` can be smaller. After
   `GetBufferSize`, either allocate at least
   `max(2 * sampleRate, bufferFrames)` (with checked bytes) or fail activation
   before `Start` if one complete packet cannot fit. Add a low-rate/large-buffer
   fixture. Overflow during ordinary operation remains terminal as already
   specified.

7. **Close global init and loader test semantics.** `RoInitialize` handling is
   now factually correct, but the helper state machine does not say what a
   second `CapInit` before destroy does, how a wrong-thread `CapDestroy` fails,
   whether a failed init leaves any state, or how idempotent destroy avoids a
   second `RoUninitialize`. Store the initializing thread ID and the exact
   successful-call balance; test `S_OK`, `S_FALSE`, changed mode, repeated init,
   wrong-thread destroy, double destroy, and re-init.

   Use `windows.NewLazySystemDLL("kernel32.dll")` for the new loader binding as
   required by R7 (the local x/sys happens to special-case kernel32 even through
   `NewLazyDLL`, but the contract should not depend on that hidden exception).
   The shown `*windows.LazyProc` cannot have its `.Call` method replaced by the
   claimed unit test; freeze an injectable function/wrapper seam and test exact
   `APPMODEL_ERROR_NO_PACKAGE` fallback, all other errors, absolute path, flags,
   and process-lifetime no-unload behavior.

## Resubmission

- Amend and reattach one byte-identical authoritative outcome; keep product
  source untouched and return to `to-review`, never `done`.
- Preserve the accepted AppContainer posture, real HRESULT constants, safe
  24-bit conversion, packet flag handling, picker truth table, conditional WAV
  gate, process-lifetime DLL, and same-thread COM release rules.
- Replace prose summaries with executable branch tables and deterministic
  barriers for launch/timeout/cancel, last-ref thread exit, permission mapping
  and fallback, WASAPI promotion, UI-message/event integration, packet commit,
  init balance, and loader selection.
