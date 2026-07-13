# Root review round 11 — Windows AppContainer capture bridge

Date: 2026-07-12  
Task: `TASK-260712-6kba80`  
Verdict: rejected. I read all 2,972 lines of Rev 11 and verified the
authoritative and outcome copies are byte-identical (2,972 lines, 37,758 words,
291,063 bytes, SHA-256
`af8ea8828babbe494c4746a15f78fd82fb90583b5d35870943d210df4a1de5fa`).
Product source remains untouched. Rev 11 materially improves the intended
reason seal, packet commit ordering, and UI-thread `CapDestroy` ownership, but
the executable contract still contradicts those repairs. It also leaves an
unbounded graceful-quit path and a privacy-unsafe orphan-draft recovery path.

## Blocking corrections

1. **Remove the remaining pre-barrier/final-access contradictions.** The
   handoff diagram at lines 315–324 still calls `threadDone=1` the final
   session-state access, then publishes terminal after it. This is the exact
   contradiction R10 rejected. Later text correctly defines `threadDone` as
   “cleanup complete; one terminal store remains,” so every diagram and test
   must use that definition.

   The standalone overflow sequence at lines 1347–1355 is also still a second
   normative algorithm: `ReleaseBuffer` → `Stop` → releases →
   `CoUninitialize` → directly set `FAILED` → signal. It omits the packed
   reason seal, `threadDone`, the fence, the final terminal store, and the
   thread-local notification handle. The branch table at lines 508–510 likewise
   ends several cleanup descriptions at “sets threadDone, exits” without the
   required final store.

   Generate the diagram, branch table, overflow/error prose, two-phase lifetime,
   tests, and final summary from one transition table. There must be no route
   that says `threadDone` is final, and no terminal publisher that omits:
   seal/pending-cause resolution → legal COM cleanup → `CoUninitialize` (when
   initialized) → `threadDone=1` → fence → one final terminal store → local
   notification. Add a static consistency test/grep fixture for the obsolete
   sequences; do not rely on a later “all rows” assertion to override them.

2. **Finish the packed state machine for activation cancellation and internal
   capture failures.** The packed layout at lines 2157–2165 contains state,
   sealed, and reason fields, but no `cancelled` bit. Multiple normative paths
   nevertheless read or write “the cancelled bit via the packed CAS.” Either
   assign an exact bit and transitions or use `state=STOPPING` +
   `reason=CANCEL` consistently; an implementer cannot infer both.

   The only seal algorithm requires `state==STOPPING` (lines 2177–2182), while
   overflow, discontinuity, conversion failure, and WASAPI failure arise in
   `CAPTURING` and are described as directly sealing their reason. Freeze the
   internal-failure CAS that first installs/priority-merges the failure reason
   and reaches `STOPPING`, then seals it. It must race correctly with an
   external user/permission stop.

   The wake target is also inconsistent: the general `CaptureRequestStop`
   algorithm signals only `stopEvent`, but an activating capture thread is
   blocked on `captureThreadWakeEvent`; cancellation prose separately says it
   signals that event. Define the event(s) signaled per source state. Finally,
   `CaptureGetResult` cannot map `SEALED` to “`ACTIVATING` (or the last public
   state)” unless the chosen last state is actually stored. Freeze the exact
   projection for activation and running capture. Add deterministic transition
   tests for every state × reason, internal failure vs permission/user stop,
   activation cancel wakeup, and the seal linearization edge.

3. **Make graceful quit terminate with every operation category, and make
   quiescence observable.** Lines 913–925 call `CaptureRequestStop` “for every
   active operation,” wait until **all** operations are terminal, and only then
   release them. But line 1851 explicitly says permission, enumeration,
   default-device, and picker operations have no cancellation; the picker is
   user-driven and may remain pending forever. `PickerRelease` before terminal
   is forbidden. Therefore tray Quit can wait forever and never post
   `WM_APP+CLEANUP_READY`.

   Freeze a per-operation quit table: exact cancellation/dismissal request,
   callback/handle ownership, terminal wait, release, and timeout/fail-safe for
   capture, picker, permission request, enumeration, and default-device query.
   WinRT exposes `IAsyncInfo::Cancel`, but the contract must select it and prove
   the concrete operation honors it, or freeze another documented dismissal
   path; merely saying operations “complete naturally” is not a quit protocol:
   https://learn.microsoft.com/en-us/uwp/api/windows.foundation.iasyncinfo.cancel

   The waiter also says “wait for callback refs = 0,” but no ABI lets Go observe
   that condition. The UI handler then calls `CapDestroy` and posts `WM_QUIT`
   without checking whether `CapDestroy` returned `E_ILLEGAL_METHOD_CALL`.
   Add a helper quiescence event/query or a retryable UI-thread destroy
   handshake; `WM_QUIT` is legal only after `CapDestroy==S_OK`. Freeze the live
   Ctrl-C/SIGTERM bridge too: current Windows `awaitShutdown` ignores its signal
   channel, so prose that SIGTERM sends `GracefulQuit` is not an implementation
   delta. Test quit with an open picker, pending permission prompt, delayed
   enumeration callback, in-flight `AccessChanged`, first `CapDestroy` failure,
   and SIGTERM while the UI pump remains live.

