# TASK-260712-2y74io Windows lifecycle rework R8

R6 is rejected. Implement the independent R7 findings plus the root's failed-stop boundary correction as one ownership-state rework. Preserve every previously accepted F1-F24 invariant. Do not commit or push, and do not mark the task `done`.

## Frozen starting boundary

- `coordinators.go` — `9b11b99e6b43d47b90ec0fc48cf1f903bbb080a1186faf4eb8801d02bcca85d9`
- `lifecycle.go` — `acf1a135b42d28d254a2a707903b4748034f54bf8db3d89362ce7a1ccd145bf0`
- `main_windows.go` — `386387ba3e28b421ea410f3b25f92ba7a89a23c9a43ceabbd7cd2aed0148201f`
- `lifecycle_r3_test.go` — `805c56fceee3bc61f7506acd90fe9b11f27a6f4e096edf57246985671d7ce5c9`
- `lifecycle_r4_test.go` — `ca39aa58dfab61dacdf0923e601fe5242995b3d8b6e1c31ecaccf3a3224a0b90`
- `lifecycle_r5_test.go` — `cb2ba971e396567fcc08a78b9c088ccb20a611a276f5783c3dd42586aaed4c23`
- `lifecycle_r6_test.go` — `c04f3bd904ea4faa9aa76770a0db2c151daaefb7546c72ea5b1e042cdc73bcd8`
- `lifecycle_source_test.go` — `e17a6be49ac0ef1b1d4f2dc0d2abb0922e5c54a7b0f46a8fe9dca1c096e23073`
- `probe-msix/README.md` — `8bfd29b7253eb94e2d2ca699392a3d65bba323362d4cb0fb84ca0edb692dd926`
- `LOGBOOK.md` — `9070ab50a038a7178911495535e8e8ec73474190a6f18834eabf74b6750a5766`
- Independent rejection: `TASK-260712-2y74io_independent-review-r7.md` — `5d93a4d9c88982b8bdf6ff330b10cb0b02a093ae1d640c5e1e4a832eb2c7b261`

Read the frozen Rev16 bridge contract, the R7 report, and all prior task guards/outcomes needed to preserve F1-F24. The untracked probe tree must be reviewed as full files, not as a git diff.

## F25 — one exact Stop/result/Release ownership transition

The accepted owner identity is the immutable `(generation, operationID, owner pointer)` tuple. Implement one coherent state transition that orders all of the following for that exact owner:

1. stop acceptance;
2. ownership of the native `CaptureRequestStop` call;
3. publication of its exact HRESULT or a stable terminal-first `not requested` outcome;
4. admission of the waiter-owned native `CaptureRelease` call;
5. marking/clearing the app-side owner only after successful release.

Mandatory properties:

- A returned `pending` outcome always has a live, identified producer that will publish a result. Release/clear/cancellation cannot destroy that producer.
- If authoritative native terminal evidence wins before a stop is admitted, later stop requests are stable `not requested` and never invoke the helper. Do not fabricate `S_OK`.
- If a stop has been admitted, `CaptureRelease` cannot begin until the nonblocking native stop callback has returned and its result is visible, unless one atomic terminal-first transition cancels the not-yet-invoked stop and publishes a stable non-pending outcome.
- The immediate and activation-deferred paths obey the same rule. No callback selected for generation N may execute after release or against a reused operation ID.
- The release gate must cover the actual `CaptureRelease` invocation. Calling `CaptureRelease` first and only then taking `stopMu`/clearing the owner is insufficient. Route every capture-release site, including terminal, finalized retry, and query-failure recovery, through the exact-owner gate.
- A failed `CaptureRelease` retains the exact owner and a retryable release obligation. A successful release marks and clears only the same pointer/generation/ID.
- Do not introduce a lifecycle/global mutex into confirmed `WM_ENDSESSION`. Its stop request remains nonblocking and bounded; it never waits for terminal, finalizes, releases, syncs evidence, or runs ordinary successors.
- Preserve waiter-only query/read/release affinity, UI-only prepare/activation affinity, abrupt confirmation priority, required evidence gates, and all F1-F24 settlement/artifact rules.

## F26 — invalid successful prepare is fail-closed

