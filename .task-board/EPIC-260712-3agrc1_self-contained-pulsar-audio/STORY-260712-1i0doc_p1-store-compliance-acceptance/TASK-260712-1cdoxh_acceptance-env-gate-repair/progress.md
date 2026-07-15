## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:45:05Z

## Last Update
2026-07-15T05:39:59Z

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

## Precondition Resources
(none)

## Outcome Resources
- [engineering-6fc6479-manifest.json](file://TASK-260712-1cdoxh/engineering-6fc6479-manifest.json) — Clean exact-head automated acceptance provenance: 12/12 pass, dirty=false
- [phase1-acceptance-environment.md](file://TASK-260712-1cdoxh/phase1-acceptance-environment.md) — Pinned environment, topology, WACK, metrics, artifact and manual boundary runbook
- [phase1-toolchains.json](file://TASK-260712-1cdoxh/phase1-toolchains.json) — Machine-readable Go, Xcode, Swift, runner and WACK contract
