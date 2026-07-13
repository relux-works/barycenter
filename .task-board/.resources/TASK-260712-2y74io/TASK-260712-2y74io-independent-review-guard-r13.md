# TASK-260712-2y74io — frozen lifecycle review guard R13

Date: 2026-07-14 (Asia/Tbilisi)
Base: `3565c1e1ca0511168026ec2ba72440d23fb1317f`

## Review mode

Review is performed inline by the same strict-sequential executor at the
user's direction; independence is not claimed and no task-board spawn workflow
is used. Production, existing tests, and documentation are frozen until the
verdict. Adversarial additions may exist only in a fresh `/tmp` copy. Planning,
board progress, this guard, and the eventual review report are outside the
implementation boundary.

## Required predecessor resources

```text
279e2b4d8b411a9db28282effac4e620e8d9f9caeab43977f45b541b1affb80c  TASK-260712-2y74io-independent-review-guard-r11b.md
2ceb0b6497d35cb4c0995a4201953cba75fbc3d941ceb5286b4a7373c1592314  TASK-260712-2y74io_independent-review-r11.md
62679ca73e8853b2c2902fd0eda940f2bf3cb0a6d833601e55f72f4d273fa4a9  TASK-260712-2y74io_rework-r12-results.md
```

## Frozen implementation boundary

Abort and report a boundary violation if any hash changes before the verdict:

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

## Mandatory falsification

1. Re-run every R10 operation/successor schedule and directly reproduce each
   R11 F33-F35 seam against the repaired production coordinators.
2. Block prepare through confirmed latch and prove zero Stop/helper, active or
   orphan publication, lifecycle result commit, result evidence, owner state,
   activation, release, UI, and settlement successors. The only retained
   ledger fact may be the pre-latch `prepare-in-flight` transition.
3. Race exact owner publication on both sides of gate closure: confirmation
   must either see and pre-latch-stop the published exact owner or publication
   must remove/abandon it without a late helper call.
4. Publish an orphan producer, confirm immediately before invocation, and prove
   the invocation API itself rejects the call; ordinary open-gate Stop/query/
   terminal/Release ordering must remain exact once.
5. Confirm while activation owns the native interval and immediately before
   deferred Stop admission. The activation may finish, but the distinct Stop
   must not begin after latch. The ordinary open-gate case must still invoke
   exactly once.
6. Confirm between every command dequeue, UI transition claim/post/finish,
   invalidated-permission mutation/cancel, structural escalation, evidence
   failure, helper callback, artifact operation, owner clear, lifecycle
   mutation, log/evidence/UI callback, and successor. No stale outer permit may
   authorize a later operation.
7. Verify confirmed wndproc remains lock-free/nonblocking and the bounded
   no-sync append remains the sole post-latch callback. Falsify graceful and
   cancelled lifecycle paths, repeated generation/operation-ID reuse, retries,
   permission/picker/artifact ownership, failure propagation, and privacy.
8. Compare diagrams byte-for-byte and to production semantics. Re-run focused
   high-count/race, full/race/vet, Windows amd64/arm64 cross, manifest/privacy/
   Rev16/static/format/diff/board checks. Report unavailable signed-Windows,
   MSIX, WACK, and hardware gates honestly.

The review report must enumerate attempted schedules and use exactly one
verdict: `ACCEPT FOR ROOT AUDIT` or `BACK TO DEVELOPMENT`. Acceptance remains
provisional until a separate root full-file/hash/diff/test audit.
