## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:45:05Z

## Last Update
2026-07-15T05:56:37Z

## Blocked By
- (none)

## Blocks
- TASK-260712-1xik11

## Checklist
- [x] Inventory current gaps in docs/acceptance-run.md, Windows evidence, clean install media, migration fixtures and rollback coverage.
- [x] Define a repeatable artifact layout and command set for automated suites, live logs, screenshots and Store submission evidence.
- [x] Add or update migration and rollback rehearsal inputs so new report, block and media rows can be proven additive and reversible.
- [x] Document any remaining external prerequisites separately from internal setup debt so blockers stay concrete.
- [x] Pin the Swift toolchain and repair the current missing Testing module baseline
- [x] Define WACK, p95 sample counts, clock sync, maximum-memory and build-provenance evidence
- [x] Rehearse previous-version rollback using nonproduction fixtures

## Notes
2026-07-15 strict inline kickoff from synchronized main 5be6f15 after PR #62. Engineering scope is the reproducible repository harness, pinned toolchains, deterministic fixtures, provenance, sanitized artifacts and nonproduction migration/rollback rehearsal. Actual Windows 10/11 app operation, WACK UI execution, physical hardware/audio, screenshots and Partner Center observations remain manual evidence in EPIC-260714-th54l3; this task may prepare commands and fail-closed templates but will not claim those runs.
2026-07-15 engineering acceptance: exact code head 6fc647989b36779c765a31fd78eef5b44947c61a. Frozen Go 1.25.0, Xcode 26.2 build 17C52, Swift 6.2.3 and explicit hosted runner images; added a mode-0700 sanitized provenance runner, deterministic fresh two-Pulsar nonproduction fixture with private mode-0600 credentials, exact previous-head rollback rehearsal, WACK package-identity wrapper, fail-closed p95/memory calculator, templates and topology documentation. Clean local all-suite manifest is pass with dirty=false and 12/12 commands; Swift ran 211 tests. Hosted run 29391844793 passed coordinator, node-core, pulsar-win and signed packaged-probe jobs on exact head; PR #63 is ready. Actual Windows 10/11 real-app use, WACK execution/review, audible output, physical hardware, screenshots and Partner Center remain manual-required in EPIC-260714-th54l3 and are not claimed.
2026-07-15 tracking CI run 29392093503 revoked provisional acceptance: the new gate exposed a race in fakeWindowsSelfTestCapture cancellation and a zero-busy-timeout SQLite inspection connection in TestTelegramLinkRateLimitAuditFailureUsesGenericAdapterResponse. Merge is stopped; task returned to development until both are fixed and exact-head/tracking hosted runs are green. Hosted manifests also report post-run dirty=true, so the harness will record sanitized dirty paths and enforce end-clean provenance to identify and eliminate that drift.
2026-07-15 final replacement acceptance: exact branch head f8ae90328bc1448f018bb6c8727c2680bd49e063 passed a clean local 12-stage run with startDirty=false and endDirty=false. The concurrency fixes passed 20 targeted repetitions. Hosted run 29392625265 then passed all four jobs; coordinator, Swift and Windows uploaded manifests all report pass with empty endDirtyPaths on synthetic merge head 20e06c34b3c0c2be6f4d283225dddefe83c12fd4. The earlier failures were retained in history as evidence that the gate fails closed. PR #63 is mergeable. Manual boundary is unchanged.

## Precondition Resources
(none)

## Outcome Resources
- [phase1-acceptance-environment.md](file://TASK-260712-1cdoxh/phase1-acceptance-environment.md) — Pinned environment, topology, WACK, metrics, artifact and manual boundary runbook
- [phase1-toolchains.json](file://TASK-260712-1cdoxh/phase1-toolchains.json) — Machine-readable Go, Xcode, Swift, runner and WACK contract
- [engineering-f8ae903-manifest.json](file://TASK-260712-1cdoxh/engineering-f8ae903-manifest.json) — Clean exact-head automated acceptance provenance: 12/12 pass, start/end dirty=false
