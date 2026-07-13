# TASK-260712-2xkyot — Telegram durable-audit R4 producer evidence

Date: 2026-07-13 18:59 +04
Role: developer
Handoff target: `to-review`

## Outcome

The Telegram link-consume rolling limiter keeps its accepted ordering and
semantics: feature/chat/syntax validation, then atomic per-verified-user
reservation, then credential lookup and `BEGIN IMMEDIATE` only for admitted
attempts. A rejected reservation now calls the frozen shared
`Store.RecordRateLimitAudit` repository with:

- class `telegram-link-consume/telegram-user`;
- the exact decimal verified Telegram user ID as the in-memory subject; and
- empty pre-identity actor/orbit scope.

Only the class-domain-separated SHA-256 subject digest is persisted. The old
best-effort `Store.LogEvent`/legacy `events` write is removed. A durable insert
failure returns a constant-wrapped structural persistence error; it does not
return the rate-limit, credential, or conflict sentinel. The reservation remains
consumed. Only a successful durable insert permits
`ErrTelegramLinkRateLimited` and the normal bot rate-limit response.

The trusted bot adapter's unexpected-error log retains the constant operation
name and safe underlying cause but no longer attaches the raw Telegram user ID.
The adapter maps durable-audit failure to its ordinary generic credential
message and maps the next durably audited rejection to the rate-limit message.

No shared audit, schema, identity, or onboarding implementation was edited.

## Changed files and exact SHA-256

```text
1d99a568881d5bc22b53166a9d76cd04d6bae10ef59a53c27d39d5b1dab72451  coordinator/internal/store/identity_telegram.go
17b046bc1202f632f8082d7121ae40e835106de5ddd4ecf2fe14794887f07c4d  coordinator/internal/store/identity_telegram_test.go
90940d1252d9a44b6174bb7482b8a71aed522c450022321c02003e3c3f6137c1  coordinator/cmd/duet-coordinator/loop.go
30aa12a2a08895e048e7aeb1b7f3830b83ba73d453de59056c79b04c380959fe  coordinator/cmd/duet-coordinator/telegram_identity_test.go
2474ea87bf1e43beea9d569df2181354c6504ee908c3c1487ec4339cca9cd627  LOGBOOK.md
```

The accepted pre-R4 Telegram hashes were:

```text
583633651c14995eafd9c1bb2d3647cf2c39582e07f34f66f00b2042003ff8db  coordinator/internal/store/identity_telegram.go
a040832d88b061fcbae98558a3b7380d2b43f18bd3b8e5a692730481d987d587  coordinator/internal/store/identity_telegram_test.go
7a4786c970b97e6c4992bbec3b46be1445a6a601b4ab586566a3eaf4d866760a  coordinator/cmd/duet-coordinator/loop.go
3dff8d2fbebfd6661ec406432e4f35738f3dd591441bc9e60d99e2e22d4ecb3d  coordinator/cmd/duet-coordinator/telegram_identity_test.go
```

## Frozen shared onboarding/audit boundary

The exact root-accepted shared hashes remained byte-for-byte unchanged before
and after this R4 edit:

```text
840eea9ca9222e2077b363599b173ea2f6060e752fcfaa8a0f4361536fd38134  coordinator/internal/store/identity.go
6c28dd5fbcfea56357584a4c033ed9f13c8ab1875b50f69c70c072c17937308f  coordinator/internal/store/identity_schema.go
77f30536883a5798274e0b001bde7299f37bea5f6e64b0d402f1362cc1bba0f9  coordinator/internal/store/onboarding.go
194c04fc7861b9521b98d42d7e84e1a517807445e7974b5bcc073a498f821faa  coordinator/internal/store/security_audit.go
8c2d5544a75cc09eb6b9b3980e91096a6ba8ef46e093c8fa12f847bc4f45cf2a  coordinator/internal/store/identity_migration_rework_test.go
08d8d49e269701a03bb4bbcf5be49f6e9fd71a54aa00a9fabc6f1fa96c566ec0  coordinator/internal/store/onboarding_rework_r2_test.go
d0c969f388d2b4138918c3e07490216c99c99f8565d4b90a39cb9238c53a1d1e  coordinator/cmd/duet-coordinator/onboarding.go
8b7e8582a7de081653e778e5d88fb6ba0db7858d5c813bdf3a40f4166ab7c350  coordinator/cmd/duet-coordinator/onboarding_rework_r2_test.go
```

