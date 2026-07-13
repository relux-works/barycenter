## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T16:11:49Z

## Last Update
2026-07-12T16:22:53Z

## Blocked By
- TASK-260712-14u0yk

## Blocks
- TASK-260712-3vkcki
- TASK-260712-298tyq
- TASK-260712-1canzv
- TASK-260712-ibuaxj
- TASK-260712-2eympi
- TASK-260712-350u8d

## Checklist
- [ ] Compare original-upload versus canonical-variant storage for MP3 AAC and Opus fixtures
- [ ] Prototype stream_variants rows plus variant metadata and integrity fields
- [ ] Define the byte-range HTTP contract, auth behavior, and restart-safe cache semantics
- [ ] Name cache ceilings and eviction rules that stay bounded on two-hour media
- [ ] Publish fixture and contract details that every decoder prototype can consume unchanged
- [ ] Specify RFC range, conditional, target ACL and non-disclosing failure behavior
- [ ] Specify chunk integrity, VBR seek mapping and delete or disable cache invalidation

## Notes

## Precondition Resources
(none)

## Outcome Resources
- [p2-codec-player-spike-components.puml](file://TASK-260712-dqdoqj/p2-codec-player-spike-components.puml) — Component seam reference for stream variants and range contract work
