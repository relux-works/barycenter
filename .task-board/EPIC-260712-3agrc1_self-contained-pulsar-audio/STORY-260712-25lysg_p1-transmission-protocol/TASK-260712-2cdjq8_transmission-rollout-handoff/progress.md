## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:39:19Z

## Last Update
2026-07-14T13:02:03Z

## Blocked By
- TASK-260712-2qc27p

## Blocks
- TASK-260712-1xik11

## Checklist
- [x] Record the final request and receipt contract plus legacy downgrade rules
- [x] Document rollout order for additive migrations and mixed-version nodes
- [x] Hand off exact status, presence, DND, block, and receipt semantics to downstream stories

## Notes
Strict inline execution started 2026-07-14 from synchronized main merge 70f26072cb36f3ee6e5cd4358bdded2bf98b7214 (PR #27; exact-code CI 29333494719 and tracking CI 29333795623 green). Deliver one stable contract/diagram/evidence index, an honest coordinator-first capability/surface-gated rollout and drain-before-rollback procedure, and exact UI/Telegram/mixer/moderation/Store handoffs. No nonexistent transmission feature flag or manual/hardware result will be claimed.
Accepted exact documentation/code head cd234c913634db2fef5bbfcd866e8298e45f23cb after local coordinator vet/unit/race, Windows vet/unit/race and amd64/arm64 builds, Swift release build, document-link guard, diff check and board validation. Hosted CI 29334550550 passed coordinator, node-core, pulsar-win and signed packaged-probe. Canonical handoff records coordinator-first caller-surface-last mixed rollout, exact legacy whole-downgrade/interrupt challenge, downstream receipt/presence/DND/block semantics and zero-nonterminal drain-before-rollback. Real-app, audible, packaged-install and physical-hardware evidence remains unpassed in EPIC-260714-th54l3.

## Precondition Resources
- [p1-transmission-protocol-components.puml](file://TASK-260712-2cdjq8/p1-transmission-protocol-components.puml) — Documentation context for the final transmission handoff

## Outcome Resources
- [transmission-rollout-handoff-outcome.md](file://TASK-260712-2cdjq8/transmission-rollout-handoff-outcome.md) — Stable contract, rollout, rollback, downstream handoff and evidence index
