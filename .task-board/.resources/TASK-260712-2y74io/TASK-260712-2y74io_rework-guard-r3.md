# TASK-260712-2y74io — root review round 3 / mandatory R4 guard

Date: 2026-07-13  
Reviewer: root orchestrator  
Verdict: **REWORK — R3 is not accepted**

Root recomputed every producer hash, read the ten-file R3 inventory and the
independent R3 review in full, inspected the final production paths, reran the
focused/full/race/vet/Windows-cross/manifest/Rev16 checks, and confirms both
independent HIGH findings. Green builds do not exercise either ordering below.
This guard is cumulative with the accepted Rev16 bridge and all prior root
guards; it supersedes the R3 producer outcome where they conflict.

## R3-F10 — confirmed `WM_ENDSESSION` crosses ordinary release paths (HIGH)

Current production detects `shutdownEvent` in `waiter`, but calls every ordinary
drain before testing `confirmedShutdown`. A terminal permission, enumeration,
default-device, capture, picker, or artifact operation can therefore execute its
normal query/take/finalize/abort/`*Release` path after Windows has confirmed
session termination. This contradicts Rev16: the wndproc requests the
nonblocking capture stop, signals shutdown, and returns; the waiter may make a
best-effort buffered write to the existing `.partial`, but it must not release
registry ownership, promote/delete artifacts, post cleanup-ready, or destroy the
helper. Windows and next-launch recovery own the remaining boundary.

Required correction:

1. Confirmation must become a nonblocking, monotonic production latch before
   `shutdownEvent` is signalled. The waiter must check that latch with priority
   over lower-index/coalesced wait events, not merely when
   `WaitForMultipleObjects` happens to return the shutdown-event index.
2. After confirmation, no new ordinary drain/release permit may start. Make the
   linearization explicit so a release already admitted before confirmation may
   finish, while every release/query/take/finalize/abort attempt admitted after
   confirmation is rejected. The wndproc must never wait for an in-flight
   release and must still return promptly.
3. Branch before `drainTerminalIntent`, command, permission, enumeration,
   default-device, normal capture, artifact-cleanup, picker, UI-transition, or
   evidence-failure drains. Run only a shutdown-specific best-effort capture
   drain that:
   - reads already-buffered PCM only when the current writer/format make that
     safe;
   - keeps the artifact as `.partial` for startup recovery;
   - is bounded by available buffered data and never starts/restarts capture;
   - never calls any `*Release`, result-take, permission query, `Finalize`,
     `Abort`, `CapDestroy`, `File.Sync`, cleanup-ready post, rearm, or ordinary
     lifecycle cleanup;
   - emits at most nonblocking/best-effort evidence and then exits the waiter
     without closing OS-owned process resources.
4. Preserve the required order: `CaptureRequestStop(shutdown)` is initiated by
   the wndproc before the confirmation wake. No test-only branch or mock may
   enter the packaged production path.

Required executable evidence:

- A portable production coordinator/seam test with active capture plus pending
  permission, enumeration, default-device, picker, and artifact ownership.
  Coalesce ordinary event readiness with confirmed shutdown and prove shutdown
  wins regardless of wait-array index.
- Assert stop linearizes before the shutdown wake, a bounded buffered write may
  occur, the partial remains recovery-owned, the waiter exits, and counters for
  every release/take/finalize/abort, `CLEANUP_READY`, UI transition,
  `CapDestroy`, and clean evidence sync remain exactly zero.
- Add a barrier race in which an ordinary release permit is acquired just
  before confirmation and another is attempted just after it. The first is
  classified pre-confirmation without blocking the wndproc; the second must be
  rejected deterministically.
- Source-presence assertions alone are not production-seam proof.

## R3-F11 — queued PASS evidence survives a failed prerequisite (HIGH)

The evidence worker sets `failed` when row A returns an error, but then
unconditionally invokes `logFn`/`syncFn` for row B that was queued while A was
still in flight. A later synchronous caller returns false, yet the underlying
JSONL can physically contain B's passing cleanup claim without A. Queue
saturation has the same consequence: it marks failure, but the worker currently
drains already accepted follow-ons into the writer. This violates the root
R2-F5 invariant that no passing cleanup claim survives a missing prerequisite.

Required correction:

1. The first write error, short write, acknowledgement timeout, enqueue timeout,
   or saturation remains sticky. Once sticky failure is observed, the worker
   must not invoke `logFn` or `syncFn` for any subsequent queued operation.
2. Drain queued operations only as control messages: discard asynchronous
   follow-ons and send a stable non-nil sticky error to every synchronous reply
   channel so no caller or worker can hang. The failed prerequisite must be
   linearized before its reply.
3. If saturation/timeout marks failure while the current callback is in flight,
   that callback may finish, but all queued successors are suppressed. Do not
   claim that every accepted row is later written after a sticky failure.
4. Unknown operation kinds must fail closed rather than receive a successful
   acknowledgement.

Required executable evidence:

- Block required row A inside the real production `logFn`; queue both an async
  passing lifecycle row B and a synchronous passing row C before releasing A;
  make A fail; prove the underlying writer sees A only, B/C are never invoked,
  C returns false without hanging, `syncFn` is never called, and health/clean
  claims remain false.
- Repeat with failure introduced by queue saturation while the first callback
  is blocked. Update the existing saturation test: after overflow, queued rows
  are discarded rather than counted as physically processed.
- Exercise a blocked `syncFn`/timeout followed by queued pass evidence and prove
  the same suppression rule. Use real lifecycle action names, not unrelated
  dummy channels.
- Run these schedules repeatedly and under `-race` with no leaked goroutine.

## Handoff and verification

Update production code, tests, README, LOGBOOK, and the superseding outcome so
they state the corrected abrupt-shutdown and evidence semantics exactly. Return
with an exact changed-file inventory and SHA-256 values, failure-to-test mapping,
all corrected development anomalies, and honest native/signed-Windows gaps.

At minimum rerun after the final edit:

- focused R3/R4 lifecycle and evidence schedules at least 50 times;
- full uncached `go test ./...` and `go test -race ./...`;
- `go vet`, `gofmt`, Windows amd64/arm64 cross-build and Windows test
  compilation;
- manifest/artifact/privacy tests, `xmllint`, accepted Rev16 consistency,
  diagram delimiters, whitespace, `git diff --check`, and `task-board validate`.

Do not commit or push. Do not mark the task done. A fresh independent review and
a new root line-by-line/hash/test audit are mandatory. Signed Windows 10/11
MSIX/AppContainer execution remains a downstream hardware gate and must not be
fabricated.
