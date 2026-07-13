# TASK-260712-2y74io independent review R7

Review only. Do not edit production code, tests, docs, board status, or existing resources. The root has not accepted R6.

## Frozen input

Audit these exact SHA-256 inputs in full, not merely a git diff (the probe tree is untracked):

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

Read the frozen Rev16 ownership contract and all prior task preconditions/outcomes needed to understand F1-F24. Treat signed-Windows execution as an honest residual platform gate, never as host evidence.

## Mandatory adversarial review

1. Trace every owner state transition and every call site for prepare, publish/conflict, activation intent, native activation, Stop, terminal result, Release, clear, settlement, and abrupt confirmation.
2. Prove exact `(generation, operationID, owner pointer)` identity through all delayed callbacks, including operation-ID reuse.
3. Prove Stop/Activate/Release ordering for stop-first, activation-first, close-before/after admission, evidence failure, result-query failure, terminal-first, and confirmed shutdown.
4. Check that `pending` can only mean a result will still become observable, and that no pending path finalizes artifacts, releases native ownership, clears the owner, or fabricates `S_OK`.
5. Specifically try to falsify the root concern: `requestStop` may set `captureOwnerStopClaimed`, then `markReleased` may acquire `stopMu` first, set `captureOwnerReleased`, and then `requestStop` returns true without a stop result. Determine whether `requestCaptureStopOrReuse` can return an indefinitely pending outcome and whether any production schedule makes that safety/liveness relevant. Require a deterministic regression if substantive.
6. Examine successful helper returns with zero/invalid operation IDs and all rollback/suppressed-generation branches for leaks or stranded gates.
7. Inspect tests for self-fulfilling assertions, source-text-only coverage, missing production seams, nondeterminism, and false claims.
8. Re-run focused high-count/race tests, full host/race/vet, Windows amd64+arm64 build/test compilation, manifest/privacy/artifact/Rev16/static checks. Record exact commands and results.

## Output

Create exactly one outcome resource named `TASK-260712-2y74io_independent-review-r7.md`. Include verdict (`PASS` or `BACK TO DEVELOPMENT`), severity-ranked findings with exact file/line/schedule evidence, hash verification, commands/results, and residual Windows gates. Do not change task status or code. A PASS is not root acceptance.
