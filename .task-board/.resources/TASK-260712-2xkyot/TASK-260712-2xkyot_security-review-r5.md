# TASK-260712-2xkyot — independent Telegram security/compatibility review R5

Date: 2026-07-13  
Role: independent reviewer  
Verdict: **BACK TO DEVELOPMENT**

## Executive result

The frozen R5 production/test boundary matched all nine required SHA-256 hashes. The full requested test, race, previous-head, vet, build, formatting, diff, and board-validation matrix passed. The R4 durable rate-limit audit correction itself is structurally sound: a rejected reservation writes one typed, unscoped, class-domain-separated audit row before returning the rate-limit sentinel, and an audit insert failure remains a consumed reservation while returning a non-sentinel structural error.

One release-blocking redaction defect remains in the production Telegram transport/logging graph. In a private chat, the chat ID is the Telegram user ID used as the limiter subject. Reply-send, source-delete, and outbox-overflow failures log that raw identifier. The focused R4 adapter test uses a fake sender and therefore does not traverse this production logging path; the best-effort-delete test checks only URL/form spellings of the ID and misses the actual structured-log spelling.

## Findings

### HIGH — release blocker: private-chat transport failures log the raw Telegram user/limiter identifier

Evidence:

- `coordinator/internal/bot/bot.go:145-150` logs `chat` with the raw `chatID` when the outbox is full.
- `coordinator/internal/bot/bot.go:155-169` logs raw `chat` on both best-effort deletion failure and reply-send failure, and also logs raw `message` for deletion.
- `coordinator/internal/bot/bot.go:199-212` derives these values directly from the authenticated Telegram Update and captures them in the reply/delete closures.
- `coordinator/cmd/duet-coordinator/loop.go:1383-1414` sends both the durable rate-limit response and the generic audit-persistence-failure response through that reply closure.
- `coordinator/cmd/duet-coordinator/telegram_identity_test.go:405-485` exercises the real Store and loop but injects a fake sender, so it cannot observe transport/outbox logs.
- `coordinator/internal/bot/bot_test.go:462-492` exercises the delete-failure logger, but the assertion excludes only URL/form fragments. The production logger renders the values as structured attributes named `chat` and `message`, so the raw decimal identifiers remain in the captured log while the test passes.

Concrete failure schedule:

1. An authenticated Telegram user submits a syntactically valid private-chat link attempt after the rolling limiter boundary.
2. The Store durably records the rejection, or its durable insert fails and the loop selects the generic response.
3. The loop invokes the Update-derived reply closure.
4. Telegram delivery fails, or the bot outbox is full.
5. The sender/overflow path logs the private-chat ID. For private chats this identifies the Telegram user and is the exact limiter subject before hashing. A successful consume followed by a failed best-effort source deletion has the same leak and additionally logs the source message ID.

This violates the R4 guard's prohibition on including the subject/user ID in errors or logs and the R5 requirement that the raw identifier not survive in logs. It also leaves the transport test insufficient for the actual structured-log rendering.

Minimal remedy:

1. Remove raw `chat` and `message` attributes from the four Telegram outbox failure logs, or replace them with a reviewed non-identifying correlation mechanism. Keep constant operation names and the already-sanitized error.
2. Extend transport tests with sentinel decimal chat/user and message identifiers and assert that the bare decimal values, structured attribute renderings, URL/form renderings, link material, and bot token are absent from the complete captured log/error graph.
3. Exercise both rate-limit-audit success and persistence-failure responses through the real Bot outbox/sender with an injected transport failure; do not stop at a fake loop sender.
4. Re-run the R5 matrix after the correction.

No other release blocker was found.

## Contract and architecture review

The implementation otherwise fits the frozen identity model and onboarding sequence:

- The exact rolling limiter is mutex-protected, keeps the minimum ten timestamps per principal, advances the window on rejected valid attempts, and caps principals with a 10,000-entry LRU (`identity_telegram.go:40-101`).
- Ordering is feature/chat/syntax validation, reservation, durable audit on rejection, then transaction acquisition (`identity_telegram.go:110-142`). The typed audit repository hashes the class-domain-separated subject and writes NULL actor/orbit scope (`security_audit.go:10-24,51-53,64-92`).
- The credential gate is one fixed-shape transaction query with dummy target data and one constant-time submitted-code comparison before validity is combined (`identity_telegram.go:144-212`).
- The production-neutral transaction checkpoint is immediately before the real `db.Begin()`; both concurrency tests signal from that seam and separately observe the preflight read (`identity_telegram.go:135-142`; `identity_telegram_test.go:495-659`). The Store DSN uses `_txlock=immediate` (`store.go:92-99`).
- Actor resolution, revoked-actor handling, additive and legacy conflict checks, conditional consume, conflict-safe membership/legacy UPSERT, audit, and commit stay in one transaction (`identity_telegram.go:215-305`). No reviewed identity mutation uses `INSERT OR REPLACE`.
- Link consumption sanitizes the refreshable Telegram display hint (`identity_telegram.go:215-240,310-327`). Existing same-orbit and foreign-orbit state is checked in both authorities before the code reservation, so link input cannot overwrite migrated roles (`identity_telegram.go:242-265`).
- Installation credentials are not reassigned by link consume. Tests prove the linked Telegram actor remains separate from the app installation owner and that new pair credentials belong to a separate node-only actor.
- Bot command authorization uses shared `ResolveTelegramActorContext`; feature-off falls back to exact legacy membership behavior, while unknown/left and revoked/disabled lifecycle classifications remain distinct (`loop.go:854-873,1356-1381`).
- The only production `ConsumeTelegramLink` caller is the in-process bot loop. Route inspection found only authenticated `/v1/telegram-links` issuance and no public consume endpoint.
- The HTTP adapter clones injected clients, rejects redirects, replaces raw transport/filesystem causes with safe sentinels, and does not render Telegram descriptions, token-bearing URLs, bodies, paths, or file IDs (`bot.go:284-480`). The blocker above is in caller-added structured identifiers, not the sanitized HTTP error object.
- Migration, role/slot preservation, mixed legacy/self-service pair behavior, transfer/leave/revoke behavior, rollback/reconciliation, and pinned predecessor compatibility are covered and green.

