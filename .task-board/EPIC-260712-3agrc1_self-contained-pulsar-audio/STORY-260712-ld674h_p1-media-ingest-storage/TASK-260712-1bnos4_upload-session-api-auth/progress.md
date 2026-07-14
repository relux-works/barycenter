## Status
development

## Assigned To
codex-inline

## Created
2026-07-12T15:28:12Z

## Last Update
2026-07-14T02:09:10Z

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
Strict inline execution started 2026-07-14 from merged main 31bbeb9257b2555c86858c4087521466b58d673a on branch task/task-260712-1bnos4-upload-session-api-auth. TASK-260712-z6h6wh is landed; this task remains engineering-only with any real-app acceptance routed to EPIC-260714-th54l3.
Implementation complete pending commit/hosted CI: authenticated POST plus scoped PUT, HMAC replay without plaintext, atomic rate/concurrency/daily/hard-byte quotas, CAS offsets, staged fsync/crash-tail reconciliation, expiry and durable temp cleanup. Local go vet/test, full go test -race, 20x concurrency stress, exact 31bbeb9 predecessor rollback, pulsar-win vet/test/cross-build and board validation pass. Local Swift remains environment-limited because CommandLineTools lacks the Testing module; hosted macOS CI is required.

## Precondition Resources
(none)

## Outcome Resources
- [p1-media-ingest-sequence.puml](file://TASK-260712-1bnos4/p1-media-ingest-sequence.puml) — Upload session creation, resume and finalize flow
- [p1-media-upload-session-contract.md](file://TASK-260712-1bnos4/p1-media-upload-session-contract.md) — Authenticated resumable upload HTTP, quota, durability, cleanup and rollback contract
