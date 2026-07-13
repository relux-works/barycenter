# Root review round 9 — Windows AppContainer capture bridge

Date: 2026-07-12  
Task: `TASK-260712-6kba80`  
Verdict: rejected. I covered every Rev 9 line by combining the prior complete
2,520-line Rev 8 read with the complete Rev 8→Rev 9 diff and direct reads of
every modified section. The authoritative Rev 9 is 2,686 lines with SHA-256
`68bdc6f4c5372c8d92c421bd01bf963a9d9197311ce8d0bd2996f1de1dfcef21`.
The task outcome is still the old 2,520-line Rev 8 with SHA-256
`00a0a21d659da68544e0f4235cea44658d492394cc6b1ef85eab7abaa3a84256`;
the agent's claim that it reattached a byte-identical outcome is false. Product
source remains untouched. Permission normalization, truthful WASAPI reasons,
ring sizing, loader injection, and global apartment tracking are useful fixes,
but the replacement capture lifetime is a use-after-free design and the branch
table retains the exact pre-cleanup terminal contradiction it claims to remove.

## Blocking corrections

1. **Make terminal publication follow the cleanup fence in the actual branch
   table.** The Rev 9 summary says timeout and synchronous launch failures only
   set a pending cause and the capture thread publishes terminal after apartment
   cleanup. The authoritative table and diagram say the opposite:
   - `readyEvent` timeout: UI thread sets terminal and signals Go, then capture
     thread later calls `CoUninitialize`;
   - synchronous `ActivateAudioInterfaceAsync` failure: UI publishes terminal,
     then capture thread cleans up;
   - async activation failure: callback publishes terminal while the capture
     thread still has to wake and uninitialize;
   - null handoff says capture thread publishes terminal, while cancel prose
     says the late callback is the publisher and one table cell says the
     callback “sees terminal already set.”

   These violate the invariant that terminal means all required COM work and
   session access are finished. Use one composite completion barrier. UI and
   callback paths store only a pending result/cause and wake the capture thread;
   the final owner publishes terminal after its cleanup, except pre-handoff
   cancellation where the late callback may publish only after it observes the
   capture-thread-done fence and releases the returned interface. Rewrite the
   diagram, branch table, callback ordering, tests, highlights, and final answer
   from the same state transition table. A barrier before `CoUninitialize` must
   make `CaptureGetResult` remain nonterminal for timeout, sync launch failure,
   async activation failure, and cancellation.

2. **Replace the new capture-thread use-after-free with a real lifetime
   holder.** Rev 9 removes the thread's strong reference entirely. Yet it allows
   terminal to become visible before `threadDone=1`, and `CaptureRelease` drops
   the registry reference whenever terminal is visible. With the activation
   callback already gone, the operation destructor frees session memory while
   the capture thread is paused and still must write `threadDone`; “the
   destructor checks threadDone but does not wait” cannot keep freed memory
   alive. `CapDestroy` is irrelevant because destruction already occurred in
   `CaptureRelease`. The required barrier test actually forces this UAF.

   Choose an executable ownership model: retain a thread lifetime reference
   whose final release cannot self-join; use a separate non-session thread
   context/join owner; or prohibit `CaptureRelease` until a thread-done fence is
   set and signal terminal readiness only after the thread's final session
   access. If `threadDone` is set before the native thread's tiny return
   epilogue, define it as “no further state access,” keep the process-lifetime
   DLL assumption, and signal Go from a local duplicated handle after the fence.
   Test with registry as the only other ref, callback absent, and a barrier at
   the final state access under ASAN/page heap. No destructor may observe false
   and free anyway.

3. **Commit a packet only after `ReleaseBuffer` succeeds.** The new scratch
   buffer prevents partial conversion visibility, but the pseudocode still
   copies scratch to the ring and publishes the producer index **before**
   calling `ReleaseBuffer`. It then claims a `ReleaseBuffer` failure makes zero
   frames visible, which is impossible after publication. The silent path also
   writes directly to the ring before release. Convert/fill scratch, call
   `ReleaseBuffer(frames)`, and only on `S_OK` copy/commit the complete packet to
   the ring; capacity cannot shrink in the single-producer/single-consumer
   interval. Freeze stop behavior in the release→commit window and add a
   consumer barrier proving zero visibility on injected `ReleaseBuffer`
   failure for both silent and non-silent packets.

