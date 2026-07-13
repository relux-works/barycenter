# TASK-260712-2y74io — R6 F15–F24 lifecycle rework outcome

Date: 2026-07-13  
Role: developer  
Disposition: ready for independent review and root line-by-line review; signed-Windows hardware gates remain mandatory.

## Superseding outcome

This resource replaces every earlier R6 revision. Live root directives through F24 arrived before handoff. The resulting production change treats prepare, native ownership, lifecycle settlement, evidence, UI readiness, activation-call ownership, and stop/result/release identity as one ordered state machine:

- Every successful native prepare creates an exact immutable generation/operation owner. A publication conflict one-shot stops unpublished B at the result seam without clearing, stopping, settling, or releasing incumbent A.
- The lifecycle mutex covers prepare admission, helper return, ownership publication, conflict disposal, and tracker commit/rollback. Lifecycle stop cannot observe B as authoritative or bind the wrong operation.
- Pre-helper suppression distinguishes an exact same-generation native incumbent from a genuinely ownerless invalidated generation. A suppressed duplicate is diagnostic-only; it cannot falsely settle A's generation.
- Native-result evidence and successful-owner successors are separate gates. Ordinary failures emit one result row; abrupt closure suppresses it; unpublished success emits neither result evidence nor successor work.
- Published success commits `captureOp`/generation state before potentially blocking result evidence. Readiness can therefore be consumed against the exact operation even while evidence I/O is blocked.
- If required prepare evidence fails, the exact owner is one-shot stopped. Terminal draining reuses the stored stop result and cannot issue a second native stop.
- Queued readiness atomically claims activation intent on the exact owner before activation evidence. Stop-first admits no intent/native work; a stop during intent evidence blocks native admission; native-first permits the admitted callback and its exact one-shot stop follows.
- Stop reuse reports not-requested, pending, or completed state. Pending is never represented as `S_OK`; query-failure cleanup retains artifact/native ownership and cannot finalize, release, or clear until the recorded result is visible. Confirmed shutdown remains nonblocking and suppresses post-latch ordinary retry.
- Successful native admission owns the external activation-call interval. Stops arriving after admission are retained and run once after `CaptureActivate` returns; stops winning before admission suppress activation. Completion/abandon is armed before the post-admission shutdown check, so close-after-admission performs zero activation and still drains its deferred stop exactly once.
- All active stop APIs preserve generation plus operation identity and require the exact still-published owner. Authoritative terminal release marks/clears that owner; a late callback becomes not-requested with no native fallback, and a reused operation ID cannot cross generations.
- Confirmed shutdown still stops exact active A once before latch/wake without entering the lifecycle mutex or adding evidence, release, finalization, sync, cleanup, or helper destruction to `WM_ENDSESSION`.

No capability, manifest, package identity, AppContainer boundary, native helper ABI, accepted Rev16 ownership boundary, permission model, or production mock/fallback path changed.

## Exact changed-file inventory

Hashes are SHA-256. “Before” is the accepted R5/current-pre-R6 value. Probe directories are untracked as whole trees, so reviewers must inspect every current file in full rather than rely on `git diff`.

