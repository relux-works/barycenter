## Status
to-review

## Assigned To
codex-inline-reviewer

## Created
2026-07-12T16:23:01Z

## Last Update
2026-07-16T14:18:01Z

## Blocked By
- TASK-260712-14rxuk
- TASK-260712-2ogntd
- TASK-260712-3nq0tq
- TASK-260712-1vklop

## Blocks
- TASK-260712-2bdi4a
- TASK-260712-21kz3b
- TASK-260712-3u5cdn
- TASK-260712-3qybi2
- TASK-260712-2pnc5a
- TASK-260712-1kfnpu

## Checklist
- [x] Expose section 17 metrics for stream readiness, seek, skew, queue depth, and offline reasons
- [x] Add storage and egress accounting with quota and alert visibility
- [x] Fail health or readiness when mandatory phase-two processing or storage is degraded
- [x] Document operator scrape or query recipes used by acceptance and beta tasks
- [x] Consume canonical accounting instead of implementing duplicate quota counters
- [x] Keep flag-off Phase 1 readiness healthy and metric labels privacy-safe and bounded

## Notes
2026-07-16 strict-sequence start from synchronized main merge 70073dbe9fd3f0668d61a4ddb1e8cc23e09c9b1d after TASK-260712-n11rg6 technical review. Executing inline outside task-board spawn workflow per owner instruction. Manual/production evidence remains routed separately; this task implements repository observability and quota views best effort.
2026-07-16 implementation complete for review: authenticated no-store GET /v1/moderation/phase2-observability aggregates canonical stream accounting plus processing, playback, target/inbox and Air state with a rolling 24h window and fixed privacy-safe dimensions. Public /healthz uses a lightweight snapshot; enabled stream processor/storage/accounting and authoritative Air integrity fail readiness, while flags off preserve Phase 1. Contract, alert/query/retention runbook and fail-closed source anchors added. Air onboarding route delta reviewed and exact hash refreshed. Evidence: observability contract validator pass; Air validator pass; acceptance contracts 85/85; go vet pass; focused Go and 100x repetitions pass; focused race pass. Full local go test ./... reaches all packages but two existing OGG fixture cases cannot run because workstation ffmpeg lacks libvorbis encoder; hosted CI is authoritative. Client seek/buffer/audible, hardware, rollout and beta evidence remain in EPIC-260714-th54l3.

## Precondition Resources
- [p2-acceptance-evidence-map.puml](file://TASK-260712-qi81vf/p2-acceptance-evidence-map.puml) — Phase-two metrics and evidence consumers to instrument

## Outcome Resources
(none)
