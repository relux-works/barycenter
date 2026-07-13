# TASK-260712-2xkyot — Telegram rework R6 producer evidence

Date: 2026-07-13
Role: developer
Handoff result: **PASS — ready for independent security/compatibility review and root audit.**

This outcome supersedes the rejected R4 producer outcome for the narrow R6
privacy correction. It does not claim task acceptance. No commit or push was
performed.

## Implemented correction

- Telegram outbox overflow records now contain only a constant operation field.
- Reply-send and secret-delete failures pass through a second Bot-boundary
  sanitizer. Already-sanitized HTTP adapter errors retain their safe cause
  class; arbitrary alternate API errors and outer wrappers are reduced to a
  constant operation failure and cannot expose their raw cause through
  rendering, `errors.Is`, or `errors.As`.
- Best-effort source deletion remains asynchronous and non-authoritative.
  Its failure or queue saturation cannot alter the committed consume, limiter
  reservation, durable audit, or user-facing reply.
- The accepted R4 ordering and shared implementation remain unchanged:
  feature/chat/syntax validation, atomic rolling-window reservation, durable
  typed audit on rejection, typed limiter sentinel only after audit success,
  then credential and `BEGIN IMMEDIATE` work for admitted attempts.

## Exact R6 implementation/log changed-file inventory

The outcome artifact itself is excluded from this self-referential inventory;
its SHA-256 is recorded separately at attachment time.

```text
96d295381a10197506eee4bf0d99adb7f0a9ecbf04bc3abb596e929f33fa5b04  coordinator/internal/bot/bot.go
96638935ed384bd6ff99a776bcd6b505eb39a96b0aafaeabb3625355411db04b  coordinator/internal/bot/bot_test.go
175c65f22c92649d27140964f911b1b7deb9621a2e1361301b66ccba8481b1ac  coordinator/cmd/duet-coordinator/telegram_identity_test.go
e85f5888f68675756302e01455a24e22b93b80786d1b1fa65234caa48471b222  LOGBOOK.md
```

Frozen R6 starting hashes for the three Telegram files were respectively:

```text
a94852e8dbed50e4faf5ae45569247022c09d6c2c04bab8696627d11f5033d8c  coordinator/internal/bot/bot.go
96718ef5a213327de1240985a4205a1c949f67dcf22cd32be7b230f8ea5260fb  coordinator/internal/bot/bot_test.go
30aa12a2a08895e048e7aeb1b7f3830b83ba73d453de59056c79b04c380959fe  coordinator/cmd/duet-coordinator/telegram_identity_test.go
```

`LOGBOOK.md` received only the task-scoped R6 decision and evidence entries.

## Frozen shared boundary — unchanged

The R5-frozen Store, schema, onboarding, audit, and adapter boundary hashes were
recomputed after the last product/test edit and remain exact:

```text
1d99a568881d5bc22b53166a9d76cd04d6bae10ef59a53c27d39d5b1dab72451  coordinator/internal/store/identity_telegram.go
17b046bc1202f632f8082d7121ae40e835106de5ddd4ecf2fe14794887f07c4d  coordinator/internal/store/identity_telegram_test.go
efdec398578634c162f44b60e51254d82c49b12b4fe90ee6688fdf3b03ca963b  coordinator/internal/store/identity_telegram_previous_head_test.go
90940d1252d9a44b6174bb7482b8a71aed522c450022321c02003e3c3f6137c1  coordinator/cmd/duet-coordinator/loop.go
840eea9ca9222e2077b363599b173ea2f6060e752fcfaa8a0f4361536fd38134  coordinator/internal/store/identity.go
6c28dd5fbcfea56357584a4c033ed9f13c8ab1875b50f69c70c072c17937308f  coordinator/internal/store/identity_schema.go
77f30536883a5798274e0b001bde7299f37bea5f6e64b0d402f1362cc1bba0f9  coordinator/internal/store/onboarding.go
194c04fc7861b9521b98d42d7e84e1a517807445e7974b5bcc073a498f821faa  coordinator/internal/store/security_audit.go
```

No shared onboarding, schema, identity, durable-audit, transaction, or loop
implementation was edited in R6.

## Line-by-line invariant and test mapping

