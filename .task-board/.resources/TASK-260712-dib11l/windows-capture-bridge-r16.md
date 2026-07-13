# 2026-07-12 Windows AppContainer capture and picker bridge decision

Task: `TASK-260712-6kba80`  
Scope: select the legal capture, picker, hotkey, and lifecycle API surface for the signed Windows Store probe under `packagedClassicApp` + `appContainer`, without `runFullTrust`, broad filesystem access, undocumented APIs, or developer-mode-only behavior.

**Revision 16** — amended by root after the complete Rev 15 read (2026-07-13).
This revision is normative; older revision narratives below are historical and
must not be used to infer current behavior. Changes: **R15-1** one graceful-quit
contract keeps the sole-owner waiter alive through every late terminal result;
five seconds only exposes Force Quit, and `CLEANUP_READY` requires an empty
registry plus proven quiescence. **R15-2** graceful destroy atomically defeats
the 30-second watchdog before `WM_QUIT`. **R15-3** one executable notification
ownership/branch table covers eager duplicates, lost CAS, launch failure,
callback paths, exact signal/close counts, never signals Go's original handle,
and uses CRT-safe `_beginthreadex` with a creator-held early-start fence.
**R15-4** the private FSM is uniquely numbered through `TERMINAL=6`;
MTA readiness is session-owned monotonic atomic state copied into caller-owned
`CaptureFormat` by `CaptureGetResult`. **R15-5** the sidecar validates all eleven
reason/HRESULT combinations from one table, including
`ERROR_CANCELLED`. **R15-6** final summaries/tests and the executable checker
are generated from those contracts. Product source remains outside this
research-only revision.

**Revision 15** — completes the interrupted Rev 14 per the root review round 14
continuation (2026-07-13). Changes:
**R14-1** the private packed FSM now freezes distinct `PREPARING`(0) and
`PREPARED`(1) states (state field renumbered `PREPARING=0, PREPARED=1,
ACTIVATING=2, CAPTURING=3, STOPPING=4, SEALED=5, TERMINAL=6`; there is no `IDLE`).
Both preparation states surface publicly as `preparing`(0), disambiguated by
`format->ready`. `CaptureRequestStop` now accepts `PREPARING`/`PREPARED`/
`ACTIVATING` (a stop while `CoInitializeEx` is blocked is latched and observed on
return; `PREPARED`/`ACTIVATING` waits are woken by `captureThreadWakeEvent`), and
`CaptureActivate` atomically requires `PREPARED`. The packed layout, wake-event
table, transition tests (added 1a PREPARING-cancel and 1b PREPARED-cancel), the
`§States` diagram, and the `CapturePrepare`/`CaptureActivate`/`CaptureGetResult`
ABI comments all derive from this one FSM. `lastPublicState` stores the collapsed
public value (`preparing`=0 for both preparation states).
**R14-2** notification duplicates are created **eagerly** with failure branches,
not lazily: the capture-thread duplicate is created in `CapturePrepare` before
the operation/thread is published (failure → HRESULT, no op, no thread, no
`*opId`); the callback duplicate is created in `CaptureActivate` before launching
activation (failure → HRESULT, no activation, state stays `PREPARED`). Each
duplicate is closed exactly once on every path (Diagram A callback signals+closes;
Diagram B / normal handoff / async-failure close-without-signal). Removed the
Rev-14 internal contradiction between "duplicated at registration time" and
"created lazily only in Diagram A"; no worker signals Go's original `notifyEvent`.
Added duplicate-failure tests 6a/6b.
**R14-3** the `WM_APP+CLEANUP_READY` handler no longer calls `Sleep` in the
wndproc: a refused `CapDestroy` arms a one-shot `SetTimer(DESTROY_RETRY_TIMER,
100 ms)` and returns, and the `WM_TIMER` tick retries — the pump keeps dispatching
between attempts, with no finite retry cap; the 30-second `ForceQuit` watchdog is
the only bound. `WM_QUIT` is still posted only after `CapDestroy==S_OK`. Quit test
5 updated to assert the wndproc returns between attempts.
**R14-4** verified: the cancel terminal HRESULT is `HRESULT_FROM_WIN32(
ERROR_CANCELLED)` (not `hresult=0`) in the cancellation diagrams, the MTA-timeout
test, and the sidecar validation order; duplicate-JSON-key rejection is a concrete
token-walking `rejectDuplicateKeys` (nested/array/EOF/trailing-content), not an
illustrative snippet.
**R14-5** added an executable static checker
`.research/root-checks/windows-consistency-check.sh` that greps the normative body
(excluding the revision-history preamble and explicit negations) for every named
anti-pattern — legacy `IDLE`/`STOPPED` states, worker `SetEvent(notifyEvent)`,
stale `readyEvent`, wndproc `Sleep`, live `cancelled bit`, `permission_revoke`
rank 2, helper/native sidecar writer, sidecar-before-terminal, and `hresult=0`
cancel — and exits non-zero on any hit. Its real output (all PASS) is reproduced
in the outcome. The R14-3 blocking `Sleep` and two R14-2 stale worker signals were
found and fixed by this checker.
Honest residual (not a claimed pass): this Rev-15 completion focused on the five
named R14-continuation corrections and the checker's anti-pattern set. It did not
re-derive every diagram from scratch; a full independent read of all ~3.7k lines
by root is still required, and any diagram cell not covered by the checker's
regexes remains subject to review. This is flagged, not asserted resolved.
Prior revision (Rev 14) header follows.
**Revision 14** — amended per root review round 13 blocking findings (2026-07-13).
Changes: **R13-1** one canonical public-state enum everywhere (preparing=0,
activating=1, capturing=2, stopped=3, failed=4, cancelled=5) — terminal values
disjoint from nonterminal; diagrams, packed→public mapping, tests 9/26 and the
final answer corrected off the old stopped(2)/failed(3)/cancelled(4); added the
documented `ready` field to `CaptureFormat` (struct version 2) removing the
state==0/valid==0 "initializing vs MTA-ready" ambiguity. **R13-2** every
`localNotify` duplicate has exactly one close owner on every path (capture-thread
dup closed even in Diagram-A cancel; callback dup created lazily only in Diagram A
where it both signals and closes); no worker signals Go's original `notifyEvent`
handle (former `SetEvent(notifyEvent)` and stale `readyEvent` signals corrected to
`SetEvent(localNotify)` / `format->ready`). **R13-3** the waiter never abandons the
release owner — it keeps querying/releasing late-terminal operations until the
registry is empty and quiescent before posting `WM_APP+CLEANUP_READY`; timeouts
only surface Force Quit, they do not permit exit. **R13-4** removed the unsourced
"all first-party WinRT ops terminate after Cancel" overclaim (Cancel only
*requests*); graceful and forced exit are no longer called mutually exclusive —
forced is the automatic 30-second fallback of graceful. **R13-5** Go is the sole
sidecar writer *after* terminal (all "before terminal notification" and
"capture thread writes the sidecar" text removed); explicit token-level
duplicate-JSON-key rejection replaces the false `DisallowUnknownFields`
dedup claim; stale `promotable=false` fields removed from proofs. **R13-6** the
static self-check greps are rewritten as scoped anti-pattern checks whose real
output is empty and reproduced in the outcome (the old literal `→ 0` claims were
false). **R13-7** one canonical strict priority order with a single documented
wasapi_error/format_error tie (first-sealed wins); test 6 corrected to
permission_revoke rank 3; tie and permission-vs-user tests added.
Known residual (not a fabricated pass): the `WM_APP+CLEANUP_READY` handler still
shows a `Sleep(100)` retry loop in the wndproc; per R13-3 this must become a
timer/`PostMessage`-driven retry so the pump never blocks — flagged in Unresolved
proofs, not claimed fixed.
Prior revision (Rev 13) header follows.
**Revision 13** — amended per root review round 12 blocking findings (2026-07-13).
Changes: **R12-1: Publish one executable pre-handoff cancellation algorithm** —
Rev 12 still referenced a `cancelled` bit (lines 636, 2279) despite line 2324
stating no such bit exists; line 616 called `threadDone` the capture thread's
"final instruction" when terminal store, notification, handle close, and exit
follow it; the pre-handoff cancellation exception at line 572 said the capture
thread executes steps 5–11 (including terminal publication) while line 585 said
the thread exits after `threadDone` without publishing terminal. Fixed: all
cancellation paths now derive from two frozen diagrams — callback-before-
`threadDone` and callback-after-`threadDone` — each naming the sole terminal
publisher, exact packed-word transition, owner of every notification duplicate,
and final close. No prose mentions a `cancelled` bit. Public ABI mapping from
packed private `TERMINAL` to distinct `stopped`(3), `failed`(4), `cancelled`(5)
is frozen with derivation from sealed reason (R13-1: matches the single
canonical `CaptureGetResult` enum preparing=0/activating=1/capturing=2/
stopped=3/failed=4/cancelled=5).
**R12-2: Replace graceful-quit timeout with coherent termination policy** —
Rev 12 posted `WM_QUIT` on unexpected `CapDestroy` failure and after three
refused retries (lines 1114–1124), exactly the behavior the fix claimed to
remove. The timeout path was internally impossible: a pending picker after
`IAsyncInfo::Cancel` would leave `PickerRelease` illegal, the registry
nonempty, `CapDestroy` rejecting, and the UI posting `WM_QUIT` anyway. The
default-device quit row named a nonexistent `GetDefaultAudioCaptureIdAsync`;
the documented API is synchronous. Fixed: graceful quit keeps the pump and
waiter alive until every registry entry is terminal/released and `CapDestroy`
succeeds; a separate explicit forced-exit path (`ForceQuit` in tray menu +
30-second watchdog) abandons cleanup and is never described/tested as graceful.
Cancel exports have frozen public ABI signatures. `GetDefaultAudioCaptureId`
is synchronous — no cancel needed. Tests cover cancellation-not-honored
indefinitely, registry nonempty, callback not yet entered, callback executing,
and every `CapDestroy` failure class.
**R12-3: Choose one owner for the reason journal** — Rev 12 said Go is the
sole draft writer (line 2545) and the helper never touches the filesystem, but
then made the capture thread write/flush/rename `.partial.reason` (lines
2646–2656) using a draft directory not in the `CaptureStart` ABI (lines
1886–1888). The helper responsibility inventory and final rejection list
explicitly forbade helper filesystem access. Recovery trusted a redundant
`promotable` boolean that could be `true` with `reason=PERMISSION_REVOKE`.
Fixed: Go is the sole sidecar writer — after terminal state, Go reads the
sealed reason via `CaptureGetResult`, derives promotability from the reason
enum (not a redundant boolean), and writes the sidecar atomically. Process
death before Go records the reason → fail-closed discard (missing sidecar).
The sidecar JSON drops the redundant `promotable` field; recovery derives
it. Mismatched-field counterexample added to crash-recovery matrix.
**R12-4: Make CaptureStart actually async on the UI thread** — Rev 12's
`CaptureStart` waited on `readyEvent` for up to 5 seconds on the pinned UI
thread (line 514), freezing the message pump. The note called it "not a WinRT
`.get()`" but a 5-second UI wait directly contradicts the async ABI principle.
Fixed: two-step `CapturePrepare`/`CaptureActivate` handshake.
`CapturePrepare` creates the operation and capture thread and returns
immediately. The thread signals MTA-ready or failure through the operation's
`notifyEvent`. The waiter observes readiness and posts a message to the UI
thread. `CaptureActivate` launches `ActivateAudioInterfaceAsync` and returns
immediately. No UI-thread export waits for thread readiness. Stop/quit races
in every intermediate state are frozen and tested.
**R12-5: Unify resource cleanup HRESULT classification** — Rev 12 appended
a second cleanup table whose rows spelled out releases + `CoUninitialize` and
then appended "→ §Normative cleanup path" (reading as running cleanup twice).
The `initialized` flag was set when `IAudioClient` was obtained, not when
`Initialize` was called. Global `Stop`-failure HRESULT → `CAP_REASON_WASAPI_ERROR`
contradicted the table's "preserve-original" policy. Fixed: `initialized`
renamed to `audioClientOwned`; `mixFormatOwned` flag added; one cleanup
function consumes and clears each flag exactly once; table rows set cause/flags
and invoke that function; preserve-original is the sole `Stop`-failure rule.
**R12-6/R13-7: Repair priority and verification inventories** — Rev 12 test 6
claimed overflow and permission_revoke shared one priority rank, and stale
prose called permission_revoke rank 2. Rev 13's canonical priority table assigns
one rank per reason with a single documented tie:
overflow(1) > discontinuity(2) > permission_revoke(3) >
wasapi_error(4) = format_error(4, tie) > device_lost(5) > shutdown(6) >
suspend(7) > lock(8) > cancel(9) > user_stop(10). The one equal-ranked pair
(wasapi_error vs format_error) has a frozen tie-break: **the first reason
sealed wins** (both are non-promotable, so the distinction is immaterial for
promotion but deterministic for evidence — see §Packed atomic compare-and-swap
step 5, which no-ops on equal priority, leaving the already-installed reason).
Every generated priority test derives its expectation from that single table
(overflow beats permission_revoke because 1 < 3; overflow beats discontinuity
because 1 < 2; wasapi_error vs format_error resolves by first-sealed). The distinction between
overflow and discontinuity is immaterial for promotion (both non-promotable)
but is deterministic for evidence logging. Static grep fixture commands are run
against the note itself before claiming consistency; see §Static grep fixture
self-check for the exact commands and their real observed output.
Prior revision history: Rev 12 introduced R11-1 through R11-5.
**R11-1: Eliminate the last pre-barrier/final-access contradictions** —
Rev 11's handoff diagram (lines 315–324) still called `threadDone=1` "the final
session-state access" and then published terminal after it; the standalone
overflow sequence (lines 1347–1355) was a second normative algorithm omitting
the packed reason seal, `threadDone`, fence, terminal store, and `localNotify`;
the branch table (lines 508–510) ended several rows at "sets threadDone, exits"
without the required final store. All three sites now reference the single
normative cleanup path (§Normative cleanup path). A static consistency grep
fixture asserts no stale sequences remain.
**R11-2: Complete packed state machine for activation cancellation and internal
capture failures** — Rev 11's packed layout had no explicit `cancelled`
representation despite multiple normative paths reading/writing "the cancelled
bit." Cancellation is now canonically `state=STOPPING` + `reason=CANCEL` (no
separate bit). Internal capture failures (overflow, discontinuity, conversion,
WASAPI error) arising in `CAPTURING` use a two-step CAS: first install
`state=STOPPING` + `reason=<cause>` (priority merge), then the existing seal
CAS. Wake events are specified per source state (`stopEvent` for `CAPTURING`,
`captureThreadWakeEvent` for `ACTIVATING`). `CaptureGetResult` maps `SEALED` →
a stored `lastPublicState` field set at the `CAPTURING→STOPPING` or
`ACTIVATING→STOPPING` transition. Deterministic transition tests added for every
state × reason, internal failure vs external stop, activation cancel wakeup, and
seal linearization edge.
**R11-3: Per-operation graceful quit table with quiescence and SIGTERM bridge** —
Rev 11's graceful quit said "call `CaptureRequestStop` for every active
operation" but picker, permission, enumeration, and default-device had no
documented cancellation or dismissal, so the quit could pend forever. A
per-operation quit table now specifies the exact cancellation/dismissal
mechanism (WinRT `IAsyncInfo::Cancel` for permission, enumeration, and
default-device; `IFileOpenPickerStatics::ResumePickSingleFileAsync`-cancel +
fallback `IAsyncInfo::Cancel` for picker), callback/handle ownership, terminal
wait, release, and a 5-second timeout fail-safe. Helper exposes
`CapIsQuiescent` for Go to poll callback ref count before `CapDestroy`.
`CapDestroy` return is checked: `WM_QUIT` only after `CapDestroy==S_OK`, with
bounded retry. Ctrl-C/SIGTERM bridged to `GracefulQuit` via a `signal.Notify`
goroutine posting to `commandCh`. Tests: quit with open picker, pending
permission, delayed enumeration, in-flight `AccessChanged`, first `CapDestroy`
failure, and SIGTERM while UI pump lives.
**R11-4: HRESULT/cleanup table with resource ownership flags** — Rev 11 freed
`pMixFormat` on pre-`Start` failures but omitted it from running-stream rows;
`GetBuffer`/`ReleaseBuffer` "any failure" contradicted the per-HRESULT
`E_ACCESSDENIED`→`PERMISSION_REVOKE` and `AUDCLNT_E_DEVICE_INVALIDATED`→
`DEVICE_LOST` mappings; two-phase cleanup unconditionally called `Stop` and
released `IAudioCaptureClient` even for activation/init failures; the Stop-
before-Start claim cited a non-existent `AUDCLNT_E_NOT_STOPPED` code. Fixed:
`pMixFormat` is `CoTaskMemFree`d immediately after `Initialize` succeeds (fields
copied first); running-stream rows have no `pMixFormat` to free. Running-stream
rows split `E_ACCESSDENIED`, `AUDCLNT_E_DEVICE_INVALIDATED`, and "any other
failure" explicitly; no contradictory global claim. Explicit `audioClientOwned`,
`serviceAcquired`, and `started` ownership flags derive cleanup. `Stop` is
called only after successful `Start` (policy, not HRESULT-based: Microsoft
documents `S_FALSE` for already-stopped, not `AUDCLNT_E_NOT_STOPPED`). Tests
cover the complete table including running-stream HRESULT splits and the
successful-start allocation lifetime proof.
**R11-5: Reason-aware fail-closed orphan `.partial` recovery** — Rev 11's
startup recovery inspected only header shape and duration, promoting any
structurally valid orphan — a permission-revoke or overflow session killed
before Go deleted the file would be promoted on restart. Fixed: a durable
`.partial.reason` sidecar file is written atomically (write-to-temp +
`FlushFileBuffers` + rename) by Go after it observes terminal state (R13-5 —
Go is the sole writer; the helper has no filesystem ABI), containing the
sealed stop reason and session identity. Startup recovery reads the sidecar:
present + promotable reason + current permission `Allowed` → promote; present +
non-promotable reason → discard; missing or corrupt sidecar → discard
(fail-closed). Stale sidecar detection via session-ID mismatch. Tests: process
kill at every edge (before/after reason persistence, terminal publication, final
drain, delete, header rewrite, flush, rename) for every terminal reason. No
permission/integrity failure ever becomes a `.wav` after restart.
Prior revision history: Rev 11 introduced R10-1 through R10-6 (composite
barrier, truthful final-access, release-before-commit, packed reason seal,
async quit state machine, complete HRESULT table). Rev 10 introduced composite
completion barrier (R9-1), capture-thread lifetime without UAF (R9-2), packet
ring commit after ReleaseBuffer (R9-3), atomic terminal-reason seal (R9-4),
single-owner waiter with synchronized command/result queue (R9-5), `CapInit`
rollback (R9-6). Revisions 1–9 history preserved in prior revision notes.
**Single-owner waiter with synchronized command/result queue** — Rev 9 allowed
the waiter to query/read operations while UI messages could trigger stop/release,
and did not freeze who performs `*Release`, handle take, or WAV promotion. A
posted terminal message could make the UI release an operation while the waiter
was still draining it. Fixed: the waiter goroutine is the **single owner** of
all query/read/take/release/promotion operations. `PostMessageW` carries only
stable operation IDs (never Go pointers). The UI thread initiates only the APIs
that require its apartment (`CapturePrepare`, `CaptureActivate`, `PickerOpenFile`, `CapPermissionRequest`).
A synchronized command/result channel passes requests from UI to waiter and
results from waiter to UI. Graceful quit is separated from `WM_ENDSESSION`:
graceful quit requests stop, waits asynchronously for terminal, final-drains,
releases/unsubscribes, stops and joins the waiter, closes events, then calls
`CapDestroy` on the UI thread. `WM_ENDSESSION` requests stop and returns from
the wndproc without pretending that one `shutdownEvent` drain completed; the OS
reclaims the still-live waiter/handles and startup recovery owns the partial.
Tests cover release racing a waiter drain, picker-handle transfer, graceful quit,
and abrupt `WM_ENDSESSION` (R9-5).
**Init rollback and stale summary cleanup** — if `RoInitialize` succeeds but a
later `CapInit` state allocation or registration fails, the helper now calls
same-thread `RoUninitialize` before returning (R9-6). Stale contradictions
removed: the highlights and generic async section no longer say the capture
thread holds a strong session ref (it does not — R9/R8-2). The generic async
section no longer says all launch errors return directly (post-publication launch
errors travel through the operation — R8-1). The ring size in the allocation
summary is corrected from "6.1 MiB" to "≈23 MiB" (matching the actual
calculation of 24,576,000 bytes at 384 kHz × 8 channels × 4 bytes). The ring
guarantee is correctly stated as "at least one full WASAPI endpoint buffer,"
not "two periods" (the ring is `max(2 × sampleRate, bufferFrames)` frames;
`GetBufferSize()` returns the total buffer, not one period) (R9-6).
**Byte-identical outcome verification** — the authoritative note and the
task-board outcome are verified byte-identical via SHA-256 checksum on disk,
not from metadata or intention (R9-7).
All earlier rev-1/2/3/4/5/6/7/8/9/10 changes preserved.

## Highlights (R12 additions)

