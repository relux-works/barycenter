# TASK-260712-2y74io — R12 lifecycle review repair results

Date: 2026-07-14 (Asia/Tbilisi)
Base: `3565c1e1ca0511168026ec2ba72440d23fb1317f`
Executor: `codex-inline` (strict sequential inline execution; no task-board spawn)

## Outcome

R11-F33/F34/F35 are repaired. A subsequent root audit also found and repaired
R12-F36: a `CapturePrepare` callback admitted before confirmation could avoid
the forbidden late `Stop` yet still advance the lifecycle ledger after the
latch by inheriting the callback's stale permit.

The corrected boundary now enforces all four immediate-operation seams:

1. exact-owner publication has its own pre-close permit and a post-CAS close
   check;
2. orphan producer publication and orphan `Stop` invocation are distinct
   permitted operations, with the invocation permit encoded in its API;
3. deferred activation completion may update the minimum one-shot state, but
   its distinct `Stop` callback requires a fresh permit and is abandoned after
   confirmation;
4. post-helper lifecycle result commit requires its own fresh permit. A prepare
   that returns after confirmation remains at the already-entered
   `prepare-in-flight` state with operation ID zero and is handed to
   Windows/process teardown and next-launch recovery.

Each waiter command dequeue, UI transition claim/finish, invalidated permission
cancel state/callback, orphan structural escalation, graceful deadline stamp,
and evidence-failure mutation also obtains an immediate operation permit.
The confirmed wndproc remains unchanged and nonblocking: close gate, request
the exact active owner's one-shot pre-latch stop, latch, wake, return. The sole
post-latch exception remains the bounded no-sync append to the already-owned
`.partial`.

## Exact frozen implementation inventory

```text
3b2a6babf6c48d701428e6930aa27d143306d740ab5a517a7aff88f0d2241adb  pulsar-win/cmd/pulsar-win-probe/coordinators.go
7195f43017f4b290639621dbaf48814cbd40948e76d3ee65b88bf5fe0559b1a0  pulsar-win/cmd/pulsar-win-probe/lifecycle.go
6fbce3c773b9d4c56cce6692c0567701b669dbf20e2786ba282724327b2cfb00  pulsar-win/cmd/pulsar-win-probe/main_windows.go
97a65fab63958e2b84814312d8b06e6e1f81a91cc31a616c7d58ae27072580a1  pulsar-win/cmd/pulsar-win-probe/window_windows.go
b87ed265f0b6ebe533211d270eabc8cd094620c9ceb2d0bce1d30c24a4383aab  pulsar-win/cmd/pulsar-win-probe/lifecycle_r5_test.go
a3e1e98fd8eca23ae67d534cdc22444f471b1d57ea3cd5db7238f3a6e1ced220  pulsar-win/cmd/pulsar-win-probe/lifecycle_r6_test.go
96b50ebc94f4658ff10bd83275cdf804c570100449be69cb2a1734952ec2537e  pulsar-win/cmd/pulsar-win-probe/lifecycle_r8_test.go
a78b3b91603c560b678520c695817723647c6210bfa3f6b386508b7ace3c233b  pulsar-win/cmd/pulsar-win-probe/lifecycle_r10_test.go
5fb9d9ad4e3d5b2944fdb9c3079ffcba187cd6d7a2b3a8005a1c93d74dbffd1f  pulsar-win/cmd/pulsar-win-probe/lifecycle_r11_repair_test.go
136783d1237cce458e2969810e624ee4a17e10b9b8b61a9b55efc4f2ef2dd74f  pulsar-win/cmd/pulsar-win-probe/lifecycle_source_test.go
a15b5a94c16d9fb69774b2b2293eacb5c4daca1a0ff728fc2d3ccf14ab760d3c  pulsar-win/probe-msix/README.md
391a1b73e77dd64fc34c696804f2f379897799cd79bce6eea052efeb85f4a68d  docs/diagrams/p1-windows-store-spike-lifecycle.puml
391a1b73e77dd64fc34c696804f2f379897799cd79bce6eea052efeb85f4a68d  .task-board/.resources/TASK-260712-2y74io/p1-windows-store-spike-lifecycle.puml
```

Frozen predecessor review:

```text
2ceb0b6497d35cb4c0995a4201953cba75fbc3d941ceb5286b4a7373c1592314  TASK-260712-2y74io_independent-review-r11.md
```

## Finding-to-regression mapping

| Finding | Production repair | Deterministic evidence |
|---|---|---|
| R11-F33 late prepare `Stop` | publication and lifecycle commit have fresh permits; no late self-stop | `TestR11LatePrepareHandsOffWithoutPostLatchStop`, strengthened `TestR5F12ConfirmedShutdownDoesNotWaitForPrepareAndHandsOffLateOwner` |
| R11-F34 orphan pre-invocation | invocation API owns the fresh permit; confirmed handoff retains obligation | `TestR11OrphanPreInvocationSeamRejectsPostLatchStop`, corrected `TestR8W4ConfirmationAtOrphanPreInvocationSeam` |
| R11-F35 deferred activation `Stop` | completion extracts one-shot producer but invokes only through a fresh permit | `TestR11DeferredActivationStopRequiresFreshPermit` plus corrected R5/R6 schedules and ordinary exact-once subtest |
| R12-F36 lifecycle successor | `runCapturePrepareCommit` separately gates all post-helper ledger mutations | late-prepare tests assert operation ID zero and `captureGenerationPrepareInFlight` after latch |
| stale waiter dequeue | every command dequeue obtains a fresh permit | `TestR11EachWaiterDequeueRequiresFreshPermit` |

## Verification on final producer bytes

Host: Darwin x86_64, Go 1.26.0 darwin/amd64.

- Focused lifecycle matrix x100: PASS.
- Focused lifecycle race matrix x50: PASS.
- `go test ./...`: PASS.
- `go test -race ./...`: PASS.
- `go vet ./...`: PASS.
- Windows amd64 and arm64: vet, all-package build, probe build, probe test
  compile, and winprobe test compile all PASS.
- `file` identified all six cross outputs as PE32+ x86-64/Aarch64 as expected.
- Privacy x50 and artifact/recovery/manifest/sandbox/helper/privacy x10: PASS.
- Both manifest XML files: `xmllint --noout` PASS.
- Rev16 consistency: PASS, zero normative anti-patterns.
- Gofmt inventory, trailing whitespace, diagram delimiters, byte identity,
  confirmed-branch assertions, and `git diff --check`: PASS.
- `task-board validate`: PASS.

## Honest residual gates

This macOS host has no `pwsh`, MSVC, `makeappx.exe`, `signtool.exe`, or
PlantUML renderer. Native helper execution, signed-MSIX packaging/install,
WACK, installed AppContainer behavior, and real `WM_ENDSESSION`/lock/suspend/
permission/recovery evidence on Windows 10 and Windows 11 remain downstream
hardware gates in `TASK-260712-13rbnw` and `TASK-260712-1vtwkl`. They are not
claimed here and do not substitute for review of this portable lifecycle
boundary.

Producer completion is not task acceptance. A new frozen same-executor
adversarial review and root audit are required next.
