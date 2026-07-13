# Root review round 12 — Windows AppContainer capture bridge

Date: 2026-07-12  
Task: `TASK-260712-6kba80`  
Verdict: rejected. I read all 3,296 lines of Rev 12 and verified the
authoritative and task-board outcome copies are byte-identical (3,296 lines,
43,112 words, 336,070 bytes, SHA-256
`98e62a4303512399f1d7c25c93ea44d90509262cb3555fa69476e8de13ad17bd`).
Product source remains untouched. Rev 12 improves the packed-state layout and
the WASAPI ownership table, but its claimed R11 fixes are not executable as one
contract. The static consistency rule proposed by the revision already fails
against the revision itself.

## Blocking corrections

1. **Publish one executable pre-handoff cancellation algorithm.** The
   “normative” exception at line 572 says the capture thread executes steps
   5–11. Those steps include terminal publication, notification, handle close,
   and exit. Line 585 instead says the thread exits after `threadDone=1` and
   explicitly does **not** publish terminal because the late callback owns that
   publication. Both cannot be implemented. The notification-handle ownership
   is likewise split between a thread-local duplicate (lines 568–569) and an
   independently claimed callback-local duplicate (line 590), without creation,
   transfer, or close rules for the latter.

   The obsolete state representation also remains normative: line 636 checks a
   `cancelled` bit and the lifecycle matrix at line 2279 checks it again, while
   line 2324 says no such bit exists. Line 616 again calls `threadDone` the
   capture thread's “final instruction,” although terminal store, notification,
   handle close, and thread exit follow it. These are direct failures of the
   revision's own static consistency requirement at lines 574–577.

   Generate every cancellation diagram, branch table, lifetime table, test,
   lifecycle row, and summary from one transition function. Freeze separately:
   (a) callback-before-threadDone and (b) callback-after-threadDone; for each,
   name the sole terminal publisher, exact packed-word transition, the owner of
   every notification duplicate, and the final close. No prose may mention a
   `cancelled` bit. Also define how packed private `TERMINAL` maps atomically to
   the public ABI's distinct `stopped`, `failed`, and `cancelled` values and how
   `hresult`/reason become visible before the release store. The current packed
   layout has only a generic `TERMINAL` state and does not freeze that mapping.

2. **Replace the graceful-quit timeout story with a coherent termination
   policy.** The revision repeatedly promises `WM_QUIT` only after
   `CapDestroy==S_OK` (lines 122, 990, 1034, 1049, and 3077). Its actual handler
   posts `WM_QUIT` on an unexpected `CapDestroy` failure and after three refused
   retries (lines 1114–1124). This is the exact behavior the claimed fix says it
   removed.

   The timeout path is internally impossible as written. For example, let an
   open picker remain pending after `IAsyncInfo::Cancel`:

   1. after five seconds the waiter proceeds (lines 1059–1068);
   2. `PickerRelease` is illegal while pending, so the registry remains nonempty;
   3. after another bounded quiescence wait the waiter still posts
      `WM_APP+CLEANUP_READY` and exits (lines 984–991);
   4. `CapDestroy` must reject the nonempty registry;
   5. the UI posts `WM_QUIT` anyway at line 1121.

   Microsoft's raw contract says `IAsyncInfo::Cancel` **requests** cancellation;
   it does not document a five-second completion or picker-dismissal guarantee:
   https://learn.microsoft.com/en-us/windows/win32/api/asyncinfo/nf-asyncinfo-iasyncinfo-cancel
   The revision's stronger “all first-party operations terminate” claim is not
   sourced evidence. The default-device row additionally names a nonexistent
   `GetDefaultAudioCaptureIdAsync`; the documented API is synchronous and
   returns a string:
   https://learn.microsoft.com/en-us/uwp/api/windows.media.devices.mediadevice.getdefaultaudiocaptureid

   Freeze one policy: either graceful quit keeps the pump and waiter alive until
   every registry entry is terminal/released and `CapDestroy` succeeds, or a
   separate explicitly forced process-exit path abandons cleanup and must never
   be described/tested as graceful. A timeout may change UI state or offer a
   force-quit choice; it cannot manufacture quiescence. Add the exact public ABI
   signatures and thread rules for every cancel function (`PickerCancel`,
   permission, enumeration, and any real async wrapper). “Internal export
   called by Go” at lines 1060 and 2007 is a contradiction: if Go calls it, it
   is part of the ABI. Test cancellation-not-honored indefinitely, registry
   nonempty, callback not yet entered, callback currently executing, and every
   `CapDestroy` failure class.

