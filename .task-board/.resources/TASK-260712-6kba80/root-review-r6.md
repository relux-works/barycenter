# Root review round 6 — Windows AppContainer bridge

Date: 2026-07-12  
Task: `TASK-260712-6kba80`  
Verdict: rejected. I read all 1,985 lines of Rev 6, verified the two copies are
byte-identical (`99f757637155a334cfaf24ccfc1ff1c7a16ec84b61cc6aadf9cae1b5a12d1f8a`),
checked the live `pulsar-win` parser/manifest/window seams, and confirmed product
source is untouched. Rev 6 fixes the R5 numeric/error/probe-boundary findings,
but the lifetime fence it claims is not implementable as written and several
ABI contracts still contradict themselves.

## Blocking corrections

1. **Make callback completion and DLL lifetime real, not a refcount observed
   before the callback has actually left DLL code.**
   The revision header says callback return precedes callback-ref release, while
   the normative sequence releases the ref at step 5 and returns at step 6.
   A callback cannot perform an action after it returns. More importantly, a
   state `shared_ptr` is not a module-lifetime fence: the global count can reach
   zero after `SetEvent` and immediately before the callback epilogue, and the
   system can release the activation handler / async-operation COM references
   after `ActivateCompleted` returns. Another thread may then let `CapDestroy`
   succeed and call `FreeLibrary` while callback or COM `Release` code from the
   DLL is still executing. Freeze one safe contract. The recommended probe
   contract is to load the helper once and **never call `FreeLibrary` during the
   process lifetime**; `CapDestroy` tears down application state only, and the
   loader/module is reclaimed at process exit. Otherwise specify a proven
   module-pin design that covers callback entry through the system's final COM
   release, not merely operation-state lifetime. Also enumerate ownership and
   release of `IActivateAudioInterfaceAsyncOperation`, the completion-handler
   object, and every C++/WinRT async operation/delegate, including synchronous
   launch failure and cycle breaking. Add a deterministic barrier test that
   pauses a callback after publication/signal and before its epilogue; a
   zero-delay stress loop alone does not satisfy R5's barrier requirement.

2. **Give `AccessChanged` a close-safe unsubscribe fence.**
   Microsoft explicitly warns that an asynchronous WinRT event may reach its
   recipient after revocation has begun. “Acquire a strong ref at handler entry”
   does not protect the dispatch-to-entry interval. The registered delegate
   itself must own safe subscription state (with its source/delegate cycle
   explicitly broken), and a copied in-flight delegate must keep that state
   alive. There is a second handle race: the current ownership rule lets Go
   close `notifyEvent` after `CapPermissionUnsubscribe` returns, while a handler
   that was already in flight may still call `SetEvent` on that closed or reused
   handle. Choose an exact solution: duplicate/own the notification handle until
   the last delegate invocation exits, or make unsubscribe an async
   initiate/query/release operation whose completed state guarantees that no
   handler can signal again. Freeze return values/idempotence and add a test
   barrier at handler dispatch, unsubscribe and Go-handle close/reuse.

3. **Stop overloading the picker operation HRESULT with transfer state.**
   The revision summary promises that `*hresult` remains the picker operation
   outcome and `*handleTaken` reports transfer state. The normative repeated-take
   branch instead overwrites `*hresult` with `E_HANDLE` while returning `S_OK`,
   then calls that namespace “distinct.” It is not distinct. Keep the operation
   outcome immutable and add an explicit transfer-state value, return a
   documented call-level status, or make repeated take a fully specified
   idempotent no-transfer result. Provide one truth table for pending, picked
   before take, picked after take, cancelled, failed, invalid `takeHandle`, null
   output pointers, unknown/released ID and release-before-terminal. Every row
   must state function HRESULT, operation HRESULT, transfer state, written
   outputs and handle owner.