- **One executable pre-handoff cancellation algorithm from two frozen diagrams.** Callback-after-`threadDone` (typical): callback is the sole terminal publisher; callback-before-`threadDone` (rare): callback stores pending cause, capture thread publishes. No prose mentions a `cancelled` bit. Public ABI maps packed `TERMINAL` → `stopped`(3)/`failed`(4)/`cancelled`(5) via sealed reason (R12-1, R13-1).
- **Two-step `CapturePrepare`/`CaptureActivate` replaces the blocking `CaptureStart`.** No UI-thread export waits for thread readiness. `CapturePrepare` creates the thread and its notification duplicate, then returns immediately; the thread signals MTA-ready only through that duplicate (the same kernel event becomes signaled, but Go's original handle value is never used); waiter posts to UI; `CaptureActivate` launches `ActivateAudioInterfaceAsync` and returns immediately (R12-4/R15-3).
- **Graceful quit keeps pump and waiter alive until quiescence.** No `PostQuitMessage` on `CapDestroy` failure — the pump continues and waiter retries. Separate `ForceQuit` (tray menu + 30-second watchdog) abandons cleanup and is never described as graceful. Cancel exports have frozen public ABI signatures. `GetDefaultAudioCaptureId` is synchronous — no cancel needed (R12-2).
- **Go is the sole sidecar writer.** Go reads sealed reason via `CaptureGetResult`, derives promotability from the reason enum, writes `.partial.reason` atomically. Process death before Go records → fail-closed discard. No redundant `promotable` boolean in sidecar JSON (R12-3).
- **Unified cleanup function with explicit ownership flags.** `audioClientOwned` (renamed from `initialized`), `mixFormatOwned`, `serviceAcquired`, `started`. One function consumes and clears each flag exactly once. Table rows set cause/flags and call that function. Preserve-original is the sole `Stop`-failure rule (R12-5).
- **Canonical priority order with one explicit tie.** Overflow wins over discontinuity. `WASAPI_ERROR` and `FORMAT_ERROR` share rank 4 and the first installed reason wins; both are non-promotable. Static checks must preserve that rule rather than calling it a tie-free total order (R12-6, R15-6).

## Highlights (R11 additions)

- **One normative cleanup path, no competing algorithms.** Every terminal publisher — handoff diagram, branch table, overflow/error prose, two-phase lifetime, tests, and final summary — references a single transition table. No text says `threadDone` is the final session-state access; no standalone sequence omits seal → cleanup → `CoUninitialize` → `threadDone` → fence → terminal → `localNotify`. A static grep fixture asserts no stale sequences remain (R11-1).
- **Cancellation is `state=STOPPING` + `reason=CANCEL` in the packed word.** No separate `cancelled` bit. Internal capture failures (overflow, discontinuity, conversion, WASAPI) use a two-step CAS: first install `STOPPING` + failure reason (priority merge with any concurrent external stop), then the existing seal CAS. Wake events are per-state: `stopEvent` for `CAPTURING`, `captureThreadWakeEvent` for `ACTIVATING`. `CaptureGetResult` maps `SEALED` → stored `lastPublicState` (R11-2).
- **Per-operation quit table with retained ownership.** `IAsyncInfo::Cancel` is only a cooperative request for picker, permission, and enumeration operations. Five seconds exposes/logs Force Quit but never abandons the operation owner. The waiter handles any late success/cancel/failure, releases every registry entry, and posts `CLEANUP_READY` only after `CapIsQuiescent()==S_OK`. Default-device wraps a synchronous API and has no `IAsyncInfo` cancellation object. Ctrl-C/SIGTERM are bridged via `signal.Notify` (R15-1).
- **`pMixFormat` freed immediately after `Initialize`.** Needed fields copied to session state first. Running-stream rows have no `pMixFormat` to leak. Running-stream HRESULT rows split `E_ACCESSDENIED`, `AUDCLNT_E_DEVICE_INVALIDATED`, and "any other failure" explicitly — no contradictory global "any failure" claim. Cleanup derived from `audioClientOwned`/`mixFormatOwned`/`serviceAcquired`/`started` ownership flags (R12-5). `Stop` called only after successful `Start` (policy, not HRESULT-based; Microsoft documents `S_FALSE` for already-stopped) (R11-4).
- **Reason-aware fail-closed orphan recovery.** `.partial.reason` sidecar written atomically by Go **after** it observes terminal state (R13-5 — sole writer; helper has no filesystem ABI). Startup recovery: present + promotable reason (derived from the reason enum, no `promotable` field) + permission `Allowed` → promote; present + non-promotable → discard; missing/corrupt/stale/duplicate-key → discard (fail-closed). Death before Go's durable write → missing sidecar → discard. No permission/integrity failure becomes a `.wav` after restart (R11-5, R13-5).

## Highlights (R10 additions)

- **Every terminal publisher goes through the composite barrier.** All normative sections — diagram, branch table, timeout prose, sync launch failure text, two-phase contract, late callback, acquired-packet error — derive from one rule: UI and callback paths store only a pending cause; the capture thread (or late callback after observing `threadDone==1`) publishes terminal after `CoUninitialize` → `threadDone` → fence → terminal → `localNotify` (R10-1).
- **`threadDone` means "cleanup complete; one terminal store remains."** The terminal store is the final session-state access. `CaptureGetResult` uses acquire read. The registry cannot be dropped until the terminal store is observable (R10-2).
- **C++/WinRT cycle broken at callback return, not after terminal.** The normal (successful activation) callback never publishes terminal — it hands off and returns while the capture thread runs for minutes. The stored async-operation reference is cleared at callback return (after `GetActivateResult` + handoff/pending-cause store) on every success/failure/cancel branch (R10-2).
- **One normative packet algorithm: preflight → scratch → ReleaseBuffer → commit.** All packet descriptions use release-before-commit ordering. Cleanup `ReleaseBuffer` failure is classified for every early-exit branch: the cleanup HRESULT is logged but the terminal reason is the original cause (R10-3).
- **Packed atomic reason seal: state + sealed-bit + reason in one CAS word.** All updates — `CaptureRequestStop` priority CAS and the capture thread's seal — operate on the same `uint64_t` via `InterlockedCompareExchange64`. No mutex. No post-snapshot CAS race. Private `SEALED` state is never exposed by `CaptureGetResult` (R10-4).
- **Graceful quit is an async state machine.** Waiter requests stop, waits for terminal, drains, releases, posts `WM_APP+CLEANUP_READY`; UI calls `CapDestroy` on its own thread; then posts `WM_QUIT`. `OnQuit` starts the state machine, not immediate `WM_QUIT`. Waiter never calls `CapDestroy`. `WM_QUERYENDSESSION`/`WM_ENDSESSION` call `CaptureRequestStop(shutdown)` before signaling `shutdownEvent` (R10-5).
- **Complete HRESULT/cleanup table covers every COM/WASAPI stage.** From `GetMixFormat` through `GetService`, including activation `E_ACCESSDENIED`, `Initialize`, `GetBufferSize`, `SetEventHandle`, and `Stop` failure. Each row: terminal state/reason/HRESULT, format validity, PCM existence, promotability, `Stop` eligibility, release/free order (including `CoTaskMemFree`), final publisher (R10-6).

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
- **Every async callback holds a strong operation reference** until its return. `CaptureRelease`/`PickerRelease`/destroy drop only the registry reference; the operation destructs only when the last reference (registry or callback) is released. The helper DLL is loaded once and **never unloaded** during the process lifetime (`FreeLibrary` is never called); `CapDestroy` tears down application state only; the module is reclaimed at process exit (R6-1).
- **AccessChanged unsubscribe is handle-safe.** At `CapPermissionSubscribe` time, the helper duplicates Go's `notifyEvent` via `DuplicateHandle`; handlers signal only the duplicate; Go can safely close its original handle immediately after `CapPermissionUnsubscribe` returns (R6-2).
- **Stop-reason arbitration uses atomic priority.** Overflow, discontinuity, and permission_revoke dominate all finalizable reasons regardless of arrival order. Go rechecks final terminal reason AND permission status (must be exactly `Allowed`) before `.partial` → `.wav` promotion; unknown WASAPI errors map to `CAP_REASON_WASAPI_ERROR` (non-promotable), not fake `CAP_REASON_PERMISSION_REVOKE` (R7-2). `AUDCLNT_E_NOT_ALLOWED` removed from the HRESULT table — `0x88890003` is `AUDCLNT_E_WRONG_ENDPOINT_TYPE` (R7-2).
- **PCM sample reads use `memcpy`** (or byte assembly for packed 24-bit), not pointer casts, eliminating unaligned-access and strict-aliasing UB. Signed 24-bit conversion uses safe signed arithmetic on representable values: `int32_t val = (int32_t)u; if (u >= 0x800000u) val -= 0x1000000;` — the first cast is safe because `u <= 0xFFFFFF`; the subtraction is signed arithmetic on `int32_t` values, not the implementation-defined out-of-range unsigned-to-signed cast that R7 correctly identified in the prior `(int32_t)(u - 0x1000000u)` form (R7-1). WAV header is the **selected initial build-time contract**, confirmed or rejected by independent decoder gate before signed hardware scenarios — no artifact promoted using pre-gate assumptions (R7-7).
- **Whole-packet ring preflight** before conversion/copy. If the recording ring lacks room for the entire WASAPI packet, zero frames are written, `ReleaseBuffer` is called, and the session transitions to overflow failure. The prior "copy then check" design could overrun or leave a partial packet (R7-3).
- **WASAPI discontinuity handling.** First packet `DATA_DISCONTINUITY` is accepted (stream start); subsequent discontinuity is terminal `CAP_REASON_DISCONTINUITY` (non-promotable integrity failure). `TIMESTAMP_ERROR` logged but accepted (R7-3).
- **Helper loaded via `kernel32.NewProc("LoadPackagedLibrary")`**, not the non-existent `windows.LoadPackagedLibrary` in `x/sys v0.46.0` (R7-5).
- **`CapInit` initializes the UI-thread WinRT apartment** via `RoInitialize(RO_INIT_SINGLETHREADED)`, balanced by `RoUninitialize` in `CapDestroy` (R7-5).
- **Coherent two-step capture state machine (R12-4).** `CapturePrepare` creates thread and returns immediately; waiter monitors MTA-readiness with 5-second timeout (UI never blocked); `CaptureActivate` launches activation and returns immediately. All async failures (including `CoInitializeEx`) travel through the operation, not return HRESULT; terminal state published only after `CoUninitialize`; capture thread does NOT hold a ref-counted session reference (R8-2); the thread sets `threadDone=1` (atomic release — cleanup complete, one terminal store remains; R10-2), then publishes terminal (atomic release — final session-state access; R10-2), with a seq-cst fence between — any thread observing terminal also observes `threadDone=1` (R9-2); `CaptureGetResult` uses acquire read; registry cannot be dropped until terminal store is observable (R10-2); C++/WinRT async cycle explicitly broken at callback return, not after terminal publication (R10-2 — normal callback never publishes terminal). All terminal publication routes through the capture thread via a composite barrier (R9-1, R10-1), except pre-handoff cancellation where the late callback publishes only after observing `threadDone==1`.
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
  ├── Go calls helper: CapturePrepare(notifyEvent) → opId (R12-4/R15-3)
  │     helper validates inputs, allocates unpublished state/reserves an ID,
  │     then eagerly duplicates notifyEvent for the capture thread before
  │     publishing any operation/thread. Duplicate failure rolls back state/
  │     reservation: no operation, thread, or written opId. On success it
  │     calls _beginthreadex(initflag=0), performs a pre-reserved/no-fail
  │     registry publish, closes the returned kernel thread HANDLE exactly once
  │     (thread lifetime is observed via threadDone), and returns immediately
  │     (activation is deferred to CaptureActivate).
  │
  ├── Helper creates the capture thread (BEFORE activation — R6-4, R12-4)
  │
  ╔══════════════════════════════════════════════════════════════════╗
  ║  Capture thread (helper-owned) — started first                 ║
  ║  Thread does NOT hold a ref-counted session ref (R8-2/R9-2).   ║
  ║  Thread accesses session state via packed atomics (R10-4) and  ║
  ║  handoff mutex only.                                           ║
  ║  The thread receives the already-created captureThreadNotify   ║
  ║  duplicate as its localNotify; it never reads/signals Go's     ║
  ║  original handle and never calls DuplicateHandle itself.       ║
  ║  threadDone=1 (atomic release) means "COM cleanup complete;     ║
  ║  exactly one terminal store remains." After threadDone, the    ║
  ║  thread publishes the terminal store (atomic release — the     ║
  ║  FINAL session-state access), with a seq-cst fence between,    ║
  ║  so any observer of terminal also sees threadDone=1.           ║
  ║                                                                ║
  ║  CoInitializeEx(nullptr, COINIT_MULTITHREADED) — must succeed  ║
  ║  If CoInitializeEx fails:                                      ║
  ║    → store pending failure cause (returned HRESULT)            ║
  ║    → (no separate ready signal — the terminal                 ║
  ║       SetEvent(localNotify) below wakes Go's waiter; R13-2)    ║
  ║    → (no CoUninitialize needed — init failed)                  ║
  ║    → set threadDone=1 (atomic release — cleanup complete;      ║
  ║      one terminal store remains)                               ║
  ║    → atomic_thread_fence(seq_cst)                              ║
  ║    → publish terminal FAILED (atomic release — FINAL           ║
  ║      session-state access)                                     ║
  ║    → SetEvent(localNotify) — Go sees terminal via GetResult    ║
  ║    → CloseHandle(localNotify) — thread-local only              ║
  ║    → thread exits                                              ║
  ║  Release-store session.mtaReady=1; SetEvent(localNotify)       ║
  ║    — CaptureGetResult acquire-loads this session-owned atomic, ║
  ║      copies it into caller-owned format->ready, and the waiter ║
  ║      observes MTA-ready without worker access to caller memory ║
  ║    (state==0 preparing + ready==1); R13-1/R13-2. Signals the  ║
  ║    thread's own duplicate, never Go's original handle.        ║
  ║  WaitForSingleObject(captureThreadWakeEvent) — wait for        ║
  ║  activation handoff or cancellation                            ║
  ╚══════════════════════════════════════════════════════════════════╝
  │
  ├── CapturePrepare returns immediately with opId (R12-4)
  │     Waiter monitors notifyEvent for MTA-ready state.
  │     If timeout (5 seconds, monitored by waiter — R7-4, R12-4):
  │       → waiter calls CaptureRequestStop(opId, cancel)
  │       → capture thread sees cancel, publishes terminal
  │     If capture thread reported CoInitializeEx failure:
  │       → waiter sees terminal via CaptureGetResult, does
  │         not post WM_APP+ACTIVATE_READY
  │     If MTA-ready:
  │       → waiter posts WM_APP+ACTIVATE_READY to UI thread
  │
  ├── UI thread receives WM_APP+ACTIVATE_READY:
  │     Calls CaptureActivate(opId, deviceId) (R12-4/R15-3)
  │     While state is PREPARED, helper eagerly creates callbackNotify.
  │     Duplicate failure returns directly and leaves PREPARED. It then
  │     CASes PREPARED→ACTIVATING. If the CAS loses to stop, it closes
  │     callbackNotify and returns E_NOT_VALID_STATE. Only after winning
  │     the CAS does it call ActivateAudioInterfaceAsync(deviceId,
  │       IID_IAudioClient, ...) with agile completion handler
  │     If ActivateAudioInterfaceAsync returns error HRESULT (R9-1):
  │       → no callback will fire
  │       → close callbackNotify without signaling (exactly once)
  │       → store pending failure HRESULT in session state
  │       → signal captureThreadWakeEvent (thread sees pending
  │         cause, runs CoUninitialize, publishes terminal)
  │       → CaptureActivate returns S_OK (failure is async)
  │     CaptureActivate returns S_OK immediately
  │
  │   ... consent prompt may appear on this UI thread ...
  │
  ╔══════════════════════════════════════════════════════════════════╗
  ║  MTA worker thread (system-owned)                              ║
  ║                                                                ║
  ║  ActivateCompleted(asyncOp) fires:                             ║
  ║    1. Lock helper mutex                                        ║
  ║    2. If state>=STOPPING (R11-2) AND threadDone==1 (R9-1):     ║
  ║         GetActivateResult → release returned interface         ║
  ║         → callback is the LAST entity — publishes terminal     ║
  ║           CANCELLED (atomic release — FINAL session-state      ║
  ║           access)                                              ║
  ║         → SetEvent(callbackNotify); CloseHandle(callbackNotify)║
  ║         → releases strong operation ref → returns              ║
  ║       If state>=STOPPING AND threadDone==0:                    ║
  ║         GetActivateResult → release returned interface         ║
  ║         → store pending cancelled cause                        ║
  ║         → signal captureThreadWakeEvent (thread publishes)     ║
  ║         → CloseHandle(callbackNotify) without signaling        ║
  ║         → releases strong operation ref → returns              ║
  ║    3. GetActivateResult → IAudioClient*                        ║
  ║    4. Store IAudioClient* in handoff slot                       ║
  ║       *** LINEARIZATION POINT: before this write, the callback ║
  ║       owns the IAudioClient*. After this write (under mutex),  ║
  ║       the capture thread owns it exclusively. ***               ║
  ║    5. Store activation HRESULT in result slot                   ║
  ║    6. If GetActivateResult failed (no valid IAudioClient*):    ║
  ║         store null in handoff slot + pending failure cause     ║
  ║    7. Unlock mutex                                              ║
  ║    8. SetEvent(captureThreadWakeEvent) — capture thread wakes   ║
  ║    9. CloseHandle(callbackNotify) without signaling: the       ║
  ║       capture thread owns terminal publication/notification.   ║
  ║   10. Callback releases its strong operation ref (see §Callback║
  ║       strong-reference lifetime)                                ║
  ║   11. Callback returns to the OS                                ║
  ║                                                                ║
  ║  Note (R9-1): the callback NEVER publishes terminal in the     ║
  ║  normal (non-cancelled) path. It always wakes the capture      ║
  ║  thread, which publishes terminal after CoUninitialize. For    ║
  ║  pre-handoff cancel, the callback publishes terminal only when ║
  ║  it observes threadDone==1 (capture thread already exited and  ║
  ║  completed CoUninitialize).                                     ║
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
  ║    2. If handoff slot is null (pending cause present):          ║
  ║       → go to step 11b (cleanup and terminal via barrier)      ║
  ║   2a. Before each of steps 3/4/6/7/8, acquire-load packed      ║
  ║       state. If STOPPING/SEALED won, skip remaining startup    ║
  ║       calls and go to cleanup with the resources already owned.║
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
  ║   8a. CAS ACTIVATING→CAPTURING only after Start succeeds.      ║
  ║       If a concurrent stop already won, CAS fails and cleanup  ║
  ║       runs immediately; public capturing is never exposed.     ║
  ║    9. SetEvent(localNotify) — Go queries format via            ║
  ║       CaptureGetResult (R13-2: signals the thread's OWN         ║
  ║       duplicate, never Go's original notifyEvent handle)       ║
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
  ║             if AUDCLNT_BUFFERFLAGS_SILENT:                     ║
  ║               fill scratch buffer with zeros                   ║
  ║             else: convert PCM → float32 into scratch buffer    ║
  ║             ReleaseBuffer(frames) — release BEFORE commit      ║
  ║             copy scratch to ring (commit producer index)       ║
  ║           }                                                    ║
  ║         SetEvent(localNotify) — Go reads via CaptureRead       ║
  ║           (R13-2: thread's own duplicate, not Go's handle)     ║
  ║   11. On stop signal or pending failure cause:                   ║
  ║         Packed CAS: seal reason + set SEALED state (R10-4).    ║
  ║           SEALED is private — CaptureGetResult maps it to the  ║
  ║           last public state. The packed CAS captures the       ║
  ║           latest priority reason. CaptureRequestStop must      ║
  ║           update before this CAS or observe SEALED and no-op.  ║
  ║         IAudioClient::Stop()                                   ║
  ║         IAudioCaptureClient::Release()  ← same thread          ║
  ║         IAudioClient::Release()         ← same thread          ║
  ║         CoUninitialize() — BEFORE threadDone (R7-4)            ║
  ║         Set threadDone=1 (atomic release) — COM cleanup         ║
  ║           complete; exactly one terminal store remains (R10-2) ║
  ║         atomic_thread_fence(memory_order_seq_cst) (R9-2)       ║
  ║         Publish terminal state (atomic release) — FINAL        ║
  ║           session-state access (R10-2). Any observer of        ║
  ║           terminal also sees threadDone=1 due to fence.        ║
  ║           CaptureGetResult uses acquire read; registry cannot  ║
  ║           be dropped until this store is fully observable.     ║
  ║         SetEvent(localNotify) — thread-local duplicated handle ║
  ║           only; no session-state access (R9-2)                  ║
  ║         CloseHandle(localNotify)                                ║
  ║         Thread exits                                            ║
  ║         Note: threadDone=1 means "cleanup complete; exactly    ║
  ║         one terminal store remains." The terminal store IS     ║
  ║         the final session-state access. After terminal is      ║
  ║         published, the thread touches only local-stack handles.║
  ║         CaptureRelease drops only the registry ref. The        ║
  ║         operation destructor checks threadDone (never joins     ║
  ║         the thread — R8-2). The DLL is process-lifetime loaded ║
  ║         so no module-unload join is needed.                     ║
  ║                                                                ║
  ║   11b. On pending failure (timeout, sync launch failure,       ║
  ║         async activation failure with null handoff — R9-1):    ║
  ║         Same cleanup: CoUninitialize → threadDone → fence →   ║
  ║         terminal → SetEvent(localNotify). The pending cause    ║
  ║         stored by UI thread or callback becomes the terminal   ║
  ║         HRESULT and reason.                                     ║
  ╚══════════════════════════════════════════════════════════════════╝
```

### Why the capture thread starts before activation

*Addresses R6 finding 4. Coherent state machine per R7 finding 4.*

If the capture thread were created after activation completes (as in revisions 1–6), a `CoInitializeEx` failure on the capture thread would leave an `IAudioClient*` in the handoff slot with no thread to release it. By starting the capture thread first and proving `CoInitializeEx` succeeds before launching `ActivateAudioInterfaceAsync`, the handoff slot is guaranteed to have a ready consumer. If `CoInitializeEx` fails, the capture thread publishes the failure through terminal state plus its duplicated notification event, not as a `CapturePrepare` return HRESULT and not through Go's original handle value.

**Two-step `CapturePrepare`/`CaptureActivate`** (R12-4): `CapturePrepare` creates the operation and capture thread via `_beginthreadex`, preserves a creator hold across possible early execution, publishes through the pre-reserved registry slot, closes the launch handle, writes `opId`, and returns immediately. The capture thread calls `CoInitializeEx(MTA)` and signals its pre-created `localNotify` duplicate to report MTA-ready or failure; this signals the shared kernel event without using Go's original handle value. The waiter observes readiness (via `CaptureGetResult` state query after wake), and if MTA-ready, posts a stable operation ID to the UI thread via `PostMessageW(WM_APP+ACTIVATE_READY, opId, 0)`. The UI thread handler calls `CaptureActivate(opId, deviceId)`, which launches `ActivateAudioInterfaceAsync` and returns immediately. **No UI-thread export waits for thread readiness** — `CapturePrepare` returns instantly, and `CaptureActivate` does not block.

**MTA-readiness timeout** (R7-4, R10-1, R12-4): the **waiter** (not the UI thread) monitors the `notifyEvent` for MTA-ready. If the capture thread does not signal MTA-ready within 5 seconds (pathological system state), the waiter calls `CaptureRequestStop(opId, cancel)` to cancel the preparation. The capture thread sees the cancel and publishes terminal after cleanup. The UI thread is never blocked. In practice, `CoInitializeEx` completes in microseconds; the timeout is a safety net.

**`CaptureActivate` failures** (R12-4): if `ActivateAudioInterfaceAsync` returns an error HRESULT synchronously, no callback will run, so `CaptureActivate` first closes `callbackNotify` without signaling, then stores the pending failure in the session state and signals `captureThreadWakeEvent`. The capture thread wakes, sees the pending cause, and publishes terminal after cleanup: `CoUninitialize` → `threadDone` → fence → terminal → `SetEvent(localNotify)` → `CloseHandle(localNotify)`. `CaptureActivate` returns `S_OK` — the failure is an operation outcome queried through `CaptureGetResult`. The UI thread neither publishes terminal nor signals/closes Go's original event (R10-1).

**Linearization point**: the mutex-protected write to the handoff slot (callback step 4) is the linearization point for `IAudioClient*` ownership transfer. Before this write, the callback owns the pointer (via `GetActivateResult`). After this write, the capture thread owns it exclusively. The mutex provides the memory-ordering guarantee.

### `CapturePrepare`/`CaptureActivate` executable branch table

*Addresses R8 finding 1, R12-4. Replaces contradictory prose with one authoritative table. Two-step handshake: `CapturePrepare` creates the operation and thread; `CaptureActivate` launches activation.*

The rule has two layers. `CapturePrepare` failures before publication return
directly and write no `opId`. After `CapturePrepare` publishes an operation,
worker/activation outcomes are queried asynchronously, but later API calls may
still return **call-level** validation/handle/CAS errors directly without
changing the operation. In particular, `CaptureActivate` can return
`E_POINTER`, `E_NOT_VALID_STATE`, or a `DuplicateHandle` failure while the
existing session remains retryable/cancellable. A synchronous
`ActivateAudioInterfaceAsync` launch failure occurs only after the
`PREPARED→ACTIVATING` CAS and is therefore stored as an operation outcome;
`CaptureActivate` returns `S_OK` for that row.

| Branch / linearization point | Call HRESULT | Existing `opId` / private→public state | Registry | Duplicates: capture / callback | Callback | Wake / terminal publisher | Cleanup owner |
|---|---|---|---|---|---|---|---|
| `CapturePrepare`: null output/event, invalid handle | `E_POINTER` / `E_HANDLE` | No ID | None | 0 / 0 | No | None | Caller |
| `CapturePrepare`: `DuplicateHandle` failure | failure HRESULT | No ID | None | 0 / 0 | No | None | Caller; no operation/thread exists |
| `CapturePrepare`: ID exhaustion/allocation failure | `E_OUTOFMEMORY` | No ID | None | 0 / 0 (allocation/ID reservation precede duplication) | No | None | UI helper path |
| `CapturePrepare`: `_beginthreadex` returns 0 | mapped CRT launch HRESULT | No ID | None | capture duplicate closed once / 0 | No | None | UI helper frees creator-held unpublished session |
| Capture thread `CoInitializeEx` fails | `CapturePrepare` already returned `S_OK` | ID exists; `PREPARING(0)` → `STOPPING/SEALED` → terminal `failed(4)` | Active → terminal, then releasable | signal+close once / 0 | No | capture duplicate only; capture thread publishes after `threadDone` fence | Capture thread; no `CoUninitialize` after failed init |
| Five-second MTA-readiness timeout | `CaptureRequestStop` returns `S_OK` | ID exists; `PREPARING/PREPARED(0)` → `STOPPING` → `cancelled(5)` | Active → terminal | capture: signal terminal+close / 0 | No activation callback launched | `captureThreadWakeEvent`; capture thread publishes | Capture thread calls `CoUninitialize` iff initialized |
| `CaptureActivate`: null device or observed state not `PREPARED` | `E_POINTER` / `E_NOT_VALID_STATE` | ID exists; state unchanged | Active | existing capture duplicate / 0 | No | None | Caller; session remains retryable iff still `PREPARED` |
| `CaptureActivate`: `DuplicateHandle` failure | failure HRESULT | ID exists; remains `PREPARED(1)` → public `preparing(0)`, `ready=1` | Active | existing / 0 | No | None | Caller; retry or cancel |
| Callback duplicate created, then `PREPARED→ACTIVATING` CAS loses to stop | `E_NOT_VALID_STATE` | ID exists; winning stop state retained | Active | existing / callback duplicate close-without-signal once | No | stop path's state-specific wake | Stop winner/capture thread; activation not launched |
| CAS wins, synchronous `ActivateAudioInterfaceAsync` launch fails | `CaptureActivate` returns `S_OK`; HRESULT stored as outcome | ID exists; `ACTIVATING(2)` → public `activating(1)` → terminal failure | Active → terminal | capture: signal terminal+close; callback: close-without-signal once | No callback | `captureThreadWakeEvent`; capture thread publishes | UI stores cause and closes callback duplicate; capture thread cleans COM apartment |
| Normal callback handoff, then successful `Start` | `S_OK` operation outcome | Callback leaves `ACTIVATING(2)`; capture thread CASes `ACTIVATING→CAPTURING(3)` only after `Start` succeeds → public `capturing(2)` | Active | capture retained; callback close-without-signal once | Yes | `captureThreadWakeEvent`; capture thread later signals its duplicate | Callback transfers `IAudioClient`; capture thread owns initialization/start/cleanup |
| Async callback `GetActivateResult` failure | stored HRESULT | ID exists; `ACTIVATING(2)` → terminal `failed(4)` | Active → terminal | capture: signal terminal+close; callback: close-without-signal once | Yes | `captureThreadWakeEvent`; capture thread publishes | Callback releases partial interface/stores cause; capture thread cleans apartment |
| Pre-handoff cancel, Diagram A (`threadDone==1` before callback) | `CaptureRequestStop` returns `S_OK` | ID exists; `ACTIVATING(2)` → `STOPPING/SEALED` → `cancelled(5)` | Active → terminal | capture close-without-signal once; callback signal+close once | Late callback required | callback duplicate; callback publishes | Thread uninitializes/exits; callback releases activation result |
| Pre-handoff cancel, Diagram B (callback first) | `CaptureRequestStop` returns `S_OK` | Same mapping | Active → terminal | capture signal terminal+close once; callback close-without-signal once | Callback stores cause | `captureThreadWakeEvent`; capture thread publishes | Callback releases result; capture thread cleans apartment |
| Stop after handoff / normal lifecycle stop | `S_OK` | ID exists; `CAPTURING(3)` → `STOPPING/SEALED` → terminal by reason | Active → terminal | capture signal terminal+close; callback already closed | Completed | `stopEvent`; capture thread publishes | Normative cleanup path |
| Capture-loop failure | operation outcome | Same transition to terminal `failed(4)` except finalizable device loss | Active → terminal | capture signal terminal+close; callback already closed | Completed | capture duplicate; capture thread publishes | Acquired packet release → internal-failure CAS → normative cleanup |

**Signal/close cardinality generated from the table.** Each duplicate is
created once and closed once. `callbackNotify` signals exactly once only in
Diagram A; every other callback/launch/CAS branch closes it with zero signals.
The capture-thread duplicate is a readiness-hint channel, so its total signal
count is intentionally not fixed: zero or one MTA-ready signal, zero or one
capture-start/format signal, zero or more coalescible data signals, and exactly
one terminal signal when the thread is publisher. Diagram A is the only
capture-thread path that closes without terminal signal; if MTA-ready was
already published it may have one earlier readiness signal. No count refers to
or mutates Go's original handle.

The helper is linked `/MT` and its capture routine uses C++/WinRT/CRT-owned
objects, so it launches with `_beginthreadex`, not raw `CreateThread`.
Microsoft explicitly recommends the CRT launcher for a thread that calls the
CRT; `_beginthreadex` returns 0 on failure and a Win32 thread handle on success,
and a normal return from the start routine performs the matching CRT thread
cleanup `[MS-50]`. The routine returns normally; it never calls `ExitThread`.

The successful `_beginthreadex` handle is a third, short-lived launch artifact,
not a notification duplicate and not the thread's lifetime owner. The UI helper
closes it exactly once immediately after the pre-reserved/no-fail registry
publication; `CloseHandle` does not terminate the thread `[MS-50]`. Because a
runnable thread may execute before the launcher returns, `CapturePrepare` keeps
a local creator `shared_ptr` from allocation through registry publication and
`*opId` write. The raw pointer passed to the thread therefore cannot dangle even
if the thread reaches terminal before publication; after publication the
registry reference owns the state. Launch failure closes the capture duplicate,
releases the ID reservation/creator hold, writes no `opId`, and has no valid
thread handle to close.

The launch error mapping is frozen: `_beginthreadex==0` snapshots `errno` and
`_doserrno` immediately; a nonzero `_doserrno` becomes
`HRESULT_FROM_WIN32(_doserrno)`, otherwise `EINVAL→E_INVALIDARG`,
`EACCES→E_ACCESSDENIED`, `EAGAIN/ENOMEM→E_OUTOFMEMORY`, and any other value
becomes `E_FAIL`. Tests inject every mapping, early thread completion before
registry publication, one close per successful launch, and zero outstanding
thread handles.

**Cancelled activation is not immediately terminal (R9-1, R10-1, R10-2, R10-4, R11-2, R12-1 composite barrier).** When `CaptureRequestStop(cancel)` is called while activation is in flight, the packed CAS installs `state=STOPPING` + `reason=CANCEL` (R11-2, R12-1: cancellation is `STOPPING`+`CANCEL`, not a separate bit) and signals `captureThreadWakeEvent` (R11-2: wake event for `ACTIVATING` state). The two possible orderings are frozen in §Cancellation before activation completes Diagram A (callback after `threadDone` — typical) and Diagram B (callback before `threadDone` — rare). In both cases, `CaptureGetResult` (acquire read) returns the stored `lastPublicState` (R11-2) until the final terminal store is published, preventing Go from calling `CaptureRelease` before all cleanup is complete. The callback's strong reference keeps the operation state alive until both paths have completed.

**Other ID-creating exports follow the publication half of this rule**:
`PickerOpenFile`, `CapPermissionRequest`, `CapEnumerateDevices`, and
`CapGetDefaultDevice` return direct failures before an ID/registry entry exists;
after publication their worker results are queried. `CaptureActivate` is not an
ID-creating export — it operates on the existing capture ID and retains the
explicit direct call-level exceptions in the table above.

### Why this handoff is legal

- Both the MTA callback thread and the helper-owned capture thread are in the MTA. Microsoft explicitly documents that "all the threads in the process that have been initialized as free-threaded reside in a single apartment" and that "interface pointers are passed directly from thread to thread within a multithreaded apartment." `[MS-42]` The `IAudioClient*` pointer stored in the handoff slot is protected by a mutex (memory ordering), and used exclusively by the capture thread after handoff.
- `GetMixFormat`, `Initialize`, `GetService`, `Start`, `Stop`, and `Release` are all called on the same capture thread. `GetMixFormat` runs before `Initialize` because its returned format is the format passed to shared-mode `Initialize` `[MS-38]`. The documented same-thread release rule for `IAudioCaptureClient` and `IAudioClient` is satisfied. `[MS-17] [MS-34]`
- The completion handler COM object is reference-counted. Windows holds a reference until the operation completes. The helper holds its own reference and releases it only after the callback has fired and the handoff is complete. `[MS-5]`
- The capture thread calls `CoInitializeEx(nullptr, COINIT_MULTITHREADED)` before touching any COM pointer. If this fails (returns anything other than `S_OK` or `S_FALSE`), the session fails immediately.

### Normative cleanup path (R11-1)

*Addresses R11 finding 1. Single authoritative cleanup sequence — all diagrams, branch tables, overflow/error prose, two-phase lifetime text, and tests reference this path. No competing algorithms.*

Every capture-thread terminal path, regardless of the stop source (user stop, lifecycle, cancel, overflow, discontinuity, format error, WASAPI error, permission revoke, device lost), executes the following steps in order. Steps are skipped when the corresponding resource was never acquired (governed by ownership flags — R11-4).

1. **Release acquired WASAPI packet** (if a buffer is currently acquired): `ReleaseBuffer(frames)`. Cleanup `ReleaseBuffer` HRESULT is logged; the terminal reason is the original cause, not the cleanup failure.
2. **Reach `STOPPING` state**: either already `STOPPING` (from external `CaptureRequestStop`) or via internal-failure CAS (R11-2: `CAPTURING`→`STOPPING` + failure reason, priority-merged with any concurrent external stop).
3. **Packed CAS seal**: `STOPPING`→`SEALED` + `sealed=1` + snapshot reason. On CAS failure (a higher-priority reason was installed), retry captures the latest value.
4. **Call the one cleanup function** (R11-4, R12-5): the function consumes and clears each ownership flag exactly once in order: `started` → `Stop`, `serviceAcquired` → `IAudioCaptureClient::Release`, `mixFormatOwned` → `CoTaskMemFree(pMixFormat)`, `audioClientOwned` → `IAudioClient::Release`. Each step is skipped if its flag is not set. `Stop` HRESULT is logged but does not override the terminal reason (preserve-original — R12-5). See §Complete HRESULT/cleanup table for the flag definitions.
5. **`CoUninitialize()`** — all COM work complete. Skipped only if `CoInitializeEx` failed.
6. **`threadDone=1`** (atomic release) — cleanup complete; exactly one terminal store remains (R10-2).
7. **`atomic_thread_fence(memory_order_seq_cst)`** — ensures any observer of the terminal store also sees `threadDone=1`.
8. **Publish terminal state** (atomic release) — the FINAL session-state access. `CaptureGetResult` acquire-reads this store.
9. **`SetEvent(localNotify)`** — thread-local duplicate of Go's notification handle (R9-2). Go wakes and queries terminal via `CaptureGetResult`.
10. **`CloseHandle(localNotify)`** — thread-local only; no session-state access after this point.
11. Thread exits.

**Pre-handoff cancellation exception** (R9-1, R12-1, R15-3): when
`CaptureRequestStop(cancel)` arrives during activation before handoff, Diagram A
has the capture thread execute step 5 (`CoUninitialize`) and step 6
(`threadDone=1`), then **close its capture-thread duplicate without signaling**
and exit. It does not execute steps 7–11. The late callback is sole terminal
publisher: after observing `threadDone==1`, it publishes terminal, signals and
closes its distinct callback duplicate, releases its strong ref, and returns.
If the callback fires first (Diagram B), it stores the pending cause, closes its
callback duplicate without signaling, and wakes the capture thread; the thread
then executes steps 5–11, including signal+close of its own duplicate. Thus
both diagrams close both duplicates exactly once.

**Static consistency grep fixture** (R11-1, R12-6): the implementation must include a build-time or test-time grep/regex that asserts:
- No occurrence of `threadDone` described as "final session-state access" or "last session access" or "final instruction" (the terminal store is the final access; `threadDone` means "cleanup complete, one terminal store remains").
- No standalone terminal-publication sequence that omits any of: seal (step 3), `threadDone` (step 6), fence (step 7), terminal store (step 8), `localNotify` (step 9).
- No cleanup path that calls `Stop` without checking `started`, releases `IAudioCaptureClient` without checking `serviceAcquired`, releases `IAudioClient` without checking `audioClientOwned` (R12-5), or calls `CoTaskMemFree(pMixFormat)` without checking `mixFormatOwned` (R12-5).
- No reference to a `cancelled` bit (R12-1: cancellation is `state=STOPPING` + `reason=CANCEL`).

**Static grep fixture self-check against this note** (R12-6, R13-6): the naive
`grep -c 'cancelled bit'` returns a non-zero count because the phrase survives
legitimately in *negations* ("no separate cancelled bit") and *history*
("paths that previously read or wrote 'the cancelled bit' now read
`state>=STOPPING`"). A count of the literal phrase is therefore NOT the check —
R12/R13 correctly flagged the earlier "→ 0" claims as false. The checker must
target the **normative anti-pattern** and exclude negation/history/self-check
lines. The following commands are run against the authoritative note and their
real output recorded in the outcome; the fixture FAILS the build if any returns
a non-empty result:

1. Normative cancelled-bit usage (a live bit being read/written), excluding
   negations, history, and this fixture's own text:
   ```
   grep -nE 'cancelled bit' note.md \
     | grep -vE 'no separate|no such|not a separate|previously read or wrote|reading/writing|No reference|grep -c|grep -nE'
   ```
   → **empty** (verified 2026-07-13: all four literal occurrences are the two
   negation lines, one history line, and the fixture description itself; no
   normative usage remains).
2. `threadDone` mislabeled as the final access. The *correct* phrase
   "publish terminal (atomic release — final session-state access)" is expected
   and must be excluded; the anti-pattern is `threadDone` itself being called
   the final access/instruction. Match only that:
   ```
   grep -nE 'threadDone[^.]*\b(is|as)\b[^.]*final (session-state access|instruction)|threadDone[^.]*final instruction' note.md \
     | grep -vE 'terminal \(atomic release — final session-state access\)|grep -nE|described as'
   ```
   → **empty** (verified 2026-07-13: `threadDone` is everywhere defined as
   "cleanup complete; one terminal store remains"; only the terminal store —
   never `threadDone` — is labeled the final session-state access. A naive grep
   that matches any line containing both `threadDone` and "final session-state
   access" produces false positives on the *correct* terminal-store lines and
   must NOT be used.).
3. Any "tied at priority" claim outside this fixture's own description:
   ```
   grep -nE 'tied at priority' note.md | grep -vE 'grep -c|grep -nE|Any .tied at priority'
   ```
   → **empty** (verified 2026-07-13: the canonical table assigns one rank per
   reason with a single documented wasapi_error/format_error tie resolved by
   first-sealed; no "tied at priority 1" claim survives).

These three commands, plus the four normative-anti-pattern asserts above
(no `threadDone`-as-final-access, no seal/`threadDone`/fence/terminal/`localNotify`
omission, no unguarded cleanup call, no normative `cancelled` bit), are the
frozen static fixture. Their real observed output is reproduced in the outcome
resource so the reviewer can re-run them without trusting a summary claim.

### Cancellation before activation completes

There is no `Cancel` API on `IActivateAudioInterfaceAsyncOperation`. The two frozen cancellation diagrams (R12-1):

**Common preamble (both diagrams):**

1. Go calls `CaptureRequestStop(opId, reason=cancel)` while activation is in flight.
2. `CaptureRequestStop` packed CAS installs `state=STOPPING` + `reason=CANCEL` (R11-2: cancellation is `STOPPING`+`CANCEL` in the packed word, not a separate bit; R12-1: no `cancelled` bit exists). The CAS also stores `lastPublicState=ACTIVATING` (R11-2).
3. Helper signals `captureThreadWakeEvent` (R11-2: wake event for `ACTIVATING` source state).

**Diagram A: callback fires after `threadDone==1` (typical — capture thread exits first):**

4. Capture thread wakes, sees `state>=STOPPING` and no handoff (null slot), calls `CoUninitialize`, sets `threadDone=1` (atomic release — cleanup complete, one terminal store remains; R10-2), **closes its own `localNotify` duplicate** (`CloseHandle` — pre-created by `CapturePrepare`; the callback is the publisher here, so the thread never signals it but must still close it to avoid a handle leak — R13-2), and exits. The thread does NOT publish terminal and does NOT signal `localNotify`.
5. The MTA callback fires (guaranteed by the OS).
6. Callback reads packed atomic state (acquire), sees `state>=STOPPING` + `reason==CANCEL` (R11-2, R12-1).
7. Calls `GetActivateResult`, releases the returned `IAudioClient*` immediately (on the callback's MTA thread — legal since `Initialize`/`GetService` were never called), clears its stored async-operation reference (R10-2 cycle break).
8. Callback checks `threadDone` (acquire read) → `threadDone==1`.
9. **Callback is the sole terminal publisher.** It publishes terminal `cancelled`(5) (atomic release — final session-state access; R10-2, R12-1). Terminal HRESULT is `HRESULT_FROM_WIN32(ERROR_CANCELLED)`. Terminal reason is `CAP_REASON_CANCEL`.
10. Callback signals its distinct **`callbackNotify` duplicate** (created eagerly in `CaptureActivate` before activation was launched — R14-2, not lazily).
11. Callback closes its callback duplicate (`CloseHandle`) — exactly-once close on this path.
12. Callback releases its strong operation reference (see §Callback strong-reference lifetime).
13. Callback returns to the OS.

Go sees terminal via `CaptureGetResult` acquire read.

**Diagram B: callback fires before `threadDone==1` (rare — callback fires while capture thread is still cleaning up):**

4. The MTA callback fires before the capture thread has exited.
5. Callback reads packed atomic state (acquire), sees `state>=STOPPING` + `reason==CANCEL`.
6. Calls `GetActivateResult`, releases the returned `IAudioClient*`, clears stored async-operation reference (R10-2 cycle break).
7. Callback checks `threadDone` (acquire read) → `threadDone==0`.
8. **Callback is NOT the terminal publisher.** It stores the pending cancel cause (HRESULT + reason) in the session state and signals `captureThreadWakeEvent`.
9. Callback **closes its callback duplicate WITHOUT signaling it** (created eagerly in `CaptureActivate` — R14-2; exactly-once close on this path), then releases its strong operation reference and returns. The capture thread is the terminal publisher here and signals **its own** duplicate (step 10), so the callback's duplicate is unused on this path but is still closed to avoid a leak.
10. Capture thread finishes its cleanup (`CoUninitialize`), sets `threadDone=1` (atomic release), reads the pending cause, executes `atomic_thread_fence(seq_cst)`, publishes terminal `cancelled`(5) (atomic release — final session-state access), signals its own `localNotify`, closes `localNotify`, and exits.

Go sees terminal via `CaptureGetResult` acquire read.

**Notification handle ownership (R12-1, R13-2):** each entity that may signal `localNotify` owns a **separate** duplicate, and **each duplicate is closed by exactly one owner on every path**, so no handle leaks in either cancellation ordering:

Both duplicates are **created eagerly, before the operation they belong to can
signal** (R14-2 — never lazily at terminal publication, which would have an
unhandled `DuplicateHandle`-failure branch where the callback publishes terminal
but fails to wake the sole waiter):

- **Capture thread duplicate** — created via `DuplicateHandle` in `CapturePrepare`, **before** the operation and capture thread are published. If the `DuplicateHandle` fails, `CapturePrepare` returns the failure HRESULT, creates **no** operation and **no** thread, and does not write `*opId` (R14-2). On success, the duplicate is closed by the capture thread immediately before it exits, on **every** path: normal terminal (step 10 of §Normative cleanup path, after signaling it), and Diagram A pre-handoff cancel (the thread closes it without signaling — Diagram A step 4). Exactly-once close; never leaked.
- **Callback duplicate** — created via `DuplicateHandle` in `CaptureActivate`, **before** `ActivateAudioInterfaceAsync` is launched. If the `DuplicateHandle` fails, `CaptureActivate` returns the failure HRESULT, launches **no** activation, and leaves the packed state at `PREPARED` (retryable or cancellable — R14-2); no duplicate exists to leak. On success, the callback closes this duplicate **exactly once** on whichever callback path runs: Diagram A (sole publisher) signals then closes (steps 10–11); Diagram B (not publisher), the normal successful handoff, and the async-activation-failure handoff all **close without signaling** (the capture thread publishes and signals its own duplicate). Because the duplicate is created unconditionally in `CaptureActivate` and closed on every callback path, it can neither leak nor be missing when the callback needs to signal.

The two duplicates are independent kernel objects — closing one does not affect the other. Go's original `notifyEvent` handle is **never** signaled or closed by either helper entity; only Go closes it (after `CaptureRelease`). This is the property that makes it safe for Go to close its handle without racing a worker's `SetEvent` (R9-2).

**After signaling, the callback releases its strong operation reference** (see §Callback strong-reference lifetime). Only after this release can the operation's destructor fire.

The operation destructor sees `threadDone=1` (never joins the thread — R8-2). In the cancel case, the thread has typically already exited (Diagram A step 4) and `threadDone=1`.

### Shutdown without UI-thread deadlock

`CaptureRequestStop` is non-blocking: its packed CAS installs/updates the stop
reason, then signals the wake selected by the source state. `PREPARING`,
`PREPARED`, and `ACTIVATING` signal `captureThreadWakeEvent`; `CAPTURING`
signals `stopEvent`; a priority update in `STOPPING` needs no second wake. The
capture thread (or the callback in Diagram A) handles cleanup. The UI thread
never joins it and continues pumping messages. Terminal state is communicated
through the publisher-owned duplicate and `CaptureGetResult`.

---

## Callback strong-reference lifetime

*Addresses R5 finding 1.*

### Problem

The R4 design said "No ref-counted session lifetime is needed" because `CapDestroy` requires all operations to be terminal. But the race is not at `CapDestroy` — it is at `CaptureRelease` / `PickerRelease` / `CapPermissionRequestRelease`. The activation-cancel path sets terminal state and signals Go **before the callback has returned**; Go can immediately call `CaptureRelease` and free the session while callback code (after the `SetEvent` call but before `return`) is still executing on the system MTA thread. The same race exists for picker completion, permission-request completion, enumeration/default-device completions, and `AccessChanged` racing `CapPermissionUnsubscribe`.

### Frozen contract

Every async callback and event handler in the helper holds a **strong reference** (C++ `shared_ptr` or explicit ref-count increment) to its owning operation's state for the entire duration of the callback — from entry to final return/destructor. The operation state is ref-counted with at least two reference holders:

1. **Registry reference**: held by the helper's operation registry (the map keyed by operation ID). Dropped by `CaptureRelease` / `PickerRelease` / `CapPermissionRequestRelease` / `CapEnumerateDevicesRelease` / `CapGetDefaultDeviceRelease`.
2. **Callback reference**: held by each in-flight callback or event handler. Acquired before the callback is registered with the OS/WinRT; released as the callback's final action before returning.

The operation's destructor (which frees memory, closes internal handles, etc.) runs only when the reference count reaches zero — i.e., when **both** the registry has released and **all** callbacks have returned. The destructor never joins threads (R8-2): it checks `threadDone` (an atomic flag set by the capture thread when COM cleanup is complete and exactly one terminal store remains — R10-2, R12-1), but never waits on it — if `threadDone` is not set, `CapDestroy` would have rejected the call. Since the DLL is process-lifetime loaded, no module-unload join is needed.

### Exact ordering per callback type

#### Activation (`ActivateCompleted`)

*Updated for R9-1 composite barrier and R9-2 threadDone ordering.*

**Normal path (activation succeeds):**

1. Callback acquires strong ref at entry (already held from registration).
2. Callback calls `GetActivateResult`, obtains `IAudioClient*`.
3. Callback stores handoff in the mutex-protected slot and clears the stored async-operation reference.
4. Callback signals `captureThreadWakeEvent`, closes `callbackNotify` without signaling it, releases its strong ref, and returns.
5. Capture thread wakes, takes handoff, runs capture loop, then publishes terminal through its capture-thread duplicate.
6. After terminal, Go wakes and calls `CaptureRelease`; the callback ref is already gone, so the registry drop may destroy the state only after the thread barrier permits it.

**Cancel path (composite barrier — R9-1):**

1. Callback acquires strong ref at entry.
2. Callback reads packed atomic state (acquire), sees `state>=STOPPING` and `reason==CANCEL` (R11-2, R12-1 — no separate cancelled bit).
3. Callback calls `GetActivateResult`, releases returned `IAudioClient*`, clears stored async-operation reference (R10-2 cycle break).
4. Callback checks `threadDone` (atomic acquire):
   - `threadDone==1` (typical): callback publishes terminal `CANCELLED`, signals and closes `callbackNotify`, releases its strong ref, returns.
   - `threadDone==0` (rare): callback stores pending cause, signals `captureThreadWakeEvent`, closes `callbackNotify` without signaling, releases its strong ref, returns. Capture thread publishes terminal through its own duplicate.
5. Go sees terminal, calls `CaptureRelease`.
6. `CaptureRelease` drops registry ref. If callback strong ref already released: ref-count zero, destructor fires. If callback still executing (race): destructor fires when callback releases.

**Failure path (GetActivateResult fails):**

1. Callback acquires strong ref at entry.
2. Callback calls `GetActivateResult` → fails. Releases any partial interface.
3. Callback stores pending failure cause, clears the stored async-operation reference, and signals `captureThreadWakeEvent` (the callback does not publish terminal).
4. Callback closes `callbackNotify` without signaling, releases its strong ref, and returns.
5. Capture thread wakes, sees null handoff + pending failure, runs cleanup, and publishes terminal through its own duplicate.
6. Go sees terminal, calls `CaptureRelease`, destructor fires when ref-count reaches zero.

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
| `IActivateAudioInterfaceCompletionHandler` impl | `CaptureActivate` (at `ActivateAudioInterfaceAsync` call) | Documented: "until the activation operation completes and the `IActivateAudioInterfaceAsyncOperation` interface is released" `[MS-5]` | After callback returns — part of operation state destructor |
| `IActivateAudioInterfaceAsyncOperation` | OS (returned by `ActivateAudioInterfaceAsync`) | Application owns the reference | Release is part of activation cleanup (callback or capture thread) |
| C++/WinRT `FileOpenPicker` completion delegate | `PickerOpenFile` | Until the WinRT async operation completes | Part of picker operation state destructor |
| C++/WinRT `RequestAccessAsync` completion delegate | `CapPermissionRequest` | Until the WinRT async operation completes | Part of permission-request operation state destructor |
| C++/WinRT `FindAllAsync` completion delegate | `CapEnumerateDevices` | Until the WinRT async operation completes | Part of enumeration operation state destructor |
| C++/WinRT `AccessChanged` event delegate | `CapPermissionSubscribe` | Until token revocation + in-flight dispatch completes | Subscription state destructor (see §AccessChanged unsubscribe fence) |

**Synchronous launch failure** (R10-1): if `ActivateAudioInterfaceAsync` returns an error HRESULT synchronously (before any callback fires), no activation callback will consume its duplicate. For capture operations (where an `opId` is already published), the UI thread closes `callbackNotify` without signaling, stores the failure HRESULT as a pending cause, and signals `captureThreadWakeEvent`; the capture thread publishes terminal after its own cleanup (`CoUninitialize` → `threadDone` → fence → terminal → signal+close `localNotify`). The UI thread does NOT transition to `FAILED` directly — `CaptureGetResult` remains nonterminal until the capture thread publishes. For non-capture WinRT operations (`PickSingleFileAsync`, `RequestAccessAsync`, etc.) where no helper-owned cleanup thread exists, a synchronous launch failure transitions to `FAILED` immediately — these operations have no `CoUninitialize` / `threadDone` barrier.

**Cycle breaking** (R7-4, R10-2): C++/WinRT async operations hold a reference to the completion delegate, and the delegate captures a `shared_ptr` to the operation state. This creates a cycle: async operation → delegate → state → (may hold) async operation. The cycle is explicitly broken at callback return — not after publishing terminal, since the normal (successful activation) callback never publishes terminal. The exact clearing point for each branch:
- **Normal activation (success)**: callback calls `GetActivateResult`, writes `IAudioClient*` to the handoff slot under mutex, clears its stored `IActivateAudioInterfaceAsyncOperation` reference, signals `captureThreadWakeEvent`, closes `callbackNotify` without signaling, releases its strong operation reference, and returns. The capture thread is the eventual terminal publisher.
- **Activation failure**: callback calls `GetActivateResult` (fails), releases any returned interface, stores the pending cause, clears its stored async-operation reference, signals `captureThreadWakeEvent` (null handoff), closes `callbackNotify` without signaling, releases its strong reference, and returns. The capture thread publishes terminal.
- **Cancel before callback**: callback calls `GetActivateResult`, releases any returned interface, and clears the async-operation reference. If `threadDone==1`, it writes the exact cancel result fields, publishes terminal, signal+closes `callbackNotify`, releases its strong ref, and returns. If `threadDone==0`, it stores the pending cancel cause, signals `captureThreadWakeEvent`, closes `callbackNotify` without signaling, releases its strong ref, and returns; the capture thread publishes through its own duplicate.
In every branch, the stored async-operation reference is cleared at callback return (after `GetActivateResult` and handoff/pending-cause store), breaking the cycle regardless of which thread eventually publishes terminal. For `AccessChanged`, the `AppCapability` → delegate cycle is broken at token revocation (see §AccessChanged unsubscribe fence). Assert: after all operations are released and all callback strong refs are returned, the registry is empty and ref counts are zero — no cycle survives.

#### `CapDestroy` and module state

`CapDestroy` checks the global callback ref-count and returns `E_ILLEGAL_METHOD_CALL` if any callback reference is still live. After `CapDestroy` succeeds, all application state is freed, but the DLL module remains loaded. Only `CapInit` can re-initialize. The module is reclaimed at process exit.

### Adversarial tests

The probe must include tests that exercise the callback-release race:

1. **Capture cancel + immediate release**: Call `CapturePrepare` then `CaptureActivate`, immediately call `CaptureRequestStop(cancel)`. On terminal event, call `CaptureRelease` with zero delay. Verify no crash/use-after-free. Run under ASAN or page-heap if available.
2. **Picker cancel + immediate release**: Open picker, simulate user cancel. On terminal event, call `PickerRelease` with zero delay.
3. **Permission unsubscribe racing `AccessChanged`**: Subscribe to `AccessChanged`. Revoke microphone permission in system settings (to trigger the handler). Call `CapPermissionUnsubscribe` while the handler may be in-flight. Verify no crash. The handler's strong ref must keep state alive. Verify Go's original `notifyEvent` handle can be closed immediately after `CapPermissionUnsubscribe` returns without affecting the in-flight handler (which signals only the duplicated handle).
4. **Rapid start/stop/release cycles**: Repeat capture start → stop → wait terminal → release 100 times in a tight loop. No leaked refs, no crash.
5. **Deterministic Diagram-A callback barrier** (R6-1): hold the activation callback after it publishes terminal and signals `callbackNotify`, but before it closes that duplicate/releases its strong ref/returns. Go may wake and call `CaptureRelease`, but `CapDestroy` must reject while the callback ref lives. Release the barrier; verify close/release/return and later destroy success. Other callback branches never publish terminal and use separate barriers before their close-without-signal step.
6. **Unsubscribe fence barrier** (R6-2): inject a test barrier in the `AccessChanged` handler after acquiring the strong ref but before calling `SetEvent`. Call `CapPermissionUnsubscribe` from Go while the handler is held. Verify unsubscribe returns, Go closes its original notification handle, and no crash occurs. Release the barrier. Verify the handler signals the still-open subscription duplicate, returns and releases its last strong ref; only then does the subscription-state destructor close the duplicate. The ref-count reaches zero cleanly.
6a. **CapturePrepare DuplicateHandle failure** (R14-2): inject a `DuplicateHandle` failure into `CapturePrepare`. Verify `CapturePrepare` returns the failure HRESULT, writes **no** `*opId`, creates **no** operation (registry unchanged) and **no** capture thread, and leaks no handle. A subsequent `CapturePrepare` with a valid handle succeeds.
6b. **CaptureActivate DuplicateHandle failure** (R14-2): drive an operation to `PREPARED`, then inject a `DuplicateHandle` failure into `CaptureActivate`. Verify `CaptureActivate` returns the failure HRESULT, launches **no** activation (`ActivateAudioInterfaceAsync` never called), leaves the packed state at `PREPARED`, and leaks no handle. Verify the operation is still retryable (`CaptureActivate` with duplication restored succeeds) and cancellable (`CaptureRequestStop(cancel)` from `PREPARED` transitions to terminal `cancelled`).
7. **Injected CoInitializeEx failure** (R6-4): mock `CoInitializeEx` on the capture thread to return `RPC_E_CHANGED_MODE`. Verify that `CapturePrepare` creates the thread, which transitions through STOPPING/SEALED, publishes `FAILED`, signals+closes only its capture duplicate, and launches no activation. Verify exact counts and no COM leak.
8. **Injected activation launch failure** (R6-4): mock `ActivateAudioInterfaceAsync` to return an error HRESULT synchronously. Verify that the capture session transitions to `FAILED`, no callback is registered, no COM objects leak, and the capture thread exits cleanly.
9. **MTA-readiness timeout** (R7-4, R12-4): inject a blocking delay in the capture thread before `CoInitializeEx` so that MTA-ready is never signaled within 5 seconds. Verify that `CapturePrepare` returns `S_OK` with a valid `opId`, the **waiter** (not UI thread) times out, calls `CaptureRequestStop(cancel)`, the operation transitions to `cancelled`(5) with terminal reason `CAP_REASON_CANCEL` and HRESULT `HRESULT_FROM_WIN32(ERROR_CANCELLED)` (R13-1: the frozen timeout mechanism is `CaptureRequestStop(cancel)`, so the public state is `cancelled`, not `failed`; `ERROR_TIMEOUT` is logged as evidence but is not the terminal HRESULT), the capture thread is signaled to exit and eventually does, and `CaptureRelease` + `CapDestroy` succeed after thread exit. The UI thread is never blocked (R12-4).
10. **Cancellation wakes capture thread** (R7-4, R9-1): call `CapturePrepare`, wait for MTA-ready via waiter, call `CaptureActivate`, then call `CaptureRequestStop(cancel)` before activation completes. Verify that `captureThreadWakeEvent` is signaled, the capture thread wakes, sees no handoff (null slot + cancelled), calls `CoUninitialize`, sets `threadDone=1`, and exits. The thread does NOT publish terminal (R9-1 — the callback is the final owner). Verify exact `threadDone` state and object counts. (See test 15 for the full composite barrier verification.)
11. **Terminal state after CoUninitialize** (R7-4, R9-2): inject a barrier in the capture thread after `CoUninitialize` but before `threadDone=1`. Verify that `CaptureGetResult` does not return terminal while the barrier is held. Release the barrier. Verify `threadDone=1` is set, then terminal state is published (seq-cst fence between), `localNotify` is signaled.
12. **Synchronous activation failure + thread cleanup** (R7-4): mock `ActivateAudioInterfaceAsync` to fail synchronously. Verify `CaptureActivate` closes the pre-created callback duplicate without signaling (no callback exists), stores the pending cause and signals `captureThreadWakeEvent`; the capture thread calls `CoUninitialize`, publishes terminal through and closes its own duplicate, and exits. The capture thread holds no strong session ref. Verify no leaked handles, threads, handler, session, or COM objects.
13. **C++/WinRT cycle breaking** (R7-4): inject a test where the callback completes but does NOT clear its async-operation reference. Verify that the cycle prevents destruction (leak). Then run normally with the clearing step. Verify clean destruction and zero live objects.
14. **Capture thread last-ref no-deadlock** (R8-2, R9-2): start `CapturePrepare`/`CaptureActivate`, wait for activation + capture. While the capture thread is running, call `CaptureRequestStop(user_stop)`. On terminal event, call `CaptureRelease`. Verify that the operation destructor does NOT wait on the capture thread (no `WaitForSingleObject` on self). The capture thread sets `threadDone=1` (atomic release) BEFORE publishing terminal (atomic release), with a seq-cst fence between. Inject a barrier in the capture thread **after `threadDone=1` but before terminal publication** (R9-2 — this is the correct ordering, not the reverse); while the barrier is held, verify `CaptureGetResult` does not yet return terminal. Release the barrier. Verify terminal is published, Go wakes, and `CaptureRelease` + `CapDestroy` succeed. Any observer of terminal is guaranteed to also see `threadDone=1` due to the seq-cst fence.
15. **Composite barrier cancel test** (R9-1): in Diagram A verify the thread closes its duplicate silently, then the callback observes `threadDone==1`, publishes terminal, and signals+closes `callbackNotify`. Reverse timing for Diagram B: callback stores cause, closes its duplicate silently, wakes the thread, and the thread publishes/signals+closes its duplicate.

---

## Asynchronous ABI design

*Addresses R2 finding 1.*

### Design principle

C++/WinRT explicitly warns: calling `.get()` on the UI thread "is not concurrent nor asynchronous, so it's not appropriate for a UI thread (and an assertion will fire in unoptimized builds if you attempt to use it on one)." `[MS-39]`

Every native export that wraps a WinRT async operation, WASAPI activation, or picker dialog follows an **initiate → event/message → query/take-result** contract:

1. **Initiate/call boundary**: Go calls UI-apartment exports on the UI thread.
   `CapturePrepare` validates, allocates, pre-creates the capture notification
   duplicate, creates the thread, publishes one operation ID, and returns.
   Failures before publication return directly with no ID. Once published,
   worker outcomes (MTA failure/timeout, activation launch/completion, WASAPI)
   travel through `CaptureGetResult`; UI/callback code stores a pending cause
   and never publishes capture terminal directly. Later calls on the existing
   ID still have ordinary call-level errors: `CaptureActivate` may return
   `E_POINTER`, `E_NOT_VALID_STATE`, or callback-duplicate failure without
   changing a `PREPARED` session; if its pre-created duplicate loses the
   `PREPARED→ACTIVATING` CAS, it closes the duplicate and returns
   `E_NOT_VALID_STATE`. This explicit exception prevents the generic async rule
   from hiding retryable validation/ownership failures.
2. **Event**: the owning worker signals its helper-owned duplicate of the Go
   event object. The duplicate references the same kernel event, so Go's
   `WaitForMultipleObjects` wakes; no worker uses or closes Go's original handle.
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

Operation IDs are monotonically incrementing `uint32_t` values starting at 1. ID 0 is reserved (invalid). The sequence wraps at `UINT32_MAX` back to 1. Before assigning a new ID, the helper checks that the candidate is not currently occupied by an active (non-released) operation. If every ID in the `uint32_t` space is occupied (theoretically 4,294,967,295 concurrent operations — impossible given the one-at-a-time limits), the **initiating export** (e.g. `CapturePrepare`, `PickerOpenFile`) fails with `E_OUTOFMEMORY` — not `CapInit`, which has no involvement in ID allocation (R7-4 correction of stale sentence). In practice, the one-active-per-category limits (§Ownership rules) mean at most ~5 operations exist simultaneously; wrap is a theoretical safety, not a realistic concern.

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
        fill scratchBuf with zeros (frames * channels floats)
    } else {
        // Convert into scratch buffer first (R8-6). The data pointer is
        // only valid between GetBuffer and ReleaseBuffer — copy out before
        // releasing. The producer index is NOT published yet (R9-3).
        convert data to float32 into scratchBuf per §Frozen sample representation
        if conversion fails:
            ReleaseBuffer(frames);
            → terminal FAILED with CAP_REASON_FORMAT_ERROR; break;
    }

    // Release the WASAPI buffer BEFORE committing to the ring (R9-3).
    // If ReleaseBuffer fails, the packet must not become visible to the
    // consumer — it may indicate a device removal or stream reset.
    hr = captureClient->ReleaseBuffer(frames);
    if (FAILED(hr)) { → terminal FAILED with hr; break; }

    // WASAPI buffer released successfully — now commit to the ring.
    copy scratchBuf to recording ring (publish producer index)
}
```

**Whole-packet ring preflight** (R7-3): the previous design copied frames into the ring and then checked whether the ring was full. This could overrun the ring (writing beyond capacity) or leave a partial packet (some frames written, remainder dropped). The corrected design checks ring capacity **before** conversion/copy. If the ring cannot hold the entire packet, zero frames are written, the WASAPI packet is released via `ReleaseBuffer`, and the session transitions to overflow failure. The ring capacity check is a single atomic read of the write-available count.

**Scratch-buffer conversion** (R8-6, R9-3): the capture thread converts into a pre-allocated scratch buffer (`scratchBuf`, sized to `maxFrames * channels * sizeof(float32)` where `maxFrames = bufferFrames`). The data pointer from `GetBuffer` is only valid until `ReleaseBuffer`, so the conversion copies data out first. The ordering is: convert→`ReleaseBuffer`→commit (R9-3). The producer index is published (ring write committed) only after **both** (a) the entire packet converts successfully into the scratch buffer **and** (b) `ReleaseBuffer` succeeds. On conversion failure, `ReleaseBuffer` is called but the ring is not committed. On `ReleaseBuffer` failure, the ring is not committed. In both cases zero frames become visible to the consumer — the ring's producer index is not advanced. The consumer racing a mid-packet failure sees the ring as unchanged. Test: consumer reads while a conversion failure occurs mid-packet; verify zero new frames are visible and the ring is in a consistent state.

**WASAPI buffer flag handling** (R7-3):
- `AUDCLNT_BUFFERFLAGS_DATA_DISCONTINUITY`: the first packet after `IAudioClient::Start()` commonly carries this flag (documented as a stream transition signal). Subsequent packets with this flag indicate a timing glitch where WASAPI lost samples between packets. Accepting this while treating an app-ring overflow as fatal would be inconsistent — both produce corrupted, discontinuous audio. The policy: first packet accepts it; any subsequent discontinuity is terminal `CAP_REASON_DISCONTINUITY` (non-promotable). The exact app-classification HRESULT is `HRESULT_FROM_WIN32(ERROR_INVALID_DATA)` (`0x8007000D`), distinct from the overflow code.
- `AUDCLNT_BUFFERFLAGS_TIMESTAMP_ERROR`: the device position/QPC timestamp is unreliable. Since the recording does not consume device-position timestamps (it writes frames sequentially), this flag is logged for evidence but accepted.
- `AUDCLNT_BUFFERFLAGS_SILENT`: handled as before — write zeros.

**Error cleanup for an acquired packet** (R10-1): if conversion fails while a packet is acquired (between `GetBuffer` and `ReleaseBuffer`), the capture thread **must** call `ReleaseBuffer(frames)` before any COM teardown. Leaving a buffer acquired prevents `IAudioClient::Stop()` from completing cleanly. The sequence on mid-packet failure is: cleanup `ReleaseBuffer(frames)` (HRESULT logged; terminal reason is the conversion failure, not the cleanup `ReleaseBuffer` result) → internal-failure CAS `CAPTURING→STOPPING` + `CAP_REASON_FORMAT_ERROR` → packed seal CAS `STOPPING→SEALED` (R10-4/R11-2) → `Stop` → release services → release `IAudioClient` → `CoUninitialize` → `threadDone=1` (atomic release — cleanup complete, one terminal store remains) → `atomic_thread_fence(seq_cst)` → publish terminal (atomic release — final session-state access) → `SetEvent(localNotify)` → thread exits (R10-1, R10-2). Zero frames become visible to the consumer — the ring's producer index is not advanced.

**Cleanup `ReleaseBuffer` failure classification** (R10-3): in every early-exit branch where a WASAPI buffer is acquired, the cleanup `ReleaseBuffer` is called before COM teardown. The cleanup `ReleaseBuffer` HRESULT is **logged for evidence** but does NOT override the terminal reason:
- **Overflow** (ring full): terminal reason = `CAP_REASON_OVERFLOW`, HRESULT = `HRESULT_FROM_WIN32(ERROR_BUFFER_OVERFLOW)`. Cleanup `ReleaseBuffer` HRESULT is logged. Zero frames visible to consumer.
- **Discontinuity** (non-first `DATA_DISCONTINUITY`): terminal reason = `CAP_REASON_DISCONTINUITY`, HRESULT = `HRESULT_FROM_WIN32(ERROR_INVALID_DATA)` (`0x8007000D`). Cleanup `ReleaseBuffer` HRESULT is logged. Zero frames visible.
- **Format error** (conversion failure): terminal reason = `CAP_REASON_FORMAT_ERROR`, HRESULT = `E_INVALIDARG`. Cleanup `ReleaseBuffer` HRESULT is logged. Zero frames visible.
- **Stop-while-acquired** (stop signal received between `GetBuffer` and `ReleaseBuffer`): cleanup `ReleaseBuffer` is called first (HRESULT logged), then normal stop sequence proceeds. The frames from the acquired packet are NOT committed to the ring (conversion may be incomplete). Terminal reason is the sealed stop reason.
If the cleanup `ReleaseBuffer` itself fails, `IAudioClient::Stop()` is still attempted — a failed `ReleaseBuffer` does not skip the stop sequence. The `Stop` HRESULT is also logged. All subsequent releases (`IAudioCaptureClient`, `IAudioClient`, `CoUninitialize`) proceed regardless.

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

*Addresses R8 finding 5. Single-owner waiter with command/result queue per R9 finding 5.*

The live `pulsar-win` main thread blocks in `GetMessageW` (`pumpMessages()` in `ui_windows.go`). A second independent `WaitForMultipleObjects` blocking loop cannot coexist on the same OS thread — they would race for ownership of the thread's wait state. The two blocking APIs are incompatible on a single thread.

**Frozen integration: single-owner dedicated waiter goroutine (R9-5).**

A new goroutine, pinned to its own OS thread via `runtime.LockOSThread()`, is the **single owner** of all helper query/read/take/release/promotion operations. The UI thread never calls query or read exports directly — it sends commands to the waiter via a synchronized command queue and receives results via a result queue. This eliminates the race where two threads call `CaptureRead` or `CaptureGetResult` concurrently.

```
Command/result queue (Go channels, synchronized)
  │
  ├── commandCh chan WaiterCommand   // UI thread → waiter
  │     Commands: StartCapture{opId}, StopCapture{reason},
  │               StartPicker, QueryPermission, GracefulQuit
  │
  ├── resultCh  chan WaiterResult    // waiter → UI thread (via PostMessageW)
  │     Results: CaptureTerminal{reason, hresult, promoted},
  │              PickerResult{handle, takeState, err},
  │              PermissionChanged{status}

Waiter goroutine (separate OS thread, LockOSThread)
  │
  ├── WaitForMultipleObjects({captureNotifyEvent, pickerNotifyEvent,
  │     permissionNotifyEvent, enumerationNotifyEvent,
  │     commandQueueEvent, shutdownEvent})
  │
  ├── On captureNotifyEvent / pickerNotifyEvent / permissionNotifyEvent:
  │     1. Drain CaptureRead until S_FALSE (waiter is the SOLE caller).
  │     2. Write drained frames to .partial (Go file I/O, no UI dependency).
  │     3. Query CaptureGetResult, CapPermissionCheck, all pending results
  │        (waiter is the SOLE caller of all query/read exports).
  │     4. On terminal capture: waiter owns promotion decision —
  │        re-check terminal reason + CapPermissionCheck, rename
  │        .partial → .wav or delete .partial, then post result to UI.
  │     5. For UI-affecting results (permission change → prompt,
  │        terminal → update tray, picker → process file):
  │        PostMessageW(lifecycleHwnd, WM_APP+N, ...) to UI thread.
  │     6. Loop back to WaitForMultipleObjects.
  │
  ├── On commandQueueEvent:
  │     Dequeue all pending commands from commandCh:
  │     - StopCapture{reason}: call CaptureRequestStop(opId, reason)
  │       (non-blocking, legal from any thread).
  │     - GracefulQuit (R10-5, R11-3, R12-2): begin async quit state
  │       machine. Start 30-second ForceQuit watchdog (R12-2).
  │       1. Dismiss/cancel every pending operation per the
  │          §Per-operation quit table (R11-3, R12-2).
  │          Capture: CaptureRequestStop.
  │          Picker: PickerCancel(opId) (R12-2).
  │          Permission: CapPermissionRequestCancel(opId) (R12-2).
  │          Enumeration: CapEnumerateDevicesCancel(opId) (R12-2).
  │          Default-device: no cancel (synchronous — R12-2).
  │       2. Continue wait/drain loop for EVERY operation, with a
  │          per-operation five-second threshold ONLY as a "surface Force
  │          Quit affordance" trigger (R13-3) — NOT as permission to
  │          stop owning the operation. A cooperatively-cancelled op
  │          (picker/permission/enum) may never terminate; the waiter
  │          keeps querying it. The waiter is the SOLE owner of
  │          query/take/release, so it must NOT exit while any op is
  │          unreleased (R13-3: exiting would orphan a late-terminal
  │          op with no owner to release it → registry never empties
  │          → CapDestroy can never succeed).
  │       3. As each op reaches terminal: final drain (CaptureRead
  │          until S_FALSE), then CaptureRelease / PickerRelease
  │          (take HANDLE/state — not stale PickerDone{path} — R10-5).
  │          Continue this per-op loop for late terminals indefinitely
  │          (bounded only by the 30-second ForceQuit fallback).
  │       4. Unsubscribe AccessChanged (once, after issuing it).
  │       5. When (and only when) the registry is EMPTY (all ops
  │          released) AND CapIsQuiescent()==S_OK (all callback refs
  │          = 0): PostMessageW(lifecycleHwnd, WM_APP+CLEANUP_READY).
  │          The UI thread then calls CapDestroy; WM_QUIT only after
  │          CapDestroy==S_OK (R12-2).
  │       6. Exit the waiter loop ONLY after posting CLEANUP_READY
  │          with an empty registry — never while an op is still owned.
  │          If the 30-second ForceQuit watchdog fires first, os.Exit(1)
  │          reclaims everything (R13-3/R13-4: forced fallback).
  │       NOTE: waiter does NOT call CapDestroy (wrong thread — R10-5).
  │     - Other commands: dispatch to appropriate helper export.
  │
  ├── On shutdownEvent (WM_ENDSESSION forced kill — see below):
  │     Best-effort final drain. Do NOT call CaptureRelease/CapDestroy —
  │     the OS is reclaiming the process. Exit loop immediately.
  │     NOTE (R10-5): the wndproc calls CaptureRequestStop (non-blocking)
  │     BEFORE signaling shutdownEvent — the capture thread begins its
  │     cleanup sequence. The waiter does not need to call stop again.
  │
  UI thread (pinned main goroutine, GetMessageW loop)
  │
  ├── User actions (tray menu, hotkey) → send command to waiter:
  │     commandCh <- StopCapture{reason: user_stop}
  │     commandCh <- StartCapture{...}    (via WM_APP+N roundtrip)
  │
  ├── Receives WM_APP+N from waiter → executes UI-only actions:
  │     - Show permission prompt (CapPermissionRequest — UI-thread-only)
  │     - Update tray menu state
  │     - Start new CapturePrepare / CaptureActivate / PickerOpenFile (UI-thread-only exports)
  │
  ├── Receives WM_QUERYENDSESSION → calls CaptureRequestStop(opId,
  │     shutdown) for each active operation (non-blocking — R10-5),
  │     returns TRUE (must return quickly).
  │
  ├── Receives WM_ENDSESSION (wParam=TRUE) → calls CaptureRequestStop
  │     if not already called by WM_QUERYENDSESSION (idempotent),
  │     signals shutdownEvent, returns from wndproc immediately.
  │     OS reclaims process resources (R4-2).
  │     Does NOT send GracefulQuit — no time for orderly cleanup.
  │     Does NOT call CapDestroy — wrong context and no time (R10-5).
  │
  ├── Receives WM_APP+CLEANUP_READY (from waiter after graceful quit):
  │     Calls CapDestroy on the UI thread with retry loop (R12-2).
  │     Posts WM_QUIT only after CapDestroy==S_OK (R12-2). (R10-5)
  │
  ├── App quit (OnQuit / Ctrl-C / SIGTERM) → sends GracefulQuit command
  │     to waiter and starts 30-second ForceQuit watchdog (R10-5,
  │     R12-2: this starts the async quit state machine, NOT an
  │     immediate WM_QUIT). The UI thread continues pumping messages
  │     (WM_APP+N results, WM_APP+CLEANUP_READY). When the waiter
  │     posts WM_APP+CLEANUP_READY, the UI thread calls CapDestroy
  │     with retry loop (R12-2: no PostQuitMessage on failure) and
  │     posts WM_QUIT only after CapDestroy==S_OK. If CapDestroy
  │     never succeeds, ForceQuit watchdog terminates process (R12-2).
  │     After the message loop exits, proceed with existing shutdown.
  │     **Ctrl-C/SIGTERM bridge** (R11-3): a dedicated goroutine calls
  │     signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM) and sends
  │     GracefulQuit to commandCh on receipt. This replaces the current
  │     awaitShutdown signal channel which is ignored on Windows.
  │     The goroutine is started in run() before the message pump.
  │
  ├── Receives WM_POWERBROADCAST, WM_WTSSESSION_CHANGE →
  │     commandCh <- StopCapture{reason: suspend / lock}
  │
  ├── Receives tray callback, onboarding messages as before
```

**Graceful quit vs WM_ENDSESSION (R9-5, R10-5, R11-3).** These are fundamentally different shutdown paths:
- **Graceful quit** (tray Quit, Ctrl-C, or SIGTERM): the UI thread sends
  `GracefulQuit`, atomically arms the 30-second watchdog, and keeps pumping.
  The waiter issues cooperative cancel/stop requests once, but retains sole
  ownership of every event and operation. Five seconds only logs the stall and
  exposes Force Quit. Whenever an operation later completes — cancelled,
  successful, or failed — the waiter queries/takes/drains it and calls its
  release export exactly once. It unsubscribes `AccessChanged`, then posts
  `WM_APP+CLEANUP_READY` only after the registry is empty and
  `CapIsQuiescent()==S_OK`. It never posts cleanup while a pending operation or
  callback remains, and never exits merely because five seconds elapsed. The
  UI handler retries `CapDestroy` asynchronously via `WM_TIMER`; on success it
  atomically changes the shared exit state from `graceful_pending` to
  `graceful_complete`, thereby defeating the watchdog, then posts `WM_QUIT`.
  A never-terminal operation produces no cleanup message and the watchdog (or
  explicit Force Quit) is the only bound. The waiter never calls `CapDestroy`.
- **WM_ENDSESSION** (OS is killing the process): the wndproc calls `CaptureRequestStop(opId, shutdown)` for each active operation (non-blocking, legal from wndproc — R10-5) and then signals `shutdownEvent` (manual-reset event in the wait array), returning immediately — `WM_ENDSESSION` has a ~5-second budget and the wndproc must return to avoid a forced kill. The waiter wakes, does a best-effort final drain (write any buffered frames to `.partial`), but does NOT call `CaptureRelease` or `CapDestroy` — the OS reclaims all handles and memory. No `GracefulQuit` command is sent.
- **WM_QUERYENDSESSION**: the wndproc calls `CaptureRequestStop(opId, shutdown)` for each active operation (non-blocking, idempotent) and returns TRUE immediately. This ensures the capture thread begins cleanup before `WM_ENDSESSION` arrives. The stop request is idempotent — `WM_ENDSESSION` may call it again safely.

#### Per-operation quit table (R11-3)

*R15-1: cancellation is cooperative; ownership, not a timeout, controls
cleanup.*

| Operation | Request issued once | Late terminal handling by sole waiter | Release gate | Five-second behavior |
|---|---|---|---|---|
| **Capture** | `CaptureRequestStop(opId, cancel)` | Wait for notification; query public terminal; final-drain `CaptureRead` | `CaptureRelease` only after terminal and drain | Log/show Force Quit; keep waiting/owning |
| **Picker** | `PickerCancel(opId)` calls `IAsyncInfo::Cancel` as a cooperative request | Handler may report cancelled, picked, or failed, at any later time; waiter queries/takes any picked handle | `PickerRelease` only after a terminal result; closes untaken handle | Log/show Force Quit; do not assume dialog dismissal or `Canceled` |
| **Permission request** | `CapPermissionRequestCancel(opId)` requests cancellation | Handler may complete with cancelled, success, or failure; waiter queries actual result | `CapPermissionRequestRelease` after terminal | Log/show Force Quit; prompt may remain |
| **Device enumeration** | `CapEnumerateDevicesCancel(opId)` requests cancellation | Handler may complete with cancelled, collection, or failure | `CapEnumerateDevicesRelease` after terminal | Log/show Force Quit; retain event/owner |
| **Default-device wrapper** | No cancel object: underlying `MediaDevice.GetDefaultAudioCaptureId` returns a string synchronously; wrapper still publishes/query/releases an operation for ABI uniformity | Waiter drains the wrapper result event | `CapGetDefaultDeviceRelease` after wrapper terminal | If unexpectedly stalled, log/show Force Quit and retain ownership |
| **AccessChanged subscription** | `CapPermissionUnsubscribe` revokes the token once | Already-dispatched handlers keep strong refs and finish normally | No registry release; `CapIsQuiescent()==S_OK` after all handlers return | Log/show Force Quit; never infer quiescence from elapsed time |

**`IAsyncInfo::Cancel` contract** `[MS-49]` (R13-4 — do not overclaim): Microsoft documents only that `Cancel` **requests** cancellation. It does NOT promise prompt dismissal, a bounded completion time, or that a user-driven picker will close. The earlier claim that "all first-party WinRT async operations reach a terminal state after `Cancel`" is not sourced and is removed. Cancellation is **cooperative**: the operation *may* transition to `AsyncStatus::Canceled` and fire its `Completed` handler, or it may keep running until it finishes on its own (a picker waits on the user). The helper's completion handler checks `asyncStatus` and maps `Canceled` to the cancelled terminal state; until that handler fires, the operation remains live and its strong ref keeps state alive. If `Cancel` throws (the operation already completed), the exception is caught — the operation is already terminal. Because completion is not guaranteed within any bound, the quit protocol keeps the release owner (the waiter) alive until the callback actually fires (R13-3) and relies on the automatic forced-exit fallback (below) as the only hard bound.

**Five seconds is an affordance threshold, never an ownership timeout.** The
waiter may release other operations that have independently reached terminal,
but it keeps every pending entry/event and stays in its wait/drain loop. It
does not post `CLEANUP_READY`, does not close the pending event, and does not
exit. A terminal result at 6, 15, or 29 seconds is queried and released by the
same waiter; only then can the empty-registry/quiescence gate open. If no
terminal result arrives, the forced-exit path may win at 30 seconds without any
false cleanup message.

**Cancel exports — frozen public ABI signatures** (R12-2):

```c
// Cancel a pending picker operation. Called by the waiter during quit.
// Internally calls IAsyncInfo::Cancel on the IAsyncOperation<StorageFile>.
// Returns S_OK if cancel was requested, E_HANDLE if opId is unknown,
// E_NOT_VALID_STATE if the operation is already terminal.
// Non-blocking. S_OK means the cancellation request was issued, not that the
// dialog closed or that the eventual status will be Canceled. If the operation
// has already completed, Cancel is a no-op and the actual result is queried.
HRESULT __stdcall PickerCancel(uint32_t opId);

// Cancel a pending permission request. Same contract.
HRESULT __stdcall CapPermissionRequestCancel(uint32_t opId);

// Cancel a pending device enumeration. Same contract.
HRESULT __stdcall CapEnumerateDevicesCancel(uint32_t opId);
```

Note: `CapGetDefaultDevice` does not need a cancel export — the underlying `MediaDevice.GetDefaultAudioCaptureId` is synchronous (R12-2).

#### `CapIsQuiescent` export (R11-3)

```c
// Returns S_OK if the global callback reference count is zero
// (no callback or event handler is currently executing DLL code)
// and all capture threads have set threadDone=1.
// Returns S_FALSE if any callback ref is still live or any
// capture thread has not completed.
// This is a non-blocking, lock-free query.
HRESULT __stdcall CapIsQuiescent(void);
```

The waiter polls `CapIsQuiescent` after unsubscribing `AccessChanged` and
releasing all operations. It posts `WM_APP+CLEANUP_READY` only when the
registry is empty, the subscription is unwound, and this query returns `S_OK`.
`CapDestroy` rechecks those invariants authoritatively on the UI thread; the
timer retry is defense in depth for any destructor/publication edge still
settling between the waiter observation and destroy, not permission to post
cleanup before the gate is proven.

#### Ctrl-C/SIGTERM bridge (R11-3)

*Addresses R11 finding 3: current `awaitShutdown` on Windows ignores the signal channel, so prose saying SIGTERM sends `GracefulQuit` was an implementation delta, not a contract.*

The bridge is a dedicated goroutine started in `run()` before the message pump:

```go
sigCh := make(chan os.Signal, 1)
signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
go func() {
    <-sigCh
    commandCh <- GracefulQuit{}
    SetEvent(commandQueueEvent)
}()
```

On receipt of Ctrl-C (`os.Interrupt`) or `SIGTERM`, the goroutine sends `GracefulQuit` to `commandCh` and signals `commandQueueEvent`. The waiter wakes and begins the async quit state machine. The UI message pump continues running (it is on a different goroutine — the main goroutine pinned to the UI thread). This replaces the currently ignored signal channel in `awaitShutdown`.

#### `WM_APP+CLEANUP_READY` → `CapDestroy` → `WM_QUIT` handshake (R11-3)

The UI thread's handler for `WM_APP+CLEANUP_READY` (R14-3: **no blocking `Sleep`
in the wndproc** — every refused `CapDestroy` re-arms a one-shot `SetTimer`, so
the message pump keeps dispatching picker/`WM_HOTKEY`/lifecycle messages between
attempts, with no finite retry cap; the 30-second `ForceQuit` watchdog is the
only bound):

```
const DESTROY_RETRY_TIMER = 0xD5  // wndproc-local WM_TIMER id

const (
    exitRunning uint32 = iota
    exitGracefulPending
    exitGracefulComplete
    exitForceCommitted
)
var exitState atomic.Uint32

// Called once when ordinary Quit/Ctrl-C/SIGTERM begins.
func armGracefulAndWatchdog() {
    if !exitState.CompareAndSwap(exitRunning, exitGracefulPending) { return }
    time.AfterFunc(30*time.Second, func() {
        if exitState.CompareAndSwap(exitGracefulPending, exitForceCommitted) {
            os.Exit(1)
        }
    })
}

func forceQuitNow() {
    if exitState.CompareAndSwap(exitGracefulPending, exitForceCommitted) {
        os.Exit(1)
    }
}

// Invoked once from WM_APP+CLEANUP_READY and again from each WM_TIMER tick.
// Returns immediately; never blocks the pump.
func tryCapDestroyOnce(hwnd) {
    hr := CapDestroy()
    if hr == S_OK {
        KillTimer(hwnd, DESTROY_RETRY_TIMER)  // idempotent; safe if not armed
        // Linearization against the independent watchdog. If force already
        // committed, do not claim/post a graceful exit; os.Exit is imminent.
        if exitState.CompareAndSwap(exitGracefulPending, exitGracefulComplete) {
            PostQuitMessage(0)                // graceful exit — only WM_QUIT path
        }
        return
    }
    if hr != E_ILLEGAL_METHOD_CALL {
        // Unexpected/transient failure — log; a later attempt may still succeed.
        log error
    }
    // Callback/capture-thread/subscription state is still live. Schedule the
    // next attempt WITHOUT blocking the pump. One-shot timer; re-armed only
    // while destroy is still refused. Keeps retrying until S_OK or the
    // 30-second ForceQuit watchdog fires (R14-3).
    SetTimer(hwnd, DESTROY_RETRY_TIMER, 100 /*ms*/, nil)
}

case WM_APP+CLEANUP_READY:
    tryCapDestroyOnce(hwnd)   // returns immediately; pump keeps running
    return

case WM_TIMER:
    if wParam == DESTROY_RETRY_TIMER {
        KillTimer(hwnd, DESTROY_RETRY_TIMER)  // one-shot; re-armed by the call below iff still refused
        tryCapDestroyOnce(hwnd)
        return
    }
    // ... other timers ...
```

**`WM_QUIT` is posted only after both `CapDestroy==S_OK` and the successful
`exitGracefulPending→exitGracefulComplete` CAS.** The CAS is the watchdog defeat
point. A watchdog observing `exitGracefulComplete` is a no-op; a UI path that
loses to `exitForceCommitted` does not post `WM_QUIT`. Timer cancellation alone
is not used as a correctness primitive. Refused destroy attempts keep the pump
responsive and re-arm the one-shot UI timer without a finite retry count.

**`ForceQuit` path** (R12-2): a separate explicit forced-exit that abandons cleanup:

1. **Tray menu item**: "Force Quit" appears five seconds after graceful quit remains pending. Clicking calls `forceQuitNow`; only the winning `exitGracefulPending→exitForceCommitted` CAS calls `os.Exit(1)`.
2. **30-second watchdog**: the timer performs the same CAS. If graceful destroy already won, it observes `exitGracefulComplete` and returns. If it wins first, hard process exit is committed and the UI path cannot later post a graceful `WM_QUIT`.
3. **Never described or tested as graceful.** The `ForceQuit` path does not call `CapDestroy`, does not drain captures, does not release operations. It is a last-resort safety net. Tests verify that `ForceQuit` terminates the process cleanly (no crash) but do not verify resource cleanup.

Every ordinary Quit/Ctrl-C/SIGTERM starts graceful work and arms forced fallback,
but their terminal commits are mutually exclusive through `exitState`: either
graceful completion wins and posts `WM_QUIT`, or force wins and calls
`os.Exit(1)`. The work attempts coexist; the exit decisions cannot both fire.

#### Required quit tests (R11-3)

1. **Quit with open picker / eventual cancel**: issue cooperative cancel; inject eventual `Canceled`; verify query/release and graceful destroy.
2. **Cancel races successful completion**: for picker, permission, and enumeration, issue cancel but inject normal success. Verify the waiter accepts the actual success result, takes/owns any picker handle, releases once, and does not rewrite it as cancelled.
3. **Cancel races failure**: inject a failed terminal result after cancel. Verify actual HRESULT preserved, query/release once, graceful cleanup continues.
4. **Quit with in-flight AccessChanged**: send GracefulQuit while an `AccessChanged` handler is executing (injected delay in handler). Verify: `CapPermissionUnsubscribe` returns, `CapIsQuiescent` returns `S_FALSE` while handler runs, `S_OK` after handler returns.
5. **First and repeated CapDestroy failures**: verify every refusal arms one timer and returns; inject unrelated UI messages between retries; eventual success wins the exit CAS, defeats watchdog, then posts `WM_QUIT`.
6. **SIGTERM while UI pump lives**: send `SIGTERM` during active capture. Verify: signal goroutine sends `GracefulQuit`, waiter performs async quit, UI continues pumping, `CapDestroy` succeeds, `WM_QUIT` posted.
7. **late terminal at 6 seconds**: at five seconds verify Force Quit becomes visible but waiter/event/registry ownership remains. At six seconds inject terminal, query/release, prove empty registry/quiescence, destroy gracefully, and prove watchdog defeated.
8. **late terminal at 15 seconds**: same assertions; no premature `CLEANUP_READY`, event close, waiter exit, or `CapDestroy` call.
9. **late terminal at 29 seconds / watchdog race**: release at 29 seconds, make `CapDestroy` succeed, and race the exit CAS against the watchdog. If graceful CAS wins, watchdog must be a no-op and `os.Exit` count remains zero; if force CAS wins, no `WM_QUIT` is posted. Never both. This test proves successful destroy can atomically cancel/defeat the watchdog before `WM_QUIT`.
10. **Never-terminal operation**: release all independently terminal operations but leave picker pending forever. Verify no `CLEANUP_READY`, no `CapDestroy`, waiter/event ownership retained, and the watchdog's CAS is the sole automatic forced exit at 30 seconds.
11. **Registry nonempty invariant**: at every scheduler step assert `CLEANUP_READY ⇒ registry.empty && subscriptionUnwound && CapIsQuiescent()==S_OK`; inject a would-be cleanup post with a pending entry and require the test to fail.
12. **Callback not yet entered/currently executing**: exercise both schedules. Waiter remains owner; `CapIsQuiescent` stays `S_FALSE` while appropriate; release/destroy only after actual terminal/ref drain.
13. **Every CapDestroy failure class**: inject registry nonempty, subscription active, callback ref live, capture thread running, and wrong-thread use. No failure posts `WM_QUIT`; UI stays responsive and retries only after `CLEANUP_READY` was legitimately reached.

**Why not `MsgWaitForMultipleObjectsEx`.** While `MsgWaitForMultipleObjectsEx` can atomically wait for both handles and messages on one thread, the existing codebase uses `pGetMessageW.Call` (a raw `user32.dll` proc), not the Go standard library. Combining `MsgWaitForMultipleObjectsEx` with the existing `syscall.NewCallback`-based wndproc requires careful re-dispatch logic and risks starving either messages or handle events. The dedicated waiter goroutine is simpler and avoids modifying the existing proven message pump.

**Ownership rules.** `CapInit` and `CapDestroy` are called from the UI thread (WinRT STA requirement). `CapturePrepare`, `CaptureActivate`, and `PickerOpenFile` are called from the UI thread (WinRT/activation — R12-4). `CaptureRequestStop` is non-blocking and legal from any thread — the waiter calls it on behalf of the UI thread via the command queue. All query/read/take/release exports (`CaptureRead`, `CaptureGetResult`, `CapPermissionCheck`, `CaptureRelease`, `PickerGetResult`, `PickerTake`, `PickerRelease`) are called **exclusively by the waiter goroutine**. This single-owner invariant eliminates concurrent-caller races on these exports.

**Event handle lifetime and waiter shutdown.** The waiter goroutine's wait array includes `shutdownEvent` (manual-reset, created by the waiter) and `commandQueueEvent` (auto-reset, signaled when UI thread writes to `commandCh`). Event handles for helper operations are created before the waiter starts and closed only after the waiter exits and all operations are released. The `commandQueueEvent` is a Go-side notification: when the UI thread sends to `commandCh`, it also calls `SetEvent(commandQueueEvent)` so the `WaitForMultipleObjects` wakes.

**Required tests** (R8-5, R9-5, R10-5):
1. **Coalesced-signal test**: helper signals capture data, `AccessChanged`, and picker completion in rapid succession (coalesced into one wake) while `WM_ENDSESSION` is also queued on the UI thread. Verify the waiter drains all three results, the UI thread processes `WM_ENDSESSION`, and no data or state transition is lost.
2. **Command queue ordering**: UI thread sends StopCapture then GracefulQuit in rapid succession. Verify waiter processes stop first (calls `CaptureRequestStop`), waits for terminal, then performs the async quit state machine (final drain + release + `WM_APP+CLEANUP_READY`). Verify waiter does NOT call `CapDestroy`.
3. **WM_ENDSESSION does not call CapDestroy**: simulate `WM_ENDSESSION` during active capture. Verify the wndproc calls `CaptureRequestStop(shutdown)` and signals `shutdownEvent`. Verify the waiter does best-effort drain, does NOT call `CaptureRelease` or `CapDestroy`, and exits.
4. **WM_QUERYENDSESSION calls CaptureRequestStop** (R10-5): simulate `WM_QUERYENDSESSION` during active capture. Verify the wndproc calls `CaptureRequestStop(shutdown)` (non-blocking) and returns TRUE. Verify idempotent — a subsequent `WM_ENDSESSION` calling stop again is a no-op.
5. **Graceful quit async state machine** (R10-5): start capture, send GracefulQuit. Verify: waiter calls `CaptureRequestStop`, waiter continues waiting until terminal is published, waiter drains, waiter calls `CaptureRelease`, waiter posts `WM_APP+CLEANUP_READY`. Verify: UI thread receives `WM_APP+CLEANUP_READY`, calls `CapDestroy` on its own thread (same thread as `CapInit`), posts `WM_QUIT`. Verify: no `CapDestroy` from the waiter goroutine, no `WM_QUIT` before `CapDestroy` completes, no release before terminal.
6. **OnQuit does not post immediate WM_QUIT** (R10-5): simulate tray Quit click. Verify `OnQuit` sends `GracefulQuit` to waiter and does NOT call `pPostQuitMessage`. The UI message pump continues dispatching messages. `WM_QUIT` is posted only after `WM_APP+CLEANUP_READY` is processed.
7. **Picker HANDLE/take state in waiter result** (R10-5): start picker, complete with a file. Verify the waiter's result carries the kernel HANDLE and take state (taken/untaken), not a `PickerDone{path}` struct with a filesystem path.

### Operation release/take semantics

*Addresses R3 finding 2 (partial). Callback strong-ref interaction per R5 finding 1.*

Every initiate export (`CapturePrepare`, `CapPermissionRequest`, `CapEnumerateDevices`, `CapGetDefaultDevice`, `PickerOpenFile`) creates an operation that must be released after reaching terminal state:

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
4. **A capture thread has not completed.** The capture thread's `threadDone`
   atomic must be 1 (COM cleanup and `CoUninitialize` completed; exactly one
   terminal store may still follow). Registry-empty plus quiescence separately
   prove that terminal publication/release and callback return completed.
   `CapDestroy` never joins the thread (R8-2).

The caller must: `CaptureRequestStop` → wait for terminal state → `CaptureRelease`; similarly release every other operation; call `CapPermissionUnsubscribe`; wait for all callback references to drain; then call `CapDestroy`.

After `CapDestroy` succeeds, all application state is freed. The DLL module remains loaded (process-lifetime — see §Module lifetime). Only `CapInit` can re-initialize.

**Operation-ID exhaustion** (R6-5 correction): ID allocation fails the **initiating export** (e.g. `CapturePrepare` returns `E_OUTOFMEMORY`), not `CapInit`. `CapInit` has no involvement in ID allocation.

**Required tests** (R6-5): terminal-but-unreleased operation blocks `CapDestroy`; callback-held-after-release blocks `CapDestroy`; active subscription blocks `CapDestroy`; unsubscribe-in-flight (handler strong ref still live) blocks `CapDestroy`; repeated `CapDestroy` after success is idempotent; `CapInit` after `CapDestroy` re-initializes cleanly.

#### Imminent process termination (`WM_ENDSESSION`)

On `WM_ENDSESSION` with `wParam == TRUE` (confirmed shutdown), Go calls `CaptureRequestStop(opId, shutdown)` and returns from the wndproc immediately **without** calling `CapDestroy`. The OS reclaims all process resources (memory, handles, COM references) when the process exits. No `CapDestroy` call is attempted — the previous design tried to free state while un-cancellable callbacks were pending, which is a contradiction.

#### Lifetime reference graph

Every holder of a reference to helper-internal state:

| Holder | When acquired | When released | Can outlive release export? |
|---|---|---|---|
| Registry (operation map) | Operation initiation | `*Release` export call | N/A — this IS the release |
| `ActivateCompleted` callback (system MTA thread) | `ActivateAudioInterfaceAsync` initiation | Callback returns to OS (strong ref released as final action) | **Yes** — strong ref keeps state alive if release races callback |
| Capture thread | Created at `CapturePrepare` (before activation — R6-4, R12-4). Thread does **not** hold a ref-counted session reference (R8-2 — eliminates self-join deadlock). Thread accesses session state via packed atomics only (R10-4). | Thread sets `threadDone=1` (atomic release — cleanup complete; R10-2), then publishes terminal (atomic release — final session-state access), with a seq-cst fence between. After terminal publication, thread touches only local-stack handles. | **No** — the thread never holds a reference that can trigger the destructor. The destructor checks `threadDone` but never joins the thread. `CapDestroy` requires `threadDone==1`. |
| `AccessChanged` event handler (per invocation) | Delegate copy/dispatch (before handler entry — R6-2) | Handler return releases the `shared_ptr` copy | **Yes** — handler's strong ref keeps subscription state (including duplicated handle) alive if `CapPermissionUnsubscribe` races handler |
| Picker async completion handler | `PickerOpenFile` initiation | Callback return | **Yes** — strong ref keeps state alive if `PickerRelease` races callback |
| Permission request completion handler | `CapPermissionRequest` initiation | Callback return | **Yes** — same pattern |
| `IActivateAudioInterfaceAsyncOperation` | Returned by `ActivateAudioInterfaceAsync` | Released in activation cleanup (callback or capture thread) | No — released within the session's stop sequence |
| `IActivateAudioInterfaceCompletionHandler` impl | `CaptureActivate` (registered with OS — R12-4) | OS releases after operation completes; helper releases via operation state destructor | **Yes** — OS may hold a reference beyond callback return (documented `[MS-5]`). Process-lifetime DLL loading prevents unload during this window. |

`CapDestroy` requires an **empty operation registry** (all operations released), a fully unwound permission subscription, a zero global callback ref-count, and `threadDone==1` for any capture operation that was started. The module remains loaded (process-lifetime — R6-1). In the common case (no race), all callbacks have returned and all threads have exited before the terminal state is observed. In the race case, the strong-ref mechanism ensures safety without `CapDestroy` needing to know about specific in-flight callbacks — it simply checks the global callback count, confirms `threadDone`, and verifies an empty registry. The destructor never joins threads (R8-2).

---

## Two-phase session lifetime

*Addresses R2 finding 2.*

### Problem

Rev 2 said `CaptureDestroy` is nonblocking, releases COM objects/thread, invalidates the handle on return, and is safe while an un-cancellable activation callback still owns state. Those guarantees conflict — the callback may still be running when `CaptureDestroy` returns, and the COM objects cannot be released from the wrong thread.

### Frozen lifetime contract

Two-phase: **`CaptureRequestStop(opId, reason)`** + **`CaptureRelease(opId)`**.

#### Phase 1: `CaptureRequestStop(opId, reason)`

- Non-blocking. Sets the stop reason via packed atomic CAS (R10-4). A winning
  transition from `PREPARING`, `PREPARED`, or `ACTIVATING` signals
  `captureThreadWakeEvent`; a winning transition from `CAPTURING` signals
  `stopEvent`; a priority upgrade in `STOPPING` needs no new signal. Returns
  `S_OK` immediately.
- Idempotent: calling it on an already-stopping or stopped session is a no-op (returns `S_OK`).
- The capture thread sees the stop signal and performs cleanup per the §Normative cleanup path (R10-1, R11-1):
  1. Reach `STOPPING` (already there from `CaptureRequestStop`)
  2. Packed CAS seal: `STOPPING`→`SEALED` + `sealed=1` + snapshot reason
  3. Call one cleanup function (R11-4, R12-5): `Stop` only if `started`, release `IAudioCaptureClient` only if `serviceAcquired`, `CoTaskMemFree(pMixFormat)` only if `mixFormatOwned`, release `IAudioClient` only if `audioClientOwned`
  4. `CoUninitialize()`
  5. `threadDone=1` (atomic release — cleanup complete; one terminal store remains; R10-2)
  6. `atomic_thread_fence(memory_order_seq_cst)`
  7. Publish terminal state (atomic release — FINAL session-state access; `CaptureGetResult` acquire-reads this)
  8. `SetEvent(localNotify)` then `CloseHandle(localNotify)` — the pre-created
     capture-thread duplicate; Go wakes and queries terminal state
- If activation is still in flight when stop is requested, Diagram A/B owns
  cleanup. If `threadDone==1`, the capture thread has already closed its
  duplicate without signal; the callback publishes terminal and signals+closes
  `callbackNotify`. If `threadDone==0`, the callback stores the cause, closes
  `callbackNotify` without signal, and wakes the thread; the thread publishes,
  signals, and closes its own duplicate. The callback releases its strong ref
  as its final action.
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

If `CaptureRequestStop` is called while `ActivateAudioInterfaceAsync` has not yet completed (R10-1, R10-2):

1. `CaptureRequestStop` installs `state=STOPPING` + `reason=CANCEL` via packed CAS (R11-2: no separate `cancelled` flag; cancellation is `STOPPING`+`CANCEL` in the packed word) and stores `lastPublicState=ACTIVATING`. Signals `captureThreadWakeEvent` (R11-2: wake event for `ACTIVATING` state).
2. The callback fires on an MTA thread (guaranteed by the OS — there is no cancellation API).
3. The callback reads the packed state (acquire), sees `state>=STOPPING` (R11-2), calls `GetActivateResult`, releases any returned `IAudioClient*` on its own MTA thread (legal since `Initialize`/`GetService` were never called, so no service release ordering applies), and clears its stored `IActivateAudioInterfaceAsyncOperation` reference (R10-2 cycle break).
4. The callback checks `threadDone` (acquire read):
   - If `threadDone==1`: the capture thread has already exited and closed its
     duplicate without signaling. The callback publishes terminal
     `CANCELLED`, signals+closes `callbackNotify`, and releases its strong ref.
   - If `threadDone==0`: the callback stores a pending cancel cause, signals
     `captureThreadWakeEvent`, closes `callbackNotify` without signaling, and
     releases its strong ref. The capture thread performs cleanup, publishes
     terminal, then signals+closes its capture-thread duplicate.
5. **The callback releases its strong operation reference** as its final action before returning. If Go has already called `CaptureRelease` (dropping the registry ref), this release causes the ref-count to reach zero and the session's destructor fires on the callback's thread.
6. The callback returns to the OS.

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
    uint32_t version;       // 2 (R13-1: added `ready` field)
    uint32_t ready;         // Acquire-loaded copy of session.mtaReady.
                            //   Monotonic: 0 before successful MTA init, 1
                            //   thereafter. When public state==preparing(0),
                            //   ready==0 means PREPARING and ready==1 means
                            //   PREPARED/CaptureActivate legal. In later
                            //   states it remains 1. Distinct from `valid`.
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
- The helper calls `IAudioCaptureClient::GetBuffer`, converts the packet to float32 into a scratch buffer, calls `ReleaseBuffer` to return the WASAPI buffer, then commits the scratch to the recording ring (publishes producer index) only after `ReleaseBuffer` succeeds (R10-3). If `ReleaseBuffer` fails, zero frames become visible to the consumer.
- **Silent-buffer handling**: WASAPI may report `AUDCLNT_BUFFERFLAGS_SILENT`; the helper fills the scratch buffer with zeros, calls `ReleaseBuffer`, then commits zeros to the ring. Same release-before-commit ordering.

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

The helper's overflow sequence follows the §Normative cleanup path (corrected per R4-3 for acquired-packet safety, R11-1 for single normative algorithm):

1. **Release the currently acquired WASAPI packet** via `ReleaseBuffer(frames)` — a buffer must never remain acquired. Cleanup `ReleaseBuffer` HRESULT is logged; the terminal reason remains `CAP_REASON_OVERFLOW`.
2. **Internal-failure CAS** (R11-2): pack `state=STOPPING` + `sealed=0` + `reason=CAP_REASON_OVERFLOW` and CAS the packed word. If a concurrent `CaptureRequestStop` installs a higher-priority reason between load and CAS, the retry captures it. On success, proceed to seal.
3. **Packed CAS seal**: pack `state=SEALED` + `sealed=1` + snapshot reason. CAS. On retry, capture any interleaved higher-priority reason.
4. Call one cleanup function (R12-5): `Stop` (only if `started`), release `IAudioCaptureClient` (only if `serviceAcquired`), `CoTaskMemFree(pMixFormat)` (only if `mixFormatOwned`), release `IAudioClient` (only if `audioClientOwned`). For running-stream paths, `mixFormatOwned` is already false (freed after `Initialize` — R11-4).
5. `CoUninitialize()`.
6. `threadDone=1` (atomic release — cleanup complete; one terminal store remains).
7. `atomic_thread_fence(memory_order_seq_cst)`.
8. Publish terminal `FAILED` with sealed reason and HRESULT `0x8007006F` (atomic release — final session-state access).
9. `SetEvent(localNotify)` — Go sees terminal via `CaptureGetResult`.
10. Go sees the failure, deletes the `.partial` file (and its `.partial.reason` sidecar — R11-5), and reports the error. Go never promotes a partial from a session that terminated with overflow.

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

#### Capture (async, two-step — R12-4)

```c
// Step 1: Prepare capture — create the operation and capture thread.
// Returns S_OK and writes opId. The capture thread initializes MTA
// and signals notifyEvent when ready (or on failure).
// No UI-thread blocking — returns immediately (R12-4).
// Eagerly DuplicateHandle(notifyEvent) for the capture thread BEFORE
// publishing the operation/thread (R14-2). If DuplicateHandle fails,
// returns the failure HRESULT, creates no operation and no thread, and
// does NOT write *opId.
// Only one capture session may be active at a time. Preparing a second
// returns E_NOT_VALID_STATE without creating a new operation.
HRESULT __stdcall CapturePrepare(HANDLE notifyEvent,
                                 uint32_t *opId);

// Step 2: Activate capture on the given device ID (UI thread for consent).
// Must be called only after CapturePrepare succeeds and the waiter
// observes MTA-ready state via CaptureGetResult (public state==0
// preparing with format->ready==1, i.e. private packed state PREPARED
// — R14-1). While still PREPARED, eagerly duplicates the stored Go event
// for callback ownership. Duplicate failure returns directly and leaves
// PREPARED. It then atomically CASes PREPARED→ACTIVATING; a lost CAS closes
// the callback duplicate and returns E_NOT_VALID_STATE. Only a winning CAS
// launches ActivateAudioInterfaceAsync, returning immediately. If the packed
// state is not PREPARED (still PREPARING, already ACTIVATING, or
// STOPPING/terminal after a cancel), returns E_NOT_VALID_STATE.
// If deviceId is null, returns E_POINTER.
// Synchronous activation-launch failures are stored as pending causes
// (async — Go queries via CaptureGetResult); because no callback will fire,
// CaptureActivate closes the callback duplicate without signaling first.
HRESULT __stdcall CaptureActivate(uint32_t opId,
                                  const wchar_t *deviceId);

// Query the capture session state and results.
// state: 0=preparing (MTA init in progress — R12-4), 1=activating,
//   2=capturing, 3=stopped, 4=failed, 5=cancelled.
// `ready` is copied from session.mtaReady using an acquire load; the worker
// never retains or mutates this caller-owned CaptureFormat. mtaReady is
// monotonic: 0 before successful MTA init, 1 thereafter (including
// ACTIVATING/CAPTURING/terminal states). The public state==0 collapses the two
// private packed states PREPARING and PREPARED; ready disambiguates them.
// On state==0 (preparing) with format->ready==0: private PREPARING —
//   MTA init in progress. format->valid==0. CaptureActivate is not yet legal
//   (returns E_NOT_VALID_STATE).
// On state==0 (preparing) with format->ready==1: private PREPARED — MTA is
//   ready — the capture thread completed CoInitializeEx(MTA) and is waiting
//   for the activation handoff. The waiter may now post WM_APP+ACTIVATE_READY
//   to the UI thread, and CaptureActivate is now legal. format->valid is
//   still 0 (no format negotiated yet).
//   (R13-1: `ready` is a documented field in the CaptureFormat struct —
//   see §Versioned format struct — set when MTA init succeeds. It removes
//   the prior ambiguity where state==0 + valid==0 meant both "still
//   initializing" and "MTA ready.") In later states ready remains 1 and
//   valid alone reports negotiated-format validity.
// On state>=2 AND format->valid==1: format is populated with the
//   negotiated capture format. On failed activation:
//   format->valid==0 and all other format fields are zero.
// On state=2: framesAvailable > 0 means CaptureRead will return data.
// On state>=3: hresult contains the terminal HRESULT;
//   *terminalReason contains the CAP_REASON_* enum value.
//   Public state mapping (R12-1): 3=stopped (finalizable reasons),
//   4=failed (non-promotable), 5=cancelled.
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
// Unknown/released opId returns S_OK as an idempotent no-op; query/read calls
// use E_HANDLE. Invalid reason returns E_INVALIDARG without changing state.
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
// CapDestroy returns E_NOT_VALID_STATE. If RoInitialize succeeds but
// a later step fails (e.g. operation registry allocation), CapInit
// calls RoUninitialize before returning the failure HRESULT — no
// partial state is left (R9-6). A fully failed CapInit (RoInitialize
// itself fails) also leaves no state. In both cases: no thread ID
// stored, no RoUninitialize needed from the caller.
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

All WinRT-using threads must be initialized. The UI thread (Go's main goroutine, pinned via `runtime.LockOSThread`) is the thread from which `CapInit`, `CapturePrepare`, `CaptureActivate`, `PickerOpenFile`, and other UI-thread exports are called. `CapInit` initializes the WinRT apartment on this thread:

1. `CapInit` calls `RoInitialize(RO_INIT_SINGLETHREADED)` (value 0) internally. The C++/WinRT equivalent is `winrt::init_apartment(winrt::apartment_type::single_threaded)`.
2. Accepted return values:
   - `S_OK` (0): first initialization on this thread. `CapDestroy` must call `RoUninitialize`.
   - `S_FALSE` (1): the apartment was already initialized (compatible mode). **Still balanced** — `RoUninitialize` is called in `CapDestroy` even for `S_FALSE`, because the COM apartment model uses a per-thread reference count and every successful `RoInitialize` must be balanced.
3. Rejected return value:
   - `RPC_E_CHANGED_MODE` (`0x80010106`): the thread was already initialized as MTA (`COINIT_MULTITHREADED`). This is incompatible with STA WinRT UI objects (e.g. `FileOpenPicker`). `CapInit` returns this HRESULT to Go, which logs the failure and refuses to use the helper.
4. `CapDestroy` calls `RoUninitialize` on the same UI thread to balance the `RoInitialize`. `CapDestroy` from a different thread returns `RPC_E_WRONG_THREAD` (R8-7).
5. Repeated `CapInit`/`CapDestroy` cycles: each `CapInit` increments the apartment ref count; each `CapDestroy` decrements it. No leaked apartment refs.
6. **State machine** (R8-7): a second `CapInit` before `CapDestroy` returns `E_NOT_VALID_STATE`. A failed `CapInit` (e.g. `RPC_E_CHANGED_MODE`) leaves no state — no `RoUninitialize` is needed, no thread ID is stored, and `CapDestroy` returns `S_OK` (no-op — nothing to destroy). Idempotent `CapDestroy` after success returns `S_OK`.
7. **Init rollback** (R9-6): `CapInit` may fail after `RoInitialize` succeeds (e.g. operation-registry allocation fails, internal state setup fails). In this case, `CapInit` calls `RoUninitialize` before returning the failure HRESULT. No partial state is left — no thread ID is stored, no `CapDestroy` is needed. The rollback sequence is: (a) `RoInitialize` succeeds → (b) later step fails → (c) `RoUninitialize` → (d) return failure HRESULT.
8. **Required tests** (R8-7, R9-6): `S_OK` init + destroy cycle; `S_FALSE` init (already initialized) + destroy; `RPC_E_CHANGED_MODE` init → verify no state left, `CapDestroy` is `S_OK`; repeated `CapInit` without destroy → `E_NOT_VALID_STATE`; wrong-thread `CapDestroy` → `RPC_E_WRONG_THREAD`; double `CapDestroy` → second is `S_OK`; re-init after `CapDestroy` → `S_OK`; **partial CapInit failure** (inject allocation failure after `RoInitialize` succeeds) → verify `RoUninitialize` is called, no thread ID stored, `CapDestroy` returns `S_OK` (no-op).

### Ownership rules

- **Operation IDs**: `uint32_t` returned by initiate exports. The caller must eventually call the corresponding `*Release` export after terminal state. The ID is invalid after release. Using a released/unknown ID returns `S_OK` (no-op) for release calls, `E_HANDLE` for query/read calls.
- **OS kernel `HANDLE`s** (events, picked file handle): distinct from opaque operation IDs. Events are created by Go (`CreateEvent`) and passed to the helper; Go owns their lifetime and must not close them before calling the corresponding release. Picked file handles are created by the helper and transferred to Go on query; Go must `CloseHandle` them.
- **UTF-16 buffers**: always caller-allocated, with explicit size parameters. For device/ID string exports (`CapGetDeviceInfo`, `CapGetDefaultDeviceResult`): the helper writes up to `bufLen - 1` characters plus a null terminator and returns `E_NOT_SUFFICIENT_BUFFER` if too small. For picker name buffers (`PickerGetResult`): the helper truncates to `nameBufLen - 1` + null and returns `S_OK` with `requiredNameChars` indicating the full size needed — Go uses the size-discovery step (`takeHandle=0`) to allocate correctly before the take step. Maximum string sizes: 512 `wchar_t` for device IDs/names, 260 `wchar_t` for file names.
- **PCM buffers**: caller-allocated `float*` in `CaptureRead`. The helper copies converted float32 from its internal ring into the caller's buffer. The caller owns the buffer after return.
- **Notification events**: Go creates/owns the original events and waits via
  `WaitForMultipleObjects`. Before publishing async work, the helper creates
  owner-specific duplicates; workers signal/close only those duplicates. The
  helper never signals or closes Go's originals.
- **Idempotent stop**: `CaptureRequestStop` is safe to call in any state — before activation completes, during capture, after stop, or on an unknown/released ID (no-op).
- **Thread safety**: `CapPermissionCheck` and `CaptureRead` may be called from any thread. `CapturePrepare`, `CaptureActivate`, and `PickerOpenFile` must be called from the UI thread (R12-4). `CapInit` and `CapDestroy` must be called from the UI thread. Result query and release exports may be called from any thread.
- **Maximum operation counts**: at most 1 active capture session, 1 active picker, 1 active permission request, 1 active enumeration, 1 active default-device query. Exceeding returns `E_NOT_VALID_STATE`.
- **Request cancellation** (R15-1): `CaptureRequestStop(opId, cancel)` stops a
  pending capture. Picker, permission, and enumeration wrappers expose
  cooperative `IAsyncInfo::Cancel` requests during graceful quit; they may
  still complete successfully or fail. The default-device wrapper has no
  `IAsyncInfo` because its underlying API is synchronous. In every case the
  waiter retains ownership until the actual wrapper terminal result and
  release.

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
PREPARING → PREPARED → ACTIVATING → CAPTURING → STOPPING → SEALED → TERMINAL
    │           │            │           │
    └───────────┴────────────┴───────────┘
         (any non-terminal state) → STOPPING → SEALED → TERMINAL
```

(Private packed states — R14-1. `PREPARING`/`PREPARED` both surface publicly as
`preparing`(0), disambiguated by `format->ready`; `STOPPING`/`SEALED` are
private; `TERMINAL` maps to public `stopped`/`failed`/`cancelled` by sealed
reason. See §Packed atomic compare-and-swap.)

### Signal-to-action mapping

| Signal | Source | Capture action | Draft action | Network | Window procedure |
|---|---|---|---|---|---|
| User Stop (hotkey, menu, UI) | Go shell | `CaptureRequestStop(opId, user_stop)` | Finalize valid draft (see §Crash-safe interrupted-draft handling) | None | Returns immediately |
| `WM_QUERYENDSESSION` | OS (logoff/shutdown) | `CaptureRequestStop(opId, shutdown)` | Finalize if possible; `.partial` remains if not | None — must not block wndproc | Return `TRUE` (allow shutdown) |
| `WM_ENDSESSION` (wParam=TRUE) | OS (confirmed shutdown) | `CaptureRequestStop(opId, shutdown)` (idempotent — already sent from `WM_QUERYENDSESSION`); signal `shutdownEvent` (manual-reset — wakes waiter for best-effort drain). Do **not** call `CapDestroy` — OS reclaims process resources (R4-2, R10-5). | Same as above | None | Return 0 |
| `WM_POWERBROADCAST` / `PBT_APMSUSPEND` | OS (sleep) | `CaptureRequestStop(opId, suspend)` | Finalize if possible; `.partial` remains if not | None | Return `TRUE` |
| `WM_POWERBROADCAST` / `PBT_APMRESUMEAUTOMATIC` | OS (wake from sleep) | No auto-restart | None | None | Return `TRUE` |
| `WM_WTSSESSION_CHANGE` / `WTS_SESSION_LOCK` | OS (lock screen) | `CaptureRequestStop(opId, lock)` | Finalize if possible; `.partial` remains if not | None | Return 0 |
| `WM_WTSSESSION_CHANGE` / `WTS_SESSION_UNLOCK` | OS (unlock) | No auto-restart; recheck device/permission | Startup cleanup recovers or discards `.partial` files | None | Return 0 |
| `AppCapability.AccessChanged` (denied) | WinRT event → notifyEvent | `CaptureRequestStop(opId, permission_revoke)` | Discard (permission lost mid-capture) | None | N/A (event handler, not wndproc) |
| Device invalidation (WASAPI error) | Capture thread `GetBuffer`/`GetNextPacketSize` fails | Capture thread seals reason via packed CAS (R10-4), performs cleanup, publishes terminal after `threadDone` barrier (R10-1) | Finalize if ≥ min duration; discard otherwise | None | N/A |
| Late MTA callback after cancel | `ActivateCompleted` fires after `CaptureRequestStop` | Callback reads packed state, calls `GetActivateResult`, releases interfaces, clears async-op ref. If `threadDone==1`, callback publishes terminal and signal+closes `callbackNotify` (thread already closed its duplicate without signal). Otherwise it stores pending cancel, closes `callbackNotify` without signal, and wakes the thread; the thread publishes and signal+closes its own duplicate. | N/A (no capture started) | None | N/A |

### Invariants

1. **No network operation blocks the window procedure.** All capture stop actions are non-blocking flag-sets + event signals. Upload/finalization that requires network is deferred to a non-wndproc context.
2. **No UI-thread deadlock.** `CaptureRequestStop` sets flags and signals events but never joins the capture thread synchronously. The capture thread cleans up independently. On `WM_ENDSESSION`, Go requests stop and returns from the wndproc without calling `CapDestroy` — the OS reclaims process resources (R4-2).
3. **Resume/unlock recheck:** after `WTS_SESSION_UNLOCK` or `PBT_APMRESUMEAUTOMATIC`, the shell rechecks device availability and permission status before allowing a new capture. It does not auto-restart a stopped capture.

### Stop-reason priority arbitration

*Addresses R6 finding 6.*

`CaptureRequestStop(opId, reason)` must allow higher-priority reasons to replace lower-priority ones even after stopping has begun. Otherwise a user-stop arriving before `AccessChanged` can win the reason and cause Go to finalize media even though permission was revoked before promotion. The fix is a packed atomic priority CAS (R10-4).

#### Priority order (highest to lowest)

| Priority | Reason | Value | Promotes draft? |
|---|---|---|---|
| 1 (highest) | `CAP_REASON_OVERFLOW` | 7 | Never — data integrity compromised |
| 2 | `CAP_REASON_DISCONTINUITY` | 10 | Never — data integrity compromised |
| 3 | `CAP_REASON_PERMISSION_REVOKE` | 1 | Never — recording not authorized |
| 4 | `CAP_REASON_WASAPI_ERROR` | 8 | Never — cause may include undetected permission loss |
| 4 (tie) | `CAP_REASON_FORMAT_ERROR` | 9 | Never — unsupported format |
| 5 | `CAP_REASON_DEVICE_LOST` | 2 | Yes, if ≥ min duration |
| 6 | `CAP_REASON_SHUTDOWN` | 3 | Yes, if ≥ min duration |
| 7 | `CAP_REASON_SUSPEND` | 4 | Yes, if ≥ min duration |
| 8 | `CAP_REASON_LOCK` | 5 | Yes, if ≥ min duration |
| 9 | `CAP_REASON_CANCEL` | 6 | Never — explicit cancel |
| 10 (lowest) | `CAP_REASON_USER_STOP` | 0 | Yes, if ≥ min duration |

#### Packed atomic compare-and-swap (R10-4)

*Replaces Rev 10's separate CAS + mutex seal, which had a race: a lock-free CAS could succeed after the mutex-protected reason snapshot but before the state flip, because the CAS does not take the mutex. Deterministic counterexample from R10: (1) request thread reads state STOPPING and pauses; (2) capture thread locks, snapshots USER_STOP, writes sealed reason, flips state, unlocks; (3) request thread resumes and CASes the separate reason atomic to PERMISSION_REVOKE — sealed result is stale.*

State, sealed bit, reason, and last public state are packed into a single `uint64_t` atomic:

```
Bits [63:56]  — lastPublicState (public value stored at the STOPPING transition — R11-2/R14-1:
                 preparing=0 (from private PREPARING or PREPARED), activating=1, capturing=2)
Bits [55:32]  — reserved (zero)
Bits [31:24]  — state (private FSM — R14-1):
                 PREPARING=0, PREPARED=1, ACTIVATING=2, CAPTURING=3,
                 STOPPING=4, SEALED=5, TERMINAL=6
Bit  [23]     — sealed flag (1 = reason is frozen)
Bits [22:16]  — reserved (zero)
Bits [15:0]   — reason (CAP_REASON_* value)
```

**Private preparation states are distinct from the public enum (R14-1).** The
private packed `state` field freezes two preparation states that the public
`CaptureGetResult` enum collapses into `preparing=0` (disambiguated only by the
`ready` flag in `CaptureFormat`):

| Private packed state | Meaning | Public `CaptureGetResult` state | `format->ready` |
|---|---|---|---|
| `PREPARING` (0) | `CapturePrepare` published; capture thread is inside `CoInitializeEx(MTA)` (blocked, un-interruptible) | `preparing` (0) | 0 |
| `PREPARED` (1) | MTA init succeeded; capture thread is blocked on `WaitForSingleObject(captureThreadWakeEvent)` awaiting the `CaptureActivate` handoff | `preparing` (0) | 1 |
| `ACTIVATING` (2) | Activation launched; awaiting callback | `activating` (1) | 1 |
| `CAPTURING` (3) | Format negotiated, `Start` succeeded | `capturing` (2) | 1 (`valid` reports format) |
| `STOPPING`/`SEALED` (4/5) | Stop requested / reason frozen | `lastPublicState` | 1 if MTA init succeeded, else 0 |
| `TERMINAL` (6) | Cleanup complete, terminal store published | `stopped`/`failed`/`cancelled` (3/4/5) | same monotonic session value |

The capture thread performs the state transitions via the packed CAS:
`CapturePrepare` publishes `PREPARING`; a successful `CoInitializeEx(MTA)`
CASes `PREPARING→PREPARED` and release-stores session-owned
`mtaReady=1`. `CaptureGetResult` acquire-loads it and copies it into the
caller's current `CaptureFormat`;
`CaptureActivate` CASes `PREPARED→ACTIVATING` (atomically requiring `PREPARED`,
returning `E_NOT_VALID_STATE` otherwise — §Exported ABI). The activation
callback only writes the `IAudioClient` handoff/pending cause; it does **not**
claim capture has started. The capture thread performs format validation,
`Initialize`, `GetService`, and `Start`, rechecking for `STOPPING` before each
irreversible/start step, then CASes `ACTIVATING→CAPTURING` only after `Start`
succeeds. If a stop won first, that CAS fails and the thread immediately follows
the sealed cleanup path without exposing `capturing`. A stop requested in
`PREPARING`, `PREPARED`, or `ACTIVATING` CASes to `STOPPING` and records
`lastPublicState` = the collapsed public value (`preparing`=0 for both
`PREPARING` and `PREPARED`; `activating`=1 for `ACTIVATING`). A stop requested
while `CoInitializeEx` is blocked is **latched** in the packed word and observed
by the capture thread the instant `CoInitializeEx` returns (the thread rechecks
the packed state before advancing to `PREPARED`), so it is never lost even
though the OS call itself cannot be interrupted.

**No separate `cancelled` bit** (R11-2, R12-1). Cancellation is represented as
`state=STOPPING` + `reason=CANCEL`. `lastPublicState` is written on every
nonterminal→`STOPPING` transition: public preparing(0) for private PREPARING or
PREPARED, activating(1) for ACTIVATING, and capturing(2) for CAPTURING. Thus
`CaptureGetResult` maps private `SEALED` without guessing.

**Public ABI state mapping from packed `TERMINAL`** (R12-1): the packed word's
state field uses private `TERMINAL=6`, never exposed directly by
`CaptureGetResult`. Private `SEALED=5` remains distinct. When the terminal
store is published, the query maps it to one of three public values:

| Sealed reason | Public ABI state | Value | Derivation |
|---|---|---|---|
| `CAP_REASON_USER_STOP`, `CAP_REASON_SHUTDOWN`, `CAP_REASON_SUSPEND`, `CAP_REASON_LOCK`, `CAP_REASON_DEVICE_LOST` | `stopped` | 3 | Finalizable reasons — session stopped normally or by external event |
| `CAP_REASON_PERMISSION_REVOKE`, `CAP_REASON_WASAPI_ERROR`, `CAP_REASON_FORMAT_ERROR`, `CAP_REASON_OVERFLOW`, `CAP_REASON_DISCONTINUITY` | `failed` | 4 | Non-promotable failures — data integrity or authorization compromised |
| `CAP_REASON_CANCEL` | `cancelled` | 5 | Explicit cancel request |

The public ABI state values match the single canonical enum defined in the `CaptureGetResult` ABI comment (§Exported ABI): `preparing`=0, `activating`=1, `capturing`=2, `stopped`=3, `failed`=4, `cancelled`=5. Terminal values (3, 4, 5) are disjoint from every nonterminal value (0, 1, 2) — no public value is reused across a terminal/nonterminal boundary (R13-1).

The public-state mapping is derived solely from the sealed reason value — no
redundant `promotable` boolean or separate state bits. The 64-bit packed word
contains the reason but deliberately does **not** contain a 32-bit HRESULT.
After the reason seal wins, the eventual terminal publisher writes
session-owned `sealedReason` and `sealedHresult` result fields before the
release-store of `TERMINAL`: exact-code reasons derive their HRESULT from the
canonical reason/HRESULT table; `WASAPI_ERROR` preserves the original negative
HRESULT already owned by the single capture failure path. The capture thread is
the sole internal-failure writer, so no second writer can detach an original
HRESULT from the winning reason. Diagram A is the only callback publisher and
can only synthesize the exact `CANCEL` row. `CaptureGetResult` first
acquire-loads `TERMINAL`, then copies these fields. Before terminal publication
it returns `lastPublicState`, zero `hresult`/`terminalReason`, and the current
format validity. Thus the result fields are ordered by the terminal release/
acquire edge without pretending they occupy bits absent from the packed layout.

All updates — `CaptureRequestStop` priority CAS, the internal-failure CAS (R11-2), and the capture thread's seal — operate on the same packed word via `InterlockedCompareExchange64`. No mutex is needed for the linearization.

**`CaptureRequestStop`** uses an atomic compare-and-swap on the packed word:

1. Load the packed word (acquire).
2. If state is `TERMINAL` or `SEALED` or sealed bit is set: no-op (reason is frozen). Return `S_OK`.
3. If state is `PREPARING`, `PREPARED`, or `ACTIVATING` (R14-1 — cancellation is legal during MTA init and before/at the activation handoff): pack `lastPublicState` = the collapsed public value of the current state (`preparing`=0 for `PREPARING`/`PREPARED`; `activating`=1 for `ACTIVATING`), `state=STOPPING` + `sealed=0` + new reason. CAS the packed word. On failure, retry from step 1 (another CAS won; re-evaluate priority). On success, signal `captureThreadWakeEvent` (the capture thread is either blocked in `CoInitializeEx` — in which case the latched `STOPPING` is observed on its return before advancing to `PREPARED` — or blocked on `WaitForSingleObject(captureThreadWakeEvent)` in `PREPARED`/`ACTIVATING`, which the signal wakes). Return `S_OK`.
4. If state is `CAPTURING`: pack `lastPublicState=capturing` (2), `state=STOPPING` + `sealed=0` + new reason. CAS. On failure, retry from step 1. On success, signal `stopEvent` (the capture thread is in the `WaitForMultipleObjects` capture loop). Return `S_OK`.
5. If state is `STOPPING` and sealed=0: compare the new reason's priority against the packed reason. If the new reason has **higher priority** (lower number in the table), pack `state=STOPPING` + `sealed=0` + new reason (preserving `lastPublicState` — already stored). CAS. On failure, retry from step 1. On success, return `S_OK`. If equal or lower priority, no-op.

There is no `IDLE` state in the frozen FSM (R14-1): a session begins at
`PREPARING` the moment `CapturePrepare` publishes the operation, so
`CaptureRequestStop` never sees a pre-`PREPARING` word for a valid `opId`. An
unknown/released `opId` returns `S_OK` as the frozen idempotent stop no-op;
query/read calls on that ID return `E_HANDLE`, and an invalid reason on a valid
ID returns `E_INVALIDARG` before the packed word is changed.

**Internal-failure CAS** (R11-2): overflow, discontinuity, conversion failure, and WASAPI failure arise in `CAPTURING` state. The capture thread cannot directly seal because sealing requires `state==STOPPING`. The internal-failure CAS first transitions to `STOPPING`:

1. Load the packed word (acquire). State must be `CAPTURING` (or may have become `STOPPING` from a concurrent external stop — handle below).
2. If state is `CAPTURING`: pack `lastPublicState=CAPTURING`, `state=STOPPING` + `sealed=0` + failure reason. CAS. On failure, retry from step 1 (concurrent `CaptureRequestStop` may have already moved to `STOPPING`).
3. If state is already `STOPPING` (concurrent external stop won): compare priorities. If the internal failure has higher priority, CAS to install the new reason (preserving `state=STOPPING`, `lastPublicState`). If equal or lower, proceed with the existing reason.
4. If state is `SEALED` or `TERMINAL`: the session is already sealing/sealed — no-op (impossible in normal flow, defensive only).
5. After reaching `STOPPING`, proceed to the seal CAS (below).

The capture thread's **seal CAS**:

1. Load the packed word (acquire). State must be `STOPPING` and sealed=0.
2. Pack `state=SEALED` + `sealed=1` + snapshot reason + preserve `lastPublicState` from the loaded word.
3. CAS. On failure, retry from step 1 (a higher-priority reason was installed between load and CAS — the retry captures it). On success, the reason is frozen.
4. The `SEALED` state is **private**: `CaptureGetResult` maps `SEALED` → `lastPublicState` from the packed word (R11-2). `CaptureGetResult` never exposes `SEALED` — the public terminal value appears only after the `threadDone` → fence → terminal store sequence completes.

After the packed CAS seals, the capture thread follows the §Normative cleanup path: conditional cleanup per ownership flags → `CoUninitialize` → `threadDone=1` (atomic release) → `atomic_thread_fence(seq_cst)` → publish terminal (atomic release — FINAL session-state access) → `SetEvent(localNotify)`.

This eliminates the race because every reason update and the seal are on the same atomic word: a CAS that succeeds after the seal observes `SEALED`+sealed=1 and returns no-op (step 2 of `CaptureRequestStop`). A CAS that succeeds before the seal installs its reason into the word the seal will snapshot (step 1 of the seal CAS retries until it captures the latest value).

#### Wake events per source state (R11-2)

| Source state at `CaptureRequestStop` | Wake event signaled | Why |
|---|---|---|
| `PREPARING` | `captureThreadWakeEvent` | Capture thread is inside `CoInitializeEx(MTA)` (un-interruptible). The latched `STOPPING` is observed the instant the call returns, before advancing to `PREPARED`; the signal also covers the immediately-following `PREPARED` wait so no stop is lost (R14-1) |
| `PREPARED` | `captureThreadWakeEvent` | Capture thread completed MTA init and is blocked on `WaitForSingleObject(captureThreadWakeEvent)` awaiting the `CaptureActivate` handoff (R14-1) |
| `ACTIVATING` | `captureThreadWakeEvent` | Capture thread is blocked on `WaitForSingleObject(captureThreadWakeEvent)` waiting for activation handoff |
| `CAPTURING` | `stopEvent` | Capture thread is in the `WaitForMultipleObjects` capture loop, which includes `stopEvent` |
| `STOPPING` (priority upgrade) | Neither (already woken) | The thread is already awake and processing the stop |

#### Deterministic transition tests (R11-2)

1. **PREPARING→PREPARED→ACTIVATING→CAPTURING→STOPPING→SEALED→TERMINAL** (happy path): verify each transition; session `mtaReady` release-stores 0→1 and every later `CaptureGetResult` acquire-load copies ready=1; verify `lastPublicState==capturing`(2) at seal.
1a. **PREPARING→STOPPING (cancel during MTA init)** (R14-1): install `CaptureRequestStop(cancel)` while the capture thread is blocked in `CoInitializeEx`. Verify the packed word holds `STOPPING`+`CANCEL`, `lastPublicState==preparing`(0), `captureThreadWakeEvent` signaled. Release the injected `CoInitializeEx` barrier; verify the thread observes the latched `STOPPING` on return (does **not** advance to `PREPARED`), calls `CoUninitialize`, and publishes terminal `cancelled`(5). `CaptureGetResult` returns public `preparing`(0) until the terminal store.
1b. **PREPARED→STOPPING (cancel after MTA-ready, before CaptureActivate)** (R14-1): drive the thread to `PREPARED` (`format->ready==1`), then `CaptureRequestStop(cancel)`. Verify `STOPPING`+`CANCEL`, `lastPublicState==preparing`(0), `captureThreadWakeEvent` signaled, `CaptureActivate` now returns `E_NOT_VALID_STATE` (state no longer `PREPARED`), thread publishes terminal `cancelled`(5).
2. **ACTIVATING→STOPPING (cancel)**: verify `CaptureRequestStop(cancel)` installs `STOPPING`+`CANCEL`, `lastPublicState==activating`(1), signals `captureThreadWakeEvent`.
2a. **Stop after callback handoff but before `Start`**: barrier the capture
thread before each startup stage (`GetMixFormat`, `Initialize`, `SetEventHandle`,
`GetService`, `Start`) and race stop. Verify the next acquire check sees
STOPPING, skips all later startup calls, cleans only acquired resources, never
publishes `capturing`, and terminal public pre-cleanup state remains
`activating`(1). Also stop while `Start` is executing: after successful return,
the `ACTIVATING→CAPTURING` CAS must lose and cleanup must call `Stop` because
`started=true`.
3. **CAPTURING→STOPPING (user stop)**: verify `CaptureRequestStop(user_stop)` installs `STOPPING`+`USER_STOP`, `lastPublicState==CAPTURING`, signals `stopEvent`.
4. **CAPTURING→STOPPING (internal overflow)**: verify internal-failure CAS installs `STOPPING`+`OVERFLOW`, `lastPublicState==CAPTURING`.
5. **Internal overflow vs concurrent user stop**: race test — overflow CAS and `CaptureRequestStop(user_stop)` execute simultaneously. Verify: overflow wins (higher priority). `lastPublicState==CAPTURING`.
6. **Internal overflow vs concurrent permission revoke**: overflow and permission_revoke execute simultaneously. Verify: overflow wins (priority 1 > priority 3 — R13-7: the canonical priority table assigns overflow priority 1, discontinuity priority 2, and permission_revoke priority 3; these three are the only non-promotable-integrity/authorization reasons that dominate all finalizable reasons).
7. **CAPTURING→STOPPING (permission revoke) then internal overflow**: `CaptureRequestStop(permission_revoke)` installs first, then internal overflow CAS fires. Verify: overflow wins (priority 1 > priority 3).
7b. **Internal overflow vs concurrent discontinuity** (R12-6): overflow CAS and discontinuity CAS race. Verify: overflow wins (priority 1 > priority 2). Both are non-promotable; the distinction is immaterial for promotion but must be deterministic for evidence logging.
8. **STOPPING→SEALED linearization edge**: two threads both attempt seal CAS — only one succeeds. Verify deterministic.
9. **CaptureGetResult during SEALED**: verify maps to `lastPublicState` (never exposes `SEALED`).
10. **CaptureGetResult during TERMINAL**: verify maps to terminal value.
11. **ACTIVATING cancel wakeup**: verify `captureThreadWakeEvent` is signaled, NOT `stopEvent`.
12. **Double stop (idempotent)**: `CaptureRequestStop` called twice with same reason. Second is no-op.
13. **Equal-ranked wasapi_error vs format_error tie** (R13-7): install `wasapi_error` (rank 4) first, then attempt to install `format_error` (rank 4). Verify: the second install is a no-op (step 5 of the packed CAS no-ops on equal priority), so the first-sealed reason (`wasapi_error`) wins. Run the reverse schedule (format_error first) and verify format_error wins. Both are non-promotable, so promotion is unaffected; the test asserts deterministic evidence logging.
14. **permission_revoke vs user_stop** (R13-7): install `user_stop` (rank 10) then `permission_revoke` (rank 3). Verify permission_revoke wins (3 < 10); reverse schedule (permission_revoke then user_stop) — user_stop is lower priority, no-op, permission_revoke retained.

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
| Any other negative HRESULT from `GetNextPacketSize`/`GetBuffer`/`ReleaseBuffer`/`Start` | — | Unknown failure | `CAP_REASON_WASAPI_ERROR` (**non-promotable** — Go never promotes a draft with this reason) |
| Any negative HRESULT from `Stop` | — | Cleanup failure — logged but does NOT override terminal reason (preserve-original — R12-5) | Original terminal reason preserved; `Stop` failure does not change promotability |

**HRESULT mapping scope** (R8-4, R10-6): the mapping applies to errors from **all** WASAPI and COM calls in the capture lifecycle — `ActivateAudioInterfaceAsync` activation (including `E_ACCESSDENIED`), `GetMixFormat`, format validation, `Initialize`, `GetBufferSize`, `SetEventHandle`, `GetService`, `Start`, `GetNextPacketSize`, `GetBuffer`, `ReleaseBuffer`, and `Stop`. Each call's failure HRESULT is looked up in this table. See §Complete HRESULT/cleanup table for per-call frozen outcomes.

**Packed atomic terminal-reason seal** (R8-4, R9-4, R10-4): state, sealed bit, and reason are packed into one `uint64_t` atomic. Both `CaptureRequestStop` priority updates and the capture thread's seal use `InterlockedCompareExchange64` on the same packed word. No mutex is needed — the packed CAS is the sole linearization mechanism. See §Packed atomic compare-and-swap (R10-4) for the full protocol.

The sequence after the packed CAS seals (`SEALED` + sealed=1 + final reason): `Stop` → release services → release `IAudioClient` → `CoUninitialize` → `threadDone=1` (atomic release — cleanup complete) → `atomic_thread_fence(seq_cst)` → publish terminal (atomic release — final session-state access; `CaptureGetResult` acquire-reads this) → `SetEvent(localNotify)`. `CaptureGetResult` maps `SEALED` → the last public pre-cleanup state; the public terminal value appears only after the terminal store.

**Removed** (R7-2): `AUDCLNT_E_NOT_ALLOWED` — this name does not correspond to a documented WASAPI SDK constant. The value `0x88890003` is `AUDCLNT_E_WRONG_ENDPOINT_TYPE` per the Windows SDK headers; misclassifying it as a privacy-revocation signal would destroy evidence for the wrong reason.

**Unknown-error policy is non-promotable, not fake-permission-revoke** (R7-2). Unknown audio failures map to `CAP_REASON_WASAPI_ERROR`, not `CAP_REASON_PERMISSION_REVOKE`. The actual HRESULT that Windows returns when microphone privacy is revoked mid-capture is a **mandatory probe discovery** — the hardware probe must capture the exact HRESULT on Windows 10 and 11 after toggling the microphone privacy setting in System Settings during active capture. Until that evidence exists, only `E_ACCESSDENIED` is classified as permission loss. All other unknown errors are non-promotable via `CAP_REASON_WASAPI_ERROR`, which achieves the same fail-closed property for promotion without misattributing the cause in evidence logs.

#### Complete HRESULT/cleanup table for every COM/WASAPI stage (R10-6, R11-4)

*Addresses R10 finding 6, R11 finding 4, R12-5. One normative table covering every call from activation through `Stop`. Cleanup derived from ownership flags `audioClientOwned`, `mixFormatOwned`, `serviceAcquired`, `started` (R11-4, R12-5). One cleanup function consumes each flag exactly once.*

**Ownership flags** (R11-4, R12-5): the capture thread maintains four boolean flags that determine which cleanup steps are legal. The one cleanup function consumes and clears each flag exactly once — table rows set cause/flags and call that function, not restate its body.

| Flag | Set when | Cleared when | Governs |
|---|---|---|---|
| `audioClientOwned` (R12-5, renamed from `initialized`) | `IAudioClient` is obtained (activation handoff succeeds) | Cleanup function calls `IAudioClient::Release()` | Whether `IAudioClient::Release()` is called |
| `mixFormatOwned` (R12-5, new) | `GetMixFormat` returns `S_OK` with a non-null `pMixFormat` | Cleanup function calls `CoTaskMemFree(pMixFormat)` and sets pointer to null | Whether `CoTaskMemFree(pMixFormat)` is called |
| `serviceAcquired` | `GetService(IAudioCaptureClient)` succeeds | Cleanup function calls `IAudioCaptureClient::Release()` | Whether `IAudioCaptureClient::Release()` is called |
| `started` | `IAudioClient::Start()` succeeds | Cleanup function calls `IAudioClient::Stop()` | Whether `IAudioClient::Stop()` is called |

**`pMixFormat` lifetime** (R11-4, R12-5): the mix format pointer from `GetMixFormat` is COM-allocated (`CoTaskMemAlloc`) and must be freed with `CoTaskMemFree` `[MS-48]`. The `mixFormatOwned` flag tracks whether `pMixFormat` is live. The capture thread copies the needed fields (sample rate, channels, bits per sample, block alignment, valid bits per sample, subformat GUID) to session-local storage **immediately after `Initialize` succeeds**, then calls `CoTaskMemFree(pMixFormat)`, sets `pMixFormat=null`, and clears `mixFormatOwned`. After this point, running-stream cleanup rows have no allocation to leak. If `Initialize` fails, `pMixFormat` is freed by the cleanup function (which checks `mixFormatOwned`). On `GetMixFormat` failure, the docs state `*ppDeviceFormat` is null `[MS-48]`; `mixFormatOwned` is not set.

**One cleanup function** (R12-5): the capture thread calls a single cleanup function that consumes flags in this order:
1. If `started`: `IAudioClient::Stop()`. HRESULT logged; does not override terminal reason (preserve-original is the sole rule — R12-5). Clear `started`.
2. If `serviceAcquired`: `IAudioCaptureClient::Release()`. Clear `serviceAcquired`.
3. If `mixFormatOwned`: `CoTaskMemFree(pMixFormat)`, set `pMixFormat=null`. Clear `mixFormatOwned`.
4. If `audioClientOwned`: `IAudioClient::Release()`. Clear `audioClientOwned`.

This function is called from the §Normative cleanup path step 4. Table rows below set the terminal reason and the applicable flags, then invoke this function — they do not restate its body.

Each row specifies: the call, terminal state/reason/HRESULT, whether format is valid, whether any PCM exists, promotability, ownership flags set, exact cleanup per flags, and final terminal publisher. All cleanup follows the §Normative cleanup path.

| Call | Failure HRESULT | Terminal reason | Format valid? | PCM exists? | Promotable? | Flags set | Cleanup per flags | Final publisher |
|---|---|---|---|---|---|---|---|---|
| `ActivateAudioInterfaceAsync` (sync launch) | Any failure HRESULT | `CAP_REASON_WASAPI_ERROR` | No | No | Never | None | UI stores pending cause → capture thread: `CoUninitialize` → `threadDone` → fence → terminal → `localNotify` | Capture thread |
| Activation callback `GetActivateResult` | `E_ACCESSDENIED` (`0x80070005`) | `CAP_REASON_PERMISSION_REVOKE` | No | No | Never | None | Callback: release returned interface (if any), clear async-op ref, store pending cause → capture thread: `CoUninitialize` → `threadDone` → fence → terminal → `localNotify`; or callback publishes if `threadDone==1` | Capture thread (or late callback) |
| Activation callback `GetActivateResult` | Any other failure | `CAP_REASON_WASAPI_ERROR` | No | No | Never | None | Same as above | Capture thread (or late callback) |
| `GetMixFormat` | Any failure | `CAP_REASON_WASAPI_ERROR` | No | No | Never | `audioClientOwned` | Set flags: `audioClientOwned`, `mixFormatOwned` (if `GetMixFormat` returned non-null). Invoke cleanup function → `CoUninitialize` → §Normative cleanup path steps 5–11 | Capture thread |
| Format validation (subtype check) | N/A (app-level) | `CAP_REASON_FORMAT_ERROR`, HRESULT=`E_INVALIDARG` | No (rejected) | No | Never | `audioClientOwned` | Set flags: `audioClientOwned`, `mixFormatOwned`. Invoke cleanup function → `CoUninitialize` → §Normative cleanup path steps 5–11 | Capture thread |
| `Initialize` (shared mode) | `E_ACCESSDENIED` | `CAP_REASON_PERMISSION_REVOKE` | Yes (format was valid) | No | Never | `audioClientOwned` | Set flags: `audioClientOwned`, `mixFormatOwned`. Invoke cleanup function → `CoUninitialize` → §Normative cleanup path steps 5–11 | Capture thread |
| `Initialize` (shared mode) | `AUDCLNT_E_DEVICE_INVALIDATED` | `CAP_REASON_DEVICE_LOST` | Yes | No | Never | `audioClientOwned` | Same as above | Capture thread |
| `Initialize` (shared mode) | Any other failure | `CAP_REASON_WASAPI_ERROR` | Yes | No | Never | `audioClientOwned` | Same as above | Capture thread |
| `Initialize` success | — | — | Yes | No | — | `audioClientOwned` | **`CoTaskMemFree(pMixFormat)` called here** — fields copied to session state. `pMixFormat` is null for all subsequent paths. | — |
| `GetBufferSize` | Any failure | `CAP_REASON_WASAPI_ERROR` | Yes | No | Never | `audioClientOwned` | Set flags: `audioClientOwned` (no `mixFormatOwned` — freed at `Initialize` success). Invoke cleanup function → `CoUninitialize` → §Normative cleanup path steps 5–11 | Capture thread |
| `SetEventHandle` | Any failure | `CAP_REASON_WASAPI_ERROR` | Yes | No | Never | `audioClientOwned` | Same as above | Capture thread |
| `GetService(IAudioCaptureClient)` | Any failure | `CAP_REASON_WASAPI_ERROR` | Yes | No | Never | `audioClientOwned` | Set flags: `audioClientOwned` (no `serviceAcquired` — `GetService` failed). Invoke cleanup function → `CoUninitialize` → §Normative cleanup path steps 5–11 | Capture thread |
| `Start` | `E_ACCESSDENIED` | `CAP_REASON_PERMISSION_REVOKE` | Yes | No | Never | `audioClientOwned`, `serviceAcquired` | Invoke cleanup function (no `Stop` — `started` not set) → `CoUninitialize` → §Normative cleanup path steps 5–11. | Capture thread |
| `Start` | Any other failure | `CAP_REASON_WASAPI_ERROR` | Yes | No | Never | `audioClientOwned`, `serviceAcquired` | Same as above | Capture thread |
| `GetNextPacketSize` | `E_ACCESSDENIED` | `CAP_REASON_PERMISSION_REVOKE` | Yes | Maybe | Never | `audioClientOwned`, `serviceAcquired`, `started` | Invoke cleanup function (`Stop` + releases per flags) → `CoUninitialize` → §Normative cleanup path steps 5–11 | Capture thread |
| `GetNextPacketSize` | `AUDCLNT_E_DEVICE_INVALIDATED` | `CAP_REASON_DEVICE_LOST` | Yes | Maybe | Yes, if ≥ min duration | `audioClientOwned`, `serviceAcquired`, `started` | Same as above | Capture thread |
| `GetNextPacketSize` | Any other failure | `CAP_REASON_WASAPI_ERROR` | Yes | Maybe | Never | `audioClientOwned`, `serviceAcquired`, `started` | Same as above | Capture thread |
| `GetBuffer` | `E_ACCESSDENIED` | `CAP_REASON_PERMISSION_REVOKE` | Yes | Maybe | Never | `audioClientOwned`, `serviceAcquired`, `started` | Invoke cleanup function (`Stop` + releases per flags) → `CoUninitialize` → §Normative cleanup path steps 5–11 (no buffer acquired) | Capture thread |
| `GetBuffer` | `AUDCLNT_E_DEVICE_INVALIDATED` | `CAP_REASON_DEVICE_LOST` | Yes | Maybe | Yes, if ≥ min duration | `audioClientOwned`, `serviceAcquired`, `started` | Same as above | Capture thread |
| `GetBuffer` | Any other failure | `CAP_REASON_WASAPI_ERROR` | Yes | Maybe | Never | `audioClientOwned`, `serviceAcquired`, `started` | Same as above | Capture thread |
| `ReleaseBuffer` (normal commit path) | `E_ACCESSDENIED` | `CAP_REASON_PERMISSION_REVOKE` | Yes | Maybe (ring not committed) | Never | `audioClientOwned`, `serviceAcquired`, `started` | Invoke cleanup function (`Stop` + releases per flags) → `CoUninitialize` → §Normative cleanup path steps 5–11 | Capture thread |
| `ReleaseBuffer` (normal commit path) | `AUDCLNT_E_DEVICE_INVALIDATED` | `CAP_REASON_DEVICE_LOST` | Yes | Maybe (ring not committed) | Yes, if ≥ min duration | `audioClientOwned`, `serviceAcquired`, `started` | Same as above | Capture thread |
| `ReleaseBuffer` (normal commit path) | Any other failure | `CAP_REASON_WASAPI_ERROR` | Yes | Maybe (ring not committed) | Never | `audioClientOwned`, `serviceAcquired`, `started` | Same as above | Capture thread |
| `ReleaseBuffer` (cleanup in overflow/discontinuity/format-error) | Any failure | Original cause preserved; cleanup HRESULT logged | Yes | Maybe | Per original cause | `audioClientOwned`, `serviceAcquired`, `started` | Same as above | Capture thread |
| `Stop` | Any failure | HRESULT logged; does not override terminal reason (preserve-original is the sole rule — R12-5) | Yes | Maybe | Per terminal reason | `audioClientOwned`, `serviceAcquired`, `started` | `Stop` failure does not skip subsequent releases. Cleanup function continues with remaining flags → `CoUninitialize` → §Normative cleanup path steps 5–11 | Capture thread |

Notes:
- **`pMixFormat` freed eagerly via `mixFormatOwned` flag** (R11-4, R12-5): after `Initialize` succeeds, the capture thread copies `nSamplesPerSec`, `nChannels`, `wBitsPerSample`, `nBlockAlign`, and (if `WAVEFORMATEXTENSIBLE`) `wValidBitsPerSample` and `SubFormat` to session-local fields, then calls `CoTaskMemFree(pMixFormat)`, sets the pointer to null, and clears `mixFormatOwned`. All subsequent cleanup paths (`GetBufferSize`, `SetEventHandle`, `GetService`, `Start`, running-stream) have `mixFormatOwned=false`. On `GetMixFormat` failure, the docs state `*ppDeviceFormat` is null `[MS-48]`; `mixFormatOwned` is not set.
- **"PCM exists?"**: "Maybe" means capture was running and some frames may have been committed to the ring before the error. Promotion depends on the terminal reason and min-duration check.
- **`Stop` eligibility** (R11-4, R12-5): `Stop` is called **only** when the `started` flag is true (i.e., `IAudioClient::Start()` returned `S_OK`). This is a conservative policy. Microsoft documents that calling `Stop` on a successfully initialized but never-started stream returns `S_FALSE` (already stopped) `[MS-48]`, not `AUDCLNT_E_NOT_STOPPED` (which is not a documented `Stop` return code). The policy avoids relying on this `S_FALSE` behavior and instead skips `Stop` entirely when `Start` never succeeded. **`Stop` failure does not override the terminal reason** — this is the sole normative rule for `Stop` HRESULT handling (R12-5). The HRESULT is logged for evidence, but the terminal reason remains the original cause (user_stop, permission_revoke, etc.).
- **Format valid?**: format validity refers to whether `GetMixFormat` succeeded and the subtype was validated. Once set, the format remains valid in the result struct regardless of later failures.
- **Running-stream rows split per HRESULT** (R11-4): `GetNextPacketSize`, `GetBuffer`, and `ReleaseBuffer` each have explicit rows for `E_ACCESSDENIED` → `PERMISSION_REVOKE`, `AUDCLNT_E_DEVICE_INVALIDATED` → `DEVICE_LOST`, and "any other failure" → `WASAPI_ERROR`. No contradictory "any failure" global claim. Each HRESULT is looked up in the §HRESULT mapping table; the per-call rows here specify the complete classification without relying on a separate "any failure" catch-all that would contradict the specific rows.
- **All rows**: every failure path reaches `STOPPING` (via `CaptureRequestStop` or internal-failure CAS — R11-2), seals, and follows the §Normative cleanup path. No path publishes terminal before `CoUninitialize`.
- **Activation `E_ACCESSDENIED`**: this is the documented Win32 permission-denied HRESULT. It maps to `CAP_REASON_PERMISSION_REVOKE` at every call site — not only in the capture loop. The `Initialize` and `Start` calls can also return `E_ACCESSDENIED` if the microphone permission is revoked between activation and stream start.
- **Unknown failures are fail-closed**: any HRESULT not in the known mapping defaults to `CAP_REASON_WASAPI_ERROR` (non-promotable). The probe must discover and classify new HRESULTs as evidence.
- **Successful-start allocation proof** (R11-4): after a successful `Start`, the session holds exactly: `IAudioClient` (initialized), `IAudioCaptureClient` (from `GetService`), the capture-thread-event handle (from `SetEventHandle`), the recording ring (Go-allocated), and the scratch buffer (capture-thread-allocated). `pMixFormat` is already freed. No COM allocations are outstanding. Cleanup releases `IAudioCaptureClient` and `IAudioClient` on the capture thread; the event handle and ring are freed after `threadDone`.

#### Branch tests generated from the HRESULT/cleanup table (R10-6, R11-4)

1. **GetMixFormat failure**: inject `GetMixFormat` returning `E_FAIL`. Verify: `CoTaskMemFree` called on null pointer (no-op), `IAudioClient` released (`audioClientOwned=true`), no `Stop` (`started=false`), terminal reason `WASAPI_ERROR`, format invalid, no PCM, capture thread publishes terminal after `threadDone` barrier.
2. **Format validation rejection**: inject a valid `GetMixFormat` returning an unsupported subtype (e.g., A-law). Verify: `CoTaskMemFree` called, `IAudioClient` released, no `Stop`, terminal reason `FORMAT_ERROR` with `E_INVALIDARG`, capture thread publishes terminal.
3. **Initialize E_ACCESSDENIED**: inject `Initialize` returning `E_ACCESSDENIED`. Verify: `CoTaskMemFree` called, `IAudioClient` released (no services — `serviceAcquired=false`), no `Stop` (`started=false`), terminal reason `PERMISSION_REVOKE`, capture thread publishes terminal.
4. **Initialize other failure**: inject `Initialize` returning `AUDCLNT_E_UNSUPPORTED_FORMAT`. Verify: same cleanup, terminal reason `WASAPI_ERROR`.
5. **Initialize success frees pMixFormat** (R11-4): inject successful `Initialize`. Verify: `CoTaskMemFree(pMixFormat)` called immediately after `Initialize`, session-local fields populated (rate, channels, bits, block align), `pMixFormat` set to null.
6. **GetBufferSize failure** (post-Initialize): inject `GetBufferSize` returning `E_FAIL`. Verify: `IAudioClient` release (no services — `serviceAcquired=false`), no `CoTaskMemFree` (already freed at `Initialize` success), terminal reason `WASAPI_ERROR`.
7. **SetEventHandle failure**: inject `SetEventHandle` returning `E_INVALIDARG`. Verify: same as GetBufferSize failure path.
8. **GetService failure**: inject `GetService` returning `E_NOINTERFACE`. Verify: `IAudioClient` release (no `IAudioCaptureClient` — `serviceAcquired=false`), terminal reason `WASAPI_ERROR`.
9. **Start E_ACCESSDENIED**: inject `Start` returning `E_ACCESSDENIED`. Verify: no `Stop` (`started=false`), release `IAudioCaptureClient` (`serviceAcquired=true`) + `IAudioClient`, terminal reason `PERMISSION_REVOKE`.
10. **Start other failure**: inject `Start` returning `E_FAIL`. Verify: same cleanup, terminal reason `WASAPI_ERROR`.
11. **Stop failure after successful Start**: inject `Stop` returning `E_FAIL`. Verify: HRESULT logged, subsequent releases still proceed (`IAudioCaptureClient`, `IAudioClient`, `CoUninitialize`), terminal reason is the original stop reason (not the `Stop` failure).
12. **Activation E_ACCESSDENIED**: inject activation callback `GetActivateResult` returning `E_ACCESSDENIED`. Verify: terminal reason `PERMISSION_REVOKE`, callback clears async-op ref, capture thread publishes terminal.
13. **GetNextPacketSize E_ACCESSDENIED** (R11-4): inject running `GetNextPacketSize` returning `E_ACCESSDENIED`. Verify: internal-failure CAS installs `STOPPING`+`PERMISSION_REVOKE`, `Stop` called (`started=true`), releases, terminal reason `PERMISSION_REVOKE`, non-promotable.
14. **GetNextPacketSize DEVICE_INVALIDATED** (R11-4): inject `GetNextPacketSize` returning `AUDCLNT_E_DEVICE_INVALIDATED`. Verify: terminal reason `DEVICE_LOST`, promotable if ≥ min duration.
15. **GetBuffer E_ACCESSDENIED** (R11-4): inject `GetBuffer` returning `E_ACCESSDENIED`. Verify: no buffer acquired, `Stop` called, terminal reason `PERMISSION_REVOKE`, non-promotable.
16. **GetBuffer DEVICE_INVALIDATED** (R11-4): inject `GetBuffer` returning `AUDCLNT_E_DEVICE_INVALIDATED`. Verify: terminal reason `DEVICE_LOST`, promotable if ≥ min duration.
17. **ReleaseBuffer E_ACCESSDENIED** (R11-4): inject normal-commit `ReleaseBuffer` returning `E_ACCESSDENIED`. Verify: terminal reason `PERMISSION_REVOKE`, non-promotable.
18. **Successful-start allocation proof** (R11-4): run a successful capture start → verify no outstanding COM allocations (pMixFormat freed at Initialize success; IAudioClient and IAudioCaptureClient are live and will be released at cleanup). Verify: after cleanup, all COM objects released, CoUninitialize called.

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

#### Durable `.partial.reason` sidecar (R11-5)

*Addresses R11 finding 5. Makes orphan recovery reason-aware and fail-closed.*

**Problem** (R11-5): Rev 11's startup recovery inspected only header shape and duration, promoting any structurally valid orphan. Counterexample: permission is revoked after ≥ min duration, the stop reason is `PERMISSION_REVOKE`, and the process is killed before Go deletes the `.partial` file. On restart, the old recovery would promote the unauthorized `.partial` to `.wav` because the header is valid and the duration exceeds the minimum. The same logic would promote an overflow/discontinuity draft if the process died before deletion.

**Solution**: a durable per-session `.partial.reason` sidecar file is written atomically **by Go, after it observes the terminal state** via `CaptureGetResult` (R12-3, R13-5 — Go is the sole sidecar writer; the helper never touches the filesystem, so it cannot write the sidecar "before terminal"). Startup recovery reads the sidecar to determine whether promotion is safe. The unavoidable consequence is a fail-closed gap: if the process dies after terminal publication but before Go durably writes the sidecar, no sidecar exists and the orphan `.partial` is discarded on restart (see §Write ordering and the process-kill matrix). This is the accepted tradeoff — a missing sidecar always means discard, never promote.

**Sidecar format** (R12-3): the file `<session-id>.partial.reason` contains a single JSON object:

```json
{
  "version": 1,
  "sessionId": "<session-id>",
  "reason": 0,
  "reasonName": "user_stop",
  "hresult": 0,
  "timestampMs": 1752364800000
}
```

Fields: `version` (integer, currently 1 — unknown versions → fail-closed discard; R12-3), `sessionId` (string, matches the `.partial` file name without extension), `reason` (`CAP_REASON_*` integer value), `reasonName` (human-readable string for logging), `hresult` (the sealed HRESULT from `CaptureGetResult`), `timestampMs` (Unix milliseconds at sidecar write time from Go's `time.Now().UnixMilli()`). **No `promotable` field** (R12-3) — recovery derives promotability from the `reason` enum via the §Frozen draft outcome matrix. A redundant boolean that disagrees with the reason (e.g., `reason=PERMISSION_REVOKE` + `promotable=true`) would be a silent correctness bug.

**Canonical reason/name/HRESULT compatibility table** (R15-5). The JSON
`hresult` is a signed 32-bit integer representing the exact sealed bit pattern;
hex below is for readability. Recovery accepts a row only when `reasonName`
matches the canonical name and `hresult` satisfies that row. It never trusts
`reasonName` for policy — policy derives from the numeric reason — but rejects a
mismatch to keep evidence logs truthful.

Terminal publication uses this same table. External reason-only stop requests
(`AccessChanged`, shutdown, suspend, lock, user/cancel) synthesize the row's
exact HRESULT when sealing; internal COM/WASAPI paths preserve the original
HRESULT only when the row permits it. Thus an `AccessChanged` permission stop
seals `E_ACCESSDENIED` even though `CaptureRequestStop` carries only the reason.

| Value | Reason / canonical `reasonName` | Allowed sealed HRESULT | Validation rule |
|---|---|---|---|
| 0 | `CAP_REASON_USER_STOP` / `user_stop` | `S_OK` (`0x00000000`) | exact zero |
| 1 | `CAP_REASON_PERMISSION_REVOKE` / `permission_revoke` | `E_ACCESSDENIED` (`0x80070005`) | exact; any newly proven privacy HRESULT requires a table/version update |
| 2 | `CAP_REASON_DEVICE_LOST` / `device_lost` | `AUDCLNT_E_DEVICE_INVALIDATED` (`0x88890004`) | exact |
| 3 | `CAP_REASON_SHUTDOWN` / `shutdown` | `S_OK` | exact zero |
| 4 | `CAP_REASON_SUSPEND` / `suspend` | `S_OK` | exact zero |
| 5 | `CAP_REASON_LOCK` / `lock` | `S_OK` | exact zero |
| 6 | `CAP_REASON_CANCEL` / `cancel` | `HRESULT_FROM_WIN32(ERROR_CANCELLED)` (`0x800704C7`) | exact; never zero |
| 7 | `CAP_REASON_OVERFLOW` / `overflow` | `HRESULT_FROM_WIN32(ERROR_BUFFER_OVERFLOW)` (`0x8007006F`) | exact |
| 8 | `CAP_REASON_WASAPI_ERROR` / `wasapi_error` | original negative COM/WASAPI HRESULT | `hresult < 0`; exclude privacy/device/cancel/overflow/discontinuity sentinel codes, but allow context-dependent codes such as `E_INVALIDARG` from a non-format API stage |
| 9 | `CAP_REASON_FORMAT_ERROR` / `format_error` | `E_INVALIDARG` (`0x80070057`) | exact |
| 10 | `CAP_REASON_DISCONTINUITY` / `discontinuity` | `HRESULT_FROM_WIN32(ERROR_INVALID_DATA)` (`0x8007000D`) | exact app classification for a non-first discontinuity flag |

Cleanup-only failures such as `Stop`/cleanup `ReleaseBuffer` remain logs and do
not replace the sealed HRESULT/reason. Unknown reason, unknown version,
out-of-range JSON integer, name mismatch, or incompatible HRESULT is corrupt
and causes fail-closed discard.

**Duplicate-key rejection (R13-5 — `DisallowUnknownFields` does NOT do this).** The reviewer's root check (`.research/root-checks/windows-r13-json-duplicates/main.go`) proved that `json.Decoder` with `DisallowUnknownFields` accepts `{"reason":0,"reason":1}` and silently keeps the last value (`reason=1`, `err=nil`). A crafted sidecar could therefore flip a non-promotable reason to a promotable one (or vice versa) purely by key order. Recovery MUST detect duplicate keys explicitly at the token layer **before** typed decoding:

```go
// Reject any object key that appears more than once, at any depth.
func rejectDuplicateKeys(data []byte) error {
    dec := json.NewDecoder(bytes.NewReader(data))
    var walk func(seenStack []map[string]bool) error
    walk = func(stack []map[string]bool) error {
        for {
            tok, err := dec.Token()
            if err == io.EOF { return nil }
            if err != nil { return err }
            switch t := tok.(type) {
            case json.Delim:
                if t == '{' {
                    seen := map[string]bool{}
                    for dec.More() {
                        kt, err := dec.Token() // object key
                        if err != nil { return err }
                        key := kt.(string)
                        if seen[key] { return fmt.Errorf("duplicate key %q", key) }
                        seen[key] = true
                        if err := walk(nil); err != nil { return err } // value
                    }
                    if _, err := dec.Token(); err != nil { return err } // '}'
                    return nil
                }
                if t == '[' {
                    for dec.More() {
                        if err := walk(nil); err != nil { return err }
                    }
                    if _, err := dec.Token(); err != nil { return err } // ']'
                    return nil
                }
            default:
                return nil // scalar value consumed
            }
        }
    }
    if err := walk(nil); err != nil { return err }
    // Enforce exactly one trailing EOF (no trailing garbage / second object).
    if _, err := dec.Token(); err != io.EOF { return fmt.Errorf("trailing content") }
    return nil
}
```

The full validation order is: (1) size ≤ 4096 bytes; (2)
`rejectDuplicateKeys`; (3) typed decode with `DisallowUnknownFields`; (4)
require every field exactly once; (5) require one JSON object and whitespace
EOF; (6) `version==1`; (7) exact session-ID match; (8) known numeric reason;
(9) canonical `reasonName` match; (10) signed-int32 HRESULT and compatibility
per the table; (11) `timestampMs` is a positive signed-int64 value. Any failure
is fail-closed discard.

**Maximum sidecar size** (R12-3): 4096 bytes. Files larger than this are treated as corrupt → discard.

**Corruption check** (R12-3): recovery validates `version==1`, `sessionId` matches, `reason` is known, sidecar size ≤ 4096, JSON parses without error and with no unknown fields. Any validation failure → discard (fail-closed).

**Required parser/table tests** (R15-5): accept one valid object for each of the
eleven compatibility rows. Reject each required field omitted in turn, every
unknown field, version mismatch, session mismatch, reason/name mismatch,
out-of-int32 HRESULT, and every incompatible reason/HRESULT pairing. Shared
codes (`S_OK` for several lifecycle rows, context-dependent `E_INVALIDARG`) are
distinguished by the numeric reason plus canonical name and are not falsely
required to reject one another.
Retain duplicate tests at top level in both orders, nested object, object in an
array, concatenated objects, trailing non-whitespace, and valid whitespace EOF.
`DisallowUnknownFields` is tested only for unknown fields; token walking owns
duplicate detection.

**Write ordering** (R11-5, R12-3 — Go is the sole writer):

1. The capture thread seals the reason via packed CAS (step 3 of §Normative cleanup path), publishes terminal, signals `localNotify`.
2. Go (waiter goroutine) wakes, queries `CaptureGetResult` — reads sealed reason, HRESULT, and terminal state.
3. **Go writes the sidecar atomically before any draft action**:
   a. Go derives promotability from the `reason` enum (§Frozen draft outcome matrix) — no redundant boolean (R12-3).
   b. Create a temporary file `<session-id>.partial.reason.tmp` in the draft directory.
   c. Write the JSON content (marshaled with `json.Marshal`; `version=1`, `sessionId`, `reason`, `reasonName`, `hresult`, `timestampMs`).
   d. Call `FlushFileBuffers` on the temp file (via `f.Sync()`).
   e. Close the temp file.
   f. Rename `<session-id>.partial.reason.tmp` → `<session-id>.partial.reason` (atomic on NTFS).
4. If the sidecar write fails (disk full, permissions): log the failure. If the reason is promotable, the `.partial` cannot be safely recovered on restart (missing sidecar → fail-closed discard). Go still attempts normal finalization in this session (drain, rewrite headers, promote). If the process dies before finalization, the `.partial` is safely discarded on restart.
5. **After sidecar is durable, Go proceeds with draft action** (promote or delete per the outcome matrix).
6. After draft action completes (`.partial` → `.wav` promotion or `.partial` deletion), Go deletes the `.partial.reason` sidecar.

**Go is the sole sidecar writer** (R12-3). The helper DLL never touches the filesystem — it provides only `CaptureRead` for PCM and `CaptureGetResult` for terminal state. The sidecar is written by Go after terminal publication, not before — this means a process death between terminal publication and sidecar write results in a missing sidecar, which triggers fail-closed discard on restart. This is the accepted tradeoff: the window between terminal and sidecar write is typically microseconds (Go wakes, queries, marshals JSON, writes), and fail-closed discard is the safe default.

**Mismatched-field counterexample** (R12-3): Rev 12's redundant `promotable` boolean allowed a sidecar with `reason=PERMISSION_REVOKE` + `promotable=true` to reach the promotion path. In Rev 13, recovery derives promotability solely from the `reason` field via the frozen outcome matrix. No boolean can override the matrix. The counterexample test is:
1. Manually craft a sidecar with `reason=1` (PERMISSION_REVOKE), valid `sessionId`, valid `version`, valid JSON.
2. On restart, recovery reads sidecar, derives promotability from `reason=1` → non-promotable per the matrix.
3. Result: `.partial` is discarded, not promoted. ✓

#### Startup cleanup/recovery (R11-5)

On app startup, before any new capture:

1. Scan `<draft-dir>` for `.partial` files.
2. For each `.partial` file:
   a. **Read the sidecar** (R11-5): look for `<session-id>.partial.reason` in the same directory.
      - **Sidecar present**: parse the JSON. Validate `sessionId` matches the `.partial` file name. If `sessionId` does not match: treat as missing (stale sidecar from a different session — fail-closed).
      - **Sidecar missing or corrupt** (R11-5): the process was killed before the sidecar was written, or before the reason was sealed, or the sidecar was corrupted. **Discard the `.partial` (fail-closed)** — unknown/ambiguous crash state never auto-promotes. Delete the `.partial` and log the outcome.
   b. **Check promotability from sidecar reason** (R12-3): derive promotability from `sidecar.reason` via the §Frozen draft outcome matrix — do NOT trust a redundant boolean. If the reason maps to non-promotable (permission revoke, overflow, discontinuity, WASAPI error, format error, cancel) → **delete the `.partial` and the sidecar.** Log the reason. If the reason is unknown (not a valid `CAP_REASON_*` value) → **discard** (fail-closed).
   c. **Check current permission** (R11-5, R12-3): if the reason is promotable per the matrix → call `CapPermissionCheck` (requires `CapInit` — done before recovery). If permission is not exactly `CAP_PERMISSION_ALLOWED` → **discard** (permission was revoked between the crash and restart; the recording is no longer authorized). Delete both files.
   d. **Structural validation**: read the 44-byte WAV header. Validate: RIFF magic, WAVE magic, `fmt ` chunk at offset 12 with format tag IEEE float (0x0003), `bitsPerSample == 32`, `channels >= 1`, `sampleRate > 0`, `nBlockAlign == channels * 4`, `data` chunk at offset 36. If invalid: delete both files.
   e. **Truncate incomplete frames**: compute actual PCM byte count from `fileSize - 44`. If `(fileSize - 44) % bytesPerFrame != 0`, truncate to `44 + ((fileSize - 44) / bytesPerFrame) * bytesPerFrame` and `FlushFileBuffers`.
   f. **Duration check**: compute `(truncatedSize - 44) / bytesPerFrame / sampleRate`. If < minimum duration: delete both files (too short).
   g. **Promote**: rewrite RIFF and `data` chunk sizes, `FlushFileBuffers`, rename `.partial` → `.wav`. Delete the sidecar.
   h. **Verify the recovered WAV**: parse with `parseWAV`. If parsing fails, delete the `.wav` and log the failure. For the probe, additionally verify with an independent decoder/tool.
   i. If too short, corrupt, or empty: delete the `.partial` and sidecar.
3. **Clean orphan sidecars**: scan for `.partial.reason` files with no matching `.partial`. Delete them (stale — the `.partial` was already deleted or promoted in a previous run).
4. Log each recovery or deletion with the file name, sidecar reason, recovered duration, format, and outcome.

#### Process-kill recovery tests (R11-5)

For each terminal reason (`user_stop`, `permission_revoke`, `device_lost`, `shutdown`, `suspend`, `lock`, `cancel`, `overflow`, `discontinuity`, `wasapi_error`, `format_error`), test process kill at every edge:

| Kill point | Expected recovery outcome |
|---|---|
| Before reason seal (Go has not seen terminal) | `.partial` has no sidecar → **discard** (fail-closed) |
| After reason seal, before terminal publication | `.partial` has no sidecar (Go hasn't been notified) → **discard** |
| After terminal publication, before Go reads `CaptureGetResult` | `.partial` has no sidecar → **discard** |
| After Go reads terminal, before Go writes sidecar (R12-3) | `.partial` has no sidecar → **discard** (fail-closed — this is the accepted tradeoff; window is microseconds) |
| After Go writes sidecar, before Go deletes `.partial` (non-promotable reason) | Sidecar present with non-promotable reason → **discard** on recovery |
| After Go writes sidecar, before Go rewrites headers (promotable reason) | Sidecar present with promotable reason. Headers still zero-size. → **promote** (recovery rewrites headers) |
| After header rewrite, before flush | Sidecar present, promotable. Rewritten headers may be partially flushed. Recovery re-reads file size, re-truncates, re-rewrites headers. **Promote** if valid. |
| After flush, before rename `.partial` → `.wav` | Sidecar present, promotable. `.partial` has correct headers. Recovery promotes. |
| After rename, before sidecar deletion | `.wav` exists (already promoted). Orphan sidecar cleaned in step 3. |

**Permission-revoke counterexample proof** (R11-5, R13-5): permission revoked after ≥ min duration, process killed after Go durably wrote the sidecar but before Go deletes `.partial`:
1. Sidecar written by Go with `reason=CAP_REASON_PERMISSION_REVOKE` (no `promotable` field — R12-3).
2. On restart, recovery derives promotability from `reason` via the §Frozen draft outcome matrix → non-promotable → deletes `.partial` and sidecar.
3. Result: no unauthorized `.wav` created. ✓ (If the process died *before* Go wrote the sidecar, there is no sidecar → discard anyway.)

**Overflow counterexample proof** (R11-5, R13-5): overflow after ≥ min duration, process killed after Go durably wrote the sidecar but before Go deletes `.partial`:
1. Sidecar written by Go with `reason=CAP_REASON_OVERFLOW` (no `promotable` field).
2. On restart, recovery derives non-promotable from `reason` via the matrix → deletes both files.
3. Result: no integrity-failed `.wav` created. ✓

**Missing sidecar proof** (R11-5): process killed before sidecar is written (or during sidecar write):
1. No sidecar found for `.partial`.
2. Recovery discards `.partial` (fail-closed).
3. Result: unknown crash state never auto-promotes. ✓

#### Atomic promotion

A draft is promoted from `.partial` to `.wav` only on **confirmed finalization**:

- **User stop**: Go reads all remaining PCM via `CaptureRead` (loop until `S_FALSE` + terminal state), writes final frames to `.partial`, rewrites header sizes, flushes, renames to `.wav`. After promotion, Go deletes the `.partial.reason` sidecar (R11-5). The finalization is complete only when the `.wav` file exists with correct headers and passes the parse verification.
- **Permission revoke / cancel / overflow / discontinuity / WASAPI error / format error / too-short capture**: delete the `.partial` file **and** its `.partial.reason` sidecar (R11-5). This is an **evidenced deliberate discard** — the probe records the reason, duration captured, and deletion. It is not a pass (no valid media produced) and not a failure (the system correctly discarded unauthorized or insufficient media).
- **Shutdown / suspend / lock**: `CaptureRequestStop` is called from the wndproc (non-blocking). The capture thread seals the reason, cleans up, and publishes terminal per the §Normative cleanup path — it does **not** write any file (R13-5: the helper never touches the filesystem; Go is the sole sidecar writer). If Go's waiter is still alive when terminal publishes, Go reads `CaptureGetResult`, writes the `.partial.reason` sidecar atomically (§Write ordering), then drains and finalizes: normal promotion (sidecar deleted after). If the OS kills the process before Go durably writes the sidecar, the orphan `.partial` has **no** sidecar and is discarded on restart (fail-closed). A shutdown/suspend/lock draft is therefore recoverable **only** if Go managed to write a promotable-reason sidecar before the process died; otherwise it is discarded. This is the accepted fail-closed tradeoff — the shutdown path cannot guarantee sidecar durability because only Go (not the helper) may write it and the OS may kill Go at any point.

#### Invariants

1. A `.wav` file in the draft directory is always a valid, finalized artifact with correct RIFF/data sizes, verified by the local `parseWAV` parser (and an independent decoder/tool for probe evidence).
2. A `.partial` file has a 44-byte IEEE-float WAV header with zero sizes, valid `fmt ` metadata at the native capture rate/channels, and raw float32 PCM data after byte 44. It is **only** recoverable if a matching `.partial.reason` sidecar exists with a promotable reason and current permission is `Allowed` (R11-5). Otherwise it is deleted on startup.
3. No network operation or blocking wait occurs in the window procedure path.
4. A **valid user media pass** requires a finalized `.wav` on disk or a proven-recoverable `.partial` (with sidecar). A queued `CaptureRequestStop` alone is not success.
5. A **deliberate discard** (permission revoke, explicit cancel, too-short capture, overflow, discontinuity, WASAPI error, format error) is an evidenced outcome, not a pass — the probe records the reason and verifies deletion of both `.partial` and `.partial.reason`.
6. Interrupted shutdown/suspend/lock never reports a pass — the draft either survives as `.partial` (with sidecar) for recovery, or is cleanly discarded if too short, non-promotable, or missing sidecar.
7. **No permission/integrity failure ever becomes a `.wav` after restart** (R11-5). Unknown or ambiguous crash state (missing/corrupt/stale sidecar) never auto-promotes.

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

1. **Default input**: `MediaDevice.GetDefaultAudioCaptureId(Default)` → device ID → `CapturePrepare(notifyEvent, &opId)` → waiter waits for MTA-ready → `CaptureActivate(opId, deviceId)` → activation → `Initialize(SHARED, EVENT_CALLBACK)` → `GetService` → `Start` → capture loop.
2. **Selected input**: `DeviceInformation.FindAllAsync(AudioCapture)` → user picks from list → `DeviceInformation.Id` → same `CapturePrepare`/`CaptureActivate` → same pipeline.

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
- If allowed (or if AppCapability unavailable), Go calls `CapturePrepare(notifyEvent, &opId)` → waiter waits for MTA-ready → UI calls `CaptureActivate(opId, deviceId)` → waits for event → `CaptureGetResult(opId, ...)` to confirm activation and get format (R12-4).
- Evidence:
  - AppCapability.Create success/failure HRESULT
  - access status before prompt
  - access status after prompt (or activation HRESULT if using fallback)
  - activation HRESULT
  - actual capture format from `CaptureFormat` struct (native subtype, native bits, converted float32)

### 2. Record default input

- Go calls `CapGetDefaultDevice(Default, notifyEvent, &opId)` → waits → `CapGetDefaultDeviceResult(opId, ...)`.
- Go calls `CapturePrepare(notifyEvent, &opId)` → waiter waits for MTA-ready → UI calls `CaptureActivate(opId, deviceId)` → waits → `CaptureGetResult(opId, ...)`.
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
- Go calls `CapturePrepare(notifyEvent, &opId)` → waiter waits for MTA-ready → UI calls `CaptureActivate(opId, selectedDeviceId)`.
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
10. Prove crash-safe draft recovery with reason-aware sidecar (R11-5, R12-3): simulate process kill during capture (e.g., `taskkill /F`), verify `.partial` file and `.partial.reason` sidecar survive, verify startup recovery reads the sidecar, derives promotability from reason enum (no `promotable` boolean — R12-3), checks current permission, rewrites headers from actual file length and produces a valid `.wav` for promotable reasons, or correctly discards for non-promotable reasons/missing sidecar. Run the full process-kill matrix from §Process-kill recovery tests (R11-5, R12-3): kill at every edge (before/after Go sidecar write, before/after terminal, before/after drain/delete/header-rewrite/flush/rename) for every terminal reason. Prove that no permission-revoke, overflow, discontinuity, WASAPI-error, or format-error draft ever becomes a `.wav` after restart. Prove that a missing/corrupt sidecar causes discard (fail-closed).
11. Prove callback strong-reference safety: run every adversarial test from §Callback strong-reference lifetime, including immediate release, Diagram-A/Diagram-B handle barriers, unsubscribe fencing, duplicate/launch failures, MTA failure/timeout, cycle breaking, and threadDone-before-terminal order. No crash, use-after-free, leaked reference, duplicate, or thread launch handle.
12. Prove stop-reason priority arbitration: run the barrier tests from §Stop-reason priority arbitration (user-stop vs permission revoke both orderings, user-stop vs overflow, device-loss vs revoke, Go promotion guard with stale reason). Verify that higher-priority reasons always win and that Go rejects promotion when permission is denied regardless of terminal reason.
13. Prove WAV interoperability: run the independent decoder gate from §Native-format WAV validity (mono, stereo, 4-channel, 8-channel synthetic IEEE-float WAVs) against an independent decoder/tool on Windows. Record channel/rate/frame metadata. If the decoder requires a `fact` chunk or `WAVEFORMATEXTENSIBLE`, update the writer and record the decision.
14. Prove PCM conversion safety under sanitizers: run the deliberately unaligned conversion test vectors under AddressSanitizer and UBSan. Verify no undefined behavior reports. Verify bit-exact results for power-of-two vectors and tolerance-bounded results for others.
15. Prove composite completion barrier (R9-1, R9-2): run the composite barrier cancel test (test 15 in §Callback strong-reference lifetime adversarial tests) — verify that the callback publishes terminal only when `threadDone==1`, and that the capture thread publishes terminal when the callback fires before the thread exits. Run the `threadDone`-before-terminal barrier test (test 14) — verify correct ordering with seq-cst fence. No race, no use-after-free, no lost terminal.
16. Prove single-owner waiter integration (R9-5, R10-5): run the command-queue ordering test, the `WM_ENDSESSION`-does-not-call-`CapDestroy` test, the `WM_QUERYENDSESSION`-calls-`CaptureRequestStop` test, the graceful-quit async state machine test (waiter requests stop → waits for terminal → drains → releases → posts `WM_APP+CLEANUP_READY` → UI calls `CapDestroy` → UI posts `WM_QUIT`), and the `OnQuit`-does-not-post-immediate-`WM_QUIT` test from §UI-thread event/message integration. Verify no `CapDestroy` from waiter, no release before terminal, no `WM_QUIT` before `CapDestroy`.
17. Prove `CapInit` rollback (R9-6): inject an allocation failure after `RoInitialize` succeeds. Verify `CapInit` calls `RoUninitialize` before returning, no thread ID is stored, and `CapDestroy` returns `S_OK` (no-op).
18. Prove packed CAS reason seal (R10-4): run the deterministic counterexample — request thread reads STOPPING and pauses, capture thread seals, request thread resumes and attempts CAS. Verify: the CAS observes `SEALED` state and returns no-op. Verify: seal CAS retries if a higher-priority reason is installed between load and CAS. Verify: `CaptureGetResult` never exposes `SEALED` state — maps to `ACTIVATING` or last public state.
19. Prove complete HRESULT/cleanup table (R10-6): run all 18 branch tests from §Branch tests generated from the HRESULT/cleanup table — inject every listed COM/WASAPI failure class and verify the exact terminal reason/HRESULT, release/free order (including `CoTaskMemFree`), terminal publication after the `threadDone` barrier, and `Stop` only when `started=true`.
20. Prove cleanup `ReleaseBuffer` failure classification (R10-3): inject `ReleaseBuffer` failure in overflow, discontinuity, format-error, and stop-while-acquired branches. Verify: terminal reason is the original cause (not the cleanup `ReleaseBuffer` HRESULT), zero frames visible to consumer, `Stop` still attempted, subsequent releases proceed.
21. Prove internal-failure CAS (R11-2): inject overflow during `CAPTURING` state with a concurrent `CaptureRequestStop(user_stop)`. Verify: internal-failure CAS installs `STOPPING` + `OVERFLOW` (higher priority than `USER_STOP`). Verify `lastPublicState==CAPTURING` stored in packed word. Run all 18 scenarios from §Deterministic transition tests (R11-2), including the suffixed race cases.
22. Prove per-operation graceful quit (R15-1): run all 13 tests from
§Required quit tests. In particular, inject actual success/failure/cancel after
cooperative requests, late terminals at 6/15/29 seconds, and never-terminal.
Assert the sole waiter keeps ownership and no `CLEANUP_READY` exists while any
registry/subscription/callback/thread condition remains live.
23. Prove `CapIsQuiescent` and watchdog linearization: keep a callback ref live,
verify `S_FALSE`, release it and verify `S_OK`; then race graceful destroy at 29
seconds against the watchdog. Exactly one exit-state CAS wins; graceful success
defeats `os.Exit`, force success prevents `WM_QUIT`.
24. Prove HRESULT/cleanup ownership flags (R11-4): run the 18 branch tests from §Branch tests generated from the HRESULT/cleanup table — including the 7 new running-stream HRESULT splits (GetNextPacketSize E_ACCESSDENIED/DEVICE_INVALIDATED, GetBuffer E_ACCESSDENIED/DEVICE_INVALIDATED, ReleaseBuffer E_ACCESSDENIED) and the Initialize-success pMixFormat free. Verify cleanup uses ownership flags: no `Stop` without `started`, no `IAudioCaptureClient::Release` without `serviceAcquired`, no `IAudioClient::Release` without `audioClientOwned`. Verify successful-start allocation proof: zero outstanding COM allocations after `Initialize` success + `CoTaskMemFree`.
25. Prove normative cleanup path consistency (R11-1): run the static grep fixture — verify no text in the note calls `threadDone` "the final session-state access"; no standalone terminal-publication sequence omits seal, `threadDone`, fence, terminal store, or `localNotify`; no cleanup path calls `Stop` without checking `started`.
26. Prove two-step `CapturePrepare`/`CaptureActivate` (R15-3/R15-4): inject
capture-duplicate failure (no ID/thread); callback-duplicate failure (state
remains PREPARED); callback duplicate followed by lost CAS (close once, no
launch); synchronous launch failure (callback duplicate closes once, thread
publishes); normal/failure/both-cancel callback paths with exact duplicate
counts. Delay `CoInitializeEx` ten seconds and verify the separate five-second
MTA-readiness safety stop produces `ERROR_CANCELLED` without blocking UI.
27. Prove `.partial.reason` sidecar write ordering and compatibility (R15-5):
retain all process-kill edges, then accept one valid object for every reason/
HRESULT row and reject all incompatible row pairings, zero cancel,
name mismatch, missing/unknown fields, duplicates at every required nesting,
concatenated objects, trailing content, and invalid whitespace/EOF cases.

---

## Final answer to the task

Select a narrow native WinRT helper (`pulsar-capture.dll`) plus AppContainer-safe WASAPI activation:

- Permission: `AppCapability` (with SUA-only caveat, complete `AppCapabilityAccessStatus` enum mapping, and `ActivateAudioInterfaceAsync` consent fallback — fallback acceptable only with proven WASAPI revoke detection)
- Enumeration: `MediaDevice` + `DeviceInformation`
- Capture: `ActivateAudioInterfaceAsync` → `IAudioClient` / `IAudioCaptureClient` on a helper-owned MTA capture thread; `GetMixFormat` runs before `Initialize` (the format it returns is passed to shared-mode `Initialize`)
- Format: `GetMixFormat()` from the device; helper converts to interleaved float32 with exact conversion for `WAVEFORMATEX` and `WAVEFORMATEXTENSIBLE` including packed 24-bit, 24-in-32; all sample reads via `memcpy` or byte assembly (no pointer casts — R6-7); signed 24-bit uses safe signed arithmetic on representable values: `int32_t val = (int32_t)u; if (u >= 0x800000u) val -= 0x1000000;` — no implementation-defined unsigned-to-signed casts (R7-1); `wValidBitsPerSample`, `nBlockAlign` validation; conversion vectors are bit-exact where float32 is exactly representable, tolerance-based otherwise; int32 `INT32_MAX` correctly rounds to `1.0f` in float32; deliberately unaligned test vectors run under ASan/UBSan
- Picker: `FileOpenPicker` + `IInitializeWithWindow` with a **visible** Pulsar window; returns a kernel read handle via `IStorageItemHandleAccess::Create` (probe hypothesis under AppContainer — blocked if it fails), not a path; take-once handle transfer; `*hresult` is always the operation outcome (never overwritten with transfer-state codes — R6-3); `*handleTaken` alone reports transfer; complete truth table covers all states (R6-3); max enforced against actual bytes read; picker name buffer truncates and returns `S_OK` with `requiredNameChars` (not `E_NOT_SUFFICIENT_BUFFER`); invalid `takeHandle` values return `E_INVALIDARG`; `PENDING` state returns `S_FALSE`
- Hotkey/lifecycle owner: hidden top-level Win32 window on the existing pinned UI thread (probe hypothesis for appContainer — blocked/no-go if it fails, not silently degraded)
- UI-thread event integration: one pinned waiter owns every query/read/take/
  release/promotion operation while the UI thread continues its existing
  `GetMessageW` pump and calls only UI-apartment exports. Ordinary quit issues
  cooperative stop/cancel requests but the waiter keeps all pending operations
  and events through actual terminal results at any time. Five seconds only
  exposes Force Quit. `CLEANUP_READY` requires empty registry, unwound
  subscription, completed capture thread, and `CapIsQuiescent()==S_OK`.
  `CapDestroy` retries via `WM_TIMER`, never `Sleep`; successful destroy must win
  `exitGracefulPending→exitGracefulComplete` before `WM_QUIT`, atomically
  defeating the watchdog. A never-terminal operation emits no cleanup message;
  force exit is the only 30-second bound. `WM_ENDSESSION` remains abrupt:
  nonblocking stop, best-effort drain, no `CapDestroy`.
- Helper ABI: fully asynchronous (initiate → event → query), auto-reset notification events are **readiness hints only** (coalescing is expected; Go drains all ready state per wake, including `CapPermissionCheck` for `AccessChanged`), versioned structs with `structSize` and `valid` flags, operation IDs with wrap/exhaustion handling, `__stdcall`, fixed-width types, HRESULT returns (Go truncates `uintptr` → `int32` before sign test), C++ exception safety at every export, `/MT` static CRT, no runtime redistributables
- Helper loading: `LoadPackagedLibrary` via `kernel32.NewProc("LoadPackagedLibrary")` (production — `windows.LoadPackagedLibrary` does not exist in `x/sys v0.46.0`; R7-5), absolute-path `windows.LoadLibraryEx` (unpackaged dev only, on `APPMODEL_ERROR_NO_PACKAGE`); kernel32 handle obtained via `windows.NewLazySystemDLL("kernel32.dll")` (not `NewLazyDLL` — R8-7); injectable function wrapper seam for unit-test loader selection (R8-7)
- UI WinRT apartment: `CapInit` calls `RoInitialize(RO_INIT_SINGLETHREADED)` on the UI thread; accepts `S_OK`/`S_FALSE`, rejects `RPC_E_CHANGED_MODE`; balanced by `RoUninitialize` in `CapDestroy`; if `RoInitialize` succeeds but a later `CapInit` step fails, `CapInit` calls `RoUninitialize` before returning the failure HRESULT — no partial init state left (R9-6); repeated `CapInit`/`CapDestroy` cycles work correctly (R7-5); `CapInit` stores the initializing thread ID; a second `CapInit` before `CapDestroy` returns `E_NOT_VALID_STATE`; `CapDestroy` from a wrong thread returns `RPC_E_WRONG_THREAD` without teardown (R8-7); required tests: `S_OK`/`S_FALSE` init+destroy, `RPC_E_CHANGED_MODE` leaves no state, repeated init → `E_NOT_VALID_STATE`, wrong-thread destroy → `RPC_E_WRONG_THREAD`, double destroy → `S_OK`, re-init after destroy → `S_OK`, partial `CapInit` failure after `RoInitialize` → `RoUninitialize` called, no state left (R8-7, R9-6)
- COM/handle ownership: `CapturePrepare` allocates/reserves state, eagerly
  duplicates the capture notification handle, then creates/publishes the MTA
  thread; duplicate/thread failure publishes nothing. Successful MTA init
  release-stores session-owned monotonic `mtaReady=1`; `CaptureGetResult`
  acquire-loads and copies it to caller memory. `CaptureActivate` eagerly
  creates `callbackNotify` while PREPARED, then CASes to ACTIVATING; lost CAS
  closes it and launches nothing. Normal/failure/Diagram-B callbacks close
  without signaling; only Diagram A signals+closes callback ownership. The
  capture thread signals+closes its pre-created duplicate on thread-published
  terminal and closes without signal in Diagram A. No worker signals Go's
  original handle. MTA handoff, `threadDone`→fence→terminal ordering, and
  helper-owned WASAPI releases remain as specified in the exhaustive branch
  table. The CRT-using thread is launched with `_beginthreadex`; a creator hold
  covers execution before publication, and its short-lived kernel handle is
  closed exactly once by the UI helper after no-fail registry publication.
- HRESULT handling: dedicated `HResult` Go type with explicit `uintptr` → `int32` truncation via `HResultFromUintptr`, no `syscall.Errno` conflation, hex logging; the 64-bit packed word contains state/reason but no HRESULT — the terminal publisher writes separate `sealedReason`/`sealedHresult` result fields before the release-store of `TERMINAL`, and `CaptureGetResult` copies them only after its acquire-load observes terminal; overflow uses standard `HRESULT_FROM_WIN32(ERROR_BUFFER_OVERFLOW)` = `0x8007006F` (not a private `FACILITY_ITF` code — R5-3); WASAPI HRESULT table uses only SDK-verified constants — `AUDCLNT_E_NOT_ALLOWED` removed (R7-2: `0x88890003` is `AUDCLNT_E_WRONG_ENDPOINT_TYPE`); `AUDCLNT_E_SERVICE_NOT_RUNNING` and `AUDCLNT_E_RESOURCES_INVALIDATED` reclassified from `CAP_REASON_DEVICE_LOST` to `CAP_REASON_WASAPI_ERROR` (non-promotable — R8-4); complete HRESULT/cleanup table with explicit `audioClientOwned`/`mixFormatOwned`/`serviceAcquired`/`started` ownership flags (R11-4, R12-5) covers every COM/WASAPI call from activation through `Stop` — running-stream rows (`GetNextPacketSize`, `GetBuffer`, `ReleaseBuffer`) split `E_ACCESSDENIED`→`PERMISSION_REVOKE`, `AUDCLNT_E_DEVICE_INVALIDATED`→`DEVICE_LOST`, and "any other failure"→`WASAPI_ERROR` individually (R11-4: no contradictory global "any failure" claim); `pMixFormat` freed immediately after `Initialize` succeeds (R11-4: fields copied to session state; running-stream rows have no allocation to leak); `Stop` called only after successful `Start` (R11-4: policy — Microsoft documents `S_FALSE` for already-stopped `[MS-48]`, not the non-existent `AUDCLNT_E_NOT_STOPPED`); stop-reason priority uses packed atomic CAS on state+sealed-bit+reason+lastPublicState (R10-4, R11-2) — no mutex needed, no post-snapshot CAS race; cancellation is `state=STOPPING`+`reason=CANCEL` (R11-2: no separate `cancelled` bit); internal capture failures use a two-step CAS: `CAPTURING`→`STOPPING` + failure reason (priority merge), then seal (R11-2); `CaptureGetResult` maps `SEALED` → stored `lastPublicState` (R11-2); wake events per source state: `stopEvent` for `CAPTURING`, `captureThreadWakeEvent` for `ACTIVATING` (R11-2); unknown audio errors map to `CAP_REASON_WASAPI_ERROR` not `CAP_REASON_PERMISSION_REVOKE` (R7-2); actual privacy-revoke HRESULT is mandatory probe discovery
- Stop wake routing: `PREPARING`, `PREPARED`, and `ACTIVATING` use
  `captureThreadWakeEvent`; `CAPTURING` uses `stopEvent`; a priority-only update
  in `STOPPING` needs no second signal. The capture thread alone changes
  `ACTIVATING→CAPTURING`, after successful `Start`; callback handoff never does.
- Permission ABI: named `CAP_PERMISSION_*` enum with explicit exhaustive switch from raw `AppCapabilityAccessStatus` values (R8-3); raw `NotDeclaredByApp`(1) → `CAP_PERMISSION_NOT_DECLARED`(4), raw `Allowed`(4) → `CAP_PERMISSION_ALLOWED`(1) — a direct cast NEVER reaches Go; unknown/future values → `CAP_PERMISSION_UNKNOWN`(5) (fail-closed); `CAP_PERMISSION_UNAVAILABLE`(-1) is a no-go for promotion unless the separately gated `activation-consent + proven-revoke-monitor` mode is established; `AccessChanged` subscription state holds a **strong** `AppCapability` reference (not raw pointer — R8-3)
- Callback lifetime: every async callback (activation, picker, permission, enumeration, `AccessChanged`) holds a **strong operation reference** until its final return; release exports drop only the registry reference; ref-count reaches zero only when all holders release; helper DLL is loaded once and **never unloaded** (`FreeLibrary` never called — R6-1); `CapDestroy` tears down application state only; module reclaimed at process exit; COM object ownership and release graph frozen (R6-1); operation destructor never joins threads (R8-2); deterministic barrier tests (not only stress loops — R6-1); adversarial race tests required (R5-1)
- Session lifetime: private packed states are uniquely
  `PREPARING=0, PREPARED=1, ACTIVATING=2, CAPTURING=3, STOPPING=4,
  SEALED=5, TERMINAL=6`; public states remain preparing=0, activating=1,
  capturing=2, stopped=3, failed=4, cancelled=5. Stop from the first three
  states wakes `captureThreadWakeEvent`; capture wakes `stopEvent`; unknown IDs
  are idempotent `S_OK` stop no-ops while query/read uses `E_HANDLE`.
  Priority is overflow(1) > discontinuity(2) > permission_revoke(3) >
  wasapi_error/format_error(4, first-installed tie) > device_lost(5) >
  shutdown(6) > suspend(7) > lock(8) > cancel(9) > user_stop(10).
  `CapDestroy` still requires empty registry, unwound subscription,
  `threadDone==1`, and zero callback refs; abrupt `WM_ENDSESSION` never calls it.
- Sample representation: helper converts native format to interleaved float32; all reads via `memcpy`/byte assembly (R6-7); valid bits are **left-aligned** in PCM containers (R4-1); `CaptureFormat` struct with `valid` flag, native metadata including `nativeValidBits` and `nBlockAlign`; conversion by `2^(validBits-1)` divisor; signed 24-bit uses safe signed arithmetic: `int32_t val = (int32_t)u; if (u >= 0x800000u) val -= 0x1000000;` (R7-1); deliberately unaligned vectors under ASan/UBSan; 44-byte IEEE-float WAV is the **selected initial build-time contract** — interoperability confirmed or rejected by independent decoder gate before signed hardware scenarios (R7-7); if gate requires `fact`/extensible, all components switch together
- WASAPI packet drain: capture thread loops `GetNextPacketSize`/`GetBuffer`/convert/`ReleaseBuffer`/commit-ring until packet size is zero on every event wake (auto-reset is a readiness hint, not one-shot); ordering is convert→`ReleaseBuffer`→commit-ring: data copied to scratch buffer while WASAPI buffer is acquired, `ReleaseBuffer` called to return the buffer, ring producer index published only after `ReleaseBuffer` succeeds — if `ReleaseBuffer` fails, zero frames become visible to the consumer (R9-3); **whole-packet ring preflight before conversion/copy** — if ring lacks room for entire packet, zero frames written, `ReleaseBuffer` called, terminal overflow (R7-3); first packet `DATA_DISCONTINUITY` accepted, subsequent → terminal `CAP_REASON_DISCONTINUITY` (R7-3); `TIMESTAMP_ERROR` logged but accepted (R7-3); acquired packet always released before stop/error; Go (waiter goroutine — R9-5) drains `CaptureRead` until `S_FALSE`, then queries all operations AND `CapPermissionCheck` per wake (R4-3, R5-4)
- Recording ring overflow: terminal `FAILED` state with `HRESULT_FROM_WIN32(ERROR_BUFFER_OVERFLOW)` = `0x8007006F` plus `CAP_REASON_OVERFLOW` terminal reason; WASAPI packet released via `ReleaseBuffer` before COM teardown; separate lossy meter ring for UI only (R5-3)
- Checked allocation bounds: channel count ≤ 8 (field is `uint16`, max 65535, but >8 rejected); sample rate ≤ 384 kHz; ring capacity = `max(2 × sampleRate, bufferFrames)` frames (R8-6 — guaranteed to hold at least one full endpoint buffer; bounds to ≈23 MiB at 384 kHz 8ch — R9-6); all arithmetic in wide types with overflow check before allocation; Go uses `int64` for frame/byte counters (R4-6); scratch-buffer conversion: capture thread converts into a pre-allocated scratch buffer (`maxFrames × channels × sizeof(float32)`) and commits to ring only after ReleaseBuffer succeeds AND the entire packet converts successfully — partial-packet writes are impossible (R8-6, R9-3); `CapInit` rollback: if `RoInitialize` succeeds but a later step fails, `CapInit` calls `RoUninitialize` before returning — no partial init state (R9-6)
- Picker: two-step size-discovery/take API — `PickerGetResult(takeHandle=0)` probes `requiredNameChars` and `fileSize` without transferring; `takeHandle=1` transfers the file handle exactly once; `PickerRelease` closes untaken handles; every pointer parameter classified as mandatory or optional with validation order; null mandatory pointers (`state`, `hresult`, `handleTaken`, `fileHandle` with `takeHandle=1`) return `E_POINTER` without transfer/close; negative `nameBufLen` treated as zero capacity; complete truth table covers all null/negative combinations (R7-6); table-driven ABI tests for every row (R7-6); every error path specified for repeat take, release-before-take, invalid takeHandle, and PENDING state (R4-4, R5-4)
- Probe artifact vs production draft: the bridge/probe writes **short disposable native-format evidence WAVs** to prove the capture path; they are not user drafts; no production bounds apply; the production recording task (future) implements a streaming mono downmixer, freezes the canonical upload format, and enforces 180 s / 50 MiB against upload-ready mono bytes; `toEngineFormat` is not used for recording (R5-5)
- Draft safety: Go alone writes `.partial` and post-terminal sidecars. Recovery
  derives promotion from numeric reason, requires canonical `reasonName`, and
  validates the signed-int32 HRESULT against one eleven-row compatibility
  table: lifecycle/user zero; cancel=`0x800704C7`; permission, device loss,
  overflow, format, and discontinuity exact codes; unknown WASAPI is a negative
  code excluding the privacy/device/cancel/integrity sentinels, while
  context-dependent `E_INVALIDARG` remains legal for a non-format API stage.
  Missing/corrupt/duplicate/unknown/
  incompatible sidecars discard fail-closed. Parser tests cover nested/array
  duplicates, typed required/unknown fields, every table row and all
  incompatible pairings, while process-kill recovery retains the evidence
  matrix. `parseWAV` remains a local playback parser, not ingest validation.
- Lifecycle evidence: **valid user media** requires finalized `.wav` or proven-recoverable `.partial` (with matching sidecar — R11-5); permission revoke/cancel/overflow/discontinuity/WASAPI error/format error/too-short is **evidenced deliberate discard** (not a pass, not a failure); queued `CaptureRequestStop` alone is never a pass; `AppCapability` fallback is conditional on proven WASAPI revoke detection, not unconditionally "degraded but acceptable"; same wording in state machine, scenarios, unresolved proofs, and this final answer (R4-7)

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
- `CapturePrepare` returning `CoInitializeEx` failure HRESULT directly (all async failures travel through the operation — R7-4)
- unbounded MTA-readiness wait (5-second finite timeout in waiter — R7-4, R12-4)
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
- UI thread or MTA callback publishing terminal directly (must route through capture thread via composite barrier, except pre-handoff cancel where callback publishes only after `threadDone==1` fence — R9-1)
- `threadDone` set AFTER terminal publication (observer of terminal might not see `threadDone==1`; thread must access only locally duplicated handles after `threadDone` — R9-2)
- committing ring producer index before `ReleaseBuffer` succeeds (failed `ReleaseBuffer` leaves committed-but-invalid data in the ring — R9-3)
- reading stop-reason and flipping to TERMINAL state as separate non-atomic operations (CAS can interleave → stale reason published — R9-4)
- multiple threads calling `CaptureRead`/`CaptureGetResult`/`CapPermissionCheck` concurrently (single-owner waiter eliminates race — R9-5)
- `WM_ENDSESSION` sending `GracefulQuit` command to waiter (no time for orderly cleanup; signal `shutdownEvent` and return — R9-5)
- `CapInit` returning failure after `RoInitialize` without calling `RoUninitialize` (leaks apartment ref — R9-6)
- calling `threadDone` "the final session-state access" (the terminal store is the final access; `threadDone` means "cleanup complete, one terminal store remains" — R10-2, R11-1)
- standalone overflow/error cleanup sequences that omit seal, `threadDone`, fence, terminal store, or `localNotify` (all paths must follow the §Normative cleanup path — R11-1)
- a separate `cancelled` bit in the packed word (cancellation is `state=STOPPING` + `reason=CANCEL` — R11-2)
- internal capture failures directly sealing from `CAPTURING` state (must first CAS to `STOPPING` via the internal-failure CAS, then seal — R11-2)
- signaling `stopEvent` for `PREPARING`, `PREPARED`, or `ACTIVATING` (all use `captureThreadWakeEvent`; only `CAPTURING` uses `stopEvent`)
- `CaptureGetResult` mapping `SEALED` to a hardcoded state like `ACTIVATING` (must read the stored `lastPublicState` from the packed word — R11-2)
- graceful quit that abandons picker/permission/enumeration without issuing their cooperative `IAsyncInfo::Cancel` requests, or that invents an `IAsyncInfo` for the synchronous default-device wrapper; every pending wrapper remains waiter-owned through terminal/release
- graceful quit that "waits for callback refs=0" with no observable ABI (must use `CapIsQuiescent` — R11-3)
- posting `WM_QUIT` without both `CapDestroy==S_OK` and the winning graceful-complete CAS; destroy retries are timer-driven with no finite count, while forced exit is the separate 30-second bound
- ignoring `signal.Notify` for Ctrl-C/SIGTERM on Windows (must bridge to `GracefulQuit` command — R11-3)
- carrying `pMixFormat` through all cleanup rows (free immediately after `Initialize` succeeds, copy needed fields first — R11-4)
- `GetBuffer`/`ReleaseBuffer` "any failure" → `WASAPI_ERROR` contradicting per-HRESULT `E_ACCESSDENIED`→`PERMISSION_REVOKE` (split running-stream rows by known HRESULT — R11-4)
- unconditionally calling `Stop` before `Start` has succeeded (use `started` ownership flag; `Stop` before `Start` returns `S_FALSE`, not `AUDCLNT_E_NOT_STOPPED` — R11-4)
- unconditionally releasing `IAudioCaptureClient` before `GetService` has succeeded (use `serviceAcquired` flag — R11-4)
- startup recovery that promotes any structurally valid `.partial` orphan (must check `.partial.reason` sidecar; missing/corrupt/stale/non-promotable → discard — R11-5)
- orphan recovery that does not recheck current permission (even with a promotable sidecar reason, permission must be `Allowed` at recovery time — R11-5)
- blocking `CaptureStart` that waits for MTA-readiness on the UI thread (UI pump frozen for up to 5 seconds; use two-step `CapturePrepare`/`CaptureActivate` — R12-4)
- `PostQuitMessage` on `CapDestroy` failure (retry indefinitely; `ForceQuit` watchdog handles the timeout — R12-2)
- `GetDefaultAudioCaptureIdAsync` (no such API; `MediaDevice.GetDefaultAudioCaptureId` is synchronous — R12-2)
- helper/capture-thread as sidecar writer (Go is the sole writer; helper never receives draft paths or touches filesystem — R12-3)
- trusting a redundant `promotable` boolean in the sidecar JSON (derive promotability from reason enum; `reason=PERMISSION_REVOKE` + `promotable=true` must not reach promotion — R12-3)
- overflow and discontinuity tied at the same priority (total order: overflow(1) > discontinuity(2) — R12-6)
- `initialized` as the name of the `IAudioClient`-obtained flag (renamed to `audioClientOwned` — R12-5)
- restating cleanup function body in each HRESULT table row (rows set cause/flags and call the function — R12-5)

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
- `[MS-48]` `IAudioClient::Stop` return values (`S_OK`, `S_FALSE` for already-stopped, `AUDCLNT_E_NOT_INITIALIZED`, `AUDCLNT_E_SERVICE_NOT_RUNNING`); `IAudioClient::GetMixFormat` caller-must-free-with-`CoTaskMemFree` contract, null on failure:
  <https://learn.microsoft.com/en-us/windows/win32/api/audioclient/nf-audioclient-iaudioclient-stop>
  <https://learn.microsoft.com/en-us/windows/win32/api/audioclient/nf-audioclient-iaudioclient-getmixformat>
- `[MS-49]` `IAsyncInfo::Cancel` — cooperative cancellation request; `AsyncStatus::Canceled`; `Completed` handler fires for all terminal states including cancellation; C++/WinRT concurrency model:
  <https://learn.microsoft.com/en-us/uwp/api/windows.foundation.iasyncinfo.cancel>
  <https://learn.microsoft.com/en-us/uwp/api/windows.foundation.asyncstatus>
  <https://learn.microsoft.com/en-us/windows/uwp/cpp-and-winrt-apis/concurrency-2>
- `[MS-50]` `_beginthreadex` for multithreaded CRT startup/cleanup and its
  closeable thread handle; `CreateThread` early-start/CRT warning; closing a
  thread handle does not terminate the thread:
  <https://learn.microsoft.com/en-us/cpp/c-runtime-library/reference/beginthread-beginthreadex?view=msvc-170>
  <https://learn.microsoft.com/en-us/windows/win32/api/processthreadsapi/nf-processthreadsapi-createthread>
  <https://learn.microsoft.com/en-us/windows/win32/api/handleapi/nf-handleapi-closehandle>
