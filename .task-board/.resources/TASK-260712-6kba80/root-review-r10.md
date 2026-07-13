# Root review round 10 — Windows AppContainer capture bridge

Date: 2026-07-12  
Task: `TASK-260712-6kba80`  
Verdict: rejected. I read all 2,809 lines of Rev 10 and verified the
authoritative and outcome copies are byte-identical (2,809 lines, 33,698
words, 259,437 bytes, SHA-256
`8dd6eb3d395cf2b0fc058ffcb56371674338cf7d4fae53cdd4c37dfea5c5be3d`).
Product source remains untouched. Rev 10 improves the main packet loop, init
rollback, and intended waiter ownership, but the old pre-cleanup publishers
still exist in normative sections, the reason seal is still racy, and graceful
quit calls the same-thread teardown API from the wrong thread before capture is
terminal.

## Blocking corrections

1. **Remove every pre-cleanup terminal publisher, not only the branch-table
   cells.** The revised branch table says the capture thread publishes after
   `CoUninitialize`, but the same authoritative note still says:
   - null handoff “set terminal state, signal notifyEvent, exit” before the
     cleanup sequence (lines 381–383);
   - `readyEvent` timeout makes the UI thread set terminal and signal the event
     (lines 455–457);
   - synchronous launch failure “transitions to FAILED immediately” (line 635);
   - generic initiate HRESULT reports launch errors directly (line 675), while
     the branch table routes post-publication launch errors through the op;
   - the two-phase contract transitions in the capture thread/callback without
     the composite `threadDone` barrier (lines 1001–1008 and 1032–1037);
   - acquired-packet error cleanup publishes terminal without the documented
     `threadDone` ordering (line 798).

   These are mutually exclusive implementations. Derive the diagram, branch
   table, cancellation prose, two-phase lifetime, generic async contract,
   tests, and final summary from one executable state-transition table. UI and
   callback paths may store a pending cause only. For timeout, sync activation
   failure, async activation failure, null handoff, normal stop, and capture
   failure, `CaptureGetResult` must remain nonterminal at barriers before
   COM/service release and before `CoUninitialize` completes.

2. **Define a truthful final-access lifetime and break the normal activation
   cycle at callback completion.** Rev 10 repeatedly calls `threadDone=1` the
   capture thread's “absolute last session-state access,” then writes the
   session's terminal atomic after it. Both cannot be true. The ordering can be
   made safe only if the marker is explicitly defined as “cleanup complete;
   exactly one final terminal store remains,” the terminal store is the final
   object access, `CaptureGetResult` uses an acquire read, and the registry
   cannot be dropped until that store is fully observable. Otherwise use a
   separate thread context/completion cell whose lifetime does not depend on
   the session. Rewrite the destructor and barrier proof from the chosen model;
   do not invoke `CapDestroy` as protection for a destructor triggered by
   `CaptureRelease`.

   The C++/WinRT cycle text is also wrong for successful activation. It says the
   callback clears its stored async-operation reference after the callback
   publishes terminal, but the normal callback never publishes terminal — it
   hands off and returns while the capture thread may run for minutes. Freeze
   exact ownership of both `IActivateAudioInterfaceAsyncOperation` and the
   completion handler, and clear/release the state-held async operation after
   `GetActivateResult` and handoff (or another proven callback-completion point)
   on every success/failure/cancel branch. Assert exact ref counts and an empty
   registry after release; no cycle may rely on a destructor that the cycle
   itself prevents.

3. **Make every packet description use release-before-commit and classify
   release failures.** The detailed loop at lines 766–786 is corrected, but the
   handoff diagram still writes zeros/converts into the ring before
   `ReleaseBuffer` (lines 410–412), and §Buffer handling still says “appends to
   the recording ring, calls ReleaseBuffer” (lines 1236–1238). An implementer
   following either section reintroduces the R9-3 invalid-data visibility bug.

   Keep exactly one normative algorithm: preflight → convert/fill scratch →
   `ReleaseBuffer` → commit producer index. Specify what happens when the
   cleanup `ReleaseBuffer` itself fails in the overflow, discontinuity, format
   error, and stop-while-acquired branches; those calls are currently ignored
   while another terminal reason/HRESULT is published. Freeze final reason,
   HRESULT, `Stop` eligibility, and zero-visibility behavior. Run deterministic
   consumer barriers for silent/non-silent normal packets and every early-exit
   branch.

