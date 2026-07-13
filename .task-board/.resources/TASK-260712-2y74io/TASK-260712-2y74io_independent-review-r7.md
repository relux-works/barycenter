# TASK-260712-2y74io independent review R7

Date: 2026-07-13  
Role: independent reviewer  
Verdict: **BACK TO DEVELOPMENT**

The frozen R6 implementation is buildable and its existing suites are green, but it does not satisfy the stop/result/release invariant required by the R7 guard. Two substantive findings remain. No production code, test, documentation, manifest, or existing board resource was changed during this review.

## Review scope and frozen-input verification

I read the task card, producer R6 outcome, all ten frozen production/test/documentation inputs in full, the lifecycle diagram, `docs/spec-self-contained-audio.md` sections 3.13, 18, and 19 P1.0/P1.7, accepted Rev16 ownership/lifecycle contract, root acceptance, root amendments, and cumulative lifecycle guards. Because the probe tree is untracked, conclusions come from full-file inspection and independently executed production seams, not `git diff`.

Every required SHA-256 matched:

| Frozen input | Observed SHA-256 |
| --- | --- |
| `pulsar-win/cmd/pulsar-win-probe/coordinators.go` | `9b11b99e6b43d47b90ec0fc48cf1f903bbb080a1186faf4eb8801d02bcca85d9` |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle.go` | `acf1a135b42d28d254a2a707903b4748034f54bf8db3d89362ce7a1ccd145bf0` |
| `pulsar-win/cmd/pulsar-win-probe/main_windows.go` | `386387ba3e28b421ea410f3b25f92ba7a89a23c9a43ceabbd7cd2aed0148201f` |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle_r3_test.go` | `805c56fceee3bc61f7506acd90fe9b11f27a6f4e096edf57246985671d7ce5c9` |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle_r4_test.go` | `ca39aa58dfab61dacdf0923e601fe5242995b3d8b6e1c31ecaccf3a3224a0b90` |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle_r5_test.go` | `cb2ba971e396567fcc08a78b9c088ccb20a611a276f5783c3dd42586aaed4c23` |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle_r6_test.go` | `c04f3bd904ea4faa9aa76770a0db2c151daaefb7546c72ea5b1e042cdc73bcd8` |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle_source_test.go` | `e17a6be49ac0ef1b1d4f2dc0d2abb0922e5c54a7b0f46a8fe9dca1c096e23073` |
| `pulsar-win/probe-msix/README.md` | `8bfd29b7253eb94e2d2ca699392a3d65bba323362d4cb0fb84ca0edb692dd926` |
| `LOGBOOK.md` | `9070ab50a038a7178911495535e8e8ec73474190a6f18834eabf74b6750a5766` |

## Findings

### HIGH — F25: stop claim, native stop result, and waiter release are not one ordered ownership transition

Locations:

- `pulsar-win/cmd/pulsar-win-probe/coordinators.go:236-260,295-333,380-403,506-512`
- `pulsar-win/cmd/pulsar-win-probe/main_windows.go:440-443,456-470,807-837,1074-1092,1748-1761`
- `pulsar-win/cmd/pulsar-win-probe/window_windows.go:398-405`
- contradicted claim: `pulsar-win/probe-msix/README.md:180-197`

There are two independently reproduced interleavings.

#### F25-A — a claimed stop can become permanently pending without any result producer

`requestStop` first publishes `captureOwnerStopClaimed` outside `stopMu`. If waiter-owned terminal release then enters `markReleased` first, it sets `captureOwnerReleased` and clears deferred work. When `requestStop` finally enters `stopMu`, it sees released and returns `true` without calling `finishStop` and without setting `stopResultSet`. `requestCaptureStopOrReuse` therefore returns `captureStopPending`. Every retry sees released and returns not-requested, so the promised result can never become observable.

This is production-relevant for UI-originated suspend/lock/shutdown-query lifecycle handling racing the waiter terminal paths. The UI calls `requestLifecycleStop`; the waiter can concurrently finish `CaptureRelease` and `captureOwners.clear`. Independent settlement facts may let the lifecycle run finish, but the evidence row reports an accepted pending `CaptureRequestStop` that never existed. This violates the R6 invariant that pending means a result remains observable and the task requirement for truthful ordered cleanup evidence.

#### F25-B — both immediate and activation-deferred native stop callbacks can execute after `CaptureRelease`