4. **Seal terminal reason atomically with terminal state.** One diagram reloads
   stop reason before `Stop`/COM cleanup, while the detailed R8-4 section reloads
   it after cleanup. Even the latter leaves a race between the reload and
   terminal publication: a higher-priority permission revoke can win the reason
   CAS while state is still STOPPING, after the capture thread copied the old
   value, and terminal can publish the stale value. Under one mutex or CAS loop,
   atomically transition STOPPING→TERMINAL and snapshot the final priority
   reason; `CaptureRequestStop` must either update before that linearization or
   observe terminal and rely on the separately defined live permission guard.
   Add barriers on both sides of the seal.

   Keep HRESULT scope consistent too: the detailed table covers
   `GetNextPacketSize`, `GetBuffer`, `ReleaseBuffer`, `Start`, and `Stop`, while
   the final answer additionally claims `Initialize` and `GetService`. Define
   truthful outcomes for every failing WASAPI/COM call once, including which
   paths have PCM and which are promotable.

5. **Give the waiter one race-free operation/shutdown owner.** The dedicated
   waiter is the right integration with live `GetMessageW`, but the note lets
   the waiter query/read operations while UI messages can trigger stop/release,
   and does not freeze who performs `*Release`, handle take/close, or final WAV
   promotion. A posted terminal message can make the UI release an operation
   while the waiter is still draining it. Define a single owner (preferably the
   waiter) for query/read/take/release plus a synchronized command/result queue;
   `PostMessageW` carries only stable IDs, never transient Go pointers, and the
   UI initiates only the APIs that require its apartment.

   Split graceful Quit from `WM_ENDSESSION`: graceful shutdown requests stop,
   waits asynchronously for terminal, final-drains, releases/unsubscribes,
   stops and joins the waiter, closes events, then calls `CapDestroy` on the UI
   thread. Imminent `WM_ENDSESSION` requests stop and returns without pretending
   that one `shutdownEvent` drain completed operations; the OS may reclaim the
   still-live waiter/handles and startup recovery owns the partial. Test release
   racing a waiter drain, picker-handle transfer, graceful quit, and abrupt end
   session.

6. **Finish init rollback and remove stale contradictory summaries.** If
   `RoInitialize` succeeds but later CapInit state allocation/registration
   fails, the helper must call same-thread `RoUninitialize` before returning;
   “failed CapInit needs no uninitialize” is true only when RoInitialize itself
   failed. Add that injected barrier/test.

   The highlights still say the capture thread holds a strong session ref,
   while Rev 9 says it does not. The generic async section still says all launch
   errors return directly, while the capture table deliberately converts
   post-publication launch errors to operation outcomes. The final allocation
   summary says the maximum ring is 6.1 MiB although its own calculation is
   24,576,000 bytes (~23.4 MiB), and says two periods where the formula only
   guarantees one full endpoint buffer. Make every summary derivative of the
   normative tables; do not leave mutually exclusive implementation choices.

7. **Attach the reviewed revision as the actual outcome.** The current outcome
   resource content is Rev 8, not Rev 9. After correcting the blockers, replace
   the outcome bytes, verify line/word/byte counts and full SHA-256 for both
   paths, then set `to-review`. Do not report “byte-identical” from metadata or
   intention; verify the files on disk.

## Resubmission

- Keep product source untouched; amend one authoritative note and attach one
  byte-identical outcome; return to `to-review`, never `done`.
- Preserve the accepted permission enum, fallback gate, truthful HRESULTs,
  message-pump direction, dynamic ring size, picker table, PCM conversion,
  loader wrapper, apartment balance, conditional WAV gate, and hardware proofs.
- Prove the actual terminal/lifetime/packet/reason/waiter transitions under
  deterministic barriers and memory diagnostics, not by restating invariants.
