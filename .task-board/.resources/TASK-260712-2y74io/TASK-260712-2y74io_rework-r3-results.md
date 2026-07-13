# TASK-260712-2y74io — R3 producer outcome

Date: 2026-07-13. Role: developer. Target: `to-review`; independent and root
line-by-line reviews remain mandatory.

## Scope and Git evidence

This supersedes prior R3 resources and includes R2 F1-F8 plus all live findings:
F9/F9.1, synchronous UI consumption, privacy path/credential recursion,
top-level Windows DeviceID observability, and production evidence-queue
saturation. Rev16 ownership, helper ABI, AppContainer identity/capabilities,
and native capture remain unchanged. No production mock, sandbox weakening,
commit, push, or fabricated Windows evidence was added.

The parent worktree was already dirty and `pulsar-win` is untracked; Git diff
alone is insufficient. The exact task scope is the ten-file inventory below.
`pulsar-win/probe-msix/README.md` documents R3 settlement, UI intent,
permission/evidence/privacy ordering, the trusted DeviceID boundary, and signed
Windows limits. Unrelated coordinator, CI, spec, planning, research, diagram,
and board status entries were untouched.

## Exact inventory and SHA-256

```text
2bb6da0da24a4fd574fcf4dd44f200e9e18dde29f7a5064e5adbef3b95d2899d  LOGBOOK.md
3d67c2fd81cc1daeb92d1ba27dff1bc997d8aacfd1a96e356a51f2d1d97f0823  pulsar-win/probe-msix/README.md
4175d127898d06470fe100f1874e18f6cfffc5e190e75f1ab7105495b05b18b5  pulsar-win/cmd/pulsar-win-probe/coordinators.go
97b8b99fdb84c85533631f45ae7f4d950da43664175d9308eaa1ca2da6119c1d  pulsar-win/cmd/pulsar-win-probe/lifecycle.go
20b4ab84868cc460fb6f3a137f9860e1dbe52dfd4ebdae1f31fd1287f37ea7d3  pulsar-win/cmd/pulsar-win-probe/lifecycle_r3_test.go
23c38a402868b1907a8cac58d20efcb8c37c846943812debcd69ba2661da2783  pulsar-win/cmd/pulsar-win-probe/lifecycle_source_test.go
487d52016e1421312c3f5707bc4fa8fc9ed5872816643628cc11912ff19a7f0f  pulsar-win/cmd/pulsar-win-probe/main_windows.go
92387361c033243f1e4a41309a72a3b034f8ec948207faf9088547e6086f1ee0  pulsar-win/cmd/pulsar-win-probe/window_windows.go
b5ef921f7f548095e38ad850a7b836aa2b0df9ef7378403178b43a31e0bba4cc  pulsar-win/internal/winprobe/log_sanitize.go
af18920c79ba03cfd4b65f739fa3424d02b829dadec3b0e29401a12173684d0e  pulsar-win/internal/winprobe/log_sanitize_test.go
```

These were recomputed after all code, tests, LOGBOOK, and README edits and
immediately before this resource update; no earlier hash remains.

## AC/invariants