- Validate the helper result before lifecycle commit/publication. `S_OK` with operation ID zero is an ABI-contract failure, not a successful uncommitted conflict.
- Never call Stop, query, or Release with ID zero. Settle/cancel the app generation deterministically, write a non-secret required diagnostic, and enter the existing bounded fail-closed quit/escalation path. The next start gate must not remain silently occupied.
- Preserve the distinction between a real publication conflict and an invalid helper result; do not run conflict settlement against nil candidate/incumbent owners.
- Retain/add native contract coverage proving successful `CapturePrepare` returns a registered nonzero ID. Treat a nonzero-but-unregistered ID detected at the first query/result boundary as a structural helper failure with bounded fail-closed ownership cleanup, never an infinite cycle.

## F27 — a failed Stop HRESULT is not terminal evidence

`runCaptureQueryFailureCleanup` currently finalizes and attempts release after any completed stop callback, including a failed HRESULT. Rev16 promises exact `S_OK` for a valid ID/reason; violating that promise is an ABI fault and cannot authorize Release without a terminal result.

- Require the contractually accepted stop result before query-failure finalization/release. A failed HRESULT (and any unexpected non-`S_OK` success code) performs zero artifact finalization, zero `CaptureRelease`, zero owner clear, and zero lifecycle released settlement.
- Retain native/artifact ownership, emit redacted structural evidence, and enter a bounded fail-closed retry/quit path. Do not promote media and do not loop forever on finalized-but-nonterminal state.
- Apply exact-result reasoning consistently to release success as well; unexpected HRESULTs retain ownership and are observable as failures rather than silently treated as success.

## Mandatory deterministic production-seam tests

Add a fresh `lifecycle_r8_test.go` (or a clearly equivalent task-owned test file) and exercise real production coordinators/call paths, not a parallel test-only state machine:

1. Actual release admission after `StopClaimed` but before stop-lock inspection: no ghost pending, no lost producer.
2. Immediate stop selected/callback barrier versus terminal waiter release: recorded order is Stop then Release; never Release then Stop.
3. Activation-deferred stop/callback barrier versus release: same order and one result.
4. Terminal-first with no admitted stop: release succeeds, later stop is `not requested`, no helper Stop call.
5. Stop result failure and unexpected non-`S_OK`: no finalize/release/clear/settlement; exact owner and artifact obligation remain for the bounded failure path.
6. Release failure then retry: owner is retained after failure and exactly one same-owner clear occurs after successful retry.
7. Confirmed shutdown at each stop/activation/release boundary: wndproc path returns without waiting and no forbidden release/finalization/successor runs after confirmation.
8. Operation-ID reuse across generations with an old claimed/deferred callback: old work cannot invoke Stop/Release on the new owner.
9. `CapturePrepare` returns `S_OK, 0`: no owner, no native call with zero, required failure evidence/escalation occurs, and no generation/start gate remains silently stranded.
10. Nonzero invalid/unregistered ID and first query failure: bounded structural failure, no false terminal/release evidence.

Use channels/barriers rather than sleeps for ordering. Timeout branches may only fail a hung test. Assert call order, exact tuple identity, call counts, outcome state, lifecycle facts, artifact ownership, and required evidence. Run the new schedules at high repetition and under `-race`.

## Verification and handoff

After the last edit, run:

- focused F25-F27/F1-F24 tests at high count and focused race repetitions;
- full uncached host tests and full `-race`;
- `go vet ./...`;
- Windows amd64 and arm64 vet, build, and test compilation;
- manifest XML, package payload/helper, sandbox capability, privacy, artifact, Rev16 consistency, diagram delimiter, gofmt, whitespace, and `git diff --check` checks;
- `task-board validate`.

Refresh README/diagram/logbook claims only where the implementation actually changes them. Do not claim native MSVC, signed MSIX, WACK, installed AppContainer, hardware microphone, or Windows 10/11 lifecycle evidence from macOS.

Create exactly one superseding outcome named `TASK-260712-2y74io_rework-r8-results.md`. Record exact changed-file inventory and SHA-256 hashes, F25-F27 line/test mapping, commands/results, dirty-tree boundaries, and honest Windows gates. Set `to-review`; independent review and root line-by-line/hash/test acceptance remain mandatory.
