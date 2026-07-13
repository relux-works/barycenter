# TASK-260712-2y74io — independent lifecycle review R9

Review only. Do not edit product/test files, commit, push, or mark the task
`done`. The R8 producer outcome is untrusted evidence. Read every inventoried
file in full, recompute every SHA-256, and compare against the frozen Rev16
bridge, all prior accepted F1-F24 invariants, R8 F25-F27, and root W1-W4.

Frozen R8 source hashes:

- `coordinators.go` `1114a0a692b981eb46c2bd5508c3ccab050addf0bd4b2fc72fa2fd7832443a5a`
- `lifecycle.go` `8931b7cecd3bdece1366655c89cdca5d9cb5cb8177d4dba0a0458dc544535d68`
- `main_windows.go` `36f971e8c7877c0ab80d289d59dba09356faf5bc7551be239365f460bb2e3455`
- `lifecycle_r6_test.go` `e20b8341739d8744623643f1763e75afba54e424318b5f90ebe8f86040d5be56`
- `lifecycle_r8_test.go` `7e58ada410164a5612539a0a3537dd4c712ec041128889914fcae2814ca5206e`
- `lifecycle_source_test.go` `99a78a3fa1e120c645930fe67ed68d0db376f34c3855e4751afb0a172bcee749`
- `probe-msix/README.md` `1919528ea96be20d948b9aced81093bde152a7d1c16f23656650d3f8b2181327`
- producer outcome `692c4d2b1823dba9b6445e969bbeeafba9109f637aa3cb63516b851069319691`

`LOGBOOK.md` is concurrently shared and its producer hash has already changed;
review only the task block and do not treat unrelated churn as task evidence.
The untracked probe tree must be reviewed as full files, never as a git diff.

Independently try to falsify these boundaries with deterministic production-
seam schedules rather than implementation-mirroring tests:

1. The Stop-claimed bit before `stopMu` callback storage, immediate invocation,
   activation-deferred invocation, native-terminal observation, and the actual
   helper `CaptureRelease` interval. Every `pending` state must have exactly one
   live producer; no Stop may execute after successful Release.
2. Failed/unexpected Stop and Release HRESULTs, `E_ILLEGAL_METHOD_CALL` retry,
   terminal-first release, simultaneous release retries, and operation-ID reuse
   across different generation/owner pointers. Only exact `S_OK` transfers
   ownership; independent terminal evidence must wait for any admitted Stop
   result but need not pretend a failed Stop was terminal proof.
3. Every production capture Release call site, owner clear, artifact finalize,
   lifecycle settlement, and query-failure path. Look for any ownerless bypass,
   wrong-generation lookup, false terminal/released fact, or artifact loss.
4. A real successful prepare publication loser at every seam: before Stop claim,
   after claim/before callback storage, after storage/before orphan publication,
   after publication/before invocation, during Stop, terminal query, failed
   query/Release, retry, and exact orphan removal. Decide explicitly whether an
   ignored `publishOrphanStopProducer` failure is reachable on an open gate and
   whether closing-gate ownership is honestly OS-owned.
5. `WM_ENDSESSION` closing/confirmation at those seams. Confirmation must never
   wait for lifecycle/orphan locks or callbacks, must stop only the exact active
   owner before latch/wake, must admit no new orphan query/release/finalization,
   and may allow only callbacks already admitted by the frozen contract to
   finish. Challenge the production waiter admission point, not just a wrapper.
6. Same-generation duplicate phase rejection versus a distinct stale incumbent;
   zero-ID S_OK, unexpected success HRESULT, and first-query invalid handle.
   Prove the generation/start gate is settled without any native zero-ID call,
   while a real unregistered nonzero owner remains fail-closed and bounded.
7. Required structural evidence must be acknowledged before graceful escalation;
   sticky evidence failure and confirmed shutdown suppress successors without
   fabricating clean exit. Verify every new structural production path uses the
   gate, not merely the helper functions in isolation.
8. Preserve F1-F24: exact lifecycle generation replay, waiter/UI affinity,
   activation admission, artifact ownership, evidence suppression, hotkey/tray/
   helper teardown, and no post-latch ordinary I/O.

Run focused high-count and race schedules, full relevant host/race/vet, Windows
amd64+arm64 build/vet/test compilation, formatting, manifest/privacy/artifact/
Rev16 checks, and board validation. If shared module metadata is inconsistent,
use a hash-matching isolated copy without modifying sibling-owned files and
report both the live-tree failure and isolated evidence honestly.

Write exactly one report named
`TASK-260712-2y74io_independent-review-r9.md` with verdict, severity-ranked
findings and concrete schedules, full hash verification, exact commands/results,
and remaining signed-Windows gates. Leave task status `to-review`; root alone
accepts or returns it.
