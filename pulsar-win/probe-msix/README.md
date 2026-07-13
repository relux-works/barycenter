# Packaged Windows probe

This Windows-only probe packages `pulsar-win-probe-amd64.exe` and the actual
`pulsar-capture.dll` helper in an x64 `packagedClassicApp` + `appContainer`
MSIX. The only added device capability is `microphone`; file access is through
`FileOpenPicker` and a take-once brokered read handle.

`ReluxWorksLLC.PulsarProbe` is a separate validation identity only. It is not
the Store product package, and creating this unsigned probe MSIX does not by
itself establish Partner Center or Windows App Certification Kit eligibility.

On a Visual Studio 2022 developer shell:

```powershell
./probe-msix/build-probe.ps1 -Version 0.1.0.0
```

The script builds and runs native tests, runs all Go tests, cross-builds the
probe GUI, asserts the helper DLL is staged, and validates package creation
with MakeAppx. It does not claim signed-hardware evidence: signing, WACK, and
real Windows 10/11 scenario runs remain required evidence gates.

The visible window provides separate `Record default` and `Record selected`
actions, `Stop`, brokered picker, and hide controls. The tray duplicates those
controls. `Ctrl+Shift+R` toggles the currently selected capture mode. Structured
JSON Lines evidence is written to package-private
`%LOCALAPPDATA%\Packages\<package-family>\LocalState\PulsarProbe`. Unpackaged
development uses the normal Go user-config directory.

## Lifecycle cleanup and evidence

The hidden top-level window is the lifecycle owner. Each observed edge receives
a monotonic `cleanupId`; `scenarios.jsonl` records `cleanupOrder`,
`cleanupStage`, `lifecycleEdge`, `lifecycleMode`, `stopReason`, and the exact
latest `observedOSSignal` plus the ordered repeated-signal history. Capture and
permission continuations carry a monotonic capture generation. Closing a
lifecycle gate and binding that exact generation is one synchronized
transition, so an older or later generation cannot satisfy its cleanup run.
Terminal, temporary-artifact, and native-release observations are retained as
independent monotonic facts for that generation. If a callback wins the race
before stop publication, the facts replay only after the run reaches the stop
stage; release is not treated as proof that artifact cleanup succeeded.
The selected signal paths are:

| Edge | Selected packaged-probe signal | Behavior |
| --- | --- | --- |
| Explicit quit | tray `Quit`, hidden-window `WM_CLOSE`, or `Ctrl-C`/`SIGTERM` | Asynchronous graceful stop and drain; permission callback, hotkey, WTS registration, helper, tray, and log are released in order before `WM_QUIT`. |
| Suspend | `WM_POWERBROADCAST/PBT_APMSUSPEND` | Nonblocking capture stop with `CAP_REASON_SUSPEND`; hotkey stays unregistered until a resume message is observed. Capture is not restarted automatically. |
| Session lock | `WTSRegisterSessionNotification` then `WM_WTSSESSION_CHANGE/WTS_SESSION_LOCK` | Nonblocking capture stop with `CAP_REASON_LOCK`; hotkey stays unregistered until `WTS_SESSION_UNLOCK`. |
| Permission revoke | `AppCapability.AccessChanged` plus `CheckAccess`; the existing bounded permission poll is a deterministic defensive signal | Nonblocking stop with `CAP_REASON_PERMISSION_REVOKE`; evidence is discarded fail-closed and the hotkey remains unavailable until access is allowed again. |
| Windows sign-out/shutdown | `WM_QUERYENDSESSION`, followed by `WM_ENDSESSION` | Requests stop without blocking the window procedure. A cancelled shutdown returns through ordinary idle cleanup; confirmed shutdown performs no post-latch cleanup/evidence and hands remaining ownership to Windows/startup recovery. |

For graceful quit with an active capture, the asserted order is signal, stop
request, capture settlement, temporary-artifact disposition, capture release,
permission callback unsubscribe, hotkey unregister, WTS unregister,
`CapDestroy`, tray deletion, evidence-log sync, and process-exit-ready. Tray
ownership is retained when `Shell_NotifyIconW(NIM_DELETE)` fails so the UI
thread can retry it. The exit-ready record is itself synced before
`PostQuitMessage`. A capture result-query failure is logged as
`terminalObserved=false`; it is not presented as a native terminal result.
Owned temporary paths are retried through the production
`ArtifactWriter.Abort` postcondition before later cleanup stages may advance.