4. **Close the activation handoff failure paths and COM ownership graph.**
   The capture thread is required to call `CoInitializeEx` before touching the
   handed-off `IAudioClient`, but if that call fails after the callback has put
   the pointer in the slot, no thread is specified to release the pointer. Start
   and prove the MTA capture thread ready before launching activation, or retain
   a callback-side owner that releases the interface if the thread cannot
   accept it. Define the linearization point for successful handoff, who owns
   each AddRef before/after it, how synchronous activation failure is published,
   and when the capture thread has truly exited. The lifetime table must match
   the actual reference holders. Add injected `CoInitializeEx` failure and
   activation-launch failure tests with exact release counts.

5. **Make teardown and operation-registry emptiness unambiguous.**
   `CapDestroy` alternates between “terminal/released,” “zero active
   operations,” and instructions requiring every `*Release`. A terminal but
   unreleased operation still occupies the registry and owns its result/event
   contract. Freeze success to require an **empty operation registry**, a fully
   completed permission-unsubscribe fence, no capture thread, and no remaining
   application callback/delegate state; then state that the module remains
   loaded if correction 1 uses the recommended process-lifetime loader. Correct
   the operation-ID exhaustion text: ID allocation fails the initiating export,
   not `CapInit`. Test terminal-but-unreleased, callback-held-after-release,
   active-subscription, unsubscribe-in-flight, repeated destroy and re-init.

6. **Freeze stop-reason arbitration so a benign stop cannot promote media after
   permission loss or integrity failure.**
   `CaptureRequestStop` is currently a no-op once stopping begins, so a user-stop
   racing `AccessChanged` can win the reason and finalize a file even though
   permission was revoked before promotion. Define an atomic priority/merge
   policy: overflow and permission revoke must dominate finalizable reasons, and
   Go must recheck the final terminal reason (and permission when available)
   immediately before rename. Specify how fallback WASAPI HRESULTs distinguish
   permission loss from device loss; unknown privacy-related failure must fail
   closed for promotion. Add barrier tests for both orderings of user-stop vs
   revoke, user-stop vs overflow, and device-loss vs revoke.

7. **Remove the remaining C++17 undefined/implementation-defined PCM operations
   and turn WAV interoperability into an actual gate.**
   The proposed 24-in-32 code dereferences `uint32_t*` from an arbitrary byte
   pointer (possible unaligned/aliasing UB) and casts values such as
   `0xFFFFFFFFu` to `int32_t`; the latter is implementation-defined in C++17,
   despite the note claiming defined behavior on every conforming compiler.
   Assemble bytes or use `memcpy`, and compute negative 24-bit values with
   range-safe arithmetic rather than out-of-range unsigned-to-signed casts.
   Apply the same rule to PCM16/PCM32 reads. Add deliberately unaligned vectors
   and run them under sanitizers where available. Most power-of-two conversion
   vectors here have an exact expected float32 bit pattern; do not use a loose
   tolerance that can mask an extraction error.

   R5 also required proof for the retained 44-byte IEEE-float header and
   multichannel behavior. The live `parseWAV` accepts that shape, but that proves
   only the local parser. Add explicit mono, stereo and >2-channel synthetic
   files to the independent-decoder gate and record expected channel/rate/frame
   metadata. If the selected independent decoder requires a `fact` chunk or
   `WAVEFORMATEXTENSIBLE`, change the writer and recovery offsets instead of
   continuing to assert broad interoperability. Keep these files disposable
   probe evidence, never production drafts.

## Resubmission

- Amend and reattach the single authoritative decision note byte-identically;
  product source remains untouched.
- Preserve the accepted AppContainer posture, WASAPI call/packet order, numeric
  bounds, standard overflow HRESULT, readiness-hint drain, visible picker owner,
  probe-vs-production split and signed-hardware no-go gates.
- Prefer a process-lifetime-loaded helper for the probe; it is simpler and safer
  than trying to prove unloadability across OS-owned COM callbacks.
- Include deterministic callback/unsubscribe/stop-reason barriers, not only
  probabilistic stress loops.
- Return to `to-review`, never `done`.