| Requirement | Production/test evidence |
| --- | --- |
| Send overflow contains no private identifiers or payload | `coordinator/internal/bot/bot.go:145-150`; deterministic exact-capacity unit coverage at `coordinator/internal/bot/bot_test.go:383-408` |
| Send/delete transport failures contain only operation plus sanitized cause | `coordinator/internal/bot/bot.go:155-169`; arbitrary wrapped-error/error-graph coverage at `coordinator/internal/bot/bot_test.go:312-381` |
| Delete overflow contains no chat/message identifier | `coordinator/internal/bot/bot.go:208-213`; private-update coverage at `coordinator/internal/bot/bot_test.go:410-432` |
| Alternate API errors cannot bypass HTTP-adapter redaction | `coordinator/internal/bot/bot.go:308-318`; safe rendering plus unreachable raw cause at `coordinator/internal/bot/bot_test.go:312-379` |
| Tests use the real Bot polling, update, loop, outbox, and sender graph | R6 API/log/harness at `coordinator/cmd/duet-coordinator/telegram_identity_test.go:56-296` |
| Durable rate-limit success and injected audit failure traverse real Bot | `coordinator/cmd/duet-coordinator/telegram_identity_test.go:654-743`; verifies generic persistence-failure reply, subsequent typed rate-limit reply, typed durable row/digest/NULL scope, no legacy row, and full log/reply redaction |
| Successful consume survives failed asynchronous deletion and send | `coordinator/cmd/duet-coordinator/telegram_identity_test.go:745-796`; verifies committed `ActorContext` after both transport failures |
| Exact-capacity outbox saturation is deterministic and nonblocking | `coordinator/cmd/duet-coordinator/telegram_identity_test.go:798-846`; a channel checkpoint holds the real sender, fills all 1,024 slots, and verifies committed Store state without sleep-based saturation proof |
| R4 reservation/audit/transaction order unchanged | `coordinator/internal/store/identity_telegram.go:110-142` (unchanged hash) |
| Audit failure cannot map to typed rate limit; deletion follows commit only | `coordinator/cmd/duet-coordinator/loop.go:1383-1415` (unchanged hash), plus the real-Bot tests above |
| Migration, role/slot ownership, ActorContext, mixed legacy/self-service pair, conflict, rollback, rolling limiter, deterministic writer barrier, and previous-head behavior remain covered | repeated Store/coordinator focused suites, 50-count writer races, full suite, full race, and pinned `previoushead` commands below |

Every R6 privacy test uses runtime-generated link credentials. Plaintext link
codes, credentials, hashes derived from submitted codes, raw identifiers, bot
tokens, request bodies, URLs, file IDs, and destination paths are absent from
this artifact.

## Verification command ledger

All commands ran from `coordinator/` unless an explicit project-root command is
shown. No command failed after implementation; therefore there is no corrected
failure transcript to report.

### R6 smoke and high-count tests

```text
$ go test -count=1 ./internal/bot -run '^(TestOutbox|TestBestEffortDelete|TestHTTPAPI)' -v
PASS
ok  relux.works/duet/coordinator/internal/bot  0.378s
exit 0

$ go test -count=1 ./cmd/duet-coordinator -run '^(TestTelegramLinkRateLimitAuditFailureUsesGenericAdapterResponse|TestTelegramLinkSuccessfulConsumeKeepsCommitWhenRealBotDeleteFails|TestTelegramLinkRealBotQueueSaturationDoesNotBlockCommittedConsume)$' -v
PASS
ok  relux.works/duet/coordinator/cmd/duet-coordinator  0.459s
exit 0

$ go test -count=20 ./internal/bot -run '^(TestOutboxSenderFailureLogsRedactUpdateIdentifiersAndErrorGraph|TestOutboxOverflowLogsRedactPrivateUpdateIdentifiersAndPayload|TestBestEffortDeleteFailureLogUsesSanitizedHTTPAdapterError|TestHTTPAPITransportErrorsRedactTokenURLsAndRequestMaterial)$'
ok  relux.works/duet/coordinator/internal/bot  0.413s
exit 0

$ go test -count=20 ./cmd/duet-coordinator -run '^(TestTelegramLinkRateLimitAuditFailureUsesGenericAdapterResponse|TestTelegramLinkSuccessfulConsumeKeepsCommitWhenRealBotDeleteFails|TestTelegramLinkRealBotQueueSaturationDoesNotBlockCommittedConsume)$'
ok  relux.works/duet/coordinator/cmd/duet-coordinator  1.370s
exit 0

$ go test -race -count=10 ./internal/bot -run '^(TestOutboxSenderFailureLogsRedactUpdateIdentifiersAndErrorGraph|TestOutboxOverflowLogsRedactPrivateUpdateIdentifiersAndPayload)$'
ok  relux.works/duet/coordinator/internal/bot  1.410s
exit 0

$ go test -race -count=10 ./cmd/duet-coordinator -run '^(TestTelegramLinkRateLimitAuditFailureUsesGenericAdapterResponse|TestTelegramLinkSuccessfulConsumeKeepsCommitWhenRealBotDeleteFails|TestTelegramLinkRealBotQueueSaturationDoesNotBlockCommittedConsume)$'
ok  relux.works/duet/coordinator/cmd/duet-coordinator  5.860s
exit 0
```