The previous-head Telegram compatibility test was also unchanged:

```text
efdec398578634c162f44b60e51254d82c49b12b4fe90ee6688fdf3b03ca963b  coordinator/internal/store/identity_telegram_previous_head_test.go
```

## Guard, AC, and executable-flow mapping

| Requirement | Production and executable proof |
|---|---|
| Exact limiter ordering/rolling semantics unchanged | `consumeTelegramLinkAt` still validates feature, principal/chat, and syntax before the existing limiter reservation. Existing fake-clock, rejected-boundary, fixed-window-burst, concurrency, and LRU tests pass unchanged. |
| Durable N+1 audit | `TestConsumeTelegramLinkFeatureOffPrivateChatAndRateLimit/atomic_attempt_boundary` crosses attempts 1–10 plus the rejected attempt and asserts exactly one `security.rate_limited` row with the Telegram-consume class, independently computed class-domain-separated digest, NULL actor/orbit scope, and sane timestamp. |
| No legacy sink/raw identity | The same test asserts zero legacy `events` rate-limit rows and that the raw decimal subject is absent from every persisted field rendered by the test. Static scans prove the production Store path contains no `LogEvent`, legacy event type, raw-ID field, or generic JSON map. |
| Persistence failure semantics | `.../durable_audit_failure_consumes_attempt_and_fails_structurally` injects a `BEFORE INSERT` abort, asserts a constant-wrapped non-sentinel error, zero durable rows, no identity/code in the error, then removes the trigger and proves the next attempt is still rejected and persists exactly one typed digest row. |
| Production adapter mapping | `TestTelegramLinkRateLimitAuditFailureUsesGenericAdapterResponse` drives the real bot loop: audit failure produces the generic credential reply and no row; the following successful durable audit produces the normal rate-limit reply. Captured logs/messages contain neither raw identity nor any runtime-generated link material. |
| Shared ActorContext and compatibility | Full focused Telegram store/bot/coordinator repetitions, the complete uncached suite, and exact previous-head suite retain migrated role, slot, pair, share, transfer, leave, revoke, feature-off, and app-owned identity behavior. |
| Transaction/concurrency proof | The two R2 independent-connection tests pass 50 repetitions. Writer two still signals immediately before real `db.Begin()` and cannot reach credential preflight while writer one holds the immediate transaction. |
| HTTP redaction | Focused bot transport/redirection/error-graph tests pass 20 repetitions; no transport or URL code changed. |

## Commands and complete result ledger

All Go commands ran from `coordinator/`. Repository commands ran from the
project root unless stated otherwise.

### Pre-edit baseline

```text
go test -count=1 ./internal/store -run '^(TestConsumeTelegramLinkFeatureOffPrivateChatAndRateLimit|TestTelegramLinkAttemptLimiter)'
ok  relux.works/duet/coordinator/internal/store  0.457s

go test -count=1 ./cmd/duet-coordinator -run '^(TestAppOwnedOrbitTelegramLinkUsesTrustedAdapterAndPreservesPairOwnership)$'
ok  relux.works/duet/coordinator/cmd/duet-coordinator  0.664s
```

### First post-edit focused run

```text
gofmt -w internal/store/identity_telegram.go internal/store/identity_telegram_test.go cmd/duet-coordinator/loop.go cmd/duet-coordinator/telegram_identity_test.go
go test -count=1 ./internal/store -run '^(TestConsumeTelegramLinkFeatureOffPrivateChatAndRateLimit|TestTelegramLinkAttemptLimiter)' -v
go test -count=1 ./cmd/duet-coordinator -run '^(TestTelegramLinkRateLimitAuditFailureUsesGenericAdapterResponse|TestAppOwnedOrbitTelegramLinkUsesTrustedAdapterAndPreservesPairOwnership)$' -v
```