The immediate path unlocks `stopMu` at `coordinators.go:251` before `finishStop` calls the helper closure. The deferred path similarly unlocks at line 315 before calling `finishStop` at line 317. In either path, a waiter that has independently observed authoritative terminal state can call `CaptureRelease` and `captureOwners.clear` before the old closure reaches `Helper.CaptureStop`. The result is a real Release-before-Stop order and a stale stop export against an already released operation ID.

The current production call graph prevents generation N+1 from being published before that stale stop returns: `CapturePrepare`/publication is UI-thread-only, so an UI-originated old stop occupies the only publication thread; waiter-originated stops occupy the sole waiter that owns release. Thus the stronger wrong-generation reuse schedule was **falsified for current production wiring**, and is not claimed as a present failure. The coordinator itself does not encode that protection, but the present blocker is the independently reachable stop-after-release order.

Required correction:

- Make stop acceptance, native callback ownership, result publication, and release admission a coherent production state transition. Once stop is claimed, waiter release must not mark/clear the owner until the native stop is completed, or until authoritative terminal handling atomically classifies the stop as no longer required with a stable non-pending outcome.
- Do not return accepted/pending unless an identified owner still guarantees result publication.
- Apply the same release gate to immediate and `completeNativeActivation` deferred callbacks without blocking `WM_ENDSESSION`.
- Add deterministic production-seam barriers for: release after `StopClaimed` but before `stopMu`; release after immediate callback selection but before native call; release after deferred callback selection but before native call; terminal-first; result failure; confirmed shutdown; and exact operation-ID reuse. Assert no `CaptureStop` occurs after `CaptureRelease`, and every pending outcome has a future completion producer.

Existing `TestR6F24ExactOwnerStopNeverFallsBackAfterRelease` does not cover these windows. It releases fully before requesting stop, or completes a deferred stop without concurrent release. Its reused-ID case invokes the stale request only after old release and new publication, so the early released check rejects it; it does not test an already-claimed/in-flight old stop.

### MEDIUM — F26: successful prepare with invalid operation ID silently strands the capture generation

Locations:

- `pulsar-win/cmd/pulsar-win-probe/coordinators.go:464-492,665-709`
- `pulsar-win/cmd/pulsar-win-probe/lifecycle.go:388-426`
- `pulsar-win/cmd/pulsar-win-probe/main_windows.go:1357-1406`
- helper boundary: `pulsar-win/internal/winprobe/helper_windows.go:217-221`

If the helper boundary returns `S_OK` with operation ID zero, `runCapturePrepareOwned` records `succeeded=true`, but `captureOwnershipCoordinator.publish` returns no owner. `runCapturePrepareCommit` treats this as a successful uncommitted prepare and restores the generation to its previous `captureGenerationRequested` phase. The main conflict path cannot settle it because both `owner` and `conflictingOwner` are nil. No stop/disposal or diagnostic is produced, and the next `beginCaptureGeneration` is rejected indefinitely because the old generation remains active.

Rev16 reserves ID zero and a conforming native helper must never return it on success, so this is lower severity than F25. It is still a concrete fail-open boundary behavior explicitly required by the R7 adversarial scope, and it violates repeated-cycle liveness if the ABI contract is ever breached. The same class applies to a nonzero ID that is not actually registered: the shell publishes unreachable ownership and can remain stuck on repeated invalid result/stop/release calls.

Required correction:

- Validate a successful helper operation ID before lifecycle commit. Invalid success must be a logged ABI-contract failure, must settle/cancel the app generation, and must trigger a bounded fail-closed exit/escalation rather than leave a reusable start gate occupied.
- Retain native tests proving successful `CapturePrepare` always publishes a valid nonzero registered ID, and add production-seam zero/invalid-ID regression tests. Do not fabricate a stop of ID zero.

## Independent executable evidence

I used Go's `-overlay` facility to add review-only tests from `/tmp` without touching the frozen worktree. These tests call the real `captureOwnerSnapshot`, `captureOwnershipCoordinator`, `runCapturePrepareOwned`, and `lifecycleTracker` production seams. They assert the defective outcomes, so a PASS means the interleaving was reproduced, not that the product satisfies the invariant.

Reproduced tests:

- `TestR7F25ReleaseAfterStopClaimLeavesPermanentPending`
- `TestR7F25ImmediateStopCanRunAfterWaiterRelease`
- `TestR7F25DeferredActivationStopCanRunAfterWaiterRelease`
- `TestR7InvalidZeroSuccessfulPrepareStrandsGeneration`

Commands and results:

```text
go test -overlay=/tmp/TASK-260712-2y74io_r7_overlay.json ./cmd/pulsar-win-probe -run '^TestR7' -count=100
ok relux.works/duet/pulsar-win/cmd/pulsar-win-probe 0.394s

go test -race -overlay=/tmp/TASK-260712-2y74io_r7_overlay.json ./cmd/pulsar-win-probe -run '^TestR7' -count=100
ok relux.works/duet/pulsar-win/cmd/pulsar-win-probe 1.913s
```

No repository file was added by the overlay.

## Existing suite and static verification

All unrelated/general health checks passed:

```text
go test ./cmd/pulsar-win-probe ./internal/winprobe -run 'TestR3|TestR4|TestR5|TestR6|TestArtifactWriterConfirmedShutdown' -count=50
PASS: probe 2.059s; winprobe 0.811s

go test -race ./cmd/pulsar-win-probe ./internal/winprobe -run 'TestR3|TestR4|TestR5|TestR6|TestArtifactWriterConfirmedShutdown' -count=50
PASS: probe 4.668s; winprobe 1.756s

go test ./... -count=1
PASS: root 4.390s; probe 0.369s; winprobe 2.059s; wire 1.114s

go test -race ./... -count=1
PASS: root 5.400s; probe 1.956s; winprobe 3.027s; wire 2.757s

go vet ./...
PASS: no output

GOOS=windows GOARCH={amd64,arm64} CGO_ENABLED=0 go vet ./...
GOOS=windows GOARCH={amd64,arm64} CGO_ENABLED=0 go build ./cmd/pulsar-win-probe
GOOS=windows GOARCH={amd64,arm64} CGO_ENABLED=0 go test -c ./cmd/pulsar-win-probe
GOOS=windows GOARCH={amd64,arm64} CGO_ENABLED=0 go test -c ./internal/winprobe
PASS: all commands; six outputs identified as PE32+ x86-64/AArch64 Windows executables

go test ./internal/winprobe -run '^TestSanitizeLogEvent' -count=50
PASS: 0.366s

go test ./internal/winprobe -run 'TestArtifactWriterConfirmedShutdownAppendNeverSyncsOrClosesPartial|TestArtifactWriterAbortRetriesOwnedCleanupPostcondition|TestManifest|TestProbeManifestKeepsReviewedSandbox|TestPackagePayloadRequiresActualHelper|TestSanitizeLogEvent' -count=1
PASS: 0.349s

xmllint --noout msix/AppxManifest.xml.in probe-msix/AppxManifest.xml.in
PASS: no output

bash .task-board/.resources/TASK-260712-6kba80/windows-consistency-check.sh .task-board/.resources/TASK-260712-dib11l/windows-capture-bridge-r16.md
PASS: RESULT: PASS (0 anti-patterns in normative body)

gofmt -l <frozen Go inventory>
PASS: no output

rg -n '[[:blank:]]+$' <frozen inventory>
PASS: no matches (expected `rg` exit 1)

PlantUML delimiter checks
PASS: exactly one @startuml and one @enduml

git diff --check
PASS: no output

task-board validate
PASS: Board is valid. No issues found.
```

`plantuml` is unavailable on this host, so rendering was not rerun. The architecture-diagram skill's required syntax/delimiter inspection passed and the supplied lifecycle sequence remains structurally consistent with its stated participant flow.

## Architecture and acceptance assessment

- Generation and operation identity are correctly paired in immutable owner snapshots, stale already-released pointers are rejected, and current UI/waiter affinity prevents cross-generation operation-ID reuse during an in-flight stop.
- Abrupt confirmation remains nonblocking, waiter-prioritized, and separated from ordinary release/finalization work in the paths inspected.
- AppContainer capabilities, manifests, privacy redaction, artifact ownership, and signed-hardware claims were not weakened or overstated.
- F25 nevertheless breaks the required Stop-before-Release ordering and truthful pending/result evidence. F26 breaks repeated-cycle liveness on an invalid successful helper result. Therefore the lifecycle AC and review definition of done are not met.

## Residual platform gates

Not run and not claimed on this macOS host:

- native MSVC helper compilation/CTest and injected native registry/operation tests;
- PowerShell packaging, MakeAppx, signing, WACK, and installed MSIX execution;
- signed AppContainer Windows 10/11 lifecycle delivery and timing for query/confirmed/cancelled shutdown, suspend, WTS lock/unlock, and permission `AccessChanged`;
- real microphone permission revoke, hardware capture, repeated hotkey/capture cycles, process-exit evidence, and startup recovery.

These remain downstream hardware/release gates and do not excuse the host-reproduced coordinator findings.