### Mandatory Telegram identity, migration, limiter, compatibility, and writer tests

```text
$ go test -count=20 ./internal/store -run '^(TestTelegramMigration|TestConsumeTelegramLink|TestLinkedTelegram|TestTelegramLinkAttemptLimiter|TestTelegramLinkIssue|TestTelegramResolver)'
ok  relux.works/duet/coordinator/internal/store  18.923s
exit 0

$ go test -count=20 ./cmd/duet-coordinator -run '^(TestTelegram|TestMigratedTelegram|TestAppOwnedOrbitTelegram)'
ok  relux.works/duet/coordinator/cmd/duet-coordinator  5.071s
exit 0

$ go test -count=20 ./internal/bot -run '^(TestTelegramLinkCodeCarriesVerifiedUpdateMetadata|TestParseTelegramLinkCodeNormalizationShape|TestOutboxSenderFailureLogsRedactUpdateIdentifiersAndErrorGraph|TestOutboxOverflowLogsRedactPrivateUpdateIdentifiersAndPayload|TestHTTPAPITransportErrorsRedactTokenURLsAndRequestMaterial|TestHTTPAPIRedirectsFailClosedWithoutReachingTarget|TestHTTPAPIFilesystemErrorGraphDoesNotExposeDestinationPath|TestHTTPAPIRejectedResponseDoesNotEchoTelegramDescription|TestBestEffortDeleteFailureLogUsesSanitizedHTTPAdapterError)$'
ok  relux.works/duet/coordinator/internal/bot  1.342s
exit 0

$ go test -count=50 ./internal/store -run '^(TestConsumeTelegramLinkConcurrentSameCodeTwoUsersHasOneWinner|TestConsumeTelegramLinkConcurrentTwoCodesSameUserLeavesLoserCode)$'
ok  relux.works/duet/coordinator/internal/store  13.794s
exit 0

$ go test -race -count=10 ./internal/store -run '^(TestConsumeTelegramLinkFeatureOffPrivateChatAndRateLimit|TestTelegramLinkAttemptLimiterConcurrentReservationsAndLRUBound)$'
ok  relux.works/duet/coordinator/internal/store  7.367s
exit 0

$ go test -race -count=10 ./cmd/duet-coordinator -run '^(TestTelegramLinkRateLimitAuditFailureUsesGenericAdapterResponse|TestTelegramLinkSuccessfulConsumeKeepsCommitWhenRealBotDeleteFails|TestTelegramLinkRealBotQueueSaturationDoesNotBlockCommittedConsume)$'
ok  relux.works/duet/coordinator/cmd/duet-coordinator  6.979s
exit 0

$ go test -race -count=10 ./internal/bot -run '^(TestOutboxSenderFailureLogsRedactUpdateIdentifiersAndErrorGraph|TestOutboxOverflowLogsRedactPrivateUpdateIdentifiersAndPayload|TestHTTPAPITransportErrorsRedactTokenURLsAndRequestMaterial|TestBestEffortDeleteFailureLogUsesSanitizedHTTPAdapterError)$'
ok  relux.works/duet/coordinator/internal/bot  1.429s
exit 0
```

### Full, previous-head, race, vet, and build

