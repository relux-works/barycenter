## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:12:52Z

## Last Update
2026-07-16T10:15:18Z

## Blocked By
- TASK-260712-2j5fkr
- TASK-260712-2zoy4u
- TASK-260712-2ctf3x
- TASK-260712-3dmllz
- TASK-260712-3e4p0c
- TASK-260712-2zdetx
- TASK-260712-285pag
- TASK-260712-2h6snp

## Blocks
- TASK-260712-3u5cdn

## Checklist
- [x] Map voice audio and document actions to the common transmission service
- [x] Expose Air selection explicit targets and queue replace controls
- [x] Preserve legacy defaults only through the shared contract
- [x] Render human readable rights capability and unsupported target errors
- [x] Reuse secure actor-bound callbacks and immediate legacy voice defaults
- [x] Prove Air and explicit target selection cannot address foreign recipients

## Notes
2026-07-16 strict-sequence start from synchronized main merge 6069948 after TASK-260712-3aj8w2 exact head e6f0685 and hosted run 29487762262 passed 4/4. Implementing Telegram explicit-target and Air parity inline outside task-board spawn workflow; no real Telegram client, real recipient or hardware result will be claimed.
2026-07-16 implementation checkpoint: common Telegram target picker now issues actor-bound opaque Barycenter/Pulsar references; additive fail-closed Phase 2 callback bindings carry explicit audience, include-origin and future queue/replace intent; audio/document require current policy plus per-upload rights acknowledgement. Focused bot/store/presentation/coordinator suites pass. Targeted tracks remain honestly unsupported under the accepted production codec no-go; real Telegram and physical-recipient evidence remains manual and unclaimed.
2026-07-16 accepted on exact engineering head 3a822a1766b80cd0e0f3d67f68b4be3686037af7 through PR #160, merge 35f5fd4d13267199f3383ee437f5f1fe77bace36, after hosted run 29489910594 passed coordinator, node-core, pulsar-win and signed packaged-probe 4/4. Telegram now uses common opaque Barycenter/Pulsar target capabilities, immutable N-target snapshots, current-Air and include-origin policy; Phase 2 callbacks are actor/chat/message/media-generation bound and old-binary rollback fails closed. Audio/document require current versioned policy plus per-upload rights acknowledgement. Targeted queue/replace remains honestly unsupported under the accepted production codec no-go and cannot fall back to clip or transport-owned queue logic. Focused/full coordinator, race, vet and B5-B7 validators passed. Real Telegram, recipient, packaged-app and audible evidence remains unclaimed in manual TASK-260712-3u5cdn.

## Precondition Resources
(none)

## Outcome Resources
- [p2-targets-inbox-parity-sequence.puml](file://TASK-260712-wt2n7m/p2-targets-inbox-parity-sequence.puml) — Telegram parity flow through the common transport neutral service
- [p2-telegram-explicit-target-parity.md](file://TASK-260712-wt2n7m/p2-telegram-explicit-target-parity.md) — Common target picker, rights gate, fail-closed callback and manual-evidence boundary