Result: PASS. Every Store subtest passed, including
`atomic_attempt_boundary` and
`durable_audit_failure_consumes_attempt_and_fails_structurally`; both adapter
tests passed.

### Focused repetitions

```text
go test -count=20 ./internal/store -run '^(TestTelegramMigration|TestConsumeTelegramLink|TestLinkedTelegram|TestTelegramLinkAttemptLimiter|TestTelegramLinkIssue|TestTelegramResolver)'
ok  relux.works/duet/coordinator/internal/store  18.821s

go test -count=20 ./cmd/duet-coordinator -run '^(TestTelegram|TestMigratedTelegram|TestAppOwnedOrbitTelegram)'
ok  relux.works/duet/coordinator/cmd/duet-coordinator  3.780s

go test -count=20 ./internal/bot -run '^(TestTelegramLinkCodeCarriesVerifiedUpdateMetadata|TestParseTelegramLinkCodeNormalizationShape|TestHTTPAPITransportErrorsRedactTokenURLsAndRequestMaterial|TestHTTPAPIRedirectsFailClosedWithoutReachingTarget|TestHTTPAPIFilesystemErrorGraphDoesNotExposeDestinationPath|TestHTTPAPIRejectedResponseDoesNotEchoTelegramDescription|TestBestEffortDeleteFailureLogUsesSanitizedHTTPAdapterError)$'
ok  relux.works/duet/coordinator/internal/bot  0.475s
```

### Focused race

```text
go test -race -count=10 ./internal/store -run '^(TestConsumeTelegramLinkFeatureOffPrivateChatAndRateLimit|TestTelegramLinkAttemptLimiterConcurrentReservationsAndLRUBound)$'
ok  relux.works/duet/coordinator/internal/store  7.087s

go test -race -count=10 ./cmd/duet-coordinator -run '^TestTelegramLinkRateLimitAuditFailureUsesGenericAdapterResponse$'
ok  relux.works/duet/coordinator/cmd/duet-coordinator  3.737s
```

### Deterministic immediate-transaction barriers

```text
go test -count=50 ./internal/store -run '^(TestConsumeTelegramLinkConcurrentSameCodeTwoUsersHasOneWinner|TestConsumeTelegramLinkConcurrentTwoCodesSameUserLeavesLoserCode)$'
ok  relux.works/duet/coordinator/internal/store  13.097s
```

### Full uncached coordinator suite

```text
go test -count=1 ./...
ok  relux.works/duet/coordinator/cmd/duet-coordinator  3.933s
ok  relux.works/duet/coordinator/internal/bot  1.302s
ok  relux.works/duet/coordinator/internal/config  1.655s
ok  relux.works/duet/coordinator/internal/hub  0.511s
ok  relux.works/duet/coordinator/internal/links  0.887s
ok  relux.works/duet/coordinator/internal/media  1.990s
ok  relux.works/duet/coordinator/internal/protocol  2.301s
ok  relux.works/duet/coordinator/internal/resolver  2.906s
ok  relux.works/duet/coordinator/internal/session  2.980s
?   relux.works/duet/coordinator/internal/spotify  [no test files]
ok  relux.works/duet/coordinator/internal/store  7.791s
?   relux.works/duet/coordinator/internal/ulid  [no test files]
```

### Exact pinned previous-head compatibility

```text
go test -tags previoushead -count=1 ./internal/store
ok  relux.works/duet/coordinator/internal/store  15.244s
```

This locally materialized and executed the exact pinned predecessor source via
its real Store API. It is not an external CI claim.

### Full race detector — corrected command-wrapper failure

Initial command:

```text
go test -race -count=1 ./...
```

The first tool wrapper yielded after 30 seconds while the Store package was
still running, returned no exit code, and the wrapper itself raised
`Error: exit undefined`. All packages printed by that attempt had passed; the
wrapper did not retain the remaining process. This was an orchestration error,
not a Go test assertion failure.

The exact command was immediately rerun with an explicit session poll:

