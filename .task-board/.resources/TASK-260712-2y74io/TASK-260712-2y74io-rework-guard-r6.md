# TASK-260712-2y74io — root R5 audit / mandatory R6 guard

Date: 2026-07-13  
Reviewer: root orchestrator  
Verdict: **REWORK — R5 is not accepted**

This guard is cumulative with Rev16, root R1–R4, W1–W4, and the live F13/F14
directives. Preserve the dirty worktree; do not commit or push.

## R5-F15 — an unpublished successful prepare can escape every stop (HIGH)

`runCapturePrepareOwned` creates a real native operation inside the lifecycle
callback. `captureOwnershipCoordinator.publish` then attempts
`active.CompareAndSwap(nil, owner)`. If another exact owner is still active,
the CAS returns false without stopping the newly created operation. The normal
main-window continuation later notices `ownerPublished=false` and requests a
cancel, but that continuation is deliberately suppressed after abrupt gate
closure.

Concrete schedule:

1. owner A is already in `captureOwnershipCoordinator.active`;
2. a real lifecycle prepare callback returns successful operation B;
3. publication of B loses the CAS to A and returns `ownerPublished=false`;
4. confirmed `WM_ENDSESSION` closes the abrupt gate before the ordinary
   owner-publication-failure continuation runs;
5. `runCapturePrepareOwned` checks only `owners.matching(generation, B)`, which
   is nil because A remains active;
6. the main continuation containing `CaptureStop(B, ReasonCancel)` is rejected.

B is now neither published nor stopped. This violates the R5 guard requirement
that every native operation created by a late prepare be nonblockingly stopped
and receive no successor. Do not waive the schedule as impossible: the
production lifecycle prepare method does not phase-reject a second delivery,
and duplicate/stale permission-ready delivery or a future caller must fail
closed without leaking a hidden recording operation.

Required correction:

1. Give every successful prepare result an exact immutable owner object even
   when publication loses. Ensure an unpublished operation is one-shot stopped
   at the production ownership/result seam, independent of later ordinary
   continuation admission and independent of whether `closing` has already
   become true.
2. Never stop or clear A while disposing B. Preserve atomic generation +
   operation identity and exactly-once shutdown-stop behavior for confirmation.
3. Remove or make idempotent the later main-window publication-failure stop so
   it cannot issue a second stop for B. Preserve lifecycle settlement/evidence
   only while the ordinary gate is open; do not add logging, release,
   finalization, sync, or cleanup to `WM_ENDSESSION`.
4. Preserve F13: waiter stays alive while `closing=true, confirmed=false`; all
   wndproc application callbacks are suppressed in that interval.

Required executable evidence:

- Use the real production coordinator and lifecycle seam. Bind/publish A, have
  another successful prepare create B, deterministically force B's publication
  conflict, and close the abrupt gate before any ordinary fallback.
- Assert B receives exactly one nonblocking stop and no activate/evidence/post/
  result-take/release/finalize successor. Assert A remains the active exact
  owner until confirmation, confirmation stops A exactly once before latch/wake,
  and no stale clear for B removes A.
- Exercise both open-gate conflict disposal and the close-before-continuation
  schedule; repeat under `-race`. Source-string checks alone do not count.

After the final edit, rerun focused R3–R6 schedules repeatedly, full host tests
and race, vet, Windows amd64/arm64 build plus test compilation, manifest/privacy/
artifact checks, formatting, Rev16 consistency, `git diff --check`, and
`task-board validate`. Replace/freshen the task outcome as
`TASK-260712-2y74io_rework-r6-results.md` with exact hashes and honest signed
Windows hardware gaps. Fresh independent review and root acceptance remain
mandatory.
