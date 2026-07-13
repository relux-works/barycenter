# TASK-260712-2xkyot — independent Telegram R5 review mandate

Date: 2026-07-13
Owner: root orchestrator
Mode: **read-only independent security and compatibility review**

Review the Telegram R4 producer result against the frozen Rev15 contract, all
task guards, the prior R3 finding, and the actual working-tree files. Do not
edit production code, tests, specs, board internals, or existing resources.
Do not commit, push, reset, checkout, clean, or change the task status. The only
permitted write is one new task-scoped review outcome named
`TASK-260712-2xkyot_security-review-r5.md`.

## Frozen inputs

- `.task-board/.resources/TASK-260712-3v1k7q/research.md`
- `.task-board/.resources/TASK-260712-3v1k7q/p1-root-review-amendments.md`
- `.task-board/.resources/TASK-260712-2xkyot/TASK-260712-2xkyot-rework-guard-r4.md`
- `.task-board/.resources/TASK-260712-2xkyot/TASK-260712-2xkyot_security-review-r3.md`
- `.task-board/.resources/TASK-260712-2xkyot/TASK-260712-2xkyot_rework-r4-results.md`

Review these exact current hashes; report and stop on drift:

```text
1d99a568881d5bc22b53166a9d76cd04d6bae10ef59a53c27d39d5b1dab72451  coordinator/internal/store/identity_telegram.go
17b046bc1202f632f8082d7121ae40e835106de5ddd4ecf2fe14794887f07c4d  coordinator/internal/store/identity_telegram_test.go
efdec398578634c162f44b60e51254d82c49b12b4fe90ee6688fdf3b03ca963b  coordinator/internal/store/identity_telegram_previous_head_test.go
90940d1252d9a44b6174bb7482b8a71aed522c450022321c02003e3c3f6137c1  coordinator/cmd/duet-coordinator/loop.go
30aa12a2a08895e048e7aeb1b7f3830b83ba73d453de59056c79b04c380959fe  coordinator/cmd/duet-coordinator/telegram_identity_test.go
840eea9ca9222e2077b363599b173ea2f6060e752fcfaa8a0f4361536fd38134  coordinator/internal/store/identity.go
6c28dd5fbcfea56357584a4c033ed9f13c8ab1875b50f69c70c072c17937308f  coordinator/internal/store/identity_schema.go
77f30536883a5798274e0b001bde7299f37bea5f6e64b0d402f1362cc1bba0f9  coordinator/internal/store/onboarding.go
194c04fc7861b9521b98d42d7e84e1a517807445e7974b5bcc073a498f821faa  coordinator/internal/store/security_audit.go
```

## Required review questions

1. Does every Telegram consume rejection that maps to
   `ErrTelegramLinkRateLimited` first persist exactly one durable
   `security.rate_limited` record through the frozen typed audit repository?
2. Is the limiter order still exactly feature/chat/syntax validation, atomic
   reservation, durable audit only on rejection, then credential/transaction
   work only for admitted attempts? Does audit failure leave the reservation
   consumed and return a structural non-sentinel error?
3. Is the stored subject only the class-domain-separated SHA-256 digest of the
   verified decimal Telegram user ID, with NULL pre-identity scope and no raw
   ID/code/body in durable rows, logs, errors, replies, URLs, or test artifacts?
4. Can any database/trigger/transport failure be misclassified as a 429 or a
   credential/conflict sentinel? Can the adapter expose a raw user ID or link
   code through structured logging or an error graph?
5. Do the new tests independently verify the digest and inject a real durable
   insert failure, including the next-attempt behavior? Look for tests that
   merely mirror production or accidentally pass without exercising the real
   adapter/store path.
6. Re-audit the surrounding R2/R3 Telegram invariants: exact rolling limiter,
   transaction-attempt checkpoint, independent-connection races, one-time
   consume/rollback, ActorContext lifecycle authorization, legacy dual-write,
   generic credential failure, redirect/error redaction, and pinned
   previous-head compatibility. Report any regression even if outside the
   narrow R4 diff.
7. Check that the accepted shared onboarding/audit files are unchanged and
   that no hidden call site still writes the legacy best-effort rate-limit
   event or bypasses the trusted in-process consume adapter.

## Independent executable evidence

Run, at minimum, from `coordinator/`:

```text
go test -count=20 ./internal/store -run '^(TestTelegramMigration|TestConsumeTelegramLink|TestLinkedTelegram|TestTelegramLinkAttemptLimiter|TestTelegramLinkIssue|TestTelegramResolver)'
go test -count=20 ./cmd/duet-coordinator -run '^(TestTelegram|TestMigratedTelegram|TestAppOwnedOrbitTelegram)'
go test -race -count=10 ./internal/store -run '^(TestConsumeTelegramLinkFeatureOffPrivateChatAndRateLimit|TestTelegramLinkAttemptLimiterConcurrentReservationsAndLRUBound)$'
go test -race -count=10 ./cmd/duet-coordinator -run '^TestTelegramLinkRateLimitAuditFailureUsesGenericAdapterResponse$'
go test -count=50 ./internal/store -run '^(TestConsumeTelegramLinkConcurrentSameCodeTwoUsersHasOneWinner|TestConsumeTelegramLinkConcurrentTwoCodesSameUserLeavesLoserCode)$'
go test -count=1 ./...
go test -tags previoushead -count=1 ./internal/store
go test -race -count=1 ./...
go vet ./...
go vet -tags previoushead ./internal/store
go build ./...
```

Also run formatting, `git diff --check`, focused secret/raw-ID/legacy-sink
scans, call-graph inspection, and `task-board validate`. Tests may read/write
only isolated temporary databases; do not contact Telegram or production.

## Outcome format

Lead with `PASS` or `BACK TO DEVELOPMENT`. List findings by severity with exact
file/line evidence and a concrete failure schedule. Record commands and exit
codes, current hashes, limitations, and the SHA-256 of the review outcome.
Absence of live Telegram/external CI is an evidence boundary, not a failure.
Do not claim root acceptance or task completion.