```text
go test -race -count=1 ./...
ok  relux.works/duet/coordinator/cmd/duet-coordinator  14.475s
ok  relux.works/duet/coordinator/internal/bot  2.104s
ok  relux.works/duet/coordinator/internal/config  1.391s
ok  relux.works/duet/coordinator/internal/hub  1.726s
ok  relux.works/duet/coordinator/internal/links  2.955s
ok  relux.works/duet/coordinator/internal/media  3.262s
ok  relux.works/duet/coordinator/internal/protocol  2.353s
ok  relux.works/duet/coordinator/internal/resolver  2.428s
ok  relux.works/duet/coordinator/internal/session  2.407s
?   relux.works/duet/coordinator/internal/spotify  [no test files]
ok  relux.works/duet/coordinator/internal/store  31.968s
?   relux.works/duet/coordinator/internal/ulid  [no test files]
```

Corrected result: PASS, exit 0.

### Vet and build

```text
go vet ./...
PASS (exit 0; no output)

go vet -tags previoushead ./internal/store
PASS (exit 0; no output)

go build ./...
PASS (exit 0; no output)
```

### Formatting, whitespace, and static security checks

```text
rg --files . -g '*.go' -0 | xargs -0 gofmt -l
./pulsar-win/player.go
./pulsar-win/ui_windows.go
```

The coordinator module and every R4 file produced no formatting output. The two
listed Windows files are pre-existing sibling-owned outliers and were not
rewritten.

```text
git diff --check
PASS (exit 0; no output)

for f in coordinator/internal/store/identity_telegram.go coordinator/internal/store/identity_telegram_test.go coordinator/cmd/duet-coordinator/telegram_identity_test.go; do out=$(git diff --no-index --check /dev/null "$f" 2>&1 || true); [[ -z "$out" ]] || { print -r -- "$out"; exit 1; }; done
PASS (exit 0; no output)
```

The following scans all passed:

- forbidden `LogEvent`, legacy rate-limit event, raw-ID field, and generic
  JSON sink scan on `identity_telegram.go`: no matches;
- required `RecordRateLimitAudit`, Telegram-consume class, and empty scope:
  exact matches at the new call;
- `INSERT OR REPLACE` across Telegram production/tests: no matches;
- plaintext 27-character link-code literal scan: no matches;
- credential-bearing Telegram URL/token scan: no matches;
- `ConsumeTelegramLink(` call graph: only the Store method, trusted in-process
  loop, previous-head/test callers; no HTTP handler.

### Board validation

Before outcome attachment:

```text
task-board validate
Board is valid. No issues found.
```

The post-attachment validation result is recorded in the task notes and board
resource metadata after this file is attached.

## Dirty worktree and sibling drift

- The repository was already substantially dirty and uncommitted. No reset,
  checkout, clean, commit, or push was performed.
- R4 production/test edits are limited to the four Telegram files hashed above;
  the logbook received exactly two R4 bullets.
- `identity_telegram.go`, `identity_telegram_test.go`, and
  `telegram_identity_test.go` were already untracked task files. `loop.go` and
  `LOGBOOK.md` already contained accepted sibling/task changes; only the
  rate-limit error log field and two R4 logbook bullets were changed here.
- A sibling consolidation rewrote `LOGBOOK.md` after this outcome was first
  drafted, moving its hash from `3468e0a5...` to the exact value above. The
  consolidated file retained both task-owned R4 entries byte-for-byte. No
  Telegram production/test file or frozen shared onboarding/audit file drifted;
  those hashes matched root acceptance before editing and at handoff freeze.
- The previous-head Telegram test remained unchanged and passed.

## External evidence boundaries

- No live Telegram Bot API, production database, bot token, production
  identity, deployment, or external CI run was used or claimed.
- The exact predecessor source was executed locally through its real Store API;
  no separately archived production binary was run.
- All link material in tests is generated at runtime and is never rendered in
  output. No plaintext credential, link code, token-bearing URL, Telegram
  message body, or raw limiter subject appears in this artifact.
- This producer evidence is not acceptance. Fresh independent
  security/compatibility review and a new root line-by-line/hash/test audit
  remain mandatory.

The developer handoff is ready for review.
