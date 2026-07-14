## Status
done

## Assigned To
codex-inline

## Created
2026-07-12T15:40:21Z

## Last Update
2026-07-14T19:53:43Z

## Blocked By
- TASK-260712-3coble
- TASK-260712-2xkyot
- TASK-260712-2af2dp

## Blocks
- TASK-260712-21ers7
- TASK-260712-dlltnr
- TASK-260712-wt2n7m
- TASK-260712-2zdetx

## Checklist
- [x] Add typed transport events for callback queries, clip-eligible audio updates and document updates.
- [x] Implement safe callback-data encoding and validation without leaking raw identifiers.
- [x] Return honest user-facing errors for unsupported, over-limit and Phase-2-only attachment paths.
- [x] Treat all Telegram attachment metadata as hints and defer proof to common ingest
- [x] Test forged, expired, cross-actor and replayed opaque callbacks and terminal keyboard cleanup

## Notes
2026-07-14 kickoff: strict sequential inline execution started from synchronized main merge c820a86 after TASK-260712-1gx6mh acceptance. Scope is transport-only typed callback/audio/document updates, 36-byte opaque callback references, prompt acknowledgements, terminal keyboard cleanup, ActorContext binding and common-ingest handoff. Queue/routing business logic remains outside this task. No manual or hardware evidence is claimed.
2026-07-14 engineering gate: typed callback/audio/document transport, prompt callback queue, terminal keyboard cleanup, 36-byte opaque HMAC-indexed references, 15-minute expiry, 24-hour actor-bound query dedupe, strict ActorContext role/orbit/chat/message binding, common-ingest metadata trust boundary and stable unsupported/oversize/P2-only replies are implemented. Forgery, expiry, cross-actor/role/orbit/message, source-primary, replay, retry, HTTP form and redaction tests pass. Local coordinator vet/full/race, pinned previous-head suite, moderation check, Windows vet/tests/cross-build, Swift release build, board validation and diff checks are green. Local swift test remains environment-limited by the pre-existing unavailable Testing module; hosted macos-15 CI is authoritative. No real Telegram client or hardware evidence is claimed.
Exact engineering head 773b417eaff6308fe3cfa14cbe3d80e812854e75 passed all four hosted CI jobs in run 29362994920, including authoritative macOS Swift tests and the signed packaged Windows probe. Engineering acceptance is complete; physical/manual evidence remains unclaimed.
Tracking head c5bdf42abb51c4b5c23f66500b672cc3ad84771c passed all four hosted jobs in run 29363292374; PR #43 is ready to land.

## Precondition Resources
(none)

## Outcome Resources
- [p1-telegram-history-presence-components.puml](file://TASK-260712-3dmllz/p1-telegram-history-presence-components.puml) — Component diagram for Telegram transport boundaries
- [p1-telegram-history-presence-flows.puml](file://TASK-260712-3dmllz/p1-telegram-history-presence-flows.puml) — Sequence diagram for Telegram transport and callback handling
