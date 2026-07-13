# TASK-260712-2xkyot — independent Telegram R7 review mandate

Date: 2026-07-13
Owner: root orchestrator
Mode: **read-only independent security and compatibility review**

Review the Telegram R6 producer result against the frozen Rev15 contract, all
Telegram guards and prior findings, and the actual working-tree files. Do not
edit production code, tests, specs, board internals, existing resources, or
task status. Do not commit, push, reset, checkout, or clean. The only permitted
write is one new task-scoped review outcome named
`TASK-260712-2xkyot_security-review-r7.md`.

## Frozen inputs

- `.task-board/.resources/TASK-260712-3v1k7q/research.md`
- `.task-board/.resources/TASK-260712-3v1k7q/p1-root-review-amendments.md`
- `.task-board/.resources/TASK-260712-2xkyot/TASK-260712-2xkyot-rework-guard-r6.md`
- `.task-board/.resources/TASK-260712-2xkyot/TASK-260712-2xkyot_security-review-r5.md`
- `.task-board/.resources/TASK-260712-2xkyot/TASK-260712-2xkyot_rework-r6-results.md`

Review these exact current hashes; report and stop on drift:

```text
96d295381a10197506eee4bf0d99adb7f0a9ecbf04bc3abb596e929f33fa5b04  coordinator/internal/bot/bot.go
96638935ed384bd6ff99a776bcd6b505eb39a96b0aafaeabb3625355411db04b  coordinator/internal/bot/bot_test.go
175c65f22c92649d27140964f911b1b7deb9621a2e1361301b66ccba8481b1ac  coordinator/cmd/duet-coordinator/telegram_identity_test.go
1d99a568881d5bc22b53166a9d76cd04d6bae10ef59a53c27d39d5b1dab72451  coordinator/internal/store/identity_telegram.go
17b046bc1202f632f8082d7121ae40e835106de5ddd4ecf2fe14794887f07c4d  coordinator/internal/store/identity_telegram_test.go
efdec398578634c162f44b60e51254d82c49b12b4fe90ee6688fdf3b03ca963b  coordinator/internal/store/identity_telegram_previous_head_test.go
90940d1252d9a44b6174bb7482b8a71aed522c450022321c02003e3c3f6137c1  coordinator/cmd/duet-coordinator/loop.go
840eea9ca9222e2077b363599b173ea2f6060e752fcfaa8a0f4361536fd38134  coordinator/internal/store/identity.go
6c28dd5fbcfea56357584a4c033ed9f13c8ab1875b50f69c70c072c17937308f  coordinator/internal/store/identity_schema.go
77f30536883a5798274e0b001bde7299f37bea5f6e64b0d402f1362cc1bba0f9  coordinator/internal/store/onboarding.go
194c04fc7861b9521b98d42d7e84e1a517807445e7974b5bcc073a498f821faa  coordinator/internal/store/security_audit.go
9a04b44784201d11ad688ae624f3202343946d16ec37746bf59a0c2205c5cd16  TASK-260712-2xkyot_rework-r6-results.md
```

## Required review questions

1. Audit all four mandatory failure paths line by line: `SendTo` queue
   overflow, reply-send failure, source-delete failure, and source-delete
   queue overflow. Can a private chat/user ID, message ID, code, token, body,
   URL, file ID, destination path, or raw wrapped cause appear in structured
   output or the reachable error graph?
2. Is `safeTelegramLogError` a real second redaction boundary for arbitrary
   alternate API wrappers while preserving only an already-sanitized
   `telegramOperationError`? Look for nested/joined errors, unsafe `Unwrap`,
   formatting, and custom-wrapper bypasses that matter in the real path.
3. Do the tests use sentinel private-chat IDs (`Chat.ID == From.ID`), distinct
   message IDs, complete slog capture, and all required canaries? Do they
   actually trigger each path, or can scheduling/capacity make them pass
   without the intended failure?
4. Trace the real `bot.New` -> `Bot.Run` -> update -> loop/store -> deletion /
   reply -> outbox -> sender graph in the three R6 integration tests. Verify
   deterministic exact-capacity saturation and that cleanup/goroutine ordering
   cannot hide missing logs, races, or a blocked/rolled-back commit.
