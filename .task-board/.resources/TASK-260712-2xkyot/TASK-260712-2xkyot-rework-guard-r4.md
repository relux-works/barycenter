# TASK-260712-2xkyot — mandatory Telegram durable-audit R4 guard

Date: 2026-07-13
Owner: root orchestrator

The R2 Telegram implementation remains accepted only provisionally and the R3
review's single blocker is real. This run is deliberately narrow and starts
only after `TASK-260712-m5264f` freezes the shared durable audit repository.
Preserve the dirty worktree; do not commit, push, reset, clean, or reformat
unrelated files.

## Required production correction

Keep the exact rolling limiter and its ordering unchanged: feature/chat/syntax
validation first, then atomically reserve the Telegram-user attempt, before any
link-code lookup or `BEGIN IMMEDIATE`. When that reservation is rejected:

1. Call the shared error-returning `Store.RecordRateLimitAudit` with
   `RateLimitTelegramLinkConsumeTelegram`, the exact decimal limiter subject,
   and an empty pre-identity scope. Never call `Store.LogEvent` and never store
   the raw Telegram user ID, link code, code hash, display name, chat ID, bot
   token, URL, or generic JSON.
2. Return `ErrTelegramLinkRateLimited` only after the durable insert succeeds.
   If persistence fails, return the structural persistence error (wrapped only
   with a constant message), so the bot emits its ordinary generic failure and
   not the rate-limit response. Do not include the subject/user ID in errors or
   logs. The already-reserved attempt remains consumed.
3. Do not edit the shared onboarding/schema/audit implementation in this run.
   If its frozen API is insufficient, stop and return to root instead of making
   a competing schema change.

## Mandatory executable evidence

- Cross the exact N+1 rolling boundary and assert one typed
  `security.rate_limited` row with class
  `telegram-link-consume/telegram-user`, NULL actor/orbit scope, the exact
  class-domain-separated SHA-256 digest, and a sane timestamp; assert no legacy
  `events` rate-limit row and no raw Telegram ID anywhere in the durable audit.
- Inject a `BEFORE INSERT` failure on `rate_limit_audit_events`. Prove the
  rejected attempt returns neither `ErrTelegramLinkRateLimited` nor a
  credential/conflict sentinel, creates no audit row, consumes the attempt, and
  leaks neither raw ID nor code through returned errors or bot logs/messages.
  Remove the failure and prove the next attempt remains rejected, now writes
  exactly one durable row, and maps to the normal rate-limit bot response.
- Preserve invalid/private-chat/feature-off ordering, exact rolling-window and
  bounded-LRU tests, uniform credential paths, both deterministic second-writer
  barriers, one-winner conflict semantics, ActorContext authorization, legacy
  dual-write/reconciliation, previous-head compatibility, and Telegram HTTP
  redaction.
- Add a production adapter test for persistence failure versus rate-limit
  sentinel mapping; store-only assertions are insufficient.

Run focused repetitions, focused race, full uncached coordinator, full race,
previous-head, vet including previous-head, build, gofmt, diff/secret/URL scans,
exact frozen onboarding and Telegram boundary hashes, and `task-board validate`.
Attach `TASK-260712-2xkyot_rework-r4-results.md` with exact final hashes and
command results, then leave the task `to-review`. Fresh independent review and
root line-by-line/hash/test audit remain mandatory; producer claims are not
acceptance.