The attached class and sequence diagrams were used as review maps: Telegram linking creates/reuses a Telegram actor and membership additively without changing `AppInstallationCredential` ownership, and the one-time consume remains a trusted Bot-to-Store in-process flow rather than an HTTP route.

## Reviewed inventory

Frozen contract/evidence:

- `.task-board/.resources/TASK-260712-3v1k7q/research.md`
- `.task-board/.resources/TASK-260712-3v1k7q/p1-root-review-amendments.md`
- every implementation/rework/review guard attached to this task through R5
- `TASK-260712-2xkyot_security-review-r3.md`
- `TASK-260712-2xkyot_rework-r4-results.md`
- accepted identity-foundation producer, independent-review, and root-acceptance evidence for `TASK-260712-1bpog0`
- `docs/spec-self-contained-audio.md` sections 3.13, 6, 11, 12, 18, and 19
- `p1-identity-model.puml` and `p1-onboarding-flows.puml`

Current production and tests:

- `coordinator/internal/store/identity_telegram.go`
- `coordinator/internal/store/identity_telegram_test.go`
- `coordinator/internal/store/identity_telegram_previous_head_test.go`
- `coordinator/internal/store/security_audit.go`
- `coordinator/internal/store/identity.go`
- `coordinator/internal/store/identity_schema.go`
- `coordinator/internal/store/onboarding.go`
- `coordinator/internal/store/orbits.go`
- `coordinator/internal/store/store.go`
- `coordinator/cmd/duet-coordinator/loop.go`
- `coordinator/cmd/duet-coordinator/telegram_identity_test.go`
- `coordinator/cmd/duet-coordinator/onboarding.go`
- `coordinator/cmd/duet-coordinator/main.go`
- `coordinator/internal/bot/bot.go`
- `coordinator/internal/bot/bot_test.go`
- `coordinator/internal/bot/commands.go`
- relevant shared identity/onboarding/schema/concurrency/rollback tests and the pinned previous-head driver

## Frozen hash verification

All required current hashes matched the R5 mandate exactly:

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

The pinned predecessor suite resolves and archives exact commit `e8bd240664a40b9cc78b974f3c34ad30712e2aa5`; it does not infer compatibility from current tests.

## Independent executable evidence

All commands ran from `coordinator/` unless stated otherwise.