| Area | Result |
|---|---|
| Quit/shutdown | Non-droppable intent binds one generation; pending operations cancel; capture/artifact/release, permission/WTS, hotkey/tray, helper, evidence, and exit advance only after real postconditions. Hard deadline arms before I/O and never blocks on evidence. |
| Suspend/lock | Gate closure and nonblocking stop precede evidence; ordered OS signal history persists; artifact/hotkey cleanup precedes current permission rearm. |
| Permission failure | AccessChanged, owned runtime, explicit-record, and rearm query failures enter revoke cleanup before diagnostic I/O. Native stop happens first; requested generation settles in memory; rearm token closes. No permission-ready/discovery/hotkey/prepare/activate continuation survives. Runtime `CapPermissionCheck` remains waiter-owned. |
| Generations | Monotonic IDs bind lifecycle runs. Independent terminal/artifact/release facts survive all registration/stop boundaries, replay in order, and reject N+1. Release never proves artifact disposal. |
| UI intents | Exact-ID pending/posting/queued work retries post failure. Synchronous consume while `PostMessageW` is in flight succeeds; completion cannot recreate/duplicate it. Bounded failure escalates. |
| Evidence | Bounded serialized writes/syncs make error, short write, timeout, and queue saturation sticky. A deterministic test blocks the worker, fills exactly `cap(operations)`, rejects overflow, suppresses health/sync/clean claims, then releases and drains every accepted row. Required rows gate capture continuations and clean exit claims. |
| Privacy/DeviceID | Recursive typed/cycle-bounded sanitation whole-value redacts Windows/UNC/POSIX/root-level/`file:///` paths, sensitive keys, and assignment/whitespace credentials. Only top-level DeviceID from trusted default-device/enumeration output permits recognized `\\?\SWD#MMDEVAPI#...` and MMDevice forms; real file paths/credentials are rejected and nested fields have no exception. |
| Repetition/resources | 100-cycle production-coordinator coverage, repeated 50 times, leaves no intent, generation, rearm token, or stale gate. Hotkey/tray clear only after successful removal; artifacts stay owned until absence is proved. |
| Platform limit | Confirmed `WM_ENDSESSION` may preempt callback/destruction/sync. The probe records this and retains startup recovery; signed native behavior remains downstream evidence. |

## Deterministic test map

- F1: `TestR3F1SettlementLedgerSurvivesEveryStopPublicationBoundary`,
  `TestR3F1GracefulQuitPermissionCancellationAndOverlappingEdgesReplayOneGeneration`,
  `TestR3F1ReleasedGenerationCannotBeAdvancedByNPlusOne`.
- F2: `TestR3F2DurableUITransitionsRetryUntilAcknowledged`,
  `TestR3F2DurableUITransitionHasBoundedGracefulEscalation`,
  `TestR3F2SynchronousConsumeDuringPostCannotLoseIntent`,
  `TestR3F2ConcurrentPublishCoalescesOneOwnedIntent`.
- F3/F9/F9.1: `TestR3F3PermissionQueryFailureDecisionFailsClosedForEveryOwnedState`,
  `TestR3F9PermissionFailureStopsAndClosesGateBeforeBlockedEvidence`,
  `TestR3F9RequiredEvidenceGateSuppressesStartContinuations`,
  `TestR3F91WaiterPermissionCheckFailureSettlesBeforeBlockedEvidence`,
  `TestR3F91PermissionRearmFailureClosesTokenBeforeEvidence`, plus
  `TestR3WindowsWiringUsesProductionLifecycleCoordinators` source ordering.
- F4/F5: `TestR3F4HardDeadlineIsArmedBeforeBlockingEvidenceWrite`,
  `TestR3F5JSONLoggerShortWriteReachesStickyProductionEvidenceSeam`,
  `TestR3F5EvidenceFailureIsStickyAndSuppressesCleanClaims`,
  `TestR3F5EvidenceQueueSaturationIsStickyAndSuppressesCleanClaims`.
- F6: `TestSanitizeLogEventRecursivelyRemovesPrivateValues`,
  `TestSanitizeLogEventDirectPathCredentialAndCycleCases`,
  `TestSanitizeLogEventKeepsCaptureDeviceIdentity` (real device-interface and
  MMDevice IDs plus file/credential rejection).
- F7/F8: `TestR3F7RearmValidationPermissionAndWorkAreOneStartGate`,
  `TestR3F7ConcurrentCaptureWaitsForRearmOwnership`,
  `TestR3F8ProductionCoordinatorsSurviveRepeatedLifecycleCycles`.

Tests use the same portable coordinators called by Windows production; fakes
enter only through production seams.

## Exact verification

Darwin 24.6.0 x86_64; `go version go1.26.0 darwin/amd64`.
Unavailable: `pwsh`, `cmake`, `cl`, `msbuild`, `MakeAppx`, `appcert`, `plantuml`.

After the final test/production change, from `pulsar-win`:

```bash
test -z "$(gofmt -l cmd/pulsar-win-probe/coordinators.go cmd/pulsar-win-probe/lifecycle.go cmd/pulsar-win-probe/lifecycle_r3_test.go cmd/pulsar-win-probe/lifecycle_source_test.go cmd/pulsar-win-probe/main_windows.go cmd/pulsar-win-probe/window_windows.go internal/winprobe/log_sanitize.go internal/winprobe/log_sanitize_test.go)"
go test -count=50 ./cmd/pulsar-win-probe -run 'TestR3F|TestR3WindowsWiring'
go test -count=50 ./internal/winprobe -run 'TestSanitizeLogEvent'
go test -count=1 ./...
go test -race -count=1 ./...
go vet ./...
GOOS=windows GOARCH=amd64 go build -o /tmp/TASK-260712-2y74io-probe.exe ./cmd/pulsar-win-probe
GOOS=windows GOARCH=amd64 go vet ./...
GOOS=windows GOARCH=amd64 go test -c -o /tmp/TASK-260712-2y74io-probe.test.exe ./cmd/pulsar-win-probe
GOOS=windows GOARCH=amd64 go test -c -o /tmp/TASK-260712-2y74io-winprobe.test.exe ./internal/winprobe
xmllint --noout probe-msix/AppxManifest.xml.in
go test -count=1 -v ./internal/winprobe -run 'TestArtifactWriterAbortRetriesOwnedCleanupPostcondition|TestManifest|TestProbeManifestKeepsReviewedSandbox|TestPackagePayloadRequiresActualHelper|TestSanitizeLogEvent'
```

```text
ok  relux.works/duet/pulsar-win/cmd/pulsar-win-probe  1.449s  # focused x50
ok  relux.works/duet/pulsar-win/internal/winprobe      0.354s  # privacy x50
ok  relux.works/duet/pulsar-win                       3.378s
ok  relux.works/duet/pulsar-win/cmd/pulsar-win-probe 0.410s
ok  relux.works/duet/pulsar-win/internal/winprobe     2.655s
ok  relux.works/duet/pulsar-win/wire                  1.506s
ok  relux.works/duet/pulsar-win                       4.170s  # race
ok  relux.works/duet/pulsar-win/cmd/pulsar-win-probe 1.502s  # race
ok  relux.works/duet/pulsar-win/internal/winprobe     3.302s  # race
ok  relux.works/duet/pulsar-win/wire                  2.332s  # race
PASS
ok  relux.works/duet/pulsar-win/internal/winprobe     0.397s  # manifest/artifact/privacy
```

Formatting, vet, Windows cross commands, and `xmllint` emitted no output and
exited 0. Cross binaries were compiled under `/tmp`, not executed.

Repository-root validation after final documentation/logbook edits:

```bash
bash .task-board/.resources/TASK-260712-6kba80/windows-consistency-check.sh .task-board/.resources/TASK-260712-dib11l/windows-capture-bridge-r16.md
test "$(rg -c '^@startuml$' .task-board/.resources/TASK-260712-2y74io/p1-windows-store-spike-lifecycle.puml)" -eq 1
test "$(rg -c '^@enduml$' .task-board/.resources/TASK-260712-2y74io/p1-windows-store-spike-lifecycle.puml)" -eq 1
rg -n '[[:blank:]]+$' <all ten R3 files>  # no matches required
git diff --check
task-board validate
```

Every Rev16 guard printed `PASS`; final line:
`RESULT: PASS (0 anti-patterns in normative body)`. Diagram/whitespace/diff
checks exited 0 silently. Board: `Board is valid. No issues found.`

Development anomalies retained: (1) an early invalid short-writer fake returned
partial count plus nil error and failed 20/20; it was corrected to real
`io.ErrShortWrite`; (2) the first new saturation-test compile attempt failed
with `undefined: fmt`; the missing test import was added and the schedule then
passed 100 repetitions plus all final runs. The unrelated historical
`TestSupervisorSpawnRestartStop` contention anomaly did not reproduce.

## Residual signed-Windows gate

macOS cannot execute MSVC helper, signed MSIX/AppContainer, WACK, microphone/
permission callbacks, WTS lock, suspend, Store shutdown, tray/hotkey, or Windows
10/11 hardware cycles. Independent review must inspect every production file
and reproduce host/race/cross/static checks. Then signed Windows must exercise
quit, lock, suspend, revoke/regrant, query-end-session cancel/confirm, repeated
ownership, exact device evidence, privacy/order, and startup recovery. Root
line-by-line review follows.
