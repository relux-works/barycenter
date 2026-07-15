## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:24:06Z

## Last Update
2026-07-15T16:07:29Z

## Blocked By
- TASK-260712-2vhf80
- TASK-260712-25862f
- TASK-260712-2bjdlb
- TASK-260712-3dmllz

## Blocks
- TASK-260712-3nq0tq
- TASK-260712-wt2n7m

## Checklist
- [x] Reuse common Air services with actor-bound opaque callbacks and no raw ID or invite leakage
- [x] Test group-role, forged, stale, duplicate and concurrent Telegram actions

## Notes
Strict inline execution started from synchronized main 7516ce8 after accepted Windows Air tracking merge. Inspecting the Telegram actor-context adapter, opaque callback store and common Air service boundary before adding lifecycle parity; legacy approach/apart remain aliases over the same service.
Accepted inline after merge of PR #98 as 009fba2e9e3f93bd36614725da3702c76625ba1f. Common Air store now serves private Telegram /air list/create/invite/consume/confirm/activate/switch/park/leave/dissolve/withdraw/policy controls; legacy approach/apart remains on the same Air store and runtime reconciler. Opaque durable callbacks bind current ActorContext role, chat, message and revisions; forged, forwarded, expired, repeated, query-replayed and concurrent claims fail closed. Invite secret appears only in a private ordinary reply and is absent from callback data/prompt text/logs/durable mutation results; successful join deletes the source best effort. Local full coordinator test/vet/race passed, security and E2E tests passed 10x plus targeted race, repository automated gate passed 12/12 at .temp/acceptance/task-260712-2zdetx-final/manifest.json including exact previous-head rollback, and hosted run 29430796136 passed all four jobs. Real Telegram clients/apps/hardware remain manual-epic evidence and are not claimed.

## Precondition Resources
- [p2-air-rooms-components.puml](file://TASK-260712-2zdetx/p2-air-rooms-components.puml) — Air control-plane and transport boundaries
- [p2-air-rooms-lifecycle-sequence.puml](file://TASK-260712-2zdetx/p2-air-rooms-lifecycle-sequence.puml) — Air join, confirm, leave and park lifecycle

## Outcome Resources
- [p2-telegram-air-lifecycle-parity.md](file://TASK-260712-2zdetx/p2-telegram-air-lifecycle-parity.md) — Implementation, security contract, automated evidence and manual boundary
