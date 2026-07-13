## Status
backlog

## Assigned To
(none)

## Created
2026-07-12T15:28:12Z

## Last Update
2026-07-12T16:19:32Z

## Blocked By
- TASK-260712-z6h6wh
- TASK-260712-m5264f

## Blocks
- TASK-260712-gj0cko
- TASK-260712-3huupe
- TASK-260712-1sae4q
- TASK-260712-2fe5bz
- TASK-260712-3dqc3l
- TASK-260712-285pag

## Checklist
- [ ] Implement session creation and scoped upload authorization endpoints
- [ ] Support monotonic resume offsets, idempotency keys and phase-one quota enforcement
- [ ] Cover retry, unauthorized access, malformed requests and concurrent limit behavior with tests
- [ ] Use expiring scoped tokens, actual-byte enforcement and concurrency-safe monotonic offsets
- [ ] Test restart, repeated finalize and abandoned-session cleanup

## Notes

## Precondition Resources
(none)

## Outcome Resources
- [p1-media-ingest-sequence.puml](file://TASK-260712-1bnos4/p1-media-ingest-sequence.puml) — Upload session creation, resume and finalize flow
