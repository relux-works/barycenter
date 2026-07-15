## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:14:56Z

## Last Update
2026-07-15T13:49:46Z

## Blocked By
- TASK-260712-17yizc
- TASK-260712-3n36ny
- TASK-260712-2vhf80
- TASK-260712-1c1ska
- TASK-260712-2qpp6w

## Blocks
- TASK-260712-2i3u7v
- TASK-260712-31zja2
- TASK-260712-3nq0tq
- TASK-260712-2zdetx
- TASK-260712-1c34fe
- TASK-260712-2h6snp

## Checklist
- [x] Persist per Air invite, overlay, queue, and replace permissions and defaults
- [x] Enforce policy checks across lifecycle, overlay, queue, and replace actions
- [x] Keep local DND and block stricter than Air policy at all entry points
- [x] Cover unauthorized, allowed, restart persistence, and migrated pair defaults
- [x] Version and audit policy mutations and prevent retroactive target expansion
- [x] Enforce the same policy through app, Telegram and legacy aliases below local controls

## Notes
Strict inline execution started from synchronized main 7a4a2a4 after accepted Air control-plane tracking merge. Mapping frozen policy decisions onto lifecycle and immutable transmission acceptance before implementation.
Accepted engineering commit 7a3e31f via PR #90; merged to main as aa40b50. Hosted CI run 29420598338 passed coordinator, node-core, pulsar-win and packaged probe. Local evidence: full go test ./..., go vet ./..., targeted race, exact previous-head rollback. Immutable Air policy snapshots, migrated FIFO continuity, local DND precedence and app/Telegram/legacy gates are covered.

## Precondition Resources
- [p2-air-rooms-components.puml](file://TASK-260712-25862f/p2-air-rooms-components.puml) — Policy service and enforcement boundaries inside the coordinator

## Outcome Resources
- [p2-air-policy-enforcement.md](file://TASK-260712-25862f/p2-air-policy-enforcement.md) — Implementation, invariants, transport mapping and verification handoff
