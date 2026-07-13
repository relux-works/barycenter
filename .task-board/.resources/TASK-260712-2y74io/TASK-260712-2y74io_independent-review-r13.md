# TASK-260712-2y74io — lifecycle shutdown review R13 result

Date: 2026-07-14 (Asia/Tbilisi)
Base: `3565c1e1ca0511168026ec2ba72440d23fb1317f`
Executor: `codex-inline` (same-executor review at the user's direction; independence is not claimed)

## Verdict

`ACCEPT FOR ROOT AUDIT`

No defect was reproduced in the frozen R13 boundary. This verdict is
provisional: the task remains unaccepted until the separate root full-file,
hash, diff, and verification audit completes.

## Boundary integrity

- Read the R11b guard, R11 rejection report, R12 repair outcome, R13 guard,
  task/Rev16 contract, all frozen implementation/test/doc files, and direct
  lifecycle/helper callers.
- Verified all 13 frozen SHA-256 values immediately before review and again
  after every adversarial run; every value matched R13.
- Both PlantUML files remain byte-identical.
- No workspace production, existing-test, or documentation byte changed during
  review. Review-only source was created in
  `/tmp/barycenter-r13-review.G6eHVA/pulsar-win/cmd/pulsar-win-probe/lifecycle_r13_review_test.go`.

## Attempted falsification schedules

### Late successful prepare and lifecycle-result commit

`TestR13ReviewLatePrepareHasNoPostLatchPublicationOrSuccessor` blocked the real
production prepare callback, completed confirmed shutdown, and only then
returned `S_OK` plus operation ID 13101. It proved:

- zero native `Stop` callbacks;
- no active owner and no orphan obligation;
- no app-side owner state, result evidence, failure escalation, activation,
  release, UI, or settlement successor;
- `trackerInvoked=true`, `succeeded=true`, but `trackerAllowed=false`;
- the ledger retained only its pre-latch `prepare-in-flight` state with
  operation ID zero.

This falsifies R11-F33 and root R12-F36 on the repaired seam.

### Atomic owner publication versus confirmation

`TestR13ReviewOwnerPublicationRaceEitherStopsBeforeLatchOrHandsOff` raced the
real `captureOwnershipCoordinator.publish` and confirmed adapter 5000 times per
test invocation. Across the repeated matrices it exercised hundreds of
thousands of schedules. Every iteration resolved to exactly one allowed case:

- publication became authoritative and the confirmed adapter invoked the exact
  owner stop while `confirmed=false`; or
- publication observed closure, retained no active owner, and invoked no
  legacy/late self-stop callback.

No stop callback observed the confirmation latch and no iteration retained an
unpublished owner.

### Orphan publication-to-invocation seam

`TestR13ReviewOrphanInvocationPermitIsEncoded` published a real pending orphan,
confirmed against a distinct incumbent, and invoked the production orphan API
after latch. The encoded fresh permit rejected the callback: zero orphan Stop,
zero query/release, obligation retained, incumbent unchanged. Existing ordinary
R8 schedules still proved exact `Stop -> terminal query -> Release` once.

This falsifies R11-F34 without relying on caller discipline around an unsafe
ungated method.

### Deferred native activation stop

`TestR13ReviewDeferredActivationCannotStartStopAfterLatch` blocked the native
activation interval, confirmed, then released activation. The admitted
activation finished once; deferred Stop, continuation, release, and other
successors remained zero. The R11 ordinary subtest and R8 activation schedules
still invoked the same deferred Stop exactly once while the gate was open.

This falsifies R11-F35 on both sides of its linearization point.

### UI transition and waiter dequeue successors

`TestR13ReviewUIFinishAndEachDequeueNeedFreshPermit` confirmed from inside a UI
post callback. The separately gated `finishPost` mutation and every retry were
rejected, leaving only the already-admitted posting state. A two-entry command
queue consumed the first item before confirmation and retained the second after
confirmation; no stale waiter-iteration permit authorized another dequeue.

### Cumulative R3-R10 schedules

The repeated matrices re-exercised query-failure Stop/Finalize/Release gaps,
blocked Finalize and Release callbacks, orphan Release completion, structural
evidence/escalation, permission and helper operations, artifact retries,
owner-clear/lifecycle/evidence/UI successors, repeated generations and reused
IDs, graceful/cancelled lifecycle behavior, wndproc boundedness, and the sole
bounded no-sync `.partial` append exception. No new failure was observed.

## Verification in the isolated review copy

- Review-only R13 tests: PASS once, PASS x20, race PASS x2.
- R13 + R11 + cumulative R3-R10 focused matrix: PASS x100.
- Same cumulative matrix under race: PASS x20.
- Full `go test ./... -count=1`: PASS.
- Full `go test -race ./... -count=1`: PASS.
- `go vet ./...`: PASS.
- Windows amd64 and arm64: vet, all-package build, probe build, probe test
  compile, and winprobe test compile all PASS.
- Privacy x50 and artifact/recovery/manifest/sandbox/helper/privacy x10: PASS.
- Manifest XML and full probe Go formatting: PASS.

The first isolated full run failed only because the initial review copy
contained `pulsar-win` but omitted the repository-level `protocol/golden`
fixtures expected by `wire/golden_test.go`. Copying the unchanged frozen
fixture directory into the review root made the full and race suites pass. This
was a review-environment completeness error, not a production failure.

Workspace-side Rev16 consistency, diagram identity/delimiters/semantics,
whitespace, `git diff --check`, and `task-board validate` had already passed on
the frozen bytes and are reserved for a fresh root rerun before acceptance.

## Residual platform gates

`pwsh`, MSVC, `makeappx.exe`, `signtool.exe`, PlantUML rendering, native helper
execution, signed-MSIX install/WACK, and installed Windows 10/11 hardware
lifecycle evidence are unavailable on this macOS host. They remain explicit
downstream gates; this review does not claim them.
