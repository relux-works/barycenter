# TASK-260712-2y74io — lifecycle rework guard R10

Date: 2026-07-13
Owner: root review
Verdict entering this round: **BACK TO DEVELOPMENT**

This guard is mandatory and cumulative with the accepted Rev16 bridge,
R3-F10/W1-W4 abrupt-shutdown contract, all prior root guards, and the task AC.
Do not weaken an earlier ownership, evidence, privacy, or retry invariant to make
this round pass.

## 1. Frozen input and reproduced blocker

Root read the R9 report and relevant production paths, verified the frozen
hashes, and independently reran the review-only reproducer 100 times normally
and 100 times under `-race`. The test intentionally asserts the defective
behavior; both commands passed, proving that Finalize and CaptureRelease cross
the confirmed-shutdown latch after a pre-confirmation Stop blocks.

Frozen SHA-256 values:

- `pulsar-win/cmd/pulsar-win-probe/coordinators.go`
  `1114a0a692b981eb46c2bd5508c3ccab050addf0bd4b2fc72fa2fd7832443a5a`
- `pulsar-win/cmd/pulsar-win-probe/lifecycle.go`
  `8931b7cecd3bdece1366655c89cdca5d9cb5cb8177d4dba0a0458dc544535d68`
- `pulsar-win/cmd/pulsar-win-probe/main_windows.go`
  `36f971e8c7877c0ab80d289d59dba09356faf5bc7551be239365f460bb2e3455`
- `pulsar-win/cmd/pulsar-win-probe/window_windows.go`
  `97a65fab63958e2b84814312d8b06e6e1f81a91cc31a616c7d58ae27072580a1`
- `pulsar-win/cmd/pulsar-win-probe/lifecycle_r6_test.go`
  `e20b8341739d8744623643f1763e75afba54e424318b5f90ebe8f86040d5be56`
- `pulsar-win/cmd/pulsar-win-probe/lifecycle_r8_test.go`
  `7e58ada410164a5612539a0a3537dd4c712ec041128889914fcae2814ca5206e`
- `pulsar-win/cmd/pulsar-win-probe/lifecycle_source_test.go`
  `99a78a3fa1e120c645930fe67ed68d0db376f34c3855e4751afb0a172bcee749`
- `pulsar-win/probe-msix/README.md`
  `1919528ea96be20d948b9aced81093bde152a7d1c16f23656650d3f8b2181327`
- `docs/diagrams/p1-windows-store-spike-lifecycle.puml`
  `17cb913ec3a93703ddf307020e2356842615d9577b1c234e901aded6bd756c02`
- task lifecycle diagram
  `3a1685e3385636ec96187a76750e5f37d71d24cc2557a874da596364f03e1aa1`
- R8 producer outcome
  `692c4d2b1823dba9b6445e969bbeeafba9109f637aa3cb63516b851069319691`
- R9 independent report
  `2585858f8b71313ee84bbf869140f70794a403a3ea6e91c7fa462226636463f4`

Re-hash before editing. `window_windows.go` already has the correct confirmed
wndproc boundary and should remain byte-identical unless a concrete conflict is
reported. The concurrent DPAPI/onboarding task owns root-package credential
files; do not touch them or use their transient module-wide failures as a
reason to expand this task.

## 2. R10-F28 — operation-level abrupt admission

The current `shutdown.runOrdinary` admits an entire waiter drain. That is too
coarse. A drain admitted while open may block in Stop, confirmation may latch,
and the same callback then starts Finalize and Release. Admission must instead
be immediately around every independently dangerous operation.

Implement one nonblocking, atomic-load/CAS-compatible abrupt gate for each
ordinary operation, including all production paths for:

- capture query and buffered take/read;
- artifact append/write, periodic sync, Finalize, Abort, cleanup retry, and
  any filesystem successor;
- permission query and any other helper call;
- exact-owner CaptureRelease, including query-failure cleanup, terminal
  cleanup, finalized-release retry, and artifact-cleanup retry;
- UI posts, passing lifecycle settlement, hotkey/tray cleanup, evidence/log
  enqueue, sync, and graceful-success successors.

An outer drain permit may remain as an optimization, but it is never authority
for a later operation. Each operation must obtain its own permit immediately
before the actual callback. Confirmation remains lock-free/nonblocking and
never waits for an admitted callback.

Linearization rules:

1. An operation whose own permit linearized before `closing=true` may finish
   after confirmation.
2. An operation without its own pre-close permit executes zero callback calls.
3. After any admitted callback returns, re-check the abrupt gate before every
   successor. A post-latch result may update only the minimum internal one-shot
   call/result state needed to prevent duplicate native ownership calls.
4. After the latch, do not clear the published owner, settle a lifecycle as
   passing, post UI work, enqueue/physically write evidence, unregister the
   hotkey/tray, finalize/abort another artifact, call another helper export, or
   start Release.