4. **Replace the “complete” HRESULT/cleanup table with one that actually owns
   every resource and uses one mapping.** Microsoft documents that the caller
   must free a successful `GetMixFormat` allocation with `CoTaskMemFree`:
   https://learn.microsoft.com/en-us/windows/win32/api/audioclient/nf-audioclient-iaudioclient-getmixformat
   Rev 11 frees `pMixFormat` on failures through `Start`, but every running-
   stream row (`GetNextPacketSize`, `GetBuffer`, `ReleaseBuffer`, `Stop`) omits
   it. Either copy the needed fields and free immediately after successful
   `Initialize`, or carry and free it in every later cleanup path. Prove the
   successful-start path has zero outstanding allocations.

   The table also says `GetBuffer`/normal `ReleaseBuffer` “any failure” maps to
   `CAP_REASON_WASAPI_ERROR`, while its own note says `E_ACCESSDENIED` maps to
   `PERMISSION_REVOKE` at every call site and the preceding mapping assigns
   `AUDCLNT_E_DEVICE_INVALIDATED` to `DEVICE_LOST`. Split known HRESULT rows or
   remove the contradictory global claim; privacy classification must not
   depend on which paragraph an implementer follows.

   The two-phase cleanup at lines 1076–1085 unconditionally calls `Stop` and
   releases `IAudioCaptureClient`, even for activation/initialization failures.
   Freeze explicit `initialized`, `serviceAcquired`, and `started` ownership
   flags and derive cleanup from them. The claim that `Stop` before `Start`
   returns `AUDCLNT_E_NOT_STOPPED` is not the documented `Stop` contract;
   Microsoft documents prior successful `Initialize` and `S_FALSE` for an
   already-stopped stream:
   https://learn.microsoft.com/en-us/windows/win32/api/audioclient/nf-audioclient-iaudioclient-stop
   A conservative “call only after successful Start” policy is acceptable, but
   state it as policy rather than inventing an HRESULT. Generate tests for
   every table row, including running `GetNextPacketSize`/`GetBuffer`/
   `ReleaseBuffer` mappings and the successful allocation lifetime; the current
   11 tests do not cover the whole table.

5. **Make orphan `.partial` recovery reason-aware and fail-closed.** The outcome
   matrix requires permission-revoke, cancel, overflow, discontinuity, format
   error, and unknown WASAPI failure to delete the draft. That deletion is not
   atomic with the terminal signal and process lifetime. Counterexample:
   permission is revoked after more than the minimum duration, the stop reason
   is recorded, and the process is killed before Go deletes the file. On next
   launch, lines 2404–2416 inspect only header shape and duration and promote
   the unauthorized `.partial` to `.wav`. The same algorithm can promote an
   integrity-failed overflow/discontinuity draft. Calling every structurally
   valid orphan “recoverable” erases the security decision.

   Freeze a durable per-session journal/sidecar state machine (or a stricter
   default-discard policy) that distinguishes promotable shutdown/suspend/lock
   interruption from prohibited reasons and unknown crash state. Specify write
   ordering, `FlushFileBuffers`, atomic replace, file/journal identity, stale or
   missing journal behavior, current permission recheck, and cleanup. Unknown
   or ambiguous state must never auto-promote. Test process kill at every edge:
   before/after reason persistence, terminal publication, final drain, delete,
   header rewrite, flush, and rename, for every terminal reason. Prove that no
   permission/integrity failure becomes a `.wav` after restart.

## Resubmission

- Amend one authoritative note, attach a byte-identical outcome, keep product
  source untouched, and return to `to-review`, never `done`.
- Preserve the selected AppContainer/manifest lane, UI-thread WinRT calls,
  agile activation callback, release-before-ring-commit packet loop, picker
  handle API, process-lifetime DLL loading, permission enum mapping, and the
  parts of the HRESULT table that are internally consistent.
- Execute deterministic state, quit, cleanup-allocation, and crash-recovery
  models. A final summary saying the blockers are fixed is not evidence; every
  normative occurrence and the generated tests must agree.