5. Re-audit the durable rate-limit behavior: reservation before audit, generic
   structural response on audit failure, reservation retained, exactly one
   typed durable row on the next rejection, correct class-separated digest,
   NULL pre-identity scope, no legacy event, and no raw identifiers.
6. Confirm successful consume remains committed despite asynchronous delete
   and send failures. Recheck one-time transactionality, ActorContext lifecycle
   authorization, migrated/mixed legacy behavior, and pinned previous-head
   compatibility for regressions.
7. Confirm the shared identity/schema/onboarding/audit/loop files did not drift
   and no hidden legacy sink or production caller bypasses the reviewed path.
   Report any material surrounding privacy issue even if it predates R6.

## Independent executable evidence

Run, at minimum, from `coordinator/`:

```text
go test -count=20 ./internal/bot -run '^(TestOutboxSenderFailureLogsRedactUpdateIdentifiersAndErrorGraph|TestOutboxOverflowLogsRedactPrivateUpdateIdentifiersAndPayload|TestHTTPAPITransportErrorsRedactTokenURLsAndRequestMaterial|TestHTTPAPIRedirectsFailClosedWithoutReachingTarget|TestHTTPAPIFilesystemErrorGraphDoesNotExposeDestinationPath|TestHTTPAPIRejectedResponseDoesNotEchoTelegramDescription|TestBestEffortDeleteFailureLogUsesSanitizedHTTPAdapterError)$'
go test -race -count=10 ./internal/bot -run '^(TestOutboxSenderFailureLogsRedactUpdateIdentifiersAndErrorGraph|TestOutboxOverflowLogsRedactPrivateUpdateIdentifiersAndPayload|TestHTTPAPITransportErrorsRedactTokenURLsAndRequestMaterial|TestBestEffortDeleteFailureLogUsesSanitizedHTTPAdapterError)$'
go test -count=20 ./cmd/duet-coordinator -run '^(TestTelegramLinkRateLimitAuditFailureUsesGenericAdapterResponse|TestTelegramLinkSuccessfulConsumeKeepsCommitWhenRealBotDeleteFails|TestTelegramLinkRealBotQueueSaturationDoesNotBlockCommittedConsume)$'
go test -race -count=10 ./cmd/duet-coordinator -run '^(TestTelegramLinkRateLimitAuditFailureUsesGenericAdapterResponse|TestTelegramLinkSuccessfulConsumeKeepsCommitWhenRealBotDeleteFails|TestTelegramLinkRealBotQueueSaturationDoesNotBlockCommittedConsume)$'
go test -count=20 ./internal/store -run '^(TestTelegramMigration|TestConsumeTelegramLink|TestLinkedTelegram|TestTelegramLinkAttemptLimiter|TestTelegramLinkIssue|TestTelegramResolver)'
go test -count=50 ./internal/store -run '^(TestConsumeTelegramLinkConcurrentSameCodeTwoUsersHasOneWinner|TestConsumeTelegramLinkConcurrentTwoCodesSameUserLeavesLoserCode)$'
go test -race -count=10 ./internal/store -run '^(TestConsumeTelegramLinkFeatureOffPrivateChatAndRateLimit|TestTelegramLinkAttemptLimiterConcurrentReservationsAndLRUBound)$'
go test -count=1 ./...
go test -tags previoushead -count=1 ./internal/store
go test -race -count=1 ./...
go vet ./...
go vet -tags previoushead ./internal/store
go build ./...
```

Also run formatting, `git diff --check`, focused privacy/raw-ID/legacy-sink
scans, production call-graph inspection, and `task-board validate`. Tests may
touch only isolated temporary databases; do not contact Telegram or production.

## Outcome format

Lead with `PASS` or `BACK TO DEVELOPMENT`. List findings by severity with exact
file/line evidence and a concrete failure schedule. Record commands and exit
codes, current hashes, limitations, and the SHA-256 of the outcome after it is
attached. Absence of live Telegram/external CI is an evidence boundary, not a
failure. Do not claim root acceptance or task completion.