Quit intent does not share the bounded ordinary command queue. The waiter
observes it on every bounded poll, so a full queue or failed wake cannot lose
terminal cleanup. Permission-ready, capture-ready, discovery, picker, and
rearm starts validate the current lifecycle gate; stale capture continuations
are logged no-ops and never call prepare or activate.

Idle cleanup and permission-rearm callbacks are durable waiter-owned intents.
The UI acknowledges an exact intent ID while `PostMessageW` is in flight or
after it succeeds; post completion observes synchronous consumption instead of
recreating the intent. A failed post is retried by the bounded waiter poll and
repeated failures escalate to graceful exit. Rearm itself remains a capture
start gate until the current
waiter permission result, discovery initiation, and UI hotkey registration have
been accepted as one transition. An `AccessChanged` query failure, or any
runtime permission-query failure while a capture generation is owned, closes
the permission gate and starts the same fail-closed stop path before diagnostic
logging. Failed explicit-record and lifecycle-rearm permission queries use that
same ordering: requested work is settled or stopped, the rearm token is closed,
and no permission-ready, discovery, hotkey, prepare, or activation continuation
is published.

Evidence sync after helper destruction uses a bounded retry so a storage fault
cannot leave a hidden hung process. The 30-second process deadline stays armed
through helper destruction and evidence sync until posting `WM_QUIT` is
irrevocably committed. The watchdog is logged when armed. Its hard-exit
callback, evidence-retry exhaustion, and the user-visible `Force Quit` path do
not put another potentially blocking log or filesystem sync in front of the
sole exit action. Missing process-exit-ready evidence therefore makes the run a
failure and startup recovery remains the next signed-run check; it is never a
passing clean-shutdown result.

JSONL writes and syncs run through one ordered, bounded evidence coordinator.
The first short write, error, queue saturation, or acknowledgement timeout is
sticky: the worker discards every queued successor without invoking the logger
or sync callback, and synchronous successors receive the same non-nil failure.
Later code cannot emit `evidence_log_synced` or a passing process-exit claim,
and bounded retry exhaustion takes the nonblocking hard-exit path. The evidence
worker also shares the confirmed-shutdown ordinary-work gate: an already
admitted callback may finish, but enqueue attempts and queued logger/sync
callbacks are suppressed without sanitization or physical I/O after the latch.
Before serialization, nested typed fields are cloned and scrubbed with bounded
cycle depth. Absolute Windows, UNC, POSIX (including root-level files), and
`file:///` paths trigger whole-value redaction, as do original picker names,
usernames, auth/credential/token/password values, and audio/payload content.
Evidence keeps generated session IDs, hashes, sizes, reasons, and result codes
instead of local artifact names. The top-level `DeviceID` has a narrower,
explicit trust boundary: it is populated only by the Windows default-device or
enumeration APIs, so recognized `\\?\SWD#MMDEVAPI#...` and ordinary MMDevice ID
forms remain available as exact hardware evidence. Credential text and actual
filesystem paths in that field are still rejected, and nested fields never use
the device-ID exception.

Suspend, lock, and permission-revoke paths return idle after capture/artifact
cleanup and hotkey unregistration. Resume or restored permission may re-register
the hotkey, but never restarts the previous capture. This makes repeated
start/stop and lifecycle cycles explicit rather than leaving a hidden active
session.

There are two real platform limits that cannot be resolved on a non-Windows
host or inferred from an unpackaged build:

- AppContainer delivery of power and WTS session notifications must be
  exercised with the signed MSIX on the Windows 10/11 hardware matrix. A failed
  WTS registration is logged as `blocked`, including `GetLastError` and the next
  action; without registration the probe cannot directly observe session lock.
