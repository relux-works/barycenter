# Phase 3 migration and recovery technical pre-review

- Date: 2026-07-17
- Original task: `TASK-260712-6mz9xg`
- Root-reviewed source: `d94f51644a3acf37601b4a869b4247380372f9ec`
- Root-reviewed tree: `4e4cca878db806650eda6f1e1642051b87a18b93`
- Engineering reviewer: `codex-inline-pre-reviewer`
- Independent approval: `TASK-260717-1sgb5r`, owner Ivan Oparin

## Decision

The repository technical pre-review is complete. No new Critical or High
migration, rollback, recovery, feature-kill or false-recovery finding was found
in the frozen source. Reversible strict-sequence engineering may move to
`TASK-260712-3b7bp4`.

This is deliberately **not** an independent review, a destructive production
restore, a signed real-app rollout/rollback drill or implemented E2EE recovery.
All four Phase 3 capabilities, beta entry and promotion remain blocked. The
fail-closed machine record is
`acceptance/phase3/migration-recovery-technical-pre-review-v1.json`.

## Reviewed seams

| Seam | Repository conclusion |
| --- | --- |
| Additive migration | Legacy, identity, Air, media, transmission, moderation and automation schemas use transactional/retryable migration boundaries. Partial or injected failure keeps the prior authority readable. |
| Exact predecessor | The pinned predecessor matrix preserves current additive rows while proving the legacy authority surface. Identity rollback projection rejects incompatible YAML before opening the database and is idempotent. |
| Unsafe rollback | Divergent Air state enters `rollback_hold`; it never resurrects stale links or warms a dual runtime. |
| Identity recovery | Recovery is actor/origin/generation scoped, replay-safe and audited. Promotion preserves the node credential, never restores plaintext secrets and cannot turn a satellite into primary authority. |
| Client recovery | Pending candidates survive crash boundaries, require exact context and readback, serialize per scope and retain explicit destructive-abandon semantics. Clipboard/file exports remain explicit and bounded. |
| Capture kill | `DUET_LIVE_PTT` defaults off. Disabled clients reject incoming live work before capture; stall, policy revoke, sleep, lock, disconnect, rollback and quit release capture/routing state. |
| Automation kill | Feature disable, emergency disable, schedule disable, principal revoke and issuer revoke cancel canonical pending work and remain idempotent. |
| E2EE recovery | Fork, transfer and irreversible key-loss rules are validated only as a fail-closed contract. There is no production implementation or recovery claim; `e2ee_media` stays disabled. |
| Backup/restore | Litestream restore is manual, uses `-if-db-not-exists`, restores SQLite only and requires current migrations plus media retention reconciliation before availability. Provider snapshots and a real throwaway restore are not repository-proved. |
| Beta boundary | The seven-day beta cannot begin until independent review and signed rollout/recovery evidence bind the same build. Any affected code, schema, flag, config or fixture delta resets the gate. |

## Reproduced evidence

```text
(cd coordinator && go test -race -count=10 ./internal/store -run '<sixteen frozen migration/recovery tests>')
(cd coordinator && go test -race -count=10 ./cmd/duet-coordinator -run '<seven frozen rollback/recovery/feature-off tests>')
(cd coordinator && go test -tags previoushead -count=10 ./internal/store -run '<twelve exact predecessor tests>')
(cd pulsar-win && go test -race -count=10 ./... -run '<eighteen migration/recovery/live-disable tests>')
DEVELOPER_DIR=/Applications/Xcode.app/Contents/Developer xcrun swift test --package-path node-app --filter 'RecoveryService|RecoveryExport|E2EEAuditContract|MacLivePTTNode|CaptureMediaLifecycle'
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest -v <Phase 3 and E2EE fail-closed contract tests>
```

The coordinator migration/recovery group passed race times ten in 419.239
seconds. The command/feature-kill group passed race times ten in 93.612 seconds,
and the twelve-test exact-predecessor matrix passed ten repetitions in 483.746
seconds. Windows passed four packages under race and ten repetitions. macOS
passed 49 tests in five suites. The Phase 3/E2EE contract group passed 35 tests
and its validators explicitly reported production E2EE disabled.

## Evidence boundary and external closure

Repository evidence covers copied-state fault injection, exact predecessor
compatibility, identity recovery generations, feature-off behavior and client
pending-recovery state machines. It cannot prove provider credentials or
snapshots, a destructive restore, signed mixed-fleet application behavior,
audible capture stop, physical automation stop, or an E2EE path that has not
been implemented.

Manual rollout/rollback/recovery evidence remains `TASK-260712-30xwu2` under
`EPIC-260714-th54l3`. Deferred E2EE remains under `EPIC-260716-3qsztl`. Ivan
Oparin must select an implementation-independent reviewer and close
`TASK-260717-1sgb5r` against the exact root-reviewed commit and signed drill
artifacts. Any Critical or High finding must be fixed and independently
re-reviewed. Until then `NF-migration-recovery-review`, `NF-rollout-recovery`,
beta and Phase 3 promotion remain blocked.