3. **Choose one owner and an actual ABI for the reason journal.** Line 2545 says
   Go is the sole draft writer, the helper only emits `CaptureRead`, and the
   helper never touches the filesystem. Lines 2646–2656 instead make the native
   capture thread write, flush, close, and rename `.partial.reason`, claiming
   the draft directory and session metadata were passed to `CaptureStart`.
   They are not: the frozen ABI at lines 1886–1888 accepts only `deviceId`,
   `notifyEvent`, and `opId`. The helper responsibility inventory at lines
   2817–2831 contains no journal/filesystem responsibility, and the final
   rejection list at line 3111 explicitly forbids the helper as draft writer.
   Rev 12 therefore cannot produce the artifact on which all new recovery
   guarantees depend.

   Select and specify one design end to end. If the helper persists the sealed
   reason before terminal publication, extend the versioned ABI with bounded
   session identity and safe app-private journal destination/handle ownership;
   add filesystem-error, blocking-I/O, temp collision, replace, flush, and close
   behavior to the normative cleanup sequence and responsibility table. If Go
   remains the only writer, introduce an observable sealed-reason handshake or
   accept fail-closed discard when the process dies before Go durably records
   the terminal reason. Do not claim recovery across an edge that the selected
   owner cannot observe.

   Recovery must derive promotability from a validated, known reason enum; it
   must not trust the JSON's redundant `promotable` boolean as the authority
   (lines 2669–2670 and test 26 at line 3063 do exactly that). A syntactically
   valid sidecar containing `reason=PERMISSION_REVOKE` plus
   `promotable=true` currently reaches the promotion path. Freeze version,
   length limits, allowed reason/HRESULT combinations, duplicate-field
   consistency, unknown-field/version behavior, and a corruption check. Add
   this mismatched-field counterexample to the crash-recovery matrix.

4. **Make `CaptureStart` actually asynchronous on the UI thread.** The ABI
   principle says every initiating export returns immediately and no export
   blocks waiting for async work (lines 763–769). `CaptureStart` nevertheless
   waits on `readyEvent` for as long as five seconds on the pinned UI thread
   (line 514), freezing the message pump, picker ownership, and permission UI.
   Calling it “not a WinRT `.get()`” does not make a five-second UI wait
   asynchronous and directly contradicts the stated contract and final
   rejection list.

   Use an event-driven preparation/activation handshake. One viable shape is:
   `CapturePrepare` creates the operation/thread and returns; the thread reports
   MTA-ready or failure through the operation event; the waiter posts a stable
   ID to the UI; the UI invokes a short `CaptureActivate` export that launches
   `ActivateAudioInterfaceAsync`. An equivalent message-based design is fine,
   but no UI-thread export may wait for thread readiness. Freeze stop/quit races
   in every intermediate state and test that the UI pump remains responsive
   while MTA initialization is delayed beyond five seconds.

5. **Unify resource cleanup and HRESULT classification instead of appending a
   second table.** The single normative path says `pMixFormat` is already freed
   (line 563), which is false on format-validation and `Initialize` failures;
   the later table adds separate frees for those paths. Its rows then spell out
   releases and `CoUninitialize` and append “→ §Normative cleanup path” (lines
   2438–2459), which reads as running the same cleanup twice. Add an explicit
   `mixFormatOwned` flag (and preferably rename `initialized`, which is set when
   `IAudioClient` is merely obtained at line 2425, to `audioClientOwned`). The
   one cleanup function must consume and clear each ownership flag exactly once.
   Table rows should set cause/flags and call that function, not restate its
   body.

   HRESULT classification also still has two answers. The global mapping at
   lines 2405–2407 applies “any other” negative `Stop` HRESULT to
   `CAP_REASON_WASAPI_ERROR`; the complete table at line 2459 says a `Stop`
   failure never overrides the original terminal reason. Preserve-original is
   the sound cleanup policy, but it must be the only normative rule. Cover
   initial stop cause × every `Stop` HRESULT, every pre-Initialize mix-format
   ownership edge, and exact once-only free/release counts in generated tests.

6. **Repair the priority and verification inventories so they test the chosen
   contract, not assertions.** Test 6 at line 2373 says overflow beats
   permission revoke because they are “tied at priority 1,” while the priority
   table assigns permission revoke priority 2. Define deterministic behavior
   for the actual equal-priority pair, overflow vs discontinuity; either freeze
   a total order or preserve the first sealed integrity reason and test both
   schedules. The unresolved-proof list also alternates between 11 and 18
   cleanup tests and its “static grep” currently would fail on lines 616, 636,
   and 2279. Run the checks against the authoritative note itself before
   claiming consistency, and include their commands/results in the outcome.

## Resubmission

- Amend one authoritative note, attach a byte-identical outcome, keep product
  source untouched, and return to `to-review`, never `done`.
- Preserve the selected AppContainer/manifest lane, documented UI-thread WinRT
  calls, MTA handoff, release-before-ring-commit packet loop, picker handle API,
  process-lifetime DLL loading, explicit permission enum, and the parts of the
  packed reason arbitration that are internally consistent.
- Execute deterministic cancellation publisher/handle-ownership, ignored-
  cancellation quit, nonblocking UI preparation, once-only cleanup, and
  corrupt/mismatched journal models. A revision summary or future test list is
  not evidence that the current normative contract agrees with itself.
