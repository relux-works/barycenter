# TASK-260712-2y74io — corrected landed-byte lifecycle review guard R11b

Date: 2026-07-14 (Asia/Tbilisi)
Base: `3565c1e1ca0511168026ec2ba72440d23fb1317f`

## Purpose and delta from R11

This guard supersedes only the frozen-file inventory in
`TASK-260712-2y74io-independent-review-guard-r11.md`. Its mandatory
falsification matrix, evidence requirements, and root-audit requirement remain
in force.

The original R11 inventory was frozen before CI commit
`cb214cfe7fec27cae56b2ded048feab28cd1ac98` (`make Windows source assertions
line-ending safe`). That commit changed one lifecycle test file from SHA-256
`d8369ed22d334598cf967277e68a180d2ec678638a9476b6b164db473a74ca8d`
to `86488aef41e385cb18ce0147d657282367b6353b78f34947fb4efc040e7c5424`
by normalizing CRLF to LF before source-text assertions. The other frozen
production, test, and documentation bytes are unchanged. The delta adds no
production behavior, removes no assertion after normalization, and is itself
part of this review boundary.

Review is performed inline by the same strict-sequential executor at the
user's direction; independence is not claimed. No task-board spawn workflow is
used. Tracking-only `.planning` and task progress edits are outside this frozen
boundary.

## Required predecessor resources

```text
2585858f8b71313ee84bbf869140f70794a403a3ea6e91c7fa462226636463f4  TASK-260712-2y74io_independent-review-r9.md
2812e88196611ef5dc1b7792df12825a4c2620e2d9914d5ff7b0e5bcbaf56a83  TASK-260712-2y74io-rework-guard-r10.md
3f78730a5be0a14e9168d0f73439dd854295a9a827ac21b56d2fa1dba15717fc  TASK-260712-2y74io_rework-r10-results.md
448cd299467296b16c03e6bb29c4b9622142747cc9381efa1fd640bfd28c8096  TASK-260712-2y74io-independent-review-guard-r11.md
```

## Frozen landed boundary

Abort substantive review and record a new boundary violation if any hash below
changes before the R11b verdict:

```text
0806f508eaf2df9ef95ea9d701af95d6c6f49965e1a9730339bf81dfd71dad05  pulsar-win/cmd/pulsar-win-probe/coordinators.go
51a0e03dbdc06e1f4fa26de761103f943be14ef411e81a2f4cdf1e3aece1639a  pulsar-win/cmd/pulsar-win-probe/main_windows.go
a78b3b91603c560b678520c695817723647c6210bfa3f6b386508b7ace3c233b  pulsar-win/cmd/pulsar-win-probe/lifecycle_r10_test.go
86488aef41e385cb18ce0147d657282367b6353b78f34947fb4efc040e7c5424  pulsar-win/cmd/pulsar-win-probe/lifecycle_source_test.go
b0c0ab8b1e62e00f2c6179a536f08b1b744e5751c3f4e4fe1684c6c8a86ae47b  pulsar-win/probe-msix/README.md
01ffa997d7a7b9e8867d3cb5a15da662c4bbc937b6a428ff0a20be9e73bd8cd2  docs/diagrams/p1-windows-store-spike-lifecycle.puml
01ffa997d7a7b9e8867d3cb5a15da662c4bbc937b6a428ff0a20be9e73bd8cd2  .task-board/.resources/TASK-260712-2y74io/p1-windows-store-spike-lifecycle.puml
8931b7cecd3bdece1366655c89cdca5d9cb5cb8177d4dba0a0458dc544535d68  pulsar-win/cmd/pulsar-win-probe/lifecycle.go
97a65fab63958e2b84814312d8b06e6e1f81a91cc31a616c7d58ae27072580a1  pulsar-win/cmd/pulsar-win-probe/window_windows.go
e20b8341739d8744623643f1763e75afba54e424318b5f90ebe8f86040d5be56  pulsar-win/cmd/pulsar-win-probe/lifecycle_r6_test.go
7e58ada410164a5612539a0a3537dd4c712ec041128889914fcae2814ca5206e  pulsar-win/cmd/pulsar-win-probe/lifecycle_r8_test.go
```

## Review contract

Read the R9 finding, R10 repair contract, R10 producer handoff, original R11
guard, task/Rev16 contract, every frozen file, and relevant callers/helpers in
full. Dynamically challenge every operation and successor gap described in R11:

- query-failure Stop/Finalize-or-Abort/Release operation-level admission;
- required-evidence escalation and stale outer-permit inheritance;
- orphan Release completion versus ownership/lifecycle/evidence successors;
- all delayed closures, goroutines, callbacks, retries, watchdogs and UI/log
  publication after confirmed shutdown;
- the sole bounded no-sync `.partial` append exception;
- graceful/cancelled exact-owner cleanup, repeated generations and reused IDs;
- byte-identical and semantically accurate diagrams;
- high-count, race, full, vet/build, amd64/arm64 Windows cross, manifest,
  privacy, consistency, formatting, diff and board gates.

Any defect is severity-ranked with an exact interleaving and missing regression,
then returned to development before production edits. If no defect is found,
the review report must enumerate the attempted gap schedules and use verdict
`ACCEPT FOR ROOT AUDIT`; root still owns final acceptance.
