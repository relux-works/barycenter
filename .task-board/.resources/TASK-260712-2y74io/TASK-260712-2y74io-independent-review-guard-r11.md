# TASK-260712-2y74io — independent lifecycle shutdown review R11

Date: 2026-07-13 (Asia/Tbilisi)

This is a review-only run over the frozen R10 producer bytes. R10 is not
accepted. Independently attempt to falsify the operation-admission model and
all claimed evidence. Do not edit production or existing test/docs sources,
commit, push, reset, checkout, clean, touch installed packages/hardware, or mark
the task done. Adversarial tests may be created only in a fresh `/tmp` copy.
The only workspace output is the required R11 report resource. Shared
`LOGBOOK.md` and unrelated dirty files are deliberately outside the frozen
boundary and must be preserved, not used as an abort condition.

Read in full before judging:

- R9 independent review `TASK-260712-2y74io_independent-review-r9.md`
  (SHA-256 `2585858f8b71313ee84bbf869140f70794a403a3ea6e91c7fa462226636463f4`);
- R10 repair contract `TASK-260712-2y74io-rework-guard-r10.md`
  (SHA-256 `2812e88196611ef5dc1b7792df12825a4c2620e2d9914d5ff7b0e5bcbaf56a83`);
- R10 producer handoff `TASK-260712-2y74io_rework-r10-results.md`
  (SHA-256 `3f78730a5be0a14e9168d0f73439dd854295a9a827ac21b56d2fa1dba15717fc`);
- the original task/Rev16 lifecycle contract, all prior accepted invariants,
  every frozen file below in full, and relevant helper/caller dependencies.

## Frozen R10 boundary

Abort and report a boundary violation if any hash differs before or after
review:

```text
0806f508eaf2df9ef95ea9d701af95d6c6f49965e1a9730339bf81dfd71dad05  pulsar-win/cmd/pulsar-win-probe/coordinators.go
51a0e03dbdc06e1f4fa26de761103f943be14ef411e81a2f4cdf1e3aece1639a  pulsar-win/cmd/pulsar-win-probe/main_windows.go
a78b3b91603c560b678520c695817723647c6210bfa3f6b386508b7ace3c233b  pulsar-win/cmd/pulsar-win-probe/lifecycle_r10_test.go
d8369ed22d334598cf967277e68a180d2ec678638a9476b6b164db473a74ca8d  pulsar-win/cmd/pulsar-win-probe/lifecycle_source_test.go
b0c0ab8b1e62e00f2c6179a536f08b1b744e5751c3f4e4fe1684c6c8a86ae47b  pulsar-win/probe-msix/README.md
01ffa997d7a7b9e8867d3cb5a15da662c4bbc937b6a428ff0a20be9e73bd8cd2  docs/diagrams/p1-windows-store-spike-lifecycle.puml
01ffa997d7a7b9e8867d3cb5a15da662c4bbc937b6a428ff0a20be9e73bd8cd2  .task-board/.resources/TASK-260712-2y74io/p1-windows-store-spike-lifecycle.puml
8931b7cecd3bdece1366655c89cdca5d9cb5cb8177d4dba0a0458dc544535d68  pulsar-win/cmd/pulsar-win-probe/lifecycle.go
97a65fab63958e2b84814312d8b06e6e1f81a91cc31a616c7d58ae27072580a1  pulsar-win/cmd/pulsar-win-probe/window_windows.go
e20b8341739d8744623643f1763e75afba54e424318b5f90ebe8f86040d5be56  pulsar-win/cmd/pulsar-win-probe/lifecycle_r6_test.go
7e58ada410164a5612539a0a3537dd4c712ec041128889914fcae2814ca5206e  pulsar-win/cmd/pulsar-win-probe/lifecycle_r8_test.go
```

## Mandatory falsification

1. Reproduce R9-F28 at production seams. For every query-failure cleanup,
   Stop/Finalize-or-Abort/Release must each acquire an independent atomic
   pre-close permit. Block each operation, publish confirmation at every gap,
   release it, and prove no not-yet-admitted successor starts.
2. Reproduce live root F30-F32: blocked required evidence cannot authorize
   escalation; no dispatch/helper/prepare/activation/log/enqueue/lifecycle/UI
   continuation may inherit a stale outer permit; an orphan Release returning
   after confirmation may return its native result but cannot clear ownership,
   settle lifecycle, emit evidence/log/UI, or start any successor.
3. Enumerate every `runOperation*`, direct helper/native callback, artifact
   callback, lifecycle mutation, evidence/log/enqueue/UI publication, state
   mutation, Release, retry, watchdog, and quit path in both production files.
   For each multi-step path, identify the exact linearization point of every
   operation and then try confirmation immediately before it. A pre-close
   permit never authorizes a later callback or state publication.
4. Audit check/use races, not only direct callbacks: every `isClosing` check,
   captured permit, delayed closure, goroutine, callback, waiter completion,
   orphan cleanup, and owner/generation transition must be unable to smuggle
   post-latch ordinary work. AST call counting is supporting evidence only;
   it cannot prove dynamic sequencing or complete semantic coverage.
5. Prove the only admitted confirmed-shutdown exception remains the already
   reviewed bounded no-sync append to the owned `.partial`. Confirm wndproc
   stays nonblocking and contains no lifecycle/global lock, Release, cleanup,
   sync, helper destroy, hotkey/tray/UI, evidence, or terminal PASS work.
6. Falsify graceful/cancelled paths after the refactor: exact-owner cleanup,
   Stop/Finalize/Release ordering, start-gate restore, waiter wake, watchdog,
   repeated generations/operation-ID reuse, permission/picker/artifact paths,
   and failure propagation must remain live and deterministic.
7. Compare both diagrams byte-for-byte and semantically to production. The
   confirmed branch must say only Stop admission, latch/wake, bounded append,
   then waiter/OS/next-launch recovery; no ordinary cleanup is implied.
8. Independently rerun focused high-count/barrier schedules, full/race tests,
   vet/build, both Windows architectures, formatting, manifest/privacy/
   consistency checks, diagram validation, diff check, and board validation.
   State unavailable native Windows/MSIX/hardware gates honestly.

Severity-rank every finding with exact file/line, concrete interleaving, and
missing or false-positive test. If no defect is found, document each attempted
gap schedule. Write exactly
`TASK-260712-2y74io_independent-review-r11.md`, attach it as an outcome resource,
and use exactly one verdict: `ACCEPT FOR ROOT AUDIT` or `BACK TO DEVELOPMENT`.
Do not mark the task done; root retains final acceptance.
