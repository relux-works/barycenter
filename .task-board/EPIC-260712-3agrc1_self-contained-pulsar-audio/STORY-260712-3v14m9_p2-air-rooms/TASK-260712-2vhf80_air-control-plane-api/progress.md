## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:14:55Z

## Last Update
2026-07-15T13:18:11Z

## Blocked By
- TASK-260712-17yizc
- TASK-260712-3n36ny
- TASK-260712-kr64r2
- TASK-260712-m5264f

## Blocks
- TASK-260712-25862f
- TASK-260712-2bjdlb
- TASK-260712-2i3u7v
- TASK-260712-31zja2
- TASK-260712-3nq0tq
- TASK-260712-2zdetx
- TASK-260712-1c34fe
- TASK-260712-2h6snp

## Checklist
- [x] Implement create, invite, join, confirm, leave, and dissolve services and handlers
- [x] Expose current Air, pending membership, and lifecycle read models for Pulsar consumers
- [x] Enforce one active Air and capacity rules with stable user errors
- [x] Drive activation, parking, and dissolve through the Air runtime resolver
- [x] Use hashed single-use invites, opaque references, role checks, rate limits and audit
- [x] Test concurrent consume, active switch and lifecycle idempotency transactionally

## Notes
Strict inline execution started from synchronized main b54e4aa after accepted Air runtime resolution. Freezing the existing ActorContext, Air repositories and HTTP surface before implementing the transactional lifecycle control plane.
Accepted on main 69f32e2. Engineering commit efa02ac merged via PR #88 after hosted run 29418360729 passed coordinator, node-core, pulsar-win, and packaged-probe. Clean pinned coordinator acceptance passed 5/5; full Go suite, targeted race, vet, restart, concurrent consume, governance, capacity, stable-error, secret-redaction, and synchronous runtime-barrier coverage passed.

## Precondition Resources
- [p2-air-rooms-lifecycle-sequence.puml](file://TASK-260712-2vhf80/p2-air-rooms-lifecycle-sequence.puml) — Lifecycle service flow for Air API and control-plane handlers

## Outcome Resources
- [p2-air-control-plane-api.md](file://TASK-260712-2vhf80/p2-air-control-plane-api.md) — Accepted Air control-plane implementation, security boundary, runtime barrier, and downstream handoff
