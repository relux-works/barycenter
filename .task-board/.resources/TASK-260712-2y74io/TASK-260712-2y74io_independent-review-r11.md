# TASK-260712-2y74io — lifecycle shutdown review R11b result

Date: 2026-07-14 (Asia/Tbilisi)
Base: `3565c1e1ca0511168026ec2ba72440d23fb1317f`
Executor: `codex-inline` (same-executor review at the user's direction; independence is not claimed)

## Verdict

`BACK TO DEVELOPMENT`

The frozen R11b boundary violates the accepted confirmed-shutdown rule that
the bounded no-sync append to an already-owned `.partial` is the sole callback
allowed after the confirmation latch. Three production schedules start a new
native `CaptureStop` callback after that latch.

## Boundary and review evidence

- Read the R9 finding, R10 repair guard, R10 producer handoff, original R11
  guard, corrected R11b inventory, task/Rev16 contract, relevant epic
  specification, every frozen source/test/doc file, and their direct lifecycle
  helpers/callers in full.
- Verified every R11b SHA-256 before review and again after the adversarial
  run. All hashes matched the corrected inventory; both diagrams are
  byte-identical.
- Added no production or existing-test changes before this verdict. The only
  adversarial source was
  `/tmp/barycenter-r11-review.1Bg9cc/pulsar-win/cmd/pulsar-win-probe/lifecycle_r11_review_test.go`.
- Command:

  ```text
  go test ./cmd/pulsar-win-probe -run '^TestR11Review' -count=1
  ```

  Result: all three review tests failed because a helper Stop callback ran
  after confirmation.

## Findings

### R11-F33 — HIGH — late successful prepare starts Stop after confirmation

Files/lines:

- `pulsar-win/cmd/pulsar-win-probe/coordinators.go:911-938`
- `pulsar-win/cmd/pulsar-win-probe/coordinators.go:1191-1239`
- false-positive regression:
  `pulsar-win/cmd/pulsar-win-probe/lifecycle_r5_test.go:11-83`

Concrete interleaving:

1. `CapturePrepare` obtains its pre-close permit and blocks inside the helper.
2. `WM_ENDSESSION(TRUE)` closes the gate, observes no published owner, stores
   the confirmation latch, and wakes the waiter.
3. The admitted prepare returns `S_OK` with a nonzero operation ID.
4. `captureOwnershipCoordinator.publish` observes `isClosing()` and calls
   `owner.requestShutdownStop` at lines 921-923 (or after the CAS at 933-936).
5. `requestShutdownStop -> requestStop -> invokeClaimedStop -> finishStop`
   starts the native Stop callback after the latch.

The existing R5 test requires exactly this forbidden post-latch Stop instead
of asserting OS/startup-recovery handoff. Missing regression: a blocked prepare
must be released only after `isConfirmed()==true` and prove zero Stop/helper,
owner/orphan, evidence, lifecycle, and UI successors.

### R11-F34 — HIGH — published orphan producer can be invoked after confirmation

Files/lines:

- `pulsar-win/cmd/pulsar-win-probe/coordinators.go:788-800`
- `pulsar-win/cmd/pulsar-win-probe/coordinators.go:1223-1229`
- false-positive regression:
  `pulsar-win/cmd/pulsar-win-probe/lifecycle_r8_test.go:912-976`
- false-positive source assertion:
  `pulsar-win/cmd/pulsar-win-probe/lifecycle_source_test.go:119-130`

Concrete interleaving:

1. A successful prepare loses publication to a distinct incumbent.
2. `publishOrphanStopProducer` claims the loser's callback and publishes the
   waiter-visible orphan obligation.
3. The goroutine is preempted before line 1228.
4. Confirmation stops only the incumbent, stores the latch, and wakes the
   waiter without waiting for the orphan.
5. The prepare goroutine resumes and calls `owner.invokeClaimedStop()`, which
   starts the orphan's native Stop after the latch.

`TestR8W4ConfirmationAtOrphanPreInvocationSeam` currently treats step 5 as
passing behavior, directly contradicting R11's sole-exception rule. Missing
regression: the same publication-to-invocation seam must prove that a fresh
ordinary permit rejects the Stop and leaves the obligation to OS/startup
recovery.

### R11-F35 — HIGH — deferred activation completion starts Stop after confirmation

Files/lines:

- `pulsar-win/cmd/pulsar-win-probe/coordinators.go:313-341`
- `pulsar-win/cmd/pulsar-win-probe/coordinators.go:371-397`
- `pulsar-win/cmd/pulsar-win-probe/coordinators.go:1297-1335`
- false-positive schedules:
  `pulsar-win/cmd/pulsar-win-probe/lifecycle_r6_test.go:763-899` and
  `pulsar-win/cmd/pulsar-win-probe/lifecycle_r6_test.go:931-1036`

Concrete interleaving:

1. Native activation acquires its pre-close operation permit and blocks.
2. Confirmation atomically claims the exact owner's Stop callback but cannot
   invoke it while activation owns the native interval.
3. Confirmation stores the latch and wakes the waiter.
4. Activation returns after the latch.
5. deferred `completeNativeActivation` consumes `deferredStop` at lines
   391-395 and starts native Stop with no fresh immediate pre-callback permit.

The activation callback may finish under its own pre-close permit, but that
permit cannot authorize the distinct later Stop callback. Existing tests
require the post-latch call. Missing regression: completion after confirmation
must abandon/suppress the stored callback while ordinary graceful/cancelled
activation completion still invokes it exactly once.

## Required repair

1. Make native Stop invocation itself admission-aware at every ordinary and
   deferred seam; a claim/publication is not an immediate pre-callback permit.
2. Preserve the special confirmed-shutdown ordering only for an exact active
   owner whose Stop callback actually begins before the latch.
3. If prepare/orphan/activation completion reaches invocation after the latch,
   abandon helper work to Windows/process teardown/startup recovery without
   fabricating a Stop result, terminal/release state, lifecycle PASS, evidence,
   UI work, or cleanup.
4. Replace the three false-positive schedules with adversarial regressions for
   both sides of each linearization point, then rerun the full R11 matrix.

Because a defect is sufficient to return the frozen bytes to development, the
remaining full/race/vet/cross/manifest matrix was not used to claim acceptance
on this verdict and must be rerun after repair.
