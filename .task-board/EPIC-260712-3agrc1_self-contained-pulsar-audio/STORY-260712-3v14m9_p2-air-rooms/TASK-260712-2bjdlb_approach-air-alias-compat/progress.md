## Status
done

## Assigned To
codex-inline-developer

## Created
2026-07-12T16:14:56Z

## Last Update
2026-07-15T14:15:44Z

## Blocked By
- TASK-260712-17yizc
- TASK-260712-3n36ny
- TASK-260712-kr64r2
- TASK-260712-2vhf80
- TASK-260712-2xkyot

## Blocks
- TASK-260712-2i3u7v
- TASK-260712-31zja2
- TASK-260712-3nq0tq
- TASK-260712-2zdetx

## Checklist
- [x] Route approach and accept and decline and apart through Air services instead of direct link mutation
- [x] Preserve current two party bot copy plus home and status wording while switching to Air ids
- [x] Keep apart local to the caller and prevent stale link resurrection after restart
- [x] Cover migrated alias flows and coordinator restart behavior
- [x] Prevent any alias or stale link from creating a second runtime after cutover

## Notes
Strict inline execution started from synchronized main 63aa321 after accepted Air policy enforcement tracking merge. Inspecting frozen approach-to-Air alias semantics and every app/Telegram compatibility entry point before implementation.
Accepted engineering commit d2af5aa via PR #92; merged to main as 095bf823. Hosted CI run 29422446508 passed coordinator, node-core, pulsar-win and signed packaged probe. Local evidence: coordinator go test ./... and go vet ./..., targeted Air alias race suite, exact previous-head Air rollback, plus pulsar-win go test/vet. Air-only alias mutations, joining-side confirmation, caller-local apart, duplicate suppression, migration restart, no raw IDs and rollback-hold stale-link guard are covered.

## Precondition Resources
- [p2-air-rooms-lifecycle-sequence.puml](file://TASK-260712-2bjdlb/p2-air-rooms-lifecycle-sequence.puml) — Alias flow showing how approach and apart map onto Air lifecycle operations

## Outcome Resources
- [p2-approach-air-alias-compat.md](file://TASK-260712-2bjdlb/p2-approach-air-alias-compat.md) — Implementation, authority boundary, compatibility semantics and verification handoff