```text
$ go test -count=1 ./...
ok   relux.works/duet/coordinator/cmd/duet-coordinator  3.208s
ok   relux.works/duet/coordinator/internal/bot  0.700s
ok   relux.works/duet/coordinator/internal/config  1.090s
ok   relux.works/duet/coordinator/internal/hub  2.194s
ok   relux.works/duet/coordinator/internal/links  2.682s
ok   relux.works/duet/coordinator/internal/media  3.086s
ok   relux.works/duet/coordinator/internal/protocol  2.778s
ok   relux.works/duet/coordinator/internal/resolver  2.843s
ok   relux.works/duet/coordinator/internal/session  2.096s
?    relux.works/duet/coordinator/internal/spotify  [no test files]
ok   relux.works/duet/coordinator/internal/store  6.899s
?    relux.works/duet/coordinator/internal/ulid  [no test files]
exit 0

$ go test -tags previoushead -count=1 ./internal/store
ok  relux.works/duet/coordinator/internal/store  14.737s
exit 0

$ go test -race -count=1 ./...
ok   relux.works/duet/coordinator/cmd/duet-coordinator  13.978s
ok   relux.works/duet/coordinator/internal/bot  2.467s
ok   relux.works/duet/coordinator/internal/config  3.241s
ok   relux.works/duet/coordinator/internal/hub  2.000s
ok   relux.works/duet/coordinator/internal/links  2.841s
ok   relux.works/duet/coordinator/internal/media  3.484s
ok   relux.works/duet/coordinator/internal/protocol  1.978s
ok   relux.works/duet/coordinator/internal/resolver  1.937s
ok   relux.works/duet/coordinator/internal/session  2.008s
?    relux.works/duet/coordinator/internal/spotify  [no test files]
ok   relux.works/duet/coordinator/internal/store  33.627s
?    relux.works/duet/coordinator/internal/ulid  [no test files]
exit 0

$ go vet ./...
(no output)
exit 0

$ go vet -tags previoushead ./internal/store
(no output)
exit 0

$ go build ./...
(no output)
exit 0
```

### Formatting, diff, privacy, call-graph, and board checks

```text
$ files=(${(f)"$(rg --files -g '*.go')"}); gofmt -l $files
(no output)
exit 0

$ git diff --check
(no output)
exit 0

$ rg -n 'outbox full|secret message deletion failed|send failed|safeTelegramLogError' coordinator/internal/bot/bot.go
149: b.log.Warn("outbox full, dropping message", "operation", "sendMessage")
163: b.log.Debug("secret message deletion failed", "operation", "deleteMessage", "err", safeTelegramLogError("deleteMessage", err))
168: b.log.Warn("send failed", "operation", "sendMessage", "err", safeTelegramLogError("sendMessage", err))
212: b.log.Warn("outbox full, cannot delete secret message", "operation", "deleteMessage")
308: // safeTelegramLogError is a second redaction boundary for alternate API
312: func safeTelegramLogError(operation string, err error) error {
exit 0

$ rg -n '"(chat|message)",' coordinator/internal/bot/bot.go
(no matches)
exit 1, expected negative scan

$ rg -n 'fmt\.Errorf\([^\n]*%w|errors\.Join' coordinator/internal/bot/bot.go
(no matches)
exit 1, expected negative scan

$ rg -n 'ConsumeTelegramLink\(' coordinator --glob '*.go'
Production definitions/callers: internal/store/identity_telegram.go:106 and cmd/duet-coordinator/loop.go:1391. All other matches are tests.
exit 0

$ rg -n 'LogEvent\([^\n]*(rate|Rate)|telegram_link\.rate_limited|INSERT OR REPLACE' coordinator/internal/store/identity_telegram.go coordinator/cmd/duet-coordinator/loop.go
(no matches)
exit 1, expected negative scan

$ task-board validate
Board is valid. No issues found.
exit 0
```

## Dirty-worktree and concurrency boundary

The worktree was dirty before R6 and remained dirty. Concurrent sibling work
is present in CI, coordinator onboarding/config/orbit/store files, shared
identity/schema/audit files, documentation, Swift node-app code, and Windows
runtime code. Several accepted identity/onboarding files and
`telegram_identity_test.go` are untracked relative to repository `HEAD`.

R6 preserved that work. Its product/test/log modification boundary is exactly
the four files in the R6 inventory above; the only additional file is this
task-scoped outcome artifact. The frozen hashes demonstrate that the shared Telegram
Store, loop, identity, schema, onboarding, and durable-audit inputs did not drift
during this run. `task-board spawn directives` reported no operator directive.

## Evidence boundaries

- Tests use isolated temporary SQLite databases and injected Bot/HTTP APIs.
- No request contacted Telegram or production infrastructure.
- Live Telegram behavior and external CI were not run and are not claimed.
- Producer evidence is not independent acceptance. A fresh independent
  security/compatibility review and root line-by-line/hash/test audit remain
  required before downstream reliance.