4. **The claimed atomic reason seal still allows a post-snapshot CAS.** Section
   2054–2110 keeps `CaptureRequestStop` lock-free on the reason while only the
   capture thread takes the mutex for sealing. The mutex cannot exclude a CAS
   that does not take it. Deterministic counterexample:
   1. request thread reads state `STOPPING` and pauses;
   2. capture thread locks, snapshots `USER_STOP`, writes the sealed reason,
      flips state, and unlocks;
   3. request thread resumes and successfully CASes the separate reason atomic
      to higher-priority `PERMISSION_REVOKE`.
   The sealed result remains stale.

   Put state/sealed-bit/reason in one packed CAS state, or make every reason
   update and the final seal participate in the same mutex. Add barriers before
   and after the linearization point. Also remove the unexplained two-terminal
   model: §2110 flips state to `TERMINAL` under the mutex, then later “publishes
   terminal” after `threadDone`. Define a private sealed state distinct from the
   one and only public terminal value, or use a single visibility mechanism;
   `CaptureGetResult` must never observe the private pre-cleanup state.

5. **Graceful quit still violates both thread affinity and two-phase release.**
   The waiter diagram and prose make the waiter call `CaptureRelease` and
   `CapDestroy` during `GracefulQuit` (lines 871–873, 906–908, 2613). The ABI
   later requires `CapDestroy` on the exact UI thread that called `CapInit`, with
   wrong-thread calls returning `RPC_E_WRONG_THREAD` (lines 1732–1734). The
   waiter also performs one final drain and immediately releases even though
   `CaptureRequestStop` is nonblocking; a still-pending operation makes
   `CaptureRelease` and then `CapDestroy` fail. The command-ordering test does
   not wait for terminal.

   Freeze an asynchronous quit state machine: UI keeps pumping messages; waiter
   requests stop, continues wait/drain until terminal, finalizes, releases every
   operation, explicitly unsubscribes and waits for callback refs; waiter posts
   a stable cleanup-ready ID; UI thread calls `CapDestroy`; only after success
   does UI post `WM_QUIT` and join/close waiter events. Do not block the UI
   message pump while completion depends on posted messages. Update live-shell
   integration: current tray `OnQuit` immediately posts `WM_QUIT`, so this must
   become the start of the state machine, not completion.

   Freeze the abrupt path separately and consistently. The signal matrix calls
   `CaptureRequestStop` from `WM_QUERYENDSESSION`/`WM_ENDSESSION`, while the
   waiter diagram only signals `shutdownEvent` and exits without requesting
   stop. State exactly which wndproc performs the nonblocking stop and when the
   shutdown event is set. The waiter result must carry the picker HANDLE/take
   state, not the stale `PickerDone{path,...}` shown in the queue diagram.

6. **Publish one complete HRESULT/cleanup table for every COM/WASAPI stage.**
   The normative mapping at lines 2106–2108 covers only
   `GetNextPacketSize`, `GetBuffer`, `ReleaseBuffer`, `Start`, and `Stop`, while
   the final answer claims the scope includes `Initialize` and `GetService`, and
   the rejection summary itself says those calls need classification. There is
   still no frozen outcome for `GetMixFormat`, format validation,
   `Initialize`, `GetBufferSize`, `SetEventHandle`, or `GetService` failures.

   One table must specify for each call: returned terminal state/reason/HRESULT,
   whether format is valid, whether any PCM exists, promotability, whether
   `Stop` is legal (only after successful `Start`), exact release/free order
   including `CoTaskMemFree` for the mix format, and final publisher. Include
   activation `E_ACCESSDENIED` and every known service/resource invalidation,
   with unknowns fail-closed. Generate the branch tests and final summary from
   that table.

## Resubmission

- Amend one authoritative note, attach a byte-identical outcome, keep product
  source untouched, and return to `to-review`, never `done`.
- Preserve the named permission enum, fallback gate, dynamic ring sizing,
  conversion vectors, picker truth table, loader wrapper, init rollback,
  process-lifetime module, conditional WAV gate, and hardware-proof matrix.
- Prove terminal/cleanup/lifetime/reason transitions with deterministic
  barriers; prove graceful and abrupt shutdown against the live `GetMessageW`
  shell; prove every release-before-commit and HRESULT branch. Summary claims
  are not substitutes for the normative tables.