```text
$ go test -count=20 ./internal/store -run '^(TestTelegramMigration|TestConsumeTelegramLink|TestLinkedTelegram|TestTelegramLinkAttemptLimiter|TestTelegramLinkIssue|TestTelegramResolver)'
exit 0
ok  relux.works/duet/coordinator/internal/store  18.663s

$ go test -count=20 ./cmd/duet-coordinator -run '^(TestTelegram|TestMigratedTelegram|TestAppOwnedOrbitTelegram)'
exit 0
ok  relux.works/duet/coordinator/cmd/duet-coordinator  3.469s

$ go test -race -count=10 ./internal/store -run '^(TestConsumeTelegramLinkFeatureOffPrivateChatAndRateLimit|TestTelegramLinkAttemptLimiterConcurrentReservationsAndLRUBound)$'
exit 0
ok  relux.works/duet/coordinator/internal/store  7.487s

$ go test -race -count=10 ./cmd/duet-coordinator -run '^TestTelegramLinkRateLimitAuditFailureUsesGenericAdapterResponse$'
exit 0
ok  relux.works/duet/coordinator/cmd/duet-coordinator  4.070s

$ go test -count=50 ./internal/store -run '^(TestConsumeTelegramLinkConcurrentSameCodeTwoUsersHasOneWinner|TestConsumeTelegramLinkConcurrentTwoCodesSameUserLeavesLoserCode)$'
exit 0
ok  relux.works/duet/coordinator/internal/store  13.707s

$ go test -count=20 ./internal/bot -run '^(TestTelegramLinkCodeCarriesVerifiedUpdateMetadata|TestParseTelegramLinkCodeNormalizationShape|TestHTTPAPITransportErrorsRedactTokenURLsAndRequestMaterial|TestHTTPAPIRedirectsFailClosedWithoutReachingTarget|TestHTTPAPIFilesystemErrorGraphDoesNotExposeDestinationPath|TestHTTPAPIRejectedResponseDoesNotEchoTelegramDescription|TestBestEffortDeleteFailureLogUsesSanitizedHTTPAdapterError)$'
exit 0
ok  relux.works/duet/coordinator/internal/bot  0.445s

$ go test -count=1 ./...
exit 0
ok  relux.works/duet/coordinator/cmd/duet-coordinator  2.446s
ok  relux.works/duet/coordinator/internal/bot  1.295s
ok  relux.works/duet/coordinator/internal/config  2.694s
ok  relux.works/duet/coordinator/internal/hub  1.702s
ok  relux.works/duet/coordinator/internal/links  2.253s
ok  relux.works/duet/coordinator/internal/media  3.012s
ok  relux.works/duet/coordinator/internal/protocol  2.067s
ok  relux.works/duet/coordinator/internal/resolver  2.046s
ok  relux.works/duet/coordinator/internal/session  1.825s
?   relux.works/duet/coordinator/internal/spotify  [no test files]
ok  relux.works/duet/coordinator/internal/store  6.464s
?   relux.works/duet/coordinator/internal/ulid  [no test files]

$ go test -tags previoushead -count=1 ./internal/store
exit 0
ok  relux.works/duet/coordinator/internal/store  14.866s

$ go test -race -count=1 ./...
exit 0
ok  relux.works/duet/coordinator/cmd/duet-coordinator  14.379s
ok  relux.works/duet/coordinator/internal/bot  1.399s
ok  relux.works/duet/coordinator/internal/config  1.744s
ok  relux.works/duet/coordinator/internal/hub  3.049s
ok  relux.works/duet/coordinator/internal/links  2.596s
ok  relux.works/duet/coordinator/internal/media  3.507s
ok  relux.works/duet/coordinator/internal/protocol  2.655s
ok  relux.works/duet/coordinator/internal/resolver  2.734s
ok  relux.works/duet/coordinator/internal/session  2.260s
?   relux.works/duet/coordinator/internal/spotify  [no test files]
ok  relux.works/duet/coordinator/internal/store  31.881s
?   relux.works/duet/coordinator/internal/ulid  [no test files]

$ go vet ./...
exit 0
(no output)

$ go vet -tags previoushead ./internal/store
exit 0
(no output)

$ go build ./...
exit 0
(no output)

$ rg --files -g '*.go' | sort | xargs gofmt -d
exit 0
(no output)

$ git diff --check
exit 0
(no output)

$ task-board validate
exit 0
Board is valid. No issues found.
```

Static/call-graph evidence:

```text
$ if rg -n 'INSERT OR REPLACE' <reviewed identity/Telegram production files>; then exit 1; fi
exit 0
(no matches)

$ if rg -n 'fmt\.Errorf\([^\n]*%w|errors\.Join|url\.Error' coordinator/internal/bot/bot.go; then exit 1; fi
exit 0
(no raw wrapped transport error graph)

$ rg -n 'ConsumeTelegramLink\(' coordinator --glob '*.go' --glob '!**/*_test.go'
exit 0
coordinator/internal/store/identity_telegram.go:106: production method
coordinator/cmd/duet-coordinator/loop.go:1391: trusted in-process bot caller

$ rg -n 'HandleFunc\(' coordinator/cmd/duet-coordinator --glob '*.go' | rg -i 'telegram|onboarding|device|recovery|actor'
exit 0
Only the secure onboarding/create/invite/recovery/actor routes and authenticated `/v1/telegram-links` issuance were listed; no Telegram consume route exists.
```

One initial legacy-sink scan intentionally remains recorded as a corrected false positive:

```text
$ if rg -n 'telegram_link\.rate_limited|LogEvent\(' coordinator/internal/store/identity_telegram.go coordinator/cmd/duet-coordinator/loop.go; then exit 1; fi
exit 1
The broad `LogEvent(` branch matched unrelated existing offline, node-error, and desync events.

$ if rg -n 'telegram_link\.rate_limited|LogEvent\([^\n]*telegram' coordinator/internal/store/identity_telegram.go coordinator/cmd/duet-coordinator/loop.go; then exit 1; fi
exit 0
(no Telegram legacy rate-limit sink)
```

## Dirty-worktree and evidence boundaries

The repository is intentionally dirty with concurrent sibling identity/onboarding, node-app, Windows, docs, CI, and board work. I did not modify production code, tests, specs, existing resources, or sibling files. The nine frozen hashes above establish the reviewed Telegram/onboarding/audit boundary at review time; this review does not accept sibling onboarding or Windows work.

Tests used isolated temporary SQLite databases. No live Telegram request, production database, external CI run, or deployment was performed. That is an evidence boundary, not the reason for rejection.

The exact SHA-256 of this attached review file is recorded in the task notes after attachment, because embedding an exact whole-file digest inside the file would change the digest.
