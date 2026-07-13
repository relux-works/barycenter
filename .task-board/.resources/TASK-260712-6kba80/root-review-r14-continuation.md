# Root continuation review — incomplete Windows Rev 14 candidate

Date: 2026-07-12  
Task: `TASK-260712-6kba80`  
Status: prior correction agent exhausted its budget and exited 1. The modified
authoritative note is **not** an outcome and is not reviewable as complete:
`research.md` still contains Rev 13 bytes. Continue only the corrections below,
keep product source untouched, then attach one byte-identical canonical outcome
and return to `to-review`.

## Preserve already-correct amendments

- Public capture states are now disjoint:
  preparing=0, activating=1, capturing=2, stopped=3, failed=4, cancelled=5.
- `CaptureFormat` v2 has observable `ready`; terminal mapping uses 3/4/5.
- Go-only post-terminal sidecar and explicit duplicate-key validation direction.
- Priority table/tests now use overflow=1, discontinuity=2,
  permission-revoke=3 and first-installed for the equal rank-4 pair.
- New waiter diagram correctly says the waiter remains the sole owner until
  every late terminal operation is queried and released.

## Required completion corrections

1. **Finish the private preparation state machine.** The public ABI now has a
   `ready` flag, but the packed private FSM still contains only
   `IDLE, ACTIVATING, CAPTURING, STOPPING, SEALED, TERMINAL` (current lines
   2602–2610). `CaptureRequestStop` returns `E_NOT_VALID_STATE` for IDLE and
   handles only ACTIVATING/CAPTURING, while the contract requires cancellation
   during blocked MTA initialization and after MTA-ready but before
   `CaptureActivate`. The test at current line 3436 is therefore not
   implementable from the packed transitions.

   Freeze distinct private `PREPARING` and `PREPARED` states (or an equally
   explicit atomic representation). `CapturePrepare` publishes PREPARING;
   successful MTA init publishes PREPARED + ready; `CaptureActivate` atomically
   requires PREPARED and moves to ACTIVATING; callback handoff moves to
   CAPTURING. Stop from PREPARING/PREPARED/ACTIVATING stores the correct public
   last state and signals `captureThreadWakeEvent`; stop from CAPTURING signals
   `stopEvent`. A stop requested while `CoInitializeEx` is blocked remains
   latched and is observed immediately when it returns. Generate the packed
   layout, transition table, diagrams, ABI query mapping, and tests from this
   one FSM.

   `ready` must be session-owned atomic state: capture thread release-stores it;
   `CaptureGetResult` acquire-loads it and copies the value to the caller-owned
   `CaptureFormat`. Do not describe the caller buffer itself as concurrently
   mutated by the worker.

2. **Use pre-created notification duplicates with failure branches and remove
   every stale original-handle signal.** The partial candidate still says
   `SetEvent(notifyEvent)`/undefined `readyEvent` in prose/branch tests (e.g.
   current lines 640, 660, and 949), and Diagram B line 787 says the callback
   does not hold a duplicate while the ownership paragraph uses lazy creation.
   Lazy `DuplicateHandle` at terminal publication has an unhandled failure: the
   callback can publish terminal but fail to wake the sole waiter.

   Freeze this executable policy:

   - `CapturePrepare` duplicates the Go event for the capture thread before the
     operation/thread is published. Duplicate failure returns an HRESULT with
     no operation/thread and no `opId`.
   - `CaptureActivate` creates the callback-owned duplicate before launching
     activation. Duplicate failure returns an HRESULT, launches no activation,
     and leaves PREPARED retryable/cancellable.
   - The capture-thread duplicate is closed exactly once on every path; it is
     signalled for readiness/data/thread-published terminal, and closed without
     signal in Diagram A.
   - The callback duplicate is closed exactly once on every callback path:
     signal+close only when callback is terminal publisher (Diagram A),
     close-without-signal in Diagram B, normal handoff, and async failure paths.
   - No worker calls `SetEvent` on Go's original handle. Replace every stale
     diagram/test/summary occurrence and add exact duplicate/signal/close counts
     plus both duplicate-failure tests.

3. **Replace every surviving old quit algorithm, not just the first diagram.**
   Current lines 1270, 1276–1289, 1298–1300, 1344–1392, 3451, and 3553 still
   contain the rejected policy: timeout→proceed/exit, pending registry plus
   `CLEANUP_READY`, `Sleep(100)` in the wndproc, finite retry exhaustion, and
   overclaims that Cancel dismisses/completes with Canceled.

   One normative quit policy everywhere:

   - `IAsyncInfo::Cancel` is only a cooperative request. A completion may be
     cancelled, successful, failed, or may never arrive before forced exit.
   - Five seconds only exposes/logs the Force Quit affordance. The waiter keeps
     all events and operation ownership, queries every late terminal, releases
     it, and never posts `CLEANUP_READY` while registry/subscription/callback/
     capture-thread state is non-quiescent.
   - `CLEANUP_READY` is posted only after every registry entry is released and
     `CapIsQuiescent()==S_OK`.
   - The UI handler calls `CapDestroy` once and returns immediately. A refused
     destroy schedules a timer/PostMessage retry; it never calls `Sleep` inside
     the wndproc and keeps retrying until success or the 30-second forced
     fallback. On success, atomically cancel/defeat the watchdog then post
     `WM_QUIT`.
   - Every ordinary Quit/Ctrl-C/SIGTERM attempts graceful cleanup and arms an
     automatic forced fallback; these are not mutually exclusive. `os.Exit(1)`
     is explicitly non-graceful.

   Update tests for late terminal at 6, 15, and 29 seconds (query/release,
   graceful destroy, watchdog defeated), never-terminal (no CLEANUP_READY,
   forced exit), and UI-pump responsiveness during repeated destroy failures.

4. **Make sidecar validation match terminal HRESULTs and provide executable
   duplicate parsing.** Current line 2958 requires `hresult=0` for cancel, but
   cancellation diagrams/tests freeze
   `HRESULT_FROM_WIN32(ERROR_CANCELLED)`. Publish one reason/HRESULT table for
   every `CAP_REASON_*`: user/lifecycle success, cancel, device loss,
   permission, overflow, discontinuity, format, and unknown WASAPI failure.
   Recovery validation must use that same table.

   Keep token-level duplicate rejection, but replace the illustrative recursive
   snippet with compilable/tested Go or sufficiently precise pseudocode. Tests
   must cover duplicate keys at top level and nested/array objects in both
   orders, missing required fields, unknown fields, two concatenated objects,
   trailing non-whitespace, and valid whitespace EOF. The standard
   `DisallowUnknownFields` check remains only for unknown fields, never
   duplicates.

5. **Finish and actually run consistency checks.** Remove all stale Rev 13
   summaries/history that assert the superseded state/quit/handle/sidecar
   policies. Run robust scoped checks against the final authoritative bytes and
   record commands plus actual results; do not claim output was reproduced in
   an outcome before the outcome exists. At minimum assert:

   - one public state declaration and one private FSM;
   - no direct worker `SetEvent(notifyEvent)` or `readyEvent`;
   - no timeout→waiter-exit/CLEANUP_READY with unreleased operations;
   - no `Sleep` in the UI retry handler;
   - no sidecar-before-terminal/native-writer claim;
   - no priority-2 permission-revoke claim;
   - no `hresult=0` cancel claim.

## Delivery

Amend the existing authoritative note. Do not edit product files or root-check
fixtures. Only after all corrections and checks succeed, replace the single
canonical `research.md` outcome byte-identically, append a concise note, and
return to `to-review`, never `done`.
