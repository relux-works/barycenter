# TASK-260712-2y74io — root audit R13

Date: 2026-07-14 (Asia/Tbilisi)
Base: `3565c1e1ca0511168026ec2ba72440d23fb1317f`
Executor: `codex-inline`

## Verdict

`ACCEPT`

The frozen R13 implementation satisfies the task's portable lifecycle,
ownership, evidence, cleanup, and confirmed-shutdown acceptance boundary. The
task may be marked done. Signed-MSIX and real Windows 10/11 hardware evidence
remain explicit downstream work and are not silently treated as completed.

## Root audit performed

- Reverified all 13 implementation/test/doc hashes from the R13 guard before
  and after review. Every hash matched.
- Read the final full production files, changed tests, README, diagrams,
  predecessor guards/outcomes, R11 rejection, R12 producer report, and R13
  review report; inspected the complete diff from landed base.
- Enumerated final direct helper exports, `CaptureRelease`, owner publication,
  Stop claim/invocation/completion, lifecycle commit, waiter dequeue, UI
  transition, evidence, watchdog, and resource-cleanup call sites.
- Confirmed the only ungated low-level orphan Stop method is
  `invokeClaimedStopAdmitted`, package-private and called only by
  `requestStop`, where the immediate caller owns either an ordinary operation
  permit or the special exact-owner pre-latch confirmed-shutdown authority.
  All separately delayed orphan invocation uses the gate-owning API.
- Confirmed every deferred activation call site passes the non-nil shutdown
  coordinator, and the distinct Stop callback cannot inherit activation's
  permit.
- Confirmed production `runCapturePrepareCommit` supplies the fresh completion
  gate, while publication, orphan producer publication, orphan invocation,
  lifecycle result commit, result evidence, and owner successor are separate
  operations.
- Confirmed the wndproc file is byte-identical to its previously accepted hash,
  uses no lifecycle/global mutex in confirmed shutdown, requests only the exact
  active owner Stop before latch/wake, and never performs Release, artifact
  cleanup, evidence, hotkey/tray cleanup, helper destruction, or PASS
  settlement.
- Confirmed the bounded no-sync append remains the sole post-latch callback and
  both diagrams are byte-identical and semantically match that branch.
- Inspected planning and task-board mutations: they contain only strict-order
  tracking, status/resource history, and the task-scoped evidence set.

## Final frozen hashes

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

Review controls:

```text
47dc5f0a92c9bd999fc9a9bf095891e48b54cf103bafa0fcfad642a4115d22d5  TASK-260712-2y74io-independent-review-guard-r13.md
7d025f2bbc46339523a3bfdacf1a0d06afa03cdcb2d5d79638d11031374b2ebe  TASK-260712-2y74io_independent-review-r13.md
```

## Fresh root verification

- Focused cumulative lifecycle matrix x100: PASS.
- Focused cumulative lifecycle race matrix x50: PASS.
- Full host tests and full race tests: PASS.
- Host vet: PASS.
- Windows amd64 and arm64: vet, all-package build, probe build, probe test
  compile, and winprobe test compile: PASS.
- Privacy x50 and artifact/recovery/manifest/sandbox/helper/privacy x10: PASS.
- Both manifest XML files, gofmt inventory, trailing whitespace, Rev16
  consistency (zero normative anti-patterns), diagram identity/delimiters,
  `git diff --check`, and `task-board validate`: PASS.
- R13 isolated review additionally passed its review-only tests, hundreds of
  thousands of publication/confirmation race schedules, cumulative x100/race
  x20, full/race/vet, both Windows cross matrices, privacy, manifest, and
  formatting checks.

## Residual downstream evidence

This acceptance is intentionally limited to the task's code/review boundary.
Native MSVC helper execution, signed-MSIX creation/install, WACK, and physical
Windows 10/11 microphone lifecycle evidence require unavailable Windows tools
and hardware. They remain mandatory in the following strict-order packaging
and evidence tasks; they do not invalidate or inflate this task's result.
