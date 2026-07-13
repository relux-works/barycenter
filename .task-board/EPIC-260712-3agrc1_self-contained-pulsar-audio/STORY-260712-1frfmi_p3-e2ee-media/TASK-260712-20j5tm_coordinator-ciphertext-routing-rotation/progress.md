## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T16:40:33Z

## Last Update
2026-07-12T16:54:24Z

## Blocked By
- TASK-260712-2ys1ww
- TASK-260712-3w1cst
- TASK-260712-aniuyy

## Blocks
- TASK-260712-2i0w6x
- TASK-260712-2nppt6
- TASK-260712-2q4jbu
- TASK-260712-1bcpda
- TASK-260712-1yz5ca
- TASK-260712-25dzp4
- TASK-260712-1x9ruo
- TASK-260712-1rziyo

## Checklist
- [ ] Replace plaintext protected-media routing with manifest and ciphertext handling behind the feature flag.
- [ ] Seal recipient snapshots to epochs and rotate on join, leave, revoke, and Air changes.
- [ ] Reuse ACL, delete, retention, and inbox or history services without plaintext regressions.
- [ ] Reject unsupported or stale-epoch fetches explicitly in mixed-version tests.
- [ ] Keep feature-off legacy behavior intact.

## Notes
Root-reviewed correction: rotate means serialize and route client-produced signed group commits. Coordinator must never create unwrap escrow or log group or content secrets. Original checklist wording is subordinate to this invariant.

## Precondition Resources
- [p3-e2ee-media-components.puml](file://TASK-260712-20j5tm/p3-e2ee-media-components.puml) — Coordinator ciphertext-routing and rotation boundary diagram
- [p3-e2ee-media-sequence.puml](file://TASK-260712-20j5tm/p3-e2ee-media-sequence.puml) — Protected-send and revoke-rotation sequence for coordinator runtime

## Outcome Resources
(none)