5. A CaptureRelease invocation admitted before confirmation may return once.
   It must not cause post-latch owner-clear/lifecycle/UI/evidence successors.
   Do not call Release twice to compensate.
6. Stop claimed before confirmation may return/publish its exact result once;
   it gives no authority to admit later Finalize or Release.
7. The confirmed wndproc stays exactly: close start gate, exact-owner
   nonblocking Stop, monotonic confirmed latch, one wake, return. Do not move
   ordinary cleanup into it and do not take lifecycle/artifact/evidence locks.

Do not encode abrupt suppression as fake HRESULT failure or structural
evidence. A callback rejected by the shutdown gate was not attempted. Preserve
retry ownership for cancelled graceful paths, but confirmed process handoff
belongs to Windows/startup recovery.

## 3. Required production-seam schedules

Add a narrowly named R10 test file and exercise real coordinators/production
seams, not a parallel toy state machine. At minimum prove deterministically:

1. Query failure enters exact-owner Stop and blocks; confirmed shutdown returns
   without waiting; Stop later returns exact S_OK; Finalize and Release remain
   zero.
2. Confirmation after Stop-result publication but before Finalize admission
   produces zero Finalize/Abort/Release.
3. Finalize is individually admitted and blocks; confirmation returns; the
   admitted Finalize may finish, but Release, owner clear, lifecycle settlement,
   UI post, and passing evidence remain zero.
4. Confirmation immediately before exact-owner Release admission produces
   zero Release.
5. Release is individually admitted and blocks; confirmation returns; Release
   finishes exactly once, with no post-latch owner clear, lifecycle settlement,
   UI post, passing evidence, duplicate Stop, or Release retry.
6. A terminal CaptureGetResult or CaptureRead admitted before confirmation may
   return, but no later permission query, artifact operation, Release, log, or
   UI successor starts.
7. Finalized-release retry and pending-artifact cleanup retry each reject their
   not-yet-admitted Release/Abort when confirmation wins.
8. Confirmation remains bounded while every callback seam above is blocked.
9. Cancelled WM_ENDSESSION and graceful quit/suspend/lock/revoke still retain
   their existing ordered cleanup, retry, evidence, hotkey/tray, and lifecycle
   settlement behavior.
10. Operation/generation reuse, pending Stop producer, exact S_OK release gate,
    orphan ownership, and failed Release retry from R8 remain intact.

Run the new schedules at least `-count=100` and under `-race -count=50`.
Include source-boundary checks that enumerate every production
`CaptureRelease` call site and prove it remains both exact-owner gated and
operation-admission gated. Avoid a string test that passes merely because a
comment contains the expected token.

## 4. R10-F29 — truthful lifecycle diagrams

Update both:

- `docs/diagrams/p1-windows-store-spike-lifecycle.puml`;
- `.task-board/.resources/TASK-260712-2y74io/p1-windows-store-spike-lifecycle.puml`.

Show separate branches for graceful quit/lock/suspend/revoke, cancelled
shutdown, and confirmed `WM_ENDSESSION`. The confirmed branch must show:

- exact-owner nonblocking Stop before latch/wake;
- rejection of not-yet-admitted ordinary operations;
- one bounded no-sync partial handoff append by the waiter;
- waiter exit and Windows/startup-recovery ownership.

It must show no confirmed-branch Finalize, Abort, Release successor, evidence
sync/log, hotkey unregister, tray cleanup, helper destruction, or lifecycle
PASS settlement. Do not collapse graceful and abrupt ownership into one arrow.
Keep PlantUML delimiters valid; render if tooling exists and otherwise report
the unavailable renderer honestly.

## 5. Scope and handoff

Allowed implementation scope is limited to lifecycle production files, a new
R10 test, narrowly necessary existing lifecycle/source tests, the two diagrams,
README clarification if semantics changed, LOGBOOK task block, and a new
task-scoped outcome. Do not edit native bridge ABI, manifests, root-package
DPAPI/onboarding files, audio/product code, or prior outcome/review resources.
Do not commit, push, reset, checkout, clean, or create checked-in binaries.

The producer outcome must be
`TASK-260712-2y74io_rework-r10-results.md` and include:

- exact changed-file SHA-256 inventory;
- invariant-to-test mapping for every schedule above;
- fresh focused/repeated/race, relevant host test/race/vet, Windows amd64 and
  arm64 build/vet/test-compilation, privacy/manifest/XML/Rev16/static/diagram,
  `git diff --check`, and `task-board validate` results;
- explicit unrelated concurrent root-package failures, if any, without editing
  the sibling task;
- honest residual signed-Windows/MSIX/hardware gates.

Completion of the producer run is not acceptance. Fresh independent review,
root full-file audit, frozen hash verification, and root reruns are mandatory.