| File | Before | R6 F15–F24 SHA-256 | Scope |
| --- | --- | --- | --- |
| `pulsar-win/cmd/pulsar-win-probe/coordinators.go` | `01e5732d1590918bdf6a974edd448b91fdcd10f8455d4d9c89f9cecbf870aacb` | `9b11b99e6b43d47b90ec0fc48cf1f903bbb080a1186faf4eb8801d02bcca85d9` | Exact ownership, released-owner rejection, activation state, deferred one-shot stop, explicit stop outcome, pending cleanup gate, and post-helper dispatch. |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle.go` | `97b8b99fdb84c85533631f45ae7f4d950da43664175d9308eaa1ca2da6119c1d` | `acf1a135b42d28d254a2a707903b4748034f54bf8db3d89362ce7a1ccd145bf0` | Locked prepare transaction plus generation+operation lifecycle-stop callback identity. |
| `pulsar-win/cmd/pulsar-win-probe/main_windows.go` | `a38fef8130cb6d87007b4eaf40096bad818fdd2fd488775109fd0ccad8638254` | `386387ba3e28b421ea410f3b25f92ba7a89a23c9a43ceabbd7cd2aed0148201f` | Production conflict/suppression, owner-gated activation, exact generation+operation stop routing, pending evidence, and cleanup/release gating. |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle_r3_test.go` | `7a1625d2a24a193ee389c82e26496f9e8a232d6c97ac20e73b78322c6a188486` | `805c56fceee3bc61f7506acd90fe9b11f27a6f4e096edf57246985671d7ce5c9` | Existing lifecycle-stop tests now assert/preserve the bound generation argument. |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle_r4_test.go` | `1316439bb2cfc1a0667697517579d6b80f530da279c2dfc730cb814635e05f97` | `ca39aa58dfab61dacdf0923e601fe5242995b3d8b6e1c31ecaccf3a3224a0b90` | Shared production prepare helper checks exact owner and both published-success gates. |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle_r5_test.go` | `c965cb628ebabff503677ed9267ef62051fd9ff5201207c7c2401f4723173bb1` | `cb2ba971e396567fcc08a78b9c088ccb20a611a276f5783c3dd42586aaed4c23` | Late prepare/activation shutdown checks including nonblocking wake before deferred activation-owned stop. |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle_r6_test.go` | new | `c04f3bd904ea4faa9aa76770a0db2c151daaefb7546c72ea5b1e042cdc73bcd8` | Deterministic F15–F24 ownership, rollback, admission/call/abandon, stale-release, reused-ID, pending-stop, confirmation, and retry schedules. |
| `pulsar-win/cmd/pulsar-win-probe/lifecycle_source_test.go` | `af14fd7cb00c1b21dbb7cd07c0d3b0286bea50201de0fa82e0c660a57c652d53` | `e17a6be49ac0ef1b1d4f2dc0d2abb0922e5c54a7b0f46a8fe9dca1c096e23073` | Production wiring, activation completion order, exact stop identity, pending cleanup gate, and no ownerless fallback. |
| `pulsar-win/probe-msix/README.md` | `ae6fcad9b20cba9990c41ad3330c36bf4fc32cba9ad5b691fd3fbb535c3f9520` | `8bfd29b7253eb94e2d2ca699392a3d65bba323362d4cb0fb84ca0edb692dd926` | Documents activation-call ownership, exact stop identity, released-owner suppression, and pending/completed cleanup ownership. |
| `LOGBOOK.md` | `497f159470e500d070276fbe4f54a272247a1945530faa48cf467f7f78ee62ac` | `9070ab50a038a7178911495535e8e8ec73474190a6f18834eabf74b6750a5766` | Records the F15–F24 findings and corrections. |

No other project file was changed in this R6 run. No commit or push was made.

## Invariant and test mapping

| Finding / invariant | Production result and deterministic evidence |
| --- | --- |
| F15 exact unpublished owner | `TestR6F15OpenGatePublicationConflictStopsUnpublishedOwnerOnce` exposes B and incumbent A, stops B once, rejects stale clear, and admits no successor. |
| F16 atomic rollback | `TestR6F16LifecycleStopWaitsForConflictRollbackAndBindsIncumbent` blocks B disposal inside the production transaction; concurrent lifecycle stop waits, then binds/stops A. |
| F16 distinct settlement | `TestR6F16DistinctUnpublishedGenerationMaySettleWithoutTouchingIncumbent` permits settlement only for a distinct ownerless generation and preserves A. |
| F17 result versus owner gates | `TestR6F17PrepareFailureEvidenceIsSeparateFromOwnerSuccessors` covers open failure, abrupt failure, failed duplicate, and published success. |
| F18 suppressed duplicate | `TestR6F18SuppressedDuplicatePreservesExactNativeIncumbent` proves same-generation suppression performs no helper call or false settlement, preserves A/run/owner, and later accepts A's real settlement; its distinct-generation case preserves the unrelated incumbent while settling the pre-native generation. |
| F19 state before evidence | `TestR6F19PublishedStatePrecedesBlockingResultEvidence` blocks the real evidence callback, observes published state, consumes readiness/activates exactly once, then fails evidence and proves exact-owner stop once with zero terminal fallback stop. |
| F20 stop versus queued activation | `TestR6F20StoppedOwnerRejectsQueuedActivation` covers stop-first (zero intent/native work and no later stop duplication) and native-admission-first (one native callback followed by one exact stop). Its stop-first path drives real lifecycle terminal/release settlement. |
| F21 pending stop ownership | `TestR6F21PendingStopRetainsReleaseOwnership` blocks the winning native stop, proves fail-closed query and terminal callers see pending with zero fallback/finalize/release, proves confirmation latches/wakes without waiting, suppresses post-confirmation retry, then proves an ordinary completed-result retry finalizes/releases exactly once. |
| F22 activation-call ownership | `TestR6F22NativeCallOwnershipDefersStop` blocks after native admission but before the fake `CaptureActivate`, races query/hotkey/confirmation stops, proves no stop-before-activate or release-before-stop, then observes one activation followed by one stop; its stop-first case performs zero activation. |
| F23 close-after-admission abandon | `TestR6F23AdmissionAlwaysCompletesOnClosingRace` blocks exactly between admission and the second shutdown check, proves confirmation returns/wakes immediately, then proves zero activation, one deferred stop through the armed abandon defer, no release while pending, and one closing-first stop. |
| F24 released/reused exact owner | `TestR6F24ExactOwnerStopNeverFallsBackAfterRelease` covers lifecycle plan bind followed by terminal settle+clear before callback (zero stop), the same operation ID on a new generation (zero stale/cross-generation stop), a stale released pointer, and a real exact owner with one deferred stop plus pending/completed reuse. |
| Close before continuation | `TestR6F15AbruptCloseBeforeContinuationKeepsExactActiveOwner` proves B stays one-shot stopped, all follow-ons are suppressed, and confirmation stops A before latch/wake. |
| Production wiring | `TestR3WindowsWiringUsesProductionLifecycleCoordinators` requires the real `dispatchPostHelper` path, state assignment before `capture_prepare`, one result row, and stop-result reuse. |
| Lifecycle baseline | Focused R3–R6 tests cover confirmed shutdown priority, release permits, sticky evidence suppression, durable UI transitions, permission fail-closed behavior, quit deadline, rearm, hotkey retry, and generation settlement. |
| AppContainer/privacy/artifacts | Manifests are unchanged; exact capability, package-helper, recursive privacy redaction, confirmed-shutdown partial preservation, and owned-artifact cleanup tests pass. |

## Exact verification after the last project edit

Host: macOS 15.7.4 / Darwin 24.6.0 x86_64; `go version go1.26.0 darwin/amd64`.

From `pulsar-win`:

```bash
go test ./cmd/pulsar-win-probe -run '^TestR6F24|^TestR6F23|^TestR6F22|^TestR5F12' -count=100
go test -race ./cmd/pulsar-win-probe -run '^TestR6F24|^TestR6F23|^TestR6F22|^TestR5F12' -count=50
test -z "$(gofmt -l cmd/pulsar-win-probe/coordinators.go cmd/pulsar-win-probe/lifecycle.go cmd/pulsar-win-probe/main_windows.go cmd/pulsar-win-probe/lifecycle_r3_test.go cmd/pulsar-win-probe/lifecycle_r4_test.go cmd/pulsar-win-probe/lifecycle_r5_test.go cmd/pulsar-win-probe/lifecycle_r6_test.go cmd/pulsar-win-probe/lifecycle_source_test.go)"
go test ./cmd/pulsar-win-probe ./internal/winprobe -run 'TestR3|TestR4|TestR5|TestR6|TestArtifactWriterConfirmedShutdown' -count=50
go test -race ./cmd/pulsar-win-probe ./internal/winprobe -run 'TestR3|TestR4|TestR5|TestR6|TestArtifactWriterConfirmedShutdown' -count=20
go test ./... -count=1
go test -race ./... -count=1
go vet ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go vet ./...
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go vet ./...
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go build -o /tmp/TASK-260712-2y74io-r6-probe-amd64.exe ./cmd/pulsar-win-probe
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go build -o /tmp/TASK-260712-2y74io-r6-probe-arm64.exe ./cmd/pulsar-win-probe
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/TASK-260712-2y74io-r6-probe-test-amd64.exe ./cmd/pulsar-win-probe
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go test -c -o /tmp/TASK-260712-2y74io-r6-probe-test-arm64.exe ./cmd/pulsar-win-probe
GOOS=windows GOARCH=amd64 CGO_ENABLED=0 go test -c -o /tmp/TASK-260712-2y74io-r6-winprobe-test-amd64.exe ./internal/winprobe
GOOS=windows GOARCH=arm64 CGO_ENABLED=0 go test -c -o /tmp/TASK-260712-2y74io-r6-winprobe-test-arm64.exe ./internal/winprobe
file /tmp/TASK-260712-2y74io-r6-probe-amd64.exe /tmp/TASK-260712-2y74io-r6-probe-arm64.exe /tmp/TASK-260712-2y74io-r6-probe-test-amd64.exe /tmp/TASK-260712-2y74io-r6-probe-test-arm64.exe /tmp/TASK-260712-2y74io-r6-winprobe-test-amd64.exe /tmp/TASK-260712-2y74io-r6-winprobe-test-arm64.exe
go test ./internal/winprobe -run '^TestSanitizeLogEvent' -count=50
go test ./internal/winprobe -run 'TestArtifactWriterConfirmedShutdownAppendNeverSyncsOrClosesPartial|TestArtifactWriterAbortRetriesOwnedCleanupPostcondition|TestManifest|TestProbeManifestKeepsReviewedSandbox|TestPackagePayloadRequiresActualHelper|TestSanitizeLogEvent' -count=1
xmllint --noout msix/AppxManifest.xml.in probe-msix/AppxManifest.xml.in
```

Pass summary, preserving every command result:

```text
PASS  F22–F24 + F12 activation/shutdown schedules x100: probe 0.565s
PASS  F22–F24 + F12 race x50: probe 1.829s
PASS  gofmt inventory: no output
PASS  focused R3/R4/R5/R6/artifact x50: probe 2.015s; winprobe 0.777s
PASS  focused race x20: probe 2.688s; winprobe 1.987s
PASS  full uncached: root 3.061s; probe 0.832s; winprobe 2.577s; wire 1.573s
PASS  full race: root 5.366s; probe 1.501s; winprobe 3.259s; wire 2.202s
PASS  host vet: no output
PASS  Windows amd64 vet/build/probe-test-compile/winprobe-test-compile: no output
PASS  Windows arm64 vet/build/probe-test-compile/winprobe-test-compile: no output
PASS  file: all six outputs are PE32+ x86-64/AArch64 Windows executables
PASS  recursive privacy x50: winprobe 0.362s
PASS  artifact/manifest/sandbox/package-helper/privacy suite: winprobe 0.368s
PASS  xmllint for both manifest templates: no output
```

From repository root:

```bash
bash .task-board/.resources/TASK-260712-6kba80/windows-consistency-check.sh .task-board/.resources/TASK-260712-dib11l/windows-capture-bridge-r16.md
test "$(rg -c '^@startuml$' .task-board/.resources/TASK-260712-2y74io/p1-windows-store-spike-lifecycle.puml)" -eq 1
test "$(rg -c '^@enduml$' .task-board/.resources/TASK-260712-2y74io/p1-windows-store-spike-lifecycle.puml)" -eq 1
if rg -n '[[:blank:]]+$' LOGBOOK.md pulsar-win/probe-msix/README.md pulsar-win/cmd/pulsar-win-probe/coordinators.go pulsar-win/cmd/pulsar-win-probe/lifecycle.go pulsar-win/cmd/pulsar-win-probe/main_windows.go pulsar-win/cmd/pulsar-win-probe/lifecycle_r3_test.go pulsar-win/cmd/pulsar-win-probe/lifecycle_r4_test.go pulsar-win/cmd/pulsar-win-probe/lifecycle_r5_test.go pulsar-win/cmd/pulsar-win-probe/lifecycle_r6_test.go pulsar-win/cmd/pulsar-win-probe/lifecycle_source_test.go; then exit 1; fi
git diff --check
task-board validate
```

```text
PASS  Rev16 consistency: RESULT: PASS (0 anti-patterns in normative body)
PASS  diagram: exactly one @startuml and one @enduml
PASS  changed-file whitespace scan: no output
PASS  git diff --check: no output
PASS  task-board validate after outcome replacement
```

## Corrected development anomalies

- After F17 moved the continuation marker, the first repeated focused and race runs failed only the source-boundary assertion because it still sliced through the old marker and included a later legitimate evidence-failure stop. The boundary was corrected, and every fresh command above passed.
- The first combined F19 patch did not match the current context and made no mutation. The edits were then applied in isolated patches and verified.
- The explicit F21 return-type migration first exposed expected compile errors in three portable test assertions and then four Windows-only call sites. Each caller was converted to pending/completed handling; subsequent host, Windows test compilation, focused, full, race, vet, and cross-build commands all passed.
- The first F22/F23 targeted compile exposed one missing `sync` import in the new barrier test. The import was added; the exact targeted suite, x100/race x50 schedules, and every fresh verification command above then passed.
- The F24 generation-aware callback signature intentionally surfaced seven portable test compile errors; each existing callback was updated to accept/assert the bound generation before the fresh verification matrix passed.
- Earlier R6 outcome revisions were attached before later live directives and are replaced in place by this resource; there is one R6 outcome name.

No product test, build, vet, manifest, privacy, or static failure remains in the reported verification.

## Worktree scope and residual platform gates

The parent worktree contains extensive pre-existing/sibling coordinator, docs, CI, planning, research, and task-board changes. Probe directories are untracked as whole trees, so `git diff` alone is not review evidence. The ten-file inventory above is the exact R6 scope; no unrelated edit was reverted.

Not run and not claimed:

- Native MSVC helper tests, PowerShell packaging, MakeAppx, signing, and WACK: `pwsh`, `cl.exe`, `makeappx.exe`, and `signtool.exe` are unavailable on this macOS host.
- Signed MSIX/AppContainer Windows 10/11 delivery and timing for query/confirmed/cancelled shutdown, suspend, WTS lock/unlock, and `AppCapability.AccessChanged`.
- Real microphone revoke, repeated hardware capture cycles, native operation registry, hotkey release, temporary-artifact startup recovery, and process-exit observations.
- PlantUML rendering: `plantuml` is unavailable; exact delimiter validation is the host gate.

No host test is presented as signed-Windows hardware evidence. Fresh independent review and root line-by-line/hash/test audit remain mandatory.
