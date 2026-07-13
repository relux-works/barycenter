# TASK-260712-2xkyot Telegram rework R6

The R4 producer result is rejected. Implement the narrow privacy correction from independent R5, preserve every accepted identity/rate-limit/audit invariant, and hand back a fresh outcome for independent and root review. Do not commit or push.

## Frozen starting boundary

- `identity_telegram.go` — `1d99a568881d5bc22b53166a9d76cd04d6bae10ef59a53c27d39d5b1dab72451`
- `identity_telegram_test.go` — `17b046bc1202f632f8082d7121ae40e835106de5ddd4ecf2fe14794887f07c4d`
- `identity_telegram_previous_head_test.go` — `efdec398578634c162f44b60e51254d82c49b12b4fe90ee6688fdf3b03ca963b`
- `loop.go` — `90940d1252d9a44b6174bb7482b8a71aed522c450022321c02003e3c3f6137c1`
- `telegram_identity_test.go` — `30aa12a2a08895e048e7aeb1b7f3830b83ba73d453de59056c79b04c380959fe`
- `bot.go` — `a94852e8dbed50e4faf5ae45569247022c09d6c2c04bab8696627d11f5033d8c`
- `bot_test.go` — `96718ef5a213327de1240985a4205a1c949f67dcf22cd32be7b230f8ea5260fb`
- accepted shared identity/schema/onboarding/audit hashes remain those recorded in the R5 review.
- independent rejection: `TASK-260712-2xkyot_security-review-r5.md`, SHA-256 `a9ddf5f63ea2a3751b27f7d5f4cbbb9b821cb17270ad41e05c9e57cb4fea1068`.

## Mandatory production correction

1. No Telegram transport/outbox failure log or error graph may contain a raw chat ID, Telegram user ID, source message ID, link/human code, bot token, request body, token-bearing URL, file ID, or destination path. This includes all four current paths: `SendTo` queue overflow, reply-send failure, secret-delete failure, and secret-delete queue overflow.
2. Prefer constant operation/action fields plus the already-sanitized HTTP adapter error. If correlation is truly required, use only a separately reviewed non-identifying mechanism; never reuse the limiter subject digest or expose a stable cross-purpose identifier casually.
3. Keep best-effort source deletion asynchronous and non-authoritative. Logging failure must not change consume success, audit result, limiter reservation, or user-facing reply.
4. Preserve exact R4 ordering: feature/chat/syntax validation → rolling reservation → durable typed rate-limit audit → typed limiter sentinel; audit persistence failure remains a generic structural error and the reservation remains consumed.
5. Do not alter actor ownership, migration, link consume transactionality, previous-head compatibility, public routes, or accepted onboarding/schema/audit APIs unless a demonstrated blocker requires it.

## Mandatory deterministic tests

1. For every outbox failure path, use sentinel decimal values with a private chat whose `Chat.ID == From.ID`, plus a distinct source message ID. Capture the complete structured slog output and assert absence of bare decimals and rendered variants (`chat=`, `message=`, JSON, URL/form spellings), code/token/request/path canaries, and raw wrapped error causes.
2. Drive both durable-rate-limit success and injected audit-persistence failure through a real `bot.Bot` update/reply/outbox/sender path with an injected failing `bot.API`; a fake loop sender alone is insufficient. Assert the correct constant user response while the full production log graph remains redacted.
3. Cover successful consume followed by failed best-effort DeleteMessage through the same production path; committed identity state must remain committed and all identifiers remain absent.
4. Cover queue saturation deterministically without timing/sleeps that can pass spuriously. Assert the overflow log is redacted and the FSM/Store path does not block.
5. Retain and rerun focused rolling-limiter, concurrent-writer, audit-failure, migration/compatibility, sanitized HTTP-adapter, and pinned previous-head suites under repetition and race.

## Verification and handoff

Run focused high-count tests, full uncached `go test ./...`, pinned previous-head, full `-race`, `go vet ./...`, previous-head vet, `go build ./...`, scoped gofmt, `git diff --check`, privacy/legacy-sink scans, and `task-board validate` after the last edit.

Create exactly one superseding outcome resource named `TASK-260712-2xkyot_rework-r6-results.md`. Record exact changed-file inventory and SHA-256 hashes, line-by-line invariant/test mapping, commands and results, dirty-tree boundaries, and honest external/live Telegram gaps. Set `to-review`; do not mark done. Independent review and root line-by-line/hash/test acceptance remain mandatory.