- Once Windows confirms `WM_ENDSESSION`, Windows owns the remaining lifetime.
  The current native capture generation and operation are published together
  as one immutable atomic owner snapshot. The window procedure closes the
  abrupt start gate, claims that exact snapshot without entering the ordinary
  lifecycle mutex, requests its one-shot stop, publishes the monotonic
  confirmation latch, and wakes the waiter. It then returns without unregistering the hotkey,
  advancing lifecycle evidence, or releasing any resource. Confirmation wins
  over every coalesced ordinary wait event. The waiter may append at most eight
  already-buffered 4096-frame reads to an existing safe `.partial`, without
  logging, sync, finalization, cleanup, result-take, release, UI publication, or
  helper destruction, and then exits. While the stop call is between the closed
  gate and confirmation latch, the waiter suppresses ordinary drains but stays
  alive; it exits only after observing confirmation and claiming the bounded
  abrupt drain. An outer waiter-drain admission is not cleanup authority: each
  query/read, permission or helper call, artifact write/sync/finalize/abort,
  exact-owner release, UI post, lifecycle settlement, and evidence operation
  acquires its own atomic pre-close permit immediately before the callback. A
  callback holding that permit may return after confirmation, but its result
  cannot start a separately gated successor. In particular, an admitted Stop
  cannot authorize a late Finalize or Release, and an admitted Release may
  publish its one-shot result but cannot clear the owner or assert passing
  lifecycle/evidence state after the latch. Both wndprocs also guard their
  entry from gate closure: the
  still-running message pump returns protocol-required values directly and
  suppresses every queued app, timer, hotkey, command, close, cancel/resume, and
  repeated-shutdown callback during and after confirmation. A helper prepare or
  activation callback admitted before confirmation may finish, but it observes
  the abrupt gate on return. Every successful prepare owns an exact immutable
  generation/operation snapshot even if publication conflicts with the active
  owner. The unpublished loser is one-shot cancel-stopped synchronously at the
  helper-result seam. A same-generation duplicate readiness message is rejected
  by capture phase before a second helper call, so it creates no registry entry
  and cannot disturb the incumbent. If a real successful prepare nevertheless
  loses publication to a distinct/stale atomic incumbent, the lifecycle tracker
  keeps that loser native-owned. Its unique one-shot Stop callback is atomically
  claimed and stored before the exact orphan obligation becomes waiter-visible;
  only then is native Stop invoked. A waiter at that publication-to-invocation
  seam therefore observes a live pending producer, never a structural gap. The
  loser is never activated and owns no
  artifact or ordinary UI/evidence successor. On the ordinary open-gate path,
  only the waiter may query its exact ID to terminal and invoke `CaptureRelease`
  through the loser's own release gate. A failed release retains the obligation
  for exact retry; success removes only that obligation and settles only its
  rejected generation, never the distinct active owner. New recording,
  `CLEANUP_READY`, and `CapDestroy` remain blocked while either an active or
  orphan owner exists. Confirmed shutdown never waits for an orphan Stop/query
  and admits zero orphan query/release work; Windows reclaims that boundary.
  Post-helper result evidence has a separate
  admission from successful-owner successors: an ordinary open-gate
  `CapturePrepare` failure records its HRESULT once without creating or
  activating an owner, while abrupt gate closure suppresses that pending row.
  A duplicate attempt preserves the incumbent generation/operation and invokes
  no second helper operation or result row. A queued duplicate refused before the helper
  after suspend/lock has already bound native A is diagnostic-only: it cannot
  fabricate terminal, artifact, release, or idle-cleanup progress for A. A
  genuinely pre-native invalidated generation retains suppressed settlement.
  For a published success, exact app-side operation/generation state is made
  visible before the potentially blocking result-evidence write, so an immediate
  auto-reset readiness signal cannot be consumed while the waiter still sees
  operation zero. Evidence failure then claims the exact owner's one-shot cancel
  stop before quit escalation; abrupt stop cannot claim it again, and graceful
  terminal cleanup reuses the recorded HRESULT instead of calling native stop a
  second time. Queued readiness must claim activation intent on that exact owner
  before activation-intent evidence. Stop, intent admission, and native
  admission are atomic: a stop-first owner emits no activation evidence and
  performs no native activation; a stop that arrives during intent evidence
  prevents native admission; and an already-admitted native callback is followed
  by the same one-shot exact-owner stop. Capture drain, hotkey, lifecycle, quit,
  and shutdown stop paths reuse that claim instead of issuing a second native
  stop. Reuse exposes explicit pending versus completed state: an in-flight stop
  is never logged as `S_OK`, and query-failure cleanup cannot abort an artifact,
  release native ownership, clear the exact owner, or finalize lifecycle release
  until the recorded stop result is visible or independent native-terminal
  evidence authorizes the normal terminal path. Confirmed shutdown never waits
  for the in-flight result and admits no later ordinary cleanup retry. Native
  activation admission owns the helper-call interval: a later query, hotkey,
  lifecycle, or confirmation stop is retained as pending and invoked once only
  after the admitted `CaptureActivate` callback returns. This prevents both
  stop-before-activate and release-before-stop while keeping confirmed shutdown
  nonblocking. The completion/abandon defer is armed immediately after successful
  native admission, before the second shutdown check, so close-after-admission
  cannot strand a deferred stop even when the external activation call is
  correctly suppressed. Every later stop request carries both generation and
  operation identity. If authoritative terminal handling already released and
  cleared that exact owner, the late request is a not-requested no-op—there is
  no direct native fallback, and a reused operation ID in another generation is
  never stopped. Stop admission, native Stop result publication, terminal
  observation, and the actual waiter-owned `CaptureRelease` call now share that
  immutable `(generation, operation ID, owner)` state. A pending result always
  has either the immediate Stop callback or the admitted activation/deferred
  Stop callback as its live producer. Release cannot overtake either producer;
  the release-admitted bit covers the helper call itself, and only exact `S_OK`
  marks the owner released and permits the same pointer to be cleared. A failed
  or unexpected success HRESULT retains the owner for retry and bounded
  fail-closed exit. Query-failure cleanup requires exact `S_OK` from Stop before
  artifact abort or release; a failed Stop is structural evidence, not terminal
  evidence. Independently observed native terminal state may authorize its
  normal terminal path only after any already-admitted Stop result is visible.
  Because Stop is nonblocking, an immediate query-failure `CaptureRelease` may
  honestly return `E_ILLEGAL_METHOD_CALL` until native terminal cleanup catches
  up. That result is recorded as waiting, not success or structural terminal
  proof; finalized retry derives authority only from the exact completed
  `S_OK` Stop (or separately observed terminal), retries the same owner/ID, and
  clears/settles once only after exact `S_OK` Release.
  `CapturePrepare` returning `S_OK` with operation ID zero is classified before
  publication as an ABI failure: no native API receives zero, the generation is
  settled, required redacted evidence is acknowledged first, and only then is
  the already-bounded graceful-exit path armed. Every task-owned structural
  ownership/ABI path uses that same explicit evidence-before-escalation API;
  evidence failure suppresses the successor and leaves sticky evidence-failure
  cleanup authoritative, while confirmed shutdown suppresses both callbacks.
  Required structural evidence and graceful escalation therefore have separate
  permits: an evidence callback admitted before confirmation may return, but it
  cannot arm escalation afterward. The same rule applies after an orphan
  Release: orphan removal, passing evidence, lifecycle settlement, and UI wake
  each require a fresh permit and remain suppressed when Release returns after
  the latch.
  A first result query that rejects a published
  nonzero operation as an invalid handle similarly retains ownership and enters
  bounded fail-closed exit without fabricating terminal or release evidence.
  Non-window SIGTERM/evidence-failure entry, the graceful watchdog callback,
  post-message-pump error logging/UI, and deferred local cleanup use the same
  gate, so they also add no post-latch work.
  Windows still cannot guarantee a terminal callback or durable file/log sync
  before process termination, so the hardware run must inspect fail-closed
  partial recovery on the next launch.

Portable Go tests drive the production generation ledger, exact atomic capture
owner, prepare/activation continuation, durable UI-intent,
bounded evidence, lifecycle, permission-query, rearm, resource-ownership,
timer-fallback, and process-exit coordinators. They validate the message
decision table, deterministic pre/post-publication race schedules, injected
post/write/sync/stall failures, cleanup ordering, privacy redaction,
idempotency, stop-to-confirmed waiter/message barriers, and 100 repeated cycles.
They are host-verifiable evidence, not a
substitute for the signed-MSIX lifecycle and hardware gate.
